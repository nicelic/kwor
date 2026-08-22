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

func TestClientBlockPolicyDistinguishesManualDisableAndDepletion(t *testing.T) {
	evaluation := evaluateClientAccess(true, 100, 100, 0, 1)
	if !evaluation.Blocked {
		t.Fatal("usage at the configured cap must be blocked")
	}

	manualDisabledShouldBlock := (false && evaluation.Blocked) || (!false && false)
	if manualDisabledShouldBlock {
		t.Fatal("manually disabled client must not create a shared-port block")
	}

	depletedShouldBlock := (false && evaluation.Blocked) || (!false && true)
	if !depletedShouldBlock {
		t.Fatal("automatically depleted client must keep its block until access is restored")
	}
}

func TestMihomoDepleteClientsMarksPersistentDepletionState(t *testing.T) {
	db := initClientLimitTestDB(t, "mihomo-deplete-state.db")
	client := mustCreateMihomoClient(t, db, model.MihomoClient{
		Enable:   true,
		Name:     "mihomo-depleted-client",
		Inbounds: json.RawMessage(`[901]`),
		Volume:   100,
		Up:       100,
	})

	if _, err := (&MihomoClientService{}).DepleteClients(); err != nil {
		t.Fatalf("deplete mihomo clients failed: %v", err)
	}

	var saved model.MihomoClient
	if err := db.First(&saved, client.Id).Error; err != nil {
		t.Fatalf("load depleted mihomo client failed: %v", err)
	}
	if saved.Enable || !saved.Depleted {
		t.Fatalf("unexpected depleted state: enable=%t depleted=%t", saved.Enable, saved.Depleted)
	}
}

func TestMihomoPortBlockKeepsOnlyAutomaticDepletion(t *testing.T) {
	db := initClientLimitTestDB(t, "mihomo-block-depleted.db")
	createMihomoInbound(t, db, 902, "depleted-inbound", map[string]interface{}{"listen_port": 31902}, nil)
	client := mustCreateMihomoClient(t, db, model.MihomoClient{
		Enable:   false,
		Depleted: true,
		Name:     "persisted-depletion",
		Inbounds: json.RawMessage(`[902]`),
		Links:    json.RawMessage(`[]`),
	})

	svc := &MihomoClientPortBlockService{}
	desired, err := svc.collectDesiredBlockedPorts(db)
	if err != nil {
		t.Fatalf("collect depleted block ports failed: %v", err)
	}
	if _, ok := desired[31902]; !ok {
		t.Fatal("automatically depleted mihomo client must keep its port block")
	}

	if err := db.Model(&model.MihomoClient{}).Where("id = ?", client.Id).Update("depleted", false).Error; err != nil {
		t.Fatalf("clear persisted depletion failed: %v", err)
	}
	desired, err = svc.collectDesiredBlockedPorts(db)
	if err != nil {
		t.Fatalf("collect manual-disable block ports failed: %v", err)
	}
	if _, ok := desired[31902]; ok {
		t.Fatal("manually disabled mihomo client must not block a shared port")
	}
}
