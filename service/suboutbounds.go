package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/alireza0/s-ui/config"
	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/logger"
	"github.com/alireza0/s-ui/util"
	"github.com/alireza0/s-ui/util/common"

	"gorm.io/gorm"
)

// SubOutboundService handles subscription outbounds.
type SubOutboundService struct{}

// subJsonDefaultConfig is the default template used for subscription JSON generation.
const subJsonDefaultConfig = `{
  "inbounds": [
    {
      "type": "tun",
      "address": ["172.19.0.1/30", "fdfe:dcba:9876::1/126"],
      "mtu": 1500,
      "auto_route": true,
      "strict_route": true,
      "endpoint_independent_nat": false,
      "stack": "mixed",
      "exclude_package": []
    },
    {
      "type": "mixed",
      "listen": "127.0.0.1",
      "listen_port": 2080,
      "users": []
    }
  ]
}`

const (
	nodeSelectorTag         = "节点选择"
	autoSelectorTag         = "自动选择"
	globalDirectSelectorTag = "全球直连"
	globalBlockSelectorTag  = "全球拦截"
	finalSelectorTag        = "漏网之鱼"
	globalSelectorTag       = "GLOBAL"
	managedSubHTTPClientTag = "rules-download"
)

var legacySingboxSelectorTagAliases = map[string]string{
	"🚀 节点选择":           nodeSelectorTag,
	"🚀节点选择":            nodeSelectorTag,
	"\\U0001F680 节点选择": nodeSelectorTag,
	"\\U0001F680节点选择":  nodeSelectorTag,
	"🎈 自动选择":           autoSelectorTag,
	"🎈自动选择":            autoSelectorTag,
	"\\U0001F388 自动选择": autoSelectorTag,
	"\\U0001F388自动选择":  autoSelectorTag,
	"🎯 全球直连":           globalDirectSelectorTag,
	"🎯全球直连":            globalDirectSelectorTag,
	"\\U0001F3AF 全球直连": globalDirectSelectorTag,
	"\\U0001F3AF全球直连":  globalDirectSelectorTag,
	"🛑 全球拦截":           globalBlockSelectorTag,
	"🛑全球拦截":            globalBlockSelectorTag,
	"\\U0001F6D1 全球拦截": globalBlockSelectorTag,
	"\\U0001F6D1全球拦截":  globalBlockSelectorTag,
	"🐟 漏网之鱼":           finalSelectorTag,
	"🐟漏网之鱼":            finalSelectorTag,
	"\\U0001F41F 漏网之鱼": finalSelectorTag,
	"\\U0001F41F漏网之鱼":  finalSelectorTag,
}

func normalizeLegacySingboxSelectorTag(raw string) string {
	trimmed := strings.TrimSpace(raw)
	if normalized, exists := legacySingboxSelectorTagAliases[trimmed]; exists {
		return normalized
	}
	return trimmed
}

type selectorGroupConfig struct {
	Tag             string
	DefaultOutbound string
}

type subOutboundClientOrderRow struct {
	Id       uint
	Inbounds json.RawMessage
}

type subOutboundManagedSortContext struct {
	clientOrder       map[uint]int
	clientInboundRank map[uint]map[uint]int
	mihomoOrder       map[uint]int
	mihomoInboundRank map[uint]map[uint]int
}

func buildSubOutboundInboundRank(raw json.RawMessage) map[uint]int {
	if len(raw) == 0 || strings.TrimSpace(string(raw)) == "" {
		return map[uint]int{}
	}

	var inboundIDs []uint
	if err := json.Unmarshal(raw, &inboundIDs); err != nil {
		return map[uint]int{}
	}

	rank := make(map[uint]int, len(inboundIDs))
	for index, inboundID := range inboundIDs {
		if inboundID == 0 {
			continue
		}
		if _, exists := rank[inboundID]; exists {
			continue
		}
		rank[inboundID] = index
	}
	return rank
}

func loadManagedSubOutboundSortContext(db *gorm.DB) (*subOutboundManagedSortContext, error) {
	ctx := &subOutboundManagedSortContext{
		clientOrder:       make(map[uint]int),
		clientInboundRank: make(map[uint]map[uint]int),
		mihomoOrder:       make(map[uint]int),
		mihomoInboundRank: make(map[uint]map[uint]int),
	}

	var clients []subOutboundClientOrderRow
	if err := db.Model(model.Client{}).
		Select("id", "inbounds").
		Order("id ASC").
		Scan(&clients).Error; err != nil {
		return nil, err
	}
	for index, client := range clients {
		ctx.clientOrder[client.Id] = index
		ctx.clientInboundRank[client.Id] = buildSubOutboundInboundRank(client.Inbounds)
	}

	var mihomoClients []subOutboundClientOrderRow
	if err := db.Model(model.MihomoClient{}).
		Select("id", "inbounds").
		Order("id ASC").
		Scan(&mihomoClients).Error; err != nil {
		return nil, err
	}
	for index, client := range mihomoClients {
		ctx.mihomoOrder[client.Id] = index
		ctx.mihomoInboundRank[client.Id] = buildSubOutboundInboundRank(client.Inbounds)
	}

	return ctx, nil
}

func getSubOutboundInboundRank(ranks map[uint]map[uint]int, clientID uint, inboundID uint) (int, bool) {
	clientRanks, exists := ranks[clientID]
	if !exists {
		return 0, false
	}
	rank, exists := clientRanks[inboundID]
	return rank, exists
}

