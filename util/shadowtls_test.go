package util

import (
	"encoding/json"
	"testing"
)

func TestBuildShadowTLSClientPair_UsesVersionSpecificPasswordSources(t *testing.T) {
	tests := []struct {
		name            string
		version         int
		clientPassword  string
		inboundPassword string
		wantPassword    string
		wantHasPassword bool
	}{
		{
			name:            "v1 omits password",
			version:         1,
			clientPassword:  "client-pass",
			inboundPassword: "inbound-pass",
			wantHasPassword: false,
		},
		{
			name:            "v2 uses inbound password",
			version:         2,
			clientPassword:  "client-pass",
			inboundPassword: "inbound-pass",
			wantPassword:    "inbound-pass",
			wantHasPassword: true,
		},
		{
			name:            "v3 uses client password",
			version:         3,
			clientPassword:  "client-pass",
			inboundPassword: "inbound-pass",
			wantPassword:    "client-pass",
			wantHasPassword: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			outJSON := map[string]interface{}{
				"type":        "shadowtls",
				"tag":         "stls-node",
				"server":      "203.0.113.10",
				"server_port": 443,
				"version":     test.version,
				"tls": map[string]interface{}{
					"server_name": "addons.mozilla.org",
				},
				"ss_config": map[string]interface{}{
					"method":       "2022-blake3-aes-128-gcm",
					"password":     "ss-pass",
					"udp_over_tcp": true,
				},
			}
			clientConfigs := map[string]interface{}{
				"shadowtls": map[string]interface{}{
					"password": test.clientPassword,
				},
			}
			inboundOptions, err := json.Marshal(map[string]interface{}{
				"password": test.inboundPassword,
			})
			if err != nil {
				t.Fatalf("json.Marshal failed: %v", err)
			}

			ssOutbound, stlsOutbound := BuildShadowTLSClientPair(outJSON, clientConfigs, inboundOptions)
			if ssOutbound == nil || stlsOutbound == nil {
				t.Fatalf("expected both shadowtls client outbounds, got ss=%#v stls=%#v", ssOutbound, stlsOutbound)
			}

			if got, _ := ssOutbound["detour"].(string); got != "stls-node-out" {
				t.Fatalf("unexpected shadowsocks detour: %v", ssOutbound["detour"])
			}
			if got, _ := ssOutbound["password"].(string); got != "ss-pass" {
				t.Fatalf("unexpected shadowsocks password: %v", ssOutbound["password"])
			}

			gotPassword, hasPassword := stlsOutbound["password"]
			if hasPassword != test.wantHasPassword {
				t.Fatalf("password presence mismatch: got %v want %v, outbound=%#v", hasPassword, test.wantHasPassword, stlsOutbound)
			}
			if test.wantHasPassword {
				if got, _ := gotPassword.(string); got != test.wantPassword {
					t.Fatalf("unexpected shadowtls password: got %q want %q", got, test.wantPassword)
				}
			}
		})
	}
}

func TestBuildShadowTLSRuntimeOutboundPairMap_RespectsMultiplexPolicy(t *testing.T) {
	raw := map[string]interface{}{
		"type":         "shadowtls",
		"tag":          "stls-node",
		"server":       "203.0.113.10",
		"wildcard_sni": "all",
		"strict_mode":  true,
		"handshake": map[string]interface{}{
			"server":      "addons.mozilla.org",
			"server_port": 443,
		},
		"ss_config": map[string]interface{}{
			"method":   "2022-blake3-aes-128-gcm",
			"password": "ss-pass",
			"multiplex": map[string]interface{}{
				"enabled": false,
			},
		},
	}

	ssOutbound, stlsOutbound := BuildShadowTLSRuntimeOutboundPairMap(raw, false)
	if ssOutbound == nil || stlsOutbound == nil {
		t.Fatalf("expected shadowtls runtime pair, got ss=%#v stls=%#v", ssOutbound, stlsOutbound)
	}
	if _, ok := ssOutbound["multiplex"]; ok {
		t.Fatalf("disabled multiplex should be dropped when preserveDisabledMultiplex=false: %#v", ssOutbound)
	}
	if _, ok := stlsOutbound["handshake"]; ok {
		t.Fatalf("shadowtls runtime outbound should not contain handshake: %#v", stlsOutbound)
	}
	if _, ok := stlsOutbound["strict_mode"]; ok {
		t.Fatalf("shadowtls runtime outbound should not contain strict_mode: %#v", stlsOutbound)
	}
	if _, ok := stlsOutbound["wildcard_sni"]; ok {
		t.Fatalf("shadowtls runtime outbound should not contain wildcard_sni: %#v", stlsOutbound)
	}

	ssOutbound, _ = BuildShadowTLSRuntimeOutboundPairMap(raw, true)
	if _, ok := ssOutbound["multiplex"]; !ok {
		t.Fatalf("disabled multiplex should be preserved when preserveDisabledMultiplex=true: %#v", ssOutbound)
	}
}

