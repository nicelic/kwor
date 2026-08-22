package sub

import (
	"encoding/json"
	"path/filepath"
	"strings"
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
	if jsonExtension["route_final"] != "节点选择" || jsonUI["updateMethod"] != "节点选择" || jsonUI["routeFinal"] != "节点选择" || jsonUI["enableSniff"] != false || jsonUI["enableHijackDns"] != false || jsonUI["latencyTestInterval"] != "10m" {
		t.Fatalf("canonical JSON UI defaults = %#v", jsonUI)
	}
	if value, ok := toInt(json.Number("73")); !ok || value != 73 {
		t.Fatalf("toInt(json.Number) = %d, %v", value, ok)
	}

	clashExtension, err := service.ParseSubClashExtension("")
	if err != nil {
		t.Fatalf("parse canonical Clash extension: %v", err)
	}
	if err := service.ValidateSubClashExtension(clashExtension); err != nil {
		t.Fatalf("validate canonical Clash extension: %v", err)
	}
	tun := clashExtension["tun"].(map[string]interface{})
	dns := clashExtension["dns"].(map[string]interface{})
	clashUI := clashExtension["_uiConfig"].(map[string]interface{})
	if tun["enable"] != true || dns["enable"] != true {
		t.Fatalf("canonical Clash TUN/DNS defaults: tun=%#v dns=%#v", tun, dns)
	}
	if clashUI["noResolveGlobal"] != true || clashUI["updateMethod"] != "节点选择" || clashUI["routeFinal"] != "节点选择" || clashUI["enableSniff"] != true || clashUI["snifferOverrideDestination"] != true || clashUI["snifferForceDnsMapping"] != true || clashUI["snifferParsePureIp"] != true || clashUI["enableRejectQuic"] != true || clashUI["latencyTestInterval"] != "180s" {
		t.Fatalf("canonical Clash UI defaults = %#v", clashUI)
	}
	sniffer, ok := clashExtension["sniffer"].(map[string]interface{})
	if !ok || sniffer["enable"] != true || sniffer["override-destination"] != true || sniffer["force-dns-mapping"] != true || sniffer["parse-pure-ip"] != true {
		t.Fatalf("canonical Clash sniffer defaults = %#v", clashExtension["sniffer"])
	}
	if dns["use-system-hosts"] != false {
		t.Fatalf("canonical Clash use-system-hosts default = %#v", dns["use-system-hosts"])
	}
	if _, exists := dns["use-hosts"]; exists {
		t.Fatalf("canonical Clash use-hosts must remain unspecified: %#v", dns["use-hosts"])
	}
	rules, ok := clashExtension["rules"].([]interface{})
	if !ok {
		t.Fatalf("canonical Clash rules = %#v", clashExtension["rules"])
	}
	expectedRejectQuicRules := map[string]bool{
		"AND,((NETWORK,UDP),(DST-PORT,80)),REJECT":   true,
		"AND,((NETWORK,UDP),(DST-PORT,443)),REJECT":  true,
		"AND,((NETWORK,UDP),(DST-PORT,2443)),REJECT": true,
		"AND,((NETWORK,UDP),(DST-PORT,4443)),REJECT": true,
		"AND,((NETWORK,UDP),(DST-PORT,6443)),REJECT": true,
		"AND,((NETWORK,UDP),(DST-PORT,8080)),REJECT": true,
		"AND,((NETWORK,UDP),(DST-PORT,8081)),REJECT": true,
		"AND,((NETWORK,UDP),(DST-PORT,8443)),REJECT": true,
	}
	for _, value := range rules {
		if rule, ok := value.(string); ok {
			delete(expectedRejectQuicRules, rule)
		}
	}
	if len(expectedRejectQuicRules) != 0 {
		t.Fatalf("canonical Clash missing QUIC UDP reject rules: %#v", expectedRejectQuicRules)
	}
}

