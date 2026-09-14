package cronjob

import (
	"runtime/debug"
	"sync"
	"time"

	"github.com/alireza0/s-ui/logger"
	"github.com/alireza0/s-ui/service"
)

const (
	runtimeSamplerTrafficInterval     = 10 * time.Second
	runtimeSamplerIntegrityInterval   = 15 * time.Second
	runtimeSamplerPortForwardInterval = 15 * time.Second
	runtimeSamplerDepleteInterval     = time.Minute
	runtimeSamplerFlushInterval       = time.Minute

	runtimeSamplerTrafficPhase     = 0 * time.Second
	runtimeSamplerIntegrityPhase   = 3 * time.Second
	runtimeSamplerPortForwardPhase = 7 * time.Second
	runtimeSamplerDepletePhase     = 12 * time.Second
	runtimeSamplerFlushPhase       = 26 * time.Second
)

// RuntimeSampler owns the panel's frequent runtime work.  It deliberately
// serializes the jobs because they all inspect the same host and share one
// SQLite connection.  sing-box and Mihomo keep separate job instances and
// runtime state; only their scheduling is centralized.
//
// ============================================================================
// 架构隔离铁律与职责边界（严禁擅自合并）：
// 1. 本调度中枢专职负责“微秒/毫秒级本地宿主机状态同步与单主库 (s-ui.db) 保护”任务：
//    - 10s 流量记账 (traffic)
//    - 15s sing-box/Mihomo 与 nftables 核心同步 (integrity)
//    - 15s 端口转发探测与同步 (port-forward)
//    - 1m 额度耗尽封禁检查 (deplete)
//    - 1m 流量账本安全落库 (flush)
//    通过“相锁错峰 (Phase-locked staggering)”与“单连接强串行化”彻底杜绝 CPU 突发共振与 SQLite 死锁。
//
// 2. 严禁合并 DDNS 任务 (service.StartDDNSRuntimeWorker)：
//    - DDNS 涉及外网 HTTP IP 探测与 34 家云商 OpenAPI 交互（含不可预测的网络延迟与最高 45s 超时）；
//    - DDNS 独享完全隔离的物理数据库 Promanager_data/db/ddns.db (database.GetDDNSDB())；
//    - DDNS 运行于专属内核物理线程 (runtime.LockOSThread) 并维护快慢分离并发 Worker 池。
//    若合并入本调度器，外部网络抖动将直接阻塞串行循环，导致面板流量记账掉帧与 nftables 同步瘫痪！
//
// 3. 严禁合并系统优化监视器 (service.StartSystemOptimizationWatcher)：
//    - 系统优化负责 10s 轮询监控 sysctl、journald、resolv.conf 等受管配置文件；
//    - 当配置被外部篡改时，需执行 sysctl -p 或 systemctl restart systemd-journald 等外部重型系统命令；
//    若合并入本调度器，外部系统命令调用的停顿将直接阻断核心网络层的精准错峰采样。
//
// 4. 反向代理监视器 (service.StartReverseProxyRuntimeWorker) 亦由独立 30s 协程托管，不在此处混用。
// ============================================================================
type RuntimeSampler struct {
	mu sync.Mutex

	running    bool
	stopping   bool
	stopCh     chan struct{}
	doneCh     chan struct{}
	completeCh chan struct{}
	wakeCh     chan struct{}
	stopErr    error

	nftCoreSync       *NftCoreSyncJob
	mihomoNftCoreSync *MihomoNftCoreSyncJob
	stats             *StatsJob
	portForward       *PortForwardSyncJob
	deplete           *DepleteJob

	// taskOverrides exists for focused scheduler tests. Production instances
	// leave it nil and always execute the concrete panel services above.
	taskOverrides *runtimeSamplerTaskOverrides
}

type runtimeSamplerTaskOverrides struct {
	integrity   func(force bool)
	traffic     func()
	portForward func()
	reverse     func()
	deplete     func()
	flush       func() error
}

func NewRuntimeSampler(trafficAge int) *RuntimeSampler {
	return &RuntimeSampler{
		nftCoreSync:       NewNftCoreSyncJob(),
		mihomoNftCoreSync: NewMihomoNftCoreSyncJob(),
		stats:             NewStatsJob(trafficAge > 0),
		portForward:       NewPortForwardSyncJob(),
		deplete:           NewDepleteJob(),
	}
}

func (s *RuntimeSampler) Start() {
	if s == nil {
		return
	}
	for {
		s.mu.Lock()
		if s.running {
			s.mu.Unlock()
			s.Wake()
			return
		}
		if s.stopping {
			completeCh := s.completeCh
			s.mu.Unlock()
			if completeCh != nil {
				<-completeCh
			}
			continue
		}

		s.running = true
		s.stopCh = make(chan struct{})
		s.doneCh = make(chan struct{})
		s.completeCh = make(chan struct{})
		s.wakeCh = make(chan struct{}, 1)
		s.stopErr = nil
		stopCh := s.stopCh
		doneCh := s.doneCh
		wakeCh := s.wakeCh
		s.mu.Unlock()

		go s.run(stopCh, wakeCh, doneCh)
		return
	}
}