func sortManagedSubOutboundsBySourceOrder(
	items []*model.SubOutbound,
	sourceType string,
	clientOrder map[uint]int,
	inboundRank map[uint]map[uint]int,
) {
	indexes := make([]int, 0, len(items))
	subset := make([]*model.SubOutbound, 0, len(items))
	for index, item := range items {
		if item == nil || strings.TrimSpace(item.SourceType) != sourceType {
			continue
		}
		indexes = append(indexes, index)
		subset = append(subset, item)
	}
	if len(subset) <= 1 {
		return
	}

	sort.SliceStable(subset, func(i, j int) bool {
		left := subset[i]
		right := subset[j]

		leftInboundRank, leftInboundOK := getSubOutboundInboundRank(inboundRank, left.SourceClientId, left.SourceInboundId)
		rightInboundRank, rightInboundOK := getSubOutboundInboundRank(inboundRank, right.SourceClientId, right.SourceInboundId)
		if leftInboundOK != rightInboundOK {
			return leftInboundOK
		}
		if leftInboundOK && rightInboundOK && leftInboundRank != rightInboundRank {
			return leftInboundRank < rightInboundRank
		}

		leftClientOrder, leftClientOK := clientOrder[left.SourceClientId]
		rightClientOrder, rightClientOK := clientOrder[right.SourceClientId]
		if leftClientOK != rightClientOK {
			return leftClientOK
		}
		if leftClientOK && rightClientOK && leftClientOrder != rightClientOrder {
			return leftClientOrder < rightClientOrder
		}

		if left.Id != right.Id {
			return left.Id < right.Id
		}
		return left.Tag < right.Tag
	})

	for index, targetIndex := range indexes {
		items[targetIndex] = subset[index]
	}
}

func reorderManagedSubOutbounds(items []*model.SubOutbound, db *gorm.DB) ([]*model.SubOutbound, error) {
	if len(items) <= 1 {
		return items, nil
	}

	ctx, err := loadManagedSubOutboundSortContext(db)
	if err != nil {
		return nil, err
	}

	ordered := append([]*model.SubOutbound(nil), items...)
	sortManagedSubOutboundsBySourceOrder(
		ordered,
		subOutboundSourceClient,
		ctx.clientOrder,
		ctx.clientInboundRank,
	)
	sortManagedSubOutboundsBySourceOrder(
		ordered,
		subOutboundSourceMihomoClient,
		ctx.mihomoOrder,
		ctx.mihomoInboundRank,
	)
	return ordered, nil
}

// GetAll returns all subscription outbounds.
func (s *SubOutboundService) GetAll() (*[]map[string]interface{}, error) {
	db := database.GetDB()
	subOutbounds := []*model.SubOutbound{}
	err := db.Model(model.SubOutbound{}).Order("id ASC").Scan(&subOutbounds).Error
	if err != nil {
		return nil, err
	}
	subOutbounds, err = reorderManagedSubOutbounds(subOutbounds, db)
	if err != nil {
		return nil, err
	}
	var data []map[string]interface{}
	for _, subOutbound := range subOutbounds {
		outboundJSON, err := resolveSubOutboundJSON(subOutbound)
		if err != nil {
			return nil, err
		}
		outData := map[string]interface{}{}
		if err := json.Unmarshal(outboundJSON, &outData); err != nil {
			return nil, err
		}
		outData["id"] = subOutbound.Id
		data = append(data, outData)
	}
	return &data, nil
}

// GetAllConfig returns all subscription outbounds for sing-box config generation.
func (s *SubOutboundService) GetAllConfig(db *gorm.DB) ([]json.RawMessage, error) {
	var subOutboundsJson []json.RawMessage
	var subOutbounds []*model.SubOutbound
	err := db.Model(model.SubOutbound{}).Scan(&subOutbounds).Error
	if err != nil {
		return nil, err
	}
	for _, subOutbound := range subOutbounds {
		subOutboundJson, err := resolveSubOutboundJSON(subOutbound)
		if err != nil {
			return nil, err
		}

		// Comment cleaned to avoid mojibake.
		if subOutbound.Type == "shadowtls" {
			ssJson, shadowtlsJson, err := s.processShadowTLSOutbound(subOutboundJson, subOutbound)
			if err != nil {
				return nil, err
			}
			// shadowsocks outbound must be added before shadowtls outbound.
			if ssJson != nil {
				subOutboundsJson = append(subOutboundsJson, ssJson)
			}
			subOutboundsJson = append(subOutboundsJson, shadowtlsJson)
		} else {
			subOutboundsJson = append(subOutboundsJson, subOutboundJson)
		}
	}
	return subOutboundsJson, nil
}

// processShadowTLSOutbound builds combined shadowtls+shadowsocks outbounds when ss_config exists.
func (s *SubOutboundService) processShadowTLSOutbound(outboundJson []byte, outbound *model.SubOutbound) (json.RawMessage, json.RawMessage, error) {
	return util.BuildShadowTLSRuntimeOutboundPairJSON(outboundJson, false)
}

