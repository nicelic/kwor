package service

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestHostOwnershipStoreKeepsPendingUntilVerifiedCleanup(t *testing.T) {
	store := newHostOwnershipStore(filepath.Join(t.TempDir(), "ownership-v1.json"))
	resource, err := store.Upsert(HostResource{
		ID:            "test-resource",
		Kind:          HostResourceHostFile,
		State:         hostResourceStatePending,
		CleanupPolicy: HostCleanupDelete,
		Paths:         []string{"/tmp/kwor-owned-test"},
	})
	if err != nil {
		t.Fatalf("create pending resource: %v", err)
	}

	manifest, found, err := store.Load()
	if err != nil || !found {
		t.Fatalf("load pending manifest: found=%v err=%v", found, err)
	}
	if len(manifest.Resources) != 1 || manifest.Resources[0].State != hostResourceStatePending {
		t.Fatalf("unexpected pending manifest: %#v", manifest)
	}

	if err := store.MarkState(resource.ID, hostResourceStateActive); err != nil {
		t.Fatalf("activate resource: %v", err)
	}
	if err := store.MarkState(resource.ID, hostResourceStateCleanupPending); err != nil {
		t.Fatalf("mark cleanup pending: %v", err)
	}
	if err := store.Remove(resource.ID); err != nil {
		t.Fatalf("remove verified resource: %v", err)
	}
	if err := store.RemoveManifestIfEmpty(); err != nil {
		t.Fatalf("remove empty manifest: %v", err)
	}
	if _, err := os.Stat(store.path); !os.IsNotExist(err) {
		t.Fatalf("ownership manifest should be removed, stat err=%v", err)
	}
}

func TestOwnershipPathBeforeStatesRecordsExistingAndAbsentPaths(t *testing.T) {
	existingPath := filepath.Join(t.TempDir(), "existing.conf")
	if err := os.WriteFile(existingPath, []byte("before\n"), 0o600); err != nil {
		t.Fatalf("write existing path: %v", err)
	}
	absentPath := filepath.Join(filepath.Dir(existingPath), "absent.conf")
	states := ownershipPathBeforeStates([]string{existingPath, absentPath})
	if existing := states[existingPath]; !existing.Existed || existing.Hash == "" {
		t.Fatalf("existing path state = %#v, want existence and hash", existing)
	}
	if absent := states[absentPath]; absent.Existed || absent.Hash != "" {
		t.Fatalf("absent path state = %#v, want non-existent without hash", absent)
	}
}

func TestRestoreOwnedKernelForwardingUsesRecordedBaseline(t *testing.T) {
	store := newHostOwnershipStore(filepath.Join(t.TempDir(), "ownership-v1.json"))
	if _, err := store.Upsert(HostResource{
		ID:            "kernel-forwarding",
		Kind:          HostResourceKernelForward,
		State:         hostResourceStateActive,
		CleanupPolicy: HostCleanupRestoreValue,
		Paths:         []string{portForwardIPv4ForwardPath, portForwardIPv6ForwardPath},
		Metadata: map[string]string{
			"hostFingerprint": "host-a",
			"ipv4Modified":    "true",
			"ipv4Original":    "0\n",
			"ipv6Modified":    "false",
		},
	}); err != nil {
		t.Fatalf("create forwarding ownership: %v", err)
	}

	previousFingerprint := portForwardKernelHostFingerprintFn
	previousRead := portForwardKernelReadFileFn
	previousWrite := portForwardKernelWriteFileFn
	values := map[string]string{
		portForwardIPv4ForwardPath: "1\n",
		portForwardIPv6ForwardPath: "1\n",
	}
	portForwardKernelHostFingerprintFn = func() (string, error) { return "host-a", nil }
	portForwardKernelReadFileFn = func(path string) ([]byte, error) { return []byte(values[path]), nil }
	portForwardKernelWriteFileFn = func(path string, value []byte, _ os.FileMode) error {
		values[path] = string(value)
		return nil
	}
	t.Cleanup(func() {
		portForwardKernelHostFingerprintFn = previousFingerprint
		portForwardKernelReadFileFn = previousRead
		portForwardKernelWriteFileFn = previousWrite
	})

	manifest, found, err := store.Load()
	if err != nil || !found {
		t.Fatalf("load forwarding manifest: found=%v err=%v", found, err)
	}
	report := &UninstallReport{}
	if err := restoreOwnedKernelForwardingFromManifest(store, manifest, report); err != nil {
		t.Fatalf("restore owned forwarding: %v", err)
	}
	if values[portForwardIPv4ForwardPath] != "0\n" {
		t.Fatalf("ipv4 forwarding = %q, want baseline", values[portForwardIPv4ForwardPath])
	}
	manifest, found, err = store.Load()
	if err != nil || !found {
		t.Fatalf("reload manifest after forwarding cleanup: found=%v err=%v", found, err)
	}
	if len(manifest.Resources) != 0 {
		t.Fatalf("forwarding ownership should be removed after verified restore: %#v", manifest.Resources)
	}
}

