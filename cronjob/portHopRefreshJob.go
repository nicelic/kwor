package cronjob

import (
	"github.com/alireza0/s-ui/logger"
	"github.com/alireza0/s-ui/service"
)

// PortHopRefreshJob refreshes HY/HY2 redirect rules by port_hop_interval,
// independent from traffic statistics collection.
type PortHopRefreshJob struct {
	service.NftTrafficService
}

func NewPortHopRefreshJob() *PortHopRefreshJob {
	return &PortHopRefreshJob{}
}

func (s *PortHopRefreshJob) Run() {
	if err := s.NftTrafficService.RefreshPortHopRedirects(); err != nil {
		logger.Warning("port hop refresh job failed: ", err)
	}
}
