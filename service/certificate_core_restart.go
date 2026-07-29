package service

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/logger"
	"gopkg.in/yaml.v3"
)

const (
	certificateCoreKindSingbox = "singbox"
	certificateCoreKindMihomo  = "mihomo"

	certificateCoreRestartStateSettingKey = "certificateCoreRestartStateV1"
	certificateCoreRestartRetryRapid      = "rapid_retry"
	certificateCoreRestartRetryPeriodic   = "periodic_retry"
	certificateCoreRestartRapidLimit      = 3
)

type certificateCoreRestarter interface {
	IsRunning() bool
	RestartCore() error
}

type certificateCoreRestartState struct {
	Pending         bool   `json:"pending"`
	WindowStartedAt int64  `json:"windowStartedAt"`
	WindowEndsAt    int64  `json:"windowEndsAt"`
	RetryPhase      string `json:"retryPhase"`
	RetryCount      int    `json:"retryCount"`
	NextRetryAt     int64  `json:"nextRetryAt"`
	LastError       string `json:"lastError"`
	LastRecordID    uint   `json:"lastRecordId"`
	LastFingerprint string `json:"lastFingerprint"`
	Generation      uint64 `json:"generation"`
}

type certificateCoreRestartPersistentState struct {
	Singbox certificateCoreRestartState `json:"singbox"`
	Mihomo  certificateCoreRestartState `json:"mihomo"`
}

type certificateCoreRestartCoordinatorState struct {
	mu        sync.Mutex
	loaded    bool
	singbox   certificateCoreRestarter
	mihomo    certificateCoreRestarter
	persisted certificateCoreRestartPersistentState
}

var (
	certificateCoreConfigGate         sync.Mutex
	certificateCoreRestartCoordinator = &certificateCoreRestartCoordinatorState{}
)

// RegisterCertificateCoreRestartManagers injects the same process-owning
// manager instances used by the Web API. This is required on Windows because a
// fresh manager cannot safely stop a process held by another instance.
func RegisterCertificateCoreRestartManagers(singbox *CoreManagerService, mihomo *MihomoCoreManagerService) {
	coordinator := certificateCoreRestartCoordinator
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	coordinator.singbox = singbox
	coordinator.mihomo = mihomo
	coordinator.loaded = false
	coordinator.loadLocked()
}

func withCertificateCoreConfigGate(run func() error) error {
	certificateCoreConfigGate.Lock()
	defer certificateCoreConfigGate.Unlock()
	if run == nil {
		return nil
	}
	return run()
}

// WithCertificateCoreConfigGate coordinates manual Core lifecycle actions with
// certificate-driven TLS/config updates.
func WithCertificateCoreConfigGate(run func() error) error {
	return withCertificateCoreConfigGate(run)
}

func certificateCoreManagerRunning(kind string) bool {
	coordinator := certificateCoreRestartCoordinator
	coordinator.mu.Lock()
	manager := coordinator.managerLocked(kind)
	coordinator.mu.Unlock()
	return manager != nil && manager.IsRunning()
}

func queueCertificateCoreRestart(kind string, recordID uint, fingerprint string) error {
	coordinator := certificateCoreRestartCoordinator
	nowUnix := time.Now().Unix()
	windowStartedAt := nowUnix
	windowEndsAt := nowUnix + int64(acmeAutoRenewBatchDuration/time.Second)
	if batchStartedAt, batchEndsAt, active := currentCertificateAutoRenewBatchWindow(); active {
		windowStartedAt = batchStartedAt
		windowEndsAt = batchEndsAt
	}

	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	coordinator.loadLocked()
	state := coordinator.stateLocked(kind)
	if state == nil {
		return fmt.Errorf("unsupported certificate Core kind: %s", kind)
	}
	if !state.Pending {
		state.Pending = true
		state.WindowStartedAt = windowStartedAt
		state.WindowEndsAt = windowEndsAt
		state.RetryPhase = ""
		state.RetryCount = 0
		state.NextRetryAt = 0
		state.LastError = ""
	}
	state.LastRecordID = recordID
	state.LastFingerprint = normalizeCertificateFingerprint(fingerprint)
	state.Generation++
	if err := coordinator.persistLocked(); err != nil {
		return err
	}
	logger.Infof("certificate update queued %s Core restart: record=%d window_end=%d", kind, recordID, state.WindowEndsAt)
	return nil
}

// ProcessCertificateCoreRestartQueue is intentionally lightweight and may run
// every minute. It never starts a stopped Core and it defers while an automatic
// renewal batch is still accepting candidates or resolving rapid retries.
func ProcessCertificateCoreRestartQueue() error {
	if certificateCoreRestartMustWaitForRenewalBatch() {
		return nil
	}
	errors := make([]string, 0, 2)
	for _, kind := range []string{certificateCoreKindSingbox, certificateCoreKindMihomo} {
		if err := processCertificateCoreRestart(kind, time.Now().Unix()); err != nil {
			errors = append(errors, err.Error())
		}
	}
	if len(errors) > 0 {
		return fmt.Errorf("%s", strings.Join(errors, "; "))
	}
	return nil
}

