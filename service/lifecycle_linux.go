//go:build linux

package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

const kworLifecycleControlSocketName = "lifecycle.sock"

const kworLifecycleMetadataLockName = "metadata.lock"

type kworLifecycleControlRequest struct {
	Command string `json:"command"`
}

type kworLifecycleControlResponse struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

var kworLifecycleControlServer = struct {
	sync.Mutex
	listener *net.UnixListener
}{}

func acquireKworLifecycleMetadataLock() (func() error, error) {
	directory := KworLifecycleRuntimeDir()
	if err := os.MkdirAll(directory, 0o750); err != nil {
		return nil, fmt.Errorf("创建生命周期元数据锁目录失败: %w", err)
	}
	if err := os.Chmod(directory, 0o750); err != nil {
		return nil, fmt.Errorf("设置生命周期元数据锁目录权限失败: %w", err)
	}
	path := filepath.Join(directory, kworLifecycleMetadataLockName)
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("打开生命周期元数据锁失败: %w", err)
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("设置生命周期元数据锁权限失败: %w", err)
	}
	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("取得生命周期元数据锁失败: %w", err)
	}
	var once sync.Once
	return func() error {
		var result error
		once.Do(func() {
			if err := unix.Flock(int(file.Fd()), unix.LOCK_UN); err != nil {
				result = err
			}
			if err := file.Close(); err != nil && result == nil {
				result = err
			}
		})
		return result
	}, nil
}

func kworLifecycleControlSocketPath() string {
	return filepath.Join(KworLifecycleRuntimeDir(), kworLifecycleControlSocketName)
}

func StartKworLifecycleControlServer() error {
	kworLifecycleControlServer.Lock()
	defer kworLifecycleControlServer.Unlock()
	if kworLifecycleControlServer.listener != nil {
		return nil
	}
	if err := os.MkdirAll(KworLifecycleRuntimeDir(), 0o750); err != nil {
		return fmt.Errorf("创建生命周期控制目录失败: %w", err)
	}
	socketPath := kworLifecycleControlSocketPath()
	if info, err := os.Lstat(socketPath); err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return fmt.Errorf("生命周期控制 socket 路径不是 socket: %s", socketPath)
		}
		connection, dialErr := net.DialTimeout("unix", socketPath, 250*time.Millisecond)
		if dialErr == nil {
			_ = connection.Close()
			return nil
		}
		if err := os.Remove(socketPath); err != nil {
			return fmt.Errorf("删除失效的生命周期控制 socket 失败: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: socketPath, Net: "unix"})
	if err != nil {
		return fmt.Errorf("创建生命周期控制 socket 失败: %w", err)
	}
	if err := os.Chmod(socketPath, 0o600); err != nil {
		_ = listener.Close()
		_ = os.Remove(socketPath)
		return fmt.Errorf("设置生命周期控制 socket 权限失败: %w", err)
	}
	kworLifecycleControlServer.listener = listener
	go serveKworLifecycleControl(listener)
	return nil
}

func StopKworLifecycleControlServer() {
	kworLifecycleControlServer.Lock()
	listener := kworLifecycleControlServer.listener
	kworLifecycleControlServer.listener = nil
	kworLifecycleControlServer.Unlock()
	if listener != nil {
		_ = listener.Close()
	}
	_ = os.Remove(kworLifecycleControlSocketPath())
}

func serveKworLifecycleControl(listener *net.UnixListener) {
	for {
		connection, err := listener.AcceptUnix()
		if err != nil {
			return
		}
		go handleKworLifecycleControlConnection(connection)
	}
}

func handleKworLifecycleControlConnection(connection *net.UnixConn) {
	defer connection.Close()
	_ = connection.SetDeadline(time.Now().Add(8 * time.Second))
	request := kworLifecycleControlRequest{}
	if err := json.NewDecoder(connection).Decode(&request); err != nil {
		_ = json.NewEncoder(connection).Encode(kworLifecycleControlResponse{Error: "无效的生命周期控制请求"})
		return
	}
	if strings.TrimSpace(request.Command) != "quiesce" {
		_ = json.NewEncoder(connection).Encode(kworLifecycleControlResponse{Error: "不支持的生命周期控制命令"})
		return
	}
	if err := QuiesceKworManagedOperations(); err != nil {
		_ = json.NewEncoder(connection).Encode(kworLifecycleControlResponse{Error: err.Error()})
		return
	}
	_ = json.NewEncoder(connection).Encode(kworLifecycleControlResponse{OK: true})
}

func RequestKworLifecycleQuiesce() error {
	socketPath := kworLifecycleControlSocketPath()
	connection, err := net.DialTimeout("unix", socketPath, 2*time.Second)
	if isKworLifecycleSocketUnavailable(err) {
		if err := removeStaleKworLifecycleSocket(socketPath); err != nil {
			return err
		}
		// 旧版本面板没有控制 socket。此时仍会由卸载 worker 处理登记的
		// 进程组和 transient unit，避免兼容路径重新把数据删在进程之前。
		return QuiesceKworManagedOperations()
	}
	if err != nil {
		return fmt.Errorf("连接面板生命周期控制 socket 失败: %w", err)
	}
	defer connection.Close()
	_ = connection.SetDeadline(time.Now().Add(10 * time.Second))
	if err := json.NewEncoder(connection).Encode(kworLifecycleControlRequest{Command: "quiesce"}); err != nil {
		return fmt.Errorf("发送停止后台任务请求失败: %w", err)
	}
	response := kworLifecycleControlResponse{}
	if err := json.NewDecoder(connection).Decode(&response); err != nil {
		return fmt.Errorf("读取停止后台任务响应失败: %w", err)
	}
	if !response.OK {
		return fmt.Errorf("面板未能停止后台任务: %s", strings.TrimSpace(response.Error))
	}
	return nil
}

func isKworLifecycleSocketUnavailable(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, os.ErrNotExist) || errors.Is(err, syscall.ECONNREFUSED) ||
		errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.ENOTCONN) {
		return true
	}
	lower := strings.ToLower(fmt.Sprint(err))
	return strings.Contains(lower, "no such file") || strings.Contains(lower, "connection refused")
}

