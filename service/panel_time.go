package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/logger"
)

const (
	panelTimeLocationCacheTTL        = 30 * time.Second
	panelTimeRemoteValidationTimeout = 5 * time.Second
)

// PanelTimeContext is the small, explicit contract used by the browser.  Unix
// is an absolute instant; TimeLocation only controls how that instant is
// rendered as a calendar date and time.
type PanelTimeContext struct {
	Unix         int64  `json:"unix"`
	TimeLocation string `json:"timeLocation"`
	Selectable   bool   `json:"selectable"`
}

type panelTimeLocationCache struct {
	mu        sync.RWMutex
	name      string
	location  *time.Location
	expiresAt time.Time
}

var cachedPanelTimeLocation panelTimeLocationCache

// Database recovery can be reached by several requests at the same time after
// a manual database repair/reset. Settings has no uniqueness constraint on its
// key column, so serialize this rare initialization path to avoid duplicate
// timeLocation rows and duplicate remote validation requests.
var panelTimeInitializationMu sync.Mutex

var (
	panelTimeNow                    = time.Now
	panelTimeRemoteValidator        = validateTimeZoneWithRemoteSources
	panelTimeSystemLocationDetector = detectSystemTimeLocationName
	panelTimeHTTPClient             = http.DefaultClient
	panelTimeRemoteURLBuilder       = timeZoneRemoteURLs
)

func init() {
	database.RegisterDBResetHook(InvalidatePanelTimeLocationCache)
}

// InvalidatePanelTimeLocationCache intentionally only drops a parsed
// time.Location.  It owns no timer, connection or goroutine.
func InvalidatePanelTimeLocationCache() {
	cachedPanelTimeLocation.mu.Lock()
	cachedPanelTimeLocation.name = ""
	cachedPanelTimeLocation.location = nil
	cachedPanelTimeLocation.expiresAt = time.Time{}
	cachedPanelTimeLocation.mu.Unlock()
}

func cachePanelTimeLocation(name string, location *time.Location) {
	if location == nil {
		return
	}
	cachedPanelTimeLocation.mu.Lock()
	cachedPanelTimeLocation.name = name
	cachedPanelTimeLocation.location = location
	cachedPanelTimeLocation.expiresAt = panelTimeNow().Add(panelTimeLocationCacheTTL)
	cachedPanelTimeLocation.mu.Unlock()
}

func cachedPanelLocation() (string, *time.Location, bool) {
	now := panelTimeNow()
	cachedPanelTimeLocation.mu.RLock()
	name := cachedPanelTimeLocation.name
	location := cachedPanelTimeLocation.location
	expiresAt := cachedPanelTimeLocation.expiresAt
	cachedPanelTimeLocation.mu.RUnlock()
	if name == "" || location == nil || !now.Before(expiresAt) {
		return "", nil, false
	}
	return name, location, true
}

// IsSelectableTimeLocation reports whether a zone is present in the fixed UI
// list.  A valid IANA zone may still be stored for compatibility/fallback but
// must not be injected into the selector.
func IsSelectableTimeLocation(value string) bool {
	_, ok := supportedTimeLocationSet[strings.TrimSpace(value)]
	return ok
}

// NormalizePanelTimeLocation accepts every IANA location Go can load.  The
// selector itself exposes a conservative subset, while this keeps existing
// database values usable after an OS migration.
func NormalizePanelTimeLocation(value string) (string, error) {
	normalized := normalizeTimeLocationName(value)
	if normalized == "" {
		return "", fmt.Errorf("无效的 IANA 时区：%s", strings.TrimSpace(value))
	}
	return normalized, nil
}

func (s *SettingService) storedPanelTimeLocation() (string, bool, error) {
	setting, err := s.getSetting("timeLocation")
	if database.IsNotFound(err) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	normalized := normalizeTimeLocationName(setting.Value)
	if normalized == "" {
		return "", false, nil
	}
	return normalized, true, nil
}

