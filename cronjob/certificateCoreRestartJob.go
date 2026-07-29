package cronjob

import (
	"github.com/alireza0/s-ui/logger"
	"github.com/alireza0/s-ui/service"
)

type CertificateCoreRestartJob struct{}

func NewCertificateCoreRestartJob() *CertificateCoreRestartJob {
	return &CertificateCoreRestartJob{}
}

func (j *CertificateCoreRestartJob) Run() {
	if err := service.ProcessCertificateCoreRestartQueue(); err != nil {
		logger.Warning("certificate Core restart coordinator finished with errors: ", err)
	}
}
