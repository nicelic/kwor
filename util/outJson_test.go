package util

import (
	"encoding/json"
	"testing"

	"github.com/alireza0/s-ui/database/model"
)

func mustRawMessage(t *testing.T, m map[string]interface{}) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	return data
}

func outboundTLSMap(t *testing.T, outbound map[string]interface{}) map[string]interface{} {
	t.Helper()
	tlsRaw, ok := outbound["tls"]
	if !ok {
		t.Fatalf("outbound tls is missing")
	}
	tlsMap, ok := tlsRaw.(map[string]interface{})
	if !ok {
		t.Fatalf("outbound tls type mismatch: %T", tlsRaw)
	}
	return tlsMap
}

func TestAddTls_DefaultBehavior_AddsServerCertificate(t *testing.T) {
	tlsModel := &model.Tls{
		Server: mustRawMessage(t, map[string]interface{}{
			"enabled":     true,
			"server_name": "example.com",
			"certificate": []string{"-----BEGIN CERTIFICATE-----", "AAA", "-----END CERTIFICATE-----"},
		}),
		Client: mustRawMessage(t, map[string]interface{}{
			"insecure": true,
		}),
	}

	outbound := map[string]interface{}{}
	addTls(&outbound, tlsModel)

	tlsMap := outboundTLSMap(t, outbound)
	if _, ok := tlsMap["certificate"]; !ok {
		t.Fatalf("expected certificate to be included by default")
	}
	if _, ok := tlsMap["include_server_certificate"]; ok {
		t.Fatalf("control flag should not leak into outbound tls")
	}
}

func TestAddTls_DisabledServerCertificate_RemovesCertificateAndSHA256(t *testing.T) {
	tlsModel := &model.Tls{
		Server: mustRawMessage(t, map[string]interface{}{
			"enabled":     true,
			"server_name": "example.com",
			"certificate": []string{"-----BEGIN CERTIFICATE-----", "AAA", "-----END CERTIFICATE-----"},
		}),
		Client: mustRawMessage(t, map[string]interface{}{
			"include_server_certificate":    false,
			"include_server_fingerprint":    false,
			"insecure":                      false,
			"certificate":                   []string{"client-overridden-cert"},
			"certificate_public_key_sha256": []string{"abc"},
		}),
	}

	outbound := map[string]interface{}{}
	addTls(&outbound, tlsModel)

	tlsMap := outboundTLSMap(t, outbound)
	if _, ok := tlsMap["certificate"]; ok {
		t.Fatalf("certificate must be removed when include_server_certificate is false")
	}
	if _, ok := tlsMap["certificate_path"]; ok {
		t.Fatalf("certificate_path must be removed when include_server_certificate is false")
	}
	if _, ok := tlsMap["certificate_public_key_sha256"]; ok {
		t.Fatalf("certificate_public_key_sha256 must be removed when include_server_certificate is false")
	}
	if _, ok := tlsMap["include_server_certificate"]; ok {
		t.Fatalf("control flag should not leak into outbound tls")
	}
	if _, ok := tlsMap["include_server_fingerprint"]; ok {
		t.Fatalf("fingerprint control flag should not leak into outbound tls")
	}
}

func TestNaiveOut_RemovesServerNetworkAndPreservesClientFields(t *testing.T) {
	outbound := map[string]interface{}{
		"network":                 "udp",
		"quic":                    false,
		"quic_congestion_control": "bbr2",
		"insecure_concurrency":    0,
	}

	naiveOut(&outbound, map[string]interface{}{
		"network":                 "tcp",
		"quic_congestion_control": "reno",
	})

	if _, exists := outbound["network"]; exists {
		t.Fatalf("naive client outbound must not contain network: %#v", outbound)
	}
	if got, _ := outbound["quic"].(bool); got {
		t.Fatalf("expected quic to be preserved as false, got %#v", outbound["quic"])
	}
	if got, _ := outbound["quic_congestion_control"].(string); got != "bbr2" {
		t.Fatalf("expected client quic_congestion_control to be preserved, got %#v", outbound["quic_congestion_control"])
	}
	if got, _ := outbound["insecure_concurrency"].(int); got != 0 {
		t.Fatalf("expected insecure_concurrency to be preserved, got %#v", outbound["insecure_concurrency"])
	}
}

