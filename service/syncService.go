package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/logger"
	"github.com/alireza0/s-ui/util"
	"gorm.io/gorm"
)

const subOutboundSourceClient = "client"
const managedClientSubTagPrefix = "s_"
const legacyManagedClientSubTagPrefix = "sm_"

// SyncService syncs client outbounds to SubManager.
type SyncService struct {
	ClientService
	InboundService
}

// SyncResult describes the sync outcome.
type SyncResult struct {
	ClientName string `json:"clientName"`
	Action     string `json:"action"` // "synced" | "updated"
	Count      int    `json:"count"`
}

// SyncClientToSubManager syncs a client's inbound-based outbounds into SubManager.
func (s *SyncService) SyncClientToSubManager(clientName string, hostname string) (*SyncResult, error) {
	db := database.GetDB()
	tx := db.Begin()
	if tx.Error != nil {
		return nil, tx.Error
	}
	BeginManagedRuntimeHookScope(tx)

	client := &model.Client{}
	err := tx.Model(model.Client{}).Where("name = ?", clientName).First(client).Error
	if err != nil {
		DiscardManagedRuntimeHookScope(tx)
		tx.Rollback()
		if database.IsNotFound(err) {
			return nil, fmt.Errorf("client not found: %s", clientName)
		}
		return nil, err
	}
	if err := clearBlockedSubSyncInboundsForClient(tx, subOutboundSourceClient, client.Id); err != nil {
		DiscardManagedRuntimeHookScope(tx)
		tx.Rollback()
		return nil, err
	}

	result, err := s.syncClientSubOutbounds(tx, nil, client, hostname, true, true)
	if err != nil {
		DiscardManagedRuntimeHookScope(tx)
		tx.Rollback()
		return nil, err
	}

	if err := tx.Commit().Error; err != nil {
		DiscardManagedRuntimeHookScope(tx)
		tx.Rollback()
		return nil, err
	}

	LastUpdate = time.Now().Unix()
	if err := RunManagedRuntimeHookScope(tx); err != nil {
		return nil, err
	}

	return result, nil
}

// SyncClientOnSave reconciles previously synced suboutbounds after client edit/save.
// It only runs when this client has existing managed suboutbounds (or recognizable legacy tags).
func (s *SyncService) SyncClientOnSave(tx *gorm.DB, oldClient *model.Client, newClient *model.Client, hostname string) error {
	if tx == nil || newClient == nil {
		return nil
	}
	_, err := s.syncClientSubOutbounds(tx, oldClient, newClient, hostname, false, false)
	return err
}

// SyncClientOnAutoPush reconciles one client after related infrastructure settings change.
// It skips "has managed records" detection because callers already filter by auto-sync registry.
func (s *SyncService) SyncClientOnAutoPush(tx *gorm.DB, client *model.Client, hostname string) error {
	if tx == nil || client == nil {
		return nil
	}
	_, err := s.syncClientSubOutbounds(tx, nil, client, hostname, false, true)
	return err
}

// CleanupClientSubOutboundsOnDelete removes suboutbounds managed by a client when the client is deleted.
func (s *SyncService) CleanupClientSubOutboundsOnDelete(tx *gorm.DB, oldClient *model.Client) error {
	if tx == nil || oldClient == nil {
		return nil
	}
	// Reuse reconciliation flow with empty target inbounds to trigger full cleanup.
	cleanupClient := *oldClient
	cleanupClient.Inbounds = json.RawMessage("[]")
	if _, err := s.syncClientSubOutbounds(tx, oldClient, &cleanupClient, "", false, false); err != nil {
		return err
	}
	return clearBlockedSubSyncInboundsForClient(tx, subOutboundSourceClient, oldClient.Id)
}

// CleanupSubOutboundsByInboundID removes managed suboutbounds linked to a deleted inbound.
func (s *SyncService) CleanupSubOutboundsByInboundID(tx *gorm.DB, sourceType string, inboundID uint) error {
	if tx == nil || inboundID == 0 || !supportsSubSyncBlockSourceType(sourceType) {
		return nil
	}

	var records []*model.SubOutbound
	if err := tx.Model(model.SubOutbound{}).
		Where("source_type = ? AND source_inbound_id = ?", sourceType, inboundID).
		Find(&records).Error; err != nil {
		return err
	}

	var subOutboundService SubOutboundService
	seenTags := make(map[string]struct{}, len(records))
	for _, record := range records {
		if record == nil {
			continue
		}
		tag := strings.TrimSpace(record.Tag)
		if tag == "" {
			continue
		}
		if _, exists := seenTags[tag]; exists {
			continue
		}
		seenTags[tag] = struct{}{}
		if err := s.deleteSubOutboundRecord(tx, &subOutboundService, tag); err != nil {
			return err
		}
	}

	return clearBlockedSubSyncInboundsByInbound(tx, sourceType, inboundID)
}

