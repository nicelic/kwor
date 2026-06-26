package service

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alireza0/s-ui/logger"
)

// CoreManagerService 管理 sing-box 内核的下载、版本管理和运行控制
type CoreManagerService struct {
	mu        sync.Mutex
	coreCmd   *exec.Cmd
	isStarted bool
	stdout    *os.File
	stderr    *os.File
}

const (
	// systemd 服务名，使用唯一前缀避免与系统其他 sing-box 服务冲突
	singboxSystemdName              = "kwor-singbox"
	InternalSystemdCommandEnv       = "KWOR_INTERNAL_SYSTEMD"
	managedCoreConfigMaterializeTTL = 10 * time.Second
	singboxConfigCheckTimeout       = 8 * time.Second

	coreAutoCheckEnabledKey       = "coreAutoCheckEnabled"
	coreAutoCheckIntervalHoursKey = "coreAutoCheckIntervalHours"
	coreAutoCheckLastAtKey        = "coreAutoCheckLastAt"
	coreAutoCheckLatestStableKey  = "coreAutoCheckLatestStable"
	coreAutoCheckLatestAlphaKey   = "coreAutoCheckLatestAlpha"
	coreAutoCheckPendingStableKey = "coreAutoCheckPendingStable"
	coreAutoCheckPendingAlphaKey  = "coreAutoCheckPendingAlpha"
)

var legacySingboxSystemdNames = []string{
	"sing-box",
	"singbox",
	"s-ui-singbox",
	"sui-singbox",
}

var coreAutoCheckMu sync.Mutex

const (
	coreReleaseGitHubPerPage = 20
	coreReleaseMaxPages      = 30
	coreVersionCacheTTL      = 10 * time.Minute
	coreVersionMaxLimit      = 20
	coreLocalVersionCacheTTL = 45 * time.Second
)

type coreVersionCacheEntry struct {
	expiresAt time.Time
	response  VersionListResponse
}

var coreVersionCache = struct {
	sync.Mutex
	items map[string]coreVersionCacheEntry
}{
	items: make(map[string]coreVersionCacheEntry),
}

type coreLocalVersionCacheEntry struct {
	expiresAt   time.Time
	binModTime  time.Time
	binSize     int64
	version     string
	versionInfo string
}

var coreLocalVersionCache = struct {
	sync.Mutex
	items map[string]coreLocalVersionCacheEntry
}{
	items: make(map[string]coreLocalVersionCacheEntry),
}

func cleanupCoreVersionCacheLocked(now time.Time) {
	for key, entry := range coreVersionCache.items {
		if now.After(entry.expiresAt) {
			delete(coreVersionCache.items, key)
		}
	}
}

func getCoreLocalVersionCache(binPath string, binModTime time.Time, binSize int64) (string, string, bool) {
	now := time.Now()
	coreLocalVersionCache.Lock()
	defer coreLocalVersionCache.Unlock()

	entry, ok := coreLocalVersionCache.items[binPath]
	if !ok {
		return "", "", false
	}
	if now.After(entry.expiresAt) {
		delete(coreLocalVersionCache.items, binPath)
		return "", "", false
	}
	if !entry.binModTime.Equal(binModTime) || entry.binSize != binSize {
		delete(coreLocalVersionCache.items, binPath)
		return "", "", false
	}
	return entry.version, entry.versionInfo, true
}

func setCoreLocalVersionCache(binPath string, binModTime time.Time, binSize int64, version string, versionInfo string) {
	coreLocalVersionCache.Lock()
	defer coreLocalVersionCache.Unlock()

	coreLocalVersionCache.items[binPath] = coreLocalVersionCacheEntry{
		expiresAt:   time.Now().Add(coreLocalVersionCacheTTL),
		binModTime:  binModTime,
		binSize:     binSize,
		version:     version,
		versionInfo: versionInfo,
	}
}

func clearCoreLocalVersionCache(binPath string) {
	coreLocalVersionCache.Lock()
	defer coreLocalVersionCache.Unlock()
	delete(coreLocalVersionCache.items, binPath)
}

// GitHubRelease GitHub Release 信息
type GitHubRelease struct {
	TagName     string        `json:"tag_name"`
	Name        string        `json:"name"`
	Prerelease  bool          `json:"prerelease"`
	Assets      []GitHubAsset `json:"assets"`
	PublishedAt string        `json:"published_at"`
	CreatedAt   string        `json:"created_at"`
}

// GitHubAsset GitHub Release 资源
type GitHubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
	Size               int64  `json:"size"`
}

// CoreInfo 内核状态信息
type CoreInfo struct {
	LocalVersion       string                 `json:"localVersion"`
	Running            bool                   `json:"running"`
	VersionInfo        string                 `json:"versionInfo"`
	Platform           string                 `json:"platform"`
	RuntimeMode        string                 `json:"runtimeMode,omitempty"`
	InstalledTarget    CoreDownloadTarget     `json:"installedTarget,omitempty"`
	DownloadPreference CoreDownloadPreference `json:"downloadPreference"`
}

// VersionListResponse 版本列表响应
type VersionListResponse struct {
	Versions []VersionItem `json:"versions"`
	Page     int           `json:"page,omitempty"`
	PerPage  int           `json:"per_page,omitempty"`
	Offset   int           `json:"offset,omitempty"`
	Limit    int           `json:"limit,omitempty"`
	HasMore  bool          `json:"has_more"`
}

// VersionItem 版本项
type VersionItem struct {
	TagName     string `json:"tag_name"`
	Name        string `json:"name"`
	Prerelease  bool   `json:"prerelease"`
	PublishedAt string `json:"published_at"`
	AssetName   string `json:"asset_name,omitempty"`
	AssetSize   int64  `json:"asset_size,omitempty"`
}

// CoreUpdateInfo 内核更新检测信息
type CoreDownloadTarget struct {
	OS         string `json:"os"`
	Arch       string `json:"arch"`
	Libc       string `json:"libc"`
	Amd64Level string `json:"amd64Level"`
}

type CoreUpdateInfo struct {
	Enabled       bool   `json:"enabled"`
	IntervalHours int    `json:"intervalHours"`
	LastCheckedAt int64  `json:"lastCheckedAt"`
	LatestStable  string `json:"latestStable"`
	LatestAlpha   string `json:"latestAlpha"`
	PendingStable string `json:"pendingStable"`
	PendingAlpha  string `json:"pendingAlpha"`
	HasUpdate     bool   `json:"hasUpdate"`
	UpdateCount   int    `json:"updateCount"`
}

func (s *CoreManagerService) getCoreDir() string {
	return GetSingboxCoreDir()
}

func (s *CoreManagerService) getCoreBinName() string {
	if runtime.GOOS == "windows" {
		return "sing-box.exe"
	}
	return "sing-box"
}

func (s *CoreManagerService) getCoreBinPath() string {
	return filepath.Join(s.getCoreDir(), s.getCoreBinName())
}

func (s *CoreManagerService) getConfigPath() string {
	return GetSingboxConfigPath()
}

func (s *CoreManagerService) regenerateRuntimeConfig() {
	configService := NewConfigService(nil)
	NewProManagerService(configService).SaveInboundJson()
}

func (s *CoreManagerService) getPlatformInfo() string {
	return fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)
}

// GetSingboxSystemdName 返回 sing-box systemd 服务名（供外部调用，例如 cmd.go）
func GetSingboxSystemdName() string {
	return singboxSystemdName
}

// getSingboxServiceFilePath 返回 systemd service 文件路径
func getSingboxServiceFilePath() string {
	return getSystemdServiceFilePathByName(singboxSystemdName)
}

func getSystemdServiceFilePathByName(serviceName string) string {
	return "/etc/systemd/system/" + serviceName + ".service"
}

func getSystemdServiceFileCandidates(serviceName string) []string {
	fileName := serviceName + ".service"
	return []string{
		filepath.Join("/etc/systemd/system", fileName),
		filepath.Join("/lib/systemd/system", fileName),
		filepath.Join("/usr/lib/systemd/system", fileName),
	}
}

func getSystemdControlBinaryPath() string {
	execPath, err := os.Executable()
	if err != nil {
		return "kwor"
	}
	if realPath, resolveErr := filepath.EvalSymlinks(execPath); resolveErr == nil && realPath != "" {
		return realPath
	}
	return execPath
}

func escapeSystemdUnitValue(value string) string {
	var builder strings.Builder
	builder.Grow(len(value))

	for _, r := range value {
		switch r {
		case '\\':
			builder.WriteString(`\\`)
		case '"':
			builder.WriteString(`\"`)
		case '%':
			builder.WriteString(`%%`)
		case ' ':
			builder.WriteString(`\x20`)
		case '\t':
			builder.WriteString(`\x09`)
		case '\n':
			builder.WriteString(`\x0a`)
		case '\r':
			builder.WriteString(`\x0d`)
		default:
			builder.WriteRune(r)
		}
	}

	return builder.String()
}

func quoteSystemdUnitValue(value string) string {
	return escapeSystemdUnitValue(value)
}

func quoteSystemdEnvironmentAssignment(key, value string) string {
	return escapeSystemdUnitValue(key + "=" + value)
}

func buildSystemdExecCommand(args ...string) string {
	escaped := make([]string, 0, len(args))
	for _, arg := range args {
		if arg == "" {
			continue
		}
		escaped = append(escaped, escapeSystemdUnitValue(arg))
	}
	return strings.Join(escaped, " ")
}

func verifySystemdUnitFile(servicePath string) error {
	out, err := exec.Command("systemd-analyze", "verify", servicePath).CombinedOutput()
	if err == nil {
		return nil
	}
	if errors.Is(err, exec.ErrNotFound) {
		return nil
	}

	detail := strings.TrimSpace(string(out))
	if detail == "" {
		return fmt.Errorf("systemd unit verify failed: %v", err)
	}
	return fmt.Errorf("systemd unit verify failed: %v: %s", err, detail)
}

func buildSingboxSystemdServiceContent(controlPath, binPath, configPath, workDir string) string {
	return fmt.Sprintf(`[Unit]
Description=kwor sing-box service
Documentation=https://sing-box.sagernet.org
After=network.target nss-lookup.target

[Service]
Type=simple
Environment=%s
ExecStartPre=%s
ExecStart=%s
ExecStopPost=%s
WorkingDirectory=%s
Restart=on-failure
RestartSec=2s
LimitNOFILE=infinity
StandardOutput=journal
StandardError=journal

[Install]
WantedBy=multi-user.target
`,
		quoteSystemdEnvironmentAssignment(InternalSystemdCommandEnv, "1"),
		buildSystemdExecCommand(controlPath, "materialize-core-config", "singbox"),
		buildSystemdExecCommand(binPath, "run", "-c", configPath),
		buildSystemdExecCommand(controlPath, "cleanup-core-config", "singbox"),
		escapeSystemdUnitValue(workDir),
	)
}

