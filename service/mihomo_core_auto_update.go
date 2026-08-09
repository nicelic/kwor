package service

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

const mihomoCoreAutoUpdateRetryCount = 3

type mihomoCoreAutoUpdateState struct {
	Enabled       bool
	LastAttemptAt int64
	LastSuccessAt int64
	Error         string
	ErrorAt       int64
	DisableReason string
}

func parseMihomoCoreUnixSetting(raw string) int64 {
	value, err := parseUnixSetting(raw)
	if err != nil || value < 0 {
		return 0
	}
	return value
}

func (s *MihomoCoreManagerService) getCoreAutoUpdateState() (mihomoCoreAutoUpdateState, error) {
	settingSvc := &SettingService{}
	enabled, err := settingSvc.getBool(mihomoCoreAutoUpdateEnabledKey)
	if err != nil {
		return mihomoCoreAutoUpdateState{}, err
	}
	lastAttemptRaw, err := settingSvc.getString(mihomoCoreAutoUpdateLastAttemptKey)
	if err != nil {
		return mihomoCoreAutoUpdateState{}, err
	}
	lastSuccessRaw, err := settingSvc.getString(mihomoCoreAutoUpdateLastSuccessKey)
	if err != nil {
		return mihomoCoreAutoUpdateState{}, err
	}
	errorText, err := settingSvc.getString(mihomoCoreAutoUpdateErrorKey)
	if err != nil {
		return mihomoCoreAutoUpdateState{}, err
	}
	errorAtRaw, err := settingSvc.getString(mihomoCoreAutoUpdateErrorAtKey)
	if err != nil {
		return mihomoCoreAutoUpdateState{}, err
	}
	disableReason, err := settingSvc.getString(mihomoCoreAutoUpdateDisableReasonKey)
	if err != nil {
		return mihomoCoreAutoUpdateState{}, err
	}
	return mihomoCoreAutoUpdateState{
		Enabled:       enabled,
		LastAttemptAt: parseMihomoCoreUnixSetting(lastAttemptRaw),
		LastSuccessAt: parseMihomoCoreUnixSetting(lastSuccessRaw),
		Error:         strings.TrimSpace(errorText),
		ErrorAt:       parseMihomoCoreUnixSetting(errorAtRaw),
		DisableReason: strings.TrimSpace(disableReason),
	}, nil
}

func (s *MihomoCoreManagerService) setCoreAutoUpdateEnabledLocked(settingSvc *SettingService, enabled bool) error {
	return settingSvc.setString(mihomoCoreAutoUpdateEnabledKey, strconv.FormatBool(enabled))
}

func (s *MihomoCoreManagerService) setCoreAutoUpdateAttemptLocked(settingSvc *SettingService, ts int64) error {
	return settingSvc.setString(mihomoCoreAutoUpdateLastAttemptKey, fmt.Sprintf("%d", ts))
}

func (s *MihomoCoreManagerService) setCoreAutoUpdateSuccessLocked(settingSvc *SettingService, ts int64) error {
	return settingSvc.setString(mihomoCoreAutoUpdateLastSuccessKey, fmt.Sprintf("%d", ts))
}

func (s *MihomoCoreManagerService) setCoreAutoUpdateErrorLocked(settingSvc *SettingService, errText string, ts int64) error {
	if err := settingSvc.setString(mihomoCoreAutoUpdateErrorKey, strings.TrimSpace(errText)); err != nil {
		return err
	}
	return settingSvc.setString(mihomoCoreAutoUpdateErrorAtKey, fmt.Sprintf("%d", ts))
}

func (s *MihomoCoreManagerService) clearCoreAutoUpdateErrorLocked(settingSvc *SettingService) error {
	if err := settingSvc.setString(mihomoCoreAutoUpdateErrorKey, ""); err != nil {
		return err
	}
	return settingSvc.setString(mihomoCoreAutoUpdateErrorAtKey, "0")
}

func (s *MihomoCoreManagerService) setCoreAutoUpdateDisableReasonLocked(settingSvc *SettingService, reason string) error {
	return settingSvc.setString(mihomoCoreAutoUpdateDisableReasonKey, strings.TrimSpace(reason))
}

