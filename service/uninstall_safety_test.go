package service

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/shirou/gopsutil/v4/process"
)

func TestVerifySystemdUnitStatusAbsentRequiresExplicitNotFoundState(t *testing.T) {
	if err := verifySystemdUnitStatusAbsentForUninstall("kwor", uninstallSystemdUnitStatus{}); err == nil {
		t.Fatal("an entirely empty systemd status must not prove absence")
	}
	if err := verifySystemdUnitStatusAbsentForUninstall("kwor", uninstallSystemdUnitStatus{
		LoadState:     "not-found",
		ActiveState:   "inactive",
		UnitFileState: "masked",
	}); err == nil {
		t.Fatal("a masked unit must not be treated as absent")
	}
	if err := verifySystemdUnitStatusAbsentForUninstall("kwor", uninstallSystemdUnitStatus{
		LoadState:   "not-found",
		ActiveState: "inactive",
	}); err != nil {
		t.Fatalf("explicit not-found systemd status was rejected: %v", err)
	}
}

func TestSystemdUnitActivePropagatesStatusQueryFailure(t *testing.T) {
	previousShow := uninstallSystemdShowFn
	uninstallSystemdShowFn = func(string, string) (uninstallSystemdUnitStatus, error) {
		return uninstallSystemdUnitStatus{}, errors.New("system bus unavailable")
	}
	t.Cleanup(func() { uninstallSystemdShowFn = previousShow })
	if _, err := systemdUnitActiveForUninstall("systemctl", "kwor"); err == nil || !strings.Contains(err.Error(), "system bus") {
		t.Fatalf("systemd active query error = %v, want propagated bus failure", err)
	}
}

func TestNftOwnershipMarkerRejectsArbitraryRuleComments(t *testing.T) {
	tableMarker := []byte(`table inet kwor {
	comment "kwor-owner:v1"
	chain input { }
}`)
	if !nftTableHasOwnershipMarker(tableMarker) {
		t.Fatal("table-level ownership marker was not recognized")
	}
	ruleMarker := []byte(`table inet kwor {
	chain input {
		tcp dport 443 accept comment "kwor-owner:v1"
	}
}`)
	if nftTableHasOwnershipMarker(ruleMarker) {
		t.Fatal("a rule comment must not confer ownership of the whole table")
	}
}

func TestCreateOwnedNftTableRollsBackWhenMarkerVerificationIsEmpty(t *testing.T) {
	withNftCapabilitiesTestGlobals(t)
	setNftCapabilitiesForTest(buildNftablesCapabilities("nftables v0.9.8", "5.10.0", nil))

	previousRunNft := runNftFn
	previousManifestPath := hostOwnershipManifestPathFn
	manifestRoot := t.TempDir()
	hostOwnershipManifestPathFn = func() string { return filepath.Join(manifestRoot, "ownership-v1.json") }
	listCalls := 0
	commands := make([]string, 0)
	runNftFn = func(args ...string) ([]byte, error) {
		command := strings.Join(args, " ")
		commands = append(commands, command)
		switch {
		case strings.HasPrefix(command, "list table"):
			listCalls++
			if listCalls == 2 {
				return []byte{}, nil
			}
			return nil, errors.New("No such file or directory")
		case strings.HasPrefix(command, "add table"):
			return nil, nil
		case strings.HasPrefix(command, "add chain"), strings.HasPrefix(command, "add rule"):
			return nil, nil
		case strings.HasPrefix(command, "delete table"):
			return nil, nil
		default:
			return nil, errors.New("unexpected nft command: " + command)
		}
	}
	t.Cleanup(func() {
		runNftFn = previousRunNft
		hostOwnershipManifestPathFn = previousManifestPath
	})

	err := createOwnedNftTable("nft-test", "inet", "kwor")
	if err == nil || !strings.Contains(err.Error(), "marker is missing") {
		t.Fatalf("createOwnedNftTable error = %v, want marker verification failure", err)
	}
	deleteSeen := false
	for _, command := range commands {
		if strings.HasPrefix(command, "delete table inet kwor") {
			deleteSeen = true
		}
	}
	if !deleteSeen {
		t.Fatalf("created nft table was not rolled back: %#v", commands)
	}
	manifest, found, loadErr := LoadHostOwnershipManifest()
	if loadErr != nil {
		t.Fatalf("load nft rollback ownership: %v", loadErr)
	}
	if found && manifest != nil && len(manifest.Resources) != 0 {
		t.Fatalf("rolled-back nft table retained destructive ownership: %#v", manifest.Resources)
	}
}

