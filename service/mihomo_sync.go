package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/logger"
	"github.com/alireza0/s-ui/util"
	"gorm.io/gorm"
)

const subOutboundSourceMihomoClient = "mihomo_client"
const managedMihomoClientSubTagPrefix = "m_"
const legacyManagedMihomoClientSubTagPrefix = "mihomo_"

type MihomoSyncService struct {
	SyncService
}

func (s *MihomoSyncService) SyncClientToSubManager(clientName string, hostname string) (*SyncResult, error) {
	db := database.GetDB()
	tx := db.Begin()
	if tx.Error != nil {
		return nil, tx.Error
	}
	BeginManagedRuntimeHookScope(tx)

	client := &model.MihomoClient{}
	err := tx.Model(model.MihomoClient{}).Where("name = ?", clientName).First(client).Error
	if err != nil {
		DiscardManagedRuntimeHookScope(tx)
		tx.Rollback()
		if database.IsNotFound(err) {
			return nil, fmt.Errorf("mihomo client not found: %s", clientName)
		}
		return nil, err
	}
	if err := clearBlockedSubSyncInboundsForClient(tx, subOutboundSourceMihomoClient, client.Id); err != nil {
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

func (s *MihomoSyncService) SyncClientOnSave(tx *gorm.DB, oldClient *model.MihomoClient, newClient *model.MihomoClient, hostname string) error {
	if tx == nil || newClient == nil {
		return nil
	}
	_, err := s.syncClientSubOutbounds(tx, oldClient, newClient, hostname, false, false)
	return err
}

// SyncClientOnAutoPush reconciles one mihomo client after related infrastructure settings change.
// It skips "has managed records" detection because callers already filter by auto-sync registry.
func (s *MihomoSyncService) SyncClientOnAutoPush(tx *gorm.DB, client *model.MihomoClient, hostname string) error {
	if tx == nil || client == nil {
		return nil
	}
	_, err := s.syncClientSubOutbounds(tx, nil, client, hostname, false, true)
	return err
}

func (s *MihomoSyncService) CleanupClientSubOutboundsOnDelete(tx *gorm.DB, oldClient *model.MihomoClient) error {
	if tx == nil || oldClient == nil {
		return nil
	}
	cleanupClient := *oldClient
	cleanupClient.Inbounds = json.RawMessage("[]")
	if _, err := s.syncClientSubOutbounds(tx, oldClient, &cleanupClient, "", false, false); err != nil {
		return err
	}
	return clearBlockedSubSyncInboundsForClient(tx, subOutboundSourceMihomoClient, oldClient.Id)
}

func (s *MihomoSyncService) syncClientSubOutbounds(
	db *gorm.DB,
	oldClient *model.MihomoClient,
	client *model.MihomoClient,
	hostname string,
	force bool,
	assumeManaged bool,
) (*SyncResult, error) {
	if client == nil {
		return nil, fmt.Errorf("mihomo client is nil")
	}
	if strings.TrimSpace(client.Name) == "" {
		return nil, fmt.Errorf("mihomo client name is empty")
	}

	inboundIDs, err := parseClientInboundIDs(client.Inbounds)
	if err != nil {
		return nil, fmt.Errorf("failed to parse mihomo client inbounds: %v", err)
	}

	var oldInboundIDs []uint
	if oldClient != nil {
		oldInboundIDs, err = parseClientInboundIDs(oldClient.Inbounds)
		if err != nil {
			return nil, fmt.Errorf("failed to parse old mihomo client inbounds: %v", err)
		}
	}

	inboundMap, err := s.loadInboundsByIDs(db, mergeInboundIDs(inboundIDs, oldInboundIDs))
	if err != nil {
		return nil, fmt.Errorf("failed to load mihomo inbounds: %v", err)
	}
	blockedInboundIDs, err := loadBlockedSubSyncInboundIDs(db, subOutboundSourceMihomoClient, client.Id)
	if err != nil {
		return nil, fmt.Errorf("failed to load blocked mihomo inbounds: %v", err)
	}

	if !force && !assumeManaged {
		hasManaged, checkErr := s.hasManagedSubOutbounds(db, client, oldClient, inboundMap, oldInboundIDs, inboundIDs)
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
			return nil, fmt.Errorf("failed to parse mihomo client config: %v", err)
		}
	}

	serverHost := util.NormalizeSubscriptionServerHost(client.ServerIp)
	if serverHost == "" {
		serverHost = util.NormalizeSubscriptionServerHost(hostname)
	}

	desiredTags := make(map[uint]string, len(inboundIDs))
	desiredTagSet := make(map[string]struct{}, len(inboundIDs))
	for _, inboundID := range inboundIDs {
		if isBlockedSubSyncInbound(blockedInboundIDs, inboundID) {
			continue
		}
		inbound := inboundMap[inboundID]
		if inbound == nil {
			continue
		}
		tag := buildMihomoClientSubTag(inbound.Tag, client.Name)
		if tag == "" {
			continue
		}
		desiredTags[inboundID] = tag
		desiredTagSet[tag] = struct{}{}
	}

	var managed []*model.SubOutbound
	err = db.Model(model.SubOutbound{}).
		Where("source_type = ? AND source_client_id = ?", subOutboundSourceMihomoClient, client.Id).
		Find(&managed).Error
	if err != nil {
		return nil, err
	}

	managedByInboundID := make(map[uint]*model.SubOutbound, len(managed))
	var subOutboundService SubOutboundService
	removedCount := 0
	removedTags := make(map[string]struct{})
	for _, record := range managed {
		if record == nil || strings.TrimSpace(record.Tag) == "" {
			continue
		}
		if record.SourceInboundId > 0 {
			managedByInboundID[record.SourceInboundId] = record
		}
		if _, keep := desiredTags[record.SourceInboundId]; keep {
			continue
		}
		if err := s.deleteSubOutboundRecord(db, &subOutboundService, record.Tag); err != nil {
			return nil, err
		}
		removedTags[record.Tag] = struct{}{}
		removedCount++
	}

	candidateTags := s.collectLegacyCandidateTags(
		db,
		client,
		oldClient,
		inboundMap,
		mergeInboundIDs(oldInboundIDs, inboundIDs),
		desiredTagSet,
	)
	for tag, inboundID := range candidateTags {
		if _, already := removedTags[tag]; already {
			continue
		}

		record, err := s.findSubOutboundByTag(db, tag)
		if err != nil {
			return nil, err
		}
		if record == nil {
			continue
		}
		if record.SourceType == subOutboundSourceMihomoClient && record.SourceClientId != client.Id {
			continue
		}
		if record.SourceType != "" && record.SourceType != subOutboundSourceMihomoClient {
			continue
		}

		baseInboundTag := ""
		if inbound := inboundMap[inboundID]; inbound != nil {
			baseInboundTag = inbound.Tag
		}
		if isMihomoLegacyBaseTag(tag, baseInboundTag) && !s.canReuseLegacyInboundTag(db, client.Id, inboundID) {
			continue
		}

		if err := s.deleteSubOutboundRecord(db, &subOutboundService, tag); err != nil {
			return nil, err
		}
		removedTags[tag] = struct{}{}
		removedCount++
	}

	syncCount := 0
	action := "synced"
	usedTargetIDs := make(map[uint]struct{}, len(inboundIDs))

	for _, inboundID := range inboundIDs {
		if isBlockedSubSyncInbound(blockedInboundIDs, inboundID) {
			continue
		}
		inbound := inboundMap[inboundID]
		if inbound == nil {
			logger.Warningf("[MihomoSync] skip inbound id=%d: not found", inboundID)
			continue
		}

		desiredTag := desiredTags[inboundID]
		if desiredTag == "" {
			continue
		}

		// Keep raw subscription payload unchanged during incremental sync.
		outbound, clashSource, err := s.buildSyncedOutbound(db, inbound, clientConfig, client.Name, serverHost, true)
		if err != nil {
			logger.Warningf("[MihomoSync] skip inbound %s: %v", inbound.Tag, err)
			continue
		}
		rewriteOutboundTagReferences(outbound, inbound.Tag, desiredTag)
		rewriteOutboundTagReferences(clashSource, inbound.Tag, desiredTag)

		target, err := s.findSyncTargetSubOutbound(db, client, oldClient, inbound, desiredTag, usedTargetIDs)
		if err != nil {
			return nil, err
		}
		if target == nil {
			target = managedByInboundID[inboundID]
		}
		if target != nil {
			usedTargetIDs[target.Id] = struct{}{}
		}

		oldTag := ""
		if target != nil {
			oldTag = target.Tag
			if oldTag != desiredTag {
				if err := s.replaceSubGroupOutboundTag(db, oldTag, desiredTag); err != nil {
					logger.Warningf("[MihomoSync] failed to remap subgroup tag from %s to %s: %v", oldTag, desiredTag, err)
				}
				action = "updated"
			}
		}

		if err := s.saveSyncedSubOutbound(db, &subOutboundService, target, outbound, clashSource, desiredTag, client.Id, inboundID); err != nil {
			return nil, err
		}

		if target != nil {
			action = "updated"
		}
		syncCount++
	}

	if syncCount == 0 {
		if len(desiredTagSet) == 0 {
			if removedCount > 0 {
				return &SyncResult{
					ClientName: client.Name,
					Action:     "updated",
					Count:      0,
				}, nil
			}
			if force && len(inboundIDs) == 0 {
				return nil, fmt.Errorf("mihomo client %s has no inbounds", client.Name)
			}
			return nil, nil
		}
		if removedCount == 0 {
			return nil, fmt.Errorf("no valid outbound configs found for mihomo client %s", client.Name)
		}
	}
	if removedCount > 0 {
		action = "updated"
	}

	return &SyncResult{
		ClientName: client.Name,
		Action:     action,
		Count:      syncCount,
	}, nil
}

func (s *MihomoSyncService) loadInboundsByIDs(db *gorm.DB, inboundIDs []uint) (map[uint]*model.MihomoInbound, error) {
	result := make(map[uint]*model.MihomoInbound, len(inboundIDs))
	if len(inboundIDs) == 0 {
		return result, nil
	}

	var inbounds []*model.MihomoInbound
	err := db.Model(model.MihomoInbound{}).Preload("Tls").Where("id in ?", inboundIDs).Find(&inbounds).Error
	if err != nil {
		return nil, err
	}
	for _, inbound := range inbounds {
		result[inbound.Id] = inbound
	}
	return result, nil
}

func (s *MihomoSyncService) hasManagedSubOutbounds(
	db *gorm.DB,
	client *model.MihomoClient,
	oldClient *model.MihomoClient,
	inboundMap map[uint]*model.MihomoInbound,
	oldInboundIDs []uint,
	newInboundIDs []uint,
) (bool, error) {
	var managedCount int64
	if err := db.Model(model.SubOutbound{}).
		Where("source_type = ? AND source_client_id = ?", subOutboundSourceMihomoClient, client.Id).
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
		tag := buildMihomoClientSubTag(inbound.Tag, client.Name)
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
		if record.SourceType == subOutboundSourceMihomoClient && record.SourceClientId != client.Id {
			continue
		}
		return true, nil
	}

	return false, nil
}

