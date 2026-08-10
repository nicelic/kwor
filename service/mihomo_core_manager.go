package service

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alireza0/s-ui/logger"
	"golang.org/x/sys/cpu"
)

type MihomoCoreDownloadTarget struct {
	OS         string `json:"os"`
	Arch       string `json:"arch"`
	Amd64Level string `json:"amd64Level"`
}

type MihomoCoreInfo struct {
	LocalVersion       string                       `json:"localVersion"`
	Installed          bool                         `json:"installed"`
	Compatible         bool                         `json:"compatible"`
	Running            bool                         `json:"running"`
	VersionInfo        string                       `json:"versionInfo"`
	Platform           string                       `json:"platform"`
	RuntimeMode        string                       `json:"runtimeMode,omitempty"`
	InstalledTarget    MihomoCoreDownloadTarget     `json:"installedTarget,omitempty"`
	InstalledChannel   string                       `json:"installedChannel,omitempty"`
	DownloadPreference MihomoCoreDownloadPreference `json:"downloadPreference"`
}

type MihomoCoreUpdateInfo struct {
	Enabled                 bool   `json:"enabled"`
	IntervalHours           int    `json:"intervalHours"`
	LastCheckedAt           int64  `json:"lastCheckedAt"`
	LatestStable            string `json:"latestStable"`
	LatestAlpha             string `json:"latestAlpha"`
	PendingStable           string `json:"pendingStable"`
	PendingAlpha            string `json:"pendingAlpha"`
	HasUpdate               bool   `json:"hasUpdate"`
	UpdateCount             int    `json:"updateCount"`
	AutoUpdateEnabled       bool   `json:"autoUpdateEnabled"`
	AutoUpdateDisabled      bool   `json:"autoUpdateDisabled"`
	AutoUpdateDisableReason string `json:"autoUpdateDisableReason"`
	AutoUpdateLastAttemptAt int64  `json:"autoUpdateLastAttemptAt"`
	AutoUpdateLastSuccessAt int64  `json:"autoUpdateLastSuccessAt"`
	AutoUpdateError         string `json:"autoUpdateError"`
	AutoUpdateErrorAt       int64  `json:"autoUpdateErrorAt"`
}

type MihomoCoreVersionItem struct {
	TagName     string `json:"tag_name"`
	Version     string `json:"version,omitempty"`
	Name        string `json:"name"`
	Prerelease  bool   `json:"prerelease"`
	PublishedAt string `json:"published_at"`
	AssetName   string `json:"asset_name,omitempty"`
	AssetSize   int64  `json:"asset_size,omitempty"`
}

type MihomoCoreVersionListResponse struct {
	Versions []MihomoCoreVersionItem `json:"versions"`
	Page     int                     `json:"page,omitempty"`
	PerPage  int                     `json:"per_page,omitempty"`
	Offset   int                     `json:"offset,omitempty"`
	Limit    int                     `json:"limit,omitempty"`
	HasMore  bool                    `json:"has_more"`
}

type mihomoCoreVersionCacheEntry struct {
	expiresAt time.Time
	response  MihomoCoreVersionListResponse
}

var mihomoCoreVersionCache = struct {
	sync.Mutex
	items map[string]mihomoCoreVersionCacheEntry
}{
	items: make(map[string]mihomoCoreVersionCacheEntry),
}

func cleanupMihomoCoreVersionCacheLocked(now time.Time) {
	for key, entry := range mihomoCoreVersionCache.items {
		if now.After(entry.expiresAt) {
			delete(mihomoCoreVersionCache.items, key)
		}
	}
}

func cloneMihomoCoreVersionListResponse(response *MihomoCoreVersionListResponse) *MihomoCoreVersionListResponse {
	if response == nil {
		return nil
	}
	cloned := *response
	if response.Versions != nil {
		cloned.Versions = append([]MihomoCoreVersionItem(nil), response.Versions...)
	}
	return &cloned
}

func getMihomoCoreVersionCache(key string) (*MihomoCoreVersionListResponse, bool) {
	now := time.Now()
	mihomoCoreVersionCache.Lock()
	defer mihomoCoreVersionCache.Unlock()
	cleanupMihomoCoreVersionCacheLocked(now)

	entry, ok := mihomoCoreVersionCache.items[key]
	if !ok {
		return nil, false
	}
	if now.After(entry.expiresAt) {
		delete(mihomoCoreVersionCache.items, key)
		return nil, false
	}
	return cloneMihomoCoreVersionListResponse(&entry.response), true
}

func setMihomoCoreVersionCache(key string, response *MihomoCoreVersionListResponse) {
	if response == nil {
		return
	}
	now := time.Now()
	mihomoCoreVersionCache.Lock()
	defer mihomoCoreVersionCache.Unlock()
	cleanupMihomoCoreVersionCacheLocked(now)
	mihomoCoreVersionCache.items[key] = mihomoCoreVersionCacheEntry{
		expiresAt: now.Add(coreVersionCacheTTL),
		response:  *cloneMihomoCoreVersionListResponse(response),
	}
}

type MihomoCoreManagerService struct {
	mu        sync.Mutex
	coreCmd   *exec.Cmd
	isStarted bool
	stdout    *os.File
	stderr    *os.File
}

const (
	mihomoSystemdName = "kwor-mihomo"

	mihomoCoreAutoCheckEnabledKey        = "mihomoCoreAutoCheckEnabled"
	mihomoCoreAutoCheckIntervalHoursKey  = "mihomoCoreAutoCheckIntervalHours"
	mihomoCoreAutoCheckLastAtKey         = "mihomoCoreAutoCheckLastAt"
	mihomoCoreAutoCheckLatestStableKey   = "mihomoCoreAutoCheckLatestStable"
	mihomoCoreAutoCheckLatestAlphaKey    = "mihomoCoreAutoCheckLatestAlpha"
	mihomoCoreAutoCheckPendingStableKey  = "mihomoCoreAutoCheckPendingStable"
	mihomoCoreAutoCheckPendingAlphaKey   = "mihomoCoreAutoCheckPendingAlpha"
	mihomoCoreAutoUpdateEnabledKey       = "mihomoCoreAutoUpdateEnabled"
	mihomoCoreAutoUpdateLastAttemptKey   = "mihomoCoreAutoUpdateLastAttemptAt"
	mihomoCoreAutoUpdateLastSuccessKey   = "mihomoCoreAutoUpdateLastSuccessAt"
	mihomoCoreAutoUpdateErrorKey         = "mihomoCoreAutoUpdateError"
	mihomoCoreAutoUpdateErrorAtKey       = "mihomoCoreAutoUpdateErrorAt"
	mihomoCoreAutoUpdateDisableReasonKey = "mihomoCoreAutoUpdateDisableReason"
)

var (
	legacyMihomoSystemdNames = []string{
		"mihomo",
		"metacubex-mihomo",
		"s-ui-mihomo",
		"sui-mihomo",
	}
	mihomoCoreAutoCheckMu       sync.Mutex
	mihomoVersionOutputRe       = regexp.MustCompile(`(?im)^\s*Mihomo\s+Meta\s+([^\s]+)`)
	mihomoVersionFallbackRe     = regexp.MustCompile(`(?i)\bmihomo(?:\s+(?:meta|version))?[\s:=]+((?:v?\d+\.\d+\.\d+(?:[-+._A-Za-z0-9]+)?)|(?:(?:alpha|beta|rc)-[0-9A-Za-z][0-9A-Za-z._+-]*))\b`)
	mihomoRollingVersionRe      = regexp.MustCompile(`(?i)^(?:(?:alpha|beta|rc)-[0-9A-Za-z][0-9A-Za-z._+-]*|(?:alpha|beta|rc)@[0-9T:Z.+-]+|prerelease-(?:alpha|beta|rc)(?:[-._][0-9A-Za-z._+-]+)?)$`)
	mihomoRollingAssetVersionRe = regexp.MustCompile(`(?i)(?:^|-)((?:alpha|beta|rc)-[0-9A-Za-z]+)(?:-|\.|$)`)
)

const mihomoReleaseVersionMaxBytes int64 = 64 * 1024

type mihomoLocalVersionCacheEntry struct {
	expiresAt   time.Time
	binModTime  time.Time
	binSize     int64
	binMode     os.FileMode
	version     string
	versionInfo string
}

var mihomoLocalVersionCache = struct {
	sync.Mutex
	items map[string]mihomoLocalVersionCacheEntry
}{
	items: make(map[string]mihomoLocalVersionCacheEntry),
}

func normalizeMihomoAMD64Level(level string) string {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "v1":
		return "v1"
	case "v2":
		return "v2"
	case "v3":
		return "v3"
	default:
		return ""
	}
}

func inferMihomoHostAMD64Level() string {
	if GetSystemPlatformArchitecture() != "amd64" {
		return ""
	}
	if !(cpu.X86.HasCX16 &&
		cpu.X86.HasSSE3 &&
		cpu.X86.HasSSSE3 &&
		cpu.X86.HasSSE41 &&
		cpu.X86.HasSSE42 &&
		cpu.X86.HasPOPCNT) {
		return "v1"
	}
	if cpu.X86.HasAVX &&
		cpu.X86.HasAVX2 &&
		cpu.X86.HasBMI1 &&
		cpu.X86.HasBMI2 &&
		cpu.X86.HasFMA {
		return "v3"
	}
	return "v2"
}

func hasMihomoCoreDownloadTargetFilter(target MihomoCoreDownloadTarget) bool {
	return strings.TrimSpace(target.OS) != "" ||
		strings.TrimSpace(target.Arch) != "" ||
		strings.TrimSpace(target.Amd64Level) != ""
}

