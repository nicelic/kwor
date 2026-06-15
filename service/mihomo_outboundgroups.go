package service

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/logger"
	"github.com/alireza0/s-ui/util/common"
	"gopkg.in/yaml.v3"
	"gorm.io/gorm"
)

type MihomoOutboundGroupService struct{}

const mihomoImportedClashProxyKey = "_mihomo_clash_proxy"

type mihomoOutboundGroupReorderRequest struct {
	IDs []uint `json:"ids"`
}

func (s *MihomoOutboundGroupService) GetAll() ([]*model.MihomoOutboundGroup, error) {
	db := database.GetDB()
	if err := ensureMihomoOutboundGroupSortOrders(db); err != nil {
		return nil, err
	}
	var groups []*model.MihomoOutboundGroup
	if err := db.Model(model.MihomoOutboundGroup{}).Order("sort_order ASC").Order("id ASC").Find(&groups).Error; err != nil {
		return nil, err
	}
	return groups, nil
}

func ensureMihomoOutboundGroupSortOrders(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("database is not initialized")
	}

	var groups []model.MihomoOutboundGroup
	if err := db.Model(model.MihomoOutboundGroup{}).
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
		if err := db.Model(&model.MihomoOutboundGroup{}).
			Where("id = ?", group.Id).
			Update("sort_order", index+1).Error; err != nil {
			return err
		}
	}

	return nil
}

func nextMihomoOutboundGroupSortOrder(tx *gorm.DB) (int, error) {
	if err := ensureMihomoOutboundGroupSortOrders(tx); err != nil {
		return 0, err
	}

	var maxSortOrder int
	if err := tx.Model(model.MihomoOutboundGroup{}).
		Select("COALESCE(MAX(sort_order), 0)").
		Scan(&maxSortOrder).Error; err != nil {
		return 0, err
	}

	return maxSortOrder + 1, nil
}

