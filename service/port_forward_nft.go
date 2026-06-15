package service

import (
	"fmt"
	"net/netip"
	"os"
	"strconv"
	"strings"

	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/logger"
)

type portForwardLimitRuntime struct {
	effectiveRateLimitMbps int
	status                 string
	warning                string
}

func ensureManagedPortForwardBase() error {
	if !portForwardSupported() {
		return nil
	}

	if _, err := runNft("list", "table", nftFamily, portForwardNftTable); err != nil {
		if _, addErr := runNft("add", "table", nftFamily, portForwardNftTable); addErr != nil {
			return addErr
		}
	}

	chains := []struct {
		name string
		spec []string
	}{
		{
			name: portForwardPreroutingChain,
			spec: []string{"type", "nat", "hook", "prerouting", "priority", "dstnat", ";", "policy", "accept", ";"},
		},
		{
			name: portForwardPostroutingChain,
			spec: []string{"type", "nat", "hook", "postrouting", "priority", "srcnat", ";", "policy", "accept", ";"},
		},
		{
			name: portForwardForwardChain,
			spec: []string{"type", "filter", "hook", "forward", "priority", "0", ";", "policy", "accept", ";"},
		},
		{
			name: portForwardInputChain,
			spec: []string{"type", "filter", "hook", "input", "priority", "0", ";", "policy", "accept", ";"},
		},
		{
			name: portForwardOutputChain,
			spec: []string{"type", "filter", "hook", "output", "priority", "0", ";", "policy", "accept", ";"},
		},
	}

	for _, chain := range chains {
		if _, err := runNft("list", "chain", nftFamily, portForwardNftTable, chain.name); err == nil {
			continue
		}
		args := []string{"add", "chain", nftFamily, portForwardNftTable, chain.name, "{"}
		args = append(args, chain.spec...)
		args = append(args, "}")
		if _, err := runNft(args...); err != nil {
			return err
		}
	}
	return nil
}

func flushManagedPortForwardChains() error {
	if !portForwardSupported() || !portForwardTableExists() {
		return nil
	}
	if err := ensureManagedPortForwardBase(); err != nil {
		return err
	}
	for _, chain := range []string{
		portForwardPreroutingChain,
		portForwardPostroutingChain,
		portForwardForwardChain,
		portForwardInputChain,
		portForwardOutputChain,
	} {
		if _, err := runNft("flush", "chain", nftFamily, portForwardNftTable, chain); err != nil {
			return err
		}
	}
	return nil
}

func ensureKernelForwardingForRows(rows []model.PortForwardRule) error {
	needIPv4 := false
	needIPv6 := false
	for _, row := range rows {
		if !row.Enabled {
			continue
		}
		if portForwardTargetIsLocal(row.TargetIP) {
			continue
		}
		flags := portForwardFamilyFlagsFor(row.Family)
		if !flags.ipv4 && !flags.ipv6 {
			needIPv4 = true
			continue
		}
		needIPv4 = needIPv4 || flags.ipv4
		needIPv6 = needIPv6 || flags.ipv6
	}
	if needIPv4 {
		if err := ensureKernelForwardingEnabled("/proc/sys/net/ipv4/ip_forward"); err != nil {
			return err
		}
	}
	if needIPv6 {
		if err := ensureKernelForwardingEnabled("/proc/sys/net/ipv6/conf/all/forwarding"); err != nil {
			return err
		}
	}
	return nil
}

func ensureKernelForwardingEnabled(filePath string) error {
	if readKernelForwardingEnabled(filePath) {
		return nil
	}
	return os.WriteFile(filePath, []byte("1\n"), 0o644)
}

func readKernelForwardingEnabled(filePath string) bool {
	body, err := os.ReadFile(filePath)
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(body)) == "1"
}

func ensureManagedPortForwardNamedCounter(counterName string) error {
	if counterName == "" {
		return nil
	}
	if _, err := runNft("list", "counter", nftFamily, portForwardNftTable, counterName); err == nil {
		return nil
	}
	_, err := runNft("add", "counter", nftFamily, portForwardNftTable, counterName)
	return err
}

func deletePortForwardNamedCounter(counterName string) error {
	if counterName == "" || !portForwardSupported() || !portForwardTableExists() {
		return nil
	}
	_, err := runNft("delete", "counter", nftFamily, portForwardNftTable, counterName)
	if portForwardNftObjectMissing(err) {
		return nil
	}
	return err
}

