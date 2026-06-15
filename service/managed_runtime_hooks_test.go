package service

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/alireza0/s-ui/config"
	"github.com/alireza0/s-ui/database"
	"gorm.io/gorm"
)

func TestQueueManagedRuntimeHook_TransactionCommitRunsHookAfterCommit(t *testing.T) {
	db := setupManagedRuntimeHookTestDB(t, "managed-runtime-hook-commit.db")
	filePath := filepath.Join(config.GetDataDir(), "sub_json", fmt.Sprintf("managed-runtime-hook-commit-%d.json", time.Now().UnixNano()))

	if err := ManagedRuntimeWriteFile(filePath, []byte(`{"tag":"commit"}`)); err != nil {
		t.Fatalf("ManagedRuntimeWriteFile failed: %v", err)
	}

	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("begin tx failed: %v", tx.Error)
	}

	if err := QueueManagedRuntimeHook(tx, func() error {
		return ManagedRuntimeDeleteFile(filePath)
	}); err != nil {
		tx.Rollback()
		t.Fatalf("QueueManagedRuntimeHook failed: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- tx.Commit().Error
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("commit failed: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("commit timed out while waiting for managed runtime hook")
	}

	exists, err := ManagedRuntimeFileExists(filePath)
	if err != nil {
		t.Fatalf("ManagedRuntimeFileExists failed: %v", err)
	}
	if exists {
		t.Fatalf("expected managed runtime file to be deleted after commit: %s", filePath)
	}
}

func TestQueueManagedRuntimeHook_TransactionRollbackDiscardsHook(t *testing.T) {
	db := setupManagedRuntimeHookTestDB(t, "managed-runtime-hook-rollback.db")
	filePath := filepath.Join(config.GetDataDir(), "sub_json", fmt.Sprintf("managed-runtime-hook-rollback-%d.json", time.Now().UnixNano()))

	if err := ManagedRuntimeWriteFile(filePath, []byte(`{"tag":"rollback"}`)); err != nil {
		t.Fatalf("ManagedRuntimeWriteFile failed: %v", err)
	}

	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("begin tx failed: %v", tx.Error)
	}

	if err := QueueManagedRuntimeHook(tx, func() error {
		return ManagedRuntimeDeleteFile(filePath)
	}); err != nil {
		tx.Rollback()
		t.Fatalf("QueueManagedRuntimeHook failed: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- tx.Rollback().Error
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("rollback failed: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("rollback timed out while discarding managed runtime hook")
	}

	exists, err := ManagedRuntimeFileExists(filePath)
	if err != nil {
		t.Fatalf("ManagedRuntimeFileExists failed: %v", err)
	}
	if !exists {
		t.Fatalf("expected managed runtime file to remain after rollback: %s", filePath)
	}

	if err := ManagedRuntimeDeleteFile(filePath); err != nil {
		t.Fatalf("ManagedRuntimeDeleteFile cleanup failed: %v", err)
	}
}

func setupManagedRuntimeHookTestDB(t *testing.T, dbName string) *gorm.DB {
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
