package service

import (
	"strings"
	"testing"
)

func TestValidateCertificateMaterialTotalUses512MiBLimit(t *testing.T) {
	const wantLimit = 512 * 1024 * 1024
	if certificateMaterialMaxBytes != wantLimit {
		t.Fatalf("certificate material limit = %d, want %d", certificateMaterialMaxBytes, wantLimit)
	}

	if err := validateCertificateMaterialTotal(certificateMaterialMaxBytes-1, 1); err != nil {
		t.Fatalf("material total at the 512 MiB boundary should pass: %v", err)
	}
	if err := validateCertificateMaterialTotal(certificateMaterialMaxBytes); err != nil {
		t.Fatalf("material total exactly at the 512 MiB boundary should pass: %v", err)
	}

	err := validateCertificateMaterialTotal(certificateMaterialMaxBytes, 1)
	if err == nil || !strings.Contains(err.Error(), "512 MiB") {
		t.Fatalf("material total above 512 MiB should be rejected with the new limit, got %v", err)
	}
}
