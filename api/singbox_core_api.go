package api

import (
	"fmt"
	"strings"

	"github.com/alireza0/s-ui/logger"
	"github.com/alireza0/s-ui/service"
	"github.com/gin-gonic/gin"
)

type singboxCoreVersionRequest struct {
	Channel string
	Offset  int
	Limit   int
	Target  service.SingboxCoreDownloadTarget
}

type singboxCoreDownloadRequest struct {
	Version           string
	CustomURL         string
	DownloadSessionID string
	Target            service.SingboxCoreDownloadTarget
}

type singboxCorePreferenceRequest struct {
	CustomURL string
	Target    service.SingboxCoreDownloadTarget
	HasOS     bool
	HasArch   bool
	HasLibc   bool
}

func parseSingboxCoreVersionWindowQuery(c *gin.Context) singboxCoreVersionRequest {
	channel, offset, limit := parseCoreVersionWindowPagination(c)
	return singboxCoreVersionRequest{
		Channel: channel,
		Offset:  offset,
		Limit:   limit,
		Target: service.SingboxCoreDownloadTarget{
			OS:   strings.TrimSpace(c.Query("target_os")),
			Arch: strings.TrimSpace(c.Query("target_arch")),
			Libc: strings.TrimSpace(c.Query("target_libc")),
		},
	}
}

func parseSingboxCoreDownloadRequest(c *gin.Context) singboxCoreDownloadRequest {
	return singboxCoreDownloadRequest{
		Version:           strings.TrimSpace(c.Request.FormValue("version")),
		CustomURL:         strings.TrimSpace(c.Request.FormValue("custom_url")),
		DownloadSessionID: strings.TrimSpace(c.Request.FormValue("downloadSessionId")),
		Target: service.SingboxCoreDownloadTarget{
			OS:   strings.TrimSpace(c.Request.FormValue("target_os")),
			Arch: strings.TrimSpace(c.Request.FormValue("target_arch")),
			Libc: strings.TrimSpace(c.Request.FormValue("target_libc")),
		},
	}
}

func parseSingboxCorePreferenceRequest(c *gin.Context) singboxCorePreferenceRequest {
	target := service.SingboxCoreDownloadTarget{
		OS:   strings.TrimSpace(c.Request.FormValue("target_os")),
		Arch: strings.TrimSpace(c.Request.FormValue("target_arch")),
		Libc: strings.TrimSpace(c.Request.FormValue("target_libc")),
	}
	_, hasOS := c.Request.Form["target_os"]
	_, hasArch := c.Request.Form["target_arch"]
	_, hasLibc := c.Request.Form["target_libc"]
	return singboxCorePreferenceRequest{
		CustomURL: strings.TrimSpace(c.Request.FormValue("custom_url")),
		Target:    target,
		HasOS:     hasOS,
		HasArch:   hasArch,
		HasLibc:   hasLibc,
	}
}

func (a *ApiService) GetCoreManagerStatus(c *gin.Context) {
	info, err := a.coreManagerService().GetCoreStatus()
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, info, nil)
}

func (a *ApiService) GetCoreRemoteVersions(c *gin.Context) {
	request := parseSingboxCoreVersionWindowQuery(c)
	result, err := a.coreManagerService().GetRemoteVersionsWindow(request.Channel, request.Offset, request.Limit, request.Target)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, result, nil)
}

func (a *ApiService) GetCoreUpdateInfo(c *gin.Context) {
	forceCheck := strings.EqualFold(c.Query("force"), "true")
	info, err := a.coreManagerService().GetSingboxCoreUpdateInfo(forceCheck)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, info, nil)
}

func (a *ApiService) SaveCoreUpdateSettings(c *gin.Context) {
	enabledRaw := strings.TrimSpace(c.Request.FormValue("enabled"))
	enabled := strings.EqualFold(enabledRaw, "true") || enabledRaw == "1"
	intervalHours, err := parseCoreIntervalHours(c.Request.FormValue("interval"))
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	if err = a.coreManagerService().SetCoreAutoCheckSettings(enabled, intervalHours); err != nil {
		jsonMsg(c, "", err)
		return
	}
	if enabled {
		if checkErr := a.coreManagerService().CheckAndMarkCoreUpdates(true); checkErr != nil {
			logger.Warning("check core updates after settings update failed: ", checkErr)
		}
	}
	info, err := a.coreManagerService().GetSingboxCoreUpdateInfo(false)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, info, nil)
}

func (a *ApiService) AckCoreUpdateNotice(c *gin.Context) {
	if err := a.coreManagerService().ClearCoreUpdatePending(); err != nil {
		jsonMsg(c, "", err)
		return
	}
	info, err := a.coreManagerService().GetSingboxCoreUpdateInfo(false)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, info, nil)
}

func (a *ApiService) syncNftablesWithCoreState() {
	if a.coreManagerService().IsRunning() {
		(&service.NftTrafficService{}).InitOnStartup()
		(&service.ClientRateLimitService{}).InitOnStartup()
		(&service.ClientPortBlockService{}).InitOnStartup()
		return
	}
	(&service.NftTrafficService{}).CleanupOnShutdown()
	(&service.ClientRateLimitService{}).CleanupOnShutdown()
	(&service.ClientPortBlockService{}).CleanupOnShutdown()
}

func (a *ApiService) DownloadCoreManager(c *gin.Context) {
	request := parseSingboxCoreDownloadRequest(c)
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
	progress, err := a.coreManagerService().StartManagedCoreDownload(request.Version, request.Target, request.CustomURL, func() {
		a.syncNftablesWithCoreState()
	})
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, progress, nil)
}

func (a *ApiService) SaveCoreDownloadPreference(c *gin.Context) {
	request := parseSingboxCorePreferenceRequest(c)
	preference, err := a.coreManagerService().GetDownloadPreference()
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
	if request.HasLibc {
		preference.Target.Libc = request.Target.Libc
	}
	preference.CustomURL = request.CustomURL
	if err = a.coreManagerService().SaveDownloadPreference(preference); err != nil {
		jsonMsg(c, "", err)
		return
	}
	preference, err = a.coreManagerService().GetDownloadPreference()
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, preference, nil)
}

func (a *ApiService) StartCoreManager(c *gin.Context) {
	err := service.WithCertificateCoreConfigGate(a.coreManagerService().StartCore)
	if err == nil {
		a.syncNftablesWithCoreState()
	}
	jsonMsg(c, "startCore", err)
}

func (a *ApiService) StopCoreManager(c *gin.Context) {
	err := a.coreManagerService().StopCore()
	if err == nil {
		a.syncNftablesWithCoreState()
	}
	jsonMsg(c, "stopCore", err)
}

func (a *ApiService) RestartCoreManager(c *gin.Context) {
	err := service.WithCertificateCoreConfigGate(a.coreManagerService().RestartCore)
	if err == nil {
		a.syncNftablesWithCoreState()
	}
	jsonMsg(c, "restartCore", err)
}

func (a *ApiService) DeleteCoreManager(c *gin.Context) {
	err := a.coreManagerService().DeleteCore()
	if err == nil {
		a.syncNftablesWithCoreState()
	}
	jsonMsg(c, "deleteCore", err)
}
