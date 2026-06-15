package sub

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/hex"
	"encoding/pem"
	"math/big"
	"strings"
	"testing"
	"time"

	"gopkg.in/yaml.v3"
)

func singleProxyFromOutbound(t *testing.T, outbound map[string]interface{}) map[string]interface{} {
	t.Helper()

	svc := &ClashService{}
	outbounds := []map[string]interface{}{outbound}
	raw, err := svc.ConvertToClashMeta(&outbounds, "http://www.gstatic.com/generate_204", 300, 50, nil)
	if err != nil {
		t.Fatalf("ConvertToClashMeta failed: %v", err)
	}

	var doc map[string]interface{}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("yaml.Unmarshal failed: %v", err)
	}

	proxiesRaw, ok := doc["proxies"].([]interface{})
	if !ok || len(proxiesRaw) != 1 {
		t.Fatalf("expected one proxy, got %#v", doc["proxies"])
	}

	proxy, ok := proxiesRaw[0].(map[string]interface{})
	if !ok {
		t.Fatalf("proxy is not a map: %T", proxiesRaw[0])
	}
	return proxy
}

func asMap(t *testing.T, raw interface{}) map[string]interface{} {
	t.Helper()
	m, ok := raw.(map[string]interface{})
	if !ok {
		t.Fatalf("expected map, got %T", raw)
	}
	return m
}

func asIntValue(t *testing.T, raw interface{}) int {
	t.Helper()
	switch v := raw.(type) {
	case int:
		return v
	case int64:
		return int(v)
	default:
		t.Fatalf("expected int, got %T", raw)
	}
	return 0
}

func asStringSliceValue(t *testing.T, raw interface{}) []string {
	t.Helper()
	switch value := raw.(type) {
	case []string:
		return value
	case []interface{}:
		result := make([]string, 0, len(value))
		for _, item := range value {
			str, ok := item.(string)
			if !ok {
				t.Fatalf("expected string item, got %T", item)
			}
			result = append(result, str)
		}
		return result
	default:
		t.Fatalf("expected slice, got %T", raw)
	}
	return nil
}

func buildLeafCertificateForFingerprint(t *testing.T) ([]interface{}, string) {
	t.Helper()

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate ECDSA key: %v", err)
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "example.com"},
		NotBefore:    time.Now().Add(-1 * time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		DNSNames:     []string{"example.com"},
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		t.Fatalf("failed to create certificate: %v", err)
	}

	sum := sha256.Sum256(certDER)
	hexStr := strings.ToUpper(hex.EncodeToString(sum[:]))
	parts := make([]string, 0, len(hexStr)/2)
	for i := 0; i < len(hexStr); i += 2 {
		parts = append(parts, hexStr[i:i+2])
	}
	expectedFingerprint := strings.Join(parts, ":")

	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	pemLines := strings.Split(strings.TrimSpace(string(pemBytes)), "\n")
	rawLines := make([]interface{}, 0, len(pemLines))
	for _, line := range pemLines {
		rawLines = append(rawLines, line)
	}

	return rawLines, expectedFingerprint
}

func TestConvertToClashMeta_Hysteria2MihomoFields(t *testing.T) {
	proxy := singleProxyFromOutbound(t, map[string]interface{}{
		"type":             "hysteria2",
		"tag":              "hy2-node",
		"server":           "example.com",
		"server_port":      443,
		"password":         "pwd",
		"up_mbps":          30,
		"down_mbps":        200,
		"server_ports":     []interface{}{"443:8443", "9000"},
		"hop_interval":     "30s",
		"mihomo_fast_open": true,
		"obfs": map[string]interface{}{
			"type":     "salamander",
			"password": "obfs-pass",
		},
		"mihomo_hy2": map[string]interface{}{
			"initial_stream_receive_window":     8388608,
			"max_stream_receive_window":         8388608,
			"initial_connection_receive_window": 20971520,
			"max_connection_receive_window":     20971520,
		},
		"tls": map[string]interface{}{
			"enabled":     true,
			"server_name": "hy2.example.com",
			"insecure":    true,
			"utls": map[string]interface{}{
				"enabled":     true,
				"fingerprint": "chrome",
			},
		},
	})

	if got, _ := proxy["type"].(string); got != "hysteria2" {
		t.Fatalf("expected type hysteria2, got %v", proxy["type"])
	}
	if got, _ := proxy["ports"].(string); got != "443-8443,9000" {
		t.Fatalf("unexpected ports: %v", proxy["ports"])
	}
	if got := asIntValue(t, proxy["hop-interval"]); got != 30 {
		t.Fatalf("unexpected hop-interval: %v", got)
	}
	if got, _ := proxy["obfs"].(string); got != "salamander" {
		t.Fatalf("unexpected obfs: %v", proxy["obfs"])
	}
	if got, _ := proxy["obfs-password"].(string); got != "obfs-pass" {
		t.Fatalf("unexpected obfs-password: %v", proxy["obfs-password"])
	}
	if got := asIntValue(t, proxy["initial-stream-receive-window"]); got != 8388608 {
		t.Fatalf("unexpected initial-stream-receive-window: %v", got)
	}
	if got := asIntValue(t, proxy["max-connection-receive-window"]); got != 20971520 {
		t.Fatalf("unexpected max-connection-receive-window: %v", got)
	}
	if got, _ := proxy["sni"].(string); got != "hy2.example.com" {
		t.Fatalf("unexpected sni: %v", proxy["sni"])
	}
	if got, _ := proxy["client-fingerprint"].(string); got != "chrome" {
		t.Fatalf("unexpected client-fingerprint: %v", proxy["client-fingerprint"])
	}
	if got, _ := proxy["fast-open"].(bool); !got {
		t.Fatalf("expected fast-open=true when mihomo_fast_open=true, got %v", proxy["fast-open"])
	}
}

func TestConvertToClashMeta_Hysteria2RangeHopIntervalFormatsRangeString(t *testing.T) {
	proxy := singleProxyFromOutbound(t, map[string]interface{}{
		"type":             "hysteria2",
		"tag":              "hy2-hop-range",
		"server":           "example.com",
		"server_port":      443,
		"password":         "pwd",
		"hop_interval":     "30s",
		"hop_interval_max": "60s",
	})

	if got, _ := proxy["hop-interval"].(string); got != "30-60" {
		t.Fatalf("unexpected hop-interval: %v", got)
	}
}

