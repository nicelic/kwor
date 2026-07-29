package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// PanelUninstallConfirmedArgument is accepted only by the detached worker
// created after an authenticated panel confirmation.
const PanelUninstallConfirmedArgument = "--confirmed-from-panel"

type PanelUninstallService struct{}

type PanelUninstallResult struct {
	Started    bool   `json:"started"`
	BinaryPath string `json:"binaryPath"`
	Message    string `json:"message"`
}

type PanelUninstallMode string

const (
	PanelUninstallModeNative      PanelUninstallMode = "native"
	PanelUninstallModeDockerGuide PanelUninstallMode = "docker-guide"
	PanelUninstallModeUnsupported PanelUninstallMode = "unsupported"
)

// PanelDockerUninstallCommand is intentionally a host-side instruction only.
// The container does not receive Docker daemon access and never deletes itself.
type PanelDockerUninstallCommand struct {
	ID      string `json:"id"`
	Command string `json:"command"`
}

// PanelUninstallStatus is a non-destructive status view used by the existing
// authenticated panel-update-status API. It exposes no uninstall control.
type PanelUninstallStatus struct {
	Mode           PanelUninstallMode            `json:"mode"`
	CanSchedule    bool                          `json:"canSchedule"`
	Hint           string                        `json:"hint,omitempty"`
	State          string                        `json:"state"`
	Phase          string                        `json:"phase,omitempty"`
	Error          string                        `json:"error,omitempty"`
	Failures       []string                      `json:"failures,omitempty"`
	Warnings       []string                      `json:"warnings,omitempty"`
	CanRetry       bool                          `json:"canRetry"`
	DockerCommands []PanelDockerUninstallCommand `json:"dockerCommands,omitempty"`
}

var panelUninstallSchedule = struct {
	sync.Mutex
	scheduled bool
}{}

var panelUninstallPreflightFn = preflightPanelUninstall
var panelUninstallWorkerStarter = startPanelUninstallWorker
var panelUninstallSystemdHostFn = isPanelUninstallSystemdHost
var panelUninstallSystemdRunLookPathFn = exec.LookPath
var panelUninstallSystemdRunFn = runCommandOutputWithTimeout
var panelUninstallWorkerRecordFn = RecordPanelUninstallLifecycleWorker
var panelUninstallSystemdWorkerRollbackFn = stopStartedPanelUninstallSystemdWorker
var panelUninstallRollbackFailureFn = RecordPanelUninstallLifecycleRollbackFailure
var panelUninstallNowFn = time.Now

// Schedule starts a separate process which invokes the same CLI uninstall
// implementation used by an SSH session. It never performs cleanup itself.
func (s *PanelUninstallService) Schedule() (*PanelUninstallResult, error) {
	panelUninstallSchedule.Lock()
	defer panelUninstallSchedule.Unlock()

	if panelUninstallSchedule.scheduled {
		state, found, stateErr := LoadKworUninstallLifecycleState()
		if stateErr != nil {
			return nil, stateErr
		}
		if !found || state == nil || lifecycleUninstallTakeoverBlocked(*state, kworLifecycleNowFn(), lifecycleUninstallWorkerAlive(*state)) {
			return nil, fmt.Errorf("卸载任务已启动，请勿重复提交")
		}
		// 独立 worker 已退出或报告失败，不能让旧的进程内标记永久锁死
		// 网页入口；持久状态会在下一次提交时被新的 worker 接管。
		panelUninstallSchedule.scheduled = false
	}
	reservationID, reserved, err := ReservePanelUninstallLifecycle()
	if err != nil {
		return nil, err
	}
	if !reserved {
		return nil, fmt.Errorf("卸载任务已启动，请勿重复提交")
	}

	binaryPath, err := panelUninstallPreflightFn()
	if err != nil {
		MarkPanelUninstallLifecycleScheduleFailed(reservationID, err)
		return nil, err
	}
	if err := panelUninstallWorkerStarter(binaryPath, reservationID); err != nil {
		MarkPanelUninstallLifecycleScheduleFailed(reservationID, err)
		return nil, err
	}

	panelUninstallSchedule.scheduled = true
	return &PanelUninstallResult{
		Started:    true,
		BinaryPath: binaryPath,
		Message:    "卸载任务已启动，面板连接即将断开",
	}, nil
}

