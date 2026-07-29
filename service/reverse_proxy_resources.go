package service

import (
	"errors"
	"fmt"
	"sync"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/util/common"
	"gorm.io/gorm"
)

const (
	reverseProxySettingsRecordID uint64 = model.ReverseProxySettingsSingletonID

	reverseProxyMinimumRewriteReservationBytes int64 = 500 * 1024

	reverseProxyMaximumConfiguredLimit         = 1000000
	reverseProxyMaximumConfiguredStreams int64 = 65535
	reverseProxyMaximumMemoryPoolBytes   int64 = 64 * 1024 * 1024 * 1024
)

var errReverseProxyRevisionConflict = errors.New("reverse proxy configuration revision conflict")

func IsReverseProxyRevisionConflict(err error) bool {
	return errors.Is(err, errReverseProxyRevisionConflict)
}

func (s *ReverseProxyService) CurrentRevision() (uint64, error) {
	return s.peekReverseProxyRevision()
}

// ReverseProxyResourceSettings is deliberately separate from the GORM model:
// it is the immutable public/runtime shape and never exposes timestamps or
// database implementation details.
type ReverseProxyResourceSettings struct {
	ListenerConnectionLimit           int    `json:"listenerConnectionLimit"`
	GlobalHTTPMaxConcurrent           int    `json:"globalHttpMaxConcurrent"`
	GlobalDNSMaxConcurrent            int    `json:"globalDnsMaxConcurrent"`
	HTTP2MaxConcurrentStreams         uint32 `json:"http2MaxConcurrentStreams"`
	QUICMaxIncomingStreams            int64  `json:"quicMaxIncomingStreams"`
	DefaultUpstreamMaxIdleConnections int    `json:"defaultUpstreamMaxIdleConnections"`
	MemoryPoolBytes                   int64  `json:"memoryPoolBytes"`
	DefaultRuleMemoryLimitBytes       int64  `json:"defaultRuleMemoryLimitBytes"`
	ResponseRewriteInputBytes         int64  `json:"responseRewriteInputBytes"`
	ResponseRewriteOutputBytes        int64  `json:"responseRewriteOutputBytes"`
	ResponseRewriteMaxConcurrent      int    `json:"responseRewriteMaxConcurrent"`
}

type ReverseProxySettingsPayload struct {
	ExpectedRevision *uint64 `json:"expectedRevision"`
	ReverseProxyResourceSettings
}

type ReverseProxyRuntimeRuleStateView struct {
	ID                      uint   `json:"id"`
	RuntimeStatus           string `json:"runtimeStatus"`
	LastError               string `json:"lastError"`
	LocalConnectionCount    int    `json:"localConnectionCount"`
	UpstreamConnectionCount int    `json:"upstreamConnectionCount"`
}

type ReverseProxyRuntimeResourceUsage struct {
	ActiveHTTPRequests int   `json:"activeHttpRequests"`
	ActiveDNSQueries   int   `json:"activeDnsQueries"`
	MemoryUsedBytes    int64 `json:"memoryUsedBytes"`
	CacheUsedBytes     int64 `json:"cacheUsedBytes"`
	RewriteUsedBytes   int64 `json:"rewriteUsedBytes"`
}

type ReverseProxyRuntimeOverview struct {
	Revision      uint64                             `json:"revision"`
	Available     bool                               `json:"available"`
	Started       bool                               `json:"started"`
	ListenerCount int                                `json:"listenerCount"`
	LastSyncAt    int64                              `json:"lastSyncAt"`
	Rules         []ReverseProxyRuntimeRuleStateView `json:"rules"`
	Resources     ReverseProxyRuntimeResourceUsage   `json:"resources"`
	Warnings      []string                           `json:"warnings,omitempty"`
	Error         string                             `json:"error,omitempty"`
}

func defaultReverseProxySettingsModel() model.ReverseProxySettings {
	return model.DefaultReverseProxySettings()
}

