package cronjob

import (
	"sync"
	"time"

	"github.com/alireza0/s-ui/logger"
	"github.com/alireza0/s-ui/service"
)

type MihomoNftCoreSyncJob struct {
	service.MihomoCoreManagerService
	service.MihomoNftTrafficService
	service.MihomoClientRateLimitService
	service.MihomoClientPortBlockService

	mu                    sync.Mutex
	initialized           bool
	lastRunning           bool
	lastIntegrityScan     time.Time
	lastFullIntegrityScan time.Time
	fullIntegrityRetry    bool
	lastRecoverAt         time.Time
	recoverRetryAfter     time.Duration
	runningSince          time.Time
}

const (
	mihomoNftIntegrityScanInterval     = nftIntegrityScanInterval
	mihomoCoreRecoveryMaxRetryInterval = 5 * time.Minute
	mihomoCoreRecoveryStableDuration   = 30 * time.Second
)

func NewMihomoNftCoreSyncJob() *MihomoNftCoreSyncJob {
	return &MihomoNftCoreSyncJob{}
}

func (s *MihomoNftCoreSyncJob) Run() {
	s.run(false)
}

// RunNow forces one integrity pass after a panel-side runtime change without
// coupling Mihomo's independent lifecycle to the default core.
func (s *MihomoNftCoreSyncJob) RunNow() {
	s.run(true)
}

func (s *MihomoNftCoreSyncJob) run(forceIntegrity bool) {
	if !service.IsSystemPlatformLinux() {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	running := s.MihomoCoreManagerService.IsRunning()
	if !running {
		s.runningSince = time.Time{}
	}
	if !running && service.ShouldAutoRecoverManagedCoreRuntime("mihomo") {
		now := time.Now()
		retryAfter := s.recoverRetryAfter
		if retryAfter <= 0 {
			retryAfter = managedCoreAutoRecoverRetryInterval
		}
		if s.lastRecoverAt.IsZero() || now.Sub(s.lastRecoverAt) >= retryAfter {
			s.lastRecoverAt = now
			if err := s.MihomoCoreManagerService.StartCore(); err != nil {
				logger.Warning("mihomo direct runtime auto-recover failed: ", err)
			}
			running = s.MihomoCoreManagerService.IsRunning()
			if running {
				s.runningSince = now
			} else {
				s.recoverRetryAfter = nextMihomoCoreRecoveryRetryInterval(retryAfter)
			}
		}
	}
	needInit := false
	if running {
		needInit = !s.initialized || !s.lastRunning || !s.MihomoNftTrafficService.IsNftTableReady()
	}
	needCleanup := !running && (!s.initialized || s.lastRunning)

	if needInit {
		s.MihomoNftTrafficService.InitOnStartup()
		s.MihomoClientRateLimitService.InitOnStartup()
		s.MihomoClientPortBlockService.InitOnStartup()
		s.lastIntegrityScan = time.Now()
	} else if needCleanup {
		s.MihomoNftTrafficService.CleanupOnShutdown()
		s.MihomoClientRateLimitService.CleanupOnShutdown()
		s.MihomoClientPortBlockService.CleanupOnShutdown()
		s.lastIntegrityScan = time.Time{}
	} else if running {
		now := time.Now()
		if forceIntegrity || s.lastIntegrityScan.IsZero() || now.Sub(s.lastIntegrityScan) >= mihomoNftIntegrityScanInterval {
			full := forceIntegrity || s.fullIntegrityRetry || s.lastFullIntegrityScan.IsZero() || now.Sub(s.lastFullIntegrityScan) >= nftFullIntegrityScanInterval
			if !full && !s.MihomoNftTrafficService.IsNftTableReady() {
				full = true
			}
			if full {
				fullErr := error(nil)
				if err := s.MihomoNftTrafficService.EnsureRuleIntegrityWhenRunning(); err != nil {
					logger.Warning("mihomo nft rule integrity scan failed: ", err)
					fullErr = err
				}
				if err := s.MihomoClientRateLimitService.EnsureRuleIntegrityWhenRunning(); err != nil {
					logger.Warning("mihomo client rate limit nft integrity scan failed: ", err)
					if fullErr == nil {
						fullErr = err
					}
				}
				if err := s.MihomoClientPortBlockService.EnsureRuleIntegrityWhenRunning(); err != nil {
					logger.Warning("mihomo client block nft integrity scan failed: ", err)
					if fullErr == nil {
						fullErr = err
					}
				}
				if fullErr == nil {
					s.lastFullIntegrityScan = now
					s.fullIntegrityRetry = false
				} else {
					s.fullIntegrityRetry = true
				}
			}
			s.lastIntegrityScan = now
		}
	}

	if running {
		if s.runningSince.IsZero() {
			s.runningSince = time.Now()
		}
		if time.Since(s.runningSince) >= mihomoCoreRecoveryStableDuration {
			s.lastRecoverAt = time.Time{}
			s.recoverRetryAfter = 0
		}
	}
	s.lastRunning = running
	s.initialized = true
}

func nextMihomoCoreRecoveryRetryInterval(current time.Duration) time.Duration {
	if current <= 0 {
		return managedCoreAutoRecoverRetryInterval
	}
	next := current * 2
	if next > mihomoCoreRecoveryMaxRetryInterval {
		return mihomoCoreRecoveryMaxRetryInterval
	}
	return next
}
