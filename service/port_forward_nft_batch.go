package service

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/alireza0/s-ui/database/model"
)

// renderManagedPortForwardRulesAtomic flushes and rebuilds every panel-owned
// forwarding chain in one nft transaction. nft validates the complete script
// before committing it, so a rejected new rule leaves the prior ruleset in
// place instead of exposing the old clear-then-add interruption window.
func renderManagedPortForwardRulesAtomic(rows []model.PortForwardRule) (map[uint]portForwardLimitRuntime, []string, error) {
	return renderManagedPortForwardRulesAtomicWithTrafficBlocks(rows, nil)
}

func renderManagedPortForwardRulesAtomicWithTrafficBlocks(rows []model.PortForwardRule, trafficBlocks map[uint]string) (map[uint]portForwardLimitRuntime, []string, error) {
	if !nftUsesCompatibilityLayout() {
		for _, row := range rows {
			if err := ensureManagedPortForwardNamedCounter(portForwardCounterName(row.Id, "up")); err != nil {
				return nil, nil, err
			}
			if err := ensureManagedPortForwardNamedCounter(portForwardCounterName(row.Id, "down")); err != nil {
				return nil, nil, err
			}
		}
	}

	supportsMeters := GetNftablesCapabilities().SupportsMeters
	states, warnings, script := buildPortForwardAtomicScript(rows, trafficBlocks, supportsMeters, nil)
	if _, err := runNftScript(script); err == nil {
		return states, warnings, nil
	} else if !supportsMeters || !portForwardRowsRequestRateLimit(rows) {
		return nil, nil, err
	} else {
		fallbackStates, fallbackWarnings, fallbackScript := buildPortForwardAtomicScript(rows, trafficBlocks, false, err)
		if _, fallbackErr := runNftScript(fallbackScript); fallbackErr != nil {
			return nil, nil, fmt.Errorf("nft meter batch failed: %w; retry without meter failed: %v", err, fallbackErr)
		}
		markNftMetersUnsupported(err)
		return fallbackStates, fallbackWarnings, nil
	}
}

func buildPortForwardAtomicScript(
	rows []model.PortForwardRule,
	trafficBlocks map[uint]string,
	supportsMeters bool,
	meterErr error,
) (map[uint]portForwardLimitRuntime, []string, string) {
	commands := buildPortForwardAtomicFlushCommands()
	states := make(map[uint]portForwardLimitRuntime, len(rows))
	warnings := make([]string, 0)
	for _, row := range rows {
		limitState, ruleCommands := buildManagedPortForwardRuleCommandsForMeterSupport(
			row,
			trafficBlocks[row.Id],
			supportsMeters,
			meterErr,
		)
		commands = append(commands, ruleCommands...)
		states[row.Id] = limitState
		if warning := strings.TrimSpace(limitState.warning); warning != "" {
			warnings = append(warnings, warning)
		}
	}

	lines := make([]string, 0, len(commands))
	for _, command := range commands {
		lines = append(lines, portForwardNftScriptLine(command))
	}
	return states, warnings, strings.Join(lines, "\n") + "\n"
}

func portForwardRowsRequestRateLimit(rows []model.PortForwardRule) bool {
	for _, row := range rows {
		if row.Enabled && row.RateLimitMbps > 0 {
			return true
		}
	}
	return false
}

func buildPortForwardAtomicFlushCommands() [][]string {
	commands := make([][]string, 0, 9)
	for _, chain := range []string{portForwardForwardChain, portForwardInputChain, portForwardOutputChain} {
		commands = append(commands, []string{"flush", "chain", nftFamily, portForwardNftTable, chain})
	}
	if nftUsesCompatibilityLayout() {
		for _, family := range nftCompatibilityNatFamilies() {
			commands = append(commands,
				[]string{"flush", "chain", family, portForwardNftTable, portForwardPreroutingChain},
				[]string{"flush", "chain", family, portForwardNftTable, portForwardPostroutingChain},
			)
		}
		return commands
	}
	commands = append(commands,
		[]string{"flush", "chain", nftFamily, portForwardNftTable, portForwardPreroutingChain},
		[]string{"flush", "chain", nftFamily, portForwardNftTable, portForwardPostroutingChain},
	)
	return commands
}

func buildManagedPortForwardRuleCommands(row model.PortForwardRule) (portForwardLimitRuntime, [][]string) {
	return buildManagedPortForwardRuleCommandsWithTrafficBlock(row, "")
}

func buildManagedPortForwardRuleCommandsWithTrafficBlock(row model.PortForwardRule, trafficBlockReason string) (portForwardLimitRuntime, [][]string) {
	return buildManagedPortForwardRuleCommandsForMeterSupport(
		row,
		trafficBlockReason,
		GetNftablesCapabilities().SupportsMeters,
		nil,
	)
}

