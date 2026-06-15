package service

import (
	"encoding/json"

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

func (s *MihomoConfigService) SaveConfig(tx *gorm.DB, config json.RawMessage) error {
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
