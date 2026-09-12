package service

import (
	"testing"

	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/util"
)

func TestSupportsMihomoRuntimeListenerType_Hysteria2Realm(t *testing.T) {
	if !util.SupportsMihomoRuntimeListenerType("hysteria2-realm") {
		t.Fatalf("expected hysteria2-realm to be supported in mihomo runtime listener types")
	}
	if !util.IsSubscriptionServerOnlyInboundType("hysteria2-realm") {
		t.Fatalf("expected hysteria2-realm to be recognized as server-only inbound type")
	}
}

func TestMihomoInboundUserManagement_Hysteria2Realm(t *testing.T) {
	meta := buildMihomoInboundUserManagement("hysteria2-realm", 0)
	if meta.Selectable {
		t.Fatalf("expected Selectable to be false for hysteria2-realm")
	}
	if meta.UsesUsersField {
		t.Fatalf("expected UsesUsersField to be false for hysteria2-realm")
	}
	if meta.Mode != "not_applicable" {
		t.Fatalf("expected mode not_applicable, got %q", meta.Mode)
	}
	if meta.Reason != "rendezvous_server" {
		t.Fatalf("expected reason rendezvous_server, got %q", meta.Reason)
	}
}

func TestNormalizeMihomoHysteria2RealmListener(t *testing.T) {
	raw := map[string]interface{}{
		"type":                 "hysteria2-realm",
		"token":                "",
		"users":                []interface{}{"alice", "bob"},
		"up_mbps":              100,
		"down_mbps":            200,
		"server_up_mbps":       100,
		"server_down_mbps":     200,
		"obfs":                 map[string]interface{}{"password": "pass"},
		"masquerade":           "https://bing.com",
		"detour":               "direct",
		"routing_mark":         1234,
		"rule":                 "rule-a",
		"proxy":                "proxy-a",
		"max_realms":           5000,
		"max_realms_per_ip":    2,
		"trusted_proxy_header": "X-Forwarded-For",
		"realm_name_pattern":   "^[a-z0-9]+$",
	}

	normalizeMihomoHysteria2RealmListener(raw)

	if raw["token"] != "public" {
		t.Fatalf("expected default token public, got %v", raw["token"])
	}
	if raw["max-realms"] != 5000 {
		t.Fatalf("expected max-realms 5000, got %v", raw["max-realms"])
	}
	if _, exists := raw["max_realms"]; exists {
		t.Fatalf("max_realms should have been deleted")
	}
	if raw["max-realms-per-ip"] != 2 {
		t.Fatalf("expected max-realms-per-ip 2, got %v", raw["max-realms-per-ip"])
	}
	if _, exists := raw["max_realms_per_ip"]; exists {
		t.Fatalf("max_realms_per_ip should have been deleted")
	}
	if raw["trusted-proxy-header"] != "X-Forwarded-For" {
		t.Fatalf("expected trusted-proxy-header X-Forwarded-For, got %v", raw["trusted-proxy-header"])
	}
	if raw["realm-name-pattern"] != "^[a-z0-9]+$" {
		t.Fatalf("expected realm-name-pattern ^[a-z0-9]+$, got %v", raw["realm-name-pattern"])
	}
	for _, forbidden := range []string{"users", "up_mbps", "down_mbps", "server_up_mbps", "server_down_mbps", "obfs", "masquerade", "detour", "routing_mark", "rule", "proxy"} {
		if _, exists := raw[forbidden]; exists {
			t.Fatalf("forbidden key %q was not removed", forbidden)
		}
	}
}

func TestBuildMihomoListener_Hysteria2RealmStripsRouting(t *testing.T) {
	inbound := model.MihomoInbound{
		Tag:  "my-realm",
		Type: "hysteria2-realm",
	}
	ref := mihomoInboundRouteRef{
		RuleName:    "subrule-1",
		ProxyTarget: "direct",
	}

	payload := map[string]interface{}{
		"type":        "hysteria2-realm",
		"listen":      "0.0.0.0",
		"listen_port": 8443,
		"detour":      "direct",
		"token":       "secret",
	}

	listener := buildMihomoListener(inbound, payload, ref)
	if listener == nil {
		t.Fatalf("expected non-nil listener")
	}
	if listener["type"] != "hysteria2-realm" {
		t.Fatalf("expected type hysteria2-realm, got %v", listener["type"])
	}
	if listener["name"] != "my-realm" {
		t.Fatalf("expected name my-realm, got %v", listener["name"])
	}
	if _, exists := listener["proxy"]; exists {
		t.Fatalf("expected proxy to be stripped from hysteria2-realm listener")
	}
	if _, exists := listener["rule"]; exists {
		t.Fatalf("expected rule to be stripped from hysteria2-realm listener")
	}
	if _, exists := listener["detour"]; exists {
		t.Fatalf("expected detour to be stripped from hysteria2-realm listener")
	}
}
