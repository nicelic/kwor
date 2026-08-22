package service

import (
	"encoding/json"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alireza0/s-ui/config"
	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/logger"
	"github.com/alireza0/s-ui/util/common"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var defaultConfig = `{
  "dns": {
    "servers": [
      {
        "type": "tls",
        "tag": "tls_1.1.1.1",
        "server": "1.1.1.1",
        "server_port": 853,
        "tls": {
          "enabled": true,
          "server_name": "1.1.1.1"
        }
      }
    ],
    "rules": [],
    "final": "tls_1.1.1.1",
    "strategy": "prefer_ipv4"
  },
  "route": {
    "rules": [
		  {
        "action": "sniff"
      },
      {
        "protocol": [
          "dns"
        ],
        "action": "hijack-dns"
      }
    ]
  },
  "experimental": {}
}`

var defaultMihomoConfig = `{
  "dns": {
    "nameserver": [
      "tls://1.1.1.1#disable-ipv6=true",
      "tls://1.0.0.1#disable-ipv6=true"
    ]
  },
  "route": {
    "no_resolve": true,
    "rules": [],
    "rule_set": []
  }
}`

var supportedTimeLocations = []string{
	"UTC",
	"Asia/Shanghai",
	"Asia/Hong_Kong",
	"Asia/Taipei",
	"Asia/Tokyo",
	"Asia/Seoul",
	"Asia/Singapore",
	"Asia/Bangkok",
	"Asia/Ho_Chi_Minh",
	"Asia/Kuala_Lumpur",
	"Asia/Jakarta",
	"Asia/Manila",
	"Asia/Kolkata",
	"Asia/Karachi",
	"Asia/Dhaka",
	"Asia/Kathmandu",
	"Asia/Almaty",
	"Asia/Tashkent",
	"Asia/Dubai",
	"Asia/Riyadh",
	"Asia/Tehran",
	"Asia/Jerusalem",
	"Europe/London",
	"Europe/Dublin",
	"Europe/Lisbon",
	"Europe/Madrid",
	"Europe/Paris",
	"Europe/Brussels",
	"Europe/Amsterdam",
	"Europe/Berlin",
	"Europe/Zurich",
	"Europe/Rome",
	"Europe/Vienna",
	"Europe/Prague",
	"Europe/Warsaw",
	"Europe/Stockholm",
	"Europe/Oslo",
	"Europe/Helsinki",
	"Europe/Athens",
	"Europe/Bucharest",
	"Europe/Kyiv",
	"Europe/Istanbul",
	"Europe/Moscow",
	"Africa/Cairo",
	"Africa/Casablanca",
	"Africa/Lagos",
	"Africa/Nairobi",
	"Africa/Johannesburg",
	"America/New_York",
	"America/Chicago",
	"America/Denver",
	"America/Los_Angeles",
	"America/Anchorage",
	"Pacific/Honolulu",
	"America/Toronto",
	"America/Vancouver",
	"America/Mexico_City",
	"America/Bogota",
	"America/Lima",
	"America/Santiago",
	"America/Caracas",
	"America/Sao_Paulo",
	"America/Argentina/Buenos_Aires",
	"America/Montevideo",
	"Australia/Sydney",
	"Australia/Melbourne",
	"Australia/Brisbane",
	"Australia/Perth",
	"Pacific/Auckland",
	"Pacific/Fiji",
	"Pacific/Guam",
}

var supportedTimeLocationSet = func() map[string]struct{} {
	set := make(map[string]struct{}, len(supportedTimeLocations))
	for _, value := range supportedTimeLocations {
		set[value] = struct{}{}
	}
	return set
}()

const (
	sessionMaxAgeMaxMinutes  = 72 * 60
	defaultSessionMaxAgeUnit = "d"
	sessionMaxAgeCacheTTL    = time.Minute
	trafficAgeCacheTTL       = time.Minute
)

var cachedSessionMaxAge = struct {
	sync.RWMutex
	loaded    bool
	value     int
	expiresAt time.Time
}{}

var cachedTrafficAge = struct {
	sync.RWMutex
	loaded    bool
	value     int
	expiresAt time.Time
}{}

func init() {
	database.RegisterDBResetHook(InvalidateSessionMaxAgeCache)
	database.RegisterDBResetHook(InvalidateTrafficAgeCache)
}

// InvalidateSessionMaxAgeCache is called only after settings changes commit,
// so session refreshes never retain a value from a rolled-back transaction.
func InvalidateSessionMaxAgeCache() {
	cachedSessionMaxAge.Lock()
	cachedSessionMaxAge.loaded = false
	cachedSessionMaxAge.value = 0
	cachedSessionMaxAge.expiresAt = time.Time{}
	cachedSessionMaxAge.Unlock()
}

// InvalidateTrafficAgeCache is called after settings writes commit, keeping
// frequent stats and API reads from repeatedly waiting on SQLite's one pooled
// connection.
func InvalidateTrafficAgeCache() {
	cachedTrafficAge.Lock()
	cachedTrafficAge.loaded = false
	cachedTrafficAge.value = 0
	cachedTrafficAge.expiresAt = time.Time{}
	cachedTrafficAge.Unlock()
}

var supportedTimeLocationLowerMap = func() map[string]string {
	set := make(map[string]string, len(supportedTimeLocations))
	for _, value := range supportedTimeLocations {
		set[strings.ToLower(value)] = value
	}
	return set
}()

var timeLocationAliasLowerMap = map[string]string{
	"etc/utc":                       "UTC",
	"etc/gmt":                       "UTC",
	"etc/gmt0":                      "UTC",
	"etc/greenwich":                 "UTC",
	"gmt":                           "UTC",
	"gmt0":                          "UTC",
	"utc0":                          "UTC",
	"zulu":                          "UTC",
	"local":                         "",
	"asia/calcutta":                 "Asia/Kolkata",
	"asia/chongqing":                "Asia/Shanghai",
	"asia/harbin":                   "Asia/Shanghai",
	"asia/katmandu":                 "Asia/Kathmandu",
	"asia/saigon":                   "Asia/Ho_Chi_Minh",
	"europe/kiev":                   "Europe/Kyiv",
	"us/eastern":                    "America/New_York",
	"us/central":                    "America/Chicago",
	"us/mountain":                   "America/Denver",
	"us/arizona":                    "America/Denver",
	"us/pacific":                    "America/Los_Angeles",
	"canada/eastern":                "America/Toronto",
	"america/buenos_aires":          "America/Argentina/Buenos_Aires",
	"america/argentina/buenosaires": "America/Argentina/Buenos_Aires",
}

var defaultValueMap = map[string]string{
	"webListen":                         "",
	"webDomain":                         "",
	"webPort":                           "8888",
	"secret":                            common.Random(32),
	"webCertFile":                       "",
	"webKeyFile":                        "",
	"webSelfSignedCertSQLite":           "false",
	"webPath":                           "/app/",
	"webURI":                            "",
	"sessionMaxAge":                     "0",
	"sessionMaxAgeUnit":                 defaultSessionMaxAgeUnit,
	"trafficAge":                        "30",
	"trafficOverviewLimitGiB":           "0",
	"trafficOverviewEnabled":            "true",
	"trafficOverviewResetDay":           "0",
	"trafficOverviewExpiryDate":         "",
	"trafficOverviewState":              "{}",
	"trafficOverviewSnapshot":           "{}",
	"trafficOverviewCapState":           "{}",
	"trafficOverviewPauseState":         "{}",
	"trafficOverviewVnstatManifest":     "{}",
	"firewallEnabled":                   "false",
	"firewallLastSyncAt":                "0",
	"firewallGeoUpdateIntervalMinutes":  "360",
	"firewallGeoLastRefreshAt":          "0",
	"systemLogDisableEnabled":           "false",
	"systemLogJournaldContent":          defaultSystemLogJournaldContent,
	"systemLogJournaldPath":             "",
	"systemSysctlEnabled":               "false",
	"systemSysctlContent":               defaultSystemSysctlContent,
	"systemSysctlPath":                  "",
	"systemLinuxDnsContent":             "",
	"systemLinuxDnsPath":                "",
	"systemLinuxDnsNameServersInput":    "",
	"systemMTUEnabled":                  "false",
	"systemMTUValue":                    "1500",
	"systemMTUScriptPath":               "",
	"systemMTUInterface":                "",
	"systemMTUOriginalValue":            "0",
	"acmeScriptPath":                    "",
	"acmeContactEmail":                  "",
	"acmePreferredCA":                   "letsencrypt",
	"acmeDefaultChallenge":              "standalone",
	"acmeDefaultWebroot":                "",
	"acmeDefaultDNSProvider":            "",
	"acmeDefaultKeyLength":              "ec-256",
	"acmeAutoUpgrade":                   "true",
	"panelAssignedCertificateRecordID":  "0",
	"panelAssignedCertificateRecordIDs": "[]",
	"timeLocation":                      "UTC",
	"subListen":                         "",
	"subPort":                           "22780",
	"subDomain":                         "",
	"subCertFile":                       "",
	"subKeyFile":                        "",
	"subSelfSignedCertSQLite":           "false",
	"subAssignedCertificateRecordID":    "0",
	"subAssignedCertificateRecordIDs":   "[]",
	"subUpdates":                        "12",
	"subEncode":                         "true",
	"subShowInfo":                       "true",
	"subURI":                            "",
	"serverTlsStoreEnabled":             "true",
	"serverTlsStore":                    "chrome",
	"clientTlsStoreEnabled":             "true",
	"clientTlsStore":                    "chrome",
	"subJsonExt":                        "",
	"subClashExt":                       "",
	"mihomo_config":                     defaultMihomoConfig,
	"coreAutoCheckEnabled":              "false",
	"coreAutoCheckIntervalHours":        "12",
	"coreAutoCheckLastAt":               "0",
	"coreAutoCheckFirstAt":              "0",
	"coreAutoCheckFirstTimeZone":        "",
	"coreAutoCheckLatestStable":         "",
	"coreAutoCheckLatestAlpha":          "",
	"coreAutoCheckPendingStable":        "",
	"coreAutoCheckPendingAlpha":         "",
	"coreAutoUpdateEnabled":             "false",
	"coreAutoUpdateLastAttemptAt":       "0",
	"coreAutoUpdateLastSuccessAt":       "0",
	"coreAutoUpdateError":               "",
	"coreAutoUpdateErrorAt":             "0",
	"coreAutoUpdateDisableReason":       "",
	"coreDownloadPreference":            "{}",
	"mihomoCoreAutoCheckEnabled":        "false",
	"mihomoCoreAutoCheckIntervalHours":  "12",
	"mihomoCoreAutoCheckLastAt":         "0",
	"mihomoCoreAutoCheckFirstAt":        "0",
	"mihomoCoreAutoCheckFirstTimeZone":  "",
	"mihomoCoreAutoCheckLatestStable":   "",
	"mihomoCoreAutoCheckLatestAlpha":    "",
	"mihomoCoreAutoCheckPendingStable":  "",
	"mihomoCoreAutoCheckPendingAlpha":   "",
	"mihomoCoreAutoUpdateEnabled":       "false",
	"mihomoCoreAutoUpdateLastAttemptAt": "0",
	"mihomoCoreAutoUpdateLastSuccessAt": "0",
	"mihomoCoreAutoUpdateError":         "",
	"mihomoCoreAutoUpdateErrorAt":       "0",
	"mihomoCoreAutoUpdateDisableReason": "",
	"mihomoCoreDownloadPreference":      "{}",
	"singboxCoreLogLevel":               defaultSingboxCoreLogLevel,
	"mihomoCoreLogLevel":                defaultMihomoCoreLogLevel,
	"subGroupAutoUpdateEnabled":         "false",
	"subGroupAutoUpdateIntervalMinutes": "5",
	"subGroupAutoUpdateLastAt":          "0",
	"kernelCleanupPinnedKernel":         "",
	"subManagerAutoSyncClientIds":       "[]",
	"subManagerAutoSyncMihomoClientIds": "[]",
	"config":                            defaultConfig,
	"version":                           config.GetVersion(),
}

// Keep this list aligned with SETTINGS_SAVE_KEYS in the settings page. Generic
// settings saves must not overwrite state owned by dedicated feature APIs.
var genericSettingsSaveKeys = map[string]struct{}{
	"webListen":             {},
	"webDomain":             {},
	"webPort":               {},
	"webPath":               {},
	"webURI":                {},
	"sessionMaxAge":         {},
	"sessionMaxAgeUnit":     {},
	"trafficAge":            {},
	"timeLocation":          {},
	"subListen":             {},
	"subPort":               {},
	"subPath":               {},
	"subDomain":             {},
	"subUpdates":            {},
	"subEncode":             {},
	"subShowInfo":           {},
	"subURI":                {},
	"serverTlsStoreEnabled": {},
	"serverTlsStore":        {},
	"clientTlsStoreEnabled": {},
	"clientTlsStore":        {},
	"subJsonExt":            {},
	"subClashExt":           {},
}

const (
	initialRandomSubPortMin  = 25000
	initialRandomSubPortMax  = 65000
	initialRandomSubPortStep = 10
)

type SettingService struct {
}

func extractTimeLocationFromZoneinfoPath(raw string) string {
	trimmed := filepath.ToSlash(strings.TrimSpace(raw))
	if trimmed == "" {
		return ""
	}
	for _, prefix := range []string{"/zoneinfo/posix/", "zoneinfo/posix/", "/zoneinfo/right/", "zoneinfo/right/"} {
		if idx := strings.Index(trimmed, prefix); idx >= 0 {
			return strings.Trim(trimmed[idx+len(prefix):], "/")
		}
	}
	if idx := strings.Index(trimmed, "/zoneinfo/"); idx >= 0 {
		return strings.Trim(trimmed[idx+len("/zoneinfo/"):], "/")
	}
	if idx := strings.Index(trimmed, "zoneinfo/"); idx >= 0 {
		return strings.Trim(trimmed[idx+len("zoneinfo/"):], "/")
	}
	return ""
}

func normalizeTimeLocationName(raw string) string {
	trimmed := strings.TrimSpace(strings.TrimPrefix(raw, ":"))
	if trimmed == "" {
		return ""
	}

	if extracted := extractTimeLocationFromZoneinfoPath(trimmed); extracted != "" {
		trimmed = extracted
	}

	if _, ok := supportedTimeLocationSet[trimmed]; ok {
		return trimmed
	}

	lower := strings.ToLower(trimmed)
	if normalized, ok := timeLocationAliasLowerMap[lower]; ok {
		return normalized
	}
	if normalized, ok := supportedTimeLocationLowerMap[lower]; ok {
		return normalized
	}
	if _, err := time.LoadLocation(trimmed); err == nil {
		return trimmed
	}
	return ""
}

func readTimeLocationFile(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		return strings.Trim(trimmed, "\"'")
	}
	return ""
}