func reverseProxySettingsView(row *model.ReverseProxySettings) ReverseProxyResourceSettings {
	if row == nil {
		row = ptrReverseProxySettings(defaultReverseProxySettingsModel())
	}
	return ReverseProxyResourceSettings{
		ListenerConnectionLimit:           row.ListenerConnectionLimit,
		GlobalHTTPMaxConcurrent:           row.GlobalHTTPMaxConcurrent,
		GlobalDNSMaxConcurrent:            row.GlobalDNSMaxConcurrent,
		HTTP2MaxConcurrentStreams:         row.HTTP2MaxConcurrentStreams,
		QUICMaxIncomingStreams:            row.QUICMaxIncomingStreams,
		DefaultUpstreamMaxIdleConnections: row.DefaultUpstreamMaxIdleConnections,
		MemoryPoolBytes:                   row.MemoryPoolBytes,
		DefaultRuleMemoryLimitBytes:       row.DefaultRuleMemoryLimitBytes,
		ResponseRewriteInputBytes:         row.ResponseRewriteInputBytes,
		ResponseRewriteOutputBytes:        row.ResponseRewriteOutputBytes,
		ResponseRewriteMaxConcurrent:      row.ResponseRewriteMaxConcurrent,
	}
}

func ptrReverseProxySettings(value model.ReverseProxySettings) *model.ReverseProxySettings {
	return &value
}

func normalizeReverseProxySettingsModel(row *model.ReverseProxySettings) {
	if row == nil {
		return
	}
	defaults := defaultReverseProxySettingsModel()
	if row.Revision == 0 {
		row.Revision = defaults.Revision
	}
	if row.ListenerConnectionLimit < 0 {
		row.ListenerConnectionLimit = defaults.ListenerConnectionLimit
	}
	if row.GlobalHTTPMaxConcurrent < 0 {
		row.GlobalHTTPMaxConcurrent = defaults.GlobalHTTPMaxConcurrent
	}
	if row.GlobalDNSMaxConcurrent < 0 {
		row.GlobalDNSMaxConcurrent = defaults.GlobalDNSMaxConcurrent
	}
	if row.HTTP2MaxConcurrentStreams == 0 {
		row.HTTP2MaxConcurrentStreams = defaults.HTTP2MaxConcurrentStreams
	}
	if row.QUICMaxIncomingStreams <= 0 {
		row.QUICMaxIncomingStreams = defaults.QUICMaxIncomingStreams
	}
	if row.DefaultUpstreamMaxIdleConnections < 0 {
		row.DefaultUpstreamMaxIdleConnections = defaults.DefaultUpstreamMaxIdleConnections
	}
	if row.MemoryPoolBytes <= 0 {
		row.MemoryPoolBytes = defaults.MemoryPoolBytes
	}
	if row.DefaultRuleMemoryLimitBytes <= 0 {
		row.DefaultRuleMemoryLimitBytes = defaults.DefaultRuleMemoryLimitBytes
	}
	if row.ResponseRewriteInputBytes <= 0 {
		row.ResponseRewriteInputBytes = defaults.ResponseRewriteInputBytes
	}
	if row.ResponseRewriteOutputBytes <= 0 {
		row.ResponseRewriteOutputBytes = defaults.ResponseRewriteOutputBytes
	}
	if row.ResponseRewriteMaxConcurrent <= 0 {
		row.ResponseRewriteMaxConcurrent = defaults.ResponseRewriteMaxConcurrent
	}
}

