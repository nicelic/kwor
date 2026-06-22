package app

import (
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"time"

	"github.com/alireza0/s-ui/config"
	// "github.com/alireza0/s-ui/core"
	"github.com/alireza0/s-ui/cronjob"
	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/logger"
	"github.com/alireza0/s-ui/service"
	"github.com/alireza0/s-ui/sub"
	"github.com/alireza0/s-ui/web"

	"github.com/op/go-logging"
)

type APP struct {
	service.SettingService
	configService *service.ConfigService
	webServer     *web.Server
	subServer     *sub.Server
	reverseProxy  *service.ReverseProxyService
	cronJob       *cronjob.CronJob
	logger        *logging.Logger
	core          interface{} // *core.Core
}

func NewApp() *APP {
	return &APP{}
}

func (a *APP) Init() error {
	log.Printf("%v %v", config.GetName(), config.GetVersion())

	a.initLog()

	err := database.InitDB(config.GetDBPath())
	if err != nil {
		return err
	}
	if err := service.InitManagedRuntimeFileStore(); err != nil {
		return err
	}
	if err := service.InitSystemMonitorStore(); err != nil {
		logger.Warning("init system monitor store failed:", err)
	}
	if err := service.EnsureManagedCoreLayout(); err != nil {
		return err
	}

	// Init Setting
	a.SettingService.GetAllSetting()
	if reconcileErr := service.ReconcileSystemOptimizationOnStartup(); reconcileErr != nil {
		logger.Warning("reconcile managed system optimization on startup failed:", reconcileErr)
	}
	if updated, migrateErr := service.MigrateLegacySubscriptionSelectorTags(); migrateErr != nil {
		logger.Warning("normalize legacy subscription selector tags failed:", migrateErr)
	} else if updated > 0 {
		logger.Infof("normalized legacy subscription selector tags in settings: %d", updated)
	}
	if migrateErr := service.MigrateLegacyPanelSQLiteCertificatesToInventory(); migrateErr != nil {
		logger.Warning("migrate legacy sqlite self-signed certificates failed:", migrateErr)
	}
	if migrateErr := service.MigrateLegacySettingsPathCertificatesToInventory(&a.SettingService); migrateErr != nil {
		logger.Warning("migrate legacy settings-path certificates failed:", migrateErr)
	}
	if repairErr := (&service.CertificateInventoryService{}).RepairDisplayIDs(); repairErr != nil {
		logger.Warning("repair certificate display ids failed:", repairErr)
	}
	if syncErr := service.SyncPanelTLSAssignments(&a.SettingService); syncErr != nil {
		logger.Warning("sync panel tls assignments failed:", syncErr)
	}
	if syncErr := (&service.AcmeService{}).MigrateLegacyDNSSecretsOnStartup(); syncErr != nil {
		logger.Warning("migrate legacy acme dns secrets failed:", syncErr)
	}
	if syncErr := (&service.AcmeService{}).EnsureOverviewRuntimeConsistency(true); syncErr != nil {
		logger.Warning("prepare acme overview runtime consistency failed:", syncErr)
	}
	if syncErr := (&service.FirewallService{}).CleanupTemporaryRulesOnStartup(); syncErr != nil {
		logger.Warning("cleanup temporary firewall rules on startup failed:", syncErr)
	}

	// a.core = core.NewCore()

	a.cronJob = cronjob.NewCronJob()
	a.webServer = web.NewServer()
	a.subServer = sub.NewServer()
	a.reverseProxy = &service.ReverseProxyService{}
	service.RegisterPanelTLSRuntimeApplier(a)
	service.RegisterFirewallRuntimePortProvider(a)

	a.configService = service.NewConfigService(a.core)

	// Sync generated configs to ProManager files during startup.
	proManager := service.NewProManagerService(a.configService)
	proManager.SetJsonService(&sub.JsonService{})
	proManager.SaveInboundJson()
	if err := service.NewMihomoManagerService().RegenerateServerConfig(); err != nil {
		logger.Warning("generate mihomo server config failed:", err)
	}

	return nil
}

