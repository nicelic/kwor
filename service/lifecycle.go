package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	kworLifecycleStateVersion       = 1
	kworLifecycleStateFileName      = "uninstall-v1.json"
	kworLifecycleOperationsFileName = "operations-v1.json"
	kworLifecycleOperationIDEnv     = "KWOR_LIFECYCLE_OPERATION_ID"
	kworUninstallStartupGrace       = 30 * time.Second
)

const (
	kworUninstallStatusScheduled = "scheduled"
	kworUninstallStatusRunning   = "running"
	kworUninstallStatusFailed    = "failed"
)

// KworUninstallLifecycleState 保存在运行时目录，独立于面板进程内存。
// 它只保存调度与诊断信息；长期的“未完成卸载”判定仍由 ownership-v1.json
// 的 Uninstalling 字段承担，以便重启后继续保护现场。
type KworUninstallLifecycleState struct {
	Version         int      `json:"version"`
	Status          string   `json:"status"`
	Phase           string   `json:"phase,omitempty"`
	Error           string   `json:"error,omitempty"`
	Failures        []string `json:"failures,omitempty"`
	Warnings        []string `json:"warnings,omitempty"`
	BlockNewWork    bool     `json:"blockNewWork,omitempty"`
	ReservationID   string   `json:"reservationId,omitempty"`
	WorkerPID       int      `json:"workerPid,omitempty"`
	WorkerPGID      int      `json:"workerPgid,omitempty"`
	WorkerStartTime uint64   `json:"workerStartTime,omitempty"`
	WorkerUnit      string   `json:"workerUnit,omitempty"`
	UpdatedAt       int64    `json:"updatedAt"`
}

// KworManagedOperationRecord 是可跨进程读取的受管后台操作登记。PID/进程组
// 只在子进程真实启动后填入，避免把面板自身误作可终止目标。
type KworManagedOperationRecord struct {
	ID               string `json:"id"`
	Kind             string `json:"kind"`
	PID              int    `json:"pid,omitempty"`
	PGID             int    `json:"pgid,omitempty"`
	ProcessStartTime uint64 `json:"processStartTime,omitempty"`
	Unit             string `json:"unit,omitempty"`
	BlockNewWork     bool   `json:"blockNewWork,omitempty"`
	BlockingSince    int64  `json:"blockingSince,omitempty"`
	StartedAt        int64  `json:"startedAt"`
}

type kworManagedOperationsFile struct {
	Version    int                          `json:"version"`
	Operations []KworManagedOperationRecord `json:"operations"`
}

type KworManagedOperationHandle struct {
	id         string
	ctx        context.Context
	cancel     context.CancelFunc
	done       chan struct{}
	persistent bool
	once       sync.Once
	detachOnce sync.Once
}

type kworManagedOperationContextKey struct{}

var (
	kworLifecycleRuntimeDirFn = func() string { return "/run/kwor" }
	kworLifecycleNowFn        = time.Now

	kworLifecycleFileMu                      sync.Mutex
	kworLifecycleTaskMu                      sync.Mutex
	kworLifecycleTasks                       = make(map[string]*KworManagedOperationHandle)
	kworLifecycleQuiescing                   bool
	kworLifecycleTerminateManagedOperationFn = terminateKworManagedOperation
	kworLifecycleManagedOperationAliveFn     = kworManagedOperationAlive
	kworLifecycleLoadManagedOperationsFn     = loadKworManagedOperations
	kworLifecycleRemoveManagedOperationFn    = removeKworManagedOperation
	kworLifecycleQuiesceTimeout              = 5 * time.Second
	kworLifecycleBeforeRegisterHook          func()
)

func acquireKworLifecycleFileLock() (func() error, error) {
	kworLifecycleFileMu.Lock()
	releaseProcessLock, err := acquireKworLifecycleMetadataLock()
	if err != nil {
		kworLifecycleFileMu.Unlock()
		return nil, err
	}
	return func() error {
		releaseErr := releaseProcessLock()
		kworLifecycleFileMu.Unlock()
		return releaseErr
	}, nil
}

func KworLifecycleRuntimeDir() string {
	return filepath.Clean(kworLifecycleRuntimeDirFn())
}

func KworUninstallLifecycleStatePath() string {
	return filepath.Join(KworLifecycleRuntimeDir(), kworLifecycleStateFileName)
}