// EnsurePanelTimeLocation creates the database value only when it is absent
// or invalid.  It first validates UTC against international HTTPS sources. If
// all sources are blocked, the current Linux system zone is retained as the
// fallback; UTC is the final safe fallback. Existing valid values are never
// overwritten by a remote validation failure.
func (s *SettingService) EnsurePanelTimeLocation() (string, error) {
	panelTimeInitializationMu.Lock()
	defer panelTimeInitializationMu.Unlock()

	name, exists, err := s.storedPanelTimeLocation()
	if err != nil {
		return "", err
	}
	if exists {
		return name, nil
	}

	selected := "UTC"
	if err := ValidatePanelTimeZoneRemote(selected); err != nil {
		logger.Warning("validate UTC while initializing panel timezone failed, fallback to system timezone: ", err)
		if detected := normalizeTimeLocationName(panelTimeSystemLocationDetector()); detected != "" {
			selected = detected
		}
	}

	if err := s.saveSetting("timeLocation", selected); err != nil {
		return "", err
	}
	return selected, nil
}

// InitializePanelTimeOnStartup performs the only automatic remote validation.
// It is called once for every process start. A valid saved setting remains the
// source of truth even if every remote source is unavailable.
func (s *SettingService) InitializePanelTimeOnStartup() error {
	name, exists, err := s.storedPanelTimeLocation()
	if err != nil {
		return err
	}
	if !exists {
		_, err = s.EnsurePanelTimeLocation()
		return err
	}

	location, loadErr := time.LoadLocation(name)
	if loadErr != nil {
		InvalidatePanelTimeLocationCache()
		_, err = s.EnsurePanelTimeLocation()
		return err
	}
	cachePanelTimeLocation(name, location)

	if err := ValidatePanelTimeZoneRemote(name); err != nil {
		logger.Warningf("validate saved panel timezone %q on startup failed; keep database value: %v", name, err)
	}
	return nil
}

// GetPanelTimeLocation returns a parsed, lazily refreshed database setting.
// The cache has a short TTL to make direct database recovery visible without a
// persistent watcher or background work.
func (s *SettingService) GetPanelTimeLocation() (*time.Location, error) {
	if _, location, ok := cachedPanelLocation(); ok {
		return location, nil
	}

	name, err := s.EnsurePanelTimeLocation()
	if err != nil {
		return nil, err
	}
	location, err := time.LoadLocation(name)
	if err != nil {
		// A value can become invalid only after external DB manipulation. Treat
		// it like a missing value and let the defined initialization fallback run.
		InvalidatePanelTimeLocationCache()
		if saveErr := s.saveSetting("timeLocation", ""); saveErr != nil {
			return nil, err
		}
		name, ensureErr := s.EnsurePanelTimeLocation()
		if ensureErr != nil {
			return nil, ensureErr
		}
		location, err = time.LoadLocation(name)
		if err != nil {
			return nil, err
		}
	}
	cachePanelTimeLocation(name, location)
	return location, nil
}

func (s *SettingService) GetPanelTimeContext() (*PanelTimeContext, error) {
	location, err := s.GetPanelTimeLocation()
	if err != nil {
		return nil, err
	}
	name := location.String()
	return &PanelTimeContext{
		Unix:         panelTimeNow().Unix(),
		TimeLocation: name,
		Selectable:   IsSelectableTimeLocation(name),
	}, nil
}

// WillChangePanelTimeLocation is evaluated before a settings transaction so
// cron is only rebuilt after an actual timezone change, not after every
// unrelated settings save whose payload happens to include timeLocation.
func (s *SettingService) WillChangePanelTimeLocation(data []byte) (bool, error) {
	settings := map[string]string{}
	if err := json.Unmarshal(data, &settings); err != nil {
		return false, err
	}
	requested, supplied := settings["timeLocation"]
	if !supplied {
		return false, nil
	}
	requested = normalizeTimeLocationSettingValue(requested, defaultTimeLocationValue())
	current, exists, err := s.storedPanelTimeLocation()
	if err != nil {
		return false, err
	}
	return !exists || current != requested, nil
}

// PanelNow is the common calendar-time entry point. It must only be used for
// user-visible calendar semantics and scheduled date boundaries. TTLs,
// protocol timestamps and certificate validity remain absolute time values.
func PanelNow() time.Time {
	location, err := (&SettingService{}).GetPanelTimeLocation()
	if err != nil || location == nil {
		return panelTimeNow().UTC()
	}
	return panelTimeNow().In(location)
}

// ValidatePanelTimeZoneRemote validates a selected IANA zone before any user
// initiated mutation. It never changes the host clock or produces a virtual
// database clock.
func ValidatePanelTimeZoneRemote(value string) error {
	name, err := NormalizePanelTimeLocation(value)
	if err != nil {
		return err
	}
	return panelTimeRemoteValidator(name)
}

type remoteTimeValidationError struct {
	failures []string
}

