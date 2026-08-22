package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/logger"
	"github.com/alireza0/s-ui/util/common"

	"gorm.io/gorm"
)

// OutboundGroupService manages outbound groups in Outbounds page.
type OutboundGroupService struct{}

const (
	singboxOutboundSubscriptionImportMaxResponseBytes = 8 * 1024 * 1024
	singboxOutboundSubscriptionImportMaxNodes         = 512
	singboxOutboundSubscriptionImportMaxNodeBytes     = 64 * 1024
	singboxOutboundSubscriptionImportMaxPayloadBytes  = 8 * 1024 * 1024
)

var singboxOutboundSubscriptionImportMu sync.Mutex

var ErrSingboxOutboundSubscriptionImportBusy = errors.New("sing-box outbound subscription import is already running")

// CommittedSingboxOutboundSubscriptionImportError reports that imported
// outbounds are durable but the independent sing-box runtime configuration
// could not be regenerated.
type CommittedSingboxOutboundSubscriptionImportError struct {
	Err error
}

func (e *CommittedSingboxOutboundSubscriptionImportError) Error() string {
	if e == nil || e.Err == nil {
		return "sing-box outbound subscription import committed but runtime configuration update failed"
	}
	return e.Err.Error()
}

func (e *CommittedSingboxOutboundSubscriptionImportError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

type outboundGroupReorderRequest struct {
	IDs []uint `json:"ids"`
}

func (s *OutboundGroupService) GetAll() ([]*model.OutboundGroup, error) {
	db := database.GetDB()
	if err := ensureOutboundGroupSortOrders(db); err != nil {
		return nil, err
	}
	groups := make([]*model.OutboundGroup, 0)
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
	_, err := s.SaveWithRuntimeImpact(tx, act, data)
	return err
}

// SaveWithRuntimeImpact distinguishes panel-only group metadata/order updates
// from deletion, which removes actual default-chain outbounds.
func (s *OutboundGroupService) SaveWithRuntimeImpact(tx *gorm.DB, act string, data json.RawMessage) (bool, error) {
	if !singboxOutboundSubscriptionImportMu.TryLock() {
		return false, ErrSingboxOutboundSubscriptionImportBusy
	}
	defer singboxOutboundSubscriptionImportMu.Unlock()
	return s.saveWithRuntimeImpactLocked(tx, act, data)
}

func (s *OutboundGroupService) saveWithRuntimeImpactLocked(tx *gorm.DB, act string, data json.RawMessage) (bool, error) {
	switch act {
	case "new", "edit":
		var group model.OutboundGroup
		if err := json.Unmarshal(data, &group); err != nil {
			return false, err
		}
		if act == "new" {
			// A copied create payload must receive a fresh database identity.
			group.Id = 0
		} else if group.Id == 0 {
			return false, fmt.Errorf("outbound group id is required for edit")
		}
		group.Name = strings.TrimSpace(group.Name)
		group.SubscriptionUrl = strings.TrimSpace(group.SubscriptionUrl)
		if group.Outbounds == "" {
			group.Outbounds = "[]"
		}
		if act == "edit" {
			var existing model.OutboundGroup
			if err := tx.Where("id = ?", group.Id).First(&existing).Error; err != nil {
				return false, err
			} else if group.SortOrder <= 0 {
				group.SortOrder = existing.SortOrder
			}
		} else if group.SortOrder <= 0 {
			nextSortOrder, err := nextOutboundGroupSortOrder(tx)
			if err != nil {
				return false, err
			}
			group.SortOrder = nextSortOrder
		}
		return false, tx.Save(&group).Error
	case "del":
		var name string
		if err := json.Unmarshal(data, &name); err != nil {
			return false, err
		}
		name = strings.TrimSpace(name)
		if name == "" {
			return false, fmt.Errorf("group name is required")
		}

		var group model.OutboundGroup
		if err := tx.Where("name = ?", name).First(&group).Error; err != nil {
			return false, err
		}

		tags := parseOutboundGroupTags(group.Outbounds)
		if err := validateSingboxOutboundRemovalReferences(tx, tags, []string{group.Name}); err != nil {
			return false, err
		}
		if len(tags) > 0 {
			if err := tx.Where("tag IN ?", tags).Delete(&model.Outbound{}).Error; err != nil {
				return false, err
			}
		}

		if err := tx.Where("name = ?", name).Delete(model.OutboundGroup{}).Error; err != nil {
			return false, err
		}
		return len(tags) > 0, nil
	case "reorder":
		var request outboundGroupReorderRequest
		if err := json.Unmarshal(data, &request); err != nil {
			return false, err
		}
		return false, s.reorder(tx, request.IDs)
	default:
		return false, common.NewErrorf("unknown action: %s", act)
	}
}

func (s *OutboundGroupService) FetchAndSaveSubscription(groupName string, url string, allowInsecure bool) error {
	if !singboxOutboundSubscriptionImportMu.TryLock() {
		return ErrSingboxOutboundSubscriptionImportBusy
	}
	defer singboxOutboundSubscriptionImportMu.Unlock()

	groupName = strings.TrimSpace(groupName)
	url = strings.TrimSpace(url)
	if groupName == "" || url == "" {
		return fmt.Errorf("group_name and url are required")
	}

	jsonData, err := fetchSubscriptionJSONWithTimeoutAndLimit(
		url,
		allowInsecure,
		30*time.Second,
		singboxOutboundSubscriptionImportMaxResponseBytes,
	)
	if err != nil {
		return err
	}
	proxyOutbounds, rawByTag, err := extractSingboxOutboundGroupSubscription(jsonData)
	if err != nil {
		return err
	}

	db := database.GetDB()
	if err := s.applyImportedSubscription(db, groupName, url, allowInsecure, proxyOutbounds, rawByTag, true); err != nil {
		return err
	}

	return s.notifyOutboundsChanged()
}

func (s *OutboundGroupService) RefreshSubscription(groupName string, url string, allowInsecure bool) (*SubscriptionRefreshResult, error) {
	if !singboxOutboundSubscriptionImportMu.TryLock() {
		return nil, ErrSingboxOutboundSubscriptionImportBusy
	}
	defer singboxOutboundSubscriptionImportMu.Unlock()

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

	jsonData, err := fetchSubscriptionJSONWithTimeoutAndLimit(
		url,
		allowInsecure,
		30*time.Second,
		singboxOutboundSubscriptionImportMaxResponseBytes,
	)
	if err != nil {
		return nil, err
	}
	newOutbounds, rawByTag, err := extractSingboxOutboundGroupSubscription(jsonData)
	if err != nil {
		return nil, err
	}

	oldOutbounds, err := loadGroupedOutbounds(dbConn, parseOutboundGroupTags(group.Outbounds))
	if err != nil {
		return nil, err
	}

	oldMap := make(map[string]map[string]interface{})
	for _, ob := range oldOutbounds {
		tag, _ := ob["tag"].(string)
		outType, _ := ob["type"].(string)
		if tag != "" {
			oldMap[tag+"|"+outType] = ob
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

	if err := s.applyImportedSubscription(dbConn, groupName, url, allowInsecure, newOutbounds, rawByTag, true); err != nil {
		return nil, err
	}

	if err := s.notifyOutboundsChanged(); err != nil {
		return result, err
	}
	return result, nil
}

func (s *OutboundGroupService) notifyOutboundsChanged() error {
	markLastUpdate(time.Now().Unix())
	proManager := GetProManagerService(&ConfigService{})
	if err := proManager.RegenerateCoreConfig(); err != nil {
		logger.Warning("[OutboundGroup] regenerate sing-box config failed: ", err)
		return &CommittedSingboxOutboundSubscriptionImportError{Err: err}
	}
	return nil
}

func (s *OutboundGroupService) applyImportedSubscription(
	db *gorm.DB,
	groupName string,
	url string,
	allowInsecure bool,
	outbounds []map[string]interface{},
	rawByTag map[string]json.RawMessage,
	replaceExisting bool,
) error {
	if db == nil {
		return fmt.Errorf("database is not initialized")
	}
	tx := db.Begin()
	if tx.Error != nil {
		return tx.Error
	}

	rollback := func(err error) error {
		tx.Rollback()
		return err
	}

	var group model.OutboundGroup
	if err := tx.Where("name = ?", groupName).First(&group).Error; err != nil {
		return rollback(err)
	}

	newTags := singboxImportedOutboundTags(outbounds)
	if len(newTags) == 0 {
		return rollback(fmt.Errorf("subscription has no valid proxy outbounds"))
	}
	oldTags := parseOutboundGroupTags(group.Outbounds)
	if err := validateSingboxImportedTagOwnership(tx, group, newTags); err != nil {
		return rollback(err)
	}

	if replaceExisting {
		previousRuntimeTags, err := collectSingboxRuntimeOutboundTagsByTags(tx, oldTags)
		if err != nil {
			return rollback(err)
		}
		currentRuntimeTags := collectSingboxRuntimeOutboundTagsFromMaps(outbounds)
		newTagSet := normalizedSingboxOutboundReferenceSet(newTags)
		removedTags := make([]string, 0)
		for _, tag := range oldTags {
			if _, kept := newTagSet[tag]; !kept {
				removedTags = append(removedTags, tag)
			}
		}
		removedRuntimeTags := removedSingboxRuntimeTags(previousRuntimeTags, currentRuntimeTags)
		if err := validateSingboxOutboundRemovalReferences(tx, removedRuntimeTags, []string{group.Name}); err != nil {
			return rollback(err)
		}
		if len(removedTags) > 0 {
			if err := tx.Where("tag IN ?", removedTags).Delete(&model.Outbound{}).Error; err != nil {
				return rollback(err)
			}
		}
	}

	for _, outbound := range outbounds {
		tag := strings.TrimSpace(firstString(outbound["tag"]))
		if err := upsertImportedOutbound(tx, outbound, rawByTag[tag]); err != nil {
			return rollback(fmt.Errorf("save imported sing-box outbound %q: %w", tag, err))
		}
	}

	tagsJSON, err := json.Marshal(newTags)
	if err != nil {
		return rollback(err)
	}
	if err := tx.Model(&model.OutboundGroup{}).Where("id = ?", group.Id).Updates(map[string]interface{}{
		"outbounds":        string(tagsJSON),
		"subscription_url": url,
		"allow_insecure":   allowInsecure,
	}).Error; err != nil {
		return rollback(err)
	}

	if _, err := GetProManagerService(&ConfigService{}).GenerateFullConfigWithDB(tx); err != nil {
		return rollback(fmt.Errorf("invalid sing-box runtime config after subscription import: %w", err))
	}
	if err := tx.Commit().Error; err != nil {
		return err
	}
	return nil
}

func validateSingboxImportedSubscriptionSize(outbounds []map[string]interface{}, rawByTag map[string]json.RawMessage) error {
	if len(outbounds) > singboxOutboundSubscriptionImportMaxNodes {
		return fmt.Errorf("sing-box subscription contains more than %d valid nodes", singboxOutboundSubscriptionImportMaxNodes)
	}

	totalBytes := 0
	for _, outbound := range outbounds {
		if outbound == nil {
			continue
		}
		tag := strings.TrimSpace(firstString(outbound["tag"]))
		encoded, err := json.Marshal(outbound)
		if err != nil {
			return err
		}
		entryBytes := len(encoded) + len(rawByTag[tag])
		if entryBytes > singboxOutboundSubscriptionImportMaxNodeBytes {
			return fmt.Errorf("sing-box subscription node %q exceeds the %d KiB safety limit", tag, singboxOutboundSubscriptionImportMaxNodeBytes/1024)
		}
		totalBytes += entryBytes
		if totalBytes > singboxOutboundSubscriptionImportMaxPayloadBytes {
			return fmt.Errorf("sing-box subscription nodes exceed the %d MiB safety limit", singboxOutboundSubscriptionImportMaxPayloadBytes/(1024*1024))
		}
	}
	return nil
}

func singboxImportedOutboundTags(outbounds []map[string]interface{}) []string {
	tags := make([]string, 0, len(outbounds))
	seen := make(map[string]struct{}, len(outbounds))
	for _, outbound := range outbounds {
		tag := strings.TrimSpace(firstString(outbound["tag"]))
		if tag == "" {
			continue
		}
		if _, exists := seen[tag]; exists {
			continue
		}
		seen[tag] = struct{}{}
		tags = append(tags, tag)
	}
	return tags
}

func validateSingboxImportedTagOwnership(tx *gorm.DB, group model.OutboundGroup, tags []string) error {
	owned := normalizedSingboxOutboundReferenceSet(parseOutboundGroupTags(group.Outbounds))
	if len(tags) == 0 {
		return nil
	}

	var existing []model.Outbound
	if err := tx.Model(&model.Outbound{}).Select("tag").Where("tag IN ?", tags).Find(&existing).Error; err != nil {
		return err
	}
	conflicts := make([]string, 0)
	for _, outbound := range existing {
		if _, allowed := owned[strings.TrimSpace(outbound.Tag)]; !allowed {
			conflicts = append(conflicts, strings.TrimSpace(outbound.Tag))
		}
	}

	if len(conflicts) == 0 {
		candidateTags := normalizedSingboxOutboundReferenceSet(tags)
		var groups []model.OutboundGroup
		if err := tx.Model(&model.OutboundGroup{}).Select("id", "name", "outbounds").Find(&groups).Error; err != nil {
			return err
		}
		for _, otherGroup := range groups {
			if otherGroup.Id == group.Id {
				continue
			}
			for _, tag := range parseOutboundGroupTags(otherGroup.Outbounds) {
				if _, found := candidateTags[tag]; found {
					conflicts = append(conflicts, fmt.Sprintf("%s (group %s)", tag, otherGroup.Name))
				}
			}
		}
	}
	if len(conflicts) == 0 {
		return nil
	}
	return fmt.Errorf("subscription contains outbound tag(s) already owned outside group %q: %s", group.Name, strings.Join(conflicts, ", "))
}

// extractSingboxOutboundGroupSubscription decodes each outbound exactly once,
// keeping raw JSON only for accepted proxy nodes. It bounds both node count and
// retained payload before the data reaches SQLite or the reactive frontend.
func extractSingboxOutboundGroupSubscription(jsonData []byte) ([]map[string]interface{}, map[string]json.RawMessage, error) {
	var document struct {
		Outbounds []json.RawMessage `json:"outbounds"`
	}
	if err := json.Unmarshal(jsonData, &document); err != nil {
		return nil, nil, fmt.Errorf("failed to parse subscription JSON: %v", err)
	}
	if len(document.Outbounds) > singboxOutboundSubscriptionImportMaxNodes {
		return nil, nil, fmt.Errorf("sing-box subscription contains more than %d outbound entries", singboxOutboundSubscriptionImportMaxNodes)
	}

	outbounds := make([]map[string]interface{}, 0, len(document.Outbounds))
	rawByTag := make(map[string]json.RawMessage, len(document.Outbounds))
	seen := make(map[string]struct{}, len(document.Outbounds))
	totalBytes := 0
	for _, raw := range document.Outbounds {
		if len(raw) == 0 {
			continue
		}
		if len(raw) > singboxOutboundSubscriptionImportMaxNodeBytes {
			return nil, nil, fmt.Errorf("sing-box subscription node exceeds the %d KiB safety limit", singboxOutboundSubscriptionImportMaxNodeBytes/1024)
		}

		outbound := map[string]interface{}{}
		if err := json.Unmarshal(raw, &outbound); err != nil {
			continue
		}
		tag := strings.TrimSpace(firstString(outbound["tag"]))
		outboundType := strings.TrimSpace(firstString(outbound["type"]))
		if tag == "" || outboundType == "" || !isProxyOutbound(outbound) {
			continue
		}
		if _, exists := seen[tag]; exists {
			return nil, nil, fmt.Errorf("sing-box subscription contains duplicate outbound tag %q", tag)
		}
		seen[tag] = struct{}{}
		totalBytes += len(raw)
		if totalBytes > singboxOutboundSubscriptionImportMaxPayloadBytes {
			return nil, nil, fmt.Errorf("sing-box subscription nodes exceed the %d MiB safety limit", singboxOutboundSubscriptionImportMaxPayloadBytes/(1024*1024))
		}
		rawByTag[tag] = append(json.RawMessage(nil), raw...)
		outbounds = append(outbounds, outbound)
	}
	if len(outbounds) == 0 {
		return nil, nil, fmt.Errorf("subscription has no valid proxy outbounds")
	}
	if err := validateSingboxImportedSubscriptionSize(outbounds, rawByTag); err != nil {
		return nil, nil, err
	}
	return outbounds, rawByTag, nil
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
	// RawOutbound retains the complete imported payload; avoid persisting its
	// editable projection a second time in Options.
	dbOutbound.Options = nil

	var existing model.Outbound
	if err := db.Where("tag = ?", dbOutbound.Tag).First(&existing).Error; err == nil {
		dbOutbound.Id = existing.Id
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
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
