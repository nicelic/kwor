package service

import (
	"archive/tar"
	"bufio"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/logger"
	psnet "github.com/shirou/gopsutil/v4/net"
)

type TrafficOverview struct {
	Source     string              `json:"source"`
	Interface  string              `json:"interface"`
	Enabled    bool                `json:"enabled"`
	Status     string              `json:"status"`
	Available  bool                `json:"available"`
	Up         int64               `json:"up"`
	Down       int64               `json:"down"`
	Total      int64               `json:"total"`
	AccumUp    int64               `json:"accumUp"`
	AccumDown  int64               `json:"accumDown"`
	AccumTotal int64               `json:"accumTotal"`
	LimitGiB   float64             `json:"limitGiB"`
	ResetDay   int                 `json:"resetDay"`
	UpdatedAt  int64               `json:"updatedAt"`
	Vnstat     VnstatPackageStatus `json:"vnstat"`
	Error      string              `json:"error,omitempty"`
}

type VnstatPackageStatus struct {
	Supported      bool     `json:"supported"`
	CanManage      bool     `json:"canManage"`
	Installed      bool     `json:"installed"`
	Managed        bool     `json:"managed"`
	Running        bool     `json:"running"`
	Version        string   `json:"version,omitempty"`
	SystemFamily   string   `json:"systemFamily,omitempty"`
	PackageManager string   `json:"packageManager,omitempty"`
	InstallMethod  string   `json:"installMethod,omitempty"`
	BinaryPath     string   `json:"binaryPath,omitempty"`
	FileCount      int      `json:"fileCount"`
	DataPaths      []string `json:"dataPaths,omitempty"`
	ManageHint     string   `json:"manageHint,omitempty"`
	Error          string   `json:"error,omitempty"`
}

