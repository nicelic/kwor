package service

import (
	"strings"
	"testing"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
)

func TestPortForwardListenerClaimsRejectReverseProxyAgainstSavedRule(t *testing.T) {
	openPortForwardMultiTestDB(t)
	rule := model.PortForwardRule{
		Name:           "forward-9443",
		Enabled:        true,
		Family:         portForwardFamilyIPv4,
		Protocol:       portForwardProtocolTCP,
		LocalPortMode:  portForwardLocalPortModeSingle,
		LocalPortSpec:  "9443",
		LocalPortStart: 9443,
		LocalPortCount: 1,
		LocalPortEnd:   9443,
		TargetIP:       portForwardLoopbackIPv4,
		TargetPort:     10443,
	}
	if err := database.GetDB().Create(&rule).Error; err != nil {
		t.Fatalf("create forwarding rule: %v", err)
	}
	proxy := model.ReverseProxyRule{
		Name:           "web",
		Enabled:        true,
		ListenProtocol: reverseProxyProtocolHTTP,
		ListenPort:     9443,

		TargetProtocol:            reverseProxyProtocolHTTP,
		TargetPort:                8080,
		ListenHTTPVersionStrategy: reverseProxyListenHTTPVersionH2Only,
	}
	if err := database.GetDB().Create(&proxy).Error; err != nil {
		t.Fatalf("create reverse proxy: %v", err)
	}
	if err := validatePortForwardListenerClaimsAgainstActiveRules(database.GetDB()); err == nil {
		t.Fatal("expected reverse proxy listener declaration to conflict")
	} else if !strings.Contains(err.Error(), "forward-9443") {
		t.Fatalf("unexpected conflict error: %v", err)
	}
}

func TestPortForwardListenerClaimsRejectDirectPanelAndSubscriptionPortSave(t *testing.T) {
	openPortForwardMultiTestDB(t)
	rule := model.PortForwardRule{
		Name:           "forward-18443",
		Enabled:        true,
		Family:         portForwardFamilyIPv4,
		Protocol:       portForwardProtocolTCP,
		LocalPortMode:  portForwardLocalPortModeSingle,
		LocalPortSpec:  "18443",
		LocalPortStart: 18443,
		LocalPortCount: 1,
		LocalPortEnd:   18443,
		TargetIP:       portForwardLoopbackIPv4,
		TargetPort:     8443,
	}
	if err := database.GetDB().Create(&rule).Error; err != nil {
		t.Fatalf("create forwarding rule: %v", err)
	}

	settings := &SettingService{}
	if err := settings.SetPort(18443); err == nil {
		t.Fatal("expected direct panel port save to be rejected")
	}
	if err := settings.SetSubPort(18443); err == nil {
		t.Fatal("expected direct subscription port save to be rejected")
	}
}

func TestPortForwardSocketConflictUsesDualStackWildcardSemantics(t *testing.T) {
	originalGOOS := portForwardRuntimeGOOS
	originalSockets := portForwardReadSocketConflictSockets
	originalBindV6Only := portForwardReadSocketConflictBindV6Only
	t.Cleanup(func() {
		portForwardRuntimeGOOS = originalGOOS
		portForwardReadSocketConflictSockets = originalSockets
		portForwardReadSocketConflictBindV6Only = originalBindV6Only
	})
	portForwardRuntimeGOOS = func() string { return "linux" }
	portForwardReadSocketConflictSockets = func(filter firewallListenerFilter) ([]procListenerSocket, error) {
		return []procListenerSocket{{
			protocol: firewallProtocolTCP,
			family:   firewallFamilyIPv6,
			port:     9443,
			wildcard: true,
		}}, nil
	}
	row := normalizedPortForwardRule{
		family:         portForwardFamilyIPv4,
		protocol:       portForwardProtocolTCP,
		localPortSpans: []portSpan{{start: 9443, end: 9443}},
	}

	portForwardReadSocketConflictBindV6Only = func() (bool, bool) { return false, true }
	tcp, _, err := findPortForwardSocketConflicts(row)
	if err != nil || len(tcp) != 1 || tcp[0] != 9443 {
		t.Fatalf("dual-stack wildcard should conflict with IPv4 rule, ports=%v err=%v", tcp, err)
	}

	portForwardReadSocketConflictBindV6Only = func() (bool, bool) { return true, true }
	tcp, _, err = findPortForwardSocketConflicts(row)
	if err != nil || len(tcp) != 0 {
		t.Fatalf("IPv6-only wildcard must not conflict with IPv4 rule, ports=%v err=%v", tcp, err)
	}
}

