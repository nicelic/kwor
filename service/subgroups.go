package service

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/alireza0/s-ui/config"
	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/logger"
	"github.com/alireza0/s-ui/util/common"

	"gorm.io/gorm"
)

// SubGroupService manages subscription-manager groups.
type SubGroupService struct{}

const subOutboundSourceSubGroup = "subgroup"

type subGroupReorderRequest struct {
	IDs []uint `json:"ids"`
}

func (s *SubGroupService) GetAll() ([]*model.SubGroup, error) {
	db := database.GetDB()
	if err := ensureSubGroupSortOrders(db); err != nil {
		return nil, err
	}
	if _, err := s.pruneMissingOutboundTags(db); err != nil {
		return nil, err
	}
	var groups []*model.SubGroup
	if err := db.Model(model.SubGroup{}).Order("sort_order ASC").Order("id ASC").Find(&groups).Error; err != nil {
		return nil, err
	}
	return groups, nil
}

func (s *SubGroupService) GetAllForAutoUpdate() ([]*model.SubGroup, error) {
	db := database.GetDB()
	var groups []*model.SubGroup
	if err := db.Model(model.SubGroup{}).Order("id ASC").Find(&groups).Error; err != nil {
		return nil, err
	}
	return groups, nil
}

func ensureSubGroupSortOrders(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("database is not initialized")
	}

	var groups []model.SubGroup
	if err := db.Model(model.SubGroup{}).
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
		if err := db.Model(&model.SubGroup{}).
			Where("id = ?", group.Id).
			Update("sort_order", index+1).Error; err != nil {
			return err
		}
	}

	return nil
}

func nextSubGroupSortOrder(tx *gorm.DB) (int, error) {
	if err := ensureSubGroupSortOrders(tx); err != nil {
		return 0, err
	}

	var maxSortOrder int
	if err := tx.Model(model.SubGroup{}).
		Select("COALESCE(MAX(sort_order), 0)").
		Scan(&maxSortOrder).Error; err != nil {
		return 0, err
	}

	return maxSortOrder + 1, nil
}

func normalizeSubGroupReorderIDs(ids []uint) []uint {
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

func (s *SubGroupService) reorder(tx *gorm.DB, ids []uint) error {
	cleanedIDs := normalizeSubGroupReorderIDs(ids)
	if len(cleanedIDs) == 0 {
		return fmt.Errorf("group ids are required")
	}

	if err := ensureSubGroupSortOrders(tx); err != nil {
		return err
	}

	var groups []model.SubGroup
	if err := tx.Model(model.SubGroup{}).Select("id").Find(&groups).Error; err != nil {
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
		if err := tx.Model(&model.SubGroup{}).
			Where("id = ?", id).
			Update("sort_order", index+1).Error; err != nil {
			return err
		}
	}

	return nil
}

func (s *SubGroupService) Save(tx *gorm.DB, act string, data json.RawMessage) error {
	switch act {
	case "new", "edit":
		var group model.SubGroup
		if err := json.Unmarshal(data, &group); err != nil {
			return err
		}
		group.Name = strings.TrimSpace(group.Name)
		group.SubscriptionUrl = strings.TrimSpace(group.SubscriptionUrl)
		group.SubscriptionUrlClash = strings.TrimSpace(group.SubscriptionUrlClash)
		if strings.TrimSpace(group.Outbounds) == "" {
			group.Outbounds = "[]"
		}
		oldName := ""
		if group.Id > 0 {
			var existing model.SubGroup
			if err := tx.Where("id = ?", group.Id).First(&existing).Error; err == nil {
				group.AutoUpdateLastAt = existing.AutoUpdateLastAt
				group.AutoUpdateFailedSources = existing.AutoUpdateFailedSources
				group.AutoUpdateError = existing.AutoUpdateError
				if group.SortOrder <= 0 {
					group.SortOrder = existing.SortOrder
				}
				oldName = strings.TrimSpace(existing.Name)
			} else if !database.IsNotFound(err) {
				return err
			}
		} else if group.SortOrder <= 0 {
			nextSortOrder, err := nextSubGroupSortOrder(tx)
			if err != nil {
				return err
			}
			group.SortOrder = nextSortOrder
		}
		if group.SubscriptionUrl == "" && group.SubscriptionUrlClash == "" {
			group.AutoUpdateLastAt = 0
			group.AutoUpdateFailedSources = ""
			group.AutoUpdateError = ""
		}
		if err := validateSubGroupSubJSONFileName(tx, &group); err != nil {
			return err
		}
		if err := tx.Save(&group).Error; err != nil {
			return err
		}
		if oldName != "" && oldName != group.Name {
			if err := s.removeGroupConfig(tx, oldName); err != nil {
				return err
			}
		}
		if err := s.syncGroupConfig(tx, &group); err != nil {
			return err
		}
		return nil

	case "del":
		var name string
		if err := json.Unmarshal(data, &name); err != nil {
			return err
		}
		name = strings.TrimSpace(name)
		if name == "" {
			return fmt.Errorf("group name is required")
		}

		var group model.SubGroup
		if err := tx.Where("name = ?", name).First(&group).Error; err != nil {
			return err
		}

		// Imported groups own their imported suboutbounds; delete their records and files on group delete.
		if strings.TrimSpace(group.SubscriptionUrl) != "" || strings.TrimSpace(group.SubscriptionUrlClash) != "" {
			cleanupTags, cleanupErr := s.collectSubscriptionGroupCleanupTags(tx, &group)
			if cleanupErr != nil {
				logger.Errorf("[SubGroup] collect imported suboutbounds failed: %v", cleanupErr)
			} else if err := s.deleteSubOutboundsByTags(tx, cleanupTags); err != nil {
				logger.Errorf("[SubGroup] delete imported suboutbounds failed: %v", err)
			}
		}

		if err := tx.Where("name = ?", name).Delete(model.SubGroup{}).Error; err != nil {
			return err
		}
		if err := s.removeGroupConfig(tx, name); err != nil {
			return err
		}
		return nil

	case "reorder":
		var request subGroupReorderRequest
		if err := json.Unmarshal(data, &request); err != nil {
			return err
		}
		return s.reorder(tx, request.IDs)

	default:
		return common.NewErrorf("unknown action: %s", act)
	}
}

func cloneSubGroupForArtifacts(src *model.SubGroup) *model.SubGroup {
	if src == nil {
		return nil
	}
	cloned := *src
	return &cloned
}

func (s *SubGroupService) syncGroupConfig(db *gorm.DB, group *model.SubGroup) error {
	snapshot := cloneSubGroupForArtifacts(group)
	if snapshot == nil {
		return nil
	}
	return QueueManagedRuntimeHook(db, func() error {
		return s.saveGroupJson(snapshot)
	})
}

func (s *SubGroupService) removeGroupConfig(db *gorm.DB, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil
	}
	return QueueManagedRuntimeHook(db, func() error {
		return s.deleteGroupJson(name)
	})
}

