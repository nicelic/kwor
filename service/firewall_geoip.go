package service

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/alireza0/s-ui/config"
	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/util/common"
	"go4.org/netipx"
)

const (
	firewallGeoUpdateIntervalMinutesKey = "firewallGeoUpdateIntervalMinutes"
	firewallGeoLastRefreshAtKey         = "firewallGeoLastRefreshAt"

	firewallGeoHTTPTimeout = 45 * time.Second
)

var firewallGeoState = struct {
	loaded map[uint]firewallGeoResolvedPrefixes
}{
	loaded: make(map[uint]firewallGeoResolvedPrefixes),
}

type FirewallGeoRulePayload struct {
	ID               uint     `json:"id"`
	Name             string   `json:"name"`
	Description      string   `json:"description"`
	Family           string   `json:"family"`
	Protocol         string   `json:"protocol"`
	PortSpec         string   `json:"portSpec"`
	Action           string   `json:"action"`
	CountryCode      string   `json:"countryCode"`
	SourceProviders  []string `json:"sourceProviders"`
	CustomSourceURLs string   `json:"customSourceUrls"`
}

type FirewallGeoRuleView struct {
	ID               uint      `json:"id"`
	Name             string    `json:"name"`
	Description      string    `json:"description"`
	Enabled          bool      `json:"enabled"`
	Family           string    `json:"family"`
	Protocol         string    `json:"protocol"`
	PortSpec         string    `json:"portSpec"`
	Action           string    `json:"action"`
	CountryCode      string    `json:"countryCode"`
	SourceProviders  []string  `json:"sourceProviders"`
	CustomSourceURLs []string  `json:"customSourceUrls"`
	ResolvedSources  []string  `json:"resolvedSources"`
	CachedFiles      []string  `json:"cachedFiles"`
	ContentHash      string    `json:"contentHash"`
	PrefixCount      int       `json:"prefixCount"`
	LastRefreshAt    int64     `json:"lastRefreshAt"`
	LastRefreshError string    `json:"lastRefreshError"`
	UpdatedAt        time.Time `json:"updatedAt"`
	CreatedAt        time.Time `json:"createdAt"`
}

type firewallGeoDownloadedSource struct {
	URL    string
	Format string
	Body   []byte
	Parsed firewallGeoResolvedPrefixes
}

type firewallGeoRuleRefreshResult struct {
	Sources []firewallGeoDownloadedSource
	Merged  firewallGeoResolvedPrefixes
}

