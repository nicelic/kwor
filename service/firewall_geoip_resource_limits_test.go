package service

import (
	"bufio"
	"bytes"
	"fmt"
	"net/netip"
	"strings"
	"testing"

	"github.com/alireza0/s-ui/database/model"
	"go4.org/netipx"
)

func TestParseFirewallGeoCustomSourceURLsLimitsSourceCount(t *testing.T) {
	items := make([]string, 0, firewallGeoMaxSourcesPerRule+1)
	for index := 0; index <= firewallGeoMaxSourcesPerRule; index++ {
		items = append(items, "https://example.com/rules-"+string(rune('a'+index))+".txt")
	}

	if _, err := parseFirewallGeoCustomSourceURLs(strings.Join(items, "\n")); err == nil {
		t.Fatalf("expected custom source count limit error")
	}
}

func TestParseFirewallGeoCustomSourceURLsLimitsInputBytes(t *testing.T) {
	value := "https://example.com/" + strings.Repeat("a", firewallGeoMaxCustomSourceURLsBytes)
	if _, err := parseFirewallGeoCustomSourceURLs(value); err == nil {
		t.Fatalf("expected custom source input byte limit error")
	}
}

func TestDecodeFirewallGeoStringListRejectsOversizedStoredValue(t *testing.T) {
	value := `["]` + strings.Repeat("a", firewallGeoMaxStoredListBytes) + `"]`
	if result := decodeFirewallGeoStringList(value); len(result) != 0 {
		t.Fatalf("oversized stored list must be rejected: %#v", result)
	}
}

func TestFirewallGeoCachedFileNameMustStayInGeoIPDirectory(t *testing.T) {
	for _, value := range []string{"rule_1_00.txt", "rule_2_01.srs"} {
		if !isSafeFirewallGeoCachedFileName(value) {
			t.Fatalf("safe cache name rejected: %q", value)
		}
	}
	for _, value := range []string{"../outside.txt", "nested/file.txt", `nested\\file.txt`, "..", ""} {
		if isSafeFirewallGeoCachedFileName(value) {
			t.Fatalf("unsafe cache name accepted: %q", value)
		}
	}
}

func TestFirewallGeoResolvedPrefixesDoNotKeepDuplicateAllSlice(t *testing.T) {
	resolved, err := parseFirewallGeoTXT([]byte("192.0.2.1\n2001:db8::1\n"))
	if err != nil {
		t.Fatalf("parse geo text: %v", err)
	}
	if resolved.PrefixCount != len(resolved.IPv4)+len(resolved.IPv6) {
		t.Fatalf("prefix count=%d ipv4=%d ipv6=%d", resolved.PrefixCount, len(resolved.IPv4), len(resolved.IPv6))
	}
}

func TestFirewallGeoParserBudgetRejectsCumulativeIPSetRanges(t *testing.T) {
	budget := &firewallGeoParserBudget{ipSetRanges: firewallGeoMaxTotalIPSetRanges - 1}
	if err := budget.addIPSetRanges(1); err != nil {
		t.Fatalf("budget should accept final allowed range: %v", err)
	}
	if err := budget.addIPSetRanges(1); err == nil {
		t.Fatal("expected cumulative ip range limit error")
	}
}

func TestFirewallGeoParserBudgetRejectsRangeExpansionBeforeBuildingIPSet(t *testing.T) {
	var builder netipx.IPSetBuilder
	budget := &firewallGeoParserBudget{candidatePrefixes: firewallGeoMaxPrefixCount - 1}
	if err := budget.addIPRange(&builder, netip.MustParseAddr("192.0.2.1"), netip.MustParseAddr("192.0.2.2")); err == nil {
		t.Fatal("expected ip range expansion budget error")
	}
}

func TestFirewallGeoParserBudgetRejectsDeepJSONRules(t *testing.T) {
	var builder netipx.IPSetBuilder
	rule := map[string]any{"ip_cidr": "192.0.2.1"}
	for index := 0; index <= firewallGeoMaxJSONDepth; index++ {
		rule = map[string]any{"rules": []any{rule}}
	}
	if err := parseFirewallGeoJSONRule(rule, &builder, &firewallGeoParserBudget{}, 0); err == nil {
		t.Fatal("expected json nesting limit error")
	}
}

func TestEnsureFirewallGeoRuntimeLoadedRejectsDeclaredTotalBeforeReadingFiles(t *testing.T) {
	rows := []model.FirewallGeoRule{
		{Id: 1, PrefixCount: firewallGeoMaxRuntimePrefixCount},
		{Id: 2, PrefixCount: 1},
	}
	if err := ensureFirewallGeoRuntimeLoadedLocked(rows); err == nil {
		t.Fatal("expected declared total prefix limit error")
	}
}

func TestObservedFirewallRuleDropsOversizedComment(t *testing.T) {
	comment := strings.Repeat("x", firewallMaxRuleDescriptionBytes+1)
	line := fmt.Sprintf("tcp dport 443 accept comment %q # handle 1", comment)
	rule, ok := parseObservedFirewallRuleLine(line, "ip", "filter", "input", 1)
	if !ok {
		t.Fatal("expected observed firewall rule")
	}
	if rule.Comment != "" {
		t.Fatalf("oversized comment must not reach storage: %d bytes", len(rule.Comment))
	}
	if rule.Description != "导入规则 filter/input#1" {
		t.Fatalf("unexpected fallback description: %q", rule.Description)
	}
}

func TestReadFirewallGeoSRSAddrRejectsUnexpectedLengthBeforeAllocation(t *testing.T) {
	reader := bufio.NewReader(bytes.NewReader([]byte{5}))
	if _, err := readFirewallGeoSRSAddr(reader); err == nil {
		t.Fatalf("expected unsupported address length error")
	}
}

func TestParseFirewallGeoJSONRejectsDuplicateRulesArray(t *testing.T) {
	if _, err := parseFirewallGeoJSON([]byte(`{"rules":[],"rules":[]}`)); err == nil {
		t.Fatalf("expected duplicate rules array error")
	}
}