func (s *MihomoSyncService) findSyncTargetSubOutbound(
	db *gorm.DB,
	client *model.MihomoClient,
	oldClient *model.MihomoClient,
	inbound *model.MihomoInbound,
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
		if target.SourceType == subOutboundSourceMihomoClient && target.SourceClientId != client.Id {
			return nil, fmt.Errorf("tag %s is already managed by another mihomo client", desiredTag)
		}
		if target.SourceType != "" && target.SourceType != subOutboundSourceMihomoClient {
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
	for _, legacyTag := range buildMihomoLegacySubTags(nameCandidates, inbound.Tag) {
		if legacyTag == "" || legacyTag == desiredTag {
			continue
		}
		if isMihomoLegacyBaseTag(legacyTag, inbound.Tag) && !s.canReuseLegacyInboundTag(db, client.Id, inbound.Id) {
			continue
		}

		record, err := s.findSubOutboundByTag(db, legacyTag)
		if err != nil {
			return nil, fmt.Errorf("failed to query legacy suboutbound %s: %v", legacyTag, err)
		}
		if record == nil {
			continue
		}
		if record.SourceType == subOutboundSourceMihomoClient && record.SourceClientId != client.Id {
			continue
		}
		if record.SourceType != "" && record.SourceType != subOutboundSourceMihomoClient {
			continue
		}
		if _, used := usedTargetIDs[record.Id]; used {
			continue
		}
		return record, nil
	}

	return nil, nil
}

func (s *MihomoSyncService) collectLegacyCandidateTags(
	db *gorm.DB,
	client *model.MihomoClient,
	oldClient *model.MihomoClient,
	inboundMap map[uint]*model.MihomoInbound,
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

		for _, tag := range buildMihomoLegacySubTags(nameCandidates, baseTag) {
			if tag == "" {
				continue
			}
			if _, keep := desiredTagSet[tag]; keep {
				continue
			}
			if isMihomoLegacyBaseTag(tag, baseTag) && !s.canReuseLegacyInboundTag(db, client.Id, inboundID) {
				continue
			}
			result[tag] = inboundID
		}
	}

	return result
}