func (s *SyncService) syncClientSubOutbounds(
	db *gorm.DB,
	oldClient *model.Client,
	client *model.Client,
	hostname string,
	force bool,
	assumeManaged bool,
) (*SyncResult, error) {
	if client == nil {
		return nil, fmt.Errorf("client is nil")
	}
	if strings.TrimSpace(client.Name) == "" {
		return nil, fmt.Errorf("client name is empty")
	}

	newInboundIDs, err := parseClientInboundIDs(client.Inbounds)
	if err != nil {
		return nil, fmt.Errorf("failed to parse client inbounds: %v", err)
	}

	var oldInboundIDs []uint
	if oldClient != nil {
		oldInboundIDs, err = parseClientInboundIDs(oldClient.Inbounds)
		if err != nil {
			return nil, fmt.Errorf("failed to parse old client inbounds: %v", err)
		}
	}

	allInboundIDs := mergeInboundIDs(newInboundIDs, oldInboundIDs)
	inboundMap, err := loadInboundsByIDs(db, allInboundIDs)
	if err != nil {
		return nil, fmt.Errorf("failed to load inbounds: %v", err)
	}
	blockedInboundIDs, err := loadBlockedSubSyncInboundIDs(db, subOutboundSourceClient, client.Id)
	if err != nil {
		return nil, fmt.Errorf("failed to load blocked inbounds: %v", err)
	}

	if !force && !assumeManaged {
		hasManaged, checkErr := s.hasManagedSubOutbounds(db, client, oldClient, inboundMap, oldInboundIDs, newInboundIDs)
		if checkErr != nil {
			return nil, checkErr
		}
		if !hasManaged {
			return nil, nil
		}
	}

	clientConfig := map[string]interface{}{}
	if len(client.Config) > 0 {
		if err := json.Unmarshal(client.Config, &clientConfig); err != nil {
			return nil, fmt.Errorf("failed to parse client config: %v", err)
		}
	}

	serverHost := util.NormalizeSubscriptionServerHost(client.ServerIp)
	if serverHost == "" {
		serverHost = util.NormalizeSubscriptionServerHost(hostname)
	}

	if force {
		return s.syncClientSubOutboundsFullRebuild(db, client, inboundMap, newInboundIDs, blockedInboundIDs, clientConfig, serverHost, true)
	}

	desiredTagSet := make(map[string]struct{}, len(newInboundIDs))
	usedTargetIDs := make(map[uint]struct{}, len(newInboundIDs))
	syncCount := 0
	resultAction := "synced"
	var subOutboundService SubOutboundService

	for _, inboundID := range newInboundIDs {
		if isBlockedSubSyncInbound(blockedInboundIDs, inboundID) {
			continue
		}
		inbound := inboundMap[inboundID]
		if inbound == nil {
			logger.Warningf("[Sync] skip inbound id=%d: not found", inboundID)
			continue
		}
		baseTag := strings.TrimSpace(inbound.Tag)
		if baseTag == "" {
			logger.Warningf("[Sync] skip inbound id=%d: empty inbound tag", inbound.Id)
			continue
		}

		subTag := buildManagedClientSubTag(baseTag, client.Name)
		if subTag == "" {
			logger.Warningf("[Sync] skip inbound id=%d: failed to build sub tag", inbound.Id)
			continue
		}
		desiredTagSet[subTag] = struct{}{}

		// Keep raw subscription payload unchanged during incremental sync.
		outbound, clashSource, buildErr := s.buildSyncedOutbound(db, inbound, clientConfig, client.Name, serverHost, true)
		if buildErr != nil {
			logger.Warningf("[Sync] skip inbound %s: %v", baseTag, buildErr)
			continue
		}
		rewriteOutboundTagReferences(outbound, baseTag, subTag)
		rewriteOutboundTagReferences(clashSource, baseTag, subTag)

		target, findErr := s.findSyncTargetSubOutbound(db, client, oldClient, inbound, subTag, usedTargetIDs)
		if findErr != nil {
			return nil, findErr
		}
		if target != nil {
			usedTargetIDs[target.Id] = struct{}{}
			resultAction = "updated"
		}

		oldTag := ""
		if target != nil {
			oldTag = target.Tag
			if oldTag != "" && oldTag != subTag {
				if remapErr := s.replaceSubGroupOutboundTag(db, oldTag, subTag); remapErr != nil {
					logger.Warningf("[Sync] failed to remap subgroup tag from %s to %s: %v", oldTag, subTag, remapErr)
				}
			}
		}

		if saveErr := s.saveSyncedSubOutbound(db, &subOutboundService, target, outbound, clashSource, subTag, client.Id, inbound.Id); saveErr != nil {
			logger.Warningf("[Sync] failed to save suboutbound %s: %v", subTag, saveErr)
			continue
		}

		syncCount++
		logger.Infof("[Sync] synced suboutbound: %s", subTag)
	}

	removedCount, cleanupErr := s.cleanupStaleClientSubOutbounds(
		db,
		&subOutboundService,
		client,
		oldClient,
		inboundMap,
		newInboundIDs,
		oldInboundIDs,
		desiredTagSet,
	)
	if cleanupErr != nil {
		return nil, cleanupErr
	}
	if removedCount > 0 {
		resultAction = "updated"
	}

	if syncCount == 0 {
		if len(desiredTagSet) == 0 {
			if removedCount > 0 {
				return &SyncResult{
					ClientName: client.Name,
					Action:     resultAction,
					Count:      0,
				}, nil
			}
			return nil, nil
		}
		if removedCount == 0 {
			return nil, fmt.Errorf("no valid outbound configs found for client %s", client.Name)
		}
	}

	return &SyncResult{
		ClientName: client.Name,
		Action:     resultAction,
		Count:      syncCount,
	}, nil
}

