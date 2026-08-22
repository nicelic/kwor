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
	defaultResetChanged, err := s.ClientService.ResetTrafficBySchedule()
	if err != nil {
		logger.Warning("Reset traffic by schedule failed: ", err)
	}
	defaultDepletedInbounds, err := s.ClientService.DepleteClients()
	if err != nil {
		logger.Warning("Deplete clients failed: ", err)
	}
	if defaultResetChanged || len(defaultDepletedInbounds) > 0 {
		if s.CoreManagerService.IsRunning() {
			if err := s.ClientPortBlockService.ReconcileAfterTraffic(); err != nil {
				logger.Warning("reconcile client block rules after state change failed: ", err)
			}
		} else if err := s.ClientPortBlockService.Reconcile(false); err != nil {
			logger.Warning("reconcile client block state after state change failed: ", err)
		}
	}

	mihomoResetChanged, err := s.MihomoClientService.ResetTrafficBySchedule()
	if err != nil {
		logger.Warning("Reset mihomo traffic by schedule failed: ", err)
	}
	mihomoDepletedInbounds, err := s.MihomoClientService.DepleteClients()
	if err != nil {
		logger.Warning("Deplete mihomo clients failed: ", err)
	}
	if mihomoResetChanged || len(mihomoDepletedInbounds) > 0 {
		if s.MihomoCoreManagerService.IsRunning() {
			if err := s.MihomoClientPortBlockService.ReconcileAfterTraffic(); err != nil {
				logger.Warning("reconcile mihomo client block rules after state change failed: ", err)
			}
		} else if err := s.MihomoClientPortBlockService.Reconcile(false); err != nil {
			logger.Warning("reconcile mihomo client block state after state change failed: ", err)
		}
	}
}