// GetCoreStatus 获取内核状态
func (s *CoreManagerService) GetCoreStatus() (*CoreInfo, error) {
	if err := EnsureManagedCoreLayout(); err != nil {
		return nil, err
	}

	info := &CoreInfo{
		Platform: s.getPlatformInfo(),
	}
	info.RuntimeMode = string(getManagedCoreRuntimeMode())
	if preference, err := s.GetDownloadPreference(); err == nil {
		info.DownloadPreference = preference
	} else {
		logger.Warning("failed to load core download preference: ", err)
	}

	binPath := s.getCoreBinPath()
	if statInfo, err := os.Stat(binPath); err == nil {
		if version, versionInfo, ok := getCoreLocalVersionCache(binPath, statInfo.ModTime(), statInfo.Size()); ok {
			info.LocalVersion = version
			info.VersionInfo = versionInfo
		} else {
			version, versionInfo := s.getLocalVersion(binPath)
			setCoreLocalVersionCache(binPath, statInfo.ModTime(), statInfo.Size(), version, versionInfo)
			info.LocalVersion = version
			info.VersionInfo = versionInfo
		}
		installedTarget := inferTargetFromGoBuildInfo(binPath)
		if installedTarget.OS == "" && installedTarget.Arch == "" {
			installedTarget = inferTargetFromPlatform(info.Platform)
		}
		info.InstalledTarget = mergeInstalledTargetWithPreference(installedTarget, info.DownloadPreference.Target)
	} else {
		clearCoreLocalVersionCache(binPath)
	}

	info.Running = s.isRunning()
	return info, nil
}

func (s *CoreManagerService) getLocalVersion(binPath string) (string, string) {
	cmd := exec.Command(binPath, "version")
	cmd.Dir = filepath.Dir(binPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		logger.Warning("Failed to get sing-box version: ", err)
		return "", ""
	}
	outputStr := strings.TrimSpace(string(output))
	parts := strings.Fields(outputStr)
	version := ""
	if len(parts) >= 3 {
		version = parts[2]
	}
	return version, outputStr
}

// GetRemoteVersions 从 GitHub 获取远程版本列表
func (s *CoreManagerService) GetRemoteVersions(channel string) (*VersionListResponse, error) {
	return s.GetRemoteVersionsWindow(channel, 0, 20, CoreDownloadTarget{})
}

func (s *CoreManagerService) GetRemoteVersionsWindow(channel string, offset int, limit int, target CoreDownloadTarget) (*VersionListResponse, error) {
	offset, limit = normalizeCoreVersionWindow(offset, limit)
	filterTarget := hasCoreDownloadTargetFilter(target)
	if filterTarget {
		target = s.normalizeDownloadTarget(target)
	}

	cacheKey := coreVersionCacheKey("SagerNet/sing-box", channel, offset, limit, target, filterTarget)
	if cached, ok := getCoreVersionCache(cacheKey); ok {
		return cached, nil
	}

	client := &http.Client{Timeout: 30 * time.Second}
	seenTags := make(map[string]struct{})
	result := &VersionListResponse{
		Versions: make([]VersionItem, 0, limit+1),
		Offset:   offset,
		Limit:    limit,
		PerPage:  limit,
		Page:     offset/limit + 1,
	}

	matchedCount := 0
	for apiPage := 1; apiPage <= coreReleaseMaxPages && len(result.Versions) < limit+1; apiPage++ {
		releases, err := s.fetchGitHubReleasePage(client, apiPage, coreReleaseGitHubPerPage)
		if err != nil {
			return nil, err
		}
		if len(releases) == 0 {
			break
		}

		for _, r := range releases {
			if !shouldIncludeRelease(channel, r.Prerelease) {
				continue
			}
			if _, ok := seenTags[r.TagName]; ok {
				continue
			}
			seenTags[r.TagName] = struct{}{}

			var assetName string
			var assetSize int64
			if filterTarget {
				asset, ok := pickSingboxAssetFromAssets(r.TagName, r.Assets, target)
				if !ok {
					continue
				}
				assetName = asset.Name
				assetSize = asset.Size
			}

			if matchedCount < offset {
				matchedCount++
				continue
			}
			matchedCount++

			result.Versions = append(result.Versions, VersionItem{
				TagName:     r.TagName,
				Name:        r.Name,
				Prerelease:  r.Prerelease,
				PublishedAt: r.PublishedAt,
				AssetName:   assetName,
				AssetSize:   assetSize,
			})
			if len(result.Versions) >= limit+1 {
				break
			}
		}

		if len(releases) < coreReleaseGitHubPerPage {
			break
		}
	}

	if len(result.Versions) > limit {
		result.HasMore = true
		result.Versions = result.Versions[:limit]
	}
	setCoreVersionCache(cacheKey, result)
	return cloneVersionListResponse(result), nil
}

func shouldIncludeRelease(channel string, prerelease bool) bool {
	if channel == "stable" {
		return !prerelease
	}
	if channel == "alpha" {
		return prerelease
	}
	return true
}

func normalizeCoreVersionWindow(offset int, limit int) (int, int) {
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 {
		limit = 5
	}
	if limit > coreVersionMaxLimit {
		limit = coreVersionMaxLimit
	}
	return offset, limit
}

func hasCoreDownloadTargetFilter(target CoreDownloadTarget) bool {
	return strings.TrimSpace(target.OS) != "" ||
		strings.TrimSpace(target.Arch) != "" ||
		strings.TrimSpace(target.Libc) != "" ||
		strings.TrimSpace(target.Amd64Level) != ""
}

func cloneVersionListResponse(response *VersionListResponse) *VersionListResponse {
	if response == nil {
		return nil
	}
	cloned := *response
	if response.Versions != nil {
		cloned.Versions = append([]VersionItem(nil), response.Versions...)
	}
	return &cloned
}

func getCoreVersionCache(key string) (*VersionListResponse, bool) {
	now := time.Now()
	coreVersionCache.Lock()
	defer coreVersionCache.Unlock()
	cleanupCoreVersionCacheLocked(now)

	entry, ok := coreVersionCache.items[key]
	if !ok {
		return nil, false
	}
	if now.After(entry.expiresAt) {
		delete(coreVersionCache.items, key)
		return nil, false
	}
	return cloneVersionListResponse(&entry.response), true
}

func setCoreVersionCache(key string, response *VersionListResponse) {
	if response == nil {
		return
	}
	now := time.Now()
	coreVersionCache.Lock()
	defer coreVersionCache.Unlock()
	cleanupCoreVersionCacheLocked(now)
	coreVersionCache.items[key] = coreVersionCacheEntry{
		expiresAt: now.Add(coreVersionCacheTTL),
		response:  *cloneVersionListResponse(response),
	}
}

func coreVersionCacheKey(repo string, channel string, offset int, limit int, target CoreDownloadTarget, filterTarget bool) string {
	if !filterTarget {
		return fmt.Sprintf("%s|%s|%d|%d|all", repo, channel, offset, limit)
	}
	return fmt.Sprintf(
		"%s|%s|%d|%d|%s|%s|%s|%s",
		repo,
		channel,
		offset,
		limit,
		target.OS,
		target.Arch,
		target.Libc,
		target.Amd64Level,
	)
}

func fetchGitHubReleasePageForRepo(repo string, client *http.Client, apiPage int, perPage int) ([]GitHubRelease, error) {
	apiURL := fmt.Sprintf(
		"https://api.github.com/repos/%s/releases?per_page=%d&page=%d",
		repo,
		perPage,
		apiPage,
	)
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "kwor")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request GitHub API failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned %d", resp.StatusCode)
	}

	var releases []GitHubRelease
	if err = json.NewDecoder(resp.Body).Decode(&releases); err != nil {
		return nil, fmt.Errorf("failed to parse GitHub releases: %v", err)
	}
	return releases, nil
}

func (s *CoreManagerService) fetchGitHubReleasePage(client *http.Client, apiPage int, perPage int) ([]GitHubRelease, error) {
	return fetchGitHubReleasePageForRepo("SagerNet/sing-box", client, apiPage, perPage)
}

// GetRemoteVersionsPage keeps the old page/per_page contract while using the new window loader.
func (s *CoreManagerService) GetRemoteVersionsPage(channel string, page int, perPage int) (*VersionListResponse, error) {
	if page < 1 {
		page = 1
	}
	if perPage <= 0 {
		perPage = 5
	}
	offset := (page - 1) * perPage
	return s.GetRemoteVersionsWindow(channel, offset, perPage, CoreDownloadTarget{})
}

func pickSingboxAssetFromAssets(version string, assets []GitHubAsset, target CoreDownloadTarget) (GitHubAsset, bool) {
	normalizedTarget := target
	if normalizedTarget.OS == "" {
		normalizedTarget.OS = runtime.GOOS
	}
	if normalizedTarget.Arch == "" {
		normalizedTarget.Arch = runtime.GOARCH
	}
	normalizedTarget = (&CoreManagerService{}).normalizeDownloadTarget(normalizedTarget)
	ext := coreArchiveExtForOS(normalizedTarget.OS)

	for _, candidateName := range buildCoreAssetCandidates(version, normalizedTarget) {
		for _, asset := range assets {
			if asset.Name == candidateName {
				return asset, true
			}
		}
	}

	for _, asset := range assets {
		if assetMatchesDownloadTarget(asset, normalizedTarget, ext) {
			return asset, true
		}
	}

	return GitHubAsset{}, false
}

func (s *CoreManagerService) getDownloadAsset(version string, target CoreDownloadTarget) (string, error) {
	apiURL := fmt.Sprintf("https://api.github.com/repos/SagerNet/sing-box/releases/tags/%s", version)

	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "kwor")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("请求 GitHub API 失败: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return "", fmt.Errorf("GitHub API 返回 %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var release GitHubRelease
	if err := json.Unmarshal(body, &release); err != nil {
		return "", fmt.Errorf("解析 GitHub 响应失败: %v", err)
	}

	ver := strings.TrimPrefix(release.TagName, "v")
	normalizedTarget := s.normalizeDownloadTarget(target)
	ext := coreArchiveExtForOS(normalizedTarget.OS)

	for _, candidateName := range buildCoreAssetCandidates(ver, normalizedTarget) {
		for _, asset := range release.Assets {
			if asset.Name == candidateName {
				return asset.BrowserDownloadURL, nil
			}
		}
	}

	for _, asset := range release.Assets {
		if assetMatchesDownloadTarget(asset, normalizedTarget, ext) {
			return asset.BrowserDownloadURL, nil
		}
	}

	return "", fmt.Errorf("no sing-box asset found for %s, version: %s", describeCoreDownloadTarget(normalizedTarget), ver)
}

