package service

import (
	"encoding/json"
	"testing"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
)

func TestMigrateLegacySingboxDNSServersMovesCardsOutOfConfig(t *testing.T) {
	settingService := initPanelSQLiteSettingTestDB(t)
	raw := "{\n" +
		"\t\"dns\": {\n" +
		"\t\t\"final\": \"dns-selected\",\n" +
		"\t\t\"servers\": [\n" +
		"\t\t\t{\"type\":\"tls\",\"tag\":\"dns-idle\",\"server\":\"1.1.1.1\",\"server_port\":853},\n" +
		"\t\t\t{\"type\":\"https\",\"tag\":\"dns-selected\",\"server\":\"1.0.0.1\",\"server_port\":443,\"path\":\"/dns-query\"}\n" +
		"\t\t],\n" +
		"\t\t\"rules\": []\n" +
		"\t}\n" +
		"}"
	if err := settingService.saveSetting("config", raw); err != nil {
		t.Fatalf("seed legacy config: %v", err)
	}

	migrated, err := MigrateLegacySingboxDNSServers()
	if err != nil {
		t.Fatalf("migrate legacy DNS servers: %v", err)
	}
	if !migrated {
		t.Fatal("expected legacy DNS servers to be migrated")
	}

	servers := make([]model.DnsServer, 0)
	if err := database.GetDB().Order("id ASC").Find(&servers).Error; err != nil {
		t.Fatalf("load migrated DNS servers: %v", err)
	}
	if len(servers) != 2 || servers[0].Tag != "dns-idle" || servers[1].Tag != "dns-selected" {
		t.Fatalf("unexpected migrated DNS servers: %#v", servers)
	}

	var stored model.Setting
	if err := database.GetDB().Where("key = ?", "config").First(&stored).Error; err != nil {
		t.Fatalf("load migrated config setting: %v", err)
	}
	root := map[string]interface{}{}
	if err := json.Unmarshal([]byte(stored.Value), &root); err != nil {
		t.Fatalf("decode migrated config: %v", err)
	}
	dns, _ := root["dns"].(map[string]interface{})
	if _, exists := dns["servers"]; exists {
		t.Fatalf("legacy dns.servers must be removed from settings: %#v", dns)
	}
	if final, _ := dns["final"].(string); final != "dns-selected" {
		t.Fatalf("unexpected final DNS tag: %q", final)
	}
}

func TestMigrateLegacySingboxDNSServersImportsMultipleCardsBeforeValidation(t *testing.T) {
	settingService := initPanelSQLiteSettingTestDB(t)
	raw := `{"dns":{"final":"dns-main","servers":[
		{"type":"udp","tag":"dns-bootstrap","server":"1.1.1.1","server_port":53},
		{"type":"udp","tag":"dns-main","server":"8.8.8.8","server_port":53}
	],"rules":[]}}`
	if err := settingService.saveSetting("config", raw); err != nil {
		t.Fatalf("seed multi-card legacy config: %v", err)
	}

	migrated, err := MigrateLegacySingboxDNSServers()
	if err != nil || !migrated {
		t.Fatalf("multi-card DNS migration failed: migrated=%v err=%v", migrated, err)
	}
	var migratedServer model.DnsServer
	if err := database.GetDB().Where("tag = ?", "dns-bootstrap").First(&migratedServer).Error; err != nil {
		t.Fatalf("load migrated bootstrap DNS card: %v", err)
	}
}

