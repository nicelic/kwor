package api

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/logger"
	"github.com/alireza0/s-ui/service"
	"github.com/alireza0/s-ui/util"

	"github.com/gin-gonic/gin"
)

type ApiService struct {
	service.SettingService
	service.UserService
	service.ConfigService
	service.ClientService
	service.TlsService
	service.InboundService
	service.OutboundService
	service.MihomoConfigService
	service.MihomoClientService
	service.MihomoTlsService
	service.MihomoInboundService
	service.MihomoOutboundService
	service.MihomoOutboundGroupService
	service.OutboundGroupService
	service.SubOutboundService
	service.SubGroupService
	service.EndpointService
	service.ServicesService
	service.PanelService
	service.StatsService
	service.ServerService
	service.SystemMonitorService
	service.SyncService
	service.MihomoSyncService
	service.IPDetectService
	service.PortCheckService
	service.CoreManagerService
	service.MihomoCoreManagerService
	service.TrafficOverviewService
	service.FirewallService
	service.PortForwardService
	service.ReverseProxyService
	service.KernelManagerService
	service.SystemLogOptimizationService
	service.SystemSysctlOptimizationService
	service.SystemLinuxDNSOptimizationService
	service.SystemMTUOptimizationService
	service.AcmeService
	service.CertificateInventoryService
	service.SelfSignedService
	service.PanelUpdateService
}

type tlsSha256Request struct {
	SourceType      string `json:"source_type" form:"source_type"`
	CertificatePath string `json:"certificate_path" form:"certificate_path"`
	CertificatePEM  string `json:"certificate_pem" form:"certificate_pem"`
}

type trafficOverviewSettingsRequest struct {
	LimitGiB       *float64 `json:"limit_gib" form:"limit_gib"`
	ResetDay       *int     `json:"reset_day" form:"reset_day"`
	LimitGiBCompat *float64 `json:"limitGiB" form:"limitGiB"`
	ResetDayCompat *int     `json:"resetDay" form:"resetDay"`
}

type trafficOverviewSwitchRequest struct {
	Enabled *bool `json:"enabled" form:"enabled"`
}

type trafficOverviewVnstatInstallRequest struct {
	Source string `json:"source" form:"source"`
}

type trafficOverviewVnstatUpdateRequest struct {
	Source string `json:"source" form:"source"`
}

type systemMonitorSettingsRequest struct {
	SampleIntervalSec     *int `json:"sample_interval_sec" form:"sample_interval_sec"`
	PrimaryRetentionHours *int `json:"primary_retention_hours" form:"primary_retention_hours"`
	ArchiveRetentionDays  *int `json:"archive_retention_days" form:"archive_retention_days"`
}

type firewallSwitchRequest struct {
	Enabled *bool `json:"enabled" form:"enabled"`
}

type firewallRuleDeleteRequest struct {
	ID *uint `json:"id" form:"id"`
}

type firewallGeoSettingsRequest struct {
	IntervalMinutes *int `json:"intervalMinutes" form:"intervalMinutes"`
}

type reverseProxyReorderRequest struct {
	IDs []uint `json:"ids" form:"ids"`
}

type firewallSSHPortRequest struct {
	Port *int `json:"port" form:"port"`
}

type firewallSSHProxyRequest struct {
	Enabled *bool `json:"enabled" form:"enabled"`
}

type firewallSystemRuleRequest struct {
	SystemKey *string `json:"systemKey" form:"systemKey"`
	Enabled   *bool   `json:"enabled" form:"enabled"`
}

type systemLogOptimizationSwitchRequest struct {
	Enabled *bool `json:"enabled" form:"enabled"`
}

type systemLogOptimizationContentRequest struct {
	Content *string `json:"content" form:"content"`
}

type systemSysctlOptimizationSwitchRequest struct {
	Enabled *bool `json:"enabled" form:"enabled"`
}

type systemSysctlOptimizationContentRequest struct {
	Content *string `json:"content" form:"content"`
}

type systemLinuxDNSOptimizationContentRequest struct {
	Content *string `json:"content" form:"content"`
}

type systemLinuxDNSOptimizationNameServersRequest struct {
	NameServers *string `json:"nameServers" form:"nameServers"`
}

type systemMTUOptimizationSwitchRequest struct {
	Enabled *bool `json:"enabled" form:"enabled"`
	MTU     *int  `json:"mtu" form:"mtu"`
}

type systemMTUOptimizationSaveRequest struct {
	MTU *int `json:"mtu" form:"mtu"`
}

type kernelActionRequest struct {
	Provider          *string `json:"provider" form:"provider"`
	Line              *string `json:"line" form:"line"`
	Version           *string `json:"version" form:"version"`
	Arch              *string `json:"arch" form:"arch"`
	DownloadSessionID *string `json:"downloadSessionId" form:"downloadSessionId"`
}

type kernelCleanupPurgeRequest struct {
	Packages []string `json:"packages" form:"packages"`
}

type panelUpdateInstallRequest struct {
	Version string `json:"version" form:"version"`
}

type kernelCleanupMarkerRequest struct {
	Kernel *string `json:"kernel" form:"kernel"`
}

type acmeInstallRequest struct {
	Email   *string `json:"email" form:"email"`
	Version *string `json:"version" form:"version"`
}

type acmeContactEmailSaveRequest struct {
	Email *string `json:"email" form:"email"`
}

type acmeRemoveRequest struct {
	RemoveCertificates *bool `json:"removeCertificates" form:"removeCertificates"`
}

type acmeIssueRequest struct {
	Domains         *string `json:"domains" form:"domains"`
	CertificateType *string `json:"certificateType" form:"certificateType"`
	Challenge       *string `json:"challenge" form:"challenge"`
	Webroot         *string `json:"webroot" form:"webroot"`
	DNSProvider     *string `json:"dnsProvider" form:"dnsProvider"`
	DNSEnv          *string `json:"dnsEnv" form:"dnsEnv"`
	Server          *string `json:"server" form:"server"`
	KeyLength       *string `json:"keyLength" form:"keyLength"`
	CustomArgs      *string `json:"customArgs" form:"customArgs"`
	AcmeAccountID   *uint   `json:"acmeAccountId" form:"acmeAccountId"`
	DNSAccountID    *uint   `json:"dnsAccountId" form:"dnsAccountId"`
	AutoRenew       *bool   `json:"autoRenew" form:"autoRenew"`
	Remark          *string `json:"remark" form:"remark"`
	ApplyTarget     *string `json:"applyTarget" form:"applyTarget"`
	PushDir         *string `json:"pushDir" form:"pushDir"`
	LogSessionID    *string `json:"logSessionId" form:"logSessionId"`
}

type acmeRenewRequest struct {
	ID          *uint   `json:"id" form:"id"`
	Force       *bool   `json:"force" form:"force"`
	ApplyTarget *string `json:"applyTarget" form:"applyTarget"`
	PushDir     *string `json:"pushDir" form:"pushDir"`
}

type acmePushRequest struct {
	ID        *uint   `json:"id" form:"id"`
	TargetDir *string `json:"targetDir" form:"targetDir"`
}

type acmeSetAutoRenewRequest struct {
	ID        *uint `json:"id" form:"id"`
	AutoRenew *bool `json:"autoRenew" form:"autoRenew"`
}

type acmeApplyRequest struct {
	ID     *uint   `json:"id" form:"id"`
	Target *string `json:"target" form:"target"`
}

type acmeUnapplyRequest struct {
	ID     *uint   `json:"id" form:"id"`
	Target *string `json:"target" form:"target"`
}

type acmeDeleteRequest struct {
	ID *uint `json:"id" form:"id"`
}

type acmeViewRequest struct {
	ID *uint `json:"id" form:"id"`
}

type acmeAccountSaveRequest struct {
	ID        *uint   `json:"id" form:"id"`
	Name      *string `json:"name" form:"name"`
	Email     *string `json:"email" form:"email"`
	Server    *string `json:"server" form:"server"`
	KeyLength *string `json:"keyLength" form:"keyLength"`
	Remark    *string `json:"remark" form:"remark"`
}

type acmeAccountDeleteRequest struct {
	ID *uint `json:"id" form:"id"`
}

type acmeDNSAccountSaveRequest struct {
	ID           *uint   `json:"id" form:"id"`
	Name         *string `json:"name" form:"name"`
	ProviderCode *string `json:"providerCode" form:"providerCode"`
	EnvJSON      *string `json:"envJson" form:"envJson"`
	Remark       *string `json:"remark" form:"remark"`
}

type acmeDNSAccountDeleteRequest struct {
	ID *uint `json:"id" form:"id"`
}

type selfSignedIssueRequest struct {
	AuthorityID        *uint   `json:"authorityId" form:"authorityId"`
	AuthorityName      *string `json:"authorityName" form:"authorityName"`
	PlatformCode       *string `json:"platformCode" form:"platformCode"`
	PlatformName       *string `json:"platformName" form:"platformName"`
	SubjectCN          *string `json:"subjectCn" form:"subjectCn"`
	Organization       *string `json:"organization" form:"organization"`
	Department         *string `json:"department" form:"department"`
	Country            *string `json:"country" form:"country"`
	Province           *string `json:"province" form:"province"`
	City               *string `json:"city" form:"city"`
	SaveAuthority      *bool   `json:"saveAuthority" form:"saveAuthority"`
	Domains            *string `json:"domains" form:"domains"`
	KeyAlgorithm       *string `json:"keyAlgorithm" form:"keyAlgorithm"`
	SignatureAlgorithm *string `json:"signatureAlgorithm" form:"signatureAlgorithm"`
	DurationValue      *int    `json:"durationValue" form:"durationValue"`
	DurationUnit       *string `json:"durationUnit" form:"durationUnit"`
	Remark             *string `json:"remark" form:"remark"`
	ApplyTarget        *string `json:"applyTarget" form:"applyTarget"`
	PushDir            *string `json:"pushDir" form:"pushDir"`
}

type selfSignedDeleteAuthorityRequest struct {
	ID *uint `json:"id" form:"id"`
}

type selfSignedSaveAuthorityRequest struct {
	ID           *uint   `json:"id" form:"id"`
	Name         *string `json:"name" form:"name"`
	PlatformCode *string `json:"platformCode" form:"platformCode"`
	PlatformName *string `json:"platformName" form:"platformName"`
	SubjectCN    *string `json:"subjectCn" form:"subjectCn"`
	Organization *string `json:"organization" form:"organization"`
	Department   *string `json:"department" form:"department"`
	Country      *string `json:"country" form:"country"`
	Province     *string `json:"province" form:"province"`
	City         *string `json:"city" form:"city"`
	KeyAlgorithm *string `json:"keyAlgorithm" form:"keyAlgorithm"`
	IssuerName   *string `json:"issuerName" form:"issuerName"`
	IssuerOrg    *string `json:"issuerOrg" form:"issuerOrg"`
	CAURL        *string `json:"caUrl" form:"caUrl"`
	OCSPURL      *string `json:"ocspUrl" form:"ocspUrl"`
	CRLURL       *string `json:"crlUrl" form:"crlUrl"`
	KeyUsage     *string `json:"keyUsage" form:"keyUsage"`
	ExtKeyUsage  *string `json:"extKeyUsage" form:"extKeyUsage"`
	SignAlgo     *string `json:"signAlgo" form:"signAlgo"`
	Brand        *string `json:"brand" form:"brand"`
	Notes        *string `json:"notes" form:"notes"`
}

func normalizeAcmeIssueCertificateType(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "ip", "ipcert", "ip_certificate":
		return "ip"
	default:
		return "domain"
	}
}

func (a *ApiService) LoadData(c *gin.Context) {
	data, err := a.getData(c)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, data, nil)
}

func (a *ApiService) LoadMihomoData(c *gin.Context) {
	data, err := a.getMihomoData(c)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, data, nil)
}

func (a *ApiService) getData(c *gin.Context) (interface{}, error) {
	data := make(map[string]interface{}, 0)
	lu := c.Query("lu")
	light := strings.EqualFold(strings.TrimSpace(c.Query("light")), "true")
	isUpdated, err := a.ConfigService.CheckChanges(lu)
	if err != nil {
		return "", err
	}
	onlines, err := a.StatsService.GetOnlines()

	if err != nil {
		return "", err
	}
	trafficAge, err := a.SettingService.GetTrafficAge()
	if err != nil {
		return "", err
	}
	data["enableTraffic"] = trafficAge > 0

	if isUpdated {
		config, err := a.SettingService.GetConfig()
		if err != nil {
			return "", err
		}
		clients, err := a.ClientService.GetAll()
		if err != nil {
			return "", err
		}
		tlsConfigs, err := a.TlsService.GetAll()
		if err != nil {
			return "", err
		}
		inbounds, err := a.InboundService.GetAll()
		if err != nil {
			return "", err
		}
		outbounds, err := a.OutboundService.GetAll()
		if err != nil {
			return "", err
		}
		outboundGroups, err := a.OutboundGroupService.GetAll()
		if err != nil {
			return "", err
		}
		subOutbounds, err := a.SubOutboundService.GetAll()
		if err != nil {
			return "", err
		}
		subGroups, err := a.SubGroupService.GetAll()
		if err != nil {
			return "", err
		}
		endpoints, err := a.EndpointService.GetAll()
		if err != nil {
			return "", err
		}
		services, err := a.ServicesService.GetAll()
		if err != nil {
			return "", err
		}
		subURI, err := a.SettingService.GetFinalSubURI(getHostname(c))
		if err != nil {
			return "", err
		}
		data["config"] = json.RawMessage(config)
		data["clients"] = clients
		data["tls"] = tlsConfigs
		data["inbounds"] = inbounds
		data["outbounds"] = outbounds
		data["outboundgroups"] = outboundGroups
		data["suboutbounds"] = subOutbounds
		data["subgroups"] = subGroups
		data["endpoints"] = endpoints
		data["services"] = services
		data["subURI"] = subURI
		data["onlines"] = onlines
	} else if !light {
		config, err := a.SettingService.GetConfig()
		if err != nil {
			return "", err
		}
		clients, err := a.ClientService.GetAll()
		if err != nil {
			return "", err
		}
		data["config"] = json.RawMessage(config)
		data["clients"] = clients
		data["onlines"] = onlines
	} else {
		data["onlines"] = onlines
	}

	return data, nil
}

