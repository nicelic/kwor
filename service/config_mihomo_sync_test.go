package service

import (
	"encoding/json"
	"testing"

	"github.com/alireza0/s-ui/database/model"
)

func TestSyncManagedMihomoClientsRefreshesSudokuClashOptions(t *testing.T) {
	db := setupMihomoSyncTestDB(t, "config-mihomo-sync-managed.db")

	inbound := model.MihomoInbound{
		Type: "sudoku",
		Tag:  "sudoku-43350",
		Options: mustJSONRaw(t, map[string]interface{}{
			"listen":      "::",
			"listen_port": 43350,
			"key":         "797ce555-a6c7-4e30-9388-fbeaf8709760",
			"httpmask": map[string]interface{}{
				"disable": false,
				"mode":    "legacy",
			},
		}),
	}
	if err := db.Create(&inbound).Error; err != nil {
		t.Fatalf("create mihomo inbound failed: %v", err)
	}

	expectedKey := "7387cb3a-f00c-4d06-b77d-5a5ed9e96169"
	client := model.MihomoClient{
		Enable: true,
		Name:   "sudoku-user",
		Config: mustJSONRaw(t, map[string]interface{}{
			"sudoku": map[string]interface{}{
				"uuid": expectedKey,
			},
		}),
		Inbounds: mustJSONRaw(t, []uint{inbound.Id}),
		Links:    mustJSONRaw(t, []map[string]string{}),
	}
	if err := db.Create(&client).Error; err != nil {
		t.Fatalf("create mihomo client failed: %v", err)
	}

	subTag := buildMihomoClientSubTag(inbound.Tag, client.Name)
	managed := model.SubOutbound{
		Type: "sudoku",
		Tag:  subTag,
		Options: mustJSONRaw(t, map[string]interface{}{
			"server":      "149.104.5.26",
			"server_port": 43350,
			"key":         "old-stale-key",
		}),
		ClashOptions: mustJSONRaw(t, map[string]interface{}{
			"name":   subTag,
			"type":   "sudoku",
			"server": "149.104.5.26",
			"port":   43350,
			"key":    "old-stale-key",
		}),
		SourceType:      subOutboundSourceMihomoClient,
		SourceClientId:  client.Id,
		SourceInboundId: inbound.Id,
	}
	if err := db.Create(&managed).Error; err != nil {
		t.Fatalf("create managed suboutbound failed: %v", err)
	}

	svc := &ConfigService{}
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("begin tx failed: %v", tx.Error)
	}
	if err := svc.syncManagedMihomoClients(tx, "149.104.5.26"); err != nil {
		tx.Rollback()
		t.Fatalf("syncManagedMihomoClients failed: %v", err)
	}
	if err := tx.Commit().Error; err != nil {
		t.Fatalf("commit tx failed: %v", err)
	}

	var reloaded model.SubOutbound
	if err := db.Where("tag = ?", subTag).First(&reloaded).Error; err != nil {
		t.Fatalf("reload suboutbound failed: %v", err)
	}

	var clashProxy map[string]interface{}
	if err := json.Unmarshal(reloaded.ClashOptions, &clashProxy); err != nil {
		t.Fatalf("unmarshal clash options failed: %v", err)
	}

	if got, _ := clashProxy["key"].(string); got != expectedKey {
		t.Fatalf("expected refreshed sudoku key %q, got %#v", expectedKey, clashProxy["key"])
	}
}
