package service

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
)

func stubSingboxRouteRuntimeRegeneration(t *testing.T) {
	t.Helper()
	original := regenerateSingboxRouteRuntimeConfig
	regenerateSingboxRouteRuntimeConfig = func(*ConfigService) error { return nil }
	t.Cleanup(func() {
		regenerateSingboxRouteRuntimeConfig = original
	})
}

func TestSaveSingboxRouteOnlyChangesRouteAndKeepsMihomoIndependent(t *testing.T) {
	stubSingboxRouteRuntimeRegeneration(t)
	settingService := initPanelSQLiteSettingTestDB(t)
	initialConfig := `{
		"log":{"level":"info"},
		"dns":{"strategy":"prefer_ipv4","rules":[]},
		"route":{"rules":[{"action":"sniff"}],"rule_set":[]}
	}`
	if err := settingService.SaveSetting("config", initialConfig); err != nil {
		t.Fatalf("seed sing-box config: %v", err)
	}
	initialMihomo := `{"route":{"rules":[{"type":"match","payload":"keep"}]}}`
	if err := settingService.SaveSetting(mihomoConfigSettingKey, initialMihomo); err != nil {
		t.Fatalf("seed Mihomo config: %v", err)
	}
	if err := database.GetDB().Create(&model.Outbound{Type: "socks", Tag: "route-node", Options: json.RawMessage(`{"server":"127.0.0.1","server_port":1080}`)}).Error; err != nil {
		t.Fatalf("create route outbound: %v", err)
	}

	configService := &ConfigService{}
	before, err := configService.GetSingboxRouteEditorContext()
	if err != nil {
		t.Fatalf("load route context: %v", err)
	}
	result, err := configService.SaveSingboxRoute(SingboxRouteSaveRequest{
		ExpectedRevision: before.Revision,
		Route:            json.RawMessage(`{"final":"route-node","rules":[{"action":"route","outbound":"route-node"}],"rule_set":[]}`),
	}, "test")
	if err != nil {
		t.Fatalf("save route: %v", err)
	}
	if !result.Changed || result.Revision != before.Revision+1 {
		t.Fatalf("unexpected route save result: %#v", result)
	}

	var stored model.Setting
	if err := database.GetDB().Where("key = ?", "config").First(&stored).Error; err != nil {
		t.Fatalf("load sing-box config: %v", err)
	}
	var config map[string]any
	if err := json.Unmarshal([]byte(stored.Value), &config); err != nil {
		t.Fatalf("decode sing-box config: %v", err)
	}
	if _, exists := config["log"]; exists {
		t.Fatalf("route save retained legacy log: %#v", config["log"])
	}
	if dns, _ := config["dns"].(map[string]any); dns["strategy"] != "prefer_ipv4" {
		t.Fatalf("route save changed DNS: %#v", dns)
	}
	route, _ := config["route"].(map[string]any)
	if route["final"] != "route-node" {
		t.Fatalf("route section was not updated: %#v", route)
	}

	var mihomo model.Setting
	if err := database.GetDB().Where("key = ?", mihomoConfigSettingKey).First(&mihomo).Error; err != nil {
		t.Fatalf("load Mihomo config: %v", err)
	}
	if mihomo.Value != initialMihomo {
		t.Fatalf("focused sing-box route save touched Mihomo config: %s", mihomo.Value)
	}
}

