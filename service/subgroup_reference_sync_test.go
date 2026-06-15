package service

import (
	"encoding/json"
	"testing"

	"github.com/alireza0/s-ui/database/model"
)

func TestSubOutboundServiceEditRenamesSubGroupReferences(t *testing.T) {
	db := initOutboundEditMergeTestDB(t, "subgroup-rename.db")

	original := map[string]interface{}{
		"type":        "trojan",
		"tag":         "node-old",
		"server":      "1.1.1.1",
		"server_port": float64(443),
		"password":    "secret",
	}

	record := &model.SubOutbound{}
	if err := record.UnmarshalJSON(mustMarshalJSON(t, original)); err != nil {
		t.Fatalf("SubOutbound.UnmarshalJSON failed: %v", err)
	}
	record.RawOutbound = mustMarshalJSON(t, original)
	if err := db.Create(record).Error; err != nil {
		t.Fatalf("create suboutbound failed: %v", err)
	}

	groupTags, _ := json.Marshal([]string{"node-old"})
	group := &model.SubGroup{
		Name:      "group-a",
		Outbounds: string(groupTags),
	}
	if err := db.Create(group).Error; err != nil {
		t.Fatalf("create subgroup failed: %v", err)
	}

	editPayload := cloneJSONMapForTest(original)
	editPayload["id"] = float64(record.Id)
	editPayload["tag"] = "node-new"

	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("begin tx failed: %v", tx.Error)
	}
	BeginManagedRuntimeHookScope(tx)
	if err := (&SubOutboundService{}).Save(tx, "edit", mustMarshalJSON(t, editPayload)); err != nil {
		DiscardManagedRuntimeHookScope(tx)
		tx.Rollback()
		t.Fatalf("save edit failed: %v", err)
	}

	updatedGroup := &model.SubGroup{}
	if err := tx.Where("id = ?", group.Id).First(updatedGroup).Error; err != nil {
		DiscardManagedRuntimeHookScope(tx)
		tx.Rollback()
		t.Fatalf("load subgroup failed: %v", err)
	}

	if got := parseSubGroupOutboundTags(updatedGroup.Outbounds); len(got) != 1 || got[0] != "node-new" {
		DiscardManagedRuntimeHookScope(tx)
		tx.Rollback()
		t.Fatalf("expected subgroup tags [node-new], got %#v", got)
	}

	DiscardManagedRuntimeHookScope(tx)
	_ = tx.Rollback()
}

func TestSubOutboundServiceDeleteRemovesSubGroupReferences(t *testing.T) {
	db := initOutboundEditMergeTestDB(t, "subgroup-delete.db")

	original := map[string]interface{}{
		"type":        "trojan",
		"tag":         "node-old",
		"server":      "1.1.1.1",
		"server_port": float64(443),
		"password":    "secret",
	}

	record := &model.SubOutbound{}
	if err := record.UnmarshalJSON(mustMarshalJSON(t, original)); err != nil {
		t.Fatalf("SubOutbound.UnmarshalJSON failed: %v", err)
	}
	record.RawOutbound = mustMarshalJSON(t, original)
	if err := db.Create(record).Error; err != nil {
		t.Fatalf("create suboutbound failed: %v", err)
	}

	groupTags, _ := json.Marshal([]string{"node-old"})
	group := &model.SubGroup{
		Name:      "group-a",
		Outbounds: string(groupTags),
	}
	if err := db.Create(group).Error; err != nil {
		t.Fatalf("create subgroup failed: %v", err)
	}

	rawTag, err := json.Marshal("node-old")
	if err != nil {
		t.Fatalf("marshal delete tag failed: %v", err)
	}

	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("begin tx failed: %v", tx.Error)
	}
	BeginManagedRuntimeHookScope(tx)
	if err := (&SubOutboundService{}).Save(tx, "del", rawTag); err != nil {
		DiscardManagedRuntimeHookScope(tx)
		tx.Rollback()
		t.Fatalf("save delete failed: %v", err)
	}

	updatedGroup := &model.SubGroup{}
	if err := tx.Where("id = ?", group.Id).First(updatedGroup).Error; err != nil {
		DiscardManagedRuntimeHookScope(tx)
		tx.Rollback()
		t.Fatalf("load subgroup failed: %v", err)
	}

	if got := parseSubGroupOutboundTags(updatedGroup.Outbounds); len(got) != 0 {
		DiscardManagedRuntimeHookScope(tx)
		tx.Rollback()
		t.Fatalf("expected subgroup tags to be empty, got %#v", got)
	}

	DiscardManagedRuntimeHookScope(tx)
	_ = tx.Rollback()
}

func TestSubGroupServiceGetAllPrunesMissingSubOutboundReferences(t *testing.T) {
	db := initOutboundEditMergeTestDB(t, "subgroup-prune-missing.db")

	valid := map[string]interface{}{
		"type":        "trojan",
		"tag":         "node-valid",
		"server":      "1.1.1.1",
		"server_port": float64(443),
		"password":    "secret",
	}

	record := &model.SubOutbound{}
	if err := record.UnmarshalJSON(mustMarshalJSON(t, valid)); err != nil {
		t.Fatalf("SubOutbound.UnmarshalJSON failed: %v", err)
	}
	record.RawOutbound = mustMarshalJSON(t, valid)
	if err := db.Create(record).Error; err != nil {
		t.Fatalf("create suboutbound failed: %v", err)
	}

	groupTags, _ := json.Marshal([]string{"node-valid", "node-stale"})
	group := &model.SubGroup{
		Name:      "group-a",
		Outbounds: string(groupTags),
	}
	if err := db.Create(group).Error; err != nil {
		t.Fatalf("create subgroup failed: %v", err)
	}

	BeginManagedRuntimeHookScope(db)
	groups, err := (&SubGroupService{}).GetAll()
	DiscardManagedRuntimeHookScope(db)
	if err != nil {
		t.Fatalf("GetAll failed: %v", err)
	}
	if len(groups) != 1 {
		t.Fatalf("expected 1 subgroup, got %d", len(groups))
	}
	if got := parseSubGroupOutboundTags(groups[0].Outbounds); len(got) != 1 || got[0] != "node-valid" {
		t.Fatalf("expected subgroup tags [node-valid], got %#v", got)
	}

	updatedGroup := &model.SubGroup{}
	if err := db.Where("id = ?", group.Id).First(updatedGroup).Error; err != nil {
		t.Fatalf("reload subgroup failed: %v", err)
	}
	if got := parseSubGroupOutboundTags(updatedGroup.Outbounds); len(got) != 1 || got[0] != "node-valid" {
		t.Fatalf("expected persisted subgroup tags [node-valid], got %#v", got)
	}
}
