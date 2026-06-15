package service

import (
	"bufio"
	"bytes"
	"compress/zlib"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/netip"
	"net/url"
	"path"
	"strings"

	"github.com/klauspost/compress/zstd"
	"go4.org/netipx"
)

const (
	firewallGeoFormatSRS  = "srs"
	firewallGeoFormatMRS  = "mrs"
	firewallGeoFormatJSON = "json"
	firewallGeoFormatTXT  = "txt"
)

const (
	firewallGeoRuleActionAllow = "allow"
	firewallGeoRuleActionBlock = "block"
)

type firewallGeoResolvedPrefixes struct {
	All         []string
	IPv4        []string
	IPv6        []string
	PrefixCount int
	ContentHash string
}

// The SRS parser below is adapted from the user's provided sing-box
// 1.14.0-alpha.20 source. We only support the IP-only rule-set subset needed
// by the firewall GeoIP feature and intentionally reject non-IP rule items.
const (
	firewallGeoSRSMagic = "SRS"

	firewallGeoSRSRuleItemQueryType uint8 = iota
	firewallGeoSRSRuleItemNetwork
	firewallGeoSRSRuleItemDomain
	firewallGeoSRSRuleItemDomainKeyword
	firewallGeoSRSRuleItemDomainRegex
	firewallGeoSRSRuleItemSourceIPCIDR
	firewallGeoSRSRuleItemIPCIDR
	firewallGeoSRSRuleItemSourcePort
	firewallGeoSRSRuleItemSourcePortRange
	firewallGeoSRSRuleItemPort
	firewallGeoSRSRuleItemPortRange
	firewallGeoSRSRuleItemProcessName
	firewallGeoSRSRuleItemProcessPath
	firewallGeoSRSRuleItemPackageName
	firewallGeoSRSRuleItemWIFISSID
	firewallGeoSRSRuleItemWIFIBSSID
	firewallGeoSRSRuleItemAdGuardDomain
	firewallGeoSRSRuleItemProcessPathRegex
	firewallGeoSRSRuleItemNetworkType
	firewallGeoSRSRuleItemNetworkIsExpensive
	firewallGeoSRSRuleItemNetworkIsConstrained
	firewallGeoSRSRuleItemNetworkInterfaceAddress
	firewallGeoSRSRuleItemDefaultInterfaceAddress
	firewallGeoSRSRuleItemPackageNameRegex
	firewallGeoSRSRuleItemFinal uint8 = 0xFF
)

// The MRS parser below is adapted from the user's provided mihomo
// 1.19.24 source. Only IPCIDR behavior is supported because the firewall
// feature only consumes IP country/region rule-sets.
var firewallGeoMRSMagic = [4]byte{'M', 'R', 'S', 1}

func parseFirewallGeoRuleBytes(sourceName string, content []byte) (firewallGeoResolvedPrefixes, error) {
	formatOrder := firewallGeoCandidateFormats(sourceName)
	var errs []string
	for _, format := range formatOrder {
		var (
			parsed firewallGeoResolvedPrefixes
			err    error
		)
		switch format {
		case firewallGeoFormatSRS:
			parsed, err = parseFirewallGeoSRS(content)
		case firewallGeoFormatMRS:
			parsed, err = parseFirewallGeoMRS(content)
		case firewallGeoFormatJSON:
			parsed, err = parseFirewallGeoJSON(content)
		case firewallGeoFormatTXT:
			parsed, err = parseFirewallGeoTXT(content)
		default:
			err = fmt.Errorf("unsupported ruleset format: %s", format)
		}
		if err == nil {
			return parsed, nil
		}
		errs = append(errs, fmt.Sprintf("%s: %v", format, err))
	}
	return firewallGeoResolvedPrefixes{}, fmt.Errorf("unable to parse ruleset %s (%s)", sourceName, strings.Join(errs, "; "))
}

