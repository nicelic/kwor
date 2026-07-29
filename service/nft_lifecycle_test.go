package service

import (
	"errors"
	"strings"
	"testing"
)

func withNftLifecycleTestGlobals(t *testing.T) {
	t.Helper()
	withNftCapabilitiesTestGlobals(t)

	originalGOOS := nftLifecycleRuntimeGOOS
	originalHasDatabase := nftLifecycleHasDatabaseFn
	originalSyncFirewall := nftLifecycleSyncFirewallFn
	originalSyncPortForward := nftLifecycleSyncPortForwardFn
	originalSyncTrafficCap := nftLifecycleSyncTrafficCapFn
	originalPrepareLayout := nftLifecyclePrepareLayoutFn
	originalMarkLayout := nftLifecycleMarkLayoutAppliedFn
	originalVerifyLayout := nftLifecycleVerifyLayoutFn
	originalSyncDefault := nftLifecycleSyncDefaultCoreFn
	originalSyncMihomo := nftLifecycleSyncMihomoCoreFn
	originalCleanupDefault := nftLifecycleCleanupDefaultCoreFn
	originalCleanupMihomo := nftLifecycleCleanupMihomoCoreFn
	originalCleanupFirewall := nftLifecycleCleanupFirewallFn
	originalCleanupPortForward := nftLifecycleCleanupPortForwardFn
	originalCleanupTrafficCap := nftLifecycleCleanupTrafficCapFn
	originalCommandCleanup := nftLifecycleCommandCleanupFn
	t.Cleanup(func() {
		nftLifecycleRuntimeGOOS = originalGOOS
		nftLifecycleHasDatabaseFn = originalHasDatabase
		nftLifecycleSyncFirewallFn = originalSyncFirewall
		nftLifecycleSyncPortForwardFn = originalSyncPortForward
		nftLifecycleSyncTrafficCapFn = originalSyncTrafficCap
		nftLifecyclePrepareLayoutFn = originalPrepareLayout
		nftLifecycleMarkLayoutAppliedFn = originalMarkLayout
		nftLifecycleVerifyLayoutFn = originalVerifyLayout
		nftLifecycleSyncDefaultCoreFn = originalSyncDefault
		nftLifecycleSyncMihomoCoreFn = originalSyncMihomo
		nftLifecycleCleanupDefaultCoreFn = originalCleanupDefault
		nftLifecycleCleanupMihomoCoreFn = originalCleanupMihomo
		nftLifecycleCleanupFirewallFn = originalCleanupFirewall
		nftLifecycleCleanupPortForwardFn = originalCleanupPortForward
		nftLifecycleCleanupTrafficCapFn = originalCleanupTrafficCap
		nftLifecycleCommandCleanupFn = originalCommandCleanup
	})

	setNftCapabilitiesForTest(buildNftablesCapabilities("nftables v0.9.8", "5.10.0", nil))
	clearNftCapabilityLayoutApplied()
	clearNftLifecycleBaseRestore()
	nftLifecycleRuntimeGOOS = func() string { return "linux" }
	nftLifecycleHasDatabaseFn = func() bool { return true }
	nftLifecyclePrepareLayoutFn = func() (bool, error) { return true, nil }
	nftLifecycleSyncFirewallFn = func() error { return nil }
	nftLifecycleSyncPortForwardFn = func() error { return nil }
	nftLifecycleSyncTrafficCapFn = func() error { return nil }
	nftLifecycleSyncDefaultCoreFn = func() {}
	nftLifecycleSyncMihomoCoreFn = func() {}
	nftLifecycleVerifyLayoutFn = func() error { return nil }
	nftLifecycleMarkLayoutAppliedFn = markNftCapabilityLayoutApplied
}