func (s *RuntimeSampler) StopAndFlush() error {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	if !s.running && !s.stopping {
		s.mu.Unlock()
		if err := s.flushJournal(); err != nil {
			logger.Warning("flush traffic runtime journal failed: ", err)
			return err
		}
		return nil
	}
	if s.stopping {
		completeCh := s.completeCh
		s.mu.Unlock()
		if completeCh != nil {
			<-completeCh
		}
		s.mu.Lock()
		err := s.stopErr
		s.mu.Unlock()
		return err
	}

	doneCh := s.doneCh
	completeCh := s.completeCh
	if s.running {
		stopCh := s.stopCh
		s.running = false
		s.stopping = true
		close(stopCh)
	}
	s.mu.Unlock()

	if doneCh != nil {
		<-doneCh
	}
	flushErr := s.flushJournal()
	s.mu.Lock()
	if s.doneCh == doneCh {
		s.stopCh = nil
		s.doneCh = nil
		s.completeCh = nil
		s.wakeCh = nil
		s.stopErr = flushErr
		s.stopping = false
		if completeCh != nil {
			close(completeCh)
		}
	}
	s.mu.Unlock()
	if flushErr != nil {
		logger.Warning("flush traffic runtime journal on shutdown failed: ", flushErr)
		return flushErr
	}
	return nil
}

// Wake asks the worker to run the safety-critical reconciliation paths now.
// The channel is intentionally coalescing: repeated saves must not queue an
// unbounded number of host scans.
func (s *RuntimeSampler) Wake() {
	if s == nil {
		return
	}
	s.mu.Lock()
	wakeCh := s.wakeCh
	running := s.running
	s.mu.Unlock()
	if !running || wakeCh == nil {
		return
	}
	select {
	case wakeCh <- struct{}{}:
	default:
	}
}

func (s *RuntimeSampler) run(stopCh <-chan struct{}, wakeCh <-chan struct{}, doneCh chan<- struct{}) {
	defer func() {
		if recovered := recover(); recovered != nil {
			logger.Error("runtime sampler panicked: ", recovered)
		}
		s.mu.Lock()
		if s.doneCh == doneCh {
			s.running = false
			if !s.stopping {
				s.stopCh = nil
				s.doneCh = nil
				completeCh := s.completeCh
				s.completeCh = nil
				s.wakeCh = nil
				if completeCh != nil {
					close(completeCh)
				}
			}
		}
		s.mu.Unlock()
		close(doneCh)
	}()

	now := time.Now()
	// Phase-locked staggering: anchor each task to dedicated non-overlapping
	// second slots across the minute. This completely avoids the periodic
	// resonance (30s and 60s CPU bursts) caused by simple interval addition.
	nextTraffic := nextPhaseSlot(now, runtimeSamplerTrafficInterval, runtimeSamplerTrafficPhase)
	nextIntegrity := nextPhaseSlot(now, runtimeSamplerIntegrityInterval, runtimeSamplerIntegrityPhase)
	nextPortForward := nextPhaseSlot(now, runtimeSamplerPortForwardInterval, runtimeSamplerPortForwardPhase)
	nextDeplete := nextPhaseSlot(now, runtimeSamplerDepleteInterval, runtimeSamplerDepletePhase)
	nextFlush := nextPhaseSlot(now, runtimeSamplerFlushInterval, runtimeSamplerFlushPhase)

	timer := time.NewTimer(time.Hour)
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
	defer timer.Stop()

	for {
		now = time.Now()
		next := earliestRuntimeSamplerDeadline(nextTraffic, nextIntegrity, nextPortForward, nextDeplete, nextFlush)
		delay := time.Until(next)
		if delay < 0 {
			delay = 0
		}
		timer.Reset(delay)

		select {
		case <-stopCh:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			return
		case <-wakeCh:
			if !timer.Stop() {
				select {
				case <-timer.C:
				default:
				}
			}
			s.runWakePass()
			now = time.Now()
			nextTraffic = nextPhaseSlot(now, runtimeSamplerTrafficInterval, runtimeSamplerTrafficPhase)
			nextIntegrity = nextPhaseSlot(now, runtimeSamplerIntegrityInterval, runtimeSamplerIntegrityPhase)
			nextPortForward = nextPhaseSlot(now, runtimeSamplerPortForwardInterval, runtimeSamplerPortForwardPhase)
			nextDeplete = nextPhaseSlot(now, runtimeSamplerDepleteInterval, runtimeSamplerDepletePhase)
			nextFlush = nextPhaseSlot(now, runtimeSamplerFlushInterval, runtimeSamplerFlushPhase)
			continue
		case <-timer.C:
		}

		// Keep one bounded nft read snapshot for the complete scheduled round.
		// Mutating commands invalidate it, so later tasks still observe fresh data.
		service.WithNftReadSnapshot(func() {
			now = time.Now()
			if !now.Before(nextIntegrity) {
				s.runTask("integrity", func() { s.runIntegrity(false) })
				nextIntegrity = nextPhaseSlot(time.Now(), runtimeSamplerIntegrityInterval, runtimeSamplerIntegrityPhase)
			}
			if !now.Before(nextTraffic) {
				s.runTask("traffic", s.runTraffic)
				nextTraffic = nextPhaseSlot(time.Now(), runtimeSamplerTrafficInterval, runtimeSamplerTrafficPhase)
			}
			if !now.Before(nextPortForward) {
				s.runTask("port-forward", s.runPortForward)
				nextPortForward = nextPhaseSlot(time.Now(), runtimeSamplerPortForwardInterval, runtimeSamplerPortForwardPhase)
			}
			if !now.Before(nextDeplete) {
				s.runTask("deplete", s.runDeplete)
				nextDeplete = nextPhaseSlot(time.Now(), runtimeSamplerDepleteInterval, runtimeSamplerDepletePhase)
			}
			if !now.Before(nextFlush) {
				s.runTask("flush", func() {
					if err := s.flushJournal(); err != nil {
						logger.Warning("flush traffic runtime journal failed: ", err)
					}
				})
				nextFlush = nextPhaseSlot(time.Now(), runtimeSamplerFlushInterval, runtimeSamplerFlushPhase)
			}
		})
	}
}

