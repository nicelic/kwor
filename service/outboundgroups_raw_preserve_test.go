package service

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
)

func TestUpsertImportedOutboundPreservesRawPayloadForConfigGeneration(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "outbound-import.db")
	if err := database.InitDB(dbPath); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	if sqlDB, err := database.GetDB().DB(); err == nil && sqlDB != nil {
		t.Cleanup(func() {
			_ = sqlDB.Close()
		})
	}

	raw := json.RawMessage(`{
		"type": "hysteria",
		"tag": "s_hy1_out_sg_out_v4",
		"server": "77.93.90.104",
		"server_port": 45880,
		"auth_str": "token-value",
		"up_mbps": 1000,
		"down_mbps": 1000,
		"recv_window": 88000000,
		"recv_window_conn": 25000000,
		"tls": {
			"enabled": true,
			"server_name": "aaa.cc",
			"alpn": ["h3", "h2", "http/1.1"],
			"certificate_public_key_sha256": ["YxTLpzLzgad+pcDBCc4bo5x/pdVWM3XXxgdVYqpUUHA="],
			"client_certificate": ["CERT-LINE-1", "CERT-LINE-2"],
			"client_key": ["KEY-LINE-1", "KEY-LINE-2"]
		}
	}`)

	var outboundMap map[string]interface{}
	if err := json.Unmarshal(raw, &outboundMap); err != nil {
		t.Fatalf("unmarshal raw failed: %v", err)
	}

	db := database.GetDB()
	if err := upsertImportedOutbound(db, outboundMap, raw); err != nil {
		t.Fatalf("upsertImportedOutbound failed: %v", err)
	}

	record := &model.Outbound{}
	if err := db.Where("tag = ?", "s_hy1_out_sg_out_v4").First(record).Error; err != nil {
		t.Fatalf("load outbound failed: %v", err)
	}

	resolved, err := resolveOutboundJSON(record)
	if err != nil {
		t.Fatalf("resolveOutboundJSON failed: %v", err)
	}

	var got map[string]interface{}
	if err := json.Unmarshal(resolved, &got); err != nil {
		t.Fatalf("unmarshal resolved failed: %v", err)
	}

	tlsMap, ok := got["tls"].(map[string]interface{})
	if !ok || tlsMap == nil {
		t.Fatalf("expected tls map, got %#v", got["tls"])
	}

	if gotValue, _ := tlsMap["server_name"].(string); gotValue != "aaa.cc" {
		t.Fatalf("expected server_name aaa.cc, got %#v", tlsMap["server_name"])
	}

	expectedALPN := []interface{}{"h3", "h2", "http/1.1"}
	if !reflect.DeepEqual(tlsMap["alpn"], expectedALPN) {
		t.Fatalf("expected alpn %#v, got %#v", expectedALPN, tlsMap["alpn"])
	}

	expectedHash := []interface{}{"YxTLpzLzgad+pcDBCc4bo5x/pdVWM3XXxgdVYqpUUHA="}
	if !reflect.DeepEqual(tlsMap["certificate_public_key_sha256"], expectedHash) {
		t.Fatalf("expected certificate_public_key_sha256 %#v, got %#v", expectedHash, tlsMap["certificate_public_key_sha256"])
	}

	expectedClientCert := []interface{}{"CERT-LINE-1", "CERT-LINE-2"}
	if !reflect.DeepEqual(tlsMap["client_certificate"], expectedClientCert) {
		t.Fatalf("expected client_certificate %#v, got %#v", expectedClientCert, tlsMap["client_certificate"])
	}

	expectedClientKey := []interface{}{"KEY-LINE-1", "KEY-LINE-2"}
	if !reflect.DeepEqual(tlsMap["client_key"], expectedClientKey) {
		t.Fatalf("expected client_key %#v, got %#v", expectedClientKey, tlsMap["client_key"])
	}

	outboundService := &OutboundService{}
	configOutbounds, err := outboundService.GetAllConfig(db)
	if err != nil {
		t.Fatalf("GetAllConfig failed: %v", err)
	}
	if len(configOutbounds) != 2 {
		t.Fatalf("expected 2 outbounds including default direct, got %d", len(configOutbounds))
	}

	var imported map[string]interface{}
	found := false
	for _, item := range configOutbounds {
		var decoded map[string]interface{}
		if err := json.Unmarshal(item, &decoded); err != nil {
			t.Fatalf("unmarshal config outbound failed: %v", err)
		}
		if decoded["tag"] == "s_hy1_out_sg_out_v4" {
			imported = decoded
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected imported outbound in config output")
	}

	importedTLS, ok := imported["tls"].(map[string]interface{})
	if !ok || importedTLS == nil {
		t.Fatalf("expected imported tls map, got %#v", imported["tls"])
	}
	if gotValue, _ := importedTLS["server_name"].(string); gotValue != "aaa.cc" {
		t.Fatalf("expected imported config server_name aaa.cc, got %#v", importedTLS["server_name"])
	}
	if !reflect.DeepEqual(importedTLS["client_certificate"], expectedClientCert) {
		t.Fatalf("expected imported config client_certificate %#v, got %#v", expectedClientCert, importedTLS["client_certificate"])
	}
}

func TestOutboundServiceSaveStoresRawPayloadWithoutID(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "outbound-save.db")
	if err := database.InitDB(dbPath); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	if sqlDB, err := database.GetDB().DB(); err == nil && sqlDB != nil {
		t.Cleanup(func() {
			_ = sqlDB.Close()
		})
	}

	service := &OutboundService{}
	db := database.GetDB()
	payload := json.RawMessage(`{
		"id": 99,
		"type": "hysteria",
		"tag": "manual-node",
		"server": "1.1.1.1",
		"server_port": 443,
		"tls": {
			"enabled": true,
			"server_name": "manual.example"
		}
	}`)

	if err := service.Save(db, "new", payload); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	record := &model.Outbound{}
	if err := db.Where("tag = ?", "manual-node").First(record).Error; err != nil {
		t.Fatalf("load outbound failed: %v", err)
	}

	if len(record.RawOutbound) == 0 {
		t.Fatalf("expected RawOutbound to be stored")
	}

	var stored map[string]interface{}
	if err := json.Unmarshal(record.RawOutbound, &stored); err != nil {
		t.Fatalf("unmarshal RawOutbound failed: %v", err)
	}
	if _, exists := stored["id"]; exists {
		t.Fatalf("expected RawOutbound to omit id, got %#v", stored["id"])
	}
	if gotValue, _ := stored["tag"].(string); gotValue != "manual-node" {
		t.Fatalf("expected tag manual-node, got %#v", stored["tag"])
	}
}
