package sub

import (
	"testing"
)

func TestClashHysteria2RealmOpts_GenerationAndSanitize(t *testing.T) {
	cs := &ClashService{}

	outbounds := []map[string]interface{}{
		{
			"type":        "hysteria2",
			"tag":         "hy2-node",
			"server":      "127.0.0.1",
			"server_port": 443,
			"password":    "pass",
			"realm_opts": map[string]interface{}{
				"enable":       true,
				"server_url":   "https://realm.hy2.io",
				"token":        "public",
				"realm_id":     "my-room",
				"stun_servers": []string{"stun.hy2.io:3478"},
				"proxy":        "http://127.0.0.1:7890",
				"alpn":         []string{"h3", "h2"},
			},
		},
	}

	result, err := cs.convertToClashMetaMap(&outbounds, "", 0, 0, nil, true)
	if err != nil {
		t.Fatalf("convertToClashMetaMap failed: %v", err)
	}

	proxies, ok := result["proxies"].([]interface{})
	if !ok || len(proxies) == 0 {
		t.Fatalf("expected proxies in result")
	}

	proxy, ok := proxies[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected proxy map")
	}

	realmOpts, ok := proxy["realm-opts"].(map[string]interface{})
	if !ok || realmOpts == nil {
		t.Fatalf("expected realm-opts in generated proxy")
	}
	if realmOpts["enable"] != true {
		t.Errorf("expected enable=true, got %v", realmOpts["enable"])
	}
	if realmOpts["server-url"] != "https://realm.hy2.io" {
		t.Errorf("expected server-url=https://realm.hy2.io, got %v", realmOpts["server-url"])
	}
	if realmOpts["realm-id"] != "my-room" {
		t.Errorf("expected realm-id=my-room, got %v", realmOpts["realm-id"])
	}
	if _, exists := realmOpts["proxy"]; exists {
		t.Errorf("expected proxy to be stripped from clash proxy realm-opts")
	}
	alpn, ok := realmOpts["alpn"].([]string)
	if !ok || len(alpn) != 2 || alpn[0] != "h3" || alpn[1] != "h2" {
		t.Errorf("expected alpn to be preserved in clash proxy realm-opts, got %v", realmOpts["alpn"])
	}

	// stripSubscriptionClashProxyPanelFields should remove internal realm_opts but keep realm-opts
	proxy["realm_opts"] = "should-be-deleted"
	stripSubscriptionClashProxyPanelFields(proxy)
	if _, exists := proxy["realm_opts"]; exists {
		t.Errorf("expected realm_opts to be removed by stripSubscriptionClashProxyPanelFields")
	}
	if _, exists := proxy["realm-opts"]; !exists {
		t.Errorf("expected realm-opts to be preserved in final clash proxy")
	}
}