func TestRestoreOwnedKernelForwardingSkipsOnlyRestoreOnHostMismatch(t *testing.T) {
	store := newHostOwnershipStore(filepath.Join(t.TempDir(), "ownership-v1.json"))
	if _, err := store.Upsert(HostResource{
		ID:            "kernel-forwarding",
		Kind:          HostResourceKernelForward,
		State:         hostResourceStateActive,
		CleanupPolicy: HostCleanupRestoreValue,
		Metadata: map[string]string{
			"hostFingerprint": "other-host",
			"ipv4Modified":    "true",
			"ipv4Original":    "0\n",
		},
	}); err != nil {
		t.Fatalf("create forwarding ownership: %v", err)
	}
	manifest, found, err := store.Load()
	if err != nil || !found {
		t.Fatalf("load forwarding ownership: found=%v err=%v", found, err)
	}

	writeCalls := 0
	previousWrite := portForwardKernelWriteFileFn
	portForwardKernelWriteFileFn = func(string, []byte, os.FileMode) error {
		writeCalls++
		return nil
	}
	t.Cleanup(func() { portForwardKernelWriteFileFn = previousWrite })

	report := &UninstallReport{}
	if err := restoreOwnedKernelForwardingFromManifest(store, manifest, report, false); err != nil {
		t.Fatalf("skip forwarding restore: %v", err)
	}
	if writeCalls != 0 {
		t.Fatalf("host mismatch must not write forwarding values, write calls=%d", writeCalls)
	}
	if len(report.Preserved) != 1 || !strings.Contains(report.Preserved[0], "host fingerprint mismatch") {
		t.Fatalf("host mismatch report = %#v", report.Preserved)
	}
	manifest, found, err = store.Load()
	if err != nil || !found || len(manifest.Resources) != 0 {
		t.Fatalf("forwarding record should be cleared after intentional skip: found=%v manifest=%#v err=%v", found, manifest, err)
	}
}

