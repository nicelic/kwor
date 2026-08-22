package service

import (
	"encoding/json"
	"testing"

	"github.com/alireza0/s-ui/database/model"
)

func TestSanitizeMihomoClientCommonFieldsUDP(t *testing.T) {
	tests := []struct {
		name      string
		value     interface{}
		hasValue  bool
		wantValue bool
		shadowTLS bool
	}{
		{name: "true", value: true, hasValue: true, wantValue: true},
		{name: "false", value: false, hasValue: true, wantValue: false},
		{name: "null", value: nil, hasValue: false},
		{name: "invalid", value: "true", hasValue: false},
		{name: "nested template", value: nil, hasValue: false, shadowTLS: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			common := map[string]interface{}{}
			if tt.name != "nested template" {
				common["udp"] = tt.value
			}

			outbound := map[string]interface{}{}
			if tt.shadowTLS {
				outbound["ss_config"] = map[string]interface{}{
					"mihomo_common": map[string]interface{}{"udp": tt.value},
				}
			} else {
				outbound["mihomo_common"] = common
			}
			raw, err := json.Marshal(outbound)
			if err != nil {
				t.Fatalf("marshal outbound: %v", err)
			}

			inbound := &model.MihomoInbound{OutJson: raw}
			if err := sanitizeMihomoClientCommonFields(inbound); err != nil {
				t.Fatalf("sanitize failed: %v", err)
			}

			var normalized map[string]interface{}
			if err := json.Unmarshal(inbound.OutJson, &normalized); err != nil {
				t.Fatalf("unmarshal normalized outbound: %v", err)
			}
			root := normalized
			if tt.shadowTLS {
				root = normalized["ss_config"].(map[string]interface{})
			}
			commonMap, _ := root["mihomo_common"].(map[string]interface{})
			commonValue, exists := commonMap["udp"]
			if exists != tt.hasValue {
				t.Fatalf("udp presence = %v, want %v; normalized=%#v", exists, tt.hasValue, normalized)
			}
			if tt.hasValue && commonValue != tt.wantValue {
				t.Fatalf("udp value = %#v, want %v", commonValue, tt.wantValue)
			}
		})
	}
}

func TestSanitizeMihomoClientCommonFieldsNormalizesIPVersionAndRoutingMark(t *testing.T) {
	tests := []struct {
		name            string
		ipVersion       interface{}
		routingMark     interface{}
		wantIPVersion   string
		wantRoutingMark int
	}{
		{name: "valid values", ipVersion: " IPv6-Prefer ", routingMark: float64(12), wantIPVersion: "ipv6-prefer", wantRoutingMark: 12},
		{name: "invalid values", ipVersion: "unsupported", routingMark: -1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			outbound := map[string]interface{}{
				"mihomo_common": map[string]interface{}{
					"udp":          false,
					"ip_version":   tt.ipVersion,
					"routing_mark": tt.routingMark,
				},
			}
			raw, err := json.Marshal(outbound)
			if err != nil {
				t.Fatalf("marshal outbound: %v", err)
			}

			inbound := &model.MihomoInbound{OutJson: raw}
			if err := sanitizeMihomoClientCommonFields(inbound); err != nil {
				t.Fatalf("sanitize failed: %v", err)
			}

			var normalized map[string]interface{}
			if err := json.Unmarshal(inbound.OutJson, &normalized); err != nil {
				t.Fatalf("unmarshal normalized outbound: %v", err)
			}
			common := normalized["mihomo_common"].(map[string]interface{})
			if tt.wantIPVersion == "" {
				if _, exists := common["ip_version"]; exists {
					t.Fatalf("invalid ip_version survived: %#v", common)
				}
			} else if common["ip_version"] != tt.wantIPVersion {
				t.Fatalf("ip_version = %#v, want %q", common["ip_version"], tt.wantIPVersion)
			}
			if tt.wantRoutingMark == 0 && tt.name == "invalid values" {
				if _, exists := common["routing_mark"]; exists {
					t.Fatalf("invalid routing_mark survived: %#v", common)
				}
			} else if common["routing_mark"] != float64(tt.wantRoutingMark) {
				t.Fatalf("routing_mark = %#v, want %d", common["routing_mark"], tt.wantRoutingMark)
			}
		})
	}
}

func TestSanitizeMihomoClientCommonFieldsStripsUnsupportedFastOpen(t *testing.T) {
	for _, inboundType := range []string{"tuic"} {
		t.Run(inboundType, func(t *testing.T) {
			inbound := &model.MihomoInbound{
				Type: inboundType,
				OutJson: json.RawMessage(`{
					"mihomo_fast_open": true,
					"fast_open": true,
					"fast-open": true
				}`),
			}

			if err := sanitizeMihomoClientCommonFields(inbound); err != nil {
				t.Fatalf("sanitize failed: %v", err)
			}

			var normalized map[string]interface{}
			if err := json.Unmarshal(inbound.OutJson, &normalized); err != nil {
				t.Fatalf("unmarshal normalized outbound: %v", err)
			}
			for _, key := range []string{"mihomo_fast_open", "fast_open", "fast-open"} {
				if _, exists := normalized[key]; exists {
					t.Fatalf("%s survived: %#v", key, normalized)
				}
			}
		})
	}
}