func TestSyncManagedNftablesOnStartupRunsBaseRestoreWithoutMarkingLayout(t *testing.T) {
	withNftLifecycleTestGlobals(t)

	calls := make([]string, 0, 7)
	nftLifecyclePrepareLayoutFn = func() (bool, error) { calls = append(calls, "prepare"); return true, nil }
	nftLifecycleSyncFirewallFn = func() error { calls = append(calls, "firewall"); return nil }
	nftLifecycleSyncTrafficCapFn = func() error { calls = append(calls, "traffic-cap"); return nil }
	nftLifecycleSyncDefaultCoreFn = func() { calls = append(calls, "default-core") }
	nftLifecycleSyncMihomoCoreFn = func() { calls = append(calls, "mihomo-core") }
	nftLifecycleVerifyLayoutFn = func() error { calls = append(calls, "verify"); return nil }
	nftLifecycleMarkLayoutAppliedFn = func() { calls = append(calls, "mark") }

	if err := syncManagedNftablesOnStartup(); err != nil {
		t.Fatalf("base restore failed: %v", err)
	}
	want := []string{"prepare", "firewall", "traffic-cap", "default-core", "mihomo-core"}
	if strings.Join(calls, ",") != strings.Join(want, ",") {
		t.Fatalf("unexpected early restore order: got=%v want=%v", calls, want)
	}
	if !nftCapabilityLayoutReconcilePending() {
		t.Fatal("base restore alone must leave the renderer layout pending")
	}
}

func TestNftLifecycleMarksOnlyAfterPortForwardAndFinalVerification(t *testing.T) {
	withNftLifecycleTestGlobals(t)

	calls := make([]string, 0, 8)
	nftLifecyclePrepareLayoutFn = func() (bool, error) { calls = append(calls, "prepare"); return true, nil }
	nftLifecycleSyncFirewallFn = func() error { calls = append(calls, "firewall"); return nil }
	nftLifecycleSyncTrafficCapFn = func() error { calls = append(calls, "traffic-cap"); return nil }
	nftLifecycleSyncDefaultCoreFn = func() { calls = append(calls, "default-core") }
	nftLifecycleSyncMihomoCoreFn = func() { calls = append(calls, "mihomo-core") }
	nftLifecycleSyncPortForwardFn = func() error { calls = append(calls, "port-forward"); return nil }
	nftLifecycleVerifyLayoutFn = func() error { calls = append(calls, "verify"); return nil }
	nftLifecycleMarkLayoutAppliedFn = func() { calls = append(calls, "mark"); markNftCapabilityLayoutApplied() }

	if err := syncManagedNftablesOnStartup(); err != nil {
		t.Fatalf("base restore failed: %v", err)
	}
	if err := syncPortForwardNftablesAfterListenersOnStartup(); err != nil {
		t.Fatalf("final restore failed: %v", err)
	}
	want := []string{"prepare", "firewall", "traffic-cap", "default-core", "mihomo-core", "port-forward", "verify", "mark"}
	if strings.Join(calls, ",") != strings.Join(want, ",") {
		t.Fatalf("unexpected two-phase restore order: got=%v want=%v", calls, want)
	}
	if nftCapabilityLayoutReconcilePending() {
		t.Fatal("successful complete verification must mark the renderer layout applied")
	}
	if nftCapabilityLayoutLastApplyError() != "" {
		t.Fatalf("successful apply retained an error: %q", nftCapabilityLayoutLastApplyError())
	}
}

func TestNftLifecycleKeepsPendingAfterBaseRestoreFailure(t *testing.T) {
	withNftLifecycleTestGlobals(t)

	verified := false
	marked := false
	nftLifecycleSyncTrafficCapFn = func() error { return errors.New("traffic cap restore failed") }
	nftLifecycleVerifyLayoutFn = func() error { verified = true; return nil }
	nftLifecycleMarkLayoutAppliedFn = func() { marked = true }

	if err := syncManagedNftablesOnStartup(); err == nil || !strings.Contains(err.Error(), "traffic cap restore failed") {
		t.Fatalf("unexpected base restore error: %v", err)
	}
	if err := syncPortForwardNftablesAfterListenersOnStartup(); err == nil || !strings.Contains(err.Error(), "traffic cap restore failed") {
		t.Fatalf("late phase must retain the base restore failure: %v", err)
	}
	if verified || marked {
		t.Fatalf("failed base restore must skip final verification and mark: verified=%v marked=%v", verified, marked)
	}
	if !strings.Contains(nftCapabilityLayoutLastApplyError(), "traffic cap restore failed") {
		t.Fatalf("missing persisted apply diagnostic: %q", nftCapabilityLayoutLastApplyError())
	}
}

