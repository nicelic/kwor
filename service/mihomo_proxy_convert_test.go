package service

import "testing"

func asIntValue(t *testing.T, raw interface{}) int {
	t.Helper()

	switch value := raw.(type) {
	case int:
		return value
	case int32:
		return int(value)
	case int64:
		return int(value)
	case float32:
		return int(value)
	case float64:
		return int(value)
	default:
		t.Fatalf("expected numeric value, got %#v", raw)
		return 0
	}
}

func TestConvertMihomoOutboundsToClash_SelectorKeepsProxyMembers(t *testing.T) {
	rawOutbounds := []map[string]interface{}{
		{
			"type":                        "selector",
			"tag":                         "auto-group",
			"outbounds":                   []interface{}{"node-a", "direct"},
			"default":                     "node-a",
			"interrupt_exist_connections": true,
		},
		{
			"type":        "socks",
			"tag":         "node-a",
			"server":      "1.1.1.1",
			"server_port": 1080,
		},
		{
			"type": "direct",
			"tag":  "direct",
		},
	}

	result := convertMihomoOutboundsToClash(rawOutbounds)
	if len(result.ProxyGroups) != 1 {
		t.Fatalf("expected 1 proxy group, got %d", len(result.ProxyGroups))
	}

	group := result.ProxyGroups[0]
	if got, _ := group["type"].(string); got != "select" {
		t.Fatalf("expected select group, got %q", got)
	}
	if _, exists := group["interrupt-exist-connections"]; exists {
		t.Fatalf("unexpected non-standard interrupt-exist-connections field in %#v", group)
	}

	proxies, ok := group["proxies"].([]string)
	if !ok {
		t.Fatalf("expected []string proxies, got %#v", group["proxies"])
	}
	if len(proxies) != 2 {
		t.Fatalf("expected 2 proxies in group, got %#v", proxies)
	}
	if proxies[0] != "node-a" || proxies[1] != "DIRECT" {
		t.Fatalf("unexpected group members: %#v", proxies)
	}
}

func TestConvertMihomoOutboundsToClash_URLTestKeepsProxyMembers(t *testing.T) {
	rawOutbounds := []map[string]interface{}{
		{
			"type":                        "urltest",
			"tag":                         "latency",
			"outbounds":                   []interface{}{"node-a"},
			"url":                         "https://cp.cloudflare.com/generate_204",
			"interval":                    "300s",
			"tolerance":                   150,
			"interrupt_exist_connections": true,
		},
		{
			"type":        "http",
			"tag":         "node-a",
			"server":      "2.2.2.2",
			"server_port": 8080,
		},
	}

	result := convertMihomoOutboundsToClash(rawOutbounds)
	if len(result.ProxyGroups) != 1 {
		t.Fatalf("expected 1 proxy group, got %d", len(result.ProxyGroups))
	}

	group := result.ProxyGroups[0]
	if got, _ := group["type"].(string); got != "url-test" {
		t.Fatalf("expected url-test group, got %q", got)
	}
	if _, exists := group["interrupt-exist-connections"]; exists {
		t.Fatalf("unexpected non-standard interrupt-exist-connections field in %#v", group)
	}

	proxies, ok := group["proxies"].([]string)
	if !ok {
		t.Fatalf("expected []string proxies, got %#v", group["proxies"])
	}
	if len(proxies) != 1 || proxies[0] != "node-a" {
		t.Fatalf("unexpected url-test members: %#v", proxies)
	}
	if got, _ := group["url"].(string); got != "https://cp.cloudflare.com/generate_204" {
		t.Fatalf("unexpected url-test url: %#v", group["url"])
	}
	if got, _ := group["interval"].(int); got != 300 {
		t.Fatalf("unexpected url-test interval: %#v", group["interval"])
	}
	if got, _ := group["tolerance"].(int); got != 150 {
		t.Fatalf("unexpected url-test tolerance: %#v", group["tolerance"])
	}
}

func TestConvertMihomoOutboundsToClash_MapsSupportedProxyFields(t *testing.T) {
	rawOutbounds := []map[string]interface{}{
		{
			"type":           "vless",
			"tag":            "node-a",
			"server":         "1.1.1.1",
			"server_port":    443,
			"uuid":           "11111111-1111-1111-1111-111111111111",
			"udp":            false,
			"ip_version":     "ipv6-prefer",
			"detour":         "group-a",
			"bind_interface": "eth0",
			"routing_mark":   100,
			"tcp_fast_open":  true,
			"tcp_multi_path": true,
			"tls": map[string]interface{}{
				"enabled":     true,
				"server_name": "example.com",
				"disable_sni": true,
				"utls": map[string]interface{}{
					"enabled":     true,
					"fingerprint": "chrome",
				},
				"reality": map[string]interface{}{
					"enabled":    true,
					"public_key": "pub-key",
					"short_id":   "short-id",
				},
				"ech": map[string]interface{}{
					"enabled":           true,
					"config":            []interface{}{"-----BEGIN ECH CONFIGS-----", "ABC", "-----END ECH CONFIGS-----"},
					"query_server_name": "ech.example.com",
				},
			},
			"transport": map[string]interface{}{
				"type":                   "ws",
				"path":                   "/ws",
				"headers":                map[string]interface{}{"X-Test": "1"},
				"max_early_data":         2048,
				"early_data_header_name": "Sec-WebSocket-Protocol",
			},
			"multiplex": map[string]interface{}{
				"enabled":         true,
				"protocol":        "yamux",
				"max_connections": 8,
				"min_streams":     4,
				"max_streams":     16,
				"padding":         true,
				"brutal": map[string]interface{}{
					"enabled":   true,
					"up_mbps":   100,
					"down_mbps": 200,
				},
			},
		},
	}

	result := convertMihomoOutboundsToClash(rawOutbounds)
	if len(result.Proxies) != 1 {
		t.Fatalf("expected 1 proxy, got %d", len(result.Proxies))
	}

	proxy := result.Proxies[0]
	if got, _ := proxy["dialer-proxy"].(string); got != "group-a" {
		t.Fatalf("unexpected dialer-proxy: %#v", proxy["dialer-proxy"])
	}
	if got, _ := proxy["interface-name"].(string); got != "eth0" {
		t.Fatalf("unexpected interface-name: %#v", proxy["interface-name"])
	}
	if got, _ := proxy["routing-mark"].(int); got != 100 {
		t.Fatalf("unexpected routing-mark: %#v", proxy["routing-mark"])
	}
	if got, _ := proxy["ip-version"].(string); got != "ipv6-prefer" {
		t.Fatalf("unexpected ip-version: %#v", proxy["ip-version"])
	}
	if got, _ := proxy["tfo"].(bool); !got {
		t.Fatalf("unexpected tfo: %#v", proxy["tfo"])
	}
	if got, _ := proxy["mptcp"].(bool); !got {
		t.Fatalf("unexpected mptcp: %#v", proxy["mptcp"])
	}
	if got, ok := proxy["udp"].(bool); !ok || got {
		t.Fatalf("unexpected udp override: %#v", proxy["udp"])
	}
	if got, _ := proxy["client-fingerprint"].(string); got != "chrome" {
		t.Fatalf("unexpected client-fingerprint: %#v", proxy["client-fingerprint"])
	}
	if got, _ := proxy["network"].(string); got != "ws" {
		t.Fatalf("unexpected network: %#v", proxy["network"])
	}

	realityOpts, ok := proxy["reality-opts"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected reality-opts map, got %#v", proxy["reality-opts"])
	}
	if got, _ := realityOpts["public-key"].(string); got != "pub-key" {
		t.Fatalf("unexpected reality public-key: %#v", realityOpts["public-key"])
	}
	if got, _ := realityOpts["short-id"].(string); got != "short-id" {
		t.Fatalf("unexpected reality short-id: %#v", realityOpts["short-id"])
	}

	echOpts, ok := proxy["ech-opts"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected ech-opts map, got %#v", proxy["ech-opts"])
	}
	if got, _ := echOpts["config"].(string); got != "ABC" {
		t.Fatalf("unexpected ech config: %#v", echOpts["config"])
	}
	if got, _ := echOpts["query-server-name"].(string); got != "ech.example.com" {
		t.Fatalf("unexpected ech query-server-name: %#v", echOpts["query-server-name"])
	}

	wsOpts, ok := proxy["ws-opts"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected ws-opts map, got %#v", proxy["ws-opts"])
	}
	if got, _ := wsOpts["path"].(string); got != "/ws" {
		t.Fatalf("unexpected ws path: %#v", wsOpts["path"])
	}
	if got, _ := wsOpts["max-early-data"].(int); got != 2048 {
		t.Fatalf("unexpected ws max-early-data: %#v", wsOpts["max-early-data"])
	}
	if got, _ := wsOpts["early-data-header-name"].(string); got != "Sec-WebSocket-Protocol" {
		t.Fatalf("unexpected ws early-data-header-name: %#v", wsOpts["early-data-header-name"])
	}

	smux, ok := proxy["smux"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected smux map, got %#v", proxy["smux"])
	}
	if got, _ := smux["protocol"].(string); got != "yamux" {
		t.Fatalf("unexpected smux protocol: %#v", smux["protocol"])
	}
	if got, _ := smux["max-connections"].(int); got != 8 {
		t.Fatalf("unexpected smux max-connections: %#v", smux["max-connections"])
	}
	brutalOpts, ok := smux["brutal-opts"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected brutal-opts map, got %#v", smux["brutal-opts"])
	}
	if got, _ := brutalOpts["up"].(int); got != 100 {
		t.Fatalf("unexpected brutal up: %#v", brutalOpts["up"])
	}
	if got, _ := brutalOpts["down"].(int); got != 200 {
		t.Fatalf("unexpected brutal down: %#v", brutalOpts["down"])
	}
}

