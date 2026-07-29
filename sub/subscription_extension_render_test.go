package sub

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/service"
	"gopkg.in/yaml.v3"
)

func TestCanonicalSubscriptionExtensionDefaults(t *testing.T) {
	jsonExtension, err := service.ParseSubJSONExtension("")
	if err != nil {
		t.Fatalf("parse canonical JSON extension: %v", err)
	}
	inbounds, ok := jsonExtension["inbounds"].([]interface{})
	if !ok || len(inbounds) != 2 {
		t.Fatalf("canonical JSON inbounds = %#v", jsonExtension["inbounds"])
	}
	if inbounds[0].(map[string]interface{})["type"] != "tun" || inbounds[1].(map[string]interface{})["type"] != "mixed" {
		t.Fatalf("canonical JSON inbound types = %#v", inbounds)
	}
	jsonUI := jsonExtension["_uiConfig"].(map[string]interface{})
	if jsonUI["enableSniff"] != false || jsonUI["enableHijackDns"] != false || jsonUI["latencyTestInterval"] != "10m" {
		t.Fatalf("canonical JSON UI defaults = %#v", jsonUI)
	}
	if value, ok := toInt(json.Number("73")); !ok || value != 73 {
		t.Fatalf("toInt(json.Number) = %d, %v", value, ok)
	}

	clashExtension, err := service.ParseSubClashExtension("")
	if err != nil {
		t.Fatalf("parse canonical Clash extension: %v", err)
	}
	tun := clashExtension["tun"].(map[string]interface{})
	dns := clashExtension["dns"].(map[string]interface{})
	clashUI := clashExtension["_uiConfig"].(map[string]interface{})
	if tun["enable"] != true || dns["enable"] != true {
		t.Fatalf("canonical Clash TUN/DNS defaults: tun=%#v dns=%#v", tun, dns)
	}
	if clashUI["enableSniff"] != false || clashUI["latencyTestInterval"] != "180s" {
		t.Fatalf("canonical Clash UI defaults = %#v", clashUI)
	}
	if _, exists := clashExtension["sniffer"]; exists {
		t.Fatalf("canonical Clash extension unexpectedly emits sniffer: %#v", clashExtension["sniffer"])
	}
}

func TestRenderMergedClashSubscriptionDeduplicatesGeneratedNames(t *testing.T) {
	extension := map[string]interface{}{
		"rules": []interface{}{"MATCH,custom"},
		"rule-providers": map[string]interface{}{
			"custom": map[string]interface{}{"type": "http", "behavior": "domain", "url": "https://example.com/custom.yaml"},
		},
		"proxies": []interface{}{
			map[string]interface{}{"name": "same", "type": "http", "server": "old.example"},
			map[string]interface{}{"name": "custom", "type": "http", "server": "custom.example"},
		},
		"proxy-groups": []interface{}{
			map[string]interface{}{"name": "same-group", "type": "select", "proxies": []interface{}{"custom"}},
		},
	}
	generated := map[string]interface{}{
		"proxies": []interface{}{
			map[string]interface{}{"name": "same", "type": "ss", "server": "new.example", "port": 443},
		},
		"proxy-groups": []interface{}{
			map[string]interface{}{"name": "same-group", "type": "select", "proxies": []interface{}{"same"}},
		},
	}
	rendered, err := renderMergedClashSubscription(extension, generated)
	if err != nil {
		t.Fatalf("merge clash subscription: %v", err)
	}
	document := yaml.Node{}
	if err := yaml.Unmarshal([]byte(rendered), &document); err != nil {
		t.Fatalf("parse merged YAML: %v\n%s", err, rendered)
	}
	assertUniqueYAMLMappingKeys(t, document.Content[0])
	root := map[string]interface{}{}
	if err := yaml.Unmarshal([]byte(rendered), &root); err != nil {
		t.Fatalf("decode merged YAML: %v", err)
	}
	proxies := root["proxies"].([]interface{})
	if len(proxies) != 2 {
		t.Fatalf("expected generated override plus custom proxy, got %#v", proxies)
	}
	byName := map[string]map[string]interface{}{}
	for _, raw := range proxies {
		proxy := raw.(map[string]interface{})
		byName[proxy["name"].(string)] = proxy
	}
	if byName["same"]["server"] != "new.example" || byName["custom"] == nil {
		t.Fatalf("generated proxy did not override while preserving custom: %#v", byName)
	}
	if _, exists := root["rules"]; !exists {
		t.Fatal("extension rules were discarded")
	}
	if _, exists := root["rule-providers"]; !exists {
		t.Fatal("extension rule-providers were discarded")
	}
}

func assertUniqueYAMLMappingKeys(t *testing.T, node *yaml.Node) {
	t.Helper()
	if node.Kind == yaml.MappingNode {
		seen := map[string]struct{}{}
		for index := 0; index+1 < len(node.Content); index += 2 {
			key := node.Content[index].Value
			if _, exists := seen[key]; exists {
				t.Fatalf("duplicate YAML mapping key %q", key)
			}
			seen[key] = struct{}{}
			assertUniqueYAMLMappingKeys(t, node.Content[index+1])
		}
		return
	}
	for _, child := range node.Content {
		assertUniqueYAMLMappingKeys(t, child)
	}
}

func TestSubscriptionRenderersPropagateStoredExtensionParseErrors(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "invalid-extensions.db")); err != nil {
		t.Fatalf("init database: %v", err)
	}
	sqlDB, err := database.GetDB().DB()
	if err != nil {
		t.Fatalf("access database handle: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if _, err := (&JsonService{}).SettingService.GetAllSetting(); err != nil {
		t.Fatalf("seed settings: %v", err)
	}
	if err := database.GetDB().Model(&model.Setting{}).Where("key = ?", "subJsonExt").Update("value", "{").Error; err != nil {
		t.Fatalf("store invalid JSON fixture: %v", err)
	}
	outbounds := []map[string]interface{}{}
	tags := []string{}
	if _, err := (&JsonService{}).renderJSONSubscription(&outbounds, &tags, ""); err == nil {
		t.Fatal("invalid stored JSON extension was silently ignored")
	}

	if err := database.GetDB().Model(&model.Setting{}).Where("key = ?", "subClashExt").Update("value", "mixed-port: [").Error; err != nil {
		t.Fatalf("store invalid Clash fixture: %v", err)
	}
	if _, err := (&ClashService{}).loadClashExtension(); err == nil {
		t.Fatal("invalid stored Clash extension was silently ignored")
	}

	if err := database.GetDB().Model(&model.Setting{}).Where("key = ?", "subJsonExt").Update("value", `{"inbounds":[{"listen_port":70000}]}`).Error; err != nil {
		t.Fatalf("store semantically invalid JSON fixture: %v", err)
	}
	if _, err := (&JsonService{}).renderJSONSubscription(&outbounds, &tags, ""); err == nil {
		t.Fatal("semantically invalid stored JSON extension was silently rendered")
	}
	if err := database.GetDB().Model(&model.Setting{}).Where("key = ?", "subClashExt").Update("value", "mixed-port: 70000\n").Error; err != nil {
		t.Fatalf("store semantically invalid Clash fixture: %v", err)
	}
	if _, err := (&ClashService{}).loadClashExtension(); err == nil {
		t.Fatal("semantically invalid stored Clash extension was silently rendered")
	}
}
