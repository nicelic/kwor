//go:build linux

package service

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

func TestClearKworUninstallLifecycleRemovesControlledRuntimeDirectory(t *testing.T) {
	previousRuntimeDir := kworLifecycleRuntimeDirFn
	runtimeDir := filepath.Join(t.TempDir(), "run", "kwor")
	kworLifecycleRuntimeDirFn = func() string { return runtimeDir }
	t.Cleanup(func() { kworLifecycleRuntimeDirFn = previousRuntimeDir })

	if err := os.MkdirAll(runtimeDir, 0o750); err != nil {
		t.Fatalf("create lifecycle runtime directory: %v", err)
	}
	files := map[string]string{
		kworLifecycleStateFileName:               `{"version":1,"status":"running","blockNewWork":true}`,
		kworLifecycleOperationsFileName:          `{"version":1,"operations":[]}`,
		kworLifecycleStateFileName + ".tmp":      "stale state",
		kworLifecycleOperationsFileName + ".tmp": "stale operations",
		"lifecycle.lock":                         "",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(runtimeDir, name), []byte(content), 0o600); err != nil {
			t.Fatalf("write lifecycle artifact %s: %v", name, err)
		}
	}

	if err := ClearKworUninstallLifecycle(); err != nil {
		t.Fatalf("clear lifecycle runtime: %v", err)
	}
	if _, err := os.Stat(runtimeDir); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("lifecycle runtime directory still exists, stat error = %v", err)
	}
}

func TestStaleLifecycleSocketIsRecoverableAndSocketScoped(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "lifecycle.sock")
	listener, err := net.ListenUnix("unix", &net.UnixAddr{Name: socketPath, Net: "unix"})
	if err != nil {
		t.Fatalf("create lifecycle socket: %v", err)
	}
	listener.SetUnlinkOnClose(false)
	if err := listener.Close(); err != nil {
		t.Fatalf("close lifecycle socket: %v", err)
	}
	if info, err := os.Lstat(socketPath); err != nil || info.Mode()&os.ModeSocket == 0 {
		t.Fatalf("stale lifecycle socket = info=%#v err=%v", info, err)
	}
	if !isKworLifecycleSocketUnavailable(syscall.ECONNREFUSED) {
		t.Fatal("connection-refused lifecycle socket must be recoverable")
	}
	if err := removeStaleKworLifecycleSocket(socketPath); err != nil {
		t.Fatalf("remove stale lifecycle socket: %v", err)
	}
	if _, err := os.Lstat(socketPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale socket remains, stat error = %v", err)
	}

	regularPath := filepath.Join(t.TempDir(), "lifecycle.sock")
	if err := os.WriteFile(regularPath, []byte("not a socket"), 0o600); err != nil {
		t.Fatalf("write regular lifecycle path: %v", err)
	}
	if err := removeStaleKworLifecycleSocket(regularPath); err == nil {
		t.Fatal("regular lifecycle path must not be removed as a stale socket")
	}
	if _, err := os.Stat(regularPath); err != nil {
		t.Fatalf("regular lifecycle path was removed, stat error = %v", err)
	}
}

func TestFailedUninstallLifecyclePersistsBoundedReport(t *testing.T) {
	previousRuntimeDir := kworLifecycleRuntimeDirFn
	runtimeDir := filepath.Join(t.TempDir(), "run", "kwor")
	kworLifecycleRuntimeDirFn = func() string { return runtimeDir }
	t.Cleanup(func() { kworLifecycleRuntimeDirFn = previousRuntimeDir })

	if err := BeginKworUninstallLifecycle("正在删除面板运行文件"); err != nil {
		t.Fatalf("begin uninstall lifecycle: %v", err)
	}
	FailKworUninstallLifecycleWithReport(errors.New("cleanup failed"), &UninstallReport{
		Failures: []string{"删除 kwor_amd64: permission denied", "删除 kwor_amd64: permission denied"},
		Warnings: []string{"保留未知资源", "保留未知资源"},
	})
	state, found, err := LoadKworUninstallLifecycleState()
	if err != nil || !found || state == nil {
		t.Fatalf("load failed uninstall lifecycle = state=%#v found=%v err=%v", state, found, err)
	}
	if state.Status != kworUninstallStatusFailed || state.Phase != "正在删除面板运行文件" || !state.BlockNewWork {
		t.Fatalf("unexpected failed lifecycle state: %#v", state)
	}
	if len(state.Failures) != 1 || len(state.Warnings) != 1 || state.Error != "cleanup failed" {
		t.Fatalf("failed lifecycle report was not normalized: %#v", state)
	}
}
