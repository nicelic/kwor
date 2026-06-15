package service

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"gorm.io/gorm"
)

func TestSubOutboundDelete_RecordsBlockedAutoSyncTarget(t *testing.T) {
	db := setupSubSyncBlockTestDB(t, "sub-sync-block-delete.db")

	record := &model.SubOutbound{
		Type:            "direct",
		Tag:             "sub_node_a",
		Options:         json.RawMessage(`{}`),
		SourceType:      subOutboundSourceClient,
		SourceClientId:  11,
		SourceInboundId: 22,
	}
	if err := db.Create(record).Error; err != nil {
		t.Fatalf("create suboutbound failed: %v", err)
	}

	rawTag, err := json.Marshal(record.Tag)
	if err != nil {
		t.Fatalf("marshal tag failed: %v", err)
	}

	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("begin tx failed: %v", tx.Error)
	}
	BeginManagedRuntimeHookScope(tx)
	svc := &SubOutboundService{}
	if err := svc.Save(tx, "del", rawTag); err != nil {
		DiscardManagedRuntimeHookScope(tx)
		tx.Rollback()
		t.Fatalf("delete suboutbound failed: %v", err)
	}
	DiscardManagedRuntimeHookScope(tx)
	if err := tx.Commit().Error; err != nil {
		DiscardManagedRuntimeHookScope(tx)
		t.Fatalf("commit tx failed: %v", err)
	}

	var blockCount int64
	if err := db.Model(model.SubSyncBlock{}).
		Where("source_type = ? AND source_client_id = ? AND source_inbound_id = ?", subOutboundSourceClient, 11, 22).
		Count(&blockCount).Error; err != nil {
		t.Fatalf("count sub sync block failed: %v", err)
	}
	if blockCount != 1 {
		t.Fatalf("expected one sub sync block record, got %d", blockCount)
	}
}

func TestSyncClientOnAutoPush_SkipsBlockedInbound(t *testing.T) {
	db := setupSubSyncBlockTestDB(t, "sub-sync-block-default.db")

	inbound := &model.Inbound{
		Type:    "direct",
		Tag:     "in_default",
		OutJson: json.RawMessage(`{"type":"direct","tag":"in_default"}`),
		Options: json.RawMessage(`{}`),
	}
	if err := db.Create(inbound).Error; err != nil {
		t.Fatalf("create inbound failed: %v", err)
	}

	clientInboundIDs, _ := json.Marshal([]uint{inbound.Id})
	client := &model.Client{
		Name:     "client_default",
		Inbounds: clientInboundIDs,
		Config:   json.RawMessage(`{}`),
	}
	if err := db.Create(client).Error; err != nil {
		t.Fatalf("create client failed: %v", err)
	}

	if err := blockSubSyncInbound(db, subOutboundSourceClient, client.Id, inbound.Id); err != nil {
		t.Fatalf("block inbound failed: %v", err)
	}

	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("begin tx failed: %v", tx.Error)
	}
	if err := (&SyncService{}).SyncClientOnAutoPush(tx, client, ""); err != nil {
		tx.Rollback()
		t.Fatalf("SyncClientOnAutoPush returned error: %v", err)
	}
	if err := tx.Commit().Error; err != nil {
		t.Fatalf("commit tx failed: %v", err)
	}

	var count int64
	if err := db.Model(model.SubOutbound{}).
		Where("source_type = ? AND source_client_id = ?", subOutboundSourceClient, client.Id).
		Count(&count).Error; err != nil {
		t.Fatalf("count suboutbounds failed: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected blocked inbound to not be recreated, got %d records", count)
	}
}

func TestMihomoSyncClientOnAutoPush_SkipsBlockedInbound(t *testing.T) {
	db := setupSubSyncBlockTestDB(t, "sub-sync-block-mihomo.db")

	inbound := &model.MihomoInbound{
		Type:    "direct",
		Tag:     "in_mihomo",
		OutJson: json.RawMessage(`{"type":"direct","tag":"in_mihomo"}`),
		Options: json.RawMessage(`{}`),
	}
	if err := db.Create(inbound).Error; err != nil {
		t.Fatalf("create mihomo inbound failed: %v", err)
	}

	clientInboundIDs, _ := json.Marshal([]uint{inbound.Id})
	client := &model.MihomoClient{
		Name:     "client_mihomo",
		Inbounds: clientInboundIDs,
		Config:   json.RawMessage(`{}`),
	}
	if err := db.Create(client).Error; err != nil {
		t.Fatalf("create mihomo client failed: %v", err)
	}

	if err := blockSubSyncInbound(db, subOutboundSourceMihomoClient, client.Id, inbound.Id); err != nil {
		t.Fatalf("block mihomo inbound failed: %v", err)
	}

	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("begin tx failed: %v", tx.Error)
	}
	if err := (&MihomoSyncService{}).SyncClientOnAutoPush(tx, client, ""); err != nil {
		tx.Rollback()
		t.Fatalf("Mihomo SyncClientOnAutoPush returned error: %v", err)
	}
	if err := tx.Commit().Error; err != nil {
		t.Fatalf("commit tx failed: %v", err)
	}

	var count int64
	if err := db.Model(model.SubOutbound{}).
		Where("source_type = ? AND source_client_id = ?", subOutboundSourceMihomoClient, client.Id).
		Count(&count).Error; err != nil {
		t.Fatalf("count mihomo suboutbounds failed: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected blocked mihomo inbound to not be recreated, got %d records", count)
	}
}

func setupSubSyncBlockTestDB(t *testing.T, dbName string) *gorm.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), dbName)
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

	return db
}
