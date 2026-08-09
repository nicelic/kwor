package cronjob

import (
	"github.com/alireza0/s-ui/logger"
	"github.com/alireza0/s-ui/service"
)

type AutoUpdateMihomoCoreJob struct {
	service.MihomoCoreManagerService
}

func NewAutoUpdateMihomoCoreJob() *AutoUpdateMihomoCoreJob {
	return &AutoUpdateMihomoCoreJob{}
}

func (s *AutoUpdateMihomoCoreJob) Run() {
	if err := s.MihomoCoreManagerService.RunScheduledAutoUpdate(); err != nil {
		logger.Warning("scheduled Mihomo core auto update failed: ", err)
	}
}