func (s *SubGroupService) syncSubscriptionGroupConfig(
	db *gorm.DB,
	groupName string,
	jsonURL string,
	clashURL string,
	allowInsecure bool,
	jsonOutbounds []map[string]interface{},
) error {
	groupName = strings.TrimSpace(groupName)
	if groupName == "" {
		return nil
	}

	snapshot := make([]map[string]interface{}, 0, len(jsonOutbounds))
	for _, outbound := range jsonOutbounds {
		snapshot = append(snapshot, cloneMap(outbound))
	}

	return QueueManagedRuntimeHook(db, func() error {
		return s.saveSubGroupSubscriptionJson(groupName, jsonURL, clashURL, allowInsecure, snapshot)
	})
}

// saveGroupJson writes merged outbounds of a group to Promanager_data/sub_json/<group>.json.
func (s *SubGroupService) saveGroupJson(group *model.SubGroup) error {
	if group == nil {
		return nil
	}
	if err := validateSubGroupSubJSONFileName(nil, group); err != nil {
		return err
	}

	subJsonDir, err := getSubJsonDir()
	if err != nil {
		logger.Errorf("[SubGroup] get sub_json dir failed: %v", err)
		return err
	}
	outboundTags := parseSubGroupOutboundTags(group.Outbounds)
	subOutboundsByTag := make(map[string]*model.SubOutbound, len(outboundTags))
	if len(outboundTags) > 0 {
		db := database.GetDB()
		var subOutbounds []*model.SubOutbound
		if err := db.Model(model.SubOutbound{}).Where("tag IN ?", outboundTags).Find(&subOutbounds).Error; err != nil {
			logger.Errorf("[SubGroup] load suboutbounds failed: %v", err)
			return err
		}
		for _, subOutbound := range subOutbounds {
			subOutboundsByTag[subOutbound.Tag] = subOutbound
		}
	}

	// Preserve tag order from group.Outbounds.
	rawOutbounds := make([]map[string]interface{}, 0, len(outboundTags))
	for _, tag := range outboundTags {
		subOutbound := subOutboundsByTag[tag]
		if subOutbound == nil {
			continue
		}
		outboundJSON, err := resolveSubOutboundJSON(subOutbound)
		if err != nil {
			logger.Errorf("[SubGroup] marshal outbound failed [%s]: %v", tag, err)
			continue
		}

		var outboundMap map[string]interface{}
		if err := json.Unmarshal(outboundJSON, &outboundMap); err != nil {
			logger.Errorf("[SubGroup] parse outbound failed [%s]: %v", tag, err)
			continue
		}
		if value, _ := outboundMap["tag"].(string); strings.TrimSpace(value) == "" {
			outboundMap["tag"] = subOutbound.Tag
		}
		if value, _ := outboundMap["type"].(string); strings.TrimSpace(value) == "" {
			outboundMap["type"] = subOutbound.Type
		}
		refreshManagedSubOutboundTLS(outboundMap, subOutbound)
		rawOutbounds = append(rawOutbounds, outboundMap)
	}

	var settingService SettingService
	othersStr, _ := settingService.GetSubJsonExt()
	configData, err := renderManagedSingboxSubscriptionJSON(
		rawOutbounds,
		othersStr,
		settingService.ResolveSubscriptionTLSStore,
	)
	if err != nil {
		logger.Errorf("[SubGroup] render group config failed: %v", err)
		return err
	}

	baseFilename := sanitizeGroupFilename(group.Name)
	configFilePath := filepath.Join(subJsonDir, fmt.Sprintf("%s.json", baseFilename))
	if err := ManagedRuntimeWriteFile(configFilePath, configData); err != nil {
		logger.Errorf("[SubGroup] write group config failed: %v", err)
		return err
	}
	return nil
}

func (s *SubGroupService) deleteGroupJson(name string) error {
	subJsonDir, err := getSubJsonDir()
	if err != nil {
		logger.Errorf("[SubGroup] get sub_json dir failed: %v", err)
		return err
	}

	baseFilename := sanitizeGroupFilename(name)
	configFilePath := filepath.Join(subJsonDir, fmt.Sprintf("%s.json", baseFilename))
	if err := ManagedRuntimeDeleteFile(configFilePath); err != nil {
		logger.Errorf("[SubGroup] remove group config failed: %v", err)
		return err
	}
	return nil
}

func (s *SubGroupService) RegenerateAllGroupConfigs() {
	db := database.GetDB()

	var groups []*model.SubGroup
	if err := db.Model(model.SubGroup{}).Find(&groups).Error; err != nil {
		logger.Errorf("[SubGroup] load groups failed: %v", err)
		return
	}

	for _, group := range groups {
		if err := s.saveGroupJson(group); err != nil {
			logger.Errorf("[SubGroup] regenerate group config failed [%s]: %v", group.Name, err)
		}
	}
}

func sanitizeGroupFilename(name string) string {
	name = strings.TrimSpace(name)
	if name == "" {
		return "group"
	}

	replacer := strings.NewReplacer(
		"/", "_",
		"\\", "_",
		":", "_",
		"*", "_",
		"?", "_",
		"\"", "_",
		"<", "_",
		">", "_",
		"|", "_",
	)
	name = replacer.Replace(name)
	if strings.TrimSpace(name) == "" {
		return "group"
	}
	return name
}

// SubscriptionRefreshResult describes refresh diff details.
type SubscriptionRefreshResult struct {
	Added   []string `json:"added"`
	Removed []string `json:"removed"`
	Updated []string `json:"updated"`
}

// isProxyOutbound returns whether outbound is a real proxy node and should be imported.
func isProxyOutbound(outbound map[string]interface{}) bool {
	typeVal, ok := outbound["type"].(string)
	if !ok {
		return false
	}
	nonProxyTypes := map[string]bool{
		"selector": true,
		"urltest":  true,
		"direct":   true,
		"block":    true,
		"dns":      true,
	}
	return !nonProxyTypes[typeVal]
}