func TestConvertToClashMeta_HysteriaUsesNewQUICFields(t *testing.T) {
	proxy := singleProxyFromOutbound(t, map[string]interface{}{
		"type":                  "hysteria",
		"tag":                   "hy1-new-quic",
		"server":                "example.com",
		"server_port":           443,
		"auth_str":              "pwd",
		"up_mbps":               30,
		"down_mbps":             200,
		"recv_window_conn":      25000000,
		"recv_window":           67108864,
		"disable_mtu_discovery": true,
	})

	if got := asIntValue(t, proxy["recv-window-conn"]); got != 25000000 {
		t.Fatalf("unexpected recv-window-conn: %v", got)
	}
	if got := asIntValue(t, proxy["recv-window"]); got != 67108864 {
		t.Fatalf("unexpected recv-window: %v", got)
	}
	if got, _ := proxy["disable-mtu-discovery"].(bool); !got {
		t.Fatalf("expected disable-mtu-discovery=true, got %v", proxy["disable-mtu-discovery"])
	}
}

func TestConvertToClashMeta_HysteriaMihomoFastOpenDisabled(t *testing.T) {
	proxy := singleProxyFromOutbound(t, map[string]interface{}{
		"type":             "hysteria",
		"tag":              "hy1-no-fast-open",
		"server":           "example.com",
		"server_port":      443,
		"auth_str":         "pwd",
		"up_mbps":          30,
		"down_mbps":        200,
		"mihomo_fast_open": false,
	})

	if _, exists := proxy["fast-open"]; exists {
		t.Fatalf("expected fast-open to be omitted when mihomo_fast_open=false, got %v", proxy["fast-open"])
	}
}

func TestConvertToClashMeta_HysteriaMihomoFastOpenDefaultEnabled(t *testing.T) {
	proxy := singleProxyFromOutbound(t, map[string]interface{}{
		"type":        "hysteria",
		"tag":         "hy1-fast-open-default",
		"server":      "example.com",
		"server_port": 443,
		"auth_str":    "pwd",
	})

	if got, _ := proxy["fast-open"].(bool); !got {
		t.Fatalf("expected fast-open=true by default, got %v", proxy["fast-open"])
	}
}

func TestConvertToClashMeta_HysteriaOmitsZeroBandwidth(t *testing.T) {
	proxy := singleProxyFromOutbound(t, map[string]interface{}{
		"type":        "hysteria",
		"tag":         "hy1-zero-bandwidth",
		"server":      "example.com",
		"server_port": 443,
		"auth_str":    "pwd",
		"up_mbps":     0,
		"down_mbps":   0,
	})

	if _, exists := proxy["up"]; exists {
		t.Fatalf("expected up to be omitted when zero, got %#v", proxy["up"])
	}
	if _, exists := proxy["down"]; exists {
		t.Fatalf("expected down to be omitted when zero, got %#v", proxy["down"])
	}
}

func TestConvertToClashMeta_SnellMapsPSKVersionReuseAndObfs(t *testing.T) {
	proxy := singleProxyFromOutbound(t, map[string]interface{}{
		"type":        "snell",
		"tag":         "snell-node",
		"server":      "example.com",
		"server_port": 8443,
		"psk":         "secret-pass",
		"version":     4,
		"reuse":       true,
		"obfs_opts": map[string]interface{}{
			"mode": "http",
			"host": "cdn.example.com",
		},
	})

	if got, _ := proxy["type"].(string); got != "snell" {
		t.Fatalf("expected type snell, got %v", proxy["type"])
	}
	if got, _ := proxy["psk"].(string); got != "secret-pass" {
		t.Fatalf("expected psk secret-pass, got %v", proxy["psk"])
	}
	if got := asIntValue(t, proxy["version"]); got != 4 {
		t.Fatalf("expected version 4, got %v", proxy["version"])
	}
	if got, _ := proxy["reuse"].(bool); !got {
		t.Fatalf("expected reuse=true, got %v", proxy["reuse"])
	}
	obfsOpts, ok := proxy["obfs-opts"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected obfs-opts map, got %#v", proxy["obfs-opts"])
	}
	if got, _ := obfsOpts["mode"].(string); got != "http" {
		t.Fatalf("expected obfs-opts.mode=http, got %v", obfsOpts["mode"])
	}
	if got, _ := obfsOpts["host"].(string); got != "cdn.example.com" {
		t.Fatalf("expected obfs-opts.host=cdn.example.com, got %v", obfsOpts["host"])
	}
}

func TestConvertToClashMeta_SudokuMapsHTTPMaskAndKey(t *testing.T) {
	proxy := singleProxyFromOutbound(t, map[string]interface{}{
		"type":                 "sudoku",
		"tag":                  "sudoku-node",
		"server":               "example.com",
		"server_port":          443,
		"key":                  "12345678-1234-1234-1234-1234567890ab",
		"aead_method":          "aes-128-gcm",
		"padding_min":          2,
		"padding_max":          7,
		"table_type":           "prefer_entropy",
		"custom_table":         "xpvxvvpv",
		"custom_tables":        []interface{}{"xpvxvvpv", "vxpvxvvp"},
		"enable_pure_downlink": true,
		"httpmask": map[string]interface{}{
			"disable":   false,
			"mode":      "split-stream",
			"tls":       true,
			"host":      "mask.example.com",
			"path_root": "aabbcc",
			"multiplex": "auto",
		},
	})

	if got, _ := proxy["type"].(string); got != "sudoku" {
		t.Fatalf("expected type sudoku, got %v", proxy["type"])
	}
	if got, _ := proxy["key"].(string); got != "12345678-1234-1234-1234-1234567890ab" {
		t.Fatalf("expected key to be preserved, got %#v", proxy["key"])
	}
	httpmask := asMap(t, proxy["httpmask"])
	if got, _ := httpmask["mode"].(string); got != "stream" {
		t.Fatalf("expected httpmask.mode=stream, got %#v", httpmask["mode"])
	}
	if got, _ := httpmask["mask-host"].(string); got != "mask.example.com" {
		t.Fatalf("expected httpmask.mask-host, got %#v", httpmask["mask-host"])
	}
	if got, _ := httpmask["path-root"].(string); got != "aabbcc" {
		t.Fatalf("expected httpmask.path-root, got %#v", httpmask["path-root"])
	}
}

func TestConvertToClashMeta_Hysteria2FastOpenDisabledOmitsFastOpen(t *testing.T) {
	proxy := singleProxyFromOutbound(t, map[string]interface{}{
		"type":             "hysteria2",
		"tag":              "hy2-fast-open",
		"server":           "example.com",
		"server_port":      443,
		"password":         "pwd",
		"mihomo_fast_open": false,
	})

	if _, exists := proxy["fast-open"]; exists {
		t.Fatalf("expected fast-open to be omitted when mihomo_fast_open=false, got %v", proxy["fast-open"])
	}
}

