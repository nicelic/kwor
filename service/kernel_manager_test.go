package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/alireza0/s-ui/config"
	"github.com/alireza0/s-ui/database"
)

func TestExtractNetSFFilesJSON(t *testing.T) {
	html := `<!doctype html><script>net.sf.files = {"a":{"name":"a","type":"d"},"b":{"name":"b","type":"f"}};net.sf.staging_days=3;</script>`
	blob, err := extractNetSFFilesJSON(html)
	if err != nil {
		t.Fatalf("extractNetSFFilesJSON failed: %v", err)
	}

	got := map[string]sourceForgeFileEntry{}
	if err = json.Unmarshal([]byte(blob), &got); err != nil {
		t.Fatalf("unmarshal extracted json failed: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(got))
	}
	if got["a"].Type != "d" {
		t.Fatalf("expected entry a to be directory, got %#v", got["a"])
	}
}

func TestExtractNetSFFilesJSONMissingMarker(t *testing.T) {
	_, err := extractNetSFFilesJSON(`<html><body>missing marker</body></html>`)
	if err == nil {
		t.Fatalf("expected error when marker is missing")
	}
}

func TestKernelManagerTraversalAndPackageFilter(t *testing.T) {
	oldBaseURL := xanmodSourceForgeBaseURL
	defer func() { xanmodSourceForgeBaseURL = oldBaseURL }()

	pages := map[string]map[string]sourceForgeFileEntry{
		"/releases/lts/": {
			"6.18.27-xanmod1":     {Name: "6.18.27-xanmod1", Type: "d"},
			"6.18.25-xanmod1":     {Name: "6.18.25-xanmod1", Type: "d"},
			"6.18.4-rt-xanmod1":   {Name: "6.18.4-rt-xanmod1", Type: "d"},
			"6.6.63-rt46-xanmod1": {Name: "6.6.63-rt46-xanmod1", Type: "d"},
			"README.txt":          {Name: "README.txt", Type: "f"},
		},
		"/releases/lts/6.18.27-xanmod1/": {
			"6.18.27-x64v3-xanmod1": {Name: "6.18.27-x64v3-xanmod1", Type: "d"},
			"6.18.27-x64v2-xanmod1": {Name: "6.18.27-x64v2-xanmod1", Type: "d"},
			"6.18.27-x64v1-xanmod1": {Name: "6.18.27-x64v1-xanmod1", Type: "d"},
		},
		"/releases/lts/6.18.27-xanmod1/6.18.27-x64v3-xanmod1/": {
			"linux-image-6.18.27-x64v3-xanmod1_1_amd64.deb": {
				Name:        "linux-image-6.18.27-x64v3-xanmod1_1_amd64.deb",
				Type:        "f",
				DownloadURL: "https://example.com/linux-image.deb",
				FullPath:    "releases/lts/6.18.27-xanmod1/6.18.27-x64v3-xanmod1/linux-image-6.18.27-x64v3-xanmod1_1_amd64.deb",
			},
			"linux-headers-6.18.27-x64v3-xanmod1_1_amd64.deb": {
				Name:        "linux-headers-6.18.27-x64v3-xanmod1_1_amd64.deb",
				Type:        "f",
				DownloadURL: "https://example.com/linux-headers.deb",
				FullPath:    "releases/lts/6.18.27-xanmod1/6.18.27-x64v3-xanmod1/linux-headers-6.18.27-x64v3-xanmod1_1_amd64.deb",
			},
			"linux-libc-dev_1_amd64.deb": {Name: "linux-libc-dev_1_amd64.deb", Type: "f"},
			"build.log":                  {Name: "build.log", Type: "f"},
		},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		entries, ok := pages[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		body, err := json.Marshal(entries)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_, _ = fmt.Fprintf(w, "<html><script>net.sf.files = %s;net.sf.staging_days=3;</script></html>", string(body))
	}))
	defer srv.Close()

	xanmodSourceForgeBaseURL = srv.URL

	svc := &KernelManagerService{}
	versions, err := svc.GetVersions("xanmod", "lts")
	if err != nil {
		t.Fatalf("GetVersions failed: %v", err)
	}
	if len(versions.Versions) != 4 {
		t.Fatalf("expected 4 versions, got %d", len(versions.Versions))
	}
	expectedOrder := []string{
		"6.18.27-xanmod1",
		"6.18.25-xanmod1",
		"6.18.4-rt-xanmod1",
		"6.6.63-rt46-xanmod1",
	}
	for index, expected := range expectedOrder {
		if versions.Versions[index].Name != expected {
			t.Fatalf("unexpected version order at index %d, got %s want %s", index, versions.Versions[index].Name, expected)
		}
	}

	arches, err := svc.GetArches("xanmod", "lts", "6.18.27-xanmod1")
	if err != nil {
		t.Fatalf("GetArches failed: %v", err)
	}
	if len(arches.Arches) != 3 {
		t.Fatalf("expected 3 arches, got %d", len(arches.Arches))
	}

	pkgs, err := svc.GetPackages("xanmod", "lts", "6.18.27-xanmod1", "x64v3")
	if err != nil {
		t.Fatalf("GetPackages failed: %v", err)
	}
	if len(pkgs.Packages) != 2 {
		t.Fatalf("expected 2 filtered packages, got %d", len(pkgs.Packages))
	}
	if pkgs.Packages[0].Type != "headers" || pkgs.Packages[1].Type != "image" {
		t.Fatalf("unexpected package order/types: %#v", pkgs.Packages)
	}

	bbrVersions, err := svc.GetVersions("bbrplus", "")
	if err != nil {
		t.Fatalf("GetVersions bbrplus failed: %v", err)
	}
	if len(bbrVersions.Versions) != len(bbrplusReleaseCatalog) {
		t.Fatalf("expected %d bbrplus versions, got %d", len(bbrplusReleaseCatalog), len(bbrVersions.Versions))
	}
	expectedBBROrder := []string{
		"6.1.81-bbrplus",
		"6.7.9-bbrplus",
		"6.6.21-bbrplus",
		"6.5.13-bbrplus",
		"6.4.16-bbrplus",
		"6.3.13-bbrplus",
		"6.2.16-bbrplus",
		"6.0.19-bbrplus",
		"5.15.151-bbrplus",
		"5.10.212-bbrplus",
	}
	for index, expected := range expectedBBROrder {
		if bbrVersions.Versions[index].Name != expected {
			t.Fatalf("unexpected bbrplus version order at index %d, got %s want %s", index, bbrVersions.Versions[index].Name, expected)
		}
	}

	bbrPkgs, err := svc.GetPackages("bbrplus", "", "6.7.9-bbrplus", "")
	if err != nil {
		t.Fatalf("GetPackages bbrplus failed: %v", err)
	}
	if runtime.GOARCH == "amd64" || runtime.GOARCH == "arm64" {
		if len(bbrPkgs.Packages) != 2 {
			t.Fatalf("expected 2 bbrplus packages, got %d", len(bbrPkgs.Packages))
		}
		if bbrPkgs.Packages[0].Type != "headers" || bbrPkgs.Packages[1].Type != "image" {
			t.Fatalf("unexpected bbrplus package order/types: %#v", bbrPkgs.Packages)
		}
	}
}

