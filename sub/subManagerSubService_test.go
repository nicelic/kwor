package sub

import "testing"

func TestBuildSubManagerRuntimeOutbounds_PassThroughTypes(t *testing.T) {
	passThroughTypes := []string{
		"direct",
		"socks",
		"http",
		"shadowsocks",
		"vmess",
		"trojan",
		"hysteria",
		"vless",
		"tuic",
		"hysteria2",
		"anytls",
		"tor",
		"ssh",
		"selector",
		"urltest",
	}

	for _, typ := range passThroughTypes {
		raw := []map[string]interface{}{
			{
				"type": typ,
				"tag":  "node-" + typ,
			},
		}
		outbounds, tags := buildSubManagerRuntimeOutbounds(raw)
		if len(outbounds) != 1 {
			t.Fatalf("type=%s expected 1 outbound, got %d", typ, len(outbounds))
		}
		if len(tags) != 1 || tags[0] != "node-"+typ {
			t.Fatalf("type=%s expected out tag node-%s, got %#v", typ, typ, tags)
		}
		if gotType, _ := outbounds[0]["type"].(string); gotType != typ {
			t.Fatalf("type=%s expected outbound type %s, got %s", typ, typ, gotType)
		}
	}
}

func TestBuildSubManagerRuntimeOutboundsClonesNestedPassThroughFields(t *testing.T) {
	raw := []map[string]interface{}{
		{
			"type": "trojan",
			"tag":  "trojan-node",
			"tls": map[string]interface{}{
				"server_name": "source.example.com",
				"utls":        map[string]interface{}{"fingerprint": "chrome"},
			},
			"transport": map[string]interface{}{
				"type":    "ws",
				"headers": map[string]interface{}{"Host": "source.example.com"},
			},
		},
	}

	outbounds, _ := buildSubManagerRuntimeOutbounds(raw)
	if len(outbounds) != 1 {
		t.Fatalf("expected one pass-through runtime outbound, got %#v", outbounds)
	}

	outbounds[0]["tls"].(map[string]interface{})["server_name"] = "mutated.example.com"
	outbounds[0]["tls"].(map[string]interface{})["utls"].(map[string]interface{})["fingerprint"] = "firefox"
	outbounds[0]["transport"].(map[string]interface{})["headers"].(map[string]interface{})["Host"] = "mutated.example.com"

	sourceTLS := raw[0]["tls"].(map[string]interface{})
	if got, _ := sourceTLS["server_name"].(string); got != "source.example.com" {
		t.Fatalf("sub-manager runtime expansion mutated source TLS: %#v", sourceTLS)
	}
	if got, _ := sourceTLS["utls"].(map[string]interface{})["fingerprint"].(string); got != "chrome" {
		t.Fatalf("sub-manager runtime expansion mutated source nested TLS: %#v", sourceTLS)
	}
	sourceTransport := raw[0]["transport"].(map[string]interface{})
	if got, _ := sourceTransport["headers"].(map[string]interface{})["Host"].(string); got != "source.example.com" {
		t.Fatalf("sub-manager runtime expansion mutated source transport: %#v", sourceTransport)
	}
}

func TestBuildSubManagerRuntimeOutbounds_ExpandsMixed(t *testing.T) {
	raw := []map[string]interface{}{
		{
			"type":        "mixed",
			"tag":         "manual-mixed",
			"server":      "proxy.example.com",
			"server_port": 1080,
			"username":    "alice",
			"password":    "secret",
		},
	}

	outbounds, tags := buildSubManagerRuntimeOutbounds(raw)
	if len(outbounds) != 2 || len(tags) != 2 {
		t.Fatalf("mixed endpoint must expand into two runtime outbounds, got outbounds=%#v tags=%#v", outbounds, tags)
	}
	if tags[0] != "manual-mixed-socks" || tags[1] != "manual-mixed-http" {
		t.Fatalf("unexpected mixed runtime tags: %#v", tags)
	}
	if outbounds[0]["type"] != "socks" || outbounds[1]["type"] != "http" {
		t.Fatalf("unexpected mixed runtime outbounds: %#v", outbounds)
	}
}

