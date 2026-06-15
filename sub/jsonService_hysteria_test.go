package sub

import (
	"encoding/json"
	"testing"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
)

func TestGetJson_NormalizesLegacyHysteriaSubscriptionQUICFields(t *testing.T) {
	setupSubscriptionTestDB(t, "json-hysteria-quic.db")

	db := database.GetDB()
	inbound := model.Inbound{
		Type:  "hysteria",
		Tag:   "hy1-node",
		Addrs: mustRawJSON(t, []interface{}{}),
		OutJson: mustRawJSON(t, map[string]interface{}{
			"type":                  "hysteria",
			"tag":                   "hy1-node",
			"server":                "example.com",
			"server_port":           443,
			"recv_window_conn":      1111,
			"recv_window":           2222,
			"disable_mtu_discovery": true,
		}),
		Options: mustRawJSON(t, map[string]interface{}{
			"stream_receive_window":      25000000,
			"connection_receive_window":  99000000,
			"max_concurrent_streams":     1024,
			"disable_path_mtu_discovery": true,
		}),
	}
	if err := db.Create(&inbound).Error; err != nil {
		t.Fatalf("create inbound failed: %v", err)
	}

	client := model.Client{
		Enable: true,
		Name:   "hy-client",
		Config: mustRawJSON(t, map[string]interface{}{
			"hysteria": map[string]interface{}{
				"auth_str": "secret",
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

	if got := jsonOutbound["stream_receive_window"]; got != float64(25000000) {
		t.Fatalf("expected stream_receive_window from inbound options, got %#v", got)
	}
	if got := jsonOutbound["connection_receive_window"]; got != float64(99000000) {
		t.Fatalf("expected connection_receive_window from inbound options, got %#v", got)
	}
	if got := jsonOutbound["max_concurrent_streams"]; got != float64(1024) {
		t.Fatalf("expected max_concurrent_streams from inbound options, got %#v", got)
	}
	if got, _ := jsonOutbound["disable_path_mtu_discovery"].(bool); !got {
		t.Fatalf("expected disable_path_mtu_discovery=true, got %#v", jsonOutbound["disable_path_mtu_discovery"])
	}
	if _, exists := jsonOutbound["recv_window_conn"]; exists {
		t.Fatalf("legacy recv_window_conn should be removed, got %#v", jsonOutbound["recv_window_conn"])
	}
	if _, exists := jsonOutbound["recv_window"]; exists {
		t.Fatalf("legacy recv_window should be removed, got %#v", jsonOutbound["recv_window"])
	}
	if _, exists := jsonOutbound["disable_mtu_discovery"]; exists {
		t.Fatalf("legacy disable_mtu_discovery should be removed, got %#v", jsonOutbound["disable_mtu_discovery"])
	}
}

func TestGetJson_HysteriaOmitsZeroBandwidthFields(t *testing.T) {
	setupSubscriptionTestDB(t, "json-hysteria-zero-bandwidth.db")

	db := database.GetDB()
	inbound := model.Inbound{
		Type:  "hysteria",
		Tag:   "hy1-zero-bandwidth",
		Addrs: mustRawJSON(t, []interface{}{}),
		OutJson: mustRawJSON(t, map[string]interface{}{
			"type":        "hysteria",
			"tag":         "hy1-zero-bandwidth",
			"server":      "example.com",
			"server_port": 443,
		}),
		Options: mustRawJSON(t, map[string]interface{}{
			"up_mbps":   0,
			"down_mbps": 0,
		}),
	}
	if err := db.Create(&inbound).Error; err != nil {
		t.Fatalf("create inbound failed: %v", err)
	}

	client := model.Client{
		Enable: true,
		Name:   "hy-zero-client",
		Config: mustRawJSON(t, map[string]interface{}{
			"hysteria": map[string]interface{}{
				"auth_str": "secret",
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

	if _, exists := jsonOutbound["up_mbps"]; exists {
		t.Fatalf("expected up_mbps to be omitted when zero, got %#v", jsonOutbound["up_mbps"])
	}
	if _, exists := jsonOutbound["down_mbps"]; exists {
		t.Fatalf("expected down_mbps to be omitted when zero, got %#v", jsonOutbound["down_mbps"])
	}
}
