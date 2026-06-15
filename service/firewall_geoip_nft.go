package service

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"

	"github.com/alireza0/s-ui/database/model"
)

type firewallGeoRenderGroup struct {
	Action   string
	Family   string
	Protocol string
	PortSpec string
	Prefixes map[string]struct{}
}

func addManagedFirewallGeoRules(rows []model.FirewallGeoRule) error {
	if len(rows) == 0 {
		return nil
	}

	blockGroups := make(map[string]*firewallGeoRenderGroup)
	allowGroups := make(map[string]*firewallGeoRenderGroup)

	for _, row := range rows {
		if !row.Enabled {
			continue
		}
		runtimeEntry, exists := currentFirewallGeoRuntime(row.Id)
		if !exists {
			return fmt.Errorf("geo rule runtime missing: %d", row.Id)
		}
		if err := collectFirewallGeoRenderGroup(blockGroups, allowGroups, row, runtimeEntry); err != nil {
			return err
		}
	}

	for _, group := range sortFirewallGeoRenderGroups(blockGroups) {
		if err := addManagedFirewallGeoBlockGroup(*group); err != nil {
			return err
		}
	}
	for _, group := range sortFirewallGeoRenderGroups(allowGroups) {
		if err := addManagedFirewallGeoAllowGroup(*group); err != nil {
			return err
		}
	}
	return nil
}

func appendManagedFirewallGeoRulesScript(script *strings.Builder, rows []model.FirewallGeoRule) error {
	if len(rows) == 0 {
		return nil
	}

	blockGroups := make(map[string]*firewallGeoRenderGroup)
	allowGroups := make(map[string]*firewallGeoRenderGroup)

	for _, row := range rows {
		if !row.Enabled {
			continue
		}
		runtimeEntry, exists := currentFirewallGeoRuntime(row.Id)
		if !exists {
			return fmt.Errorf("geo rule runtime missing: %d", row.Id)
		}
		if err := collectFirewallGeoRenderGroup(blockGroups, allowGroups, row, runtimeEntry); err != nil {
			return err
		}
	}

	for _, group := range sortFirewallGeoRenderGroups(blockGroups) {
		if err := appendManagedFirewallGeoBlockGroupScript(script, *group); err != nil {
			return err
		}
	}
	for _, group := range sortFirewallGeoRenderGroups(allowGroups) {
		if err := appendManagedFirewallGeoAllowGroupScript(script, *group); err != nil {
			return err
		}
	}
	return nil
}

func collectFirewallGeoRenderGroup(
	blockGroups map[string]*firewallGeoRenderGroup,
	allowGroups map[string]*firewallGeoRenderGroup,
	row model.FirewallGeoRule,
	runtimeEntry firewallGeoResolvedPrefixes,
) error {
	switch row.Family {
	case firewallFamilyIPv4:
		addFirewallGeoRenderGroupEntry(selectFirewallGeoGroupMap(row.Action, blockGroups, allowGroups), row, firewallFamilyIPv4, runtimeEntry.IPv4, row.Action == firewallGeoRuleActionAllow)
	case firewallFamilyIPv6:
		addFirewallGeoRenderGroupEntry(selectFirewallGeoGroupMap(row.Action, blockGroups, allowGroups), row, firewallFamilyIPv6, runtimeEntry.IPv6, row.Action == firewallGeoRuleActionAllow)
	case firewallFamilyDual:
		addFirewallGeoRenderGroupEntry(selectFirewallGeoGroupMap(row.Action, blockGroups, allowGroups), row, firewallFamilyIPv4, runtimeEntry.IPv4, row.Action == firewallGeoRuleActionAllow)
		addFirewallGeoRenderGroupEntry(selectFirewallGeoGroupMap(row.Action, blockGroups, allowGroups), row, firewallFamilyIPv6, runtimeEntry.IPv6, row.Action == firewallGeoRuleActionAllow)
	default:
		return fmt.Errorf("unsupported geo rule family: %s", row.Family)
	}
	return nil
}

func selectFirewallGeoGroupMap(
	action string,
	blockGroups map[string]*firewallGeoRenderGroup,
	allowGroups map[string]*firewallGeoRenderGroup,
) map[string]*firewallGeoRenderGroup {
	if action == firewallGeoRuleActionAllow {
		return allowGroups
	}
	return blockGroups
}

