package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	acmeTaskStatusQueued  = "queued"
	acmeTaskStatusRunning = "running"
	acmeTaskStatusSuccess = "success"
	acmeTaskStatusWarning = "warning"
	acmeTaskStatusError   = "error"
)

type acmeTask struct {
	view      AcmeTaskView
	ctx       context.Context
	operation *KworManagedOperationHandle
	managed   *ManagedDownloadTaskHandle
}

type acmeTaskJob struct {
	id  string
	run func(logSessionID string) (*AcmeActionResult, error)
}

// acmeTaskStore is intentionally process-local. acme.sh commands share a
// global operation lock already, and a small serial queue makes that waiting
// visible to the UI instead of leaving browser requests open for minutes.
type acmeTaskStore struct {
	mu      sync.Mutex
	queueMu sync.Mutex
	tasks   map[string]*acmeTask
	queue   chan acmeTaskJob
}

var acmeTaskSessionStore = newAcmeTaskStore()

func newAcmeTaskStore() *acmeTaskStore {
	store := &acmeTaskStore{
		tasks: make(map[string]*acmeTask),
		queue: make(chan acmeTaskJob, acmeTaskQueueCapacity),
	}
	go store.run()
	return store
}

func (s *acmeTaskStore) enqueue(operation string, title string, run func(logSessionID string) (*AcmeActionResult, error)) (*AcmeTaskView, error) {
	if run == nil {
		return nil, fmt.Errorf("acme task runner is required")
	}
	operation = strings.TrimSpace(operation)
	if operation == "" {
		return nil, fmt.Errorf("acme task operation is required")
	}
	operationContext, operationHandle, err := BeginKworManagedOperation("acme-" + operation)
	if err != nil {
		return nil, err
	}

	now := time.Now().Unix()
	taskID := newAcmeTaskID()
	logSessionID := "log-" + taskID
	entry := &acmeTask{ctx: operationContext, operation: operationHandle, view: AcmeTaskView{
		ID:           taskID,
		Operation:    operation,
		Status:       acmeTaskStatusQueued,
		LogSessionID: logSessionID,
		StartedAt:    now,
		UpdatedAt:    now,
	}}

	s.mu.Lock()
	s.pruneLocked(now)
	s.tasks[taskID] = entry
	s.mu.Unlock()
	acmeLogSessionStore.queue(logSessionID, title, taskID, operationContext, operationHandle)

	job := acmeTaskJob{id: taskID, run: run}
	if s.enqueueJob(job) {
		return cloneAcmeTaskView(&entry.view), nil
	}
	s.finish(taskID, nil, fmt.Errorf("ACME 后台任务队列已满，请稍后重试"))
	operationHandle.Done()
	return nil, fmt.Errorf("ACME 后台任务队列已满，请稍后重试")
}

// enqueueManaged keeps install/reinstall work behind the same ACME serial
// queue as certificate actions while the managed download task remains the
// sole owner of cancellation, deadline handling and operation cleanup.
func (s *acmeTaskStore) enqueueManaged(operation string, title string, handle *ManagedDownloadTaskHandle, run func(logSessionID string) (*AcmeActionResult, error)) (string, error) {
	if handle == nil || strings.TrimSpace(handle.ID()) == "" {
		return "", fmt.Errorf("managed ACME task handle is required")
	}
	if run == nil {
		return "", fmt.Errorf("acme task runner is required")
	}
	operation = strings.TrimSpace(operation)
	if operation == "" {
		return "", fmt.Errorf("acme task operation is required")
	}

	now := time.Now().Unix()
	taskID := handle.ID()
	logSessionID := "log-" + taskID
	entry := &acmeTask{
		ctx:     handle.Context(),
		managed: handle,
		view: AcmeTaskView{
			ID:           taskID,
			Operation:    operation,
			Status:       acmeTaskStatusQueued,
			LogSessionID: logSessionID,
			StartedAt:    now,
			UpdatedAt:    now,
		},
	}

	s.mu.Lock()
	s.pruneLocked(now)
	s.tasks[taskID] = entry
	s.mu.Unlock()
	acmeLogSessionStore.queue(logSessionID, title, taskID, entry.ctx, handle.Operation())

	job := acmeTaskJob{id: taskID, run: run}
	if s.enqueueJob(job) {
		go s.finishQueuedManagedWhenCancelled(taskID)
		return logSessionID, nil
	}
	queueErr := fmt.Errorf("ACME 后台任务队列已满，请稍后重试")
	handle.FinishError("failed", queueErr)
	s.finishManaged(taskID, nil, queueErr, handle.Snapshot())
	return "", queueErr
}

