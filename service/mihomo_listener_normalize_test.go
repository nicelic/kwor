package service

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/alireza0/s-ui/database/model"
)

func TestNormalizeMihomoUsersForMapProtocols(t *testing.T) {
	tests := []struct {
		name         string
		inboundType  string
		identityKeys []string
		users        []string
		want         map[string]string
	}{
		{
			name:         "hysteria2 uses username or legacy name as map key",
			inboundType:  "hysteria2",
			identityKeys: []string{"username", "name"},
			users: []string{
				`{"name":"alice","password":"pw-1"}`,
				`{"username":"bob","password":"pw-2"}`,
			},
			want: map[string]string{
				"alice": "pw-1",
				"bob":   "pw-2",
			},
		},
		{
			name:         "tuic uses uuid as map key",
			inboundType:  "tuic",
			identityKeys: []string{"uuid"},
			users: []string{
				`{"uuid":"00000000-0000-0000-0000-000000000001","password":"pw-tuic","name":"ignored"}`,
			},
			want: map[string]string{
				"00000000-0000-0000-0000-000000000001": "pw-tuic",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := normalizeMihomoUsersForMap(tt.inboundType, tt.users, tt.identityKeys)
			if err != nil {
				t.Fatalf("normalizeMihomoUsersForMap() error = %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("normalizeMihomoUsersForMap() = %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestNormalizeMihomoUsersForMapRejectsUnsupportedLegacyIdentityFallback(t *testing.T) {
	if _, err := normalizeMihomoUsersForMap("hysteria2", []string{
		`{"uuid":"00000000-0000-0000-0000-000000000001","password":"pw-1"}`,
	}, []string{"username", "name"}); err == nil {
		t.Fatalf("expected hysteria2 uuid-only identity to be rejected")
	}

	if _, err := normalizeMihomoUsersForMap("tuic", []string{
		`{"username":"alice","password":"pw-tuic"}`,
	}, []string{"uuid"}); err == nil {
		t.Fatalf("expected tuic username-only identity to be rejected")
	}
}

func TestNormalizeMihomoUsersForListProtocols(t *testing.T) {
	vmessUsers, err := normalizeMihomoUsersForList("vmess", []string{
		`{"name":"alice","uuid":"11111111-1111-1111-1111-111111111111","alterId":0}`,
	}, map[string]interface{}{"tls": map[string]interface{}{"enabled": true}})
	if err != nil {
		t.Fatalf("normalizeMihomoUsersForList(vmess) error = %v", err)
	}

	var vmessUser map[string]interface{}
	if err := json.Unmarshal(vmessUsers[0], &vmessUser); err != nil {
		t.Fatalf("unmarshal vmess user failed: %v", err)
	}
	if got := vmessUser["username"]; got != "alice" {
		t.Fatalf("vmess username = %#v, want %q", got, "alice")
	}
	if _, exists := vmessUser["name"]; exists {
		t.Fatalf("vmess user still contains legacy name field: %#v", vmessUser)
	}

	vlessUsers, err := normalizeMihomoUsersForList("vless", []string{
		`{"name":"carol","uuid":"22222222-2222-2222-2222-222222222222","flow":"xtls-rprx-vision"}`,
	}, map[string]interface{}{})
	if err != nil {
		t.Fatalf("normalizeMihomoUsersForList(vless) error = %v", err)
	}

	var vlessUser map[string]interface{}
	if err := json.Unmarshal(vlessUsers[0], &vlessUser); err != nil {
		t.Fatalf("unmarshal vless user failed: %v", err)
	}
	if got := vlessUser["username"]; got != "carol" {
		t.Fatalf("vless username = %#v, want %q", got, "carol")
	}
	if _, exists := vlessUser["flow"]; exists {
		t.Fatalf("vless user still contains flow without tls: %#v", vlessUser)
	}

	trustTunnelUsers, err := normalizeMihomoUsersForList("trusttunnel", []string{
		`{"uuid":"tt-secret"}`,
	}, map[string]interface{}{})
	if err != nil {
		t.Fatalf("normalizeMihomoUsersForList(trusttunnel) error = %v", err)
	}

	var trustTunnelUser map[string]interface{}
	if err := json.Unmarshal(trustTunnelUsers[0], &trustTunnelUser); err != nil {
		t.Fatalf("unmarshal trusttunnel user failed: %v", err)
	}
	if got := trustTunnelUser["username"]; got != "tt-secret" {
		t.Fatalf("trusttunnel username = %#v, want %q", got, "tt-secret")
	}
	if got := trustTunnelUser["password"]; got != "tt-secret" {
		t.Fatalf("trusttunnel password = %#v, want %q", got, "tt-secret")
	}
	if _, exists := trustTunnelUser["uuid"]; exists {
		t.Fatalf("trusttunnel user should not keep uuid: %#v", trustTunnelUser)
	}
}

func TestNormalizeAnyTLSPaddingScheme(t *testing.T) {
	tests := []struct {
		name string
		raw  interface{}
		want string
	}{
		{
			name: "splits legacy comma format only on directive boundaries",
			raw:  "stop=8,0=30-30,1=100-400,2=400-500,c,500-1000,c,500-1000,3=9-9,500-1000",
			want: "stop=8\n0=30-30\n1=100-400\n2=400-500,c,500-1000,c,500-1000\n3=9-9,500-1000",
		},
		{
			name: "joins array format and trims whitespace",
			raw:  []interface{}{" stop=8 ", "0=30-30", "", " 1=100-400 "},
			want: "stop=8\n0=30-30\n1=100-400",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := normalizeAnyTLSPaddingScheme(tt.raw); got != tt.want {
				t.Fatalf("normalizeAnyTLSPaddingScheme() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNormalizeMihomoListenerPayloadCompatAcrossInboundTypes(t *testing.T) {
	t.Run("anytls converts padding scheme to listener string", func(t *testing.T) {
		listener := map[string]interface{}{
			"type": "anytls",
			"padding_scheme": []interface{}{
				"stop=8",
				"0=30-30",
				"1=100-400",
			},
		}

		normalizeMihomoListenerPayload(listener)

		if got := listener["padding-scheme"]; got != "stop=8\n0=30-30\n1=100-400" {
			t.Fatalf("padding-scheme = %#v", got)
		}
		if _, exists := listener["padding_scheme"]; exists {
			t.Fatalf("padding_scheme should be removed: %#v", listener)
		}
	})

	t.Run("hysteria2 flattens legacy inbound fields", func(t *testing.T) {
		listener := map[string]interface{}{
			"type":                    "hysteria2",
			"up_mbps":                 370,
			"down":                    "380 Mbps",
			"ignore_client_bandwidth": true,
			"max_idle_time":           "5m",
			"obfs": map[string]interface{}{
				"type":     "salamander",
				"password": "secret",
			},
			"masquerade": map[string]interface{}{
				"type":      "file",
				"directory": "/var/www",
			},
			"mihomo_hy2": map[string]interface{}{
				"initial_stream_receive_window": 1111,
				"max_connection_receive_window": 2222,
			},
			"multiplex": map[string]interface{}{
				"enabled": true,
				"padding": true,
				"brutal": map[string]interface{}{
					"enabled":   true,
					"up_mbps":   10,
					"down_mbps": 20,
				},
			},
		}

		normalizeMihomoListenerPayload(listener)

		if got := listener["up"]; got != "370 Mbps" {
			t.Fatalf("up = %#v", got)
		}
		if got := listener["down"]; got != "380 Mbps" {
			t.Fatalf("down = %#v", got)
		}
		if got := listener["ignore-client-bandwidth"]; got != true {
			t.Fatalf("ignore-client-bandwidth = %#v", got)
		}
		if got := listener["obfs"]; got != "salamander" {
			t.Fatalf("obfs = %#v", got)
		}
		if got := listener["obfs-password"]; got != "secret" {
			t.Fatalf("obfs-password = %#v", got)
		}
		if got := listener["max-idle-time"]; got != 300000 {
			t.Fatalf("max-idle-time = %#v", got)
		}
		if got := listener["masquerade"]; got != "file:///var/www" {
			t.Fatalf("masquerade = %#v", got)
		}
		if got := listener["initial-stream-receive-window"]; got != 1111 {
			t.Fatalf("initial-stream-receive-window = %#v", got)
		}
		if got := listener["max-connection-receive-window"]; got != 2222 {
			t.Fatalf("max-connection-receive-window = %#v", got)
		}
		if _, exists := listener["mihomo_hy2"]; exists {
			t.Fatalf("mihomo_hy2 should be removed: %#v", listener)
		}
		if _, exists := listener["initial_stream_receive_window"]; exists {
			t.Fatalf("legacy initial_stream_receive_window should be removed: %#v", listener)
		}
		if _, exists := listener["ignore_client_bandwidth"]; exists {
			t.Fatalf("legacy ignore_client_bandwidth should be removed: %#v", listener)
		}
		muxOption, ok := listener["mux-option"].(map[string]interface{})
		if !ok {
			t.Fatalf("mux-option missing: %#v", listener["mux-option"])
		}
		if got := muxOption["padding"]; got != true {
			t.Fatalf("mux-option.padding = %#v", got)
		}
		brutal, ok := muxOption["brutal"].(map[string]interface{})
		if !ok {
			t.Fatalf("mux-option.brutal missing: %#v", muxOption["brutal"])
		}
		if got := brutal["up"]; got != "10 Mbps" {
			t.Fatalf("mux brutal up = %#v", got)
		}
		if got := brutal["down"]; got != "20 Mbps" {
			t.Fatalf("mux brutal down = %#v", got)
		}
	})

	t.Run("snell rewrites obfs opts to kebab-case and defaults host", func(t *testing.T) {
		listener := map[string]interface{}{
			"type":    "snell",
			"version": 5,
			"udp":     true,
			"obfs_opts": map[string]interface{}{
				"mode": "tls",
			},
		}

		normalizeMihomoListenerPayload(listener)

		if got := listener["version"]; got != 5 {
			t.Fatalf("version = %#v", got)
		}
		if got := listener["udp"]; got != true {
			t.Fatalf("udp = %#v", got)
		}
		obfsOpts, ok := listener["obfs-opts"].(map[string]interface{})
		if !ok {
			t.Fatalf("obfs-opts = %#v", listener["obfs-opts"])
		}
		if got := obfsOpts["mode"]; got != "tls" {
			t.Fatalf("obfs-opts.mode = %#v", got)
		}
		if got := obfsOpts["host"]; got != "www.bing.com" {
			t.Fatalf("obfs-opts.host = %#v", got)
		}
		if _, exists := listener["obfs_opts"]; exists {
			t.Fatalf("obfs_opts should be removed: %#v", listener["obfs_opts"])
		}
	})

	t.Run("tuic renames listener fields and strips outbound-only options", func(t *testing.T) {
		listener := map[string]interface{}{
			"type":                      "tuic",
			"congestion_control":        "bbr",
			"auth_timeout":              "5s",
			"max_idle_time":             "2m",
			"max_udp_relay_packet_size": 1400,
			"heartbeat":                 "10s",
			"zero_rtt_handshake":        true,
			"network":                   "udp",
			"multiplex": map[string]interface{}{
				"enabled": true,
				"padding": true,
			},
		}

		normalizeMihomoListenerPayload(listener)

		if got := listener["congestion-controller"]; got != "bbr" {
			t.Fatalf("congestion-controller = %#v", got)
		}
		if got := listener["authentication-timeout"]; got != 5000 {
			t.Fatalf("authentication-timeout = %#v", got)
		}
		if got := listener["max-idle-time"]; got != 120000 {
			t.Fatalf("max-idle-time = %#v", got)
		}
		if got := listener["max-udp-relay-packet-size"]; got != 1400 {
			t.Fatalf("max-udp-relay-packet-size = %#v", got)
		}
		if _, exists := listener["heartbeat"]; exists {
			t.Fatalf("heartbeat should be removed: %#v", listener)
		}
		if _, exists := listener["zero_rtt_handshake"]; exists {
			t.Fatalf("zero_rtt_handshake should be removed: %#v", listener)
		}
		if _, exists := listener["network"]; exists {
			t.Fatalf("network should be removed: %#v", listener)
		}
		if _, ok := listener["mux-option"].(map[string]interface{}); !ok {
			t.Fatalf("mux-option missing: %#v", listener["mux-option"])
		}
	})

	t.Run("shadowsocks converts method and network", func(t *testing.T) {
		listener := map[string]interface{}{
			"type":    "shadowsocks",
			"method":  "2022-blake3-aes-128-gcm",
			"network": "tcp",
			"multiplex": map[string]interface{}{
				"enabled": true,
				"padding": true,
			},
		}

		normalizeMihomoListenerPayload(listener)

		if got := listener["cipher"]; got != "2022-blake3-aes-128-gcm" {
			t.Fatalf("cipher = %#v", got)
		}
		if got := listener["udp"]; got != false {
			t.Fatalf("udp = %#v", got)
		}
		if _, exists := listener["method"]; exists {
			t.Fatalf("method should be removed: %#v", listener)
		}
		if _, exists := listener["network"]; exists {
			t.Fatalf("network should be removed: %#v", listener)
		}
		if _, ok := listener["mux-option"].(map[string]interface{}); !ok {
			t.Fatalf("mux-option missing: %#v", listener["mux-option"])
		}
	})

	t.Run("shadowtls is converted to shadowsocks listener with nested shadow-tls", func(t *testing.T) {
		listener := map[string]interface{}{
			"type":         "shadowtls",
			"version":      3,
			"strict_mode":  true,
			"wildcard_sni": "authed",
			"users": []interface{}{
				map[string]interface{}{
					"name":     "alice",
					"password": "shadow-pass",
				},
			},
			"handshake": map[string]interface{}{
				"server":      "addons.mozilla.org",
				"server_port": 443,
				"detour":      "handshake-proxy",
			},
			"handshake_for_server_name": map[string]interface{}{
				"edge.example.com": map[string]interface{}{
					"server":      "edge.example.com",
					"server_port": 8443,
					"detour":      "edge-proxy",
				},
			},
			"ss_config": map[string]interface{}{
				"method":   "2022-blake3-aes-128-gcm",
				"password": "ss-pass",
				"network":  "tcp",
				"multiplex": map[string]interface{}{
					"enabled": true,
					"padding": true,
				},
			},
		}

		normalizeMihomoListenerPayload(listener)

		if got := listener["type"]; got != "shadowsocks" {
			t.Fatalf("type = %#v", got)
		}
		if got := listener["cipher"]; got != "2022-blake3-aes-128-gcm" {
			t.Fatalf("cipher = %#v", got)
		}
		if got := listener["password"]; got != "ss-pass" {
			t.Fatalf("password = %#v", got)
		}
		if _, exists := listener["udp"]; exists {
			t.Fatalf("udp should be omitted for shadowtls inbound ss_config.network: %#v", listener["udp"])
		}
		if _, ok := listener["mux-option"].(map[string]interface{}); !ok {
			t.Fatalf("mux-option missing: %#v", listener["mux-option"])
		}

		shadowTLS, ok := listener["shadow-tls"].(map[string]interface{})
		if !ok {
			t.Fatalf("shadow-tls = %#v", listener["shadow-tls"])
		}
		if got := shadowTLS["enable"]; got != true {
			t.Fatalf("shadow-tls.enable = %#v", got)
		}
		if got := shadowTLS["version"]; got != 3 {
			t.Fatalf("shadow-tls.version = %#v", got)
		}
		if _, exists := shadowTLS["strict-mode"]; exists {
			t.Fatalf("shadow-tls.strict-mode should be omitted: %#v", shadowTLS["strict-mode"])
		}
		if _, exists := shadowTLS["wildcard-sni"]; exists {
			t.Fatalf("shadow-tls.wildcard-sni should be omitted: %#v", shadowTLS["wildcard-sni"])
		}
		shadowUsers, ok := shadowTLS["users"].([]interface{})
		if !ok || len(shadowUsers) != 1 {
			t.Fatalf("shadow-tls.users = %#v", shadowTLS["users"])
		}
		handshake, ok := shadowTLS["handshake"].(map[string]interface{})
		if !ok {
			t.Fatalf("shadow-tls.handshake = %#v", shadowTLS["handshake"])
		}
		if got := handshake["dest"]; got != "addons.mozilla.org:443" {
			t.Fatalf("shadow-tls.handshake.dest = %#v", got)
		}
		if _, exists := handshake["proxy"]; exists {
			t.Fatalf("shadow-tls.handshake.proxy should be omitted: %#v", handshake["proxy"])
		}
		if _, exists := shadowTLS["handshake-for-server-name"]; exists {
			t.Fatalf("shadow-tls.handshake-for-server-name should be omitted: %#v", shadowTLS["handshake-for-server-name"])
		}

		for _, key := range []string{"version", "users", "handshake", "handshake_for_server_name", "strict_mode", "wildcard_sni", "ss_config"} {
			if _, exists := listener[key]; exists {
				t.Fatalf("%s should be removed: %#v", key, listener[key])
			}
		}
	})

	t.Run("shadowtls v2 keeps outer password separate from ss password", func(t *testing.T) {
		listener := map[string]interface{}{
			"type":     "shadowtls",
			"version":  2,
			"password": "shadow-pass",
			"handshake": map[string]interface{}{
				"server":      "addons.mozilla.org",
				"server_port": 443,
			},
			"ss_config": map[string]interface{}{
				"method":   "2022-blake3-aes-128-gcm",
				"password": "ss-pass",
			},
		}

		normalizeMihomoListenerPayload(listener)

		if got := listener["password"]; got != "ss-pass" {
			t.Fatalf("listener password = %#v", got)
		}
		shadowTLS, ok := listener["shadow-tls"].(map[string]interface{})
		if !ok {
			t.Fatalf("shadow-tls = %#v", listener["shadow-tls"])
		}
		if got := shadowTLS["password"]; got != "shadow-pass" {
			t.Fatalf("shadow-tls password = %#v", got)
		}
	})

	t.Run("vmess converts transport and synthesizes users", func(t *testing.T) {
		listener := map[string]interface{}{
			"type":     "vmess",
			"uuid":     "11111111-1111-1111-1111-111111111111",
			"username": "alice",
			"alter_id": 1,
			"transport": map[string]interface{}{
				"type": "ws",
				"path": "/ws",
			},
			"multiplex": map[string]interface{}{
				"enabled": true,
				"padding": true,
			},
		}

		normalizeMihomoListenerPayload(listener)

		if got := listener["ws-path"]; got != "/ws" {
			t.Fatalf("ws-path = %#v", got)
		}
		users, ok := listener["users"].([]interface{})
		if !ok || len(users) != 1 {
			t.Fatalf("users = %#v", listener["users"])
		}
		user, ok := users[0].(map[string]interface{})
		if !ok {
			t.Fatalf("user = %#v", users[0])
		}
		if got := user["uuid"]; got != "11111111-1111-1111-1111-111111111111" {
			t.Fatalf("user uuid = %#v", got)
		}
		if got := user["username"]; got != "alice" {
			t.Fatalf("user username = %#v", got)
		}
		if got := user["alterId"]; got != 1 {
			t.Fatalf("user alterId = %#v", got)
		}
		if _, exists := listener["transport"]; exists {
			t.Fatalf("transport should be removed: %#v", listener)
		}
		if _, exists := listener["uuid"]; exists {
			t.Fatalf("uuid should be removed: %#v", listener)
		}
	})

	t.Run("vless synthesizes users and drops flow without tls", func(t *testing.T) {
		listener := map[string]interface{}{
			"type":     "vless",
			"uuid":     "22222222-2222-2222-2222-222222222222",
			"username": "bob",
			"flow":     "xtls-rprx-vision",
			"transport": map[string]interface{}{
				"type":         "grpc",
				"service_name": "svc",
			},
		}

		normalizeMihomoListenerPayload(listener)

		if got := listener["grpc-service-name"]; got != "svc" {
			t.Fatalf("grpc-service-name = %#v", got)
		}
		users, ok := listener["users"].([]interface{})
		if !ok || len(users) != 1 {
			t.Fatalf("users = %#v", listener["users"])
		}
		user, ok := users[0].(map[string]interface{})
		if !ok {
			t.Fatalf("user = %#v", users[0])
		}
		if _, exists := user["flow"]; exists {
			t.Fatalf("flow should be dropped without tls: %#v", user)
		}
	})

	t.Run("vless converts xhttp transport config", func(t *testing.T) {
		listener := map[string]interface{}{
			"type": "vless",
			"uuid": "22222222-2222-2222-2222-222222222222",
			"transport": map[string]interface{}{
				"type":                   "xhttp",
				"path":                   "/x",
				"host":                   "example.com",
				"mode":                   "stream-up",
				"no_grpc_header":         true,
				"sc_max_each_post_bytes": 1000000,
			},
		}

		normalizeMihomoListenerPayload(listener)

		xhttpConfig, ok := listener["xhttp-config"].(map[string]interface{})
		if !ok {
			t.Fatalf("xhttp-config = %#v", listener["xhttp-config"])
		}
		if got := xhttpConfig["path"]; got != "/x" {
			t.Fatalf("xhttp-config.path = %#v", got)
		}
		if got := xhttpConfig["host"]; got != "example.com" {
			t.Fatalf("xhttp-config.host = %#v", got)
		}
		if got := xhttpConfig["mode"]; got != "stream-up" {
			t.Fatalf("xhttp-config.mode = %#v", got)
		}
		if got, _ := xhttpConfig["no-sse-header"].(bool); !got {
			t.Fatalf("xhttp-config.no-sse-header = %#v", xhttpConfig["no-sse-header"])
		}
		if got := xhttpConfig["sc-max-each-post-bytes"]; got != 1000000 {
			t.Fatalf("xhttp-config.sc-max-each-post-bytes = %#v", got)
		}
	})

	t.Run("vless helper fields generate decryption and are stripped from listener", func(t *testing.T) {
		listener := map[string]interface{}{
			"type":                                "vless",
			"uuid":                                "33333333-3333-3333-3333-333333333333",
			"vless_encryption_enabled":            true,
			"vless_encryption_auth_method":        "x25519",
			"vless_encryption_mode":               "random",
			"vless_encryption_server_rtt":         "600s",
			"vless_encryption_client_rtt":         "0rtt",
			"vless_encryption_padding":            "100-111-1111.75-0-111.50-0-3333",
			"vless_encryption_x25519_private_key": "x25519-private",
			"vless_encryption_x25519_password":    "x25519-password",
		}

		normalizeMihomoListenerPayload(listener)

		if got := listener["decryption"]; got != "mlkem768x25519plus.random.600s.100-111-1111.75-0-111.50-0-3333.x25519-private" {
			t.Fatalf("decryption = %#v", got)
		}
		for _, key := range []string{
			"vless_encryption_enabled",
			"vless_encryption_auth_method",
			"vless_encryption_mode",
			"vless_encryption_server_rtt",
			"vless_encryption_client_rtt",
			"vless_encryption_rtt",
			"vless_encryption_padding",
			"vless_encryption_x25519_private_key",
			"vless_encryption_x25519_password",
		} {
			if _, exists := listener[key]; exists {
				t.Fatalf("%s should be stripped from final listener: %#v", key, listener[key])
			}
		}
	})

	t.Run("vless helper defaults server rtt to 0s when not provided", func(t *testing.T) {
		listener := map[string]interface{}{
			"type":                                "vless",
			"uuid":                                "33333333-3333-3333-3333-333333333333",
			"vless_encryption_enabled":            true,
			"vless_encryption_auth_method":        "x25519",
			"vless_encryption_mode":               "random",
			"vless_encryption_padding":            "100-111-1111.75-0-111.50-0-3333",
			"vless_encryption_x25519_private_key": "x25519-private",
			"vless_encryption_x25519_password":    "x25519-password",
		}

		normalizeMihomoListenerPayload(listener)

		if got := listener["decryption"]; got != "mlkem768x25519plus.random.0s.100-111-1111.75-0-111.50-0-3333.x25519-private" {
			t.Fatalf("decryption = %#v", got)
		}
	})

	t.Run("vless helper toggle off removes decryption", func(t *testing.T) {
		listener := map[string]interface{}{
			"type":                     "vless",
			"decryption":               "stale-decryption",
			"vless_encryption_enabled": false,
		}

		normalizeMihomoListenerPayload(listener)

		if _, exists := listener["decryption"]; exists {
			t.Fatalf("decryption should be removed when helper toggle is off: %#v", listener["decryption"])
		}
	})

	t.Run("vless helper fields support mlkem auth method", func(t *testing.T) {
		listener := map[string]interface{}{
			"type":                         "vless",
			"uuid":                         "33333333-3333-3333-3333-333333333333",
			"vless_encryption_enabled":     true,
			"vless_encryption_auth_method": "mlkem768",
			"vless_encryption_mode":        "random",
			"vless_encryption_server_rtt":  "600s",
			"vless_encryption_client_rtt":  "0rtt",
			"vless_encryption_padding":     "100-111-1111.75-0-111.50-0-3333",
			"vless_encryption_mlkem_seed":  "mlkem-seed",
		}

		normalizeMihomoListenerPayload(listener)

		if got := listener["decryption"]; got != "mlkem768x25519plus.random.600s.100-111-1111.75-0-111.50-0-3333.mlkem-seed" {
			t.Fatalf("decryption = %#v", got)
		}
	})

	t.Run("trojan synthesizes users from top-level password", func(t *testing.T) {
		listener := map[string]interface{}{
			"type":     "trojan",
			"password": "pw-1",
			"username": "carol",
			"transport": map[string]interface{}{
				"type":         "grpc",
				"service_name": "trojan-svc",
			},
		}

		normalizeMihomoListenerPayload(listener)

		if got := listener["grpc-service-name"]; got != "trojan-svc" {
			t.Fatalf("grpc-service-name = %#v", got)
		}
		users, ok := listener["users"].([]interface{})
		if !ok || len(users) != 1 {
			t.Fatalf("users = %#v", listener["users"])
		}
		user, ok := users[0].(map[string]interface{})
		if !ok {
			t.Fatalf("user = %#v", users[0])
		}
		if got := user["password"]; got != "pw-1" {
			t.Fatalf("password = %#v", got)
		}
		if got := user["username"]; got != "carol" {
			t.Fatalf("username = %#v", got)
		}
	})

	t.Run("mieru keeps listener port single strips legacy custom bindings and defaults user hint mandatory", func(t *testing.T) {
		listener := map[string]interface{}{
			"type":          "mieru",
			"listen":        "0.0.0.0",
			"port":          10818,
			"port_bindings": "200,204,401-429,501-503",
			"transport":     "tcp",
			"users": map[string]interface{}{
				"alice": "pw-1",
			},
		}

		normalizeMihomoListenerPayload(listener)

		if got := listener["transport"]; got != "TCP" {
			t.Fatalf("transport = %#v", got)
		}
		if got := listener["port"]; got != 10818 {
			t.Fatalf("port = %#v", got)
		}
		if got := listener["user-hint-is-mandatory"]; got != true {
			t.Fatalf("user-hint-is-mandatory = %#v", got)
		}
		if _, exists := listener["port_bindings"]; exists {
			t.Fatalf("port_bindings should be removed: %#v", listener["port_bindings"])
		}
	})

	t.Run("mieru accepts legacy snake case user hint mandatory field", func(t *testing.T) {
		listener := map[string]interface{}{
			"type":                   "mieru",
			"transport":              "udp",
			"user_hint_is_mandatory": false,
		}

		normalizeMihomoListenerPayload(listener)

		if got := listener["transport"]; got != "UDP" {
			t.Fatalf("transport = %#v", got)
		}
		if got := listener["user-hint-is-mandatory"]; got != false {
			t.Fatalf("user-hint-is-mandatory = %#v", got)
		}
		if _, exists := listener["user_hint_is_mandatory"]; exists {
			t.Fatalf("user_hint_is_mandatory should be removed: %#v", listener["user_hint_is_mandatory"])
		}
	})

	t.Run("sudoku normalizes nested httpmask and preserves fallback", func(t *testing.T) {
		listener := map[string]interface{}{
			"type":                 "sudoku",
			"key":                  "\"12345678-1234-1234-1234-1234567890ab\"",
			"aead_method":          "aes-128-gcm",
			"padding_min":          2,
			"padding_max":          7,
			"table_type":           "prefer_entropy",
			"custom_table":         "\"xpvxvvpv\"",
			"custom_tables":        []interface{}{"\"xpvxvvpv\"", "vxpvxvvp"},
			"handshake_timeout":    5,
			"enable_pure_downlink": false,
			"fallback":             "\"127.0.0.1:80\"",
			"disable_http_mask":    true,
			"httpmask": map[string]interface{}{
				"disable":   false,
				"mode":      "split-stream",
				"path_root": "\"aabbcc\"",
			},
			"managed": true,
		}

		normalizeMihomoListenerPayload(listener)

		if got := listener["aead-method"]; got != "aes-128-gcm" {
			t.Fatalf("aead-method = %#v", got)
		}
		if got := listener["table-type"]; got != "prefer_entropy" {
			t.Fatalf("table-type = %#v", got)
		}
		if got := listener["custom-table"]; got != "xpvxvvpv" {
			t.Fatalf("custom-table = %#v", got)
		}
		customTables, ok := listener["custom-tables"].([]string)
		if !ok || len(customTables) != 2 || customTables[0] != "xpvxvvpv" || customTables[1] != "vxpvxvvp" {
			t.Fatalf("custom-tables = %#v", listener["custom-tables"])
		}
		if got := listener["fallback"]; got != "127.0.0.1:80" {
			t.Fatalf("fallback = %#v", got)
		}
		if got := listener["disable-http-mask"]; got != true {
			t.Fatalf("disable-http-mask = %#v", got)
		}
		httpmask, ok := listener["httpmask"].(map[string]interface{})
		if !ok {
			t.Fatalf("httpmask = %#v", listener["httpmask"])
		}
		if got := httpmask["mode"]; got != "stream" {
			t.Fatalf("httpmask.mode = %#v", got)
		}
		if got := httpmask["path_root"]; got != "aabbcc" {
			t.Fatalf("httpmask.path_root = %#v", got)
		}
		if _, exists := listener["aead_method"]; exists {
			t.Fatalf("aead_method should be removed: %#v", listener["aead_method"])
		}
		if _, exists := listener["disable_http_mask"]; exists {
			t.Fatalf("disable_http_mask should be removed: %#v", listener["disable_http_mask"])
		}
	})

	t.Run("sudoku forces entropy table type and drops invalid custom tables", func(t *testing.T) {
		listener := map[string]interface{}{
			"type":          "sudoku",
			"table_type":    "prefer_ascii",
			"custom_table":  "xpxvvpvv",
			"custom_tables": `["xpxvvpvv", "bad-layout", "vxpvxvvp"]`,
		}

		normalizeMihomoListenerPayload(listener)

		if got := listener["table-type"]; got != "prefer_entropy" {
			t.Fatalf("expected table-type prefer_entropy when custom tables are set, got %#v", got)
		}
		customTables, ok := listener["custom-tables"].([]string)
		if !ok || len(customTables) != 2 || customTables[0] != "xpxvvpvv" || customTables[1] != "vxpvxvvp" {
			t.Fatalf("custom-tables = %#v", listener["custom-tables"])
		}
	})

	t.Run("trusttunnel renames congestion controller and normalizes networks", func(t *testing.T) {
		listener := map[string]interface{}{
			"type":               "trusttunnel",
			"congestion_control": "bbr",
			"network":            []interface{}{"udp", "tcp", "udp", "invalid"},
			"users":              []interface{}{map[string]interface{}{"username": "alice", "password": "secret"}},
			"tcp_fast_open":      true,
			"udp_fragment":       true,
			"managed":            true,
			"fallback":           map[string]interface{}{"tag": "noop"},
		}

		normalizeMihomoListenerPayload(listener)

		if got := listener["congestion-controller"]; got != "bbr" {
			t.Fatalf("congestion-controller = %#v", got)
		}
		if !reflect.DeepEqual(listener["network"], []string{"udp", "tcp"}) {
			t.Fatalf("network = %#v", listener["network"])
		}
		if _, exists := listener["congestion_control"]; exists {
			t.Fatalf("congestion_control should be removed: %#v", listener)
		}
		if _, exists := listener["tcp_fast_open"]; exists {
			t.Fatalf("tcp_fast_open should be removed: %#v", listener)
		}
		if _, exists := listener["fallback"]; exists {
			t.Fatalf("fallback should be removed: %#v", listener)
		}
	})

	t.Run("tun renames device addresses and udp timeout", func(t *testing.T) {
		listener := map[string]interface{}{
			"type":           "tun",
			"interface_name": "tun0",
			"address":        []interface{}{"172.18.0.1/30", "fd00::1/126"},
			"udp_timeout":    "5m",
			"tcp_fast_open":  true,
			"udp_fragment":   true,
			"tcp_multi_path": true,
			"managed":        true,
		}

		normalizeMihomoListenerPayload(listener)

		if got := listener["device"]; got != "tun0" {
			t.Fatalf("device = %#v", got)
		}
		if got := listener["udp-timeout"]; got != 300 {
			t.Fatalf("udp-timeout = %#v", got)
		}
		if !reflect.DeepEqual(listener["inet4-address"], []string{"172.18.0.1/30"}) {
			t.Fatalf("inet4-address = %#v", listener["inet4-address"])
		}
		if !reflect.DeepEqual(listener["inet6-address"], []string{"fd00::1/126"}) {
			t.Fatalf("inet6-address = %#v", listener["inet6-address"])
		}
		if _, exists := listener["interface_name"]; exists {
			t.Fatalf("interface_name should be removed: %#v", listener)
		}
		if _, exists := listener["address"]; exists {
			t.Fatalf("address should be removed: %#v", listener)
		}
		if _, exists := listener["tcp_fast_open"]; exists {
			t.Fatalf("tcp_fast_open should be removed: %#v", listener)
		}
	})

	t.Run("tproxy converts network to udp flag", func(t *testing.T) {
		listener := map[string]interface{}{
			"type":    "tproxy",
			"network": "udp",
		}

		normalizeMihomoListenerPayload(listener)

		if got := listener["udp"]; got != true {
			t.Fatalf("udp = %#v", got)
		}
		if _, exists := listener["network"]; exists {
			t.Fatalf("network should be removed: %#v", listener)
		}
	})
}

func TestBuildMihomoListenerNormalizesPayload(t *testing.T) {
	inbound := model.MihomoInbound{
		Type: "redirect",
		Tag:  "redir-in",
	}
	payload := map[string]interface{}{
		"type":        "redirect",
		"listen":      "0.0.0.0",
		"listen_port": 1080,
		"tls": map[string]interface{}{
			"certificate": []interface{}{
				"-----BEGIN CERTIFICATE-----",
				"LINE",
				"-----END CERTIFICATE-----",
			},
			"key": []interface{}{
				"-----BEGIN PRIVATE KEY-----",
				"KEY",
				"-----END PRIVATE KEY-----",
			},
			"ech": map[string]interface{}{
				"enabled": true,
				"key": []interface{}{
					"-----BEGIN ECH KEYS-----",
					"ECH",
					"-----END ECH KEYS-----",
				},
			},
		},
	}

	listener := buildMihomoListener(inbound, payload, mihomoInboundRouteRef{RuleName: "redir-in"})
	if got := listener["type"]; got != "redir" {
		t.Fatalf("listener type = %#v, want %q", got, "redir")
	}
	if got := listener["port"]; got != 1080 {
		t.Fatalf("listener port = %#v, want %d", got, 1080)
	}
	if got := listener["certificate"]; got != "-----BEGIN CERTIFICATE-----\nLINE\n-----END CERTIFICATE-----" {
		t.Fatalf("listener certificate = %#v", got)
	}
	if got := listener["private-key"]; got != "-----BEGIN PRIVATE KEY-----\nKEY\n-----END PRIVATE KEY-----" {
		t.Fatalf("listener private-key = %#v", got)
	}
	if got := listener["ech-key"]; got != "-----BEGIN ECH KEYS-----\nECH\n-----END ECH KEYS-----" {
		t.Fatalf("listener ech-key = %#v", got)
	}
	if _, exists := listener["tls"]; exists {
		t.Fatalf("listener still contains non-standard tls block: %#v", listener["tls"])
	}
}

func TestBuildMihomoListenerNormalizesSnellObfsOpts(t *testing.T) {
	inbound := model.MihomoInbound{
		Type: "snell",
		Tag:  "snell-in",
	}
	payload := map[string]interface{}{
		"type":        "snell",
		"listen":      "::",
		"listen_port": 28088,
		"psk":         "shared-pass",
		"version":     5,
		"obfs_opts": map[string]interface{}{
			"mode": "tls",
		},
	}

	listener := buildMihomoListener(inbound, payload, mihomoInboundRouteRef{RuleName: "snell-in"})

	if got := listener["port"]; got != 28088 {
		t.Fatalf("listener port = %#v, want %d", got, 28088)
	}
	if got := listener["psk"]; got != "shared-pass" {
		t.Fatalf("listener psk = %#v", got)
	}
	obfsOpts, ok := listener["obfs-opts"].(map[string]interface{})
	if !ok {
		t.Fatalf("listener obfs-opts = %#v", listener["obfs-opts"])
	}
	if got := obfsOpts["mode"]; got != "tls" {
		t.Fatalf("listener obfs-opts.mode = %#v", got)
	}
	if got := obfsOpts["host"]; got != "www.bing.com" {
		t.Fatalf("listener obfs-opts.host = %#v", got)
	}
	if _, exists := listener["obfs_opts"]; exists {
		t.Fatalf("listener should not keep obfs_opts: %#v", listener["obfs_opts"])
	}
}

func TestBuildMihomoListenerRealityConfigSupportsDurationString(t *testing.T) {
	inbound := model.MihomoInbound{
		Type: "vless",
		Tag:  "reality-in",
	}
	payload := map[string]interface{}{
		"type":        "vless",
		"listen":      "::",
		"listen_port": 443,
		"tls": map[string]interface{}{
			"server_name": "example.com",
			"reality": map[string]interface{}{
				"enabled":             true,
				"private_key":         "priv-key",
				"short_id":            []interface{}{"abcd"},
				"max_time_difference": "1m",
				"handshake": map[string]interface{}{
					"server":      "addons.mozilla.org",
					"server_port": 443,
				},
			},
		},
	}

	listener := buildMihomoListener(inbound, payload, mihomoInboundRouteRef{RuleName: "reality-in"})
	realityConfig, ok := listener["reality-config"].(map[string]interface{})
	if !ok {
		t.Fatalf("reality-config = %#v", listener["reality-config"])
	}
	if got := realityConfig["dest"]; got != "addons.mozilla.org:443" {
		t.Fatalf("dest = %#v", got)
	}
	if got := realityConfig["private-key"]; got != "priv-key" {
		t.Fatalf("private-key = %#v", got)
	}
	if !reflect.DeepEqual(realityConfig["short-id"], []string{"abcd"}) {
		t.Fatalf("short-id = %#v", realityConfig["short-id"])
	}
	if !reflect.DeepEqual(realityConfig["server-names"], []string{"example.com"}) {
		t.Fatalf("server-names = %#v", realityConfig["server-names"])
	}
	if got := realityConfig["max-time-difference"]; got != 60000000 {
		t.Fatalf("max-time-difference = %#v", got)
	}
}

func TestBuildMihomoListenerMergesHysteria2OutJSONCompatFields(t *testing.T) {
	inbound := model.MihomoInbound{
		Type: "hysteria2",
		Tag:  "hy2-in",
		OutJson: json.RawMessage(`{
  "mihomo_hy2": {
    "initial_stream_receive_window": 1111,
    "max_stream_receive_window": 2222,
    "initial_connection_receive_window": 3333,
    "max_connection_receive_window": 4444
  }
}`),
	}
	payload := map[string]interface{}{
		"type":        "hysteria2",
		"listen":      "::",
		"listen_port": 8443,
	}

	listener := buildMihomoListener(inbound, payload, mihomoInboundRouteRef{RuleName: "hy2-in"})

	if got, ok := toInt(listener["initial-stream-receive-window"]); !ok || got != 1111 {
		t.Fatalf("initial-stream-receive-window = %#v", listener["initial-stream-receive-window"])
	}
	if got, ok := toInt(listener["max-stream-receive-window"]); !ok || got != 2222 {
		t.Fatalf("max-stream-receive-window = %#v", listener["max-stream-receive-window"])
	}
	if got, ok := toInt(listener["initial-connection-receive-window"]); !ok || got != 3333 {
		t.Fatalf("initial-connection-receive-window = %#v", listener["initial-connection-receive-window"])
	}
	if got, ok := toInt(listener["max-connection-receive-window"]); !ok || got != 4444 {
		t.Fatalf("max-connection-receive-window = %#v", listener["max-connection-receive-window"])
	}
}

func TestBuildMihomoListenerOmitsUnsetHysteria2Bandwidth(t *testing.T) {
	inbound := model.MihomoInbound{
		Type: "hysteria2",
		Tag:  "hy2-zero-bandwidth",
	}
	payload := map[string]interface{}{
		"type":             "hysteria2",
		"listen":           "::",
		"listen_port":      8443,
		"server_up_mbps":   0,
		"server_down_mbps": "",
	}

	listener := buildMihomoListener(inbound, payload, mihomoInboundRouteRef{RuleName: "hy2-zero-bandwidth"})

	if _, exists := listener["up"]; exists {
		t.Fatalf("expected up to be omitted when listener bandwidth is unset, got %#v", listener["up"])
	}
	if _, exists := listener["down"]; exists {
		t.Fatalf("expected down to be omitted when listener bandwidth is unset, got %#v", listener["down"])
	}
}

func TestRenderMihomoRoutesSkipsUnreachableTerminalMatch(t *testing.T) {
	route := map[string]interface{}{
		"final": "DIRECT",
		"rules": []interface{}{
			map[string]interface{}{
				"action":   "route",
				"inbound":  []interface{}{"hysteria2-28012"},
				"outbound": "hy2_tg",
			},
		},
	}

	targets := &mihomoProxyConversionResult{
		SupportedTags: map[string]struct{}{
			"DIRECT": {},
			"hy2_tg": {},
		},
		DirectTags: map[string]struct{}{},
	}

	inboundRefs := map[string]mihomoInboundRouteRef{
		"hysteria2-28012": {
			RuleName:      "hysteria2-28012",
			DefaultTarget: "DIRECT",
		},
	}

	result := renderMihomoRoutes(route, map[string]struct{}{}, nil, targets, "", inboundRefs, nil)

	if !reflect.DeepEqual(result.Rules, []string{"MATCH,DIRECT"}) {
		t.Fatalf("global rules = %#v, want %#v", result.Rules, []string{"MATCH,DIRECT"})
	}
	if !reflect.DeepEqual(result.SubRules["hysteria2-28012"], []string{"MATCH,hy2_tg"}) {
		t.Fatalf("sub-rules = %#v, want %#v", result.SubRules["hysteria2-28012"], []string{"MATCH,hy2_tg"})
	}
}

func TestBuildMihomoListenerBindsRuleNameToGeneratedSubRules(t *testing.T) {
	inbound := model.MihomoInbound{
		Tag:  "hysteria2-28012",
		Type: "hysteria2",
	}
	payload := map[string]interface{}{
		"type":        "hysteria2",
		"listen":      "::",
		"listen_port": 28012,
	}
	ref := mihomoInboundRouteRef{
		RuleName:      "hysteria2-28012",
		DefaultTarget: "DIRECT",
	}

	listener := buildMihomoListener(inbound, payload, ref)
	if got := listener["rule"]; got != "hysteria2-28012" {
		t.Fatalf("listener rule = %#v, want %q", got, "hysteria2-28012")
	}

	route := map[string]interface{}{
		"final": "DIRECT",
		"rules": []interface{}{
			map[string]interface{}{
				"action":   "route",
				"inbound":  []interface{}{"hysteria2-28012"},
				"outbound": "hy2_tg",
			},
		},
	}
	targets := &mihomoProxyConversionResult{
		SupportedTags: map[string]struct{}{
			"DIRECT": {},
			"hy2_tg": {},
		},
	}
	inboundRefs := map[string]mihomoInboundRouteRef{
		ref.RuleName: ref,
	}

	result := renderMihomoRoutes(route, map[string]struct{}{}, nil, targets, "", inboundRefs, nil)
	if !reflect.DeepEqual(result.SubRules[ref.RuleName], []string{"MATCH,hy2_tg"}) {
		t.Fatalf("sub-rules[%q] = %#v", ref.RuleName, result.SubRules[ref.RuleName])
	}
}

func TestBuildMihomoInboundRouteRef_UsesNativeListenerProxyForDetour(t *testing.T) {
	inbound := model.MihomoInbound{
		Tag:     "hysteria2-28012",
		Type:    "hysteria2",
		Options: json.RawMessage(`{"detour":"hy2_tg"}`),
	}
	targets := &mihomoProxyConversionResult{
		SupportedTags: map[string]struct{}{
			"DIRECT": {},
			"hy2_tg": {},
		},
	}

	ref, err := buildMihomoInboundRouteRef(inbound, targets, "DIRECT")
	if err != nil {
		t.Fatalf("buildMihomoInboundRouteRef returned error: %v", err)
	}
	if ref.RuleName != "" {
		t.Fatalf("expected detoured listener to skip sub-rule binding, got %#v", ref.RuleName)
	}
	if ref.ProxyTarget != "hy2_tg" {
		t.Fatalf("expected proxy target hy2_tg, got %#v", ref.ProxyTarget)
	}
	if ref.DefaultTarget != "DIRECT" {
		t.Fatalf("expected default target DIRECT, got %#v", ref.DefaultTarget)
	}

	payload := map[string]interface{}{
		"type":        "hysteria2",
		"listen":      "::",
		"listen_port": 28012,
	}
	listener := buildMihomoListener(inbound, payload, ref)
	if got := listener["proxy"]; got != "hy2_tg" {
		t.Fatalf("listener proxy = %#v, want %q", got, "hy2_tg")
	}
	if _, exists := listener["rule"]; exists {
		t.Fatalf("detoured listener should not emit rule binding: %#v", listener["rule"])
	}
}

func TestRenderMihomoRoutes_ReportsInvalidRuleTarget(t *testing.T) {
	route := map[string]interface{}{
		"final": "DIRECT",
		"rules": []interface{}{
			map[string]interface{}{
				"action":   "route",
				"outbound": "missing-node",
			},
		},
	}
	targets := &mihomoProxyConversionResult{
		SupportedTags: map[string]struct{}{
			"DIRECT": {},
		},
	}

	result := renderMihomoRoutes(route, nil, nil, targets, "", nil, nil)
	if len(result.ValidationErrs) != 1 {
		t.Fatalf("expected one validation error, got %#v", result.ValidationErrs)
	}
	if result.ValidationErrs[0] != `route rule references unknown outbound "missing-node"` {
		t.Fatalf("unexpected validation error: %#v", result.ValidationErrs[0])
	}
}

func TestBuildMihomoInboundRouteRef_RejectsUnknownDetourTarget(t *testing.T) {
	inbound := model.MihomoInbound{
		Tag:     "hysteria2-28012",
		Type:    "hysteria2",
		Options: json.RawMessage(`{"detour":"missing-node"}`),
	}
	targets := &mihomoProxyConversionResult{
		SupportedTags: map[string]struct{}{
			"DIRECT": {},
		},
	}

	if _, err := buildMihomoInboundRouteRef(inbound, targets, "DIRECT"); err == nil {
		t.Fatal("expected buildMihomoInboundRouteRef to fail for unknown detour target")
	}
}

func TestBuildMihomoRuleProviders_NormalizesUnsupportedMrsClassicalBehavior(t *testing.T) {
	providers, tags := buildMihomoRuleProviders([]interface{}{
		map[string]interface{}{
			"tag":      "rs-a",
			"type":     "file",
			"path":     "./rules.mrs",
			"format":   "mrs",
			"behavior": "classical",
		},
	}, nil)

	if _, ok := tags["rs-a"]; !ok {
		t.Fatalf("expected provider tag to be present: %#v", tags)
	}

	provider, ok := providers["rs-a"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected provider map, got %#v", providers["rs-a"])
	}
	if got, _ := provider["format"].(string); got != "mrs" {
		t.Fatalf("provider format = %#v, want %q", provider["format"], "mrs")
	}
	if got, _ := provider["behavior"].(string); got != "domain" {
		t.Fatalf("provider behavior = %#v, want %q", provider["behavior"], "domain")
	}
}

func TestBuildMihomoRuleProviders_SupportsInlinePayload(t *testing.T) {
	providers, tags := buildMihomoRuleProviders([]interface{}{
		map[string]interface{}{
			"tag":      "rs-inline",
			"type":     "inline",
			"behavior": "domain",
			"payload":  []interface{}{".example.com", "api.example.com"},
		},
	}, nil)

	if _, ok := tags["rs-inline"]; !ok {
		t.Fatalf("expected inline provider tag to be present: %#v", tags)
	}

	provider, ok := providers["rs-inline"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected inline provider map, got %#v", providers["rs-inline"])
	}
	if got, _ := provider["type"].(string); got != "inline" {
		t.Fatalf("provider type = %#v, want %q", provider["type"], "inline")
	}
	if got, _ := provider["behavior"].(string); got != "domain" {
		t.Fatalf("provider behavior = %#v, want %q", provider["behavior"], "domain")
	}
	payload, ok := provider["payload"].([]string)
	if !ok || !reflect.DeepEqual(payload, []string{".example.com", "api.example.com"}) {
		t.Fatalf("provider payload = %#v", provider["payload"])
	}
	if _, exists := provider["format"]; exists {
		t.Fatalf("inline provider should not emit format: %#v", provider["format"])
	}
}

func TestRenderMihomoRoutes_RejectsUnknownRuleSetProviderWithoutFallbackMatch(t *testing.T) {
	route := map[string]interface{}{
		"final": "DIRECT",
		"rules": []interface{}{
			map[string]interface{}{
				"action":   "route",
				"outbound": "proxy",
				"rule_set": []interface{}{"missing-rs"},
			},
		},
	}
	targets := &mihomoProxyConversionResult{
		SupportedTags: map[string]struct{}{
			"DIRECT": {},
			"proxy":  {},
		},
	}

	result := renderMihomoRoutes(route, map[string]struct{}{}, nil, targets, "", nil, nil)
	if len(result.ValidationErrs) != 1 {
		t.Fatalf("expected one validation error, got %#v", result.ValidationErrs)
	}
	if got := result.ValidationErrs[0]; got != `route rule references unknown rule_set provider(s): missing-rs` {
		t.Fatalf("unexpected validation error: %#v", got)
	}
	if len(result.Rules) != 1 || result.Rules[0] != "MATCH,DIRECT" {
		t.Fatalf("expected only terminal global MATCH, got %#v", result.Rules)
	}
}