func TestConvertMihomoOutboundsToClash_XHTTPTransportForVLESS(t *testing.T) {
	rawOutbounds := []map[string]interface{}{
		{
			"type":        "vless",
			"tag":         "xhttp-node",
			"server":      "1.2.3.4",
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
		},
	}

	result := convertMihomoOutboundsToClash(rawOutbounds)
	if len(result.Proxies) != 1 {
		t.Fatalf("expected 1 proxy, got %d", len(result.Proxies))
	}

	proxy := result.Proxies[0]
	if got, _ := proxy["network"].(string); got != "xhttp" {
		t.Fatalf("expected network=xhttp, got %#v", proxy["network"])
	}

	xhttpOpts, ok := proxy["xhttp-opts"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected xhttp-opts map, got %#v", proxy["xhttp-opts"])
	}
	if got, _ := xhttpOpts["path"].(string); got != "/x" {
		t.Fatalf("unexpected xhttp path: %#v", xhttpOpts["path"])
	}
	if got, _ := xhttpOpts["host"].(string); got != "example.com" {
		t.Fatalf("unexpected xhttp host: %#v", xhttpOpts["host"])
	}
	if got, _ := xhttpOpts["mode"].(string); got != "stream-up" {
		t.Fatalf("unexpected xhttp mode: %#v", xhttpOpts["mode"])
	}
	if got, _ := xhttpOpts["no-grpc-header"].(bool); !got {
		t.Fatalf("unexpected xhttp no-grpc-header: %#v", xhttpOpts["no-grpc-header"])
	}
	if got, _ := xhttpOpts["x-padding-bytes"].(string); got != "100-1000" {
		t.Fatalf("unexpected xhttp x-padding-bytes: %#v", xhttpOpts["x-padding-bytes"])
	}
	if got, _ := xhttpOpts["sc-max-each-post-bytes"].(int); got != 1000000 {
		t.Fatalf("unexpected xhttp sc-max-each-post-bytes: %#v", xhttpOpts["sc-max-each-post-bytes"])
	}

	reuseSettings, ok := xhttpOpts["reuse-settings"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected reuse-settings map, got %#v", xhttpOpts["reuse-settings"])
	}
	if got, _ := reuseSettings["max-connections"].(string); got != "16-32" {
		t.Fatalf("unexpected reuse-settings.max-connections: %#v", reuseSettings["max-connections"])
	}
}

func TestConvertMihomoOutboundsToClash_HTTPTransportWithMethodKeepsHTTPOnTLS(t *testing.T) {
	rawOutbounds := []map[string]interface{}{
		{
			"type":        "vmess",
			"tag":         "http-tls-node",
			"server":      "1.1.1.1",
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
		},
	}

	result := convertMihomoOutboundsToClash(rawOutbounds)
	if len(result.Proxies) != 1 {
		t.Fatalf("expected 1 proxy, got %d", len(result.Proxies))
	}

	proxy := result.Proxies[0]
	if got, _ := proxy["network"].(string); got != "http" {
		t.Fatalf("expected network=http, got %#v", proxy["network"])
	}
	if _, exists := proxy["h2-opts"]; exists {
		t.Fatalf("h2-opts should not be emitted when explicit http method is set: %#v", proxy["h2-opts"])
	}
	httpOpts, ok := proxy["http-opts"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected http-opts map, got %#v", proxy["http-opts"])
	}
	if got, _ := httpOpts["method"].(string); got != "GET" {
		t.Fatalf("unexpected http method: %#v", httpOpts["method"])
	}
}

func TestConvertMihomoOutboundsToClash_HTTPTransportLegacyH2Compatibility(t *testing.T) {
	rawOutbounds := []map[string]interface{}{
		{
			"type":        "vmess",
			"tag":         "legacy-h2-node",
			"server":      "1.1.1.1",
			"server_port": 443,
			"uuid":        "11111111-1111-1111-1111-111111111111",
			"alter_id":    0,
			"tls": map[string]interface{}{
				"enabled": true,
			},
			"transport": map[string]interface{}{
				"type": "http",
				"path": "/api",
				"host": []interface{}{"h2.example.com"},
			},
		},
	}

	result := convertMihomoOutboundsToClash(rawOutbounds)
	if len(result.Proxies) != 1 {
		t.Fatalf("expected 1 proxy, got %d", len(result.Proxies))
	}

	proxy := result.Proxies[0]
	if got, _ := proxy["network"].(string); got != "h2" {
		t.Fatalf("expected legacy http transport to map to h2, got %#v", proxy["network"])
	}
	h2Opts, ok := proxy["h2-opts"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected h2-opts map, got %#v", proxy["h2-opts"])
	}
	gotHosts := toStringSlice(h2Opts["host"])
	if len(gotHosts) != 1 || gotHosts[0] != "h2.example.com" {
		t.Fatalf("unexpected h2 hosts: %#v", h2Opts["host"])
	}
}