func PanelUninstallAvailable() (bool, string) {
	status := GetPanelUninstallStatus()
	return status.Mode == PanelUninstallModeNative && status.CanSchedule, status.Hint
}

// GetPanelUninstallStatus reports the detachable worker state without
// scheduling work. A failed worker is retryable once its recorded PID/unit is
// no longer alive; that keeps a failed browser request from permanently
// disabling the settings page.
func GetPanelUninstallStatus() PanelUninstallStatus {
	if RunningInsideDocker() {
		return PanelUninstallStatus{
			Mode:           PanelUninstallModeDockerGuide,
			Hint:           "Docker 部署必须在宿主机停止并删除容器；容器内不会访问 Docker daemon",
			State:          "idle",
			DockerCommands: DockerUninstallHostCommands(),
		}
	}
	if runtime.GOOS != "linux" || RunningInsideContainer() {
		return PanelUninstallStatus{
			Mode:  PanelUninstallModeUnsupported,
			Hint:  "面板内卸载仅支持原生 Linux 宿主机部署",
			State: "unsupported",
		}
	}

	status := PanelUninstallStatus{
		Mode:  PanelUninstallModeNative,
		State: "idle",
	}
	_, preflightErr := preflightPanelUninstall()
	state, found, err := LoadKworUninstallLifecycleState()
	if err != nil {
		status.State = kworUninstallStatusFailed
		status.Error = fmt.Sprintf("读取卸载运行状态失败: %v", err)
		status.Failures = []string{status.Error}
		status.Hint = status.Error
		status.CanSchedule = false
		return status
	}
	blocked := false
	if found && state != nil {
		status.State = strings.TrimSpace(state.Status)
		status.Phase = strings.TrimSpace(state.Phase)
		status.Error = strings.TrimSpace(state.Error)
		status.Failures = normalizeKworUninstallMessages(state.Failures)
		status.Warnings = normalizeKworUninstallMessages(state.Warnings)
		workerAlive := lifecycleUninstallWorkerAlive(*state)
		blocked = lifecycleUninstallTakeoverBlocked(*state, kworLifecycleNowFn(), workerAlive)
		if blocked {
			status.Hint = "卸载任务正在运行，请等待当前 worker 结束"
		}
	}
	if preflightErr != nil {
		status.Hint = preflightErr.Error()
		return status
	}
	status.CanSchedule = !blocked
	if status.State == kworUninstallStatusFailed {
		status.CanRetry = !blocked
		status.CanSchedule = status.CanRetry
		if status.Hint == "" && !status.CanRetry {
			status.Hint = "上一次卸载 worker 仍在运行，暂时不能重试"
		}
	}
	return status
}

// DockerUninstallHostCommands only addresses the data mount whose destination
// is /app/Promanager_data. It never removes the deployment directory itself.
func DockerUninstallHostCommands() []PanelDockerUninstallCommand {
	return []PanelDockerUninstallCommand{
		{
			ID: "compose",
			Command: "cd /path/to/kwor\n" +
				"docker compose down --remove-orphans\n" +
				"rm -rf -- ./Promanager_data",
		},
		{
			ID: "docker-run",
			Command: "container=kwor\n" +
				"mount_type=\"$(docker inspect -f '{{range .Mounts}}{{if eq .Destination \"/app/Promanager_data\"}}{{.Type}}{{end}}{{end}}' \"$container\")\"\n" +
				"mount_name=\"$(docker inspect -f '{{range .Mounts}}{{if eq .Destination \"/app/Promanager_data\"}}{{.Name}}{{end}}{{end}}' \"$container\")\"\n" +
				"mount_source=\"$(docker inspect -f '{{range .Mounts}}{{if eq .Destination \"/app/Promanager_data\"}}{{.Source}}{{end}}{{end}}' \"$container\")\"\n" +
				"docker rm -f \"$container\"\n" +
				"case \"$mount_type\" in\n" +
				"  volume) [ -n \"$mount_name\" ] && docker volume rm \"$mount_name\" ;;\n" +
				"  bind) case \"$mount_source\" in ''|/|/opt|/usr|/var|/home|/root) printf '%s\\n' \"refusing unsafe bind mount: $mount_source\" >&2; exit 1 ;; esac; rm -rf -- \"$mount_source\" ;;\n" +
				"  *) printf '%s\\n' \"no /app/Promanager_data mount found\" >&2; exit 1 ;;\n" +
				"esac",
		},
	}
}