// fetchSubscriptionJSON downloads content from subscription URL.
func fetchSubscriptionJSON(url string, allowInsecure bool) ([]byte, error) {
	return fetchSubscriptionJSONWithTimeout(url, allowInsecure, 30*time.Second)
}

func fetchSubscriptionJSONWithTimeout(url string, allowInsecure bool, timeout time.Duration) ([]byte, error) {
	client := &http.Client{
		Timeout: timeout,
	}

	if allowInsecure {
		client.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
	}

	resp, err := client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch subscription: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("subscription returned status code: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read subscription body: %v", err)
	}

	return body, nil
}

// extractSubscriptionJSONOutboundsRaw returns all tagged outbounds from a JSON subscription.
// It intentionally keeps source payload semantics untouched (no type filtering or merging).
func extractSubscriptionJSONOutboundsRaw(jsonData []byte) ([]map[string]interface{}, error) {
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

	outbounds := make([]map[string]interface{}, 0, len(outboundsArr))
	for _, item := range outboundsArr {
		outbound, ok := item.(map[string]interface{})
		if !ok || outbound == nil {
			continue
		}

		tag, _ := outbound["tag"].(string)
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		outbounds = append(outbounds, cloneMap(outbound))
	}

	return outbounds, nil
}

// extractSubscriptionJSONOutboundRawByTag returns exact raw JSON payload of each tagged outbound.
func extractSubscriptionJSONOutboundRawByTag(jsonData []byte) (map[string]json.RawMessage, error) {
	var doc struct {
		Outbounds []json.RawMessage `json:"outbounds"`
	}
	if err := json.Unmarshal(jsonData, &doc); err != nil {
		return nil, fmt.Errorf("failed to parse subscription JSON: %v", err)
	}

	rawByTag := make(map[string]json.RawMessage, len(doc.Outbounds))
	for _, raw := range doc.Outbounds {
		if len(raw) == 0 {
			continue
		}

		var outbound map[string]interface{}
		if err := json.Unmarshal(raw, &outbound); err != nil {
			continue
		}

		tag, _ := outbound["tag"].(string)
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		if _, exists := rawByTag[tag]; exists {
			continue
		}
		rawByTag[tag] = append(json.RawMessage(nil), raw...)
	}

	return rawByTag, nil
}

// extractProxyOutbounds parses sing-box JSON subscription and returns proxy outbounds only.
func extractProxyOutbounds(jsonData []byte) ([]map[string]interface{}, error) {
	return extractProxyOutboundsWithTLSStore(jsonData, true)
}

func extractProxyOutboundsWithoutTLSStore(jsonData []byte) ([]map[string]interface{}, error) {
	return extractProxyOutboundsWithTLSStore(jsonData, false)
}

func extractProxyOutboundsWithTLSStore(jsonData []byte, injectTLSStore bool) ([]map[string]interface{}, error) {
	var config map[string]interface{}
	if err := json.Unmarshal(jsonData, &config); err != nil {
		return nil, fmt.Errorf("failed to parse subscription JSON: %v", err)
	}
	certStore := extractCertificateStore(config)

	outboundsRaw, ok := config["outbounds"]
	if !ok {
		return nil, fmt.Errorf("subscription JSON does not contain outbounds")
	}

	outboundsArr, ok := outboundsRaw.([]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid outbounds format")
	}

	shadowTLSByTag := make(map[string]map[string]interface{})
	shadowTLSTagsForMerge := make(map[string]bool)
	for _, ob := range outboundsArr {
		outbound, ok := ob.(map[string]interface{})
		if !ok {
			continue
		}
		if outboundType, _ := outbound["type"].(string); outboundType == "shadowtls" {
			tag, _ := outbound["tag"].(string)
			if tag != "" {
				shadowTLSByTag[tag] = outbound
			}
		}
	}
	for _, ob := range outboundsArr {
		outbound, ok := ob.(map[string]interface{})
		if !ok {
			continue
		}
		if outboundType, _ := outbound["type"].(string); outboundType != "shadowsocks" {
			continue
		}
		detour, _ := outbound["detour"].(string)
		if detour == "" {
			continue
		}
		if _, ok := shadowTLSByTag[detour]; ok {
			shadowTLSTagsForMerge[detour] = true
		}
	}

	proxyOutbounds := make([]map[string]interface{}, 0, len(outboundsArr))
	for _, ob := range outboundsArr {
		outbound, ok := ob.(map[string]interface{})
		if !ok {
			continue
		}

		outboundType, _ := outbound["type"].(string)
		if outboundType == "shadowsocks" {
			detour, _ := outbound["detour"].(string)
			if detour != "" {
				if stlsOutbound, exists := shadowTLSByTag[detour]; exists {
					combined := mergeImportedShadowTLSOutbound(outbound, stlsOutbound)
					normalizeImportedOutboundTLS(combined, certStore, injectTLSStore)
					proxyOutbounds = append(proxyOutbounds, combined)
					continue
				}
			}
		}
		if outboundType == "shadowtls" {
			tag, _ := outbound["tag"].(string)
			if tag != "" && shadowTLSTagsForMerge[tag] {
				continue
			}
		}

		if isProxyOutbound(outbound) {
			normalizeImportedOutboundTLS(outbound, certStore, injectTLSStore)
			proxyOutbounds = append(proxyOutbounds, outbound)
		}
	}

	return proxyOutbounds, nil
}

// mergeImportedShadowTLSOutbound restores UI-editable shadowtls from runtime pair:
// shadowsocks(detour) + shadowtls(tag-out).
func mergeImportedShadowTLSOutbound(ssOutbound map[string]interface{}, stlsOutbound map[string]interface{}) map[string]interface{} {
	merged := make(map[string]interface{}, len(stlsOutbound)+1)
	for k, v := range stlsOutbound {
		merged[k] = v
	}

	if tag, _ := ssOutbound["tag"].(string); tag != "" {
		merged["tag"] = tag
	}

	ssConfig := map[string]interface{}{}
	for _, key := range []string{"method", "network", "password", "udp_over_tcp", "multiplex"} {
		if value, ok := ssOutbound[key]; ok && value != nil {
			ssConfig[key] = value
		}
	}
	if len(ssConfig) > 0 {
		merged["ss_config"] = ssConfig
	}

	return merged
}