func TestConvertMihomoOutboundsToClash_AnyTLSOmitsRealityOpts(t *testing.T) {
	rawOutbounds := []map[string]interface{}{
		{
			"type":             "anytls",
			"tag":              "node-a",
			"server":           "2.2.2.2",
			"server_port":      443,
			"password":         "secret",
			"min_idle_session": 1,
			"tls": map[string]interface{}{
				"enabled": true,
				"utls": map[string]interface{}{
					"enabled":     true,
					"fingerprint": "safari",
				},
				"reality": map[string]interface{}{
					"enabled":    true,
					"public_key": "pub-key",
					"short_id":   "short-id",
				},
			},
		},
	}

	result := convertMihomoOutboundsToClash(rawOutbounds)
	if len(result.Proxies) != 1 {
		t.Fatalf("expected 1 proxy, got %d", len(result.Proxies))
	}

	proxy := result.Proxies[0]
	if got, _ := proxy["client-fingerprint"].(string); got != "safari" {
		t.Fatalf("unexpected client-fingerprint: %#v", proxy["client-fingerprint"])
	}
	if _, exists := proxy["reality-opts"]; exists {
		t.Fatalf("anytls should not emit reality-opts: %#v", proxy["reality-opts"])
	}
}

func TestConvertMihomoOutboundsToClash_HysteriaMapsNewQUICFields(t *testing.T) {
	rawOutbounds := []map[string]interface{}{
		{
			"type":                       "hysteria",
			"tag":                        "hy1-node",
			"server":                     "example.com",
			"server_port":                443,
			"auth_str":                   "pwd",
			"up_mbps":                    30,
			"down_mbps":                  200,
			"stream_receive_window":      25000000,
			"connection_receive_window":  67108864,
			"disable_path_mtu_discovery": true,
			"mihomo_fast_open":           true,
			"server_ports":               []interface{}{"443:8443", "9000"},
		},
	}

	result := convertMihomoOutboundsToClash(rawOutbounds)
	if len(result.Proxies) != 1 {
		t.Fatalf("expected 1 proxy, got %d", len(result.Proxies))
	}

	proxy := result.Proxies[0]
	if got, _ := proxy["recv-window-conn"].(int); got != 25000000 {
		t.Fatalf("unexpected recv-window-conn: %#v", proxy["recv-window-conn"])
	}
	if got, _ := proxy["recv-window"].(int); got != 67108864 {
		t.Fatalf("unexpected recv-window: %#v", proxy["recv-window"])
	}
	if got, _ := proxy["disable-mtu-discovery"].(bool); !got {
		t.Fatalf("expected disable-mtu-discovery=true, got %#v", proxy["disable-mtu-discovery"])
	}
	if got, _ := proxy["fast-open"].(bool); !got {
		t.Fatalf("expected fast-open=true, got %#v", proxy["fast-open"])
	}
	if got, _ := proxy["ports"].(string); got != "443-8443,9000" {
		t.Fatalf("unexpected ports: %#v", proxy["ports"])
	}
}

func TestConvertMihomoOutboundsToClash_HysteriaOmitsZeroBandwidth(t *testing.T) {
	rawOutbounds := []map[string]interface{}{
		{
			"type":        "hysteria",
			"tag":         "hy1-zero-bandwidth",
			"server":      "example.com",
			"server_port": 443,
			"auth_str":    "pwd",
			"up_mbps":     0,
			"down_mbps":   0,
		},
	}

	result := convertMihomoOutboundsToClash(rawOutbounds)
	if len(result.Proxies) != 1 {
		t.Fatalf("expected 1 proxy, got %d", len(result.Proxies))
	}

	proxy := result.Proxies[0]
	if _, exists := proxy["up"]; exists {
		t.Fatalf("expected hysteria up to be omitted when zero, got %#v", proxy["up"])
	}
	if _, exists := proxy["down"]; exists {
		t.Fatalf("expected hysteria down to be omitted when zero, got %#v", proxy["down"])
	}
}

func TestConvertMihomoOutboundsToClash_TrustTunnelMapsLegacyUDPAndMihomoFields(t *testing.T) {
	rawOutbounds := []map[string]interface{}{
		{
			"type":               "trusttunnel",
			"tag":                "tt-node",
			"server":             "4.4.4.4",
			"server_port":        443,
			"username":           "alice",
			"password":           "secret",
			"network":            []interface{}{"tcp", "udp"},
			"quic":               true,
			"health_check":       true,
			"congestion_control": "bbr",
			"max_connections":    1,
			"min_streams":        0,
			"max_streams":        0,
			"tls": map[string]interface{}{
				"enabled":     true,
				"server_name": "edge.example.com",
				"alpn":        []interface{}{"h2"},
				"insecure":    true,
				"disable_sni": true,
				"fingerprint": "AA:BB:CC",
				"utls": map[string]interface{}{
					"enabled":     true,
					"fingerprint": "chrome",
				},
				"reality": map[string]interface{}{
					"enabled":    true,
					"public_key": "pub-key",
					"short_id":   "short-id",
				},
				"ech": map[string]interface{}{
					"enabled":           true,
					"config":            []interface{}{"ABC"},
					"query_server_name": "ech.example.com",
				},
			},
		},
	}

	result := convertMihomoOutboundsToClash(rawOutbounds)
	if len(result.Proxies) != 1 {
		t.Fatalf("expected 1 proxy, got %d", len(result.Proxies))
	}

	proxy := result.Proxies[0]
	if got, _ := proxy["type"].(string); got != "trusttunnel" {
		t.Fatalf("type = %#v", proxy["type"])
	}
	if got, _ := proxy["username"].(string); got != "alice" {
		t.Fatalf("username = %#v", proxy["username"])
	}
	if got, _ := proxy["password"].(string); got != "secret" {
		t.Fatalf("password = %#v", proxy["password"])
	}
	if got, _ := proxy["udp"].(bool); !got {
		t.Fatalf("udp = %#v", proxy["udp"])
	}
	if got, _ := proxy["quic"].(bool); !got {
		t.Fatalf("quic = %#v", proxy["quic"])
	}
	if got, _ := proxy["health-check"].(bool); !got {
		t.Fatalf("health-check = %#v", proxy["health-check"])
	}
	if got, _ := proxy["congestion-controller"].(string); got != "bbr" {
		t.Fatalf("congestion-controller = %#v", proxy["congestion-controller"])
	}
	if got, _ := proxy["max-connections"].(int); got != 1 {
		t.Fatalf("max-connections = %#v", proxy["max-connections"])
	}
	if got, _ := proxy["min-streams"].(int); got != 0 {
		t.Fatalf("min-streams = %#v", proxy["min-streams"])
	}
	if got, _ := proxy["max-streams"].(int); got != 0 {
		t.Fatalf("max-streams = %#v", proxy["max-streams"])
	}
	if got, _ := proxy["sni"].(string); got != "edge.example.com" {
		t.Fatalf("sni = %#v", proxy["sni"])
	}
	if alpn, ok := proxy["alpn"].([]string); !ok || len(alpn) != 1 || alpn[0] != "h2" {
		t.Fatalf("alpn = %#v", proxy["alpn"])
	}
	if got, _ := proxy["client-fingerprint"].(string); got != "chrome" {
		t.Fatalf("client-fingerprint = %#v", proxy["client-fingerprint"])
	}
	if got, _ := proxy["fingerprint"].(string); got != "AA:BB:CC" {
		t.Fatalf("fingerprint = %#v", proxy["fingerprint"])
	}
	if got, _ := proxy["skip-cert-verify"].(bool); !got {
		t.Fatalf("skip-cert-verify = %#v", proxy["skip-cert-verify"])
	}
	if got, _ := proxy["disable-sni"].(bool); !got {
		t.Fatalf("disable-sni = %#v", proxy["disable-sni"])
	}
	if _, exists := proxy["reality-opts"]; exists {
		t.Fatalf("trusttunnel should not emit reality-opts: %#v", proxy["reality-opts"])
	}
	if _, exists := proxy["ech-opts"]; exists {
		t.Fatalf("trusttunnel should not emit ech-opts: %#v", proxy["ech-opts"])
	}
}

