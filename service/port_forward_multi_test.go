package service

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
)

func openPortForwardMultiTestDB(t *testing.T) {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "port-forward-multi.db")
	if err := database.InitDB(dbPath); err != nil {
		t.Fatalf("init db failed: %v", err)
	}

	sqlDB, err := database.GetDB().DB()
	if err == nil && sqlDB != nil {
		t.Cleanup(func() {
			_ = sqlDB.Close()
		})
	}
}

func TestNormalizePortForwardLocalPorts_SupportsPortSpec(t *testing.T) {
	mode, start, count, end, spec, spans, err := normalizePortForwardLocalPorts("multi", "66, 88,99", 0, 0, 0)
	if err != nil {
		t.Fatalf("normalizePortForwardLocalPorts returned error: %v", err)
	}
	if mode != "multi" {
		t.Fatalf("unexpected mode: %s", mode)
	}
	if start != 66 || end != 99 || count != 3 {
		t.Fatalf("unexpected summary: start=%d end=%d count=%d", start, end, count)
	}
	if spec != "66,88,99" {
		t.Fatalf("unexpected spec: %q", spec)
	}
	if len(spans) != 3 {
		t.Fatalf("unexpected spans length: %d", len(spans))
	}
}

func TestNormalizePortForwardLocalPorts_SupportsRange(t *testing.T) {
	mode, start, count, end, spec, spans, err := normalizePortForwardLocalPorts("range", "22-88", 0, 0, 0)
	if err != nil {
		t.Fatalf("normalizePortForwardLocalPorts returned error: %v", err)
	}
	if mode != "range" || start != 22 || end != 88 || count != 67 {
		t.Fatalf("unexpected normalized range: mode=%s start=%d end=%d count=%d", mode, start, end, count)
	}
	if spec != "22-88" {
		t.Fatalf("unexpected spec: %q", spec)
	}
	if len(spans) != 1 || spans[0].start != 22 || spans[0].end != 88 {
		t.Fatalf("unexpected spans: %#v", spans)
	}
}

func TestNormalizePortForwardLocalPorts_InvalidSegment(t *testing.T) {
	if _, _, _, _, _, _, err := normalizePortForwardLocalPorts("multi", "66,,99", 0, 0, 0); err == nil {
		t.Fatal("expected invalid port spec error")
	}
}

func TestValidatePortForwardRuleOverlap_SameProtocolBlockedDifferentProtocolAllowed(t *testing.T) {
	openPortForwardMultiTestDB(t)

	existing := model.PortForwardRule{
		Name:           "existing-tcp",
		Enabled:        true,
		Family:         portForwardFamilyIPv4,
		Protocol:       portForwardProtocolTCP,
		LocalPortMode:  "multi",
		LocalPortSpec:  "66",
		LocalPortStart: 66,
		LocalPortCount: 1,
		LocalPortEnd:   66,
		TargetIP:       portForwardLoopbackIPv4,
		TargetPort:     9000,
		RateLimitMbps:  500,
	}
	if err := database.GetDB().Create(&existing).Error; err != nil {
		t.Fatalf("create existing rule failed: %v", err)
	}

	blocked := normalizedPortForwardRule{
		enabled:        true,
		protocol:       portForwardProtocolTCP,
		localPortSpec:  "66",
		localPortStart: 66,
		localPortEnd:   66,
		localPortSpans: []portSpan{{start: 66, end: 66}},
	}
	err := validatePortForwardRuleOverlap(database.GetDB(), 0, blocked)
	if err == nil {
		t.Fatal("expected tcp overlap error")
	}
	if !strings.Contains(err.Error(), "500 Mbps") {
		t.Fatalf("expected limit hint in error, got: %v", err)
	}

	allowed := normalizedPortForwardRule{
		enabled:        true,
		protocol:       portForwardProtocolUDP,
		localPortSpec:  "66",
		localPortStart: 66,
		localPortEnd:   66,
		localPortSpans: []portSpan{{start: 66, end: 66}},
	}
	if err := validatePortForwardRuleOverlap(database.GetDB(), 0, allowed); err != nil {
		t.Fatalf("udp rule should be allowed, got: %v", err)
	}
}

