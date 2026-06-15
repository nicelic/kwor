package service

import (
	"encoding/json"
	"testing"
)

func TestStripOutboundsTLSStore(t *testing.T) {
	raw := []json.RawMessage{
		json.RawMessage(`{"type":"hysteria","tag":"a","tls":{"enabled":true,"tls_store":"mozilla"}}`),
		json.RawMessage(`{"type":"hysteria2","tag":"b","tls":{"enabled":true,"store":"chrome"}}`),
	}

	normalized, err := stripOutboundsTLSStore(raw)
	if err != nil {
		t.Fatalf("stripOutboundsTLSStore error: %v", err)
	}
	if len(normalized) != 2 {
		t.Fatalf("expected 2 outbounds, got %d", len(normalized))
	}

	for i, item := range normalized {
		var m map[string]interface{}
		if err := json.Unmarshal(item, &m); err != nil {
			t.Fatalf("unmarshal normalized[%d] failed: %v", i, err)
		}
		tlsMap, ok := m["tls"].(map[string]interface{})
		if !ok {
			t.Fatalf("normalized[%d] missing tls map", i)
		}
		if _, ok := tlsMap["tls_store"]; ok {
			t.Fatalf("normalized[%d] still has tls_store", i)
		}
		if _, ok := tlsMap["store"]; ok {
			t.Fatalf("normalized[%d] still has tls.store", i)
		}
	}
}

func TestSanitizeShadowTLSOutboundJSONRemovesInboundOnlyFields(t *testing.T) {
	raw := []byte(`{
		"type":"shadowtls",
		"tag":"stls",
		"server":"1.2.3.4",
		"server_port":443,
		"version":3,
		"wildcard_sni":"all",
		"strict_mode":true,
		"handshake":{"server":"addons.mozilla.org","server_port":443},
		"handshake_for_server_name":{"example.com":{"server":"addons.mozilla.org","server_port":443}}
	}`)

	sanitized, err := sanitizeShadowTLSOutboundJSON(raw)
	if err != nil {
		t.Fatalf("sanitizeShadowTLSOutboundJSON error: %v", err)
	}

	outbound := map[string]interface{}{}
	if err := json.Unmarshal(sanitized, &outbound); err != nil {
		t.Fatalf("unmarshal sanitized failed: %v", err)
	}

	if _, ok := outbound["wildcard_sni"]; ok {
		t.Fatalf("sanitized outbound should not contain wildcard_sni: %#v", outbound)
	}
	if _, ok := outbound["strict_mode"]; ok {
		t.Fatalf("sanitized outbound should not contain strict_mode: %#v", outbound)
	}
	if _, ok := outbound["handshake"]; ok {
		t.Fatalf("sanitized outbound should not contain handshake: %#v", outbound)
	}
	if _, ok := outbound["handshake_for_server_name"]; ok {
		t.Fatalf("sanitized outbound should not contain handshake_for_server_name: %#v", outbound)
	}
	if outbound["type"] != "shadowtls" || outbound["tag"] != "stls" {
		t.Fatalf("sanitized outbound lost base fields: %#v", outbound)
	}
}