func TestConvertMihomoOutboundsToClash_SnellMapsPSKVersionReuseAndObfs(t *testing.T) {
	rawOutbounds := []map[string]interface{}{
		{
			"type":        "snell",
			"tag":         "snell-node",
			"server":      "1.2.3.4",
			"server_port": 8443,
			"psk":         "secret-pass",
			"version":     4,
			"reuse":       true,
			"obfs_opts": map[string]interface{}{
				"mode": "tls",
				"host": "cdn.example.com",
			},
		},
	}

	result := convertMihomoOutboundsToClash(rawOutbounds)
	if len(result.Proxies) != 1 {
		t.Fatalf("expected 1 proxy, got %d", len(result.Proxies))
	}

	proxy := result.Proxies[0]
	if got, _ := proxy["type"].(string); got != "snell" {
		t.Fatalf("expected type=snell, got %#v", proxy["type"])
	}
	if got, _ := proxy["psk"].(string); got != "secret-pass" {
		t.Fatalf("expected psk=secret-pass, got %#v", proxy["psk"])
	}
	if got := asIntValue(t, proxy["version"]); got != 4 {
		t.Fatalf("expected version=4, got %#v", proxy["version"])
	}
	if got, _ := proxy["reuse"].(bool); !got {
		t.Fatalf("expected reuse=true, got %#v", proxy["reuse"])
	}
	obfsOpts, ok := proxy["obfs-opts"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected obfs-opts map, got %#v", proxy["obfs-opts"])
	}
	if got, _ := obfsOpts["mode"].(string); got != "tls" {
		t.Fatalf("expected obfs-opts.mode=tls, got %#v", obfsOpts["mode"])
	}
	if got, _ := obfsOpts["host"].(string); got != "cdn.example.com" {
		t.Fatalf("expected obfs-opts.host=cdn.example.com, got %#v", obfsOpts["host"])
	}
}

func TestConvertMihomoOutboundsToClash_TUICOmitsFastOpen(t *testing.T) {
	rawOutbounds := []map[string]interface{}{
		{
			"type":                      "tuic",
			"tag":                       "tuic-node",
			"server":                    "1.1.1.1",
			"server_port":               443,
			"uuid":                      "00000000-0000-0000-0000-000000000001",
			"password":                  "pwd",
			"request_timeout":           "8s",
			"auth_timeout":              "5s",
			"heartbeat":                 "10s",
			"max_open_streams":          20,
			"max_udp_relay_packet_size": 1400,
			"cwnd":                      16,
			"ip":                        "1.1.1.1",
			"udp_over_stream":           true,
			"udp_over_stream_version":   2,
			"disable_mtu_discovery":     true,
			"max_datagram_frame_size":   1200,
			"tls": map[string]interface{}{
				"enabled": true,
			},
		},
		{
			"type":             "tuic",
			"tag":              "tuic-fast-open-disabled",
			"server":           "2.2.2.2",
			"server_port":      443,
			"uuid":             "00000000-0000-0000-0000-000000000002",
			"password":         "pwd",
			"mihomo_fast_open": false,
			"tls": map[string]interface{}{
				"enabled": true,
			},
		},
	}

	result := convertMihomoOutboundsToClash(rawOutbounds)
	if len(result.Proxies) != 2 {
		t.Fatalf("expected 2 proxies, got %d", len(result.Proxies))
	}

	if _, exists := result.Proxies[0]["fast-open"]; exists {
		t.Fatalf("expected tuic fast-open to be omitted, got %#v", result.Proxies[0]["fast-open"])
	}
	if got, _ := result.Proxies[0]["request-timeout"].(int); got != 8000 {
		t.Fatalf("expected request-timeout=8000, got %#v", result.Proxies[0]["request-timeout"])
	}
	if got, _ := result.Proxies[0]["heartbeat-interval"].(int); got != 10000 {
		t.Fatalf("expected heartbeat-interval=10000, got %#v", result.Proxies[0]["heartbeat-interval"])
	}
	if got, _ := result.Proxies[0]["max-open-streams"].(int); got != 20 {
		t.Fatalf("expected max-open-streams=20, got %#v", result.Proxies[0]["max-open-streams"])
	}
	if got, _ := result.Proxies[0]["max-udp-relay-packet-size"].(int); got != 1400 {
		t.Fatalf("expected max-udp-relay-packet-size=1400, got %#v", result.Proxies[0]["max-udp-relay-packet-size"])
	}
	if got, _ := result.Proxies[0]["cwnd"].(int); got != 16 {
		t.Fatalf("expected cwnd=16, got %#v", result.Proxies[0]["cwnd"])
	}
	if got, _ := result.Proxies[0]["ip"].(string); got != "1.1.1.1" {
		t.Fatalf("expected ip=1.1.1.1, got %#v", result.Proxies[0]["ip"])
	}
	if got, _ := result.Proxies[0]["udp-over-stream"].(bool); !got {
		t.Fatalf("expected udp-over-stream=true, got %#v", result.Proxies[0]["udp-over-stream"])
	}
	if got, _ := result.Proxies[0]["udp-over-stream-version"].(int); got != 2 {
		t.Fatalf("expected udp-over-stream-version=2, got %#v", result.Proxies[0]["udp-over-stream-version"])
	}
	if got, _ := result.Proxies[0]["disable-mtu-discovery"].(bool); !got {
		t.Fatalf("expected disable-mtu-discovery=true, got %#v", result.Proxies[0]["disable-mtu-discovery"])
	}
	if got, _ := result.Proxies[0]["max-datagram-frame-size"].(int); got != 1200 {
		t.Fatalf("expected max-datagram-frame-size=1200, got %#v", result.Proxies[0]["max-datagram-frame-size"])
	}
	if _, exists := result.Proxies[1]["fast-open"]; exists {
		t.Fatalf("expected tuic fast-open to stay omitted when mihomo_fast_open=false, got %#v", result.Proxies[1]["fast-open"])
	}
}