func TestAcmeOwnershipRefreshKeepsOriginalBeforeState(t *testing.T) {
	store := newHostOwnershipStore(filepath.Join(t.TempDir(), "ownership-v1.json"))
	path := filepath.Join(t.TempDir(), "acme.sh")
	if err := os.WriteFile(path, []byte("before\n"), 0o700); err != nil {
		t.Fatalf("write initial acme path: %v", err)
	}
	before := ownershipPathBeforeStates([]string{path})
	if _, err := store.Upsert(HostResource{
		ID:            "acme-managed-runtime",
		Kind:          HostResourceACME,
		State:         hostResourceStatePending,
		CleanupPolicy: HostCleanupDelete,
		Paths:         []string{path},
		Before:        before,
	}); err != nil {
		t.Fatalf("record initial acme ownership: %v", err)
	}
	if err := os.WriteFile(path, []byte("after\n"), 0o700); err != nil {
		t.Fatalf("replace acme path: %v", err)
	}
	if _, err := store.Upsert(HostResource{
		ID:            "acme-managed-runtime",
		Kind:          HostResourceACME,
		State:         hostResourceStateActive,
		VerifiedAt:    1,
		CleanupPolicy: HostCleanupDelete,
		Paths:         []string{path},
		Before:        ownershipPathsAssumedNew([]string{path}),
		Hashes:        ownershipPathHashes([]string{path}),
	}); err != nil {
		t.Fatalf("refresh acme ownership: %v", err)
	}
	manifest, found, err := store.Load()
	if err != nil || !found || len(manifest.Resources) != 1 {
		t.Fatalf("load refreshed acme ownership: found=%v manifest=%#v err=%v", found, manifest, err)
	}
	state := manifest.Resources[0].Before[path]
	if !state.Existed || state.Hash != before[path].Hash {
		t.Fatalf("acme refresh replaced initial before state: got=%#v want=%#v", state, before[path])
	}
}

func TestUninstallSystemdUnitFileNamePreservesTimerSuffix(t *testing.T) {
	if got := uninstallSystemdUnitFileName("acme-renew.timer"); got != "acme-renew.timer" {
		t.Fatalf("timer unit filename = %q", got)
	}
	if got := uninstallSystemdUnitFileName("kwor"); got != "kwor.service" {
		t.Fatalf("service unit filename = %q", got)
	}
}

func TestRemoveOwnedAcmeCronLinesKeepsOtherCronEntries(t *testing.T) {
	content := strings.Join([]string{
		"0 2 * * * /opt/Promanager_data/acme/acme.sh --cron",
		"0 3 * * * /root/.acme.sh/acme.sh --cron",
		"0 4 * * * /opt/Promanager_data/acme-backup/acme.sh --cron",
		"0 5 * * * /opt/Promanager_data/acme/acme.sh.backup --cron",
		"",
	}, "\n")
	updated, removed := removeOwnedAcmeCronLines(content, "/opt/Promanager_data")
	if !removed {
		t.Fatal("managed acme cron entry should be selected")
	}
	if updated == content || !containsAllHostOwnershipTest(updated, []string{
		"/root/.acme.sh/acme.sh",
		"/opt/Promanager_data/acme-backup/acme.sh",
		"/opt/Promanager_data/acme/acme.sh.backup",
	}) {
		t.Fatalf("unexpected filtered cron content: %q", updated)
	}
	if strings.Contains(updated, "/opt/Promanager_data/acme/acme.sh --cron") {
		t.Fatalf("managed acme cron entry remains: %q", updated)
	}
}

func TestManagedCoreProcessPathMatchesDeletedExecutable(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Linux deleted executable suffix is not used on Windows")
	}
	expected := filepath.Join(t.TempDir(), "sing-box")
	if !managedCoreProcessPathEquals(expected, expected+" (deleted)") {
		t.Fatalf("deleted executable path should match its original path: %q", expected)
	}
}

func TestVerifyOwnedUninstallResourcePathRejectsModifiedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "owned.conf")
	if err := os.WriteFile(path, []byte("original\n"), 0o600); err != nil {
		t.Fatalf("write owned file: %v", err)
	}
	resource := HostResource{
		Kind:   HostResourceHostFile,
		Paths:  []string{path},
		Hashes: ownershipPathHashes([]string{path}),
	}
	if err := os.WriteFile(path, []byte("changed\n"), 0o600); err != nil {
		t.Fatalf("rewrite owned file: %v", err)
	}
	if err := verifyOwnedUninstallResourcePath(resource, path); err == nil {
		t.Fatal("modified owned file must not be deleted without a refreshed ownership hash")
	}
}

