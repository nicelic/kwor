package database

import (
	"path/filepath"
	"testing"

	"github.com/alireza0/s-ui/database/model"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestInitDBMigratesDuplicateSettingsAndCreatesUniqueIndex(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy-settings.db")
	legacy, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatalf("open legacy database: %v", err)
	}
	if err := legacy.Exec(`CREATE TABLE settings (id integer PRIMARY KEY AUTOINCREMENT, key text, value text)`).Error; err != nil {
		t.Fatalf("create legacy settings table: %v", err)
	}
	if err := legacy.Exec(`INSERT INTO settings (id, key, value) VALUES (1, 'subJsonExt', 'old'), (9, 'subJsonExt', 'effective'), (3, 'webPort', '8888')`).Error; err != nil {
		t.Fatalf("seed duplicate settings: %v", err)
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

	rows := []model.Setting{}
	if err := current.Where("key = ?", "subJsonExt").Find(&rows).Error; err != nil {
		t.Fatalf("load migrated setting: %v", err)
	}
	if len(rows) != 1 || rows[0].Id != 9 || rows[0].Value != "effective" {
		t.Fatalf("unexpected migrated rows: %#v", rows)
	}
	if err := current.Create(&model.Setting{Key: "subJsonExt", Value: "duplicate"}).Error; err == nil {
		t.Fatal("expected unique settings.key index to reject duplicate")
	}
	state := model.SettingsState{}
	if err := current.First(&state, 1).Error; err != nil {
		t.Fatalf("load settings revision singleton: %v", err)
	}
	if state.Id != 1 || state.Revision != 1 {
		t.Fatalf("unexpected settings state: %#v", state)
	}
}
