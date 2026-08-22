package service

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
)

var (
	subscriptionTLSPathWatchMu          sync.Mutex
	subscriptionTLSPathWatchInitialized bool
	subscriptionTLSPathWatchLastDigest  string
	subscriptionTLSPathWatchLastEntries map[string]subscriptionTLSPathDigestRecord
)

type subscriptionTLSPathBinding struct {
	SourceType string
	TLSID      uint
	Section    string
	Key        string
	Path       string
	ReadErr    error
}

var subscriptionTLSPathBindings = struct {
	mu      sync.Mutex
	valid   bool
	entries []subscriptionTLSPathBinding
}{}

var subscriptionTLSPathKeys = []string{
	"certificate_path",
	"key_path",
	"client_certificate_path",
	"client_key_path",
}

type subscriptionTLSPathEntry struct {
	SourceType string
	TLSID      uint
	Section    string
	Key        string
	Path       string
	Digest     string
	Data       []byte
	ReadErr    error
	Issue      string
}

type subscriptionTLSPathState struct {
	Digest      string
	WatchedFile int
	Issues      []string
	Entries     map[string]subscriptionTLSPathDigestRecord
}

type subscriptionTLSPathDigestRecord struct {
	SourceType string
	TLSID      uint
	Digest     string
}

type subscriptionTLSPathFileCacheEntry struct {
	Identity tlsPathFileIdentity
	Digest   string
	Issue    string
}

var (
	subscriptionTLSPathFileCacheMu sync.Mutex
	subscriptionTLSPathFileCache   = map[string]subscriptionTLSPathFileCacheEntry{}
)

func init() {
	database.RegisterDBResetHook(func() {
		InvalidateSubscriptionTLSPathWatchBindings()
		subscriptionTLSPathFileCacheMu.Lock()
		subscriptionTLSPathFileCache = map[string]subscriptionTLSPathFileCacheEntry{}
		subscriptionTLSPathFileCacheMu.Unlock()
	})
}

// CheckAndSyncAutoManagedSubscriptionsOnTLSPathChange checks whether any watched
// TLS path-backed certificate/key material changed and triggers managed
// subscription sync when needed.
func CheckAndSyncAutoManagedSubscriptionsOnTLSPathChange(hostname string) (bool, error) {
	subscriptionTLSPathWatchMu.Lock()
	defer subscriptionTLSPathWatchMu.Unlock()

	state, err := buildSubscriptionTLSPathState()
	if err != nil {
		SetSubscriptionTLSLoginWarning("TLS path certificate check failed: " + err.Error())
		return false, err
	}
	SetSubscriptionTLSLoginWarning(buildSubscriptionTLSPathWarning(state.Issues))

	changed := !subscriptionTLSPathWatchInitialized || state.Digest != subscriptionTLSPathWatchLastDigest
	defaultTLSIDs, mihomoTLSIDs := changedTLSPathBindingIDs(
		state.Entries,
		subscriptionTLSPathWatchLastEntries,
		!subscriptionTLSPathWatchInitialized,
	)
	subscriptionTLSPathWatchInitialized = true
	subscriptionTLSPathWatchLastDigest = state.Digest
	subscriptionTLSPathWatchLastEntries = cloneSubscriptionTLSPathDigestRecords(state.Entries)

	if !changed || state.WatchedFile == 0 {
		return false, nil
	}

	synced, err := ForceSyncTLSPathBindingsForTLSIDs(defaultTLSIDs, mihomoTLSIDs, hostname)
	if err != nil {
		return false, err
	}
	if synced {
		markBothLastUpdates(time.Now().Unix())
	}

	return changed, nil
}

// InvalidateSubscriptionTLSPathWatchBindings makes the next scheduled scan
// rebuild path references from TLS records. Normal scans only inspect those
// already known paths, avoiding repeated JSON decoding of every TLS record.
func InvalidateSubscriptionTLSPathWatchBindings() {
	subscriptionTLSPathBindings.mu.Lock()
	subscriptionTLSPathBindings.valid = false
	subscriptionTLSPathBindings.entries = nil
	subscriptionTLSPathBindings.mu.Unlock()
}

func buildSubscriptionTLSPathDigest() (string, int, error) {
	state, err := buildSubscriptionTLSPathState()
	if err != nil {
		return "", 0, err
	}
	return state.Digest, state.WatchedFile, nil
}