func cleanupPortForwardNftObjects(ruleID uint) {
	if ruleID == 0 {
		return
	}
	_ = deletePortForwardNamedCounter(portForwardCounterName(ruleID, "up"))
	_ = deletePortForwardNamedCounter(portForwardCounterName(ruleID, "down"))
}

func addManagedPortForwardRule(row model.PortForwardRule) (portForwardLimitRuntime, error) {
	upCounter := portForwardCounterName(row.Id, "up")
	downCounter := portForwardCounterName(row.Id, "down")
	if err := ensureManagedPortForwardNamedCounter(upCounter); err != nil {
		return portForwardLimitRuntime{}, err
	}
	if err := ensureManagedPortForwardNamedCounter(downCounter); err != nil {
		return portForwardLimitRuntime{}, err
	}

	families := portForwardExpandFamilies(row.Family)
	if len(families) == 0 {
		families = []string{portForwardFamilyIPv4}
	}

	warnings := make([]string, 0, len(families))
	for _, family := range families {
		var state portForwardLimitRuntime
		var err error
		if portForwardTargetIsLocal(row.TargetIP) {
			state, err = addManagedLocalPortForwardRuleForFamily(row, family, upCounter, downCounter)
		} else {
			state, err = addManagedRemotePortForwardRuleForFamily(row, family, upCounter, downCounter)
		}
		if err != nil {
			return portForwardLimitRuntime{}, err
		}
		if strings.TrimSpace(state.warning) != "" {
			warnings = append(warnings, strings.TrimSpace(state.warning))
		}
	}

	if row.RateLimitMbps > 0 {
		if len(warnings) > 0 {
			return portForwardLimitRuntime{
				effectiveRateLimitMbps: 0,
				status:                 "degraded",
				warning:                strings.Join(warnings, "；"),
			}, nil
		}
		return portForwardLimitRuntime{
			effectiveRateLimitMbps: row.RateLimitMbps,
			status:                 "applied",
		}, nil
	}

	if len(warnings) > 0 {
		return portForwardLimitRuntime{
			effectiveRateLimitMbps: 0,
			status:                 "degraded",
			warning:                strings.Join(warnings, "；"),
		}, nil
	}

	return portForwardLimitRuntime{
		effectiveRateLimitMbps: 0,
		status:                 "disabled",
	}, nil
}

func addManagedRemotePortForwardRuleForFamily(row model.PortForwardRule, family string, upCounter string, downCounter string) (portForwardLimitRuntime, error) {
	dnatArgs := []string{
		"add", "rule", nftFamily, portForwardNftTable, portForwardPreroutingChain,
		"meta", "nfproto", mapFirewallTargetFamily(family),
	}
	dnatArgs = appendPortForwardProtocolMatch(dnatArgs, row.Protocol)
	dnatArgs = append(dnatArgs, "th", "dport")
	dnatArgs = append(dnatArgs, buildNftPortSetArgs(row.LocalPortSpec)...)
	dnatArgs = append(dnatArgs,
		"counter",
		"dnat", "to", portForwardNatTargetValue(row.TargetIP, row.TargetPort),
		"comment", portForwardRuleComment(row.Id, "dnat"),
	)
	if _, err := runNft(dnatArgs...); err != nil {
		return portForwardLimitRuntime{}, err
	}

	masqueradeArgs := []string{
		"add", "rule", nftFamily, portForwardNftTable, portForwardPostroutingChain,
		"meta", "nfproto", mapFirewallTargetFamily(family),
	}
	masqueradeArgs = appendPortForwardProtocolMatch(masqueradeArgs, row.Protocol)
	masqueradeArgs = append(masqueradeArgs,
		"ct", "status", "dnat",
		"ct", "direction", "original",
		"ct", "original", "proto-dst",
	)
	masqueradeArgs = append(masqueradeArgs, buildNftPortSetArgs(row.LocalPortSpec)...)
	masqueradeArgs = append(masqueradeArgs,
		"counter",
		"masquerade",
		"comment", portForwardRuleComment(row.Id, "snat"),
	)
	if _, err := runNft(masqueradeArgs...); err != nil {
		return portForwardLimitRuntime{}, err
	}

	trackedArgs := buildPortForwardTrackedArgs(row, family)

	downArgs := append([]string{"add", "rule", nftFamily, portForwardNftTable, portForwardForwardChain}, trackedArgs...)
	downArgs = append(downArgs,
		"ct", "direction", "original",
		"counter", "name", downCounter,
		"comment", portForwardRuleComment(row.Id, "down"),
	)
	if _, err := runNft(downArgs...); err != nil {
		return portForwardLimitRuntime{}, err
	}

	upArgs := append([]string{"add", "rule", nftFamily, portForwardNftTable, portForwardForwardChain}, trackedArgs...)
	upArgs = append(upArgs,
		"ct", "direction", "reply",
		"counter", "name", upCounter,
		"comment", portForwardRuleComment(row.Id, "up"),
	)
	if _, err := runNft(upArgs...); err != nil {
		return portForwardLimitRuntime{}, err
	}

	if row.RateLimitMbps > 0 {
		bytesPerSecond := int64(row.RateLimitMbps) * 125000
		limitArgs := append([]string{"add", "rule", nftFamily, portForwardNftTable, portForwardForwardChain}, trackedArgs...)
		limitArgs = append(limitArgs,
			"ct", "direction", "original",
			"limit", "rate", "over", strconv.FormatInt(bytesPerSecond, 10), "bytes/second",
			"counter",
			"drop",
			"comment", portForwardRuleComment(row.Id, "limit"),
		)
		if _, err := runNft(limitArgs...); err != nil {
			state := portForwardLimitRuntime{
				warning:                fmt.Sprintf("规则 %s 的 %s 限速未生效: %s", strings.TrimSpace(row.Name), portForwardProtocolDisplay(row.Protocol), strings.TrimSpace(err.Error())),
				status:                 "degraded",
				effectiveRateLimitMbps: 0,
			}
			logger.Warning(state.warning)
			return state, nil
		}
		return portForwardLimitRuntime{
			status:                 "applied",
			effectiveRateLimitMbps: row.RateLimitMbps,
		}, nil
	}

	return portForwardLimitRuntime{
		status:                 "disabled",
		effectiveRateLimitMbps: 0,
	}, nil
}

