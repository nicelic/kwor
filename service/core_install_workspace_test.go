package service

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestActivateManagedCoreBinaryInstallKeepsOldOnMissingStageBinary(t *testing.T) {
	coreDir := t.TempDir()
	targetPath := filepath.Join(coreDir, "mihomo")
	if err := os.WriteFile(targetPath, []byte("old-core"), 0o755); err != nil {
		t.Fatalf("write old core failed: %v", err)
	}

	stageDir, cleanupStageDir, err := createManagedCoreInstallWorkspace(coreDir, mihomoCoreInstallStagePrefix)
	if err != nil {
		t.Fatalf("create stage dir failed: %v", err)
	}
	defer cleanupStageDir()

	_, err = activateManagedCoreBinaryInstall(coreDir, "mihomo", stageDir, mihomoCoreInstallBackupPrefix)
	if err == nil {
		t.Fatal("expected activation to fail when staged binary is missing")
	}

	got, readErr := os.ReadFile(targetPath)
	if readErr != nil {
		t.Fatalf("read old core after failed activation failed: %v", readErr)
	}
	if string(got) != "old-core" {
		t.Fatalf("old core changed after failed activation: %q", string(got))
	}
}

func TestActivateManagedCoreBinaryInstallReplacesBinaryWhenStageReady(t *testing.T) {
	coreDir := t.TempDir()
	targetPath := filepath.Join(coreDir, "sing-box")
	if err := os.WriteFile(targetPath, []byte("old-core"), 0o755); err != nil {
		t.Fatalf("write old core failed: %v", err)
	}

	stageDir, cleanupStageDir, err := createManagedCoreInstallWorkspace(coreDir, singboxCoreInstallStagePrefix)
	if err != nil {
		t.Fatalf("create stage dir failed: %v", err)
	}
	defer cleanupStageDir()

	stagedPath := filepath.Join(stageDir, "sing-box")
	if err := os.WriteFile(stagedPath, []byte("new-core"), 0o755); err != nil {
		t.Fatalf("write staged core failed: %v", err)
	}

	activation, err := activateManagedCoreBinaryInstall(coreDir, "sing-box", stageDir, singboxCoreInstallBackupPrefix)
	if err != nil {
		t.Fatalf("activate managed core binary failed: %v", err)
	}
	if err := activation.Commit(); err != nil {
		t.Fatalf("commit managed core activation failed: %v", err)
	}

	got, readErr := os.ReadFile(targetPath)
	if readErr != nil {
		t.Fatalf("read activated core failed: %v", readErr)
	}
	if string(got) != "new-core" {
		t.Fatalf("unexpected activated core content: %q", string(got))
	}

	if matches, err := filepath.Glob(filepath.Join(coreDir, singboxCoreInstallBackupPrefix+"*")); err != nil {
		t.Fatalf("glob backup dirs failed: %v", err)
	} else if len(matches) != 0 {
		t.Fatalf("expected backup workspace cleaned, got %#v", matches)
	}
}

