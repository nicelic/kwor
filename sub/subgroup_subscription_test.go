package sub

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"gopkg.in/yaml.v3"
	"gorm.io/gorm"
)

func TestGetSubGroupJsonReturnsEmptyConfigForEmptyGroup(t *testing.T) {
	db := initSubGroupSubscriptionTestDB(t, "subgroup-empty-json.db")

	group := &model.SubGroup{
		Name:      "empty-group",
		Outbounds: "[]",
	}
	if err := db.Create(group).Error; err != nil {
		t.Fatalf("create subgroup failed: %v", err)
	}

	result, err := (&SubManagerSubService{}).GetSubGroupJson(group.Name)
	if err != nil {
		t.Fatalf("GetSubGroupJson failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected JSON subscription payload, got nil")
	}
	if !strings.Contains(*result, "\"outbounds\"") {
		t.Fatalf("expected outbounds block in JSON payload, got:\n%s", *result)
	}
}

func TestGetSubGroupClashReturnsEmptyConfigForEmptyGroup(t *testing.T) {
	db := initSubGroupSubscriptionTestDB(t, "subgroup-empty-clash.db")

	group := &model.SubGroup{
		Name:      "empty-group",
		Outbounds: "[]",
	}
	if err := db.Create(group).Error; err != nil {
		t.Fatalf("create subgroup failed: %v", err)
	}

	result, err := (&SubManagerSubService{}).GetSubGroupClash(group.Name)
	if err != nil {
		t.Fatalf("GetSubGroupClash failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected Clash subscription payload, got nil")
	}
	if !strings.Contains(*result, "proxies:\n") {
		t.Fatalf("expected proxies section in Clash payload, got:\n%s", *result)
	}
	if !strings.Contains(*result, "proxy-groups:") {
		t.Fatalf("expected proxy-groups section in Clash payload, got:\n%s", *result)
	}
}

func TestGetSubGroupJsonPreservesConfiguredOutboundOrder(t *testing.T) {
	db := initSubGroupSubscriptionTestDB(t, "subgroup-ordered-json.db")

	createOrderedTestSubOutbound(t, db, "node-a", "1.1.1.1", 1001)
	createOrderedTestSubOutbound(t, db, "node-b", "2.2.2.2", 1002)

	group := &model.SubGroup{
		Name:      "ordered-json-group",
		Outbounds: `["node-b","node-a"]`,
	}
	if err := db.Create(group).Error; err != nil {
		t.Fatalf("create subgroup failed: %v", err)
	}

	result, err := (&SubManagerSubService{}).GetSubGroupJson(group.Name)
	if err != nil {
		t.Fatalf("GetSubGroupJson failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected JSON subscription payload, got nil")
	}

	var payload struct {
		Outbounds []map[string]interface{} `json:"outbounds"`
	}
	if err := json.Unmarshal([]byte(*result), &payload); err != nil {
		t.Fatalf("unmarshal JSON payload failed: %v", err)
	}

	got := collectOrderedJSONOutboundTags(payload.Outbounds, "node-a", "node-b")
	want := []string{"node-b", "node-a"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("expected JSON outbound order %v, got %v", want, got)
	}
}

func TestGetSubGroupClashPreservesConfiguredOutboundOrder(t *testing.T) {
	db := initSubGroupSubscriptionTestDB(t, "subgroup-ordered-clash.db")

	createOrderedTestSubOutbound(t, db, "node-a", "1.1.1.1", 1001)
	createOrderedTestSubOutbound(t, db, "node-b", "2.2.2.2", 1002)

	group := &model.SubGroup{
		Name:      "ordered-clash-group",
		Outbounds: `["node-b","node-a"]`,
	}
	if err := db.Create(group).Error; err != nil {
		t.Fatalf("create subgroup failed: %v", err)
	}

	result, err := (&SubManagerSubService{}).GetSubGroupClash(group.Name)
	if err != nil {
		t.Fatalf("GetSubGroupClash failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected Clash subscription payload, got nil")
	}

	var payload map[string]interface{}
	if err := yaml.Unmarshal([]byte(*result), &payload); err != nil {
		t.Fatalf("unmarshal Clash payload failed: %v", err)
	}

	proxiesRaw, _ := payload["proxies"].([]interface{})
	got := collectOrderedClashProxyNames(proxiesRaw, "node-a", "node-b")
	want := []string{"node-b", "node-a"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("expected Clash proxy order %v, got %v", want, got)
	}
}

