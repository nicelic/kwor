package service

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/alireza0/s-ui/logger"
)

const (
	KernelRebootConfirmedArgument = "--confirmed-from-panel"
	kernelRebootWorkerSubcommand  = "kernel-reboot-worker"
	kernelRebootCommandTimeout    = 12 * time.Second
	kernelRebootWorkerDelay       = 1500 * time.Millisecond
)

var errKernelRebootSystemdWorkerUnavailable = errors.New("kernel reboot systemd worker unavailable")

var (
	kernelBuildRebootPlanFn             = buildKernelRebootPlan
	kernelScheduleRebootWorkerFn        = scheduleKernelRebootWorker
	kernelStartRebootWorkerFn           = startKernelRebootWorker
	kernelStartRebootSystemdWorkerFn    = startKernelRebootSystemdWorker
	kernelStartRebootDirectWorkerFn     = startKernelRebootDirectWorker
	kernelResolveRebootExecutableFn     = resolveKernelRebootExecutablePath
	kernelResolveOperationalSystemctlFn = resolveKernelOperationalSystemctlPath
	kernelRebootExecutableFn            = os.Executable
	kernelRebootEvalSymlinksFn          = filepath.EvalSymlinks
	kernelRebootSystemdRunLookPathFn    = exec.LookPath
	kernelRebootSleepFn                 = time.Sleep
	kernelRebootNowFn                   = time.Now
)

type kernelRebootAttempt struct {
	label   string
	command string
	args    []string
}

type kernelRebootPlan struct {
	attempts []kernelRebootAttempt
}

func scheduleKernelRebootWorker() error {
	if _, err := kernelBuildRebootPlanFn(); err != nil {
		return err
	}
	return kernelStartRebootWorkerFn()
}

func startKernelRebootWorker() error {
	binaryPath, err := kernelResolveRebootExecutableFn()
	if err != nil {
		return err
	}

	args := []string{kernelRebootWorkerSubcommand, KernelRebootConfirmedArgument}
	if err := kernelStartRebootSystemdWorkerFn(binaryPath, args); err == nil {
		return nil
	} else if !errors.Is(err, errKernelRebootSystemdWorkerUnavailable) {
		logger.Warning("start kernel reboot systemd worker failed, fallback to detached process: ", err)
	}

	return kernelStartRebootDirectWorkerFn(binaryPath, args)
}

func startKernelRebootSystemdWorker(binaryPath string, args []string) error {
	if _, err := kernelResolveOperationalSystemctlFn(); err != nil {
		return fmt.Errorf("%w: %v", errKernelRebootSystemdWorkerUnavailable, err)
	}

	systemdRunPath, err := kernelRebootSystemdRunLookPathFn("systemd-run")
	if err != nil || strings.TrimSpace(systemdRunPath) == "" {
		if err != nil {
			return fmt.Errorf("%w: %v", errKernelRebootSystemdWorkerUnavailable, err)
		}
		return fmt.Errorf("%w: systemd-run command not found", errKernelRebootSystemdWorkerUnavailable)
	}

	unitName := fmt.Sprintf("kwor-kernel-reboot-%d", kernelRebootNowFn().UnixNano())
	systemdArgs := buildKernelRebootSystemdRunArgs(unitName, binaryPath, args)
	output, err := kernelRunCommandOutput(systemCommandTimeout, systemdRunPath, systemdArgs...)
	if err != nil {
		message := strings.TrimSpace(output)
		if message != "" {
			return fmt.Errorf("启动独立系统重启单元失败: %w: %s", err, message)
		}
		return fmt.Errorf("启动独立系统重启单元失败: %w", err)
	}

	return nil
}

func buildKernelRebootSystemdRunArgs(unitName string, binaryPath string, workerArgs []string) []string {
	args := []string{
		"--unit", unitName,
		"--collect",
		"--description", "kwor kernel reboot",
		"--working-directory=" + filepath.Dir(binaryPath),
		"--setenv=" + InternalSystemdCommandEnv + "=1",
		binaryPath,
	}
	return append(args, workerArgs...)
}

func resolveKernelRebootExecutablePath() (string, error) {
	execPath, err := kernelRebootExecutableFn()
	if err != nil {
		return "", fmt.Errorf("resolve kwor executable failed: %w", err)
	}
	execPath = strings.TrimSpace(execPath)
	if execPath == "" {
		return "", fmt.Errorf("resolve kwor executable failed: empty executable path")
	}

	realPath, err := kernelRebootEvalSymlinksFn(execPath)
	if err != nil || strings.TrimSpace(realPath) == "" {
		return execPath, nil
	}
	return realPath, nil
}

