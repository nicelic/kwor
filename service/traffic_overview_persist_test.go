package service

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/alireza0/s-ui/database"
)

func TestTrafficOverviewSettingsPersistAcrossServiceInstances(t *testing.T) {
	initTrafficOverviewTestDB(t)

	svc := &TrafficOverviewService{}
	if err := svc.UpdateTrafficOverviewSettings(128.64, 15); err != nil {
		t.Fatalf("update settings failed: %v", err)
	}

	limitGiB, resetDay, _, err := svc.getOverviewConfig()
	if err != nil {
		t.Fatalf("read settings from same service failed: %v", err)
	}
	if limitGiB != 128.64 {
		t.Fatalf("limitGiB = %.2f, want 128.64", limitGiB)
	}
	if resetDay != 15 {
		t.Fatalf("resetDay = %d, want 15", resetDay)
	}

	other := &TrafficOverviewService{}
	otherLimit, otherResetDay, _, otherErr := other.getOverviewConfig()
	if otherErr != nil {
		t.Fatalf("read settings from another service failed: %v", otherErr)
	}
	if otherLimit != 128.64 || otherResetDay != 15 {
		t.Fatalf("persisted settings mismatch: got limit %.2f day %d", otherLimit, otherResetDay)
	}
}

func TestSetTrafficOverviewEnabledFalsePersistsPauseSnapshot(t *testing.T) {
	initTrafficOverviewTestDB(t)
	resetTrafficOverviewSnapshotCacheForTest()

	svc := &TrafficOverviewService{}
	snapshot := trafficOverviewSnapshot{
		Source:     "vnstat",
		Interface:  "eth0",
		Available:  true,
		Up:         123,
		Down:       456,
		Total:      579,
		AccumUp:    123,
		AccumDown:  456,
		AccumTotal: 579,
		UpdatedAt:  2000,
	}
	if err := svc.stageOverviewSnapshot(snapshot, true); err != nil {
		t.Fatalf("seed snapshot failed: %v", err)
	}

	if err := svc.SetTrafficOverviewEnabled(false); err != nil {
		t.Fatalf("disable traffic overview failed: %v", err)
	}

	_, _, enabled, err := svc.getOverviewConfig()
	if err != nil {
		t.Fatalf("read overview config failed: %v", err)
	}
	if enabled {
		t.Fatalf("expected overview enabled=false after disabling")
	}

	pauseState, ok := svc.loadPauseState()
	if !ok || !pauseState.Paused {
		t.Fatalf("expected pause state to be persisted: ok=%v state=%+v", ok, pauseState)
	}
	if pauseState.Snapshot.Total != snapshot.Total || pauseState.Snapshot.AccumTotal != snapshot.AccumTotal {
		t.Fatalf("pause snapshot mismatch: got=%+v want=%+v", pauseState.Snapshot, snapshot)
	}

	overview, err := svc.GetTrafficOverview()
	if err != nil {
		t.Fatalf("GetTrafficOverview failed: %v", err)
	}
	if overview.Enabled {
		t.Fatalf("overview should report disabled")
	}
	if overview.Status != "stopped" || overview.Available {
		t.Fatalf("overview should be stopped and unavailable: %+v", overview)
	}
	if overview.Total != snapshot.Total || overview.AccumTotal != snapshot.AccumTotal {
		t.Fatalf("disabled overview should show frozen snapshot: got total=%d accum=%d", overview.Total, overview.AccumTotal)
	}
}

func TestClearVnstatManagedStateClearsPauseState(t *testing.T) {
	initTrafficOverviewTestDB(t)

	svc := &TrafficOverviewService{}
	if err := svc.savePauseState(trafficOverviewPauseState{
		Paused:   true,
		Snapshot: trafficOverviewSnapshot{Source: "vnstat", Total: 100, AccumTotal: 100},
		PausedAt: 123,
	}); err != nil {
		t.Fatalf("seed pause state failed: %v", err)
	}

	if err := svc.clearVnstatManagedState(); err != nil {
		t.Fatalf("clear managed state failed: %v", err)
	}

	if pauseState, ok := svc.loadPauseState(); ok || pauseState.Paused {
		t.Fatalf("expected pause state to be cleared, got ok=%v state=%+v", ok, pauseState)
	}
}