func (s *CoreManagerService) normalizeDownloadTarget(target CoreDownloadTarget) CoreDownloadTarget {
	normalized := CoreDownloadTarget{
		OS:         strings.ToLower(strings.TrimSpace(target.OS)),
		Arch:       strings.ToLower(strings.TrimSpace(target.Arch)),
		Libc:       strings.ToLower(strings.TrimSpace(target.Libc)),
		Amd64Level: normalizeAmd64Level(target.Amd64Level),
	}
	if normalized.OS == "" {
		normalized.OS = runtime.GOOS
	}
	if normalized.Arch == "" {
		normalized.Arch = s.getArchName()
	}
	if normalized.Arch == "amd64" {
		if normalized.Amd64Level == "" {
			normalized.Amd64Level = inferHostAMD64Level()
		}
	} else {
		normalized.Amd64Level = ""
	}
	if normalized.OS != "linux" {
		normalized.Libc = ""
		return normalized
	}
	switch normalized.Libc {
	case "", "glibc", "musl", "universal":
	default:
		normalized.Libc = ""
	}
	if normalized.Libc == "" {
		if detected := detectHostLinuxLibc(); detected != "" {
			normalized.Libc = detected
		}
	}
	return normalized
}

func coreArchiveExtForOS(goos string) string {
	if goos == "windows" {
		return ".zip"
	}
	return ".tar.gz"
}

func buildCoreAssetCandidates(version string, target CoreDownloadTarget) []string {
	ext := coreArchiveExtForOS(target.OS)
	candidates := make([]string, 0, 3)
	appendUnique := func(name string) {
		if name == "" {
			return
		}
		for _, existing := range candidates {
			if existing == name {
				return
			}
		}
		candidates = append(candidates, name)
	}

	if target.OS == "linux" {
		switch target.Libc {
		case "glibc", "musl":
			appendUnique(fmt.Sprintf("sing-box-%s-linux-%s-%s%s", version, target.Arch, target.Libc, ext))
			appendUnique(fmt.Sprintf("sing-box-%s-linux-%s%s", version, target.Arch, ext))
		case "universal":
			appendUnique(fmt.Sprintf("sing-box-%s-linux-%s%s", version, target.Arch, ext))
		default:
			appendUnique(fmt.Sprintf("sing-box-%s-linux-%s%s", version, target.Arch, ext))
			appendUnique(fmt.Sprintf("sing-box-%s-linux-%s-glibc%s", version, target.Arch, ext))
			appendUnique(fmt.Sprintf("sing-box-%s-linux-%s-musl%s", version, target.Arch, ext))
		}
		return candidates
	}

	appendUnique(fmt.Sprintf("sing-box-%s-%s-%s%s", version, target.OS, target.Arch, ext))
	return candidates
}

func assetMatchesDownloadTarget(asset GitHubAsset, target CoreDownloadTarget, ext string) bool {
	lowerName := strings.ToLower(asset.Name)
	if !strings.Contains(lowerName, target.OS) || !strings.Contains(lowerName, target.Arch) || !strings.HasSuffix(lowerName, ext) {
		return false
	}
	if target.OS != "linux" {
		return true
	}
	switch target.Libc {
	case "glibc":
		return strings.Contains(lowerName, "-glibc") || isUniversalLinuxAssetName(lowerName)
	case "musl":
		return strings.Contains(lowerName, "-musl") || isUniversalLinuxAssetName(lowerName)
	case "universal":
		return isUniversalLinuxAssetName(lowerName)
	default:
		return true
	}
}

func isUniversalLinuxAssetName(name string) bool {
	return strings.Contains(name, "linux") &&
		!strings.Contains(name, "-glibc") &&
		!strings.Contains(name, "-musl")
}

func describeCoreDownloadTarget(target CoreDownloadTarget) string {
	if target.OS == "linux" && target.Arch == "amd64" && target.Amd64Level != "" {
		if target.Libc != "" {
			return fmt.Sprintf("%s/%s/%s (%s)", target.OS, target.Arch, target.Amd64Level, target.Libc)
		}
		return fmt.Sprintf("%s/%s/%s", target.OS, target.Arch, target.Amd64Level)
	}
	if target.OS == "linux" && target.Libc != "" {
		return fmt.Sprintf("%s/%s (%s)", target.OS, target.Arch, target.Libc)
	}
	return fmt.Sprintf("%s/%s", target.OS, target.Arch)
}

func normalizeAmd64Level(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "v1", "1":
		return "v1"
	case "v2", "2":
		return "v2"
	case "v3", "3":
		return "v3"
	default:
		return ""
	}
}

func detectHostLinuxLibc() string {
	if runtime.GOOS != "linux" {
		return ""
	}
	if _, err := os.Stat("/etc/alpine-release"); err == nil {
		return "musl"
	}
	if matches, _ := filepath.Glob("/lib/ld-musl-*.so.1"); len(matches) > 0 {
		return "musl"
	}
	cmd := exec.Command("ldd", "--version")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return ""
	}
	lower := strings.ToLower(string(output))
	if strings.Contains(lower, "musl") {
		return "musl"
	}
	if strings.Contains(lower, "glibc") || strings.Contains(lower, "gnu libc") {
		return "glibc"
	}
	return ""
}

func (s *CoreManagerService) getArchName() string {
	switch runtime.GOARCH {
	case "amd64":
		return "amd64"
	case "386":
		return "386"
	case "arm64":
		return "arm64"
	case "arm":
		return "armv7"
	default:
		return runtime.GOARCH
	}
}

// DownloadCore 下载 sing-box 内核
func (s *CoreManagerService) DownloadCore(version string, target CoreDownloadTarget, requestedSessionID string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sessionID := StartCoreDownloadProgressSession("sing-box", requestedSessionID, false)
	defer func() {
		if r := recover(); r != nil {
			FinishCoreDownloadProgressError(sessionID, coreDownloadStageCompleted, fmt.Sprintf("%v", r))
			panic(r)
		}
	}()

	if err := EnsureManagedCoreLayout(); err != nil {
		FinishCoreDownloadProgressError(sessionID, coreDownloadStageDownloading, err.Error())
		return "", err
	}
	if err := cleanupStaleManagedCoreInstallWorkspaces(s.getCoreDir(), singboxCoreInstallStagePrefix, singboxCoreInstallBackupPrefix); err != nil {
		FinishCoreDownloadProgressError(sessionID, coreDownloadStageDownloading, err.Error())
		return "", err
	}
	if err := cleanupManagedCoreInstallWorkspaceArtifacts(s.getCoreDir(), s.getCoreBinName()); err != nil {
		FinishCoreDownloadProgressError(sessionID, coreDownloadStageDownloading, err.Error())
		return "", err
	}

	wasRunning := s.isRunning()
	if wasRunning {
		sharedCoreDownloadProgressStore.mu.Lock()
		if session := sharedCoreDownloadProgressStore.sessions[sessionID]; session != nil {
			session.runningBefore = true
			session.updatedAt = time.Now().Unix()
		}
		sharedCoreDownloadProgressStore.mu.Unlock()
	}
	failProgress := func(stage string, err error) {
		if err != nil {
			FinishCoreDownloadProgressError(sessionID, stage, err.Error())
		}
	}

	normalizedTarget := s.normalizeDownloadTarget(target)
	if strings.TrimSpace(target.OS) != "" && normalizedTarget.OS != runtime.GOOS {
		err := fmt.Errorf("requested core target %s cannot be installed on runtime %s/%s", describeCoreDownloadTarget(normalizedTarget), runtime.GOOS, runtime.GOARCH)
		failProgress(coreDownloadStageDownloading, err)
		return "", err
	}
	if strings.TrimSpace(target.Arch) != "" && normalizedTarget.Arch != s.getArchName() {
		err := fmt.Errorf("requested core target %s does not match runtime architecture %s", describeCoreDownloadTarget(normalizedTarget), s.getArchName())
		failProgress(coreDownloadStageDownloading, err)
		return "", err
	}
	if normalizedTarget.OS == "linux" && (normalizedTarget.Libc == "glibc" || normalizedTarget.Libc == "musl") {
		hostLibc := detectHostLinuxLibc()
		if hostLibc != "" && hostLibc != normalizedTarget.Libc {
			err := fmt.Errorf("requested core target %s does not match host libc %s", describeCoreDownloadTarget(normalizedTarget), hostLibc)
			failProgress(coreDownloadStageDownloading, err)
			return "", err
		}
	}

	downloadURL, err := s.getDownloadAsset(version, normalizedTarget)
	if err != nil {
		failProgress(coreDownloadStageDownloading, err)
		return "", err
	}

	logger.Info("开始下载 sing-box: ", downloadURL)
	SetCoreDownloadProgressStage(sessionID, coreDownloadStageDownloading)

	client := &http.Client{Timeout: 600 * time.Second}
	resp, err := client.Get(downloadURL)
	if err != nil {
		err = fmt.Errorf("下载失败: %v", err)
		failProgress(coreDownloadStageDownloading, err)
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		err = fmt.Errorf("下载失败，HTTP %d", resp.StatusCode)
		failProgress(coreDownloadStageDownloading, err)
		return "", err
	}
	SetCoreDownloadProgressTotals(sessionID, resp.ContentLength, resp.ContentLength <= 0)

	coreDir := s.getCoreDir()
	os.MkdirAll(coreDir, 0755)

	tmpExt := detectCoreArchiveExtFromURL(downloadURL)
	tmpFile := filepath.Join(coreDir, "sing-box-download"+tmpExt)

	out, err := os.Create(tmpFile)
	if err != nil {
		err = fmt.Errorf("创建临时文件失败: %v", err)
		failProgress(coreDownloadStageDownloading, err)
		return "", err
	}

	_, err = io.Copy(out, io.TeeReader(resp.Body, &coreDownloadProgressWriter{sessionID: sessionID}))
	if err != nil {
		_ = out.Close()
		os.Remove(tmpFile)
		err = fmt.Errorf("写入文件失败: %v", err)
		failProgress(coreDownloadStageDownloading, err)
		return "", err
	}
	if err = out.Close(); err != nil {
		os.Remove(tmpFile)
		err = fmt.Errorf("close temp file failed: %v", err)
		failProgress(coreDownloadStageDownloading, err)
		return "", err
	}

	SetCoreDownloadProgressStage(sessionID, coreDownloadStageReplacing)
	stageDir, cleanupStageDir, err := createManagedCoreInstallWorkspace(coreDir, singboxCoreInstallStagePrefix)
	if err != nil {
		os.Remove(tmpFile)
		failProgress(coreDownloadStageReplacing, err)
		return "", err
	}
	defer cleanupStageDir()

	err = s.installCoreFromArchiveFile(tmpFile, stageDir)

	os.Remove(tmpFile)

	if err != nil {
		err = fmt.Errorf("解压失败: %v", err)
		failProgress(coreDownloadStageReplacing, err)
		return "", err
	}

	binName := s.getCoreBinName()
	stagedBinPath := filepath.Join(stageDir, binName)

	SetCoreDownloadProgressStage(sessionID, coreDownloadStageValidating)
	if !s.validateCoreBinary(stagedBinPath) {
		err = fmt.Errorf("downloaded sing-box binary is not executable on current runtime %s/%s", runtime.GOOS, runtime.GOARCH)
		failProgress(coreDownloadStageValidating, err)
		return "", err
	}

	activation, activationStage, err := activateManagedCoreBinaryInstallWithRuntime(
		wasRunning,
		func() error {
			SetCoreDownloadProgressStage(sessionID, coreDownloadStageStopping)
			return s.stopCoreInternal()
		},
		func() {
			SetCoreDownloadProgressStage(sessionID, coreDownloadStageReplacing)
		},
		s.startCoreLocked,
		func() (*managedCoreBinaryActivation, error) {
			return activateManagedCoreBinaryInstall(coreDir, binName, stageDir, singboxCoreInstallBackupPrefix)
		},
	)
	if err != nil {
		if strings.TrimSpace(activationStage) == "" {
			activationStage = coreDownloadStageReplacing
		}
		failProgress(activationStage, err)
		return "", err
	}
	finalized := false
	defer func() {
		if !finalized {
			if rollbackErr := activation.Rollback(); rollbackErr != nil {
				logger.Warning("rollback sing-box staged install failed: ", rollbackErr)
			}
		}
	}()

	binPath := filepath.Join(coreDir, binName)

	localVersion, _ := s.getLocalVersion(binPath)
	logger.Info("sing-box 下载完成, 版本: ", localVersion)
	if err := s.SaveDownloadTarget(normalizedTarget); err != nil {
		logger.Warning("failed to save core download preference: ", err)
	}

	if wasRunning {
		SetCoreDownloadProgressStage(sessionID, coreDownloadStageStarting)
		if err = s.startCoreLocked(); err != nil {
			rollbackErr := activation.Rollback()
			finalized = true
			if rollbackErr == nil {
				if restartErr := s.startCoreLocked(); restartErr != nil {
					err = fmt.Errorf("下载完成，但新版本自动启动失败: %v；已回滚旧版本，但旧版本恢复启动失败: %v", err, restartErr)
				} else {
					err = fmt.Errorf("下载完成，但新版本自动启动失败，已自动回滚到旧版本: %v", err)
				}
			} else {
				err = fmt.Errorf("下载完成，但自动启动失败: %v；回滚旧版本失败: %v", err, rollbackErr)
			}
			failProgress(coreDownloadStageStarting, err)
			return localVersion, err
		}
		SetCoreDownloadProgressStage(sessionID, coreDownloadStageStarted)
		time.Sleep(900 * time.Millisecond)
	}
	if err := activation.Commit(); err != nil {
		logger.Warning("cleanup sing-box install backup workspace failed: ", err)
	}
	finalized = true

	FinishCoreDownloadProgressSuccess(sessionID, coreDownloadStageCompleted)
	return localVersion, nil
}