func normalizeSubOutboundRawPayload(data json.RawMessage) json.RawMessage {
	if len(data) == 0 {
		return nil
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(data, &payload); err != nil {
		return append(json.RawMessage(nil), data...)
	}

	delete(payload, "id")
	normalized, err := json.Marshal(payload)
	if err != nil {
		return append(json.RawMessage(nil), data...)
	}
	return normalized
}

func resolveSubOutboundJSON(subOutbound *model.SubOutbound) ([]byte, error) {
	if subOutbound == nil {
		return nil, fmt.Errorf("sub outbound is nil")
	}

	if len(subOutbound.RawOutbound) > 0 {
		var payload map[string]interface{}
		if err := json.Unmarshal(subOutbound.RawOutbound, &payload); err == nil && payload != nil {
			return append(json.RawMessage(nil), subOutbound.RawOutbound...), nil
		}
	}

	return subOutbound.MarshalJSON()
}

func (s *SubOutboundService) processShadowTLSOutboundLegacy(outboundJson []byte, outbound *model.SubOutbound) (json.RawMessage, json.RawMessage, error) {
	var outboundData map[string]interface{}
	if err := json.Unmarshal(outboundJson, &outboundData); err != nil {
		return nil, nil, err
	}

	ssConfig, hasSsConfig := outboundData["ss_config"].(map[string]interface{})
	if !hasSsConfig || ssConfig == nil {
		stripShadowTLSInboundOnlyFields(outboundData)
		sanitizedJson, err := json.Marshal(outboundData)
		if err != nil {
			return nil, nil, err
		}
		return nil, sanitizedJson, nil
	}

	// 删除 ss_config
	delete(outboundData, "ss_config")
	stripShadowTLSInboundOnlyFields(outboundData)

	tag, ok := outboundData["tag"].(string)
	if !ok || tag == "" {
		shadowtlsJson, err := json.Marshal(outboundData)
		if err != nil {
			return nil, nil, err
		}
		return nil, shadowtlsJson, nil
	}

	shadowtlsTag := tag + "-out"
	outboundData["tag"] = shadowtlsTag

	shadowtlsJson, err := json.Marshal(outboundData)
	if err != nil {
		return nil, nil, err
	}

	ssOutbound := map[string]interface{}{
		"type":   "shadowsocks",
		"tag":    tag,
		"detour": shadowtlsTag,
	}

	if method, ok := ssConfig["method"]; ok && method != nil {
		ssOutbound["method"] = method
	}
	if network, ok := ssConfig["network"]; ok && network != nil && network != "" {
		ssOutbound["network"] = network
	}
	if password, ok := ssConfig["password"]; ok && password != nil {
		ssOutbound["password"] = password
	}
	if udpOverTcp, ok := ssConfig["udp_over_tcp"]; ok && udpOverTcp != nil {
		ssOutbound["udp_over_tcp"] = udpOverTcp
	}

	if multiplex, ok := ssConfig["multiplex"].(map[string]interface{}); ok && multiplex != nil {
		if enabled, ok := multiplex["enabled"].(bool); ok && enabled {
			ssOutbound["multiplex"] = multiplex
		}
	}

	ssOutboundJson, err := json.Marshal(ssOutbound)
	if err != nil {
		return nil, nil, err
	}

	return ssOutboundJson, shadowtlsJson, nil
}

// Save persists a sub outbound and regenerates related files.
func (s *SubOutboundService) Save(tx *gorm.DB, act string, data json.RawMessage) error {
	var err error

	switch act {
	case "new", "edit":
		var subOutbound model.SubOutbound
		err = subOutbound.UnmarshalJSON(data)
		if err != nil {
			return err
		}
		incomingRaw := normalizeSubOutboundRawPayload(data)
		subOutbound.RawOutbound = incomingRaw

		// Preserve sync source metadata when editing through generic SubOutbound APIs.
		existing := &model.SubOutbound{}
		var lookupErr error
		oldTag := ""
		switch {
		case subOutbound.Id > 0:
			lookupErr = tx.Model(model.SubOutbound{}).Where("id = ?", subOutbound.Id).First(existing).Error
		case subOutbound.Tag != "":
			lookupErr = tx.Model(model.SubOutbound{}).Where("tag = ?", subOutbound.Tag).First(existing).Error
		}
		if lookupErr == nil {
			if subOutbound.SourceType == "" {
				subOutbound.SourceType = existing.SourceType
			}
			if subOutbound.SourceClientId == 0 {
				subOutbound.SourceClientId = existing.SourceClientId
			}
			if subOutbound.SourceInboundId == 0 {
				subOutbound.SourceInboundId = existing.SourceInboundId
			}
			if existing.Type == subOutbound.Type {
				if baseRaw, resolveErr := resolveSubOutboundJSON(existing); resolveErr == nil {
					subOutbound.RawOutbound = mergeEditableOutboundRawPayload(baseRaw, data, "default", subOutbound.Type)
				}
			}
			if len(subOutbound.ClashOptions) == 0 && existing.Type == subOutbound.Type {
				subOutbound.ClashOptions = buildMergedClashProxyOptions(subOutbound.RawOutbound, existing.ClashOptions, subOutbound.Tag)
			}
			if len(subOutbound.RawOutbound) == 0 {
				subOutbound.RawOutbound = append(json.RawMessage(nil), existing.RawOutbound...)
			}
			oldTag = strings.TrimSpace(existing.Tag)
		} else if lookupErr != nil && !errors.Is(lookupErr, gorm.ErrRecordNotFound) {
			return lookupErr
		}
		if len(subOutbound.RawOutbound) == 0 {
			subOutbound.RawOutbound = incomingRaw
		}
		subOutbound.RawClashYAML = nil
		subOutbound.ClashOptions = normalizeClashProxyOptionsTag(subOutbound.ClashOptions, subOutbound.Tag)
		if err := validateSubOutboundSubJSONFileName(tx, &subOutbound); err != nil {
			return err
		}

		err = tx.Save(&subOutbound).Error
		if err != nil {
			return err
		}

		var subGroupService SubGroupService
		if oldTag != "" && oldTag != strings.TrimSpace(subOutbound.Tag) {
			if err := subGroupService.replaceOutboundTagInGroups(tx, oldTag, subOutbound.Tag); err != nil {
				return err
			}
			if err := s.removeManagedArtifacts(tx, oldTag); err != nil {
				return err
			}
		}

		if err := s.syncManagedArtifacts(tx, &subOutbound); err != nil {
			return err
		}

	case "del":
		var tag string
		err = json.Unmarshal(data, &tag)
		if err != nil {
			return err
		}

		existing := &model.SubOutbound{}
		lookupErr := tx.Model(model.SubOutbound{}).Where("tag = ?", tag).First(existing).Error
		if lookupErr == nil {
			if blockErr := blockSubSyncInboundBySubOutbound(tx, existing); blockErr != nil {
				return blockErr
			}
		} else if lookupErr != nil && !errors.Is(lookupErr, gorm.ErrRecordNotFound) {
			return lookupErr
		}

		err = tx.Where("tag = ?", tag).Delete(model.SubOutbound{}).Error
		if err != nil {
			return err
		}
		var subGroupService SubGroupService
		if err := subGroupService.removeOutboundTagsFromGroups(tx, []string{tag}); err != nil {
			return err
		}
		if err := s.removeManagedArtifacts(tx, tag); err != nil {
			return err
		}
	default:
		return common.NewErrorf("unknown action: %s", act)
	}
	return nil
}

// SubOutboundMetadata describes the metadata sidecar for a saved sub outbound.
type SubOutboundMetadata struct {
	Id        uint   `json:"id"`
	Tag       string `json:"tag"`
	Type      string `json:"type"`
	UpdatedAt int64  `json:"updated_at"`
}

func cloneSubOutboundForArtifacts(src *model.SubOutbound) *model.SubOutbound {
	if src == nil {
		return nil
	}

	cloned := *src
	cloned.Options = append(json.RawMessage(nil), src.Options...)
	cloned.RawOutbound = append(json.RawMessage(nil), src.RawOutbound...)
	cloned.ClashOptions = append(json.RawMessage(nil), src.ClashOptions...)
	cloned.RawClashYAML = cloneRawYAMLBytes(src.RawClashYAML)
	return &cloned
}

func (s *SubOutboundService) syncManagedArtifacts(db *gorm.DB, subOutbound *model.SubOutbound) error {
	snapshot := cloneSubOutboundForArtifacts(subOutbound)
	if snapshot == nil {
		return nil
	}

	return QueueManagedRuntimeHook(db, func() error {
		if err := s.saveSubOutboundJson(snapshot); err != nil {
			return err
		}
		return s.generateSubJsonFile(snapshot)
	})
}

func (s *SubOutboundService) removeManagedArtifacts(db *gorm.DB, tag string) error {
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return nil
	}

	return QueueManagedRuntimeHook(db, func() error {
		if err := s.deleteSubOutboundJson(tag); err != nil {
			return err
		}
		return s.deleteSubJsonFile(tag)
	})
}

