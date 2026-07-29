package service

import (
	"strings"
	"testing"

	"github.com/alireza0/s-ui/database"
)

func withMockedPortForwardSocketSnapshot(t *testing.T, snapshot *socketSnapshot, snapshotErr error) {
	t.Helper()

	originalGOOS := portForwardRuntimeGOOS
	originalSocketReader := portForwardReadSocketConflictSockets
	originalBindV6Only := portForwardReadSocketConflictBindV6Only

	portForwardRuntimeGOOS = func() string { return "linux" }
	portForwardReadSocketConflictSockets = func(filter firewallListenerFilter) ([]procListenerSocket, error) {
		if snapshotErr != nil {
			return nil, snapshotErr
		}
		result := make([]procListenerSocket, 0)
		for port := range snapshot.tcp {
			if filter.matches(firewallProtocolTCP, port) {
				result = append(result, procListenerSocket{protocol: firewallProtocolTCP, family: firewallFamilyIPv4, port: port})
			}
		}
		for port := range snapshot.udp {
			if filter.matches(firewallProtocolUDP, port) {
				result = append(result, procListenerSocket{protocol: firewallProtocolUDP, family: firewallFamilyIPv4, port: port})
			}
		}
		return result, nil
	}
	portForwardReadSocketConflictBindV6Only = func() (bool, bool) { return true, true }

	t.Cleanup(func() {
		portForwardRuntimeGOOS = originalGOOS
		portForwardReadSocketConflictSockets = originalSocketReader
		portForwardReadSocketConflictBindV6Only = originalBindV6Only
	})
}

func TestFindPortForwardSocketConflicts_ProtocolSpecific(t *testing.T) {
	withMockedPortForwardSocketSnapshot(t, &socketSnapshot{
		tcp: map[int]struct{}{
			443: {},
		},
		udp: map[int]struct{}{
			443: {},
		},
	}, nil)

	rowTCP := normalizedPortForwardRule{
		protocol:       portForwardProtocolTCP,
		localPortSpans: []portSpan{{start: 443, end: 443}},
	}
	tcpOnly, udpOnly, err := findPortForwardSocketConflicts(rowTCP)
	if err != nil {
		t.Fatalf("findPortForwardSocketConflicts tcp returned error: %v", err)
	}
	if len(tcpOnly) != 1 || tcpOnly[0] != 443 {
		t.Fatalf("unexpected tcp conflicts: %#v", tcpOnly)
	}
	if len(udpOnly) != 0 {
		t.Fatalf("tcp rule should not collect udp conflicts, got: %#v", udpOnly)
	}

	rowUDP := normalizedPortForwardRule{
		protocol:       portForwardProtocolUDP,
		localPortSpans: []portSpan{{start: 443, end: 443}},
	}
	tcpOnly, udpOnly, err = findPortForwardSocketConflicts(rowUDP)
	if err != nil {
		t.Fatalf("findPortForwardSocketConflicts udp returned error: %v", err)
	}
	if len(tcpOnly) != 0 {
		t.Fatalf("udp rule should not collect tcp conflicts, got: %#v", tcpOnly)
	}
	if len(udpOnly) != 1 || udpOnly[0] != 443 {
		t.Fatalf("unexpected udp conflicts: %#v", udpOnly)
	}

	rowTCPUDP := normalizedPortForwardRule{
		protocol:       portForwardProtocolTCPUDP,
		localPortSpans: []portSpan{{start: 443, end: 443}},
	}
	tcpOnly, udpOnly, err = findPortForwardSocketConflicts(rowTCPUDP)
	if err != nil {
		t.Fatalf("findPortForwardSocketConflicts tcp_udp returned error: %v", err)
	}
	if len(tcpOnly) != 1 || tcpOnly[0] != 443 {
		t.Fatalf("unexpected tcp conflicts for tcp_udp rule: %#v", tcpOnly)
	}
	if len(udpOnly) != 1 || udpOnly[0] != 443 {
		t.Fatalf("unexpected udp conflicts for tcp_udp rule: %#v", udpOnly)
	}
}

func TestFindPortForwardSocketConflicts_NonLinuxNoCheck(t *testing.T) {
	originalGOOS := portForwardRuntimeGOOS
	originalSocketReader := portForwardReadSocketConflictSockets

	portForwardRuntimeGOOS = func() string { return "windows" }
	portForwardReadSocketConflictSockets = func(filter firewallListenerFilter) ([]procListenerSocket, error) {
		t.Fatal("socket reader should not be called on non-linux")
		return nil, nil
	}
	t.Cleanup(func() {
		portForwardRuntimeGOOS = originalGOOS
		portForwardReadSocketConflictSockets = originalSocketReader
	})

	row := normalizedPortForwardRule{
		protocol:       portForwardProtocolTCPUDP,
		localPortSpans: []portSpan{{start: 443, end: 443}},
	}
	tcpConflicts, udpConflicts, err := findPortForwardSocketConflicts(row)
	if err != nil {
		t.Fatalf("findPortForwardSocketConflicts returned error: %v", err)
	}
	if len(tcpConflicts) != 0 || len(udpConflicts) != 0 {
		t.Fatalf("expected no conflicts on non-linux, got tcp=%#v udp=%#v", tcpConflicts, udpConflicts)
	}
}

func TestValidatePortForwardRuleAvailability_MultiAndRangePorts(t *testing.T) {
	openPortForwardMultiTestDB(t)

	withMockedPortForwardSocketSnapshot(t, &socketSnapshot{
		tcp: map[int]struct{}{
			66: {},
			88: {},
		},
		udp: map[int]struct{}{
			99:  {},
			100: {},
		},
	}, nil)

	row := normalizedPortForwardRule{
		enabled:        true,
		family:         portForwardFamilyIPv4,
		protocol:       portForwardProtocolTCPUDP,
		localPortSpec:  "66,88-100",
		localPortStart: 66,
		localPortEnd:   100,
		localPortSpans: []portSpan{
			{start: 66, end: 66},
			{start: 88, end: 100},
		},
		targetIP:   portForwardLoopbackIPv4,
		targetPort: 18080,
	}

	err := validatePortForwardRuleAvailability(database.GetDB(), row)
	if err == nil {
		t.Fatal("expected socket occupancy validation error")
	}
	got := err.Error()
	if !strings.Contains(got, "system TCP ports already in use: 66,88") {
		t.Fatalf("expected tcp occupied ports in error, got: %s", got)
	}
	if !strings.Contains(got, "system UDP ports already in use: 99,100") {
		t.Fatalf("expected udp occupied ports in error, got: %s", got)
	}
}
