package service

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
)

const systemPlatformRecordID uint = 1

var (
	systemPlatformRuntimeGOOS         = runtime.GOOS
	systemPlatformRuntimeGOARCH       = runtime.GOARCH
	systemPlatformReadFile            = os.ReadFile
	systemPlatformOSReleasePaths      = []string{"/etc/os-release", "/usr/lib/os-release"}
	systemPlatformDetectLibc          = detectSystemPlatformLibc
	systemPlatformDetectKernelRelease = detectSystemPlatformKernelRelease
	systemPlatformNow                 = time.Now
)

var systemPlatformSnapshot = struct {
	mu    sync.RWMutex
	value *model.SystemPlatform
}{}

func init() {
	database.RegisterDBResetHook(clearSystemPlatformSnapshot)
}

// RefreshSystemPlatform detects the host once for a panel start, persists the
// singleton record, and replaces the runtime snapshot used by all readers.
func RefreshSystemPlatform() (*model.SystemPlatform, error) {
	db := database.GetDB()
	if db == nil {
		return nil, fmt.Errorf("system platform database is not initialized")
	}

	platform := detectSystemPlatform()
	platform.Id = systemPlatformRecordID

	stored := &model.SystemPlatform{}
	err := db.First(stored, systemPlatformRecordID).Error
	if database.IsNotFound(err) {
		if err := db.Create(platform).Error; err != nil {
			return nil, err
		}
		setSystemPlatformSnapshot(platform)
		return cloneSystemPlatform(platform), nil
	}
	if err != nil {
		return nil, err
	}

	stored.OS = platform.OS
	stored.Architecture = platform.Architecture
	stored.SystemID = platform.SystemID
	stored.SystemIDLike = platform.SystemIDLike
	stored.SystemFamily = platform.SystemFamily
	stored.Libc = platform.Libc
	stored.KernelRelease = platform.KernelRelease
	stored.VersionID = platform.VersionID
	stored.VersionCodename = platform.VersionCodename
	stored.DetectedAt = platform.DetectedAt
	if err := db.Save(stored).Error; err != nil {
		return nil, err
	}
	setSystemPlatformSnapshot(stored)
	return cloneSystemPlatform(stored), nil
}

// GetSystemPlatform returns the in-memory startup snapshot without probing the
// operating system or acquiring a SQLite connection.
func GetSystemPlatform() (*model.SystemPlatform, error) {
	systemPlatformSnapshot.mu.RLock()
	platform := cloneSystemPlatform(systemPlatformSnapshot.value)
	systemPlatformSnapshot.mu.RUnlock()
	if platform == nil {
		return nil, fmt.Errorf("system platform snapshot is unavailable until the panel starts")
	}
	return platform, nil
}

func setSystemPlatformSnapshot(platform *model.SystemPlatform) {
	systemPlatformSnapshot.mu.Lock()
	systemPlatformSnapshot.value = cloneSystemPlatform(platform)
	systemPlatformSnapshot.mu.Unlock()
}

func clearSystemPlatformSnapshot() {
	systemPlatformSnapshot.mu.Lock()
	systemPlatformSnapshot.value = nil
	systemPlatformSnapshot.mu.Unlock()
}

func cloneSystemPlatform(platform *model.SystemPlatform) *model.SystemPlatform {
	if platform == nil {
		return nil
	}
	cloned := *platform
	return &cloned
}

func IsSystemPlatformLinux() bool {
	platform, err := GetSystemPlatform()
	return err == nil && strings.EqualFold(strings.TrimSpace(platform.OS), "linux")
}

func IsSystemPlatformWindows() bool {
	platform, err := GetSystemPlatform()
	return err == nil && strings.EqualFold(strings.TrimSpace(platform.OS), "windows")
}

// GetSystemPlatformOS reads the runtime startup snapshot only.
func GetSystemPlatformOS() string {
	platform, err := GetSystemPlatform()
	if err != nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(platform.OS))
}

// GetSystemPlatformArchitecture reads the runtime startup snapshot only.
func GetSystemPlatformArchitecture() string {
	platform, err := GetSystemPlatform()
	if err != nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(platform.Architecture))
}

// GetSystemPlatformLibc reads the libc classification captured at startup.
func GetSystemPlatformLibc() string {
	platform, err := GetSystemPlatform()
	if err != nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(platform.Libc))
}

