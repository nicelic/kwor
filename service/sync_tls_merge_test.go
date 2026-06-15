package service

import (
	"encoding/json"
	"testing"

	"github.com/alireza0/s-ui/database/model"
)

func TestHydrateOutboundTLSFromInboundTLSAddsMissingFields(t *testing.T) {
	outbound := map[string]interface{}{
		"type": "hysteria2",
		"tag":  "test",
		"tls": map[string]interface{}{
			"enabled": true,
		},
	}

	inbound := &model.Inbound{
		TlsId: 1,
		Tls: &model.Tls{
			Server: mustJSON(t, map[string]interface{}{
				"enabled":     true,
				"server_name": "example.com",
				"alpn":        []string{"h2"},
				"min_version": "1.2",
				"max_version": "1.3",
			}),
			Client: mustJSON(t, map[string]interface{}{
				"tls_store": "mozilla",
				"utls": map[string]interface{}{
					"enabled":     true,
					"fingerprint": "firefox",
				},
			}),
		},
	}

	hydrateOutboundTLSFromInboundTLS(outbound, inbound)

	tlsMap, ok := outbound["tls"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected outbound.tls map")
	}
	if got, _ := tlsMap["tls_store"].(string); got != "mozilla" {
		t.Fatalf("expected tls_store=mozilla, got %v", tlsMap["tls_store"])
	}
	if got, _ := tlsMap["server_name"].(string); got != "example.com" {
		t.Fatalf("expected server_name=example.com, got %v", tlsMap["server_name"])
	}
	if got, _ := tlsMap["min_version"].(string); got != "1.2" {
		t.Fatalf("expected min_version=1.2, got %v", tlsMap["min_version"])
	}
	if got, _ := tlsMap["max_version"].(string); got != "1.3" {
		t.Fatalf("expected max_version=1.3, got %v", tlsMap["max_version"])
	}

	utlsMap, ok := tlsMap["utls"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected utls map, got %T", tlsMap["utls"])
	}
	if got, _ := utlsMap["fingerprint"].(string); got != "firefox" {
		t.Fatalf("expected utls.fingerprint=firefox, got %v", utlsMap["fingerprint"])
	}
}

func TestHydrateOutboundTLSFromInboundTLSDoesNotOverrideExisting(t *testing.T) {
	outbound := map[string]interface{}{
		"type": "vless",
		"tag":  "test",
		"tls": map[string]interface{}{
			"tls_store":   "chrome",
			"min_version": "1.0",
		},
	}

	inbound := &model.Inbound{
		TlsId: 1,
		Tls: &model.Tls{
			Server: mustJSON(t, map[string]interface{}{
				"min_version": "1.3",
			}),
			Client: mustJSON(t, map[string]interface{}{
				"tls_store": "mozilla",
			}),
		},
	}

	hydrateOutboundTLSFromInboundTLS(outbound, inbound)

	tlsMap, ok := outbound["tls"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected outbound.tls map")
	}
	if got, _ := tlsMap["tls_store"].(string); got != "chrome" {
		t.Fatalf("expected existing tls_store=chrome to be kept, got %v", tlsMap["tls_store"])
	}
	if got, _ := tlsMap["min_version"].(string); got != "1.0" {
		t.Fatalf("expected existing min_version=1.0 to be kept, got %v", tlsMap["min_version"])
	}
}

func mustJSON(t *testing.T, v interface{}) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	return b
}
