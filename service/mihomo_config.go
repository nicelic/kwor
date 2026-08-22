package service

import (
	"encoding/json"
	"fmt"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"gorm.io/gorm"
)

const mihomoConfigSettingKey = "mihomo_config"

type MihomoConfigService struct {
	SettingService
}

func (s *MihomoConfigService) GetConfig() (string, error) {
	value, err := s.getString(mihomoConfigSettingKey)
	if err != nil {
		return "", err
	}

	sanitized, err := sanitizeMihomoConfigJSON(json.RawMessage(value))
	if err != nil {
		return "", err
	}
	return string(sanitized), nil
}

// GetConfigWithDB reads the current Mihomo configuration through the supplied
// connection. It is used by transaction-local config preflight so a pending
// save is validated before it becomes visible to other requests.
func (s *MihomoConfigService) GetConfigWithDB(db *gorm.DB) (string, error) {
	if db == nil {
		return "", fmt.Errorf("mihomo config requires a database connection")
	}

	var setting model.Setting
	err := db.Where("key = ?", mihomoConfigSettingKey).First(&setting).Error
	if database.IsNotFound(err) {
		return "{}", nil
	}
	if err != nil {
		return "", err
	}

	sanitized, err := sanitizeMihomoConfigJSON(json.RawMessage(setting.Value))
	if err != nil {
		return "", err
	}
	return string(sanitized), nil
}

func (s *MihomoConfigService) SaveConfig(tx *gorm.DB, config json.RawMessage) error {
	if len(config) > maxMihomoConfigBytes {
		return fmt.Errorf("mihomo configuration exceeds the %d byte safety limit", maxMihomoConfigBytes)
	}
	if err := validateMihomoConfigDNS(config); err != nil {
		return err
	}
	if err := validateMihomoConfigRouteBounds(config, tx); err != nil {
		return err
	}
	if err := validateMihomoConfigInboundReferences(tx, config); err != nil {
		return err
	}
	sanitized, err := sanitizeMihomoConfigJSON(config)
	if err != nil {
		return err
	}

	configs, err := json.MarshalIndent(sanitized, "", "  ")
	if err != nil {
		return err
	}

	var setting model.Setting
	err = tx.Where("key = ?", mihomoConfigSettingKey).First(&setting).Error
	if database.IsNotFound(err) {
		return tx.Create(&model.Setting{
			Key:   mihomoConfigSettingKey,
			Value: string(configs),
		}).Error
	}
	if err != nil {
		return err
	}

	return tx.Model(&setting).Update("value", string(configs)).Error
}

func validateMihomoConfigDNS(config json.RawMessage) error {
	if len(config) == 0 {
		return nil
	}
	var document map[string]interface{}
	if err := json.Unmarshal(config, &document); err != nil {
		return err
	}
	if document == nil || document["dns"] == nil {
		return nil
	}
	return validateMihomoDNSConfig(document["dns"])
}