func validateReverseProxyResourceSettings(value ReverseProxyResourceSettings) error {
	if value.ListenerConnectionLimit < 0 || value.ListenerConnectionLimit > reverseProxyMaximumConfiguredLimit {
		return common.NewError("listener connection limit must be between 0 and ", reverseProxyMaximumConfiguredLimit)
	}
	if value.GlobalHTTPMaxConcurrent < 0 || value.GlobalHTTPMaxConcurrent > reverseProxyMaximumConfiguredLimit {
		return common.NewError("global http concurrency must be between 0 and ", reverseProxyMaximumConfiguredLimit)
	}
	if value.GlobalDNSMaxConcurrent < 0 || value.GlobalDNSMaxConcurrent > reverseProxyMaximumConfiguredLimit {
		return common.NewError("global dns concurrency must be between 0 and ", reverseProxyMaximumConfiguredLimit)
	}
	if value.HTTP2MaxConcurrentStreams < 1 || int64(value.HTTP2MaxConcurrentStreams) > reverseProxyMaximumConfiguredStreams {
		return common.NewError("http2 max concurrent streams must be between 1 and ", reverseProxyMaximumConfiguredStreams)
	}
	if value.QUICMaxIncomingStreams < 1 || value.QUICMaxIncomingStreams > reverseProxyMaximumConfiguredStreams {
		return common.NewError("quic max incoming streams must be between 1 and ", reverseProxyMaximumConfiguredStreams)
	}
	if value.DefaultUpstreamMaxIdleConnections < 0 || value.DefaultUpstreamMaxIdleConnections > reverseProxyMaximumConfiguredLimit {
		return common.NewError("default upstream idle connections must be between 0 and ", reverseProxyMaximumConfiguredLimit)
	}
	if value.MemoryPoolBytes < reverseProxyMinimumRewriteReservationBytes || value.MemoryPoolBytes > reverseProxyMaximumMemoryPoolBytes {
		return common.NewError("memory pool size is invalid")
	}
	if value.DefaultRuleMemoryLimitBytes < reverseProxyMinimumRewriteReservationBytes || value.DefaultRuleMemoryLimitBytes > value.MemoryPoolBytes {
		return common.NewError("default rule memory limit is invalid")
	}
	if value.ResponseRewriteInputBytes < reverseProxyMinimumRewriteReservationBytes || value.ResponseRewriteInputBytes > value.DefaultRuleMemoryLimitBytes {
		return common.NewError("response rewrite input limit is invalid")
	}
	if value.ResponseRewriteOutputBytes < reverseProxyMinimumRewriteReservationBytes || value.ResponseRewriteOutputBytes > value.DefaultRuleMemoryLimitBytes {
		return common.NewError("response rewrite output limit is invalid")
	}
	// A conversion retains the input while the bounded origin and relative-path
	// passes each own an output buffer.  Validate the real peak reservation,
	// not just one output size, so a saved policy cannot make rewriting fail for
	// every otherwise eligible response.
	if reverseProxyRewriteReservationBytes(value.ResponseRewriteInputBytes, value.ResponseRewriteOutputBytes) > value.DefaultRuleMemoryLimitBytes {
		return common.NewError("response rewrite input and two output buffers exceed the default rule memory limit")
	}
	if value.ResponseRewriteMaxConcurrent < 1 || value.ResponseRewriteMaxConcurrent > reverseProxyMaximumConfiguredLimit {
		return common.NewError("response rewrite concurrency is invalid")
	}
	return nil
}

func reverseProxyRewriteReservationBytes(inputBytes int64, outputBytes int64) int64 {
	if inputBytes < 0 || outputBytes < 0 {
		return 0
	}
	return inputBytes + outputBytes*2
}

