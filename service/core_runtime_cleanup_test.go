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
