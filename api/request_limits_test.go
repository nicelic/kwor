package api

import (
	"strings"
	"testing"

	"github.com/alireza0/s-ui/database"
)

func TestAPIRequestBodyLimit(t *testing.T) {
	tests := []struct {
		action string
		want   int64
	}{
		{action: "save", want: apiSettingsRequestMaxBytes},
		{action: "settings-patch", want: apiSettingsRequestMaxBytes},
		{action: "singbox-runtime-retry", want: apiSingboxRuntimeRetryRequestMaxBytes},
		{action: "tlsSha256", want: apiTLSCertificateRequestMaxBytes},
		{action: "tlsFingerprint", want: apiTLSCertificateRequestMaxBytes},
		{action: "tlsCertAlgorithm", want: apiTLSCertificateRequestMaxBytes},
		{action: "tlsCertificateInfo", want: apiTLSCertificateRequestMaxBytes},
		{action: "tlsSelfSignedTemplate", want: apiTLSCertificateRequestMaxBytes},
		{action: "mihomo-dns-save", want: 64 * 1024},
		{action: "singbox-dns-save", want: 256 * 1024},
		{action: "singbox-basics-save", want: 256 * 1024},
		{action: "singbox-route-save", want: 2 * 1024 * 1024},
		{action: "mihomo-route-save", want: 2 * 1024 * 1024},
		{action: "system-log-optimization-content", want: 256 * 1024},
		{action: "system-sysctl-optimization-content", want: 256 * 1024},
		{action: "system-linux-dns-optimization-content", want: 256 * 1024},
		{action: "system-linux-dns-optimization-nameservers", want: 16 * 1024},
		{action: "importdb", want: database.MaxDatabaseImportUploadBytes()},
		{action: "restore-db-backup", want: database.MaxDBBackupArchiveUploadBytes()},
	}

	for _, tt := range tests {
		if got := apiRequestBodyLimit(tt.action); got != tt.want {
			t.Fatalf("apiRequestBodyLimit(%q) = %d, want %d", tt.action, got, tt.want)
		}
	}
}

func TestDefaultAPIRequestBodyLimitIsInclusive32MiB(t *testing.T) {
	const want int64 = 32 * 1024 * 1024
	for _, action := range []string{"port-forward-rule", "unrecognized-action"} {
		if got := apiRequestBodyLimit(action); got != want {
			t.Fatalf("apiRequestBodyLimit(%q) = %d, want %d", action, got, want)
		}
	}
}

func TestValidateSingboxRuntimeRetryPayload(t *testing.T) {
	for _, tt := range []struct {
		name    string
		payload string
		wantErr bool
	}{
		{name: "empty", payload: "", wantErr: false},
		{name: "empty object", payload: " {} ", wantErr: false},
		{name: "mutation payload", payload: `{"object":"outbounds"}`, wantErr: true},
		{name: "array", payload: "[]", wantErr: true},
		{name: "oversized", payload: strings.Repeat("x", int(apiSingboxRuntimeRetryRequestMaxBytes)+1), wantErr: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			err := validateSingboxRuntimeRetryPayload(strings.NewReader(tt.payload))
			if (err != nil) != tt.wantErr {
				t.Fatalf("validateSingboxRuntimeRetryPayload(%q) error = %v, wantErr=%v", tt.payload, err, tt.wantErr)
			}
		})
	}
}
