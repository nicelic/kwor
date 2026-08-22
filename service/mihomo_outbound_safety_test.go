package service

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"testing"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
)

func createMihomoOutboundForSafetyTest(t *testing.T, tag string, outboundType string, payload map[string]interface{}) model.MihomoOutbound {
	t.Helper()
	payload["tag"] = tag
	payload["type"] = outboundType
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal mihomo outbound payload failed: %v", err)
	}
	record := model.MihomoOutbound{
		Tag:         tag,
		Type:        outboundType,
		RawOutbound: raw,
	}
	if err := database.GetDB().Create(&record).Error; err != nil {
		t.Fatalf("create mihomo outbound %q failed: %v", tag, err)
	}
	return record
}

func TestMihomoOutboundDeleteRejectsSelectorReference(t *testing.T) {
	openMihomoOutboundGroupOrderTestDB(t)

	createMihomoOutboundForSafetyTest(t, "node-a", "socks", map[string]interface{}{
		"server":      "1.1.1.1",
		"server_port": 1080,
	})
	createMihomoOutboundForSafetyTest(t, "AUTO", "selector", map[string]interface{}{
		"outbounds": []string{"node-a"},
	})

	tx := database.GetDB().Begin()
	if tx.Error != nil {
		t.Fatalf("begin transaction failed: %v", tx.Error)
	}
	err := (&MihomoOutboundService{}).Save(tx, "del", json.RawMessage(`"node-a"`))
	tx.Rollback()
	if err == nil {
		t.Fatal("expected selector reference to prevent deletion")
	}
	if !strings.Contains(err.Error(), `outbound "AUTO" -> node-a`) {
		t.Fatalf("unexpected deletion error: %v", err)
	}
}

func TestMihomoOutboundTrustTunnelEditDropsUnsupportedTLSBranches(t *testing.T) {
	openMihomoOutboundGroupOrderTestDB(t)

	db := database.GetDB()
	service := &MihomoOutboundService{}
	newPayload := json.RawMessage(`{
		"type":"trusttunnel",
		"tag":"trusttunnel-node",
		"server":"1.1.1.1",
		"server_port":443,
		"tls":{"enabled":true,"utls":{"enabled":true,"fingerprint":"chrome"}}
	}`)
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("begin create transaction failed: %v", tx.Error)
	}
	if err := service.Save(tx, "new", newPayload); err != nil {
		tx.Rollback()
		t.Fatalf("create TrustTunnel outbound failed: %v", err)
	}
	if err := tx.Commit().Error; err != nil {
		t.Fatalf("commit create transaction failed: %v", err)
	}

	var saved model.MihomoOutbound
	if err := db.Where("tag = ?", "trusttunnel-node").First(&saved).Error; err != nil {
		t.Fatalf("load created TrustTunnel outbound failed: %v", err)
	}
	editPayload := json.RawMessage(`{
		"id":` + strconv.FormatUint(uint64(saved.Id), 10) + `,
		"type":"trusttunnel",
		"tag":"trusttunnel-node",
		"server":"1.1.1.1",
		"server_port":443,
		"tls":{
			"enabled":true,
			"utls":{"enabled":true,"fingerprint":"firefox"},
			"reality":{"enabled":true,"public_key":"stale","short_id":"stale"},
			"ech":{"enabled":true,"config":["stale"]}
		}
	}`)
	tx = db.Begin()
	if tx.Error != nil {
		t.Fatalf("begin edit transaction failed: %v", tx.Error)
	}
	if err := service.Save(tx, "edit", editPayload); err != nil {
		tx.Rollback()
		t.Fatalf("edit TrustTunnel outbound failed: %v", err)
	}
	if err := tx.Commit().Error; err != nil {
		t.Fatalf("commit edit transaction failed: %v", err)
	}

	if err := db.Where("id = ?", saved.Id).First(&saved).Error; err != nil {
		t.Fatalf("reload edited TrustTunnel outbound failed: %v", err)
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(saved.RawOutbound, &payload); err != nil {
		t.Fatalf("decode saved raw outbound failed: %v", err)
	}
	tls, ok := payload["tls"].(map[string]interface{})
	if !ok {
		t.Fatalf("saved TrustTunnel TLS payload missing: %#v", payload)
	}
	if _, exists := tls["reality"]; exists {
		t.Fatalf("unsupported TrustTunnel Reality survived: %#v", tls)
	}
	if _, exists := tls["ech"]; exists {
		t.Fatalf("unsupported TrustTunnel ECH survived: %#v", tls)
	}
	utls, ok := tls["utls"].(map[string]interface{})
	if !ok || utls["fingerprint"] != "firefox" {
		t.Fatalf("supported TrustTunnel uTLS was not retained: %#v", tls)
	}
}

