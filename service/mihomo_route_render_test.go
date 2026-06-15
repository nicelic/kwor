package service

import "testing"

func TestBuildMihomoRuleProviders_ConvertsUpdateIntervalToSeconds(t *testing.T) {
	cases := []struct {
		name  string
		input interface{}
		want  int
	}{
		{name: "24h", input: "24h", want: 24 * 3600},
		{name: "1d", input: "1d", want: 24 * 3600},
		{name: "30m", input: "30m", want: 30 * 60},
		{name: "45s", input: "45s", want: 45},
		{name: "86400", input: "86400", want: 86400},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			providers, _ := buildMihomoRuleProviders([]interface{}{
				map[string]interface{}{
					"tag":             "rs-a",
					"type":            "http",
					"url":             "https://example.com/rules.yaml",
					"format":          "yaml",
					"behavior":        "classical",
					"update_interval": tc.input,
				},
			}, nil)

			provider, ok := providers["rs-a"].(map[string]interface{})
			if !ok {
				t.Fatalf("expected provider map for rs-a, got %#v", providers["rs-a"])
			}

			interval, ok := provider["interval"].(int)
			if !ok {
				t.Fatalf("expected integer interval, got %#v", provider["interval"])
			}
			if interval != tc.want {
				t.Fatalf("interval = %d, want %d", interval, tc.want)
			}
		})
	}
}

func TestBuildMihomoRuleProviders_NormalizesDirectProxyToDIRECT(t *testing.T) {
	targets := convertMihomoOutboundsToClash([]map[string]interface{}{
		{
			"type": "direct",
			"tag":  "direct",
		},
	})

	providers, _ := buildMihomoRuleProviders([]interface{}{
		map[string]interface{}{
			"tag":             "rs-direct",
			"type":            "http",
			"url":             "https://example.com/rules.yaml",
			"format":          "yaml",
			"behavior":        "classical",
			"proxy":           "direct",
			"update_interval": "24h",
		},
	}, targets)

	provider, ok := providers["rs-direct"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected provider map for rs-direct, got %#v", providers["rs-direct"])
	}

	if got, _ := provider["proxy"].(string); got != "DIRECT" {
		t.Fatalf("provider proxy = %#v, want %q", provider["proxy"], "DIRECT")
	}
}

func TestBuildMihomoRuleStrings_MapsExtendedSupportedMatchers(t *testing.T) {
	rule := map[string]interface{}{
		"action":                        "route",
		"outbound":                      "DIRECT",
		"network":                       []interface{}{"tcp"},
		"auth_user":                     []interface{}{"alice"},
		"source_ip_cidr":                []interface{}{"10.0.0.0/8"},
		"source_ip_is_private":          true,
		"source_port_range":             []interface{}{"1000-2000"},
		"port_range":                    []interface{}{"8443-9443"},
		"process_name":                  []interface{}{"mihomo"},
		"process_path":                  []interface{}{"/usr/bin/mihomo"},
		"process_path_regex":            []interface{}{"^/opt/.+/mihomo$"},
		"user_id":                       []interface{}{1000},
		"rule_set":                      []interface{}{"rs-a"},
		"rule_set_ip_cidr_match_source": true,
	}

	got, ok := buildMihomoRuleStrings(rule, map[string]struct{}{"rs-a": {}}, nil, nil, false)
	if !ok {
		t.Fatalf("buildMihomoRuleStrings returned ok=false")
	}

	want := []string{
		"AND,((NETWORK,TCP),(IN-USER,alice),(SRC-IP-CIDR,10.0.0.0/8),(SRC-GEOIP,private),(SRC-PORT,1000-2000),(DST-PORT,8443-9443),(PROCESS-NAME,mihomo),(PROCESS-PATH,/usr/bin/mihomo),(PROCESS-PATH-REGEX,^/opt/.+/mihomo$),(UID,1000),(RULE-SET,rs-a,src)),DIRECT",
	}
	if len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("buildMihomoRuleStrings() = %#v, want %#v", got, want)
	}
}

