package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestRuntimeSupportFilePathsFollowDataDir(t *testing.T) {
	binDir := GetBinDir()
	dataDir := filepath.Join(binDir, "Promanager_data")

	if got := GetRuntimeSupportDir(); got != dataDir {
		t.Fatalf("GetRuntimeSupportDir()=%q want %q", got, dataDir)
	}
	if got := GetRuntimeInstallScriptPath(); got != filepath.Join(dataDir, "install.sh") {
		t.Fatalf("GetRuntimeInstallScriptPath()=%q", got)
	}
	if got := GetRuntimeServiceFilePath(); got != filepath.Join(dataDir, "kwor.service") {
		t.Fatalf("GetRuntimeServiceFilePath()=%q", got)
	}
}

func TestMigrateLegacyRuntimeSupportFilesForBinDir(t *testing.T) {
	binDir := t.TempDir()
	legacyInstallPath := filepath.Join(binDir, "install.sh")
	legacyServicePath := filepath.Join(binDir, "kwor.service")

	if err := os.WriteFile(legacyInstallPath, []byte("#!/bin/sh\necho hi\n"), 0o755); err != nil {
		t.Fatalf("write legacy install.sh failed: %v", err)
	}
	if err := os.WriteFile(legacyServicePath, []byte("[Unit]\nDescription=kwor\n"), 0o644); err != nil {
		t.Fatalf("write legacy kwor.service failed: %v", err)
	}

	if err := migrateLegacyRuntimeSupportFilesForBinDir(binDir); err != nil {
		t.Fatalf("migrateLegacyRuntimeSupportFilesForBinDir failed: %v", err)
	}

	targetInstallPath := filepath.Join(binDir, "Promanager_data", "install.sh")
	targetServicePath := filepath.Join(binDir, "Promanager_data", "kwor.service")

	if _, err := os.Stat(targetInstallPath); err != nil {
		t.Fatalf("expected migrated install.sh, stat err=%v", err)
	}
	if _, err := os.Stat(targetServicePath); err != nil {
		t.Fatalf("expected migrated kwor.service, stat err=%v", err)
	}
	if _, err := os.Stat(legacyInstallPath); !os.IsNotExist(err) {
		t.Fatalf("expected legacy install.sh removed, stat err=%v", err)
	}
	if _, err := os.Stat(legacyServicePath); !os.IsNotExist(err) {
		t.Fatalf("expected legacy kwor.service removed, stat err=%v", err)
	}
}

func TestMigrateLegacyRuntimeSupportFilesForBinDirKeepsExistingRuntimeFiles(t *testing.T) {
	binDir := t.TempDir()
	targetDir := filepath.Join(binDir, "Promanager_data")
	targetInstallPath := filepath.Join(targetDir, "install.sh")
	targetServicePath := filepath.Join(targetDir, "kwor.service")
	legacyInstallPath := filepath.Join(binDir, "install.sh")
	legacyServicePath := filepath.Join(binDir, "kwor.service")

	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		t.Fatalf("mkdir target dir failed: %v", err)
	}
	if err := os.WriteFile(targetInstallPath, []byte("#!/bin/sh\necho new\n"), 0o755); err != nil {
		t.Fatalf("write target install.sh failed: %v", err)
	}
	if err := os.WriteFile(targetServicePath, []byte("[Unit]\nDescription=new\n"), 0o644); err != nil {
		t.Fatalf("write target kwor.service failed: %v", err)
	}
	if err := os.WriteFile(legacyInstallPath, []byte("#!/bin/sh\necho legacy\n"), 0o755); err != nil {
		t.Fatalf("write legacy install.sh failed: %v", err)
	}
	if err := os.WriteFile(legacyServicePath, []byte("[Unit]\nDescription=legacy\n"), 0o644); err != nil {
		t.Fatalf("write legacy kwor.service failed: %v", err)
	}

	if err := migrateLegacyRuntimeSupportFilesForBinDir(binDir); err != nil {
		t.Fatalf("migrateLegacyRuntimeSupportFilesForBinDir failed: %v", err)
	}

	if got, err := os.ReadFile(targetInstallPath); err != nil {
		t.Fatalf("read target install.sh failed: %v", err)
	} else if string(got) != "#!/bin/sh\necho new\n" {
		t.Fatalf("target install.sh content=%q", string(got))
	}
	if got, err := os.ReadFile(targetServicePath); err != nil {
		t.Fatalf("read target kwor.service failed: %v", err)
	} else if string(got) != "[Unit]\nDescription=new\n" {
		t.Fatalf("target kwor.service content=%q", string(got))
	}
	if _, err := os.Stat(legacyInstallPath); !os.IsNotExist(err) {
		t.Fatalf("expected legacy install.sh removed, stat err=%v", err)
	}
	if _, err := os.Stat(legacyServicePath); !os.IsNotExist(err) {
		t.Fatalf("expected legacy kwor.service removed, stat err=%v", err)
	}
}

func TestShouldMigrateLegacyRuntimeSupportFilesForBinDirSkipsSourceTree(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("source-tree migration guard is only relevant on linux")
	}

	binDir := t.TempDir()
	for _, path := range []string{
		filepath.Join(binDir, "go.mod"),
		filepath.Join(binDir, "main.go"),
		filepath.Join(binDir, "install.sh"),
		filepath.Join(binDir, "kwor.service"),
	} {
		if err := os.WriteFile(path, []byte("placeholder\n"), 0o644); err != nil {
			t.Fatalf("write %s failed: %v", path, err)
		}
	}
	if err := os.Mkdir(filepath.Join(binDir, "service"), 0o755); err != nil {
		t.Fatalf("mkdir service failed: %v", err)
	}

	if shouldMigrateLegacyRuntimeSupportFilesForBinDir(binDir) {
		t.Fatalf("expected source tree bin dir to skip runtime support file migration")
	}
}

func TestShouldMigrateLegacyRuntimeSupportFilesForBinDirAllowsInstalledLayout(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("runtime support migration only runs on linux")
	}

	binDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(binDir, "kwor"), []byte("bin\n"), 0o755); err != nil {
		t.Fatalf("write kwor binary marker failed: %v", err)
	}
	if !shouldMigrateLegacyRuntimeSupportFilesForBinDir(binDir) {
		t.Fatalf("expected installed layout to allow runtime support file migration")
	}
}
