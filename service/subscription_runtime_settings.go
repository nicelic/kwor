package service

import (
	"strconv"
	"strings"
	"sync"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/logger"
	"github.com/alireza0/s-ui/util/common"
)

var subscriptionRuntimeSettingKeys = []string{
	"subEncode",
	"subShowInfo",
	"subUpdates",
	"clientTlsStoreEnabled",
	"clientTlsStore",
}

var subscriptionRuntimeSettingsCache = struct {
	sync.RWMutex
	loaded     bool
	values     map[string]string
	generation uint64
}{}

func init() {
	database.RegisterDBResetHook(invalidateSubscriptionRuntimeSettings)
}

func invalidateSubscriptionRuntimeSettings() {
	subscriptionRuntimeSettingsCache.Lock()
	subscriptionRuntimeSettingsCache.loaded = false
	subscriptionRuntimeSettingsCache.values = nil
	subscriptionRuntimeSettingsCache.generation++
	subscriptionRuntimeSettingsCache.Unlock()
}

// SubscriptionRuntimeSettingsGeneration changes whenever a setting that can
// affect subscription rendering is committed. Callers can include it in
// request-coalescing keys so a request started after a settings change never
// joins a render built from the previous snapshot.
func SubscriptionRuntimeSettingsGeneration() uint64 {
	subscriptionRuntimeSettingsCache.RLock()
	defer subscriptionRuntimeSettingsCache.RUnlock()
	return subscriptionRuntimeSettingsCache.generation
}

func isSubscriptionRuntimeSettingsKey(key string) bool {
	for _, candidate := range subscriptionRuntimeSettingKeys {
		if key == candidate {
			return true
		}
	}
	return false
}

func isSubscriptionRenderSettingKey(key string) bool {
	if isSubscriptionRuntimeSettingsKey(key) {
		return true
	}
	switch key {
	case "subJsonExt", "subClashExt":
		return true
	default:
		return false
	}
}

func (s *SettingService) getSubscriptionRuntimeSetting(key string) (string, error) {
	subscriptionRuntimeSettingsCache.RLock()
	if subscriptionRuntimeSettingsCache.loaded {
		value, exists := subscriptionRuntimeSettingsCache.values[key]
		subscriptionRuntimeSettingsCache.RUnlock()
		if !exists {
			return "", common.NewErrorf("unknown subscription runtime setting: %s", key)
		}
		return value, nil
	}
	subscriptionRuntimeSettingsCache.RUnlock()

	subscriptionRuntimeSettingsCache.Lock()
	defer subscriptionRuntimeSettingsCache.Unlock()
	if !subscriptionRuntimeSettingsCache.loaded {
		db := database.GetDB()
		if db == nil {
			return "", common.NewError("database is not ready")
		}
		rows := make([]model.Setting, 0, len(subscriptionRuntimeSettingKeys))
		if err := db.Where("key IN ?", subscriptionRuntimeSettingKeys).Find(&rows).Error; err != nil {
			return "", err
		}
		values := make(map[string]string, len(subscriptionRuntimeSettingKeys))
		for _, row := range rows {
			values[row.Key] = row.Value
		}
		for _, candidate := range subscriptionRuntimeSettingKeys {
			if _, exists := values[candidate]; exists {
				continue
			}
			defaultValue, err := s.defaultSettingValue(candidate)
			if err != nil {
				return "", err
			}
			values[candidate] = defaultValue
		}
		subscriptionRuntimeSettingsCache.values = values
		subscriptionRuntimeSettingsCache.loaded = true
	}
	value, exists := subscriptionRuntimeSettingsCache.values[key]
	if !exists {
		return "", common.NewErrorf("unknown subscription runtime setting: %s", key)
	}
	return value, nil
}

func (s *SettingService) getSubscriptionRuntimeBool(key string) (bool, error) {
	value, err := s.getSubscriptionRuntimeSetting(key)
	if err != nil {
		return false, err
	}
	parsed, parseErr := strconv.ParseBool(strings.TrimSpace(value))
	if parseErr == nil {
		return parsed, nil
	}
	defaultValue, defaultErr := strconv.ParseBool(defaultValueMap[key])
	if defaultErr != nil {
		return false, parseErr
	}
	logger.Warningf("invalid bool subscription runtime setting %q=%q, fallback to default", key, value)
	return defaultValue, nil
}

func (s *SettingService) getSubscriptionRuntimeInt(key string) (int, error) {
	value, err := s.getSubscriptionRuntimeSetting(key)
	if err != nil {
		return 0, err
	}
	parsed, parseErr := strconv.Atoi(strings.TrimSpace(value))
	if parseErr == nil {
		return parsed, nil
	}
	defaultValue, defaultErr := strconv.Atoi(defaultValueMap[key])
	if defaultErr != nil {
		return 0, parseErr
	}
	logger.Warningf("invalid int subscription runtime setting %q=%q, fallback to default", key, value)
	return defaultValue, nil
}
