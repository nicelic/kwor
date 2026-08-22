package service

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/util/common"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type SettingsPatchRequest struct {
	ExpectedRevision           uint64            `json:"expectedRevision"`
	Changes                    map[string]string `json:"changes"`
	ConfirmTrafficHistoryClear bool              `json:"confirmTrafficHistoryClear"`
	AuditAction                string            `json:"-"`
	// ForceRevision is used for host-level settings that have no row in the
	// settings table, currently the Linux system timezone. It stays internal so
	// clients cannot bypass normal change detection.
	ForceRevision     bool `json:"-"`
	SystemTimeChanged bool `json:"-"`
}

type SettingsPatchResult struct {
	Revision          uint64   `json:"revision"`
	ChangedKeys       []string `json:"changedKeys"`
	Warnings          []string `json:"warnings,omitempty"`
	StatsCleared      bool     `json:"-"`
	MaintenanceQueued bool     `json:"maintenanceQueued,omitempty"`
	SystemTimeChanged bool     `json:"systemTimeChanged,omitempty"`
}

type SettingsSnapshotDefaults struct {
	JSONExt  string `json:"jsonExt"`
	ClashExt string `json:"clashExt"`
}

type SettingsSnapshot struct {
	Revision           uint64                          `json:"revision"`
	Values             map[string]string               `json:"values"`
	Defaults           SettingsSnapshotDefaults        `json:"defaults"`
	RuleSetSources     map[string][]RuleSetSourceEntry `json:"ruleSetSources"`
	ExtensionsIncluded bool                            `json:"extensionsIncluded"`
}

type SubscriptionSettingsSnapshot struct {
	Revision       uint64               `json:"revision"`
	Kind           string               `json:"kind"`
	Value          string               `json:"value"`
	Default        string               `json:"default"`
	RuleSetSources []RuleSetSourceEntry `json:"ruleSetSources"`
}

type SettingsRevisionConflictError struct {
	CurrentRevision uint64
}

var settingsSnapshotExcludedKeys = []string{
	"secret",
	"config",
	"mihomo_config",
	"version",
	"trafficOverviewState",
	"trafficOverviewSnapshot",
	"trafficOverviewCapState",
	"trafficOverviewPauseState",
	"trafficOverviewVnstatManifest",
	certificateCoreRestartStateSettingKey,
	certificateAutoRenewBatchStateSettingKey,
	systemLinuxDNSContentKey,
	systemLinuxDNSPathKey,
	systemLinuxDNSNameServersInputKey,
}

func (e *SettingsRevisionConflictError) Error() string {
	if e == nil {
		return "settings revision conflict"
	}
	return "settings revision conflict"
}

func IsSettingsRevisionConflict(err error) bool {
	var target *SettingsRevisionConflictError
	return errors.As(err, &target)
}

func SettingsRevisionFromConflict(err error) uint64 {
	var target *SettingsRevisionConflictError
	if errors.As(err, &target) && target != nil {
		return target.CurrentRevision
	}
	return 0
}

func (s *SettingService) GetSettingsSnapshot(includeExtensions ...bool) (*SettingsSnapshot, error) {
	includeExtensionValues := true
	if len(includeExtensions) > 0 {
		includeExtensionValues = includeExtensions[0]
	}
	// Initialize legacy/missing settings before the read transaction. The
	// snapshot itself is then loaded together with its revision in one SQLite
	// transaction, so a client can never receive old values with a new revision.
	if err := s.ensureSettingsSnapshotDefaults(); err != nil {
		return nil, err
	}
	if err := s.ensureSubscriptionInitialState(); err != nil {
		return nil, err
	}
	db := database.GetDB()
	if db == nil {
		return nil, common.NewError("database is not ready")
	}

	snapshot := &SettingsSnapshot{
		ExtensionsIncluded: includeExtensionValues,
		Defaults: SettingsSnapshotDefaults{
			JSONExt:  CanonicalSubJSONExtension(),
			ClashExt: CanonicalSubClashExtension(),
		},
		RuleSetSources: SubscriptionRuleSetSources(),
	}
	err := db.Transaction(func(tx *gorm.DB) error {
		revision, err := ensureSettingsRevisionState(tx)
		if err != nil {
			return err
		}
		excludedKeys := append([]string(nil), settingsSnapshotExcludedKeys...)
		if !includeExtensionValues {
			excludedKeys = append(excludedKeys, "subJsonExt", "subClashExt")
		}
		rows := make([]model.Setting, 0)
		if err := tx.Select("key", "value").
			Where("key NOT IN ?", excludedKeys).
			Order("id ASC").
			Find(&rows).Error; err != nil {
			return err
		}
		values := make(map[string]string, len(rows))
		for _, row := range rows {
			values[row.Key] = row.Value
		}
		snapshot.Revision = revision
		snapshot.Values = values
		return nil
	})
	if err != nil {
		return nil, err
	}
	return snapshot, nil
}

