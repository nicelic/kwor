package service

import "testing"

func TestExpandSubOutboundsForSubscription_PassThroughTypes(t *testing.T) {
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
		outbounds, tags := expandSubOutboundsForSubscription(raw)
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

func TestExpandSubOutboundsForSubscription_ShadowTLSSplit(t *testing.T) {
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

	outbounds, tags := expandSubOutboundsForSubscription(raw)
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

func TestExpandSubOutboundsForSubscription_ShadowTLSNoSsConfigSanitizesInboundOnlyFields(t *testing.T) {
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

	outbounds, tags := expandSubOutboundsForSubscription(raw)
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
