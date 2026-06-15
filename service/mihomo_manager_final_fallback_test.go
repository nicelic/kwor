package service

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"gorm.io/gorm"
)

func TestGenerateServerDocument_UsesFirstOutboundAsFallbackFinalWhenRouteFinalMissing(t *testing.T) {
	db := initMihomoManagerFinalFallbackTestDB(t, "mihomo-final-first-outbound.db")
	if err := db.Where("tag = ?", "direct").Delete(&model.MihomoOutbound{}).Error; err != nil {
		t.Fatalf("delete default direct outbound failed: %v", err)
	}

	insertMihomoOutboundForFallbackTest(t, db, `{
		"type": "socks",
		"tag": "m_hy2_out_sgv6",
		"server": "77.93.90.104",
		"server_port": 1080
	}`)
	insertMihomoOutboundForFallbackTest(t, db, `{
		"type": "socks",
		"tag": "m_hy2_out_sgv4",
		"server": "77.93.90.105",
		"server_port": 1081
	}`)

	document, err := NewMihomoManagerService().GenerateServerDocument()
	if err != nil {
		t.Fatalf("GenerateServerDocument failed: %v", err)
	}

	rules := extractMihomoRenderedRules(t, document)
	if len(rules) != 1 || rules[0] != "MATCH,m_hy2_out_sgv6" {
		t.Fatalf("rules = %#v, want %#v", rules, []string{"MATCH,m_hy2_out_sgv6"})
	}
}

func TestGenerateServerDocument_UsesDirectAsFallbackFinalWhenDirectIsFirstOutbound(t *testing.T) {
	db := initMihomoManagerFinalFallbackTestDB(t, "mihomo-final-direct-first.db")

	insertMihomoOutboundForFallbackTest(t, db, `{
		"type": "socks",
		"tag": "m_hy2_out_sgv6",
		"server": "77.93.90.104",
		"server_port": 1080
	}`)

	document, err := NewMihomoManagerService().GenerateServerDocument()
	if err != nil {
		t.Fatalf("GenerateServerDocument failed: %v", err)
	}

	rules := extractMihomoRenderedRules(t, document)
	if len(rules) != 1 || rules[0] != "MATCH,DIRECT" {
		t.Fatalf("rules = %#v, want %#v", rules, []string{"MATCH,DIRECT"})
	}
}

func TestGenerateServerDocument_InvalidRouteFinalFallsBackToFirstAvailableOutbound(t *testing.T) {
	db := initMihomoManagerFinalFallbackTestDB(t, "mihomo-final-invalid-fallback.db")
	if err := db.Where("tag = ?", "direct").Delete(&model.MihomoOutbound{}).Error; err != nil {
		t.Fatalf("delete default direct outbound failed: %v", err)
	}

	insertMihomoOutboundForFallbackTest(t, db, `{
		"type": "socks",
		"tag": "m_hy2_out_sgv6",
		"server": "77.93.90.104",
		"server_port": 1080
	}`)
	insertMihomoOutboundForFallbackTest(t, db, `{
		"type": "socks",
		"tag": "m_hy2_out_sgv4",
		"server": "77.93.90.105",
		"server_port": 1081
	}`)

	setMihomoConfigForFallbackTest(t, db, `{
		"route": {
			"final": "missing-outbound",
			"rules": [],
			"rule_set": []
		}
	}`)

	document, err := NewMihomoManagerService().GenerateServerDocument()
	if err != nil {
		t.Fatalf("GenerateServerDocument failed: %v", err)
	}

	rules := extractMihomoRenderedRules(t, document)
	if len(rules) != 1 || rules[0] != "MATCH,m_hy2_out_sgv6" {
		t.Fatalf("rules = %#v, want %#v", rules, []string{"MATCH,m_hy2_out_sgv6"})
	}
}

