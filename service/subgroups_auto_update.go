package service

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/logger"

	"gorm.io/gorm"
)

var (
	subGroupSubscriptionUpdateMu sync.Mutex
	subGroupAutoUpdateMu         sync.Mutex
)

const (
	subGroupAutoUpdateEnabledKey         = "subGroupAutoUpdateEnabled"
	subGroupAutoUpdateIntervalMinutesKey = "subGroupAutoUpdateIntervalMinutes"
	subGroupAutoUpdateLastAtKey          = "subGroupAutoUpdateLastAt"

	subGroupAutoUpdateSourceJSON  = "json"
	subGroupAutoUpdateSourceClash = "clash"

	subGroupAutoUpdateSourceTimeout = 10 * time.Second
	subGroupAutoUpdateRetryCount    = 3
)

type SubGroupAutoUpdateInfo struct {
	Enabled         bool  `json:"enabled"`
	IntervalMinutes int   `json:"intervalMinutes"`
	LastRunAt       int64 `json:"lastRunAt"`
}

type subGroupAutoUpdatePayloads struct {
	jsonOutbounds []map[string]interface{}
	jsonRawByTag  map[string]json.RawMessage
	clashProxies  []map[string]interface{}
}

type subGroupAutoUpdateGroupRun struct {
	group         *model.SubGroup
	payloads      subGroupAutoUpdatePayloads
	failedSources map[string]string
	applied       bool
}

func (s *SettingService) GetSubGroupAutoUpdateSettings() (bool, int, int64, error) {
	enabled, err := s.getBool(subGroupAutoUpdateEnabledKey)
	if err != nil {
		return false, 0, 0, err
	}
	intervalMinutes, err := s.getInt(subGroupAutoUpdateIntervalMinutesKey)
	if err != nil {
		return false, 0, 0, err
	}
	if intervalMinutes <= 0 {
		intervalMinutes = 5
	}
	lastRunAt, err := s.getString(subGroupAutoUpdateLastAtKey)
	if err != nil {
		return false, 0, 0, err
	}
	lastRunAtInt, parseErr := parseUnixSetting(lastRunAt)
	if parseErr != nil {
		lastRunAtInt = 0
	}
	return enabled, intervalMinutes, lastRunAtInt, nil
}

func (s *SettingService) GetSubGroupAutoUpdateInfo() (*SubGroupAutoUpdateInfo, error) {
	enabled, intervalMinutes, lastRunAt, err := s.GetSubGroupAutoUpdateSettings()
	if err != nil {
		return nil, err
	}
	return &SubGroupAutoUpdateInfo{
		Enabled:         enabled,
		IntervalMinutes: intervalMinutes,
		LastRunAt:       lastRunAt,
	}, nil
}

func (s *SettingService) SaveSubGroupAutoUpdateSettings(enabled bool, intervalMinutes int) error {
	if intervalMinutes <= 0 {
		intervalMinutes = 5
	}
	if err := s.setString(subGroupAutoUpdateEnabledKey, boolToSettingValue(enabled)); err != nil {
		return err
	}
	return s.setInt(subGroupAutoUpdateIntervalMinutesKey, intervalMinutes)
}

func (s *SettingService) SetSubGroupAutoUpdateLastAt(ts int64) error {
	return s.setString(subGroupAutoUpdateLastAtKey, fmt.Sprintf("%d", ts))
}

func parseUnixSetting(raw string) (int64, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	var ts int64
	_, err := fmt.Sscanf(raw, "%d", &ts)
	return ts, err
}

func boolToSettingValue(value bool) string {
	if value {
		return "true"
	}
	return "false"
}

func (s *SubGroupService) clearSubGroupAutoUpdateFailure(db *gorm.DB, groupID uint) error {
	if groupID == 0 {
		return nil
	}
	return db.Model(&model.SubGroup{}).Where("id = ?", groupID).Updates(map[string]interface{}{
		"auto_update_failed_sources": "",
		"auto_update_error":          "",
	}).Error
}