func (s *SyncService) syncClientSubOutboundsFullRebuild(
	db *gorm.DB,
	client *model.Client,
	inboundMap map[uint]*model.Inbound,
	newInboundIDs []uint,
	blockedInboundIDs map[uint]struct{},
	clientConfig map[string]interface{},
	serverHost string,
	preserveRaw bool,
) (*SyncResult, error) {
	desiredTagSet := make(map[string]struct{}, len(newInboundIDs))
	for _, inboundID := range newInboundIDs {
		if isBlockedSubSyncInbound(blockedInboundIDs, inboundID) {
			continue
		}
		inbound := inboundMap[inboundID]
		if inbound == nil {
			continue
		}
		tag := buildManagedClientSubTag(inbound.Tag, client.Name)
		if tag == "" {
			continue
		}
		desiredTagSet[tag] = struct{}{}
	}

	var subOutboundService SubOutboundService
	removedCount := 0

	var managed []*model.SubOutbound
	if err := db.Model(model.SubOutbound{}).
		Where("source_type = ? AND source_client_id = ?", subOutboundSourceClient, client.Id).
		Find(&managed).Error; err != nil {
		return nil, err
	}
	for _, record := range managed {
		if record == nil || strings.TrimSpace(record.Tag) == "" {
			continue
		}
		if _, keep := desiredTagSet[record.Tag]; !keep {
			if err := s.removeSubGroupOutboundTag(db, record.Tag); err != nil {
				return nil, err
			}
		}
		if err := db.Where("id = ?", record.Id).Delete(&model.SubOutbound{}).Error; err != nil {
			return nil, err
		}
		if err := subOutboundService.removeManagedArtifacts(db, record.Tag); err != nil {
			return nil, err
		}
		removedCount++
	}

	for tag := range desiredTagSet {
		record, err := s.findSubOutboundByTag(db, tag)
		if err != nil {
			return nil, err
		}
		if record == nil {
			continue
		}
		if record.SourceType == subOutboundSourceClient && record.SourceClientId != client.Id {
			return nil, fmt.Errorf("tag %s is already managed by another client", tag)
		}
		if record.SourceType != "" && record.SourceType != subOutboundSourceClient {
			return nil, fmt.Errorf("tag %s is managed by source type %s", tag, record.SourceType)
		}
		if err := db.Where("id = ?", record.Id).Delete(&model.SubOutbound{}).Error; err != nil {
			return nil, err
		}
		if err := subOutboundService.removeManagedArtifacts(db, tag); err != nil {
			return nil, err
		}
		removedCount++
	}

	syncCount := 0
	for _, inboundID := range newInboundIDs {
		if isBlockedSubSyncInbound(blockedInboundIDs, inboundID) {
			continue
		}
		inbound := inboundMap[inboundID]
		if inbound == nil {
			logger.Warningf("[Sync] skip inbound id=%d: not found", inboundID)
			continue
		}
		baseTag := strings.TrimSpace(inbound.Tag)
		if baseTag == "" {
			logger.Warningf("[Sync] skip inbound id=%d: empty inbound tag", inbound.Id)
			continue
		}

		subTag := buildManagedClientSubTag(baseTag, client.Name)
		if subTag == "" {
			logger.Warningf("[Sync] skip inbound id=%d: failed to build sub tag", inbound.Id)
			continue
		}

		outbound, clashSource, buildErr := s.buildSyncedOutbound(db, inbound, clientConfig, client.Name, serverHost, preserveRaw)
		if buildErr != nil {
			logger.Warningf("[Sync] skip inbound %s: %v", baseTag, buildErr)
			continue
		}
		rewriteOutboundTagReferences(outbound, baseTag, subTag)
		rewriteOutboundTagReferences(clashSource, baseTag, subTag)

		if saveErr := s.saveSyncedSubOutbound(db, &subOutboundService, nil, outbound, clashSource, subTag, client.Id, inbound.Id); saveErr != nil {
			logger.Warningf("[Sync] failed to save suboutbound %s: %v", subTag, saveErr)
			continue
		}

		syncCount++
		logger.Infof("[Sync] full rebuild synced suboutbound: %s", subTag)
	}

	legacyRemovedCount, cleanupErr := s.cleanupStaleClientSubOutbounds(
		db,
		&subOutboundService,
		client,
		nil,
		inboundMap,
		newInboundIDs,
		nil,
		desiredTagSet,
	)
	if cleanupErr != nil {
		return nil, cleanupErr
	}
	removedCount += legacyRemovedCount

	if syncCount == 0 {
		if len(newInboundIDs) == 0 {
			if removedCount > 0 {
				return &SyncResult{
					ClientName: client.Name,
					Action:     "updated",
					Count:      0,
				}, nil
			}
			return nil, fmt.Errorf("client %s has no inbounds", client.Name)
		}
		if removedCount == 0 {
			return nil, fmt.Errorf("no valid outbound configs found for client %s", client.Name)
		}
	}

	action := "synced"
	if removedCount > 0 {
		action = "updated"
	}
	return &SyncResult{
		ClientName: client.Name,
		Action:     action,
		Count:      syncCount,
	}, nil
}

