package service

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
)

func openSubGroupOrderTestDB(t *testing.T) {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "subgroups-order.db")
	if err := database.InitDB(dbPath); err != nil {
		t.Fatalf("init db failed: %v", err)
	}

	sqlDB, err := database.GetDB().DB()
	if err == nil && sqlDB != nil {
		t.Cleanup(func() {
			_ = sqlDB.Close()
		})
	}
}

func TestSubGroupServiceGetAllNormalizesLegacySortOrder(t *testing.T) {
	openSubGroupOrderTestDB(t)

	db := database.GetDB()
	legacyGroups := []model.SubGroup{
		{Name: "alpha", Outbounds: "[]"},
		{Name: "beta", Outbounds: "[]"},
		{Name: "gamma", Outbounds: "[]"},
	}
	for index := range legacyGroups {
		if err := db.Create(&legacyGroups[index]).Error; err != nil {
			t.Fatalf("create legacy group failed: %v", err)
		}
	}

	service := &SubGroupService{}
	groups, err := service.GetAll()
	if err != nil {
		t.Fatalf("get all groups failed: %v", err)
	}
	if len(groups) != 3 {
		t.Fatalf("unexpected group count: %d", len(groups))
	}

	expectedNames := []string{"alpha", "beta", "gamma"}
	for index, expectedName := range expectedNames {
		if groups[index].Name != expectedName {
			t.Fatalf("unexpected group order at %d: got %s want %s", index, groups[index].Name, expectedName)
		}
		if groups[index].SortOrder != index+1 {
			t.Fatalf("unexpected sort order at %d: got %d want %d", index, groups[index].SortOrder, index+1)
		}
	}
}

func TestSubGroupServiceSaveReorderPersistsGroupOrder(t *testing.T) {
	openSubGroupOrderTestDB(t)

	db := database.GetDB()
	initialGroups := []model.SubGroup{
		{Name: "alpha", Outbounds: "[]", SortOrder: 1},
		{Name: "beta", Outbounds: "[]", SortOrder: 2},
		{Name: "gamma", Outbounds: "[]", SortOrder: 3},
	}
	for index := range initialGroups {
		if err := db.Create(&initialGroups[index]).Error; err != nil {
			t.Fatalf("create initial group failed: %v", err)
		}
	}

	payload, err := json.Marshal(map[string][]uint{
		"ids": {
			initialGroups[2].Id,
			initialGroups[0].Id,
			initialGroups[1].Id,
		},
	})
	if err != nil {
		t.Fatalf("marshal reorder payload failed: %v", err)
	}

	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("begin tx failed: %v", tx.Error)
	}

	service := &SubGroupService{}
	if err := service.Save(tx, "reorder", payload); err != nil {
		tx.Rollback()
		t.Fatalf("save reorder failed: %v", err)
	}
	if err := tx.Commit().Error; err != nil {
		t.Fatalf("commit reorder failed: %v", err)
	}

	groups, err := service.GetAll()
	if err != nil {
		t.Fatalf("get all groups failed: %v", err)
	}

	expectedNames := []string{"gamma", "alpha", "beta"}
	for index, expectedName := range expectedNames {
		if groups[index].Name != expectedName {
			t.Fatalf("unexpected reordered group at %d: got %s want %s", index, groups[index].Name, expectedName)
		}
		if groups[index].SortOrder != index+1 {
			t.Fatalf("unexpected persisted sort order at %d: got %d want %d", index, groups[index].SortOrder, index+1)
		}
	}
}
