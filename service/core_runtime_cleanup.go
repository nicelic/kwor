package service

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// cleanupManagedCoreRuntimeArtifacts removes remaining runtime artifacts inside
// an isolated per-core directory. Do not call it on the shared core root.
func cleanupManagedCoreRuntimeArtifacts(coreDir string, binName string) error {
	coreDir = strings.TrimSpace(coreDir)
	if coreDir == "" {
		return nil
	}

	if _, err := os.Stat(coreDir); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}

	entries, err := os.ReadDir(coreDir)
	if err != nil {
		return err
	}

	binName = strings.TrimSpace(binName)
	for _, entry := range entries {
		name := strings.TrimSpace(entry.Name())
		if name == "" {
			continue
		}
		// Keep binary removal behavior explicit in caller.
		if binName != "" && strings.EqualFold(name, binName) {
			continue
		}
		if isManagedCoreConfigArtifactName(name) {
			continue
		}

		targetPath := filepath.Join(coreDir, name)
		if err := os.RemoveAll(targetPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove managed core artifact %s failed: %w", targetPath, err)
		}
	}

	return nil
}

func isManagedCoreConfigArtifactName(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "config.json", "server.yaml", strings.ToLower(mihomoInboundMetaFilename):
		return true
	default:
		return false
	}
}

func cleanupManagedSingboxRootRuntimeArtifacts(coreDir string) error {
	coreDir = strings.TrimSpace(coreDir)
	if coreDir == "" {
		return nil
	}

	if _, err := os.Stat(coreDir); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}

	removePaths := []string{
		filepath.Join(coreDir, ".cache", "sing-box"),
		filepath.Join(coreDir, ".config", "sing-box"),
	}
	for _, path := range removePaths {
		if err := os.RemoveAll(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove sing-box runtime artifact %s failed: %w", path, err)
		}
	}

	patterns := []string{
		filepath.Join(coreDir, "sing-box-download*"),
		filepath.Join(coreDir, "sing-box-custom-download*"),
		filepath.Join(coreDir, "sing-box-*.tar*"),
		filepath.Join(coreDir, "sing-box-*.zip"),
		filepath.Join(coreDir, "sing-box-*.gz"),
	}
	for _, pattern := range patterns {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return err
		}
		for _, match := range matches {
			if filepath.Base(match) == "sing-box" || filepath.Base(match) == "sing-box.exe" {
				continue
			}
			if err := os.RemoveAll(match); err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("remove sing-box runtime artifact %s failed: %w", match, err)
			}
		}
	}

	_ = removeDirIfEmpty(filepath.Join(coreDir, ".cache"))
	_ = removeDirIfEmpty(filepath.Join(coreDir, ".config"))
	return nil
}
