package service

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/alireza0/s-ui/database/model"
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
	createManagedSourceFile(t, stageRoot, "usr/share/man/man1/vnstati.1")
	createManagedSourceFile(t, stageRoot, "usr/share/man/man5/vnstat.conf.5")
	createManagedSourceFile(t, stageRoot, "usr/share/man/man8/vnstatd.8")

	got := collectManagedSourceVnstatPaths(stageRoot)
	want := []string{
		"/etc/vnstat.conf",
		"/usr/bin/vnstat",
		"/usr/bin/vnstati",
		"/usr/sbin/vnstatd",
		"/usr/share/man/man1/vnstat.1",
		"/usr/share/man/man1/vnstati.1",
		"/usr/share/man/man5/vnstat.conf.5",
		"/usr/share/man/man8/vnstatd.8",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("collectManagedSourceVnstatPaths = %v, want %v", got, want)
	}
}

func TestValidateManagedSourceVnstatPathsRejectsIncompleteStage(t *testing.T) {
	if err := validateManagedSourceVnstatPaths([]string{"/usr/bin/vnstat", "/usr/sbin/vnstatd"}); err == nil {
		t.Fatal("source staging inventory without vnstat.conf must fail before real installation")
	}
	if err := validateManagedSourceVnstatPaths([]string{"/usr/bin/vnstat", "/usr/sbin/vnstatd", "/etc/vnstat.conf"}); err != nil {
		t.Fatalf("complete source staging inventory should pass: %v", err)
	}
}

func TestParseSystemdExecStartPaths(t *testing.T) {
	got := parseSystemdExecStartPaths("{ path=/usr/sbin/vnstatd ; argv[]=/usr/sbin/vnstatd --nodaemon ; }\n")
	want := []string{"/usr/sbin/vnstatd"}
	if !slices.Equal(got, want) {
		t.Fatalf("parseSystemdExecStartPaths = %v, want %v", got, want)
	}
}

func TestVnstatStatusDoesNotDiscoverExternalInstances(t *testing.T) {
	initTrafficOverviewTestDB(t)
	previousDiscover := vnstatDiscoverRunningExternalFn
	vnstatDiscoverRunningExternalFn = func(trafficOverviewVnstatManifest, bool) []vnstatExternalInstallation {
		t.Fatal("traffic status polling must not inspect external vnstat processes")
		return nil
	}
	t.Cleanup(func() {
		vnstatDiscoverRunningExternalFn = previousDiscover
	})

	_ = (&TrafficOverviewService{}).GetVnstatStatus()
}

func TestShouldPrepareVnstatPanelPathOnlyForFixedRuntimePath(t *testing.T) {
	if !shouldPrepareVnstatPanelPathForInstall(nil, vnstatInstallMethodSystemPackage, []vnstatExternalInstallation{{DaemonPath: "/usr/sbin/vnstatd"}}) {
		t.Fatal("a running daemon at the panel fixed path must release that path before install")
	}
	if shouldPrepareVnstatPanelPathForInstall(nil, vnstatInstallMethodSystemPackage, []vnstatExternalInstallation{{DaemonPath: "/opt/vnstat/sbin/vnstatd"}}) {
		t.Fatal("a running external daemon outside panel paths must not trigger file preparation")
	}
}

func TestSelectVnstatPackageManagerUsesCachedPlatformFamily(t *testing.T) {
	manager := selectVnstatPackageManagerPlan("debian", func(name string) bool {
		return name == "apt-get" || name == "dnf"
	})
	if manager == nil || manager.Name != "apt-get" {
		t.Fatalf("debian cached platform selected %#v, want apt-get", manager)
	}

	manager = selectVnstatPackageManagerPlan("rhel", func(name string) bool {
		return name == "apt-get"
	})
	if manager != nil {
		t.Fatalf("rhel must not select apt-get from PATH, got %#v", manager)
	}
}