func removeStaleKworLifecycleSocket(socketPath string) error {
	info, err := os.Lstat(socketPath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("检查生命周期控制 socket 失败: %w", err)
	}
	if info.Mode()&os.ModeSocket == 0 {
		return fmt.Errorf("生命周期控制路径不可用且不是 socket: %s", socketPath)
	}
	if err := os.Remove(socketPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("删除失效的生命周期控制 socket 失败: %w", err)
	}
	return nil
}

func lifecycleUninstallWorkerAlive(state KworUninstallLifecycleState) bool {
	if state.WorkerPID > 0 {
		if state.WorkerStartTime > 0 {
			if startTime, err := kworLifecycleProcessStartTime(state.WorkerPID); err == nil {
				if startTime == state.WorkerStartTime {
					return true
				}
			} else if lifecyclePIDAlive(state.WorkerPID) {
				// A live PID whose start time cannot be read is not safe evidence
				// that the worker exited. Keep the reservation blocked.
				return true
			}
		} else if lifecyclePIDAlive(state.WorkerPID) {
			// Compatibility with v1 state written before process start-time
			// identity was available. It only blocks duplicate scheduling and is
			// never used as authority to signal the process.
			return true
		}
	}
	if strings.TrimSpace(state.WorkerUnit) == "" {
		return false
	}
	if !isKworSystemdHost() {
		return false
	}
	systemctl, err := exec.LookPath("systemctl")
	if err != nil {
		return true
	}
	active, err := systemdUnitActiveForUninstall(systemctl, state.WorkerUnit)
	if err != nil {
		return true
	}
	return active
}

