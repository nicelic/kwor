package service

import "testing"

func TestExtractAcmeVersionFromOutput(t *testing.T) {
	tests := []struct {
		name   string
		raw    string
		expect string
	}{
		{
			name:   "picks semver line instead of homepage",
			raw:    "https://github.com/acmesh-official/acme.sh\nv3.1.3\n",
			expect: "v3.1.3",
		},
		{
			name:   "accepts plain semver line",
			raw:    "3.1.3\n",
			expect: "3.1.3",
		},
		{
			name:   "returns empty when no semver exists",
			raw:    "https://github.com/acmesh-official/acme.sh\nusage: acme.sh\n",
			expect: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := extractAcmeVersionFromOutput(tt.raw); got != tt.expect {
				t.Fatalf("extractAcmeVersionFromOutput() = %q, want %q", got, tt.expect)
			}
		})
	}
}
