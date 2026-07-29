//go:build linux

package service

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"syscall"

	"github.com/alireza0/s-ui/logger"
)

func startPanelUninstallWorker(binaryPath string, reservationID string) error {
	args := panelUninstallCommandArgs()

	if panelUninstallSystemdHostFn() {
		return startPanelUninstallSystemdWorker(binaryPath, reservationID)
	}

	// A new session is independent of the direct panel process and does not
	// depend on a distribution shipping external setsid/nohup utilities.
	cmd := newPanelUninstallDirectCommand(binaryPath, args)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动独立卸载进程失败: %v", err)
	}
	startTime, identityErr := kworLifecycleProcessStartTime(cmd.Process.Pid)
	if identityErr != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return fmt.Errorf("读取独立卸载进程身份失败: %w", identityErr)
	}
	pgid, pgidErr := syscall.Getpgid(cmd.Process.Pid)
	if pgidErr != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return fmt.Errorf("读取独立卸载进程组失败: %w", pgidErr)
	}
	if err := panelUninstallWorkerRecordFn(reservationID, cmd.Process.Pid, pgid, ""); err != nil {
		recordErr := fmt.Errorf("记录独立卸载进程失败: %w", err)
		if rollbackErr := stopStartedKworDetachedWorker(cmd.Process.Pid, pgid, startTime); rollbackErr != nil {
			combined := fmt.Errorf("%w；停止已启动卸载进程失败: %v", recordErr, rollbackErr)
			if stateErr := panelUninstallRollbackFailureFn(reservationID, cmd.Process.Pid, pgid, startTime, "", combined); stateErr != nil {
				return fmt.Errorf("%w；记录阻塞恢复状态失败: %v", combined, stateErr)
			}
			return combined
		}
		return recordErr
	}
	if err := cmd.Process.Release(); err != nil {
		logger.Warning("release panel uninstall worker process failed: ", err)
	}
	return nil
}

func newPanelUninstallDirectCommand(binaryPath string, args []string) *exec.Cmd {
	cmd := exec.Command(binaryPath, args...)
	cmd.Dir = filepath.Dir(binaryPath)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return cmd
}