func (a *APP) Start() error {
	service.RegisterPanelTLSRuntimeApplier(a)

	service.SyncManagedNftablesOnStartup()

	loc, err := a.SettingService.GetTimeLocation()
	if err != nil {
		logger.Warning("get time location failed, fallback to Local:", err)
		loc = time.Local
	}

	trafficAge, err := a.SettingService.GetTrafficAge()
	if err != nil {
		logger.Warning("get trafficAge failed, fallback to 30:", err)
		trafficAge = 30
	}

	err = a.cronJob.Start(loc, trafficAge)
	if err != nil {
		return err
	}

	err = a.webServer.Start()
	if err != nil {
		return err
	}

	err = a.subServer.Start()
	if err != nil {
		// Keep panel available even if subscription service fails.
		logger.Warning("Sub server start failed, panel keeps running:", err)
	}

	if a.reverseProxy != nil {
		if rpErr := a.reverseProxy.StartRuntime(); rpErr != nil {
			logger.Warning("reverse proxy runtime start failed:", rpErr)
		}
	}

	a.startTrafficOverviewRuntimeProbe()
	a.startSystemMonitorRuntimeProbe()
	a.startManagedCoreOnLinuxStartup()

	// err = a.configService.StartCore("")
	// if err != nil {
	// 	logger.Error(err)
	// }

	return nil
}

func (a *APP) startManagedCoreOnLinuxStartup() {
	if runtime.GOOS != "linux" {
		return
	}

	go func() {
		time.Sleep(1200 * time.Millisecond)
		a.reconcileManagedCoreOnStartup(
			service.GetSingboxSystemdName(),
			&service.CoreManagerService{},
			"sing-box",
		)
		a.reconcileManagedCoreOnStartup(
			service.GetMihomoSystemdName(),
			&service.MihomoCoreManagerService{},
			"mihomo",
		)
	}()
}

func (a *APP) startTrafficOverviewRuntimeProbe() {
	go func() {
		if err := (&service.TrafficOverviewService{}).EnsureRuntimeReady(); err != nil {
			logger.Warning("traffic overview runtime prepare failed:", err)
		}
	}()
}

func (a *APP) startSystemMonitorRuntimeProbe() {
	go func() {
		if err := (&service.SystemMonitorService{}).EnsureRuntimeReady(); err != nil {
			logger.Warning("system monitor runtime prepare failed:", err)
		}
	}()
}

type managedCoreController interface {
	IsRunning() bool
	StartCore() error
	RestartCore() error
}

func (a *APP) reconcileManagedCoreOnStartup(serviceName string, starter managedCoreController, label string) {
	if service.ShouldRecoverManagedCoreOnStartup(label) {
		if starter.IsRunning() {
			return
		}
		if err := starter.StartCore(); err != nil {
			logger.Warningf("%s startup auto-recover failed: %v", label, err)
		}
		return
	}

	servicePath := filepath.Join("/etc/systemd/system", serviceName+".service")
	if _, err := os.Stat(servicePath); err != nil {
		return
	}

	wasEnabled := isSystemdServiceEnabled(serviceName)
	if wasEnabled {
		if err := disableSystemdServiceAutostart(serviceName); err != nil {
			logger.Warningf("disable %s auto-start failed: %v", serviceName, err)
		}
	}

	if starter.IsRunning() {
		// If service was previously enabled, it might have started before panel startup.
		// Restart once so the startup path always uses freshly generated runtime config.
		if wasEnabled {
			if err := starter.RestartCore(); err != nil {
				logger.Warningf("%s startup auto-reconcile restart failed: %v", label, err)
			}
		}
		return
	}
	if err := starter.StartCore(); err != nil {
		logger.Warningf("%s startup auto-recover failed: %v", label, err)
	}
}

func isSystemdServiceEnabled(serviceName string) bool {
	cmd := exec.Command("systemctl", "is-enabled", "--quiet", serviceName)
	return cmd.Run() == nil
}

func disableSystemdServiceAutostart(serviceName string) error {
	cmd := exec.Command("systemctl", "disable", serviceName)
	return cmd.Run()
}

