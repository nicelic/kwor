package service

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestValidatePanelUninstallEnvironment(t *testing.T) {
	testCases := []struct {
		name        string
		isLinux     bool
		container   bool
		euid        int
		errContains string
	}{
		{name: "linux root host", isLinux: true, euid: 0},
		{name: "non linux", isLinux: false, euid: 0, errContains: "仅支持 Linux"},
		{name: "container", isLinux: true, container: true, euid: 0, errContains: "Docker/容器"},
		{name: "non root", isLinux: true, euid: 1000, errContains: "root"},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := validatePanelUninstallEnvironment(tc.isLinux, tc.container, tc.euid)
			if tc.errContains == "" {
				if err != nil {
					t.Fatalf("validatePanelUninstallEnvironment() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.errContains) {
				t.Fatalf("validatePanelUninstallEnvironment() error = %v, want %q", err, tc.errContains)
			}
		})
	}
}

func TestPanelUninstallCommandArguments(t *testing.T) {
	want := []string{"uninstall", PanelUninstallConfirmedArgument}
	if got := panelUninstallCommandArgs(); !reflect.DeepEqual(got, want) {
		t.Fatalf("panelUninstallCommandArgs() = %#v, want %#v", got, want)
	}
}

func TestDockerUninstallHostCommandsAreMountScoped(t *testing.T) {
	commands := DockerUninstallHostCommands()
	if len(commands) != 2 {
		t.Fatalf("DockerUninstallHostCommands() count = %d, want 2", len(commands))
	}
	byID := make(map[string]string, len(commands))
	for _, command := range commands {
		byID[command.ID] = command.Command
	}
	compose := byID["compose"]
	if !strings.Contains(compose, "docker compose down --remove-orphans") || !strings.Contains(compose, "rm -rf -- ./Promanager_data") {
		t.Fatalf("compose uninstall command is incomplete: %q", compose)
	}
	run := byID["docker-run"]
	for _, expected := range []string{
		"docker inspect",
		"eq .Destination \"/app/Promanager_data\"",
		"docker rm -f \"$container\"",
		"docker volume rm \"$mount_name\"",
		"rm -rf -- \"$mount_source\"",
		"refusing unsafe bind mount",
	} {
		if !strings.Contains(run, expected) {
			t.Fatalf("docker run uninstall command missing %q: %q", expected, run)
		}
	}
	if strings.Contains(run, "docker.sock") || strings.Contains(run, "rm -rf -- /path/to/kwor") {
		t.Fatalf("docker run uninstall command must not use daemon sockets or deployment parents: %q", run)
	}
	if strings.Contains(run, "\\\"/app/Promanager_data\\\"") {
		t.Fatalf("docker template must receive plain quoted destination: %q", run)
	}
}

func TestGetPanelUninstallStatusReturnsDockerGuide(t *testing.T) {
	t.Setenv("KWOR_RUNTIME_MODE", "docker")
	status := GetPanelUninstallStatus()
	if status.Mode != PanelUninstallModeDockerGuide || status.CanSchedule || status.State != "idle" {
		t.Fatalf("Docker uninstall status = %#v", status)
	}
	if len(status.DockerCommands) == 0 || !strings.Contains(status.Hint, "Docker") {
		t.Fatalf("Docker uninstall guide missing from status: %#v", status)
	}
}

func TestPanelUpdateStatusIncludesDockerUninstallGuide(t *testing.T) {
	t.Setenv("KWOR_RUNTIME_MODE", "docker")
	status, err := (&PanelUpdateService{}).GetStatus()
	if err != nil {
		t.Fatalf("get panel update status: %v", err)
	}
	if status.UninstallMode != PanelUninstallModeDockerGuide || status.CanUninstall || len(status.DockerUninstallCommands) != 2 {
		t.Fatalf("panel update Docker uninstall status = %#v", status)
	}
}

func TestBuildPanelUninstallSystemdRunArgs(t *testing.T) {
	got := buildPanelUninstallSystemdRunArgs("kwor-panel-uninstall-test", "/opt/kwor/kwor_amd64")
	want := []string{
		"--unit", "kwor-panel-uninstall-test",
		"--collect",
		"--description", "kwor panel uninstall",
		"/opt/kwor/kwor_amd64",
		"uninstall", PanelUninstallConfirmedArgument,
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("buildPanelUninstallSystemdRunArgs() = %#v, want %#v", got, want)
	}
}