func TestNormalizeMihomoOutboundRawPayloadSanitizesHysteria2ReceiveWindows(t *testing.T) {
	raw := normalizeMihomoOutboundRawPayload(json.RawMessage(`{
		"type": "hysteria2",
		"tag": "hy2-node",
		"mihomo_hy2": {
			"initial_stream_receive_window": 1024,
			"max_stream_receive_window": 2048.5,
			"initial_connection_receive_window": "4096",
			"max_connection_receive_window": -1,
			"unexpected": true
		}
	}`))

	payload := map[string]interface{}{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("decode normalized payload: %v", err)
	}
	windows, ok := payload["mihomo_hy2"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected normalized receive windows, got %#v", payload)
	}
	if got := windows["initial_stream_receive_window"]; got != float64(1024) {
		t.Fatalf("initial stream receive window = %#v", got)
	}
	for _, key := range []string{
		"max_stream_receive_window",
		"initial_connection_receive_window",
		"max_connection_receive_window",
		"unexpected",
	} {
		if _, exists := windows[key]; exists {
			t.Fatalf("invalid receive window key %q survived: %#v", key, windows)
		}
	}
}

func TestNormalizeMihomoOutboundRawPayloadMigratesLegacyHysteriaOutboundBandwidth(t *testing.T) {
	for _, outboundType := range []string{"hysteria", "hysteria2"} {
		t.Run(outboundType, func(t *testing.T) {
			raw := normalizeMihomoOutboundRawPayload(json.RawMessage(`{
				"type": "` + outboundType + `",
				"tag": "legacy-bandwidth",
				"server": "example.com",
				"server_port": 443,
				"up_mbps": 100,
				"down_mbps": 100,
				"server_up_mbps": "500",
				"server_down_mbps": 900
			}`))

			payload := map[string]interface{}{}
			if err := json.Unmarshal(raw, &payload); err != nil {
				t.Fatalf("decode normalized payload: %v", err)
			}
			if got := payload["up_mbps"]; got != float64(500) {
				t.Fatalf("up_mbps = %#v", got)
			}
			if got := payload["down_mbps"]; got != float64(900) {
				t.Fatalf("down_mbps = %#v", got)
			}
			for _, key := range []string{"server_up_mbps", "server_down_mbps"} {
				if _, exists := payload[key]; exists {
					t.Fatalf("legacy outbound field %q survived: %#v", key, payload)
				}
			}

			result := convertMihomoOutboundsToClash([]map[string]interface{}{payload})
			if len(result.Proxies) != 1 {
				t.Fatalf("expected 1 proxy, got %d", len(result.Proxies))
			}
			if got := result.Proxies[0]["up"]; got != 500 {
				t.Fatalf("proxy up = %#v", got)
			}
			if got := result.Proxies[0]["down"]; got != 900 {
				t.Fatalf("proxy down = %#v", got)
			}
		})
	}
}

