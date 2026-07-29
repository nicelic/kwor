package service

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/process"
)

type managedCoreCmd interface {
	Wait() error
}

type managedProcessIdentity struct {
	pid        int32
	createTime int64
	binaryPath string
}

func normalizeManagedCoreProcessPath(path string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		return ""
	}
	if runtime.GOOS != "windows" {
		path = strings.TrimSuffix(path, " (deleted)")
	}
	path = filepath.Clean(path)
	if resolved, err := filepath.EvalSymlinks(path); err == nil && strings.TrimSpace(resolved) != "" {
		path = filepath.Clean(resolved)
	}
	return path
}

func managedCoreProcessPathEquals(expected string, actual string) bool {
	expected = normalizeManagedCoreProcessPath(expected)
	actual = normalizeManagedCoreProcessPath(actual)
	if expected == "" || actual == "" {
		return false
	}
	if runtime.GOOS == "windows" {
		return strings.EqualFold(expected, actual)
	}
	return expected == actual
}

func findManagedCoreProcessesByBinaryPath(binPath string) ([]*process.Process, error) {
	expected := normalizeManagedCoreProcessPath(binPath)
	if expected == "" {
		return nil, nil
	}

	processes, err := process.Processes()
	if err != nil {
		return nil, err
	}

	matches := make([]*process.Process, 0, 2)
	for _, proc := range processes {
		if proc == nil {
			continue
		}
		if exe, err := proc.Exe(); err == nil && managedCoreProcessPathEquals(expected, exe) {
			matches = append(matches, proc)
			continue
		}
		if runtime.GOOS == "windows" {
			cmdline, err := proc.CmdlineSlice()
			if err != nil || len(cmdline) == 0 || !managedCoreProcessPathEquals(expected, cmdline[0]) {
				continue
			}
			matches = append(matches, proc)
		}
	}
	return matches, nil
}

func managedProcessMatchesBinaryPath(proc *process.Process, expected string) bool {
	if proc == nil {
		return false
	}
	if exe, err := proc.Exe(); err == nil && managedCoreProcessPathEquals(expected, exe) {
		return true
	}
	if runtime.GOOS == "windows" {
		cmdline, err := proc.CmdlineSlice()
		return err == nil && len(cmdline) > 0 && managedCoreProcessPathEquals(expected, cmdline[0])
	}
	return false
}

func captureManagedProcessIdentity(proc *process.Process, binaryPath string) (managedProcessIdentity, error) {
	if proc == nil || proc.Pid <= 0 {
		return managedProcessIdentity{}, fmt.Errorf("managed process is invalid")
	}
	createTime, err := proc.CreateTime()
	if err != nil || createTime <= 0 {
		if err == nil {
			err = fmt.Errorf("process create time is empty")
		}
		return managedProcessIdentity{}, err
	}
	if !managedProcessMatchesBinaryPath(proc, binaryPath) {
		return managedProcessIdentity{}, fmt.Errorf("managed process path changed before identity capture")
	}
	return managedProcessIdentity{pid: proc.Pid, createTime: createTime, binaryPath: normalizeManagedCoreProcessPath(binaryPath)}, nil
}

func managedProcessIdentityMatches(identity managedProcessIdentity) bool {
	if identity.pid <= 0 || identity.createTime <= 0 || identity.binaryPath == "" {
		return false
	}
	proc, err := process.NewProcess(identity.pid)
	if err != nil || !managedCoreProcessRunning(proc) {
		return false
	}
	createTime, err := proc.CreateTime()
	if err != nil || createTime != identity.createTime {
		return false
	}
	return managedProcessMatchesBinaryPath(proc, identity.binaryPath)
}

func managedCoreProcessRunning(proc *process.Process) bool {
	if proc == nil {
		return false
	}
	running, err := proc.IsRunning()
	if err != nil || !running {
		return false
	}
	statuses, err := proc.Status()
	if err != nil || len(statuses) == 0 {
		return true
	}
	for _, status := range statuses {
		if strings.EqualFold(strings.TrimSpace(status), "Z") {
			return false
		}
	}
	return true
}

func signalManagedProcessIdentity(identity managedProcessIdentity, force bool) error {
	if !managedProcessIdentityMatches(identity) {
		return nil
	}
	proc, err := process.NewProcess(identity.pid)
	if err != nil {
		return nil
	}
	if force {
		err = proc.Kill()
	} else {
		err = proc.Terminate()
	}
	if err != nil && managedProcessIdentityMatches(identity) {
		return err
	}
	return nil
}

func isManagedCoreProcessRunningByBinaryPath(binPath string) bool {
	processes, err := findManagedCoreProcessesByBinaryPath(binPath)
	return err == nil && len(processes) > 0
}

