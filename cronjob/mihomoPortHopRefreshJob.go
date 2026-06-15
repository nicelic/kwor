package cronjob

import (
	"github.com/alireza0/s-ui/logger"
	"github.com/alireza0/s-ui/service"
)

type MihomoPortHopRefreshJob struct {
	service.MihomoNftTrafficService
}

func NewMihomoPortHopRefreshJob() *MihomoPortHopRefreshJob {
	return &MihomoPortHopRefreshJob{}
}

func (s *MihomoPortHopRefreshJob) Run() {
	if err := s.MihomoNftTrafficService.RefreshPortHopRedirects(); err != nil {
		logger.Warning("mihomo port hop refresh job failed: ", err)
	}
}
