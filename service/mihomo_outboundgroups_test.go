package service

import "testing"

func TestBuildMihomoImportedOutbounds_NormalizesUIAndPreservesRawClashProxy(t *testing.T) {
	proxies := []map[string]interface{}{
		{
			"name":       "mieru-node",
			"type":       "mieru",
			"server":     "1.2.3.4",
			"port-range": "2090-2099",
			"username":   "alice",
			"password":   "secret",
			"transport":  "TCP",
			"udp":        true,
			"extra": map[string]interface{}{
				"note": "raw",
			},
		},
	}

	outbounds := buildMihomoImportedOutbounds(proxies)
	if len(outbounds) != 1 {
		t.Fatalf("expected 1 outbound, got %d", len(outbounds))
	}

	outbound := outbounds[0]
	if got, _ := outbound["tag"].(string); got != "mieru-node" {
		t.Fatalf("expected tag mieru-node, got %#v", outbound["tag"])
	}
	if got, _ := outbound["type"].(string); got != "mieru" {
		t.Fatalf("expected type mieru, got %#v", outbound["type"])
	}
	if got, _ := outbound["server"].(string); got != "1.2.3.4" {
		t.Fatalf("expected server 1.2.3.4, got %#v", outbound["server"])
	}
	if got, _ := outbound["server_port"].(int); got != 2090 {
		t.Fatalf("expected server_port 2090, got %#v", outbound["server_port"])
	}
	if got, _ := outbound["port_range"].(string); got != "2090-2099" {
		t.Fatalf("expected port_range 2090-2099, got %#v", outbound["port_range"])
	}
	if got, _ := outbound["transport"].(string); got != "TCP" {
		t.Fatalf("expected transport TCP, got %#v", outbound["transport"])
	}
	rawProxy, ok := outbound[mihomoImportedClashProxyKey].(map[string]interface{})
	if !ok {
		t.Fatalf("expected preserved raw clash proxy, got %#v", outbound[mihomoImportedClashProxyKey])
	}
	if got, _ := rawProxy["name"].(string); got != "mieru-node" {
		t.Fatalf("expected raw name mieru-node, got %#v", rawProxy["name"])
	}
	if got, _ := rawProxy["port-range"].(string); got != "2090-2099" {
		t.Fatalf("expected raw port-range 2090-2099, got %#v", rawProxy["port-range"])
	}
	if extra, ok := rawProxy["extra"].(map[string]interface{}); !ok || extra["note"] != "raw" {
		t.Fatalf("expected raw extra.note=raw, got %#v", rawProxy["extra"])
	}
}

func TestBuildMihomoImportedOutbounds_TrustTunnelShowsTLSFields(t *testing.T) {
	proxies := []map[string]interface{}{
		{
			"name":                  "trusttunnel-node",
			"type":                  "trusttunnel",
			"server":                "6.6.6.6",
			"port":                  443,
			"username":              "alice",
			"password":              "secret",
			"udp":                   true,
			"quic":                  true,
			"congestion-controller": "bbr",
			"health-check":          true,
			"tls":                   true,
			"sni":                   "edge.example.com",
			"alpn":                  []interface{}{"h2"},
			"fingerprint":           "AA:BB:CC",
			"disable-sni":           true,
			"client-fingerprint":    "chrome",
			"extra": map[string]interface{}{
				"note": "raw",
			},
		},
	}

	outbounds := buildMihomoImportedOutbounds(proxies)
	if len(outbounds) != 1 {
		t.Fatalf("expected 1 outbound, got %d", len(outbounds))
	}

	outbound := outbounds[0]
	if got, _ := outbound["server_port"].(int); got != 443 {
		t.Fatalf("expected server_port 443, got %#v", outbound["server_port"])
	}
	if got, _ := outbound["udp"].(bool); !got {
		t.Fatalf("expected udp=true for UI, got %#v", outbound["udp"])
	}
	if got, _ := outbound["health_check"].(bool); !got {
		t.Fatalf("expected health_check=true for UI, got %#v", outbound["health_check"])
	}
	tlsMap, ok := outbound["tls"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected tls map for UI, got %#v", outbound["tls"])
	}
	if enabled, _ := tlsMap["enabled"].(bool); !enabled {
		t.Fatalf("expected tls.enabled=true, got %#v", tlsMap["enabled"])
	}
	if got, _ := tlsMap["server_name"].(string); got != "edge.example.com" {
		t.Fatalf("expected tls.server_name edge.example.com, got %#v", tlsMap["server_name"])
	}
	alpn, ok := tlsMap["alpn"].([]string)
	if !ok || len(alpn) != 1 || alpn[0] != "h2" {
		t.Fatalf("expected tls.alpn [h2], got %#v", tlsMap["alpn"])
	}
	if got, _ := tlsMap["fingerprint"].(string); got != "AA:BB:CC" {
		t.Fatalf("expected tls.fingerprint AA:BB:CC, got %#v", tlsMap["fingerprint"])
	}
	utls, ok := tlsMap["utls"].(map[string]interface{})
	if !ok || utls["fingerprint"] != "chrome" {
		t.Fatalf("expected tls.utls.fingerprint chrome, got %#v", tlsMap["utls"])
	}
	rawProxy, ok := outbound[mihomoImportedClashProxyKey].(map[string]interface{})
	if !ok {
		t.Fatalf("expected preserved raw clash proxy, got %#v", outbound[mihomoImportedClashProxyKey])
	}
	if extra, ok := rawProxy["extra"].(map[string]interface{}); !ok || extra["note"] != "raw" {
		t.Fatalf("expected raw extra.note=raw, got %#v", rawProxy["extra"])
	}
}

