package service

import (
	"strings"
	"testing"
)

func TestMergeImportedSubscriptionNodes_PreservesJSONOrderAndMergesByTag(t *testing.T) {
	jsonOutbounds := []map[string]interface{}{
		{"type": "vless", "tag": "node-a"},
		{"type": "hysteria2", "tag": "node-b"},
	}
	clashProxies := []map[string]interface{}{
		{"name": "node-b", "type": "hysteria2", "server": "2.2.2.2", "port": 443},
		{"name": "node-a", "type": "vless", "server": "1.1.1.1", "port": 8443},
	}

	nodes := mergeImportedSubscriptionNodes(jsonOutbounds, clashProxies)
	if len(nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(nodes))
	}

	if nodes[0].Tag != "node-a" || nodes[1].Tag != "node-b" {
		t.Fatalf("unexpected node order: %#v", nodes)
	}

	if nodes[0].ClashProxy == nil || nodes[0].ClashProxy["name"] != "node-a" {
		t.Fatalf("expected node-a clash proxy merged by tag, got %#v", nodes[0].ClashProxy)
	}
	if nodes[1].ClashProxy == nil || nodes[1].ClashProxy["name"] != "node-b" {
		t.Fatalf("expected node-b clash proxy merged by tag, got %#v", nodes[1].ClashProxy)
	}
}

func TestMergeImportedSubscriptionNodes_ClashOnlyBuildsJSONOutbounds(t *testing.T) {
	clashProxies := []map[string]interface{}{
		{
			"name":     "only-node",
			"type":     "hysteria2",
			"server":   "8.8.8.8",
			"port":     443,
			"password": "secret",
			"up":       100,
			"down":     200,
		},
	}

	nodes := mergeImportedSubscriptionNodes(nil, clashProxies)
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}

	node := nodes[0]
	if node.Tag != "only-node" {
		t.Fatalf("expected tag only-node, got %q", node.Tag)
	}
	if node.JSONOutbound == nil {
		t.Fatalf("expected JSON outbound for clash-only node")
	}
	if got, _ := node.JSONOutbound["type"].(string); got != "hysteria2" {
		t.Fatalf("expected converted type hysteria2, got %q", got)
	}
	if got, _ := node.JSONOutbound["tag"].(string); got != "only-node" {
		t.Fatalf("expected converted tag only-node, got %q", got)
	}
	if got, _ := node.JSONOutbound["server_port"].(int); got != 443 {
		t.Fatalf("expected converted server_port 443, got %#v", node.JSONOutbound["server_port"])
	}
}

func TestMergeImportedSubscriptionNodes_PreservesJSONOutboundWhenTagAlsoExistsInClash(t *testing.T) {
	jsonOutbounds := []map[string]interface{}{
		{
			"type":        "hysteria2",
			"tag":         "node-hop",
			"server":      "1.2.3.4",
			"server_port": float64(46365),
			"tls": map[string]interface{}{
				"enabled": true,
			},
		},
	}
	clashProxies := []map[string]interface{}{
		{
			"name":         "node-hop",
			"type":         "hysteria2",
			"server":       "1.2.3.4",
			"port":         46365,
			"password":     "secret",
			"ports":        "41000-45000,46000",
			"hop-interval": 30,
			"alpn":         []interface{}{"h3", "h2"},
			"tls":          true,
		},
	}

	nodes := mergeImportedSubscriptionNodes(jsonOutbounds, clashProxies)
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}

	if _, exists := nodes[0].JSONOutbound["server_ports"]; exists {
		t.Fatalf("expected json outbound to keep original fields only, got server_ports=%#v", nodes[0].JSONOutbound["server_ports"])
	}

	if _, exists := nodes[0].JSONOutbound["hop_interval"]; exists {
		t.Fatalf("expected json outbound to keep original fields only, got hop_interval=%#v", nodes[0].JSONOutbound["hop_interval"])
	}

	tlsMap, ok := nodes[0].JSONOutbound["tls"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected tls map, got %#v", nodes[0].JSONOutbound["tls"])
	}
	if _, exists := tlsMap["alpn"]; exists {
		t.Fatalf("expected json outbound tls to keep original fields only, got alpn=%#v", tlsMap["alpn"])
	}

	if nodes[0].ClashProxy == nil {
		t.Fatalf("expected clash proxy to be preserved")
	}
	if got, _ := nodes[0].ClashProxy["name"].(string); got != "node-hop" {
		t.Fatalf("expected clash proxy name node-hop, got %#v", nodes[0].ClashProxy["name"])
	}
	if got, _ := nodes[0].ClashProxy["ports"].(string); got != "41000-45000,46000" {
		t.Fatalf("expected clash proxy ports preserved, got %#v", nodes[0].ClashProxy["ports"])
	}
}

