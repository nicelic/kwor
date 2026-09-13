package service

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/alireza0/s-ui/config"
	"github.com/alireza0/s-ui/logger"
	"github.com/alireza0/s-ui/util/common"
)

const (
	optimizationWatcherInterval = 10 * time.Second
	optimizationWatcherTimeout  = 8 * time.Second

	GoldenSysctlMain   = "sysctl.conf"
	GoldenSysctlDropIn = "99-s-ui-optimize.conf"
	GoldenJournald     = "journald.conf"
	GoldenDNS          = "resolv.conf"
)

// OptimizationWatchItem 定义单个受监视的配置项
type OptimizationWatchItem struct {
	Key            string
	DisplayName    string
	TargetPath     string
	GoldenFileName string
	OnCorrected    func(ctx context.Context, targetPath string) error
}

// SystemOptimizationWatcher 负责高频（10秒）轮询受管文件与母本的比对与自动纠正。
// 作为独立的后台守护线程运行，避免外部系统命令（如 sysctl -p 或 systemd 重启）阻塞核心采样。
//
// ============================================================================
// 架构隔离规范（严禁合并进主调度中枢 cronjob.RuntimeSampler）：
// 1. 职责边界：本监视器专注于 Linux 主机系统优化（sysctl、journald、resolv.conf 等）配置母本的防篡改巡检与自动纠正。
// 2. 命令开销：当检测到受管文件被外部篡改或异常时，需要调用 sysctl -p 或 systemctl restart systemd-journald
//    等外部系统命令，执行过程具有不可控的外部进程耗时与系统锁风险。
// 3. 隔离依据：主调度中枢 cronjob.RuntimeSampler 采用严格的相锁强串行机制，专职保障毫秒级网络流量记账与 nftables 完整性。
//    若将本监视器合并入 RuntimeSampler，外部系统命令调用的停顿将直接导致核心采样掉帧、错峰失效甚至引发死锁。
// ============================================================================
type SystemOptimizationWatcher struct {
	mu      sync.RWMutex
	items   map[string]*OptimizationWatchItem
	running bool
	stopCh  chan struct{}
	wakeCh  chan struct{}
	doneCh  chan struct{}
}

var (
	globalOptimizationWatcher     *SystemOptimizationWatcher
	globalOptimizationWatcherOnce sync.Once
)

func GetSystemOptimizationWatcher() *SystemOptimizationWatcher {
	globalOptimizationWatcherOnce.Do(func() {
		globalOptimizationWatcher = &SystemOptimizationWatcher{
			items:  make(map[string]*OptimizationWatchItem),
			wakeCh: make(chan struct{}, 1),
		}
	})
	return globalOptimizationWatcher
}

func GetOptimizationGoldenDir() string {
	return filepath.Join(config.GetDataDir(), "mtu")
}

func GetOptimizationGoldenPath(goldenFileName string) string {
	return filepath.Join(GetOptimizationGoldenDir(), filepath.Clean(goldenFileName))
}

func SaveOptimizationGoldenFile(goldenFileName string, content string) error {
	goldenFileName = strings.TrimSpace(goldenFileName)
	if goldenFileName == "" {
		return common.NewError("母本文件名不能为空")
	}
	dir := GetOptimizationGoldenDir()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return common.NewError("创建母本存储目录失败: ", err)
	}
	target := GetOptimizationGoldenPath(goldenFileName)
	if err := os.WriteFile(target, []byte(content), 0o644); err != nil {
		return common.NewError("写入母本文件失败: ", err)
	}
	logger.Infof("[SystemOptimizationWatcher] saved golden baseline file: %s", target)
	return nil
}

func RemoveOptimizationGoldenFile(goldenFileName string) error {
	goldenFileName = strings.TrimSpace(goldenFileName)
	if goldenFileName == "" {
		return nil
	}
	target := GetOptimizationGoldenPath(goldenFileName)
	if err := os.Remove(target); err != nil && !os.IsNotExist(err) {
		return common.NewError("删除母本文件失败: ", err)
	}
	logger.Infof("[SystemOptimizationWatcher] removed golden baseline file: %s", target)
	return nil
}