func buildMihomoLegacySubTags(clientNames []string, inboundTag string) []string {
	baseTags := buildLegacySubTags(clientNames, inboundTag)
	result := make([]string, 0, len(baseTags)*2)
	seen := make(map[string]struct{}, len(baseTags)*2)
	appendWithPrefix := func(prefix string) {
		for _, tag := range baseTags {
			trimmed := strings.TrimSpace(tag)
			if trimmed == "" {
				continue
			}
			prefixed := prefix + trimmed
			if _, exists := seen[prefixed]; exists {
				continue
			}
			seen[prefixed] = struct{}{}
			result = append(result, prefixed)
		}
	}
	appendWithPrefix(managedMihomoClientSubTagPrefix)
	appendWithPrefix(legacyManagedMihomoClientSubTagPrefix)
	return result
}

func isMihomoLegacyBaseTag(tag string, inboundTag string) bool {
	base := strings.TrimSpace(inboundTag)
	if base == "" {
		return false
	}
	trimmed := strings.TrimSpace(tag)
	if trimmed == "" {
		return false
	}
	if trimmed == buildMihomoLegacyBaseTag(base) {
		return true
	}
	return trimmed == buildMihomoClientSubTagWithPrefix(base, "", legacyManagedMihomoClientSubTagPrefix)
}