func (s *SyncService) buildSyncedOutbound(
	db *gorm.DB,
	inbound *model.Inbound,
	clientConfig map[string]interface{},
	clientName string,
	serverHost string,
	preserveRaw bool,
) (map[string]interface{}, map[string]interface{}, error) {
	if inbound == nil {
		return nil, nil, fmt.Errorf("inbound is nil")
	}
	if len(inbound.OutJson) < 5 {
		if len(inbound.OutJson) == 0 {
			inbound.OutJson = []byte("{}")
		}
		if err := util.FillOutJson(inbound, serverHost); err != nil {
			return nil, nil, fmt.Errorf("failed to build out_json: %v", err)
		}
		if err := db.Model(model.Inbound{}).Where("id = ?", inbound.Id).Update("out_json", inbound.OutJson).Error; err != nil {
			logger.Warningf("[Sync] failed to persist out_json for inbound %s: %v", inbound.Tag, err)
		}
	}

	var outbound map[string]interface{}
	if err := json.Unmarshal(inbound.OutJson, &outbound); err != nil {
		if len(inbound.OutJson) == 0 {
			inbound.OutJson = []byte("{}")
		}
		if regenErr := util.FillOutJson(inbound, serverHost); regenErr != nil {
			return nil, nil, fmt.Errorf("failed to parse out_json: %v", err)
		}
		if persistErr := db.Model(model.Inbound{}).Where("id = ?", inbound.Id).Update("out_json", inbound.OutJson).Error; persistErr != nil {
			logger.Warningf("[Sync] failed to persist regenerated out_json for inbound %s: %v", inbound.Tag, persistErr)
		}
		if err := json.Unmarshal(inbound.OutJson, &outbound); err != nil {
			return nil, nil, fmt.Errorf("failed to parse regenerated out_json: %v", err)
		}
	}

	applyServerHostOverride(outbound, serverHost)

	protocol, _ := outbound["type"].(string)
	if protocol == "trusttunnel" {
		util.SanitizeTrustTunnelOutbound(outbound)
	}
	if protocol == "shadowsocks" {
		s.applyShadowsocksConfig(outbound, clientConfig, inbound)
	} else {
		config, _ := clientConfig[protocol].(map[string]interface{})
		mergeClientProtocolConfig(outbound, config, inbound, clientName)
	}
	if protocol == "hysteria" {
		util.ApplyHysteriaInboundQUICToOutbound(outbound, inbound.Options)
	}
	hydrateOutboundTLSFromInboundTLS(outbound, inbound)
	if inbound.Tls != nil {
		refreshManagedSubscriptionOutboundTLS(outbound, inbound.Tls)
	}

	clashSource, err := cloneMihomoOutboundMap(outbound)
	if err != nil {
		return nil, nil, err
	}

	stripSyncedSubscriptionJSONFields(outbound)

	return outbound, clashSource, nil
}

func stripSyncedSubscriptionJSONFields(outbound map[string]interface{}) {
	if outbound == nil {
		return
	}
	delete(outbound, "mihomo_common")
	delete(outbound, "mihomo_hy2")
	delete(outbound, "mihomo_fast_open")
	delete(outbound, "fast_open")
	tlsMap, ok := outbound["tls"].(map[string]interface{})
	if !ok || tlsMap == nil {
		return
	}
	delete(tlsMap, "fingerprint")
	delete(tlsMap, "mihomo_use_fingerprint")
	delete(tlsMap, "include_server_certificate")
	delete(tlsMap, "include_server_fingerprint")
}

func parseClientInboundIDs(raw json.RawMessage) ([]uint, error) {
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" || trimmed == "null" {
		return []uint{}, nil
	}

	var inboundIDs []uint
	if err := json.Unmarshal(raw, &inboundIDs); err == nil {
		return deduplicateInboundIDs(inboundIDs), nil
	}

	var mixed []interface{}
	if err := json.Unmarshal(raw, &mixed); err != nil {
		return nil, err
	}

	parsed := make([]uint, 0, len(mixed))
	for _, item := range mixed {
		switch value := item.(type) {
		case float64:
			if value > 0 && math.Trunc(value) == value {
				parsed = append(parsed, uint(value))
			}
		case int:
			if value > 0 {
				parsed = append(parsed, uint(value))
			}
		case string:
			numeric, err := strconv.ParseUint(strings.TrimSpace(value), 10, 64)
			if err == nil && numeric > 0 {
				parsed = append(parsed, uint(numeric))
			}
		case json.Number:
			numeric, err := value.Int64()
			if err == nil && numeric > 0 {
				parsed = append(parsed, uint(numeric))
			}
		}
	}

	return deduplicateInboundIDs(parsed), nil
}

func mergeInboundIDs(parts ...[]uint) []uint {
	seen := make(map[uint]struct{})
	result := make([]uint, 0)
	for _, part := range parts {
		for _, id := range part {
			if _, exists := seen[id]; exists {
				continue
			}
			seen[id] = struct{}{}
			result = append(result, id)
		}
	}
	return result
}

func loadInboundsByIDs(db *gorm.DB, inboundIDs []uint) (map[uint]*model.Inbound, error) {
	result := make(map[uint]*model.Inbound, len(inboundIDs))
	if len(inboundIDs) == 0 {
		return result, nil
	}
	var inbounds []*model.Inbound
	if err := db.Model(model.Inbound{}).Preload("Tls").Where("id in ?", inboundIDs).Find(&inbounds).Error; err != nil {
		return nil, err
	}
	for _, inbound := range inbounds {
		result[inbound.Id] = inbound
	}
	return result, nil
}

