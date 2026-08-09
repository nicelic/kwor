package cronjob

import (
	"github.com/alireza0/s-ui/logger"
	"github.com/alireza0/s-ui/service"
)

type AutoUpdateCoreJob struct {
	service.CoreManagerService
}

func NewAutoUpdateCoreJob() *AutoUpdateCoreJob {
	return &AutoUpdateCoreJob{}
}

func (s *AutoUpdateCoreJob) Run() {
	if err := s.CoreManagerService.RunScheduledAutoUpdate(); err != nil {
		logger.Warning("scheduled sing-box core auto update failed: ", err)
	}
}
