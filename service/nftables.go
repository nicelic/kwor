package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alireza0/s-ui/logger"
)

// Minimal nftables helper.
//
// Notes:
// - Only works on Linux with nft installed and proper permissions.
// - Uses a dedicated table/chain to avoid interfering with existing firewall rules.
//
// We create:
// - table: inet kwor
// - chain: input  (hook input  priority 0; policy accept;)
// - chain: output (hook output priority 0; policy accept;)
// - chain: forward (hook forward priority 0; policy accept;)
//
// For each port we create 2 rules (both tcp+udp):
// - input : meta l4proto {tcp,udp} th dport <port> counter comment "..."
// - output: meta l4proto {tcp,udp} th sport <port> counter comment "..."

const (
	nftCommandOutputLimit          = 4 << 20
	nftReadSnapshotCacheMaxBytes   = 8 << 20
	nftReadSnapshotCacheMaxEntries = 256

	nftTableOwnershipMarker = "kwor-owner:v1"
	nftOwnershipChain       = "kwor_owner"

	nftFamily           = "inet"
	nftChainIn          = "input"
	nftChainOut         = "output"
	nftChainForward     = "forward"
	nftChainPrerouting  = "prerouting"
	nftChainPostrouting = "postrouting"
	nftNatFamilyIPv4    = "ip"
	nftNatFamilyIPv6    = "ip6"
	// Keep NAT hook priorities numeric. nftables 0.7 and 0.9.0 do not
	// parse the newer dstnat/srcnat aliases, while -100/100 are their exact
	// netfilter priorities and remain valid on current nftables releases.
	nftNatPreroutingPriority  = "-100"
	nftNatPostroutingPriority = "100"
)

var errNftCommandOutputLimit = errors.New("nft command output exceeds the configured limit")

type nftReadSnapshotCacheEntry struct {
	data []byte
}

// nftReadSnapshotCache is enabled only around one sampler round.  It makes
// the default chain and Mihomo integrity/traffic passes reuse the same read
// result, while every mutating nft command invalidates it immediately.
var nftReadSnapshotCache = struct {
	sync.Mutex
	depth          int
	generation     uint64
	supportedKnown bool
	supported      bool
	entries        map[string]nftReadSnapshotCacheEntry
	bytes          int
}{
	entries: make(map[string]nftReadSnapshotCacheEntry),
}

type nftLimitedOutputBuffer struct {
	buffer   bytes.Buffer
	limit    int
	exceeded bool
}

func newNftLimitedOutputBuffer(limit int) *nftLimitedOutputBuffer {
	return &nftLimitedOutputBuffer{limit: limit}
}

func (b *nftLimitedOutputBuffer) Write(value []byte) (int, error) {
	if b == nil || b.limit <= 0 {
		return 0, errNftCommandOutputLimit
	}
	remaining := b.limit - b.buffer.Len()
	if remaining <= 0 {
		b.exceeded = true
		return 0, errNftCommandOutputLimit
	}
	if len(value) > remaining {
		_, _ = b.buffer.Write(value[:remaining])
		b.exceeded = true
		return remaining, errNftCommandOutputLimit
	}
	return b.buffer.Write(value)
}

func (b *nftLimitedOutputBuffer) Bytes() []byte {
	if b == nil {
		return nil
	}
	return b.buffer.Bytes()
}

func (b *nftLimitedOutputBuffer) String() string {
	if b == nil {
		return ""
	}
	return b.buffer.String()
}

func nftCommandOutputError(action string, stdout *nftLimitedOutputBuffer, stderr *nftLimitedOutputBuffer, runErr error) error {
	if (stdout != nil && stdout.exceeded) || (stderr != nil && stderr.exceeded) || errors.Is(runErr, errNftCommandOutputLimit) {
		return fmt.Errorf("%s output exceeds %d bytes", action, nftCommandOutputLimit)
	}
	if runErr == nil {
		return nil
	}
	message := ""
	if stderr != nil {
		message = strings.TrimSpace(stderr.String())
	}
	if message == "" {
		return fmt.Errorf("%s failed: %w", action, runErr)
	}
	return fmt.Errorf("%s failed: %w: %s", action, runErr, message)
}

var (
	nftTable          = loadNftTableName()
	nftHandleRe       = regexp.MustCompile(`handle\s+(\d+)`)
	nftCommentRe      = regexp.MustCompile(`comment\s+"((?:[^"\\]|\\.)*)"`)
	nftCounterBytesRe = regexp.MustCompile(`\bcounter\s+packets\s+\d+\s+bytes\s+(\d+)\b`)
	nftCandidates     = []string{
		"/usr/sbin/nft",
		"/sbin/nft",
		"/usr/bin/nft",
		"/bin/nft",
	}
)

var nftIsLinuxHost = IsSystemPlatformLinux
var nftLookPathFn = exec.LookPath
var nftStatFn = os.Stat

func resolveNftBinaryPath() (string, error) {
	if !nftIsLinuxHost() {
		return "", fmt.Errorf("nft is supported on linux only")
	}
	if path, err := nftLookPathFn("nft"); err == nil && strings.TrimSpace(path) != "" {
		return path, nil
	}
	for _, candidate := range nftCandidates {
		info, err := nftStatFn(candidate)
		if err != nil || info == nil || info.IsDir() {
			continue
		}
		return candidate, nil
	}
	return "", fmt.Errorf("nft binary not found")
}

func nftSupported() bool {
	nftReadSnapshotCache.Lock()
	if nftReadSnapshotCache.depth > 0 && nftReadSnapshotCache.supportedKnown {
		supported := nftReadSnapshotCache.supported
		nftReadSnapshotCache.Unlock()
		return supported
	}
	nftReadSnapshotCache.Unlock()

	supported := nftSupportedFn()
	nftReadSnapshotCache.Lock()
	if nftReadSnapshotCache.depth > 0 {
		nftReadSnapshotCache.supportedKnown = true
		nftReadSnapshotCache.supported = supported
	}
	nftReadSnapshotCache.Unlock()
	return supported
}

var nftSupportedFn = func() bool {
	_, err := resolveNftBinaryPath()
	return err == nil
}