func addManagedLocalPortForwardRuleForFamily(row model.PortForwardRule, family string, upCounter string, downCounter string) (portForwardLimitRuntime, error) {
	redirectArgs := []string{
		"add", "rule", nftFamily, portForwardNftTable, portForwardPreroutingChain,
		"meta", "nfproto", mapFirewallTargetFamily(family),
	}
	redirectArgs = appendPortForwardProtocolMatch(redirectArgs, row.Protocol)
	redirectArgs = append(redirectArgs, "th", "dport")
	redirectArgs = append(redirectArgs, buildNftPortSetArgs(row.LocalPortSpec)...)
	redirectArgs = append(redirectArgs,
		"counter",
		"redirect", "to", fmt.Sprintf(":%d", row.TargetPort),
		"comment", portForwardRuleComment(row.Id, "dnat"),
	)
	if _, err := runNft(redirectArgs...); err != nil {
		return portForwardLimitRuntime{}, err
	}

	trackedArgs := buildPortForwardTrackedArgs(row, family)

	downArgs := append([]string{"add", "rule", nftFamily, portForwardNftTable, portForwardInputChain}, trackedArgs...)
	downArgs = append(downArgs,
		"ct", "direction", "original",
		"counter", "name", downCounter,
		"comment", portForwardRuleComment(row.Id, "down"),
	)
	if _, err := runNft(downArgs...); err != nil {
		return portForwardLimitRuntime{}, err
	}

	upArgs := append([]string{"add", "rule", nftFamily, portForwardNftTable, portForwardOutputChain}, trackedArgs...)
	upArgs = append(upArgs,
		"ct", "direction", "reply",
		"counter", "name", upCounter,
		"comment", portForwardRuleComment(row.Id, "up"),
	)
	if _, err := runNft(upArgs...); err != nil {
		return portForwardLimitRuntime{}, err
	}

	if row.RateLimitMbps > 0 {
		bytesPerSecond := int64(row.RateLimitMbps) * 125000
		limitArgs := append([]string{"add", "rule", nftFamily, portForwardNftTable, portForwardInputChain}, trackedArgs...)
		limitArgs = append(limitArgs,
			"ct", "direction", "original",
			"limit", "rate", "over", strconv.FormatInt(bytesPerSecond, 10), "bytes/second",
			"counter",
			"drop",
			"comment", portForwardRuleComment(row.Id, "limit"),
		)
		if _, err := runNft(limitArgs...); err != nil {
			state := portForwardLimitRuntime{
				warning:                fmt.Sprintf("规则 %s 的 %s 限速未生效: %s", strings.TrimSpace(row.Name), portForwardProtocolDisplay(row.Protocol), strings.TrimSpace(err.Error())),
				status:                 "degraded",
				effectiveRateLimitMbps: 0,
			}
			logger.Warning(state.warning)
			return state, nil
		}
		return portForwardLimitRuntime{
			status:                 "applied",
			effectiveRateLimitMbps: row.RateLimitMbps,
		}, nil
	}
	return portForwardLimitRuntime{
		status:                 "disabled",
		effectiveRateLimitMbps: 0,
	}, nil
}