func TestMergeImportedSubscriptionNodes_PreservesExistingPortHopFields(t *testing.T) {
	jsonOutbounds := []map[string]interface{}{
		{
			"type":         "hysteria",
			"tag":          "node-hop",
			"server_port":  float64(38855),
			"server_ports": []interface{}{"31185:35650"},
			"hop_interval": "15s",
		},
	}
	clashProxies := []map[string]interface{}{
		{
			"name":         "node-hop",
			"type":         "hysteria",
			"server":       "1.2.3.4",
			"port":         38855,
			"auth-str":     "secret",
			"ports":        "41000-45000",
			"hop-interval": 30,
		},
	}

	nodes := mergeImportedSubscriptionNodes(jsonOutbounds, clashProxies)
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}

	serverPorts, ok := nodes[0].JSONOutbound["server_ports"].([]interface{})
	if !ok || len(serverPorts) != 1 || serverPorts[0] != "31185:35650" {
		t.Fatalf("expected existing server_ports to be preserved, got %#v", nodes[0].JSONOutbound["server_ports"])
	}
	if got, _ := nodes[0].JSONOutbound["hop_interval"].(string); got != "15s" {
		t.Fatalf("expected existing hop_interval=15s, got %#v", nodes[0].JSONOutbound["hop_interval"])
	}
}

func TestConvertClashProxyToSubOutbound_MapsClientFingerprint(t *testing.T) {
	proxy := map[string]interface{}{
		"name":               "fp-node",
		"type":               "vless",
		"server":             "9.9.9.9",
		"port":               443,
		"uuid":               "00000000-0000-0000-0000-000000000000",
		"tls":                true,
		"client-fingerprint": "chrome",
	}

	outbound, ok := convertClashProxyToSubOutbound(proxy)
	if !ok {
		t.Fatalf("expected conversion success")
	}

	tlsMap, ok := outbound["tls"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected tls map, got %#v", outbound["tls"])
	}

	utls, ok := tlsMap["utls"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected utls map, got %#v", tlsMap["utls"])
	}
	if got, _ := utls["fingerprint"].(string); got != "chrome" {
		t.Fatalf("expected fingerprint chrome, got %#v", utls["fingerprint"])
	}
}

