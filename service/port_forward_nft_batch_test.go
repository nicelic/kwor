package service

import (
	"errors"
	"strings"
	"testing"

	"github.com/alireza0/s-ui/database/model"
)

func TestPortForwardAtomicBatchUsesPerOriginalPortMeter(t *testing.T) {
	withNftCapabilitiesTestGlobals(t)
	setNftCapabilitiesForTest(buildNftablesCapabilities("nftables v1.0.6", "6.1.0", nil))
	commands := make([]string, 0)
	runNftFn = func(args ...string) ([]byte, error) {
		commands = append(commands, strings.Join(args, " "))
		if strings.HasPrefix(strings.Join(args, " "), "list counter") {
			return nil, errors.New("No such file or directory")
		}
		return nil, nil
	}
	var script string
	runNftScriptFn = func(value string) ([]byte, error) {
		script = value
		return nil, nil
	}

	row := model.PortForwardRule{
		Id:            42,
		Name:          "meter",
		Enabled:       true,
		Family:        portForwardFamilyIPv4,
		Protocol:      portForwardProtocolTCPUDP,
		LocalPortSpec: "1000,2000-2001",
		TargetIP:      "198.51.100.12",
		TargetPort:    8443,
		RateLimitMbps: 20,
	}
	states, warnings, err := renderManagedPortForwardRulesAtomic([]model.PortForwardRule{row})
	if err != nil {
		t.Fatalf("atomic render: %v", err)
	}
	if len(warnings) != 0 || states[row.Id].status != "applied" || states[row.Id].effectiveRateLimitMbps != 20 {
		t.Fatalf("unexpected meter state: states=%#v warnings=%v", states, warnings)
	}
	if !strings.Contains(script, "meter kwor_pf_meter_42_ipv4 { meta l4proto . ct original proto-dst limit rate over 2500000 bytes/second }") {
		t.Fatalf("missing meter key in batch script:\n%s", script)
	}
	if !strings.Contains(script, "flush chain inet "+portForwardNftTable+" "+portForwardForwardChain) {
		t.Fatalf("batch script must flush owned chains:\n%s", script)
	}
	for _, command := range commands {
		if strings.HasPrefix(command, "flush chain") || strings.HasPrefix(command, "add rule") {
			t.Fatalf("rules must be emitted through one nft -f script, got command %q", command)
		}
	}
}

func TestPortForwardAtomicBatchDegradesRateWhenMeterUnsupported(t *testing.T) {
	withNftCapabilitiesTestGlobals(t)
	setNftCapabilitiesForTest(buildNftablesCapabilities("nftables v0.9.8", "4.19.0", nil))
	row := model.PortForwardRule{
		Id:            7,
		Name:          "legacy",
		Family:        portForwardFamilyIPv4,
		Protocol:      portForwardProtocolTCP,
		LocalPortSpec: "443",
		TargetIP:      portForwardLoopbackIPv4,
		TargetPort:    8443,
		RateLimitMbps: 10,
	}
	state, commands := buildManagedPortForwardRuleCommands(row)
	if state.status != "degraded" || state.effectiveRateLimitMbps != 0 || state.warning == "" {
		t.Fatalf("expected explicit meter degradation, got %#v", state)
	}
	for _, command := range commands {
		line := strings.Join(command, " ")
		if strings.Contains(line, " meter ") || strings.Contains(line, " limit rate ") {
			t.Fatalf("unsupported meter must not emit semantic-inaccurate rate rule: %s", line)
		}
	}
}

func TestPortForwardAtomicBatchRetriesWithoutMeterAfterRuntimeRejection(t *testing.T) {
	withNftCapabilitiesTestGlobals(t)
	setNftCapabilitiesForTest(buildNftablesCapabilities("nftables v1.0.6", "6.1.0", nil))
	runNftFn = func(args ...string) ([]byte, error) {
		if strings.HasPrefix(strings.Join(args, " "), "list counter") {
			return nil, errors.New("No such file or directory")
		}
		return nil, nil
	}
	scripts := make([]string, 0, 2)
	runNftScriptFn = func(script string) ([]byte, error) {
		scripts = append(scripts, script)
		if len(scripts) == 1 {
			return nil, errors.New("meter expression rejected by kernel")
		}
		return nil, nil
	}

	row := model.PortForwardRule{
		Id:            51,
		Name:          "runtime-meter",
		Enabled:       true,
		Family:        portForwardFamilyIPv4,
		Protocol:      portForwardProtocolTCP,
		LocalPortSpec: "8443",
		TargetIP:      portForwardLoopbackIPv4,
		TargetPort:    9443,
		RateLimitMbps: 15,
	}
	states, warnings, err := renderManagedPortForwardRulesAtomic([]model.PortForwardRule{row})
	if err != nil {
		t.Fatalf("fallback render failed: %v", err)
	}
	if len(scripts) != 2 {
		t.Fatalf("expected meter batch plus one fallback batch, got %d", len(scripts))
	}
	if !strings.Contains(scripts[0], " meter ") || strings.Contains(scripts[1], " meter ") {
		t.Fatalf("unexpected fallback scripts:\nfirst:\n%s\nsecond:\n%s", scripts[0], scripts[1])
	}
	state := states[row.Id]
	if state.status != "degraded" || state.effectiveRateLimitMbps != 0 || !strings.Contains(state.warning, "meter expression rejected by kernel") {
		t.Fatalf("unexpected fallback state: %#v", state)
	}
	if len(warnings) != 1 || warnings[0] != state.warning {
		t.Fatalf("unexpected fallback warnings: %v", warnings)
	}
	capabilities := GetNftablesCapabilities()
	if capabilities.SupportsMeters || !strings.Contains(capabilities.MeterProbeError, "meter expression rejected by kernel") {
		t.Fatalf("runtime meter rejection was not cached: %+v", capabilities)
	}
}