func terminateManagedCoreProcessesByBinaryPath(binPath string, gracefulTimeout time.Duration) error {
	return terminateManagedProcessesByBinaryPathExceptPIDs(binPath, gracefulTimeout, nil)
}

// TerminateManagedProcessesByBinaryPathExceptPIDs stops only processes whose
// executable (including Linux's " (deleted)" proc path) resolves to binPath.
// It is intentionally path-based so uninstall never kills a user-managed
// sing-box or mihomo instance merely because it has the same process name.
func TerminateManagedProcessesByBinaryPathExceptPIDs(binPath string, gracefulTimeout time.Duration, excludedPIDs []int) error {
	return terminateManagedProcessesByBinaryPathExceptPIDs(binPath, gracefulTimeout, excludedPIDs)
}

func ManagedProcessesRunningByBinaryPath(binPath string, excludedPIDs []int) bool {
	processes, err := findManagedCoreProcessesByBinaryPath(binPath)
	if err != nil {
		return false
	}
	excluded := make(map[int]struct{}, len(excludedPIDs))
	for _, pid := range excludedPIDs {
		if pid > 0 {
			excluded[pid] = struct{}{}
		}
	}
	for _, proc := range processes {
		if proc == nil {
			continue
		}
		if _, skip := excluded[int(proc.Pid)]; !skip {
			return true
		}
	}
	return false
}

func terminateManagedProcessesByBinaryPathExceptPIDs(binPath string, gracefulTimeout time.Duration, excludedPIDs []int) error {
	processes, err := findManagedCoreProcessesByBinaryPath(binPath)
	if err != nil {
		return err
	}
	excluded := make(map[int]struct{}, len(excludedPIDs))
	for _, pid := range excludedPIDs {
		if pid > 0 {
			excluded[pid] = struct{}{}
		}
	}
	if len(processes) == 0 {
		return nil
	}

	identities := make([]managedProcessIdentity, 0, len(processes))
	seen := make(map[int]struct{}, len(processes))
	for _, proc := range processes {
		pid := int(proc.Pid)
		if pid <= 0 {
			continue
		}
		if _, skip := excluded[pid]; skip {
			continue
		}
		if _, ok := seen[pid]; ok {
			continue
		}
		seen[pid] = struct{}{}
		identity, identityErr := captureManagedProcessIdentity(proc, binPath)
		if identityErr != nil {
			return fmt.Errorf("capture managed process %d identity: %w", pid, identityErr)
		}
		identities = append(identities, identity)
	}
	if len(identities) == 0 {
		return nil
	}

	for _, identity := range identities {
		if err := signalManagedProcessIdentity(identity, false); err != nil {
			return fmt.Errorf("terminate managed process %d: %w", identity.pid, err)
		}
	}

	deadline := time.Now().Add(gracefulTimeout)
	for time.Now().Before(deadline) {
		remaining := false
		for _, identity := range identities {
			if managedProcessIdentityMatches(identity) {
				remaining = true
				break
			}
		}
		if !remaining {
			return nil
		}
		time.Sleep(120 * time.Millisecond)
	}

	for _, identity := range identities {
		if !managedProcessIdentityMatches(identity) {
			continue
		}
		if err := signalManagedProcessIdentity(identity, true); err != nil {
			return fmt.Errorf("kill managed process %d: %w", identity.pid, err)
		}
	}

	stillAlive := make([]string, 0, len(identities))
	for _, identity := range identities {
		if managedProcessIdentityMatches(identity) {
			stillAlive = append(stillAlive, fmt.Sprintf("%d", identity.pid))
		}
	}
	if len(stillAlive) > 0 {
		return fmt.Errorf("managed core process is still alive after stop request: %s", strings.Join(stillAlive, ", "))
	}
	return nil
}

func managedCoreProcessPIDAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := process.NewProcess(int32(pid))
	if err != nil {
		return false
	}
	return managedCoreProcessRunning(proc)
}

func waitManagedCoreCommandAsync(startedCmd managedCoreCmd, onExit func()) {
	if startedCmd == nil {
		return
	}
	go func() {
		_ = startedCmd.Wait()
		if onExit != nil {
			onExit()
		}
	}()
}

func resolveManagedCoreDirectStdStreams() (*os.File, *os.File) {
	if runningInsideContainer() {
		if stdout, err := os.OpenFile("/proc/1/fd/1", os.O_WRONLY, 0); err == nil {
			if stderr, err := os.OpenFile("/proc/1/fd/2", os.O_WRONLY, 0); err == nil {
				return stdout, stderr
			} else {
				_ = stdout.Close()
			}
		}
	}
	return nil, nil
}

func closeManagedCoreDirectStdStreams(stdout *os.File, stderr *os.File) {
	if stdout != nil {
		_ = stdout.Close()
	}
	if stderr != nil && stderr != stdout {
		_ = stderr.Close()
	}
}
