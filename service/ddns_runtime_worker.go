package service

import (
	"context"
	"runtime"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/logger"
)

const (
	// ddnsDetectorWorkerCount 快路径高频探测并发 Worker 数量
	ddnsDetectorWorkerCount = 4
	// ddnsSyncerWorkerCount 慢路径云商 DNS 同步并发 Worker 数量
	ddnsSyncerWorkerCount = 2
	// ddnsQueueCapacity 队列缓冲容量
	ddnsQueueCapacity = 64
	// ddnsSchedulerInterval 调度器心跳周期
	ddnsSchedulerInterval = 30 * time.Second
	// ddnsDefaultIdleInterval 无启用规则时的休眠周期
	ddnsDefaultIdleInterval = 30 * time.Second
)

type ddnsDetectTask struct {
	rule  *model.DDNSRule
	force bool
}

type ddnsSyncTask struct {
	rule  *model.DDNSRule
	plan  DDNSSyncPlan
	force bool
}

// ============================================================================
// 架构隔离规范（严禁合并进主调度中枢 cronjob.RuntimeSampler）：
// 1. 独立运行架构：本调度器运行在专属锁定 OS 物理线程 (runtime.LockOSThread)，并维护快慢分离并发 Worker 池
//    （4 探测 Worker + 2 云商同步 Worker），与主协程调度池物理隔离。
// 2. 独立数据库保护：独享完全独立的物理 SQLite 数据库 Promanager_data/db/ddns.db (database.GetDDNSDB())，
//    绝不与主库 s-ui.db 争抢连接与锁。
// 3. 外部网络 I/O 隔离：DDNS 涉及公网多源 IP HTTP 探测与 34 家第三方云厂商 OpenAPI 交互，具备不可控的外部网络延迟（最高 45s 超时）。
// 4. 隔离依据：主调度中枢 cronjob.RuntimeSampler 采用严格的相锁强串行化机制保障面板毫秒级流量记账与 nftables 完整性。
//    严禁将 DDNS 任务合并入 RuntimeSampler，以防任何外部网络抖动或超时直接拖死面板核心网络中枢！
// ============================================================================
type ddnsRuntimeWorker struct {
	mu            sync.Mutex
	running       bool
	stopping      bool
	isInitialPass bool // 标记开机启动后首轮扫描，确保重启后第一时间对齐探测
	stopCh        chan struct{}
	doneCh        chan struct{}
	completeCh    chan struct{}
	wakeCh        chan struct{}

	service    *DDNSService
	ruleGuards sync.Map // key: uint (rule.Id), value: *atomic.Bool

	detectQueue chan *ddnsDetectTask
	syncQueue   chan *ddnsSyncTask
	workerWg    sync.WaitGroup
}

var globalDDNSWorker = &ddnsRuntimeWorker{
	service: &DDNSService{},
}

func StartDDNSRuntimeWorker() {
	globalDDNSWorker.Start()
}

func StopDDNSRuntimeWorker() {
	globalDDNSWorker.StopAndWait()
}

func WakeDDNSRuntimeWorker() {
	globalDDNSWorker.Wake()
}

func (w *ddnsRuntimeWorker) Start() {
	if w == nil {
		return
	}
	for {
		w.mu.Lock()
		if w.running {
			w.mu.Unlock()
			w.Wake()
			return
		}
		if w.stopping {
			completeCh := w.completeCh
			w.mu.Unlock()
			if completeCh != nil {
				<-completeCh
			}
			continue
		}

		w.running = true
		w.isInitialPass = true
		w.stopCh = make(chan struct{})
		w.doneCh = make(chan struct{})
		w.completeCh = make(chan struct{})
		w.wakeCh = make(chan struct{}, 1)
		w.detectQueue = make(chan *ddnsDetectTask, ddnsQueueCapacity)
		w.syncQueue = make(chan *ddnsSyncTask, ddnsQueueCapacity)

		stopCh := w.stopCh
		wakeCh := w.wakeCh
		doneCh := w.doneCh
		w.mu.Unlock()

		// 启动专属 OS 物理线程绑定的核心调度器
		go w.runScheduler(stopCh, wakeCh, doneCh)

		// 启动快慢分离工作池（并发探测池与并发同步池）
		w.startWorkerPool(stopCh)

		logger.Info("[DDNS] Runtime worker started on dedicated OS thread with fast/slow worker pool")
		return
	}
}

func (w *ddnsRuntimeWorker) StopAndWait() {
	if w == nil {
		return
	}
	w.mu.Lock()
	if !w.running && !w.stopping {
		w.mu.Unlock()
		return
	}
	if w.stopping {
		completeCh := w.completeCh
		w.mu.Unlock()
		if completeCh != nil {
			<-completeCh
		}
		return
	}

	doneCh := w.doneCh
	completeCh := w.completeCh
	stopCh := w.stopCh
	w.running = false
	w.stopping = true
	if stopCh != nil {
		close(stopCh)
	}
	w.mu.Unlock()

	// 等待专属调度线程退出
	if doneCh != nil {
		<-doneCh
	}

	// 等待探测与同步工作池所有正在运行的任务排空退出
	w.workerWg.Wait()

	w.mu.Lock()
	if w.doneCh == doneCh {
		w.stopCh = nil
		w.doneCh = nil
		w.completeCh = nil
		w.wakeCh = nil
		w.detectQueue = nil
		w.syncQueue = nil
		w.stopping = false
		if completeCh != nil {
			close(completeCh)
		}
	}
	w.mu.Unlock()
	logger.Info("[DDNS] Runtime worker stopped cleanly")
}