// saveSubOutboundJson writes outbound config and metadata into Promanager_data/sub_manager.
func (s *SubOutboundService) saveSubOutboundJson(subOutbound *model.SubOutbound) error {
	subManagerDir := filepath.Join(config.GetDataDir(), "sub_manager")

	outboundJson, err := resolveSubOutboundJSON(subOutbound)
	if err != nil {
		logger.Errorf("[SubOutbound] failed to marshal outbound: %v", err)
		return err
	}

	baseFilename := sanitizeSubFilename(subOutbound.Tag)
	configFilePath := filepath.Join(subManagerDir, fmt.Sprintf("%s.json", baseFilename))
	metaFilePath := filepath.Join(subManagerDir, fmt.Sprintf("%s_meta.json", baseFilename))

	var prettyJson interface{}
	if err := json.Unmarshal(outboundJson, &prettyJson); err != nil {
		logger.Errorf("[SubOutbound] failed to parse outbound JSON: %v", err)
		return err
	}
	configData, err := json.MarshalIndent(prettyJson, "", "  ")
	if err != nil {
		logger.Errorf("[SubOutbound] failed to pretty-print outbound JSON: %v", err)
		return err
	}
	if err := ManagedRuntimeWriteFile(configFilePath, configData); err != nil {
		logger.Errorf("[SubOutbound] failed to write outbound file: %v", err)
		return err
	}

	metadata := &SubOutboundMetadata{
		Id:        subOutbound.Id,
		Tag:       subOutbound.Tag,
		Type:      subOutbound.Type,
		UpdatedAt: time.Now().Unix(),
	}
	metaData, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		logger.Errorf("[SubOutbound] failed to marshal outbound metadata: %v", err)
		return err
	}
	if err := ManagedRuntimeWriteFile(metaFilePath, metaData); err != nil {
		logger.Errorf("[SubOutbound] failed to write metadata file: %v", err)
		return err
	}

	logger.Infof("[SubOutbound] saved config: %s (with metadata)", configFilePath)
	return nil
}

// deleteSubOutboundJson removes outbound config and metadata files by tag.
func (s *SubOutboundService) deleteSubOutboundJson(tag string) error {
	subManagerDir := filepath.Join(config.GetDataDir(), "sub_manager")

	baseFilename := sanitizeSubFilename(tag)
	configFilePath := filepath.Join(subManagerDir, fmt.Sprintf("%s.json", baseFilename))
	metaFilePath := filepath.Join(subManagerDir, fmt.Sprintf("%s_meta.json", baseFilename))

	if err := ManagedRuntimeDeleteFile(configFilePath); err != nil {
		logger.Errorf("[SubOutbound] failed to remove config file: %v", err)
		return err
	}

	if err := ManagedRuntimeDeleteFile(metaFilePath); err != nil {
		logger.Errorf("[SubOutbound] failed to remove metadata file: %v", err)
		return err
	}

	logger.Infof("[SubOutbound] deleted config and metadata: %s", configFilePath)
	return nil
}

// generateSubJsonFile generates a full sing-box subscription JSON file for one outbound.
func (s *SubOutboundService) generateSubJsonFile(subOutbound *model.SubOutbound) error {
	subJsonDir := filepath.Join(config.GetDataDir(), "sub_json")
	if err := validateSubOutboundSubJSONFileName(nil, subOutbound); err != nil {
		return err
	}

	outboundJson, err := resolveSubOutboundJSON(subOutbound)
	if err != nil {
		logger.Errorf("[SubOutbound] failed to marshal outbound: %v", err)
		return err
	}

	var outboundMap map[string]interface{}
	if err := json.Unmarshal(outboundJson, &outboundMap); err != nil {
		logger.Errorf("[SubOutbound] failed to parse outbound JSON: %v", err)
		return err
	}
	if tag, _ := outboundMap["tag"].(string); strings.TrimSpace(tag) == "" {
		outboundMap["tag"] = subOutbound.Tag
	}
	if outType, _ := outboundMap["type"].(string); strings.TrimSpace(outType) == "" {
		outboundMap["type"] = subOutbound.Type
	}

	var settingService SettingService
	othersStr, _ := settingService.GetSubJsonExt()
	refreshManagedSubOutboundTLS(outboundMap, subOutbound)

	result, err := renderManagedSingboxSubscriptionJSON(
		[]map[string]interface{}{outboundMap},
		othersStr,
		settingService.ResolveSubscriptionTLSStore,
	)
	if err != nil {
		logger.Errorf("[SubOutbound] failed to marshal subscription JSON: %v", err)
		return err
	}

	baseFilename := sanitizeSubFilename(subOutbound.Tag)
	filePath := filepath.Join(subJsonDir, fmt.Sprintf("%s.json", baseFilename))
	if err := ManagedRuntimeWriteFile(filePath, result); err != nil {
		logger.Errorf("[SubOutbound] failed to write subscription JSON file: %v", err)
		return err
	}

	logger.Infof("[SubOutbound] generated subscription JSON file: %s (size: %d bytes)", filePath, len(result))
	return nil
}

