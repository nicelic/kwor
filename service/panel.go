package service

import (
	"fmt"
	"os"
	"syscall"
	"time"

	"github.com/alireza0/s-ui/logger"
)

type PanelService struct {
}

func PanelRestartSupport() (bool, string) {
	switch GetSystemPlatformOS() {
	case "windows":
		return false, "Windows 部署不支持面板内重启，请通过 VS Code、服务管理器或启动脚本重启进程"
	case "linux":
		return true, ""
	default:
		return false, "当前平台状态尚未就绪，无法安全执行面板内重启"
	}
}

func (s *PanelService) RestartPanel(delay time.Duration) error {
	if supported, reason := PanelRestartSupport(); !supported {
		return fmt.Errorf("%s", reason)
	}
	p, err := os.FindProcess(syscall.Getpid())
	if err != nil {
		return err
	}
	go func() {
		time.Sleep(delay)
		if signalErr := p.Signal(syscall.SIGHUP); signalErr != nil {
			logger.Error("send signal SIGHUP failed:", signalErr)
		}
	}()
	return nil
}