func (s *SyncService) hasManagedSubOutbounds(
	db *gorm.DB,
	client *model.Client,
	oldClient *model.Client,
	inboundMap map[uint]*model.Inbound,
	oldInboundIDs []uint,
	newInboundIDs []uint,
) (bool, error) {
	var managedCount int64
	if err := db.Model(model.SubOutbound{}).
		Where("source_type = ? AND source_client_id = ?", subOutboundSourceClient, client.Id).
		Count(&managedCount).Error; err != nil {
		return false, err
	}
	if managedCount > 0 {
		return true, nil
	}

	if oldClient == nil {
		return false, nil
	}

	desiredTagSet := make(map[string]struct{}, len(newInboundIDs))
	for _, inboundID := range newInboundIDs {
		inbound := inboundMap[inboundID]
		if inbound == nil {
			continue
		}
		tag := buildManagedClientSubTag(inbound.Tag, client.Name)
		if tag == "" {
			continue
		}
		desiredTagSet[tag] = struct{}{}
	}

	candidates := s.collectLegacyCandidateTags(
		db,
		client,
		oldClient,
		inboundMap,
		mergeInboundIDs(oldInboundIDs, newInboundIDs),
		desiredTagSet,
	)
	for tag := range candidates {
		record, err := s.findSubOutboundByTag(db, tag)
		if err != nil {
			return false, err
		}
		if record == nil {
			continue
		}
		if record.SourceType == subOutboundSourceClient && record.SourceClientId != client.Id {
			continue
		}
		return true, nil
	}

	return false, nil
}

func (s *SyncService) findSyncTargetSubOutbound(
	db *gorm.DB,
	client *model.Client,
	oldClient *model.Client,
	inbound *model.Inbound,
	desiredTag string,
	usedTargetIDs map[uint]struct{},
) (*model.SubOutbound, error) {
	target, err := s.findClientManagedSubOutbound(db, client.Id, inbound.Id)
	if err != nil {
		return nil, err
	}
	if target != nil {
		if _, used := usedTargetIDs[target.Id]; !used {
			return target, nil
		}
	}

	target, err = s.findSubOutboundByTag(db, desiredTag)
	if err != nil {
		return nil, fmt.Errorf("failed to query suboutbound by tag %s: %v", desiredTag, err)
	}
	if target != nil {
		if target.SourceType == subOutboundSourceClient && target.SourceClientId != client.Id {
			return nil, fmt.Errorf("tag %s is already managed by another client", desiredTag)
		}
		if target.SourceType != "" && target.SourceType != subOutboundSourceClient {
			return nil, fmt.Errorf("tag %s is managed by source type %s", desiredTag, target.SourceType)
		}
		if _, used := usedTargetIDs[target.Id]; !used {
			return target, nil
		}
	}

	nameCandidates := []string{client.Name}
	if oldClient != nil && strings.TrimSpace(oldClient.Name) != "" && oldClient.Name != client.Name {
		nameCandidates = append(nameCandidates, oldClient.Name)
	}
	for _, legacyTag := range buildManagedLegacySubTags(nameCandidates, inbound.Tag) {
		if legacyTag == "" || legacyTag == desiredTag {
			continue
		}
		if legacyTag == strings.TrimSpace(inbound.Tag) && !s.canReuseLegacyInboundTag(db, client.Id, inbound.Id) {
			continue
		}

		record, err := s.findSubOutboundByTag(db, legacyTag)
		if err != nil {
			return nil, fmt.Errorf("failed to query legacy suboutbound %s: %v", legacyTag, err)
		}
		if record == nil {
			continue
		}
		if record.SourceType == subOutboundSourceClient && record.SourceClientId != client.Id {
			continue
		}
		if record.SourceType != "" && record.SourceType != subOutboundSourceClient {
			continue
		}
		if _, used := usedTargetIDs[record.Id]; used {
			continue
		}
		return record, nil
	}

	return nil, nil
}

func (s *SyncService) saveSyncedSubOutbound(
	db *gorm.DB,
	subOutboundService *SubOutboundService,
	target *model.SubOutbound,
	outbound map[string]interface{},
	clashSource map[string]interface{},
	subTag string,
	clientID uint,
	inboundID uint,
) error {
	if outbound == nil {
		return fmt.Errorf("outbound is nil")
	}
	outbound["tag"] = subTag

	outboundData, err := json.Marshal(outbound)
	if err != nil {
		return err
	}

	return replaceSyncedSubOutboundRecord(
		db,
		subOutboundService,
		target,
		outboundData,
		clashSource,
		subOutboundSourceClient,
		clientID,
		inboundID,
		subTag,
	)
}