// extractTlsStoreFromSubOutbounds extracts tls store from outbound tls and removes tls_store/store keys.
func extractTlsStoreFromSubOutbounds(outbounds []map[string]interface{}) string {
	var tlsStore string
	for _, outbound := range outbounds {
		tlsRaw, ok := outbound["tls"]
		if !ok {
			continue
		}
		tlsMap, ok := tlsRaw.(map[string]interface{})
		if !ok {
			continue
		}
		if store, ok := tlsMap["tls_store"].(string); ok && store != "" && tlsStore == "" {
			tlsStore = store
		}
		if store, ok := tlsMap["store"].(string); ok && store != "" && tlsStore == "" {
			tlsStore = store
		}
		delete(tlsMap, "tls_store")
		delete(tlsMap, "store")
	}
	return tlsStore
}

func expandSubOutboundsForSubscription(raw []map[string]interface{}) ([]map[string]interface{}, []string) {
	outbounds := make([]map[string]interface{}, 0, len(raw))
	outTags := make([]string, 0, len(raw))

	for _, outbound := range raw {
		if outbound == nil {
			continue
		}

		outType, _ := outbound["type"].(string)
		tag, _ := outbound["tag"].(string)
		if tag == "" {
			continue
		}

		if outType != "shadowtls" {
			outbounds = append(outbounds, cloneSubOutboundMap(outbound))
			outTags = append(outTags, tag)
			continue
		}

		ssConfig, hasSS := outbound["ss_config"].(map[string]interface{})
		if !hasSS || ssConfig == nil {
			stlsOutbound := cloneSubOutboundMap(outbound)
			delete(stlsOutbound, "ss_config")
			stripShadowTLSInboundOnlyFields(stlsOutbound)
			outbounds = append(outbounds, stlsOutbound)
			outTags = append(outTags, tag)
			continue
		}

		stlsTag := tag + "-out"
		ssOutbound := map[string]interface{}{
			"type":   "shadowsocks",
			"tag":    tag,
			"detour": stlsTag,
		}
		if method, ok := ssConfig["method"]; ok && method != nil {
			ssOutbound["method"] = method
		}
		if network, ok := ssConfig["network"]; ok && network != nil && network != "" {
			ssOutbound["network"] = network
		}
		if password, ok := ssConfig["password"]; ok && password != nil {
			ssOutbound["password"] = password
		}
		if udpOverTCP, ok := ssConfig["udp_over_tcp"]; ok && udpOverTCP != nil {
			ssOutbound["udp_over_tcp"] = udpOverTCP
		}
		if multiplex, ok := ssConfig["multiplex"].(map[string]interface{}); ok && multiplex != nil {
			if enabled, ok := multiplex["enabled"].(bool); ok && enabled {
				ssOutbound["multiplex"] = multiplex
			}
		}

		stlsOutbound := cloneSubOutboundMap(outbound)
		delete(stlsOutbound, "ss_config")
		stlsOutbound["tag"] = stlsTag
		stripShadowTLSInboundOnlyFields(stlsOutbound)

		outbounds = append(outbounds, ssOutbound, stlsOutbound)
		outTags = append(outTags, tag)
	}

	return outbounds, outTags
}

