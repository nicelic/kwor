package sub

import "testing"

func TestAddDefaultOutbounds_UsesSelectorStyle(t *testing.T) {
	svc := &JsonService{}
	outbounds := []map[string]interface{}{
		{"type": "vmess", "tag": "vmess-1"},
	}
	outTags := []string{"vmess-1"}

	svc.addDefaultOutbounds(&outbounds, &outTags, "https://cp.cloudflare.com/generate_204", "3m", 50, nil)

	wantPrefix := []string{
		nodeSelectorTag,
		autoSelectorTag,
		globalDirectSelectorTag,
		globalBlockSelectorTag,
		finalSelectorTag,
		globalSelectorTag,
		"direct",
		"block",
	}

	if len(outbounds) != len(wantPrefix)+1 {
		t.Fatalf("expected %d outbounds, got %d", len(wantPrefix)+1, len(outbounds))
	}

	for i, want := range wantPrefix {
		got, _ := outbounds[i]["tag"].(string)
		if got != want {
			t.Fatalf("index=%d expected tag %q, got %q", i, want, got)
		}
	}

	if got, _ := outbounds[len(wantPrefix)]["tag"].(string); got != "vmess-1" {
		t.Fatalf("expected last tag vmess-1, got %q", got)
	}
}

func TestNormalizeRouteRuleMap_MapsLegacyOutbounds(t *testing.T) {
	rule := map[string]interface{}{
		"action":   "route",
		"outbound": "proxy",
	}
	got := normalizeRouteRuleMap(rule)
	if outbound, _ := got["outbound"].(string); outbound != nodeSelectorTag {
		t.Fatalf("expected proxy -> %q, got %q", nodeSelectorTag, outbound)
	}

	rule = map[string]interface{}{
		"action":   "route",
		"outbound": "direct",
	}
	got = normalizeRouteRuleMap(rule)
	if outbound, _ := got["outbound"].(string); outbound != globalDirectSelectorTag {
		t.Fatalf("expected direct -> %q, got %q", globalDirectSelectorTag, outbound)
	}

	rule = map[string]interface{}{
		"action":     "route",
		"clash_mode": "Global",
		"outbound":   "proxy",
	}
	got = normalizeRouteRuleMap(rule)
	if outbound, _ := got["outbound"].(string); outbound != globalSelectorTag {
		t.Fatalf("expected clash_mode global -> %q, got %q", globalSelectorTag, outbound)
	}
}

func TestNormalizeRouteRuleMap_RejectRuleRemovesOutbound(t *testing.T) {
	rule := map[string]interface{}{
		"action":   "route",
		"outbound": "block",
	}
	got := normalizeRouteRuleMap(rule)
	if action, _ := got["action"].(string); action != "reject" {
		t.Fatalf("expected action reject, got %q", action)
	}
	if _, exists := got["outbound"]; exists {
		t.Fatalf("reject rule should not contain outbound: %#v", got)
	}
}

func TestNormalizeRouteFinalOutbound_UsesSelectorTags(t *testing.T) {
	if got := normalizeRouteFinalOutbound("proxy"); got != finalSelectorTag {
		t.Fatalf("expected proxy -> %q, got %q", finalSelectorTag, got)
	}
	if got := normalizeRouteFinalOutbound("direct"); got != globalDirectSelectorTag {
		t.Fatalf("expected direct -> %q, got %q", globalDirectSelectorTag, got)
	}
	if got := normalizeRouteFinalOutbound("global-proxy"); got != globalSelectorTag {
		t.Fatalf("expected global-proxy -> %q, got %q", globalSelectorTag, got)
	}
}

func TestAddDefaultOutbounds_AppendsNamedSelectorGroups(t *testing.T) {
	svc := &JsonService{}
	outbounds := []map[string]interface{}{
		{"type": "vmess", "tag": "node-a"},
	}
	outTags := []string{"node-a"}
	groups := []selectorGroupConfig{
		{Tag: "CN", DefaultOutbound: nodeSelectorTag},
	}

	svc.addDefaultOutbounds(&outbounds, &outTags, "https://cp.cloudflare.com/generate_204", "3m", 50, groups)

	found := false
	for _, outbound := range outbounds {
		tag, _ := outbound["tag"].(string)
		if tag != "CN" {
			continue
		}
		found = true
		if typ, _ := outbound["type"].(string); typ != "selector" {
			t.Fatalf("expected CN type selector, got %q", typ)
		}
		options, _ := outbound["outbounds"].([]string)
		if len(options) == 0 {
			t.Fatalf("expected CN selector options")
		}
		if options[0] != nodeSelectorTag {
			t.Fatalf("expected CN default option %q, got %q", nodeSelectorTag, options[0])
		}
		hasNodeA := false
		for _, option := range options {
			if option == "node-a" {
				hasNodeA = true
				break
			}
		}
		if !hasNodeA {
			t.Fatalf("expected CN selector to include node-a options: %#v", options)
		}
	}

	if !found {
		t.Fatalf("expected named selector CN in outbounds")
	}
}

func TestNormalizeNaiveSubscriptionOutbound_RemovesNetworkOnly(t *testing.T) {
	outbound := map[string]interface{}{
		"type":                    "naive",
		"tag":                     "naive-1",
		"network":                 "udp",
		"quic":                    false,
		"quic_congestion_control": "bbr2",
		"insecure_concurrency":    0,
		"udp_over_tcp":            false,
		"tls": map[string]interface{}{
			"alpn": []interface{}{"h2", "http/1.1"},
			"utls": map[string]interface{}{
				"enabled":     true,
				"fingerprint": "chrome",
			},
			"server_name": "edge.example.com",
		},
	}

	normalizeNaiveSubscriptionOutbound(outbound)

	if _, exists := outbound["network"]; exists {
		t.Fatalf("naive json subscription must not contain network: %#v", outbound)
	}
	if got, _ := outbound["quic"].(bool); got {
		t.Fatalf("expected quic=false to be preserved, got %#v", outbound["quic"])
	}
	if got, _ := outbound["quic_congestion_control"].(string); got != "bbr2" {
		t.Fatalf("expected quic_congestion_control to be preserved, got %#v", outbound["quic_congestion_control"])
	}
	if got, _ := outbound["udp_over_tcp"].(bool); got {
		t.Fatalf("expected udp_over_tcp=false to be preserved, got %#v", outbound["udp_over_tcp"])
	}
	tlsMap, ok := outbound["tls"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected tls map to remain, got %#v", outbound["tls"])
	}
	if _, exists := tlsMap["alpn"]; exists {
		t.Fatalf("naive json subscription must not contain tls.alpn: %#v", tlsMap)
	}
	if _, exists := tlsMap["utls"]; exists {
		t.Fatalf("naive json subscription must not contain tls.utls: %#v", tlsMap)
	}
	if got, _ := tlsMap["server_name"].(string); got != "edge.example.com" {
		t.Fatalf("expected unrelated tls fields to remain, got %#v", tlsMap["server_name"])
	}
}

func TestNormalizeNaiveSubscriptionOutbound_RemovesEmptyQCC(t *testing.T) {
	outbound := map[string]interface{}{
		"type":                    "naive",
		"tag":                     "naive-1",
		"quic_congestion_control": "",
	}

	normalizeNaiveSubscriptionOutbound(outbound)

	if _, exists := outbound["quic_congestion_control"]; exists {
		t.Fatalf("empty quic_congestion_control should be removed: %#v", outbound)
	}
}