func buildManagedPortForwardRuleCommandsForMeterSupport(
	row model.PortForwardRule,
	trafficBlockReason string,
	supportsMeters bool,
	meterErr error,
) (portForwardLimitRuntime, [][]string) {
	families := portForwardExpandFamilies(row.Family)
	if len(families) == 0 {
		families = []string{portForwardFamilyIPv4}
	}
	commands := make([][]string, 0, len(families)*5)
	for _, family := range families {
		if nftUsesCompatibilityLayout() {
			if portForwardTargetIsLocal(row.TargetIP) {
				commands = append(commands, buildCompatibilityLocalPortForwardRuleCommands(row, family, trafficBlockReason, supportsMeters)...)
			} else {
				commands = append(commands, buildCompatibilityRemotePortForwardRuleCommands(row, family, trafficBlockReason, supportsMeters)...)
			}
			continue
		}
		if portForwardTargetIsLocal(row.TargetIP) {
			commands = append(commands, buildNativeLocalPortForwardRuleCommands(row, family, trafficBlockReason, supportsMeters)...)
		} else {
			commands = append(commands, buildNativeRemotePortForwardRuleCommands(row, family, trafficBlockReason, supportsMeters)...)
		}
	}
	return portForwardBatchLimitStateForMeterSupport(row, supportsMeters, meterErr), commands
}

func buildNativeRemotePortForwardRuleCommands(row model.PortForwardRule, family string, trafficBlockReason string, supportsMeters bool) [][]string {
	upCounter := portForwardCounterName(row.Id, "up")
	downCounter := portForwardCounterName(row.Id, "down")
	dnat := []string{"add", "rule", nftFamily, portForwardNftTable, portForwardPreroutingChain, "meta", "nfproto", mapFirewallTargetFamily(family)}
	dnat = appendPortForwardProtocolMatch(dnat, row.Protocol)
	dnat = appendNftTransportPortMatch(dnat, "dport")
	dnat = append(dnat, buildNftPortSetArgs(row.LocalPortSpec)...)
	dnat = append(dnat, "counter", "dnat", "to", portForwardNatTargetValue(row.TargetIP, row.TargetPort), "comment", portForwardRuleComment(row.Id, "dnat"))

	masquerade := []string{"add", "rule", nftFamily, portForwardNftTable, portForwardPostroutingChain, "meta", "nfproto", mapFirewallTargetFamily(family)}
	masquerade = appendPortForwardProtocolMatch(masquerade, row.Protocol)
	masquerade = append(masquerade, "ct", "status", "dnat", "ct", "direction", "original", "ct", "original", "proto-dst")
	masquerade = append(masquerade, buildNftPortSetArgs(row.LocalPortSpec)...)
	masquerade = append(masquerade, "counter", "masquerade", "comment", portForwardRuleComment(row.Id, "snat"))

	tracked := buildPortForwardTrackedArgs(row, family)
	down := append([]string{"add", "rule", nftFamily, portForwardNftTable, portForwardForwardChain}, tracked...)
	down = append(down, "ct", "direction", "original", "counter", "name", downCounter, "comment", portForwardRuleComment(row.Id, "down"))
	up := append([]string{"add", "rule", nftFamily, portForwardNftTable, portForwardForwardChain}, tracked...)
	up = append(up, "ct", "direction", "reply", "counter", "name", upCounter, "comment", portForwardRuleComment(row.Id, "up"))

	commands := [][]string{dnat, masquerade}
	if block := buildPortForwardTrafficBlockCommand(row, portForwardForwardChain, tracked, trafficBlockReason); block != nil {
		commands = append(commands, block)
	}
	commands = append(commands, down, up)
	if limit := buildPortForwardMeterLimitCommandForSupport(row, family, portForwardForwardChain, tracked, supportsMeters); limit != nil {
		commands = append(commands, limit)
	}
	return commands
}