func cloneSubOutboundMap(src map[string]interface{}) map[string]interface{} {
	dst := make(map[string]interface{}, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

func applyCertificateStoreToSubConfig(jsonConfig map[string]interface{}, tlsStore string) {
	if tlsStore == "" {
		return
	}

	certificate := map[string]interface{}{}
	if existing, ok := jsonConfig["certificate"].(map[string]interface{}); ok && existing != nil {
		for k, v := range existing {
			certificate[k] = v
		}
	}
	certificate["store"] = tlsStore
	jsonConfig["certificate"] = certificate
}

// removeDeprecatedDnsClashModeRules removes deprecated DNS clash_mode rules from subscription DNS config.
func removeDeprecatedDnsClashModeRules(dns interface{}) interface{} {
	dnsMap, ok := dns.(map[string]interface{})
	if !ok {
		return dns
	}

	rules, ok := dnsMap["rules"].([]interface{})
	if !ok || len(rules) == 0 {
		return dnsMap
	}

	filteredRules := make([]interface{}, 0, len(rules))
	for _, rawRule := range rules {
		ruleMap, ok := rawRule.(map[string]interface{})
		if !ok {
			filteredRules = append(filteredRules, rawRule)
			continue
		}

		// Compatibility cleanup:
		// Remove legacy fakeip DNS query_type route rules that are no longer used.
		if isDeprecatedFakeipQueryTypeRule(ruleMap) {
			continue
		}

		action, _ := ruleMap["action"].(string)
		clashMode, _ := ruleMap["clash_mode"].(string)
		server, _ := ruleMap["server"].(string)
		if strings.EqualFold(action, "route") &&
			((strings.EqualFold(clashMode, "global") && server == "proxy-dns") ||
				(strings.EqualFold(clashMode, "direct") && server == "direct-dns")) {
			continue
		}

		filteredRules = append(filteredRules, ruleMap)
	}

	dnsMap["rules"] = filteredRules
	return dnsMap
}

func isDeprecatedFakeipQueryTypeRule(rule map[string]interface{}) bool {
	action, _ := rule["action"].(string)
	if !strings.EqualFold(action, "route") {
		return false
	}

	server, _ := rule["server"].(string)
	if !strings.EqualFold(strings.TrimSpace(server), "fakeip") {
		return false
	}

	_, hasQueryType := rule["query_type"]
	return hasQueryType
}

func hasNonEmptyStringArrayValue(value interface{}) bool {
	switch list := value.(type) {
	case []interface{}:
		for _, item := range list {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				return true
			}
		}
	case []string:
		for _, item := range list {
			if strings.TrimSpace(item) != "" {
				return true
			}
		}
	}
	return false
}

func isManagedCustomDomainMatcherDnsRule(rule map[string]interface{}) bool {
	if _, hasRuleSet := rule["rule_set"]; hasRuleSet {
		return false
	}
	if _, hasQueryType := rule["query_type"]; hasQueryType {
		return false
	}

	action, _ := rule["action"].(string)
	action = strings.ToLower(strings.TrimSpace(action))
	if action != "reject" && action != "route" {
		return false
	}
	if action == "route" {
		server, _ := rule["server"].(string)
		if strings.TrimSpace(server) == "" {
			return false
		}
	}

	matcherCount := 0
	for _, key := range []string{"domain", "domain_suffix", "domain_keyword", "domain_regex"} {
		if hasNonEmptyStringArrayValue(rule[key]) {
			matcherCount++
		}
	}
	return matcherCount == 1
}

func isRuleSetDnsRouteRule(rule map[string]interface{}) bool {
	action, _ := rule["action"].(string)
	if !strings.EqualFold(strings.TrimSpace(action), "route") {
		return false
	}
	_, hasRuleSet := rule["rule_set"]
	return hasRuleSet
}

func reorderManagedCustomDnsRules(dns interface{}) interface{} {
	dnsMap, ok := dns.(map[string]interface{})
	if !ok {
		return dns
	}

	rules, ok := dnsMap["rules"].([]interface{})
	if !ok || len(rules) == 0 {
		return dnsMap
	}

	customMatcherRules := make([]interface{}, 0, len(rules))
	ruleSetRules := make([]interface{}, 0, len(rules))
	otherRules := make([]interface{}, 0, len(rules))

	for _, rawRule := range rules {
		ruleMap, ok := rawRule.(map[string]interface{})
		if !ok {
			otherRules = append(otherRules, rawRule)
			continue
		}

		if isManagedCustomDomainMatcherDnsRule(ruleMap) {
			customMatcherRules = append(customMatcherRules, ruleMap)
			continue
		}
		if isRuleSetDnsRouteRule(ruleMap) {
			ruleSetRules = append(ruleSetRules, ruleMap)
			continue
		}
		otherRules = append(otherRules, ruleMap)
	}

	dnsMap["rules"] = append(append(customMatcherRules, ruleSetRules...), otherRules...)
	return dnsMap
}

func normalizeSubRouteRules(rules []interface{}) []interface{} {
	normalized := make([]interface{}, 0, len(rules))
	for _, rawRule := range rules {
		ruleMap, ok := rawRule.(map[string]interface{})
		if !ok {
			normalized = append(normalized, rawRule)
			continue
		}
		normalized = append(normalized, normalizeSubRouteRuleMap(ruleMap))
	}
	return normalized
}

func normalizeSubRouteRuleMap(rule map[string]interface{}) map[string]interface{} {
	action, _ := rule["action"].(string)
	outbound, _ := rule["outbound"].(string)
	clashMode, _ := rule["clash_mode"].(string)
	if strings.EqualFold(action, "route") && (outbound == "block" || outbound == "reject") {
		rule["action"] = "reject"
		delete(rule, "outbound")
		action = "reject"
		outbound = ""
	}

	if strings.EqualFold(action, "route") {
		switch {
		case strings.EqualFold(clashMode, "global"):
			rule["outbound"] = globalSelectorTag
		case strings.EqualFold(clashMode, "direct"):
			if strings.TrimSpace(outbound) == "" {
				rule["outbound"] = globalDirectSelectorTag
			} else {
				rule["outbound"] = normalizeRouteOutboundTag(outbound)
			}
		default:
			normalized := normalizeRouteOutboundTag(outbound)
			if normalized != "" {
				rule["outbound"] = normalized
			}
		}
	}

	if nested, ok := rule["rules"].([]interface{}); ok {
		rule["rules"] = normalizeSubRouteRules(nested)
	}

	return rule
}

func normalizeRouteFinalOutbound(routeFinal string) string {
	normalized := normalizeLegacySingboxSelectorTag(routeFinal)
	switch normalized {
	case nodeSelectorTag, autoSelectorTag, globalDirectSelectorTag, globalBlockSelectorTag, finalSelectorTag:
		return normalized
	case globalSelectorTag:
		return globalSelectorTag
	}

	switch strings.ToLower(strings.TrimSpace(normalized)) {
	case "", "proxy":
		// Keep legacy "proxy" final behavior for backward compatibility.
		return finalSelectorTag
	case "auto":
		return autoSelectorTag
	case "direct", "global-direct":
		return globalDirectSelectorTag
	case "global-proxy", "global":
		return globalSelectorTag
	case "global-block", "block", "reject", "reject-drop":
		return globalBlockSelectorTag
	case "final":
		return finalSelectorTag
	default:
		return normalized
	}
}

func normalizeSubRuleSetDownloadDetours(ruleSet interface{}) interface{} {
	ruleSetList, ok := ruleSet.([]interface{})
	if !ok {
		return ruleSet
	}

	for _, raw := range ruleSetList {
		ruleMap, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		detour, _ := ruleMap["download_detour"].(string)
		if detour == "" {
			continue
		}
		normalizedDetour := normalizeDetourTag(detour)
		delete(ruleMap, "download_detour")
		ruleMap["http_client"] = map[string]interface{}{
			"detour": normalizedDetour,
		}
	}

	return ruleSetList
}

func normalizeSubDnsDetours(dns interface{}) interface{} {
	dnsMap, ok := dns.(map[string]interface{})
	if !ok {
		return dns
	}

	servers, ok := dnsMap["servers"].([]interface{})
	if !ok || len(servers) == 0 {
		return dnsMap
	}

	for _, rawServer := range servers {
		serverMap, ok := rawServer.(map[string]interface{})
		if !ok {
			continue
		}
		detour, _ := serverMap["detour"].(string)
		if detour == "" {
			continue
		}
		serverMap["detour"] = normalizeDetourTag(detour)
	}

	return dnsMap
}

func normalizeSubExperimentalClashAPIDetour(experimental interface{}) interface{} {
	experimentalMap, ok := experimental.(map[string]interface{})
	if !ok || experimentalMap == nil {
		return experimental
	}

	clashAPI, ok := experimentalMap["clash_api"].(map[string]interface{})
	if !ok || clashAPI == nil {
		return experimentalMap
	}

	rawDetour, ok := clashAPI["external_ui_download_detour"].(string)
	if !ok {
		return experimentalMap
	}

	detour := strings.TrimSpace(rawDetour)
	if detour == "" {
		delete(clashAPI, "external_ui_download_detour")
		return experimentalMap
	}

	// Convert when possible; keep original value when conversion is not possible.
	clashAPI["external_ui_download_detour"] = normalizeDetourTag(detour)
	return experimentalMap
}

func normalizeRouteOutboundTag(outbound string) string {
	normalized := normalizeLegacySingboxSelectorTag(outbound)
	switch strings.ToLower(normalized) {
	case "proxy":
		return nodeSelectorTag
	case "auto":
		return autoSelectorTag
	case "direct", "global-direct":
		return globalDirectSelectorTag
	case "global-proxy":
		return globalSelectorTag
	default:
		return normalized
	}
}

func normalizeDetourTag(detour string) string {
	normalized := normalizeLegacySingboxSelectorTag(detour)
	switch normalized {
	case nodeSelectorTag, autoSelectorTag, globalDirectSelectorTag, globalBlockSelectorTag, finalSelectorTag:
		return normalized
	case globalSelectorTag:
		return globalSelectorTag
	}
	switch strings.ToLower(strings.TrimSpace(normalized)) {
	case "proxy":
		return nodeSelectorTag
	case "auto":
		return autoSelectorTag
	case "direct", "global-direct":
		return globalDirectSelectorTag
	case "global-proxy", "global":
		return globalSelectorTag
	case "global-block", "block", "reject", "reject-drop":
		return globalBlockSelectorTag
	case "final":
		return finalSelectorTag
	default:
		return normalized
	}
}

func buildSubHTTPClients(extJson map[string]interface{}) ([]map[string]interface{}, bool) {
	if extJson == nil {
		return nil, false
	}

	rawDetour, ok := extJson["update_method"].(string)
	if !ok {
		return nil, false
	}

	detour := normalizeDetourTag(rawDetour)
	if strings.TrimSpace(detour) == "" {
		return nil, false
	}

	return []map[string]interface{}{
		{
			"tag":    managedSubHTTPClientTag,
			"detour": detour,
		},
	}, true
}

func parseSelectorGroupsFromExt(extJson map[string]interface{}) []selectorGroupConfig {
	if extJson == nil {
		return nil
	}

	rawGroups, ok := extJson["selector_groups"].([]interface{})
	if !ok || len(rawGroups) == 0 {
		return nil
	}

	reserved := map[string]struct{}{
		nodeSelectorTag:         {},
		autoSelectorTag:         {},
		globalDirectSelectorTag: {},
		globalBlockSelectorTag:  {},
		finalSelectorTag:        {},
		globalSelectorTag:       {},
		"direct":                {},
		"block":                 {},
	}

	seen := make(map[string]struct{}, len(rawGroups))
	groups := make([]selectorGroupConfig, 0, len(rawGroups))
	for _, raw := range rawGroups {
		groupMap, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}

		tag, _ := groupMap["tag"].(string)
		if strings.TrimSpace(tag) == "" {
			if fallback, ok := groupMap["name"].(string); ok {
				tag = fallback
			}
		}
		tag = normalizeLegacySingboxSelectorTag(tag)
		if tag == "" {
			continue
		}
		if _, exists := reserved[tag]; exists {
			continue
		}
		if _, exists := seen[tag]; exists {
			continue
		}
		seen[tag] = struct{}{}

		defaultOutbound, _ := groupMap["default_outbound"].(string)
		if strings.TrimSpace(defaultOutbound) == "" {
			if fallback, ok := groupMap["default"].(string); ok {
				defaultOutbound = fallback
			}
		}
		defaultOutbound = normalizeRouteOutboundTag(defaultOutbound)
		if defaultOutbound == "" || strings.EqualFold(defaultOutbound, "reject") || strings.EqualFold(defaultOutbound, "block") {
			defaultOutbound = nodeSelectorTag
		}

		groups = append(groups, selectorGroupConfig{
			Tag:             tag,
			DefaultOutbound: defaultOutbound,
		})
	}

	return groups
}

