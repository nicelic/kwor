package service

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
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

func TestAcmeTaskExecutionDeadlineStartsOnlyWhenWorkerBegins(t *testing.T) {
	previousLogStore := acmeLogSessionStore
	acmeLogSessionStore = newAcmeLogStore()
	t.Cleanup(func() { acmeLogSessionStore = previousLogStore })

	store := newAcmeTaskStore()
	firstStarted := make(chan struct{})
	releaseFirst := make(chan struct{})
	first, err := store.enqueue("issue", "blocking task", func(string) (*AcmeActionResult, error) {
		close(firstStarted)
		<-releaseFirst
		return nil, nil
	})
	if err != nil {
		t.Fatalf("enqueue first task: %v", err)
	}
	select {
	case <-firstStarted:
	case <-time.After(time.Second):
		t.Fatal("blocking task did not start")
	}

	secondStarted := make(chan struct{})
	releaseSecond := make(chan struct{})
	second, err := store.enqueue("renew", "queued task", func(string) (*AcmeActionResult, error) {
		close(secondStarted)
		<-releaseSecond
		return nil, nil
	})
	if err != nil {
		t.Fatalf("enqueue second task: %v", err)
	}

	store.mu.Lock()
	queued := store.tasks[second.ID]
	if queued == nil || queued.ctx == nil {
		store.mu.Unlock()
		t.Fatalf("queued task runtime is missing: %#v", queued)
	}
	if _, hasDeadline := queued.ctx.Deadline(); hasDeadline {
		store.mu.Unlock()
		t.Fatal("queued ACME task must not start its execution deadline")
	}
	if queued.cancel != nil {
		store.mu.Unlock()
		t.Fatal("queued ACME task must not have an execution cancel function")
	}
	store.mu.Unlock()

	close(releaseFirst)
	_ = waitForAcmeTask(t, store, first.ID, acmeTaskStatusSuccess)
	select {
	case <-secondStarted:
	case <-time.After(time.Second):
		t.Fatal("queued task did not begin after first task completed")
	}

	executionCtx, _, _ := store.getOperation(second.ID)
	if executionCtx == nil {
		t.Fatal("running task execution context is missing")
	}
	deadline, hasDeadline := executionCtx.Deadline()
	if !hasDeadline {
		t.Fatal("running ACME task must have an execution deadline")
	}
	if remaining := time.Until(deadline); remaining <= 0 || remaining > acmeTaskDeadline {
		t.Fatalf("unexpected execution deadline remaining=%s", remaining)
	}

	acmeLogSessionStore.mu.Lock()
	logSession := acmeLogSessionStore.sessions[second.LogSessionID]
	logCtx := context.Background()
	if logSession != nil {
		logCtx = logSession.operationContext()
	}
	acmeLogSessionStore.mu.Unlock()
	if logSession == nil || logCtx != executionCtx {
		t.Fatalf("task and log session must share the execution context: task=%p log=%p", executionCtx, logCtx)
	}

	close(releaseSecond)
	_ = waitForAcmeTask(t, store, second.ID, acmeTaskStatusSuccess)
}

func TestAcmeTaskManagedExecutionKeepsManagedDeadline(t *testing.T) {
	previousLogStore := acmeLogSessionStore
	acmeLogSessionStore = newAcmeLogStore()
	t.Cleanup(func() { acmeLogSessionStore = previousLogStore })

	parent, cancel := context.WithCancel(context.Background())
	defer cancel()
	store := &acmeTaskStore{tasks: map[string]*acmeTask{
		"managed-install": {
			ctx:     parent,
			managed: &ManagedDownloadTaskHandle{},
			view: AcmeTaskView{
				ID:           "managed-install",
				Status:       acmeTaskStatusRunning,
				LogSessionID: "log-managed-install",
			},
		},
	}}
	acmeLogSessionStore.queue("log-managed-install", "下载 / 重装", "managed-install", parent, nil)

	executionCtx, cancelExecution, started := store.beginExecution("managed-install", "log-managed-install")
	if !started || executionCtx != parent {
		t.Fatalf("managed task must keep its existing context: started=%v task=%p parent=%p", started, executionCtx, parent)
	}
	if _, hasDeadline := executionCtx.Deadline(); hasDeadline {
		t.Fatal("managed install task must not receive the certificate issuance deadline")
	}
	cancelExecution()
	select {
	case <-parent.Done():
		t.Fatal("managed task no-op execution cancellation must not cancel its own context")
	default:
	}

	acmeLogSessionStore.mu.Lock()
	logSession := acmeLogSessionStore.sessions["log-managed-install"]
	logCtx := context.Background()
	if logSession != nil {
		logCtx = logSession.operationContext()
	}
	acmeLogSessionStore.mu.Unlock()
	if logSession == nil || logCtx != parent {
		t.Fatalf("managed task log must retain the managed context: log=%p parent=%p", logCtx, parent)
	}
}

