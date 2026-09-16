package service

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/alireza0/s-ui/database"
)

const (
	panelTimeLocationCacheTTL = 30 * time.Second
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
// timeLocation rows.
var panelTimeInitializationMu sync.Mutex

var (
	panelTimeNow                    = time.Now
	panelTimeSystemLocationDetector = detectSystemTimeLocationName
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
// or invalid. It first checks the detected Linux host timezone. If unavailable,
// UTC is the safe fallback.
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
	if detected := normalizeTimeLocationName(panelTimeSystemLocationDetector()); detected != "" {
		if err := ValidatePanelTimeZoneLocal(detected); err == nil {
			selected = detected
		}
	}

	if err := s.saveSetting("timeLocation", selected); err != nil {
		return "", err
	}
	return selected, nil
}

// InitializePanelTimeOnStartup loads and verifies the saved panel timezone.
// It is called once for every process start without remote network dependencies.
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

// ValidatePanelTimeZoneLocal validates a selected IANA zone using local Go
// time databases. It does not perform any remote HTTP requests.
func ValidatePanelTimeZoneLocal(value string) error {
	name, err := NormalizePanelTimeLocation(value)
	if err != nil {
		return err
	}
	if _, err := time.LoadLocation(name); err != nil {
		return fmt.Errorf("系统无法加载 IANA 时区 %q: %w", name, err)
	}
	return nil
}

// ValidatePanelTimeZoneRemote is preserved for backwards compatibility and
// directly executes local validation without remote network calls.
func ValidatePanelTimeZoneRemote(value string) error {
	return ValidatePanelTimeZoneLocal(value)
}
