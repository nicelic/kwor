package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/alireza0/s-ui/database/model"
)

func TestNormalizeMihomoPortHopRange(t *testing.T) {
	normalized, err := normalizeMihomoPortHopRange(" 22000:22001, 21000-21002 ")
	if err != nil {
		t.Fatalf("normalize range failed: %v", err)
	}
	if normalized != "21000-21002,22000-22001" {
		t.Fatalf("normalized range = %q", normalized)
	}

	if _, err := normalizeMihomoPortHopRange("20000-24100"); err == nil {
		t.Fatal("expected oversized range to be rejected")
	}
}

func TestSanitizeMihomoHysteria2PortHop(t *testing.T) {
	inbound := &model.MihomoInbound{
		Type: "hysteria2",
		Options: json.RawMessage(`{
			"port_hop_range":"21000:21001",
			"port_hop_interval":"30s",
			"port_hop_interval_max":"10s"
		}`),
	}
	if err := sanitizeMihomoHysteria2PortHop(inbound); err != nil {
		t.Fatalf("sanitize Hysteria2 port hop failed: %v", err)
	}
	if !strings.Contains(string(inbound.Options), `"port_hop_range":"21000-21001"`) {
		t.Fatalf("range was not normalized: %s", inbound.Options)
	}
	if !strings.Contains(string(inbound.Options), `"port_hop_interval":"10s"`) || !strings.Contains(string(inbound.Options), `"port_hop_interval_max":"30s"`) {
		t.Fatalf("intervals were not normalized: %s", inbound.Options)
	}

	inbound.Options = json.RawMessage(`{"port_hop_range":"21000-21001","port_hop_interval":"5s"}`)
	if err := sanitizeMihomoHysteria2PortHop(inbound); err == nil {
		t.Fatal("expected an interval below 10 seconds to be rejected")
	}
}

func TestMihomoInboundSaveRejectsHysteriaV1(t *testing.T) {
	payload := json.RawMessage(`{"id":0,"type":"hysteria","tag":"hy1","listen_port":443,"tls_id":0}`)
	if _, err := (&MihomoInboundService{}).Save(nil, "new", payload, "", "example.com"); err == nil || !strings.Contains(err.Error(), "Hysteria v1") {
		t.Fatalf("expected Hysteria v1 rejection, got %v", err)
	}
}