func buildSubscriptionTLSPathState() (*subscriptionTLSPathState, error) {
	bindings, err := loadSubscriptionTLSPathBindings()
	if err != nil {
		return nil, err
	}
	return buildSubscriptionTLSPathStateFromBindings(bindings)
}

func loadSubscriptionTLSPathBindings() ([]subscriptionTLSPathBinding, error) {
	subscriptionTLSPathBindings.mu.Lock()
	defer subscriptionTLSPathBindings.mu.Unlock()
	if subscriptionTLSPathBindings.valid {
		return append([]subscriptionTLSPathBinding(nil), subscriptionTLSPathBindings.entries...), nil
	}

	db := database.GetDB()

	defaultTLS := make([]model.Tls, 0)
	if err := db.Model(model.Tls{}).Select("id", "server", "client").Find(&defaultTLS).Error; err != nil {
		return nil, err
	}

	mihomoTLS := make([]model.MihomoTls, 0)
	if err := db.Model(model.MihomoTls{}).Select("id", "server", "client").Find(&mihomoTLS).Error; err != nil {
		return nil, err
	}

	entries := make([]subscriptionTLSPathBinding, 0, len(defaultTLS)*6+len(mihomoTLS)*6)
	for _, tlsConfig := range defaultTLS {
		appendTLSPathBindings(&entries, "default", tlsConfig.Id, tlsConfig.Server, "server")
		appendTLSPathBindings(&entries, "default", tlsConfig.Id, tlsConfig.Client, "client")
	}
	for _, tlsConfig := range mihomoTLS {
		appendTLSPathBindings(&entries, "mihomo", tlsConfig.Id, tlsConfig.Server, "server")
		appendTLSPathBindings(&entries, "mihomo", tlsConfig.Id, tlsConfig.Client, "client")
	}
	subscriptionTLSPathBindings.entries = entries
	subscriptionTLSPathBindings.valid = true
	return append([]subscriptionTLSPathBinding(nil), entries...), nil
}

func buildSubscriptionTLSPathStateFromBindings(bindings []subscriptionTLSPathBinding) (*subscriptionTLSPathState, error) {
	if len(bindings) == 0 {
		return &subscriptionTLSPathState{
			Digest:      "",
			WatchedFile: 0,
			Issues:      []string{},
			Entries:     map[string]subscriptionTLSPathDigestRecord{},
		}, nil
	}

	currentPaths := make(map[string]struct{}, len(bindings))
	digestEntries := make([]string, 0, len(bindings))
	digestRecords := make(map[string]subscriptionTLSPathDigestRecord, len(bindings))
	issues := make([]string, 0)
	seenIssue := make(map[string]struct{}, len(bindings))
	for _, binding := range bindings {
		entry := subscriptionTLSPathEntry{
			SourceType: binding.SourceType,
			TLSID:      binding.TLSID,
			Section:    binding.Section,
			Key:        binding.Key,
			Path:       binding.Path,
			ReadErr:    binding.ReadErr,
		}
		if entry.ReadErr == nil && entry.Path != "" {
			entry.Digest, entry.Data, entry.Issue, entry.ReadErr = digestTLSPathFile(entry.Path, entry.Key)
		} else if entry.ReadErr != nil {
			entry.Digest = "invalid_json"
		}
		if entry.Path != "" {
			currentPaths[entry.Path] = struct{}{}
		}
		digestEntries = append(digestEntries, formatTLSPathDigestEntry(entry))
		digestRecords[formatTLSPathDigestIdentity(entry)] = subscriptionTLSPathDigestRecord{
			SourceType: entry.SourceType,
			TLSID:      entry.TLSID,
			Digest:     entry.Digest,
		}
		issue := entry.Issue
		if issue == "" {
			issue = validateTLSPathEntry(entry)
		}
		if issue != "" {
			if _, exists := seenIssue[issue]; !exists {
				seenIssue[issue] = struct{}{}
				issues = append(issues, issue)
			}
		}
	}
	pruneSubscriptionTLSPathFileCache(currentPaths)
	sort.Strings(digestEntries)
	sort.Strings(issues)

	joined := strings.Join(digestEntries, "\n")
	sum := sha256.Sum256([]byte(joined))
	return &subscriptionTLSPathState{
		Digest:      hex.EncodeToString(sum[:]),
		WatchedFile: len(bindings),
		Issues:      issues,
		Entries:     digestRecords,
	}, nil
}