func TestBuildKernelInstallCommand(t *testing.T) {
	command := buildKernelInstallCommand("/usr/bin/dpkg", "/usr/bin/sudo", 1000, "/tmp/i.deb", "/tmp/h.deb")
	if len(command) < 6 {
		t.Fatalf("unexpected command: %#v", command)
	}
	if command[0] != "/usr/bin/sudo" || command[1] != "-n" {
		t.Fatalf("expected sudo command, got %#v", command)
	}

	command = buildKernelInstallCommand("/usr/bin/dpkg", "", 1000, "/tmp/i.deb", "/tmp/h.deb")
	if len(command) != 4 || command[0] != "/usr/bin/dpkg" || command[1] != "-i" {
		t.Fatalf("expected direct dpkg command, got %#v", command)
	}
}

func TestFindKernelInstallDebPair(t *testing.T) {
	dir := t.TempDir()
	image := filepath.Join(dir, "linux-image-6.18.27-x64v3-xanmod1_1_amd64.deb")
	headers := filepath.Join(dir, "linux-headers-6.18.27-x64v3-xanmod1_1_amd64.deb")
	other := filepath.Join(dir, "linux-libc-dev_1_amd64.deb")

	if err := os.WriteFile(image, []byte("image"), 0o644); err != nil {
		t.Fatalf("write image failed: %v", err)
	}
	if err := os.WriteFile(headers, []byte("headers"), 0o644); err != nil {
		t.Fatalf("write headers failed: %v", err)
	}
	if err := os.WriteFile(other, []byte("other"), 0o644); err != nil {
		t.Fatalf("write other failed: %v", err)
	}

	gotImage, gotHeaders, err := findKernelInstallDebPair(dir)
	if err != nil {
		t.Fatalf("findKernelInstallDebPair failed: %v", err)
	}
	if filepath.Base(gotImage) != filepath.Base(image) {
		t.Fatalf("unexpected image package: %s", gotImage)
	}
	if filepath.Base(gotHeaders) != filepath.Base(headers) {
		t.Fatalf("unexpected headers package: %s", gotHeaders)
	}
}