func TestBuildSubManagerRuntimeOutbounds_ShadowTLSSplit(t *testing.T) {
	raw := []map[string]interface{}{
		{
			"type":         "shadowtls",
			"tag":          "stls",
			"server":       "1.2.3.4",
			"wildcard_sni": "all",
			"strict_mode":  true,
			"handshake": map[string]interface{}{
				"server":      "addons.mozilla.org",
				"server_port": 443,
			},
			"ss_config": map[string]interface{}{
				"method":       "2022-blake3-aes-128-gcm",
				"network":      "tcp",
				"password":     "pass",
				"udp_over_tcp": true,
				"multiplex": map[string]interface{}{
					"enabled": true,
				},
			},
		},
	}

	outbounds, tags := buildSubManagerRuntimeOutbounds(raw)
	if len(outbounds) != 2 {
		t.Fatalf("shadowtls expected 2 outbounds, got %d", len(outbounds))
	}
	if len(tags) != 1 || tags[0] != "stls" {
		t.Fatalf("shadowtls expected tags [stls], got %#v", tags)
	}

	if outbounds[0]["type"] != "shadowsocks" || outbounds[0]["tag"] != "stls" || outbounds[0]["detour"] != "stls-out" {
		t.Fatalf("unexpected shadowsocks outbound: %#v", outbounds[0])
	}
	if outbounds[0]["network"] != "tcp" {
		t.Fatalf("expected shadowsocks network=tcp, got %#v", outbounds[0]["network"])
	}
	if outbounds[1]["type"] != "shadowtls" || outbounds[1]["tag"] != "stls-out" {
		t.Fatalf("unexpected shadowtls outbound: %#v", outbounds[1])
	}
	if _, ok := outbounds[1]["wildcard_sni"]; ok {
		t.Fatalf("shadowtls outbound should not contain wildcard_sni: %#v", outbounds[1])
	}
	if _, ok := outbounds[1]["strict_mode"]; ok {
		t.Fatalf("shadowtls outbound should not contain strict_mode: %#v", outbounds[1])
	}
	if _, ok := outbounds[1]["handshake"]; ok {
		t.Fatalf("shadowtls outbound should not contain handshake: %#v", outbounds[1])
	}
	if _, ok := outbounds[1]["ss_config"]; ok {
		t.Fatalf("shadowtls outbound should not contain ss_config: %#v", outbounds[1])
	}
}

func TestBuildSubManagerRuntimeOutbounds_ShadowTLSNoSsConfig(t *testing.T) {
	raw := []map[string]interface{}{
		{
			"type":         "shadowtls",
			"tag":          "stls-no-ss",
			"wildcard_sni": "all",
			"strict_mode":  true,
			"handshake": map[string]interface{}{
				"server":      "addons.mozilla.org",
				"server_port": 443,
			},
		},
	}

	outbounds, tags := buildSubManagerRuntimeOutbounds(raw)
	if len(outbounds) != 1 {
		t.Fatalf("shadowtls without ss_config expected 1 outbound, got %d", len(outbounds))
	}
	if len(tags) != 1 || tags[0] != "stls-no-ss" {
		t.Fatalf("shadowtls without ss_config expected tags [stls-no-ss], got %#v", tags)
	}
	if outbounds[0]["type"] != "shadowtls" || outbounds[0]["tag"] != "stls-no-ss" {
		t.Fatalf("unexpected outbound: %#v", outbounds[0])
	}
	if _, ok := outbounds[0]["wildcard_sni"]; ok {
		t.Fatalf("shadowtls outbound should not contain wildcard_sni: %#v", outbounds[0])
	}
	if _, ok := outbounds[0]["strict_mode"]; ok {
		t.Fatalf("shadowtls outbound should not contain strict_mode: %#v", outbounds[0])
	}
	if _, ok := outbounds[0]["handshake"]; ok {
		t.Fatalf("shadowtls outbound should not contain handshake: %#v", outbounds[0])
	}
}

func TestBuildSubManagerRuntimeOutbounds_MihomoShadowsocksShadowTLSPlugin(t *testing.T) {
	raw := []map[string]interface{}{
		{
			"type":               "shadowsocks",
			"tag":                "mihomo-ss-shadowtls",
			"server":             "203.0.113.10",
			"server_port":        443,
			"method":             "2022-blake3-aes-128-gcm",
			"password":           "ss-pass",
			"plugin":             "shadow-tls",
			"plugin_opts":        map[string]interface{}{"host": "addons.mozilla.org", "version": 3, "password": "shadow-pass"},
			"client_fingerprint": "safari",
		},
	}

	outbounds, tags := buildSubManagerRuntimeOutbounds(raw)
	if len(outbounds) != 2 || len(tags) != 1 || tags[0] != "mihomo-ss-shadowtls" {
		t.Fatalf("unexpected expanded outbounds=%#v tags=%#v", outbounds, tags)
	}
	if outbounds[0]["type"] != "shadowsocks" || outbounds[0]["detour"] != "mihomo-ss-shadowtls-out" {
		t.Fatalf("unexpected Shadowsocks detour outbound: %#v", outbounds[0])
	}
	if _, exists := outbounds[0]["plugin"]; exists {
		t.Fatalf("sing-box Shadowsocks outbound must not retain plugin: %#v", outbounds[0])
	}
	if outbounds[1]["type"] != "shadowtls" || outbounds[1]["tag"] != "mihomo-ss-shadowtls-out" || outbounds[1]["password"] != "shadow-pass" {
		t.Fatalf("unexpected ShadowTLS outbound: %#v", outbounds[1])
	}
	tls, ok := outbounds[1]["tls"].(map[string]interface{})
	if !ok || tls["server_name"] != "addons.mozilla.org" {
		t.Fatalf("unexpected ShadowTLS TLS settings: %#v", outbounds[1]["tls"])
	}
	utls, ok := tls["utls"].(map[string]interface{})
	if !ok || utls["fingerprint"] != "safari" {
		t.Fatalf("unexpected ShadowTLS uTLS settings: %#v", tls["utls"])
	}
}
