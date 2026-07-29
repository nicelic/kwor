package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallScriptUsesExactExecutablePaths(t *testing.T) {
	contentBytes, err := os.ReadFile(filepath.Join("..", "install.sh"))
	if err != nil {
		t.Fatalf("read install.sh: %v", err)
	}
	content := string(contentBytes)

	for _, required := range []string{
		"service_file_is_kwor_install()",
		"process_matches_binary_path()",
		"process_start_time()",
		"stop_processes_by_binary_path()",
		"/proc/[0-9]*/exe",
		"--property=MainPID",
		"wait_for_target_runtime",
	} {
		if !strings.Contains(content, required) {
			t.Fatalf("install.sh missing exact-process safeguard %q", required)
		}
	}
	for _, forbidden := range []string{
		"pgrep",
		"pkill",
		"find_running_pid",
		"INSTALL_SOURCE=\"running process\"",
		"\"${STOP_BIN_PATH}\" stop",
	} {
		if strings.Contains(content, forbidden) {
			t.Fatalf("install.sh contains unsafe legacy process handling %q", forbidden)
		}
	}
}
