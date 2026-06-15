package service

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
)

func TestUpsertImportedMihomoOutboundPreservesRawPayloadForServerGeneration(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "mihomo-outbound-import.db")
	if err := database.InitDB(dbPath); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	if sqlDB, err := database.GetDB().DB(); err == nil && sqlDB != nil {
		t.Cleanup(func() {
			_ = sqlDB.Close()
		})
	}

	raw := json.RawMessage(`{
		"type": "trusttunnel",
		"tag": "tt-imported",
		"server": "6.6.6.6",
		"server_port": 443,
		"username": "alice",
		"password": "secret",
		"quic": true,
		"congestion_controller": "bbr",
		"tls": {
			"enabled": true,
			"server_name": "edge.example.com",
			"alpn": ["h2"],
			"fingerprint": "AA:BB:CC",
			"disable_sni": true,
			"utls": {
				"enabled": true,
				"fingerprint": "chrome"
			}
		},
		"_mihomo_clash_proxy": {
			"name": "tt-imported",
			"type": "trusttunnel",
			"server": "9.9.9.9",
			"port": 8443,
			"username": "legacy-user",
			"password": "legacy-pass",
			"tls": true,
			"extra": {
				"note": "raw"
			}
		}
	}`)

	var outbound map[string]interface{}
	if err := json.Unmarshal(raw, &outbound); err != nil {
		t.Fatalf("unmarshal raw failed: %v", err)
	}

	rawClashYAML := []byte("  -   name: tt-imported\n      type: trusttunnel\n      server: 6.6.6.6\n      port: 443\n      username: alice\n      password: secret\n")

	db := database.GetDB()
	if err := upsertImportedMihomoOutbound(db, outbound, raw, rawClashYAML); err != nil {
		t.Fatalf("upsertImportedMihomoOutbound failed: %v", err)
	}

	record := &model.MihomoOutbound{}
	if err := db.Where("tag = ?", "tt-imported").First(record).Error; err != nil {
		t.Fatalf("load mihomo outbound failed: %v", err)
	}
	if len(record.RawOutbound) == 0 {
		t.Fatalf("expected RawOutbound to be stored")
	}
	if got := string(record.RawClashYAML); got != string(rawClashYAML) {
		t.Fatalf("expected RawClashYAML %q, got %q", string(rawClashYAML), got)
	}

	resolved, err := resolveMihomoOutboundJSON(record)
	if err != nil {
		t.Fatalf("resolveMihomoOutboundJSON failed: %v", err)
	}

	var resolvedMap map[string]interface{}
	if err := json.Unmarshal(resolved, &resolvedMap); err != nil {
		t.Fatalf("unmarshal resolved payload failed: %v", err)
	}
	rawProxy, ok := resolvedMap[mihomoImportedClashProxyKey].(map[string]interface{})
	if !ok || rawProxy == nil {
		t.Fatalf("expected hidden raw clash proxy in resolved payload, got %#v", resolvedMap[mihomoImportedClashProxyKey])
	}
	extra, ok := rawProxy["extra"].(map[string]interface{})
	if !ok || extra["note"] != "raw" {
		t.Fatalf("expected raw extra.note=raw, got %#v", rawProxy["extra"])
	}

	outboundService := &MihomoOutboundService{}
	configOutbounds, err := outboundService.GetAllConfig(db)
	if err != nil {
		t.Fatalf("GetAllConfig failed: %v", err)
	}

	foundConfig := false
	for _, item := range configOutbounds {
		decoded := map[string]interface{}{}
		if err := json.Unmarshal(item, &decoded); err != nil {
			t.Fatalf("unmarshal config outbound failed: %v", err)
		}
		if decoded["tag"] != "tt-imported" {
			continue
		}
		tlsMap, ok := decoded["tls"].(map[string]interface{})
		if !ok || tlsMap == nil {
			t.Fatalf("expected tls map in config payload, got %#v", decoded["tls"])
		}
		if got, _ := tlsMap["server_name"].(string); got != "edge.example.com" {
			t.Fatalf("expected server_name edge.example.com, got %#v", tlsMap["server_name"])
		}
		if got, _ := tlsMap["disable_sni"].(bool); !got {
			t.Fatalf("expected disable_sni=true, got %#v", tlsMap["disable_sni"])
		}
		foundConfig = true
		break
	}
	if !foundConfig {
		t.Fatalf("expected imported mihomo outbound in config output")
	}

	document, err := NewMihomoManagerService().GenerateServerDocument()
	if err != nil {
		t.Fatalf("GenerateServerDocument failed: %v", err)
	}

	proxy := findMihomoProxyByName(document, "tt-imported")
	if proxy == nil {
		t.Fatalf("expected tt-imported proxy in generated document")
	}
	if got, _ := proxy["server"].(string); got != "6.6.6.6" {
		t.Fatalf("expected server 6.6.6.6, got %#v", proxy["server"])
	}
	if got := asInt(proxy["port"]); got != 443 {
		t.Fatalf("expected port 443, got %#v", proxy["port"])
	}
	if got, _ := proxy["username"].(string); got != "alice" {
		t.Fatalf("expected username alice, got %#v", proxy["username"])
	}
	if got, _ := proxy["password"].(string); got != "secret" {
		t.Fatalf("expected password secret, got %#v", proxy["password"])
	}
	if got, _ := proxy["sni"].(string); got != "edge.example.com" {
		t.Fatalf("expected sni edge.example.com, got %#v", proxy["sni"])
	}
	if got, _ := proxy["client-fingerprint"].(string); got != "chrome" {
		t.Fatalf("expected client-fingerprint chrome, got %#v", proxy["client-fingerprint"])
	}
	if got, _ := proxy["disable-sni"].(bool); !got {
		t.Fatalf("expected disable-sni=true, got %#v", proxy["disable-sni"])
	}
	generatedExtra, ok := proxy["extra"].(map[string]interface{})
	if !ok || generatedExtra["note"] != "raw" {
		t.Fatalf("expected generated raw extra.note=raw, got %#v", proxy["extra"])
	}
}

