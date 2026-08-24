package app

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/alireza0/s-ui/api"
	"github.com/alireza0/s-ui/config"
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
	coreManager   *service.CoreManagerService
	mihomoManager *service.MihomoCoreManagerService
	cronJob       *cronjob.CronJob
	logger        *logging.Logger
}

func NewApp() *APP {
	return &APP{}
}

func (a *APP) Init() error {
	log.Printf("%v %v", config.GetName(), config.GetVersion())
	if err := service.KworServiceStartAllowed(); err != nil {
		return err
	}

	a.initLog()

	err := database.InitDB(config.GetDBPath())
	if err != nil {
		return err
	}
	if database.HasPendingDBRestoreToApply() {
		if err := database.ApplyPendingDBRestore(); err != nil {
			return err
		}
	}
	if err := api.InitializeLoginSessionStore(); err != nil {
		return err
	}
	if _, err := service.RefreshSystemPlatform(); err != nil {
		return err
	}
	service.RefreshNftablesCapabilities()
	// Initialize the panel-owned timezone before generic settings defaults can
	// seed it. This performs the documented one-shot remote validation only at
	// process startup and keeps the resulting IANA name in SQLite.
	if err := a.SettingService.InitializePanelTimeOnStartup(); err != nil {
		return err
	}
	if err := a.prepareStartupData(); err != nil {
		return err
	}
	if err := refreshPanelRuntimeOwnershipOnStartup(); err != nil {
		return err
	}

	a.cronJob = cronjob.NewCronJob()
	a.coreManager = &service.CoreManagerService{}
	a.mihomoManager = &service.MihomoCoreManagerService{}
	a.webServer = a.newWebServer()
	a.subServer = sub.NewServer()
	a.reverseProxy = &service.ReverseProxyService{}
	service.RegisterPanelTLSRuntimeApplier(a)
	service.RegisterFirewallRuntimePortProvider(a)
	service.RegisterPanelTimeScheduleReloader(a)

	a.configService = service.NewConfigService()

	a.regenerateManagedRuntimeConfigs()

	return nil
}

// refreshPanelRuntimeOwnershipOnStartup 在每次服务实际启动后刷新二进制和
// 运行时附属文件的指纹。升级 worker 替换文件后会重启服务，因此无需依赖
// 已经可能消失的旧进程内存状态。
func refreshPanelRuntimeOwnershipOnStartup() error {
	binaryPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve panel executable for ownership refresh: %w", err)
	}
	if resolved, resolveErr := filepath.EvalSymlinks(binaryPath); resolveErr == nil {
		binaryPath = resolved
	}
	binaryPath = filepath.Clean(binaryPath)
	runtimePaths := []string{
		filepath.Join(filepath.Dir(binaryPath), "install.sh"),
		filepath.Join(filepath.Dir(binaryPath), "kwor.service"),
		config.GetRuntimeInstallScriptPath(),
		config.GetRuntimeServiceFilePath(),
		filepath.Join(config.GetRuntimeSupportDir(), "panel-update-last.log"),
	}
	if err := service.RefreshPanelHostOwnership(binaryPath, config.GetDataDir(), "kwor", runtimePaths); err != nil {
		return fmt.Errorf("refresh panel runtime ownership: %w", err)
	}
	unitPath := "/etc/systemd/system/kwor.service"
	if _, err := service.RefreshPanelSystemdHostOwnershipIfVerified("kwor", unitPath, binaryPath); err != nil {
		return fmt.Errorf("refresh panel systemd ownership: %w", err)
	}
	return nil
}

func (a *APP) Start() error {
	service.RegisterPanelTLSRuntimeApplier(a)
	if err := service.StartKworLifecycleControlServer(); err != nil {
		return err
	}

	service.SyncManagedNftablesOnStartup()

	err := a.ReloadPanelTimeSchedule()
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

	service.SyncPortForwardNftablesAfterListenersOnStartup()

	a.startTrafficOverviewRuntimeProbe()
	a.startManagedCoreOnLinuxStartup()
	if database.HasPendingDBRestoreToFinalize() {
		if err := database.FinalizePendingDBRestore(); err != nil {
			logger.Warning("finalize pending db restore failed:", err)
		}
	}

	return nil
}

