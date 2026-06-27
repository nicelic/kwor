package service

import (
	"encoding/json"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
)

func initStatsStorageTestDB(t *testing.T) {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "stats-storage.db")
	if err := database.InitDB(dbPath); err != nil {
		t.Fatalf("init db failed: %v", err)
	}
	if sqlDB, err := database.GetDB().DB(); err == nil && sqlDB != nil {
		t.Cleanup(func() {
			_ = sqlDB.Close()
		})
	}
}

func TestPrepareHistoryStorageCompactsLegacyStats(t *testing.T) {
	initStatsStorageTestDB(t)
	db := database.GetDB()

	rows := []model.Stats{
		{DateTime: 125, Resource: "user", Tag: "alice", Direction: true, Traffic: 10},
		{DateTime: 129, Resource: "user", Tag: "alice", Direction: true, Traffic: 15},
		{DateTime: 130, Resource: "user", Tag: "alice", Direction: false, Traffic: 20},
		{DateTime: 181, Resource: "client", Tag: "alice", Direction: true, Traffic: 7},
		{DateTime: 181, Resource: "client", Tag: "", Direction: true, Traffic: 99},
		{DateTime: 181, Resource: "client", Tag: "alice", Direction: true, Traffic: 0},
	}
	if err := db.Create(&rows).Error; err != nil {
		t.Fatalf("seed stats failed: %v", err)
	}

	resetHistoryStorageState()
	if err := EnsureHistoryStorageReady(); err != nil {
		t.Fatalf("EnsureHistoryStorageReady failed: %v", err)
	}

	var got []model.Stats
	if err := db.Order("date_time, direction, traffic").Find(&got).Error; err != nil {
		t.Fatalf("query compacted stats failed: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("compacted stats len=%d want 3: %#v", len(got), got)
	}

	want := map[string]int64{
		"120/client/alice/true":  25,
		"120/client/alice/false": 20,
		"180/client/alice/true":  7,
	}
	for _, row := range got {
		key := rowKey(row)
		if want[key] != row.Traffic {
			t.Fatalf("row %s traffic=%d want %d", key, row.Traffic, want[key])
		}
		delete(want, key)
	}
	if len(want) != 0 {
		t.Fatalf("missing rows after compaction: %#v", want)
	}
}

func TestUpsertStatsTrafficAccumulatesIntoMinuteBucket(t *testing.T) {
	initStatsStorageTestDB(t)
	db := database.GetDB()

	if err := PrepareHistoryStorageOnStartup(); err != nil {
		t.Fatalf("PrepareHistoryStorageOnStartup failed: %v", err)
	}

	tx := db.Begin()
	if err := upsertStatsTraffic(tx, model.Stats{
		DateTime:  125,
		Resource:  "user",
		Tag:       "alice",
		Direction: true,
		Traffic:   10,
	}); err != nil {
		t.Fatalf("first upsert failed: %v", err)
	}
	if err := upsertStatsTraffic(tx, model.Stats{
		DateTime:  129,
		Resource:  "client",
		Tag:       "alice",
		Direction: true,
		Traffic:   15,
	}); err != nil {
		t.Fatalf("second upsert failed: %v", err)
	}
	if err := tx.Commit().Error; err != nil {
		t.Fatalf("commit failed: %v", err)
	}

	var row model.Stats
	if err := db.Where("date_time = ? AND resource = ? AND tag = ? AND direction = ?", 120, "client", "alice", true).First(&row).Error; err != nil {
		t.Fatalf("query upserted row failed: %v", err)
	}
	if row.Traffic != 25 {
		t.Fatalf("traffic=%d want 25", row.Traffic)
	}
}

func TestReadSQLitePageStatsReadsPragmaValues(t *testing.T) {
	initStatsStorageTestDB(t)

	pageCount, freelistCount, err := readSQLitePageStats(database.GetDB())
	if err != nil {
		t.Fatalf("readSQLitePageStats failed: %v", err)
	}
	if pageCount <= 0 {
		t.Fatalf("pageCount=%d want > 0", pageCount)
	}
	if freelistCount < 0 {
		t.Fatalf("freelistCount=%d want >= 0", freelistCount)
	}
}

func TestPruneChangesHistoryEnforcesMaxRows(t *testing.T) {
	initStatsStorageTestDB(t)
	db := database.GetDB()

	rows := make([]model.Changes, 0, changesMaxRows+50)
	now := time.Now().Unix()
	for i := int64(0); i < changesMaxRows+50; i++ {
		rows = append(rows, model.Changes{
			DateTime: now,
			Actor:    "tester",
			Key:      "settings",
			Action:   "set",
			Obj:      json.RawMessage(`{"trafficAge":"30"}`),
		})
	}
	if err := db.CreateInBatches(&rows, 500).Error; err != nil {
		t.Fatalf("seed changes failed: %v", err)
	}

	deleted, err := pruneChangesHistory(db)
	if err != nil {
		t.Fatalf("pruneChangesHistory failed: %v", err)
	}
	if deleted == 0 {
		t.Fatalf("deleted=%d want > 0", deleted)
	}

	var count int64
	if err := db.Model(model.Changes{}).Count(&count).Error; err != nil {
		t.Fatalf("count changes failed: %v", err)
	}
	if count != changesMaxRows {
		t.Fatalf("count=%d want %d", count, changesMaxRows)
	}
}

func rowKey(row model.Stats) string {
	return strconv.FormatInt(row.DateTime, 10) + "/" + row.Resource + "/" + row.Tag + "/" + boolString(row.Direction)
}

func boolString(value bool) string {
	if value {
		return "true"
	}
	return "false"
}
