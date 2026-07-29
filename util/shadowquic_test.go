package util

import "testing"

func TestShadowQUICJLSUpstreamAddrValidation(t *testing.T) {
	valid := []string{
		"www.example.com:443",
		"127.0.0.1:10443",
		"[2001:db8::1]:443",
	}
	for _, value := range valid {
		if err := ValidateMihomoShadowQUICJLSUpstreamAddr(value); err != nil {
			t.Fatalf("expected %q to be valid, got %v", value, err)
		}
	}

	invalid := []string{"", "www.example.com", "2001:db8::1:443", "[2001:db8::1]", "[not-an-ip]:443", "host:+443", "host:0", "host:65536"}
	for _, value := range invalid {
		if err := ValidateMihomoShadowQUICJLSUpstreamAddr(value); err == nil {
			t.Fatalf("expected %q to be invalid", value)
		}
	}
}

func TestShadowQUICOutboundSanitizeAndClashBuild(t *testing.T) {
	outbound := map[string]interface{}{
		"type":                  "shadowquic",
		"tag":                   "sq-node",
		"server":                "edge.example.com",
		"server_port":           float64(10443),
		"username":              "alice",
		"password":              "secret",
		"sni":                   "",
		"alpn":                  []interface{}{"h3", "", "h3"},
		"quic-versions":         []interface{}{"v1"},
		"udp-over-stream":       false,
		"zero_rtt":              false,
		"keep_alive_interval":   float64(0),
		"congestion-controller": "bbr",
		"up":                    "100 Mbps",
		"cwnd":                  float64(0),
		"disable-mtu-discovery": false,
		"mihomo_common": map[string]interface{}{
			"udp":            true,
			"ip_version":     "ipv6",
			"routing_mark":   float64(100),
			"tcp_fast_open":  true,
			"tcp_multi_path": true,
			"smux":           map[string]interface{}{"enabled": true},
		},
		"tls":                      map[string]interface{}{"enabled": true},
		"detour":                   "unexpected",
		"routing_mark":             100,
		"unknown_shadowquic_field": "must-not-survive",
		"_mihomo_clash_proxy":      map[string]interface{}{"unexpected": "must-not-survive"},
	}

	SanitizeMihomoShadowQUICOutbound(outbound)

	for _, key := range []string{"tls", "detour", "routing_mark", "unknown_shadowquic_field", "_mihomo_clash_proxy", "sni"} {
		if _, exists := outbound[key]; exists {
			t.Fatalf("expected %s to be removed, got %#v", key, outbound)
		}
	}
	for _, key := range []string{"udp_over_stream", "zero_rtt", "keep_alive_interval", "cwnd", "disable_mtu_discovery"} {
		if _, exists := outbound[key]; !exists {
			t.Fatalf("expected explicitly enabled optional key %s to remain, got %#v", key, outbound)
		}
	}
	common, ok := outbound["mihomo_common"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected supported mihomo common fields to remain, got %#v", outbound)
	}
	for _, key := range []string{"udp", "ip_version", "routing_mark"} {
		if _, exists := common[key]; !exists {
			t.Fatalf("expected common field %s to remain, got %#v", key, common)
		}
	}
	for _, key := range []string{"tcp_fast_open", "tcp_multi_path", "smux"} {
		if _, exists := common[key]; exists {
			t.Fatalf("unsupported common field %s must be removed, got %#v", key, common)
		}
	}

	proxy, ok := BuildMihomoShadowQUICClashProxy(outbound, "")
	if !ok {
		t.Fatalf("expected a valid ShadowQUIC Clash proxy, got %#v", outbound)
	}
	if got := proxy["name"]; got != "sq-node" {
		t.Fatalf("unexpected proxy name: %#v", got)
	}
	if got := proxy["port"]; got != 10443 {
		t.Fatalf("unexpected proxy port: %#v", got)
	}
	if got := proxy["quic-versions"]; len(got.([]string)) != 1 || got.([]string)[0] != "v1" {
		t.Fatalf("unexpected quic-versions: %#v", got)
	}
	for _, key := range []string{"udp-over-stream", "zero-rtt", "keep-alive-interval", "cwnd", "disable-mtu-discovery"} {
		if _, exists := proxy[key]; !exists {
			t.Fatalf("expected explicit Clash key %s, got %#v", key, proxy)
		}
	}
	for _, key := range []string{"tls", "detour", "proxy", "rule", "tfo", "mptcp", "smux"} {
		if _, exists := proxy[key]; exists {
			t.Fatalf("unexpected generic field %s in Clash proxy: %#v", key, proxy)
		}
	}
	if proxy["udp"] != true || proxy["ip-version"] != "ipv6" || proxy["routing-mark"] != 100 {
		t.Fatalf("expected supported common fields in Clash proxy, got %#v", proxy)
	}
}