func TestJSONSubscriptionRenderCanonicalizesDoHPath(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "json-doh-path.db")); err != nil {
		t.Fatalf("init database: %v", err)
	}
	sqlDB, err := database.GetDB().DB()
	if err != nil {
		t.Fatalf("access database handle: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	settingService := &service.SettingService{}
	if _, err := settingService.GetAllSetting(); err != nil {
		t.Fatalf("seed settings: %v", err)
	}
	if err := database.GetDB().Model(&model.Setting{}).Where("key = ?", "subJsonExt").Update("value", `{
  "dns": {
    "servers": [
      {"tag":"direct-dns","type":"h3","server":"dns.example","server_port":443,"path":"dns-query"}
    ]
  }
}`).Error; err != nil {
		t.Fatalf("store JSON subscription extension: %v", err)
	}

	outbounds := []map[string]interface{}{}
	tags := []string{}
	rendered, err := (&JsonService{}).renderJSONSubscription(&outbounds, &tags, "")
	if err != nil {
		t.Fatalf("render JSON subscription: %v", err)
	}
	root := map[string]interface{}{}
	if err := json.Unmarshal([]byte(rendered), &root); err != nil {
		t.Fatalf("decode rendered JSON subscription: %v", err)
	}
	dns := root["dns"].(map[string]interface{})
	servers := dns["servers"].([]interface{})
	if len(servers) != 1 {
		t.Fatalf("rendered DNS servers = %#v", servers)
	}
	server := servers[0].(map[string]interface{})
	if path, _ := server["path"].(string); path != "/dns-query" {
		t.Fatalf("rendered DoH3 path = %q, want /dns-query", path)
	}
}

func TestClashSubscriptionExtensionIntervalBounds(t *testing.T) {
	valid, err := service.ParseSubClashExtension(`_uiConfig:
  latencyTestInterval: 30s
  updateInterval: 1h
rule-providers:
  domain:
    type: http
    behavior: domain
    format: yaml
    url: https://example.com/domain.yaml
    interval: 3600
`)
	if err != nil {
		t.Fatalf("parse valid Clash interval extension: %v", err)
	}
	if err := service.ValidateSubClashExtension(valid); err != nil {
		t.Fatalf("validate minimum Clash intervals: %v", err)
	}

	tooShort, err := service.ParseSubClashExtension(`_uiConfig:
  latencyTestInterval: 1s
  updateInterval: 30m
`)
	if err != nil {
		t.Fatalf("parse low Clash interval extension: %v", err)
	}
	if err := service.ValidateSubClashExtension(tooShort); err == nil {
		t.Fatal("low Clash intervals unexpectedly passed validation")
	}

	if got := parseMihomoLatencyIntervalSeconds("30s"); got != 30 {
		t.Fatalf("minimum runtime latency interval = %d, want 30", got)
	}
	if got := parseMihomoLatencyIntervalSeconds("1s"); got != 0 {
		t.Fatalf("low runtime latency interval = %d, want 0", got)
	}
	oversized := "x: \"" + strings.Repeat("a", service.SubscriptionClashExtensionMaxBytes) + "\"\n"
	if _, err := service.ParseSubClashExtension(oversized); err == nil {
		t.Fatal("oversized historical Clash source unexpectedly entered the parser")
	}
}

func TestExplicitClashExtensionOptionsAreNotRestored(t *testing.T) {
	extension, err := service.ParseSubClashExtension(`mixed-port: 7890
dns:
  enable: true
  use-system-hosts: true
  use-hosts: false
  default-nameserver:
    - udp://223.5.5.5
  nameserver:
    - "udp://8.8.8.8#节点选择"
sniffer:
  enable: false
  force-dns-mapping: false
  parse-pure-ip: false
  override-destination: false
rules:
  - MATCH,节点选择
_uiConfig:
  enableRejectQuic: false
`)
	if err != nil {
		t.Fatalf("parse explicitly disabled Clash extension: %v", err)
	}
	if err := service.ValidateSubClashExtension(extension); err != nil {
		t.Fatalf("validate explicitly disabled Clash extension: %v", err)
	}

	rendered, err := renderMergedClashSubscription(extension, map[string]interface{}{})
	if err != nil {
		t.Fatalf("render explicitly disabled Clash extension: %v", err)
	}
	root := map[string]interface{}{}
	if err := yaml.Unmarshal([]byte(rendered), &root); err != nil {
		t.Fatalf("decode explicitly disabled Clash extension: %v", err)
	}

	sniffer, ok := root["sniffer"].(map[string]interface{})
	if !ok || sniffer["enable"] != false {
		t.Fatalf("disabled sniffer was changed during render: %#v", root["sniffer"])
	}
	if sniffer["force-dns-mapping"] != false || sniffer["parse-pure-ip"] != false || sniffer["override-destination"] != false {
		t.Fatalf("explicit sniffer options were changed during render: %#v", sniffer)
	}
	dns, ok := root["dns"].(map[string]interface{})
	if !ok || dns["use-system-hosts"] != true || dns["use-hosts"] != false {
		t.Fatalf("explicit DNS host options were changed during render: %#v", root["dns"])
	}
	rules, ok := root["rules"].([]interface{})
	if !ok {
		t.Fatalf("explicitly disabled Clash rules = %#v", root["rules"])
	}
	for _, blockedRule := range []string{
		"AND,((NETWORK,UDP),(DST-PORT,80)),REJECT",
		"AND,((NETWORK,UDP),(DST-PORT,443)),REJECT",
		"AND,((NETWORK,UDP),(DST-PORT,2443)),REJECT",
		"AND,((NETWORK,UDP),(DST-PORT,4443)),REJECT",
		"AND,((NETWORK,UDP),(DST-PORT,6443)),REJECT",
		"AND,((NETWORK,UDP),(DST-PORT,8080)),REJECT",
		"AND,((NETWORK,UDP),(DST-PORT,8081)),REJECT",
		"AND,((NETWORK,UDP),(DST-PORT,8443)),REJECT",
	} {
		for _, value := range rules {
			if rule, ok := value.(string); ok && rule == blockedRule {
				t.Fatalf("disabled Clash extension unexpectedly restored %q", blockedRule)
			}
		}
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