func TestManagedVnstatInstallJobTracksProgressAndPreventsConcurrentInstalls(t *testing.T) {
	previousRunner := vnstatManagedInstallRunner
	vnstatInstallJobMu.Lock()
	previousJob := vnstatInstallJobState
	vnstatInstallJobState = nil
	vnstatInstallJobMu.Unlock()

	started := make(chan struct{})
	release := make(chan struct{})
	done := make(chan struct{})
	vnstatManagedInstallRunner = func(_ context.Context, _ *TrafficOverviewService, source string, report vnstatInstallProgressReporter) (*TrafficOverview, error) {
		defer close(done)
		report("正在下载测试 vnStat")
		close(started)
		<-release
		if source == vnstatInstallMethodGitHubRelease {
			return nil, errors.New("GitHub test install failed")
		}
		report("正在刷新测试状态")
		return &TrafficOverview{}, nil
	}
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
		select {
		case <-done:
		case <-time.After(time.Second):
		}
		vnstatManagedInstallRunner = previousRunner
		vnstatInstallJobMu.Lock()
		vnstatInstallJobState = previousJob
		vnstatInstallJobMu.Unlock()
	})

	svc := &TrafficOverviewService{}
	job, err := svc.StartManagedVnstatInstall(vnstatInstallMethodSystemPackage)
	if err != nil {
		t.Fatalf("start managed vnStat install failed: %v", err)
	}
	if job.State != managedDownloadTaskQueued || job.ID == "" {
		t.Fatalf("unexpected newly started job: %#v", job)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("install runner did not start")
	}

	progress := svc.GetManagedVnstatInstallJob(job.ID)
	if progress.State != "running" || progress.Phase != "正在下载测试 vnStat" {
		t.Fatalf("unexpected running job progress: %#v", progress)
	}
	repeated, err := svc.StartManagedVnstatInstall(vnstatInstallMethodSystemPackage)
	if err != nil || repeated.ID != job.ID {
		t.Fatalf("same-source retry must return the active job, got job=%#v err=%v", repeated, err)
	}
	if _, err := svc.StartManagedVnstatInstall(vnstatInstallMethodGitHubRelease); err == nil || !strings.Contains(err.Error(), "already running") {
		t.Fatalf("different-source concurrent install must be rejected, got %v", err)
	}

	close(release)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("install runner did not finish")
	}
	completed := waitForManagedVnstatInstallJobState(t, svc, job.ID, managedDownloadTaskSuccess)
	if completed.FinishedAt == 0 {
		t.Fatalf("expected successful job to have a completion time, got %#v", completed)
	}

	// A terminal task permits a later install, while surfacing its actual error
	// through the status endpoint instead of an HTTP timeout.
	started = make(chan struct{})
	release = make(chan struct{})
	done = make(chan struct{})
	failedJob, err := svc.StartManagedVnstatInstall(vnstatInstallMethodGitHubRelease)
	if err != nil {
		t.Fatalf("start follow-up install failed: %v", err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("follow-up install runner did not start")
	}
	close(release)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("follow-up install runner did not finish")
	}
	failed := waitForManagedVnstatInstallJobState(t, svc, failedJob.ID, managedDownloadTaskError)
	if !strings.Contains(failed.Error, "GitHub test install failed") {
		t.Fatalf("expected failed job status, got %#v", failed)
	}
}

