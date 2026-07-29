package service

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestReadPanelUpdateLastError(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "panel-update-last.log")
	content := strings.Join([]string{
		"",
		"line-1",
		"line-2",
		"line-3",
		"line-4",
		"line-5",
		"",
	}, "\n")
	if err := os.WriteFile(logPath, []byte(content), 0o600); err != nil {
		t.Fatalf("write log failed: %v", err)
	}

	got := readPanelUpdateLastError(logPath)
	want := "line-2 | line-3 | line-4 | line-5"
	if got != want {
		t.Fatalf("readPanelUpdateLastError()=%q want %q", got, want)
	}
}

func TestClearPanelUpdateLastError(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "panel-update-last.log")
	if err := os.WriteFile(logPath, []byte("failure"), 0o600); err != nil {
		t.Fatalf("write log failed: %v", err)
	}

	clearPanelUpdateLastError(logPath)

	if _, err := os.Stat(logPath); !os.IsNotExist(err) {
		t.Fatalf("expected log to be removed, stat err=%v", err)
	}
}

func TestPanelUpdateVersionSelectable(t *testing.T) {
	testCases := []struct {
		name    string
		version string
		want    bool
	}{
		{name: "lower version", version: "v1.5.26", want: false},
		{name: "minimum version", version: "v1.6.0", want: true},
		{name: "bare minimum version", version: "1.6.0", want: true},
		{name: "minimum prerelease", version: "v1.6.0-rc.1", want: false},
		{name: "newer prerelease", version: "v1.6.1-rc.1", want: true},
		{name: "newer stable version", version: "v1.7.0", want: true},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if got := isPanelUpdateVersionSelectable(testCase.version); got != testCase.want {
				t.Fatalf("isPanelUpdateVersionSelectable(%q) = %t, want %t", testCase.version, got, testCase.want)
			}
		})
	}
}

func TestWritePanelUpdateScriptIncludesRuntimeVerificationAndFailureLog(t *testing.T) {
	tempDir := t.TempDir()
	scriptPath, err := writePanelUpdateScript(
		tempDir,
		"/opt/kwor/kwor",
		"/tmp/staged-kwor",
		"/tmp/install.sh",
		"/tmp/kwor.service",
		"kwor",
	)
	if err != nil {
		t.Fatalf("writePanelUpdateScript failed: %v", err)
	}

	contentBytes, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("read script failed: %v", err)
	}
	content := string(contentBytes)

	for _, needle := range []string{
		"kwor-owner:v1 resource=panel-update-workspace",
		"LAST_LOG_PATH=",
		"UPDATE_SUCCESS=0",
		"CLEANUP_DONE=0",
		"process_matches_target()",
		"process_start_time()",
		"process_identity_matches_target()",
		"stop_target_systemd_service()",
		"--property=MainPID",
		"wait_for_target_runtime()",
		"if \"$TARGET_BIN\" start >> \"$LOG_PATH\" 2>&1 && wait_for_target_runtime; then",
		"if wait_for_target_runtime; then",
		"cp -f \"$LOG_PATH\" \"$LAST_LOG_PATH\"",
		"Promanager_data",
		"UPDATE_SUCCESS=1",
	} {
		if !strings.Contains(content, needle) {
			t.Fatalf("script missing %q", needle)
		}
	}
	for _, forbidden := range []string{
		"pkill",
		"pgrep -x",
		"/proc/$pid/cmdline",
		"\"$TARGET_BIN\" stop",
		"trap 'cleanup",
	} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("script contains unsafe legacy process handling %q", forbidden)
		}
	}
}

func TestGeneratedPanelUpdateScriptHasValidBashSyntax(t *testing.T) {
	bashPath, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash is unavailable")
	}
	tempDir := t.TempDir()
	scriptPath, err := writePanelUpdateScript(
		tempDir,
		filepath.Join(tempDir, "kwor"),
		filepath.Join(tempDir, "staged-kwor"),
		filepath.Join(tempDir, "install.sh"),
		filepath.Join(tempDir, "kwor.service"),
		"kwor",
	)
	if err != nil {
		t.Fatalf("writePanelUpdateScript failed: %v", err)
	}
	output, err := exec.Command(bashPath, "-n", scriptPath).CombinedOutput()
	if err != nil {
		t.Fatalf("generated update script has invalid bash syntax: %v: %s", err, strings.TrimSpace(string(output)))
	}
}

