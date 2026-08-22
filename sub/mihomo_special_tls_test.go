package sub

import "testing"

func TestMihomoWrapperTLSProjectsForAllSupportedClashProtocols(t *testing.T) {
	protocols := []struct {
		name   string
		sniKey string
		apply  func(map[string]interface{})
	}{
		{
			name:   "vmess",
			sniKey: "servername",
			apply: func(outbound map[string]interface{}) {
				outbound["uuid"] = "00000000-0000-0000-0000-000000000001"
			},
		},
		{
			name:   "vless",
			sniKey: "servername",
			apply: func(outbound map[string]interface{}) {
				outbound["uuid"] = "00000000-0000-0000-0000-000000000002"
				outbound["encryption"] = "none"
			},
		},
		{
			name:   "trojan",
			sniKey: "sni",
			apply: func(outbound map[string]interface{}) {
				outbound["password"] = "trojan-pass"
			},
		},
		{
			name:   "anytls",
			sniKey: "sni",
			apply: func(outbound map[string]interface{}) {
				outbound["password"] = "anytls-pass"
			},
		},
	}
	wrappers := []struct {
		name       string
		inputKey   string
		outputKey  string
		options    map[string]interface{}
		expectedKV map[string]interface{}
	}{
		{
			name:       "shadowtls",
			inputKey:   "shadow_tls_opts",
			outputKey:  "shadow-tls-opts",
			options:    map[string]interface{}{"version": 3, "password": "shadow-pass"},
			expectedKV: map[string]interface{}{"version": 3, "password": "shadow-pass"},
		},
		{
			name:       "restls",
			inputKey:   "restls_opts",
			outputKey:  "restls-opts",
			options:    map[string]interface{}{"password": "restls-pass", "version_hint": "tls13", "restls_script": "300?100"},
			expectedKV: map[string]interface{}{"password": "restls-pass", "version-hint": "tls13", "restls-script": "300?100"},
		},
		{
			name:       "jls",
			inputKey:   "jls_opts",
			outputKey:  "jls-opts",
			options:    map[string]interface{}{"username": "jls-user", "password": "jls-pass"},
			expectedKV: map[string]interface{}{"username": "jls-user", "password": "jls-pass"},
		},
	}

	for _, protocol := range protocols {
		for _, wrapper := range wrappers {
			t.Run(protocol.name+"/"+wrapper.name, func(t *testing.T) {
				tls := map[string]interface{}{
					"enabled":        true,
					"server_name":    "sni.example.com",
					wrapper.inputKey: wrapper.options,
				}
				outbound := map[string]interface{}{
					"type":        protocol.name,
					"tag":         protocol.name + "-" + wrapper.name,
					"server":      "edge.example.com",
					"server_port": 443,
					"tls":         tls,
				}
				protocol.apply(outbound)

				outbounds := []map[string]interface{}{outbound}
				document, err := (&ClashService{}).convertToClashMetaMap(&outbounds, "http://www.gstatic.com/generate_204", 300, 50, nil, true)
				if err != nil {
					t.Fatalf("convert subscription: %v", err)
				}
				proxies, ok := document["proxies"].([]interface{})
				if !ok || len(proxies) != 1 {
					t.Fatalf("unexpected proxies: %#v", document["proxies"])
				}
				proxy, ok := proxies[0].(map[string]interface{})
				if !ok {
					t.Fatalf("unexpected proxy: %#v", proxies[0])
				}
				if proxy["tls"] != true {
					t.Fatalf("expected TLS to stay enabled: %#v", proxy)
				}
				if proxy[protocol.sniKey] != "sni.example.com" {
					t.Fatalf("expected %s to use outer TLS SNI, got %#v", protocol.sniKey, proxy[protocol.sniKey])
				}
				options, ok := proxy[wrapper.outputKey].(map[string]interface{})
				if !ok {
					t.Fatalf("missing %s: %#v", wrapper.outputKey, proxy)
				}
				for key, expected := range wrapper.expectedKV {
					if options[key] != expected {
						t.Fatalf("%s.%s = %#v, want %#v", wrapper.outputKey, key, options[key], expected)
					}
				}
			})
		}
	}
}