func TestBuildMihomoRuleStrings_ExpandsNetworkAndPortCombinations(t *testing.T) {
	rule := map[string]interface{}{
		"action":   "route",
		"outbound": "DIRECT",
		"network":  []interface{}{"tcp", "udp"},
		"port":     []interface{}{80, 443},
	}

	got, ok := buildMihomoRuleStrings(rule, nil, nil, nil, false)
	if !ok {
		t.Fatalf("buildMihomoRuleStrings returned ok=false")
	}
	if len(got) != 4 {
		t.Fatalf("expected 4 combinations, got %#v", got)
	}

	expected := map[string]struct{}{
		"AND,((NETWORK,TCP),(DST-PORT,80)),DIRECT":  {},
		"AND,((NETWORK,TCP),(DST-PORT,443)),DIRECT": {},
		"AND,((NETWORK,UDP),(DST-PORT,80)),DIRECT":  {},
		"AND,((NETWORK,UDP),(DST-PORT,443)),DIRECT": {},
	}
	for _, ruleString := range got {
		if _, ok := expected[ruleString]; !ok {
			t.Fatalf("unexpected combination: %q", ruleString)
		}
		delete(expected, ruleString)
	}
	if len(expected) != 0 {
		t.Fatalf("missing combinations: %#v", expected)
	}
}

func TestBuildMihomoRuleStrings_AppendsNoResolveForTargetIPMatchers(t *testing.T) {
	rule := map[string]interface{}{
		"action":   "route",
		"outbound": "DIRECT",
		"ip_cidr":  []interface{}{"1.1.1.0/24"},
	}

	got, ok := buildMihomoRuleStrings(rule, nil, nil, nil, true)
	if !ok {
		t.Fatalf("buildMihomoRuleStrings returned ok=false")
	}
	if len(got) != 1 {
		t.Fatalf("expected one rendered rule, got %#v", got)
	}
	if got[0] != "IP-CIDR,1.1.1.0/24,DIRECT,no-resolve" {
		t.Fatalf("rendered rule = %#v, want %#v", got[0], "IP-CIDR,1.1.1.0/24,DIRECT,no-resolve")
	}
}

func TestRenderMihomoRoutes_RespectsNoResolveGlobalSwitch(t *testing.T) {
	baseRules := []interface{}{
		map[string]interface{}{
			"action":   "route",
			"outbound": "DIRECT",
			"ip_cidr":  []interface{}{"1.1.1.0/24"},
		},
	}

	defaultEnabled := map[string]interface{}{
		"final": "DIRECT",
		"rules": baseRules,
	}
	result := renderMihomoRoutes(defaultEnabled, nil, nil, nil, "", nil, nil)
	if len(result.Rules) < 1 || result.Rules[0] != "IP-CIDR,1.1.1.0/24,DIRECT,no-resolve" {
		t.Fatalf("default no-resolve rules = %#v", result.Rules)
	}

	explicitDisabled := map[string]interface{}{
		"final":      "DIRECT",
		"no_resolve": false,
		"rules":      baseRules,
	}
	disabledResult := renderMihomoRoutes(explicitDisabled, nil, nil, nil, "", nil, nil)
	if len(disabledResult.Rules) < 1 || disabledResult.Rules[0] != "IP-CIDR,1.1.1.0/24,DIRECT" {
		t.Fatalf("disabled no-resolve rules = %#v", disabledResult.Rules)
	}
}

func TestBuildMihomoRuleStrings_AppendsNoResolveForIPRuleSetMatchers(t *testing.T) {
	rule := map[string]interface{}{
		"action":   "route",
		"outbound": "DIRECT",
		"rule_set": []interface{}{"rs-ip"},
	}

	got, ok := buildMihomoRuleStrings(
		rule,
		map[string]struct{}{"rs-ip": {}},
		map[string]struct{}{"rs-ip": {}},
		nil,
		true,
	)
	if !ok {
		t.Fatalf("buildMihomoRuleStrings returned ok=false")
	}
	if len(got) != 1 {
		t.Fatalf("expected one rendered rule, got %#v", got)
	}
	if got[0] != "RULE-SET,rs-ip,DIRECT,no-resolve" {
		t.Fatalf("rendered rule = %#v, want %#v", got[0], "RULE-SET,rs-ip,DIRECT,no-resolve")
	}
}

