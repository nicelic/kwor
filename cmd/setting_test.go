package cmd

import (
	"strings"
	"testing"
)

func TestParsePublicIPResponse(t *testing.T) {
	tests := []struct {
		name    string
		body    string
		want    string
		wantErr bool
	}{
		{name: "IPv4", body: " 203.0.113.8\n", want: "203.0.113.8"},
		{name: "IPv6", body: "2001:db8::8", want: "2001:db8::8"},
		{name: "HTML error page", body: "<html>error</html>", wantErr: true},
		{name: "oversized body", body: strings.Repeat("x", int(publicIPResponseMaxBytes)+1), wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parsePublicIPResponse([]byte(tt.body))
			if (err != nil) != tt.wantErr {
				t.Fatalf("parsePublicIPResponse() error = %v, wantErr %v", err, tt.wantErr)
			}
			if got != tt.want {
				t.Fatalf("parsePublicIPResponse() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestHasInterfaceFlag(t *testing.T) {
	flags := []string{"broadcast", "multicast", "up"}
	if !hasInterfaceFlag(flags, "up") {
		t.Fatal("hasInterfaceFlag() did not find a flag outside the old positional assumptions")
	}
	if hasInterfaceFlag(flags, "loopback") {
		t.Fatal("hasInterfaceFlag() reported a missing flag")
	}
}