func TestConvertMihomoOutboundsToClash_ShadowTLSMapsCommonFieldsFromSSConfig(t *testing.T) {
	rawOutbounds := []map[string]interface{}{
		{
			"type":        "shadowtls",
			"tag":         "stls-node",
			"server":      "203.0.113.10",
			"server_port": 443,
			"version":     3,
			"password":    "shadow-pass",
			"tls": map[string]interface{}{
				"server_name": "addons.mozilla.org",
			},
			"ss_config": map[string]interface{}{
				"method":         "2022-blake3-aes-128-gcm",
				"password":       "ss-pass",
				"udp":            false,
				"ip_version":     "ipv4-prefer",
				"routing_mark":   200,
				"tcp_fast_open":  true,
				"tcp_multi_path": true,
				"multiplex": map[string]interface{}{
					"enabled":         true,
					"protocol":        "yamux",
					"max_connections": 16,
				},
			},
		},
	}

	result := convertMihomoOutboundsToClash(rawOutbounds)
	if len(result.Proxies) != 1 {
		t.Fatalf("expected 1 proxy, got %d", len(result.Proxies))
	}

	proxy := result.Proxies[0]
	if got, _ := proxy["type"].(string); got != "ss" {
		t.Fatalf("unexpected shadowtls proxy type: %#v", proxy["type"])
	}
	if got, ok := proxy["udp"].(bool); !ok || got {
		t.Fatalf("expected udp override false, got %#v", proxy["udp"])
	}
	if got, _ := proxy["ip-version"].(string); got != "ipv4-prefer" {
		t.Fatalf("unexpected ip-version: %#v", proxy["ip-version"])
	}
	if got, _ := proxy["routing-mark"].(int); got != 200 {
		t.Fatalf("unexpected routing-mark: %#v", proxy["routing-mark"])
	}
	if got, _ := proxy["tfo"].(bool); !got {
		t.Fatalf("unexpected tfo: %#v", proxy["tfo"])
	}
	if got, _ := proxy["mptcp"].(bool); !got {
		t.Fatalf("unexpected mptcp: %#v", proxy["mptcp"])
	}
	smux, ok := proxy["smux"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected smux map, got %#v", proxy["smux"])
	}
	if got, _ := smux["protocol"].(string); got != "yamux" {
		t.Fatalf("unexpected smux protocol: %#v", smux["protocol"])
	}
	if got, _ := smux["max-connections"].(int); got != 16 {
		t.Fatalf("unexpected smux max-connections: %#v", smux["max-connections"])
	}
}

func TestConvertMihomoOutboundsToClash_PreservesZeroValuedCommonFields(t *testing.T) {
	rawOutbounds := []map[string]interface{}{
		{
			"type":         "vless",
			"tag":          "node-zero-fields",
			"server":       "1.1.1.1",
			"server_port":  443,
			"uuid":         "11111111-1111-1111-1111-111111111112",
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
		},
	}

	result := convertMihomoOutboundsToClash(rawOutbounds)
	if len(result.Proxies) != 1 {
		t.Fatalf("expected 1 proxy, got %d", len(result.Proxies))
	}

	proxy := result.Proxies[0]
	if got, ok := proxy["routing-mark"].(int); !ok || got != 0 {
		t.Fatalf("unexpected routing-mark: %#v", proxy["routing-mark"])
	}
	smux, ok := proxy["smux"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected smux map, got %#v", proxy["smux"])
	}
	if got, ok := smux["max-connections"].(int); !ok || got != 0 {
		t.Fatalf("unexpected smux max-connections: %#v", smux["max-connections"])
	}
	if got, ok := smux["min-streams"].(int); !ok || got != 0 {
		t.Fatalf("unexpected smux min-streams: %#v", smux["min-streams"])
	}
	if got, ok := smux["max-streams"].(int); !ok || got != 0 {
		t.Fatalf("unexpected smux max-streams: %#v", smux["max-streams"])
	}
	if got, ok := smux["statistic"].(bool); !ok || got {
		t.Fatalf("unexpected smux statistic: %#v", smux["statistic"])
	}
	if got, ok := smux["only-tcp"].(bool); !ok || got {
		t.Fatalf("unexpected smux only-tcp: %#v", smux["only-tcp"])
	}
	brutalOpts, ok := smux["brutal-opts"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected brutal-opts map, got %#v", smux["brutal-opts"])
	}
	if got, ok := brutalOpts["up"].(int); !ok || got != 0 {
		t.Fatalf("unexpected brutal up: %#v", brutalOpts["up"])
	}
	if got, ok := brutalOpts["down"].(int); !ok || got != 0 {
		t.Fatalf("unexpected brutal down: %#v", brutalOpts["down"])
	}
}

func TestConvertMihomoOutboundsToClash_MapsNestedMihomoCommonFields(t *testing.T) {
	rawOutbounds := []map[string]interface{}{
		{
			"type":        "vless",
			"tag":         "node-nested-common",
			"server":      "1.1.1.1",
			"server_port": 443,
			"uuid":        "11111111-1111-1111-1111-111111111113",
			"mihomo_common": map[string]interface{}{
				"udp":            false,
				"ip_version":     "ipv4-prefer",
				"routing_mark":   88,
				"tcp_fast_open":  true,
				"tcp_multi_path": true,
				"smux": map[string]interface{}{
					"enabled":         true,
					"protocol":        "yamux",
					"max_connections": 6,
					"statistic":       true,
					"only_tcp":        false,
				},
			},
		},
	}

	result := convertMihomoOutboundsToClash(rawOutbounds)
	if len(result.Proxies) != 1 {
		t.Fatalf("expected 1 proxy, got %d", len(result.Proxies))
	}

	proxy := result.Proxies[0]
	if got, ok := proxy["udp"].(bool); !ok || got {
		t.Fatalf("unexpected udp override: %#v", proxy["udp"])
	}
	if got, _ := proxy["ip-version"].(string); got != "ipv4-prefer" {
		t.Fatalf("unexpected ip-version: %#v", proxy["ip-version"])
	}
	if got, ok := proxy["routing-mark"].(int); !ok || got != 88 {
		t.Fatalf("unexpected routing-mark: %#v", proxy["routing-mark"])
	}
	if got, _ := proxy["tfo"].(bool); !got {
		t.Fatalf("unexpected tfo: %#v", proxy["tfo"])
	}
	if got, _ := proxy["mptcp"].(bool); !got {
		t.Fatalf("unexpected mptcp: %#v", proxy["mptcp"])
	}
	smux, ok := proxy["smux"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected smux map, got %#v", proxy["smux"])
	}
	if got, _ := smux["protocol"].(string); got != "yamux" {
		t.Fatalf("unexpected smux protocol: %#v", smux["protocol"])
	}
	if got, ok := smux["statistic"].(bool); !ok || !got {
		t.Fatalf("unexpected smux statistic: %#v", smux["statistic"])
	}
	if got, ok := smux["only-tcp"].(bool); !ok || got {
		t.Fatalf("unexpected smux only-tcp: %#v", smux["only-tcp"])
	}
}