func extractCertificateStore(config map[string]interface{}) string {
	certRaw, ok := config["certificate"]
	if !ok {
		return ""
	}
	certMap, ok := certRaw.(map[string]interface{})
	if !ok || certMap == nil {
		return ""
	}
	return normalizeCertificateStoreValue(certMap["store"])
}

func normalizeImportedOutboundTLS(outbound map[string]interface{}, certStore string, injectTLSStore bool) {
	tlsRaw, ok := outbound["tls"]
	if !ok {
		return
	}

	tlsMap, ok := tlsRaw.(map[string]interface{})
	if !ok || tlsMap == nil {
		return
	}

	if fp, ok := tlsMap["client_fingerprint"].(string); ok && fp != "" {
		if _, hasUTLS := tlsMap["utls"]; !hasUTLS {
			tlsMap["utls"] = map[string]interface{}{
				"enabled":     true,
				"fingerprint": fp,
			}
		}
	}
	if _, hasMin := tlsMap["min_version"]; !hasMin {
		if v, ok := tlsMap["minVersion"]; ok {
			tlsMap["min_version"] = v
		}
	}
	if _, hasMax := tlsMap["max_version"]; !hasMax {
		if v, ok := tlsMap["maxVersion"]; ok {
			tlsMap["max_version"] = v
		}
	}

	if injectTLSStore {
		if _, hasTLSStore := tlsMap["tls_store"]; !hasTLSStore {
			store := normalizeCertificateStoreValue(tlsMap["store"])
			if store == "" {
				store = certStore
			}
			if store != "" {
				tlsMap["tls_store"] = store
			}
		}
		return
	}

	delete(tlsMap, "tls_store")
	delete(tlsMap, "store")
	if len(tlsMap) == 0 {
		delete(outbound, "tls")
	}
}

func getSubJsonDir() (string, error) {
	return filepath.Join(config.GetDataDir(), "sub_json"), nil
}

func getSubManagerDir() (string, error) {
	return filepath.Join(config.GetDataDir(), "sub_manager"), nil
}

func (s *SubGroupService) replaceSubscriptionGroupNodesTx(
	groupName string,
	nodes []subscriptionImportNode,
	jsonURL string,
	clashURL string,
	allowInsecure bool,
	clearFailure bool,
) (*SubscriptionRefreshResult, error) {
	db := database.GetDB()
	tx := db.Begin()
	if tx.Error != nil {
		return nil, tx.Error
	}
	BeginManagedRuntimeHookScope(tx)

	group := &model.SubGroup{}
	if err := tx.Where("name = ?", groupName).First(group).Error; err != nil {
		DiscardManagedRuntimeHookScope(tx)
		tx.Rollback()
		return nil, err
	}

	result, err := s.replaceSubscriptionGroupNodes(tx, group, nodes, jsonURL, clashURL, allowInsecure)
	if err != nil {
		DiscardManagedRuntimeHookScope(tx)
		tx.Rollback()
		return nil, err
	}

	if clearFailure {
		if err := s.clearSubGroupAutoUpdateFailure(tx, group.Id); err != nil {
			DiscardManagedRuntimeHookScope(tx)
			tx.Rollback()
			return nil, err
		}
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

// FetchAndSaveSubscription keeps backward compatibility for JSON-only calls.
func (s *SubGroupService) FetchAndSaveSubscription(groupName string, url string, allowInsecure bool) error {
	return s.FetchAndSaveSubscriptionSources(groupName, strings.TrimSpace(url), "", allowInsecure)
}

func (s *SubGroupService) FetchAndSaveSubscriptionSources(groupName string, jsonURL string, clashURL string, allowInsecure bool) error {
	subGroupSubscriptionUpdateMu.Lock()
	defer subGroupSubscriptionUpdateMu.Unlock()

	groupName = strings.TrimSpace(groupName)
	jsonURL = strings.TrimSpace(jsonURL)
	clashURL = strings.TrimSpace(clashURL)

	logger.Infof("[SubGroup] start fetch subscription: group=%s json=%s clash=%s", groupName, jsonURL, clashURL)

	nodes, err := s.loadSubscriptionImportNodes(jsonURL, clashURL, allowInsecure)
	if err != nil {
		return err
	}
	if len(nodes) == 0 {
		return fmt.Errorf("subscription contains no valid nodes")
	}

	if _, err := s.replaceSubscriptionGroupNodesTx(groupName, nodes, jsonURL, clashURL, allowInsecure, true); err != nil {
		return err
	}
	logger.Infof("[SubGroup] fetch subscription done: group=%s, saved=%d", groupName, len(nodes))
	return nil
}

// RefreshSubscription keeps backward compatibility for JSON-only calls.
func (s *SubGroupService) RefreshSubscription(groupName string, url string, allowInsecure bool) (*SubscriptionRefreshResult, error) {
	return s.RefreshSubscriptionSources(groupName, strings.TrimSpace(url), "", allowInsecure)
}

func (s *SubGroupService) RefreshSubscriptionSources(groupName string, jsonURL string, clashURL string, allowInsecure bool) (*SubscriptionRefreshResult, error) {
	subGroupSubscriptionUpdateMu.Lock()
	defer subGroupSubscriptionUpdateMu.Unlock()

	groupName = strings.TrimSpace(groupName)
	jsonURL = strings.TrimSpace(jsonURL)
	clashURL = strings.TrimSpace(clashURL)

	logger.Infof("[SubGroup] start refresh subscription: group=%s json=%s clash=%s", groupName, jsonURL, clashURL)

	result, err := s.refreshSubscriptionSourcesWithTimeout(groupName, jsonURL, clashURL, allowInsecure, true, 30*time.Second)
	if err != nil {
		return nil, err
	}
	logger.Infof(
		"[SubGroup] refresh subscription done: added=%d removed=%d updated=%d",
		len(result.Added),
		len(result.Removed),
		len(result.Updated),
	)
	return result, nil
}

func (s *SubGroupService) loadSubscriptionImportNodes(jsonURL string, clashURL string, allowInsecure bool) ([]subscriptionImportNode, error) {
	return s.loadSubscriptionImportNodesWithTimeout(jsonURL, clashURL, allowInsecure, 30*time.Second)
}

func (s *SubGroupService) loadSubscriptionImportNodesWithTimeout(jsonURL string, clashURL string, allowInsecure bool, timeout time.Duration) ([]subscriptionImportNode, error) {
	jsonURL = strings.TrimSpace(jsonURL)
	clashURL = strings.TrimSpace(clashURL)
	if jsonURL == "" && clashURL == "" {
		return nil, fmt.Errorf("at least one subscription url is required")
	}

	type jsonResult struct {
		outbounds []map[string]interface{}
		rawByTag  map[string]json.RawMessage
		err       error
	}
	type clashResult struct {
		proxies  []map[string]interface{}
		rawByTag map[string][]byte
		err      error
	}

	var (
		jsonRes  = jsonResult{outbounds: []map[string]interface{}{}, rawByTag: map[string]json.RawMessage{}}
		clashRes = clashResult{proxies: []map[string]interface{}{}, rawByTag: map[string][]byte{}}
		wg       sync.WaitGroup
	)

	if jsonURL != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			jsonData, err := fetchSubscriptionJSONWithTimeout(jsonURL, allowInsecure, timeout)
			if err != nil {
				jsonRes.err = err
				return
			}
			parsed, err := extractSubscriptionJSONOutboundsRaw(jsonData)
			if err != nil {
				jsonRes.err = err
				return
			}
			rawByTag, err := extractSubscriptionJSONOutboundRawByTag(jsonData)
			if err != nil {
				jsonRes.err = err
				return
			}
			jsonRes.outbounds = parsed
			jsonRes.rawByTag = rawByTag
		}()
	}

	if clashURL != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			clashData, err := fetchSubscriptionJSONWithTimeout(clashURL, allowInsecure, timeout)
			if err != nil {
				clashRes.err = err
				return
			}
			parsed, err := extractClashProxies(clashData)
			if err != nil {
				clashRes.err = err
				return
			}
			rawByTag, err := extractClashProxyRawYAMLByName(clashData)
			if err != nil {
				clashRes.err = err
				return
			}
			clashRes.proxies = parsed
			clashRes.rawByTag = rawByTag
		}()
	}

	wg.Wait()

	if jsonURL != "" && clashURL != "" {
		if jsonRes.err != nil && clashRes.err != nil {
			return nil, fmt.Errorf("json subscription failed: %v; clash subscription failed: %v", jsonRes.err, clashRes.err)
		}
	} else if jsonURL != "" {
		if jsonRes.err != nil {
			return nil, jsonRes.err
		}
	} else if clashURL != "" {
		if clashRes.err != nil {
			return nil, clashRes.err
		}
	}

	nodes, err := buildSubscriptionImportNodes(jsonRes.outbounds, clashRes.proxies)
	if err != nil {
		return nil, err
	}
	nodes = attachSubscriptionJSONRawByTag(nodes, jsonRes.rawByTag)
	nodes = attachSubscriptionClashRawYAMLByTag(nodes, clashRes.rawByTag)
	return nodes, nil
}