func (a *ApiService) getMihomoData(c *gin.Context) (interface{}, error) {
	data := make(map[string]interface{}, 0)
	lu := c.Query("lu")
	light := strings.EqualFold(strings.TrimSpace(c.Query("light")), "true")
	isUpdated, err := a.ConfigService.CheckChanges(lu)
	if err != nil {
		return "", err
	}

	onlines, err := a.StatsService.GetMihomoOnlines()
	if err != nil {
		return "", err
	}
	trafficAge, err := a.SettingService.GetTrafficAge()
	if err != nil {
		return "", err
	}
	data["onlines"] = onlines
	data["enableTraffic"] = trafficAge > 0

	if !isUpdated {
		if light {
			return data, nil
		}
		config, err := a.MihomoConfigService.GetConfig()
		if err != nil {
			return "", err
		}
		clients, err := a.MihomoClientService.GetAll()
		if err != nil {
			return "", err
		}
		data["config"] = json.RawMessage(config)
		data["clients"] = clients
		return data, nil
	}

	config, err := a.MihomoConfigService.GetConfig()
	if err != nil {
		return "", err
	}
	clients, err := a.MihomoClientService.GetAll()
	if err != nil {
		return "", err
	}
	tlsConfigs, err := a.MihomoTlsService.GetAll()
	if err != nil {
		return "", err
	}
	inbounds, err := a.MihomoInboundService.GetAll()
	if err != nil {
		return "", err
	}
	outbounds, err := a.MihomoOutboundService.GetAll()
	if err != nil {
		return "", err
	}
	outboundGroups, err := a.MihomoOutboundGroupService.GetAll()
	if err != nil {
		return "", err
	}
	subOutbounds, err := a.SubOutboundService.GetAll()
	if err != nil {
		return "", err
	}
	subGroups, err := a.SubGroupService.GetAll()
	if err != nil {
		return "", err
	}
	subURI, err := a.SettingService.GetFinalSubURI(getHostname(c))
	if err != nil {
		return "", err
	}

	data["config"] = json.RawMessage(config)
	data["clients"] = clients
	data["tls"] = tlsConfigs
	data["inbounds"] = inbounds
	data["outbounds"] = outbounds
	data["outboundgroups"] = outboundGroups
	data["suboutbounds"] = subOutbounds
	data["subgroups"] = subGroups
	data["subURI"] = subURI
	data["onlines"] = onlines
	data["enableTraffic"] = trafficAge > 0

	return data, nil
}

func (a *ApiService) LoadPartialData(c *gin.Context, objs []string) error {
	data := make(map[string]interface{}, 0)
	id := c.Query("id")

	for _, obj := range objs {
		switch obj {
		case "inbounds":
			inbounds, err := a.InboundService.Get(id)
			if err != nil {
				return err
			}
			data[obj] = inbounds
		case "outbounds":
			outbounds, err := a.OutboundService.GetAll()
			if err != nil {
				return err
			}
			data[obj] = outbounds
		case "outboundgroups":
			outboundGroups, err := a.OutboundGroupService.GetAll()
			if err != nil {
				return err
			}
			data[obj] = outboundGroups
		case "suboutbounds":
			subOutbounds, err := a.SubOutboundService.GetAll()
			if err != nil {
				return err
			}
			data[obj] = subOutbounds
		case "subgroups":
			subGroups, err := a.SubGroupService.GetAll()
			if err != nil {
				return err
			}
			data[obj] = subGroups
		case "endpoints":
			endpoints, err := a.EndpointService.GetAll()
			if err != nil {
				return err
			}
			data[obj] = endpoints
		case "services":
			services, err := a.ServicesService.GetAll()
			if err != nil {
				return err
			}
			data[obj] = services
		case "tls":
			tlsConfigs, err := a.TlsService.GetAll()
			if err != nil {
				return err
			}
			data[obj] = tlsConfigs
		case "clients":
			clients, err := a.ClientService.Get(id)
			if err != nil {
				return err
			}
			data[obj] = clients
		case "config":
			config, err := a.SettingService.GetConfig()
			if err != nil {
				return err
			}
			data[obj] = json.RawMessage(config)
		case "settings":
			settings, err := a.SettingService.GetAllSetting()
			if err != nil {
				return err
			}
			data[obj] = settings
		}
	}

	jsonObj(c, data, nil)
	return nil
}

func (a *ApiService) LoadMihomoPartialData(c *gin.Context, objs []string) error {
	data := make(map[string]interface{}, 0)
	id := c.Query("id")

	for _, obj := range objs {
		switch obj {
		case "mihomo_inbounds":
			inbounds, err := a.MihomoInboundService.Get(id)
			if err != nil {
				return err
			}
			data["inbounds"] = inbounds
		case "mihomo_outbounds":
			outbounds, err := a.MihomoOutboundService.GetAll()
			if err != nil {
				return err
			}
			data["outbounds"] = outbounds
		case "mihomo_outboundgroups":
			outboundGroups, err := a.MihomoOutboundGroupService.GetAll()
			if err != nil {
				return err
			}
			data["outboundgroups"] = outboundGroups
		case "mihomo_tls":
			tlsConfigs, err := a.MihomoTlsService.GetAll()
			if err != nil {
				return err
			}
			data["tls"] = tlsConfigs
		case "mihomo_clients":
			clients, err := a.MihomoClientService.Get(id)
			if err != nil {
				return err
			}
			data["clients"] = clients
		case "mihomo_config":
			config, err := a.MihomoConfigService.GetConfig()
			if err != nil {
				return err
			}
			data["config"] = json.RawMessage(config)
		case "suboutbounds":
			subOutbounds, err := a.SubOutboundService.GetAll()
			if err != nil {
				return err
			}
			data["suboutbounds"] = subOutbounds
		case "subgroups":
			subGroups, err := a.SubGroupService.GetAll()
			if err != nil {
				return err
			}
			data["subgroups"] = subGroups
		}
	}

	jsonObj(c, data, nil)
	return nil
}

func (a *ApiService) GetUsers(c *gin.Context) {
	users, err := a.UserService.GetUsers()
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, *users, nil)
}

func (a *ApiService) GetSettings(c *gin.Context) {
	data, err := a.SettingService.GetAllSetting()
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, data, err)
}

func (a *ApiService) GetStats(c *gin.Context) {
	resource := c.Query("resource")
	namespace := strings.TrimSpace(c.Query("namespace"))
	tag := c.Query("tag")
	limit, err := strconv.Atoi(c.Query("limit"))
	if err != nil {
		limit = 100
	}
	if namespace == "mihomo" {
		switch resource {
		case "inbound":
			resource = "mihomo_inbound"
		case "client":
			resource = "mihomo_client"
		}
	}
	data, err := a.StatsService.GetStats(resource, tag, limit)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, data, err)
}

func (a *ApiService) GetStatus(c *gin.Context) {
	request := c.Query("r")
	result := a.ServerService.GetStatus(request)
	jsonObj(c, result, nil)
}

func (a *ApiService) GetSystemMonitorOverview(c *gin.Context) {
	result, err := a.SystemMonitorService.GetOverview()
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, result, nil)
}

func (a *ApiService) GetSystemMonitorHistory(c *gin.Context) {
	startSec, _ := strconv.ParseInt(strings.TrimSpace(c.Query("start")), 10, 64)
	endSec, _ := strconv.ParseInt(strings.TrimSpace(c.Query("end")), 10, 64)
	bucketSeconds, _ := strconv.Atoi(strings.TrimSpace(c.Query("bucket_sec")))
	customValue, _ := strconv.Atoi(strings.TrimSpace(c.Query("value")))
	result, err := a.SystemMonitorService.GetHistory(service.SystemMonitorHistoryQuery{
		RangeKey:             c.Query("range"),
		CustomValue:          customValue,
		CustomUnit:           c.Query("unit"),
		RequestedGranularity: c.Query("granularity"),
		StartSec:             startSec,
		EndSec:               endSec,
		BucketSeconds:        bucketSeconds,
	})
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, result, nil)
}

func (a *ApiService) SaveSystemMonitorSettings(c *gin.Context) {
	req := systemMonitorSettingsRequest{}
	if err := c.ShouldBind(&req); err != nil {
		jsonMsg(c, "", fmt.Errorf("invalid request body: %w", err))
		return
	}
	if req.SampleIntervalSec == nil || req.PrimaryRetentionHours == nil || req.ArchiveRetentionDays == nil {
		jsonMsg(c, "", fmt.Errorf("sample_interval_sec, primary_retention_hours and archive_retention_days are required"))
		return
	}
	if err := a.SystemMonitorService.SaveSettings(*req.SampleIntervalSec, *req.PrimaryRetentionHours, *req.ArchiveRetentionDays); err != nil {
		jsonMsg(c, "", err)
		return
	}
	a.GetSystemMonitorOverview(c)
}

func (a *ApiService) ResetSystemMonitorStats(c *gin.Context) {
	overview, err := a.SystemMonitorService.ClearStats()
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, overview, nil)
}

func (a *ApiService) GetTrafficOverview(c *gin.Context) {
	overview, err := a.TrafficOverviewService.GetTrafficOverview()
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, overview, nil)
}

func (a *ApiService) SaveTrafficOverviewSettings(c *gin.Context) {
	req := trafficOverviewSettingsRequest{}
	if err := c.ShouldBind(&req); err != nil {
		jsonMsg(c, "", fmt.Errorf("invalid request body: %w", err))
		return
	}

	limitGiB, limitExists := pickTrafficOverviewLimitGiB(req)
	resetDay, resetExists := pickTrafficOverviewResetDay(req)
	if !limitExists || !resetExists {
		jsonMsg(c, "", fmt.Errorf("limit_gib and reset_day are required"))
		return
	}

	if err := a.TrafficOverviewService.UpdateTrafficOverviewSettings(limitGiB, resetDay); err != nil {
		jsonMsg(c, "", err)
		return
	}
	a.GetTrafficOverview(c)
}

func (a *ApiService) SaveTrafficOverviewSwitch(c *gin.Context) {
	req := trafficOverviewSwitchRequest{}
	if err := c.ShouldBind(&req); err != nil {
		jsonMsg(c, "", fmt.Errorf("invalid request body: %w", err))
		return
	}
	if req.Enabled == nil {
		jsonMsg(c, "", fmt.Errorf("enabled is required"))
		return
	}
	if err := a.TrafficOverviewService.SetTrafficOverviewEnabled(*req.Enabled); err != nil {
		jsonMsg(c, "", err)
		return
	}
	a.GetTrafficOverview(c)
}

func (a *ApiService) GetTrafficOverviewVnstatVersions(c *gin.Context) {
	result, err := a.TrafficOverviewService.GetVnstatVersionOptions()
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, result, nil)
}

func (a *ApiService) GetTrafficOverviewVnstatUpdateInfo(c *gin.Context) {
	req := trafficOverviewVnstatUpdateRequest{}
	if err := c.ShouldBindQuery(&req); err != nil {
		jsonMsg(c, "", fmt.Errorf("invalid query params: %w", err))
		return
	}
	result, err := a.TrafficOverviewService.GetVnstatUpdateInfo(req.Source)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, result, nil)
}

func (a *ApiService) InstallTrafficOverviewVnstat(c *gin.Context) {
	req := trafficOverviewVnstatInstallRequest{}
	if err := c.ShouldBind(&req); err != nil {
		jsonMsg(c, "", fmt.Errorf("invalid request body: %w", err))
		return
	}
	overview, err := a.TrafficOverviewService.InstallManagedVnstat(req.Source)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, overview, nil)
}

func (a *ApiService) RemoveTrafficOverviewVnstat(c *gin.Context) {
	overview, err := a.TrafficOverviewService.RemoveManagedVnstat()
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, overview, nil)
}

func pickTrafficOverviewLimitGiB(req trafficOverviewSettingsRequest) (float64, bool) {
	if req.LimitGiB != nil {
		return *req.LimitGiB, true
	}
	if req.LimitGiBCompat != nil {
		return *req.LimitGiBCompat, true
	}
	return 0, false
}

func pickTrafficOverviewResetDay(req trafficOverviewSettingsRequest) (int, bool) {
	if req.ResetDay != nil {
		return *req.ResetDay, true
	}
	if req.ResetDayCompat != nil {
		return *req.ResetDayCompat, true
	}
	return 0, false
}

func (a *ApiService) ResetTrafficOverview(c *gin.Context) {
	if err := a.TrafficOverviewService.ResetAllTrafficOverviewStats(); err != nil {
		jsonMsg(c, "", err)
		return
	}
	a.GetTrafficOverview(c)
}

func (a *ApiService) ResetTrafficOverviewPeriod(c *gin.Context) {
	if err := a.TrafficOverviewService.ResetPeriodTrafficOverviewStats(); err != nil {
		jsonMsg(c, "", err)
		return
	}
	a.GetTrafficOverview(c)
}