func firewallGeoCandidateFormats(sourceName string) []string {
	format := detectFirewallGeoFormat(sourceName)
	switch format {
	case firewallGeoFormatSRS:
		return []string{firewallGeoFormatSRS, firewallGeoFormatJSON, firewallGeoFormatTXT}
	case firewallGeoFormatMRS:
		return []string{firewallGeoFormatMRS, firewallGeoFormatTXT}
	case firewallGeoFormatJSON:
		return []string{firewallGeoFormatJSON, firewallGeoFormatTXT}
	case firewallGeoFormatTXT:
		return []string{firewallGeoFormatTXT, firewallGeoFormatJSON}
	default:
		return []string{firewallGeoFormatSRS, firewallGeoFormatMRS, firewallGeoFormatJSON, firewallGeoFormatTXT}
	}
}

func detectFirewallGeoFormat(sourceName string) string {
	sourceName = strings.TrimSpace(sourceName)
	if sourceName == "" {
		return ""
	}
	if parsedURL, err := url.Parse(sourceName); err == nil && parsedURL.Path != "" {
		sourceName = parsedURL.Path
	}
	switch strings.ToLower(path.Ext(sourceName)) {
	case ".srs":
		return firewallGeoFormatSRS
	case ".mrs":
		return firewallGeoFormatMRS
	case ".json":
		return firewallGeoFormatJSON
	case ".txt", ".yaml", ".yml":
		return firewallGeoFormatTXT
	default:
		return ""
	}
}

func parseFirewallGeoSRS(content []byte) (firewallGeoResolvedPrefixes, error) {
	reader := bytes.NewReader(content)
	magic := make([]byte, 3)
	if _, err := io.ReadFull(reader, magic); err != nil {
		return firewallGeoResolvedPrefixes{}, err
	}
	if string(magic) != firewallGeoSRSMagic {
		return firewallGeoResolvedPrefixes{}, fmt.Errorf("invalid srs magic")
	}

	var version uint8
	if err := binary.Read(reader, binary.BigEndian, &version); err != nil {
		return firewallGeoResolvedPrefixes{}, err
	}
	if version < 1 || version > 5 {
		return firewallGeoResolvedPrefixes{}, fmt.Errorf("unsupported srs version: %d", version)
	}

	zr, err := zlib.NewReader(reader)
	if err != nil {
		return firewallGeoResolvedPrefixes{}, err
	}
	defer zr.Close()

	br := bufio.NewReader(zr)
	ruleCount, err := binary.ReadUvarint(br)
	if err != nil {
		return firewallGeoResolvedPrefixes{}, err
	}

	var builder netipx.IPSetBuilder
	for index := uint64(0); index < ruleCount; index++ {
		if err := parseFirewallGeoSRSRule(br, &builder); err != nil {
			return firewallGeoResolvedPrefixes{}, fmt.Errorf("read srs rule[%d]: %w", index, err)
		}
	}
	return buildFirewallGeoResolvedPrefixes(&builder)
}

func parseFirewallGeoSRSRule(reader *bufio.Reader, builder *netipx.IPSetBuilder) error {
	ruleType, err := reader.ReadByte()
	if err != nil {
		return err
	}
	switch ruleType {
	case 0:
		return parseFirewallGeoSRSDefaultRule(reader, builder)
	case 1:
		return parseFirewallGeoSRSLogicalRule(reader, builder)
	default:
		return fmt.Errorf("unsupported srs rule type: %d", ruleType)
	}
}

func parseFirewallGeoSRSDefaultRule(reader *bufio.Reader, builder *netipx.IPSetBuilder) error {
	for {
		itemType, err := reader.ReadByte()
		if err != nil {
			return err
		}
		switch itemType {
		case firewallGeoSRSRuleItemSourceIPCIDR, firewallGeoSRSRuleItemIPCIDR:
			if err := readFirewallGeoSRSIPSet(reader, builder); err != nil {
				return err
			}
		case firewallGeoSRSRuleItemFinal:
			return readFirewallGeoSRSInvertFlag(reader)
		case firewallGeoSRSRuleItemNetworkIsExpensive, firewallGeoSRSRuleItemNetworkIsConstrained:
			// These items are not expected in GeoIP rule-sets. We reject them to
			// keep parser behavior explicit for firewall usage.
			return fmt.Errorf("unsupported non-ip srs rule item: %d", itemType)
		default:
			return fmt.Errorf("unsupported non-ip srs rule item: %d", itemType)
		}
	}
}