func kworManagedOperationsPath() string {
	return filepath.Join(KworLifecycleRuntimeDir(), kworLifecycleOperationsFileName)
}

func LoadKworUninstallLifecycleState() (*KworUninstallLifecycleState, bool, error) {
	if runtime.GOOS != "linux" {
		return nil, false, nil
	}
	unlock, err := acquireKworLifecycleFileLock()
	if err != nil {
		return nil, false, err
	}
	defer unlock()
	return loadKworUninstallLifecycleStateLocked()
}

func loadKworUninstallLifecycleStateLocked() (*KworUninstallLifecycleState, bool, error) {
	raw, err := os.ReadFile(KworUninstallLifecycleStatePath())
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	state := &KworUninstallLifecycleState{}
	if err := json.Unmarshal(raw, state); err != nil {
		return nil, false, fmt.Errorf("解析卸载运行状态失败: %w", err)
	}
	if state.Version != kworLifecycleStateVersion {
		return nil, false, fmt.Errorf("不支持的卸载运行状态版本: %d", state.Version)
	}
	state.Status = strings.TrimSpace(state.Status)
	state.Phase = strings.TrimSpace(state.Phase)
	state.Error = strings.TrimSpace(state.Error)
	state.Failures = normalizeKworUninstallMessages(state.Failures)
	state.Warnings = normalizeKworUninstallMessages(state.Warnings)
	state.ReservationID = strings.TrimSpace(state.ReservationID)
	state.WorkerUnit = strings.TrimSpace(state.WorkerUnit)
	return state, true, nil
}

func ReservePanelUninstallLifecycle() (string, bool, error) {
	if runtime.GOOS != "linux" {
		return "non-linux", true, nil
	}
	unlock, err := acquireKworLifecycleFileLock()
	if err != nil {
		return "", false, err
	}
	defer unlock()
	state, found, err := loadKworUninstallLifecycleStateLocked()
	if err != nil {
		return "", false, err
	}
	if found && state != nil && lifecycleUninstallTakeoverBlocked(*state, kworLifecycleNowFn(), lifecycleUninstallWorkerAlive(*state)) {
		return "", false, nil
	}
	reservationID, err := newHostOwnershipID()
	if err != nil {
		return "", false, fmt.Errorf("生成卸载调度凭证失败: %w", err)
	}
	if state == nil {
		state = &KworUninstallLifecycleState{}
	}
	state.Version = kworLifecycleStateVersion
	state.Status = kworUninstallStatusScheduled
	state.Phase = "等待独立卸载 worker 启动"
	state.Error = ""
	state.Failures = nil
	state.Warnings = nil
	state.BlockNewWork = true
	state.ReservationID = reservationID
	state.WorkerPID = 0
	state.WorkerPGID = 0
	state.WorkerStartTime = 0
	state.WorkerUnit = ""
	state.UpdatedAt = kworLifecycleNowFn().Unix()
	if err := saveKworUninstallLifecycleStateLocked(*state); err != nil {
		return "", false, err
	}
	return reservationID, true, nil
}

func RecordPanelUninstallLifecycleWorker(reservationID string, pid int, pgid int, unit string) error {
	if runtime.GOOS != "linux" {
		return nil
	}
	reservationID = strings.TrimSpace(reservationID)
	if reservationID == "" {
		return errors.New("卸载调度凭证为空")
	}
	unlock, err := acquireKworLifecycleFileLock()
	if err != nil {
		return err
	}
	defer unlock()
	state, found, err := loadKworUninstallLifecycleStateLocked()
	if err != nil {
		return err
	}
	// worker 可能已运行、失败，甚至已完成并清除了运行状态。父进程只能
	// 补全自己预留的 scheduled 状态，绝不能重建或回退已经推进的状态。
	if !found || state == nil {
		return nil
	}
	if state.ReservationID != reservationID {
		return errors.New("卸载运行状态已由其他调度接管")
	}
	if state.Status != kworUninstallStatusScheduled {
		return nil
	}
	startTime := uint64(0)
	if pid > 0 {
		var identityErr error
		startTime, identityErr = kworLifecycleProcessStartTime(pid)
		if identityErr != nil {
			return fmt.Errorf("读取独立卸载 worker 身份失败: %w", identityErr)
		}
	}
	updated, err := applyPanelUninstallWorkerRecord(state, reservationID, pid, pgid, startTime, unit)
	if err != nil || !updated {
		return err
	}
	return saveKworUninstallLifecycleStateLocked(*state)
}