func replaceSyncedSubOutboundRecord(
	db *gorm.DB,
	subOutboundService *SubOutboundService,
	target *model.SubOutbound,
	outboundData json.RawMessage,
	clashSource map[string]interface{},
	sourceType string,
	clientID uint,
	inboundID uint,
	subTag string,
) error {
	if subOutboundService == nil {
		subOutboundService = &SubOutboundService{}
	}
	subTag = strings.TrimSpace(subTag)
	if subTag == "" {
		return fmt.Errorf("suboutbound tag is empty")
	}
	if target != nil {
		if target.SourceType != "" && target.SourceType != sourceType {
			return fmt.Errorf("tag %s is managed by source type %s", strings.TrimSpace(target.Tag), target.SourceType)
		}
		if target.SourceType == sourceType && target.SourceClientId != 0 && target.SourceClientId != clientID {
			return fmt.Errorf("tag %s is already managed by another client", strings.TrimSpace(target.Tag))
		}
	}

	clashOptions, err := buildMihomoClashOptions(clashSource, subTag)
	if err != nil {
		return err
	}

	var fresh model.SubOutbound
	if err := fresh.UnmarshalJSON(outboundData); err != nil {
		return err
	}
	fresh.Id = 0
	fresh.Tag = subTag
	if strings.TrimSpace(fresh.Type) == "" {
		return fmt.Errorf("suboutbound %s type is empty", subTag)
	}
	if exactRaw := normalizeSubOutboundRawPayload(outboundData); len(exactRaw) > 0 {
		fresh.RawOutbound = exactRaw
	} else {
		fresh.RawOutbound = append(json.RawMessage(nil), outboundData...)
	}
	fresh.ClashOptions = clashOptions
	fresh.RawClashYAML = nil
	fresh.SourceType = sourceType
	fresh.SourceClientId = clientID
	fresh.SourceInboundId = inboundID

	validationProbe := fresh
	if target != nil {
		validationProbe.Id = target.Id
	}
	if err := validateSubOutboundSubJSONFileName(db, &validationProbe); err != nil {
		return err
	}

	deletedTags := make(map[string]struct{})
	if target != nil && target.Id > 0 {
		oldTag := strings.TrimSpace(target.Tag)
		if err := db.Where("id = ?", target.Id).Delete(&model.SubOutbound{}).Error; err != nil {
			return err
		}
		if oldTag != "" {
			deletedTags[oldTag] = struct{}{}
		}
	}

	if err := db.Create(&fresh).Error; err != nil {
		return err
	}

	for tag := range deletedTags {
		if err := subOutboundService.removeManagedArtifacts(db, tag); err != nil {
			return err
		}
	}
	return subOutboundService.syncManagedArtifacts(db, &fresh)
}

func (s *SyncService) cleanupStaleClientSubOutbounds(
	db *gorm.DB,
	subOutboundService *SubOutboundService,
	client *model.Client,
	oldClient *model.Client,
	inboundMap map[uint]*model.Inbound,
	newInboundIDs []uint,
	oldInboundIDs []uint,
	desiredTagSet map[string]struct{},
) (int, error) {
	removedCount := 0
	removedTags := make(map[string]struct{})

	var managed []*model.SubOutbound
	err := db.Model(model.SubOutbound{}).
		Where("source_type = ? AND source_client_id = ?", subOutboundSourceClient, client.Id).
		Find(&managed).Error
	if err != nil {
		return removedCount, err
	}

	for _, record := range managed {
		if record == nil || record.Tag == "" {
			continue
		}
		if _, keep := desiredTagSet[record.Tag]; keep {
			continue
		}
		if _, already := removedTags[record.Tag]; already {
			continue
		}
		if err := s.deleteSubOutboundRecord(db, subOutboundService, record.Tag); err != nil {
			return removedCount, err
		}
		removedTags[record.Tag] = struct{}{}
		removedCount++
	}

	candidateTags := s.collectLegacyCandidateTags(
		db,
		client,
		oldClient,
		inboundMap,
		mergeInboundIDs(oldInboundIDs, newInboundIDs),
		desiredTagSet,
	)
	for tag, inboundID := range candidateTags {
		if _, already := removedTags[tag]; already {
			continue
		}

		record, err := s.findSubOutboundByTag(db, tag)
		if err != nil {
			return removedCount, err
		}
		if record == nil {
			continue
		}
		if record.SourceType == subOutboundSourceClient && record.SourceClientId != client.Id {
			continue
		}
		if record.SourceType != "" && record.SourceType != subOutboundSourceClient {
			continue
		}

		baseTag := ""
		if inbound := inboundMap[inboundID]; inbound != nil {
			baseTag = strings.TrimSpace(inbound.Tag)
		}
		if tag == baseTag && !s.canReuseLegacyInboundTag(db, client.Id, inboundID) {
			continue
		}

		if err := s.deleteSubOutboundRecord(db, subOutboundService, tag); err != nil {
			return removedCount, err
		}
		removedTags[tag] = struct{}{}
		removedCount++
	}

	return removedCount, nil
}

func (s *SyncService) collectLegacyCandidateTags(
	db *gorm.DB,
	client *model.Client,
	oldClient *model.Client,
	inboundMap map[uint]*model.Inbound,
	inboundIDs []uint,
	desiredTagSet map[string]struct{},
) map[string]uint {
	result := make(map[string]uint)
	nameCandidates := []string{client.Name}
	if oldClient != nil && strings.TrimSpace(oldClient.Name) != "" && oldClient.Name != client.Name {
		nameCandidates = append(nameCandidates, oldClient.Name)
	}

	for _, inboundID := range inboundIDs {
		inbound := inboundMap[inboundID]
		if inbound == nil {
			continue
		}
		baseTag := strings.TrimSpace(inbound.Tag)
		if baseTag == "" {
			continue
		}

		for _, tag := range buildManagedLegacySubTags(nameCandidates, baseTag) {
			if tag == "" {
				continue
			}
			if _, keep := desiredTagSet[tag]; keep {
				continue
			}
			if tag == baseTag && !s.canReuseLegacyInboundTag(db, client.Id, inboundID) {
				continue
			}
			result[tag] = inboundID
		}
	}

	return result
}

func (s *SyncService) deleteSubOutboundRecord(db *gorm.DB, subOutboundService *SubOutboundService, tag string) error {
	if tag == "" {
		return nil
	}
	if err := s.removeSubGroupOutboundTag(db, tag); err != nil {
		logger.Warningf("[Sync] failed to remove subgroup references for %s: %v", tag, err)
	}
	if err := db.Where("tag = ?", tag).Delete(model.SubOutbound{}).Error; err != nil {
		return err
	}
	if err := subOutboundService.removeManagedArtifacts(db, tag); err != nil {
		return err
	}
	logger.Infof("[Sync] deleted stale suboutbound: %s", tag)
	return nil
}