func loadFirewallGeoRulesLocked() ([]model.FirewallGeoRule, error) {
	db := database.GetDB()
	rows := make([]model.FirewallGeoRule, 0)
	if err := db.Order("id asc").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func buildFirewallGeoRuleView(row model.FirewallGeoRule) FirewallGeoRuleView {
	return FirewallGeoRuleView{
		ID:               row.Id,
		Name:             row.Name,
		Description:      row.Description,
		Enabled:          row.Enabled,
		Family:           row.Family,
		Protocol:         row.Protocol,
		PortSpec:         row.PortSpec,
		Action:           row.Action,
		CountryCode:      row.CountryCode,
		SourceProviders:  decodeFirewallGeoStringList(row.SourceProviders),
		CustomSourceURLs: decodeFirewallGeoStringList(row.CustomSourceURLs),
		ResolvedSources:  decodeFirewallGeoStringList(row.ResolvedSources),
		CachedFiles:      decodeFirewallGeoStringList(row.CachedFiles),
		ContentHash:      row.ContentHash,
		PrefixCount:      row.PrefixCount,
		LastRefreshAt:    row.LastRefreshAt,
		LastRefreshError: row.LastRefreshError,
		UpdatedAt:        row.UpdatedAt,
		CreatedAt:        row.CreatedAt,
	}
}

func normalizeFirewallGeoAction(raw string) string {
	switch strings.TrimSpace(strings.ToLower(raw)) {
	case firewallGeoRuleActionAllow:
		return firewallGeoRuleActionAllow
	default:
		return firewallGeoRuleActionBlock
	}
}

func normalizeFirewallGeoProtocol(raw string) (string, error) {
	protocol := normalizeFirewallProtocol(raw)
	if protocol == firewallProtocolAny {
		return "", common.NewError("geoip firewall rules do not support ANY protocol")
	}
	if firewallProtocolIsICMP(protocol) {
		return "", common.NewError("geoip firewall rules do not support ICMP protocols")
	}
	return protocol, nil
}

func parseFirewallGeoCustomSourceURLs(raw string) ([]string, error) {
	raw = strings.ReplaceAll(raw, "\r\n", "\n")
	segments := make([]string, 0)
	for _, line := range strings.Split(raw, "\n") {
		for _, part := range strings.Split(line, ",") {
			value := strings.TrimSpace(part)
			if value == "" {
				continue
			}
			segments = append(segments, value)
		}
	}
	if len(segments) == 0 {
		return nil, nil
	}
	result := make([]string, 0, len(segments))
	seen := make(map[string]struct{}, len(segments))
	for _, item := range segments {
		value := strings.TrimSpace(item)
		if value == "" {
			continue
		}
		if !strings.HasPrefix(value, "http://") && !strings.HasPrefix(value, "https://") {
			return nil, fmt.Errorf("custom geo source must start with http:// or https://: %s", value)
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result, nil
}

func buildFirewallGeoRuleDefaultName(action string, countryCode string, portSpec string) string {
	actionLabel := "阻断"
	if action == firewallGeoRuleActionAllow {
		actionLabel = "放行"
	}
	countryLabel := strings.ToUpper(strings.TrimSpace(countryCode))
	if countryLabel == "" {
		countryLabel = "自定义来源"
	}
	return fmt.Sprintf("%s %s (%s)", actionLabel, countryLabel, portSpec)
}

func encodeFirewallGeoStringList(items []string) string {
	if len(items) == 0 {
		return "[]"
	}
	data, err := json.Marshal(items)
	if err != nil {
		return "[]"
	}
	return string(data)
}

func decodeFirewallGeoStringList(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []string{}
	}
	var result []string
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return []string{}
	}
	cleaned := make([]string, 0, len(result))
	seen := make(map[string]struct{}, len(result))
	for _, item := range result {
		value := strings.TrimSpace(item)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		cleaned = append(cleaned, value)
	}
	return cleaned
}

func firewallGeoDir() string {
	return filepath.Join(config.GetDataDir(), "geoip")
}

func ensureFirewallGeoDir() error {
	return os.MkdirAll(firewallGeoDir(), 0o755)
}

func firewallGeoFinalFileName(ruleID uint, index int, sourceURL string) string {
	raw := strings.TrimSpace(sourceURL)
	if parsedURL, err := url.Parse(raw); err == nil && parsedURL.Path != "" {
		raw = parsedURL.Path
	}
	ext := path.Ext(raw)
	if ext == "" {
		switch detectFirewallGeoFormat(sourceURL) {
		case firewallGeoFormatSRS:
			ext = ".srs"
		case firewallGeoFormatMRS:
			ext = ".mrs"
		case firewallGeoFormatJSON:
			ext = ".json"
		default:
			ext = ".txt"
		}
	}
	return fmt.Sprintf("rule_%d_%02d%s", ruleID, index, ext)
}

func firewallGeoFilePath(fileName string) string {
	return filepath.Join(firewallGeoDir(), fileName)
}

func hasFirewallGeoCache(row model.FirewallGeoRule) bool {
	return strings.TrimSpace(row.ContentHash) != "" && len(decodeFirewallGeoStringList(row.CachedFiles)) > 0
}

func mergeFirewallGeoResolvedSets(items []firewallGeoResolvedPrefixes) (firewallGeoResolvedPrefixes, error) {
	var builder netipx.IPSetBuilder
	for _, item := range items {
		for _, prefix := range item.All {
			if err := addFirewallGeoPrefixString(&builder, prefix); err != nil {
				return firewallGeoResolvedPrefixes{}, err
			}
		}
	}
	return buildFirewallGeoResolvedPrefixes(&builder)
}

func downloadFirewallGeoSource(sourceURL string, cache map[string]firewallGeoDownloadedSource) (firewallGeoDownloadedSource, error) {
	if cached, exists := cache[sourceURL]; exists {
		return cached, nil
	}
	client := &http.Client{Timeout: firewallGeoHTTPTimeout}
	req, err := http.NewRequest(http.MethodGet, sourceURL, nil)
	if err != nil {
		return firewallGeoDownloadedSource{}, err
	}
	req.Header.Set("User-Agent", "kwor-firewall-geoip/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return firewallGeoDownloadedSource{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return firewallGeoDownloadedSource{}, fmt.Errorf("http status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return firewallGeoDownloadedSource{}, err
	}
	parsed, err := parseFirewallGeoRuleBytes(sourceURL, body)
	if err != nil {
		return firewallGeoDownloadedSource{}, err
	}
	result := firewallGeoDownloadedSource{
		URL:    sourceURL,
		Format: detectFirewallGeoFormat(sourceURL),
		Body:   body,
		Parsed: parsed,
	}
	cache[sourceURL] = result
	return result, nil
}

func resolveFirewallGeoRuleSources(row model.FirewallGeoRule, cache map[string]firewallGeoDownloadedSource) (firewallGeoRuleRefreshResult, error) {
	customURLs := decodeFirewallGeoStringList(row.CustomSourceURLs)
	if len(customURLs) > 0 {
		sources := make([]firewallGeoDownloadedSource, 0, len(customURLs))
		parsed := make([]firewallGeoResolvedPrefixes, 0, len(customURLs))
		for _, sourceURL := range customURLs {
			source, err := downloadFirewallGeoSource(sourceURL, cache)
			if err != nil {
				return firewallGeoRuleRefreshResult{}, fmt.Errorf("custom source %s: %w", sourceURL, err)
			}
			sources = append(sources, source)
			parsed = append(parsed, source.Parsed)
		}
		merged, err := mergeFirewallGeoResolvedSets(parsed)
		if err != nil {
			return firewallGeoRuleRefreshResult{}, err
		}
		return firewallGeoRuleRefreshResult{
			Sources: sources,
			Merged:  merged,
		}, nil
	}

	countryCode := normalizeFirewallGeoCountryCode(row.CountryCode)
	if countryCode == "" {
		return firewallGeoRuleRefreshResult{}, fmt.Errorf("country code is required when custom URLs are empty")
	}
	providers := decodeFirewallGeoStringList(row.SourceProviders)
	if len(providers) == 0 {
		providers = firewallGeoDefaultSourceKeys()
	}

	attemptErrors := make([]string, 0, len(providers))
	for _, provider := range providers {
		sourceURL, _, err := buildFirewallGeoProviderURL(provider, countryCode)
		if err != nil {
			attemptErrors = append(attemptErrors, err.Error())
			continue
		}
		source, err := downloadFirewallGeoSource(sourceURL, cache)
		if err != nil {
			attemptErrors = append(attemptErrors, fmt.Sprintf("%s: %v", provider, err))
			continue
		}
		return firewallGeoRuleRefreshResult{
			Sources: []firewallGeoDownloadedSource{source},
			Merged:  source.Parsed,
		}, nil
	}
	return firewallGeoRuleRefreshResult{}, fmt.Errorf("no usable geo source for %s (%s)", countryCode, strings.Join(attemptErrors, "; "))
}

func applyFirewallGeoRuleRefreshResultLocked(row *model.FirewallGeoRule, result firewallGeoRuleRefreshResult, refreshedAt int64) error {
	if row == nil {
		return fmt.Errorf("nil geo rule")
	}
	if row.Id == 0 {
		return fmt.Errorf("geo rule id is required")
	}
	if err := ensureFirewallGeoDir(); err != nil {
		return err
	}

	finalNames := make([]string, 0, len(result.Sources))
	tempPaths := make([]string, 0, len(result.Sources))
	for index, source := range result.Sources {
		finalName := firewallGeoFinalFileName(row.Id, index, source.URL)
		finalPath := firewallGeoFilePath(finalName)
		tempPath := finalPath + ".tmp." + strconv.FormatInt(time.Now().UnixNano(), 10)
		if err := os.WriteFile(tempPath, source.Body, 0o644); err != nil {
			for _, temp := range tempPaths {
				_ = os.Remove(temp)
			}
			return err
		}
		finalNames = append(finalNames, finalName)
		tempPaths = append(tempPaths, tempPath)
	}

	for index, tempPath := range tempPaths {
		finalPath := firewallGeoFilePath(finalNames[index])
		_ = os.Remove(finalPath)
		if err := os.Rename(tempPath, finalPath); err != nil {
			for _, temp := range tempPaths {
				_ = os.Remove(temp)
			}
			return err
		}
	}

	oldFiles := decodeFirewallGeoStringList(row.CachedFiles)
	keep := make(map[string]struct{}, len(finalNames))
	for _, name := range finalNames {
		keep[name] = struct{}{}
	}
	for _, oldFile := range oldFiles {
		if _, exists := keep[oldFile]; exists {
			continue
		}
		_ = os.Remove(firewallGeoFilePath(oldFile))
	}

	resolvedSources := make([]string, 0, len(result.Sources))
	for _, source := range result.Sources {
		resolvedSources = append(resolvedSources, source.URL)
	}

	row.ResolvedSources = encodeFirewallGeoStringList(resolvedSources)
	row.CachedFiles = encodeFirewallGeoStringList(finalNames)
	row.ContentHash = result.Merged.ContentHash
	row.PrefixCount = result.Merged.PrefixCount
	row.LastRefreshAt = refreshedAt
	row.LastRefreshError = ""

	if err := database.GetDB().Save(row).Error; err != nil {
		return err
	}
	firewallGeoState.loaded[row.Id] = result.Merged
	return nil
}

func markFirewallGeoRuleRefreshErrorLocked(row *model.FirewallGeoRule, err error) error {
	if row == nil {
		return nil
	}
	row.LastRefreshError = strings.TrimSpace(err.Error())
	return database.GetDB().Save(row).Error
}

func cleanupFirewallGeoRuleFiles(ruleID uint) {
	if ruleID == 0 {
		return
	}
	matches, _ := filepath.Glob(filepath.Join(firewallGeoDir(), fmt.Sprintf("rule_%d_*", ruleID)))
	for _, match := range matches {
		_ = os.Remove(match)
	}
}

func loadFirewallGeoRuleCachedPrefixes(row model.FirewallGeoRule) (firewallGeoResolvedPrefixes, error) {
	files := decodeFirewallGeoStringList(row.CachedFiles)
	if len(files) == 0 {
		return firewallGeoResolvedPrefixes{}, fmt.Errorf("geo rule has no cached files")
	}

	parsed := make([]firewallGeoResolvedPrefixes, 0, len(files))
	for _, fileName := range files {
		body, err := os.ReadFile(firewallGeoFilePath(fileName))
		if err != nil {
			return firewallGeoResolvedPrefixes{}, err
		}
		result, err := parseFirewallGeoRuleBytes(fileName, body)
		if err != nil {
			return firewallGeoResolvedPrefixes{}, err
		}
		parsed = append(parsed, result)
	}

	merged, err := mergeFirewallGeoResolvedSets(parsed)
	if err != nil {
		return firewallGeoResolvedPrefixes{}, err
	}
	if row.ContentHash != "" && merged.ContentHash != row.ContentHash {
		return firewallGeoResolvedPrefixes{}, fmt.Errorf("geo cache content hash mismatch")
	}
	return merged, nil
}

func ensureFirewallGeoRuntimeLoadedLocked(rows []model.FirewallGeoRule) error {
	if len(rows) == 0 {
		firewallGeoState.loaded = make(map[uint]firewallGeoResolvedPrefixes)
		return nil
	}

	loaded := make(map[uint]firewallGeoResolvedPrefixes, len(rows))
	for _, row := range rows {
		if cached, exists := firewallGeoState.loaded[row.Id]; exists && cached.ContentHash == row.ContentHash {
			loaded[row.Id] = cached
			continue
		}
		result, err := loadFirewallGeoRuleCachedPrefixes(row)
		if err != nil {
			return fmt.Errorf("load geo cache for rule %d failed: %w", row.Id, err)
		}
		loaded[row.Id] = result
	}
	firewallGeoState.loaded = loaded
	return nil
}

func (s *FirewallService) getFirewallGeoUpdateIntervalMinutesLocked() (int, error) {
	value, err := s.getInt(firewallGeoUpdateIntervalMinutesKey)
	if err != nil {
		return 0, err
	}
	if value <= 0 {
		value = 360
	}
	return value, nil
}

func (s *FirewallService) setFirewallGeoUpdateIntervalMinutesLocked(intervalMinutes int) error {
	if intervalMinutes <= 0 {
		intervalMinutes = 360
	}
	return s.setInt(firewallGeoUpdateIntervalMinutesKey, intervalMinutes)
}

func (s *FirewallService) getFirewallGeoLastRefreshAtLocked() (int64, error) {
	raw, err := s.getString(firewallGeoLastRefreshAtKey)
	if err != nil {
		return 0, err
	}
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	value, parseErr := strconv.ParseInt(raw, 10, 64)
	if parseErr != nil {
		return 0, nil
	}
	return value, nil
}

func (s *FirewallService) setFirewallGeoLastRefreshAtLocked(timestamp int64) error {
	return s.setString(firewallGeoLastRefreshAtKey, strconv.FormatInt(timestamp, 10))
}

func (s *FirewallService) syncFirewallGeoCacheLocked(forceRefresh bool) error {
	rows, err := loadFirewallGeoRulesLocked()
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}

	strictRefresh := firewallGeoRowsNeedStrictRefresh(rows)
	if !strictRefresh && !forceRefresh && !s.shouldAutoRefreshFirewallGeoLocked() {
		return nil
	}
	return s.refreshFirewallGeoRulesLocked(rows, strictRefresh)
}

