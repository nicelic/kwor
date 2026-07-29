package service

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/alireza0/s-ui/database/model"
)

type portForwardNftTableSnapshot struct {
	known         bool
	exists        bool
	chains        map[string]bool
	commentCounts map[string]map[string]int
	counters      map[string]int64
}

type portForwardNftSnapshot struct {
	tables map[string]portForwardNftTableSnapshot
}

var portForwardNftChainLineRe = regexp.MustCompile(`^\s*chain\s+([A-Za-z0-9_][A-Za-z0-9_-]*)\s*\{`)

// loadPortForwardNftSnapshot reads every owned nft table at most once. The
// resulting text drives integrity, comment cardinality and counters together.
func loadPortForwardNftSnapshot() (portForwardNftSnapshot, error) {
	snapshot := portForwardNftSnapshot{tables: make(map[string]portForwardNftTableSnapshot)}
	for _, family := range append([]string{nftFamily}, nftCompatibilityNatFamilies()...) {
		out, err := runNft("list", "table", family, portForwardNftTable)
		if err != nil {
			if portForwardNftObjectMissing(err) {
				snapshot.tables[family] = portForwardNftTableSnapshot{
					known:         true,
					exists:        false,
					chains:        map[string]bool{},
					commentCounts: map[string]map[string]int{},
					counters:      map[string]int64{},
				}
				continue
			}
			return snapshot, err
		}
		snapshot.tables[family] = parsePortForwardNftTableSnapshot(out)
	}
	return snapshot, nil
}

func parsePortForwardNftTableSnapshot(out []byte) portForwardNftTableSnapshot {
	result := portForwardNftTableSnapshot{
		known:         true,
		exists:        true,
		chains:        make(map[string]bool),
		commentCounts: make(map[string]map[string]int),
		counters:      make(map[string]int64),
	}
	for _, match := range portForwardCounterBlockRe.FindAllStringSubmatch(string(out), -1) {
		if len(match) != 4 {
			continue
		}
		if value, err := strconv.ParseInt(match[3], 10, 64); err == nil {
			result.counters[match[1]] = value
		}
	}

	currentChain := ""
	chainDepth := 0
	for _, line := range strings.Split(string(out), "\n") {
		if match := portForwardNftChainLineRe.FindStringSubmatch(line); len(match) == 2 {
			currentChain = match[1]
			chainDepth = portForwardNftBraceDelta(line)
			if chainDepth < 1 {
				chainDepth = 1
			}
			result.chains[currentChain] = true
			if _, exists := result.commentCounts[currentChain]; !exists {
				result.commentCounts[currentChain] = make(map[string]int)
			}
			continue
		}
		if currentChain == "" {
			continue
		}
		comment, ok := extractRuleComment(line)
		if ok {
			result.commentCounts[currentChain][comment]++
			match := portForwardInlineCounterCommentRe.FindStringSubmatch(comment)
			if len(match) == 3 {
				counterMatch := nftCounterBytesRe.FindStringSubmatch(line)
				if len(counterMatch) == 2 {
					ruleID, ruleErr := strconv.ParseUint(match[1], 10, 64)
					bytes, bytesErr := strconv.ParseInt(counterMatch[1], 10, 64)
					if ruleErr == nil && bytesErr == nil && ruleID > 0 {
						counterName := portForwardCounterName(uint(ruleID), match[2])
						result.counters[counterName] += bytes
					}
				}
			}
		}
		chainDepth += portForwardNftBraceDelta(line)
		if chainDepth <= 0 {
			currentChain = ""
			chainDepth = 0
		}
	}
	return result
}

func portForwardNftBraceDelta(line string) int {
	return strings.Count(line, "{") - strings.Count(line, "}")
}

func (s portForwardNftSnapshot) table(family string) (portForwardNftTableSnapshot, bool) {
	value, exists := s.tables[family]
	return value, exists && value.known
}

func (s portForwardNftSnapshot) commentCount(family string, chain string, comment string) (int, bool) {
	table, known := s.table(family)
	if !known || !table.exists || !table.chains[chain] {
		return 0, false
	}
	return table.commentCounts[chain][comment], true
}

func (s portForwardNftSnapshot) renderIntact(activeRows []model.PortForwardRule, limitStates map[uint]portForwardLimitStateView) bool {
	return s.renderIntactWithTrafficBlocks(activeRows, limitStates, nil)
}

