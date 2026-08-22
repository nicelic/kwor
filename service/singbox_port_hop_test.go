package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/alireza0/s-ui/database/model"
)

func TestSanitizeSingboxHysteriaPortHopNormalizesBoundedValues(t *testing.T) {
	inbound := &model.Inbound{
		Type: "hysteria2",
		Options: json.RawMessage(`{
      "port_hop_range":"21000:21001",
      "port_hop_interval":"30s",
      "port_hop_interval_max":"10s"
    }`),
	}

	if err := sanitizeSingboxHysteriaPortHop(inbound); err != nil {
		t.Fatalf("sanitize sing-box port hop failed: %v", err)
	}
	options := map[string]string{}
	if err := json.Unmarshal(inbound.Options, &options); err != nil {
		t.Fatalf("decode normalized options failed: %v", err)
	}
	if options["port_hop_range"] != "21000-21001" {
		t.Fatalf("unexpected normalized range: %#v", options)
	}
	if options["port_hop_interval"] != "10s" || options["port_hop_interval_max"] != "30s" {
		t.Fatalf("unexpected normalized intervals: %#v", options)
	}
}

func TestSanitizeSingboxHysteriaPortHopRejectsOversizedRange(t *testing.T) {
	inbound := &model.Inbound{
		Type:    "hysteria",
		Options: json.RawMessage(`{"port_hop_range":"20000-24100"}`),
	}

	err := sanitizeSingboxHysteriaPortHop(inbound)
	if err == nil || !strings.Contains(err.Error(), "too many ports") {
		t.Fatalf("expected oversized range rejection, got %v", err)
	}
}
