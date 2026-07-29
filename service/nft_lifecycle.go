package service

import (
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/logger"
)

var nftLifecycleRestoreState = struct {
	mu               sync.RWMutex
	baseSignature    string
	baseRestoreError string
}{}

var (
	nftLifecycleRuntimeGOOS = func() string {
		return GetSystemPlatformOS()
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
	nftLifecyclePrepareLayoutFn      = prepareNftablesCapabilityLayoutTransition
	nftLifecycleMarkLayoutAppliedFn  = markNftCapabilityLayoutApplied
	nftLifecycleVerifyLayoutFn       = verifyNftablesCapabilityLayoutRestore
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
	if err := syncManagedNftablesOnStartup(); err != nil {
		logger.Warning("startup managed nftables restore failed: ", err)
	}
}

func syncManagedNftablesOnStartup() error {
	if nftLifecycleRuntimeGOOS() != "linux" {
		return nil
	}

	// A panel-only restart deliberately preserves nftables rules. Serialize the
	// capability hand-over here so an upgraded nft/kernel pair cannot leave the
	// previous NAT layout alongside the restored one.
	nftLayoutTransitionMu.Lock()
	defer nftLayoutTransitionMu.Unlock()

	signature := nftCapabilityLayoutSignature()
	if err := ensureNftRendererSupported(); err != nil {
		recordNftLifecycleBaseRestore(signature, err)
		setNftCapabilityLayoutApplyError(err)
		return err
	}

	_, transitionErr := nftLifecyclePrepareLayoutFn()
	if transitionErr != nil {
		err := fmt.Errorf("prepare nftables capability layout: %w", transitionErr)
		recordNftLifecycleBaseRestore(signature, err)
		setNftCapabilityLayoutApplyError(err)
		return err
	}

	restoreErrors := make([]error, 0, 2)

	if err := nftLifecycleSyncFirewallFn(); err != nil {
		logger.Warning("startup firewall nft sync failed: ", err)
		restoreErrors = append(restoreErrors, fmt.Errorf("restore firewall nftables: %w", err))
	}
	if err := nftLifecycleSyncTrafficCapFn(); err != nil {
		logger.Warning("startup traffic-cap nft sync failed: ", err)
		restoreErrors = append(restoreErrors, fmt.Errorf("restore traffic-cap nftables: %w", err))
	}

	nftLifecycleSyncDefaultCoreFn()
	nftLifecycleSyncMihomoCoreFn()

	restoreErr := errors.Join(restoreErrors...)
	recordNftLifecycleBaseRestore(signature, restoreErr)
	if nftCapabilityLayoutReconcilePending() {
		setNftCapabilityLayoutApplyError(restoreErr)
	}
	return restoreErr
}

// SyncPortForwardNftablesAfterListenersOnStartup runs after the panel,
// subscription and reverse-proxy listeners have bound their sockets. This
// preserves historical forwarding rules while allowing their first runtime
// snapshot to report real conflicts instead of racing startup.
func SyncPortForwardNftablesAfterListenersOnStartup() {
	if err := syncPortForwardNftablesAfterListenersOnStartup(); err != nil {
		logger.Warning("port-forward nft sync after listeners failed: ", err)
	}
}

func syncPortForwardNftablesAfterListenersOnStartup() error {
	if nftLifecycleRuntimeGOOS() != "linux" || !nftLifecycleHasDatabaseFn() {
		return nil
	}

	nftLayoutTransitionMu.Lock()
	defer nftLayoutTransitionMu.Unlock()

	forwardErr := nftLifecycleSyncPortForwardFn()
	if !nftCapabilityLayoutReconcilePending() {
		return forwardErr
	}

	signature := nftCapabilityLayoutSignature()
	baseSignature, baseRestoreError := readNftLifecycleBaseRestore()
	applyErrors := make([]error, 0, 3)
	if baseSignature != signature {
		applyErrors = append(applyErrors, fmt.Errorf("base nftables restore has not completed for renderer %s", signature))
	}
	if strings.TrimSpace(baseRestoreError) != "" {
		applyErrors = append(applyErrors, errors.New(baseRestoreError))
	}
	if forwardErr != nil {
		applyErrors = append(applyErrors, fmt.Errorf("restore port-forward nftables: %w", forwardErr))
	}
	if len(applyErrors) == 0 {
		if err := nftLifecycleVerifyLayoutFn(); err != nil {
			applyErrors = append(applyErrors, fmt.Errorf("verify restored nftables layout: %w", err))
		}
	}

	applyErr := errors.Join(applyErrors...)
	if applyErr != nil {
		setNftCapabilityLayoutApplyError(applyErr)
		return applyErr
	}
	nftLifecycleMarkLayoutAppliedFn()
	return nil
}

// SyncPortForwardNftablesAfterCoreRuntimeReady is also used after a managed
// default or Mihomo core starts/restarts, because those inbounds may have
// introduced listeners after the initial startup pass.
func SyncPortForwardNftablesAfterCoreRuntimeReady() {
	if err := syncPortForwardNftablesAfterCoreRuntimeReady(); err != nil {
		logger.Warning("managed nftables verification after core runtime ready failed: ", err)
	}
}

func syncPortForwardNftablesAfterCoreRuntimeReady() error {
	pendingBeforeSync := nftCapabilityLayoutReconcilePending()
	if err := syncPortForwardNftablesAfterListenersOnStartup(); err != nil {
		if !pendingBeforeSync {
			markNftCapabilityLayoutPending(err)
		}
		return err
	}
	if pendingBeforeSync || nftLifecycleRuntimeGOOS() != "linux" || !nftLifecycleHasDatabaseFn() {
		return nil
	}

	nftLayoutTransitionMu.Lock()
	defer nftLayoutTransitionMu.Unlock()
	if nftCapabilityLayoutReconcilePending() {
		return nil
	}
	if err := nftLifecycleVerifyLayoutFn(); err != nil {
		verificationErr := fmt.Errorf("verify nftables after core runtime ready: %w", err)
		markNftCapabilityLayoutPending(verificationErr)
		return verificationErr
	}
	return nil
}

// CleanupManagedNftablesOnShutdown removes runtime nftables rules while keeping
// DB mirror records for next startup restoration.
func CleanupManagedNftablesOnShutdown() {
	if nftLifecycleRuntimeGOOS() != "linux" {
		clearNftCapabilityLayoutApplied()
		clearNftLifecycleBaseRestore()
		return
	}
	if !nftLifecycleHasDatabaseFn() {
		nftLifecycleCommandCleanupFn()
		clearNftCapabilityLayoutApplied()
		clearNftLifecycleBaseRestore()
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
	clearNftCapabilityLayoutApplied()
	clearNftLifecycleBaseRestore()
}

func recordNftLifecycleBaseRestore(signature string, err error) {
	message := ""
	if err != nil {
		message = strings.TrimSpace(err.Error())
	}
	nftLifecycleRestoreState.mu.Lock()
	nftLifecycleRestoreState.baseSignature = signature
	nftLifecycleRestoreState.baseRestoreError = message
	nftLifecycleRestoreState.mu.Unlock()
}

func readNftLifecycleBaseRestore() (string, string) {
	nftLifecycleRestoreState.mu.RLock()
	signature := nftLifecycleRestoreState.baseSignature
	restoreError := nftLifecycleRestoreState.baseRestoreError
	nftLifecycleRestoreState.mu.RUnlock()
	return signature, restoreError
}

func clearNftLifecycleBaseRestore() {
	nftLifecycleRestoreState.mu.Lock()
	nftLifecycleRestoreState.baseSignature = ""
	nftLifecycleRestoreState.baseRestoreError = ""
	nftLifecycleRestoreState.mu.Unlock()
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