func (s *SubGroupService) refreshSubscriptionSourcesWithTimeout(
	groupName string,
	jsonURL string,
	clashURL string,
	allowInsecure bool,
	clearFailure bool,
	timeout time.Duration,
) (*SubscriptionRefreshResult, error) {
	nodes, err := s.loadSubscriptionImportNodesWithTimeout(jsonURL, clashURL, allowInsecure, timeout)
	if err != nil {
		return nil, err
	}
	if len(nodes) == 0 {
		return nil, fmt.Errorf("subscription contains no valid nodes")
	}
	return s.replaceSubscriptionGroupNodesTx(groupName, nodes, jsonURL, clashURL, allowInsecure, clearFailure)
}

func attachSubscriptionJSONRawByTag(nodes []subscriptionImportNode, rawByTag map[string]json.RawMessage) []subscriptionImportNode {
	if len(nodes) == 0 || len(rawByTag) == 0 {
		return nodes
	}
	for i := range nodes {
		tag := strings.TrimSpace(nodes[i].Tag)
		if tag == "" {
			continue
		}
		if raw, exists := rawByTag[tag]; exists {
			if !shouldAttachSubscriptionJSONRaw(nodes[i], raw, tag) {
				continue
			}
			nodes[i].JSONRaw = append(json.RawMessage(nil), raw...)
		}
	}
	return nodes
}

func attachSubscriptionClashRawYAMLByTag(nodes []subscriptionImportNode, rawByTag map[string][]byte) []subscriptionImportNode {
	if len(nodes) == 0 || len(rawByTag) == 0 {
		return nodes
	}
	for index := range nodes {
		tag := strings.TrimSpace(nodes[index].Tag)
		if tag == "" {
			continue
		}
		raw, exists := rawByTag[tag]
		if !exists || len(raw) == 0 {
			continue
		}
		nodes[index].ClashRawYAML = cloneRawYAMLBytes(raw)
	}
	return nodes
}

func shouldAttachSubscriptionJSONRaw(node subscriptionImportNode, raw json.RawMessage, expectedTag string) bool {
	if len(raw) == 0 {
		return false
	}

	var rawOutbound map[string]interface{}
	if err := json.Unmarshal(raw, &rawOutbound); err != nil || rawOutbound == nil {
		return false
	}

	rawTag, _ := rawOutbound["tag"].(string)
	if strings.TrimSpace(rawTag) != expectedTag {
		return false
	}
	if !isProxyOutbound(rawOutbound) {
		return false
	}

	if node.JSONOutbound == nil {
		return true
	}
	rawType, _ := rawOutbound["type"].(string)
	nodeType, _ := node.JSONOutbound["type"].(string)
	rawType = strings.TrimSpace(rawType)
	nodeType = strings.TrimSpace(nodeType)
	if rawType == "" || nodeType == "" {
		return true
	}
	return rawType == nodeType
}

func buildSubscriptionImportNodes(jsonOutbounds []map[string]interface{}, clashProxies []map[string]interface{}) ([]subscriptionImportNode, error) {
	jsonProxyOutbounds := make([]map[string]interface{}, 0, len(jsonOutbounds))
	for _, outbound := range jsonOutbounds {
		if outbound == nil || !isProxyOutbound(outbound) {
			continue
		}
		tag, _ := outbound["tag"].(string)
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		normalized := cloneMap(outbound)
		normalized["tag"] = tag
		jsonProxyOutbounds = append(jsonProxyOutbounds, normalized)
	}

	nodes := mergeImportedSubscriptionNodes(jsonProxyOutbounds, clashProxies)
	filtered := make([]subscriptionImportNode, 0, len(nodes))
	for _, node := range nodes {
		tag := strings.TrimSpace(node.Tag)
		if tag == "" || node.JSONOutbound == nil {
			continue
		}
		node.Tag = tag
		if node.ClashProxy != nil {
			node.ClashProxy["name"] = tag
		}
		if !isProxyOutbound(node.JSONOutbound) {
			continue
		}
		node.JSONOutbound["tag"] = tag
		filtered = append(filtered, node)
	}

	if len(filtered) == 0 {
		return nil, fmt.Errorf("subscription contains no valid nodes")
	}
	return filtered, nil
}

