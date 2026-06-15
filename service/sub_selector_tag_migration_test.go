package service

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
)

func TestMigrateLegacySubscriptionSelectorTags(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "selector-tag-migration.db")
	if err := database.InitDB(dbPath); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	t.Cleanup(func() {
		sqlDB, err := database.GetDB().DB()
		if err == nil {
			_ = sqlDB.Close()
		}
	})

	settingService := &SettingService{}
	if _, err := settingService.GetAllSetting(); err != nil {
		t.Fatalf("GetAllSetting failed: %v", err)
	}

	subJsonExt := `{"route_final":"🐟 漏网之鱼","rules":[{"action":"route","outbound":"🚀 节点选择"}],"dns":{"servers":[{"detour":"\\U0001F3AF 全球直连"}]}}`
	subClashExt := "rules:\n  - MATCH,\\U0001F680 节点选择\nproxy-groups:\n  - name: \"🎈 自动选择\"\n"

	if err := settingService.SaveSetting("subJsonExt", subJsonExt); err != nil {
		t.Fatalf("SaveSetting subJsonExt failed: %v", err)
	}
	if err := settingService.SaveSetting("subClashExt", subClashExt); err != nil {
		t.Fatalf("SaveSetting subClashExt failed: %v", err)
	}

	updated, err := MigrateLegacySubscriptionSelectorTags()
	if err != nil {
		t.Fatalf("MigrateLegacySubscriptionSelectorTags failed: %v", err)
	}
	if updated != 2 {
		t.Fatalf("updated rows = %d, want 2", updated)
	}

	db := database.GetDB()
	settings := []model.Setting{}
	if err := db.Model(model.Setting{}).Where("key IN ?", []string{"subJsonExt", "subClashExt"}).Find(&settings).Error; err != nil {
		t.Fatalf("query settings failed: %v", err)
	}
	if len(settings) != 2 {
		t.Fatalf("settings count = %d, want 2", len(settings))
	}

	for _, setting := range settings {
		for _, legacy := range []string{
			"🚀", "🎈", "🎯", "🛑", "🐟",
			"\\U0001F680", "\\U0001F388", "\\U0001F3AF", "\\U0001F6D1", "\\U0001F41F",
		} {
			if strings.Contains(setting.Value, legacy) {
				t.Fatalf("setting %s still contains legacy token %q: %s", setting.Key, legacy, setting.Value)
			}
		}
	}

	subJsonValue, err := settingService.GetSubJsonExt()
	if err != nil {
		t.Fatalf("GetSubJsonExt failed: %v", err)
	}
	if !strings.Contains(subJsonValue, "节点选择") || !strings.Contains(subJsonValue, "漏网之鱼") || !strings.Contains(subJsonValue, "全球直连") {
		t.Fatalf("subJsonExt not normalized as expected: %s", subJsonValue)
	}

	subClashValue, err := settingService.GetSubClashExt()
	if err != nil {
		t.Fatalf("GetSubClashExt failed: %v", err)
	}
	if !strings.Contains(subClashValue, "MATCH,节点选择") || !strings.Contains(subClashValue, "自动选择") {
		t.Fatalf("subClashExt not normalized as expected: %s", subClashValue)
	}

	updated, err = MigrateLegacySubscriptionSelectorTags()
	if err != nil {
		t.Fatalf("second MigrateLegacySubscriptionSelectorTags failed: %v", err)
	}
	if updated != 0 {
		t.Fatalf("second updated rows = %d, want 0", updated)
	}
}