func processCertificateCoreRestart(kind string, nowUnix int64) error {
	coordinator := certificateCoreRestartCoordinator
	coordinator.mu.Lock()
	coordinator.loadLocked()
	state := coordinator.stateLocked(kind)
	manager := coordinator.managerLocked(kind)
	if state == nil || !state.Pending || manager == nil {
		coordinator.mu.Unlock()
		return nil
	}
	dueAt := state.WindowEndsAt
	if state.RetryPhase != "" {
		dueAt = state.NextRetryAt
	}
	if dueAt <= 0 || nowUnix < dueAt {
		coordinator.mu.Unlock()
		return nil
	}
	coordinator.mu.Unlock()

	return withCertificateCoreConfigGate(func() error {
		if certificateCoreRestartMustWaitForRenewalBatch() {
			return nil
		}
		coordinator.mu.Lock()
		currentState := coordinator.stateLocked(kind)
		currentManager := coordinator.managerLocked(kind)
		stillDue := currentState != nil && currentState.Pending && currentManager == manager
		if stillDue {
			currentDueAt := currentState.WindowEndsAt
			if currentState.RetryPhase != "" {
				currentDueAt = currentState.NextRetryAt
			}
			stillDue = currentDueAt > 0 && nowUnix >= currentDueAt
		}
		coordinator.mu.Unlock()
		if !stillDue {
			return nil
		}
		if !manager.IsRunning() {
			clearCertificateCoreRestartState(kind, "Core is stopped; latest config will be loaded on the next manual start")
			return nil
		}

		restartErr := manager.RestartCore()
		if restartErr == nil {
			clearCertificateCoreRestartState(kind, "")
			logger.Info("certificate update restarted Core successfully: ", kind)
			return nil
		}
		if err := recordCertificateCoreRestartFailure(kind, nowUnix, restartErr); err != nil {
			return fmt.Errorf("%s Core restart failed: %v (save retry state failed: %v)", kind, restartErr, err)
		}
		return fmt.Errorf("%s Core restart failed: %w", kind, restartErr)
	})
}

func certificateCoreRestartMustWaitForRenewalBatch() bool {
	if acmeAutoRenewRunning.Load() || isCertificateAutoRenewBatchOpen() {
		return true
	}
	if database.GetDB() == nil {
		return false
	}
	query := database.GetDB().Model(&model.CertificateRecord{}).
		Where("auto_renew = ? AND auto_renew_retry_phase = ?", true, acmeAutoRenewRetryPhaseRapid)
	if !IsSystemPlatformLinux() {
		query = query.Where("source_type = ?", CertificateSourceSelfSigned)
	}
	count := int64(0)
	if err := query.Count(&count).Error; err != nil {
		logger.Warning("check pending certificate rapid retries failed: ", err)
		return true
	}
	return count > 0
}

func recordCertificateCoreRestartFailure(kind string, nowUnix int64, restartErr error) error {
	coordinator := certificateCoreRestartCoordinator
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	coordinator.loadLocked()
	state := coordinator.stateLocked(kind)
	if state == nil || !state.Pending {
		return nil
	}
	switch state.RetryPhase {
	case certificateCoreRestartRetryRapid:
		state.RetryCount++
		if state.RetryCount >= certificateCoreRestartRapidLimit {
			state.RetryPhase = certificateCoreRestartRetryPeriodic
			state.RetryCount = certificateCoreRestartRapidLimit
			state.NextRetryAt = nowUnix + int64(acmeAutoRenewPeriodicRetryInterval/time.Second)
		} else {
			state.NextRetryAt = nowUnix + int64(acmeAutoRenewRapidRetryInterval/time.Second)
		}
	case certificateCoreRestartRetryPeriodic:
		state.RetryCount = certificateCoreRestartRapidLimit
		state.NextRetryAt = nowUnix + int64(acmeAutoRenewPeriodicRetryInterval/time.Second)
	default:
		state.RetryPhase = certificateCoreRestartRetryRapid
		state.RetryCount = 0
		state.NextRetryAt = nowUnix + int64(acmeAutoRenewRapidRetryInterval/time.Second)
	}
	state.LastError = strings.TrimSpace(restartErr.Error())
	return coordinator.persistLocked()
}

func notifyCertificateCoreLoadedLatestConfig(kind string) {
	clearCertificateCoreRestartState(kind, "")
}