func (s *CoreManagerService) extractZip(zipPath, destDir, binName string) error {
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}
	defer r.Close()

	for _, f := range r.File {
		name := filepath.Base(f.Name)
		if name == binName {
			rc, err := f.Open()
			if err != nil {
				return err
			}
			defer rc.Close()

			destPath := filepath.Join(destDir, binName)
			outFile, err := os.Create(destPath)
			if err != nil {
				return err
			}
			defer outFile.Close()

			_, err = io.Copy(outFile, rc)
			return err
		}
	}
	return fmt.Errorf("在压缩包中未找到 %s", binName)
}

func (s *CoreManagerService) extractTarGz(tarGzPath, destDir, binName string) error {
	f, err := os.Open(tarGzPath)
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

	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}

		name := filepath.Base(header.Name)
		if name == binName && header.Typeflag == tar.TypeReg {
			destPath := filepath.Join(destDir, binName)
			outFile, err := os.Create(destPath)
			if err != nil {
				return err
			}
			defer outFile.Close()

			_, err = io.Copy(outFile, tr)
			return err
		}
	}
	return fmt.Errorf("在压缩包中未找到 %s", binName)
}

// =====================================================================
// 内核启停控制
// =====================================================================

// StartCore 启动内核
// Linux: 创建 systemd 服务文件 → daemon-reload → systemctl start
// Windows: 直接启动进程
func (s *CoreManagerService) StartCore() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := EnsureManagedCoreLayout(); err != nil {
		return err
	}

	if s.isRunning() {
		return fmt.Errorf("内核已在运行中")
	}

	binPath := s.getCoreBinPath()
	if _, err := os.Stat(binPath); os.IsNotExist(err) {
		return fmt.Errorf("内核文件不存在: %s", binPath)
	}
	if !s.validateCoreBinary(binPath) {
		return fmt.Errorf("内核文件无法在当前系统执行，请确认下载的架构与系统匹配（当前 %s/%s）", runtime.GOOS, runtime.GOARCH)
	}

	s.regenerateRuntimeConfig()
	configPath := s.getConfigPath()
	configExists, err := ManagedRuntimeFileExists(configPath)
	if err != nil {
		return err
	}
	if !configExists {
		return fmt.Errorf("配置文件不存在: %s", configPath)
	}
	if err := MaterializeManagedRuntimeCoreFile(configPath, managedCoreConfigMaterializeTTL); err != nil {
		return fmt.Errorf("准备配置文件失败: %v", err)
	}

	coreDir := s.getCoreDir()
	absCoreDir, _ := filepath.Abs(coreDir)
	if err := CheckSingboxRuntimeConfig(binPath, configPath, absCoreDir); err != nil {
		DiscardMaterializedManagedRuntimeCoreFile(configPath)
		return err
	}

	if runtime.GOOS == "windows" {
		err = s.startCoreWindows(absCoreDir)
	} else {
		err = s.startCoreLinux(absCoreDir)
	}
	if err != nil {
		DiscardMaterializedManagedRuntimeCoreFile(configPath)
		return err
	}
	if runtime.GOOS == "linux" {
		if markerErr := markManagedCoreShouldRun("singbox"); markerErr != nil {
			logger.Warning("failed to persist sing-box runtime marker: ", markerErr)
		}
	}
	return nil
}

// startCoreLocked starts core while caller already holds s.mu.
func (s *CoreManagerService) startCoreLocked() error {
	if err := EnsureManagedCoreLayout(); err != nil {
		return err
	}

	binPath := s.getCoreBinPath()
	if _, err := os.Stat(binPath); os.IsNotExist(err) {
		return fmt.Errorf("内核文件不存在: %s", binPath)
	}
	if !s.validateCoreBinary(binPath) {
		return fmt.Errorf("内核文件无法在当前系统执行，请确认下载的架构与系统匹配（当前 %s/%s）", runtime.GOOS, runtime.GOARCH)
	}

	s.regenerateRuntimeConfig()
	configPath := s.getConfigPath()
	configExists, err := ManagedRuntimeFileExists(configPath)
	if err != nil {
		return err
	}
	if !configExists {
		return fmt.Errorf("配置文件不存在: %s", configPath)
	}
	if err := MaterializeManagedRuntimeCoreFile(configPath, managedCoreConfigMaterializeTTL); err != nil {
		return fmt.Errorf("准备配置文件失败: %v", err)
	}

	coreDir := s.getCoreDir()
	absCoreDir, _ := filepath.Abs(coreDir)
	if err := CheckSingboxRuntimeConfig(binPath, configPath, absCoreDir); err != nil {
		DiscardMaterializedManagedRuntimeCoreFile(configPath)
		return err
	}

	if runtime.GOOS == "windows" {
		err = s.startCoreWindows(absCoreDir)
	} else {
		err = s.startCoreLinux(absCoreDir)
	}
	if err != nil {
		DiscardMaterializedManagedRuntimeCoreFile(configPath)
		return err
	}
	if runtime.GOOS == "linux" {
		if markerErr := markManagedCoreShouldRun("singbox"); markerErr != nil {
			logger.Warning("failed to persist sing-box runtime marker: ", markerErr)
		}
	}
	return nil
}

// StopCore 停止内核（UI 点击停止）
// Linux: systemctl stop → disable → 删除 service 文件 → daemon-reload
// Windows: 直接终止进程
func (s *CoreManagerService) StopCore() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if runtime.GOOS == "linux" {
		err := s.stopCoreLinuxFull()
		if err == nil {
			clearManagedCoreShouldRun("singbox")
		}
		return err
	}
	return s.stopCoreInternal()
}

// DeleteCore stops running core/service and removes the core binary.
func (s *CoreManagerService) DeleteCore() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := EnsureManagedCoreLayout(); err != nil {
		return err
	}

	if runtime.GOOS == "linux" {
		if err := s.stopCoreLinuxFull(); err != nil {
			return err
		}
		s.cleanupLegacySingboxSystemdServices()
		s.removeSingboxSystemdService()
		clearManagedCoreShouldRun("singbox")
	} else {
		if err := s.stopCoreInternal(); err != nil {
			return err
		}
	}

	binPath := s.getCoreBinPath()
	if err := os.Remove(binPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove core binary %s: %v", binPath, err)
	}
	if sameManagedCorePath(s.getCoreDir(), GetManagedCoreRootDir()) {
		if err := cleanupManagedSingboxRootRuntimeArtifacts(s.getCoreDir()); err != nil {
			return err
		}
	} else {
		if err := cleanupManagedCoreRuntimeArtifacts(s.getCoreDir(), s.getCoreBinName()); err != nil {
			return err
		}
	}

	s.isStarted = false
	s.coreCmd = nil
	return nil
}

