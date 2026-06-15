package util

import "testing"

func TestNormalizeSubscriptionServerHostStripsDecorations(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "ipv4", input: " 1.2.3.4 ", want: "1.2.3.4"},
		{name: "ipv6 brackets", input: " [2001:db8::1] ", want: "2001:db8::1"},
		{name: "domain", input: " 节点.example.com ", want: "节点.example.com"},
		{name: "host with port", input: "example.com:443", want: "example.com"},
		{name: "ipv6 with port", input: "[2001:db8::8]:8443", want: "2001:db8::8"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeSubscriptionServerHost(tt.input); got != tt.want {
				t.Fatalf("NormalizeSubscriptionServerHost(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestFormatSubscriptionLinkHostPort(t *testing.T) {
	tests := []struct {
		name string
		host string
		port int
		want string
	}{
		{name: "ipv4", host: "1.2.3.4", port: 443, want: "1.2.3.4:443"},
		{name: "ipv6", host: "2001:db8::1", port: 443, want: "[2001:db8::1]:443"},
		{name: "domain", host: "节点.example.com", port: 443, want: "节点.example.com:443"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FormatSubscriptionLinkHostPort(tt.host, tt.port); got != tt.want {
				t.Fatalf("FormatSubscriptionLinkHostPort(%q, %d) = %q, want %q", tt.host, tt.port, got, tt.want)
			}
		})
	}
}
