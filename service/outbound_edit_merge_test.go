package service

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"gorm.io/gorm"
)

func TestOutboundServiceEditPreservesHiddenRawFieldsWhileUpdatingPublicFields(t *testing.T) {
	db := initOutboundEditMergeTestDB(t, "outbound-edit.db")

	original := map[string]interface{}{
		"type":        "hysteria",
		"tag":         "merge-default-node",
		"server":      "1.1.1.1",
		"server_port": float64(443),
		"auth_str":    "token",
		"up_mbps":     float64(100),
		"down_mbps":   float64(100),
		"tls": map[string]interface{}{
			"enabled":            true,
			"server_name":        "old.example",
			"insecure":           false,
			"alpn":               []interface{}{"h3", "h2"},
			"fingerprint":        "AA:BB:CC",
			"client_certificate": []interface{}{"CERT-A", "CERT-B"},
			"client_key":         []interface{}{"KEY-A"},
		},
		"raw_only": map[string]interface{}{
			"note": "keep",
		},
	}

	originalPayload := mustMarshalJSON(t, original)
	service := &OutboundService{}
	if err := service.Save(db, "new", originalPayload); err != nil {
		t.Fatalf("Save new failed: %v", err)
	}

	record := &model.Outbound{}
	if err := db.Where("tag = ?", "merge-default-node").First(record).Error; err != nil {
		t.Fatalf("load record failed: %v", err)
	}

	editPayloadMap := cloneJSONMapForTest(original)
	delete(editPayloadMap, "raw_only")
	editTLS := cloneJSONMapForTest(editPayloadMap["tls"].(map[string]interface{}))
	delete(editTLS, "fingerprint")
	delete(editTLS, "client_certificate")
	delete(editTLS, "client_key")
	editTLS["server_name"] = "new.example"
	editTLS["insecure"] = true
	editPayloadMap["tls"] = editTLS
	editPayloadMap["id"] = float64(record.Id)

	if err := service.Save(db, "edit", mustMarshalJSON(t, editPayloadMap)); err != nil {
		t.Fatalf("Save edit failed: %v", err)
	}

	updated := &model.Outbound{}
	if err := db.Where("tag = ?", "merge-default-node").First(updated).Error; err != nil {
		t.Fatalf("reload record failed: %v", err)
	}

	resolved, err := resolveOutboundJSON(updated)
	if err != nil {
		t.Fatalf("resolveOutboundJSON failed: %v", err)
	}

	resolvedMap := mustDecodeJSONMap(t, resolved)
	if rawOnly, ok := resolvedMap["raw_only"].(map[string]interface{}); !ok || rawOnly["note"] != "keep" {
		t.Fatalf("expected raw_only.note to survive edit, got %#v", resolvedMap["raw_only"])
	}

	tlsMap, ok := resolvedMap["tls"].(map[string]interface{})
	if !ok || tlsMap == nil {
		t.Fatalf("expected tls map, got %#v", resolvedMap["tls"])
	}
	if got, _ := tlsMap["server_name"].(string); got != "new.example" {
		t.Fatalf("expected updated server_name, got %#v", tlsMap["server_name"])
	}
	if got, _ := tlsMap["insecure"].(bool); !got {
		t.Fatalf("expected updated insecure=true, got %#v", tlsMap["insecure"])
	}
	if got, _ := tlsMap["fingerprint"].(string); got != "AA:BB:CC" {
		t.Fatalf("expected hidden fingerprint to survive edit, got %#v", tlsMap["fingerprint"])
	}
	if cert, ok := tlsMap["client_certificate"].([]interface{}); !ok || len(cert) != 2 {
		t.Fatalf("expected hidden client_certificate to survive edit, got %#v", tlsMap["client_certificate"])
	}
	if key, ok := tlsMap["client_key"].([]interface{}); !ok || len(key) != 1 {
		t.Fatalf("expected hidden client_key to survive edit, got %#v", tlsMap["client_key"])
	}
}

