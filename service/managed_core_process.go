package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/shirou/gopsutil/v4/process"
	"golang.org/x/sys/cpu"
)

type managedCoreCmd interface {
	Wait() error
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
		if cmdline, err := proc.CmdlineSlice(); err == nil && len(cmdline) > 0 && managedCoreProcessPathEquals(expected, cmdline[0]) {
			matches = append(matches, proc)
		}
	}
	return matches, nil
}

func isManagedCoreProcessRunningByBinaryPath(binPath string) bool {
	processes, err := findManagedCoreProcessesByBinaryPath(binPath)
	return err == nil && len(processes) > 0
}

func terminateManagedCoreProcessesByBinaryPath(binPath string, gracefulTimeout time.Duration) error {
	processes, err := findManagedCoreProcessesByBinaryPath(binPath)
	if err != nil {
		return err
	}
	if len(processes) == 0 {
		return nil
	}

	pids := make([]int, 0, len(processes))
	seen := make(map[int]struct{}, len(processes))
	for _, proc := range processes {
		pid := int(proc.Pid)
		if pid <= 0 {
			continue
		}
		if _, ok := seen[pid]; ok {
			continue
		}
		seen[pid] = struct{}{}
		pids = append(pids, pid)
	}
	if len(pids) == 0 {
		return nil
	}

	if runtime.GOOS == "windows" {
		for _, pid := range pids {
			if err := exec.Command("taskkill", "/F", "/T", "/PID", fmt.Sprintf("%d", pid)).Run(); err != nil {
				return err
			}
		}
		return nil
	}

	for _, pid := range pids {
		if proc, err := os.FindProcess(pid); err == nil {
			_ = proc.Signal(os.Interrupt)
		}
	}

	deadline := time.Now().Add(gracefulTimeout)
	for time.Now().Before(deadline) {
		remaining := false
		for _, pid := range pids {
			if managedCoreProcessPIDAlive(pid) {
				remaining = true
				break
			}
		}
		if !remaining {
			return nil
		}
		time.Sleep(120 * time.Millisecond)
	}

	for _, pid := range pids {
		if !managedCoreProcessPIDAlive(pid) {
			continue
		}
		if proc, err := os.FindProcess(pid); err == nil {
			_ = proc.Kill()
		}
	}

	stillAlive := make([]string, 0, len(pids))
	for _, pid := range pids {
		if managedCoreProcessPIDAlive(pid) {
			stillAlive = append(stillAlive, fmt.Sprintf("%d", pid))
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

func inferHostAMD64Level() string {
	if runtime.GOARCH != "amd64" {
		return ""
	}
	if !(cpu.X86.HasCX16 &&
		cpu.X86.HasSSE3 &&
		cpu.X86.HasSSSE3 &&
		cpu.X86.HasSSE41 &&
		cpu.X86.HasSSE42 &&
		cpu.X86.HasPOPCNT) {
		return "v1"
	}
	if cpu.X86.HasAVX &&
		cpu.X86.HasAVX2 &&
		cpu.X86.HasBMI1 &&
		cpu.X86.HasBMI2 &&
		cpu.X86.HasFMA {
		return "v3"
	}
	return "v2"
}
