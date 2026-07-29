package cmd

import (
	"testing"
	"time"

	"github.com/alireza0/s-ui/service"
)

func TestShouldRemoveInstallDirAfterUninstall(t *testing.T) {
	testCases := []struct {
		name string
		dir  string
		want bool
	}{
		{
			name: "dedicated kwor dir under opt must not be removed",
			dir:  "/opt/kwor",
			want: false,
		},
		{
			name: "legacy s-ui dir under usr local must not be removed",
			dir:  "/usr/local/s-ui",
			want: false,
		},
		{
			name: "public opt dir must not be removed",
			dir:  "/opt",
			want: false,
		},
		{
			name: "protected home dir must not be removed",
			dir:  "/home",
			want: false,
		},
		{
			name: "generic custom dir without app name must not be removed",
			dir:  "/data/releases/current",
			want: false,
		},
		{
			name: "windows program files must not be removed",
			dir:  "C:/Program Files",
			want: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			got := shouldRemoveInstallDirAfterUninstall(tc.dir)
			if got != tc.want {
				t.Fatalf("shouldRemoveInstallDirAfterUninstall(%q) = %v, want %v", tc.dir, got, tc.want)
			}
		})
	}
}

func TestHandleUninstallCommandRejectsUnknownArguments(t *testing.T) {
	err := handleUninstallCommand([]string{"--unexpected"})
	if err == nil || err.Error() != "usage: kwor uninstall" {
		t.Fatalf("handleUninstallCommand() error = %v", err)
	}
}

func TestTerminalAndPanelUninstallUseSharedCleanup(t *testing.T) {
	previousWait := waitBeforePanelConfirmedUninstall
	previousCleanup := executeKworUninstall
	t.Cleanup(func() {
		waitBeforePanelConfirmedUninstall = previousWait
		executeKworUninstall = previousCleanup
	})

	var waited time.Duration
	cleanupCalls := 0
	waitBeforePanelConfirmedUninstall = func(delay time.Duration) {
		waited = delay
	}
	executeKworUninstall = func() error {
		cleanupCalls++
		return nil
	}

	terminalInputs := []string{"y", kworServiceName}
	if err := runTerminalUninstall(func(prompt string, defaultValue string) string {
		if len(terminalInputs) == 0 {
			t.Fatalf("unexpected terminal prompt: %q", prompt)
		}
		result := terminalInputs[0]
		terminalInputs = terminalInputs[1:]
		return result
	}); err != nil {
		t.Fatalf("terminal uninstall failed: %v", err)
	}
	if cleanupCalls != 1 {
		t.Fatalf("terminal shared cleanup calls = %d, want 1", cleanupCalls)
	}

	if err := runPanelConfirmedUninstall(); err != nil {
		t.Fatalf("panel uninstall failed: %v", err)
	}
	if waited != 2*time.Second {
		t.Fatalf("panel uninstall delay = %s, want %s", waited, 2*time.Second)
	}
	if cleanupCalls != 2 {
		t.Fatalf("shared cleanup calls = %d, want 2", cleanupCalls)
	}

	if service.PanelUninstallConfirmedArgument != "--confirmed-from-panel" {
		t.Fatalf("unexpected panel uninstall argument: %q", service.PanelUninstallConfirmedArgument)
	}
}