func terminateKworManagedOperation(operation KworManagedOperationRecord) error {
	if unit := strings.TrimSpace(operation.Unit); unit != "" && isKworSystemdHost() {
		systemctl, err := exec.LookPath("systemctl")
		if err != nil {
			return fmt.Errorf("停止受管任务 %s 需要 systemctl: %w", operation.ID, err)
		}
		owned, identityErr := verifyKworPanelUpdateSystemdUnit(systemctl, unit, operation.ID, operation.ProcessStartTime > 0)
		if identityErr != nil {
			return fmt.Errorf("核验受管任务 unit %s 身份失败: %w", unit, identityErr)
		}
		if !owned {
			return fmt.Errorf("拒绝停止无法证明属于受管任务 %s 的 unit: %s", operation.ID, unit)
		}
		stopErr := exec.Command(systemctl, "stop", unit).Run()
		active, statusErr := systemdUnitActiveForUninstall(systemctl, unit)
		if statusErr != nil {
			return fmt.Errorf("核验受管任务 unit %s 停止状态失败: %w", unit, statusErr)
		}
		if active {
			if stopErr != nil {
				return fmt.Errorf("停止受管任务 unit %s 失败: %w", unit, stopErr)
			}
			return fmt.Errorf("受管任务 unit 仍在运行: %s", unit)
		}
	}
	processOwned, identityErr := kworManagedOperationProcessMatches(operation)
	if identityErr != nil {
		return fmt.Errorf("核验受管任务 %s 进程身份失败: %w", operation.ID, identityErr)
	}
	if !processOwned {
		// systemd-run 的 launcher 正常会先退出；unit 已按独立凭证停止后，
		// 不能再凭一个可能复用的旧 PGID 发信号。
		return nil
	}
	if operation.PGID > 0 {
		if operation.PGID == unix.Getpgrp() {
			return fmt.Errorf("拒绝终止与面板相同进程组的受管任务: %s", operation.ID)
		}
		if err := signalKworProcessGroup(operation.PGID, unix.SIGTERM); err != nil {
			return fmt.Errorf("终止受管任务进程组 %d 失败: %w", operation.PGID, err)
		}
		if waitKworManagedOperationExit(operation, 3*time.Second) {
			return nil
		}
		if matches, err := kworManagedOperationProcessMatches(operation); err != nil {
			return err
		} else if !matches {
			return nil
		}
		if err := signalKworProcessGroup(operation.PGID, unix.SIGKILL); err != nil {
			return fmt.Errorf("强制终止受管任务进程组 %d 失败: %w", operation.PGID, err)
		}
		if !waitKworManagedOperationExit(operation, 2*time.Second) {
			return fmt.Errorf("受管任务进程组仍在运行: %d", operation.PGID)
		}
		return nil
	}
	if operation.PID > 0 {
		if err := signalKworPID(operation.PID, unix.SIGTERM); err != nil {
			return fmt.Errorf("终止受管任务进程 %d 失败: %w", operation.PID, err)
		}
		if waitKworManagedOperationExit(operation, 3*time.Second) {
			return nil
		}
		if matches, err := kworManagedOperationProcessMatches(operation); err != nil {
			return err
		} else if !matches {
			return nil
		}
		if err := signalKworPID(operation.PID, unix.SIGKILL); err != nil {
			return fmt.Errorf("强制终止受管任务进程 %d 失败: %w", operation.PID, err)
		}
		if !waitKworManagedOperationExit(operation, 2*time.Second) {
			return fmt.Errorf("受管任务进程仍在运行: %d", operation.PID)
		}
	}
	return nil
}

func kworManagedOperationAlive(operation KworManagedOperationRecord) (bool, error) {
	if unit := strings.TrimSpace(operation.Unit); unit != "" && isKworSystemdHost() {
		systemctl, err := exec.LookPath("systemctl")
		if err != nil {
			return false, err
		}
		owned, err := verifyKworPanelUpdateSystemdUnit(systemctl, unit, operation.ID, strings.TrimSpace(operation.ID) != "")
		if err != nil {
			return false, err
		}
		if !owned {
			return false, fmt.Errorf("无法证明 systemd unit 属于受管任务: %s", unit)
		}
		active, err := systemdUnitActiveForUninstall(systemctl, unit)
		if err != nil {
			return false, err
		}
		if active {
			return true, nil
		}
	}
	return kworManagedOperationProcessMatches(operation)
}

func kworManagedOperationProcessMatches(operation KworManagedOperationRecord) (bool, error) {
	if operation.PID <= 0 {
		return false, nil
	}
	if operation.ProcessStartTime == 0 {
		if lifecyclePIDAlive(operation.PID) {
			return false, errors.New("持久任务缺少进程启动时间，不能安全发送信号")
		}
		return false, nil
	}
	startTime, err := kworLifecycleProcessStartTime(operation.PID)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if startTime != operation.ProcessStartTime {
		return false, nil
	}
	owned, err := kworProcessEnvironmentHasOperationID(operation.PID, operation.ID)
	if err != nil {
		return false, err
	}
	if !owned {
		return false, errors.New("进程环境中的 operation ID 不匹配")
	}
	return true, nil
}

func waitKworManagedOperationExit(operation KworManagedOperationRecord, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		matches, err := kworManagedOperationProcessMatches(operation)
		if err == nil && !matches {
			return true
		}
		time.Sleep(80 * time.Millisecond)
	}
	matches, err := kworManagedOperationProcessMatches(operation)
	return err == nil && !matches
}

