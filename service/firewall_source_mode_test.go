package service

import (
	"strings"
	"testing"

	"github.com/alireza0/s-ui/database/model"
)

func TestNormalizeFirewallSourceMode(t *testing.T) {
	cases := map[string]string{
		"":        "",
		" allow ": firewallSourceModeAllow,
		"BLOCK":   firewallSourceModeBlock,
	}
	for raw, expected := range cases {
		got, err := normalizeFirewallSourceMode(raw)
		if err != nil {
			t.Fatalf("normalizeFirewallSourceMode(%q) returned error: %v", raw, err)
		}
		if got != expected {
			t.Fatalf("normalizeFirewallSourceMode(%q)=%q, want %q", raw, got, expected)
		}
	}
	if _, err := normalizeFirewallSourceMode("deny"); err == nil {
		t.Fatal("unsupported source mode should fail")
	}
}

func TestEffectiveFirewallSourceModePreservesLegacySourceAllowlist(t *testing.T) {
	row := model.FirewallRule{
		Origin:     firewallOriginManual,
		SourceSpec: "1.1.1.1/32",
	}
	if got := effectiveFirewallSourceMode(row); got != firewallSourceModeAllow {
		t.Fatalf("legacy source specification mode=%q, want %q", got, firewallSourceModeAllow)
	}

	external := row
	external.Origin = firewallOriginExternal
	if got := effectiveFirewallSourceMode(external); got != "" {
		t.Fatalf("external observed source mode=%q, want empty", got)
	}
}

func TestBuildManagedFirewallSourceRulesScriptBlockAllowsOtherSources(t *testing.T) {
	row := model.FirewallRule{
		Id:         7,
		Enabled:    true,
		Origin:     firewallOriginManual,
		Family:     firewallFamilyIPv4,
		Protocol:   firewallProtocolTCP,
		PortSpec:   "22",
		SourceSpec: "1.1.1.1/32",
		SourceMode: firewallSourceModeBlock,
	}
	var script strings.Builder
	plan, err := appendManagedFirewallSourceRulesScript(&script, []model.FirewallRule{row})
	if err != nil {
		t.Fatalf("appendManagedFirewallSourceRulesScript returned error: %v", err)
	}
	if err := appendManagedFirewallSourceBlockFallbacksScript(&script, plan, nil); err != nil {
		t.Fatalf("appendManagedFirewallSourceBlockFallbacksScript returned error: %v", err)
	}
	text := script.String()
	blockIndex := strings.Index(text, "ip saddr 1.1.1.1/32 counter drop")
	fallbackIndex := strings.Index(text, "kwor_firewall_rule_7_ipv4_source_block_fallback")
	if blockIndex < 0 || fallbackIndex < 0 {
		t.Fatalf("block script missing expected rules: %s", text)
	}
	if blockIndex > fallbackIndex {
		t.Fatalf("block rule must precede fallback accept: %s", text)
	}
}

func TestBuildManagedFirewallSourceRulesScriptAllowDropsOtherSources(t *testing.T) {
	row := model.FirewallRule{
		Id:         8,
		Enabled:    true,
		Origin:     firewallOriginManual,
		Family:     firewallFamilyIPv4,
		Protocol:   firewallProtocolTCP,
		PortSpec:   "22",
		SourceSpec: "1.1.1.1/32",
		SourceMode: firewallSourceModeAllow,
	}
	var script strings.Builder
	if _, err := appendManagedFirewallSourceRulesScript(&script, []model.FirewallRule{row}); err != nil {
		t.Fatalf("appendManagedFirewallSourceRulesScript returned error: %v", err)
	}
	text := script.String()
	allowIndex := strings.Index(text, "ip saddr 1.1.1.1/32 counter accept")
	fallbackIndex := strings.Index(text, "kwor_firewall_rule_8_ipv4_source_allow_fallback")
	if allowIndex < 0 || fallbackIndex < 0 {
		t.Fatalf("allow script missing expected rules: %s", text)
	}
	if allowIndex > fallbackIndex {
		t.Fatalf("allow rule must precede fallback drop: %s", text)
	}
}

func TestBuildManagedFirewallSourceRulesScriptDualModeCoversMissingFamily(t *testing.T) {
	row := model.FirewallRule{
		Id:         9,
		Enabled:    true,
		Origin:     firewallOriginManual,
		Family:     firewallFamilyDual,
		Protocol:   firewallProtocolTCP,
		PortSpec:   "22",
		SourceSpec: "1.1.1.1/32",
		SourceMode: firewallSourceModeAllow,
	}
	var script strings.Builder
	if _, err := appendManagedFirewallSourceRulesScript(&script, []model.FirewallRule{row}); err != nil {
		t.Fatalf("appendManagedFirewallSourceRulesScript returned error: %v", err)
	}
	text := script.String()
	if !strings.Contains(text, "meta nfproto ipv6 meta l4proto tcp") || !strings.Contains(text, "kwor_firewall_rule_9_ipv6_source_allow_fallback") {
		t.Fatalf("dual-stack allow mode must drop unspecified IPv6 sources: %s", text)
	}
	if strings.Contains(text, "meta nfproto ipv6 ip6 saddr") {
		t.Fatalf("IPv6 source accept should not be generated without IPv6 sources: %s", text)
	}
}

