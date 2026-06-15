package model

import (
	"encoding/json"
	"testing"
)

func inboundTestAsFloat64(t *testing.T, value interface{}) float64 {
	t.Helper()
	switch v := value.(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int:
		return float64(v)
	case int64:
		return float64(v)
	default:
		t.Fatalf("unexpected numeric type: %T (%#v)", value, value)
		return 0
	}
}

func mustRawMessage(t *testing.T, value interface{}) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	return data
}

func mustInboundTLSMap(t *testing.T, raw []byte) map[string]interface{} {
	t.Helper()
	payload := map[string]interface{}{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	tlsRaw, ok := payload["tls"]
	if !ok {
		t.Fatalf("inbound tls is missing")
	}
	tlsMap, ok := tlsRaw.(map[string]interface{})
	if !ok {
		t.Fatalf("unexpected inbound tls type: %T", tlsRaw)
	}
	return tlsMap
}

func TestInboundMarshalJSON_DoesNotAutoCopyServerCertificate(t *testing.T) {
	inbound := Inbound{
		Type: "hysteria2",
		Tag:  "hy1",
		Tls: &Tls{
			Server: mustRawMessage(t, map[string]interface{}{
				"enabled":               true,
				"client_authentication": "require-and-verify",
				"certificate":           []string{"-----BEGIN CERTIFICATE-----", "SERVER", "-----END CERTIFICATE-----"},
			}),
		},
	}

	raw, err := inbound.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON failed: %v", err)
	}

	tlsMap := mustInboundTLSMap(t, raw)
	if _, exists := tlsMap["client_certificate"]; exists {
		t.Fatalf("unexpected client_certificate in inbound tls: %#v", tlsMap["client_certificate"])
	}
	if _, exists := tlsMap["client_certificate_path"]; exists {
		t.Fatalf("unexpected client_certificate_path in inbound tls: %#v", tlsMap["client_certificate_path"])
	}
}

func TestInboundMarshalJSON_RemovesClientCertificateWhenSHA256Present(t *testing.T) {
	inbound := Inbound{
		Type: "hysteria2",
		Tag:  "hy1",
		Tls: &Tls{
			Server: mustRawMessage(t, map[string]interface{}{
				"enabled":                              true,
				"client_authentication":                "require-and-verify",
				"client_certificate":                   []string{"-----BEGIN CERTIFICATE-----", "CLIENT", "-----END CERTIFICATE-----"},
				"client_certificate_public_key_sha256": []string{"hash-a"},
			}),
		},
	}

	raw, err := inbound.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON failed: %v", err)
	}

	tlsMap := mustInboundTLSMap(t, raw)
	if _, exists := tlsMap["client_certificate"]; exists {
		t.Fatalf("unexpected client_certificate in inbound tls: %#v", tlsMap["client_certificate"])
	}
	if got, ok := tlsMap["client_certificate_public_key_sha256"].([]interface{}); !ok || len(got) != 1 {
		t.Fatalf("expected client_certificate_public_key_sha256 to remain, got %#v", tlsMap["client_certificate_public_key_sha256"])
	}
}

func TestInboundMarshalJSON_ExcludesPortHopRuntimeOnlyFields(t *testing.T) {
	inbound := Inbound{
		Type: "hysteria2",
		Tag:  "hy2",
		Options: mustRawMessage(t, map[string]interface{}{
			"listen":                "::",
			"listen_port":           24443,
			"port_hop_range":        "20000:21000",
			"port_hop_interval":     "15s",
			"port_hop_interval_max": "45s",
		}),
	}

	raw, err := inbound.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON failed: %v", err)
	}

	payload := map[string]interface{}{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if _, exists := payload["port_hop_range"]; exists {
		t.Fatalf("expected port_hop_range to be excluded from runtime payload, got %#v", payload["port_hop_range"])
	}
	if _, exists := payload["port_hop_interval"]; exists {
		t.Fatalf("expected port_hop_interval to be excluded from runtime payload, got %#v", payload["port_hop_interval"])
	}
	if _, exists := payload["port_hop_interval_max"]; exists {
		t.Fatalf("expected port_hop_interval_max to be excluded from runtime payload, got %#v", payload["port_hop_interval_max"])
	}
}

