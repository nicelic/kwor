package service

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/alireza0/s-ui/config"
	"github.com/alireza0/s-ui/logger"
)

const (
	panelUpdateRepo              = "nicelic/kwor"
	panelUpdateInstallScriptURL  = "https://raw.githubusercontent.com/nicelic/kwor/main/install.sh"
	panelUpdatePerPage           = 20
	panelUpdateMaxPages          = 30
	panelUpdateVersionMaxLimit   = 20
	panelUpdateVersionCacheTTL   = 10 * time.Minute
	panelUpdateSupportFileMaxLen = 512 * 1024
	panelUpdateServiceName       = "kwor"
	panelUpdateDefaultInstallDir = "/opt/kwor"
)

type PanelUpdateService struct{}

type PanelUpdateStatus struct {
	LocalVersion      string `json:"localVersion"`
	BinaryPath        string `json:"binaryPath"`
	BinaryName        string `json:"binaryName"`
	InstallDir        string `json:"installDir"`
	ServiceFilePath   string `json:"serviceFilePath"`
	ServiceBinaryPath string `json:"serviceBinaryPath"`
	RunningBinaryPath string `json:"runningBinaryPath"`
	InstallSource     string `json:"installSource"`
	Platform          string `json:"platform"`
}

type PanelVersionListResponse struct {
	Versions []VersionItem `json:"versions"`
	Offset   int           `json:"offset"`
	Limit    int           `json:"limit"`
	Page     int           `json:"page"`
	PerPage  int           `json:"per_page"`
	HasMore  bool          `json:"has_more"`
}

type PanelInstallResult struct {
	Version    string `json:"version"`
	BinaryPath string `json:"binaryPath"`
	Started    bool   `json:"started"`
	Message    string `json:"message"`
}

type panelUpdateVersionCacheEntry struct {
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

func (s *PanelUpdateService) GetStatus() (*PanelUpdateStatus, error) {
	binaryPath, runningPath, servicePath, serviceBinPath, source := resolvePanelUpdateBinaryPath()
	installDir := filepath.Dir(binaryPath)

	return &PanelUpdateStatus{
		LocalVersion:      config.GetVersion(),
		BinaryPath:        binaryPath,
		BinaryName:        filepath.Base(binaryPath),
		InstallDir:        installDir,
		ServiceFilePath:   servicePath,
		ServiceBinaryPath: serviceBinPath,
		RunningBinaryPath: runningPath,
		InstallSource:     source,
		Platform:          fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH),
	}, nil
}

