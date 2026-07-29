package service

import (
	"strings"
	"testing"
)

func TestNormalizePortForwardRulePayloadRejectsOversizedAndInvalidText(t *testing.T) {
	base := PortForwardRulePayload{
		Name:           "valid",
		Description:    "valid",
		Enabled:        true,
		Family:         portForwardFamilyIPv4,
		Protocol:       portForwardProtocolTCP,
		LocalPortMode:  portForwardLocalPortModeSingle,
		LocalPortStart: 18080,
		TargetIP:       "198.51.100.10",
		TargetPort:     443,
	}
	testCases := []struct {
		name   string
		mutate func(*PortForwardRulePayload)
	}{
		{"name length", func(value *PortForwardRulePayload) { value.Name = strings.Repeat("n", portForwardRuleNameMaxRunes+1) }},
		{"description length", func(value *PortForwardRulePayload) {
			value.Description = strings.Repeat("d", portForwardRuleDescriptionMaxRunes+1)
		}},
		{"port spec length", func(value *PortForwardRulePayload) {
			value.LocalPortSpec = strings.Repeat("1", portForwardPortSpecMaxRunes+1)
		}},
		{"target length", func(value *PortForwardRulePayload) {
			value.TargetIP = strings.Repeat("1", portForwardTargetIPMaxRunes+1)
		}},
		{"control character", func(value *PortForwardRulePayload) { value.Name = "bad\nname" }},
		{"invalid utf8", func(value *PortForwardRulePayload) { value.Description = string([]byte{0xff}) }},
		{"negative rate", func(value *PortForwardRulePayload) { value.RateLimitMbps = -1 }},
		{"excessive rate", func(value *PortForwardRulePayload) { value.RateLimitMbps = portForwardRateLimitMaxMbps + 1 }},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			payload := base
			testCase.mutate(&payload)
			if _, err := normalizePortForwardRulePayload(payload); err == nil {
				t.Fatal("expected payload validation failure")
			}
		})
	}
}

func TestNormalizePortForwardRulePayloadValidatesTrafficControlsAndKeepsOmittedFieldsCompatible(t *testing.T) {
	base := PortForwardRulePayload{
		Name:           "traffic",
		Enabled:        true,
		Family:         portForwardFamilyIPv4,
		Protocol:       portForwardProtocolTCP,
		LocalPortMode:  portForwardLocalPortModeSingle,
		LocalPortStart: 18081,
		TargetIP:       "198.51.100.10",
		TargetPort:     443,
	}
	withoutTraffic, err := normalizePortForwardRulePayload(base)
	if err != nil {
		t.Fatalf("normalize legacy payload: %v", err)
	}
	if withoutTraffic.trafficLimitProvided || withoutTraffic.trafficResetProvided || withoutTraffic.trafficExpiryProvided {
		t.Fatalf("omitted traffic fields must remain distinguishable for old clients: %#v", withoutTraffic)
	}

	limit := 64.25
	resetDay := 17
	expiry := "2027-05-04"
	base.TrafficLimitGiB = &limit
	base.TrafficResetDay = &resetDay
	base.TrafficExpiryDate = &expiry
	normalized, err := normalizePortForwardRulePayload(base)
	if err != nil {
		t.Fatalf("normalize traffic payload: %v", err)
	}
	if normalized.trafficLimitBytes != int64(limit*float64(portForwardTrafficGiBBytes)) || normalized.trafficResetDay != resetDay || normalized.trafficExpiryDate != expiry {
		t.Fatalf("unexpected normalized traffic controls: %#v", normalized)
	}

	badLimit := -1.0
	base.TrafficLimitGiB = &badLimit
	if _, err := normalizePortForwardRulePayload(base); err == nil {
		t.Fatal("expected negative traffic limit rejection")
	}
	validLimit := 1.0
	badDay := 32
	base.TrafficLimitGiB = &validLimit
	base.TrafficResetDay = &badDay
	if _, err := normalizePortForwardRulePayload(base); err == nil {
		t.Fatal("expected invalid traffic reset day rejection")
	}
	validDay := 0
	badExpiry := "2027-99-99"
	base.TrafficResetDay = &validDay
	base.TrafficExpiryDate = &badExpiry
	if _, err := normalizePortForwardRulePayload(base); err == nil {
		t.Fatal("expected invalid traffic expiry rejection")
	}
}