func preflightPanelUninstall() (string, error) {
	if err := validatePanelUninstallEnvironment(runtime.GOOS == "linux", RunningInsideContainer(), os.Geteuid()); err != nil {
		return "", err
	}

	binaryPath, err := resolvePanelUninstallExecutable()
	if err != nil {
		return "", err
	}

	// On every systemd host, use a transient unit rather than inheriting the
	// panel service cgroup. This remains necessary even if `kwor.service`
	// momentarily reports an inactive state.
	if panelUninstallSystemdHostFn() {
		if _, err := exec.LookPath("systemd-run"); err != nil {
			return "", fmt.Errorf("systemd 面板卸载需要 systemd-run: %v", err)
		}
	}

	return binaryPath, nil
}

func isPanelUninstallSystemdHost() bool {
	return isKworSystemdHost()
}

func validatePanelUninstallEnvironment(isLinux bool, insideContainer bool, euid int) error {
	if !isLinux {
		return fmt.Errorf("面板内卸载仅支持 Linux 宿主机部署")
	}
	if insideContainer {
		return fmt.Errorf("Docker/容器部署请使用 Docker 或 Compose 卸载面板")
	}
	if euid != 0 {
		return fmt.Errorf("面板内卸载需要 root 权限")
	}
	return nil
}

func resolvePanelUninstallExecutable() (string, error) {
	binaryPath, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("无法解析当前面板可执行文件: %v", err)
	}
	if resolved, resolveErr := filepath.EvalSymlinks(binaryPath); resolveErr == nil {
		binaryPath = resolved
	}

	binaryPath = filepath.Clean(strings.TrimSpace(binaryPath))
	if binaryPath == "" || binaryPath == "." {
		return "", fmt.Errorf("当前面板可执行文件路径无效")
	}
	info, err := os.Stat(binaryPath)
	if err != nil {
		return "", fmt.Errorf("当前面板可执行文件不可用: %v", err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("当前面板路径不是可执行文件")
	}
	if info.Mode().Perm()&0o111 == 0 {
		return "", fmt.Errorf("当前面板可执行文件没有执行权限")
	}
	return binaryPath, nil
}

func panelUninstallCommandArgs() []string {
	return []string{"uninstall", PanelUninstallConfirmedArgument}
}

func buildPanelUninstallSystemdRunArgs(unitName string, binaryPath string) []string {
	args := []string{
		"--unit", unitName,
		"--collect",
		"--description", "kwor panel uninstall",
		binaryPath,
	}
	return append(args, panelUninstallCommandArgs()...)
}

func startPanelUninstallSystemdWorker(binaryPath string, reservationID string) error {
	if _, err := panelUninstallSystemdRunLookPathFn("systemd-run"); err != nil {
		return fmt.Errorf("systemd 面板卸载需要 systemd-run: %v", err)
	}

	unitName := fmt.Sprintf("kwor-panel-uninstall-%d", panelUninstallNowFn().UnixNano())
	systemdArgs := buildPanelUninstallSystemdRunArgs(unitName, binaryPath)
	output, err := panelUninstallSystemdRunFn(systemCommandTimeout, "systemd-run", systemdArgs...)
	if err != nil {
		message := strings.TrimSpace(output)
		if message != "" {
			return fmt.Errorf("启动独立卸载单元失败: %v: %s", err, message)
		}
		return fmt.Errorf("启动独立卸载单元失败: %v", err)
	}
	if err := panelUninstallWorkerRecordFn(reservationID, 0, 0, unitName); err != nil {
		recordErr := fmt.Errorf("记录独立卸载单元失败: %w", err)
		if rollbackErr := panelUninstallSystemdWorkerRollbackFn(unitName); rollbackErr != nil {
			combined := fmt.Errorf("%w；停止已启动卸载单元失败: %v", recordErr, rollbackErr)
			if stateErr := panelUninstallRollbackFailureFn(reservationID, 0, 0, 0, unitName, combined); stateErr != nil {
				return fmt.Errorf("%w；记录阻塞恢复状态失败: %v", combined, stateErr)
			}
			return combined
		}
		return recordErr
	}
	return nil
}

func stopStartedPanelUninstallSystemdWorker(unit string) error {
	return stopSystemdUnitForUninstall(unit, false)
}