func kworLifecycleProcessStartTime(pid int) (uint64, error) {
	if pid <= 0 {
		return 0, errors.New("进程 PID 无效")
	}
	raw, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "stat"))
	if err != nil {
		return 0, err
	}
	// comm is enclosed in parentheses and may itself contain spaces or ')'.
	end := strings.LastIndexByte(string(raw), ')')
	if end < 0 || end+1 >= len(raw) {
		return 0, errors.New("/proc stat 格式无效")
	}
	fields := strings.Fields(string(raw[end+1:]))
	// The suffix starts at field 3 (state); starttime is field 22.
	if len(fields) <= 19 {
		return 0, errors.New("/proc stat 缺少进程启动时间")
	}
	startTime, err := strconv.ParseUint(fields[19], 10, 64)
	if err != nil || startTime == 0 {
		if err == nil {
			err = errors.New("进程启动时间为零")
		}
		return 0, err
	}
	return startTime, nil
}

func kworProcessEnvironmentHasOperationID(pid int, operationID string) (bool, error) {
	raw, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "environ"))
	if err != nil {
		return false, err
	}
	want := kworLifecycleOperationIDEnv + "=" + strings.TrimSpace(operationID)
	for _, entry := range strings.Split(string(raw), "\x00") {
		if entry == want {
			return true, nil
		}
	}
	return false, nil
}

func kworManagedOperationProcessIdentity(pid int, operationID string) (uint64, error) {
	startTime, err := kworLifecycleProcessStartTime(pid)
	if err != nil {
		return 0, err
	}
	owned, err := kworProcessEnvironmentHasOperationID(pid, operationID)
	if err != nil {
		return 0, err
	}
	if !owned {
		return 0, errors.New("受管子进程未继承 operation ID")
	}
	return startTime, nil
}

func verifyKworPanelUpdateSystemdUnit(systemctlPath string, unit string, operationID string, requireToken bool) (bool, error) {
	unit = strings.TrimSpace(unit)
	if !panelUpdateSystemdUnitNamePattern.MatchString(unit) {
		return false, nil
	}
	output, err := runCommandOutputWithTimeout(
		shortSystemCommandTimeout,
		systemctlPath,
		"show",
		unit,
		"--property=LoadState",
		"--property=Environment",
		"--property=ExecStart",
		"--property=WorkingDirectory",
	)
	if err != nil {
		lower := strings.ToLower(output + " " + err.Error())
		if strings.Contains(lower, "not found") || strings.Contains(lower, "not loaded") {
			return true, nil
		}
		return false, err
	}
	values := make(map[string]string)
	for _, line := range strings.Split(output, "\n") {
		key, value, found := strings.Cut(strings.TrimSpace(line), "=")
		if found {
			values[key] = strings.TrimSpace(value)
		}
	}
	if strings.EqualFold(values["LoadState"], "not-found") {
		return true, nil
	}
	wantToken := kworLifecycleOperationIDEnv + "=" + strings.TrimSpace(operationID)
	hasToken := false
	for _, field := range strings.Fields(values["Environment"]) {
		if strings.Trim(field, `"'`) == wantToken {
			hasToken = true
			break
		}
	}
	if requireToken && !hasToken {
		return false, nil
	}
	scriptPath := panelUpdateScriptPathFromCommandText(values["ExecStart"])
	if scriptPath == "" || !panelUpdateWorkspaceScriptOwned(scriptPath) {
		return false, nil
	}
	workingDirectory := filepath.Clean(strings.TrimSpace(values["WorkingDirectory"]))
	if workingDirectory == "" || workingDirectory == "." || workingDirectory != filepath.Dir(scriptPath) {
		return false, nil
	}
	return hasToken || !requireToken, nil
}

