package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
)

func TestNormalizeSingboxDNSConfig_RemovesLegacyServerStrategy(t *testing.T) {
	config := &ProManagerSingBoxConfig{
		Dns: json.RawMessage(`{
			"independent_cache": true,
			"servers": [
				{
					"type": "tls",
					"tag": "tls_1.1.1.1",
					"server": "1.1.1.1",
					"server_port": 853,
					"strategy": "ipv4_only",
					"tls": {
						"enabled": true
					}
				}
			],
			"strategy": "prefer_ipv4",
			"final": "tls_1.1.1.1"
		}`),
	}

	if err := normalizeSingboxDNSConfig(config); err != nil {
		t.Fatalf("normalizeSingboxDNSConfig failed: %v", err)
	}

	var dnsMap map[string]interface{}
	if err := json.Unmarshal(config.Dns, &dnsMap); err != nil {
		t.Fatalf("unmarshal normalized dns failed: %v", err)
	}

	if got, _ := dnsMap["strategy"].(string); got != "prefer_ipv4" {
		t.Fatalf("expected top-level dns.strategy preserved, got %q", got)
	}
	if _, exists := dnsMap["independent_cache"]; exists {
		t.Fatalf("expected deprecated independent_cache removed, got %#v", dnsMap)
	}

	servers, ok := dnsMap["servers"].([]interface{})
	if !ok || len(servers) != 1 {
		t.Fatalf("expected one dns server, got %#v", dnsMap["servers"])
	}
	serverMap, ok := servers[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected dns server object, got %#v", servers[0])
	}
	if _, exists := serverMap["strategy"]; exists {
		t.Fatalf("expected legacy dns server strategy removed, got %#v", serverMap)
	}
}

