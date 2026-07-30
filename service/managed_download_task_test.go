package service

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestManagedDownloadTaskManagerRepeatedStartAndBusyRequest(t *testing.T) {
	manager := NewManagedDownloadTaskManager("test download")
	first, firstStatus, created, err := manager.Start("test-download", "same")
	if err != nil || !created || first == nil {
		t.Fatalf("expected initial task, created=%v handle=%v err=%v", created, first, err)
	}
	if firstStatus.State != managedDownloadTaskQueued || firstStatus.ID == "" {
		t.Fatalf("unexpected initial status: %#v", firstStatus)
	}

	second, secondStatus, created, err := manager.Start("test-download", "same")
	if err != nil || created || second == nil {
		t.Fatalf("expected idempotent task result, created=%v handle=%v err=%v", created, second, err)
	}
	if second.ID() != first.ID() || secondStatus.ID != first.ID() {
		t.Fatalf("expected repeated request to return task %q, got handle=%q status=%q", first.ID(), second.ID(), secondStatus.ID)
	}

	_, _, _, err = manager.Start("test-download", "different")
	if err == nil {
		t.Fatal("expected different active request to be rejected")
	}

	first.FinishSuccess("completed")
}

func TestManagedDownloadTaskManagerStopAndWorkerCleanup(t *testing.T) {
	manager := NewManagedDownloadTaskManager("test download")
	handle, _, created, err := manager.Start("test-download", "one")
	if err != nil || !created {
		t.Fatalf("start task: created=%v err=%v", created, err)
	}
	if !handle.MarkRunning("downloading") {
		t.Fatal("expected running transition")
	}

	stopping, err := manager.Stop(handle.ID())
	if err != nil {
		t.Fatalf("stop task: %v", err)
	}
	if stopping.State != managedDownloadTaskStopping || !stopping.StopRequested || stopping.CanCancel {
		t.Fatalf("unexpected stopping state: %#v", stopping)
	}
	select {
	case <-handle.Context().Done():
	case <-time.After(time.Second):
		t.Fatal("task context was not cancelled")
	}

	handle.FinishCancelled("cancelled")
	finished := manager.Get(handle.ID())
	if finished.State != managedDownloadTaskCancelled || finished.FinishedAt == 0 {
		t.Fatalf("unexpected finished state: %#v", finished)
	}
	if manager.IsActive() {
		t.Fatal("terminal task must release active slot")
	}
}

func TestManagedDownloadTaskManagerLifecycleCancellationFinishesCancelled(t *testing.T) {
	manager := NewManagedDownloadTaskManager("test download")
	handle, _, created, err := manager.Start("test-download", "one")
	if err != nil || !created {
		t.Fatalf("start task: created=%v err=%v", created, err)
	}
	operation := handle.Operation()
	if operation == nil {
		t.Fatal("expected managed operation")
	}

	operation.Done()
	handle.FinishCancelled("cancelled")
	finished := manager.Get(handle.ID())
	if finished.State != managedDownloadTaskCancelled {
		t.Fatalf("lifecycle cancellation should remain cancelled, got %#v", finished)
	}
}

func TestManagedDownloadTaskManagerDoesNotStopIrreversibleTask(t *testing.T) {
	manager := NewManagedDownloadTaskManager("test download")
	handle, _, created, err := manager.Start("test-download", "one")
	if err != nil || !created {
		t.Fatalf("start task: created=%v err=%v", created, err)
	}
	if !handle.BeginApplying("applying") {
		t.Fatal("expected irreversible transition")
	}

	status, err := manager.Stop(handle.ID())
	if err == nil {
		t.Fatal("expected stop rejection after applying starts")
	}
	if status.CanCancel || status.State != managedDownloadTaskRunning || status.Phase != "applying" {
		t.Fatalf("unexpected irreversible status: %#v", status)
	}
	if handle.Context().Err() != nil {
		t.Fatalf("irreversible task context was unexpectedly cancelled: %v", handle.Context().Err())
	}

	handle.FinishSuccess("completed")
}

func TestManagedDownloadTaskManagerTimeoutStopsCancelableTask(t *testing.T) {
	manager := NewManagedDownloadTaskManager("test download")
	handle, _, created, err := manager.Start("test-download", "one")
	if err != nil || !created {
		t.Fatalf("start task: created=%v err=%v", created, err)
	}
	if !handle.MarkRunning("downloading") {
		t.Fatal("expected running transition")
	}

	stopping, stopped := manager.requestStop(handle.ID(), true)
	if !stopped || stopping.State != managedDownloadTaskStopping || !stopping.DeadlineExceeded {
		t.Fatalf("unexpected timeout stop status: %#v stopped=%v", stopping, stopped)
	}
	select {
	case <-handle.Context().Done():
	case <-time.After(time.Second):
		t.Fatal("timeout did not cancel the task context")
	}

	handle.FinishCancelled("cancelled")
	finished := manager.Get(handle.ID())
	if finished.State != managedDownloadTaskTimedOut || !finished.DeadlineExceeded {
		t.Fatalf("expected timed out terminal state, got %#v", finished)
	}
}

func TestManagedDownloadTaskPanicFinishesTask(t *testing.T) {
	manager := NewManagedDownloadTaskManager("test download")
	handle, _, created, err := manager.Start("test-download", "one")
	if err != nil || !created {
		t.Fatalf("start task: created=%v err=%v", created, err)
	}

	func() {
		defer finishManagedDownloadTaskPanic(handle, "failed", "test download")
		panic("test worker panic")
	}()

	status := manager.Get(handle.ID())
	if status.State != managedDownloadTaskError || !strings.Contains(status.Error, "test worker panic") {
		t.Fatalf("panic should be surfaced as task error, got %#v", status)
	}
	if manager.IsActive() {
		t.Fatal("panic recovery must release the active task slot")
	}
}

func TestManagedDownloadTaskManagerDetachedOperationStillReleasesTaskSlot(t *testing.T) {
	manager := NewManagedDownloadTaskManager("test download")
	handle, _, created, err := manager.Start("test-download", "one")
	if err != nil || !created {
		t.Fatalf("start task: created=%v err=%v", created, err)
	}
	operation := handle.DetachOperation()
	if operation == nil {
		t.Fatal("expected detachable operation")
	}
	t.Cleanup(operation.Done)

	handle.FinishSuccess("handoff")
	if manager.IsActive() {
		t.Fatal("detached handoff must not keep the task slot active")
	}
	status := manager.Get(handle.ID())
	if status.State != managedDownloadTaskSuccess || status.Phase != "handoff" {
		t.Fatalf("unexpected detached handoff status: %#v", status)
	}
}

func TestManagedDownloadTaskManagerExpiresTerminalSnapshot(t *testing.T) {
	manager := NewManagedDownloadTaskManager("test download")
	handle, _, created, err := manager.Start("test-download", "one")
	if err != nil || !created {
		t.Fatalf("start task: created=%v err=%v", created, err)
	}
	handle.FinishError("failed", context.Canceled)

	manager.mu.Lock()
	task := manager.tasks[handle.ID()]
	if task == nil {
		manager.mu.Unlock()
		t.Fatal("expected retained terminal task")
	}
	task.status.FinishedAt = time.Now().Add(-managedDownloadTaskTTL - time.Second).Unix()
	manager.mu.Unlock()

	status := manager.Get("")
	if status.State != managedDownloadTaskIdle {
		t.Fatalf("expected expired snapshot to be removed, got %#v", status)
	}
}