func TestManagedVnstatInstallStopsBeforeApplying(t *testing.T) {
	previousRunner := vnstatManagedInstallRunner
	vnstatInstallJobMu.Lock()
	previousJob := vnstatInstallJobState
	vnstatInstallJobState = nil
	vnstatInstallJobMu.Unlock()

	started := make(chan struct{})
	done := make(chan struct{})
	vnstatManagedInstallRunner = func(ctx context.Context, _ *TrafficOverviewService, _ string, report vnstatInstallProgressReporter) (*TrafficOverview, error) {
		defer close(done)
		report("正在下载测试 vnStat")
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	}
	t.Cleanup(func() {
		vnstatManagedInstallRunner = previousRunner
		select {
		case <-done:
		case <-time.After(time.Second):
		}
		vnstatInstallJobMu.Lock()
		vnstatInstallJobState = previousJob
		vnstatInstallJobMu.Unlock()
	})

	svc := &TrafficOverviewService{}
	job, err := svc.StartManagedVnstatInstall(vnstatInstallMethodSystemPackage)
	if err != nil {
		t.Fatalf("start managed vnStat install: %v", err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("install runner did not start")
	}

	stopping, err := svc.StopManagedVnstatInstall(job.ID)
	if err != nil {
		t.Fatalf("stop managed vnStat install: %v", err)
	}
	if stopping.State != managedDownloadTaskStopping || !stopping.StopRequested {
		t.Fatalf("unexpected stop status: %#v", stopping)
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("cancelled vnStat runner did not finish")
	}

	_ = waitForManagedVnstatInstallJobState(t, svc, job.ID, managedDownloadTaskCancelled)
}

func TestManagedVnstatInstallStopWinsBeforeIrreversiblePhase(t *testing.T) {
	previousRunner := vnstatManagedInstallRunner
	vnstatInstallJobMu.Lock()
	previousJob := vnstatInstallJobState
	vnstatInstallJobState = nil
	vnstatInstallJobMu.Unlock()

	ready := make(chan struct{})
	allowIrreversiblePhase := make(chan struct{})
	irreversibleWorkStarted := make(chan struct{})
	done := make(chan struct{})
	vnstatManagedInstallRunner = func(ctx context.Context, _ *TrafficOverviewService, _ string, report vnstatInstallProgressReporter) (*TrafficOverview, error) {
		defer close(done)
		if !report("正在下载测试 vnStat") {
			return nil, ctx.Err()
		}
		close(ready)
		<-allowIrreversiblePhase
		if !report("正在通过系统软件源安装 vnStat") {
			return nil, ctx.Err()
		}
		close(irreversibleWorkStarted)
		return &TrafficOverview{}, nil
	}
	t.Cleanup(func() {
		select {
		case <-allowIrreversiblePhase:
		default:
			close(allowIrreversiblePhase)
		}
		select {
		case <-done:
		case <-time.After(time.Second):
		}
		vnstatManagedInstallRunner = previousRunner
		vnstatInstallJobMu.Lock()
		vnstatInstallJobState = previousJob
		vnstatInstallJobMu.Unlock()
	})

	svc := &TrafficOverviewService{}
	job, err := svc.StartManagedVnstatInstall(vnstatInstallMethodSystemPackage)
	if err != nil {
		t.Fatalf("start managed vnStat install: %v", err)
	}
	select {
	case <-ready:
	case <-time.After(time.Second):
		t.Fatal("install runner did not reach the reversible phase")
	}

	if _, err := svc.StopManagedVnstatInstall(job.ID); err != nil {
		t.Fatalf("stop managed vnStat install: %v", err)
	}
	close(allowIrreversiblePhase)
	select {
	case <-irreversibleWorkStarted:
		t.Fatal("stop request must prevent irreversible vnStat work from starting")
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("cancelled vnStat runner did not finish")
	}

	_ = waitForManagedVnstatInstallJobState(t, svc, job.ID, managedDownloadTaskCancelled)
}

func TestManagedVnstatInstallPanicBecomesTaskError(t *testing.T) {
	previousRunner := vnstatManagedInstallRunner
	vnstatInstallJobMu.Lock()
	previousJob := vnstatInstallJobState
	vnstatInstallJobState = nil
	vnstatInstallJobMu.Unlock()
	vnstatManagedInstallRunner = func(context.Context, *TrafficOverviewService, string, vnstatInstallProgressReporter) (*TrafficOverview, error) {
		panic("test vnStat worker panic")
	}
	t.Cleanup(func() {
		vnstatManagedInstallRunner = previousRunner
		vnstatInstallJobMu.Lock()
		vnstatInstallJobState = previousJob
		vnstatInstallJobMu.Unlock()
	})

	svc := &TrafficOverviewService{}
	job, err := svc.StartManagedVnstatInstall(vnstatInstallMethodSystemPackage)
	if err != nil {
		t.Fatalf("start managed vnStat install: %v", err)
	}
	finished := waitForManagedVnstatInstallJobState(t, svc, job.ID, managedDownloadTaskError)
	if !strings.Contains(finished.Error, "test vnStat worker panic") || finished.Phase != "vnStat 安装失败" {
		t.Fatalf("panic should be exposed as a managed task error, got %#v", finished)
	}
}

func waitForManagedVnstatInstallJobState(t *testing.T, svc *TrafficOverviewService, jobID string, expectedState string) VnstatInstallJobStatus {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	var latest VnstatInstallJobStatus
	for time.Now().Before(deadline) {
		latest = svc.GetManagedVnstatInstallJob(jobID)
		if latest.State == expectedState {
			return latest
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("expected vnStat task %q state %q, got %#v", jobID, expectedState, latest)
	return VnstatInstallJobStatus{}
}

func TestRemoveManagedVnstatRejectsWhileInstallTaskRuns(t *testing.T) {
	vnstatInstallJobMu.Lock()
	previousJob := vnstatInstallJobState
	vnstatInstallJobState = &vnstatInstallJob{status: VnstatInstallJobStatus{State: "running"}}
	vnstatInstallJobMu.Unlock()
	t.Cleanup(func() {
		vnstatInstallJobMu.Lock()
		vnstatInstallJobState = previousJob
		vnstatInstallJobMu.Unlock()
	})

	if _, err := (&TrafficOverviewService{}).RemoveManagedVnstat(); err == nil || !strings.Contains(err.Error(), "安装任务正在运行") {
		t.Fatalf("delete must be blocked while an install task runs, got %v", err)
	}
}

func TestVnstatManifestOwnershipAndPathSafety(t *testing.T) {
	if got := normalizeVnstatOwnership("", true); got != "" {
		t.Fatalf("blank legacy ownership = %q, want empty", got)
	}
	if got := normalizeVnstatOwnership("adopted-system-package", true); got != "" {
		t.Fatalf("adopted ownership = %q, want empty", got)
	}
	if got := normalizeVnstatOwnership(vnstatOwnershipPanelInstalled, true); got != vnstatOwnershipPanelInstalled {
		t.Fatalf("panel ownership = %q", got)
	}

	trusted := trafficOverviewVnstatManifest{
		Managed:        true,
		Ownership:      vnstatOwnershipPanelInstalled,
		InstallMethod:  vnstatInstallMethodGitHubRelease,
		BinaryPath:     "/usr/bin/vnstat",
		FilePaths:      []string{"/usr/bin/vnstat", "/usr/sbin/vnstatd"},
		DataPaths:      defaultVnstatDataPaths(),
		ServiceUnits:   []string{vnstatPanelSystemdUnit},
		EvidenceNonce:  "test-nonce",
		EvidenceSchema: vnstatEvidenceSchema,
	}
	if isTrustedVnstatManifest(trusted) {
		t.Fatal("a manifest without a local ownership marker must be quarantined")
	}
	incomplete := trusted
	incomplete.FilePaths = []string{"/usr/bin/vnstat"}
	if isVnstatManifestBaselineSafe(incomplete) {
		t.Fatal("a panel manifest without the tracked daemon path must be rejected")
	}
	legacy := trusted
	legacy.Ownership = "legacy-managed"
	if isVnstatManifestBaselineSafe(legacy) {
		t.Fatal("a historical ownership record must not become a panel-managed manifest")
	}
	setVnstatOwnershipEvidenceForTest(t, trusted, "test-host", "verified-hash")
	if !isTrustedVnstatManifest(trusted) {
		t.Fatal("matching local ownership evidence should trust the standard panel manifest")
	}
	changedDataPaths := trusted
	changedDataPaths.DataPaths = []string{"/var/lib/vnstat"}
	if isTrustedVnstatManifest(changedDataPaths) {
		t.Fatal("database data paths that differ from local evidence must be quarantined")
	}
	changedServiceUnits := trusted
	changedServiceUnits.ServiceUnits = []string{"vnstat"}
	if isTrustedVnstatManifest(changedServiceUnits) {
		t.Fatal("database service units that differ from local evidence must be quarantined")
	}

	vnstatHostFingerprintFn = func() (string, error) { return "another-host", nil }
	if isTrustedVnstatManifest(trusted) {
		t.Fatal("host-mismatched ownership evidence must be quarantined")
	}
	vnstatHostFingerprintFn = func() (string, error) { return "test-host", nil }

	if err := writeVnstatOwnershipEvidence(trafficOverviewVnstatOwnershipEvidence{
		Schema:          vnstatEvidenceSchema,
		HostFingerprint: "test-host",
		Nonce:           trusted.EvidenceNonce,
		Ownership:       trusted.Ownership,
		InstallMethod:   trusted.InstallMethod,
		BinaryPath:      trusted.BinaryPath,
		Files:           []trafficOverviewVnstatEvidenceFile{{Path: trusted.BinaryPath, SHA256: "changed-hash"}},
		DataPaths:       trusted.DataPaths,
		ServiceUnits:    trusted.ServiceUnits,
	}); err != nil {
		t.Fatalf("rewrite test ownership evidence failed: %v", err)
	}
	if isTrustedVnstatManifest(trusted) {
		t.Fatal("hash-mismatched ownership evidence must be quarantined")
	}

	trusted.FilePaths = append(trusted.FilePaths, "/etc/vnstat.conf")
	if isTrustedVnstatManifest(trusted) {
		t.Fatal("database inventory that differs from evidence must be quarantined")
	}
	trusted.BinaryPath = "/root/vnstat"
	if isTrustedVnstatManifest(trusted) {
		t.Fatal("custom binary path must not form a trusted manifest")
	}

	for _, unsafePath := range []string{"/", "/root", "/root/vnstat", "/usr", "/etc", "../vnstat", "vnstat"} {
		if isSafeVnstatResidualPath(unsafePath) || isSafeVnstatDataPath(unsafePath) {
			t.Fatalf("unsafe path %q passed vnstat deletion whitelist", unsafePath)
		}
	}
	if !isSafeVnstatResidualPath("/usr/share/man/man1/vnstat.1") {
		t.Fatal("official vnstat man page must be tracked as a removable individual file")
	}
}

func TestPopulateVnstatPlatformStatusUsesStartupSnapshot(t *testing.T) {
	previous, previousErr := GetSystemPlatform()
	setSystemPlatformSnapshot(&model.SystemPlatform{
		OS:           "linux",
		SystemFamily: "debian",
		SystemID:     "ubuntu",
		VersionID:    "24.04",
	})
	t.Cleanup(func() {
		if previousErr != nil || previous == nil {
			clearSystemPlatformSnapshot()
			return
		}
		setSystemPlatformSnapshot(previous)
	})

	status := VnstatPackageStatus{}
	populateVnstatPlatformStatus(&status)
	if status.SystemFamily != "debian" || status.SystemID != "ubuntu" || status.SystemVersion != "24.04" {
		t.Fatalf("unexpected status platform fields: %#v", status)
	}
}

func TestVnstatRuntimeConflictContainsOnlyRunningExternalDetails(t *testing.T) {
	clearVnstatRuntimeConflict()
	t.Cleanup(clearVnstatRuntimeConflict)

	recordVnstatRuntimeConflict([]vnstatExternalInstallation{{
		DaemonPath:   "/opt/legacy-vnstat/sbin/vnstatd",
		PIDs:         []int{91, 91, 44},
		ServiceUnits: []string{"legacy-vnstat.service"},
	}})
	conflict := getVnstatRuntimeConflict()
	if conflict == nil {
		t.Fatal("running external vnstat must produce a runtime conflict")
	}
	if !strings.Contains(conflict.Message, "无法启动") || !slices.Equal(conflict.Paths, []string{"/opt/legacy-vnstat/sbin/vnstatd"}) || !slices.Equal(conflict.PIDs, []int{44, 91}) || !slices.Equal(conflict.Units, []string{"legacy-vnstat.service"}) {
		t.Fatalf("unexpected runtime conflict: %#v", conflict)
	}

	clearVnstatRuntimeConflict()
	if getVnstatRuntimeConflict() != nil {
		t.Fatal("successful panel daemon start must clear the runtime conflict")
	}
}

func TestRemoveManagedVnstatForUninstallRemovesVerifiedPanelManifest(t *testing.T) {
	initTrafficOverviewTestDB(t)
	t.Setenv("KWOR_RUNTIME_MODE", "host")
	previousRuntimeGOOS := vnstatRuntimeGOOS
	vnstatRuntimeGOOS = func() string { return "linux" }
	t.Cleanup(func() { vnstatRuntimeGOOS = previousRuntimeGOOS })
	previous, previousErr := GetSystemPlatform()
	clearSystemPlatformSnapshot()
	t.Cleanup(func() {
		if previousErr != nil || previous == nil {
			clearSystemPlatformSnapshot()
			return
		}
		setSystemPlatformSnapshot(previous)
	})

	svc := &TrafficOverviewService{}
	manifest := trafficOverviewVnstatManifest{
		Managed:        true,
		Ownership:      vnstatOwnershipPanelInstalled,
		SystemFamily:   "debian",
		InstallMethod:  vnstatInstallMethodGitHubRelease,
		PackageName:    vnstatPackageName,
		BinaryPath:     "/usr/bin/vnstat",
		FilePaths:      []string{"/usr/bin/vnstat", "/usr/sbin/vnstatd"},
		DataPaths:      defaultVnstatDataPaths(),
		EvidenceNonce:  "uninstall-verified-panel",
		EvidenceSchema: vnstatEvidenceSchema,
	}
	setVnstatOwnershipEvidenceForTest(t, manifest, "test-host", "verified-hash")
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal manifest failed: %v", err)
	}
	if err := (&SettingService{}).setString(trafficOverviewVnstatManifestKey, string(raw)); err != nil {
		t.Fatalf("store manifest failed: %v", err)
	}
	previousEUID := vnstatCurrentEUIDFn
	previousStop := vnstatStopForUninstallFn
	previousRemoveData := vnstatRemoveTrackedDataFn
	previousCleanupCap := vnstatCleanupTrafficCapFn
	stopped := false
	removed := false
	cleanedCap := false
	vnstatCurrentEUIDFn = func() int { return 0 }
	vnstatStopForUninstallFn = func(got trafficOverviewVnstatManifest) error {
		stopped = got.EvidenceNonce == manifest.EvidenceNonce
		return nil
	}
	vnstatRemoveTrackedDataFn = func(got trafficOverviewVnstatManifest) error {
		removed = got.EvidenceNonce == manifest.EvidenceNonce
		return nil
	}
	vnstatCleanupTrafficCapFn = func() error {
		cleanedCap = true
		return nil
	}
	t.Cleanup(func() {
		vnstatCurrentEUIDFn = previousEUID
		vnstatStopForUninstallFn = previousStop
		vnstatRemoveTrackedDataFn = previousRemoveData
		vnstatCleanupTrafficCapFn = previousCleanupCap
	})

	if err := svc.RemoveManagedVnstatForUninstall(); err != nil {
		t.Fatalf("panel uninstall vnstat cleanup failed: %v", err)
	}
	if !stopped || !removed || !cleanedCap {
		t.Fatalf("verified panel manifest must stop daemon, remove tracked artifacts, and clean cap: stopped=%v removed=%v cleanedCap=%v", stopped, removed, cleanedCap)
	}
	if _, ok := svc.loadVnstatManifest(); ok {
		t.Fatal("panel uninstall must clear the verified vnstat manifest")
	}
	if _, err := os.Stat(vnstatEvidencePathFn()); !os.IsNotExist(err) {
		t.Fatalf("panel uninstall must remove local ownership evidence, stat err=%v", err)
	}
}

func TestRemoveManagedVnstatForUninstallPreservesUnverifiedOrHistoricalManifest(t *testing.T) {
	initTrafficOverviewTestDB(t)
	t.Setenv("KWOR_RUNTIME_MODE", "host")
	previousRuntimeGOOS := vnstatRuntimeGOOS
	vnstatRuntimeGOOS = func() string { return "linux" }
	t.Cleanup(func() { vnstatRuntimeGOOS = previousRuntimeGOOS })
	previous, previousErr := GetSystemPlatform()
	setSystemPlatformSnapshot(&model.SystemPlatform{OS: "linux", SystemFamily: "debian"})
	t.Cleanup(func() {
		if previousErr != nil || previous == nil {
			clearSystemPlatformSnapshot()
			return
		}
		setSystemPlatformSnapshot(previous)
	})

	manifest := trafficOverviewVnstatManifest{
		Managed:       true,
		Ownership:     "adopted-system-package",
		InstallMethod: vnstatInstallMethodSystemPackage,
		BinaryPath:    "/usr/bin/vnstat",
		FilePaths:     []string{"/usr/bin/vnstat"},
	}
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal manifest failed: %v", err)
	}
	settingSvc := &SettingService{}
	if err := settingSvc.setString(trafficOverviewVnstatManifestKey, string(raw)); err != nil {
		t.Fatalf("store historical manifest failed: %v", err)
	}

	previousStop := vnstatStopForDeleteFn
	previousRemoveData := vnstatRemoveTrackedDataFn
	previousCleanupCap := vnstatCleanupTrafficCapFn
	stopped := false
	removed := false
	cleanedCap := false
	vnstatStopForDeleteFn = func(trafficOverviewVnstatManifest) error {
		stopped = true
		return nil
	}
	vnstatRemoveTrackedDataFn = func(trafficOverviewVnstatManifest) error {
		removed = true
		return nil
	}
	vnstatCleanupTrafficCapFn = func() error {
		cleanedCap = true
		return errors.New("traffic cap cleanup failed")
	}
	t.Cleanup(func() {
		vnstatStopForDeleteFn = previousStop
		vnstatRemoveTrackedDataFn = previousRemoveData
		vnstatCleanupTrafficCapFn = previousCleanupCap
	})

	if err := (&TrafficOverviewService{}).RemoveManagedVnstatForUninstall(); err != nil {
		t.Fatalf("unverified manifest cleanup should leave host vnstat alone: %v", err)
	}
	if stopped || removed || !cleanedCap {
		t.Fatalf("unverified or historical manifest must only clean cap state: stopped=%v removed=%v cleanedCap=%v", stopped, removed, cleanedCap)
	}
	storedRaw, err := settingSvc.getString(trafficOverviewVnstatManifestKey)
	if err != nil || !strings.Contains(storedRaw, "adopted-system-package") {
		t.Fatalf("historical manifest must remain untouched: raw=%q err=%v", storedRaw, err)
	}
}

func TestRemoveManagedVnstatAbortsBeforeCleanupWhenStopFails(t *testing.T) {
	initTrafficOverviewTestDB(t)
	t.Setenv("KWOR_RUNTIME_MODE", "host")
	previousRuntimeGOOS := vnstatRuntimeGOOS
	vnstatRuntimeGOOS = func() string { return "linux" }
	t.Cleanup(func() { vnstatRuntimeGOOS = previousRuntimeGOOS })
	previousPlatform, previousPlatformErr := GetSystemPlatform()
	setSystemPlatformSnapshot(&model.SystemPlatform{OS: "linux", SystemFamily: "debian"})
	t.Cleanup(func() {
		if previousPlatformErr != nil || previousPlatform == nil {
			clearSystemPlatformSnapshot()
			return
		}
		setSystemPlatformSnapshot(previousPlatform)
	})

	manifest := trafficOverviewVnstatManifest{
		Managed:        true,
		Ownership:      vnstatOwnershipPanelInstalled,
		SystemFamily:   "debian",
		InstallMethod:  vnstatInstallMethodGitHubRelease,
		PackageName:    vnstatPackageName,
		BinaryPath:     "/usr/bin/vnstat",
		FilePaths:      []string{"/usr/bin/vnstat", "/usr/sbin/vnstatd"},
		DataPaths:      defaultVnstatDataPaths(),
		EvidenceNonce:  "delete-stop-failure",
		EvidenceSchema: vnstatEvidenceSchema,
	}
	setVnstatOwnershipEvidenceForTest(t, manifest, "test-host", "verified-hash")
	raw, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("marshal manifest failed: %v", err)
	}
	settingSvc := &SettingService{}
	if err := settingSvc.setString(trafficOverviewVnstatManifestKey, string(raw)); err != nil {
		t.Fatalf("store manifest failed: %v", err)
	}
	if err := settingSvc.setString(trafficOverviewEnabledKey, "true"); err != nil {
		t.Fatalf("enable traffic overview failed: %v", err)
	}

	previousEUID := vnstatCurrentEUIDFn
	previousStop := vnstatStopForDeleteFn
	vnstatCurrentEUIDFn = func() int { return 0 }
	vnstatStopForDeleteFn = func(trafficOverviewVnstatManifest) error { return errors.New("daemon refused to stop") }
	t.Cleanup(func() {
		vnstatCurrentEUIDFn = previousEUID
		vnstatStopForDeleteFn = previousStop
	})

	if _, err := (&TrafficOverviewService{}).RemoveManagedVnstat(); err == nil || !strings.Contains(err.Error(), "已取消删除") {
		t.Fatalf("delete must abort on stop failure, got %v", err)
	}
	if enabled, err := settingSvc.getBool(trafficOverviewEnabledKey); err != nil || !enabled {
		t.Fatalf("traffic setting changed despite rejected delete: enabled=%v err=%v", enabled, err)
	}
	if stored, ok := (&TrafficOverviewService{}).loadVnstatManifest(); !ok || stored.EvidenceNonce != manifest.EvidenceNonce {
		t.Fatalf("manifest was changed despite rejected delete: ok=%v manifest=%+v", ok, stored)
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

func TestNormalizeDetectedVnstatPackageVersion(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{name: "apt epoch and suffix", input: "1:2.12-5build1", want: "2.12-5build1"},
		{name: "rpm release suffix", input: "2.11-3.el9", want: "2.11-3.el9"},
		{name: "plain version", input: "2.13", want: "2.13"},
		{name: "apk package prefix", input: "vnstat-2.13-r2", want: "2.13-r2"},
		{name: "pacman package output", input: "vnstat 2.13-2", want: "2.13-2"},
		{name: "invalid", input: "(none)", want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := normalizeDetectedVnstatPackageVersion(tc.input); got != tc.want {
				t.Fatalf("normalizeDetectedVnstatPackageVersion(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

func TestCompareSemverLikeTagsWithPackageRevision(t *testing.T) {
	cases := []struct {
		name string
		a    string
		b    string
		want int
	}{
		{name: "package revision increases", a: "2.12-1", b: "2.12-2", want: -1},
		{name: "two digit revision compares numerically", a: "2.12-10", b: "2.12-2", want: 1},
		{name: "rpm style release compares numerically", a: "2.12-3.el9", b: "2.12-12.el9", want: -1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := compareSemverLikeTags(tc.a, tc.b); got != tc.want {
				t.Fatalf("compareSemverLikeTags(%q, %q) = %d, want %d", tc.a, tc.b, got, tc.want)
			}
		})
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

func setVnstatOwnershipEvidenceForTest(t *testing.T, manifest trafficOverviewVnstatManifest, hostFingerprint string, hash string) {
	t.Helper()
	previousPathFn := vnstatEvidencePathFn
	previousHostFn := vnstatHostFingerprintFn
	previousHashFn := vnstatEvidenceFileHashFn
	previousPresentFn := vnstatEvidenceFilePresentFn
	markerPath := filepath.Join(t.TempDir(), "ownership.json")
	vnstatEvidencePathFn = func() string { return markerPath }
	vnstatHostFingerprintFn = func() (string, error) { return hostFingerprint, nil }
	vnstatEvidenceFileHashFn = func(path string) (string, error) { return hash, nil }
	vnstatEvidenceFilePresentFn = func(path string) (bool, error) { return true, nil }
	t.Cleanup(func() {
		vnstatEvidencePathFn = previousPathFn
		vnstatHostFingerprintFn = previousHostFn
		vnstatEvidenceFileHashFn = previousHashFn
		vnstatEvidenceFilePresentFn = previousPresentFn
	})

	files := make([]trafficOverviewVnstatEvidenceFile, 0, len(manifest.FilePaths))
	for _, path := range safeVnstatFilePaths(manifest.FilePaths) {
		if isSafeVnstatDataPath(path) {
			continue
		}
		files = append(files, trafficOverviewVnstatEvidenceFile{Path: path, SHA256: hash})
	}
	evidence := trafficOverviewVnstatOwnershipEvidence{
		Schema:          vnstatEvidenceSchema,
		HostFingerprint: hostFingerprint,
		Nonce:           manifest.EvidenceNonce,
		Ownership:       manifest.Ownership,
		InstallMethod:   manifest.InstallMethod,
		PackageManager:  manifest.PackageManager,
		BinaryPath:      manifest.BinaryPath,
		Files:           files,
		DataPaths:       safeVnstatDataPaths(manifest.DataPaths),
		ServiceUnits:    managedVnstatServiceUnits(manifest),
	}
	if err := writeVnstatOwnershipEvidence(evidence); err != nil {
		t.Fatalf("write test ownership evidence failed: %v", err)
	}
}
