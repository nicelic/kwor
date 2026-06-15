package cronjob

import (
	"time"

	"github.com/alireza0/s-ui/logger"
	"github.com/alireza0/s-ui/service"
)

type ReverseProxySyncJob struct {
	service.ReverseProxyService
}

func NewReverseProxySyncJob() *ReverseProxySyncJob {
	return &ReverseProxySyncJob{}
}

func (j *ReverseProxySyncJob) Run() {
	if err := j.ReverseProxyService.SyncIfNeeded(3 * time.Second); err != nil {
		logger.Warning("reverse-proxy sync job failed: ", err)
	}
}