func (s *SubGroupService) replaceSubscriptionGroupNodes(
	db *gorm.DB,
	group *model.SubGroup,
	nodes []subscriptionImportNode,
	jsonURL string,
	clashURL string,
	allowInsecure bool,
) (*SubscriptionRefreshResult, error) {
	if group == nil {
		return nil, fmt.Errorf("group is nil")
	}
	if len(nodes) == 0 {
		return nil, fmt.Errorf("subscription contains no valid nodes")
	}

	cleanupTags, err := s.collectSubscriptionGroupCleanupTags(db, group)
	if err != nil {
		return nil, err
	}
	if err := s.deleteSubOutboundsByTags(db, cleanupTags); err != nil {
		return nil, err
	}

	savedTags, jsonOutbounds, err := s.persistSubscriptionImportNodes(db, nodes, group.Id)
	if err != nil {
		return nil, err
	}
	if len(savedTags) == 0 {
		return nil, fmt.Errorf("no valid nodes were saved")
	}

	result := buildSubscriptionRefreshResult(cleanupTags, savedTags)
	if err := s.syncSubscriptionGroupConfig(db, group.Name, jsonURL, clashURL, allowInsecure, jsonOutbounds); err != nil {
		return nil, err
	}
	if err := s.updateSubGroupSubscriptionSources(db, group.Name, savedTags, jsonURL, clashURL, allowInsecure); err != nil {
		return nil, err
	}
	if err := s.removeOutboundTagsFromGroups(db, result.Removed); err != nil {
		logger.Warningf("[SubGroup] remove stale subgroup tag references failed: %v", err)
	}

	tagsJSON, _ := json.Marshal(savedTags)
	group.Outbounds = string(tagsJSON)
	group.SubscriptionUrl = jsonURL
	group.SubscriptionUrlClash = clashURL
	group.AllowInsecure = allowInsecure
	return result, nil
}

func (s *SubGroupService) persistSubscriptionImportNodes(
	db *gorm.DB,
	nodes []subscriptionImportNode,
	sourceGroupID uint,
) ([]string, []map[string]interface{}, error) {
	savedTags := make([]string, 0, len(nodes))
	jsonOutbounds := make([]map[string]interface{}, 0, len(nodes))
	seen := make(map[string]struct{}, len(nodes))

	var subOutboundService SubOutboundService
	for _, node := range nodes {
		tag := strings.TrimSpace(node.Tag)
		if tag == "" || node.JSONOutbound == nil {
			continue
		}
		if _, exists := seen[tag]; exists {
			continue
		}

		node.JSONOutbound["tag"] = tag
		outboundBytes, err := json.Marshal(node.JSONOutbound)
		if err != nil {
			logger.Errorf("[SubGroup] marshal outbound failed [%s]: %v", tag, err)
			continue
		}

		var subOutbound model.SubOutbound
		if err := subOutbound.UnmarshalJSON(outboundBytes); err != nil {
			logger.Errorf("[SubGroup] parse outbound failed [%s]: %v", tag, err)
			continue
		}
		if len(node.JSONRaw) > 0 {
			subOutbound.RawOutbound = append(json.RawMessage(nil), node.JSONRaw...)
		} else {
			subOutbound.RawOutbound = append(json.RawMessage(nil), outboundBytes...)
		}
		subOutbound.Tag = tag
		subOutbound.SourceType = subOutboundSourceSubGroup
		subOutbound.SourceClientId = sourceGroupID
		subOutbound.SourceInboundId = 0
		if node.ClashProxy != nil {
			if rawClash, marshalErr := json.MarshalIndent(node.ClashProxy, "", "  "); marshalErr == nil {
				subOutbound.ClashOptions = rawClash
			}
		}
		subOutbound.RawClashYAML = cloneRawYAMLBytes(node.ClashRawYAML)

		var existing model.SubOutbound
		err = db.Where("tag = ?", tag).First(&existing).Error
		if err == nil {
			if existing.SourceType != subOutboundSourceSubGroup {
				if existing.SourceType == "" {
					return nil, nil, fmt.Errorf("tag %s already exists outside subscription group management", tag)
				}
				return nil, nil, fmt.Errorf("tag %s is managed by source type %s", tag, existing.SourceType)
			}
			if existing.SourceClientId != 0 && existing.SourceClientId != sourceGroupID {
				return nil, nil, fmt.Errorf("tag %s is managed by subscription group %d", tag, existing.SourceClientId)
			}
			if err := db.Where("id = ?", existing.Id).Delete(&model.SubOutbound{}).Error; err != nil {
				return nil, nil, err
			}
			if oldTag := strings.TrimSpace(existing.Tag); oldTag != "" {
				if err := subOutboundService.removeManagedArtifacts(db, oldTag); err != nil {
					return nil, nil, err
				}
			}
		} else if !database.IsNotFound(err) {
			return nil, nil, err
		}

		if err := db.Create(&subOutbound).Error; err != nil {
			logger.Errorf("[SubGroup] save outbound failed [%s]: %v", tag, err)
			continue
		}
		if err := validateSubOutboundSubJSONFileName(db, &subOutbound); err != nil {
			return nil, nil, err
		}

		if err := db.Where("tag = ?", tag).First(&subOutbound).Error; err == nil {
			if err := subOutboundService.syncManagedArtifacts(db, &subOutbound); err != nil {
				return nil, nil, err
			}
		}

		seen[tag] = struct{}{}
		savedTags = append(savedTags, tag)
		snapshot := cloneMap(node.JSONOutbound)
		if len(subOutbound.RawOutbound) > 0 {
			rawMap := map[string]interface{}{}
			if err := json.Unmarshal(subOutbound.RawOutbound, &rawMap); err == nil && len(rawMap) > 0 {
				delete(rawMap, "id")
				if rawTag, _ := rawMap["tag"].(string); strings.TrimSpace(rawTag) == "" {
					rawMap["tag"] = tag
				}
				snapshot = rawMap
			}
		}
		jsonOutbounds = append(jsonOutbounds, snapshot)
	}

	return savedTags, jsonOutbounds, nil
}