func (s *FirewallService) shouldAutoRefreshFirewallGeoLocked() bool {
	intervalMinutes, err := s.getFirewallGeoUpdateIntervalMinutesLocked()
	if err != nil || intervalMinutes <= 0 {
		intervalMinutes = 360
	}
	lastRefreshAt, err := s.getFirewallGeoLastRefreshAtLocked()
	if err != nil || lastRefreshAt <= 0 {
		return true
	}
	return time.Now().Unix() >= lastRefreshAt+int64(intervalMinutes)*60
}

func (s *FirewallService) refreshFirewallGeoRulesLocked(rows []model.FirewallGeoRule, strict bool) error {
	if len(rows) == 0 {
		firewallGeoState.loaded = make(map[uint]firewallGeoResolvedPrefixes)
		return nil
	}

	cache := make(map[string]firewallGeoDownloadedSource)
	now := time.Now().Unix()
	for index := range rows {
		row := &rows[index]
		result, err := resolveFirewallGeoRuleSources(*row, cache)
		if err != nil {
			if strict || !hasFirewallGeoCache(*row) {
				return fmt.Errorf("refresh geo rule %s failed: %w", row.Name, err)
			}
			if saveErr := markFirewallGeoRuleRefreshErrorLocked(row, err); saveErr != nil {
				return saveErr
			}
			continue
		}
		if err := applyFirewallGeoRuleRefreshResultLocked(row, result, now); err != nil {
			return err
		}
	}
	return s.setFirewallGeoLastRefreshAtLocked(now)
}