func TestFirewallSourceBlockFallbackDefersToGeoAllow(t *testing.T) {
	row := model.FirewallRule{
		Id:         10,
		Enabled:    true,
		Origin:     firewallOriginManual,
		Family:     firewallFamilyIPv4,
		Protocol:   firewallProtocolTCP,
		PortSpec:   "443",
		SourceSpec: "1.1.1.1/32",
		SourceMode: firewallSourceModeBlock,
	}
	geoAllow := model.FirewallGeoRule{
		Enabled:  true,
		Family:   firewallFamilyIPv4,
		Protocol: firewallProtocolTCP,
		PortSpec: "443",
		Action:   firewallGeoRuleActionAllow,
	}
	var script strings.Builder
	plan, err := appendManagedFirewallSourceRulesScript(&script, []model.FirewallRule{row})
	if err != nil {
		t.Fatalf("appendManagedFirewallSourceRulesScript returned error: %v", err)
	}
	if err := appendManagedFirewallSourceBlockFallbacksScript(&script, plan, []model.FirewallGeoRule{geoAllow}); err != nil {
		t.Fatalf("appendManagedFirewallSourceBlockFallbacksScript returned error: %v", err)
	}
	if strings.Contains(script.String(), "source_block_fallback") {
		t.Fatalf("manual block fallback must not override a GeoIP allowlist: %s", script.String())
	}
}

func TestFirewallSourceBlockFallbackStillAllowsWhenGeoOnlyBlocks(t *testing.T) {
	row := model.FirewallRule{
		Id:         11,
		Enabled:    true,
		Origin:     firewallOriginManual,
		Family:     firewallFamilyIPv4,
		Protocol:   firewallProtocolTCP,
		PortSpec:   "443",
		SourceSpec: "1.1.1.1/32",
		SourceMode: firewallSourceModeBlock,
	}
	geoBlock := model.FirewallGeoRule{
		Enabled:  true,
		Family:   firewallFamilyIPv4,
		Protocol: firewallProtocolTCP,
		PortSpec: "443",
		Action:   firewallGeoRuleActionBlock,
	}
	var script strings.Builder
	plan, err := appendManagedFirewallSourceRulesScript(&script, []model.FirewallRule{row})
	if err != nil {
		t.Fatalf("appendManagedFirewallSourceRulesScript returned error: %v", err)
	}
	if err := appendManagedFirewallSourceBlockFallbacksScript(&script, plan, []model.FirewallGeoRule{geoBlock}); err != nil {
		t.Fatalf("appendManagedFirewallSourceBlockFallbacksScript returned error: %v", err)
	}
	if !strings.Contains(script.String(), "source_block_fallback") {
		t.Fatalf("manual block fallback should remain when GeoIP only blocks selected sources: %s", script.String())
	}
}

func TestSubtractFirewallPortRanges(t *testing.T) {
	got := portRangesToNft(subtractFirewallPortRanges(
		parsePortRangeInput("443-445"),
		parsePortRangeInput("444"),
	))
	if got != "443, 445" {
		t.Fatalf("subtractFirewallPortRanges=%q, want %q", got, "443, 445")
	}
}

func TestBuildManagedFirewallScriptSourceRulesPrecedeSystemAllows(t *testing.T) {
	rows := []model.FirewallRule{
		{
			Id:       1,
			Enabled:  true,
			Origin:   firewallOriginSystem,
			Family:   firewallFamilyDual,
			Protocol: firewallProtocolTCP,
			PortSpec: "22",
		},
		{
			Id:         2,
			Enabled:    true,
			Origin:     firewallOriginManual,
			Family:     firewallFamilyIPv4,
			Protocol:   firewallProtocolTCP,
			PortSpec:   "22",
			SourceSpec: "1.1.1.1/32",
			SourceMode: firewallSourceModeBlock,
		},
	}
	script, err := buildManagedFirewallScript(rows, nil, false)
	if err != nil {
		t.Fatalf("buildManagedFirewallScript returned error: %v", err)
	}
	blockIndex := strings.Index(script, "kwor_firewall_rule_2_ipv4_source_block")
	systemIndex := strings.Index(script, "kwor_firewall_rule_1_ipv4")
	if blockIndex < 0 || systemIndex < 0 {
		t.Fatalf("script missing source block or system allow: %s", script)
	}
	if blockIndex > systemIndex {
		t.Fatalf("source block must precede system allow: %s", script)
	}
}