func TestSubOutboundServiceEditRebuildsClashProxyWithUIOverridesAndPreservesRawExtras(t *testing.T) {
	db := initOutboundEditMergeTestDB(t, "suboutbound-edit.db")

	original := map[string]interface{}{
		"type":        "trusttunnel",
		"tag":         "merge-sub-node",
		"server":      "2.2.2.2",
		"server_port": float64(443),
		"username":    "alice",
		"password":    "secret",
		"tls": map[string]interface{}{
			"enabled":                       true,
			"server_name":                   "old.example",
			"alpn":                          []interface{}{"h2"},
			"fingerprint":                   "11:22:33",
			"certificate_public_key_sha256": []interface{}{"hash-a"},
		},
		"raw_only": map[string]interface{}{
			"note": "keep",
		},
	}

	record := &model.SubOutbound{}
	if err := record.UnmarshalJSON(mustMarshalJSON(t, original)); err != nil {
		t.Fatalf("SubOutbound.UnmarshalJSON failed: %v", err)
	}
	record.RawOutbound = mustMarshalJSON(t, original)
	record.ClashOptions = mustMarshalJSON(t, map[string]interface{}{
		"name":             "merge-sub-node",
		"type":             "trusttunnel",
		"server":           "2.2.2.2",
		"port":             float64(443),
		"username":         "alice",
		"password":         "secret",
		"udp":              true,
		"health-check":     true,
		"tls":              true,
		"sni":              "old.example",
		"skip-cert-verify": false,
		"fingerprint":      "11:22:33",
		"extra": map[string]interface{}{
			"note": "keep",
		},
	})
	record.RawClashYAML = []byte("  - name: merge-sub-node\n    type: trusttunnel\n")
	if err := db.Save(record).Error; err != nil {
		t.Fatalf("save seed suboutbound failed: %v", err)
	}

	editPayloadMap := cloneJSONMapForTest(original)
	delete(editPayloadMap, "raw_only")
	editTLS := cloneJSONMapForTest(editPayloadMap["tls"].(map[string]interface{}))
	delete(editTLS, "fingerprint")
	delete(editTLS, "certificate_public_key_sha256")
	editTLS["server_name"] = "new.example"
	editTLS["insecure"] = true
	editPayloadMap["tls"] = editTLS
	editPayloadMap["id"] = float64(record.Id)

	service := &SubOutboundService{}
	BeginManagedRuntimeHookScope(db)
	if err := service.Save(db, "edit", mustMarshalJSON(t, editPayloadMap)); err != nil {
		DiscardManagedRuntimeHookScope(db)
		t.Fatalf("Save edit failed: %v", err)
	}
	DiscardManagedRuntimeHookScope(db)

	updated := &model.SubOutbound{}
	if err := db.Where("tag = ?", "merge-sub-node").First(updated).Error; err != nil {
		t.Fatalf("reload suboutbound failed: %v", err)
	}
	if len(updated.RawClashYAML) != 0 {
		t.Fatalf("expected RawClashYAML to be cleared after edit, got %q", string(updated.RawClashYAML))
	}

	resolved, err := resolveSubOutboundJSON(updated)
	if err != nil {
		t.Fatalf("resolveSubOutboundJSON failed: %v", err)
	}
	resolvedMap := mustDecodeJSONMap(t, resolved)
	if rawOnly, ok := resolvedMap["raw_only"].(map[string]interface{}); !ok || rawOnly["note"] != "keep" {
		t.Fatalf("expected raw_only.note to survive edit, got %#v", resolvedMap["raw_only"])
	}

	tlsMap, ok := resolvedMap["tls"].(map[string]interface{})
	if !ok || tlsMap == nil {
		t.Fatalf("expected tls map, got %#v", resolvedMap["tls"])
	}
	if got, _ := tlsMap["server_name"].(string); got != "new.example" {
		t.Fatalf("expected updated server_name, got %#v", tlsMap["server_name"])
	}
	if got, _ := tlsMap["insecure"].(bool); !got {
		t.Fatalf("expected updated insecure=true, got %#v", tlsMap["insecure"])
	}
	if got, _ := tlsMap["fingerprint"].(string); got != "11:22:33" {
		t.Fatalf("expected hidden fingerprint to survive edit, got %#v", tlsMap["fingerprint"])
	}
	if hashes, ok := tlsMap["certificate_public_key_sha256"].([]interface{}); !ok || len(hashes) != 1 {
		t.Fatalf("expected hidden certificate_public_key_sha256 to survive edit, got %#v", tlsMap["certificate_public_key_sha256"])
	}

	proxy := mustDecodeJSONMap(t, updated.ClashOptions)
	if got, _ := proxy["sni"].(string); got != "new.example" {
		t.Fatalf("expected clash proxy sni to use edited value, got %#v", proxy["sni"])
	}
	if got, _ := proxy["skip-cert-verify"].(bool); !got {
		t.Fatalf("expected clash proxy skip-cert-verify=true, got %#v", proxy["skip-cert-verify"])
	}
	if got, _ := proxy["fingerprint"].(string); got != "11:22:33" {
		t.Fatalf("expected clash proxy fingerprint to survive edit, got %#v", proxy["fingerprint"])
	}
	if got, _ := proxy["udp"].(bool); !got {
		t.Fatalf("expected clash proxy udp=true to survive edit, got %#v", proxy["udp"])
	}
	if got, _ := proxy["health-check"].(bool); !got {
		t.Fatalf("expected clash proxy health-check=true to survive edit, got %#v", proxy["health-check"])
	}
	if extra, ok := proxy["extra"].(map[string]interface{}); !ok || extra["note"] != "keep" {
		t.Fatalf("expected clash proxy extra.note to survive edit, got %#v", proxy["extra"])
	}
}

