package cronjob

import (
	"sync"

	"github.com/alireza0/s-ui/logger"
	"github.com/alireza0/s-ui/service"
)

type StatsJob struct {
	service.StatsService
	enableTraffic bool
	mu            sync.Mutex
	running       bool
}

func NewStatsJob(saveTraffic bool) *StatsJob {
	return &StatsJob{
		enableTraffic: saveTraffic,
	}
}

func (s *StatsJob) Run() {
	if !s.beginRun() {
		logger.Warning("stats job is still running; skip this scheduled run")
		return
	}
	defer s.finishRun()

	// Keep port-hop refresh alive even when traffic accounting is disabled.
	if refreshErr := s.StatsService.NftTrafficService.RefreshPortHopRedirects(); refreshErr != nil {
		logger.Warning("port hop refresh failed: ", refreshErr)
	}

	enableTraffic := s.enableTraffic
	if trafficAge, trafficErr := (&service.SettingService{}).GetTrafficAge(); trafficErr == nil {
		enableTraffic = trafficAge > 0
	} else {
		logger.Warning("failed to load trafficAge for stats job: ", trafficErr)
	}

	err := s.StatsService.SaveStats(enableTraffic)
	if err != nil {
		logger.Warning("Get stats failed: ", err)
		return
	}

	// Collect nftables-based traffic AFTER core stats transaction is fully committed.
	// This avoids SQLite lock conflicts since CollectAndSaveTraffic uses its own transaction.
	if enableTraffic {
		trafficChanged, nftErr := s.StatsService.NftTrafficService.CollectAndSaveTrafficWithHistory(enableTraffic)
		if nftErr != nil {
			logger.Warning("nftables traffic collection failed: ", nftErr)
		}
		if trafficChanged {
			if blockErr := (&service.ClientPortBlockService{}).ReconcileAfterTraffic(); blockErr != nil {
				logger.Warning("client block nft reconcile after traffic failed: ", blockErr)
			}
		}
	}
	if capErr := (&service.TrafficOverviewService{}).ReconcileTrafficCap(); capErr != nil {
		logger.Warning("traffic cap reconcile failed: ", capErr)
	}

	// Mihomo traffic is collected independently from sing-box tracker stats.
	// Collecting still updates client up/down and online view even when traffic history is disabled.
	mihomoNftSvc := &service.MihomoNftTrafficService{}
	if refreshErr := mihomoNftSvc.RefreshPortHopRedirects(); refreshErr != nil {
		logger.Warning("mihomo port hop refresh failed: ", refreshErr)
	}
	mihomoTrafficChanged, nftErr := mihomoNftSvc.CollectAndSaveTrafficWithHistory(enableTraffic)
	if nftErr != nil {
		logger.Warning("mihomo nftables traffic collection failed: ", nftErr)
	}
	if mihomoTrafficChanged {
		if blockErr := (&service.MihomoClientPortBlockService{}).ReconcileAfterTraffic(); blockErr != nil {
			logger.Warning("mihomo client block nft reconcile failed: ", blockErr)
		}
	}
}

func (s *StatsJob) beginRun() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return false
	}
	s.running = true
	return true
}

func (s *StatsJob) finishRun() {
	s.mu.Lock()
	s.running = false
	s.mu.Unlock()
}
