package service

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
)

func TestSingboxUserManagedProtocolsAreSelectable(t *testing.T) {
	for _, test := range []struct {
		inboundType      string
		shadowTLSVersion uint
	}{
		{inboundType: "mixed"},
		{inboundType: "socks"},
		{inboundType: "http"},
		{inboundType: "shadowsocks"},
		{inboundType: "vmess"},
		{inboundType: "vless"},
		{inboundType: "trojan"},
		{inboundType: "naive"},
		{inboundType: "hysteria"},
		{inboundType: "shadowtls", shadowTLSVersion: 3},
		{inboundType: "tuic"},
		{inboundType: "hysteria2"},
		{inboundType: "anytls"},
		{inboundType: "ssh"},
	} {
		t.Run(test.inboundType, func(t *testing.T) {
			management := buildSingboxInboundUserManagement(test.inboundType, test.shadowTLSVersion)
			if !management.Selectable {
				t.Fatalf("%s must be selectable in sing-box client management: %#v", test.inboundType, management)
			}
		})
	}

	for _, test := range []struct {
		inboundType      string
		shadowTLSVersion uint
	}{
		{inboundType: "shadowtls", shadowTLSVersion: 2},
		{inboundType: "direct"},
		{inboundType: "redirect"},
		{inboundType: "tproxy"},
		{inboundType: "tun"},
		{inboundType: "unknown"},
	} {
		management := buildSingboxInboundUserManagement(test.inboundType, test.shadowTLSVersion)
		if management.Selectable {
			t.Fatalf("%s must not be selectable in sing-box client management: %#v", test.inboundType, management)
		}
	}
}

func TestInboundServiceGetAllPublishesSingboxUserManagementWithoutPersistingIt(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "singbox-inbound-user-management.db")
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

	mixed := model.Inbound{
		Type:    "mixed",
		Tag:     "mixed-user-management",
		Options: json.RawMessage(`{"listen":"::","listen_port":18080}`),
	}
	ssh := model.Inbound{
		Type:    "ssh",
		Tag:     "ssh-subscription-only",
		Options: json.RawMessage(`{"listen":"::","listen_port":22}`),
	}
	shadowTLS := model.Inbound{
		Type: "shadowtls",
		Tag:  "shadowtls-string-v3",
		Options: json.RawMessage(`{
			"listen":"::",
			"listen_port":24443,
			"version":"3",
			"handshake":{"server":"example.com","server_port":443}
		}`),
	}
	direct := model.Inbound{
		Type:    "direct",
		Tag:     "direct-no-users",
		Options: json.RawMessage(`{"listen":"::","listen_port":0}`),
	}
	for _, inbound := range []*model.Inbound{&mixed, &ssh, &shadowTLS, &direct} {
		if err := db.Create(inbound).Error; err != nil {
			t.Fatalf("create %s inbound failed: %v", inbound.Type, err)
		}
	}

	client := model.Client{
		Enable:   true,
		Name:     "alice",
		Config:   json.RawMessage(`{"mixed":{"username":"alice","password":"secret"}}`),
		Inbounds: json.RawMessage(fmt.Sprintf("[%d,%d,%d]", mixed.Id, ssh.Id, shadowTLS.Id)),
		Links:    json.RawMessage(`[]`),
	}
	if err := db.Create(&client).Error; err != nil {
		t.Fatalf("create client failed: %v", err)
	}

	items, err := (&InboundService{}).GetAll()
	if err != nil {
		t.Fatalf("GetAll failed: %v", err)
	}
	byTag := make(map[string]map[string]interface{}, len(*items))
	for _, item := range *items {
		byTag[item["tag"].(string)] = item
	}

	assertSelectable := func(tag string, usesUsersField bool) {
		t.Helper()
		item := byTag[tag]
		management, ok := item["user_management"].(SingboxInboundUserManagement)
		if !ok {
			t.Fatalf("inbound %s user_management has unexpected type: %#v", tag, item["user_management"])
		}
		if !management.Selectable || management.UsesUsersField != usesUsersField {
			t.Fatalf("inbound %s has unexpected user management: %#v", tag, management)
		}
		users, ok := item["users"].([]string)
		if !ok || len(users) != 1 || users[0] != "alice" {
			t.Fatalf("inbound %s users = %#v, want [alice]", tag, item["users"])
		}
	}
	assertSelectable(mixed.Tag, true)
	assertSelectable(ssh.Tag, false)
	assertSelectable(shadowTLS.Tag, true)

	directManagement, ok := byTag[direct.Tag]["user_management"].(SingboxInboundUserManagement)
	if !ok || directManagement.Selectable {
		t.Fatalf("direct inbound must remain non-selectable: %#v", byTag[direct.Tag]["user_management"])
	}
	if _, exists := byTag[direct.Tag]["users"]; exists {
		t.Fatalf("direct inbound must not expose a users list: %#v", byTag[direct.Tag])
	}
}

func TestInboundServiceGetAllConfigRebuildsWhitelistedRuntimeUsers(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "singbox-runtime-users.db")
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

	inbound := model.Inbound{
		Type: "mixed",
		Tag:  "mixed-runtime-clean",
		Options: json.RawMessage(`{
			"listen": "::",
			"listen_port": 18081,
			"route_tag": "mixed-runtime-clean",
			"user_management": {"selectable": true},
			"metadata": {"source": "legacy-panel"},
			"users": [{"username": "legacy", "password": "legacy", "unexpected": "drop"}]
		}`),
	}
	if err := db.Create(&inbound).Error; err != nil {
		t.Fatalf("create inbound failed: %v", err)
	}

	client := model.Client{
		Enable: true,
		Name:   "alice",
		Config: json.RawMessage(`{
			"mixed": {
				"name": "alice",
				"password": "secret",
				"metadata": {"source": "client"},
				"user_management": {"selectable": true},
				"unexpected": "drop"
			}
		}`),
		Inbounds: json.RawMessage(fmt.Sprintf("[%d]", inbound.Id)),
		Links:    json.RawMessage(`[]`),
	}
	if err := db.Create(&client).Error; err != nil {
		t.Fatalf("create client failed: %v", err)
	}

	configs, err := (&InboundService{}).GetAllConfig(db)
	if err != nil {
		t.Fatalf("GetAllConfig failed: %v", err)
	}
	if len(configs) != 1 {
		t.Fatalf("expected one runtime inbound, got %d", len(configs))
	}

	runtimeInbound := map[string]interface{}{}
	if err := json.Unmarshal(configs[0], &runtimeInbound); err != nil {
		t.Fatalf("decode runtime inbound failed: %v", err)
	}
	for _, key := range []string{"route_tag", "user_management", "metadata"} {
		if _, exists := runtimeInbound[key]; exists {
			t.Fatalf("runtime inbound leaked %q: %#v", key, runtimeInbound)
		}
	}

	users, ok := runtimeInbound["users"].([]interface{})
	if !ok || len(users) != 1 {
		t.Fatalf("runtime users = %#v, want one user", runtimeInbound["users"])
	}
	runtimeUser, ok := users[0].(map[string]interface{})
	if !ok {
		t.Fatalf("runtime user has unexpected type: %#v", users[0])
	}
	wantUser := map[string]interface{}{"username": "alice", "password": "secret"}
	if len(runtimeUser) != len(wantUser) {
		t.Fatalf("runtime user leaked fields: %#v", runtimeUser)
	}
	for key, want := range wantUser {
		if got := runtimeUser[key]; got != want {
			t.Fatalf("runtime user %s = %#v, want %#v", key, got, want)
		}
	}
}