func TestConvertClashProxyToSubOutbound_SnellMapsPSKVersionReuseAndObfs(t *testing.T) {
	proxy := map[string]interface{}{
		"name":    "snell-node",
		"type":    "snell",
		"server":  "8.8.8.8",
		"port":    8443,
		"psk":     "secret-pass",
		"version": 4,
		"reuse":   true,
		"obfs-opts": map[string]interface{}{
			"mode": "tls",
			"host": "cdn.example.com",
		},
	}

	outbound, ok := convertClashProxyToSubOutbound(proxy)
	if !ok {
		t.Fatalf("expected conversion success")
	}
	if got, _ := outbound["type"].(string); got != "snell" {
		t.Fatalf("expected type snell, got %#v", outbound["type"])
	}
	if got, _ := outbound["psk"].(string); got != "secret-pass" {
		t.Fatalf("expected psk secret-pass, got %#v", outbound["psk"])
	}
	if got, _ := outbound["version"].(int); got != 4 {
		t.Fatalf("expected version 4, got %#v", outbound["version"])
	}
	if got, _ := outbound["reuse"].(bool); !got {
		t.Fatalf("expected reuse=true, got %#v", outbound["reuse"])
	}
	obfsOpts, ok := outbound["obfs_opts"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected obfs_opts map, got %#v", outbound["obfs_opts"])
	}
	if got, _ := obfsOpts["mode"].(string); got != "tls" {
		t.Fatalf("expected obfs_opts.mode=tls, got %#v", obfsOpts["mode"])
	}
	if got, _ := obfsOpts["host"].(string); got != "cdn.example.com" {
		t.Fatalf("expected obfs_opts.host=cdn.example.com, got %#v", obfsOpts["host"])
	}
}

func TestExtractClashProxies_ReadsProxiesList(t *testing.T) {
	yamlData := []byte(`
proxies:
  - name: node-1
    type: hysteria2
    server: 1.2.3.4
    port: 443
  - name: ""
    type: vless
`)

	proxies, err := extractClashProxies(yamlData)
	if err != nil {
		t.Fatalf("extractClashProxies failed: %v", err)
	}
	if len(proxies) != 1 {
		t.Fatalf("expected 1 valid proxy, got %d", len(proxies))
	}
	if got, _ := proxies[0]["name"].(string); got != "node-1" {
		t.Fatalf("expected proxy name node-1, got %q", got)
	}
}

func TestEncodeClashProxyOptions_NormalizesIntegralFloatValues(t *testing.T) {
	proxy := map[string]interface{}{
		"name":                          "node-1",
		"type":                          "hysteria2",
		"max-connection-receive-window": 2.56e+08,
		"max-stream-receive-window":     8e+07,
	}

	encoded := encodeClashProxyOptions(proxy)
	if len(encoded) == 0 {
		t.Fatalf("expected encoded clash options")
	}

	text := string(encoded)
	if strings.Contains(strings.ToLower(text), "e+") {
		t.Fatalf("expected normalized integer JSON, got scientific notation: %s", text)
	}
	if !strings.Contains(text, "256000000") || !strings.Contains(text, "80000000") {
		t.Fatalf("expected normalized integers in JSON, got: %s", text)
	}
}

func TestConvertClashProxyToSubOutbound_ShadowTLSPluginBuildsShadowTLSOutbound(t *testing.T) {
	proxy := map[string]interface{}{
		"name":               "shadow-node",
		"type":               "ss",
		"server":             "1.2.3.4",
		"port":               443,
		"cipher":             "2022-blake3-aes-128-gcm",
		"password":           "ss-pass",
		"plugin":             "shadow-tls",
		"client-fingerprint": "chrome",
		"plugin-opts": map[string]interface{}{
			"host":     "addons.mozilla.org",
			"password": "shadow-pass",
			"version":  3,
		},
	}

	outbound, ok := convertClashProxyToSubOutbound(proxy)
	if !ok {
		t.Fatalf("expected conversion success")
	}
	if got, _ := outbound["type"].(string); got != "shadowtls" {
		t.Fatalf("expected shadowtls type, got %q", got)
	}

	ssConfig, ok := outbound["ss_config"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected ss_config map, got %#v", outbound["ss_config"])
	}
	if got, _ := ssConfig["method"].(string); got != "2022-blake3-aes-128-gcm" {
		t.Fatalf("unexpected ss_config.method: %#v", ssConfig["method"])
	}

	tlsMap, ok := outbound["tls"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected tls map, got %#v", outbound["tls"])
	}
	if got, _ := tlsMap["server_name"].(string); got != "addons.mozilla.org" {
		t.Fatalf("unexpected tls.server_name: %#v", tlsMap["server_name"])
	}
	utls, ok := tlsMap["utls"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected tls.utls map, got %#v", tlsMap["utls"])
	}
	if got, _ := utls["fingerprint"].(string); got != "chrome" {
		t.Fatalf("unexpected tls.utls.fingerprint: %#v", utls["fingerprint"])
	}
}