func (s *SettingService) GetSubscriptionSettingsSnapshot(kind string) (*SubscriptionSettingsSnapshot, error) {
	kind = strings.ToLower(strings.TrimSpace(kind))
	key := ""
	defaultValue := ""
	if kind == "json" {
		key = "subJsonExt"
		defaultValue = CanonicalSubJSONExtension()
	} else if kind == "clash" {
		key = "subClashExt"
		defaultValue = CanonicalSubClashExtension()
	} else {
		return nil, common.NewError("订阅扩展类型必须是 json 或 clash")
	}
	if err := s.ensureSettingsSnapshotDefaults(); err != nil {
		return nil, err
	}
	if err := s.ensureSubscriptionInitialState(); err != nil {
		return nil, err
	}
	db := database.GetDB()
	if db == nil {
		return nil, common.NewError("database is not ready")
	}
	snapshot := &SubscriptionSettingsSnapshot{
		Kind:    kind,
		Default: defaultValue,
	}
	err := db.Transaction(func(tx *gorm.DB) error {
		revision, err := ensureSettingsRevisionState(tx)
		if err != nil {
			return err
		}
		var row model.Setting
		err = tx.Where("key = ?", key).First(&row).Error
		if err != nil && !database.IsNotFound(err) {
			return err
		}
		if err == nil {
			snapshot.Value = row.Value
		}
		snapshot.Revision = revision
		return nil
	})
	if err != nil {
		return nil, err
	}
	sources := SubscriptionRuleSetSources()
	snapshot.RuleSetSources = append([]RuleSetSourceEntry(nil), sources[kind]...)
	return snapshot, nil
}

func (s *SettingService) CurrentSettingsRevision() (uint64, error) {
	db := database.GetDB()
	if db == nil {
		return 0, common.NewError("database is not ready")
	}
	var revision uint64
	err := db.Transaction(func(tx *gorm.DB) error {
		var err error
		revision, err = ensureSettingsRevisionState(tx)
		return err
	})
	return revision, err
}

func (s *SettingService) CheckSettingsRevision(expectedRevision uint64) error {
	db := database.GetDB()
	if db == nil {
		return common.NewError("database is not ready")
	}
	return db.Transaction(func(tx *gorm.DB) error {
		currentRevision, err := ensureSettingsRevisionState(tx)
		if err != nil {
			return err
		}
		if currentRevision != expectedRevision {
			return &SettingsRevisionConflictError{CurrentRevision: currentRevision}
		}
		return nil
	})
}

func ensureSettingsRevisionState(tx *gorm.DB) (uint64, error) {
	if tx == nil {
		return 0, common.NewError("database transaction is not ready")
	}
	state := model.SettingsState{Id: 1, Revision: 1}
	err := tx.Where("id = ?", state.Id).First(&state).Error
	if database.IsNotFound(err) {
		if err := tx.Create(&state).Error; err != nil {
			return 0, err
		}
		return state.Revision, nil
	}
	if err != nil {
		return 0, err
	}
	return state.Revision, nil
}