func TestConvertToClashMeta_Hysteria2FastOpenDefaultsDisabled(t *testing.T) {
	proxy := singleProxyFromOutbound(t, map[string]interface{}{
		"type":        "hysteria2",
		"tag":         "hy2-fast-open-default-disabled",
		"server":      "example.com",
		"server_port": 443,
		"password":    "pwd",
	})

	if _, exists := proxy["fast-open"]; exists {
		t.Fatalf("expected fast-open to be omitted by default for hysteria2, got %v", proxy["fast-open"])
	}
}

func TestConvertToClashMeta_AnyTLSIdleFieldsAndNoReality(t *testing.T) {
	proxy := singleProxyFromOutbound(t, map[string]interface{}{
		"type":                        "anytls",
		"tag":                         "anytls-node",
		"server":                      "example.com",
		"server_port":                 443,
		"password":                    "pwd",
		"idle_session_check_interval": "30s",
		"idle_session_timeout":        "45",
		"min_idle_session":            2,
		"tls": map[string]interface{}{
			"enabled":     true,
			"server_name": "anytls.example.com",
			"insecure":    true,
			"reality": map[string]interface{}{
				"enabled":    true,
				"public_key": "should-not-export",
				"short_id":   "abcd",
			},
		},
	})

	if got := asIntValue(t, proxy["idle-session-check-interval"]); got != 30 {
		t.Fatalf("unexpected idle-session-check-interval: %v", got)
	}
	if got := asIntValue(t, proxy["idle-session-timeout"]); got != 45 {
		t.Fatalf("unexpected idle-session-timeout: %v", got)
	}
	if got := asIntValue(t, proxy["min-idle-session"]); got != 2 {
		t.Fatalf("unexpected min-idle-session: %v", got)
	}
	if got, _ := proxy["sni"].(string); got != "anytls.example.com" {
		t.Fatalf("unexpected sni: %v", proxy["sni"])
	}
	if _, exists := proxy["reality-opts"]; exists {
		t.Fatalf("anytls should not emit reality-opts: %#v", proxy["reality-opts"])
	}
}

func TestConvertToClashMeta_TUICWithECH_NoMTLSPEMExport(t *testing.T) {
	proxy := singleProxyFromOutbound(t, map[string]interface{}{
		"type":                      "tuic",
		"tag":                       "tuic-node",
		"server":                    "example.com",
		"server_port":               10443,
		"uuid":                      "00000000-0000-0000-0000-000000000001",
		"password":                  "pwd",
		"congestion_control":        "bbr",
		"udp_relay_mode":            "native",
		"zero_rtt_handshake":        true,
		"request_timeout":           "8s",
		"auth_timeout":              "5s",
		"heartbeat":                 "10s",
		"max_open_streams":          20,
		"max_udp_relay_packet_size": 1500,
		"cwnd":                      16,
		"ip":                        "1.1.1.1",
		"udp_over_stream":           true,
		"udp_over_stream_version":   2,
		"disable_mtu_discovery":     true,
		"max_datagram_frame_size":   1200,
		"mihomo_fast_open":          true,
		"tls": map[string]interface{}{
			"enabled":     true,
			"disable_sni": true,
			"server_name": "tuic.example.com",
			"client_certificate": []interface{}{
				"-----BEGIN CERTIFICATE-----",
				"AAA",
				"-----END CERTIFICATE-----",
			},
			"client_key": []interface{}{
				"-----BEGIN PRIVATE KEY-----",
				"BBB",
				"-----END PRIVATE KEY-----",
			},
			"ech": map[string]interface{}{
				"enabled": true,
				"config": []interface{}{
					"-----BEGIN ECH CONFIGS-----",
					"ABC",
					"-----END ECH CONFIGS-----",
				},
			},
		},
	})

	if got, _ := proxy["congestion-controller"].(string); got != "bbr" {
		t.Fatalf("unexpected congestion-controller: %v", proxy["congestion-controller"])
	}
	if got, _ := proxy["udp-relay-mode"].(string); got != "native" {
		t.Fatalf("unexpected udp-relay-mode: %v", proxy["udp-relay-mode"])
	}
	if got, _ := proxy["reduce-rtt"].(bool); !got {
		t.Fatalf("expected reduce-rtt=true, got %v", proxy["reduce-rtt"])
	}
	if got := asIntValue(t, proxy["heartbeat-interval"]); got != 10000 {
		t.Fatalf("unexpected heartbeat-interval: %v", got)
	}
	if got := asIntValue(t, proxy["request-timeout"]); got != 8000 {
		t.Fatalf("unexpected request-timeout: %v", got)
	}
	if got := asIntValue(t, proxy["max-open-streams"]); got != 20 {
		t.Fatalf("unexpected max-open-streams: %v", got)
	}
	if got := asIntValue(t, proxy["max-udp-relay-packet-size"]); got != 1500 {
		t.Fatalf("unexpected max-udp-relay-packet-size: %v", got)
	}
	if got := asIntValue(t, proxy["cwnd"]); got != 16 {
		t.Fatalf("unexpected cwnd: %v", got)
	}
	if got, _ := proxy["ip"].(string); got != "1.1.1.1" {
		t.Fatalf("unexpected ip: %v", proxy["ip"])
	}
	if got, _ := proxy["udp-over-stream"].(bool); !got {
		t.Fatalf("expected udp-over-stream=true, got %v", proxy["udp-over-stream"])
	}
	if got := asIntValue(t, proxy["udp-over-stream-version"]); got != 2 {
		t.Fatalf("unexpected udp-over-stream-version: %v", got)
	}
	if got, _ := proxy["disable-mtu-discovery"].(bool); !got {
		t.Fatalf("expected disable-mtu-discovery=true, got %v", proxy["disable-mtu-discovery"])
	}
	if got := asIntValue(t, proxy["max-datagram-frame-size"]); got != 1200 {
		t.Fatalf("unexpected max-datagram-frame-size: %v", got)
	}
	if got, _ := proxy["fast-open"].(bool); !got {
		t.Fatalf("expected fast-open=true, got %v", proxy["fast-open"])
	}
	if got, _ := proxy["disable-sni"].(bool); !got {
		t.Fatalf("expected disable-sni=true, got %v", proxy["disable-sni"])
	}
	if _, exists := proxy["certificate"]; exists {
		t.Fatalf("certificate should not be exported for clash")
	}
	if _, exists := proxy["private-key"]; exists {
		t.Fatalf("private-key should not be exported for clash")
	}
	echOpts := asMap(t, proxy["ech-opts"])
	if got, _ := echOpts["config"].(string); got != "ABC" {
		t.Fatalf("unexpected ech-opts.config: %v", echOpts["config"])
	}
}

