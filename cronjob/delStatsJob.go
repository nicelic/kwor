package cronjob

import (
	"github.com/alireza0/s-ui/logger"
	"github.com/alireza0/s-ui/service"
)

type DelStatsJob struct {
	service.StatsService
}

func NewDelStatsJob() *DelStatsJob {
	return &DelStatsJob{}
}

func (s *DelStatsJob) Run() {
	trafficAge, err := (&service.SettingService{}).GetTrafficAge()
	if err != nil {
		logger.Warning("failed to load trafficAge for cleanup job: ", err)
		return
	}
	if trafficAge <= 0 {
		return
	}
	err = s.StatsService.DelOldStats(trafficAge)
	if err != nil {
		logger.Warning("Deleting old statistics failed: ", err)
		return
	}
	logger.Debug("Stats older than ", trafficAge, " days were deleted")
}