func TestNormalizeMihomoOutboundRawPayloadDropsUnsupportedManualMihomoFields(t *testing.T) {
	tests := []struct {
		name        string
		payload     string
		removedKeys []string
	}{
		{
			name:        "socks",
			payload:     `{"type":"socks","version":"4a","udp_over_tcp":{"enabled":true,"version":2}}`,
			removedKeys: []string{"version", "udp_over_tcp"},
		},
		{
			name:        "http",
			payload:     `{"type":"http","path":"/proxy"}`,
			removedKeys: []string{"path"},
		},
		{
			name:        "vmess",
			payload:     `{"type":"vmess","network":"udp"}`,
			removedKeys: []string{"network"},
		},
		{
			name:        "tuic",
			payload:     `{"type":"tuic","network":"udp"}`,
			removedKeys: []string{"network"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := normalizeMihomoOutboundRawPayload(json.RawMessage(tt.payload))
			payload := map[string]interface{}{}
			if err := json.Unmarshal(raw, &payload); err != nil {
				t.Fatalf("decode normalized payload: %v", err)
			}
			for _, key := range tt.removedKeys {
				if _, exists := payload[key]; exists {
					t.Fatalf("unsupported field %q survived: %#v", key, payload)
				}
			}
		})
	}
}

func TestNormalizeMihomoOutboundRawPayloadRejectsFractionalRoutingMark(t *testing.T) {
	raw := normalizeMihomoOutboundRawPayload(json.RawMessage(`{
		"type": "vless",
		"tag": "vless-node",
		"routing_mark": 12.5
	}`))

	payload := map[string]interface{}{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("decode normalized payload: %v", err)
	}
	if _, exists := payload["routing_mark"]; exists {
		t.Fatalf("fractional routing mark survived: %#v", payload)
	}
}

func TestNormalizeMihomoOutboundRawPayloadRejectsFractionalGRPCValues(t *testing.T) {
	raw := normalizeMihomoOutboundRawPayload(json.RawMessage(`{
		"type": "vless",
		"tag": "grpc-node",
		"transport": {
			"type": "grpc",
			"ping_interval": 10.5,
			"max_connections": 8,
			"min_streams": -1,
			"max_streams": 16.5
		}
	}`))

	payload := map[string]interface{}{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("decode normalized payload: %v", err)
	}
	transport, ok := payload["transport"].(map[string]interface{})
	if !ok {
		t.Fatalf("transport missing: %#v", payload)
	}
	if got := transport["max_connections"]; got != float64(8) {
		t.Fatalf("max_connections = %#v", got)
	}
	for _, key := range []string{"ping_interval", "min_streams", "max_streams"} {
		if _, exists := transport[key]; exists {
			t.Fatalf("invalid gRPC field %q survived: %#v", key, transport)
		}
	}
}

func TestNormalizeMihomoOutboundRawPayloadSanitizesMultiplexAndTLSFragment(t *testing.T) {
	raw := normalizeMihomoOutboundRawPayload(json.RawMessage(`{
		"type": "vless",
		"tag": "multiplex-node",
		"multiplex": {
			"enabled": true,
			"max_connections": 8.5,
			"min_streams": 0,
			"max_streams": 16,
			"brutal": {
				"enabled": true,
				"up_mbps": 100.5,
				"down_mbps": 200
			}
		},
		"tls": {
			"enabled": true,
			"fragment": true,
			"fragment_fallback_delay": "500ms",
			"record_fragment": true
		}
	}`))

	payload := map[string]interface{}{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("decode normalized payload: %v", err)
	}
	mux, ok := payload["multiplex"].(map[string]interface{})
	if !ok {
		t.Fatalf("multiplex missing: %#v", payload)
	}
	if _, exists := mux["max_connections"]; exists {
		t.Fatalf("fractional max_connections survived: %#v", mux)
	}
	if got := mux["min_streams"]; got != float64(0) {
		t.Fatalf("min_streams = %#v", got)
	}
	if got := mux["max_streams"]; got != float64(16) {
		t.Fatalf("max_streams = %#v", got)
	}
	brutal, ok := mux["brutal"].(map[string]interface{})
	if !ok {
		t.Fatalf("brutal missing: %#v", mux)
	}
	if _, exists := brutal["up_mbps"]; exists {
		t.Fatalf("fractional brutal up_mbps survived: %#v", brutal)
	}
	if got := brutal["down_mbps"]; got != float64(200) {
		t.Fatalf("brutal down_mbps = %#v", got)
	}
	tls, ok := payload["tls"].(map[string]interface{})
	if !ok {
		t.Fatalf("TLS missing: %#v", payload)
	}
	for _, key := range []string{"fragment", "fragment_fallback_delay", "record_fragment"} {
		if _, exists := tls[key]; exists {
			t.Fatalf("unsupported TLS field %q survived: %#v", key, tls)
		}
	}
}

