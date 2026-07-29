package service

import (
	"errors"
	"fmt"
	"sync"
)

// nftLayoutTransitionMu serializes lifecycle layout migrations. Normal rule
// writers keep their existing service locks; this lock prevents two lifecycle
// refreshes from deleting and restoring different layouts concurrently.
var nftLayoutTransitionMu sync.Mutex

var (
	nftLayoutDefaultCoreRunningFn   = func() bool { return (&CoreManagerService{}).IsRunning() }
	nftLayoutMihomoCoreRunningFn    = func() bool { return (&MihomoCoreManagerService{}).IsRunning() }
	nftLayoutVerifyDefaultInboundFn = func() error {
		return (&NftTrafficService{}).EnsureRuleIntegrity()
	}
	nftLayoutVerifyDefaultLimitFn = func() error {
		return (&ClientRateLimitService{}).EnsureRuleIntegrity()
	}
	nftLayoutVerifyDefaultBlockFn = func() error {
		return (&ClientPortBlockService{}).EnsureRuleIntegrity()
	}
	nftLayoutVerifyMihomoInboundFn = func() error {
		return (&MihomoNftTrafficService{}).EnsureRuleIntegrity()
	}
	nftLayoutVerifyMihomoLimitFn = func() error {
		return (&MihomoClientRateLimitService{}).EnsureRuleIntegrity()
	}
	nftLayoutVerifyMihomoBlockFn = func() error {
		return (&MihomoClientPortBlockService{}).EnsureRuleIntegrity()
	}
	nftLayoutVerifyPortForwardFn = func() error {
		return (&PortForwardService{}).SyncIfNeeded(0)
	}
)

// prepareNftablesCapabilityLayoutTransition removes NAT artifacts from both
// managed layouts when the cached capability profile changed. Callers must
// restore their runtime rules afterwards and only then mark the signature as
// applied. No database transaction is held while this function runs.
func prepareNftablesCapabilityLayoutTransition() (bool, error) {
	if !nftCapabilityLayoutReconcilePending() {
		return false, nil
	}
	if !nftSupported() {
		// There cannot be live panel-managed nft rules without the binary. Keep
		// the transition pending so a later successful install restores the
		// profile rather than silently accepting an unverified state.
		return false, nil
	}

	// A non-empty signature proves that this process previously rendered a
	// different capability profile, so both layouts must be removed before the
	// restore. On a cold start there is no such proof: deleting the matching
	// native table would also delete its named counters during an ordinary
	// panel-only restart. Inspect only the opposite layout in that case. Its
	// presence proves a cross-layout hand-over (or stale residue), which still
	// requires the full cleanup.
	needsFullCleanup := nftCapabilityLayoutHasAppliedSignature()
	if !needsFullCleanup {
		hasOppositeLayout, err := nftHasOppositeManagedNatLayout()
		if err != nil {
			return true, err
		}
		needsFullCleanup = hasOppositeLayout
	}
	if needsFullCleanup {
		if err := cleanupManagedNftNatLayouts(); err != nil {
			return true, err
		}
	}
	return true, nil
}

// nftHasOppositeManagedNatLayout checks only artifacts that uniquely identify
// the non-current layout. The shared inet default-core and forwarding tables
// also hold filter rules, so their mere existence cannot be used as evidence
// of a NAT layout change.
func nftHasOppositeManagedNatLayout() (bool, error) {
	if nftUsesCompatibilityLayout() {
		for _, target := range []nftRuleLocation{
			{tableFamily: nftFamily, table: nftTable, chain: nftChainPrerouting},
			{tableFamily: nftFamily, table: nftTable, chain: nftChainPostrouting},
			{tableFamily: nftFamily, table: portForwardNftTable, chain: portForwardPreroutingChain},
			{tableFamily: nftFamily, table: portForwardNftTable, chain: portForwardPostroutingChain},
		} {
			exists, err := nftManagedChainExists(target)
			if err != nil {
				return false, err
			}
			if exists {
				return true, nil
			}
		}
		return false, nil
	}

	for _, table := range []struct {
		family string
		name   string
	}{
		{family: nftNatFamilyIPv4, name: nftTable},
		{family: nftNatFamilyIPv6, name: nftTable},
		{family: nftNatFamilyIPv4, name: portForwardNftTable},
		{family: nftNatFamilyIPv6, name: portForwardNftTable},
	} {
		exists, err := nftManagedTableExists(table.family, table.name)
		if err != nil {
			return false, err
		}
		if exists {
			return true, nil
		}
	}
	return false, nil
}