func (a *ApiService) ResetTrafficOverviewTotal(c *gin.Context) {
	if err := a.TrafficOverviewService.ResetTotalTrafficOverviewStats(); err != nil {
		jsonMsg(c, "", err)
		return
	}
	a.GetTrafficOverview(c)
}

func (a *ApiService) GetFirewallOverview(c *gin.Context) {
	overview, err := a.FirewallService.GetOverview()
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, overview, nil)
}

func (a *ApiService) SaveFirewallSwitch(c *gin.Context) {
	req := firewallSwitchRequest{}
	if err := c.ShouldBind(&req); err != nil {
		jsonMsg(c, "", fmt.Errorf("invalid request body: %w", err))
		return
	}
	if req.Enabled == nil {
		jsonMsg(c, "", fmt.Errorf("enabled is required"))
		return
	}
	if err := a.FirewallService.SetEnabled(*req.Enabled); err != nil {
		jsonMsg(c, "", err)
		return
	}
	a.GetFirewallOverview(c)
}

func (a *ApiService) InstallFirewallNftables(c *gin.Context) {
	overview, err := a.FirewallService.InstallNftables()
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, overview, nil)
}

func (a *ApiService) SaveFirewallSSHPort(c *gin.Context) {
	req := firewallSSHPortRequest{}
	if err := c.ShouldBind(&req); err != nil {
		jsonMsg(c, "", fmt.Errorf("invalid request body: %w", err))
		return
	}
	if req.Port == nil {
		jsonMsg(c, "", fmt.Errorf("port is required"))
		return
	}
	if err := a.FirewallService.UpdateSSHPort(*req.Port); err != nil {
		jsonMsg(c, "", err)
		return
	}
	a.GetFirewallOverview(c)
}

func (a *ApiService) SaveFirewallSSHProxy(c *gin.Context) {
	req := firewallSSHProxyRequest{}
	if err := c.ShouldBind(&req); err != nil {
		jsonMsg(c, "", fmt.Errorf("invalid request body: %w", err))
		return
	}
	if req.Enabled == nil {
		jsonMsg(c, "", fmt.Errorf("enabled is required"))
		return
	}
	if err := a.FirewallService.SetSSHProxyEnabled(*req.Enabled); err != nil {
		jsonMsg(c, "", err)
		return
	}
	a.GetFirewallOverview(c)
}

func (a *ApiService) SaveFirewallSystemRule(c *gin.Context) {
	req := firewallSystemRuleRequest{}
	if err := c.ShouldBind(&req); err != nil {
		jsonMsg(c, "", fmt.Errorf("invalid request body: %w", err))
		return
	}
	if req.SystemKey == nil || strings.TrimSpace(*req.SystemKey) == "" {
		jsonMsg(c, "", fmt.Errorf("systemKey is required"))
		return
	}
	if req.Enabled == nil {
		jsonMsg(c, "", fmt.Errorf("enabled is required"))
		return
	}
	if err := a.FirewallService.SetSystemRuleReserved(*req.SystemKey, *req.Enabled); err != nil {
		jsonMsg(c, "", err)
		return
	}
	a.GetFirewallOverview(c)
}

func (a *ApiService) SaveFirewallRule(c *gin.Context) {
	req := service.FirewallRulePayload{}
	if err := c.ShouldBind(&req); err != nil {
		jsonMsg(c, "", fmt.Errorf("invalid request body: %w", err))
		return
	}
	if err := a.FirewallService.UpsertRule(req); err != nil {
		jsonMsg(c, "", err)
		return
	}
	a.GetFirewallOverview(c)
}

func (a *ApiService) DeleteFirewallRule(c *gin.Context) {
	req := firewallRuleDeleteRequest{}
	if err := c.ShouldBind(&req); err != nil {
		jsonMsg(c, "", fmt.Errorf("invalid request body: %w", err))
		return
	}
	if req.ID == nil || *req.ID == 0 {
		jsonMsg(c, "", fmt.Errorf("id is required"))
		return
	}
	if err := a.FirewallService.DeleteRule(*req.ID); err != nil {
		jsonMsg(c, "", err)
		return
	}
	a.GetFirewallOverview(c)
}

func (a *ApiService) SaveFirewallGeoRule(c *gin.Context) {
	req := service.FirewallGeoRulePayload{}
	if err := c.ShouldBind(&req); err != nil {
		jsonMsg(c, "", fmt.Errorf("invalid request body: %w", err))
		return
	}
	if err := a.FirewallService.UpsertGeoRule(req); err != nil {
		jsonMsg(c, "", err)
		return
	}
	a.GetFirewallOverview(c)
}

func (a *ApiService) DeleteFirewallGeoRule(c *gin.Context) {
	req := firewallRuleDeleteRequest{}
	if err := c.ShouldBind(&req); err != nil {
		jsonMsg(c, "", fmt.Errorf("invalid request body: %w", err))
		return
	}
	if req.ID == nil || *req.ID == 0 {
		jsonMsg(c, "", fmt.Errorf("id is required"))
		return
	}
	if err := a.FirewallService.DeleteGeoRule(*req.ID); err != nil {
		jsonMsg(c, "", err)
		return
	}
	a.GetFirewallOverview(c)
}

func (a *ApiService) RefreshFirewallGeoRules(c *gin.Context) {
	if err := a.FirewallService.RefreshGeoRules(); err != nil {
		jsonMsg(c, "", err)
		return
	}
	a.GetFirewallOverview(c)
}

func (a *ApiService) SaveFirewallGeoSettings(c *gin.Context) {
	req := firewallGeoSettingsRequest{}
	if err := c.ShouldBind(&req); err != nil {
		jsonMsg(c, "", fmt.Errorf("invalid request body: %w", err))
		return
	}
	if req.IntervalMinutes == nil || *req.IntervalMinutes <= 0 {
		jsonMsg(c, "", fmt.Errorf("intervalMinutes must be a positive integer"))
		return
	}
	if err := a.FirewallService.SaveGeoSettings(*req.IntervalMinutes); err != nil {
		jsonMsg(c, "", err)
		return
	}
	a.GetFirewallOverview(c)
}

func (a *ApiService) GetSystemLogOptimizationOverview(c *gin.Context) {
	overview, err := a.SystemLogOptimizationService.GetOverview()
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, overview, nil)
}

func (a *ApiService) SaveSystemLogOptimizationSwitch(c *gin.Context) {
	req := systemLogOptimizationSwitchRequest{}
	if err := c.ShouldBind(&req); err != nil {
		jsonMsg(c, "", fmt.Errorf("invalid request body: %w", err))
		return
	}
	if req.Enabled == nil {
		jsonMsg(c, "", fmt.Errorf("enabled is required"))
		return
	}
	if err := a.SystemLogOptimizationService.SetDisabled(*req.Enabled); err != nil {
		jsonMsg(c, "", err)
		return
	}
	a.GetSystemLogOptimizationOverview(c)
}

func (a *ApiService) SaveSystemLogOptimizationContent(c *gin.Context) {
	req := systemLogOptimizationContentRequest{}
	if err := c.ShouldBind(&req); err != nil {
		jsonMsg(c, "", fmt.Errorf("invalid request body: %w", err))
		return
	}
	if req.Content == nil {
		jsonMsg(c, "", fmt.Errorf("content is required"))
		return
	}
	if err := a.SystemLogOptimizationService.SaveContent(*req.Content); err != nil {
		jsonMsg(c, "", err)
		return
	}
	a.GetSystemLogOptimizationOverview(c)
}

func (a *ApiService) ResetSystemLogOptimizationContent(c *gin.Context) {
	if err := a.SystemLogOptimizationService.ResetContent(); err != nil {
		jsonMsg(c, "", err)
		return
	}
	a.GetSystemLogOptimizationOverview(c)
}

func (a *ApiService) GetSystemSysctlOptimizationOverview(c *gin.Context) {
	overview, err := a.SystemSysctlOptimizationService.GetOverview()
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, overview, nil)
}

func (a *ApiService) SaveSystemSysctlOptimizationSwitch(c *gin.Context) {
	req := systemSysctlOptimizationSwitchRequest{}
	if err := c.ShouldBind(&req); err != nil {
		jsonMsg(c, "", fmt.Errorf("invalid request body: %w", err))
		return
	}
	if req.Enabled == nil {
		jsonMsg(c, "", fmt.Errorf("enabled is required"))
		return
	}
	if err := a.SystemSysctlOptimizationService.SetEnabled(*req.Enabled); err != nil {
		jsonMsg(c, "", err)
		return
	}
	a.GetSystemSysctlOptimizationOverview(c)
}

func (a *ApiService) SaveSystemSysctlOptimizationContent(c *gin.Context) {
	req := systemSysctlOptimizationContentRequest{}
	if err := c.ShouldBind(&req); err != nil {
		jsonMsg(c, "", fmt.Errorf("invalid request body: %w", err))
		return
	}
	if req.Content == nil {
		jsonMsg(c, "", fmt.Errorf("content is required"))
		return
	}
	if err := a.SystemSysctlOptimizationService.SaveContent(*req.Content); err != nil {
		jsonMsg(c, "", err)
		return
	}
	a.GetSystemSysctlOptimizationOverview(c)
}

func (a *ApiService) ResetSystemSysctlOptimizationContent(c *gin.Context) {
	if err := a.SystemSysctlOptimizationService.ResetContent(); err != nil {
		jsonMsg(c, "", err)
		return
	}
	a.GetSystemSysctlOptimizationOverview(c)
}

func (a *ApiService) GetSystemLinuxDNSOptimizationOverview(c *gin.Context) {
	overview, err := a.SystemLinuxDNSOptimizationService.GetOverview()
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, overview, nil)
}

func (a *ApiService) SaveSystemLinuxDNSOptimizationContent(c *gin.Context) {
	req := systemLinuxDNSOptimizationContentRequest{}
	if err := c.ShouldBind(&req); err != nil {
		jsonMsg(c, "", fmt.Errorf("invalid request body: %w", err))
		return
	}
	if req.Content == nil {
		jsonMsg(c, "", fmt.Errorf("content is required"))
		return
	}
	if err := a.SystemLinuxDNSOptimizationService.SaveContent(*req.Content); err != nil {
		jsonMsg(c, "", err)
		return
	}
	a.GetSystemLinuxDNSOptimizationOverview(c)
}

func (a *ApiService) SaveSystemLinuxDNSOptimizationNameServers(c *gin.Context) {
	req := systemLinuxDNSOptimizationNameServersRequest{}
	if err := c.ShouldBind(&req); err != nil {
		jsonMsg(c, "", fmt.Errorf("invalid request body: %w", err))
		return
	}
	if req.NameServers == nil {
		jsonMsg(c, "", fmt.Errorf("nameServers is required"))
		return
	}
	if err := a.SystemLinuxDNSOptimizationService.SaveNameServers(*req.NameServers); err != nil {
		jsonMsg(c, "", err)
		return
	}
	a.GetSystemLinuxDNSOptimizationOverview(c)
}

func (a *ApiService) GetSystemMTUOptimizationOverview(c *gin.Context) {
	overview, err := a.SystemMTUOptimizationService.GetOverview()
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, overview, nil)
}

func (a *ApiService) SaveSystemMTUOptimizationSwitch(c *gin.Context) {
	req := systemMTUOptimizationSwitchRequest{}
	if err := c.ShouldBind(&req); err != nil {
		jsonMsg(c, "", fmt.Errorf("invalid request body: %w", err))
		return
	}
	if req.Enabled == nil {
		jsonMsg(c, "", fmt.Errorf("enabled is required"))
		return
	}
	if err := a.SystemMTUOptimizationService.SetEnabled(*req.Enabled, req.MTU); err != nil {
		jsonMsg(c, "", err)
		return
	}
	a.GetSystemMTUOptimizationOverview(c)
}

func (a *ApiService) SaveSystemMTUOptimizationMTU(c *gin.Context) {
	req := systemMTUOptimizationSaveRequest{}
	if err := c.ShouldBind(&req); err != nil {
		jsonMsg(c, "", fmt.Errorf("invalid request body: %w", err))
		return
	}
	if req.MTU == nil {
		jsonMsg(c, "", fmt.Errorf("mtu is required"))
		return
	}
	if err := a.SystemMTUOptimizationService.SaveMTU(*req.MTU); err != nil {
		jsonMsg(c, "", err)
		return
	}
	a.GetSystemMTUOptimizationOverview(c)
}

func (a *ApiService) GetAcmeOverview(c *gin.Context) {
	overview, err := a.AcmeService.GetOverview()
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, overview, nil)
}

func (a *ApiService) GetAcmeVersions(c *gin.Context) {
	page := 1
	if pageRaw := c.Query("page"); pageRaw != "" {
		if parsed, parseErr := strconv.Atoi(pageRaw); parseErr == nil && parsed > 0 {
			page = parsed
		}
	}

	perPage := 5
	if perPageRaw := c.Query("per_page"); perPageRaw != "" {
		if parsed, parseErr := strconv.Atoi(perPageRaw); parseErr == nil && parsed > 0 {
			perPage = parsed
		}
	}

	result, err := a.AcmeService.GetRemoteVersionsPage(page, perPage)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, result, nil)
}

func (a *ApiService) GetAcmeUpdateInfo(c *gin.Context) {
	info, err := a.AcmeService.CheckUpdate()
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, info, nil)
}

func (a *ApiService) GetAcmeLog(c *gin.Context) {
	id := strings.TrimSpace(c.Query("id"))
	if id == "" {
		jsonMsg(c, "", fmt.Errorf("id is required"))
		return
	}
	session, err := a.AcmeService.GetLogSession(id)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, session, nil)
}

func (a *ApiService) GetAcmeIPPortStatus(c *gin.Context) {
	result, err := a.AcmeService.GetIPCertificatePortStatus()
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, result, nil)
}