func addFirewallGeoRenderGroupEntry(
	groups map[string]*firewallGeoRenderGroup,
	row model.FirewallGeoRule,
	family string,
	prefixes []string,
	createIfEmpty bool,
) {
	if len(prefixes) == 0 && !createIfEmpty {
		return
	}
	key := strings.Join([]string{row.Action, family, row.Protocol, row.PortSpec}, "|")
	group, exists := groups[key]
	if !exists {
		group = &firewallGeoRenderGroup{
			Action:   row.Action,
			Family:   family,
			Protocol: row.Protocol,
			PortSpec: row.PortSpec,
			Prefixes: make(map[string]struct{}, len(prefixes)),
		}
		groups[key] = group
	}
	for _, prefix := range prefixes {
		group.Prefixes[prefix] = struct{}{}
	}
}

func sortFirewallGeoRenderGroups(groups map[string]*firewallGeoRenderGroup) []*firewallGeoRenderGroup {
	result := make([]*firewallGeoRenderGroup, 0, len(groups))
	for _, group := range groups {
		result = append(result, group)
	}
	sort.SliceStable(result, func(i, j int) bool {
		left := strings.Join([]string{result[i].Family, result[i].Protocol, result[i].PortSpec}, "|")
		right := strings.Join([]string{result[j].Family, result[j].Protocol, result[j].PortSpec}, "|")
		return left < right
	})
	return result
}

func addManagedFirewallGeoBlockGroup(group firewallGeoRenderGroup) error {
	setName := firewallGeoSetName(group.Action, group.Family, group.Protocol, group.PortSpec)
	if err := addManagedFirewallGeoSet(setName, group.Family, flattenFirewallGeoPrefixes(group.Prefixes)); err != nil {
		return err
	}
	args, err := buildManagedFirewallGeoRuleArgs(group, setName)
	if err != nil {
		return err
	}
	args = append(args, "counter", "drop", "comment", firewallGeoRuleComment("block", group, "match"))
	if _, err := runNft(args...); err != nil {
		return err
	}
	return nil
}

func appendManagedFirewallGeoBlockGroupScript(script *strings.Builder, group firewallGeoRenderGroup) error {
	setName := firewallGeoSetName(group.Action, group.Family, group.Protocol, group.PortSpec)
	appendManagedFirewallGeoSetScript(script, setName, group.Family, flattenFirewallGeoPrefixes(group.Prefixes))
	args, err := buildManagedFirewallGeoRuleArgs(group, setName)
	if err != nil {
		return err
	}
	args = append(args, "counter", "drop", "comment", firewallGeoRuleComment("block", group, "match"))
	appendFirewallScriptArgs(script, args)
	return nil
}

func addManagedFirewallGeoAllowGroup(group firewallGeoRenderGroup) error {
	if len(group.Prefixes) > 0 {
		setName := firewallGeoSetName(group.Action, group.Family, group.Protocol, group.PortSpec)
		if err := addManagedFirewallGeoSet(setName, group.Family, flattenFirewallGeoPrefixes(group.Prefixes)); err != nil {
			return err
		}
		acceptArgs, err := buildManagedFirewallGeoRuleArgs(group, setName)
		if err != nil {
			return err
		}
		acceptArgs = append(acceptArgs, "counter", "accept", "comment", firewallGeoRuleComment("allow", group, "match"))
		if _, err := runNft(acceptArgs...); err != nil {
			return err
		}
	}

	dropArgs, err := buildManagedFirewallGeoPortOnlyArgs(group)
	if err != nil {
		return err
	}
	dropArgs = append(dropArgs, "counter", "drop", "comment", firewallGeoRuleComment("allow", group, "fallback_drop"))
	if _, err := runNft(dropArgs...); err != nil {
		return err
	}
	return nil
}

func appendManagedFirewallGeoAllowGroupScript(script *strings.Builder, group firewallGeoRenderGroup) error {
	if len(group.Prefixes) > 0 {
		setName := firewallGeoSetName(group.Action, group.Family, group.Protocol, group.PortSpec)
		appendManagedFirewallGeoSetScript(script, setName, group.Family, flattenFirewallGeoPrefixes(group.Prefixes))
		acceptArgs, err := buildManagedFirewallGeoRuleArgs(group, setName)
		if err != nil {
			return err
		}
		acceptArgs = append(acceptArgs, "counter", "accept", "comment", firewallGeoRuleComment("allow", group, "match"))
		appendFirewallScriptArgs(script, acceptArgs)
	}

	dropArgs, err := buildManagedFirewallGeoPortOnlyArgs(group)
	if err != nil {
		return err
	}
	dropArgs = append(dropArgs, "counter", "drop", "comment", firewallGeoRuleComment("allow", group, "fallback_drop"))
	appendFirewallScriptArgs(script, dropArgs)
	return nil
}

