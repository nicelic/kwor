package service

import (
	"path/filepath"
	"testing"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
)

func TestReadSystemBootID(t *testing.T) {
	bootID, err := ReadSystemBootID()
	if err != nil {
		t.Fatalf("ReadSystemBootID returned unexpected error: %v", err)
	}
	if bootID == "" {
		t.Fatalf("ReadSystemBootID returned empty string")
	}
}

func TestEvaluateBootLifecycleState(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "boot-test.db")
	if err := database.InitDB(dbPath); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	if sqlDB, err := database.GetDB().DB(); err == nil && sqlDB != nil {
		t.Cleanup(func() { _ = sqlDB.Close() })
	}

	// 1. 初次启动：无历史记录，判定为 isHostReboot = true
	isReboot, bootID, err := EvaluateBootLifecycleState()
	if err != nil {
		t.Fatalf("EvaluateBootLifecycleState failed: %v", err)
	}
	if !isReboot {
		t.Fatalf("expected isHostReboot=true on initial run, got false")
	}
	if bootID == "" {
		t.Fatalf("expected non-empty bootID")
	}

	// 验证数据库中已经保存了 bootID
	var setting model.Setting
	if err := database.GetDB().Where("key = ?", systemBootIDKey).First(&setting).Error; err != nil {
		t.Fatalf("failed to query saved bootID from db: %v", err)
	}
	if setting.Value != bootID {
		t.Fatalf("saved bootID mismatch: got %q, want %q", setting.Value, bootID)
	}

	// 2. 第二次运行：bootID 未改变，判定为仅面板重启 isHostReboot = false
	isRebootSecond, bootIDSecond, err := EvaluateBootLifecycleState()
	if err != nil {
		t.Fatalf("second EvaluateBootLifecycleState failed: %v", err)
	}
	if isRebootSecond {
		t.Fatalf("expected isHostReboot=false when bootID unchanged, got true")
	}
	if bootIDSecond != bootID {
		t.Fatalf("bootID changed unexpectedly: got %q, want %q", bootIDSecond, bootID)
	}

	// 3. 模拟整机重启：人为将数据库中的 bootID 改为旧值
	fakeOldBootID := "11111111-2222-3333-4444-555555555555"
	setting.Value = fakeOldBootID
	if err := database.GetDB().Save(&setting).Error; err != nil {
		t.Fatalf("failed to update fake bootID: %v", err)
	}

	// 4. 再次执行判定：bootID 不一致，判定为整机重启 isHostReboot = true
	isRebootThird, bootIDThird, err := EvaluateBootLifecycleState()
	if err != nil {
		t.Fatalf("third EvaluateBootLifecycleState failed: %v", err)
	}
	if !isRebootThird {
		t.Fatalf("expected isHostReboot=true when bootID changed, got false")
	}
	if bootIDThird == fakeOldBootID {
		t.Fatalf("bootID should have been updated to current bootID")
	}
}
