package service

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/alireza0/s-ui/config"
	"github.com/alireza0/s-ui/logger"
)

const (
	panelUpdateRepo              = "nicelic/kwor"
	panelUpdateInstallScriptURL  = "https://raw.githubusercontent.com/nicelic/kwor/main/install.sh"
	panelUpdateMinimumVersion    = "v1.6.0"
	panelUpdatePerPage           = 20
	panelUpdateMaxPages          = 30
	panelUpdateVersionMaxLimit   = 20
	panelUpdateVersionMaxOffset  = panelUpdateMaxPages * panelUpdatePerPage
	panelUpdateVersionCacheMax   = 64
	panelUpdateVersionCacheTTL   = 10 * time.Minute
	panelUpdateSupportFileMaxLen = 512 * 1024
	panelUpdateServiceName       = "kwor"
	panelUpdateDefaultInstallDir = "/opt/kwor"
)

var (
	panelUpdateScriptPathPattern      = regexp.MustCompile(`(?:^|[\s"'=;{])((?:/[^/\s"'=;{}]+)+/apply-update\.sh)(?:$|[\s"';}])`)
	panelUpdateSystemdUnitNamePattern = regexp.MustCompile(`^kwor-panel-update-[0-9]+(?:\.service)?$`)
)

const (
	panelUpdateWorkspaceMarkerFileName = ".kwor-owner-v1"
	panelUpdateWorkspaceMarkerContent  = "kwor-owner:v1 resource=panel-update-workspace\n"
)

type panelUpdateSystemdLaunchResult struct {
	Started bool
	Output  string
	Err     error
}

type panelUpdateWorkerRollbackIncompleteError struct {
	cause error
}

func (e *panelUpdateWorkerRollbackIncompleteError) Error() string {
	if e == nil || e.cause == nil {
		return "panel update worker rollback is incomplete"
	}
	return e.cause.Error()
}

func (e *panelUpdateWorkerRollbackIncompleteError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.cause
}

var panelUpdateSystemdRunLookPathFn = exec.LookPath
var panelUpdateSystemdLauncherFn = launchPanelUpdateSystemdWorker
var panelUpdateSetSystemdUnitFn = func(operation *KworManagedOperationHandle, unit string) error {
	if operation == nil {
		return nil
	}
	return operation.SetSystemdUnit(unit)
}
var panelUpdateSystemdWorkerRollbackFn = stopStartedPanelUpdateSystemdWorker
var panelUpdateSystemdActiveFn = isPanelSystemdServiceActive
var panelUpdateDirectWorkerRollbackFn = stopStartedKworDetachedWorker
var panelUpdateNowFn = time.Now

type PanelUpdateService struct{}

type PanelUpdateStatus struct {
	LocalVersion            string                        `json:"localVersion"`
	BinaryPath              string                        `json:"binaryPath"`
	BinaryName              string                        `json:"binaryName"`
	InstallDir              string                        `json:"installDir"`
	ServiceFilePath         string                        `json:"serviceFilePath"`
	ServiceBinaryPath       string                        `json:"serviceBinaryPath"`
	RunningBinaryPath       string                        `json:"runningBinaryPath"`
	InstallSource           string                        `json:"installSource"`
	Platform                string                        `json:"platform"`
	CanRestart              bool                          `json:"canRestart"`
	RestartHint             string                        `json:"restartHint,omitempty"`
	CanInstall              bool                          `json:"canInstall"`
	InstallHint             string                        `json:"installHint,omitempty"`
	CanUninstall            bool                          `json:"canUninstall"`
	UninstallHint           string                        `json:"uninstallHint,omitempty"`
	UninstallMode           PanelUninstallMode            `json:"uninstallMode"`
	UninstallState          string                        `json:"uninstallState"`
	UninstallPhase          string                        `json:"uninstallPhase,omitempty"`
	UninstallError          string                        `json:"uninstallError,omitempty"`
	UninstallFailures       []string                      `json:"uninstallFailures,omitempty"`
	UninstallWarnings       []string                      `json:"uninstallWarnings,omitempty"`
	UninstallCanRetry       bool                          `json:"uninstallCanRetry"`
	DockerUninstallCommands []PanelDockerUninstallCommand `json:"dockerUninstallCommands,omitempty"`
	LastUpdateLogPath       string                        `json:"lastUpdateLogPath,omitempty"`
	LastUpdateError         string                        `json:"lastUpdateError,omitempty"`
	UpdateTask              *ManagedDownloadTaskStatus    `json:"updateTask,omitempty"`
}

type PanelVersionItem struct {
	TagName     string `json:"tag_name"`
	Name        string `json:"name"`
	Prerelease  bool   `json:"prerelease"`
	PublishedAt string `json:"published_at"`
	AssetName   string `json:"asset_name,omitempty"`
	AssetSize   int64  `json:"asset_size,omitempty"`
}

type PanelVersionListResponse struct {
	Versions []PanelVersionItem `json:"versions"`
	Offset   int                `json:"offset"`
	Limit    int                `json:"limit"`
	Page     int                `json:"page"`
	PerPage  int                `json:"per_page"`
	HasMore  bool               `json:"has_more"`
}

type PanelInstallResult struct {
	Version    string `json:"version"`
	BinaryPath string `json:"binaryPath"`
	Started    bool   `json:"started"`
	Message    string `json:"message"`
}

type PanelUpdateLogView struct {
	Path     string   `json:"path"`
	Exists   bool     `json:"exists"`
	Lines    []string `json:"lines"`
	TooLong  bool     `json:"tooLong"`
	Modified int64    `json:"modified"`
}

type panelUpdateVersionCacheEntry struct {
	createdAt time.Time
	expiresAt time.Time
	response  PanelVersionListResponse
}

var panelUpdateVersionCache = struct {
	sync.Mutex
	items map[string]panelUpdateVersionCacheEntry
}{
	items: make(map[string]panelUpdateVersionCacheEntry),
}

var panelUpdateMu sync.Mutex
var panelUpdateTaskManager = NewManagedDownloadTaskManager("panel update")

const panelUpdateLogMaxBytes = 128 * 1024
const panelUpdateLogMaxLines = 300

