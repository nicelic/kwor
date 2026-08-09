package service

import (
	"errors"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestBuildKernelRebootPlanAllowsPasswordlessSudo(t *testing.T) {
	previousLookPath := kernelLookPath
	previousRun := kernelRunCommandWithTimeout
	previousOutput := kernelRunCommandOutput
	previousGeteuid := kernelGeteuid
	previousResolveSystemctl := kernelResolveOperationalSystemctlFn
	t.Cleanup(func() {
		kernelLookPath = previousLookPath
		kernelRunCommandWithTimeout = previousRun
		kernelRunCommandOutput = previousOutput
		kernelGeteuid = previousGeteuid
		kernelResolveOperationalSystemctlFn = previousResolveSystemctl
	})

	kernelGeteuid = func() int { return 1000 }
	kernelLookPath = func(file string) (string, error) {
		switch file {
		case "systemctl":
			return "/usr/bin/systemctl", nil
		case "reboot":
			return "/usr/sbin/reboot", nil
		case "shutdown":
			return "/usr/sbin/shutdown", nil
		case "init":
			return "/usr/sbin/init", nil
		case "telinit":
			return "/usr/sbin/telinit", nil
		case "sudo":
			return "/usr/bin/sudo", nil
		case "true":
			return "/usr/bin/true", nil
		case "loginctl":
			return "", errors.New("loginctl not found")
		default:
			return "", errors.New("unexpected lookup: " + file)
		}
	}
	kernelRunCommandWithTimeout = func(_ time.Duration, command string, args ...string) error {
		if command != "/usr/bin/sudo" || !reflect.DeepEqual(args, []string{"-n", "/usr/bin/true"}) {
			t.Fatalf("unexpected privileged preflight command: %s %#v", command, args)
		}
		return nil
	}
	kernelRunCommandOutput = func(_ time.Duration, command string, args ...string) (string, error) {
		t.Fatalf("unexpected output command: %s %#v", command, args)
		return "", nil
	}
	kernelResolveOperationalSystemctlFn = func() (string, error) { return "/usr/bin/systemctl", nil }

	plan, err := buildKernelRebootPlan()
	if err != nil {
		t.Fatalf("buildKernelRebootPlan() error = %v", err)
	}

	got := make([]string, 0, len(plan.attempts))
	for _, attempt := range plan.attempts {
		got = append(got, attempt.command+" "+strings.Join(attempt.args, " "))
	}
	want := []string{
		"/usr/bin/sudo -n /usr/bin/systemctl --no-block reboot",
		"/usr/bin/sudo -n /usr/sbin/reboot",
		"/usr/bin/sudo -n /usr/sbin/shutdown -r now",
		"/usr/bin/sudo -n /usr/sbin/init 6",
		"/usr/bin/sudo -n /usr/sbin/telinit 6",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("reboot attempts = %#v, want %#v", got, want)
	}
}

func TestBuildKernelRebootPlanRejectsUnauthorizedUser(t *testing.T) {
	previousLookPath := kernelLookPath
	previousRun := kernelRunCommandWithTimeout
	previousOutput := kernelRunCommandOutput
	previousGeteuid := kernelGeteuid
	previousResolveSystemctl := kernelResolveOperationalSystemctlFn
	t.Cleanup(func() {
		kernelLookPath = previousLookPath
		kernelRunCommandWithTimeout = previousRun
		kernelRunCommandOutput = previousOutput
		kernelGeteuid = previousGeteuid
		kernelResolveOperationalSystemctlFn = previousResolveSystemctl
	})

	kernelGeteuid = func() int { return 1000 }
	kernelLookPath = func(file string) (string, error) {
		switch file {
		case "systemctl":
			return "/usr/bin/systemctl", nil
		case "loginctl":
			return "/usr/bin/loginctl", nil
		case "sudo":
			return "/usr/bin/sudo", nil
		case "true":
			return "/usr/bin/true", nil
		default:
			return "", errors.New("unexpected lookup: " + file)
		}
	}
	kernelRunCommandWithTimeout = func(_ time.Duration, command string, args ...string) error {
		if command != "/usr/bin/sudo" || !reflect.DeepEqual(args, []string{"-n", "/usr/bin/true"}) {
			t.Fatalf("unexpected privileged preflight command: %s %#v", command, args)
		}
		return errors.New("sudo requires password")
	}
	kernelRunCommandOutput = func(_ time.Duration, command string, args ...string) (string, error) {
		switch command {
		case "/usr/bin/loginctl":
			if !reflect.DeepEqual(args, []string{"can-reboot"}) {
				t.Fatalf("unexpected loginctl args: %#v", args)
			}
			return "challenge", nil
		default:
			t.Fatalf("unexpected output command: %s %#v", command, args)
			return "", nil
		}
	}
	kernelResolveOperationalSystemctlFn = func() (string, error) { return "/usr/bin/systemctl", nil }

	_, err := buildKernelRebootPlan()
	if err == nil {
		t.Fatal("buildKernelRebootPlan() error = nil, want authorization failure")
	}
	if !strings.Contains(err.Error(), "无法无交互重启 Linux 系统") {
		t.Fatalf("buildKernelRebootPlan() error = %v, want Linux reboot authorization failure", err)
	}
}

func TestScheduleKernelRebootWorkerPreflightsBeforeStart(t *testing.T) {
	previousPlan := kernelBuildRebootPlanFn
	previousStart := kernelStartRebootWorkerFn
	t.Cleanup(func() {
		kernelBuildRebootPlanFn = previousPlan
		kernelStartRebootWorkerFn = previousStart
	})

	kernelBuildRebootPlanFn = func() (*kernelRebootPlan, error) {
		return nil, errors.New("preflight failed")
	}
	started := false
	kernelStartRebootWorkerFn = func() error {
		started = true
		return nil
	}

	err := scheduleKernelRebootWorker()
	if err == nil || !strings.Contains(err.Error(), "preflight failed") {
		t.Fatalf("scheduleKernelRebootWorker() error = %v", err)
	}
	if started {
		t.Fatal("scheduleKernelRebootWorker() started worker despite failed preflight")
	}
}

func TestBuildKernelRebootSystemdRunArgs(t *testing.T) {
	got := buildKernelRebootSystemdRunArgs(
		"kwor-kernel-reboot-1234",
		"/opt/kwor/kwor_amd64",
		[]string{kernelRebootWorkerSubcommand, KernelRebootConfirmedArgument},
	)
	want := []string{
		"--unit", "kwor-kernel-reboot-1234",
		"--collect",
		"--description", "kwor kernel reboot",
		"--working-directory=" + filepath.Dir("/opt/kwor/kwor_amd64"),
		"--setenv=" + InternalSystemdCommandEnv + "=1",
		"/opt/kwor/kwor_amd64",
		kernelRebootWorkerSubcommand,
		KernelRebootConfirmedArgument,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("systemd-run args = %#v, want %#v", got, want)
	}
}

func TestRunKernelRebootWorkerExecutesPlanAfterDelay(t *testing.T) {
	previousEnsure := kernelEnsureRebootRuntime
	previousPlan := kernelBuildRebootPlanFn
	previousSleep := kernelRebootSleepFn
	previousRun := kernelRunCommandWithTimeout
	t.Cleanup(func() {
		kernelEnsureRebootRuntime = previousEnsure
		kernelBuildRebootPlanFn = previousPlan
		kernelRebootSleepFn = previousSleep
		kernelRunCommandWithTimeout = previousRun
	})

	kernelEnsureRebootRuntime = func() error { return nil }
	kernelBuildRebootPlanFn = func() (*kernelRebootPlan, error) {
		return &kernelRebootPlan{
			attempts: []kernelRebootAttempt{{
				label:   "reboot",
				command: "/usr/sbin/reboot",
			}},
		}, nil
	}

	slept := time.Duration(0)
	kernelRebootSleepFn = func(duration time.Duration) {
		slept = duration
	}

	called := ""
	kernelRunCommandWithTimeout = func(_ time.Duration, command string, args ...string) error {
		called = command + " " + strings.Join(args, " ")
		return nil
	}

	if err := RunKernelRebootWorker(KernelRebootConfirmedArgument); err != nil {
		t.Fatalf("RunKernelRebootWorker() error = %v", err)
	}
	if slept != kernelRebootWorkerDelay {
		t.Fatalf("kernel reboot worker delay = %s, want %s", slept, kernelRebootWorkerDelay)
	}
	if strings.TrimSpace(called) != "/usr/sbin/reboot" {
		t.Fatalf("kernel reboot command = %q, want /usr/sbin/reboot", called)
	}
}