func TestValidatePortForwardRuleOverlap_SameProtocolDifferentFamilyAllowed(t *testing.T) {
	openPortForwardMultiTestDB(t)

	existing := model.PortForwardRule{
		Name:           "existing-ipv4",
		Enabled:        true,
		Family:         portForwardFamilyIPv4,
		Protocol:       portForwardProtocolTCP,
		LocalPortMode:  "single",
		LocalPortSpec:  "8899",
		LocalPortStart: 8899,
		LocalPortCount: 1,
		LocalPortEnd:   8899,
		TargetIP:       portForwardLoopbackIPv4,
		TargetPort:     9000,
	}
	if err := database.GetDB().Create(&existing).Error; err != nil {
		t.Fatalf("create existing rule failed: %v", err)
	}

	ipv6Only := normalizedPortForwardRule{
		enabled:        true,
		family:         portForwardFamilyIPv6,
		protocol:       portForwardProtocolTCP,
		localPortSpec:  "8899",
		localPortStart: 8899,
		localPortEnd:   8899,
		localPortSpans: []portSpan{{start: 8899, end: 8899}},
	}
	if err := validatePortForwardRuleOverlap(database.GetDB(), 0, ipv6Only); err != nil {
		t.Fatalf("ipv6-only rule should be allowed, got: %v", err)
	}

	dual := normalizedPortForwardRule{
		enabled:        true,
		family:         portForwardFamilyDual,
		protocol:       portForwardProtocolTCP,
		localPortSpec:  "8899",
		localPortStart: 8899,
		localPortEnd:   8899,
		localPortSpans: []portSpan{{start: 8899, end: 8899}},
	}
	if err := validatePortForwardRuleOverlap(database.GetDB(), 0, dual); err == nil {
		t.Fatal("dual-stack rule should conflict with existing ipv4 rule")
	}
}

func TestGenerateUniqueThreeDigitPortForwardName(t *testing.T) {
	openPortForwardMultiTestDB(t)

	existingNames := []string{"001", "123", "888"}
	for _, name := range existingNames {
		row := model.PortForwardRule{
			Name:           name,
			Enabled:        false,
			Family:         portForwardFamilyIPv4,
			Protocol:       portForwardProtocolTCP,
			LocalPortMode:  "single",
			LocalPortSpec:  "12000",
			LocalPortStart: 12000,
			LocalPortCount: 1,
			LocalPortEnd:   12000,
			TargetIP:       portForwardLoopbackIPv4,
			TargetPort:     12001,
		}
		if err := database.GetDB().Create(&row).Error; err != nil {
			t.Fatalf("create row %q failed: %v", name, err)
		}
	}

	got, err := generateUniqueThreeDigitPortForwardName(database.GetDB(), 0)
	if err != nil {
		t.Fatalf("generateUniqueThreeDigitPortForwardName returned error: %v", err)
	}
	if matched, _ := regexp.MatchString(`^\d{3}$`, got); !matched {
		t.Fatalf("expected 3-digit name, got %q", got)
	}
	for _, name := range existingNames {
		if got == name {
			t.Fatalf("generated duplicate name: %q", got)
		}
	}
}

func TestNormalizePortForwardTarget_DualFamilyRequiresLocalTarget(t *testing.T) {
	target, family, err := normalizePortForwardTarget("", portForwardFamilyDual)
	if err != nil {
		t.Fatalf("normalizePortForwardTarget local dual returned error: %v", err)
	}
	if family != portForwardFamilyDual {
		t.Fatalf("unexpected family: %s", family)
	}
	if target != portForwardLoopbackIPv4 {
		t.Fatalf("unexpected local target: %s", target)
	}

	if _, _, err := normalizePortForwardTarget("1.2.3.4", portForwardFamilyDual); err == nil {
		t.Fatal("expected dual family with remote ip to be rejected")
	}
}