func TestPanelUninstallScheduleOnlyMarksStartedAfterWorkerLaunch(t *testing.T) {
	previousPreflight := panelUninstallPreflightFn
	previousStarter := panelUninstallWorkerStarter
	previousSystemdHost := panelUninstallSystemdHostFn
	panelUninstallSchedule.Lock()
	previousScheduled := panelUninstallSchedule.scheduled
	panelUninstallSchedule.scheduled = false
	panelUninstallSchedule.Unlock()
	t.Cleanup(func() {
		panelUninstallPreflightFn = previousPreflight
		panelUninstallWorkerStarter = previousStarter
		panelUninstallSystemdHostFn = previousSystemdHost
		panelUninstallSchedule.Lock()
		panelUninstallSchedule.scheduled = previousScheduled
		panelUninstallSchedule.Unlock()
	})

	panelUninstallPreflightFn = func() (string, error) {
		return "/opt/kwor/kwor_amd64", nil
	}
	workerCalls := 0
	panelUninstallWorkerStarter = func(binaryPath string, reservationID string) error {
		workerCalls++
		if binaryPath != "/opt/kwor/kwor_amd64" {
			t.Fatalf("worker binary path = %q", binaryPath)
		}
		if reservationID == "" {
			t.Fatal("worker reservation id is empty")
		}
		if workerCalls == 1 {
			return errors.New("worker unavailable")
		}
		return nil
	}

	svc := &PanelUninstallService{}
	if _, err := svc.Schedule(); err == nil || !strings.Contains(err.Error(), "worker unavailable") {
		t.Fatalf("first Schedule() error = %v, want worker launch failure", err)
	}
	panelUninstallSchedule.Lock()
	scheduledAfterFailure := panelUninstallSchedule.scheduled
	panelUninstallSchedule.Unlock()
	if scheduledAfterFailure {
		t.Fatal("failed worker launch must not mark an uninstall task as scheduled")
	}

	result, err := svc.Schedule()
	if err != nil {
		t.Fatalf("second Schedule() error = %v", err)
	}
	if !result.Started || result.BinaryPath != "/opt/kwor/kwor_amd64" {
		t.Fatalf("unexpected successful schedule result: %#v", result)
	}
	if workerCalls != 2 {
		t.Fatalf("worker call count = %d, want 2", workerCalls)
	}

	if _, err := svc.Schedule(); err == nil || !strings.Contains(err.Error(), "请勿重复提交") {
		t.Fatalf("duplicate Schedule() error = %v, want duplicate request rejection", err)
	}
	if workerCalls != 2 {
		t.Fatalf("duplicate request started another worker, calls = %d", workerCalls)
	}
}