func TestConvertToClashMeta_TUICFastOpenDefaultsDisabled(t *testing.T) {
	proxy := singleProxyFromOutbound(t, map[string]interface{}{
		"type":        "tuic",
		"tag":         "tuic-no-fast-open",
		"server":      "example.com",
		"server_port": 10443,
		"uuid":        "00000000-0000-0000-0000-000000000002",
		"password":    "pwd",
		"tls": map[string]interface{}{
			"enabled": true,
		},
	})

	if _, exists := proxy["fast-open"]; exists {
		t.Fatalf("expected fast-open to be omitted by default for tuic, got %v", proxy["fast-open"])
	}
}

func TestConvertToClashMeta_TUICFastOpenDisabledOmitsFastOpen(t *testing.T) {
	proxy := singleProxyFromOutbound(t, map[string]interface{}{
		"type":             "tuic",
		"tag":              "tuic-fast-open-disabled",
		"server":           "example.com",
		"server_port":      10443,
		"uuid":             "00000000-0000-0000-0000-000000000003",
		"password":         "pwd",
		"mihomo_fast_open": false,
		"tls": map[string]interface{}{
			"enabled": true,
		},
	})

	if _, exists := proxy["fast-open"]; exists {
		t.Fatalf("expected fast-open to be omitted when mihomo_fast_open=false, got %v", proxy["fast-open"])
	}
}

func TestConvertToClashMeta_ProtocolFastOpenIgnoresTCPFastOpen(t *testing.T) {
	cases := []struct {
		name     string
		outbound map[string]interface{}
	}{
		{
			name: "hysteria",
			outbound: map[string]interface{}{
				"type":             "hysteria",
				"tag":              "hy1-ignore-tcp-fast-open",
				"server":           "example.com",
				"server_port":      443,
				"auth_str":         "pwd",
				"mihomo_fast_open": false,
				"tcp_fast_open":    true,
			},
		},
		{
			name: "hysteria2",
			outbound: map[string]interface{}{
				"type":             "hysteria2",
				"tag":              "hy2-ignore-tcp-fast-open",
				"server":           "example.com",
				"server_port":      443,
				"password":         "pwd",
				"mihomo_fast_open": false,
				"tcp_fast_open":    true,
			},
		},
		{
			name: "tuic",
			outbound: map[string]interface{}{
				"type":             "tuic",
				"tag":              "tuic-ignore-tcp-fast-open",
				"server":           "example.com",
				"server_port":      10443,
				"uuid":             "00000000-0000-0000-0000-000000000004",
				"password":         "pwd",
				"mihomo_fast_open": false,
				"tcp_fast_open":    true,
				"tls": map[string]interface{}{
					"enabled": true,
				},
			},
		},
	}

	for _, tt := range cases {
		proxy := singleProxyFromOutbound(t, tt.outbound)
		if _, exists := proxy["fast-open"]; exists {
			t.Fatalf("%s: expected fast-open to ignore tcp_fast_open and stay omitted, got %v", tt.name, proxy["fast-open"])
		}
	}
}

func TestConvertToClashMeta_MapsMihomoCommonFields(t *testing.T) {
	proxy := singleProxyFromOutbound(t, map[string]interface{}{
		"type":            "vless",
		"tag":             "vless-common-fields",
		"server":          "example.com",
		"server_port":     443,
		"uuid":            "00000000-0000-0000-0000-000000000099",
		"udp":             false,
		"ip_version":      "ipv6-prefer",
		"routing_mark":    123,
		"tcp_fast_open":   true,
		"tcp_multi_path":  true,
		"packet_encoding": "xudp",
		"transport": map[string]interface{}{
			"type": "ws",
			"path": "/ws",
		},
		"multiplex": map[string]interface{}{
			"enabled":         true,
			"protocol":        "smux",
			"max_connections": 12,
		},
		"tls": map[string]interface{}{
			"enabled": true,
		},
	})

	if got, ok := proxy["udp"].(bool); !ok || got {
		t.Fatalf("expected udp override false, got %#v", proxy["udp"])
	}
	if got, _ := proxy["ip-version"].(string); got != "ipv6-prefer" {
		t.Fatalf("unexpected ip-version: %#v", proxy["ip-version"])
	}
	if got := asIntValue(t, proxy["routing-mark"]); got != 123 {
		t.Fatalf("unexpected routing-mark: %#v", proxy["routing-mark"])
	}
	if got, _ := proxy["tfo"].(bool); !got {
		t.Fatalf("unexpected tfo: %#v", proxy["tfo"])
	}
	if got, _ := proxy["mptcp"].(bool); !got {
		t.Fatalf("unexpected mptcp: %#v", proxy["mptcp"])
	}
	smux := asMap(t, proxy["smux"])
	if got, _ := smux["protocol"].(string); got != "smux" {
		t.Fatalf("unexpected smux protocol: %#v", smux["protocol"])
	}
	if got := asIntValue(t, smux["max-connections"]); got != 12 {
		t.Fatalf("unexpected smux max-connections: %#v", smux["max-connections"])
	}
}

func TestConvertToClashMeta_PreservesZeroValuedCommonFields(t *testing.T) {
	proxy := singleProxyFromOutbound(t, map[string]interface{}{
		"type":         "vless",
		"tag":          "vless-zero-fields",
		"server":       "example.com",
		"server_port":  443,
		"uuid":         "00000000-0000-0000-0000-000000000100",
		"routing_mark": 0,
		"multiplex": map[string]interface{}{
			"enabled":         true,
			"max_connections": 0,
			"min_streams":     0,
			"max_streams":     0,
			"brutal": map[string]interface{}{
				"enabled":   true,
				"up_mbps":   0,
				"down_mbps": 0,
			},
		},
		"tls": map[string]interface{}{
			"enabled": true,
		},
	})

	if got := asIntValue(t, proxy["routing-mark"]); got != 0 {
		t.Fatalf("unexpected routing-mark: %#v", proxy["routing-mark"])
	}
	smux := asMap(t, proxy["smux"])
	if got := asIntValue(t, smux["max-connections"]); got != 0 {
		t.Fatalf("unexpected smux max-connections: %#v", smux["max-connections"])
	}
	if got := asIntValue(t, smux["min-streams"]); got != 0 {
		t.Fatalf("unexpected smux min-streams: %#v", smux["min-streams"])
	}
	if got := asIntValue(t, smux["max-streams"]); got != 0 {
		t.Fatalf("unexpected smux max-streams: %#v", smux["max-streams"])
	}
	if got, ok := smux["statistic"].(bool); !ok || got {
		t.Fatalf("unexpected smux statistic: %#v", smux["statistic"])
	}
	if got, ok := smux["only-tcp"].(bool); !ok || got {
		t.Fatalf("unexpected smux only-tcp: %#v", smux["only-tcp"])
	}
	brutalOpts := asMap(t, smux["brutal-opts"])
	if got := asIntValue(t, brutalOpts["up"]); got != 0 {
		t.Fatalf("unexpected brutal up: %#v", brutalOpts["up"])
	}
	if got := asIntValue(t, brutalOpts["down"]); got != 0 {
		t.Fatalf("unexpected brutal down: %#v", brutalOpts["down"])
	}
}