func clearCertificateCoreRestartState(kind string, reason string) {
	coordinator := certificateCoreRestartCoordinator
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	coordinator.loadLocked()
	state := coordinator.stateLocked(kind)
	if state == nil || !state.Pending {
		return
	}
	*state = certificateCoreRestartState{}
	if err := coordinator.persistLocked(); err != nil {
		logger.Warning("persist cleared certificate Core restart state failed: ", err)
	}
	if strings.TrimSpace(reason) != "" {
		logger.Info("cleared pending certificate Core restart for ", kind, ": ", reason)
	}
}

func (c *certificateCoreRestartCoordinatorState) managerLocked(kind string) certificateCoreRestarter {
	switch strings.TrimSpace(kind) {
	case certificateCoreKindSingbox:
		return c.singbox
	case certificateCoreKindMihomo:
		return c.mihomo
	default:
		return nil
	}
}

func (c *certificateCoreRestartCoordinatorState) stateLocked(kind string) *certificateCoreRestartState {
	switch strings.TrimSpace(kind) {
	case certificateCoreKindSingbox:
		return &c.persisted.Singbox
	case certificateCoreKindMihomo:
		return &c.persisted.Mihomo
	default:
		return nil
	}
}

func (c *certificateCoreRestartCoordinatorState) loadLocked() {
	if c.loaded {
		return
	}
	c.loaded = true
	c.persisted = certificateCoreRestartPersistentState{}
	if database.GetDB() == nil {
		return
	}
	setting, err := (&SettingService{}).getSetting(certificateCoreRestartStateSettingKey)
	if database.IsNotFound(err) {
		return
	}
	if err != nil {
		logger.Warning("load certificate Core restart state failed: ", err)
		return
	}
	if err := json.Unmarshal([]byte(setting.Value), &c.persisted); err != nil {
		logger.Warning("parse certificate Core restart state failed: ", err)
		c.persisted = certificateCoreRestartPersistentState{}
	}
}

func (c *certificateCoreRestartCoordinatorState) persistLocked() error {
	if database.GetDB() == nil {
		return nil
	}
	raw, err := json.Marshal(c.persisted)
	if err != nil {
		return err
	}
	return (&SettingService{}).saveSetting(certificateCoreRestartStateSettingKey, string(raw))
}

func singboxFinalConfigContainsCertificateFingerprint(fingerprint string) (bool, error) {
	raw, err := ManagedRuntimeReadFile(GetSingboxConfigPath())
	if err != nil {
		return false, err
	}
	document := map[string]interface{}{}
	if err := json.Unmarshal(raw, &document); err != nil {
		return false, fmt.Errorf("parse final sing-box config: %w", err)
	}
	for _, section := range []string{"inbounds", "services"} {
		if certificateConfigValueContainsFingerprint(document[section], fingerprint) {
			return true, nil
		}
	}
	return false, nil
}

func mihomoFinalConfigContainsCertificateFingerprint(fingerprint string) (bool, error) {
	raw, err := ManagedRuntimeReadFile(GetMihomoConfigPath())
	if err != nil {
		return false, err
	}
	document := map[string]interface{}{}
	if err := yaml.Unmarshal(raw, &document); err != nil {
		return false, fmt.Errorf("parse final Mihomo config: %w", err)
	}
	return certificateConfigValueContainsFingerprint(document["listeners"], fingerprint), nil
}

func certificateConfigValueContainsFingerprint(value interface{}, fingerprint string) bool {
	target := normalizeCertificateFingerprint(fingerprint)
	if target == "" {
		return false
	}
	switch typed := value.(type) {
	case map[string]interface{}:
		for key, child := range typed {
			if strings.EqualFold(strings.TrimSpace(key), "certificate") {
				if certificateValueFingerprint(child) == target {
					return true
				}
			}
			if certificateConfigValueContainsFingerprint(child, target) {
				return true
			}
		}
	case []interface{}:
		for _, child := range typed {
			if certificateConfigValueContainsFingerprint(child, target) {
				return true
			}
		}
	}
	return false
}

func certificateValueFingerprint(value interface{}) string {
	var certificateText string
	switch typed := value.(type) {
	case string:
		certificateText = typed
	case []string:
		certificateText = strings.Join(typed, "\n")
	case []interface{}:
		lines := make([]string, 0, len(typed))
		for _, item := range typed {
			if line, ok := item.(string); ok {
				lines = append(lines, line)
			}
		}
		certificateText = strings.Join(lines, "\n")
	default:
		return ""
	}

	rest := []byte(strings.TrimSpace(certificateText))
	for len(rest) > 0 {
		block, next := pem.Decode(rest)
		if block == nil {
			return ""
		}
		rest = next
		if block.Type != "CERTIFICATE" {
			continue
		}
		leaf, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return ""
		}
		sum := sha256.Sum256(leaf.Raw)
		return hex.EncodeToString(sum[:])
	}
	return ""
}

func normalizeCertificateFingerprint(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	replacer := strings.NewReplacer(":", "", "-", "", " ", "", "\t", "", "\r", "", "\n", "")
	return replacer.Replace(value)
}