func TestBuildMihomoRuleStrings_SkipsNoResolveForDomainRuleSetMatchers(t *testing.T) {
	rule := map[string]interface{}{
		"action":   "route",
		"outbound": "DIRECT",
		"rule_set": []interface{}{"rs-domain"},
	}

	got, ok := buildMihomoRuleStrings(
		rule,
		map[string]struct{}{"rs-domain": {}},
		map[string]struct{}{},
		nil,
		true,
	)
	if !ok {
		t.Fatalf("buildMihomoRuleStrings returned ok=false")
	}
	if len(got) != 1 {
		t.Fatalf("expected one rendered rule, got %#v", got)
	}
	if got[0] != "RULE-SET,rs-domain,DIRECT" {
		t.Fatalf("rendered rule = %#v, want %#v", got[0], "RULE-SET,rs-domain,DIRECT")
	}
}

func TestBuildMihomoRuleStrings_AppendsNoResolveForIPRuleSetMatchersWhenSourceMatchEnabled(t *testing.T) {
	rule := map[string]interface{}{
		"action":                        "route",
		"outbound":                      "DIRECT",
		"rule_set":                      []interface{}{"rs-ip"},
		"rule_set_ip_cidr_match_source": true,
	}

	got, ok := buildMihomoRuleStrings(
		rule,
		map[string]struct{}{"rs-ip": {}},
		map[string]struct{}{"rs-ip": {}},
		nil,
		true,
	)
	if !ok {
		t.Fatalf("buildMihomoRuleStrings returned ok=false")
	}
	if len(got) != 1 {
		t.Fatalf("expected one rendered rule, got %#v", got)
	}
	if got[0] != "RULE-SET,rs-ip,src,DIRECT,no-resolve" {
		t.Fatalf("rendered rule = %#v, want %#v", got[0], "RULE-SET,rs-ip,src,DIRECT,no-resolve")
	}
}

func TestRenderMihomoRoutes_RuleSetOnlyWithoutRulesFallsBackToFinalMatch(t *testing.T) {
	route := map[string]interface{}{
		"final": "DIRECT",
		"rule_set": []interface{}{
			map[string]interface{}{
				"tag":      "rs-ip",
				"type":     "file",
				"path":     "./rs-ip.mrs",
				"format":   "mrs",
				"behavior": "ipcidr",
			},
		},
		"rules": []interface{}{},
	}

	result := renderMihomoRoutes(route, nil, nil, nil, "", nil, nil)
	if len(result.Rules) != 1 || result.Rules[0] != "MATCH,DIRECT" {
		t.Fatalf("rules with rule_set-only config = %#v, want %#v", result.Rules, []string{"MATCH,DIRECT"})
	}
}

func TestRenderMihomoRoutes_AppendsNoResolveForIPRuleSetInSubRules(t *testing.T) {
	route := map[string]interface{}{
		"final": "DIRECT",
		"rules": []interface{}{
			map[string]interface{}{
				"action":   "route",
				"outbound": "proxy",
				"inbound":  []interface{}{"in-a"},
				"rule_set": []interface{}{"rs-ip"},
			},
		},
	}

	targets := &mihomoProxyConversionResult{
		SupportedTags: map[string]struct{}{
			"DIRECT": {},
			"proxy":  {},
		},
	}
	inboundRefs := map[string]mihomoInboundRouteRef{
		"in-a": {
			RuleName:      "in-a",
			DefaultTarget: "DIRECT",
		},
	}

	result := renderMihomoRoutes(
		route,
		map[string]struct{}{"rs-ip": {}},
		map[string]struct{}{"rs-ip": {}},
		targets,
		"",
		inboundRefs,
		nil,
	)

	if len(result.Rules) != 1 || result.Rules[0] != "MATCH,DIRECT" {
		t.Fatalf("global rules = %#v, want %#v", result.Rules, []string{"MATCH,DIRECT"})
	}

	subRules, ok := result.SubRules["in-a"]
	if !ok {
		t.Fatalf("expected sub-rules for in-a, got %#v", result.SubRules)
	}
	want := []string{
		"RULE-SET,rs-ip,proxy,no-resolve",
		"MATCH,DIRECT",
	}
	if len(subRules) != len(want) || subRules[0] != want[0] || subRules[1] != want[1] {
		t.Fatalf("sub-rules = %#v, want %#v", subRules, want)
	}
}