func (w *ddnsRuntimeWorker) Stop() {
	w.StopAndWait()
}

func (w *ddnsRuntimeWorker) Wake() {
	if w == nil {
		return
	}
	w.mu.Lock()
	wakeCh := w.wakeCh
	running := w.running
	w.mu.Unlock()
	if !running || wakeCh == nil {
		return
	}
	select {
	case wakeCh <- struct{}{}:
	default:
	}
}

// runScheduler 在专用的操作系统内核物理线程（OS Thread M）上运行主调度循环
func (w *ddnsRuntimeWorker) runScheduler(stopCh <-chan struct{}, wakeCh <-chan struct{}, doneCh chan<- struct{}) {
	// 锁定当前 Goroutine 到专用的物理 OS 线程，实现内核级线程隔离
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	defer func() {
		if recovered := recover(); recovered != nil {
			logger.Error("[DDNS] Runtime scheduler panicked: ", recovered)
		}
		close(doneCh)
	}()

	// 启动后延迟 1 秒进行首次扫描，保证主程序其他系统资源准备就绪
	select {
	case <-time.After(1 * time.Second):
	case <-stopCh:
		return
	}

	delay := w.schedulePass()
	timer := time.NewTimer(delay)
	defer timer.Stop()

	for {
		select {
		case <-stopCh:
			return
		case <-wakeCh:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			delay = w.schedulePass()
			timer.Reset(delay)
		case <-timer.C:
			delay = w.schedulePass()
			timer.Reset(delay)
		}
	}
}

func (w *ddnsRuntimeWorker) schedulePass() time.Duration {
	operation, err := BeginKworInProcessOperation("ddns-runtime-scheduler")
	if err != nil {
		return ddnsDefaultIdleInterval
	}
	defer operation.Done()

	db := database.GetDDNSDB()
	if db == nil {
		return ddnsDefaultIdleInterval
	}

	var rules []model.DDNSRule
	if err := db.Where("enabled = ?", true).Find(&rules).Error; err != nil {
		return ddnsDefaultIdleInterval
	}

	if len(rules) == 0 {
		return ddnsDefaultIdleInterval
	}

	w.mu.Lock()
	initialPass := w.isInitialPass
	w.isInitialPass = false
	w.mu.Unlock()

	now := time.Now()
	nextDelay := ddnsDefaultIdleInterval
	for i := range rules {
		rule := &rules[i]
		ruleInterval := w.getRuleInterval(rule)
		if initialPass || w.shouldDetectRuleWithInterval(rule, now, ruleInterval) {
			// 防重叠守卫：检查该规则是否已有探测或同步正在运行
			guardVal, _ := w.ruleGuards.LoadOrStore(rule.Id, &atomic.Bool{})
			guard := guardVal.(*atomic.Bool)
			if !guard.CompareAndSwap(false, true) {
				// 上一轮任务尚未结束，跳过本次调度，避免因网络抖动堆叠
				continue
			}

			task := &ddnsDetectTask{
				rule:  rule,
				force: false,
			}

			select {
			case w.detectQueue <- task:
			default:
				// 探测队列满保护：释放守卫并告警
				guard.Store(false)
				logger.Warningf("[DDNS] Detect queue full, skipping rule %s", rule.Name)
			}
			if ruleInterval < nextDelay {
				nextDelay = ruleInterval
			}
		} else {
			if rule.LastSyncTime != nil {
				rem := ruleInterval - now.Sub(*rule.LastSyncTime)
				if rem > 0 && rem < nextDelay {
					nextDelay = rem
				}
			}
		}
	}

	if nextDelay < 2*time.Second {
		nextDelay = 2 * time.Second
	} else if nextDelay > ddnsDefaultIdleInterval {
		nextDelay = ddnsDefaultIdleInterval
	}
	return nextDelay
}

func (w *ddnsRuntimeWorker) getRuleInterval(rule *model.DDNSRule) time.Duration {
	needV4 := rule.IPType == "ipv4" || rule.IPType == "dual"
	needV6 := rule.IPType == "ipv6" || rule.IPType == "dual"
	hasInterface := (needV4 && strings.Contains(rule.IPV4Source, "interface")) || (needV6 && strings.Contains(rule.IPV6Source, "interface"))

	var interval time.Duration
	if hasInterface {
		sec := rule.LocalIntervalSeconds
		if sec <= 0 {
			sec = 10
		}
		interval = time.Duration(sec) * time.Second
	} else {
		min := rule.IntervalMinutes
		if min <= 0 {
			min = 1
		}
		interval = time.Duration(min) * time.Minute
	}

	// 故障快速自愈机制：若规则处于异常状态（如开机网络未就绪或API推送失败），启用10秒快速重试周期加速收敛
	if rule.LastStatus == "error" {
		errorRetryInterval := 10 * time.Second
		if errorRetryInterval < interval {
			interval = errorRetryInterval
		}
	}
	return interval
}

