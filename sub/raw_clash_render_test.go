package sub

import (
	"strings"
	"testing"
)

func TestRenderClashSubscriptionFromEntriesPreservesRawYAMLBytes(t *testing.T) {
	rawProxyYAML := "  -   name: raw-node\n      type: trojan\n      server: 1.1.1.1\n      port: 443\n      password: \"p,ass\"\n"

	rendered, err := renderClashSubscriptionFromEntries(
		[]clashProxyRenderEntry{
			{
				Name: "raw-node",
				Proxy: map[string]interface{}{
					"name":     "raw-node",
					"type":     "trojan",
					"server":   "1.1.1.1",
					"port":     443,
					"password": "p,ass",
				},
				RawYAML: []byte(rawProxyYAML),
			},
		},
		"http://www.gstatic.com/generate_204",
		300,
		50,
		nil,
	)
	if err != nil {
		t.Fatalf("renderClashSubscriptionFromEntries failed: %v", err)
	}

	text := string(rendered)
	if !strings.Contains(text, rawProxyYAML) {
		t.Fatalf("expected exact raw proxy yaml in rendered output:\n%s", text)
	}
}

func TestRenderClashSubscriptionFromEntries_RegeneratesHysteria2RawYAMLWithoutFastOpen(t *testing.T) {
	rawProxyYAML := "  - name: hy2-node\n    type: hysteria2\n    server: 1.1.1.1\n    port: 443\n    password: pwd\n    fast-open: true\n"

	rendered, err := renderClashSubscriptionFromEntries(
		[]clashProxyRenderEntry{
			{
				Name: "hy2-node",
				Proxy: map[string]interface{}{
					"name":     "hy2-node",
					"type":     "hysteria2",
					"server":   "1.1.1.1",
					"port":     443,
					"password": "pwd",
				},
				RawYAML: []byte(rawProxyYAML),
			},
		},
		"http://www.gstatic.com/generate_204",
		300,
		50,
		nil,
	)
	if err != nil {
		t.Fatalf("renderClashSubscriptionFromEntries failed: %v", err)
	}

	text := string(rendered)
	if strings.Contains(text, "fast-open") {
		t.Fatalf("expected hysteria2 raw YAML to be regenerated without fast-open:\n%s", text)
	}
}

func TestRenderClashSubscriptionFromEntries_RegeneratesTUICRawYAMLWithoutFastOpen(t *testing.T) {
	rawProxyYAML := "  - name: tuic-node\n    type: tuic\n    server: 1.1.1.1\n    port: 443\n    uuid: 00000000-0000-0000-0000-000000000001\n    password: pwd\n    fast-open: true\n"

	rendered, err := renderClashSubscriptionFromEntries(
		[]clashProxyRenderEntry{
			{
				Name: "tuic-node",
				Proxy: map[string]interface{}{
					"name":     "tuic-node",
					"type":     "tuic",
					"server":   "1.1.1.1",
					"port":     443,
					"uuid":     "00000000-0000-0000-0000-000000000001",
					"password": "pwd",
				},
				RawYAML: []byte(rawProxyYAML),
			},
		},
		"http://www.gstatic.com/generate_204",
		300,
		50,
		nil,
	)
	if err != nil {
		t.Fatalf("renderClashSubscriptionFromEntries failed: %v", err)
	}

	text := string(rendered)
	if strings.Contains(text, "fast-open") {
		t.Fatalf("expected tuic raw YAML to be regenerated without fast-open:\n%s", text)
	}
}