func buildNativeLocalPortForwardRuleCommands(row model.PortForwardRule, family string, trafficBlockReason string, supportsMeters bool) [][]string {
	upCounter := portForwardCounterName(row.Id, "up")
	downCounter := portForwardCounterName(row.Id, "down")
	redirect := []string{"add", "rule", nftFamily, portForwardNftTable, portForwardPreroutingChain, "meta", "nfproto", mapFirewallTargetFamily(family)}
	redirect = appendPortForwardProtocolMatch(redirect, row.Protocol)
	redirect = appendNftTransportPortMatch(redirect, "dport")
	redirect = append(redirect, buildNftPortSetArgs(row.LocalPortSpec)...)
	redirect = append(redirect, "counter", "redirect", "to", fmt.Sprintf(":%d", row.TargetPort), "comment", portForwardRuleComment(row.Id, "dnat"))

	tracked := buildPortForwardTrackedArgs(row, family)
	down := append([]string{"add", "rule", nftFamily, portForwardNftTable, portForwardInputChain}, tracked...)
	down = append(down, "ct", "direction", "original", "counter", "name", downCounter, "comment", portForwardRuleComment(row.Id, "down"))
	up := append([]string{"add", "rule", nftFamily, portForwardNftTable, portForwardOutputChain}, tracked...)
	up = append(up, "ct", "direction", "reply", "counter", "name", upCounter, "comment", portForwardRuleComment(row.Id, "up"))

	commands := [][]string{redirect}
	if block := buildPortForwardTrafficBlockCommand(row, portForwardInputChain, tracked, trafficBlockReason); block != nil {
		commands = append(commands, block)
	}
	commands = append(commands, down, up)
	if limit := buildPortForwardMeterLimitCommandForSupport(row, family, portForwardInputChain, tracked, supportsMeters); limit != nil {
		commands = append(commands, limit)
	}
	return commands
}

func buildCompatibilityRemotePortForwardRuleCommands(row model.PortForwardRule, family string, trafficBlockReason string, supportsMeters bool) [][]string {
	natFamily := portForwardNatTableFamily(family)
	dnat := []string{"add", "rule", natFamily, portForwardNftTable, portForwardPreroutingChain}
	dnat = appendPortForwardProtocolMatch(dnat, row.Protocol)
	dnat = appendNftTransportPortMatch(dnat, "dport")
	dnat = append(dnat, buildNftPortSetArgs(row.LocalPortSpec)...)
	dnat = append(dnat, "counter", "dnat", "to", portForwardNatTargetValue(row.TargetIP, row.TargetPort), "comment", portForwardRuleComment(row.Id, "dnat"))

	masquerade := []string{"add", "rule", natFamily, portForwardNftTable, portForwardPostroutingChain}
	masquerade = appendPortForwardProtocolMatch(masquerade, row.Protocol)
	masquerade = append(masquerade, "ct", "status", "dnat", "ct", "direction", "original", "ct", "original", "proto-dst")
	masquerade = append(masquerade, buildNftPortSetArgs(row.LocalPortSpec)...)
	masquerade = append(masquerade, "counter", "masquerade", "comment", portForwardRuleComment(row.Id, "snat"))

	tracked := buildPortForwardTrackedArgs(row, family)
	down := append([]string{"add", "rule", nftFamily, portForwardNftTable, portForwardForwardChain}, tracked...)
	down = append(down, "ct", "direction", "original", "counter", "comment", portForwardRuleComment(row.Id, "down"))
	up := append([]string{"add", "rule", nftFamily, portForwardNftTable, portForwardForwardChain}, tracked...)
	up = append(up, "ct", "direction", "reply", "counter", "comment", portForwardRuleComment(row.Id, "up"))

	commands := [][]string{dnat, masquerade}
	if block := buildPortForwardTrafficBlockCommand(row, portForwardForwardChain, tracked, trafficBlockReason); block != nil {
		commands = append(commands, block)
	}
	commands = append(commands, down, up)
	if limit := buildPortForwardMeterLimitCommandForSupport(row, family, portForwardForwardChain, tracked, supportsMeters); limit != nil {
		commands = append(commands, limit)
	}
	return commands
}

func buildCompatibilityLocalPortForwardRuleCommands(row model.PortForwardRule, family string, trafficBlockReason string, supportsMeters bool) [][]string {
	natFamily := portForwardNatTableFamily(family)
	redirect := []string{"add", "rule", natFamily, portForwardNftTable, portForwardPreroutingChain}
	redirect = appendPortForwardProtocolMatch(redirect, row.Protocol)
	redirect = appendNftTransportPortMatch(redirect, "dport")
	redirect = append(redirect, buildNftPortSetArgs(row.LocalPortSpec)...)
	redirect = append(redirect, "counter", "redirect", "to", fmt.Sprintf(":%d", row.TargetPort), "comment", portForwardRuleComment(row.Id, "dnat"))

	tracked := buildPortForwardTrackedArgs(row, family)
	down := append([]string{"add", "rule", nftFamily, portForwardNftTable, portForwardInputChain}, tracked...)
	down = append(down, "ct", "direction", "original", "counter", "comment", portForwardRuleComment(row.Id, "down"))
	up := append([]string{"add", "rule", nftFamily, portForwardNftTable, portForwardOutputChain}, tracked...)
	up = append(up, "ct", "direction", "reply", "counter", "comment", portForwardRuleComment(row.Id, "up"))

	commands := [][]string{redirect}
	if block := buildPortForwardTrafficBlockCommand(row, portForwardInputChain, tracked, trafficBlockReason); block != nil {
		commands = append(commands, block)
	}
	commands = append(commands, down, up)
	if limit := buildPortForwardMeterLimitCommandForSupport(row, family, portForwardInputChain, tracked, supportsMeters); limit != nil {
		commands = append(commands, limit)
	}
	return commands
}

