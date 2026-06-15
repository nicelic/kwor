package service

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestMigrateLegacySingboxConfigFragments(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "singbox")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatalf("create target dir failed: %v", err)
	}

	srcConfigDir := filepath.Join(root, ".config", "sing-box")
	srcCacheDir := filepath.Join(root, ".cache", "sing-box")
	if err := os.MkdirAll(srcConfigDir, 0o755); err != nil {
		t.Fatalf("create source config dir failed: %v", err)
	}
	if err := os.MkdirAll(srcCacheDir, 0o755); err != nil {
		t.Fatalf("create source cache dir failed: %v", err)
	}

	configFile := filepath.Join(srcConfigDir, "state.json")
	cacheFile := filepath.Join(srcCacheDir, "cache.db")
	if err := os.WriteFile(configFile, []byte(`{"ok":true}`), 0o644); err != nil {
		t.Fatalf("write source config file failed: %v", err)
	}
	if err := os.WriteFile(cacheFile, []byte("cache"), 0o644); err != nil {
		t.Fatalf("write source cache file failed: %v", err)
	}

	if err := migrateLegacySingboxConfigFragments(root, target); err != nil {
		t.Fatalf("migrate legacy singbox fragments failed: %v", err)
	}

	dstConfigFile := filepath.Join(target, ".config", "sing-box", "state.json")
	dstCacheFile := filepath.Join(target, ".cache", "sing-box", "cache.db")

	if _, err := os.Stat(dstConfigFile); err != nil {
		t.Fatalf("expected migrated config file at %s: %v", dstConfigFile, err)
	}
	if _, err := os.Stat(dstCacheFile); err != nil {
		t.Fatalf("expected migrated cache file at %s: %v", dstCacheFile, err)
	}

	if _, err := os.Stat(configFile); !os.IsNotExist(err) {
		t.Fatalf("expected source config file removed, got err=%v", err)
	}
	if _, err := os.Stat(cacheFile); !os.IsNotExist(err) {
		t.Fatalf("expected source cache file removed, got err=%v", err)
	}
}

func TestMigrateSingboxSubdirArtifactsToRoot(t *testing.T) {
	root := t.TempDir()
	sourceDir := filepath.Join(root, "singbox")
	if err := os.MkdirAll(filepath.Join(sourceDir, ".cache", "sing-box"), 0o755); err != nil {
		t.Fatalf("create source dir failed: %v", err)
	}

	binPath := filepath.Join(sourceDir, "sing-box")
	cachePath := filepath.Join(sourceDir, ".cache", "sing-box", "cache.db")
	if err := os.WriteFile(binPath, []byte("bin"), 0o755); err != nil {
		t.Fatalf("write source binary failed: %v", err)
	}
	if err := os.WriteFile(cachePath, []byte("cache"), 0o644); err != nil {
		t.Fatalf("write source cache failed: %v", err)
	}

	if err := migrateSingboxSubdirArtifactsToRoot(root); err != nil {
		t.Fatalf("migrate singbox subdir artifacts failed: %v", err)
	}

	if data, err := os.ReadFile(filepath.Join(root, "sing-box")); err != nil {
		t.Fatalf("expected binary moved to root: %v", err)
	} else if string(data) != "bin" {
		t.Fatalf("unexpected binary content: %q", string(data))
	}
	if data, err := os.ReadFile(filepath.Join(root, ".cache", "sing-box", "cache.db")); err != nil {
		t.Fatalf("expected cache moved to root: %v", err)
	} else if string(data) != "cache" {
		t.Fatalf("unexpected cache content: %q", string(data))
	}
	if _, err := os.Stat(sourceDir); !os.IsNotExist(err) {
		t.Fatalf("expected source singbox dir removed, got err=%v", err)
	}
}

func TestMigrateLegacyManagedCoreConfigFilesMovesRootConfigToSingboxSubdir(t *testing.T) {
	root := t.TempDir()
	rootConfig := filepath.Join(root, "config.json")
	if err := os.WriteFile(rootConfig, []byte(`{"log":{"level":"info"}}`), 0o644); err != nil {
		t.Fatalf("write legacy root config failed: %v", err)
	}

	if err := migrateLegacyManagedCoreConfigFiles(root); err != nil {
		t.Fatalf("migrate legacy managed core config files failed: %v", err)
	}

	targetConfig := filepath.Join(root, "singbox", "config.json")
	data, err := os.ReadFile(targetConfig)
	if err != nil {
		t.Fatalf("expected config moved to singbox subdir: %v", err)
	}
	if string(data) != `{"log":{"level":"info"}}` {
		t.Fatalf("unexpected migrated config content: %s", data)
	}
	if _, err := os.Stat(rootConfig); !os.IsNotExist(err) {
		t.Fatalf("expected root config removed, got err=%v", err)
	}
}