func TestTrustTunnelOut_MapsListenerUDPToClientProxy(t *testing.T) {
	outbound := map[string]interface{}{}

	trustTunnelOut(&outbound, map[string]interface{}{
		"network": []interface{}{"tcp", "udp"},
	})

	if got, _ := outbound["udp"].(bool); !got {
		t.Fatalf("expected udp=true from listener network, got %#v", outbound["udp"])
	}
	if _, exists := outbound["network"]; exists {
		t.Fatalf("trusttunnel client outbound must not contain network: %#v", outbound["network"])
	}
}

func TestTrustTunnelOut_PreservesExplicitClientUDPChoice(t *testing.T) {
	outbound := map[string]interface{}{
		"udp":     false,
		"network": []interface{}{"udp"},
	}

	trustTunnelOut(&outbound, map[string]interface{}{
		"network": []interface{}{"tcp", "udp"},
	})

	if got, _ := outbound["udp"].(bool); got {
		t.Fatalf("expected explicit udp=false to be preserved, got %#v", outbound["udp"])
	}
	if _, exists := outbound["network"]; exists {
		t.Fatalf("trusttunnel client outbound must not contain network: %#v", outbound["network"])
	}
}

func TestSnellOut_UsesClientVersionReuseAndInboundObfs(t *testing.T) {
	outbound := map[string]interface{}{
		"version": 4,
		"reuse":   true,
	}

	snellOut(&outbound, map[string]interface{}{
		"version": 5,
		"obfs_opts": map[string]interface{}{
			"mode": "http",
			"host": "cdn.example.com",
		},
	})

	if got, _ := outbound["version"].(int); got != 4 {
		t.Fatalf("expected client version=4 to be preserved, got %#v", outbound["version"])
	}
	if got, _ := outbound["reuse"].(bool); !got {
		t.Fatalf("expected reuse=true to be preserved, got %#v", outbound["reuse"])
	}
	obfsOpts, ok := outbound["obfs_opts"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected obfs_opts map, got %#v", outbound["obfs_opts"])
	}
	if got, _ := obfsOpts["mode"].(string); got != "http" {
		t.Fatalf("expected obfs_opts.mode=http, got %#v", obfsOpts["mode"])
	}
	if got, _ := obfsOpts["host"].(string); got != "cdn.example.com" {
		t.Fatalf("expected obfs_opts.host=cdn.example.com, got %#v", obfsOpts["host"])
	}
}

func TestSnellOut_RemovesObfsWhenModeEmpty(t *testing.T) {
	outbound := map[string]interface{}{
		"obfs_opts": map[string]interface{}{
			"mode": "tls",
			"host": "old.example.com",
		},
	}

	snellOut(&outbound, map[string]interface{}{
		"obfs_opts": map[string]interface{}{
			"mode": "",
			"host": "ignored.example.com",
		},
	})

	if _, exists := outbound["obfs_opts"]; exists {
		t.Fatalf("expected obfs_opts to be removed when mode is empty, got %#v", outbound["obfs_opts"])
	}
	if got, _ := outbound["version"].(int); got != 5 {
		t.Fatalf("expected default version=5, got %#v", outbound["version"])
	}
	if got, _ := outbound["reuse"].(bool); got {
		t.Fatalf("expected default reuse=false, got %#v", outbound["reuse"])
	}
}