func TestOwnedUninstallResourcePathRemovesRecordedNewAndPreservesPreexistingFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "owned.conf")
	if err := os.WriteFile(path, []byte("original\n"), 0o600); err != nil {
		t.Fatalf("write owned file: %v", err)
	}

	pending := HostResource{
		Kind:   HostResourceHostFile,
		State:  hostResourceStatePending,
		Paths:  []string{path},
		Before: map[string]HostPathBeforeState{path: {}},
	}
	if action, err := ownedUninstallResourcePathAction(pending, path); err != nil || action != ownedUninstallPathRemove {
		t.Fatalf("pending project-created resource action = %v, %v; want remove", action, err)
	}

	preexisting := HostResource{
		Kind:       HostResourceHostFile,
		State:      hostResourceStateActive,
		VerifiedAt: 1,
		Paths:      []string{path},
		Before: map[string]HostPathBeforeState{
			path: {Existed: true, Hash: ownershipPathHashes([]string{path})[path]},
		},
		Hashes: ownershipPathHashes([]string{path}),
	}
	if action, err := ownedUninstallResourcePathAction(preexisting, path); err != nil || action != ownedUninstallPathPreserve {
		t.Fatalf("pre-existing resource action = %v, %v; want preserve", action, err)
	}
}

func TestOwnedUninstallResourcePathRemovesModifiedRecordedNewFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kwor")
	if err := os.WriteFile(path, []byte("original binary\n"), 0o700); err != nil {
		t.Fatalf("write owned binary: %v", err)
	}
	resource := HostResource{
		Kind:       HostResourcePanelRuntime,
		State:      hostResourceStateActive,
		VerifiedAt: 1,
		Paths:      []string{path},
		Before:     ownershipPathsAssumedNew([]string{path}),
		Hashes:     ownershipPathHashes([]string{path}),
	}
	if err := os.WriteFile(path, []byte("locally modified binary\n"), 0o700); err != nil {
		t.Fatalf("modify owned binary: %v", err)
	}
	if action, err := ownedUninstallResourcePathAction(resource, path); err != nil || action != ownedUninstallPathRemove {
		t.Fatalf("modified recorded-new path action = %v, %v; want remove", action, err)
	}
}

func TestPanelProcessPathsForUninstallIncludesVerifiedAliases(t *testing.T) {
	root := t.TempDir()
	primary := filepath.Join(root, "kwor")
	alias := filepath.Join(root, "kwor_amd64")
	dataDir := filepath.Join(root, "Promanager_data")
	manifest := &HostOwnershipManifest{Resources: []HostResource{{
		ID:         "panel-runtime",
		Kind:       HostResourcePanelRuntime,
		State:      hostResourceStateActive,
		VerifiedAt: 1,
		Paths:      []string{primary, alias, dataDir},
	}}}

	paths := panelProcessPathsForUninstall(manifest, KworUninstallOptions{PanelBinaryPath: primary})
	if !containsHostOwnershipPath(paths, primary) || !containsHostOwnershipPath(paths, alias) {
		t.Fatalf("panel process paths missing verified aliases: %#v", paths)
	}
	if containsHostOwnershipPath(paths, dataDir) {
		t.Fatalf("panel process paths must not include data directory: %#v", paths)
	}
}

func TestPanelRuntimeOwnershipPathsKeepsPreviousAliasesAcrossRefresh(t *testing.T) {
	root := t.TempDir()
	primary := filepath.Join(root, "kwor")
	oldAlias := filepath.Join(root, "kwor_amd64")
	dataDir := filepath.Join(root, "Promanager_data")

	paths := panelRuntimeOwnershipPaths(primary, dataDir, nil, []string{oldAlias, dataDir})
	if !containsHostOwnershipPath(paths, primary) {
		t.Fatalf("refreshed ownership paths missing current binary: %#v", paths)
	}
	if !containsHostOwnershipPath(paths, oldAlias) {
		t.Fatalf("refreshed ownership paths dropped old panel alias: %#v", paths)
	}
}

