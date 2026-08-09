package service

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
)

const (
	managedDownloadTaskDeadline = 20 * time.Minute
	managedDownloadTaskTTL      = 30 * time.Minute
)

const (
	managedDownloadTaskQueued    = "queued"
	managedDownloadTaskRunning   = "running"
	managedDownloadTaskStopping  = "stopping"
	managedDownloadTaskCancelled = "cancelled"
	managedDownloadTaskTimedOut  = "timed_out"
	managedDownloadTaskSuccess   = "success"
	managedDownloadTaskError     = "error"
	managedDownloadTaskIdle      = "idle"
)

// ManagedDownloadTaskStatus is deliberately small. Download-specific stores
// add byte counters and other presentation-only details on top of it.
type ManagedDownloadTaskStatus struct {
	ID               string `json:"id,omitempty"`
	State            string `json:"state"`
	Phase            string `json:"phase,omitempty"`
	CanCancel        bool   `json:"canCancel"`
	StopRequested    bool   `json:"stopRequested"`
	DeadlineExceeded bool   `json:"deadlineExceeded"`
	StartedAt        int64  `json:"startedAt,omitempty"`
	UpdatedAt        int64  `json:"updatedAt,omitempty"`
	DeadlineAt       int64  `json:"deadlineAt,omitempty"`
	FinishedAt       int64  `json:"finishedAt,omitempty"`
	Error            string `json:"error,omitempty"`
}

type managedDownloadTask struct {
	status       ManagedDownloadTaskStatus
	fingerprint  string
	ctx          context.Context
	cancel       context.CancelFunc
	operation    *KworManagedOperationHandle
	deadline     *time.Timer
	expires      *time.Timer
	timeoutStop  bool
	irreversible bool
}

// ManagedDownloadTaskHandle is only handed to the worker that owns the task.
// Stop requests never finalize it; the worker must clean its files first.
type ManagedDownloadTaskHandle struct {
	manager *ManagedDownloadTaskManager
	id      string
}

type ManagedDownloadTaskManagerOptions struct {
	Deadline time.Duration
}

// ManagedDownloadTaskManager keeps one active task per resource. Completed
// snapshots remain queryable briefly so a refreshed browser can show a result.
type ManagedDownloadTaskManager struct {
	mu       sync.Mutex
	name     string
	deadline time.Duration
	tasks    map[string]*managedDownloadTask
	activeID string
	latestID string
}

func NewManagedDownloadTaskManager(name string) *ManagedDownloadTaskManager {
	return NewManagedDownloadTaskManagerWithOptions(name, ManagedDownloadTaskManagerOptions{})
}

func NewManagedDownloadTaskManagerWithOptions(name string, options ManagedDownloadTaskManagerOptions) *ManagedDownloadTaskManager {
	deadline := options.Deadline
	if deadline <= 0 {
		deadline = managedDownloadTaskDeadline
	}
	return &ManagedDownloadTaskManager{
		name:     strings.TrimSpace(name),
		deadline: deadline,
		tasks:    make(map[string]*managedDownloadTask),
	}
}

func (m *ManagedDownloadTaskManager) taskDeadline() time.Duration {
	if m == nil || m.deadline <= 0 {
		return managedDownloadTaskDeadline
	}
	return m.deadline
}