func TestMihomoOutboundServiceSaveStoresRawPayloadWithoutID(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "mihomo-outbound-save.db")
	if err := database.InitDB(dbPath); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	if sqlDB, err := database.GetDB().DB(); err == nil && sqlDB != nil {
		t.Cleanup(func() {
			_ = sqlDB.Close()
		})
	}

	service := &MihomoOutboundService{}
	db := database.GetDB()
	payload := json.RawMessage(`{
		"id": 88,
		"type": "trusttunnel",
		"tag": "manual-mihomo-node",
		"server": "1.1.1.1",
		"server_port": 443,
		"username": "manual-user",
		"password": "manual-pass",
		"domain_resolver": "dns-out",
		"tls": {
			"enabled": true,
			"server_name": "manual.example",
			"disable_sni": true
		}
	}`)

	if err := service.Save(db, "new", payload); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	record := &model.MihomoOutbound{}
	if err := db.Where("tag = ?", "manual-mihomo-node").First(record).Error; err != nil {
		t.Fatalf("load mihomo outbound failed: %v", err)
	}
	if len(record.RawOutbound) == 0 {
		t.Fatalf("expected RawOutbound to be stored")
	}

	var stored map[string]interface{}
	if err := json.Unmarshal(record.RawOutbound, &stored); err != nil {
		t.Fatalf("unmarshal RawOutbound failed: %v", err)
	}
	if _, exists := stored["id"]; exists {
		t.Fatalf("expected RawOutbound to omit id, got %#v", stored["id"])
	}
	if got, _ := stored["domain_resolver"].(string); got != "dns-out" {
		t.Fatalf("expected domain_resolver dns-out, got %#v", stored["domain_resolver"])
	}

	resolved, err := resolveMihomoOutboundJSON(record)
	if err != nil {
		t.Fatalf("resolveMihomoOutboundJSON failed: %v", err)
	}

	var resolvedMap map[string]interface{}
	if err := json.Unmarshal(resolved, &resolvedMap); err != nil {
		t.Fatalf("unmarshal resolved payload failed: %v", err)
	}
	if got, _ := resolvedMap["domain_resolver"].(string); got != "dns-out" {
		t.Fatalf("expected resolved domain_resolver dns-out, got %#v", resolvedMap["domain_resolver"])
	}
}

