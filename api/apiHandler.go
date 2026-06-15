package api

import (
	"strings"

	"github.com/alireza0/s-ui/util/common"

	"github.com/gin-gonic/gin"
)

type APIHandler struct {
	ApiService
	apiv2 *APIv2Handler
}

func NewAPIHandler(g *gin.RouterGroup, a2 *APIv2Handler) {
	a := &APIHandler{
		apiv2: a2,
	}
	a.initRouter(g)
}

func (a *APIHandler) initRouter(g *gin.RouterGroup) {
	g.Use(func(c *gin.Context) {
		path := c.Request.URL.Path
		if !strings.HasSuffix(path, "login") && !strings.HasSuffix(path, "logout") && !strings.HasSuffix(path, "session") {
			checkLogin(c)
		}
	})
	g.POST("/:postAction", a.postHandler)
	g.GET("/:getAction", a.getHandler)
}

func (a *APIHandler) postHandler(c *gin.Context) {
	loginUser := GetLoginUser(c)
	action := c.Param("postAction")

	switch action {
	case "login":
		a.ApiService.Login(c)
	case "changePass":
		a.ApiService.ChangePass(c)
	case "save":
		a.ApiService.Save(c, loginUser)
	case "restartApp":
		a.ApiService.RestartApp(c)
	case "restartSb":
		a.ApiService.RestartSb(c)
	case "linkConvert":
		a.ApiService.LinkConvert(c)
	case "syncToSubManager":
		a.ApiService.SyncToSubManager(c)
	case "mihomoSyncToSubManager":
		a.ApiService.SyncMihomoToSubManager(c)
	case "fetchSubscription":
		a.ApiService.FetchSubscription(c)
	case "refreshSubscription":
		a.ApiService.RefreshSubscription(c)
	case "subgroup-auto-update-settings":
		a.ApiService.SaveSubGroupAutoUpdateSettings(c)
	case "clearSubManager":
		a.ApiService.ClearSubManager(c)
	case "fetchOutboundSubscription":
		a.ApiService.FetchOutboundSubscription(c)
	case "refreshOutboundSubscription":
		a.ApiService.RefreshOutboundSubscription(c)
	case "fetchMihomoOutboundSubscription":
		a.ApiService.FetchMihomoOutboundSubscription(c)
	case "refreshMihomoOutboundSubscription":
		a.ApiService.RefreshMihomoOutboundSubscription(c)
	case "coreDownload":
		a.ApiService.DownloadCoreManager(c)
	case "coreStart":
		a.ApiService.StartCoreManager(c)
	case "coreStop":
		a.ApiService.StopCoreManager(c)
	case "coreRestart":
		a.ApiService.RestartCoreManager(c)
	case "coreDelete":
		a.ApiService.DeleteCoreManager(c)
	case "core-update-settings":
		a.ApiService.SaveCoreUpdateSettings(c)
	case "core-update-ack":
		a.ApiService.AckCoreUpdateNotice(c)
	case "core-download-preference":
		a.ApiService.SaveCoreDownloadPreference(c)
	case "mihomo-coreDownload":
		a.ApiService.DownloadMihomoCoreManager(c)
	case "mihomo-coreStart":
		a.ApiService.StartMihomoCoreManager(c)
	case "mihomo-coreStop":
		a.ApiService.StopMihomoCoreManager(c)
	case "mihomo-coreRestart":
		a.ApiService.RestartMihomoCoreManager(c)
	case "mihomo-coreDelete":
		a.ApiService.DeleteMihomoCoreManager(c)
	case "mihomo-core-update-settings":
		a.ApiService.SaveMihomoCoreUpdateSettings(c)
	case "mihomo-core-update-ack":
		a.ApiService.AckMihomoCoreUpdateNotice(c)
	case "mihomo-core-download-preference":
		a.ApiService.SaveMihomoCoreDownloadPreference(c)
	case "importdb":
		a.ApiService.ImportDb(c)
	case "addToken":
		a.ApiService.AddToken(c)
		a.apiv2.ReloadTokens()
	case "deleteToken":
		a.ApiService.DeleteToken(c)
		a.apiv2.ReloadTokens()
	case "tlsSha256":
		a.ApiService.GenerateTLSSha256(c)
	case "tlsFingerprint":
		a.ApiService.GenerateTLSFingerprint(c)
	case "tlsCertAlgorithm":
		a.ApiService.GenerateTLSCertAlgorithm(c)
	case "tlsSelfSignedTemplate":
		a.ApiService.DetectTLSSelfSignedTemplate(c)
	case "portOccupancy":
		a.ApiService.CheckPortOccupancy(c)
	case "traffic-overview-settings":
		a.ApiService.SaveTrafficOverviewSettings(c)
	case "traffic-overview-switch":
		a.ApiService.SaveTrafficOverviewSwitch(c)
	case "traffic-overview-reset":
		a.ApiService.ResetTrafficOverview(c)
	case "traffic-overview-vnstat-install":
		a.ApiService.InstallTrafficOverviewVnstat(c)
	case "traffic-overview-vnstat-remove":
		a.ApiService.RemoveTrafficOverviewVnstat(c)
	case "system-monitor-settings":
		a.ApiService.SaveSystemMonitorSettings(c)
	case "system-monitor-reset":
		a.ApiService.ResetSystemMonitorStats(c)
	case "firewall-switch":
		a.ApiService.SaveFirewallSwitch(c)
	case "firewall-nftables-install":
		a.ApiService.InstallFirewallNftables(c)
	case "firewall-ssh-port":
		a.ApiService.SaveFirewallSSHPort(c)
	case "firewall-ssh-proxy":
		a.ApiService.SaveFirewallSSHProxy(c)
	case "firewall-system-rule":
		a.ApiService.SaveFirewallSystemRule(c)
	case "firewall-rule":
		a.ApiService.SaveFirewallRule(c)
	case "firewall-rule-delete":
		a.ApiService.DeleteFirewallRule(c)
	case "firewall-geo-rule":
		a.ApiService.SaveFirewallGeoRule(c)
	case "firewall-geo-rule-delete":
		a.ApiService.DeleteFirewallGeoRule(c)
	case "firewall-geo-refresh":
		a.ApiService.RefreshFirewallGeoRules(c)
	case "firewall-geo-settings":
		a.ApiService.SaveFirewallGeoSettings(c)
	case "port-forward-rule":
		a.ApiService.SavePortForwardRule(c)
	case "port-forward-rule-delete":
		a.ApiService.DeletePortForwardRule(c)
	case "reverse-proxy-rule":
		a.ApiService.SaveReverseProxyRule(c)
	case "reverse-proxy-rule-delete":
		a.ApiService.DeleteReverseProxyRule(c)
	case "reverse-proxy-rule-reorder":
		a.ApiService.ReorderReverseProxyRules(c)
	case "kernel-download":
		a.ApiService.DownloadKernelPackages(c)
	case "kernel-install":
		a.ApiService.InstallKernelPackages(c)
	case "kernel-reboot":
		a.ApiService.RebootKernelHost(c)
	case "kernel-cleanup-purge":
		a.ApiService.PurgeKernelCleanupPackages(c)
	case "kernel-cleanup-auto":
		a.ApiService.AutoCleanupKernelPackages(c)
	case "kernel-cleanup-marker":
		a.ApiService.SaveKernelCleanupMarker(c)
	case "kernel-downloaded-clear":
		a.ApiService.ClearDownloadedKernel(c)
	case "system-log-optimization-switch":
		a.ApiService.SaveSystemLogOptimizationSwitch(c)
	case "system-log-optimization-content":
		a.ApiService.SaveSystemLogOptimizationContent(c)
	case "system-log-optimization-reset":
		a.ApiService.ResetSystemLogOptimizationContent(c)
	case "system-sysctl-optimization-switch":
		a.ApiService.SaveSystemSysctlOptimizationSwitch(c)
	case "system-sysctl-optimization-content":
		a.ApiService.SaveSystemSysctlOptimizationContent(c)
	case "system-sysctl-optimization-reset":
		a.ApiService.ResetSystemSysctlOptimizationContent(c)
	case "system-linux-dns-optimization-content":
		a.ApiService.SaveSystemLinuxDNSOptimizationContent(c)
	case "system-linux-dns-optimization-nameservers":
		a.ApiService.SaveSystemLinuxDNSOptimizationNameServers(c)
	case "system-mtu-optimization-switch":
		a.ApiService.SaveSystemMTUOptimizationSwitch(c)
	case "system-mtu-optimization-mtu":
		a.ApiService.SaveSystemMTUOptimizationMTU(c)
	case "acme-install":
		a.ApiService.InstallAcme(c)
	case "acme-remove":
		a.ApiService.RemoveAcme(c)
	case "acme-upgrade":
		a.ApiService.UpgradeAcme(c)
	case "acme-issue":
		a.ApiService.IssueAcmeCertificate(c)
	case "acme-renew":
		a.ApiService.RenewAcmeCertificate(c)
	case "acme-push":
		a.ApiService.PushAcmeCertificate(c)
	case "acme-set-auto-renew":
		a.ApiService.SetAcmeCertificateAutoRenew(c)
	case "acme-apply":
		a.ApiService.ApplyAcmeCertificate(c)
	case "acme-unapply":
		a.ApiService.UnapplyAcmeCertificate(c)
	case "acme-delete":
		a.ApiService.DeleteAcmeCertificate(c)
	case "certificate-list":
		a.ApiService.ListCertificates(c)
	case "certificate-material":
		a.ApiService.GetCertificateMaterial(c)
	case "acme-view":
		a.ApiService.ViewAcmeCertificate(c)
	case "acme-contact-email-save":
		a.ApiService.SaveAcmeContactEmail(c)
	case "acme-account-save":
		a.ApiService.SaveAcmeAccount(c)
	case "acme-account-delete":
		a.ApiService.DeleteAcmeAccount(c)
	case "acme-dns-account-save":
		a.ApiService.SaveAcmeDNSAccount(c)
	case "acme-dns-account-delete":
		a.ApiService.DeleteAcmeDNSAccount(c)
	case "self-signed-issue":
		a.ApiService.IssueSelfSignedCertificate(c)
	case "self-signed-authority-save":
		a.ApiService.SaveSelfSignedAuthority(c)
	case "self-signed-authority-delete":
		a.ApiService.DeleteSelfSignedAuthority(c)
	default:
		jsonMsg(c, "failed", common.NewError("unknown action: ", action))
	}
}

