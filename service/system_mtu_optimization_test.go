package service

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/alireza0/s-ui/database"
)

func TestManagedMTUScriptUsesStoredInterfaceAndSysfsFallback(t *testing.T) {
	script := buildManagedMTUScriptContent(1420, "eth0")
	for _, expected := range []string{
		`PREFERRED_IFACE="eth0"`,
		`/sys/class/net/*`,
		`[ -w "/sys/class/net/$IFACE/mtu" ]`,
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
		Interface:   "eth0",
		ScriptPath:  "/var/lib/kwor/_set_mtu_.sh",
	}
	if err := svc.saveMTUPersistedState(want); err != nil {
		t.Fatalf("save MTU state failed: %v", err)
	}
	got, err := svc.loadMTUPersistedState()
	if err != nil {
		t.Fatalf("load MTU state failed: %v", err)
	}
	if got != want {
		t.Fatalf("MTU state mismatch: got=%#v want=%#v", got, want)
	}
}