// buildPortForwardTrafficBlockCommand is deliberately placed before the
// per-rule counters in the filter chain. Once a quota is exhausted, only the
// original packet direction at this rule's local entry is dropped; return
// traffic and unrelated forwarding rules retain their existing behavior.
func buildPortForwardTrafficBlockCommand(row model.PortForwardRule, chain string, tracked []string, trafficBlockReason string) []string {
	if strings.TrimSpace(trafficBlockReason) == "" {
		return nil
	}
	args := append([]string{"add", "rule", nftFamily, portForwardNftTable, chain}, tracked...)
	args = append(args,
		"ct", "direction", "original",
		"drop",
		"comment", portForwardTrafficBlockComment(row.Id),
	)
	return args
}

func portForwardTrafficBlockComment(ruleID uint) string {
	return portForwardRuleComment(ruleID, "traffic_block")
}

func buildPortForwardMeterLimitCommand(row model.PortForwardRule, family string, chain string, tracked []string) []string {
	return buildPortForwardMeterLimitCommandForSupport(
		row,
		family,
		chain,
		tracked,
		GetNftablesCapabilities().SupportsMeters,
	)
}

func buildPortForwardMeterLimitCommandForSupport(
	row model.PortForwardRule,
	family string,
	chain string,
	tracked []string,
	supportsMeters bool,
) []string {
	if row.RateLimitMbps <= 0 || !supportsMeters {
		return nil
	}
	bytesPerSecond := int64(row.RateLimitMbps) * 125000
	args := append([]string{"add", "rule", nftFamily, portForwardNftTable, chain}, tracked...)
	args = append(args,
		"ct", "direction", "original",
		"meter", portForwardMeterName(row.Id, family), "{",
		"meta", "l4proto", ".", "ct", "original", "proto-dst",
		"limit", "rate", "over", strconv.FormatInt(bytesPerSecond, 10), "bytes/second",
		"}", "counter", "drop", "comment", portForwardRuleComment(row.Id, "limit"),
	)
	return args
}

func portForwardBatchLimitState(row model.PortForwardRule) portForwardLimitRuntime {
	capabilities := GetNftablesCapabilities()
	var meterErr error
	if strings.TrimSpace(capabilities.MeterProbeError) != "" {
		meterErr = fmt.Errorf("%s", strings.TrimSpace(capabilities.MeterProbeError))
	}
	return portForwardBatchLimitStateForMeterSupport(row, capabilities.SupportsMeters, meterErr)
}

func portForwardBatchLimitStateForMeterSupport(row model.PortForwardRule, supportsMeters bool, meterErr error) portForwardLimitRuntime {
	if row.RateLimitMbps <= 0 {
		return portForwardLimitRuntime{status: "disabled"}
	}
	if !supportsMeters {
		reason := "当前 nftables/内核不支持 meter"
		if meterErr != nil && strings.TrimSpace(meterErr.Error()) != "" {
			reason = "meter 规则被 nft 拒绝: " + strings.TrimSpace(meterErr.Error())
		}
		name := strings.TrimSpace(row.Name)
		if name == "" {
			name = strconv.FormatUint(uint64(row.Id), 10)
		}
		return portForwardLimitRuntime{
			status:                 "degraded",
			effectiveRateLimitMbps: 0,
			warning: fmt.Sprintf(
				"规则 %s 的限速未生效：%s，转发保持可用",
				name,
				reason,
			),
		}
	}
	return portForwardLimitRuntime{
		status:                 "applied",
		effectiveRateLimitMbps: row.RateLimitMbps,
	}
}

func portForwardMeterName(ruleID uint, family string) string {
	return fmt.Sprintf("kwor_pf_meter_%d_%s", ruleID, strings.ToLower(strings.TrimSpace(family)))
}

func portForwardNftScriptLine(args []string) string {
	parts := make([]string, 0, len(args))
	quoteNext := false
	for _, arg := range args {
		if quoteNext {
			parts = append(parts, strconv.Quote(arg))
			quoteNext = false
			continue
		}
		parts = append(parts, arg)
		if arg == "comment" {
			quoteNext = true
		}
	}
	return strings.Join(parts, " ")
}
