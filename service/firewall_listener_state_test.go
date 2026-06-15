package service

import (
	"testing"

	"github.com/alireza0/s-ui/database/model"
)

func TestBuildFirewallListenerFilterTracksRelevantProtocols(t *testing.T) {
	rows := []model.FirewallRule{
		{Enabled: true, Protocol: firewallProtocolTCP, PortSpec: "8888-8889"},
		{Enabled: true, Protocol: firewallProtocolUDP, PortSpec: "9000"},
		{Enabled: true, Protocol: firewallProtocolTCPUDP, PortSpec: "10000-10001"},
		{Enabled: true, Protocol: firewallProtocolICMP, PortSpec: ""},
		{Enabled: false, Protocol: firewallProtocolTCP, PortSpec: "11000"},
	}

	filter := buildFirewallListenerFilter(rows)
	if got := portRangesToNft(filter.tcpRanges); got != "8888-8889, 10000-10001" {
		t.Fatalf("tcp filter mismatch: got %q", got)
	}
	if got := portRangesToNft(filter.udpRanges); got != "9000, 10000-10001" {
		t.Fatalf("udp filter mismatch: got %q", got)
	}
}

func TestMatchFirewallListenersForRuleReturnsAllPortRangeListeners(t *testing.T) {
	row := model.FirewallRule{
		Enabled:  true,
		Protocol: firewallProtocolTCP,
		PortSpec: "8888-8890",
	}
	listeners := []FirewallPortListenerView{
		{Port: 8888, Protocol: firewallProtocolTCP},
		{Port: 8889, Protocol: firewallProtocolUDP},
		{Port: 8890, Protocol: firewallProtocolTCP},
		{Port: 8891, Protocol: firewallProtocolTCP},
	}

	matched := matchFirewallListenersForRule(row, listeners)
	if len(matched) != 2 {
		t.Fatalf("matched listener count mismatch: got %d want %d", len(matched), 2)
	}
	if matched[0].Port != 8888 || matched[1].Port != 8890 {
		t.Fatalf("unexpected matched ports: %+v", matched)
	}
}

func TestDecodeProcSocketLocalAddress(t *testing.T) {
	ipv4, wildcard4 := decodeProcSocketLocalAddress("0100007F:1F90", firewallFamilyIPv4)
	if ipv4 != "127.0.0.1" || wildcard4 {
		t.Fatalf("ipv4 decode mismatch: got %q wildcard=%v", ipv4, wildcard4)
	}

	ipv6, wildcard6 := decodeProcSocketLocalAddress("00000000000000000000000001000000:1F90", firewallFamilyIPv6)
	if ipv6 != "::1" || wildcard6 {
		t.Fatalf("ipv6 decode mismatch: got %q wildcard=%v", ipv6, wildcard6)
	}

	unspecified, wildcardAny := decodeProcSocketLocalAddress("00000000000000000000000000000000:1F90", firewallFamilyIPv6)
	if unspecified != "::" || !wildcardAny {
		t.Fatalf("ipv6 unspecified decode mismatch: got %q wildcard=%v", unspecified, wildcardAny)
	}
}

func TestResolveProcListenerStack(t *testing.T) {
	if stack, source := resolveProcListenerStack(procListenerSocket{
		family:   firewallFamilyIPv6,
		wildcard: true,
	}, false, true); stack != firewallFamilyDual || source != "inferred" {
		t.Fatalf("dual-stack inference mismatch: stack=%q source=%q", stack, source)
	}

	if stack, source := resolveProcListenerStack(procListenerSocket{
		family:      firewallFamilyIPv6,
		bindAddress: "::1",
		wildcard:    false,
	}, false, true); stack != firewallFamilyIPv6 || source != "exact" {
		t.Fatalf("ipv6 exact mismatch: stack=%q source=%q", stack, source)
	}
}