func (a *APIHandler) getHandler(c *gin.Context) {
	action := c.Param("getAction")

	switch action {
	case "logout":
		a.ApiService.Logout(c)
	case "session":
		a.ApiService.Session(c)
	case "load":
		a.ApiService.LoadData(c)
	case "mihomo-load":
		a.ApiService.LoadMihomoData(c)
	case "inbounds", "outbounds", "outboundgroups", "subgroups", "endpoints", "services", "tls", "clients", "config":
		err := a.ApiService.LoadPartialData(c, []string{action})
		if err != nil {
			jsonMsg(c, action, err)
		}
		return
	case "mihomo-inbounds":
		err := a.ApiService.LoadMihomoPartialData(c, []string{"mihomo_inbounds"})
		if err != nil {
			jsonMsg(c, action, err)
		}
		return
	case "mihomo-outbounds":
		err := a.ApiService.LoadMihomoPartialData(c, []string{"mihomo_outbounds"})
		if err != nil {
			jsonMsg(c, action, err)
		}
		return
	case "mihomo-outboundgroups":
		err := a.ApiService.LoadMihomoPartialData(c, []string{"mihomo_outboundgroups"})
		if err != nil {
			jsonMsg(c, action, err)
		}
		return
	case "mihomo-tls":
		err := a.ApiService.LoadMihomoPartialData(c, []string{"mihomo_tls"})
		if err != nil {
			jsonMsg(c, action, err)
		}
		return
	case "mihomo-clients":
		err := a.ApiService.LoadMihomoPartialData(c, []string{"mihomo_clients"})
		if err != nil {
			jsonMsg(c, action, err)
		}
		return
	case "mihomo-config":
		err := a.ApiService.LoadMihomoPartialData(c, []string{"mihomo_config"})
		if err != nil {
			jsonMsg(c, action, err)
		}
		return
	case "users":
		a.ApiService.GetUsers(c)
	case "settings":
		a.ApiService.GetSettings(c)
	case "stats":
		a.ApiService.GetStats(c)
	case "status":
		a.ApiService.GetStatus(c)
	case "system-monitor-overview":
		a.ApiService.GetSystemMonitorOverview(c)
	case "system-monitor-history":
		a.ApiService.GetSystemMonitorHistory(c)
	case "traffic-overview":
		a.ApiService.GetTrafficOverview(c)
	case "traffic-overview-vnstat-versions":
		a.ApiService.GetTrafficOverviewVnstatVersions(c)
	case "firewall-overview":
		a.ApiService.GetFirewallOverview(c)
	case "port-forward-overview":
		a.ApiService.GetPortForwardOverview(c)
	case "reverse-proxy-overview":
		a.ApiService.GetReverseProxyOverview(c)
	case "kernel-overview":
		a.ApiService.GetKernelOverview(c)
	case "kernel-versions":
		a.ApiService.GetKernelVersions(c)
	case "kernel-arches":
		a.ApiService.GetKernelArches(c)
	case "kernel-packages":
		a.ApiService.GetKernelPackages(c)
	case "kernel-cleanup-scan":
		a.ApiService.GetKernelCleanupScan(c)
	case "kernel-download-progress":
		a.ApiService.GetKernelDownloadProgress(c)
	case "system-log-optimization-overview":
		a.ApiService.GetSystemLogOptimizationOverview(c)
	case "system-sysctl-optimization-overview":
		a.ApiService.GetSystemSysctlOptimizationOverview(c)
	case "system-linux-dns-optimization-overview":
		a.ApiService.GetSystemLinuxDNSOptimizationOverview(c)
	case "system-mtu-optimization-overview":
		a.ApiService.GetSystemMTUOptimizationOverview(c)
	case "acme-overview":
		a.ApiService.GetAcmeOverview(c)
	case "acme-versions":
		a.ApiService.GetAcmeVersions(c)
	case "acme-update-info":
		a.ApiService.GetAcmeUpdateInfo(c)
	case "acme-ip-port-status":
		a.ApiService.GetAcmeIPPortStatus(c)
	case "certificate-list":
		a.ApiService.ListCertificates(c)
	case "tlsSelfSignedTemplates":
		a.ApiService.GetTLSSelfSignedTemplates(c)
	case "self-signed-authorities":
		a.ApiService.GetSelfSignedAuthorities(c)
	case "acme-log":
		a.ApiService.GetAcmeLog(c)
	case "onlines":
		a.ApiService.GetOnlines(c)
	case "logs":
		a.ApiService.GetLogs(c)
	case "changes":
		a.ApiService.CheckChanges(c)
	case "keypairs":
		a.ApiService.GetKeypairs(c)
	case "getdb":
		a.ApiService.GetDb(c)
	case "tokens":
		a.ApiService.GetTokens(c)
	case "singbox-config":
		a.ApiService.GetSingboxConfig(c)
	case "server-ips":
		a.ApiService.GetServerIPs(c)
	case "inbound-ips":
		a.ApiService.GetInboundIPs(c)
	case "mihomo-inbound-ips":
		a.ApiService.GetMihomoInboundIPs(c)
	case "core-status":
		a.ApiService.GetCoreManagerStatus(c)
	case "core-versions":
		a.ApiService.GetCoreRemoteVersions(c)
	case "core-update-info":
		a.ApiService.GetCoreUpdateInfo(c)
	case "core-download-progress":
		a.ApiService.GetCoreDownloadProgress(c)
	case "subgroup-auto-update-info":
		a.ApiService.GetSubGroupAutoUpdateInfo(c)
	case "mihomo-core-status":
		a.ApiService.GetMihomoCoreManagerStatus(c)
	case "mihomo-core-versions":
		a.ApiService.GetMihomoCoreRemoteVersions(c)
	case "mihomo-core-update-info":
		a.ApiService.GetMihomoCoreUpdateInfo(c)
	case "mihomo-core-download-progress":
		a.ApiService.GetCoreDownloadProgress(c)
	default:
		jsonMsg(c, "failed", common.NewError("unknown action: ", action))
	}
}