func (s *acmeTaskStore) enqueueJob(job acmeTaskJob) bool {
	if s == nil {
		return false
	}
	s.queueMu.Lock()
	defer s.queueMu.Unlock()
	select {
	case s.queue <- job:
		return true
	default:
		return false
	}
}

// removeQueuedJob returns a cancelled managed install's queue slot immediately.
// A worker may already have received the job; in that race the cancelled task
// context makes the worker exit without running the installer.
func (s *acmeTaskStore) removeQueuedJob(id string) bool {
	if s == nil {
		return false
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return false
	}

	s.queueMu.Lock()
	defer s.queueMu.Unlock()
	queued := make([]acmeTaskJob, 0, len(s.queue))
	removed := false
	for {
		select {
		case job := <-s.queue:
			if job.id == id {
				removed = true
				continue
			}
			queued = append(queued, job)
		default:
			for _, job := range queued {
				s.queue <- job
			}
			return removed
		}
	}
}

// finishQueuedManagedWhenCancelled releases an install that never left the
// shared ACME queue. Running work is deliberately left to its own worker so it
// can close files and processes before finalizing the managed task.
func (s *acmeTaskStore) finishQueuedManagedWhenCancelled(id string) {
	s.mu.Lock()
	entry := s.tasks[strings.TrimSpace(id)]
	if entry == nil || entry.managed == nil || entry.ctx == nil {
		s.mu.Unlock()
		return
	}
	ctx := entry.ctx
	s.mu.Unlock()

	<-ctx.Done()
	s.finishQueuedManagedIfCancelled(id)
}

func (s *acmeTaskStore) finishQueuedManagedIfCancelled(id string) {
	id = strings.TrimSpace(id)
	if id == "" {
		return
	}

	s.mu.Lock()
	entry := s.tasks[id]
	if entry == nil || entry.managed == nil || entry.view.Status != acmeTaskStatusQueued || entry.ctx == nil || entry.ctx.Err() == nil {
		s.mu.Unlock()
		return
	}
	managed := entry.managed
	s.mu.Unlock()

	s.removeQueuedJob(id)
	managed.FinishCancelled("cancelled")
	s.finishManaged(id, nil, context.Canceled, managed.Snapshot())
}

func (s *acmeTaskStore) run() {
	for job := range s.queue {
		s.markRunning(job.id)
		logSessionID, ok := s.getLogSessionID(job.id)
		if !ok {
			continue
		}
		ctx, operation, managed := s.getOperation(job.id)
		if ctx != nil && ctx.Err() != nil {
			cancelErr := fmt.Errorf("ACME 后台任务已取消: %w", ctx.Err())
			if managed != nil {
				managed.FinishCancelled("cancelled")
				s.finishManaged(job.id, nil, cancelErr, managed.Snapshot())
			} else {
				s.finish(job.id, nil, cancelErr)
			}
			if managed == nil && operation != nil {
				operation.Done()
			}
			continue
		}
		result, err := runAcmeTaskJob(job.run, logSessionID)
		if managed != nil {
			managedStatus := managed.Snapshot()
			if err != nil && !isManagedDownloadTaskTerminal(managedStatus.State) {
				if ctx != nil && ctx.Err() != nil {
					managed.FinishCancelled("cancelled")
				} else {
					managed.FinishError("failed", err)
				}
				managedStatus = managed.Snapshot()
			}
			s.finishManaged(job.id, result, err, managedStatus)
		} else {
			s.finish(job.id, result, err)
		}
		if managed == nil && operation != nil {
			operation.Done()
		}
	}
}

func runAcmeTaskJob(run func(logSessionID string) (*AcmeActionResult, error), logSessionID string) (result *AcmeActionResult, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("ACME background task panicked: %v", recovered)
			result = nil
		}
	}()
	return run(logSessionID)
}