func normalizeMihomoOutboundGroupReorderIDs(ids []uint) []uint {
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

func (s *MihomoOutboundGroupService) reorder(tx *gorm.DB, ids []uint) error {
	cleanedIDs := normalizeMihomoOutboundGroupReorderIDs(ids)
	if len(cleanedIDs) == 0 {
		return fmt.Errorf("group ids are required")
	}

	if err := ensureMihomoOutboundGroupSortOrders(tx); err != nil {
		return err
	}

	var groups []model.MihomoOutboundGroup
	if err := tx.Model(model.MihomoOutboundGroup{}).Select("id").Find(&groups).Error; err != nil {
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
		if err := tx.Model(&model.MihomoOutboundGroup{}).
			Where("id = ?", id).
			Update("sort_order", index+1).Error; err != nil {
			return err
		}
	}

	return nil
}

func (s *MihomoOutboundGroupService) Save(tx *gorm.DB, act string, data json.RawMessage) error {
	switch act {
	case "new", "edit":
		var group model.MihomoOutboundGroup
		if err := json.Unmarshal(data, &group); err != nil {
			return err
		}
		group.Name = strings.TrimSpace(group.Name)
		group.SubscriptionUrl = strings.TrimSpace(group.SubscriptionUrl)
		if group.Outbounds == "" {
			group.Outbounds = "[]"
		}
		if group.Id > 0 {
			var existing model.MihomoOutboundGroup
			if err := tx.Where("id = ?", group.Id).First(&existing).Error; err == nil {
				if group.SortOrder <= 0 {
					group.SortOrder = existing.SortOrder
				}
			} else if !database.IsNotFound(err) {
				return err
			}
		} else if group.SortOrder <= 0 {
			nextSortOrder, err := nextMihomoOutboundGroupSortOrder(tx)
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
		name = strings.TrimSpace(name)
		if name == "" {
			return fmt.Errorf("group name is required")
		}

		var group model.MihomoOutboundGroup
		if err := tx.Where("name = ?", name).First(&group).Error; err != nil {
			return err
		}

		tags := parseOutboundGroupTags(group.Outbounds)
		if len(tags) > 0 {
			if err := tx.Where("tag IN ?", tags).Delete(&model.MihomoOutbound{}).Error; err != nil {
				return err
			}
		}

		return tx.Where("name = ?", name).Delete(model.MihomoOutboundGroup{}).Error
	case "reorder":
		var request mihomoOutboundGroupReorderRequest
		if err := json.Unmarshal(data, &request); err != nil {
			return err
		}
		return s.reorder(tx, request.IDs)
	default:
		return common.NewErrorf("unknown action: %s", act)
	}
}

func (s *MihomoOutboundGroupService) FetchAndSaveSubscription(groupName string, url string, allowInsecure bool) error {
	groupName = strings.TrimSpace(groupName)
	url = strings.TrimSpace(url)
	if groupName == "" || url == "" {
		return fmt.Errorf("group_name and url are required")
	}

	clashData, err := fetchSubscriptionJSON(url, allowInsecure)
	if err != nil {
		return err
	}
	proxies, err := extractClashProxiesRaw(clashData)
	if err != nil {
		return err
	}
	rawByTag, err := extractClashProxyRawYAMLByName(clashData)
	if err != nil {
		return err
	}
	outbounds := buildMihomoImportedOutbounds(proxies)
	if len(outbounds) == 0 {
		return fmt.Errorf("subscription has no valid mihomo outbounds")
	}

	db := database.GetDB()
	var group model.MihomoOutboundGroup
	if err := db.Where("name = ?", groupName).First(&group).Error; err != nil {
		return err
	}

	savedTags := make([]string, 0, len(outbounds))
	for _, outbound := range outbounds {
		tag, _ := outbound["tag"].(string)
		if strings.TrimSpace(tag) == "" {
			continue
		}
		if err := upsertImportedMihomoOutbound(db, outbound, nil, rawByTag[tag]); err != nil {
			logger.Errorf("[MihomoOutboundGroup] save outbound failed [%s]: %v", tag, err)
			continue
		}
		savedTags = append(savedTags, tag)
	}

	if len(savedTags) == 0 {
		return fmt.Errorf("no valid mihomo outbounds were saved")
	}

	tagsJSON, _ := json.Marshal(savedTags)
	if err := db.Model(&model.MihomoOutboundGroup{}).Where("name = ?", groupName).Updates(map[string]interface{}{
		"outbounds":        string(tagsJSON),
		"subscription_url": url,
		"allow_insecure":   allowInsecure,
	}).Error; err != nil {
		return err
	}

	s.notifyOutboundsChanged()
	return nil
}

func (s *MihomoOutboundGroupService) RefreshSubscription(groupName string, url string, allowInsecure bool) (*SubscriptionRefreshResult, error) {
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
	var group model.MihomoOutboundGroup
	if err := dbConn.Where("name = ?", groupName).First(&group).Error; err != nil {
		return nil, err
	}

	clashData, err := fetchSubscriptionJSON(url, allowInsecure)
	if err != nil {
		return nil, err
	}
	proxies, err := extractClashProxiesRaw(clashData)
	if err != nil {
		return nil, err
	}
	rawByTag, err := extractClashProxyRawYAMLByName(clashData)
	if err != nil {
		return nil, err
	}
	newOutbounds := buildMihomoImportedOutbounds(proxies)
	if len(newOutbounds) == 0 {
		return nil, fmt.Errorf("subscription has no valid mihomo outbounds")
	}

	oldOutbounds, err := loadGroupedMihomoOutbounds(dbConn, parseOutboundGroupTags(group.Outbounds))
	if err != nil {
		return nil, err
	}

	oldMap := make(map[string]map[string]interface{}, len(oldOutbounds))
	for _, ob := range oldOutbounds {
		tag, _ := ob["tag"].(string)
		outType, _ := ob["type"].(string)
		if tag != "" {
			oldMap[tag+"|"+outType] = ob
		}
	}

	newMap := make(map[string]map[string]interface{}, len(newOutbounds))
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
		if err := dbConn.Where("tag IN ?", result.Removed).Delete(&model.MihomoOutbound{}).Error; err != nil {
			return nil, err
		}
	}

	savedTags := make([]string, 0, len(newOutbounds))
	for _, outbound := range newOutbounds {
		tag, _ := outbound["tag"].(string)
		if strings.TrimSpace(tag) == "" {
			continue
		}
		if err := upsertImportedMihomoOutbound(dbConn, outbound, nil, rawByTag[tag]); err != nil {
			logger.Errorf("[MihomoOutboundGroup] refresh save outbound failed [%s]: %v", tag, err)
			continue
		}
		savedTags = append(savedTags, tag)
	}

	tagsJSON, _ := json.Marshal(savedTags)
	if err := dbConn.Model(&model.MihomoOutboundGroup{}).Where("name = ?", groupName).Updates(map[string]interface{}{
		"outbounds":        string(tagsJSON),
		"subscription_url": url,
		"allow_insecure":   allowInsecure,
	}).Error; err != nil {
		return nil, err
	}

	s.notifyOutboundsChanged()
	return result, nil
}

func (s *MihomoOutboundGroupService) notifyOutboundsChanged() {
	LastUpdate = time.Now().Unix()
	if err := NewMihomoManagerService().RegenerateServerConfig(); err != nil {
		logger.Warning("[MihomoOutboundGroup] regenerate mihomo server config failed: ", err)
	}
}

func buildMihomoImportedOutbounds(proxies []map[string]interface{}) []map[string]interface{} {
	result := make([]map[string]interface{}, 0, len(proxies))
	seen := make(map[string]struct{}, len(proxies))
	for _, proxy := range proxies {
		if proxy == nil {
			continue
		}

		tag, _ := proxy["name"].(string)
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		outType, _ := proxy["type"].(string)
		outType = strings.TrimSpace(outType)
		if outType == "" {
			continue
		}
		if _, exists := seen[tag]; exists {
			continue
		}
		seen[tag] = struct{}{}

		outbound := normalizeMihomoImportedOutbound(proxy, tag, outType)
		result = append(result, outbound)
	}
	return result
}

func normalizeMihomoImportedOutbound(proxy map[string]interface{}, tag string, outType string) map[string]interface{} {
	if converted, ok := convertClashProxyToSubOutbound(proxy); ok && converted != nil {
		converted["tag"] = tag
		converted["type"] = outType
		enrichMihomoImportedOutbound(converted, proxy, outType)
		converted[mihomoImportedClashProxyKey] = cloneMap(proxy)
		return converted
	}

	outbound := cloneMap(proxy)
	if outbound == nil {
		outbound = map[string]interface{}{}
	}
	outbound["tag"] = tag
	outbound["type"] = outType
	return outbound
}

func enrichMihomoImportedOutbound(outbound map[string]interface{}, proxy map[string]interface{}, outType string) {
	if outbound == nil || proxy == nil {
		return
	}

	switch strings.ToLower(strings.TrimSpace(outType)) {
	case "tuic":
		enrichMihomoImportedTUICOutbound(outbound, proxy)
	case "hysteria":
		enrichMihomoImportedHysteriaOutbound(outbound, proxy)
	}
}

func enrichMihomoImportedHysteriaOutbound(outbound map[string]interface{}, proxy map[string]interface{}) {
	if auth, ok := proxy["auth-str"].(string); ok && strings.TrimSpace(auth) != "" {
		outbound["auth_str"] = strings.TrimSpace(auth)
	}
	if obfs, ok := proxy["obfs"].(string); ok && strings.TrimSpace(obfs) != "" {
		outbound["obfs"] = strings.TrimSpace(obfs)
	}
	if up, ok := toIntValue(proxy["up"]); ok && up > 0 {
		outbound["up_mbps"] = up
	}
	if down, ok := toIntValue(proxy["down"]); ok && down > 0 {
		outbound["down_mbps"] = down
	}
	if streamReceiveWindow, ok := toIntValue(proxy["recv-window-conn"]); ok && streamReceiveWindow > 0 {
		outbound["stream_receive_window"] = streamReceiveWindow
	}
	if connectionReceiveWindow, ok := toIntValue(proxy["recv-window"]); ok && connectionReceiveWindow > 0 {
		outbound["connection_receive_window"] = connectionReceiveWindow
	}
	if disablePathMTUDiscovery, ok := toBoolValue(proxy["disable-mtu-discovery"]); ok {
		outbound["disable_path_mtu_discovery"] = disablePathMTUDiscovery
	}
	if fastOpen, ok := toBoolValue(proxy["fast-open"]); ok {
		outbound["mihomo_fast_open"] = fastOpen
	}
	if ports, ok := proxy["ports"].(string); ok && strings.TrimSpace(ports) != "" {
		serverPorts := parseClashPortsString(ports)
		if len(serverPorts) > 0 {
			outbound["server_ports"] = serverPorts
		}
	}
}

func enrichMihomoImportedTUICOutbound(outbound map[string]interface{}, proxy map[string]interface{}) {
	if timeoutMS, ok := toIntValue(proxy["request-timeout"]); ok && timeoutMS > 0 {
		outbound["request_timeout"] = formatMihomoImportedMilliseconds(timeoutMS)
	}
	if heartbeatMS, ok := toIntValue(proxy["heartbeat-interval"]); ok && heartbeatMS > 0 {
		outbound["heartbeat"] = formatMihomoImportedMilliseconds(heartbeatMS)
	}
	if maxOpenStreams, ok := toIntValue(proxy["max-open-streams"]); ok && maxOpenStreams > 0 {
		outbound["max_open_streams"] = maxOpenStreams
	}
	if maxPacketSize, ok := toIntValue(proxy["max-udp-relay-packet-size"]); ok && maxPacketSize > 0 {
		outbound["max_udp_relay_packet_size"] = maxPacketSize
	}
	if value, ok := toIntValue(proxy["cwnd"]); ok && value > 0 {
		outbound["cwnd"] = value
	}
	if value, ok := toIntValue(proxy["udp-over-stream-version"]); ok && value > 0 {
		outbound["udp_over_stream_version"] = value
	}
	if value, ok := toIntValue(proxy["max-datagram-frame-size"]); ok && value > 0 {
		outbound["max_datagram_frame_size"] = value
	}
	if ip, ok := proxy["ip"].(string); ok && strings.TrimSpace(ip) != "" {
		outbound["ip"] = strings.TrimSpace(ip)
	}
	if value, ok := toBoolValue(proxy["udp-over-stream"]); ok {
		outbound["udp_over_stream"] = value
	}
	if value, ok := toBoolValue(proxy["disable-mtu-discovery"]); ok {
		outbound["disable_mtu_discovery"] = value
	}
}

func formatMihomoImportedMilliseconds(value int) string {
	if value <= 0 {
		return ""
	}
	if value%1000 == 0 {
		return fmt.Sprintf("%ds", value/1000)
	}
	return fmt.Sprintf("%dms", value)
}

func extractClashProxiesRaw(yamlData []byte) ([]map[string]interface{}, error) {
	var doc map[string]interface{}
	if err := yaml.Unmarshal(yamlData, &doc); err != nil {
		return nil, fmt.Errorf("failed to parse clash yaml: %v", err)
	}

	raw, ok := doc["proxies"]
	if !ok {
		return nil, fmt.Errorf("clash subscription has no proxies field")
	}

	proxies, ok := raw.([]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid clash proxies format")
	}

	result := make([]map[string]interface{}, 0, len(proxies))
	for _, item := range proxies {
		proxy, ok := item.(map[string]interface{})
		if !ok || proxy == nil {
			continue
		}

		name, _ := proxy["name"].(string)
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}

		result = append(result, cloneMap(proxy))
	}

	return result, nil
}