func TestMigrateLegacyMihomoHomeArtifacts(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "mihomo")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatalf("create target dir failed: %v", err)
	}

	cachePath := filepath.Join(root, "cache.db")
	geoIPPath := filepath.Join(root, "GeoIP.dat")
	uiDir := filepath.Join(root, "ui")
	uiFile := filepath.Join(uiDir, "index.html")

	if err := os.WriteFile(cachePath, []byte("cache"), 0o644); err != nil {
		t.Fatalf("write cache file failed: %v", err)
	}
	if err := os.WriteFile(geoIPPath, []byte("geoip"), 0o644); err != nil {
		t.Fatalf("write geodata file failed: %v", err)
	}
	if err := os.MkdirAll(uiDir, 0o755); err != nil {
		t.Fatalf("create ui dir failed: %v", err)
	}
	if err := os.WriteFile(uiFile, []byte("<html></html>"), 0o644); err != nil {
		t.Fatalf("write ui file failed: %v", err)
	}

	if err := migrateLegacyMihomoHomeArtifacts(root, target); err != nil {
		t.Fatalf("migrate legacy mihomo home artifacts failed: %v", err)
	}

	for _, expectedPath := range []string{
		filepath.Join(target, "cache.db"),
		filepath.Join(target, "GeoIP.dat"),
		filepath.Join(target, "ui", "index.html"),
	} {
		if _, err := os.Stat(expectedPath); err != nil {
			t.Fatalf("expected migrated file at %s: %v", expectedPath, err)
		}
	}

	if runtime.GOOS != "windows" {
		if info, err := os.Lstat(cachePath); err != nil {
			t.Fatalf("expected legacy cache path retained as compatibility symlink: %v", err)
		} else if info.Mode()&os.ModeSymlink == 0 {
			t.Fatalf("expected legacy cache path to be symlink, mode=%v", info.Mode())
		}
	}
}

func TestMigrateLegacyMihomoHomeArtifactsKeepsCompatibilityLinkIdempotent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink compatibility is only exercised on non-windows")
	}

	root := t.TempDir()
	target := filepath.Join(root, "mihomo")
	if err := os.MkdirAll(target, 0o755); err != nil {
		t.Fatalf("create target dir failed: %v", err)
	}

	cachePath := filepath.Join(root, "cache.db")
	if err := os.WriteFile(cachePath, []byte("cache"), 0o644); err != nil {
		t.Fatalf("write cache file failed: %v", err)
	}

	if err := migrateLegacyMihomoHomeArtifacts(root, target); err != nil {
		t.Fatalf("first migration failed: %v", err)
	}
	if err := migrateLegacyMihomoHomeArtifacts(root, target); err != nil {
		t.Fatalf("second migration failed: %v", err)
	}

	targetCachePath := filepath.Join(target, "cache.db")
	if data, err := os.ReadFile(targetCachePath); err != nil {
		t.Fatalf("expected migrated target cache file: %v", err)
	} else if string(data) != "cache" {
		t.Fatalf("unexpected target cache content: %q", string(data))
	}

	info, err := os.Lstat(cachePath)
	if err != nil {
		t.Fatalf("expected legacy compatibility symlink: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("expected legacy path to remain a symlink, mode=%v", info.Mode())
	}
	if !symlinkPointsToPath(cachePath, targetCachePath) {
		t.Fatalf("expected legacy symlink to point to migrated target")
	}
}

func TestMigrateLegacyCoreDirectoryNameConflict(t *testing.T) {
	root := t.TempDir()
	legacyPath := filepath.Join(root, "mihomo")
	if err := os.WriteFile(legacyPath, []byte("bin"), 0o755); err != nil {
		t.Fatalf("write legacy mihomo binary failed: %v", err)
	}

	if err := migrateLegacyCoreDirectoryNameConflict(root, "mihomo", "mihomo"); err != nil {
		t.Fatalf("migrate directory-name conflict failed: %v", err)
	}

	if info, err := os.Stat(legacyPath); err != nil {
		t.Fatalf("expected new mihomo directory: %v", err)
	} else if !info.IsDir() {
		t.Fatalf("expected new mihomo path to be a directory")
	}

	migratedBin := filepath.Join(root, "mihomo", "mihomo")
	if data, err := os.ReadFile(migratedBin); err != nil {
		t.Fatalf("expected migrated mihomo binary: %v", err)
	} else if string(data) != "bin" {
		t.Fatalf("unexpected migrated binary content: %q", string(data))
	}

	if err := migrateLegacyCoreDirectoryNameConflict(root, "mihomo", "mihomo"); err != nil {
		t.Fatalf("second conflict migration should be idempotent: %v", err)
	}
}

func TestEnsureCompatibilityLinksForLegacyParent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink compatibility is only exercised on non-windows")
	}

	root := t.TempDir()
	sourceParent := filepath.Join(root, ".config")
	sourceChild := filepath.Join(sourceParent, "clash")
	linkParent := filepath.Join(root, "mihomo", ".config")

	if err := os.MkdirAll(sourceChild, 0o755); err != nil {
		t.Fatalf("create source child failed: %v", err)
	}

	if err := ensureCompatibilityLinksForLegacyParent(sourceParent, linkParent); err != nil {
		t.Fatalf("ensure compatibility links failed: %v", err)
	}

	linkPath := filepath.Join(linkParent, "clash")
	info, err := os.Lstat(linkPath)
	if err != nil {
		t.Fatalf("expected compatibility link at %s: %v", linkPath, err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("expected compatibility link to be symlink, mode=%v", info.Mode())
	}
}