func TestStartPanelUninstallSystemdWorkerRollsBackAfterRecordFailure(t *testing.T) {
	previousLookPath := panelUninstallSystemdRunLookPathFn
	previousRun := panelUninstallSystemdRunFn
	previousRecord := panelUninstallWorkerRecordFn
	previousRollback := panelUninstallSystemdWorkerRollbackFn
	previousFail := panelUninstallRollbackFailureFn
	previousNow := panelUninstallNowFn
	t.Cleanup(func() {
		panelUninstallSystemdRunLookPathFn = previousLookPath
		panelUninstallSystemdRunFn = previousRun
		panelUninstallWorkerRecordFn = previousRecord
		panelUninstallSystemdWorkerRollbackFn = previousRollback
		panelUninstallRollbackFailureFn = previousFail
		panelUninstallNowFn = previousNow
	})

	panelUninstallSystemdRunLookPathFn = func(name string) (string, error) {
		if name != "systemd-run" {
			t.Fatalf("looked up unexpected command %q", name)
		}
		return "/usr/bin/systemd-run", nil
	}
	panelUninstallNowFn = func() time.Time { return time.Unix(0, 1234) }
	panelUninstallSystemdRunFn = func(_ time.Duration, command string, args ...string) (string, error) {
		if command != "systemd-run" {
			t.Fatalf("ran unexpected command %q", command)
		}
		want := buildPanelUninstallSystemdRunArgs("kwor-panel-uninstall-1234", "/opt/kwor/kwor")
		if !reflect.DeepEqual(args, want) {
			t.Fatalf("systemd-run args = %#v, want %#v", args, want)
		}
		return "", nil
	}
	panelUninstallWorkerRecordFn = func(reservationID string, pid int, pgid int, unit string) error {
		if reservationID != "reservation-1" || pid != 0 || pgid != 0 || unit != "kwor-panel-uninstall-1234" {
			t.Fatalf("unexpected worker record: reservation=%q pid=%d pgid=%d unit=%q", reservationID, pid, pgid, unit)
		}
		return errors.New("record failed")
	}
	rolledBack := ""
	panelUninstallSystemdWorkerRollbackFn = func(unit string) error {
		rolledBack = unit
		return nil
	}
	failCalls := 0
	panelUninstallRollbackFailureFn = func(string, int, int, uint64, string, error) error {
		failCalls++
		return nil
	}

	err := startPanelUninstallSystemdWorker("/opt/kwor/kwor", "reservation-1")
	if err == nil || !strings.Contains(err.Error(), "record failed") {
		t.Fatalf("startPanelUninstallSystemdWorker() error = %v", err)
	}
	if rolledBack != "kwor-panel-uninstall-1234" {
		t.Fatalf("rolled back unit = %q", rolledBack)
	}
	if failCalls != 0 {
		t.Fatalf("successful rollback must not force failed lifecycle state, calls=%d", failCalls)
	}
}

func TestStartPanelUninstallSystemdWorkerKeepsBlockingStateWhenRollbackFails(t *testing.T) {
	previousLookPath := panelUninstallSystemdRunLookPathFn
	previousRun := panelUninstallSystemdRunFn
	previousRecord := panelUninstallWorkerRecordFn
	previousRollback := panelUninstallSystemdWorkerRollbackFn
	previousFail := panelUninstallRollbackFailureFn
	previousNow := panelUninstallNowFn
	t.Cleanup(func() {
		panelUninstallSystemdRunLookPathFn = previousLookPath
		panelUninstallSystemdRunFn = previousRun
		panelUninstallWorkerRecordFn = previousRecord
		panelUninstallSystemdWorkerRollbackFn = previousRollback
		panelUninstallRollbackFailureFn = previousFail
		panelUninstallNowFn = previousNow
	})

	panelUninstallSystemdRunLookPathFn = func(string) (string, error) { return "/usr/bin/systemd-run", nil }
	panelUninstallSystemdRunFn = func(time.Duration, string, ...string) (string, error) { return "", nil }
	panelUninstallWorkerRecordFn = func(string, int, int, string) error { return errors.New("record failed") }
	panelUninstallSystemdWorkerRollbackFn = func(string) error { return errors.New("worker remains active") }
	panelUninstallNowFn = func() time.Time { return time.Unix(0, 5678) }
	var failedStateErr error
	panelUninstallRollbackFailureFn = func(reservationID string, pid int, pgid int, startTime uint64, unit string, err error) error {
		if reservationID != "reservation-2" || pid != 0 || pgid != 0 || startTime != 0 || unit != "kwor-panel-uninstall-5678" {
			t.Fatalf("unexpected rollback state identity: reservation=%q pid=%d pgid=%d start=%d unit=%q", reservationID, pid, pgid, startTime, unit)
		}
		failedStateErr = err
		return nil
	}

	err := startPanelUninstallSystemdWorker("/opt/kwor/kwor", "reservation-2")
	if err == nil || !strings.Contains(err.Error(), "worker remains active") {
		t.Fatalf("startPanelUninstallSystemdWorker() error = %v", err)
	}
	if failedStateErr == nil || !strings.Contains(failedStateErr.Error(), "record failed") || !strings.Contains(failedStateErr.Error(), "worker remains active") {
		t.Fatalf("blocking lifecycle failure = %v", failedStateErr)
	}
}
