package cronjob

import (
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
)

// RuntimeSampler owns the panel's frequent runtime work.  It deliberately
// serializes the jobs because they all inspect the same host and share one
// SQLite connection.  sing-box and Mihomo keep separate job instances and
// runtime state; only their scheduling is centralized.
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
	// Spread the first pass over several seconds.  The prior cron setup started
	// every high-frequency task together, which created avoidable CPU spikes.
	// Configuration and Core lifecycle changes still call Wake explicitly; a
	// clean scheduler start does not turn that staggered pass into a burst.
	nextTraffic := now.Add(time.Second)
	nextIntegrity := now.Add(2 * time.Second)
	nextPortForward := now.Add(4 * time.Second)
	nextDeplete := now.Add(8 * time.Second)
	nextFlush := now.Add(runtimeSamplerFlushInterval)

	for {
		now = time.Now()
		next := earliestRuntimeSamplerDeadline(nextTraffic, nextIntegrity, nextPortForward, nextDeplete, nextFlush)
		delay := time.Until(next)
		if delay < 0 {
			delay = 0
		}
		timer := time.NewTimer(delay)

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
			nextTraffic = now.Add(runtimeSamplerTrafficInterval)
			nextIntegrity = now.Add(runtimeSamplerIntegrityInterval)
			nextPortForward = now.Add(runtimeSamplerPortForwardInterval)
			nextDeplete = now.Add(runtimeSamplerDepleteInterval)
			nextFlush = now.Add(runtimeSamplerFlushInterval)
			continue
		case <-timer.C:
		}

		// Keep one bounded nft read snapshot for the complete scheduled round.
		// Mutating commands invalidate it, so later tasks still observe fresh data.
		service.WithNftReadSnapshot(func() {
			now = time.Now()
			if !now.Before(nextIntegrity) {
				s.runTask("integrity", func() { s.runIntegrity(false) })
				nextIntegrity = time.Now().Add(runtimeSamplerIntegrityInterval)
			}
			if !now.Before(nextTraffic) {
				s.runTask("traffic", s.runTraffic)
				nextTraffic = time.Now().Add(runtimeSamplerTrafficInterval)
			}
			if !now.Before(nextPortForward) {
				s.runTask("port-forward", s.runPortForward)
				nextPortForward = time.Now().Add(runtimeSamplerPortForwardInterval)
			}
			if !now.Before(nextDeplete) {
				s.runTask("deplete", s.runDeplete)
				nextDeplete = time.Now().Add(runtimeSamplerDepleteInterval)
			}
			if !now.Before(nextFlush) {
				s.runTask("flush", func() {
					if err := s.flushJournal(); err != nil {
						logger.Warning("flush traffic runtime journal failed: ", err)
					}
				})
				nextFlush = time.Now().Add(runtimeSamplerFlushInterval)
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
	return service.FlushTrafficRuntimeJournal()
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
