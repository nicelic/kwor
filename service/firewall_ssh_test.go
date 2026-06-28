package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRenderSSHConfigWithDirectiveOverrides_InsertsBeforeMatch(t *testing.T) {
	input := strings.Join([]string{
		"# base config",
		"Port 22",
		"AllowTcpForwarding no",
		"Match User backup",
		"  AllowTcpForwarding yes",
		"",
	}, "\r\n")

	updated, changed := renderSSHConfigWithDirectiveOverrides([]byte(input), map[string]string{
		sshDirectivePort:               "2222",
		sshDirectiveAllowTcpForwarding: "yes",
		sshDirectivePermitOpen:         "any",
		sshDirectiveGatewayPorts:       "no",
	})
	if !changed {
		t.Fatalf("expected config render to change")
	}

	result := string(updated)
	if !strings.Contains(result, "\r\n") {
		t.Fatalf("expected CRLF line endings to be preserved")
	}

	matchIndex := strings.Index(result, "Match User backup")
	if matchIndex < 0 {
		t.Fatalf("missing Match block in updated config")
	}

	beforeMatch := result[:matchIndex]
	afterMatch := result[matchIndex:]
	for _, expected := range []string{
		"Port 2222",
		"AllowTcpForwarding yes",
		"PermitOpen any",
		"GatewayPorts no",
	} {
		if !strings.Contains(beforeMatch, expected) {
			t.Fatalf("expected directive %q before Match block, got: %s", expected, beforeMatch)
		}
	}
	if strings.Contains(afterMatch, "Port 2222") {
		t.Fatalf("updated global port should not be injected inside Match block")
	}
}

func TestRenderSSHConfigWithDirectiveOverrides_IgnoreUnknownAndEmpty(t *testing.T) {
	input := "Port 22\n"
	updated, changed := renderSSHConfigWithDirectiveOverrides([]byte(input), map[string]string{
		"UnknownKey":           "value",
		sshDirectivePermitOpen: "   ",
	})
	if changed {
		t.Fatalf("unexpected change for unknown/empty overrides: %s", string(updated))
	}
	if string(updated) != input {
		t.Fatalf("content should remain unchanged")
	}
}

func TestProbeSSHConfig_ResolvesIncludeAndSkipsMatchSection(t *testing.T) {
	root := t.TempDir()
	mainPath := filepath.Join(root, "sshd_config")
	includeDir := filepath.Join(root, "sshd_config.d")
	if err := os.MkdirAll(includeDir, 0o755); err != nil {
		t.Fatalf("mkdir include dir failed: %v", err)
	}

	mainContent := strings.Join([]string{
		"Port 2200",
		"AllowTcpForwarding yes",
		"Include sshd_config.d/*.conf",
		"Match User backup",
		"  Port 2999",
		"  AllowTcpForwarding no",
		"",
	}, "\n")
	if err := os.WriteFile(mainPath, []byte(mainContent), 0o644); err != nil {
		t.Fatalf("write main config failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(includeDir, "10-proxy.conf"), []byte(strings.Join([]string{
		"PermitOpen any",
		"GatewayPorts no",
		"Port 2300",
		"",
	}, "\n")), 0o644); err != nil {
		t.Fatalf("write include file failed: %v", err)
	}

	probe, err := probeSSHConfig(mainPath)
	if err != nil {
		t.Fatalf("probeSSHConfig returned error: %v", err)
	}
	if probe.Port != 2200 {
		t.Fatalf("primary port mismatch: got %d want %d", probe.Port, 2200)
	}
	if len(probe.Ports) != 2 || probe.Ports[0] != 2200 || probe.Ports[1] != 2300 {
		t.Fatalf("ports mismatch: got %v want [2200 2300]", probe.Ports)
	}
	if probe.AllowTcpForwarding != "yes" {
		t.Fatalf("allowtcpforwarding mismatch: got %q", probe.AllowTcpForwarding)
	}
	if probe.PermitOpen != "any" {
		t.Fatalf("permitopen mismatch: got %q", probe.PermitOpen)
	}
	if probe.GatewayPorts != "no" {
		t.Fatalf("gatewayports mismatch: got %q", probe.GatewayPorts)
	}
}

func TestRenderSSHConfigWithDirectiveOverrides_InsertsBeforeInclude(t *testing.T) {
	input := strings.Join([]string{
		"# managed by distro",
		"Include /etc/ssh/sshd_config.d/*.conf",
		"",
	}, "\n")

	updated, changed := renderSSHConfigWithDirectiveOverrides([]byte(input), map[string]string{
		sshDirectivePort:               "2222",
		sshDirectiveAllowTcpForwarding: "yes",
	})
	if !changed {
		t.Fatalf("expected config render to change")
	}

	resultLines := strings.Split(strings.TrimSpace(string(updated)), "\n")
	if len(resultLines) < 4 {
		t.Fatalf("updated config too short: %q", string(updated))
	}
	if resultLines[1] != "Port 2222" {
		t.Fatalf("expected inserted Port before Include, got lines: %#v", resultLines)
	}
	if resultLines[2] != "AllowTcpForwarding yes" {
		t.Fatalf("expected inserted AllowTcpForwarding before Include, got lines: %#v", resultLines)
	}
	if resultLines[3] != "Include /etc/ssh/sshd_config.d/*.conf" {
		t.Fatalf("expected Include to stay after inserted directives, got lines: %#v", resultLines)
	}
}
