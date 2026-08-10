package service

import (
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/alireza0/s-ui/database/model"
)

func TestSingboxCoreReviewContracts(t *testing.T) {
	t.Run("version parser only accepts sing-box version field", func(t *testing.T) {
		output := "sing-box version 1.14.0-beta.12\n" +
			"Environment: go1.26.1 linux/amd64\n"
		if got := extractSingboxVersionFromOutput(output); got != "1.14.0-beta.12" {
			t.Fatalf("version = %q, want 1.14.0-beta.12", got)
		}

		if got := extractSingboxVersionFromOutput("sing-box version\nGo version go1.26.1\n"); got != "" {
			t.Fatalf("parser must not use unrelated Go version, got %q", got)
		}

		if got := extractSingboxVersionFromOutput("sing-box version go1.26.1\n"); got != "" {
			t.Fatalf("parser must reject a Go toolchain token in the sing-box version field, got %q", got)
		}
	})

	t.Run("remote channel is normalized to stable or alpha", func(t *testing.T) {
		cases := map[string]string{
			"":        singboxReleaseChannelStable,
			"stable":  singboxReleaseChannelStable,
			"STABLE":  singboxReleaseChannelStable,
			"alpha":   singboxReleaseChannelAlpha,
			" Alpha ": singboxReleaseChannelAlpha,
			"unknown": singboxReleaseChannelStable,
		}
		for input, want := range cases {
			if got := normalizeSingboxReleaseChannel(input); got != want {
				t.Fatalf("channel %q = %q, want %q", input, got, want)
			}
		}
		if shouldIncludeRelease("unknown", true) {
			t.Fatal("unknown channel must not include prereleases")
		}
	})

	t.Run("all semantic prerelease forms use the test channel", func(t *testing.T) {
		for _, version := range []string{"1.14.0-alpha.1", "1.14.0-beta.2", "1.14.0-rc.1", "1.14.0-dev.3"} {
			if got := detectSingboxInstalledChannelFromVersion(version); got != singboxReleaseChannelAlpha {
				t.Fatalf("version %q detected as %q, want alpha", version, got)
			}
		}
		if got := detectSingboxInstalledChannelFromVersion("1.14.0+build.1"); got != singboxReleaseChannelStable {
			t.Fatalf("stable build metadata detected as %q", got)
		}
		if got := detectSingboxInstalledChannelFromVersion("go1.26.1"); got != "" {
			t.Fatalf("Go toolchain token detected as sing-box channel %q", got)
		}
	})

	t.Run("semantic build metadata does not create a false update", func(t *testing.T) {
		if singboxRemoteVersionIsNewer("v1.14.0", "1.14.0+build.1") {
			t.Fatal("build metadata must not make an equal stable version appear outdated")
		}
		if !singboxRemoteVersionIsNewer("v1.14.0-beta.2+build.9", "1.14.0-beta.1+build.1") {
			t.Fatal("prerelease ordering must remain effective after stripping build metadata")
		}
	})

	t.Run("pending update is scoped to installed channel", func(t *testing.T) {
		stableStatus := &SingboxCoreInfo{
			LocalVersion:     "1.13.17",
			InstalledChannel: singboxReleaseChannelStable,
		}
		stable, alpha := selectSingboxPendingUpdateForChannel(stableStatus, "v1.13.18", "v1.14.0-beta.12")
		if stable != "v1.13.18" || alpha != "" {
			t.Fatalf("stable target mismatch: stable=%q alpha=%q", stable, alpha)
		}

		alphaStatus := &SingboxCoreInfo{
			LocalVersion:     "1.14.0-beta.11",
			InstalledChannel: singboxReleaseChannelAlpha,
		}
		stable, alpha = selectSingboxPendingUpdateForChannel(alphaStatus, "v1.13.18", "v1.14.0-beta.12")
		if stable != "" || alpha != "v1.14.0-beta.12" {
			t.Fatalf("alpha target mismatch: stable=%q alpha=%q", stable, alpha)
		}
	})

	t.Run("unrecoverable local-state failures disable scheduled auto update", func(t *testing.T) {
		for _, reason := range []string{
			"当前平台不支持 sing-box 自动更新",
			"本地未安装 sing-box 内核",
			"本地内核不兼容，无法用于自动更新",
			"无法识别本地内核架构",
			"无法识别本地内核包类型",
			"无法识别本地内核版本频道",
		} {
			if !singboxShouldDisableAutoUpdateOnError(reason) {
				t.Fatalf("reason %q must disable auto update", reason)
			}
		}
		if singboxShouldDisableAutoUpdateOnError("GitHub request timed out") {
			t.Fatal("transient download failure must not disable auto update")
		}
	})
}

func TestSingboxCoreWindowsProcessRecoveryUsesManagedBinaryPath(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("Windows process reattachment is platform-specific")
	}

	previousPlatform, previousPlatformErr := GetSystemPlatform()
	t.Cleanup(func() {
		if previousPlatformErr != nil || previousPlatform == nil {
			clearSystemPlatformSnapshot()
			return
		}
		setSystemPlatformSnapshot(previousPlatform)
	})
	setSystemPlatformSnapshot(&model.SystemPlatform{OS: "windows", Architecture: "amd64"})

	manager := &CoreManagerService{}
	managedPath := manager.getCoreBinPath()
	if err := os.MkdirAll(filepath.Dir(managedPath), 0o755); err != nil {
		t.Fatalf("create managed Core directory: %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(managedPath) })

	testBinary, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve test binary: %v", err)
	}
	if err := copyCoreFile(testBinary, managedPath); err != nil {
		t.Fatalf("copy helper binary to managed Core path: %v", err)
	}

	cmd := exec.Command(managedPath, "-test.run=TestManagedCoreHelperProcess")
	cmd.Env = append(os.Environ(), "GO_WANT_MANAGED_CORE_HELPER_PROCESS=1")
	cmd.Stdout = io.Discard
	cmd.Stderr = io.Discard
	if err := cmd.Start(); err != nil {
		t.Fatalf("start managed Core helper: %v", err)
	}
	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()
		}
	})

	found := false
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if manager.isRunning() {
			found = true
			break
		}
		time.Sleep(80 * time.Millisecond)
	}
	if !found {
		t.Fatal("managed Windows sing-box process should be detected after panel restart")
	}

	if err := manager.stopCoreInternal(); err != nil {
		t.Fatalf("stop reattached managed Windows sing-box process: %v", err)
	}
	if managedCoreProcessPIDAlive(cmd.Process.Pid) {
		t.Fatal("managed Windows sing-box process remains alive after stop")
	}
}
