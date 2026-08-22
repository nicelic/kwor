package cronjob

import (
	"time"

	"github.com/alireza0/s-ui/logger"
	"github.com/alireza0/s-ui/service"
)

type FirewallSyncJob struct {
	service.FirewallService
}

func NewFirewallSyncJob() *FirewallSyncJob {
	return &FirewallSyncJob{}
}

func (j *FirewallSyncJob) Run() {
	if !service.IsSystemPlatformLinux() {
		return
	}
	if err := j.FirewallService.SyncIfNeeded(4*time.Minute + 55*time.Second); err != nil {
		logger.Warning("firewall sync job failed: ", err)
	}
}
