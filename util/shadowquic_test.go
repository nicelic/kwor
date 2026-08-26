package util

import (
	"fmt"
	"reflect"
	"testing"
)

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

	invalid := []string{"", "2001:db8::1:443", "[not-an-ip]:443", "host:+443", "host:0", "host:65536"}
	for _, value := range invalid {
		if err := ValidateMihomoShadowQUICJLSUpstreamAddr(value); err == nil {
			t.Fatalf("expected %q to be invalid", value)
		}
	}
}

func TestShadowQUICJLSUpstreamAddrNormalization(t *testing.T) {
	tests := []struct{ input, addr, host string }{
		{"www.example.com", "www.example.com:443", "www.example.com"},
		{"1.1.1.1", "1.1.1.1:443", "1.1.1.1"},
		{"[2001:db8::1]", "[2001:db8::1]:443", "2001:db8::1"},
		{"www.example.com:8808", "www.example.com:8808", "www.example.com"},
	}
	for _, test := range tests {
		addr, host, err := NormalizeMihomoShadowQUICJLSUpstreamAddr(test.input)
		if err != nil || addr != test.addr || host != test.host {
			t.Fatalf("normalize %q = %q, %q, %v", test.input, addr, host, err)
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
		"up":                    "100",
		"down":                  float64(200),
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
	for key, want := range map[string]int{"up": 100, "down": 200} {
		if got, ok := outbound[key].(int); !ok || got != want {
			t.Fatalf("sanitized %s = %#v, want numeric %d; outbound=%#v", key, outbound[key], want, outbound)
		}
		if got, ok := proxy[key].(int); !ok || got != want {
			t.Fatalf("Clash proxy %s = %#v, want numeric %d; proxy=%#v", key, proxy[key], want, proxy)
		}
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

func TestShadowQUICBandwidthRequiresNonNegativeIntegers(t *testing.T) {
	outbound := map[string]interface{}{
		"type":        "shadowquic",
		"tag":         "sq-bandwidth",
		"server":      "edge.example.com",
		"server_port": 10443,
		"username":    "alice",
		"password":    "secret",
		"up":          "500",
		"down":        600,
	}

	SanitizeMihomoShadowQUICOutbound(outbound)
	for key, want := range map[string]int{"up": 500, "down": 600} {
		if got, ok := outbound[key].(int); !ok || got != want {
			t.Fatalf("sanitized %s = %#v, want numeric %d; outbound=%#v", key, outbound[key], want, outbound)
		}
	}

	outbound["up"] = "500 Mbps"
	outbound["down"] = -1
	SanitizeMihomoShadowQUICOutbound(outbound)
	for _, key := range []string{"up", "down"} {
		if _, exists := outbound[key]; exists {
			t.Fatalf("invalid ShadowQUIC bandwidth %s must be removed: %#v", key, outbound)
		}
	}
}

func TestShadowQUICOutboundEmptyOptionalFieldsAreOmitted(t *testing.T) {
	outbound := map[string]interface{}{
		"type":                    "shadowquic",
		"tag":                     "sq-empty-options",
		"server":                  "edge.example.com",
		"server_port":             10443,
		"username":                "alice",
		"password":                "secret",
		"sni":                     "",
		"alpn":                    []interface{}{},
		"quic_versions":           []interface{}{},
		"zero_rtt":                "",
		"keep_alive_interval":     "",
		"congestion_controller":   "",
		"up":                      "",
		"down":                    "",
		"cwnd":                    "",
		"bbr_profile":             "",
		"max_datagram_frame_size": "",
		"max_open_streams":        "",
		"recv_window_conn":        "",
		"recv_window":             "",
		"disable_mtu_discovery":   "",
	}

	SanitizeMihomoShadowQUICOutbound(outbound)

	for _, key := range []string{
		"sni", "alpn", "quic_versions", "zero_rtt", "keep_alive_interval", "congestion_controller",
		"up", "down", "cwnd", "bbr_profile", "max_datagram_frame_size", "max_open_streams",
		"recv_window_conn", "recv_window", "disable_mtu_discovery",
	} {
		if _, exists := outbound[key]; exists {
			t.Fatalf("empty optional field %s must not survive outbound sanitization: %#v", key, outbound)
		}
	}

	proxy, ok := BuildMihomoShadowQUICClashProxy(outbound, "")
	if !ok {
		t.Fatalf("expected required ShadowQUIC outbound to remain valid: %#v", outbound)
	}
	for _, key := range []string{
		"sni", "alpn", "quic-versions", "zero-rtt", "keep-alive-interval", "congestion-controller",
		"up", "down", "cwnd", "bbr-profile", "max-datagram-frame-size", "max-open-streams",
		"recv-window-conn", "recv-window", "disable-mtu-discovery",
	} {
		if _, exists := proxy[key]; exists {
			t.Fatalf("empty optional field %s must not be generated in Clash proxy: %#v", key, proxy)
		}
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
	if ipVersion, ok := NormalizeMihomoClientIPVersion(" IPv6-Prefer "); !ok || ipVersion != "ipv6-prefer" {
		t.Fatalf("unexpected normalized IP version: %q, %v", ipVersion, ok)
	}
	if _, ok := NormalizeMihomoClientIPVersion("unsupported"); ok {
		t.Fatal("unsupported Mihomo IP version must be rejected")
	}
}

func TestShadowQUICOutboundSanitizerNormalizesControlValues(t *testing.T) {
	outbound := map[string]interface{}{
		"type":                  "shadowquic",
		"tag":                   "sq-controls",
		"server":                "edge.example.com",
		"server_port":           10443,
		"username":              "alice",
		"password":              "secret",
		"quic_versions":         []interface{}{"v2", "unsupported", "v1", "v2"},
		"congestion_controller": "BBR",
		"mihomo_common": map[string]interface{}{
			"udp":          false,
			"ip_version":   " IPv4-Prefer ",
			"routing_mark": float64(0),
		},
	}

	SanitizeMihomoShadowQUICOutbound(outbound)

	if got := fmt.Sprint(outbound["quic_versions"]); got != "[v2 v1]" {
		t.Fatalf("unexpected normalized quic_versions: %#v", outbound["quic_versions"])
	}
	if got := outbound["congestion_controller"]; got != "bbr" {
		t.Fatalf("unexpected normalized congestion_controller: %#v", got)
	}
	common, ok := outbound["mihomo_common"].(map[string]interface{})
	if !ok || common["udp"] != false || common["ip_version"] != "ipv4-prefer" || common["routing_mark"] != 0 {
		t.Fatalf("unexpected normalized common fields: %#v", outbound["mihomo_common"])
	}

	outbound["quic_versions"] = []interface{}{"unsupported"}
	outbound["congestion_controller"] = "unsupported"
	outbound["mihomo_common"] = map[string]interface{}{
		"ip_version":   "unsupported",
		"routing_mark": -1,
	}
	SanitizeMihomoShadowQUICOutbound(outbound)
	for _, key := range []string{"quic_versions", "congestion_controller", "mihomo_common"} {
		if _, exists := outbound[key]; exists {
			t.Fatalf("invalid %s survived sanitizer: %#v", key, outbound)
		}
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

func TestShadowQUICClashProxySanitizerPreservesFinalMihomoCommonFields(t *testing.T) {
	stored := map[string]interface{}{
		"name":                     "stored-sq",
		"type":                     "shadowquic",
		"server":                   "edge.example.com",
		"port":                     float64(443),
		"username":                 "alice",
		"password":                 "secret",
		"sni":                      "edge.example.com",
		"quic-versions":            []interface{}{"v2", "invalid", "v1"},
		"udp-over-stream":          false,
		"zero-rtt":                 false,
		"keep-alive-interval":      "10000",
		"congestion-controller":    "BBR",
		"max-open-streams":         float64(1024),
		"disable-mtu-discovery":    false,
		"udp":                      true,
		"ip-version":               " IPv6-Prefer ",
		"routing-mark":             float64(100),
		"routing_mark":             101,
		"tls":                      true,
		"detour":                   "must-not-survive",
		"dialer-proxy":             "must-not-survive",
		"rule":                     "must-not-survive",
		"proxy":                    "must-not-survive",
		"unknown_shadowquic_field": "must-not-survive",
	}

	proxy, ok := SanitizeMihomoShadowQUICClashProxy(stored, "")
	if !ok {
		t.Fatalf("expected stored Clash proxy to sanitize, got %#v", stored)
	}
	if proxy["udp"] != true || proxy["ip-version"] != "ipv6-prefer" || proxy["routing-mark"] != 100 {
		t.Fatalf("expected final Mihomo common fields to remain, got %#v", proxy)
	}
	if got := fmt.Sprint(proxy["quic-versions"]); got != "[v2 v1]" {
		t.Fatalf("unexpected normalized quic versions: %#v", proxy["quic-versions"])
	}
	for _, key := range []string{"tls", "detour", "dialer-proxy", "rule", "proxy", "routing_mark", "unknown_shadowquic_field"} {
		if _, exists := proxy[key]; exists {
			t.Fatalf("unexpected stored field %s in sanitized proxy: %#v", key, proxy)
		}
	}

	again, ok := SanitizeMihomoShadowQUICClashProxy(proxy, "")
	if !ok || !reflect.DeepEqual(again, proxy) {
		t.Fatalf("ShadowQUIC Clash sanitizer must be idempotent: first=%#v second=%#v", proxy, again)
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
