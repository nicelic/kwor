package service

import (
	"strings"
	"testing"
)

func TestBuildSingboxSystemdServiceContent_UsesSystemdSafeExecSyntax(t *testing.T) {
	controlPath := "/opt/kwor/bin/kwor ctl"
	binPath := "/opt/kwor/core/singbox/sing-box"
	configPath := "/opt/kwor/Promanager data/core/singbox/config.json"
	workDir := "/opt/kwor/Promanager data/core/singbox"

	content := buildSingboxSystemdServiceContent(controlPath, binPath, configPath, workDir)

	expectedLines := []string{
		`ExecStartPre=/opt/kwor/bin/kwor\x20ctl materialize-core-config singbox`,
		`ExecStart=/opt/kwor/core/singbox/sing-box run -c /opt/kwor/Promanager\x20data/core/singbox/config.json`,
		`ExecStopPost=/opt/kwor/bin/kwor\x20ctl cleanup-core-config singbox`,
		`WorkingDirectory=/opt/kwor/Promanager\x20data/core/singbox`,
	}
	for _, expected := range expectedLines {
		if !strings.Contains(content, expected) {
			t.Fatalf("expected service content to contain %q, got:\n%s", expected, content)
		}
	}

	unexpectedFragments := []string{
		`ExecStartPre="/opt/kwor/bin/kwor ctl"`,
		`ExecStart="/opt/kwor/core/singbox/sing-box"`,
		`WorkingDirectory="/opt/kwor/Promanager data/core/singbox"`,
	}
	for _, unexpected := range unexpectedFragments {
		if strings.Contains(content, unexpected) {
			t.Fatalf("expected service content to avoid quoted systemd command/path fragment %q, got:\n%s", unexpected, content)
		}
	}
}

func TestBuildMihomoSystemdServiceContent_UsesSystemdSafeExecSyntax(t *testing.T) {
	controlPath := "/opt/kwor/bin/kwor ctl"
	binPath := "/opt/kwor/core/mihomo/mihomo"
	configPath := "/opt/kwor/Promanager data/core/mihomo/server.yaml"
	workDir := "/opt/kwor/Promanager data/core/mihomo"

	content := buildMihomoSystemdServiceContent(controlPath, binPath, configPath, workDir)

	expectedLines := []string{
		`Environment=XDG_CONFIG_HOME=/opt/kwor/Promanager\x20data/core/mihomo/.config`,
		`Environment=XDG_CACHE_HOME=/opt/kwor/Promanager\x20data/core/mihomo/.cache`,
		`ExecStartPre=/opt/kwor/bin/kwor\x20ctl materialize-core-config mihomo`,
		`ExecStart=/opt/kwor/core/mihomo/mihomo -d /opt/kwor/Promanager\x20data/core/mihomo -f /opt/kwor/Promanager\x20data/core/mihomo/server.yaml`,
		`ExecStopPost=/opt/kwor/bin/kwor\x20ctl cleanup-core-config mihomo`,
		`WorkingDirectory=/opt/kwor/Promanager\x20data/core/mihomo`,
	}
	for _, expected := range expectedLines {
		if !strings.Contains(content, expected) {
			t.Fatalf("expected service content to contain %q, got:\n%s", expected, content)
		}
	}

	unexpectedFragments := []string{
		`Environment="XDG_CONFIG_HOME=/opt/kwor/Promanager data/core/mihomo/.config"`,
		`ExecStart="/opt/kwor/core/mihomo/mihomo"`,
		`WorkingDirectory="/opt/kwor/Promanager data/core/mihomo"`,
	}
	for _, unexpected := range unexpectedFragments {
		if strings.Contains(content, unexpected) {
			t.Fatalf("expected service content to avoid quoted systemd command/path fragment %q, got:\n%s", unexpected, content)
		}
	}
}
