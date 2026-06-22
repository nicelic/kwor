package service

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alireza0/s-ui/logger"
)

type MihomoCoreManagerService struct {
	mu        sync.Mutex
	coreCmd   *exec.Cmd
	isStarted bool
	stdout    *os.File
	stderr    *os.File
}

const (
	mihomoSystemdName = "kwor-mihomo"

	mihomoCoreAutoCheckEnabledKey       = "mihomoCoreAutoCheckEnabled"
	mihomoCoreAutoCheckIntervalHoursKey = "mihomoCoreAutoCheckIntervalHours"
	mihomoCoreAutoCheckLastAtKey        = "mihomoCoreAutoCheckLastAt"
	mihomoCoreAutoCheckLatestStableKey  = "mihomoCoreAutoCheckLatestStable"
	mihomoCoreAutoCheckLatestAlphaKey   = "mihomoCoreAutoCheckLatestAlpha"
	mihomoCoreAutoCheckPendingStableKey = "mihomoCoreAutoCheckPendingStable"
	mihomoCoreAutoCheckPendingAlphaKey  = "mihomoCoreAutoCheckPendingAlpha"
)

var (
	legacyMihomoSystemdNames = []string{
		"mihomo",
		"metacubex-mihomo",
		"s-ui-mihomo",
		"sui-mihomo",
	}
	mihomoCoreAutoCheckMu sync.Mutex
	mihomoVersionRe       = regexp.MustCompile(`v?\d+\.\d+\.\d+(?:[-+._A-Za-z0-9]+)?`)
)

type mihomoLocalVersionCacheEntry struct {
	expiresAt   time.Time
	binModTime  time.Time
	binSize     int64
	version     string
	versionInfo string
}

var mihomoLocalVersionCache = struct {
	sync.Mutex
	items map[string]mihomoLocalVersionCacheEntry
}{
	items: make(map[string]mihomoLocalVersionCacheEntry),
}

func getMihomoLocalVersionCache(binPath string, binModTime time.Time, binSize int64) (string, string, bool) {
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
	if !entry.binModTime.Equal(binModTime) || entry.binSize != binSize {
		delete(mihomoLocalVersionCache.items, binPath)
		return "", "", false
	}
	return entry.version, entry.versionInfo, true
}