func TestConvertClashProxyToSubOutbound_MapsUDPFlagToNetwork(t *testing.T) {
	proxy := map[string]interface{}{
		"name":     "udp-node",
		"type":     "hysteria2",
		"server":   "8.8.8.8",
		"port":     443,
		"password": "secret",
		"udp":      true,
	}

	outbound, ok := convertClashProxyToSubOutbound(proxy)
	if !ok {
		t.Fatalf("expected conversion success")
	}
	if got, _ := outbound["network"].(string); got != "udp" {
		t.Fatalf("expected network=udp, got %#v", outbound["network"])
	}
}

func TestConvertClashProxyToSubOutbound_MapsH2AndXHTTPTransport(t *testing.T) {
	h2Proxy := map[string]interface{}{
		"name":    "h2-node",
		"type":    "vless",
		"server":  "1.2.3.4",
		"port":    443,
		"uuid":    "00000000-0000-0000-0000-000000000000",
		"tls":     true,
		"network": "h2",
		"h2-opts": map[string]interface{}{
			"path": "/h2",
			"host": []interface{}{"h2.example.com"},
		},
	}

	h2Outbound, ok := convertClashProxyToSubOutbound(h2Proxy)
	if !ok {
		t.Fatalf("expected h2 conversion success")
	}
	h2Transport, ok := h2Outbound["transport"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected h2 transport map, got %#v", h2Outbound["transport"])
	}
	if got, _ := h2Transport["type"].(string); got != "h2" {
		t.Fatalf("expected transport.type h2, got %#v", h2Transport["type"])
	}
	if got, _ := h2Transport["path"].(string); got != "/h2" {
		t.Fatalf("expected transport.path /h2, got %#v", h2Transport["path"])
	}

	xhttpProxy := map[string]interface{}{
		"name":    "xhttp-node",
		"type":    "vless",
		"server":  "5.6.7.8",
		"port":    443,
		"uuid":    "11111111-1111-1111-1111-111111111111",
		"tls":     true,
		"network": "xhttp",
		"xhttp-opts": map[string]interface{}{
			"path":                   "/x",
			"host":                   "example.com",
			"mode":                   "stream-up",
			"no-grpc-header":         true,
			"x-padding-bytes":        "100-1000",
			"sc-max-each-post-bytes": 1000000,
			"reuse-settings":         map[string]interface{}{"max-connections": "16-32"},
			"download-settings":      map[string]interface{}{"path": "/d", "no-grpc-header": false},
		},
	}

	xhttpOutbound, ok := convertClashProxyToSubOutbound(xhttpProxy)
	if !ok {
		t.Fatalf("expected xhttp conversion success")
	}
	xhttpTransport, ok := xhttpOutbound["transport"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected xhttp transport map, got %#v", xhttpOutbound["transport"])
	}
	if got, _ := xhttpTransport["type"].(string); got != "xhttp" {
		t.Fatalf("expected transport.type xhttp, got %#v", xhttpTransport["type"])
	}
	if got, _ := xhttpTransport["path"].(string); got != "/x" {
		t.Fatalf("expected transport.path /x, got %#v", xhttpTransport["path"])
	}
	if got, _ := xhttpTransport["host"].(string); got != "example.com" {
		t.Fatalf("expected transport.host example.com, got %#v", xhttpTransport["host"])
	}
	if got, _ := xhttpTransport["mode"].(string); got != "stream-up" {
		t.Fatalf("expected transport.mode stream-up, got %#v", xhttpTransport["mode"])
	}
	if got, _ := xhttpTransport["no_grpc_header"].(bool); !got {
		t.Fatalf("expected transport.no_grpc_header=true, got %#v", xhttpTransport["no_grpc_header"])
	}
	reuseSettings, ok := xhttpTransport["reuse_settings"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected reuse_settings map, got %#v", xhttpTransport["reuse_settings"])
	}
	if got, _ := reuseSettings["max_connections"].(string); got != "16-32" {
		t.Fatalf("expected reuse_settings.max_connections 16-32, got %#v", reuseSettings["max_connections"])
	}
	downloadSettings, ok := xhttpTransport["download_settings"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected download_settings map, got %#v", xhttpTransport["download_settings"])
	}
	if got, _ := downloadSettings["path"].(string); got != "/d" {
		t.Fatalf("expected download_settings.path /d, got %#v", downloadSettings["path"])
	}
	if got, _ := downloadSettings["no_grpc_header"].(bool); got {
		t.Fatalf("expected download_settings.no_grpc_header=false, got %#v", downloadSettings["no_grpc_header"])
	}
}