func TestShadowQUICProtocolNormalizers(t *testing.T) {
	alpn := NormalizeMihomoShadowQUICALPN([]interface{}{"h3", "invalid", "h2", "http/1.1", "h3"})
	if len(alpn) != 3 || alpn[0] != "h3" || alpn[1] != "h2" || alpn[2] != "http/1.1" {
		t.Fatalf("unexpected normalized alpn: %#v", alpn)
	}
	versions := NormalizeMihomoShadowQUICVersions([]interface{}{"invalid", "v2", "v1", "v2"})
	if len(versions) != 2 || versions[0] != "v2" || versions[1] != "v1" {
		t.Fatalf("unexpected normalized quic versions: %#v", versions)
	}
	if controller, ok := NormalizeMihomoShadowQUICCongestionController("BBR"); !ok || controller != "bbr" {
		t.Fatalf("unexpected normalized congestion controller: %q, %v", controller, ok)
	}
}

func TestShadowQUICInboundTemplateDropsCredentials(t *testing.T) {
	template := map[string]interface{}{
		"type":        "shadowquic",
		"tag":         "sq-in",
		"server":      "edge.example.com",
		"server_port": 10443,
		"username":    "template-user",
		"password":    "template-password",
		"zero_rtt":    false,
	}

	SanitizeMihomoShadowQUICInboundTemplate(template)

	for _, key := range []string{"username", "password"} {
		if _, exists := template[key]; exists {
			t.Fatalf("inbound template must not retain %s: %#v", key, template)
		}
	}
	if got, _ := template["server"].(string); got != "edge.example.com" {
		t.Fatalf("expected transport fields to remain, got %#v", template)
	}
	if value, exists := template["zero_rtt"]; !exists || value != false {
		t.Fatalf("expected explicit optional field to remain, got %#v", template)
	}
}

func TestShadowQUICClashProxyCanNormalizeStoredClashShape(t *testing.T) {
	stored := map[string]interface{}{
		"name":                  "stored-sq",
		"type":                  "shadowquic",
		"server":                "edge.example.com",
		"port":                  443,
		"username":              "alice",
		"password":              "secret",
		"quic-versions":         []interface{}{"v1"},
		"disable-mtu-discovery": false,
		"tls":                   true,
	}

	proxy, ok := BuildMihomoShadowQUICClashProxy(stored, "stored-sq")
	if !ok {
		t.Fatalf("expected stored Clash shape to normalize, got %#v", stored)
	}
	if got := proxy["port"]; got != 443 {
		t.Fatalf("unexpected normalized port: %#v", got)
	}
	if _, exists := proxy["tls"]; exists {
		t.Fatalf("stored tls must not survive normalization: %#v", proxy)
	}
}

func TestShadowQUICOutboundValidationReportsRequiredFields(t *testing.T) {
	valid := map[string]interface{}{
		"type":        "shadowquic",
		"tag":         "sq-node",
		"server":      "edge.example.com",
		"server_port": 443,
		"username":    "alice",
		"password":    "secret",
	}
	if err := ValidateMihomoShadowQUICOutbound(valid, ""); err != nil {
		t.Fatalf("expected valid ShadowQUIC outbound, got %v", err)
	}

	for name, invalid := range map[string]map[string]interface{}{
		"missing tag": {
			"type": "shadowquic", "server": "edge.example.com", "server_port": 443, "username": "alice", "password": "secret",
		},
		"missing server": {
			"type": "shadowquic", "tag": "sq-node", "server_port": 443, "username": "alice", "password": "secret",
		},
		"invalid port": {
			"type": "shadowquic", "tag": "sq-node", "server": "edge.example.com", "server_port": 65536, "username": "alice", "password": "secret",
		},
		"missing username": {
			"type": "shadowquic", "tag": "sq-node", "server": "edge.example.com", "server_port": 443, "password": "secret",
		},
		"missing password": {
			"type": "shadowquic", "tag": "sq-node", "server": "edge.example.com", "server_port": 443, "username": "alice",
		},
	} {
		t.Run(name, func(t *testing.T) {
			if err := ValidateMihomoShadowQUICOutbound(invalid, ""); err == nil {
				t.Fatalf("expected validation error for %#v", invalid)
			}
		})
	}
}

func TestShadowQUICBBRProfileSanitization(t *testing.T) {
	outbound := map[string]interface{}{
		"type":        "shadowquic",
		"tag":         "sq-node",
		"server":      "edge.example.com",
		"server_port": 443,
		"username":    "alice",
		"password":    "secret",
		"bbr_profile": "AGGRESSIVE",
	}
	SanitizeMihomoShadowQUICOutbound(outbound)
	if got := outbound["bbr_profile"]; got != "aggressive" {
		t.Fatalf("expected normalized BBR profile, got %#v", got)
	}

	outbound["bbr_profile"] = "unsupported"
	SanitizeMihomoShadowQUICOutbound(outbound)
	if _, exists := outbound["bbr_profile"]; exists {
		t.Fatalf("invalid BBR profile must be removed: %#v", outbound)
	}
}