func resolveKernelOperationalSystemctlPath() (string, error) {
	if !pathEntryExists("/run/systemd/system") {
		return "", fmt.Errorf("当前环境没有运行中的 systemd 管理器")
	}

	systemctlPath, err := kernelLookPath("systemctl")
	if err != nil || strings.TrimSpace(systemctlPath) == "" {
		if err != nil {
			return "", fmt.Errorf("未找到 systemctl: %w", err)
		}
		return "", fmt.Errorf("未找到 systemctl")
	}

	output, err := kernelRunCommandOutput(5*time.Second, systemctlPath, "show", "--property=Version", "--value")
	if err != nil {
		return "", fmt.Errorf("无法连接 systemd 管理器: %w", err)
	}
	if strings.TrimSpace(output) == "" {
		return "", fmt.Errorf("systemd 管理器未返回版本信息")
	}

	return systemctlPath, nil
}

func RunKernelRebootWorker(confirmation string) error {
	if strings.TrimSpace(confirmation) != KernelRebootConfirmedArgument {
		return fmt.Errorf("拒绝执行未确认的系统重启 worker")
	}
	if err := kernelEnsureRebootRuntime(); err != nil {
		return err
	}

	plan, err := kernelBuildRebootPlanFn()
	if err != nil {
		return err
	}

	kernelRebootSleepFn(kernelRebootWorkerDelay)
	return executeKernelRebootPlan(plan)
}

func buildKernelRebootPlan() (*kernelRebootPlan, error) {
	plan := &kernelRebootPlan{}
	euid := kernelGeteuid()
	systemctlPath, _ := kernelResolveOperationalSystemctlFn()
	loginctlPath, _ := kernelResolveOptionalCommandPath("loginctl")
	rebootPath, _ := kernelResolveOptionalCommandPath("reboot")
	shutdownPath, _ := kernelResolveOptionalCommandPath("shutdown")
	initPath, _ := kernelResolveOptionalCommandPath("init")
	telinitPath, _ := kernelResolveOptionalCommandPath("telinit")

	if euid == 0 {
		plan.attempts = appendKernelRebootAttempt(plan.attempts, "systemctl reboot", systemctlPath, "--no-block", "reboot")
		plan.attempts = appendKernelRebootAttempt(plan.attempts, "loginctl reboot", loginctlPath, "reboot")
		plan.attempts = appendKernelRebootAttempt(plan.attempts, "reboot", rebootPath)
		plan.attempts = appendKernelRebootAttempt(plan.attempts, "shutdown -r now", shutdownPath, "-r", "now")
		plan.attempts = appendKernelRebootAttempt(plan.attempts, "init 6", initPath, "6")
		plan.attempts = appendKernelRebootAttempt(plan.attempts, "telinit 6", telinitPath, "6")
		if len(plan.attempts) == 0 {
			return nil, fmt.Errorf("未找到可用的 Linux 系统重启命令（systemctl/loginctl/reboot/shutdown/init/telinit）")
		}
		return plan, nil
	}

	reasons := make([]string, 0, 3)

	if loginctlPath != "" {
		logindReady, reason := kernelLoginctlCanReboot(loginctlPath)
		if logindReady {
			plan.attempts = appendKernelRebootAttempt(plan.attempts, "loginctl reboot", loginctlPath, "reboot")
			plan.attempts = appendKernelRebootAttempt(plan.attempts, "systemctl reboot", systemctlPath, "--no-block", "reboot")
		} else if reason != "" {
			reasons = append(reasons, reason)
		}
	} else {
		reasons = append(reasons, "未找到 loginctl")
	}

	sudoPath, sudoReason := kernelPasswordlessSudoPath()
	if strings.TrimSpace(sudoPath) != "" {
		plan.attempts = appendKernelRebootAttempt(plan.attempts, "sudo systemctl reboot", sudoPath, "-n", systemctlPath, "--no-block", "reboot")
		plan.attempts = appendKernelRebootAttempt(plan.attempts, "sudo loginctl reboot", sudoPath, "-n", loginctlPath, "reboot")
		plan.attempts = appendKernelRebootAttempt(plan.attempts, "sudo reboot", sudoPath, "-n", rebootPath)
		plan.attempts = appendKernelRebootAttempt(plan.attempts, "sudo shutdown -r now", sudoPath, "-n", shutdownPath, "-r", "now")
		plan.attempts = appendKernelRebootAttempt(plan.attempts, "sudo init 6", sudoPath, "-n", initPath, "6")
		plan.attempts = appendKernelRebootAttempt(plan.attempts, "sudo telinit 6", sudoPath, "-n", telinitPath, "6")
	} else if sudoReason != "" {
		reasons = append(reasons, sudoReason)
	}

	if len(plan.attempts) == 0 {
		message := "当前运行账户无法无交互重启 Linux 系统"
		if len(reasons) > 0 {
			message += "：" + strings.Join(reasons, "；")
		}
		return nil, fmt.Errorf("%s", message)
	}

	return plan, nil
}

