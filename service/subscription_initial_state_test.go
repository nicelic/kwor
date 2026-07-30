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
	if baseline.JSONExtension != "" || baseline.ClashExtension != "" || baseline.ServerTLSStoreEnabled != "true" || baseline.ServerTLSStore != "chrome" || baseline.ClientTLSStoreEnabled != "true" || baseline.ClientTLSStore != "chrome" {
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
	if subscriptionInitialStateSettingValue(t, "subJsonExt") != "" || subscriptionInitialStateSettingValue(t, "serverTlsStoreEnabled") != "true" || subscriptionInitialStateSettingValue(t, "serverTlsStore") != "chrome" || subscriptionInitialStateSettingValue(t, "clientTlsStoreEnabled") != "true" || subscriptionInitialStateSettingValue(t, "clientTlsStore") != "chrome" {
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

func TestSubscriptionInitialStateUpgradesLegacyBaselineWithoutChangingCurrentSettings(t *testing.T) {
	settingService := initSubscriptionInitialStateTestDB(t)
	snapshot, err := settingService.GetSettingsSnapshot(false)
	if err != nil {
		t.Fatalf("load settings snapshot: %v", err)
	}

	changed, err := settingService.ApplySettingsPatch(SettingsPatchRequest{
		ExpectedRevision: snapshot.Revision,
		Changes: map[string]string{
			"subJsonExt":  `{"inbounds":[]}`,
			"subClashExt": "tun:\n  enable: true\ndns:\n  enable: true\n  nameserver:\n    - udp://1.1.1.1\n  default-nameserver:\n    - udp://1.1.1.1\n",
		},
	}, "test")
	if err != nil {
		t.Fatalf("save enabled subscription settings: %v", err)
	}

	legacyBaseline := model.SubscriptionInitialState{
		Id:                    1,
		JSONExtension:         `{"inbounds":[{"type":"tun"}]}`,
		ClashExtension:        "tun:\n  enable: true\n",
		ServerTLSStoreEnabled: "false",
		ServerTLSStore:        "system",
		ClientTLSStoreEnabled: "false",
		ClientTLSStore:        "system",
		BaselineVersion:       1,
	}
	if err := database.GetDB().Save(&legacyBaseline).Error; err != nil {
		t.Fatalf("seed legacy reset baseline: %v", err)
	}

	jsonBeforeReset := subscriptionInitialStateSettingValue(t, "subJsonExt")
	clashBeforeReset := subscriptionInitialStateSettingValue(t, "subClashExt")
	if jsonBeforeReset == "" || clashBeforeReset == "" {
		t.Fatal("test setup did not persist enabled subscription settings")
	}

	if err := settingService.ensureSubscriptionInitialState(); err != nil {
		t.Fatalf("upgrade reset baseline: %v", err)
	}
	var baseline model.SubscriptionInitialState
	if err := database.GetDB().Where("id = ?", 1).First(&baseline).Error; err != nil {
		t.Fatalf("load upgraded reset baseline: %v", err)
	}
	if baseline.BaselineVersion != subscriptionInitialBaselineVersion || baseline.JSONExtension != "" || baseline.ClashExtension != "" || baseline.ServerTLSStoreEnabled != "true" || baseline.ServerTLSStore != "chrome" || baseline.ClientTLSStoreEnabled != "true" || baseline.ClientTLSStore != "chrome" {
		t.Fatalf("legacy reset baseline was not repaired: %#v", baseline)
	}

	legacyBaseline.BaselineVersion = subscriptionInitialBaselineVersion
	if err := database.GetDB().Save(&legacyBaseline).Error; err != nil {
		t.Fatalf("seed invalid current-version reset baseline: %v", err)
	}
	if err := settingService.ensureSubscriptionInitialState(); err != nil {
		t.Fatalf("repair invalid current-version reset baseline: %v", err)
	}
	baseline = model.SubscriptionInitialState{}
	if err := database.GetDB().Where("id = ?", 1).First(&baseline).Error; err != nil {
		t.Fatalf("load repaired current-version reset baseline: %v", err)
	}
	if baseline.BaselineVersion != subscriptionInitialBaselineVersion || baseline.JSONExtension != "" || baseline.ClashExtension != "" || baseline.ServerTLSStoreEnabled != "true" || baseline.ServerTLSStore != "chrome" || baseline.ClientTLSStoreEnabled != "true" || baseline.ClientTLSStore != "chrome" {
		t.Fatalf("invalid current-version reset baseline was not repaired: %#v", baseline)
	}
	if subscriptionInitialStateSettingValue(t, "subJsonExt") != jsonBeforeReset || subscriptionInitialStateSettingValue(t, "subClashExt") != clashBeforeReset {
		t.Fatal("upgrading the reset baseline changed current subscription settings")
	}

	jsonReset, err := (&ConfigService{}).ResetSubscriptionToInitialState("json", changed.Revision, "test")
	if err != nil {
		t.Fatalf("reset JSON subscription after legacy baseline upgrade: %v", err)
	}
	if jsonReset.Values["subJsonExt"] != "" || subscriptionInitialStateSettingValue(t, "subJsonExt") != "" || subscriptionInitialStateSettingValue(t, "serverTlsStoreEnabled") != "true" || subscriptionInitialStateSettingValue(t, "serverTlsStore") != "chrome" || subscriptionInitialStateSettingValue(t, "clientTlsStoreEnabled") != "true" || subscriptionInitialStateSettingValue(t, "clientTlsStore") != "chrome" {
		t.Fatal("JSON reset did not clear the enabled legacy state")
	}
	jsonReloaded, err := settingService.GetSubscriptionSettingsSnapshot("json")
	if err != nil {
		t.Fatalf("reload JSON subscription settings after reset: %v", err)
	}
	if jsonReloaded.Value != "" {
		t.Fatalf("reloaded JSON subscription state = %q, want empty first-installation state", jsonReloaded.Value)
	}

	clashReset, err := (&ConfigService{}).ResetSubscriptionToInitialState("clash", jsonReset.Revision, "test")
	if err != nil {
		t.Fatalf("reset Clash subscription after legacy baseline upgrade: %v", err)
	}
	if clashReset.Values["subClashExt"] != "" || subscriptionInitialStateSettingValue(t, "subClashExt") != "" {
		t.Fatal("Clash reset did not clear the enabled legacy state")
	}
	clashReloaded, err := settingService.GetSubscriptionSettingsSnapshot("clash")
	if err != nil {
		t.Fatalf("reload Clash subscription settings after reset: %v", err)
	}
	if clashReloaded.Value != "" {
		t.Fatalf("reloaded Clash subscription state = %q, want empty first-installation state", clashReloaded.Value)
	}
}