func applyPanelUninstallWorkerRecord(state *KworUninstallLifecycleState, reservationID string, pid int, pgid int, startTime uint64, unit string) (bool, error) {
	if state == nil {
		return false, nil
	}
	if state.ReservationID != strings.TrimSpace(reservationID) {
		return false, errors.New("卸载运行状态已由其他调度接管")
	}
	if state.Status != kworUninstallStatusScheduled {
		return false, nil
	}
	state.Phase = "独立卸载 worker 已启动"
	state.Error = ""
	state.WorkerPID = pid
	state.WorkerPGID = pgid
	state.WorkerStartTime = startTime
	state.WorkerUnit = strings.TrimSpace(unit)
	return true, nil
}

func RecordPanelUninstallLifecycleRollbackFailure(reservationID string, pid int, pgid int, startTime uint64, unit string, taskErr error) error {
	if runtime.GOOS != "linux" {
		return nil
	}
	reservationID = strings.TrimSpace(reservationID)
	if reservationID == "" {
		return errors.New("卸载调度凭证为空")
	}
	unlock, err := acquireKworLifecycleFileLock()
	if err != nil {
		return err
	}
	defer unlock()
	state, found, err := loadKworUninstallLifecycleStateLocked()
	if err != nil {
		return err
	}
	if !found || state == nil {
		return nil
	}
	updated, err := applyPanelUninstallRollbackFailure(state, reservationID, pid, pgid, startTime, unit, taskErr)
	if err != nil || !updated {
		return err
	}
	return saveKworUninstallLifecycleStateLocked(*state)
}

func applyPanelUninstallRollbackFailure(state *KworUninstallLifecycleState, reservationID string, pid int, pgid int, startTime uint64, unit string, taskErr error) (bool, error) {
	if state == nil {
		return false, nil
	}
	if state.ReservationID != strings.TrimSpace(reservationID) {
		return false, errors.New("卸载运行状态已由其他调度接管")
	}
	if state.Status != kworUninstallStatusScheduled {
		return false, nil
	}
	state.Status = kworUninstallStatusFailed
	state.Phase = "独立卸载 worker 回滚失败"
	state.BlockNewWork = true
	state.WorkerPID = pid
	state.WorkerPGID = pgid
	state.WorkerStartTime = startTime
	state.WorkerUnit = strings.TrimSpace(unit)
	if taskErr != nil {
		state.Error = strings.TrimSpace(taskErr.Error())
	}
	return true, nil
}

func MarkPanelUninstallLifecycleScheduleFailed(reservationID string, taskErr error) {
	if runtime.GOOS != "linux" {
		return
	}
	reservationID = strings.TrimSpace(reservationID)
	_ = updateKworUninstallLifecycleState(func(state *KworUninstallLifecycleState) {
		if reservationID == "" || state.ReservationID != reservationID || state.Status != kworUninstallStatusScheduled {
			return
		}
		state.Status = kworUninstallStatusFailed
		state.Phase = "独立卸载 worker 未能启动"
		state.BlockNewWork = false
		if taskErr != nil {
			state.Error = strings.TrimSpace(taskErr.Error())
			state.Failures = normalizeKworUninstallMessages([]string{state.Error})
		}
		state.WorkerPID = 0
		state.WorkerPGID = 0
		state.WorkerStartTime = 0
		state.WorkerUnit = ""
	})
}

func BeginKworUninstallLifecycle(phase string) error {
	if runtime.GOOS != "linux" {
		return nil
	}
	return updateKworUninstallLifecycleState(func(state *KworUninstallLifecycleState) {
		state.Status = kworUninstallStatusRunning
		state.Phase = strings.TrimSpace(phase)
		state.Error = ""
		state.Failures = nil
		state.Warnings = nil
		state.BlockNewWork = true
		if state.WorkerPID == 0 {
			state.WorkerPID = os.Getpid()
			if startTime, err := kworLifecycleProcessStartTime(state.WorkerPID); err == nil {
				state.WorkerStartTime = startTime
			}
		}
	})
}