// Start either creates a task or returns the equivalent active task. A caller
// must start the worker only when created is true.
func (m *ManagedDownloadTaskManager) Start(operationKind string, fingerprint string) (*ManagedDownloadTaskHandle, ManagedDownloadTaskStatus, bool, error) {
	if m == nil {
		return nil, ManagedDownloadTaskStatus{}, false, fmt.Errorf("managed download task manager is nil")
	}
	fingerprint = strings.TrimSpace(fingerprint)
	operationKind = strings.TrimSpace(operationKind)
	if operationKind == "" {
		return nil, ManagedDownloadTaskStatus{}, false, fmt.Errorf("managed download task operation kind is required")
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.pruneExpiredLocked(time.Now())
	if active := m.tasks[m.activeID]; active != nil {
		if active.fingerprint == fingerprint {
			return &ManagedDownloadTaskHandle{manager: m, id: active.status.ID}, cloneManagedDownloadTaskStatus(active.status), false, nil
		}
		return nil, ManagedDownloadTaskStatus{}, false, fmt.Errorf("%s task is already running", firstManagedDownloadTaskName(m.name, operationKind))
	}

	operationContext, operation, err := BeginKworManagedOperation(operationKind)
	if err != nil {
		return nil, ManagedDownloadTaskStatus{}, false, err
	}
	ctx, cancel := context.WithCancel(operationContext)
	now := time.Now()
	deadline := m.taskDeadline()
	task := &managedDownloadTask{
		fingerprint: fingerprint,
		ctx:         ctx,
		cancel:      cancel,
		operation:   operation,
		status: ManagedDownloadTaskStatus{
			ID:         operation.ID(),
			State:      managedDownloadTaskQueued,
			Phase:      "preparing",
			CanCancel:  true,
			StartedAt:  now.Unix(),
			UpdatedAt:  now.Unix(),
			DeadlineAt: now.Add(deadline).Unix(),
		},
	}
	task.deadline = time.AfterFunc(deadline, func() {
		m.requestStop(task.status.ID, true)
	})
	m.tasks[task.status.ID] = task
	m.activeID = task.status.ID
	m.latestID = task.status.ID
	return &ManagedDownloadTaskHandle{manager: m, id: task.status.ID}, cloneManagedDownloadTaskStatus(task.status), true, nil
}

func firstManagedDownloadTaskName(name string, fallback string) string {
	if trimmed := strings.TrimSpace(name); trimmed != "" {
		return trimmed
	}
	return fallback
}

func (h *ManagedDownloadTaskHandle) ID() string {
	if h == nil {
		return ""
	}
	return h.id
}

func (h *ManagedDownloadTaskHandle) Context() context.Context {
	if h == nil || h.manager == nil {
		return context.Background()
	}
	h.manager.mu.Lock()
	defer h.manager.mu.Unlock()
	if task := h.manager.tasks[h.id]; task != nil && task.ctx != nil {
		return task.ctx
	}
	return context.Background()
}

func (h *ManagedDownloadTaskHandle) Operation() *KworManagedOperationHandle {
	if h == nil || h.manager == nil {
		return nil
	}
	h.manager.mu.Lock()
	defer h.manager.mu.Unlock()
	if task := h.manager.tasks[h.id]; task != nil {
		return task.operation
	}
	return nil
}

// DetachOperation transfers lifecycle ownership to an external worker. The
// worker must eventually call FinishKworManagedOperation with this operation ID.
func (h *ManagedDownloadTaskHandle) DetachOperation() *KworManagedOperationHandle {
	if h == nil || h.manager == nil {
		return nil
	}
	h.manager.mu.Lock()
	task := h.manager.tasks[h.id]
	if task == nil {
		h.manager.mu.Unlock()
		return nil
	}
	operation := task.operation
	task.operation = nil
	h.manager.mu.Unlock()
	if operation != nil {
		operation.Detach()
	}
	return operation
}

func (h *ManagedDownloadTaskHandle) MarkRunning(phase string) bool {
	return h.update(phase, true, false)
}

// BeginApplying is the atomic boundary between reversible work and host
// changes. If a stop won the race, it returns false and the caller must exit.
func (h *ManagedDownloadTaskHandle) BeginApplying(phase string) bool {
	return h.update(phase, false, true)
}

func (h *ManagedDownloadTaskHandle) SetPhase(phase string, canCancel bool) bool {
	return h.update(phase, canCancel, false)
}

func (h *ManagedDownloadTaskHandle) update(phase string, canCancel bool, applying bool) bool {
	if h == nil || h.manager == nil {
		return false
	}
	phase = strings.TrimSpace(phase)
	h.manager.mu.Lock()
	defer h.manager.mu.Unlock()
	task := h.manager.tasks[h.id]
	if task == nil {
		return false
	}
	if task.status.State == managedDownloadTaskStopping || isManagedDownloadTaskTerminal(task.status.State) || task.ctx.Err() != nil {
		return false
	}
	if task.status.State == managedDownloadTaskQueued {
		task.status.State = managedDownloadTaskRunning
	}
	if phase != "" {
		task.status.Phase = phase
	}
	if applying {
		task.irreversible = true
		task.status.CanCancel = false
	} else if task.irreversible {
		task.status.CanCancel = false
	} else {
		task.status.CanCancel = canCancel
	}
	task.status.UpdatedAt = time.Now().Unix()
	return true
}

func (h *ManagedDownloadTaskHandle) Snapshot() ManagedDownloadTaskStatus {
	if h == nil || h.manager == nil {
		return ManagedDownloadTaskStatus{State: managedDownloadTaskIdle}
	}
	return h.manager.Get(h.id)
}

func (h *ManagedDownloadTaskHandle) FinishSuccess(phase string) {
	h.finish(managedDownloadTaskSuccess, phase, "")
}

func (h *ManagedDownloadTaskHandle) FinishError(phase string, err error) {
	message := ""
	if err != nil {
		message = strings.TrimSpace(err.Error())
	}
	h.finish(managedDownloadTaskError, phase, message)
}

func (h *ManagedDownloadTaskHandle) FinishCancelled(phase string) {
	h.finish("", phase, "")
}

// finishManagedDownloadTaskPanic keeps a background worker fault scoped to
// its task instead of letting it terminate the panel process. Workers defer it
// near their start so later resource-release defers run before it.
func finishManagedDownloadTaskPanic(handle *ManagedDownloadTaskHandle, phase string, taskName string) {
	if recovered := recover(); recovered != nil && handle != nil {
		taskName = strings.TrimSpace(taskName)
		if taskName == "" {
			taskName = "managed download"
		}
		handle.FinishError(phase, fmt.Errorf("%s task panicked: %v", taskName, recovered))
	}
}

func (h *ManagedDownloadTaskHandle) finish(state string, phase string, message string) {
	if h == nil || h.manager == nil {
		return
	}
	phase = strings.TrimSpace(phase)
	message = strings.TrimSpace(message)
	h.manager.mu.Lock()
	task := h.manager.tasks[h.id]
	if task == nil || isManagedDownloadTaskTerminal(task.status.State) {
		h.manager.mu.Unlock()
		return
	}
	// FinishCancelled is an explicit terminal choice. A lifecycle shutdown can
	// cancel the task context without first changing the browser-facing state to
	// "stopping", so relying on that state alone would incorrectly surface a
	// cancellation as an error.
	if task.status.State == managedDownloadTaskStopping || state == "" {
		if task.timeoutStop {
			state = managedDownloadTaskTimedOut
			if message == "" {
				message = "download task timed out"
			}
		} else {
			state = managedDownloadTaskCancelled
			if message == "" {
				message = "download task was stopped"
			}
		}
	}
	if state == "" {
		state = managedDownloadTaskError
	}
	now := time.Now()
	task.status.State = state
	if phase != "" {
		task.status.Phase = phase
	}
	task.status.CanCancel = false
	task.status.Error = message
	task.status.UpdatedAt = now.Unix()
	task.status.FinishedAt = now.Unix()
	if task.deadline != nil {
		task.deadline.Stop()
		task.deadline = nil
	}
	if task.cancel != nil {
		task.cancel()
		task.cancel = nil
	}
	if h.manager.activeID == h.id {
		h.manager.activeID = ""
	}
	operation := task.operation
	task.operation = nil
	task.expires = time.AfterFunc(managedDownloadTaskTTL, func() {
		h.manager.expire(h.id)
	})
	h.manager.mu.Unlock()
	if operation != nil {
		operation.Done()
	}
}

func (m *ManagedDownloadTaskManager) Stop(id string) (ManagedDownloadTaskStatus, error) {
	if m == nil {
		return ManagedDownloadTaskStatus{}, fmt.Errorf("managed download task manager is nil")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return ManagedDownloadTaskStatus{}, fmt.Errorf("task id is required")
	}
	status, ok := m.requestStop(id, false)
	if !ok {
		return status, fmt.Errorf("download task cannot be stopped")
	}
	return status, nil
}

func (m *ManagedDownloadTaskManager) requestStop(id string, timeout bool) (ManagedDownloadTaskStatus, bool) {
	if m == nil {
		return ManagedDownloadTaskStatus{State: managedDownloadTaskIdle}, false
	}
	m.mu.Lock()
	task := m.tasks[strings.TrimSpace(id)]
	if task == nil || isManagedDownloadTaskTerminal(task.status.State) || !task.status.CanCancel {
		if task != nil && timeout && !isManagedDownloadTaskTerminal(task.status.State) {
			task.status.DeadlineExceeded = true
			task.status.UpdatedAt = time.Now().Unix()
		}
		status := ManagedDownloadTaskStatus{State: managedDownloadTaskIdle}
		if task != nil {
			status = cloneManagedDownloadTaskStatus(task.status)
		}
		m.mu.Unlock()
		return status, false
	}
	task.status.State = managedDownloadTaskStopping
	task.status.Phase = "stopping"
	task.status.CanCancel = false
	task.status.StopRequested = true
	if timeout {
		task.timeoutStop = true
		task.status.DeadlineExceeded = true
	}
	task.status.UpdatedAt = time.Now().Unix()
	status := cloneManagedDownloadTaskStatus(task.status)
	cancel := task.cancel
	m.mu.Unlock()
	if cancel != nil {
		cancel()
	}
	return status, true
}

func (m *ManagedDownloadTaskManager) Get(id string) ManagedDownloadTaskStatus {
	if m == nil {
		return ManagedDownloadTaskStatus{State: managedDownloadTaskIdle}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.pruneExpiredLocked(time.Now())
	if strings.TrimSpace(id) == "" {
		id = m.activeID
		if id == "" {
			id = m.latestID
		}
	}
	if task := m.tasks[strings.TrimSpace(id)]; task != nil {
		return cloneManagedDownloadTaskStatus(task.status)
	}
	return ManagedDownloadTaskStatus{State: managedDownloadTaskIdle}
}

func (m *ManagedDownloadTaskManager) IsActive() bool {
	if m == nil {
		return false
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.tasks[m.activeID] != nil
}

func (m *ManagedDownloadTaskManager) expire(id string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	task := m.tasks[id]
	if task == nil || !isManagedDownloadTaskTerminal(task.status.State) {
		return
	}
	delete(m.tasks, id)
	if m.latestID == id {
		m.latestID = ""
	}
}

func (m *ManagedDownloadTaskManager) pruneExpiredLocked(now time.Time) {
	for id, task := range m.tasks {
		if task == nil || !isManagedDownloadTaskTerminal(task.status.State) || task.status.FinishedAt == 0 {
			continue
		}
		if now.Sub(time.Unix(task.status.FinishedAt, 0)) <= managedDownloadTaskTTL {
			continue
		}
		if task.expires != nil {
			task.expires.Stop()
		}
		delete(m.tasks, id)
		if m.latestID == id {
			m.latestID = ""
		}
	}
}

func isManagedDownloadTaskTerminal(state string) bool {
	switch strings.TrimSpace(state) {
	case managedDownloadTaskCancelled, managedDownloadTaskTimedOut, managedDownloadTaskSuccess, managedDownloadTaskError:
		return true
	default:
		return false
	}
}

func cloneManagedDownloadTaskStatus(status ManagedDownloadTaskStatus) ManagedDownloadTaskStatus {
	return status
}

// lockManagedDownloadTaskMutex waits in short, cancellable intervals instead
// of leaving a background task stranded behind a regular sync.Mutex. The
// caller owns the successful lock and must unlock it normally.
func lockManagedDownloadTaskMutex(ctx context.Context, mu *sync.Mutex) error {
	if mu == nil {
		return fmt.Errorf("managed download task mutex is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if mu.TryLock() {
		return nil
	}

	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if mu.TryLock() {
				return nil
			}
		}
	}
}

// copyManagedDownloadTaskContext checks the task context between bounded
// chunks so local archive extraction stops promptly alongside HTTP downloads.
func copyManagedDownloadTaskContext(ctx context.Context, destination io.Writer, source io.Reader) (int64, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}

	buffer := make([]byte, 64*1024)
	var written int64
	for {
		if err := ctx.Err(); err != nil {
			return written, err
		}
		read, readErr := source.Read(buffer)
		if read > 0 {
			remaining := buffer[:read]
			for len(remaining) > 0 {
				if err := ctx.Err(); err != nil {
					return written, err
				}
				count, writeErr := destination.Write(remaining)
				if count > 0 {
					written += int64(count)
					remaining = remaining[count:]
				}
				if writeErr != nil {
					return written, writeErr
				}
				if count == 0 {
					return written, io.ErrShortWrite
				}
			}
		}
		if readErr == io.EOF {
			return written, nil
		}
		if readErr != nil {
			return written, readErr
		}
	}
}
