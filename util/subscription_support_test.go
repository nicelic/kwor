package util

import "testing"

func TestFilterTaggedSubscriptionOutbounds(t *testing.T) {
	outbounds := []map[string]interface{}{
		{"type": "vmess", "tag": "vmess-node"},
		{"type": "mieru", "tag": "mieru-node"},
		{"type": "trusttunnel", "tag": "trusttunnel-node"},
		{"type": "shadowtls", "tag": "shadowtls-node"},
	}
	outTags := []string{"vmess-node", "mieru-node", "trusttunnel-node", "shadowtls-node"}

	filteredOutbounds, filteredTags := FilterTaggedSubscriptionOutbounds(
		outbounds,
		outTags,
		SupportsSingboxSubscriptionOutboundType,
	)

	if len(filteredOutbounds) != 2 {
		t.Fatalf("expected 2 supported sing-box outbounds, got %d", len(filteredOutbounds))
	}
	if len(filteredTags) != 2 {
		t.Fatalf("expected 2 supported sing-box tags, got %d", len(filteredTags))
	}
	if filteredTags[0] != "vmess-node" || filteredTags[1] != "shadowtls-node" {
		t.Fatalf("unexpected filtered tags: %#v", filteredTags)
	}
}

func TestBuildMixedSubscriptionOutboundPair(t *testing.T) {
	source := map[string]interface{}{
		"type":         "mixed",
		"tag":          "mixed-proxy",
		"server":       "proxy.example.com",
		"server_port":  1080,
		"username":     "alice",
		"password":     "secret",
		"network":      "tcp",
		"udp_over_tcp": true,
		"tls": map[string]interface{}{
			"server_name": "source.example.com",
			"utls": map[string]interface{}{
				"fingerprint": "chrome",
			},
		},
	}

	socks, http := BuildMixedSubscriptionOutboundPair(source)
	if socks == nil || http == nil {
		t.Fatal("expected mixed endpoint to produce SOCKS and HTTP variants")
	}
	if socks["type"] != "socks" || socks["tag"] != "mixed-proxy-socks" || socks["version"] != "5" {
		t.Fatalf("unexpected SOCKS mixed variant: %#v", socks)
	}
	if http["type"] != "http" || http["tag"] != "mixed-proxy-http" {
		t.Fatalf("unexpected HTTP mixed variant: %#v", http)
	}
	for _, key := range []string{"version", "network", "udp_over_tcp"} {
		if _, exists := http[key]; exists {
			t.Fatalf("HTTP mixed variant retained SOCKS-only key %q: %#v", key, http)
		}
	}
	if source["type"] != "mixed" || source["tag"] != "mixed-proxy" {
		t.Fatalf("mixed source was mutated: %#v", source)
	}

	socksTLS, _ := socks["tls"].(map[string]interface{})
	socksTLS["server_name"] = "socks.example.com"
	socksUTLS, _ := socksTLS["utls"].(map[string]interface{})
	socksUTLS["fingerprint"] = "firefox"

	httpTLS, _ := http["tls"].(map[string]interface{})
	if got, _ := httpTLS["server_name"].(string); got != "source.example.com" {
		t.Fatalf("HTTP variant reused SOCKS TLS map: %#v", httpTLS)
	}
	httpUTLS, _ := httpTLS["utls"].(map[string]interface{})
	if got, _ := httpUTLS["fingerprint"].(string); got != "chrome" {
		t.Fatalf("HTTP variant reused SOCKS nested uTLS map: %#v", httpUTLS)
	}
	sourceTLS, _ := source["tls"].(map[string]interface{})
	if got, _ := sourceTLS["server_name"].(string); got != "source.example.com" {
		t.Fatalf("source reused a variant TLS map: %#v", sourceTLS)
	}
}

