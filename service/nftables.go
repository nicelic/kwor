package service

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
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
	nftFamily          = "inet"
	nftChainIn         = "input"
	nftChainOut        = "output"
	nftChainForward    = "forward"
	nftChainPrerouting = "prerouting"
)

var (
	nftTable      = loadNftTableName()
	nftHandleRe   = regexp.MustCompile(`handle\s+(\d+)`)
	nftCommentRe  = regexp.MustCompile(`comment\s+"((?:[^"\\]|\\.)*)"`)
	nftCandidates = []string{
		"/usr/sbin/nft",
		"/sbin/nft",
		"/usr/bin/nft",
		"/bin/nft",
	}
)

var nftRuntimeGOOS = runtime.GOOS
var nftLookPathFn = exec.LookPath
var nftStatFn = os.Stat

func resolveNftBinaryPath() (string, error) {
	if nftRuntimeGOOS != "linux" {
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
	return nftSupportedFn()
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
	binaryPath, err := resolveNftBinaryPath()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binaryPath, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("nft %s failed: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

func runNftScript(script string) ([]byte, error) {
	binaryPath, err := resolveNftBinaryPath()
	if err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binaryPath, "-f", "-")
	cmd.Stdin = strings.NewReader(script)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err = cmd.Run()
	if err != nil {
		return nil, fmt.Errorf("nft script failed: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

func ensureNftBase() error {
	if !nftSupported() {
		return nil
	}

	// Ensure table
	_, err := runNft("list", "table", nftFamily, nftTable)
	if err != nil {
		if _, addErr := runNft("add", "table", nftFamily, nftTable); addErr != nil {
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

// ensureNftNatChain ensures the nat/prerouting chain exists for port hopping REDIRECT rules.
func ensureNftNatChain() error {
	if !nftSupported() {
		return nil
	}
	if err := ensureNftBase(); err != nil {
		return err
	}

	_, err := runNft("list", "chain", nftFamily, nftTable, nftChainPrerouting)
	if err != nil {
		_, addErr := runNft(
			"add", "chain", nftFamily, nftTable, nftChainPrerouting,
			"{", "type", "nat", "hook", "prerouting", "priority", "dstnat", ";", "policy", "accept", ";", "}",
		)
		if addErr != nil {
			return addErr
		}
	}
	return nil
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
		"th", direction, fmt.Sprint(port),
		"counter",
		"comment", comment,
	}
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
		"th", direction, fmt.Sprint(port),
		"limit", "rate", "over", fmt.Sprint(bytesPerSecond), "bytes/second",
		"counter",
		"drop",
		"comment", comment,
	}
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
		"th", direction, fmt.Sprint(port),
		"counter",
		"drop",
		"comment", comment,
	}
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
		"th", direction,
	}
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
		"th", direction, "!=",
	}
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
	args = append(args, "th", "dport")
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

// cleanupNftTable deletes the entire kwor nftables table and all its rules.
// Called on program shutdown to avoid leaving stale rules.
func cleanupNftTable() {
	if !nftSupported() {
		return
	}

	// Check if table exists first
	_, err := runNft("list", "table", nftFamily, nftTable)
	if err != nil {
		return // table doesn't exist, nothing to clean
	}

	// Delete the entire table (this removes all chains and rules inside it)
	_, err = runNft("delete", "table", nftFamily, nftTable)
	if err != nil {
		logger.Warning("failed to delete nftables table ", nftTable, ": ", err)
	} else {
		logger.Info("nftables table ", nftFamily, " ", nftTable, " cleaned up")
	}
}

// CleanupAllNftRulesForCommand removes the whole managed nft table.
// Intended for command paths like `kwor stop` / `kwor uninstall` as a final cleanup fallback.
func CleanupAllNftRulesForCommand() {
	cleanupNftTable()
	if err := cleanupManagedFirewallTable(); err != nil && !firewallNftObjectMissing(err) {
		logger.Warning("failed to cleanup managed firewall nft table: ", err)
	}
	if err := cleanupManagedPortForwardTable(); err != nil && !portForwardNftObjectMissing(err) {
		logger.Warning("failed to cleanup managed port-forward nft table: ", err)
	}
}

func nftTableExists() bool {
	return nftTableExistsFn()
}

var nftTableExistsFn = func() bool {
	if !nftSupported() {
		return false
	}
	_, err := runNft("list", "table", nftFamily, nftTable)
	return err == nil
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
	if handle <= 0 {
		return 0, fmt.Errorf("invalid handle")
	}
	if err := ensureNftBase(); err != nil {
		return 0, err
	}

	out, err := runNft("-j", "list", "chain", nftFamily, nftTable, chain)
	if err != nil {
		return 0, err
	}

	var parsed nftList
	if err := json.Unmarshal(out, &parsed); err != nil {
		return 0, fmt.Errorf("parse nft json failed: %w", err)
	}

	for _, item := range parsed.Nftables {
		raw, ok := item["rule"]
		if !ok {
			continue
		}
		var r nftRule
		if err := json.Unmarshal(raw, &r); err != nil {
			continue
		}
		if r.Handle != handle {
			continue
		}
		for _, expr := range r.Expr {
			if cRaw, ok := expr["counter"]; ok {
				var c nftCounter
				if err := json.Unmarshal(cRaw, &c); err == nil {
					return c.Bytes, nil
				}
			}
		}
		// rule found but no counter expr
		return 0, nil
	}

	return 0, fmt.Errorf("nft rule handle %d not found in %s", handle, chain)
}
