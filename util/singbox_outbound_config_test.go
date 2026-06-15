package util

import "testing"

func TestShouldSkipSingboxOutboundClientConfigKey(t *testing.T) {
	tests := []struct {
		protocol string
		key      string
		hasTLS   bool
		wantSkip bool
	}{
		{protocol: "vmess", key: "username", hasTLS: true, wantSkip: true},
		{protocol: "vless", key: "username", hasTLS: true, wantSkip: true},
		{protocol: "trojan", key: "username", hasTLS: true, wantSkip: true},
		{protocol: "anytls", key: "username", hasTLS: true, wantSkip: true},
		{protocol: "hysteria2", key: "username", hasTLS: true, wantSkip: true},
		{protocol: "naive", key: "username", hasTLS: true, wantSkip: false},
		{protocol: "http", key: "username", hasTLS: true, wantSkip: false},
		{protocol: "socks", key: "username", hasTLS: true, wantSkip: false},
		{protocol: "vless", key: "flow", hasTLS: false, wantSkip: true},
		{protocol: "vless", key: "flow", hasTLS: true, wantSkip: false},
		{protocol: "trojan", key: "name", hasTLS: true, wantSkip: true},
	}

	for _, tt := range tests {
		if got := ShouldSkipSingboxOutboundClientConfigKey(tt.protocol, tt.key, tt.hasTLS); got != tt.wantSkip {
			t.Fatalf("protocol=%s key=%s hasTLS=%v wantSkip=%v got=%v", tt.protocol, tt.key, tt.hasTLS, tt.wantSkip, got)
		}
	}
}

func TestShouldSkipMihomoOutboundClientConfigKey(t *testing.T) {
	tests := []struct {
		protocol string
		key      string
		hasTLS   bool
		wantSkip bool
	}{
		{protocol: "vmess", key: "username", hasTLS: true, wantSkip: false},
		{protocol: "vless", key: "username", hasTLS: true, wantSkip: false},
		{protocol: "trojan", key: "username", hasTLS: true, wantSkip: false},
		{protocol: "anytls", key: "username", hasTLS: true, wantSkip: false},
		{protocol: "hysteria2", key: "username", hasTLS: true, wantSkip: false},
		{protocol: "mieru", key: "username", hasTLS: true, wantSkip: false},
		{protocol: "trusttunnel", key: "username", hasTLS: true, wantSkip: false},
		{protocol: "tuic", key: "username", hasTLS: true, wantSkip: true},
		{protocol: "vless", key: "flow", hasTLS: false, wantSkip: true},
		{protocol: "vless", key: "flow", hasTLS: true, wantSkip: false},
		{protocol: "trojan", key: "name", hasTLS: true, wantSkip: true},
	}

	for _, tt := range tests {
		if got := ShouldSkipMihomoOutboundClientConfigKey(tt.protocol, tt.key, tt.hasTLS); got != tt.wantSkip {
			t.Fatalf("protocol=%s key=%s hasTLS=%v wantSkip=%v got=%v", tt.protocol, tt.key, tt.hasTLS, tt.wantSkip, got)
		}
	}
}

func TestSanitizeSingboxSubscriptionOutbound(t *testing.T) {
	vmess := map[string]interface{}{
		"type":     "vmess",
		"username": "client",
		"name":     "legacy",
	}
	SanitizeSingboxSubscriptionOutbound(vmess)
	if _, exists := vmess["username"]; exists {
		t.Fatalf("expected vmess username to be removed, got %#v", vmess["username"])
	}
	if _, exists := vmess["name"]; exists {
		t.Fatalf("expected vmess name to be removed, got %#v", vmess["name"])
	}

	naive := map[string]interface{}{
		"type":     "naive",
		"username": "client",
		"name":     "legacy",
	}
	SanitizeSingboxSubscriptionOutbound(naive)
	if got, _ := naive["username"].(string); got != "client" {
		t.Fatalf("expected naive username to be preserved, got %#v", naive["username"])
	}
	if _, exists := naive["name"]; exists {
		t.Fatalf("expected naive name to be removed, got %#v", naive["name"])
	}
}

func TestSanitizeSingboxSubscriptionOutbound_TransportCompat(t *testing.T) {
	t.Run("removes mihomo ws extension fields", func(t *testing.T) {
		outbound := map[string]interface{}{
			"type": "vless",
			"transport": map[string]interface{}{
				"type":                         "ws",
				"path":                         "/ws",
				"v2ray_http_upgrade":           true,
				"v2ray_http_upgrade_fast_open": true,
				"x_padding_bytes":              "100-1000",
				"sc_max_each_post_bytes":       1000000,
				"download_settings":            map[string]interface{}{"path": "/d"},
				"reuse_settings":               map[string]interface{}{"max_connections": "16-32"},
				"grpc_user_agent":              "ua",
				"no_grpc_header":               true,
				"mode":                         "stream-up",
				"sc_stream_up_server_secs":     "15",
				"ping_interval":                10,
				"max_connections":              8,
				"min_streams":                  0,
				"max_streams":                  0,
				"no_sse_header":                true,
			},
		}

		SanitizeSingboxSubscriptionOutbound(outbound)
		transport, ok := outbound["transport"].(map[string]interface{})
		if !ok {
			t.Fatalf("expected transport map, got %#v", outbound["transport"])
		}
		if got, _ := transport["type"].(string); got != "ws" {
			t.Fatalf("expected transport.type ws, got %#v", transport["type"])
		}
		for _, key := range []string{
			"v2ray_http_upgrade",
			"v2ray_http_upgrade_fast_open",
			"x_padding_bytes",
			"sc_max_each_post_bytes",
			"download_settings",
			"reuse_settings",
			"grpc_user_agent",
			"no_grpc_header",
			"mode",
			"sc_stream_up_server_secs",
			"ping_interval",
			"max_connections",
			"min_streams",
			"max_streams",
			"no_sse_header",
		} {
			if _, exists := transport[key]; exists {
				t.Fatalf("expected key %s to be removed, got %#v", key, transport[key])
			}
		}
	})

	t.Run("maps h2 to http for sing-box transport schema", func(t *testing.T) {
		outbound := map[string]interface{}{
			"type": "vmess",
			"transport": map[string]interface{}{
				"type": "h2",
				"path": "/api",
				"host": []interface{}{"h2.example.com"},
			},
		}

		SanitizeSingboxSubscriptionOutbound(outbound)
		transport, ok := outbound["transport"].(map[string]interface{})
		if !ok {
			t.Fatalf("expected transport map, got %#v", outbound["transport"])
		}
		if got, _ := transport["type"].(string); got != "http" {
			t.Fatalf("expected transport.type http, got %#v", transport["type"])
		}
		if got, _ := transport["path"].(string); got != "/api" {
			t.Fatalf("expected transport.path /api, got %#v", transport["path"])
		}
	})

	t.Run("drops xhttp transport for sing-box output", func(t *testing.T) {
		outbound := map[string]interface{}{
			"type": "vless",
			"transport": map[string]interface{}{
				"type": "xhttp",
				"path": "/x",
			},
		}

		SanitizeSingboxSubscriptionOutbound(outbound)
		if _, exists := outbound["transport"]; exists {
			t.Fatalf("expected xhttp transport to be removed, got %#v", outbound["transport"])
		}
	})
}
