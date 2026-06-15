package service

import (
	"encoding/json"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/util/common"
	"gorm.io/gorm"
)

type MihomoTlsService struct {
	MihomoInboundService
}

func (s *MihomoTlsService) GetAll() ([]model.MihomoTls, error) {
	db := database.GetDB()
	tlsConfig := []model.MihomoTls{}
	err := db.Model(model.MihomoTls{}).Scan(&tlsConfig).Error
	if err != nil {
		return nil, err
	}
	for i := range tlsConfig {
		tlsConfig[i].Sanitize()
	}
	return tlsConfig, nil
}

func (s *MihomoTlsService) Save(tx *gorm.DB, action string, data json.RawMessage, hostname string) error {
	switch action {
	case "new", "edit":
		var tls model.MihomoTls
		if err := json.Unmarshal(data, &tls); err != nil {
			return err
		}
		tls.Sanitize()
		if err := tx.Save(&tls).Error; err != nil {
			return err
		}
		if action == "edit" {
			var inbounds []model.MihomoInbound
			err := tx.Model(model.MihomoInbound{}).Preload("Tls").Where("tls_id = ?", tls.Id).Find(&inbounds).Error
			if err != nil {
				return err
			}
			if len(inbounds) > 0 {
				if err := s.MihomoClientService.UpdateLinksByInboundChange(tx, &inbounds, hostname, ""); err != nil {
					return err
				}
				var inboundIDs []uint
				for _, inbound := range inbounds {
					inboundIDs = append(inboundIDs, inbound.Id)
				}
				if err := s.MihomoInboundService.UpdateOutJsons(tx, inboundIDs, hostname); err != nil {
					return common.NewError("unable to update out_json of mihomo inbounds: ", err.Error())
				}
			}
		}
	case "del":
		var id uint
		if err := json.Unmarshal(data, &id); err != nil {
			return err
		}
		var inboundCount int64
		if err := tx.Model(model.MihomoInbound{}).Where("tls_id = ?", id).Count(&inboundCount).Error; err != nil {
			return err
		}
		if inboundCount > 0 {
			return common.NewError("tls in use")
		}
		if err := tx.Where("id = ?", id).Delete(model.MihomoTls{}).Error; err != nil {
			return err
		}
	default:
		return common.NewErrorf("unknown action: %s", action)
	}

	return nil
}
