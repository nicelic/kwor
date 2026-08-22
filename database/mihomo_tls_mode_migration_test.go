package database

import (
	"path/filepath"
	"testing"

	"github.com/alireza0/s-ui/database/model"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestInitDBBackfillsExistingMihomoRealityModeOnly(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy-mihomo-tls.db")
	legacy, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatalf("open legacy database: %v", err)
	}
	if err := legacy.Exec(`CREATE TABLE mihomo_tls (
		id integer PRIMARY KEY,
		name text,
		certificate_record_id integer NOT NULL DEFAULT 0,
		server blob,
		client blob
	)`).Error; err != nil {
		t.Fatalf("create legacy mihomo_tls table: %v", err)
	}
	if err := legacy.Exec(`INSERT INTO mihomo_tls (id, name, server, client) VALUES
		(1, 'legacy-reality', CAST('{"reality":{"enabled":true}}' AS BLOB), CAST('{}' AS BLOB)),
		(2, 'legacy-tls', CAST('{"enabled":true}' AS BLOB), CAST('{}' AS BLOB))`).Error; err != nil {
		t.Fatalf("seed legacy mihomo TLS rows: %v", err)
	}
	legacySQL, err := legacy.DB()
	if err != nil {
		t.Fatalf("get legacy sql handle: %v", err)
	}
	if err := legacySQL.Close(); err != nil {
		t.Fatalf("close legacy database: %v", err)
	}

	previous := db
	t.Cleanup(func() {
		current := GetDB()
		if current != nil && current != previous {
			if currentSQL, err := current.DB(); err == nil && currentSQL != nil {
				_ = currentSQL.Close()
			}
		}
		db = previous
	})
	if err := InitDB(path); err != nil {
		t.Fatalf("migrate database: %v", err)
	}
	current := GetDB()

	var rows []model.MihomoTls
	if err := current.Order("id ASC").Find(&rows).Error; err != nil {
		t.Fatalf("load migrated Mihomo TLS rows: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("unexpected migrated row count: %d", len(rows))
	}
	if rows[0].Mode != model.MihomoTlsModeReality {
		t.Fatalf("expected Reality row to be backfilled, got %q", rows[0].Mode)
	}
	if rows[1].Mode != model.MihomoTlsModeTLS {
		t.Fatalf("expected ordinary TLS row to remain TLS, got %q", rows[1].Mode)
	}
}
