package service

import (
	"encoding/json"
	"testing"

	"gorm.io/gorm"
)

func TestCoreLogLevelValidationUsesCoreSpecificValues(t *testing.T) {
	for _, level := range []string{"debug", "info", "warn", "error", "fatal", "panic"} {
		if got, err := normalizeSingboxCoreLogLevel(level); err != nil || got != level {
			t.Fatalf("normalize sing-box level %q = %q, %v", level, got, err)
		}
	}
	if _, err := normalizeSingboxCoreLogLevel("silent"); err == nil {
		t.Fatal("sing-box accepted unsupported silent level")
	}

	for _, level := range []string{"silent", "error", "warning", "info", "debug"} {
		if got, err := normalizeMihomoCoreLogLevel(level); err != nil || got != level {
			t.Fatalf("normalize mihomo level %q = %q, %v", level, got, err)
		}
	}
	if _, err := normalizeMihomoCoreLogLevel("panic"); err == nil {
		t.Fatal("mihomo accepted unsupported panic level")
	}
}

func TestSingboxRuntimeLogLevelIgnoresLegacyBaseLog(t *testing.T) {
	db := setupManagedRuntimeFileStoreTestDB(t, "singbox-core-log-level.db")
	upsertCoreLogLevelTestSetting(t, db, "config", `{"log":{"level":"info","output":"legacy.log"},"dns":{"rules":[]},"route":{"rules":[]}}`)
	upsertCoreLogLevelTestSetting(t, db, singboxCoreLogLevelSettingKey, "debug")

	config, err := (&ProManagerService{ConfigService: &ConfigService{}}).GenerateFullConfigWithDB(db)
	if err != nil {
		t.Fatalf("GenerateFullConfigWithDB failed: %v", err)
	}
	var log map[string]interface{}
	if err := json.Unmarshal(config.Log, &log); err != nil {
		t.Fatalf("decode generated sing-box log failed: %v", err)
	}
	if len(log) != 1 || log["level"] != "debug" {
		t.Fatalf("generated sing-box log = %#v, want only debug", log)
	}
}

func TestMihomoRuntimeLogLevelIgnoresLegacyBaseLog(t *testing.T) {
	db := initMihomoManagerFinalFallbackTestDB(t, "mihomo-core-log-level.db")
	upsertCoreLogLevelTestSetting(t, db, "mihomo_config", `{
		"log":{"level":"info"},
		"route":{"rules":[],"rule_set":[]}
	}`)
	upsertCoreLogLevelTestSetting(t, db, mihomoCoreLogLevelSettingKey, "warning")

	document, err := NewMihomoManagerService().GenerateServerDocument()
	if err != nil {
		t.Fatalf("GenerateServerDocument failed: %v", err)
	}
	if document["log-level"] != "warning" {
		t.Fatalf("generated mihomo log-level = %#v, want warning", document["log-level"])
	}
}

func upsertCoreLogLevelTestSetting(t *testing.T, db *gorm.DB, key, value string) {
	t.Helper()
	if err := db.Transaction(func(tx *gorm.DB) error {
		return upsertSetting(tx, key, value)
	}); err != nil {
		t.Fatalf("upsert setting %s failed: %v", key, err)
	}
}