func TestTrafficOverviewSnapshotFlushPolicy(t *testing.T) {
	initTrafficOverviewTestDB(t)
	resetTrafficOverviewSnapshotCacheForTest()

	svc := &TrafficOverviewService{}
	first := trafficOverviewSnapshot{
		Source:     "vnstat",
		Interface:  "eth0",
		Available:  true,
		Up:         100,
		Down:       200,
		Total:      300,
		AccumUp:    100,
		AccumDown:  200,
		AccumTotal: 300,
		UpdatedAt:  1000,
	}
	if err := svc.stageOverviewSnapshot(first, false); err != nil {
		t.Fatalf("stage first snapshot failed: %v", err)
	}
	if got, ok := loadPersistedSnapshotFromDB(t); !ok || got.Total != first.Total || got.AccumTotal != first.AccumTotal {
		t.Fatalf("first snapshot was not flushed as expected")
	}

	second := first
	second.Up = 101
	second.Total = 301
	second.AccumUp = 101
	second.AccumTotal = 301
	second.UpdatedAt = 1001
	if err := svc.stageOverviewSnapshot(second, false); err != nil {
		t.Fatalf("stage second snapshot failed: %v", err)
	}

	stillPersisted, ok := loadPersistedSnapshotFromDB(t)
	if !ok {
		t.Fatalf("expected persisted snapshot to exist")
	}
	if stillPersisted.Total != first.Total {
		t.Fatalf("second snapshot should be pending only before forced flush")
	}

	if err := svc.FlushPendingSnapshot(); err != nil {
		t.Fatalf("force flush pending snapshot failed: %v", err)
	}
	flushed, ok := loadPersistedSnapshotFromDB(t)
	if !ok || flushed.Total != second.Total || flushed.AccumTotal != second.AccumTotal {
		t.Fatalf("pending snapshot was not flushed")
	}
}

func TestCleanupTrafficCapOnShutdownMarksStateInactiveButPreservesLimitReached(t *testing.T) {
	initTrafficOverviewTestDB(t)

	originalShutdownEnabled := trafficOverviewShutdownEnabledFn
	defer func() {
		trafficOverviewShutdownEnabledFn = originalShutdownEnabled
	}()
	trafficOverviewShutdownEnabledFn = func() bool { return true }

	initial := trafficOverviewCapState{
		Active:       true,
		LimitReached: true,
		AllowedPorts: []int{22, 443},
		UpdatedAt:    100,
	}
	trafficOverviewCapMu.Lock()
	if err := (&TrafficOverviewService{}).saveCapStateLocked(initial); err != nil {
		trafficOverviewCapMu.Unlock()
		t.Fatalf("seed cap state failed: %v", err)
	}
	trafficOverviewCapMu.Unlock()

	before := time.Now().Unix()
	if err := (&TrafficOverviewService{}).CleanupTrafficCapOnShutdown(); err != nil {
		t.Fatalf("CleanupTrafficCapOnShutdown failed: %v", err)
	}

	trafficOverviewCapMu.Lock()
	state, err := (&TrafficOverviewService{}).loadCapStateLocked()
	trafficOverviewCapMu.Unlock()
	if err != nil {
		t.Fatalf("load cap state failed: %v", err)
	}
	if state.Active {
		t.Fatalf("expected cap state to be inactive after shutdown cleanup: %+v", state)
	}
	if !state.LimitReached {
		t.Fatalf("expected limitReached to stay true for startup restore fallback: %+v", state)
	}
	if state.UpdatedAt < before {
		t.Fatalf("expected updatedAt to advance, got %d < %d", state.UpdatedAt, before)
	}
	if len(state.AllowedPorts) != len(initial.AllowedPorts) {
		t.Fatalf("allowed ports should be preserved: got=%v want=%v", state.AllowedPorts, initial.AllowedPorts)
	}
}

func initTrafficOverviewTestDB(t *testing.T) {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "traffic-overview.db")
	if err := database.InitDB(dbPath); err != nil {
		t.Fatalf("init db failed: %v", err)
	}
	if sqlDB, err := database.GetDB().DB(); err == nil && sqlDB != nil {
		t.Cleanup(func() {
			_ = sqlDB.Close()
		})
	}

	settingSvc := &SettingService{}
	if _, err := settingSvc.GetAllSetting(); err != nil {
		t.Fatalf("seed default settings failed: %v", err)
	}
}

func loadPersistedSnapshotFromDB(t *testing.T) (trafficOverviewSnapshot, bool) {
	t.Helper()

	raw, err := (&SettingService{}).getString(trafficOverviewSnapshotKey)
	if err != nil {
		t.Fatalf("load traffic overview snapshot setting failed: %v", err)
	}
	if raw == "" || raw == "{}" {
		return trafficOverviewSnapshot{}, false
	}
	var snapshot trafficOverviewSnapshot
	if err := json.Unmarshal([]byte(raw), &snapshot); err != nil {
		t.Fatalf("unmarshal traffic overview snapshot failed: %v", err)
	}
	return snapshot, true
}

func resetTrafficOverviewSnapshotCacheForTest() {
	trafficOverviewSnapshotMu.Lock()
	trafficOverviewSnapshotCache = trafficOverviewSnapshotState{}
	trafficOverviewSnapshotMu.Unlock()
}