// GetSystemPlatformReleaseFields exposes the cached os-release values to
// callers that need distribution-specific behavior.
func GetSystemPlatformReleaseFields() map[string]string {
	platform, err := GetSystemPlatform()
	if err != nil {
		return map[string]string{}
	}
	return map[string]string{
		"ID":               strings.TrimSpace(platform.SystemID),
		"ID_LIKE":          strings.TrimSpace(platform.SystemIDLike),
		"VERSION_ID":       strings.TrimSpace(platform.VersionID),
		"VERSION_CODENAME": strings.TrimSpace(platform.VersionCodename),
	}
}

func detectSystemPlatform() *model.SystemPlatform {
	platform := &model.SystemPlatform{
		OS:           strings.ToLower(strings.TrimSpace(systemPlatformRuntimeGOOS)),
		Architecture: strings.ToLower(strings.TrimSpace(systemPlatformRuntimeGOARCH)),
		DetectedAt:   systemPlatformNow().Unix(),
	}
	if platform.OS != "linux" {
		return platform
	}

	fields := readSystemPlatformOSReleaseFields()
	platform.SystemID = strings.ToLower(strings.TrimSpace(fields["ID"]))
	platform.SystemIDLike = strings.ToLower(strings.TrimSpace(fields["ID_LIKE"]))
	platform.VersionID = strings.TrimSpace(fields["VERSION_ID"])
	platform.VersionCodename = strings.TrimSpace(fields["VERSION_CODENAME"])
	if platform.VersionCodename == "" {
		platform.VersionCodename = strings.TrimSpace(fields["UBUNTU_CODENAME"])
	}
	platform.SystemFamily = detectSystemPlatformFamily(platform.SystemID, platform.SystemIDLike)
	platform.Libc = systemPlatformDetectLibc(platform.SystemID, platform.SystemFamily)
	platform.KernelRelease = systemPlatformDetectKernelRelease()
	return platform
}

func detectSystemPlatformKernelRelease() string {
	ctx, cancel := context.WithTimeout(context.Background(), shortSystemCommandTimeout)
	defer cancel()
	output, err := exec.CommandContext(ctx, "uname", "-r").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

func detectSystemPlatformLibc(systemID string, systemFamily string) string {
	if strings.EqualFold(strings.TrimSpace(systemID), "alpine") || strings.EqualFold(strings.TrimSpace(systemFamily), "alpine") {
		return "musl"
	}
	if _, err := os.Stat("/etc/alpine-release"); err == nil {
		return "musl"
	}
	if matches, _ := filepath.Glob("/lib/ld-musl-*.so.1"); len(matches) > 0 {
		return "musl"
	}
	ctx, cancel := context.WithTimeout(context.Background(), shortSystemCommandTimeout)
	defer cancel()
	output, err := exec.CommandContext(ctx, "ldd", "--version").CombinedOutput()
	if err != nil {
		return ""
	}
	lower := strings.ToLower(string(output))
	if strings.Contains(lower, "musl") {
		return "musl"
	}
	if strings.Contains(lower, "glibc") || strings.Contains(lower, "gnu libc") {
		return "glibc"
	}
	return ""
}

func readSystemPlatformOSReleaseFields() map[string]string {
	for _, path := range systemPlatformOSReleasePaths {
		content, err := systemPlatformReadFile(path)
		if err != nil {
			continue
		}
		return parseSystemPlatformOSReleaseFields(string(content))
	}
	return map[string]string{}
}

func parseSystemPlatformOSReleaseFields(content string) map[string]string {
	result := make(map[string]string)
	for _, rawLine := range strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n") {
		line := strings.TrimSpace(rawLine)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		result[strings.TrimSpace(key)] = strings.Trim(strings.TrimSpace(value), "\"'")
	}
	return result
}

func detectSystemPlatformFamily(systemID string, systemIDLike string) string {
	id := strings.ToLower(strings.TrimSpace(systemID))
	idLike := strings.ToLower(strings.TrimSpace(systemIDLike))
	switch {
	case strings.Contains(idLike, "debian") || id == "debian" || id == "ubuntu":
		return "debian"
	case strings.Contains(idLike, "rhel") || strings.Contains(idLike, "fedora") || id == "fedora" || id == "rhel" || id == "centos" || id == "rocky" || id == "almalinux" || id == "ol" || id == "oracle" || id == "amzn":
		return "rhel"
	case strings.Contains(idLike, "suse") || id == "sles" || id == "opensuse" || id == "opensuse-leap" || id == "opensuse-tumbleweed":
		return "suse"
	case strings.Contains(idLike, "arch") || id == "arch" || id == "manjaro":
		return "arch"
	case strings.Contains(idLike, "alpine") || id == "alpine":
		return "alpine"
	default:
		return id
	}
}