func TestHysteria2Out_PreservesExplicitClientNetworkChoice(t *testing.T) {
	outbound := map[string]interface{}{
		"network":   "udp",
		"up_mbps":   100,
		"down_mbps": 200,
	}

	hysteria2Out(&outbound, map[string]interface{}{
		"bbr_profile":           "aggressive",
		"port_hop_range":        "3000:4000",
		"port_hop_interval":     "15s",
		"port_hop_interval_max": "30s",
	})

	if got, _ := outbound["network"].(string); got != "udp" {
		t.Fatalf("expected hysteria2 client network to be preserved, got %#v", outbound["network"])
	}
	if got, _ := outbound["up_mbps"].(int); got != 100 {
		t.Fatalf("expected up_mbps=100 from client out_json, got %#v", outbound["up_mbps"])
	}
	if got, _ := outbound["down_mbps"].(int); got != 200 {
		t.Fatalf("expected down_mbps=200 from client out_json, got %#v", outbound["down_mbps"])
	}
	serverPorts, ok := outbound["server_ports"].([]string)
	if !ok || len(serverPorts) == 0 {
		t.Fatalf("expected server_ports to be generated, got %#v", outbound["server_ports"])
	}
	if got, _ := outbound["hop_interval"].(string); got != "15s" {
		t.Fatalf("expected hop_interval=15s, got %#v", outbound["hop_interval"])
	}
	if got, _ := outbound["hop_interval_max"].(string); got != "30s" {
		t.Fatalf("expected hop_interval_max=30s, got %#v", outbound["hop_interval_max"])
	}
	if got, _ := outbound["bbr_profile"].(string); got != "aggressive" {
		t.Fatalf("expected bbr_profile=aggressive, got %#v", outbound["bbr_profile"])
	}
}

func TestHysteria2Out_RemovesEmptyClientNetworkChoice(t *testing.T) {
	outbound := map[string]interface{}{
		"network":          "   ",
		"bbr_profile":      "standard",
		"hop_interval_max": "20s",
	}

	hysteria2Out(&outbound, map[string]interface{}{})

	if _, exists := outbound["network"]; exists {
		t.Fatalf("expected empty hysteria2 client network to be removed, got %#v", outbound["network"])
	}
	if _, exists := outbound["bbr_profile"]; exists {
		t.Fatalf("expected bbr_profile to be removed when inbound is unset, got %#v", outbound["bbr_profile"])
	}
	if _, exists := outbound["hop_interval_max"]; exists {
		t.Fatalf("expected hop_interval_max to be removed when inbound is unset, got %#v", outbound["hop_interval_max"])
	}
}

func TestHysteria2Out_MapsBBRProfileExactly(t *testing.T) {
	cases := []string{"conservative", "standard", "aggressive"}
	for _, profile := range cases {
		outbound := map[string]interface{}{}
		hysteria2Out(&outbound, map[string]interface{}{
			"bbr_profile": profile,
		})

		if got, _ := outbound["bbr_profile"].(string); got != profile {
			t.Fatalf("profile=%s expected bbr_profile=%s, got %#v", profile, profile, outbound["bbr_profile"])
		}
	}
}

func TestHysteriaOut_UsesSharedQUICFields(t *testing.T) {
	outbound := map[string]interface{}{
		"recv_window_conn":      1111,
		"recv_window":           2222,
		"disable_mtu_discovery": true,
		"up_mbps":               100,
		"down_mbps":             200,
	}

	hysteriaOut(&outbound, map[string]interface{}{
		"stream_receive_window":      25000000,
		"connection_receive_window":  99000000,
		"max_concurrent_streams":     1024,
		"disable_path_mtu_discovery": true,
	})

	if got, _ := outbound["up_mbps"].(int); got != 100 {
		t.Fatalf("expected up_mbps=100 from client out_json, got %#v", outbound["up_mbps"])
	}
	if got, _ := outbound["down_mbps"].(int); got != 200 {
		t.Fatalf("expected down_mbps=200 from client out_json, got %#v", outbound["down_mbps"])
	}
	if got := outbound["stream_receive_window"]; got != 25000000 {
		t.Fatalf("expected stream_receive_window=25000000, got %#v", got)
	}
	if got := outbound["connection_receive_window"]; got != 99000000 {
		t.Fatalf("expected connection_receive_window=99000000, got %#v", got)
	}
	if got := outbound["max_concurrent_streams"]; got != 1024 {
		t.Fatalf("expected max_concurrent_streams=1024, got %#v", got)
	}
	if got, _ := outbound["disable_path_mtu_discovery"].(bool); !got {
		t.Fatalf("expected disable_path_mtu_discovery=true, got %#v", outbound["disable_path_mtu_discovery"])
	}
	if _, exists := outbound["recv_window_conn"]; exists {
		t.Fatalf("legacy recv_window_conn should be removed, got %#v", outbound["recv_window_conn"])
	}
	if _, exists := outbound["recv_window"]; exists {
		t.Fatalf("legacy recv_window should be removed, got %#v", outbound["recv_window"])
	}
	if _, exists := outbound["disable_mtu_discovery"]; exists {
		t.Fatalf("legacy disable_mtu_discovery should be removed, got %#v", outbound["disable_mtu_discovery"])
	}
}