// RestartCore 重启内核
func (s *CoreManagerService) RestartCore() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := EnsureManagedCoreLayout(); err != nil {
		return err
	}

	if runtime.GOOS == "linux" && !shouldUseDirectManagedCoreRuntime() && s.isSingboxSystemdActive() {
		s.regenerateRuntimeConfig()
		configPath := s.getConfigPath()
		configExists, err := ManagedRuntimeFileExists(configPath)
		if err != nil {
			return err
		}
		if !configExists {
			return fmt.Errorf("配置文件不存在: %s", configPath)
		}
		if err := MaterializeManagedRuntimeCoreFile(configPath, managedCoreConfigMaterializeTTL); err != nil {
			return fmt.Errorf("准备配置文件失败: %v", err)
		}
		coreDir := s.getCoreDir()
		absCoreDir, _ := filepath.Abs(coreDir)
		if err := CheckSingboxRuntimeConfig(s.getCoreBinPath(), configPath, absCoreDir); err != nil {
			DiscardMaterializedManagedRuntimeCoreFile(configPath)
			return err
		}
		if err := s.createSingboxSystemdService(s.getCoreBinPath(), configPath, absCoreDir); err != nil {
			DiscardMaterializedManagedRuntimeCoreFile(configPath)
			return fmt.Errorf("refresh systemd service for sing-box failed: %v", err)
		}
		cmd := exec.Command("systemctl", "restart", singboxSystemdName)
		output, err := cmd.CombinedOutput()
		if err != nil {
			DiscardMaterializedManagedRuntimeCoreFile(configPath)
			diagnostics := collectSystemdStartupDiagnostics(singboxSystemdName, systemdCoreJournalTailLines)
			message := buildSystemdActivationErrorMessage(
				singboxSystemdName,
				systemdUnitActivationResult{State: "restart-command-failed", LastErr: err},
				string(output),
				diagnostics,
			)
			logger.Warning("systemd restart sing-box failed: ", message)
			return fmt.Errorf("%s", message)
		}
		waitResult := waitForSystemdUnitActive(singboxSystemdName, systemdCoreStartWaitTimeout)
		if waitResult.State != "active" {
			DiscardMaterializedManagedRuntimeCoreFile(configPath)
			diagnostics := collectSystemdStartupDiagnostics(singboxSystemdName, systemdCoreJournalTailLines)
			message := buildSystemdActivationErrorMessage(singboxSystemdName, waitResult, string(output), diagnostics)
			logger.Warning("systemd 重启 sing-box 后未进入 active: ", message)
			return fmt.Errorf("%s", message)
		}
		stableResult := waitForSystemdUnitRemainActive(singboxSystemdName, systemdCorePostActiveHold)
		if stableResult.State != "active" {
			DiscardMaterializedManagedRuntimeCoreFile(configPath)
			diagnostics := collectSystemdStartupDiagnostics(singboxSystemdName, systemdCoreJournalTailLines)
			message := buildSystemdActivationErrorMessage(
				singboxSystemdName,
				stableResult,
				"unit dropped out of active shortly after restart",
				diagnostics,
			)
			logger.Warning("sing-box systemd 重启后未保持 active: ", message)
			return fmt.Errorf("%s", message)
		}
		s.isStarted = true
		if markerErr := markManagedCoreShouldRun("singbox"); markerErr != nil {
			logger.Warning("failed to persist sing-box runtime marker: ", markerErr)
		}
		logger.Info("sing-box 已通过 systemd 重启")
		return nil
	}

	// 非 systemd 场景或 Windows：先停再启
	s.stopCoreInternal()
	time.Sleep(1 * time.Second)
	s.regenerateRuntimeConfig()

	binPath := s.getCoreBinPath()
	if _, err := os.Stat(binPath); os.IsNotExist(err) {
		return fmt.Errorf("内核文件不存在")
	}
	configPath := s.getConfigPath()
	configExists, err := ManagedRuntimeFileExists(configPath)
	if err != nil {
		return err
	}
	if !configExists {
		return fmt.Errorf("配置文件不存在")
	}
	if err := MaterializeManagedRuntimeCoreFile(configPath, managedCoreConfigMaterializeTTL); err != nil {
		return fmt.Errorf("准备配置文件失败: %v", err)
	}

	coreDir := s.getCoreDir()
	absCoreDir, _ := filepath.Abs(coreDir)
	if err := CheckSingboxRuntimeConfig(binPath, configPath, absCoreDir); err != nil {
		DiscardMaterializedManagedRuntimeCoreFile(configPath)
		return err
	}

	if runtime.GOOS == "windows" {
		err = s.startCoreWindows(absCoreDir)
	} else {
		err = s.startCoreLinux(absCoreDir)
	}
	if err != nil {
		DiscardMaterializedManagedRuntimeCoreFile(configPath)
		return err
	}
	if runtime.GOOS == "linux" {
		if markerErr := markManagedCoreShouldRun("singbox"); markerErr != nil {
			logger.Warning("failed to persist sing-box runtime marker: ", markerErr)
		}
	}
	return nil
}

// =====================================================================
// 运行状态检查
// =====================================================================

// IsRunning reports whether the core process is currently running.
// It uses the same internal detection path as GetCoreStatus().Running,
// but avoids querying local version details.
func (s *CoreManagerService) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.isRunning()
}

// isRunning 检查内核是否在运行
func (s *CoreManagerService) isRunning() bool {
	// Linux 宿主机模式优先检查 systemd 状态
	if runtime.GOOS == "linux" && !shouldUseDirectManagedCoreRuntime() {
		if s.isSingboxSystemdActive() {
			s.isStarted = true
			return true
		}
	}

	// 检查我们直接启动的进程（Windows 或 Linux fallback）
	if s.isStarted && s.coreCmd != nil && s.coreCmd.Process != nil {
		if s.isProcessAlive(s.coreCmd.Process.Pid) {
			return true
		}
		// 进程已退出
		s.isStarted = false
		s.coreCmd = nil
	}

	if runtime.GOOS == "linux" && shouldUseDirectManagedCoreRuntime() && isManagedCoreProcessRunningByBinaryPath(s.getCoreBinPath()) {
		s.isStarted = true
		return true
	}

	return false
}

// isSingboxSystemdActive 检查 sing-box systemd 服务是否 active
func (s *CoreManagerService) isSingboxSystemdActive() bool {
	cmd := exec.Command("systemctl", "is-active", "--quiet", singboxSystemdName)
	err := cmd.Run()
	return err == nil
}

// isSingboxSystemdExists 检查 sing-box systemd 服务文件是否存在
func (s *CoreManagerService) isSingboxSystemdExists() bool {
	_, err := os.Stat(getSingboxServiceFilePath())
	return err == nil
}

// isProcessAlive 检查 PID 对应的进程是否存在
func (s *CoreManagerService) isProcessAlive(pid int) bool {
	return managedCoreProcessPIDAlive(pid)
}

// =====================================================================
// Windows 进程管理
// =====================================================================

func (s *CoreManagerService) startCoreWindows(coreDir string) error {
	binName := s.getCoreBinName()
	binPath := filepath.Join(coreDir, binName)
	configPath := s.getConfigPath()

	s.coreCmd = exec.Command(binPath, "run", "-c", configPath)
	s.coreCmd.Dir = coreDir
	s.coreCmd.Stdout = nil
	s.coreCmd.Stderr = nil
	s.stdout = nil
	s.stderr = nil

	err := s.coreCmd.Start()
	if err != nil {
		s.coreCmd = nil
		return fmt.Errorf("启动内核失败: %v", err)
	}

	s.isStarted = true
	logger.Info("sing-box 内核已启动 (Windows), PID: ", s.coreCmd.Process.Pid)

	startedCmd := s.coreCmd
	waitManagedCoreCommandAsync(startedCmd, func() {
		s.mu.Lock()
		if s.coreCmd == startedCmd {
			s.isStarted = false
			s.coreCmd = nil
			closeManagedCoreDirectStdStreams(s.stdout, s.stderr)
			s.stdout = nil
			s.stderr = nil
			logger.Info("sing-box 内核进程已退出")
		}
		s.mu.Unlock()
	})

	return nil
}

// =====================================================================
// Linux systemd 管理
// =====================================================================

// startCoreLinux 通过 systemd 启动 sing-box
// 流程: 生成 service 文件 → daemon-reload → systemctl start
func (s *CoreManagerService) startCoreLinux(coreDir string) error {
	if shouldUseDirectManagedCoreRuntime() {
		return s.startCoreDirectLinux(coreDir)
	}

	binPath := s.getCoreBinPath()
	configPath := s.getConfigPath()
	s.cleanupLegacySingboxSystemdServices()

	// 1. 生成 systemd service 文件
	if err := s.createSingboxSystemdService(binPath, configPath, coreDir); err != nil {
		logger.Warning("创建 systemd 服务文件失败，尝试直接启动: ", err)
		return fmt.Errorf("create systemd service for sing-box failed: %v", err)
	}

	// 2. systemctl start
	_ = exec.Command("systemctl", "reset-failed", singboxSystemdName).Run()
	startCmd := exec.Command("systemctl", "start", singboxSystemdName)
	startOutput, startErr := startCmd.CombinedOutput()
	if startErr != nil {
		diagnostics := collectSystemdStartupDiagnostics(singboxSystemdName, systemdCoreJournalTailLines)
		message := buildSystemdActivationErrorMessage(
			singboxSystemdName,
			systemdUnitActivationResult{State: "start-command-failed", LastErr: startErr},
			string(startOutput),
			diagnostics,
		)
		logger.Warning("systemd start sing-box failed: ", message)
		return fmt.Errorf("%s", message)
	}

	// 3. 验证是否真正启动并保持 active
	waitResult := waitForSystemdUnitActive(singboxSystemdName, systemdCoreStartWaitTimeout)
	if waitResult.State != "active" {
		diagnostics := collectSystemdStartupDiagnostics(singboxSystemdName, systemdCoreJournalTailLines)
		message := buildSystemdActivationErrorMessage(
			singboxSystemdName,
			waitResult,
			string(startOutput),
			diagnostics,
		)
		logger.Warning("systemd 启动 sing-box 后未进入 active: ", message)
		return fmt.Errorf("%s", message)
	}
	stableResult := waitForSystemdUnitRemainActive(singboxSystemdName, systemdCorePostActiveHold)
	if stableResult.State != "active" {
		diagnostics := collectSystemdStartupDiagnostics(singboxSystemdName, systemdCoreJournalTailLines)
		message := buildSystemdActivationErrorMessage(
			singboxSystemdName,
			stableResult,
			"unit dropped out of active shortly after start",
			diagnostics,
		)
		logger.Warning("sing-box systemd 启动后未保持 active: ", message)
		return fmt.Errorf("%s", message)
	}

	s.isStarted = true
	logger.Info("sing-box 已通过 systemd 启动 (服务: ", singboxSystemdName, ")")
	return nil
}

// startCoreDirectLinux 不使用 systemd，直接启动进程（fallback）
func (s *CoreManagerService) startCoreDirectLinux(coreDir string) error {
	binPath := s.getCoreBinPath()
	configPath := s.getConfigPath()

	s.coreCmd = exec.Command(binPath, "run", "-c", configPath)
	s.coreCmd.Dir = coreDir
	s.stdout, s.stderr = resolveManagedCoreDirectStdStreams()
	s.coreCmd.Stdout = s.stdout
	s.coreCmd.Stderr = s.stderr

	err := s.coreCmd.Start()
	if err != nil {
		closeManagedCoreDirectStdStreams(s.stdout, s.stderr)
		s.stdout = nil
		s.stderr = nil
		s.coreCmd = nil
		return fmt.Errorf("直接启动内核失败: %v", err)
	}

	s.isStarted = true
	logger.Info("sing-box 内核已直接启动 (Linux, 无systemd), PID: ", s.coreCmd.Process.Pid)

	startedCmd := s.coreCmd
	waitManagedCoreCommandAsync(startedCmd, func() {
		s.mu.Lock()
		if s.coreCmd == startedCmd {
			s.isStarted = false
			s.coreCmd = nil
			closeManagedCoreDirectStdStreams(s.stdout, s.stderr)
			s.stdout = nil
			s.stderr = nil
			logger.Info("sing-box 内核进程已退出")
		}
		s.mu.Unlock()
	})

	return nil
}