func (a *APP) Stop() {
	service.RegisterPanelTLSRuntimeApplier(nil)

	a.cronJob.Stop()

	panelOnlyStop := service.ConsumePanelStopOnlyMarker()
	if panelOnlyStop {
		logger.Info("skip managed nftables cleanup for panel-only stop")
	} else {
		// Cleanup nftables rules before stopping servers and process runtime.
		service.CleanupManagedNftablesOnShutdown()
	}
	if err := (&service.TrafficOverviewService{}).FlushPendingSnapshot(); err != nil {
		logger.Warning("flush traffic overview snapshot on shutdown failed:", err)
	}

	err := a.subServer.Stop()
	if err != nil {
		logger.Warning("stop Sub Server err:", err)
	}
	err = a.webServer.Stop()
	if err != nil {
		logger.Warning("stop Web Server err:", err)
	}
	if a.reverseProxy != nil {
		if rpErr := a.reverseProxy.StopRuntime(); rpErr != nil {
			logger.Warning("stop reverse proxy runtime err:", rpErr)
		}
	}
	// err = a.configService.StopCore()
	// if err != nil {
	// 	logger.Warning("stop Core err:", err)
	// }
}

func (a *APP) initLog() {
	switch config.GetLogLevel() {
	case config.Debug:
		logger.InitLogger(logging.DEBUG)
	case config.Info:
		logger.InitLogger(logging.INFO)
	case config.Warn:
		logger.InitLogger(logging.WARNING)
	case config.Error:
		logger.InitLogger(logging.ERROR)
	default:
		log.Fatal("unknown log level:", config.GetLogLevel())
	}
}

func (a *APP) RestartApp() {
	if err := service.MarkPanelStopOnly(); err != nil {
		logger.Error("prepare panel-only restart failed:", err)
		return
	}
	a.Stop()

	// Recreate servers with fresh contexts so Start() works properly
	a.webServer = web.NewServer()
	a.subServer = sub.NewServer()

	err := a.Start()
	if err != nil {
		logger.Error("restart app failed:", err)
	}
}

func (a *APP) GetCore() interface{} {
	return a.core
}

func (a *APP) GetActivePanelPort() int {
	if a.webServer == nil {
		return 0
	}
	return a.webServer.CurrentPort()
}

func (a *APP) GetActiveSubPort() int {
	if a.subServer == nil {
		return 0
	}
	return a.subServer.CurrentPort()
}

func (a *APP) ApplyPanelTLSSettings(target service.PanelSelfSignedTarget) error {
	switch target {
	case service.PanelSelfSignedTargetPanel:
		return a.applyTargetTLSSettings(target, a.webServer.TLSState, a.webServer.ReloadTLSCertificateMaterials, a.webServer.Restart, true)
	case service.PanelSelfSignedTargetSub:
		return a.applyTargetTLSSettings(target, a.subServer.TLSState, a.subServer.ReloadTLSCertificateMaterials, a.subServer.Restart, false)
	default:
		return nil
	}
}

func (a *APP) applyTargetTLSSettings(
	target service.PanelSelfSignedTarget,
	tlsState func() (bool, string, time.Time),
	reloadMaterials func([]*service.PanelTLSMaterial) (string, error),
	restart func() error,
	asyncRestart bool,
) error {
	materials, _, err := service.EnsurePanelTLSMaterials(&a.SettingService, target, time.Now())
	if err != nil {
		return err
	}
	if len(materials) == 0 {
		return nil
	}

	active, _, _ := tlsState()
	if active {
		_, err := reloadMaterials(materials)
		return err
	}

	restartFunc := func() {
		time.Sleep(300 * time.Millisecond)
		if restartErr := restart(); restartErr != nil {
			logger.Warningf("restart %s server for tls apply failed: %v", target, restartErr)
		}
	}
	if asyncRestart {
		go restartFunc()
		return nil
	}
	restartFunc()
	return nil
}

func (a *APP) DrainPanelTLSConnectionsByFingerprint(target service.PanelSelfSignedTarget, fingerprint string, gracePeriod time.Duration) error {
	switch target {
	case service.PanelSelfSignedTargetPanel:
		if a.webServer == nil {
			return nil
		}
		a.webServer.DrainTLSConnectionsByFingerprint(fingerprint, gracePeriod)
	case service.PanelSelfSignedTargetSub:
		if a.subServer == nil {
			return nil
		}
		a.subServer.DrainTLSConnectionsByFingerprint(fingerprint, gracePeriod)
	}
	return nil
}
