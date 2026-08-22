package util

import "testing"

func TestOutboundClientConfigAllowListsRejectUnknownFields(t *testing.T) {
	tests := []struct {
		name     string
		skip     func(string, string, bool) bool
		protocol string
		key      string
		hasTLS   bool
		wantSkip bool
	}{
		{name: "singbox socks credentials", skip: ShouldSkipSingboxOutboundClientConfigKey, protocol: "socks", key: "username", wantSkip: false},
		{name: "singbox vless flow with tls", skip: ShouldSkipSingboxOutboundClientConfigKey, protocol: "vless", key: "flow", hasTLS: true, wantSkip: false},
		{name: "singbox vless flow without tls", skip: ShouldSkipSingboxOutboundClientConfigKey, protocol: "vless", key: "flow", wantSkip: true},
		{name: "singbox vmess username", skip: ShouldSkipSingboxOutboundClientConfigKey, protocol: "vmess", key: "username", wantSkip: true},
		{name: "singbox mieru credentials", skip: ShouldSkipSingboxOutboundClientConfigKey, protocol: "mieru", key: "username", wantSkip: true},
		{name: "singbox snell psk", skip: ShouldSkipSingboxOutboundClientConfigKey, protocol: "snell", key: "psk", wantSkip: true},
		{name: "clash mieru credentials", skip: ShouldSkipClashSubscriptionClientConfigKey, protocol: "mieru", key: "username", wantSkip: false},
		{name: "singbox unknown metadata", skip: ShouldSkipSingboxOutboundClientConfigKey, protocol: "trojan", key: "metadata", wantSkip: true},
		{name: "mihomo vmess username", skip: ShouldSkipMihomoOutboundClientConfigKey, protocol: "vmess", key: "username", wantSkip: false},
		{name: "mihomo trusttunnel credentials", skip: ShouldSkipMihomoOutboundClientConfigKey, protocol: "trusttunnel", key: "password", wantSkip: false},
		{name: "mihomo trusttunnel legacy uuid", skip: ShouldSkipMihomoOutboundClientConfigKey, protocol: "trusttunnel", key: "uuid", wantSkip: true},
		{name: "mihomo snell psk", skip: ShouldSkipMihomoOutboundClientConfigKey, protocol: "snell", key: "psk", wantSkip: false},
		{name: "mihomo unknown field", skip: ShouldSkipMihomoOutboundClientConfigKey, protocol: "mieru", key: "unexpected", wantSkip: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.skip(test.protocol, test.key, test.hasTLS); got != test.wantSkip {
				t.Fatalf("skip(%q, %q, hasTLS=%t) = %t, want %t", test.protocol, test.key, test.hasTLS, got, test.wantSkip)
			}
		})
	}
}

func TestSanitizeSingboxSubscriptionOutboundRemovesPanelFields(t *testing.T) {
	outbound := map[string]interface{}{
		"type":             "vmess",
		"tag":              "node-a",
		"uuid":             "11111111-1111-1111-1111-111111111111",
		"id":               7,
		"tls_id":           3,
		"route_tag":        "node-a",
		"user_management":  map[string]interface{}{"selectable": true},
		"metadata":         map[string]interface{}{"source": "panel"},
		"users":            []interface{}{},
		"inbounds":         []interface{}{7},
		"addrs":            []interface{}{},
		"out_json":         map[string]interface{}{},
		"options":          map[string]interface{}{},
		"alterId":          0,
		"mihomo_common":    map[string]interface{}{"udp": true},
		"mihomo_hy2":       map[string]interface{}{"fast_open": true},
		"mihomo_fast_open": true,
		"fast_open":        true,
		"tls": map[string]interface{}{
			"enabled":                    true,
			"server_name":                "edge.example.com",
			"mihomo_use_fingerprint":     true,
			"fingerprint":                "AA:BB",
			"include_server_certificate": true,
			"include_server_fingerprint": true,
		},
	}

	SanitizeSingboxSubscriptionOutbound(outbound)

	for _, key := range []string{"id", "tls_id", "route_tag", "user_management", "metadata", "users", "inbounds", "addrs", "out_json", "options", "alterId", "mihomo_common", "mihomo_hy2", "mihomo_fast_open", "fast_open"} {
		if _, exists := outbound[key]; exists {
			t.Fatalf("panel-only field %q survived outbound sanitization: %#v", key, outbound)
		}
	}
	tlsMap, _ := outbound["tls"].(map[string]interface{})
	for _, key := range []string{"mihomo_use_fingerprint", "fingerprint", "include_server_certificate", "include_server_fingerprint"} {
		if _, exists := tlsMap[key]; exists {
			t.Fatalf("Mihomo-only TLS field %q survived outbound sanitization: %#v", key, tlsMap)
		}
	}
	if got := tlsMap["server_name"]; got != "edge.example.com" {
		t.Fatalf("expected valid TLS server_name to remain, got %#v", got)
	}
	if got := outbound["uuid"]; got != "11111111-1111-1111-1111-111111111111" {
		t.Fatalf("expected valid uuid to remain, got %#v", got)
	}
}

func TestSanitizeSingboxSubscriptionOutboundPromotesHysteria2ReceiveWindows(t *testing.T) {
	outbound := map[string]interface{}{
		"type": "hysteria2",
		"mihomo_hy2": map[string]interface{}{
			"initial_stream_receive_window":     38000000,
			"max_stream_receive_window":         70000000,
			"initial_connection_receive_window": 120000000,
			"max_connection_receive_window":     150000000,
		},
		"mihomo_fast_open": true,
	}

	SanitizeSingboxSubscriptionOutbound(outbound)

	for key, want := range map[string]interface{}{
		"initial_stream_receive_window":     38000000,
		"max_stream_receive_window":         70000000,
		"initial_connection_receive_window": 120000000,
		"max_connection_receive_window":     150000000,
	} {
		if got := outbound[key]; got != want {
			t.Fatalf("%s = %#v, want %#v", key, got, want)
		}
	}
	if _, exists := outbound["mihomo_hy2"]; exists {
		t.Fatalf("mihomo_hy2 wrapper survived sing-box sanitization: %#v", outbound)
	}
	if _, exists := outbound["mihomo_fast_open"]; exists {
		t.Fatalf("mihomo_fast_open survived sing-box sanitization: %#v", outbound)
	}
}

func TestSubscriptionClientConfigUsernamePrefersCanonicalValue(t *testing.T) {
	if got := SubscriptionClientConfigUsername(map[string]interface{}{"name": "legacy"}); got != "legacy" {
		t.Fatalf("legacy name fallback = %q, want legacy", got)
	}
	if got := SubscriptionClientConfigUsername(map[string]interface{}{"username": "current", "name": "legacy"}); got != "current" {
		t.Fatalf("canonical username = %q, want current", got)
	}
}