func readTimeLocationConfigValue(path string, keys ...string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}

	keyMap := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		keyMap[strings.ToUpper(strings.TrimSpace(key))] = struct{}{}
	}

	for _, line := range strings.Split(string(data), "\n") {
		if idx := strings.Index(line, "#"); idx >= 0 {
			line = line[:idx]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.ToUpper(strings.TrimSpace(parts[0]))
		if _, ok := keyMap[key]; !ok {
			continue
		}
		return strings.Trim(strings.TrimSpace(parts[1]), "\"'")
	}
	return ""
}

func readLocaltimeZoneinfoName(path string) string {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return ""
	}
	return extractTimeLocationFromZoneinfoPath(resolved)
}

func detectSystemTimeLocationName() string {
	candidates := []string{
		readSystemTimeLocationFromTimedatectl(),
		readLocaltimeZoneinfoName("/etc/localtime"),
		readTimeLocationFile("/etc/timezone"),
		readTimeLocationConfigValue("/etc/sysconfig/clock", "ZONE", "TIMEZONE"),
		readTimeLocationConfigValue("/etc/conf.d/clock", "ZONE", "TIMEZONE"),
		// TZ can override only the panel process. Keep it last so it never
		// masks the actual Linux host timezone when system files are present.
		os.Getenv("TZ"),
	}

	for _, candidate := range candidates {
		if normalized := normalizeTimeLocationName(candidate); normalized != "" {
			return normalized
		}
	}
	return ""
}