func (e *remoteTimeValidationError) Error() string {
	if e == nil || len(e.failures) == 0 {
		return "国际时间源校验失败"
	}
	return "国际时间源校验失败：" + strings.Join(e.failures, "；")
}

type remoteTimeSourceResult struct {
	url string
	err error
}

func timeZoneRemoteURLs(name string) []string {
	queryName := url.QueryEscape(name)
	// WorldTimeAPI models an IANA name as nested path segments. Preserve its
	// slash separator while escaping every other path character.
	pathName := strings.ReplaceAll(url.PathEscape(name), "%2F", "/")
	return []string{
		"https://timeapi.io/api/Time/current/zone?timeZone=" + queryName,
		"https://worldtimeapi.org/api/timezone/" + pathName,
	}
}

func validateTimeZoneWithRemoteSources(name string) error {
	urls := panelTimeRemoteURLBuilder(name)
	ctx, cancel := context.WithTimeout(context.Background(), panelTimeRemoteValidationTimeout)
	defer cancel()

	resultCh := make(chan remoteTimeSourceResult, len(urls))
	for _, sourceURL := range urls {
		sourceURL := sourceURL
		go func() {
			resultCh <- remoteTimeSourceResult{
				url: sourceURL,
				err: validateSingleRemoteTimeSource(ctx, sourceURL, name),
			}
		}()
	}

	failures := make([]string, 0, len(urls))
	for range urls {
		result := <-resultCh
		if result.err == nil {
			return nil
		}
		failures = append(failures, result.url+"（"+result.err.Error()+"）")
	}
	return &remoteTimeValidationError{failures: failures}
}

func validateSingleRemoteTimeSource(ctx context.Context, sourceURL string, expectedZone string) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/json")
	// 校验仅发生在启动/用户保存时；关闭本次连接，避免为这类低频请求
	// 保留空闲 HTTP 连接或后台资源。
	request.Close = true
	response, err := panelTimeHTTPClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		io.Copy(io.Discard, io.LimitReader(response.Body, 1024))
		return fmt.Errorf("HTTP %d", response.StatusCode)
	}

	var payload map[string]json.RawMessage
	decoder := json.NewDecoder(io.LimitReader(response.Body, 64*1024))
	if err := decoder.Decode(&payload); err != nil {
		return fmt.Errorf("响应不是有效 JSON：%w", err)
	}

	remoteZone := readRemoteString(payload, "timeZone", "timezone")
	normalizedRemoteZone := normalizeTimeLocationName(remoteZone)
	if normalizedRemoteZone == "" || normalizedRemoteZone != expectedZone {
		return fmt.Errorf("返回时区 %q 与请求时区 %q 不一致", remoteZone, expectedZone)
	}
	if !remotePayloadContainsUsableTime(payload) {
		return fmt.Errorf("响应未包含可用时间")
	}
	return nil
}

func readRemoteString(payload map[string]json.RawMessage, keys ...string) string {
	for _, key := range keys {
		raw, ok := payload[key]
		if !ok {
			continue
		}
		var value string
		if json.Unmarshal(raw, &value) == nil {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func remotePayloadContainsUsableTime(payload map[string]json.RawMessage) bool {
	for _, key := range []string{"unixtime", "unixTime", "currentFileTime"} {
		if raw, ok := payload[key]; ok {
			var value float64
			if json.Unmarshal(raw, &value) == nil && value > 0 {
				return true
			}
		}
	}
	for _, key := range []string{"datetime", "dateTime"} {
		value := readRemoteString(payload, key)
		if value == "" {
			continue
		}
		if _, err := time.Parse(time.RFC3339Nano, value); err == nil {
			return true
		}
		if _, err := time.Parse("2006-01-02T15:04:05", value); err == nil {
			return true
		}
	}

	var year, month, day int
	if !readRemoteInt(payload, "year", &year) || !readRemoteInt(payload, "month", &month) || !readRemoteInt(payload, "day", &day) {
		return false
	}
	if year < 1970 || month < 1 || month > 12 || day < 1 || day > 31 {
		return false
	}
	validated := time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.UTC)
	return validated.Year() == year && int(validated.Month()) == month && validated.Day() == day
}

func readRemoteInt(payload map[string]json.RawMessage, key string, target *int) bool {
	raw, ok := payload[key]
	if !ok {
		return false
	}
	var number float64
	if err := json.Unmarshal(raw, &number); err != nil {
		return false
	}
	*target = int(number)
	return true
}