func (a *ApiService) GetSelfSignedAuthorities(c *gin.Context) {
	rows, err := a.SelfSignedService.ListAuthorities()
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, rows, nil)
}

func (a *ApiService) InstallAcme(c *gin.Context) {
	req := acmeInstallRequest{}
	if err := c.ShouldBind(&req); err != nil {
		jsonMsg(c, "", fmt.Errorf("invalid request body: %w", err))
		return
	}
	email := ""
	emailProvided := false
	version := ""
	if req.Email != nil {
		email = strings.Join(strings.Fields(*req.Email), "")
		emailProvided = true
	}
	if req.Version != nil {
		version = strings.TrimSpace(*req.Version)
	}
	result, err := a.AcmeService.InstallOrReinstall(service.AcmeInstallPayload{
		Email:         email,
		EmailProvided: emailProvided,
		Version:       version,
	})
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, result, nil)
}

func (a *ApiService) SaveAcmeContactEmail(c *gin.Context) {
	req := acmeContactEmailSaveRequest{}
	if err := c.ShouldBind(&req); err != nil {
		jsonMsg(c, "", fmt.Errorf("invalid request body: %w", err))
		return
	}
	if req.Email == nil {
		jsonMsg(c, "", fmt.Errorf("email is required"))
		return
	}

	result, err := a.AcmeService.SaveContactEmail(*req.Email)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, result, nil)
}

func (a *ApiService) RemoveAcme(c *gin.Context) {
	req := acmeRemoveRequest{}
	if err := c.ShouldBind(&req); err != nil {
		jsonMsg(c, "", fmt.Errorf("invalid request body: %w", err))
		return
	}
	removeCertificates := false
	if req.RemoveCertificates != nil {
		removeCertificates = *req.RemoveCertificates
	}
	result, err := a.AcmeService.RemoveManagedAcme(service.AcmeRemovePayload{
		RemoveCertificates: removeCertificates,
	})
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, result, nil)
}

func (a *ApiService) UpgradeAcme(c *gin.Context) {
	result, err := a.AcmeService.Upgrade()
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, result, nil)
}

func (a *ApiService) IssueAcmeCertificate(c *gin.Context) {
	req := acmeIssueRequest{}
	if err := c.ShouldBind(&req); err != nil {
		jsonMsg(c, "", fmt.Errorf("invalid request body: %w", err))
		return
	}

	certificateType := "domain"
	if req.CertificateType != nil {
		certificateType = strings.TrimSpace(*req.CertificateType)
	}
	normalizedCertificateType := normalizeAcmeIssueCertificateType(certificateType)
	if normalizedCertificateType == "domain" && (req.AcmeAccountID == nil || *req.AcmeAccountID == 0) {
		jsonMsg(c, "", fmt.Errorf("acmeAccountId is required for domain certificate"))
		return
	}

	payload := service.AcmeIssuePayload{}
	if req.Domains != nil {
		payload.DomainsText = strings.TrimSpace(*req.Domains)
	}
	if req.CertificateType != nil {
		payload.CertificateType = strings.TrimSpace(*req.CertificateType)
	}
	if req.Challenge != nil {
		payload.Challenge = strings.TrimSpace(*req.Challenge)
	}
	if req.Webroot != nil {
		payload.Webroot = strings.TrimSpace(*req.Webroot)
	}
	if req.DNSProvider != nil {
		payload.DNSProvider = strings.TrimSpace(*req.DNSProvider)
	}
	if req.DNSEnv != nil {
		payload.DNSEnvText = *req.DNSEnv
	}
	if req.Server != nil {
		payload.Server = strings.TrimSpace(*req.Server)
	}
	if req.KeyLength != nil {
		payload.KeyLength = strings.TrimSpace(*req.KeyLength)
	}
	if req.CustomArgs != nil {
		payload.CustomArgs = strings.TrimSpace(*req.CustomArgs)
	}
	if normalizedCertificateType == "domain" && req.AcmeAccountID != nil {
		payload.AcmeAccountID = *req.AcmeAccountID
	}
	if req.DNSAccountID != nil {
		payload.DNSAccountID = *req.DNSAccountID
	}
	payload.AutoRenew = true
	if req.AutoRenew != nil {
		payload.AutoRenew = *req.AutoRenew
	}
	if req.Remark != nil {
		payload.Remark = strings.TrimSpace(*req.Remark)
	}
	if req.ApplyTarget != nil {
		payload.ApplyTarget = strings.TrimSpace(*req.ApplyTarget)
	}
	if req.PushDir != nil {
		payload.PushDir = strings.TrimSpace(*req.PushDir)
		payload.PushExplicit = payload.PushDir != ""
	}
	if req.LogSessionID != nil {
		payload.LogSessionID = strings.TrimSpace(*req.LogSessionID)
	}

	result, err := a.AcmeService.Issue(payload)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, result, nil)
}

func (a *ApiService) RenewAcmeCertificate(c *gin.Context) {
	req := acmeRenewRequest{}
	if err := c.ShouldBind(&req); err != nil {
		jsonMsg(c, "", fmt.Errorf("invalid request body: %w", err))
		return
	}
	if req.ID == nil || *req.ID == 0 {
		jsonMsg(c, "", fmt.Errorf("id is required"))
		return
	}

	payload := service.AcmeRenewPayload{
		ID: *req.ID,
	}
	if req.Force != nil {
		payload.Force = *req.Force
	}
	if req.ApplyTarget != nil {
		payload.ApplyTarget = strings.TrimSpace(*req.ApplyTarget)
	}
	result, err := a.AcmeService.Renew(payload)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, result, nil)
}

func (a *ApiService) PushAcmeCertificate(c *gin.Context) {
	req := acmePushRequest{}
	if err := c.ShouldBind(&req); err != nil {
		jsonMsg(c, "", fmt.Errorf("invalid request body: %w", err))
		return
	}
	if req.ID == nil || *req.ID == 0 {
		jsonMsg(c, "", fmt.Errorf("id is required"))
		return
	}
	if req.TargetDir == nil || strings.TrimSpace(*req.TargetDir) == "" {
		jsonMsg(c, "", fmt.Errorf("targetDir is required"))
		return
	}

	result, err := a.AcmeService.Push(service.AcmePushPayload{
		ID:        *req.ID,
		TargetDir: strings.TrimSpace(*req.TargetDir),
	})
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, result, nil)
}

func (a *ApiService) SetAcmeCertificateAutoRenew(c *gin.Context) {
	req := acmeSetAutoRenewRequest{}
	if err := c.ShouldBind(&req); err != nil {
		jsonMsg(c, "", fmt.Errorf("invalid request body: %w", err))
		return
	}
	if req.ID == nil || *req.ID == 0 {
		jsonMsg(c, "", fmt.Errorf("id is required"))
		return
	}
	if req.AutoRenew == nil {
		jsonMsg(c, "", fmt.Errorf("autoRenew is required"))
		return
	}

	result, err := a.AcmeService.SetAutoRenew(service.AcmeSetAutoRenewPayload{
		ID:        *req.ID,
		AutoRenew: *req.AutoRenew,
	})
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, result, nil)
}

func (a *ApiService) ApplyAcmeCertificate(c *gin.Context) {
	req := acmeApplyRequest{}
	if err := c.ShouldBind(&req); err != nil {
		jsonMsg(c, "", fmt.Errorf("invalid request body: %w", err))
		return
	}
	if req.ID == nil || *req.ID == 0 {
		jsonMsg(c, "", fmt.Errorf("id is required"))
		return
	}
	if req.Target == nil || strings.TrimSpace(*req.Target) == "" {
		jsonMsg(c, "", fmt.Errorf("target is required"))
		return
	}

	result, err := a.AcmeService.Apply(service.AcmeApplyPayload{
		ID:     *req.ID,
		Target: strings.TrimSpace(*req.Target),
	})
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, result, nil)
}

func (a *ApiService) UnapplyAcmeCertificate(c *gin.Context) {
	req := acmeUnapplyRequest{}
	if err := c.ShouldBind(&req); err != nil {
		jsonMsg(c, "", fmt.Errorf("invalid request body: %w", err))
		return
	}
	if req.ID == nil || *req.ID == 0 {
		jsonMsg(c, "", fmt.Errorf("id is required"))
		return
	}
	if req.Target == nil || strings.TrimSpace(*req.Target) == "" {
		jsonMsg(c, "", fmt.Errorf("target is required"))
		return
	}

	result, err := a.AcmeService.Unapply(service.AcmeUnapplyPayload{
		ID:     *req.ID,
		Target: strings.TrimSpace(*req.Target),
	})
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, result, nil)
}

func (a *ApiService) DeleteAcmeCertificate(c *gin.Context) {
	req := acmeDeleteRequest{}
	if err := c.ShouldBind(&req); err != nil {
		jsonMsg(c, "", fmt.Errorf("invalid request body: %w", err))
		return
	}
	if req.ID == nil || *req.ID == 0 {
		jsonMsg(c, "", fmt.Errorf("id is required"))
		return
	}

	result, err := a.AcmeService.Delete(service.AcmeDeletePayload{
		ID: *req.ID,
	})
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, result, nil)
}

func (a *ApiService) ListCertificates(c *gin.Context) {
	certificates, err := a.CertificateInventoryService.List()
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, certificates, nil)
}

func (a *ApiService) GetCertificateMaterial(c *gin.Context) {
	req := acmeViewRequest{}
	if err := c.ShouldBind(&req); err != nil {
		jsonMsg(c, "", fmt.Errorf("invalid request body: %w", err))
		return
	}
	if req.ID == nil || *req.ID == 0 {
		jsonMsg(c, "", fmt.Errorf("id is required"))
		return
	}

	material, err := a.CertificateInventoryService.GetMaterial(*req.ID)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, material, nil)
}

func (a *ApiService) ViewAcmeCertificate(c *gin.Context) {
	req := acmeViewRequest{}
	if err := c.ShouldBind(&req); err != nil {
		jsonMsg(c, "", fmt.Errorf("invalid request body: %w", err))
		return
	}
	if req.ID == nil || *req.ID == 0 {
		jsonMsg(c, "", fmt.Errorf("id is required"))
		return
	}

	material, err := a.CertificateInventoryService.GetMaterial(*req.ID)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, material, nil)
}

func (a *ApiService) SaveAcmeAccount(c *gin.Context) {
	req := acmeAccountSaveRequest{}
	if err := c.ShouldBind(&req); err != nil {
		jsonMsg(c, "", fmt.Errorf("invalid request body: %w", err))
		return
	}
	if req.Name == nil || strings.TrimSpace(*req.Name) == "" {
		jsonMsg(c, "", fmt.Errorf("name is required"))
		return
	}
	if req.Email == nil || strings.TrimSpace(*req.Email) == "" {
		jsonMsg(c, "", fmt.Errorf("email is required"))
		return
	}

	payload := service.AcmeAccountPayload{
		Name:  strings.TrimSpace(*req.Name),
		Email: strings.Join(strings.Fields(*req.Email), ""),
	}
	if req.ID != nil {
		payload.ID = *req.ID
	}
	if req.Server != nil {
		payload.Server = strings.TrimSpace(*req.Server)
	}
	if req.KeyLength != nil {
		payload.KeyLength = strings.TrimSpace(*req.KeyLength)
	}
	if req.Remark != nil {
		payload.Remark = strings.TrimSpace(*req.Remark)
	}

	result, err := a.AcmeService.SaveAcmeAccount(payload)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, result, nil)
}

func (a *ApiService) DeleteAcmeAccount(c *gin.Context) {
	req := acmeAccountDeleteRequest{}
	if err := c.ShouldBind(&req); err != nil {
		jsonMsg(c, "", fmt.Errorf("invalid request body: %w", err))
		return
	}
	if req.ID == nil || *req.ID == 0 {
		jsonMsg(c, "", fmt.Errorf("id is required"))
		return
	}

	result, err := a.AcmeService.DeleteAcmeAccount(*req.ID)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, result, nil)
}

func (a *ApiService) SaveAcmeDNSAccount(c *gin.Context) {
	req := acmeDNSAccountSaveRequest{}
	if err := c.ShouldBind(&req); err != nil {
		jsonMsg(c, "", fmt.Errorf("invalid request body: %w", err))
		return
	}
	if req.Name == nil || strings.TrimSpace(*req.Name) == "" {
		jsonMsg(c, "", fmt.Errorf("name is required"))
		return
	}
	if req.ProviderCode == nil || strings.TrimSpace(*req.ProviderCode) == "" {
		jsonMsg(c, "", fmt.Errorf("providerCode is required"))
		return
	}

	payload := service.AcmeDNSAccountPayload{
		Name:         strings.TrimSpace(*req.Name),
		ProviderCode: strings.TrimSpace(*req.ProviderCode),
		EnvJSON:      "{}",
	}
	if req.ID != nil {
		payload.ID = *req.ID
	}
	if req.EnvJSON != nil && strings.TrimSpace(*req.EnvJSON) != "" {
		payload.EnvJSON = strings.TrimSpace(*req.EnvJSON)
	}
	if req.Remark != nil {
		payload.Remark = strings.TrimSpace(*req.Remark)
	}

	result, err := a.AcmeService.SaveDNSAccount(payload)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, result, nil)
}

func (a *ApiService) DeleteAcmeDNSAccount(c *gin.Context) {
	req := acmeDNSAccountDeleteRequest{}
	if err := c.ShouldBind(&req); err != nil {
		jsonMsg(c, "", fmt.Errorf("invalid request body: %w", err))
		return
	}
	if req.ID == nil || *req.ID == 0 {
		jsonMsg(c, "", fmt.Errorf("id is required"))
		return
	}

	result, err := a.AcmeService.DeleteDNSAccount(*req.ID)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, result, nil)
}

