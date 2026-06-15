package cronjob

import (
	"github.com/alireza0/s-ui/logger"
	"github.com/alireza0/s-ui/service"
)

type PanelCertificateBalanceSyncJob struct {
	service.PanelCertificateBalanceService
}

func NewPanelCertificateBalanceSyncJob() *PanelCertificateBalanceSyncJob {
	return &PanelCertificateBalanceSyncJob{}
}

func (j *PanelCertificateBalanceSyncJob) Run() {
	if err := j.PanelCertificateBalanceService.Maintain(false); err != nil {
		logger.Warning("panel-certificate-balance sync job failed: ", err)
	}
}