func flattenFirewallGeoPrefixes(prefixSet map[string]struct{}) []string {
	result := make([]string, 0, len(prefixSet))
	for prefix := range prefixSet {
		result = append(result, prefix)
	}
	sort.Strings(result)
	return result
}

func buildManagedFirewallGeoRuleArgs(group firewallGeoRenderGroup, setName string) ([]string, error) {
	args, err := buildManagedFirewallGeoPortOnlyArgs(group)
	if err != nil {
		return nil, err
	}
	switch group.Family {
	case firewallFamilyIPv4:
		args = append(args, "ip", "saddr", "@"+setName)
	case firewallFamilyIPv6:
		args = append(args, "ip6", "saddr", "@"+setName)
	default:
		return nil, fmt.Errorf("unsupported geo family: %s", group.Family)
	}
	return args, nil
}

func buildManagedFirewallGeoPortOnlyArgs(group firewallGeoRenderGroup) ([]string, error) {
	args := []string{
		"add", "rule", nftFamily, firewallNftTable, firewallInputChain,
		"meta", "nfproto", mapFirewallTargetFamily(group.Family),
	}
	switch group.Protocol {
	case firewallProtocolTCP:
		args = append(args, "meta", "l4proto", "tcp", "th", "dport")
	case firewallProtocolUDP:
		args = append(args, "meta", "l4proto", "udp", "th", "dport")
	case firewallProtocolTCPUDP:
		args = append(args, "meta", "l4proto", "{", "tcp", ",", "udp", "}", "th", "dport")
	default:
		return nil, fmt.Errorf("unsupported geo protocol: %s", group.Protocol)
	}
	args = append(args, buildNftPortSetArgs(group.PortSpec)...)
	return args, nil
}

func addManagedFirewallGeoSet(setName string, family string, prefixes []string) error {
	if len(prefixes) == 0 {
		return nil
	}
	addrType := "ipv4_addr"
	if family == firewallFamilyIPv6 {
		addrType = "ipv6_addr"
	}

	script := &strings.Builder{}
	script.WriteString(fmt.Sprintf("add set %s %s %s { type %s; flags interval; }\n", nftFamily, firewallNftTable, setName, addrType))
	for _, chunk := range chunkFirewallGeoPrefixes(prefixes, 180) {
		script.WriteString(fmt.Sprintf("add element %s %s %s { %s }\n", nftFamily, firewallNftTable, setName, strings.Join(chunk, ", ")))
	}
	_, err := runNftScript(script.String())
	return err
}

func appendManagedFirewallGeoSetScript(script *strings.Builder, setName string, family string, prefixes []string) {
	if len(prefixes) == 0 {
		return
	}
	addrType := "ipv4_addr"
	if family == firewallFamilyIPv6 {
		addrType = "ipv6_addr"
	}

	script.WriteString(fmt.Sprintf("add set %s %s %s { type %s; flags interval; }\n", nftFamily, firewallNftTable, setName, addrType))
	for _, chunk := range chunkFirewallGeoPrefixes(prefixes, 180) {
		script.WriteString(fmt.Sprintf("add element %s %s %s { %s }\n", nftFamily, firewallNftTable, setName, strings.Join(chunk, ", ")))
	}
}

func chunkFirewallGeoPrefixes(prefixes []string, chunkSize int) [][]string {
	if len(prefixes) == 0 {
		return nil
	}
	if chunkSize <= 0 {
		chunkSize = 180
	}
	result := make([][]string, 0, (len(prefixes)+chunkSize-1)/chunkSize)
	for start := 0; start < len(prefixes); start += chunkSize {
		end := start + chunkSize
		if end > len(prefixes) {
			end = len(prefixes)
		}
		result = append(result, prefixes[start:end])
	}
	return result
}

func firewallGeoSetName(action string, family string, protocol string, portSpec string) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{action, family, protocol, portSpec}, "|")))
	prefix := "fgb"
	if action == firewallGeoRuleActionAllow {
		prefix = "fga"
	}
	familyCode := "4"
	if family == firewallFamilyIPv6 {
		familyCode = "6"
	}
	return fmt.Sprintf("%s%s_%s", prefix, familyCode, hex.EncodeToString(sum[:6]))
}

func firewallGeoRuleComment(kind string, group firewallGeoRenderGroup, suffix string) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{kind, group.Family, group.Protocol, group.PortSpec, suffix}, "|")))
	return "kwor_firewall_geo_" + hex.EncodeToString(sum[:8])
}