func ReadOptimizationGoldenFile(goldenFileName string) (string, error) {
	target := GetOptimizationGoldenPath(goldenFileName)
	data, err := os.ReadFile(target)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func (w *SystemOptimizationWatcher) Start() {
	if w == nil {
		return
	}
	w.mu.Lock()
	if w.running {
		w.mu.Unlock()
		return
	}
	w.running = true
	w.stopCh = make(chan struct{})
	w.doneCh = make(chan struct{})
	stopCh := w.stopCh
	doneCh := w.doneCh
	wakeCh := w.wakeCh
	w.mu.Unlock()

	w.restorePersistedGoldenWatches()

	go w.run(stopCh, wakeCh, doneCh)
	logger.Infof("[SystemOptimizationWatcher] started 10s polling watcher thread")
}

func (w *SystemOptimizationWatcher) restorePersistedGoldenWatches() {
	if !IsSystemPlatformLinux() {
		return
	}
	// sysctl main
	if pathExists(GetOptimizationGoldenPath(GoldenSysctlMain)) && !w.HasTarget("sysctl-main") {
		w.Register(OptimizationWatchItem{
			Key:            "sysctl-main",
			DisplayName:    "sysctl 主配置 (/etc/sysctl.conf)",
			TargetPath:     sysctlManagedMainPath,
			GoldenFileName: GoldenSysctlMain,
			OnCorrected: func(ctx context.Context, correctedPath string) error {
				return applySysctlFromManagedFilesContext(ctx, []string{correctedPath})
			},
		})
	}
	// sysctl drop-in
	if pathExists(GetOptimizationGoldenPath(GoldenSysctlDropIn)) && !w.HasTarget("sysctl-dropin") {
		w.Register(OptimizationWatchItem{
			Key:            "sysctl-dropin",
			DisplayName:    "sysctl 拓展配置 (/etc/sysctl.d/99-s-ui-optimize.conf)",
			TargetPath:     sysctlManagedDropInPath,
			GoldenFileName: GoldenSysctlDropIn,
			OnCorrected: func(ctx context.Context, correctedPath string) error {
				return applySysctlFromManagedFilesContext(ctx, []string{correctedPath})
			},
		})
	}
	// journald
	if pathExists(GetOptimizationGoldenPath(GoldenJournald)) && !w.HasTarget("journald") {
		w.Register(OptimizationWatchItem{
			Key:            "journald",
			DisplayName:    "journald 配置",
			TargetPath:     "/etc/systemd/journald.conf",
			GoldenFileName: GoldenJournald,
			OnCorrected: func(ctx context.Context, correctedPath string) error {
				return restartJournaldServiceContext(ctx)
			},
		})
	}
	// dns
	if pathExists(GetOptimizationGoldenPath(GoldenDNS)) && !w.HasTarget("dns") {
		w.Register(OptimizationWatchItem{
			Key:            "dns",
			DisplayName:    "resolv.conf 配置",
			TargetPath:     defaultSystemLinuxDNSPath,
			GoldenFileName: GoldenDNS,
			OnCorrected:    nil,
		})
	}
}

func (w *SystemOptimizationWatcher) Stop() {
	if w == nil {
		return
	}
	w.mu.Lock()
	if !w.running {
		w.mu.Unlock()
		return
	}
	w.running = false
	close(w.stopCh)
	doneCh := w.doneCh
	w.mu.Unlock()

	if doneCh != nil {
		<-doneCh
	}
	logger.Infof("[SystemOptimizationWatcher] stopped watcher thread")
}

func (w *SystemOptimizationWatcher) Wake() {
	if w == nil {
		return
	}
	w.mu.RLock()
	running := w.running
	wakeCh := w.wakeCh
	w.mu.RUnlock()
	if !running || wakeCh == nil {
		return
	}
	select {
	case wakeCh <- struct{}{}:
	default:
	}
}

func (w *SystemOptimizationWatcher) Register(item OptimizationWatchItem) {
	if w == nil || item.Key == "" {
		return
	}
	w.mu.Lock()
	copyItem := item
	w.items[item.Key] = &copyItem
	w.mu.Unlock()
	logger.Infof("[SystemOptimizationWatcher] registered watch target: %s (%s -> %s)", item.Key, item.TargetPath, item.GoldenFileName)
	w.Wake()
}

func (w *SystemOptimizationWatcher) Unregister(key string) {
	if w == nil || key == "" {
		return
	}
	w.mu.Lock()
	delete(w.items, key)
	w.mu.Unlock()
	logger.Infof("[SystemOptimizationWatcher] unregistered watch target: %s", key)
}

func (w *SystemOptimizationWatcher) HasTarget(key string) bool {
	if w == nil || key == "" {
		return false
	}
	w.mu.RLock()
	defer w.mu.RUnlock()
	_, exists := w.items[key]
	return exists
}

func (w *SystemOptimizationWatcher) run(stopCh <-chan struct{}, wakeCh <-chan struct{}, doneCh chan<- struct{}) {
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("[SystemOptimizationWatcher] worker panicked: %v", r)
		}
		close(doneCh)
	}()

	ticker := time.NewTicker(optimizationWatcherInterval)
	defer ticker.Stop()

	for {
		select {
		case <-stopCh:
			return
		case <-wakeCh:
			w.checkAndCorrectAll()
		case <-ticker.C:
			w.checkAndCorrectAll()
		}
	}
}