func firewallGeoRowsNeedStrictRefresh(rows []model.FirewallGeoRule) bool {
	for _, row := range rows {
		if !hasFirewallGeoCache(row) {
			return true
		}
	}
	return false
}

func (s *FirewallService) prepareFirewallGeoRulesLocked(forceRefresh bool) ([]model.FirewallGeoRule, error) {
	rows, err := loadFirewallGeoRulesLocked()
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		firewallGeoState.loaded = make(map[uint]firewallGeoResolvedPrefixes)
		return rows, nil
	}

	if firewallGeoRowsNeedStrictRefresh(rows) {
		if err := s.refreshFirewallGeoRulesLocked(rows, true); err != nil {
			return nil, err
		}
		rows, err = loadFirewallGeoRulesLocked()
		if err != nil {
			return nil, err
		}
	}

	if err := ensureFirewallGeoRuntimeLoadedLocked(rows); err != nil {
		return nil, err
	}

	if forceRefresh || s.shouldAutoRefreshFirewallGeoLocked() {
		if err := s.refreshFirewallGeoRulesLocked(rows, false); err != nil {
			return nil, err
		}
		rows, err = loadFirewallGeoRulesLocked()
		if err != nil {
			return nil, err
		}
		if err := ensureFirewallGeoRuntimeLoadedLocked(rows); err != nil {
			return nil, err
		}
	}

	return rows, nil
}

