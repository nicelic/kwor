package cronjob

import (
	"runtime"
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

	mu                sync.Mutex
	initialized       bool
	lastRunning       bool
	lastIntegrityScan time.Time
	lastRecoverAt     time.Time
}

func NewMihomoNftCoreSyncJob() *MihomoNftCoreSyncJob {
	return &MihomoNftCoreSyncJob{}
}

func (s *MihomoNftCoreSyncJob) Run() {
	if runtime.GOOS != "linux" {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	running := s.MihomoCoreManagerService.IsRunning()
	if !running && service.ShouldAutoRecoverManagedCoreRuntime("mihomo") {
		now := time.Now()
		if s.lastRecoverAt.IsZero() || now.Sub(s.lastRecoverAt) >= managedCoreAutoRecoverRetryInterval {
			s.lastRecoverAt = now
			if err := s.MihomoCoreManagerService.StartCore(); err != nil {
				logger.Warning("mihomo direct runtime auto-recover failed: ", err)
			}
			running = s.MihomoCoreManagerService.IsRunning()
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
		if s.lastIntegrityScan.IsZero() || now.Sub(s.lastIntegrityScan) >= nftIntegrityScanInterval {
			if err := s.MihomoNftTrafficService.EnsureRuleIntegrity(); err != nil {
				logger.Warning("mihomo nft rule integrity scan failed: ", err)
			}
			if err := s.MihomoClientRateLimitService.EnsureRuleIntegrity(); err != nil {
				logger.Warning("mihomo client rate limit nft integrity scan failed: ", err)
			}
			if err := s.MihomoClientPortBlockService.EnsureRuleIntegrity(); err != nil {
				logger.Warning("mihomo client block nft integrity scan failed: ", err)
			}
			s.lastIntegrityScan = now
		}
	}

	if running {
		s.lastRecoverAt = time.Time{}
	}
	s.lastRunning = running
	s.initialized = true
}