func UpdateKworUninstallLifecycle(phase string) error {
	if runtime.GOOS != "linux" {
		return nil
	}
	return updateKworUninstallLifecycleState(func(state *KworUninstallLifecycleState) {
		state.Status = kworUninstallStatusRunning
		state.Phase = strings.TrimSpace(phase)
		state.Error = ""
		state.BlockNewWork = true
	})
}

func FailKworUninstallLifecycle(taskErr error) {
	FailKworUninstallLifecycleWithReport(taskErr, nil)
}

// FailKworUninstallLifecycleWithReport keeps the last actionable phase and a
// bounded cleanup report so callers can present a real recovery error instead
// of leaving the panel at an unexplained permanent overlay.
func FailKworUninstallLifecycleWithReport(taskErr error, report *UninstallReport) {
	if runtime.GOOS != "linux" {
		return
	}
	_ = updateKworUninstallLifecycleState(func(state *KworUninstallLifecycleState) {
		state.Status = kworUninstallStatusFailed
		if strings.TrimSpace(state.Phase) == "" {
			state.Phase = "卸载未完成"
		}
		state.BlockNewWork = true
		if taskErr != nil {
			state.Error = strings.TrimSpace(taskErr.Error())
		}
		if report != nil {
			state.Failures = normalizeKworUninstallMessages(report.Failures)
			state.Warnings = normalizeKworUninstallMessages(report.Warnings)
		}
		if len(state.Failures) == 0 && state.Error != "" {
			state.Failures = []string{state.Error}
		}
	})
}

