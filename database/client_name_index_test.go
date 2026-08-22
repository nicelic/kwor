package database

import (
	"path/filepath"
	"testing"

	"github.com/alireza0/s-ui/database/model"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestInitDBMigratesDuplicateClientNamesAndCreatesUniqueIndex(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy-clients.db")
	legacy, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatalf("open legacy database: %v", err)
	}
	if err := legacy.Exec(`CREATE TABLE clients (id integer PRIMARY KEY, enable numeric, name text)`).Error; err != nil {
		t.Fatalf("create legacy clients table: %v", err)
	}
	if err := legacy.Exec(`INSERT INTO clients (id, enable, name) VALUES (1, 1, 'duplicate'), (2, 1, 'duplicate'), (3, 1, 'duplicate-2')`).Error; err != nil {
		t.Fatalf("seed duplicate client names: %v", err)
	}
	legacySQL, err := legacy.DB()
	if err != nil {
		t.Fatalf("get legacy sql handle: %v", err)
	}
	if err := legacySQL.Close(); err != nil {
		t.Fatalf("close legacy database: %v", err)
	}

	previous := db
	if err := InitDB(path); err != nil {
		t.Fatalf("migrate database: %v", err)
	}
	current := GetDB()
	currentSQL, _ := current.DB()
	t.Cleanup(func() {
		if currentSQL != nil {
			_ = currentSQL.Close()
		}
		db = previous
	})

	var clients []model.Client
	if err := current.Order("id ASC").Find(&clients).Error; err != nil {
		t.Fatalf("load migrated clients: %v", err)
	}
	if len(clients) != 3 {
		t.Fatalf("unexpected migrated client count: %d", len(clients))
	}
	if clients[0].Name != "duplicate" || clients[1].Name != "duplicate-2-2" || clients[2].Name != "duplicate-2" {
		t.Fatalf("unexpected migrated client names: %#v", clients)
	}
	if err := current.Create(&model.Client{Name: "duplicate"}).Error; err == nil {
		t.Fatal("expected unique clients.name index to reject duplicate")
	}
}