func TestBuildShadowTLSRuntimeOutboundPairMap_CopiesCommonClientFields(t *testing.T) {
	raw := map[string]interface{}{
		"type":        "shadowtls",
		"tag":         "stls-node",
		"server":      "203.0.113.10",
		"server_port": 443,
		"version":     3,
		"ss_config": map[string]interface{}{
			"method":         "2022-blake3-aes-128-gcm",
			"password":       "ss-pass",
			"udp":            false,
			"ip_version":     "ipv4-prefer",
			"routing_mark":   300,
			"tcp_fast_open":  true,
			"tcp_multi_path": true,
		},
	}

	ssOutbound, stlsOutbound := BuildShadowTLSRuntimeOutboundPairMap(raw, true)
	if ssOutbound == nil || stlsOutbound == nil {
		t.Fatalf("expected shadowtls runtime pair, got ss=%#v stls=%#v", ssOutbound, stlsOutbound)
	}
	if got, ok := ssOutbound["udp"].(bool); !ok || got {
		t.Fatalf("expected udp override false, got %#v", ssOutbound["udp"])
	}
	if got, _ := ssOutbound["ip_version"].(string); got != "ipv4-prefer" {
		t.Fatalf("unexpected ip_version: %#v", ssOutbound["ip_version"])
	}
	if got, ok := ssOutbound["routing_mark"].(int); !ok || got != 300 {
		t.Fatalf("unexpected routing_mark: %#v", ssOutbound["routing_mark"])
	}
	if got, ok := ssOutbound["tcp_fast_open"].(bool); !ok || !got {
		t.Fatalf("unexpected tcp_fast_open: %#v", ssOutbound["tcp_fast_open"])
	}
	if got, ok := ssOutbound["tcp_multi_path"].(bool); !ok || !got {
		t.Fatalf("unexpected tcp_multi_path: %#v", ssOutbound["tcp_multi_path"])
	}
}

func TestBuildShadowTLSRuntimeOutboundPairMap_CopiesNestedMihomoCommonFields(t *testing.T) {
	raw := map[string]interface{}{
		"type":        "shadowtls",
		"tag":         "stls-node",
		"server":      "203.0.113.10",
		"server_port": 443,
		"version":     3,
		"ss_config": map[string]interface{}{
			"method":   "2022-blake3-aes-128-gcm",
			"password": "ss-pass",
			"mihomo_common": map[string]interface{}{
				"udp":          false,
				"routing_mark": 9,
				"smux": map[string]interface{}{
					"enabled":   true,
					"statistic": true,
				},
			},
		},
	}

	ssOutbound, stlsOutbound := BuildShadowTLSRuntimeOutboundPairMap(raw, true)
	if ssOutbound == nil || stlsOutbound == nil {
		t.Fatalf("expected shadowtls runtime pair, got ss=%#v stls=%#v", ssOutbound, stlsOutbound)
	}
	mihomoCommon, ok := ssOutbound["mihomo_common"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected nested mihomo_common, got %#v", ssOutbound["mihomo_common"])
	}
	if got, ok := mihomoCommon["udp"].(bool); !ok || got {
		t.Fatalf("unexpected nested udp: %#v", mihomoCommon["udp"])
	}
	if got, ok := mihomoCommon["routing_mark"].(int); !ok || got != 9 {
		t.Fatalf("unexpected nested routing_mark: %#v", mihomoCommon["routing_mark"])
	}
}

func TestDeriveShadowTLSPluginHost_FallsBackToHandshakeDest(t *testing.T) {
	host := DeriveShadowTLSPluginHost(map[string]interface{}{
		"handshake": map[string]interface{}{
			"dest": "addons.mozilla.org:443",
		},
	})
	if host != "addons.mozilla.org" {
		t.Fatalf("unexpected host: %q", host)
	}
}