func TestSanitizeMihomoClientCommonFieldsKeepsHysteria2FastOpen(t *testing.T) {
	inbound := &model.MihomoInbound{
		Type:    "hysteria2",
		OutJson: json.RawMessage(`{"mihomo_fast_open":true}`),
	}
	if err := sanitizeMihomoClientCommonFields(inbound); err != nil {
		t.Fatalf("sanitize failed: %v", err)
	}
	var normalized map[string]interface{}
	if err := json.Unmarshal(inbound.OutJson, &normalized); err != nil {
		t.Fatalf("unmarshal normalized outbound: %v", err)
	}
	if got, _ := normalized["mihomo_fast_open"].(bool); !got {
		t.Fatalf("expected hysteria2 mihomo_fast_open to survive, got %#v", normalized)
	}
}

func TestSanitizeMihomoHysteria2ClientReceiveWindows(t *testing.T) {
	inbound := &model.MihomoInbound{
		Type: "hysteria2",
		OutJson: json.RawMessage(`{
			"mihomo_hy2": {
				"initial_stream_receive_window": 1024,
				"max_stream_receive_window": 2048.5,
				"initial_connection_receive_window": "4096",
				"max_connection_receive_window": 0,
				"unexpected": 123
			}
		}`),
	}

	if err := sanitizeMihomoClientCommonFields(inbound); err != nil {
		t.Fatalf("sanitize failed: %v", err)
	}

	var normalized map[string]interface{}
	if err := json.Unmarshal(inbound.OutJson, &normalized); err != nil {
		t.Fatalf("unmarshal normalized outbound: %v", err)
	}
	windows, ok := normalized["mihomo_hy2"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected normalized receive windows, got %#v", normalized)
	}
	if got := windows["initial_stream_receive_window"]; got != float64(1024) {
		t.Fatalf("initial stream receive window = %#v", got)
	}
	for _, key := range []string{
		"max_stream_receive_window",
		"initial_connection_receive_window",
		"max_connection_receive_window",
		"unexpected",
	} {
		if _, exists := windows[key]; exists {
			t.Fatalf("invalid receive window key %q survived: %#v", key, windows)
		}
	}
}

func TestSanitizeMihomoClientCommonFieldsRejectsFractionalSMuxValues(t *testing.T) {
	inbound := &model.MihomoInbound{
		OutJson: json.RawMessage(`{
			"mihomo_common": {
				"smux": {
					"enabled": true,
					"max_connections": 8.5,
					"min_streams": 0,
					"max_streams": 16,
					"brutal": {
						"enabled": true,
						"up_mbps": 100.5,
						"down_mbps": 200
					}
				}
			}
		}`),
	}

	if err := sanitizeMihomoClientCommonFields(inbound); err != nil {
		t.Fatalf("sanitize failed: %v", err)
	}

	var normalized map[string]interface{}
	if err := json.Unmarshal(inbound.OutJson, &normalized); err != nil {
		t.Fatalf("unmarshal normalized outbound: %v", err)
	}
	common := normalized["mihomo_common"].(map[string]interface{})
	mux := common["smux"].(map[string]interface{})
	if _, exists := mux["max_connections"]; exists {
		t.Fatalf("fractional max_connections survived: %#v", mux)
	}
	if got := mux["min_streams"]; got != float64(0) {
		t.Fatalf("min_streams = %#v", got)
	}
	if got := mux["max_streams"]; got != float64(16) {
		t.Fatalf("max_streams = %#v", got)
	}
	brutal := mux["brutal"].(map[string]interface{})
	if _, exists := brutal["up_mbps"]; exists {
		t.Fatalf("fractional brutal up_mbps survived: %#v", brutal)
	}
	if got := brutal["down_mbps"]; got != float64(200) {
		t.Fatalf("brutal down_mbps = %#v", got)
	}
}

func TestSanitizeMihomoClientCommonFieldsRemovesNullSMuxBranches(t *testing.T) {
	inbound := &model.MihomoInbound{
		OutJson: json.RawMessage(`{
			"mihomo_common": {
				"smux": null
			}
		}`),
	}

	if err := sanitizeMihomoClientCommonFields(inbound); err != nil {
		t.Fatalf("sanitize failed: %v", err)
	}

	var normalized map[string]interface{}
	if err := json.Unmarshal(inbound.OutJson, &normalized); err != nil {
		t.Fatalf("unmarshal normalized outbound: %v", err)
	}
	if _, exists := normalized["mihomo_common"]; exists {
		t.Fatalf("empty mihomo_common survived: %#v", normalized)
	}
}