func TestStartPanelUpdateSystemdWorkerRollsBackAfterLauncherFailure(t *testing.T) {
	previousLookPath := panelUpdateSystemdRunLookPathFn
	previousLauncher := panelUpdateSystemdLauncherFn
	previousSetUnit := panelUpdateSetSystemdUnitFn
	previousRollback := panelUpdateSystemdWorkerRollbackFn
	previousActive := panelUpdateSystemdActiveFn
	previousNow := panelUpdateNowFn
	t.Cleanup(func() {
		panelUpdateSystemdRunLookPathFn = previousLookPath
		panelUpdateSystemdLauncherFn = previousLauncher
		panelUpdateSetSystemdUnitFn = previousSetUnit
		panelUpdateSystemdWorkerRollbackFn = previousRollback
		panelUpdateSystemdActiveFn = previousActive
		panelUpdateNowFn = previousNow
	})

	panelUpdateSystemdRunLookPathFn = func(string) (string, error) { return "/usr/bin/systemd-run", nil }
	panelUpdateNowFn = func() time.Time { return time.Unix(0, 1234) }
	scriptPath := filepath.Join(t.TempDir(), "apply-update.sh")
	panelUpdateSystemdLauncherFn = func(_ context.Context, args []string) panelUpdateSystemdLaunchResult {
		want := []string{
			"--unit", "kwor-panel-update-1234",
			"--collect",
			"--description", "kwor panel update",
			"--working-directory=" + filepath.Dir(scriptPath),
			"--setenv=" + kworLifecycleOperationIDEnv + "=panel-update-op",
			"bash", scriptPath,
		}
		if !reflect.DeepEqual(args, want) {
			t.Fatalf("systemd-run args = %#v, want %#v", args, want)
		}
		return panelUpdateSystemdLaunchResult{Started: true, Err: errors.New("launcher tracking failed")}
	}
	setUnits := make([]string, 0, 2)
	panelUpdateSetSystemdUnitFn = func(_ *KworManagedOperationHandle, unit string) error {
		setUnits = append(setUnits, unit)
		return nil
	}
	rolledBackUnit := ""
	rolledBackOperation := ""
	panelUpdateSystemdWorkerRollbackFn = func(unit string, operationID string) error {
		rolledBackUnit = unit
		rolledBackOperation = operationID
		return nil
	}
	panelUpdateSystemdActiveFn = func() bool { return true }

	err := startPanelUpdateWorker(context.Background(), &KworManagedOperationHandle{id: "panel-update-op"}, scriptPath)
	if err == nil || !strings.Contains(err.Error(), "launcher tracking failed") {
		t.Fatalf("startPanelUpdateWorker() error = %v", err)
	}
	if panelUpdateWorkerRollbackIncomplete(err) {
		t.Fatalf("successful rollback must not be marked incomplete: %v", err)
	}
	if rolledBackUnit != "kwor-panel-update-1234" || rolledBackOperation != "panel-update-op" {
		t.Fatalf("rollback identity = unit %q operation %q", rolledBackUnit, rolledBackOperation)
	}
	if !reflect.DeepEqual(setUnits, []string{"kwor-panel-update-1234", ""}) {
		t.Fatalf("operation unit updates = %#v", setUnits)
	}
}

func TestStartPanelUpdateSystemdWorkerKeepsRegistrationWhenRollbackFails(t *testing.T) {
	previousLookPath := panelUpdateSystemdRunLookPathFn
	previousLauncher := panelUpdateSystemdLauncherFn
	previousSetUnit := panelUpdateSetSystemdUnitFn
	previousRollback := panelUpdateSystemdWorkerRollbackFn
	previousNow := panelUpdateNowFn
	t.Cleanup(func() {
		panelUpdateSystemdRunLookPathFn = previousLookPath
		panelUpdateSystemdLauncherFn = previousLauncher
		panelUpdateSetSystemdUnitFn = previousSetUnit
		panelUpdateSystemdWorkerRollbackFn = previousRollback
		panelUpdateNowFn = previousNow
	})

	panelUpdateSystemdRunLookPathFn = func(string) (string, error) { return "/usr/bin/systemd-run", nil }
	panelUpdateNowFn = func() time.Time { return time.Unix(0, 5678) }
	panelUpdateSystemdLauncherFn = func(context.Context, []string) panelUpdateSystemdLaunchResult {
		return panelUpdateSystemdLaunchResult{Started: true, Err: errors.New("record failed")}
	}
	setUnits := make([]string, 0, 2)
	panelUpdateSetSystemdUnitFn = func(_ *KworManagedOperationHandle, unit string) error {
		setUnits = append(setUnits, unit)
		return nil
	}
	panelUpdateSystemdWorkerRollbackFn = func(unit string, operationID string) error {
		if unit != "kwor-panel-update-5678" || operationID != "panel-update-op" {
			t.Fatalf("unexpected rollback identity: unit=%q operation=%q", unit, operationID)
		}
		return errors.New("unit remains active")
	}

	err := startPanelUpdateWorker(context.Background(), &KworManagedOperationHandle{id: "panel-update-op"}, filepath.Join(t.TempDir(), "apply-update.sh"))
	if err == nil || !panelUpdateWorkerRollbackIncomplete(err) || !strings.Contains(err.Error(), "unit remains active") {
		t.Fatalf("rollback failure error = %v", err)
	}
	if !reflect.DeepEqual(setUnits, []string{"kwor-panel-update-5678"}) {
		t.Fatalf("failed rollback must keep recorded unit, updates = %#v", setUnits)
	}
}

func TestLoadPanelUpdateLogViewTrimsAndCapsLines(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "panel-update-last.log")
	content := strings.Join([]string{
		"",
		"first",
		"second",
		"third",
		"fourth",
		"fifth",
		"",
	}, "\n")
	if err := os.WriteFile(logPath, []byte(content), 0o600); err != nil {
		t.Fatalf("write log failed: %v", err)
	}

	view, err := loadPanelUpdateLogView(logPath)
	if err != nil {
		t.Fatalf("loadPanelUpdateLogView failed: %v", err)
	}
	if !view.Exists {
		t.Fatalf("expected Exists=true")
	}
	if len(view.Lines) == 0 {
		t.Fatalf("expected lines to be returned")
	}
	if view.Lines[0] != "first" {
		t.Fatalf("first line=%q want %q", view.Lines[0], "first")
	}

	normalized := normalizePanelUpdateLogLines([]byte(content), 3)
	if len(normalized) != 4 {
		t.Fatalf("normalized line count=%d want 4", len(normalized))
	}
	if normalized[0] != "日志过长，已隐藏较早输出" {
		t.Fatalf("normalized head=%q", normalized[0])
	}
	if normalized[3] != "fifth" {
		t.Fatalf("normalized last=%q want %q", normalized[3], "fifth")
	}
}