// stopCoreLinuxFull 完整的 Linux 停止流程：
// 1. systemctl stop（停止进程）
// 2. systemctl disable（取消开机启动）
// 3. 删除 service 文件
// 4. daemon-reload + reset-failed
func (s *CoreManagerService) stopCoreLinuxFull() error {
	if shouldUseDirectManagedCoreRuntime() {
		if err := s.stopCoreInternal(); err != nil {
			return err
		}
		s.cleanupLegacySingboxSystemdServices()
		s.removeSingboxSystemdService()
		clearManagedCoreShouldRun("singbox")
		logger.Info("sing-box 内核已停止，Docker/直启模式运行标记已清理")
		return nil
	}

	s.cleanupLegacySingboxSystemdServices()
	// 先通过 systemd 停止
	if s.isSingboxSystemdActive() {
		cmd := exec.Command("systemctl", "stop", singboxSystemdName)
		if err := cmd.Run(); err != nil {
			logger.Warning("systemctl stop ", singboxSystemdName, " 失败: ", err)
		} else {
			logger.Info("sing-box 已通过 systemd 停止")
		}
	}

	// 如果还有直接启动的进程，也停掉
	if s.coreCmd != nil && s.coreCmd.Process != nil {
		pid := s.coreCmd.Process.Pid
		_ = s.coreCmd.Process.Signal(os.Interrupt)
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			if !s.isProcessAlive(pid) {
				break
			}
			time.Sleep(120 * time.Millisecond)
		}
		if s.isProcessAlive(pid) {
			_ = s.coreCmd.Process.Kill()
		}
	}

	// 删除 systemd 服务注册
	s.removeSingboxSystemdService()

	s.isStarted = false
	s.coreCmd = nil
	closeManagedCoreDirectStdStreams(s.stdout, s.stderr)
	s.stdout = nil
	s.stderr = nil
	clearManagedCoreShouldRun("singbox")
	logger.Info("sing-box 内核已停止，systemd 服务已清理")
	return nil
}

// stopCoreInternal 内部停止方法（Windows 或非 systemd 场景）
func (s *CoreManagerService) stopCoreInternal() error {
	// Linux 宿主机模式：先尝试 systemd stop
	if runtime.GOOS == "linux" && !shouldUseDirectManagedCoreRuntime() && s.isSingboxSystemdActive() {
		cmd := exec.Command("systemctl", "stop", singboxSystemdName)
		if err := cmd.Run(); err == nil {
			time.Sleep(300 * time.Millisecond)
			if s.isSingboxSystemdActive() {
				return fmt.Errorf("sing-box systemd service is still active after stop request")
			}
			logger.Info("sing-box 已通过 systemd 停止")
			s.isStarted = false
			s.coreCmd = nil
			closeManagedCoreDirectStdStreams(s.stdout, s.stderr)
			s.stdout = nil
			s.stderr = nil
			return nil
		} else {
			return fmt.Errorf("failed to stop sing-box systemd service: %v", err)
		}
	}

	// 直接停止进程
	if runtime.GOOS == "linux" && shouldUseDirectManagedCoreRuntime() {
		if err := terminateManagedCoreProcessesByBinaryPath(s.getCoreBinPath(), 5*time.Second); err != nil {
			return fmt.Errorf("failed to stop sing-box direct runtime process: %v", err)
		}
		s.isStarted = false
		s.coreCmd = nil
		closeManagedCoreDirectStdStreams(s.stdout, s.stderr)
		s.stdout = nil
		s.stderr = nil
		logger.Info("sing-box 内核已停止")
		return nil
	}

	if s.coreCmd != nil && s.coreCmd.Process != nil {
		pid := s.coreCmd.Process.Pid
		logger.Info("正在停止 sing-box 内核, PID: ", pid)

		if runtime.GOOS == "windows" {
			killCmd := exec.Command("taskkill", "/F", "/T", "/PID", fmt.Sprintf("%d", pid))
			if err := killCmd.Run(); err != nil {
				return fmt.Errorf("failed to stop sing-box process %d: %v", pid, err)
			}
		} else {
			if err := s.coreCmd.Process.Signal(os.Interrupt); err != nil {
				return fmt.Errorf("failed to interrupt sing-box process %d: %v", pid, err)
			}
			deadline := time.Now().Add(5 * time.Second)
			for time.Now().Before(deadline) {
				if !s.isProcessAlive(pid) {
					break
				}
				time.Sleep(120 * time.Millisecond)
			}
			if s.isProcessAlive(pid) {
				if err := s.coreCmd.Process.Kill(); err != nil {
					return fmt.Errorf("failed to kill sing-box process %d: %v", pid, err)
				}
			}
		}
		if s.isProcessAlive(pid) {
			return fmt.Errorf("sing-box process %d is still alive after stop request", pid)
		}

		s.isStarted = false
		s.coreCmd = nil
		closeManagedCoreDirectStdStreams(s.stdout, s.stderr)
		s.stdout = nil
		s.stderr = nil
		logger.Info("sing-box 内核已停止")
		return nil
	}

	s.isStarted = false
	closeManagedCoreDirectStdStreams(s.stdout, s.stderr)
	s.stdout = nil
	s.stderr = nil
	return nil
}

// =====================================================================
// systemd 服务文件管理
// =====================================================================

// createSingboxSystemdService 创建 sing-box 的 systemd 服务文件
func (s *CoreManagerService) cleanupLegacySingboxSystemdServices() {
	for _, serviceName := range legacySingboxSystemdNames {
		if serviceName == singboxSystemdName {
			continue
		}
		s.removeSystemdServiceByName(serviceName)
	}
}

func (s *CoreManagerService) removeSystemdServiceByName(serviceName string) {
	if serviceName == "" {
		return
	}

	useSystemctl := runtime.GOOS == "linux" && !shouldUseDirectManagedCoreRuntime()
	if useSystemctl {
		exec.Command("systemctl", "stop", serviceName).Run()
		exec.Command("systemctl", "disable", serviceName).Run()
	}

	removed := false
	for _, servicePath := range getSystemdServiceFileCandidates(serviceName) {
		if _, err := os.Stat(servicePath); err != nil {
			continue
		}
		if err := os.Remove(servicePath); err != nil {
			logger.Warning("failed to remove systemd service file ", servicePath, ": ", err)
			continue
		}
		removed = true
	}

	if removed {
		if useSystemctl {
			exec.Command("systemctl", "daemon-reload").Run()
			exec.Command("systemctl", "reset-failed").Run()
		}
		logger.Info("removed systemd service: ", serviceName)
	}
}

func (s *CoreManagerService) createSingboxSystemdService(binPath, configPath, workDir string) error {
	controlPath := getSystemdControlBinaryPath()
	serviceContent := buildSingboxSystemdServiceContent(controlPath, binPath, configPath, workDir)

	servicePath := getSingboxServiceFilePath()
	err := os.WriteFile(servicePath, []byte(serviceContent), 0644)
	if err != nil {
		return fmt.Errorf("无法写入 systemd 服务文件 %s: %v", servicePath, err)
	}
	if err := verifySystemdUnitFile(servicePath); err != nil {
		return err
	}

	// daemon-reload 使 systemd 加载新文件
	if err := exec.Command("systemctl", "daemon-reload").Run(); err != nil {
		return fmt.Errorf("systemctl daemon-reload 失败: %v", err)
	}

	logger.Info("已创建 systemd 服务文件: ", servicePath)
	return nil
}

// removeSingboxSystemdService 删除 sing-box 的 systemd 服务文件并清理
func (s *CoreManagerService) removeSingboxSystemdService() {
	s.removeSystemdServiceByName(singboxSystemdName)

	servicePath := getSingboxServiceFilePath()

	if _, err := os.Stat(servicePath); os.IsNotExist(err) {
		return // 文件不存在，无需删除
	}

	useSystemctl := runtime.GOOS == "linux" && !shouldUseDirectManagedCoreRuntime()
	if useSystemctl {
		exec.Command("systemctl", "disable", singboxSystemdName).Run()
	}

	// 删除服务文件
	err := os.Remove(servicePath)
	if err != nil {
		logger.Warning("无法删除 systemd 服务文件: ", err)
		return
	}

	if useSystemctl {
		exec.Command("systemctl", "daemon-reload").Run()
		exec.Command("systemctl", "reset-failed").Run()
	}

	logger.Info("已删除 ", singboxSystemdName, " systemd 服务")
}

func normalizeCoreAutoCheckIntervalHours(raw string) int {
	raw = strings.TrimSpace(strings.ToLower(raw))
	raw = strings.TrimSuffix(raw, "h")
	if raw == "" {
		return 12
	}
	hours, err := strconv.Atoi(raw)
	if err != nil || hours <= 0 {
		return 12
	}
	return hours
}

func (s *CoreManagerService) getCoreAutoCheckSettings() (enabled bool, intervalHours int, lastCheckedAt int64, err error) {
	settingSvc := &SettingService{}

	enabled, err = settingSvc.getBool(coreAutoCheckEnabledKey)
	if err != nil {
		return false, 12, 0, err
	}

	intervalRaw, err := settingSvc.getString(coreAutoCheckIntervalHoursKey)
	if err != nil {
		return false, 12, 0, err
	}
	intervalHours = normalizeCoreAutoCheckIntervalHours(intervalRaw)

	lastRaw, err := settingSvc.getString(coreAutoCheckLastAtKey)
	if err != nil {
		return false, 12, 0, err
	}
	lastRaw = strings.TrimSpace(lastRaw)
	if lastRaw == "" {
		return enabled, intervalHours, 0, nil
	}

	lastCheckedAt, parseErr := strconv.ParseInt(lastRaw, 10, 64)
	if parseErr != nil || lastCheckedAt < 0 {
		lastCheckedAt = 0
	}
	return enabled, intervalHours, lastCheckedAt, nil
}

func (s *CoreManagerService) buildCoreUpdateInfo() (*CoreUpdateInfo, error) {
	enabled, intervalHours, lastCheckedAt, err := s.getCoreAutoCheckSettings()
	if err != nil {
		return nil, err
	}

	settingSvc := &SettingService{}
	latestStable, err := settingSvc.getString(coreAutoCheckLatestStableKey)
	if err != nil {
		return nil, err
	}
	latestAlpha, err := settingSvc.getString(coreAutoCheckLatestAlphaKey)
	if err != nil {
		return nil, err
	}
	pendingStable, err := settingSvc.getString(coreAutoCheckPendingStableKey)
	if err != nil {
		return nil, err
	}
	pendingAlpha, err := settingSvc.getString(coreAutoCheckPendingAlphaKey)
	if err != nil {
		return nil, err
	}

	info := &CoreUpdateInfo{
		Enabled:       enabled,
		IntervalHours: intervalHours,
		LastCheckedAt: lastCheckedAt,
		LatestStable:  latestStable,
		LatestAlpha:   latestAlpha,
		PendingStable: pendingStable,
		PendingAlpha:  pendingAlpha,
	}
	if pendingStable != "" {
		info.UpdateCount++
	}
	if pendingAlpha != "" {
		info.UpdateCount++
	}
	info.HasUpdate = info.UpdateCount > 0
	return info, nil
}