func normalizeKworUninstallMessages(values []string) []string {
	const maxMessages = 32
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		duplicate := false
		for _, existing := range result {
			if existing == value {
				duplicate = true
				break
			}
		}
		if duplicate {
			continue
		}
		result = append(result, value)
		if len(result) == maxMessages {
			break
		}
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func ClearKworUninstallLifecycle() error {
	if runtime.GOOS != "linux" {
		return nil
	}
	unlock, err := acquireKworLifecycleFileLock()
	if err != nil {
		return err
	}
	if err := clearKworLifecycleRuntimeArtifactsLocked(); err != nil {
		_ = unlock()
		return err
	}
	if err := unlock(); err != nil {
		return err
	}
	return finishKworLifecycleRuntimeCleanup()
}

func updateKworUninstallLifecycleState(update func(*KworUninstallLifecycleState)) error {
	unlock, err := acquireKworLifecycleFileLock()
	if err != nil {
		return err
	}
	defer unlock()
	state, _, err := loadKworUninstallLifecycleStateLocked()
	if err != nil {
		return err
	}
	if state == nil {
		state = &KworUninstallLifecycleState{Version: kworLifecycleStateVersion}
	}
	update(state)
	state.Version = kworLifecycleStateVersion
	state.UpdatedAt = kworLifecycleNowFn().Unix()
	return saveKworUninstallLifecycleStateLocked(*state)
}

func saveKworUninstallLifecycleStateLocked(state KworUninstallLifecycleState) error {
	state.Version = kworLifecycleStateVersion
	state.UpdatedAt = kworLifecycleNowFn().Unix()
	return writeKworLifecycleJSON(KworUninstallLifecycleStatePath(), state)
}

func KworServiceStartAllowed() error {
	pending, err := IsKworUninstalling()
	if err != nil {
		return err
	}
	if pending {
		return errors.New("检测到未完成卸载，拒绝启动并重建受管资源")
	}
	state, found, err := LoadKworUninstallLifecycleState()
	if err != nil {
		return err
	}
	if found && state != nil && lifecycleStateBlocksNewWork(*state) {
		return errors.New("卸载 worker 正在运行，拒绝启动并重建受管资源")
	}
	return nil
}

func EnsureKworLifecycleAcceptsNewWork() error {
	kworLifecycleTaskMu.Lock()
	defer kworLifecycleTaskMu.Unlock()
	return ensureKworLifecycleAcceptsNewWorkLocked()
}

func ensureKworLifecycleAcceptsNewWorkLocked() error {
	if kworLifecycleQuiescing {
		return errors.New("卸载正在停止后台任务，拒绝创建新任务")
	}
	if runtime.GOOS == "linux" {
		state, found, stateErr := LoadKworUninstallLifecycleState()
		if stateErr != nil {
			return stateErr
		}
		if found && state != nil && lifecycleStateBlocksNewWork(*state) {
			return errors.New("卸载尚未完成，拒绝创建新任务")
		}
	}
	if runtime.GOOS != "linux" {
		return nil
	}
	pending, err := IsKworUninstalling()
	if err != nil {
		return err
	}
	if pending {
		return errors.New("卸载尚未完成，拒绝创建新任务")
	}
	return ensureNoBlockingKworManagedOperations()
}

func lifecycleStateBlocksNewWork(state KworUninstallLifecycleState) bool {
	if state.BlockNewWork {
		return true
	}
	// 早于 BlockNewWork 字段的 v1 状态文件，仍在 worker 已调度或运行时
	// 保持原有的禁止新任务语义。
	return state.Status == kworUninstallStatusScheduled || state.Status == kworUninstallStatusRunning
}

func lifecycleUninstallTakeoverBlocked(state KworUninstallLifecycleState, now time.Time, workerAlive bool) bool {
	switch state.Status {
	case kworUninstallStatusScheduled:
		if workerAlive {
			return true
		}
		if state.UpdatedAt <= 0 {
			return false
		}
		return now.Before(time.Unix(state.UpdatedAt, 0).Add(kworUninstallStartupGrace))
	case kworUninstallStatusRunning:
		return workerAlive
	case kworUninstallStatusFailed:
		return state.BlockNewWork && workerAlive
	default:
		return false
	}
}

func ensureNoBlockingKworManagedOperations() error {
	operations, err := kworLifecycleLoadManagedOperationsFn()
	if err != nil {
		return err
	}
	now := kworLifecycleNowFn()
	for _, operation := range operations {
		if !operation.BlockNewWork {
			continue
		}
		if managedOperationBlockingGraceActive(operation, now) {
			return fmt.Errorf("受管任务正在交接，拒绝创建新任务: %s", operation.Kind)
		}
		alive, aliveErr := kworLifecycleManagedOperationAliveFn(operation)
		if aliveErr != nil {
			return fmt.Errorf("核验阻塞任务 %s 状态失败: %w", operation.ID, aliveErr)
		}
		if alive {
			return fmt.Errorf("受管任务仍在运行，拒绝创建新任务: %s", operation.Kind)
		}
		if err := kworLifecycleRemoveManagedOperationFn(operation.ID); err != nil {
			return fmt.Errorf("清理陈旧阻塞任务 %s 失败: %w", operation.ID, err)
		}
	}
	return nil
}

func managedOperationBlockingGraceActive(operation KworManagedOperationRecord, now time.Time) bool {
	if !operation.BlockNewWork || operation.BlockingSince <= 0 {
		return false
	}
	return now.Before(time.Unix(operation.BlockingSince, 0).Add(kworUninstallStartupGrace))
}

func BeginKworManagedOperation(kind string) (context.Context, *KworManagedOperationHandle, error) {
	return beginKworManagedOperation(kind, true)
}

// BeginKworInProcessOperation tracks a short operation only in the current
// panel process. API writes and cron jobs use it so quiesce can wait for them
// without rewriting operations-v1.json for every request.
func BeginKworInProcessOperation(kind string) (*KworManagedOperationHandle, error) {
	_, handle, err := beginKworManagedOperation(kind, false)
	return handle, err
}

func beginKworManagedOperation(kind string, persistent bool) (context.Context, *KworManagedOperationHandle, error) {
	kind = strings.TrimSpace(kind)
	if kind == "" {
		return nil, nil, errors.New("受管后台任务类型不能为空")
	}
	idPart, err := newHostOwnershipID()
	if err != nil {
		return nil, nil, fmt.Errorf("生成受管后台任务凭证失败: %w", err)
	}
	id := fmt.Sprintf("%s-%s", kind, idPart)
	baseContext, cancel := context.WithCancel(context.Background())
	handle := &KworManagedOperationHandle{id: id, cancel: cancel, done: make(chan struct{}), persistent: persistent}
	handle.ctx = context.WithValue(baseContext, kworManagedOperationContextKey{}, handle)

	// The work-acceptance check and insertion are one critical section. Once
	// quiesce obtains this mutex it therefore sees every operation that passed
	// the gate, including one which was checking the on-disk state concurrently.
	kworLifecycleTaskMu.Lock()
	if err := ensureKworLifecycleAcceptsNewWorkLocked(); err != nil {
		kworLifecycleTaskMu.Unlock()
		cancel()
		return nil, nil, err
	}
	if kworLifecycleBeforeRegisterHook != nil {
		kworLifecycleBeforeRegisterHook()
	}
	kworLifecycleTasks[id] = handle
	if persistent {
		err = upsertKworManagedOperation(KworManagedOperationRecord{ID: id, Kind: kind, StartedAt: kworLifecycleNowFn().Unix()})
	}
	if err != nil {
		delete(kworLifecycleTasks, id)
		kworLifecycleTaskMu.Unlock()
		cancel()
		return nil, nil, err
	}
	kworLifecycleTaskMu.Unlock()
	return handle.ctx, handle, nil
}

func (h *KworManagedOperationHandle) ID() string {
	if h == nil {
		return ""
	}
	return h.id
}

func (h *KworManagedOperationHandle) Context() context.Context {
	if h == nil {
		return context.Background()
	}
	return h.ctx
}

func (h *KworManagedOperationHandle) SetProcess(pid int, pgid int) error {
	if h == nil || strings.TrimSpace(h.id) == "" {
		return nil
	}
	startTime, err := kworManagedOperationProcessIdentity(pid, h.id)
	if err != nil {
		return err
	}
	return updateKworManagedOperation(h.id, func(record *KworManagedOperationRecord) {
		record.PID = pid
		record.PGID = pgid
		record.ProcessStartTime = startTime
	})
}

func (h *KworManagedOperationHandle) SetSystemdUnit(unit string) error {
	if h == nil || strings.TrimSpace(h.id) == "" {
		return nil
	}
	return updateKworManagedOperation(h.id, func(record *KworManagedOperationRecord) {
		record.Unit = strings.TrimSpace(unit)
	})
}

func (h *KworManagedOperationHandle) SetBlockNewWork(block bool) error {
	if h == nil || strings.TrimSpace(h.id) == "" {
		return nil
	}
	return updateKworManagedOperation(h.id, func(record *KworManagedOperationRecord) {
		record.BlockNewWork = block
		if block {
			record.BlockingSince = kworLifecycleNowFn().Unix()
		} else {
			record.BlockingSince = 0
		}
	})
}

func (h *KworManagedOperationHandle) TrackCommand(cmd *exec.Cmd) error {
	if h == nil || cmd == nil || cmd.Process == nil {
		return nil
	}
	return h.SetProcess(cmd.Process.Pid, kworManagedOperationProcessGroup(cmd.Process.Pid))
}

// TrackKworManagedCommandContext 将已启动命令登记到其所属受管操作。调用方
// 应在 Start 之前先调用 PrepareKworManagedCommand，避免子进程逃离登记的进程组。
func TrackKworManagedCommandContext(ctx context.Context, cmd *exec.Cmd) error {
	if ctx == nil {
		return nil
	}
	handle, _ := ctx.Value(kworManagedOperationContextKey{}).(*KworManagedOperationHandle)
	if handle == nil {
		return nil
	}
	return handle.TrackCommand(cmd)
}

func PrepareKworManagedCommand(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	prepareKworManagedCommand(cmd)
}

// PrepareKworManagedCommandContext also injects the operation token inherited
// by the child. A persisted PID is never signalled unless both its Linux start
// time and this token still match.
func PrepareKworManagedCommandContext(ctx context.Context, cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	prepareKworManagedCommand(cmd)
	if ctx == nil {
		return
	}
	handle, _ := ctx.Value(kworManagedOperationContextKey{}).(*KworManagedOperationHandle)
	if handle == nil || strings.TrimSpace(handle.id) == "" {
		return
	}
	cmd.Env = setKworCommandEnvironment(cmd.Environ(), kworLifecycleOperationIDEnv, handle.id)
}

func setKworCommandEnvironment(environment []string, key string, value string) []string {
	key = strings.TrimSpace(key)
	if key == "" {
		return environment
	}
	prefix := key + "="
	result := make([]string, 0, len(environment)+1)
	for _, entry := range environment {
		if strings.HasPrefix(entry, prefix) {
			continue
		}
		result = append(result, entry)
	}
	return append(result, prefix+value)
}

func (h *KworManagedOperationHandle) Done() {
	if h == nil {
		return
	}
	h.once.Do(func() {
		h.cancel()
		kworLifecycleTaskMu.Lock()
		delete(kworLifecycleTasks, h.id)
		kworLifecycleTaskMu.Unlock()
		if h.persistent {
			_ = removeKworManagedOperation(h.id)
		}
		close(h.done)
	})
}

// Detach 仅解除当前面板进程对操作的内存引用，保留运行时登记。它用于由
// 独立 worker 接手的任务；worker 完成后会通过 FinishKworManagedOperation
// 删除登记，卸载则仍能读取并终止该 worker。
func (h *KworManagedOperationHandle) Detach() {
	if h == nil {
		return
	}
	h.detachOnce.Do(func() {
		kworLifecycleTaskMu.Lock()
		if kworLifecycleTasks[h.id] == h {
			delete(kworLifecycleTasks, h.id)
		}
		kworLifecycleTaskMu.Unlock()
	})
}

// FinishKworManagedOperation 由独立 worker 在退出前调用。即使启动它的
// 面板进程已替换或退出，也会清除持久操作登记。
func FinishKworManagedOperation(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil
	}
	kworLifecycleTaskMu.Lock()
	handle := kworLifecycleTasks[id]
	kworLifecycleTaskMu.Unlock()
	if handle != nil {
		handle.Done()
		return nil
	}
	return removeKworManagedOperation(id)
}

