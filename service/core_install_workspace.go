package service

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/alireza0/s-ui/logger"
)

const (
	singboxCoreInstallStagePrefix  = "sing-box-stage-"
	singboxCoreInstallBackupPrefix = "sing-box-backup-"
	mihomoCoreInstallStagePrefix   = "mihomo-stage-"
	mihomoCoreInstallBackupPrefix  = "mihomo-backup-"
)

type managedCoreBinaryActivation struct {
	targetPath       string
	backupRoot       string
	backupPath       string
	replacedExisting bool
	finished         bool
}

func createManagedCoreInstallWorkspace(parentDir string, prefix string) (string, func(), error) {
	parentDir = filepath.Clean(strings.TrimSpace(parentDir))
	prefix = strings.TrimSpace(prefix)
	if parentDir == "" || parentDir == "." {
		return "", nil, fmt.Errorf("managed core workspace parent directory is empty")
	}
	if prefix == "" {
		return "", nil, fmt.Errorf("managed core workspace prefix is empty")
	}
	if err := os.MkdirAll(parentDir, 0o755); err != nil {
		return "", nil, fmt.Errorf("create managed core workspace parent directory failed: %w", err)
	}

	baseDir, err := os.MkdirTemp(parentDir, prefix)
	if err != nil {
		return "", nil, fmt.Errorf("create managed core workspace failed: %w", err)
	}
	cleanup := func() {
		if err := os.RemoveAll(baseDir); err != nil && !os.IsNotExist(err) {
			logger.Warning("cleanup managed core workspace failed: ", err)
		}
	}
	return baseDir, cleanup, nil
}

func cleanupStaleManagedCoreInstallWorkspaces(parentDir string, prefixes ...string) error {
	parentDir = filepath.Clean(strings.TrimSpace(parentDir))
	if parentDir == "" || parentDir == "." {
		return nil
	}

	entries, err := os.ReadDir(parentDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("list managed core workspaces failed: %w", err)
	}

	for _, entry := range entries {
		if entry == nil {
			continue
		}
		name := strings.TrimSpace(entry.Name())
		if !isManagedCoreInstallWorkspaceName(name, prefixes...) {
			continue
		}
		target := filepath.Join(parentDir, name)
		if err := os.RemoveAll(target); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove stale managed core workspace %s failed: %w", target, err)
		}
	}
	return nil
}

