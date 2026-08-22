package api

import (
	"errors"
	"fmt"
	"strings"

	"github.com/alireza0/s-ui/service"
	"github.com/gin-gonic/gin"
)

type mihomoCoreVersionRequest struct {
	Channel string
	Offset  int
	Limit   int
	Target  service.MihomoCoreDownloadTarget
}

type mihomoCoreDownloadRequest struct {
	Version           string
	CustomURL         string
	DownloadSessionID string
	Target            service.MihomoCoreDownloadTarget
}

type mihomoCorePreferenceRequest struct {
	CustomURL     string
	Target        service.MihomoCoreDownloadTarget
	HasOS         bool
	HasArch       bool
	HasAMD64Level bool
}

type mihomoCoreUpdateSettingsRequest struct {
	Action        string
	Enabled       bool
	IntervalHours int
	AutoUpdate    bool
	HasAutoUpdate bool
}

func parseMihomoCoreVersionWindowQuery(c *gin.Context) mihomoCoreVersionRequest {
	channel, offset, limit := parseCoreVersionWindowPagination(c)
	return mihomoCoreVersionRequest{
		Channel: channel,
		Offset:  offset,
		Limit:   limit,
		Target: service.MihomoCoreDownloadTarget{
			OS:         strings.TrimSpace(c.Query("target_os")),
			Arch:       strings.TrimSpace(c.Query("target_arch")),
			Amd64Level: strings.TrimSpace(c.Query("target_amd64_level")),
		},
	}
}

func parseMihomoCoreDownloadRequest(c *gin.Context) mihomoCoreDownloadRequest {
	return mihomoCoreDownloadRequest{
		Version:           strings.TrimSpace(c.Request.FormValue("version")),
		CustomURL:         strings.TrimSpace(c.Request.FormValue("custom_url")),
		DownloadSessionID: strings.TrimSpace(c.Request.FormValue("downloadSessionId")),
		Target: service.MihomoCoreDownloadTarget{
			OS:         strings.TrimSpace(c.Request.FormValue("target_os")),
			Arch:       strings.TrimSpace(c.Request.FormValue("target_arch")),
			Amd64Level: strings.TrimSpace(c.Request.FormValue("target_amd64_level")),
		},
	}
}

func parseMihomoCorePreferenceRequest(c *gin.Context) mihomoCorePreferenceRequest {
	target := service.MihomoCoreDownloadTarget{
		OS:         strings.TrimSpace(c.Request.FormValue("target_os")),
		Arch:       strings.TrimSpace(c.Request.FormValue("target_arch")),
		Amd64Level: strings.TrimSpace(c.Request.FormValue("target_amd64_level")),
	}
	_, hasOS := c.Request.Form["target_os"]
	_, hasArch := c.Request.Form["target_arch"]
	_, hasAMD64Level := c.Request.Form["target_amd64_level"]
	return mihomoCorePreferenceRequest{
		CustomURL:     strings.TrimSpace(c.Request.FormValue("custom_url")),
		Target:        target,
		HasOS:         hasOS,
		HasArch:       hasArch,
		HasAMD64Level: hasAMD64Level,
	}
}

func parseMihomoCoreUpdateSettingsRequest(c *gin.Context) (mihomoCoreUpdateSettingsRequest, error) {
	action := strings.ToLower(strings.TrimSpace(c.Request.FormValue("action")))
	enabledRaw := strings.TrimSpace(c.Request.FormValue("enabled"))
	enabled := strings.EqualFold(enabledRaw, "true") || enabledRaw == "1"
	autoUpdateRaw := strings.TrimSpace(c.Request.FormValue("auto_update_enabled"))
	_, hasAutoUpdate := c.Request.Form["auto_update_enabled"]
	autoUpdateEnabled := strings.EqualFold(autoUpdateRaw, "true") || autoUpdateRaw == "1"
	request := mihomoCoreUpdateSettingsRequest{
		Action:        action,
		Enabled:       enabled,
		AutoUpdate:    autoUpdateEnabled,
		HasAutoUpdate: hasAutoUpdate,
	}

	switch action {
	case "auto_check":
		if _, exists := c.Request.Form["enabled"]; !exists {
			return mihomoCoreUpdateSettingsRequest{}, fmt.Errorf("enabled is required for auto_check")
		}
		return request, nil
	case "auto_update":
		if !hasAutoUpdate {
			return mihomoCoreUpdateSettingsRequest{}, fmt.Errorf("auto_update_enabled is required for auto_update")
		}
		return request, nil
	case "interval", "":
		intervalHours, err := parseCoreIntervalHours(c.Request.FormValue("interval"))
		if err != nil {
			return mihomoCoreUpdateSettingsRequest{}, err
		}
		request.IntervalHours = intervalHours
		return request, nil
	default:
		return mihomoCoreUpdateSettingsRequest{}, fmt.Errorf("unsupported core update settings action: %s", action)
	}
}

func (a *ApiService) GetMihomoCoreManagerStatus(c *gin.Context) {
	info, err := a.mihomoCoreManagerService().GetCoreStatus()
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, info, nil)
}

func (a *ApiService) GetMihomoCoreRemoteVersions(c *gin.Context) {
	request := parseMihomoCoreVersionWindowQuery(c)
	result, err := a.mihomoCoreManagerService().GetRemoteVersionsWindow(request.Channel, request.Offset, request.Limit, request.Target)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, result, nil)
}

