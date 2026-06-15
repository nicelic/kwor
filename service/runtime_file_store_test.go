package service

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alireza0/s-ui/database"
	"gorm.io/gorm"
)

func TestManagedRuntimeFileStoreResetForDatabaseReloadClearsTempCoreTimers(t *testing.T) {
	store := &managedRuntimeFileStore{
		cache:  make(map[string]*managedRuntimeFileEntry),
		timers: make(map[string]*time.Timer),
	}

	canonical := "core/mihomo/server.yaml"
	diskPath := managedRuntimeDiskPath(canonical)
	t.Cleanup(func() {
		_ = os.Remove(diskPath)
	})
	if err := os.MkdirAll(filepath.Dir(diskPath), 0o755); err != nil {
		t.Fatalf("create temp core dir failed: %v", err)
	}
	if err := os.WriteFile(diskPath, []byte("test"), 0o600); err != nil {
		t.Fatalf("write temp core file failed: %v", err)
	}

	store.timers[canonical] = time.NewTimer(time.Hour)
	store.resetForDatabaseReload()

	if len(store.timers) != 0 {
		t.Fatalf("expected timers to be cleared, got %d", len(store.timers))
	}
	if _, err := os.Stat(diskPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected temp core file to be removed, got err=%v", err)
	}
}

func TestManagedRuntimeWriteSingboxConfigDoesNotPersistDiskFile(t *testing.T) {
	setupManagedRuntimeFileStoreTestDB(t, "managed-runtime-singbox-temp.db")

	configPath := filepath.Join("core", "singbox", "config.json")
	configData := []byte(`{"log":{"level":"info"}}`)
	diskPath := managedRuntimeDiskPath("core/singbox/config.json")
	_ = os.Remove(diskPath)
	t.Cleanup(func() {
		_ = os.Remove(diskPath)
	})

	if err := ManagedRuntimeWriteFile(configPath, configData); err != nil {
		t.Fatalf("write singbox config failed: %v", err)
	}

	if _, err := os.Stat(diskPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected singbox config to remain store-only before materialize, got err=%v", err)
	}
}

func TestMaterializeManagedSingboxConfigCreatesTempDiskFile(t *testing.T) {
	setupManagedRuntimeFileStoreTestDB(t, "managed-runtime-singbox-materialize.db")

	configPath := filepath.Join("core", "singbox", "config.json")
	configData := []byte(`{"log":{"level":"info"}}`)
	diskPath := managedRuntimeDiskPath("core/singbox/config.json")
	_ = os.Remove(diskPath)
	t.Cleanup(func() {
		_ = os.Remove(diskPath)
	})

	if err := ManagedRuntimeWriteFile(configPath, configData); err != nil {
		t.Fatalf("write singbox config failed: %v", err)
	}
	if err := MaterializeManagedRuntimeCoreFile(configPath, time.Minute); err != nil {
		t.Fatalf("materialize singbox config failed: %v", err)
	}

	data, err := os.ReadFile(diskPath)
	if err != nil {
		t.Fatalf("expected singbox config to be materialized to disk: %v", err)
	}
	if string(data) != string(configData) {
		t.Fatalf("unexpected singbox config content: %s", data)
	}
}

func TestDiscardMaterializedSingboxConfigRemovesTempDiskFile(t *testing.T) {
	setupManagedRuntimeFileStoreTestDB(t, "managed-runtime-singbox-discard.db")

	configPath := filepath.Join("core", "singbox", "config.json")
	configData := []byte(`{"log":{"level":"debug"}}`)
	diskPath := managedRuntimeDiskPath("core/singbox/config.json")
	_ = os.Remove(diskPath)
	t.Cleanup(func() {
		_ = os.Remove(diskPath)
	})

	if err := ManagedRuntimeWriteFile(configPath, configData); err != nil {
		t.Fatalf("write singbox config failed: %v", err)
	}
	if err := MaterializeManagedRuntimeCoreFile(configPath, time.Minute); err != nil {
		t.Fatalf("materialize singbox config failed: %v", err)
	}
	DiscardMaterializedManagedRuntimeCoreFile(configPath)

	if _, err := os.Stat(diskPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected materialized singbox config removed after discard, got err=%v", err)
	}
}