func buildNamedSelectorOutbounds(groups []selectorGroupConfig, nodeTags []string) []map[string]interface{} {
	if len(groups) == 0 {
		return nil
	}

	result := make([]map[string]interface{}, 0, len(groups))
	for _, group := range groups {
		if strings.TrimSpace(group.Tag) == "" {
			continue
		}
		result = append(result, map[string]interface{}{
			"type":                        "selector",
			"tag":                         group.Tag,
			"outbounds":                   buildNamedSelectorOptions(group.DefaultOutbound, nodeTags),
			"interrupt_exist_connections": true,
		})
	}
	return result
}

func buildNamedSelectorOptions(defaultOutbound string, nodeTags []string) []string {
	base := []string{
		nodeSelectorTag,
		autoSelectorTag,
		globalDirectSelectorTag,
		globalBlockSelectorTag,
		finalSelectorTag,
	}

	result := make([]string, 0, len(base)+len(nodeTags)+1)
	seen := make(map[string]struct{}, len(base)+len(nodeTags)+1)
	add := func(tag string) {
		normalized := strings.TrimSpace(tag)
		if normalized == "" {
			return
		}
		if _, exists := seen[normalized]; exists {
			return
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
	}

	add(defaultOutbound)
	for _, tag := range base {
		add(tag)
	}
	for _, tag := range nodeTags {
		add(tag)
	}

	return result
}

func buildSubJsonFullConfig(outbounds []map[string]interface{}, othersStr string) map[string]interface{} {
	// Use default subscription JSON template.
	var jsonConfig map[string]interface{}
	if err := json.Unmarshal([]byte(subJsonDefaultConfig), &jsonConfig); err != nil {
		// Fallback.
		jsonConfig = map[string]interface{}{}
	}

	jsonConfig["outbounds"] = outbounds

	// Build route config.
	route := map[string]interface{}{
		"auto_detect_interface":   true,
		"default_domain_resolver": "proxy-dns",
		"final":                   finalSelectorTag,
		"rules":                   []interface{}{},
	}

	// Apply SubJsonExt settings.
	if len(othersStr) > 0 {
		var othersJson map[string]interface{}
		if err := json.Unmarshal([]byte(othersStr), &othersJson); err == nil {
			if log, ok := othersJson["log"]; ok {
				jsonConfig["log"] = log
			}
			if dns, ok := othersJson["dns"]; ok {
				jsonConfig["dns"] = normalizeSubDnsDetours(reorderManagedCustomDnsRules(removeDeprecatedDnsClashModeRules(dns)))
			}
			if inbounds, ok := othersJson["inbounds"]; ok {
				jsonConfig["inbounds"] = inbounds
			}
			if experimental, ok := othersJson["experimental"]; ok {
				jsonConfig["experimental"] = normalizeSubExperimentalClashAPIDetour(experimental)
			}
			if certificate, ok := othersJson["certificate"]; ok {
				jsonConfig["certificate"] = certificate
			}
			if httpClients, ok := buildSubHTTPClients(othersJson); ok {
				jsonConfig["http_clients"] = httpClients
				route["default_http_client"] = managedSubHTTPClientTag
			}
			if ruleSet, ok := othersJson["rule_set"]; ok {
				route["rule_set"] = normalizeSubRuleSetDownloadDetours(ruleSet)
			}
			if settingRules, ok := othersJson["rules"].([]interface{}); ok {
				route["rules"] = normalizeSubRouteRules(settingRules)
			}
			if routeFinal, ok := othersJson["route_final"].(string); ok {
				route["final"] = normalizeRouteFinalOutbound(routeFinal)
			}
			if defaultDomainResolver, ok := othersJson["default_domain_resolver"].(string); ok {
				route["default_domain_resolver"] = defaultDomainResolver
			}
			// Remove front-end-only UI state from generated config.
			delete(othersJson, "_uiConfig")
		}
	}

	jsonConfig["route"] = route

	return jsonConfig
}

// deleteSubJsonFile removes one generated subscription JSON file by tag.
func (s *SubOutboundService) deleteSubJsonFile(tag string) error {
	subJsonDir := filepath.Join(config.GetDataDir(), "sub_json")

	baseFilename := sanitizeSubFilename(tag)
	filePath := filepath.Join(subJsonDir, fmt.Sprintf("%s.json", baseFilename))

	if err := ManagedRuntimeDeleteFile(filePath); err != nil {
		logger.Errorf("[SubOutbound] failed to delete subscription file: %v", err)
		return err
	}
	logger.Infof("[SubOutbound] deleted subscription JSON file: %s", filePath)
	return nil
}

// RegenerateAllSubOutboundConfigs rewrites all sub_manager outbound config files.
func (s *SubOutboundService) RegenerateAllSubOutboundConfigs() {
	db := database.GetDB()

	var subOutbounds []*model.SubOutbound
	if err := db.Model(model.SubOutbound{}).Find(&subOutbounds).Error; err != nil {
		logger.Errorf("[SubOutbound] failed to list sub outbounds: %v", err)
		return
	}

	subManagerDir := filepath.Join(config.GetDataDir(), "sub_manager")

	clearSubJsonFilesInDir(subManagerDir)

	for _, subOutbound := range subOutbounds {
		if err := s.saveSubOutboundJson(subOutbound); err != nil {
			logger.Errorf("[SubOutbound] failed to regenerate sub_manager config [%s]: %v", subOutbound.Tag, err)
		}
	}

	logger.Infof("[SubOutbound] regenerated %d sub outbound configs (sub_manager)", len(subOutbounds))
}

// RegenerateAllSubJsonFiles rewrites all generated subscription JSON files.
func (s *SubOutboundService) RegenerateAllSubJsonFiles() {
	db := database.GetDB()
	if err := validateManagedSubJSONFileNames(db); err != nil {
		logger.Errorf("[SubOutbound] validate sub_json filenames failed: %v", err)
		return
	}

	var subOutbounds []*model.SubOutbound
	if err := db.Model(model.SubOutbound{}).Find(&subOutbounds).Error; err != nil {
		logger.Errorf("[SubOutbound] failed to list sub outbounds: %v", err)
		return
	}

	for _, subOutbound := range subOutbounds {
		if err := s.generateSubJsonFile(subOutbound); err != nil {
			logger.Errorf("[SubOutbound] failed to regenerate sub_json file [%s]: %v", subOutbound.Tag, err)
		}
	}

	logger.Infof("[SubOutbound] regenerated %d subscription JSON files (sub_json)", len(subOutbounds))
}

// sanitizeSubFilename replaces invalid path chars with underscore.
func sanitizeSubFilename(name string) string {
	unsafe := []string{"/", "\\", ":", "*", "?", "\"", "<", ">", "|"}
	result := name
	for _, char := range unsafe {
		result = replaceSubAll(result, char, "_")
	}
	return result
}

func replaceSubAll(s, old, new string) string {
	for {
		idx := indexSubOf(s, old)
		if idx == -1 {
			return s
		}
		s = s[:idx] + new + s[idx+len(old):]
	}
}

func indexSubOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

// clearSubJsonFilesInDir removes all *.json files under a directory.
func clearSubJsonFilesInDir(dir string) {
	_ = ManagedRuntimeClearDirJSONFiles(dir)
}

func normalizeClashProxyOptionsTag(raw json.RawMessage, tag string) json.RawMessage {
	if len(raw) == 0 || strings.TrimSpace(tag) == "" {
		return raw
	}

	var proxy map[string]interface{}
	if err := json.Unmarshal(raw, &proxy); err != nil {
		return raw
	}

	name, _ := proxy["name"].(string)
	if strings.TrimSpace(name) == strings.TrimSpace(tag) {
		return raw
	}
	proxy["name"] = strings.TrimSpace(tag)

	normalized, err := json.MarshalIndent(proxy, "", "  ")
	if err != nil {
		return raw
	}
	return normalized
}