func (s *SubGroupService) markSubGroupAutoUpdateResult(
	db *gorm.DB,
	groupID uint,
	lastAt int64,
	failedSources []string,
	errMsg string,
) error {
	if groupID == 0 {
		return nil
	}
	updates := map[string]interface{}{
		"auto_update_last_at":        lastAt,
		"auto_update_failed_sources": encodeSubGroupAutoUpdateFailedSources(failedSources),
		"auto_update_error":          strings.TrimSpace(errMsg),
	}
	if err := db.Model(&model.SubGroup{}).Where("id = ?", groupID).Updates(updates).Error; err != nil {
		return err
	}
	LastUpdate = time.Now().Unix()
	return nil
}

func encodeSubGroupAutoUpdateFailedSources(sources []string) string {
	if len(sources) == 0 {
		return ""
	}
	cleaned := make([]string, 0, len(sources))
	seen := make(map[string]struct{}, len(sources))
	for _, source := range sources {
		source = strings.TrimSpace(strings.ToLower(source))
		if source == "" {
			continue
		}
		if _, exists := seen[source]; exists {
			continue
		}
		seen[source] = struct{}{}
		cleaned = append(cleaned, source)
	}
	if len(cleaned) == 0 {
		return ""
	}
	data, err := json.Marshal(cleaned)
	if err != nil {
		return ""
	}
	return string(data)
}

func (s *SubGroupService) RunAutoUpdate() error {
	subGroupAutoUpdateMu.Lock()
	defer subGroupAutoUpdateMu.Unlock()

	settingSvc := &SettingService{}
	enabled, intervalMinutes, lastRunAt, err := settingSvc.GetSubGroupAutoUpdateSettings()
	if err != nil {
		return err
	}
	if !enabled {
		return nil
	}

	now := time.Now().Unix()
	if lastRunAt > 0 && now < lastRunAt+int64(intervalMinutes)*60 {
		return nil
	}
	if err := settingSvc.SetSubGroupAutoUpdateLastAt(now); err != nil {
		return err
	}

	subGroupSubscriptionUpdateMu.Lock()
	defer subGroupSubscriptionUpdateMu.Unlock()

	groups, err := s.GetAllForAutoUpdate()
	if err != nil {
		return err
	}

	db := database.GetDB()
	pendingRetries := make(map[uint]error)
	for _, group := range groups {
		if !subGroupSupportsAutoUpdate(group) {
			continue
		}
		_, refreshErr := s.refreshSubscriptionSourcesWithTimeout(
			strings.TrimSpace(group.Name),
			strings.TrimSpace(group.SubscriptionUrl),
			strings.TrimSpace(group.SubscriptionUrlClash),
			group.AllowInsecure,
			false,
			subGroupAutoUpdateSourceTimeout,
		)
		if refreshErr == nil {
			if err := s.markSubGroupAutoUpdateResult(db, group.Id, now, nil, ""); err != nil {
				logger.Warningf("[SubGroup] mark auto-update success failed: %v", err)
			}
			continue
		}
		pendingRetries[group.Id] = refreshErr
	}

	for _, group := range groups {
		lastErr, pending := pendingRetries[group.Id]
		if !pending {
			continue
		}

		for attempt := 0; attempt < subGroupAutoUpdateRetryCount; attempt++ {
			_, refreshErr := s.refreshSubscriptionSourcesWithTimeout(
				strings.TrimSpace(group.Name),
				strings.TrimSpace(group.SubscriptionUrl),
				strings.TrimSpace(group.SubscriptionUrlClash),
				group.AllowInsecure,
				false,
				subGroupAutoUpdateSourceTimeout,
			)
			if refreshErr == nil {
				lastErr = nil
				break
			}
			lastErr = refreshErr
		}

		if lastErr == nil {
			if err := s.markSubGroupAutoUpdateResult(db, group.Id, now, nil, ""); err != nil {
				logger.Warningf("[SubGroup] mark auto-update success failed: %v", err)
			}
			continue
		}

		failedSources := configuredSubGroupAutoUpdateSources(group)
		if err := s.markSubGroupAutoUpdateResult(db, group.Id, now, failedSources, lastErr.Error()); err != nil {
			logger.Warningf("[SubGroup] mark auto-update failure failed: %v", err)
		}
	}

	return nil
}