func TestConvertMihomoOutboundsToClash_MapsBBRProfileFromNestedMihomoCommon(t *testing.T) {
	rawOutbounds := []map[string]interface{}{
		{
			"type":        "hysteria2",
			"tag":         "hy2-bbr-profile",
			"server":      "1.1.1.1",
			"server_port": 443,
			"password":    "secret",
			"mihomo_common": map[string]interface{}{
				"bbr_profile": "aggressive",
			},
		},
	}

	result := convertMihomoOutboundsToClash(rawOutbounds)
	if len(result.Proxies) != 1 {
		t.Fatalf("expected 1 proxy, got %d", len(result.Proxies))
	}

	proxy := result.Proxies[0]
	if got, _ := proxy["bbr-profile"].(string); got != "aggressive" {
		t.Fatalf("unexpected bbr-profile: %#v", proxy["bbr-profile"])
	}
}

func TestConvertMihomoOutboundsToClash_Hysteria2RangeHopIntervalFormatsRangeString(t *testing.T) {
	rawOutbounds := []map[string]interface{}{
		{
			"type":             "hysteria2",
			"tag":              "hy2-hop-range",
			"server":           "1.1.1.1",
			"server_port":      443,
			"password":         "secret",
			"hop_interval":     "30s",
			"hop_interval_max": "60s",
		},
	}

	result := convertMihomoOutboundsToClash(rawOutbounds)
	if len(result.Proxies) != 1 {
		t.Fatalf("expected 1 proxy, got %d", len(result.Proxies))
	}

	proxy := result.Proxies[0]
	if got, _ := proxy["hop-interval"].(string); got != "30-60" {
		t.Fatalf("unexpected hop-interval: %v", got)
	}
}

func TestConvertMihomoOutboundsToClash_ProtocolFastOpenSeparatedFromTFO(t *testing.T) {
	rawOutbounds := []map[string]interface{}{
		{
			"type":             "hysteria",
			"tag":              "hy1-fast-open-off-tfo-on",
			"server":           "1.1.1.1",
			"server_port":      443,
			"auth_str":         "pwd",
			"mihomo_fast_open": false,
			"tcp_fast_open":    true,
		},
		{
			"type":             "hysteria",
			"tag":              "hy1-fast-open-on-tfo-off",
			"server":           "1.1.1.2",
			"server_port":      443,
			"auth_str":         "pwd",
			"mihomo_fast_open": true,
			"tcp_fast_open":    false,
		},
		{
			"type":             "hysteria2",
			"tag":              "hy2-fast-open-off-tfo-on",
			"server":           "2.2.2.1",
			"server_port":      443,
			"password":         "pwd",
			"mihomo_fast_open": false,
			"tcp_fast_open":    true,
		},
		{
			"type":             "hysteria2",
			"tag":              "hy2-fast-open-on-tfo-off",
			"server":           "2.2.2.2",
			"server_port":      443,
			"password":         "pwd",
			"mihomo_fast_open": true,
			"tcp_fast_open":    false,
		},
		{
			"type":             "tuic",
			"tag":              "tuic-fast-open-off-tfo-on",
			"server":           "3.3.3.1",
			"server_port":      443,
			"uuid":             "00000000-0000-0000-0000-000000000011",
			"password":         "pwd",
			"mihomo_fast_open": false,
			"tcp_fast_open":    true,
			"tls": map[string]interface{}{
				"enabled": true,
			},
		},
		{
			"type":             "tuic",
			"tag":              "tuic-fast-open-on-tfo-off",
			"server":           "3.3.3.2",
			"server_port":      443,
			"uuid":             "00000000-0000-0000-0000-000000000012",
			"password":         "pwd",
			"mihomo_fast_open": true,
			"tcp_fast_open":    false,
			"tls": map[string]interface{}{
				"enabled": true,
			},
		},
	}

	result := convertMihomoOutboundsToClash(rawOutbounds)
	if len(result.Proxies) != len(rawOutbounds) {
		t.Fatalf("expected %d proxies, got %d", len(rawOutbounds), len(result.Proxies))
	}

	findProxy := func(tag string) map[string]interface{} {
		for _, proxy := range result.Proxies {
			if got, _ := proxy["name"].(string); got == tag {
				return proxy
			}
		}
		t.Fatalf("proxy %q not found", tag)
		return nil
	}

	cases := []struct {
		tag          string
		wantFastOpen bool
		wantTFO      bool
	}{
		{tag: "hy1-fast-open-off-tfo-on", wantFastOpen: false, wantTFO: true},
		{tag: "hy1-fast-open-on-tfo-off", wantFastOpen: true, wantTFO: false},
		{tag: "hy2-fast-open-off-tfo-on", wantFastOpen: false, wantTFO: true},
		{tag: "hy2-fast-open-on-tfo-off", wantFastOpen: false, wantTFO: false},
		{tag: "tuic-fast-open-off-tfo-on", wantFastOpen: false, wantTFO: true},
		{tag: "tuic-fast-open-on-tfo-off", wantFastOpen: false, wantTFO: false},
	}

	for _, tt := range cases {
		proxy := findProxy(tt.tag)
		if tt.wantFastOpen {
			if got, _ := proxy["fast-open"].(bool); !got {
				t.Fatalf("%s: expected fast-open=true, got %#v", tt.tag, proxy["fast-open"])
			}
		} else if _, exists := proxy["fast-open"]; exists {
			t.Fatalf("%s: expected fast-open to be omitted, got %#v", tt.tag, proxy["fast-open"])
		}

		gotTFO, ok := proxy["tfo"].(bool)
		if !ok || gotTFO != tt.wantTFO {
			t.Fatalf("%s: expected tfo=%v, got %#v", tt.tag, tt.wantTFO, proxy["tfo"])
		}
	}
}

func TestConvertMihomoOutboundsToClash_Hysteria2OmitsUnsupportedFastOpenAndUnsetBandwidth(t *testing.T) {
	rawOutbounds := []map[string]interface{}{
		{
			"type":             "hysteria2",
			"tag":              "hy2-node",
			"server":           "2.2.2.2",
			"server_port":      443,
			"password":         "pwd",
			"mihomo_fast_open": true,
		},
	}

	result := convertMihomoOutboundsToClash(rawOutbounds)
	if len(result.Proxies) != 1 {
		t.Fatalf("expected 1 proxy, got %d", len(result.Proxies))
	}

	proxy := result.Proxies[0]
	if _, exists := proxy["fast-open"]; exists {
		t.Fatalf("expected hysteria2 fast-open to be omitted, got %#v", proxy["fast-open"])
	}
	if _, exists := proxy["up"]; exists {
		t.Fatalf("expected hysteria2 up to be omitted when unset, got %#v", proxy["up"])
	}
	if _, exists := proxy["down"]; exists {
		t.Fatalf("expected hysteria2 down to be omitted when unset, got %#v", proxy["down"])
	}
}