func TestPortForwardAtomicBatchFailureReturnsWithoutCleanup(t *testing.T) {
	withNftCapabilitiesTestGlobals(t)
	setNftCapabilitiesForTest(buildNftablesCapabilities("nftables v1.0.6", "6.1.0", nil))
	runNftFn = func(args ...string) ([]byte, error) {
		if strings.HasPrefix(strings.Join(args, " "), "list counter") {
			return nil, errors.New("No such file or directory")
		}
		return nil, nil
	}
	runNftScriptFn = func(string) ([]byte, error) { return nil, errors.New("syntax rejected") }
	_, _, err := renderManagedPortForwardRulesAtomic([]model.PortForwardRule{{
		Id: 8, Family: portForwardFamilyIPv4, Protocol: portForwardProtocolTCP, LocalPortSpec: "8080", TargetIP: portForwardLoopbackIPv4, TargetPort: 80,
	}})
	if err == nil || !strings.Contains(err.Error(), "syntax rejected") {
		t.Fatalf("expected batch error, got %v", err)
	}
}

func TestPortForwardAtomicBatchReportsBothFailuresWithoutCachingFalseMeterDowngrade(t *testing.T) {
	withNftCapabilitiesTestGlobals(t)
	setNftCapabilitiesForTest(buildNftablesCapabilities("nftables v1.0.6", "6.1.0", nil))
	runNftFn = func(args ...string) ([]byte, error) {
		if strings.HasPrefix(strings.Join(args, " "), "list counter") {
			return nil, errors.New("No such file or directory")
		}
		return nil, nil
	}
	calls := 0
	runNftScriptFn = func(string) ([]byte, error) {
		calls++
		if calls == 1 {
			return nil, errors.New("meter batch rejected")
		}
		return nil, errors.New("base syntax rejected")
	}
	_, _, err := renderManagedPortForwardRulesAtomic([]model.PortForwardRule{{
		Id: 9, Name: "both-fail", Enabled: true, Family: portForwardFamilyIPv4, Protocol: portForwardProtocolTCP,
		LocalPortSpec: "8080", TargetIP: portForwardLoopbackIPv4, TargetPort: 80, RateLimitMbps: 10,
	}})
	if err == nil || !strings.Contains(err.Error(), "meter batch rejected") || !strings.Contains(err.Error(), "base syntax rejected") {
		t.Fatalf("expected combined batch error, got %v", err)
	}
	if !GetNftablesCapabilities().SupportsMeters {
		t.Fatal("failed no-meter retry must not cache a meter-only diagnosis")
	}
}

func TestParsePortForwardNftSnapshotBuildsCountersAndCommentsTogether(t *testing.T) {
	snapshot := parsePortForwardNftTableSnapshot([]byte(`
table inet kwor_forward {
  counter kwor_pf_counter_9_up { packets 3 bytes 300 }
  counter kwor_pf_counter_9_down { packets 2 bytes 120 }
  chain pf_forward {
    meta nfproto ipv4 counter packets 1 bytes 40 comment "kwor_pf_rule_9_up"
    meta nfproto ipv4 counter packets 1 bytes 20 comment "kwor_pf_rule_9_down"
  }
}
`))
	if !snapshot.chains[portForwardForwardChain] {
		t.Fatalf("forward chain not parsed: %#v", snapshot.chains)
	}
	if snapshot.commentCounts[portForwardForwardChain][portForwardRuleComment(9, "up")] != 1 {
		t.Fatalf("comment count not parsed: %#v", snapshot.commentCounts)
	}
	if snapshot.counters[portForwardCounterName(9, "up")] != 340 || snapshot.counters[portForwardCounterName(9, "down")] != 140 {
		t.Fatalf("combined counter values not parsed: %#v", snapshot.counters)
	}
}

func TestParsePortForwardNftSnapshotKeepsChainScopeAcrossMultilineMeter(t *testing.T) {
	snapshot := parsePortForwardNftTableSnapshot([]byte(`
table inet kwor_forward {
  chain pf_forward {
    meta nfproto ipv4 ct direction original meter kwor_pf_meter_9_ipv4 {
      meta l4proto . ct original proto-dst limit rate over 1250000 bytes/second
    } counter packets 1 bytes 75 drop comment "kwor_pf_rule_9_down"
  }
}
`))
	if snapshot.commentCounts[portForwardForwardChain][portForwardRuleComment(9, "down")] != 1 {
		t.Fatalf("meter rule comment escaped chain scope: %#v", snapshot.commentCounts)
	}
	if snapshot.counters[portForwardCounterName(9, "down")] != 75 {
		t.Fatalf("meter rule counter was not parsed: %#v", snapshot.counters)
	}
}
