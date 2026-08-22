package service

import (
	"encoding/json"
	"testing"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
)

func TestSettingServiceInitialSingboxDNSDefaultIsPersistedOnce(t *testing.T) {
	settingService := initPanelSQLiteSettingTestDB(t)

	readConfig := func() map[string]interface{} {
		t.Helper()

		var stored model.Setting
		if err := database.GetDB().Where("key = ?", "config").First(&stored).Error; err != nil {
			t.Fatalf("load config setting: %v", err)
		}
		config := map[string]interface{}{}
		if err := json.Unmarshal([]byte(stored.Value), &config); err != nil {
			t.Fatalf("decode config setting: %v", err)
		}
		return config
	}

	initial := readConfig()
	dns, ok := initial["dns"].(map[string]interface{})
	if !ok {
		t.Fatalf("initial config has no dns object: %#v", initial)
	}
	servers, ok := dns["servers"].([]interface{})
	if !ok || len(servers) != 1 {
		t.Fatalf("initial DNS server count = %d, want 1: %#v", len(servers), dns["servers"])
	}
	server, ok := servers[0].(map[string]interface{})
	if !ok {
		t.Fatalf("initial DNS server is not an object: %#v", servers[0])
	}
	if server["type"] != "tls" || server["tag"] != "tls_1.1.1.1" || server["server"] != "1.1.1.1" {
		t.Fatalf("unexpected initial DNS server: %#v", server)
	}
	if server["server_port"] != float64(853) {
		t.Fatalf("initial DNS server port = %#v, want 853", server["server_port"])
	}
	tls, ok := server["tls"].(map[string]interface{})
	if !ok || tls["enabled"] != true || tls["server_name"] != "1.1.1.1" {
		t.Fatalf("unexpected initial DNS TLS settings: %#v", server["tls"])
	}
	if dns["final"] != "tls_1.1.1.1" {
		t.Fatalf("initial dns.final = %#v, want tls_1.1.1.1", dns["final"])
	}

	modified := `{
  "dns": {
    "servers": [
      {
        "type": "udp",
        "tag": "custom-dns",
        "server": "9.9.9.9",
        "server_port": 53
      }
    ],
    "rules": [],
    "final": "custom-dns"
  }
}`
	if err := settingService.SaveSetting("config", modified); err != nil {
		t.Fatalf("save modified DNS config: %v", err)
	}
	if _, err := settingService.GetAllSetting(); err != nil {
		t.Fatalf("reload settings after modifying DNS config: %v", err)
	}
	modifiedConfig := readConfig()
	modifiedDNS, _ := modifiedConfig["dns"].(map[string]interface{})
	modifiedServers, _ := modifiedDNS["servers"].([]interface{})
	modifiedServer, _ := modifiedServers[0].(map[string]interface{})
	if modifiedServer["tag"] != "custom-dns" || modifiedDNS["final"] != "custom-dns" {
		t.Fatalf("modified DNS config was overwritten: %#v", modifiedDNS)
	}

	deleted := `{
  "dns": {
    "servers": [],
    "rules": []
  }
}`
	if err := settingService.SaveSetting("config", deleted); err != nil {
		t.Fatalf("save deleted DNS config: %v", err)
	}
	if _, err := settingService.GetAllSetting(); err != nil {
		t.Fatalf("reload settings after deleting DNS config: %v", err)
	}
	deletedConfig := readConfig()
	deletedDNS, _ := deletedConfig["dns"].(map[string]interface{})
	deletedServers, _ := deletedDNS["servers"].([]interface{})
	if len(deletedServers) != 0 {
		t.Fatalf("deleted DNS server was restored: %#v", deletedDNS["servers"])
	}
	if _, exists := deletedDNS["final"]; exists {
		t.Fatalf("deleted DNS final tag was restored: %#v", deletedDNS["final"])
	}
}
