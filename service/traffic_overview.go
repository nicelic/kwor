package service

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"net/http"
	"os"
	"os/exec"
	pathpkg "path"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alireza0/s-ui/config"
	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/logger"
	psnet "github.com/shirou/gopsutil/v4/net"
	"golang.org/x/sync/singleflight"
	"gorm.io/gorm"
)

type TrafficOverview struct {
	Source      string              `json:"source"`
	Interface   string              `json:"interface"`
	Enabled     bool                `json:"enabled"`
	Status      string              `json:"status"`
	Available   bool                `json:"available"`
	Up          int64               `json:"up"`
	Down        int64               `json:"down"`
	Total       int64               `json:"total"`
	AccumUp     int64               `json:"accumUp"`
	AccumDown   int64               `json:"accumDown"`
	AccumTotal  int64               `json:"accumTotal"`
	LimitGiB    float64             `json:"limitGiB"`
	ResetDay    int                 `json:"resetDay"`
	ExpiryDate  string              `json:"expiryDate,omitempty"`
	Expired     bool                `json:"expired"`
	NextResetAt int64               `json:"nextResetAt"`
	UpdatedAt   int64               `json:"updatedAt"`
	Vnstat      VnstatPackageStatus `json:"vnstat"`
	Error       string              `json:"error,omitempty"`
}

type VnstatPackageStatus struct {
	Supported       bool                   `json:"supported"`
	CanManage       bool                   `json:"canManage"`
	Installed       bool                   `json:"installed"`
	Managed         bool                   `json:"managed"`
	Ownership       string                 `json:"ownership,omitempty"`
	OwnershipState  string                 `json:"ownershipState,omitempty"`
	OwnershipHint   string                 `json:"ownershipHint,omitempty"`
	Running         bool                   `json:"running"`
	Version         string                 `json:"version,omitempty"`
	SystemFamily    string                 `json:"systemFamily,omitempty"`
	SystemID        string                 `json:"systemId,omitempty"`
	SystemVersion   string                 `json:"systemVersion,omitempty"`
	PackageManager  string                 `json:"packageManager,omitempty"`
	InstallMethod   string                 `json:"installMethod,omitempty"`
	BinaryPath      string                 `json:"binaryPath,omitempty"`
	FileCount       int                    `json:"fileCount"`
	DataPaths       []string               `json:"dataPaths,omitempty"`
	ExternalPaths   []string               `json:"externalPaths,omitempty"`
	ExternalUnits   []string               `json:"externalUnits,omitempty"`
	RuntimeConflict *VnstatRuntimeConflict `json:"runtimeConflict,omitempty"`
	ManageHint      string                 `json:"manageHint,omitempty"`
	Error           string                 `json:"error,omitempty"`
}

// VnstatRuntimeConflict is set only after a panel-managed daemon fails to
// start and a separate running vnstatd process is then observed.
type VnstatRuntimeConflict struct {
	Message    string   `json:"message"`
	Paths      []string `json:"paths,omitempty"`
	PIDs       []int    `json:"pids,omitempty"`
	Units      []string `json:"units,omitempty"`
	DetectedAt int64    `json:"detectedAt"`
}