func mihomoCoreVersionCacheKey(repo string, channel string, offset int, limit int, target MihomoCoreDownloadTarget, filterTarget bool) string {
	if !filterTarget {
		return fmt.Sprintf("%s|%s|%d|%d|all", repo, channel, offset, limit)
	}
	return fmt.Sprintf(
		"%s|%s|%d|%d|%s|%s|%s",
		repo,
		channel,
		offset,
		limit,
		target.OS,
		target.Arch,
		target.Amd64Level,
	)
}

func describeMihomoCoreDownloadTarget(target MihomoCoreDownloadTarget) string {
	if target.Arch == "amd64" && target.Amd64Level != "" {
		return fmt.Sprintf("%s/%s (%s)", target.OS, target.Arch, target.Amd64Level)
	}
	return fmt.Sprintf("%s/%s", target.OS, target.Arch)
}

func getMihomoLocalVersionCache(binPath string, binModTime time.Time, binSize int64, binMode os.FileMode) (string, string, bool) {
	now := time.Now()
	mihomoLocalVersionCache.Lock()
	defer mihomoLocalVersionCache.Unlock()

	entry, ok := mihomoLocalVersionCache.items[binPath]
	if !ok {
		return "", "", false
	}
	if now.After(entry.expiresAt) {
		delete(mihomoLocalVersionCache.items, binPath)
		return "", "", false
	}
	if !entry.binModTime.Equal(binModTime) || entry.binSize != binSize || entry.binMode != binMode {
		delete(mihomoLocalVersionCache.items, binPath)
		return "", "", false
	}
	return entry.version, entry.versionInfo, true
}

func setMihomoLocalVersionCache(binPath string, binModTime time.Time, binSize int64, binMode os.FileMode, version string, versionInfo string) {
	mihomoLocalVersionCache.Lock()
	defer mihomoLocalVersionCache.Unlock()

	mihomoLocalVersionCache.items[binPath] = mihomoLocalVersionCacheEntry{
		expiresAt:   time.Now().Add(coreLocalVersionCacheTTL),
		binModTime:  binModTime,
		binSize:     binSize,
		binMode:     binMode,
		version:     version,
		versionInfo: versionInfo,
	}
}

func clearMihomoLocalVersionCache(binPath string) {
	mihomoLocalVersionCache.Lock()
	defer mihomoLocalVersionCache.Unlock()
	delete(mihomoLocalVersionCache.items, binPath)
}

func GetMihomoSystemdName() string {
	return mihomoSystemdName
}

func (s *MihomoCoreManagerService) getCoreDir() string {
	return GetMihomoCoreDir()
}

func (s *MihomoCoreManagerService) getCoreBinName() string {
	if IsSystemPlatformWindows() {
		return "mihomo.exe"
	}
	return "mihomo"
}

func (s *MihomoCoreManagerService) getCoreBinPath() string {
	return filepath.Join(s.getCoreDir(), s.getCoreBinName())
}

func (s *MihomoCoreManagerService) getConfigPath() string {
	return GetMihomoConfigPath()
}

func (s *MihomoCoreManagerService) regenerateRuntimeConfig() error {
	return NewMihomoManagerService().RegenerateServerConfig()
}

func (s *MihomoCoreManagerService) getPlatformInfo() string {
	return fmt.Sprintf("%s/%s", GetSystemPlatformOS(), GetSystemPlatformArchitecture())
}

func getMihomoServiceFilePath() string {
	return getSystemdServiceFilePathByName(mihomoSystemdName)
}

func isMihomoRollingVersion(version string) bool {
	return mihomoRollingVersionRe.MatchString(strings.TrimSpace(version))
}

func isMihomoResolvedRollingVersion(version string) bool {
	normalized := strings.ToLower(strings.TrimSpace(version))
	if !isMihomoRollingVersion(normalized) {
		return false
	}
	return !strings.Contains(normalized, "@") && !strings.HasPrefix(normalized, "prerelease-")
}

func mihomoRollingReleaseKind(release GitHubRelease) string {
	text := strings.ToLower(strings.TrimSpace(release.TagName + " " + release.Name))
	switch {
	case strings.Contains(text, "beta"):
		return "beta"
	case strings.Contains(text, "rc"):
		return "rc"
	default:
		return "alpha"
	}
}

func normalizeMihomoDetectedVersion(value string) string {
	value = strings.TrimSpace(strings.Trim(value, "\"'()[]{}"))
	if value == "" {
		return ""
	}
	if fields := strings.Fields(value); len(fields) > 0 {
		value = strings.TrimSpace(strings.Trim(fields[0], "\"'()[]{}"))
	}
	if isLikelySemverTag(value) || isMihomoRollingVersion(value) {
		return value
	}
	return ""
}

func extractMihomoVersion(output string) string {
	output = strings.TrimSpace(output)
	if output == "" {
		return ""
	}

	if match := mihomoVersionOutputRe.FindStringSubmatch(output); len(match) > 1 {
		return normalizeMihomoDetectedVersion(match[1])
	}

	if !strings.Contains(strings.ToLower(output), "mihomo") {
		return ""
	}
	if match := mihomoVersionFallbackRe.FindStringSubmatch(output); len(match) > 1 {
		return normalizeMihomoDetectedVersion(match[1])
	}
	return ""
}

func extractMihomoRollingVersionFromAssetName(name string) string {
	if match := mihomoRollingAssetVersionRe.FindStringSubmatch(strings.TrimSpace(name)); len(match) > 1 {
		return strings.TrimSpace(match[1])
	}
	return ""
}

func mihomoReleaseVersionIdentity(release GitHubRelease) string {
	if !release.Prerelease {
		return strings.TrimSpace(release.TagName)
	}

	for _, asset := range release.Assets {
		if version := extractMihomoRollingVersionFromAssetName(asset.Name); version != "" {
			return version
		}
	}

	if updatedAt := strings.TrimSpace(release.UpdatedAt); updatedAt != "" {
		return mihomoRollingReleaseKind(release) + "@" + updatedAt
	}
	return strings.TrimSpace(release.TagName)
}

func (s *MihomoCoreManagerService) fetchMihomoReleaseVersion(client *http.Client, release GitHubRelease) (string, error) {
	if client == nil {
		return "", fmt.Errorf("mihomo release client is nil")
	}
	var versionURL string
	for _, asset := range release.Assets {
		if strings.EqualFold(strings.TrimSpace(asset.Name), "version.txt") {
			versionURL = strings.TrimSpace(asset.BrowserDownloadURL)
			break
		}
	}
	if versionURL == "" {
		return "", fmt.Errorf("mihomo release version asset is missing")
	}

	req, err := http.NewRequest(http.MethodGet, versionURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "kwor")
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request mihomo release version failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("mihomo release version API returned %d", resp.StatusCode)
	}
	body, err := readBoundedHTTPResponseBody(resp.Body, mihomoReleaseVersionMaxBytes)
	if err != nil {
		return "", err
	}
	version := normalizeMihomoDetectedVersion(string(body))
	if version == "" {
		return "", fmt.Errorf("mihomo release version is invalid")
	}
	return version, nil
}

func (s *MihomoCoreManagerService) getLocalVersion(binPath string) (string, string) {
	commands := [][]string{
		{"-v"},
		{"version"},
	}
	var fallbackOutput string
	for _, args := range commands {
		output, err := runCommandOutputInDirWithTimeout(coreVersionCommandTimeout, filepath.Dir(binPath), binPath, args...)
		if err != nil {
			continue
		}
		outputStr := strings.TrimSpace(string(output))
		if outputStr == "" {
			continue
		}
		if match := extractMihomoVersion(outputStr); match != "" {
			return match, outputStr
		}
		if fallbackOutput == "" {
			fallbackOutput = outputStr
		}
	}
	return "", fallbackOutput
}

func (s *MihomoCoreManagerService) getCachedLocalVersion(binPath string, statInfo os.FileInfo, forceRefresh bool) (string, string) {
	if forceRefresh {
		clearMihomoLocalVersionCache(binPath)
	}
	binMode := statInfo.Mode().Perm()
	if version, versionInfo, ok := getMihomoLocalVersionCache(binPath, statInfo.ModTime(), statInfo.Size(), binMode); ok {
		return version, versionInfo
	}
	version, versionInfo := s.getLocalVersion(binPath)
	setMihomoLocalVersionCache(binPath, statInfo.ModTime(), statInfo.Size(), binMode, version, versionInfo)
	return version, versionInfo
}

func inferMihomoInstalledAMD64Level(binPath string, versionInfo string) string {
	return inferMihomoAMD64Level(inferMihomoTargetFromGoBuildInfo(binPath).Amd64Level, versionInfo)
}

func inferMihomoAMD64Level(goBuildLevel string, versionInfo string) string {
	if level := normalizeMihomoAMD64Level(goBuildLevel); level != "" {
		return level
	}
	return inferMihomoAmd64LevelFromVersionInfo(versionInfo)
}