func QuiesceKworManagedOperations() error {
	kworLifecycleTaskMu.Lock()
	kworLifecycleQuiescing = true
	handles := make([]*KworManagedOperationHandle, 0, len(kworLifecycleTasks))
	handlesByID := make(map[string]*KworManagedOperationHandle, len(kworLifecycleTasks))
	for _, handle := range kworLifecycleTasks {
		handles = append(handles, handle)
		if handle != nil && strings.TrimSpace(handle.id) != "" {
			handlesByID[handle.id] = handle
		}
	}
	kworLifecycleTaskMu.Unlock()

	for _, handle := range handles {
		if handle != nil && handle.cancel != nil {
			handle.cancel()
		}
	}
	operations, err := kworLifecycleLoadManagedOperationsFn()
	if err != nil {
		return err
	}
	for _, operation := range operations {
		if err := kworLifecycleTerminateManagedOperationFn(operation); err != nil {
			return err
		}
		// 仍由当前面板进程持有的任务必须等其工作函数真正收尾。若在
		// 超时时先删除登记，下一次卸载将无法知道这个任务曾经未退出。
		if _, inMemory := handlesByID[operation.ID]; inMemory {
			continue
		}
		if err := kworLifecycleRemoveManagedOperationFn(operation.ID); err != nil {
			return err
		}
	}
	deadline := time.NewTimer(kworLifecycleQuiesceTimeout)
	defer deadline.Stop()
	for _, handle := range handles {
		if handle == nil {
			continue
		}
		select {
		case <-handle.done:
		case <-deadline.C:
			return fmt.Errorf("受管后台任务未在取消后退出: %s", handle.id)
		}
	}
	return nil
}