func (s *SubGroupService) saveSubGroupSubscriptionJson(
	groupName string,
	jsonURL string,
	clashURL string,
	allowInsecure bool,
	jsonOutbounds []map[string]interface{},
) error {
	effectiveDB := database.GetDB()
	group := &model.SubGroup{Name: groupName}
	if effectiveDB != nil {
		loadErr := effectiveDB.Model(model.SubGroup{}).
			Select("id", "name").
			Where("name = ?", groupName).
			First(group).Error
		if loadErr != nil && !database.IsNotFound(loadErr) {
			return loadErr
		}
	}
	if err := validateSubGroupSubJSONFileName(effectiveDB, group); err != nil {
		return err
	}

	subJsonDir, err := getSubJsonDir()
	if err != nil {
		return err
	}
	baseFilename := sanitizeGroupFilename(groupName)
	filePath := filepath.Join(subJsonDir, fmt.Sprintf("%s.json", baseFilename))

	var settingService SettingService
	othersStr, _ := settingService.GetSubJsonExt()
	content, err := renderManagedSingboxSubscriptionJSON(
		jsonOutbounds,
		othersStr,
		settingService.ResolveSubscriptionTLSStore,
	)
	if err != nil {
		return fmt.Errorf("failed to render group config: %v", err)
	}
	if err := ManagedRuntimeWriteFile(filePath, content); err != nil {
		return fmt.Errorf("failed to write group config file: %v", err)
	}

	return nil
}

func (s *SubGroupService) updateSubGroupSubscriptionSources(
	db *gorm.DB,
	groupName string,
	tags []string,
	jsonURL string,
	clashURL string,
	allowInsecure bool,
) error {
	tagsJSON, _ := json.Marshal(tags)
	updates := map[string]interface{}{
		"outbounds":              string(tagsJSON),
		"subscription_url":       jsonURL,
		"subscription_url_clash": clashURL,
		"allow_insecure":         allowInsecure,
	}
	return db.Model(&model.SubGroup{}).Where("name = ?", groupName).Updates(updates).Error
}

func parseSubGroupOutboundTags(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return []string{}
	}
	var tags []string
	if err := json.Unmarshal([]byte(raw), &tags); err != nil {
		return []string{}
	}
	result := make([]string, 0, len(tags))
	seen := make(map[string]struct{}, len(tags))
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		if _, exists := seen[tag]; exists {
			continue
		}
		seen[tag] = struct{}{}
		result = append(result, tag)
	}
	return result
}

func normalizeUniqueTags(tags []string) []string {
	result := make([]string, 0, len(tags))
	seen := make(map[string]struct{}, len(tags))
	for _, tag := range tags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		if _, exists := seen[tag]; exists {
			continue
		}
		seen[tag] = struct{}{}
		result = append(result, tag)
	}
	return result
}

func buildSubscriptionRefreshResult(oldTags []string, newTags []string) *SubscriptionRefreshResult {
	result := &SubscriptionRefreshResult{
		Added:   []string{},
		Removed: []string{},
		Updated: []string{},
	}

	oldTags = normalizeUniqueTags(oldTags)
	newTags = normalizeUniqueTags(newTags)

	oldSet := make(map[string]struct{}, len(oldTags))
	for _, tag := range oldTags {
		oldSet[tag] = struct{}{}
	}

	newSet := make(map[string]struct{}, len(newTags))
	for _, tag := range newTags {
		newSet[tag] = struct{}{}
		if _, exists := oldSet[tag]; exists {
			result.Updated = append(result.Updated, tag)
		} else {
			result.Added = append(result.Added, tag)
		}
	}

	for _, tag := range oldTags {
		if _, exists := newSet[tag]; !exists {
			result.Removed = append(result.Removed, tag)
		}
	}

	return result
}

func (s *SubGroupService) deleteSubOutboundsByTags(db *gorm.DB, tags []string) error {
	cleanedTags := normalizeUniqueTags(tags)
	if len(cleanedTags) == 0 {
		return nil
	}

	var subOutboundService SubOutboundService
	if err := db.Where("tag IN ?", cleanedTags).Delete(&model.SubOutbound{}).Error; err != nil {
		return err
	}

	for _, tag := range cleanedTags {
		if err := subOutboundService.removeManagedArtifacts(db, tag); err != nil {
			return err
		}
	}
	return nil
}

func (s *SubGroupService) collectSubscriptionGroupCleanupTags(db *gorm.DB, group *model.SubGroup) ([]string, error) {
	if group == nil {
		return nil, fmt.Errorf("group is nil")
	}
	tags := parseSubGroupOutboundTags(group.Outbounds)
	if group.Id == 0 {
		return normalizeUniqueTags(tags), nil
	}

	var managedTags []string
	if err := db.Model(model.SubOutbound{}).
		Where("source_type = ? AND source_client_id = ?", subOutboundSourceSubGroup, group.Id).
		Pluck("tag", &managedTags).Error; err != nil {
		return nil, err
	}
	tags = append(tags, managedTags...)
	return normalizeUniqueTags(tags), nil
}

func (s *SubGroupService) removeOutboundTagsFromGroups(db *gorm.DB, tags []string) error {
	cleanedTags := normalizeUniqueTags(tags)
	if len(cleanedTags) == 0 {
		return nil
	}
	removeSet := make(map[string]struct{}, len(cleanedTags))
	for _, tag := range cleanedTags {
		removeSet[tag] = struct{}{}
	}

	var groups []model.SubGroup
	if err := db.Model(model.SubGroup{}).Find(&groups).Error; err != nil {
		return err
	}

	for _, group := range groups {
		groupTags := parseSubGroupOutboundTags(group.Outbounds)
		if len(groupTags) == 0 {
			continue
		}

		nextTags := make([]string, 0, len(groupTags))
		changed := false
		for _, tag := range groupTags {
			if _, removed := removeSet[tag]; removed {
				changed = true
				continue
			}
			nextTags = append(nextTags, tag)
		}
		if !changed {
			continue
		}

		tagsJSON, err := json.Marshal(nextTags)
		if err != nil {
			return err
		}
		group.Outbounds = string(tagsJSON)
		if err := db.Model(&model.SubGroup{}).Where("id = ?", group.Id).Update("outbounds", group.Outbounds).Error; err != nil {
			return err
		}
		groupCopy := group
		if err := s.syncGroupConfig(db, &groupCopy); err != nil {
			return err
		}
	}
	return nil
}

