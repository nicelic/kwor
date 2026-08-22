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
	lifecycleMu          sync.Mutex
	mu                   sync.Mutex
	cron                 *cron.Cron
	runGuards            map[string]*atomic.Bool
	manualRuns           sync.WaitGroup
	runtimeSampler       *RuntimeSampler
	runtimeSamplerPaused bool
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

func stopCronSchedulerAndWait(scheduler *cron.Cron) {
	if scheduler == nil {
		return
	}
	<-scheduler.Stop().Done()
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
	if c == nil {
		return nil
	}
	c.lifecycleMu.Lock()
	defer c.lifecycleMu.Unlock()

	scheduler := cron.New(cron.WithLocation(loc), cron.WithSeconds())

	firewallSync := c.wrapNonOverlappingJob("firewall sync", NewFirewallSyncJob())
	panelCertificateBalanceSync := c.wrapNonOverlappingJob("panel certificate-balance sync", NewPanelCertificateBalanceSyncJob())
	tlsPathSync := c.wrapNonOverlappingJob("TLS path sync", NewTLSPathSyncJob())
	acmeAutoRenew := c.wrapNonOverlappingJob("ACME auto-renew", NewAcmeAutoRenewJob())
	certificateCoreRestart := c.wrapNonOverlappingJob("certificate Core restart", NewCertificateCoreRestartJob())
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

	register("@every 5m", firewallSync, "firewall sync")
	register("@every 5m", panelCertificateBalanceSync, "panel certificate-balance sync")
	register("@daily", delStatsJob, "delete old stats")
	// Auto-check core updates based on the configured interval.
	register("@every 1m", checkCoreJob, "sing-box core update check")
	register("0 0 4 * * *", autoUpdateCoreJob, "sing-box core auto update")
	register("@every 1m", checkMihomoCoreJob, "mihomo core update check")
	register("0 0 4 * * *", autoUpdateMihomoCoreJob, "mihomo core auto update")
	register("@every 1m", subGroupAutoUpdateJob, "subscription group auto-update")
	register("@every 10m", acmeAutoRenew, "ACME auto-renew")
	register("@every 1m", certificateCoreRestart, "certificate Core restart")
	register("@every 1m", tlsPathSync, "TLS path sync")

	c.mu.Lock()
	previous := c.cron
	if c.runtimeSampler == nil {
		c.runtimeSampler = NewRuntimeSampler(trafficAge)
	}
	runtimeSampler := c.runtimeSampler
	c.mu.Unlock()
	stopCronSchedulerAndWait(previous)
	c.manualRuns.Wait()

	c.mu.Lock()
	c.cron = scheduler
	c.mu.Unlock()
	scheduler.Start()
	service.RegisterRuntimeSamplerWake(c.WakeRuntimeSampler)
	service.RegisterRuntimeSamplerDatabaseBarrier(c.PauseRuntimeSamplerForDatabaseRestore, c.ResumeRuntimeSamplerAfterDatabaseRestoreFailure)
	runtimeSampler.Start()

	c.runImmediateJob(firewallSync)
	c.runImmediateJob(panelCertificateBalanceSync)
	c.runImmediateJob(tlsPathSync)
	c.runImmediateJob(acmeAutoRenew)

	return nil
}

func (c *CronJob) Stop() {
	if c == nil {
		return
	}
	c.lifecycleMu.Lock()
	defer c.lifecycleMu.Unlock()

	c.mu.Lock()
	scheduler := c.cron
	runtimeSampler := c.runtimeSampler
	c.cron = nil
	c.mu.Unlock()
	stopCronSchedulerAndWait(scheduler)
	c.manualRuns.Wait()
	if runtimeSampler != nil {
		if err := runtimeSampler.StopAndFlush(); err != nil {
			logger.Warning("runtime sampler shutdown flush failed: ", err)
		}
	}
	c.mu.Lock()
	c.runtimeSamplerPaused = false
	c.mu.Unlock()
	service.RegisterRuntimeSamplerDatabaseBarrier(nil, nil)
	service.RegisterRuntimeSamplerWake(nil)
}

func (c *CronJob) runImmediateJob(job cron.Job) {
	if c == nil || job == nil {
		return
	}
	c.manualRuns.Add(1)
	go func() {
		defer c.manualRuns.Done()
		job.Run()
	}()
}

func (c *CronJob) WakeRuntimeSampler() {
	if c == nil {
		return
	}
	c.mu.Lock()
	runtimeSampler := c.runtimeSampler
	c.mu.Unlock()
	if runtimeSampler != nil {
		runtimeSampler.Wake()
	}
}

// PauseRuntimeSamplerForDatabaseRestore closes the sampler worker before a
// database file is replaced. The paused state is remembered so a failed
// import can resume the exact scheduler that was active before the attempt.
func (c *CronJob) PauseRuntimeSamplerForDatabaseRestore() error {
	if c == nil {
		return nil
	}
	c.lifecycleMu.Lock()
	defer c.lifecycleMu.Unlock()

	c.mu.Lock()
	if c.runtimeSamplerPaused {
		c.mu.Unlock()
		return nil
	}
	runtimeSampler := c.runtimeSampler
	wasActive := c.cron != nil && runtimeSampler != nil
	c.runtimeSamplerPaused = wasActive
	c.mu.Unlock()
	if wasActive {
		if err := runtimeSampler.StopAndFlush(); err != nil {
			// StopAndFlush reports a flush failure only after the worker has
			// already exited. Keep the paused marker so the restore-abort hook
			// restarts that worker instead of silently leaving runtime sampling
			// stopped after a failed database replacement.
			return err
		}
	}
	return nil
}

func (c *CronJob) ResumeRuntimeSamplerAfterDatabaseRestoreFailure() {
	if c == nil {
		return
	}
	c.lifecycleMu.Lock()
	defer c.lifecycleMu.Unlock()

	c.mu.Lock()
	if !c.runtimeSamplerPaused {
		c.mu.Unlock()
		return
	}
	runtimeSampler := c.runtimeSampler
	shouldResume := c.cron != nil && runtimeSampler != nil
	c.runtimeSamplerPaused = false
	c.mu.Unlock()
	if shouldResume {
		runtimeSampler.Start()
	}
}