func TestConvertToClashMeta_MapsNestedMihomoCommonFields(t *testing.T) {
	proxy := singleProxyFromOutbound(t, map[string]interface{}{
		"type":        "vless",
		"tag":         "vless-nested-common",
		"server":      "example.com",
		"server_port": 443,
		"uuid":        "00000000-0000-0000-0000-000000000101",
		"mihomo_common": map[string]interface{}{
			"udp":            false,
			"ip_version":     "ipv4",
			"routing_mark":   66,
			"tcp_fast_open":  true,
			"tcp_multi_path": true,
			"smux": map[string]interface{}{
				"enabled":         true,
				"protocol":        "smux",
				"max_connections": 3,
				"statistic":       true,
				"only_tcp":        true,
			},
		},
		"tls": map[string]interface{}{
			"enabled": true,
		},
	})

	if got, ok := proxy["udp"].(bool); !ok || got {
		t.Fatalf("expected udp override false, got %#v", proxy["udp"])
	}
	if got, _ := proxy["ip-version"].(string); got != "ipv4" {
		t.Fatalf("unexpected ip-version: %#v", proxy["ip-version"])
	}
	if got := asIntValue(t, proxy["routing-mark"]); got != 66 {
		t.Fatalf("unexpected routing-mark: %#v", proxy["routing-mark"])
	}
	if got, _ := proxy["tfo"].(bool); !got {
		t.Fatalf("unexpected tfo: %#v", proxy["tfo"])
	}
	if got, _ := proxy["mptcp"].(bool); !got {
		t.Fatalf("unexpected mptcp: %#v", proxy["mptcp"])
	}
	smux := asMap(t, proxy["smux"])
	if got, ok := smux["statistic"].(bool); !ok || !got {
		t.Fatalf("unexpected smux statistic: %#v", smux["statistic"])
	}
	if got, ok := smux["only-tcp"].(bool); !ok || !got {
		t.Fatalf("unexpected smux only-tcp: %#v", smux["only-tcp"])
	}
}

func TestConvertToClashMeta_Hysteria2OmitsUnsetBandwidth(t *testing.T) {
	proxy := singleProxyFromOutbound(t, map[string]interface{}{
		"type":        "hysteria2",
		"tag":         "hy2-no-bandwidth",
		"server":      "example.com",
		"server_port": 443,
		"password":    "pwd",
	})

	if _, exists := proxy["up"]; exists {
		t.Fatalf("expected up to be omitted when unset, got %#v", proxy["up"])
	}
	if _, exists := proxy["down"]; exists {
		t.Fatalf("expected down to be omitted when unset, got %#v", proxy["down"])
	}
}

func TestConvertToClashMeta_MTLSPEMDisabledForClash(t *testing.T) {
	proxy := singleProxyFromOutbound(t, map[string]interface{}{
		"type":        "hysteria2",
		"tag":         "hy2-mtls-disabled",
		"server":      "example.com",
		"server_port": 443,
		"password":    "pwd",
		"tls": map[string]interface{}{
			"enabled": true,
			"client_certificate": []interface{}{
				"-----BEGIN CERTIFICATE-----",
				"AAA",
				"-----END CERTIFICATE-----",
			},
			"client_key": []interface{}{
				"-----BEGIN PRIVATE KEY-----",
				"BBB",
				"-----END PRIVATE KEY-----",
			},
		},
	})

	if _, exists := proxy["certificate"]; exists {
		t.Fatalf("certificate should not be exported for clash")
	}
	if _, exists := proxy["private-key"]; exists {
		t.Fatalf("private-key should not be exported for clash")
	}
}

func TestConvertToClashMeta_MihomoFingerprintDisablesInsecureAndMTLS(t *testing.T) {
	certLines, expectedFingerprint := buildLeafCertificateForFingerprint(t)

	proxy := singleProxyFromOutbound(t, map[string]interface{}{
		"type":        "hysteria2",
		"tag":         "hy2-fp-mode",
		"server":      "example.com",
		"server_port": 443,
		"password":    "pwd",
		"tls": map[string]interface{}{
			"enabled":                true,
			"insecure":               true,
			"mihomo_use_fingerprint": true,
			"certificate":            certLines,
			"client_certificate": []interface{}{
				"-----BEGIN CERTIFICATE-----",
				"AAA",
				"-----END CERTIFICATE-----",
			},
			"client_key": []interface{}{
				"-----BEGIN PRIVATE KEY-----",
				"BBB",
				"-----END PRIVATE KEY-----",
			},
		},
	})

	if got, _ := proxy["fingerprint"].(string); got != expectedFingerprint {
		t.Fatalf("unexpected fingerprint: %v", proxy["fingerprint"])
	}
	if _, exists := proxy["skip-cert-verify"]; exists {
		t.Fatalf("skip-cert-verify should be omitted in mihomo fingerprint mode")
	}
	if _, exists := proxy["certificate"]; exists {
		t.Fatalf("certificate should be omitted in mihomo fingerprint mode")
	}
	if _, exists := proxy["private-key"]; exists {
		t.Fatalf("private-key should be omitted in mihomo fingerprint mode")
	}
}

func TestConvertToClashMeta_DisabledServerFingerprintBlocksExplicitFingerprint(t *testing.T) {
	proxy := singleProxyFromOutbound(t, map[string]interface{}{
		"type":        "hysteria2",
		"tag":         "hy2-explicit-fp-off",
		"server":      "example.com",
		"server_port": 443,
		"password":    "pwd",
		"tls": map[string]interface{}{
			"enabled":                    true,
			"fingerprint":                "AA:BB:CC",
			"include_server_fingerprint": false,
		},
	})

	if _, exists := proxy["fingerprint"]; exists {
		t.Fatalf("expected fingerprint to be removed when include_server_fingerprint is false, got %#v", proxy["fingerprint"])
	}
}

