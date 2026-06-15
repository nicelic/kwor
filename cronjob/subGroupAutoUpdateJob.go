package cronjob

import (
	"github.com/alireza0/s-ui/logger"
	"github.com/alireza0/s-ui/service"
)

type SubGroupAutoUpdateJob struct {
	service.SubGroupService
}

func NewSubGroupAutoUpdateJob() *SubGroupAutoUpdateJob {
	return &SubGroupAutoUpdateJob{}
}

func (j *SubGroupAutoUpdateJob) Run() {
	if err := j.SubGroupService.RunAutoUpdate(); err != nil {
		logger.Warning("subgroup auto-update job failed: ", err)
	}
}