func TestInboundUnmarshalJSON_NormalizesLegacyHysteriaQUICFields(t *testing.T) {
	var inbound Inbound
	if err := json.Unmarshal([]byte(`{
		"type": "hysteria",
		"tag": "hy1",
		"recv_window_conn": 25000000,
		"recv_window_client": 99000000,
		"max_conn_client": 1024,
		"disable_mtu_discovery": true
	}`), &inbound); err != nil {
		t.Fatalf("UnmarshalJSON failed: %v", err)
	}

	options := map[string]interface{}{}
	if err := json.Unmarshal(inbound.Options, &options); err != nil {
		t.Fatalf("unmarshal options failed: %v", err)
	}

	if got := options["stream_receive_window"]; got != float64(25000000) {
		t.Fatalf("expected stream_receive_window to be normalized, got %#v", got)
	}
	if got := options["connection_receive_window"]; got != float64(99000000) {
		t.Fatalf("expected connection_receive_window to be normalized, got %#v", got)
	}
	if got := options["max_concurrent_streams"]; got != float64(1024) {
		t.Fatalf("expected max_concurrent_streams to be normalized, got %#v", got)
	}
	if got, _ := options["disable_path_mtu_discovery"].(bool); !got {
		t.Fatalf("expected disable_path_mtu_discovery=true, got %#v", options["disable_path_mtu_discovery"])
	}
	if _, exists := options["recv_window_conn"]; exists {
		t.Fatalf("legacy recv_window_conn should be removed, got %#v", options["recv_window_conn"])
	}
	if _, exists := options["recv_window_client"]; exists {
		t.Fatalf("legacy recv_window_client should be removed, got %#v", options["recv_window_client"])
	}
	if _, exists := options["max_conn_client"]; exists {
		t.Fatalf("legacy max_conn_client should be removed, got %#v", options["max_conn_client"])
	}
	if _, exists := options["disable_mtu_discovery"]; exists {
		t.Fatalf("legacy disable_mtu_discovery should be removed, got %#v", options["disable_mtu_discovery"])
	}
}

func TestInboundMarshalJSON_NormalizesLegacyHysteriaQUICFields(t *testing.T) {
	inbound := Inbound{
		Type: "hysteria",
		Tag:  "hy1",
		Options: mustRawMessage(t, map[string]interface{}{
			"recv_window_conn":      25000000,
			"recv_window_client":    99000000,
			"max_conn_client":       1024,
			"disable_mtu_discovery": true,
		}),
	}

	raw, err := inbound.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON failed: %v", err)
	}

	payload := map[string]interface{}{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if got := payload["stream_receive_window"]; got != float64(25000000) {
		t.Fatalf("expected stream_receive_window in runtime payload, got %#v", got)
	}
	if got := payload["connection_receive_window"]; got != float64(99000000) {
		t.Fatalf("expected connection_receive_window in runtime payload, got %#v", got)
	}
	if got := payload["max_concurrent_streams"]; got != float64(1024) {
		t.Fatalf("expected max_concurrent_streams in runtime payload, got %#v", got)
	}
	if got, _ := payload["disable_path_mtu_discovery"].(bool); !got {
		t.Fatalf("expected disable_path_mtu_discovery=true, got %#v", payload["disable_path_mtu_discovery"])
	}
	if _, exists := payload["recv_window_conn"]; exists {
		t.Fatalf("legacy recv_window_conn should be removed from runtime payload, got %#v", payload["recv_window_conn"])
	}
	if _, exists := payload["recv_window_client"]; exists {
		t.Fatalf("legacy recv_window_client should be removed from runtime payload, got %#v", payload["recv_window_client"])
	}
	if _, exists := payload["max_conn_client"]; exists {
		t.Fatalf("legacy max_conn_client should be removed from runtime payload, got %#v", payload["max_conn_client"])
	}
	if _, exists := payload["disable_mtu_discovery"]; exists {
		t.Fatalf("legacy disable_mtu_discovery should be removed from runtime payload, got %#v", payload["disable_mtu_discovery"])
	}
}