func TestHysteriaOut_OmitsZeroBandwidth(t *testing.T) {
	outbound := map[string]interface{}{}

	hysteriaOut(&outbound, map[string]interface{}{
		"server_up_mbps":   10000,
		"server_down_mbps": 10000,
	})

	if _, exists := outbound["up_mbps"]; exists {
		t.Fatalf("expected up_mbps to be omitted when inbound bandwidth is zero, got %#v", outbound["up_mbps"])
	}
	if _, exists := outbound["down_mbps"]; exists {
		t.Fatalf("expected down_mbps to be omitted when inbound bandwidth is zero, got %#v", outbound["down_mbps"])
	}
}

func TestHysteria2Out_OmitsZeroBandwidth(t *testing.T) {
	outbound := map[string]interface{}{}

	hysteria2Out(&outbound, map[string]interface{}{
		"server_up_mbps":   10000,
		"server_down_mbps": 10000,
	})

	if _, exists := outbound["up_mbps"]; exists {
		t.Fatalf("expected up_mbps to be omitted when inbound bandwidth is zero, got %#v", outbound["up_mbps"])
	}
	if _, exists := outbound["down_mbps"]; exists {
		t.Fatalf("expected down_mbps to be omitted when inbound bandwidth is zero, got %#v", outbound["down_mbps"])
	}
}

