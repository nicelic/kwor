package cronjob

import (
	"sync"

	"github.com/alireza0/s-ui/logger"
	"github.com/alireza0/s-ui/service"
)

var tlsPathSyncJobMu sync.Mutex

// TLSPathSyncJob watches path-based TLS certificate material updates and
// triggers managed subscription sync when content changes.
type TLSPathSyncJob struct{}

func NewTLSPathSyncJob() *TLSPathSyncJob {
	return &TLSPathSyncJob{}
}

func (j *TLSPathSyncJob) Run() {
	tlsPathSyncJobMu.Lock()
	defer tlsPathSyncJobMu.Unlock()

	changed, err := service.CheckAndSyncAutoManagedSubscriptionsOnTLSPathChange("")
	if err != nil {
		logger.Warning("tls path sync job failed: ", err)
		return
	}
	if changed {
		logger.Info("[TLSPathSync] detected certificate path material change and synced managed subscriptions")
	}
}