func TestConvertClashProxyToSubOutbound_XHTTPIgnoredForNonVLESS(t *testing.T) {
	proxy := map[string]interface{}{
		"name":     "trojan-xhttp-node",
		"type":     "trojan",
		"server":   "5.6.7.8",
		"port":     443,
		"password": "secret",
		"tls":      true,
		"network":  "xhttp",
		"xhttp-opts": map[string]interface{}{
			"path": "/x",
		},
	}

	outbound, ok := convertClashProxyToSubOutbound(proxy)
	if !ok {
		t.Fatalf("expected conversion success")
	}
	if _, exists := outbound["transport"]; exists {
		t.Fatalf("expected non-vless xhttp transport to be ignored, got %#v", outbound["transport"])
	}
}

func TestConvertClashProxyToSubOutbound_MapsMieruProxy(t *testing.T) {
	proxy := map[string]interface{}{
		"name":           "mieru-node",
		"type":           "mieru",
		"server":         "7.7.7.7",
		"port-range":     "2090-2099",
		"username":       "alice",
		"password":       "secret",
		"transport":      "tcp",
		"udp":            true,
		"multiplexing":   "multiplexing_high",
		"handshake-mode": "handshake_no_wait",
	}

	outbound, ok := convertClashProxyToSubOutbound(proxy)
	if !ok {
		t.Fatalf("expected conversion success")
	}
	if got, _ := outbound["type"].(string); got != "mieru" {
		t.Fatalf("expected type mieru, got %#v", outbound["type"])
	}
	if got, _ := outbound["server"].(string); got != "7.7.7.7" {
		t.Fatalf("expected server 7.7.7.7, got %#v", outbound["server"])
	}
	if got, _ := outbound["port_range"].(string); got != "2090-2099" {
		t.Fatalf("expected port_range 2090-2099, got %#v", outbound["port_range"])
	}
	if got, _ := outbound["server_port"].(int); got != 2090 {
		t.Fatalf("expected server_port 2090, got %#v", outbound["server_port"])
	}
	if got, _ := outbound["username"].(string); got != "alice" {
		t.Fatalf("expected username alice, got %#v", outbound["username"])
	}
	if got, _ := outbound["password"].(string); got != "secret" {
		t.Fatalf("expected password secret, got %#v", outbound["password"])
	}
	if got, _ := outbound["transport"].(string); got != "TCP" {
		t.Fatalf("expected transport TCP, got %#v", outbound["transport"])
	}
	if got, _ := outbound["multiplexing"].(string); got != "MULTIPLEXING_HIGH" {
		t.Fatalf("expected multiplexing MULTIPLEXING_HIGH, got %#v", outbound["multiplexing"])
	}
	if got, _ := outbound["handshake_mode"].(string); got != "HANDSHAKE_NO_WAIT" {
		t.Fatalf("expected handshake_mode HANDSHAKE_NO_WAIT, got %#v", outbound["handshake_mode"])
	}
	if got, _ := outbound["udp"].(bool); !got {
		t.Fatalf("expected udp=true, got %#v", outbound["udp"])
	}
	if _, exists := outbound["network"]; exists {
		t.Fatalf("expected mieru conversion to avoid network field, got %#v", outbound["network"])
	}
}