func TestGetSubGroupClashPreservesMixedSourceOutboundOrder(t *testing.T) {
	db := initSubGroupSubscriptionTestDB(t, "subgroup-mixed-source-clash.db")

	runtimeOnly := &model.SubOutbound{
		Type:       "trojan",
		Tag:        "m_hy2_hk3",
		SourceType: "mihomo_client",
		RawOutbound: mustJSONRawMessage(t, map[string]interface{}{
			"type":        "trojan",
			"tag":         "m_hy2_hk3",
			"server":      "mihomo-first.example.com",
			"server_port": 443,
			"password":    "mihomo-pass",
			"tls": map[string]interface{}{
				"enabled": true,
			},
		}),
	}
	if err := db.Create(runtimeOnly).Error; err != nil {
		t.Fatalf("create runtime-only suboutbound failed: %v", err)
	}

	storedProxy := &model.SubOutbound{
		Type:       "trojan",
		Tag:        "s_hy1_hk3",
		SourceType: "client",
		RawOutbound: mustJSONRawMessage(t, map[string]interface{}{
			"type":        "trojan",
			"tag":         "s_hy1_hk3",
			"server":      "stored-second.example.com",
			"server_port": 8443,
			"password":    "stored-pass",
			"tls": map[string]interface{}{
				"enabled": true,
			},
		}),
		ClashOptions: mustJSONRawMessage(t, map[string]interface{}{
			"name":     "s_hy1_hk3",
			"type":     "trojan",
			"server":   "stored-second.example.com",
			"port":     8443,
			"password": "stored-pass",
			"sni":      "stored-second.example.com",
		}),
	}
	if err := db.Create(storedProxy).Error; err != nil {
		t.Fatalf("create stored-proxy suboutbound failed: %v", err)
	}

	group := &model.SubGroup{
		Name:      "mixed-source-group",
		Outbounds: `["m_hy2_hk3","s_hy1_hk3"]`,
	}
	if err := db.Create(group).Error; err != nil {
		t.Fatalf("create subgroup failed: %v", err)
	}

	result, err := (&SubManagerSubService{}).GetSubGroupClash(group.Name)
	if err != nil {
		t.Fatalf("GetSubGroupClash failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected Clash subscription payload, got nil")
	}

	var payload map[string]interface{}
	if err := yaml.Unmarshal([]byte(*result), &payload); err != nil {
		t.Fatalf("unmarshal Clash payload failed: %v", err)
	}

	proxiesRaw, _ := payload["proxies"].([]interface{})
	got := collectOrderedClashProxyNames(proxiesRaw, "m_hy2_hk3", "s_hy1_hk3")
	want := []string{"m_hy2_hk3", "s_hy1_hk3"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("expected mixed-source Clash proxy order %v, got %v", want, got)
	}
}

func TestGetSubGroupJson_PreservesStoredSHA256ForImportedSubgroupNode(t *testing.T) {
	db := initSubGroupSubscriptionTestDB(t, "subgroup-preserve-sha256-json.db")

	group := &model.SubGroup{
		Name:      "imported-json-group",
		Outbounds: `["hy1-node"]`,
	}
	if err := db.Create(group).Error; err != nil {
		t.Fatalf("create subgroup failed: %v", err)
	}

	record := &model.SubOutbound{
		Type:       "hysteria",
		Tag:        "hy1-node",
		SourceType: subManagerSourceSubGroup,
		RawOutbound: mustJSONRawMessage(t, map[string]interface{}{
			"type":        "hysteria",
			"tag":         "hy1-node",
			"server":      "1.2.3.4",
			"server_port": 443,
			"auth_str":    "secret",
			"tls": map[string]interface{}{
				"enabled": true,
				"certificate_public_key_sha256": []interface{}{
					"stored-sha256-value",
				},
			},
		}),
	}
	if err := db.Create(record).Error; err != nil {
		t.Fatalf("create subgroup imported suboutbound failed: %v", err)
	}

	result, err := (&SubManagerSubService{}).GetSubGroupJson(group.Name)
	if err != nil {
		t.Fatalf("GetSubGroupJson failed: %v", err)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal([]byte(*result), &payload); err != nil {
		t.Fatalf("unmarshal subgroup json failed: %v", err)
	}
	jsonOutbound := findTaggedOutbound(t, payload["outbounds"], "hy1-node")
	jsonTLS := asMap(t, jsonOutbound["tls"])
	hashes := asStringSliceValue(t, jsonTLS["certificate_public_key_sha256"])
	if len(hashes) != 1 || hashes[0] != "stored-sha256-value" {
		t.Fatalf("expected imported subgroup json to preserve stored certificate_public_key_sha256, got %#v", jsonTLS["certificate_public_key_sha256"])
	}
}

func createOrderedTestSubOutbound(t *testing.T, db *gorm.DB, tag string, server string, port int) {
	t.Helper()

	record := &model.SubOutbound{
		Type:        "shadowsocks",
		Tag:         tag,
		RawOutbound: mustJSONRawMessage(t, map[string]interface{}{"type": "shadowsocks", "tag": tag, "server": server, "server_port": port, "method": "aes-128-gcm", "password": "test-pass"}),
		ClashOptions: mustJSONRawMessage(t, map[string]interface{}{
			"name":     tag,
			"type":     "ss",
			"server":   server,
			"port":     port,
			"cipher":   "aes-128-gcm",
			"password": "test-pass",
		}),
	}
	if err := db.Create(record).Error; err != nil {
		t.Fatalf("create suboutbound %s failed: %v", tag, err)
	}
}

func mustJSONRawMessage(t *testing.T, value interface{}) json.RawMessage {
	t.Helper()

	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal JSON failed: %v", err)
	}
	return data
}

func collectOrderedJSONOutboundTags(outbounds []map[string]interface{}, wanted ...string) []string {
	allowed := make(map[string]struct{}, len(wanted))
	for _, tag := range wanted {
		allowed[tag] = struct{}{}
	}

	result := make([]string, 0, len(wanted))
	for _, outbound := range outbounds {
		tag, _ := outbound["tag"].(string)
		if _, ok := allowed[tag]; !ok {
			continue
		}
		result = append(result, tag)
	}
	return result
}

func collectOrderedClashProxyNames(proxies []interface{}, wanted ...string) []string {
	allowed := make(map[string]struct{}, len(wanted))
	for _, tag := range wanted {
		allowed[tag] = struct{}{}
	}

	result := make([]string, 0, len(wanted))
	for _, raw := range proxies {
		proxy, ok := raw.(map[string]interface{})
		if !ok || proxy == nil {
			continue
		}
		name, _ := proxy["name"].(string)
		if _, ok := allowed[name]; !ok {
			continue
		}
		result = append(result, name)
	}
	return result
}

func initSubGroupSubscriptionTestDB(t *testing.T, filename string) *gorm.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), filename)
	if err := database.InitDB(dbPath); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}

	db := database.GetDB()
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db.DB failed: %v", err)
	}
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	return db
}