func TestSupportsMihomoSubscriptionTypes(t *testing.T) {
	if !SupportsSingboxSubscriptionOutboundType("naive") {
		t.Fatalf("expected naive runtime outbound to be supported by sing-box subscription conversion")
	}
	if !SupportsMihomoSubscriptionOutboundType("mieru") {
		t.Fatalf("expected mieru runtime outbound to be supported by mihomo subscription conversion")
	}
	if !SupportsMihomoSubscriptionOutboundType("sudoku") {
		t.Fatalf("expected sudoku runtime outbound to be supported by mihomo subscription conversion")
	}
	if !SupportsMihomoSubscriptionOutboundType("trusttunnel") {
		t.Fatalf("expected trusttunnel runtime outbound to be supported by mihomo subscription conversion")
	}
	if !SupportsMihomoSubscriptionOutboundType("ssh") {
		t.Fatalf("expected ssh runtime outbound to be supported by mihomo subscription conversion")
	}
	if !SupportsMihomoSubscriptionOutboundType("snell") {
		t.Fatalf("expected snell runtime outbound to be supported by mihomo subscription conversion")
	}
	if !SupportsMihomoSubscriptionOutboundType("shadowquic") {
		t.Fatalf("expected ShadowQUIC runtime outbound to be supported by mihomo subscription conversion")
	}
	if !SupportsMihomoSubscriptionClashProxyType("shadowquic") {
		t.Fatalf("expected ShadowQUIC Clash proxy to be supported by mihomo")
	}
	if SupportsSingboxSubscriptionOutboundType("shadowquic") {
		t.Fatalf("expected ShadowQUIC to be filtered from sing-box subscriptions")
	}
	if SupportsMihomoSubscriptionOutboundType("shadowtls") {
		t.Fatalf("expected standalone shadowtls runtime outbound to be unsupported for mihomo subscription conversion")
	}
	if !SupportsMihomoSubscriptionClashProxyType("ss") {
		t.Fatalf("expected ss clash proxy type to be supported by mihomo")
	}
	if !SupportsMihomoSubscriptionClashProxyType("trusttunnel") {
		t.Fatalf("expected trusttunnel clash proxy type to be supported by mihomo")
	}
	if !SupportsMihomoSubscriptionClashProxyType("sudoku") {
		t.Fatalf("expected sudoku clash proxy type to be supported by mihomo")
	}
	if !SupportsMihomoSubscriptionClashProxyType("ssh") {
		t.Fatalf("expected ssh clash proxy type to be supported by mihomo")
	}
	if !SupportsMihomoSubscriptionClashProxyType("snell") {
		t.Fatalf("expected snell clash proxy type to be supported by mihomo")
	}
	if SupportsMihomoSubscriptionClashProxyType("tor") {
		t.Fatalf("expected tor clash proxy type to be unsupported by mihomo")
	}
}

func TestSupportsMihomoRuntimeListenerType(t *testing.T) {
	for _, inboundType := range []string{"mixed", "socks", "http", "redirect", "tproxy", "tun", "snell", "shadowsocks", "shadowquic", "vmess", "vless", "trojan", "anytls", "tuic", "hysteria2", "mieru", "sudoku", "trusttunnel"} {
		if !SupportsMihomoRuntimeListenerType(inboundType) {
			t.Fatalf("expected %s to be accepted as a Mihomo runtime listener", inboundType)
		}
	}
	for _, inboundType := range []string{"direct", "naive", "ssh", "hysteria", "shadowtls"} {
		if SupportsMihomoRuntimeListenerType(inboundType) {
			t.Fatalf("expected %s to be rejected as a Mihomo runtime listener", inboundType)
		}
	}
}

func TestMihomoOnlyOutboundTypesAreExcludedFromSingboxJSON(t *testing.T) {
	for _, outboundType := range []string{"mieru", "snell", "sudoku", "shadowquic", "trusttunnel"} {
		if SupportsSingboxSubscriptionOutboundType(outboundType) {
			t.Fatalf("%s must not be emitted as a sing-box JSON outbound", outboundType)
		}
		if !SupportsMihomoSubscriptionOutboundType(outboundType) {
			t.Fatalf("%s must remain available to Mihomo/Clash rendering", outboundType)
		}
	}
}