func loadReverseProxySettingsTx(tx *gorm.DB) (*model.ReverseProxySettings, error) {
	if tx == nil {
		return nil, common.NewError("database is not ready")
	}
	settings := defaultReverseProxySettingsModel()
	result := tx.Where("id = ?", reverseProxySettingsRecordID).First(&settings)
	if database.IsNotFound(result.Error) {
		if err := tx.Create(&settings).Error; err != nil {
			return nil, err
		}
		// Rows produced before the resource-settings singleton used zero as a
		// legacy default. Preserve their former effective limit exactly once.
		if err := tx.Model(&model.ReverseProxyRule{}).Where("max_concurrent_requests = ?", 0).Update("max_concurrent_requests", model.ReverseProxyLegacyMaxConcurrentRequests).Error; err != nil {
			return nil, err
		}
		if err := tx.Model(&model.ReverseProxyRule{}).Where("dns_max_concurrent_queries = ?", 0).Update("dns_max_concurrent_queries", model.ReverseProxyLegacyDNSMaxConcurrentQueries).Error; err != nil {
			return nil, err
		}
		return &settings, nil
	}
	if result.Error != nil {
		return nil, result.Error
	}
	normalized := settings
	normalizeReverseProxySettingsModel(&normalized)
	if normalized != settings {
		if err := tx.Model(&model.ReverseProxySettings{}).Where("id = ?", reverseProxySettingsRecordID).Updates(map[string]interface{}{
			"revision":                              normalized.Revision,
			"listener_connection_limit":             normalized.ListenerConnectionLimit,
			"global_http_max_concurrent":            normalized.GlobalHTTPMaxConcurrent,
			"global_dns_max_concurrent":             normalized.GlobalDNSMaxConcurrent,
			"http2_max_concurrent_streams":          normalized.HTTP2MaxConcurrentStreams,
			"quic_max_incoming_streams":             normalized.QUICMaxIncomingStreams,
			"default_upstream_max_idle_connections": normalized.DefaultUpstreamMaxIdleConnections,
			"memory_pool_bytes":                     normalized.MemoryPoolBytes,
			"default_rule_memory_limit_bytes":       normalized.DefaultRuleMemoryLimitBytes,
			"response_rewrite_input_bytes":          normalized.ResponseRewriteInputBytes,
			"response_rewrite_output_bytes":         normalized.ResponseRewriteOutputBytes,
			"response_rewrite_max_concurrent":       normalized.ResponseRewriteMaxConcurrent,
		}).Error; err != nil {
			return nil, err
		}
		settings = normalized
	}
	return &settings, nil
}

func (s *ReverseProxyService) loadReverseProxySettings() (*model.ReverseProxySettings, error) {
	db := database.GetDB()
	if db == nil {
		return nil, common.NewError("database is not ready")
	}
	var settings *model.ReverseProxySettings
	err := db.Transaction(func(tx *gorm.DB) error {
		row, err := loadReverseProxySettingsTx(tx)
		if err != nil {
			return err
		}
		settings = row
		return nil
	})
	if err == nil && settings != nil {
		reverseProxyResources.apply(reverseProxySettingsView(settings))
	}
	return settings, err
}