func kernelResolveOptionalCommandPath(name string) (string, error) {
	path, err := kernelLookPath(name)
	if err != nil {
		return "", err
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("%s command not found", name)
	}
	return path, nil
}

func kernelLoginctlCanReboot(loginctlPath string) (bool, string) {
	if strings.TrimSpace(loginctlPath) == "" {
		return false, ""
	}

	output, err := kernelRunCommandOutput(5*time.Second, loginctlPath, "can-reboot")
	if err != nil {
		return false, fmt.Sprintf("loginctl 无法确认重启授权：%s", strings.TrimSpace(err.Error()))
	}

	switch strings.ToLower(strings.TrimSpace(output)) {
	case "yes":
		return true, ""
	case "challenge":
		return false, "loginctl 重启需要交互认证，网页内无法完成"
	case "no":
		return false, "logind 不允许当前运行账户重启系统"
	default:
		trimmed := strings.TrimSpace(output)
		if trimmed == "" {
			return false, "loginctl 未返回可用的重启授权结果"
		}
		return false, fmt.Sprintf("loginctl can-reboot 返回了不可用结果 %q", trimmed)
	}
}

func kernelPasswordlessSudoPath() (string, string) {
	sudoPath, err := kernelResolveOptionalCommandPath("sudo")
	if err != nil {
		return "", "未找到 sudo"
	}

	truePath, trueErr := kernelResolveOptionalCommandPath("true")
	if trueErr != nil {
		return "", fmt.Sprintf("无法校验 sudo -n：%s", strings.TrimSpace(trueErr.Error()))
	}

	if err := kernelRunCommandWithTimeout(5*time.Second, sudoPath, "-n", truePath); err != nil {
		return "", "sudo -n 没有可用的免密授权"
	}

	return sudoPath, ""
}

func appendKernelRebootAttempt(attempts []kernelRebootAttempt, label string, command string, args ...string) []kernelRebootAttempt {
	command = strings.TrimSpace(command)
	if command == "" {
		return attempts
	}

	filteredArgs := make([]string, 0, len(args))
	for _, arg := range args {
		if strings.TrimSpace(arg) == "" {
			return attempts
		}
		filteredArgs = append(filteredArgs, arg)
	}

	signature := command + "\x00" + strings.Join(filteredArgs, "\x00")
	for _, item := range attempts {
		if item.command+"\x00"+strings.Join(item.args, "\x00") == signature {
			return attempts
		}
	}

	return append(attempts, kernelRebootAttempt{
		label:   strings.TrimSpace(label),
		command: command,
		args:    filteredArgs,
	})
}

func executeKernelRebootPlan(plan *kernelRebootPlan) error {
	if plan == nil || len(plan.attempts) == 0 {
		return fmt.Errorf("没有可执行的 Linux 系统重启方案")
	}

	failures := make([]string, 0, len(plan.attempts))
	for _, attempt := range plan.attempts {
		err := kernelRunCommandWithTimeout(kernelRebootCommandTimeout, attempt.command, attempt.args...)
		if err == nil {
			logger.Info("kernel reboot accepted by ", attempt.label)
			return nil
		}
		detail := strings.TrimSpace(err.Error())
		if detail == "" {
			detail = "unknown error"
		}
		logger.Warning("kernel reboot attempt failed: ", attempt.label, " -> ", detail)
		failures = append(failures, fmt.Sprintf("%s: %s", attempt.label, detail))
	}

	return fmt.Errorf("所有 Linux 系统重启方式都失败了：%s", strings.Join(failures, " | "))
}