func (s *SettingService) ApplySettingsPatch(request SettingsPatchRequest, actor string) (*SettingsPatchResult, error) {
	db := database.GetDB()
	if db == nil {
		return nil, common.NewError("database is not ready")
	}

	normalized, err := normalizeSettingsPatchChanges(request.Changes)
	if err != nil {
		return nil, err
	}
	if normalized["trafficAge"] == "0" {
		if flushErr := FlushTrafficRuntimeJournal(); flushErr != nil {
			return nil, common.NewErrorf("清空历史流量前刷新运行态账本失败: %v", flushErr)
		}
	}
	result := &SettingsPatchResult{}
	err = db.Transaction(func(tx *gorm.DB) error {
		currentRevision, err := ensureSettingsRevisionState(tx)
		if err != nil {
			return err
		}
		if currentRevision != request.ExpectedRevision {
			return &SettingsRevisionConflictError{CurrentRevision: currentRevision}
		}

		current, err := loadCurrentPatchValues(tx, normalized)
		if err != nil {
			return err
		}
		changed := changedSettingsValues(current, normalized)
		if len(changed) == 0 && !request.ForceRevision {
			result.Revision = currentRevision
			result.ChangedKeys = []string{}
			return nil
		}
		if changedTrafficAgeToZero(current, changed) && !request.ConfirmTrafficHistoryClear {
			return common.NewError("将历史流量图表保留时限设为 0 会删除全部历史记录，请确认后再保存")
		}

		keys := sortedSettingsKeys(changed)
		update := tx.Model(&model.SettingsState{}).
			Where("id = ? AND revision = ?", 1, currentRevision).
			Update("revision", gorm.Expr("revision + ?", 1))
		if update.Error != nil {
			return update.Error
		}
		if update.RowsAffected != 1 {
			latest, latestErr := ensureSettingsRevisionState(tx)
			if latestErr != nil {
				return latestErr
			}
			return &SettingsRevisionConflictError{CurrentRevision: latest}
		}

		for _, key := range keys {
			if err := upsertSetting(tx, key, changed[key]); err != nil {
				return err
			}
		}
		if hasAnySettingsKey(keys, "webListen", "webPort", "subListen", "subPort") {
			if err := validatePortForwardListenerClaimsAgainstActiveRules(tx); err != nil {
				return err
			}
		}
		if changedTrafficAgeToZero(current, changed) {
			if err := tx.Where("id > 0").Delete(model.Stats{}).Error; err != nil {
				return err
			}
			result.StatsCleared = true
		}
		if len(keys) > 0 {
			action := strings.TrimSpace(request.AuditAction)
			if action == "" {
				action = "patch"
			}
			if err := recordChange(tx, model.Changes{
				DateTime: time.Now().Unix(),
				Actor:    actor,
				Key:      "settings",
				Action:   action,
				Obj:      buildSettingsPatchAudit(current, changed, keys),
			}); err != nil {
				return err
			}
		}
		if request.SystemTimeChanged {
			if err := recordChange(tx, model.Changes{
				DateTime: time.Now().Unix(),
				Actor:    actor,
				Key:      "settings",
				Action:   "system-timezone",
				Obj:      json.RawMessage(`{"changed":true}`),
			}); err != nil {
				return err
			}
		}
		result.Revision = currentRevision + 1
		result.ChangedKeys = keys
		result.SystemTimeChanged = request.SystemTimeChanged
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(result.ChangedKeys) > 0 || result.SystemTimeChanged {
		markLastUpdate(time.Now().Unix())
		if hasSettingsKey(result.ChangedKeys, "sessionMaxAge") {
			InvalidateSessionMaxAgeCache()
		}
		if hasSettingsKey(result.ChangedKeys, "trafficAge") {
			InvalidateTrafficAgeCache()
		}
		if hasSettingsKey(result.ChangedKeys, "timeLocation") {
			InvalidatePanelTimeLocationCache()
		}
		for _, key := range result.ChangedKeys {
			if isSubscriptionRenderSettingKey(key) {
				invalidateSubscriptionRuntimeSettings()
				break
			}
		}
	}
	return result, nil
}

func loadCurrentPatchValues(tx *gorm.DB, requested map[string]string) (map[string]string, error) {
	result := make(map[string]string, len(requested))
	if len(requested) == 0 {
		return result, nil
	}
	keys := make([]string, 0, len(requested))
	for key := range requested {
		keys = append(keys, key)
	}
	rows := make([]model.Setting, 0, len(keys))
	if err := tx.Where("key IN ?", keys).Find(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		result[row.Key] = row.Value
	}
	for _, key := range keys {
		if _, exists := result[key]; exists {
			continue
		}
		if key == "subPath" {
			result[key] = ""
			continue
		}
		value, err := (&SettingService{}).defaultSettingValue(key)
		if err != nil {
			return nil, err
		}
		result[key] = value
	}
	return result, nil
}

func changedSettingsValues(current map[string]string, requested map[string]string) map[string]string {
	changed := make(map[string]string)
	for key, value := range requested {
		if current[key] != value {
			changed[key] = value
		}
	}
	return changed
}

func sortedSettingsKeys(values map[string]string) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func upsertSetting(tx *gorm.DB, key string, value string) error {
	return tx.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"value"}),
	}).Create(&model.Setting{Key: key, Value: value}).Error
}