func TestApplyMihomoSpecialTLSProxyOptionsUsesOfficialKeys(t *testing.T) {
	proxy := map[string]interface{}{}
	tlsMap := map[string]interface{}{
		"shadow_tls_opts": map[string]interface{}{"version": 3, "password": "p"},
		"restls_opts":     map[string]interface{}{"password": "p", "version_hint": "tls13", "restls_script": ""},
		"jls_opts":        map[string]interface{}{"username": "u", "password": "p"},
	}
	applyMihomoSpecialTLSProxyOptions(proxy, tlsMap)
	if _, ok := proxy["shadow-tls-opts"].(map[string]interface{}); !ok {
		t.Fatalf("missing shadow-tls-opts: %#v", proxy)
	}
	restls, ok := proxy["restls-opts"].(map[string]interface{})
	if !ok || restls["version-hint"] != "tls13" || restls["restls-script"] != "" {
		t.Fatalf("unexpected restls-opts: %#v", proxy["restls-opts"])
	}
	if _, ok := proxy["jls-opts"].(map[string]interface{}); !ok {
		t.Fatalf("missing jls-opts: %#v", proxy)
	}
}

func TestNormalizeMihomoSSPluginOptsUsesOfficialHyphenKeys(t *testing.T) {
	opts := normalizeMihomoSSPluginOpts(map[string]interface{}{
		"host":          "example.com",
		"version_hint":  "tls13",
		"restls_script": "300?100",
	})
	values, ok := opts.(map[string]interface{})
	if !ok || values["version-hint"] != "tls13" || values["restls-script"] != "300?100" {
		t.Fatalf("unexpected Shadowsocks plugin opts: %#v", opts)
	}
}

func TestFilterMihomoJSONUnsupportedOutboundsExpandsShadowTLSAndRemovesUnsupportedWrappers(t *testing.T) {
	outbounds := []map[string]interface{}{
		{"type": "vless", "tag": "vless-shadowtls", "tls": map[string]interface{}{
			"enabled":         true,
			"shadow_tls_opts": map[string]interface{}{"version": 3, "password": "p"},
		}},
		{"type": "vless", "tag": "vless-wrapper", "tls": map[string]interface{}{
			"enabled":     true,
			"restls_opts": map[string]interface{}{"password": "p", "version_hint": "tls13"},
		}},
		{
			"type":               "shadowsocks",
			"tag":                "ss-shadowtls",
			"server":             "203.0.113.10",
			"server_port":        443,
			"method":             "2022-blake3-aes-128-gcm",
			"password":           "ss-pass",
			"plugin":             "shadow-tls",
			"plugin_opts":        map[string]interface{}{"host": "addons.mozilla.org", "version": 3, "password": "shadow-pass"},
			"client_fingerprint": "chrome",
		},
		{"type": "shadowsocks", "tag": "ss-wrapper", "plugin": "jls", "plugin_opts": map[string]interface{}{"host": "edge.example.com"}},
		{"type": "trojan", "tag": "trojan-tls", "tls": map[string]interface{}{"enabled": true}},
	}
	tags := []string{"vless-shadowtls", "vless-wrapper", "ss-shadowtls", "ss-wrapper", "trojan-tls"}
	filterMihomoJSONUnsupportedOutbounds(&outbounds, &tags)
	if len(outbounds) != 3 {
		t.Fatalf("unexpected filtered outbounds: %#v", outbounds)
	}
	ssOutbound := outbounds[0]
	if ssOutbound["type"] != "shadowsocks" || ssOutbound["tag"] != "ss-shadowtls" || ssOutbound["detour"] != "ss-shadowtls-out" {
		t.Fatalf("unexpected Shadowsocks detour outbound: %#v", ssOutbound)
	}
	if _, exists := ssOutbound["plugin"]; exists {
		t.Fatalf("sing-box Shadowsocks outbound must not keep the Mihomo plugin field: %#v", ssOutbound)
	}
	shadowTLSOutbound := outbounds[1]
	if shadowTLSOutbound["type"] != "shadowtls" || shadowTLSOutbound["tag"] != "ss-shadowtls-out" || shadowTLSOutbound["password"] != "shadow-pass" {
		t.Fatalf("unexpected ShadowTLS outbound: %#v", shadowTLSOutbound)
	}
	tls, ok := shadowTLSOutbound["tls"].(map[string]interface{})
	if !ok || tls["server_name"] != "addons.mozilla.org" {
		t.Fatalf("unexpected ShadowTLS TLS options: %#v", shadowTLSOutbound["tls"])
	}
	utls, ok := tls["utls"].(map[string]interface{})
	if !ok || utls["fingerprint"] != "chrome" {
		t.Fatalf("unexpected ShadowTLS uTLS options: %#v", tls["utls"])
	}
	if outbounds[2]["tag"] != "trojan-tls" {
		t.Fatalf("ordinary TLS outbound should remain available: %#v", outbounds[2])
	}
	if len(tags) != 2 || tags[0] != "ss-shadowtls" || tags[1] != "trojan-tls" {
		t.Fatalf("unexpected filtered tags: %#v", tags)
	}
}
