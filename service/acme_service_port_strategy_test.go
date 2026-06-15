package service

import (
	"testing"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
)

func TestSelectAcmeChallengePortDecisionSwitchesToAvailableTCP(t *testing.T) {
	snapshot := &acmeChallengePortSnapshot{
		Supported: true,
		ByPort: map[int]SinglePortStatus{
			acmeIPCertificatePortHTTP: {Port: acmeIPCertificatePortHTTP, TCP: true, UDP: false},
			acmeIPCertificatePortALPN: {Port: acmeIPCertificatePortALPN, TCP: false, UDP: false},
		},
	}
	decision, err := selectAcmeChallengePortDecision(acmeCertificateTypeIP, "standalone", snapshot)
	if err != nil {
		t.Fatalf("expected switch decision, got err: %v", err)
	}
	if decision.Challenge != "alpn" || !decision.Switched || !decision.Available {
		t.Fatalf("unexpected decision: %#v", decision)
	}
}

func TestSelectAcmeChallengePortDecisionRejectsUDPOnlyFallback(t *testing.T) {
	snapshot := &acmeChallengePortSnapshot{
		Supported: true,
		ByPort: map[int]SinglePortStatus{
			acmeIPCertificatePortHTTP: {Port: acmeIPCertificatePortHTTP, TCP: true, UDP: true},
			acmeIPCertificatePortALPN: {Port: acmeIPCertificatePortALPN, TCP: true, UDP: false},
		},
	}
	if _, err := selectAcmeChallengePortDecision(acmeCertificateTypeIP, "standalone", snapshot); err == nil {
		t.Fatal("expected no-port error when only 443/udp is available")
	}
}

func TestSelectAcmeChallengePortDecisionBlocksWhenNoPortCombination(t *testing.T) {
	snapshot := &acmeChallengePortSnapshot{
		Supported: true,
		ByPort: map[int]SinglePortStatus{
			acmeIPCertificatePortHTTP: {Port: acmeIPCertificatePortHTTP, TCP: true, UDP: true},
			acmeIPCertificatePortALPN: {Port: acmeIPCertificatePortALPN, TCP: true, UDP: true},
		},
	}
	if _, err := selectAcmeChallengePortDecision(acmeCertificateTypeIP, "standalone", snapshot); err == nil {
		t.Fatal("expected no-port error, got nil")
	}
}

func TestSelectAcmeChallengePortDecisionKeepsExplicitWebrootWhenPortOccupied(t *testing.T) {
	snapshot := &acmeChallengePortSnapshot{
		Supported: true,
		ByPort: map[int]SinglePortStatus{
			acmeIPCertificatePortHTTP: {Port: acmeIPCertificatePortHTTP, TCP: true, UDP: false},
			acmeIPCertificatePortALPN: {Port: acmeIPCertificatePortALPN, TCP: false, UDP: false},
		},
	}
	decision, err := selectAcmeChallengePortDecision(acmeCertificateTypeDomain, "webroot", snapshot)
	if err != nil {
		t.Fatalf("expected explicit webroot decision to continue, got err: %v", err)
	}
	if decision.Challenge != "webroot" || decision.Switched {
		t.Fatalf("unexpected webroot decision: %#v", decision)
	}
	if !decision.Available {
		t.Fatalf("expected explicit webroot to stay available: %#v", decision)
	}
}

func TestSelectAcmeChallengePortDecisionSwitchesFromALPNToStandalone(t *testing.T) {
	snapshot := &acmeChallengePortSnapshot{
		Supported: true,
		ByPort: map[int]SinglePortStatus{
			acmeIPCertificatePortHTTP: {Port: acmeIPCertificatePortHTTP, TCP: false, UDP: false},
			acmeIPCertificatePortALPN: {Port: acmeIPCertificatePortALPN, TCP: true, UDP: false},
		},
	}
	decision, err := selectAcmeChallengePortDecision(acmeCertificateTypeIP, "alpn", snapshot)
	if err != nil {
		t.Fatalf("expected switch decision, got err: %v", err)
	}
	if decision.Challenge != "standalone" || !decision.Switched || !decision.Available {
		t.Fatalf("unexpected decision: %#v", decision)
	}
}