func TestFindKernelInstallDebPairBBRPlusNames(t *testing.T) {
	dir := t.TempDir()
	image := filepath.Join(dir, "Debian-Ubuntu_Required_linux-image-6.7.9-bbrplus_6.7.9-bbrplus-1_amd64.deb")
	headers := filepath.Join(dir, "Debian-Ubuntu_Optional_linux-headers-6.7.9-bbrplus_6.7.9-bbrplus-1_amd64.deb")

	if err := os.WriteFile(image, []byte("image"), 0o644); err != nil {
		t.Fatalf("write image failed: %v", err)
	}
	if err := os.WriteFile(headers, []byte("headers"), 0o644); err != nil {
		t.Fatalf("write headers failed: %v", err)
	}

	gotImage, gotHeaders, err := findKernelInstallDebPair(dir)
	if err != nil {
		t.Fatalf("findKernelInstallDebPair failed: %v", err)
	}
	if filepath.Base(gotImage) != filepath.Base(image) {
		t.Fatalf("unexpected image package: %s", gotImage)
	}
	if filepath.Base(gotHeaders) != filepath.Base(headers) {
		t.Fatalf("unexpected headers package: %s", gotHeaders)
	}
	if got := kernelDebPackageName(gotImage); got != "linux-image-6.7.9-bbrplus" {
		t.Fatalf("unexpected bbrplus image package name: %q", got)
	}
	if got := kernelDebPackageName(gotHeaders); got != "linux-headers-6.7.9-bbrplus" {
		t.Fatalf("unexpected bbrplus headers package name: %q", got)
	}
}

func TestFindKernelInstallDebPairRejectsMixedKernelIDs(t *testing.T) {
	dir := t.TempDir()
	image := filepath.Join(dir, "linux-image-6.18.27-x64v3-xanmod1_1_amd64.deb")
	headers := filepath.Join(dir, "linux-headers-6.18.25-x64v3-xanmod1_1_amd64.deb")

	if err := os.WriteFile(image, []byte("image"), 0o644); err != nil {
		t.Fatalf("write image failed: %v", err)
	}
	if err := os.WriteFile(headers, []byte("headers"), 0o644); err != nil {
		t.Fatalf("write headers failed: %v", err)
	}

	if _, _, err := findKernelInstallDebPair(dir); err == nil {
		t.Fatal("expected mixed kernel ids to be rejected")
	}
}

func TestDetectKernelLinuxSystemFamily(t *testing.T) {
	if got := detectKernelLinuxSystemFamily(map[string]string{"ID": "ubuntu"}); got != "debian" {
		t.Fatalf("ubuntu should map to debian family, got %q", got)
	}
	if got := detectKernelLinuxSystemFamily(map[string]string{"ID": "debian"}); got != "debian" {
		t.Fatalf("debian should map to debian family, got %q", got)
	}
	if got := detectKernelLinuxSystemFamily(map[string]string{"ID": "fedora"}); got != "rhel" {
		t.Fatalf("fedora should map to rhel family, got %q", got)
	}
}

func TestParseKernelSelectionsAndCleanupItems(t *testing.T) {
	raw := `
console-setup-linux                             install
linux-headers-6.18.25-rt-x64v3-xanmod1         install
linux-image-6.18.27-x64v3-xanmod1              install
linux-image-6.1.0-39-amd64                      deinstall
util-linux                                      install
`
	entries := parseKernelSelections(raw)
	if len(entries) != 5 {
		t.Fatalf("expected 5 entries, got %d", len(entries))
	}

	items := buildKernelCleanupPackageItems(entries, "6.18.27-x64v3-xanmod1", "6.18.25-rt-x64v3-xanmod1")
	if len(items) != 5 {
		t.Fatalf("expected 5 items, got %d", len(items))
	}

	if !items[1].IsHeaders || !items[1].IsPinnedKernel {
		t.Fatalf("expected headers entry to be pinned: %#v", items[1])
	}
	if !items[2].IsImage || !items[2].IsCurrentKernel {
		t.Fatalf("expected image entry to match current kernel: %#v", items[2])
	}
	if items[4].Risk != "high" {
		t.Fatalf("expected util-linux risk high, got %q", items[4].Risk)
	}
}