func (s *PanelUpdateService) GetStatus() (*PanelUpdateStatus, error) {
	binaryPath, runningPath, servicePath, serviceBinPath, source := resolvePanelUpdateBinaryPath()
	installDir := filepath.Dir(binaryPath)
	canInstall, installHint := panelUpdateInstallSupport()
	canRestart, restartHint := PanelRestartSupport()
	uninstallStatus := GetPanelUninstallStatus()
	lastUpdateLogPath := filepath.Join(config.GetRuntimeSupportDir(), "panel-update-last.log")
	lastUpdateError := readPanelUpdateLastError(lastUpdateLogPath)
	updateTask := panelUpdateTaskManager.Get("")
	var updateTaskView *ManagedDownloadTaskStatus
	if updateTask.State != managedDownloadTaskIdle {
		copy := cloneManagedDownloadTaskStatus(updateTask)
		updateTaskView = &copy
	}

	return &PanelUpdateStatus{
		LocalVersion:            config.GetVersion(),
		BinaryPath:              binaryPath,
		BinaryName:              filepath.Base(binaryPath),
		InstallDir:              installDir,
		ServiceFilePath:         servicePath,
		ServiceBinaryPath:       serviceBinPath,
		RunningBinaryPath:       runningPath,
		InstallSource:           source,
		Platform:                fmt.Sprintf("%s/%s", GetSystemPlatformOS(), GetSystemPlatformArchitecture()),
		CanRestart:              canRestart,
		RestartHint:             restartHint,
		CanInstall:              canInstall,
		InstallHint:             installHint,
		CanUninstall:            uninstallStatus.Mode == PanelUninstallModeNative && uninstallStatus.CanSchedule,
		UninstallHint:           uninstallStatus.Hint,
		UninstallMode:           uninstallStatus.Mode,
		UninstallState:          uninstallStatus.State,
		UninstallPhase:          uninstallStatus.Phase,
		UninstallError:          uninstallStatus.Error,
		UninstallFailures:       uninstallStatus.Failures,
		UninstallWarnings:       uninstallStatus.Warnings,
		UninstallCanRetry:       uninstallStatus.CanRetry,
		DockerUninstallCommands: uninstallStatus.DockerCommands,
		LastUpdateLogPath:       lastUpdateLogPath,
		LastUpdateError:         lastUpdateError,
		UpdateTask:              updateTaskView,
	}, nil
}

func (s *PanelUpdateService) GetRemoteVersions(offset int, limit int) (*PanelVersionListResponse, error) {
	return s.GetRemoteVersionsContext(context.Background(), offset, limit)
}

func (s *PanelUpdateService) GetRemoteVersionsContext(ctx context.Context, offset int, limit int) (*PanelVersionListResponse, error) {
	offset, limit = normalizePanelVersionWindow(offset, limit)
	cacheKey := fmt.Sprintf("%s|%d|%d", panelUpdateRepo, offset, limit)
	if cached, ok := getPanelVersionCache(cacheKey); ok {
		return cached, nil
	}

	client := &http.Client{Timeout: 30 * time.Second}
	result := &PanelVersionListResponse{
		Versions: make([]PanelVersionItem, 0, limit+1),
		Offset:   offset,
		Limit:    limit,
		PerPage:  limit,
		Page:     offset/limit + 1,
	}

	seenTags := make(map[string]struct{})
	matchedCount := 0
	for apiPage := 1; apiPage <= panelUpdateMaxPages && len(result.Versions) < limit+1; apiPage++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		releases, err := fetchGitHubReleasePageForRepoContext(ctx, panelUpdateRepo, client, apiPage, panelUpdatePerPage, coreGitHubResponseMaxBytes)
		if err != nil {
			return nil, err
		}
		if len(releases) == 0 {
			break
		}

		for _, release := range releases {
			if _, ok := seenTags[release.TagName]; ok {
				continue
			}
			seenTags[release.TagName] = struct{}{}
			if !isPanelUpdateVersionSelectable(release.TagName) {
				continue
			}

			asset, ok := pickPanelReleaseAsset(release.Assets)
			if !ok {
				continue
			}
			if matchedCount < offset {
				matchedCount++
				continue
			}
			matchedCount++

			result.Versions = append(result.Versions, PanelVersionItem{
				TagName:     release.TagName,
				Name:        release.Name,
				Prerelease:  release.Prerelease,
				PublishedAt: release.PublishedAt,
				AssetName:   asset.Name,
				AssetSize:   asset.Size,
			})
			if len(result.Versions) >= limit+1 {
				break
			}
		}

		if len(releases) < panelUpdatePerPage {
			break
		}
	}

	if len(result.Versions) > limit {
		result.HasMore = true
		result.Versions = result.Versions[:limit]
	}
	setPanelVersionCache(cacheKey, result)
	return clonePanelVersionListResponse(result), nil
}

func (s *PanelUpdateService) Install(version string) (*PanelInstallResult, error) {
	panelUpdateMu.Lock()
	defer panelUpdateMu.Unlock()
	if supported, reason := panelUpdateInstallSupport(); !supported {
		return nil, fmt.Errorf("%s", reason)
	}
	lifecycleLock, err := AcquireKworLifecycleLock()
	if err != nil {
		return nil, err
	}
	defer func() { _ = lifecycleLock.Release() }()
	operationContext, operation, err := BeginKworManagedOperation("panel-update")
	if err != nil {
		return nil, err
	}
	result, detached, installErr := s.installWithContext(operationContext, operation, version, nil, nil)
	if detached {
		operation.Detach()
	} else {
		operation.Done()
	}
	return result, installErr
}

func (s *PanelUpdateService) StartManagedInstall(version string) (ManagedDownloadTaskStatus, error) {
	version = normalizePanelVersionTag(version)
	if version == "" {
		return ManagedDownloadTaskStatus{}, fmt.Errorf("version is required")
	}
	if !isPanelUpdateVersionSelectable(version) {
		return ManagedDownloadTaskStatus{}, fmt.Errorf("only %s and later panel versions can be installed", panelUpdateMinimumVersion)
	}
	if supported, reason := panelUpdateInstallSupport(); !supported {
		return ManagedDownloadTaskStatus{}, fmt.Errorf("%s", reason)
	}
	handle, status, created, err := panelUpdateTaskManager.Start("panel-update", "version|"+version)
	if err != nil || !created {
		return status, err
	}
	go func() {
		defer finishManagedDownloadTaskPanic(handle, "failed", "panel update")
		if !handle.MarkRunning("preparing") {
			handle.FinishCancelled("cancelled")
			return
		}
		if lockErr := lockManagedDownloadTaskMutex(handle.Context(), &panelUpdateMu); lockErr != nil {
			handle.FinishCancelled("cancelled")
			return
		}
		defer panelUpdateMu.Unlock()
		ctx := handle.Context()
		if err := ctx.Err(); err != nil {
			handle.FinishCancelled("cancelled")
			return
		}
		lifecycleLock, lockErr := AcquireKworLifecycleLockContext(ctx)
		if lockErr != nil {
			handle.FinishError("failed", lockErr)
			return
		}
		defer func() { _ = lifecycleLock.Release() }()
		if err := ctx.Err(); err != nil {
			handle.FinishCancelled("cancelled")
			return
		}
		_, detached, installErr := s.installWithContext(ctx, handle.Operation(), version, func(phase string) {
			handle.SetPhase(phase, true)
		}, func() bool {
			return handle.BeginApplying("handoff")
		})
		if detached {
			handle.DetachOperation()
		}
		if installErr != nil {
			if errors.Is(ctx.Err(), context.Canceled) && !detached {
				handle.FinishCancelled("cancelled")
			} else {
				handle.FinishError("failed", installErr)
			}
			return
		}
		if detached {
			// The updater now owns the detached managed operation. The browser-facing
			// preparation task must still become terminal, otherwise it would keep the
			// single-resource slot occupied until process restart.
			handle.FinishSuccess("handoff")
			return
		}
		handle.FinishSuccess("completed")
	}()
	return status, nil
}

func (s *PanelUpdateService) StopManagedInstall(id string) (ManagedDownloadTaskStatus, error) {
	return panelUpdateTaskManager.Stop(id)
}