func readSystemTimeLocationFromTimedatectl() string {
	command, err := exec.LookPath("timedatectl")
	if err != nil {
		return ""
	}
	output, err := runCommandOutputWithTimeout(shortSystemCommandTimeout, command, "show", "--property=Timezone", "--value")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(output)
}

func defaultTimeLocationValue() string {
	return defaultValueMap["timeLocation"]
}

func normalizeTimeLocationSettingValue(raw string, fallback string) string {
	if normalized := normalizeTimeLocationName(raw); normalized != "" {
		return normalized
	}
	if normalized := normalizeTimeLocationName(fallback); normalized != "" {
		return normalized
	}
	return defaultValueMap["timeLocation"]
}

func normalizeSessionMaxAgeUnit(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "m":
		return "m"
	case "h":
		return "h"
	case "d":
		return "d"
	default:
		return ""
	}
}

func EffectiveSessionMaxAgeMinutes(value int) int {
	if value <= 0 {
		return sessionMaxAgeMaxMinutes
	}
	if value > sessionMaxAgeMaxMinutes {
		return sessionMaxAgeMaxMinutes
	}
	return value
}

func generateRandomSubPath() string {
	const letters = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

	var builder strings.Builder
	builder.Grow(22)
	builder.WriteByte('/')
	for segment := 0; segment < 3; segment++ {
		if segment > 0 {
			builder.WriteByte('-')
		}
		for i := 0; i < 3; i++ {
			builder.WriteByte(letters[common.RandomInt(len(letters))])
		}
		for i := 0; i < 3; i++ {
			builder.WriteByte(byte('0' + common.RandomInt(10)))
		}
	}
	builder.WriteByte('/')
	return builder.String()
}

func normalizeInitialRandomSubPortStart(port int) int {
	if port < initialRandomSubPortMin {
		port = initialRandomSubPortMin
	}
	if port > initialRandomSubPortMax {
		port = initialRandomSubPortMax
	}

	offset := port - initialRandomSubPortMin
	return initialRandomSubPortMin + (offset/initialRandomSubPortStep)*initialRandomSubPortStep
}

func buildInitialRandomSubPortSequence(start int) []int {
	start = normalizeInitialRandomSubPortStart(start)
	total := ((initialRandomSubPortMax - initialRandomSubPortMin) / initialRandomSubPortStep) + 1
	ports := make([]int, 0, total)
	current := start
	for i := 0; i < total; i++ {
		ports = append(ports, current)
		current += initialRandomSubPortStep
		if current > initialRandomSubPortMax {
			current = initialRandomSubPortMin
		}
	}
	return ports
}

func probeSubscriptionPortAvailable(port int) bool {
	if port <= 0 || port > 65535 {
		return false
	}

	addr := ":" + strconv.Itoa(port)

	tcpListener, err := net.Listen("tcp", addr)
	if err != nil {
		return false
	}
	defer tcpListener.Close()

	udpConn, err := net.ListenPacket("udp", addr)
	if err != nil {
		return false
	}
	defer udpConn.Close()

	return true
}

func chooseInitialRandomSubPortFromStart(start int, availabilityChecker func(int) bool) (int, error) {
	if availabilityChecker == nil {
		availabilityChecker = probeSubscriptionPortAvailable
	}

	for _, port := range buildInitialRandomSubPortSequence(start) {
		if availabilityChecker(port) {
			return port, nil
		}
	}

	return 0, common.NewErrorf(
		"no available subscription port found in range %d-%d with step %d",
		initialRandomSubPortMin,
		initialRandomSubPortMax,
		initialRandomSubPortStep,
	)
}

func chooseInitialRandomSubPort() (int, error) {
	total := ((initialRandomSubPortMax - initialRandomSubPortMin) / initialRandomSubPortStep) + 1
	start := initialRandomSubPortMin + common.RandomInt(total)*initialRandomSubPortStep
	return chooseInitialRandomSubPortFromStart(start, nil)
}

func normalizeSubPortOrGenerate(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed != "" {
		port, err := strconv.Atoi(trimmed)
		if err == nil && port > 0 && port <= 65535 {
			return strconv.Itoa(port), nil
		}
	}

	port, err := chooseInitialRandomSubPort()
	if err != nil {
		return "", err
	}
	return strconv.Itoa(port), nil
}