func TestBuildAutoCleanupPurgeTargetsSkipsOnlyCurrentKernel(t *testing.T) {
	items := []KernelCleanupPackageItem{
		{Name: "linux-image-6.18.25-rt-x64v3-xanmod1", IsImage: true},
		{Name: "linux-headers-6.18.25-rt-x64v3-xanmod1", IsHeaders: true},
		{Name: "linux-image-6.18.27-x64v3-xanmod1", IsImage: true},
		{Name: "linux-headers-6.18.27-x64v3-xanmod1", IsHeaders: true},
		{Name: "linux-image-6.18.29-x64v3-xanmod1", IsImage: true},
		{Name: "linux-headers-6.18.29-x64v3-xanmod1", IsHeaders: true},
		{Name: "util-linux", IsImage: false, IsHeaders: false},
	}
	targets := buildAutoCleanupPurgeTargets(items, "6.18.29-x64v3-xanmod1")
	if len(targets) != 4 {
		t.Fatalf("expected 4 targets, got %d: %#v", len(targets), targets)
	}
	expected := []string{
		"linux-image-6.18.25-rt-x64v3-xanmod1",
		"linux-headers-6.18.25-rt-x64v3-xanmod1",
		"linux-image-6.18.27-x64v3-xanmod1",
		"linux-headers-6.18.27-x64v3-xanmod1",
	}
	for i, want := range expected {
		if targets[i] != want {
			t.Fatalf("unexpected target at %d: got %q want %q; all targets: %#v", i, targets[i], want, targets)
		}
	}
}

func TestBuildAutoCleanupPurgeTargetsSkipsCurrentKernel(t *testing.T) {
	items := []KernelCleanupPackageItem{
		{Name: "linux-image-6.18.25-rt-x64v3-xanmod1", IsImage: true},
		{Name: "linux-headers-6.18.25-rt-x64v3-xanmod1", IsHeaders: true},
		{Name: "linux-image-6.18.27-x64v3-xanmod1", IsImage: true},
		{Name: "linux-headers-6.18.27-x64v3-xanmod1", IsHeaders: true},
	}
	targets := buildAutoCleanupPurgeTargets(items, "6.18.25-rt-x64v3-xanmod1")
	if len(targets) != 2 {
		t.Fatalf("expected 2 targets, got %d: %#v", len(targets), targets)
	}
	if targets[0] != "linux-image-6.18.27-x64v3-xanmod1" || targets[1] != "linux-headers-6.18.27-x64v3-xanmod1" {
		t.Fatalf("unexpected targets: %#v", targets)
	}
}

func TestNormalizeKernelPurgeTargetsAndCommand(t *testing.T) {
	targets, err := normalizeKernelPurgeTargets([]string{
		" linux-image-6.18.27-x64v3-xanmod1 ",
		"linux-headers-6.18.27-x64v3-xanmod1",
		"linux-image-6.18.27-x64v3-xanmod1",
	})
	if err != nil {
		t.Fatalf("normalizeKernelPurgeTargets failed: %v", err)
	}
	if len(targets) != 2 {
		t.Fatalf("expected deduplicated 2 targets, got %d", len(targets))
	}

	cmd := buildKernelPurgeCommand("/usr/bin/apt-get", targets)
	if len(cmd) != 5 {
		t.Fatalf("unexpected purge command length: %#v", cmd)
	}
	if cmd[0] != "/usr/bin/apt-get" || cmd[1] != "purge" || cmd[2] != "-y" {
		t.Fatalf("unexpected purge command: %#v", cmd)
	}
}

func TestExtractKernelIDFromPackage(t *testing.T) {
	if got := extractKernelIDFromPackage("linux-image-6.18.27-x64v3-xanmod1"); got != "6.18.27-x64v3-xanmod1" {
		t.Fatalf("unexpected image kernel id: %q", got)
	}
	if got := extractKernelIDFromPackage("linux-headers-6.18.27-x64v3-xanmod1"); got != "6.18.27-x64v3-xanmod1" {
		t.Fatalf("unexpected headers kernel id: %q", got)
	}
	if got := extractKernelIDFromPackage("util-linux"); got != "" {
		t.Fatalf("expected empty kernel id, got %q", got)
	}
}

