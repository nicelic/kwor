package service

import (
	"strings"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
)

type selectorTagReplacement struct {
	legacy     string
	normalized string
}

var legacySubscriptionSelectorTagReplacements = []selectorTagReplacement{
	{legacy: "🚀 节点选择", normalized: "节点选择"},
	{legacy: "🚀节点选择", normalized: "节点选择"},
	{legacy: "\\U0001F680 节点选择", normalized: "节点选择"},
	{legacy: "\\U0001F680节点选择", normalized: "节点选择"},
	{legacy: "🎈 自动选择", normalized: "自动选择"},
	{legacy: "🎈自动选择", normalized: "自动选择"},
	{legacy: "\\U0001F388 自动选择", normalized: "自动选择"},
	{legacy: "\\U0001F388自动选择", normalized: "自动选择"},
	{legacy: "🎯 全球直连", normalized: "全球直连"},
	{legacy: "🎯全球直连", normalized: "全球直连"},
	{legacy: "\\U0001F3AF 全球直连", normalized: "全球直连"},
	{legacy: "\\U0001F3AF全球直连", normalized: "全球直连"},
	{legacy: "🛑 全球拦截", normalized: "全球拦截"},
	{legacy: "🛑全球拦截", normalized: "全球拦截"},
	{legacy: "\\U0001F6D1 全球拦截", normalized: "全球拦截"},
	{legacy: "\\U0001F6D1全球拦截", normalized: "全球拦截"},
	{legacy: "🐟 漏网之鱼", normalized: "漏网之鱼"},
	{legacy: "🐟漏网之鱼", normalized: "漏网之鱼"},
	{legacy: "\\U0001F41F 漏网之鱼", normalized: "漏网之鱼"},
	{legacy: "\\U0001F41F漏网之鱼", normalized: "漏网之鱼"},
}

func normalizeLegacySubscriptionSelectorTags(raw string) string {
	normalized := raw
	for _, replacement := range legacySubscriptionSelectorTagReplacements {
		normalized = strings.ReplaceAll(normalized, replacement.legacy, replacement.normalized)
	}
	return normalized
}

// MigrateLegacySubscriptionSelectorTags normalizes legacy selector tags in settings payloads.
// This migration is idempotent and safe to run on every startup.
func MigrateLegacySubscriptionSelectorTags() (int, error) {
	db := database.GetDB()
	if db == nil {
		return 0, nil
	}

	keys := []string{"subJsonExt", "subClashExt"}
	settings := make([]model.Setting, 0, len(keys))
	if err := db.Model(model.Setting{}).Where("key IN ?", keys).Find(&settings).Error; err != nil {
		return 0, err
	}

	updated := 0
	for _, setting := range settings {
		normalized := normalizeLegacySubscriptionSelectorTags(setting.Value)
		if normalized == setting.Value {
			continue
		}
		if err := db.Model(model.Setting{}).Where("id = ?", setting.Id).Update("value", normalized).Error; err != nil {
			return updated, err
		}
		updated++
	}

	return updated, nil
}
