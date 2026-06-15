package service

import (
	"runtime"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/logger"
)

var (
	nftLifecycleRuntimeGOOS = func() string {
		return runtime.GOOS
	}
	nftLifecycleHasDatabaseFn = func() bool {
		return database.GetDB() != nil
	}
	nftLifecycleSyncFirewallFn = func() error {
		return (&FirewallService{}).SyncIfNeeded(0)
	}
	nftLifecycleSyncPortForwardFn = func() error {
		return (&PortForwardService{}).SyncIfNeeded(0)
	}
	nftLifecycleSyncTrafficCapFn = func() error {
		return (&TrafficOverviewService{}).reconcileTrafficCapFromOverview(nil)
	}
	nftLifecycleSyncDefaultCoreFn    = syncDefaultCoreNftablesWithCoreState
	nftLifecycleSyncMihomoCoreFn     = syncMihomoCoreNftablesWithCoreState
	nftLifecycleCleanupDefaultCoreFn = func() {
		(&NftTrafficService{}).CleanupOnShutdown()
		(&ClientRateLimitService{}).CleanupOnShutdown()
		(&ClientPortBlockService{}).CleanupOnShutdown()
	}
	nftLifecycleCleanupMihomoCoreFn = func() {
		(&MihomoNftTrafficService{}).CleanupOnShutdown()
		(&MihomoClientRateLimitService{}).CleanupOnShutdown()
		(&MihomoClientPortBlockService{}).CleanupOnShutdown()
	}
	nftLifecycleCleanupFirewallFn = func() {
		(&FirewallService{}).CleanupOnShutdown()
	}
	nftLifecycleCleanupPortForwardFn = func() {
		(&PortForwardService{}).CleanupOnShutdown()
	}
	nftLifecycleCleanupTrafficCapFn = func() error {
		return (&TrafficOverviewService{}).CleanupTrafficCapOnShutdown()
	}
	nftLifecycleCommandCleanupFn = CleanupAllNftRulesForCommand
)

// SyncManagedNftablesOnStartup restores managed nftables runtime rules from DB
// snapshots at process startup.
func SyncManagedNftablesOnStartup() {
	if nftLifecycleRuntimeGOOS() != "linux" {
		return
	}

	if err := nftLifecycleSyncFirewallFn(); err != nil {
		logger.Warning("startup firewall nft sync failed: ", err)
	}
	if err := nftLifecycleSyncPortForwardFn(); err != nil {
		logger.Warning("startup port-forward nft sync failed: ", err)
	}
	if err := nftLifecycleSyncTrafficCapFn(); err != nil {
		logger.Warning("startup traffic-cap nft sync failed: ", err)
	}

	nftLifecycleSyncDefaultCoreFn()
	nftLifecycleSyncMihomoCoreFn()
}

// CleanupManagedNftablesOnShutdown removes runtime nftables rules while keeping
// DB mirror records for next startup restoration.
func CleanupManagedNftablesOnShutdown() {
	if nftLifecycleRuntimeGOOS() != "linux" {
		return
	}
	if !nftLifecycleHasDatabaseFn() {
		nftLifecycleCommandCleanupFn()
		return
	}

	nftLifecycleCleanupDefaultCoreFn()
	nftLifecycleCleanupMihomoCoreFn()
	nftLifecycleCleanupFirewallFn()
	nftLifecycleCleanupPortForwardFn()

	if err := nftLifecycleCleanupTrafficCapFn(); err != nil {
		logger.Warning("shutdown traffic cap cleanup failed: ", err)
	}

	// Final cleanup fallback for orphan managed chains/tables.
	nftLifecycleCommandCleanupFn()
}

func syncDefaultCoreNftablesWithCoreState() {
	if (&CoreManagerService{}).IsRunning() {
		(&NftTrafficService{}).InitOnStartup()
		(&ClientRateLimitService{}).InitOnStartup()
		(&ClientPortBlockService{}).InitOnStartup()
		return
	}
	(&NftTrafficService{}).CleanupOnShutdown()
	(&ClientRateLimitService{}).CleanupOnShutdown()
	(&ClientPortBlockService{}).CleanupOnShutdown()
}

func syncMihomoCoreNftablesWithCoreState() {
	if (&MihomoCoreManagerService{}).IsRunning() {
		(&MihomoNftTrafficService{}).InitOnStartup()
		(&MihomoClientRateLimitService{}).InitOnStartup()
		(&MihomoClientPortBlockService{}).InitOnStartup()
		return
	}
	(&MihomoNftTrafficService{}).CleanupOnShutdown()
	(&MihomoClientRateLimitService{}).CleanupOnShutdown()
	(&MihomoClientPortBlockService{}).CleanupOnShutdown()
}
