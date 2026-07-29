package cmd

import (
	"os"
	"strings"
	"testing"
)

func TestCommandProcessHandlingUsesExecutablePaths(t *testing.T) {
	cmdSource, err := os.ReadFile("cmd.go")
	if err != nil {
		t.Fatalf("read cmd.go: %v", err)
	}
	uninstallSource, err := os.ReadFile("resetadmin_uninstall.go")
	if err != nil {
		t.Fatalf("read resetadmin_uninstall.go: %v", err)
	}
	content := string(cmdSource) + "\n" + string(uninstallSource)

	for _, required := range []string{
		"service.ManagedProcessesRunningByBinaryPath",
		"service.TerminateManagedProcessesByBinaryPathExceptPIDs",
	} {
		if !strings.Contains(content, required) {
			t.Fatalf("command process handling missing %q", required)
		}
	}
	for _, forbidden := range []string{
		"pgrep",
		"pkill",
		"killProcessesByNameExceptSelf",
		"stopManagedChildService",
		"isNamedProcessRunning",
	} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("command process handling contains legacy name-based helper %q", forbidden)
		}
	}
}