func TestMihomoOutboundServiceEditPreservesHiddenRawFieldsAndUsesUIOverrides(t *testing.T) {
	db := initOutboundEditMergeTestDB(t, "mihomo-edit.db")

	original := map[string]interface{}{
		"type":               "trusttunnel",
		"tag":                "merge-mihomo-node",
		"server":             "6.6.6.6",
		"server_port":        float64(443),
		"username":           "alice",
		"password":           "secret",
		"max_connections":    float64(5),
		"min_streams":        float64(0),
		"max_streams":        float64(0),
		"domain_resolver":    "dns-out",
		"inet4_bind_address": "192.0.2.10",
		"tls": map[string]interface{}{
			"enabled":     true,
			"server_name": "old.example",
			"alpn":        []interface{}{"h2"},
			"fingerprint": "AA:BB:CC",
		},
		mihomoImportedClashProxyKey: map[string]interface{}{
			"name":         "merge-mihomo-node",
			"type":         "trusttunnel",
			"server":       "9.9.9.9",
			"port":         float64(8443),
			"udp":          true,
			"health-check": true,
			"tls":          true,
			"extra":        map[string]interface{}{"note": "raw"},
		},
	}

	record := &model.MihomoOutbound{}
	if err := record.UnmarshalJSON(mustMarshalJSON(t, original)); err != nil {
		t.Fatalf("MihomoOutbound.UnmarshalJSON failed: %v", err)
	}
	record.RawOutbound = mustMarshalJSON(t, original)
	if err := db.Save(record).Error; err != nil {
		t.Fatalf("save seed mihomo outbound failed: %v", err)
	}

	editPayloadMap := cloneJSONMapForTest(original)
	delete(editPayloadMap, "domain_resolver")
	delete(editPayloadMap, "inet4_bind_address")
	delete(editPayloadMap, "max_connections")
	delete(editPayloadMap, "min_streams")
	delete(editPayloadMap, "max_streams")
	delete(editPayloadMap, mihomoImportedClashProxyKey)
	editTLS := cloneJSONMapForTest(editPayloadMap["tls"].(map[string]interface{}))
	delete(editTLS, "fingerprint")
	editTLS["server_name"] = "new.example"
	editTLS["insecure"] = true
	editPayloadMap["tls"] = editTLS
	editPayloadMap["id"] = float64(record.Id)

	service := &MihomoOutboundService{}
	if err := service.Save(db, "edit", mustMarshalJSON(t, editPayloadMap)); err != nil {
		t.Fatalf("Save edit failed: %v", err)
	}

	updated := &model.MihomoOutbound{}
	if err := db.Where("tag = ?", "merge-mihomo-node").First(updated).Error; err != nil {
		t.Fatalf("reload mihomo outbound failed: %v", err)
	}

	resolved, err := resolveMihomoOutboundJSON(updated)
	if err != nil {
		t.Fatalf("resolveMihomoOutboundJSON failed: %v", err)
	}
	resolvedMap := mustDecodeJSONMap(t, resolved)
	if got, _ := resolvedMap["domain_resolver"].(string); got != "dns-out" {
		t.Fatalf("expected domain_resolver to survive edit, got %#v", resolvedMap["domain_resolver"])
	}
	if got, _ := resolvedMap["inet4_bind_address"].(string); got != "192.0.2.10" {
		t.Fatalf("expected inet4_bind_address to survive edit, got %#v", resolvedMap["inet4_bind_address"])
	}
	if _, exists := resolvedMap["max_connections"]; exists {
		t.Fatalf("expected max_connections to be removed when omitted by UI, got %#v", resolvedMap["max_connections"])
	}
	if _, exists := resolvedMap["min_streams"]; exists {
		t.Fatalf("expected min_streams to be removed when omitted by UI, got %#v", resolvedMap["min_streams"])
	}
	if _, exists := resolvedMap["max_streams"]; exists {
		t.Fatalf("expected max_streams to be removed when omitted by UI, got %#v", resolvedMap["max_streams"])
	}

	tlsMap, ok := resolvedMap["tls"].(map[string]interface{})
	if !ok || tlsMap == nil {
		t.Fatalf("expected tls map, got %#v", resolvedMap["tls"])
	}
	if got, _ := tlsMap["server_name"].(string); got != "new.example" {
		t.Fatalf("expected updated server_name, got %#v", tlsMap["server_name"])
	}
	if got, _ := tlsMap["insecure"].(bool); !got {
		t.Fatalf("expected updated insecure=true, got %#v", tlsMap["insecure"])
	}
	if got, _ := tlsMap["fingerprint"].(string); got != "AA:BB:CC" {
		t.Fatalf("expected hidden fingerprint to survive edit, got %#v", tlsMap["fingerprint"])
	}

	rawProxy, ok := resolvedMap[mihomoImportedClashProxyKey].(map[string]interface{})
	if !ok || rawProxy == nil {
		t.Fatalf("expected imported raw clash proxy to survive edit, got %#v", resolvedMap[mihomoImportedClashProxyKey])
	}

	document, err := NewMihomoManagerService().GenerateServerDocument()
	if err != nil {
		t.Fatalf("GenerateServerDocument failed: %v", err)
	}
	proxy := findMihomoProxyByName(document, "merge-mihomo-node")
	if proxy == nil {
		t.Fatalf("expected proxy in generated document")
	}
	if got, _ := proxy["sni"].(string); got != "new.example" {
		t.Fatalf("expected generated proxy sni to use edited value, got %#v", proxy["sni"])
	}
	if got, _ := proxy["skip-cert-verify"].(bool); !got {
		t.Fatalf("expected generated proxy skip-cert-verify=true, got %#v", proxy["skip-cert-verify"])
	}
	if got, _ := proxy["fingerprint"].(string); got != "AA:BB:CC" {
		t.Fatalf("expected generated proxy fingerprint to survive edit, got %#v", proxy["fingerprint"])
	}
	if got, _ := proxy["udp"].(bool); !got {
		t.Fatalf("expected generated proxy udp=true to survive edit, got %#v", proxy["udp"])
	}
	if got, _ := proxy["health-check"].(bool); !got {
		t.Fatalf("expected generated proxy health-check=true to survive edit, got %#v", proxy["health-check"])
	}
	if _, exists := proxy["max-connections"]; exists {
		t.Fatalf("expected generated proxy to omit max-connections when disabled, got %#v", proxy["max-connections"])
	}
	if _, exists := proxy["min-streams"]; exists {
		t.Fatalf("expected generated proxy to omit min-streams when disabled, got %#v", proxy["min-streams"])
	}
	if _, exists := proxy["max-streams"]; exists {
		t.Fatalf("expected generated proxy to omit max-streams when disabled, got %#v", proxy["max-streams"])
	}
	if extra, ok := proxy["extra"].(map[string]interface{}); !ok || extra["note"] != "raw" {
		t.Fatalf("expected generated proxy extra.note to survive edit, got %#v", proxy["extra"])
	}
}