type VnstatVersionOption struct {
	Value       string `json:"value"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	Available   bool   `json:"available"`
	Reason      string `json:"reason,omitempty"`
}

type VnstatVersionListResult struct {
	Versions []VnstatVersionOption `json:"versions"`
}

type VnstatVersionCheckResult struct {
	Supported      bool   `json:"supported"`
	CanManage      bool   `json:"canManage"`
	Installed      bool   `json:"installed"`
	Managed        bool   `json:"managed"`
	CurrentVersion string `json:"currentVersion"`
	LatestVersion  string `json:"latestVersion"`
	HasUpdate      bool   `json:"hasUpdate"`
	Source         string `json:"source,omitempty"`
	Message        string `json:"message"`
}

// VnstatInstallJobStatus describes the one vnStat installation task that may
// run at a time. Installing from a package manager or compiling a GitHub
// source release can take several minutes, so it must not be tied to the HTTP
// request lifetime.
type VnstatInstallJobStatus struct {
	ID               string `json:"id,omitempty"`
	Source           string `json:"source,omitempty"`
	State            string `json:"state"`
	Phase            string `json:"phase,omitempty"`
	CanCancel        bool   `json:"canCancel"`
	StopRequested    bool   `json:"stopRequested"`
	DeadlineExceeded bool   `json:"deadlineExceeded"`
	Error            string `json:"error,omitempty"`
	StartedAt        int64  `json:"startedAt,omitempty"`
	UpdatedAt        int64  `json:"updatedAt,omitempty"`
	DeadlineAt       int64  `json:"deadlineAt,omitempty"`
	FinishedAt       int64  `json:"finishedAt,omitempty"`
}

// VnstatRemovalJobStatus represents the destructive vnStat cleanup task. It
// is intentionally non-cancellable once accepted: stopping the daemon and a
// package-manager removal must be allowed to finish coherently.
type VnstatRemovalJobStatus struct {
	ID         string `json:"id,omitempty"`
	State      string `json:"state"`
	Phase      string `json:"phase,omitempty"`
	Error      string `json:"error,omitempty"`
	StartedAt  int64  `json:"startedAt,omitempty"`
	UpdatedAt  int64  `json:"updatedAt,omitempty"`
	FinishedAt int64  `json:"finishedAt,omitempty"`
}

type vnstatInstallJob struct {
	status VnstatInstallJobStatus
	task   *ManagedDownloadTaskHandle
}

// vnstatInstallProgressReporter returns false when the managed task was
// stopped before the reported phase could begin. This makes the transition
// into a host-changing phase atomic with a concurrent stop request.
type vnstatInstallProgressReporter func(phase string) bool

type trafficOverviewVnstatManifest struct {
	Managed        bool     `json:"managed"`
	Ownership      string   `json:"ownership"`
	SystemFamily   string   `json:"systemFamily"`
	PackageManager string   `json:"packageManager"`
	InstallMethod  string   `json:"installMethod"`
	PackageName    string   `json:"packageName"`
	Version        string   `json:"version"`
	BinaryPath     string   `json:"binaryPath"`
	FilePaths      []string `json:"filePaths"`
	DataPaths      []string `json:"dataPaths"`
	ServiceUnits   []string `json:"serviceUnits"`
	EvidenceNonce  string   `json:"evidenceNonce"`
	EvidenceSchema int      `json:"evidenceSchema"`
	InstalledAt    int64    `json:"installedAt"`
}

// trafficOverviewVnstatOwnershipEvidence is deliberately stored outside the
// database. A copied/restored SQLite database must not grant permission to
// stop or remove a vnStat installation on another host.
type trafficOverviewVnstatOwnershipEvidence struct {
	Schema          int                                 `json:"schema"`
	HostFingerprint string                              `json:"hostFingerprint"`
	Nonce           string                              `json:"nonce"`
	Ownership       string                              `json:"ownership"`
	InstallMethod   string                              `json:"installMethod"`
	PackageManager  string                              `json:"packageManager"`
	BinaryPath      string                              `json:"binaryPath"`
	Files           []trafficOverviewVnstatEvidenceFile `json:"files"`
	DataPaths       []string                            `json:"dataPaths"`
	ServiceUnits    []string                            `json:"serviceUnits"`
	CreatedAt       int64                               `json:"createdAt"`
}

type trafficOverviewVnstatEvidenceFile struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
}

type vnstatOwnershipValidation struct {
	Trusted bool
	Reason  string
}

type vnstatExternalInstallation struct {
	BinaryPath   string
	DaemonPath   string
	PIDs         []int
	ServiceUnits []string
}

type vnstatVerifiedService struct {
	Name    string
	Systemd bool
}

const (
	maxInt64AsUint64                   = ^uint64(0) >> 1
	trafficOverviewStateKey            = "trafficOverviewState"
	trafficOverviewLimitGiBKey         = "trafficOverviewLimitGiB"
	trafficOverviewEnabledKey          = "trafficOverviewEnabled"
	trafficOverviewResetDayKey         = "trafficOverviewResetDay"
	trafficOverviewExpiryDateKey       = "trafficOverviewExpiryDate"
	trafficOverviewSnapshotKey         = "trafficOverviewSnapshot"
	trafficOverviewCapStateKey         = "trafficOverviewCapState"
	trafficOverviewPauseStateKey       = "trafficOverviewPauseState"
	trafficOverviewVnstatManifestKey   = "trafficOverviewVnstatManifest"
	trafficOverviewMinDisplayGiB       = 0.01
	trafficOverviewFlushDelta          = int64(1024 * 1024)
	trafficOverviewFlushInterval       = 30 * time.Second
	trafficOverviewConfigCacheTTL      = 30 * time.Second
	trafficOverviewCapEvaluateInterval = 30 * time.Second
	vnstatStatusCacheTTL               = 15 * time.Second
	maxVnstatCommandOutputBytes        = 1024 * 1024
	trafficCapTagLoopback              = "loopback"
	trafficCapTagDropExcept            = "drop_except_allowed"
	trafficCapTagDropForward           = "drop_all_forward"
	vnstatPackageName                  = "vnstat"
	vnstatInstallMethodSystemPackage   = "system-package"
	vnstatInstallMethodGitHubRelease   = "github-release"
	vnstatOwnershipPanelInstalled      = "panel-installed"
	vnstatOwnershipStateManaged        = "managed"
	vnstatOwnershipStateUnmanaged      = "unmanaged"
	vnstatOwnershipStateQuarantined    = "quarantined"
	vnstatEvidenceSchema               = 2
	vnstatGitHubLatestReleaseAPI       = "https://api.github.com/repos/vergoh/vnstat/releases/latest"
	vnstatSystemdUnitPath              = "/etc/systemd/system/kwor-vnstat.service"
	vnstatPanelSystemdUnit             = "kwor-vnstat"
)

var vnstatStandardProgramPaths = []string{
	"/usr/bin/vnstat",
	"/usr/bin/vnstati",
	"/usr/sbin/vnstatd",
}

var vnstatStandardConfigAndUnitPaths = []string{
	"/etc/vnstat.conf",
	"/etc/default/vnstat",
	"/etc/conf.d/vnstat",
	"/etc/init.d/vnstat",
	"/etc/systemd/system/kwor-vnstat.service",
	"/lib/systemd/system/vnstat.service",
	"/usr/lib/systemd/system/vnstat.service",
}

var vnstatStandardManPagePaths = []string{
	"/usr/share/man/man1/vnstat.1",
	"/usr/share/man/man1/vnstati.1",
	"/usr/share/man/man5/vnstat.conf.5",
	"/usr/share/man/man8/vnstatd.8",
}

var trafficOverviewStateMu sync.Mutex
var trafficOverviewSnapshotMu sync.Mutex
var trafficOverviewCapMu sync.Mutex
var trafficOverviewOperationMu sync.Mutex
var vnstatInstallJobMu sync.Mutex
var vnstatInstallJobState *vnstatInstallJob
var vnstatInstallTaskManager = NewManagedDownloadTaskManager("vnstat install")
var vnstatRemovalTaskManager = NewManagedDownloadTaskManager("vnstat removal")
var vnstatLifecycleMu sync.Mutex
var trafficOverviewCapScheduleMu sync.Mutex
var trafficOverviewCapLastEvaluatedAt time.Time
var trafficOverviewShutdownEnabledFn = func() bool {
	return IsSystemPlatformLinux() && nftSupported()
}

var (
	vnstatEvidencePathFn = func() string {
		return filepath.Join(config.GetDataDir(), "vnstat", "ownership.json")
	}
	vnstatHostFingerprintFn                   = readVnstatHostFingerprint
	vnstatEvidenceFileHashFn                  = hashVnstatEvidenceFile
	vnstatEvidenceFilePresentFn               = vnstatEvidenceFilePresent
	vnstatCurrentEUIDFn                       = os.Geteuid
	vnstatRuntimeGOOS                         = func() string { return runtime.GOOS }
	vnstatStopForDeleteFn                     = stopVnstatDaemonForManifest
	vnstatStopForUninstallFn                  = stopVnstatDaemonForUninstall
	vnstatDiscoverRunningExternalFn           = discoverRunningExternalVnstatInstallations
	vnstatStopRunningExternalFn               = stopRunningExternalVnstatForPanelInstall
	vnstatRemoveTrackedDataFn                 = removeVnstatTrackedData
	vnstatCleanupTrafficCapFn                 = cleanupTrafficCapRules
	vnstatRandomReader              io.Reader = rand.Reader
	vnstatManagedInstallRunner                = func(ctx context.Context, s *TrafficOverviewService, source string, report vnstatInstallProgressReporter) (*TrafficOverview, error) {
		return s.installManagedVnstatWithContext(ctx, source, report)
	}
)

type TrafficOverviewService struct{}

type vnstatPackageManagerPlan struct {
	Name            string
	SystemFamily    string
	InstallPlan     [][]string
	BuildDepsPlan   [][]string
	RemovePlan      [][]string
	FileListCommand []string
}

type trafficOverviewRuntimeState struct {
	Interface        string `json:"interface"`
	ManualBaseUp     int64  `json:"manualBaseUp"`
	ManualBaseDown   int64  `json:"manualBaseDown"`
	PeriodBaseUp     int64  `json:"periodBaseUp"`
	PeriodBaseDown   int64  `json:"periodBaseDown"`
	PeriodTag        string `json:"periodTag"`
	PeriodResetDay   int    `json:"periodResetDay"`
	LastFullResetAt  int64  `json:"lastFullResetAt"`
	LastPeriodReset  int64  `json:"lastPeriodReset"`
	KernelOffsetUp   int64  `json:"kernelOffsetUp"`
	KernelOffsetDown int64  `json:"kernelOffsetDown"`
	LastKernelUp     int64  `json:"lastKernelUp"`
	LastKernelDown   int64  `json:"lastKernelDown"`
}

type trafficOverviewPauseState struct {
	Paused         bool                    `json:"paused"`
	Interface      string                  `json:"interface"`
	CurrentUp      int64                   `json:"currentUp"`
	CurrentDown    int64                   `json:"currentDown"`
	PeriodBaseUp   int64                   `json:"periodBaseUp"`
	PeriodBaseDown int64                   `json:"periodBaseDown"`
	ManualBaseUp   int64                   `json:"manualBaseUp"`
	ManualBaseDown int64                   `json:"manualBaseDown"`
	LastVnstatUp   int64                   `json:"lastVnstatUp"`
	LastVnstatDown int64                   `json:"lastVnstatDown"`
	Snapshot       trafficOverviewSnapshot `json:"snapshot"`
	PausedAt       int64                   `json:"pausedAt"`
}

type trafficOverviewSnapshot struct {
	Source     string `json:"source"`
	Interface  string `json:"interface"`
	Available  bool   `json:"available"`
	Up         int64  `json:"up"`
	Down       int64  `json:"down"`
	Total      int64  `json:"total"`
	AccumUp    int64  `json:"accumUp"`
	AccumDown  int64  `json:"accumDown"`
	AccumTotal int64  `json:"accumTotal"`
	UpdatedAt  int64  `json:"updatedAt"`
}

type trafficOverviewSnapshotState struct {
	Loaded       bool
	HasPersisted bool
	Persisted    trafficOverviewSnapshot
	HasPending   bool
	Pending      trafficOverviewSnapshot
	LastFlushAt  time.Time
}

type trafficOverviewRuntimeStateCacheState struct {
	Loaded       bool
	HasPersisted bool
	Persisted    trafficOverviewRuntimeState
	HasPending   bool
	Pending      trafficOverviewRuntimeState
	LastFlushAt  time.Time
}

type trafficOverviewCapState struct {
	Active       bool  `json:"active"`
	LimitReached bool  `json:"limitReached"`
	AllowedPorts []int `json:"allowedPorts"`
	UpdatedAt    int64 `json:"updatedAt"`
}

var trafficOverviewSnapshotCache trafficOverviewSnapshotState
var trafficOverviewRuntimeStateCache trafficOverviewRuntimeStateCacheState
var trafficOverviewRuntimeStateCacheMu sync.Mutex

type trafficOverviewConfigCacheState struct {
	loaded         bool
	limitGiB       float64
	resetDay       int
	expiryDate     string
	expiryBoundary time.Time
	enabled        bool
	updatedAt      time.Time
}

type vnstatStatusCacheState struct {
	loaded    bool
	status    VnstatPackageStatus
	updatedAt time.Time
}

var trafficOverviewConfigMu sync.Mutex
var trafficOverviewConfigCache trafficOverviewConfigCacheState
var trafficOverviewSettingsMu sync.Mutex
var vnstatStatusCacheMu sync.Mutex
var vnstatStatusCache vnstatStatusCacheState
var trafficOverviewFlight singleflight.Group
var trafficOverviewCapStartupChecked bool

func init() {
	database.RegisterDBResetHook(func() {
		trafficOverviewConfigMu.Lock()
		trafficOverviewConfigCache = trafficOverviewConfigCacheState{}
		trafficOverviewConfigMu.Unlock()
		vnstatStatusCacheMu.Lock()
		vnstatStatusCache = vnstatStatusCacheState{}
		vnstatStatusCacheMu.Unlock()
		trafficOverviewSnapshotMu.Lock()
		trafficOverviewSnapshotCache = trafficOverviewSnapshotState{}
		trafficOverviewSnapshotMu.Unlock()
		trafficOverviewRuntimeStateCacheMu.Lock()
		trafficOverviewRuntimeStateCache = trafficOverviewRuntimeStateCacheState{}
		trafficOverviewRuntimeStateCacheMu.Unlock()
		trafficOverviewCapMu.Lock()
		trafficOverviewCapStartupChecked = false
		trafficOverviewCapMu.Unlock()
		trafficOverviewCapScheduleMu.Lock()
		trafficOverviewCapLastEvaluatedAt = time.Time{}
		trafficOverviewCapScheduleMu.Unlock()
	})
}

type vnstatRuntimeConflictState struct {
	conflict *VnstatRuntimeConflict
}

var vnstatRuntimeConflictMu sync.RWMutex
var vnstatRuntimeConflictCache vnstatRuntimeConflictState

func (s *TrafficOverviewService) GetTrafficOverview() (*TrafficOverview, error) {
	result, err, _ := trafficOverviewFlight.Do("full", func() (any, error) {
		return s.getTrafficOverviewUncached()
	})
	if err != nil {
		return nil, err
	}
	overview, _ := result.(*TrafficOverview)
	if overview == nil {
		return nil, errors.New("traffic overview result is unavailable")
	}
	clone := *overview
	clone.Vnstat = cloneVnstatPackageStatus(overview.Vnstat)
	return &clone, nil
}

func (s *TrafficOverviewService) getTrafficOverviewUncached() (*TrafficOverview, error) {
	overview := &TrafficOverview{
		Source:    "vnstat",
		Enabled:   true,
		Status:    "stopped",
		UpdatedAt: time.Now().Unix(),
	}
	limitGiB, resetDay, expiryDate, expiryBoundary, enabled, configErr := s.getOverviewConfig()
	if configErr != nil {
		overview.Error = configErr.Error()
		return overview, nil
	}
	overview.LimitGiB = limitGiB
	overview.ResetDay = resetDay
	overview.ExpiryDate = expiryDate
	now := PanelNow()
	overview.Expired = isTrafficOverviewExpired(expiryBoundary, now)
	if nextResetAt, ok := nextClientMonthlyResetBoundary(resetDay, now); ok && !nextResetAt.IsZero() {
		overview.NextResetAt = nextResetAt.Unix()
	}
	overview.Enabled = enabled
	overview.Vnstat = s.GetVnstatStatus()
	if cached, ok := s.getSnapshotForDisplay(); ok {
		applySnapshotToOverview(overview, cached)
	}
	pauseState, hasPauseState := s.loadPauseState()
	if !enabled {
		if hasPauseState && pauseState.Paused {
			applySnapshotToOverview(overview, pauseState.Snapshot)
		}
		overview.Available = false
		overview.Status = "stopped"
		return overview, nil
	}
	if hasPauseState && pauseState.Paused {
		if err := s.resumeTrafficOverviewAccounting(); err != nil {
			overview.Error = err.Error()
			overview.Available = false
			overview.Status = "error"
			return overview, nil
		}
	}

	if !IsSystemPlatformLinux() {
		overview.Error = "vnstat is supported on linux only"
		overview.Available = false
		overview.Status = "unsupported"
		return overview, nil
	}
	if !overview.Vnstat.Installed {
		overview.Error = "vnstat is not installed"
		overview.Available = false
		overview.Status = "missing"
		return overview, nil
	}

	iface, err := detectDefaultTrafficInterface()
	if err != nil {
		overview.Error = err.Error()
		overview.Available = false
		overview.Status = "error"
		return overview, nil
	}
	overview.Interface = iface

	vnstatUp, vnstatDown, err := loadVnstatTrafficTotals(iface)
	if err != nil {
		overview.Error = err.Error()
		overview.Available = false
		overview.Status = "error"
		return overview, nil
	}
	trafficOverviewStateMu.Lock()
	defer trafficOverviewStateMu.Unlock()

	state, stateErr := s.loadRuntimeState()
	if stateErr != nil {
		overview.Error = stateErr.Error()
		overview.Available = false
		overview.Status = "error"
		return overview, nil
	}

	stateChanged := false
	currentUp, currentDown, source, derivedChanged, deriveErr := deriveCurrentAlltimeTotals(&state, iface, vnstatUp, vnstatDown)
	if deriveErr != nil {
		overview.Error = deriveErr.Error()
		overview.Available = false
		overview.Status = "error"
		return overview, nil
	}
	overview.Source = source
	stateChanged = stateChanged || derivedChanged

	normalizedChanged, normalizeErr := normalizeStateForTotals(&state, iface, currentUp, currentDown)
	if normalizeErr != nil {
		overview.Error = normalizeErr.Error()
		overview.Available = false
		overview.Status = "error"
		return overview, nil
	}
	stateChanged = stateChanged || normalizedChanged

	periodChanged, applyErr := applyPeriodResetIfNeeded(&state, resetDay, currentUp, currentDown, now)
	if applyErr != nil {
		overview.Error = applyErr.Error()
		overview.Available = false
		overview.Status = "error"
		return overview, nil
	}
	stateChanged = stateChanged || periodChanged

	overview.Up = nonNegativeDiff(currentUp, state.PeriodBaseUp)
	overview.Down = nonNegativeDiff(currentDown, state.PeriodBaseDown)
	overview.Total = overview.Up + overview.Down

	overview.AccumUp = nonNegativeDiff(currentUp, state.ManualBaseUp)
	overview.AccumDown = nonNegativeDiff(currentDown, state.ManualBaseDown)
	overview.AccumTotal = overview.AccumUp + overview.AccumDown
	overview.Available = true
	overview.Status = "running"

	if stateChanged {
		if err := s.stageRuntimeState(state, periodChanged); err != nil {
			logger.Warning("save traffic overview state failed:", err)
		}
	}
	if err := s.stageOverviewSnapshot(snapshotFromOverview(overview), false); err != nil {
		logger.Warning("save traffic overview snapshot failed:", err)
	}
	return overview, nil
}

func cloneVnstatPackageStatus(status VnstatPackageStatus) VnstatPackageStatus {
	status.DataPaths = append([]string(nil), status.DataPaths...)
	status.ExternalPaths = append([]string(nil), status.ExternalPaths...)
	status.ExternalUnits = append([]string(nil), status.ExternalUnits...)
	if status.RuntimeConflict != nil {
		clone := *status.RuntimeConflict
		clone.Paths = append([]string(nil), clone.Paths...)
		clone.PIDs = append([]int(nil), clone.PIDs...)
		clone.Units = append([]string(nil), clone.Units...)
		status.RuntimeConflict = &clone
	}
	return status
}

func (s *TrafficOverviewService) UpdateTrafficOverviewSettings(limitGiB float64, resetDay int, expiryDate string, expiryDateProvided bool) error {
	trafficOverviewOperationMu.Lock()
	defer trafficOverviewOperationMu.Unlock()
	limitGiB = normalizeLimitGiB(limitGiB)
	resetDay = normalizeResetDay(resetDay)
	normalizedExpiryDate, err := normalizeTrafficOverviewExpiryDate(expiryDate)
	if err != nil {
		return err
	}

	trafficOverviewSettingsMu.Lock()
	defer trafficOverviewSettingsMu.Unlock()

	changes := map[string]string{
		trafficOverviewLimitGiBKey: strconv.FormatFloat(limitGiB, 'f', 2, 64),
		trafficOverviewResetDayKey: strconv.Itoa(resetDay),
	}
	if expiryDateProvided {
		changes[trafficOverviewExpiryDateKey] = normalizedExpiryDate
	}
	if err := saveTrafficOverviewSettingsAtomically(changes); err != nil {
		return err
	}
	invalidateTrafficOverviewConfigCache()
	markTrafficOverviewCapReconcileNeeded()
	if err := s.ReconcileTrafficCap(); err != nil {
		logger.Warning("reconcile traffic cap after settings update failed:", err)
	}
	return nil
}

func saveTrafficOverviewSettingsAtomically(changes map[string]string) error {
	db := database.GetDB()
	if db == nil {
		return errors.New("database is not ready")
	}
	changed := false
	err := db.Transaction(func(tx *gorm.DB) error {
		current, err := loadCurrentPatchValues(tx, changes)
		if err != nil {
			return err
		}
		updates := changedSettingsValues(current, changes)
		if len(updates) == 0 {
			return nil
		}
		revision, err := ensureSettingsRevisionState(tx)
		if err != nil {
			return err
		}
		for _, key := range sortedSettingsKeys(updates) {
			if err := upsertSetting(tx, key, updates[key]); err != nil {
				return err
			}
		}
		updated := tx.Model(&model.SettingsState{}).
			Where("id = ? AND revision = ?", 1, revision).
			Update("revision", gorm.Expr("revision + ?", 1))
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected != 1 {
			return errors.New("traffic overview settings revision conflict")
		}
		changed = true
		return nil
	})
	if err == nil && changed {
		markLastUpdate(time.Now().Unix())
	}
	return err
}

func (s *TrafficOverviewService) SetTrafficOverviewEnabled(enabled bool) error {
	trafficOverviewOperationMu.Lock()
	defer trafficOverviewOperationMu.Unlock()
	currentEnabled, currentErr := s.isOverviewEnabled()
	if currentErr != nil {
		return currentErr
	}
	if currentEnabled == enabled {
		if enabled {
			return s.resumeTrafficOverviewAccounting()
		}
		return nil
	}

	if !enabled {
		if err := s.pauseTrafficOverviewAccounting(); err != nil {
			return err
		}
		if manifest, ok := loadTrustedVnstatManifest(); ok {
			if err := stopVnstatDaemonForRestart(manifest); err != nil {
				logger.Warning("stop managed vnstat daemon while disabling traffic overview failed: ", err)
			} else {
				invalidateVnstatStatusCache()
			}
		}
		invalidateVnstatStatusCache()
		if err := (&SettingService{}).setString(trafficOverviewEnabledKey, "false"); err != nil {
			return err
		}
		invalidateTrafficOverviewConfigCache()
		markTrafficOverviewCapReconcileNeeded()
		if err := cleanupTrafficCapRules(); err != nil {
			logger.Warning("cleanup traffic cap after disabling overview failed:", err)
		}
		return nil
	}
	if err := s.resumeTrafficOverviewAccounting(); err != nil {
		return err
	}
	if err := (&SettingService{}).setString(trafficOverviewEnabledKey, "true"); err != nil {
		return err
	}
	invalidateTrafficOverviewConfigCache()
	markTrafficOverviewCapReconcileNeeded()
	if !IsSystemPlatformLinux() {
		return nil
	}
	if _, ok := loadTrustedVnstatManifest(); !ok {
		return nil
	}
	if iface, err := detectDefaultTrafficInterface(); err == nil && iface != "" {
		if trackErr := ensureVnstatTracking(iface); trackErr != nil {
			return trackErr
		}
	}
	if daemonErr := ensureVnstatDaemonRunning(); daemonErr != nil {
		logger.Warning("ensure vnstat daemon after enabling overview failed:", daemonErr)
	}
	invalidateVnstatStatusCache()
	if err := s.ReconcileTrafficCap(); err != nil {
		logger.Warning("reconcile traffic cap after enabling overview failed:", err)
	}
	return nil
}

func (s *TrafficOverviewService) pauseTrafficOverviewAccounting() error {
	if !IsSystemPlatformLinux() {
		return s.pauseTrafficOverviewWithCachedSnapshot()
	}

	if _, ok := loadTrustedVnstatManifest(); !ok {
		return s.pauseTrafficOverviewWithCachedSnapshot()
	}

	iface, err := detectDefaultTrafficInterface()
	if err != nil {
		return err
	}
	if iface == "" {
		return errors.New("default interface is empty")
	}

	vnstatUp, vnstatDown, err := loadVnstatTrafficTotals(iface)
	if err != nil {
		return err
	}

	_, resetDay, _, _, _, cfgErr := s.getOverviewConfig()
	if cfgErr != nil {
		return cfgErr
	}

	now := PanelNow()
	trafficOverviewStateMu.Lock()
	state, stateErr := s.loadRuntimeState()
	if stateErr != nil {
		trafficOverviewStateMu.Unlock()
		return stateErr
	}

	currentUp, currentDown, source, derivedChanged, deriveErr := deriveCurrentAlltimeTotals(&state, iface, vnstatUp, vnstatDown)
	if deriveErr != nil {
		trafficOverviewStateMu.Unlock()
		return deriveErr
	}

	normalizedChanged, normalizeErr := normalizeStateForTotals(&state, iface, currentUp, currentDown)
	if normalizeErr != nil {
		trafficOverviewStateMu.Unlock()
		return normalizeErr
	}

	periodChanged, applyErr := applyPeriodResetIfNeeded(&state, resetDay, currentUp, currentDown, now)
	if applyErr != nil {
		trafficOverviewStateMu.Unlock()
		return applyErr
	}

	if derivedChanged || normalizedChanged || periodChanged {
		if err := s.saveRuntimeState(state); err != nil {
			trafficOverviewStateMu.Unlock()
			return err
		}
	}
	trafficOverviewStateMu.Unlock()

	snapshot := trafficOverviewSnapshot{
		Source:    source,
		Interface: iface,
		Available: true,
		Up:        nonNegativeDiff(currentUp, state.PeriodBaseUp),
		Down:      nonNegativeDiff(currentDown, state.PeriodBaseDown),
		AccumUp:   nonNegativeDiff(currentUp, state.ManualBaseUp),
		AccumDown: nonNegativeDiff(currentDown, state.ManualBaseDown),
		UpdatedAt: now.Unix(),
	}
	snapshot.Total = snapshot.Up + snapshot.Down
	snapshot.AccumTotal = snapshot.AccumUp + snapshot.AccumDown
	if err := s.stageOverviewSnapshot(snapshot, true); err != nil {
		return err
	}

	return s.savePauseState(trafficOverviewPauseState{
		Paused:         true,
		Interface:      iface,
		CurrentUp:      currentUp,
		CurrentDown:    currentDown,
		PeriodBaseUp:   state.PeriodBaseUp,
		PeriodBaseDown: state.PeriodBaseDown,
		ManualBaseUp:   state.ManualBaseUp,
		ManualBaseDown: state.ManualBaseDown,
		LastVnstatUp:   vnstatUp,
		LastVnstatDown: vnstatDown,
		Snapshot:       snapshot,
		PausedAt:       now.Unix(),
	})
}

func (s *TrafficOverviewService) pauseTrafficOverviewWithCachedSnapshot() error {
	now := time.Now().Unix()
	snapshot := trafficOverviewSnapshot{
		Source:    "vnstat",
		UpdatedAt: now,
	}
	if cached, ok := s.getSnapshotForDisplay(); ok {
		snapshot = normalizeOverviewSnapshot(cached)
	}
	if snapshot.UpdatedAt <= 0 {
		snapshot.UpdatedAt = now
	}
	if err := s.stageOverviewSnapshot(snapshot, true); err != nil {
		return err
	}
	return s.savePauseState(trafficOverviewPauseState{
		Paused:   true,
		Snapshot: snapshot,
		PausedAt: now,
	})
}

func (s *TrafficOverviewService) resumeTrafficOverviewAccounting() error {
	pauseState, ok := s.loadPauseState()
	if !ok || !pauseState.Paused {
		return nil
	}
	if !IsSystemPlatformLinux() {
		return s.clearPauseState()
	}
	if _, ok := loadTrustedVnstatManifest(); !ok {
		return s.clearPauseState()
	}

	iface, err := detectDefaultTrafficInterface()
	if err != nil {
		return err
	}
	if iface == "" {
		return errors.New("default interface is empty")
	}
	if err := ensureVnstatTracking(iface); err != nil {
		return err
	}
	if daemonErr := ensureVnstatDaemonRunning(); daemonErr != nil {
		logger.Warning("ensure vnstat daemon on resume failed:", daemonErr)
	}

	vnstatUp, vnstatDown, err := loadVnstatTrafficTotals(iface)
	if err != nil {
		return err
	}
	_, resetDay, _, _, _, cfgErr := s.getOverviewConfig()
	if cfgErr != nil {
		return cfgErr
	}
	now := PanelNow()

	trafficOverviewStateMu.Lock()
	state, stateErr := s.loadRuntimeState()
	if stateErr != nil {
		trafficOverviewStateMu.Unlock()
		return stateErr
	}

	currentUp, currentDown, _, _, deriveErr := deriveCurrentAlltimeTotals(&state, iface, vnstatUp, vnstatDown)
	if deriveErr != nil {
		trafficOverviewStateMu.Unlock()
		return deriveErr
	}

	state.Interface = iface
	state.ManualBaseUp = nonNegativeDiff(currentUp, pauseState.Snapshot.AccumUp)
	state.ManualBaseDown = nonNegativeDiff(currentDown, pauseState.Snapshot.AccumDown)
	state.PeriodBaseUp = nonNegativeDiff(currentUp, pauseState.Snapshot.Up)
	state.PeriodBaseDown = nonNegativeDiff(currentDown, pauseState.Snapshot.Down)
	state.PeriodTag = computePeriodTag(resetDay, now)
	state.PeriodResetDay = normalizeResetDay(resetDay)
	state.LastPeriodReset = maxInt64(state.LastPeriodReset, pauseState.PausedAt)
	if err = s.saveRuntimeState(state); err != nil {
		trafficOverviewStateMu.Unlock()
		return err
	}
	trafficOverviewStateMu.Unlock()

	if err = s.clearPauseState(); err != nil {
		return err
	}
	return s.stageOverviewSnapshot(pauseState.Snapshot, true)
}

func (s *TrafficOverviewService) GetVnstatStatus() VnstatPackageStatus {
	vnstatStatusCacheMu.Lock()
	if vnstatStatusCache.loaded && time.Since(vnstatStatusCache.updatedAt) < vnstatStatusCacheTTL {
		status := cloneVnstatPackageStatus(vnstatStatusCache.status)
		vnstatStatusCacheMu.Unlock()
		return status
	}
	vnstatStatusCacheMu.Unlock()

	status := s.getVnstatStatusUncached()
	vnstatStatusCacheMu.Lock()
	vnstatStatusCache = vnstatStatusCacheState{
		loaded:    true,
		status:    cloneVnstatPackageStatus(status),
		updatedAt: time.Now(),
	}
	vnstatStatusCacheMu.Unlock()
	return status
}

func (s *TrafficOverviewService) getVnstatStatusUncached() VnstatPackageStatus {
	status := VnstatPackageStatus{
		Supported: IsSystemPlatformLinux(),
		CanManage: IsSystemPlatformLinux(),
		DataPaths: defaultVnstatDataPaths(),
	}
	populateVnstatPlatformStatus(&status)
	if !IsSystemPlatformLinux() {
		status.Error = "vnstat is supported on linux only"
		status.CanManage = false
		return status
	}
	if canManage, manageHint := vnstatManagementSupport(); !canManage {
		status.CanManage = false
		status.ManageHint = manageHint
	}

	if manager := detectVnstatPackageManagerPlan(); manager != nil {
		status.PackageManager = manager.Name
		status.InstallMethod = vnstatInstallMethodSystemPackage
		if status.SystemFamily == "" {
			status.SystemFamily = manager.SystemFamily
		}
	}

	manifest, hasManifest := s.loadVnstatManifest()
	if hasManifest && isPanelInstalledVnstatManifest(manifest) {
		status.Ownership = manifest.Ownership
		ownershipValidation := validateVnstatOwnership(manifest)
		status.Managed = ownershipValidation.Trusted
		if status.Managed {
			status.OwnershipState = vnstatOwnershipStateManaged
		} else {
			status.OwnershipState = vnstatOwnershipStateQuarantined
			status.OwnershipHint = firstNonEmpty(ownershipValidation.Reason, "vnstat 受管清单无法核验")
		}
		status.PackageManager = firstNonEmpty(manifest.PackageManager, status.PackageManager)
		status.InstallMethod = firstNonEmpty(manifest.InstallMethod, normalizeVnstatInstallMethod("", manifest.PackageManager), status.InstallMethod)
		status.SystemFamily = firstNonEmpty(status.SystemFamily, manifest.SystemFamily)
		status.FileCount = len(normalizeAbsolutePathList(manifest.FilePaths))
		if len(manifest.DataPaths) > 0 {
			status.DataPaths = safeVnstatDataPaths(manifest.DataPaths)
		}
	} else {
		status.OwnershipState = vnstatOwnershipStateUnmanaged
		if hasManifest {
			status.Ownership = manifest.Ownership
			status.OwnershipHint = "历史 vnStat 记录不会自动接管，请在流量管理中重新安装"
		}
	}

	if status.Managed {
		binaryPath, binaryOK := managedVnstatBinaryPath(manifest)
		if binaryOK {
			status.Installed = true
			status.BinaryPath = binaryPath
			status.Running = isVnstatDaemonRunningForStatus(manifest)
			if status.FileCount == 0 {
				status.FileCount = len(safeVnstatFilePaths(collectVnstatPackageFilesByManager(status.PackageManager)))
			}
			if status.InstallMethod == vnstatInstallMethodSystemPackage {
				if version := detectInstalledVnstatPackageVersion(status.PackageManager); version != "" {
					status.Version = version
				}
			}
			if status.Version == "" {
				if version := detectVnstatVersionAt(binaryPath); version != "" {
					status.Version = version
				} else if hasManifest {
					status.Version = strings.TrimSpace(manifest.Version)
				}
			}
		} else if hasManifest {
			status.Version = strings.TrimSpace(manifest.Version)
			status.OwnershipHint = firstNonEmpty(status.OwnershipHint, "受管 vnstat 程序文件缺失，可删除以清理残留")
		}
	}

	if status.Managed {
		status.RuntimeConflict = getVnstatRuntimeConflict()
	}

	return status
}

func invalidateVnstatStatusCache() {
	vnstatStatusCacheMu.Lock()
	vnstatStatusCache = vnstatStatusCacheState{}
	vnstatStatusCacheMu.Unlock()
}

func populateVnstatPlatformStatus(status *VnstatPackageStatus) {
	if status == nil {
		return
	}
	platform, err := GetSystemPlatform()
	if err != nil || platform == nil {
		return
	}
	status.SystemFamily = strings.ToLower(strings.TrimSpace(platform.SystemFamily))
	status.SystemID = strings.ToLower(strings.TrimSpace(platform.SystemID))
	status.SystemVersion = strings.TrimSpace(platform.VersionID)
}

func getVnstatRuntimeConflict() *VnstatRuntimeConflict {
	vnstatRuntimeConflictMu.RLock()
	defer vnstatRuntimeConflictMu.RUnlock()
	if vnstatRuntimeConflictCache.conflict == nil {
		return nil
	}
	clone := *vnstatRuntimeConflictCache.conflict
	clone.Paths = append([]string(nil), clone.Paths...)
	clone.PIDs = append([]int(nil), clone.PIDs...)
	clone.Units = append([]string(nil), clone.Units...)
	return &clone
}

func clearVnstatRuntimeConflict() {
	vnstatRuntimeConflictMu.Lock()
	vnstatRuntimeConflictCache.conflict = nil
	vnstatRuntimeConflictMu.Unlock()
}

func recordVnstatRuntimeConflict(installations []vnstatExternalInstallation) {
	paths := externalPaths(installations)
	pids := make([]int, 0)
	for _, installation := range installations {
		pids = append(pids, installation.PIDs...)
	}
	pids = uniqueSortedIntSlice(pids)
	units := externalUnits(installations)
	if len(paths) == 0 && len(pids) == 0 && len(units) == 0 {
		clearVnstatRuntimeConflict()
		return
	}
	vnstatRuntimeConflictMu.Lock()
	vnstatRuntimeConflictCache.conflict = &VnstatRuntimeConflict{
		Message:    "面板受管 vnStat 无法启动，检测到非面板 vnStat 正在运行，可能发生冲突。",
		Paths:      paths,
		PIDs:       pids,
		Units:      units,
		DetectedAt: time.Now().Unix(),
	}
	vnstatRuntimeConflictMu.Unlock()
}

func normalizeVnstatOwnership(value string, managed bool) string {
	if managed && strings.EqualFold(strings.TrimSpace(value), vnstatOwnershipPanelInstalled) {
		return vnstatOwnershipPanelInstalled
	}
	return ""
}

func isTrustedVnstatManifest(manifest trafficOverviewVnstatManifest) bool {
	return validateVnstatOwnership(manifest).Trusted
}

func isPanelInstalledVnstatManifest(manifest trafficOverviewVnstatManifest) bool {
	return manifest.Managed && strings.EqualFold(strings.TrimSpace(manifest.Ownership), vnstatOwnershipPanelInstalled)
}

func isVnstatManifestBaselineSafe(manifest trafficOverviewVnstatManifest) bool {
	if !isPanelInstalledVnstatManifest(manifest) {
		return false
	}
	if !isSafeVnstatCommandPath(manifest.BinaryPath) {
		return false
	}
	requiredPrograms := map[string]bool{
		normalizeVnstatPath("/usr/bin/vnstat"):   false,
		normalizeVnstatPath("/usr/sbin/vnstatd"): false,
	}
	for _, path := range safeVnstatFilePaths(manifest.FilePaths) {
		if _, required := requiredPrograms[normalizeVnstatPath(path)]; required {
			requiredPrograms[normalizeVnstatPath(path)] = true
		}
	}
	for _, present := range requiredPrograms {
		if !present {
			return false
		}
	}
	if !sameVnstatDataPathInventory(manifest.DataPaths, defaultVnstatDataPaths()) {
		return false
	}
	method := normalizeVnstatInstallMethod(manifest.InstallMethod, manifest.PackageManager)
	return method == vnstatInstallMethodSystemPackage || method == vnstatInstallMethodGitHubRelease
}

func readVnstatHostFingerprint() (string, error) {
	for _, path := range []string{"/etc/machine-id", "/var/lib/dbus/machine-id"} {
		content, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		machineID := strings.TrimSpace(string(content))
		if machineID == "" {
			continue
		}
		sum := sha256.Sum256([]byte(machineID))
		return hex.EncodeToString(sum[:]), nil
	}
	return "", errors.New("unable to read linux machine-id for vnstat ownership evidence")
}

func newVnstatEvidenceNonce() (string, error) {
	bytes := make([]byte, 32)
	if _, err := io.ReadFull(vnstatRandomReader, bytes); err != nil {
		return "", fmt.Errorf("generate vnstat ownership nonce failed: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}

func hashVnstatEvidenceFile(path string) (string, error) {
	if !isSafeVnstatResidualPath(path) || isSafeVnstatDataPath(path) {
		return "", errors.New("refusing to hash an unsafe vnstat ownership file")
	}
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return "", errors.New("vnstat ownership file must be a regular non-symlink file")
	}
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	sum := sha256.New()
	if _, err := io.Copy(sum, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(sum.Sum(nil)), nil
}

func buildVnstatOwnershipEvidence(manifest trafficOverviewVnstatManifest) (trafficOverviewVnstatOwnershipEvidence, error) {
	if !isVnstatManifestBaselineSafe(manifest) {
		return trafficOverviewVnstatOwnershipEvidence{}, errors.New("vnstat manifest is unsafe for ownership evidence")
	}
	hostFingerprint, err := vnstatHostFingerprintFn()
	if err != nil {
		return trafficOverviewVnstatOwnershipEvidence{}, err
	}
	if strings.TrimSpace(hostFingerprint) == "" {
		return trafficOverviewVnstatOwnershipEvidence{}, errors.New("vnstat ownership host fingerprint is empty")
	}
	nonce := strings.TrimSpace(manifest.EvidenceNonce)
	if nonce == "" {
		nonce, err = newVnstatEvidenceNonce()
		if err != nil {
			return trafficOverviewVnstatOwnershipEvidence{}, err
		}
	}

	files := make([]trafficOverviewVnstatEvidenceFile, 0, len(manifest.FilePaths))
	for _, path := range safeVnstatFilePaths(manifest.FilePaths) {
		if isSafeVnstatDataPath(path) {
			continue
		}
		hash, hashErr := vnstatEvidenceFileHashFn(path)
		if hashErr != nil {
			return trafficOverviewVnstatOwnershipEvidence{}, fmt.Errorf("hash managed vnstat file %s failed: %w", path, hashErr)
		}
		files = append(files, trafficOverviewVnstatEvidenceFile{Path: normalizeVnstatPath(path), SHA256: hash})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	if len(files) == 0 {
		return trafficOverviewVnstatOwnershipEvidence{}, errors.New("vnstat ownership evidence requires a non-empty file inventory")
	}
	foundBinary := false
	for _, file := range files {
		if normalizeVnstatPath(file.Path) == normalizeVnstatPath(manifest.BinaryPath) {
			foundBinary = true
			break
		}
	}
	if !foundBinary {
		return trafficOverviewVnstatOwnershipEvidence{}, errors.New("vnstat ownership evidence does not contain the managed binary")
	}

	return trafficOverviewVnstatOwnershipEvidence{
		Schema:          vnstatEvidenceSchema,
		HostFingerprint: strings.TrimSpace(hostFingerprint),
		Nonce:           nonce,
		Ownership:       normalizeVnstatOwnership(manifest.Ownership, true),
		InstallMethod:   normalizeVnstatInstallMethod(manifest.InstallMethod, manifest.PackageManager),
		PackageManager:  strings.TrimSpace(strings.ToLower(manifest.PackageManager)),
		BinaryPath:      normalizeVnstatPath(manifest.BinaryPath),
		Files:           files,
		DataPaths:       safeVnstatDataPaths(manifest.DataPaths),
		ServiceUnits:    managedVnstatServiceUnits(manifest),
		CreatedAt:       time.Now().Unix(),
	}, nil
}

func readVnstatOwnershipEvidence() (trafficOverviewVnstatOwnershipEvidence, error) {
	path := strings.TrimSpace(vnstatEvidencePathFn())
	if path == "" {
		return trafficOverviewVnstatOwnershipEvidence{}, errors.New("vnstat ownership evidence path is empty")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return trafficOverviewVnstatOwnershipEvidence{}, err
	}
	evidence := trafficOverviewVnstatOwnershipEvidence{}
	if err := json.Unmarshal(raw, &evidence); err != nil {
		return trafficOverviewVnstatOwnershipEvidence{}, fmt.Errorf("decode vnstat ownership evidence failed: %w", err)
	}
	return evidence, nil
}

func writeVnstatOwnershipEvidence(evidence trafficOverviewVnstatOwnershipEvidence) error {
	path := strings.TrimSpace(vnstatEvidencePathFn())
	if path == "" || !filepath.IsAbs(path) {
		return errors.New("vnstat ownership evidence path is unsafe")
	}
	raw, err := json.Marshal(evidence)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".ownership-*.tmp")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	cleanup := func() {
		_ = temp.Close()
		_ = os.Remove(tempPath)
	}
	if err := temp.Chmod(0o600); err != nil {
		cleanup()
		return err
	}
	if _, err := temp.Write(raw); err != nil {
		cleanup()
		return err
	}
	if err := temp.Sync(); err != nil {
		cleanup()
		return err
	}
	if err := temp.Close(); err != nil {
		_ = os.Remove(tempPath)
		return err
	}
	if err := os.Rename(tempPath, path); err != nil {
		_ = os.Remove(tempPath)
		return err
	}
	return nil
}

func removeVnstatOwnershipEvidence() error {
	path := strings.TrimSpace(vnstatEvidencePathFn())
	if path == "" || !filepath.IsAbs(path) {
		return errors.New("vnstat ownership evidence path is unsafe")
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func validateVnstatOwnership(manifest trafficOverviewVnstatManifest) vnstatOwnershipValidation {
	if !isVnstatManifestBaselineSafe(manifest) {
		return vnstatOwnershipValidation{Reason: "vnstat 清单不符合受管安全条件"}
	}
	if manifest.EvidenceSchema != vnstatEvidenceSchema || strings.TrimSpace(manifest.EvidenceNonce) == "" {
		return vnstatOwnershipValidation{Reason: "vnstat 缺少本机受管凭据"}
	}
	evidence, err := readVnstatOwnershipEvidence()
	if err != nil {
		if os.IsNotExist(err) {
			return vnstatOwnershipValidation{Reason: "vnstat 本机受管凭据不存在"}
		}
		return vnstatOwnershipValidation{Reason: "无法读取 vnstat 本机受管凭据"}
	}
	if evidence.Schema != vnstatEvidenceSchema || strings.TrimSpace(evidence.Nonce) != strings.TrimSpace(manifest.EvidenceNonce) {
		return vnstatOwnershipValidation{Reason: "vnstat 本机受管凭据与数据库记录不匹配"}
	}
	hostFingerprint, hostErr := vnstatHostFingerprintFn()
	if hostErr != nil || strings.TrimSpace(hostFingerprint) == "" || strings.TrimSpace(evidence.HostFingerprint) != strings.TrimSpace(hostFingerprint) {
		return vnstatOwnershipValidation{Reason: "vnstat 受管凭据不属于当前主机"}
	}
	if evidence.Ownership != normalizeVnstatOwnership(manifest.Ownership, true) ||
		evidence.InstallMethod != normalizeVnstatInstallMethod(manifest.InstallMethod, manifest.PackageManager) ||
		evidence.PackageManager != strings.TrimSpace(strings.ToLower(manifest.PackageManager)) ||
		normalizeVnstatPath(evidence.BinaryPath) != normalizeVnstatPath(manifest.BinaryPath) {
		return vnstatOwnershipValidation{Reason: "vnstat 受管凭据内容不匹配"}
	}
	if len(evidence.Files) == 0 {
		return vnstatOwnershipValidation{Reason: "vnstat 受管凭据缺少文件清单"}
	}
	if !sameVnstatOwnershipInventory(manifest.FilePaths, evidence.Files) {
		return vnstatOwnershipValidation{Reason: "vnstat 数据库文件清单与本机受管凭据不匹配"}
	}
	if !sameVnstatDataPathInventory(manifest.DataPaths, evidence.DataPaths) {
		return vnstatOwnershipValidation{Reason: "vnstat 数据目录清单与本机受管凭据不匹配"}
	}
	if !sameManagedVnstatServiceUnitInventory(manifest.ServiceUnits, evidence.ServiceUnits) {
		return vnstatOwnershipValidation{Reason: "vnstat 服务清单与本机受管凭据不匹配"}
	}
	foundBinary := false
	for _, file := range evidence.Files {
		path := normalizeVnstatPath(file.Path)
		if !isSafeVnstatResidualPath(path) || isSafeVnstatDataPath(path) || strings.TrimSpace(file.SHA256) == "" {
			return vnstatOwnershipValidation{Reason: "vnstat 受管凭据包含不安全文件"}
		}
		if path == normalizeVnstatPath(manifest.BinaryPath) {
			foundBinary = true
		}
		present, presentErr := vnstatEvidenceFilePresentFn(path)
		if presentErr != nil {
			return vnstatOwnershipValidation{Reason: "无法核验 vnstat 受管文件"}
		}
		if !present {
			// Missing files are allowed only as managed residuals. The caller may
			// still use the evidence to clean the remaining exact paths.
			continue
		}
		hash, hashErr := vnstatEvidenceFileHashFn(path)
		if hashErr != nil || hash != file.SHA256 {
			return vnstatOwnershipValidation{Reason: "vnstat 受管文件已变化，已停止接管"}
		}
	}
	if !foundBinary {
		return vnstatOwnershipValidation{Reason: "vnstat 受管凭据缺少程序文件"}
	}
	return vnstatOwnershipValidation{Trusted: true}
}

func vnstatEvidenceFilePresent(path string) (bool, error) {
	_, err := os.Lstat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

// sameVnstatOwnershipInventory makes the database manifest a projection of
// the immutable evidence marker. Without this comparison, a modified database
// record could add another otherwise-allowed path to the deletion list.
func sameVnstatOwnershipInventory(manifestPaths []string, evidenceFiles []trafficOverviewVnstatEvidenceFile) bool {
	manifestSet := make(map[string]struct{})
	for _, path := range safeVnstatFilePaths(manifestPaths) {
		if isSafeVnstatDataPath(path) {
			continue
		}
		manifestSet[normalizeVnstatPath(path)] = struct{}{}
	}
	evidenceSet := make(map[string]struct{})
	for _, file := range evidenceFiles {
		path := normalizeVnstatPath(file.Path)
		if !isSafeVnstatResidualPath(path) || isSafeVnstatDataPath(path) {
			return false
		}
		evidenceSet[path] = struct{}{}
	}
	if len(manifestSet) != len(evidenceSet) {
		return false
	}
	for path := range manifestSet {
		if _, ok := evidenceSet[path]; !ok {
			return false
		}
	}
	return true
}

func sameVnstatDataPathInventory(manifestPaths []string, evidencePaths []string) bool {
	manifestSafe := safeVnstatDataPaths(manifestPaths)
	evidenceSafe := safeVnstatDataPaths(evidencePaths)
	if !stringSliceEqual(normalizeAbsolutePathList(manifestPaths), manifestSafe) ||
		!stringSliceEqual(normalizeAbsolutePathList(evidencePaths), evidenceSafe) {
		return false
	}
	return stringSliceEqual(manifestSafe, evidenceSafe)
}

func sameManagedVnstatServiceUnitInventory(manifestUnits []string, evidenceUnits []string) bool {
	manifestAll := normalizeVnstatServiceUnitList(manifestUnits)
	evidenceAll := normalizeVnstatServiceUnitList(evidenceUnits)
	manifestManaged := managedVnstatServiceUnits(trafficOverviewVnstatManifest{ServiceUnits: manifestUnits})
	evidenceManaged := managedVnstatServiceUnits(trafficOverviewVnstatManifest{ServiceUnits: evidenceUnits})
	if !stringSliceEqual(manifestAll, manifestManaged) || !stringSliceEqual(evidenceAll, evidenceManaged) {
		return false
	}
	return stringSliceEqual(manifestManaged, evidenceManaged)
}

func stringSliceEqual(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func (s *TrafficOverviewService) GetVnstatVersionOptions() (*VnstatVersionListResult, error) {
	title := "系统软件源"
	description := "使用当前系统的软件包管理器安装或重装 vnstat"
	manager := detectVnstatPackageManagerPlan()
	canManage, manageHint := vnstatManagementSupport()
	systemAvailable := canManage && manager != nil
	systemReason := ""
	if manager != nil {
		description = fmt.Sprintf("通过 %s 安装或重装 vnstat", manager.Name)
	}
	if !canManage {
		description = manageHint
		systemReason = manageHint
	} else if manager == nil {
		systemReason = "未识别到当前系统可用的软件包管理器"
		description = systemReason
	}
	githubAvailable := canManage
	githubReason := ""
	if !githubAvailable {
		githubReason = manageHint
	}
	return &VnstatVersionListResult{
		Versions: []VnstatVersionOption{
			{
				Value:       vnstatInstallMethodSystemPackage,
				Title:       title,
				Description: description,
				Available:   systemAvailable,
				Reason:      systemReason,
			},
			{
				Value:       vnstatInstallMethodGitHubRelease,
				Title:       "GitHub 官方源码包",
				Description: "从 GitHub 官方 release 源码包编译安装或重装 vnstat",
				Available:   githubAvailable,
				Reason:      githubReason,
			},
		},
	}, nil
}

func (s *TrafficOverviewService) GetVnstatUpdateInfo(source string) (*VnstatVersionCheckResult, error) {
	status := s.GetVnstatStatus()
	result := &VnstatVersionCheckResult{
		Supported:      status.Supported,
		CanManage:      status.CanManage,
		Installed:      status.Installed,
		Managed:        status.Managed,
		CurrentVersion: strings.TrimSpace(status.Version),
	}
	if !status.Supported {
		result.Message = firstNonEmpty(status.Error, "vnstat is supported on linux only")
		return result, nil
	}
	if !status.CanManage && strings.TrimSpace(status.ManageHint) != "" {
		result.Message = strings.TrimSpace(status.ManageHint)
		return result, nil
	}

	latestVersion, detectedSource, err := detectLatestVnstatVersion(status, source)
	result.Source = detectedSource
	if err != nil {
		result.Message = strings.TrimSpace(err.Error())
		return result, nil
	}
	result.LatestVersion = strings.TrimSpace(latestVersion)
	if result.CurrentVersion == "" {
		if result.Installed {
			result.Message = fmt.Sprintf("已安装 vnstat，但未能识别当前版本；所选来源最新版本：%s", firstNonEmpty(result.LatestVersion, "-"))
		} else {
			result.Message = fmt.Sprintf("vnstat 尚未安装；所选来源最新版本：%s", firstNonEmpty(result.LatestVersion, "-"))
		}
		return result, nil
	}
	if result.LatestVersion == "" {
		result.Message = "未能识别远端版本信息"
		return result, nil
	}

	switch compareSemverLikeTags(result.CurrentVersion, result.LatestVersion) {
	case -1:
		result.HasUpdate = true
		result.Message = fmt.Sprintf("发现新版本：%s -> %s", result.CurrentVersion, result.LatestVersion)
	case 0:
		result.Message = fmt.Sprintf("当前已是最新版本：%s", result.CurrentVersion)
	default:
		result.Message = fmt.Sprintf("当前版本 %s 高于可检测版本 %s", result.CurrentVersion, result.LatestVersion)
	}
	return result, nil
}

// StartManagedVnstatInstall starts (or returns) the current asynchronous
// install task. A repeated request for the same source is deliberately
// idempotent so a dropped browser response cannot create competing installs.
func (s *TrafficOverviewService) StartManagedVnstatInstall(source string) (VnstatInstallJobStatus, error) {
	selectedSource := normalizeRequestedVnstatSource(source)
	if selectedSource == "" {
		return VnstatInstallJobStatus{}, errors.New("请先选择来源")
	}
	vnstatLifecycleMu.Lock()
	defer vnstatLifecycleMu.Unlock()
	if vnstatRemovalTaskManager.IsActive() {
		return VnstatInstallJobStatus{}, errors.New("vnstat 删除任务正在运行，请等待完成后再安装")
	}
	handle, taskStatus, created, err := vnstatInstallTaskManager.Start("vnstat-install", "source|"+selectedSource)
	if err != nil {
		return VnstatInstallJobStatus{}, err
	}
	if !created {
		return s.GetManagedVnstatInstallJob(taskStatus.ID), nil
	}
	job := &vnstatInstallJob{status: VnstatInstallJobStatus{
		ID:         handle.ID(),
		Source:     selectedSource,
		State:      managedDownloadTaskQueued,
		Phase:      "正在准备 vnStat 安装任务",
		CanCancel:  true,
		StartedAt:  taskStatus.StartedAt,
		UpdatedAt:  taskStatus.UpdatedAt,
		DeadlineAt: taskStatus.DeadlineAt,
	}, task: handle}
	vnstatInstallJobMu.Lock()
	vnstatInstallJobState = job
	initialStatus := cloneVnstatInstallJobStatus(job.status)
	vnstatInstallJobMu.Unlock()
	go s.runManagedVnstatInstallJob(handle.ID(), selectedSource, handle)
	return initialStatus, nil
}

// GetManagedVnstatInstallJob returns the latest task. Supplying an ID avoids
// accidentally attaching a browser tab to a newer task after a page reload.
func (s *TrafficOverviewService) GetManagedVnstatInstallJob(jobID string) VnstatInstallJobStatus {
	vnstatInstallJobMu.Lock()
	defer vnstatInstallJobMu.Unlock()
	if vnstatInstallJobState != nil && vnstatInstallJobState.status.FinishedAt > 0 && time.Since(time.Unix(vnstatInstallJobState.status.FinishedAt, 0)) > managedDownloadTaskTTL {
		vnstatInstallJobState = nil
	}
	if vnstatInstallJobState != nil && (strings.TrimSpace(jobID) == "" || vnstatInstallJobState.status.ID == strings.TrimSpace(jobID)) {
		return applyManagedVnstatTaskStatus(vnstatInstallJobState.status, vnstatInstallTaskManager.Get(vnstatInstallJobState.status.ID))
	}
	taskStatus := vnstatInstallTaskManager.Get(jobID)
	if taskStatus.State == managedDownloadTaskIdle {
		return VnstatInstallJobStatus{State: managedDownloadTaskIdle}
	}
	return applyManagedVnstatTaskStatus(VnstatInstallJobStatus{ID: taskStatus.ID}, taskStatus)
}

func (s *TrafficOverviewService) runManagedVnstatInstallJob(jobID string, source string, task *ManagedDownloadTaskHandle) {
	defer s.recoverManagedVnstatInstallPanic(jobID, task)
	if task == nil || !task.MarkRunning("preparing") {
		if task != nil {
			task.FinishCancelled("cancelled")
		}
		return
	}
	ctx := task.Context()
	overview, err := vnstatManagedInstallRunner(ctx, s, source, func(phase string) bool {
		return s.updateManagedVnstatInstallJob(jobID, phase)
	})

	vnstatInstallJobMu.Lock()
	defer vnstatInstallJobMu.Unlock()
	if vnstatInstallJobState == nil || vnstatInstallJobState.status.ID != jobID {
		return
	}
	status := &vnstatInstallJobState.status
	managedStatus := task.Snapshot()
	vnstatInstallJobState.status = applyManagedVnstatTaskStatus(*status, managedStatus)
	status = &vnstatInstallJobState.status
	status.FinishedAt = time.Now().Unix()
	if err != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			task.FinishCancelled("cancelled")
			status.State = task.Snapshot().State
			status.Phase = "vnStat 安装已停止"
			status.Error = task.Snapshot().Error
		} else {
			task.FinishError("failed", err)
			status.State = task.Snapshot().State
			status.Phase = "vnStat 安装失败"
			status.Error = strings.TrimSpace(err.Error())
		}
		vnstatInstallJobState.status = applyManagedVnstatTaskStatus(*status, task.Snapshot())
		return
	}
	if overview == nil {
		err = errors.New("安装完成后未能读取 vnStat 状态")
		task.FinishError("failed", err)
		status.State = task.Snapshot().State
		status.Phase = "vnStat 安装失败"
		status.Error = err.Error()
		vnstatInstallJobState.status = applyManagedVnstatTaskStatus(*status, task.Snapshot())
		return
	}
	task.FinishSuccess("completed")
	status.State = task.Snapshot().State
	status.Phase = "vnStat 安装完成"
	status.Error = ""
	vnstatInstallJobState.status = applyManagedVnstatTaskStatus(*status, task.Snapshot())
}

func (s *TrafficOverviewService) recoverManagedVnstatInstallPanic(jobID string, task *ManagedDownloadTaskHandle) {
	if recovered := recover(); recovered == nil || task == nil {
		return
	} else {
		panicErr := fmt.Errorf("vnStat install task panicked: %v", recovered)
		task.FinishError("failed", panicErr)
		managedStatus := task.Snapshot()

		vnstatInstallJobMu.Lock()
		defer vnstatInstallJobMu.Unlock()
		if vnstatInstallJobState == nil || vnstatInstallJobState.status.ID != jobID {
			return
		}
		status := applyManagedVnstatTaskStatus(vnstatInstallJobState.status, managedStatus)
		status.Phase = "vnStat 安装失败"
		status.Error = panicErr.Error()
		vnstatInstallJobState.status = status
	}
}

func (s *TrafficOverviewService) updateManagedVnstatInstallJob(jobID string, phase string) bool {
	phase = strings.TrimSpace(phase)
	if phase == "" {
		return true
	}
	vnstatInstallJobMu.Lock()
	defer vnstatInstallJobMu.Unlock()
	if vnstatInstallJobState == nil || vnstatInstallJobState.status.ID != jobID || isManagedDownloadTaskTerminal(vnstatInstallJobState.status.State) {
		return false
	}
	vnstatInstallJobState.status.Phase = phase
	vnstatInstallJobState.status.UpdatedAt = time.Now().Unix()
	if vnstatInstallJobState.task != nil {
		advanced := false
		if vnstatInstallPhaseIsIrreversible(phase) {
			advanced = vnstatInstallJobState.task.BeginApplying(phase)
		} else {
			advanced = vnstatInstallJobState.task.SetPhase(phase, true)
		}
		vnstatInstallJobState.status = applyManagedVnstatTaskStatus(vnstatInstallJobState.status, vnstatInstallJobState.task.Snapshot())
		return advanced
	}
	return true
}

func isManagedVnstatInstallRunning() bool {
	vnstatInstallJobMu.Lock()
	defer vnstatInstallJobMu.Unlock()
	return vnstatInstallJobState != nil && !isManagedDownloadTaskTerminal(vnstatInstallJobState.status.State) && vnstatInstallJobState.status.State != managedDownloadTaskIdle
}

func (s *TrafficOverviewService) StopManagedVnstatInstall(jobID string) (VnstatInstallJobStatus, error) {
	taskStatus, err := vnstatInstallTaskManager.Stop(jobID)
	vnstatInstallJobMu.Lock()
	defer vnstatInstallJobMu.Unlock()
	if vnstatInstallJobState != nil && vnstatInstallJobState.status.ID == strings.TrimSpace(jobID) {
		vnstatInstallJobState.status = applyManagedVnstatTaskStatus(vnstatInstallJobState.status, taskStatus)
		return cloneVnstatInstallJobStatus(vnstatInstallJobState.status), err
	}
	return applyManagedVnstatTaskStatus(VnstatInstallJobStatus{ID: taskStatus.ID}, taskStatus), err
}

func cloneVnstatInstallJobStatus(status VnstatInstallJobStatus) VnstatInstallJobStatus {
	return status
}

func applyManagedVnstatTaskStatus(status VnstatInstallJobStatus, task ManagedDownloadTaskStatus) VnstatInstallJobStatus {
	if task.State == managedDownloadTaskIdle {
		return cloneVnstatInstallJobStatus(status)
	}
	if status.ID == "" {
		status.ID = task.ID
	}
	status.State = task.State
	status.CanCancel = task.CanCancel
	status.StopRequested = task.StopRequested
	status.DeadlineExceeded = task.DeadlineExceeded
	if status.Phase == "" && task.Phase != "" {
		status.Phase = task.Phase
	}
	if task.Error != "" {
		status.Error = task.Error
	}
	if task.StartedAt > 0 {
		status.StartedAt = task.StartedAt
	}
	if task.UpdatedAt > 0 {
		status.UpdatedAt = task.UpdatedAt
	}
	if task.DeadlineAt > 0 {
		status.DeadlineAt = task.DeadlineAt
	}
	if task.FinishedAt > 0 {
		status.FinishedAt = task.FinishedAt
	}
	return status
}

func vnstatInstallPhaseIsIrreversible(phase string) bool {
	phase = strings.TrimSpace(phase)
	for _, marker := range []string{
		"正在停止并核验",
		"正在释放面板默认",
		"正在通过系统软件源安装",
		"正在安装 GitHub 源码构建依赖",
		"正在安装 vnStat 到面板默认路径",
		"正在创建面板专属 vnStat 服务",
		"正在写入本机受管凭据",
		"正在启动 vnStat 服务",
	} {
		if strings.Contains(phase, marker) {
			return true
		}
	}
	return false
}

func reportVnstatInstallProgress(report vnstatInstallProgressReporter, phase string) bool {
	if report == nil {
		return true
	}
	return report(phase)
}

func reportVnstatInstallProgressContext(ctx context.Context, report vnstatInstallProgressReporter, phase string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if !reportVnstatInstallProgress(report, phase) {
		if err := ctx.Err(); err != nil {
			return err
		}
		return context.Canceled
	}
	return ctx.Err()
}

func (s *TrafficOverviewService) InstallManagedVnstat(source string) (*TrafficOverview, error) {
	return s.installManagedVnstat(source, nil)
}

func (s *TrafficOverviewService) installManagedVnstat(source string, report vnstatInstallProgressReporter) (*TrafficOverview, error) {
	return s.installManagedVnstatWithContext(context.Background(), source, report)
}

func (s *TrafficOverviewService) installManagedVnstatWithContext(ctx context.Context, source string, report vnstatInstallProgressReporter) (*TrafficOverview, error) {
	trafficOverviewOperationMu.Lock()
	defer trafficOverviewOperationMu.Unlock()
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := reportVnstatInstallProgressContext(ctx, report, "正在验证 vnStat 安装环境"); err != nil {
		return nil, err
	}
	if !IsSystemPlatformLinux() {
		return nil, errors.New("vnstat is supported on linux only")
	}
	if canManage, manageHint := vnstatManagementSupport(); !canManage {
		return nil, errors.New(manageHint)
	}
	selectedSource := normalizeRequestedVnstatSource(source)
	if selectedSource == "" {
		return nil, errors.New("请先选择来源")
	}

	manager := detectVnstatPackageManagerPlan()
	if selectedSource == vnstatInstallMethodSystemPackage && manager == nil {
		return nil, errors.New("未识别到当前系统的软件包管理器，无法通过系统软件源安装 vnstat")
	}
	manifest, hasManifest := s.loadVnstatManifest()
	managed := hasManifest && isTrustedVnstatManifest(manifest)
	if managed {
		currentMethod := detectInstalledVnstatInstallMethod(manifest, true, manager, manifest.BinaryPath)
		if currentMethod != "" && currentMethod != selectedSource {
			return nil, fmt.Errorf(
				"当前 vnstat 安装方式为%s；如需切换到%s，请先删除 vnstat 后再重新安装",
				describeVnstatInstallMethod(currentMethod, packageManagerName(manager)),
				describeVnstatInstallMethod(selectedSource, packageManagerName(manager)),
			)
		}
		if currentMethod == "" {
			return nil, errors.New("检测到现有 vnstat 安装来源不明确；请先删除 vnstat 后再按所选来源重新安装")
		}
	}
	if vnstatCurrentEUIDFn() != 0 {
		if selectedSource == vnstatInstallMethodSystemPackage && manager != nil && len(manager.InstallPlan) > 0 {
			return nil, fmt.Errorf("vnstat install requires root. install it manually: %s", strings.Join(manager.InstallPlan[len(manager.InstallPlan)-1], " "))
		}
		return nil, errors.New("vnstat install requires root")
	}
	// External vnStat inspection happens only for this explicit install action.
	// It considers running daemons only; non-panel files outside the fixed
	// panel paths are never removed.
	external := vnstatDiscoverRunningExternalFn(manifest, managed)
	if len(external) > 0 {
		if err := reportVnstatInstallProgressContext(ctx, report, "正在停止并核验已发现的 vnStat"); err != nil {
			return nil, err
		}
		if err := vnstatStopRunningExternalFn(external); err != nil {
			return nil, err
		}
	}
	if !managed && shouldPrepareVnstatPanelPathForInstall(manager, selectedSource, external) {
		if err := reportVnstatInstallProgressContext(ctx, report, "正在释放面板默认 vnStat 路径"); err != nil {
			return nil, err
		}
		if err := prepareUnmanagedStandardVnstatForPanelInstall(manager); err != nil {
			return nil, err
		}
	}
	pendingOwnership, err := BeginVnstatHostOwnership(vnstatOwnershipCandidatePaths(), nil, map[string]string{
		"installMethod":  selectedSource,
		"packageManager": packageManagerName(manager),
		"packageName":    vnstatPackageName,
		"ownership":      vnstatOwnershipPanelInstalled,
	})
	if err != nil {
		return nil, fmt.Errorf("record pending vnstat ownership: %w", err)
	}
	pendingOwnershipActivated := false
	defer func() {
		if !pendingOwnershipActivated && pendingOwnership.ID != "" {
			_ = RemoveHostResource(pendingOwnership.ID)
		}
	}()

	manifest, err = s.installManagedVnstatFromSourceWithContext(ctx, manager, selectedSource, report)
	if err != nil {
		return nil, err
	}
	if err := reportVnstatInstallProgressContext(ctx, report, "正在写入本机受管凭据"); err != nil {
		return nil, err
	}
	if err := s.saveVnstatManifest(manifest); err != nil {
		return nil, err
	}
	ownedPaths := append([]string{manifest.BinaryPath, strings.TrimSpace(vnstatEvidencePathFn())}, manifest.FilePaths...)
	ownedPaths = append(ownedPaths, manifest.DataPaths...)
	if err := RegisterVnstatHostOwnership(ownedPaths, manifest.ServiceUnits, map[string]string{
		"installMethod":  manifest.InstallMethod,
		"packageManager": manifest.PackageManager,
		"packageName":    manifest.PackageName,
		"ownership":      manifest.Ownership,
	}); err != nil {
		return nil, fmt.Errorf("record managed vnstat ownership: %w", err)
	}
	pendingOwnershipActivated = true
	if err := (&SettingService{}).setString(trafficOverviewEnabledKey, "true"); err != nil {
		return nil, err
	}
	invalidateTrafficOverviewConfigCache()
	markTrafficOverviewCapReconcileNeeded()
	if err := s.clearPauseState(); err != nil {
		return nil, err
	}

	if iface, detectErr := detectDefaultTrafficInterface(); detectErr == nil && iface != "" {
		if err := reportVnstatInstallProgressContext(ctx, report, "正在配置 vnStat 流量网卡"); err != nil {
			return nil, err
		}
		if trackErr := ensureVnstatTracking(iface); trackErr != nil {
			logger.Warning("ensure vnstat tracking after install failed:", trackErr)
		}
	}
	if err := reportVnstatInstallProgressContext(ctx, report, "正在启动 vnStat 服务"); err != nil {
		return nil, err
	}
	if daemonErr := restartVnstatDaemonForManifest(manifest); daemonErr != nil {
		logger.Warning("restart vnstat daemon after install failed:", daemonErr)
		recordVnstatRuntimeConflict(vnstatDiscoverRunningExternalFn(manifest, true))
	} else {
		clearVnstatRuntimeConflict()
	}

	if err := reportVnstatInstallProgressContext(ctx, report, "正在刷新 vnStat 状态"); err != nil {
		return nil, err
	}
	return s.GetTrafficOverview()
}

func vnstatOwnershipCandidatePaths() []string {
	paths := make([]string, 0, len(vnstatStandardProgramPaths)+len(vnstatStandardConfigAndUnitPaths)+len(vnstatStandardManPagePaths)+len(defaultVnstatDataPaths())+1)
	paths = append(paths, vnstatStandardProgramPaths...)
	paths = append(paths, vnstatStandardConfigAndUnitPaths...)
	paths = append(paths, vnstatStandardManPagePaths...)
	paths = append(paths, defaultVnstatDataPaths()...)
	paths = append(paths, strings.TrimSpace(vnstatEvidencePathFn()))
	return normalizeAbsolutePathList(paths)
}

func shouldPrepareVnstatPanelPathForInstall(manager *vnstatPackageManagerPlan, source string, installations []vnstatExternalInstallation) bool {
	for _, installation := range installations {
		if isPanelDefaultVnstatRuntimePath(installation.DaemonPath) {
			return true
		}
	}
	// GitHub source installation writes the panel's fixed /usr paths. Release
	// package ownership first so it never overwrites files still owned by the
	// distribution package manager.
	return normalizeRequestedVnstatSource(source) == vnstatInstallMethodGitHubRelease &&
		manager != nil && packageOwnsStandardVnstatBinary(collectVnstatPackageFilesByManager(manager.Name))
}

type managedVnstatRemovalPlan struct {
	manifest      trafficOverviewVnstatManifest
	installMethod string
	installed     bool
	manager       *vnstatPackageManagerPlan
}

// prepareManagedVnstatRemoval accepts only a panel-installed manifest whose
// database inventory, local credential, host fingerprint, and current file
// hashes all agree. A historical or externally-created vnStat never reaches
// the destructive phase.
func (s *TrafficOverviewService) prepareManagedVnstatRemoval() (managedVnstatRemovalPlan, error) {
	if vnstatRuntimeGOOS() != "linux" {
		return managedVnstatRemovalPlan{}, errors.New("vnstat is supported on linux only")
	}
	if canManage, manageHint := vnstatManagementSupport(); !canManage {
		return managedVnstatRemovalPlan{}, errors.New(manageHint)
	}
	return s.prepareManagedVnstatRemovalWithManager(managedVnstatPackageManager)
}

// prepareManagedVnstatRemovalForUninstall deliberately avoids the database
// platform snapshot. The panel database is allowed to be damaged or removed
// while uninstalling, so only the actual runtime and the signed local
// ownership evidence may authorize host cleanup.
func (s *TrafficOverviewService) prepareManagedVnstatRemovalForUninstall() (managedVnstatRemovalPlan, error) {
	if vnstatRuntimeGOOS() != "linux" {
		return managedVnstatRemovalPlan{}, errors.New("vnstat is supported on linux only")
	}
	return s.prepareManagedVnstatRemovalWithManager(managedVnstatPackageManagerForUninstall)
}

func (s *TrafficOverviewService) prepareManagedVnstatRemovalWithManager(resolveManager func(trafficOverviewVnstatManifest) (*vnstatPackageManagerPlan, bool)) (managedVnstatRemovalPlan, error) {
	manifest, hasManifest := s.loadVnstatManifest()
	if !hasManifest || !isTrustedVnstatManifest(manifest) {
		return managedVnstatRemovalPlan{}, errors.New("未检测到可安全删除的面板受管 vnstat")
	}
	if vnstatCurrentEUIDFn() != 0 {
		return managedVnstatRemovalPlan{}, errors.New("removing vnstat requires root")
	}

	plan := managedVnstatRemovalPlan{
		manifest:      manifest,
		installMethod: normalizeVnstatInstallMethod(manifest.InstallMethod, manifest.PackageManager),
	}
	_, plan.installed = managedVnstatBinaryPath(manifest)
	if plan.installMethod == vnstatInstallMethodSystemPackage {
		manager, ok := resolveManager(manifest)
		if ok {
			plan.manager = manager
		} else if plan.installed {
			return managedVnstatRemovalPlan{}, errors.New("vnstat 系统软件包归属无法验证，已拒绝卸载以保护系统文件")
		}
	}
	return plan, nil
}

func removeManagedVnstatArtifacts(plan managedVnstatRemovalPlan) error {
	if plan.manager != nil {
		for _, command := range plan.manager.RemovePlan {
			if err := runInstallCommand(command); err != nil {
				return err
			}
		}
	}
	return vnstatRemoveTrackedDataFn(plan.manifest)
}

// disableTrafficOverviewForVnstatRemoval deliberately skips the normal pause
// snapshot. The daemon has already been confirmed stopped and all associated
// overview state is removed immediately afterwards.
func (s *TrafficOverviewService) disableTrafficOverviewForVnstatRemoval() error {
	if err := (&SettingService{}).setString(trafficOverviewEnabledKey, "false"); err != nil {
		return err
	}
	invalidateTrafficOverviewConfigCache()
	markTrafficOverviewCapReconcileNeeded()
	return nil
}

func (s *TrafficOverviewService) finishManagedVnstatRemoval() error {
	if systemctlPath, err := exec.LookPath("systemctl"); err == nil {
		_ = runCommandWithTimeout(systemCommandTimeout, systemctlPath, "daemon-reload")
		_ = runCommandWithTimeout(systemCommandTimeout, systemctlPath, "reset-failed")
	}
	if err := s.clearVnstatManagedState(); err != nil {
		return err
	}
	if err := RemoveHostResource("vnstat-managed-runtime"); err != nil {
		return fmt.Errorf("clear vnstat host ownership: %w", err)
	}
	clearVnstatRuntimeConflict()
	if err := vnstatCleanupTrafficCapFn(); err != nil {
		logger.Warning("cleanup traffic cap after vnstat removal failed:", err)
	}
	return nil
}

func (s *TrafficOverviewService) RemoveManagedVnstat() (*TrafficOverview, error) {
	vnstatLifecycleMu.Lock()
	defer vnstatLifecycleMu.Unlock()
	trafficOverviewOperationMu.Lock()
	defer trafficOverviewOperationMu.Unlock()
	if isManagedVnstatInstallRunning() {
		return nil, errors.New("vnstat 安装任务正在运行，请等待完成后再删除")
	}
	plan, err := s.prepareManagedVnstatRemoval()
	if err != nil {
		return nil, err
	}
	if err := vnstatStopForDeleteFn(plan.manifest); err != nil {
		return nil, fmt.Errorf("停止并确认 vnstat 已退出失败，已取消删除：%w", err)
	}
	if err := s.disableTrafficOverviewForVnstatRemoval(); err != nil {
		return nil, err
	}
	if err := removeManagedVnstatArtifacts(plan); err != nil {
		return nil, err
	}
	if err := s.finishManagedVnstatRemoval(); err != nil {
		return nil, err
	}
	return s.GetTrafficOverview()
}

// StartManagedVnstatRemoval moves package-manager work out of the HTTP
// lifetime. The task is deliberately non-cancellable after acceptance so the
// daemon stop, package removal, and ownership cleanup cannot be left halfway.
func (s *TrafficOverviewService) StartManagedVnstatRemoval() (VnstatRemovalJobStatus, error) {
	vnstatLifecycleMu.Lock()
	defer vnstatLifecycleMu.Unlock()
	if isManagedVnstatInstallRunning() || vnstatInstallTaskManager.IsActive() {
		return VnstatRemovalJobStatus{}, errors.New("vnstat 安装任务正在运行，请等待完成后再删除")
	}
	handle, status, created, err := vnstatRemovalTaskManager.Start("vnstat-remove", "remove")
	if err != nil {
		return VnstatRemovalJobStatus{}, err
	}
	if !created {
		return managedVnstatRemovalJobStatus(status), nil
	}
	go s.runManagedVnstatRemovalJob(handle)
	return managedVnstatRemovalJobStatus(status), nil
}

func (s *TrafficOverviewService) GetManagedVnstatRemovalJob(jobID string) VnstatRemovalJobStatus {
	return managedVnstatRemovalJobStatus(vnstatRemovalTaskManager.Get(jobID))
}

func managedVnstatRemovalJobStatus(status ManagedDownloadTaskStatus) VnstatRemovalJobStatus {
	return VnstatRemovalJobStatus{
		ID:         status.ID,
		State:      status.State,
		Phase:      status.Phase,
		Error:      status.Error,
		StartedAt:  status.StartedAt,
		UpdatedAt:  status.UpdatedAt,
		FinishedAt: status.FinishedAt,
	}
}

func (s *TrafficOverviewService) runManagedVnstatRemovalJob(task *ManagedDownloadTaskHandle) {
	defer finishManagedDownloadTaskPanic(task, "failed", "vnStat removal")
	if task == nil || !task.MarkRunning("正在核验 vnStat 删除条件") {
		return
	}
	if !task.BeginApplying("正在停止并删除 vnStat") {
		task.FinishCancelled("cancelled")
		return
	}
	if _, err := s.RemoveManagedVnstat(); err != nil {
		task.FinishError("vnStat 删除失败", err)
		return
	}
	task.FinishSuccess("vnStat 删除完成")
}

func (s *TrafficOverviewService) RemoveManagedVnstatForUninstall() error {
	vnstatLifecycleMu.Lock()
	defer vnstatLifecycleMu.Unlock()
	if isManagedVnstatInstallRunning() {
		return errors.New("vnstat 安装任务正在运行，请等待完成后再卸载面板")
	}
	if vnstatRuntimeGOOS() != "linux" {
		cleanupTrafficCapAfterSkippedVnstatUninstall()
		return nil
	}
	if runningInsideContainer() {
		cleanupTrafficCapAfterSkippedVnstatUninstall()
		return nil
	}

	// A missing, historical, or tampered manifest is intentionally not an
	// error for the overall panel uninstall. It is not authorization to delete
	// host vnStat files, so only panel firewall state is cleaned up.
	manifest, hasManifest := s.loadVnstatManifest()
	if !hasManifest || !isTrustedVnstatManifest(manifest) {
		cleanupTrafficCapAfterSkippedVnstatUninstall()
		return nil
	}
	plan, err := s.prepareManagedVnstatRemovalForUninstall()
	if err != nil {
		return err
	}
	if err := vnstatStopForUninstallFn(plan.manifest); err != nil {
		return fmt.Errorf("停止并确认受管 vnstat 已退出失败，已取消面板 vnstat 清理：%w", err)
	}
	if err := s.disableTrafficOverviewForVnstatRemoval(); err != nil {
		return err
	}
	if err := removeManagedVnstatArtifacts(plan); err != nil {
		return err
	}
	return s.finishManagedVnstatRemoval()
}

func cleanupTrafficCapAfterSkippedVnstatUninstall() {
	if err := vnstatCleanupTrafficCapFn(); err != nil {
		logger.Warning("cleanup traffic cap while skipping unmanaged vnstat uninstall failed:", err)
	}
}

func prepareUnmanagedStandardVnstatForPanelInstall(manager *vnstatPackageManagerPlan) error {
	if manager == nil {
		return nil
	}
	if packageFiles := collectVnstatPackageFilesByManager(manager.Name); packageOwnsStandardVnstatBinary(packageFiles) {
		for _, command := range manager.RemovePlan {
			if err := runInstallCommand(command); err != nil {
				return err
			}
		}
	}
	// Do not delete any untracked file or data directory here. The selected
	// install source owns its fixed output paths and will overwrite only what
	// it installs; external installations in other directories are preserved.
	return nil
}

func managedVnstatPackageManager(manifest trafficOverviewVnstatManifest) (*vnstatPackageManagerPlan, bool) {
	manager := detectVnstatPackageManagerPlan()
	if manager == nil {
		return nil, false
	}
	if configured := strings.ToLower(strings.TrimSpace(manifest.PackageManager)); configured != "" && configured != manager.Name {
		return nil, false
	}
	if !packageOwnsStandardVnstatBinary(collectVnstatPackageFilesByManager(manager.Name)) {
		return nil, false
	}
	return manager, true
}

// managedVnstatPackageManagerForUninstall verifies the manager named by the
// trusted ownership manifest instead of rediscovering the platform from the
// panel database. It still requires both the executable and ownership of the
// standard vnStat binary before a package removal is allowed.
func managedVnstatPackageManagerForUninstall(manifest trafficOverviewVnstatManifest) (*vnstatPackageManagerPlan, bool) {
	manager := managerByName(manifest.PackageManager)
	if manager == nil {
		return nil, false
	}
	if _, err := exec.LookPath(manager.Name); err != nil {
		return nil, false
	}
	if !packageOwnsStandardVnstatBinary(collectVnstatPackageFilesByManager(manager.Name)) {
		return nil, false
	}
	return manager, true
}

func packageOwnsStandardVnstatBinary(paths []string) bool {
	for _, path := range paths {
		if isSafeVnstatCommandPath(path) {
			return true
		}
	}
	return false
}

func (s *TrafficOverviewService) installManagedVnstatFromSource(manager *vnstatPackageManagerPlan, source string, report vnstatInstallProgressReporter) (trafficOverviewVnstatManifest, error) {
	return s.installManagedVnstatFromSourceWithContext(context.Background(), manager, source, report)
}

func (s *TrafficOverviewService) installManagedVnstatFromSourceWithContext(ctx context.Context, manager *vnstatPackageManagerPlan, source string, report vnstatInstallProgressReporter) (trafficOverviewVnstatManifest, error) {
	switch normalizeRequestedVnstatSource(source) {
	case vnstatInstallMethodSystemPackage:
		return s.installVnstatViaSystemPackageWithContext(ctx, manager, report)
	case vnstatInstallMethodGitHubRelease:
		return s.installVnstatViaGitHubReleaseWithContext(ctx, manager, report)
	default:
		return trafficOverviewVnstatManifest{}, errors.New("请先选择来源")
	}
}

func (s *TrafficOverviewService) installVnstatViaSystemPackage(manager *vnstatPackageManagerPlan, report vnstatInstallProgressReporter) (trafficOverviewVnstatManifest, error) {
	return s.installVnstatViaSystemPackageWithContext(context.Background(), manager, report)
}

func (s *TrafficOverviewService) installVnstatViaSystemPackageWithContext(ctx context.Context, manager *vnstatPackageManagerPlan, report vnstatInstallProgressReporter) (trafficOverviewVnstatManifest, error) {
	if manager == nil {
		return trafficOverviewVnstatManifest{}, errors.New("no supported linux package manager was found")
	}
	if err := reportVnstatInstallProgressContext(ctx, report, "正在通过系统软件源安装 vnStat"); err != nil {
		return trafficOverviewVnstatManifest{}, err
	}
	for _, command := range manager.InstallPlan {
		if err := runInstallCommandContext(ctx, command); err != nil {
			return trafficOverviewVnstatManifest{}, err
		}
	}

	binaryPath, binaryFound := findStandardVnstatBinaryPath()
	if !binaryFound {
		return trafficOverviewVnstatManifest{}, errors.New("vnstat package install completed but the standard binary /usr/bin/vnstat is missing")
	}

	filePaths := appendDetectedVnstatManagedPaths(collectVnstatPackageFilesByManager(manager.Name))
	version := firstNonEmpty(detectInstalledVnstatPackageVersion(manager.Name), detectVnstatVersionAt(binaryPath))
	return trafficOverviewVnstatManifest{
		Managed:        true,
		Ownership:      vnstatOwnershipPanelInstalled,
		SystemFamily:   firstNonEmpty(detectLinuxSystemFamily(), manager.SystemFamily),
		PackageManager: manager.Name,
		InstallMethod:  vnstatInstallMethodSystemPackage,
		PackageName:    vnstatPackageName,
		Version:        version,
		BinaryPath:     binaryPath,
		FilePaths:      filePaths,
		DataPaths:      defaultVnstatDataPaths(),
		ServiceUnits:   []string{"vnstat", "vnstatd"},
		InstalledAt:    time.Now().Unix(),
	}, nil
}

func (s *TrafficOverviewService) installVnstatViaGitHubRelease(manager *vnstatPackageManagerPlan, report vnstatInstallProgressReporter) (trafficOverviewVnstatManifest, error) {
	return s.installVnstatViaGitHubReleaseWithContext(context.Background(), manager, report)
}

func (s *TrafficOverviewService) installVnstatViaGitHubReleaseWithContext(ctx context.Context, manager *vnstatPackageManagerPlan, report vnstatInstallProgressReporter) (trafficOverviewVnstatManifest, error) {
	var buildDepsErr error
	if manager != nil {
		if err := reportVnstatInstallProgressContext(ctx, report, "正在安装 GitHub 源码构建依赖"); err != nil {
			return trafficOverviewVnstatManifest{}, err
		}
		for _, command := range manager.BuildDepsPlan {
			if err := runInstallCommandContext(ctx, command); err != nil {
				buildDepsErr = fmt.Errorf("install build dependencies failed: %w", err)
				logger.Warning(buildDepsErr)
				break
			}
		}
	}

	if err := reportVnstatInstallProgressContext(ctx, report, "正在查询 GitHub 官方 vnStat 版本"); err != nil {
		return trafficOverviewVnstatManifest{}, err
	}
	release, err := fetchLatestVnstatReleaseContext(ctx)
	if err != nil {
		return trafficOverviewVnstatManifest{}, err
	}
	asset, err := selectVnstatReleaseSourceAsset(release)
	if err != nil {
		return trafficOverviewVnstatManifest{}, err
	}

	workDir, err := os.MkdirTemp("", "kwor-vnstat-source-")
	if err != nil {
		return trafficOverviewVnstatManifest{}, fmt.Errorf("create vnstat work directory failed: %w", err)
	}
	defer os.RemoveAll(workDir)

	archivePath := filepath.Join(workDir, asset.Name)
	if err := reportVnstatInstallProgressContext(ctx, report, "正在下载 GitHub 官方 vnStat 源码"); err != nil {
		return trafficOverviewVnstatManifest{}, err
	}
	if err := downloadFileWithUserAgentContext(ctx, asset.BrowserDownloadURL, archivePath, 10*time.Minute); err != nil {
		return trafficOverviewVnstatManifest{}, fmt.Errorf("download GitHub release asset failed: %w", err)
	}

	if err := reportVnstatInstallProgressContext(ctx, report, "正在解压 vnStat 源码"); err != nil {
		return trafficOverviewVnstatManifest{}, err
	}
	sourceDir, err := extractVnstatSourceArchiveContext(ctx, archivePath, workDir)
	if err != nil {
		return trafficOverviewVnstatManifest{}, fmt.Errorf("extract vnstat source archive failed: %w", err)
	}
	if err := reportVnstatInstallProgressContext(ctx, report, "正在配置 vnStat 源码"); err != nil {
		return trafficOverviewVnstatManifest{}, err
	}
	if _, err := runCommandInDirContext(ctx, sourceDir, 5*time.Minute, "./configure", "--prefix=/usr", "--sysconfdir=/etc"); err != nil {
		if buildDepsErr != nil {
			return trafficOverviewVnstatManifest{}, fmt.Errorf("%w; %v", err, buildDepsErr)
		}
		return trafficOverviewVnstatManifest{}, fmt.Errorf("configure vnstat source failed: %w", err)
	}
	if err := reportVnstatInstallProgressContext(ctx, report, "正在编译 vnStat 源码"); err != nil {
		return trafficOverviewVnstatManifest{}, err
	}
	if _, err := runCommandInDirContext(ctx, sourceDir, 10*time.Minute, "make"); err != nil {
		if buildDepsErr != nil {
			return trafficOverviewVnstatManifest{}, fmt.Errorf("%w; %v", err, buildDepsErr)
		}
		return trafficOverviewVnstatManifest{}, fmt.Errorf("build vnstat source failed: %w", err)
	}

	stageDir := filepath.Join(workDir, "stage")
	if err := reportVnstatInstallProgressContext(ctx, report, "正在核验源码阶段安装文件"); err != nil {
		return trafficOverviewVnstatManifest{}, err
	}
	if _, err := runCommandInDirContext(ctx, sourceDir, 5*time.Minute, "make", "install", "DESTDIR="+stageDir); err != nil {
		return trafficOverviewVnstatManifest{}, fmt.Errorf("stage vnstat source install for ownership inventory failed: %w", err)
	}
	managedPaths := collectManagedSourceVnstatPaths(stageDir)
	if err := validateManagedSourceVnstatPaths(managedPaths); err != nil {
		return trafficOverviewVnstatManifest{}, err
	}

	if err := reportVnstatInstallProgressContext(ctx, report, "正在安装 vnStat 到面板默认路径"); err != nil {
		return trafficOverviewVnstatManifest{}, err
	}
	if _, err := runCommandInDirContext(ctx, sourceDir, 5*time.Minute, "make", "install"); err != nil {
		return trafficOverviewVnstatManifest{}, fmt.Errorf("install vnstat source failed: %w", err)
	}

	serviceUnits := []string{}
	if err := reportVnstatInstallProgressContext(ctx, report, "正在创建面板专属 vnStat 服务"); err != nil {
		return trafficOverviewVnstatManifest{}, err
	}
	if unitPath, unitErr := installVnstatSystemdUnitContext(ctx, sourceDir); unitErr == nil && unitPath != "" {
		managedPaths = append(managedPaths, unitPath)
		serviceUnits = append(serviceUnits, vnstatPanelSystemdUnit)
	} else if unitErr != nil {
		return trafficOverviewVnstatManifest{}, fmt.Errorf("install panel vnstat systemd unit failed: %w", unitErr)
	}

	binaryPath, binaryFound := findStandardVnstatBinaryPath()
	if !binaryFound {
		return trafficOverviewVnstatManifest{}, errors.New("vnstat GitHub install completed but the standard binary /usr/bin/vnstat is missing")
	}

	managedPaths = appendDetectedVnstatManagedPaths(managedPaths)
	if err := validateManagedSourceVnstatPaths(managedPaths); err != nil {
		return trafficOverviewVnstatManifest{}, err
	}
	return trafficOverviewVnstatManifest{
		Managed:        true,
		Ownership:      vnstatOwnershipPanelInstalled,
		SystemFamily:   firstNonEmpty(detectLinuxSystemFamily(), systemFamilyFromManager(manager)),
		PackageManager: packageManagerName(manager),
		InstallMethod:  vnstatInstallMethodGitHubRelease,
		PackageName:    vnstatPackageName,
		Version:        firstNonEmpty(detectVnstatVersionAt(binaryPath), strings.TrimPrefix(strings.TrimSpace(release.TagName), "v")),
		BinaryPath:     binaryPath,
		FilePaths:      managedPaths,
		DataPaths:      defaultVnstatDataPaths(),
		ServiceUnits:   serviceUnits,
		InstalledAt:    time.Now().Unix(),
	}, nil
}

func buildVnstatInstallUnavailableError(systemErr error, githubErr error) error {
	switch {
	case systemErr != nil && githubErr != nil:
		return fmt.Errorf("无法下载 vnstat，功能无法使用。系统软件源安装失败：%v；GitHub 官方版本安装失败：%v", systemErr, githubErr)
	case systemErr != nil:
		return fmt.Errorf("无法下载 vnstat，功能无法使用。系统软件源安装失败：%v", systemErr)
	case githubErr != nil:
		return fmt.Errorf("无法下载 vnstat，功能无法使用。GitHub 官方版本安装失败：%v", githubErr)
	default:
		return errors.New("无法下载 vnstat，功能无法使用。")
	}
}

func packageManagerName(manager *vnstatPackageManagerPlan) string {
	if manager == nil {
		return ""
	}
	return strings.TrimSpace(manager.Name)
}

func systemFamilyFromManager(manager *vnstatPackageManagerPlan) string {
	if manager == nil {
		return ""
	}
	return strings.TrimSpace(manager.SystemFamily)
}

func appendDetectedVnstatManagedPaths(paths []string) []string {
	managed := make([]string, 0, len(paths))
	for _, candidate := range safeVnstatFilePaths(paths) {
		info, err := os.Lstat(candidate)
		if err != nil || info.IsDir() {
			continue
		}
		managed = append(managed, candidate)
	}
	return normalizeAbsolutePathList(managed)
}

func normalizeVnstatInstallMethod(method string, packageManager string) string {
	method = strings.TrimSpace(strings.ToLower(method))
	switch method {
	case "system", vnstatInstallMethodSystemPackage:
		return vnstatInstallMethodSystemPackage
	case "github", vnstatInstallMethodGitHubRelease:
		return vnstatInstallMethodGitHubRelease
	case "":
	default:
		return method
	}
	if managerByName(packageManager) != nil {
		return vnstatInstallMethodSystemPackage
	}
	return ""
}

func normalizeRequestedVnstatSource(source string) string {
	switch strings.TrimSpace(strings.ToLower(source)) {
	case "system", vnstatInstallMethodSystemPackage:
		return vnstatInstallMethodSystemPackage
	case "github", vnstatInstallMethodGitHubRelease:
		return vnstatInstallMethodGitHubRelease
	default:
		return ""
	}
}

func describeVnstatInstallMethod(method string, packageManager string) string {
	switch normalizeVnstatInstallMethod(method, packageManager) {
	case vnstatInstallMethodSystemPackage:
		if strings.TrimSpace(packageManager) != "" {
			return fmt.Sprintf("系统软件源（%s）", strings.TrimSpace(packageManager))
		}
		return "系统软件源"
	case vnstatInstallMethodGitHubRelease:
		return "GitHub 官方源码包"
	default:
		return "未知来源"
	}
}

func detectInstalledVnstatInstallMethod(manifest trafficOverviewVnstatManifest, hasManifest bool, manager *vnstatPackageManagerPlan, binaryPath string) string {
	if hasManifest {
		if method := normalizeVnstatInstallMethod(manifest.InstallMethod, manifest.PackageManager); method != "" {
			return method
		}
	}
	if manager != nil && len(collectVnstatPackageFilesByManager(manager.Name)) > 0 {
		return vnstatInstallMethodSystemPackage
	}
	if strings.TrimSpace(binaryPath) != "" && hasManifest && len(manifest.FilePaths) > 0 && strings.EqualFold(manifest.InstallMethod, vnstatInstallMethodGitHubRelease) {
		return vnstatInstallMethodGitHubRelease
	}
	return ""
}

func fetchLatestVnstatRelease() (GitHubRelease, error) {
	return fetchLatestVnstatReleaseContext(context.Background())
}

func fetchLatestVnstatReleaseContext(ctx context.Context) (GitHubRelease, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "GET", vnstatGitHubLatestReleaseAPI, nil)
	if err != nil {
		return GitHubRelease{}, err
	}
	req.Header.Set("User-Agent", "kwor")

	resp, err := client.Do(req)
	if err != nil {
		return GitHubRelease{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return GitHubRelease{}, fmt.Errorf("GitHub latest release API returned %d", resp.StatusCode)
	}

	release := GitHubRelease{}
	if err := unmarshalBoundedHTTPResponseJSON(resp.Body, coreGitHubResponseMaxBytes, &release); err != nil {
		return GitHubRelease{}, err
	}
	return release, nil
}

func selectVnstatReleaseSourceAsset(release GitHubRelease) (GitHubAsset, error) {
	for _, asset := range release.Assets {
		name := strings.ToLower(strings.TrimSpace(asset.Name))
		if strings.HasPrefix(name, "vnstat-") && strings.HasSuffix(name, ".tar.gz") && !strings.HasSuffix(name, ".tar.gz.asc") {
			return asset, nil
		}
	}
	return GitHubAsset{}, fmt.Errorf("vnstat source archive not found in GitHub release %s", firstNonEmpty(release.TagName, release.Name))
}

func downloadFileWithUserAgent(url string, destPath string, timeout time.Duration) error {
	return downloadFileWithUserAgentContext(context.Background(), url, destPath, timeout)
}

func downloadFileWithUserAgentContext(ctx context.Context, url string, destPath string, timeout time.Duration) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	client := &http.Client{Timeout: timeout}
	req, err := http.NewRequestWithContext(ctx, "GET", strings.TrimSpace(url), nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "kwor")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download failed, HTTP %d", resp.StatusCode)
	}

	tmpPath := destPath + ".tmp"
	out, err := os.Create(tmpPath)
	if err != nil {
		return err
	}
	if _, err := copyManagedDownloadTaskContext(ctx, out, resp.Body); err != nil {
		_ = out.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := out.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if err := ctx.Err(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if err := os.Rename(tmpPath, destPath); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}

func extractVnstatSourceArchive(archivePath string, workDir string) (string, error) {
	return extractVnstatSourceArchiveContext(context.Background(), archivePath, workDir)
}

func extractVnstatSourceArchiveContext(ctx context.Context, archivePath string, workDir string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	extractRoot := filepath.Join(workDir, "source")
	if err := os.MkdirAll(extractRoot, 0o755); err != nil {
		return "", err
	}

	file, err := os.Open(archivePath)
	if err != nil {
		return "", err
	}
	defer file.Close()

	gzr, err := gzip.NewReader(file)
	if err != nil {
		return "", err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	cleanRoot := filepath.Clean(extractRoot)
	rootName := ""

	for {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}

		relPath := filepath.Clean(strings.TrimPrefix(strings.TrimSpace(header.Name), "./"))
		if relPath == "." || relPath == "" {
			continue
		}
		topLevel := strings.Split(filepath.ToSlash(relPath), "/")[0]
		if rootName == "" {
			rootName = topLevel
		}

		targetPath := filepath.Join(cleanRoot, relPath)
		if targetPath != cleanRoot && !strings.HasPrefix(targetPath, cleanRoot+string(os.PathSeparator)) {
			return "", fmt.Errorf("vnstat source archive contains an invalid path: %s", header.Name)
		}

		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(targetPath, 0o755); err != nil {
				return "", err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
				return "", err
			}
			out, err := os.Create(targetPath)
			if err != nil {
				return "", err
			}
			if _, err := copyManagedDownloadTaskContext(ctx, out, tr); err != nil {
				_ = out.Close()
				_ = os.Remove(targetPath)
				return "", err
			}
			if err := out.Close(); err != nil {
				_ = os.Remove(targetPath)
				return "", err
			}
			if err := ctx.Err(); err != nil {
				_ = os.Remove(targetPath)
				return "", err
			}
			if err := os.Chmod(targetPath, header.FileInfo().Mode().Perm()); err != nil {
				return "", err
			}
		}
	}

	if rootName == "" {
		return "", errors.New("vnstat source archive is empty")
	}
	sourceDir := filepath.Join(cleanRoot, filepath.FromSlash(rootName))
	if _, err := os.Stat(filepath.Join(sourceDir, "configure")); err != nil {
		return "", fmt.Errorf("vnstat source directory does not contain configure script: %w", err)
	}
	return sourceDir, nil
}

func runCommandInDir(dir string, timeout time.Duration, name string, args ...string) (string, error) {
	return runCommandInDirContext(context.Background(), dir, timeout, name, args...)
}

func runCommandInDirContext(parent context.Context, dir string, timeout time.Duration, name string, args ...string) (string, error) {
	output, err, timedOut := runManagedCommandOutputContext(parent, dir, timeout, name, args...)
	text := strings.TrimSpace(string(output))
	if timedOut {
		if text == "" {
			return "", fmt.Errorf("%s timed out", name)
		}
		return text, fmt.Errorf("%s timed out: %s", name, text)
	}
	if errors.Is(parent.Err(), context.Canceled) {
		return text, fmt.Errorf("%s canceled: %w", name, parent.Err())
	}
	if err != nil {
		if text == "" {
			return "", fmt.Errorf("%s failed: %w", name, err)
		}
		return text, fmt.Errorf("%s failed: %w: %s", name, err, text)
	}
	return text, nil
}

func runManagedCommandOutputContext(parent context.Context, dir string, timeout time.Duration, name string, args ...string) ([]byte, error, bool) {
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	PrepareKworManagedCommandContext(parent, cmd)
	if err := cmd.Start(); err != nil {
		return output.Bytes(), err, false
	}
	if err := TrackKworManagedCommandContext(parent, cmd); err != nil {
		stopKworManagedCommand(cmd)
		_ = cmd.Wait()
		return output.Bytes(), fmt.Errorf("record command process: %w", err), false
	}
	stopWatchingCommand := watchKworManagedCommandContext(ctx, cmd)
	defer stopWatchingCommand()
	err := cmd.Wait()
	return output.Bytes(), err, errors.Is(ctx.Err(), context.DeadlineExceeded)
}

func collectManagedSourceVnstatPaths(stageRoot string) []string {
	seen := make(map[string]struct{})
	paths := make([]string, 0, 8)
	_ = filepath.WalkDir(stageRoot, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry == nil || entry.IsDir() {
			return nil
		}
		relPath, relErr := filepath.Rel(stageRoot, path)
		if relErr != nil {
			return nil
		}
		absPath := filepath.ToSlash(filepath.Clean("/" + filepath.ToSlash(relPath)))
		if isManagedSourceVnstatPath(absPath) {
			if _, ok := seen[absPath]; !ok {
				seen[absPath] = struct{}{}
				paths = append(paths, absPath)
			}
		}
		return nil
	})
	sort.Strings(paths)
	return paths
}

func isManagedSourceVnstatPath(path string) bool {
	return !isSafeVnstatDataPath(path) && isSafeVnstatResidualPath(path)
}

func validateManagedSourceVnstatPaths(paths []string) error {
	available := make(map[string]struct{}, len(paths))
	for _, path := range safeVnstatFilePaths(paths) {
		available[normalizeVnstatPath(path)] = struct{}{}
	}
	for _, required := range []string{"/usr/bin/vnstat", "/usr/sbin/vnstatd", "/etc/vnstat.conf"} {
		if _, ok := available[normalizeVnstatPath(required)]; !ok {
			return fmt.Errorf("vnstat source ownership inventory is incomplete: missing %s", required)
		}
	}
	return nil
}

func installVnstatSystemdUnit(sourceDir string) (string, error) {
	return installVnstatSystemdUnitContext(context.Background(), sourceDir)
}

func installVnstatSystemdUnitContext(ctx context.Context, sourceDir string) (string, error) {
	if vnstatRuntimeGOOS() != "linux" {
		return "", nil
	}
	systemctlPath, err := exec.LookPath("systemctl")
	if err != nil {
		return "", nil
	}

	_ = sourceDir // Keep the source directory in the signature for install-call context.
	ownership, err := BeginSystemdHostOwnership("vnstat-systemd", vnstatPanelSystemdUnit, []string{vnstatSystemdUnitPath}, nil)
	if err != nil {
		return "", fmt.Errorf("record pending vnstat systemd ownership: %w", err)
	}
	unitContent := []byte("# kwor-owner:v1 resource=vnstat-systemd\n[Unit]\nDescription=kwor managed vnStat network traffic monitor\nDocumentation=man:vnstatd(8) man:vnstat(1) man:vnstat.conf(5)\nAfter=network.target\n\n[Service]\nExecStart=/usr/sbin/vnstatd --nodaemon\nExecReload=/bin/kill -HUP $MAINPID\nRestart=on-failure\n\n[Install]\nWantedBy=multi-user.target\n")
	if err := os.WriteFile(vnstatSystemdUnitPath, unitContent, 0o644); err != nil {
		return "", err
	}
	output, err, timedOut := runManagedCommandOutputContext(ctx, "", systemCommandTimeout, systemctlPath, "daemon-reload")
	if timedOut {
		return "", fmt.Errorf("systemctl daemon-reload timed out")
	}
	if err != nil {
		return "", fmt.Errorf("systemctl daemon-reload failed: %v: %s", err, strings.TrimSpace(string(output)))
	}
	if err := VerifyAndActivateHostResource(ownership.ID); err != nil {
		return "", fmt.Errorf("activate vnstat systemd ownership: %w", err)
	}
	return vnstatSystemdUnitPath, nil
}

func (s *TrafficOverviewService) ResetAllTrafficOverviewStats() error {
	trafficOverviewOperationMu.Lock()
	defer trafficOverviewOperationMu.Unlock()
	if !IsSystemPlatformLinux() {
		return nil
	}
	if enabled, err := s.isOverviewEnabled(); err != nil {
		return err
	} else if !enabled {
		return nil
	}

	iface, err := detectDefaultTrafficInterface()
	if err != nil {
		return err
	}
	if iface == "" {
		return errors.New("default interface is empty")
	}

	vnstatUp, vnstatDown, err := loadVnstatTrafficTotals(iface)
	if err != nil {
		return err
	}

	_, resetDay, _, _, _, cfgErr := s.getOverviewConfig()
	if cfgErr != nil {
		return cfgErr
	}

	now := PanelNow()
	trafficOverviewStateMu.Lock()
	state, stateErr := s.loadRuntimeState()
	if stateErr != nil {
		trafficOverviewStateMu.Unlock()
		return stateErr
	}

	currentUp, currentDown, source, _, deriveErr := deriveCurrentAlltimeTotals(&state, iface, vnstatUp, vnstatDown)
	if deriveErr != nil {
		trafficOverviewStateMu.Unlock()
		return deriveErr
	}
	if _, normalizeErr := normalizeStateForTotals(&state, iface, currentUp, currentDown); normalizeErr != nil {
		trafficOverviewStateMu.Unlock()
		return normalizeErr
	}
	state.Interface = iface
	state.ManualBaseUp = currentUp
	state.ManualBaseDown = currentDown
	state.PeriodBaseUp = currentUp
	state.PeriodBaseDown = currentDown
	state.PeriodTag = computePeriodTag(resetDay, now)
	state.PeriodResetDay = normalizeResetDay(resetDay)
	state.LastFullResetAt = now.Unix()
	state.LastPeriodReset = now.Unix()

	err = s.saveRuntimeState(state)
	trafficOverviewStateMu.Unlock()
	if err != nil {
		return err
	}

	resetSnapshot := trafficOverviewSnapshot{
		Source:     source,
		Interface:  iface,
		Available:  true,
		Up:         0,
		Down:       0,
		Total:      0,
		AccumUp:    0,
		AccumDown:  0,
		AccumTotal: 0,
		UpdatedAt:  now.Unix(),
	}
	if err := s.stageOverviewSnapshot(resetSnapshot, true); err != nil {
		return err
	}
	markTrafficOverviewCapReconcileNeeded()
	if err := s.ReconcileTrafficCap(); err != nil {
		logger.Warning("reconcile traffic cap after manual reset failed:", err)
	}
	return nil
}

func (s *TrafficOverviewService) ResetPeriodTrafficOverviewStats() error {
	trafficOverviewOperationMu.Lock()
	defer trafficOverviewOperationMu.Unlock()
	if !IsSystemPlatformLinux() {
		return nil
	}
	if enabled, err := s.isOverviewEnabled(); err != nil {
		return err
	} else if !enabled {
		return nil
	}

	iface, err := detectDefaultTrafficInterface()
	if err != nil {
		return err
	}
	if iface == "" {
		return errors.New("default interface is empty")
	}

	vnstatUp, vnstatDown, err := loadVnstatTrafficTotals(iface)
	if err != nil {
		return err
	}

	_, resetDay, _, _, _, cfgErr := s.getOverviewConfig()
	if cfgErr != nil {
		return cfgErr
	}

	now := PanelNow()
	trafficOverviewStateMu.Lock()
	state, stateErr := s.loadRuntimeState()
	if stateErr != nil {
		trafficOverviewStateMu.Unlock()
		return stateErr
	}

	currentUp, currentDown, source, _, deriveErr := deriveCurrentAlltimeTotals(&state, iface, vnstatUp, vnstatDown)
	if deriveErr != nil {
		trafficOverviewStateMu.Unlock()
		return deriveErr
	}
	if _, normalizeErr := normalizeStateForTotals(&state, iface, currentUp, currentDown); normalizeErr != nil {
		trafficOverviewStateMu.Unlock()
		return normalizeErr
	}
	state.Interface = iface
	state.PeriodBaseUp = currentUp
	state.PeriodBaseDown = currentDown
	state.PeriodTag = computePeriodTag(resetDay, now)
	state.PeriodResetDay = normalizeResetDay(resetDay)
	state.LastPeriodReset = now.Unix()

	accumUp := nonNegativeDiff(currentUp, state.ManualBaseUp)
	accumDown := nonNegativeDiff(currentDown, state.ManualBaseDown)

	err = s.saveRuntimeState(state)
	trafficOverviewStateMu.Unlock()
	if err != nil {
		return err
	}

	resetSnapshot := trafficOverviewSnapshot{
		Source:     source,
		Interface:  iface,
		Available:  true,
		Up:         0,
		Down:       0,
		Total:      0,
		AccumUp:    accumUp,
		AccumDown:  accumDown,
		AccumTotal: accumUp + accumDown,
		UpdatedAt:  now.Unix(),
	}
	if err := s.stageOverviewSnapshot(resetSnapshot, true); err != nil {
		return err
	}
	markTrafficOverviewCapReconcileNeeded()
	if err := s.ReconcileTrafficCap(); err != nil {
		logger.Warning("reconcile traffic cap after period reset failed:", err)
	}
	return nil
}

func (s *TrafficOverviewService) ResetTotalTrafficOverviewStats() error {
	trafficOverviewOperationMu.Lock()
	defer trafficOverviewOperationMu.Unlock()
	if !IsSystemPlatformLinux() {
		return nil
	}
	if enabled, err := s.isOverviewEnabled(); err != nil {
		return err
	} else if !enabled {
		return nil
	}

	iface, err := detectDefaultTrafficInterface()
	if err != nil {
		return err
	}
	if iface == "" {
		return errors.New("default interface is empty")
	}

	vnstatUp, vnstatDown, err := loadVnstatTrafficTotals(iface)
	if err != nil {
		return err
	}

	now := PanelNow()
	trafficOverviewStateMu.Lock()
	state, stateErr := s.loadRuntimeState()
	if stateErr != nil {
		trafficOverviewStateMu.Unlock()
		return stateErr
	}

	currentUp, currentDown, source, _, deriveErr := deriveCurrentAlltimeTotals(&state, iface, vnstatUp, vnstatDown)
	if deriveErr != nil {
		trafficOverviewStateMu.Unlock()
		return deriveErr
	}
	if _, normalizeErr := normalizeStateForTotals(&state, iface, currentUp, currentDown); normalizeErr != nil {
		trafficOverviewStateMu.Unlock()
		return normalizeErr
	}
	state.Interface = iface
	state.ManualBaseUp = currentUp
	state.ManualBaseDown = currentDown
	state.LastFullResetAt = now.Unix()

	periodUp := nonNegativeDiff(currentUp, state.PeriodBaseUp)
	periodDown := nonNegativeDiff(currentDown, state.PeriodBaseDown)

	err = s.saveRuntimeState(state)
	trafficOverviewStateMu.Unlock()
	if err != nil {
		return err
	}

	resetSnapshot := trafficOverviewSnapshot{
		Source:     source,
		Interface:  iface,
		Available:  true,
		Up:         periodUp,
		Down:       periodDown,
		Total:      periodUp + periodDown,
		AccumUp:    0,
		AccumDown:  0,
		AccumTotal: 0,
		UpdatedAt:  now.Unix(),
	}
	if err := s.stageOverviewSnapshot(resetSnapshot, true); err != nil {
		return err
	}
	markTrafficOverviewCapReconcileNeeded()
	if err := s.ReconcileTrafficCap(); err != nil {
		logger.Warning("reconcile traffic cap after total reset failed:", err)
	}
	return nil
}

func (s *TrafficOverviewService) EnsureRuntimeReady() error {
	if !IsSystemPlatformLinux() {
		return nil
	}
	if enabled, err := s.isOverviewEnabled(); err != nil {
		return err
	} else if !enabled {
		return nil
	}
	if _, ok := loadTrustedVnstatManifest(); !ok {
		return nil
	}
	// Do not inspect any external process while the panel-owned daemon is
	// healthy. A collision is meaningful only when the panel daemon itself
	// cannot be started.
	if err := ensureVnstatDaemonRunning(); err != nil {
		logger.Warning("initial vnstat daemon ensure failed:", err)
		return err
	}

	iface, err := detectDefaultTrafficInterface()
	if err != nil {
		return err
	}
	if iface == "" {
		return nil
	}

	if err := ensureVnstatTracking(iface); err != nil {
		return err
	}
	if err := s.ReconcileTrafficCap(); err != nil {
		logger.Warning("initial traffic cap reconcile failed:", err)
	}
	return nil
}

func (s *TrafficOverviewService) FlushPendingSnapshot() error {
	if err := s.flushRuntimeState(); err != nil {
		return err
	}
	return s.flushOverviewSnapshot(true)
}

func (s *TrafficOverviewService) ReconcileTrafficCap() error {
	limitGiB, _, _, expiryBoundary, enabled, err := s.getOverviewConfig()
	if err != nil {
		return err
	}
	expired := isTrafficOverviewExpired(expiryBoundary, PanelNow())
	if !enabled || (limitGiB <= 0 && !expired) {
		return s.reconcileInactiveTrafficCap()
	}
	if !shouldEvaluateTrafficCapNow() {
		return nil
	}
	overview, err := s.GetTrafficOverview()
	if err != nil {
		return err
	}
	return s.reconcileTrafficCapFromOverview(overview)
}

// reconcileInactiveTrafficCap performs the expensive legacy rule probe only
// once after startup or a traffic-cap setting change. The normal disabled and
// unlimited state must not spawn nft commands every StatsJob tick.
func (s *TrafficOverviewService) reconcileInactiveTrafficCap() error {
	trafficOverviewCapMu.Lock()
	defer trafficOverviewCapMu.Unlock()
	if trafficOverviewCapStartupChecked {
		return nil
	}

	state, err := s.loadCapStateLocked()
	if err != nil {
		return err
	}
	hasRules := state.Active && IsSystemPlatformLinux() && nftSupported()
	if !hasRules && IsSystemPlatformLinux() && nftSupported() {
		hasRules = hasTrafficCapRules()
	}
	if hasRules {
		if err := cleanupTrafficCapRules(); err != nil {
			return err
		}
	}
	trafficOverviewCapStartupChecked = true
	if !state.Active && !state.LimitReached && len(state.AllowedPorts) == 0 {
		return nil
	}
	state.Active = false
	state.LimitReached = false
	state.AllowedPorts = nil
	state.UpdatedAt = time.Now().Unix()
	return s.saveCapStateLocked(state)
}

func markTrafficOverviewCapReconcileNeeded() {
	trafficOverviewCapMu.Lock()
	trafficOverviewCapStartupChecked = false
	trafficOverviewCapMu.Unlock()
	trafficOverviewCapScheduleMu.Lock()
	trafficOverviewCapLastEvaluatedAt = time.Time{}
	trafficOverviewCapScheduleMu.Unlock()
}

func shouldEvaluateTrafficCapNow() bool {
	trafficOverviewCapScheduleMu.Lock()
	defer trafficOverviewCapScheduleMu.Unlock()
	now := time.Now()
	if !trafficOverviewCapLastEvaluatedAt.IsZero() && now.Sub(trafficOverviewCapLastEvaluatedAt) < trafficOverviewCapEvaluateInterval {
		return false
	}
	trafficOverviewCapLastEvaluatedAt = now
	return true
}

func (s *TrafficOverviewService) CleanupTrafficCapOnShutdown() error {
	if !trafficOverviewShutdownEnabledFn() {
		return nil
	}

	trafficOverviewCapMu.Lock()
	defer trafficOverviewCapMu.Unlock()

	if err := cleanupTrafficCapRules(); err != nil {
		return err
	}
	state, err := s.loadCapStateLocked()
	if err != nil {
		return err
	}
	state.Active = false
	// Keep the last over-limit marker so startup reconcile can restore
	// traffic-cap rules even before live counters are available again.
	state.UpdatedAt = time.Now().Unix()
	return s.saveCapStateLocked(state)
}

func (s *TrafficOverviewService) reconcileTrafficCapFromOverview(overview *TrafficOverview) error {
	if !IsSystemPlatformLinux() || !nftSupported() {
		return nil
	}

	limitGiB := 0.0
	expired := false
	if overview != nil {
		limitGiB = normalizeLimitGiB(overview.LimitGiB)
		expired = overview.Expired
	}
	if limitGiB <= 0 {
		loadedLimitGiB, _, _, expiryBoundary, _, err := s.getOverviewConfig()
		if err == nil {
			limitGiB = normalizeLimitGiB(loadedLimitGiB)
			if overview == nil {
				expired = isTrafficOverviewExpired(expiryBoundary, PanelNow())
			}
		}
	}

	allowedPorts := resolveTrafficCapAllowedPorts()
	if len(allowedPorts) == 0 {
		allowedPorts = []int{22}
	}

	trafficOverviewCapMu.Lock()
	defer trafficOverviewCapMu.Unlock()

	state, err := s.loadCapStateLocked()
	if err != nil {
		return err
	}
	state.AllowedPorts = normalizePortList(state.AllowedPorts)

	active := state.Active
	previousActive := state.Active
	previousLimitReached := state.LimitReached
	previousAllowedPorts := append([]int(nil), state.AllowedPorts...)
	probeLegacyRules := !trafficOverviewCapStartupChecked
	legacyRules := false
	if probeLegacyRules {
		legacyRules = hasTrafficCapRules()
		trafficOverviewCapStartupChecked = true
	}
	limitReached := evaluateTrafficOverviewCapReached(
		state.LimitReached,
		limitGiB,
		trafficCapUsageBytes(overview),
		func() bool {
			if overview == nil {
				return false
			}
			return overview.Available
		}(),
		func() string {
			if overview == nil {
				return ""
			}
			return overview.Error
		}(),
		expired,
		overview != nil,
	)

	desiredActive := (limitGiB > 0 || expired) && limitReached

	if desiredActive {
		hasRules := legacyRules || (active && hasTrafficCapRules())
		needsRuleRefresh := !active || !intSliceEqual(state.AllowedPorts, allowedPorts) || !hasRules
		if needsRuleRefresh {
			if err := applyTrafficCapRules(allowedPorts); err != nil {
				return err
			}
			hasRules = true
		}
		if state.Active == hasRules && state.LimitReached && intSliceEqual(state.AllowedPorts, allowedPorts) {
			return nil
		}
		state.Active = hasRules
		state.LimitReached = true
		state.AllowedPorts = allowedPorts
		state.UpdatedAt = time.Now().Unix()
		return s.saveCapStateLocked(state)
	}

	if active || legacyRules {
		if err := cleanupTrafficCapRules(); err != nil {
			return err
		}
	}
	state.Active = false
	state.LimitReached = false
	state.AllowedPorts = nil
	if !previousActive && !previousLimitReached && len(previousAllowedPorts) == 0 {
		return nil
	}
	state.UpdatedAt = time.Now().Unix()
	return s.saveCapStateLocked(state)
}

func (s *TrafficOverviewService) getOverviewConfig() (float64, int, string, time.Time, bool, error) {
	trafficOverviewConfigMu.Lock()
	if trafficOverviewConfigCache.loaded && time.Since(trafficOverviewConfigCache.updatedAt) < trafficOverviewConfigCacheTTL {
		cache := trafficOverviewConfigCache
		trafficOverviewConfigMu.Unlock()
		return cache.limitGiB, cache.resetDay, cache.expiryDate, cache.expiryBoundary, cache.enabled, nil
	}
	trafficOverviewConfigMu.Unlock()

	limitGiB, resetDay, expiryDate, expiryBoundary, enabled, err := s.getOverviewConfigUncached()
	if err != nil {
		return 0, 0, "", time.Time{}, true, err
	}
	trafficOverviewConfigMu.Lock()
	trafficOverviewConfigCache = trafficOverviewConfigCacheState{
		loaded:         true,
		limitGiB:       limitGiB,
		resetDay:       resetDay,
		expiryDate:     expiryDate,
		expiryBoundary: expiryBoundary,
		enabled:        enabled,
		updatedAt:      time.Now(),
	}
	trafficOverviewConfigMu.Unlock()
	return limitGiB, resetDay, expiryDate, expiryBoundary, enabled, nil
}

func (s *TrafficOverviewService) getOverviewConfigUncached() (float64, int, string, time.Time, bool, error) {
	settingSvc := &SettingService{}

	limitRaw, err := settingSvc.getString(trafficOverviewLimitGiBKey)
	if err != nil {
		return 0, 0, "", time.Time{}, true, err
	}
	limitGiB := 0.0
	if strings.TrimSpace(limitRaw) != "" {
		if parsed, parseErr := strconv.ParseFloat(strings.TrimSpace(limitRaw), 64); parseErr == nil {
			limitGiB = parsed
		}
	}

	resetRaw, err := settingSvc.getString(trafficOverviewResetDayKey)
	if err != nil {
		return 0, 0, "", time.Time{}, true, err
	}
	resetDay := 0
	if strings.TrimSpace(resetRaw) != "" {
		if parsed, parseErr := strconv.Atoi(strings.TrimSpace(resetRaw)); parseErr == nil {
			resetDay = parsed
		}
	}

	expiryRaw, err := settingSvc.getString(trafficOverviewExpiryDateKey)
	if err != nil {
		return 0, 0, "", time.Time{}, true, err
	}
	expiryDate, expiryBoundary, expiryErr := parseTrafficOverviewExpiryDate(expiryRaw, s.getOverviewLocation())
	if expiryErr != nil {
		return 0, 0, "", time.Time{}, true, expiryErr
	}

	enabled, err := settingSvc.getBool(trafficOverviewEnabledKey)
	if err != nil {
		return 0, 0, "", time.Time{}, true, err
	}

	return normalizeLimitGiB(limitGiB), normalizeResetDay(resetDay), expiryDate, expiryBoundary, enabled, nil
}

func invalidateTrafficOverviewConfigCache() {
	trafficOverviewConfigMu.Lock()
	trafficOverviewConfigCache = trafficOverviewConfigCacheState{}
	trafficOverviewConfigMu.Unlock()
}

func (s *TrafficOverviewService) loadRuntimeState() (trafficOverviewRuntimeState, error) {
	trafficOverviewRuntimeStateCacheMu.Lock()
	if trafficOverviewRuntimeStateCache.Loaded {
		state := trafficOverviewRuntimeState{}
		if trafficOverviewRuntimeStateCache.HasPending {
			state = trafficOverviewRuntimeStateCache.Pending
		} else if trafficOverviewRuntimeStateCache.HasPersisted {
			state = trafficOverviewRuntimeStateCache.Persisted
		}
		trafficOverviewRuntimeStateCacheMu.Unlock()
		return state, nil
	}
	trafficOverviewRuntimeStateCacheMu.Unlock()

	settingSvc := &SettingService{}
	raw, err := settingSvc.getString(trafficOverviewStateKey)
	if err != nil {
		return trafficOverviewRuntimeState{}, err
	}

	state := trafficOverviewRuntimeState{}
	trimmed := strings.TrimSpace(raw)
	if trimmed != "" && trimmed != "{}" {
		if err := json.Unmarshal([]byte(trimmed), &state); err != nil {
			return trafficOverviewRuntimeState{}, err
		}
	}
	state = normalizeTrafficOverviewRuntimeState(state)

	trafficOverviewRuntimeStateCacheMu.Lock()
	if !trafficOverviewRuntimeStateCache.Loaded {
		trafficOverviewRuntimeStateCache = trafficOverviewRuntimeStateCacheState{
			Loaded:       true,
			HasPersisted: true,
			Persisted:    state,
		}
	}
	if trafficOverviewRuntimeStateCache.HasPending {
		state = trafficOverviewRuntimeStateCache.Pending
	} else if trafficOverviewRuntimeStateCache.HasPersisted {
		state = trafficOverviewRuntimeStateCache.Persisted
	}
	trafficOverviewRuntimeStateCacheMu.Unlock()
	return state, nil
}

func normalizeTrafficOverviewRuntimeState(state trafficOverviewRuntimeState) trafficOverviewRuntimeState {
	state.ManualBaseUp = maxInt64(state.ManualBaseUp, 0)
	state.ManualBaseDown = maxInt64(state.ManualBaseDown, 0)
	state.PeriodBaseUp = maxInt64(state.PeriodBaseUp, 0)
	state.PeriodBaseDown = maxInt64(state.PeriodBaseDown, 0)
	state.PeriodResetDay = normalizeResetDay(state.PeriodResetDay)
	state.LastFullResetAt = maxInt64(state.LastFullResetAt, 0)
	state.LastPeriodReset = maxInt64(state.LastPeriodReset, 0)
	state.KernelOffsetUp = maxInt64(state.KernelOffsetUp, 0)
	state.KernelOffsetDown = maxInt64(state.KernelOffsetDown, 0)
	state.LastKernelUp = maxInt64(state.LastKernelUp, 0)
	state.LastKernelDown = maxInt64(state.LastKernelDown, 0)
	return state
}

func (s *TrafficOverviewService) saveRuntimeState(state trafficOverviewRuntimeState) error {
	state = normalizeTrafficOverviewRuntimeState(state)
	raw, err := json.Marshal(state)
	if err != nil {
		return err
	}
	if err := (&SettingService{}).setString(trafficOverviewStateKey, string(raw)); err != nil {
		return err
	}
	trafficOverviewRuntimeStateCacheMu.Lock()
	pendingChanged := trafficOverviewRuntimeStateCache.HasPending && trafficOverviewRuntimeStateCache.Pending != state
	trafficOverviewRuntimeStateCache.Loaded = true
	trafficOverviewRuntimeStateCache.HasPersisted = true
	trafficOverviewRuntimeStateCache.Persisted = state
	trafficOverviewRuntimeStateCache.LastFlushAt = time.Now()
	if !pendingChanged {
		trafficOverviewRuntimeStateCache.HasPending = false
		trafficOverviewRuntimeStateCache.Pending = trafficOverviewRuntimeState{}
	}
	trafficOverviewRuntimeStateCacheMu.Unlock()
	return nil
}

func (s *TrafficOverviewService) stageRuntimeState(state trafficOverviewRuntimeState, force bool) error {
	state = normalizeTrafficOverviewRuntimeState(state)
	trafficOverviewRuntimeStateCacheMu.Lock()
	if !trafficOverviewRuntimeStateCache.Loaded {
		trafficOverviewRuntimeStateCacheMu.Unlock()
		if _, err := s.loadRuntimeState(); err != nil {
			return err
		}
		trafficOverviewRuntimeStateCacheMu.Lock()
	}
	trafficOverviewRuntimeStateCache.Pending = state
	trafficOverviewRuntimeStateCache.HasPending = true
	shouldFlush := force || !trafficOverviewRuntimeStateCache.HasPersisted ||
		trafficOverviewRuntimeStateCache.LastFlushAt.IsZero() ||
		time.Since(trafficOverviewRuntimeStateCache.LastFlushAt) >= trafficOverviewFlushInterval
	trafficOverviewRuntimeStateCacheMu.Unlock()
	if !shouldFlush {
		return nil
	}
	return s.flushRuntimeState()
}

func (s *TrafficOverviewService) flushRuntimeState() error {
	trafficOverviewRuntimeStateCacheMu.Lock()
	if !trafficOverviewRuntimeStateCache.HasPending {
		trafficOverviewRuntimeStateCacheMu.Unlock()
		return nil
	}
	pending := trafficOverviewRuntimeStateCache.Pending
	trafficOverviewRuntimeStateCacheMu.Unlock()
	return s.saveRuntimeState(pending)
}

func (s *TrafficOverviewService) loadCapStateLocked() (trafficOverviewCapState, error) {
	settingSvc := &SettingService{}
	raw, err := settingSvc.getString(trafficOverviewCapStateKey)
	if err != nil {
		return trafficOverviewCapState{}, err
	}

	state := trafficOverviewCapState{}
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "{}" {
		return state, nil
	}
	if err := json.Unmarshal([]byte(trimmed), &state); err != nil {
		return trafficOverviewCapState{}, err
	}

	state.AllowedPorts = normalizePortList(state.AllowedPorts)
	state.UpdatedAt = maxInt64(state.UpdatedAt, 0)
	return state, nil
}

func (s *TrafficOverviewService) saveCapStateLocked(state trafficOverviewCapState) error {
	state.AllowedPorts = normalizePortList(state.AllowedPorts)
	state.UpdatedAt = maxInt64(state.UpdatedAt, 0)
	raw, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return (&SettingService{}).setString(trafficOverviewCapStateKey, string(raw))
}

func (s *TrafficOverviewService) loadPauseState() (trafficOverviewPauseState, bool) {
	raw, err := (&SettingService{}).getString(trafficOverviewPauseStateKey)
	if err != nil {
		return trafficOverviewPauseState{}, false
	}

	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "{}" {
		return trafficOverviewPauseState{}, false
	}

	state := trafficOverviewPauseState{}
	if err := json.Unmarshal([]byte(trimmed), &state); err != nil {
		logger.Warning("load traffic overview pause state failed:", err)
		return trafficOverviewPauseState{}, false
	}
	state.Interface = strings.TrimSpace(state.Interface)
	state.CurrentUp = maxInt64(state.CurrentUp, 0)
	state.CurrentDown = maxInt64(state.CurrentDown, 0)
	state.PeriodBaseUp = maxInt64(state.PeriodBaseUp, 0)
	state.PeriodBaseDown = maxInt64(state.PeriodBaseDown, 0)
	state.ManualBaseUp = maxInt64(state.ManualBaseUp, 0)
	state.ManualBaseDown = maxInt64(state.ManualBaseDown, 0)
	state.LastVnstatUp = maxInt64(state.LastVnstatUp, 0)
	state.LastVnstatDown = maxInt64(state.LastVnstatDown, 0)
	state.Snapshot = normalizeOverviewSnapshot(state.Snapshot)
	state.PausedAt = maxInt64(state.PausedAt, 0)
	return state, state.Paused
}

func (s *TrafficOverviewService) savePauseState(state trafficOverviewPauseState) error {
	state.Interface = strings.TrimSpace(state.Interface)
	state.CurrentUp = maxInt64(state.CurrentUp, 0)
	state.CurrentDown = maxInt64(state.CurrentDown, 0)
	state.PeriodBaseUp = maxInt64(state.PeriodBaseUp, 0)
	state.PeriodBaseDown = maxInt64(state.PeriodBaseDown, 0)
	state.ManualBaseUp = maxInt64(state.ManualBaseUp, 0)
	state.ManualBaseDown = maxInt64(state.ManualBaseDown, 0)
	state.LastVnstatUp = maxInt64(state.LastVnstatUp, 0)
	state.LastVnstatDown = maxInt64(state.LastVnstatDown, 0)
	state.Snapshot = normalizeOverviewSnapshot(state.Snapshot)
	state.PausedAt = maxInt64(state.PausedAt, 0)
	raw, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return (&SettingService{}).setString(trafficOverviewPauseStateKey, string(raw))
}

func (s *TrafficOverviewService) clearPauseState() error {
	return (&SettingService{}).setString(trafficOverviewPauseStateKey, "{}")
}

func (s *TrafficOverviewService) loadSnapshotCacheLocked() error {
	if trafficOverviewSnapshotCache.Loaded {
		return nil
	}
	trafficOverviewSnapshotCache.Loaded = true

	settingSvc := &SettingService{}
	raw, err := settingSvc.getString(trafficOverviewSnapshotKey)
	if err != nil {
		return err
	}

	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "{}" {
		return nil
	}

	var snapshot trafficOverviewSnapshot
	if err := json.Unmarshal([]byte(trimmed), &snapshot); err != nil {
		return err
	}
	snapshot = normalizeOverviewSnapshot(snapshot)
	trafficOverviewSnapshotCache.HasPersisted = true
	trafficOverviewSnapshotCache.Persisted = snapshot
	return nil
}

func (s *TrafficOverviewService) getSnapshotForDisplay() (trafficOverviewSnapshot, bool) {
	trafficOverviewSnapshotMu.Lock()
	defer trafficOverviewSnapshotMu.Unlock()

	if err := s.loadSnapshotCacheLocked(); err != nil {
		logger.Warning("load traffic overview snapshot cache failed:", err)
		return trafficOverviewSnapshot{}, false
	}
	if trafficOverviewSnapshotCache.HasPending {
		return trafficOverviewSnapshotCache.Pending, true
	}
	if trafficOverviewSnapshotCache.HasPersisted {
		return trafficOverviewSnapshotCache.Persisted, true
	}
	return trafficOverviewSnapshot{}, false
}

func (s *TrafficOverviewService) saveOverviewSnapshot(snapshot trafficOverviewSnapshot) error {
	raw, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	return (&SettingService{}).setString(trafficOverviewSnapshotKey, string(raw))
}

func (s *TrafficOverviewService) stageOverviewSnapshot(snapshot trafficOverviewSnapshot, force bool) error {
	snapshot = normalizeOverviewSnapshot(snapshot)

	trafficOverviewSnapshotMu.Lock()
	if err := s.loadSnapshotCacheLocked(); err != nil {
		trafficOverviewSnapshotMu.Unlock()
		return err
	}

	trafficOverviewSnapshotCache.Pending = snapshot
	trafficOverviewSnapshotCache.HasPending = true
	now := time.Now()

	shouldFlush := force
	if !shouldFlush {
		if !trafficOverviewSnapshotCache.HasPersisted || trafficOverviewSnapshotCache.LastFlushAt.IsZero() {
			shouldFlush = true
		} else {
			elapsed := now.Sub(trafficOverviewSnapshotCache.LastFlushAt)
			if elapsed >= trafficOverviewFlushInterval {
				shouldFlush = true
			} else if snapshotDeltaBytes(snapshot, trafficOverviewSnapshotCache.Persisted) >= trafficOverviewFlushDelta {
				shouldFlush = true
			}
		}
	}
	trafficOverviewSnapshotMu.Unlock()

	if !shouldFlush {
		return nil
	}
	return s.flushOverviewSnapshot(force)
}

func (s *TrafficOverviewService) flushOverviewSnapshot(force bool) error {
	trafficOverviewSnapshotMu.Lock()
	if err := s.loadSnapshotCacheLocked(); err != nil {
		trafficOverviewSnapshotMu.Unlock()
		return err
	}
	if !trafficOverviewSnapshotCache.HasPending {
		trafficOverviewSnapshotMu.Unlock()
		return nil
	}

	pending := trafficOverviewSnapshotCache.Pending
	if !force && trafficOverviewSnapshotCache.HasPersisted && !trafficOverviewSnapshotCache.LastFlushAt.IsZero() {
		elapsed := time.Since(trafficOverviewSnapshotCache.LastFlushAt)
		if elapsed < trafficOverviewFlushInterval &&
			snapshotDeltaBytes(pending, trafficOverviewSnapshotCache.Persisted) < trafficOverviewFlushDelta {
			trafficOverviewSnapshotMu.Unlock()
			return nil
		}
	}
	trafficOverviewSnapshotMu.Unlock()

	if err := s.saveOverviewSnapshot(pending); err != nil {
		return err
	}

	trafficOverviewSnapshotMu.Lock()
	if trafficOverviewSnapshotCache.HasPending && trafficOverviewSnapshotCache.Pending == pending {
		trafficOverviewSnapshotCache.HasPending = false
	}
	trafficOverviewSnapshotCache.HasPersisted = true
	trafficOverviewSnapshotCache.Persisted = pending
	trafficOverviewSnapshotCache.LastFlushAt = time.Now()
	trafficOverviewSnapshotMu.Unlock()
	return nil
}

func snapshotFromOverview(overview *TrafficOverview) trafficOverviewSnapshot {
	if overview == nil {
		return trafficOverviewSnapshot{}
	}
	return normalizeOverviewSnapshot(trafficOverviewSnapshot{
		Source:     overview.Source,
		Interface:  overview.Interface,
		Available:  overview.Available,
		Up:         overview.Up,
		Down:       overview.Down,
		Total:      overview.Total,
		AccumUp:    overview.AccumUp,
		AccumDown:  overview.AccumDown,
		AccumTotal: overview.AccumTotal,
		UpdatedAt:  overview.UpdatedAt,
	})
}

func applySnapshotToOverview(overview *TrafficOverview, snapshot trafficOverviewSnapshot) {
	if overview == nil {
		return
	}
	snapshot = normalizeOverviewSnapshot(snapshot)
	overview.Source = snapshot.Source
	overview.Interface = snapshot.Interface
	overview.Available = snapshot.Available
	overview.Up = snapshot.Up
	overview.Down = snapshot.Down
	overview.Total = snapshot.Total
	overview.AccumUp = snapshot.AccumUp
	overview.AccumDown = snapshot.AccumDown
	overview.AccumTotal = snapshot.AccumTotal
	if snapshot.UpdatedAt > 0 {
		overview.UpdatedAt = snapshot.UpdatedAt
	}
}

func normalizeOverviewSnapshot(snapshot trafficOverviewSnapshot) trafficOverviewSnapshot {
	snapshot.Source = strings.TrimSpace(snapshot.Source)
	if snapshot.Source == "" {
		snapshot.Source = "vnstat"
	}
	snapshot.Interface = strings.TrimSpace(snapshot.Interface)

	snapshot.Up = maxInt64(snapshot.Up, 0)
	snapshot.Down = maxInt64(snapshot.Down, 0)
	snapshot.Total = maxInt64(snapshot.Total, 0)
	snapshot.AccumUp = maxInt64(snapshot.AccumUp, 0)
	snapshot.AccumDown = maxInt64(snapshot.AccumDown, 0)
	snapshot.AccumTotal = maxInt64(snapshot.AccumTotal, 0)
	snapshot.UpdatedAt = maxInt64(snapshot.UpdatedAt, 0)
	return snapshot
}

func snapshotDeltaBytes(current trafficOverviewSnapshot, previous trafficOverviewSnapshot) int64 {
	totalDelta := absInt64(current.Total - previous.Total)
	accumDelta := absInt64(current.AccumTotal - previous.AccumTotal)
	if accumDelta > totalDelta {
		return accumDelta
	}
	return totalDelta
}

func absInt64(value int64) int64 {
	if value < 0 {
		return -value
	}
	return value
}

func (s *TrafficOverviewService) getOverviewLocation() *time.Location {
	loc, err := (&SettingService{}).GetTimeLocation()
	if err != nil || loc == nil {
		return time.UTC
	}
	return loc
}

func normalizeStateForTotals(state *trafficOverviewRuntimeState, iface string, up int64, down int64) (bool, error) {
	if state == nil {
		return false, errors.New("state is nil")
	}

	up = maxInt64(up, 0)
	down = maxInt64(down, 0)

	needsReset := strings.TrimSpace(state.Interface) == "" || state.Interface != iface
	needsReset = needsReset || state.ManualBaseUp > up || state.ManualBaseDown > down
	needsReset = needsReset || state.PeriodBaseUp > up || state.PeriodBaseDown > down
	if needsReset {
		state.Interface = iface
		state.ManualBaseUp = up
		state.ManualBaseDown = down
		state.PeriodBaseUp = up
		state.PeriodBaseDown = down
		state.PeriodTag = ""
		state.PeriodResetDay = 0
		state.LastPeriodReset = time.Now().Unix()
		return true, nil
	}
	return false, nil
}

func applyPeriodResetIfNeeded(state *trafficOverviewRuntimeState, resetDay int, up int64, down int64, now time.Time) (bool, error) {
	if state == nil {
		return false, errors.New("state is nil")
	}

	resetDay = normalizeResetDay(resetDay)
	if resetDay <= 0 {
		changed := state.PeriodTag != "" || state.PeriodResetDay != 0
		state.PeriodTag = ""
		state.PeriodResetDay = 0
		return changed, nil
	}

	expectedTag := computePeriodTag(resetDay, now)
	if state.PeriodResetDay != resetDay {
		changed := state.PeriodResetDay != resetDay || state.PeriodTag != expectedTag
		state.PeriodResetDay = resetDay
		state.PeriodTag = expectedTag
		return changed, nil
	}
	if strings.TrimSpace(state.PeriodTag) == "" {
		state.PeriodTag = expectedTag
		return true, nil
	}
	if state.PeriodTag == expectedTag {
		return false, nil
	}
	state.PeriodTag = expectedTag
	state.PeriodBaseUp = maxInt64(up, 0)
	state.PeriodBaseDown = maxInt64(down, 0)
	state.LastPeriodReset = now.Unix()
	return true, nil
}

func computePeriodTag(resetDay int, now time.Time) string {
	if resetDay <= 0 {
		return ""
	}
	boundary, ok := latestClientMonthlyResetBoundary(resetDay, now)
	if !ok || boundary.IsZero() {
		return ""
	}
	return fmt.Sprintf("boundary:%d", boundary.Unix())
}

func clampResetDayToMonthEnd(resetDay int, year int, month time.Month, loc *time.Location) int {
	if resetDay <= 0 {
		return 0
	}
	lastDay := time.Date(year, month+1, 0, 0, 0, 0, 0, loc).Day()
	if resetDay > lastDay {
		return lastDay
	}
	return resetDay
}

func normalizeLimitGiB(value float64) float64 {
	if !isFiniteFloat(value) || value < 0 {
		return 0
	}
	rounded := math.Round(value*100) / 100
	if rounded > 0 && rounded < trafficOverviewMinDisplayGiB {
		return trafficOverviewMinDisplayGiB
	}
	return rounded
}

func normalizeResetDay(value int) int {
	if value < 0 {
		return 0
	}
	if value > 31 {
		return 31
	}
	return value
}

func normalizeTrafficOverviewExpiryDate(value string) (string, error) {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", nil
	}
	parsed, err := time.Parse("2006-01-02", trimmed)
	if err != nil {
		return "", fmt.Errorf("invalid expiry_date, expected YYYY-MM-DD: %w", err)
	}
	return parsed.Format("2006-01-02"), nil
}

func parseTrafficOverviewExpiryDate(value string, loc *time.Location) (string, time.Time, error) {
	normalized, err := normalizeTrafficOverviewExpiryDate(value)
	if err != nil {
		return "", time.Time{}, err
	}
	if normalized == "" {
		return "", time.Time{}, nil
	}
	if loc == nil {
		loc = time.UTC
	}
	parsed, err := time.ParseInLocation("2006-01-02", normalized, loc)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("parse expiry date failed: %w", err)
	}
	return normalized, parsed, nil
}

func isTrafficOverviewExpired(boundary time.Time, now time.Time) bool {
	if boundary.IsZero() {
		return false
	}
	if now.IsZero() {
		now = PanelNow()
	}
	if now.Location() != boundary.Location() {
		now = now.In(boundary.Location())
	}
	return !now.Before(boundary)
}

func isFiniteFloat(value float64) bool {
	return !math.IsInf(value, 0) && !math.IsNaN(value)
}

func nonNegativeDiff(current int64, baseline int64) int64 {
	diff := current - baseline
	if diff < 0 {
		return 0
	}
	return diff
}

func maxInt64(value int64, floor int64) int64 {
	if value < floor {
		return floor
	}
	return value
}

func limitGiBToBytes(limitGiB float64) int64 {
	normalized := normalizeLimitGiB(limitGiB)
	if normalized <= 0 {
		return 0
	}
	total := normalized * 1024 * 1024 * 1024
	if total >= float64(maxInt64AsUint64) {
		return int64(maxInt64AsUint64)
	}
	if total <= 0 {
		return 0
	}
	return int64(total)
}

// trafficCapUsageBytes returns the current billing-period usage. Historical
// accumulated traffic is intentionally independent from the monthly reset and
// must not cause a new period to remain capped.
func trafficCapUsageBytes(overview *TrafficOverview) int64 {
	if overview == nil {
		return 0
	}
	return maxInt64(overview.Total, 0)
}

func evaluateTrafficOverviewCapReached(previous bool, limitGiB float64, periodTotal int64, available bool, errText string, expired bool, hasOverview bool) bool {
	if expired {
		return true
	}

	normalizedLimitGiB := normalizeLimitGiB(limitGiB)
	if normalizedLimitGiB <= 0 {
		return false
	}
	if hasOverview && available && strings.TrimSpace(errText) == "" {
		return periodTotal >= limitGiBToBytes(normalizedLimitGiB)
	}
	return previous
}

func intSliceEqual(left []int, right []int) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func resolveTrafficCapAllowedPorts() []int {
	normalized := resolveFirewallDefaultPorts().All
	if len(normalized) == 0 {
		return []int{22}
	}
	return normalized
}

func detectSSHPorts() []int {
	if !IsSystemPlatformLinux() {
		return []int{22}
	}
	probe, err := probeFirewallSSHConfigCached(detectSSHConfigMainPath())
	if err == nil {
		ports := append([]int(nil), probe.Ports...)
		if len(ports) > 0 {
			return ports
		}
	}
	ports := parseSSHPortsFromConfig(detectSSHConfigMainPath())
	if len(ports) == 0 {
		return []int{22}
	}
	return ports
}

func parseSSHPortsFromConfig(rootPath string) []int {
	visited := map[string]struct{}{}
	ports := map[int]struct{}{}
	collectSSHPortsFromFile(rootPath, visited, ports)
	if len(ports) == 0 {
		return nil
	}

	result := make([]int, 0, len(ports))
	for port := range ports {
		result = append(result, port)
	}
	sort.Ints(result)
	return result
}

func collectSSHPortsFromFile(path string, visited map[string]struct{}, ports map[int]struct{}) {
	path = strings.TrimSpace(path)
	if path == "" {
		return
	}
	cleanPath := filepath.Clean(path)
	if _, exists := visited[cleanPath]; exists {
		return
	}
	visited[cleanPath] = struct{}{}

	content, err := os.ReadFile(cleanPath)
	if err != nil {
		return
	}

	lines := strings.Split(string(content), "\n")
	inMatchBlock := false
	for _, rawLine := range lines {
		line := stripSSHConfigComment(rawLine)
		if line == "" {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		key := strings.ToLower(fields[0])
		if key == "match" {
			inMatchBlock = true
			continue
		}
		if inMatchBlock {
			continue
		}
		switch key {
		case "port":
			for _, value := range fields[1:] {
				port, parseErr := strconv.Atoi(strings.Trim(value, "\"'"))
				if parseErr != nil || port < 1 || port > 65535 {
					continue
				}
				ports[port] = struct{}{}
			}
		case "include":
			for _, includePattern := range fields[1:] {
				for _, includeFile := range expandSSHIncludePattern(includePattern, cleanPath) {
					collectSSHPortsFromFile(includeFile, visited, ports)
				}
			}
		}
	}
}

func stripSSHConfigComment(line string) string {
	commentIndex := strings.Index(line, "#")
	if commentIndex >= 0 {
		line = line[:commentIndex]
	}
	return strings.TrimSpace(line)
}

func expandSSHIncludePattern(pattern string, basePath string) []string {
	pattern = strings.Trim(strings.TrimSpace(pattern), "\"'")
	if pattern == "" {
		return nil
	}

	if strings.HasPrefix(pattern, "~") {
		if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
			pattern = filepath.Join(home, strings.TrimPrefix(pattern, "~"))
		}
	}
	if !filepath.IsAbs(pattern) {
		pattern = filepath.Join(filepath.Dir(basePath), pattern)
	}

	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) == 0 {
		if _, statErr := os.Stat(pattern); statErr == nil {
			return []string{filepath.Clean(pattern)}
		}
		return nil
	}
	for index := range matches {
		matches[index] = filepath.Clean(matches[index])
	}
	sort.Strings(matches)
	return matches
}

func applyTrafficCapRules(allowedPorts []int) error {
	if !IsSystemPlatformLinux() || !nftSupported() {
		return nil
	}

	normalized := normalizePortList(allowedPorts)
	if len(normalized) == 0 {
		return errors.New("traffic cap allowlist is empty")
	}
	if err := cleanupTrafficCapRules(); err != nil {
		return err
	}
	if err := ensureNftBase(); err != nil {
		return err
	}

	if _, err := addLoopbackAcceptRule(nftChainIn, trafficCapNftRuleComments.in(trafficCapTagLoopback)); err != nil {
		return err
	}
	if _, err := addLoopbackAcceptRule(nftChainOut, trafficCapNftRuleComments.out(trafficCapTagLoopback)); err != nil {
		return err
	}
	if _, err := addDropExceptPortsRule(nftChainIn, "dport", normalized, trafficCapNftRuleComments.in(trafficCapTagDropExcept)); err != nil {
		return err
	}
	if _, err := addDropExceptPortsRule(nftChainOut, "sport", normalized, trafficCapNftRuleComments.out(trafficCapTagDropExcept)); err != nil {
		return err
	}
	if _, err := addDropAllTransportRule(nftChainForward, trafficCapNftRuleComments.forward(trafficCapTagDropForward)); err != nil {
		return err
	}
	if err := flushConntrackTable(); err != nil {
		logger.Warning("failed to flush conntrack after traffic cap apply: ", err)
	}
	return nil
}

func cleanupTrafficCapRules() error {
	if !IsSystemPlatformLinux() || !nftSupported() {
		return nil
	}
	return deleteRulesByCommentPrefix(trafficCapNftRuleComments.prefix)
}

func hasTrafficCapRules() bool {
	if !IsSystemPlatformLinux() || !nftSupported() || !nftTableExists() {
		return false
	}
	inHandle := findHandleByComment(nftChainIn, trafficCapNftRuleComments.in(trafficCapTagDropExcept))
	outHandle := findHandleByComment(nftChainOut, trafficCapNftRuleComments.out(trafficCapTagDropExcept))
	forwardHandle := findHandleByComment(nftChainForward, trafficCapNftRuleComments.forward(trafficCapTagDropForward))
	return inHandle > 0 && outHandle > 0 && forwardHandle > 0
}

func deriveCurrentAlltimeTotals(state *trafficOverviewRuntimeState, iface string, vnstatUp int64, vnstatDown int64) (int64, int64, string, bool, error) {
	if state == nil {
		return 0, 0, "vnstat", false, errors.New("state is nil")
	}

	vnstatUp = maxInt64(vnstatUp, 0)
	vnstatDown = maxInt64(vnstatDown, 0)
	ifaceChanged := strings.TrimSpace(state.Interface) == "" || state.Interface != strings.TrimSpace(iface)

	kernelUp, kernelDown, err := loadKernelTrafficTotals(iface)
	if err != nil {
		return vnstatUp, vnstatDown, "vnstat", false, nil
	}

	upChanged, currentUp := reconcileKernelRealtimeCounter(&state.KernelOffsetUp, &state.LastKernelUp, ifaceChanged, vnstatUp, kernelUp)
	downChanged, currentDown := reconcileKernelRealtimeCounter(&state.KernelOffsetDown, &state.LastKernelDown, ifaceChanged, vnstatDown, kernelDown)
	return currentUp, currentDown, "vnstat+kernel", upChanged || downChanged, nil
}

func reconcileKernelRealtimeCounter(offset *int64, lastKernel *int64, ifaceChanged bool, vnstatCurrent int64, kernelCurrent int64) (bool, int64) {
	changed := false
	vnstatCurrent = maxInt64(vnstatCurrent, 0)
	kernelCurrent = maxInt64(kernelCurrent, 0)

	if ifaceChanged {
		nextOffset := nonNegativeDiff(vnstatCurrent, kernelCurrent)
		if *offset != nextOffset {
			*offset = nextOffset
			changed = true
		}
		if *lastKernel != kernelCurrent {
			*lastKernel = kernelCurrent
			changed = true
		}
		return changed, maxInt64(vnstatCurrent, nextOffset+kernelCurrent)
	}

	previousSynthetic := maxInt64(*offset, 0) + maxInt64(*lastKernel, 0)
	if kernelCurrent < *lastKernel {
		nextSyntheticBase := maxInt64(previousSynthetic, vnstatCurrent)
		nextOffset := nonNegativeDiff(nextSyntheticBase, kernelCurrent)
		if *offset != nextOffset {
			*offset = nextOffset
			changed = true
		}
	}

	current := maxInt64(vnstatCurrent, maxInt64(*offset, 0)+kernelCurrent)
	desiredOffset := nonNegativeDiff(current, kernelCurrent)
	if *offset != desiredOffset {
		*offset = desiredOffset
		changed = true
	}
	if *lastKernel != kernelCurrent {
		*lastKernel = kernelCurrent
		changed = true
	}
	return changed, current
}

func loadKernelTrafficTotals(iface string) (int64, int64, error) {
	if strings.TrimSpace(iface) == "" {
		return 0, 0, errors.New("default interface is empty")
	}

	ioStats, err := psnet.IOCounters(true)
	if err != nil {
		return 0, 0, err
	}
	for _, stat := range ioStats {
		if stat.Name != iface {
			continue
		}
		return uint64ToSafeInt64(stat.BytesSent), uint64ToSafeInt64(stat.BytesRecv), nil
	}
	return 0, 0, fmt.Errorf("kernel traffic counters not found for interface %s", iface)
}

func loadVnstatTrafficTotals(iface string) (int64, int64, error) {
	up, down, err := queryVnstatTrafficTotals(iface)
	if err == nil {
		return up, down, nil
	}

	if ensureErr := ensureVnstatTracking(iface); ensureErr != nil {
		return 0, 0, fmt.Errorf("%w; ensure tracking failed: %v", err, ensureErr)
	}

	return queryVnstatTrafficTotals(iface)
}

func queryVnstatTrafficTotals(iface string) (int64, int64, error) {
	output, err := runVnstatCommand("-i", iface, "--json")
	if err != nil {
		return 0, 0, err
	}

	return parseVnstatTrafficTotals(output)
}

func ensureVnstatTracking(iface string) error {
	if iface == "" {
		return errors.New("default interface is empty")
	}

	if err := ensureVnstatAvailable(); err != nil {
		return err
	}

	if _, _, err := queryVnstatTrafficTotals(iface); err == nil {
		return nil
	}

	if _, err := runVnstatCommand("-i", iface, "--add"); err != nil {
		if !strings.Contains(strings.ToLower(err.Error()), "exist") {
			return err
		}
	}

	if err := restartVnstatDaemon(); err != nil {
		logger.Warning("vnstat daemon restart after interface update failed:", err)
	}

	_, _, err := queryVnstatTrafficTotals(iface)
	return err
}

func ensureVnstatAvailable() error {
	if !IsSystemPlatformLinux() {
		return nil
	}

	manifest, hasManifest := (&TrafficOverviewService{}).loadVnstatManifest()
	if hasManifest {
		if _, ok := managedVnstatBinaryPath(manifest); ok {
			return nil
		}
	}

	return errors.New("vnstat is not installed or is not managed by the panel")
}

func loadTrustedVnstatManifest() (trafficOverviewVnstatManifest, bool) {
	manifest, ok := (&TrafficOverviewService{}).loadVnstatManifest()
	if !ok || !isTrustedVnstatManifest(manifest) {
		return trafficOverviewVnstatManifest{}, false
	}
	if _, ok := managedVnstatBinaryPath(manifest); !ok {
		return trafficOverviewVnstatManifest{}, false
	}
	return manifest, true
}

type limitedOutputBuffer struct {
	buffer   bytes.Buffer
	limit    int
	overflow bool
}

func (b *limitedOutputBuffer) Write(data []byte) (int, error) {
	if b == nil {
		return len(data), nil
	}
	if b.limit <= 0 {
		b.limit = maxVnstatCommandOutputBytes
	}
	remaining := b.limit - b.buffer.Len()
	if remaining <= 0 {
		b.overflow = true
		return len(data), nil
	}
	if len(data) > remaining {
		_, _ = b.buffer.Write(data[:remaining])
		b.overflow = true
		return len(data), nil
	}
	_, _ = b.buffer.Write(data)
	return len(data), nil
}

func (b *limitedOutputBuffer) String() string {
	if b == nil {
		return ""
	}
	return b.buffer.String()
}

func runVnstatCommand(args ...string) (string, error) {
	manifest, ok := loadTrustedVnstatManifest()
	if !ok {
		return "", errors.New("vnstat is not installed or is not managed by the panel")
	}
	binaryPath, ok := managedVnstatBinaryPath(manifest)
	if !ok {
		return "", errors.New("managed vnstat binary is unavailable")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binaryPath, args...)
	output := &limitedOutputBuffer{limit: maxVnstatCommandOutputBytes}
	cmd.Stdout = output
	cmd.Stderr = output
	err := cmd.Run()
	if ctx.Err() == context.DeadlineExceeded {
		return "", ctx.Err()
	}
	if output.overflow {
		return "", errors.New("vnstat command output exceeds 1 MiB limit")
	}
	if err != nil {
		return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(output.String()))
	}
	return strings.TrimSpace(output.String()), nil
}

func detectVnstatPackageManagerPlan() *vnstatPackageManagerPlan {
	return selectVnstatPackageManagerPlan(detectLinuxSystemFamily(), func(name string) bool {
		_, err := exec.LookPath(name)
		return err == nil
	})
}

// selectVnstatPackageManagerPlan only considers package managers belonging to
// the platform family captured at panel startup. It must not infer a different
// distribution from whichever command happens to appear in PATH.
func selectVnstatPackageManagerPlan(systemFamily string, commandAvailable func(string) bool) *vnstatPackageManagerPlan {
	systemFamily = strings.ToLower(strings.TrimSpace(systemFamily))
	if systemFamily == "" || commandAvailable == nil {
		return nil
	}
	for _, candidate := range vnstatPackageManagerPlans() {
		if !strings.EqualFold(candidate.SystemFamily, systemFamily) {
			continue
		}
		if commandAvailable(candidate.Name) {
			plan := candidate
			return &plan
		}
	}
	return nil
}

func managerByName(name string) *vnstatPackageManagerPlan {
	normalized := strings.ToLower(strings.TrimSpace(name))
	if normalized == "" {
		return nil
	}
	for _, candidate := range vnstatPackageManagerPlans() {
		if candidate.Name == normalized {
			plan := candidate
			return &plan
		}
	}
	return nil
}

func vnstatPackageManagerPlans() []vnstatPackageManagerPlan {
	return []vnstatPackageManagerPlan{
		{
			Name:         "apt-get",
			SystemFamily: "debian",
			InstallPlan: [][]string{
				{"apt-get", "update"},
				{"apt-get", "install", "-y", vnstatPackageName},
			},
			BuildDepsPlan: [][]string{
				{"apt-get", "update"},
				{"apt-get", "install", "-y", "build-essential", "pkg-config", "libsqlite3-dev"},
			},
			RemovePlan:      [][]string{{"apt-get", "purge", "-y", vnstatPackageName}},
			FileListCommand: []string{"dpkg-query", "-L", vnstatPackageName},
		},
		{
			Name:         "dnf",
			SystemFamily: "rhel",
			InstallPlan:  [][]string{{"dnf", "install", "-y", vnstatPackageName}},
			BuildDepsPlan: [][]string{
				{"dnf", "install", "-y", "gcc", "make", "pkgconf-pkg-config", "sqlite-devel"},
			},
			RemovePlan:      [][]string{{"dnf", "remove", "-y", vnstatPackageName}},
			FileListCommand: []string{"rpm", "-ql", vnstatPackageName},
		},
		{
			Name:         "yum",
			SystemFamily: "rhel",
			InstallPlan:  [][]string{{"yum", "install", "-y", vnstatPackageName}},
			BuildDepsPlan: [][]string{
				{"yum", "install", "-y", "gcc", "make", "pkgconfig", "sqlite-devel"},
			},
			RemovePlan:      [][]string{{"yum", "remove", "-y", vnstatPackageName}},
			FileListCommand: []string{"rpm", "-ql", vnstatPackageName},
		},
		{
			Name:         "zypper",
			SystemFamily: "suse",
			InstallPlan:  [][]string{{"zypper", "--non-interactive", "install", vnstatPackageName}},
			BuildDepsPlan: [][]string{
				{"zypper", "--non-interactive", "install", "gcc", "make", "pkg-config", "sqlite3-devel"},
			},
			RemovePlan:      [][]string{{"zypper", "--non-interactive", "remove", vnstatPackageName}},
			FileListCommand: []string{"rpm", "-ql", vnstatPackageName},
		},
		{
			Name:         "pacman",
			SystemFamily: "arch",
			InstallPlan:  [][]string{{"pacman", "-Sy", "--noconfirm", vnstatPackageName}},
			BuildDepsPlan: [][]string{
				{"pacman", "-Sy", "--noconfirm", "base-devel", "pkgconf", "sqlite"},
			},
			RemovePlan:      [][]string{{"pacman", "-R", "--noconfirm", vnstatPackageName}},
			FileListCommand: []string{"pacman", "-Qlq", vnstatPackageName},
		},
		{
			Name:         "apk",
			SystemFamily: "alpine",
			InstallPlan:  [][]string{{"apk", "add", "--no-cache", vnstatPackageName}},
			BuildDepsPlan: [][]string{
				{"apk", "add", "--no-cache", "build-base", "pkgconf", "sqlite-dev"},
			},
			RemovePlan:      [][]string{{"apk", "del", vnstatPackageName}},
			FileListCommand: []string{"apk", "info", "-L", vnstatPackageName},
		},
	}
}

func runInstallCommand(command []string) error {
	return runInstallCommandContext(context.Background(), command)
}

func runInstallCommandContext(parent context.Context, command []string) error {
	if len(command) == 0 {
		return nil
	}

	output, err, timedOut := runManagedCommandOutputContext(parent, "", 2*time.Minute, command[0], command[1:]...)
	if timedOut {
		return fmt.Errorf("install command timed out: %s", strings.Join(command, " "))
	}
	if errors.Is(parent.Err(), context.Canceled) {
		return fmt.Errorf("install command canceled: %s: %w", strings.Join(command, " "), parent.Err())
	}
	if err != nil {
		return fmt.Errorf("install command failed (%s): %w: %s", strings.Join(command, " "), err, strings.TrimSpace(string(output)))
	}
	return nil
}

func runCommandOutput(command []string, timeout time.Duration) (string, error) {
	if len(command) == 0 {
		return "", nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, command[0], command[1:]...)
	output, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("command timed out: %s", strings.Join(command, " "))
	}
	if err != nil {
		return "", fmt.Errorf("command failed (%s): %w: %s", strings.Join(command, " "), err, strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output)), nil
}

func collectVnstatPackageFilesByManager(managerName string) []string {
	manager := managerByName(managerName)
	if manager == nil || len(manager.FileListCommand) == 0 {
		return nil
	}
	output, err := runCommandOutput(manager.FileListCommand, 20*time.Second)
	if err != nil {
		return nil
	}
	return parseVnstatPackageFileList(manager.Name, output)
}

func parseVnstatPackageFileList(managerName string, output string) []string {
	lines := strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n")
	paths := make([]string, 0, len(lines))
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, ".") || strings.Contains(line, ":") {
			continue
		}
		if managerName == "apk" && !strings.HasPrefix(line, "/") {
			line = "/" + line
		}
		if !filepath.IsAbs(line) {
			continue
		}
		paths = append(paths, filepath.Clean(line))
	}
	return normalizeAbsolutePathList(paths)
}

func detectLatestVnstatVersion(status VnstatPackageStatus, source string) (string, string, error) {
	method := normalizeRequestedVnstatSource(source)
	if method == "" {
		return "", "", errors.New("请先选择来源")
	}
	switch method {
	case vnstatInstallMethodGitHubRelease:
		version, err := fetchLatestVnstatGitHubVersion()
		return version, "github-release", err
	case vnstatInstallMethodSystemPackage:
		manager := managerByName(status.PackageManager)
		if manager == nil {
			manager = detectVnstatPackageManagerPlan()
		}
		if manager == nil {
			return "", "system-package", errors.New("未识别到可用的软件包管理器，无法检测 vnstat 更新")
		}
		version, err := detectLatestVnstatPackageVersion(manager)
		return version, manager.Name, err
	default:
		if manager := managerByName(status.PackageManager); manager != nil {
			version, err := detectLatestVnstatPackageVersion(manager)
			return version, manager.Name, err
		}
		version, err := fetchLatestVnstatGitHubVersion()
		return version, "github-release", err
	}
}

func fetchLatestVnstatGitHubVersion() (string, error) {
	release, err := fetchLatestVnstatRelease()
	if err != nil {
		return "", fmt.Errorf("获取 GitHub 最新 vnstat 版本失败: %w", err)
	}
	version := normalizeDetectedVnstatVersion(firstNonEmpty(release.TagName, release.Name))
	if version == "" {
		return "", errors.New("GitHub 最新 vnstat 版本信息无效")
	}
	return version, nil
}

func detectLatestVnstatPackageVersion(manager *vnstatPackageManagerPlan) (string, error) {
	if manager == nil {
		return "", errors.New("未识别到可用的软件包管理器，无法检测 vnstat 更新")
	}
	switch manager.Name {
	case "apt-get":
		return parseVnstatVersionFromCommand(
			[]string{"apt-cache", "policy", vnstatPackageName},
			8*time.Second,
			func(output string) string {
				for _, rawLine := range strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n") {
					line := strings.TrimSpace(rawLine)
					if !strings.HasPrefix(strings.ToLower(line), "candidate:") {
						continue
					}
					version := normalizeDetectedVnstatPackageVersion(strings.TrimSpace(strings.TrimPrefix(line, "Candidate:")))
					if version != "" && !strings.EqualFold(version, "(none)") {
						return version
					}
				}
				return ""
			},
			"apt 软件源中未找到可用的 vnstat 版本",
		)
	case "dnf", "yum":
		return parseVnstatVersionFromCommand(
			[]string{manager.Name, "info", vnstatPackageName},
			12*time.Second,
			extractLatestRpmInfoVersion,
			fmt.Sprintf("%s 软件源中未找到可用的 vnstat 版本", manager.Name),
		)
	case "zypper":
		return parseVnstatVersionFromCommand(
			[]string{"zypper", "info", vnstatPackageName},
			12*time.Second,
			func(output string) string {
				return extractLabeledVnstatVersion(output, "version")
			},
			"zypper 软件源中未找到可用的 vnstat 版本",
		)
	case "pacman":
		return parseVnstatVersionFromCommand(
			[]string{"pacman", "-Si", vnstatPackageName},
			8*time.Second,
			func(output string) string {
				return extractLabeledVnstatPackageVersion(output, "version")
			},
			"pacman 软件源中未找到可用的 vnstat 版本",
		)
	case "apk":
		return parseVnstatVersionFromCommand(
			[]string{"apk", "policy", vnstatPackageName},
			8*time.Second,
			extractFirstVnstatPackageVersionToken,
			"apk 软件源中未找到可用的 vnstat 版本",
		)
	default:
		return "", fmt.Errorf("当前包管理器 %s 暂不支持远端版本检测，可直接点击下载 / 重装尝试更新", manager.Name)
	}
}

func parseVnstatVersionFromCommand(command []string, timeout time.Duration, parser func(string) string, emptyErr string) (string, error) {
	output, err := runCommandOutput(command, timeout)
	if err != nil {
		return "", err
	}
	version := ""
	if parser != nil {
		version = strings.TrimSpace(parser(output))
	}
	if version == "" {
		return "", errors.New(emptyErr)
	}
	return version, nil
}

func detectInstalledVnstatPackageVersion(managerName string) string {
	manager := managerByName(managerName)
	if manager == nil {
		manager = detectVnstatPackageManagerPlan()
	}
	if manager == nil {
		return ""
	}
	switch manager.Name {
	case "apt-get":
		version, err := parseVnstatVersionFromCommand(
			[]string{"dpkg-query", "-W", "-f=${Version}", vnstatPackageName},
			8*time.Second,
			func(output string) string {
				return normalizeDetectedVnstatPackageVersion(output)
			},
			"",
		)
		if err == nil {
			return version
		}
	case "dnf", "yum", "zypper":
		version, err := parseVnstatVersionFromCommand(
			[]string{"rpm", "-q", "--queryformat", "%{VERSION}-%{RELEASE}", vnstatPackageName},
			8*time.Second,
			func(output string) string {
				return normalizeDetectedVnstatPackageVersion(output)
			},
			"",
		)
		if err == nil {
			return version
		}
	case "pacman":
		version, err := parseVnstatVersionFromCommand(
			[]string{"pacman", "-Q", vnstatPackageName},
			8*time.Second,
			extractInstalledPacmanVnstatVersion,
			"",
		)
		if err == nil {
			return version
		}
	case "apk":
		version, err := parseVnstatVersionFromCommand(
			[]string{"apk", "info", "-v", vnstatPackageName},
			8*time.Second,
			extractInstalledApkVnstatVersion,
			"",
		)
		if err == nil {
			return version
		}
	}
	return ""
}

func detectVnstatVersionAt(binaryPath string) string {
	binaryPath = normalizeVnstatPath(binaryPath)
	if !isSafeVnstatCommandPath(binaryPath) {
		return ""
	}
	if info, err := os.Lstat(binaryPath); err != nil || info.IsDir() {
		return ""
	}
	output, err := runCommandOutput([]string{binaryPath, "--version"}, 4*time.Second)
	if err != nil {
		return ""
	}
	return extractVnstatVersion(output)
}

func extractVnstatVersion(output string) string {
	fields := strings.Fields(strings.ReplaceAll(output, "\n", " "))
	for _, field := range fields {
		candidate := strings.Trim(field, " \t\r\n,;:()[]{}\"'")
		if candidate == "" {
			continue
		}
		if strings.HasPrefix(candidate, "v") || strings.HasPrefix(candidate, "V") {
			candidate = strings.TrimSpace(candidate[1:])
		}
		if looksLikeDottedVersion(candidate) {
			return candidate
		}
	}
	return ""
}

func normalizeDetectedVnstatVersion(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	if version := extractVnstatVersion(trimmed); version != "" {
		return version
	}
	return ""
}

func normalizeDetectedVnstatPackageVersion(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	if idx := strings.Index(trimmed, ":"); idx > 0 {
		prefix := strings.TrimSpace(trimmed[:idx])
		isEpoch := prefix != ""
		for _, r := range prefix {
			if r < '0' || r > '9' {
				isEpoch = false
				break
			}
		}
		if isEpoch {
			trimmed = strings.TrimSpace(trimmed[idx+1:])
		}
	}
	fields := strings.Fields(trimmed)
	for _, field := range fields {
		if normalized := normalizeDetectedVnstatPackageVersionToken(field); normalized != "" {
			return normalized
		}
	}
	return ""
}

func normalizeDetectedVnstatPackageVersionToken(value string) string {
	token := strings.Trim(value, " \t\r\n,;:()[]{}\"'")
	if token == "" {
		return ""
	}
	lowerToken := strings.ToLower(token)
	prefix := strings.ToLower(vnstatPackageName) + "-"
	if strings.HasPrefix(lowerToken, prefix) {
		token = strings.TrimSpace(token[len(prefix):])
	}
	if version := extractVnstatVersion(token); version != "" {
		return version
	}
	return ""
}

func extractLabeledVnstatVersion(output string, label string) string {
	target := strings.ToLower(strings.TrimSpace(label))
	version := ""
	for _, rawLine := range strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		if strings.ToLower(strings.TrimSpace(key)) != target {
			continue
		}
		if normalized := normalizeDetectedVnstatVersion(value); normalized != "" {
			version = normalized
		}
	}
	return version
}

func extractLabeledVnstatPackageVersion(output string, label string) string {
	target := strings.ToLower(strings.TrimSpace(label))
	version := ""
	for _, rawLine := range strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		if strings.ToLower(strings.TrimSpace(key)) != target {
			continue
		}
		if normalized := normalizeDetectedVnstatPackageVersion(value); normalized != "" {
			version = normalized
		}
	}
	return version
}

func extractLatestRpmInfoVersion(output string) string {
	version := ""
	for _, rawLine := range strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "version":
			if normalized := normalizeDetectedVnstatPackageVersion(value); normalized != "" {
				version = normalized
			}
		}
	}
	return version
}

func extractFirstVnstatVersionToken(output string) string {
	return normalizeDetectedVnstatVersion(output)
}

func extractFirstVnstatPackageVersionToken(output string) string {
	return normalizeDetectedVnstatPackageVersion(output)
}

func extractInstalledPacmanVnstatVersion(output string) string {
	fields := strings.Fields(strings.TrimSpace(output))
	if len(fields) >= 2 {
		if normalized := normalizeDetectedVnstatPackageVersion(fields[1]); normalized != "" {
			return normalized
		}
	}
	return normalizeDetectedVnstatPackageVersion(output)
}

func extractInstalledApkVnstatVersion(output string) string {
	lines := strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n")
	for _, rawLine := range lines {
		if normalized := normalizeDetectedVnstatPackageVersion(strings.TrimSpace(rawLine)); normalized != "" {
			return normalized
		}
	}
	return ""
}

func looksLikeDottedVersion(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" || !strings.Contains(value, ".") {
		return false
	}
	parts := strings.Split(value, ".")
	if len(parts) < 2 {
		return false
	}
	for _, part := range parts {
		if part == "" {
			return false
		}
		digitSeen := false
		for _, r := range part {
			switch {
			case r >= '0' && r <= '9':
				digitSeen = true
			case r == '-' || r == '+' || r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z'):
			default:
				return false
			}
		}
		if !digitSeen {
			return false
		}
	}
	return true
}

func isVnstatDaemonRunning() bool {
	manifest, ok := loadTrustedVnstatManifest()
	return ok && isVnstatDaemonRunningForManifest(manifest)
}

func isVnstatDaemonRunningForManifest(manifest trafficOverviewVnstatManifest) bool {
	if !IsSystemPlatformLinux() || !isTrustedVnstatManifest(manifest) {
		return false
	}
	if systemctlPath, err := exec.LookPath("systemctl"); err == nil {
		for _, service := range verifiedVnstatServices(manifest, systemctlPath) {
			if service.Systemd && runCommandWithTimeout(shortSystemCommandTimeout, systemctlPath, "is-active", "--quiet", service.Name) == nil {
				return true
			}
		}
	}
	return isManagedVnstatDaemonProcessRunning(manifest)
}

// isVnstatDaemonRunningForStatus avoids /proc so the traffic-page polling
// endpoint never performs process discovery. Runtime start/stop paths still
// use the exact PID check when they need to prove daemon termination.
func isVnstatDaemonRunningForStatus(manifest trafficOverviewVnstatManifest) bool {
	if !IsSystemPlatformLinux() || !isTrustedVnstatManifest(manifest) {
		return false
	}
	systemctlPath, err := exec.LookPath("systemctl")
	if err != nil {
		return false
	}
	for _, service := range verifiedVnstatServices(manifest, systemctlPath) {
		if service.Systemd && runCommandWithTimeout(shortSystemCommandTimeout, systemctlPath, "is-active", "--quiet", service.Name) == nil {
			return true
		}
	}
	return false
}

func isManagedVnstatDaemonProcessRunning(manifest trafficOverviewVnstatManifest) bool {
	return len(managedVnstatDaemonPIDs(manifest)) > 0
}

func stopVnstatDaemon() error {
	manifest, ok := loadTrustedVnstatManifest()
	if !ok {
		return nil
	}
	return stopVnstatDaemonForManifest(manifest)
}

// stopVnstatDaemonForManifest stops and disables only service registrations
// that are verified to execute the exact managed daemon binary. It returns an
// error until every matching process has exited; callers must never delete
// files after a failed stop.
func stopVnstatDaemonForManifest(manifest trafficOverviewVnstatManifest) error {
	if !IsSystemPlatformLinux() || !isTrustedVnstatManifest(manifest) {
		return errors.New("managed vnstat daemon is unavailable")
	}
	return stopVnstatDaemonWithOptions(manifest, true)
}

// stopVnstatDaemonForUninstall uses runtime.GOOS through vnstatRuntimeGOOS
// rather than the database platform snapshot. A deleted or stale database
// must never let a panel-owned daemon survive the panel uninstall.
func stopVnstatDaemonForUninstall(manifest trafficOverviewVnstatManifest) error {
	if vnstatRuntimeGOOS() != "linux" || !isTrustedVnstatManifest(manifest) {
		return errors.New("managed vnstat daemon is unavailable")
	}
	return stopVnstatDaemonWithOptions(manifest, true)
}

func stopVnstatDaemonForRestart(manifest trafficOverviewVnstatManifest) error {
	if !IsSystemPlatformLinux() || !isTrustedVnstatManifest(manifest) {
		return errors.New("managed vnstat daemon is unavailable")
	}
	return stopVnstatDaemonWithOptions(manifest, false)
}

func stopVnstatDaemonWithOptions(manifest trafficOverviewVnstatManifest, disableAutostart bool) error {
	pids := managedVnstatDaemonPIDs(manifest)
	systemctlPath := ""
	if path, err := exec.LookPath("systemctl"); err == nil {
		systemctlPath = path
	}
	verified := verifiedVnstatServices(manifest, systemctlPath)
	if systemctlPath != "" {
		for _, service := range verified {
			if !service.Systemd {
				continue
			}
			if err := runCommandWithTimeout(systemCommandTimeout, systemctlPath, "stop", service.Name); err != nil {
				return fmt.Errorf("stop managed vnstat systemd unit %s failed: %w", service.Name, err)
			}
			if disableAutostart {
				if err := runCommandWithTimeout(systemCommandTimeout, systemctlPath, "disable", service.Name); err != nil {
					return fmt.Errorf("disable managed vnstat systemd unit %s failed: %w", service.Name, err)
				}
			}
		}
		if len(verified) > 0 {
			if err := runCommandWithTimeout(systemCommandTimeout, systemctlPath, "daemon-reload"); err != nil {
				return fmt.Errorf("reload systemd after stopping managed vnstat failed: %w", err)
			}
			_ = runCommandWithTimeout(systemCommandTimeout, systemctlPath, "reset-failed")
		}
	}
	servicePath := ""
	if path, err := exec.LookPath("service"); err == nil {
		servicePath = path
	}
	for _, service := range verified {
		if service.Systemd {
			continue
		}
		if servicePath != "" {
			if err := runCommandWithTimeout(systemCommandTimeout, servicePath, service.Name, "stop"); err != nil {
				return fmt.Errorf("stop managed vnstat service %s failed: %w", service.Name, err)
			}
		} else if err := runCommandWithTimeout(systemCommandTimeout, filepath.Join("/etc/init.d", service.Name), "stop"); err != nil {
			return fmt.Errorf("stop managed vnstat service %s failed: %w", service.Name, err)
		}
		if disableAutostart {
			if err := disableVerifiedSysVService(service.Name); err != nil {
				return fmt.Errorf("disable managed vnstat SysV autostart %s failed: %w", service.Name, err)
			}
		}
	}
	if err := terminateVnstatPIDs(pids); err != nil {
		return err
	}
	if remaining := managedVnstatDaemonPIDs(manifest); len(remaining) > 0 {
		return fmt.Errorf("managed vnstat daemon is still running: %v", remaining)
	}
	return nil
}

func ensureVnstatDaemonRunning() error {
	if !IsSystemPlatformLinux() {
		return nil
	}
	manifest, ok := loadTrustedVnstatManifest()
	if !ok {
		return nil
	}
	if isVnstatDaemonRunningForManifest(manifest) {
		clearVnstatRuntimeConflict()
		return nil
	}
	if err := restartVnstatDaemonForManifest(manifest); err != nil {
		recordVnstatRuntimeConflict(vnstatDiscoverRunningExternalFn(manifest, true))
		return err
	}
	clearVnstatRuntimeConflict()
	return nil
}

func restartVnstatDaemon() error {
	manifest, ok := loadTrustedVnstatManifest()
	if !ok {
		return nil
	}
	return restartVnstatDaemonForManifest(manifest)
}

func restartVnstatDaemonForManifest(manifest trafficOverviewVnstatManifest) error {
	if !IsSystemPlatformLinux() || !isTrustedVnstatManifest(manifest) {
		return errors.New("managed vnstat daemon is unavailable")
	}
	systemctlPath := ""
	if path, err := exec.LookPath("systemctl"); err == nil {
		systemctlPath = path
	}
	verified := verifiedVnstatServices(manifest, systemctlPath)
	if systemctlPath != "" {
		for _, service := range verified {
			if service.Systemd && runCommandWithTimeout(systemCommandTimeout, systemctlPath, "restart", service.Name) == nil && waitForManagedVnstatDaemon(manifest) {
				return nil
			}
		}
	}
	if servicePath, err := exec.LookPath("service"); err == nil {
		for _, service := range verified {
			if !service.Systemd && runCommandWithTimeout(systemCommandTimeout, servicePath, service.Name, "restart") == nil && waitForManagedVnstatDaemon(manifest) {
				return nil
			}
		}
	}
	if daemonPath, ok := managedVnstatDaemonPath(manifest); ok {
		if err := stopVnstatDaemonForRestart(manifest); err != nil {
			return err
		}
		if err := runCommandWithTimeout(systemCommandTimeout, daemonPath, "--daemon"); err == nil && waitForManagedVnstatDaemon(manifest) {
			return nil
		}
	}
	return errors.New("vnstat daemon restart failed")
}

func waitForManagedVnstatDaemon(manifest trafficOverviewVnstatManifest) bool {
	deadline := time.Now().Add(5 * time.Second)
	for {
		if isVnstatDaemonRunningForManifest(manifest) {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func managedVnstatServiceUnits(manifest trafficOverviewVnstatManifest) []string {
	seen := make(map[string]struct{})
	units := make([]string, 0, 3)
	for _, raw := range manifest.ServiceUnits {
		unit := strings.ToLower(strings.TrimSpace(raw))
		if unit != "vnstat" && unit != "vnstatd" && unit != vnstatPanelSystemdUnit {
			continue
		}
		if _, exists := seen[unit]; exists {
			continue
		}
		seen[unit] = struct{}{}
		units = append(units, unit)
	}
	sort.Strings(units)
	return units
}

func verifiedVnstatServices(manifest trafficOverviewVnstatManifest, systemctlPath string) []vnstatVerifiedService {
	daemonPath, ok := managedVnstatDaemonPath(manifest)
	if !ok {
		return nil
	}
	services := make([]vnstatVerifiedService, 0, len(managedVnstatServiceUnits(manifest)))
	for _, unit := range managedVnstatServiceUnits(manifest) {
		if systemctlPath != "" && systemdUnitExecutesPath(systemctlPath, unit, daemonPath) {
			services = append(services, vnstatVerifiedService{Name: unit, Systemd: true})
			continue
		}
		if initScriptExecutesPath(unit, daemonPath) {
			services = append(services, vnstatVerifiedService{Name: unit, Systemd: false})
		}
	}
	return services
}

func systemdUnitExecutesPath(systemctlPath string, unit string, daemonPath string) bool {
	if strings.TrimSpace(systemctlPath) == "" || strings.TrimSpace(unit) == "" || strings.TrimSpace(daemonPath) == "" {
		return false
	}
	output, err := runCommandOutput([]string{systemctlPath, "show", "--property=ExecStart", "--value", unit}, shortSystemCommandTimeout)
	if err != nil {
		return false
	}
	for _, candidate := range parseSystemdExecStartPaths(output) {
		if normalizeVnstatPath(candidate) == normalizeVnstatPath(daemonPath) {
			return true
		}
	}
	return false
}

func parseSystemdExecStartPaths(output string) []string {
	parts := strings.Split(output, "path=")
	paths := make([]string, 0, len(parts))
	for index, part := range parts {
		if index == 0 {
			continue
		}
		candidate := strings.TrimLeft(part, " \t\"")
		if end := strings.IndexAny(candidate, " \t;}"); end >= 0 {
			candidate = candidate[:end]
		}
		candidate = normalizeVnstatPath(candidate)
		if candidate != "" {
			paths = append(paths, candidate)
		}
	}
	return normalizeAbsolutePathList(paths)
}

func initScriptExecutesPath(unit string, daemonPath string) bool {
	if unit != "vnstat" && unit != "vnstatd" {
		return false
	}
	path := filepath.Join("/etc/init.d", unit)
	info, err := os.Lstat(path)
	if err != nil || info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return false
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return strings.Contains(string(content), normalizeVnstatPath(daemonPath))
}

// disableVerifiedSysVService is intentionally restricted to the two standard
// vnStat init-script names. It never edits rc.local, cron, shell profiles, or
// any unknown startup mechanism.
func disableVerifiedSysVService(unit string) error {
	if unit != "vnstat" && unit != "vnstatd" {
		return errors.New("refusing to disable an unknown SysV service")
	}
	if updateRCD, err := exec.LookPath("update-rc.d"); err == nil {
		return runCommandWithTimeout(systemCommandTimeout, updateRCD, unit, "disable")
	}
	if chkconfig, err := exec.LookPath("chkconfig"); err == nil {
		return runCommandWithTimeout(systemCommandTimeout, chkconfig, unit, "off")
	}
	if rcUpdate, err := exec.LookPath("rc-update"); err == nil {
		return runCommandWithTimeout(systemCommandTimeout, rcUpdate, "del", unit, "default")
	}
	return errors.New("no supported SysV autostart controller is available")
}

func terminateVnstatPIDs(pids []int) error {
	if len(pids) == 0 {
		return nil
	}
	killPath, err := exec.LookPath("kill")
	if err != nil {
		return errors.New("cannot stop vnstat daemon because kill command is unavailable")
	}
	for _, pid := range pids {
		if _, statErr := os.Stat(filepath.Join("/proc", strconv.Itoa(pid))); os.IsNotExist(statErr) {
			continue
		}
		if err := runCommandWithTimeout(shortSystemCommandTimeout, killPath, "-TERM", strconv.Itoa(pid)); err != nil {
			if _, statErr := os.Stat(filepath.Join("/proc", strconv.Itoa(pid))); os.IsNotExist(statErr) {
				continue
			}
			return fmt.Errorf("stop vnstat pid %d failed: %w", pid, err)
		}
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		remaining := runningVnstatPIDs(pids)
		if len(remaining) == 0 {
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("vnstat daemon did not stop: %v", runningVnstatPIDs(pids))
}

func runningVnstatPIDs(pids []int) []int {
	remaining := make([]int, 0, len(pids))
	for _, pid := range pids {
		if _, err := os.Stat(filepath.Join("/proc", strconv.Itoa(pid))); err == nil {
			remaining = append(remaining, pid)
		}
	}
	return remaining
}

func managedVnstatDaemonPath(manifest trafficOverviewVnstatManifest) (string, bool) {
	for _, path := range manifest.FilePaths {
		if normalizeVnstatPath(path) != normalizeVnstatPath("/usr/sbin/vnstatd") {
			continue
		}
		// Keep the inventory path even when its file was already removed. A
		// daemon started from that file can remain alive as "(deleted)" and
		// must still be stopped before data or residual files are cleaned.
		return normalizeVnstatPath(path), true
	}
	return "", false
}

func managedVnstatDaemonPIDs(manifest trafficOverviewVnstatManifest) []int {
	daemonPath, ok := managedVnstatDaemonPath(manifest)
	if !ok {
		return nil
	}
	return vnstatDaemonPIDsAtPath(daemonPath)
}

func vnstatDaemonPIDsAtPath(daemonPath string) []int {
	daemonPath = normalizeVnstatPath(daemonPath)
	if daemonPath == "" || !filepath.IsAbs(daemonPath) {
		return nil
	}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	pids := make([]int, 0)
	for _, entry := range entries {
		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid <= 0 {
			continue
		}
		exePath, err := os.Readlink(filepath.Join("/proc", entry.Name(), "exe"))
		if err != nil || normalizeProcExecutablePath(exePath) != daemonPath {
			continue
		}
		pids = append(pids, pid)
	}
	sort.Ints(pids)
	return pids
}

func normalizeProcExecutablePath(path string) string {
	return normalizeVnstatPath(strings.TrimSuffix(strings.TrimSpace(path), " (deleted)"))
}

// discoverRunningExternalVnstatInstallations observes only currently running
// vnstatd processes. It deliberately does not inspect standard paths, PATH,
// stopped units, cron, rc.local, or startup scripts. A systemd unit is exposed
// only when the process belongs to that unit and its ExecStart is verified to
// execute the exact discovered daemon path.
func discoverRunningExternalVnstatInstallations(manifest trafficOverviewVnstatManifest, managed bool) []vnstatExternalInstallation {
	if !IsSystemPlatformLinux() {
		return nil
	}
	managedDaemon := ""
	if managed {
		if daemonPath, ok := managedVnstatDaemonPath(manifest); ok {
			managedDaemon = daemonPath
		}
	}

	installations := make(map[string]*vnstatExternalInstallation)
	processUnits := make(map[string]map[string]struct{})
	addDaemon := func(path string, pid int) {
		path = normalizeVnstatPath(path)
		if path == "" || !filepath.IsAbs(path) || filepath.Base(path) != "vnstatd" || path == managedDaemon {
			return
		}
		key := externalVnstatInstallationKey(path)
		installation, ok := installations[key]
		if !ok {
			installation = &vnstatExternalInstallation{DaemonPath: path}
			installations[key] = installation
		}
		if pid > 0 {
			installation.PIDs = append(installation.PIDs, pid)
			if unit := systemdServiceUnitForPID(pid); unit != "" {
				if processUnits[key] == nil {
					processUnits[key] = make(map[string]struct{})
				}
				processUnits[key][unit] = struct{}{}
			}
		}
	}

	if entries, err := os.ReadDir("/proc"); err == nil {
		for _, entry := range entries {
			pid, parseErr := strconv.Atoi(entry.Name())
			if parseErr != nil || pid <= 0 {
				continue
			}
			exePath, readErr := os.Readlink(filepath.Join("/proc", entry.Name(), "exe"))
			path := normalizeProcExecutablePath(exePath)
			if readErr == nil && filepath.Base(path) == "vnstatd" {
				addDaemon(path, pid)
			}
		}
	}

	systemctlPath := ""
	if path, err := exec.LookPath("systemctl"); err == nil {
		systemctlPath = path
	}
	if systemctlPath != "" {
		for key, installation := range installations {
			for unit := range processUnits[key] {
				if systemdUnitExecutesPath(systemctlPath, unit, installation.DaemonPath) {
					installation.ServiceUnits = append(installation.ServiceUnits, unit)
				}
			}
		}
	}

	result := make([]vnstatExternalInstallation, 0, len(installations))
	for _, installation := range installations {
		installation.PIDs = uniqueSortedIntSlice(installation.PIDs)
		installation.ServiceUnits = normalizeVnstatServiceUnitList(installation.ServiceUnits)
		result = append(result, *installation)
	}
	sort.Slice(result, func(i, j int) bool {
		return firstNonEmpty(result[i].DaemonPath, result[i].BinaryPath) < firstNonEmpty(result[j].DaemonPath, result[j].BinaryPath)
	})
	return result
}

func externalVnstatInstallationKey(path string) string {
	path = normalizeVnstatPath(path)
	if isPanelDefaultVnstatRuntimePath(path) {
		return "panel-default"
	}
	return "directory:" + normalizeVnstatPath(filepath.Dir(path))
}

func isPanelDefaultVnstatRuntimePath(path string) bool {
	path = normalizeVnstatPath(path)
	return path == normalizeVnstatPath("/usr/bin/vnstat") || path == normalizeVnstatPath("/usr/sbin/vnstatd")
}

func systemdServiceUnitForPID(pid int) string {
	if pid <= 0 {
		return ""
	}
	content, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "cgroup"))
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(strings.ReplaceAll(string(content), "\r\n", "\n"), "\n") {
		parts := strings.SplitN(strings.TrimSpace(line), ":", 3)
		if len(parts) != 3 {
			continue
		}
		unit := filepath.Base(strings.TrimSpace(parts[2]))
		if strings.HasSuffix(unit, ".service") && isSafeSystemdUnitName(unit) {
			return unit
		}
	}
	return ""
}

func isSafeSystemdUnitName(unit string) bool {
	unit = strings.TrimSpace(unit)
	if unit == "" || strings.ContainsAny(unit, `/\\`) {
		return false
	}
	for _, r := range unit {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' || r == '@' {
			continue
		}
		return false
	}
	return true
}

func normalizeVnstatServiceUnitList(units []string) []string {
	seen := make(map[string]struct{})
	result := make([]string, 0, len(units))
	for _, raw := range units {
		unit := strings.TrimSpace(raw)
		if !isSafeSystemdUnitName(unit) {
			continue
		}
		if _, ok := seen[unit]; ok {
			continue
		}
		seen[unit] = struct{}{}
		result = append(result, unit)
	}
	sort.Strings(result)
	return result
}

func uniqueSortedIntSlice(values []int) []int {
	seen := make(map[int]struct{})
	result := make([]int, 0, len(values))
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Ints(result)
	return result
}

func externalPaths(installations []vnstatExternalInstallation) []string {
	paths := make([]string, 0, len(installations)*2)
	for _, installation := range installations {
		paths = append(paths, installation.BinaryPath, installation.DaemonPath)
	}
	return normalizeAbsolutePathList(paths)
}

func externalUnits(installations []vnstatExternalInstallation) []string {
	units := make([]string, 0, len(installations))
	for _, installation := range installations {
		units = append(units, installation.ServiceUnits...)
	}
	return normalizeVnstatServiceUnitList(units)
}

// stopRunningExternalVnstatForPanelInstall intentionally operates only on a daemon
// process or a registered systemd/SysV service that has been checked against
// its exact executable path. It never deletes any external-path file and does
// not inspect cron, rc.local, shell configuration, or other opaque startup.
func stopRunningExternalVnstatForPanelInstall(installations []vnstatExternalInstallation) error {
	if len(installations) == 0 {
		return nil
	}
	systemctlPath := ""
	if path, err := exec.LookPath("systemctl"); err == nil {
		systemctlPath = path
	}
	servicePath := ""
	if path, err := exec.LookPath("service"); err == nil {
		servicePath = path
	}

	verifiedSystemd := make(map[string]string)
	verifiedSysV := make(map[string]string)
	pids := make([]int, 0)
	for _, installation := range installations {
		pids = append(pids, installation.PIDs...)
		daemonPath := normalizeVnstatPath(installation.DaemonPath)
		if daemonPath == "" {
			continue
		}
		for _, unit := range normalizeVnstatServiceUnitList(append(append([]string{}, installation.ServiceUnits...), "vnstat", "vnstatd")) {
			if systemctlPath != "" && systemdUnitExecutesPath(systemctlPath, unit, daemonPath) {
				verifiedSystemd[unit] = daemonPath
				continue
			}
			if (unit == "vnstat" || unit == "vnstatd") && initScriptExecutesPath(unit, daemonPath) {
				verifiedSysV[unit] = daemonPath
			}
		}
	}

	for unit := range verifiedSystemd {
		if err := runCommandWithTimeout(systemCommandTimeout, systemctlPath, "stop", unit); err != nil {
			return fmt.Errorf("stop external vnstat systemd unit %s failed: %w", unit, err)
		}
		if err := runCommandWithTimeout(systemCommandTimeout, systemctlPath, "disable", unit); err != nil {
			return fmt.Errorf("disable external vnstat systemd unit %s failed: %w", unit, err)
		}
	}
	if len(verifiedSystemd) > 0 {
		if err := runCommandWithTimeout(systemCommandTimeout, systemctlPath, "daemon-reload"); err != nil {
			return fmt.Errorf("reload systemd after stopping external vnstat failed: %w", err)
		}
		_ = runCommandWithTimeout(systemCommandTimeout, systemctlPath, "reset-failed")
	}
	for unit := range verifiedSysV {
		if servicePath != "" {
			if err := runCommandWithTimeout(systemCommandTimeout, servicePath, unit, "stop"); err != nil {
				return fmt.Errorf("stop external vnstat SysV service %s failed: %w", unit, err)
			}
		} else if err := runCommandWithTimeout(systemCommandTimeout, filepath.Join("/etc/init.d", unit), "stop"); err != nil {
			return fmt.Errorf("stop external vnstat SysV service %s failed: %w", unit, err)
		}
		if err := disableVerifiedSysVService(unit); err != nil {
			return fmt.Errorf("disable external vnstat SysV autostart %s failed: %w", unit, err)
		}
	}

	pids = uniqueSortedIntSlice(pids)
	if err := terminateVnstatPIDs(pids); err != nil {
		return err
	}
	for _, installation := range installations {
		if daemonPath := normalizeVnstatPath(installation.DaemonPath); daemonPath != "" {
			if remaining := vnstatDaemonPIDsAtPath(daemonPath); len(remaining) > 0 {
				return fmt.Errorf("external vnstat daemon is still running at %s: %v", daemonPath, remaining)
			}
		}
	}
	return nil
}

func (s *TrafficOverviewService) isOverviewEnabled() (bool, error) {
	return (&SettingService{}).getBool(trafficOverviewEnabledKey)
}

func (s *TrafficOverviewService) hasManagedVnstatManifest() bool {
	manifest, ok := s.loadVnstatManifest()
	return ok && isTrustedVnstatManifest(manifest)
}

func (s *TrafficOverviewService) loadVnstatManifest() (trafficOverviewVnstatManifest, bool) {
	raw, err := (&SettingService{}).getString(trafficOverviewVnstatManifestKey)
	if err != nil {
		return trafficOverviewVnstatManifest{}, false
	}
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "{}" {
		return trafficOverviewVnstatManifest{}, false
	}
	manifest := trafficOverviewVnstatManifest{}
	if err := json.Unmarshal([]byte(trimmed), &manifest); err != nil {
		return trafficOverviewVnstatManifest{}, false
	}
	manifest.Ownership = normalizeVnstatOwnership(manifest.Ownership, manifest.Managed)
	manifest.PackageManager = strings.TrimSpace(strings.ToLower(manifest.PackageManager))
	manifest.SystemFamily = strings.TrimSpace(strings.ToLower(manifest.SystemFamily))
	manifest.InstallMethod = normalizeVnstatInstallMethod(manifest.InstallMethod, manifest.PackageManager)
	manifest.PackageName = firstNonEmpty(manifest.PackageName, vnstatPackageName)
	manifest.BinaryPath = normalizeVnstatPath(manifest.BinaryPath)
	manifest.Version = strings.TrimSpace(manifest.Version)
	manifest.EvidenceNonce = strings.TrimSpace(manifest.EvidenceNonce)
	manifest.FilePaths = safeVnstatFilePaths(manifest.FilePaths)
	manifest.DataPaths = safeVnstatDataPaths(manifest.DataPaths)
	manifest.ServiceUnits = managedVnstatServiceUnits(manifest)
	return manifest, manifest.Managed || manifest.BinaryPath != "" || len(manifest.FilePaths) > 0
}

func (s *TrafficOverviewService) saveVnstatManifest(manifest trafficOverviewVnstatManifest) error {
	manifest.Managed = true
	manifest.Ownership = normalizeVnstatOwnership(manifest.Ownership, true)
	if manifest.Ownership == "" {
		return errors.New("vnstat ownership is required")
	}
	manifest.PackageManager = strings.TrimSpace(strings.ToLower(manifest.PackageManager))
	manifest.SystemFamily = strings.TrimSpace(strings.ToLower(manifest.SystemFamily))
	manifest.InstallMethod = normalizeVnstatInstallMethod(manifest.InstallMethod, manifest.PackageManager)
	manifest.PackageName = firstNonEmpty(manifest.PackageName, vnstatPackageName)
	manifest.BinaryPath = normalizeVnstatPath(manifest.BinaryPath)
	if !isSafeVnstatCommandPath(manifest.BinaryPath) {
		return errors.New("refusing to save an unsafe vnstat binary path")
	}
	manifest.Version = strings.TrimSpace(manifest.Version)
	manifest.FilePaths = safeVnstatFilePaths(manifest.FilePaths)
	manifest.DataPaths = safeVnstatDataPaths(manifest.DataPaths)
	manifest.ServiceUnits = managedVnstatServiceUnits(manifest)
	if !isVnstatManifestBaselineSafe(manifest) {
		return errors.New("refusing to save an unsafe vnstat manifest")
	}
	evidence, err := buildVnstatOwnershipEvidence(manifest)
	if err != nil {
		return err
	}
	manifest.EvidenceNonce = evidence.Nonce
	manifest.EvidenceSchema = evidence.Schema
	raw, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	evidencePath := strings.TrimSpace(vnstatEvidencePathFn())
	previousEvidence, previousErr := os.ReadFile(evidencePath)
	if err := writeVnstatOwnershipEvidence(evidence); err != nil {
		return fmt.Errorf("write vnstat ownership evidence failed: %w", err)
	}
	if err := (&SettingService{}).setString(trafficOverviewVnstatManifestKey, string(raw)); err != nil {
		if previousErr == nil {
			_ = os.WriteFile(evidencePath, previousEvidence, 0o600)
		} else {
			_ = removeVnstatOwnershipEvidence()
		}
		return err
	}
	invalidateVnstatStatusCache()
	return nil
}

func (s *TrafficOverviewService) clearVnstatManagedState() error {
	settingSvc := &SettingService{}
	if err := settingSvc.setString(trafficOverviewVnstatManifestKey, "{}"); err != nil {
		return err
	}
	if err := settingSvc.setString(trafficOverviewStateKey, "{}"); err != nil {
		return err
	}
	if err := settingSvc.setString(trafficOverviewSnapshotKey, "{}"); err != nil {
		return err
	}
	if err := settingSvc.setString(trafficOverviewCapStateKey, "{}"); err != nil {
		return err
	}
	if err := settingSvc.setString(trafficOverviewPauseStateKey, "{}"); err != nil {
		return err
	}
	trafficOverviewSnapshotMu.Lock()
	trafficOverviewSnapshotCache = trafficOverviewSnapshotState{}
	trafficOverviewSnapshotMu.Unlock()
	trafficOverviewRuntimeStateCacheMu.Lock()
	trafficOverviewRuntimeStateCache = trafficOverviewRuntimeStateCacheState{}
	trafficOverviewRuntimeStateCacheMu.Unlock()
	invalidateVnstatStatusCache()
	invalidateTrafficOverviewConfigCache()
	markTrafficOverviewCapReconcileNeeded()
	if err := removeVnstatOwnershipEvidence(); err != nil {
		return err
	}
	return nil
}

func removeVnstatTrackedData(manifest trafficOverviewVnstatManifest) error {
	targets := defaultVnstatDataPaths()
	targets = append(targets, manifest.DataPaths...)
	for _, path := range safeVnstatDataPaths(targets) {
		if err := removeExactVnstatDataPath(path); err != nil {
			return fmt.Errorf("remove vnstat data path failed %s: %w", path, err)
		}
	}

	for _, path := range safeVnstatFilePaths(manifest.FilePaths) {
		if isSafeVnstatDataPath(path) {
			continue
		}
		if err := removeExactVnstatResidualFile(path); err != nil {
			return err
		}
	}
	return nil
}

func removeExactVnstatDataPath(path string) error {
	if !isSafeVnstatDataPath(path) {
		return errors.New("refusing to remove an unsafe vnstat data path")
	}
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	// Never follow a link while removing a directory. Removing the link itself
	// is safe; recursively removing its target is not.
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return os.Remove(path)
	}
	return os.RemoveAll(path)
}

func removeExactVnstatResidualFile(path string) error {
	if !isSafeVnstatResidualPath(path) || isSafeVnstatDataPath(path) {
		return errors.New("refusing to remove an unsafe vnstat file")
	}
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	// Residual paths are always individual files or links. A directory at one
	// of these paths is deliberately left untouched rather than recursively
	// deleting a parent or an unexpected replacement directory.
	if info.IsDir() {
		return nil
	}
	return os.Remove(path)
}

func defaultVnstatDataPaths() []string {
	return []string{
		"/var/lib/vnstat",
		"/var/log/vnstat",
		"/var/cache/vnstat",
	}
}

func safeVnstatDataPaths(paths []string) []string {
	filtered := make([]string, 0, len(paths))
	for _, path := range normalizeAbsolutePathList(paths) {
		if isSafeVnstatDataPath(path) {
			filtered = append(filtered, path)
		}
	}
	return filtered
}

func safeVnstatFilePaths(paths []string) []string {
	filtered := make([]string, 0, len(paths))
	for _, path := range normalizeAbsolutePathList(paths) {
		if isSafeVnstatResidualPath(path) {
			filtered = append(filtered, path)
		}
	}
	return filtered
}

func vnstatManagementSupport() (bool, string) {
	return vnstatManagementSupportForRuntime(GetSystemPlatformOS(), runningInsideContainer())
}

func vnstatManagementSupportForRuntime(goos string, insideContainer bool) (bool, string) {
	if goos != "linux" {
		return false, "vnstat is supported on linux only"
	}
	if insideContainer {
		return false, "Docker/容器部署不支持在面板内安装或卸载 vnstat。请在镜像中预装 vnstat，并自行持久化 /var/lib/vnstat 等数据目录。"
	}
	return true, ""
}

func isSafeVnstatDataPath(path string) bool {
	cleaned := normalizeVnstatPath(path)
	for _, allowed := range defaultVnstatDataPaths() {
		if cleaned == normalizeVnstatPath(allowed) {
			return true
		}
	}
	return false
}

func isSafeVnstatResidualPath(path string) bool {
	cleaned := normalizeVnstatPath(path)
	if isSafeVnstatDataPath(cleaned) {
		return true
	}
	for _, allowed := range append(append([]string{}, vnstatStandardProgramPaths...), vnstatStandardConfigAndUnitPaths...) {
		if cleaned == normalizeVnstatPath(allowed) {
			return true
		}
	}
	for _, allowed := range vnstatStandardManPagePaths {
		if cleaned == normalizeVnstatPath(allowed) {
			return true
		}
	}
	return false
}

func isSafeVnstatProgramPath(path string) bool {
	cleaned := normalizeVnstatPath(path)
	for _, allowed := range vnstatStandardProgramPaths {
		if cleaned == normalizeVnstatPath(allowed) {
			return true
		}
	}
	return false
}

func isSafeVnstatCommandPath(path string) bool {
	return normalizeVnstatPath(path) == normalizeVnstatPath("/usr/bin/vnstat")
}

func managedVnstatBinaryPath(manifest trafficOverviewVnstatManifest) (string, bool) {
	path := normalizeVnstatPath(manifest.BinaryPath)
	if !isTrustedVnstatManifest(manifest) || !isSafeVnstatCommandPath(path) {
		return "", false
	}
	info, err := os.Lstat(path)
	if err != nil || info.IsDir() {
		return "", false
	}
	return path, true
}

func findStandardVnstatBinaryPath() (string, bool) {
	path := normalizeVnstatPath("/usr/bin/vnstat")
	info, err := os.Lstat(path)
	if err != nil || info.IsDir() {
		return "", false
	}
	return path, true
}

func normalizeVnstatPath(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	return pathpkg.Clean(strings.ReplaceAll(trimmed, "\\", "/"))
}

func normalizeAbsolutePathList(items []string) []string {
	seen := make(map[string]struct{})
	result := make([]string, 0, len(items))
	for _, item := range items {
		path := normalizeVnstatPath(item)
		// vnStat manifests always use Linux paths. Keep that interpretation
		// stable when Go unit tests run on a non-Linux development host.
		if path == "" || !strings.HasPrefix(path, "/") {
			continue
		}
		if _, exists := seen[path]; exists {
			continue
		}
		seen[path] = struct{}{}
		result = append(result, path)
	}
	sort.Strings(result)
	return result
}

func detectLinuxSystemFamily() string {
	platform, err := GetSystemPlatform()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(platform.SystemFamily)
}

func parseOsReleaseFields(content string) map[string]string {
	result := make(map[string]string)
	for _, rawLine := range strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		result[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), "\"'")
	}
	return result
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func detectDefaultTrafficInterface() (string, error) {
	if iface := parseDefaultInterfaceFromProcRoute("/proc/net/route"); iface != "" {
		return iface, nil
	}
	if iface := parseDefaultInterfaceFromIPRouteCommand(); iface != "" {
		return iface, nil
	}
	if iface := fallbackFirstActiveInterface(); iface != "" {
		return iface, nil
	}
	return "", errors.New("no default network interface found")
}

func parseDefaultInterfaceFromProcRoute(path string) string {
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		return ""
	}

	bestIface := ""
	bestMetric := int(^uint(0) >> 1)

	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 8 {
			continue
		}
		if fields[1] != "00000000" {
			continue
		}
		flags, err := strconv.ParseUint(fields[3], 16, 64)
		if err != nil || flags&2 == 0 {
			continue
		}
		metric, err := strconv.Atoi(fields[6])
		if err != nil {
			metric = 0
		}
		if bestIface == "" || metric < bestMetric {
			bestIface = fields[0]
			bestMetric = metric
		}
	}

	if err := scanner.Err(); err != nil {
		return ""
	}
	return bestIface
}

func parseDefaultInterfaceFromIPRouteCommand() string {
	if _, err := exec.LookPath("ip"); err != nil {
		return ""
	}

	if iface := parseDefaultInterfaceFromIPRouteOutput(runIPRouteCommand("route", "show", "default")); iface != "" {
		return iface
	}
	return parseDefaultInterfaceFromIPRouteOutput(runIPRouteCommand("-6", "route", "show", "default"))
}

func runIPRouteCommand(args ...string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "ip", args...)
	output, err := cmd.CombinedOutput()
	if err != nil || ctx.Err() == context.DeadlineExceeded {
		return ""
	}
	return string(output)
}

func parseDefaultInterfaceFromIPRouteOutput(output string) string {
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) == 0 {
			continue
		}
		for idx, field := range fields {
			if field == "dev" && idx+1 < len(fields) {
				return fields[idx+1]
			}
		}
	}
	return ""
}

func fallbackFirstActiveInterface() string {
	interfaces, err := psnet.Interfaces()
	if err != nil {
		return ""
	}

	for _, iface := range interfaces {
		if iface.Name == "lo" {
			continue
		}
		if !hasFlag(iface.Flags, "up") {
			continue
		}
		if hasFlag(iface.Flags, "loopback") {
			continue
		}
		return iface.Name
	}
	return ""
}

func hasFlag(flags []string, target string) bool {
	for _, flag := range flags {
		if strings.EqualFold(flag, target) {
			return true
		}
	}
	return false
}

func parseVnstatTrafficTotals(output string) (int64, int64, error) {
	type vnstatTotals struct {
		RX json.Number `json:"rx"`
		TX json.Number `json:"tx"`
	}
	type vnstatTraffic struct {
		Total   vnstatTotals `json:"total"`
		Alltime vnstatTotals `json:"alltime"`
	}
	type vnstatInterface struct {
		Traffic vnstatTraffic `json:"traffic"`
	}
	var payload struct {
		Interfaces []vnstatInterface `json:"interfaces"`
	}
	decoder := json.NewDecoder(strings.NewReader(output))
	decoder.UseNumber()
	if err := decoder.Decode(&payload); err != nil {
		return 0, 0, err
	}

	if len(payload.Interfaces) == 0 {
		return 0, 0, errors.New("vnstat json does not contain interfaces")
	}
	for _, totals := range []vnstatTotals{payload.Interfaces[0].Traffic.Total, payload.Interfaces[0].Traffic.Alltime} {
		rx, rxOK := parseVnstatJSONNumber(totals.RX)
		tx, txOK := parseVnstatJSONNumber(totals.TX)
		if rxOK && txOK {
			return tx, rx, nil
		}
	}

	return 0, 0, errors.New("vnstat json traffic total is missing")
}

func parseVnstatJSONNumber(value json.Number) (int64, bool) {
	if strings.TrimSpace(string(value)) == "" {
		return 0, false
	}
	parsed, err := value.Int64()
	if err == nil && parsed >= 0 {
		return parsed, true
	}
	floatValue, floatErr := value.Float64()
	if floatErr == nil && floatValue >= 0 && floatValue <= float64(maxInt64AsUint64) {
		return int64(floatValue), true
	}
	return 0, false
}

func uint64ToSafeInt64(value uint64) int64 {
	if value > maxInt64AsUint64 {
		return int64(maxInt64AsUint64)
	}
	return int64(value)
}
