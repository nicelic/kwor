package service

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/alireza0/s-ui/database/model"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func openSubJSONGuardTestDB(t *testing.T) *gorm.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open test db failed: %v", err)
	}

	if err := db.AutoMigrate(&model.Client{}, &model.Inbound{}, &model.SubGroup{}, &model.SubOutbound{}); err != nil {
		t.Fatalf("migrate test db failed: %v", err)
	}

	return db
}

func TestValidateSubOutboundSubJSONFileNameDetectsClientConflict(t *testing.T) {
	db := openSubJSONGuardTestDB(t)

	inbound := &model.Inbound{Tag: "node/1", Type: "vmess", Options: json.RawMessage(`{}`)}
	if err := db.Create(inbound).Error; err != nil {
		t.Fatalf("create inbound failed: %v", err)
	}

	client := &model.Client{
		Enable:   true,
		Name:     "alice",
		Inbounds: json.RawMessage(fmt.Sprintf("[%d]", inbound.Id)),
	}
	if err := db.Create(client).Error; err != nil {
		t.Fatalf("create client failed: %v", err)
	}

	err := validateSubOutboundSubJSONFileName(db, &model.SubOutbound{Tag: "node_1_alice"})
	if err == nil {
		t.Fatal("expected client collision error, got nil")
	}
}

func TestValidateSubGroupSubJSONFileNameDetectsClientConflict(t *testing.T) {
	db := openSubJSONGuardTestDB(t)

	inbound := &model.Inbound{Tag: "hk/01", Type: "vmess", Options: json.RawMessage(`{}`)}
	if err := db.Create(inbound).Error; err != nil {
		t.Fatalf("create inbound failed: %v", err)
	}

	client := &model.Client{
		Enable:   true,
		Name:     "bob",
		Inbounds: json.RawMessage(fmt.Sprintf("[%d]", inbound.Id)),
	}
	if err := db.Create(client).Error; err != nil {
		t.Fatalf("create client failed: %v", err)
	}

	err := validateSubGroupSubJSONFileName(db, &model.SubGroup{Name: "hk_01_bob"})
	if err == nil {
		t.Fatal("expected client collision error, got nil")
	}
}

func TestValidateManagedSubJSONFileNamesDetectsDuplicateClientFiles(t *testing.T) {
	db := openSubJSONGuardTestDB(t)

	inboundA := &model.Inbound{Tag: "node/1", Type: "vmess", Options: json.RawMessage(`{}`)}
	if err := db.Create(inboundA).Error; err != nil {
		t.Fatalf("create inboundA failed: %v", err)
	}
	inboundB := &model.Inbound{Tag: "node_1", Type: "vmess", Options: json.RawMessage(`{}`)}
	if err := db.Create(inboundB).Error; err != nil {
		t.Fatalf("create inboundB failed: %v", err)
	}

	client := &model.Client{
		Enable:   true,
		Name:     "carol",
		Inbounds: json.RawMessage(fmt.Sprintf("[%d,%d]", inboundA.Id, inboundB.Id)),
	}
	if err := db.Create(client).Error; err != nil {
		t.Fatalf("create client failed: %v", err)
	}

	if err := validateManagedSubJSONFileNames(db); err == nil {
		t.Fatal("expected duplicate client filename error, got nil")
	}
}
