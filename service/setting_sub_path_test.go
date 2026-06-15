package service

import (
	"encoding/json"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/alireza0/s-ui/database"
)

var subPathPattern = regexp.MustCompile(`^/[A-Z]{3}[0-9]{3}/$`)

func initSubPathSettingTestDB(t *testing.T) *SettingService {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "sub-path-settings.db")
	if err := database.InitDB(dbPath); err != nil {
		t.Fatalf("init db failed: %v", err)
	}
	if sqlDB, err := database.GetDB().DB(); err == nil && sqlDB != nil {
		t.Cleanup(func() {
			_ = sqlDB.Close()
		})
	}

	return &SettingService{}
}

func TestSettingServiceGetAllSetting_GeneratesRandomSubPath(t *testing.T) {
	settingService := initSubPathSettingTestDB(t)

	settings, err := settingService.GetAllSetting()
	if err != nil {
		t.Fatalf("GetAllSetting failed: %v", err)
	}

	subPath := (*settings)["subPath"]
	if !subPathPattern.MatchString(subPath) {
		t.Fatalf("unexpected generated subPath: %q", subPath)
	}

	storedPath, err := settingService.GetSubPath()
	if err != nil {
		t.Fatalf("GetSubPath failed: %v", err)
	}
	if storedPath != subPath {
		t.Fatalf("stored subPath mismatch: got=%q want=%q", storedPath, subPath)
	}
}

func TestSettingServiceSave_BlankSubPathRegeneratesRandomPath(t *testing.T) {
	settingService := initSubPathSettingTestDB(t)
	if _, err := settingService.GetAllSetting(); err != nil {
		t.Fatalf("seed default settings failed: %v", err)
	}
	if err := settingService.SetSubPath("/KEEPME/"); err != nil {
		t.Fatalf("SetSubPath failed: %v", err)
	}

	payload, err := json.Marshal(map[string]string{
		"subPath": "   ",
	})
	if err != nil {
		t.Fatalf("marshal payload failed: %v", err)
	}
	if err := settingService.Save(database.GetDB(), payload); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	subPath, err := settingService.GetSubPath()
	if err != nil {
		t.Fatalf("GetSubPath failed: %v", err)
	}
	if !subPathPattern.MatchString(subPath) {
		t.Fatalf("blank subPath should regenerate random value, got %q", subPath)
	}
	if subPath == "/KEEPME/" {
		t.Fatalf("blank subPath should not keep previous custom value")
	}
}

func TestSettingServiceSave_CustomSubPathPreservesUserInput(t *testing.T) {
	settingService := initSubPathSettingTestDB(t)
	if _, err := settingService.GetAllSetting(); err != nil {
		t.Fatalf("seed default settings failed: %v", err)
	}

	payload, err := json.Marshal(map[string]string{
		"subPath": "  abc123  ",
	})
	if err != nil {
		t.Fatalf("marshal payload failed: %v", err)
	}
	if err := settingService.Save(database.GetDB(), payload); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	subPath, err := settingService.GetSubPath()
	if err != nil {
		t.Fatalf("GetSubPath failed: %v", err)
	}
	if subPath != "/abc123/" {
		t.Fatalf("custom subPath mismatch: got=%q want=%q", subPath, "/abc123/")
	}
}