func TestSaveSingboxRouteRejectsStaleRevisionAndDoesNotPersistRejectedData(t *testing.T) {
	stubSingboxRouteRuntimeRegeneration(t)
	settingService := initPanelSQLiteSettingTestDB(t)
	if err := settingService.SaveSetting("config", `{"route":{"rules":[],"rule_set":[]}}`); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	configService := &ConfigService{}
	before, err := configService.GetSingboxRouteEditorContext()
	if err != nil {
		t.Fatalf("load route context: %v", err)
	}
	first, err := configService.SaveSingboxRoute(SingboxRouteSaveRequest{
		ExpectedRevision: before.Revision,
		Route:            json.RawMessage(`{"rules":[{"action":"sniff"}],"rule_set":[]}`),
	}, "test")
	if err != nil || !first.Changed {
		t.Fatalf("first route mutation failed: result=%#v err=%v", first, err)
	}
	_, err = configService.SaveSingboxRoute(SingboxRouteSaveRequest{
		ExpectedRevision: before.Revision,
		Route:            json.RawMessage(`{"rules":[{"action":"reject"}],"rule_set":[]}`),
	}, "test")
	var conflict *SingboxRouteRevisionConflictError
	if !errors.As(err, &conflict) || conflict.CurrentRevision != first.Revision {
		t.Fatalf("expected stale route conflict at revision %d, got %v", first.Revision, err)
	}

	tooMany := make([]any, SingboxRouteMaxRules+1)
	for index := range tooMany {
		tooMany[index] = map[string]any{"action": "sniff"}
	}
	overbounds, marshalErr := json.Marshal(map[string]any{"rules": tooMany, "rule_set": []any{}})
	if marshalErr != nil {
		t.Fatalf("marshal oversized route: %v", marshalErr)
	}
	_, err = configService.SaveSingboxRoute(SingboxRouteSaveRequest{
		ExpectedRevision: first.Revision,
		Route:            overbounds,
	}, "test")
	if err == nil {
		t.Fatalf("oversized route was accepted: %v", err)
	}
	after, err := configService.GetSingboxRouteEditorContext()
	if err != nil {
		t.Fatalf("reload route context: %v", err)
	}
	if after.Revision != first.Revision {
		t.Fatalf("rejected route advanced revision: before=%d after=%d", first.Revision, after.Revision)
	}
}

func TestSingboxRouteContextUsesDefaultRuntimeTargets(t *testing.T) {
	settingService := initPanelSQLiteSettingTestDB(t)
	if err := settingService.SaveSetting("config", `{"route":{"rules":[],"rule_set":[]}}`); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	context, err := (&ConfigService{}).GetSingboxRouteEditorContext()
	if err != nil {
		t.Fatalf("load route context: %v", err)
	}
	if !containsString(context.OutboundTags, "direct") {
		t.Fatalf("default direct outbound is missing: %#v", context.OutboundTags)
	}
	if containsString(context.OutboundTags, nodeSelectorTag) || containsString(context.OutboundTags, finalSelectorTag) {
		t.Fatalf("subscription-only selector leaked into default Core targets: %#v", context.OutboundTags)
	}

	_, err = (&ConfigService{}).SaveSingboxRoute(SingboxRouteSaveRequest{
		ExpectedRevision: context.Revision,
		Route:            json.RawMessage(`{"final":"节点选择","rules":[],"rule_set":[]}`),
	}, "test")
	if err == nil {
		t.Fatal("subscription-only selector was accepted as a default Core route target")
	}
}

func TestSaveSingboxRouteNormalizesInboundAliases(t *testing.T) {
	stubSingboxRouteRuntimeRegeneration(t)
	settingService := initPanelSQLiteSettingTestDB(t)
	if err := settingService.SaveSetting("config", `{"route":{"rules":[],"rule_set":[]}}`); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	if err := database.GetDB().Create(&model.Inbound{
		Type:    "shadowtls",
		Tag:     "shadow-listener",
		Options: json.RawMessage(`{"ss_config":{"method":"aes-128-gcm"}}`),
	}).Error; err != nil {
		t.Fatalf("create shadowtls inbound: %v", err)
	}

	configService := &ConfigService{}
	context, err := configService.GetSingboxRouteEditorContext()
	if err != nil {
		t.Fatalf("load route context: %v", err)
	}
	result, err := configService.SaveSingboxRoute(SingboxRouteSaveRequest{
		ExpectedRevision: context.Revision,
		Route: json.RawMessage(`{
			"rules":[{"inbound":["shadow-listener"],"action":"sniff"}],
			"rule_set":[]
		}`),
	}, "test")
	if err != nil {
		t.Fatalf("save route: %v", err)
	}
	var route map[string]any
	if err := json.Unmarshal(result.Route, &route); err != nil {
		t.Fatalf("decode saved route: %v", err)
	}
	rules, ok := route["rules"].([]any)
	if !ok || len(rules) != 1 {
		t.Fatalf("unexpected saved rules: %#v", route["rules"])
	}
	rule, ok := rules[0].(map[string]any)
	if !ok {
		t.Fatalf("unexpected saved rule: %#v", rules[0])
	}
	inbounds, ok := rule["inbound"].([]any)
	if !ok || len(inbounds) != 1 || inbounds[0] != "shadow-listener-in" {
		t.Fatalf("inbound alias was not normalized: %#v", rule["inbound"])
	}
}