func TestConvertToClashMeta_DisabledServerFingerprintBlocksDerivedFingerprint(t *testing.T) {
	certLines, _ := buildLeafCertificateForFingerprint(t)

	proxy := singleProxyFromOutbound(t, map[string]interface{}{
		"type":        "hysteria2",
		"tag":         "hy2-derived-fp-off",
		"server":      "example.com",
		"server_port": 443,
		"password":    "pwd",
		"tls": map[string]interface{}{
			"enabled":                    true,
			"mihomo_use_fingerprint":     true,
			"certificate":                certLines,
			"include_server_fingerprint": false,
		},
	})

	if _, exists := proxy["fingerprint"]; exists {
		t.Fatalf("expected derived fingerprint to be removed when include_server_fingerprint is false, got %#v", proxy["fingerprint"])
	}
}

func TestConvertToClashMeta_TransportMappings(t *testing.T) {
	proxy := singleProxyFromOutbound(t, map[string]interface{}{
		"type":        "vmess",
		"tag":         "vmess-node",
		"server":      "example.com",
		"server_port": 443,
		"uuid":        "11111111-1111-1111-1111-111111111111",
		"alter_id":    0,
		"tls": map[string]interface{}{
			"enabled": true,
		},
		"transport": map[string]interface{}{
			"type": "http",
			"host": []interface{}{"h1.example.com", "h2.example.com"},
			"path": "/api",
		},
	})

	if got, _ := proxy["network"].(string); got != "h2" {
		t.Fatalf("expected network=h2, got %v", proxy["network"])
	}
	h2Opts := asMap(t, proxy["h2-opts"])
	hosts := asStringSliceValue(t, h2Opts["host"])
	if len(hosts) != 2 || hosts[0] != "h1.example.com" {
		t.Fatalf("unexpected h2 host list: %#v", h2Opts["host"])
	}
	if got, _ := h2Opts["path"].(string); got != "/api" {
		t.Fatalf("unexpected h2 path: %v", h2Opts["path"])
	}
}

func TestConvertToClashMeta_TransportMappingsHTTPWithMethodKeepsHTTPOnTLS(t *testing.T) {
	proxy := singleProxyFromOutbound(t, map[string]interface{}{
		"type":        "vmess",
		"tag":         "vmess-http-node",
		"server":      "example.com",
		"server_port": 443,
		"uuid":        "11111111-1111-1111-1111-111111111111",
		"alter_id":    0,
		"tls": map[string]interface{}{
			"enabled": true,
		},
		"transport": map[string]interface{}{
			"type":   "http",
			"method": "GET",
			"path":   "/api",
		},
	})

	if got, _ := proxy["network"].(string); got != "http" {
		t.Fatalf("expected network=http, got %v", proxy["network"])
	}
	if _, exists := proxy["h2-opts"]; exists {
		t.Fatalf("h2-opts should be omitted when explicit method is set: %#v", proxy["h2-opts"])
	}
	httpOpts := asMap(t, proxy["http-opts"])
	if got, _ := httpOpts["method"].(string); got != "GET" {
		t.Fatalf("unexpected http method: %v", httpOpts["method"])
	}
}

func TestConvertToClashMeta_TransportMappingsXHTTP(t *testing.T) {
	proxy := singleProxyFromOutbound(t, map[string]interface{}{
		"type":        "vless",
		"tag":         "vless-xhttp-node",
		"server":      "example.com",
		"server_port": 443,
		"uuid":        "11111111-1111-1111-1111-111111111111",
		"tls": map[string]interface{}{
			"enabled": true,
		},
		"transport": map[string]interface{}{
			"type":                   "xhttp",
			"path":                   "/x",
			"host":                   "example.com",
			"mode":                   "stream-up",
			"headers":                map[string]interface{}{"X-Test": "1"},
			"no_grpc_header":         true,
			"x_padding_bytes":        "100-1000",
			"sc_max_each_post_bytes": 1000000,
			"reuse_settings": map[string]interface{}{
				"max_connections": "16-32",
			},
		},
	})

	if got, _ := proxy["network"].(string); got != "xhttp" {
		t.Fatalf("expected network=xhttp, got %v", proxy["network"])
	}
	xhttpOpts := asMap(t, proxy["xhttp-opts"])
	if got, _ := xhttpOpts["path"].(string); got != "/x" {
		t.Fatalf("unexpected xhttp path: %v", xhttpOpts["path"])
	}
	if got, _ := xhttpOpts["host"].(string); got != "example.com" {
		t.Fatalf("unexpected xhttp host: %v", xhttpOpts["host"])
	}
	if got, _ := xhttpOpts["mode"].(string); got != "stream-up" {
		t.Fatalf("unexpected xhttp mode: %v", xhttpOpts["mode"])
	}
	if got, _ := xhttpOpts["no-grpc-header"].(bool); !got {
		t.Fatalf("unexpected xhttp no-grpc-header: %v", xhttpOpts["no-grpc-header"])
	}
	if got, _ := xhttpOpts["x-padding-bytes"].(string); got != "100-1000" {
		t.Fatalf("unexpected xhttp x-padding-bytes: %v", xhttpOpts["x-padding-bytes"])
	}
	if got := asIntValue(t, xhttpOpts["sc-max-each-post-bytes"]); got != 1000000 {
		t.Fatalf("unexpected xhttp sc-max-each-post-bytes: %v", xhttpOpts["sc-max-each-post-bytes"])
	}
	reuse := asMap(t, xhttpOpts["reuse-settings"])
	if got, _ := reuse["max-connections"].(string); got != "16-32" {
		t.Fatalf("unexpected reuse max-connections: %v", reuse["max-connections"])
	}
}

func TestConvertToClashMeta_ShadowsocksUoTVersion(t *testing.T) {
	proxy := singleProxyFromOutbound(t, map[string]interface{}{
		"type":        "shadowsocks",
		"tag":         "ss-node",
		"server":      "example.com",
		"server_port": 443,
		"method":      "aes-128-gcm",
		"password":    "pwd",
		"network":     "udp",
		"udp_over_tcp": map[string]interface{}{
			"enabled": true,
			"version": 2,
		},
	})

	if got, _ := proxy["type"].(string); got != "ss" {
		t.Fatalf("expected type=ss, got %v", proxy["type"])
	}
	if got, _ := proxy["udp-over-tcp"].(bool); !got {
		t.Fatalf("expected udp-over-tcp=true, got %v", proxy["udp-over-tcp"])
	}
	if got := asIntValue(t, proxy["udp-over-tcp-version"]); got != 2 {
		t.Fatalf("unexpected udp-over-tcp-version: %v", got)
	}
}