func (s *MihomoCoreManagerService) clearCoreAutoUpdateDisableReasonLocked(settingSvc *SettingService) error {
	return settingSvc.setString(mihomoCoreAutoUpdateDisableReasonKey, "")
}

func (s *MihomoCoreManagerService) disableCoreAutoUpdateLocked(settingSvc *SettingService, reason string) error {
	if err := s.setCoreAutoUpdateEnabledLocked(settingSvc, false); err != nil {
		return err
	}
	return s.setCoreAutoUpdateDisableReasonLocked(settingSvc, reason)
}

func (s *MihomoCoreManagerService) DisableCoreAutoUpdate(reason string) error {
	mihomoCoreAutoCheckMu.Lock()
	defer mihomoCoreAutoCheckMu.Unlock()

	settingSvc := &SettingService{}
	return s.disableCoreAutoUpdateLocked(settingSvc, reason)
}

func (s *MihomoCoreManagerService) ClearCoreAutoUpdateError() error {
	mihomoCoreAutoCheckMu.Lock()
	defer mihomoCoreAutoCheckMu.Unlock()

	settingSvc := &SettingService{}
	return s.clearCoreAutoUpdateErrorLocked(settingSvc)
}

func (s *MihomoCoreManagerService) ClearCoreAutoUpdateDisableReason() error {
	mihomoCoreAutoCheckMu.Lock()
	defer mihomoCoreAutoCheckMu.Unlock()

	settingSvc := &SettingService{}
	return s.clearCoreAutoUpdateDisableReasonLocked(settingSvc)
}

func mihomoAutoUpdateReadinessReason(status *MihomoCoreInfo) string {
	if !IsSystemPlatformLinux() {
		return "当前平台不支持 Mihomo 自动更新"
	}
	if status == nil || !status.Installed {
		return "本地未安装 Mihomo 内核"
	}
	if !status.Compatible {
		return "本地内核不兼容，无法用于自动更新"
	}
	if strings.TrimSpace(status.InstalledTarget.Arch) == "" {
		return "无法识别本地内核架构"
	}
	if strings.TrimSpace(status.InstalledChannel) == "" {
		return "无法识别本地内核版本频道"
	}
	if strings.TrimSpace(status.InstalledTarget.Arch) == "amd64" && strings.TrimSpace(status.InstalledTarget.Amd64Level) == "" {
		return "无法识别本地内核 AMD64 级别"
	}
	return ""
}

func mihomoShouldDisableAutoUpdateInUI(enabled bool, status *MihomoCoreInfo) bool {
	if enabled {
		return false
	}
	return mihomoAutoUpdateReadinessReason(status) != ""
}

func mihomoShouldDisableAutoUpdateOnError(reason string) bool {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return false
	}
	for _, fragment := range []string{
		"当前平台不支持 Mihomo 自动更新",
		"本地未安装 Mihomo 内核",
		"无法识别本地内核架构",
		"无法识别本地内核版本频道",
		"无法识别本地内核 AMD64 级别",
	} {
		if strings.Contains(reason, fragment) {
			return true
		}
	}
	return false
}

func mihomoRemoteVersionIsNewer(remoteVersion string, localVersion string) bool {
	remoteVersion = normalizeMihomoVersionTag(remoteVersion)
	localVersion = normalizeMihomoVersionTag(localVersion)
	if remoteVersion == "" || localVersion == "" {
		return false
	}
	return compareSemverLikeTags(remoteVersion, localVersion) > 0
}

func selectMihomoPendingUpdateForChannel(status *MihomoCoreInfo, pendingStable string, pendingAlpha string) (string, string) {
	if status == nil || strings.TrimSpace(status.LocalVersion) == "" {
		return "", ""
	}
	switch strings.TrimSpace(status.InstalledChannel) {
	case mihomoReleaseChannelStable:
		if mihomoRemoteVersionIsNewer(pendingStable, status.LocalVersion) {
			return pendingStable, ""
		}
	case mihomoReleaseChannelAlpha:
		if mihomoRemoteVersionIsNewer(pendingAlpha, status.LocalVersion) {
			return "", pendingAlpha
		}
	}
	return "", ""
}

