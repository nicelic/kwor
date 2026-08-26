package service

import (
	"encoding/json"
	"testing"
)

func TestNormalizeSingboxRuntimeOutbounds_StripsUnsupportedFieldsAndExtractsStore(t *testing.T) {
	raw := []json.RawMessage{
		json.RawMessage(`{
			"type":"vmess",
			"tag":"node-a",
			"server":"1.2.3.4",
			"server_port":443,
			"uuid":"u-1",
			"username":"client",
			"name":"legacy",
			"fast_open":true,
			"mihomo_common":{"udp":true},
			"mihomo_hy2":{"initial_stream_receive_window":1},
			"mihomo_fast_open":true,
			"tls":{
				"enabled":true,
				"tls_store":"mozilla",
				"fingerprint":"AA:BB",
				"mihomo_use_fingerprint":true
			}
		}`),
	}

	normalized, store, err := normalizeSingboxRuntimeOutbounds(raw)
	if err != nil {
		t.Fatalf("normalizeSingboxRuntimeOutbounds returned error: %v", err)
	}
	if store != "mozilla" {
		t.Fatalf("expected extracted store mozilla, got %q", store)
	}
	if len(normalized) != 1 {
		t.Fatalf("expected 1 outbound, got %d", len(normalized))
	}

	outbound := map[string]interface{}{}
	if err := json.Unmarshal(normalized[0], &outbound); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}

	for _, key := range []string{"username", "name", "fast_open", "mihomo_common", "mihomo_hy2", "mihomo_fast_open"} {
		if _, exists := outbound[key]; exists {
			t.Fatalf("expected %s to be removed, got %#v", key, outbound[key])
		}
	}

	tlsMap, ok := outbound["tls"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected tls block, got %#v", outbound["tls"])
	}
	for _, key := range []string{"tls_store", "store", "fingerprint", "mihomo_use_fingerprint"} {
		if _, exists := tlsMap[key]; exists {
			t.Fatalf("expected tls.%s to be removed, got %#v", key, tlsMap[key])
		}
	}
}

func TestNormalizeSingboxRuntimeOutboundsRemovesMihomoHysteria2ReceiveWindows(t *testing.T) {
	raw := []json.RawMessage{json.RawMessage(`{
		"type":"hysteria2",
		"tag":"hy2-runtime",
		"initial_stream_receive_window":1,
		"max_stream_receive_window":2,
		"initial_connection_receive_window":3,
		"max_connection_receive_window":4,
		"mihomo_hy2":{
			"initial_stream_receive_window":38000000,
			"max_stream_receive_window":70000000,
			"initial_connection_receive_window":120000000,
			"max_connection_receive_window":150000000
		},
		"mihomo_fast_open":true
	}`)}

	normalized, _, err := normalizeSingboxRuntimeOutbounds(raw)
	if err != nil {
		t.Fatalf("normalizeSingboxRuntimeOutbounds returned error: %v", err)
	}
	if len(normalized) != 1 {
		t.Fatalf("expected one normalized outbound, got %d", len(normalized))
	}
	outbound := map[string]interface{}{}
	if err := json.Unmarshal(normalized[0], &outbound); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	for _, key := range []string{
		"initial_stream_receive_window",
		"max_stream_receive_window",
		"initial_connection_receive_window",
		"max_connection_receive_window",
	} {
		if _, exists := outbound[key]; exists {
			t.Fatalf("Mihomo-only receive window %q survived runtime sanitization: %#v", key, outbound)
		}
	}
	for _, key := range []string{"mihomo_hy2", "mihomo_fast_open", "fast_open", "fast-open"} {
		if _, exists := outbound[key]; exists {
			t.Fatalf("Mihomo-only key %q survived runtime sanitization: %#v", key, outbound)
		}
	}
}