func TestSudokuOut_OverwritesServerControlledFieldsAndKeepsClientHTTPMaskFields(t *testing.T) {
	outbound := map[string]interface{}{
		"key":                  "stale-key",
		"aead_method":          "none",
		"padding_min":          9,
		"padding_max":          12,
		"table_type":           "prefer_entropy",
		"custom_table":         "old-table",
		"custom_tables":        []interface{}{"old-table"},
		"enable_pure_downlink": true,
		"handshake_timeout":    30,
		"fallback":             "127.0.0.1:80",
		"disable_http_mask":    true,
		"httpmask": map[string]interface{}{
			"disable":   true,
			"mode":      "poll",
			"path_root": "old-root",
			"tls":       false,
			"host":      "mask.example.com",
			"multiplex": "auto",
		},
	}

	inbound := map[string]interface{}{
		"aead_method":          "aes-128-gcm",
		"padding_min":          2,
		"padding_max":          7,
		"table_type":           "prefer_ascii",
		"custom_table":         "xpxvvpvv",
		"custom_tables":        []interface{}{"xpxvvpvv", "vxpvxvvp"},
		"enable_pure_downlink": false,
		"httpmask": map[string]interface{}{
			"disable":   false,
			"mode":      "split-stream",
			"path_root": "server-root",
		},
	}

	sudokuOut(&outbound, inbound)

	if _, exists := outbound["key"]; exists {
		t.Fatalf("expected key to be removed from generated out_json, got %#v", outbound["key"])
	}
	if _, exists := outbound["handshake_timeout"]; exists {
		t.Fatalf("expected handshake_timeout to be removed from generated out_json, got %#v", outbound["handshake_timeout"])
	}
	if _, exists := outbound["fallback"]; exists {
		t.Fatalf("expected fallback to be removed from generated out_json, got %#v", outbound["fallback"])
	}
	if _, exists := outbound["disable_http_mask"]; exists {
		t.Fatalf("expected disable_http_mask to be removed from generated out_json, got %#v", outbound["disable_http_mask"])
	}
	if got, _ := outbound["aead_method"].(string); got != "aes-128-gcm" {
		t.Fatalf("expected aead_method from inbound, got %#v", outbound["aead_method"])
	}
	if got, _ := outbound["padding_min"].(int); got != 2 {
		t.Fatalf("expected padding_min from inbound, got %#v", outbound["padding_min"])
	}
	if got, _ := outbound["padding_max"].(int); got != 7 {
		t.Fatalf("expected padding_max from inbound, got %#v", outbound["padding_max"])
	}
	if got, _ := outbound["table_type"].(string); got != "prefer_entropy" {
		t.Fatalf("expected table_type to switch to prefer_entropy when custom table is set, got %#v", outbound["table_type"])
	}
	if got, _ := outbound["custom_table"].(string); got != "xpxvvpvv" {
		t.Fatalf("expected custom_table from inbound, got %#v", outbound["custom_table"])
	}
	customTables, ok := outbound["custom_tables"].([]string)
	if !ok || len(customTables) != 2 || customTables[0] != "xpxvvpvv" || customTables[1] != "vxpvxvvp" {
		t.Fatalf("expected custom_tables from inbound, got %#v", outbound["custom_tables"])
	}
	if got, _ := outbound["enable_pure_downlink"].(bool); got {
		t.Fatalf("expected enable_pure_downlink=false from inbound, got %#v", outbound["enable_pure_downlink"])
	}

	httpmask, ok := outbound["httpmask"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected nested httpmask, got %#v", outbound["httpmask"])
	}
	if got, _ := httpmask["disable"].(bool); got {
		t.Fatalf("expected httpmask.disable from inbound, got %#v", httpmask["disable"])
	}
	if got, _ := httpmask["mode"].(string); got != "stream" {
		t.Fatalf("expected httpmask.mode normalized from inbound, got %#v", httpmask["mode"])
	}
	if got, _ := httpmask["path_root"].(string); got != "server-root" {
		t.Fatalf("expected httpmask.path_root from inbound, got %#v", httpmask["path_root"])
	}
	if got, _ := httpmask["tls"].(bool); got {
		t.Fatalf("expected client httpmask.tls to be preserved, got %#v", httpmask["tls"])
	}
	if got, _ := httpmask["host"].(string); got != "mask.example.com" {
		t.Fatalf("expected client httpmask.host to be preserved, got %#v", httpmask["host"])
	}
	if got, _ := httpmask["multiplex"].(string); got != "auto" {
		t.Fatalf("expected client httpmask.multiplex to be preserved, got %#v", httpmask["multiplex"])
	}
}

func TestSudokuOut_DefaultsServerControlledHTTPMaskWhenInboundConfigMissing(t *testing.T) {
	outbound := map[string]interface{}{
		"httpmask": map[string]interface{}{
			"disable":   true,
			"mode":      "poll",
			"path_root": "old-root",
			"tls":       false,
			"host":      "mask.example.com",
			"multiplex": "auto",
		},
	}

	sudokuOut(&outbound, map[string]interface{}{})

	httpmask, ok := outbound["httpmask"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected nested httpmask, got %#v", outbound["httpmask"])
	}
	if got, _ := httpmask["disable"].(bool); got {
		t.Fatalf("expected missing inbound config to default disable=false, got %#v", httpmask["disable"])
	}
	if got, _ := httpmask["mode"].(string); got != "legacy" {
		t.Fatalf("expected missing inbound config to default mode=legacy, got %#v", httpmask["mode"])
	}
	if _, exists := httpmask["path_root"]; exists {
		t.Fatalf("expected stale path_root to be removed, got %#v", httpmask["path_root"])
	}
	if got, _ := httpmask["tls"].(bool); got {
		t.Fatalf("expected client httpmask.tls to stay preserved, got %#v", httpmask["tls"])
	}
	if got, _ := httpmask["host"].(string); got != "mask.example.com" {
		t.Fatalf("expected client httpmask.host to stay preserved, got %#v", httpmask["host"])
	}
	if got, _ := httpmask["multiplex"].(string); got != "auto" {
		t.Fatalf("expected client httpmask.multiplex to stay preserved, got %#v", httpmask["multiplex"])
	}
}