func buildMihomoClientSubTagWithPrefix(inboundTag, clientName, prefix string) string {
	tag := buildClientSubTag(inboundTag, clientName)
	if tag == "" {
		return ""
	}
	return strings.TrimSpace(prefix) + tag
}

func buildMihomoLegacyBaseTag(inboundTag string) string {
	return buildMihomoClientSubTagWithPrefix(inboundTag, "", managedMihomoClientSubTagPrefix)
}

func buildMihomoClientSubTag(inboundTag, clientName string) string {
	return buildMihomoClientSubTagWithPrefix(inboundTag, clientName, managedMihomoClientSubTagPrefix)
}

func (s *MihomoSyncService) canReuseLegacyInboundTag(db *gorm.DB, clientID uint, inboundID uint) bool {
	if inboundID == 0 {
		return false
	}

	condition := "EXISTS (SELECT 1 FROM json_each(mihomo_clients.inbounds) WHERE json_each.value = ?)"
	var ownCount int64
	if err := db.Table("mihomo_clients").Where("id = ? AND "+condition, clientID, inboundID).Count(&ownCount).Error; err != nil {
		logger.Warningf("[MihomoSync] failed to verify inbound ownership for id=%d: %v", inboundID, err)
		return false
	}
	if ownCount == 0 {
		return false
	}

	var count int64
	if err := db.Table("mihomo_clients").Where(condition, inboundID).Count(&count).Error; err != nil {
		logger.Warningf("[MihomoSync] failed to count inbound owners for id=%d: %v", inboundID, err)
		return false
	}
	return count <= 1
}

func (s *MihomoSyncService) findClientManagedSubOutbound(db *gorm.DB, clientID uint, inboundID uint) (*model.SubOutbound, error) {
	if clientID == 0 || inboundID == 0 {
		return nil, nil
	}
	record := &model.SubOutbound{}
	err := db.Model(model.SubOutbound{}).
		Where("source_type = ? AND source_client_id = ? AND source_inbound_id = ?", subOutboundSourceMihomoClient, clientID, inboundID).
		First(record).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return record, nil
}