func (a *ApiService) IssueSelfSignedCertificate(c *gin.Context) {
	req := selfSignedIssueRequest{}
	if err := c.ShouldBind(&req); err != nil {
		jsonMsg(c, "", fmt.Errorf("invalid request body: %w", err))
		return
	}
	if req.Domains == nil || strings.TrimSpace(*req.Domains) == "" {
		jsonMsg(c, "", fmt.Errorf("domains is required"))
		return
	}

	payload := service.SelfSignedIssuePayload{
		DomainsText: strings.TrimSpace(*req.Domains),
	}
	if req.AuthorityID != nil {
		payload.AuthorityID = *req.AuthorityID
	}
	if req.AuthorityName != nil {
		payload.AuthorityName = strings.TrimSpace(*req.AuthorityName)
	}
	if req.PlatformCode != nil {
		payload.PlatformCode = strings.TrimSpace(*req.PlatformCode)
	}
	if req.PlatformName != nil {
		payload.PlatformName = strings.TrimSpace(*req.PlatformName)
	}
	if req.SubjectCN != nil {
		payload.SubjectCN = strings.TrimSpace(*req.SubjectCN)
	}
	if req.Organization != nil {
		payload.Organization = strings.TrimSpace(*req.Organization)
	}
	if req.Department != nil {
		payload.Department = strings.TrimSpace(*req.Department)
	}
	if req.Country != nil {
		payload.Country = strings.TrimSpace(*req.Country)
	}
	if req.Province != nil {
		payload.Province = strings.TrimSpace(*req.Province)
	}
	if req.City != nil {
		payload.City = strings.TrimSpace(*req.City)
	}
	if req.SaveAuthority != nil {
		payload.SaveAuthority = *req.SaveAuthority
	}
	if req.KeyAlgorithm != nil {
		payload.KeyAlgorithm = strings.TrimSpace(*req.KeyAlgorithm)
	}
	if req.SignatureAlgorithm != nil {
		payload.SignatureAlgorithm = strings.TrimSpace(*req.SignatureAlgorithm)
	}
	if req.DurationValue != nil {
		payload.DurationValue = *req.DurationValue
	}
	if req.DurationUnit != nil {
		payload.DurationUnit = strings.TrimSpace(*req.DurationUnit)
	}
	if req.Remark != nil {
		payload.Remark = strings.TrimSpace(*req.Remark)
	}
	if req.ApplyTarget != nil {
		payload.ApplyTarget = strings.TrimSpace(*req.ApplyTarget)
	}
	if req.PushDir != nil {
		payload.PushDir = strings.TrimSpace(*req.PushDir)
		payload.PushExplicit = payload.PushDir != ""
	}

	result, err := a.SelfSignedService.Issue(payload)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, result, nil)
}

func (a *ApiService) SaveSelfSignedAuthority(c *gin.Context) {
	req := selfSignedSaveAuthorityRequest{}
	if err := c.ShouldBind(&req); err != nil {
		jsonMsg(c, "", fmt.Errorf("invalid request body: %w", err))
		return
	}
	if req.Name == nil || strings.TrimSpace(*req.Name) == "" {
		jsonMsg(c, "", fmt.Errorf("name is required"))
		return
	}
	if req.SubjectCN == nil || strings.TrimSpace(*req.SubjectCN) == "" {
		jsonMsg(c, "", fmt.Errorf("subjectCn is required"))
		return
	}
	if req.Organization == nil || strings.TrimSpace(*req.Organization) == "" {
		jsonMsg(c, "", fmt.Errorf("organization is required"))
		return
	}
	if req.Country == nil || strings.TrimSpace(*req.Country) == "" {
		jsonMsg(c, "", fmt.Errorf("country is required"))
		return
	}

	payload := &model.SelfSignedAuthority{
		Name:         strings.TrimSpace(*req.Name),
		SubjectCN:    strings.TrimSpace(*req.SubjectCN),
		Organization: strings.TrimSpace(*req.Organization),
	}
	if req.ID != nil {
		payload.Id = *req.ID
	}
	if req.PlatformCode != nil {
		payload.PlatformCode = strings.TrimSpace(*req.PlatformCode)
	}
	if req.PlatformName != nil {
		payload.PlatformName = strings.TrimSpace(*req.PlatformName)
	}
	if req.Department != nil {
		payload.Department = strings.TrimSpace(*req.Department)
	}
	if req.Country != nil {
		payload.Country = strings.TrimSpace(*req.Country)
	}
	if req.Province != nil {
		payload.Province = strings.TrimSpace(*req.Province)
	}
	if req.City != nil {
		payload.City = strings.TrimSpace(*req.City)
	}
	if req.KeyAlgorithm != nil {
		payload.KeyAlgorithm = strings.TrimSpace(*req.KeyAlgorithm)
	}
	if req.IssuerName != nil {
		payload.IssuerName = strings.TrimSpace(*req.IssuerName)
	}
	if req.IssuerOrg != nil {
		payload.IssuerOrg = strings.TrimSpace(*req.IssuerOrg)
	}
	if req.CAURL != nil {
		payload.CAURL = strings.TrimSpace(*req.CAURL)
	}
	if req.OCSPURL != nil {
		payload.OCSPURL = strings.TrimSpace(*req.OCSPURL)
	}
	if req.CRLURL != nil {
		payload.CRLURL = strings.TrimSpace(*req.CRLURL)
	}
	if req.KeyUsage != nil {
		payload.KeyUsage = strings.TrimSpace(*req.KeyUsage)
	}
	if req.ExtKeyUsage != nil {
		payload.ExtKeyUsage = strings.TrimSpace(*req.ExtKeyUsage)
	}
	if req.SignAlgo != nil {
		payload.SignAlgo = strings.TrimSpace(*req.SignAlgo)
	}
	if req.Brand != nil {
		payload.Brand = strings.TrimSpace(*req.Brand)
	}
	if req.Notes != nil {
		payload.Notes = strings.TrimSpace(*req.Notes)
	}

	result, err := a.SelfSignedService.SaveAuthority(payload)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, result, nil)
}

func (a *ApiService) DeleteSelfSignedAuthority(c *gin.Context) {
	req := selfSignedDeleteAuthorityRequest{}
	if err := c.ShouldBind(&req); err != nil {
		jsonMsg(c, "", fmt.Errorf("invalid request body: %w", err))
		return
	}
	if req.ID == nil || *req.ID == 0 {
		jsonMsg(c, "", fmt.Errorf("id is required"))
		return
	}
	result, err := a.SelfSignedService.DeleteAuthority(*req.ID)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, result, nil)
}

func (a *ApiService) GetPortForwardOverview(c *gin.Context) {
	overview, err := a.PortForwardService.GetOverview()
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, overview, nil)
}

func (a *ApiService) SavePortForwardRule(c *gin.Context) {
	req := service.PortForwardRulePayload{}
	if err := c.ShouldBind(&req); err != nil {
		jsonMsg(c, "", fmt.Errorf("invalid request body: %w", err))
		return
	}
	if err := a.PortForwardService.UpsertRule(req); err != nil {
		jsonMsg(c, "", err)
		return
	}
	a.GetPortForwardOverview(c)
}

func (a *ApiService) DeletePortForwardRule(c *gin.Context) {
	req := firewallRuleDeleteRequest{}
	if err := c.ShouldBind(&req); err != nil {
		jsonMsg(c, "", fmt.Errorf("invalid request body: %w", err))
		return
	}
	if req.ID == nil || *req.ID == 0 {
		jsonMsg(c, "", fmt.Errorf("id is required"))
		return
	}
	if err := a.PortForwardService.DeleteRule(*req.ID); err != nil {
		jsonMsg(c, "", err)
		return
	}
	a.GetPortForwardOverview(c)
}

func (a *ApiService) GetReverseProxyOverview(c *gin.Context) {
	overview, err := a.ReverseProxyService.GetOverview()
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, overview, nil)
}

func (a *ApiService) SaveReverseProxyRule(c *gin.Context) {
	req := service.ReverseProxyRulePayload{}
	if err := c.ShouldBind(&req); err != nil {
		jsonMsg(c, "", fmt.Errorf("invalid request body: %w", err))
		return
	}
	if err := a.ReverseProxyService.UpsertRule(req); err != nil {
		jsonMsg(c, "", err)
		return
	}
	a.GetReverseProxyOverview(c)
}

func (a *ApiService) DeleteReverseProxyRule(c *gin.Context) {
	req := firewallRuleDeleteRequest{}
	if err := c.ShouldBind(&req); err != nil {
		jsonMsg(c, "", fmt.Errorf("invalid request body: %w", err))
		return
	}
	if req.ID == nil || *req.ID == 0 {
		jsonMsg(c, "", fmt.Errorf("id is required"))
		return
	}
	if err := a.ReverseProxyService.DeleteRule(*req.ID); err != nil {
		jsonMsg(c, "", err)
		return
	}
	a.GetReverseProxyOverview(c)
}

func (a *ApiService) ReorderReverseProxyRules(c *gin.Context) {
	req := reverseProxyReorderRequest{}
	if err := c.ShouldBind(&req); err != nil {
		jsonMsg(c, "", fmt.Errorf("invalid request body: %w", err))
		return
	}
	if len(req.IDs) == 0 {
		jsonMsg(c, "", fmt.Errorf("ids are required"))
		return
	}
	if err := a.ReverseProxyService.ReorderRules(service.ReverseProxyRuleReorderPayload{
		IDs: req.IDs,
	}); err != nil {
		jsonMsg(c, "", err)
		return
	}
	a.GetReverseProxyOverview(c)
}

func (a *ApiService) GetKernelOverview(c *gin.Context) {
	provider := strings.TrimSpace(c.Query("provider"))
	if provider == "" {
		jsonMsg(c, "", fmt.Errorf("provider is required"))
		return
	}
	overview, err := a.KernelManagerService.GetOverview(provider)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, overview, nil)
}

func (a *ApiService) GetKernelVersions(c *gin.Context) {
	provider := strings.TrimSpace(c.Query("provider"))
	line := strings.TrimSpace(c.Query("line"))
	if provider == "" {
		jsonMsg(c, "", fmt.Errorf("provider is required"))
		return
	}
	if provider == "xanmod" && line == "" {
		jsonMsg(c, "", fmt.Errorf("line is required"))
		return
	}
	result, err := a.KernelManagerService.GetVersions(provider, line)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, result, nil)
}

func (a *ApiService) GetKernelArches(c *gin.Context) {
	provider := strings.TrimSpace(c.Query("provider"))
	line := strings.TrimSpace(c.Query("line"))
	version := strings.TrimSpace(c.Query("version"))
	if provider == "" {
		jsonMsg(c, "", fmt.Errorf("provider is required"))
		return
	}
	if provider == "xanmod" && line == "" {
		jsonMsg(c, "", fmt.Errorf("line is required"))
		return
	}
	if version == "" {
		jsonMsg(c, "", fmt.Errorf("version is required"))
		return
	}

	result, err := a.KernelManagerService.GetArches(provider, line, version)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, result, nil)
}

func (a *ApiService) GetKernelPackages(c *gin.Context) {
	provider := strings.TrimSpace(c.Query("provider"))
	line := strings.TrimSpace(c.Query("line"))
	version := strings.TrimSpace(c.Query("version"))
	arch := strings.TrimSpace(c.Query("arch"))
	if provider == "" {
		jsonMsg(c, "", fmt.Errorf("provider is required"))
		return
	}
	if provider == "xanmod" && line == "" {
		jsonMsg(c, "", fmt.Errorf("line is required"))
		return
	}
	if version == "" {
		jsonMsg(c, "", fmt.Errorf("version is required"))
		return
	}
	if provider == "xanmod" && arch == "" {
		jsonMsg(c, "", fmt.Errorf("arch is required"))
		return
	}

	result, err := a.KernelManagerService.GetPackages(provider, line, version, arch)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, result, nil)
}

func (a *ApiService) GetKernelCleanupScan(c *gin.Context) {
	result, err := a.KernelManagerService.ScanCleanupPackages()
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, result, nil)
}

func (a *ApiService) PurgeKernelCleanupPackages(c *gin.Context) {
	req := kernelCleanupPurgeRequest{}
	if err := c.ShouldBind(&req); err != nil {
		jsonMsg(c, "", fmt.Errorf("invalid request body: %w", err))
		return
	}
	if len(req.Packages) == 0 {
		jsonMsg(c, "", fmt.Errorf("packages are required"))
		return
	}
	result, err := a.KernelManagerService.PurgePackages(req.Packages)
	jsonObj(c, result, err)
}

func (a *ApiService) AutoCleanupKernelPackages(c *gin.Context) {
	result, err := a.KernelManagerService.AutoCleanupPackages()
	jsonObj(c, result, err)
}

func (a *ApiService) SaveKernelCleanupMarker(c *gin.Context) {
	req := kernelCleanupMarkerRequest{}
	if err := c.ShouldBind(&req); err != nil {
		jsonMsg(c, "", fmt.Errorf("invalid request body: %w", err))
		return
	}
	if req.Kernel == nil || strings.TrimSpace(*req.Kernel) == "" {
		jsonMsg(c, "", fmt.Errorf("kernel is required"))
		return
	}
	if err := a.KernelManagerService.SetPinnedKernel(*req.Kernel); err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, map[string]string{"pinnedKernel": strings.TrimSpace(*req.Kernel)}, nil)
}

