package service

import (
	"path/filepath"
	"testing"

	"github.com/alireza0/s-ui/database"
)

func TestDefaultListServicesReturnNonNilEmptyArrays(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "default-empty-lists.db")
	if err := database.InitDB(dbPath); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	db := database.GetDB()
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db.DB failed: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })

	if result, err := (&OutboundService{}).GetAll(); err != nil {
		t.Fatalf("outbound GetAll failed: %v", err)
	} else if result == nil || *result == nil {
		t.Fatal("outbound GetAll returned a nil slice")
	}
	if result, err := (&ServicesService{}).GetAll(); err != nil {
		t.Fatalf("services GetAll failed: %v", err)
	} else if result == nil || *result == nil {
		t.Fatal("services GetAll returned a nil slice")
	}
	if result, err := (&EndpointService{}).GetAll(); err != nil {
		t.Fatalf("endpoint GetAll failed: %v", err)
	} else if result == nil || *result == nil {
		t.Fatal("endpoint GetAll returned a nil slice")
	}
	if groups, err := (&OutboundGroupService{}).GetAll(); err != nil {
		t.Fatalf("outbound group GetAll failed: %v", err)
	} else if groups == nil {
		t.Fatal("outbound group GetAll returned a nil slice")
	}
	if result, err := (&InboundService{}).getById("999999"); err != nil {
		t.Fatalf("inbound getById failed: %v", err)
	} else if result == nil || *result == nil {
		t.Fatal("inbound getById returned a nil slice")
	}
	if result, err := (&ClientService{}).getById("999999"); err != nil {
		t.Fatalf("client getById failed: %v", err)
	} else if result == nil || *result == nil {
		t.Fatal("client getById returned a nil slice")
	}
	if result, err := (&InboundService{}).GetOutJsonIPs("999999"); err != nil {
		t.Fatalf("inbound GetOutJsonIPs failed: %v", err)
	} else if result == nil {
		t.Fatal("inbound GetOutJsonIPs returned a nil slice")
	}
}
