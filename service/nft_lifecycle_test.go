package service

import "testing"

func TestSyncManagedNftablesOnStartupInvokesExistingRestoreChain(t *testing.T) {
	originalGOOS := nftLifecycleRuntimeGOOS
	originalSyncFirewall := nftLifecycleSyncFirewallFn
	originalSyncPortForward := nftLifecycleSyncPortForwardFn
	originalSyncTrafficCap := nftLifecycleSyncTrafficCapFn
	originalSyncDefault := nftLifecycleSyncDefaultCoreFn
	originalSyncMihomo := nftLifecycleSyncMihomoCoreFn
	defer func() {
		nftLifecycleRuntimeGOOS = originalGOOS
		nftLifecycleSyncFirewallFn = originalSyncFirewall
		nftLifecycleSyncPortForwardFn = originalSyncPortForward
		nftLifecycleSyncTrafficCapFn = originalSyncTrafficCap
		nftLifecycleSyncDefaultCoreFn = originalSyncDefault
		nftLifecycleSyncMihomoCoreFn = originalSyncMihomo
	}()

	nftLifecycleRuntimeGOOS = func() string { return "linux" }
	calls := make([]string, 0, 5)
	nftLifecycleSyncFirewallFn = func() error {
		calls = append(calls, "firewall")
		return nil
	}
	nftLifecycleSyncPortForwardFn = func() error {
		calls = append(calls, "port-forward")
		return nil
	}
	nftLifecycleSyncTrafficCapFn = func() error {
		calls = append(calls, "traffic-cap")
		return nil
	}
	nftLifecycleSyncDefaultCoreFn = func() {
		calls = append(calls, "default-core")
	}
	nftLifecycleSyncMihomoCoreFn = func() {
		calls = append(calls, "mihomo-core")
	}

	SyncManagedNftablesOnStartup()

	want := []string{"firewall", "port-forward", "traffic-cap", "default-core", "mihomo-core"}
	if len(calls) != len(want) {
		t.Fatalf("unexpected startup restore calls: got=%v want=%v", calls, want)
	}
	for i := range want {
		if calls[i] != want[i] {
			t.Fatalf("unexpected startup restore call %d: got=%q want=%q", i, calls[i], want[i])
		}
	}
}

func TestSyncManagedNftablesOnStartupCleansTemporaryFirewallRulesFirst(t *testing.T) {
	originalGOOS := nftLifecycleRuntimeGOOS
	originalSyncFirewall := nftLifecycleSyncFirewallFn
	originalSyncPortForward := nftLifecycleSyncPortForwardFn
	originalSyncTrafficCap := nftLifecycleSyncTrafficCapFn
	originalSyncDefault := nftLifecycleSyncDefaultCoreFn
	originalSyncMihomo := nftLifecycleSyncMihomoCoreFn
	defer func() {
		nftLifecycleRuntimeGOOS = originalGOOS
		nftLifecycleSyncFirewallFn = originalSyncFirewall
		nftLifecycleSyncPortForwardFn = originalSyncPortForward
		nftLifecycleSyncTrafficCapFn = originalSyncTrafficCap
		nftLifecycleSyncDefaultCoreFn = originalSyncDefault
		nftLifecycleSyncMihomoCoreFn = originalSyncMihomo
	}()

	nftLifecycleRuntimeGOOS = func() string { return "linux" }
	calls := make([]string, 0, 5)
	nftLifecycleSyncFirewallFn = func() error {
		calls = append(calls, "firewall")
		return nil
	}
	nftLifecycleSyncPortForwardFn = func() error {
		calls = append(calls, "port-forward")
		return nil
	}
	nftLifecycleSyncTrafficCapFn = func() error {
		calls = append(calls, "traffic-cap")
		return nil
	}
	nftLifecycleSyncDefaultCoreFn = func() {
		calls = append(calls, "default-core")
	}
	nftLifecycleSyncMihomoCoreFn = func() {
		calls = append(calls, "mihomo-core")
	}

	SyncManagedNftablesOnStartup()

	if len(calls) == 0 || calls[0] != "firewall" {
		t.Fatalf("expected firewall sync to run first after temporary cleanup, got=%v", calls)
	}
}

func TestCleanupManagedNftablesOnShutdownInvokesTrafficCapCleanup(t *testing.T) {
	originalGOOS := nftLifecycleRuntimeGOOS
	originalHasDatabase := nftLifecycleHasDatabaseFn
	originalCleanupDefault := nftLifecycleCleanupDefaultCoreFn
	originalCleanupMihomo := nftLifecycleCleanupMihomoCoreFn
	originalCleanupFirewall := nftLifecycleCleanupFirewallFn
	originalCleanupPortForward := nftLifecycleCleanupPortForwardFn
	originalCleanupTrafficCap := nftLifecycleCleanupTrafficCapFn
	originalCommandCleanup := nftLifecycleCommandCleanupFn
	defer func() {
		nftLifecycleRuntimeGOOS = originalGOOS
		nftLifecycleHasDatabaseFn = originalHasDatabase
		nftLifecycleCleanupDefaultCoreFn = originalCleanupDefault
		nftLifecycleCleanupMihomoCoreFn = originalCleanupMihomo
		nftLifecycleCleanupFirewallFn = originalCleanupFirewall
		nftLifecycleCleanupPortForwardFn = originalCleanupPortForward
		nftLifecycleCleanupTrafficCapFn = originalCleanupTrafficCap
		nftLifecycleCommandCleanupFn = originalCommandCleanup
	}()

	nftLifecycleRuntimeGOOS = func() string { return "linux" }
	nftLifecycleHasDatabaseFn = func() bool { return true }
	nftLifecycleCleanupDefaultCoreFn = func() {}
	nftLifecycleCleanupMihomoCoreFn = func() {}
	nftLifecycleCleanupFirewallFn = func() {}
	nftLifecycleCleanupPortForwardFn = func() {}

	calls := make([]string, 0, 2)
	nftLifecycleCleanupTrafficCapFn = func() error {
		calls = append(calls, "traffic-cap")
		return nil
	}
	nftLifecycleCommandCleanupFn = func() {
		calls = append(calls, "command-cleanup")
	}

	CleanupManagedNftablesOnShutdown()

	want := []string{"traffic-cap", "command-cleanup"}
	if len(calls) != len(want) {
		t.Fatalf("unexpected shutdown cleanup calls: got=%v want=%v", calls, want)
	}
	for i := range want {
		if calls[i] != want[i] {
			t.Fatalf("unexpected shutdown cleanup call %d: got=%q want=%q", i, calls[i], want[i])
		}
	}
}