func (a *APP) startManagedCoreOnLinuxStartup() {
	if !service.IsSystemPlatformLinux() {
		return
	}

	go func() {
		operation, err := service.BeginKworInProcessOperation("startup-managed-core-reconcile")
		if err != nil {
			logger.Warning("managed core startup reconcile skipped by lifecycle state:", err)
			return
		}
		defer operation.Done()
		time.Sleep(1200 * time.Millisecond)
		a.reconcileManagedCoreOnStartup(
			service.GetSingboxSystemdName(),
			a.coreManager,
			"sing-box",
		)
		a.reconcileManagedCoreOnStartup(
			service.GetMihomoSystemdName(),
			a.mihomoManager,
			"mihomo",
		)
		service.SyncPortForwardNftablesAfterCoreRuntimeReady()
	}()
}

func (a *APP) startTrafficOverviewRuntimeProbe() {
	go func() {
		operation, err := service.BeginKworInProcessOperation("startup-traffic-overview-probe")
		if err != nil {
			logger.Warning("traffic overview runtime probe skipped by lifecycle state:", err)
			return
		}
		defer operation.Done()
		if err := (&service.TrafficOverviewService{}).EnsureRuntimeReady(); err != nil {
			logger.Warning("traffic overview runtime prepare failed:", err)
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
		if err := service.WithCertificateCoreConfigGate(starter.StartCore); err != nil {
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
			if err := service.WithCertificateCoreConfigGate(starter.RestartCore); err != nil {
				logger.Warningf("%s startup auto-reconcile restart failed: %v", label, err)
			}
		}
		return
	}
	if err := service.WithCertificateCoreConfigGate(starter.StartCore); err != nil {
		logger.Warningf("%s startup auto-recover failed: %v", label, err)
	}
}

func isSystemdServiceEnabled(serviceName string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "systemctl", "is-enabled", "--quiet", serviceName)
	return cmd.Run() == nil
}

func disableSystemdServiceAutostart(serviceName string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "systemctl", "disable", serviceName)
	return cmd.Run()
}

func (a *APP) Stop() {
	service.RegisterPanelTLSRuntimeApplier(nil)
	service.RegisterPanelTimeScheduleReloader(nil)

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
	service.StopKworLifecycleControlServer()
}

// ReloadPanelTimeSchedule is called after a successful panel timezone save.
// CronJob.Start swaps and stops the prior scheduler under its own mutex, so no
// schedule goroutine is retained after a timezone change.
func (a *APP) ReloadPanelTimeSchedule() error {
	if a == nil || a.cronJob == nil {
		return nil
	}

	loc, err := a.SettingService.GetPanelTimeLocation()
	if err != nil {
		logger.Warning("get panel timezone failed, fallback to UTC:", err)
		loc = time.UTC
	}

	trafficAge, err := a.SettingService.GetTrafficAge()
	if err != nil {
		logger.Warning("get trafficAge failed, fallback to 30:", err)
		trafficAge = 30
	}
	return a.cronJob.Start(loc, trafficAge)
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
	// A full panel restart is an explicit authentication boundary. TLS listener
	// reloads use web.Server.Restart directly and intentionally skip this reset.
	api.InvalidateAllLoginSessions("panel_restart")
	a.Stop()
	restoreApplied := false

	if database.HasPendingDBRestoreToApply() {
		if err := database.ApplyPendingDBRestore(); err != nil {
			logger.Error("apply pending db restore failed:", err)
			if _, platformErr := service.RefreshSystemPlatform(); platformErr != nil {
				logger.Error("refresh system platform after database restore failure failed:", platformErr)
			} else {
				service.RefreshNftablesCapabilities()
			}
			a.webServer = a.newWebServer()
			a.subServer = sub.NewServer()
			if startErr := a.Start(); startErr != nil {
				logger.Error("restart app after restore failure failed:", startErr)
			}
			return
		}
		restoreApplied = true
	}
	if err := api.InitializeLoginSessionStore(); err != nil {
		logger.Error("reset persisted login sessions after panel restart failed:", err)
	}

	if restoreApplied {
		if _, err := service.RefreshSystemPlatform(); err != nil {
			logger.Error("refresh system platform after database restore failed:", err)
		} else {
			service.RefreshNftablesCapabilities()
		}
		if err := a.prepareStartupData(); err != nil {
			logger.Error("prepare startup data after db restore failed:", err)
			a.webServer = a.newWebServer()
			a.subServer = sub.NewServer()
			if startErr := a.Start(); startErr != nil {
				logger.Error("restart app after startup data reload failure failed:", startErr)
			}
			return
		}
		a.regenerateManagedRuntimeConfigs()
	} else if _, err := service.RefreshSystemPlatform(); err != nil {
		logger.Error("refresh system platform before panel restart failed:", err)
	} else {
		service.RefreshNftablesCapabilities()
	}
	// Recreate servers with fresh contexts so Start() works properly
	a.webServer = a.newWebServer()
	a.subServer = sub.NewServer()

	err := a.Start()
	if err != nil {
		logger.Error("restart app failed:", err)
	}
}

