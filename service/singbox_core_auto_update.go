package service

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const singboxCoreAutoUpdateRetryCount = 3

type singboxCoreAutoUpdateState struct {
	Enabled       bool
	LastAttemptAt int64
	LastSuccessAt int64
	Error         string
	ErrorAt       int64
	DisableReason string
}

func parseSingboxCoreUnixSetting(raw string) int64 {
	value, err := parseUnixSetting(raw)
	if err != nil || value < 0 {
		return 0
	}
	return value
}

func (s *CoreManagerService) getCoreAutoUpdateState() (singboxCoreAutoUpdateState, error) {
	settingSvc := &SettingService{}
	enabled, err := settingSvc.getBool(coreAutoUpdateEnabledKey)
	if err != nil {
		return singboxCoreAutoUpdateState{}, err
	}
	lastAttemptRaw, err := settingSvc.getString(coreAutoUpdateLastAttemptKey)
	if err != nil {
		return singboxCoreAutoUpdateState{}, err
	}
	lastSuccessRaw, err := settingSvc.getString(coreAutoUpdateLastSuccessKey)
	if err != nil {
		return singboxCoreAutoUpdateState{}, err
	}
	errorText, err := settingSvc.getString(coreAutoUpdateErrorKey)
	if err != nil {
		return singboxCoreAutoUpdateState{}, err
	}
	errorAtRaw, err := settingSvc.getString(coreAutoUpdateErrorAtKey)
	if err != nil {
		return singboxCoreAutoUpdateState{}, err
	}
	disableReason, err := settingSvc.getString(coreAutoUpdateDisableReasonKey)
	if err != nil {
		return singboxCoreAutoUpdateState{}, err
	}
	return singboxCoreAutoUpdateState{
		Enabled:       enabled,
		LastAttemptAt: parseSingboxCoreUnixSetting(lastAttemptRaw),
		LastSuccessAt: parseSingboxCoreUnixSetting(lastSuccessRaw),
		Error:         strings.TrimSpace(errorText),
		ErrorAt:       parseSingboxCoreUnixSetting(errorAtRaw),
		DisableReason: strings.TrimSpace(disableReason),
	}, nil
}

func (s *CoreManagerService) setCoreAutoUpdateEnabledLocked(settingSvc *SettingService, enabled bool) error {
	return settingSvc.setString(coreAutoUpdateEnabledKey, strconv.FormatBool(enabled))
}

func (s *CoreManagerService) setCoreAutoUpdateAttemptLocked(settingSvc *SettingService, ts int64) error {
	return settingSvc.setString(coreAutoUpdateLastAttemptKey, fmt.Sprintf("%d", ts))
}

func (s *CoreManagerService) setCoreAutoUpdateSuccessLocked(settingSvc *SettingService, ts int64) error {
	return settingSvc.setString(coreAutoUpdateLastSuccessKey, fmt.Sprintf("%d", ts))
}

func (s *CoreManagerService) setCoreAutoUpdateErrorLocked(settingSvc *SettingService, errText string, ts int64) error {
	if err := settingSvc.setString(coreAutoUpdateErrorKey, strings.TrimSpace(errText)); err != nil {
		return err
	}
	return settingSvc.setString(coreAutoUpdateErrorAtKey, fmt.Sprintf("%d", ts))
}

func (s *CoreManagerService) clearCoreAutoUpdateErrorLocked(settingSvc *SettingService) error {
	if err := settingSvc.setString(coreAutoUpdateErrorKey, ""); err != nil {
		return err
	}
	return settingSvc.setString(coreAutoUpdateErrorAtKey, "0")
}

func (s *CoreManagerService) setCoreAutoUpdateDisableReasonLocked(settingSvc *SettingService, reason string) error {
	return settingSvc.setString(coreAutoUpdateDisableReasonKey, strings.TrimSpace(reason))
}

func (s *CoreManagerService) clearCoreAutoUpdateDisableReasonLocked(settingSvc *SettingService) error {
	return settingSvc.setString(coreAutoUpdateDisableReasonKey, "")
}

func (s *CoreManagerService) disableCoreAutoUpdateLocked(settingSvc *SettingService, reason string) error {
	if err := s.setCoreAutoUpdateEnabledLocked(settingSvc, false); err != nil {
		return err
	}
	return s.setCoreAutoUpdateDisableReasonLocked(settingSvc, reason)
}

func (s *CoreManagerService) DisableCoreAutoUpdate(reason string) error {
	coreAutoCheckMu.Lock()
	defer coreAutoCheckMu.Unlock()

	settingSvc := &SettingService{}
	return s.disableCoreAutoUpdateLocked(settingSvc, reason)
}

func (s *CoreManagerService) ClearCoreAutoUpdateError() error {
	coreAutoCheckMu.Lock()
	defer coreAutoCheckMu.Unlock()

	settingSvc := &SettingService{}
	return s.clearCoreAutoUpdateErrorLocked(settingSvc)
}