func (s *SyncService) removeSubGroupOutboundTag(db *gorm.DB, removeTag string) error {
	var subGroupService SubGroupService
	return subGroupService.removeOutboundTagsFromGroups(db, []string{removeTag})
}

func (s *SyncService) canReuseLegacyInboundTag(db *gorm.DB, clientID uint, inboundID uint) bool {
	if inboundID == 0 {
		return false
	}

	condition := "EXISTS (SELECT 1 FROM json_each(clients.inbounds) WHERE json_each.value = ?)"
	var ownCount int64
	if err := db.Table("clients").Where("id = ? AND "+condition, clientID, inboundID).Count(&ownCount).Error; err != nil {
		logger.Warningf("[Sync] failed to verify inbound ownership for id=%d: %v", inboundID, err)
		return false
	}
	if ownCount == 0 {
		return false
	}

	var count int64
	if err := db.Table("clients").Where(condition, inboundID).Count(&count).Error; err != nil {
		logger.Warningf("[Sync] failed to count inbound owners for id=%d: %v", inboundID, err)
		return false
	}
	return count <= 1
}

func buildClientSubTag(inboundTag, clientName string) string {
	base := strings.TrimSpace(inboundTag)
	name := strings.TrimSpace(clientName)
	if base == "" {
		return ""
	}
	if name == "" {
		return base
	}
	return base + "_" + name
}

func buildClientSubTagWithPrefix(inboundTag, clientName, prefix string) string {
	tag := buildClientSubTag(inboundTag, clientName)
	if tag == "" {
		return ""
	}
	return strings.TrimSpace(prefix) + tag
}

func buildManagedClientSubTag(inboundTag, clientName string) string {
	return buildClientSubTagWithPrefix(inboundTag, clientName, managedClientSubTagPrefix)
}

func buildManagedLegacySubTags(clientNames []string, inboundTag string) []string {
	base := strings.TrimSpace(inboundTag)
	if base == "" {
		return nil
	}

	result := make([]string, 0, len(clientNames)*5+3)
	seen := make(map[string]struct{}, len(clientNames)*5+3)
	add := func(tag string) {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			return
		}
		if _, exists := seen[tag]; exists {
			return
		}
		seen[tag] = struct{}{}
		result = append(result, tag)
	}

	for _, rawName := range clientNames {
		name := strings.TrimSpace(rawName)
		if name == "" {
			continue
		}
		add(buildManagedClientSubTag(base, name))
		add(buildClientSubTagWithPrefix(base, name, legacyManagedClientSubTagPrefix))
	}
	for _, tag := range buildLegacySubTags(clientNames, inboundTag) {
		add(tag)
	}
	add(buildManagedClientSubTag(base, ""))
	add(buildClientSubTagWithPrefix(base, "", legacyManagedClientSubTagPrefix))
	return result
}

func buildLegacySubTags(clientNames []string, inboundTag string) []string {
	base := strings.TrimSpace(inboundTag)
	if base == "" {
		return nil
	}

	result := make([]string, 0, len(clientNames)*3+1)
	seen := make(map[string]struct{}, len(clientNames)*3+1)

	add := func(tag string) {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			return
		}
		if _, exists := seen[tag]; exists {
			return
		}
		seen[tag] = struct{}{}
		result = append(result, tag)
	}

	for _, rawName := range clientNames {
		name := strings.TrimSpace(rawName)
		if name == "" {
			continue
		}
		add(buildClientSubTag(base, name))
		add(fmt.Sprintf("%s-%s", name, base))
		add(name)
	}
	add(base)

	return result
}

func rewriteOutboundTagReferences(outbound map[string]interface{}, oldTag string, newTag string) {
	if outbound == nil {
		return
	}
	oldTag = strings.TrimSpace(oldTag)
	newTag = strings.TrimSpace(newTag)
	if newTag == "" {
		return
	}

	outbound["tag"] = newTag

	if detour, ok := outbound["detour"].(string); ok {
		detour = strings.TrimSpace(detour)
		switch detour {
		case oldTag:
			outbound["detour"] = newTag
		case oldTag + "-out":
			outbound["detour"] = newTag + "-out"
		}
	}
}

func applyServerHostOverride(outbound map[string]interface{}, serverHost string) {
	if outbound == nil {
		return
	}
	host := strings.TrimSpace(serverHost)
	host = util.NormalizeSubscriptionServerHost(host)
	if host == "" {
		return
	}
	if _, ok := outbound["server"]; ok {
		outbound["server"] = host
	}
}