func TestConvertToClashMeta_ShadowTLSDetourPairToSSPlugin(t *testing.T) {
	svc := &ClashService{}
	outbounds := []map[string]interface{}{
		{
			"type":     "shadowsocks",
			"tag":      "stls-node",
			"detour":   "stls-node-out",
			"method":   "2022-blake3-aes-128-gcm",
			"password": "ss-pass",
			"network":  "udp",
			"udp_over_tcp": map[string]interface{}{
				"enabled": true,
				"version": 2,
			},
		},
		{
			"type":        "shadowtls",
			"tag":         "stls-node-out",
			"server":      "203.0.113.10",
			"server_port": 443,
			"version":     3,
			"password":    "shadow-pass",
			"tls": map[string]interface{}{
				"server_name": "addons.mozilla.org",
				"insecure":    true,
				"alpn":        []interface{}{"h2", "http/1.1"},
				"utls": map[string]interface{}{
					"enabled":     true,
					"fingerprint": "safari",
				},
			},
		},
	}

	raw, err := svc.ConvertToClashMeta(&outbounds, "http://www.gstatic.com/generate_204", 300, 50, nil)
	if err != nil {
		t.Fatalf("ConvertToClashMeta failed: %v", err)
	}

	var doc map[string]interface{}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("yaml.Unmarshal failed: %v", err)
	}

	proxiesRaw, ok := doc["proxies"].([]interface{})
	if !ok || len(proxiesRaw) != 1 {
		t.Fatalf("expected one merged shadowtls proxy, got %#v", doc["proxies"])
	}

	proxy := asMap(t, proxiesRaw[0])
	if got, _ := proxy["name"].(string); got != "stls-node" {
		t.Fatalf("unexpected name: %v", proxy["name"])
	}
	if got, _ := proxy["type"].(string); got != "ss" {
		t.Fatalf("expected merged type ss, got %v", proxy["type"])
	}
	if got, _ := proxy["server"].(string); got != "203.0.113.10" {
		t.Fatalf("unexpected server: %v", proxy["server"])
	}
	if got := asIntValue(t, proxy["port"]); got != 443 {
		t.Fatalf("unexpected port: %v", proxy["port"])
	}
	if got, _ := proxy["cipher"].(string); got != "2022-blake3-aes-128-gcm" {
		t.Fatalf("unexpected cipher: %v", proxy["cipher"])
	}
	if got, _ := proxy["password"].(string); got != "ss-pass" {
		t.Fatalf("unexpected ss password: %v", proxy["password"])
	}
	if got, _ := proxy["plugin"].(string); got != "shadow-tls" {
		t.Fatalf("expected plugin shadow-tls, got %v", proxy["plugin"])
	}
	if got, _ := proxy["udp"].(bool); !got {
		t.Fatalf("expected udp=true, got %v", proxy["udp"])
	}
	if got, _ := proxy["udp-over-tcp"].(bool); !got {
		t.Fatalf("expected udp-over-tcp=true, got %v", proxy["udp-over-tcp"])
	}
	if got := asIntValue(t, proxy["udp-over-tcp-version"]); got != 2 {
		t.Fatalf("unexpected udp-over-tcp-version: %v", proxy["udp-over-tcp-version"])
	}
	if got, _ := proxy["client-fingerprint"].(string); got != "safari" {
		t.Fatalf("unexpected client-fingerprint: %v", proxy["client-fingerprint"])
	}

	pluginOpts := asMap(t, proxy["plugin-opts"])
	if got, _ := pluginOpts["host"].(string); got != "addons.mozilla.org" {
		t.Fatalf("unexpected shadow-tls host: %v", pluginOpts["host"])
	}
	if got, _ := pluginOpts["password"].(string); got != "shadow-pass" {
		t.Fatalf("unexpected shadow-tls password: %v", pluginOpts["password"])
	}
	if got := asIntValue(t, pluginOpts["version"]); got != 3 {
		t.Fatalf("unexpected shadow-tls version: %v", pluginOpts["version"])
	}
	if got, _ := pluginOpts["skip-cert-verify"].(bool); !got {
		t.Fatalf("expected skip-cert-verify=true, got %v", pluginOpts["skip-cert-verify"])
	}
	alpn := asStringSliceValue(t, pluginOpts["alpn"])
	if len(alpn) != 2 || alpn[0] != "h2" || alpn[1] != "http/1.1" {
		t.Fatalf("unexpected shadow-tls alpn: %#v", pluginOpts["alpn"])
	}
}

func TestConvertToClashMeta_FixedMihomoProxyGroups(t *testing.T) {
	svc := &ClashService{}
	outbounds := []map[string]interface{}{
		{
			"type":        "vmess",
			"tag":         "node-1",
			"server":      "example.com",
			"server_port": 443,
			"uuid":        "00000000-0000-0000-0000-000000000001",
		},
	}

	raw, err := svc.ConvertToClashMeta(&outbounds, "http://www.gstatic.com/generate_204", 300, 50, nil)
	if err != nil {
		t.Fatalf("ConvertToClashMeta failed: %v", err)
	}

	var doc map[string]interface{}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("yaml.Unmarshal failed: %v", err)
	}

	proxyGroupsRaw, ok := doc["proxy-groups"].([]interface{})
	if !ok || len(proxyGroupsRaw) == 0 {
		t.Fatalf("proxy-groups missing: %#v", doc["proxy-groups"])
	}

	requireEntry := func(entries []string, want string) {
		for _, item := range entries {
			if item == want {
				return
			}
		}
		t.Fatalf("expected %q in %#v", want, entries)
	}

	findGroup := func(name string) map[string]interface{} {
		for _, rawGroup := range proxyGroupsRaw {
			group, ok := rawGroup.(map[string]interface{})
			if !ok {
				continue
			}
			if groupName, _ := group["name"].(string); groupName == name {
				return group
			}
		}
		return nil
	}

	fixedGroups := []string{
		clashNodeSelectorTag,
		clashAutoSelectorTag,
		clashGlobalDirectSelectorTag,
		clashGlobalBlockSelectorTag,
		clashFinalSelectorTag,
		clashGlobalSelectorTag,
	}
	for _, groupName := range fixedGroups {
		if findGroup(groupName) == nil {
			t.Fatalf("fixed group %q not found", groupName)
		}
	}

	nodeSelectorGroup := findGroup(clashNodeSelectorTag)
	if nodeSelectorGroup == nil {
		t.Fatalf("%s group not found", clashNodeSelectorTag)
	}
	nodeSelectorEntries := asStringSliceValue(t, nodeSelectorGroup["proxies"])
	requireEntry(nodeSelectorEntries, clashAutoSelectorTag)
	requireEntry(nodeSelectorEntries, "node-1")

	globalGroup := findGroup(clashGlobalSelectorTag)
	if globalGroup == nil {
		t.Fatalf("%s group not found", clashGlobalSelectorTag)
	}
	globalEntries := asStringSliceValue(t, globalGroup["proxies"])
	requireEntry(globalEntries, clashNodeSelectorTag)
	requireEntry(globalEntries, clashAutoSelectorTag)
	requireEntry(globalEntries, clashGlobalDirectSelectorTag)
	requireEntry(globalEntries, clashGlobalBlockSelectorTag)
	requireEntry(globalEntries, clashFinalSelectorTag)
	requireEntry(globalEntries, "node-1")
}