func (s *CoreManagerService) ClearCoreAutoUpdateDisableReason() error {
	coreAutoCheckMu.Lock()
	defer coreAutoCheckMu.Unlock()

	settingSvc := &SettingService{}
	return s.clearCoreAutoUpdateDisableReasonLocked(settingSvc)
}

func singboxAutoUpdateReadinessReason(status *SingboxCoreInfo) string {
	if !IsSystemPlatformLinux() {
		return "当前平台不支持 sing-box 自动更新"
	}
	if status == nil || !status.Installed {
		return "本地未安装 sing-box 内核"
	}
	if !status.Compatible {
		return "本地内核不兼容，无法用于自动更新"
	}
	if strings.TrimSpace(status.InstalledTarget.Arch) == "" {
		return "无法识别本地内核架构"
	}
	if strings.TrimSpace(status.InstalledTarget.Libc) == "" {
		return "无法识别本地内核包类型"
	}
	if strings.TrimSpace(status.InstalledChannel) == "" {
		return "无法识别本地内核版本频道"
	}
	return ""
}

func singboxShouldDisableAutoUpdateInUI(enabled bool, status *SingboxCoreInfo) bool {
	if enabled {
		return false
	}
	return singboxAutoUpdateReadinessReason(status) != ""
}

func singboxShouldDisableAutoUpdateOnError(reason string) bool {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return false
	}
	for _, fragment := range []string{
		"当前平台不支持 sing-box 自动更新",
		"本地未安装 sing-box 内核",
		"本地内核不兼容",
		"无法识别本地内核架构",
		"无法识别本地内核包类型",
		"无法识别本地内核版本频道",
	} {
		if strings.Contains(reason, fragment) {
			return true
		}
	}
	return false
}

func singboxRemoteVersionIsNewer(remoteVersion string, localVersion string) bool {
	remoteVersion = normalizeSingboxComparableVersion(remoteVersion)
	localVersion = normalizeSingboxComparableVersion(localVersion)
	if remoteVersion == "" || localVersion == "" {
		return false
	}
	return compareSemverLikeTags(remoteVersion, localVersion) > 0
}

// normalizeSingboxComparableVersion retains prerelease identifiers but drops
// SemVer build metadata, which does not participate in version precedence.
func normalizeSingboxComparableVersion(value string) string {
	normalized := normalizeSingboxVersionTag(value)
	if buildMetadataIndex := strings.Index(normalized, "+"); buildMetadataIndex >= 0 {
		normalized = normalized[:buildMetadataIndex]
	}
	if !isLikelySingboxVersionTag(normalized) {
		return ""
	}
	return normalized
}

func selectSingboxPendingUpdateForChannel(status *SingboxCoreInfo, pendingStable string, pendingAlpha string) (string, string) {
	if status == nil || strings.TrimSpace(status.LocalVersion) == "" {
		return "", ""
	}
	switch strings.TrimSpace(status.InstalledChannel) {
	case singboxReleaseChannelStable:
		if singboxRemoteVersionIsNewer(pendingStable, status.LocalVersion) {
			return pendingStable, ""
		}
	case singboxReleaseChannelAlpha:
		if singboxRemoteVersionIsNewer(pendingAlpha, status.LocalVersion) {
			return "", pendingAlpha
		}
	}
	return "", ""
}

func (s *CoreManagerService) resolveAutoUpdateTargetVersion(status *SingboxCoreInfo) (string, error) {
	if status == nil {
		return "", fmt.Errorf("本地内核状态不可用")
	}
	client := &http.Client{Timeout: 30 * time.Second}
	switch strings.TrimSpace(status.InstalledChannel) {
	case singboxReleaseChannelStable:
		return s.fetchLatestStableTag(client)
	case singboxReleaseChannelAlpha:
		return s.fetchLatestAlphaTag(client)
	default:
		return "", fmt.Errorf("无法识别本地内核版本频道")
	}
}

func (s *CoreManagerService) getStrictDownloadAssetContext(ctx context.Context, version string, target SingboxCoreDownloadTarget) (string, error) {
	apiURL := fmt.Sprintf("https://api.github.com/repos/SagerNet/sing-box/releases/tags/%s", version)
	client := &http.Client{Timeout: 30 * time.Second}
	req, err := http.NewRequestWithContext(ctx, "GET", apiURL, nil)
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
	body, err := readBoundedHTTPResponseBody(resp.Body, coreGitHubResponseMaxBytes)
	if err != nil {
		return "", err
	}
	var release GitHubRelease
	if err := json.Unmarshal(body, &release); err != nil {
		return "", fmt.Errorf("解析 GitHub 响应失败: %v", err)
	}

	ver := strings.TrimPrefix(release.TagName, "v")
	normalizedTarget := s.normalizeDownloadTarget(target)
	candidateName := exactSingboxAssetName(ver, normalizedTarget)
	if candidateName == "" {
		return "", fmt.Errorf("无法构造 %s 的精确内核文件名", describeSingboxCoreDownloadTarget(normalizedTarget))
	}
	for _, asset := range release.Assets {
		if asset.Name == candidateName {
			return asset.BrowserDownloadURL, nil
		}
	}
	return "", fmt.Errorf("当前频道下未找到与 %s 精确匹配的 sing-box 安装包", describeSingboxCoreDownloadTarget(normalizedTarget))
}

