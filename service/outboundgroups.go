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
	"github.com/alireza0/s-ui/util/common"

	"gorm.io/gorm"
)

// OutboundGroupService manages outbound groups in Outbounds page.
type OutboundGroupService struct{}

type outboundGroupReorderRequest struct {
	IDs []uint `json:"ids"`
}

func (s *OutboundGroupService) GetAll() ([]*model.OutboundGroup, error) {
	db := database.GetDB()
	if err := ensureOutboundGroupSortOrders(db); err != nil {
		return nil, err
	}
	var groups []*model.OutboundGroup
	if err := db.Model(model.OutboundGroup{}).Order("sort_order ASC").Order("id ASC").Find(&groups).Error; err != nil {
		return nil, err
	}
	return groups, nil
}

func ensureOutboundGroupSortOrders(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("database is not initialized")
	}

	var groups []model.OutboundGroup
	if err := db.Model(model.OutboundGroup{}).
		Select("id", "sort_order").
		Order("CASE WHEN sort_order <= 0 THEN 1 ELSE 0 END").
		Order("sort_order ASC").
		Order("id ASC").
		Find(&groups).Error; err != nil {
		return err
	}
	if len(groups) == 0 {
		return nil
	}

	seen := make(map[int]struct{}, len(groups))
	needsNormalization := false
	for _, group := range groups {
		if group.SortOrder <= 0 {
			needsNormalization = true
			break
		}
		if _, exists := seen[group.SortOrder]; exists {
			needsNormalization = true
			break
		}
		seen[group.SortOrder] = struct{}{}
	}
	if !needsNormalization {
		return nil
	}

	for index, group := range groups {
		if err := db.Model(&model.OutboundGroup{}).
			Where("id = ?", group.Id).
			Update("sort_order", index+1).Error; err != nil {
			return err
		}
	}

	return nil
}

func nextOutboundGroupSortOrder(tx *gorm.DB) (int, error) {
	if err := ensureOutboundGroupSortOrders(tx); err != nil {
		return 0, err
	}

	var maxSortOrder int
	if err := tx.Model(model.OutboundGroup{}).
		Select("COALESCE(MAX(sort_order), 0)").
		Scan(&maxSortOrder).Error; err != nil {
		return 0, err
	}

	return maxSortOrder + 1, nil
}