func (a *ApiService) DownloadKernelPackages(c *gin.Context) {
	req := kernelActionRequest{}
	if err := c.ShouldBind(&req); err != nil {
		jsonMsg(c, "", fmt.Errorf("invalid request body: %w", err))
		return
	}
	provider := ""
	if req.Provider != nil {
		provider = strings.TrimSpace(*req.Provider)
	}
	if provider == "" {
		jsonMsg(c, "", fmt.Errorf("provider is required"))
		return
	}
	if provider == "xanmod" && (req.Line == nil || strings.TrimSpace(*req.Line) == "") {
		jsonMsg(c, "", fmt.Errorf("line is required"))
		return
	}
	if req.Version == nil || strings.TrimSpace(*req.Version) == "" {
		jsonMsg(c, "", fmt.Errorf("version is required"))
		return
	}
	if provider == "xanmod" && (req.Arch == nil || strings.TrimSpace(*req.Arch) == "") {
		jsonMsg(c, "", fmt.Errorf("arch is required"))
		return
	}

	line := ""
	if req.Line != nil {
		line = strings.TrimSpace(*req.Line)
	}
	arch := ""
	if req.Arch != nil {
		arch = strings.TrimSpace(*req.Arch)
	}
	downloadSessionID := ""
	if req.DownloadSessionID != nil {
		downloadSessionID = strings.TrimSpace(*req.DownloadSessionID)
	}
	result, err := a.KernelManagerService.DownloadPackages(provider, line, *req.Version, arch, downloadSessionID)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, result, nil)
}

func (a *ApiService) GetKernelDownloadProgress(c *gin.Context) {
	id := strings.TrimSpace(c.Query("id"))
	if id == "" {
		jsonMsg(c, "", fmt.Errorf("id is required"))
		return
	}
	progress := a.KernelManagerService.GetDownloadProgress(id)
	jsonObj(c, progress, nil)
}

func (a *ApiService) InstallKernelPackages(c *gin.Context) {
	req := kernelActionRequest{}
	if err := c.ShouldBind(&req); err != nil {
		jsonMsg(c, "", fmt.Errorf("invalid request body: %w", err))
		return
	}
	provider := ""
	if req.Provider != nil {
		provider = strings.TrimSpace(*req.Provider)
	}
	if provider == "" {
		jsonMsg(c, "", fmt.Errorf("provider is required"))
		return
	}
	if provider == "xanmod" && (req.Line == nil || strings.TrimSpace(*req.Line) == "") {
		jsonMsg(c, "", fmt.Errorf("line is required"))
		return
	}
	if req.Version == nil || strings.TrimSpace(*req.Version) == "" {
		jsonMsg(c, "", fmt.Errorf("version is required"))
		return
	}
	if provider == "xanmod" && (req.Arch == nil || strings.TrimSpace(*req.Arch) == "") {
		jsonMsg(c, "", fmt.Errorf("arch is required"))
		return
	}

	line := ""
	if req.Line != nil {
		line = strings.TrimSpace(*req.Line)
	}
	arch := ""
	if req.Arch != nil {
		arch = strings.TrimSpace(*req.Arch)
	}
	result, err := a.KernelManagerService.InstallDownloadedPackages(provider, line, *req.Version, arch)
	jsonObj(c, result, err)
}

func (a *ApiService) RebootKernelHost(c *gin.Context) {
	if err := a.KernelManagerService.RebootSystem(); err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, map[string]bool{"rebooting": true}, nil)
}

func (a *ApiService) ClearDownloadedKernel(c *gin.Context) {
	result, err := a.KernelManagerService.ClearDownloadedKernel()
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, result, nil)
}

func (a *ApiService) GetOnlines(c *gin.Context) {
	onlines, err := a.StatsService.GetOnlines()
	jsonObj(c, onlines, err)
}

func (a *ApiService) GetLogs(c *gin.Context) {
	count := c.Query("c")
	level := c.Query("l")
	logs := a.ServerService.GetLogs(count, level)
	jsonObj(c, logs, nil)
}

func (a *ApiService) CheckChanges(c *gin.Context) {
	actor := c.Query("a")
	chngKey := c.Query("k")
	count := c.Query("c")
	changes := a.ConfigService.GetChanges(actor, chngKey, count)
	jsonObj(c, changes, nil)
}

func (a *ApiService) GetKeypairs(c *gin.Context) {
	kType := c.Query("k")
	options := c.Query("o")
	templateCode := c.Query("template")
	if strings.EqualFold(strings.TrimSpace(kType), "tls") && strings.TrimSpace(templateCode) != "" && !service.IsKnownTLSSelfSignedTemplateCode(templateCode) {
		jsonMsg(c, "", fmt.Errorf("unknown tls self-signed template: %s", strings.TrimSpace(templateCode)))
		return
	}
	keypair := a.ServerService.GenKeypairWithTemplate(kType, options, templateCode)
	if len(keypair) == 1 {
		line := strings.TrimSpace(keypair[0])
		lowerLine := strings.ToLower(line)
		if strings.HasPrefix(lowerLine, "failed to generate ") || lowerLine == "no keypair to generate" || strings.HasPrefix(lowerLine, "failed to generate tls keypair:") {
			jsonMsg(c, "", fmt.Errorf("%s", line))
			return
		}
	}
	jsonObj(c, keypair, nil)
}

func (a *ApiService) GetTLSSelfSignedTemplates(c *gin.Context) {
	jsonObj(c, service.ListTLSSelfSignedTemplateOptions(), nil)
}

func (a *ApiService) GenerateTLSSha256(c *gin.Context) {
	req := tlsSha256Request{}
	if err := c.ShouldBind(&req); err != nil {
		jsonMsg(c, "", err)
		return
	}

	sha256, err := a.ServerService.GenerateTLSPublicKeySHA256(req.SourceType, req.CertificatePath, req.CertificatePEM)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, sha256, nil)
}

func (a *ApiService) GenerateTLSFingerprint(c *gin.Context) {
	req := tlsSha256Request{}
	if err := c.ShouldBind(&req); err != nil {
		jsonMsg(c, "", err)
		return
	}

	fingerprint, err := a.ServerService.GenerateTLSCertificateFingerprint(req.SourceType, req.CertificatePath, req.CertificatePEM)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, fingerprint, nil)
}

func (a *ApiService) GenerateTLSCertAlgorithm(c *gin.Context) {
	req := tlsSha256Request{}
	if err := c.ShouldBind(&req); err != nil {
		jsonMsg(c, "", err)
		return
	}

	algorithmInfo, err := a.ServerService.DetectTLSCertificateAlgorithm(req.SourceType, req.CertificatePath, req.CertificatePEM)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, algorithmInfo, nil)
}

func (a *ApiService) DetectTLSSelfSignedTemplate(c *gin.Context) {
	req := tlsSha256Request{}
	if err := c.ShouldBind(&req); err != nil {
		jsonMsg(c, "", err)
		return
	}

	templateCode, err := a.ServerService.DetectTLSSelfSignedTemplate(req.SourceType, req.CertificatePath, req.CertificatePEM)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, map[string]string{
		"template_code": templateCode,
	}, nil)
}

func (a *ApiService) GetDb(c *gin.Context) {
	exclude := c.Query("exclude")
	db, err := database.GetDb(exclude)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	c.Header("Content-Type", "application/octet-stream")
	c.Header("Content-Disposition", "attachment; filename=kwor_"+time.Now().Format("20060102-150405")+".db")
	c.Writer.Write(db)
}

func (a *ApiService) postActions(c *gin.Context) (string, json.RawMessage, error) {
	var data map[string]json.RawMessage
	err := c.ShouldBind(&data)
	if err != nil {
		return "", nil, err
	}
	return string(data["action"]), data["data"], nil
}

func (a *ApiService) Login(c *gin.Context) {
	remoteIP := getRemoteIp(c)
	loginUser, err := a.UserService.Login(c.Request.FormValue("user"), c.Request.FormValue("pass"), remoteIP)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}

	sessionMaxAge, err := a.SettingService.GetSessionMaxAge()
	if err != nil {
		logger.Infof("Unable to get session's max age from DB")
	}

	err = SetLoginUser(c, loginUser, sessionMaxAge)
	if err == nil {
		logger.Info("user ", loginUser, " login success")
	} else {
		logger.Warning("login failed: ", err)
	}

	warning := strings.TrimSpace(service.GetLoginWarning())
	if warning == "" {
		jsonMsg(c, "", nil)
		return
	}
	jsonObj(c, map[string]string{
		"warning": warning,
	}, nil)
}

func (a *ApiService) Session(c *gin.Context) {
	pureJsonMsg(c, IsLogin(c), "")
}

func (a *ApiService) ChangePass(c *gin.Context) {
	id := c.Request.FormValue("id")
	oldPass := c.Request.FormValue("oldPass")
	newUsername := c.Request.FormValue("newUsername")
	newPass := c.Request.FormValue("newPass")
	err := a.UserService.ChangePass(id, oldPass, newUsername, newPass)
	if err == nil {
		logger.Info("change user credentials success")
		jsonMsg(c, "save", nil)
	} else {
		logger.Warning("change user credentials failed:", err)
		jsonMsg(c, "", err)
	}
}

func (a *ApiService) Save(c *gin.Context, loginUser string) {
	hostname := getHostname(c)
	obj := c.Request.FormValue("object")
	act := c.Request.FormValue("action")
	data := c.Request.FormValue("data")
	initUsers := c.Request.FormValue("initUsers")
	objs, err := a.ConfigService.Save(obj, act, json.RawMessage(data), initUsers, loginUser, hostname)
	if err != nil {
		jsonMsg(c, "save", err)
		return
	}
	if strings.HasPrefix(obj, "mihomo_") {
		err = a.LoadMihomoPartialData(c, objs)
	} else {
		err = a.LoadPartialData(c, objs)
	}
	if err != nil {
		jsonMsg(c, obj, err)
	}
}

func (a *ApiService) RestartApp(c *gin.Context) {
	err := a.PanelService.RestartPanel(3 * time.Second)
	jsonMsg(c, "restartApp", err)
}

func (a *ApiService) RestartSb(c *gin.Context) {
	err := a.ConfigService.RestartCore()
	jsonMsg(c, "restartSb", err)
}

func (a *ApiService) LinkConvert(c *gin.Context) {
	link := c.Request.FormValue("link")
	result, _, err := util.GetOutbound(link, 0)
	jsonObj(c, result, err)
}

func (a *ApiService) ImportDb(c *gin.Context) {
	file, _, err := c.Request.FormFile("db")
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	defer file.Close()
	err = database.ImportDB(file)
	jsonMsg(c, "", err)
}

func (a *ApiService) Logout(c *gin.Context) {
	loginUser := GetLoginUser(c)
	if loginUser != "" {
		logger.Infof("user %s logout", loginUser)
	}
	ClearSession(c)
	jsonMsg(c, "", nil)
}

func (a *ApiService) LoadTokens() ([]byte, error) {
	return a.UserService.LoadTokens()
}

func (a *ApiService) GetTokens(c *gin.Context) {
	loginUser := GetLoginUser(c)
	tokens, err := a.UserService.GetUserTokens(loginUser)
	jsonObj(c, tokens, err)
}

func (a *ApiService) AddToken(c *gin.Context) {
	loginUser := GetLoginUser(c)
	expiry := c.Request.FormValue("expiry")
	expiryInt, err := strconv.ParseInt(expiry, 10, 64)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	desc := c.Request.FormValue("desc")
	token, err := a.UserService.AddToken(loginUser, expiryInt, desc)
	jsonObj(c, token, err)
}

func (a *ApiService) DeleteToken(c *gin.Context) {
	tokenId := c.Request.FormValue("id")
	err := a.UserService.DeleteToken(tokenId)
	jsonMsg(c, "", err)
}

func (a *ApiService) SyncToSubManager(c *gin.Context) {
	clientName := c.Request.FormValue("name")
	if clientName == "" {
		jsonMsg(c, "", fmt.Errorf("client name is required"))
		return
	}
	hostname := getHostname(c)
	result, err := a.SyncService.SyncClientToSubManager(clientName, hostname)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	client := &model.Client{}
	if err := database.GetDB().Model(model.Client{}).Where("name = ?", clientName).First(client).Error; err == nil {
		if markErr := a.SettingService.SetSubManagerAutoSyncClient(client.Id, true); markErr != nil {
			logger.Warning("set default auto sync client failed: ", markErr)
		}
	} else if !database.IsNotFound(err) {
		logger.Warning("load default client for auto sync marker failed: ", err)
	}
	jsonObj(c, result, nil)
}

func (a *ApiService) SyncMihomoToSubManager(c *gin.Context) {
	clientName := c.Request.FormValue("name")
	if clientName == "" {
		jsonMsg(c, "", fmt.Errorf("mihomo client name is required"))
		return
	}
	hostname := getHostname(c)
	result, err := a.MihomoSyncService.SyncClientToSubManager(clientName, hostname)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	client := &model.MihomoClient{}
	if err := database.GetDB().Model(model.MihomoClient{}).Where("name = ?", clientName).First(client).Error; err == nil {
		if markErr := a.SettingService.SetSubManagerAutoSyncMihomoClient(client.Id, true); markErr != nil {
			logger.Warning("set mihomo auto sync client failed: ", markErr)
		}
	} else if !database.IsNotFound(err) {
		logger.Warning("load mihomo client for auto sync marker failed: ", err)
	}
	jsonObj(c, result, nil)
}

func (a *ApiService) GetSingboxConfig(c *gin.Context) {
	config, err := a.ConfigService.GetConfig("")
	if err != nil {
		c.Status(400)
		c.Writer.WriteString(err.Error())
		return
	}
	rawConfig, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		c.Status(400)
		c.Writer.WriteString(err.Error())
		return
	}
	c.Header("Content-Type", "application/json")
	c.Header("Content-Disposition", "attachment; filename=config_"+time.Now().Format("20060102-150405")+".json")
	c.Writer.Write(rawConfig)
}