func TestMihomoOutboundGroupSaveWithRuntimeImpactSkipsReorder(t *testing.T) {
	openMihomoOutboundGroupOrderTestDB(t)

	db := database.GetDB()
	groups := []model.MihomoOutboundGroup{
		{Name: "alpha", Outbounds: "[]", SortOrder: 1},
		{Name: "beta", Outbounds: "[]", SortOrder: 2},
	}
	for index := range groups {
		if err := db.Create(&groups[index]).Error; err != nil {
			t.Fatalf("create group failed: %v", err)
		}
	}

	payload, err := json.Marshal(map[string][]uint{"ids": {groups[1].Id, groups[0].Id}})
	if err != nil {
		t.Fatalf("marshal reorder payload failed: %v", err)
	}
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("begin transaction failed: %v", tx.Error)
	}
	changed, err := (&MihomoOutboundGroupService{}).SaveWithRuntimeImpact(tx, "reorder", payload)
	if err != nil {
		tx.Rollback()
		t.Fatalf("reorder groups failed: %v", err)
	}
	if changed {
		tx.Rollback()
		t.Fatal("group order must not require a Mihomo runtime regeneration")
	}
	if err := tx.Commit().Error; err != nil {
		t.Fatalf("commit reorder failed: %v", err)
	}
}

func TestMihomoOutboundGroupSaveRejectsWhileSubscriptionImportIsRunning(t *testing.T) {
	openMihomoOutboundGroupOrderTestDB(t)

	if !mihomoOutboundSubscriptionImportMu.TryLock() {
		t.Fatal("expected test subscription lock to be available")
	}
	t.Cleanup(mihomoOutboundSubscriptionImportMu.Unlock)

	tx := database.GetDB().Begin()
	if tx.Error != nil {
		t.Fatalf("begin transaction failed: %v", tx.Error)
	}
	defer tx.Rollback()

	payload, err := json.Marshal(model.MihomoOutboundGroup{Name: "blocked-during-import", Outbounds: "[]"})
	if err != nil {
		t.Fatalf("marshal group payload failed: %v", err)
	}

	_, err = (&MihomoOutboundGroupService{}).SaveWithRuntimeImpact(tx, "new", payload)
	if !errors.Is(err, ErrMihomoSubscriptionImportBusy) {
		t.Fatalf("expected subscription import busy error, got %v", err)
	}

	var count int64
	if err := tx.Model(&model.MihomoOutboundGroup{}).Count(&count).Error; err != nil {
		t.Fatalf("count groups failed: %v", err)
	}
	if count != 0 {
		t.Fatalf("blocked group save must not create records, got %d", count)
	}
}

func TestConfigSaveRejectsMihomoOutboundWriteWhileSubscriptionImportIsRunning(t *testing.T) {
	if !mihomoOutboundSubscriptionImportMu.TryLock() {
		t.Fatal("expected test subscription lock to be available")
	}
	defer mihomoOutboundSubscriptionImportMu.Unlock()

	_, err := (&ConfigService{}).Save("mihomo_outbounds", "new", json.RawMessage(`{}`), "", "", "")
	if !errors.Is(err, ErrMihomoSubscriptionImportBusy) {
		t.Fatalf("expected Mihomo outbound save to reject during subscription import, got %v", err)
	}
}

