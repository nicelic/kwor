package service

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/alireza0/s-ui/config"
)

type managedCoreRuntimeMode string

const (
	managedCoreRuntimeModeSystemd managedCoreRuntimeMode = "systemd"
	managedCoreRuntimeModeDirect  managedCoreRuntimeMode = "direct"
)

var managedCoreRuntimeModeCache struct {
	once sync.Once
	mode managedCoreRuntimeMode
}

var kworSystemdHostCache struct {
	once      sync.Once
	available bool
}

func getManagedCoreRuntimeMode() managedCoreRuntimeMode {
	managedCoreRuntimeModeCache.once.Do(func() {
		managedCoreRuntimeModeCache.mode = detectManagedCoreRuntimeMode()
	})
	return managedCoreRuntimeModeCache.mode
}

func detectManagedCoreRuntimeMode() managedCoreRuntimeMode {
	if isKworSystemdHost() {
		return managedCoreRuntimeModeSystemd
	}
	return managedCoreRuntimeModeDirect
}

func runningInsideContainer() bool {
	if value := strings.TrimSpace(os.Getenv("KWOR_RUNTIME_MODE")); value != "" {
		lower := strings.ToLower(value)
		if lower == "docker" || lower == "container" || lower == "direct" {
			return true
		}
		if lower == "systemd" || lower == "host" {
			return false
		}
	}
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return true
	}
	for _, envKey := range []string{"container", "KWOR_IN_DOCKER"} {
		value := strings.TrimSpace(os.Getenv(envKey))
		if value == "" {
			continue
		}
		if strings.EqualFold(value, "1") || strings.EqualFold(value, "true") || strings.EqualFold(value, "docker") {
			return true
		}
	}
	data, err := os.ReadFile("/proc/1/cgroup")
	if err == nil {
		text := strings.ToLower(string(data))
		for _, marker := range []string{"docker", "containerd", "kubepods", "podman"} {
			if strings.Contains(text, marker) {
				return true
			}
		}
	}
	return false
}

func RunningInsideContainer() bool {
	return runningInsideContainer()
}

// RunningInsideDocker is narrower than generic container detection. Only the
// packaged Docker image has the documented /app/Promanager_data mount layout.
func RunningInsideDocker() bool {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("KWOR_RUNTIME_MODE")))
	if mode != "" {
		return mode == "docker"
	}
	if strings.EqualFold(strings.TrimSpace(os.Getenv("KWOR_IN_DOCKER")), "true") {
		return true
	}
	_, err := os.Stat("/.dockerenv")
	return err == nil
}

// isKworSystemdHost verifies that a reachable system manager exists. Some
// non-systemd Linux distributions ship a systemctl compatibility binary.
func isKworSystemdHost() bool {
	kworSystemdHostCache.once.Do(func() {
		if runtime.GOOS != "linux" || runningInsideContainer() {
			return
		}
		if _, err := os.Stat("/run/systemd/system"); err != nil {
			return
		}
		systemctl, err := exec.LookPath("systemctl")
		if err != nil {
			return
		}
		kworSystemdHostCache.available = runCommandWithTimeout(shortSystemCommandTimeout, systemctl, "show-environment") == nil
	})
	return kworSystemdHostCache.available
}

func shouldUseDirectManagedCoreRuntime() bool {
	return getManagedCoreRuntimeMode() == managedCoreRuntimeModeDirect
}

func ShouldAutoRecoverManagedCoreRuntime(coreName string) bool {
	if !IsSystemPlatformLinux() {
		return false
	}
	if !shouldUseDirectManagedCoreRuntime() {
		return false
	}
	return managedCoreShouldRun(coreName)
}

func ShouldRecoverManagedCoreOnStartup(coreName string) bool {
	return ShouldAutoRecoverManagedCoreRuntime(coreName)
}

func managedCoreRuntimeMarkerPath(coreName string) string {
	coreName = normalizeManagedCoreMarkerName(coreName)
	return filepath.Join(config.GetDataDir(), "runtime", fmt.Sprintf("%s-running.flag", coreName))
}

func normalizeManagedCoreMarkerName(coreName string) string {
	coreName = strings.TrimSpace(strings.ToLower(coreName))
	coreName = strings.ReplaceAll(coreName, "-", "")
	switch coreName {
	case "singbox", "sing-box":
		return "singbox"
	case "mihomo":
		return "mihomo"
	default:
		return coreName
	}
}

func markManagedCoreShouldRun(coreName string) error {
	markerPath := managedCoreRuntimeMarkerPath(coreName)
	if err := os.MkdirAll(filepath.Dir(markerPath), 0o740); err != nil {
		return err
	}
	return os.WriteFile(markerPath, []byte("1"), 0o640)
}

func clearManagedCoreShouldRun(coreName string) {
	_ = os.Remove(managedCoreRuntimeMarkerPath(coreName))
}

func managedCoreShouldRun(coreName string) bool {
	_, err := os.Stat(managedCoreRuntimeMarkerPath(coreName))
	return err == nil
}