func TestCleanupManagedCoreInstallWorkspaceArtifacts(t *testing.T) {
	coreDir := t.TempDir()

	paths := []string{
		filepath.Join(coreDir, "extract_tmp_1"),
		filepath.Join(coreDir, singboxCoreInstallStagePrefix+"abc"),
		filepath.Join(coreDir, singboxCoreInstallBackupPrefix+"def"),
		filepath.Join(coreDir, "sing-box"),
		filepath.Join(coreDir, "config.json"),
	}
	for _, path := range paths {
		if filepath.Ext(path) == "" && filepath.Base(path) != "sing-box" && filepath.Base(path) != "config.json" {
			if err := os.MkdirAll(path, 0o755); err != nil {
				t.Fatalf("mkdir test path failed: %v", err)
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("mkdir parent failed: %v", err)
		}
		if err := os.WriteFile(path, []byte("x"), 0o644); err != nil {
			t.Fatalf("write test file failed: %v", err)
		}
	}

	if err := cleanupManagedCoreInstallWorkspaceArtifacts(coreDir, "sing-box"); err != nil {
		t.Fatalf("cleanup managed core install workspace artifacts failed: %v", err)
	}

	for _, removed := range []string{
		filepath.Join(coreDir, "extract_tmp_1"),
		filepath.Join(coreDir, singboxCoreInstallStagePrefix+"abc"),
		filepath.Join(coreDir, singboxCoreInstallBackupPrefix+"def"),
	} {
		if _, err := os.Stat(removed); !os.IsNotExist(err) {
			t.Fatalf("expected workspace artifact removed at %s, got err=%v", removed, err)
		}
	}

	for _, kept := range []string{
		filepath.Join(coreDir, "sing-box"),
		filepath.Join(coreDir, "config.json"),
	} {
		if _, err := os.Stat(kept); err != nil {
			t.Fatalf("expected path kept at %s: %v", kept, err)
		}
	}
}

func TestActivateManagedCoreBinaryInstallWithRuntimeRestoresPreviousRuntimeOnActivationFailure(t *testing.T) {
	calls := make([]string, 0, 4)
	_, stage, err := activateManagedCoreBinaryInstallWithRuntime(
		true,
		func() error {
			calls = append(calls, "stop")
			return nil
		},
		func() {
			calls = append(calls, "before")
		},
		func() error {
			calls = append(calls, "restore")
			return nil
		},
		func() (*managedCoreBinaryActivation, error) {
			calls = append(calls, "activate")
			return nil, errors.New("boom")
		},
	)
	if err == nil {
		t.Fatal("expected activation failure")
	}
	if stage != coreDownloadStageReplacing {
		t.Fatalf("unexpected stage: got %q want %q", stage, coreDownloadStageReplacing)
	}
	if !strings.Contains(err.Error(), "previous runtime was restored") {
		t.Fatalf("expected restore hint in error, got %v", err)
	}
	if got := strings.Join(calls, ","); got != "stop,before,activate,restore" {
		t.Fatalf("unexpected callback order: %s", got)
	}
}

func TestActivateManagedCoreBinaryInstallWithRuntimeReportsStopFailure(t *testing.T) {
	calls := make([]string, 0, 1)
	_, stage, err := activateManagedCoreBinaryInstallWithRuntime(
		true,
		func() error {
			calls = append(calls, "stop")
			return errors.New("stop failed")
		},
		func() {
			calls = append(calls, "before")
		},
		func() error {
			calls = append(calls, "restore")
			return nil
		},
		func() (*managedCoreBinaryActivation, error) {
			calls = append(calls, "activate")
			return nil, nil
		},
	)
	if err == nil {
		t.Fatal("expected stop failure")
	}
	if stage != coreDownloadStageStopping {
		t.Fatalf("unexpected stage: got %q want %q", stage, coreDownloadStageStopping)
	}
	if got := strings.Join(calls, ","); got != "stop" {
		t.Fatalf("unexpected callback order after stop failure: %s", got)
	}
}

func TestCoreDownloadFromURLFailureKeepsRunningCoreProcess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "forced failure", http.StatusInternalServerError)
	}))
	defer srv.Close()

	cmd := startManagedCoreHelperProcess(t)
	svc := &CoreManagerService{
		coreCmd:   cmd,
		isStarted: true,
	}
	if !svc.isRunning() {
		t.Fatal("expected helper process to be reported as running")
	}

	_, err := svc.DownloadCoreFromURL(srv.URL, "test-singbox-download-failure-keeps-old")
	if err == nil {
		t.Fatal("expected download failure")
	}
	if !managedCoreProcessPIDAlive(cmd.Process.Pid) {
		t.Fatal("expected running sing-box process to stay alive when download fails before activation")
	}
}

func TestMihomoDownloadFromURLFailureKeepsRunningCoreProcess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "forced failure", http.StatusInternalServerError)
	}))
	defer srv.Close()

	cmd := startManagedCoreHelperProcess(t)
	svc := &MihomoCoreManagerService{
		coreCmd:   cmd,
		isStarted: true,
	}
	if !svc.isRunning() {
		t.Fatal("expected helper process to be reported as running")
	}

	_, err := svc.DownloadCoreFromURL(srv.URL, "test-mihomo-download-failure-keeps-old")
	if err == nil {
		t.Fatal("expected download failure")
	}
	if !managedCoreProcessPIDAlive(cmd.Process.Pid) {
		t.Fatal("expected running mihomo process to stay alive when download fails before activation")
	}
}

func TestManagedCoreHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_MANAGED_CORE_HELPER_PROCESS") != "1" {
		return
	}
	time.Sleep(30 * time.Second)
	os.Exit(0)
}

func startManagedCoreHelperProcess(t *testing.T) *exec.Cmd {
	t.Helper()

	cmd := exec.Command(os.Args[0], "-test.run=TestManagedCoreHelperProcess")
	cmd.Env = append(os.Environ(), "GO_WANT_MANAGED_CORE_HELPER_PROCESS=1")
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		t.Fatalf("start managed core helper process failed: %v", err)
	}
	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()
		}
	})
	return cmd
}
