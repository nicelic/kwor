package service

import (
	"context"
	"errors"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestLifecycleStateBlocksNewWorkKeepsFailedUninstallBlocked(t *testing.T) {
	if !lifecycleStateBlocksNewWork(KworUninstallLifecycleState{Status: kworUninstallStatusRunning}) {
		t.Fatal("legacy running lifecycle state must block new work")
	}
	if !lifecycleStateBlocksNewWork(KworUninstallLifecycleState{Status: kworUninstallStatusFailed, BlockNewWork: true}) {
		t.Fatal("failed uninstall with recovery state must block new work")
	}
	if lifecycleStateBlocksNewWork(KworUninstallLifecycleState{Status: kworUninstallStatusFailed}) {
		t.Fatal("worker launch failure without recovery state must not block new work")
	}
}

func TestPanelUninstallTakeoverWaitsForWorkerRegistrationGrace(t *testing.T) {
	state := KworUninstallLifecycleState{
		Status:    kworUninstallStatusScheduled,
		UpdatedAt: 100,
	}
	if !lifecycleUninstallTakeoverBlocked(state, time.Unix(129, 0), false) {
		t.Fatal("scheduled reservation must remain protected during worker startup grace")
	}
	if lifecycleUninstallTakeoverBlocked(state, time.Unix(130, 0), false) {
		t.Fatal("stale scheduled reservation without a worker must become recoverable after grace")
	}
	if !lifecycleUninstallTakeoverBlocked(state, time.Unix(200, 0), true) {
		t.Fatal("a live scheduled worker must block takeover after grace")
	}
}

func TestPanelUninstallTakeoverKeepsFailedLiveWorkerBlocked(t *testing.T) {
	state := KworUninstallLifecycleState{
		Status:       kworUninstallStatusFailed,
		BlockNewWork: true,
	}
	if !lifecycleUninstallTakeoverBlocked(state, time.Unix(200, 0), true) {
		t.Fatal("failed blocking state with a live worker must reject rescheduling")
	}
	if lifecycleUninstallTakeoverBlocked(state, time.Unix(200, 0), false) {
		t.Fatal("failed state with no live worker must remain recoverable")
	}
}

func TestPanelUninstallRollbackFailureKeepsKnownWorkerIdentity(t *testing.T) {
	state := &KworUninstallLifecycleState{
		Status:        kworUninstallStatusScheduled,
		ReservationID: "reservation-a",
	}
	updated, err := applyPanelUninstallRollbackFailure(state, "reservation-a", 101, 202, 303, "kwor-panel-uninstall-1", errors.New("rollback failed"))
	if err != nil || !updated {
		t.Fatalf("apply rollback failure: updated=%v err=%v", updated, err)
	}
	if state.Status != kworUninstallStatusFailed || !state.BlockNewWork || state.WorkerPID != 101 || state.WorkerPGID != 202 || state.WorkerStartTime != 303 || state.WorkerUnit != "kwor-panel-uninstall-1" {
		t.Fatalf("rollback failure lost worker identity: %#v", state)
	}
	if !strings.Contains(state.Error, "rollback failed") {
		t.Fatalf("rollback failure error was not preserved: %#v", state)
	}

	advanced := &KworUninstallLifecycleState{
		Status:        kworUninstallStatusRunning,
		ReservationID: "reservation-a",
		WorkerPID:     404,
	}
	updated, err = applyPanelUninstallRollbackFailure(advanced, "reservation-a", 505, 606, 707, "", errors.New("late parent failure"))
	if err != nil || updated || advanced.Status != kworUninstallStatusRunning || advanced.WorkerPID != 404 {
		t.Fatalf("advanced worker state was overwritten: updated=%v state=%#v err=%v", updated, advanced, err)
	}
	if _, err := applyPanelUninstallRollbackFailure(state, "reservation-b", 1, 2, 3, "", nil); err == nil {
		t.Fatal("rollback failure from another reservation must be rejected")
	}
}

func TestManagedOperationBlockingGraceAndStaleCleanup(t *testing.T) {
	operation := KworManagedOperationRecord{
		ID:            "panel-update-1",
		Kind:          "panel-update",
		BlockNewWork:  true,
		BlockingSince: 100,
	}
	if !managedOperationBlockingGraceActive(operation, time.Unix(129, 0)) {
		t.Fatal("new blocking operation must retain its handoff grace")
	}
	if managedOperationBlockingGraceActive(operation, time.Unix(130, 0)) {
		t.Fatal("blocking operation handoff grace must expire")
	}

	previousLoad := kworLifecycleLoadManagedOperationsFn
	previousRemove := kworLifecycleRemoveManagedOperationFn
	previousAlive := kworLifecycleManagedOperationAliveFn
	previousNow := kworLifecycleNowFn
	t.Cleanup(func() {
		kworLifecycleLoadManagedOperationsFn = previousLoad
		kworLifecycleRemoveManagedOperationFn = previousRemove
		kworLifecycleManagedOperationAliveFn = previousAlive
		kworLifecycleNowFn = previousNow
	})

	kworLifecycleLoadManagedOperationsFn = func() ([]KworManagedOperationRecord, error) {
		return []KworManagedOperationRecord{operation}, nil
	}
	kworLifecycleNowFn = func() time.Time { return time.Unix(200, 0) }
	kworLifecycleManagedOperationAliveFn = func(KworManagedOperationRecord) (bool, error) { return false, nil }
	removedID := ""
	kworLifecycleRemoveManagedOperationFn = func(id string) error {
		removedID = id
		return nil
	}
	if err := ensureNoBlockingKworManagedOperations(); err != nil {
		t.Fatalf("clean stale blocking operation: %v", err)
	}
	if removedID != operation.ID {
		t.Fatalf("removed blocking operation = %q, want %q", removedID, operation.ID)
	}

	removedID = ""
	kworLifecycleManagedOperationAliveFn = func(KworManagedOperationRecord) (bool, error) { return true, nil }
	if err := ensureNoBlockingKworManagedOperations(); err == nil || !strings.Contains(err.Error(), "仍在运行") {
		t.Fatalf("live blocking operation error = %v", err)
	}
	if removedID != "" {
		t.Fatalf("live blocking operation was removed: %q", removedID)
	}
}

func TestQuiesceKworManagedOperationsKeepsRecordWhenLocalTaskTimesOut(t *testing.T) {
	previousTerminate := kworLifecycleTerminateManagedOperationFn
	previousLoad := kworLifecycleLoadManagedOperationsFn
	previousRemove := kworLifecycleRemoveManagedOperationFn
	previousTimeout := kworLifecycleQuiesceTimeout

	ctx, cancel := context.WithCancel(context.Background())
	handle := &KworManagedOperationHandle{
		id:     "stalled-operation",
		ctx:    ctx,
		cancel: cancel,
		done:   make(chan struct{}),
	}
	kworLifecycleTaskMu.Lock()
	previousTasks := kworLifecycleTasks
	previousQuiescing := kworLifecycleQuiescing
	kworLifecycleTasks = map[string]*KworManagedOperationHandle{handle.id: handle}
	kworLifecycleQuiescing = false
	kworLifecycleTaskMu.Unlock()

	removed := 0
	terminated := 0
	kworLifecycleTerminateManagedOperationFn = func(operation KworManagedOperationRecord) error {
		if operation.ID != handle.id {
			t.Fatalf("terminated operation = %q", operation.ID)
		}
		terminated++
		return nil
	}
	kworLifecycleLoadManagedOperationsFn = func() ([]KworManagedOperationRecord, error) {
		return []KworManagedOperationRecord{{ID: handle.id, Kind: "test"}}, nil
	}
	kworLifecycleRemoveManagedOperationFn = func(id string) error {
		removed++
		return nil
	}
	kworLifecycleQuiesceTimeout = 20 * time.Millisecond
	t.Cleanup(func() {
		cancel()
		kworLifecycleTerminateManagedOperationFn = previousTerminate
		kworLifecycleLoadManagedOperationsFn = previousLoad
		kworLifecycleRemoveManagedOperationFn = previousRemove
		kworLifecycleQuiesceTimeout = previousTimeout
		kworLifecycleTaskMu.Lock()
		kworLifecycleTasks = previousTasks
		kworLifecycleQuiescing = previousQuiescing
		kworLifecycleTaskMu.Unlock()
	})

	err := QuiesceKworManagedOperations()
	if err == nil || !strings.Contains(err.Error(), "stalled-operation") {
		t.Fatalf("quiesce error = %v, want timed-out operation", err)
	}
	if terminated != 1 {
		t.Fatalf("terminated operations = %d, want 1", terminated)
	}
	if removed != 0 {
		t.Fatalf("timed-out in-memory task must keep its persistent record, remove calls = %d", removed)
	}
	select {
	case <-ctx.Done():
	default:
		t.Fatal("quiesce must cancel the local task context")
	}
}

func TestQuiesceKworManagedOperationsRemovesExitedDetachedRecord(t *testing.T) {
	previousTerminate := kworLifecycleTerminateManagedOperationFn
	previousLoad := kworLifecycleLoadManagedOperationsFn
	previousRemove := kworLifecycleRemoveManagedOperationFn
	kworLifecycleTaskMu.Lock()
	previousTasks := kworLifecycleTasks
	previousQuiescing := kworLifecycleQuiescing
	kworLifecycleTasks = map[string]*KworManagedOperationHandle{}
	kworLifecycleQuiescing = false
	kworLifecycleTaskMu.Unlock()

	removedID := ""
	kworLifecycleTerminateManagedOperationFn = func(KworManagedOperationRecord) error { return nil }
	kworLifecycleLoadManagedOperationsFn = func() ([]KworManagedOperationRecord, error) {
		return []KworManagedOperationRecord{{ID: "detached-operation", Kind: "test"}}, nil
	}
	kworLifecycleRemoveManagedOperationFn = func(id string) error {
		removedID = id
		return nil
	}
	t.Cleanup(func() {
		kworLifecycleTerminateManagedOperationFn = previousTerminate
		kworLifecycleLoadManagedOperationsFn = previousLoad
		kworLifecycleRemoveManagedOperationFn = previousRemove
		kworLifecycleTaskMu.Lock()
		kworLifecycleTasks = previousTasks
		kworLifecycleQuiescing = previousQuiescing
		kworLifecycleTaskMu.Unlock()
	})

	if err := QuiesceKworManagedOperations(); err != nil {
		t.Fatalf("quiesce detached operation: %v", err)
	}
	if removedID != "detached-operation" {
		t.Fatalf("removed detached operation = %q", removedID)
	}
}

func TestManagedOperationRegistrationIsAtomicWithQuiesce(t *testing.T) {
	previousLoad := kworLifecycleLoadManagedOperationsFn
	previousTerminate := kworLifecycleTerminateManagedOperationFn
	previousRemove := kworLifecycleRemoveManagedOperationFn
	previousHook := kworLifecycleBeforeRegisterHook
	previousRuntimeDir := kworLifecycleRuntimeDirFn
	previousManifestPath := hostOwnershipManifestPathFn
	kworLifecycleTaskMu.Lock()
	previousTasks := kworLifecycleTasks
	previousQuiescing := kworLifecycleQuiescing
	kworLifecycleTasks = map[string]*KworManagedOperationHandle{}
	kworLifecycleQuiescing = false
	kworLifecycleTaskMu.Unlock()
	root := t.TempDir()
	kworLifecycleRuntimeDirFn = func() string { return root + "/run" }
	hostOwnershipManifestPathFn = func() string { return root + "/ownership-v1.json" }
	kworLifecycleLoadManagedOperationsFn = func() ([]KworManagedOperationRecord, error) { return nil, nil }
	kworLifecycleTerminateManagedOperationFn = func(KworManagedOperationRecord) error { return nil }
	kworLifecycleRemoveManagedOperationFn = func(string) error { return nil }
	entered := make(chan struct{})
	release := make(chan struct{})
	kworLifecycleBeforeRegisterHook = func() {
		close(entered)
		<-release
	}
	t.Cleanup(func() {
		kworLifecycleLoadManagedOperationsFn = previousLoad
		kworLifecycleTerminateManagedOperationFn = previousTerminate
		kworLifecycleRemoveManagedOperationFn = previousRemove
		kworLifecycleBeforeRegisterHook = previousHook
		kworLifecycleRuntimeDirFn = previousRuntimeDir
		hostOwnershipManifestPathFn = previousManifestPath
		kworLifecycleTaskMu.Lock()
		kworLifecycleTasks = previousTasks
		kworLifecycleQuiescing = previousQuiescing
		kworLifecycleTaskMu.Unlock()
	})

	type beginResult struct {
		handle *KworManagedOperationHandle
		err    error
	}
	beginDone := make(chan beginResult, 1)
	go func() {
		handle, err := BeginKworInProcessOperation("atomic-registration")
		beginDone <- beginResult{handle: handle, err: err}
	}()
	<-entered
	quiesceDone := make(chan error, 1)
	go func() { quiesceDone <- QuiesceKworManagedOperations() }()
	select {
	case err := <-quiesceDone:
		t.Fatalf("quiesce crossed the registration critical section: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(release)
	result := <-beginDone
	if result.err != nil || result.handle == nil {
		t.Fatalf("begin in-process operation: handle=%#v err=%v", result.handle, result.err)
	}
	select {
	case <-result.handle.Context().Done():
	case <-time.After(time.Second):
		t.Fatal("quiesce did not cancel the operation registered at the gate")
	}
	result.handle.Done()
	if err := <-quiesceDone; err != nil {
		t.Fatalf("quiesce atomic operation: %v", err)
	}
}

func TestPrepareKworManagedCommandContextInjectsOperationID(t *testing.T) {
	base, cancel := context.WithCancel(context.Background())
	defer cancel()
	handle := &KworManagedOperationHandle{id: "operation-test-token"}
	ctx := context.WithValue(base, kworManagedOperationContextKey{}, handle)
	cmd := exec.Command("kwor-test-command")
	PrepareKworManagedCommandContext(ctx, cmd)
	want := kworLifecycleOperationIDEnv + "=" + handle.id
	found := false
	for _, entry := range cmd.Env {
		if entry == want {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("managed command environment does not contain %q", want)
	}
}

func TestPanelUninstallWorkerRecordDoesNotDowngradeAdvancedState(t *testing.T) {
	state := &KworUninstallLifecycleState{
		Status:        kworUninstallStatusRunning,
		Phase:         "cleanup",
		ReservationID: "reservation-a",
		WorkerPID:     42,
	}
	updated, err := applyPanelUninstallWorkerRecord(state, "reservation-a", 99, 99, 123, "kwor-panel-uninstall-1")
	if err != nil || updated {
		t.Fatalf("advanced state update = %v, %v; want ignored", updated, err)
	}
	if state.Status != kworUninstallStatusRunning || state.WorkerPID != 42 || state.Phase != "cleanup" {
		t.Fatalf("advanced worker state was downgraded: %#v", state)
	}
	if _, err := applyPanelUninstallWorkerRecord(state, "other-reservation", 99, 99, 123, ""); err == nil {
		t.Fatal("mismatched reservation must be rejected")
	}
}
