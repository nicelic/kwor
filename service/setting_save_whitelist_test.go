package service

import (
	"encoding/json"
	"testing"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
)

func TestSettingServiceSaveOnlyUpdatesGeneralSettingsKeys(t *testing.T) {
	settingService := initTimeLocationSettingTestDB(t)
	if _, err := settingService.GetAllSetting(); err != nil {
		t.Fatalf("seed default settings failed: %v", err)
	}

	payload, err := json.Marshal(map[string]string{
		"webPort":          "9443",
		"systemMTUEnabled": "true",
		"unexpectedKey":    "unexpected-value",
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
	if got := (*settings)["webPort"]; got != "9443" {
		t.Fatalf("webPort=%q want %q", got, "9443")
	}
	if got := (*settings)[systemMTUEnabledKey]; got != "false" {
		t.Fatalf("%s=%q want %q", systemMTUEnabledKey, got, "false")
	}

	var count int64
	if err := database.GetDB().Model(&model.Setting{}).Where("key = ?", "unexpectedKey").Count(&count).Error; err != nil {
		t.Fatalf("count unexpected setting failed: %v", err)
	}
	if count != 0 {
		t.Fatalf("unexpected key was persisted: count=%d", count)
	}
}