func TestBuildMihomoImportedOutbounds_TUICKeepsExtendedClientFieldsWithoutFastOpen(t *testing.T) {
	proxies := []map[string]interface{}{
		{
			"name":                      "tuic-node",
			"type":                      "tuic",
			"server":                    "4.4.4.4",
			"port":                      443,
			"uuid":                      "00000000-0000-0000-0000-000000000001",
			"password":                  "secret",
			"request-timeout":           8000,
			"heartbeat-interval":        10000,
			"max-open-streams":          20,
			"max-udp-relay-packet-size": 1400,
			"cwnd":                      16,
			"ip":                        "1.1.1.1",
			"fast-open":                 false,
			"udp-over-stream":           true,
			"udp-over-stream-version":   2,
			"disable-mtu-discovery":     true,
			"max-datagram-frame-size":   1200,
			"congestion-controller":     "bbr",
			"udp-relay-mode":            "native",
			"reduce-rtt":                true,
		},
	}

	outbounds := buildMihomoImportedOutbounds(proxies)
	if len(outbounds) != 1 {
		t.Fatalf("expected 1 outbound, got %d", len(outbounds))
	}

	outbound := outbounds[0]
	if got, _ := outbound["request_timeout"].(string); got != "8s" {
		t.Fatalf("expected request_timeout 8s, got %#v", outbound["request_timeout"])
	}
	if got, _ := outbound["heartbeat"].(string); got != "10s" {
		t.Fatalf("expected heartbeat 10s, got %#v", outbound["heartbeat"])
	}
	if got, _ := outbound["max_open_streams"].(int); got != 20 {
		t.Fatalf("expected max_open_streams 20, got %#v", outbound["max_open_streams"])
	}
	if got, _ := outbound["max_udp_relay_packet_size"].(int); got != 1400 {
		t.Fatalf("expected max_udp_relay_packet_size 1400, got %#v", outbound["max_udp_relay_packet_size"])
	}
	if got, _ := outbound["cwnd"].(int); got != 16 {
		t.Fatalf("expected cwnd 16, got %#v", outbound["cwnd"])
	}
	if got, _ := outbound["ip"].(string); got != "1.1.1.1" {
		t.Fatalf("expected ip 1.1.1.1, got %#v", outbound["ip"])
	}
	if _, exists := outbound["mihomo_fast_open"]; exists {
		t.Fatalf("expected mihomo_fast_open to stay omitted for tuic, got %#v", outbound["mihomo_fast_open"])
	}
	if got, _ := outbound["udp_over_stream"].(bool); !got {
		t.Fatalf("expected udp_over_stream true, got %#v", outbound["udp_over_stream"])
	}
	if got, _ := outbound["udp_over_stream_version"].(int); got != 2 {
		t.Fatalf("expected udp_over_stream_version 2, got %#v", outbound["udp_over_stream_version"])
	}
	if got, _ := outbound["disable_mtu_discovery"].(bool); !got {
		t.Fatalf("expected disable_mtu_discovery true, got %#v", outbound["disable_mtu_discovery"])
	}
	if got, _ := outbound["max_datagram_frame_size"].(int); got != 1200 {
		t.Fatalf("expected max_datagram_frame_size 1200, got %#v", outbound["max_datagram_frame_size"])
	}
	rawProxy, ok := outbound[mihomoImportedClashProxyKey].(map[string]interface{})
	if !ok || rawProxy["request-timeout"] != 8000 {
		t.Fatalf("expected raw clash proxy to be preserved, got %#v", outbound[mihomoImportedClashProxyKey])
	}
}

