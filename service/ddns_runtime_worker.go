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
	// ddnsSchedulerInterval 调度器心跳周期（1秒周期检查是否有规则到期）
	ddnsSchedulerInterval = 1 * time.Second
)

type ddnsDetectTask struct {
	rule  *model.DDNSRule
	force bool
}

type ddnsSyncTask struct {
	rule      *model.DDNSRule
	v4        string
	v6        string
	v4Changed bool
	v6Changed bool
	force     bool
}

type ddnsRuntimeWorker struct {
	mu         sync.Mutex
	running    bool
	stopping   bool
	stopCh     chan struct{}
	doneCh     chan struct{}
	completeCh chan struct{}
	wakeCh     chan struct{}

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
		w.schedulePass()
	case <-stopCh:
		return
	}

	ticker := time.NewTicker(ddnsSchedulerInterval)
	defer ticker.Stop()

	for {
		select {
		case <-stopCh:
			return
		case <-wakeCh:
			w.schedulePass()
		case <-ticker.C:
			w.schedulePass()
		}
	}
}

func (w *ddnsRuntimeWorker) schedulePass() {
	operation, err := BeginKworInProcessOperation("ddns-runtime-scheduler")
	if err != nil {
		return
	}
	defer operation.Done()

	db := database.GetDDNSDB()
	if db == nil {
		return
	}

	var rules []model.DDNSRule
	if err := db.Where("enabled = ?", true).Find(&rules).Error; err != nil {
		return
	}

	now := time.Now()
	for i := range rules {
		rule := &rules[i]
		if !w.shouldDetectRule(rule, now) {
			continue
		}

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
	}
}

func (w *ddnsRuntimeWorker) shouldDetectRule(rule *model.DDNSRule, now time.Time) bool {
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

	if rule.LastSyncTime == nil {
		return true
	}
	return now.Sub(*rule.LastSyncTime) >= interval
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

	currentV4, currentV6, detectErrors := w.service.detectRuleIPs(ctx, rule)

	if len(detectErrors) > 0 && currentV4 == "" && currentV6 == "" {
		errMsg := strings.Join(detectErrors, "; ")
		w.service.updateRuleStatus(rule.Id, "error", errMsg, currentV4, currentV6)
		w.releaseRuleGuard(rule.Id)
		return
	}

	needV4 := rule.IPType == "ipv4" || rule.IPType == "dual"
	needV6 := rule.IPType == "ipv6" || rule.IPType == "dual"

	v4Changed := needV4 && currentV4 != "" && currentV4 != rule.LastIPV4
	v6Changed := needV6 && currentV6 != "" && currentV6 != rule.LastIPV6

	// 【快慢分离关键短路】：IP 未发生变化且非强制同步，微秒级短路返回！
	if !task.force && !v4Changed && !v6Changed {
		w.service.touchRuleSyncTime(rule.Id)
		w.releaseRuleGuard(rule.Id)
		return
	}

	// IP 发生变动或强制同步，投递到慢路径云商同步队列
	syncTask := &ddnsSyncTask{
		rule:      rule,
		v4:        currentV4,
		v6:        currentV6,
		v4Changed: v4Changed,
		v6Changed: v6Changed,
		force:     task.force,
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

	if err := w.service.syncRuleDNS(ctx, task.rule, task.v4, task.v6, task.v4Changed, task.v6Changed, task.force); err != nil {
		logger.Warningf("[DDNS] Sync failed for rule %s: %v", task.rule.Name, err)
	}
}
