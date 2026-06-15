package cronjob

import (
	"runtime"
	"sync"
	"time"

	"github.com/alireza0/s-ui/logger"
	"github.com/alireza0/s-ui/service"
)

// NftCoreSyncJob keeps nftables lifecycle aligned with sing-box core running state.
// - core running  => ensure nftables rules exist
// - core stopped  => remove nftables rules
type NftCoreSyncJob struct {
	service.CoreManagerService
	service.NftTrafficService
	service.ClientRateLimitService
	service.ClientPortBlockService

	mu                sync.Mutex
	initialized       bool
	lastRunning       bool
	lastIntegrityScan time.Time
}

const nftIntegrityScanInterval = 15 * time.Second

func NewNftCoreSyncJob() *NftCoreSyncJob {
	return &NftCoreSyncJob{}
}

func (s *NftCoreSyncJob) Run() {
	if runtime.GOOS != "linux" {
		return
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	running := s.CoreManagerService.IsRunning()
	needInit := false
	if running {
		// Re-apply rules when core transitions to running, or when nft table was
		// externally flushed while core keeps running.
		needInit = !s.initialized || !s.lastRunning || !s.NftTrafficService.IsNftTableReady()
	}
	needCleanup := !running && (!s.initialized || s.lastRunning)

	if needInit {
		s.NftTrafficService.InitOnStartup()
		s.ClientRateLimitService.InitOnStartup()
		s.ClientPortBlockService.InitOnStartup()
		s.lastIntegrityScan = time.Now()
	} else if needCleanup {
		s.NftTrafficService.CleanupOnShutdown()
		s.ClientRateLimitService.CleanupOnShutdown()
		s.ClientPortBlockService.CleanupOnShutdown()
		s.lastIntegrityScan = time.Time{}
	} else if running {
		now := time.Now()
		if s.lastIntegrityScan.IsZero() || now.Sub(s.lastIntegrityScan) >= nftIntegrityScanInterval {
			if err := s.NftTrafficService.EnsureRuleIntegrity(); err != nil {
				logger.Warning("nft rule integrity scan failed: ", err)
			}
			if err := s.ClientRateLimitService.EnsureRuleIntegrity(); err != nil {
				logger.Warning("client rate limit nft integrity scan failed: ", err)
			}
			if err := s.ClientPortBlockService.EnsureRuleIntegrity(); err != nil {
				logger.Warning("client block nft integrity scan failed: ", err)
			}
			s.lastIntegrityScan = now
		}
	}

	s.lastRunning = running
	s.initialized = true
}