func TestVLESSOut_GeneratesEncryptionWhenHelperEnabled(t *testing.T) {
	outbound := map[string]interface{}{}
	inbound := map[string]interface{}{
		"vless_encryption_auth_method":        "x25519",
		"transport":                           map[string]interface{}{"type": "ws"},
		"vless_encryption_enabled":            true,
		"vless_encryption_mode":               "random",
		"vless_encryption_server_rtt":         "600s",
		"vless_encryption_client_rtt":         "0rtt",
		"vless_encryption_padding":            "100-111-1111.75-0-111.50-0-3333",
		"vless_encryption_x25519_password":    "x25519-password",
		"vless_encryption_x25519_private_key": "x25519-private",
	}

	vlessOut(&outbound, inbound)

	transport, ok := outbound["transport"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected transport to be preserved, got %#v", outbound["transport"])
	}
	if got, _ := transport["type"].(string); got != "ws" {
		t.Fatalf("transport.type = %#v", got)
	}

	encryption, ok := outbound["encryption"].(string)
	if !ok || encryption == "" {
		t.Fatalf("expected encryption to be generated, got %#v", outbound["encryption"])
	}
	if encryption != "mlkem768x25519plus.random.0rtt.100-111-1111.75-0-111.50-0-3333.x25519-password" {
		t.Fatalf("unexpected encryption value: %q", encryption)
	}
}

func TestVLESSOut_GeneratesMLKEMEncryptionWhenAuthMethodSelected(t *testing.T) {
	outbound := map[string]interface{}{}
	inbound := map[string]interface{}{
		"vless_encryption_auth_method":  "mlkem768",
		"vless_encryption_enabled":      true,
		"vless_encryption_mode":         "random",
		"vless_encryption_server_rtt":   "600s",
		"vless_encryption_client_rtt":   "0rtt",
		"vless_encryption_padding":      "100-111-1111.75-0-111.50-0-3333",
		"vless_encryption_mlkem_client": "mlkem-client",
		"vless_encryption_mlkem_seed":   "mlkem-seed",
	}

	vlessOut(&outbound, inbound)

	encryption, ok := outbound["encryption"].(string)
	if !ok || encryption == "" {
		t.Fatalf("expected encryption to be generated, got %#v", outbound["encryption"])
	}
	if encryption != "mlkem768x25519plus.random.0rtt.100-111-1111.75-0-111.50-0-3333.mlkem-client" {
		t.Fatalf("unexpected encryption value: %q", encryption)
	}
}

func TestVLESSOut_RemovesEncryptionWhenHelperDisabled(t *testing.T) {
	outbound := map[string]interface{}{
		"encryption": "stale-encryption",
	}
	inbound := map[string]interface{}{
		"vless_encryption_enabled": false,
	}

	vlessOut(&outbound, inbound)

	if _, exists := outbound["encryption"]; exists {
		t.Fatalf("expected encryption to be removed when helper is disabled, got %#v", outbound["encryption"])
	}
}