func TestBuildMihomoImportedOutbounds_HysteriaUsesNewQUICFields(t *testing.T) {
	proxies := []map[string]interface{}{
		{
			"name":                  "hy1-node",
			"type":                  "hysteria",
			"server":                "4.4.4.4",
			"port":                  443,
			"auth-str":              "secret",
			"obfs":                  "obfs-pass",
			"up":                    30,
			"down":                  200,
			"recv-window-conn":      25000000,
			"recv-window":           67108864,
			"disable-mtu-discovery": true,
			"fast-open":             true,
			"ports":                 "443-8443,9000",
		},
	}

	outbounds := buildMihomoImportedOutbounds(proxies)
	if len(outbounds) != 1 {
		t.Fatalf("expected 1 outbound, got %d", len(outbounds))
	}

	outbound := outbounds[0]
	if got, _ := outbound["auth_str"].(string); got != "secret" {
		t.Fatalf("expected auth_str secret, got %#v", outbound["auth_str"])
	}
	if got, _ := outbound["obfs"].(string); got != "obfs-pass" {
		t.Fatalf("expected obfs obfs-pass, got %#v", outbound["obfs"])
	}
	if got, _ := outbound["stream_receive_window"].(int); got != 25000000 {
		t.Fatalf("expected stream_receive_window 25000000, got %#v", outbound["stream_receive_window"])
	}
	if got, _ := outbound["connection_receive_window"].(int); got != 67108864 {
		t.Fatalf("expected connection_receive_window 67108864, got %#v", outbound["connection_receive_window"])
	}
	if got, _ := outbound["disable_path_mtu_discovery"].(bool); !got {
		t.Fatalf("expected disable_path_mtu_discovery true, got %#v", outbound["disable_path_mtu_discovery"])
	}
	if got, _ := outbound["mihomo_fast_open"].(bool); !got {
		t.Fatalf("expected mihomo_fast_open true, got %#v", outbound["mihomo_fast_open"])
	}
	serverPorts, ok := outbound["server_ports"].([]string)
	if !ok || len(serverPorts) != 2 || serverPorts[0] != "443:8443" || serverPorts[1] != "9000" {
		t.Fatalf("expected server_ports [443:8443 9000], got %#v", outbound["server_ports"])
	}
}

func TestExtractClashProxiesRaw_PreservesProxyMap(t *testing.T) {
	yamlData := []byte(`
proxies:
  - name: raw-node
    type: mieru
    server: 8.8.8.8
    port-range: "41100-41199"
    custom:
      key: value
`)

	proxies, err := extractClashProxiesRaw(yamlData)
	if err != nil {
		t.Fatalf("extractClashProxiesRaw failed: %v", err)
	}
	if len(proxies) != 1 {
		t.Fatalf("expected 1 proxy, got %d", len(proxies))
	}

	proxy := proxies[0]
	if got, _ := proxy["name"].(string); got != "raw-node" {
		t.Fatalf("expected name raw-node, got %#v", proxy["name"])
	}
	if got, _ := proxy["type"].(string); got != "mieru" {
		t.Fatalf("expected type mieru, got %#v", proxy["type"])
	}
	if got, _ := proxy["port-range"].(string); got != "41100-41199" {
		t.Fatalf("expected port-range 41100-41199, got %#v", proxy["port-range"])
	}
	custom, ok := proxy["custom"].(map[string]interface{})
	if !ok || custom["key"] != "value" {
		t.Fatalf("expected custom.key=value, got %#v", proxy["custom"])
	}
}