func ResetKworLifecycleQuiescingForTest() {
	kworLifecycleTaskMu.Lock()
	kworLifecycleQuiescing = false
	kworLifecycleTaskMu.Unlock()
}

func upsertKworManagedOperation(record KworManagedOperationRecord) error {
	if runtime.GOOS != "linux" {
		return nil
	}
	record.ID = strings.TrimSpace(record.ID)
	record.Kind = strings.TrimSpace(record.Kind)
	if record.ID == "" || record.Kind == "" {
		return errors.New("受管后台任务登记不完整")
	}
	unlock, err := acquireKworLifecycleFileLock()
	if err != nil {
		return err
	}
	defer unlock()
	file, err := loadKworManagedOperationsLocked()
	if err != nil {
		return err
	}
	for index := range file.Operations {
		if file.Operations[index].ID == record.ID {
			file.Operations[index] = record
			return saveKworManagedOperationsLocked(file)
		}
	}
	file.Operations = append(file.Operations, record)
	return saveKworManagedOperationsLocked(file)
}

func updateKworManagedOperation(id string, update func(*KworManagedOperationRecord)) error {
	if runtime.GOOS != "linux" {
		return nil
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return nil
	}
	unlock, err := acquireKworLifecycleFileLock()
	if err != nil {
		return err
	}
	defer unlock()
	file, err := loadKworManagedOperationsLocked()
	if err != nil {
		return err
	}
	for index := range file.Operations {
		if file.Operations[index].ID != id {
			continue
		}
		update(&file.Operations[index])
		return saveKworManagedOperationsLocked(file)
	}
	return fmt.Errorf("受管后台任务不存在: %s", id)
}