func appendPortForwardProtocolMatch(args []string, protocol string) []string {
	flags := portForwardProtocolFlagsFor(protocol)
	args = append(args, "meta", "l4proto")
	switch {
	case flags.tcp && flags.udp:
		args = append(args, "{", "tcp", ",", "udp", "}")
	case flags.tcp:
		args = append(args, "tcp")
	case flags.udp:
		args = append(args, "udp")
	default:
		fallback := strings.ToLower(strings.TrimSpace(protocol))
		if fallback == "" {
			fallback = "tcp"
		}
		args = append(args, fallback)
	}
	return args
}

func buildPortForwardTrackedArgs(row model.PortForwardRule, family string) []string {
	args := []string{
		"meta", "nfproto", mapFirewallTargetFamily(family),
	}
	args = appendPortForwardProtocolMatch(args, row.Protocol)
	args = append(args,
		"ct", "status", "dnat",
		"ct", "original", "proto-dst",
	)
	args = append(args, buildNftPortSetArgs(row.LocalPortSpec)...)
	return args
}

func (s *PortForwardService) savePortForwardLimitStates(states map[uint]portForwardLimitRuntime) {
	savePortForwardLimitStates(states)
}

func readPortForwardCounterBytes() (map[string]int64, error) {
	result := make(map[string]int64)
	if !portForwardSupported() || !portForwardTableExists() {
		return result, nil
	}

	out, err := runNft("list", "table", nftFamily, portForwardNftTable)
	if err != nil {
		return nil, err
	}
	for _, match := range portForwardCounterBlockRe.FindAllStringSubmatch(string(out), -1) {
		if len(match) != 4 {
			continue
		}
		value, parseErr := strconv.ParseInt(match[3], 10, 64)
		if parseErr != nil {
			continue
		}
		result[match[1]] = value
	}
	return result, nil
}

func portForwardFindHandleByComment(chain string, comment string) int {
	if !portForwardSupported() || comment == "" {
		return 0
	}

	out, err := runNft("--handle", "--numeric", "list", "chain", nftFamily, portForwardNftTable, chain)
	if err != nil {
		return 0
	}

	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		if !ruleLineHasExactComment(line, comment) || !strings.Contains(line, "handle") {
			continue
		}
		match := nftHandleRe.FindStringSubmatch(line)
		if len(match) != 2 {
			continue
		}
		handle := 0
		_, _ = fmt.Sscanf(match[1], "%d", &handle)
		if handle > 0 {
			return handle
		}
	}
	return 0
}

func portForwardRenderIntact(activeRows []model.PortForwardRule) bool {
	if !portForwardTableExists() {
		return false
	}
	if len(activeRows) == 0 {
		return true
	}
	return portForwardFindHandleByComment(portForwardPreroutingChain, portForwardRuleComment(activeRows[0].Id, "dnat")) > 0
}

func portForwardRuleComment(ruleID uint, suffix string) string {
	return fmt.Sprintf("kwor_pf_rule_%d_%s", ruleID, strings.TrimSpace(suffix))
}

func portForwardCounterName(ruleID uint, direction string) string {
	return fmt.Sprintf("kwor_pf_counter_%d_%s", ruleID, strings.TrimSpace(direction))
}

func portForwardNatTargetValue(targetIP string, targetPort int) string {
	addr, err := netip.ParseAddr(targetIP)
	if err != nil {
		return targetIP + ":" + strconv.Itoa(targetPort)
	}
	if addr.Is6() {
		return "[" + addr.String() + "]:" + strconv.Itoa(targetPort)
	}
	return addr.String() + ":" + strconv.Itoa(targetPort)
}

func portForwardNftObjectMissing(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(message, "no such file or directory") ||
		strings.Contains(message, "no such file") ||
		strings.Contains(message, "not found")
}