func TestManagedCoreSystemdUnitRecognizesVerifiedLegacyNames(t *testing.T) {
	for _, unit := range []string{"kwor-singbox", "sing-box", "kwor-mihomo", "metacubex-mihomo"} {
		if !isManagedCoreSystemdUnitForUninstall(unit) {
			t.Fatalf("managed core unit %q was not recognized", unit)
		}
	}
	if isManagedCoreSystemdUnitForUninstall("kwor-mtu-opt") {
		t.Fatal("MTU unit must not be treated as a Core unit")
	}
}

func TestPanelRuntimePathAllowsMutableDataDirRemoval(t *testing.T) {
	root := t.TempDir()
	dataDir := filepath.Join(root, "Promanager_data")
	staticPath := filepath.Join(root, "kwor")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatalf("create data dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dataDir, "kwor.db"), []byte("changed after startup\n"), 0o600); err != nil {
		t.Fatalf("write mutable data file: %v", err)
	}
	if err := os.WriteFile(staticPath, []byte("binary\n"), 0o755); err != nil {
		t.Fatalf("write static runtime file: %v", err)
	}
	resource := HostResource{
		Kind:       HostResourcePanelRuntime,
		State:      hostResourceStateActive,
		VerifiedAt: 1,
		Paths:      []string{dataDir, staticPath},
		Before:     ownershipPathsAssumedNew([]string{dataDir, staticPath}),
		Hashes:     map[string]string{dataDir: "stale-directory-hash", staticPath: ownershipPathHashes([]string{staticPath})[staticPath]},
	}
	options := KworUninstallOptions{PanelBinaryPath: staticPath, PanelBinDir: root, DataDir: dataDir}

	if !panelRuntimePathCanRemoveWithoutHash(resource, dataDir, options) {
		t.Fatal("mutable Promanager_data path should be removable without stale directory hash")
	}
	if panelRuntimePathCanRemoveWithoutHash(resource, staticPath, options) {
		t.Fatal("static panel binary must still require normal ownership verification")
	}
}

func TestLegacyHostFileOwnershipRequiresMarker(t *testing.T) {
	root := t.TempDir()
	unmarked := filepath.Join(root, "99-s-ui-optimize.conf")
	marked := filepath.Join(root, "99-kwor-optimize.conf")
	if err := os.WriteFile(unmarked, []byte("net.ipv4.ip_forward=1\n"), 0o644); err != nil {
		t.Fatalf("write unmarked sysctl file: %v", err)
	}
	if err := os.WriteFile(marked, []byte("# kwor-owner:v1 resource=sysctl-dropin\nnet.ipv4.ip_forward=1\n"), 0o644); err != nil {
		t.Fatalf("write marked sysctl file: %v", err)
	}

	if legacyHostFileLooksOwned(unmarked) {
		t.Fatal("unmarked fixed-name sysctl file must not be treated as kwor-owned")
	}
	if !legacyHostFileLooksOwned(marked) {
		t.Fatal("marked legacy host file should be treated as kwor-owned")
	}
}

func TestUninstallSystemdEnableLinksFailsWhenScanCannotComplete(t *testing.T) {
	previousRoots := uninstallSystemdEnableLinkRoots
	previousWalkDir := uninstallSystemdWalkDirFn
	root := t.TempDir()
	uninstallSystemdEnableLinkRoots = []string{root}
	uninstallSystemdWalkDirFn = func(string, fs.WalkDirFunc) error {
		return errors.New("permission denied")
	}
	t.Cleanup(func() {
		uninstallSystemdEnableLinkRoots = previousRoots
		uninstallSystemdWalkDirFn = previousWalkDir
	})

	if _, err := uninstallSystemdEnableLinks("kwor"); err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("enable-link scan error = %v, want propagated permission failure", err)
	}
}

