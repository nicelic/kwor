package service

import (
	"encoding/json"
	"testing"
)

func TestNormalizeSubRouteRuleMap_MapsLegacyOutbounds(t *testing.T) {
	rule := map[string]interface{}{
		"action":   "route",
		"outbound": "proxy",
	}
	got := normalizeSubRouteRuleMap(rule)
	if outbound, _ := got["outbound"].(string); outbound != nodeSelectorTag {
		t.Fatalf("expected proxy -> %q, got %q", nodeSelectorTag, outbound)
	}

	rule = map[string]interface{}{
		"action":   "route",
		"outbound": "direct",
	}
	got = normalizeSubRouteRuleMap(rule)
	if outbound, _ := got["outbound"].(string); outbound != globalDirectSelectorTag {
		t.Fatalf("expected direct -> %q, got %q", globalDirectSelectorTag, outbound)
	}

	rule = map[string]interface{}{
		"action":     "route",
		"clash_mode": "Global",
		"outbound":   "proxy",
	}
	got = normalizeSubRouteRuleMap(rule)
	if outbound, _ := got["outbound"].(string); outbound != globalSelectorTag {
		t.Fatalf("expected clash_mode global -> %q, got %q", globalSelectorTag, outbound)
	}
}

func TestBuildSubJsonFullConfig_NormalizesDetourAndRouteFinal(t *testing.T) {
	outbounds := []map[string]interface{}{
		{"type": "vmess", "tag": "node-1"},
	}
	others := map[string]interface{}{
		"dns": map[string]interface{}{
			"servers": []interface{}{
				map[string]interface{}{"tag": "proxy-dns", "detour": "proxy"},
			},
		},
		"rule_set": []interface{}{
			map[string]interface{}{"tag": "geosite-cn", "download_detour": "proxy"},
		},
		"rules": []interface{}{
			map[string]interface{}{"action": "route", "outbound": "proxy"},
		},
		"route_final": "proxy",
	}
	othersRaw, err := json.Marshal(others)
	if err != nil {
		t.Fatalf("marshal others failed: %v", err)
	}

	cfg := buildSubJsonFullConfig(outbounds, string(othersRaw))
	route, _ := cfg["route"].(map[string]interface{})
	if route == nil {
		t.Fatalf("route should exist")
	}
	if got, _ := route["final"].(string); got != finalSelectorTag {
		t.Fatalf("expected route.final %q, got %q", finalSelectorTag, got)
	}

	ruleSet, _ := route["rule_set"].([]interface{})
	if len(ruleSet) != 1 {
		t.Fatalf("expected 1 rule_set item, got %d", len(ruleSet))
	}
	ruleSetMap, _ := ruleSet[0].(map[string]interface{})
	httpClient, _ := ruleSetMap["http_client"].(map[string]interface{})
	if httpClient == nil {
		t.Fatalf("expected http_client to exist, got %#v", ruleSetMap["http_client"])
	}
	if got, _ := httpClient["detour"].(string); got != nodeSelectorTag {
		t.Fatalf("expected http_client.detour %q, got %q", nodeSelectorTag, got)
	}

	rules, _ := route["rules"].([]interface{})
	if len(rules) != 1 {
		t.Fatalf("expected 1 route rule, got %d", len(rules))
	}
	ruleMap, _ := rules[0].(map[string]interface{})
	if got, _ := ruleMap["outbound"].(string); got != nodeSelectorTag {
		t.Fatalf("expected route outbound %q, got %q", nodeSelectorTag, got)
	}

	dns, _ := cfg["dns"].(map[string]interface{})
	if dns == nil {
		t.Fatalf("dns should exist")
	}
	servers, _ := dns["servers"].([]interface{})
	if len(servers) != 1 {
		t.Fatalf("expected 1 dns server, got %d", len(servers))
	}
	serverMap, _ := servers[0].(map[string]interface{})
	if got, _ := serverMap["detour"].(string); got != nodeSelectorTag {
		t.Fatalf("expected dns detour %q, got %q", nodeSelectorTag, got)
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

func TestBuildNamedSelectorOutbounds_IncludesNodeTags(t *testing.T) {
	groups := []selectorGroupConfig{
		{Tag: "CN", DefaultOutbound: nodeSelectorTag},
	}
	outbounds := buildNamedSelectorOutbounds(groups, []string{"vmess-1", "hy1-2"})
	if len(outbounds) != 1 {
		t.Fatalf("expected 1 selector outbound, got %d", len(outbounds))
	}
	selector := outbounds[0]
	if tag, _ := selector["tag"].(string); tag != "CN" {
		t.Fatalf("expected selector tag CN, got %q", tag)
	}
	options, _ := selector["outbounds"].([]string)
	if len(options) == 0 {
		t.Fatalf("expected selector options")
	}
	if options[0] != nodeSelectorTag {
		t.Fatalf("expected first option %q, got %q", nodeSelectorTag, options[0])
	}
	hasNode := false
	for _, option := range options {
		if option == "vmess-1" {
			hasNode = true
			break
		}
	}
	if !hasNode {
		t.Fatalf("expected options include vmess-1, got %#v", options)
	}
}