func buildSettingsPatchAudit(previous map[string]string, changed map[string]string, keys []string) json.RawMessage {
	type valueChange struct {
		Before string `json:"before"`
		After  string `json:"after"`
	}
	type largeValueChange struct {
		Before settingsLargeValueMetadata `json:"before"`
		After  settingsLargeValueMetadata `json:"after"`
	}
	payload := struct {
		ChangedKeys []string                    `json:"changedKeys"`
		Values      map[string]valueChange      `json:"values,omitempty"`
		Extensions  map[string]largeValueChange `json:"extensions,omitempty"`
	}{
		ChangedKeys: append([]string(nil), keys...),
		Values:      map[string]valueChange{},
		Extensions:  map[string]largeValueChange{},
	}
	for _, key := range keys {
		if key == "subJsonExt" || key == "subClashExt" {
			payload.Extensions[key] = largeValueChange{
				Before: newSettingsLargeValueMetadata(previous[key]),
				After:  newSettingsLargeValueMetadata(changed[key]),
			}
			continue
		}
		payload.Values[key] = valueChange{Before: previous[key], After: changed[key]}
	}
	if len(payload.Values) == 0 {
		payload.Values = nil
	}
	if len(payload.Extensions) == 0 {
		payload.Extensions = nil
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return json.RawMessage(`{"changedKeys":[]}`)
	}
	return raw
}

type settingsLargeValueMetadata struct {
	SHA256 string `json:"sha256"`
	Bytes  int    `json:"bytes"`
}

func newSettingsLargeValueMetadata(value string) settingsLargeValueMetadata {
	sum := sha256.Sum256([]byte(value))
	return settingsLargeValueMetadata{
		SHA256: hex.EncodeToString(sum[:]),
		Bytes:  len([]byte(value)),
	}
}

func normalizeSettingsPatchChanges(changes map[string]string) (map[string]string, error) {
	if changes == nil {
		return map[string]string{}, nil
	}
	result := make(map[string]string, len(changes))
	for key, value := range changes {
		if _, allowed := genericSettingsSaveKeys[key]; !allowed {
			return nil, common.NewErrorf("不允许保存设置项: %s", key)
		}
		normalized, err := normalizeGenericSettingValue(key, value)
		if err != nil {
			return nil, err
		}
		result[key] = normalized
	}
	if unit, exists := result["sessionMaxAgeUnit"]; exists {
		normalizedUnit := normalizeSessionMaxAgeUnit(unit)
		if normalizedUnit == "" {
			return nil, common.NewError("sessionMaxAgeUnit 必须是 m、h 或 d")
		}
		result["sessionMaxAgeUnit"] = normalizedUnit
	}
	if rawMinutes, exists := result["sessionMaxAge"]; exists && strings.TrimSpace(rawMinutes) == "0" {
		result["sessionMaxAge"] = strconv.Itoa(EffectiveSessionMaxAgeMinutes(0))
		result["sessionMaxAgeUnit"] = defaultSessionMaxAgeUnit
	}
	return result, nil
}

func normalizeGenericSettingValue(key string, value string) (string, error) {
	value = strings.TrimSpace(value)
	switch key {
	case "timeLocation":
		normalized, err := NormalizePanelTimeLocation(value)
		if err != nil {
			return "", err
		}
		return normalized, nil
	case "serverTlsStoreEnabled", "clientTlsStoreEnabled", "subEncode", "subShowInfo":
		parsed, err := strconv.ParseBool(value)
		if err != nil {
			return "", common.NewErrorf("%s 必须是 true 或 false", key)
		}
		return strconv.FormatBool(parsed), nil
	case "serverTlsStore", "clientTlsStore":
		normalized := normalizeCertificateStoreValue(value)
		if normalized == "" {
			normalized = "chrome"
		}
		return normalized, nil
	case "sessionMaxAgeUnit":
		normalized := normalizeSessionMaxAgeUnit(value)
		if normalized == "" {
			return "", common.NewError("sessionMaxAgeUnit 必须是 m、h 或 d")
		}
		return normalized, nil
	case "webPath":
		return normalizePanelRoutePath(value, false)
	case "subPath":
		return normalizePanelRoutePath(value, true)
	case "webPort", "subPort":
		if key == "subPort" && value == "" {
			return normalizeSubPortOrGenerate(value)
		}
		port, err := strconv.Atoi(value)
		if err != nil || port < 1 || port > 65535 {
			return "", common.NewErrorf("%s 必须在 1-65535 之间", key)
		}
		return strconv.Itoa(port), nil
	case "sessionMaxAge", "trafficAge", "subUpdates":
		return normalizeGenericSettingsNumber(key, value)
	case "subJsonExt", "subClashExt":
		return NormalizeSubscriptionExtension(key, value)
	case "webListen", "subListen":
		return normalizeListenAddress(value)
	case "webDomain", "subDomain":
		return normalizeConfiguredHost(value)
	case "webURI":
		return normalizePanelURI(value)
	case "subURI":
		if value == "" {
			return "", nil
		}
		return NormalizeSubscriptionBaseURI(value)
	default:
		return value, nil
	}
}