func TestGenerateFullConfigIncludesBootstrapAndFinalDNSServer(t *testing.T) {
	settingService := initPanelSQLiteSettingTestDB(t)
	raw := "{\"dns\":{\"final\":\"dns-selected\",\"rules\":[{\"action\":\"route\",\"server\":\"dns-idle\",\"domain_suffix\":[\"example.com\"]}]}}"
	if err := settingService.saveSetting("config", raw); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	servers := []model.DnsServer{
		{Type: "tls", Tag: "dns-idle", Options: json.RawMessage("{\"server\":\"1.1.1.1\",\"server_port\":853}")},
		{Type: "https", Tag: "dns-selected", Options: json.RawMessage("{\"server\":\"1.0.0.1\",\"server_port\":443,\"path\":\"/dns-query\"}")},
	}
	if err := database.GetDB().Create(&servers).Error; err != nil {
		t.Fatalf("create DNS cards: %v", err)
	}

	config, err := (&ProManagerService{ConfigService: &ConfigService{}}).GenerateFullConfig()
	if err != nil {
		t.Fatalf("generate full config: %v", err)
	}

	dns := map[string]interface{}{}
	if err := json.Unmarshal(config.Dns, &dns); err != nil {
		t.Fatalf("decode generated dns config: %v", err)
	}
	runtimeServers, ok := dns["servers"].([]interface{})
	if !ok || len(runtimeServers) != 2 {
		t.Fatalf("expected bootstrap and final runtime DNS servers, got %#v", dns["servers"])
	}
	bootstrap, _ := runtimeServers[0].(map[string]interface{})
	if tag, _ := bootstrap["tag"].(string); tag != singboxRuntimeBootstrapDNSTag {
		t.Fatalf("runtime bootstrap DNS tag = %q, want %q", tag, singboxRuntimeBootstrapDNSTag)
	}
	if server, _ := bootstrap["server"].(string); server != "8.8.8.8" {
		t.Fatalf("runtime bootstrap DNS server = %q, want 8.8.8.8", server)
	}
	if port, _ := bootstrap["server_port"].(float64); port != 53 {
		t.Fatalf("runtime bootstrap DNS port = %v, want 53", port)
	}
	server, _ := runtimeServers[1].(map[string]interface{})
	if tag, _ := server["tag"].(string); tag != "dns-selected" {
		t.Fatalf("runtime DNS tag = %q, want dns-selected", tag)
	}
	if resolver, _ := server["domain_resolver"].(string); resolver != singboxRuntimeBootstrapDNSTag {
		t.Fatalf("runtime DNS domain_resolver = %q, want %q", resolver, singboxRuntimeBootstrapDNSTag)
	}
	rules, _ := dns["rules"].([]interface{})
	rule, _ := rules[0].(map[string]interface{})
	if target, _ := rule["server"].(string); target != "dns-selected" {
		t.Fatalf("runtime DNS rule target = %q, want dns-selected", target)
	}
}

func TestDnsServerServiceRejectsBootstrapDNSTag(t *testing.T) {
	server := &model.DnsServer{
		Type:    "udp",
		Tag:     singboxRuntimeBootstrapDNSTag,
		Options: json.RawMessage(`{"server":"8.8.8.8","server_port":53}`),
	}
	if err := validateSingboxDNSServerPayload(server); err == nil {
		t.Fatal("expected bootstrap DNS tag to be reserved")
	}
}

func TestDnsServerServiceRejectsDeletingSelectedServer(t *testing.T) {
	settingService := initPanelSQLiteSettingTestDB(t)
	if err := settingService.saveSetting("config", "{\"dns\":{\"final\":\"dns-final\",\"rules\":[]}}"); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	server := model.DnsServer{
		Type:    "tls",
		Tag:     "dns-final",
		Options: json.RawMessage("{\"server\":\"1.1.1.1\",\"server_port\":853}"),
	}
	if err := database.GetDB().Create(&server).Error; err != nil {
		t.Fatalf("create selected DNS server: %v", err)
	}

	tx := database.GetDB().Begin()
	if tx.Error != nil {
		t.Fatalf("begin transaction: %v", tx.Error)
	}
	defer tx.Rollback()

	payload, err := json.Marshal(server.Id)
	if err != nil {
		t.Fatalf("marshal delete payload: %v", err)
	}
	if err := (&DnsServerService{}).Save(tx, "del", payload); err == nil {
		t.Fatal("expected selected DNS server deletion to be rejected")
	}
}