func parseFirewallGeoSRSLogicalRule(reader *bufio.Reader, builder *netipx.IPSetBuilder) error {
	mode, err := reader.ReadByte()
	if err != nil {
		return err
	}
	if mode != 0 && mode != 1 {
		return fmt.Errorf("unsupported srs logical mode: %d", mode)
	}
	length, err := binary.ReadUvarint(reader)
	if err != nil {
		return err
	}
	for index := uint64(0); index < length; index++ {
		if err := parseFirewallGeoSRSRule(reader, builder); err != nil {
			return fmt.Errorf("read logical rule[%d]: %w", index, err)
		}
	}
	return readFirewallGeoSRSInvertFlag(reader)
}

func readFirewallGeoSRSInvertFlag(reader *bufio.Reader) error {
	value, err := reader.ReadByte()
	if err != nil {
		return err
	}
	if value != 0 {
		return fmt.Errorf("inverted srs rules are not supported for firewall geoip")
	}
	return nil
}

func readFirewallGeoSRSIPSet(reader *bufio.Reader, builder *netipx.IPSetBuilder) error {
	version, err := reader.ReadByte()
	if err != nil {
		return err
	}
	if version != 1 {
		return fmt.Errorf("unsupported srs ipset version: %d", version)
	}

	var length uint64
	if err := binary.Read(reader, binary.BigEndian, &length); err != nil {
		return err
	}
	for index := uint64(0); index < length; index++ {
		from, err := readFirewallGeoSRSAddr(reader)
		if err != nil {
			return err
		}
		to, err := readFirewallGeoSRSAddr(reader)
		if err != nil {
			return err
		}
		builder.AddRange(netipx.IPRangeFrom(from, to))
	}
	return nil
}

func readFirewallGeoSRSAddr(reader *bufio.Reader) (netip.Addr, error) {
	length, err := binary.ReadUvarint(reader)
	if err != nil {
		return netip.Addr{}, err
	}
	raw := make([]byte, length)
	if _, err := io.ReadFull(reader, raw); err != nil {
		return netip.Addr{}, err
	}
	return firewallGeoAddrFromRawBytes(raw)
}

func parseFirewallGeoMRS(content []byte) (firewallGeoResolvedPrefixes, error) {
	zr, err := zstd.NewReader(bytes.NewReader(content))
	if err != nil {
		return firewallGeoResolvedPrefixes{}, err
	}
	defer zr.Close()

	var header [4]byte
	if _, err := io.ReadFull(zr, header[:]); err != nil {
		return firewallGeoResolvedPrefixes{}, err
	}
	if header != firewallGeoMRSMagic {
		return firewallGeoResolvedPrefixes{}, fmt.Errorf("invalid mrs magic")
	}

	behavior := make([]byte, 1)
	if _, err := io.ReadFull(zr, behavior); err != nil {
		return firewallGeoResolvedPrefixes{}, err
	}
	if behavior[0] != 1 {
		return firewallGeoResolvedPrefixes{}, fmt.Errorf("unsupported mrs behavior: %d", behavior[0])
	}

	var count int64
	if err := binary.Read(zr, binary.BigEndian, &count); err != nil {
		return firewallGeoResolvedPrefixes{}, err
	}
	if count < 0 {
		return firewallGeoResolvedPrefixes{}, fmt.Errorf("invalid mrs count")
	}

	var extraLength int64
	if err := binary.Read(zr, binary.BigEndian, &extraLength); err != nil {
		return firewallGeoResolvedPrefixes{}, err
	}
	if extraLength < 0 {
		return firewallGeoResolvedPrefixes{}, fmt.Errorf("invalid mrs extra length")
	}
	if extraLength > 0 {
		if _, err := io.CopyN(io.Discard, zr, extraLength); err != nil {
			return firewallGeoResolvedPrefixes{}, err
		}
	}

	version := make([]byte, 1)
	if _, err := io.ReadFull(zr, version); err != nil {
		return firewallGeoResolvedPrefixes{}, err
	}
	if version[0] != 1 {
		return firewallGeoResolvedPrefixes{}, fmt.Errorf("unsupported mrs ipcidr version: %d", version[0])
	}

	var length int64
	if err := binary.Read(zr, binary.BigEndian, &length); err != nil {
		return firewallGeoResolvedPrefixes{}, err
	}
	if length < 1 {
		return firewallGeoResolvedPrefixes{}, fmt.Errorf("invalid mrs ipcidr length")
	}

	var builder netipx.IPSetBuilder
	for index := int64(0); index < length; index++ {
		var from16 [16]byte
		if err := binary.Read(zr, binary.BigEndian, &from16); err != nil {
			return firewallGeoResolvedPrefixes{}, err
		}
		var to16 [16]byte
		if err := binary.Read(zr, binary.BigEndian, &to16); err != nil {
			return firewallGeoResolvedPrefixes{}, err
		}
		from := netip.AddrFrom16(from16).Unmap()
		to := netip.AddrFrom16(to16).Unmap()
		builder.AddRange(netipx.IPRangeFrom(from, to))
	}

	return buildFirewallGeoResolvedPrefixes(&builder)
}