func changedTrafficAgeToZero(previous map[string]string, changed map[string]string) bool {
	value, exists := changed["trafficAge"]
	return exists && value == "0" && previous["trafficAge"] != "0"
}

func hasSettingsKey(keys []string, expected string) bool {
	for _, key := range keys {
		if key == expected {
			return true
		}
	}
	return false
}

func (s *SettingService) saveEditableSettingDirect(key string, value string) error {
	if _, allowed := genericSettingsSaveKeys[key]; !allowed {
		return s.saveSetting(key, value)
	}
	normalized, err := normalizeGenericSettingValue(key, value)
	if err != nil {
		return err
	}
	if key == "trafficAge" && normalized == "0" {
		if flushErr := FlushTrafficRuntimeJournal(); flushErr != nil {
			return common.NewErrorf("清空历史流量前刷新运行态账本失败: %v", flushErr)
		}
	}
	db := database.GetDB()
	if db == nil {
		return common.NewError("database is not ready")
	}
	changed := false
	err = db.Transaction(func(tx *gorm.DB) error {
		currentRevision, err := ensureSettingsRevisionState(tx)
		if err != nil {
			return err
		}
		var current model.Setting
		err = tx.Where("key = ?", key).First(&current).Error
		if err == nil && current.Value == normalized {
			return nil
		}
		if err != nil && !database.IsNotFound(err) {
			return err
		}
		if err := upsertSetting(tx, key, normalized); err != nil {
			return err
		}
		if isPortForwardListenerSettingKey(key) {
			if err := validatePortForwardListenerClaimsAgainstActiveRules(tx); err != nil {
				return err
			}
		}
		if changedTrafficAgeToZero(map[string]string{"trafficAge": current.Value}, map[string]string{key: normalized}) {
			if err := tx.Where("id > 0").Delete(model.Stats{}).Error; err != nil {
				return err
			}
		}
		updated := tx.Model(&model.SettingsState{}).
			Where("id = ? AND revision = ?", 1, currentRevision).
			Update("revision", gorm.Expr("revision + ?", 1))
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return common.NewError("设置版本更新冲突")
		}
		changed = true
		return nil
	})
	if err != nil {
		return err
	}
	if changed {
		markLastUpdate(time.Now().Unix())
		if key == "sessionMaxAge" {
			InvalidateSessionMaxAgeCache()
		}
		if key == "trafficAge" {
			InvalidateTrafficAgeCache()
		}
		if key == "timeLocation" {
			InvalidatePanelTimeLocationCache()
		}
		if isSubscriptionRuntimeSettingsKey(key) {
			invalidateSubscriptionRuntimeSettings()
		}
	}
	return nil
}

func (s *ConfigService) SaveSettingsPatch(request SettingsPatchRequest, actor string) (*SettingsPatchResult, error) {
	result, err := s.SettingService.ApplySettingsPatch(request, actor)
	if err != nil || result == nil || len(result.ChangedKeys) == 0 {
		return result, err
	}
	s.ApplySettingsPatchPostCommit(result)
	return result, nil
}

func (s *ConfigService) ApplySettingsPatchPostCommit(result *SettingsPatchResult) {
	if result == nil || len(result.ChangedKeys) == 0 {
		return
	}
	warnings := make([]string, 0)
	if hasAnySettingsKey(result.ChangedKeys, "serverTlsStoreEnabled", "serverTlsStore") {
		if err := GetProManagerService(s).RegenerateCoreConfig(); err != nil {
			warnings = append(warnings, fmt.Sprintf("核心配置重建失败: %v", err))
		}
	}
	if hasAnySettingsKey(result.ChangedKeys, "webListen", "webPort", "subListen", "subPort") {
		if err := (&FirewallService{}).SyncIfNeeded(0); err != nil {
			warnings = append(warnings, fmt.Sprintf("防火墙同步失败: %v", err))
		}
	}
	if hasSettingsKey(result.ChangedKeys, "timeLocation") {
		if err := ReloadPanelTimeSchedule(); err != nil {
			warnings = append(warnings, fmt.Sprintf("时区计划重建失败: %v", err))
		}
		if err := ReschedulePendingCoreAutoChecksForPanelTimeZone(); err != nil {
			warnings = append(warnings, fmt.Sprintf("内核首次检查计划重排失败: %v", err))
		}
	}
	if result.StatsCleared {
		result.MaintenanceQueued = requestMainSQLiteCompaction(database.GetDB(), true)
	}
	result.Warnings = warnings
}

func hasAnySettingsKey(keys []string, expected ...string) bool {
	for _, key := range keys {
		for _, candidate := range expected {
			if key == candidate {
				return true
			}
		}
	}
	return false
}
