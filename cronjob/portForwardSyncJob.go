package cronjob

import (
	"time"

	"github.com/alireza0/s-ui/logger"
	"github.com/alireza0/s-ui/service"
)

type PortForwardSyncJob struct {
	service.PortForwardService
}

func NewPortForwardSyncJob() *PortForwardSyncJob {
	return &PortForwardSyncJob{}
}

func (j *PortForwardSyncJob) Run() {
	if !service.IsSystemPlatformLinux() {
		return
	}
	if err := j.PortForwardService.SyncIfNeeded(3 * time.Second); err != nil {
		logger.Warning("port-forward sync job failed: ", err)
	}
}
