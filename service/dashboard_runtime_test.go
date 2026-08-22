package service

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestNormalizeDashboardRuntimeRequestCanonicalizesAndFilters(t *testing.T) {
	got := normalizeDashboardRuntimeRequest(" mem,CPU,unknown, cpu ,sbd,net ")
	if got != "cpu,mem,net,sbd" {
		t.Fatalf("normalized dashboard request = %q, want %q", got, "cpu,mem,net,sbd")
	}
}

func TestPreferManagedCoreRuntimeStatsUsesCgroupBeforeRSS(t *testing.T) {
	got := preferManagedCoreRuntimeStatsFromCgroup(
		systemdUnitStats{Memory: 900, Tasks: 7, UptimeSec: 30},
		runtimeStats{MemoryBytes: 400, Threads: 3, Uptime: 12},
	)
	if got.MemoryBytes != 900 || got.Threads != 7 || got.Uptime != 30 {
		t.Fatalf("cgroup runtime stats = %#v, want cgroup values", got)
	}
	got = preferManagedCoreRuntimeStatsFromCgroup(systemdUnitStats{}, runtimeStats{MemoryBytes: 400, Threads: 3, Uptime: 12})
	if got.MemoryBytes != 400 || got.Threads != 3 || got.Uptime != 12 {
		t.Fatalf("RSS fallback runtime stats = %#v, want process values", got)
	}
}

func TestMergeManagedCoreProcessStatsFillsMissingSystemdMetrics(t *testing.T) {
	stats := systemdUnitStats{Active: true, MainPID: 0, Memory: 900}
	mergeManagedCoreProcessStats(&stats, runtimeStats{MemoryBytes: 400, Threads: 3, Uptime: 12})
	if stats.Memory != 900 || stats.Tasks != 3 || stats.UptimeSec != 12 {
		t.Fatalf("merged systemd/process stats = %#v, want cgroup memory plus process fallbacks", stats)
	}
}

func TestManagedCoreRuntimeStatsFromDirectProcessKeepsRunningStateWithoutProcessMetrics(t *testing.T) {
	missing := managedCoreRuntimeStatsFromDirectProcess(true, processRuntimeStatsWithPID{})
	if !missing.Active || missing.MainPID != 0 || missing.Memory != 0 {
		t.Fatalf("direct runtime without readable process stats = %#v, want active empty stats", missing)
	}

	withProcess := managedCoreRuntimeStatsFromDirectProcess(true, processRuntimeStatsWithPID{
		pid: 42,
		runtimeStats: runtimeStats{
			MemoryBytes: 123,
			Threads:     4,
			Uptime:      9,
		},
	})
	if !withProcess.Active || withProcess.MainPID != 42 || withProcess.Memory != 123 || withProcess.Tasks != 4 || withProcess.UptimeSec != 9 {
		t.Fatalf("direct runtime with process stats = %#v, want populated active stats", withProcess)
	}

	stopped := managedCoreRuntimeStatsFromDirectProcess(false, processRuntimeStatsWithPID{pid: 42})
	if stopped.Active || stopped.MainPID != 0 {
		t.Fatalf("stopped direct runtime = %#v, want empty inactive stats", stopped)
	}
}

func TestDashboardStatusRunningReusesSingboxStatus(t *testing.T) {
	status := map[string]interface{}{
		"sbd": map[string]interface{}{"running": true, "mihomoRunning": false},
	}
	running, known := dashboardStatusRunning(&status, "sbd")
	if !known || !running {
		t.Fatalf("dashboard status running = (%v, %v), want (true, true)", running, known)
	}
	if _, known := dashboardStatusRunning(&status, "missing"); known {
		t.Fatal("missing dashboard status key must not be treated as known")
	}
	if running, known := dashboardMihomoRunning(&status); !known || running {
		t.Fatalf("dashboard Mihomo status = (%v, %v), want (false, true)", running, known)
	}
}

func TestGetCpuPercentHandlesEmptyProbeResult(t *testing.T) {
	original := cpuPercentFn
	cpuPercentFn = func(time.Duration, bool) ([]float64, error) { return []float64{}, nil }
	t.Cleanup(func() { cpuPercentFn = original })

	if got := (&ServerService{}).GetCpuPercent(); got != 0 {
		t.Fatalf("cpu percent = %v, want 0 for an empty probe result", got)
	}
}

func TestSystemctlUnitIsActiveCoalescesAndInvalidates(t *testing.T) {
	original := systemctlUnitIsActiveFn
	invalidateSystemdUnitActiveCache()
	var calls atomic.Int32
	systemctlUnitIsActiveFn = func(unit string) bool {
		if unit != "kwor-test.service" {
			t.Fatalf("unexpected unit %q", unit)
		}
		calls.Add(1)
		return true
	}
	t.Cleanup(func() {
		systemctlUnitIsActiveFn = original
		invalidateSystemdUnitActiveCache()
	})

	if !systemctlUnitIsActive("kwor-test.service") || !systemctlUnitIsActive("kwor-test.service") {
		t.Fatal("cached systemctl probe must report the fixture state")
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("systemctl probe calls=%d, want 1 before invalidation", got)
	}

	invalidateSystemdUnitActiveCache()
	if !systemctlUnitIsActive("kwor-test.service") {
		t.Fatal("systemctl probe after invalidation must report the fixture state")
	}
	if got := calls.Load(); got != 2 {
		t.Fatalf("systemctl probe calls=%d, want 2 after invalidation", got)
	}
}