func appendTLSPathBindings(entries *[]subscriptionTLSPathBinding, sourceType string, tlsID uint, raw json.RawMessage, section string) {
	if len(raw) == 0 {
		return
	}

	payload := map[string]interface{}{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		*entries = append(*entries, subscriptionTLSPathBinding{
			SourceType: sourceType,
			TLSID:      tlsID,
			Section:    section,
			Key:        "raw_json",
			ReadErr:    err,
		})
		return
	}

	pathEntries := collectTLSPathEntries(payload)
	for _, item := range pathEntries {
		*entries = append(*entries, subscriptionTLSPathBinding{
			SourceType: sourceType,
			TLSID:      tlsID,
			Section:    section,
			Key:        item.Key,
			Path:       item.Path,
		})
	}
}

func digestTLSPathFile(path string, key string) (string, []byte, string, error) {
	identity, err := inspectTLSPathMaterial(path)
	if err != nil {
		return "err:" + err.Error(), nil, "", err
	}

	cacheKey := subscriptionTLSPathFileCacheKey(path, key)
	subscriptionTLSPathFileCacheMu.Lock()
	cached, found := subscriptionTLSPathFileCache[cacheKey]
	if found && cached.Identity == identity {
		subscriptionTLSPathFileCacheMu.Unlock()
		return cached.Digest, nil, cached.Issue, nil
	}
	subscriptionTLSPathFileCacheMu.Unlock()

	data, identity, err := readTLSPathMaterial(path)
	if err != nil {
		return "err:" + err.Error(), nil, "", err
	}

	sum := sha256.Sum256(data)
	digest := "ok:" + hex.EncodeToString(sum[:])
	entry := subscriptionTLSPathEntry{Path: path, Key: key, Data: data}
	issue := validateTLSPathEntry(entry)

	subscriptionTLSPathFileCacheMu.Lock()
	subscriptionTLSPathFileCache[cacheKey] = subscriptionTLSPathFileCacheEntry{
		Identity: identity,
		Digest:   digest,
		Issue:    issue,
	}
	subscriptionTLSPathFileCacheMu.Unlock()
	return digest, data, issue, nil
}

func pruneSubscriptionTLSPathFileCache(currentPaths map[string]struct{}) {
	subscriptionTLSPathFileCacheMu.Lock()
	defer subscriptionTLSPathFileCacheMu.Unlock()
	for cacheKey := range subscriptionTLSPathFileCache {
		path := subscriptionTLSPathFileCachePath(cacheKey)
		if _, exists := currentPaths[path]; !exists {
			delete(subscriptionTLSPathFileCache, cacheKey)
		}
	}
}

func subscriptionTLSPathFileCacheKey(path string, key string) string {
	return path + "\x00" + key
}

func subscriptionTLSPathFileCachePath(cacheKey string) string {
	if separator := strings.IndexByte(cacheKey, 0); separator >= 0 {
		return cacheKey[:separator]
	}
	return cacheKey
}

type tlsPathItem struct {
	Key  string
	Path string
}

func collectTLSPathEntries(raw interface{}) []tlsPathItem {
	result := make([]tlsPathItem, 0)
	seen := make(map[string]struct{})
	var walk func(interface{})
	walk = func(value interface{}) {
		switch typed := value.(type) {
		case map[string]interface{}:
			for key, child := range typed {
				if isWatchedTLSPathKey(key) {
					if path, ok := child.(string); ok {
						path = strings.TrimSpace(path)
						if path != "" {
							identity := key + "\n" + path
							if _, exists := seen[identity]; !exists {
								seen[identity] = struct{}{}
								result = append(result, tlsPathItem{Key: key, Path: path})
							}
						}
					}
				}
				walk(child)
			}
		case []interface{}:
			for _, child := range typed {
				walk(child)
			}
		}
	}
	walk(raw)
	return result
}

func isWatchedTLSPathKey(key string) bool {
	for _, watched := range subscriptionTLSPathKeys {
		if key == watched {
			return true
		}
	}
	return false
}

func formatTLSPathDigestEntry(entry subscriptionTLSPathEntry) string {
	return fmt.Sprintf(
		"%s:%d:%s:%s:%s=%s",
		entry.SourceType,
		entry.TLSID,
		entry.Section,
		entry.Key,
		entry.Path,
		entry.Digest,
	)
}

func formatTLSPathDigestIdentity(entry subscriptionTLSPathEntry) string {
	return fmt.Sprintf(
		"%s:%d:%s:%s:%s",
		entry.SourceType,
		entry.TLSID,
		entry.Section,
		entry.Key,
		entry.Path,
	)
}