func TestConvertClashProxyToSubOutbound_MapsSudokuProxy(t *testing.T) {
	proxy := map[string]interface{}{
		"name":                 "sudoku-node",
		"type":                 "sudoku",
		"server":               "7.7.7.7",
		"port":                 443,
		"key":                  "12345678-1234-1234-1234-1234567890ab",
		"aead-method":          "aes-128-gcm",
		"padding-min":          2,
		"padding-max":          7,
		"table-type":           "prefer-entropy",
		"custom-table":         "xpvxvvpv",
		"custom-tables":        []interface{}{"xpvxvvpv", "vxpvxvvp"},
		"enable-pure-downlink": true,
		"httpmask": map[string]interface{}{
			"disable":   false,
			"mode":      "split-stream",
			"tls":       true,
			"mask-host": "mask.example.com",
			"path-root": "aabbcc",
			"multiplex": "auto",
		},
	}

	outbound, ok := convertClashProxyToSubOutbound(proxy)
	if !ok {
		t.Fatalf("expected conversion success")
	}
	if got, _ := outbound["type"].(string); got != "sudoku" {
		t.Fatalf("expected type sudoku, got %#v", outbound["type"])
	}
	if got, _ := outbound["key"].(string); got != "12345678-1234-1234-1234-1234567890ab" {
		t.Fatalf("expected key to be preserved, got %#v", outbound["key"])
	}
	if got, _ := outbound["table_type"].(string); got != "prefer_entropy" {
		t.Fatalf("expected table_type prefer_entropy, got %#v", outbound["table_type"])
	}
	httpmask, ok := outbound["httpmask"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected httpmask map, got %#v", outbound["httpmask"])
	}
	if got, _ := httpmask["mode"].(string); got != "stream" {
		t.Fatalf("expected httpmask.mode stream, got %#v", httpmask["mode"])
	}
	if got, _ := httpmask["host"].(string); got != "mask.example.com" {
		t.Fatalf("expected httpmask.host, got %#v", httpmask["host"])
	}
	if got, _ := httpmask["path_root"].(string); got != "aabbcc" {
		t.Fatalf("expected httpmask.path_root, got %#v", httpmask["path_root"])
	}
	if _, exists := outbound["network"]; exists {
		t.Fatalf("expected sudoku conversion to avoid network field, got %#v", outbound["network"])
	}
}