func TestPortForwardSparseOverlapDoesNotUseBoundingRange(t *testing.T) {
	left := []portSpan{{start: 1000, end: 1000}, {start: 2000, end: 2000}}
	right := []portSpan{{start: 1500, end: 1500}}
	if overlap := summarizePortForwardSpanOverlap(left, right); overlap.count != 0 {
		t.Fatalf("sparse spans must not overlap their bounding range: %#v", overlap)
	}
	wide := summarizePortForwardSpanOverlap([]portSpan{{start: 1, end: 65535}}, []portSpan{{start: 2, end: 65534}})
	if wide.count != 65533 || len(wide.sample) != 12 {
		t.Fatalf("wide overlap must remain bounded: %#v", wide)
	}
}

func TestPortForwardRuntimeConflictsKeepRulesEnabledAndExposeOwners(t *testing.T) {
	originalGOOS := portForwardRuntimeGOOS
	originalSockets := portForwardReadListenerSocketsFn
	originalBindV6Only := portForwardReadBindV6OnlyFn
	originalOwners := portForwardResolveOwnersFn
	t.Cleanup(func() {
		portForwardRuntimeGOOS = originalGOOS
		portForwardReadListenerSocketsFn = originalSockets
		portForwardReadBindV6OnlyFn = originalBindV6Only
		portForwardResolveOwnersFn = originalOwners
	})
	portForwardRuntimeGOOS = func() string { return "linux" }
	portForwardReadListenerSocketsFn = func(filter firewallListenerFilter) ([]procListenerSocket, error) {
		if !filter.matches(firewallProtocolTCP, 18080) {
			t.Fatal("runtime conflict scanner did not filter forwarding port")
		}
		return []procListenerSocket{{
			protocol:    firewallProtocolTCP,
			family:      firewallFamilyIPv4,
			port:        18080,
			bindAddress: "0.0.0.0",
			wildcard:    true,
			inode:       "123",
		}}, nil
	}
	portForwardReadBindV6OnlyFn = func() (bool, bool) { return true, true }
	portForwardResolveOwnersFn = func(inodes map[string]struct{}) map[string][]FirewallListenerOwnerView {
		return map[string][]FirewallListenerOwnerView{"123": {{PID: 4321, Name: "external-service"}}}
	}
	conflicts := collectPortForwardRuntimeConflicts([]model.PortForwardRule{{
		Id: 5, Name: "keep-enabled", Enabled: true, Family: portForwardFamilyIPv4, Protocol: portForwardProtocolTCP,
		LocalPortSpec: "18080", LocalPortStart: 18080, LocalPortEnd: 18080,
	}})
	if len(conflicts) != 1 {
		t.Fatalf("runtime conflicts = %#v", conflicts)
	}
	if conflicts[0].RuleID != 5 || conflicts[0].Port != 18080 || len(conflicts[0].Owners) != 1 || conflicts[0].Owners[0].PID != 4321 {
		t.Fatalf("unexpected runtime conflict: %#v", conflicts[0])
	}
}

func TestPortForwardRuntimeConflictsReachConfiguredMaximum(t *testing.T) {
	originalGOOS := portForwardRuntimeGOOS
	originalSockets := portForwardReadListenerSocketsFn
	originalBindV6Only := portForwardReadBindV6OnlyFn
	originalOwners := portForwardResolveOwnersFn
	t.Cleanup(func() {
		portForwardRuntimeGOOS = originalGOOS
		portForwardReadListenerSocketsFn = originalSockets
		portForwardReadBindV6OnlyFn = originalBindV6Only
		portForwardResolveOwnersFn = originalOwners
	})

	portForwardRuntimeGOOS = func() string { return "linux" }
	portForwardReadListenerSocketsFn = func(filter firewallListenerFilter) ([]procListenerSocket, error) {
		if !filter.matches(firewallProtocolTCP, 18081) {
			t.Fatal("runtime conflict scanner did not include the forwarding port")
		}
		return []procListenerSocket{{
			protocol: firewallProtocolTCP,
			family:   firewallFamilyIPv4,
			port:     18081,
			wildcard: true,
		}}, nil
	}
	portForwardReadBindV6OnlyFn = func() (bool, bool) { return true, true }
	portForwardResolveOwnersFn = func(map[string]struct{}) map[string][]FirewallListenerOwnerView {
		return map[string][]FirewallListenerOwnerView{}
	}

	rows := make([]model.PortForwardRule, portForwardRuntimeConflictMaxCount+1)
	for index := range rows {
		rows[index] = model.PortForwardRule{
			Id:             uint(index + 1),
			Name:           "conflict",
			Enabled:        true,
			Family:         portForwardFamilyIPv4,
			Protocol:       portForwardProtocolTCP,
			LocalPortSpec:  "18081",
			LocalPortStart: 18081,
			LocalPortEnd:   18081,
		}
	}

	conflicts := collectPortForwardRuntimeConflicts(rows)
	if len(conflicts) != portForwardRuntimeConflictMaxCount {
		t.Fatalf("runtime conflicts = %d, want configured maximum %d", len(conflicts), portForwardRuntimeConflictMaxCount)
	}
}
