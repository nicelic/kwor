package service

import (
	"strings"
	"testing"

	"github.com/alireza0/s-ui/database/model"
)

func TestNormalizeFirewallProtocolSupportsICMPVariants(t *testing.T) {
	cases := map[string]string{
		"icmp":    firewallProtocolICMP,
		"ICMP":    firewallProtocolICMP,
		"icmp_v4": firewallProtocolICMPv4,
		"icmp-v4": firewallProtocolICMPv4,
		"icmpv4":  firewallProtocolICMPv4,
		"icmp4":   firewallProtocolICMPv4,
		"icmp_v6": firewallProtocolICMPv6,
		"icmp-v6": firewallProtocolICMPv6,
		"icmpv6":  firewallProtocolICMPv6,
		"icmp6":   firewallProtocolICMPv6,
	}

	for raw, expected := range cases {
		if got := normalizeFirewallProtocol(raw); got != expected {
			t.Fatalf("normalizeFirewallProtocol(%q)=%q, want %q", raw, got, expected)
		}
	}
}

func TestNormalizeFirewallPortSpecSkipsICMPPorts(t *testing.T) {
	for _, protocol := range []string{
		firewallProtocolICMP,
		firewallProtocolICMPv4,
		firewallProtocolICMPv6,
	} {
		got, err := normalizeFirewallPortSpec("443", protocol)
		if err != nil {
			t.Fatalf("normalizeFirewallPortSpec(%q) returned error: %v", protocol, err)
		}
		if got != "" {
			t.Fatalf("normalizeFirewallPortSpec(%q)=%q, want empty", protocol, got)
		}
	}
}

func TestBuildManagedFirewallRuleArgsRejectsAnyProtocol(t *testing.T) {
	row := model.FirewallRule{
		Id:       99,
		Family:   firewallFamilyDual,
		Protocol: firewallProtocolAny,
	}
	if _, err := buildManagedFirewallRuleArgs(row, firewallRenderTarget{family: firewallFamilyIPv4}); err == nil {
		t.Fatalf("buildManagedFirewallRuleArgs should reject ANY protocol")
	}
}

func TestNormalizeFirewallGeoProtocolRejectsICMP(t *testing.T) {
	for _, protocol := range []string{
		firewallProtocolICMP,
		firewallProtocolICMPv4,
		firewallProtocolICMPv6,
	} {
		if _, err := normalizeFirewallGeoProtocol(protocol); err == nil {
			t.Fatalf("normalizeFirewallGeoProtocol(%q) should fail", protocol)
		}
	}
}

func TestBuildManagedFirewallRuleArgsForICMPv4(t *testing.T) {
	row := model.FirewallRule{
		Id:       9,
		Family:   firewallFamilyIPv4,
		Protocol: firewallProtocolICMPv4,
	}
	args, err := buildManagedFirewallRuleArgs(row, firewallRenderTarget{family: firewallFamilyIPv4})
	if err != nil {
		t.Fatalf("buildManagedFirewallRuleArgs returned error: %v", err)
	}
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "meta l4proto icmp icmp type echo-request") {
		t.Fatalf("icmp v4 rule missing echo-request match: %s", joined)
	}
	if strings.Contains(joined, "th dport") {
		t.Fatalf("icmp v4 rule must not contain dport match: %s", joined)
	}
}

func TestManagedFirewallStaticRuleSpecsDoNotAllowPingRequests(t *testing.T) {
	joinedRules := make([]string, 0, len(managedFirewallStaticRuleSpecs()))
	for _, rule := range managedFirewallStaticRuleSpecs() {
		joinedRules = append(joinedRules, strings.Join(rule, " "))
	}
	joined := strings.Join(joinedRules, "\n")

	if strings.Contains(joined, "echo-request") {
		t.Fatalf("static firewall rules must not allow ping requests: %s", joined)
	}
	if !strings.Contains(joined, "icmp type { destination-unreachable , time-exceeded , parameter-problem , echo-reply }") {
		t.Fatalf("static firewall rules missing IPv4 control ICMP allowlist: %s", joined)
	}
	if !strings.Contains(joined, "icmpv6 type { destination-unreachable , packet-too-big , time-exceeded , parameter-problem , echo-reply") {
		t.Fatalf("static firewall rules missing IPv6 control ICMP allowlist: %s", joined)
	}
}

func TestFilterFirewallRulesForRenderExcludesExternalRules(t *testing.T) {
	rows := []model.FirewallRule{
		{Id: 1, Origin: firewallOriginSystem},
		{Id: 2, Origin: firewallOriginManual},
		{Id: 3, Origin: firewallOriginExternal},
		{Id: 4, Origin: firewallOriginManual, Protocol: firewallProtocolAny},
	}

	filtered := filterFirewallRulesForRender(rows)
	if len(filtered) != 2 {
		t.Fatalf("unexpected renderable rule count: %d", len(filtered))
	}
	for _, row := range filtered {
		if row.Origin == firewallOriginExternal {
			t.Fatalf("external rule must not participate in managed chain render: %+v", row)
		}
	}
}

func TestBuildManagedFirewallScriptSkipsExternalRules(t *testing.T) {
	rows := []model.FirewallRule{
		{
			Id:       11,
			Enabled:  true,
			Origin:   firewallOriginManual,
			Family:   firewallFamilyDual,
			Protocol: firewallProtocolTCP,
			PortSpec: "22",
		},
		{
			Id:       22,
			Enabled:  true,
			Origin:   firewallOriginExternal,
			Family:   firewallFamilyDual,
			Protocol: firewallProtocolTCP,
			PortSpec: "443",
		},
	}

	script, err := buildManagedFirewallScript(filterFirewallRulesForRender(rows), nil, false)
	if err != nil {
		t.Fatalf("buildManagedFirewallScript returned error: %v", err)
	}
	if !strings.Contains(script, "kwor_firewall_rule_11_ipv4") {
		t.Fatalf("managed manual rule missing from script: %s", script)
	}
	if strings.Contains(script, "kwor_firewall_rule_22_") {
		t.Fatalf("external observed rule must not be rendered into managed script: %s", script)
	}
}