func loadNftTableName() string {
	const fallback = "kwor"
	raw := strings.TrimSpace(os.Getenv("KWOR_NFT_TABLE"))
	if raw == "" {
		return fallback
	}
	valid := regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_-]{0,31}$`)
	if !valid.MatchString(raw) {
		return fallback
	}
	return raw
}

func unescapeNftComment(raw string) string {
	// nft list output escapes backslashes and quotes in comments.
	raw = strings.ReplaceAll(raw, `\\`, `\`)
	raw = strings.ReplaceAll(raw, `\"`, `"`)
	return raw
}

func extractRuleComment(line string) (string, bool) {
	m := nftCommentRe.FindStringSubmatch(line)
	if len(m) != 2 {
		return "", false
	}
	return unescapeNftComment(m[1]), true
}

func ruleLineHasExactComment(line string, comment string) bool {
	current, ok := extractRuleComment(line)
	if !ok {
		return false
	}
	return current == comment
}

func runNft(args ...string) ([]byte, error) {
	readOnly := nftReadOnlyCommand(args)
	key := strings.Join(args, "\x00")
	cacheGeneration := uint64(0)
	if readOnly {
		nftReadSnapshotCache.Lock()
		if nftReadSnapshotCache.depth > 0 {
			if cached, ok := nftReadSnapshotCache.entries[key]; ok {
				data := append([]byte(nil), cached.data...)
				nftReadSnapshotCache.Unlock()
				return data, nil
			}
			cacheGeneration = nftReadSnapshotCache.generation
		}
		nftReadSnapshotCache.Unlock()
	} else {
		invalidateNftReadSnapshotCache()
	}

	data, err := runNftFn(args...)
	if readOnly && err == nil {
		nftReadSnapshotCache.Lock()
		if nftReadSnapshotCache.depth > 0 && nftReadSnapshotCache.generation == cacheGeneration &&
			len(nftReadSnapshotCache.entries) < nftReadSnapshotCacheMaxEntries &&
			len(data) <= nftReadSnapshotCacheMaxBytes &&
			nftReadSnapshotCache.bytes <= nftReadSnapshotCacheMaxBytes-len(data) {
			cachedData := append([]byte(nil), data...)
			nftReadSnapshotCache.entries[key] = nftReadSnapshotCacheEntry{data: cachedData}
			nftReadSnapshotCache.bytes += len(cachedData)
		}
		nftReadSnapshotCache.Unlock()
	} else if !readOnly {
		// A read can finish while this mutation is in flight. Invalidate again
		// after the command returns so that old output can never be reinserted
		// into the active sampler snapshot.
		invalidateNftReadSnapshotCache()
	}
	return data, err
}

func nftReadOnlyCommand(args []string) bool {
	for index, arg := range args {
		if strings.EqualFold(strings.TrimSpace(arg), "list") {
			return index+1 < len(args)
		}
	}
	return false
}

// WithNftReadSnapshot coalesces read-only nft commands performed by one
// serialized sampler round.  It is deliberately scoped and never caches
// mutations or failed reads.
func WithNftReadSnapshot(fn func()) {
	if fn == nil {
		return
	}
	nftReadSnapshotCache.Lock()
	if nftReadSnapshotCache.depth == 0 {
		nftReadSnapshotCache.entries = make(map[string]nftReadSnapshotCacheEntry)
		nftReadSnapshotCache.supportedKnown = false
		nftReadSnapshotCache.supported = false
		nftReadSnapshotCache.bytes = 0
	}
	nftReadSnapshotCache.depth++
	nftReadSnapshotCache.Unlock()
	defer func() {
		nftReadSnapshotCache.Lock()
		if nftReadSnapshotCache.depth > 0 {
			nftReadSnapshotCache.depth--
		}
		if nftReadSnapshotCache.depth == 0 {
			nftReadSnapshotCache.entries = make(map[string]nftReadSnapshotCacheEntry)
			nftReadSnapshotCache.supportedKnown = false
			nftReadSnapshotCache.supported = false
			nftReadSnapshotCache.bytes = 0
		}
		nftReadSnapshotCache.Unlock()
	}()
	fn()
}

func invalidateNftReadSnapshotCache() {
	nftReadSnapshotCache.Lock()
	nftReadSnapshotCache.generation++
	nftReadSnapshotCache.entries = make(map[string]nftReadSnapshotCacheEntry)
	nftReadSnapshotCache.bytes = 0
	nftReadSnapshotCache.Unlock()
}

var runNftFn = func(args ...string) ([]byte, error) {
	binaryPath, err := resolveNftBinaryPath()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binaryPath, args...)
	stdout := newNftLimitedOutputBuffer(nftCommandOutputLimit)
	stderr := newNftLimitedOutputBuffer(nftCommandOutputLimit)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	err = cmd.Run()
	if commandErr := nftCommandOutputError("nft "+strings.Join(args, " "), stdout, stderr, err); commandErr != nil {
		return nil, commandErr
	}
	return append([]byte(nil), stdout.Bytes()...), nil
}

func runNftScript(script string) ([]byte, error) {
	invalidateNftReadSnapshotCache()
	data, err := runNftScriptFn(script)
	// A read may begin after the pre-script invalidation but finish before the
	// nft batch commits. Invalidate again so that pre-commit output cannot be
	// inserted into the active sampler snapshot after this mutation returns.
	invalidateNftReadSnapshotCache()
	return data, err
}

var runNftScriptFn = func(script string) ([]byte, error) {
	binaryPath, err := resolveNftBinaryPath()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binaryPath, "-f", "-")
	cmd.Stdin = strings.NewReader(script)
	stdout := newNftLimitedOutputBuffer(nftCommandOutputLimit)
	stderr := newNftLimitedOutputBuffer(nftCommandOutputLimit)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	err = cmd.Run()
	if commandErr := nftCommandOutputError("nft script", stdout, stderr, err); commandErr != nil {
		return nil, commandErr
	}
	return append([]byte(nil), stdout.Bytes()...), nil
}

func nftTableHasOwnershipMarker(output []byte) bool {
	tokens := tokenizeNftOwnershipOutput(string(output))
	depth := 0
	seenTable := false
	ownerChainDepth := 0
	pendingOwnerChain := false
	statementHasCounter := false
	statementHasMarker := false
	resetStatement := func() {
		statementHasCounter = false
		statementHasMarker = false
	}
	for index, token := range tokens {
		switch token.value {
		case "table":
			if depth == 0 {
				seenTable = true
			}
		case "chain":
			pendingOwnerChain = seenTable && depth == 1 && index+1 < len(tokens) &&
				strings.EqualFold(strings.TrimSpace(tokens[index+1].value), nftOwnershipChain)
		case "{":
			if seenTable {
				depth++
				if pendingOwnerChain {
					ownerChainDepth = depth
					pendingOwnerChain = false
					resetStatement()
				}
			}
		case "}":
			if ownerChainDepth > 0 && depth == ownerChainDepth {
				if statementHasCounter && statementHasMarker {
					return true
				}
				ownerChainDepth = 0
				resetStatement()
			}
			if seenTable && depth > 0 {
				depth--
			}
		case ";", "\n":
			if ownerChainDepth > 0 && depth == ownerChainDepth {
				if statementHasCounter && statementHasMarker {
					return true
				}
				resetStatement()
			}
		default:
			if seenTable && depth == 1 && strings.EqualFold(token.value, "comment") && index+1 < len(tokens) {
				next := tokens[index+1]
				if next.quoted && strings.EqualFold(strings.TrimSpace(next.value), nftTableOwnershipMarker) {
					return true
				}
			}
			if ownerChainDepth > 0 && depth == ownerChainDepth {
				if strings.EqualFold(token.value, "counter") {
					statementHasCounter = true
				}
				if strings.EqualFold(token.value, "comment") && index+1 < len(tokens) {
					next := tokens[index+1]
					statementHasMarker = next.quoted && strings.EqualFold(strings.TrimSpace(next.value), nftTableOwnershipMarker)
				}
			}
		}
	}
	return false
}

type nftOwnershipToken struct {
	value  string
	quoted bool
}

func tokenizeNftOwnershipOutput(value string) []nftOwnershipToken {
	tokens := make([]nftOwnershipToken, 0, 32)
	for index := 0; index < len(value); {
		switch value[index] {
		case ' ', '\t', '\r':
			index++
			continue
		case '\n', ';':
			tokens = append(tokens, nftOwnershipToken{value: string(value[index])})
			index++
			continue
		case '{', '}':
			tokens = append(tokens, nftOwnershipToken{value: string(value[index])})
			index++
			continue
		case '"':
			start := index
			index++
			for index < len(value) {
				if value[index] == '\\' && index+1 < len(value) {
					index += 2
					continue
				}
				if value[index] == '"' {
					index++
					break
				}
				index++
			}
			raw := value[start:index]
			unquoted, err := strconv.Unquote(raw)
			if err != nil {
				continue
			}
			tokens = append(tokens, nftOwnershipToken{value: unquoted, quoted: true})
			continue
		}
		start := index
		for index < len(value) && !strings.ContainsRune(" \t\r\n;{}\"", rune(value[index])) {
			index++
		}
		if index > start {
			tokens = append(tokens, nftOwnershipToken{value: value[start:index]})
		}
	}
	return tokens
}

func nftTableHasTrustedKworRuleEvidence(tableName string, output []byte) bool {
	knownPrefixes := []string{}
	knownChains := []string{}
	switch tableName {
	case nftTable:
		knownPrefixes = []string{
			"kwor_inbound_",
			"kwor_client_limit_",
			"kwor_client_block_",
			"kwor_mihomo_inbound_",
			"kwor_mihomo_client_limit_",
			"kwor_mihomo_client_block_",
			"kwor_traffic_cap_",
		}
		knownChains = []string{nftChainIn, nftChainOut, nftChainForward, nftChainPrerouting, nftChainPostrouting}
	case firewallNftTable:
		knownPrefixes = []string{"kwor_firewall_static_", "kwor_firewall_rule_", "kwor_firewall_geo_"}
		knownChains = []string{firewallInputChain}
	case portForwardNftTable:
		knownPrefixes = []string{"kwor_pf_rule_", "kwor_pf_counter_", "kwor_pf_meter_"}
		knownChains = []string{
			portForwardPreroutingChain,
			portForwardPostroutingChain,
			portForwardForwardChain,
			portForwardInputChain,
			portForwardOutputChain,
		}
	default:
		return false
	}
	seenComment := false
	for _, line := range strings.Split(string(output), "\n") {
		comment, ok := extractRuleComment(line)
		if !ok {
			continue
		}
		comment = strings.ToLower(strings.TrimSpace(comment))
		for _, prefix := range knownPrefixes {
			if strings.HasPrefix(comment, prefix) {
				seenComment = true
				break
			}
		}
		if seenComment {
			break
		}
	}
	if !seenComment {
		return false
	}
	lower := strings.ToLower(string(output))
	for _, chain := range knownChains {
		if strings.Contains(lower, "chain "+strings.ToLower(chain)) {
			return true
		}
	}
	return false
}

func nftManifestOwnsActiveTable(tableFamily string, tableName string) (bool, error) {
	manifest, found, err := LoadHostOwnershipManifest()
	if err != nil || !found || manifest == nil {
		return false, err
	}
	if hostID := strings.TrimSpace(manifest.HostID); hostID != "" && hostID != hostOwnershipHostID() {
		return false, nil
	}
	for _, resource := range manifest.Resources {
		if resource.Kind != HostResourceNftTable || resource.State != hostResourceStateActive {
			continue
		}
		for _, table := range resource.NftTables {
			if table.Family == tableFamily && table.Name == tableName {
				return true, nil
			}
		}
	}
	return false, nil
}

func inspectOwnedNftTableForMutation(tableFamily string, tableName string) (bool, error) {
	output, err := runNft("list", "table", tableFamily, tableName)
	if err != nil {
		if nftObjectMissing(err) {
			return false, nil
		}
		return false, err
	}
	if nftTableHasOwnershipMarker(output) {
		return true, nil
	}
	if strings.TrimSpace(string(output)) == "" {
		return false, fmt.Errorf("refuse to modify nft table with empty ownership evidence: %s %s", tableFamily, tableName)
	}
	manifestOwned, manifestErr := nftManifestOwnsActiveTable(tableFamily, tableName)
	if manifestErr != nil {
		return false, manifestErr
	}
	if manifestOwned && nftTableHasTrustedKworRuleEvidence(tableName, output) {
		return true, nil
	}
	return false, fmt.Errorf("refuse to modify nft table without kwor ownership proof: %s %s", tableFamily, tableName)
}

func nftTableIsSafeToDelete(tableFamily string, tableName string, output []byte) (bool, error) {
	if nftTableHasOwnershipMarker(output) {
		return true, nil
	}
	if strings.TrimSpace(string(output)) == "" {
		return false, nil
	}
	manifestOwned, err := nftManifestOwnsActiveTable(tableFamily, tableName)
	if err != nil {
		return false, err
	}
	if manifestOwned && nftTableHasTrustedKworRuleEvidence(tableName, output) {
		return true, nil
	}
	// A manifest may already have been removed by an older release. Only the
	// exact project comments and matching project chains are strong enough to
	// adopt and delete that historic table.
	return nftTableHasTrustedKworRuleEvidence(tableName, output), nil
}

func deleteOwnedNftTableForRuntime(tableFamily string, tableName string) error {
	output, err := runNft("list", "table", tableFamily, tableName)
	if err != nil {
		if nftObjectMissing(err) {
			return nil
		}
		return err
	}
	owned, err := nftTableIsSafeToDelete(tableFamily, tableName, output)
	if err != nil {
		return err
	}
	if !owned {
		return fmt.Errorf("refuse to delete nft table without kwor ownership proof: %s %s", tableFamily, tableName)
	}
	_, err = runNft("delete", "table", tableFamily, tableName)
	return err
}

func ensureNftBase() error {
	if !nftSupported() {
		return nil
	}
	if err := ensureNftRendererSupported(); err != nil {
		return err
	}

	// Ensure table
	exists, err := inspectOwnedNftTableForMutation(nftFamily, nftTable)
	if err != nil {
		return err
	}
	if !exists {
		if addErr := createOwnedNftTable("nft-default-"+nftFamily, nftFamily, nftTable); addErr != nil {
			return addErr
		}
	}

	// Ensure base chains (hook input/output). Policy accept to avoid changing behavior.
	_, err = runNft("list", "chain", nftFamily, nftTable, nftChainIn)
	if err != nil {
		_, addErr := runNft(
			"add", "chain", nftFamily, nftTable, nftChainIn,
			"{", "type", "filter", "hook", "input", "priority", "0", ";", "policy", "accept", ";", "}",
		)
		if addErr != nil {
			return addErr
		}
	}

	_, err = runNft("list", "chain", nftFamily, nftTable, nftChainOut)
	if err != nil {
		_, addErr := runNft(
			"add", "chain", nftFamily, nftTable, nftChainOut,
			"{", "type", "filter", "hook", "output", "priority", "0", ";", "policy", "accept", ";", "}",
		)
		if addErr != nil {
			return addErr
		}
	}

	_, err = runNft("list", "chain", nftFamily, nftTable, nftChainForward)
	if err != nil {
		_, addErr := runNft(
			"add", "chain", nftFamily, nftTable, nftChainForward,
			"{", "type", "filter", "hook", "forward", "priority", "0", ";", "policy", "accept", ";", "}",
		)
		if addErr != nil {
			return addErr
		}
	}

	return nil
}

// ensureNftNatChain ensures the NAT chains for port hopping REDIRECT rules.
// nftables before Linux 5.2 cannot attach NAT base chains to inet tables, so
// the compatibility layout keeps filter rules in inet and creates dedicated
// ip/ip6 NAT tables with the same managed table name.
func ensureNftNatChain() error {
	if !nftSupported() {
		return nil
	}
	if err := ensureNftBase(); err != nil {
		return err
	}
	if nftUsesCompatibilityLayout() {
		if err := removeNativeNftNatChain(); err != nil {
			return err
		}
		return ensureNftCompatibilityNatChains()
	}
	if err := cleanupNftCompatibilityNatTables(); err != nil {
		return err
	}

	_, err := ensureManagedNftNatChain(
		nftFamily,
		nftChainPrerouting,
		nftPreroutingNatChainSpec(),
	)
	return err
}

func nftCompatibilityNatFamilies() []string {
	return []string{nftNatFamilyIPv4, nftNatFamilyIPv6}
}

func nftNatTableExists(tableFamily string, table string) bool {
	if !nftSupported() {
		return false
	}
	exists, err := inspectOwnedNftTableForMutation(tableFamily, table)
	return err == nil && exists
}

func nftPreroutingNatChainSpec() []string {
	return []string{"type", "nat", "hook", "prerouting", "priority", nftNatPreroutingPriority, ";", "policy", "accept", ";"}
}

func nftPostroutingNatChainSpec() []string {
	return []string{"type", "nat", "hook", "postrouting", "priority", nftNatPostroutingPriority, ";", "policy", "accept", ";"}
}

func ensureNftCompatibilityNatChains() error {
	createdTables := make(map[string]struct{})
	createdChains := make([]nftManagedNatChain, 0, len(nftCompatibilityNatFamilies())*2)
	rollback := func() {
		// A table created by this call owns all of its just-created chains, so
		// deleting the table is both safer and more complete. Chains added to a
		// pre-existing panel table must be removed individually.
		for i := len(createdChains) - 1; i >= 0; i-- {
			chain := createdChains[i]
			if _, createdTable := createdTables[chain.tableFamily]; createdTable {
				continue
			}
			_, _ = runNft("flush", "chain", chain.tableFamily, nftTable, chain.name)
			_, _ = runNft("delete", "chain", chain.tableFamily, nftTable, chain.name)
		}
		for _, tableFamily := range nftCompatibilityNatFamilies() {
			if _, createdTable := createdTables[tableFamily]; !createdTable {
				continue
			}
			_, _ = runNft("delete", "table", tableFamily, nftTable)
		}
	}

	for _, tableFamily := range nftCompatibilityNatFamilies() {
		createdTable, err := ensureManagedNftNatTable(tableFamily)
		if err != nil {
			rollback()
			return err
		}
		if createdTable {
			createdTables[tableFamily] = struct{}{}
		}
		createdChain, err := ensureManagedNftNatChain(
			tableFamily,
			nftChainPrerouting,
			nftPreroutingNatChainSpec(),
		)
		if err != nil {
			rollback()
			return err
		}
		if createdChain {
			createdChains = append(createdChains, nftManagedNatChain{tableFamily: tableFamily, name: nftChainPrerouting})
		}
		// Linux kernels before 4.18 require both NAT base hooks to be present
		// for reply-path NAT. Keeping postrouting in every compatibility table
		// is harmless on newer kernels and makes the layout deterministic.
		createdChain, err = ensureManagedNftNatChain(
			tableFamily,
			nftChainPostrouting,
			nftPostroutingNatChainSpec(),
		)
		if err != nil {
			rollback()
			return err
		}
		if createdChain {
			createdChains = append(createdChains, nftManagedNatChain{tableFamily: tableFamily, name: nftChainPostrouting})
		}
	}
	return nil
}

type nftManagedNatChain struct {
	tableFamily string
	name        string
}

func ensureManagedNftNatTable(tableFamily string) (bool, error) {
	if exists, err := inspectOwnedNftTableForMutation(tableFamily, nftTable); err != nil {
		return false, err
	} else if exists {
		return false, nil
	}
	if err := createOwnedNftTable("nft-default-"+tableFamily, tableFamily, nftTable); err != nil {
		return false, err
	}
	return true, nil
}

func createOwnedNftTable(id string, tableFamily string, tableName string) error {
	if err := ensureNftRendererSupported(); err != nil {
		return err
	}
	if exists, err := inspectOwnedNftTableForMutation(tableFamily, tableName); err != nil {
		return err
	} else if exists {
		return nil
	}
	ownership, err := BeginNftHostOwnership(id, []HostNftTable{{Family: tableFamily, Name: tableName}})
	if err != nil {
		return fmt.Errorf("record nft table ownership before creation: %w", err)
	}
	script := &strings.Builder{}
	appendOwnedNftTableCreationScript(script, tableFamily, tableName)
	if _, err := runNftScript(script.String()); err != nil {
		return finishUnconfirmedNftCreationFailure(ownership, tableFamily, tableName, err)
	}
	if output, verifyErr := runNft("list", "table", tableFamily, tableName); verifyErr != nil || !nftTableHasOwnershipMarker(output) {
		if verifyErr != nil {
			return rollbackConfirmedNftCreation(ownership, tableFamily, tableName, fmt.Errorf("verify nft table ownership marker: %w", verifyErr))
		}
		return rollbackConfirmedNftCreation(ownership, tableFamily, tableName, fmt.Errorf("nft table ownership marker is missing after creation: %s %s", tableFamily, tableName))
	}
	if ownership.ID != "" {
		if err := ActivateHostResource(ownership.ID); err != nil {
			return rollbackConfirmedNftCreation(ownership, tableFamily, tableName, fmt.Errorf("activate nft table ownership: %w", err))
		}
	}
	return nil
}

func appendOwnedNftTableCreationScript(script *strings.Builder, tableFamily string, tableName string) {
	if GetNftablesCapabilities().SupportsTableComments {
		script.WriteString(fmt.Sprintf("add table %s %s { comment %q; }\n", tableFamily, tableName, nftTableOwnershipMarker))
	} else {
		script.WriteString(fmt.Sprintf("add table %s %s\n", tableFamily, tableName))
	}
	script.WriteString(fmt.Sprintf("add chain %s %s %s\n", tableFamily, tableName, nftOwnershipChain))
	script.WriteString(fmt.Sprintf(
		"add rule %s %s %s counter comment %q\n",
		tableFamily,
		tableName,
		nftOwnershipChain,
		nftTableOwnershipMarker,
	))
}

func finishUnconfirmedNftCreationFailure(ownership HostResource, tableFamily string, tableName string, cause error) error {
	output, err := runNft("list", "table", tableFamily, tableName)
	if err != nil && nftObjectMissing(err) {
		if ownership.ID != "" {
			if removeErr := RemoveHostResource(ownership.ID); removeErr != nil {
				return fmt.Errorf("%w; clear absent nft ownership record: %v", cause, removeErr)
			}
		}
		return cause
	}
	if err != nil {
		return fmt.Errorf("%w; inspect failed nft creation result: %v", cause, err)
	}
	if !nftTableHasOwnershipMarker(output) {
		return fmt.Errorf("%w; a same-name nft table appeared without kwor ownership proof", cause)
	}
	return rollbackConfirmedNftCreation(ownership, tableFamily, tableName, cause)
}

func rollbackConfirmedNftCreation(ownership HostResource, tableFamily string, tableName string, cause error) error {
	if _, err := runNft("delete", "table", tableFamily, tableName); err != nil && !nftObjectMissing(err) {
		return fmt.Errorf("%w; rollback nft table %s %s failed: %v", cause, tableFamily, tableName, err)
	}
	if _, err := runNft("list", "table", tableFamily, tableName); err == nil {
		return fmt.Errorf("%w; rollback nft table %s %s did not remove the table", cause, tableFamily, tableName)
	} else if !nftObjectMissing(err) {
		return fmt.Errorf("%w; verify nft table rollback failed: %v", cause, err)
	}
	if ownership.ID != "" {
		if err := RemoveHostResource(ownership.ID); err != nil {
			return fmt.Errorf("%w; clear rolled-back nft ownership record: %v", cause, err)
		}
	}
	return cause
}

func ensureManagedNftNatChain(tableFamily string, chainName string, spec []string) (bool, error) {
	if _, err := runNft("list", "chain", tableFamily, nftTable, chainName); err == nil {
		return false, nil
	} else if !nftObjectMissing(err) {
		return false, err
	}
	args := []string{"add", "chain", tableFamily, nftTable, chainName, "{"}
	args = append(args, spec...)
	args = append(args, "}")
	if _, err := runNft(args...); err != nil {
		return false, err
	}
	return true, nil
}

func removeNativeNftNatChain() error {
	if exists, err := inspectOwnedNftTableForMutation(nftFamily, nftTable); err != nil {
		return err
	} else if !exists {
		return nil
	}
	var firstErr error
	for _, chain := range []string{nftChainPrerouting, nftChainPostrouting} {
		if _, err := runNft("list", "chain", nftFamily, nftTable, chain); err != nil {
			if !nftObjectMissing(err) && firstErr == nil {
				firstErr = err
			}
			continue
		}
		if _, err := runNft("flush", "chain", nftFamily, nftTable, chain); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if _, err := runNft("delete", "chain", nftFamily, nftTable, chain); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func cleanupNftCompatibilityNatTables() error {
	var firstErr error
	for _, tableFamily := range nftCompatibilityNatFamilies() {
		if err := deleteOwnedNftTableForRuntime(tableFamily, nftTable); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func nftTransportPortMatchArgs(direction string) []string {
	if GetNftablesCapabilities().SupportsTransportHeader {
		return []string{"th", direction}
	}
	if direction == "sport" {
		return []string{"@th,0,16"}
	}
	return []string{"@th,16,16"}
}

func appendNftTransportPortMatch(args []string, direction string) []string {
	return append(args, nftTransportPortMatchArgs(direction)...)
}

func addPortCounterRule(chain string, port int, direction string, comment string) (int, error) {
	// direction: "dport" or "sport"
	if !nftSupported() {
		return 0, nil
	}
	if err := ensureNftBase(); err != nil {
		return 0, err
	}

	// First add the rule without --handle to avoid output parsing issues
	args := []string{
		"add", "rule",
		nftFamily, nftTable, chain,
		"meta", "l4proto", "{", "tcp", ",", "udp", "}",
	}
	args = appendNftTransportPortMatch(args, direction)
	args = append(args, fmt.Sprint(port), "counter", "comment", comment)
	_, err := runNft(args...)
	if err != nil {
		return 0, err
	}

	// Then list the chain with --handle to find the newly created rule's handle
	listArgs := []string{
		"--handle", "--numeric", "list", "chain",
		nftFamily, nftTable, chain,
	}
	out, err := runNft(listArgs...)
	if err != nil {
		return 0, fmt.Errorf("failed to list chain after adding rule: %w", err)
	}

	// Find the handle for our comment
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		if ruleLineHasExactComment(line, comment) && strings.Contains(line, "handle") {
			m := nftHandleRe.FindStringSubmatch(line)
			if len(m) == 2 {
				handle := 0
				_, _ = fmt.Sscanf(m[1], "%d", &handle)
				if handle > 0 {
					return handle, nil
				}
			}
		}
	}

	// If we can't find the handle, log warning but don't fail
	// The rule was created successfully, we just can't track its handle
	logger.Warning("nftables rule created but handle not found for comment: ", comment)
	return 0, nil
}

// addPortRateLimitRule creates a "drop when over rate" rule for one direction on one port.
// bytesPerSecond uses decimal bytes/second (for example 25,000,000 for 200 Mbps).
func addPortRateLimitRule(chain string, port int, direction string, bytesPerSecond int64, comment string) (int, error) {
	if !nftSupported() {
		return 0, nil
	}
	if port <= 0 || bytesPerSecond <= 0 {
		return 0, nil
	}
	if err := ensureNftBase(); err != nil {
		return 0, err
	}

	args := []string{
		"add", "rule",
		nftFamily, nftTable, chain,
		"meta", "l4proto", "{", "tcp", ",", "udp", "}",
	}
	args = appendNftTransportPortMatch(args, direction)
	args = append(args,
		fmt.Sprint(port),
		"limit", "rate", "over", fmt.Sprint(bytesPerSecond), "bytes/second",
		"counter", "drop", "comment", comment,
	)
	if _, err := runNft(args...); err != nil {
		return 0, err
	}

	handle := findHandleByComment(chain, comment)
	if handle > 0 {
		return handle, nil
	}
	logger.Warning("nftables rate limit rule created but handle not found for comment: ", comment)
	return 0, nil
}

// addPortDropRule creates an unconditional drop rule for one direction on one port.
func addPortDropRule(chain string, port int, direction string, comment string) (int, error) {
	if !nftSupported() {
		return 0, nil
	}
	if port <= 0 {
		return 0, nil
	}
	if err := ensureNftBase(); err != nil {
		return 0, err
	}

	args := []string{
		"add", "rule",
		nftFamily, nftTable, chain,
		"meta", "l4proto", "{", "tcp", ",", "udp", "}",
	}
	args = appendNftTransportPortMatch(args, direction)
	args = append(args, fmt.Sprint(port), "counter", "drop", "comment", comment)
	if _, err := runNft(args...); err != nil {
		return 0, err
	}

	handle := findHandleByComment(chain, comment)
	if handle > 0 {
		return handle, nil
	}
	logger.Warning("nftables block rule created but handle not found for comment: ", comment)
	return 0, nil
}

func addPortRangeDropRule(chain string, direction string, ranges []portRange, comment string) (int, error) {
	if !nftSupported() {
		return 0, nil
	}
	if direction != "dport" && direction != "sport" {
		return 0, fmt.Errorf("invalid direction: %s", direction)
	}
	portSetArgs := buildNftPortSetArgsFromRanges(ranges)
	if len(portSetArgs) == 0 {
		return 0, nil
	}
	if err := ensureNftBase(); err != nil {
		return 0, err
	}

	args := []string{
		"add", "rule",
		nftFamily, nftTable, chain,
		"meta", "l4proto", "{", "tcp", ",", "udp", "}",
	}
	args = appendNftTransportPortMatch(args, direction)
	args = append(args, portSetArgs...)
	args = append(args, "counter", "drop", "comment", comment)

	if _, err := runNft(args...); err != nil {
		return 0, err
	}

	handle := findHandleByComment(chain, comment)
	if handle > 0 {
		return handle, nil
	}
	logger.Warning("nftables block-range rule created but handle not found for comment: ", comment)
	return 0, nil
}

func normalizePortList(ports []int) []int {
	if len(ports) == 0 {
		return nil
	}
	seen := make(map[int]struct{}, len(ports))
	normalized := make([]int, 0, len(ports))
	for _, port := range ports {
		if port < 1 || port > 65535 {
			continue
		}
		if _, exists := seen[port]; exists {
			continue
		}
		seen[port] = struct{}{}
		normalized = append(normalized, port)
	}
	sort.Ints(normalized)
	return normalized
}

func buildNftPortSetArgsFromInts(ports []int) []string {
	normalized := normalizePortList(ports)
	if len(normalized) == 0 {
		return nil
	}
	if len(normalized) == 1 {
		return []string{fmt.Sprint(normalized[0])}
	}

	args := []string{"{"}
	for index, port := range normalized {
		if index > 0 {
			args = append(args, ",")
		}
		args = append(args, fmt.Sprint(port))
	}
	args = append(args, "}")
	return args
}

func normalizeNftPortRanges(ranges []portRange) []portRange {
	if len(ranges) == 0 {
		return nil
	}

	normalized := make([]portRange, 0, len(ranges))
	for _, current := range ranges {
		start := current.start
		end := current.end
		if start > end {
			start, end = end, start
		}
		if end < 1 || start > 65535 {
			continue
		}
		if start < 1 {
			start = 1
		}
		if end > 65535 {
			end = 65535
		}
		normalized = append(normalized, portRange{start: start, end: end})
	}
	if len(normalized) == 0 {
		return nil
	}
	return mergePortRanges(normalized)
}

func buildNftPortSetArgsFromRanges(ranges []portRange) []string {
	normalized := normalizeNftPortRanges(ranges)
	if len(normalized) == 0 {
		return nil
	}
	if len(normalized) == 1 {
		item := normalized[0]
		if item.start == item.end {
			return []string{fmt.Sprint(item.start)}
		}
		return []string{fmt.Sprintf("%d-%d", item.start, item.end)}
	}

	args := []string{"{"}
	for index, current := range normalized {
		if index > 0 {
			args = append(args, ",")
		}
		if current.start == current.end {
			args = append(args, fmt.Sprint(current.start))
		} else {
			args = append(args, fmt.Sprintf("%d-%d", current.start, current.end))
		}
	}
	args = append(args, "}")
	return args
}

func addLoopbackAcceptRule(chain string, comment string) (int, error) {
	if !nftSupported() {
		return 0, nil
	}
	if err := ensureNftBase(); err != nil {
		return 0, err
	}

	args := []string{
		"add", "rule",
		nftFamily, nftTable, chain,
	}
	if chain == nftChainIn {
		args = append(args, "iifname", "lo")
	} else {
		args = append(args, "oifname", "lo")
	}
	args = append(args, "counter", "accept", "comment", comment)

	if _, err := runNft(args...); err != nil {
		return 0, err
	}

	handle := findHandleByComment(chain, comment)
	if handle > 0 {
		return handle, nil
	}
	logger.Warning("nftables loopback allow rule created but handle not found for comment: ", comment)
	return 0, nil
}

func addDropExceptPortsRule(chain string, direction string, allowedPorts []int, comment string) (int, error) {
	if !nftSupported() {
		return 0, nil
	}
	if direction != "dport" && direction != "sport" {
		return 0, fmt.Errorf("invalid direction: %s", direction)
	}
	portSetArgs := buildNftPortSetArgsFromInts(allowedPorts)
	if len(portSetArgs) == 0 {
		return 0, fmt.Errorf("allowed ports are empty")
	}
	if err := ensureNftBase(); err != nil {
		return 0, err
	}

	args := []string{
		"add", "rule",
		nftFamily, nftTable, chain,
		"meta", "l4proto", "{", "tcp", ",", "udp", "}",
	}
	args = appendNftTransportPortMatch(args, direction)
	args = append(args, "!=")
	args = append(args, portSetArgs...)
	args = append(args, "counter", "drop", "comment", comment)

	if _, err := runNft(args...); err != nil {
		return 0, err
	}

	handle := findHandleByComment(chain, comment)
	if handle > 0 {
		return handle, nil
	}
	logger.Warning("nftables drop-except-ports rule created but handle not found for comment: ", comment)
	return 0, nil
}

func addDropAllTransportRule(chain string, comment string) (int, error) {
	if !nftSupported() {
		return 0, nil
	}
	if err := ensureNftBase(); err != nil {
		return 0, err
	}

	args := []string{
		"add", "rule",
		nftFamily, nftTable, chain,
		"meta", "l4proto", "{", "tcp", ",", "udp", "}",
		"counter", "drop",
		"comment", comment,
	}
	if _, err := runNft(args...); err != nil {
		return 0, err
	}

	handle := findHandleByComment(chain, comment)
	if handle > 0 {
		return handle, nil
	}
	logger.Warning("nftables drop-all transport rule created but handle not found for comment: ", comment)
	return 0, nil
}

// addRedirectRule creates a UDP REDIRECT rule in nat/prerouting.
// Kept for backward compatibility with existing port-hopping flows.
func addRedirectRule(hopPortsNft string, listenPort int, comment string) (int, error) {
	return addRedirectRuleWithProtocols(hopPortsNft, listenPort, comment, false)
}

// addRedirectRuleWithProtocols creates a REDIRECT rule in nat/prerouting to forward
// a configured dport set to listenPort.
// When includeTCP is true, both TCP and UDP are redirected; otherwise UDP only.
func addRedirectRuleWithProtocols(hopPortsNft string, listenPort int, comment string, includeTCP bool) (int, error) {
	if !nftSupported() {
		return 0, nil
	}
	if hopPortsNft == "" || listenPort <= 0 {
		return 0, nil
	}
	if err := ensureNftNatChain(); err != nil {
		return 0, err
	}
	if nftUsesCompatibilityLayout() {
		return addCompatibilityRedirectRule(hopPortsNft, listenPort, comment, includeTCP)
	}
	return addNativeRedirectRule(hopPortsNft, listenPort, comment, includeTCP)
}

func addNativeRedirectRule(hopPortsNft string, listenPort int, comment string, includeTCP bool) (int, error) {

	// Build port set args: { 899-999 , 5000-6000 }
	portSetArgs := buildNftPortSetArgs(hopPortsNft)
	if len(portSetArgs) == 0 {
		return 0, nil
	}

	// Build: add rule inet kwor prerouting meta l4proto (...) th dport { ... } redirect to :PORT comment "..."
	args := []string{
		"add", "rule",
		nftFamily, nftTable, nftChainPrerouting,
	}
	if includeTCP {
		args = append(args, "meta", "l4proto", "{", "tcp", ",", "udp", "}")
	} else {
		args = append(args, "meta", "l4proto", "udp")
	}
	args = appendNftTransportPortMatch(args, "dport")
	args = append(args, portSetArgs...)
	args = append(args, "redirect", "to", fmt.Sprintf(":%d", listenPort))
	args = append(args, "comment", comment)

	_, err := runNft(args...)
	if err != nil {
		return 0, err
	}

	// Find handle by listing the chain
	listArgs := []string{
		"--handle", "--numeric", "list", "chain",
		nftFamily, nftTable, nftChainPrerouting,
	}
	out, err := runNft(listArgs...)
	if err != nil {
		return 0, nil // rule created but can't find handle
	}

	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		if ruleLineHasExactComment(line, comment) && strings.Contains(line, "handle") {
			m := nftHandleRe.FindStringSubmatch(line)
			if len(m) == 2 {
				handle := 0
				fmt.Sscanf(m[1], "%d", &handle)
				if handle > 0 {
					return handle, nil
				}
			}
		}
	}

	logger.Warning("nftables REDIRECT rule created but handle not found for comment: ", comment)
	return 0, nil
}

func addCompatibilityRedirectRule(hopPortsNft string, listenPort int, comment string, includeTCP bool) (int, error) {
	portSetArgs := buildNftPortSetArgs(hopPortsNft)
	if len(portSetArgs) == 0 {
		return 0, nil
	}

	addedFamilies := make([]string, 0, len(nftCompatibilityNatFamilies()))
	for _, tableFamily := range nftCompatibilityNatFamilies() {
		args := []string{"add", "rule", tableFamily, nftTable, nftChainPrerouting}
		if includeTCP {
			args = append(args, "meta", "l4proto", "{", "tcp", ",", "udp", "}")
		} else {
			args = append(args, "meta", "l4proto", "udp")
		}
		args = appendNftTransportPortMatch(args, "dport")
		args = append(args, portSetArgs...)
		args = append(args, "redirect", "to", fmt.Sprintf(":%d", listenPort), "comment", comment)
		if _, err := runNft(args...); err != nil {
			var rollbackErr error
			for _, addedFamily := range addedFamilies {
				if cleanupErr := deleteNftRulesByExactComment(addedFamily, nftTable, nftChainPrerouting, comment); cleanupErr != nil && rollbackErr == nil {
					rollbackErr = cleanupErr
				}
			}
			if rollbackErr != nil {
				return 0, fmt.Errorf("add compatibility REDIRECT for %s failed: %w; rollback failed: %v", tableFamily, err, rollbackErr)
			}
			return 0, err
		}
		addedFamilies = append(addedFamilies, tableFamily)
	}

	// A single database handle cannot represent the two managed NAT rules.
	// Compatibility callers use the stable comment for lifecycle operations.
	return 0, nil
}

// buildNftPortSetArgs converts a comma-separated port spec (e.g. "899-999, 5000-6000")
// into nft command arguments for a port set: { 899-999 , 5000-6000 }
// For a single element, returns it without braces (e.g. ["899-999"]).
func buildNftPortSetArgs(portsNft string) []string {
	parts := strings.Split(portsNft, ",")
	var cleaned []string
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			cleaned = append(cleaned, part)
		}
	}
	if len(cleaned) == 0 {
		return nil
	}
	if len(cleaned) == 1 {
		return []string{cleaned[0]}
	}
	var args []string
	args = append(args, "{")
	for i, part := range cleaned {
		if i > 0 {
			args = append(args, ",")
		}
		args = append(args, part)
	}
	args = append(args, "}")
	return args
}

// portHopRangeToNftLegacy is the previous string-only normalizer (kept for reference).
// Converts ":" to "-" (nftables range separator), normalizes commas and whitespace.
func portHopRangeToNftLegacy(input string) string {
	if input == "" {
		return ""
	}
	input = strings.ReplaceAll(input, "\uFF0C", ",")
	input = strings.ReplaceAll(input, ":", "-")
	parts := strings.Split(input, ",")
	var cleaned []string
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			cleaned = append(cleaned, part)
		}
	}
	return strings.Join(cleaned, ", ")
}

// portHopRangeToNft converts a user-input port_hop_range to nftables format.
func portHopRangeToNft(input string) string {
	ranges := parsePortRangeInput(input)
	return portRangesToNft(ranges)
}

// portHopRangeToNftWithExclusions converts a user-input port_hop_range to nftables format.
// It intentionally does NOT exclude ports occupied by other processes, so REDIRECT remains
// forceful for the configured range. Only listenPort itself is excluded to avoid self-redirect.
// Returns (nftPorts, skippedCount, skippedSample).
func portHopRangeToNftWithExclusions(input string, listenPort int) (string, int, []int) {
	ranges := parsePortRangeInput(input)
	if len(ranges) == 0 {
		return "", 0, nil
	}

	excluded := map[int]struct{}{}
	if listenPort > 0 && listenPort <= 65535 {
		excluded[listenPort] = struct{}{}
	}

	allowed, skippedCount, skippedSample := excludePortsFromRanges(ranges, excluded)
	if len(allowed) == 0 {
		return "", skippedCount, skippedSample
	}
	return portRangesToNft(allowed), skippedCount, skippedSample
}

type portRange struct {
	start int
	end   int
}

func parsePortRangeInput(input string) []portRange {
	if input == "" {
		return nil
	}

	// Normalize full-width comma and whitespace.
	input = strings.ReplaceAll(input, "\uFF0C", ",")
	parts := strings.Split(input, ",")

	ranges := make([]portRange, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		part = strings.ReplaceAll(part, "-", ":")
		if strings.Contains(part, ":") {
			seg := strings.SplitN(part, ":", 2)
			if len(seg) != 2 {
				continue
			}
			start, err1 := strconv.Atoi(seg[0])
			end, err2 := strconv.Atoi(seg[1])
			if err1 != nil || err2 != nil {
				continue
			}
			if start > end {
				start, end = end, start
			}
			if end < 1 || start > 65535 {
				continue
			}
			if start < 1 {
				start = 1
			}
			if end > 65535 {
				end = 65535
			}
			ranges = append(ranges, portRange{start: start, end: end})
			continue
		}

		port, err := strconv.Atoi(part)
		if err != nil || port < 1 || port > 65535 {
			continue
		}
		ranges = append(ranges, portRange{start: port, end: port})
	}

	return mergePortRanges(ranges)
}

func mergePortRanges(ranges []portRange) []portRange {
	if len(ranges) == 0 {
		return nil
	}
	sort.Slice(ranges, func(i, j int) bool {
		if ranges[i].start == ranges[j].start {
			return ranges[i].end < ranges[j].end
		}
		return ranges[i].start < ranges[j].start
	})

	merged := []portRange{ranges[0]}
	for i := 1; i < len(ranges); i++ {
		last := &merged[len(merged)-1]
		cur := ranges[i]
		if cur.start <= last.end+1 {
			if cur.end > last.end {
				last.end = cur.end
			}
			continue
		}
		merged = append(merged, cur)
	}
	return merged
}

func portRangesToNft(ranges []portRange) string {
	if len(ranges) == 0 {
		return ""
	}
	parts := make([]string, 0, len(ranges))
	for _, r := range ranges {
		if r.start == r.end {
			parts = append(parts, fmt.Sprintf("%d", r.start))
		} else {
			parts = append(parts, fmt.Sprintf("%d-%d", r.start, r.end))
		}
	}
	return strings.Join(parts, ", ")
}

func excludePortsFromRanges(ranges []portRange, excluded map[int]struct{}) ([]portRange, int, []int) {
	if len(ranges) == 0 {
		return nil, 0, nil
	}
	if len(excluded) == 0 {
		return ranges, 0, nil
	}

	var allowed []portRange
	skipped := 0
	sample := make([]int, 0, 5)

	for _, r := range ranges {
		curStart := -1
		for p := r.start; p <= r.end; p++ {
			if _, blocked := excluded[p]; blocked {
				if len(sample) < 5 {
					sample = append(sample, p)
				}
				skipped++
				if curStart != -1 {
					allowed = append(allowed, portRange{start: curStart, end: p - 1})
					curStart = -1
				}
				continue
			}
			if curStart == -1 {
				curStart = p
			}
		}
		if curStart != -1 {
			allowed = append(allowed, portRange{start: curStart, end: r.end})
		}
	}

	return mergePortRanges(allowed), skipped, sample
}

func getOccupiedUDPPorts() map[int]struct{} {
	if !nftSupported() {
		return nil
	}
	ports := make(map[int]struct{})
	readProcUDPPorts("/proc/net/udp", ports)
	readProcUDPPorts("/proc/net/udp6", ports)
	return ports
}

func readProcUDPPorts(path string, ports map[int]struct{}) {
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	lines := strings.Split(string(data), "\n")
	for i := 1; i < len(lines); i++ { // skip header
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		local := fields[1]
		parts := strings.Split(local, ":")
		if len(parts) != 2 {
			continue
		}
		portHex := parts[1]
		port, err := strconv.ParseInt(portHex, 16, 32)
		if err != nil {
			continue
		}
		if port > 0 && port <= 65535 {
			ports[int(port)] = struct{}{}
		}
	}
}

func deleteRuleByHandle(chain string, handle int) error {
	return deleteRuleByHandleFn(chain, handle)
}

var deleteRuleByHandleFn = func(chain string, handle int) error {
	if !nftSupported() {
		return nil
	}
	if handle <= 0 {
		return nil
	}
	_, err := runNft("delete", "rule", nftFamily, nftTable, chain, "handle", fmt.Sprint(handle))
	return err
}

func findNftRuleHandlesByExactComment(tableFamily string, table string, chain string, comment string) ([]int, error) {
	if !nftSupported() || comment == "" {
		return nil, nil
	}
	out, err := runNft("--handle", "--numeric", "list", "chain", tableFamily, table, chain)
	if err != nil {
		if nftObjectMissing(err) {
			return nil, nil
		}
		return nil, err
	}

	handles := make([]int, 0)
	for _, line := range strings.Split(string(out), "\n") {
		if !ruleLineHasExactComment(line, comment) {
			continue
		}
		match := nftHandleRe.FindStringSubmatch(line)
		if len(match) != 2 {
			continue
		}
		handle, convErr := strconv.Atoi(match[1])
		if convErr == nil && handle > 0 {
			handles = append(handles, handle)
		}
	}
	return handles, nil
}

func deleteNftRulesByExactComment(tableFamily string, table string, chain string, comment string) error {
	handles, err := findNftRuleHandlesByExactComment(tableFamily, table, chain, comment)
	if err != nil {
		return err
	}
	var firstErr error
	for _, handle := range handles {
		if _, deleteErr := runNft("delete", "rule", tableFamily, table, chain, "handle", strconv.Itoa(handle)); deleteErr != nil && firstErr == nil {
			firstErr = deleteErr
		}
	}
	return firstErr
}

type nftRuleLocation struct {
	tableFamily string
	table       string
	chain       string
}

func nftRedirectRuleLocations() []nftRuleLocation {
	locations := make([]nftRuleLocation, 0, 1+len(nftCompatibilityNatFamilies()))
	locations = append(locations, nftRuleLocation{tableFamily: nftFamily, table: nftTable, chain: nftChainPrerouting})
	for _, tableFamily := range nftCompatibilityNatFamilies() {
		locations = append(locations, nftRuleLocation{tableFamily: tableFamily, table: nftTable, chain: nftChainPrerouting})
	}
	return locations
}

func nftRedirectLocationIsCurrent(location nftRuleLocation) bool {
	if nftUsesCompatibilityLayout() {
		return location.tableFamily == nftNatFamilyIPv4 || location.tableFamily == nftNatFamilyIPv6
	}
	return location.tableFamily == nftFamily
}

// nftRedirectRuleExistsByComment is an integrity check, not merely a lookup.
// A managed REDIRECT is valid only when every current-layout table has exactly
// one matching rule and every other layout has none. This makes duplicate and
// stale cross-layout rules self-heal through the existing caller lifecycle.
func nftRedirectRuleExistsByComment(comment string) bool {
	if !nftSupported() || comment == "" {
		return false
	}
	for _, location := range nftRedirectRuleLocations() {
		handles, err := findNftRuleHandlesByExactComment(location.tableFamily, location.table, location.chain, comment)
		if err != nil {
			return false
		}
		if nftRedirectLocationIsCurrent(location) {
			if len(handles) != 1 {
				return false
			}
			continue
		}
		if len(handles) != 0 {
			return false
		}
	}
	return true
}

// nftRedirectRuleExistsInAnyLayout is used by cleanup paths. Unlike the
// strict integrity predicate above, it deliberately reports stale and
// duplicate rules so a disabled port-hop configuration can remove them.
func nftRedirectRuleExistsInAnyLayout(comment string) bool {
	if !nftSupported() || comment == "" {
		return false
	}
	for _, location := range nftRedirectRuleLocations() {
		handles, err := findNftRuleHandlesByExactComment(location.tableFamily, location.table, location.chain, comment)
		if err == nil && len(handles) > 0 {
			return true
		}
	}
	return false
}

func findNftRedirectHandleByComment(comment string) int {
	if nftUsesCompatibilityLayout() {
		return 0
	}
	return findHandleByComment(nftChainPrerouting, comment)
}

// deleteNftRedirectRulesByComment removes both current and previous managed
// layouts. This is required when a host capability profile changes at runtime.
func deleteNftRedirectRulesByComment(comment string) error {
	if !nftSupported() || comment == "" {
		return nil
	}
	var firstErr error
	for _, location := range nftRedirectRuleLocations() {
		if err := deleteNftRulesByExactComment(location.tableFamily, location.table, location.chain, comment); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func listNftRedirectCommentsByPrefix(prefix string) ([]string, error) {
	if !nftSupported() || prefix == "" {
		return nil, nil
	}
	seen := make(map[string]struct{})
	comments := make([]string, 0)
	var firstErr error
	for _, tableFamily := range append([]string{nftFamily}, nftCompatibilityNatFamilies()...) {
		out, err := runNft("--handle", "--numeric", "list", "chain", tableFamily, nftTable, nftChainPrerouting)
		if err != nil {
			if !nftObjectMissing(err) && firstErr == nil {
				firstErr = err
			}
			continue
		}
		for _, line := range strings.Split(string(out), "\n") {
			comment, ok := extractRuleComment(line)
			if !ok || !strings.HasPrefix(comment, prefix) {
				continue
			}
			if _, exists := seen[comment]; exists {
				continue
			}
			seen[comment] = struct{}{}
			comments = append(comments, comment)
		}
	}
	return comments, firstErr
}

// deleteRuleByComment deletes all rules in the given chain that contain the specified comment.
// Used as fallback when handle is unknown (0).
func deleteRuleByComment(chain string, comment string) error {
	return deleteRuleByCommentFn(chain, comment)
}

var deleteRuleByCommentFn = func(chain string, comment string) error {
	if !nftSupported() || comment == "" {
		return nil
	}

	// List chain with handles
	out, err := runNft("--handle", "--numeric", "list", "chain", nftFamily, nftTable, chain)
	if err != nil {
		return err
	}

	lines := strings.Split(string(out), "\n")
	var firstErr error
	for _, line := range lines {
		if ruleLineHasExactComment(line, comment) && strings.Contains(line, "handle") {
			m := nftHandleRe.FindStringSubmatch(line)
			if len(m) == 2 {
				handle := 0
				fmt.Sscanf(m[1], "%d", &handle)
				if handle > 0 {
					if _, delErr := runNft("delete", "rule", nftFamily, nftTable, chain, "handle", fmt.Sprint(handle)); delErr != nil && firstErr == nil {
						firstErr = delErr
					}
				}
			}
		}
	}
	return firstErr
}

// findHandleByComment searches for a rule handle in the given chain by comment string.
// Returns 0 if not found.
func findHandleByComment(chain string, comment string) int {
	if !nftSupported() || comment == "" {
		return 0
	}

	out, err := runNft("--handle", "--numeric", "list", "chain", nftFamily, nftTable, chain)
	if err != nil {
		return 0
	}

	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		if ruleLineHasExactComment(line, comment) && strings.Contains(line, "handle") {
			m := nftHandleRe.FindStringSubmatch(line)
			if len(m) == 2 {
				handle := 0
				fmt.Sscanf(m[1], "%d", &handle)
				if handle > 0 {
					return handle
				}
			}
		}
	}
	return 0
}

// cleanupNftTable deletes every managed default-core table, including the
// compatibility ip/ip6 NAT tables used on kernels without inet NAT support.
// Called on program shutdown to avoid leaving stale rules.
func cleanupNftTable() {
	if !nftSupported() {
		return
	}

	for _, tableFamily := range append([]string{nftFamily}, nftCompatibilityNatFamilies()...) {
		if err := deleteOwnedNftTableForRuntime(tableFamily, nftTable); err != nil {
			if nftObjectMissing(err) {
				continue
			}
			logger.Warning("failed to delete nftables table ", tableFamily, " ", nftTable, ": ", err)
		} else {
			logger.Info("nftables table ", tableFamily, " ", nftTable, " cleaned up")
		}
	}
}

// CleanupAllNftRulesForCommand removes the whole managed nft table.
// Intended for command paths like `kwor stop` / `kwor uninstall` as a final cleanup fallback.
func CleanupAllNftRulesForCommand() {
	cleanupNftTable()
	if err := cleanupManagedFirewallTable(); err != nil && !firewallNftObjectMissing(err) {
		logger.Warning("failed to cleanup managed firewall nft table: ", err)
	}
	if portForwardSupported() {
		if err := cleanupManagedPortForwardTable(); err != nil && !portForwardNftObjectMissing(err) {
			logger.Warning("failed to cleanup managed port-forward nft table: ", err)
		} else if err := restorePortForwardKernelForwarding(); err != nil {
			// Restore only after every owned table was removed successfully. The
			// stored baseline remains for the next safe cleanup attempt on error.
			logger.Warning("failed to restore managed forwarding sysctl state: ", err)
		}
	}
}

func nftTableExists() bool {
	return nftTableExistsFn()
}

var nftTableExistsFn = func() bool {
	if !nftSupported() {
		return false
	}
	exists, err := inspectOwnedNftTableForMutation(nftFamily, nftTable)
	return err == nil && exists
}

func nftObjectMissing(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(message, "no such file or directory") ||
		strings.Contains(message, "no such file") ||
		strings.Contains(message, "not found")
}

type nftList struct {
	Nftables []map[string]json.RawMessage `json:"nftables"`
}

type nftRule struct {
	Handle int                          `json:"handle"`
	Expr   []map[string]json.RawMessage `json:"expr"`
}

type nftCounter struct {
	Bytes int64 `json:"bytes"`
}

func getChainRuleBytesByHandle(chain string, handle int) (int64, error) {
	if !nftSupported() {
		return 0, nil
	}
	values, err := getChainRuleBytesByHandles(chain, []int{handle})
	if err != nil {
		return 0, err
	}
	bytes, ok := values[handle]
	if !ok {
		return 0, fmt.Errorf("nft rule handle %d not found in %s", handle, chain)
	}
	return bytes, nil
}

// getChainRuleBytesByHandles reads every requested counter from a chain with a
// single nft invocation. Traffic collection calls it once per direction so the
// cost does not grow with the number of configured inbounds.
func getChainRuleBytesByHandles(chain string, handles []int) (map[int]int64, error) {
	if !nftSupported() {
		return map[int]int64{}, nil
	}
	wanted := make(map[int]struct{}, len(handles))
	for _, handle := range handles {
		if handle > 0 {
			wanted[handle] = struct{}{}
		}
	}
	if len(wanted) == 0 {
		return map[int]int64{}, nil
	}
	if GetNftablesCapabilities().SupportsJSON {
		out, err := runNft("-j", "list", "chain", nftFamily, nftTable, chain)
		if err == nil {
			values, parseErr := getChainRuleBytesByHandlesFromJSON(out, chain, wanted)
			if parseErr == nil {
				return values, nil
			}
			err = parseErr
		}

		// A runtime can report a newer version while a restricted nft binary
		// rejects JSON. Fall back to the text parser before treating the
		// counter as unavailable.
		textBytes, textErr := getChainRuleBytesByHandlesFromTextNft(chain, wanted)
		if textErr == nil {
			return textBytes, nil
		}
		if err != nil {
			return nil, fmt.Errorf("read nft counters by json failed: %w; text fallback failed: %v", err, textErr)
		}
		return nil, textErr
	}
	return getChainRuleBytesByHandlesFromTextNft(chain, wanted)
}

func getChainRuleBytesByHandleFromJSON(out []byte, chain string, handle int) (int64, error) {
	if handle <= 0 {
		return 0, fmt.Errorf("invalid handle")
	}
	values, err := getChainRuleBytesByHandlesFromJSON(out, chain, map[int]struct{}{handle: {}})
	if err != nil {
		return 0, err
	}
	return values[handle], nil
}

func getChainRuleBytesByHandlesFromJSON(out []byte, chain string, wanted map[int]struct{}) (map[int]int64, error) {
	var parsed nftList
	if err := json.Unmarshal(out, &parsed); err != nil {
		return nil, fmt.Errorf("parse nft json failed: %w", err)
	}

	values := make(map[int]int64, len(wanted))
	for _, item := range parsed.Nftables {
		raw, ok := item["rule"]
		if !ok {
			continue
		}
		var r nftRule
		if err := json.Unmarshal(raw, &r); err != nil {
			continue
		}
		if _, ok := wanted[r.Handle]; !ok {
			continue
		}
		for _, expr := range r.Expr {
			if cRaw, ok := expr["counter"]; ok {
				var c nftCounter
				if err := json.Unmarshal(cRaw, &c); err == nil {
					values[r.Handle] = c.Bytes
					break
				}
			}
		}
		// A rule without a counter still represents zero bytes.
		if _, found := values[r.Handle]; !found {
			values[r.Handle] = 0
		}
	}

	if len(values) == len(wanted) {
		return values, nil
	}
	missing := missingNftHandles(wanted, values)
	return nil, fmt.Errorf("nft rule handles %s not found in %s", missing, chain)
}

func getChainRuleBytesByHandleFromTextNft(chain string, handle int) (int64, error) {
	if handle <= 0 {
		return 0, fmt.Errorf("invalid handle")
	}
	values, err := getChainRuleBytesByHandlesFromTextNft(chain, map[int]struct{}{handle: {}})
	if err != nil {
		return 0, err
	}
	return values[handle], nil
}

func getChainRuleBytesByHandleFromText(out []byte, chain string, handle int) (int64, error) {
	if handle <= 0 {
		return 0, fmt.Errorf("invalid handle")
	}
	values, err := getChainRuleBytesByHandlesFromText(out, chain, map[int]struct{}{handle: {}})
	if err != nil {
		return 0, err
	}
	return values[handle], nil
}

func getChainRuleBytesByHandlesFromTextNft(chain string, wanted map[int]struct{}) (map[int]int64, error) {
	out, err := runNft("--handle", "--numeric", "list", "chain", nftFamily, nftTable, chain)
	if err != nil {
		return nil, err
	}
	return getChainRuleBytesByHandlesFromText(out, chain, wanted)
}

func getChainRuleBytesByHandlesFromText(out []byte, chain string, wanted map[int]struct{}) (map[int]int64, error) {
	values := make(map[int]int64, len(wanted))
	for _, line := range strings.Split(string(out), "\n") {
		match := nftHandleRe.FindStringSubmatch(line)
		if len(match) != 2 {
			continue
		}
		currentHandle, convErr := strconv.Atoi(match[1])
		if convErr != nil {
			continue
		}
		if _, ok := wanted[currentHandle]; !ok {
			continue
		}
		counterMatch := nftCounterBytesRe.FindStringSubmatch(line)
		if len(counterMatch) != 2 {
			values[currentHandle] = 0
			continue
		}
		bytes, bytesErr := strconv.ParseInt(counterMatch[1], 10, 64)
		if bytesErr != nil {
			return nil, fmt.Errorf("parse nft counter bytes for handle %d: %w", currentHandle, bytesErr)
		}
		values[currentHandle] = bytes
	}
	if len(values) == len(wanted) {
		return values, nil
	}
	missing := missingNftHandles(wanted, values)
	return nil, fmt.Errorf("nft rule handles %s not found in %s", missing, chain)
}

func missingNftHandles(wanted map[int]struct{}, values map[int]int64) string {
	missing := make([]int, 0, len(wanted)-len(values))
	for handle := range wanted {
		if _, ok := values[handle]; !ok {
			missing = append(missing, handle)
		}
	}
	sort.Ints(missing)
	parts := make([]string, 0, len(missing))
	for _, handle := range missing {
		parts = append(parts, strconv.Itoa(handle))
	}
	return strings.Join(parts, ",")
}
