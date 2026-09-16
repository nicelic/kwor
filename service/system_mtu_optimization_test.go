package service

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/alireza0/s-ui/database"
)

func TestManagedMTUScriptUsesStoredInterfacesAndSysfsFallback(t *testing.T) {
	script := buildManagedMTUScriptContent(1420, []string{"eth0", "eth1"})
	for _, expected := range []string{
		`TARGET_IFACES="eth0 eth1"`,
		`for iface in $TARGET_IFACES; do`,
		`/sys/class/net/*`,
		`[ -w "/sys/class/net/$target/mtu" ]`,
		`printf '%s\n' "$MTU_VALUE"`,
	} {
		if !strings.Contains(script, expected) {
			t.Fatalf("managed MTU script is missing %q", expected)
		}
	}
}

func TestParseSystemdUnitStatus(t *testing.T) {
	unitState, activeState := parseSystemdUnitStatus("ActiveState=active\nUnitFileState=enabled\n")
	if unitState != "enabled" || activeState != "active" {
		t.Fatalf("unexpected systemd status: unit=%q active=%q", unitState, activeState)
	}
}

func TestParseSystemdUnitStatusTreatsMissingUnitAsNotFound(t *testing.T) {
	unitState, activeState := parseSystemdUnitStatus("LoadState=not-found\nActiveState=inactive\nUnitFileState=\n")
	if unitState != "not-found" || activeState != "inactive" {
		t.Fatalf("unexpected missing systemd status: unit=%q active=%q", unitState, activeState)
	}
}

func TestSanitizeInterfaceNameRejectsShellSyntax(t *testing.T) {
	if got := sanitizeInterfaceName("eth0@if12"); got != "eth0" {
		t.Fatalf("unexpected normalized interface: %q", got)
	}
	if got := sanitizeInterfaceName(`eth0";reboot`); got != "" {
		t.Fatalf("unsafe interface name was accepted: %q", got)
	}
}

func TestSaveAndLoadMTUPersistedState(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "mtu-state.db")
	if err := database.InitDB(dbPath); err != nil {
		t.Fatalf("init MTU state db failed: %v", err)
	}
	if sqlDB, err := database.GetDB().DB(); err == nil && sqlDB != nil {
		t.Cleanup(func() { _ = sqlDB.Close() })
	}

	svc := &SystemMTUOptimizationService{}
	want := systemMTUPersistedState{
		Enabled:     true,
		MTU:         1420,
		OriginalMTU: 1500,
		OriginalMTUs: map[string]int{
			"eth0": 1500,
			"eth1": 1500,
		},
		Interface:  "eth0, eth1",
		Interfaces: []string{"eth0", "eth1"},
		ScriptPath: "/var/lib/kwor/_set_mtu_.sh",
	}
	if err := svc.saveMTUPersistedState(want); err != nil {
		t.Fatalf("save MTU state failed: %v", err)
	}
	got, err := svc.loadMTUPersistedState()
	if err != nil {
		t.Fatalf("load MTU state failed: %v", err)
	}
	if got.Enabled != want.Enabled || got.MTU != want.MTU || got.OriginalMTU != want.OriginalMTU || got.ScriptPath != want.ScriptPath {
		t.Fatalf("MTU state basic fields mismatch: got=%#v want=%#v", got, want)
	}
	if len(got.Interfaces) != len(want.Interfaces) || got.Interfaces[0] != "eth0" || got.Interfaces[1] != "eth1" {
		t.Fatalf("MTU state interfaces mismatch: got=%#v want=%#v", got.Interfaces, want.Interfaces)
	}
	if got.OriginalMTUs["eth0"] != 1500 || got.OriginalMTUs["eth1"] != 1500 {
		t.Fatalf("MTU state original MTUs mismatch: got=%#v want=%#v", got.OriginalMTUs, want.OriginalMTUs)
	}
}