func removeKworManagedOperation(id string) error {
	if runtime.GOOS != "linux" {
		return nil
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return nil
	}
	unlock, err := acquireKworLifecycleFileLock()
	if err != nil {
		return err
	}
	defer unlock()
	file, err := loadKworManagedOperationsLocked()
	if err != nil {
		return err
	}
	kept := file.Operations[:0]
	for _, operation := range file.Operations {
		if operation.ID != id {
			kept = append(kept, operation)
		}
	}
	file.Operations = kept
	return saveKworManagedOperationsLocked(file)
}

func loadKworManagedOperations() ([]KworManagedOperationRecord, error) {
	if runtime.GOOS != "linux" {
		return nil, nil
	}
	unlock, err := acquireKworLifecycleFileLock()
	if err != nil {
		return nil, err
	}
	defer unlock()
	file, err := loadKworManagedOperationsLocked()
	if err != nil {
		return nil, err
	}
	return append([]KworManagedOperationRecord(nil), file.Operations...), nil
}

func loadKworManagedOperationsLocked() (kworManagedOperationsFile, error) {
	raw, err := os.ReadFile(kworManagedOperationsPath())
	if errors.Is(err, os.ErrNotExist) {
		return kworManagedOperationsFile{Version: kworLifecycleStateVersion, Operations: []KworManagedOperationRecord{}}, nil
	}
	if err != nil {
		return kworManagedOperationsFile{}, err
	}
	file := kworManagedOperationsFile{}
	if err := json.Unmarshal(raw, &file); err != nil {
		return kworManagedOperationsFile{}, fmt.Errorf("解析受管后台任务登记失败: %w", err)
	}
	if file.Version != kworLifecycleStateVersion {
		return kworManagedOperationsFile{}, fmt.Errorf("不支持的受管后台任务登记版本: %d", file.Version)
	}
	file.Operations = normalizeKworManagedOperations(file.Operations)
	return file, nil
}

func saveKworManagedOperationsLocked(file kworManagedOperationsFile) error {
	file.Version = kworLifecycleStateVersion
	file.Operations = normalizeKworManagedOperations(file.Operations)
	return writeKworLifecycleJSON(kworManagedOperationsPath(), file)
}

func normalizeKworManagedOperations(records []KworManagedOperationRecord) []KworManagedOperationRecord {
	seen := make(map[string]struct{}, len(records))
	result := make([]KworManagedOperationRecord, 0, len(records))
	for _, record := range records {
		record.ID = strings.TrimSpace(record.ID)
		record.Kind = strings.TrimSpace(record.Kind)
		record.Unit = strings.TrimSpace(record.Unit)
		if record.ID == "" || record.Kind == "" {
			continue
		}
		if _, exists := seen[record.ID]; exists {
			continue
		}
		seen[record.ID] = struct{}{}
		result = append(result, record)
	}
	sort.Slice(result, func(left, right int) bool { return result[left].ID < result[right].ID })
	return result
}

func writeKworLifecycleJSON(path string, value any) error {
	if runtime.GOOS != "linux" {
		return nil
	}
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o750); err != nil {
		return err
	}
	if err := os.Chmod(directory, 0o750); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	temporary := path + ".tmp"
	file, err := os.OpenFile(temporary, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	_, writeErr := file.Write(append(raw, '\n'))
	syncErr := file.Sync()
	closeErr := file.Close()
	if writeErr != nil {
		_ = os.Remove(temporary)
		return writeErr
	}
	if syncErr != nil {
		_ = os.Remove(temporary)
		return syncErr
	}
	if closeErr != nil {
		_ = os.Remove(temporary)
		return closeErr
	}
	if err := os.Chmod(temporary, 0o600); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	if err := os.Rename(temporary, path); err != nil {
		_ = os.Remove(temporary)
		return err
	}
	return syncKworLifecycleDirectory(directory)
}
