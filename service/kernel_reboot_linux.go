//go:build linux

package service

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"syscall"

	"github.com/alireza0/s-ui/logger"
)

func startKernelRebootDirectWorker(binaryPath string, args []string) error {
	cmd := newKernelRebootDirectCommand(binaryPath, args)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("启动独立系统重启进程失败: %v", err)
	}
	if err := cmd.Process.Release(); err != nil {
		logger.Warning("release kernel reboot worker process failed: ", err)
	}
	return nil
}

func newKernelRebootDirectCommand(binaryPath string, args []string) *exec.Cmd {
	cmd := exec.Command(binaryPath, args...)
	cmd.Dir = filepath.Dir(binaryPath)
	cmd.Env = setKworCommandEnvironment(cmd.Environ(), InternalSystemdCommandEnv, "1")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return cmd
}
