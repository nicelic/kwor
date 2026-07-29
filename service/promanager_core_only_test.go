package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alireza0/s-ui/database/model"
)

func TestSaveInboundJsonWritesOnlyFinalCoreConfig(t *testing.T) {
	db := setupManagedRuntimeFileStoreTestDB(t, "promanager-core-only.db")

	tlsConfig := &model.Tls{
		Name: "core-only-tls",
		Server: json.RawMessage(`{
			"enabled": true,
			"server_name": "core-only.example.com",
			"certificate_path": "/tmp/core-only-cert.pem",
			"key_path": "/tmp/core-only-key.pem"
		}`),
		Client: json.RawMessage(`{"enabled":true,"server_name":"core-only.example.com"}`),
	}
	if err := db.Create(tlsConfig).Error; err != nil {
		t.Fatalf("create TLS preset failed: %v", err)
	}

	normalInbound := &model.Inbound{
		Type:    "trojan",
		Tag:     "core-only-trojan",
		TlsId:   tlsConfig.Id,
		Addrs:   json.RawMessage(`[]`),
		OutJson: json.RawMessage(`{}`),
		Options: json.RawMessage(`{"listen":"::","listen_port":443}`),
	}
	shadowInbound := &model.Inbound{
		Type:    "shadowtls",
		Tag:     "core-only-shadowtls",
		Addrs:   json.RawMessage(`[]`),
		OutJson: json.RawMessage(`{}`),
		Options: json.RawMessage(`{
			"listen":"::",
			"listen_port":444,
			"version":3,
			"handshake":{"server":"example.com","server_port":443},
			"ss_config":{"network":"tcp","method":"2022-blake3-aes-128-gcm","password":"inner-secret"}
		}`),
	}
	if err := db.Create(normalInbound).Error; err != nil {
		t.Fatalf("create normal inbound failed: %v", err)
	}
	if err := db.Create(shadowInbound).Error; err != nil {
		t.Fatalf("create ShadowTLS inbound failed: %v", err)
	}

	client := &model.Client{
		Enable: true,
		Name:   "core-only-user",
		Config: json.RawMessage(`{
			"trojan":{"name":"core-only-user","password":"trojan-secret"},
			"shadowtls":{"name":"core-only-user","password":"shadow-secret"}
		}`),
		Inbounds: json.RawMessage(`[1,2]`),
		Links:    json.RawMessage(`[]`),
	}
	client.Inbounds, _ = json.Marshal([]uint{normalInbound.Id, shadowInbound.Id})
	if err := db.Create(client).Error; err != nil {
		t.Fatalf("create client failed: %v", err)
	}

	if err := db.Create(&model.Outbound{
		Type:    "socks",
		Tag:     "core-only-upstream",
		Options: json.RawMessage(`{"server":"127.0.0.1","server_port":1080}`),
	}).Error; err != nil {
		t.Fatalf("create outbound failed: %v", err)
	}
	if err := db.Create(&model.Service{
		Type:    "resolved",
		Tag:     "core-only-service",
		Options: json.RawMessage(`{"listen":"127.0.0.1","listen_port":5353}`),
	}).Error; err != nil {
		t.Fatalf("create service failed: %v", err)
	}
	if err := db.Create(&model.Endpoint{
		Type:    "wireguard",
		Tag:     "core-only-endpoint",
		Options: json.RawMessage(`{"address":["10.0.0.1/32"],"private_key":"test-key"}`),
	}).Error; err != nil {
		t.Fatalf("create endpoint failed: %v", err)
	}

	manager := &ProManagerService{ConfigService: &ConfigService{}}
	manager.SaveInboundJson()

	coreData, err := ManagedRuntimeReadFile(GetSingboxConfigPath())
	if err != nil {
		t.Fatalf("read final sing-box config failed: %v", err)
	}
	var config ProManagerSingBoxConfig
	if err := json.Unmarshal(coreData, &config); err != nil {
		t.Fatalf("unmarshal final sing-box config failed: %v", err)
	}

	inbounds := decodeTaggedRuntimeObjects(t, config.Inbounds)
	normal := inbounds[normalInbound.Tag]
	if normal == nil {
		t.Fatalf("normal inbound %s missing from final config", normalInbound.Tag)
	}
	if _, ok := normal["tls"].(map[string]interface{}); !ok {
		t.Fatalf("normal inbound TLS missing from final config: %#v", normal)
	}
	if users, ok := normal["users"].([]interface{}); !ok || len(users) != 1 {
		t.Fatalf("normal inbound users missing from final config: %#v", normal["users"])
	}

	shadow := inbounds[shadowInbound.Tag]
	if shadow == nil || shadow["detour"] != shadowInbound.Tag+"-in" {
		t.Fatalf("ShadowTLS outer inbound missing or not linked: %#v", shadow)
	}
	if users, ok := shadow["users"].([]interface{}); !ok || len(users) != 1 {
		t.Fatalf("ShadowTLS users missing from final config: %#v", shadow["users"])
	}
	shadowInner := inbounds[shadowInbound.Tag+"-in"]
	if shadowInner == nil || shadowInner["type"] != "shadowsocks" || shadowInner["password"] != "inner-secret" {
		t.Fatalf("ShadowTLS inner inbound missing from final config: %#v", shadowInner)
	}

	outbounds := decodeTaggedRuntimeObjects(t, config.Outbounds)
	if outbounds["core-only-upstream"] == nil {
		t.Fatal("configured outbound missing from final config")
	}
	services := decodeTaggedRuntimeObjects(t, config.Services)
	if services["core-only-service"] == nil {
		t.Fatal("configured service missing from final config")
	}
	endpoints := decodeTaggedRuntimeObjects(t, config.Endpoints)
	if endpoints["core-only-endpoint"] == nil {
		t.Fatal("configured endpoint missing from final config")
	}
	var route map[string]interface{}
	if err := json.Unmarshal(config.Route, &route); err != nil || len(route) == 0 {
		t.Fatalf("route missing from final config: route=%s err=%v", config.Route, err)
	}

	var obsoleteCount int64
	obsoleteWhere := `lower(ext) = '.json' AND (` +
		`path = 'Inbound' OR path LIKE 'Inbound/%' OR ` +
		`path = 'outbound' OR path LIKE 'outbound/%' OR ` +
		`path = 'sub_manager' OR path LIKE 'sub\_manager/%' ESCAPE '\' OR ` +
		`path = 'sub_json' OR path LIKE 'sub\_json/%' ESCAPE '\')`
	if err := db.Table(managedRuntimeFileTable).Where(obsoleteWhere).Count(&obsoleteCount).Error; err != nil {
		t.Fatalf("count obsolete managed runtime rows failed: %v", err)
	}
	if obsoleteCount != 0 {
		t.Fatalf("expected no obsolete managed runtime JSON rows, got %d", obsoleteCount)
	}

	var coreCount int64
	if err := db.Table(managedRuntimeFileTable).
		Where("path = ?", "core/singbox/config.json").
		Count(&coreCount).Error; err != nil {
		t.Fatalf("count final core config row failed: %v", err)
	}
	if coreCount != 1 {
		t.Fatalf("expected exactly one final core config row, got %d", coreCount)
	}

	for _, root := range obsoleteManagedRuntimeJSONRoots {
		dirPath := filepath.Join(configDataDirForRuntimeTest(), root)
		entries, readErr := os.ReadDir(dirPath)
		if readErr != nil {
			if os.IsNotExist(readErr) {
				continue
			}
			t.Fatalf("read obsolete runtime directory %s failed: %v", root, readErr)
		}
		for _, entry := range entries {
			if !entry.IsDir() && strings.EqualFold(filepath.Ext(entry.Name()), ".json") {
				t.Fatalf("unexpected obsolete disk JSON remains: %s", filepath.Join(dirPath, entry.Name()))
			}
		}
	}
}

func decodeTaggedRuntimeObjects(t *testing.T, rawItems []json.RawMessage) map[string]map[string]interface{} {
	t.Helper()
	result := make(map[string]map[string]interface{}, len(rawItems))
	for _, raw := range rawItems {
		var item map[string]interface{}
		if err := json.Unmarshal(raw, &item); err != nil {
			t.Fatalf("decode runtime object failed: %v", err)
		}
		tag, _ := item["tag"].(string)
		if tag != "" {
			result[tag] = item
		}
	}
	return result
}
