package service

import (
	"bufio"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

var (
	firewallRuleHandleTextRe = regexp.MustCompile(`#\s*handle\s+(\d+)`)
	firewallDportTextRe      = regexp.MustCompile(`(?:\bth\s+)?dport\s+(\{[^}]+\}|[0-9][0-9,\-\s]*)`)
	firewallIPv4SourceTextRe = regexp.MustCompile(`\bip\s+saddr\s+(\{[^}]+\}|[0-9./,\s]+)`)
	firewallIPv6SourceTextRe = regexp.MustCompile(`\bip6\s+saddr\s+(\{[^}]+\}|[0-9a-fA-F:./,\s]+)`)
)

type nftBlockFrame struct {
	kind   string
	family string
	name   string
}

func scanExternalFirewallRules() ([]firewallObservedRule, error) {
	if !firewallSupported() {
		return nil, nil
	}

	out, err := runNft("--handle", "--numeric", "list", "ruleset")
	if err != nil {
		return nil, err
	}

	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	stack := make([]nftBlockFrame, 0, 4)
	rules := make([]firewallObservedRule, 0)
	now := time.Now().Unix()

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		if frame, ok := parseNftTableHeader(line); ok {
			stack = append(stack, frame)
			continue
		}
		if frame, ok := parseNftChainHeader(line); ok {
			stack = append(stack, frame)
			continue
		}
		if strings.HasPrefix(line, "set ") && strings.HasSuffix(line, "{") {
			stack = append(stack, nftBlockFrame{kind: "set"})
			continue
		}
		if strings.HasPrefix(line, "map ") && strings.HasSuffix(line, "{") {
			stack = append(stack, nftBlockFrame{kind: "map"})
			continue
		}
		if line == "}" {
			if len(stack) > 0 {
				stack = stack[:len(stack)-1]
			}
			continue
		}

		tableFamily, tableName, chainName := currentFirewallScanContext(stack)
		if tableName == "" || chainName == "" {
			continue
		}
		if tableName == nftTable || tableName == firewallNftTable {
			continue
		}
		if isIgnoredFirewallChain(chainName) {
			continue
		}

		rule, ok := parseObservedFirewallRuleLine(line, tableFamily, tableName, chainName, now)
		if !ok {
			continue
		}
		rules = append(rules, rule)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}
	sortObservedFirewallRules(rules)
	return rules, nil
}

func parseNftTableHeader(line string) (nftBlockFrame, bool) {
	fields := strings.Fields(line)
	if len(fields) < 4 || fields[0] != "table" || !strings.HasSuffix(line, "{") {
		return nftBlockFrame{}, false
	}
	return nftBlockFrame{
		kind:   "table",
		family: strings.TrimSpace(fields[1]),
		name:   strings.TrimSpace(fields[2]),
	}, true
}

func parseNftChainHeader(line string) (nftBlockFrame, bool) {
	fields := strings.Fields(line)
	if len(fields) < 3 || fields[0] != "chain" || !strings.HasSuffix(line, "{") {
		return nftBlockFrame{}, false
	}
	return nftBlockFrame{
		kind: "chain",
		name: strings.TrimSpace(fields[1]),
	}, true
}

func currentFirewallScanContext(stack []nftBlockFrame) (string, string, string) {
	tableFamily := ""
	tableName := ""
	chainName := ""
	for _, frame := range stack {
		switch frame.kind {
		case "table":
			tableFamily = frame.family
			tableName = frame.name
		case "chain":
			chainName = frame.name
		}
	}
	return tableFamily, tableName, chainName
}

func isIgnoredFirewallChain(chain string) bool {
	normalized := strings.TrimSpace(strings.ToLower(chain))
	switch normalized {
	case "", "output", "postrouting", "prerouting", "forward":
		return true
	default:
		return false
	}
}