func normalizeSubPathOrGenerate(subPath string) string {
	normalized, err := normalizePanelRoutePath(subPath, true)
	if err != nil {
		return generateRandomSubPath()
	}
	return normalized
}

func (s *SettingService) defaultSettingValue(key string) (string, error) {
	if key == "subPath" {
		return generateRandomSubPath(), nil
	}
	if key == "subPort" {
		port, err := chooseInitialRandomSubPort()
		if err != nil {
			return "", err
		}
		return strconv.Itoa(port), nil
	}
	if key == "timeLocation" {
		return defaultTimeLocationValue(), nil
	}
	value, ok := defaultValueMap[key]
	if !ok {
		return "", common.NewErrorf("key <%v> not in defaultValueMap", key)
	}
	return value, nil
}

func (s *SettingService) ensureSubPathSetting() (string, error) {
	setting, err := s.getSetting("subPath")
	if database.IsNotFound(err) {
		subPath := generateRandomSubPath()
		if saveErr := s.saveSetting("subPath", subPath); saveErr != nil {
			return "", saveErr
		}
		return subPath, nil
	}
	if err != nil {
		return "", err
	}

	normalized := normalizeSubPathOrGenerate(setting.Value)
	if normalized != setting.Value {
		if saveErr := s.saveSetting("subPath", normalized); saveErr != nil {
			return "", saveErr
		}
	}
	return normalized, nil
}

func (s *SettingService) ensureSubPortSetting() (string, error) {
	setting, err := s.getSetting("subPort")
	if database.IsNotFound(err) {
		value, valueErr := normalizeSubPortOrGenerate("")
		if valueErr != nil {
			return "", valueErr
		}
		if saveErr := s.saveSetting("subPort", value); saveErr != nil {
			return "", saveErr
		}
		return value, nil
	}
	if err != nil {
		return "", err
	}

	normalized, normalizeErr := normalizeSubPortOrGenerate(setting.Value)
	if normalizeErr != nil {
		return "", normalizeErr
	}
	if normalized != strings.TrimSpace(setting.Value) {
		if saveErr := s.saveSetting("subPort", normalized); saveErr != nil {
			return "", saveErr
		}
	}
	return normalized, nil
}

func (s *SettingService) ensureTimeLocationSetting() (string, error) {
	return s.EnsurePanelTimeLocation()
}

func (s *SettingService) GetAllSetting() (*map[string]string, error) {
	db := database.GetDB()
	settings := make([]*model.Setting, 0)
	err := db.Model(model.Setting{}).Order("id ASC").Find(&settings).Error
	if err != nil {
		return nil, err
	}
	allSetting := map[string]string{}

	for _, setting := range settings {
		allSetting[setting.Key] = setting.Value
	}

	for key := range defaultValueMap {
		// Panel time is initialized through EnsurePanelTimeLocation so its
		// documented remote-UTC -> Linux-system -> UTC fallback is preserved.
		// Do not let the generic defaults loop write it first.
		if key == "timeLocation" {
			continue
		}
		if _, exists := allSetting[key]; !exists {
			defaultValue, valueErr := s.defaultSettingValue(key)
			if valueErr != nil {
				return nil, valueErr
			}
			err = s.saveSetting(key, defaultValue)
			if err != nil {
				return nil, err
			}
			allSetting[key] = defaultValue
		}
	}

	subPath, err := s.ensureSubPathSetting()
	if err != nil {
		return nil, err
	}
	allSetting["subPath"] = subPath

	subPort, err := s.ensureSubPortSetting()
	if err != nil {
		return nil, err
	}
	allSetting["subPort"] = subPort

	timeLocation, err := s.ensureTimeLocationSetting()
	if err != nil {
		return nil, err
	}
	allSetting["timeLocation"] = timeLocation
	if err := s.ensureSubscriptionInitialState(); err != nil {
		return nil, err
	}

	// Due to security principles
	delete(allSetting, "secret")
	delete(allSetting, "config")
	delete(allSetting, "mihomo_config")
	delete(allSetting, "version")
	delete(allSetting, "trafficOverviewState")
	delete(allSetting, "trafficOverviewSnapshot")
	delete(allSetting, "trafficOverviewCapState")
	delete(allSetting, "trafficOverviewPauseState")
	delete(allSetting, "trafficOverviewVnstatManifest")
	delete(allSetting, certificateCoreRestartStateSettingKey)
	delete(allSetting, certificateAutoRenewBatchStateSettingKey)
	delete(allSetting, systemLinuxDNSContentKey)
	delete(allSetting, systemLinuxDNSPathKey)
	delete(allSetting, systemLinuxDNSNameServersInputKey)

	return &allSetting, nil
}

// ensureSettingsSnapshotDefaults is deliberately narrower than GetAllSetting:
// compact settings snapshots must not read two potentially multi-megabyte
// subscription extensions merely to verify that default keys exist.
func (s *SettingService) ensureSettingsSnapshotDefaults() error {
	db := database.GetDB()
	if db == nil {
		return common.NewError("database is not ready")
	}
	rows := make([]model.Setting, 0, len(defaultValueMap))
	if err := db.Model(model.Setting{}).Select("key").Find(&rows).Error; err != nil {
		return err
	}
	existing := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		existing[row.Key] = struct{}{}
	}
	missing := make([]model.Setting, 0)
	keys := make([]string, 0, len(defaultValueMap))
	for key := range defaultValueMap {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if key == "timeLocation" {
			continue
		}
		if _, exists := existing[key]; exists {
			continue
		}
		value, err := s.defaultSettingValue(key)
		if err != nil {
			return err
		}
		missing = append(missing, model.Setting{Key: key, Value: value})
	}
	if len(missing) > 0 {
		if err := db.Transaction(func(tx *gorm.DB) error {
			return tx.Clauses(clause.OnConflict{
				Columns:   []clause.Column{{Name: "key"}},
				DoNothing: true,
			}).Create(&missing).Error
		}); err != nil {
			return err
		}
		for _, setting := range missing {
			if isSubscriptionRenderSettingKey(setting.Key) {
				invalidateSubscriptionRuntimeSettings()
			}
			if setting.Key == "sessionMaxAge" {
				InvalidateSessionMaxAgeCache()
			}
		}
	}
	if _, err := s.ensureSubPathSetting(); err != nil {
		return err
	}
	if _, err := s.ensureSubPortSetting(); err != nil {
		return err
	}
	_, err := s.ensureTimeLocationSetting()
	return err
}