func TestInboundUnmarshalJSON_NormalizesServerBandwidthForHysteria(t *testing.T) {
	var inbound Inbound
	if err := json.Unmarshal([]byte(`{
		"type": "hysteria",
		"tag": "hy1",
		"server_up_mbps": 0,
		"server_down_mbps": "",
		"listen_port": 443
	}`), &inbound); err != nil {
		t.Fatalf("UnmarshalJSON failed: %v", err)
	}

	options := map[string]interface{}{}
	if err := json.Unmarshal(inbound.Options, &options); err != nil {
		t.Fatalf("unmarshal options failed: %v", err)
	}

	if got := options["server_up_mbps"]; got != float64(0) {
		t.Fatalf("expected server_up_mbps to preserve zero after unmarshal, got %#v", got)
	}
	if got := options["server_down_mbps"]; got != float64(2000) {
		t.Fatalf("expected empty server_down_mbps to default to 2000, got %#v", got)
	}
	if _, exists := options["up_mbps"]; exists {
		t.Fatalf("expected legacy up_mbps to be removed after unmarshal, got %#v", options["up_mbps"])
	}
	if _, exists := options["down_mbps"]; exists {
		t.Fatalf("expected legacy down_mbps to be removed after unmarshal, got %#v", options["down_mbps"])
	}
	if got := options["listen_port"]; got != float64(443) {
		t.Fatalf("expected listen_port to remain, got %#v", got)
	}
}

func TestInboundUnmarshalJSON_OmitsEmptyServerBandwidthForHysteria2(t *testing.T) {
	var inbound Inbound
	if err := json.Unmarshal([]byte(`{
		"type": "hysteria2",
		"tag": "hy2",
		"server_up_mbps": 0,
		"server_down_mbps": "",
		"listen_port": 443
	}`), &inbound); err != nil {
		t.Fatalf("UnmarshalJSON failed: %v", err)
	}

	options := map[string]interface{}{}
	if err := json.Unmarshal(inbound.Options, &options); err != nil {
		t.Fatalf("unmarshal options failed: %v", err)
	}

	if got := options["server_up_mbps"]; got != float64(0) {
		t.Fatalf("expected server_up_mbps to preserve zero after unmarshal, got %#v", got)
	}
	if _, exists := options["server_down_mbps"]; exists {
		t.Fatalf("expected empty server_down_mbps to be omitted, got %#v", options["server_down_mbps"])
	}
	if _, exists := options["up_mbps"]; exists {
		t.Fatalf("expected legacy up_mbps to be removed after unmarshal, got %#v", options["up_mbps"])
	}
	if _, exists := options["down_mbps"]; exists {
		t.Fatalf("expected legacy down_mbps to be removed after unmarshal, got %#v", options["down_mbps"])
	}
	if got := options["listen_port"]; got != float64(443) {
		t.Fatalf("expected listen_port to remain, got %#v", got)
	}
}