func TestManagedRuntimeReadTempSingboxDiskFallbackRemovesDiskFile(t *testing.T) {
	setupManagedRuntimeFileStoreTestDB(t, "managed-runtime-singbox-disk-fallback.db")
	if err := InitManagedRuntimeFileStore(); err != nil {
		t.Fatalf("InitManagedRuntimeFileStore failed: %v", err)
	}

	canonical := "core/singbox/config.json"
	configData := []byte(`{"log":{"level":"warn"}}`)
	diskPath := managedRuntimeDiskPath(canonical)
	_ = os.Remove(diskPath)
	t.Cleanup(func() {
		_ = os.Remove(diskPath)
	})

	if err := os.MkdirAll(filepath.Dir(diskPath), 0o755); err != nil {
		t.Fatalf("create singbox config dir failed: %v", err)
	}
	if err := os.WriteFile(diskPath, configData, 0o600); err != nil {
		t.Fatalf("write disk fallback config failed: %v", err)
	}

	store := &managedRuntimeFileStore{
		cache:  make(map[string]*managedRuntimeFileEntry),
		timers: make(map[string]*time.Timer),
	}
	data, err := store.read(canonical)
	if err != nil {
		t.Fatalf("read persistent disk fallback failed: %v", err)
	}
	if string(data) != string(configData) {
		t.Fatalf("unexpected read content: %s", data)
	}

	if _, err := os.Stat(diskPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected temp disk fallback config removed after load, got err=%v", err)
	}
}

func TestMigrateLegacyFileRemovesTempSingboxDiskFileWhenStoreExists(t *testing.T) {
	setupManagedRuntimeFileStoreTestDB(t, "managed-runtime-singbox-migrate-temp.db")
	if err := InitManagedRuntimeFileStore(); err != nil {
		t.Fatalf("InitManagedRuntimeFileStore failed: %v", err)
	}

	canonical := "core/singbox/config.json"
	dbData := []byte(`{"log":{"level":"error"}}`)
	diskPath := managedRuntimeDiskPath(canonical)
	_ = os.Remove(diskPath)
	t.Cleanup(func() {
		_ = os.Remove(diskPath)
	})

	store := &managedRuntimeFileStore{
		cache:  make(map[string]*managedRuntimeFileEntry),
		timers: make(map[string]*time.Timer),
	}
	if err := store.put(canonical, dbData); err != nil {
		t.Fatalf("write managed config failed: %v", err)
	}
	if err := os.WriteFile(diskPath, []byte(`{"stale":true}`), 0o600); err != nil {
		t.Fatalf("write stale disk config failed: %v", err)
	}
	if err := store.migrateLegacyFile(canonical); err != nil {
		t.Fatalf("migrate temp singbox config failed: %v", err)
	}

	if _, err := os.Stat(diskPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected temp disk config removed after migrate, got err=%v", err)
	}
}

func TestCanonicalManagedRuntimePathMapsSingboxSubdirConfig(t *testing.T) {
	canonical, managed := canonicalManagedRuntimePath(filepath.Join("core", "singbox", "config.json"))
	if !managed {
		t.Fatal("expected singbox config path to be managed")
	}
	if canonical != "core/singbox/config.json" {
		t.Fatalf("expected core/singbox/config.json, got %s", canonical)
	}
}

func TestCanonicalManagedRuntimePathMapsLegacyRootConfigToSingboxConfig(t *testing.T) {
	canonical, managed := canonicalManagedRuntimePath(filepath.Join("core", "config.json"))
	if !managed {
		t.Fatal("expected legacy root config path to be managed")
	}
	if canonical != "core/singbox/config.json" {
		t.Fatalf("expected core/singbox/config.json, got %s", canonical)
	}
}