func parseObservedFirewallRuleLine(line string, tableFamily string, tableName string, chainName string, observedAt int64) (firewallObservedRule, bool) {
	comment, _ := extractRuleComment(line)
	if strings.HasPrefix(comment, "kwor_") {
		return firewallObservedRule{}, false
	}

	handleMatch := firewallRuleHandleTextRe.FindStringSubmatch(line)
	if len(handleMatch) != 2 {
		return firewallObservedRule{}, false
	}
	handle, err := strconv.Atoi(handleMatch[1])
	if err != nil || handle <= 0 {
		return firewallObservedRule{}, false
	}
	if !strings.Contains(line, "accept") {
		return firewallObservedRule{}, false
	}

	portMatch := firewallDportTextRe.FindStringSubmatch(line)
	if len(portMatch) != 2 {
		return firewallObservedRule{}, false
	}
	portSpec, err := normalizeFirewallPortSpec(cleanNftListValue(portMatch[1]), detectObservedFirewallProtocol(line))
	if err != nil {
		return firewallObservedRule{}, false
	}
	protocol := detectObservedFirewallProtocol(line)
	if protocol == "" || protocol == firewallProtocolAny {
		return firewallObservedRule{}, false
	}

	sourceSpec := ""
	if match := firewallIPv4SourceTextRe.FindStringSubmatch(line); len(match) == 2 {
		normalized, normalizeErr := normalizeFirewallSourceSpec(cleanNftListValue(match[1]), firewallFamilyIPv4)
		if normalizeErr != nil {
			return firewallObservedRule{}, false
		}
		sourceSpec = normalized
	}
	if match := firewallIPv6SourceTextRe.FindStringSubmatch(line); len(match) == 2 {
		normalized, normalizeErr := normalizeFirewallSourceSpec(cleanNftListValue(match[1]), firewallFamilyIPv6)
		if normalizeErr != nil {
			return firewallObservedRule{}, false
		}
		if sourceSpec == "" {
			sourceSpec = normalized
		} else if normalized != "" {
			sourceSpec += ", " + normalized
		}
	}

	family := detectObservedFirewallFamily(tableFamily, line, sourceSpec)
	description := comment
	if strings.TrimSpace(description) == "" {
		description = fmt.Sprintf("导入规则 %s/%s#%d", tableName, chainName, handle)
	}

	return firewallObservedRule{
		Family:      family,
		TableFamily: strings.TrimSpace(tableFamily),
		Table:       strings.TrimSpace(tableName),
		Chain:       strings.TrimSpace(chainName),
		Handle:      handle,
		Protocol:    protocol,
		PortSpec:    portSpec,
		SourceSpec:  sourceSpec,
		Comment:     comment,
		Description: description,
		ObservedAt:  observedAt,
	}, true
}

func detectObservedFirewallProtocol(line string) string {
	normalized := strings.ToLower(line)
	if strings.Contains(normalized, "meta l4proto {") &&
		strings.Contains(normalized, "tcp") &&
		strings.Contains(normalized, "udp") &&
		strings.Contains(normalized, "dport") {
		return firewallProtocolTCPUDP
	}
	if strings.Contains(normalized, "meta l4proto tcp") || strings.Contains(normalized, " tcp dport") || strings.HasPrefix(normalized, "tcp dport") {
		return firewallProtocolTCP
	}
	if strings.Contains(normalized, "meta l4proto udp") || strings.Contains(normalized, " udp dport") || strings.HasPrefix(normalized, "udp dport") {
		return firewallProtocolUDP
	}
	return ""
}

func detectObservedFirewallFamily(tableFamily string, line string, sourceSpec string) string {
	switch strings.TrimSpace(strings.ToLower(tableFamily)) {
	case "ip":
		return firewallFamilyIPv4
	case "ip6":
		return firewallFamilyIPv6
	}
	lowerLine := strings.ToLower(line)
	switch {
	case strings.Contains(lowerLine, "meta nfproto ipv4"), strings.Contains(lowerLine, " ip saddr"), strings.HasPrefix(lowerLine, "ip saddr"):
		return firewallFamilyIPv4
	case strings.Contains(lowerLine, "meta nfproto ipv6"), strings.Contains(lowerLine, " ip6 saddr"), strings.HasPrefix(lowerLine, "ip6 saddr"):
		return firewallFamilyIPv6
	default:
		if strings.Contains(sourceSpec, "/32") || strings.Contains(sourceSpec, "/128") || strings.Contains(sourceSpec, ":") {
			v4Sources, v6Sources, err := splitFirewallSourcesByFamily(sourceSpec)
			if err == nil {
				if len(v4Sources) > 0 && len(v6Sources) == 0 {
					return firewallFamilyIPv4
				}
				if len(v6Sources) > 0 && len(v4Sources) == 0 {
					return firewallFamilyIPv6
				}
			}
		}
		return firewallFamilyDual
	}
}

func cleanNftListValue(raw string) string {
	value := strings.TrimSpace(raw)
	value = strings.TrimPrefix(value, "{")
	value = strings.TrimSuffix(value, "}")
	value = strings.ReplaceAll(value, " ", "")
	value = strings.ReplaceAll(value, "\t", "")
	return value
}

func sortObservedFirewallRules(rules []firewallObservedRule) {
	sort.SliceStable(rules, func(i, j int) bool {
		left := strings.Join([]string{rules[i].TableFamily, rules[i].Table, rules[i].Chain}, "|")
		right := strings.Join([]string{rules[j].TableFamily, rules[j].Table, rules[j].Chain}, "|")
		if left != right {
			return left < right
		}
		return rules[i].Handle < rules[j].Handle
	})
}
