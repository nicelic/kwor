package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadTLSPathMaterialRejectsOversizedFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "oversized.pem")
	content := strings.Repeat("x", int(maxTLSPathMaterialBytes)+1)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write oversized TLS material: %v", err)
	}

	if _, _, err := readTLSPathMaterial(path); err == nil {
		t.Fatal("expected oversized TLS material to be rejected")
	}
}
