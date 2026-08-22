package service

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/logger"
	"github.com/alireza0/s-ui/util/common"
	"gorm.io/gorm"
)

const (
	singboxCoreLogLevelSettingKey = "singboxCoreLogLevel"
	mihomoCoreLogLevelSettingKey  = "mihomoCoreLogLevel"

	defaultSingboxCoreLogLevel = "panic"
	defaultMihomoCoreLogLevel  = "silent"
)

type CoreLogLevelSaveResult struct {
	Level   string `json:"level"`
	Changed bool   `json:"changed"`
}

func normalizeSingboxCoreLogLevel(value string) (string, error) {
	level := strings.ToLower(strings.TrimSpace(value))
	switch level {
	case "debug", "info", "warn", "error", "fatal", "panic":
		return level, nil
	default:
		return "", common.NewErrorf("unsupported sing-box log level: %s", value)
	}
}

func normalizeMihomoCoreLogLevel(value string) (string, error) {
	level := strings.ToLower(strings.TrimSpace(value))
	switch level {
	case "silent", "error", "warning", "info", "debug":
		return level, nil
	default:
		return "", common.NewErrorf("unsupported mihomo log level: %s", value)
	}
}

func GetSingboxCoreLogLevel() (string, error) {
	return GetSingboxCoreLogLevelWithDB(database.GetDB())
}

func GetSingboxCoreLogLevelWithDB(db *gorm.DB) (string, error) {
	if db == nil {
		return "", common.NewError("database is not ready")
	}
	value, err := (&SettingService{}).getStringTx(db, singboxCoreLogLevelSettingKey)
	if err != nil {
		return "", err
	}
	level, err := normalizeSingboxCoreLogLevel(value)
	if err != nil {
		logger.Warningf("invalid sing-box core log level %q; fallback to %q", value, defaultSingboxCoreLogLevel)
		return defaultSingboxCoreLogLevel, nil
	}
	return level, nil
}

func GetMihomoCoreLogLevel() (string, error) {
	return GetMihomoCoreLogLevelWithDB(database.GetDB())
}

func GetMihomoCoreLogLevelWithDB(db *gorm.DB) (string, error) {
	if db == nil {
		return "", common.NewError("database is not ready")
	}
	value, err := (&SettingService{}).getStringTx(db, mihomoCoreLogLevelSettingKey)
	if err != nil {
		return "", err
	}
	level, err := normalizeMihomoCoreLogLevel(value)
	if err != nil {
		logger.Warningf("invalid mihomo core log level %q; fallback to %q", value, defaultMihomoCoreLogLevel)
		return defaultMihomoCoreLogLevel, nil
	}
	return level, nil
}

func buildSingboxCoreLogConfig(level string) (json.RawMessage, error) {
	normalized, err := normalizeSingboxCoreLogLevel(level)
	if err != nil {
		return nil, err
	}
	data, err := json.Marshal(map[string]string{"level": normalized})
	if err != nil {
		return nil, err
	}
	return json.RawMessage(data), nil
}

func buildCurrentSingboxCoreLogConfig(db *gorm.DB) (json.RawMessage, error) {
	level, err := GetSingboxCoreLogLevelWithDB(db)
	if err != nil {
		return nil, err
	}
	return buildSingboxCoreLogConfig(level)
}

func applyMihomoCoreLogLevel(document map[string]interface{}, level string) error {
	if document == nil {
		return fmt.Errorf("mihomo runtime document is nil")
	}
	normalized, err := normalizeMihomoCoreLogLevel(level)
	if err != nil {
		return err
	}
	document["log-level"] = normalized
	return nil
}

func applyCurrentMihomoCoreLogLevel(db *gorm.DB, document map[string]interface{}) error {
	level, err := GetMihomoCoreLogLevelWithDB(db)
	if err != nil {
		return err
	}
	return applyMihomoCoreLogLevel(document, level)
}

func SaveSingboxCoreLogLevel(level string, actor string) (*CoreLogLevelSaveResult, error) {
	normalized, err := normalizeSingboxCoreLogLevel(level)
	if err != nil {
		return nil, err
	}
	result, err := saveCoreLogLevel(singboxCoreLogLevelSettingKey, normalized, actor)
	if err != nil || !result.Changed {
		return result, err
	}

	markLastUpdate(time.Now().Unix())
	if err := WithCertificateCoreConfigGate(func() error {
		return GetProManagerService(&ConfigService{}).RegenerateCoreConfig()
	}); err != nil {
		runtimeErr := fmt.Errorf("regenerate sing-box config after log level save failed: %w", err)
		logger.Warning(runtimeErr)
		return result, &CommittedSaveError{Err: runtimeErr, RetrySingboxRuntime: true}
	}
	return result, nil
}

func SaveMihomoCoreLogLevel(level string, actor string) (*CoreLogLevelSaveResult, error) {
	normalized, err := normalizeMihomoCoreLogLevel(level)
	if err != nil {
		return nil, err
	}
	result, err := saveCoreLogLevel(mihomoCoreLogLevelSettingKey, normalized, actor)
	if err != nil || !result.Changed {
		return result, err
	}

	markMihomoLastUpdate(time.Now().Unix())
	if err := WithCertificateCoreConfigGate(func() error {
		return NewMihomoManagerService().RegenerateServerConfig()
	}); err != nil {
		runtimeErr := fmt.Errorf("regenerate mihomo server config after log level save failed: %w", err)
		logger.Warning(runtimeErr)
		return result, &CommittedSaveError{Err: runtimeErr}
	}
	return result, nil
}

func saveCoreLogLevel(key, level, actor string) (*CoreLogLevelSaveResult, error) {
	db := database.GetDB()
	if db == nil {
		return nil, common.NewError("database is not ready")
	}

	result := &CoreLogLevelSaveResult{Level: level}
	err := db.Transaction(func(tx *gorm.DB) error {
		current, err := (&SettingService{}).getStringTx(tx, key)
		if err != nil {
			return err
		}
		if strings.EqualFold(strings.TrimSpace(current), level) {
			return nil
		}
		if err := upsertSetting(tx, key, level); err != nil {
			return err
		}
		if err := recordChange(tx, model.Changes{
			DateTime: time.Now().Unix(),
			Actor:    actor,
			Key:      key,
			Action:   "set",
			Obj:      json.RawMessage(`{"level":"` + level + `"}`),
		}); err != nil {
			return err
		}
		result.Changed = true
		return nil
	})
	if err != nil {
		return nil, err
	}
	return result, nil
}