func TestConvertMihomoOutboundsToClash_ReportsGroupWithNoValidMembers(t *testing.T) {
	rawOutbounds := []map[string]interface{}{
		{
			"type":      "selector",
			"tag":       "empty-group",
			"outbounds": []interface{}{"missing-node"},
		},
	}

	result := convertMihomoOutboundsToClash(rawOutbounds)
	if len(result.ValidationErrs) != 1 {
		t.Fatalf("expected one validation error, got %#v", result.ValidationErrs)
	}
	if result.ValidationErrs[0] != `proxy group "empty-group" has no valid supported members` {
		t.Fatalf("unexpected validation error: %#v", result.ValidationErrs[0])
	}
	group := result.ProxyGroups[0]
	proxies, ok := group["proxies"].([]string)
	if !ok {
		t.Fatalf("expected []string proxies, got %#v", group["proxies"])
	}
	if len(proxies) != 0 {
		t.Fatalf("expected empty invalid group members, got %#v", proxies)
	}
}

func TestConvertMihomoOutboundsToClash_MieruMapsPortRangeAndHandshake(t *testing.T) {
	rawOutbounds := []map[string]interface{}{
		{
			"type":           "mieru",
			"tag":            "mieru-node",
			"server":         "1.2.3.4",
			"server_port":    2090,
			"port_range":     "2090-2099",
			"transport":      "TCP",
			"udp":            true,
			"username":       "alice",
			"password":       "secret",
			"multiplexing":   "MULTIPLEXING_HIGH",
			"handshake_mode": "HANDSHAKE_NO_WAIT",
		},
	}

	result := convertMihomoOutboundsToClash(rawOutbounds)
	if len(result.Proxies) != 1 {
		t.Fatalf("expected 1 proxy, got %d", len(result.Proxies))
	}

	proxy := result.Proxies[0]
	if got, _ := proxy["type"].(string); got != "mieru" {
		t.Fatalf("expected type=mieru, got %v", proxy["type"])
	}
	if got, _ := proxy["port-range"].(string); got != "2090-2099" {
		t.Fatalf("expected port-range=2090-2099, got %v", proxy["port-range"])
	}
	if _, exists := proxy["port"]; exists {
		t.Fatalf("expected port to be omitted when port-range is set: %#v", proxy)
	}
	if got, _ := proxy["transport"].(string); got != "TCP" {
		t.Fatalf("expected transport=TCP, got %v", proxy["transport"])
	}
	if got, _ := proxy["udp"].(bool); !got {
		t.Fatalf("expected udp=true, got %v", proxy["udp"])
	}
	if got, _ := proxy["multiplexing"].(string); got != "MULTIPLEXING_HIGH" {
		t.Fatalf("expected multiplexing=MULTIPLEXING_HIGH, got %v", proxy["multiplexing"])
	}
	if got, _ := proxy["handshake-mode"].(string); got != "HANDSHAKE_NO_WAIT" {
		t.Fatalf("expected handshake-mode=HANDSHAKE_NO_WAIT, got %v", proxy["handshake-mode"])
	}
}

func TestConvertMihomoOutboundsToClash_SudokuMapsHTTPMaskAndKey(t *testing.T) {
	rawOutbounds := []map[string]interface{}{
		{
			"type":                 "sudoku",
			"tag":                  "sudoku-node",
			"server":               "1.2.3.4",
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
		},
	}

	result := convertMihomoOutboundsToClash(rawOutbounds)
	if len(result.Proxies) != 1 {
		t.Fatalf("expected 1 proxy, got %d", len(result.Proxies))
	}

	proxy := result.Proxies[0]
	if got, _ := proxy["type"].(string); got != "sudoku" {
		t.Fatalf("expected type=sudoku, got %v", proxy["type"])
	}
	if got, _ := proxy["key"].(string); got != "12345678-1234-1234-1234-1234567890ab" {
		t.Fatalf("expected key to be preserved, got %#v", proxy["key"])
	}
	if got, _ := proxy["aead-method"].(string); got != "aes-128-gcm" {
		t.Fatalf("expected aead-method=aes-128-gcm, got %#v", proxy["aead-method"])
	}
	if got, _ := proxy["table-type"].(string); got != "prefer_entropy" {
		t.Fatalf("expected table-type=prefer_entropy, got %#v", proxy["table-type"])
	}
	httpmask, ok := proxy["httpmask"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected nested httpmask, got %#v", proxy["httpmask"])
	}
	if got, _ := httpmask["mode"].(string); got != "stream" {
		t.Fatalf("expected httpmask.mode=stream, got %#v", httpmask["mode"])
	}
	if got, _ := httpmask["mask-host"].(string); got != "mask.example.com" {
		t.Fatalf("expected httpmask.mask-host, got %#v", httpmask["mask-host"])
	}
	if got, _ := httpmask["path-root"].(string); got != "aabbcc" {
		t.Fatalf("expected httpmask.path-root, got %#v", httpmask["path-root"])
	}
	if got, _ := httpmask["multiplex"].(string); got != "auto" {
		t.Fatalf("expected httpmask.multiplex=auto, got %#v", httpmask["multiplex"])
	}
}

func TestConvertMihomoOutboundsToClash_PreservesStoredRawClashProxy(t *testing.T) {
	rawOutbounds := []map[string]interface{}{
		{
			"type":           "mieru",
			"tag":            "raw-mieru-node",
			"name":           "raw-mieru-node",
			"server":         "9.9.9.9",
			"port-range":     "41100-41199",
			"username":       "bob",
			"password":       "pass",
			"transport":      "UDP",
			"udp":            true,
			"multiplexing":   "MULTIPLEXING_LOW",
			"handshake-mode": "HANDSHAKE_STANDARD",
		},
	}

	result := convertMihomoOutboundsToClash(rawOutbounds)
	if len(result.Proxies) != 1 {
		t.Fatalf("expected 1 proxy, got %d", len(result.Proxies))
	}

	proxy := result.Proxies[0]
	if got, _ := proxy["name"].(string); got != "raw-mieru-node" {
		t.Fatalf("expected name=raw-mieru-node, got %#v", proxy["name"])
	}
	if got, _ := proxy["type"].(string); got != "mieru" {
		t.Fatalf("expected type=mieru, got %#v", proxy["type"])
	}
	if got, _ := proxy["port-range"].(string); got != "41100-41199" {
		t.Fatalf("expected port-range=41100-41199, got %#v", proxy["port-range"])
	}
	if _, exists := proxy["tag"]; exists {
		t.Fatalf("stored raw proxy should not emit tag field: %#v", proxy["tag"])
	}
	if _, ok := result.SupportedTags["raw-mieru-node"]; !ok {
		t.Fatalf("expected raw-mieru-node to be marked as supported")
	}
}