func (w *ddnsRuntimeWorker) shouldDetectRuleWithInterval(rule *model.DDNSRule, now time.Time, interval time.Duration) bool {
	if rule.LastSyncTime == nil {
		return true
	}
	return now.Sub(*rule.LastSyncTime) >= interval
}

func (w *ddnsRuntimeWorker) shouldDetectRule(rule *model.DDNSRule, now time.Time) bool {
	return w.shouldDetectRuleWithInterval(rule, now, w.getRuleInterval(rule))
}

func (w *ddnsRuntimeWorker) releaseRuleGuard(ruleID uint) {
	if val, ok := w.ruleGuards.Load(ruleID); ok {
		val.(*atomic.Bool).Store(false)
	}
}

func (w *ddnsRuntimeWorker) startWorkerPool(stopCh <-chan struct{}) {
	// 启动快路径并发探测 Worker 池
	for i := 0; i < ddnsDetectorWorkerCount; i++ {
		w.workerWg.Add(1)
		go w.runDetectorWorker(stopCh)
	}

	// 启动慢路径并发云商同步 Worker 池
	for i := 0; i < ddnsSyncerWorkerCount; i++ {
		w.workerWg.Add(1)
		go w.runSyncerWorker(stopCh)
	}
}

func (w *ddnsRuntimeWorker) runDetectorWorker(stopCh <-chan struct{}) {
	defer w.workerWg.Done()
	defer func() {
		if recovered := recover(); recovered != nil {
			logger.Error("[DDNS] Detector worker panicked: ", recovered)
		}
	}()

	for {
		select {
		case <-stopCh:
			return
		case task, ok := <-w.detectQueue:
			if !ok {
				return
			}
			w.processDetectTask(task)
		}
	}
}

func (w *ddnsRuntimeWorker) processDetectTask(task *ddnsDetectTask) {
	rule := task.rule
	if rule == nil {
		return
	}

	operation, err := BeginKworInProcessOperation("ddns-detect-worker")
	if err != nil {
		w.releaseRuleGuard(rule.Id)
		return
	}
	defer operation.Done()

	// 独立探测超时（本地网卡微秒级，公网接口最多 10 秒超时）
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	currentV4s, currentV6s, detectErrors := w.service.detectRuleIPs(ctx, rule)

	if len(detectErrors) > 0 && len(currentV4s) == 0 && len(currentV6s) == 0 {
		errMsg := strings.Join(detectErrors, "; ")
		w.service.updateRuleStatus(rule.Id, "error", errMsg, rule.LastIPV4, rule.LastIPV6)
		w.releaseRuleGuard(rule.Id)
		return
	}

	plan := w.service.CalculateSyncPlan(rule, currentV4s, currentV6s, detectErrors)

	// 【快慢分离关键短路】：IP 未发生变化且非首次同步且上次非错误状态，微秒级短路返回！
	isFirstSync := rule.LastSyncTime == nil
	hasPreviousError := rule.LastStatus == "error"
	if !task.force && !plan.HasChanges && !isFirstSync && !hasPreviousError {
		w.service.touchRuleSyncTime(rule.Id)
		w.releaseRuleGuard(rule.Id)
		return
	}

	// IP 发生变动或首次同步，投递到慢路径云商同步队列
	syncTask := &ddnsSyncTask{
		rule:  rule,
		plan:  plan,
		force: task.force,
	}

	select {
	case w.syncQueue <- syncTask:
	default:
		// 慢路径队列满保护
		w.releaseRuleGuard(rule.Id)
		logger.Warningf("[DDNS] Sync queue full, skipping DNS update for rule %s", rule.Name)
	}
}

func (w *ddnsRuntimeWorker) runSyncerWorker(stopCh <-chan struct{}) {
	defer w.workerWg.Done()
	defer func() {
		if recovered := recover(); recovered != nil {
			logger.Error("[DDNS] Syncer worker panicked: ", recovered)
		}
	}()

	for {
		select {
		case <-stopCh:
			return
		case task, ok := <-w.syncQueue:
			if !ok {
				return
			}
			w.processSyncTask(task)
		}
	}
}

func (w *ddnsRuntimeWorker) processSyncTask(task *ddnsSyncTask) {
	defer w.releaseRuleGuard(task.rule.Id)

	operation, err := BeginKworInProcessOperation("ddns-sync-worker")
	if err != nil {
		return
	}
	defer operation.Done()

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()

	if err := w.service.syncRuleDNS(ctx, task.rule, task.plan); err != nil {
		logger.Warningf("[DDNS] Sync failed for rule %s: %v", task.rule.Name, err)
	}
}
