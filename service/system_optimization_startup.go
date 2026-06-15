package service

import (
	"runtime"
	"strings"

	"github.com/alireza0/s-ui/util/common"
)

// ReconcileSystemOptimizationOnStartup enforces startup lock policy for managed
// sysctl and journald files based on their switches.
func ReconcileSystemOptimizationOnStartup() error {
	if runtime.GOOS != "linux" {
		return nil
	}

	errs := make([]string, 0, 2)

	if err := (&SystemSysctlOptimizationService{}).ReconcileOnStartup(); err != nil {
		errs = append(errs, "sysctl: "+strings.TrimSpace(err.Error()))
	}
	if err := (&SystemLogOptimizationService{}).ReconcileOnStartup(); err != nil {
		errs = append(errs, "journald: "+strings.TrimSpace(err.Error()))
	}

	if len(errs) == 0 {
		return nil
	}
	return common.NewError("系统优化启动巡检失败: ", strings.Join(errs, " | "))
}