func (s *FirewallService) UpsertGeoRule(payload FirewallGeoRulePayload) error {
	firewallStateMu.Lock()
	defer firewallStateMu.Unlock()

	db := database.GetDB()
	var (
		row      model.FirewallGeoRule
		previous *model.FirewallGeoRule
	)
	if payload.ID > 0 {
		if err := db.Where("id = ?", payload.ID).First(&row).Error; err != nil {
			return err
		}
		copyRow := row
		previous = &copyRow
	} else {
		row = model.FirewallGeoRule{
			Enabled: true,
		}
	}

	name := strings.TrimSpace(payload.Name)
	description := strings.TrimSpace(payload.Description)
	family := normalizeFirewallFamily(payload.Family)
	protocol, err := normalizeFirewallGeoProtocol(payload.Protocol)
	if err != nil {
		return err
	}
	portSpec, err := normalizeFirewallPortSpec(payload.PortSpec, protocol)
	if err != nil {
		return err
	}
	action := normalizeFirewallGeoAction(payload.Action)
	countryCode := normalizeFirewallGeoCountryCode(payload.CountryCode)
	sourceProviders := normalizeFirewallGeoProviderKeys(payload.SourceProviders)
	customURLs, err := parseFirewallGeoCustomSourceURLs(payload.CustomSourceURLs)
	if err != nil {
		return err
	}
	if len(customURLs) == 0 && countryCode == "" {
		return common.NewError("geoip firewall rule requires a country code or at least one custom ruleset url")
	}
	if name == "" {
		name = buildFirewallGeoRuleDefaultName(action, countryCode, portSpec)
	}

	row.Name = name
	row.Description = description
	row.Enabled = true
	row.Family = family
	row.Protocol = protocol
	row.PortSpec = portSpec
	row.Action = action
	row.CountryCode = countryCode
	row.SourceProviders = encodeFirewallGeoStringList(sourceProviders)
	row.CustomSourceURLs = encodeFirewallGeoStringList(customURLs)

	if payload.ID > 0 {
		if err := db.Save(&row).Error; err != nil {
			return err
		}
	} else {
		if err := db.Create(&row).Error; err != nil {
			return err
		}
	}

	if err := s.refreshFirewallGeoRulesLocked([]model.FirewallGeoRule{row}, true); err != nil {
		if previous != nil {
			_ = db.Save(previous).Error
		} else {
			_ = db.Delete(&row).Error
			cleanupFirewallGeoRuleFiles(row.Id)
		}
		return err
	}

	enabled, enabledErr := s.getFirewallEnabledLocked()
	if enabledErr == nil && enabled && firewallSupported() {
		return s.reconcileLocked(0)
	}
	return nil
}

