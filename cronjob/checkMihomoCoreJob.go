package cronjob

import (
	"github.com/alireza0/s-ui/logger"
	"github.com/alireza0/s-ui/service"
)

type CheckMihomoCoreJob struct {
	service.MihomoCoreManagerService
}

func NewCheckMihomoCoreJob() *CheckMihomoCoreJob {
	return &CheckMihomoCoreJob{}
}

func (s *CheckMihomoCoreJob) Run() {
	if err := s.MihomoCoreManagerService.CheckAndMarkCoreUpdates(false); err != nil {
		logger.Warning("auto check mihomo core updates failed: ", err)
	}
}
