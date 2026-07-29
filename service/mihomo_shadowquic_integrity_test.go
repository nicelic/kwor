package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alireza0/s-ui/database/model"
	"gorm.io/gorm"
)

func addShadowQUICJLSReference(t *testing.T, db *gorm.DB, tag, proxy string) *model.MihomoInbound {
	t.Helper()
	inbound := &model.MihomoInbound{
		Type: "shadowquic",
		Tag:  tag,
		Options: mustJSONRaw(t, map[string]interface{}{
			"listen":      "0.0.0.0",
			"listen_port": 10443,
			"jls_upstream": map[string]interface{}{
				"addr":  "upstream.example.com:443",
				"proxy": proxy,
			},
		}),
	}
	if err := db.Create(inbound).Error; err != nil {
		t.Fatalf("create ShadowQUIC inbound failed: %v", err)
	}
	return inbound
}

func assertShadowQUICJLSReference(t *testing.T, db *gorm.DB, inboundID uint, expected string) {
	t.Helper()
	inbound := &model.MihomoInbound{}
	if err := db.First(inbound, inboundID).Error; err != nil {
		t.Fatalf("reload ShadowQUIC inbound failed: %v", err)
	}
	proxy, err := shadowQUICJLSProxyFromOptions(inbound.Options)
	if err != nil {
		t.Fatalf("read ShadowQUIC JLS proxy failed: %v", err)
	}
	if proxy != expected {
		t.Fatalf("unexpected ShadowQUIC JLS proxy: got %q want %q", proxy, expected)
	}
}

func TestShadowQUICInboundSaveRejectsUnknownJLSProxy(t *testing.T) {
	db := setupMihomoSyncTestDB(t, "shadowquic-jls-save-validation.db")
	payload := mustJSONRaw(t, map[string]interface{}{
		"type":        "shadowquic",
		"tag":         "sq-missing-target",
		"listen":      "0.0.0.0",
		"listen_port": 10443,
		"jls_upstream": map[string]interface{}{
			"addr":  "upstream.example.com:443",
			"proxy": "missing-target",
		},
	})

	_, err := (&MihomoInboundService{}).Save(db, "new", payload, "", "panel.example.com")
	if err == nil || !strings.Contains(err.Error(), "jls-upstream.proxy target") {
		t.Fatalf("expected unknown JLS proxy to be rejected, got %v", err)
	}
}

func TestShadowQUICInboundSaveNormalizesJLSDirect(t *testing.T) {
	db := setupMihomoSyncTestDB(t, "shadowquic-jls-direct-save.db")
	payload := mustJSONRaw(t, map[string]interface{}{
		"type":        "shadowquic",
		"tag":         "sq-direct-target",
		"listen":      "0.0.0.0",
		"listen_port": 10443,
		"jls_upstream": map[string]interface{}{
			"addr":  "upstream.example.com:443",
			"proxy": "direct",
		},
	})

	if _, err := (&MihomoInboundService{}).Save(db, "new", payload, "", "panel.example.com"); err != nil {
		t.Fatalf("save ShadowQUIC inbound with DIRECT failed: %v", err)
	}
	var inbound model.MihomoInbound
	if err := db.Where("tag = ?", "sq-direct-target").First(&inbound).Error; err != nil {
		t.Fatalf("reload saved ShadowQUIC inbound failed: %v", err)
	}
	options := map[string]interface{}{}
	if err := json.Unmarshal(inbound.Options, &options); err != nil {
		t.Fatalf("decode saved ShadowQUIC options failed: %v", err)
	}
	upstream, ok := options["jls_upstream"].(map[string]interface{})
	if !ok || upstream["proxy"] != "DIRECT" {
		t.Fatalf("expected persisted JLS proxy DIRECT, got %#v", options["jls_upstream"])
	}
}

func TestShadowQUICBuiltinDirectIsNotOutboundReference(t *testing.T) {
	db := setupMihomoSyncTestDB(t, "shadowquic-jls-direct-reference.db")
	addShadowQUICJLSReference(t, db, "sq-direct-reference", "direct")

	matched, err := findMihomoShadowQUICJLSProxyInboundTags(db, "direct")
	if err != nil {
		t.Fatalf("find JLS references failed: %v", err)
	}
	if len(matched) != 0 {
		t.Fatalf("built-in DIRECT must not be tracked as a database outbound reference: %#v", matched)
	}
}

func TestShadowQUICOutboundDeleteRejectsJLSReference(t *testing.T) {
	db := setupMihomoSyncTestDB(t, "shadowquic-outbound-delete-reference.db")
	if err := db.Create(&model.MihomoOutbound{Type: "direct", Tag: "sq-target"}).Error; err != nil {
		t.Fatalf("create Mihomo outbound failed: %v", err)
	}
	addShadowQUICJLSReference(t, db, "sq-inbound", "sq-target")

	err := (&MihomoOutboundService{}).Save(db, "del", mustJSONRaw(t, "sq-target"))
	if err == nil || !strings.Contains(err.Error(), "sq-inbound") {
		t.Fatalf("expected referenced Mihomo outbound delete to fail, got %v", err)
	}
}