func TestInboundMarshalJSON_MapsServerBandwidthForHysteriaAndHysteria2(t *testing.T) {
	cases := []struct {
		name              string
		typ               string
		serverUp          interface{}
		serverDown        interface{}
		wantRuntimeUp     float64
		wantRuntimeDown   float64
		wantStoredDownVal float64
	}{
		{name: "hy1-zero-fallback", typ: "hysteria", serverUp: 0, serverDown: 0, wantRuntimeUp: 10000, wantRuntimeDown: 10000, wantStoredDownVal: 0},
		{name: "hy2-positive", typ: "hysteria2", serverUp: 3456, serverDown: "4567", wantRuntimeUp: 3456, wantRuntimeDown: 4567, wantStoredDownVal: 4567},
	}

	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			inbound := Inbound{
				Type: tt.typ,
				Tag:  tt.name,
				Options: mustRawMessage(t, map[string]interface{}{
					"listen":           "::",
					"listen_port":      8443,
					"server_up_mbps":   tt.serverUp,
					"server_down_mbps": tt.serverDown,
				}),
			}

			raw, err := inbound.MarshalJSON()
			if err != nil {
				t.Fatalf("MarshalJSON failed: %v", err)
			}

			payload := map[string]interface{}{}
			if err := json.Unmarshal(raw, &payload); err != nil {
				t.Fatalf("unmarshal failed: %v", err)
			}

			if got := payload["up_mbps"]; got != tt.wantRuntimeUp {
				t.Fatalf("expected runtime up_mbps=%v, got %#v", tt.wantRuntimeUp, got)
			}
			if got := payload["down_mbps"]; got != tt.wantRuntimeDown {
				t.Fatalf("expected runtime down_mbps=%v, got %#v", tt.wantRuntimeDown, got)
			}
			if got := payload["listen_port"]; got != float64(8443) {
				t.Fatalf("expected listen_port to remain, got %#v", got)
			}

			full, err := inbound.MarshalFull()
			if err != nil {
				t.Fatalf("MarshalFull failed: %v", err)
			}
			if got := (*full)["server_up_mbps"]; got == nil {
				t.Fatalf("expected full payload to preserve server_up_mbps")
			}
			if got := inboundTestAsFloat64(t, (*full)["server_down_mbps"]); got != tt.wantStoredDownVal {
				t.Fatalf("expected full payload server_down_mbps=%v, got %#v", tt.wantStoredDownVal, (*full)["server_down_mbps"])
			}
			if _, exists := (*full)["up_mbps"]; exists {
				t.Fatalf("expected legacy up_mbps to be omitted from full payload, got %#v", (*full)["up_mbps"])
			}
			if _, exists := (*full)["down_mbps"]; exists {
				t.Fatalf("expected legacy down_mbps to be omitted from full payload, got %#v", (*full)["down_mbps"])
			}
		})
	}
}

func TestInboundMarshalJSON_OmitsZeroServerBandwidthForHysteria2(t *testing.T) {
	inbound := Inbound{
		Type: "hysteria2",
		Tag:  "hy2-zero-omit",
		Options: mustRawMessage(t, map[string]interface{}{
			"listen":           "::",
			"listen_port":      8443,
			"server_up_mbps":   0,
			"server_down_mbps": "",
		}),
	}

	raw, err := inbound.MarshalJSON()
	if err != nil {
		t.Fatalf("MarshalJSON failed: %v", err)
	}

	payload := map[string]interface{}{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}

	if _, exists := payload["up_mbps"]; exists {
		t.Fatalf("expected runtime up_mbps to be omitted, got %#v", payload["up_mbps"])
	}
	if _, exists := payload["down_mbps"]; exists {
		t.Fatalf("expected runtime down_mbps to be omitted, got %#v", payload["down_mbps"])
	}
	if got := payload["listen_port"]; got != float64(8443) {
		t.Fatalf("expected listen_port to remain, got %#v", got)
	}

	full, err := inbound.MarshalFull()
	if err != nil {
		t.Fatalf("MarshalFull failed: %v", err)
	}
	if got := inboundTestAsFloat64(t, (*full)["server_up_mbps"]); got != 0 {
		t.Fatalf("expected full payload server_up_mbps=0, got %#v", (*full)["server_up_mbps"])
	}
	if _, exists := (*full)["server_down_mbps"]; exists {
		t.Fatalf("expected empty full payload server_down_mbps to be omitted, got %#v", (*full)["server_down_mbps"])
	}
}