func TestManagedProcessIdentityRejectsChangedCreateTime(t *testing.T) {
	proc, err := process.NewProcess(int32(os.Getpid()))
	if err != nil {
		t.Fatalf("open current process: %v", err)
	}
	binaryPath, err := os.Executable()
	if err != nil {
		t.Fatalf("resolve current executable: %v", err)
	}
	identity, err := captureManagedProcessIdentity(proc, binaryPath)
	if err != nil {
		t.Fatalf("capture current process identity: %v", err)
	}
	if !managedProcessIdentityMatches(identity) {
		t.Fatal("fresh process identity did not match")
	}
	identity.createTime++
	if managedProcessIdentityMatches(identity) {
		t.Fatal("a reused PID with a different create time must not match")
	}
}

func TestCollectOwnedSystemdTargetsKeepsAcmeAndVnstatEvidence(t *testing.T) {
	acmePath := filepath.Clean(filepath.Join("root", "acme.timer"))
	vnstatPath := filepath.Clean(filepath.Join("root", "kwor-vnstat.service"))
	manifest := &HostOwnershipManifest{Resources: []HostResource{
		{
			ID:     "acme-managed-runtime",
			Kind:   HostResourceACME,
			Paths:  []string{acmePath},
			Units:  []string{"acme.timer"},
			Hashes: map[string]string{acmePath: "acme-hash"},
			Before: map[string]HostPathBeforeState{acmePath: {Existed: false}},
		},
		{
			ID:       "vnstat-managed-runtime",
			Kind:     HostResourceVnStat,
			Paths:    []string{vnstatPath},
			Units:    []string{"kwor-vnstat"},
			Hashes:   map[string]string{vnstatPath: "vnstat-hash"},
			Before:   map[string]HostPathBeforeState{vnstatPath: {Existed: true}},
			Metadata: map[string]string{"installMethod": vnstatInstallMethodGitHubRelease},
		},
	}}

	targets, remaining := collectOwnedSystemdUninstallTargets(manifest, KworUninstallOptions{}, false)
	for _, tc := range []struct {
		unit       string
		resourceID string
		path       string
		hash       string
		existed    bool
	}{
		{unit: "acme.timer", resourceID: "acme-managed-runtime", path: acmePath, hash: "acme-hash"},
		{unit: "kwor-vnstat", resourceID: "vnstat-managed-runtime", path: vnstatPath, hash: "vnstat-hash", existed: true},
	} {
		target := targets[tc.unit]
		if target == nil {
			t.Fatalf("missing systemd target for %s", tc.unit)
		}
		if _, ok := target.resourceIDs[tc.resourceID]; ok {
			t.Fatalf("external resource %s must remain owned by its dedicated cleanup stage", tc.resourceID)
		}
		if len(target.paths) != 1 || target.paths[0] != tc.path {
			t.Fatalf("target %s paths = %#v, want %q", tc.unit, target.paths, tc.path)
		}
		if target.hashes[tc.path] != tc.hash {
			t.Fatalf("target %s hash = %q, want %q", tc.unit, target.hashes[tc.path], tc.hash)
		}
		if target.before[tc.path].Existed != tc.existed {
			t.Fatalf("target %s before.Existed = %v, want %v", tc.unit, target.before[tc.path].Existed, tc.existed)
		}
		if remaining[tc.resourceID] != 0 {
			t.Fatalf("external resource %s must not be removed by systemd cleanup", tc.resourceID)
		}
	}
}