func TestShadowQUICOutboundRenameUpdatesJLSReference(t *testing.T) {
	db := setupMihomoSyncTestDB(t, "shadowquic-outbound-rename-reference.db")
	outbound := &model.MihomoOutbound{
		Type: "direct",
		Tag:  "sq-target-old",
		RawOutbound: mustJSONRaw(t, map[string]interface{}{
			"type": "direct",
			"tag":  "sq-target-old",
		}),
	}
	if err := db.Create(outbound).Error; err != nil {
		t.Fatalf("create Mihomo outbound failed: %v", err)
	}
	inbound := addShadowQUICJLSReference(t, db, "sq-inbound", outbound.Tag)

	payload := mustJSONRaw(t, map[string]interface{}{
		"id":   outbound.Id,
		"type": "direct",
		"tag":  "sq-target-new",
	})
	if err := (&MihomoOutboundService{}).Save(db, "edit", payload); err != nil {
		t.Fatalf("rename Mihomo outbound failed: %v", err)
	}

	assertShadowQUICJLSReference(t, db, inbound.Id, "sq-target-new")
}

func TestShadowQUICOutboundGroupDeleteRejectsJLSReference(t *testing.T) {
	db := setupMihomoSyncTestDB(t, "shadowquic-group-delete-reference.db")
	group := &model.MihomoOutboundGroup{Name: "sq-group", Outbounds: "[]", SortOrder: 1}
	if err := db.Create(group).Error; err != nil {
		t.Fatalf("create Mihomo outbound group failed: %v", err)
	}
	addShadowQUICJLSReference(t, db, "sq-inbound", group.Name)

	err := (&MihomoOutboundGroupService{}).Save(db, "del", mustJSONRaw(t, group.Name))
	if err == nil || !strings.Contains(err.Error(), "sq-inbound") {
		t.Fatalf("expected referenced Mihomo outbound group delete to fail, got %v", err)
	}
}

func TestShadowQUICOutboundGroupRefreshRejectsJLSReferencedRemoval(t *testing.T) {
	db := setupMihomoSyncTestDB(t, "shadowquic-group-refresh-reference.db")
	group := &model.MihomoOutboundGroup{Name: "sq-group", Outbounds: `["sq-target"]`, SortOrder: 1}
	if err := db.Create(group).Error; err != nil {
		t.Fatalf("create Mihomo outbound group failed: %v", err)
	}
	if err := db.Create(&model.MihomoOutbound{Type: "direct", Tag: "sq-target"}).Error; err != nil {
		t.Fatalf("create Mihomo outbound failed: %v", err)
	}
	addShadowQUICJLSReference(t, db, "sq-inbound", "sq-target")

	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/yaml")
		_, _ = writer.Write([]byte("proxies:\n  - name: replacement\n    type: socks5\n    server: 127.0.0.1\n    port: 1080\n"))
	}))
	defer server.Close()

	_, err := (&MihomoOutboundGroupService{}).RefreshSubscription(group.Name, server.URL, false)
	if err == nil || !strings.Contains(err.Error(), "sq-inbound") {
		t.Fatalf("expected refresh to reject the referenced outbound removal, got %v", err)
	}

	var count int64
	if err := db.Model(&model.MihomoOutbound{}).Where("tag = ?", "sq-target").Count(&count).Error; err != nil {
		t.Fatalf("count Mihomo outbound failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected referenced outbound to remain after rejected refresh, count=%d", count)
	}
}

func TestShadowQUICOutboundGroupRenameUpdatesJLSReference(t *testing.T) {
	db := setupMihomoSyncTestDB(t, "shadowquic-group-rename-reference.db")
	group := &model.MihomoOutboundGroup{Name: "sq-group-old", Outbounds: "[]", SortOrder: 1}
	if err := db.Create(group).Error; err != nil {
		t.Fatalf("create Mihomo outbound group failed: %v", err)
	}
	inbound := addShadowQUICJLSReference(t, db, "sq-inbound", group.Name)

	payload := mustJSONRaw(t, map[string]interface{}{
		"id":         group.Id,
		"name":       "sq-group-new",
		"outbounds":  "[]",
		"sort_order": group.SortOrder,
	})
	if err := (&MihomoOutboundGroupService{}).Save(db, "edit", payload); err != nil {
		t.Fatalf("rename Mihomo outbound group failed: %v", err)
	}

	assertShadowQUICJLSReference(t, db, inbound.Id, "sq-group-new")
}

func TestShadowQUICManualOutboundSavesRequireCredentials(t *testing.T) {
	db := setupMihomoSyncTestDB(t, "shadowquic-manual-outbound-validation.db")
	payload := mustJSONRaw(t, map[string]interface{}{
		"type":        "shadowquic",
		"tag":         "sq-invalid",
		"server":      "edge.example.com",
		"server_port": 10443,
		"username":    "alice",
	})

	if err := (&MihomoOutboundService{}).Save(db, "new", payload); err == nil || !strings.Contains(err.Error(), "password") {
		t.Fatalf("expected Mihomo ShadowQUIC save to reject missing password, got %v", err)
	}
	if err := (&SubOutboundService{}).Save(db, "new", payload); err == nil || !strings.Contains(err.Error(), "password") {
		t.Fatalf("expected subscription ShadowQUIC save to reject missing password, got %v", err)
	}
}

func TestShadowQUICListenerNormalizesBBRProfile(t *testing.T) {
	listener := map[string]interface{}{
		"type":        "shadowquic",
		"bbr_profile": "AGGRESSIVE",
	}
	normalizeMihomoShadowQUICListener(listener)
	if got := listener["bbr-profile"]; got != "aggressive" {
		t.Fatalf("expected normalized BBR profile, got %#v", got)
	}

	listener = map[string]interface{}{
		"type":        "shadowquic",
		"bbr_profile": "unsupported",
	}
	normalizeMihomoShadowQUICListener(listener)
	if _, exists := listener["bbr-profile"]; exists {
		t.Fatalf("invalid BBR profile must not reach listener output: %#v", listener)
	}
}