func clearKworLifecycleRuntimeArtifactsLocked() error {
	allowed := map[string]struct{}{
		kworLifecycleStateFileName:               {},
		kworLifecycleOperationsFileName:          {},
		kworLifecycleStateFileName + ".tmp":      {},
		kworLifecycleOperationsFileName + ".tmp": {},
		kworLifecycleControlSocketName:           {},
		kworLifecycleMetadataLockName:            {},
		"lifecycle.lock":                         {},
	}
	if entries, err := os.ReadDir(KworLifecycleRuntimeDir()); err == nil {
		for _, entry := range entries {
			if _, ok := allowed[entry.Name()]; !ok {
				return fmt.Errorf("生命周期运行目录包含未识别文件: %s", filepath.Join(KworLifecycleRuntimeDir(), entry.Name()))
			}
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	operations, err := loadKworManagedOperationsLocked()
	if err != nil {
		return err
	}
	if len(operations.Operations) > 0 {
		return fmt.Errorf("受管任务登记仍包含 %d 个任务", len(operations.Operations))
	}
	for _, path := range []string{
		KworUninstallLifecycleStatePath(),
		kworManagedOperationsPath(),
		KworUninstallLifecycleStatePath() + ".tmp",
		kworManagedOperationsPath() + ".tmp",
	} {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	socketPath := kworLifecycleControlSocketPath()
	if info, err := os.Lstat(socketPath); err == nil {
		if info.Mode()&os.ModeSocket == 0 {
			return fmt.Errorf("拒绝删除非 socket 生命周期控制路径: %s", socketPath)
		}
		if err := os.Remove(socketPath); err != nil {
			return err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return syncKworLifecycleDirectory(KworLifecycleRuntimeDir())
}

func finishKworLifecycleRuntimeCleanup() error {
	directory := KworLifecycleRuntimeDir()
	for _, name := range []string{kworLifecycleMetadataLockName, "lifecycle.lock"} {
		path := filepath.Join(directory, name)
		if info, err := os.Lstat(path); err == nil {
			if !info.Mode().IsRegular() {
				return fmt.Errorf("拒绝删除非普通生命周期锁文件: %s", path)
			}
			if err := os.Remove(path); err != nil {
				return err
			}
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	entries, err := os.ReadDir(directory)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(entries) != 0 {
		return fmt.Errorf("生命周期运行目录仍包含未识别文件: %s", directory)
	}
	if err := os.Remove(directory); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return syncKworLifecycleDirectory(filepath.Dir(directory))
}

func signalKworPID(pid int, signal unix.Signal) error {
	err := unix.Kill(pid, signal)
	if errors.Is(err, unix.ESRCH) {
		return nil
	}
	return err
}

func signalKworProcessGroup(pgid int, signal unix.Signal) error {
	err := unix.Kill(-pgid, signal)
	if errors.Is(err, unix.ESRCH) {
		return nil
	}
	return err
}

func stopStartedKworDetachedWorker(pid int, pgid int, startTime uint64) error {
	matches := func() bool {
		current, err := kworLifecycleProcessStartTime(pid)
		return err == nil && current == startTime
	}
	if !matches() {
		return nil
	}
	if pgid <= 0 || pgid == unix.Getpgrp() {
		return fmt.Errorf("独立 worker 进程组无效: %d", pgid)
	}
	if err := signalKworProcessGroup(pgid, unix.SIGTERM); err != nil {
		return err
	}
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) && matches() {
		time.Sleep(80 * time.Millisecond)
	}
	if !matches() {
		return nil
	}
	if err := signalKworProcessGroup(pgid, unix.SIGKILL); err != nil {
		return err
	}
	deadline = time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && matches() {
		time.Sleep(80 * time.Millisecond)
	}
	if matches() {
		return fmt.Errorf("独立 worker 仍在运行: %d", pid)
	}
	return nil
}

func lifecyclePIDAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := unix.Kill(pid, 0)
	return err == nil || errors.Is(err, unix.EPERM)
}

func lifecycleProcessGroupAlive(pgid int) bool {
	if pgid <= 0 {
		return false
	}
	err := unix.Kill(-pgid, 0)
	return err == nil || errors.Is(err, unix.EPERM)
}

func waitKworPIDExit(pid int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !lifecyclePIDAlive(pid) {
			return true
		}
		time.Sleep(80 * time.Millisecond)
	}
	return !lifecyclePIDAlive(pid)
}

func waitKworProcessGroupExit(pgid int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if !lifecycleProcessGroupAlive(pgid) {
			return true
		}
		time.Sleep(80 * time.Millisecond)
	}
	return !lifecycleProcessGroupAlive(pgid)
}

func syncKworLifecycleDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

func kworManagedOperationProcessGroup(pid int) int {
	if pid <= 0 {
		return 0
	}
	pgid, err := syscall.Getpgid(pid)
	if err != nil {
		return 0
	}
	return pgid
}

func prepareKworManagedCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}