func (s *RuntimeSampler) runWakePass() {
	service.WithNftReadSnapshot(func() {
		s.runTask("integrity", func() { s.runIntegrity(true) })
		s.runTask("traffic", s.runTraffic)
		s.runTask("port-forward", s.runPortForward)
		s.runTask("deplete", s.runDeplete)
		s.runTask("flush", func() {
			if err := s.flushJournal(); err != nil {
				logger.Warning("flush traffic runtime journal after runtime wake failed: ", err)
			}
		})
	})
}

func (s *RuntimeSampler) runTask(name string, task func()) {
	if task == nil {
		return
	}
	started := time.Now()
	defer func() {
		service.RecordRuntimePerformance(service.RuntimePerformanceSample{
			Task:       name,
			StartedAt:  started.Unix(),
			DurationMs: time.Since(started).Milliseconds(),
		})
		if recovered := recover(); recovered != nil {
			logger.Error("runtime sampler task panicked: ", name, ": ", recovered)
		}
	}()
	task()
}

func (s *RuntimeSampler) runIntegrity(force bool) {
	if s.taskOverrides != nil && s.taskOverrides.integrity != nil {
		s.taskOverrides.integrity(force)
		return
	}
	if force {
		s.nftCoreSync.RunNow()
		s.mihomoNftCoreSync.RunNow()
		return
	}
	s.nftCoreSync.Run()
	s.mihomoNftCoreSync.Run()
}

func (s *RuntimeSampler) runTraffic() {
	if s.taskOverrides != nil && s.taskOverrides.traffic != nil {
		s.taskOverrides.traffic()
		return
	}
	s.stats.Run()
}

func (s *RuntimeSampler) runPortForward() {
	if s.taskOverrides != nil && s.taskOverrides.portForward != nil {
		s.taskOverrides.portForward()
		return
	}
	if s.portForward != nil {
		s.portForward.Run()
	}
}

func (s *RuntimeSampler) runReverseProxy() {
	if s.taskOverrides != nil && s.taskOverrides.reverse != nil {
		s.taskOverrides.reverse()
	}
}

func (s *RuntimeSampler) runDeplete() {
	if s.taskOverrides != nil && s.taskOverrides.deplete != nil {
		s.taskOverrides.deplete()
		return
	}
	if s.deplete != nil {
		s.deplete.Run()
	}
}

func (s *RuntimeSampler) flushJournal() error {
	if s.taskOverrides != nil && s.taskOverrides.flush != nil {
		return s.taskOverrides.flush()
	}
	hadPending := service.HasPendingTrafficRuntimeJournal()
	err := service.FlushTrafficRuntimeJournal()
	if hadPending {
		debug.FreeOSMemory()
	}
	return err
}

func earliestRuntimeSamplerDeadline(values ...time.Time) time.Time {
	next := values[0]
	for _, value := range values[1:] {
		if value.Before(next) {
			next = value
		}
	}
	return next
}

// nextPhaseSlot calculates the next occurrence after now matching the given
// period and phase offset. It guarantees that tasks with different phase offsets
// never fire at the same second, completely preventing cyclical resonance.
func nextPhaseSlot(now time.Time, interval, phase time.Duration) time.Time {
	unixSec := now.Unix()
	periodSec := int64(interval / time.Second)
	phaseSec := int64(phase / time.Second)

	rem := (unixSec - phaseSec) % periodSec
	if rem < 0 {
		rem += periodSec
	}
	deltaSec := periodSec - rem
	next := time.Unix(unixSec+deltaSec, 0)
	if next.Sub(now) < 800*time.Millisecond {
		next = next.Add(interval)
	}
	return next
}

