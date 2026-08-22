package sub

import (
	"testing"

	"github.com/alireza0/s-ui/database/model"
)

func TestGetOutboundsForNamespaceIsolatesAddressTLSOverrides(t *testing.T) {
	inbound := &model.Inbound{
		Type:  "trojan",
		Tag:   "trojan-node",
		TlsId: 1,
		Addrs: mustRawJSON(t, []map[string]interface{}{
			{
				"server":      "one.example.com",
				"server_port": 443,
				"remark":      "-one",
				"tls": map[string]interface{}{
					"server_name": "one.sni.example.com",
					"alpn":        []string{"h2"},
					"utls": map[string]interface{}{
						"fingerprint": "chrome",
					},
				},
			},
			{
				"server":      "two.example.com",
				"server_port": 8443,
				"remark":      "-two",
				"tls": map[string]interface{}{
					"server_name": "two.sni.example.com",
					"alpn":        []string{"h3"},
					"utls": map[string]interface{}{
						"fingerprint": "firefox",
					},
				},
			},
		}),
		OutJson: mustRawJSON(t, map[string]interface{}{
			"type":        "trojan",
			"tag":         "trojan-node",
			"server":      "base.example.com",
			"server_port": 443,
			"tls": map[string]interface{}{
				"enabled":     true,
				"server_name": "base.sni.example.com",
				"utls": map[string]interface{}{
					"enabled": true,
				},
			},
		}),
	}

	clientConfig := mustRawJSON(t, map[string]interface{}{
		"trojan": map[string]interface{}{"password": "secret"},
	})
	for _, namespace := range []string{"default", "clash", "mihomo"} {
		t.Run(namespace, func(t *testing.T) {
			outbounds, tags, err := (&JsonService{}).getOutboundsForNamespace(
				"alice",
				clientConfig,
				[]*model.Inbound{inbound},
				namespace,
			)
			if err != nil {
				t.Fatalf("getOutboundsForNamespace failed: %v", err)
			}

			if len(*outbounds) != 2 || len(*tags) != 2 {
				t.Fatalf("expected two address variants, got outbounds=%d tags=%d", len(*outbounds), len(*tags))
			}

			first := findOutboundVariantByTag(t, *outbounds, "1.trojan-node-one")
			second := findOutboundVariantByTag(t, *outbounds, "2.trojan-node-two")
			assertAddressTLS(t, first, "one.sni.example.com", "h2", "chrome")
			assertAddressTLS(t, second, "two.sni.example.com", "h3", "firefox")
		})
	}
}

func TestGetOutboundsForNamespaceIsolatesShadowTLSAddressTLSOverrides(t *testing.T) {
	inbound := &model.Inbound{
		Type: "shadowtls",
		Tag:  "shadow-node",
		Addrs: mustRawJSON(t, []map[string]interface{}{
			{
				"server":      "one.example.com",
				"server_port": 443,
				"remark":      "-one",
				"tls":         map[string]interface{}{"server_name": "one.sni.example.com"},
			},
			{
				"server":      "two.example.com",
				"server_port": 8443,
				"remark":      "-two",
				"tls":         map[string]interface{}{"server_name": "two.sni.example.com"},
			},
		}),
		OutJson: mustRawJSON(t, map[string]interface{}{
			"type":        "shadowtls",
			"tag":         "shadow-node",
			"server":      "base.example.com",
			"server_port": 443,
			"version":     3,
			"tls": map[string]interface{}{
				"enabled":     true,
				"server_name": "base.sni.example.com",
			},
			"ss_config": map[string]interface{}{
				"method":   "2022-blake3-aes-128-gcm",
				"password": "ss-secret",
			},
		}),
	}

	outbounds, _, err := (&JsonService{}).getOutboundsForNamespace(
		"alice",
		mustRawJSON(t, map[string]interface{}{
			"shadowtls": map[string]interface{}{"password": "shadow-secret"},
		}),
		[]*model.Inbound{inbound},
		"default",
	)
	if err != nil {
		t.Fatalf("getOutboundsForNamespace failed: %v", err)
	}

	first := findOutboundVariantByTag(t, *outbounds, "1.shadow-node-out-one")
	second := findOutboundVariantByTag(t, *outbounds, "2.shadow-node-out-two")
	assertAddressTLS(t, first, "one.sni.example.com", "", "")
	assertAddressTLS(t, second, "two.sni.example.com", "", "")
}

func findOutboundVariantByTag(t *testing.T, outbounds []map[string]interface{}, tag string) map[string]interface{} {
	t.Helper()

	for _, outbound := range outbounds {
		if outboundTag, _ := outbound["tag"].(string); outboundTag == tag {
			return outbound
		}
	}
	t.Fatalf("outbound variant %q not found", tag)
	return nil
}

func assertAddressTLS(t *testing.T, outbound map[string]interface{}, serverName string, alpn string, fingerprint string) {
	t.Helper()

	tlsConfig, ok := outbound["tls"].(map[string]interface{})
	if !ok || tlsConfig == nil {
		t.Fatalf("outbound %q has no TLS config: %#v", outbound["tag"], outbound)
	}
	if got, _ := tlsConfig["server_name"].(string); got != serverName {
		t.Fatalf("outbound %q has server_name=%q, want %q", outbound["tag"], got, serverName)
	}
	if alpn != "" {
		values, ok := tlsConfig["alpn"].([]interface{})
		if !ok || len(values) != 1 || values[0] != alpn {
			t.Fatalf("outbound %q has alpn=%#v, want %q", outbound["tag"], tlsConfig["alpn"], alpn)
		}
	}
	if fingerprint != "" {
		utls, ok := tlsConfig["utls"].(map[string]interface{})
		if !ok || utls == nil {
			t.Fatalf("outbound %q has no uTLS config: %#v", outbound["tag"], tlsConfig)
		}
		if got, _ := utls["fingerprint"].(string); got != fingerprint {
			t.Fatalf("outbound %q has fingerprint=%q, want %q", outbound["tag"], got, fingerprint)
		}
	}
}
