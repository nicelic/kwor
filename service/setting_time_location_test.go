package service

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/alireza0/s-ui/database"
)

func initTimeLocationSettingTestDB(t *testing.T) *SettingService {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "time-location-settings.db")
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

func TestExtractTimeLocationFromZoneinfoPathSupportsPosixAndRight(t *testing.T) {
	cases := map[string]string{
		"/usr/share/zoneinfo/Asia/Shanghai":       "Asia/Shanghai",
		"/usr/share/zoneinfo/posix/Asia/Shanghai": "Asia/Shanghai",
		"/usr/share/zoneinfo/right/UTC":           "UTC",
	}

	for input, want := range cases {
		if got := extractTimeLocationFromZoneinfoPath(input); got != want {
			t.Fatalf("extractTimeLocationFromZoneinfoPath(%q)=%q want %q", input, got, want)
		}
	}
}

func TestNormalizeTimeLocationNameAcceptsValidIANAOutsidePresetList(t *testing.T) {
	got := normalizeTimeLocationName("Europe/Copenhagen")
	if got != "Europe/Copenhagen" {
		t.Fatalf("normalizeTimeLocationName returned %q want %q", got, "Europe/Copenhagen")
	}
}

func TestEnsureTimeLocationSettingPreservesValidStoredValueOutsidePresetList(t *testing.T) {
	settingService := initTimeLocationSettingTestDB(t)
	if _, err := settingService.GetAllSetting(); err != nil {
		t.Fatalf("seed default settings failed: %v", err)
	}

	if err := settingService.SaveSetting("timeLocation", "Europe/Copenhagen"); err != nil {
		t.Fatalf("SaveSetting failed: %v", err)
	}

	value, err := settingService.ensureTimeLocationSetting()
	if err != nil {
		t.Fatalf("ensureTimeLocationSetting failed: %v", err)
	}
	if value != "Europe/Copenhagen" {
		t.Fatalf("ensureTimeLocationSetting returned %q want %q", value, "Europe/Copenhagen")
	}
}

func TestSettingServiceSavePreservesValidTimeLocationOutsidePresetList(t *testing.T) {
	settingService := initTimeLocationSettingTestDB(t)
	if _, err := settingService.GetAllSetting(); err != nil {
		t.Fatalf("seed default settings failed: %v", err)
	}

	payload, err := json.Marshal(map[string]string{
		"timeLocation": "Europe/Copenhagen",
	})
	if err != nil {
		t.Fatalf("marshal payload failed: %v", err)
	}

	if err := settingService.Save(database.GetDB(), payload); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	settings, err := settingService.GetAllSetting()
	if err != nil {
		t.Fatalf("GetAllSetting failed: %v", err)
	}
	if got := (*settings)["timeLocation"]; got != "Europe/Copenhagen" {
		t.Fatalf("timeLocation=%q want %q", got, "Europe/Copenhagen")
	}
}
