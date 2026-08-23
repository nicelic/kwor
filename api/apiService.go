package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
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
	service.PanelUninstallService

	coreManagerOverride       *service.CoreManagerService
	mihomoCoreManagerOverride *service.MihomoCoreManagerService
}

func (a *ApiService) SetCoreManagers(coreManager *service.CoreManagerService, mihomoCoreManager *service.MihomoCoreManagerService) {
	if a == nil {
		return
	}
	a.coreManagerOverride = coreManager
	a.mihomoCoreManagerOverride = mihomoCoreManager
}

func (a *ApiService) coreManagerService() *service.CoreManagerService {
	if a != nil && a.coreManagerOverride != nil {
		return a.coreManagerOverride
	}
	return &a.CoreManagerService
}

func (a *ApiService) mihomoCoreManagerService() *service.MihomoCoreManagerService {
	if a != nil && a.mihomoCoreManagerOverride != nil {
		return a.mihomoCoreManagerOverride
	}
	return &a.MihomoCoreManagerService
}

var panelUninstallScheduler = func(uninstallService *service.PanelUninstallService) (*service.PanelUninstallResult, error) {
	return uninstallService.Schedule()
}

type tlsSha256Request struct {
	SourceType      string `json:"source_type" form:"source_type"`
	CertificatePath string `json:"certificate_path" form:"certificate_path"`
	CertificatePEM  string `json:"certificate_pem" form:"certificate_pem"`
}