func TestConvertClashProxyToSubOutbound_MapsHysteriaProxyToNewQUICFields(t *testing.T) {
	proxy := map[string]interface{}{
		"name":                  "hy1-node",
		"type":                  "hysteria",
		"server":                "6.6.6.6",
		"port":                  443,
		"auth-str":              "secret",
		"obfs":                  "obfs-pass",
		"up":                    30,
		"down":                  200,
		"recv-window-conn":      25000000,
		"recv-window":           67108864,
		"disable-mtu-discovery": true,
		"ports":                 "443-8443,9000",
	}

	outbound, ok := convertClashProxyToSubOutbound(proxy)
	if !ok {
		t.Fatalf("expected conversion success")
	}
	if got, _ := outbound["type"].(string); got != "hysteria" {
		t.Fatalf("expected type hysteria, got %#v", outbound["type"])
	}
	if got, _ := outbound["stream_receive_window"].(int); got != 25000000 {
		t.Fatalf("expected stream_receive_window 25000000, got %#v", outbound["stream_receive_window"])
	}
	if got, _ := outbound["connection_receive_window"].(int); got != 67108864 {
		t.Fatalf("expected connection_receive_window 67108864, got %#v", outbound["connection_receive_window"])
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
	serverPorts, ok := outbound["server_ports"].([]string)
	if !ok || len(serverPorts) != 2 || serverPorts[0] != "443:8443" || serverPorts[1] != "9000" {
		t.Fatalf("expected server_ports [443:8443 9000], got %#v", outbound["server_ports"])
	}
}

func TestMergeImportedSubscriptionNodes_ClashOnlyMieruBuildsJSONOutbound(t *testing.T) {
	clashProxies := []map[string]interface{}{
		{
			"name":           "mieru-only",
			"type":           "mieru",
			"server":         "8.8.4.4",
			"port-range":     "41100-41199",
			"username":       "bob",
			"password":       "pass",
			"transport":      "UDP",
			"multiplexing":   "MULTIPLEXING_LOW",
			"handshake-mode": "HANDSHAKE_STANDARD",
		},
	}

	nodes := mergeImportedSubscriptionNodes(nil, clashProxies)
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}

	node := nodes[0]
	if node.Tag != "mieru-only" {
		t.Fatalf("expected tag mieru-only, got %#v", node.Tag)
	}
	if node.JSONOutbound == nil {
		t.Fatalf("expected JSON outbound to be built for mieru clash proxy")
	}
	if got, _ := node.JSONOutbound["type"].(string); got != "mieru" {
		t.Fatalf("expected type mieru, got %#v", node.JSONOutbound["type"])
	}
	if got, _ := node.JSONOutbound["port_range"].(string); got != "41100-41199" {
		t.Fatalf("expected port_range 41100-41199, got %#v", node.JSONOutbound["port_range"])
	}
	if got, _ := node.JSONOutbound["server_port"].(int); got != 41100 {
		t.Fatalf("expected server_port 41100, got %#v", node.JSONOutbound["server_port"])
	}
	if got, _ := node.JSONOutbound["transport"].(string); got != "UDP" {
		t.Fatalf("expected transport UDP, got %#v", node.JSONOutbound["transport"])
	}
}