func (s *SettingService) ResetSettings() error {
	db := database.GetDB()
	if db == nil {
		return common.NewError("database is not ready")
	}
	changed := false
	err := db.Transaction(func(tx *gorm.DB) error {
		currentRevision, err := ensureSettingsRevisionState(tx)
		if err != nil {
			return err
		}
		deleted := tx.Where("1 = 1").Delete(model.Setting{})
		if deleted.Error != nil {
			return deleted.Error
		}
		if deleted.RowsAffected == 0 {
			return nil
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
	if err == nil && changed {
		markLastUpdate(time.Now().Unix())
		InvalidatePanelTimeLocationCache()
		invalidateSubscriptionRuntimeSettings()
		InvalidateSessionMaxAgeCache()
	}
	return err
}

func (s *SettingService) getSetting(key string) (*model.Setting, error) {
	db := database.GetDB()
	setting := &model.Setting{}
	err := db.Model(model.Setting{}).Where("key = ?", key).Order("id DESC").First(setting).Error
	if err != nil {
		return nil, err
	}
	return setting, nil
}

func (s *SettingService) getString(key string) (string, error) {
	if key == "subPath" {
		return s.ensureSubPathSetting()
	}
	if key == "subPort" {
		return s.ensureSubPortSetting()
	}
	if key == "timeLocation" {
		return s.ensureTimeLocationSetting()
	}
	setting, err := s.getSetting(key)
	if database.IsNotFound(err) {
		value, valueErr := s.defaultSettingValue(key)
		if valueErr != nil {
			return "", valueErr
		}
		return value, nil
	} else if err != nil {
		return "", err
	}
	return setting.Value, nil
}

func (s *SettingService) saveSetting(key string, value string) error {
	setting, err := s.getSetting(key)
	db := database.GetDB()
	var saveErr error
	if database.IsNotFound(err) {
		saveErr = db.Create(&model.Setting{
			Key:   key,
			Value: value,
		}).Error
	} else if err != nil {
		return err
	} else {
		setting.Key = key
		setting.Value = value
		saveErr = db.Save(setting).Error
	}
	if saveErr == nil {
		if key == "sessionMaxAge" {
			InvalidateSessionMaxAgeCache()
		}
		if key == "trafficAge" {
			InvalidateTrafficAgeCache()
		}
		if key == "timeLocation" {
			InvalidatePanelTimeLocationCache()
		}
		if isSubscriptionRenderSettingKey(key) {
			invalidateSubscriptionRuntimeSettings()
		}
	}
	return saveErr
}

func isPortForwardListenerSettingKey(key string) bool {
	switch strings.TrimSpace(key) {
	case "webListen", "webPort", "subListen", "subPort":
		return true
	default:
		return false
	}
}

// savePortForwardListenerSetting gives direct SettingService callers (CLI,
// bootstrap and compatibility code) the same transactional listener-claim
// protection as the normal ConfigService settings save path.
func (s *SettingService) savePortForwardListenerSetting(key string, value string) error {
	db := database.GetDB()
	if db == nil {
		return common.NewError("database is not ready")
	}
	return db.Transaction(func(tx *gorm.DB) error {
		setting := &model.Setting{}
		err := tx.Model(model.Setting{}).Where("key = ?", key).Order("id DESC").First(setting).Error
		if database.IsNotFound(err) {
			if err := tx.Create(&model.Setting{Key: key, Value: value}).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		} else {
			setting.Key = key
			setting.Value = value
			if err := tx.Save(setting).Error; err != nil {
				return err
			}
		}
		return validatePortForwardListenerClaimsAgainstActiveRules(tx)
	})
}

func (s *SettingService) setString(key string, value string) error {
	return s.saveEditableSettingDirect(key, value)
}

// SaveSetting is the exported version of saveSetting for external callers (e.g., cmd first-run setup)
func (s *SettingService) SaveSetting(key string, value string) error {
	if key == "config" {
		return s.SetConfig(value)
	}
	return s.setString(key, value)
}

func (s *SettingService) getBool(key string) (bool, error) {
	str, err := s.getString(key)
	if err != nil {
		return false, err
	}
	val, parseErr := strconv.ParseBool(strings.TrimSpace(str))
	if parseErr == nil {
		return val, nil
	}
	defaultStr, ok := defaultValueMap[key]
	if !ok {
		return false, parseErr
	}
	defaultVal, defaultErr := strconv.ParseBool(strings.TrimSpace(defaultStr))
	if defaultErr != nil {
		return false, parseErr
	}
	logger.Warningf("invalid bool setting %q=%q, fallback to default %q", key, str, defaultStr)
	return defaultVal, nil
}

// func (s *SettingService) setBool(key string, value bool) error {
// 	return s.setString(key, strconv.FormatBool(value))
// }

func (s *SettingService) getInt(key string) (int, error) {
	str, err := s.getString(key)
	if err != nil {
		return 0, err
	}
	val, parseErr := strconv.Atoi(strings.TrimSpace(str))
	if parseErr == nil {
		return val, nil
	}
	defaultStr, ok := defaultValueMap[key]
	if !ok {
		return 0, parseErr
	}
	defaultVal, defaultErr := strconv.Atoi(strings.TrimSpace(defaultStr))
	if defaultErr != nil {
		return 0, parseErr
	}
	logger.Warningf("invalid int setting %q=%q, fallback to default %q", key, str, defaultStr)
	return defaultVal, nil
}

func (s *SettingService) setInt(key string, value int) error {
	return s.setString(key, strconv.Itoa(value))
}
func (s *SettingService) GetListen() (string, error) {
	return s.getString("webListen")
}

func (s *SettingService) GetWebDomain() (string, error) {
	return s.getString("webDomain")
}

func (s *SettingService) GetPort() (int, error) {
	return s.getInt("webPort")
}

func (s *SettingService) SetPort(port int) error {
	return s.setInt("webPort", port)
}

func (s *SettingService) GetCertFile() (string, error) {
	return s.getString("webCertFile")
}

func (s *SettingService) GetKeyFile() (string, error) {
	return s.getString("webKeyFile")
}

func (s *SettingService) GetWebSelfSignedCertSQLite() (bool, error) {
	return s.getBool("webSelfSignedCertSQLite")
}

func (s *SettingService) GetWebPath() (string, error) {
	webPath, err := s.getString("webPath")
	if err != nil {
		return "", err
	}
	return normalizePanelRoutePath(webPath, false)
}

func (s *SettingService) SetWebPath(webPath string) error {
	normalized, err := normalizePanelRoutePath(webPath, false)
	if err != nil {
		return err
	}
	return s.setString("webPath", normalized)
}

func (s *SettingService) GetSecret() ([]byte, error) {
	secret, err := s.getString("secret")
	if secret == defaultValueMap["secret"] {
		err := s.saveSetting("secret", secret)
		if err != nil {
			logger.Warning("save secret failed:", err)
		}
	}
	return []byte(secret), err
}

func (s *SettingService) GetSessionMaxAge() (int, error) {
	now := time.Now()
	cachedSessionMaxAge.RLock()
	if cachedSessionMaxAge.loaded && now.Before(cachedSessionMaxAge.expiresAt) {
		value := cachedSessionMaxAge.value
		cachedSessionMaxAge.RUnlock()
		return value, nil
	}
	cachedSessionMaxAge.RUnlock()

	cachedSessionMaxAge.Lock()
	defer cachedSessionMaxAge.Unlock()
	if cachedSessionMaxAge.loaded && now.Before(cachedSessionMaxAge.expiresAt) {
		return cachedSessionMaxAge.value, nil
	}
	value, err := s.getInt("sessionMaxAge")
	if err != nil {
		return 0, err
	}
	cachedSessionMaxAge.loaded = true
	cachedSessionMaxAge.value = value
	cachedSessionMaxAge.expiresAt = now.Add(sessionMaxAgeCacheTTL)
	return value, nil
}

func (s *SettingService) GetEffectiveSessionMaxAgeMinutes() (int, error) {
	value, err := s.GetSessionMaxAge()
	if err != nil {
		return 0, err
	}
	return EffectiveSessionMaxAgeMinutes(value), nil
}

func (s *SettingService) GetTrafficAge() (int, error) {
	now := time.Now()
	cachedTrafficAge.RLock()
	if cachedTrafficAge.loaded && now.Before(cachedTrafficAge.expiresAt) {
		value := cachedTrafficAge.value
		cachedTrafficAge.RUnlock()
		return value, nil
	}
	cachedTrafficAge.RUnlock()

	cachedTrafficAge.Lock()
	defer cachedTrafficAge.Unlock()
	if cachedTrafficAge.loaded && now.Before(cachedTrafficAge.expiresAt) {
		return cachedTrafficAge.value, nil
	}
	value, err := s.getInt("trafficAge")
	if err != nil {
		return 0, err
	}
	cachedTrafficAge.loaded = true
	cachedTrafficAge.value = value
	cachedTrafficAge.expiresAt = now.Add(trafficAgeCacheTTL)
	return value, nil
}

func (s *SettingService) GetTimeLocation() (*time.Location, error) {
	return s.GetPanelTimeLocation()
}

func (s *SettingService) GetSubListen() (string, error) {
	return s.getString("subListen")
}

func (s *SettingService) GetSubPort() (int, error) {
	return s.getInt("subPort")
}

func (s *SettingService) SetSubPort(subPort int) error {
	return s.setInt("subPort", subPort)
}

func (s *SettingService) GetSubPath() (string, error) {
	return s.ensureSubPathSetting()
}

func (s *SettingService) SetSubPath(subPath string) error {
	normalized, err := normalizePanelRoutePath(subPath, true)
	if err != nil {
		return err
	}
	return s.setString("subPath", normalized)
}

func (s *SettingService) GetSubDomain() (string, error) {
	return s.getString("subDomain")
}

func (s *SettingService) GetSubCertFile() (string, error) {
	return s.getString("subCertFile")
}

func (s *SettingService) GetSubKeyFile() (string, error) {
	return s.getString("subKeyFile")
}

func (s *SettingService) GetSubSelfSignedCertSQLite() (bool, error) {
	return s.getBool("subSelfSignedCertSQLite")
}

func (s *SettingService) GetSubUpdates() (int, error) {
	return s.getSubscriptionRuntimeInt("subUpdates")
}

func (s *SettingService) GetSubEncode() (bool, error) {
	return s.getSubscriptionRuntimeBool("subEncode")
}

func (s *SettingService) GetSubShowInfo() (bool, error) {
	return s.getSubscriptionRuntimeBool("subShowInfo")
}

func (s *SettingService) GetSubURI() (string, error) {
	return s.getString("subURI")
}

func (s *SettingService) GetFinalSubURI(host string) (string, error) {
	endpoint, err := s.loadSubscriptionEndpointSettings()
	if err != nil {
		return "", err
	}
	if endpoint.uri != "" {
		return endpoint.uri, nil
	}
	protocol := "http"
	subAssignedIDs, subAssignErr := GetAssignedCertificateRecordIDsReadOnly(s, PanelSelfSignedTargetSub)
	if subAssignErr == nil && len(subAssignedIDs) > 0 {
		protocol = "https"
	}
	if endpoint.domain != "" {
		host = endpoint.domain
	}
	portNumber := endpoint.port
	port := ""
	if portNumber != "" {
		port = ":" + portNumber
		if (portNumber == "80" && protocol == "http") || (portNumber == "443" && protocol == "https") {
			port = ""
		}
	}

	if strings.Contains(host, ":") && !strings.HasPrefix(host, "[") {
		host = "[" + host + "]"
	}

	return protocol + "://" + host + port + endpoint.path, nil
}

type subscriptionEndpointSettings struct {
	uri    string
	domain string
	port   string
	path   string
}

// loadSubscriptionEndpointSettings reads only the settings needed to build a
// subscription URL. It deliberately avoids GetAllSetting because subscription
// extensions can be several MiB and are irrelevant to URI generation.
func (s *SettingService) loadSubscriptionEndpointSettings() (subscriptionEndpointSettings, error) {
	db := database.GetDB()
	if db == nil {
		return subscriptionEndpointSettings{}, common.NewError("database is not ready")
	}

	keys := []string{"subURI", "subDomain", "subPort", "subPath"}
	rows := make([]model.Setting, 0, len(keys))
	if err := db.Select("key", "value").Where("key IN ?", keys).Find(&rows).Error; err != nil {
		return subscriptionEndpointSettings{}, err
	}
	values := make(map[string]string, len(rows))
	for _, row := range rows {
		values[row.Key] = row.Value
	}

	endpoint := subscriptionEndpointSettings{
		uri:    strings.TrimSpace(values["subURI"]),
		domain: strings.TrimSpace(values["subDomain"]),
		port:   strings.TrimSpace(values["subPort"]),
		path:   strings.TrimSpace(values["subPath"]),
	}
	if endpoint.uri != "" {
		return endpoint, nil
	}
	if endpoint.port == "" {
		port, err := s.ensureSubPortSetting()
		if err != nil {
			return subscriptionEndpointSettings{}, err
		}
		endpoint.port = strings.TrimSpace(port)
	}
	if endpoint.path == "" {
		path, err := s.ensureSubPathSetting()
		if err != nil {
			return subscriptionEndpointSettings{}, err
		}
		endpoint.path = path
	}
	return endpoint, nil
}

func (s *SettingService) GetConfig() (string, error) {
	return s.GetConfigWithDB(database.GetDB())
}

// GetConfigWithDB reads the sanitized default-chain config through the caller's
// database handle. Runtime validation inside a transaction must use this path
// instead of recursively acquiring SQLite's single pooled connection.
func (s *SettingService) GetConfigWithDB(db *gorm.DB) (string, error) {
	if db == nil {
		return "", common.NewError("database is not ready")
	}
	value, err := s.getStringTx(db, "config")
	if err != nil {
		return "", err
	}

	sanitized, err := sanitizeSingboxConfigJSON(json.RawMessage(value))
	if err != nil {
		return "", err
	}
	return string(sanitized), nil
}

func (s *SettingService) SetConfig(config string) error {
	db := database.GetDB()
	if db == nil {
		return common.NewError("database is not ready")
	}
	changed := false
	err := db.Transaction(func(tx *gorm.DB) error {
		currentRevision, err := ensureSingboxConfigRevisionState(tx)
		if err != nil {
			return err
		}
		if err := validateSingboxConfigRouteBounds(json.RawMessage(config), tx); err != nil {
			return err
		}
		if err := s.SaveConfig(tx, json.RawMessage(config)); err != nil {
			return err
		}
		if _, err := bumpSingboxConfigRevision(tx, currentRevision); err != nil {
			return err
		}
		changed = true
		return nil
	})
	if err != nil {
		return err
	}
	if changed {
		markLastUpdate(time.Now().Unix())
	}
	return nil
}

func (s *SettingService) SaveConfig(tx *gorm.DB, config json.RawMessage) error {
	sanitized, err := sanitizeAndValidateSingboxConfigJSON(config)
	if err != nil {
		return err
	}
	configs, err := json.MarshalIndent(sanitized, "", "  ")
	if err != nil {
		return err
	}
	return upsertSetting(tx, "config", string(configs))
}

func (s *SettingService) Save(tx *gorm.DB, data json.RawMessage) error {
	var err error
	var settings map[string]string
	err = json.Unmarshal(data, &settings)
	if err != nil {
		return err
	}

	for key := range settings {
		if _, allowed := genericSettingsSaveKeys[key]; !allowed {
			delete(settings, key)
		}
	}

	for key, obj := range settings {
		normalized, normalizeErr := normalizeGenericSettingValue(key, obj)
		if normalizeErr != nil {
			return normalizeErr
		}
		obj = normalized

		// Delete all stats if it is set to 0
		if key == "trafficAge" && obj == "0" {
			err = tx.Where("id > 0").Delete(model.Stats{}).Error
			if err != nil {
				return err
			}
		}
		err = tx.Model(model.Setting{}).Where("key = ?", key).Update("value", obj).Error
		if err != nil {
			return err
		}
	}
	// The normal settings flow runs inside ConfigService's explicit SQLite
	// transaction. Do not invalidate the shared panel-time cache here: another
	// goroutine could then read and cache the old committed value before this
	// transaction commits. ConfigService invalidates it immediately after a
	// successful commit instead. Direct timeLocation writes use saveSetting(),
	// which is already auto-committed and invalidates the cache itself.
	return err
}

func (s *SettingService) GetSubJsonExt() (string, error) {
	// Subscription extensions may be several MiB. Do not retain them in the
	// runtime settings cache after a request finishes.
	return s.getString("subJsonExt")
}

func (s *SettingService) GetServerTLSStoreEnabled() (bool, error) {
	return s.getBool("serverTlsStoreEnabled")
}

// GetServerTLSStoreEnabledWithDB reads the setting through a caller-owned
// handle so runtime preflight can stay inside one SQLite transaction.
func (s *SettingService) GetServerTLSStoreEnabledWithDB(db *gorm.DB) (bool, error) {
	if db == nil {
		return false, common.NewError("database is not ready")
	}
	value, err := s.getStringTx(db, "serverTlsStoreEnabled")
	if err != nil {
		return false, err
	}
	parsed, parseErr := strconv.ParseBool(strings.TrimSpace(value))
	if parseErr == nil {
		return parsed, nil
	}
	defaultValue, ok := defaultValueMap["serverTlsStoreEnabled"]
	if !ok {
		return false, parseErr
	}
	parsed, defaultErr := strconv.ParseBool(strings.TrimSpace(defaultValue))
	if defaultErr != nil {
		return false, parseErr
	}
	logger.Warningf("invalid bool setting %q=%q, fallback to default %q", "serverTlsStoreEnabled", value, defaultValue)
	return parsed, nil
}

func (s *SettingService) GetServerTLSStore() (string, error) {
	store, err := s.getString("serverTlsStore")
	if err != nil {
		return "", err
	}
	normalized := normalizeCertificateStoreValue(store)
	if normalized == "" {
		return "chrome", nil
	}
	return normalized, nil
}

// GetServerTLSStoreWithDB is the transaction-safe counterpart used while a
// sing-box runtime snapshot is validated before commit.
func (s *SettingService) GetServerTLSStoreWithDB(db *gorm.DB) (string, error) {
	if db == nil {
		return "", common.NewError("database is not ready")
	}
	store, err := s.getStringTx(db, "serverTlsStore")
	if err != nil {
		return "", err
	}
	normalized := normalizeCertificateStoreValue(store)
	if normalized == "" {
		return "chrome", nil
	}
	return normalized, nil
}

func (s *SettingService) GetClientTLSStoreEnabled() (bool, error) {
	return s.getSubscriptionRuntimeBool("clientTlsStoreEnabled")
}

func (s *SettingService) GetClientTLSStore() (string, error) {
	store, err := s.getSubscriptionRuntimeSetting("clientTlsStore")
	if err != nil {
		return "", err
	}
	normalized := normalizeCertificateStoreValue(store)
	if normalized == "" {
		return "chrome", nil
	}
	return normalized, nil
}

// ResolveSubscriptionTLSStore returns the effective certificate store for generated subscription JSON.
// If client store is enabled, it always overrides tls-store derived from TLS templates/outbounds.
func (s *SettingService) ResolveSubscriptionTLSStore(fallback string) string {
	enabled, err := s.GetClientTLSStoreEnabled()
	if err == nil && enabled {
		store, storeErr := s.GetClientTLSStore()
		if storeErr == nil && store != "" {
			return store
		}
		return "chrome"
	}
	return fallback
}

func (s *SettingService) GetSubClashExt() (string, error) {
	// Subscription extensions may be several MiB. Do not retain them in the
	// runtime settings cache after a request finishes.
	return s.getString("subClashExt")
}

func normalizeAutoSyncClientIDs(ids []uint) []uint {
	if len(ids) == 0 {
		return []uint{}
	}

	seen := make(map[uint]struct{}, len(ids))
	cleaned := make([]uint, 0, len(ids))
	for _, id := range ids {
		if id == 0 {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		cleaned = append(cleaned, id)
	}

	sort.Slice(cleaned, func(i, j int) bool {
		return cleaned[i] < cleaned[j]
	})

	return cleaned
}

func (s *SettingService) getAutoSyncClientIDs(key string) ([]uint, error) {
	raw, err := s.getString(key)
	if err != nil {
		return nil, err
	}

	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return []uint{}, nil
	}

	var ids []uint
	if err := json.Unmarshal([]byte(trimmed), &ids); err != nil {
		logger.Warningf("invalid auto sync client id list for %s: %v", key, err)
		return []uint{}, nil
	}
	return normalizeAutoSyncClientIDs(ids), nil
}

// saveAutoSyncClientIDsTx updates one namespace's auto-sync registry while
// holding the caller's SQLite transaction. Keeping the read/modify/write in
// one transaction prevents two concurrent row actions from losing each
// other's client ID.
func (s *SettingService) saveAutoSyncClientIDsTx(tx *gorm.DB, key string, ids []uint) (bool, error) {
	if tx == nil {
		return false, common.NewError("database transaction is not ready")
	}
	normalized := normalizeAutoSyncClientIDs(ids)
	raw, err := json.Marshal(normalized)
	if err != nil {
		return false, err
	}

	currentRevision, err := ensureSettingsRevisionState(tx)
	if err != nil {
		return false, err
	}
	var current model.Setting
	lookupErr := tx.Where("key = ?", key).First(&current).Error
	if lookupErr == nil && current.Value == string(raw) {
		return false, nil
	}
	if lookupErr != nil && !database.IsNotFound(lookupErr) {
		return false, lookupErr
	}
	if err := upsertSetting(tx, key, string(raw)); err != nil {
		return false, err
	}
	updated := tx.Model(&model.SettingsState{}).
		Where("id = ? AND revision = ?", 1, currentRevision).
		Update("revision", gorm.Expr("revision + ?", 1))
	if updated.Error != nil {
		return false, updated.Error
	}
	if updated.RowsAffected != 1 {
		return false, common.NewError("设置版本更新冲突")
	}
	return true, nil
}

func (s *SettingService) saveAutoSyncClientIDs(key string, ids []uint, markRevision func()) error {
	db := database.GetDB()
	if db == nil {
		return common.NewError("database is not ready")
	}
	changed := false
	err := db.Transaction(func(tx *gorm.DB) error {
		var err error
		changed, err = s.saveAutoSyncClientIDsTx(tx, key, ids)
		return err
	})
	if err != nil {
		return err
	}
	if changed && markRevision != nil {
		markRevision()
	}
	return nil
}

func toggleAutoSyncClientID(ids []uint, clientID uint, enabled bool) []uint {
	normalized := normalizeAutoSyncClientIDs(ids)
	if clientID == 0 {
		return normalized
	}

	if enabled {
		return normalizeAutoSyncClientIDs(append(normalized, clientID))
	}

	filtered := make([]uint, 0, len(normalized))
	for _, id := range normalized {
		if id == clientID {
			continue
		}
		filtered = append(filtered, id)
	}
	return filtered
}

func (s *SettingService) GetSubManagerAutoSyncClientIDs() ([]uint, error) {
	return s.getAutoSyncClientIDs("subManagerAutoSyncClientIds")
}

func (s *SettingService) SetSubManagerAutoSyncClient(clientID uint, enabled bool) error {
	return s.updateAutoSyncClientID("subManagerAutoSyncClientIds", clientID, enabled, func() {
		markLastUpdate(time.Now().Unix())
	})
}

// SetSubManagerAutoSyncClientTx updates the default-chain auto-sync registry
// inside an existing transaction. Callers must mark the appropriate polling
// revision after the surrounding transaction commits.
func (s *SettingService) SetSubManagerAutoSyncClientTx(tx *gorm.DB, clientID uint, enabled bool) (bool, error) {
	return s.updateAutoSyncClientIDTx(tx, "subManagerAutoSyncClientIds", clientID, enabled)
}

func (s *SettingService) SaveSubManagerAutoSyncClientIDs(ids []uint) error {
	return s.saveAutoSyncClientIDs("subManagerAutoSyncClientIds", ids, func() {
		markLastUpdate(time.Now().Unix())
	})
}

func (s *SettingService) GetSubManagerAutoSyncMihomoClientIDs() ([]uint, error) {
	return s.getAutoSyncClientIDs("subManagerAutoSyncMihomoClientIds")
}

func (s *SettingService) SetSubManagerAutoSyncMihomoClient(clientID uint, enabled bool) error {
	return s.updateAutoSyncClientID("subManagerAutoSyncMihomoClientIds", clientID, enabled, func() {
		markMihomoLastUpdate(time.Now().Unix())
	})
}

// SetSubManagerAutoSyncMihomoClientTx updates the Mihomo auto-sync registry
// inside an existing transaction. Callers must mark the appropriate polling
// revision after the surrounding transaction commits.
func (s *SettingService) SetSubManagerAutoSyncMihomoClientTx(tx *gorm.DB, clientID uint, enabled bool) (bool, error) {
	return s.updateAutoSyncClientIDTx(tx, "subManagerAutoSyncMihomoClientIds", clientID, enabled)
}

func (s *SettingService) SaveSubManagerAutoSyncMihomoClientIDs(ids []uint) error {
	return s.saveAutoSyncClientIDs("subManagerAutoSyncMihomoClientIds", ids, func() {
		markMihomoLastUpdate(time.Now().Unix())
	})
}

func (s *SettingService) updateAutoSyncClientID(key string, clientID uint, enabled bool, markRevision func()) error {
	if clientID == 0 {
		return nil
	}
	db := database.GetDB()
	if db == nil {
		return common.NewError("database is not ready")
	}
	changed := false
	err := db.Transaction(func(tx *gorm.DB) error {
		var err error
		changed, err = s.updateAutoSyncClientIDTx(tx, key, clientID, enabled)
		return err
	})
	if err != nil {
		return err
	}
	if changed && markRevision != nil {
		markRevision()
	}
	return nil
}

func (s *SettingService) updateAutoSyncClientIDTx(tx *gorm.DB, key string, clientID uint, enabled bool) (bool, error) {
	if tx == nil {
		return false, common.NewError("database transaction is not ready")
	}
	if clientID == 0 {
		return false, nil
	}
	raw, err := s.getStringTx(tx, key)
	if err != nil {
		return false, err
	}
	var ids []uint
	if trimmed := strings.TrimSpace(raw); trimmed != "" {
		if err := json.Unmarshal([]byte(trimmed), &ids); err != nil {
			logger.Warningf("invalid auto sync client id list for %s: %v", key, err)
			ids = []uint{}
		}
	}
	ids = toggleAutoSyncClientID(ids, clientID, enabled)
	return s.saveAutoSyncClientIDsTx(tx, key, ids)
}

func (s *SettingService) getStringTx(tx *gorm.DB, key string) (string, error) {
	if key == "subPath" {
		return generateRandomSubPath(), nil
	}
	if key == "subPort" {
		port, err := chooseInitialRandomSubPort()
		if err != nil {
			return "", err
		}
		return strconv.Itoa(port), nil
	}
	if key == "timeLocation" {
		return defaultTimeLocationValue(), nil
	}
	setting := &model.Setting{}
	err := tx.Model(model.Setting{}).Where("key = ?", key).Order("id DESC").First(setting).Error
	if database.IsNotFound(err) {
		value, valueErr := s.defaultSettingValue(key)
		if valueErr != nil {
			return "", valueErr
		}
		return value, nil
	}
	if err != nil {
		return "", err
	}
	return setting.Value, nil
}
