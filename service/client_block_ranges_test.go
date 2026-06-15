package service

import (
	"encoding/json"
	"testing"

	"github.com/alireza0/s-ui/database/model"
)

func TestCollectInboundBlockRanges_Hysteria2IncludesListenAndHopRange(t *testing.T) {
	inbound := &model.Inbound{
		Type: "hysteria2",
		Options: json.RawMessage(`{
  "listen_port": 31100,
  "port_hop_range": "21000-25000"
}`),
	}

	ranges := collectInboundBlockRanges(inbound)
	if len(ranges) != 2 {
		t.Fatalf("unexpected ranges: %#v", ranges)
	}
	if ranges[0] != (portRange{start: 21000, end: 25000}) {
		t.Fatalf("unexpected hop range: %#v", ranges[0])
	}
	if ranges[1] != (portRange{start: 31100, end: 31100}) {
		t.Fatalf("unexpected listen range: %#v", ranges[1])
	}
}

func TestCollectMihomoInboundBlockRanges_MieruIncludesRangeAndListen(t *testing.T) {
	inbound := &model.MihomoInbound{
		Type:    "mieru",
		Options: json.RawMessage(`{"listen_port":31100,"port_range":"21000-25000"}`),
	}

	ranges := collectMihomoInboundBlockRanges(inbound)
	if len(ranges) != 2 {
		t.Fatalf("unexpected ranges: %#v", ranges)
	}
	if ranges[0] != (portRange{start: 21000, end: 25000}) {
		t.Fatalf("unexpected port_range: %#v", ranges[0])
	}
	if ranges[1] != (portRange{start: 31100, end: 31100}) {
		t.Fatalf("unexpected listen range: %#v", ranges[1])
	}
}

func TestCollectMihomoInboundBlockRanges_Hysteria2IncludesHopAndListen(t *testing.T) {
	inbound := &model.MihomoInbound{
		Type:    "hysteria2",
		Options: json.RawMessage(`{"listen_port":31100,"port_hop_range":"21000-25000"}`),
	}

	ranges := collectMihomoInboundBlockRanges(inbound)
	if len(ranges) != 2 {
		t.Fatalf("unexpected ranges: %#v", ranges)
	}
	if ranges[0] != (portRange{start: 21000, end: 25000}) {
		t.Fatalf("unexpected hop range: %#v", ranges[0])
	}
	if ranges[1] != (portRange{start: 31100, end: 31100}) {
		t.Fatalf("unexpected listen range: %#v", ranges[1])
	}
}

func TestCollectMihomoInboundBlockRanges_MieruFallbackToOutJSON(t *testing.T) {
	inbound := &model.MihomoInbound{
		Type:    "mieru",
		Options: json.RawMessage(`{"listen_port":31100}`),
		OutJson: json.RawMessage(`{"port_range":"21000-25000"}`),
	}

	ranges := collectMihomoInboundBlockRanges(inbound)
	if len(ranges) != 2 {
		t.Fatalf("unexpected ranges: %#v", ranges)
	}
	if ranges[0] != (portRange{start: 21000, end: 25000}) {
		t.Fatalf("unexpected range from out_json: %#v", ranges[0])
	}
	if ranges[1] != (portRange{start: 31100, end: 31100}) {
		t.Fatalf("unexpected listen range: %#v", ranges[1])
	}
}

func TestPortRangeJSONEncodeDecodeRoundTrip(t *testing.T) {
	source := []portRange{
		{start: 25000, end: 21000}, // intentionally reversed
		{start: 31100, end: 31100},
	}
	encoded := encodePortRangesJSON(source)
	decoded := decodePortRangesJSON(encoded)

	expected := []portRange{
		{start: 21000, end: 25000},
		{start: 31100, end: 31100},
	}
	if len(decoded) != len(expected) {
		t.Fatalf("unexpected decode length: %#v", decoded)
	}
	for i := range expected {
		if decoded[i] != expected[i] {
			t.Fatalf("range mismatch at %d: got=%#v want=%#v", i, decoded[i], expected[i])
		}
	}
}