func TestAcmeTaskStoreUses4096QueueCapacity(t *testing.T) {
	if acmeTaskQueueCapacity != 4096 {
		t.Fatalf("ACME task queue capacity = %d, want 4096", acmeTaskQueueCapacity)
	}

	store := &acmeTaskStore{queue: make(chan acmeTaskJob, acmeTaskQueueCapacity)}
	for index := 0; index < acmeTaskQueueCapacity; index++ {
		if !store.enqueueJob(acmeTaskJob{id: fmt.Sprintf("task-%d", index)}) {
			t.Fatalf("enqueue failed before queue reached capacity at index %d", index)
		}
	}
	if store.enqueueJob(acmeTaskJob{id: "overflow"}) {
		t.Fatal("enqueue unexpectedly succeeded after the 4096th queued task")
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

func TestBoundedAcmeOutputAndLogKeepUTF8MemoryBounds(t *testing.T) {
	output := &boundedAcmeOutput{}
	for index := 0; index < 300; index++ {
		output.AppendLine(strings.Repeat("签发输出", 1024))
	}
	outputText := output.String()
	if !utf8.ValidString(outputText) {
		t.Fatalf("bounded command output is not valid UTF-8: %q", outputText)
	}
	if len(outputText) > acmeCommandOutputMaxBytes+128 {
		t.Fatalf("bounded command output exceeded limit: %d", len(outputText))
	}

	previousStore := acmeLogSessionStore
	acmeLogSessionStore = newAcmeLogStore()
	t.Cleanup(func() { acmeLogSessionStore = previousStore })

	session := acmeLogSessionStore.start("bounded-log", "受限日志")
	for index := 0; index < acmeLogMaxLines+100; index++ {
		session.append(strings.Repeat("日志行", 2048))
	}
	view := acmeLogSessionStore.get("bounded-log")
	if view == nil || len(view.Lines) == 0 {
		t.Fatalf("expected bounded log session: %#v", view)
	}
	if len(view.Lines) > acmeLogMaxLines {
		t.Fatalf("log line limit was not enforced: %d", len(view.Lines))
	}
	for _, line := range view.Lines {
		if !utf8.ValidString(line) {
			t.Fatalf("bounded log line is not valid UTF-8: %q", line)
		}
	}

	first := acmeLogSessionStore.getAfter("bounded-log", 0)
	if first == nil || first.LineNext != len(view.Lines) {
		t.Fatalf("unexpected incremental log cursor: %#v", first)
	}
	second := acmeLogSessionStore.getAfter("bounded-log", first.LineNext)
	if second == nil || len(second.Lines) != 0 {
		t.Fatalf("expected no duplicate log lines after cursor: %#v", second)
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

func TestAcmeCommandEnvironmentRetainsManagedOperationID(t *testing.T) {
	ctx, handle, err := BeginKworManagedOperation("acme-command-env")
	if err != nil {
		t.Fatalf("create managed ACME operation: %v", err)
	}
	defer handle.Done()

	cmd := exec.Command("acme-test-command")
	cmd.Env = buildAcmeCommandEnv([]string{"CF_Token=masked-for-test"})
	PrepareKworManagedCommandContext(ctx, cmd)
	want := kworLifecycleOperationIDEnv + "=" + handle.ID()
	for _, entry := range cmd.Env {
		if entry == want {
			return
		}
	}
	t.Fatalf("ACME command environment does not retain managed operation ID %q", want)
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
