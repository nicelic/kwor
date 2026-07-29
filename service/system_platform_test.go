package service

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
)

func TestRefreshSystemPlatformPersistsSingleStartupSnapshot(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "system-platform.db")
	if err := database.InitDB(dbPath); err != nil {
		t.Fatalf("initialize database failed: %v", err)
	}
	if sqlDB, err := database.GetDB().DB(); err == nil && sqlDB != nil {
		t.Cleanup(func() {
			_ = sqlDB.Close()
		})
	}

	oldGOOS := systemPlatformRuntimeGOOS
	oldGOARCH := systemPlatformRuntimeGOARCH
	oldReadFile := systemPlatformReadFile
	oldDetectLibc := systemPlatformDetectLibc
	oldDetectKernelRelease := systemPlatformDetectKernelRelease
	oldNow := systemPlatformNow
	t.Cleanup(func() {
		systemPlatformRuntimeGOOS = oldGOOS
		systemPlatformRuntimeGOARCH = oldGOARCH
		systemPlatformReadFile = oldReadFile
		systemPlatformDetectLibc = oldDetectLibc
		systemPlatformDetectKernelRelease = oldDetectKernelRelease
		systemPlatformNow = oldNow
	})

	systemPlatformRuntimeGOOS = "linux"
	systemPlatformRuntimeGOARCH = "amd64"
	systemPlatformReadFile = func(string) ([]byte, error) {
		return []byte("ID=ubuntu\nID_LIKE=debian\nVERSION_ID=24.04\nVERSION_CODENAME=noble\n"), nil
	}
	systemPlatformDetectLibc = func(string, string) string { return "glibc" }
	systemPlatformDetectKernelRelease = func() string { return "6.8.0-test" }
	systemPlatformNow = func() time.Time { return time.Unix(1_700_000_000, 0) }

	first, err := RefreshSystemPlatform()
	if err != nil {
		t.Fatalf("refresh system platform failed: %v", err)
	}
	if first.OS != "linux" || first.Architecture != "amd64" || first.SystemFamily != "debian" || first.Libc != "glibc" || first.KernelRelease != "6.8.0-test" {
		t.Fatalf("unexpected platform snapshot: %#v", first)
	}
	if first.SystemID != "ubuntu" || first.VersionCodename != "noble" {
		t.Fatalf("unexpected release fields: %#v", first)
	}
	if !IsSystemPlatformLinux() || GetSystemPlatformOS() != "linux" || GetSystemPlatformArchitecture() != "amd64" || GetSystemPlatformLibc() != "glibc" {
		t.Fatalf("persisted platform readers returned unexpected values")
	}

	systemPlatformRuntimeGOARCH = "arm64"
	systemPlatformNow = func() time.Time { return time.Unix(1_700_000_100, 0) }
	second, err := RefreshSystemPlatform()
	if err != nil {
		t.Fatalf("refresh replacement platform failed: %v", err)
	}
	if second.Id != first.Id || second.Architecture != "arm64" || second.DetectedAt != 1_700_000_100 {
		t.Fatalf("expected singleton platform record update, got %#v", second)
	}

	var count int64
	if err := database.GetDB().Model(&model.SystemPlatform{}).Count(&count).Error; err != nil {
		t.Fatalf("count system platform records failed: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected exactly one system platform record, got %d", count)
	}
}

func TestSystemPlatformReadersDoNotWaitForDatabaseConnection(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "system-platform-cache.db")); err != nil {
		t.Fatalf("initialize database failed: %v", err)
	}
	if sqlDB, err := database.GetDB().DB(); err == nil && sqlDB != nil {
		t.Cleanup(func() {
			_ = sqlDB.Close()
		})
	}
	if _, err := RefreshSystemPlatform(); err != nil {
		t.Fatalf("refresh system platform failed: %v", err)
	}

	tx := database.GetDB().Begin()
	if tx.Error != nil {
		t.Fatalf("begin transaction failed: %v", tx.Error)
	}
	defer tx.Rollback()

	type platformResult struct {
		platform *model.SystemPlatform
		err      error
	}
	resultCh := make(chan platformResult, 1)
	go func() {
		platform, err := GetSystemPlatform()
		resultCh <- platformResult{platform: platform, err: err}
	}()

	select {
	case result := <-resultCh:
		if result.err != nil || result.platform == nil {
			t.Fatalf("read cached platform failed: %v", result.err)
		}
	case <-time.After(time.Second):
		t.Fatal("platform reader waited for the database connection")
	}
}

func TestKernelOverviewSeparatesLinuxAvailabilityFromDebianPackageSupport(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "kernel-platform.db")); err != nil {
		t.Fatalf("initialize database failed: %v", err)
	}
	if sqlDB, err := database.GetDB().DB(); err == nil && sqlDB != nil {
		t.Cleanup(func() {
			_ = sqlDB.Close()
		})
	}

	oldGOOS := systemPlatformRuntimeGOOS
	oldGOARCH := systemPlatformRuntimeGOARCH
	oldReadFile := systemPlatformReadFile
	oldDetectLibc := systemPlatformDetectLibc
	oldDetectKernelRelease := systemPlatformDetectKernelRelease
	oldNow := systemPlatformNow
	t.Cleanup(func() {
		systemPlatformRuntimeGOOS = oldGOOS
		systemPlatformRuntimeGOARCH = oldGOARCH
		systemPlatformReadFile = oldReadFile
		systemPlatformDetectLibc = oldDetectLibc
		systemPlatformDetectKernelRelease = oldDetectKernelRelease
		systemPlatformNow = oldNow
	})

	systemPlatformRuntimeGOOS = "linux"
	systemPlatformRuntimeGOARCH = "amd64"
	systemPlatformReadFile = func(string) ([]byte, error) {
		return []byte("ID=fedora\nID_LIKE=fedora\nVERSION_ID=42\n"), nil
	}
	systemPlatformDetectLibc = func(string, string) string { return "glibc" }
	systemPlatformDetectKernelRelease = func() string { return "6.12.0-test" }
	systemPlatformNow = time.Now
	if _, err := RefreshSystemPlatform(); err != nil {
		t.Fatalf("refresh system platform failed: %v", err)
	}
	overview, err := (&KernelManagerService{}).GetOverview("xanmod")
	if err != nil {
		t.Fatalf("get kernel overview failed: %v", err)
	}
	if !overview.Linux || overview.Supported || overview.SystemFamily != "rhel" {
		t.Fatalf("expected Linux runtime with unsupported Debian package management, got %#v", overview)
	}
	if overview.CurrentKernel != "6.12.0-test" {
		t.Fatalf("unexpected current kernel: %q", overview.CurrentKernel)
	}
}