func (a *APP) newWebServer() *web.Server {
	server := web.NewServer()
	server.SetCoreManagers(a.coreManager, a.mihomoManager)
	service.RegisterCertificateCoreRestartManagers(a.coreManager, a.mihomoManager)
	return server
}

func (a *APP) prepareStartupData() error {
	if err := config.MigrateLegacyRuntimeSupportFiles(service.IsSystemPlatformLinux()); err != nil {
		logger.Warning("migrate legacy runtime support files failed:", err)
	}
	if err := service.InitManagedRuntimeFileStore(); err != nil {
		return err
	}
	if err := service.EnsureManagedCoreLayout(); err != nil {
		return err
	}
	if err := service.CleanupStaleManagedCoreRuntimeWorkspacesOnStartup(); err != nil {
		logger.Warning("cleanup stale managed Core workspaces on startup failed:", err)
	}
	if _, err := a.SettingService.GetAllSetting(); err != nil {
		return err
	}
	if migrated, migrateErr := service.MigrateLegacySingboxDNSServers(); migrateErr != nil {
		logger.Warning("migrate legacy sing-box DNS servers failed:", migrateErr)
	} else if migrated {
		logger.Info("migrated legacy sing-box DNS servers into DNS cards")
	}
	if migrated, migrateErr := service.MigrateLegacySingboxCoreDownloadPreference(); migrateErr != nil {
		logger.Warning("migrate legacy sing-box core download preference failed:", migrateErr)
	} else if migrated {
		logger.Info("removed legacy AMD64 level from sing-box core download preference")
	}
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
	if syncErr := service.RefreshAllCertificateBindingUsageFlags(); syncErr != nil {
		logger.Warning("refresh certificate binding usage flags failed:", syncErr)
	}
	if syncErr := service.SyncPanelTLSAssignments(&a.SettingService); syncErr != nil {
		logger.Warning("sync panel tls assignments failed:", syncErr)
	}
	if syncErr := (&service.AcmeService{}).MigrateLegacyAcmeRuntimeOnStartup(); syncErr != nil {
		logger.Warning("migrate legacy acme runtime failed:", syncErr)
	}
	if cleanupErr := service.CleanupStaleAcmeTempWorkspaces(); cleanupErr != nil {
		logger.Warning("cleanup stale acme temporary workspaces failed:", cleanupErr)
	}
	if syncErr := (&service.AcmeService{}).EnsureOverviewRuntimeConsistency(true); syncErr != nil {
		logger.Warning("prepare acme overview runtime consistency failed:", syncErr)
	}
	if syncErr := (&service.FirewallService{}).CleanupTemporaryRulesOnStartup(); syncErr != nil {
		logger.Warning("cleanup temporary firewall rules on startup failed:", syncErr)
	}
	if syncErr := service.PrepareHistoryStorageOnStartup(); syncErr != nil {
		logger.Warning("prepare history storage on startup failed:", syncErr)
	}
	if journalErr := service.PrepareTrafficRuntimeJournalOnStartup(); journalErr != nil {
		// A damaged sidecar must never prevent the panel from starting. The
		// journal loader quarantines invalid data; a transient SQLite failure is
		// retried by the runtime sampler on its first flush pass.
		logger.Warning("prepare traffic runtime journal on startup failed:", journalErr)
	}
	return nil
}

func (a *APP) regenerateManagedRuntimeConfigs() {
	proManager := service.NewProManagerService(a.configService)
	proManager.SetJsonService(&sub.JsonService{})
	proManager.SaveInboundJson()
	if err := service.NewMihomoManagerService().RegenerateServerConfig(); err != nil {
		logger.Warning("generate mihomo server config failed:", err)
	}
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