func (s *acmeTaskStore) markRunning(id string) {
	now := time.Now().Unix()
	s.mu.Lock()
	entry := s.tasks[id]
	if entry != nil && entry.view.Status == acmeTaskStatusQueued {
		entry.view.Status = acmeTaskStatusRunning
		entry.view.UpdatedAt = now
	}
	s.mu.Unlock()
	if entry != nil {
		acmeLogSessionStore.setTaskState(entry.view.LogSessionID, entry.view.ID, acmeTaskStatusRunning, nil, nil, "")
	}
}

func (s *acmeTaskStore) finish(id string, result *AcmeActionResult, taskErr error) {
	now := time.Now().Unix()
	s.mu.Lock()
	entry := s.tasks[id]
	if entry == nil {
		s.mu.Unlock()
		return
	}
	entry.view.Result = cloneAcmeActionResult(result)
	entry.view.UpdatedAt = now
	entry.view.FinishedAt = now
	entry.view.Error = ""
	entry.view.Warnings = nil
	if taskErr != nil {
		entry.view.Status = acmeTaskStatusError
		entry.view.Error = strings.TrimSpace(taskErr.Error())
	} else {
		entry.view.Warnings = normalizeAcmeTaskWarnings(result)
		if len(entry.view.Warnings) > 0 {
			entry.view.Status = acmeTaskStatusWarning
		} else {
			entry.view.Status = acmeTaskStatusSuccess
		}
	}
	view := cloneAcmeTaskView(&entry.view)
	s.mu.Unlock()

	acmeLogSessionStore.setTaskState(view.LogSessionID, view.ID, view.Status, view.Warnings, view.Result, view.Error)
}

func (s *acmeTaskStore) get(id string) *AcmeTaskView {
	now := time.Now().Unix()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneLocked(now)
	entry := s.tasks[strings.TrimSpace(id)]
	if entry == nil {
		return nil
	}
	return cloneAcmeTaskView(&entry.view)
}

func (s *acmeTaskStore) listActive() []AcmeTaskView {
	now := time.Now().Unix()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pruneLocked(now)
	result := make([]AcmeTaskView, 0)
	for _, entry := range s.tasks {
		if entry == nil || (entry.view.Status != acmeTaskStatusQueued && entry.view.Status != acmeTaskStatusRunning) {
			continue
		}
		result = append(result, *cloneAcmeTaskView(&entry.view))
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].StartedAt == result[j].StartedAt {
			return result[i].ID < result[j].ID
		}
		return result[i].StartedAt < result[j].StartedAt
	})
	return result
}

func (s *acmeTaskStore) getLogSessionID(id string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry := s.tasks[id]
	if entry == nil {
		return "", false
	}
	return entry.view.LogSessionID, true
}

func (s *acmeTaskStore) getOperation(id string) (context.Context, *KworManagedOperationHandle, *ManagedDownloadTaskHandle) {
	s.mu.Lock()
	defer s.mu.Unlock()
	entry := s.tasks[id]
	if entry == nil {
		return nil, nil, nil
	}
	return entry.ctx, entry.operation, entry.managed
}

func (s *acmeTaskStore) finishManaged(id string, result *AcmeActionResult, taskErr error, managedStatus ManagedDownloadTaskStatus) {
	now := time.Now().Unix()
	s.mu.Lock()
	entry := s.tasks[id]
	if entry == nil {
		s.mu.Unlock()
		return
	}
	entry.view.Result = cloneManagedAcmeTaskResult(result)
	entry.view.UpdatedAt = now
	entry.view.FinishedAt = now
	entry.view.Error = strings.TrimSpace(managedStatus.Error)
	entry.view.Warnings = nil
	switch managedStatus.State {
	case managedDownloadTaskSuccess:
		entry.view.Status = acmeTaskStatusSuccess
	case managedDownloadTaskCancelled, managedDownloadTaskTimedOut, managedDownloadTaskError:
		entry.view.Status = acmeTaskStatusError
		if entry.view.Error == "" && taskErr != nil {
			entry.view.Error = strings.TrimSpace(taskErr.Error())
		}
	default:
		entry.view.Status = acmeTaskStatusError
		if taskErr != nil {
			entry.view.Error = strings.TrimSpace(taskErr.Error())
		}
	}
	view := cloneAcmeTaskView(&entry.view)
	s.mu.Unlock()
	acmeLogSessionStore.setTaskState(view.LogSessionID, view.ID, view.Status, nil, view.Result, view.Error)
}