// peekReverseProxyRevision is intentionally a tiny read: cron uses it to
// decide whether the expensive rule/certificate/runtime reconciliation is
// necessary.  It neither opens an explicit transaction nor normalizes data,
// so it cannot hold the single SQLite connection across listener work.
func (s *ReverseProxyService) peekReverseProxyRevision() (uint64, error) {
	db := database.GetDB()
	if db == nil {
		return 0, common.NewError("database is not ready")
	}
	settings := &model.ReverseProxySettings{}
	err := db.Select("revision").Where("id = ?", reverseProxySettingsRecordID).First(settings).Error
	if database.IsNotFound(err) {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return settings.Revision, nil
}

func reverseProxyExpectedRevision(tx *gorm.DB, expected *uint64) (*model.ReverseProxySettings, error) {
	settings, err := loadReverseProxySettingsTx(tx)
	if err != nil {
		return nil, err
	}
	if expected != nil && *expected != settings.Revision {
		return settings, reverseProxyFormatRevisionConflict(settings.Revision)
	}
	return settings, nil
}

func reverseProxyBumpRevisionTx(tx *gorm.DB, settings *model.ReverseProxySettings) error {
	if tx == nil || settings == nil {
		return common.NewError("reverse proxy settings are unavailable")
	}
	next := settings.Revision + 1
	result := tx.Model(&model.ReverseProxySettings{}).
		Where("id = ? AND revision = ?", reverseProxySettingsRecordID, settings.Revision).
		Update("revision", next)
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 1 {
		return errReverseProxyRevisionConflict
	}
	settings.Revision = next
	return nil
}

func (s *ReverseProxyService) SaveResourceSettings(payload ReverseProxySettingsPayload) error {
	if err := validateReverseProxyResourceSettings(payload.ReverseProxyResourceSettings); err != nil {
		return err
	}
	db := database.GetDB()
	if db == nil {
		return common.NewError("database is not ready")
	}
	err := db.Transaction(func(tx *gorm.DB) error {
		settings, err := reverseProxyExpectedRevision(tx, payload.ExpectedRevision)
		if err != nil {
			return err
		}
		settings.ListenerConnectionLimit = payload.ListenerConnectionLimit
		settings.GlobalHTTPMaxConcurrent = payload.GlobalHTTPMaxConcurrent
		settings.GlobalDNSMaxConcurrent = payload.GlobalDNSMaxConcurrent
		settings.HTTP2MaxConcurrentStreams = payload.HTTP2MaxConcurrentStreams
		settings.QUICMaxIncomingStreams = payload.QUICMaxIncomingStreams
		settings.DefaultUpstreamMaxIdleConnections = payload.DefaultUpstreamMaxIdleConnections
		settings.MemoryPoolBytes = payload.MemoryPoolBytes
		settings.DefaultRuleMemoryLimitBytes = payload.DefaultRuleMemoryLimitBytes
		settings.ResponseRewriteInputBytes = payload.ResponseRewriteInputBytes
		settings.ResponseRewriteOutputBytes = payload.ResponseRewriteOutputBytes
		settings.ResponseRewriteMaxConcurrent = payload.ResponseRewriteMaxConcurrent
		if err := reverseProxyBumpRevisionTx(tx, settings); err != nil {
			return err
		}
		return tx.Save(settings).Error
	})
	if err != nil {
		return err
	}
	reverseProxyResources.apply(payload.ReverseProxyResourceSettings)
	if err := s.syncAllRuntimeNow(); err != nil {
		// Resource changes are authoritative once committed.  Runtime failures are
		// surfaced through state and retried; rolling a concurrent SQLite write
		// back here could overwrite a newer configuration.
		return nil
	}
	return nil
}

// reverseProxyAdjustableLimiter keeps an active count separate from its limit,
// allowing a saved resource policy to take effect without tearing down live
// connections.  A zero maximum means unlimited.
type reverseProxyAdjustableLimiter struct {
	mu     sync.Mutex
	max    int
	active int
}

func (l *reverseProxyAdjustableLimiter) SetMax(max int) {
	if l == nil {
		return
	}
	if max < 0 {
		max = 0
	}
	l.mu.Lock()
	l.max = max
	l.mu.Unlock()
}

func (l *reverseProxyAdjustableLimiter) TryAcquire() bool {
	if l == nil {
		return true
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.max > 0 && l.active >= l.max {
		return false
	}
	l.active++
	return true
}

func (l *reverseProxyAdjustableLimiter) Release() {
	if l == nil {
		return
	}
	l.mu.Lock()
	if l.active > 0 {
		l.active--
	}
	l.mu.Unlock()
}

func (l *reverseProxyAdjustableLimiter) Snapshot() (active int, max int) {
	if l == nil {
		return 0, 0
	}
	l.mu.Lock()
	active, max = l.active, l.max
	l.mu.Unlock()
	return active, max
}

func reverseProxyFormatRevisionConflict(current uint64) error {
	return fmt.Errorf("%w: current revision is %d", errReverseProxyRevisionConflict, current)
}

func reverseProxyResourceSettingsStateKey() string {
	settings := reverseProxyResources.current()
	return fmt.Sprintf("%d|%d|%d|%d|%d|%d|%d|%d|%d|%d|%d",
		settings.ListenerConnectionLimit,
		settings.GlobalHTTPMaxConcurrent,
		settings.GlobalDNSMaxConcurrent,
		settings.HTTP2MaxConcurrentStreams,
		settings.QUICMaxIncomingStreams,
		settings.DefaultUpstreamMaxIdleConnections,
		settings.MemoryPoolBytes,
		settings.DefaultRuleMemoryLimitBytes,
		settings.ResponseRewriteInputBytes,
		settings.ResponseRewriteOutputBytes,
		settings.ResponseRewriteMaxConcurrent,
	)
}