func (s *FirewallService) DeleteGeoRule(id uint) error {
	firewallStateMu.Lock()
	defer firewallStateMu.Unlock()

	db := database.GetDB()
	var row model.FirewallGeoRule
	if err := db.Where("id = ?", id).First(&row).Error; err != nil {
		return err
	}
	if err := db.Delete(&row).Error; err != nil {
		return err
	}

	delete(firewallGeoState.loaded, id)
	cleanupFirewallGeoRuleFiles(id)

	enabled, enabledErr := s.getFirewallEnabledLocked()
	if enabledErr == nil && enabled && firewallSupported() {
		return s.reconcileLocked(0)
	}
	return nil
}

func (s *FirewallService) RefreshGeoRules() error {
	firewallStateMu.Lock()
	defer firewallStateMu.Unlock()

	rows, err := loadFirewallGeoRulesLocked()
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		firewallGeoState.loaded = make(map[uint]firewallGeoResolvedPrefixes)
		return s.setFirewallGeoLastRefreshAtLocked(0)
	}
	if err := s.refreshFirewallGeoRulesLocked(rows, false); err != nil {
		return err
	}
	if _, err := s.prepareFirewallGeoRulesLocked(false); err != nil {
		return err
	}

	enabled, enabledErr := s.getFirewallEnabledLocked()
	if enabledErr == nil && enabled && firewallSupported() {
		return s.reconcileLocked(0)
	}
	return nil
}

func (s *FirewallService) SaveGeoSettings(intervalMinutes int) error {
	firewallStateMu.Lock()
	defer firewallStateMu.Unlock()
	return s.setFirewallGeoUpdateIntervalMinutesLocked(intervalMinutes)
}

func currentFirewallGeoRuntime(ruleID uint) (firewallGeoResolvedPrefixes, bool) {
	result, exists := firewallGeoState.loaded[ruleID]
	return result, exists
}

func firewallGeoRuleSort(rows []model.FirewallGeoRule) {
	sort.SliceStable(rows, func(i, j int) bool {
		return rows[i].Id < rows[j].Id
	})
}
