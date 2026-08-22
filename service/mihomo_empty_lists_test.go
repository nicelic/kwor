package service

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/alireza0/s-ui/database"
)

func TestMihomoListServicesReturnNonNilEmptyArrays(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "mihomo-empty-lists.db")
	if err := database.InitDB(dbPath); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	db := database.GetDB()
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db.DB failed: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	if result, err := (&MihomoClientService{}).GetAll(); err != nil {
		t.Fatalf("mihomo client GetAll failed: %v", err)
	} else if result == nil || *result == nil {
		t.Fatal("mihomo client GetAll returned a nil slice")
	}
	if result, err := (&MihomoOutboundService{}).GetAll(); err != nil {
		t.Fatalf("mihomo outbound GetAll failed: %v", err)
	} else if result == nil || *result == nil {
		t.Fatal("mihomo outbound GetAll returned a nil slice")
	}
	if result, err := (&MihomoOutboundGroupService{}).GetAll(); err != nil {
		t.Fatalf("mihomo outbound group GetAll failed: %v", err)
	} else if result == nil {
		t.Fatal("mihomo outbound group GetAll returned a nil slice")
	}
	result, err := (&MihomoClientService{}).getById("999999")
	if err != nil {
		t.Fatalf("mihomo client getById failed: %v", err)
	} else if result == nil || *result == nil {
		t.Fatal("mihomo client getById returned a nil slice")
	}

	encoded, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal empty Mihomo result failed: %v", err)
	}
	if string(encoded) != "[]" {
		t.Fatalf("empty Mihomo result JSON = %s, want []", encoded)
	}
}
