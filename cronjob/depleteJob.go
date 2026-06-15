package cronjob

import (
	"github.com/alireza0/s-ui/logger"
	"github.com/alireza0/s-ui/service"
)

type DepleteJob struct {
	service.ClientService
	service.CoreManagerService
	service.ClientPortBlockService
	service.MihomoClientService
	service.MihomoCoreManagerService
	service.MihomoClientPortBlockService
}

func NewDepleteJob() *DepleteJob {
	return new(DepleteJob)
}

func (s *DepleteJob) Run() {
	if err := s.ClientService.ResetTrafficBySchedule(); err != nil {
		logger.Warning("Reset traffic by schedule failed: ", err)
	}
	if err := s.ClientPortBlockService.Reconcile(s.CoreManagerService.IsRunning()); err != nil {
		logger.Warning("reconcile client block rules failed: ", err)
	}

	if err := s.MihomoClientService.ResetTrafficBySchedule(); err != nil {
		logger.Warning("Reset mihomo traffic by schedule failed: ", err)
	}
	if err := s.MihomoClientPortBlockService.Reconcile(s.MihomoCoreManagerService.IsRunning()); err != nil {
		logger.Warning("reconcile mihomo client block rules failed: ", err)
	}
}
