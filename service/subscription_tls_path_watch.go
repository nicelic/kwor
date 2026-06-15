package service

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"os"
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

	if _, err := ForceSyncTLSPathBindingsForTLSIDs(defaultTLSIDs, mihomoTLSIDs, hostname); err != nil {
		return false, err
	}

	LastUpdate = time.Now().Unix()
	return true, nil
}

func buildSubscriptionTLSPathDigest() (string, int, error) {
	state, err := buildSubscriptionTLSPathState()
	if err != nil {
		return "", 0, err
	}
	return state.Digest, state.WatchedFile, nil
}

func buildSubscriptionTLSPathState() (*subscriptionTLSPathState, error) {
	db := database.GetDB()

	defaultTLS := make([]model.Tls, 0)
	if err := db.Model(model.Tls{}).Find(&defaultTLS).Error; err != nil {
		return nil, err
	}

	mihomoTLS := make([]model.MihomoTls, 0)
	if err := db.Model(model.MihomoTls{}).Find(&mihomoTLS).Error; err != nil {
		return nil, err
	}

	entries := make([]subscriptionTLSPathEntry, 0, len(defaultTLS)*6+len(mihomoTLS)*6)
	for _, tlsConfig := range defaultTLS {
		appendTLSPathDigestEntries(&entries, "default", tlsConfig.Id, tlsConfig.Server, "server")
		appendTLSPathDigestEntries(&entries, "default", tlsConfig.Id, tlsConfig.Client, "client")
	}
	for _, tlsConfig := range mihomoTLS {
		appendTLSPathDigestEntries(&entries, "mihomo", tlsConfig.Id, tlsConfig.Server, "server")
		appendTLSPathDigestEntries(&entries, "mihomo", tlsConfig.Id, tlsConfig.Client, "client")
	}

	if len(entries) == 0 {
		return &subscriptionTLSPathState{
			Digest:      "",
			WatchedFile: 0,
			Issues:      []string{},
			Entries:     map[string]subscriptionTLSPathDigestRecord{},
		}, nil
	}

	digestEntries := make([]string, 0, len(entries))
	digestRecords := make(map[string]subscriptionTLSPathDigestRecord, len(entries))
	issues := make([]string, 0)
	seenIssue := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		digestEntries = append(digestEntries, formatTLSPathDigestEntry(entry))
		digestRecords[formatTLSPathDigestIdentity(entry)] = subscriptionTLSPathDigestRecord{
			SourceType: entry.SourceType,
			TLSID:      entry.TLSID,
			Digest:     entry.Digest,
		}
		if issue := validateTLSPathEntry(entry); issue != "" {
			if _, exists := seenIssue[issue]; !exists {
				seenIssue[issue] = struct{}{}
				issues = append(issues, issue)
			}
		}
	}
	sort.Strings(digestEntries)
	sort.Strings(issues)

	joined := strings.Join(digestEntries, "\n")
	sum := sha256.Sum256([]byte(joined))
	return &subscriptionTLSPathState{
		Digest:      hex.EncodeToString(sum[:]),
		WatchedFile: len(entries),
		Issues:      issues,
		Entries:     digestRecords,
	}, nil
}

func appendTLSPathDigestEntries(entries *[]subscriptionTLSPathEntry, sourceType string, tlsID uint, raw json.RawMessage, section string) {
	if len(raw) == 0 {
		return
	}

	payload := map[string]interface{}{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		*entries = append(*entries, subscriptionTLSPathEntry{
			SourceType: sourceType,
			TLSID:      tlsID,
			Section:    section,
			Key:        "raw_json",
			Path:       "",
			Digest:     "invalid_json",
			ReadErr:    err,
		})
		return
	}

	pathEntries := collectTLSPathEntries(payload)
	for _, item := range pathEntries {
		digest, data, readErr := digestTLSPathFile(item.Path)
		*entries = append(*entries, subscriptionTLSPathEntry{
			SourceType: sourceType,
			TLSID:      tlsID,
			Section:    section,
			Key:        item.Key,
			Path:       item.Path,
			Digest:     digest,
			Data:       data,
			ReadErr:    readErr,
		})
	}
}

func digestTLSPathFile(path string) (string, []byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "err:" + err.Error(), nil, err
	}

	sum := sha256.Sum256(data)
	return "ok:" + hex.EncodeToString(sum[:]), data, nil
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
