package service

import (
	"errors"
	"fmt"
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

func TestManagedRuntimeInitializationPrunesOnlyObsoleteJSONArtifacts(t *testing.T) {
	db := setupManagedRuntimeFileStoreTestDB(t, "managed-runtime-obsolete-cleanup.db")

	obsoletePaths := []string{
		"Inbound/inbound.json",
		"Inbound/example_meta.json",
		"outbound/example.json",
		"sub_manager/example_meta.json",
		"sub_json/example.json",
	}
	preservedPaths := []string{
		"core/singbox/config.json",
		"core/mihomo/server.yaml",
		"core/" + mihomoInboundMetaFilename,
		"sub_json/notes.txt",
	}

	insertEntry := func(canonical string, content []byte) {
		t.Helper()
		entry := newManagedRuntimeFileEntry(canonical, content)
		statement := fmt.Sprintf(`INSERT INTO %s (path, dir_path, file_name, ext, content, size, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?)`, managedRuntimeFileTable)
		if err := db.Exec(
			statement,
			entry.Path,
			entry.DirPath,
			entry.FileName,
			entry.Ext,
			entry.Content,
			entry.Size,
			entry.UpdatedAt,
		).Error; err != nil {
			t.Fatalf("insert managed runtime entry %s failed: %v", canonical, err)
		}
	}
	for _, canonical := range obsoletePaths {
		insertEntry(canonical, []byte(`{"stale":true}`))
	}
	for _, canonical := range preservedPaths {
		insertEntry(canonical, []byte(`{"keep":true}`))
	}

	runtimeManagedFiles.cacheMu.Lock()
	for _, canonical := range obsoletePaths {
		runtimeManagedFiles.cache[canonical] = newManagedRuntimeFileEntry(canonical, []byte(`{"cached":true}`))
	}
	runtimeManagedFiles.cacheMu.Unlock()
	runtimeManagedFiles.timerMu.Lock()
	runtimeManagedFiles.timers[obsoletePaths[0]] = time.NewTimer(time.Hour)
	runtimeManagedFiles.timerMu.Unlock()

	unique := fmt.Sprintf("obsolete-cleanup-%d", time.Now().UnixNano())
	var staleDiskPaths []string
	for _, root := range obsoleteManagedRuntimeJSONRoots {
		dirPath := filepath.Join(configDataDirForRuntimeTest(), root)
		if err := os.MkdirAll(dirPath, 0o755); err != nil {
			t.Fatalf("create obsolete runtime directory %s failed: %v", root, err)
		}
		stalePath := filepath.Join(dirPath, unique+".json")
		if err := os.WriteFile(stalePath, []byte(`{"stale":true}`), 0o600); err != nil {
			t.Fatalf("write obsolete runtime file %s failed: %v", root, err)
		}
		staleDiskPaths = append(staleDiskPaths, stalePath)
	}

	subJSONDir := filepath.Join(configDataDirForRuntimeTest(), "sub_json")
	unknownPath := filepath.Join(subJSONDir, unique+".txt")
	if err := os.WriteFile(unknownPath, []byte("keep"), 0o600); err != nil {
		t.Fatalf("write unknown extension file failed: %v", err)
	}
	nestedDir := filepath.Join(subJSONDir, unique+"-nested")
	nestedJSONPath := filepath.Join(nestedDir, "keep.json")
	if err := os.MkdirAll(nestedDir, 0o755); err != nil {
		t.Fatalf("create nested directory failed: %v", err)
	}
	if err := os.WriteFile(nestedJSONPath, []byte(`{"keep":true}`), 0o600); err != nil {
		t.Fatalf("write nested JSON failed: %v", err)
	}

	symlinkTarget := filepath.Join(t.TempDir(), "symlink-target.json")
	if err := os.WriteFile(symlinkTarget, []byte(`{"keep":true}`), 0o600); err != nil {
		t.Fatalf("write symlink target failed: %v", err)
	}
	symlinkPath := filepath.Join(subJSONDir, unique+"-link.json")
	symlinkCreated := os.Symlink(symlinkTarget, symlinkPath) == nil

	t.Cleanup(func() {
		for _, filePath := range staleDiskPaths {
			_ = os.Remove(filePath)
		}
		_ = os.Remove(symlinkPath)
		_ = os.Remove(nestedJSONPath)
		_ = os.Remove(nestedDir)
		_ = os.Remove(unknownPath)
		for _, root := range obsoleteManagedRuntimeJSONRoots {
			_ = os.Remove(filepath.Join(configDataDirForRuntimeTest(), root))
		}
	})

	if err := InitManagedRuntimeFileStore(); err != nil {
		t.Fatalf("InitManagedRuntimeFileStore failed: %v", err)
	}

	for _, canonical := range obsoletePaths {
		var count int64
		if err := db.Table(managedRuntimeFileTable).Where("path = ?", canonical).Count(&count).Error; err != nil {
			t.Fatalf("count obsolete entry %s failed: %v", canonical, err)
		}
		if count != 0 {
			t.Fatalf("expected obsolete entry %s to be removed, count=%d", canonical, count)
		}
	}
	for _, canonical := range preservedPaths {
		var count int64
		if err := db.Table(managedRuntimeFileTable).Where("path = ?", canonical).Count(&count).Error; err != nil {
			t.Fatalf("count preserved entry %s failed: %v", canonical, err)
		}
		if count != 1 {
			t.Fatalf("expected preserved entry %s to remain, count=%d", canonical, count)
		}
	}

	for _, filePath := range staleDiskPaths {
		if _, err := os.Lstat(filePath); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("expected direct obsolete JSON %s to be removed, err=%v", filePath, err)
		}
	}
	if data, err := os.ReadFile(unknownPath); err != nil || string(data) != "keep" {
		t.Fatalf("expected unknown extension file to remain, data=%q err=%v", data, err)
	}
	if _, err := os.Stat(nestedJSONPath); err != nil {
		t.Fatalf("expected nested JSON to remain: %v", err)
	}
	if symlinkCreated {
		if _, err := os.Lstat(symlinkPath); err != nil {
			t.Fatalf("expected JSON symlink to remain: %v", err)
		}
		if _, err := os.Stat(symlinkTarget); err != nil {
			t.Fatalf("expected symlink target to remain: %v", err)
		}
	}

	runtimeManagedFiles.cacheMu.RLock()
	for _, canonical := range obsoletePaths {
		if _, exists := runtimeManagedFiles.cache[canonical]; exists {
			runtimeManagedFiles.cacheMu.RUnlock()
			t.Fatalf("expected obsolete cache entry %s to be removed", canonical)
		}
	}
	runtimeManagedFiles.cacheMu.RUnlock()
	if runtimeManagedFiles.hasActiveCleanup(obsoletePaths[0]) {
		t.Fatalf("expected obsolete cleanup timer to be cancelled")
	}
}

func configDataDirForRuntimeTest() string {
	return filepath.Dir(managedRuntimeDiskPath("core"))
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