func TestConvertClashProxyToSubOutbound_MapsTrustTunnelProxy(t *testing.T) {
	proxy := map[string]interface{}{
		"name":                  "trusttunnel-node",
		"type":                  "trusttunnel",
		"server":                "6.6.6.6",
		"port":                  443,
		"username":              "alice",
		"password":              "secret",
		"quic":                  true,
		"udp":                   true,
		"congestion-controller": "bbr",
		"tls":                   true,
		"sni":                   "edge.example.com",
		"alpn":                  []interface{}{"h2"},
		"fingerprint":           "AA:BB:CC",
		"disable-sni":           true,
		"client-fingerprint":    "chrome",
		"health-check":          true,
		"max-connections":       1,
		"min-streams":           0,
		"max-streams":           0,
	}

	outbound, ok := convertClashProxyToSubOutbound(proxy)
	if !ok {
		t.Fatalf("expected conversion success")
	}
	if got, _ := outbound["type"].(string); got != "trusttunnel" {
		t.Fatalf("expected type trusttunnel, got %#v", outbound["type"])
	}
	if got, _ := outbound["username"].(string); got != "alice" {
		t.Fatalf("expected username alice, got %#v", outbound["username"])
	}
	if got, _ := outbound["password"].(string); got != "secret" {
		t.Fatalf("expected password secret, got %#v", outbound["password"])
	}
	if got, _ := outbound["congestion_controller"].(string); got != "bbr" {
		t.Fatalf("expected congestion_controller bbr, got %#v", outbound["congestion_controller"])
	}
	if got, _ := outbound["quic"].(bool); !got {
		t.Fatalf("expected quic=true, got %#v", outbound["quic"])
	}
	if got, _ := outbound["udp"].(bool); !got {
		t.Fatalf("expected udp=true, got %#v", outbound["udp"])
	}
	if _, exists := outbound["network"]; exists {
		t.Fatalf("expected trusttunnel import to avoid network field, got %#v", outbound["network"])
	}
	if got, _ := outbound["health_check"].(bool); !got {
		t.Fatalf("expected health_check=true, got %#v", outbound["health_check"])
	}
	if got, _ := outbound["max_connections"].(int); got != 1 {
		t.Fatalf("expected max_connections=1, got %#v", outbound["max_connections"])
	}
	if got, _ := outbound["min_streams"].(int); got != 0 {
		t.Fatalf("expected min_streams=0, got %#v", outbound["min_streams"])
	}
	if got, _ := outbound["max_streams"].(int); got != 0 {
		t.Fatalf("expected max_streams=0, got %#v", outbound["max_streams"])
	}

	tlsMap, ok := outbound["tls"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected tls map, got %#v", outbound["tls"])
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
	if got, _ := tlsMap["disable_sni"].(bool); !got {
		t.Fatalf("expected tls.disable_sni=true, got %#v", tlsMap["disable_sni"])
	}
	utls, ok := tlsMap["utls"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected tls.utls map, got %#v", tlsMap["utls"])
	}
	if got, _ := utls["fingerprint"].(string); got != "chrome" {
		t.Fatalf("expected tls.utls.fingerprint chrome, got %#v", utls["fingerprint"])
	}
}

func TestMergeImportedSubscriptionNodes_ClashOnlyTrustTunnelBuildsJSONOutbound(t *testing.T) {
	clashProxies := []map[string]interface{}{
		{
			"name":        "trusttunnel-only",
			"type":        "trusttunnel",
			"server":      "8.8.4.4",
			"port":        443,
			"username":    "bob",
			"password":    "pass",
			"quic":        true,
			"fingerprint": "AA:BB:CC",
			"tls":         true,
		},
	}

	nodes := mergeImportedSubscriptionNodes(nil, clashProxies)
	if len(nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(nodes))
	}

	node := nodes[0]
	if node.Tag != "trusttunnel-only" {
		t.Fatalf("expected tag trusttunnel-only, got %#v", node.Tag)
	}
	if node.JSONOutbound == nil {
		t.Fatalf("expected JSON outbound to be built for trusttunnel clash proxy")
	}
	if got, _ := node.JSONOutbound["type"].(string); got != "trusttunnel" {
		t.Fatalf("expected type trusttunnel, got %#v", node.JSONOutbound["type"])
	}
	if got, _ := node.JSONOutbound["username"].(string); got != "bob" {
		t.Fatalf("expected username bob, got %#v", node.JSONOutbound["username"])
	}
	if got, _ := node.JSONOutbound["password"].(string); got != "pass" {
		t.Fatalf("expected password pass, got %#v", node.JSONOutbound["password"])
	}
	if got, _ := node.JSONOutbound["quic"].(bool); !got {
		t.Fatalf("expected quic=true, got %#v", node.JSONOutbound["quic"])
	}
	tlsMap, ok := node.JSONOutbound["tls"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected tls map, got %#v", node.JSONOutbound["tls"])
	}
	if got, _ := tlsMap["fingerprint"].(string); got != "AA:BB:CC" {
		t.Fatalf("expected tls.fingerprint AA:BB:CC, got %#v", tlsMap["fingerprint"])
	}
	if node.ClashProxy == nil {
		t.Fatalf("expected raw clash proxy to be preserved")
	}
	if got, _ := node.ClashProxy["type"].(string); got != "trusttunnel" {
		t.Fatalf("expected clash proxy type trusttunnel, got %#v", node.ClashProxy["type"])
	}
	if got, _ := node.ClashProxy["username"].(string); got != "bob" {
		t.Fatalf("expected clash proxy username bob, got %#v", node.ClashProxy["username"])
	}
}
