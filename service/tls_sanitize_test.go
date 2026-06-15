package service

import (
	"encoding/json"
	"testing"

	"github.com/alireza0/s-ui/database/model"
)

func mustTLSRawMessage(t *testing.T, value interface{}) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	return data
}

func mustTLSMap(t *testing.T, raw json.RawMessage) map[string]interface{} {
	t.Helper()
	payload := map[string]interface{}{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	return payload
}

func TestSanitizeStoredTLSRecord_RemovesConflictingVerificationMaterial(t *testing.T) {
	record := &model.Tls{
		Server: mustTLSRawMessage(t, map[string]interface{}{
			"client_certificate":                   []string{"CLIENT-PEM"},
			"client_certificate_path":              "/tmp/client.pem",
			"client_certificate_public_key_sha256": []string{"client-hash"},
		}),
		Client: mustTLSRawMessage(t, map[string]interface{}{
			"certificate":                   []string{"SERVER-PEM"},
			"certificate_path":              "/tmp/server.pem",
			"certificate_public_key_sha256": []string{"server-hash"},
		}),
	}

	if err := sanitizeStoredTLSRecord(record); err != nil {
		t.Fatalf("sanitizeStoredTLSRecord failed: %v", err)
	}

	server := mustTLSMap(t, record.Server)
	if _, exists := server["client_certificate"]; exists {
		t.Fatalf("unexpected client_certificate in sanitized server payload: %#v", server["client_certificate"])
	}
	if _, exists := server["client_certificate_path"]; exists {
		t.Fatalf("unexpected client_certificate_path in sanitized server payload: %#v", server["client_certificate_path"])
	}
	if got, ok := server["client_certificate_public_key_sha256"].([]interface{}); !ok || len(got) != 1 {
		t.Fatalf("expected server hash to remain, got %#v", server["client_certificate_public_key_sha256"])
	}

	client := mustTLSMap(t, record.Client)
	if _, exists := client["certificate"]; exists {
		t.Fatalf("unexpected certificate in sanitized client payload: %#v", client["certificate"])
	}
	if _, exists := client["certificate_path"]; exists {
		t.Fatalf("unexpected certificate_path in sanitized client payload: %#v", client["certificate_path"])
	}
	if got, ok := client["certificate_public_key_sha256"].([]interface{}); !ok || len(got) != 1 {
		t.Fatalf("expected client hash to remain, got %#v", client["certificate_public_key_sha256"])
	}
}