func changedTLSPathBindingIDs(current map[string]subscriptionTLSPathDigestRecord, previous map[string]subscriptionTLSPathDigestRecord, firstRun bool) ([]uint, []uint) {
	defaultIDs := make([]uint, 0)
	mihomoIDs := make([]uint, 0)
	add := func(record subscriptionTLSPathDigestRecord) {
		if record.TLSID == 0 {
			return
		}
		switch record.SourceType {
		case "default":
			defaultIDs = append(defaultIDs, record.TLSID)
		case "mihomo":
			mihomoIDs = append(mihomoIDs, record.TLSID)
		}
	}

	if firstRun {
		for _, record := range current {
			add(record)
		}
		return compactPositiveUintList(defaultIDs), compactPositiveUintList(mihomoIDs)
	}

	for key, record := range current {
		old, exists := previous[key]
		if !exists || old.Digest != record.Digest {
			add(record)
		}
	}
	for key, record := range previous {
		if _, exists := current[key]; !exists {
			add(record)
		}
	}
	return compactPositiveUintList(defaultIDs), compactPositiveUintList(mihomoIDs)
}

func cloneSubscriptionTLSPathDigestRecords(src map[string]subscriptionTLSPathDigestRecord) map[string]subscriptionTLSPathDigestRecord {
	if len(src) == 0 {
		return map[string]subscriptionTLSPathDigestRecord{}
	}
	dst := make(map[string]subscriptionTLSPathDigestRecord, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func validateTLSPathEntry(entry subscriptionTLSPathEntry) string {
	prefix := fmt.Sprintf(
		"%s TLS[%d] %s.%s (%s)",
		localizeTLSSourceType(entry.SourceType),
		entry.TLSID,
		entry.Section,
		entry.Key,
		entry.Path,
	)

	if entry.ReadErr != nil {
		return prefix + " read failed: " + entry.ReadErr.Error()
	}
	if len(strings.TrimSpace(string(entry.Data))) == 0 {
		return prefix + " file is empty"
	}
	if isCertificatePathKey(entry.Key) {
		if err := validateCertificatePEM(entry.Data); err != nil {
			return prefix + " certificate content is invalid: " + err.Error()
		}
		return ""
	}
	if isPrivateKeyPathKey(entry.Key) {
		if err := validatePrivateKeyPEM(entry.Data); err != nil {
			return prefix + " private key content is invalid: " + err.Error()
		}
	}
	return ""
}

func localizeTLSSourceType(sourceType string) string {
	switch sourceType {
	case "default":
		return "default"
	case "mihomo":
		return "mihomo"
	default:
		return sourceType
	}
}

func isCertificatePathKey(key string) bool {
	return strings.Contains(strings.ToLower(key), "certificate")
}

func isPrivateKeyPathKey(key string) bool {
	return strings.Contains(strings.ToLower(key), "key")
}

func validateCertificatePEM(data []byte) error {
	rest := data
	found := false
	for len(rest) > 0 {
		block, remaining := pem.Decode(rest)
		if block == nil {
			break
		}
		rest = remaining
		if block.Type != "CERTIFICATE" {
			continue
		}
		if _, err := x509.ParseCertificate(block.Bytes); err != nil {
			return err
		}
		found = true
	}
	if !found {
		return fmt.Errorf("CERTIFICATE PEM block not found")
	}
	return nil
}

func validatePrivateKeyPEM(data []byte) error {
	rest := data
	parseErr := fmt.Errorf("PRIVATE KEY PEM block not found")
	for len(rest) > 0 {
		block, remaining := pem.Decode(rest)
		if block == nil {
			break
		}
		rest = remaining
		if !strings.Contains(block.Type, "PRIVATE KEY") {
			continue
		}

		if _, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
			return nil
		} else {
			parseErr = err
		}
		if _, err := x509.ParseECPrivateKey(block.Bytes); err == nil {
			return nil
		} else {
			parseErr = err
		}
		if _, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
			return nil
		} else {
			parseErr = err
		}
	}
	return parseErr
}

func buildSubscriptionTLSPathWarning(issues []string) string {
	if len(issues) == 0 {
		return ""
	}

	const maxItems = 3
	display := issues
	if len(display) > maxItems {
		display = display[:maxItems]
	}

	message := "Detected TLS path certificate/key issues. Please fix them in TLS settings: "
	message += strings.Join(display, "; ")
	if len(issues) > len(display) {
		message += fmt.Sprintf("; and %d more", len(issues)-len(display))
	}
	return message
}