func (s *MihomoCoreManagerService) GetCoreStatus() (*MihomoCoreInfo, error) {
	if err := EnsureManagedCoreLayout(); err != nil {
		return nil, err
	}

	info := &MihomoCoreInfo{
		Platform: s.getPlatformInfo(),
	}
	info.RuntimeMode = string(getManagedCoreRuntimeMode())
	if preference, err := s.GetDownloadPreference(); err == nil {
		info.DownloadPreference = s.normalizeStatusDownloadPreference(preference)
	} else {
		logger.Warning("failed to load mihomo download preference: ", err)
	}

	binPath := s.getCoreBinPath()
	if !IsSystemPlatformWindows() {
		inspection := s.inspectManagedLocalMihomoBinary(binPath)
		info.Installed = inspection.Installed
		info.Compatible = inspection.Compatible
		info.LocalVersion = inspection.Version
		info.VersionInfo = inspection.VersionInfo
		if inspection.Architecture != "" {
			installedTarget := MihomoCoreDownloadTarget{
				OS:         "linux",
				Arch:       inspection.Architecture,
				Amd64Level: inspection.Amd64Level,
			}
			info.InstalledTarget = normalizeMihomoInstalledTarget(installedTarget)
		}
		if channel := strings.TrimSpace(inspection.Channel); channel != "" {
			info.InstalledChannel = channel
		}
		if !inspection.Installed {
			clearMihomoLocalVersionCache(binPath)
			clearMihomoCoreLocalTargetCache(binPath)
		}
	} else if statInfo, err := os.Stat(binPath); err == nil {
		info.Installed = true
		info.LocalVersion, info.VersionInfo = s.getCachedLocalVersion(binPath, statInfo, false)
		info.Compatible = info.LocalVersion != "" && managedCoreVersionOutputMatches(info.VersionInfo, "mihomo")
		installedTarget := inferMihomoTargetFromGoBuildInfo(binPath)
		if installedTarget.OS == "" && installedTarget.Arch == "" {
			installedTarget = inferMihomoTargetFromPlatform(info.Platform)
		}
		if installedTarget.Arch == "amd64" {
			installedTarget.Amd64Level = inferMihomoInstalledAMD64Level(binPath, info.VersionInfo)
		}
		info.InstalledTarget = normalizeMihomoInstalledTarget(installedTarget)
		info.InstalledChannel = detectMihomoInstalledChannel(info.LocalVersion, info.VersionInfo)
		if info.InstalledChannel == "" {
			info.InstalledChannel = inferMihomoInstalledChannelFromBuildInfo(binPath)
		}
	} else {
		clearMihomoLocalVersionCache(binPath)
		clearMihomoCoreLocalTargetCache(binPath)
	}

	info.Running = s.isRunning()
	return info, nil
}

func (s *MihomoCoreManagerService) fetchGitHubReleasePage(client *http.Client, apiPage int, perPage int) ([]GitHubRelease, error) {
	return fetchGitHubReleasePageForRepo("MetaCubeX/mihomo", client, apiPage, perPage, coreGitHubReleaseListMaxBytes)
}

func (s *MihomoCoreManagerService) GetRemoteVersionsPage(channel string, page int, perPage int) (*MihomoCoreVersionListResponse, error) {
	if page < 1 {
		page = 1
	}
	if perPage <= 0 {
		perPage = 5
	}
	offset := (page - 1) * perPage
	return s.GetRemoteVersionsWindow(channel, offset, perPage, MihomoCoreDownloadTarget{})
}

func pickMihomoAssetFromAssets(assets []GitHubAsset, target MihomoCoreDownloadTarget) (GitHubAsset, bool) {
	normalizedTarget := (&MihomoCoreManagerService{}).normalizeMihomoVersionFilterTarget(target)
	preferredExts := []string{".gz", ".tar.gz", ".tgz", ".tar.xz", ".txz", ".zip"}
	if normalizedTarget.OS == "windows" {
		preferredExts = []string{".zip", ".tar.gz", ".tgz", ".gz", ".tar.xz", ".txz"}
	}
	requireExactAMD64Level := normalizedTarget.Arch == "amd64" && normalizedTarget.Amd64Level != ""

	type scoredAsset struct {
		asset GitHubAsset
		score int
	}

	candidates := make([]scoredAsset, 0, len(assets))
	for _, asset := range assets {
		if requireExactAMD64Level && !mihomoAssetMatchesAMD64LevelExactly(asset.Name, normalizedTarget.Amd64Level) {
			continue
		}
		score := (&MihomoCoreManagerService{}).scoreAssetName(asset.Name, preferredExts, normalizedTarget)
		if score < 1500 {
			continue
		}
		candidates = append(candidates, scoredAsset{asset: asset, score: score})
	}
	if len(candidates) == 0 {
		return GitHubAsset{}, false
	}

	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].score == candidates[j].score {
			return len(candidates[i].asset.Name) < len(candidates[j].asset.Name)
		}
		return candidates[i].score > candidates[j].score
	})
	return candidates[0].asset, true
}