func (s *PanelUpdateService) installWithContext(operationContext context.Context, operation *KworManagedOperationHandle, version string, report func(string), beginApplying func() bool) (*PanelInstallResult, bool, error) {

	version = normalizePanelVersionTag(version)
	if version == "" {
		return nil, false, fmt.Errorf("version is required")
	}
	if !isPanelUpdateVersionSelectable(version) {
		return nil, false, fmt.Errorf("only %s and later panel versions can be installed", panelUpdateMinimumVersion)
	}
	if report != nil {
		report("preparing")
	}

	status, err := s.GetStatus()
	if err != nil {
		return nil, false, err
	}
	if strings.TrimSpace(status.BinaryPath) == "" {
		return nil, false, fmt.Errorf("failed to resolve current panel binary path")
	}
	if _, statErr := os.Lstat(status.BinaryPath + ".bak"); statErr == nil {
		return nil, false, fmt.Errorf("refuse to overwrite an existing panel backup: %s", status.BinaryPath+".bak")
	} else if !os.IsNotExist(statErr) {
		return nil, false, fmt.Errorf("inspect panel backup path failed: %v", statErr)
	}

	if report != nil {
		report("downloading")
	}
	downloadURL, err := getPanelReleaseAssetURLContext(operationContext, version)
	if err != nil {
		return nil, false, err
	}

	workDir, err := os.MkdirTemp("", "kwor-panel-update-")
	if err != nil {
		return nil, false, fmt.Errorf("create update work dir failed: %v", err)
	}
	workspaceID := "panel-update-workspace-" + filepath.Base(workDir)
	workspaceOwnership, err := BeginUpdateWorkspaceOwnership(workspaceID, workDir, status.BinaryPath)
	if err != nil {
		cleanupPanelUpdateWorkDir(workDir)
		return nil, false, fmt.Errorf("record update workspace ownership failed: %v", err)
	}
	archivePath := filepath.Join(workDir, filepath.Base(downloadURL))
	stagedBinPath := filepath.Join(workDir, "kwor")
	stagedInstallScriptPath := filepath.Join(workDir, "install.sh")
	stagedServiceFilePath := filepath.Join(workDir, "kwor.service")
	cleanupWorkDir := true
	defer func() {
		if cleanupWorkDir {
			cleanupPanelUpdateWorkDir(workDir)
			if workspaceOwnership.ID != "" && !pathExists(workDir) {
				_ = RemoveHostResource(workspaceOwnership.ID)
			}
		}
	}()
	if err := os.WriteFile(filepath.Join(workDir, panelUpdateWorkspaceMarkerFileName), []byte(panelUpdateWorkspaceMarkerContent), 0o600); err != nil {
		return nil, false, fmt.Errorf("write update workspace ownership marker failed: %v", err)
	}

	if err := downloadPanelReleaseArchiveContext(operationContext, downloadURL, archivePath); err != nil {
		return nil, false, err
	}
	if report != nil {
		report("extracting")
	}
	if err := extractPanelReleasePayloadContext(operationContext, archivePath, stagedBinPath, stagedInstallScriptPath); err != nil {
		return nil, false, err
	}
	_ = os.Remove(archivePath)
	if err := downloadPanelLatestInstallScriptContext(operationContext, stagedInstallScriptPath); err != nil {
		if ctxErr := operationContext.Err(); ctxErr != nil {
			return nil, false, ctxErr
		}
		logger.Warning("download latest panel install.sh failed, fallback to release packaged script: ", err)
	}
	if err := operationContext.Err(); err != nil {
		return nil, false, err
	}
	if _, err := os.Stat(stagedInstallScriptPath); err != nil {
		stagedInstallScriptPath = ""
	} else if err := os.Chmod(stagedInstallScriptPath, 0o755); err != nil {
		return nil, false, fmt.Errorf("chmod staged install script failed: %v", err)
	}
	if err := os.WriteFile(stagedServiceFilePath, []byte(BuildPanelSystemdServiceContent(status.BinaryPath)), 0o644); err != nil {
		return nil, false, fmt.Errorf("write staged systemd service failed: %v", err)
	}
	if err := os.Chmod(stagedBinPath, 0o755); err != nil {
		return nil, false, fmt.Errorf("chmod staged binary failed: %v", err)
	}
	if report != nil {
		report("validating")
	}
	if err := validatePanelBinaryContext(operationContext, stagedBinPath); err != nil {
		return nil, false, err
	}

	installScriptPath, err := writePanelUpdateScriptWithOperation(workDir, status.BinaryPath, stagedBinPath, stagedInstallScriptPath, stagedServiceFilePath, status.BinaryName, operation.ID())
	if err != nil {
		return nil, false, err
	}
	if workspaceOwnership.ID != "" {
		if err := ActivateHostResource(workspaceOwnership.ID); err != nil {
			return nil, false, fmt.Errorf("activate update workspace ownership failed: %v", err)
		}
	}
	cleanupWorkDir = false
	clearPanelUpdateLastError(status.LastUpdateLogPath)

	if beginApplying != nil && !beginApplying() {
		cleanupWorkDir = true
		return nil, false, coreDownloadTaskCancelledError(operationContext)
	}
	if report != nil {
		report("handoff")
	}
	if err := operation.SetBlockNewWork(true); err != nil {
		cleanupWorkDir = true
		return nil, false, fmt.Errorf("block new work while handing off panel update: %w", err)
	}
	// The preparation task is complete once the updater is handed off. Launch
	// the detached worker with the operation context, not the task child
	// context: FinishSuccess cancels that child context to release the task
	// slot, while the updater must keep running until it completes the swap.
	workerContext := panelUpdateWorkerContext(operationContext, operation)
	if err := startPanelUpdateWorker(workerContext, operation, installScriptPath); err != nil {
		if panelUpdateWorkerRollbackIncomplete(err) {
			cleanupWorkDir = false
			return nil, true, err
		} else {
			cleanupWorkDir = true
		}
		return nil, false, err
	}

	return &PanelInstallResult{
		Version:    version,
		BinaryPath: status.BinaryPath,
		Started:    true,
		Message:    "更新任务已启动，面板会自动停止、替换并重新启动",
	}, true, nil
}

func (s *PanelUpdateService) GetLastUpdateLog() (*PanelUpdateLogView, error) {
	status, err := s.GetStatus()
	if err != nil {
		return nil, err
	}
	return loadPanelUpdateLogView(status.LastUpdateLogPath)
}

func normalizePanelVersionWindow(offset int, limit int) (int, int) {
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 {
		limit = 5
	}
	if limit > panelUpdateVersionMaxLimit {
		limit = panelUpdateVersionMaxLimit
	}
	if offset > panelUpdateVersionMaxOffset {
		offset = panelUpdateVersionMaxOffset
	}
	return offset, limit
}

func panelUpdateInstallSupport() (bool, string) {
	if !IsSystemPlatformLinux() {
		return false, "面板内升级仅支持 Linux 宿主机部署"
	}
	if runningInsideContainer() {
		return false, "Docker/容器部署不支持面板内直接升级，请改为拉取新镜像并重建容器"
	}
	if os.Geteuid() != 0 {
		return false, "面板内升级需要 root 权限"
	}
	return true, ""
}

func normalizePanelVersionTag(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	if strings.HasPrefix(raw, "v") {
		return raw
	}
	return "v" + raw
}

func isPanelUpdateVersionSelectable(version string) bool {
	return compareSemverLikeTags(version, panelUpdateMinimumVersion) >= 0
}