// SetCoreAutoCheckSettings updates auto-check switch and check interval (hours).
func (s *CoreManagerService) SetCoreAutoCheckSettings(enabled bool, intervalHours int) error {
	if intervalHours <= 0 {
		intervalHours = 12
	}

	coreAutoCheckMu.Lock()
	defer coreAutoCheckMu.Unlock()

	settingSvc := &SettingService{}
	if err := settingSvc.setString(coreAutoCheckEnabledKey, strconv.FormatBool(enabled)); err != nil {
		return err
	}
	if err := settingSvc.setString(coreAutoCheckIntervalHoursKey, strconv.Itoa(intervalHours)); err != nil {
		return err
	}

	if !enabled {
		if err := settingSvc.setString(coreAutoCheckPendingStableKey, ""); err != nil {
			return err
		}
		if err := settingSvc.setString(coreAutoCheckPendingAlphaKey, ""); err != nil {
			return err
		}
	}

	return nil
}

// ClearCoreUpdatePending clears pending update markers.
func (s *CoreManagerService) ClearCoreUpdatePending() error {
	coreAutoCheckMu.Lock()
	defer coreAutoCheckMu.Unlock()

	settingSvc := &SettingService{}
	if err := settingSvc.setString(coreAutoCheckPendingStableKey, ""); err != nil {
		return err
	}
	if err := settingSvc.setString(coreAutoCheckPendingAlphaKey, ""); err != nil {
		return err
	}
	return nil
}

// GetCoreUpdateInfo returns current auto-check settings and update markers.
// If forceCheck is true, an immediate remote check will be attempted.
func (s *CoreManagerService) GetCoreUpdateInfo(forceCheck bool) (*CoreUpdateInfo, error) {
	if forceCheck {
		if err := s.CheckAndMarkCoreUpdates(true); err != nil {
			logger.Warning("check core updates failed: ", err)
		}
	}
	return s.buildCoreUpdateInfo()
}

func (s *CoreManagerService) fetchLatestStableTag(client *http.Client) (string, error) {
	req, err := http.NewRequest("GET", "https://api.github.com/repos/SagerNet/sing-box/releases/latest", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "kwor")

	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request latest stable release failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub latest release API returned %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}

	var release GitHubRelease
	if err = json.Unmarshal(body, &release); err != nil {
		return "", fmt.Errorf("failed to parse latest stable release: %v", err)
	}
	return strings.TrimSpace(release.TagName), nil
}

func (s *CoreManagerService) fetchLatestAlphaTag(client *http.Client) (string, error) {
	const (
		perPage  = 100
		maxPages = 3
	)

	for page := 1; page <= maxPages; page++ {
		releases, err := s.fetchGitHubReleasePage(client, page, perPage)
		if err != nil {
			return "", err
		}
		if len(releases) == 0 {
			break
		}
		for _, release := range releases {
			if release.Prerelease {
				return strings.TrimSpace(release.TagName), nil
			}
		}
		if len(releases) < perPage {
			break
		}
	}

	return "", nil
}

// CheckAndMarkCoreUpdates checks latest stable/alpha versions and updates pending markers when changed.
func (s *CoreManagerService) CheckAndMarkCoreUpdates(force bool) error {
	coreAutoCheckMu.Lock()
	defer coreAutoCheckMu.Unlock()

	enabled, intervalHours, lastCheckedAt, err := s.getCoreAutoCheckSettings()
	if err != nil {
		return err
	}
	if !enabled {
		return nil
	}

	now := time.Now().Unix()
	if !force && lastCheckedAt > 0 {
		nextDueAt := lastCheckedAt + int64(intervalHours)*3600
		if now < nextDueAt {
			return nil
		}
	}

	client := &http.Client{Timeout: 30 * time.Second}
	latestStable, err := s.fetchLatestStableTag(client)
	if err != nil {
		return err
	}
	latestAlpha, err := s.fetchLatestAlphaTag(client)
	if err != nil {
		return err
	}

	settingSvc := &SettingService{}
	prevStable, err := settingSvc.getString(coreAutoCheckLatestStableKey)
	if err != nil {
		return err
	}
	prevAlpha, err := settingSvc.getString(coreAutoCheckLatestAlphaKey)
	if err != nil {
		return err
	}

	if err = settingSvc.setString(coreAutoCheckLastAtKey, strconv.FormatInt(now, 10)); err != nil {
		return err
	}

	if latestStable != "" && latestStable != prevStable {
		if err = settingSvc.setString(coreAutoCheckLatestStableKey, latestStable); err != nil {
			return err
		}
		if err = settingSvc.setString(coreAutoCheckPendingStableKey, latestStable); err != nil {
			return err
		}
	}

	if latestAlpha != "" && latestAlpha != prevAlpha {
		if err = settingSvc.setString(coreAutoCheckLatestAlphaKey, latestAlpha); err != nil {
			return err
		}
		if err = settingSvc.setString(coreAutoCheckPendingAlphaKey, latestAlpha); err != nil {
			return err
		}
	}

	return nil
}

// GetRemoteVersionsAll fetches release versions across multiple pages.
// This keeps old versions (e.g. 1.11.x) available instead of only the latest page.
func (s *CoreManagerService) GetRemoteVersionsAll(channel string) (*VersionListResponse, error) {
	const (
		perPage  = 100
		maxPages = 20
	)

	client := &http.Client{Timeout: 30 * time.Second}
	result := &VersionListResponse{
		Versions: make([]VersionItem, 0),
	}
	seenTags := make(map[string]struct{})

	for page := 1; page <= maxPages; page++ {
		releases, err := s.fetchGitHubReleasePage(client, page, perPage)
		if err != nil {
			return nil, err
		}
		if len(releases) == 0 {
			break
		}

		for _, r := range releases {
			if !shouldIncludeRelease(channel, r.Prerelease) {
				continue
			}
			if _, ok := seenTags[r.TagName]; ok {
				continue
			}
			seenTags[r.TagName] = struct{}{}
			result.Versions = append(result.Versions, VersionItem{
				TagName:     r.TagName,
				Name:        r.Name,
				Prerelease:  r.Prerelease,
				PublishedAt: r.PublishedAt,
			})
		}

		if len(releases) < perPage {
			break
		}
	}

	return result, nil
}

// DownloadCoreFromURL downloads and installs core using a custom link.
func (s *CoreManagerService) DownloadCoreFromURL(downloadURL string, requestedSessionID string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	sessionID := StartCoreDownloadProgressSession("sing-box", requestedSessionID, false)
	defer func() {
		if r := recover(); r != nil {
			FinishCoreDownloadProgressError(sessionID, coreDownloadStageCompleted, fmt.Sprintf("%v", r))
			panic(r)
		}
	}()

	if err := EnsureManagedCoreLayout(); err != nil {
		FinishCoreDownloadProgressError(sessionID, coreDownloadStageDownloading, err.Error())
		return "", err
	}
	if err := cleanupStaleManagedCoreInstallWorkspaces(s.getCoreDir(), singboxCoreInstallStagePrefix, singboxCoreInstallBackupPrefix); err != nil {
		FinishCoreDownloadProgressError(sessionID, coreDownloadStageDownloading, err.Error())
		return "", err
	}
	if err := cleanupManagedCoreInstallWorkspaceArtifacts(s.getCoreDir(), s.getCoreBinName()); err != nil {
		FinishCoreDownloadProgressError(sessionID, coreDownloadStageDownloading, err.Error())
		return "", err
	}

	wasRunning := s.isRunning()
	if wasRunning {
		sharedCoreDownloadProgressStore.mu.Lock()
		if session := sharedCoreDownloadProgressStore.sessions[sessionID]; session != nil {
			session.runningBefore = true
			session.updatedAt = time.Now().Unix()
		}
		sharedCoreDownloadProgressStore.mu.Unlock()
	}
	failProgress := func(stage string, err error) {
		if err != nil {
			FinishCoreDownloadProgressError(sessionID, stage, err.Error())
		}
	}

	downloadURL = strings.TrimSpace(downloadURL)
	if downloadURL == "" {
		err := fmt.Errorf("download url is empty")
		failProgress(coreDownloadStageDownloading, err)
		return "", err
	}

	logger.Info("start downloading sing-box from custom url: ", downloadURL)
	SetCoreDownloadProgressStage(sessionID, coreDownloadStageDownloading)

	client := &http.Client{Timeout: 600 * time.Second}
	req, err := http.NewRequest("GET", downloadURL, nil)
	if err != nil {
		err = fmt.Errorf("create request failed: %v", err)
		failProgress(coreDownloadStageDownloading, err)
		return "", err
	}
	req.Header.Set("User-Agent", "kwor")

	resp, err := client.Do(req)
	if err != nil {
		err = fmt.Errorf("download failed: %v", err)
		failProgress(coreDownloadStageDownloading, err)
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		err = fmt.Errorf("download failed, HTTP %d", resp.StatusCode)
		failProgress(coreDownloadStageDownloading, err)
		return "", err
	}
	SetCoreDownloadProgressTotals(sessionID, resp.ContentLength, resp.ContentLength <= 0)

	coreDir := s.getCoreDir()
	if err = os.MkdirAll(coreDir, 0o755); err != nil {
		err = fmt.Errorf("create core directory failed: %v", err)
		failProgress(coreDownloadStageDownloading, err)
		return "", err
	}

	ext := detectCoreArchiveExtFromURL(downloadURL)
	tmpFile := filepath.Join(coreDir, "sing-box-custom-download"+ext)
	defer os.Remove(tmpFile)

	out, err := os.Create(tmpFile)
	if err != nil {
		err = fmt.Errorf("create temp file failed: %v", err)
		failProgress(coreDownloadStageDownloading, err)
		return "", err
	}

	_, err = io.Copy(out, io.TeeReader(resp.Body, &coreDownloadProgressWriter{sessionID: sessionID}))
	if err != nil {
		_ = out.Close()
		err = fmt.Errorf("write temp file failed: %v", err)
		failProgress(coreDownloadStageDownloading, err)
		return "", err
	}
	if err = out.Close(); err != nil {
		err = fmt.Errorf("close temp file failed: %v", err)
		failProgress(coreDownloadStageDownloading, err)
		return "", err
	}

	SetCoreDownloadProgressStage(sessionID, coreDownloadStageReplacing)
	stageDir, cleanupStageDir, err := createManagedCoreInstallWorkspace(coreDir, singboxCoreInstallStagePrefix)
	if err != nil {
		failProgress(coreDownloadStageReplacing, err)
		return "", err
	}
	defer cleanupStageDir()

	if err = s.installCoreFromArchiveFile(tmpFile, stageDir); err != nil {
		err = fmt.Errorf("extract/install failed: %v", err)
		failProgress(coreDownloadStageReplacing, err)
		return "", err
	}

	binName := s.getCoreBinName()
	stagedBinPath := filepath.Join(stageDir, binName)
	SetCoreDownloadProgressStage(sessionID, coreDownloadStageValidating)
	if !s.validateCoreBinary(stagedBinPath) {
		err = fmt.Errorf("downloaded sing-box binary is not executable on current runtime %s/%s", runtime.GOOS, runtime.GOARCH)
		failProgress(coreDownloadStageValidating, err)
		return "", err
	}

	activation, activationStage, err := activateManagedCoreBinaryInstallWithRuntime(
		wasRunning,
		func() error {
			SetCoreDownloadProgressStage(sessionID, coreDownloadStageStopping)
			return s.stopCoreInternal()
		},
		func() {
			SetCoreDownloadProgressStage(sessionID, coreDownloadStageReplacing)
		},
		s.startCoreLocked,
		func() (*managedCoreBinaryActivation, error) {
			return activateManagedCoreBinaryInstall(coreDir, binName, stageDir, singboxCoreInstallBackupPrefix)
		},
	)
	if err != nil {
		if strings.TrimSpace(activationStage) == "" {
			activationStage = coreDownloadStageReplacing
		}
		failProgress(activationStage, err)
		return "", err
	}
	finalized := false
	defer func() {
		if !finalized {
			if rollbackErr := activation.Rollback(); rollbackErr != nil {
				logger.Warning("rollback sing-box custom staged install failed: ", rollbackErr)
			}
		}
	}()

	binPath := filepath.Join(coreDir, binName)
	localVersion, _ := s.getLocalVersion(binPath)
	logger.Info("custom core download complete, version: ", localVersion)

	if wasRunning {
		SetCoreDownloadProgressStage(sessionID, coreDownloadStageStarting)
		if err = s.startCoreLocked(); err != nil {
			rollbackErr := activation.Rollback()
			finalized = true
			if rollbackErr == nil {
				if restartErr := s.startCoreLocked(); restartErr != nil {
					err = fmt.Errorf("download completed, but new core auto start failed: %v; rolled back old core, but old core restart failed: %v", err, restartErr)
				} else {
					err = fmt.Errorf("download completed, but new core auto start failed and was rolled back to previous version: %v", err)
				}
			} else {
				err = fmt.Errorf("download completed, but auto start failed: %v; rollback failed: %v", err, rollbackErr)
			}
			failProgress(coreDownloadStageStarting, err)
			return localVersion, err
		}
		SetCoreDownloadProgressStage(sessionID, coreDownloadStageStarted)
		time.Sleep(900 * time.Millisecond)
	}
	if err := activation.Commit(); err != nil {
		logger.Warning("cleanup sing-box custom install backup workspace failed: ", err)
	}
	finalized = true

	FinishCoreDownloadProgressSuccess(sessionID, coreDownloadStageCompleted)
	return localVersion, nil
}

