package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadPanelUpdateLastError(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "panel-update-last.log")
	content := strings.Join([]string{
		"",
		"line-1",
		"line-2",
		"line-3",
		"line-4",
		"line-5",
		"",
	}, "\n")
	if err := os.WriteFile(logPath, []byte(content), 0o600); err != nil {
		t.Fatalf("write log failed: %v", err)
	}

	got := readPanelUpdateLastError(logPath)
	want := "line-2 | line-3 | line-4 | line-5"
	if got != want {
		t.Fatalf("readPanelUpdateLastError()=%q want %q", got, want)
	}
}

func TestClearPanelUpdateLastError(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "panel-update-last.log")
	if err := os.WriteFile(logPath, []byte("failure"), 0o600); err != nil {
		t.Fatalf("write log failed: %v", err)
	}

	clearPanelUpdateLastError(logPath)

	if _, err := os.Stat(logPath); !os.IsNotExist(err) {
		t.Fatalf("expected log to be removed, stat err=%v", err)
	}
}

func TestWritePanelUpdateScriptIncludesRuntimeVerificationAndFailureLog(t *testing.T) {
	tempDir := t.TempDir()
	scriptPath, err := writePanelUpdateScript(
		tempDir,
		"/opt/kwor/kwor",
		"/tmp/staged-kwor",
		"/tmp/install.sh",
		"/tmp/kwor.service",
		"kwor",
	)
	if err != nil {
		t.Fatalf("writePanelUpdateScript failed: %v", err)
	}

	contentBytes, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("read script failed: %v", err)
	}
	content := string(contentBytes)

	for _, needle := range []string{
		"LAST_LOG_PATH=",
		"UPDATE_SUCCESS=0",
		"wait_for_target_runtime()",
		"if \"$TARGET_BIN\" start >> \"$LOG_PATH\" 2>&1 && wait_for_target_runtime; then",
		"if wait_for_target_runtime; then",
		"cp -f \"$LOG_PATH\" \"$LAST_LOG_PATH\"",
		"Promanager_data",
		"UPDATE_SUCCESS=1",
	} {
		if !strings.Contains(content, needle) {
			t.Fatalf("script missing %q", needle)
		}
	}
}

func TestLoadPanelUpdateLogViewTrimsAndCapsLines(t *testing.T) {
	tempDir := t.TempDir()
	logPath := filepath.Join(tempDir, "panel-update-last.log")
	content := strings.Join([]string{
		"",
		"first",
		"second",
		"third",
		"fourth",
		"fifth",
		"",
	}, "\n")
	if err := os.WriteFile(logPath, []byte(content), 0o600); err != nil {
		t.Fatalf("write log failed: %v", err)
	}

	view, err := loadPanelUpdateLogView(logPath)
	if err != nil {
		t.Fatalf("loadPanelUpdateLogView failed: %v", err)
	}
	if !view.Exists {
		t.Fatalf("expected Exists=true")
	}
	if len(view.Lines) == 0 {
		t.Fatalf("expected lines to be returned")
	}
	if view.Lines[0] != "first" {
		t.Fatalf("first line=%q want %q", view.Lines[0], "first")
	}

	normalized := normalizePanelUpdateLogLines([]byte(content), 3)
	if len(normalized) != 4 {
		t.Fatalf("normalized line count=%d want 4", len(normalized))
	}
	if normalized[0] != "日志过长，已隐藏较早输出" {
		t.Fatalf("normalized head=%q", normalized[0])
	}
	if normalized[3] != "fifth" {
		t.Fatalf("normalized last=%q want %q", normalized[3], "fifth")
	}
}
