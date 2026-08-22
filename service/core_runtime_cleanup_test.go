package service

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCleanupManagedCoreRuntimeArtifacts(t *testing.T) {
	coreDir := t.TempDir()
	binName := "sing-box"

	binPath := filepath.Join(coreDir, binName)
	if err := os.WriteFile(binPath, []byte("bin"), 0o755); err != nil {
		t.Fatalf("write binary file failed: %v", err)
	}

	tmpArchive := filepath.Join(coreDir, "sing-box-custom-download.tar.gz")
	if err := os.WriteFile(tmpArchive, []byte("tmp"), 0o644); err != nil {
		t.Fatalf("write tmp archive failed: %v", err)
	}
	configPath := filepath.Join(coreDir, "config.json")
	if err := os.WriteFile(configPath, []byte("config"), 0o644); err != nil {
		t.Fatalf("write config file failed: %v", err)
	}

	cacheDir := filepath.Join(coreDir, ".cache", "sing-box")
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		t.Fatalf("create cache dir failed: %v", err)
	}
	cacheFile := filepath.Join(cacheDir, "cache.db")
	if err := os.WriteFile(cacheFile, []byte("cache"), 0o644); err != nil {
		t.Fatalf("write cache file failed: %v", err)
	}

	if err := cleanupManagedCoreRuntimeArtifacts(coreDir, binName); err != nil {
		t.Fatalf("cleanup managed core runtime artifacts failed: %v", err)
	}

	if _, err := os.Stat(binPath); err != nil {
		t.Fatalf("expected binary file to remain: %v", err)
	}
	if _, err := os.Stat(configPath); err != nil {
		t.Fatalf("expected config file to remain: %v", err)
	}
	if _, err := os.Stat(tmpArchive); !os.IsNotExist(err) {
		t.Fatalf("expected tmp archive removed, got err=%v", err)
	}
	if _, err := os.Stat(cacheFile); !os.IsNotExist(err) {
		t.Fatalf("expected cache file removed, got err=%v", err)
	}
}

func TestCleanupManagedSingboxRootRuntimeArtifactsKeepsSharedFiles(t *testing.T) {
	coreDir := t.TempDir()

	keepPaths := []string{
		filepath.Join(coreDir, "config.json"),
		filepath.Join(coreDir, "mihomo", "server.yaml"),
		filepath.Join(coreDir, "sing-box"),
	}
	for _, path := range keepPaths {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create keep parent failed: %v", err)
		}
		if err := os.WriteFile(path, []byte("keep"), 0o644); err != nil {
			t.Fatalf("write keep file failed: %v", err)
		}
	}

	removePaths := []string{
		filepath.Join(coreDir, "sing-box-custom-download.tar.gz"),
		filepath.Join(coreDir, ".cache", "sing-box", "cache.db"),
		filepath.Join(coreDir, ".config", "sing-box", "state.json"),
	}
	for _, path := range removePaths {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatalf("create remove parent failed: %v", err)
		}
		if err := os.WriteFile(path, []byte("remove"), 0o644); err != nil {
			t.Fatalf("write remove file failed: %v", err)
		}
	}

	if err := cleanupManagedSingboxRootRuntimeArtifacts(coreDir); err != nil {
		t.Fatalf("cleanup singbox root runtime artifacts failed: %v", err)
	}

	for _, path := range keepPaths {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected shared file kept at %s: %v", path, err)
		}
	}
	for _, path := range removePaths {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("expected runtime artifact removed at %s, got err=%v", path, err)
		}
	}
}

func TestCleanupStaleManagedCoreRuntimeWorkspacesRemovesOnlyOwnedNames(t *testing.T) {
	singboxCoreDir := t.TempDir()
	mihomoCoreDir := t.TempDir()

	removedPaths := []string{
		filepath.Join(singboxCoreDir, "extract_tmp_singbox"),
		filepath.Join(singboxCoreDir, singboxCoreInstallStagePrefix+"new"),
		filepath.Join(singboxCoreDir, singboxCoreInstallBackupPrefix+"old"),
		filepath.Join(mihomoCoreDir, "extract_tmp_mihomo"),
		filepath.Join(mihomoCoreDir, mihomoCoreInstallStagePrefix+"new"),
		filepath.Join(mihomoCoreDir, mihomoCoreInstallBackupPrefix+"old"),
	}
	for _, dir := range removedPaths {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("create stale workspace %s failed: %v", dir, err)
		}
		if err := os.WriteFile(filepath.Join(dir, "partial"), []byte("temporary"), 0o600); err != nil {
			t.Fatalf("write stale workspace file %s failed: %v", dir, err)
		}
	}

	keptPaths := []string{
		filepath.Join(singboxCoreDir, "sing-box"),
		filepath.Join(singboxCoreDir, "config.json"),
		filepath.Join(singboxCoreDir, "custom-rule-set.srs"),
		filepath.Join(mihomoCoreDir, "mihomo"),
		filepath.Join(mihomoCoreDir, "server.yaml"),
		filepath.Join(mihomoCoreDir, "custom-provider.yaml"),
	}
	for _, filePath := range keptPaths {
		if err := os.WriteFile(filePath, []byte("keep"), 0o600); err != nil {
			t.Fatalf("write preserved runtime file %s failed: %v", filePath, err)
		}
	}

	if err := cleanupStaleManagedCoreRuntimeWorkspaces(singboxCoreDir, mihomoCoreDir); err != nil {
		t.Fatalf("cleanup stale managed Core workspaces failed: %v", err)
	}

	for _, dir := range removedPaths {
		if _, err := os.Stat(dir); !os.IsNotExist(err) {
			t.Fatalf("expected stale workspace removed at %s, got err=%v", dir, err)
		}
	}
	for _, filePath := range keptPaths {
		if _, err := os.Stat(filePath); err != nil {
			t.Fatalf("expected preserved runtime file at %s: %v", filePath, err)
		}
	}
}