func TestNormalizeSingboxDNSConfig_RejectsLegacyRuleStrategyWithQueryType(t *testing.T) {
	config := &ProManagerSingBoxConfig{
		Dns: json.RawMessage(`{
			"rules": [
				{
					"query_type": ["A"],
					"action": "route",
					"server": "dns-main"
				},
				{
					"action": "route",
					"server": "dns-main",
					"strategy": "prefer_ipv4"
				}
			]
		}`),
	}

	err := normalizeSingboxDNSConfig(config)
	if err == nil {
		t.Fatal("expected dns rule compatibility error, got nil")
	}
	if got := err.Error(); got == "" || !containsAll(got, []string{"ip_version/query_type", "strategy"}) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNormalizeSingboxDNSConfig_RejectsLegacyAddressFilterWithIPVersion(t *testing.T) {
	config := &ProManagerSingBoxConfig{
		Dns: json.RawMessage(`{
			"rules": [
				{
					"ip_version": 4,
					"action": "route",
					"server": "dns-main"
				},
				{
					"action": "route",
					"server": "dns-main",
					"ip_cidr": ["1.1.1.1/32"]
				}
			]
		}`),
	}

	err := normalizeSingboxDNSConfig(config)
	if err == nil {
		t.Fatal("expected dns rule compatibility error, got nil")
	}
	if got := err.Error(); got == "" || !containsAll(got, []string{"ip_cidr", "match_response"}) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestNormalizeSingboxDNSConfig_AllowsResponseAddressFilterWithQueryType(t *testing.T) {
	config := &ProManagerSingBoxConfig{
		Dns: json.RawMessage(`{
			"rules": [
				{
					"query_type": ["A"],
					"action": "route",
					"server": "dns-main"
				},
				{
					"action": "route",
					"server": "dns-main",
					"match_response": true,
					"ip_cidr": ["1.1.1.1/32"]
				}
			]
		}`),
	}

	if err := normalizeSingboxDNSConfig(config); err != nil {
		t.Fatalf("expected response-matching address filter to remain valid, got %v", err)
	}
}

func TestSettingServiceGetConfig_SanitizesDeprecatedDNSFields(t *testing.T) {
	settingService := initPanelSQLiteSettingTestDB(t)

	raw := `{
		"dns": {
			"independent_cache": true,
			"servers": [
				{
					"type": "tls",
					"tag": "dns-main",
					"strategy": "ipv4_only"
				}
			]
		}
	}`
	if err := settingService.saveSetting("config", raw); err != nil {
		t.Fatalf("seed raw config failed: %v", err)
	}

	value, err := settingService.GetConfig()
	if err != nil {
		t.Fatalf("GetConfig failed: %v", err)
	}

	assertDeprecatedDNSFieldsRemoved(t, json.RawMessage(value))
}

func TestSettingServiceSaveConfig_SanitizesDeprecatedDNSFields(t *testing.T) {
	settingService := initPanelSQLiteSettingTestDB(t)

	tx := database.GetDB().Begin()
	if tx.Error != nil {
		t.Fatalf("begin tx failed: %v", tx.Error)
	}

	raw := json.RawMessage(`{
		"dns": {
			"independent_cache": true,
			"servers": [
				{
					"type": "tls",
					"tag": "dns-main",
					"strategy": "ipv4_only"
				}
			]
		}
	}`)
	if err := settingService.SaveConfig(tx, raw); err != nil {
		tx.Rollback()
		t.Fatalf("SaveConfig failed: %v", err)
	}
	if err := tx.Commit().Error; err != nil {
		t.Fatalf("commit config tx failed: %v", err)
	}

	var setting model.Setting
	if err := database.GetDB().Where("key = ?", "config").First(&setting).Error; err != nil {
		t.Fatalf("load saved config failed: %v", err)
	}

	assertDeprecatedDNSFieldsRemoved(t, json.RawMessage(setting.Value))
}

func TestSettingServiceSaveConfig_RejectsIncompatibleDNSRules(t *testing.T) {
	settingService := initPanelSQLiteSettingTestDB(t)

	tx := database.GetDB().Begin()
	if tx.Error != nil {
		t.Fatalf("begin tx failed: %v", tx.Error)
	}
	defer tx.Rollback()

	err := settingService.SaveConfig(tx, json.RawMessage(`{
		"dns": {
			"rules": [
				{
					"query_type": ["A"],
					"action": "route",
					"server": "dns-main"
				},
				{
					"action": "route",
					"server": "dns-main",
					"strategy": "prefer_ipv4"
				}
			]
		}
	}`))
	if err == nil {
		t.Fatal("expected SaveConfig to reject incompatible dns rules")
	}
	if got := err.Error(); got == "" || !containsAll(got, []string{"ip_version/query_type", "strategy"}) {
		t.Fatalf("unexpected SaveConfig error: %v", err)
	}
}

func containsAll(value string, parts []string) bool {
	for _, part := range parts {
		if !strings.Contains(value, part) {
			return false
		}
	}
	return true
}

func assertDeprecatedDNSFieldsRemoved(t *testing.T, raw json.RawMessage) {
	t.Helper()

	var configMap map[string]interface{}
	if err := json.Unmarshal(raw, &configMap); err != nil {
		t.Fatalf("unmarshal config failed: %v", err)
	}

	dnsMap, ok := configMap["dns"].(map[string]interface{})
	if !ok || dnsMap == nil {
		t.Fatalf("expected dns object, got %#v", configMap["dns"])
	}

	if _, exists := dnsMap["independent_cache"]; exists {
		t.Fatalf("expected independent_cache removed, got %#v", dnsMap)
	}

	servers, ok := dnsMap["servers"].([]interface{})
	if !ok || len(servers) != 1 {
		t.Fatalf("expected one dns server, got %#v", dnsMap["servers"])
	}
	serverMap, ok := servers[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected dns server object, got %#v", servers[0])
	}
	if _, exists := serverMap["strategy"]; exists {
		t.Fatalf("expected dns server strategy removed, got %#v", serverMap)
	}
}