func detectCoreArchiveExtFromURL(downloadURL string) string {
	lower := strings.ToLower(downloadURL)
	switch {
	case strings.Contains(lower, ".tar.gz"):
		return ".tar.gz"
	case strings.Contains(lower, ".tgz"):
		return ".tgz"
	case strings.Contains(lower, ".tar.xz"):
		return ".tar.xz"
	case strings.Contains(lower, ".txz"):
		return ".txz"
	case strings.Contains(lower, ".tar.bz2"):
		return ".tar.bz2"
	case strings.Contains(lower, ".tbz2"):
		return ".tbz2"
	case strings.Contains(lower, ".zip"):
		return ".zip"
	case strings.Contains(lower, ".tar"):
		return ".tar"
	case strings.Contains(lower, ".gz"):
		return ".gz"
	default:
		return ".bin"
	}
}

func (s *CoreManagerService) installCoreFromArchiveFile(archivePath, coreDir string) error {
	binName := s.getCoreBinName()
	binPath := filepath.Join(coreDir, binName)
	_ = os.Remove(binPath)

	lower := strings.ToLower(archivePath)
	var err error

	switch {
	case strings.HasSuffix(lower, ".zip"):
		err = s.extractZip(archivePath, coreDir, binName)
	case strings.HasSuffix(lower, ".tar.gz"), strings.HasSuffix(lower, ".tgz"):
		err = s.extractTarGz(archivePath, coreDir, binName)
	case strings.HasSuffix(lower, ".tar"):
		err = extractCoreTar(archivePath, coreDir, binName)
	case strings.HasSuffix(lower, ".gz"):
		err = extractCoreGzipBinary(archivePath, coreDir, binName)
	case strings.HasSuffix(lower, ".tar.xz"), strings.HasSuffix(lower, ".txz"),
		strings.HasSuffix(lower, ".tar.bz2"), strings.HasSuffix(lower, ".tbz2"):
		err = s.extractCoreByExternalTool(archivePath, coreDir, binName)
	default:
		if copyErr := copyCoreFile(archivePath, binPath); copyErr == nil {
			if runtime.GOOS != "windows" {
				_ = os.Chmod(binPath, 0o755)
			}
			if s.validateCoreBinary(binPath) {
				return nil
			}
			_ = os.Remove(binPath)
		}
		err = s.extractCoreByExternalTool(archivePath, coreDir, binName)
	}

	if err != nil {
		fallbackErr := s.extractCoreByExternalTool(archivePath, coreDir, binName)
		if fallbackErr != nil {
			return fmt.Errorf("%v; fallback failed: %v", err, fallbackErr)
		}
	}

	if runtime.GOOS != "windows" {
		_ = os.Chmod(binPath, 0o755)
	}
	if _, statErr := os.Stat(binPath); statErr != nil {
		return fmt.Errorf("core binary not found after extraction")
	}
	return nil
}

func (s *CoreManagerService) validateCoreBinary(binPath string) bool {
	cmd := exec.Command(binPath, "version")
	cmd.Dir = filepath.Dir(binPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(string(output)), "sing-box")
}

func CheckSingboxRuntimeConfig(binPath, configPath, workDir string) error {
	binPath = strings.TrimSpace(binPath)
	configPath = strings.TrimSpace(configPath)
	workDir = strings.TrimSpace(workDir)
	if binPath == "" || configPath == "" {
		return nil
	}
	if workDir == "" {
		workDir = filepath.Dir(binPath)
	}

	ctx, cancel := context.WithTimeout(context.Background(), singboxConfigCheckTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, binPath, "check", "-c", configPath)
	cmd.Dir = workDir
	output, err := cmd.CombinedOutput()
	outputText := trimCommandOutputForError(output, 6000)
	if ctx.Err() == context.DeadlineExceeded {
		if outputText != "" {
			return fmt.Errorf("sing-box config check timed out after %s: %s", singboxConfigCheckTimeout, outputText)
		}
		return fmt.Errorf("sing-box config check timed out after %s", singboxConfigCheckTimeout)
	}
	if err == nil {
		return nil
	}
	if isSingboxCheckCommandUnsupported(outputText) {
		logger.Warning("当前 sing-box 不支持 check 命令，跳过启动前配置校验: ", outputText)
		return nil
	}
	if outputText != "" {
		return fmt.Errorf("sing-box config check failed: %v\n%s", err, outputText)
	}
	return fmt.Errorf("sing-box config check failed: %v", err)
}

func trimCommandOutputForError(output []byte, maxLen int) string {
	text := strings.TrimSpace(string(output))
	if maxLen <= 0 || len(text) <= maxLen {
		return text
	}
	return "...(truncated)\n" + text[len(text)-maxLen:]
}

func isSingboxCheckCommandUnsupported(output string) bool {
	lower := strings.ToLower(strings.TrimSpace(output))
	if lower == "" {
		return false
	}
	if strings.Contains(lower, "unknown command") && strings.Contains(lower, "check") {
		return true
	}
	if strings.Contains(lower, "unknown subcommand") && strings.Contains(lower, "check") {
		return true
	}
	if strings.Contains(lower, "no help topic") && strings.Contains(lower, "check") {
		return true
	}
	return false
}

func extractCoreTar(tarPath, destDir, binName string) error {
	f, err := os.Open(tarPath)
	if err != nil {
		return err
	}
	defer f.Close()

	tr := tar.NewReader(f)
	for {
		header, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		if filepath.Base(header.Name) == binName && header.Typeflag == tar.TypeReg {
			destPath := filepath.Join(destDir, binName)
			outFile, err := os.Create(destPath)
			if err != nil {
				return err
			}
			defer outFile.Close()
			_, err = io.Copy(outFile, tr)
			return err
		}
	}
	return fmt.Errorf("core binary %s not found in tar archive", binName)
}

func extractCoreGzipBinary(gzPath, destDir, binName string) error {
	f, err := os.Open(gzPath)
	if err != nil {
		return err
	}
	defer f.Close()

	gzr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gzr.Close()

	destPath := filepath.Join(destDir, binName)
	outFile, err := os.Create(destPath)
	if err != nil {
		return err
	}
	defer outFile.Close()

	_, err = io.Copy(outFile, gzr)
	return err
}

var errCoreBinaryFound = errors.New("core binary found")

func (s *CoreManagerService) extractCoreByExternalTool(archivePath, destDir, binName string) error {
	tmpDir := filepath.Join(destDir, fmt.Sprintf("extract_tmp_%d", time.Now().UnixNano()))
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	runAndCopy := func(name string, args ...string) error {
		cmd := exec.Command(name, args...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			msg := strings.TrimSpace(string(output))
			if msg == "" {
				msg = err.Error()
			}
			return fmt.Errorf("%s failed: %s", name, msg)
		}
		if err = s.copyCoreBinaryFromExtractedDir(tmpDir, filepath.Join(destDir, binName), binName); err != nil {
			return fmt.Errorf("%s extracted but core binary not found: %v", name, err)
		}
		return nil
	}

	if _, err := exec.LookPath("7z"); err == nil {
		if err = runAndCopy("7z", "x", "-y", "-o"+tmpDir, archivePath); err == nil {
			return nil
		}
	}
	if _, err := exec.LookPath("tar"); err == nil {
		if err = runAndCopy("tar", "-xf", archivePath, "-C", tmpDir); err == nil {
			return nil
		}
	}
	if _, err := exec.LookPath("unzip"); err == nil {
		if err = runAndCopy("unzip", "-o", archivePath, "-d", tmpDir); err == nil {
			return nil
		}
	}

	return fmt.Errorf("unsupported archive format or required extract tool missing")
}

func (s *CoreManagerService) copyCoreBinaryFromExtractedDir(srcDir, destPath, binName string) error {
	var sourcePath string
	walkErr := filepath.WalkDir(srcDir, func(path string, d os.DirEntry, err error) error {
		if err != nil || d == nil {
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if strings.EqualFold(filepath.Base(path), binName) {
			sourcePath = path
			return errCoreBinaryFound
		}
		return nil
	})
	if walkErr != nil && !errors.Is(walkErr, errCoreBinaryFound) {
		return walkErr
	}
	if sourcePath == "" {
		return fmt.Errorf("core binary %s not found", binName)
	}
	return copyCoreFile(sourcePath, destPath)
}

func copyCoreFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	_, err = io.Copy(out, in)
	return err
}