func TestVLESSOut_DefaultsClientRTTTo1RTTWhenNotProvided(t *testing.T) {
	outbound := map[string]interface{}{}
	inbound := map[string]interface{}{
		"vless_encryption_auth_method":        "x25519",
		"vless_encryption_enabled":            true,
		"vless_encryption_mode":               "random",
		"vless_encryption_padding":            "100-111-1111.75-0-111.50-0-3333",
		"vless_encryption_x25519_password":    "x25519-password",
		"vless_encryption_x25519_private_key": "x25519-private",
	}

	vlessOut(&outbound, inbound)

	encryption, ok := outbound["encryption"].(string)
	if !ok || encryption == "" {
		t.Fatalf("expected encryption to be generated, got %#v", outbound["encryption"])
	}
	if encryption != "mlkem768x25519plus.random.1rtt.100-111-1111.75-0-111.50-0-3333.x25519-password" {
		t.Fatalf("unexpected encryption value: %q", encryption)
	}
}

func TestVLESSOut_SupportsLegacyRTTFieldWhenSplitFieldsMissing(t *testing.T) {
	outbound := map[string]interface{}{}
	inbound := map[string]interface{}{
		"vless_encryption_auth_method":        "x25519",
		"vless_encryption_enabled":            true,
		"vless_encryption_mode":               "random",
		"vless_encryption_rtt":                "0rtt",
		"vless_encryption_padding":            "100-111-1111.75-0-111.50-0-3333",
		"vless_encryption_x25519_password":    "x25519-password",
		"vless_encryption_x25519_private_key": "x25519-private",
	}

	vlessOut(&outbound, inbound)

	encryption, ok := outbound["encryption"].(string)
	if !ok || encryption == "" {
		t.Fatalf("expected encryption to be generated, got %#v", outbound["encryption"])
	}
	if encryption != "mlkem768x25519plus.random.0rtt.100-111-1111.75-0-111.50-0-3333.x25519-password" {
		t.Fatalf("unexpected encryption value: %q", encryption)
	}
}

func TestSSHOut_MapsInboundFieldsToOutJSON(t *testing.T) {
	outbound := map[string]interface{}{}
	inbound := map[string]interface{}{
		"username":               "root",
		"password":               "password",
		"private_key":            "key",
		"private_key_passphrase": "key_password",
		"host_key":               []interface{}{"ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQC"},
		"host_key_algorithms":    []interface{}{"rsa"},
		"client_version":         "SSH-2.0-OpenSSH_9.0",
		"cipher":                 []interface{}{"aes128-ctr"},
		"mac":                    []interface{}{"hmac-sha2-256"},
		"kex_algorithm":          []interface{}{"curve25519-sha256"},
	}

	sshOut(&outbound, inbound)

	if got, _ := outbound["user"].(string); got != "root" {
		t.Fatalf("expected user=root, got %#v", outbound["user"])
	}
	if got, _ := outbound["username"].(string); got != "root" {
		t.Fatalf("expected username=root for compatibility, got %#v", outbound["username"])
	}
	if got, _ := outbound["password"].(string); got != "password" {
		t.Fatalf("expected password=password, got %#v", outbound["password"])
	}
	if got, _ := outbound["private_key"].(string); got != "key" {
		t.Fatalf("expected private_key=key, got %#v", outbound["private_key"])
	}
	if got, _ := outbound["private_key_passphrase"].(string); got != "key_password" {
		t.Fatalf("expected private_key_passphrase=key_password, got %#v", outbound["private_key_passphrase"])
	}
	if hostKey, ok := outbound["host_key"].([]string); !ok || len(hostKey) != 1 {
		t.Fatalf("expected host_key list, got %#v", outbound["host_key"])
	}
	if algorithms, ok := outbound["host_key_algorithms"].([]string); !ok || len(algorithms) != 1 || algorithms[0] != "rsa" {
		t.Fatalf("expected host_key_algorithms list, got %#v", outbound["host_key_algorithms"])
	}
	if got, _ := outbound["client_version"].(string); got != "SSH-2.0-OpenSSH_9.0" {
		t.Fatalf("expected client_version mapped, got %#v", outbound["client_version"])
	}
}