func TestMihomoShadowQUICJLSUpstreamRejectsPanelGroupTarget(t *testing.T) {
	openMihomoOutboundGroupOrderTestDB(t)

	db := database.GetDB()
	if err := db.Create(&model.MihomoOutboundGroup{Name: "panel-only-group", Outbounds: "[]", SortOrder: 1}).Error; err != nil {
		t.Fatalf("create panel group failed: %v", err)
	}

	inbound := &model.MihomoInbound{
		Type: "shadowquic",
		Tag:  "shadowquic-test",
		Options: json.RawMessage(`{
			"jls_upstream": {"addr": "example.com:443", "proxy": "panel-only-group"}
		}`),
	}
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("begin transaction failed: %v", tx.Error)
	}
	defer tx.Rollback()

	err := validateMihomoShadowQUICJLSUpstreamProxy(tx, inbound)
	if err == nil || !strings.Contains(err.Error(), "not a supported Mihomo proxy or proxy group") {
		t.Fatalf("expected panel-group target rejection, got %v", err)
	}
}

func TestMihomoShadowQUICJLSUpstreamNormalizesDirectOutboundTag(t *testing.T) {
	openMihomoOutboundGroupOrderTestDB(t)

	inbound := &model.MihomoInbound{
		Type: "shadowquic",
		Tag:  "shadowquic-test",
		Options: json.RawMessage(`{
			"jls_upstream": {"addr": "example.com:443", "proxy": "direct"}
		}`),
	}
	tx := database.GetDB().Begin()
	if tx.Error != nil {
		t.Fatalf("begin transaction failed: %v", tx.Error)
	}
	defer tx.Rollback()

	if err := validateMihomoShadowQUICJLSUpstreamProxy(tx, inbound); err != nil {
		t.Fatalf("normalize direct outbound target failed: %v", err)
	}

	options := map[string]interface{}{}
	if err := json.Unmarshal(inbound.Options, &options); err != nil {
		t.Fatalf("decode normalized inbound options failed: %v", err)
	}
	upstream, _ := options["jls_upstream"].(map[string]interface{})
	if got := upstream["proxy"]; got != "DIRECT" {
		t.Fatalf("expected direct target to normalize to DIRECT, got %#v", got)
	}
}

func TestValidateMihomoImportedSubscriptionSizeRejectsOversizedNodeSet(t *testing.T) {
	outbounds := make([]map[string]interface{}, mihomoSubscriptionImportMaxNodes+1)
	for index := range outbounds {
		outbounds[index] = map[string]interface{}{
			"tag":  "node",
			"type": "socks",
		}
	}

	err := validateMihomoImportedSubscriptionSize(outbounds, nil)
	if err == nil || !strings.Contains(err.Error(), "more than") {
		t.Fatalf("expected node-count limit error, got %v", err)
	}
}

func TestMihomoImportedSubscriptionRejectsManualTagCollisionWithoutMutation(t *testing.T) {
	openMihomoOutboundGroupOrderTestDB(t)

	db := database.GetDB()
	group := model.MihomoOutboundGroup{Name: "source-a", Outbounds: "[]", SortOrder: 1}
	if err := db.Create(&group).Error; err != nil {
		t.Fatalf("create group failed: %v", err)
	}
	createMihomoOutboundForSafetyTest(t, "manual-node", "socks", map[string]interface{}{
		"server":      "2.2.2.2",
		"server_port": 1080,
	})

	err := (&MihomoOutboundGroupService{}).applyImportedSubscription(
		db,
		"source-a",
		"https://example.invalid/sub.yaml",
		false,
		[]map[string]interface{}{{
			"tag":         "manual-node",
			"type":        "socks",
			"server":      "3.3.3.3",
			"server_port": 1080,
		}},
		nil,
		false,
	)
	if err == nil || !strings.Contains(err.Error(), "already owned outside") {
		t.Fatalf("expected ownership conflict, got %v", err)
	}

	var reloaded model.MihomoOutboundGroup
	if err := db.Where("id = ?", group.Id).First(&reloaded).Error; err != nil {
		t.Fatalf("reload group failed: %v", err)
	}
	if reloaded.Outbounds != "[]" {
		t.Fatalf("group mutated after rejected import: %q", reloaded.Outbounds)
	}

	var outbound model.MihomoOutbound
	if err := db.Where("tag = ?", "manual-node").First(&outbound).Error; err != nil {
		t.Fatalf("manual outbound disappeared after rejected import: %v", err)
	}
}