func TestRunKernelStrictSystemCleanupStepOrder(t *testing.T) {
	oldResolveApt := kernelResolveAptCommand
	oldRunPrivileged := kernelRunPrivilegedCommand
	oldLookPath := kernelLookPath
	defer func() {
		kernelResolveAptCommand = oldResolveApt
		kernelRunPrivilegedCommand = oldRunPrivileged
		kernelLookPath = oldLookPath
	}()

	calls := make([]string, 0, 8)
	kernelResolveAptCommand = func() (string, error) {
		return "/usr/bin/apt-get", nil
	}
	kernelLookPath = func(file string) (string, error) {
		if file == "journalctl" {
			return "/usr/bin/journalctl", nil
		}
		return "", errors.New("unexpected lookup: " + file)
	}
	kernelRunPrivilegedCommand = func(timeout time.Duration, command string, args ...string) error {
		calls = append(calls, command+" "+strings.Join(args, " "))
		return nil
	}

	report := runKernelStrictSystemCleanup()
	if report == nil {
		t.Fatal("expected cleanup report, got nil")
	}
	if !report.Done {
		t.Fatalf("expected cleanup done, got warnings: %#v", report.Warnings)
	}
	if len(report.Warnings) != 0 {
		t.Fatalf("expected no warnings, got %#v", report.Warnings)
	}

	expected := []string{
		"/usr/bin/apt-get autoremove -y",
		"/usr/bin/apt-get autoclean",
		"/usr/bin/apt-get clean",
		"/usr/bin/journalctl --rotate",
		"/usr/bin/journalctl --vacuum-time=1s",
		"sh -lc find /var/log -xdev -type f -exec truncate -s 0 -- {} +",
		"sh -lc find /tmp /var/tmp -mindepth 1 -xdev -exec rm -rf -- {} +",
	}
	if len(calls) != len(expected) {
		t.Fatalf("expected %d cleanup steps, got %d: %#v", len(expected), len(calls), calls)
	}
	for i := range expected {
		if calls[i] != expected[i] {
			t.Fatalf("unexpected step %d: got %q want %q", i, calls[i], expected[i])
		}
	}
}

func TestRunKernelStrictSystemCleanupContinuesOnFailure(t *testing.T) {
	oldResolveApt := kernelResolveAptCommand
	oldRunPrivileged := kernelRunPrivilegedCommand
	oldLookPath := kernelLookPath
	defer func() {
		kernelResolveAptCommand = oldResolveApt
		kernelRunPrivilegedCommand = oldRunPrivileged
		kernelLookPath = oldLookPath
	}()

	calls := make([]string, 0, 8)
	kernelResolveAptCommand = func() (string, error) {
		return "/usr/bin/apt-get", nil
	}
	kernelLookPath = func(file string) (string, error) {
		if file == "journalctl" {
			return "/usr/bin/journalctl", nil
		}
		return "", errors.New("unexpected lookup: " + file)
	}
	kernelRunPrivilegedCommand = func(timeout time.Duration, command string, args ...string) error {
		invoke := command + " " + strings.Join(args, " ")
		calls = append(calls, invoke)
		if strings.Contains(invoke, "autoclean") || strings.Contains(invoke, "--rotate") {
			return errors.New("forced fail")
		}
		return nil
	}

	report := runKernelStrictSystemCleanup()
	if report == nil {
		t.Fatal("expected cleanup report, got nil")
	}
	if report.Done {
		t.Fatalf("expected cleanup warnings, got done report: %#v", report)
	}
	if len(calls) != 7 {
		t.Fatalf("expected all 7 steps to run, got %d steps: %#v", len(calls), calls)
	}
	if len(report.Warnings) != 2 {
		t.Fatalf("expected 2 warnings, got %d: %#v", len(report.Warnings), report.Warnings)
	}
	if !strings.Contains(report.Summary, "apt autoclean") || !strings.Contains(report.Summary, "journalctl rotate") {
		t.Fatalf("expected summary to include failed steps, got %q", report.Summary)
	}
}

func TestAutoCleanupPackagesNoTargetsStillRunsSystemCleanup(t *testing.T) {
	oldEnsure := kernelEnsureRuntimeSupported
	oldRunCleanup := kernelRunSystemCleanup
	defer func() {
		kernelEnsureRuntimeSupported = oldEnsure
		kernelRunSystemCleanup = oldRunCleanup
	}()

	kernelEnsureRuntimeSupported = func() error { return nil }
	kernelRunSystemCleanup = func() *kernelSystemCleanupReport {
		return &kernelSystemCleanupReport{
			Done:    false,
			Summary: "system cleanup completed with warnings: mock warning",
			Warnings: []string{
				"mock warning",
			},
		}
	}

	svc := &KernelManagerService{}
	result, err := svc.autoCleanupPackagesFromScanResult(&KernelCleanupScanResponse{
		CurrentKernel: "6.0.0-test",
		PinnedKernel:  "6.0.0-test",
		Packages:      []KernelCleanupPackageItem{},
	})
	if err != nil {
		t.Fatalf("autoCleanupPackagesFromScanResult returned error: %v", err)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}
	if result.Message != "no packages matched auto cleanup policy" {
		t.Fatalf("unexpected message: %q", result.Message)
	}
	if result.SystemCleanupDone {
		t.Fatalf("expected system cleanup done=false, got %#v", result)
	}
	if result.SystemCleanupSummary == "" || !strings.Contains(result.SystemCleanupSummary, "mock warning") {
		t.Fatalf("expected cleanup summary warning, got %#v", result)
	}
}

