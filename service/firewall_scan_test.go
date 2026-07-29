package service

import (
	"strings"
	"testing"
)

func TestScanExternalFirewallRulesRequestsNumericPorts(t *testing.T) {
	withNftCapabilitiesTestGlobals(t)
	nftSupportedFn = func() bool { return true }

	commands := make([]string, 0)
	runNftFn = func(args ...string) ([]byte, error) {
		command := strings.Join(args, " ")
		commands = append(commands, command)
		switch command {
		case "list tables":
			return nil, nil
		case "--handle --numeric --numeric list ruleset":
			return []byte(`table ip external {
  chain input {
    type filter hook input priority filter; policy accept;
    tcp dport 443 counter packets 1 bytes 10 accept comment "external_https" # handle 17
  }
}
`), nil
		default:
			return nil, nil
		}
	}

	rules, err := scanExternalFirewallRules()
	if err != nil {
		t.Fatalf("scan external firewall rules: %v", err)
	}
	if len(rules) != 1 {
		t.Fatalf("rules=%v", rules)
	}
	if rules[0].PortSpec != "443" {
		t.Fatalf("port=%q", rules[0].PortSpec)
	}
	if !containsNftCommand(commands, "--handle --numeric --numeric list ruleset") {
		t.Fatalf("external scan must request numeric ports: %v", commands)
	}
}
