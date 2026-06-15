package model

import (
	"encoding/json"
	"testing"
)

func TestMihomoTlsSanitize_KeepsServerSHA256AndStripsClientSHA256(t *testing.T) {
	raw := []byte(`{
		"id": 1,
		"name": "tls-a",
		"server": {
			"min_version": "1.2",
			"client_authentication": "require-and-verify",
			"client_certificate_path": "/tmp/client.pem",
			"client_certificate_public_key_sha256": ["server-hash"]
		},
		"client": {
			"enabled": true,
			"fingerprint": "AA:BB",
			"include_server_certificate": false,
			"mihomo_use_fingerprint": true,
			"tls_store": "mozilla",
			"certificate_path": "/tmp/cert.pem",
			"certificate_public_key_sha256": ["client-hash"]
		}
	}`)

	var tls MihomoTls
	if err := json.Unmarshal(raw, &tls); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	tls.Sanitize()

	var server map[string]interface{}
	if err := json.Unmarshal(tls.Server, &server); err != nil {
		t.Fatalf("decode server failed: %v", err)
	}
	if _, exists := server["min_version"]; exists {
		t.Fatalf("unexpected min_version in sanitized server payload: %#v", server["min_version"])
	}
	if _, exists := server["client_authentication"]; exists {
		t.Fatalf("unexpected client_authentication in sanitized server payload: %#v", server["client_authentication"])
	}
	if _, exists := server["client_certificate_path"]; exists {
		t.Fatalf("unexpected client_certificate_path in sanitized server payload: %#v", server["client_certificate_path"])
	}
	if _, exists := server["client_certificate_public_key_sha256"]; exists {
		t.Fatalf("unexpected client_certificate_public_key_sha256 in sanitized server payload: %#v", server["client_certificate_public_key_sha256"])
	}

	var client map[string]interface{}
	if err := json.Unmarshal(tls.Client, &client); err != nil {
		t.Fatalf("decode client failed: %v", err)
	}
	for _, key := range []string{"mihomo_use_fingerprint", "tls_store", "certificate_path"} {
		if _, exists := client[key]; exists {
			t.Fatalf("unexpected %s in sanitized client payload: %#v", key, client[key])
		}
	}
	if got, _ := client["fingerprint"].(string); got != "AA:BB" {
		t.Fatalf("expected fingerprint to remain, got %#v", client["fingerprint"])
	}
	if got, ok := client["include_server_certificate"].(bool); !ok || got {
		t.Fatalf("expected include_server_certificate=false to remain, got %#v", client["include_server_certificate"])
	}
	if got, ok := client["certificate_public_key_sha256"].([]interface{}); !ok || len(got) != 1 || got[0] != "client-hash" {
		t.Fatalf("expected certificate_public_key_sha256 to remain, got %#v", client["certificate_public_key_sha256"])
	}
}

func TestMihomoOutboundSanitize_StripsUnsupportedDialFieldsAndAnyTLSReality(t *testing.T) {
	raw := []byte(`{
		"type": "anytls",
		"tag": "node-a",
		"inet4_bind_address": "127.0.0.1",
		"inet6_bind_address": "::1",
		"reuse_addr": true,
		"udp_fragment": true,
		"connect_timeout": "5s",
		"domain_resolver": "dns-out",
		"tls": {
			"enabled": true,
			"utls": {
				"enabled": true,
				"fingerprint": "chrome"
			},
			"reality": {
				"enabled": true,
				"public_key": "pub-key",
				"short_id": "short-id"
			}
		}
	}`)

	var outbound MihomoOutbound
	if err := outbound.UnmarshalJSON(raw); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	encoded, err := outbound.MarshalJSON()
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	for _, key := range []string{
		"inet4_bind_address",
		"inet6_bind_address",
		"reuse_addr",
		"udp_fragment",
		"connect_timeout",
		"domain_resolver",
	} {
		if _, exists := payload[key]; exists {
			t.Fatalf("unexpected %s in sanitized payload: %#v", key, payload[key])
		}
	}

	tlsMap, ok := payload["tls"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected tls map, got %#v", payload["tls"])
	}
	if _, exists := tlsMap["reality"]; exists {
		t.Fatalf("unexpected reality in anytls payload: %#v", tlsMap["reality"])
	}
	if _, exists := tlsMap["utls"]; !exists {
		t.Fatalf("expected utls to remain for anytls: %#v", tlsMap)
	}
}

func TestMihomoOutboundSanitize_StripsUnsupportedUTLSForHysteria2(t *testing.T) {
	raw := []byte(`{
		"type": "hysteria2",
		"tag": "node-a",
		"tls": {
			"enabled": true,
			"utls": {
				"enabled": true,
				"fingerprint": "chrome"
			}
		}
	}`)

	var outbound MihomoOutbound
	if err := outbound.UnmarshalJSON(raw); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	encoded, err := outbound.MarshalJSON()
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(encoded, &payload); err != nil {
		t.Fatalf("decode failed: %v", err)
	}

	tlsMap, ok := payload["tls"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected tls map, got %#v", payload["tls"])
	}
	if _, exists := tlsMap["utls"]; exists {
		t.Fatalf("unexpected utls in hysteria2 payload: %#v", tlsMap["utls"])
	}
}

func TestMihomoOutboundSanitize_StripsUnsupportedGroupHelperFields(t *testing.T) {
	tests := []struct {
		name         string
		raw          []byte
		expectedGone []string
		expectedKeep []string
	}{
		{
			name: "selector strips default helper and unsupported mihomo group fields",
			raw: []byte(`{
				"type": "selector",
				"tag": "group-a",
				"outbounds": ["node-a", "DIRECT"],
				"default": "node-a",
				"url": "https://cp.cloudflare.com/generate_204",
				"interval": "300s",
				"tolerance": 150,
				"idle_timeout": "30m",
				"interrupt_exist_connections": true
			}`),
			expectedGone: []string{"default", "url", "interval", "tolerance", "idle_timeout", "interrupt_exist_connections"},
			expectedKeep: []string{"outbounds"},
		},
		{
			name: "urltest keeps supported probe fields but strips stale helper fields",
			raw: []byte(`{
				"type": "urltest",
				"tag": "group-b",
				"outbounds": ["node-a"],
				"default": "node-a",
				"url": "https://cp.cloudflare.com/generate_204",
				"interval": "300s",
				"tolerance": 150,
				"idle_timeout": "30m",
				"interrupt_exist_connections": true
			}`),
			expectedGone: []string{"default", "idle_timeout", "interrupt_exist_connections"},
			expectedKeep: []string{"url", "interval", "tolerance", "outbounds"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var outbound MihomoOutbound
			if err := outbound.UnmarshalJSON(tt.raw); err != nil {
				t.Fatalf("unmarshal failed: %v", err)
			}

			encoded, err := outbound.MarshalJSON()
			if err != nil {
				t.Fatalf("marshal failed: %v", err)
			}

			var payload map[string]interface{}
			if err := json.Unmarshal(encoded, &payload); err != nil {
				t.Fatalf("decode failed: %v", err)
			}

			for _, key := range tt.expectedGone {
				if _, exists := payload[key]; exists {
					t.Fatalf("unexpected %s in sanitized payload: %#v", key, payload[key])
				}
			}
			for _, key := range tt.expectedKeep {
				if _, exists := payload[key]; !exists {
					t.Fatalf("expected %s to remain in sanitized payload: %#v", key, payload)
				}
			}
		})
	}
}
