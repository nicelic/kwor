package config

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	runtimeInstallScriptFileName = "install.sh"
	runtimeServiceFileName       = "kwor.service"
)

func GetRuntimeSupportDir() string {
	return GetDataDir()
}

func GetRuntimeInstallScriptPath() string {
	return filepath.Join(GetRuntimeSupportDir(), runtimeInstallScriptFileName)
}

func GetRuntimeServiceFilePath() string {
	return filepath.Join(GetRuntimeSupportDir(), runtimeServiceFileName)
}

func GetLegacyRuntimeInstallScriptPath() string {
	return filepath.Join(GetBinDir(), runtimeInstallScriptFileName)
}

func GetLegacyRuntimeServiceFilePath() string {
	return filepath.Join(GetBinDir(), runtimeServiceFileName)
}

func MigrateLegacyRuntimeSupportFiles() error {
	binDir := GetBinDir()
	if !shouldMigrateLegacyRuntimeSupportFilesForBinDir(binDir) {
		return nil
	}
	return migrateLegacyRuntimeSupportFilesForBinDir(binDir)
}

func shouldMigrateLegacyRuntimeSupportFilesForBinDir(binDir string) bool {
	if runtime.GOOS != "linux" {
		return false
	}
	return !looksLikeRuntimeSupportSourceTree(binDir)
}

func looksLikeRuntimeSupportSourceTree(binDir string) bool {
	binDir = filepath.Clean(strings.TrimSpace(binDir))
	if binDir == "" || binDir == "." {
		return false
	}
	if !pathExists(filepath.Join(binDir, "go.mod")) {
		return false
	}
	for _, marker := range []string{"main.go", ".git", "cmd", "service"} {
		if pathExists(filepath.Join(binDir, marker)) {
			return true
		}
	}
	return false
}

type runtimeSupportFileMapping struct {
	legacyPath string
	targetPath string
}

func migrateLegacyRuntimeSupportFilesForBinDir(binDir string) error {
	binDir = filepath.Clean(strings.TrimSpace(binDir))
	if binDir == "" || binDir == "." {
		return nil
	}

	mappings := []runtimeSupportFileMapping{
		{
			legacyPath: filepath.Join(binDir, runtimeInstallScriptFileName),
			targetPath: filepath.Join(binDir, "Promanager_data", runtimeInstallScriptFileName),
		},
		{
			legacyPath: filepath.Join(binDir, runtimeServiceFileName),
			targetPath: filepath.Join(binDir, "Promanager_data", runtimeServiceFileName),
		},
	}

	errs := make([]string, 0, len(mappings))
	for _, mapping := range mappings {
		if err := migrateLegacyRuntimeSupportFile(mapping.legacyPath, mapping.targetPath); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return fmt.Errorf("%s", strings.Join(errs, "; "))
}

func migrateLegacyRuntimeSupportFile(legacyPath string, targetPath string) error {
	legacyPath = filepath.Clean(strings.TrimSpace(legacyPath))
	targetPath = filepath.Clean(strings.TrimSpace(targetPath))
	if legacyPath == "" || targetPath == "" || legacyPath == targetPath {
		return nil
	}

	info, err := os.Stat(legacyPath)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read legacy runtime support file %s failed: %w", legacyPath, err)
	}
	if info.IsDir() {
		return nil
	}

	targetInfo, err := os.Stat(targetPath)
	if err == nil {
		if targetInfo.IsDir() {
			return fmt.Errorf("runtime support target path %s is a directory", targetPath)
		}
		if err := os.Remove(legacyPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("cleanup legacy runtime support file %s failed: %w", legacyPath, err)
		}
		return nil
	}
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("read runtime support target file %s failed: %w", targetPath, err)
	}

	if err := os.MkdirAll(filepath.Dir(targetPath), 0o740); err != nil {
		return fmt.Errorf("create runtime support dir for %s failed: %w", targetPath, err)
	}
	if err := copyFile(legacyPath, targetPath); err != nil {
		return fmt.Errorf("migrate legacy runtime support file %s failed: %w", legacyPath, err)
	}
	if err := os.Chmod(targetPath, info.Mode().Perm()); err != nil {
		return fmt.Errorf("restore runtime support file mode for %s failed: %w", targetPath, err)
	}
	if err := os.Remove(legacyPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("cleanup legacy runtime support file %s failed: %w", legacyPath, err)
	}
	return nil
}
