package service

import (
	"path/filepath"
	"testing"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
)

func initSubscriptionInitialStateTestDB(t *testing.T) *SettingService {
	t.Helper()
	if err := database.InitDB(filepath.Join(t.TempDir(), "subscription-initial-state.db")); err != nil {
		t.Fatalf("init database: %v", err)
	}
	if sqlDB, err := database.GetDB().DB(); err == nil && sqlDB != nil {
		t.Cleanup(func() { _ = sqlDB.Close() })
	}
	return &SettingService{}
}

func subscriptionInitialStateSettingValue(t *testing.T, key string) string {
	t.Helper()
	var setting model.Setting
	if err := database.GetDB().Where("key = ?", key).First(&setting).Error; err != nil {
		t.Fatalf("load setting %s: %v", key, err)
	}
	return setting.Value
}

func TestSubscriptionInitialStateRestoresJSONAndClashInstallValues(t *testing.T) {
	settingService := initSubscriptionInitialStateTestDB(t)
	snapshot, err := settingService.GetSettingsSnapshot(false)
	if err != nil {
		t.Fatalf("load settings snapshot: %v", err)
	}

	var baseline model.SubscriptionInitialState
	if err := database.GetDB().Where("id = ?", 1).First(&baseline).Error; err != nil {
		t.Fatalf("load subscription initial state: %v", err)
	}
	if baseline.JSONExtension != "" || baseline.ClashExtension != "" || baseline.ServerTLSStoreEnabled != "true" || baseline.ClientTLSStore != "chrome" {
		t.Fatalf("unexpected installation baseline: %#v", baseline)
	}

	jsonChange, err := settingService.ApplySettingsPatch(SettingsPatchRequest{
		ExpectedRevision: snapshot.Revision,
		Changes: map[string]string{
			"subJsonExt":            `{"inbounds":[]}`,
			"clientTlsStoreEnabled": "false",
			"clientTlsStore":        "system",
		},
	}, "test")
	if err != nil {
		t.Fatalf("save JSON test state: %v", err)
	}
	jsonReset, err := (&ConfigService{}).ResetSubscriptionToInitialState("json", jsonChange.Revision, "test")
	if err != nil {
		t.Fatalf("reset JSON subscription: %v", err)
	}
	if jsonReset.Kind != "json" || jsonReset.Values["subJsonExt"] != "" || jsonReset.Values["clientTlsStoreEnabled"] != "true" || jsonReset.Values["clientTlsStore"] != "chrome" {
		t.Fatalf("unexpected JSON reset result: %#v", jsonReset)
	}
	if subscriptionInitialStateSettingValue(t, "subJsonExt") != "" || subscriptionInitialStateSettingValue(t, "clientTlsStoreEnabled") != "true" || subscriptionInitialStateSettingValue(t, "clientTlsStore") != "chrome" {
		t.Fatal("JSON reset did not restore the installation values")
	}

	clashChange, err := settingService.ApplySettingsPatch(SettingsPatchRequest{
		ExpectedRevision: jsonReset.Revision,
		Changes: map[string]string{
			"subClashExt": "tun:\n  enable: true\ndns:\n  enable: true\n  nameserver:\n    - udp://1.1.1.1\n  default-nameserver:\n    - udp://1.1.1.1\n",
		},
	}, "test")
	if err != nil {
		t.Fatalf("save Clash test state: %v", err)
	}
	clashReset, err := (&ConfigService{}).ResetSubscriptionToInitialState("clash", clashChange.Revision, "test")
	if err != nil {
		t.Fatalf("reset Clash subscription: %v", err)
	}
	if clashReset.Kind != "clash" || clashReset.Values["subClashExt"] != "" || subscriptionInitialStateSettingValue(t, "subClashExt") != "" {
		t.Fatalf("Clash reset did not restore the installation value: %#v", clashReset)
	}

	var audit model.Changes
	if err := database.GetDB().Where("key = ? AND action = ?", "settings", "subscription-initial-reset").Order("id DESC").First(&audit).Error; err != nil {
		t.Fatalf("load reset audit: %v", err)
	}
}
