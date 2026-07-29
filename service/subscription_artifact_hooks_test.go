package service

import (
	"encoding/json"
	"testing"

	"github.com/alireza0/s-ui/database/model"
	"gorm.io/gorm"
)

func TestSubscriptionCRUDValidatesInMemoryWithoutManagedCopies(t *testing.T) {
	db := setupManagedRuntimeFileStoreTestDB(t, "subscription-crud-no-artifact.db")
	configService := &ConfigService{}

	nodePayload := json.RawMessage(`{
		"type":"socks",
		"tag":"crud-node",
		"server":"127.0.0.1",
		"server_port":1080,
		"version":"5"
	}`)
	if _, err := configService.Save("suboutbounds", "new", nodePayload, "", "", ""); err != nil {
		t.Fatalf("create subscription node failed: %v", err)
	}

	groupPayload := json.RawMessage(`{
		"name":"crud-group",
		"outbounds":"[\"crud-node\"]"
	}`)
	if _, err := configService.Save("subgroups", "new", groupPayload, "", "", ""); err != nil {
		t.Fatalf("create subscription group failed: %v", err)
	}

	var group model.SubGroup
	if err := db.Where("name = ?", "crud-group").First(&group).Error; err != nil {
		t.Fatalf("load created subscription group failed: %v", err)
	}
	groupEditPayload, err := json.Marshal(map[string]interface{}{
		"id":         group.Id,
		"name":       "crud-group-renamed",
		"sort_order": group.SortOrder,
		"outbounds":  `["crud-node"]`,
	})
	if err != nil {
		t.Fatalf("marshal group edit payload failed: %v", err)
	}
	if _, err := configService.Save("subgroups", "edit", groupEditPayload, "", "", ""); err != nil {
		t.Fatalf("rename subscription group failed: %v", err)
	}

	(&SubOutboundService{}).RegenerateAllSubOutboundConfigs()
	(&SubOutboundService{}).RegenerateAllSubJsonFiles()
	(&SubGroupService{}).RegenerateAllGroupConfigs()
	assertNoObsoleteManagedRuntimeJSONRows(t, db)

	groupDeletePayload, _ := json.Marshal("crud-group-renamed")
	if _, err := configService.Save("subgroups", "del", groupDeletePayload, "", "", ""); err != nil {
		t.Fatalf("delete subscription group failed: %v", err)
	}
	nodeDeletePayload, _ := json.Marshal("crud-node")
	if _, err := configService.Save("suboutbounds", "del", nodeDeletePayload, "", "", ""); err != nil {
		t.Fatalf("delete subscription node failed: %v", err)
	}
	assertNoObsoleteManagedRuntimeJSONRows(t, db)
}

func assertNoObsoleteManagedRuntimeJSONRows(t *testing.T, db *gorm.DB) {
	t.Helper()
	for _, root := range obsoleteManagedRuntimeJSONRoots {
		prefix := root + "/"
		var count int64
		if err := db.Table(managedRuntimeFileTable).
			Where("lower(ext) = '.json' AND (path = ? OR substr(path, 1, ?) = ?)", root, len(prefix), prefix).
			Count(&count).Error; err != nil {
			t.Fatalf("count obsolete runtime rows under %s failed: %v", root, err)
		}
		if count != 0 {
			t.Fatalf("expected no obsolete runtime rows under %s, got %d", root, count)
		}
	}
}