func parseFirewallGeoJSON(content []byte) (firewallGeoResolvedPrefixes, error) {
	var root map[string]any
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.UseNumber()
	if err := decoder.Decode(&root); err != nil {
		return firewallGeoResolvedPrefixes{}, err
	}

	rawRules, exists := root["rules"]
	if !exists {
		return firewallGeoResolvedPrefixes{}, fmt.Errorf("missing rules array")
	}
	for key := range root {
		switch key {
		case "version", "rules":
		default:
			return firewallGeoResolvedPrefixes{}, fmt.Errorf("unsupported json ruleset field: %s", key)
		}
	}

	rules, ok := rawRules.([]any)
	if !ok {
		return firewallGeoResolvedPrefixes{}, fmt.Errorf("invalid json rules array")
	}

	var builder netipx.IPSetBuilder
	for index, rule := range rules {
		if err := parseFirewallGeoJSONRule(rule, &builder); err != nil {
			return firewallGeoResolvedPrefixes{}, fmt.Errorf("read json rule[%d]: %w", index, err)
		}
	}
	return buildFirewallGeoResolvedPrefixes(&builder)
}

func parseFirewallGeoJSONRule(raw any, builder *netipx.IPSetBuilder) error {
	rule, ok := raw.(map[string]any)
	if !ok {
		return fmt.Errorf("invalid json rule object")
	}

	if parseFirewallGeoJSONBool(rule["invert"]) {
		return fmt.Errorf("inverted json rules are not supported for firewall geoip")
	}

	if rawChildren, exists := rule["rules"]; exists {
		for key := range rule {
			switch key {
			case "type", "mode", "rules", "invert":
			default:
				return fmt.Errorf("unsupported json logical rule field: %s", key)
			}
		}
		children, ok := rawChildren.([]any)
		if !ok {
			return fmt.Errorf("invalid json logical children")
		}
		for index, child := range children {
			if err := parseFirewallGeoJSONRule(child, builder); err != nil {
				return fmt.Errorf("read logical child[%d]: %w", index, err)
			}
		}
		return nil
	}

	for key := range rule {
		switch key {
		case "type", "invert", "ip_cidr", "source_ip_cidr":
		default:
			return fmt.Errorf("unsupported json rule field: %s", key)
		}
	}

	collected := 0
	for _, key := range []string{"ip_cidr", "source_ip_cidr"} {
		values, err := parseFirewallGeoJSONStringList(rule[key])
		if err != nil {
			return fmt.Errorf("%s: %w", key, err)
		}
		for _, value := range values {
			if err := addFirewallGeoPrefixString(builder, value); err != nil {
				return fmt.Errorf("%s: %w", key, err)
			}
			collected++
		}
	}
	if collected == 0 {
		return fmt.Errorf("json rule does not contain ip_cidr/source_ip_cidr")
	}
	return nil
}

func parseFirewallGeoJSONStringList(raw any) ([]string, error) {
	if raw == nil {
		return nil, nil
	}
	switch value := raw.(type) {
	case string:
		value = strings.TrimSpace(value)
		if value == "" {
			return nil, nil
		}
		return []string{value}, nil
	case []any:
		result := make([]string, 0, len(value))
		for _, item := range value {
			str, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("expected string list")
			}
			str = strings.TrimSpace(str)
			if str == "" {
				continue
			}
			result = append(result, str)
		}
		return result, nil
	default:
		return nil, fmt.Errorf("expected string or string list")
	}
}