func TestSaveSingboxRouteRejectsInvalidRuleSetShape(t *testing.T) {
	stubSingboxRouteRuntimeRegeneration(t)
	settingService := initPanelSQLiteSettingTestDB(t)
	if err := settingService.SaveSetting("config", `{"route":{"rules":[],"rule_set":[]}}`); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	configService := &ConfigService{}
	context, err := configService.GetSingboxRouteEditorContext()
	if err != nil {
		t.Fatalf("load route context: %v", err)
	}
	_, err = configService.SaveSingboxRoute(SingboxRouteSaveRequest{
		ExpectedRevision: context.Revision,
		Route: json.RawMessage(`{
			"rules":[],
			"rule_set":[{"type":"local","tag":"missing-path","format":"binary"}]
		}`),
	}, "test")
	if err == nil || !strings.Contains(err.Error(), "requires a path") {
		t.Fatalf("invalid local rule set was accepted: %v", err)
	}
	after, err := configService.GetSingboxRouteEditorContext()
	if err != nil {
		t.Fatalf("reload route context: %v", err)
	}
	if after.Revision != context.Revision {
		t.Fatalf("rejected rule set advanced revision: before=%d after=%d", context.Revision, after.Revision)
	}
}

func TestValidateSingboxRouteRuleSetAcceptsFullDurationValues(t *testing.T) {
	targets := map[string]struct{}{"direct": {}}
	for _, interval := range []string{"24h", "1h30m", "1d", "1w", "1d12h"} {
		t.Run(interval, func(t *testing.T) {
			err := validateSingboxRouteRuleSet(map[string]any{
				"type":            "remote",
				"tag":             "duration-" + interval,
				"url":             "https://example.com/rules.srs",
				"format":          "binary",
				"download_detour": "direct",
				"update_interval": interval,
			}, 0, targets)
			if err != nil {
				t.Fatalf("valid update_interval %q was rejected: %v", interval, err)
			}
		})
	}

	for _, interval := range []string{"24", "24hx", "1h30", "1.2.3h"} {
		t.Run("invalid-"+interval, func(t *testing.T) {
			err := validateSingboxRouteRuleSet(map[string]any{
				"type":            "remote",
				"tag":             "invalid-duration",
				"url":             "https://example.com/rules.srs",
				"format":          "binary",
				"download_detour": "direct",
				"update_interval": interval,
			}, 0, targets)
			if err == nil || !strings.Contains(err.Error(), "update_interval") {
				t.Fatalf("invalid update_interval %q error = %v", interval, err)
			}
		})
	}
}

func TestValidateSingboxRouteRuleSetAcceptsHTTPClientDetour(t *testing.T) {
	err := validateSingboxRouteRuleSet(map[string]any{
		"type":   "remote",
		"tag":    "current-schema",
		"url":    "https://example.com/rules.srs",
		"format": "binary",
		"http_client": map[string]any{
			"detour": "direct",
		},
	}, 0, map[string]struct{}{"direct": {}})
	if err != nil {
		t.Fatalf("http_client.detour ruleset was rejected: %v", err)
	}

	err = validateSingboxRouteRuleSet(map[string]any{
		"type":        "remote",
		"tag":         "unknown-schema-detour",
		"url":         "https://example.com/rules.srs",
		"http_client": map[string]any{"detour": "missing"},
	}, 0, map[string]struct{}{"direct": {}})
	if err == nil || !strings.Contains(err.Error(), "http_client.detour") {
		t.Fatalf("unknown http_client.detour was accepted: %v", err)
	}
}

func TestValidateSingboxRouteRuleSetInlineRules(t *testing.T) {
	targets := map[string]struct{}{"direct": {}}
	valid := map[string]any{
		"type": "inline",
		"tag":  "inline-rules",
		"rules": []any{
			map[string]any{"domain_suffix": []any{"example.com"}},
		},
	}
	if err := validateSingboxRouteRuleSet(valid, 0, targets); err != nil {
		t.Fatalf("valid inline ruleset was rejected: %v", err)
	}

	for _, invalid := range []map[string]any{
		{"type": "inline", "tag": "missing-rules"},
		{"type": "inline", "tag": "empty-rules", "rules": []any{}},
		{"type": "inline", "tag": "invalid-rule", "rules": []any{"not-an-object"}},
	} {
		if err := validateSingboxRouteRuleSet(invalid, 0, targets); err == nil {
			t.Fatalf("invalid inline ruleset was accepted: %#v", invalid)
		}
	}
}

