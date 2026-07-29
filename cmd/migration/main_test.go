package migration

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/alireza0/s-ui/config"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestMigrateDBAtPathClosesConnection(t *testing.T) {
	path := filepath.Join(t.TempDir(), "migration.db")
	db, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatalf("open database failed: %v", err)
	}
	if err := db.Exec("CREATE TABLE settings (key TEXT PRIMARY KEY, value TEXT)").Error; err != nil {
		t.Fatalf("create settings table failed: %v", err)
	}
	if err := db.Exec("INSERT INTO settings (key, value) VALUES (?, ?)", "version", config.GetVersion()).Error; err != nil {
		t.Fatalf("insert version failed: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql database failed: %v", err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("close setup database failed: %v", err)
	}

	if err := migrateDBAtPath(path); err != nil {
		t.Fatalf("migrate database failed: %v", err)
	}

	renamedPath := path + ".renamed"
	if err := os.Rename(path, renamedPath); err != nil {
		t.Fatalf("migration left the database file open: %v", err)
	}
}