func TestMigrateLegacyCoreCanonicalAliasMovesRootConfigToSingboxSubdirConfig(t *testing.T) {
	setupManagedRuntimeFileStoreTestDB(t, "managed-runtime-alias.db")
	store := &managedRuntimeFileStore{
		cache:  make(map[string]*managedRuntimeFileEntry),
		timers: make(map[string]*time.Timer),
	}

	sourceCanonical := "core/config.json"
	targetCanonical := "core/singbox/config.json"
	configData := []byte(`{"log":{"level":"panic"}}`)

	if err := store.put(sourceCanonical, configData); err != nil {
		t.Fatalf("write source managed config failed: %v", err)
	}
	if err := store.migrateLegacyCoreCanonicalAlias(sourceCanonical, targetCanonical); err != nil {
		t.Fatalf("migrate singbox config alias failed: %v", err)
	}

	if exists, err := store.exists(sourceCanonical); err != nil {
		t.Fatalf("check source managed config failed: %v", err)
	} else if exists {
		t.Fatal("expected source managed config to be removed")
	}

	if exists, err := store.exists(targetCanonical); err != nil {
		t.Fatalf("check target managed config failed: %v", err)
	} else if !exists {
		t.Fatal("expected target managed config to exist")
	}

	data, err := store.read(targetCanonical)
	if err != nil {
		t.Fatalf("read target managed config failed: %v", err)
	}
	if string(data) != string(configData) {
		t.Fatalf("unexpected target config content: %s", data)
	}
}

func TestManagedRuntimeClearDirJSONFilesDeletesManagedJSONEntries(t *testing.T) {
	db := setupManagedRuntimeFileStoreTestDB(t, "managed-runtime-clear.db")
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db.DB failed: %v", err)
	}

	if err := ManagedRuntimeWriteFile(filepath.Join("sub_json", "remove.json"), []byte(`{"tag":"remove"}`)); err != nil {
		t.Fatalf("write remove.json failed: %v", err)
	}
	if err := ManagedRuntimeWriteFile(filepath.Join("sub_json", "keep.json"), []byte(`{"tag":"keep"}`)); err != nil {
		t.Fatalf("write keep.json failed: %v", err)
	}
	if err := ManagedRuntimeWriteFile(filepath.Join("sub_json", "notes.txt"), []byte("keep")); err != nil {
		t.Fatalf("write notes.txt failed: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- ManagedRuntimeClearDirJSONFiles("sub_json", "keep.json")
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("ManagedRuntimeClearDirJSONFiles failed: %v", err)
		}
	case <-time.After(2 * time.Second):
		_ = sqlDB.Close()
		t.Fatal("ManagedRuntimeClearDirJSONFiles timed out")
	}

	if exists, err := ManagedRuntimeFileExists(filepath.Join("sub_json", "remove.json")); err != nil {
		t.Fatalf("check remove.json failed: %v", err)
	} else if exists {
		t.Fatal("expected remove.json to be deleted")
	}
	if exists, err := ManagedRuntimeFileExists(filepath.Join("sub_json", "keep.json")); err != nil {
		t.Fatalf("check keep.json failed: %v", err)
	} else if !exists {
		t.Fatal("expected keep.json to remain")
	}
	if exists, err := ManagedRuntimeFileExists(filepath.Join("sub_json", "notes.txt")); err != nil {
		t.Fatalf("check notes.txt failed: %v", err)
	} else if !exists {
		t.Fatal("expected notes.txt to remain")
	}
}

func setupManagedRuntimeFileStoreTestDB(t *testing.T, dbName string) *gorm.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), dbName)
	if err := database.InitDB(dbPath); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}

	db := database.GetDB()
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db.DB failed: %v", err)
	}
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	return db
}
