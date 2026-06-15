package service

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestNormalizeVnstatInstallMethod(t *testing.T) {
	if got := normalizeVnstatInstallMethod("", "apt-get"); got != vnstatInstallMethodSystemPackage {
		t.Fatalf("normalizeVnstatInstallMethod blank+apt-get = %q, want %q", got, vnstatInstallMethodSystemPackage)
	}
	if got := normalizeVnstatInstallMethod(vnstatInstallMethodGitHubRelease, "apt-get"); got != vnstatInstallMethodGitHubRelease {
		t.Fatalf("normalizeVnstatInstallMethod should preserve explicit method, got %q", got)
	}
	if got := normalizeVnstatInstallMethod("", "custom"); got != "" {
		t.Fatalf("normalizeVnstatInstallMethod blank+custom = %q, want empty", got)
	}
}

func TestSelectVnstatReleaseSourceAsset(t *testing.T) {
	release := GitHubRelease{
		TagName: "v2.13",
		Assets: []GitHubAsset{
			{Name: "vnstat-2.13.tar.gz.asc", BrowserDownloadURL: "https://example.invalid/vnstat-2.13.tar.gz.asc"},
			{Name: "vnstat-2.13.tar.gz", BrowserDownloadURL: "https://example.invalid/vnstat-2.13.tar.gz"},
		},
	}
	asset, err := selectVnstatReleaseSourceAsset(release)
	if err != nil {
		t.Fatalf("selectVnstatReleaseSourceAsset returned error: %v", err)
	}
	if asset.Name != "vnstat-2.13.tar.gz" {
		t.Fatalf("selectVnstatReleaseSourceAsset picked %q, want source tarball", asset.Name)
	}
}

func TestCollectManagedSourceVnstatPathsFiltersExpectedFiles(t *testing.T) {
	stageRoot := t.TempDir()
	createManagedSourceFile(t, stageRoot, "usr/bin/vnstat")
	createManagedSourceFile(t, stageRoot, "usr/sbin/vnstatd")
	createManagedSourceFile(t, stageRoot, "usr/bin/vnstati")
	createManagedSourceFile(t, stageRoot, "etc/vnstat.conf")
	createManagedSourceFile(t, stageRoot, "usr/share/man/man1/vnstat.1")

	got := collectManagedSourceVnstatPaths(stageRoot)
	want := []string{
		"/etc/vnstat.conf",
		"/usr/bin/vnstat",
		"/usr/bin/vnstati",
		"/usr/sbin/vnstatd",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("collectManagedSourceVnstatPaths = %v, want %v", got, want)
	}
}

func TestBuildVnstatInstallUnavailableErrorIncludesBothSources(t *testing.T) {
	err := buildVnstatInstallUnavailableError(
		os.ErrNotExist,
		os.ErrPermission,
	)
	if err == nil {
		t.Fatal("buildVnstatInstallUnavailableError returned nil")
	}
	text := err.Error()
	if !strings.Contains(text, "无法下载 vnstat，功能无法使用") {
		t.Fatalf("error %q does not contain user-facing summary", text)
	}
	if !strings.Contains(text, "系统软件源安装失败") || !strings.Contains(text, "GitHub 官方版本安装失败") {
		t.Fatalf("error %q does not include both failure sources", text)
	}
}

func createManagedSourceFile(t *testing.T, stageRoot string, relPath string) {
	t.Helper()

	fullPath := filepath.Join(stageRoot, filepath.FromSlash(relPath))
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		t.Fatalf("mkdir %s failed: %v", filepath.Dir(fullPath), err)
	}
	if err := os.WriteFile(fullPath, []byte("test"), 0o644); err != nil {
		t.Fatalf("write %s failed: %v", fullPath, err)
	}
}
