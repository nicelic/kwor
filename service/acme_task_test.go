package service

import (
	"errors"
	"strings"
	"testing"
	"time"
	"unicode/utf8"
)

func TestAcmeTaskStoreQueuesCompletesAndFails(t *testing.T) {
	store := newAcmeTaskStore()
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})

	first, err := store.enqueue("issue", "first", func(string) (*AcmeActionResult, error) {
		close(firstStarted)
		<-releaseFirst
		return &AcmeActionResult{Msg: "done"}, nil
	})
	if err != nil {
		t.Fatalf("enqueue first task failed: %v", err)
	}
	<-firstStarted

	second, err := store.enqueue("renew", "second", func(string) (*AcmeActionResult, error) {
		return nil, errors.New("expected task failure")
	})
	if err != nil {
		t.Fatalf("enqueue second task failed: %v", err)
	}
	if second.Status != acmeTaskStatusQueued {
		t.Fatalf("second task should stay queued, got %q", second.Status)
	}
	active := store.listActive()
	if len(active) < 2 {
		t.Fatalf("expected both tasks to be active, got %#v", active)
	}

	close(releaseFirst)
	firstDone := waitForAcmeTask(t, store, first.ID, acmeTaskStatusSuccess)
	if firstDone.Result == nil || firstDone.Result.Msg != "done" {
		t.Fatalf("unexpected completed result: %#v", firstDone)
	}
	secondDone := waitForAcmeTask(t, store, second.ID, acmeTaskStatusError)
	if secondDone.Error != "expected task failure" {
		t.Fatalf("unexpected failed task: %#v", secondDone)
	}
}

func TestAcmeTaskStoreFinishesCancelledQueuedManagedInstall(t *testing.T) {
	store := newAcmeTaskStore()
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	first, err := store.enqueue("issue", "blocking task", func(string) (*AcmeActionResult, error) {
		close(firstStarted)
		<-releaseFirst
		return nil, nil
	})
	if err != nil {
		t.Fatalf("enqueue blocking task: %v", err)
	}
	select {
	case <-firstStarted:
	case <-time.After(time.Second):
		t.Fatal("blocking task did not start")
	}

	manager := NewManagedDownloadTaskManager("test acme install")
	handle, _, created, err := manager.Start("acme-install", "test")
	if err != nil || !created {
		t.Fatalf("create managed install task: handle=%v created=%v err=%v", handle, created, err)
	}
	runnerStarted := make(chan struct{})
	if _, err := store.enqueueManaged("install", "queued install", handle, func(string) (*AcmeActionResult, error) {
		close(runnerStarted)
		return nil, nil
	}); err != nil {
		t.Fatalf("enqueue managed install: %v", err)
	}

	if _, err := manager.Stop(handle.ID()); err != nil {
		t.Fatalf("stop queued managed install: %v", err)
	}
	store.finishQueuedManagedIfCancelled(handle.ID())

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if status := manager.Get(handle.ID()); status.State == managedDownloadTaskCancelled {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if status := manager.Get(handle.ID()); status.State != managedDownloadTaskCancelled {
		t.Fatalf("queued task should be cancelled without waiting for queue, got %#v", status)
	}
	if view := store.get(handle.ID()); view == nil || view.Status != acmeTaskStatusError {
		t.Fatalf("queued task view should be finalized, got %#v", view)
	}
	deadline = time.Now().Add(time.Second)
	for len(store.queue) > 0 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if len(store.queue) != 0 {
		t.Fatal("cancelled queued managed task should release its queue slot immediately")
	}
	select {
	case <-runnerStarted:
		t.Fatal("cancelled queued runner must not start")
	default:
	}

	close(releaseFirst)
	_ = waitForAcmeTask(t, store, first.ID, acmeTaskStatusSuccess)
	select {
	case <-runnerStarted:
		t.Fatal("released worker must not execute a task removed from the queue")
	default:
	}
}

func TestCloneAcmeActionResultKeepsUTF8SafeOutputSummary(t *testing.T) {
	source := &AcmeActionResult{Output: strings.Repeat("签发输出", acmeTaskResultOutputMaxRunes)}
	clone := cloneAcmeActionResult(source)
	if clone == nil {
		t.Fatal("expected cloned result")
	}
	if !utf8.ValidString(clone.Output) {
		t.Fatalf("task output summary is not valid UTF-8: %q", clone.Output)
	}
	if len([]rune(clone.Output)) >= len([]rune(source.Output)) {
		t.Fatal("expected long task output to be summarized")
	}
	if !strings.Contains(clone.Output, "任务结果摘要已截断") {
		t.Fatalf("expected truncation marker: %q", clone.Output)
	}
}

func TestAcmeLogSessionEnsureManagedOperationCreatesSyncHandle(t *testing.T) {
	previousStore := acmeLogSessionStore
	acmeLogSessionStore = newAcmeLogStore()
	t.Cleanup(func() { acmeLogSessionStore = previousStore })

	session := acmeLogSessionStore.start("sync-acme", "同步签发")
	finish, err := session.ensureManagedOperation("issue")
	if err != nil {
		t.Fatalf("ensure managed operation: %v", err)
	}
	if finish == nil {
		t.Fatal("expected cleanup function")
	}
	if session.operation == nil || session.ctx == nil {
		t.Fatalf("sync ACME session was not attached to a managed operation: %#v", session)
	}
	select {
	case <-session.ctx.Done():
		t.Fatal("operation context should stay active before cleanup")
	default:
	}
	finish()
	select {
	case <-session.ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("cleanup should cancel sync ACME operation context")
	}
}

func TestAcmeLogSessionEnsureManagedOperationReusesQueuedHandle(t *testing.T) {
	previousStore := acmeLogSessionStore
	acmeLogSessionStore = newAcmeLogStore()
	t.Cleanup(func() { acmeLogSessionStore = previousStore })

	ctx, handle, err := BeginKworManagedOperation("acme-existing")
	if err != nil {
		t.Fatalf("create existing managed operation: %v", err)
	}
	defer handle.Done()
	queued := acmeLogSessionStore.queue("queued-acme", "队列签发", "task-id", ctx, handle)
	finish, err := queued.ensureManagedOperation("issue")
	if err != nil {
		t.Fatalf("ensure queued managed operation: %v", err)
	}
	if queued.operation != handle {
		t.Fatal("queued ACME session should reuse its existing managed operation")
	}
	finish()
	select {
	case <-ctx.Done():
		t.Fatal("no-op cleanup must not finish the queued operation")
	default:
	}
}

func waitForAcmeTask(t *testing.T, store *acmeTaskStore, id string, expectedStatus string) *AcmeTaskView {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		entry := store.get(id)
		if entry != nil && entry.Status == expectedStatus {
			return entry
		}
		time.Sleep(10 * time.Millisecond)
	}
	entry := store.get(id)
	t.Fatalf("task %s did not reach %s: %#v", id, expectedStatus, entry)
	return nil
}