func (a *ApiService) GetMihomoCoreUpdateInfo(c *gin.Context) {
	forceCheck := strings.EqualFold(c.Query("force"), "true")
	info, err := a.mihomoCoreManagerService().GetMihomoCoreUpdateInfo(forceCheck)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, info, nil)
}

func (a *ApiService) SaveMihomoCoreUpdateSettings(c *gin.Context) {
	request, err := parseMihomoCoreUpdateSettingsRequest(c)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	manager := a.mihomoCoreManagerService()
	switch request.Action {
	case "auto_check":
		err = manager.SetCoreAutoCheckEnabled(request.Enabled)
	case "auto_update":
		err = manager.SetCoreAutoUpdateEnabled(request.AutoUpdate)
	case "interval":
		err = manager.SetCoreAutoCheckInterval(request.IntervalHours)
	default:
		err = manager.SetCoreAutoCheckSettings(request.Enabled, request.IntervalHours, request.HasAutoUpdate, request.AutoUpdate)
	}
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	info, err := manager.GetMihomoCoreUpdateInfo(false)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, info, nil)
}

func (a *ApiService) AckMihomoCoreAutoUpdateError(c *gin.Context) {
	if err := a.mihomoCoreManagerService().ClearCoreAutoUpdateError(); err != nil {
		jsonMsg(c, "", err)
		return
	}
	info, err := a.mihomoCoreManagerService().GetMihomoCoreUpdateInfo(false)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, info, nil)
}

func (a *ApiService) AckMihomoCoreUpdateNotice(c *gin.Context) {
	if err := a.mihomoCoreManagerService().ClearCoreUpdatePending(); err != nil {
		jsonMsg(c, "", err)
		return
	}
	info, err := a.mihomoCoreManagerService().GetMihomoCoreUpdateInfo(false)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, info, nil)
}

func (a *ApiService) syncMihomoNftablesWithCoreState() {
	if a.mihomoCoreManagerService().IsRunning() {
		(&service.MihomoNftTrafficService{}).InitOnStartup()
		(&service.MihomoClientRateLimitService{}).InitOnStartup()
		(&service.MihomoClientPortBlockService{}).InitOnStartup()
		return
	}
	(&service.MihomoNftTrafficService{}).CleanupOnShutdown()
	(&service.MihomoClientRateLimitService{}).CleanupOnShutdown()
	(&service.MihomoClientPortBlockService{}).CleanupOnShutdown()
}

func (a *ApiService) DownloadMihomoCoreManager(c *gin.Context) {
	request := parseMihomoCoreDownloadRequest(c)
	if request.CustomURL != "" {
		if !strings.HasPrefix(request.CustomURL, "http://") && !strings.HasPrefix(request.CustomURL, "https://") {
			jsonMsg(c, "", fmt.Errorf("custom_url must start with http:// or https://"))
			return
		}
	}
	if request.CustomURL == "" && request.Version == "" {
		jsonMsg(c, "", fmt.Errorf("version or custom_url is required"))
		return
	}
	progress, err := a.mihomoCoreManagerService().StartManagedCoreDownload(request.Version, request.Target, request.CustomURL, func() {
		a.syncMihomoNftablesWithCoreState()
	})
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, progress, nil)
}

func (a *ApiService) SaveMihomoCoreDownloadPreference(c *gin.Context) {
	request := parseMihomoCorePreferenceRequest(c)
	preference, err := a.mihomoCoreManagerService().GetDownloadPreference()
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	if request.HasOS {
		preference.Target.OS = request.Target.OS
	}
	if request.HasArch {
		preference.Target.Arch = request.Target.Arch
	}
	if request.HasAMD64Level {
		preference.Target.Amd64Level = request.Target.Amd64Level
	}
	preference.CustomURL = request.CustomURL
	if err = a.mihomoCoreManagerService().SaveDownloadPreference(preference); err != nil {
		jsonMsg(c, "", err)
		return
	}
	preference, err = a.mihomoCoreManagerService().GetDownloadPreference()
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, preference, nil)
}

func (a *ApiService) SaveMihomoCoreLogLevel(c *gin.Context, loginUser string) {
	result, err := service.SaveMihomoCoreLogLevel(c.Request.FormValue("level"), loginUser)
	if err != nil {
		var committedErr *service.CommittedSaveError
		if errors.As(err, &committedErr) {
			writeCommittedSaveFailure(c, committedErr)
			return
		}
		jsonMsg(c, "save mihomo core log level", err)
		return
	}
	jsonObj(c, result, nil)
}

func (a *ApiService) StartMihomoCoreManager(c *gin.Context) {
	err := service.WithCertificateCoreConfigGate(a.mihomoCoreManagerService().StartCore)
	if err == nil {
		a.syncMihomoNftablesWithCoreState()
	}
	jsonMsg(c, "startCore", err)
}

func (a *ApiService) StopMihomoCoreManager(c *gin.Context) {
	err := a.mihomoCoreManagerService().StopCore()
	if err == nil {
		a.syncMihomoNftablesWithCoreState()
	}
	jsonMsg(c, "stopCore", err)
}

func (a *ApiService) RestartMihomoCoreManager(c *gin.Context) {
	err := service.WithCertificateCoreConfigGate(a.mihomoCoreManagerService().RestartCore)
	if err == nil {
		a.syncMihomoNftablesWithCoreState()
	}
	jsonMsg(c, "restartCore", err)
}

func (a *ApiService) DeleteMihomoCoreManager(c *gin.Context) {
	err := a.mihomoCoreManagerService().DeleteCore()
	if err == nil {
		a.syncMihomoNftablesWithCoreState()
	}
	jsonMsg(c, "deleteCore", err)
}
