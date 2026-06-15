package cronjob

import (
	"github.com/alireza0/s-ui/logger"
	"github.com/alireza0/s-ui/service"
)

type AcmeAutoRenewJob struct {
	service.AcmeService
}

func NewAcmeAutoRenewJob() *AcmeAutoRenewJob {
	return &AcmeAutoRenewJob{}
}

func (j *AcmeAutoRenewJob) Run() {
	renewedCount, err := j.AcmeService.RunAutoRenew()
	if err != nil {
		logger.Warning("acme auto-renew job finished with errors: ", err)
		return
	}
	if renewedCount > 0 {
		logger.Info("acme auto-renew job renewed certificates: ", renewedCount)
	}
}