func setMihomoLocalVersionCache(binPath string, binModTime time.Time, binSize int64, version string, versionInfo string) {
	mihomoLocalVersionCache.Lock()
	defer mihomoLocalVersionCache.Unlock()

	mihomoLocalVersionCache.items[binPath] = mihomoLocalVersionCacheEntry{
		expiresAt:   time.Now().Add(coreLocalVersionCacheTTL),
		binModTime:  binModTime,
		binSize:     binSize,
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
	if runtime.GOOS == "windows" {
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
	return fmt.Sprintf("%s/%s", runtime.GOOS, runtime.GOARCH)
}

func getMihomoServiceFilePath() string {
	return getSystemdServiceFilePathByName(mihomoSystemdName)
}

func (s *MihomoCoreManagerService) getLocalVersion(binPath string) (string, string) {
	commands := [][]string{
		{"-v"},
		{"version"},
	}
	for _, args := range commands {
		cmd := exec.Command(binPath, args...)
		cmd.Dir = filepath.Dir(binPath)
		output, err := cmd.CombinedOutput()
		if err != nil {
			continue
		}
		outputStr := strings.TrimSpace(string(output))
		if outputStr == "" {
			continue
		}
		if match := mihomoVersionRe.FindString(outputStr); match != "" {
			return match, outputStr
		}
		return "", outputStr
	}
	return "", ""
}

func (s *MihomoCoreManagerService) GetCoreStatus() (*CoreInfo, error) {
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
		logger.Warning("failed to load mihomo download preference: ", err)
	}

	binPath := s.getCoreBinPath()
	if statInfo, err := os.Stat(binPath); err == nil {
		if version, versionInfo, ok := getMihomoLocalVersionCache(binPath, statInfo.ModTime(), statInfo.Size()); ok {
			info.LocalVersion = version
			info.VersionInfo = versionInfo
		} else {
			version, versionInfo := s.getLocalVersion(binPath)
			setMihomoLocalVersionCache(binPath, statInfo.ModTime(), statInfo.Size(), version, versionInfo)
			info.LocalVersion = version
			info.VersionInfo = versionInfo
		}
		installedTarget := inferTargetFromGoBuildInfo(binPath)
		if installedTarget.OS == "" && installedTarget.Arch == "" {
			installedTarget = inferTargetFromPlatform(info.Platform)
		}
		if installedTarget.Arch == "amd64" {
			if level := inferMihomoAmd64LevelFromVersionInfo(info.VersionInfo); level != "" {
				installedTarget.Amd64Level = level
			}
		}
		info.InstalledTarget = mergeInstalledTargetWithPreference(installedTarget, info.DownloadPreference.Target)
	} else {
		clearMihomoLocalVersionCache(binPath)
	}

	info.Running = s.isRunning()
	return info, nil
}

func (s *MihomoCoreManagerService) fetchGitHubReleasePage(client *http.Client, apiPage int, perPage int) ([]GitHubRelease, error) {
	return fetchGitHubReleasePageForRepo("MetaCubeX/mihomo", client, apiPage, perPage)
}

func (s *MihomoCoreManagerService) GetRemoteVersionsPage(channel string, page int, perPage int) (*VersionListResponse, error) {
	if page < 1 {
		page = 1
	}
	if perPage <= 0 {
		perPage = 5
	}
	offset := (page - 1) * perPage
	return s.GetRemoteVersionsWindow(channel, offset, perPage, CoreDownloadTarget{})
}

func pickMihomoAssetFromAssets(assets []GitHubAsset, target CoreDownloadTarget) (GitHubAsset, bool) {
	normalizedTarget := (&MihomoCoreManagerService{}).normalizeDownloadTarget(target)
	preferredExts := []string{".gz", ".tar.gz", ".tgz", ".tar.xz", ".txz", ".zip"}
	if normalizedTarget.OS == "windows" {
		preferredExts = []string{".zip", ".tar.gz", ".tgz", ".gz", ".tar.xz", ".txz"}
	}

	type scoredAsset struct {
		asset GitHubAsset
		score int
	}

	candidates := make([]scoredAsset, 0, len(assets))
	for _, asset := range assets {
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

func (s *MihomoCoreManagerService) GetRemoteVersionsWindow(channel string, offset int, limit int, target CoreDownloadTarget) (*VersionListResponse, error) {
	channel = "stable"
	offset, limit = normalizeCoreVersionWindow(offset, limit)
	filterTarget := hasCoreDownloadTargetFilter(target)
	if filterTarget {
		target = s.normalizeDownloadTarget(target)
	}

	cacheKey := coreVersionCacheKey("MetaCubeX/mihomo", channel, offset, limit, target, filterTarget)
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

func (s *MihomoCoreManagerService) getArchName() string {
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

func (s *MihomoCoreManagerService) scoreAssetName(name string, preferredExts []string, target CoreDownloadTarget) int {
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
		level := normalizeAmd64Level(target.Amd64Level)
		if level == "" {
			level = "v3"
		}
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
		default: // v3
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

func (s *MihomoCoreManagerService) normalizeDownloadTarget(target CoreDownloadTarget) CoreDownloadTarget {
	normalized := CoreDownloadTarget{
		OS:         strings.ToLower(strings.TrimSpace(target.OS)),
		Arch:       strings.ToLower(strings.TrimSpace(target.Arch)),
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
	return normalized
}

func (s *MihomoCoreManagerService) pickDownloadAsset(release *GitHubRelease, target CoreDownloadTarget) (string, error) {
	if release == nil {
		return "", fmt.Errorf("release is nil")
	}

	if asset, ok := pickMihomoAssetFromAssets(release.Assets, target); ok {
		return asset.BrowserDownloadURL, nil
	}
	return "", fmt.Errorf("no suitable mihomo asset found for %s", describeCoreDownloadTarget(target))
}

func (s *MihomoCoreManagerService) getDownloadAsset(version string, target CoreDownloadTarget) (string, error) {
	apiURL := fmt.Sprintf("https://api.github.com/repos/MetaCubeX/mihomo/releases/tags/%s", version)

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

	return s.pickDownloadAsset(&release, s.normalizeDownloadTarget(target))
}

func (s *MihomoCoreManagerService) installCoreFromArchiveFile(archivePath, coreDir string) error {
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
		return err
	}
	if runtime.GOOS != "windows" {
		_ = os.Chmod(binPath, 0o755)
	}
	if _, statErr := os.Stat(binPath); statErr != nil {
		return fmt.Errorf("core binary not found after extraction")
	}
	return nil
}

func (s *MihomoCoreManagerService) DownloadCore(version string, target CoreDownloadTarget, requestedSessionID string) (string, error) {
	s.mu.Lock()
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
	if wasRunning {
		SetCoreDownloadProgressStage(sessionID, coreDownloadStageStopping)
		if err := s.stopCoreInternal(); err != nil {
			failProgress(coreDownloadStageStopping, err)
			return "", err
		}
	}

	normalizedTarget := s.normalizeDownloadTarget(target)
	if strings.TrimSpace(target.OS) != "" && normalizedTarget.OS != runtime.GOOS {
		err := fmt.Errorf("requested mihomo target %s cannot be installed on runtime %s/%s", describeCoreDownloadTarget(normalizedTarget), runtime.GOOS, runtime.GOARCH)
		failProgress(coreDownloadStageDownloading, err)
		return "", err
	}
	if strings.TrimSpace(target.Arch) != "" && normalizedTarget.Arch != s.getArchName() {
		err := fmt.Errorf("requested mihomo target %s does not match runtime architecture %s", describeCoreDownloadTarget(normalizedTarget), s.getArchName())
		failProgress(coreDownloadStageDownloading, err)
		return "", err
	}

	downloadURL, err := s.getDownloadAsset(version, normalizedTarget)
	if err != nil {
		failProgress(coreDownloadStageDownloading, err)
		return "", err
	}
	SetCoreDownloadProgressStage(sessionID, coreDownloadStageDownloading)

	client := &http.Client{Timeout: 600 * time.Second}
	resp, err := client.Get(downloadURL)
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

	if _, err = io.Copy(out, io.TeeReader(resp.Body, &coreDownloadProgressWriter{sessionID: sessionID})); err != nil {
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
	if err = s.installCoreFromArchiveFile(tmpFile, coreDir); err != nil {
		err = fmt.Errorf("extract/install failed: %v", err)
		failProgress(coreDownloadStageReplacing, err)
		return "", err
	}

	binPath := filepath.Join(coreDir, s.getCoreBinName())
	SetCoreDownloadProgressStage(sessionID, coreDownloadStageValidating)
	if !s.validateCoreBinary(binPath) {
		_ = os.Remove(binPath)
		err = fmt.Errorf("downloaded mihomo binary is not executable on current runtime %s/%s", runtime.GOOS, runtime.GOARCH)
		failProgress(coreDownloadStageValidating, err)
		return "", err
	}
	localVersion, _ := s.getLocalVersion(binPath)
	if err := s.SaveDownloadTarget(normalizedTarget); err != nil {
		logger.Warning("failed to save mihomo download preference: ", err)
	}

	if wasRunning {
		SetCoreDownloadProgressStage(sessionID, coreDownloadStageStarting)
		if err = s.startCoreLocked(); err != nil {
			err = fmt.Errorf("download completed, but auto start failed: %v", err)
			failProgress(coreDownloadStageStarting, err)
			return localVersion, err
		}
		SetCoreDownloadProgressStage(sessionID, coreDownloadStageStarted)
		time.Sleep(900 * time.Millisecond)
	}

	FinishCoreDownloadProgressSuccess(sessionID, coreDownloadStageCompleted)
	return localVersion, nil
}

func (s *MihomoCoreManagerService) DownloadCoreFromURL(downloadURL string, requestedSessionID string) (string, error) {
	s.mu.Lock()
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
	if wasRunning {
		SetCoreDownloadProgressStage(sessionID, coreDownloadStageStopping)
		if err := s.stopCoreInternal(); err != nil {
			failProgress(coreDownloadStageStopping, err)
			return "", err
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
	tmpFile := filepath.Join(coreDir, "mihomo-custom-download"+ext)
	defer os.Remove(tmpFile)

	out, err := os.Create(tmpFile)
	if err != nil {
		err = fmt.Errorf("create temp file failed: %v", err)
		failProgress(coreDownloadStageDownloading, err)
		return "", err
	}

	if _, err = io.Copy(out, io.TeeReader(resp.Body, &coreDownloadProgressWriter{sessionID: sessionID})); err != nil {
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
	if err = s.installCoreFromArchiveFile(tmpFile, coreDir); err != nil {
		err = fmt.Errorf("extract/install failed: %v", err)
		failProgress(coreDownloadStageReplacing, err)
		return "", err
	}

	binPath := filepath.Join(coreDir, s.getCoreBinName())
	SetCoreDownloadProgressStage(sessionID, coreDownloadStageValidating)
	if !s.validateCoreBinary(binPath) {
		_ = os.Remove(binPath)
		err = fmt.Errorf("downloaded mihomo binary is not executable on current runtime %s/%s", runtime.GOOS, runtime.GOARCH)
		failProgress(coreDownloadStageValidating, err)
		return "", err
	}
	localVersion, _ := s.getLocalVersion(binPath)
	if wasRunning {
		SetCoreDownloadProgressStage(sessionID, coreDownloadStageStarting)
		if err = s.startCoreLocked(); err != nil {
			err = fmt.Errorf("download completed, but auto start failed: %v", err)
			failProgress(coreDownloadStageStarting, err)
			return localVersion, err
		}
		SetCoreDownloadProgressStage(sessionID, coreDownloadStageStarted)
		time.Sleep(900 * time.Millisecond)
	}
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
	_, output := s.getLocalVersion(binPath)
	return strings.Contains(strings.ToLower(output), "mihomo")
}

func (s *MihomoCoreManagerService) StartCore() error {
	s.mu.Lock()
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
		return fmt.Errorf("core binary is not compatible with current runtime %s/%s", runtime.GOOS, runtime.GOARCH)
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
		return fmt.Errorf("core binary is not compatible with current runtime %s/%s", runtime.GOOS, runtime.GOARCH)
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
		if markerErr := markManagedCoreShouldRun("mihomo"); markerErr != nil {
			logger.Warning("failed to persist mihomo runtime marker: ", markerErr)
		}
	}
	return nil
}

func (s *MihomoCoreManagerService) StopCore() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if runtime.GOOS == "linux" {
		err := s.stopCoreLinuxFull()
		if err == nil {
			clearManagedCoreShouldRun("mihomo")
		}
		return err
	}
	return s.stopCoreInternal()
}

func (s *MihomoCoreManagerService) DeleteCore() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := EnsureManagedCoreLayout(); err != nil {
		return err
	}

	if runtime.GOOS == "linux" {
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
	return nil
}

func (s *MihomoCoreManagerService) RestartCore() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := EnsureManagedCoreLayout(); err != nil {
		return err
	}

	if runtime.GOOS == "linux" && !shouldUseDirectManagedCoreRuntime() && s.isMihomoSystemdActive() {
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
		cmd := exec.Command("systemctl", "restart", mihomoSystemdName)
		if err := cmd.Run(); err != nil {
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
	if runtime.GOOS == "linux" {
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
	if runtime.GOOS == "linux" && !shouldUseDirectManagedCoreRuntime() {
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
	if runtime.GOOS == "linux" && shouldUseDirectManagedCoreRuntime() && isManagedCoreProcessRunningByBinaryPath(s.getCoreBinPath()) {
		s.isStarted = true
		return true
	}
	return false
}

func (s *MihomoCoreManagerService) isMihomoSystemdActive() bool {
	cmd := exec.Command("systemctl", "is-active", "--quiet", mihomoSystemdName)
	return cmd.Run() == nil
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

	_ = exec.Command("systemctl", "reset-failed", mihomoSystemdName).Run()
	startCmd := exec.Command("systemctl", "start", mihomoSystemdName)
	startOutput, startErr := startCmd.CombinedOutput()
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
	if runtime.GOOS == "linux" && !shouldUseDirectManagedCoreRuntime() && s.isMihomoSystemdActive() {
		cmd := exec.Command("systemctl", "stop", mihomoSystemdName)
		if err := cmd.Run(); err == nil {
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

	if runtime.GOOS == "linux" && shouldUseDirectManagedCoreRuntime() {
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
		if runtime.GOOS == "windows" {
			if err := exec.Command("taskkill", "/F", "/T", "/PID", fmt.Sprintf("%d", pid)).Run(); err != nil {
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
	useSystemctl := runtime.GOOS == "linux" && !shouldUseDirectManagedCoreRuntime()
	if useSystemctl {
		_ = exec.Command("systemctl", "stop", serviceName).Run()
		_ = exec.Command("systemctl", "disable", serviceName).Run()
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
			_ = exec.Command("systemctl", "daemon-reload").Run()
			_ = exec.Command("systemctl", "reset-failed").Run()
		}
	}
}

func (s *MihomoCoreManagerService) createMihomoSystemdService(binPath, configPath, workDir string) error {
	controlPath := getSystemdControlBinaryPath()
	serviceContent := buildMihomoSystemdServiceContent(controlPath, binPath, configPath, workDir)

	servicePath := getMihomoServiceFilePath()
	if err := os.WriteFile(servicePath, []byte(serviceContent), 0o644); err != nil {
		return fmt.Errorf("unable to write systemd service file %s: %v", servicePath, err)
	}
	if err := verifySystemdUnitFile(servicePath); err != nil {
		return err
	}
	if err := exec.Command("systemctl", "daemon-reload").Run(); err != nil {
		return fmt.Errorf("systemctl daemon-reload failed: %v", err)
	}
	return nil
}

func buildMihomoSystemdServiceContent(controlPath, binPath, configPath, workDir string) string {
	return fmt.Sprintf(`[Unit]
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
Restart=no
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
	useSystemctl := runtime.GOOS == "linux" && !shouldUseDirectManagedCoreRuntime()
	if useSystemctl {
		_ = exec.Command("systemctl", "disable", mihomoSystemdName).Run()
	}
	if err := os.Remove(servicePath); err != nil {
		logger.Warning("unable to remove systemd service file: ", err)
		return
	}
	if useSystemctl {
		_ = exec.Command("systemctl", "daemon-reload").Run()
		_ = exec.Command("systemctl", "reset-failed").Run()
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

func (s *MihomoCoreManagerService) buildCoreUpdateInfo() (*CoreUpdateInfo, error) {
	enabled, intervalHours, lastCheckedAt, err := s.getCoreAutoCheckSettings()
	if err != nil {
		return nil, err
	}

	settingSvc := &SettingService{}
	latestStable, err := settingSvc.getString(mihomoCoreAutoCheckLatestStableKey)
	if err != nil {
		return nil, err
	}
	pendingStable, err := settingSvc.getString(mihomoCoreAutoCheckPendingStableKey)
	if err != nil {
		return nil, err
	}

	info := &CoreUpdateInfo{
		Enabled:       enabled,
		IntervalHours: intervalHours,
		LastCheckedAt: lastCheckedAt,
		LatestStable:  latestStable,
		LatestAlpha:   "",
		PendingStable: pendingStable,
		PendingAlpha:  "",
	}
	if pendingStable != "" {
		info.UpdateCount++
	}
	info.HasUpdate = info.UpdateCount > 0
	return info, nil
}

func (s *MihomoCoreManagerService) SetCoreAutoCheckSettings(enabled bool, intervalHours int) error {
	if intervalHours <= 0 {
		intervalHours = 12
	}

	mihomoCoreAutoCheckMu.Lock()
	defer mihomoCoreAutoCheckMu.Unlock()

	settingSvc := &SettingService{}
	if err := settingSvc.setString(mihomoCoreAutoCheckEnabledKey, strconv.FormatBool(enabled)); err != nil {
		return err
	}
	if err := settingSvc.setString(mihomoCoreAutoCheckIntervalHoursKey, strconv.Itoa(intervalHours)); err != nil {
		return err
	}
	if err := settingSvc.setString(mihomoCoreAutoCheckPendingAlphaKey, ""); err != nil {
		return err
	}
	if err := settingSvc.setString(mihomoCoreAutoCheckLatestAlphaKey, ""); err != nil {
		return err
	}
	if !enabled {
		if err := settingSvc.setString(mihomoCoreAutoCheckPendingStableKey, ""); err != nil {
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
	if err := settingSvc.setString(mihomoCoreAutoCheckLatestAlphaKey, ""); err != nil {
		return err
	}
	return nil
}

func (s *MihomoCoreManagerService) GetCoreUpdateInfo(forceCheck bool) (*CoreUpdateInfo, error) {
	if forceCheck {
		if err := s.CheckAndMarkCoreUpdates(true); err != nil {
			logger.Warning("check mihomo core updates failed: ", err)
		}
	}
	return s.buildCoreUpdateInfo()
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
	if err = json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", fmt.Errorf("failed to parse latest stable release: %v", err)
	}
	return strings.TrimSpace(release.TagName), nil
}

func (s *MihomoCoreManagerService) fetchLatestAlphaTag(client *http.Client) (string, error) {
	const (
		perPage  = coreReleaseGitHubPerPage
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
	settingSvc := &SettingService{}
	prevStable, err := settingSvc.getString(mihomoCoreAutoCheckLatestStableKey)
	if err != nil {
		return err
	}

	if err = settingSvc.setString(mihomoCoreAutoCheckLastAtKey, strconv.FormatInt(now, 10)); err != nil {
		return err
	}
	if latestStable != "" && latestStable != prevStable {
		if err = settingSvc.setString(mihomoCoreAutoCheckLatestStableKey, latestStable); err != nil {
			return err
		}
		if err = settingSvc.setString(mihomoCoreAutoCheckPendingStableKey, latestStable); err != nil {
			return err
		}
	}
	if err = settingSvc.setString(mihomoCoreAutoCheckLatestAlphaKey, ""); err != nil {
		return err
	}
	if err = settingSvc.setString(mihomoCoreAutoCheckPendingAlphaKey, ""); err != nil {
		return err
	}
	return nil
}