func cloneManagedAcmeTaskResult(source *AcmeActionResult) *AcmeActionResult {
	if source == nil {
		return nil
	}
	return cloneAcmeActionResult(&AcmeActionResult{
		Overview: source.Overview,
		Msg:      source.Msg,
		Warnings: source.Warnings,
	})
}

func (s *acmeTaskStore) pruneLocked(now int64) {
	ttlSeconds := int64(acmeTaskTTL / time.Second)
	for id, entry := range s.tasks {
		if entry == nil || (entry.view.FinishedAt > 0 && now-entry.view.UpdatedAt > ttlSeconds) {
			delete(s.tasks, id)
		}
	}
}

func cloneAcmeTaskView(source *AcmeTaskView) *AcmeTaskView {
	if source == nil {
		return nil
	}
	clone := *source
	clone.Warnings = append([]string(nil), source.Warnings...)
	clone.Result = cloneAcmeActionResult(source.Result)
	return &clone
}

func cloneAcmeActionResult(source *AcmeActionResult) *AcmeActionResult {
	if source == nil {
		return nil
	}
	clone := *source
	clone.Output = summarizeAcmeTaskOutput(source.Output)
	clone.Warnings = append([]string(nil), source.Warnings...)
	if source.Certificate != nil {
		certificate := *source.Certificate
		certificate.Domains = append([]string(nil), source.Certificate.Domains...)
		clone.Certificate = &certificate
	}
	return &clone
}

func summarizeAcmeTaskOutput(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= acmeTaskResultOutputMaxRunes {
		return value
	}
	return string(runes[:acmeTaskResultOutputMaxRunes]) + "\n...（任务结果摘要已截断，完整内容请查看任务日志）"
}

func normalizeAcmeTaskWarnings(result *AcmeActionResult) []string {
	if result == nil {
		return nil
	}
	seen := make(map[string]struct{}, len(result.Warnings))
	warnings := make([]string, 0, len(result.Warnings))
	for _, item := range result.Warnings {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if _, exists := seen[item]; exists {
			continue
		}
		seen[item] = struct{}{}
		warnings = append(warnings, item)
	}
	return warnings
}

func newAcmeTaskID() string {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return fmt.Sprintf("task-%d", time.Now().UnixNano())
	}
	return "task-" + hex.EncodeToString(buf[:])
}

func (s *AcmeService) StartIssueTask(payload AcmeIssuePayload) (*AcmeTaskView, error) {
	return acmeTaskSessionStore.enqueue("issue", "证书签发", func(logSessionID string) (*AcmeActionResult, error) {
		payload.LogSessionID = logSessionID
		return s.Issue(payload)
	})
}

func (s *AcmeService) StartReissueTask(payload AcmeIssuePayload) (*AcmeTaskView, error) {
	return acmeTaskSessionStore.enqueue("reissue", "编辑并重新签发", func(logSessionID string) (*AcmeActionResult, error) {
		payload.LogSessionID = logSessionID
		return s.Issue(payload)
	})
}

func (s *AcmeService) StartRenewTask(payload AcmeRenewPayload) (*AcmeTaskView, error) {
	return acmeTaskSessionStore.enqueue("renew", "证书续签", func(logSessionID string) (*AcmeActionResult, error) {
		payload.LogSessionID = logSessionID
		return s.Renew(payload)
	})
}

func (s *AcmeService) GetTask(id string) (*AcmeTaskView, error) {
	task := acmeTaskSessionStore.get(id)
	if task == nil {
		return nil, fmt.Errorf("ACME 任务不存在或已过期")
	}
	return task, nil
}

func (s *AcmeService) GetActiveTasks() []AcmeTaskView {
	return acmeTaskSessionStore.listActive()
}
