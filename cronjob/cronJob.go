package cronjob

import (
	"time"

	"github.com/alireza0/s-ui/logger"
	"github.com/robfig/cron/v3"
)

type CronJob struct {
	cron *cron.Cron
}

func NewCronJob() *CronJob {
	return &CronJob{}
}

func (c *CronJob) Start(loc *time.Location, trafficAge int) error {
	c.cron = cron.New(cron.WithLocation(loc), cron.WithSeconds())
	c.cron.Start()

	nftCoreSync := NewNftCoreSyncJob()
	mihomoNftCoreSync := NewMihomoNftCoreSyncJob()
	firewallSync := NewFirewallSyncJob()
	portForwardSync := NewPortForwardSyncJob()
	reverseProxySync := NewReverseProxySyncJob()
	panelCertificateBalanceSync := NewPanelCertificateBalanceSyncJob()
	tlsPathSync := NewTLSPathSyncJob()
	acmeAutoRenew := NewAcmeAutoRenewJob()
	go nftCoreSync.Run()
	go mihomoNftCoreSync.Run()
	go firewallSync.Run()
	go portForwardSync.Run()
	go reverseProxySync.Run()
	go panelCertificateBalanceSync.Run()
	go tlsPathSync.Run()
	go acmeAutoRenew.Run()

	go func() {
		// Keep nftables lifecycle aligned with sing-box core running state.
		if _, err := c.cron.AddJob("@every 5s", nftCoreSync); err != nil {
			logger.Warning("failed to register nft-core sync job: ", err)
		}
		if _, err := c.cron.AddJob("@every 5s", mihomoNftCoreSync); err != nil {
			logger.Warning("failed to register mihomo nft-core sync job: ", err)
		}
		if _, err := c.cron.AddJob("@every 5s", firewallSync); err != nil {
			logger.Warning("failed to register firewall sync job: ", err)
		}
		if _, err := c.cron.AddJob("@every 5s", portForwardSync); err != nil {
			logger.Warning("failed to register port-forward sync job: ", err)
		}
		if _, err := c.cron.AddJob("@every 5s", reverseProxySync); err != nil {
			logger.Warning("failed to register reverse-proxy sync job: ", err)
		}
		if _, err := c.cron.AddJob("@every 5m", panelCertificateBalanceSync); err != nil {
			logger.Warning("failed to register panel certificate-balance sync job: ", err)
		}
		// Start stats job
		if _, err := c.cron.AddJob("@every 10s", NewStatsJob(trafficAge > 0)); err != nil {
			logger.Warning("failed to register stats job: ", err)
		}
		// When traffic accounting is disabled, keep a standalone refresh job.
		// Otherwise refresh is already handled by StatsJob to avoid duplicate DB writers.
		if trafficAge <= 0 {
			if _, err := c.cron.AddJob("@every 10s", NewPortHopRefreshJob()); err != nil {
				logger.Warning("failed to register port-hop refresh job: ", err)
			}
		}
		// Start expiry job
		if _, err := c.cron.AddJob("@every 1m", NewDepleteJob()); err != nil {
			logger.Warning("failed to register deplete job: ", err)
		}
		// Start deleting old stats
		if trafficAge > 0 {
			if _, err := c.cron.AddJob("@daily", NewDelStatsJob(trafficAge)); err != nil {
				logger.Warning("failed to register delete-stats job: ", err)
			}
		}
		// Auto-check sing-box core updates based on configured interval.
		if _, err := c.cron.AddJob("@every 1m", NewCheckCoreJob()); err != nil {
			logger.Warning("failed to register core-check job: ", err)
		}
		if _, err := c.cron.AddJob("@every 1m", NewCheckMihomoCoreJob()); err != nil {
			logger.Warning("failed to register mihomo core-check job: ", err)
		}
		if _, err := c.cron.AddJob("@every 1m", NewSubGroupAutoUpdateJob()); err != nil {
			logger.Warning("failed to register subgroup auto-update job: ", err)
		}
		if _, err := c.cron.AddJob("@every 6h", acmeAutoRenew); err != nil {
			logger.Warning("failed to register acme auto-renew job: ", err)
		}
		if _, err := c.cron.AddJob("@every 30s", tlsPathSync); err != nil {
			logger.Warning("failed to register tls-path sync job: ", err)
		}
	}()

	return nil
}

func (c *CronJob) Stop() {
	if c.cron != nil {
		c.cron.Stop()
	}
}