func nftManagedTableExists(tableFamily string, table string) (bool, error) {
	return inspectOwnedNftTableForMutation(tableFamily, table)
}

func nftManagedChainExists(location nftRuleLocation) (bool, error) {
	exists, err := inspectOwnedNftTableForMutation(location.tableFamily, location.table)
	if err != nil || !exists {
		return false, err
	}
	_, err = runNft("list", "chain", location.tableFamily, location.table, location.chain)
	if err == nil {
		return true, nil
	}
	if nftObjectMissing(err) {
		return false, nil
	}
	return false, err
}

// cleanupManagedNftNatLayouts deliberately touches only tables/chains owned
// by the panel. The shared inet default-core table keeps its filter chains;
// only its NAT chains are removed. Compatibility ip/ip6 tables contain only
// panel-managed NAT chains and can therefore be deleted as complete tables.
func cleanupManagedNftNatLayouts() error {
	var firstErr error
	if err := removeNativeNftNatChain(); err != nil && !nftObjectMissing(err) {
		firstErr = fmt.Errorf("remove native managed NAT chains: %w", err)
	}
	if err := cleanupNftCompatibilityNatTables(); err != nil && firstErr == nil {
		firstErr = fmt.Errorf("remove compatibility managed NAT tables: %w", err)
	}
	if err := cleanupManagedPortForwardTable(); err != nil && firstErr == nil {
		firstErr = fmt.Errorf("remove managed port-forward tables: %w", err)
	}
	return firstErr
}

// verifyNftablesCapabilityLayoutRestore performs a final strict REDIRECT
// integrity pass after the lifecycle has restored panel-managed rules. It is
// deliberately limited to a capability transition, so normal overview polls
// never add another database or nftables scan.
func verifyNftablesCapabilityLayoutRestore() error {
	if !nftSupported() {
		return nil
	}
	verificationErrors := make([]error, 0, 7)
	if nftLayoutDefaultCoreRunningFn() {
		if err := nftLayoutVerifyDefaultInboundFn(); err != nil {
			verificationErrors = append(verificationErrors, fmt.Errorf("verify default-core inbound nftables: %w", err))
		}
		if err := nftLayoutVerifyDefaultLimitFn(); err != nil {
			verificationErrors = append(verificationErrors, fmt.Errorf("verify default-core client rate limits: %w", err))
		}
		if err := nftLayoutVerifyDefaultBlockFn(); err != nil {
			verificationErrors = append(verificationErrors, fmt.Errorf("verify default-core client blocks: %w", err))
		}
	}
	if nftLayoutMihomoCoreRunningFn() {
		if err := nftLayoutVerifyMihomoInboundFn(); err != nil {
			verificationErrors = append(verificationErrors, fmt.Errorf("verify mihomo inbound nftables: %w", err))
		}
		if err := nftLayoutVerifyMihomoLimitFn(); err != nil {
			verificationErrors = append(verificationErrors, fmt.Errorf("verify mihomo client rate limits: %w", err))
		}
		if err := nftLayoutVerifyMihomoBlockFn(); err != nil {
			verificationErrors = append(verificationErrors, fmt.Errorf("verify mihomo client blocks: %w", err))
		}
	}
	if nftLifecycleHasDatabaseFn() {
		if err := nftLayoutVerifyPortForwardFn(); err != nil {
			verificationErrors = append(verificationErrors, fmt.Errorf("verify port-forward nftables: %w", err))
		}
	}
	return errors.Join(verificationErrors...)
}
