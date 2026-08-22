package service

import (
	"github.com/alireza0/s-ui/database/model"
	"gorm.io/gorm"
)

func supportsSubSyncBlockSourceType(sourceType string) bool {
	return sourceType == subOutboundSourceClient || sourceType == subOutboundSourceMihomoClient
}

func blockSubSyncInbound(tx *gorm.DB, sourceType string, clientID uint, inboundID uint) error {
	if tx == nil || !supportsSubSyncBlockSourceType(sourceType) || clientID == 0 || inboundID == 0 {
		return nil
	}
	block := &model.SubSyncBlock{
		SourceType:      sourceType,
		SourceClientId:  clientID,
		SourceInboundId: inboundID,
	}
	return tx.Where(&model.SubSyncBlock{
		SourceType:      sourceType,
		SourceClientId:  clientID,
		SourceInboundId: inboundID,
	}).FirstOrCreate(block).Error
}

func blockSubSyncInboundBySubOutbound(tx *gorm.DB, record *model.SubOutbound) error {
	if record == nil {
		return nil
	}
	return blockSubSyncInbound(tx, record.SourceType, record.SourceClientId, record.SourceInboundId)
}

// disableAutoSyncBySubOutbound turns off the whole client's automatic sync
// registration when a managed subscription node is manually deleted. The
// registry update stays inside the caller's transaction so the deleted node,
// block record and auto-sync state cannot diverge.
func disableAutoSyncBySubOutbound(tx *gorm.DB, record *model.SubOutbound) error {
	if record == nil || record.SourceClientId == 0 {
		return nil
	}
	settings := &SettingService{}
	switch record.SourceType {
	case subOutboundSourceClient:
		_, err := settings.SetSubManagerAutoSyncClientTx(tx, record.SourceClientId, false)
		return err
	case subOutboundSourceMihomoClient:
		_, err := settings.SetSubManagerAutoSyncMihomoClientTx(tx, record.SourceClientId, false)
		return err
	default:
		return nil
	}
}

func loadBlockedSubSyncInboundIDs(tx *gorm.DB, sourceType string, clientID uint) (map[uint]struct{}, error) {
	result := make(map[uint]struct{})
	if tx == nil || !supportsSubSyncBlockSourceType(sourceType) || clientID == 0 {
		return result, nil
	}

	ids := make([]uint, 0)
	if err := tx.Model(model.SubSyncBlock{}).
		Where("source_type = ? AND source_client_id = ?", sourceType, clientID).
		Pluck("source_inbound_id", &ids).Error; err != nil {
		return nil, err
	}

	for _, id := range ids {
		if id == 0 {
			continue
		}
		result[id] = struct{}{}
	}
	return result, nil
}

func isBlockedSubSyncInbound(blocked map[uint]struct{}, inboundID uint) bool {
	if len(blocked) == 0 || inboundID == 0 {
		return false
	}
	_, ok := blocked[inboundID]
	return ok
}

func clearBlockedSubSyncInboundsForClient(tx *gorm.DB, sourceType string, clientID uint) error {
	if tx == nil || !supportsSubSyncBlockSourceType(sourceType) || clientID == 0 {
		return nil
	}
	return tx.Where("source_type = ? AND source_client_id = ?", sourceType, clientID).Delete(&model.SubSyncBlock{}).Error
}

func clearBlockedSubSyncInboundsByInbound(tx *gorm.DB, sourceType string, inboundID uint) error {
	if tx == nil || !supportsSubSyncBlockSourceType(sourceType) || inboundID == 0 {
		return nil
	}
	return tx.Where("source_type = ? AND source_inbound_id = ?", sourceType, inboundID).Delete(&model.SubSyncBlock{}).Error
}
