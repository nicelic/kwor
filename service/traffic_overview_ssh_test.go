package service

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseSSHPortsFromConfig_WithIncludeChain(t *testing.T) {
	root := t.TempDir()

	mainPath := filepath.Join(root, "sshd_config")
	includeDir := filepath.Join(root, "sshd_config.d")
	nestedDir := filepath.Join(root, "nested")
	if err := os.MkdirAll(includeDir, 0o755); err != nil {
		t.Fatalf("mkdir include dir failed: %v", err)
	}
	if err := os.MkdirAll(nestedDir, 0o755); err != nil {
		t.Fatalf("mkdir nested dir failed: %v", err)
	}

	customPath := filepath.Join(root, "custom.conf")
	if err := os.WriteFile(mainPath, []byte(
		"# main\n"+
			"Port 2222\n"+
			"Include sshd_config.d/*.conf "+customPath+"\n",
	), 0o644); err != nil {
		t.Fatalf("write main config failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(includeDir, "10-base.conf"), []byte("Port 2200\n"), 0o644); err != nil {
		t.Fatalf("write include file 10 failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(includeDir, "20-extra.conf"), []byte("Port 2300\nPort 2400\n"), 0o644); err != nil {
		t.Fatalf("write include file 20 failed: %v", err)
	}
	if err := os.WriteFile(customPath, []byte("Include nested/*.conf\nPort 2500\n"), 0o644); err != nil {
		t.Fatalf("write custom config failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(nestedDir, "a.conf"), []byte("Port 2600\n"), 0o644); err != nil {
		t.Fatalf("write nested config failed: %v", err)
	}

	got := parseSSHPortsFromConfig(mainPath)
	want := []int{2200, 2222, 2300, 2400, 2500, 2600}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ports mismatch: got %v want %v", got, want)
	}
}

func TestParseSSHPortsFromConfig_IgnoresInvalidValues(t *testing.T) {
	root := t.TempDir()
	mainPath := filepath.Join(root, "sshd_config")
	content := "Port 22\nPort bad\nPort 70000\nPort 2200 # comment\n"
	if err := os.WriteFile(mainPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write config failed: %v", err)
	}

	got := parseSSHPortsFromConfig(mainPath)
	want := []int{22, 2200}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ports mismatch: got %v want %v", got, want)
	}
}

func TestParseSSHPortsFromConfig_SkipsMatchSectionPorts(t *testing.T) {
	root := t.TempDir()
	mainPath := filepath.Join(root, "sshd_config")
	content := strings.Join([]string{
		"Port 22",
		"Include sshd_config.d/*.conf",
		"Match User backup",
		"  Port 2222",
		"",
	}, "\n")
	if err := os.WriteFile(mainPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write config failed: %v", err)
	}

	includeDir := filepath.Join(root, "sshd_config.d")
	if err := os.MkdirAll(includeDir, 0o755); err != nil {
		t.Fatalf("mkdir include dir failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(includeDir, "10-extra.conf"), []byte("Port 2200\n"), 0o644); err != nil {
		t.Fatalf("write include config failed: %v", err)
	}

	got := parseSSHPortsFromConfig(mainPath)
	want := []int{22, 2200}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ports mismatch: got %v want %v", got, want)
	}
}