func exactSingboxAssetName(version string, target SingboxCoreDownloadTarget) string {
	version = strings.TrimSpace(version)
	target.OS = strings.TrimSpace(target.OS)
	target.Arch = strings.TrimSpace(target.Arch)
	target.Libc = strings.TrimSpace(target.Libc)
	if version == "" || target.OS == "" || target.Arch == "" {
		return ""
	}
	ext := coreArchiveExtForOS(target.OS)
	if target.OS != "linux" {
		return fmt.Sprintf("sing-box-%s-%s-%s%s", version, target.OS, target.Arch, ext)
	}
	switch target.Libc {
	case "glibc", "musl":
		return fmt.Sprintf("sing-box-%s-linux-%s-%s%s", version, target.Arch, target.Libc, ext)
	case "universal":
		return fmt.Sprintf("sing-box-%s-linux-%s%s", version, target.Arch, ext)
	default:
		return ""
	}
}

func (s *CoreManagerService) runSingboxAutoUpdateAttempt(ctx context.Context) (bool, error) {
	status, err := s.GetCoreStatus()
	if err != nil {
		return false, err
	}
	if reason := singboxAutoUpdateReadinessReason(status); reason != "" {
		return false, fmt.Errorf("%s", reason)
	}
	targetVersion, err := s.resolveAutoUpdateTargetVersion(status)
	if err != nil {
		return false, err
	}
	if !singboxRemoteVersionIsNewer(targetVersion, status.LocalVersion) {
		return false, nil
	}
	target := SingboxCoreDownloadTarget{
		OS:   "linux",
		Arch: status.InstalledTarget.Arch,
		Libc: status.InstalledTarget.Libc,
	}
	if _, err := s.getStrictDownloadAssetContext(ctx, targetVersion, target); err != nil {
		return false, err
	}
	fingerprint := "release|" + targetVersion + "|" + target.OS + "|" + target.Arch + "|" + target.Libc
	handle, _, created, err := singboxCoreDownloadTaskManager.Start("singbox-core-download", fingerprint)
	if err != nil {
		return false, err
	}
	if !created {
		return false, fmt.Errorf("当前已有 sing-box 下载任务在运行")
	}
	if err := s.executeManagedSingboxCoreDownloadTask(handle, targetVersion, target, "", nil); err != nil {
		return false, err
	}
	return true, nil
}

func (s *CoreManagerService) RunScheduledAutoUpdate() error {
	coreAutoCheckMu.Lock()
	state, err := s.getCoreAutoUpdateState()
	coreAutoCheckMu.Unlock()
	if err != nil {
		return err
	}
	if !state.Enabled {
		return nil
	}

	now := PanelNow().Unix()
	settingSvc := &SettingService{}
	coreAutoCheckMu.Lock()
	if err := s.setCoreAutoUpdateAttemptLocked(settingSvc, now); err != nil {
		coreAutoCheckMu.Unlock()
		return err
	}
	coreAutoCheckMu.Unlock()

	var (
		lastErr error
		updated bool
	)
	for attempt := 0; attempt < singboxCoreAutoUpdateRetryCount; attempt++ {
		updated, lastErr = s.runSingboxAutoUpdateAttempt(context.Background())
		if lastErr == nil {
			coreAutoCheckMu.Lock()
			if err := s.clearCoreAutoUpdateErrorLocked(settingSvc); err != nil {
				coreAutoCheckMu.Unlock()
				return err
			}
			if err := s.clearCoreAutoUpdateDisableReasonLocked(settingSvc); err != nil {
				coreAutoCheckMu.Unlock()
				return err
			}
			if updated {
				if err := s.setCoreAutoUpdateSuccessLocked(settingSvc, now); err != nil {
					coreAutoCheckMu.Unlock()
					return err
				}
			}
			coreAutoCheckMu.Unlock()
			return nil
		}
	}

	if lastErr == nil {
		return nil
	}
	reasonText := strings.TrimSpace(lastErr.Error())
	shouldDisable := singboxShouldDisableAutoUpdateOnError(reasonText)
	coreAutoCheckMu.Lock()
	defer coreAutoCheckMu.Unlock()
	if err := s.setCoreAutoUpdateErrorLocked(settingSvc, reasonText, now); err != nil {
		return err
	}
	if shouldDisable {
		if err := s.disableCoreAutoUpdateLocked(settingSvc, reasonText); err != nil {
			return err
		}
	}
	return lastErr
}