func TestNftOwnershipRejectsEmptyTableOutput(t *testing.T) {
	previousRunNft := runNftFn
	runNftFn = func(args ...string) ([]byte, error) {
		return []byte{}, nil
	}
	t.Cleanup(func() { runNftFn = previousRunNft })

	if owned, err := inspectOwnedNftTableForMutation("inet", "kwor"); err == nil || owned {
		t.Fatalf("empty nft table output ownership = %v, %v; want rejection", owned, err)
	}
	if safe, err := nftTableIsSafeToDelete("inet", "kwor", nil); err != nil || safe {
		t.Fatalf("empty nft table deletion safety = %v, %v; want unsafe", safe, err)
	}
}

func TestPendingMarkedHostFileCanRollbackAfterWriteBeforeActivation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "99-kwor-optimize.conf")
	if err := os.WriteFile(path, []byte("# kwor-owner:v1 resource=sysctl-dropin\nnet.ipv4.ip_forward=1\n"), 0o644); err != nil {
		t.Fatalf("write pending marked host file: %v", err)
	}
	resource := HostResource{
		Kind:          HostResourceHostFile,
		State:         hostResourceStatePending,
		CleanupPolicy: HostCleanupDelete,
		Paths:         []string{path},
		Before:        ownershipPathsAssumedNew([]string{path}),
	}
	if action, err := ownedUninstallResourcePathAction(resource, path); err != nil || action != ownedUninstallPathRemove {
		t.Fatalf("pending marked host file action = %v, %v; want remove", action, err)
	}
}