func pickPanelReleaseAsset(assets []GitHubAsset) (GitHubAsset, bool) {
	targetName := fmt.Sprintf("kwor-linux-%s.tar.gz", panelUpdateArchName())
	for _, asset := range assets {
		if asset.Name == targetName {
			return asset, true
		}
	}
	return GitHubAsset{}, false
}

func panelUpdateArchName() string {
	switch GetSystemPlatformArchitecture() {
	case "amd64":
		return "amd64"
	case "arm64":
		return "arm64"
	default:
		return GetSystemPlatformArchitecture()
	}
}

func getPanelReleaseAssetURL(version string) (string, error) {
	return getPanelReleaseAssetURLContext(context.Background(), version)
}

func getPanelReleaseAssetURLContext(ctx context.Context, version string) (string, error) {
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/releases/tags/%s", panelUpdateRepo, version)
	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "kwor")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request GitHub API failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	var release GitHubRelease
	if err := unmarshalBoundedHTTPResponseJSON(resp.Body, coreGitHubResponseMaxBytes, &release); err != nil {
		return "", fmt.Errorf("failed to parse GitHub release: %v", err)
	}
	asset, ok := pickPanelReleaseAsset(release.Assets)
	if !ok {
		return "", fmt.Errorf("kwor release asset for linux/%s not found in %s", panelUpdateArchName(), version)
	}
	return asset.BrowserDownloadURL, nil
}

func downloadPanelReleaseArchive(downloadURL string, archivePath string) error {
	return downloadPanelReleaseArchiveContext(context.Background(), downloadURL, archivePath)
}

func downloadPanelReleaseArchiveContext(ctx context.Context, downloadURL string, archivePath string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	client := &http.Client{Timeout: 600 * time.Second}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, downloadURL, nil)
	if err != nil {
		return fmt.Errorf("create release archive request failed: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("download release archive failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download release archive failed, HTTP %d", resp.StatusCode)
	}

	tmpPath := archivePath + ".tmp"
	out, err := os.Create(tmpPath)
	if err != nil {
		return fmt.Errorf("create release archive file failed: %v", err)
	}
	if _, err = copyManagedDownloadTaskContext(ctx, out, resp.Body); err != nil {
		_ = out.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("write release archive failed: %v", err)
	}
	if err = out.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("close release archive failed: %v", err)
	}
	if err = ctx.Err(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if err = os.Rename(tmpPath, archivePath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("commit release archive failed: %v", err)
	}
	return nil
}

func cleanupPanelUpdateWorkDir(workDir string) {
	workDir = filepath.Clean(strings.TrimSpace(workDir))
	if !panelUpdateWorkspacePathStrict(workDir) {
		return
	}
	_ = os.RemoveAll(workDir)
}

func panelUpdateScriptPathFromCommandText(command string) string {
	matches := panelUpdateScriptPathPattern.FindStringSubmatch(command)
	if len(matches) != 2 {
		return ""
	}
	path := filepath.Clean(strings.TrimSpace(matches[1]))
	if !panelUpdateWorkspacePathStrict(filepath.Dir(path)) || filepath.Base(path) != "apply-update.sh" {
		return ""
	}
	return path
}

func panelUpdateWorkspacePathStrict(path string) bool {
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "" || path == "." || path == string(os.PathSeparator) {
		return false
	}
	base := filepath.Base(path)
	if !strings.HasPrefix(base, "kwor-panel-update-") || len(base) == len("kwor-panel-update-") {
		return false
	}
	parent := filepath.Clean(filepath.Dir(path))
	return parent == filepath.Clean(os.TempDir()) || parent == filepath.Clean("/tmp")
}

func panelUpdateWorkspaceScriptOwned(scriptPath string) bool {
	scriptPath = filepath.Clean(strings.TrimSpace(scriptPath))
	if filepath.Base(scriptPath) != "apply-update.sh" || !panelUpdateWorkspacePathStrict(filepath.Dir(scriptPath)) {
		return false
	}
	info, err := os.Lstat(scriptPath)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return false
	}
	raw, err := os.ReadFile(scriptPath)
	return err == nil && strings.Contains(string(raw), "# "+strings.TrimSpace(panelUpdateWorkspaceMarkerContent))
}

func panelUpdateWorkspaceDirectoryOwned(workDir string) bool {
	workDir = filepath.Clean(strings.TrimSpace(workDir))
	if !panelUpdateWorkspacePathStrict(workDir) {
		return false
	}
	markerPath := filepath.Join(workDir, panelUpdateWorkspaceMarkerFileName)
	if info, err := os.Lstat(markerPath); err == nil && info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
		if raw, readErr := os.ReadFile(markerPath); readErr == nil && string(raw) == panelUpdateWorkspaceMarkerContent {
			return true
		}
	}
	return panelUpdateWorkspaceScriptOwned(filepath.Join(workDir, "apply-update.sh"))
}

func readPanelUpdateLastError(logPath string) string {
	logPath = strings.TrimSpace(logPath)
	if logPath == "" {
		return ""
	}
	content, err := os.ReadFile(logPath)
	if err != nil {
		return ""
	}
	lines := strings.Split(strings.ReplaceAll(string(content), "\r\n", "\n"), "\n")
	trimmed := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		trimmed = append(trimmed, line)
	}
	if len(trimmed) == 0 {
		return ""
	}
	if len(trimmed) > 4 {
		trimmed = trimmed[len(trimmed)-4:]
	}
	return strings.Join(trimmed, " | ")
}

func clearPanelUpdateLastError(logPath string) {
	logPath = strings.TrimSpace(logPath)
	if logPath == "" {
		return
	}
	_ = os.Remove(logPath)
}

func loadPanelUpdateLogView(logPath string) (*PanelUpdateLogView, error) {
	logPath = strings.TrimSpace(logPath)
	view := &PanelUpdateLogView{
		Path:   logPath,
		Exists: false,
		Lines:  []string{},
	}
	if logPath == "" {
		return view, nil
	}
	info, err := os.Stat(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			return view, nil
		}
		return nil, err
	}

	view.Exists = true
	view.Modified = info.ModTime().Unix()

	content, tooLong, err := readPanelUpdateLogBytes(logPath, panelUpdateLogMaxBytes)
	if err != nil {
		return nil, err
	}
	view.TooLong = tooLong
	view.Lines = normalizePanelUpdateLogLines(content, panelUpdateLogMaxLines)
	return view, nil
}

func readPanelUpdateLogBytes(logPath string, maxBytes int64) ([]byte, bool, error) {
	f, err := os.Open(logPath)
	if err != nil {
		return nil, false, err
	}
	defer f.Close()

	reader := io.LimitReader(f, maxBytes+1)
	content, err := io.ReadAll(reader)
	if err != nil {
		return nil, false, err
	}
	if int64(len(content)) > maxBytes {
		return content[:maxBytes], true, nil
	}
	return content, false, nil
}

func normalizePanelUpdateLogLines(content []byte, maxLines int) []string {
	text := strings.ReplaceAll(string(content), "\r\n", "\n")
	rawLines := strings.Split(text, "\n")
	lines := make([]string, 0, len(rawLines))
	for _, line := range rawLines {
		line = strings.TrimRight(line, "\r")
		if line == "" && len(lines) == 0 {
			continue
		}
		lines = append(lines, line)
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) == 0 {
		return []string{"暂无日志"}
	}
	if len(lines) > maxLines {
		lines = append([]string{"日志过长，已隐藏较早输出"}, lines[len(lines)-maxLines:]...)
	}
	return lines
}