func (s *MihomoSyncService) buildSyncedOutbound(
	db *gorm.DB,
	inbound *model.MihomoInbound,
	clientConfig map[string]interface{},
	clientName string,
	serverHost string,
	preserveRaw bool,
) (map[string]interface{}, map[string]interface{}, error) {
	if inbound == nil {
		return nil, nil, fmt.Errorf("mihomo inbound is nil")
	}

	if len(inbound.OutJson) < 5 {
		if len(inbound.OutJson) == 0 {
			inbound.OutJson = []byte("{}")
		}
		if err := fillMihomoOutJson(inbound, serverHost); err != nil {
			return nil, nil, fmt.Errorf("failed to build out_json: %v", err)
		}
		if err := db.Model(model.MihomoInbound{}).Where("id = ?", inbound.Id).Update("out_json", inbound.OutJson).Error; err != nil {
			logger.Warningf("[MihomoSync] failed to persist out_json for inbound %s: %v", inbound.Tag, err)
		}
	}

	var outbound map[string]interface{}
	if err := json.Unmarshal(inbound.OutJson, &outbound); err != nil {
		if err := fillMihomoOutJson(inbound, serverHost); err != nil {
			return nil, nil, fmt.Errorf("failed to parse out_json: %v", err)
		}
		if err := db.Model(model.MihomoInbound{}).Where("id = ?", inbound.Id).Update("out_json", inbound.OutJson).Error; err != nil {
			logger.Warningf("[MihomoSync] failed to persist regenerated out_json for inbound %s: %v", inbound.Tag, err)
		}
		if err := json.Unmarshal(inbound.OutJson, &outbound); err != nil {
			return nil, nil, fmt.Errorf("failed to parse regenerated out_json: %v", err)
		}
	}

	applyServerHostOverride(outbound, serverHost)

	baseInbound := inbound.ToBase()
	protocol, _ := outbound["type"].(string)
	if protocol == "trusttunnel" {
		util.SanitizeTrustTunnelOutbound(outbound)
	}
	if protocol == "shadowsocks" {
		var inboundOptions map[string]interface{}
		if err := json.Unmarshal(baseInbound.Options, &inboundOptions); err == nil {
			if password, ok := inboundOptions["password"].(string); ok && password != "" {
				outbound["password"] = password
			}
		}
	} else {
		config, _ := clientConfig[protocol].(map[string]interface{})
		mergeClientProtocolConfigForNamespace(outbound, config, &baseInbound, "mihomo", clientName)
	}
	hydrateOutboundTLSFromInboundTLS(outbound, &baseInbound)
	if baseInbound.Tls != nil {
		refreshManagedSubscriptionOutboundTLS(outbound, baseInbound.Tls)
	}

	clashSource, err := cloneMihomoOutboundMap(outbound)
	if err != nil {
		return nil, nil, err
	}

	stripSyncedSubscriptionJSONFields(outbound)

	return outbound, clashSource, nil
}

func (s *MihomoSyncService) saveSyncedSubOutbound(
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
		subOutboundSourceMihomoClient,
		clientID,
		inboundID,
		subTag,
	)
}

func cloneMihomoOutboundMap(src map[string]interface{}) (map[string]interface{}, error) {
	if src == nil {
		return nil, nil
	}

	data, err := json.Marshal(src)
	if err != nil {
		return nil, err
	}

	var cloned map[string]interface{}
	if err := json.Unmarshal(data, &cloned); err != nil {
		return nil, err
	}
	return cloned, nil
}

func buildMihomoClashOptions(outbound map[string]interface{}, subTag string) (json.RawMessage, error) {
	if outbound == nil {
		return nil, nil
	}

	cloned, err := cloneMihomoOutboundMap(outbound)
	if err != nil {
		return nil, err
	}
	if cloned == nil {
		return nil, nil
	}

	if strings.TrimSpace(subTag) != "" {
		cloned["tag"] = strings.TrimSpace(subTag)
	}

	result := convertMihomoOutboundsToClash([]map[string]interface{}{cloned})
	if result == nil {
		return nil, nil
	}
	if len(result.ValidationErrs) > 0 {
		return nil, fmt.Errorf("invalid mihomo clash options: %s", strings.Join(result.ValidationErrs, "; "))
	}
	if len(result.Proxies) == 0 {
		return nil, nil
	}

	tag := strings.TrimSpace(subTag)
	var proxy map[string]interface{}
	for _, item := range result.Proxies {
		name, _ := item["name"].(string)
		if strings.TrimSpace(name) == tag {
			proxy = item
			break
		}
	}
	if proxy == nil {
		proxy = result.Proxies[0]
	}

	data, err := json.MarshalIndent(proxy, "", "  ")
	if err != nil {
		return nil, err
	}
	return normalizeClashProxyOptionsTag(data, subTag), nil
}
