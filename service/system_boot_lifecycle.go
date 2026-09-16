package service

import (
	"context"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/logger"
	"gorm.io/gorm"
)

const (
	systemBootIDKey     = "systemBootID"
	systemLastBootAtKey = "systemLastBootAt"
)

var (
	bootIDPattern = regexp.MustCompile(`^[a-fA-F0-9-]{32,64}$`)
)

// ReadSystemBootID 读取操作系统内核生成的全局唯一 boot_id。
// 在 Linux 系统中读取 /proc/sys/kernel/random/boot_id。
// 若非 Linux 或受限环境，回退至从 /proc/stat 读取 btime。
func ReadSystemBootID() (string, error) {
	if !IsSystemPlatformLinux() {
		return "non-linux-host", nil
	}

	raw, err := os.ReadFile("/proc/sys/kernel/random/boot_id")
	if err == nil {
		bootID := strings.TrimSpace(string(raw))
		if bootID != "" && bootIDPattern.MatchString(bootID) {
			return bootID, nil
		}
	}

	// 回退机制：从 /proc/stat 解析 btime
	if statRaw, statErr := os.ReadFile("/proc/stat"); statErr == nil {
		lines := strings.Split(string(statRaw), "\n")
		for _, line := range lines {
			fields := strings.Fields(line)
			if len(fields) >= 2 && fields[0] == "btime" {
				return "btime-" + fields[1], nil
			}
		}
	}

	return "", err
}

// EvaluateBootLifecycleState 读取当前系统的 boot_id，与数据库持久化记录比对，
// 判定本次启动属于「整机重启（Host Reboot）」还是「仅面板进程重启（Panel Process Restart）」，
// 并将最新 boot_id 更新持久化至数据库。
func EvaluateBootLifecycleState() (isHostReboot bool, currentBootID string, err error) {
	currentBootID, err = ReadSystemBootID()
	if err != nil || currentBootID == "" {
		currentBootID = "unknown-boot-id"
	}

	var settingService SettingService
	lastBootID, _ := settingService.getString(systemBootIDKey)
	lastBootID = strings.TrimSpace(lastBootID)

	if lastBootID == "" {
		// 数据库初次记录，视为主机冷启动
		isHostReboot = true
	} else if lastBootID != currentBootID {
		// boot_id 发生变动，确认整机/操作系统发生过重启
		isHostReboot = true
	} else {
		// boot_id 未变，说明整机持续运行，仅面板进程发生重启
		isHostReboot = false
	}

	// 落盘持久化当前最新 boot_id
	db := database.GetDB()
	if db != nil {
		nowStr := time.Now().Format(time.RFC3339)
		_ = db.Transaction(func(tx *gorm.DB) error {
			for k, v := range map[string]string{
				systemBootIDKey:     currentBootID,
				systemLastBootAtKey: nowStr,
			} {
				setting := &model.Setting{}
				queryErr := tx.Model(model.Setting{}).Where("key = ?", k).First(setting).Error
				if database.IsNotFound(queryErr) {
					_ = tx.Create(&model.Setting{Key: k, Value: v}).Error
				} else if queryErr == nil {
					setting.Value = v
					_ = tx.Save(setting).Error
				}
			}
			return nil
		})
	}

	return isHostReboot, currentBootID, nil
}

// DispatchBootLifecycle 作为启动生命周期中枢的主入口，
// 按照严格的时序依赖，主动将启动事件链式分发给 4 大核心模块：
// 1. 物理网卡检测 (NetworkInterfaceService)
// 2. 流量管理 (TrafficOverviewService)
// 3. MTU 优化 (SystemMTUOptimizationService)
// 4. DDNS 同步 (DDNSRuntimeWorker)
func DispatchBootLifecycle() {
	isHostReboot, bootID, _ := EvaluateBootLifecycleState()
	rebootType := "Panel Process Restart (面板进程重启)"
	if isHostReboot {
		rebootType = "Host System Reboot (整机系统重启)"
	}
	logger.Infof("[BootLifecycle] Detected startup type: %s (boot_id=%s)", rebootType, bootID)

	// Step 1: 物理网卡检测 —— 优先全量扫描物理与模拟物理网卡，打牢底层网络基石
	logger.Info("[BootLifecycle] Step 1/4: Triggering physical/simulated network interface detection...")
	if ifaces, err := (&NetworkInterfaceService{}).SyncSystemInterfaces(); err != nil {
		logger.Warningf("[BootLifecycle] Step 1 failed (network interface sync): %v", err)
	} else {
		logger.Infof("[BootLifecycle] Step 1 done: synchronized %d network interface(s)", len(ifaces))
	}

	// Step 2: 流量管理 —— 比对物理网卡是否有物理变动，重新校准聚合列表并确保 vnstat 守护跟踪
	logger.Info("[BootLifecycle] Step 2/4: Reconciling traffic interfaces & vnstat monitoring...")
	if err := (&TrafficOverviewService{}).ReconcileInterfacesOnStartup(isHostReboot); err != nil {
		logger.Warningf("[BootLifecycle] Step 2 failed (traffic interfaces reconcile): %v", err)
	} else {
		logger.Info("[BootLifecycle] Step 2 done: traffic interfaces & daemon reconciled")
	}

	// Step 3: MTU 优化 —— 针对最新的物理与模拟物理网卡进行 MTU 配置核验与自愈
	logger.Info("[BootLifecycle] Step 3/4: Verifying & healing MTU configuration for physical interfaces...")
	if err := (&SystemMTUOptimizationService{}).OnSystemStartup(isHostReboot); err != nil {
		logger.Warningf("[BootLifecycle] Step 3 failed (MTU startup reconcile): %v", err)
	} else {
		logger.Info("[BootLifecycle] Step 3 done: MTU state verified & active")
	}

	// Step 4: DDNS 模块 —— 此时底层网络、网卡与 MTU 已全部就绪，唤醒调度器立即执行 IP 检测与域名解析
	logger.Info("[BootLifecycle] Step 4/4: Waking up DDNS runtime worker for immediate sync...")
	WakeDDNSRuntimeWorker()
	logger.Info("[BootLifecycle] Step 4 done: DDNS runtime worker signaled")

	logger.Info("[BootLifecycle] All startup lifecycle dispatches completed successfully")
}

// StartSystemBootLifecycleInBackground 以独立受控的守护线程在后台异步执行启动分发，
// 避免阻塞面板 Web Server 与基础 API 启动。
func StartSystemBootLifecycleInBackground(ctx context.Context) {
	go func() {
		operation, err := BeginKworInProcessOperation("startup-boot-lifecycle-dispatch")
		if err != nil {
			logger.Warning("[BootLifecycle] Startup dispatch skipped by lifecycle state:", err)
			return
		}
		defer operation.Done()

		// 给予底层系统网络协议栈 500ms 基础缓冲就绪时间
		time.Sleep(500 * time.Millisecond)
		DispatchBootLifecycle()
	}()
}