// GetServerIPs returns available public IPs of the server.
// Query parameter verify=true fetches real outbound IPs from external APIs
// (recommended, NAT-friendly). Otherwise only local interface public IPs are returned.
func (a *ApiService) GetServerIPs(c *gin.Context) {
	verify := c.Query("verify")

	var ips []string
	if verify == "true" {
		// Fetch real outbound IPs from external APIs (supports NAT scenarios).
		ips = a.IPDetectService.GetOutboundIPs()
	} else {
		// Use local interface public IPs only (not NAT-aware).
		ips = a.IPDetectService.GetAllAvailableIPs()
	}

	// Fallback to the default outbound IP when no result is found.
	if len(ips) == 0 {
		defaultIP, ok := a.IPDetectService.GetDefaultOutboundIP()
		if ok {
			ips = []string{defaultIP}
		}
	}

	jsonObj(c, ips, nil)
}

// GetInboundIPs returns server IPs from outbound configs of selected inbounds.
// Query parameter ids is a comma-separated inbound ID list.
func (a *ApiService) GetInboundIPs(c *gin.Context) {
	ids := c.Query("ids")
	ips, err := a.InboundService.GetOutJsonIPs(ids)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, ips, nil)
}

func (a *ApiService) GetMihomoInboundIPs(c *gin.Context) {
	ids := c.Query("ids")
	ips, err := a.MihomoInboundService.GetOutJsonIPs(ids)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, ips, nil)
}

func (a *ApiService) CheckPortOccupancy(c *gin.Context) {
	req, err := bindPortCheckRequest(c)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}

	resp, err := a.PortCheckService.Check(req)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, resp, nil)
}

// FetchSubscription downloads subscription JSON from URL and saves it to sub_json.
func (a *ApiService) FetchSubscription(c *gin.Context) {
	groupName := c.Request.FormValue("group_name")
	jsonURL := strings.TrimSpace(c.Request.FormValue("json_url"))
	clashURL := strings.TrimSpace(c.Request.FormValue("clash_url"))
	if jsonURL == "" && clashURL == "" {
		jsonURL = strings.TrimSpace(c.Request.FormValue("url"))
	}
	allowInsecure := c.Request.FormValue("allow_insecure") == "true"

	if groupName == "" || (jsonURL == "" && clashURL == "") {
		jsonMsg(c, "", fmt.Errorf("group_name and at least one subscription url are required"))
		return
	}

	err := a.SubGroupService.FetchAndSaveSubscriptionSources(groupName, jsonURL, clashURL, allowInsecure)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}

	data, err := a.getSubGroupData()
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, data, nil)
}

// RefreshSubscription re-downloads subscription JSON, diffs node changes, and updates data.
func (a *ApiService) RefreshSubscription(c *gin.Context) {
	groupName := c.Request.FormValue("group_name")
	jsonURL := strings.TrimSpace(c.Request.FormValue("json_url"))
	clashURL := strings.TrimSpace(c.Request.FormValue("clash_url"))
	if jsonURL == "" && clashURL == "" {
		jsonURL = strings.TrimSpace(c.Request.FormValue("url"))
	}
	allowInsecure := c.Request.FormValue("allow_insecure") == "true"

	if groupName == "" || (jsonURL == "" && clashURL == "") {
		jsonMsg(c, "", fmt.Errorf("group_name and at least one subscription url are required"))
		return
	}

	result, err := a.SubGroupService.RefreshSubscriptionSources(groupName, jsonURL, clashURL, allowInsecure)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}

	data, err := a.getSubGroupData()
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	data["result"] = result
	jsonObj(c, data, nil)
}

func (a *ApiService) ClearSubManager(c *gin.Context) {
	result, err := a.SubGroupService.ClearSubManagerData()
	if err != nil {
		jsonMsg(c, "", err)
		return
	}

	data, err := a.getSubGroupData()
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	data["result"] = result
	jsonObj(c, data, nil)
}

func (a *ApiService) getSubGroupData() (map[string]interface{}, error) {
	subOutbounds, err := a.SubOutboundService.GetAll()
	if err != nil {
		return nil, err
	}
	subGroups, err := a.SubGroupService.GetAll()
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"suboutbounds": subOutbounds,
		"subgroups":    subGroups,
	}, nil
}

func (a *ApiService) getOutboundGroupData() (map[string]interface{}, error) {
	outbounds, err := a.OutboundService.GetAll()
	if err != nil {
		return nil, err
	}
	outboundGroups, err := a.OutboundGroupService.GetAll()
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"outbounds":      outbounds,
		"outboundgroups": outboundGroups,
	}, nil
}

func (a *ApiService) getMihomoOutboundGroupData() (map[string]interface{}, error) {
	outbounds, err := a.MihomoOutboundService.GetAll()
	if err != nil {
		return nil, err
	}
	outboundGroups, err := a.MihomoOutboundGroupService.GetAll()
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{
		"outbounds":      outbounds,
		"outboundgroups": outboundGroups,
	}, nil
}

// FetchOutboundSubscription imports subscription nodes into outbounds.
func (a *ApiService) FetchOutboundSubscription(c *gin.Context) {
	groupName := c.Request.FormValue("group_name")
	url := c.Request.FormValue("url")
	allowInsecure := c.Request.FormValue("allow_insecure") == "true"

	if groupName == "" || url == "" {
		jsonMsg(c, "", fmt.Errorf("group_name and url are required"))
		return
	}

	if err := a.OutboundGroupService.FetchAndSaveSubscription(groupName, url, allowInsecure); err != nil {
		jsonMsg(c, "", err)
		return
	}

	data, err := a.getOutboundGroupData()
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, data, nil)
}

// RefreshOutboundSubscription refreshes imported nodes for outbound group.
func (a *ApiService) RefreshOutboundSubscription(c *gin.Context) {
	groupName := c.Request.FormValue("group_name")
	url := c.Request.FormValue("url")
	allowInsecure := c.Request.FormValue("allow_insecure") == "true"

	if groupName == "" || url == "" {
		jsonMsg(c, "", fmt.Errorf("group_name and url are required"))
		return
	}

	result, err := a.OutboundGroupService.RefreshSubscription(groupName, url, allowInsecure)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}

	data, err := a.getOutboundGroupData()
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	data["result"] = result
	jsonObj(c, data, nil)
}

func (a *ApiService) FetchMihomoOutboundSubscription(c *gin.Context) {
	groupName := c.Request.FormValue("group_name")
	url := c.Request.FormValue("url")
	allowInsecure := c.Request.FormValue("allow_insecure") == "true"

	if groupName == "" || url == "" {
		jsonMsg(c, "", fmt.Errorf("group_name and url are required"))
		return
	}

	if err := a.MihomoOutboundGroupService.FetchAndSaveSubscription(groupName, url, allowInsecure); err != nil {
		jsonMsg(c, "", err)
		return
	}

	data, err := a.getMihomoOutboundGroupData()
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, data, nil)
}

func (a *ApiService) RefreshMihomoOutboundSubscription(c *gin.Context) {
	groupName := c.Request.FormValue("group_name")
	url := c.Request.FormValue("url")
	allowInsecure := c.Request.FormValue("allow_insecure") == "true"

	if groupName == "" || url == "" {
		jsonMsg(c, "", fmt.Errorf("group_name and url are required"))
		return
	}

	result, err := a.MihomoOutboundGroupService.RefreshSubscription(groupName, url, allowInsecure)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}

	data, err := a.getMihomoOutboundGroupData()
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	data["result"] = result
	jsonObj(c, data, nil)
}

func parsePanelVersionWindowQuery(c *gin.Context) (int, int) {
	offset := 0
	limit := 5

	if offsetRaw := strings.TrimSpace(c.Query("offset")); offsetRaw != "" {
		if parsed, err := strconv.Atoi(offsetRaw); err == nil && parsed >= 0 {
			offset = parsed
		}
		if limitRaw := strings.TrimSpace(c.Query("limit")); limitRaw != "" {
			if parsed, err := strconv.Atoi(limitRaw); err == nil && parsed > 0 {
				limit = parsed
			}
		} else if perPageRaw := strings.TrimSpace(c.Query("per_page")); perPageRaw != "" {
			if parsed, err := strconv.Atoi(perPageRaw); err == nil && parsed > 0 {
				limit = parsed
			}
		}
		return offset, limit
	}

	page := 1
	if pageRaw := strings.TrimSpace(c.Query("page")); pageRaw != "" {
		if parsed, err := strconv.Atoi(pageRaw); err == nil && parsed > 0 {
			page = parsed
		}
	}
	if limitRaw := strings.TrimSpace(c.Query("limit")); limitRaw != "" {
		if parsed, err := strconv.Atoi(limitRaw); err == nil && parsed > 0 {
			limit = parsed
		}
	} else if perPageRaw := strings.TrimSpace(c.Query("per_page")); perPageRaw != "" {
		if parsed, err := strconv.Atoi(perPageRaw); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	offset = (page - 1) * limit
	return offset, limit
}

func (a *ApiService) GetPanelUpdateStatus(c *gin.Context) {
	status, err := a.PanelUpdateService.GetStatus()
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, status, nil)
}

func (a *ApiService) GetPanelUpdateVersions(c *gin.Context) {
	offset, limit := parsePanelVersionWindowQuery(c)
	result, err := a.PanelUpdateService.GetRemoteVersions(offset, limit)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, result, nil)
}

func (a *ApiService) GetPanelUpdateLog(c *gin.Context) {
	result, err := a.PanelUpdateService.GetLastUpdateLog()
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, result, nil)
}

func (a *ApiService) InstallPanelUpdate(c *gin.Context) {
	version := strings.TrimSpace(c.Request.FormValue("version"))
	if version == "" {
		req := panelUpdateInstallRequest{}
		if err := c.ShouldBind(&req); err == nil {
			version = strings.TrimSpace(req.Version)
		}
	}

	result, err := a.PanelUpdateService.Install(version)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, result, nil)
}

// === CoreManager API ===

// GetCoreStatus returns core status (local version and running state).
func (a *ApiService) GetCoreManagerStatus(c *gin.Context) {
	info, err := a.CoreManagerService.GetCoreStatus()
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, info, nil)
}

func parseCoreVersionWindowQuery(c *gin.Context) (string, int, int, service.CoreDownloadTarget) {
	channel := strings.TrimSpace(c.Query("channel"))
	if channel == "" {
		channel = "stable"
	}

	offset := 0
	limit := 5

	if offsetRaw := strings.TrimSpace(c.Query("offset")); offsetRaw != "" {
		if parsed, err := strconv.Atoi(offsetRaw); err == nil && parsed >= 0 {
			offset = parsed
		}
		if limitRaw := strings.TrimSpace(c.Query("limit")); limitRaw != "" {
			if parsed, err := strconv.Atoi(limitRaw); err == nil && parsed > 0 {
				limit = parsed
			}
		} else if perPageRaw := strings.TrimSpace(c.Query("per_page")); perPageRaw != "" {
			if parsed, err := strconv.Atoi(perPageRaw); err == nil && parsed > 0 {
				limit = parsed
			}
		}
	} else {
		page := 1
		if pageRaw := strings.TrimSpace(c.Query("page")); pageRaw != "" {
			if parsed, err := strconv.Atoi(pageRaw); err == nil && parsed > 0 {
				page = parsed
			}
		}
		if perPageRaw := strings.TrimSpace(c.Query("per_page")); perPageRaw != "" {
			if parsed, err := strconv.Atoi(perPageRaw); err == nil && parsed > 0 {
				limit = parsed
			}
		}
		offset = (page - 1) * limit
	}

	target := service.CoreDownloadTarget{
		OS:         strings.TrimSpace(c.Query("target_os")),
		Arch:       strings.TrimSpace(c.Query("target_arch")),
		Libc:       strings.TrimSpace(c.Query("target_libc")),
		Amd64Level: strings.TrimSpace(c.Query("target_amd64_level")),
	}

	return channel, offset, limit, target
}

// GetCoreRemoteVersions returns available remote core versions.
func (a *ApiService) GetCoreRemoteVersions(c *gin.Context) {
	channel, offset, limit, target := parseCoreVersionWindowQuery(c)
	result, err := a.CoreManagerService.GetRemoteVersionsWindow(channel, offset, limit, target)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, result, nil)
}

func parseCoreIntervalHours(raw string) (int, error) {
	trimmed := strings.TrimSpace(strings.ToLower(raw))
	trimmed = strings.TrimSuffix(trimmed, "h")
	trimmed = strings.TrimSpace(trimmed)
	if trimmed == "" {
		return 0, fmt.Errorf("interval is required")
	}
	intervalHours, err := strconv.Atoi(trimmed)
	if err != nil || intervalHours <= 0 {
		return 0, fmt.Errorf("interval must be a positive hour value, e.g. 12 or 12h")
	}
	return intervalHours, nil
}

func parseSubGroupIntervalMinutes(raw string) (int, error) {
	trimmed := strings.TrimSpace(strings.ToLower(raw))
	trimmed = strings.TrimSuffix(trimmed, "m")
	trimmed = strings.TrimSpace(trimmed)
	if trimmed == "" {
		return 0, fmt.Errorf("interval is required")
	}
	intervalMinutes, err := strconv.Atoi(trimmed)
	if err != nil || intervalMinutes <= 0 {
		return 0, fmt.Errorf("interval must be a positive minute value, e.g. 5 or 5m")
	}
	return intervalMinutes, nil
}