func initOutboundEditMergeTestDB(t *testing.T, filename string) *gorm.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), filename)
	if err := database.InitDB(dbPath); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	db := database.GetDB()
	if sqlDB, err := db.DB(); err == nil && sqlDB != nil {
		t.Cleanup(func() {
			_ = sqlDB.Close()
		})
	}
	return db
}

func mustMarshalJSON(t *testing.T, value interface{}) json.RawMessage {
	t.Helper()

	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	return data
}

func mustDecodeJSONMap(t *testing.T, raw []byte) map[string]interface{} {
	t.Helper()

	payload := map[string]interface{}{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	return payload
}

func cloneJSONMapForTest(src map[string]interface{}) map[string]interface{} {
	if src == nil {
		return map[string]interface{}{}
	}
	dst := make(map[string]interface{}, len(src))
	for key, value := range src {
		switch typed := value.(type) {
		case map[string]interface{}:
			dst[key] = cloneJSONMapForTest(typed)
		case []interface{}:
			cloned := make([]interface{}, 0, len(typed))
			for _, item := range typed {
				if child, ok := item.(map[string]interface{}); ok {
					cloned = append(cloned, cloneJSONMapForTest(child))
					continue
				}
				cloned = append(cloned, item)
			}
			dst[key] = cloned
		default:
			dst[key] = value
		}
	}
	return dst
}