func TestPurgePackagesFailureStillRunsSystemCleanup(t *testing.T) {
	oldEnsure := kernelEnsureRuntimeSupported
	oldResolveApt := kernelResolveAptCommand
	oldRunPrivileged := kernelRunPrivilegedCommand
	oldRunCleanup := kernelRunSystemCleanup
	defer func() {
		kernelEnsureRuntimeSupported = oldEnsure
		kernelResolveAptCommand = oldResolveApt
		kernelRunPrivilegedCommand = oldRunPrivileged
		kernelRunSystemCleanup = oldRunCleanup
	}()

	kernelEnsureRuntimeSupported = func() error { return nil }
	kernelResolveAptCommand = func() (string, error) {
		return "/usr/bin/apt-get", nil
	}
	kernelRunPrivilegedCommand = func(timeout time.Duration, command string, args ...string) error {
		if command == "/usr/bin/apt-get" && len(args) > 0 && args[0] == "purge" {
			return errors.New("forced purge failure")
		}
		return nil
	}
	kernelRunSystemCleanup = func() *kernelSystemCleanupReport {
		return &kernelSystemCleanupReport{
			Done:    true,
			Summary: "system cleanup completed",
		}
	}

	svc := &KernelManagerService{}
	result, err := svc.PurgePackages([]string{"linux-image-6.18.27-x64v3-xanmod1"})
	if err == nil {
		t.Fatal("expected purge error, got nil")
	}
	if result == nil {
		t.Fatal("expected result object even on failure")
	}
	if !strings.Contains(err.Error(), "kernel purge command failed") {
		t.Fatalf("expected purge error text, got %q", err.Error())
	}
	if !strings.Contains(err.Error(), "system cleanup completed") {
		t.Fatalf("expected cleanup summary in error, got %q", err.Error())
	}
	if len(result.Requested) != 1 || result.Requested[0] != "linux-image-6.18.27-x64v3-xanmod1" {
		t.Fatalf("unexpected requested packages: %#v", result.Requested)
	}
	if result.SystemCleanupSummary != "system cleanup completed" {
		t.Fatalf("unexpected cleanup summary: %q", result.SystemCleanupSummary)
	}
}

