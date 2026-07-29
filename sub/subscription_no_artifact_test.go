package sub

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"gorm.io/gorm"
)

func TestNodeAndGroupSubscriptionsDoNotPersistManagedCopies(t *testing.T) {
	setupSubscriptionTestDB(t, "dynamic-subscription-no-artifact.db")
	db := database.GetDB()

	node := &model.SubOutbound{
		Type:    "socks",
		Tag:     "dynamic-node",
		Options: json.RawMessage(`{"server":"127.0.0.1","server_port":1080,"version":"5"}`),
	}
	if err := db.Create(node).Error; err != nil {
		t.Fatalf("create subscription node failed: %v", err)
	}
	group := &model.SubGroup{
		Name:      "dynamic-group",
		Outbounds: `["dynamic-node"]`,
	}
	if err := db.Create(group).Error; err != nil {
		t.Fatalf("create subscription group failed: %v", err)
	}

	assertNoObsoleteManagedSubscriptionRows(t, db)
	renderer := &SubManagerSubService{}

	nodeJSON, err := renderer.GetSubManagerJson(node.Tag)
	if err != nil || nodeJSON == nil || !strings.Contains(*nodeJSON, node.Tag) {
		t.Fatalf("render node JSON failed: payload=%v err=%v", nodeJSON, err)
	}
	nodeClash, err := renderer.GetSubManagerClash(node.Tag)
	if err != nil || nodeClash == nil || !strings.Contains(*nodeClash, node.Tag) {
		t.Fatalf("render node Clash failed: payload=%v err=%v", nodeClash, err)
	}
	groupJSON, err := renderer.GetSubGroupJson(group.Name)
	if err != nil || groupJSON == nil || !strings.Contains(*groupJSON, node.Tag) {
		t.Fatalf("render group JSON failed: payload=%v err=%v", groupJSON, err)
	}
	groupClash, err := renderer.GetSubGroupClash(group.Name)
	if err != nil || groupClash == nil || !strings.Contains(*groupClash, node.Tag) {
		t.Fatalf("render group Clash failed: payload=%v err=%v", groupClash, err)
	}
	if err := SaveSubJsonToFile(node.Tag, *nodeJSON); err != nil {
		t.Fatalf("compatibility SaveSubJsonToFile validation failed: %v", err)
	}

	assertNoObsoleteManagedSubscriptionRows(t, db)
}

func assertNoObsoleteManagedSubscriptionRows(t *testing.T, db *gorm.DB) {
	t.Helper()
	for _, root := range []string{"Inbound", "outbound", "sub_manager", "sub_json"} {
		prefix := root + "/"
		var count int64
		if err := db.Table("managed_runtime_files").
			Where("lower(ext) = '.json' AND (path = ? OR substr(path, 1, ?) = ?)", root, len(prefix), prefix).
			Count(&count).Error; err != nil {
			t.Fatalf("count obsolete runtime rows under %s failed: %v", root, err)
		}
		if count != 0 {
			t.Fatalf("expected no obsolete runtime rows under %s, got %d", root, count)
		}
	}
}