func TestNftLifecycleKeepsPendingAfterPortForwardFailure(t *testing.T) {
	withNftLifecycleTestGlobals(t)

	verified := false
	marked := false
	nftLifecycleSyncPortForwardFn = func() error { return errors.New("forward restore failed") }
	nftLifecycleVerifyLayoutFn = func() error { verified = true; return nil }
	nftLifecycleMarkLayoutAppliedFn = func() { marked = true }

	if err := syncManagedNftablesOnStartup(); err != nil {
		t.Fatalf("base restore failed: %v", err)
	}
	if err := syncPortForwardNftablesAfterListenersOnStartup(); err == nil || !strings.Contains(err.Error(), "forward restore failed") {
		t.Fatalf("unexpected forwarding restore error: %v", err)
	}
	if verified || marked {
		t.Fatalf("failed forwarding restore must skip verification and mark: verified=%v marked=%v", verified, marked)
	}
}

func TestNftLifecycleKeepsPendingAfterFinalVerificationFailure(t *testing.T) {
	withNftLifecycleTestGlobals(t)

	marked := false
	nftLifecycleVerifyLayoutFn = func() error { return errors.New("client limit verification failed") }
	nftLifecycleMarkLayoutAppliedFn = func() { marked = true }

	if err := syncManagedNftablesOnStartup(); err != nil {
		t.Fatalf("base restore failed: %v", err)
	}
	if err := syncPortForwardNftablesAfterListenersOnStartup(); err == nil || !strings.Contains(err.Error(), "client limit verification failed") {
		t.Fatalf("unexpected verification error: %v", err)
	}
	if marked {
		t.Fatal("failed final verification must leave the renderer layout pending")
	}
}

func TestSyncPortForwardNftablesStillRunsAfterLaterCoreReadyEvents(t *testing.T) {
	withNftLifecycleTestGlobals(t)
	markNftCapabilityLayoutApplied()

	calls := 0
	nftLifecycleSyncPortForwardFn = func() error { calls++; return nil }
	SyncPortForwardNftablesAfterListenersOnStartup()
	SyncPortForwardNftablesAfterCoreRuntimeReady()
	if calls != 2 {
		t.Fatalf("expected forwarding sync after listeners and core readiness, got %d calls", calls)
	}
}

func TestCoreRuntimeReadyVerificationFailureReturnsLayoutToPending(t *testing.T) {
	withNftLifecycleTestGlobals(t)
	markNftCapabilityLayoutApplied()
	nftLifecycleVerifyLayoutFn = func() error { return errors.New("late client block failed") }

	err := syncPortForwardNftablesAfterCoreRuntimeReady()
	if err == nil || !strings.Contains(err.Error(), "late client block failed") {
		t.Fatalf("unexpected core-ready verification error: %v", err)
	}
	if !nftCapabilityLayoutReconcilePending() {
		t.Fatal("late managed-rule verification failure must return the layout to pending")
	}
	if !strings.Contains(nftCapabilityLayoutLastApplyError(), "late client block failed") {
		t.Fatalf("missing late verification diagnostic: %q", nftCapabilityLayoutLastApplyError())
	}
}

