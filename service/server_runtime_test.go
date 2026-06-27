package service

import "testing"

func TestPreferRuntimeStatsUsesPrimaryValues(t *testing.T) {
	primary := runtimeStats{
		MemoryBytes: 64,
		Threads:     8,
		Uptime:      120,
	}
	fallback := runtimeStats{
		MemoryBytes: 128,
		Threads:     16,
		Uptime:      240,
	}

	got := preferRuntimeStats(primary, fallback)
	if got != primary {
		t.Fatalf("expected primary runtime stats to win, got %#v", got)
	}
}

func TestPreferManagedCoreRuntimeStatsFallsBackFieldByField(t *testing.T) {
	primary := runtimeStats{
		Threads: 3,
	}
	fallback := systemdUnitStats{
		Memory:    96,
		Tasks:     5,
		UptimeSec: 360,
	}

	got := preferManagedCoreRuntimeStats(primary, fallback)
	if got.MemoryBytes != 96 {
		t.Fatalf("expected memory fallback to be used, got %d", got.MemoryBytes)
	}
	if got.Threads != 3 {
		t.Fatalf("expected existing threads to be preserved, got %d", got.Threads)
	}
	if got.Uptime != 360 {
		t.Fatalf("expected uptime fallback to be used, got %d", got.Uptime)
	}
}
