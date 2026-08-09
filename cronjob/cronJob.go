package cronjob

import (
	"sync"
	"sync/atomic"
	"time"

	"github.com/alireza0/s-ui/logger"
	"github.com/alireza0/s-ui/service"
	"github.com/robfig/cron/v3"
)

type CronJob struct {
	mu        sync.Mutex
	cron      *cron.Cron
	runGuards map[string]*atomic.Bool
}

type nonOverlappingJob struct {
	name    string
	job     cron.Job
	running *atomic.Bool
}

func wrapNonOverlappingJob(name string, job cron.Job) cron.Job {
	return wrapNonOverlappingJobWithGuard(name, job, &atomic.Bool{})
}

func wrapNonOverlappingJobWithGuard(name string, job cron.Job, running *atomic.Bool) cron.Job {
	return &nonOverlappingJob{name: name, job: job, running: running}
}

// wrapNonOverlappingJob keeps a guard for each job name for the lifetime of a
// CronJob. Reloading the scheduler after a panel timezone change creates new
// cron.Job values, but an old scheduler job can still be finishing. Sharing
// this guard prevents that old run and the new scheduler's immediate run from
// overlapping.
func (c *CronJob) wrapNonOverlappingJob(name string, job cron.Job) cron.Job {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.runGuards == nil {
		c.runGuards = make(map[string]*atomic.Bool)
	}
	running := c.runGuards[name]
	if running == nil {
		running = &atomic.Bool{}
		c.runGuards[name] = running
	}
	return wrapNonOverlappingJobWithGuard(name, job, running)
}

func (j *nonOverlappingJob) Run() {
	if j == nil || j.job == nil || j.running == nil {
		return
	}
	if !j.running.CompareAndSwap(false, true) {
		logger.Warning("cron job is still running; skip this scheduled run: ", j.name)
		return
	}
	operation, err := service.BeginKworInProcessOperation("cron-" + j.name)
	if err != nil {
		j.running.Store(false)
		logger.Warning("cron job skipped by lifecycle quiesce: ", j.name, ": ", err)
		return
	}
	defer func() {
		operation.Done()
		j.running.Store(false)
		if recovered := recover(); recovered != nil {
			logger.Error("cron job panicked: ", j.name, ": ", recovered)
		}
	}()
	j.job.Run()
}

func NewCronJob() *CronJob {
	return &CronJob{}
}

func (c *CronJob) Start(loc *time.Location, trafficAge int) error {
	scheduler := cron.New(cron.WithLocation(loc), cron.WithSeconds())

	nftCoreSync := c.wrapNonOverlappingJob("nft-core sync", NewNftCoreSyncJob())
	mihomoNftCoreSync := c.wrapNonOverlappingJob("mihomo nft-core sync", NewMihomoNftCoreSyncJob())
	firewallSync := c.wrapNonOverlappingJob("firewall sync", NewFirewallSyncJob())
	portForwardSync := c.wrapNonOverlappingJob("port-forward sync", NewPortForwardSyncJob())
	reverseProxySync := c.wrapNonOverlappingJob("reverse-proxy sync", NewReverseProxySyncJob())
	panelCertificateBalanceSync := c.wrapNonOverlappingJob("panel certificate-balance sync", NewPanelCertificateBalanceSyncJob())
	tlsPathSync := c.wrapNonOverlappingJob("TLS path sync", NewTLSPathSyncJob())
	acmeAutoRenew := c.wrapNonOverlappingJob("ACME auto-renew", NewAcmeAutoRenewJob())
	certificateCoreRestart := c.wrapNonOverlappingJob("certificate Core restart", NewCertificateCoreRestartJob())
	statsJob := c.wrapNonOverlappingJob("stats", NewStatsJob(trafficAge > 0))
	depleteJob := c.wrapNonOverlappingJob("client deplete", NewDepleteJob())
	checkCoreJob := c.wrapNonOverlappingJob("sing-box core update check", NewCheckCoreJob())
	autoUpdateCoreJob := c.wrapNonOverlappingJob("sing-box core auto update", NewAutoUpdateCoreJob())
	checkMihomoCoreJob := c.wrapNonOverlappingJob("mihomo core update check", NewCheckMihomoCoreJob())
	autoUpdateMihomoCoreJob := c.wrapNonOverlappingJob("mihomo core auto update", NewAutoUpdateMihomoCoreJob())
	subGroupAutoUpdateJob := c.wrapNonOverlappingJob("subscription group auto-update", NewSubGroupAutoUpdateJob())
	delStatsJob := c.wrapNonOverlappingJob("delete old stats", NewDelStatsJob())
	register := func(schedule string, job cron.Job, name string) {
		if _, err := scheduler.AddJob(schedule, job); err != nil {
			logger.Warning("failed to register ", name, " job: ", err)
		}
	}

	// Keep nftables lifecycle aligned with sing-box core running state.
	register("@every 5s", nftCoreSync, "nft-core sync")
	register("@every 5s", mihomoNftCoreSync, "mihomo nft-core sync")
	register("@every 5s", firewallSync, "firewall sync")
	register("@every 5s", portForwardSync, "port-forward sync")
	register("@every 5s", reverseProxySync, "reverse-proxy sync")
	register("@every 5m", panelCertificateBalanceSync, "panel certificate-balance sync")
	register("@every 10s", statsJob, "stats")
	register("@every 1m", depleteJob, "client deplete")
	register("@daily", delStatsJob, "delete old stats")
	// Auto-check core updates based on the configured interval.
	register("@every 1m", checkCoreJob, "sing-box core update check")
	register("0 0 4 * * *", autoUpdateCoreJob, "sing-box core auto update")
	register("@every 1m", checkMihomoCoreJob, "mihomo core update check")
	register("0 0 4 * * *", autoUpdateMihomoCoreJob, "mihomo core auto update")
	register("@every 1m", subGroupAutoUpdateJob, "subscription group auto-update")
	register("@every 10m", acmeAutoRenew, "ACME auto-renew")
	register("@every 1m", certificateCoreRestart, "certificate Core restart")
	register("@every 30s", tlsPathSync, "TLS path sync")

	c.mu.Lock()
	previous := c.cron
	c.cron = scheduler
	if previous != nil {
		previous.Stop()
	}
	scheduler.Start()
	c.mu.Unlock()

	go nftCoreSync.Run()
	go mihomoNftCoreSync.Run()
	go firewallSync.Run()
	go portForwardSync.Run()
	go reverseProxySync.Run()
	go panelCertificateBalanceSync.Run()
	go tlsPathSync.Run()
	go acmeAutoRenew.Run()

	return nil
}

func (c *CronJob) Stop() {
	if c == nil {
		return
	}
	c.mu.Lock()
	scheduler := c.cron
	c.cron = nil
	c.mu.Unlock()
	if scheduler != nil {
		scheduler.Stop()
	}
}