func TestParseClashSelectorGroupsFromUI_FallbackFromRuleRows(t *testing.T) {
	uiConfig := map[string]interface{}{
		"clashRuleRows": []interface{}{
			map[string]interface{}{"name": "CN"},
			map[string]interface{}{"name": "CN"},
			map[string]interface{}{"name": "Proxy"},
			map[string]interface{}{"name": "DIRECT"},
			map[string]interface{}{"name": "HK"},
		},
	}

	groups := parseClashSelectorGroupsFromUI(uiConfig)
	if len(groups) != 2 {
		t.Fatalf("unexpected groups: %#v", groups)
	}
	if groups[0].Name != "CN" || groups[0].DefaultOutbound != clashNodeSelectorTag {
		t.Fatalf("unexpected first group: %#v", groups[0])
	}
	if groups[1].Name != "HK" || groups[1].DefaultOutbound != clashNodeSelectorTag {
		t.Fatalf("unexpected second group: %#v", groups[1])
	}
}

func TestConvertToClashMeta_IncludeNamedSelectorGroups(t *testing.T) {
	svc := &ClashService{}
	outbounds := []map[string]interface{}{
		{
			"type":        "vmess",
			"tag":         "node-1",
			"server":      "example.com",
			"server_port": 443,
			"uuid":        "00000000-0000-0000-0000-000000000001",
		},
	}

	raw, err := svc.ConvertToClashMeta(
		&outbounds,
		"http://www.gstatic.com/generate_204",
		300,
		50,
		[]clashSelectorGroupConfig{
			{Name: "CN", DefaultOutbound: "Proxy"},
			{Name: "HK", DefaultOutbound: "Auto"},
		},
	)
	if err != nil {
		t.Fatalf("ConvertToClashMeta failed: %v", err)
	}

	var doc map[string]interface{}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("yaml.Unmarshal failed: %v", err)
	}

	proxyGroupsRaw, ok := doc["proxy-groups"].([]interface{})
	if !ok || len(proxyGroupsRaw) == 0 {
		t.Fatalf("proxy-groups missing: %#v", doc["proxy-groups"])
	}

	findGroup := func(name string) map[string]interface{} {
		for _, rawGroup := range proxyGroupsRaw {
			group, ok := rawGroup.(map[string]interface{})
			if !ok {
				continue
			}
			if groupName, _ := group["name"].(string); groupName == name {
				return group
			}
		}
		return nil
	}

	cnGroup := findGroup("CN")
	if cnGroup == nil {
		t.Fatalf("CN group not found")
	}
	cnEntries := asStringSliceValue(t, cnGroup["proxies"])
	if len(cnEntries) == 0 || cnEntries[0] != clashNodeSelectorTag {
		t.Fatalf("unexpected CN proxies: %#v", cnEntries)
	}
	requireEntry := func(entries []string, want string) {
		for _, item := range entries {
			if item == want {
				return
			}
		}
		t.Fatalf("expected %q in %#v", want, entries)
	}
	requireEntry(cnEntries, clashAutoSelectorTag)
	requireEntry(cnEntries, clashGlobalDirectSelectorTag)
	requireEntry(cnEntries, clashGlobalBlockSelectorTag)
	requireEntry(cnEntries, clashFinalSelectorTag)
	requireEntry(cnEntries, "node-1")

	hkGroup := findGroup("HK")
	if hkGroup == nil {
		t.Fatalf("HK group not found")
	}
	hkEntries := asStringSliceValue(t, hkGroup["proxies"])
	if len(hkEntries) == 0 || hkEntries[0] != clashAutoSelectorTag {
		t.Fatalf("unexpected HK proxies: %#v", hkEntries)
	}

	nodeSelectorGroup := findGroup(clashNodeSelectorTag)
	if nodeSelectorGroup == nil {
		t.Fatalf("%s group not found", clashNodeSelectorTag)
	}
	nodeSelectorEntries := asStringSliceValue(t, nodeSelectorGroup["proxies"])
	requireEntry(nodeSelectorEntries, clashAutoSelectorTag)
	requireEntry(nodeSelectorEntries, "node-1")
}

func TestConvertToClashMeta_SkipsUnsupportedRuntimeOutboundTypes(t *testing.T) {
	svc := &ClashService{}
	outbounds := []map[string]interface{}{
		{
			"type":        "tor",
			"tag":         "tor-node",
			"server":      "example.com",
			"server_port": 443,
		},
		{
			"type":        "ssh",
			"tag":         "ssh-node",
			"server":      "example.com",
			"server_port": 22,
		},
		{
			"type":        "shadowtls",
			"tag":         "shadowtls-node",
			"server":      "example.com",
			"server_port": 443,
		},
	}

	raw, err := svc.ConvertToClashMeta(&outbounds, "http://www.gstatic.com/generate_204", 300, 50, nil)
	if err != nil {
		t.Fatalf("ConvertToClashMeta failed: %v", err)
	}

	var doc map[string]interface{}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("yaml.Unmarshal failed: %v", err)
	}

	proxiesRaw, ok := doc["proxies"].([]interface{})
	if !ok {
		t.Fatalf("expected proxies slice, got %#v", doc["proxies"])
	}
	if len(proxiesRaw) != 1 {
		t.Fatalf("expected only ssh runtime outbound to be converted, got %#v", proxiesRaw)
	}
	proxy, ok := proxiesRaw[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected proxy map, got %#v", proxiesRaw[0])
	}
	if got, _ := proxy["type"].(string); got != "ssh" {
		t.Fatalf("expected proxy type ssh, got %#v", proxy["type"])
	}
	if got, _ := proxy["name"].(string); got != "ssh-node" {
		t.Fatalf("expected proxy name ssh-node, got %#v", proxy["name"])
	}
}
