package util

import "testing"

func TestNormalizeMieruPortRange(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
		ok    bool
	}{
		{name: "dash", input: "400-500", want: "400-500", ok: true},
		{name: "colon", input: "400:500", want: "400-500", ok: true},
		{name: "fullwidth colon", input: "400：500", want: "400-500", ok: true},
		{name: "single port rejected", input: "400", want: "", ok: false},
		{name: "equal range rejected", input: "400-400", want: "", ok: false},
		{name: "invalid text", input: "abc", want: "", ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := NormalizeMieruPortRange(tt.input)
			if ok != tt.ok {
				t.Fatalf("NormalizeMieruPortRange(%q) ok=%v, want %v", tt.input, ok, tt.ok)
			}
			if got != tt.want {
				t.Fatalf("NormalizeMieruPortRange(%q)=%q, want %q", tt.input, got, tt.want)
			}
		})
	}
}