func subGroupSupportsAutoUpdate(group *model.SubGroup) bool {
	if group == nil {
		return false
	}
	return strings.TrimSpace(group.SubscriptionUrl) != "" || strings.TrimSpace(group.SubscriptionUrlClash) != ""
}

func (s *SubGroupService) fetchAutoUpdateJSONPayload(url string, allowInsecure bool) ([]map[string]interface{}, map[string]json.RawMessage, error) {
	body, err := fetchSubscriptionJSONWithTimeout(url, allowInsecure, subGroupAutoUpdateSourceTimeout)
	if err != nil {
		return nil, nil, err
	}
	outbounds, err := extractSubscriptionJSONOutboundsRaw(body)
	if err != nil {
		return nil, nil, err
	}
	rawByTag, err := extractSubscriptionJSONOutboundRawByTag(body)
	if err != nil {
		return nil, nil, err
	}
	if len(outbounds) == 0 {
		return nil, nil, fmt.Errorf("subscription contains no valid nodes")
	}
	return outbounds, rawByTag, nil
}

func (s *SubGroupService) fetchAutoUpdateClashPayload(url string, allowInsecure bool) ([]map[string]interface{}, error) {
	body, err := fetchSubscriptionJSONWithTimeout(url, allowInsecure, subGroupAutoUpdateSourceTimeout)
	if err != nil {
		return nil, err
	}
	proxies, err := extractClashProxies(body)
	if err != nil {
		return nil, err
	}
	if len(proxies) == 0 {
		return nil, fmt.Errorf("subscription contains no valid nodes")
	}
	return proxies, nil
}

func hasSubGroupAutoUpdatePayloads(payloads subGroupAutoUpdatePayloads) bool {
	return len(payloads.jsonOutbounds) > 0 || len(payloads.clashProxies) > 0
}

func (s *SubGroupService) applyAutoUpdatePayloads(group *model.SubGroup, payloads subGroupAutoUpdatePayloads) error {
	nodes, err := buildSubscriptionImportNodes(payloads.jsonOutbounds, payloads.clashProxies)
	if err != nil {
		return err
	}
	nodes = attachSubscriptionJSONRawByTag(nodes, payloads.jsonRawByTag)
	_, err = s.replaceSubscriptionGroupNodesTx(
		strings.TrimSpace(group.Name),
		nodes,
		strings.TrimSpace(group.SubscriptionUrl),
		strings.TrimSpace(group.SubscriptionUrlClash),
		group.AllowInsecure,
		false,
	)
	return err
}

func configuredSubGroupAutoUpdateSources(group *model.SubGroup) []string {
	if group == nil {
		return nil
	}
	sources := make([]string, 0, 2)
	if strings.TrimSpace(group.SubscriptionUrl) != "" {
		sources = append(sources, subGroupAutoUpdateSourceJSON)
	}
	if strings.TrimSpace(group.SubscriptionUrlClash) != "" {
		sources = append(sources, subGroupAutoUpdateSourceClash)
	}
	return sources
}

func cloneSubGroupAutoUpdateFailures(src map[string]string) map[string]string {
	if len(src) == 0 {
		return map[string]string{}
	}
	dst := make(map[string]string, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func orderedSubGroupAutoUpdateFailures(group *model.SubGroup, failures map[string]string) []string {
	if len(failures) == 0 {
		return nil
	}
	ordered := make([]string, 0, len(failures))
	for _, source := range configuredSubGroupAutoUpdateSources(group) {
		if _, exists := failures[source]; exists {
			ordered = append(ordered, source)
		}
	}
	return ordered
}

func joinSubGroupAutoUpdateErrors(failures map[string]string) string {
	if len(failures) == 0 {
		return ""
	}
	parts := make([]string, 0, len(failures))
	if errMsg, exists := failures[subGroupAutoUpdateSourceJSON]; exists && strings.TrimSpace(errMsg) != "" {
		parts = append(parts, fmt.Sprintf("json: %s", strings.TrimSpace(errMsg)))
	}
	if errMsg, exists := failures[subGroupAutoUpdateSourceClash]; exists && strings.TrimSpace(errMsg) != "" {
		parts = append(parts, fmt.Sprintf("clash: %s", strings.TrimSpace(errMsg)))
	}
	return strings.Join(parts, "; ")
}