func TestKernelDownloadFailureSchedulesCleanup(t *testing.T) {
	oldEnsure := kernelEnsureRuntimeSupported
	oldBaseURL := xanmodSourceForgeBaseURL
	oldDelay := kernelFailedDownloadCleanupDelay
	defer func() {
		kernelEnsureRuntimeSupported = oldEnsure
		xanmodSourceForgeBaseURL = oldBaseURL
		kernelFailedDownloadCleanupDelay = oldDelay
	}()

	kernelEnsureRuntimeSupported = func() error { return nil }
	kernelFailedDownloadCleanupDelay = 20 * time.Millisecond
	kernelDownloadCleanupStore = newKernelDownloadCleanupScheduler()
	kernelDownloadProgressStore = newKernelDownloadProgressStore()

	binDir := config.GetBinDir()
	dataRoot := filepath.Join(binDir, "Promanager_data")
	_ = os.RemoveAll(filepath.Join(dataRoot, "kernel"))
	t.Cleanup(func() {
		_ = os.RemoveAll(filepath.Join(dataRoot, "kernel"))
	})

	const (
		line    = "lts"
		version = "6.18.27-xanmod1"
		arch    = "x64v3"
	)

	headersName := "linux-headers-6.18.27-x64v3-xanmod1_1_amd64.deb"
	imageName := "linux-image-6.18.27-x64v3-xanmod1_1_amd64.deb"
	headersBody := []byte("headers-ok")

	pages := map[string]map[string]sourceForgeFileEntry{
		"/releases/lts/": {
			version: {Name: version, Type: "d"},
		},
		"/releases/lts/6.18.27-xanmod1/": {
			"6.18.27-x64v3-xanmod1": {Name: "6.18.27-x64v3-xanmod1", Type: "d"},
		},
		"/releases/lts/6.18.27-xanmod1/6.18.27-x64v3-xanmod1/": {
			headersName: {
				Name:        headersName,
				Type:        "f",
				DownloadURL: "http://placeholder/headers.deb",
				FullPath:    "releases/lts/6.18.27-xanmod1/6.18.27-x64v3-xanmod1/" + headersName,
			},
			imageName: {
				Name:        imageName,
				Type:        "f",
				DownloadURL: "http://placeholder/image.deb",
				FullPath:    "releases/lts/6.18.27-xanmod1/6.18.27-x64v3-xanmod1/" + imageName,
			},
		},
	}

	var serverURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/headers.deb":
			if r.Method == http.MethodHead {
				w.Header().Set("Content-Length", fmt.Sprintf("%d", len(headersBody)))
				w.WriteHeader(http.StatusOK)
				return
			}
			_, _ = w.Write(headersBody)
			return
		case "/image.deb":
			http.Error(w, "forced failure", http.StatusInternalServerError)
			return
		}
		entries, ok := pages[r.URL.Path]
		if !ok {
			http.NotFound(w, r)
			return
		}
		for key, entry := range entries {
			if strings.Contains(key, ".deb") {
				entry.DownloadURL = serverURL + "/" + path.Base(key)
				entries[key] = entry
			}
		}
		body, err := json.Marshal(entries)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		_, _ = fmt.Fprintf(w, "<html><script>net.sf.files = %s;net.sf.staging_days=3;</script></html>", string(body))
	}))
	defer srv.Close()
	serverURL = srv.URL
	xanmodSourceForgeBaseURL = srv.URL

	svc := &KernelManagerService{}
	result, err := svc.DownloadPackages("xanmod", line, version, arch, "kernel-download-failure-cleanup")
	if err == nil {
		t.Fatal("expected download failure, got nil")
	}
	if result != nil {
		t.Fatalf("expected nil result on failure, got %#v", result)
	}

	downloadDir := filepath.Join(config.GetDataDir(), "kernel", line, version, arch)
	tmpPath := filepath.Join(downloadDir, imageName+".tmp")
	headersPath := filepath.Join(downloadDir, headersName)
	if _, statErr := os.Stat(headersPath); statErr != nil {
		if _, tmpErr := os.Stat(tmpPath); tmpErr != nil && !os.IsNotExist(tmpErr) {
			t.Fatalf("unexpected tmp stat error: %v", tmpErr)
		}
	}

	cleanupRoot := filepath.Join(config.GetDataDir(), "kernel", kernelFailedCleanupDirName)
	entriesBefore, readErr := os.ReadDir(cleanupRoot)
	if readErr != nil && !os.IsNotExist(readErr) {
		t.Fatalf("read cleanup root failed: %v", readErr)
	}
	if len(entriesBefore) == 0 {
		t.Fatalf("expected failed download artifacts moved into cleanup root %q", cleanupRoot)
	}

	time.Sleep(120 * time.Millisecond)
	if _, statErr := os.Stat(downloadDir); !os.IsNotExist(statErr) {
		t.Fatalf("expected failed download directory cleaned, got err=%v", statErr)
	}
	entriesAfter, readErr := os.ReadDir(cleanupRoot)
	if readErr != nil && !os.IsNotExist(readErr) {
		t.Fatalf("read cleanup root after cleanup failed: %v", readErr)
	}
	if len(entriesAfter) != 0 {
		t.Fatalf("expected cleanup root emptied after delayed cleanup, still has %d entries", len(entriesAfter))
	}
}