func upsertImportedMihomoOutbound(db *gorm.DB, outbound map[string]interface{}, raw json.RawMessage, rawClashYAML []byte) error {
	outboundBytes, err := json.Marshal(outbound)
	if err != nil {
		return err
	}

	var dbOutbound model.MihomoOutbound
	if err := dbOutbound.UnmarshalJSON(outboundBytes); err != nil {
		return err
	}

	var existing model.MihomoOutbound
	if err := db.Where("tag = ?", dbOutbound.Tag).First(&existing).Error; err == nil {
		dbOutbound.Id = existing.Id
	} else if !database.IsNotFound(err) {
		return err
	}

	if len(raw) > 0 {
		dbOutbound.RawOutbound = normalizeMihomoOutboundRawPayload(raw)
	} else {
		dbOutbound.RawOutbound = normalizeMihomoOutboundRawPayload(outboundBytes)
	}
	dbOutbound.RawClashYAML = cloneRawYAMLBytes(rawClashYAML)

	return db.Save(&dbOutbound).Error
}

func loadGroupedMihomoOutbounds(db *gorm.DB, tags []string) ([]map[string]interface{}, error) {
	if len(tags) == 0 {
		return []map[string]interface{}{}, nil
	}

	var outbounds []*model.MihomoOutbound
	if err := db.Model(model.MihomoOutbound{}).Where("tag IN ?", tags).Find(&outbounds).Error; err != nil {
		return nil, err
	}

	result := make([]map[string]interface{}, 0, len(outbounds))
	for _, outbound := range outbounds {
		raw, err := resolveMihomoOutboundJSON(outbound)
		if err != nil {
			continue
		}
		payload := map[string]interface{}{}
		if err := json.Unmarshal(raw, &payload); err != nil {
			continue
		}
		result = append(result, payload)
	}
	return result, nil
}