func TestConvertMihomoOutboundsToClash_ImportedTrustTunnelUsesRawProxyAndUIOverrides(t *testing.T) {
	rawOutbounds := []map[string]interface{}{
		{
			"type":                  "trusttunnel",
			"tag":                   "tt-imported",
			"server":                "6.6.6.6",
			"server_port":           443,
			"username":              "alice",
			"password":              "secret",
			"quic":                  true,
			"congestion_controller": "bbr",
			"tls": map[string]interface{}{
				"enabled":     true,
				"server_name": "edge.example.com",
				"alpn":        []interface{}{"h2"},
				"fingerprint": "AA:BB:CC",
				"disable_sni": true,
				"utls": map[string]interface{}{
					"enabled":     true,
					"fingerprint": "chrome",
				},
			},
			mihomoImportedClashProxyKey: map[string]interface{}{
				"name":         "tt-imported",
				"type":         "trusttunnel",
				"server":       "9.9.9.9",
				"port":         8443,
				"username":     "legacy-user",
				"password":     "legacy-pass",
				"udp":          true,
				"health-check": true,
				"tls":          true,
				"extra": map[string]interface{}{
					"note": "raw",
				},
			},
		},
	}

	result := convertMihomoOutboundsToClash(rawOutbounds)
	if len(result.Proxies) != 1 {
		t.Fatalf("expected 1 proxy, got %d", len(result.Proxies))
	}

	proxy := result.Proxies[0]
	if got, _ := proxy["name"].(string); got != "tt-imported" {
		t.Fatalf("expected name tt-imported, got %#v", proxy["name"])
	}
	if got, _ := proxy["server"].(string); got != "6.6.6.6" {
		t.Fatalf("expected server 6.6.6.6, got %#v", proxy["server"])
	}
	if got, _ := proxy["port"].(int); got != 443 {
		t.Fatalf("expected port 443, got %#v", proxy["port"])
	}
	if got, _ := proxy["username"].(string); got != "alice" {
		t.Fatalf("expected username alice, got %#v", proxy["username"])
	}
	if got, _ := proxy["password"].(string); got != "secret" {
		t.Fatalf("expected password secret, got %#v", proxy["password"])
	}
	if got, _ := proxy["udp"].(bool); !got {
		t.Fatalf("expected udp=true from imported raw proxy, got %#v", proxy["udp"])
	}
	if got, _ := proxy["health-check"].(bool); !got {
		t.Fatalf("expected health-check=true from imported raw proxy, got %#v", proxy["health-check"])
	}
	if got, _ := proxy["sni"].(string); got != "edge.example.com" {
		t.Fatalf("expected sni edge.example.com, got %#v", proxy["sni"])
	}
	if got, _ := proxy["fingerprint"].(string); got != "AA:BB:CC" {
		t.Fatalf("expected fingerprint AA:BB:CC, got %#v", proxy["fingerprint"])
	}
	if got, _ := proxy["client-fingerprint"].(string); got != "chrome" {
		t.Fatalf("expected client-fingerprint chrome, got %#v", proxy["client-fingerprint"])
	}
	if got, _ := proxy["disable-sni"].(bool); !got {
		t.Fatalf("expected disable-sni=true, got %#v", proxy["disable-sni"])
	}
	if extra, ok := proxy["extra"].(map[string]interface{}); !ok || extra["note"] != "raw" {
		t.Fatalf("expected raw extra.note=raw, got %#v", proxy["extra"])
	}
	if _, exists := proxy[mihomoImportedClashProxyKey]; exists {
		t.Fatalf("expected hidden raw clash proxy key to be stripped, got %#v", proxy[mihomoImportedClashProxyKey])
	}
}

func TestConvertMihomoOutboundsToClash_MapsSSHFields(t *testing.T) {
	rawOutbounds := []map[string]interface{}{
		{
			"type":                   "ssh",
			"tag":                    "ssh-node",
			"server":                 "127.0.0.1",
			"server_port":            22,
			"user":                   "root",
			"password":               "password",
			"private_key":            "key",
			"private_key_passphrase": "key_password",
			"host_key":               []interface{}{"ssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAABAQC"},
			"host_key_algorithms":    []interface{}{"rsa"},
			"client_version":         "SSH-2.0-OpenSSH_9.0",
			"cipher":                 []interface{}{"aes128-ctr"},
			"mac":                    []interface{}{"hmac-sha2-256"},
			"kex_algorithm":          []interface{}{"curve25519-sha256"},
		},
	}

	result := convertMihomoOutboundsToClash(rawOutbounds)
	if len(result.Proxies) != 1 {
		t.Fatalf("expected 1 proxy, got %d", len(result.Proxies))
	}

	proxy := result.Proxies[0]
	if got, _ := proxy["type"].(string); got != "ssh" {
		t.Fatalf("expected type=ssh, got %#v", proxy["type"])
	}
	if got, _ := proxy["username"].(string); got != "root" {
		t.Fatalf("expected username=root, got %#v", proxy["username"])
	}
	if got, _ := proxy["password"].(string); got != "password" {
		t.Fatalf("expected password=password, got %#v", proxy["password"])
	}
	if got, _ := proxy["private-key"].(string); got != "key" {
		t.Fatalf("expected private-key=key, got %#v", proxy["private-key"])
	}
	if got, _ := proxy["private-key-passphrase"].(string); got != "key_password" {
		t.Fatalf("expected private-key-passphrase=key_password, got %#v", proxy["private-key-passphrase"])
	}
	if hostKey, ok := proxy["host-key"].([]string); !ok || len(hostKey) != 1 {
		t.Fatalf("expected host-key list, got %#v", proxy["host-key"])
	}
	if hostKeyAlgorithms, ok := proxy["host-key-algorithms"].([]string); !ok || len(hostKeyAlgorithms) != 1 || hostKeyAlgorithms[0] != "rsa" {
		t.Fatalf("expected host-key-algorithms list, got %#v", proxy["host-key-algorithms"])
	}
	if _, exists := proxy["client_version"]; exists {
		t.Fatalf("expected sing-box-only client_version to be omitted, got %#v", proxy["client_version"])
	}
	if _, exists := proxy["cipher"]; exists {
		t.Fatalf("expected sing-box-only cipher to be omitted, got %#v", proxy["cipher"])
	}
	if _, exists := proxy["mac"]; exists {
		t.Fatalf("expected sing-box-only mac to be omitted, got %#v", proxy["mac"])
	}
	if _, exists := proxy["kex_algorithm"]; exists {
		t.Fatalf("expected sing-box-only kex_algorithm to be omitted, got %#v", proxy["kex_algorithm"])
	}
	if _, unsupported := result.UnsupportedTag["ssh-node"]; unsupported {
		t.Fatalf("ssh should not be marked unsupported: %#v", result.UnsupportedTag)
	}
}