func TestVerifyNftablesCapabilityLayoutRestoreCoversAllManagedRuleFamilies(t *testing.T) {
	withNftLifecycleTestGlobals(t)
	nftSupportedFn = func() bool { return true }

	originalDefaultRunning := nftLayoutDefaultCoreRunningFn
	originalMihomoRunning := nftLayoutMihomoCoreRunningFn
	originalDefaultInbound := nftLayoutVerifyDefaultInboundFn
	originalDefaultLimit := nftLayoutVerifyDefaultLimitFn
	originalDefaultBlock := nftLayoutVerifyDefaultBlockFn
	originalMihomoInbound := nftLayoutVerifyMihomoInboundFn
	originalMihomoLimit := nftLayoutVerifyMihomoLimitFn
	originalMihomoBlock := nftLayoutVerifyMihomoBlockFn
	originalPortForward := nftLayoutVerifyPortForwardFn
	t.Cleanup(func() {
		nftLayoutDefaultCoreRunningFn = originalDefaultRunning
		nftLayoutMihomoCoreRunningFn = originalMihomoRunning
		nftLayoutVerifyDefaultInboundFn = originalDefaultInbound
		nftLayoutVerifyDefaultLimitFn = originalDefaultLimit
		nftLayoutVerifyDefaultBlockFn = originalDefaultBlock
		nftLayoutVerifyMihomoInboundFn = originalMihomoInbound
		nftLayoutVerifyMihomoLimitFn = originalMihomoLimit
		nftLayoutVerifyMihomoBlockFn = originalMihomoBlock
		nftLayoutVerifyPortForwardFn = originalPortForward
	})

	calls := make([]string, 0, 7)
	nftLayoutDefaultCoreRunningFn = func() bool { return true }
	nftLayoutMihomoCoreRunningFn = func() bool { return true }
	nftLayoutVerifyDefaultInboundFn = func() error { calls = append(calls, "default-inbound"); return nil }
	nftLayoutVerifyDefaultLimitFn = func() error { calls = append(calls, "default-limit"); return errors.New("default limit failed") }
	nftLayoutVerifyDefaultBlockFn = func() error { calls = append(calls, "default-block"); return nil }
	nftLayoutVerifyMihomoInboundFn = func() error { calls = append(calls, "mihomo-inbound"); return nil }
	nftLayoutVerifyMihomoLimitFn = func() error { calls = append(calls, "mihomo-limit"); return nil }
	nftLayoutVerifyMihomoBlockFn = func() error { calls = append(calls, "mihomo-block"); return errors.New("mihomo block failed") }
	nftLayoutVerifyPortForwardFn = func() error { calls = append(calls, "port-forward"); return nil }

	err := verifyNftablesCapabilityLayoutRestore()
	want := []string{"default-inbound", "default-limit", "default-block", "mihomo-inbound", "mihomo-limit", "mihomo-block", "port-forward"}
	if strings.Join(calls, ",") != strings.Join(want, ",") {
		t.Fatalf("unexpected verification coverage: got=%v want=%v", calls, want)
	}
	if err == nil || !strings.Contains(err.Error(), "default limit failed") || !strings.Contains(err.Error(), "mihomo block failed") {
		t.Fatalf("verification errors were not accumulated: %v", err)
	}
}

func TestCleanupManagedNftablesOnShutdownInvokesTrafficCapCleanup(t *testing.T) {
	withNftLifecycleTestGlobals(t)
	nftLifecycleCleanupDefaultCoreFn = func() {}
	nftLifecycleCleanupMihomoCoreFn = func() {}
	nftLifecycleCleanupFirewallFn = func() {}
	nftLifecycleCleanupPortForwardFn = func() {}

	calls := make([]string, 0, 2)
	nftLifecycleCleanupTrafficCapFn = func() error { calls = append(calls, "traffic-cap"); return nil }
	nftLifecycleCommandCleanupFn = func() { calls = append(calls, "command-cleanup") }

	CleanupManagedNftablesOnShutdown()
	want := []string{"traffic-cap", "command-cleanup"}
	if strings.Join(calls, ",") != strings.Join(want, ",") {
		t.Fatalf("unexpected shutdown cleanup calls: got=%v want=%v", calls, want)
	}
}
