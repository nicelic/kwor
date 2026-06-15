package cronjob

import (
	"github.com/alireza0/s-ui/logger"
	"github.com/alireza0/s-ui/service"
)

type CheckCoreJob struct {
	service.CoreManagerService
}

func NewCheckCoreJob() *CheckCoreJob {
	return &CheckCoreJob{}
}

func (s *CheckCoreJob) Run() {
	if err := s.CoreManagerService.CheckAndMarkCoreUpdates(false); err != nil {
		logger.Warning("auto check sing-box core updates failed: ", err)
	}
}
