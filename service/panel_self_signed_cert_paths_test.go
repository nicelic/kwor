package service

import (
	"testing"
	"time"
)

func TestNormalizePanelSelfSignedName(t *testing.T) {
	testCases := []struct {
		name     string
		input    string
		expected string
		ok       bool
	}{
		{
			name:     "domain",
			input:    "Example.COM",
			expected: "example.com",
			ok:       true,
		},
		{
			name:     "domain with port",
			input:    "example.com:443",
			expected: "example.com",
			ok:       true,
		},
		{
			name:     "ipv4",
			input:    "203.0.113.9",
			expected: "203.0.113.9",
			ok:       true,
		},
		{
			name:  "ipv6 must fallback to timestamp",
			input: "2001:db8::1",
			ok:    false,
		},
		{
			name:  "wildcard host is not usable sni",
			input: "0.0.0.0",
			ok:    false,
		},
		{
			name:  "empty",
			input: " ",
			ok:    false,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			got, ok := normalizePanelSelfSignedName(testCase.input)
			if ok != testCase.ok {
				t.Fatalf("unexpected ok: got=%v want=%v", ok, testCase.ok)
			}
			if got != testCase.expected {
				t.Fatalf("unexpected normalized value: got=%q want=%q", got, testCase.expected)
			}
		})
	}
}

func TestPanelSelfSignedTimestamp(t *testing.T) {
	now := time.Date(2026, 3, 4, 11, 22, 33, 0, time.UTC)
	got := panelSelfSignedTimestamp(now)
	expected := "2026-03-04-11-22-33"
	if got != expected {
		t.Fatalf("unexpected timestamp: got=%q want=%q", got, expected)
	}
}