func TestFirewallHasManagedDualTCPUDPCoverageLocked(t *testing.T) {
	setupAcmeIPBehaviorTestDB(t, "acme-port-strategy-firewall.db")

	createRule := func(family string, protocol string) {
		rule := model.FirewallRule{
			Name:       "test",
			Enabled:    true,
			Origin:     firewallOriginManual,
			Direction:  firewallDirectionIngress,
			Family:     family,
			Protocol:   protocol,
			PortSpec:   "443",
			SourceSpec: "",
		}
		if err := database.GetDB().Create(&rule).Error; err != nil {
			t.Fatalf("create rule failed: %v", err)
		}
	}

	createRule(firewallFamilyIPv4, firewallProtocolTCP)
	allowed, err := firewallHasManagedDualTCPUDPPortAllowLocked(443)
	if err != nil {
		t.Fatalf("coverage check failed: %v", err)
	}
	if allowed {
		t.Fatal("expected partial coverage to be false")
	}

	createRule(firewallFamilyIPv4, firewallProtocolUDP)
	createRule(firewallFamilyIPv6, firewallProtocolTCP)
	createRule(firewallFamilyIPv6, firewallProtocolUDP)

	allowed, err = firewallHasManagedDualTCPUDPPortAllowLocked(443)
	if err != nil {
		t.Fatalf("coverage check failed: %v", err)
	}
	if !allowed {
		t.Fatal("expected full coverage to be true")
	}
}

func TestFirewallHasManagedDualTCPUDPPortsAllowLockedRequiresBothPorts(t *testing.T) {
	setupAcmeIPBehaviorTestDB(t, "acme-port-strategy-firewall-both.db")

	createRule := func(portSpec string) {
		rule := model.FirewallRule{
			Name:       "test",
			Enabled:    true,
			Origin:     firewallOriginManual,
			Direction:  firewallDirectionIngress,
			Family:     firewallFamilyDual,
			Protocol:   firewallProtocolTCPUDP,
			PortSpec:   portSpec,
			SourceSpec: "",
		}
		if err := database.GetDB().Create(&rule).Error; err != nil {
			t.Fatalf("create rule failed: %v", err)
		}
	}

	createRule("80")
	allowed, err := firewallHasManagedDualTCPUDPPortsAllowLocked(acmeIPCertificatePortHTTP, acmeIPCertificatePortALPN)
	if err != nil {
		t.Fatalf("coverage check failed: %v", err)
	}
	if allowed {
		t.Fatal("expected partial dual-port coverage to be false")
	}

	createRule("443")
	allowed, err = firewallHasManagedDualTCPUDPPortsAllowLocked(acmeIPCertificatePortHTTP, acmeIPCertificatePortALPN)
	if err != nil {
		t.Fatalf("coverage check failed: %v", err)
	}
	if !allowed {
		t.Fatal("expected full dual-port coverage to be true")
	}
}

func TestBuildAcmeTemporaryFirewallRuleRowUsesDualPortSpec(t *testing.T) {
	row := buildAcmeTemporaryFirewallRuleRow()
	if row.Name != "ACME temporary allow 80/443" {
		t.Fatalf("unexpected rule name: %q", row.Name)
	}
	if row.Origin != firewallOriginTemporary || row.TemporaryType != acmeTemporaryFirewallType {
		t.Fatalf("unexpected temporary rule metadata: %#v", row)
	}
	if row.Family != firewallFamilyDual || row.Protocol != firewallProtocolTCPUDP {
		t.Fatalf("unexpected temporary rule network settings: %#v", row)
	}
	if row.PortSpec != acmeTemporaryFirewallPortSpec {
		t.Fatalf("unexpected temporary rule port spec: %q", row.PortSpec)
	}
}

func TestBuildAcmeIPPortItemALPNUDPOnlyIsUnavailable(t *testing.T) {
	item := buildAcmeIPPortItem("alpn", acmeIPCertificatePortALPN, true, false, true, "")
	if item.Available {
		t.Fatalf("expected alpn udp-only item to be unavailable: %#v", item)
	}
}

func TestBuildAcmeIPPortItemWebrootIsAlwaysAvailable(t *testing.T) {
	item := buildAcmeIPPortItem("webroot", acmeIPCertificatePortHTTP, true, false, false, "webroot message")
	if !item.Available {
		t.Fatalf("expected webroot item to stay available: %#v", item)
	}
	if item.Message != "webroot message" {
		t.Fatalf("unexpected message: %#v", item)
	}
}