func (s *MihomoCoreManagerService) GetRemoteVersionsWindow(channel string, offset int, limit int, target MihomoCoreDownloadTarget) (*MihomoCoreVersionListResponse, error) {
	channel = strings.ToLower(strings.TrimSpace(channel))
	switch channel {
	case "stable", "alpha":
	default:
		channel = "stable"
	}
	offset, limit = normalizeCoreVersionWindow(offset, limit)
	filterTarget := hasMihomoCoreDownloadTargetFilter(target)
	if filterTarget {
		target = s.normalizeMihomoVersionFilterTarget(target)
	}

	cacheKey := mihomoCoreVersionCacheKey("MetaCubeX/mihomo", channel, offset, limit, target, filterTarget)
	if cached, ok := getMihomoCoreVersionCache(cacheKey); ok {
		return cached, nil
	}

	client := &http.Client{Timeout: 30 * time.Second}
	seenTags := make(map[string]struct{})
	result := &MihomoCoreVersionListResponse{
		Versions: make([]MihomoCoreVersionItem, 0, limit+1),
		Offset:   offset,
		Limit:    limit,
		PerPage:  limit,
		Page:     offset/limit + 1,
	}

	matchedCount := 0
	const maxPages = 30
	for apiPage := 1; apiPage <= maxPages && len(result.Versions) < limit+1; apiPage++ {
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
				asset, ok := pickMihomoAssetFromAssets(r.Assets, target)
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

			result.Versions = append(result.Versions, MihomoCoreVersionItem{
				TagName:     r.TagName,
				Version:     mihomoReleaseVersionIdentity(r),
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
	setMihomoCoreVersionCache(cacheKey, result)
	return cloneMihomoCoreVersionListResponse(result), nil
}

func (s *MihomoCoreManagerService) getArchName() string {
	switch GetSystemPlatformArchitecture() {
	case "amd64":
		return "amd64"
	case "386":
		return "386"
	case "arm64":
		return "arm64"
	case "arm":
		return "armv7"
	default:
		return GetSystemPlatformArchitecture()
	}
}

func splitMihomoAssetTokens(name string) []string {
	lower := strings.ToLower(strings.TrimSpace(name))
	knownSuffixes := []string{
		".pkg.tar.zst",
		".tar.gz",
		".tar.xz",
		".tar.bz2",
		".tgz",
		".txz",
		".tbz2",
		".zip",
		".deb",
		".rpm",
		".apk",
		".gz",
		".tar",
	}
	for _, suffix := range knownSuffixes {
		if strings.HasSuffix(lower, suffix) {
			lower = strings.TrimSuffix(lower, suffix)
			break
		}
	}

	raw := strings.Split(lower, "-")
	tokens := make([]string, 0, len(raw))
	for _, token := range raw {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}
		tokens = append(tokens, token)
	}
	return tokens
}

func hasMihomoToken(tokens []string, expected string) bool {
	expected = strings.ToLower(strings.TrimSpace(expected))
	if expected == "" {
		return false
	}
	for _, token := range tokens {
		if token == expected {
			return true
		}
	}
	return false
}

func mihomoAssetMatchesAMD64LevelExactly(name string, level string) bool {
	level = normalizeMihomoAMD64Level(level)
	if level == "" {
		return true
	}
	tokens := splitMihomoAssetTokens(name)
	hasV1 := hasMihomoToken(tokens, "v1")
	hasV2 := hasMihomoToken(tokens, "v2")
	hasV3 := hasMihomoToken(tokens, "v3")
	hasCompatible := hasMihomoToken(tokens, "compatible")
	switch level {
	case "v1":
		return (hasV1 || hasCompatible) && !hasV2 && !hasV3
	case "v2":
		return hasV2 && !hasV1 && !hasV3 && !hasCompatible
	case "v3":
		return hasV3 && !hasV1 && !hasV2 && !hasCompatible
	default:
		return false
	}
}

func inferMihomoAmd64LevelFromVersionInfo(versionInfo string) string {
	lower := strings.ToLower(versionInfo)
	patterns := []struct {
		level string
		keys  []string
	}{
		{
			level: "v3",
			keys: []string{
				"goamd64=v3",
				"goamd64: v3",
				"goamd64 v3",
				"x86-64-v3",
				"amd64-v3",
			},
		},
		{
			level: "v2",
			keys: []string{
				"goamd64=v2",
				"goamd64: v2",
				"goamd64 v2",
				"x86-64-v2",
				"amd64-v2",
			},
		},
		{
			level: "v1",
			keys: []string{
				"goamd64=v1",
				"goamd64: v1",
				"goamd64 v1",
				"x86-64-v1",
				"amd64-v1",
				"compatible",
			},
		},
	}
	for _, pattern := range patterns {
		for _, key := range pattern.keys {
			if strings.Contains(lower, key) {
				return pattern.level
			}
		}
	}
	return ""
}

func (s *MihomoCoreManagerService) scoreAssetName(name string, preferredExts []string, target MihomoCoreDownloadTarget) int {
	lower := strings.ToLower(name)
	tokens := splitMihomoAssetTokens(lower)

	if !hasMihomoToken(tokens, target.OS) {
		return 0
	}
	if !hasMihomoToken(tokens, target.Arch) {
		return 0
	}

	score := 1900
	extMatched := false
	for index, ext := range preferredExts {
		if strings.HasSuffix(lower, ext) {
			score += 300 - index*40
			extMatched = true
			break
		}
	}
	if !extMatched {
		return 0
	}

	if strings.HasSuffix(lower, ".deb") ||
		strings.HasSuffix(lower, ".rpm") ||
		strings.HasSuffix(lower, ".apk") ||
		strings.HasSuffix(lower, ".pkg.tar.zst") {
		return 0
	}

	if target.Arch == "amd64" {
		level := normalizeMihomoAMD64Level(target.Amd64Level)
		hasV1 := hasMihomoToken(tokens, "v1")
		hasV2 := hasMihomoToken(tokens, "v2")
		hasV3 := hasMihomoToken(tokens, "v3")
		hasCompatible := hasMihomoToken(tokens, "compatible")
		isPlainAmd64 := !hasV1 && !hasV2 && !hasV3 && !hasCompatible

		switch level {
		case "v1":
			if hasV1 || hasCompatible {
				score += 500
			} else if isPlainAmd64 {
				score += 250
			}
			if hasV2 || hasV3 {
				score -= 600
			}
		case "v2":
			if hasV2 {
				score += 500
			} else if isPlainAmd64 {
				score += 250
			}
			if hasV1 || hasCompatible || hasV3 {
				score -= 600
			}
		case "v3":
			if hasV3 {
				score += 500
			} else if isPlainAmd64 {
				score += 350
			}
			if hasV1 || hasCompatible || hasV2 {
				score -= 500
			}
		}
	}

	if strings.Contains(lower, "alpha") {
		score -= 5
	}
	return score
}

func (s *MihomoCoreManagerService) normalizeDownloadTarget(target MihomoCoreDownloadTarget) MihomoCoreDownloadTarget {
	normalized := MihomoCoreDownloadTarget{
		OS:         strings.ToLower(strings.TrimSpace(target.OS)),
		Arch:       strings.ToLower(strings.TrimSpace(target.Arch)),
		Amd64Level: normalizeMihomoAMD64Level(target.Amd64Level),
	}
	if normalized.OS == "" {
		normalized.OS = GetSystemPlatformOS()
	}
	if normalized.Arch == "" {
		normalized.Arch = s.getArchName()
	}
	if normalized.Arch == "amd64" {
		if normalized.Amd64Level == "" {
			normalized.Amd64Level = inferMihomoHostAMD64Level()
		}
	} else {
		normalized.Amd64Level = ""
	}
	return normalized
}

// normalizeMihomoVersionFilterTarget keeps an omitted AMD64 level omitted.
// Version-list filtering may show all levels; an actual download still goes
// through normalizeDownloadTarget and receives a concrete runtime target.
func (s *MihomoCoreManagerService) normalizeMihomoVersionFilterTarget(target MihomoCoreDownloadTarget) MihomoCoreDownloadTarget {
	hadAMD64Level := strings.TrimSpace(target.Amd64Level) != ""
	normalized := s.normalizeDownloadTarget(target)
	if normalized.Arch == "amd64" && !hadAMD64Level {
		normalized.Amd64Level = ""
	}
	return normalized
}

func (s *MihomoCoreManagerService) pickDownloadAsset(release *GitHubRelease, target MihomoCoreDownloadTarget) (string, error) {
	if release == nil {
		return "", fmt.Errorf("release is nil")
	}

	if asset, ok := pickMihomoAssetFromAssets(release.Assets, target); ok {
		return asset.BrowserDownloadURL, nil
	}
	return "", fmt.Errorf("no suitable mihomo asset found for %s", describeMihomoCoreDownloadTarget(target))
}

func (s *MihomoCoreManagerService) getDownloadAsset(version string, target MihomoCoreDownloadTarget) (string, error) {
	return s.getDownloadAssetContext(context.Background(), version, target)
}

func (s *MihomoCoreManagerService) getDownloadAssetContext(ctx context.Context, version string, target MihomoCoreDownloadTarget) (string, error) {
	apiURL := fmt.Sprintf("https://api.github.com/repos/MetaCubeX/mihomo/releases/tags/%s", version)

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

	return s.pickDownloadAsset(&release, s.normalizeDownloadTarget(target))
}

func (s *MihomoCoreManagerService) installCoreFromArchiveFile(archivePath, coreDir string) error {
	return s.installCoreFromArchiveFileContext(context.Background(), archivePath, coreDir)
}

func (s *MihomoCoreManagerService) installCoreFromArchiveFileContext(ctx context.Context, archivePath, coreDir string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	binName := s.getCoreBinName()
	binPath := filepath.Join(coreDir, binName)
	_ = os.Remove(binPath)
	extractor := &CoreManagerService{}

	lower := strings.ToLower(archivePath)
	var err error

	switch {
	case strings.HasSuffix(lower, ".zip"):
		err = extractor.extractZipContext(ctx, archivePath, coreDir, binName)
	case strings.HasSuffix(lower, ".tar.gz"), strings.HasSuffix(lower, ".tgz"):
		err = extractor.extractTarGzContext(ctx, archivePath, coreDir, binName)
	case strings.HasSuffix(lower, ".tar"):
		err = extractCoreTarContext(ctx, archivePath, coreDir, binName)
	case strings.HasSuffix(lower, ".gz"):
		err = extractCoreGzipBinaryContext(ctx, archivePath, coreDir, binName)
	default:
		if copyErr := copyCoreFileContext(ctx, archivePath, binPath); copyErr == nil {
			if !IsSystemPlatformWindows() {
				_ = os.Chmod(binPath, 0o755)
			}
			if s.validateCoreBinary(binPath) {
				return nil
			}
			_ = os.Remove(binPath)
		}
		err = extractor.extractCoreByExternalToolContext(ctx, archivePath, coreDir, binName)
	}

	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return err
	}
	if !IsSystemPlatformWindows() {
		_ = os.Chmod(binPath, 0o755)
	}
	if _, statErr := os.Stat(binPath); statErr != nil {
		return fmt.Errorf("core binary not found after extraction")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

func (s *MihomoCoreManagerService) DownloadCore(version string, target MihomoCoreDownloadTarget, requestedSessionID string) (string, error) {
	operationContext, operation, err := BeginKworManagedOperation("mihomo-core-download")
	if err != nil {
		return "", err
	}
	defer operation.Done()
	return s.downloadCoreWithContext(operationContext, version, target, requestedSessionID)
}

func (s *MihomoCoreManagerService) downloadCoreWithContext(operationContext context.Context, version string, target MihomoCoreDownloadTarget, requestedSessionID string) (string, error) {
	lifecycleLock, err := AcquireKworLifecycleLockContext(operationContext)
	if err != nil {
		return "", err
	}
	defer func() { _ = lifecycleLock.Release() }()

	if err := lockManagedDownloadTaskMutex(operationContext, &s.mu); err != nil {
		return "", err
	}
	defer s.mu.Unlock()

	sessionID := StartCoreDownloadProgressSession("mihomo", requestedSessionID, false)
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
	if err := cleanupStaleManagedCoreInstallWorkspaces(s.getCoreDir(), mihomoCoreInstallStagePrefix, mihomoCoreInstallBackupPrefix); err != nil {
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
	if strings.TrimSpace(target.OS) != "" && normalizedTarget.OS != GetSystemPlatformOS() {
		err := fmt.Errorf("requested mihomo target %s cannot be installed on runtime %s/%s", describeMihomoCoreDownloadTarget(normalizedTarget), GetSystemPlatformOS(), GetSystemPlatformArchitecture())
		failProgress(coreDownloadStageDownloading, err)
		return "", err
	}
	if strings.TrimSpace(target.Arch) != "" && normalizedTarget.Arch != s.getArchName() {
		err := fmt.Errorf("requested mihomo target %s does not match runtime architecture %s", describeMihomoCoreDownloadTarget(normalizedTarget), s.getArchName())
		failProgress(coreDownloadStageDownloading, err)
		return "", err
	}

	if err := operationContext.Err(); err != nil {
		failProgress(coreDownloadStageDownloading, err)
		return "", err
	}
	downloadURL, err := s.getDownloadAssetContext(operationContext, version, normalizedTarget)
	if err != nil {
		failProgress(coreDownloadStageDownloading, err)
		return "", err
	}
	SetCoreDownloadProgressStage(sessionID, coreDownloadStageDownloading)

	client := &http.Client{Timeout: 600 * time.Second}
	req, err := http.NewRequestWithContext(operationContext, http.MethodGet, downloadURL, nil)
	if err != nil {
		err = fmt.Errorf("create download request failed: %v", err)
		failProgress(coreDownloadStageDownloading, err)
		return "", err
	}
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
	tmpFile := filepath.Join(coreDir, "mihomo-download"+ext)
	defer os.Remove(tmpFile)

	out, err := os.Create(tmpFile)
	if err != nil {
		err = fmt.Errorf("create temp file failed: %v", err)
		failProgress(coreDownloadStageDownloading, err)
		return "", err
	}

	if _, err = copyManagedDownloadTaskContext(operationContext, out, io.TeeReader(resp.Body, &coreDownloadProgressWriter{sessionID: sessionID})); err != nil {
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
	if err = operationContext.Err(); err != nil {
		failProgress(coreDownloadStageDownloading, err)
		return "", err
	}

	SetCoreDownloadProgressStage(sessionID, coreDownloadStageExtracting)
	stageDir, cleanupStageDir, err := createManagedCoreInstallWorkspace(coreDir, mihomoCoreInstallStagePrefix)
	if err != nil {
		failProgress(coreDownloadStageExtracting, err)
		return "", err
	}
	defer cleanupStageDir()

	if err = s.installCoreFromArchiveFileContext(operationContext, tmpFile, stageDir); err != nil {
		err = fmt.Errorf("extract/install failed: %v", err)
		failProgress(coreDownloadStageExtracting, err)
		return "", err
	}
	if err = operationContext.Err(); err != nil {
		failProgress(coreDownloadStageExtracting, err)
		return "", err
	}

	binName := s.getCoreBinName()
	stagedBinPath := filepath.Join(stageDir, binName)
	SetCoreDownloadProgressStage(sessionID, coreDownloadStageValidating)
	if !s.validateCoreBinary(stagedBinPath) {
		err = fmt.Errorf("downloaded mihomo binary is not executable on current runtime %s/%s", GetSystemPlatformOS(), GetSystemPlatformArchitecture())
		failProgress(coreDownloadStageValidating, err)
		return "", err
	}
	if !beginManagedCoreDownloadApplying(sessionID, coreDownloadStageReplacing) {
		err = coreDownloadTaskCancelledError(operationContext)
		failProgress(coreDownloadStageValidating, err)
		return "", err
	}
	SetCoreDownloadProgressStage(sessionID, coreDownloadStageReplacing)

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
			return activateManagedCoreBinaryInstall(coreDir, binName, stageDir, mihomoCoreInstallBackupPrefix)
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
				logger.Warning("rollback mihomo staged install failed: ", rollbackErr)
			}
		}
	}()

	binPath := filepath.Join(coreDir, binName)
	localVersion, _ := s.getLocalVersion(binPath)
	if err := s.SaveDownloadTarget(normalizedTarget); err != nil {
		logger.Warning("failed to save mihomo download preference: ", err)
	}
	if clearErr := s.ClearCoreAutoUpdateDisableReason(); clearErr != nil {
		logger.Warning("clear Mihomo auto update disable reason after download failed: ", clearErr)
	}

	if wasRunning {
		SetCoreDownloadProgressStage(sessionID, coreDownloadStageStarting)
		if err = s.startCoreLocked(); err != nil {
			rollbackErr := activation.Rollback()
			finalized = true
			if rollbackErr == nil {
				if restartErr := s.startCoreLocked(); restartErr != nil {
					err = fmt.Errorf("download completed, but new mihomo auto start failed: %v; rolled back old core, but old core restart failed: %v", err, restartErr)
				} else {
					err = fmt.Errorf("download completed, but new mihomo auto start failed and was rolled back to previous version: %v", err)
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
		logger.Warning("cleanup mihomo install backup workspace failed: ", err)
	}
	finalized = true

	FinishCoreDownloadProgressSuccess(sessionID, coreDownloadStageCompleted)
	return localVersion, nil
}

func (s *MihomoCoreManagerService) DownloadCoreFromURL(downloadURL string, requestedSessionID string) (string, error) {
	operationContext, operation, err := BeginKworManagedOperation("mihomo-core-download")
	if err != nil {
		return "", err
	}
	defer operation.Done()
	return s.downloadCoreFromURLWithContext(operationContext, downloadURL, requestedSessionID)
}

func (s *MihomoCoreManagerService) downloadCoreFromURLWithContext(operationContext context.Context, downloadURL string, requestedSessionID string) (string, error) {
	lifecycleLock, err := AcquireKworLifecycleLockContext(operationContext)
	if err != nil {
		return "", err
	}
	defer func() { _ = lifecycleLock.Release() }()

	if err := lockManagedDownloadTaskMutex(operationContext, &s.mu); err != nil {
		return "", err
	}
	defer s.mu.Unlock()

	sessionID := StartCoreDownloadProgressSession("mihomo", requestedSessionID, false)
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
	if err := cleanupStaleManagedCoreInstallWorkspaces(s.getCoreDir(), mihomoCoreInstallStagePrefix, mihomoCoreInstallBackupPrefix); err != nil {
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
	SetCoreDownloadProgressStage(sessionID, coreDownloadStageDownloading)

	client := &http.Client{Timeout: 600 * time.Second}
	req, err := http.NewRequestWithContext(operationContext, "GET", downloadURL, nil)
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
	tmpFile := filepath.Join(coreDir, "mihomo-custom-download"+ext)
	defer os.Remove(tmpFile)

	out, err := os.Create(tmpFile)
	if err != nil {
		err = fmt.Errorf("create temp file failed: %v", err)
		failProgress(coreDownloadStageDownloading, err)
		return "", err
	}

	if _, err = copyManagedDownloadTaskContext(operationContext, out, io.TeeReader(resp.Body, &coreDownloadProgressWriter{sessionID: sessionID})); err != nil {
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
	if err = operationContext.Err(); err != nil {
		failProgress(coreDownloadStageDownloading, err)
		return "", err
	}

	SetCoreDownloadProgressStage(sessionID, coreDownloadStageExtracting)
	stageDir, cleanupStageDir, err := createManagedCoreInstallWorkspace(coreDir, mihomoCoreInstallStagePrefix)
	if err != nil {
		failProgress(coreDownloadStageExtracting, err)
		return "", err
	}
	defer cleanupStageDir()

	if err = s.installCoreFromArchiveFileContext(operationContext, tmpFile, stageDir); err != nil {
		err = fmt.Errorf("extract/install failed: %v", err)
		failProgress(coreDownloadStageExtracting, err)
		return "", err
	}
	if err = operationContext.Err(); err != nil {
		failProgress(coreDownloadStageExtracting, err)
		return "", err
	}

	binName := s.getCoreBinName()
	stagedBinPath := filepath.Join(stageDir, binName)
	SetCoreDownloadProgressStage(sessionID, coreDownloadStageValidating)
	if !s.validateCoreBinary(stagedBinPath) {
		err = fmt.Errorf("downloaded mihomo binary is not executable on current runtime %s/%s", GetSystemPlatformOS(), GetSystemPlatformArchitecture())
		failProgress(coreDownloadStageValidating, err)
		return "", err
	}
	if !beginManagedCoreDownloadApplying(sessionID, coreDownloadStageReplacing) {
		err = coreDownloadTaskCancelledError(operationContext)
		failProgress(coreDownloadStageValidating, err)
		return "", err
	}
	SetCoreDownloadProgressStage(sessionID, coreDownloadStageReplacing)

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
			return activateManagedCoreBinaryInstall(coreDir, binName, stageDir, mihomoCoreInstallBackupPrefix)
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
				logger.Warning("rollback mihomo custom staged install failed: ", rollbackErr)
			}
		}
	}()

	binPath := filepath.Join(coreDir, binName)
	localVersion, _ := s.getLocalVersion(binPath)
	if clearErr := s.ClearCoreAutoUpdateDisableReason(); clearErr != nil {
		logger.Warning("clear Mihomo auto update disable reason after custom download failed: ", clearErr)
	}
	if wasRunning {
		SetCoreDownloadProgressStage(sessionID, coreDownloadStageStarting)
		if err = s.startCoreLocked(); err != nil {
			rollbackErr := activation.Rollback()
			finalized = true
			if rollbackErr == nil {
				if restartErr := s.startCoreLocked(); restartErr != nil {
					err = fmt.Errorf("download completed, but new mihomo auto start failed: %v; rolled back old core, but old core restart failed: %v", err, restartErr)
				} else {
					err = fmt.Errorf("download completed, but new mihomo auto start failed and was rolled back to previous version: %v", err)
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
		logger.Warning("cleanup mihomo custom install backup workspace failed: ", err)
	}
	finalized = true
	FinishCoreDownloadProgressSuccess(sessionID, coreDownloadStageCompleted)
	return localVersion, nil
}

func (s *MihomoCoreManagerService) extractZip(zipPath, destDir, binName string) error {
	tmp := &CoreManagerService{}
	return tmp.extractZip(zipPath, destDir, binName)
}

func (s *MihomoCoreManagerService) extractTarGz(tarGzPath, destDir, binName string) error {
	tmp := &CoreManagerService{}
	return tmp.extractTarGz(tarGzPath, destDir, binName)
}

func (s *MihomoCoreManagerService) extractCoreByExternalTool(archivePath, destDir, binName string) error {
	tmp := &CoreManagerService{}
	return tmp.extractCoreByExternalTool(archivePath, destDir, binName)
}

func (s *MihomoCoreManagerService) validateCoreBinary(binPath string) bool {
	if !IsSystemPlatformWindows() {
		inspection := inspectManagedLinuxCoreBinary(binPath, "mihomo", func(statInfo os.FileInfo, forceRefresh bool) (string, string) {
			return s.getCachedLocalVersion(binPath, statInfo, forceRefresh)
		})
		return inspection.Compatible
	}

	version, output := s.getLocalVersion(binPath)
	return version != "" && managedCoreVersionOutputMatches(output, "mihomo")
}

func (s *MihomoCoreManagerService) StartCore() (err error) {
	lifecycleLock, lockErr := AcquireKworLifecycleLock()
	if lockErr != nil {
		return lockErr
	}
	defer func() { _ = lifecycleLock.Release() }()

	s.mu.Lock()
	defer func() {
		if err == nil {
			SyncPortForwardNftablesAfterCoreRuntimeReady()
			notifyCertificateCoreLoadedLatestConfig(certificateCoreKindMihomo)
		}
	}()
	defer s.mu.Unlock()

	if err := EnsureManagedCoreLayout(); err != nil {
		return err
	}

	if s.isRunning() {
		return fmt.Errorf("core is already running")
	}

	binPath := s.getCoreBinPath()
	if _, err := os.Stat(binPath); os.IsNotExist(err) {
		return fmt.Errorf("core file does not exist: %s", binPath)
	}
	if !s.validateCoreBinary(binPath) {
		return fmt.Errorf("core binary is not compatible with current runtime %s/%s", GetSystemPlatformOS(), GetSystemPlatformArchitecture())
	}

	if err := s.regenerateRuntimeConfig(); err != nil {
		return err
	}

	configPath := s.getConfigPath()
	configExists, err := ManagedRuntimeFileExists(configPath)
	if err != nil {
		return err
	}
	if !configExists {
		return fmt.Errorf("config file does not exist: %s", configPath)
	}
	if err := MaterializeManagedRuntimeCoreFile(configPath, managedCoreConfigMaterializeTTL); err != nil {
		return fmt.Errorf("prepare config file failed: %v", err)
	}

	coreDir := s.getCoreDir()
	absCoreDir, _ := filepath.Abs(coreDir)
	if err := RegisterManagedCoreHostOwnership("mihomo", binPath, mihomoSystemdName); err != nil {
		DiscardMaterializedManagedRuntimeCoreFile(configPath)
		return fmt.Errorf("record mihomo ownership before start: %w", err)
	}
	if IsSystemPlatformWindows() {
		err = s.startCoreWindows(absCoreDir)
	} else {
		err = s.startCoreLinux(absCoreDir)
	}
	if err != nil {
		DiscardMaterializedManagedRuntimeCoreFile(configPath)
		return err
	}
	if IsSystemPlatformLinux() {
		if markerErr := markManagedCoreShouldRun("mihomo"); markerErr != nil {
			logger.Warning("failed to persist mihomo runtime marker: ", markerErr)
		}
	}
	return nil
}

func (s *MihomoCoreManagerService) startCoreLocked() error {
	if err := EnsureManagedCoreLayout(); err != nil {
		return err
	}

	binPath := s.getCoreBinPath()
	if _, err := os.Stat(binPath); os.IsNotExist(err) {
		return fmt.Errorf("core file does not exist: %s", binPath)
	}
	if !s.validateCoreBinary(binPath) {
		return fmt.Errorf("core binary is not compatible with current runtime %s/%s", GetSystemPlatformOS(), GetSystemPlatformArchitecture())
	}

	if err := s.regenerateRuntimeConfig(); err != nil {
		return err
	}

	configPath := s.getConfigPath()
	configExists, err := ManagedRuntimeFileExists(configPath)
	if err != nil {
		return err
	}
	if !configExists {
		return fmt.Errorf("config file does not exist: %s", configPath)
	}
	if err := MaterializeManagedRuntimeCoreFile(configPath, managedCoreConfigMaterializeTTL); err != nil {
		return fmt.Errorf("prepare config file failed: %v", err)
	}

	coreDir := s.getCoreDir()
	absCoreDir, _ := filepath.Abs(coreDir)
	if err := RegisterManagedCoreHostOwnership("mihomo", binPath, mihomoSystemdName); err != nil {
		DiscardMaterializedManagedRuntimeCoreFile(configPath)
		return fmt.Errorf("record mihomo ownership before start: %w", err)
	}
	if IsSystemPlatformWindows() {
		err = s.startCoreWindows(absCoreDir)
	} else {
		err = s.startCoreLinux(absCoreDir)
	}
	if err != nil {
		DiscardMaterializedManagedRuntimeCoreFile(configPath)
		return err
	}
	if IsSystemPlatformLinux() {
		if markerErr := markManagedCoreShouldRun("mihomo"); markerErr != nil {
			logger.Warning("failed to persist mihomo runtime marker: ", markerErr)
		}
	}
	return nil
}

func (s *MihomoCoreManagerService) StopCore() error {
	lifecycleLock, err := AcquireKworLifecycleLock()
	if err != nil {
		return err
	}
	defer func() { _ = lifecycleLock.Release() }()

	s.mu.Lock()
	defer s.mu.Unlock()

	if IsSystemPlatformLinux() {
		err := s.stopCoreLinuxFull()
		if err == nil {
			clearManagedCoreShouldRun("mihomo")
		}
		return err
	}
	return s.stopCoreInternal()
}

func (s *MihomoCoreManagerService) DeleteCore() error {
	lifecycleLock, err := AcquireKworLifecycleLock()
	if err != nil {
		return err
	}
	defer func() { _ = lifecycleLock.Release() }()

	s.mu.Lock()
	defer s.mu.Unlock()

	if err := EnsureManagedCoreLayout(); err != nil {
		return err
	}

	if IsSystemPlatformLinux() {
		if err := s.stopCoreLinuxFull(); err != nil {
			return err
		}
		s.cleanupLegacyMihomoSystemdServices()
		s.removeMihomoSystemdService()
		clearManagedCoreShouldRun("mihomo")
	} else {
		if err := s.stopCoreInternal(); err != nil {
			return err
		}
	}

	binPath := s.getCoreBinPath()
	if err := os.Remove(binPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to remove core binary %s: %v", binPath, err)
	}
	if err := cleanupManagedCoreRuntimeArtifacts(s.getCoreDir(), s.getCoreBinName()); err != nil {
		return err
	}

	s.isStarted = false
	s.coreCmd = nil
	clearMihomoLocalVersionCache(binPath)
	clearMihomoCoreLocalTargetCache(binPath)
	if disableErr := s.DisableCoreAutoUpdate("本地未安装 Mihomo 内核"); disableErr != nil {
		logger.Warning("disable Mihomo auto update after core delete failed: ", disableErr)
	}
	return nil
}

func (s *MihomoCoreManagerService) RestartCore() (err error) {
	lifecycleLock, lockErr := AcquireKworLifecycleLock()
	if lockErr != nil {
		return lockErr
	}
	defer func() { _ = lifecycleLock.Release() }()

	s.mu.Lock()
	defer func() {
		if err == nil {
			SyncPortForwardNftablesAfterCoreRuntimeReady()
			notifyCertificateCoreLoadedLatestConfig(certificateCoreKindMihomo)
		}
	}()
	defer s.mu.Unlock()

	if err := EnsureManagedCoreLayout(); err != nil {
		return err
	}
	if !s.validateCoreBinary(s.getCoreBinPath()) {
		return fmt.Errorf("core binary is not compatible with current runtime %s/%s", GetSystemPlatformOS(), GetSystemPlatformArchitecture())
	}

	if IsSystemPlatformLinux() && !shouldUseDirectManagedCoreRuntime() && s.isMihomoSystemdActive() {
		if err := s.regenerateRuntimeConfig(); err != nil {
			return err
		}

		configPath := s.getConfigPath()
		configExists, err := ManagedRuntimeFileExists(configPath)
		if err != nil {
			return err
		}
		if !configExists {
			return fmt.Errorf("config file does not exist: %s", configPath)
		}
		if err := MaterializeManagedRuntimeCoreFile(configPath, managedCoreConfigMaterializeTTL); err != nil {
			return fmt.Errorf("prepare config file failed: %v", err)
		}
		coreDir := s.getCoreDir()
		absCoreDir, _ := filepath.Abs(coreDir)
		if err := s.createMihomoSystemdService(s.getCoreBinPath(), configPath, absCoreDir); err != nil {
			DiscardMaterializedManagedRuntimeCoreFile(configPath)
			return fmt.Errorf("refresh systemd service for mihomo failed: %v", err)
		}
		if err := runSystemctlCommand("restart", mihomoSystemdName); err != nil {
			DiscardMaterializedManagedRuntimeCoreFile(configPath)
			return fmt.Errorf("systemd restart mihomo failed: %v", err)
		}
		s.isStarted = true
		if markerErr := markManagedCoreShouldRun("mihomo"); markerErr != nil {
			logger.Warning("failed to persist mihomo runtime marker: ", markerErr)
		}
		return nil
	}

	_ = s.stopCoreInternal()
	time.Sleep(1 * time.Second)
	if err := s.startCoreLocked(); err != nil {
		return err
	}
	if IsSystemPlatformLinux() {
		if markerErr := markManagedCoreShouldRun("mihomo"); markerErr != nil {
			logger.Warning("failed to persist mihomo runtime marker: ", markerErr)
		}
	}
	return nil
}

func (s *MihomoCoreManagerService) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.isRunning()
}

func (s *MihomoCoreManagerService) isRunning() bool {
	if IsSystemPlatformLinux() && !shouldUseDirectManagedCoreRuntime() {
		if s.isMihomoSystemdActive() {
			s.isStarted = true
			return true
		}
	}
	if s.isStarted && s.coreCmd != nil && s.coreCmd.Process != nil {
		tmp := &CoreManagerService{}
		if tmp.isProcessAlive(s.coreCmd.Process.Pid) {
			return true
		}
		s.isStarted = false
		s.coreCmd = nil
	}
	if IsSystemPlatformLinux() && shouldUseDirectManagedCoreRuntime() && isManagedCoreProcessRunningByBinaryPath(s.getCoreBinPath()) {
		s.isStarted = true
		return true
	}
	if IsSystemPlatformWindows() && isManagedCoreProcessRunningByBinaryPath(s.getCoreBinPath()) {
		s.isStarted = true
		return true
	}
	return false
}

func (s *MihomoCoreManagerService) isMihomoSystemdActive() bool {
	return systemctlUnitIsActive(mihomoSystemdName)
}

func (s *MihomoCoreManagerService) startCoreWindows(coreDir string) error {
	binPath := filepath.Join(coreDir, s.getCoreBinName())
	configPath := s.getConfigPath()
	s.coreCmd = exec.Command(binPath, "-d", coreDir, "-f", configPath)
	s.coreCmd.Dir = coreDir
	s.coreCmd.Env = append(os.Environ(),
		"XDG_CONFIG_HOME="+filepath.Join(coreDir, ".config"),
		"XDG_CACHE_HOME="+filepath.Join(coreDir, ".cache"),
	)
	s.stdout = nil
	s.stderr = nil
	if err := s.coreCmd.Start(); err != nil {
		s.coreCmd = nil
		return fmt.Errorf("start core failed: %v", err)
	}
	s.isStarted = true
	startedCmd := s.coreCmd
	waitManagedCoreCommandAsync(startedCmd, func() {
		s.mu.Lock()
		if s.coreCmd == startedCmd {
			s.isStarted = false
			s.coreCmd = nil
			closeManagedCoreDirectStdStreams(s.stdout, s.stderr)
			s.stdout = nil
			s.stderr = nil
		}
		s.mu.Unlock()
	})
	return nil
}

func (s *MihomoCoreManagerService) startCoreDirectLinux(coreDir string) error {
	binPath := filepath.Join(coreDir, s.getCoreBinName())
	configPath := s.getConfigPath()
	s.coreCmd = exec.Command(binPath, "-d", coreDir, "-f", configPath)
	s.coreCmd.Dir = coreDir
	s.coreCmd.Env = append(os.Environ(),
		"XDG_CONFIG_HOME="+filepath.Join(coreDir, ".config"),
		"XDG_CACHE_HOME="+filepath.Join(coreDir, ".cache"),
	)
	s.stdout, s.stderr = resolveManagedCoreDirectStdStreams()
	s.coreCmd.Stdout = s.stdout
	s.coreCmd.Stderr = s.stderr
	if err := s.coreCmd.Start(); err != nil {
		closeManagedCoreDirectStdStreams(s.stdout, s.stderr)
		s.stdout = nil
		s.stderr = nil
		s.coreCmd = nil
		return fmt.Errorf("direct start mihomo failed: %v", err)
	}
	s.isStarted = true
	logger.Info("mihomo 内核已直接启动 (Linux, 无systemd), PID: ", s.coreCmd.Process.Pid)
	startedCmd := s.coreCmd
	waitManagedCoreCommandAsync(startedCmd, func() {
		s.mu.Lock()
		if s.coreCmd == startedCmd {
			s.isStarted = false
			s.coreCmd = nil
			closeManagedCoreDirectStdStreams(s.stdout, s.stderr)
			s.stdout = nil
			s.stderr = nil
		}
		s.mu.Unlock()
	})
	return nil
}

func (s *MihomoCoreManagerService) startCoreLinux(coreDir string) error {
	if shouldUseDirectManagedCoreRuntime() {
		return s.startCoreDirectLinux(coreDir)
	}

	binPath := filepath.Join(coreDir, s.getCoreBinName())
	configPath := s.getConfigPath()

	s.cleanupLegacyMihomoSystemdServices()
	if err := s.createMihomoSystemdService(binPath, configPath, coreDir); err != nil {
		return err
	}

	_ = runSystemctlCommand("reset-failed", mihomoSystemdName)
	startOutput, startErr := runSystemctlOutput("start", mihomoSystemdName)
	if startErr != nil {
		diagnostics := collectSystemdStartupDiagnostics(mihomoSystemdName, systemdCoreJournalTailLines)
		message := buildSystemdActivationErrorMessage(
			mihomoSystemdName,
			systemdUnitActivationResult{State: "start-command-failed", LastErr: startErr},
			string(startOutput),
			diagnostics,
		)
		logger.Warning("systemd start mihomo failed: ", message)
		return fmt.Errorf("%s", message)
	}

	waitResult := waitForSystemdUnitActive(mihomoSystemdName, systemdCoreStartWaitTimeout)
	if waitResult.State != "active" {
		diagnostics := collectSystemdStartupDiagnostics(mihomoSystemdName, systemdCoreJournalTailLines)
		message := buildSystemdActivationErrorMessage(
			mihomoSystemdName,
			waitResult,
			string(startOutput),
			diagnostics,
		)
		logger.Warning("systemd 启动 mihomo 后未进入 active: ", message)
		return fmt.Errorf("%s", message)
	}
	stableResult := waitForSystemdUnitRemainActive(mihomoSystemdName, systemdCorePostActiveHold)
	if stableResult.State != "active" {
		diagnostics := collectSystemdStartupDiagnostics(mihomoSystemdName, systemdCoreJournalTailLines)
		message := buildSystemdActivationErrorMessage(
			mihomoSystemdName,
			stableResult,
			"unit dropped out of active shortly after start",
			diagnostics,
		)
		logger.Warning("mihomo systemd 启动后未保持 active: ", message)
		return fmt.Errorf("%s", message)
	}

	s.isStarted = true
	return nil
}

func (s *MihomoCoreManagerService) stopCoreLinuxFull() error {
	if shouldUseDirectManagedCoreRuntime() {
		if err := s.stopCoreInternal(); err != nil {
			return err
		}
		s.cleanupLegacyMihomoSystemdServices()
		s.removeMihomoSystemdService()
		clearManagedCoreShouldRun("mihomo")
		return nil
	}
	_ = s.stopCoreInternal()
	s.removeMihomoSystemdService()
	clearManagedCoreShouldRun("mihomo")
	return nil
}

func (s *MihomoCoreManagerService) stopCoreInternal() error {
	if IsSystemPlatformLinux() && !shouldUseDirectManagedCoreRuntime() && s.isMihomoSystemdActive() {
		if err := runSystemctlCommand("stop", mihomoSystemdName); err == nil {
			time.Sleep(300 * time.Millisecond)
			if s.isMihomoSystemdActive() {
				return fmt.Errorf("mihomo systemd service is still active after stop request")
			}
			s.isStarted = false
			s.coreCmd = nil
			closeManagedCoreDirectStdStreams(s.stdout, s.stderr)
			s.stdout = nil
			s.stderr = nil
			return nil
		} else {
			return fmt.Errorf("failed to stop mihomo systemd service: %v", err)
		}
	}

	if IsSystemPlatformLinux() && shouldUseDirectManagedCoreRuntime() {
		if err := terminateManagedCoreProcessesByBinaryPath(s.getCoreBinPath(), 5*time.Second); err != nil {
			return fmt.Errorf("failed to stop mihomo direct runtime process: %v", err)
		}
		s.isStarted = false
		s.coreCmd = nil
		closeManagedCoreDirectStdStreams(s.stdout, s.stderr)
		s.stdout = nil
		s.stderr = nil
		return nil
	}

	if s.coreCmd != nil && s.coreCmd.Process != nil {
		pid := s.coreCmd.Process.Pid
		if IsSystemPlatformWindows() {
			if err := runCommandWithTimeout(systemCommandTimeout, "taskkill", "/F", "/T", "/PID", fmt.Sprintf("%d", pid)); err != nil {
				return fmt.Errorf("failed to stop mihomo process %d: %v", pid, err)
			}
		} else {
			if err := s.coreCmd.Process.Signal(os.Interrupt); err != nil {
				return fmt.Errorf("failed to interrupt mihomo process %d: %v", pid, err)
			}
			deadline := time.Now().Add(5 * time.Second)
			for time.Now().Before(deadline) {
				if !managedCoreProcessPIDAlive(pid) {
					break
				}
				time.Sleep(120 * time.Millisecond)
			}
			if managedCoreProcessPIDAlive(pid) {
				if err := s.coreCmd.Process.Kill(); err != nil {
					return fmt.Errorf("failed to kill mihomo process %d: %v", pid, err)
				}
			}
		}
		if managedCoreProcessPIDAlive(pid) {
			return fmt.Errorf("mihomo process %d is still alive after stop request", pid)
		}
	}

	// The panel can be restarted while a managed mihomo.exe remains running.
	// In that case coreCmd is unavailable, so stop the exact managed binary
	// path rather than falsely treating the runtime as already stopped.
	if IsSystemPlatformWindows() {
		if err := terminateManagedCoreProcessesByBinaryPath(s.getCoreBinPath(), 5*time.Second); err != nil {
			return fmt.Errorf("failed to stop mihomo managed Windows process: %v", err)
		}
	}

	s.isStarted = false
	s.coreCmd = nil
	closeManagedCoreDirectStdStreams(s.stdout, s.stderr)
	s.stdout = nil
	s.stderr = nil
	return nil
}

func (s *MihomoCoreManagerService) cleanupLegacyMihomoSystemdServices() {
	for _, serviceName := range legacyMihomoSystemdNames {
		if serviceName == mihomoSystemdName {
			continue
		}
		s.removeSystemdServiceByName(serviceName)
	}
}

func (s *MihomoCoreManagerService) removeSystemdServiceByName(serviceName string) {
	if serviceName == "" {
		return
	}
	useSystemctl := IsSystemPlatformLinux() && !shouldUseDirectManagedCoreRuntime()
	if useSystemctl {
		_ = runSystemctlCommand("stop", serviceName)
		_ = runSystemctlCommand("disable", serviceName)
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
			_ = runSystemctlCommand("daemon-reload")
			_ = runSystemctlCommand("reset-failed")
		}
	}
}

func (s *MihomoCoreManagerService) createMihomoSystemdService(binPath, configPath, workDir string) error {
	controlPath := getSystemdControlBinaryPath()
	serviceContent := buildMihomoSystemdServiceContent(controlPath, binPath, configPath, workDir)

	servicePath := getMihomoServiceFilePath()
	ownership, err := BeginSystemdHostOwnership("core-mihomo-systemd", mihomoSystemdName, []string{servicePath}, map[string]string{
		"binary": binPath,
		"config": configPath,
	})
	if err != nil {
		return fmt.Errorf("record pending mihomo systemd ownership failed: %w", err)
	}
	if err := os.WriteFile(servicePath, []byte(serviceContent), 0o644); err != nil {
		return fmt.Errorf("unable to write systemd service file %s: %v", servicePath, err)
	}
	if err := verifySystemdUnitFile(servicePath); err != nil {
		return err
	}
	if err := runSystemctlCommand("daemon-reload"); err != nil {
		return fmt.Errorf("systemctl daemon-reload failed: %v", err)
	}
	if err := VerifyAndActivateHostResource(ownership.ID); err != nil {
		return fmt.Errorf("activate mihomo systemd ownership failed: %w", err)
	}
	if err := RegisterManagedCoreHostOwnership("mihomo", binPath, mihomoSystemdName); err != nil {
		return fmt.Errorf("record mihomo ownership failed: %w", err)
	}
	return nil
}

func buildMihomoSystemdServiceContent(controlPath, binPath, configPath, workDir string) string {
	return fmt.Sprintf(`# kwor-owner:v1 resource=core-mihomo
[Unit]
Description=kwor mihomo service
Documentation=https://wiki.metacubex.one
After=network.target nss-lookup.target

[Service]
Type=simple
Environment=%s
Environment=%s
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
		quoteSystemdEnvironmentAssignment("XDG_CONFIG_HOME", filepath.ToSlash(filepath.Join(workDir, ".config"))),
		quoteSystemdEnvironmentAssignment("XDG_CACHE_HOME", filepath.ToSlash(filepath.Join(workDir, ".cache"))),
		quoteSystemdEnvironmentAssignment(InternalSystemdCommandEnv, "1"),
		buildSystemdExecCommand(controlPath, "materialize-core-config", "mihomo"),
		buildSystemdExecCommand(binPath, "-d", workDir, "-f", configPath),
		buildSystemdExecCommand(controlPath, "cleanup-core-config", "mihomo"),
		escapeSystemdUnitValue(workDir),
	)
}

func (s *MihomoCoreManagerService) removeMihomoSystemdService() {
	s.removeSystemdServiceByName(mihomoSystemdName)

	servicePath := getMihomoServiceFilePath()
	if _, err := os.Stat(servicePath); os.IsNotExist(err) {
		return
	}
	useSystemctl := IsSystemPlatformLinux() && !shouldUseDirectManagedCoreRuntime()
	if useSystemctl {
		_ = runSystemctlCommand("disable", mihomoSystemdName)
	}
	if err := os.Remove(servicePath); err != nil {
		logger.Warning("unable to remove systemd service file: ", err)
		return
	}
	if useSystemctl {
		_ = runSystemctlCommand("daemon-reload")
		_ = runSystemctlCommand("reset-failed")
	}
}

func (s *MihomoCoreManagerService) getCoreAutoCheckSettings() (enabled bool, intervalHours int, lastCheckedAt int64, err error) {
	settingSvc := &SettingService{}

	enabled, err = settingSvc.getBool(mihomoCoreAutoCheckEnabledKey)
	if err != nil {
		return false, 12, 0, err
	}

	intervalRaw, err := settingSvc.getString(mihomoCoreAutoCheckIntervalHoursKey)
	if err != nil {
		return false, 12, 0, err
	}
	intervalHours = normalizeCoreAutoCheckIntervalHours(intervalRaw)

	lastRaw, err := settingSvc.getString(mihomoCoreAutoCheckLastAtKey)
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

func (s *MihomoCoreManagerService) buildMihomoCoreUpdateInfo() (*MihomoCoreUpdateInfo, error) {
	enabled, intervalHours, lastCheckedAt, err := s.getCoreAutoCheckSettings()
	if err != nil {
		return nil, err
	}

	settingSvc := &SettingService{}
	latestStable, err := settingSvc.getString(mihomoCoreAutoCheckLatestStableKey)
	if err != nil {
		return nil, err
	}
	latestAlpha, err := settingSvc.getString(mihomoCoreAutoCheckLatestAlphaKey)
	if err != nil {
		return nil, err
	}
	pendingStable, err := settingSvc.getString(mihomoCoreAutoCheckPendingStableKey)
	if err != nil {
		return nil, err
	}
	pendingAlpha, err := settingSvc.getString(mihomoCoreAutoCheckPendingAlphaKey)
	if err != nil {
		return nil, err
	}
	autoUpdateState, err := s.getCoreAutoUpdateState()
	if err != nil {
		return nil, err
	}
	status, statusErr := s.GetCoreStatus()
	if statusErr == nil {
		pendingStable, pendingAlpha = selectMihomoPendingUpdateForChannel(status, pendingStable, pendingAlpha)
	} else {
		pendingStable = ""
		pendingAlpha = ""
	}

	info := &MihomoCoreUpdateInfo{
		Enabled:                 enabled,
		IntervalHours:           intervalHours,
		LastCheckedAt:           lastCheckedAt,
		LatestStable:            latestStable,
		LatestAlpha:             latestAlpha,
		PendingStable:           pendingStable,
		PendingAlpha:            pendingAlpha,
		AutoUpdateEnabled:       autoUpdateState.Enabled,
		AutoUpdateLastAttemptAt: autoUpdateState.LastAttemptAt,
		AutoUpdateLastSuccessAt: autoUpdateState.LastSuccessAt,
		AutoUpdateError:         autoUpdateState.Error,
		AutoUpdateErrorAt:       autoUpdateState.ErrorAt,
	}
	if statusErr == nil {
		info.AutoUpdateDisabled = mihomoShouldDisableAutoUpdateInUI(autoUpdateState.Enabled, status)
		if info.AutoUpdateDisabled {
			if autoUpdateState.DisableReason != "" {
				info.AutoUpdateDisableReason = autoUpdateState.DisableReason
			} else {
				info.AutoUpdateDisableReason = mihomoAutoUpdateReadinessReason(status)
			}
		}
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

func (s *MihomoCoreManagerService) SetCoreAutoCheckSettings(enabled bool, intervalHours int, hasAutoUpdate bool, autoUpdateEnabled bool) error {
	if intervalHours <= 0 {
		intervalHours = 12
	}

	mihomoCoreAutoCheckMu.Lock()
	defer mihomoCoreAutoCheckMu.Unlock()

	settingSvc := &SettingService{}
	prevAutoUpdateEnabled, err := settingSvc.getBool(mihomoCoreAutoUpdateEnabledKey)
	if err != nil {
		return err
	}
	if err := settingSvc.setString(mihomoCoreAutoCheckEnabledKey, strconv.FormatBool(enabled)); err != nil {
		return err
	}
	if err := settingSvc.setString(mihomoCoreAutoCheckIntervalHoursKey, strconv.Itoa(intervalHours)); err != nil {
		return err
	}
	if hasAutoUpdate {
		if autoUpdateEnabled && !prevAutoUpdateEnabled {
			status, err := s.GetCoreStatus()
			if err != nil {
				return err
			}
			if reason := mihomoAutoUpdateReadinessReason(status); reason != "" {
				return fmt.Errorf("%s", reason)
			}
		}
		if err := s.setCoreAutoUpdateEnabledLocked(settingSvc, autoUpdateEnabled); err != nil {
			return err
		}
		if autoUpdateEnabled {
			if err := s.clearCoreAutoUpdateDisableReasonLocked(settingSvc); err != nil {
				return err
			}
		}
	}
	if !enabled {
		if err := settingSvc.setString(mihomoCoreAutoCheckPendingStableKey, ""); err != nil {
			return err
		}
		if err := settingSvc.setString(mihomoCoreAutoCheckPendingAlphaKey, ""); err != nil {
			return err
		}
	}
	return nil
}

func (s *MihomoCoreManagerService) ClearCoreUpdatePending() error {
	mihomoCoreAutoCheckMu.Lock()
	defer mihomoCoreAutoCheckMu.Unlock()

	settingSvc := &SettingService{}
	if err := settingSvc.setString(mihomoCoreAutoCheckPendingStableKey, ""); err != nil {
		return err
	}
	if err := settingSvc.setString(mihomoCoreAutoCheckPendingAlphaKey, ""); err != nil {
		return err
	}
	return nil
}

func (s *MihomoCoreManagerService) GetMihomoCoreUpdateInfo(forceCheck bool) (*MihomoCoreUpdateInfo, error) {
	if forceCheck {
		if err := s.CheckAndMarkCoreUpdates(true); err != nil {
			logger.Warning("check mihomo core updates failed: ", err)
		}
	}
	return s.buildMihomoCoreUpdateInfo()
}

func (s *MihomoCoreManagerService) fetchLatestStableTag(client *http.Client) (string, error) {
	req, err := http.NewRequest("GET", "https://api.github.com/repos/MetaCubeX/mihomo/releases/latest", nil)
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

	var release GitHubRelease
	if err = unmarshalBoundedHTTPResponseJSON(resp.Body, coreGitHubResponseMaxBytes, &release); err != nil {
		return "", fmt.Errorf("failed to parse latest stable release: %v", err)
	}
	return strings.TrimSpace(release.TagName), nil
}

func (s *MihomoCoreManagerService) fetchLatestAlphaRelease(client *http.Client) (GitHubRelease, string, error) {
	const (
		perPage  = coreReleaseGitHubPerPage
		maxPages = 3
	)

	for page := 1; page <= maxPages; page++ {
		releases, err := s.fetchGitHubReleasePage(client, page, perPage)
		if err != nil {
			return GitHubRelease{}, "", err
		}
		if len(releases) == 0 {
			break
		}
		for _, release := range releases {
			if release.Prerelease {
				identity := mihomoReleaseVersionIdentity(release)
				if version, versionErr := s.fetchMihomoReleaseVersion(client, release); versionErr == nil {
					identity = version
				}
				return release, identity, nil
			}
		}
		if len(releases) < perPage {
			break
		}
	}
	return GitHubRelease{}, "", fmt.Errorf("no Mihomo prerelease release found")
}

func (s *MihomoCoreManagerService) CheckAndMarkCoreUpdates(force bool) error {
	mihomoCoreAutoCheckMu.Lock()
	defer mihomoCoreAutoCheckMu.Unlock()

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
	_, latestAlpha, alphaErr := s.fetchLatestAlphaRelease(client)
	settingSvc := &SettingService{}
	prevStable, err := settingSvc.getString(mihomoCoreAutoCheckLatestStableKey)
	if err != nil {
		return err
	}
	prevAlpha, err := settingSvc.getString(mihomoCoreAutoCheckLatestAlphaKey)
	if err != nil {
		return err
	}

	if err = settingSvc.setString(mihomoCoreAutoCheckLastAtKey, strconv.FormatInt(now, 10)); err != nil {
		return err
	}
	if latestStable != prevStable {
		if err = settingSvc.setString(mihomoCoreAutoCheckLatestStableKey, latestStable); err != nil {
			return err
		}
		if err = settingSvc.setString(mihomoCoreAutoCheckPendingStableKey, latestStable); err != nil {
			return err
		}
	}
	if alphaErr == nil && latestAlpha != prevAlpha {
		if err = settingSvc.setString(mihomoCoreAutoCheckLatestAlphaKey, latestAlpha); err != nil {
			return err
		}
		if err = settingSvc.setString(mihomoCoreAutoCheckPendingAlphaKey, latestAlpha); err != nil {
			return err
		}
	}
	if alphaErr != nil {
		logger.Warning("check latest Mihomo prerelease failed; keep the previous Alpha state: ", alphaErr)
	}
	return nil
}