func TestUpdateWorkspaceAllowsOwnedMutableDirectoryAndBackup(t *testing.T) {
	workDir, err := os.MkdirTemp("", "kwor-panel-update-")
	if err != nil {
		t.Fatalf("create update workspace: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(workDir) })
	if err := os.WriteFile(filepath.Join(workDir, panelUpdateWorkspaceMarkerFileName), []byte(panelUpdateWorkspaceMarkerContent), 0o600); err != nil {
		t.Fatalf("write workspace marker: %v", err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "apply-update.log"), []byte("mutated\n"), 0o600); err != nil {
		t.Fatalf("write mutable workspace log: %v", err)
	}
	targetBinary := filepath.Join(t.TempDir(), "kwor")
	backupPath := targetBinary + ".bak"
	if err := os.WriteFile(backupPath, []byte("old binary\n"), 0o700); err != nil {
		t.Fatalf("write update backup: %v", err)
	}
	resource := HostResource{
		Kind:       HostResourceUpdateWorkspace,
		State:      hostResourceStateActive,
		VerifiedAt: 1,
		Paths:      []string{workDir, backupPath},
		Before:     ownershipPathsAssumedNew([]string{workDir, backupPath}),
		Hashes:     map[string]string{workDir: "stale-workspace-hash"},
		Metadata:   map[string]string{"targetBinary": targetBinary},
	}
	for _, path := range []string{workDir, backupPath} {
		if action, actionErr := ownedUninstallResourcePathAction(resource, path); actionErr != nil || action != ownedUninstallPathRemove {
			t.Fatalf("owned mutable update path %s action = %v, %v; want remove", path, action, actionErr)
		}
	}
}

func TestPanelRuntimeCleanupRemovesModifiedKnownFilesAndKeepsParent(t *testing.T) {
	root := t.TempDir()
	dataDir := filepath.Join(root, "Promanager_data")
	binaryPath := filepath.Join(root, "kwor")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatalf("create panel data dir: %v", err)
	}
	dataFile := filepath.Join(dataDir, "kwor.db")
	if err := os.WriteFile(dataFile, []byte("database\n"), 0o600); err != nil {
		t.Fatalf("write panel data: %v", err)
	}
	if err := os.WriteFile(binaryPath, []byte("original binary\n"), 0o700); err != nil {
		t.Fatalf("write panel binary: %v", err)
	}
	resource := HostResource{
		ID:            "panel-runtime",
		Kind:          HostResourcePanelRuntime,
		State:         hostResourceStateActive,
		CleanupPolicy: HostCleanupDelete,
		VerifiedAt:    1,
		Paths:         []string{dataDir, binaryPath},
		Before:        ownershipPathsAssumedNew([]string{dataDir, binaryPath}),
		Hashes:        ownershipPathHashes([]string{dataDir, binaryPath}),
	}
	store := newHostOwnershipStore(filepath.Join(root, "ownership", "ownership-v1.json"))
	if _, err := store.Upsert(resource); err != nil {
		t.Fatalf("store panel runtime ownership: %v", err)
	}
	if err := os.WriteFile(binaryPath, []byte("user modified binary\n"), 0o700); err != nil {
		t.Fatalf("modify panel binary: %v", err)
	}
	report := &UninstallReport{}
	err := cleanupPanelRuntimeForUninstall(store, KworUninstallOptions{
		PanelBinaryPath: binaryPath,
		PanelBinDir:     root,
		DataDir:         dataDir,
	}, report)
	if err != nil {
		t.Fatalf("panel runtime cleanup = %v", err)
	}
	if _, err := os.Stat(dataFile); !os.IsNotExist(err) {
		t.Fatalf("panel data remains after cleanup, stat error = %v", err)
	}
	if _, err := os.Stat(binaryPath); !os.IsNotExist(err) {
		t.Fatalf("modified panel binary remains after cleanup, stat error = %v", err)
	}
	if _, err := os.Stat(root); err != nil {
		t.Fatalf("install parent must be preserved, stat error = %v", err)
	}
	if !containsHostOwnershipPath(report.Removed, dataDir) || !containsHostOwnershipPath(report.Removed, binaryPath) {
		t.Fatalf("cleanup report missing removed runtime paths: %#v", report.Removed)
	}
}

func TestOwnedHostFileImmutableCleanupUsesOriginalBaseline(t *testing.T) {
	path := filepath.Join(t.TempDir(), "resolv.conf")
	if err := os.WriteFile(path, []byte("nameserver 1.1.1.1\n"), 0o644); err != nil {
		t.Fatalf("write managed host file: %v", err)
	}
	previousDetect := ownedHostFileImmutableDetectFn
	ownedHostFileImmutableDetectFn = func(string) (bool, error) { return true, nil }
	t.Cleanup(func() { ownedHostFileImmutableDetectFn = previousDetect })
	hash := ownershipPathHashes([]string{path})[path]
	base := HostResource{
		Kind:       HostResourceHostFile,
		State:      hostResourceStateActive,
		VerifiedAt: 1,
		Hashes:     map[string]string{path: hash},
	}

	projectAdded := base
	projectAdded.Before = map[string]HostPathBeforeState{path: {Existed: true, ImmutableKnown: true, Immutable: false}}
	if clear, err := shouldClearOwnedHostFileImmutable(projectAdded, path); err != nil || !clear {
		t.Fatalf("project-added immutable decision = %v, %v; want clear", clear, err)
	}

	originallyImmutable := base
	originallyImmutable.Before = map[string]HostPathBeforeState{path: {Existed: true, ImmutableKnown: true, Immutable: true}}
	if clear, err := shouldClearOwnedHostFileImmutable(originallyImmutable, path); err != nil || clear {
		t.Fatalf("original immutable decision = %v, %v; want preserve", clear, err)
	}

	legacyUnknown := base
	legacyUnknown.Before = map[string]HostPathBeforeState{path: {Existed: true}}
	if clear, err := shouldClearOwnedHostFileImmutable(legacyUnknown, path); err != nil || clear {
		t.Fatalf("legacy unknown immutable decision = %v, %v; want preserve", clear, err)
	}
}

func TestRestoreOwnedKernelForwardingRejectsLaterHostChange(t *testing.T) {
	store := newHostOwnershipStore(filepath.Join(t.TempDir(), "ownership-v1.json"))
	if _, err := store.Upsert(HostResource{
		ID:            "kernel-forwarding",
		Kind:          HostResourceKernelForward,
		State:         hostResourceStateActive,
		CleanupPolicy: HostCleanupRestoreValue,
		Metadata: map[string]string{
			"hostFingerprint": "host-a",
			"ipv4Modified":    "true",
			"ipv4Original":    "0\n",
		},
	}); err != nil {
		t.Fatalf("create forwarding ownership: %v", err)
	}
	previousFingerprint := portForwardKernelHostFingerprintFn
	previousRead := portForwardKernelReadFileFn
	previousWrite := portForwardKernelWriteFileFn
	current := "2\n"
	writes := 0
	portForwardKernelHostFingerprintFn = func() (string, error) { return "host-a", nil }
	portForwardKernelReadFileFn = func(string) ([]byte, error) { return []byte(current), nil }
	portForwardKernelWriteFileFn = func(string, []byte, os.FileMode) error { writes++; return nil }
	t.Cleanup(func() {
		portForwardKernelHostFingerprintFn = previousFingerprint
		portForwardKernelReadFileFn = previousRead
		portForwardKernelWriteFileFn = previousWrite
	})
	manifest, _, err := store.Load()
	if err != nil {
		t.Fatalf("load forwarding ownership: %v", err)
	}
	if err := restoreOwnedKernelForwardingFromManifest(store, manifest, &UninstallReport{}); err == nil || !strings.Contains(err.Error(), "no longer matches") {
		t.Fatalf("forwarding conflict error = %v, want later-change rejection", err)
	}
	if writes != 0 || current != "2\n" {
		t.Fatalf("forwarding conflict was overwritten: writes=%d value=%q", writes, current)
	}
	manifest, _, err = store.Load()
	if err != nil || len(manifest.Resources) != 1 {
		t.Fatalf("forwarding ownership evidence must remain for retry: manifest=%#v err=%v", manifest, err)
	}
}

func TestPanelSystemdOwnershipRequiresMarkerAndExactExecStart(t *testing.T) {
	root := t.TempDir()
	binaryPath := filepath.Join(root, "kwor")
	unitPath := filepath.Join(root, "kwor.service")
	if err := os.WriteFile(unitPath, []byte("[Service]\nExecStart="+binaryPath+"\n"), 0o644); err != nil {
		t.Fatalf("write unmarked unit: %v", err)
	}
	if panelSystemdUnitMatchesBinary(unitPath, binaryPath) {
		t.Fatal("unmarked same-name unit must not be adopted")
	}
	if !PanelSystemdUnitExecutesBinary(unitPath, binaryPath) || !PanelSystemdUnitIsManaged(unitPath, binaryPath) {
		t.Fatal("an unmarked legacy unit with exact ExecStart should remain operable")
	}
	if PanelSystemdUnitIsManaged(unitPath, filepath.Join(root, "other")) {
		t.Fatal("an unmarked unit with another ExecStart target must not be managed")
	}
	content := "# kwor-owner:v1 resource=panel-systemd\n[Service]\nExecStart=" + binaryPath + "\n"
	if err := os.WriteFile(unitPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write marked unit: %v", err)
	}
	if !panelSystemdUnitMatchesBinary(unitPath, binaryPath) {
		t.Fatal("marked unit with exact ExecStart should be adopted")
	}
	if panelSystemdUnitMatchesBinary(unitPath, filepath.Join(root, "other")) {
		t.Fatal("unit with another ExecStart target must not be adopted")
	}
	if !PanelSystemdUnitIsManaged(unitPath, filepath.Join(root, "other")) {
		t.Fatal("an explicit panel ownership marker should authorize operational cleanup")
	}
}

func containsHostOwnershipPath(paths []string, want string) bool {
	for _, path := range paths {
		if filepath.Clean(path) == filepath.Clean(want) {
			return true
		}
	}
	return false
}

func containsAllHostOwnershipTest(value string, parts []string) bool {
	for _, part := range parts {
		if !strings.Contains(value, part) {
			return false
		}
	}
	return true
}