func hydrateOutboundTLSFromInboundTLS(outbound map[string]interface{}, inbound *model.Inbound) {
	if outbound == nil || inbound == nil || inbound.TlsId == 0 || inbound.Tls == nil {
		return
	}

	var tlsClient map[string]interface{}
	if len(inbound.Tls.Client) > 0 {
		_ = json.Unmarshal(inbound.Tls.Client, &tlsClient)
	}

	var tlsServer map[string]interface{}
	if len(inbound.Tls.Server) > 0 {
		_ = json.Unmarshal(inbound.Tls.Server, &tlsServer)
	}

	if len(tlsClient) == 0 && len(tlsServer) == 0 {
		return
	}

	tlsRaw, hasTLS := outbound["tls"]
	tlsMap, _ := tlsRaw.(map[string]interface{})
	if !hasTLS || tlsMap == nil {
		tlsMap = map[string]interface{}{}
		outbound["tls"] = tlsMap
	}

	copyIfMissing := func(src map[string]interface{}, keys ...string) {
		for _, key := range keys {
			if _, exists := tlsMap[key]; exists {
				continue
			}
			if value, ok := src[key]; ok {
				tlsMap[key] = value
			}
		}
	}

	copyIfMissing(tlsServer, "enabled", "server_name", "alpn", "min_version", "max_version", "cipher_suites")
	copyIfMissing(
		tlsClient,
		"disable_sni",
		"insecure",
		"include_server_certificate",
		"certificate",
		"certificate_path",
		"certificate_public_key_sha256",
		"client_certificate",
		"client_certificate_path",
		"client_key",
		"client_key_path",
		"utls",
		"reality",
		"ech",
		"fragment",
		"fragment_fallback_delay",
		"record_fragment",
	)

	if _, hasTLSStore := tlsMap["tls_store"]; !hasTLSStore {
		if store, ok := tlsClient["tls_store"].(string); ok && store != "" {
			tlsMap["tls_store"] = store
		} else if store, ok := tlsClient["store"].(string); ok && store != "" {
			tlsMap["tls_store"] = store
		}
	}
}

func (s *SyncService) findSubOutboundByTag(db *gorm.DB, tag string) (*model.SubOutbound, error) {
	if tag == "" {
		return nil, nil
	}
	record := &model.SubOutbound{}
	err := db.Model(model.SubOutbound{}).Where("tag = ?", tag).First(record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return record, nil
}

func (s *SyncService) findClientManagedSubOutbound(db *gorm.DB, clientID uint, inboundID uint) (*model.SubOutbound, error) {
	if clientID == 0 || inboundID == 0 {
		return nil, nil
	}
	record := &model.SubOutbound{}
	err := db.Model(model.SubOutbound{}).
		Where("source_type = ? AND source_client_id = ? AND source_inbound_id = ?", subOutboundSourceClient, clientID, inboundID).
		First(record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return record, nil
}

func (s *SyncService) replaceSubGroupOutboundTag(db *gorm.DB, oldTag, newTag string) error {
	var subGroupService SubGroupService
	return subGroupService.replaceOutboundTagInGroups(db, oldTag, newTag)
}

// mergeClientProtocolConfig applies default-runtime client fields without overriding existing out_json values.
func mergeClientProtocolConfig(outbound map[string]interface{}, config map[string]interface{}, inbound *model.Inbound, clientName ...string) {
	mergeClientProtocolConfigForNamespace(outbound, config, inbound, "default", clientName...)
}

func mergeClientProtocolConfigForNamespace(
	outbound map[string]interface{},
	config map[string]interface{},
	inbound *model.Inbound,
	namespace string,
	clientName ...string,
) {
	if outbound == nil || inbound == nil {
		return
	}

	protocol, _ := outbound["type"].(string)
	if protocol == "trusttunnel" {
		util.SanitizeTrustTunnelOutbound(outbound)
	}
	for key, value := range config {
		if shouldSkipClientConfigKey(namespace, protocol, key, inbound) || isEmptyConfigValue(value) {
			continue
		}
		if existing, exists := outbound[key]; exists && !isEmptyConfigValue(existing) {
			continue
		}
		outbound[key] = value
	}

	if protocol == "sudoku" {
		if uuid := strings.TrimSpace(util.NormalizeSudokuKeyValue(config["uuid"])); uuid != "" {
			outbound["key"] = uuid
		} else if strings.TrimSpace(firstString(outbound["key"])) == "" {
			if sharedUUID := mihomoSudokuSharedUUIDFromOptions(inbound.Options); sharedUUID != "" {
				outbound["key"] = sharedUUID
			}
		}
	}
	if protocol == "trusttunnel" {
		util.ApplyTrustTunnelCredentials(outbound, config, clientName...)
	}
	if protocol == "hysteria2" {
		util.SanitizeOptionalNetworkField(outbound)
	}
}

func shouldSkipClientConfigKey(namespace string, protocol string, key string, inbound *model.Inbound) bool {
	hasTLS := inbound != nil && inbound.TlsId != 0
	switch namespace {
	case "mihomo":
		if util.ShouldSkipMihomoOutboundClientConfigKey(protocol, key, hasTLS) {
			return true
		}
	default:
		if util.ShouldSkipSingboxOutboundClientConfigKey(protocol, key, hasTLS) {
			return true
		}
	}
	if protocol == "sudoku" && key == "uuid" {
		return true
	}
	if protocol == "trusttunnel" && (key == "uuid" || key == "network") {
		return true
	}
	return false
}

func isEmptyConfigValue(value interface{}) bool {
	if value == nil {
		return true
	}

	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed) == ""
	case []interface{}:
		return len(typed) == 0
	case map[string]interface{}:
		return len(typed) == 0
	}

	return false
}

// applyShadowsocksConfig applies the inbound password for shadowsocks single-user mode.
func (s *SyncService) applyShadowsocksConfig(outbound map[string]interface{}, clientConfig map[string]interface{}, inbound *model.Inbound) {
	var inbOptions map[string]interface{}
	if err := json.Unmarshal(inbound.Options, &inbOptions); err != nil {
		return
	}

	inbPass, _ := inbOptions["password"].(string)
	outbound["password"] = inbPass
}