func (s *MihomoCoreManagerService) resolveAutoUpdateTargetVersion(status *MihomoCoreInfo) (string, error) {
	if status == nil {
		return "", fmt.Errorf("本地内核状态不可用")
	}
	client := &http.Client{Timeout: 30 * time.Second}
	switch strings.TrimSpace(status.InstalledChannel) {
	case mihomoReleaseChannelStable:
		return s.fetchLatestStableTag(client)
	case mihomoReleaseChannelAlpha:
		return s.fetchLatestAlphaTag(client)
	default:
		return "", fmt.Errorf("无法识别本地内核版本频道")
	}
}

func (s *MihomoCoreManagerService) runMihomoAutoUpdateAttempt(ctx context.Context) (bool, error) {
	status, err := s.GetCoreStatus()
	if err != nil {
		return false, err
	}
	if reason := mihomoAutoUpdateReadinessReason(status); reason != "" {
		return false, fmt.Errorf("%s", reason)
	}
	targetVersion, err := s.resolveAutoUpdateTargetVersion(status)
	if err != nil {
		return false, err
	}
	if !mihomoRemoteVersionIsNewer(targetVersion, status.LocalVersion) {
		return false, nil
	}
	target := MihomoCoreDownloadTarget{
		OS:         "linux",
		Arch:       status.InstalledTarget.Arch,
		Amd64Level: status.InstalledTarget.Amd64Level,
	}
	if _, err := s.getDownloadAssetContext(ctx, targetVersion, target); err != nil {
		return false, err
	}
	fingerprint := "release|" + targetVersion + "|" + target.OS + "|" + target.Arch + "|" + target.Amd64Level
	handle, _, created, err := mihomoCoreDownloadTaskManager.Start("mihomo-core-download", fingerprint)
	if err != nil {
		return false, err
	}
	if !created {
		return false, fmt.Errorf("当前已有 Mihomo 下载任务在运行")
	}
	if err := s.executeManagedMihomoCoreDownloadTask(handle, targetVersion, target, "", nil); err != nil {
		return false, err
	}
	return true, nil
}

func (s *MihomoCoreManagerService) RunScheduledAutoUpdate() error {
	mihomoCoreAutoCheckMu.Lock()
	state, err := s.getCoreAutoUpdateState()
	mihomoCoreAutoCheckMu.Unlock()
	if err != nil {
		return err
	}
	if !state.Enabled {
		return nil
	}

	now := PanelNow().Unix()
	settingSvc := &SettingService{}
	mihomoCoreAutoCheckMu.Lock()
	if err := s.setCoreAutoUpdateAttemptLocked(settingSvc, now); err != nil {
		mihomoCoreAutoCheckMu.Unlock()
		return err
	}
	mihomoCoreAutoCheckMu.Unlock()

	var (
		lastErr error
		updated bool
	)
	for attempt := 0; attempt < mihomoCoreAutoUpdateRetryCount; attempt++ {
		updated, lastErr = s.runMihomoAutoUpdateAttempt(context.Background())
		if lastErr == nil {
			mihomoCoreAutoCheckMu.Lock()
			if err := s.clearCoreAutoUpdateErrorLocked(settingSvc); err != nil {
				mihomoCoreAutoCheckMu.Unlock()
				return err
			}
			if err := s.clearCoreAutoUpdateDisableReasonLocked(settingSvc); err != nil {
				mihomoCoreAutoCheckMu.Unlock()
				return err
			}
			if updated {
				if err := s.setCoreAutoUpdateSuccessLocked(settingSvc, now); err != nil {
					mihomoCoreAutoCheckMu.Unlock()
					return err
				}
			}
			mihomoCoreAutoCheckMu.Unlock()
			return nil
		}
	}

	if lastErr == nil {
		return nil
	}
	reasonText := strings.TrimSpace(lastErr.Error())
	shouldDisable := mihomoShouldDisableAutoUpdateOnError(reasonText)
	mihomoCoreAutoCheckMu.Lock()
	defer mihomoCoreAutoCheckMu.Unlock()
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