func parseFirewallGeoJSONBool(raw any) bool {
	value, ok := raw.(bool)
	return ok && value
}

func parseFirewallGeoTXT(content []byte) (firewallGeoResolvedPrefixes, error) {
	lines := strings.Split(strings.ReplaceAll(string(content), "\r\n", "\n"), "\n")
	var builder netipx.IPSetBuilder
	count := 0
	for _, line := range lines {
		entry := strings.TrimSpace(line)
		if entry == "" || strings.HasPrefix(entry, "#") {
			continue
		}
		if strings.EqualFold(entry, "payload:") {
			continue
		}
		if strings.HasPrefix(entry, "-") {
			entry = strings.TrimSpace(strings.TrimPrefix(entry, "-"))
		}
		entry = strings.Trim(entry, "\"'")
		if entry == "" {
			continue
		}
		if strings.Contains(entry, ",") {
			parts := strings.Split(entry, ",")
			if len(parts) >= 2 {
				keyword := strings.ToUpper(strings.TrimSpace(parts[0]))
				switch keyword {
				case "IP-CIDR", "IP-CIDR6", "SRC-IP-CIDR", "SRC-IP-CIDR6":
					entry = strings.TrimSpace(parts[1])
				default:
					return firewallGeoResolvedPrefixes{}, fmt.Errorf("unsupported txt rule line: %s", entry)
				}
			}
		}
		if err := addFirewallGeoPrefixString(&builder, entry); err != nil {
			return firewallGeoResolvedPrefixes{}, fmt.Errorf("invalid txt rule line %q: %w", entry, err)
		}
		count++
	}
	if count == 0 {
		return firewallGeoResolvedPrefixes{}, fmt.Errorf("txt ruleset contains no valid ip prefixes")
	}
	return buildFirewallGeoResolvedPrefixes(&builder)
}

func addFirewallGeoPrefixString(builder *netipx.IPSetBuilder, raw string) error {
	value := strings.TrimSpace(raw)
	if value == "" {
		return nil
	}
	if strings.Contains(value, "/") {
		prefix, err := netip.ParsePrefix(value)
		if err != nil {
			return err
		}
		builder.AddPrefix(prefix.Masked())
		return nil
	}
	addr, err := netip.ParseAddr(value)
	if err != nil {
		return err
	}
	builder.Add(addr.Unmap())
	return nil
}

func buildFirewallGeoResolvedPrefixes(builder *netipx.IPSetBuilder) (firewallGeoResolvedPrefixes, error) {
	set, err := builder.IPSet()
	if err != nil {
		return firewallGeoResolvedPrefixes{}, err
	}
	prefixes := set.Prefixes()
	if len(prefixes) == 0 {
		return firewallGeoResolvedPrefixes{}, fmt.Errorf("ruleset contains no usable ip prefixes")
	}

	result := firewallGeoResolvedPrefixes{
		All:  make([]string, 0, len(prefixes)),
		IPv4: make([]string, 0, len(prefixes)),
		IPv6: make([]string, 0, len(prefixes)),
	}
	hashBuilder := strings.Builder{}
	for _, prefix := range prefixes {
		value := prefix.Masked().String()
		result.All = append(result.All, value)
		if prefix.Addr().Is4() {
			result.IPv4 = append(result.IPv4, value)
		} else {
			result.IPv6 = append(result.IPv6, value)
		}
		hashBuilder.WriteString(value)
		hashBuilder.WriteByte('\n')
	}
	sum := sha256.Sum256([]byte(hashBuilder.String()))
	result.PrefixCount = len(result.All)
	result.ContentHash = hex.EncodeToString(sum[:])
	return result, nil
}

func firewallGeoAddrFromRawBytes(raw []byte) (netip.Addr, error) {
	switch len(raw) {
	case 4:
		var addr [4]byte
		copy(addr[:], raw)
		return netip.AddrFrom4(addr).Unmap(), nil
	case 16:
		var addr [16]byte
		copy(addr[:], raw)
		return netip.AddrFrom16(addr).Unmap(), nil
	default:
		return netip.Addr{}, fmt.Errorf("unsupported ip length: %d", len(raw))
	}
}
