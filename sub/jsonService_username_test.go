package sub

import (
	"encoding/json"
	"testing"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
)

func TestGetJson_SkipsUnsupportedVmessUsername(t *testing.T) {
	setupSubscriptionTestDB(t, "json-vmess-username.db")

	db := database.GetDB()
	inbound := model.Inbound{
		Type:    "vmess",
		Tag:     "vmess-node",
		Addrs:   mustRawJSON(t, []interface{}{}),
		OutJson: mustRawJSON(t, map[string]interface{}{"type": "vmess", "tag": "vmess-node", "server": "example.com", "server_port": 443}),
		Options: mustRawJSON(t, map[string]interface{}{}),
	}
	if err := db.Create(&inbound).Error; err != nil {
		t.Fatalf("create inbound failed: %v", err)
	}

	client := model.Client{
		Enable: true,
		Name:   "alice",
		Config: mustRawJSON(t, map[string]interface{}{
			"vmess": map[string]interface{}{
				"username": "client",
				"uuid":     "8502a444-ed92-4e42-be73-000000000001",
			},
		}),
		Inbounds: mustRawJSON(t, []uint{inbound.Id}),
		Links:    mustRawJSON(t, []map[string]string{}),
	}
	if err := db.Create(&client).Error; err != nil {
		t.Fatalf("create client failed: %v", err)
	}

	jsonSub, _, err := (&JsonService{}).GetJson(client.Name, "json")
	if err != nil {
		t.Fatalf("GetJson failed: %v", err)
	}

	var jsonDoc map[string]interface{}
	if err := json.Unmarshal([]byte(*jsonSub), &jsonDoc); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	jsonOutbound := findTaggedOutbound(t, jsonDoc["outbounds"], inbound.Tag)
	if _, exists := jsonOutbound["username"]; exists {
		t.Fatalf("expected vmess username to be omitted, got %#v", jsonOutbound["username"])
	}
	if got, _ := jsonOutbound["uuid"].(string); got != "8502a444-ed92-4e42-be73-000000000001" {
		t.Fatalf("expected vmess uuid to be preserved, got %#v", jsonOutbound["uuid"])
	}
}

func TestGetJson_StripsLegacyOutboundUsername(t *testing.T) {
	setupSubscriptionTestDB(t, "json-legacy-vmess-username.db")

	db := database.GetDB()
	inbound := model.Inbound{
		Type:  "vmess",
		Tag:   "vmess-legacy-node",
		Addrs: mustRawJSON(t, []interface{}{}),
		OutJson: mustRawJSON(t, map[string]interface{}{
			"type":        "vmess",
			"tag":         "vmess-legacy-node",
			"server":      "example.com",
			"server_port": 443,
			"username":    "legacy-client",
			"uuid":        "8502a444-ed92-4e42-be73-000000000002",
		}),
		Options: mustRawJSON(t, map[string]interface{}{}),
	}
	if err := db.Create(&inbound).Error; err != nil {
		t.Fatalf("create inbound failed: %v", err)
	}

	client := model.Client{
		Enable:   true,
		Name:     "carol",
		Config:   mustRawJSON(t, map[string]interface{}{"vmess": map[string]interface{}{}}),
		Inbounds: mustRawJSON(t, []uint{inbound.Id}),
		Links:    mustRawJSON(t, []map[string]string{}),
	}
	if err := db.Create(&client).Error; err != nil {
		t.Fatalf("create client failed: %v", err)
	}

	jsonSub, _, err := (&JsonService{}).GetJson(client.Name, "json")
	if err != nil {
		t.Fatalf("GetJson failed: %v", err)
	}

	var jsonDoc map[string]interface{}
	if err := json.Unmarshal([]byte(*jsonSub), &jsonDoc); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	jsonOutbound := findTaggedOutbound(t, jsonDoc["outbounds"], inbound.Tag)
	if _, exists := jsonOutbound["username"]; exists {
		t.Fatalf("expected legacy vmess username to be stripped, got %#v", jsonOutbound["username"])
	}
}

func TestGetJson_KeepsNaiveUsername(t *testing.T) {
	setupSubscriptionTestDB(t, "json-naive-username.db")

	db := database.GetDB()
	inbound := model.Inbound{
		Type:    "naive",
		Tag:     "naive-node",
		Addrs:   mustRawJSON(t, []interface{}{}),
		OutJson: mustRawJSON(t, map[string]interface{}{"type": "naive", "tag": "naive-node", "server": "example.com", "server_port": 443}),
		Options: mustRawJSON(t, map[string]interface{}{}),
	}
	if err := db.Create(&inbound).Error; err != nil {
		t.Fatalf("create inbound failed: %v", err)
	}

	client := model.Client{
		Enable: true,
		Name:   "bob",
		Config: mustRawJSON(t, map[string]interface{}{
			"naive": map[string]interface{}{
				"username": "client",
				"password": "secret",
			},
		}),
		Inbounds: mustRawJSON(t, []uint{inbound.Id}),
		Links:    mustRawJSON(t, []map[string]string{}),
	}
	if err := db.Create(&client).Error; err != nil {
		t.Fatalf("create client failed: %v", err)
	}

	jsonSub, _, err := (&JsonService{}).GetJson(client.Name, "json")
	if err != nil {
		t.Fatalf("GetJson failed: %v", err)
	}

	var jsonDoc map[string]interface{}
	if err := json.Unmarshal([]byte(*jsonSub), &jsonDoc); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	jsonOutbound := findTaggedOutbound(t, jsonDoc["outbounds"], inbound.Tag)
	if got, _ := jsonOutbound["username"].(string); got != "client" {
		t.Fatalf("expected naive username to be preserved, got %#v", jsonOutbound["username"])
	}
	if got, _ := jsonOutbound["password"].(string); got != "secret" {
		t.Fatalf("expected naive password to be preserved, got %#v", jsonOutbound["password"])
	}
}