type VnstatVersionOption struct {
	Value       string `json:"value"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
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

type trafficOverviewVnstatManifest struct {
	Managed        bool     `json:"managed"`
	SystemFamily   string   `json:"systemFamily"`
	PackageManager string   `json:"packageManager"`
	InstallMethod  string   `json:"installMethod"`
	PackageName    string   `json:"packageName"`
	Version        string   `json:"version"`
	BinaryPath     string   `json:"binaryPath"`
	FilePaths      []string `json:"filePaths"`
	DataPaths      []string `json:"dataPaths"`
	ServiceUnits   []string `json:"serviceUnits"`
	InstalledAt    int64    `json:"installedAt"`
}

const (
	maxInt64AsUint64                 = ^uint64(0) >> 1
	trafficOverviewStateKey          = "trafficOverviewState"
	trafficOverviewLimitGiBKey       = "trafficOverviewLimitGiB"
	trafficOverviewEnabledKey        = "trafficOverviewEnabled"
	trafficOverviewResetDayKey       = "trafficOverviewResetDay"
	trafficOverviewSnapshotKey       = "trafficOverviewSnapshot"
	trafficOverviewCapStateKey       = "trafficOverviewCapState"
	trafficOverviewPauseStateKey     = "trafficOverviewPauseState"
	trafficOverviewVnstatManifestKey = "trafficOverviewVnstatManifest"
	trafficOverviewMinDisplayGiB     = 0.01
	trafficOverviewFlushInterval     = 3 * time.Second
	trafficOverviewFlushDelta        = int64(256 * 1024)
	trafficCapTagLoopback            = "loopback"
	trafficCapTagDropExcept          = "drop_except_allowed"
	trafficCapTagDropForward         = "drop_all_forward"
	vnstatPackageName                = "vnstat"
	vnstatInstallMethodSystemPackage = "system-package"
	vnstatInstallMethodGitHubRelease = "github-release"
	vnstatGitHubLatestReleaseAPI     = "https://api.github.com/repos/vergoh/vnstat/releases/latest"
	vnstatSystemdUnitPath            = "/etc/systemd/system/vnstat.service"
)

var trafficOverviewStateMu sync.Mutex
var trafficOverviewSnapshotMu sync.Mutex
var trafficOverviewCapMu sync.Mutex
var trafficOverviewShutdownEnabledFn = func() bool {
	return runtime.GOOS == "linux" && nftSupported()
}

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

type trafficOverviewCapState struct {
	Active       bool  `json:"active"`
	LimitReached bool  `json:"limitReached"`
	AllowedPorts []int `json:"allowedPorts"`
	UpdatedAt    int64 `json:"updatedAt"`
}

var trafficOverviewSnapshotCache trafficOverviewSnapshotState

func (s *TrafficOverviewService) GetTrafficOverview() (*TrafficOverview, error) {
	overview := &TrafficOverview{
		Source:    "vnstat",
		Enabled:   true,
		Status:    "stopped",
		UpdatedAt: time.Now().Unix(),
	}
	limitGiB, resetDay, enabled, configErr := s.getOverviewConfig()
	if configErr != nil {
		overview.Error = configErr.Error()
		return overview, nil
	}
	overview.LimitGiB = limitGiB
	overview.ResetDay = resetDay
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

	if runtime.GOOS != "linux" {
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

	now := time.Now().In(s.getOverviewLocation())
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
		if err := s.saveRuntimeState(state); err != nil {
			logger.Warning("save traffic overview state failed:", err)
		}
	}
	if err := s.stageOverviewSnapshot(snapshotFromOverview(overview), false); err != nil {
		logger.Warning("save traffic overview snapshot failed:", err)
	}
	return overview, nil
}

func (s *TrafficOverviewService) UpdateTrafficOverviewSettings(limitGiB float64, resetDay int) error {
	limitGiB = normalizeLimitGiB(limitGiB)
	resetDay = normalizeResetDay(resetDay)

	settingSvc := &SettingService{}
	if err := settingSvc.setString(trafficOverviewLimitGiBKey, strconv.FormatFloat(limitGiB, 'f', 2, 64)); err != nil {
		return err
	}
	if err := settingSvc.setString(trafficOverviewResetDayKey, strconv.Itoa(resetDay)); err != nil {
		return err
	}
	if err := s.ReconcileTrafficCap(); err != nil {
		logger.Warning("reconcile traffic cap after settings update failed:", err)
	}
	return nil
}

func (s *TrafficOverviewService) SetTrafficOverviewEnabled(enabled bool) error {
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
		if err := (&SettingService{}).setString(trafficOverviewEnabledKey, "false"); err != nil {
			return err
		}
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
	if runtime.GOOS != "linux" {
		return nil
	}
	if _, err := exec.LookPath("vnstat"); err != nil {
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
	if err := s.ReconcileTrafficCap(); err != nil {
		logger.Warning("reconcile traffic cap after enabling overview failed:", err)
	}
	return nil
}

func (s *TrafficOverviewService) pauseTrafficOverviewAccounting() error {
	if runtime.GOOS != "linux" {
		return s.pauseTrafficOverviewWithCachedSnapshot()
	}

	if _, err := exec.LookPath("vnstat"); err != nil {
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

	_, resetDay, _, cfgErr := s.getOverviewConfig()
	if cfgErr != nil {
		return cfgErr
	}

	now := time.Now().In(s.getOverviewLocation())
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
	if runtime.GOOS != "linux" {
		return s.clearPauseState()
	}
	if _, err := exec.LookPath("vnstat"); err != nil {
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
	_, resetDay, _, cfgErr := s.getOverviewConfig()
	if cfgErr != nil {
		return cfgErr
	}
	now := time.Now().In(s.getOverviewLocation())

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
	status := VnstatPackageStatus{
		Supported: runtime.GOOS == "linux",
		CanManage: runtime.GOOS == "linux",
		DataPaths: defaultVnstatDataPaths(),
	}
	if runtime.GOOS != "linux" {
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
		status.SystemFamily = manager.SystemFamily
	}
	if family := detectLinuxSystemFamily(); family != "" {
		status.SystemFamily = family
	}

	manifest, hasManifest := s.loadVnstatManifest()
	if hasManifest {
		status.Managed = manifest.Managed
		status.PackageManager = firstNonEmpty(manifest.PackageManager, status.PackageManager)
		status.InstallMethod = firstNonEmpty(manifest.InstallMethod, normalizeVnstatInstallMethod("", manifest.PackageManager), status.InstallMethod)
		status.SystemFamily = firstNonEmpty(manifest.SystemFamily, status.SystemFamily)
		status.BinaryPath = strings.TrimSpace(manifest.BinaryPath)
		status.FileCount = len(normalizeAbsolutePathList(manifest.FilePaths))
		if len(manifest.DataPaths) > 0 {
			status.DataPaths = normalizeAbsolutePathList(manifest.DataPaths)
		}
	}

	if binaryPath, err := exec.LookPath("vnstat"); err == nil {
		status.Installed = true
		status.BinaryPath = firstNonEmpty(binaryPath, status.BinaryPath)
		status.Running = isVnstatDaemonRunning()
		if status.FileCount == 0 {
			status.FileCount = len(collectVnstatPackageFilesByManager(status.PackageManager))
		}
		if status.InstallMethod == "" {
			status.InstallMethod = normalizeVnstatInstallMethod("", status.PackageManager)
		}
		if status.InstallMethod == vnstatInstallMethodSystemPackage {
			if version := detectInstalledVnstatPackageVersion(status.PackageManager); version != "" {
				status.Version = version
			}
		}
		if status.Version == "" {
			if version := detectVnstatVersion(); version != "" {
				status.Version = version
			} else if hasManifest {
				status.Version = strings.TrimSpace(manifest.Version)
			}
		}
	} else if hasManifest {
		status.Version = strings.TrimSpace(manifest.Version)
	}

	return status
}

func (s *TrafficOverviewService) GetVnstatVersionOptions() (*VnstatVersionListResult, error) {
	title := "自动安装（系统源优先）"
	description := "默认使用当前系统软件源安装 vnstat；系统软件源不可用时自动回退到 GitHub 官方版本"
	if manager := detectVnstatPackageManagerPlan(); manager != nil {
		description = fmt.Sprintf("默认通过 %s 安装 vnstat；失败时自动回退到 GitHub 官方版本", manager.Name)
	}
	if canManage, manageHint := vnstatManagementSupport(); !canManage {
		description = manageHint
	}
	return &VnstatVersionListResult{
		Versions: []VnstatVersionOption{
			{
				Value:       "system",
				Title:       title,
				Description: description,
			},
		},
	}, nil
}

func (s *TrafficOverviewService) GetVnstatUpdateInfo() (*VnstatVersionCheckResult, error) {
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
	if !status.Installed {
		result.Message = "vnstat 尚未安装"
		return result, nil
	}

	latestVersion, source, err := detectLatestVnstatVersion(status)
	result.Source = source
	if err != nil {
		result.Message = strings.TrimSpace(err.Error())
		return result, nil
	}
	result.LatestVersion = strings.TrimSpace(latestVersion)
	if result.CurrentVersion == "" {
		result.Message = "已安装 vnstat，但未能识别当前版本"
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

func (s *TrafficOverviewService) InstallManagedVnstat(version string) (*TrafficOverview, error) {
	if runtime.GOOS != "linux" {
		return nil, errors.New("vnstat is supported on linux only")
	}
	if canManage, manageHint := vnstatManagementSupport(); !canManage {
		return nil, errors.New(manageHint)
	}
	version = strings.TrimSpace(version)
	if version == "" {
		version = "system"
	}
	if !strings.EqualFold(version, "system") {
		return nil, fmt.Errorf("vnstat version %q is not supported by system package installation", version)
	}

	manager := detectVnstatPackageManagerPlan()
	manifest, hasManifest := s.loadVnstatManifest()
	if binaryPath, lookErr := exec.LookPath("vnstat"); lookErr == nil {
		currentMethod := detectInstalledVnstatInstallMethod(manifest, hasManifest, manager, binaryPath)
		if currentMethod != "" && os.Geteuid() != 0 {
			if currentMethod == vnstatInstallMethodSystemPackage && manager != nil && len(manager.InstallPlan) > 0 {
				return nil, fmt.Errorf("vnstat reinstall/update requires root. install it manually: %s", strings.Join(manager.InstallPlan[len(manager.InstallPlan)-1], " "))
			}
			return nil, errors.New("vnstat reinstall/update requires root")
		}
	} else if os.Geteuid() != 0 {
		if manager != nil && len(manager.InstallPlan) > 0 {
			return nil, fmt.Errorf("vnstat install requires root. install it manually: %s", strings.Join(manager.InstallPlan[len(manager.InstallPlan)-1], " "))
		}
		return nil, errors.New("vnstat install requires root")
	}

	manifest, err := s.installOrReinstallManagedVnstat(manager)
	if err != nil {
		return nil, err
	}
	if err := s.saveVnstatManifest(manifest); err != nil {
		return nil, err
	}
	if err := (&SettingService{}).setString(trafficOverviewEnabledKey, "true"); err != nil {
		return nil, err
	}
	if err := s.clearPauseState(); err != nil {
		return nil, err
	}

	if iface, detectErr := detectDefaultTrafficInterface(); detectErr == nil && iface != "" {
		if trackErr := ensureVnstatTracking(iface); trackErr != nil {
			logger.Warning("ensure vnstat tracking after install failed:", trackErr)
		}
	}
	if daemonErr := ensureVnstatDaemonRunning(); daemonErr != nil {
		logger.Warning("ensure vnstat daemon after install failed:", daemonErr)
	}

	return s.GetTrafficOverview()
}

func (s *TrafficOverviewService) RemoveManagedVnstat() (*TrafficOverview, error) {
	if runtime.GOOS != "linux" {
		return nil, errors.New("vnstat is supported on linux only")
	}
	if canManage, manageHint := vnstatManagementSupport(); !canManage {
		return nil, errors.New(manageHint)
	}

	manifest, hasManifest := s.loadVnstatManifest()
	manager := detectVnstatPackageManagerPlan()
	if hasManifest && strings.TrimSpace(manifest.PackageManager) != "" {
		if detected := managerByName(manifest.PackageManager); detected != nil {
			manager = detected
		}
	}
	installMethod := normalizeVnstatInstallMethod(manifest.InstallMethod, manifest.PackageManager)

	installed := false
	if _, err := exec.LookPath("vnstat"); err == nil {
		installed = true
	}
	if installed && os.Geteuid() != 0 {
		return nil, errors.New("removing vnstat requires root")
	}

	if err := s.SetTrafficOverviewEnabled(false); err != nil {
		return nil, err
	}
	stopVnstatDaemon()

	if installed && installMethod != vnstatInstallMethodGitHubRelease {
		if manager == nil {
			return nil, errors.New("vnstat is installed, but no supported linux package manager was found for removal")
		}
		for _, command := range manager.RemovePlan {
			if err := runInstallCommand(command); err != nil {
				return nil, err
			}
		}
	}

	if err := removeVnstatTrackedData(manifest); err != nil {
		return nil, err
	}
	if systemctlPath, err := exec.LookPath("systemctl"); err == nil {
		_ = exec.Command(systemctlPath, "daemon-reload").Run()
		_ = exec.Command(systemctlPath, "reset-failed").Run()
	}
	if err := s.clearVnstatManagedState(); err != nil {
		return nil, err
	}
	if err := cleanupTrafficCapRules(); err != nil {
		logger.Warning("cleanup traffic cap after vnstat removal failed:", err)
	}

	return s.GetTrafficOverview()
}

func (s *TrafficOverviewService) RemoveManagedVnstatForUninstall() error {
	if runtime.GOOS != "linux" {
		return nil
	}
	if database.GetDB() == nil {
		return removeVnstatWithoutDatabase()
	}
	_, err := s.RemoveManagedVnstat()
	return err
}

func removeVnstatWithoutDatabase() error {
	manifest := trafficOverviewVnstatManifest{
		Managed:      true,
		PackageName:  vnstatPackageName,
		DataPaths:    defaultVnstatDataPaths(),
		ServiceUnits: []string{"vnstat", "vnstatd"},
	}
	manager := detectVnstatPackageManagerPlan()
	if binaryPath, err := exec.LookPath("vnstat"); err == nil {
		manifest.BinaryPath = binaryPath
		if manager != nil {
			manifest.PackageManager = manager.Name
			manifest.InstallMethod = vnstatInstallMethodSystemPackage
			manifest.FilePaths = collectVnstatPackageFilesByManager(manager.Name)
		}
	}
	manifest.FilePaths = appendDetectedVnstatManagedPaths(manifest.FilePaths)

	stopVnstatDaemon()
	if manifest.BinaryPath != "" && os.Geteuid() == 0 && manager != nil {
		for _, command := range manager.RemovePlan {
			if err := runInstallCommand(command); err != nil {
				return err
			}
		}
	}
	if err := removeVnstatTrackedData(manifest); err != nil {
		return err
	}
	if systemctlPath, err := exec.LookPath("systemctl"); err == nil {
		_ = exec.Command(systemctlPath, "daemon-reload").Run()
		_ = exec.Command(systemctlPath, "reset-failed").Run()
	}
	return cleanupTrafficCapRules()
}

func (s *TrafficOverviewService) installOrReinstallManagedVnstat(manager *vnstatPackageManagerPlan) (trafficOverviewVnstatManifest, error) {
	if binaryPath, err := exec.LookPath("vnstat"); err == nil {
		manifest, hasManifest := s.loadVnstatManifest()
		currentMethod := detectInstalledVnstatInstallMethod(manifest, hasManifest, manager, binaryPath)
		switch currentMethod {
		case vnstatInstallMethodSystemPackage:
			if manager == nil {
				return trafficOverviewVnstatManifest{}, errors.New("vnstat is installed, but no supported linux package manager was found for reinstall/update")
			}
			return s.installVnstatViaSystemPackage(manager)
		case vnstatInstallMethodGitHubRelease:
			return s.installVnstatViaGitHubRelease(manager)
		default:
			return s.adoptCurrentVnstatInstallation(manager, binaryPath), nil
		}
	}
	return s.installOrAdoptManagedVnstat(manager)
}

func (s *TrafficOverviewService) installOrAdoptManagedVnstat(manager *vnstatPackageManagerPlan) (trafficOverviewVnstatManifest, error) {
	if binaryPath, err := exec.LookPath("vnstat"); err == nil {
		return s.adoptCurrentVnstatInstallation(manager, binaryPath), nil
	}

	var systemErr error
	if manager != nil {
		manifest, err := s.installVnstatViaSystemPackage(manager)
		if err == nil {
			return manifest, nil
		}
		systemErr = err
	} else {
		systemErr = errors.New("no supported linux package manager was found")
	}

	manifest, githubErr := s.installVnstatViaGitHubRelease(manager)
	if githubErr == nil {
		return manifest, nil
	}
	return trafficOverviewVnstatManifest{}, buildVnstatInstallUnavailableError(systemErr, githubErr)
}

func (s *TrafficOverviewService) adoptCurrentVnstatInstallation(manager *vnstatPackageManagerPlan, binaryPath string) trafficOverviewVnstatManifest {
	packageManager := ""
	systemFamily := detectLinuxSystemFamily()
	installMethod := ""
	filePaths := []string{binaryPath}

	if manager != nil {
		packageManager = manager.Name
		systemFamily = firstNonEmpty(systemFamily, manager.SystemFamily)
		installMethod = vnstatInstallMethodSystemPackage
		filePaths = collectVnstatPackageFilesByManager(manager.Name)
	}

	filePaths = appendDetectedVnstatManagedPaths(filePaths)
	version := detectVnstatVersion()
	if installMethod == vnstatInstallMethodSystemPackage {
		version = firstNonEmpty(detectInstalledVnstatPackageVersion(packageManager), version)
	}
	return trafficOverviewVnstatManifest{
		Managed:        true,
		SystemFamily:   systemFamily,
		PackageManager: packageManager,
		InstallMethod:  installMethod,
		PackageName:    vnstatPackageName,
		Version:        version,
		BinaryPath:     binaryPath,
		FilePaths:      filePaths,
		DataPaths:      defaultVnstatDataPaths(),
		ServiceUnits:   []string{"vnstat", "vnstatd"},
		InstalledAt:    time.Now().Unix(),
	}
}

func (s *TrafficOverviewService) installVnstatViaSystemPackage(manager *vnstatPackageManagerPlan) (trafficOverviewVnstatManifest, error) {
	if manager == nil {
		return trafficOverviewVnstatManifest{}, errors.New("no supported linux package manager was found")
	}
	for _, command := range manager.InstallPlan {
		if err := runInstallCommand(command); err != nil {
			return trafficOverviewVnstatManifest{}, err
		}
	}

	binaryPath, err := exec.LookPath("vnstat")
	if err != nil {
		return trafficOverviewVnstatManifest{}, fmt.Errorf("vnstat package install completed but binary is still missing: %w", err)
	}

	filePaths := appendDetectedVnstatManagedPaths(collectVnstatPackageFilesByManager(manager.Name))
	version := firstNonEmpty(detectInstalledVnstatPackageVersion(manager.Name), detectVnstatVersion())
	return trafficOverviewVnstatManifest{
		Managed:        true,
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

func (s *TrafficOverviewService) installVnstatViaGitHubRelease(manager *vnstatPackageManagerPlan) (trafficOverviewVnstatManifest, error) {
	var buildDepsErr error
	if manager != nil {
		for _, command := range manager.BuildDepsPlan {
			if err := runInstallCommand(command); err != nil {
				buildDepsErr = fmt.Errorf("install build dependencies failed: %w", err)
				logger.Warning(buildDepsErr)
				break
			}
		}
	}

	release, err := fetchLatestVnstatRelease()
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
	if err := downloadFileWithUserAgent(asset.BrowserDownloadURL, archivePath, 10*time.Minute); err != nil {
		return trafficOverviewVnstatManifest{}, fmt.Errorf("download GitHub release asset failed: %w", err)
	}

	sourceDir, err := extractVnstatSourceArchive(archivePath, workDir)
	if err != nil {
		return trafficOverviewVnstatManifest{}, fmt.Errorf("extract vnstat source archive failed: %w", err)
	}
	if _, err := runCommandInDir(sourceDir, 5*time.Minute, "./configure", "--prefix=/usr", "--sysconfdir=/etc"); err != nil {
		if buildDepsErr != nil {
			return trafficOverviewVnstatManifest{}, fmt.Errorf("%w; %v", err, buildDepsErr)
		}
		return trafficOverviewVnstatManifest{}, fmt.Errorf("configure vnstat source failed: %w", err)
	}
	if _, err := runCommandInDir(sourceDir, 10*time.Minute, "make"); err != nil {
		if buildDepsErr != nil {
			return trafficOverviewVnstatManifest{}, fmt.Errorf("%w; %v", err, buildDepsErr)
		}
		return trafficOverviewVnstatManifest{}, fmt.Errorf("build vnstat source failed: %w", err)
	}

	stageDir := filepath.Join(workDir, "stage")
	managedPaths := []string{}
	if _, err := runCommandInDir(sourceDir, 5*time.Minute, "make", "install", "DESTDIR="+stageDir); err == nil {
		managedPaths = collectManagedSourceVnstatPaths(stageDir)
	} else {
		logger.Warning("stage vnstat source install for manifest collection failed:", err)
	}

	if _, err := runCommandInDir(sourceDir, 5*time.Minute, "make", "install"); err != nil {
		return trafficOverviewVnstatManifest{}, fmt.Errorf("install vnstat source failed: %w", err)
	}

	if unitPath, unitErr := installVnstatSystemdUnit(sourceDir); unitErr == nil && unitPath != "" {
		managedPaths = append(managedPaths, unitPath)
	} else if unitErr != nil {
		logger.Warning("install vnstat systemd unit failed:", unitErr)
	}

	binaryPath, err := exec.LookPath("vnstat")
	if err != nil {
		return trafficOverviewVnstatManifest{}, fmt.Errorf("vnstat GitHub install completed but binary is still missing: %w", err)
	}

	managedPaths = appendDetectedVnstatManagedPaths(managedPaths)
	return trafficOverviewVnstatManifest{
		Managed:        true,
		SystemFamily:   firstNonEmpty(detectLinuxSystemFamily(), systemFamilyFromManager(manager)),
		PackageManager: packageManagerName(manager),
		InstallMethod:  vnstatInstallMethodGitHubRelease,
		PackageName:    vnstatPackageName,
		Version:        firstNonEmpty(detectVnstatVersion(), strings.TrimPrefix(strings.TrimSpace(release.TagName), "v")),
		BinaryPath:     binaryPath,
		FilePaths:      managedPaths,
		DataPaths:      defaultVnstatDataPaths(),
		ServiceUnits:   []string{"vnstat", "vnstatd"},
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
	if binaryPath, err := exec.LookPath("vnstat"); err == nil {
		paths = append(paths, binaryPath)
	}
	if daemonPath, err := exec.LookPath("vnstatd"); err == nil {
		paths = append(paths, daemonPath)
	}
	for _, candidate := range []string{"/usr/bin/vnstati", "/etc/vnstat.conf", vnstatSystemdUnitPath} {
		if _, err := os.Stat(candidate); err == nil {
			paths = append(paths, candidate)
		}
	}
	return normalizeAbsolutePathList(paths)
}

func normalizeVnstatInstallMethod(method string, packageManager string) string {
	method = strings.TrimSpace(strings.ToLower(method))
	if method != "" {
		return method
	}
	if managerByName(packageManager) != nil {
		return vnstatInstallMethodSystemPackage
	}
	return ""
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
	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest("GET", vnstatGitHubLatestReleaseAPI, nil)
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
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
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
	client := &http.Client{Timeout: timeout}
	req, err := http.NewRequest("GET", strings.TrimSpace(url), nil)
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

	out, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	return err
}

func extractVnstatSourceArchive(archivePath string, workDir string) (string, error) {
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
			if _, err := io.Copy(out, tr); err != nil {
				_ = out.Close()
				return "", err
			}
			if err := out.Close(); err != nil {
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
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(output))
	if ctx.Err() == context.DeadlineExceeded {
		if text == "" {
			return "", fmt.Errorf("%s timed out", name)
		}
		return text, fmt.Errorf("%s timed out: %s", name, text)
	}
	if err != nil {
		if text == "" {
			return "", fmt.Errorf("%s failed: %w", name, err)
		}
		return text, fmt.Errorf("%s failed: %w: %s", name, err, text)
	}
	return text, nil
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
	switch filepath.ToSlash(filepath.Clean(strings.TrimSpace(path))) {
	case "/usr/bin/vnstat", "/usr/sbin/vnstatd", "/usr/bin/vnstati", "/etc/vnstat.conf":
		return true
	default:
		return false
	}
}

func installVnstatSystemdUnit(sourceDir string) (string, error) {
	if runtime.GOOS != "linux" {
		return "", nil
	}
	systemctlPath, err := exec.LookPath("systemctl")
	if err != nil {
		return "", nil
	}

	unitContent, readErr := os.ReadFile(filepath.Join(sourceDir, "examples", "systemd", "simple", "vnstat.service"))
	if readErr != nil {
		unitContent = []byte("[Unit]\nDescription=vnStat network traffic monitor\nDocumentation=man:vnstatd(8) man:vnstat(1) man:vnstat.conf(5)\nAfter=network.target\n\n[Service]\nExecStart=/usr/sbin/vnstatd --nodaemon\nExecReload=/bin/kill -HUP $MAINPID\nRestart=on-failure\n\n[Install]\nWantedBy=multi-user.target\nAlias=vnstatd.service\n")
	}
	if err := os.WriteFile(vnstatSystemdUnitPath, unitContent, 0o644); err != nil {
		return "", err
	}
	output, err := exec.Command(systemctlPath, "daemon-reload").CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("systemctl daemon-reload failed: %v: %s", err, strings.TrimSpace(string(output)))
	}
	return vnstatSystemdUnitPath, nil
}

func (s *TrafficOverviewService) ResetAllTrafficOverviewStats() error {
	if runtime.GOOS != "linux" {
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

	_, resetDay, _, cfgErr := s.getOverviewConfig()
	if cfgErr != nil {
		return cfgErr
	}

	now := time.Now().In(s.getOverviewLocation())
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
	if err := s.ReconcileTrafficCap(); err != nil {
		logger.Warning("reconcile traffic cap after manual reset failed:", err)
	}
	return nil
}

func (s *TrafficOverviewService) ResetPeriodTrafficOverviewStats() error {
	if runtime.GOOS != "linux" {
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

	_, resetDay, _, cfgErr := s.getOverviewConfig()
	if cfgErr != nil {
		return cfgErr
	}

	now := time.Now().In(s.getOverviewLocation())
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
	if err := s.ReconcileTrafficCap(); err != nil {
		logger.Warning("reconcile traffic cap after period reset failed:", err)
	}
	return nil
}

func (s *TrafficOverviewService) ResetTotalTrafficOverviewStats() error {
	if runtime.GOOS != "linux" {
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

	now := time.Now().In(s.getOverviewLocation())
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
	if err := s.ReconcileTrafficCap(); err != nil {
		logger.Warning("reconcile traffic cap after total reset failed:", err)
	}
	return nil
}

func (s *TrafficOverviewService) EnsureRuntimeReady() error {
	if runtime.GOOS != "linux" {
		return nil
	}
	if enabled, err := s.isOverviewEnabled(); err != nil {
		return err
	} else if !enabled {
		return nil
	}
	if _, err := exec.LookPath("vnstat"); err != nil {
		return nil
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
	if err := ensureVnstatDaemonRunning(); err != nil {
		logger.Warning("initial vnstat daemon ensure failed:", err)
	}
	if err := s.ReconcileTrafficCap(); err != nil {
		logger.Warning("initial traffic cap reconcile failed:", err)
	}
	return nil
}

func (s *TrafficOverviewService) FlushPendingSnapshot() error {
	return s.flushOverviewSnapshot(true)
}

func (s *TrafficOverviewService) ReconcileTrafficCap() error {
	enabled, err := s.isOverviewEnabled()
	if err != nil {
		return err
	}
	if !enabled {
		return cleanupTrafficCapRules()
	}
	overview, err := s.GetTrafficOverview()
	if err != nil {
		return err
	}
	return s.reconcileTrafficCapFromOverview(overview)
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
	if runtime.GOOS != "linux" || !nftSupported() {
		return nil
	}

	limitGiB := 0.0
	if overview != nil {
		limitGiB = normalizeLimitGiB(overview.LimitGiB)
	}
	if limitGiB <= 0 {
		loadedLimitGiB, _, _, err := s.getOverviewConfig()
		if err == nil {
			limitGiB = normalizeLimitGiB(loadedLimitGiB)
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

	hasRules := hasTrafficCapRules()
	active := state.Active || hasRules
	limitReached := state.LimitReached
	if limitGiB <= 0 {
		limitReached = false
	} else if overview != nil && overview.Available && strings.TrimSpace(overview.Error) == "" {
		limitReached = overview.AccumTotal >= limitGiBToBytes(limitGiB)
	}

	desiredActive := limitGiB > 0 && limitReached

	if desiredActive {
		needsRuleRefresh := !active || !intSliceEqual(state.AllowedPorts, allowedPorts) || !hasRules
		if needsRuleRefresh {
			if err := applyTrafficCapRules(allowedPorts); err != nil {
				return err
			}
			hasRules = true
		}
		state.Active = hasRules
		state.LimitReached = true
		state.AllowedPorts = allowedPorts
		state.UpdatedAt = time.Now().Unix()
		return s.saveCapStateLocked(state)
	}

	if active {
		if err := cleanupTrafficCapRules(); err != nil {
			return err
		}
	}
	state.Active = false
	state.LimitReached = false
	state.AllowedPorts = allowedPorts
	state.UpdatedAt = time.Now().Unix()
	return s.saveCapStateLocked(state)
}

func (s *TrafficOverviewService) getOverviewConfig() (float64, int, bool, error) {
	settingSvc := &SettingService{}

	limitRaw, err := settingSvc.getString(trafficOverviewLimitGiBKey)
	if err != nil {
		return 0, 0, true, err
	}
	limitGiB := 0.0
	if strings.TrimSpace(limitRaw) != "" {
		if parsed, parseErr := strconv.ParseFloat(strings.TrimSpace(limitRaw), 64); parseErr == nil {
			limitGiB = parsed
		}
	}

	resetRaw, err := settingSvc.getString(trafficOverviewResetDayKey)
	if err != nil {
		return 0, 0, true, err
	}
	resetDay := 0
	if strings.TrimSpace(resetRaw) != "" {
		if parsed, parseErr := strconv.Atoi(strings.TrimSpace(resetRaw)); parseErr == nil {
			resetDay = parsed
		}
	}

	enabled, err := settingSvc.getBool(trafficOverviewEnabledKey)
	if err != nil {
		return 0, 0, true, err
	}

	return normalizeLimitGiB(limitGiB), normalizeResetDay(resetDay), enabled, nil
}

func (s *TrafficOverviewService) loadRuntimeState() (trafficOverviewRuntimeState, error) {
	settingSvc := &SettingService{}
	raw, err := settingSvc.getString(trafficOverviewStateKey)
	if err != nil {
		return trafficOverviewRuntimeState{}, err
	}

	state := trafficOverviewRuntimeState{}
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "{}" {
		return state, nil
	}
	if err := json.Unmarshal([]byte(trimmed), &state); err != nil {
		return trafficOverviewRuntimeState{}, err
	}

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
	return state, nil
}

func (s *TrafficOverviewService) saveRuntimeState(state trafficOverviewRuntimeState) error {
	raw, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return (&SettingService{}).setString(trafficOverviewStateKey, string(raw))
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
		return time.Local
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
	if runtime.GOOS != "linux" {
		return []int{22}
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
	if runtime.GOOS != "linux" || !nftSupported() {
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
	if runtime.GOOS != "linux" || !nftSupported() {
		return nil
	}
	return deleteRulesByCommentPrefix(trafficCapNftRuleComments.prefix)
}

func hasTrafficCapRules() bool {
	if runtime.GOOS != "linux" || !nftSupported() || !nftTableExists() {
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
	if runtime.GOOS != "linux" {
		return nil
	}

	if _, err := exec.LookPath("vnstat"); err == nil {
		return nil
	}

	return errors.New("vnstat is not installed")
}

func detectVnstatPackageManagerPlan() *vnstatPackageManagerPlan {
	for _, candidate := range vnstatPackageManagerPlans() {
		if _, err := exec.LookPath(candidate.Name); err == nil {
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
	if len(command) == 0 {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, command[0], command[1:]...)
	output, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("install command timed out: %s", strings.Join(command, " "))
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

func detectLatestVnstatVersion(status VnstatPackageStatus) (string, string, error) {
	method := normalizeVnstatInstallMethod(status.InstallMethod, status.PackageManager)
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

func detectVnstatVersion() string {
	if _, err := exec.LookPath("vnstat"); err != nil {
		return ""
	}
	output, err := runCommandOutput([]string{"vnstat", "--version"}, 4*time.Second)
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
	if runtime.GOOS != "linux" {
		return false
	}
	if systemctlPath, err := exec.LookPath("systemctl"); err == nil {
		for _, unit := range []string{"vnstat", "vnstatd"} {
			if exec.Command(systemctlPath, "is-active", "--quiet", unit).Run() == nil {
				return true
			}
		}
	}
	if pgrepPath, err := exec.LookPath("pgrep"); err == nil {
		for _, name := range []string{"vnstatd", "vnstat"} {
			if exec.Command(pgrepPath, "-x", name).Run() == nil {
				return true
			}
		}
	}
	return false
}

func stopVnstatDaemon() {
	if runtime.GOOS != "linux" {
		return
	}
	if systemctlPath, err := exec.LookPath("systemctl"); err == nil {
		for _, unit := range []string{"vnstat", "vnstatd"} {
			_ = exec.Command(systemctlPath, "stop", unit).Run()
			_ = exec.Command(systemctlPath, "disable", unit).Run()
		}
		_ = exec.Command(systemctlPath, "daemon-reload").Run()
		_ = exec.Command(systemctlPath, "reset-failed").Run()
	}
	if servicePath, err := exec.LookPath("service"); err == nil {
		for _, unit := range []string{"vnstat", "vnstatd"} {
			_ = exec.Command(servicePath, unit, "stop").Run()
		}
	}
	if pkillPath, err := exec.LookPath("pkill"); err == nil {
		for _, name := range []string{"vnstatd", "vnstat"} {
			_ = exec.Command(pkillPath, "-x", name).Run()
		}
	}
}

func ensureVnstatDaemonRunning() error {
	if runtime.GOOS != "linux" {
		return nil
	}
	if _, err := exec.LookPath("vnstat"); err != nil {
		return nil
	}
	if isVnstatDaemonRunning() {
		return nil
	}
	return restartVnstatDaemon()
}

func restartVnstatDaemon() error {
	if runtime.GOOS != "linux" {
		return nil
	}
	if systemctlPath, err := exec.LookPath("systemctl"); err == nil {
		for _, unit := range []string{"vnstat", "vnstatd"} {
			if err := exec.Command(systemctlPath, "restart", unit).Run(); err == nil {
				return nil
			}
		}
	}
	if servicePath, err := exec.LookPath("service"); err == nil {
		for _, unit := range []string{"vnstat", "vnstatd"} {
			if err := exec.Command(servicePath, unit, "restart").Run(); err == nil {
				return nil
			}
		}
	}
	if daemonPath, err := exec.LookPath("vnstatd"); err == nil {
		stopVnstatDaemon()
		if err := exec.Command(daemonPath, "--daemon").Run(); err == nil {
			return nil
		}
	}
	return errors.New("vnstat daemon restart failed")
}

func (s *TrafficOverviewService) isOverviewEnabled() (bool, error) {
	return (&SettingService{}).getBool(trafficOverviewEnabledKey)
}

func (s *TrafficOverviewService) hasManagedVnstatManifest() bool {
	manifest, ok := s.loadVnstatManifest()
	return ok && manifest.Managed
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
	manifest.PackageManager = strings.TrimSpace(strings.ToLower(manifest.PackageManager))
	manifest.SystemFamily = strings.TrimSpace(strings.ToLower(manifest.SystemFamily))
	manifest.InstallMethod = normalizeVnstatInstallMethod(manifest.InstallMethod, manifest.PackageManager)
	manifest.PackageName = firstNonEmpty(manifest.PackageName, vnstatPackageName)
	manifest.BinaryPath = strings.TrimSpace(manifest.BinaryPath)
	manifest.Version = strings.TrimSpace(manifest.Version)
	manifest.FilePaths = normalizeAbsolutePathList(manifest.FilePaths)
	manifest.DataPaths = normalizeAbsolutePathList(manifest.DataPaths)
	manifest.ServiceUnits = uniqueStringList(manifest.ServiceUnits)
	return manifest, manifest.Managed || manifest.BinaryPath != "" || len(manifest.FilePaths) > 0
}

func (s *TrafficOverviewService) saveVnstatManifest(manifest trafficOverviewVnstatManifest) error {
	manifest.Managed = true
	manifest.PackageManager = strings.TrimSpace(strings.ToLower(manifest.PackageManager))
	manifest.SystemFamily = strings.TrimSpace(strings.ToLower(manifest.SystemFamily))
	manifest.InstallMethod = normalizeVnstatInstallMethod(manifest.InstallMethod, manifest.PackageManager)
	manifest.PackageName = firstNonEmpty(manifest.PackageName, vnstatPackageName)
	manifest.BinaryPath = strings.TrimSpace(manifest.BinaryPath)
	manifest.Version = strings.TrimSpace(manifest.Version)
	manifest.FilePaths = normalizeAbsolutePathList(manifest.FilePaths)
	manifest.DataPaths = normalizeAbsolutePathList(manifest.DataPaths)
	manifest.ServiceUnits = uniqueStringList(manifest.ServiceUnits)
	raw, err := json.Marshal(manifest)
	if err != nil {
		return err
	}
	return (&SettingService{}).setString(trafficOverviewVnstatManifestKey, string(raw))
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
	return nil
}

func removeVnstatTrackedData(manifest trafficOverviewVnstatManifest) error {
	targets := defaultVnstatDataPaths()
	targets = append(targets, manifest.DataPaths...)
	for _, path := range normalizeAbsolutePathList(targets) {
		if !isSafeVnstatDataPath(path) {
			continue
		}
		if err := os.RemoveAll(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove vnstat data path failed %s: %w", path, err)
		}
	}

	for _, path := range normalizeAbsolutePathList(manifest.FilePaths) {
		if !isSafeVnstatResidualPath(path) {
			continue
		}
		info, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		if info.IsDir() {
			if !isSafeVnstatDataPath(path) {
				continue
			}
			if err := os.RemoveAll(path); err != nil && !os.IsNotExist(err) {
				return err
			}
			continue
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func defaultVnstatDataPaths() []string {
	return []string{
		"/var/lib/vnstat",
		"/var/log/vnstat",
		"/var/cache/vnstat",
	}
}

func vnstatManagementSupport() (bool, string) {
	return vnstatManagementSupportForRuntime(runtime.GOOS, runningInsideContainer())
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
	cleaned := filepath.Clean(strings.TrimSpace(path))
	for _, allowed := range defaultVnstatDataPaths() {
		if cleaned == filepath.Clean(allowed) {
			return true
		}
	}
	return false
}

func isSafeVnstatResidualPath(path string) bool {
	cleaned := filepath.Clean(strings.TrimSpace(path))
	if isSafeVnstatDataPath(cleaned) {
		return true
	}
	allowedFiles := map[string]struct{}{
		"/etc/vnstat.conf":                       {},
		"/etc/default/vnstat":                    {},
		"/etc/conf.d/vnstat":                     {},
		"/etc/init.d/vnstat":                     {},
		"/etc/systemd/system/vnstat.service":     {},
		"/lib/systemd/system/vnstat.service":     {},
		"/usr/lib/systemd/system/vnstat.service": {},
		"/usr/bin/vnstat":                        {},
		"/usr/bin/vnstati":                       {},
		"/usr/sbin/vnstatd":                      {},
	}
	_, ok := allowedFiles[cleaned]
	return ok
}

func normalizeAbsolutePathList(items []string) []string {
	seen := make(map[string]struct{})
	result := make([]string, 0, len(items))
	for _, item := range items {
		path := strings.TrimSpace(item)
		if path == "" || !filepath.IsAbs(path) {
			continue
		}
		path = filepath.Clean(path)
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
	content, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return ""
	}
	values := parseOsReleaseFields(string(content))
	idLike := strings.ToLower(values["ID_LIKE"])
	id := strings.ToLower(values["ID"])
	switch {
	case strings.Contains(idLike, "debian") || id == "debian" || id == "ubuntu":
		return "debian"
	case strings.Contains(idLike, "rhel") || strings.Contains(idLike, "fedora") || id == "fedora" || id == "rhel" || id == "centos" || id == "rocky" || id == "almalinux":
		return "rhel"
	case strings.Contains(idLike, "suse") || id == "sles" || id == "opensuse":
		return "suse"
	case strings.Contains(idLike, "arch") || id == "arch":
		return "arch"
	case id == "alpine":
		return "alpine"
	default:
		return id
	}
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

func runVnstatCommand(args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "vnstat", args...)
	output, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return "", ctx.Err()
	}
	if err != nil {
		return "", fmt.Errorf("%w: %s", err, strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output)), nil
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
	var payload map[string]any
	dec := json.NewDecoder(strings.NewReader(output))
	dec.UseNumber()
	if err := dec.Decode(&payload); err != nil {
		return 0, 0, err
	}

	interfaces, ok := payload["interfaces"].([]any)
	if !ok || len(interfaces) == 0 {
		return 0, 0, errors.New("vnstat json does not contain interfaces")
	}

	first, ok := interfaces[0].(map[string]any)
	if !ok {
		return 0, 0, errors.New("vnstat json interface format is invalid")
	}

	traffic, ok := first["traffic"].(map[string]any)
	if !ok {
		return 0, 0, errors.New("vnstat json does not contain traffic totals")
	}

	for _, key := range []string{"total", "alltime"} {
		if totals, ok := traffic[key].(map[string]any); ok {
			rx, rxOK := numberFromAny(totals["rx"])
			tx, txOK := numberFromAny(totals["tx"])
			if rxOK && txOK {
				return tx, rx, nil
			}
		}
	}

	return 0, 0, errors.New("vnstat json traffic total is missing")
}

func numberFromAny(value any) (int64, bool) {
	switch v := value.(type) {
	case json.Number:
		parsed, err := v.Int64()
		if err == nil {
			return parsed, true
		}
		floatValue, err := v.Float64()
		if err == nil {
			return int64(floatValue), true
		}
	case float64:
		return int64(v), true
	case float32:
		return int64(v), true
	case int:
		return int64(v), true
	case int32:
		return int64(v), true
	case int64:
		return v, true
	case uint:
		return int64(v), true
	case uint32:
		return int64(v), true
	case uint64:
		if v <= maxInt64AsUint64 {
			return int64(v), true
		}
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(v), 10, 64)
		if err == nil {
			return parsed, true
		}
	}
	return 0, false
}

func uint64ToSafeInt64(value uint64) int64 {
	if value > maxInt64AsUint64 {
		return int64(maxInt64AsUint64)
	}
	return int64(value)
}
