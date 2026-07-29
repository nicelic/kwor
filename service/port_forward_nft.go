package service

import (
	"fmt"
	"net/netip"
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
	if err := ensureNftRendererSupported(); err != nil {
		return err
	}
	if nftUsesCompatibilityLayout() {
		return ensureCompatibilityPortForwardBase()
	}
	return ensureNativePortForwardBase()
}

func ensureNativePortForwardBase() error {
	if err := cleanupCompatibilityPortForwardNatTables(); err != nil {
		return err
	}

	chains := []struct {
		name string
		spec []string
	}{
		{
			name: portForwardPreroutingChain,
			spec: nftPreroutingNatChainSpec(),
		},
		{
			name: portForwardPostroutingChain,
			spec: nftPostroutingNatChainSpec(),
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
	resources := make([]portForwardBaseChainSpec, 0, len(chains))
	for _, chain := range chains {
		resources = append(resources, portForwardBaseChainSpec{tableFamily: nftFamily, name: chain.name, spec: chain.spec})
	}
	return ensurePortForwardBaseResources(resources)
}

func ensureCompatibilityPortForwardBase() error {
	if err := removeNativePortForwardNatChains(); err != nil {
		return err
	}

	filterChains := []struct {
		name string
		spec []string
	}{
		{name: portForwardForwardChain, spec: []string{"type", "filter", "hook", "forward", "priority", "0", ";", "policy", "accept", ";"}},
		{name: portForwardInputChain, spec: []string{"type", "filter", "hook", "input", "priority", "0", ";", "policy", "accept", ";"}},
		{name: portForwardOutputChain, spec: []string{"type", "filter", "hook", "output", "priority", "0", ";", "policy", "accept", ";"}},
	}
	resources := make([]portForwardBaseChainSpec, 0, len(filterChains)+len(nftCompatibilityNatFamilies())*2)
	for _, chain := range filterChains {
		resources = append(resources, portForwardBaseChainSpec{tableFamily: nftFamily, name: chain.name, spec: chain.spec})
	}

	for _, tableFamily := range nftCompatibilityNatFamilies() {
		resources = append(resources, portForwardBaseChainSpec{
			tableFamily: tableFamily,
			name:        portForwardPreroutingChain,
			spec:        nftPreroutingNatChainSpec(),
		})
		// Create both NAT hooks for all compatibility tables. Linux < 4.18
		// needs the pair for reply-path NAT; newer kernels accept it too.
		resources = append(resources, portForwardBaseChainSpec{
			tableFamily: tableFamily,
			name:        portForwardPostroutingChain,
			spec:        nftPostroutingNatChainSpec(),
		})
	}
	return ensurePortForwardBaseResources(resources)
}

type portForwardBaseChainSpec struct {
	tableFamily string
	name        string
	spec        []string
}

type portForwardCreatedChain struct {
	tableFamily string
	name        string
}

// ensurePortForwardBaseResources creates a complete base or rolls back every
// resource created by this call. This avoids leaving an IPv4-only compatibility
// scaffold when the corresponding ip6 table or chain is rejected.
func ensurePortForwardBaseResources(resources []portForwardBaseChainSpec) error {
	createdTables := make(map[string]struct{})
	createdTableOrder := make([]string, 0, len(resources))
	createdChains := make([]portForwardCreatedChain, 0, len(resources))
	rollback := func() {
		for i := len(createdChains) - 1; i >= 0; i-- {
			chain := createdChains[i]
			if _, createdTable := createdTables[chain.tableFamily]; createdTable {
				continue
			}
			_, _ = runNft("flush", "chain", chain.tableFamily, portForwardNftTable, chain.name)
			_, _ = runNft("delete", "chain", chain.tableFamily, portForwardNftTable, chain.name)
		}
		for i := len(createdTableOrder) - 1; i >= 0; i-- {
			_, _ = runNft("delete", "table", createdTableOrder[i], portForwardNftTable)
		}
	}

	for _, resource := range resources {
		createdTable, err := ensurePortForwardTableCreated(resource.tableFamily)
		if err != nil {
			rollback()
			return err
		}
		if createdTable {
			createdTables[resource.tableFamily] = struct{}{}
			createdTableOrder = append(createdTableOrder, resource.tableFamily)
		}

		createdChain, err := ensurePortForwardChainCreated(resource.tableFamily, resource.name, resource.spec)
		if err != nil {
			rollback()
			return err
		}
		if createdChain {
			createdChains = append(createdChains, portForwardCreatedChain{tableFamily: resource.tableFamily, name: resource.name})
		}
	}
	return nil
}

func ensurePortForwardTable(tableFamily string) error {
	_, err := ensurePortForwardTableCreated(tableFamily)
	return err
}

func ensurePortForwardTableCreated(tableFamily string) (bool, error) {
	if exists, err := inspectOwnedNftTableForMutation(tableFamily, portForwardNftTable); err != nil {
		return false, err
	} else if exists {
		return false, nil
	}
	if err := createOwnedNftTable("nft-forward-"+tableFamily, tableFamily, portForwardNftTable); err != nil {
		return false, err
	}
	return true, nil
}

func ensurePortForwardChain(tableFamily string, chainName string, spec []string) error {
	_, err := ensurePortForwardChainCreated(tableFamily, chainName, spec)
	return err
}

func ensurePortForwardChainCreated(tableFamily string, chainName string, spec []string) (bool, error) {
	if _, err := runNft("list", "chain", tableFamily, portForwardNftTable, chainName); err == nil {
		return false, nil
	} else if !portForwardNftObjectMissing(err) {
		return false, err
	}
	args := []string{"add", "chain", tableFamily, portForwardNftTable, chainName, "{"}
	args = append(args, spec...)
	args = append(args, "}")
	if _, err := runNft(args...); err != nil {
		return false, err
	}
	return true, nil
}

func cleanupCompatibilityPortForwardNatTables() error {
	var firstErr error
	for _, tableFamily := range nftCompatibilityNatFamilies() {
		if err := deleteOwnedNftTableForRuntime(tableFamily, portForwardNftTable); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func removeNativePortForwardNatChains() error {
	if exists, err := inspectOwnedNftTableForMutation(nftFamily, portForwardNftTable); err != nil {
		return err
	} else if !exists {
		return nil
	}
	var firstErr error
	for _, chain := range []string{portForwardPreroutingChain, portForwardPostroutingChain} {
		if _, err := runNft("list", "chain", nftFamily, portForwardNftTable, chain); err != nil {
			if !portForwardNftObjectMissing(err) && firstErr == nil {
				firstErr = err
			}
			continue
		}
		if _, err := runNft("flush", "chain", nftFamily, portForwardNftTable, chain); err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if _, err := runNft("delete", "chain", nftFamily, portForwardNftTable, chain); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func flushManagedPortForwardChains() error {
	if !portForwardSupported() {
		return nil
	}

	var firstErr error
	for _, chain := range []string{
		portForwardPreroutingChain,
		portForwardPostroutingChain,
		portForwardForwardChain,
		portForwardInputChain,
		portForwardOutputChain,
	} {
		if err := flushPortForwardChainIfPresent(nftFamily, chain); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	for _, tableFamily := range nftCompatibilityNatFamilies() {
		for _, chain := range []string{portForwardPreroutingChain, portForwardPostroutingChain} {
			if err := flushPortForwardChainIfPresent(tableFamily, chain); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

func flushPortForwardChainIfPresent(tableFamily string, chain string) error {
	if !nftNatTableExists(tableFamily, portForwardNftTable) {
		return nil
	}
	if _, err := runNft("list", "chain", tableFamily, portForwardNftTable, chain); err != nil {
		if portForwardNftObjectMissing(err) {
			return nil
		}
		return err
	}
	_, err := runNft("flush", "chain", tableFamily, portForwardNftTable, chain)
	return err
}

func readKernelForwardingEnabled(filePath string) bool {
	body, err := portForwardKernelReadFileFn(filePath)
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(body)) == "1"
}

func ensureManagedPortForwardNamedCounter(counterName string) error {
	if counterName == "" || nftUsesCompatibilityLayout() {
		return nil
	}
	if _, err := runNft("list", "counter", nftFamily, portForwardNftTable, counterName); err == nil {
		return nil
	}
	_, err := runNft("add", "counter", nftFamily, portForwardNftTable, counterName)
	return err
}

func deletePortForwardNamedCounter(counterName string) error {
	if counterName == "" || nftUsesCompatibilityLayout() || !portForwardSupported() || !portForwardTableExists() {
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
	if nftUsesCompatibilityLayout() {
		return addCompatibilityManagedPortForwardRule(row)
	}
	return addNativeManagedPortForwardRule(row)
}

func addNativeManagedPortForwardRule(row model.PortForwardRule) (portForwardLimitRuntime, error) {
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
	dnatArgs = appendNftTransportPortMatch(dnatArgs, "dport")
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
		state := portForwardBatchLimitState(row)
		if state.status == "degraded" {
			return state, nil
		}
		limitArgs := buildPortForwardMeterLimitCommand(row, family, portForwardForwardChain, trackedArgs)
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
	redirectArgs = appendNftTransportPortMatch(redirectArgs, "dport")
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
		state := portForwardBatchLimitState(row)
		if state.status == "degraded" {
			return state, nil
		}
		limitArgs := buildPortForwardMeterLimitCommand(row, family, portForwardInputChain, trackedArgs)
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

func addCompatibilityManagedPortForwardRule(row model.PortForwardRule) (portForwardLimitRuntime, error) {
	families := portForwardExpandFamilies(row.Family)
	if len(families) == 0 {
		families = []string{portForwardFamilyIPv4}
	}

	warnings := make([]string, 0, len(families))
	for _, family := range families {
		var state portForwardLimitRuntime
		var err error
		if portForwardTargetIsLocal(row.TargetIP) {
			state, err = addCompatibilityLocalPortForwardRuleForFamily(row, family)
		} else {
			state, err = addCompatibilityRemotePortForwardRuleForFamily(row, family)
		}
		if err != nil {
			_ = cleanupCompatibilityPortForwardRule(row)
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
	return portForwardLimitRuntime{status: "disabled"}, nil
}

func portForwardNatTableFamily(family string) string {
	if strings.EqualFold(strings.TrimSpace(family), portForwardFamilyIPv6) {
		return nftNatFamilyIPv6
	}
	return nftNatFamilyIPv4
}

func addCompatibilityRemotePortForwardRuleForFamily(row model.PortForwardRule, family string) (portForwardLimitRuntime, error) {
	natFamily := portForwardNatTableFamily(family)
	dnatArgs := []string{"add", "rule", natFamily, portForwardNftTable, portForwardPreroutingChain}
	dnatArgs = appendPortForwardProtocolMatch(dnatArgs, row.Protocol)
	dnatArgs = appendNftTransportPortMatch(dnatArgs, "dport")
	dnatArgs = append(dnatArgs, buildNftPortSetArgs(row.LocalPortSpec)...)
	dnatArgs = append(dnatArgs,
		"counter",
		"dnat", "to", portForwardNatTargetValue(row.TargetIP, row.TargetPort),
		"comment", portForwardRuleComment(row.Id, "dnat"),
	)
	if _, err := runNft(dnatArgs...); err != nil {
		return portForwardLimitRuntime{}, err
	}

	masqueradeArgs := []string{"add", "rule", natFamily, portForwardNftTable, portForwardPostroutingChain}
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
		"counter",
		"comment", portForwardRuleComment(row.Id, "down"),
	)
	if _, err := runNft(downArgs...); err != nil {
		return portForwardLimitRuntime{}, err
	}

	upArgs := append([]string{"add", "rule", nftFamily, portForwardNftTable, portForwardForwardChain}, trackedArgs...)
	upArgs = append(upArgs,
		"ct", "direction", "reply",
		"counter",
		"comment", portForwardRuleComment(row.Id, "up"),
	)
	if _, err := runNft(upArgs...); err != nil {
		return portForwardLimitRuntime{}, err
	}
	return addCompatibilityPortForwardLimit(row, family, portForwardForwardChain, trackedArgs)
}

func addCompatibilityLocalPortForwardRuleForFamily(row model.PortForwardRule, family string) (portForwardLimitRuntime, error) {
	natFamily := portForwardNatTableFamily(family)
	redirectArgs := []string{"add", "rule", natFamily, portForwardNftTable, portForwardPreroutingChain}
	redirectArgs = appendPortForwardProtocolMatch(redirectArgs, row.Protocol)
	redirectArgs = appendNftTransportPortMatch(redirectArgs, "dport")
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
		"counter",
		"comment", portForwardRuleComment(row.Id, "down"),
	)
	if _, err := runNft(downArgs...); err != nil {
		return portForwardLimitRuntime{}, err
	}

	upArgs := append([]string{"add", "rule", nftFamily, portForwardNftTable, portForwardOutputChain}, trackedArgs...)
	upArgs = append(upArgs,
		"ct", "direction", "reply",
		"counter",
		"comment", portForwardRuleComment(row.Id, "up"),
	)
	if _, err := runNft(upArgs...); err != nil {
		return portForwardLimitRuntime{}, err
	}
	return addCompatibilityPortForwardLimit(row, family, portForwardInputChain, trackedArgs)
}

func addCompatibilityPortForwardLimit(row model.PortForwardRule, family string, chain string, trackedArgs []string) (portForwardLimitRuntime, error) {
	if row.RateLimitMbps <= 0 {
		return portForwardLimitRuntime{status: "disabled"}, nil
	}
	state := portForwardBatchLimitState(row)
	if state.status == "degraded" {
		return state, nil
	}
	limitArgs := buildPortForwardMeterLimitCommand(row, family, chain, trackedArgs)
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

func cleanupCompatibilityPortForwardRule(row model.PortForwardRule) error {
	comments := []string{
		portForwardRuleComment(row.Id, "dnat"),
		portForwardRuleComment(row.Id, "snat"),
		portForwardRuleComment(row.Id, "down"),
		portForwardRuleComment(row.Id, "up"),
		portForwardRuleComment(row.Id, "limit"),
		portForwardTrafficBlockComment(row.Id),
	}
	var firstErr error
	for _, tableFamily := range nftCompatibilityNatFamilies() {
		for _, comment := range comments[:2] {
			for _, chain := range []string{portForwardPreroutingChain, portForwardPostroutingChain} {
				if err := deleteNftRulesByExactComment(tableFamily, portForwardNftTable, chain, comment); err != nil && firstErr == nil {
					firstErr = err
				}
			}
		}
	}
	for _, chain := range []string{portForwardForwardChain, portForwardInputChain, portForwardOutputChain} {
		for _, comment := range comments[2:] {
			if err := deleteNftRulesByExactComment(nftFamily, portForwardNftTable, chain, comment); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
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

func portForwardRenderIntact(activeRows []model.PortForwardRule) bool {
	snapshot, err := loadPortForwardNftSnapshot()
	if err != nil {
		return false
	}
	return snapshot.renderIntact(activeRows, loadPortForwardLimitStateMap())
}

// parseCompatibilityPortForwardCounterBytes remains a pure text parser for
// legacy fixtures and migrations. Runtime counter reads use the shared table
// snapshot above instead of issuing one chain query per forwarding rule.
func parseCompatibilityPortForwardCounterBytes(out []byte) map[string]int64 {
	result := make(map[string]int64)
	for _, line := range strings.Split(string(out), "\n") {
		comment, ok := extractRuleComment(line)
		if !ok {
			continue
		}
		commentMatch := portForwardInlineCounterCommentRe.FindStringSubmatch(comment)
		if len(commentMatch) != 3 {
			continue
		}
		counterMatch := nftCounterBytesRe.FindStringSubmatch(line)
		if len(counterMatch) != 2 {
			continue
		}
		ruleID, ruleIDErr := strconv.ParseUint(commentMatch[1], 10, 64)
		bytes, bytesErr := strconv.ParseInt(counterMatch[1], 10, 64)
		if ruleIDErr != nil || bytesErr != nil || ruleID == 0 {
			continue
		}
		counterName := portForwardCounterName(uint(ruleID), commentMatch[2])
		result[counterName] += bytes
	}
	return result
}

func portForwardRuleLocationKey(tableFamily string, chain string) string {
	return tableFamily + "\x00" + chain
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
