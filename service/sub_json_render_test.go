package service

import (
	"encoding/json"
	"testing"

	"github.com/alireza0/s-ui/database/model"
)

func TestRenderManagedSingboxSubscriptionJSON_StripsMihomoFieldsAndNormalizesLatency(t *testing.T) {
	result, err := renderManagedSingboxSubscriptionJSON(
		[]map[string]interface{}{
			{
				"type":        "vmess",
				"tag":         "node-a",
				"server":      "1.2.3.4",
				"server_port": 443,
				"uuid":        "u-1",
				"security":    "auto",
				"fast_open":   true,
				"mihomo_common": map[string]interface{}{
					"test": true,
				},
				"tls": map[string]interface{}{
					"enabled":                true,
					"server_name":            "edge.example.com",
					"tls_store":              "panel",
					"fingerprint":            "legacy",
					"mihomo_use_fingerprint": true,
				},
			},
		},
		`{"latency_test_interval":" 15M ","latency_test_url":"https://example.com/generate_204","latency_tolerance":80}`,
		func(store string) string { return store },
	)
	if err != nil {
		t.Fatalf("renderManagedSingboxSubscriptionJSON returned error: %v", err)
	}

	var doc map[string]interface{}
	if err := json.Unmarshal(result, &doc); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	certificate, ok := doc["certificate"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected certificate block, got %#v", doc["certificate"])
	}
	if got, _ := certificate["store"].(string); got != "panel" {
		t.Fatalf("expected certificate.store panel, got %#v", certificate["store"])
	}

	outbounds, ok := doc["outbounds"].([]interface{})
	if !ok {
		t.Fatalf("expected outbounds array, got %#v", doc["outbounds"])
	}

	var node map[string]interface{}
	var auto map[string]interface{}
	for _, raw := range outbounds {
		outbound, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		tag, _ := outbound["tag"].(string)
		switch tag {
		case "node-a":
			node = outbound
		case autoSelectorTag:
			auto = outbound
		}
	}

	if node == nil {
		t.Fatal("expected rendered node outbound")
	}
	if _, ok := node["mihomo_common"]; ok {
		t.Fatalf("expected mihomo_common to be removed, got %#v", node["mihomo_common"])
	}
	if _, ok := node["fast_open"]; ok {
		t.Fatalf("expected fast_open to be removed, got %#v", node["fast_open"])
	}

	tlsMap, ok := node["tls"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected tls block, got %#v", node["tls"])
	}
	if _, ok := tlsMap["tls_store"]; ok {
		t.Fatalf("expected tls_store to be moved to certificate.store, got %#v", tlsMap["tls_store"])
	}
	if _, ok := tlsMap["fingerprint"]; ok {
		t.Fatalf("expected mihomo fingerprint to be removed, got %#v", tlsMap["fingerprint"])
	}
	if _, ok := tlsMap["mihomo_use_fingerprint"]; ok {
		t.Fatalf("expected mihomo_use_fingerprint to be removed, got %#v", tlsMap["mihomo_use_fingerprint"])
	}

	if auto == nil {
		t.Fatal("expected auto selector outbound")
	}
	if got, _ := auto["interval"].(string); got != "15m" {
		t.Fatalf("expected normalized latency interval 15m, got %#v", auto["interval"])
	}
	if got, _ := auto["url"].(string); got != "https://example.com/generate_204" {
		t.Fatalf("expected latency url override, got %#v", auto["url"])
	}
}

func TestRefreshManagedSubscriptionOutboundTLS_DisabledServerCertificate_RemovesCertificateAndSHA256(t *testing.T) {
	outbound := map[string]interface{}{
		"type": "trojan",
		"tag":  "managed-disabled-node",
		"tls": map[string]interface{}{
			"enabled":                       true,
			"server_name":                   "edge.example.com",
			"certificate_public_key_sha256": []interface{}{"legacy-hash"},
			"certificate":                   []interface{}{"legacy-cert"},
			"fingerprint":                   "AA:BB:CC",
		},
	}
	tlsConfig := &model.Tls{
		Server: mustJSONRaw(t, map[string]interface{}{
			"enabled":     true,
			"server_name": "edge.example.com",
			"certificate": []string{
				"-----BEGIN CERTIFICATE-----",
				"INVALID",
				"-----END CERTIFICATE-----",
			},
		}),
		Client: mustJSONRaw(t, map[string]interface{}{
			"include_server_certificate":    false,
			"certificate_public_key_sha256": []string{"configured"},
		}),
	}

	refreshManagedSubscriptionOutboundTLS(outbound, tlsConfig)

	tlsMap, ok := outbound["tls"].(map[string]interface{})
	if !ok || tlsMap == nil {
		t.Fatalf("expected tls map, got %#v", outbound["tls"])
	}
	if _, exists := tlsMap["certificate"]; exists {
		t.Fatalf("expected certificate to be removed when include_server_certificate is false, got %#v", tlsMap["certificate"])
	}
	if _, exists := tlsMap["certificate_public_key_sha256"]; exists {
		t.Fatalf("expected certificate_public_key_sha256 to be removed when include_server_certificate is false, got %#v", tlsMap["certificate_public_key_sha256"])
	}
	if _, exists := tlsMap["fingerprint"]; exists {
		t.Fatalf("expected fingerprint to be removed when include_server_certificate is false, got %#v", tlsMap["fingerprint"])
	}
}