func extractPanelReleasePayload(archivePath string, stagedBinPath string, stagedInstallScriptPath string) error {
	return extractPanelReleasePayloadContext(context.Background(), archivePath, stagedBinPath, stagedInstallScriptPath)
}

func extractPanelReleasePayloadContext(ctx context.Context, archivePath string, stagedBinPath string, stagedInstallScriptPath string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	f, err := os.Open(archivePath)
	if err != nil {
		return err
	}
	defer f.Close()

	gzr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gzr.Close()

	tr := tar.NewReader(gzr)
	foundBinary := false
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if header.Typeflag != tar.TypeReg {
			continue
		}
		switch filepath.Base(header.Name) {
		case "kwor":
			if err := writePanelTarFileContext(ctx, tr, stagedBinPath); err != nil {
				return err
			}
			foundBinary = true
		case "install.sh":
			if strings.TrimSpace(stagedInstallScriptPath) != "" {
				if err := writePanelTarFileContext(ctx, tr, stagedInstallScriptPath); err != nil {
					return err
				}
			}
		}
	}
	if !foundBinary {
		return fmt.Errorf("release archive does not contain kwor binary")
	}
	return nil
}

func writePanelTarFile(reader io.Reader, targetPath string) error {
	return writePanelTarFileContext(context.Background(), reader, targetPath)
}

func writePanelTarFileContext(ctx context.Context, reader io.Reader, targetPath string) error {
	out, err := os.Create(targetPath)
	if err != nil {
		return err
	}
	if _, err = copyManagedDownloadTaskContext(ctx, out, reader); err != nil {
		_ = out.Close()
		_ = os.Remove(targetPath)
		return err
	}
	if err = out.Close(); err != nil {
		_ = os.Remove(targetPath)
		return err
	}
	return nil
}

func downloadPanelLatestInstallScript(targetPath string) error {
	return downloadPanelLatestInstallScriptContext(context.Background(), targetPath)
}

func downloadPanelLatestInstallScriptContext(ctx context.Context, targetPath string) error {
	targetPath = strings.TrimSpace(targetPath)
	if targetPath == "" {
		return fmt.Errorf("target install script path is empty")
	}
	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "GET", panelUpdateInstallScriptURL, nil)
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
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	tmpPath := targetPath + ".download"
	out, err := os.Create(tmpPath)
	if err != nil {
		return err
	}
	written, copyErr := copyManagedDownloadTaskContext(ctx, out, io.LimitReader(resp.Body, panelUpdateSupportFileMaxLen+1))
	closeErr := out.Close()
	if copyErr != nil {
		_ = os.Remove(tmpPath)
		return copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmpPath)
		return closeErr
	}
	if written > panelUpdateSupportFileMaxLen {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("install script is too large")
	}
	if err := ctx.Err(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	content, err := os.ReadFile(tmpPath)
	if err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	if !strings.Contains(string(content), `GH_REPO="nicelic/kwor"`) {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("downloaded install script failed validation")
	}
	if err := os.Rename(tmpPath, targetPath); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return nil
}

func validatePanelBinary(binPath string) error {
	return validatePanelBinaryContext(context.Background(), binPath)
}

func validatePanelBinaryContext(parent context.Context, binPath string) error {
	ctx, cancel := context.WithTimeout(parent, 8*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binPath, "-v")
	cmd.Dir = filepath.Dir(binPath)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	PrepareKworManagedCommandContext(parent, cmd)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("downloaded kwor binary is not executable: %v", err)
	}
	if err := TrackKworManagedCommandContext(parent, cmd); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return fmt.Errorf("record downloaded kwor binary validation process: %w", err)
	}
	err := cmd.Wait()
	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("downloaded kwor binary validation timed out")
	}
	if err != nil {
		return fmt.Errorf("downloaded kwor binary is not executable: %v: %s", err, strings.TrimSpace(output.String()))
	}
	return nil
}

func panelUpdateWorkerContext(preparationContext context.Context, operation *KworManagedOperationHandle) context.Context {
	if operation != nil && operation.Context() != nil {
		return operation.Context()
	}
	if preparationContext != nil {
		return preparationContext
	}
	return context.Background()
}

func startPanelUpdateWorker(ctx context.Context, operation *KworManagedOperationHandle, scriptPath string) error {
	if _, err := panelUpdateSystemdRunLookPathFn("systemd-run"); err == nil {
		unitName := fmt.Sprintf("kwor-panel-update-%d", panelUpdateNowFn().UnixNano())
		operationID := ""
		if operation != nil {
			operationID = operation.ID()
			if setErr := panelUpdateSetSystemdUnitFn(operation, unitName); setErr != nil {
				return fmt.Errorf("record panel update worker unit: %w", setErr)
			}
		}
		systemdArgs := []string{
			"--unit", unitName,
			"--collect",
			"--description", "kwor panel update",
			"--working-directory=" + filepath.Dir(scriptPath),
		}
		if operationID != "" {
			systemdArgs = append(systemdArgs, "--setenv="+kworLifecycleOperationIDEnv+"="+operationID)
		}
		systemdArgs = append(systemdArgs, "bash", scriptPath)
		result := panelUpdateSystemdLauncherFn(ctx, systemdArgs)
		if result.Err == nil {
			return nil
		}
		if result.Started {
			if rollbackErr := panelUpdateSystemdWorkerRollbackFn(unitName, operationID); rollbackErr != nil {
				return &panelUpdateWorkerRollbackIncompleteError{cause: fmt.Errorf(
					"start panel update worker with systemd-run failed: %v; stop started unit failed: %w",
					result.Err,
					rollbackErr,
				)}
			}
		}
		if clearErr := panelUpdateSetSystemdUnitFn(operation, ""); clearErr != nil {
			return fmt.Errorf("clear panel update worker unit after launch failure: %w", clearErr)
		}
		message := strings.TrimSpace(result.Output)
		if panelUpdateSystemdActiveFn() {
			if message != "" {
				return fmt.Errorf("start panel update worker with systemd-run failed: %v: %s", result.Err, message)
			}
			return fmt.Errorf("start panel update worker with systemd-run failed: %v", result.Err)
		}
		logger.Warning("systemd-run panel update worker failed, fallback to detached process: ", result.Err, " ", message)
	}

	cmd := exec.CommandContext(ctx, "bash", scriptPath)
	cmd.Dir = filepath.Dir(scriptPath)
	cmd.Stdout = nil
	cmd.Stderr = nil
	PrepareKworManagedCommandContext(ctx, cmd)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start panel update worker failed: %v", err)
	}
	startTime, identityErr := kworLifecycleProcessStartTime(cmd.Process.Pid)
	if identityErr != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return fmt.Errorf("read panel update worker identity: %w", identityErr)
	}
	pgid := kworManagedOperationProcessGroup(cmd.Process.Pid)
	if err := TrackKworManagedCommandContext(ctx, cmd); err != nil {
		if rollbackErr := panelUpdateDirectWorkerRollbackFn(cmd.Process.Pid, pgid, startTime); rollbackErr != nil {
			_ = cmd.Process.Release()
			return &panelUpdateWorkerRollbackIncompleteError{cause: fmt.Errorf(
				"record panel update worker process: %v; stop started process failed: %w",
				err,
				rollbackErr,
			)}
		}
		_ = cmd.Wait()
		return fmt.Errorf("record panel update worker process: %w", err)
	}
	if err := cmd.Process.Release(); err != nil {
		logger.Warning("release panel update worker process failed: ", err)
	}
	return nil
}