func normalizeOutboundGroupReorderIDs(ids []uint) []uint {
	result := make([]uint, 0, len(ids))
	seen := make(map[uint]struct{}, len(ids))
	for _, id := range ids {
		if id == 0 {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
}

func (s *OutboundGroupService) reorder(tx *gorm.DB, ids []uint) error {
	cleanedIDs := normalizeOutboundGroupReorderIDs(ids)
	if len(cleanedIDs) == 0 {
		return fmt.Errorf("group ids are required")
	}

	if err := ensureOutboundGroupSortOrders(tx); err != nil {
		return err
	}

	var groups []model.OutboundGroup
	if err := tx.Model(model.OutboundGroup{}).Select("id").Find(&groups).Error; err != nil {
		return err
	}
	if len(groups) != len(cleanedIDs) {
		return fmt.Errorf("reorder payload does not match existing groups")
	}

	expected := make(map[uint]struct{}, len(groups))
	for _, group := range groups {
		expected[group.Id] = struct{}{}
	}
	for _, id := range cleanedIDs {
		if _, exists := expected[id]; !exists {
			return fmt.Errorf("unknown group id: %d", id)
		}
		delete(expected, id)
	}
	if len(expected) > 0 {
		return fmt.Errorf("reorder payload does not include all groups")
	}

	for index, id := range cleanedIDs {
		if err := tx.Model(&model.OutboundGroup{}).
			Where("id = ?", id).
			Update("sort_order", index+1).Error; err != nil {
			return err
		}
	}

	return nil
}

func (s *OutboundGroupService) Save(tx *gorm.DB, act string, data json.RawMessage) error {
	switch act {
	case "new", "edit":
		var group model.OutboundGroup
		if err := json.Unmarshal(data, &group); err != nil {
			return err
		}
		group.Name = strings.TrimSpace(group.Name)
		group.SubscriptionUrl = strings.TrimSpace(group.SubscriptionUrl)
		if group.Outbounds == "" {
			group.Outbounds = "[]"
		}
		if group.Id > 0 {
			var existing model.OutboundGroup
			if err := tx.Where("id = ?", group.Id).First(&existing).Error; err == nil {
				if group.SortOrder <= 0 {
					group.SortOrder = existing.SortOrder
				}
			} else if !database.IsNotFound(err) {
				return err
			}
		} else if group.SortOrder <= 0 {
			nextSortOrder, err := nextOutboundGroupSortOrder(tx)
			if err != nil {
				return err
			}
			group.SortOrder = nextSortOrder
		}
		return tx.Save(&group).Error
	case "del":
		var name string
		if err := json.Unmarshal(data, &name); err != nil {
			return err
		}

		var group model.OutboundGroup
		if err := tx.Where("name = ?", name).First(&group).Error; err != nil {
			return err
		}

		tags := parseOutboundGroupTags(group.Outbounds)
		if len(tags) > 0 {
			if corePtr.IsRunning() {
				typeByTag := make(map[string]string, len(tags))
				var groupedOutbounds []model.Outbound
				if err := tx.Model(model.Outbound{}).Select("tag", "type").Where("tag IN ?", tags).Find(&groupedOutbounds).Error; err != nil {
					return err
				}
				for _, outbound := range groupedOutbounds {
					typeByTag[outbound.Tag] = outbound.Type
				}
				for _, tag := range tags {
					if err := removeRuntimeOutboundFromCore(tag, typeByTag[tag]); err != nil {
						logger.Warningf("[OutboundGroup] remove outbound from running core failed: %s, err: %v", tag, err)
					}
				}
			}
			if err := tx.Where("tag IN ?", tags).Delete(&model.Outbound{}).Error; err != nil {
				return err
			}
		}

		return tx.Where("name = ?", name).Delete(model.OutboundGroup{}).Error
	case "reorder":
		var request outboundGroupReorderRequest
		if err := json.Unmarshal(data, &request); err != nil {
			return err
		}
		return s.reorder(tx, request.IDs)
	default:
		return common.NewErrorf("unknown action: %s", act)
	}
}

func (s *OutboundGroupService) FetchAndSaveSubscription(groupName string, url string, allowInsecure bool) error {
	groupName = strings.TrimSpace(groupName)
	url = strings.TrimSpace(url)
	if groupName == "" || url == "" {
		return fmt.Errorf("group_name and url are required")
	}

	jsonData, err := fetchSubscriptionJSON(url, allowInsecure)
	if err != nil {
		return err
	}
	rawByTag, err := extractSubscriptionJSONOutboundRawByTag(jsonData)
	if err != nil {
		return err
	}

	proxyOutbounds, err := extractProxyOutboundsRawWithoutConversion(jsonData)
	if err != nil {
		return err
	}
	if len(proxyOutbounds) == 0 {
		return fmt.Errorf("subscription has no valid proxy outbounds")
	}

	db := database.GetDB()
	var group model.OutboundGroup
	if err := db.Where("name = ?", groupName).First(&group).Error; err != nil {
		return err
	}

	savedTags := make([]string, 0, len(proxyOutbounds))
	for _, outbound := range proxyOutbounds {
		tag, _ := outbound["tag"].(string)
		if tag == "" {
			continue
		}

		if err := upsertImportedOutbound(db, outbound, rawByTag[tag]); err != nil {
			logger.Errorf("[OutboundGroup] save outbound failed [%s]: %v", tag, err)
			continue
		}

		savedTags = append(savedTags, tag)
	}

	if len(savedTags) == 0 {
		return fmt.Errorf("no valid outbound nodes were saved")
	}

	tagsJson, _ := json.Marshal(savedTags)
	err = db.Model(&model.OutboundGroup{}).Where("name = ?", groupName).Updates(map[string]interface{}{
		"outbounds":        string(tagsJson),
		"subscription_url": url,
		"allow_insecure":   allowInsecure,
	}).Error
	if err != nil {
		return err
	}

	s.notifyOutboundsChanged()
	return nil
}

func (s *OutboundGroupService) RefreshSubscription(groupName string, url string, allowInsecure bool) (*SubscriptionRefreshResult, error) {
	result := &SubscriptionRefreshResult{
		Added:   []string{},
		Removed: []string{},
		Updated: []string{},
	}

	groupName = strings.TrimSpace(groupName)
	url = strings.TrimSpace(url)
	if groupName == "" || url == "" {
		return nil, fmt.Errorf("group_name and url are required")
	}

	dbConn := database.GetDB()
	var group model.OutboundGroup
	if err := dbConn.Where("name = ?", groupName).First(&group).Error; err != nil {
		return nil, err
	}

	jsonData, err := fetchSubscriptionJSON(url, allowInsecure)
	if err != nil {
		return nil, err
	}
	rawByTag, err := extractSubscriptionJSONOutboundRawByTag(jsonData)
	if err != nil {
		return nil, err
	}

	newOutbounds, err := extractProxyOutboundsRawWithoutConversion(jsonData)
	if err != nil {
		return nil, err
	}
	if len(newOutbounds) == 0 {
		return nil, fmt.Errorf("subscription has no valid proxy outbounds")
	}

	oldOutbounds, err := loadGroupedOutbounds(dbConn, parseOutboundGroupTags(group.Outbounds))
	if err != nil {
		return nil, err
	}

	oldMap := make(map[string]map[string]interface{})
	oldTypeByTag := make(map[string]string)
	for _, ob := range oldOutbounds {
		tag, _ := ob["tag"].(string)
		outType, _ := ob["type"].(string)
		if tag != "" {
			oldMap[tag+"|"+outType] = ob
			if _, ok := oldTypeByTag[tag]; !ok || outType == "shadowtls" {
				oldTypeByTag[tag] = outType
			}
		}
	}

	newMap := make(map[string]map[string]interface{})
	for _, ob := range newOutbounds {
		tag, _ := ob["tag"].(string)
		outType, _ := ob["type"].(string)
		if tag != "" {
			newMap[tag+"|"+outType] = ob
		}
	}

	for key, ob := range newMap {
		tag, _ := ob["tag"].(string)
		if _, exists := oldMap[key]; exists {
			result.Updated = append(result.Updated, tag)
		} else {
			result.Added = append(result.Added, tag)
		}
	}

	for key, ob := range oldMap {
		if _, exists := newMap[key]; !exists {
			tag, _ := ob["tag"].(string)
			result.Removed = append(result.Removed, tag)
		}
	}

	if len(result.Removed) > 0 {
		if corePtr.IsRunning() {
			for _, tag := range result.Removed {
				if err := removeRuntimeOutboundFromCore(tag, oldTypeByTag[tag]); err != nil {
					logger.Warningf("[OutboundGroup] remove stale outbound from running core failed: %s, err: %v", tag, err)
				}
			}
		}
		if err := dbConn.Where("tag IN ?", result.Removed).Delete(&model.Outbound{}).Error; err != nil {
			return nil, err
		}
	}

	savedTags := make([]string, 0, len(newOutbounds))
	for _, outbound := range newOutbounds {
		tag, _ := outbound["tag"].(string)
		if tag == "" {
			continue
		}

		if err := upsertImportedOutbound(dbConn, outbound, rawByTag[tag]); err != nil {
			logger.Errorf("[OutboundGroup] refresh save outbound failed [%s]: %v", tag, err)
			continue
		}

		savedTags = append(savedTags, tag)
	}

	tagsJson, _ := json.Marshal(savedTags)
	err = dbConn.Model(&model.OutboundGroup{}).Where("name = ?", groupName).Updates(map[string]interface{}{
		"outbounds":        string(tagsJson),
		"subscription_url": url,
		"allow_insecure":   allowInsecure,
	}).Error
	if err != nil {
		return nil, err
	}

	s.notifyOutboundsChanged()
	return result, nil
}

func (s *OutboundGroupService) notifyOutboundsChanged() {
	LastUpdate = time.Now().Unix()
	proManager := GetProManagerService(&ConfigService{})
	proManager.regenerateOutboundConfigs()
	proManager.regenerateCoreConfig()
}

func extractProxyOutboundsRawWithoutConversion(jsonData []byte) ([]map[string]interface{}, error) {
	var config map[string]interface{}
	if err := json.Unmarshal(jsonData, &config); err != nil {
		return nil, fmt.Errorf("failed to parse subscription JSON: %v", err)
	}

	outboundsRaw, ok := config["outbounds"]
	if !ok {
		return nil, fmt.Errorf("subscription JSON does not contain outbounds")
	}

	outboundsArr, ok := outboundsRaw.([]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid outbounds format")
	}

	proxyOutbounds := make([]map[string]interface{}, 0, len(outboundsArr))
	for _, item := range outboundsArr {
		outbound, ok := item.(map[string]interface{})
		if !ok || outbound == nil {
			continue
		}

		tag, _ := outbound["tag"].(string)
		if strings.TrimSpace(tag) == "" {
			continue
		}
		outType, _ := outbound["type"].(string)
		if strings.TrimSpace(outType) == "" {
			continue
		}
		if !isProxyOutbound(outbound) {
			continue
		}

		proxyOutbounds = append(proxyOutbounds, cloneMap(outbound))
	}

	return proxyOutbounds, nil
}

func parseOutboundGroupTags(raw string) []string {
	if raw == "" {
		return []string{}
	}
	var tags []string
	if err := json.Unmarshal([]byte(raw), &tags); err != nil {
		return []string{}
	}
	return tags
}

func upsertImportedOutbound(db *gorm.DB, outbound map[string]interface{}, raw json.RawMessage) error {
	outboundBytes, err := json.Marshal(outbound)
	if err != nil {
		return err
	}

	var dbOutbound model.Outbound
	if err := dbOutbound.UnmarshalJSON(outboundBytes); err != nil {
		return err
	}
	if len(raw) > 0 {
		dbOutbound.RawOutbound = normalizeOutboundRawPayload(raw)
	} else {
		dbOutbound.RawOutbound = normalizeOutboundRawPayload(outboundBytes)
	}

	var existing model.Outbound
	if err := db.Where("tag = ?", dbOutbound.Tag).First(&existing).Error; err == nil {
		dbOutbound.Id = existing.Id
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}

	if corePtr.IsRunning() {
		configData, err := resolveOutboundJSON(&dbOutbound)
		if err != nil {
			return err
		}
		runtimePayloads, err := buildRuntimeOutboundPayloads(configData, dbOutbound.Type)
		if err != nil {
			return err
		}
		if dbOutbound.Id > 0 {
			if err := removeRuntimeOutboundFromCore(existing.Tag, existing.Type); err != nil {
				return err
			}
		}
		for _, payload := range runtimePayloads {
			if err := corePtr.AddOutbound(payload); err != nil {
				return err
			}
		}
	}

	return db.Save(&dbOutbound).Error
}

func loadGroupedOutbounds(db *gorm.DB, tags []string) ([]map[string]interface{}, error) {
	if len(tags) == 0 {
		return []map[string]interface{}{}, nil
	}

	var outbounds []*model.Outbound
	if err := db.Model(model.Outbound{}).Where("tag IN ?", tags).Find(&outbounds).Error; err != nil {
		return nil, err
	}

	result := make([]map[string]interface{}, 0, len(outbounds))
	for _, outbound := range outbounds {
		raw, err := resolveOutboundJSON(outbound)
		if err != nil {
			continue
		}
		m := map[string]interface{}{}
		if err := json.Unmarshal(raw, &m); err != nil {
			continue
		}
		result = append(result, m)
	}
	return result, nil
}
