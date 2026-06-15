package service

import "testing"

func TestExtractClashProxyRawYAMLByNamePreservesExactSnippet(t *testing.T) {
	yamlData := []byte("proxies:\n  -   name: raw-a\n      type: trojan\n      password: \"a,b\"\n  - name: raw-b\n    type: ss\nother: value\n")

	rawByName, err := extractClashProxyRawYAMLByName(yamlData)
	if err != nil {
		t.Fatalf("extractClashProxyRawYAMLByName failed: %v", err)
	}

	expected := "  -   name: raw-a\n      type: trojan\n      password: \"a,b\"\n"
	if got := string(rawByName["raw-a"]); got != expected {
		t.Fatalf("expected exact raw yaml chunk %q, got %q", expected, got)
	}
}