func launchPanelUpdateSystemdWorker(ctx context.Context, systemdArgs []string) panelUpdateSystemdLaunchResult {
	cmd := exec.CommandContext(ctx, "systemd-run", systemdArgs...)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	PrepareKworManagedCommandContext(ctx, cmd)
	if err := cmd.Start(); err != nil {
		return panelUpdateSystemdLaunchResult{Output: output.String(), Err: err}
	}
	if err := TrackKworManagedCommandContext(ctx, cmd); err != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return panelUpdateSystemdLaunchResult{Started: true, Output: output.String(), Err: fmt.Errorf("record panel update launcher process: %w", err)}
	}
	runErr := cmd.Wait()
	return panelUpdateSystemdLaunchResult{Started: true, Output: output.String(), Err: runErr}
}

func stopStartedPanelUpdateSystemdWorker(unit string, operationID string) error {
	systemctlPath, err := exec.LookPath("systemctl")
	if err != nil {
		return fmt.Errorf("find systemctl while rolling back panel update worker: %w", err)
	}
	owned, err := verifyKworPanelUpdateSystemdUnit(systemctlPath, unit, operationID, strings.TrimSpace(operationID) != "")
	if err != nil {
		return fmt.Errorf("verify panel update worker unit before rollback: %w", err)
	}
	if !owned {
		return fmt.Errorf("refuse to stop unverified panel update worker unit: %s", unit)
	}
	return stopSystemdUnitForUninstall(unit, false)
}

func panelUpdateWorkerRollbackIncomplete(err error) bool {
	var target *panelUpdateWorkerRollbackIncompleteError
	return errors.As(err, &target)
}

func isPanelSystemdServiceActive() bool {
	if _, err := exec.LookPath("systemctl"); err != nil {
		return false
	}
	return runCommandWithTimeout(shortSystemCommandTimeout, "systemctl", "is-active", "--quiet", panelUpdateServiceName) == nil
}

func BuildPanelSystemdServiceContent(binPath string) string {
	binPath = strings.TrimSpace(binPath)
	binDir := filepath.Dir(binPath)
	return fmt.Sprintf(`# kwor-owner:v1 resource=panel-systemd
[Unit]
Description=kwor Service
After=network.target nss-lookup.target

[Service]
Type=simple
Environment=%s
ExecCondition=%s
WorkingDirectory=%s
ExecStart=%s
Restart=on-failure
RestartSec=5s
LimitNOFILE=infinity

[Install]
WantedBy=multi-user.target
`,
		quoteSystemdEnvironmentAssignment(InternalSystemdCommandEnv, "1"),
		buildSystemdExecCommand(binPath, "lifecycle-guard"),
		escapeSystemdUnitValue(binDir),
		buildSystemdExecCommand(binPath),
	)
}

func writePanelUpdateScript(workDir string, targetBinPath string, stagedBinPath string, stagedInstallScriptPath string, stagedServiceFilePath string, binaryName string) (string, error) {
	return writePanelUpdateScriptWithOperation(workDir, targetBinPath, stagedBinPath, stagedInstallScriptPath, stagedServiceFilePath, binaryName, "")
}