func TestClearDownloadedKernelRemovesMarkerAndLegacyArtifacts(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "kernel-clear.db")
	if err := database.InitDB(dbPath); err != nil {
		t.Fatalf("init db failed: %v", err)
	}
	if sqlDB, err := database.GetDB().DB(); err == nil && sqlDB != nil {
		t.Cleanup(func() {
			_ = sqlDB.Close()
		})
	}

	binDir := config.GetBinDir()
	dataRoot := filepath.Join(binDir, "Promanager_data")
	kernelRoot := filepath.Join(dataRoot, "kernel")
	_ = os.RemoveAll(kernelRoot)
	t.Cleanup(func() {
		_ = os.RemoveAll(kernelRoot)
	})

	downloadDir := filepath.Join(kernelRoot, "lts", "6.18.36-xanmod1", "x64v3")
	if err := os.MkdirAll(downloadDir, 0o755); err != nil {
		t.Fatalf("mkdir download dir failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(downloadDir, "linux-headers-6.18.36-x64v3-xanmod1_1_amd64.deb"), []byte("h"), 0o644); err != nil {
		t.Fatalf("write headers failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(downloadDir, "linux-image-6.18.36-x64v3-xanmod1_1_amd64.deb"), []byte("i"), 0o644); err != nil {
		t.Fatalf("write image failed: %v", err)
	}
	legacyDir := filepath.Join(kernelRoot, "main", "legacy-broken", "x64v3")
	if err := os.MkdirAll(legacyDir, 0o755); err != nil {
		t.Fatalf("mkdir legacy dir failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacyDir, "linux-image-legacy_1_amd64.deb.tmp"), []byte("tmp"), 0o644); err != nil {
		t.Fatalf("write legacy tmp failed: %v", err)
	}

	settingSvc := &SettingService{}
	marker := kernelDownloadedMarker{
		Provider:   "xanmod",
		Line:       "lts",
		Version:    "6.18.36-xanmod1",
		Arch:       "x64v3",
		Directory:  downloadDir,
		Downloaded: time.Now().Unix(),
	}
	raw, err := json.Marshal(marker)
	if err != nil {
		t.Fatalf("marshal marker failed: %v", err)
	}
	if err := settingSvc.SaveSetting(kernelDownloadedMarkerSettingKey, string(raw)); err != nil {
		t.Fatalf("save marker failed: %v", err)
	}

	svc := &KernelManagerService{}
	result, err := svc.ClearDownloadedKernel()
	if err != nil {
		t.Fatalf("ClearDownloadedKernel failed: %v", err)
	}
	if !result.Cleared {
		t.Fatalf("expected cleared result, got %#v", result)
	}
	if _, statErr := os.Stat(kernelRoot); !os.IsNotExist(statErr) {
		t.Fatalf("expected kernel root removed, got err=%v", statErr)
	}
	stored, err := settingSvc.getString(kernelDownloadedMarkerSettingKey)
	if err != nil {
		t.Fatalf("load marker failed: %v", err)
	}
	if strings.TrimSpace(stored) != "" {
		t.Fatalf("expected marker cleared, got %q", stored)
	}
}

func TestGetDownloadedKernelStatusRepairsLegacyCompletePairAndCleansBrokenDir(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "kernel-legacy-status.db")
	if err := database.InitDB(dbPath); err != nil {
		t.Fatalf("init db failed: %v", err)
	}
	if sqlDB, err := database.GetDB().DB(); err == nil && sqlDB != nil {
		t.Cleanup(func() {
			_ = sqlDB.Close()
		})
	}

	binDir := config.GetBinDir()
	dataRoot := filepath.Join(binDir, "Promanager_data")
	kernelRoot := filepath.Join(dataRoot, "kernel")
	_ = os.RemoveAll(kernelRoot)
	t.Cleanup(func() {
		_ = os.RemoveAll(kernelRoot)
	})

	validDir := filepath.Join(kernelRoot, "lts", "6.18.36-xanmod1", "x64v3")
	if err := os.MkdirAll(validDir, 0o755); err != nil {
		t.Fatalf("mkdir valid dir failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(validDir, "linux-headers-6.18.36-x64v3-xanmod1_1_amd64.deb"), []byte("h"), 0o644); err != nil {
		t.Fatalf("write headers failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(validDir, "linux-image-6.18.36-x64v3-xanmod1_1_amd64.deb"), []byte("i"), 0o644); err != nil {
		t.Fatalf("write image failed: %v", err)
	}
	brokenDir := filepath.Join(kernelRoot, "lts", "broken-version", "x64v3")
	if err := os.MkdirAll(brokenDir, 0o755); err != nil {
		t.Fatalf("mkdir broken dir failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(brokenDir, "linux-image-broken_1_amd64.deb"), []byte("i"), 0o644); err != nil {
		t.Fatalf("write broken image failed: %v", err)
	}

	svc := &KernelManagerService{}
	status, err := svc.GetDownloadedKernelStatus()
	if err != nil {
		t.Fatalf("GetDownloadedKernelStatus failed: %v", err)
	}
	if status == nil || !status.Exists {
		t.Fatalf("expected recovered legacy status, got %#v", status)
	}
	if filepath.Clean(status.Directory) != filepath.Clean(validDir) {
		t.Fatalf("unexpected recovered directory: got %q want %q", status.Directory, validDir)
	}
	if _, statErr := os.Stat(brokenDir); !os.IsNotExist(statErr) {
		t.Fatalf("expected broken legacy dir cleaned, got err=%v", statErr)
	}
}