func TestSanitizeMihomoRouteRules_KeepsSupportedMatcherFields(t *testing.T) {
	raw := []interface{}{
		map[string]interface{}{
			"action":                        "route",
			"outbound":                      "DIRECT",
			"network":                       []interface{}{"tcp"},
			"auth_user":                     []interface{}{"alice"},
			"source_ip_cidr":                []interface{}{"10.0.0.0/8"},
			"source_ip_is_private":          true,
			"source_port":                   []interface{}{53},
			"source_port_range":             []interface{}{"1000-2000"},
			"port":                          []interface{}{443},
			"port_range":                    []interface{}{"8443-9443"},
			"user_id":                       []interface{}{1000},
			"rule_set_ip_cidr_match_source": true,
			"process_name":                  []interface{}{"mihomo"},
			"protocol":                      []interface{}{"tls"},
			"package_name":                  []interface{}{"com.example.app"},
			"clash_mode":                    "global",
		},
	}

	sanitized := sanitizeMihomoRouteRules(raw)
	if len(sanitized) != 1 {
		t.Fatalf("expected one sanitized rule, got %#v", sanitized)
	}

	rule, ok := sanitized[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected sanitized rule map, got %#v", sanitized[0])
	}

	for _, key := range []string{
		"network",
		"auth_user",
		"source_ip_cidr",
		"source_ip_is_private",
		"source_port",
		"source_port_range",
		"port",
		"port_range",
		"user_id",
		"rule_set_ip_cidr_match_source",
		"process_name",
	} {
		if _, exists := rule[key]; !exists {
			t.Fatalf("expected supported key %q to remain in %#v", key, rule)
		}
	}

	for _, key := range []string{"protocol", "package_name", "clash_mode"} {
		if _, exists := rule[key]; exists {
			t.Fatalf("unexpected unsupported key %q in %#v", key, rule)
		}
	}
}

func TestPruneRedundantMihomoListenerRules_RemovesRuleWhenSubRulesMatchGlobalRules(t *testing.T) {
	listener := map[string]interface{}{
		"name": "trusttunnel-in",
		"rule": "trusttunnel-in",
	}

	routeResult := &mihomoRouteRenderResult{
		Rules: []string{"MATCH,DIRECT"},
		SubRules: map[string][]string{
			"trusttunnel-in": []string{"MATCH,DIRECT"},
		},
	}

	got := pruneRedundantMihomoListenerRules([]interface{}{listener}, routeResult)
	if _, exists := listener["rule"]; exists {
		t.Fatalf("listener rule should be removed when sub-rules match global rules: %#v", listener)
	}
	if got != nil {
		t.Fatalf("pruned sub-rules = %#v, want nil", got)
	}
}

func TestPruneRedundantMihomoListenerRules_KeepsRuleWhenSubRulesDiffer(t *testing.T) {
	listener := map[string]interface{}{
		"name": "trusttunnel-in",
		"rule": "trusttunnel-in",
	}

	routeResult := &mihomoRouteRenderResult{
		Rules: []string{"MATCH,DIRECT"},
		SubRules: map[string][]string{
			"trusttunnel-in": []string{"MATCH,proxy-a"},
		},
	}

	got := pruneRedundantMihomoListenerRules([]interface{}{listener}, routeResult)
	if got == nil || len(got["trusttunnel-in"]) != 1 || got["trusttunnel-in"][0] != "MATCH,proxy-a" {
		t.Fatalf("pruned sub-rules = %#v, want trusttunnel-in => MATCH,proxy-a", got)
	}
	if rule, _ := listener["rule"].(string); rule != "trusttunnel-in" {
		t.Fatalf("listener rule = %#v, want %q", listener["rule"], "trusttunnel-in")
	}
}