func TestValidateSingboxRouteRuleSetInlineRejectsRemoteOnlyFields(t *testing.T) {
	targets := map[string]struct{}{"direct": {}}
	for _, field := range []string{"initial_path", "http_client"} {
		t.Run(field, func(t *testing.T) {
			value := map[string]any{
				"type":  "inline",
				"tag":   "inline-with-legacy-field",
				"rules": []any{map[string]any{"domain_suffix": []any{"example.com"}}},
			}
			if field == "initial_path" {
				value[field] = "cache/rules.srs"
			} else {
				value[field] = map[string]any{"headers": map[string]any{"X-Test": "1"}}
			}
			if err := validateSingboxRouteRuleSet(value, 0, targets); err == nil {
				t.Fatalf("inline ruleset with %s was accepted", field)
			}
		})
	}
}

func TestSingboxRouteAuditDoesNotStoreFullConfig(t *testing.T) {
	audit := buildSingboxConfigChangeAudit(json.RawMessage(`{
		"dns":{"rules":[{"domain":["secret-dns.example.test"]}]},
		"route":{"final":"direct","rules":[{"action":"route","outbound":"direct","domain":["secret-route.example.test"]}],"rule_set":[]}
	}`))
	if strings.Contains(string(audit), "secret-dns.example.test") || strings.Contains(string(audit), "secret-route.example.test") {
		t.Fatalf("audit must not contain full config data: %s", audit)
	}
	if !strings.Contains(string(audit), `"route_ruleCount":1`) || !strings.Contains(string(audit), `"route_final":"direct"`) {
		t.Fatalf("audit lacks route summary: %s", audit)
	}
}

func TestValidateSingboxRouteNumericFieldsRejectsLossyValues(t *testing.T) {
	cases := []map[string]any{
		{"ip_version": 5},
		{"port": []any{"80x"}},
		{"source_port": []any{80.5}},
		{"user_id": []any{float64(1 << 53)}},
		{"port_range": []any{"80:90x"}},
		{"source_port_range": []any{"9000:8000"}},
		{"override_port": 443.5},
	}
	for index, rule := range cases {
		if err := validateSingboxRouteNumericFields(map[string]any{"rules": []any{rule}}); err == nil {
			t.Fatalf("invalid route numeric case #%d was accepted: %#v", index+1, rule)
		}
	}
}

func TestValidateSingboxRouteNumericFieldsAcceptsValidValues(t *testing.T) {
	route := map[string]any{
		"default_mark": float64(123),
		"rules": []any{map[string]any{
			"ip_version":        float64(4),
			"port":              []any{float64(80), float64(443)},
			"source_port":       float64(5353),
			"user_id":           []any{float64(1000)},
			"port_range":        []any{"80:90"},
			"source_port_range": []any{"1000-2000"},
			"override_port":     float64(8443),
		}},
	}
	if err := validateSingboxRouteNumericFields(route); err != nil {
		t.Fatalf("valid route numeric fields were rejected: %v", err)
	}
}

func TestSaveSingboxRouteRetryIgnoresStaleRevision(t *testing.T) {
	settingService := initPanelSQLiteSettingTestDB(t)
	if err := settingService.SaveSetting("config", `{"route":{"rules":[],"rule_set":[]}}`); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	calls := 0
	original := regenerateSingboxRouteRuntimeConfig
	regenerateSingboxRouteRuntimeConfig = func(*ConfigService) error { calls++; return nil }
	t.Cleanup(func() { regenerateSingboxRouteRuntimeConfig = original })

	configService := &ConfigService{}
	before, err := configService.GetSingboxRouteEditorContext()
	if err != nil {
		t.Fatalf("load route context: %v", err)
	}
	first, err := configService.SaveSingboxRoute(SingboxRouteSaveRequest{
		ExpectedRevision: before.Revision,
		Route:            json.RawMessage(`{"final":"direct","rules":[],"rule_set":[]}`),
	}, "test")
	if err != nil || first == nil || !first.Changed {
		t.Fatalf("advance route revision: result=%#v err=%v", first, err)
	}
	result, err := configService.SaveSingboxRoute(SingboxRouteSaveRequest{
		ExpectedRevision: before.Revision,
		RetryRuntime:     true,
	}, "test")
	if err != nil || result == nil || result.Changed || result.Revision != first.Revision {
		t.Fatalf("stale runtime retry result=%#v err=%v", result, err)
	}
	if calls != 2 {
		t.Fatalf("runtime regeneration calls=%d, want 2", calls)
	}
}