func TestSanitizeMihomoClientCommonFieldsRejectsFractionalGRPCValues(t *testing.T) {
	inbound := &model.MihomoInbound{
		OutJson: json.RawMessage(`{
			"transport": {
				"type": "grpc",
				"ping_interval": 10.5,
				"max_connections": 8,
				"min_streams": -1,
				"max_streams": 16.5
			}
		}`),
	}

	if err := sanitizeMihomoClientCommonFields(inbound); err != nil {
		t.Fatalf("sanitize failed: %v", err)
	}

	var normalized map[string]interface{}
	if err := json.Unmarshal(inbound.OutJson, &normalized); err != nil {
		t.Fatalf("unmarshal normalized outbound: %v", err)
	}
	transport, ok := normalized["transport"].(map[string]interface{})
	if !ok {
		t.Fatalf("transport missing: %#v", normalized)
	}
	if got := transport["max_connections"]; got != float64(8) {
		t.Fatalf("max_connections = %#v", got)
	}
	for _, key := range []string{"ping_interval", "min_streams", "max_streams"} {
		if _, exists := transport[key]; exists {
			t.Fatalf("invalid gRPC field %q survived: %#v", key, transport)
		}
	}
}

func TestSanitizeMihomoClientCommonFieldsStripsUnsupportedTLSFragment(t *testing.T) {
	inbound := &model.MihomoInbound{
		OutJson: json.RawMessage(`{
			"tls": {
				"enabled": true,
				"fragment": true,
				"fragment_fallback_delay": "500ms",
				"record_fragment": true
			}
		}`),
	}

	if err := sanitizeMihomoClientCommonFields(inbound); err != nil {
		t.Fatalf("sanitize failed: %v", err)
	}

	var normalized map[string]interface{}
	if err := json.Unmarshal(inbound.OutJson, &normalized); err != nil {
		t.Fatalf("unmarshal normalized outbound: %v", err)
	}
	tls, ok := normalized["tls"].(map[string]interface{})
	if !ok {
		t.Fatalf("TLS missing: %#v", normalized)
	}
	for _, key := range []string{"fragment", "fragment_fallback_delay", "record_fragment"} {
		if _, exists := tls[key]; exists {
			t.Fatalf("unsupported TLS field %q survived: %#v", key, tls)
		}
	}
}

func TestSanitizeMihomoClientCommonFieldsNormalizesOptionalCommonSwitches(t *testing.T) {
	inbound := &model.MihomoInbound{
		Type: "tuic",
		OutJson: json.RawMessage(`{
			"mihomo_common": {
				"tcp_fast_open": true,
				"tcp_multi_path": "false",
				"bbr_profile": " AGGRESSIVE ",
				"mux": {
					"protocol": "yamux",
					"max_connections": 8
				}
			}
		}`),
	}

	if err := sanitizeMihomoClientCommonFields(inbound); err != nil {
		t.Fatalf("sanitize failed: %v", err)
	}

	var normalized map[string]interface{}
	if err := json.Unmarshal(inbound.OutJson, &normalized); err != nil {
		t.Fatalf("unmarshal normalized outbound: %v", err)
	}
	common, ok := normalized["mihomo_common"].(map[string]interface{})
	if !ok {
		t.Fatalf("mihomo_common missing: %#v", normalized)
	}
	if got := common["tcp_fast_open"]; got != true {
		t.Fatalf("tcp_fast_open = %#v", got)
	}
	if _, exists := common["tcp_multi_path"]; exists {
		t.Fatalf("invalid tcp_multi_path survived: %#v", common)
	}
	if got := common["bbr_profile"]; got != "aggressive" {
		t.Fatalf("bbr_profile = %#v", got)
	}
	if _, exists := common["mux"]; exists {
		t.Fatalf("legacy mux survived: %#v", common)
	}
	smux, ok := common["smux"].(map[string]interface{})
	if !ok || smux["enabled"] != true || smux["protocol"] != "yamux" || smux["max_connections"] != float64(8) {
		t.Fatalf("smux = %#v", smux)
	}
}

func TestSanitizeMihomoClientCommonFieldsDropsDisabledSMux(t *testing.T) {
	inbound := &model.MihomoInbound{
		OutJson: json.RawMessage(`{
			"mihomo_common": {
				"smux": {
					"enabled": false,
					"protocol": "yamux"
				}
			}
		}`),
	}

	if err := sanitizeMihomoClientCommonFields(inbound); err != nil {
		t.Fatalf("sanitize failed: %v", err)
	}

	var normalized map[string]interface{}
	if err := json.Unmarshal(inbound.OutJson, &normalized); err != nil {
		t.Fatalf("unmarshal normalized outbound: %v", err)
	}
	if _, exists := normalized["mihomo_common"]; exists {
		t.Fatalf("disabled smux survived: %#v", normalized)
	}
}