func (a *ApiService) GetSubGroupAutoUpdateInfo(c *gin.Context) {
	info, err := a.SettingService.GetSubGroupAutoUpdateInfo()
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, info, nil)
}

func (a *ApiService) SaveSubGroupAutoUpdateSettings(c *gin.Context) {
	enabledRaw := strings.TrimSpace(c.Request.FormValue("enabled"))
	enabled := strings.EqualFold(enabledRaw, "true") || enabledRaw == "1"

	intervalRaw := c.Request.FormValue("interval")
	intervalMinutes, err := parseSubGroupIntervalMinutes(intervalRaw)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}

	if err := a.SettingService.SaveSubGroupAutoUpdateSettings(enabled, intervalMinutes); err != nil {
		jsonMsg(c, "", err)
		return
	}

	info, err := a.SettingService.GetSubGroupAutoUpdateInfo()
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, info, nil)
}

// GetCoreUpdateInfo returns auto-check settings and update markers.
func (a *ApiService) GetCoreUpdateInfo(c *gin.Context) {
	forceCheck := strings.EqualFold(c.Query("force"), "true")
	info, err := a.CoreManagerService.GetCoreUpdateInfo(forceCheck)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, info, nil)
}

// SaveCoreUpdateSettings updates auto-check switch and interval.
func (a *ApiService) SaveCoreUpdateSettings(c *gin.Context) {
	enabledRaw := strings.TrimSpace(c.Request.FormValue("enabled"))
	enabled := strings.EqualFold(enabledRaw, "true") || enabledRaw == "1"

	intervalRaw := c.Request.FormValue("interval")
	intervalHours, err := parseCoreIntervalHours(intervalRaw)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}

	err = a.CoreManagerService.SetCoreAutoCheckSettings(enabled, intervalHours)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}

	if enabled {
		if checkErr := a.CoreManagerService.CheckAndMarkCoreUpdates(true); checkErr != nil {
			logger.Warning("check core updates after settings update failed: ", checkErr)
		}
	}

	info, err := a.CoreManagerService.GetCoreUpdateInfo(false)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, info, nil)
}

// AckCoreUpdateNotice clears pending update markers.
func (a *ApiService) AckCoreUpdateNotice(c *gin.Context) {
	err := a.CoreManagerService.ClearCoreUpdatePending()
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	info, err := a.CoreManagerService.GetCoreUpdateInfo(false)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, info, nil)
}

func (a *ApiService) syncNftablesWithCoreState() {
	if a.CoreManagerService.IsRunning() {
		(&service.NftTrafficService{}).InitOnStartup()
		(&service.ClientRateLimitService{}).InitOnStartup()
		(&service.ClientPortBlockService{}).InitOnStartup()
		return
	}
	(&service.NftTrafficService{}).CleanupOnShutdown()
	(&service.ClientRateLimitService{}).CleanupOnShutdown()
	(&service.ClientPortBlockService{}).CleanupOnShutdown()
}

func (a *ApiService) syncMihomoNftablesWithCoreState() {
	if a.MihomoCoreManagerService.IsRunning() {
		(&service.MihomoNftTrafficService{}).InitOnStartup()
		(&service.MihomoClientRateLimitService{}).InitOnStartup()
		(&service.MihomoClientPortBlockService{}).InitOnStartup()
		return
	}
	(&service.MihomoNftTrafficService{}).CleanupOnShutdown()
	(&service.MihomoClientRateLimitService{}).CleanupOnShutdown()
	(&service.MihomoClientPortBlockService{}).CleanupOnShutdown()
}

// DownloadCoreManager downloads the specified core version.
func (a *ApiService) DownloadCoreManager(c *gin.Context) {
	customURL := strings.TrimSpace(c.Request.FormValue("custom_url"))
	version := strings.TrimSpace(c.Request.FormValue("version"))
	targetOS := strings.TrimSpace(c.Request.FormValue("target_os"))
	targetArch := strings.TrimSpace(c.Request.FormValue("target_arch"))
	targetLibc := strings.TrimSpace(c.Request.FormValue("target_libc"))
	targetAmd64Level := strings.TrimSpace(c.Request.FormValue("target_amd64_level"))
	downloadSessionID := strings.TrimSpace(c.Request.FormValue("downloadSessionId"))

	var (
		localVer string
		err      error
	)
	if customURL != "" {
		if !strings.HasPrefix(customURL, "http://") && !strings.HasPrefix(customURL, "https://") {
			jsonMsg(c, "", fmt.Errorf("custom_url must start with http:// or https://"))
			return
		}
		localVer, err = a.CoreManagerService.DownloadCoreFromURL(customURL, downloadSessionID)
		if err == nil {
			if saveErr := a.CoreManagerService.SaveCustomDownloadURL(customURL); saveErr != nil {
				logger.Warning("save core custom download url failed: ", saveErr)
			}
		}
	} else {
		if version == "" {
			jsonMsg(c, "", fmt.Errorf("version or custom_url is required"))
			return
		}
		localVer, err = a.CoreManagerService.DownloadCore(version, service.CoreDownloadTarget{
			OS:         targetOS,
			Arch:       targetArch,
			Libc:       targetLibc,
			Amd64Level: targetAmd64Level,
		}, downloadSessionID)
	}

	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	a.syncNftablesWithCoreState()
	jsonObj(c, map[string]string{"version": localVer}, nil)
}

func (a *ApiService) SaveCoreDownloadPreference(c *gin.Context) {
	customURL := strings.TrimSpace(c.Request.FormValue("custom_url"))
	preference, err := a.CoreManagerService.GetDownloadPreference()
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	target := service.CoreDownloadTarget{
		OS:         strings.TrimSpace(c.Request.FormValue("target_os")),
		Arch:       strings.TrimSpace(c.Request.FormValue("target_arch")),
		Libc:       strings.TrimSpace(c.Request.FormValue("target_libc")),
		Amd64Level: strings.TrimSpace(c.Request.FormValue("target_amd64_level")),
	}
	if _, exists := c.Request.Form["target_os"]; exists {
		preference.Target.OS = target.OS
	}
	if _, exists := c.Request.Form["target_arch"]; exists {
		preference.Target.Arch = target.Arch
	}
	if _, exists := c.Request.Form["target_libc"]; exists {
		preference.Target.Libc = target.Libc
	}
	if _, exists := c.Request.Form["target_amd64_level"]; exists {
		preference.Target.Amd64Level = target.Amd64Level
	}
	preference.CustomURL = customURL
	err = a.CoreManagerService.SaveDownloadPreference(preference)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	preference, err = a.CoreManagerService.GetDownloadPreference()
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, preference, nil)
}

// StartCoreManager starts the core process.
func (a *ApiService) StartCoreManager(c *gin.Context) {
	err := a.CoreManagerService.StartCore()
	if err == nil {
		a.syncNftablesWithCoreState()
	}
	jsonMsg(c, "startCore", err)
}

// StopCoreManager stops the core process.
func (a *ApiService) StopCoreManager(c *gin.Context) {
	err := a.CoreManagerService.StopCore()
	if err == nil {
		a.syncNftablesWithCoreState()
	}
	jsonMsg(c, "stopCore", err)
}

// RestartCoreManager restarts the core process.
func (a *ApiService) RestartCoreManager(c *gin.Context) {
	err := a.CoreManagerService.RestartCore()
	if err == nil {
		a.syncNftablesWithCoreState()
	}
	jsonMsg(c, "restartCore", err)
}

func (a *ApiService) DeleteCoreManager(c *gin.Context) {
	err := a.CoreManagerService.DeleteCore()
	if err == nil {
		a.syncNftablesWithCoreState()
	}
	jsonMsg(c, "deleteCore", err)
}

func (a *ApiService) GetMihomoCoreManagerStatus(c *gin.Context) {
	info, err := a.MihomoCoreManagerService.GetCoreStatus()
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, info, nil)
}

func (a *ApiService) GetMihomoCoreRemoteVersions(c *gin.Context) {
	channel, offset, limit, target := parseCoreVersionWindowQuery(c)
	result, err := a.MihomoCoreManagerService.GetRemoteVersionsWindow(channel, offset, limit, target)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, result, nil)
}

func (a *ApiService) GetMihomoCoreUpdateInfo(c *gin.Context) {
	forceCheck := strings.EqualFold(c.Query("force"), "true")
	info, err := a.MihomoCoreManagerService.GetCoreUpdateInfo(forceCheck)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, info, nil)
}

func (a *ApiService) GetCoreDownloadProgress(c *gin.Context) {
	id := strings.TrimSpace(c.Query("id"))
	if id == "" {
		jsonMsg(c, "", fmt.Errorf("id is required"))
		return
	}
	progress := service.GetCoreDownloadProgress(id)
	jsonObj(c, progress, nil)
}

func (a *ApiService) SaveMihomoCoreUpdateSettings(c *gin.Context) {
	enabledRaw := strings.TrimSpace(c.Request.FormValue("enabled"))
	enabled := strings.EqualFold(enabledRaw, "true") || enabledRaw == "1"

	intervalRaw := c.Request.FormValue("interval")
	intervalHours, err := parseCoreIntervalHours(intervalRaw)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}

	err = a.MihomoCoreManagerService.SetCoreAutoCheckSettings(enabled, intervalHours)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}

	if enabled {
		if checkErr := a.MihomoCoreManagerService.CheckAndMarkCoreUpdates(true); checkErr != nil {
			logger.Warning("check mihomo core updates after settings update failed: ", checkErr)
		}
	}

	info, err := a.MihomoCoreManagerService.GetCoreUpdateInfo(false)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, info, nil)
}

func (a *ApiService) AckMihomoCoreUpdateNotice(c *gin.Context) {
	err := a.MihomoCoreManagerService.ClearCoreUpdatePending()
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	info, err := a.MihomoCoreManagerService.GetCoreUpdateInfo(false)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, info, nil)
}

func (a *ApiService) DownloadMihomoCoreManager(c *gin.Context) {
	customURL := strings.TrimSpace(c.Request.FormValue("custom_url"))
	version := strings.TrimSpace(c.Request.FormValue("version"))
	targetOS := strings.TrimSpace(c.Request.FormValue("target_os"))
	targetArch := strings.TrimSpace(c.Request.FormValue("target_arch"))
	targetAmd64Level := strings.TrimSpace(c.Request.FormValue("target_amd64_level"))
	downloadSessionID := strings.TrimSpace(c.Request.FormValue("downloadSessionId"))

	var (
		localVer string
		err      error
	)
	if customURL != "" {
		if !strings.HasPrefix(customURL, "http://") && !strings.HasPrefix(customURL, "https://") {
			jsonMsg(c, "", fmt.Errorf("custom_url must start with http:// or https://"))
			return
		}
		localVer, err = a.MihomoCoreManagerService.DownloadCoreFromURL(customURL, downloadSessionID)
		if err == nil {
			if saveErr := a.MihomoCoreManagerService.SaveCustomDownloadURL(customURL); saveErr != nil {
				logger.Warning("save mihomo custom download url failed: ", saveErr)
			}
		}
	} else {
		if version == "" {
			jsonMsg(c, "", fmt.Errorf("version or custom_url is required"))
			return
		}
		localVer, err = a.MihomoCoreManagerService.DownloadCore(version, service.CoreDownloadTarget{
			OS:         targetOS,
			Arch:       targetArch,
			Amd64Level: targetAmd64Level,
		}, downloadSessionID)
	}

	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	a.syncMihomoNftablesWithCoreState()
	jsonObj(c, map[string]string{"version": localVer}, nil)
}

func (a *ApiService) SaveMihomoCoreDownloadPreference(c *gin.Context) {
	customURL := strings.TrimSpace(c.Request.FormValue("custom_url"))
	preference, err := a.MihomoCoreManagerService.GetDownloadPreference()
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	target := service.CoreDownloadTarget{
		OS:         strings.TrimSpace(c.Request.FormValue("target_os")),
		Arch:       strings.TrimSpace(c.Request.FormValue("target_arch")),
		Amd64Level: strings.TrimSpace(c.Request.FormValue("target_amd64_level")),
	}
	if _, exists := c.Request.Form["target_os"]; exists {
		preference.Target.OS = target.OS
	}
	if _, exists := c.Request.Form["target_arch"]; exists {
		preference.Target.Arch = target.Arch
	}
	if _, exists := c.Request.Form["target_amd64_level"]; exists {
		preference.Target.Amd64Level = target.Amd64Level
	}
	preference.CustomURL = customURL
	err = a.MihomoCoreManagerService.SaveDownloadPreference(preference)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	preference, err = a.MihomoCoreManagerService.GetDownloadPreference()
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, preference, nil)
}

func (a *ApiService) StartMihomoCoreManager(c *gin.Context) {
	err := a.MihomoCoreManagerService.StartCore()
	if err == nil {
		a.syncMihomoNftablesWithCoreState()
	}
	jsonMsg(c, "startCore", err)
}

func (a *ApiService) StopMihomoCoreManager(c *gin.Context) {
	err := a.MihomoCoreManagerService.StopCore()
	if err == nil {
		a.syncMihomoNftablesWithCoreState()
	}
	jsonMsg(c, "stopCore", err)
}

func (a *ApiService) RestartMihomoCoreManager(c *gin.Context) {
	err := a.MihomoCoreManagerService.RestartCore()
	if err == nil {
		a.syncMihomoNftablesWithCoreState()
	}
	jsonMsg(c, "restartCore", err)
}

func (a *ApiService) DeleteMihomoCoreManager(c *gin.Context) {
	err := a.MihomoCoreManagerService.DeleteCore()
	if err == nil {
		a.syncMihomoNftablesWithCoreState()
	}
	jsonMsg(c, "deleteCore", err)
}