func (s portForwardNftSnapshot) renderIntactWithTrafficBlocks(activeRows []model.PortForwardRule, limitStates map[uint]portForwardLimitStateView, trafficBlocks map[uint]string) bool {
	if len(activeRows) == 0 {
		for _, family := range append([]string{nftFamily}, nftCompatibilityNatFamilies()...) {
			table, known := s.table(family)
			if !known || table.exists {
				return false
			}
		}
		return true
	}
	if !s.baseIntact() {
		return false
	}
	for _, row := range activeRows {
		families := portForwardExpandFamilies(row.Family)
		if len(families) == 0 {
			families = []string{portForwardFamilyIPv4}
		}
		if !s.natRuleIntact(row, "dnat", portForwardPreroutingChain, families) {
			return false
		}
		if !s.natRuleIntact(row, "snat", portForwardPostroutingChain, families) {
			return false
		}
		if !s.filterCountersIntact(row, len(families), limitStates[row.Id], strings.TrimSpace(trafficBlocks[row.Id]) != "") {
			return false
		}
		if !nftUsesCompatibilityLayout() {
			table, _ := s.table(nftFamily)
			if _, ok := table.counters[portForwardCounterName(row.Id, "up")]; !ok {
				return false
			}
			if _, ok := table.counters[portForwardCounterName(row.Id, "down")]; !ok {
				return false
			}
		}
	}
	return true
}

func (s portForwardNftSnapshot) baseIntact() bool {
	inet, known := s.table(nftFamily)
	if !known || !inet.exists {
		return false
	}
	for _, chain := range []string{portForwardForwardChain, portForwardInputChain, portForwardOutputChain} {
		if !inet.chains[chain] {
			return false
		}
	}
	if nftUsesCompatibilityLayout() {
		if inet.chains[portForwardPreroutingChain] || inet.chains[portForwardPostroutingChain] {
			return false
		}
		for _, family := range nftCompatibilityNatFamilies() {
			table, tableKnown := s.table(family)
			if !tableKnown || !table.exists || !table.chains[portForwardPreroutingChain] || !table.chains[portForwardPostroutingChain] {
				return false
			}
		}
		return true
	}
	if !inet.chains[portForwardPreroutingChain] || !inet.chains[portForwardPostroutingChain] {
		return false
	}
	for _, family := range nftCompatibilityNatFamilies() {
		table, tableKnown := s.table(family)
		if !tableKnown || table.exists {
			return false
		}
	}
	return true
}

func (s portForwardNftSnapshot) natRuleIntact(row model.PortForwardRule, suffix string, expectedChain string, families []string) bool {
	comment := portForwardRuleComment(row.Id, suffix)
	expectedCounts := make(map[string]int)
	if suffix == "dnat" || (suffix == "snat" && !portForwardTargetIsLocal(row.TargetIP)) {
		if nftUsesCompatibilityLayout() {
			for _, family := range families {
				expectedCounts[portForwardRuleLocationKey(portForwardNatTableFamily(family), expectedChain)]++
			}
		} else {
			expectedCounts[portForwardRuleLocationKey(nftFamily, expectedChain)] = len(families)
		}
	}
	for _, family := range append([]string{nftFamily}, nftCompatibilityNatFamilies()...) {
		for _, chain := range []string{portForwardPreroutingChain, portForwardPostroutingChain} {
			count, known := s.commentCount(family, chain, comment)
			if !known || count != expectedCounts[portForwardRuleLocationKey(family, chain)] {
				return false
			}
		}
	}
	return true
}

func (s portForwardNftSnapshot) filterCountersIntact(row model.PortForwardRule, expected int, limitState portForwardLimitStateView, trafficBlocked bool) bool {
	downChain := portForwardForwardChain
	upChain := portForwardForwardChain
	if portForwardTargetIsLocal(row.TargetIP) {
		downChain = portForwardInputChain
		upChain = portForwardOutputChain
	}
	if !s.filterCommentIntact(portForwardRuleComment(row.Id, "down"), downChain, expected) ||
		!s.filterCommentIntact(portForwardRuleComment(row.Id, "up"), upChain, expected) {
		return false
	}
	limitExpected := 0
	if row.RateLimitMbps > 0 && limitState.Status != "degraded" {
		limitExpected = expected
	}
	if !s.filterCommentIntact(portForwardRuleComment(row.Id, "limit"), downChain, limitExpected) {
		return false
	}
	blockExpected := 0
	if trafficBlocked {
		blockExpected = expected
	}
	return s.filterCommentIntact(portForwardTrafficBlockComment(row.Id), downChain, blockExpected)
}

func (s portForwardNftSnapshot) filterCommentIntact(comment string, expectedChain string, expected int) bool {
	for _, chain := range []string{portForwardForwardChain, portForwardInputChain, portForwardOutputChain} {
		wanted := 0
		if chain == expectedChain {
			wanted = expected
		}
		count, known := s.commentCount(nftFamily, chain, comment)
		if !known || count != wanted {
			return false
		}
	}
	return true
}

func readPortForwardCounterBytesFromSnapshot(snapshot portForwardNftSnapshot) map[string]int64 {
	result := make(map[string]int64)
	if table, known := snapshot.table(nftFamily); known && table.exists {
		for name, value := range table.counters {
			result[name] += value
		}
	}
	return result
}