func TestRefreshManagedSubscriptionOutboundTLS_PreservesSHA256WhenServerCertMissing(t *testing.T) {
	tests := []struct {
		name           string
		clientHashes   interface{}
		outboundHashes interface{}
		want           string
	}{
		{
			name:         "configured client hash wins",
			clientHashes: []string{"configured-hash"},
			outboundHashes: []interface{}{
				"stored-hash",
			},
			want: "configured-hash",
		},
		{
			name:           "stored outbound hash is kept",
			clientHashes:   nil,
			outboundHashes: []interface{}{"stored-hash"},
			want:           "stored-hash",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outbound := map[string]interface{}{
				"type": "trojan",
				"tag":  "managed-sha-node",
				"tls": map[string]interface{}{
					"enabled":                       true,
					"server_name":                   "edge.example.com",
					"certificate_public_key_sha256": tt.outboundHashes,
					"certificate":                   []interface{}{"legacy-cert"},
				},
			}
			clientTLS := map[string]interface{}{
				"include_server_certificate": true,
			}
			if tt.clientHashes != nil {
				clientTLS["certificate_public_key_sha256"] = tt.clientHashes
			}
			tlsConfig := &model.Tls{
				Server: mustJSONRaw(t, map[string]interface{}{
					"enabled":     true,
					"server_name": "edge.example.com",
				}),
				Client: mustJSONRaw(t, clientTLS),
			}

			refreshManagedSubscriptionOutboundTLS(outbound, tlsConfig)

			tlsMap, ok := outbound["tls"].(map[string]interface{})
			if !ok || tlsMap == nil {
				t.Fatalf("expected tls map, got %#v", outbound["tls"])
			}
			hashes := stringSliceForSyncTest(t, tlsMap["certificate_public_key_sha256"])
			if len(hashes) != 1 || hashes[0] != tt.want {
				t.Fatalf("expected certificate_public_key_sha256 %q, got %#v", tt.want, tlsMap["certificate_public_key_sha256"])
			}
			if _, exists := tlsMap["certificate"]; exists {
				t.Fatalf("expected PEM certificate to be removed when SHA256 mode is kept, got %#v", tlsMap["certificate"])
			}
		})
	}
}

func TestRefreshManagedSubscriptionOutboundTLS_DisabledServerFingerprint_RemovesFingerprint(t *testing.T) {
	outbound := map[string]interface{}{
		"type": "trojan",
		"tag":  "managed-disabled-fingerprint-node",
		"tls": map[string]interface{}{
			"enabled":     true,
			"server_name": "edge.example.com",
			"fingerprint": "AA:BB:CC",
		},
	}
	tlsConfig := &model.Tls{
		Server: mustJSONRaw(t, map[string]interface{}{
			"enabled":     true,
			"server_name": "edge.example.com",
			"certificate": []string{
				"-----BEGIN CERTIFICATE-----",
				"INVALID",
				"-----END CERTIFICATE-----",
			},
		}),
		Client: mustJSONRaw(t, map[string]interface{}{
			"include_server_certificate": true,
			"include_server_fingerprint": false,
		}),
	}

	refreshManagedSubscriptionOutboundTLS(outbound, tlsConfig)

	tlsMap, ok := outbound["tls"].(map[string]interface{})
	if !ok || tlsMap == nil {
		t.Fatalf("expected tls map, got %#v", outbound["tls"])
	}
	if _, exists := tlsMap["fingerprint"]; exists {
		t.Fatalf("expected fingerprint to be removed when include_server_fingerprint is false, got %#v", tlsMap["fingerprint"])
	}
}

func TestShouldFallbackRefreshManagedSubOutboundTLS(t *testing.T) {
	if !shouldFallbackRefreshManagedSubOutboundTLS(nil) {
		t.Fatal("expected nil suboutbound to allow fallback refresh")
	}
	if !shouldFallbackRefreshManagedSubOutboundTLS(&model.SubOutbound{}) {
		t.Fatal("expected manual suboutbound without source type to allow fallback refresh")
	}
	if shouldFallbackRefreshManagedSubOutboundTLS(&model.SubOutbound{SourceType: "subgroup"}) {
		t.Fatal("expected sourced suboutbound to skip fallback refresh")
	}
	if shouldFallbackRefreshManagedSubOutboundTLS(&model.SubOutbound{SourceType: "client"}) {
		t.Fatal("expected client-sourced suboutbound to skip fallback refresh")
	}
}