func TestMihomoOutboundServiceEditPreservesImportedRawClashProxyWhenPayloadOmitsHiddenKey(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "mihomo-outbound-edit.db")
	if err := database.InitDB(dbPath); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	if sqlDB, err := database.GetDB().DB(); err == nil && sqlDB != nil {
		t.Cleanup(func() {
			_ = sqlDB.Close()
		})
	}

	importedRaw := json.RawMessage(`{
		"type": "trusttunnel",
		"tag": "tt-edit-imported",
		"server": "6.6.6.6",
		"server_port": 443,
		"username": "alice",
		"password": "secret",
		"tls": {
			"enabled": true,
			"server_name": "edge.example.com"
		},
		"_mihomo_clash_proxy": {
			"name": "tt-edit-imported",
			"type": "trusttunnel",
			"server": "9.9.9.9",
			"port": 8443,
			"extra": {
				"note": "raw"
			}
		}
	}`)

	var imported map[string]interface{}
	if err := json.Unmarshal(importedRaw, &imported); err != nil {
		t.Fatalf("unmarshal imported raw failed: %v", err)
	}

	db := database.GetDB()
	if err := upsertImportedMihomoOutbound(db, imported, importedRaw, nil); err != nil {
		t.Fatalf("upsertImportedMihomoOutbound failed: %v", err)
	}

	record := &model.MihomoOutbound{}
	if err := db.Where("tag = ?", "tt-edit-imported").First(record).Error; err != nil {
		t.Fatalf("load imported outbound failed: %v", err)
	}

	editPayload := json.RawMessage(`{
		"id": 1,
		"type": "trusttunnel",
		"tag": "tt-edit-imported",
		"server": "7.7.7.7",
		"server_port": 9443,
		"username": "edited-user",
		"password": "edited-pass",
		"tls": {
			"enabled": true,
			"server_name": "edited.example.com"
		}
	}`)

	var editMap map[string]interface{}
	if err := json.Unmarshal(editPayload, &editMap); err != nil {
		t.Fatalf("unmarshal edit payload failed: %v", err)
	}
	editMap["id"] = float64(record.Id)
	editPayload, _ = json.Marshal(editMap)

	service := &MihomoOutboundService{}
	if err := service.Save(db, "edit", editPayload); err != nil {
		t.Fatalf("Save edit failed: %v", err)
	}

	updated := &model.MihomoOutbound{}
	if err := db.Where("tag = ?", "tt-edit-imported").First(updated).Error; err != nil {
		t.Fatalf("reload updated outbound failed: %v", err)
	}

	resolved, err := resolveMihomoOutboundJSON(updated)
	if err != nil {
		t.Fatalf("resolveMihomoOutboundJSON failed: %v", err)
	}

	var resolvedMap map[string]interface{}
	if err := json.Unmarshal(resolved, &resolvedMap); err != nil {
		t.Fatalf("unmarshal resolved payload failed: %v", err)
	}
	rawProxy, ok := resolvedMap[mihomoImportedClashProxyKey].(map[string]interface{})
	if !ok || rawProxy == nil {
		t.Fatalf("expected imported raw clash proxy to survive edit, got %#v", resolvedMap[mihomoImportedClashProxyKey])
	}

	document, err := NewMihomoManagerService().GenerateServerDocument()
	if err != nil {
		t.Fatalf("GenerateServerDocument failed: %v", err)
	}

	proxy := findMihomoProxyByName(document, "tt-edit-imported")
	if proxy == nil {
		t.Fatalf("expected tt-edit-imported proxy in generated document")
	}
	if got, _ := proxy["server"].(string); got != "7.7.7.7" {
		t.Fatalf("expected edited server 7.7.7.7, got %#v", proxy["server"])
	}
	if got := asInt(proxy["port"]); got != 9443 {
		t.Fatalf("expected edited port 9443, got %#v", proxy["port"])
	}
	if got, _ := proxy["username"].(string); got != "edited-user" {
		t.Fatalf("expected edited username, got %#v", proxy["username"])
	}
	if got, _ := proxy["sni"].(string); got != "edited.example.com" {
		t.Fatalf("expected edited sni, got %#v", proxy["sni"])
	}
	extra, ok := proxy["extra"].(map[string]interface{})
	if !ok || extra["note"] != "raw" {
		t.Fatalf("expected raw extra.note=raw after edit, got %#v", proxy["extra"])
	}
}

func TestRenderMihomoDocumentYAMLPreservesRawProxyYAML(t *testing.T) {
	document := map[string]interface{}{
		"proxies": []interface{}{
			map[string]interface{}{
				"name":     "raw-node",
				"type":     "trojan",
				"server":   "1.1.1.1",
				"port":     443,
				"password": "p,ass",
			},
		},
		"proxy-groups": []interface{}{
			map[string]interface{}{
				"name":    "AUTO",
				"type":    "select",
				"proxies": []interface{}{"raw-node"},
			},
		},
	}

	rawProxyYAML := "  -   name: raw-node\n      type: trojan\n      server: 1.1.1.1\n      port: 443\n      password: \"p,ass\"\n"
	rendered, err := renderMihomoDocumentYAML(document, map[string][]byte{
		"raw-node": []byte(rawProxyYAML),
	})
	if err != nil {
		t.Fatalf("renderMihomoDocumentYAML failed: %v", err)
	}

	text := string(rendered)
	if !strings.Contains(text, rawProxyYAML) {
		t.Fatalf("expected rendered yaml to contain exact raw proxy yaml:\n%s", text)
	}
}

func findMihomoProxyByName(document map[string]interface{}, name string) map[string]interface{} {
	rawProxies, ok := document["proxies"].([]interface{})
	if !ok {
		return nil
	}

	for _, item := range rawProxies {
		proxy, ok := item.(map[string]interface{})
		if !ok || proxy == nil {
			continue
		}
		if got, _ := proxy["name"].(string); got == name {
			return proxy
		}
	}

	return nil
}

func asInt(value interface{}) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	default:
		return 0
	}
}