func isManagedCoreInstallWorkspaceName(name string, prefixes ...string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	for _, prefix := range prefixes {
		prefix = strings.TrimSpace(prefix)
		if prefix == "" {
			continue
		}
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

func activateManagedCoreBinaryInstall(targetDir string, binName string, stagedDir string, backupPrefix string) (*managedCoreBinaryActivation, error) {
	targetDir = filepath.Clean(strings.TrimSpace(targetDir))
	binName = strings.TrimSpace(binName)
	stagedDir = filepath.Clean(strings.TrimSpace(stagedDir))
	backupPrefix = strings.TrimSpace(backupPrefix)
	if targetDir == "" || targetDir == "." {
		return nil, fmt.Errorf("managed core target directory is empty")
	}
	if stagedDir == "" || stagedDir == "." {
		return nil, fmt.Errorf("managed core staged directory is empty")
	}
	if binName == "" {
		return nil, fmt.Errorf("managed core binary name is empty")
	}

	stagedBinPath := filepath.Join(stagedDir, binName)
	if !pathExists(stagedBinPath) {
		return nil, fmt.Errorf("staged core binary was not found: %s", stagedBinPath)
	}

	targetPath := filepath.Join(targetDir, binName)
	activation := &managedCoreBinaryActivation{
		targetPath: targetPath,
	}

	if pathExists(targetPath) {
		backupRoot, cleanupBackup, err := createManagedCoreInstallWorkspace(targetDir, backupPrefix)
		if err != nil {
			return nil, err
		}
		activation.backupRoot = backupRoot
		activation.backupPath = filepath.Join(backupRoot, binName)
		activation.replacedExisting = true

		if err := moveManagedCoreFile(targetPath, activation.backupPath); err != nil {
			cleanupBackup()
			return nil, fmt.Errorf("backup current core binary failed: %w", err)
		}
	}

	if err := moveManagedCoreFile(stagedBinPath, targetPath); err != nil {
		if rollbackErr := activation.Rollback(); rollbackErr != nil {
			return nil, fmt.Errorf("activate staged core binary failed: %v; rollback failed: %v", err, rollbackErr)
		}
		return nil, fmt.Errorf("activate staged core binary failed: %w", err)
	}

	return activation, nil
}

func activateManagedCoreBinaryInstallWithRuntime(
	wasRunning bool,
	stopRuntime func() error,
	beforeActivate func(),
	restoreRuntime func() error,
	activate func() (*managedCoreBinaryActivation, error),
) (*managedCoreBinaryActivation, string, error) {
	if activate == nil {
		return nil, "", fmt.Errorf("managed core activation callback is nil")
	}
	if wasRunning {
		if stopRuntime == nil {
			return nil, "", fmt.Errorf("managed core stop callback is nil")
		}
		if restoreRuntime == nil {
			return nil, "", fmt.Errorf("managed core restore callback is nil")
		}
		if err := stopRuntime(); err != nil {
			return nil, coreDownloadStageStopping, fmt.Errorf("stop current core runtime failed: %w", err)
		}
	}
	if beforeActivate != nil {
		beforeActivate()
	}

	activation, err := activate()
	if err == nil {
		return activation, "", nil
	}
	if !wasRunning {
		return nil, coreDownloadStageReplacing, err
	}
	if restoreErr := restoreRuntime(); restoreErr != nil {
		return nil, coreDownloadStageReplacing, fmt.Errorf("activate staged core failed: %v; restart previous core runtime failed: %v", err, restoreErr)
	}
	return nil, coreDownloadStageReplacing, fmt.Errorf("activate staged core failed after stopping previous core runtime, and the previous runtime was restored: %w", err)
}

func (a *managedCoreBinaryActivation) Commit() error {
	if a == nil || a.finished {
		return nil
	}
	a.finished = true
	if strings.TrimSpace(a.backupRoot) == "" {
		return nil
	}
	if err := os.RemoveAll(a.backupRoot); err != nil && !os.IsNotExist(err) {
		logger.Warning("cleanup managed core backup workspace failed: ", err)
	}
	return nil
}

func (a *managedCoreBinaryActivation) Rollback() error {
	if a == nil || a.finished {
		return nil
	}
	a.finished = true

	if targetPath := strings.TrimSpace(a.targetPath); targetPath != "" {
		if err := os.Remove(targetPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove activated core binary failed: %w", err)
		}
	}

	if a.replacedExisting && strings.TrimSpace(a.backupPath) != "" && pathExists(a.backupPath) {
		if err := moveManagedCoreFile(a.backupPath, a.targetPath); err != nil {
			return fmt.Errorf("restore previous core binary failed: %w", err)
		}
	}

	if strings.TrimSpace(a.backupRoot) != "" {
		if err := os.RemoveAll(a.backupRoot); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("cleanup managed core backup workspace failed: %w", err)
		}
	}

	return nil
}

func moveManagedCoreFile(srcPath string, dstPath string) error {
	srcPath = filepath.Clean(strings.TrimSpace(srcPath))
	dstPath = filepath.Clean(strings.TrimSpace(dstPath))
	if srcPath == "" || srcPath == "." {
		return fmt.Errorf("managed core source path is empty")
	}
	if dstPath == "" || dstPath == "." {
		return fmt.Errorf("managed core target path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
		return err
	}
	if err := os.Rename(srcPath, dstPath); err == nil {
		return nil
	}
	if err := copyFile(srcPath, dstPath); err != nil {
		return err
	}
	if err := os.Remove(srcPath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