func (w *SystemOptimizationWatcher) checkAndCorrectAll() {
	w.mu.RLock()
	targets := make([]*OptimizationWatchItem, 0, len(w.items))
	for _, item := range w.items {
		targets = append(targets, item)
	}
	w.mu.RUnlock()

	if len(targets) == 0 {
		return
	}

	for _, item := range targets {
		w.checkAndCorrectItem(item)
	}
}

func (w *SystemOptimizationWatcher) checkAndCorrectItem(item *OptimizationWatchItem) {
	if item == nil || item.TargetPath == "" || item.GoldenFileName == "" {
		return
	}

	goldenContent, err := ReadOptimizationGoldenFile(item.GoldenFileName)
	if err != nil {
		// 母本文件若读取不到则无法进行比对，避免盲目重置
		return
	}

	// 规范化换行比对
	goldenNormalized := strings.ReplaceAll(goldenContent, "\r\n", "\n")

	needCorrection := false
	currentRaw, err := os.ReadFile(item.TargetPath)
	if err != nil {
		logger.Warningf("[SystemOptimizationWatcher] 受管文件 %s 读取失败 (%v)，判定为需要从母本纠正", item.TargetPath, err)
		needCorrection = true
	} else {
		currentNormalized := strings.ReplaceAll(string(currentRaw), "\r\n", "\n")
		if currentNormalized != goldenNormalized {
			logger.Warningf("[SystemOptimizationWatcher] 检测到 %s (%s) 被外部修改，与母本内容不一致，触发自动纠正", item.DisplayName, item.TargetPath)
			needCorrection = true
		}
	}

	if !needCorrection {
		return
	}

	// 执行纠正写入
	dir := filepath.Dir(item.TargetPath)
	if mkErr := os.MkdirAll(dir, 0o755); mkErr != nil {
		logger.Errorf("[SystemOptimizationWatcher] 纠正 %s 失败：创建目录失败: %v", item.TargetPath, mkErr)
		return
	}
	if writeErr := os.WriteFile(item.TargetPath, []byte(goldenNormalized), 0o644); writeErr != nil {
		logger.Errorf("[SystemOptimizationWatcher] 纠正 %s 失败：写回文件失败: %v", item.TargetPath, writeErr)
		return
	}

	logger.Infof("[SystemOptimizationWatcher] 成功自动纠正受管文件内容: %s", item.TargetPath)

	// 执行重载生效回调
	if item.OnCorrected != nil {
		ctx, cancel := context.WithTimeout(context.Background(), optimizationWatcherTimeout)
		defer cancel()
		if applyErr := item.OnCorrected(ctx, item.TargetPath); applyErr != nil {
			logger.Warningf("[SystemOptimizationWatcher] 纠正后执行生效命令失败 (%s): %v", item.TargetPath, applyErr)
		} else {
			logger.Infof("[SystemOptimizationWatcher] 纠正后成功重新应用生效: %s", item.TargetPath)
		}
	}
}

// StartSystemOptimizationWatcher 启动全局优化文件监视器
func StartSystemOptimizationWatcher() {
	GetSystemOptimizationWatcher().Start()
}

// StopSystemOptimizationWatcher 停止全局优化文件监视器
func StopSystemOptimizationWatcher() {
	GetSystemOptimizationWatcher().Stop()
}
