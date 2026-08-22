package service

import (
	"encoding/json"
	"testing"

	"github.com/alireza0/s-ui/database/model"
)

func TestConfigSaveDeletingImportedSubGroupReturnsSharedEmptyCollections(t *testing.T) {
	db := setupManagedRuntimeFileStoreTestDB(t, "config-subscription-response.db")

	group := model.SubGroup{
		Name:            "imported-group",
		Outbounds:       `["imported-node"]`,
		SubscriptionUrl: "https://example.invalid/subscription.json",
		SortOrder:       1,
	}
	if err := db.Create(&group).Error; err != nil {
		t.Fatalf("create subscription group: %v", err)
	}
	node := model.SubOutbound{
		Type:           "socks",
		Tag:            "imported-node",
		Options:        json.RawMessage(`{"server":"127.0.0.1","server_port":1080,"version":"5"}`),
		RawOutbound:    json.RawMessage(`{"type":"socks","tag":"imported-node","server":"127.0.0.1","server_port":1080,"version":"5"}`),
		SourceType:     subOutboundSourceSubGroup,
		SourceClientId: group.Id,
	}
	if err := db.Create(&node).Error; err != nil {
		t.Fatalf("create imported subscription node: %v", err)
	}

	payload, err := json.Marshal(group.Name)
	if err != nil {
		t.Fatalf("marshal group deletion payload: %v", err)
	}
	objects, err := (&ConfigService{}).Save("subgroups", "del", payload, "", "test", "")
	if err != nil {
		t.Fatalf("delete imported subscription group: %v", err)
	}
	if !containsConfigSaveObject(objects, "subgroups") || !containsConfigSaveObject(objects, "suboutbounds") {
		t.Fatalf("delete response objects = %#v, want subgroups and suboutbounds", objects)
	}

	var groupCount, nodeCount int64
	if err := db.Model(&model.SubGroup{}).Count(&groupCount).Error; err != nil {
		t.Fatalf("count subscription groups: %v", err)
	}
	if err := db.Model(&model.SubOutbound{}).Count(&nodeCount).Error; err != nil {
		t.Fatalf("count subscription nodes: %v", err)
	}
	if groupCount != 0 || nodeCount != 0 {
		t.Fatalf("remaining subscription data: groups=%d nodes=%d", groupCount, nodeCount)
	}

	groups, err := (&SubGroupService{}).GetAll()
	if err != nil {
		t.Fatalf("load remaining groups: %v", err)
	}
	nodes, err := (&SubOutboundService{}).GetAll()
	if err != nil {
		t.Fatalf("load remaining nodes: %v", err)
	}
	if groups == nil || len(groups) != 0 || nodes == nil || *nodes == nil || len(*nodes) != 0 {
		t.Fatalf("empty shared collections were not preserved: groups=%#v nodes=%#v", groups, nodes)
	}
}

func TestUniqueConfigSaveObjectsPreservesFirstOccurrence(t *testing.T) {
	objects := uniqueConfigSaveObjects([]string{"clients", "suboutbounds", "clients", "subgroups", "suboutbounds"})
	expected := []string{"clients", "suboutbounds", "subgroups"}
	if len(objects) != len(expected) {
		t.Fatalf("unique object count = %d, want %d: %#v", len(objects), len(expected), objects)
	}
	for index, object := range expected {
		if objects[index] != object {
			t.Fatalf("object[%d] = %q, want %q", index, objects[index], object)
		}
	}
}

func containsConfigSaveObject(objects []string, target string) bool {
	for _, object := range objects {
		if object == target {
			return true
		}
	}
	return false
}