func (s *SubGroupService) pruneMissingOutboundTags(db *gorm.DB) (int, error) {
	if db == nil {
		return 0, fmt.Errorf("database is nil")
	}

	var groups []model.SubGroup
	if err := db.Model(model.SubGroup{}).Find(&groups).Error; err != nil {
		return 0, err
	}
	if len(groups) == 0 {
		return 0, nil
	}

	var existingTags []string
	if err := db.Model(model.SubOutbound{}).Pluck("tag", &existingTags).Error; err != nil {
		return 0, err
	}
	existingSet := make(map[string]struct{}, len(existingTags))
	for _, tag := range existingTags {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		existingSet[tag] = struct{}{}
	}

	prunedCount := 0
	for _, group := range groups {
		groupTags := parseSubGroupOutboundTags(group.Outbounds)
		if len(groupTags) == 0 {
			continue
		}

		nextTags := make([]string, 0, len(groupTags))
		changed := false
		for _, tag := range groupTags {
			if _, exists := existingSet[tag]; !exists {
				changed = true
				continue
			}
			nextTags = append(nextTags, tag)
		}
		if !changed {
			continue
		}

		tagsJSON, err := json.Marshal(nextTags)
		if err != nil {
			return prunedCount, err
		}
		group.Outbounds = string(tagsJSON)
		if err := db.Model(&model.SubGroup{}).Where("id = ?", group.Id).Update("outbounds", group.Outbounds).Error; err != nil {
			return prunedCount, err
		}
		groupCopy := group
		if err := s.syncGroupConfig(db, &groupCopy); err != nil {
			return prunedCount, err
		}
		prunedCount++
	}

	if prunedCount > 0 {
		LastUpdate = time.Now().Unix()
	}

	return prunedCount, nil
}

func (s *SubGroupService) replaceOutboundTagInGroups(db *gorm.DB, oldTag string, newTag string) error {
	oldTag = strings.TrimSpace(oldTag)
	newTag = strings.TrimSpace(newTag)
	if oldTag == "" || newTag == "" || oldTag == newTag {
		return nil
	}

	var groups []model.SubGroup
	if err := db.Model(model.SubGroup{}).Find(&groups).Error; err != nil {
		return err
	}

	for _, group := range groups {
		groupTags := parseSubGroupOutboundTags(group.Outbounds)
		if len(groupTags) == 0 {
			continue
		}

		nextTags := make([]string, 0, len(groupTags))
		seen := make(map[string]struct{}, len(groupTags))
		changed := false
		for _, tag := range groupTags {
			if tag == oldTag {
				tag = newTag
				changed = true
			}
			tag = strings.TrimSpace(tag)
			if tag == "" {
				continue
			}
			if _, exists := seen[tag]; exists {
				continue
			}
			seen[tag] = struct{}{}
			nextTags = append(nextTags, tag)
		}
		if !changed {
			continue
		}

		tagsJSON, err := json.Marshal(nextTags)
		if err != nil {
			return err
		}
		group.Outbounds = string(tagsJSON)
		if err := db.Model(&model.SubGroup{}).Where("id = ?", group.Id).Update("outbounds", group.Outbounds).Error; err != nil {
			return err
		}
		groupCopy := group
		if err := s.syncGroupConfig(db, &groupCopy); err != nil {
			return err
		}
	}
	return nil
}

type SubManagerClearResult struct {
	ClearedNodes  int `json:"cleared_nodes"`
	ClearedGroups int `json:"cleared_groups"`
}

func (s *SubGroupService) ClearSubManagerData() (*SubManagerClearResult, error) {
	db := database.GetDB()

	var tags []string
	if err := db.Model(model.SubOutbound{}).Pluck("tag", &tags).Error; err != nil {
		return nil, err
	}
	cleanedTags := normalizeUniqueTags(tags)

	tx := db.Begin()
	if tx.Error != nil {
		return nil, tx.Error
	}
	BeginManagedRuntimeHookScope(tx)

	groups := []*model.SubGroup{}
	if err := tx.Model(model.SubGroup{}).Find(&groups).Error; err != nil {
		DiscardManagedRuntimeHookScope(tx)
		tx.Rollback()
		return nil, err
	}

	managedOutbounds := make([]model.SubOutbound, 0)
	if err := tx.Model(model.SubOutbound{}).
		Select("source_type, source_client_id, source_inbound_id").
		Where("source_type IN ?", []string{subOutboundSourceClient, subOutboundSourceMihomoClient}).
		Find(&managedOutbounds).Error; err != nil {
		DiscardManagedRuntimeHookScope(tx)
		tx.Rollback()
		return nil, err
	}
	for i := range managedOutbounds {
		record := &managedOutbounds[i]
		if err := blockSubSyncInboundBySubOutbound(tx, record); err != nil {
			DiscardManagedRuntimeHookScope(tx)
			tx.Rollback()
			return nil, err
		}
	}

	if err := tx.Session(&gorm.Session{AllowGlobalUpdate: true}).Delete(&model.SubOutbound{}).Error; err != nil {
		DiscardManagedRuntimeHookScope(tx)
		tx.Rollback()
		return nil, err
	}
	if err := tx.Model(&model.SubGroup{}).Where("1 = 1").Update("outbounds", "[]").Error; err != nil {
		DiscardManagedRuntimeHookScope(tx)
		tx.Rollback()
		return nil, err
	}

	var subOutboundService SubOutboundService
	for _, tag := range cleanedTags {
		if err := subOutboundService.removeManagedArtifacts(tx, tag); err != nil {
			DiscardManagedRuntimeHookScope(tx)
			tx.Rollback()
			return nil, err
		}
	}

	for _, group := range groups {
		group.Outbounds = "[]"
		if err := s.syncGroupConfig(tx, group); err != nil {
			DiscardManagedRuntimeHookScope(tx)
			tx.Rollback()
			return nil, err
		}
	}

	if err := tx.Commit().Error; err != nil {
		DiscardManagedRuntimeHookScope(tx)
		tx.Rollback()
		return nil, err
	}

	if err := RunManagedRuntimeHookScope(tx); err != nil {
		return nil, err
	}

	LastUpdate = time.Now().Unix()
	return &SubManagerClearResult{
		ClearedNodes:  len(cleanedTags),
		ClearedGroups: len(groups),
	}, nil
}
