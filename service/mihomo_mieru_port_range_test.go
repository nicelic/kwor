package service

import (
	"encoding/json"
	"testing"

	"github.com/alireza0/s-ui/database/model"
)

func TestResolveMihomoInboundRedirectSpec_MieruPrefersOptions(t *testing.T) {
	inbound := &model.MihomoInbound{
		Type:    "mieru",
		Options: json.RawMessage(`{"port_range":"2090:2099"}`),
		OutJson: json.RawMessage(`{"port_range":"3000-3009"}`),
	}

	gotRange, gotRedirectTCP := resolveMihomoInboundRedirectSpec(inbound)
	if !gotRedirectTCP {
		t.Fatalf("expected redirectTCP=true for mieru")
	}
	if gotRange != "2090-2099" {
		t.Fatalf("expected range from options, got %q", gotRange)
	}
}

func TestResolveMihomoInboundRedirectSpec_MieruFallsBackToOutJSON(t *testing.T) {
	inbound := &model.MihomoInbound{
		Type:    "mieru",
		Options: json.RawMessage(`{}`),
		OutJson: json.RawMessage(`{"port_range":"400:500"}`),
	}

	gotRange, gotRedirectTCP := resolveMihomoInboundRedirectSpec(inbound)
	if !gotRedirectTCP {
		t.Fatalf("expected redirectTCP=true for mieru")
	}
	if gotRange != "400-500" {
		t.Fatalf("expected fallback range 400-500, got %q", gotRange)
	}
}

func TestResolveMihomoInboundRedirectSpec_NonMieruUsesPortHopRange(t *testing.T) {
	inbound := &model.MihomoInbound{
		Type:    "hysteria2",
		Options: json.RawMessage(`{"port_hop_range":"9000:9010"}`),
	}

	gotRange, gotRedirectTCP := resolveMihomoInboundRedirectSpec(inbound)
	if gotRedirectTCP {
		t.Fatalf("expected redirectTCP=false for non-mieru")
	}
	if gotRange != "9000:9010" {
		t.Fatalf("expected port_hop_range from options, got %q", gotRange)
	}
}