type trafficOverviewSettingsRequest struct {
	LimitGiB         *float64 `json:"limit_gib" form:"limit_gib"`
	ResetDay         *int     `json:"reset_day" form:"reset_day"`
	ExpiryDate       *string  `json:"expiry_date" form:"expiry_date"`
	LimitGiBCompat   *float64 `json:"limitGiB" form:"limitGiB"`
	ResetDayCompat   *int     `json:"resetDay" form:"resetDay"`
	ExpiryDateCompat *string  `json:"expiryDate" form:"expiryDate"`
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

type firewallSwitchRequest struct {
	Enabled *bool `json:"enabled" form:"enabled"`
}

type firewallRuleDeleteRequest struct {
	ID *uint `json:"id" form:"id"`
}

type firewallGeoSettingsRequest struct {
	IntervalMinutes *int `json:"intervalMinutes" form:"intervalMinutes"`
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
	ExistingRecordID *uint   `json:"existingRecordId" form:"existingRecordId"`
	Domains          *string `json:"domains" form:"domains"`
	CertificateType  *string `json:"certificateType" form:"certificateType"`
	Challenge        *string `json:"challenge" form:"challenge"`
	Webroot          *string `json:"webroot" form:"webroot"`
	DNSProvider      *string `json:"dnsProvider" form:"dnsProvider"`
	DNSEnv           *string `json:"dnsEnv" form:"dnsEnv"`
	Server           *string `json:"server" form:"server"`
	KeyLength        *string `json:"keyLength" form:"keyLength"`
	CustomArgs       *string `json:"customArgs" form:"customArgs"`
	AcmeAccountID    *uint   `json:"acmeAccountId" form:"acmeAccountId"`
	DNSAccountID     *uint   `json:"dnsAccountId" form:"dnsAccountId"`
	AutoRenew        *bool   `json:"autoRenew" form:"autoRenew"`
	Remark           *string `json:"remark" form:"remark"`
	ApplyTarget      *string `json:"applyTarget" form:"applyTarget"`
	PushDir          *string `json:"pushDir" form:"pushDir"`
	LogSessionID     *string `json:"logSessionId" form:"logSessionId"`
}

type acmeRenewRequest struct {
	ID           *uint   `json:"id" form:"id"`
	Force        *bool   `json:"force" form:"force"`
	ApplyTarget  *string `json:"applyTarget" form:"applyTarget"`
	PushDir      *string `json:"pushDir" form:"pushDir"`
	LogSessionID *string `json:"logSessionId" form:"logSessionId"`
}

type acmePushRequest struct {
	ID        *uint   `json:"id" form:"id"`
	TargetDir *string `json:"targetDir" form:"targetDir"`
	Clear     *bool   `json:"clear" form:"clear"`
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
	ID               *uint   `json:"id" form:"id"`
	Name             *string `json:"name" form:"name"`
	Email            *string `json:"email" form:"email"`
	Server           *string `json:"server" form:"server"`
	AccountKeyLength *string `json:"accountKeyLength" form:"accountKeyLength"`
	KeyLength        *string `json:"keyLength" form:"keyLength"`
	Remark           *string `json:"remark" form:"remark"`
}

type acmeAccountDeleteRequest struct {
	ID *uint `json:"id" form:"id"`
}

type acmeAccountRotateKeyRequest struct {
	ID               *uint   `json:"id" form:"id"`
	AccountKeyLength *string `json:"accountKeyLength" form:"accountKeyLength"`
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
	// Capture the revision before reading the snapshot. A later write stays newer
	// than this response and will therefore be picked up by the next poll.
	snapshotVersion := service.CurrentConfigRevisionForPolling()
	onlines, err := a.StatsService.GetOnlines()

	if err != nil {
		return "", err
	}
	trafficAge, err := a.SettingService.GetTrafficAge()
	if err != nil {
		return "", err
	}
	data["enableTraffic"] = trafficAge > 0
	data["lastUpdate"] = snapshotVersion

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
	isUpdated, err := a.ConfigService.CheckMihomoChanges(lu)
	if err != nil {
		return "", err
	}
	snapshotVersion := service.CurrentMihomoConfigRevisionForPolling()

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
	data["lastUpdate"] = snapshotVersion

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
	data["lastUpdate"] = service.CurrentConfigRevisionForPolling()
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

func (a *ApiService) GetMihomoRouteTargets(c *gin.Context) {
	targets, err := service.GetMihomoRouteTargets(database.GetDB())
	jsonObj(c, map[string]interface{}{"routeTargets": targets}, err)
}

// GetMihomoRouteEditorContext returns the compact data model used exclusively
// by the Mihomo route editor. It avoids loading unrelated clients, TLS records
// and subscription collections when the user only opens /mihomo_rules.
func (a *ApiService) GetMihomoRouteEditorContext(c *gin.Context) {
	// Capture the revision before assembling the editor snapshot. If a later
	// write wins while config or targets are being read, this response remains
	// intentionally stale and its expectedRevision will be rejected on save.
	snapshotRevision := service.CurrentMihomoConfigRevisionForPolling()
	config, err := a.MihomoConfigService.GetConfig()
	if err != nil {
		jsonObj(c, nil, err)
		return
	}

	context, err := service.GetMihomoRouteEditorContext(database.GetDB())
	if err != nil {
		jsonObj(c, nil, err)
		return
	}

	jsonObj(c, map[string]interface{}{
		"config":       json.RawMessage(config),
		"inboundTags":  context.InboundTags,
		"routeTargets": context.RouteTargets,
		"revision":     snapshotRevision,
	}, nil)
}

func (a *ApiService) LoadMihomoPartialData(c *gin.Context, objs []string) error {
	data := make(map[string]interface{}, 0)
	data["lastUpdate"] = service.CurrentMihomoConfigRevisionForPolling()
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

// GetSubscriptionURI returns the current request's calculated subscription
// base URI without loading the complete runtime data set.
func normalizeSubscriptionBaseURI(value string) (string, error) {
	return service.NormalizeSubscriptionBaseURI(value)
}

func (a *ApiService) GetSubscriptionURI(c *gin.Context) {
	subURI, err := a.SettingService.GetFinalSubURI(getHostname(c))
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	subURI, err = normalizeSubscriptionBaseURI(subURI)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, gin.H{"subURI": subURI}, nil)
}

func (a *ApiService) GetStats(c *gin.Context) {
	resource := c.Query("resource")
	namespace := strings.TrimSpace(c.Query("namespace"))
	tag := c.Query("tag")
	limit, err := strconv.Atoi(c.Query("limit"))
	if err != nil || limit <= 0 {
		limit = 100
	} else if limit > 2160 {
		limit = 2160
	}
	if namespace == "mihomo" {
		switch resource {
		case "inbound":
			resource = "mihomo_inbound"
		case "client":
			resource = "mihomo_client"
		case "outbound":
			resource = "mihomo_outbound"
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

func (a *ApiService) GetDashboardRuntime(c *gin.Context) {
	request := c.Query("r")
	result := a.ServerService.GetDashboardRuntime(request)
	jsonObj(c, result, nil)
}

func (a *ApiService) GetRuntimePerformance(c *gin.Context) {
	limit, err := strconv.Atoi(strings.TrimSpace(c.Query("limit")))
	if err != nil || limit <= 0 {
		limit = 128
	}
	if limit > 256 {
		limit = 256
	}
	jsonObj(c, gin.H{
		"samples": service.GetRuntimePerformance(limit),
		"summary": service.GetRuntimePerformanceSummary(),
	}, nil)
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
	expiryDate, expiryDateProvided := pickTrafficOverviewExpiryDate(req)
	if !limitExists || !resetExists {
		jsonMsg(c, "", fmt.Errorf("limit_gib and reset_day are required"))
		return
	}

	if err := a.TrafficOverviewService.UpdateTrafficOverviewSettings(limitGiB, resetDay, expiryDate, expiryDateProvided); err != nil {
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

func (a *ApiService) GetTrafficOverviewVnstatInstallStatus(c *gin.Context) {
	jsonObj(c, a.TrafficOverviewService.GetManagedVnstatInstallJob(c.Query("jobId")), nil)
}

func (a *ApiService) GetTrafficOverviewVnstatRemovalStatus(c *gin.Context) {
	jsonObj(c, a.TrafficOverviewService.GetManagedVnstatRemovalJob(c.Query("jobId")), nil)
}

func (a *ApiService) InstallTrafficOverviewVnstat(c *gin.Context) {
	req := trafficOverviewVnstatInstallRequest{}
	if err := c.ShouldBind(&req); err != nil {
		jsonMsg(c, "", fmt.Errorf("invalid request body: %w", err))
		return
	}
	job, err := a.TrafficOverviewService.StartManagedVnstatInstall(req.Source)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, job, nil)
}

func (a *ApiService) StopTrafficOverviewVnstatInstall(c *gin.Context) {
	req := struct {
		ID *string `json:"id" form:"id"`
	}{}
	if err := c.ShouldBind(&req); err != nil {
		jsonMsg(c, "", fmt.Errorf("invalid request body: %w", err))
		return
	}
	if req.ID == nil || strings.TrimSpace(*req.ID) == "" {
		jsonMsg(c, "", fmt.Errorf("id is required"))
		return
	}
	job, err := a.TrafficOverviewService.StopManagedVnstatInstall(strings.TrimSpace(*req.ID))
	jsonObj(c, job, err)
}

func (a *ApiService) RemoveTrafficOverviewVnstat(c *gin.Context) {
	job, err := a.TrafficOverviewService.StartManagedVnstatRemoval()
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, job, nil)
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

func pickTrafficOverviewExpiryDate(req trafficOverviewSettingsRequest) (string, bool) {
	if req.ExpiryDate != nil {
		return *req.ExpiryDate, true
	}
	if req.ExpiryDateCompat != nil {
		return *req.ExpiryDateCompat, true
	}
	return "", false
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

func (a *ApiService) GetFirewallRuntime(c *gin.Context) {
	jsonObj(c, a.FirewallService.GetRuntimeOverview(), nil)
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
	task, err := a.FirewallService.StartManagedNftablesInstall()
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, task, nil)
}

func (a *ApiService) GetFirewallNftablesInstallStatus(c *gin.Context) {
	jsonObj(c, a.FirewallService.GetManagedNftablesInstall(), nil)
}

func (a *ApiService) StopFirewallNftablesInstall(c *gin.Context) {
	req := struct {
		ID *string `json:"id" form:"id"`
	}{}
	if err := c.ShouldBind(&req); err != nil {
		jsonMsg(c, "", fmt.Errorf("invalid request body: %w", err))
		return
	}
	if req.ID == nil || strings.TrimSpace(*req.ID) == "" {
		jsonMsg(c, "", fmt.Errorf("id is required"))
		return
	}
	task, err := a.FirewallService.StopManagedNftablesInstall(strings.TrimSpace(*req.ID))
	jsonObj(c, task, err)
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
	overview, err := a.SystemLogOptimizationService.GetOverviewContext(c.Request.Context())
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
	overview, err := a.SystemSysctlOptimizationService.GetOverviewContext(c.Request.Context())
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
	overview, err := a.SystemLinuxDNSOptimizationService.GetOverviewContext(c.Request.Context())
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
	overview, err := a.SystemMTUOptimizationService.GetOverviewContext(c.Request.Context())
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
	after := -1
	if rawAfter := strings.TrimSpace(c.Query("after")); rawAfter != "" {
		parsed, parseErr := strconv.Atoi(rawAfter)
		if parseErr != nil || parsed < 0 {
			jsonMsg(c, "", fmt.Errorf("after must be a non-negative integer"))
			return
		}
		after = parsed
	}
	session, err := a.AcmeService.GetLogSessionAfter(id, after)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	if session != nil {
		status := strings.TrimSpace(session.TaskStatus)
		if status == "" {
			status = strings.TrimSpace(session.Status)
		}
		if status == "success" || status == "warning" || status == "error" {
			a.AcmeService.CleanupTaskAndLog(session.TaskID, session.Id)
		}
	}
	jsonObj(c, session, nil)
}

func (a *ApiService) GetAcmeTask(c *gin.Context) {
	id := strings.TrimSpace(c.Query("id"))
	if id == "" {
		jsonMsg(c, "", fmt.Errorf("id is required"))
		return
	}
	task, err := a.AcmeService.GetTask(id)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	if task != nil && (task.Status == "success" || task.Status == "warning" || task.Status == "error") {
		a.AcmeService.CleanupTaskAndLog(task.ID, task.LogSessionID)
	}
	jsonObj(c, task, nil)
}

func (a *ApiService) GetActiveAcmeTasks(c *gin.Context) {
	jsonObj(c, a.AcmeService.GetActiveTasks(), nil)
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
	result, err := a.AcmeService.StartManagedInstallOrReinstall(service.AcmeInstallPayload{
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

func (a *ApiService) GetAcmeInstallStatus(c *gin.Context) {
	jsonObj(c, a.AcmeService.GetManagedAcmeInstall(c.Query("id")), nil)
}

func (a *ApiService) StopAcmeInstall(c *gin.Context) {
	req := struct {
		ID *string `json:"id" form:"id"`
	}{}
	if err := c.ShouldBind(&req); err != nil {
		jsonMsg(c, "", fmt.Errorf("invalid request body: %w", err))
		return
	}
	if req.ID == nil || strings.TrimSpace(*req.ID) == "" {
		jsonMsg(c, "", fmt.Errorf("id is required"))
		return
	}
	result, err := a.AcmeService.StopManagedAcmeInstall(strings.TrimSpace(*req.ID))
	jsonObj(c, result, err)
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
	a.issueAcmeCertificate(c, false)
}

func (a *ApiService) ReissueAcmeCertificate(c *gin.Context) {
	a.issueAcmeCertificate(c, true)
}

func (a *ApiService) IssueAcmeCertificateTask(c *gin.Context) {
	a.issueAcmeCertificateTask(c, false)
}

func (a *ApiService) ReissueAcmeCertificateTask(c *gin.Context) {
	a.issueAcmeCertificateTask(c, true)
}

func (a *ApiService) issueAcmeCertificate(c *gin.Context, requireExistingRecord bool) {
	payload, err := a.parseAcmeIssuePayload(c, requireExistingRecord)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	result, err := a.AcmeService.Issue(payload)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, result, nil)
}

func (a *ApiService) issueAcmeCertificateTask(c *gin.Context, requireExistingRecord bool) {
	payload, err := a.parseAcmeIssuePayload(c, requireExistingRecord)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	var task *service.AcmeTaskView
	if requireExistingRecord {
		task, err = a.AcmeService.StartReissueTask(payload)
	} else {
		task, err = a.AcmeService.StartIssueTask(payload)
	}
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, task, nil)
}

func (a *ApiService) parseAcmeIssuePayload(c *gin.Context, requireExistingRecord bool) (service.AcmeIssuePayload, error) {
	req := acmeIssueRequest{}
	if err := c.ShouldBind(&req); err != nil {
		return service.AcmeIssuePayload{}, fmt.Errorf("invalid request body: %w", err)
	}
	if requireExistingRecord && (req.ExistingRecordID == nil || *req.ExistingRecordID == 0) {
		return service.AcmeIssuePayload{}, fmt.Errorf("existingRecordId is required")
	}

	certificateType := "domain"
	if req.CertificateType != nil {
		certificateType = strings.TrimSpace(*req.CertificateType)
	}
	normalizedCertificateType := normalizeAcmeIssueCertificateType(certificateType)
	if !requireExistingRecord && normalizedCertificateType == "domain" && (req.AcmeAccountID == nil || *req.AcmeAccountID == 0) {
		return service.AcmeIssuePayload{}, fmt.Errorf("acmeAccountId is required for domain certificate")
	}

	payload := service.AcmeIssuePayload{}
	if req.ExistingRecordID != nil {
		payload.ExistingRecordID = *req.ExistingRecordID
	}
	if req.Domains != nil {
		payload.DomainsText = strings.TrimSpace(*req.Domains)
		payload.DomainsProvided = true
	}
	if req.CertificateType != nil {
		payload.CertificateType = strings.TrimSpace(*req.CertificateType)
		payload.CertificateTypeProvided = true
	}
	if req.Challenge != nil {
		payload.Challenge = strings.TrimSpace(*req.Challenge)
		payload.ChallengeProvided = true
	}
	if req.Webroot != nil {
		payload.Webroot = strings.TrimSpace(*req.Webroot)
		payload.WebrootProvided = true
	}
	if req.DNSProvider != nil {
		payload.DNSProvider = strings.TrimSpace(*req.DNSProvider)
		payload.DNSProviderProvided = true
	}
	if req.DNSEnv != nil {
		payload.DNSEnvText = *req.DNSEnv
		payload.DNSEnvProvided = true
	}
	if req.Server != nil {
		payload.Server = strings.TrimSpace(*req.Server)
	}
	if req.KeyLength != nil {
		payload.KeyLength = strings.TrimSpace(*req.KeyLength)
		payload.KeyLengthProvided = true
	}
	if req.CustomArgs != nil {
		payload.CustomArgs = strings.TrimSpace(*req.CustomArgs)
		payload.CustomArgsProvided = true
	}
	if req.AcmeAccountID != nil {
		payload.AcmeAccountID = *req.AcmeAccountID
		payload.AcmeAccountProvided = true
	}
	if req.DNSAccountID != nil {
		payload.DNSAccountID = *req.DNSAccountID
		payload.DNSAccountProvided = true
	}
	payload.AutoRenew = true
	if req.AutoRenew != nil {
		payload.AutoRenew = *req.AutoRenew
		payload.AutoRenewProvided = true
	}
	if req.Remark != nil {
		payload.Remark = strings.TrimSpace(*req.Remark)
		payload.RemarkProvided = true
	}
	if req.ApplyTarget != nil {
		payload.ApplyTarget = strings.TrimSpace(*req.ApplyTarget)
		payload.ApplyTargetProvided = true
	}
	if req.PushDir != nil {
		payload.PushDir = strings.TrimSpace(*req.PushDir)
		payload.PushDirProvided = true
		payload.PushExplicit = payload.PushDir != ""
	}
	if req.LogSessionID != nil {
		payload.LogSessionID = strings.TrimSpace(*req.LogSessionID)
	}
	return payload, nil
}

func (a *ApiService) RenewAcmeCertificate(c *gin.Context) {
	payload, err := a.parseAcmeRenewPayload(c)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	result, err := a.AcmeService.Renew(payload)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, result, nil)
}

func (a *ApiService) RenewAcmeCertificateTask(c *gin.Context) {
	payload, err := a.parseAcmeRenewPayload(c)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	task, err := a.AcmeService.StartRenewTask(payload)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, task, nil)
}

func (a *ApiService) parseAcmeRenewPayload(c *gin.Context) (service.AcmeRenewPayload, error) {
	req := acmeRenewRequest{}
	if err := c.ShouldBind(&req); err != nil {
		return service.AcmeRenewPayload{}, fmt.Errorf("invalid request body: %w", err)
	}
	if req.ID == nil || *req.ID == 0 {
		return service.AcmeRenewPayload{}, fmt.Errorf("id is required")
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
	if req.LogSessionID != nil {
		payload.LogSessionID = strings.TrimSpace(*req.LogSessionID)
	}
	return payload, nil
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
	clear := req.Clear != nil && *req.Clear
	targetDir := ""
	if req.TargetDir != nil {
		targetDir = strings.TrimSpace(*req.TargetDir)
	}
	if !clear && targetDir == "" {
		jsonMsg(c, "", fmt.Errorf("targetDir is required"))
		return
	}
	if clear && targetDir != "" {
		jsonMsg(c, "", fmt.Errorf("targetDir must be empty when clear is true"))
		return
	}

	result, err := a.AcmeService.Push(service.AcmePushPayload{
		ID:        *req.ID,
		TargetDir: targetDir,
		Clear:     clear,
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
	if c.Query("page") != "" || c.Query("per_page") != "" || c.Query("search") != "" {
		page := 1
		if rawPage := strings.TrimSpace(c.Query("page")); rawPage != "" {
			if parsed, err := strconv.Atoi(rawPage); err == nil && parsed > 0 {
				page = parsed
			}
		}
		perPage := 20
		if rawPerPage := strings.TrimSpace(c.Query("per_page")); rawPerPage != "" {
			if parsed, err := strconv.Atoi(rawPerPage); err == nil && parsed > 0 {
				perPage = parsed
			}
		}
		result, err := a.CertificateInventoryService.ListPage(page, perPage, c.Query("search"))
		if err != nil {
			jsonMsg(c, "", err)
			return
		}
		jsonObj(c, result, nil)
		return
	}
	certificates, err := a.CertificateInventoryService.List()
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, certificates, nil)
}

func (a *ApiService) ListTLSCertificateOptions(c *gin.Context) {
	options, err := a.CertificateInventoryService.ListTLSOptions()
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, options, nil)
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

func (a *ApiService) GetAcmeCertificateLog(c *gin.Context) {
	id, err := strconv.ParseUint(strings.TrimSpace(c.Query("id")), 10, 64)
	if err != nil || id == 0 {
		jsonMsg(c, "", fmt.Errorf("valid certificate id is required"))
		return
	}
	logView, err := a.CertificateInventoryService.GetLog(uint(id))
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, logView, nil)
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
	payload := service.AcmeAccountPayload{
		Name: strings.TrimSpace(*req.Name),
	}
	if req.Email != nil {
		payload.Email = strings.TrimSpace(*req.Email)
		payload.EmailProvided = true
	}
	if req.ID != nil {
		payload.ID = *req.ID
	}
	if req.Server != nil {
		payload.Server = strings.TrimSpace(*req.Server)
		payload.ServerProvided = true
	}
	if req.AccountKeyLength != nil {
		payload.AccountKeyLength = strings.TrimSpace(*req.AccountKeyLength)
		payload.AccountKeyLengthProvided = true
	} else if req.KeyLength != nil {
		payload.KeyLength = strings.TrimSpace(*req.KeyLength)
		payload.AccountKeyLengthProvided = true
	}
	if req.Remark != nil {
		payload.Remark = strings.TrimSpace(*req.Remark)
		payload.RemarkProvided = true
	}

	result, err := a.AcmeService.SaveAcmeAccount(payload)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, result, nil)
}

func (a *ApiService) RotateAcmeAccountKey(c *gin.Context) {
	req := acmeAccountRotateKeyRequest{}
	if err := c.ShouldBind(&req); err != nil {
		jsonMsg(c, "", fmt.Errorf("invalid request body: %w", err))
		return
	}
	if req.ID == nil || *req.ID == 0 {
		jsonMsg(c, "", fmt.Errorf("id is required"))
		return
	}
	if req.AccountKeyLength == nil || strings.TrimSpace(*req.AccountKeyLength) == "" {
		jsonMsg(c, "", fmt.Errorf("accountKeyLength is required"))
		return
	}
	result, err := a.AcmeService.RotateAcmeAccountKey(service.AcmeAccountRotateKeyPayload{
		ID:               *req.ID,
		AccountKeyLength: strings.TrimSpace(*req.AccountKeyLength),
	})
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
	}
	if req.ID != nil {
		payload.ID = *req.ID
	}
	if req.EnvJSON != nil {
		payload.EnvJSON = strings.TrimSpace(*req.EnvJSON)
		payload.EnvJSONProvided = true
	}
	if req.Remark != nil {
		payload.Remark = strings.TrimSpace(*req.Remark)
		payload.RemarkProvided = true
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

// GetPortForwardRuntime serves the active-page poller without repeating the
// complete SQLite overview or a /proc ownership scan on every interval.
func (a *ApiService) GetPortForwardRuntime(c *gin.Context) {
	runtime, err := a.PortForwardService.GetRuntimeOverview()
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, runtime, nil)
}

// SyncPortForward immediately reconciles the managed nftables state before
// returning the complete overview requested by the manual refresh action.
func (a *ApiService) SyncPortForward(c *gin.Context) {
	if err := a.PortForwardService.SyncIfNeeded(0); err != nil {
		jsonMsg(c, "", err)
		return
	}
	a.GetPortForwardOverview(c)
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

func (a *ApiService) ResetPortForwardRuleTraffic(c *gin.Context) {
	req := firewallRuleDeleteRequest{}
	if err := c.ShouldBind(&req); err != nil {
		jsonMsg(c, "", fmt.Errorf("invalid request body: %w", err))
		return
	}
	if req.ID == nil || *req.ID == 0 {
		jsonMsg(c, "", fmt.Errorf("id is required"))
		return
	}
	if err := a.PortForwardService.ResetRuleTraffic(*req.ID); err != nil {
		jsonMsg(c, "", err)
		return
	}
	a.GetPortForwardOverview(c)
}

func (a *ApiService) ResetPortForwardOverviewTraffic(c *gin.Context) {
	if err := a.PortForwardService.ResetOverviewTraffic(); err != nil {
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

// GetReverseProxyRuntime deliberately returns only volatile counters and
// listener state.  The settings page uses it for its short polling interval,
// avoiding a SQLite/certificate inventory read every few seconds.
func (a *ApiService) GetReverseProxyRuntime(c *gin.Context) {
	runtime, err := a.ReverseProxyService.GetRuntimeOverview()
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, runtime, nil)
}

func (a *ApiService) writeReverseProxyMutationError(c *gin.Context, err error) bool {
	if err == nil {
		return false
	}
	var obj interface{}
	if service.IsReverseProxyRevisionConflict(err) {
		currentRevision, revisionErr := a.ReverseProxyService.CurrentRevision()
		if revisionErr == nil {
			obj = map[string]interface{}{
				"code":            "revision_conflict",
				"currentRevision": currentRevision,
			}
		} else {
			obj = map[string]interface{}{"code": "revision_conflict"}
		}
	}
	jsonMsgObj(c, "", obj, err)
	return true
}

func (a *ApiService) SaveReverseProxySettings(c *gin.Context) {
	req := service.ReverseProxySettingsPayload{}
	if err := c.ShouldBind(&req); err != nil {
		jsonMsg(c, "", fmt.Errorf("invalid request body: %w", err))
		return
	}
	if req.ExpectedRevision == nil {
		jsonMsg(c, "", fmt.Errorf("expectedRevision is required"))
		return
	}
	if err := a.ReverseProxyService.SaveResourceSettings(req); a.writeReverseProxyMutationError(c, err) {
		return
	}
	a.GetReverseProxyOverview(c)
}

func (a *ApiService) SaveReverseProxyRule(c *gin.Context) {
	req := service.ReverseProxyRulePayload{}
	if err := c.ShouldBind(&req); err != nil {
		jsonMsg(c, "", fmt.Errorf("invalid request body: %w", err))
		return
	}
	if req.ExpectedRevision == nil {
		jsonMsg(c, "", fmt.Errorf("expectedRevision is required"))
		return
	}
	if err := a.ReverseProxyService.UpsertRule(req); a.writeReverseProxyMutationError(c, err) {
		return
	}
	a.GetReverseProxyOverview(c)
}

func (a *ApiService) SetReverseProxyRuleStatus(c *gin.Context) {
	req := service.ReverseProxyRuleStatusPayload{}
	if err := c.ShouldBind(&req); err != nil {
		jsonMsg(c, "", fmt.Errorf("invalid request body: %w", err))
		return
	}
	if req.ID == 0 {
		jsonMsg(c, "", fmt.Errorf("id is required"))
		return
	}
	if req.ExpectedRevision == nil {
		jsonMsg(c, "", fmt.Errorf("expectedRevision is required"))
		return
	}
	result, err := a.ReverseProxyService.SetRuleEnabled(req)
	if a.writeReverseProxyMutationError(c, err) {
		return
	}
	jsonObj(c, result, nil)
}

func (a *ApiService) MoveReverseProxyRule(c *gin.Context) {
	req := service.ReverseProxyRuleMovePayload{}
	if err := c.ShouldBind(&req); err != nil {
		jsonMsg(c, "", fmt.Errorf("invalid request body: %w", err))
		return
	}
	if req.ID == 0 {
		jsonMsg(c, "", fmt.Errorf("id is required"))
		return
	}
	if req.ExpectedRevision == nil {
		jsonMsg(c, "", fmt.Errorf("expectedRevision is required"))
		return
	}
	result, err := a.ReverseProxyService.MoveRule(req)
	if a.writeReverseProxyMutationError(c, err) {
		return
	}
	jsonObj(c, result, nil)
}

func (a *ApiService) DeleteReverseProxyRule(c *gin.Context) {
	req := service.ReverseProxyRuleDeletePayload{}
	if err := c.ShouldBind(&req); err != nil {
		jsonMsg(c, "", fmt.Errorf("invalid request body: %w", err))
		return
	}
	if req.ID == 0 {
		jsonMsg(c, "", fmt.Errorf("id is required"))
		return
	}
	if req.ExpectedRevision == nil {
		jsonMsg(c, "", fmt.Errorf("expectedRevision is required"))
		return
	}
	if err := a.ReverseProxyService.DeleteRuleWithRevision(req); a.writeReverseProxyMutationError(c, err) {
		return
	}
	a.GetReverseProxyOverview(c)
}

func (a *ApiService) ReorderReverseProxyRules(c *gin.Context) {
	req := service.ReverseProxyRuleReorderPayload{}
	if err := c.ShouldBind(&req); err != nil {
		jsonMsg(c, "", fmt.Errorf("invalid request body: %w", err))
		return
	}
	if len(req.IDs) == 0 {
		jsonMsg(c, "", fmt.Errorf("ids are required"))
		return
	}
	if req.ExpectedRevision == nil {
		jsonMsg(c, "", fmt.Errorf("expectedRevision is required"))
		return
	}
	if err := a.ReverseProxyService.ReorderRules(req); a.writeReverseProxyMutationError(c, err) {
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
	result, err := a.KernelManagerService.StartManagedCleanupPurge(req.Packages)
	jsonObj(c, result, err)
}

func (a *ApiService) AutoCleanupKernelPackages(c *gin.Context) {
	result, err := a.KernelManagerService.StartManagedAutoCleanup()
	jsonObj(c, result, err)
}

func (a *ApiService) GetKernelCleanupStatus(c *gin.Context) {
	status := a.KernelManagerService.GetManagedCleanupTaskStatus(c.Query("id"))
	jsonObj(c, status, nil)
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
	result, err := a.KernelManagerService.StartManagedDownloadPackages(provider, line, *req.Version, arch)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, result, nil)
}

func (a *ApiService) GetKernelDownloadProgress(c *gin.Context) {
	progress := a.KernelManagerService.GetManagedDownloadProgress(c.Query("id"))
	jsonObj(c, progress, nil)
}

func (a *ApiService) GetKernelInstallStatus(c *gin.Context) {
	status := a.KernelManagerService.GetInstallStatus()
	jsonObj(c, status, nil)
}

func (a *ApiService) StopKernelDownload(c *gin.Context) {
	req := struct {
		ID *string `json:"id" form:"id"`
	}{}
	if err := c.ShouldBind(&req); err != nil {
		jsonMsg(c, "", fmt.Errorf("invalid request body: %w", err))
		return
	}
	if req.ID == nil || strings.TrimSpace(*req.ID) == "" {
		jsonMsg(c, "", fmt.Errorf("id is required"))
		return
	}
	progress, err := a.KernelManagerService.StopManagedDownloadPackages(strings.TrimSpace(*req.ID))
	jsonObj(c, progress, err)
}

func (a *ApiService) InstallKernelPackages(c *gin.Context) {
	// Installation is intentionally driven by the persisted completed-download
	// marker rather than the browser's current provider/version selection.
	result, err := a.KernelManagerService.InstallDownloadedKernel()
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

func (a *ApiService) GetTLSCertificateInfo(c *gin.Context) {
	req := tlsSha256Request{}
	if err := c.ShouldBind(&req); err != nil {
		jsonMsg(c, "", err)
		return
	}

	inspection, err := a.ServerService.InspectTLSCertificate(req.SourceType, req.CertificatePath, req.CertificatePEM)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, inspection, nil)
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
	c.Header("Content-Disposition", "attachment; filename=kwor_"+service.PanelNow().Format("20060102-150405")+".db")
	c.Writer.Write(db)
}

func (a *ApiService) DownloadDBBackup(c *gin.Context) {
	archive, err := database.BuildDBBackupArchive()
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	c.Header("Content-Type", "application/zip")
	c.Header("Content-Disposition", "attachment; filename="+archive.FileName)
	c.Header("Content-Length", strconv.Itoa(len(archive.Data)))
	_, _ = c.Writer.Write(archive.Data)
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
	if !isPanelLoginTransportAllowed(c) {
		jsonMsg(c, "", errors.New("panel login requires https"))
		return
	}
	remoteIP := getRemoteIp(c)
	loginUser, err := a.UserService.Login(c.Request.FormValue("user"), c.Request.FormValue("pass"), remoteIP)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}

	sessionMaxAgeMinutes, err := a.SettingService.GetSessionMaxAge()
	if err != nil {
		logger.Warning("load login session timeout failed; fallback to default timeout for this login: ", err)
		sessionMaxAgeMinutes = 0
	}
	if err := SetLoginUser(c, loginUser, sessionMaxAgeMinutes); err != nil {
		logger.Warning("login session cookie write failed: ", err)
		jsonMsg(c, "", err)
		return
	}
	logger.Info("user ", loginUser, " login success")

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
	a.writeSessionStatus(c, false)
}

// SessionActivity is kept as a compatibility alias of the same refresh path as
// Session. Frontend keepalive and legacy callers can both use it safely.
func (a *ApiService) SessionActivity(c *gin.Context) {
	a.writeSessionStatus(c, true)
}

func (a *ApiService) writeSessionStatus(c *gin.Context, userActivity bool) {
	status, valid, reason, err := RefreshLoginSession(c, userActivity)
	if valid {
		jsonObj(c, status, nil)
		return
	}
	if err != nil {
		logger.Warningf("login session refresh failed: reason=%s remote=%s error=%v", reason, getRemoteIp(c), err)
	} else {
		logger.Infof("login session rejected: reason=%s remote=%s", reason, getRemoteIp(c))
	}
	if loginSessionReasonIsTransient(reason) {
		c.JSON(http.StatusServiceUnavailable, Msg{
			Success: false,
			Msg:     "Session temporarily unavailable",
			Obj:     map[string]string{"reason": reason},
		})
		return
	}
	c.JSON(http.StatusOK, Msg{
		Success: false,
		Msg:     "Invalid login",
		Obj:     map[string]string{"reason": reason},
	})
}

func (a *ApiService) ChangePass(c *gin.Context) {
	id := c.Request.FormValue("id")
	oldPass := c.Request.FormValue("oldPass")
	newUsername := c.Request.FormValue("newUsername")
	newPass := c.Request.FormValue("newPass")
	oldUsername, err := a.UserService.ChangePass(id, oldPass, newUsername, newPass)
	if err == nil {
		if err := RenameLoginSessionUser(oldUsername, newUsername); err != nil {
			logger.Warning("update renamed user sessions failed: ", err)
		}
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
	if obj == "settings" {
		a.saveLegacySettings(c, loginUser, data)
		return
	}
	preparedData := json.RawMessage(data)

	objs, err := a.ConfigService.Save(obj, act, preparedData, initUsers, loginUser, hostname)
	if err != nil {
		var committedErr *service.CommittedSaveError
		if errors.As(err, &committedErr) {
			writeCommittedSaveFailure(c, committedErr)
			return
		}
		jsonMsg(c, "save", err)
		return
	}
	if strings.HasPrefix(obj, "mihomo_") {
		err = a.LoadMihomoPartialData(c, objs)
	} else {
		err = a.LoadPartialData(c, objs)
	}
	if err != nil {
		writeCommittedPartialLoadFailure(c, obj, err)
	}
}

func writeCommittedSaveFailure(c *gin.Context, committedErr *service.CommittedSaveError) {
	retryRuntime := committedErr != nil && committedErr.RetrySingboxRuntime
	jsonMsgObj(c, "save", map[string]interface{}{
		"committed":    true,
		"retryRuntime": retryRuntime,
	}, fmt.Errorf("data was saved, but a post-commit operation failed: %w", committedErr))
}

// writeCommittedPartialLoadFailure distinguishes a successful database save
// from the later best-effort data reload used to build the API response.
func writeCommittedPartialLoadFailure(c *gin.Context, object string, err error) {
	jsonMsgObj(c, object, map[string]interface{}{
		"committed":     true,
		"retryRuntime":  false,
		"refreshFailed": true,
	}, fmt.Errorf("data was saved, but the response data refresh failed: %w", err))
}

func (a *ApiService) RetrySingboxRuntime(c *gin.Context, loginUser string) {
	_ = loginUser
	if err := validateSingboxRuntimeRetryPayload(c.Request.Body); err != nil {
		jsonMsg(c, "retryRuntime", err)
		return
	}
	jsonMsg(c, "retryRuntime", a.ConfigService.RetrySingboxRuntime())
}

func (a *ApiService) RestartApp(c *gin.Context) {
	err := a.PanelService.RestartPanel(3 * time.Second)
	jsonMsg(c, "restartApp", err)
}

func (a *ApiService) LinkConvert(c *gin.Context) {
	link := c.Request.FormValue("link")
	result, _, err := util.GetOutbound(link, 0)
	jsonObj(c, result, err)
}

func (a *ApiService) ImportDb(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, database.MaxDatabaseImportUploadBytes())
	file, _, err := c.Request.FormFile("db")
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	defer file.Close()
	err = database.ImportDB(file)
	jsonMsg(c, "", err)
}

func (a *ApiService) RestoreDBBackup(c *gin.Context) {
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, database.MaxDBBackupArchiveUploadBytes())
	file, _, err := c.Request.FormFile("backup")
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	defer file.Close()

	stopRunningCores := func() error {
		singboxSvc := &service.CoreManagerService{}
		if singboxSvc.IsRunning() || service.ShouldRecoverManagedCoreOnStartup("singbox") {
			if err := singboxSvc.StopCore(); err != nil {
				return fmt.Errorf("停止 Sing-Box 内核失败: %v", err)
			}
		}

		mihomoSvc := &service.MihomoCoreManagerService{}
		if mihomoSvc.IsRunning() || service.ShouldRecoverManagedCoreOnStartup("mihomo") {
			if err := mihomoSvc.StopCore(); err != nil {
				return fmt.Errorf("停止 Mihomo 内核失败: %v", err)
			}
		}
		return nil
	}

	panelRestarter := func() error {
		return a.PanelService.RestartPanel(3 * time.Second)
	}

	err = database.RestoreDBBackupArchive(file, panelRestarter, stopRunningCores)
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
	loginUser := GetLoginUser(c)
	tokenId := c.Request.FormValue("id")
	err := a.UserService.DeleteToken(loginUser, tokenId)
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
	autoSyncEnabled := false
	if result != nil {
		client := &model.Client{}
		if loadErr := database.GetDB().Model(model.Client{}).Where("name = ?", clientName).First(client).Error; loadErr == nil {
			if markErr := a.SettingService.SetSubManagerAutoSyncClient(client.Id, true); markErr != nil {
				logger.Warning("set default auto sync client failed: ", markErr)
				if err == nil {
					err = fmt.Errorf("set default auto sync client failed: %w", markErr)
				}
			} else {
				autoSyncEnabled = true
			}
		} else if database.IsNotFound(loadErr) {
			if err == nil {
				err = fmt.Errorf("default client disappeared before auto sync marker could be saved")
			}
		} else {
			logger.Warning("load default client for auto sync marker failed: ", loadErr)
			if err == nil {
				err = fmt.Errorf("load default client for auto sync marker failed: %w", loadErr)
			}
		}
	}
	if err != nil {
		if result != nil {
			jsonMsgObj(c, "", gin.H{
				"result":          result,
				"committed":       true,
				"autoSyncEnabled": autoSyncEnabled,
			}, err)
			return
		}
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, result, nil)
}

func (a *ApiService) SetClientSubManagerAutoSync(c *gin.Context) {
	id, err := strconv.ParseUint(c.Request.FormValue("id"), 10, 64)
	if err != nil || id == 0 {
		jsonMsg(c, "", fmt.Errorf("client id is required"))
		return
	}
	enabled, err := strconv.ParseBool(c.Request.FormValue("enabled"))
	if err != nil {
		jsonMsg(c, "", fmt.Errorf("auto sync enabled must be true or false"))
		return
	}
	client := &model.Client{}
	if err := database.GetDB().Select("id").Where("id = ?", id).First(client).Error; err != nil {
		jsonMsg(c, "", err)
		return
	}
	if err := a.SettingService.SetSubManagerAutoSyncClient(client.Id, enabled); err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, gin.H{"id": client.Id, "enabled": enabled}, nil)
}

func (a *ApiService) SyncMihomoToSubManager(c *gin.Context) {
	clientName := c.Request.FormValue("name")
	if clientName == "" {
		jsonMsg(c, "", fmt.Errorf("mihomo client name is required"))
		return
	}
	hostname := getHostname(c)
	result, err := a.MihomoSyncService.SyncClientToSubManager(clientName, hostname)
	autoSyncEnabled := false
	if result != nil {
		client := &model.MihomoClient{}
		if loadErr := database.GetDB().Model(model.MihomoClient{}).Where("name = ?", clientName).First(client).Error; loadErr == nil {
			if markErr := a.SettingService.SetSubManagerAutoSyncMihomoClient(client.Id, true); markErr != nil {
				logger.Warning("set mihomo auto sync client failed: ", markErr)
				if err == nil {
					err = fmt.Errorf("set mihomo auto sync client failed: %w", markErr)
				}
			} else {
				autoSyncEnabled = true
			}
		} else if database.IsNotFound(loadErr) {
			if err == nil {
				err = fmt.Errorf("mihomo client disappeared before auto sync marker could be saved")
			}
		} else {
			logger.Warning("load mihomo client for auto sync marker failed: ", loadErr)
			if err == nil {
				err = fmt.Errorf("load mihomo client for auto sync marker failed: %w", loadErr)
			}
		}
	}
	if err != nil {
		if result != nil {
			jsonMsgObj(c, "", gin.H{
				"result":          result,
				"committed":       true,
				"autoSyncEnabled": autoSyncEnabled,
			}, err)
			return
		}
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, result, nil)
}

func (a *ApiService) SetMihomoClientSubManagerAutoSync(c *gin.Context) {
	id, err := strconv.ParseUint(c.Request.FormValue("id"), 10, 64)
	if err != nil || id == 0 {
		jsonMsg(c, "", fmt.Errorf("mihomo client id is required"))
		return
	}
	enabled, err := strconv.ParseBool(c.Request.FormValue("enabled"))
	if err != nil {
		jsonMsg(c, "", fmt.Errorf("auto sync enabled must be true or false"))
		return
	}
	client := &model.MihomoClient{}
	if err := database.GetDB().Select("id").Where("id = ?", id).First(client).Error; err != nil {
		jsonMsg(c, "", err)
		return
	}
	if err := a.SettingService.SetSubManagerAutoSyncMihomoClient(client.Id, enabled); err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, gin.H{"id": client.Id, "enabled": enabled}, nil)
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
	c.Header("Content-Disposition", "attachment; filename=config_"+service.PanelNow().Format("20060102-150405")+".json")
	c.Writer.Write(rawConfig)
}

// GetServerIPs returns available public IPs of the server.
// Query parameter verify=true fetches real outbound IPs from external APIs
// (recommended, NAT-friendly). Otherwise only local interface public IPs are returned.
func (a *ApiService) GetServerIPs(c *gin.Context) {
	verify := c.Query("verify")
	forceRefresh := c.Query("refresh") == "true"

	var ips []string
	if verify == "true" {
		// Fetch real outbound IPs from external APIs (supports NAT scenarios).
		ips = a.IPDetectService.GetOutboundIPs(forceRefresh)
	} else {
		// Use local interface public IPs only (not NAT-aware).
		ips = a.IPDetectService.GetAllAvailableIPs()
		// Fallback to the default outbound IP when no local address is available.
		// Verified requests already perform this fallback inside their shared timeout.
		if len(ips) == 0 {
			defaultIP, ok := a.IPDetectService.GetDefaultOutboundIP()
			if ok {
				ips = []string{defaultIP}
			}
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
		var committedErr *service.CommittedSaveError
		if errors.As(err, &committedErr) {
			data, dataErr := a.getSubGroupData()
			if dataErr != nil {
				jsonMsg(c, "", dataErr)
				return
			}
			data["committed"] = true
			jsonMsgObj(c, "", data, fmt.Errorf("subscription data was saved, but post-commit validation failed: %w", committedErr))
			return
		}
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
		var committedErr *service.CommittedSaveError
		if errors.As(err, &committedErr) {
			data, dataErr := a.getSubGroupData()
			if dataErr != nil {
				jsonMsg(c, "", dataErr)
				return
			}
			data["committed"] = true
			data["result"] = result
			jsonMsgObj(c, "", data, fmt.Errorf("subscription data was saved, but post-commit validation failed: %w", committedErr))
			return
		}
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
		var committedErr *service.CommittedSaveError
		if errors.As(err, &committedErr) {
			data, dataErr := a.getSubGroupData()
			if dataErr != nil {
				jsonMsg(c, "", dataErr)
				return
			}
			data["committed"] = true
			data["result"] = result
			jsonMsgObj(c, "", data, fmt.Errorf("subscription manager data was cleared, but post-commit validation failed: %w", committedErr))
			return
		}
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
		var committedErr *service.CommittedSingboxOutboundSubscriptionImportError
		if errors.As(err, &committedErr) {
			data, loadErr := a.getOutboundGroupData()
			if loadErr != nil {
				jsonMsg(c, "", errors.Join(err, loadErr))
				return
			}
			data["committed"] = true
			jsonMsgObj(c, "", data, fmt.Errorf("订阅数据已保存，但运行配置更新失败: %w", committedErr))
			return
		}
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
		var committedErr *service.CommittedSingboxOutboundSubscriptionImportError
		if errors.As(err, &committedErr) {
			data, loadErr := a.getOutboundGroupData()
			if loadErr != nil {
				jsonMsg(c, "", errors.Join(err, loadErr))
				return
			}
			data["result"] = result
			data["committed"] = true
			jsonMsgObj(c, "", data, fmt.Errorf("订阅数据已保存，但运行配置更新失败: %w", committedErr))
			return
		}
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
		var committedErr *service.CommittedMihomoSubscriptionImportError
		if errors.As(err, &committedErr) {
			data, loadErr := a.getMihomoOutboundGroupData()
			if loadErr != nil {
				jsonMsg(c, "", errors.Join(err, loadErr))
				return
			}
			data["committed"] = true
			jsonMsgObj(c, "", data, fmt.Errorf("订阅数据已保存，但运行配置更新失败: %w", committedErr))
			return
		}
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
		var committedErr *service.CommittedMihomoSubscriptionImportError
		if errors.As(err, &committedErr) {
			data, loadErr := a.getMihomoOutboundGroupData()
			if loadErr != nil {
				jsonMsg(c, "", errors.Join(err, loadErr))
				return
			}
			data["result"] = result
			data["committed"] = true
			jsonMsgObj(c, "", data, fmt.Errorf("订阅数据已保存，但运行配置更新失败: %w", committedErr))
			return
		}
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
	result, err := a.PanelUpdateService.GetRemoteVersionsContext(c.Request.Context(), offset, limit)
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

	result, err := a.PanelUpdateService.StartManagedInstall(version)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, result, nil)
}

func (a *ApiService) StopPanelUpdate(c *gin.Context) {
	req := struct {
		ID *string `json:"id" form:"id"`
	}{}
	if err := c.ShouldBind(&req); err != nil {
		jsonMsg(c, "", fmt.Errorf("invalid request body: %w", err))
		return
	}
	if req.ID == nil || strings.TrimSpace(*req.ID) == "" {
		jsonMsg(c, "", fmt.Errorf("id is required"))
		return
	}
	result, err := a.PanelUpdateService.StopManagedInstall(strings.TrimSpace(*req.ID))
	jsonObj(c, result, err)
}

func (a *ApiService) UninstallPanel(c *gin.Context) {
	result, err := panelUninstallScheduler(&a.PanelUninstallService)
	if err != nil {
		jsonMsg(c, "", err)
		return
	}
	jsonObj(c, result, nil)
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
