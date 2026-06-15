package sub

import "testing"

func TestStripMihomoFields_SanitizesTransportAndMihomoKeys(t *testing.T) {
	outbounds := []map[string]interface{}{
		{
			"type":             "vless",
			"tag":              "xhttp-node",
			"mihomo_common":    map[string]interface{}{"routing_mark": 123},
			"mihomo_hy2":       map[string]interface{}{"up": 10},
			"mihomo_fast_open": true,
			"fast_open":        true,
			"tls": map[string]interface{}{
				"enabled":                true,
				"mihomo_use_fingerprint": true,
				"fingerprint":            "chrome",
			},
			"transport": map[string]interface{}{
				"type": "xhttp",
				"path": "/x",
			},
		},
		{
			"type": "vless",
			"tag":  "ws-node",
			"transport": map[string]interface{}{
				"type":                         "ws",
				"path":                         "/ws",
				"v2ray_http_upgrade":           true,
				"v2ray_http_upgrade_fast_open": true,
				"grpc_user_agent":              "ua",
				"ping_interval":                10,
				"max_connections":              8,
				"min_streams":                  0,
				"max_streams":                  0,
				"mode":                         "stream-up",
				"no_grpc_header":               true,
				"x_padding_bytes":              "100-1000",
				"sc_max_each_post_bytes":       1000000,
				"reuse_settings":               map[string]interface{}{"max_connections": "16-32"},
				"download_settings":            map[string]interface{}{"path": "/d"},
			},
		},
	}

	stripMihomoFields(&outbounds)

	xhttp := outbounds[0]
	for _, key := range []string{"mihomo_common", "mihomo_hy2", "mihomo_fast_open", "fast_open"} {
		if _, exists := xhttp[key]; exists {
			t.Fatalf("expected key %s to be removed, got %#v", key, xhttp[key])
		}
	}
	if _, exists := xhttp["transport"]; exists {
		t.Fatalf("expected xhttp transport to be removed, got %#v", xhttp["transport"])
	}
	tlsMap, ok := xhttp["tls"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected tls map, got %#v", xhttp["tls"])
	}
	for _, key := range []string{"mihomo_use_fingerprint", "fingerprint"} {
		if _, exists := tlsMap[key]; exists {
			t.Fatalf("expected tls key %s to be removed, got %#v", key, tlsMap[key])
		}
	}

	ws := outbounds[1]
	transport, ok := ws["transport"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected ws transport map, got %#v", ws["transport"])
	}
	if got, _ := transport["type"].(string); got != "ws" {
		t.Fatalf("expected transport.type ws, got %#v", transport["type"])
	}
	for _, key := range []string{
		"v2ray_http_upgrade",
		"v2ray_http_upgrade_fast_open",
		"grpc_user_agent",
		"ping_interval",
		"max_connections",
		"min_streams",
		"max_streams",
		"mode",
		"no_grpc_header",
		"x_padding_bytes",
		"sc_max_each_post_bytes",
		"reuse_settings",
		"download_settings",
	} {
		if _, exists := transport[key]; exists {
			t.Fatalf("expected transport key %s to be removed, got %#v", key, transport[key])
		}
	}
}