func (s *PanelUpdateService) GetRemoteVersions(offset int, limit int) (*PanelVersionListResponse, error) {
	offset, limit = normalizePanelVersionWindow(offset, limit)
	cacheKey := fmt.Sprintf("%s|%d|%d", panelUpdateRepo, offset, limit)
	if cached, ok := getPanelVersionCache(cacheKey); ok {
		return cached, nil
	}

	client := &http.Client{Timeout: 30 * time.Second}
	result := &PanelVersionListResponse{
		Versions: make([]VersionItem, 0, limit+1),
		Offset:   offset,
		Limit:    limit,
		PerPage:  limit,
		Page:     offset/limit + 1,
	}

	seenTags := make(map[string]struct{})
	matchedCount := 0
	for apiPage := 1; apiPage <= panelUpdateMaxPages && len(result.Versions) < limit+1; apiPage++ {
		releases, err := fetchGitHubReleasePageForRepo(panelUpdateRepo, client, apiPage, panelUpdatePerPage)
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

			asset, ok := pickPanelReleaseAsset(release.Assets)
			if !ok {
				continue
			}
			if matchedCount < offset {
				matchedCount++
				continue
			}
			matchedCount++

			result.Versions = append(result.Versions, VersionItem{
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

	version = normalizePanelVersionTag(version)
	if version == "" {
		return nil, fmt.Errorf("version is required")
	}
	if runtime.GOOS != "linux" {
		return nil, fmt.Errorf("panel update is only supported on Linux")
	}
	if os.Geteuid() != 0 {
		return nil, fmt.Errorf("panel update requires root privileges")
	}

	status, err := s.GetStatus()
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(status.BinaryPath) == "" {
		return nil, fmt.Errorf("failed to resolve current panel binary path")
	}

	downloadURL, err := getPanelReleaseAssetURL(version)
	if err != nil {
		return nil, err
	}

	workDir, err := os.MkdirTemp("", "kwor-panel-update-")
	if err != nil {
		return nil, fmt.Errorf("create update work dir failed: %v", err)
	}
	archivePath := filepath.Join(workDir, filepath.Base(downloadURL))
	stagedBinPath := filepath.Join(workDir, "kwor")
	stagedInstallScriptPath := filepath.Join(workDir, "install.sh")
	stagedServiceFilePath := filepath.Join(workDir, "kwor.service")
	cleanupWorkDir := true
	defer func() {
		if cleanupWorkDir {
			cleanupPanelUpdateWorkDir(workDir)
		}
	}()

	if err := downloadPanelReleaseArchive(downloadURL, archivePath); err != nil {
		return nil, err
	}
	if err := extractPanelReleasePayload(archivePath, stagedBinPath, stagedInstallScriptPath); err != nil {
		return nil, err
	}
	_ = os.Remove(archivePath)
	if err := downloadPanelLatestInstallScript(stagedInstallScriptPath); err != nil {
		logger.Warning("download latest panel install.sh failed, fallback to release packaged script: ", err)
	}
	if _, err := os.Stat(stagedInstallScriptPath); err != nil {
		stagedInstallScriptPath = ""
	} else if err := os.Chmod(stagedInstallScriptPath, 0o755); err != nil {
		return nil, fmt.Errorf("chmod staged install script failed: %v", err)
	}
	if err := os.WriteFile(stagedServiceFilePath, []byte(BuildPanelSystemdServiceContent(status.BinaryPath)), 0o644); err != nil {
		return nil, fmt.Errorf("write staged systemd service failed: %v", err)
	}
	if err := os.Chmod(stagedBinPath, 0o755); err != nil {
		return nil, fmt.Errorf("chmod staged binary failed: %v", err)
	}
	if err := validatePanelBinary(stagedBinPath); err != nil {
		return nil, err
	}

	installScriptPath, err := writePanelUpdateScript(workDir, status.BinaryPath, stagedBinPath, stagedInstallScriptPath, stagedServiceFilePath, status.BinaryName)
	if err != nil {
		return nil, err
	}
	cleanupWorkDir = false

	if err := startPanelUpdateWorker(installScriptPath); err != nil {
		cleanupWorkDir = true
		return nil, err
	}

	return &PanelInstallResult{
		Version:    version,
		BinaryPath: status.BinaryPath,
		Started:    true,
		Message:    "更新任务已启动，面板会自动停止、替换并重新启动",
	}, nil
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
	return offset, limit
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
	switch runtime.GOARCH {
	case "amd64":
		return "amd64"
	case "arm64":
		return "arm64"
	default:
		return runtime.GOARCH
	}
}

func getPanelReleaseAssetURL(version string) (string, error) {
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/releases/tags/%s", panelUpdateRepo, version)
	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest("GET", apiURL, nil)
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
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", fmt.Errorf("failed to parse GitHub release: %v", err)
	}
	asset, ok := pickPanelReleaseAsset(release.Assets)
	if !ok {
		return "", fmt.Errorf("kwor release asset for linux/%s not found in %s", panelUpdateArchName(), version)
	}
	return asset.BrowserDownloadURL, nil
}

func downloadPanelReleaseArchive(downloadURL string, archivePath string) error {
	client := &http.Client{Timeout: 600 * time.Second}
	resp, err := client.Get(downloadURL)
	if err != nil {
		return fmt.Errorf("download release archive failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("download release archive failed, HTTP %d", resp.StatusCode)
	}

	out, err := os.Create(archivePath)
	if err != nil {
		return fmt.Errorf("create release archive file failed: %v", err)
	}
	if _, err = io.Copy(out, resp.Body); err != nil {
		_ = out.Close()
		_ = os.Remove(archivePath)
		return fmt.Errorf("write release archive failed: %v", err)
	}
	if err = out.Close(); err != nil {
		_ = os.Remove(archivePath)
		return fmt.Errorf("close release archive failed: %v", err)
	}
	return nil
}

func cleanupPanelUpdateWorkDir(workDir string) {
	workDir = strings.TrimSpace(workDir)
	if workDir == "" {
		return
	}
	for _, name := range []string{
		"kwor",
		"install.sh",
		"install.sh.download",
		"kwor.service",
		"apply-update.sh",
		"apply-update.log",
		"kwor-linux-amd64.tar.gz",
		"kwor-linux-arm64.tar.gz",
	} {
		_ = os.Remove(filepath.Join(workDir, name))
	}
	_ = os.Remove(filepath.Join(workDir, "kwor", "kwor"))
	_ = os.Remove(filepath.Join(workDir, "kwor", "install.sh"))
	_ = os.Remove(filepath.Join(workDir, "kwor", "kwor.service"))
	_ = os.Remove(filepath.Join(workDir, "kwor"))
	_ = os.Remove(workDir)
}

func extractPanelReleasePayload(archivePath string, stagedBinPath string, stagedInstallScriptPath string) error {
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
			if err := writePanelTarFile(tr, stagedBinPath); err != nil {
				return err
			}
			foundBinary = true
		case "install.sh":
			if strings.TrimSpace(stagedInstallScriptPath) != "" {
				if err := writePanelTarFile(tr, stagedInstallScriptPath); err != nil {
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
	out, err := os.Create(targetPath)
	if err != nil {
		return err
	}
	if _, err = io.Copy(out, reader); err != nil {
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
	targetPath = strings.TrimSpace(targetPath)
	if targetPath == "" {
		return fmt.Errorf("target install script path is empty")
	}
	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest("GET", panelUpdateInstallScriptURL, nil)
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
	written, copyErr := io.Copy(out, io.LimitReader(resp.Body, panelUpdateSupportFileMaxLen+1))
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
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, binPath, "-v")
	cmd.Dir = filepath.Dir(binPath)
	output, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("downloaded kwor binary validation timed out")
	}
	if err != nil {
		return fmt.Errorf("downloaded kwor binary is not executable: %v: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func startPanelUpdateWorker(scriptPath string) error {
	if _, err := exec.LookPath("systemd-run"); err == nil {
		unitName := fmt.Sprintf("kwor-panel-update-%d", time.Now().UnixNano())
		cmd := exec.Command(
			"systemd-run",
			"--unit", unitName,
			"--collect",
			"--description", "kwor panel update",
			"bash", scriptPath,
		)
		output, runErr := cmd.CombinedOutput()
		if runErr == nil {
			return nil
		}
		message := strings.TrimSpace(string(output))
		if isPanelSystemdServiceActive() {
			if message != "" {
				return fmt.Errorf("start panel update worker with systemd-run failed: %v: %s", runErr, message)
			}
			return fmt.Errorf("start panel update worker with systemd-run failed: %v", runErr)
		}
		logger.Warning("systemd-run panel update worker failed, fallback to detached process: ", runErr, " ", message)
	}

	commandName := "bash"
	args := []string{scriptPath}
	if _, err := exec.LookPath("setsid"); err == nil {
		commandName = "setsid"
		args = []string{"bash", scriptPath}
	} else if _, err := exec.LookPath("nohup"); err == nil {
		commandName = "nohup"
		args = []string{"bash", scriptPath}
	}

	cmd := exec.Command(commandName, args...)
	cmd.Dir = filepath.Dir(scriptPath)
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start panel update worker failed: %v", err)
	}
	if err := cmd.Process.Release(); err != nil {
		logger.Warning("release panel update worker process failed: ", err)
	}
	return nil
}

func isPanelSystemdServiceActive() bool {
	if _, err := exec.LookPath("systemctl"); err != nil {
		return false
	}
	return exec.Command("systemctl", "is-active", "--quiet", panelUpdateServiceName).Run() == nil
}

func BuildPanelSystemdServiceContent(binPath string) string {
	binPath = strings.TrimSpace(binPath)
	binDir := filepath.Dir(binPath)
	return fmt.Sprintf(`[Unit]
Description=kwor Service
After=network.target nss-lookup.target

[Service]
Type=simple
WorkingDirectory=%s
ExecStart=%s
Restart=on-failure
RestartSec=5s
LimitNOFILE=infinity

[Install]
WantedBy=multi-user.target
`, escapeSystemdUnitValue(binDir), buildSystemdExecCommand(binPath))
}

func writePanelUpdateScript(workDir string, targetBinPath string, stagedBinPath string, stagedInstallScriptPath string, stagedServiceFilePath string, binaryName string) (string, error) {
	scriptPath := filepath.Join(workDir, "apply-update.sh")
	backupPath := targetBinPath + ".bak"
	logPath := filepath.Join(workDir, "apply-update.log")

	script := fmt.Sprintf(`#!/usr/bin/env bash
set -u

TARGET_BIN=%s
STAGED_BIN=%s
STAGED_INSTALL_SH=%s
STAGED_SERVICE_FILE=%s
BACKUP_BIN=%s
WORK_DIR=%s
LOG_PATH=%s
SERVICE_NAME=%s
BINARY_NAME=%s
INSTALL_DIR="$(dirname "$TARGET_BIN")"
SERVICE_FILE="/etc/systemd/system/${SERVICE_NAME}.service"

cleanup() {
  rm -f "$STAGED_BIN"
  if [[ -n "$STAGED_INSTALL_SH" ]]; then
    rm -f "$STAGED_INSTALL_SH"
  fi
  rm -f "$STAGED_SERVICE_FILE"
  rm -f "$0"
  rm -f "$LOG_PATH"
  rmdir "$WORK_DIR" 2>/dev/null || true
}
trap cleanup EXIT
trap 'cleanup; exit 143' HUP INT TERM

log() {
  printf '%%s\n' "$*" >> "$LOG_PATH"
}

sleep 1
log "starting kwor panel update"

if [[ -x "$TARGET_BIN" ]]; then
  "$TARGET_BIN" stop >> "$LOG_PATH" 2>&1 || true
fi

if command -v systemctl >/dev/null 2>&1; then
  systemctl stop "$SERVICE_NAME" >> "$LOG_PATH" 2>&1 || true
fi

for name in kwor kwor_amd64 kwor_arm64; do
  pkill -TERM -x "$name" >> "$LOG_PATH" 2>&1 || true
done
sleep 2
for name in kwor kwor_amd64 kwor_arm64; do
  if pgrep -x "$name" >/dev/null 2>&1; then
    pkill -KILL -x "$name" >> "$LOG_PATH" 2>&1 || true
  fi
done

sleep 1

if [[ -f "$TARGET_BIN" ]]; then
  cp -f "$TARGET_BIN" "$BACKUP_BIN"
fi

cp -f "$STAGED_BIN" "$TARGET_BIN"
chmod 755 "$TARGET_BIN"

if [[ -n "$STAGED_INSTALL_SH" && -f "$STAGED_INSTALL_SH" ]]; then
  cp -f "$STAGED_INSTALL_SH" "$INSTALL_DIR/install.sh" >> "$LOG_PATH" 2>&1 || true
  chmod 755 "$INSTALL_DIR/install.sh" >> "$LOG_PATH" 2>&1 || true
fi

if [[ -f "$STAGED_SERVICE_FILE" ]]; then
  cp -f "$STAGED_SERVICE_FILE" "$INSTALL_DIR/kwor.service" >> "$LOG_PATH" 2>&1 || true
  chmod 644 "$INSTALL_DIR/kwor.service" >> "$LOG_PATH" 2>&1 || true
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
  for _ in $(seq 1 40); do
    if systemctl is-active --quiet "$SERVICE_NAME"; then
      return 0
    fi
    sleep 0.3
  done
  return 1
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

start_panel() {
  if start_with_repaired_systemd; then
    return 0
  fi
  if "$TARGET_BIN" start >> "$LOG_PATH" 2>&1; then
    repair_systemd_file
    return 0
  fi
  if command -v nohup >/dev/null 2>&1; then
    nohup "$TARGET_BIN" >> "$LOG_PATH" 2>&1 &
    return 0
  fi
  return 1
}

if start_panel; then
  rm -f "$BACKUP_BIN"
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
		shellQuote(panelUpdateServiceName),
		shellQuote(binaryName),
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
	runningPath = resolvePanelRunningBinaryPath()
	if runningPath != "" {
		return runningPath, runningPath, "", "", "running process"
	}

	servicePath, serviceBinPath = resolvePanelServiceBinaryPath()
	if serviceBinPath != "" {
		return serviceBinPath, "", servicePath, serviceBinPath, "systemd service"
	}

	execPath, err := os.Executable()
	if err == nil && strings.TrimSpace(execPath) != "" {
		if realPath, realErr := filepath.EvalSymlinks(execPath); realErr == nil {
			execPath = realPath
		}
		return execPath, "", "", "", "current process"
	}

	return filepath.Join(panelUpdateDefaultInstallDir, "kwor"), "", "", "", "default"
}

func resolvePanelRunningBinaryPath() string {
	for _, processName := range []string{"kwor", "kwor_amd64", "kwor_arm64"} {
		out, err := exec.Command("pgrep", "-x", processName).Output()
		if err != nil {
			continue
		}
		for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			pid := strings.TrimSpace(line)
			if pid == "" || pid == fmt.Sprintf("%d", os.Getpid()) {
				continue
			}
			exePath := filepath.Join("/proc", pid, "exe")
			if resolved, err := filepath.EvalSymlinks(exePath); err == nil && resolved != "" {
				return resolved
			}
		}
	}
	return ""
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
	panelUpdateVersionCache.items[key] = panelUpdateVersionCacheEntry{
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
		cloned.Versions = append([]VersionItem(nil), response.Versions...)
	}
	return &cloned
}