func TestGenerateServerDocument_SnellListenerIncludesSharedPSK(t *testing.T) {
	db := initMihomoManagerFinalFallbackTestDB(t, "mihomo-snell-listener-psk.db")

	inbound := model.MihomoInbound{
		Type: "snell",
		Tag:  "snell-21340",
		Options: json.RawMessage(`{
			"listen": "::",
			"listen_port": 21340,
			"version": 5,
			"udp": true,
			"obfs_opts": {
				"mode": "tls",
				"host": "www.bing.com"
			}
		}`),
	}
	if err := db.Create(&inbound).Error; err != nil {
		t.Fatalf("create snell inbound failed: %v", err)
	}

	client := model.MihomoClient{
		Enable: true,
		Name:   "3AJ2z1B4",
		Config: json.RawMessage(`{
			"snell": {
				"name": "3AJ2z1B4",
				"psk": "rl2y3Pj6-JF2b-n8BG-2rhe-76A6gix5b9Mg"
			}
		}`),
		Inbounds: json.RawMessage(fmt.Sprintf("[%d]", inbound.Id)),
		Links:    json.RawMessage(`[]`),
	}
	if err := db.Create(&client).Error; err != nil {
		t.Fatalf("create snell client failed: %v", err)
	}

	document, err := NewMihomoManagerService().GenerateServerDocument()
	if err != nil {
		t.Fatalf("GenerateServerDocument failed: %v", err)
	}

	rawListeners, ok := document["listeners"].([]interface{})
	if !ok || len(rawListeners) == 0 {
		t.Fatalf("document.listeners = %#v", document["listeners"])
	}

	listener, ok := rawListeners[0].(map[string]interface{})
	if !ok {
		t.Fatalf("listener[0] = %#v", rawListeners[0])
	}

	if got := listener["psk"]; got != "rl2y3Pj6-JF2b-n8BG-2rhe-76A6gix5b9Mg" {
		t.Fatalf("listener psk = %#v", got)
	}
}

func initMihomoManagerFinalFallbackTestDB(t *testing.T, filename string) *gorm.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), filename)
	if err := database.InitDB(dbPath); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}

	db := database.GetDB()
	if sqlDB, err := db.DB(); err == nil && sqlDB != nil {
		t.Cleanup(func() {
			_ = sqlDB.Close()
		})
	}

	return db
}

func insertMihomoOutboundForFallbackTest(t *testing.T, db *gorm.DB, payload string) {
	t.Helper()

	record := &model.MihomoOutbound{}
	if err := record.UnmarshalJSON([]byte(payload)); err != nil {
		t.Fatalf("MihomoOutbound.UnmarshalJSON failed: %v", err)
	}
	if err := db.Create(record).Error; err != nil {
		t.Fatalf("insert mihomo outbound failed: %v", err)
	}
}

func extractMihomoRenderedRules(t *testing.T, document map[string]interface{}) []string {
	t.Helper()

	rawRules, ok := document["rules"].([]interface{})
	if !ok {
		t.Fatalf("document.rules is not []interface{}: %#v", document["rules"])
	}

	rules := make([]string, 0, len(rawRules))
	for _, raw := range rawRules {
		rule, ok := raw.(string)
		if !ok {
			t.Fatalf("rendered rule is not string: %#v", raw)
		}
		rules = append(rules, rule)
	}
	return rules
}

func setMihomoConfigForFallbackTest(t *testing.T, db *gorm.DB, value string) {
	t.Helper()

	record := &model.Setting{}
	err := db.Where("key = ?", "mihomo_config").First(record).Error
	if database.IsNotFound(err) {
		if createErr := db.Create(&model.Setting{Key: "mihomo_config", Value: value}).Error; createErr != nil {
			t.Fatalf("create mihomo_config setting failed: %v", createErr)
		}
		return
	}
	if err != nil {
		t.Fatalf("load mihomo_config setting failed: %v", err)
	}
	if updateErr := db.Model(record).Update("value", value).Error; updateErr != nil {
		t.Fatalf("update mihomo_config setting failed: %v", updateErr)
	}
}