func writePanelUpdateScriptWithOperation(workDir string, targetBinPath string, stagedBinPath string, stagedInstallScriptPath string, stagedServiceFilePath string, binaryName string, operationID string) (string, error) {
	scriptPath := filepath.Join(workDir, "apply-update.sh")
	backupPath := targetBinPath + ".bak"
	logPath := filepath.Join(workDir, "apply-update.log")
	lastLogPath := filepath.Join(config.GetRuntimeSupportDir(), "panel-update-last.log")
	installSupportPath := config.GetRuntimeInstallScriptPath()
	serviceSupportPath := config.GetRuntimeServiceFilePath()

	script := fmt.Sprintf(`#!/usr/bin/env bash
# kwor-owner:v1 resource=panel-update-workspace
set -u

TARGET_BIN=%s
STAGED_BIN=%s
STAGED_INSTALL_SH=%s
STAGED_SERVICE_FILE=%s
BACKUP_BIN=%s
WORK_DIR=%s
LOG_PATH=%s
LAST_LOG_PATH=%s
SERVICE_NAME=%s
BINARY_NAME=%s
OPERATION_ID=%s
INSTALL_DIR="$(dirname "$TARGET_BIN")"
SERVICE_FILE="/etc/systemd/system/${SERVICE_NAME}.service"
UPDATE_SUCCESS=0
CLEANUP_DONE=0
OWNER_MARKER="$WORK_DIR/.kwor-owner-v1"

cleanup() {
	if [[ "$CLEANUP_DONE" -eq 1 ]]; then
		return
	fi
	CLEANUP_DONE=1
  if [[ "$UPDATE_SUCCESS" -eq 0 && -f "$LOG_PATH" ]]; then
    cp -f "$LOG_PATH" "$LAST_LOG_PATH" 2>/dev/null || true
    chmod 600 "$LAST_LOG_PATH" 2>/dev/null || true
  else
    rm -f "$LAST_LOG_PATH"
  fi
  rm -f "$STAGED_BIN"
  if [[ -n "$STAGED_INSTALL_SH" ]]; then
    rm -f "$STAGED_INSTALL_SH"
  fi
  rm -f "$STAGED_SERVICE_FILE"
  if [[ -n "$OPERATION_ID" && -x "$TARGET_BIN" ]]; then
    %s=1 "$TARGET_BIN" lifecycle-operation-finish "$OPERATION_ID" >> "$LOG_PATH" 2>&1 || true
  fi
  rm -f "$0"
  rm -f "$LOG_PATH"
  rm -f "$OWNER_MARKER"
  rmdir "$WORK_DIR" 2>/dev/null || true
}
trap cleanup EXIT
trap 'exit 143' HUP INT TERM

log() {
  printf '%%s\n' "$*" >> "$LOG_PATH"
}

sleep 1
log "starting kwor panel update"

process_matches_target() {
  local pid="$1"
  local exe=""
  [[ "$pid" =~ ^[0-9]+$ ]] || return 1
  [[ "$pid" != "$$" ]] || return 1
  exe="$(readlink "/proc/$pid/exe" 2>/dev/null || true)"
  exe="${exe%% (deleted)}"
  [[ -n "$exe" && "$exe" == "$TARGET_BIN" ]]
}

process_start_time() {
  local pid="$1"
  local stat_content=""
  local suffix=""
  local fields=()
  [[ -r "/proc/$pid/stat" ]] || return 1
  stat_content="$(<"/proc/$pid/stat")"
  [[ "$stat_content" == *") "* ]] || return 1
  suffix="${stat_content##*) }"
  read -r -a fields <<< "$suffix"
  [[ "${#fields[@]}" -gt 19 && -n "${fields[19]}" ]] || return 1
  printf '%%s\n' "${fields[19]}"
}

process_identity_matches_target() {
  local pid="$1"
  local expected_start="$2"
  local current_start=""
  current_start="$(process_start_time "$pid" 2>/dev/null || true)"
  [[ -n "$current_start" && "$current_start" == "$expected_start" ]] || return 1
  process_matches_target "$pid"
}

target_process_pids() {
  local proc_path
  local pid
  for proc_path in /proc/[0-9]*; do
    pid="${proc_path##*/}"
    if process_matches_target "$pid"; then
      echo "$pid"
    fi
  done
}

target_process_identities() {
  local pid=""
  local start_time=""
  while IFS= read -r pid; do
    [[ -n "$pid" ]] || continue
    start_time="$(process_start_time "$pid" 2>/dev/null || true)"
    if [[ -z "$start_time" ]]; then
      if process_matches_target "$pid"; then
        printf '! %%s\n' "$pid"
      fi
      continue
    fi
    printf '%%s %%s\n' "$pid" "$start_time"
  done < <(target_process_pids)
}

stop_target_systemd_service() {
  local main_pid=""
  if ! command -v systemctl >/dev/null 2>&1 || ! systemctl is-active --quiet "$SERVICE_NAME"; then
    return 0
  fi
  main_pid="$(systemctl show "$SERVICE_NAME" --property=MainPID --value 2>/dev/null || true)"
  if ! process_matches_target "$main_pid"; then
    log "refusing to stop an unverified $SERVICE_NAME systemd service"
    return 1
  fi
  systemctl stop "$SERVICE_NAME" >> "$LOG_PATH" 2>&1 || true
  ! systemctl is-active --quiet "$SERVICE_NAME"
}

stop_target_processes() {
  local identity=""
  local pid=""
  local start_time=""
  local attempt=0
  local live=0
  local identities=()
  mapfile -t identities < <(target_process_identities)
  for identity in "${identities[@]}"; do
    if [[ "$identity" == "! "* ]]; then
      log "failed to capture panel process identity: ${identity#! }"
      return 1
    fi
    read -r pid start_time <<< "$identity"
    if process_identity_matches_target "$pid" "$start_time"; then
      kill -TERM "$pid" >> "$LOG_PATH" 2>&1 || true
    fi
  done
  for attempt in $(seq 1 20); do
    live=0
    for identity in "${identities[@]}"; do
      read -r pid start_time <<< "$identity"
      if process_identity_matches_target "$pid" "$start_time"; then
        live=1
        break
      fi
    done
    [[ "$live" -eq 0 ]] && return 0
    sleep 0.1
  done
  for identity in "${identities[@]}"; do
    read -r pid start_time <<< "$identity"
    if process_identity_matches_target "$pid" "$start_time"; then
      kill -KILL "$pid" >> "$LOG_PATH" 2>&1 || true
    fi
  done
  for attempt in $(seq 1 20); do
    live=0
    for identity in "${identities[@]}"; do
      read -r pid start_time <<< "$identity"
      if process_identity_matches_target "$pid" "$start_time"; then
        live=1
        break
      fi
    done
    [[ "$live" -eq 0 ]] && return 0
    sleep 0.1
  done
  return 1
}

if ! stop_target_systemd_service; then
  log "failed to stop the verified panel systemd service"
  exit 1
fi

if ! stop_target_processes; then
  log "failed to stop the exact panel binary process"
  exit 1
fi

sleep 1

if [[ -f "$TARGET_BIN" ]]; then
  cp -f "$TARGET_BIN" "$BACKUP_BIN"
fi

cp -f "$STAGED_BIN" "$TARGET_BIN"
chmod 755 "$TARGET_BIN"

if [[ -n "$STAGED_INSTALL_SH" && -f "$STAGED_INSTALL_SH" ]]; then
  mkdir -p %s >> "$LOG_PATH" 2>&1 || true
  if cp -f "$STAGED_INSTALL_SH" %s >> "$LOG_PATH" 2>&1; then
    chmod 755 %s >> "$LOG_PATH" 2>&1 || true
    rm -f "$INSTALL_DIR/install.sh" >> "$LOG_PATH" 2>&1 || true
  else
    log "failed to place runtime install.sh into %s"
  fi
fi

if [[ -f "$STAGED_SERVICE_FILE" ]]; then
  mkdir -p %s >> "$LOG_PATH" 2>&1 || true
  if cp -f "$STAGED_SERVICE_FILE" %s >> "$LOG_PATH" 2>&1; then
    chmod 644 %s >> "$LOG_PATH" 2>&1 || true
    rm -f "$INSTALL_DIR/kwor.service" >> "$LOG_PATH" 2>&1 || true
  else
    log "failed to place runtime kwor.service into %s"
  fi
fi

if [[ "$BINARY_NAME" == "kwor_amd64" || "$BINARY_NAME" == "kwor_arm64" ]]; then
  rm -f "$(dirname "$TARGET_BIN")/kwor"
elif [[ "$BINARY_NAME" == "kwor" ]]; then
  rm -f "$(dirname "$TARGET_BIN")/kwor_amd64" "$(dirname "$TARGET_BIN")/kwor_arm64"
fi

start_with_repaired_systemd() {
  if ! command -v systemctl >/dev/null 2>&1 || [[ ! -f "$STAGED_SERVICE_FILE" ]]; then
    return 1
  fi
  mkdir -p /etc/systemd/system >> "$LOG_PATH" 2>&1 || return 1
  cp -f "$STAGED_SERVICE_FILE" "$SERVICE_FILE" >> "$LOG_PATH" 2>&1 || return 1
  chmod 644 "$SERVICE_FILE" >> "$LOG_PATH" 2>&1 || true
  systemctl daemon-reload >> "$LOG_PATH" 2>&1 || return 1
  systemctl reset-failed "$SERVICE_NAME" >> "$LOG_PATH" 2>&1 || true
  systemctl enable "$SERVICE_NAME" >> "$LOG_PATH" 2>&1 || return 1
  systemctl restart "$SERVICE_NAME" >> "$LOG_PATH" 2>&1 || return 1
  wait_for_target_runtime
}

repair_systemd_file() {
  if ! command -v systemctl >/dev/null 2>&1 || [[ ! -f "$STAGED_SERVICE_FILE" ]]; then
    return 0
  fi
  mkdir -p /etc/systemd/system >> "$LOG_PATH" 2>&1 || return 0
  cp -f "$STAGED_SERVICE_FILE" "$SERVICE_FILE" >> "$LOG_PATH" 2>&1 || return 0
  chmod 644 "$SERVICE_FILE" >> "$LOG_PATH" 2>&1 || true
  systemctl daemon-reload >> "$LOG_PATH" 2>&1 || true
  systemctl reset-failed "$SERVICE_NAME" >> "$LOG_PATH" 2>&1 || true
  systemctl enable "$SERVICE_NAME" >> "$LOG_PATH" 2>&1 || true
}

wait_for_target_runtime() {
  local main_pid=""
  local pid=""
  for _ in $(seq 1 40); do
    if command -v systemctl >/dev/null 2>&1 && systemctl is-active --quiet "$SERVICE_NAME"; then
      main_pid="$(systemctl show "$SERVICE_NAME" --property=MainPID --value 2>/dev/null || true)"
      if process_matches_target "$main_pid"; then
        return 0
      fi
    fi
    while IFS= read -r pid; do
      [[ -n "$pid" ]] && return 0
    done < <(target_process_pids)
    sleep 0.3
  done
  return 1
}

start_panel() {
  if start_with_repaired_systemd; then
    return 0
  fi
  if "$TARGET_BIN" start >> "$LOG_PATH" 2>&1 && wait_for_target_runtime; then
    repair_systemd_file
    return 0
  fi
  if command -v nohup >/dev/null 2>&1; then
    nohup "$TARGET_BIN" >> "$LOG_PATH" 2>&1 &
    if wait_for_target_runtime; then
      return 0
    fi
  fi
  return 1
}

if start_panel; then
  rm -f "$BACKUP_BIN"
  UPDATE_SUCCESS=1
  exit 0
fi

if [[ -f "$BACKUP_BIN" ]]; then
  cp -f "$BACKUP_BIN" "$TARGET_BIN"
  chmod 755 "$TARGET_BIN"
  if start_panel; then
    rm -f "$BACKUP_BIN"
  fi
fi

exit 1
`,
		shellQuote(targetBinPath),
		shellQuote(stagedBinPath),
		shellQuote(stagedInstallScriptPath),
		shellQuote(stagedServiceFilePath),
		shellQuote(backupPath),
		shellQuote(workDir),
		shellQuote(logPath),
		shellQuote(lastLogPath),
		shellQuote(panelUpdateServiceName),
		shellQuote(binaryName),
		shellQuote(operationID),
		InternalSystemdCommandEnv,
		shellQuote(filepath.Dir(installSupportPath)),
		shellQuote(installSupportPath),
		shellQuote(installSupportPath),
		installSupportPath,
		shellQuote(filepath.Dir(serviceSupportPath)),
		shellQuote(serviceSupportPath),
		shellQuote(serviceSupportPath),
		serviceSupportPath,
	)

	if err := os.WriteFile(scriptPath, []byte(script), 0o700); err != nil {
		return "", fmt.Errorf("write update script failed: %v", err)
	}
	return scriptPath, nil
}

func shellQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "'\\''") + "'"
}

func resolvePanelUpdateBinaryPath() (binaryPath string, runningPath string, servicePath string, serviceBinPath string, source string) {
	if execPath, err := os.Executable(); err == nil && strings.TrimSpace(execPath) != "" {
		if realPath, realErr := filepath.EvalSymlinks(execPath); realErr == nil {
			execPath = realPath
		}
		execPath = filepath.Clean(execPath)
		if info, statErr := os.Stat(execPath); statErr == nil && info.Mode().IsRegular() {
			return execPath, execPath, "", "", "current process"
		}
	}

	servicePath, serviceBinPath = resolvePanelServiceBinaryPath()
	if serviceBinPath != "" {
		return serviceBinPath, "", servicePath, serviceBinPath, "systemd service"
	}

	return filepath.Join(panelUpdateDefaultInstallDir, "kwor"), "", "", "", "default"
}

func resolvePanelServiceBinaryPath() (string, string) {
	for _, servicePath := range getSystemdServiceFileCandidates(panelUpdateServiceName) {
		if _, err := os.Stat(servicePath); err != nil {
			continue
		}
		if execPath := extractPanelExecStartPath(servicePath); execPath != "" {
			return servicePath, execPath
		}
		if workDir := extractPanelWorkingDirectory(servicePath); workDir != "" {
			for _, name := range []string{"kwor", "kwor_amd64", "kwor_arm64"} {
				candidate := filepath.Join(workDir, name)
				if _, err := os.Stat(candidate); err == nil {
					if resolved, realErr := filepath.EvalSymlinks(candidate); realErr == nil {
						candidate = resolved
					}
					return servicePath, candidate
				}
			}
		}
	}
	return "", ""
}

func extractPanelExecStartPath(servicePath string) string {
	content, err := os.ReadFile(servicePath)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "ExecStart=") {
			continue
		}
		value := strings.TrimSpace(strings.TrimPrefix(line, "ExecStart="))
		if value == "" {
			return ""
		}
		token := firstSystemdExecToken(value)
		if token == "" {
			return ""
		}
		token = strings.ReplaceAll(token, `\x20`, " ")
		if resolved, err := filepath.EvalSymlinks(token); err == nil {
			token = resolved
		}
		return token
	}
	return ""
}

func extractPanelWorkingDirectory(servicePath string) string {
	content, err := os.ReadFile(servicePath)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(content), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "WorkingDirectory=") {
			continue
		}
		value := strings.TrimSpace(strings.TrimPrefix(line, "WorkingDirectory="))
		value = strings.Trim(value, `"`)
		return strings.ReplaceAll(value, `\x20`, " ")
	}
	return ""
}

func firstSystemdExecToken(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if strings.HasPrefix(value, `"`) {
		rest := strings.TrimPrefix(value, `"`)
		if idx := strings.Index(rest, `"`); idx >= 0 {
			return rest[:idx]
		}
		return rest
	}
	return strings.Fields(value)[0]
}

func getPanelVersionCache(key string) (*PanelVersionListResponse, bool) {
	now := time.Now()
	panelUpdateVersionCache.Lock()
	defer panelUpdateVersionCache.Unlock()
	for cacheKey, entry := range panelUpdateVersionCache.items {
		if now.After(entry.expiresAt) {
			delete(panelUpdateVersionCache.items, cacheKey)
		}
	}
	entry, ok := panelUpdateVersionCache.items[key]
	if !ok {
		return nil, false
	}
	if now.After(entry.expiresAt) {
		delete(panelUpdateVersionCache.items, key)
		return nil, false
	}
	return clonePanelVersionListResponse(&entry.response), true
}

func setPanelVersionCache(key string, response *PanelVersionListResponse) {
	if response == nil {
		return
	}
	now := time.Now()
	panelUpdateVersionCache.Lock()
	defer panelUpdateVersionCache.Unlock()
	for cacheKey, entry := range panelUpdateVersionCache.items {
		if now.After(entry.expiresAt) {
			delete(panelUpdateVersionCache.items, cacheKey)
		}
	}
	if _, exists := panelUpdateVersionCache.items[key]; !exists && len(panelUpdateVersionCache.items) >= panelUpdateVersionCacheMax {
		oldestKey := ""
		var oldest time.Time
		for cacheKey, entry := range panelUpdateVersionCache.items {
			if oldestKey == "" || entry.createdAt.Before(oldest) {
				oldestKey = cacheKey
				oldest = entry.createdAt
			}
		}
		if oldestKey != "" {
			delete(panelUpdateVersionCache.items, oldestKey)
		}
	}
	panelUpdateVersionCache.items[key] = panelUpdateVersionCacheEntry{
		createdAt: now,
		expiresAt: now.Add(panelUpdateVersionCacheTTL),
		response:  *clonePanelVersionListResponse(response),
	}
}

func clonePanelVersionListResponse(response *PanelVersionListResponse) *PanelVersionListResponse {
	if response == nil {
		return nil
	}
	cloned := *response
	if response.Versions != nil {
		cloned.Versions = append([]PanelVersionItem(nil), response.Versions...)
	}
	return &cloned
}
