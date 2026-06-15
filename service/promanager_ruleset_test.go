package service

import (
	"encoding/json"
	"testing"
)

func decodeRouteMapForTest(t *testing.T, route json.RawMessage) map[string]interface{} {
	t.Helper()
	var routeMap map[string]interface{}
	if err := json.Unmarshal(route, &routeMap); err != nil {
		t.Fatalf("failed to unmarshal route: %v", err)
	}
	return routeMap
}

func TestNormalizeRouteRuleSetPlacement_KeepRouteRuleSet(t *testing.T) {
	config := &ProManagerSingBoxConfig{
		Route: json.RawMessage(`{
			"rules": [{"action":"sniff"}],
			"rule_set": [{"tag":"openai","type":"remote","format":"binary","url":"https://example.com/openai.srs"}]
		}`),
	}

	if err := normalizeRouteRuleSetPlacement(config); err != nil {
		t.Fatalf("normalizeRouteRuleSetPlacement() error = %v", err)
	}

	if len(config.RuleSets) != 0 {
		t.Fatalf("top-level rule_set should be empty, got %d", len(config.RuleSets))
	}

	routeMap := decodeRouteMapForTest(t, config.Route)
	ruleSet, ok := routeMap["rule_set"].([]interface{})
	if !ok || len(ruleSet) != 1 {
		t.Fatalf("route.rule_set = %#v, want one item", routeMap["rule_set"])
	}
}

func TestNormalizeRouteRuleSetPlacement_MigrateLegacyTopLevelRuleSet(t *testing.T) {
	config := &ProManagerSingBoxConfig{
		Route: json.RawMessage(`{
			"rules": [{"action":"sniff"}]
		}`),
		RuleSets: []json.RawMessage{
			json.RawMessage(`{"tag":"anthropic","type":"remote","format":"binary","url":"https://example.com/anthropic.srs"}`),
		},
	}

	if err := normalizeRouteRuleSetPlacement(config); err != nil {
		t.Fatalf("normalizeRouteRuleSetPlacement() error = %v", err)
	}

	if len(config.RuleSets) != 0 {
		t.Fatalf("top-level rule_set should be empty after migration, got %d", len(config.RuleSets))
	}

	routeMap := decodeRouteMapForTest(t, config.Route)
	ruleSet, ok := routeMap["rule_set"].([]interface{})
	if !ok || len(ruleSet) != 1 {
		t.Fatalf("route.rule_set = %#v, want one migrated item", routeMap["rule_set"])
	}
}

func TestNormalizeRouteRuleSetPlacement_RouteWinsOverLegacyTopLevel(t *testing.T) {
	config := &ProManagerSingBoxConfig{
		Route: json.RawMessage(`{
			"rules": [{"action":"sniff"}],
			"rule_set": [{"tag":"google","type":"remote","format":"binary","url":"https://example.com/google.srs"}]
		}`),
		RuleSets: []json.RawMessage{
			json.RawMessage(`{"tag":"legacy","type":"remote","format":"binary","url":"https://example.com/legacy.srs"}`),
		},
	}

	if err := normalizeRouteRuleSetPlacement(config); err != nil {
		t.Fatalf("normalizeRouteRuleSetPlacement() error = %v", err)
	}

	if len(config.RuleSets) != 0 {
		t.Fatalf("top-level rule_set should always be cleared, got %d", len(config.RuleSets))
	}

	routeMap := decodeRouteMapForTest(t, config.Route)
	ruleSet, ok := routeMap["rule_set"].([]interface{})
	if !ok || len(ruleSet) != 1 {
		t.Fatalf("route.rule_set = %#v, want one route-defined item", routeMap["rule_set"])
	}

	item, ok := ruleSet[0].(map[string]interface{})
	if !ok {
		t.Fatalf("rule_set[0] type = %T, want map[string]interface{}", ruleSet[0])
	}
	if item["tag"] != "google" {
		t.Fatalf("route.rule_set[0].tag = %v, want google", item["tag"])
	}
}

func TestEnsureCoreLogLevel_DefaultToPanicWhenMissing(t *testing.T) {
	config := &ProManagerSingBoxConfig{
		Log: json.RawMessage(`{}`),
	}

	if err := ensureCoreLogLevel(config); err != nil {
		t.Fatalf("ensureCoreLogLevel() error = %v", err)
	}

	var logMap map[string]interface{}
	if err := json.Unmarshal(config.Log, &logMap); err != nil {
		t.Fatalf("failed to unmarshal log: %v", err)
	}
	if logMap["level"] != "panic" {
		t.Fatalf("log.level = %v, want panic", logMap["level"])
	}
}

func TestEnsureCoreLogLevel_KeepExistingLevel(t *testing.T) {
	config := &ProManagerSingBoxConfig{
		Log: json.RawMessage(`{"level":"info","timestamp":true}`),
	}

	if err := ensureCoreLogLevel(config); err != nil {
		t.Fatalf("ensureCoreLogLevel() error = %v", err)
	}

	var logMap map[string]interface{}
	if err := json.Unmarshal(config.Log, &logMap); err != nil {
		t.Fatalf("failed to unmarshal log: %v", err)
	}
	if logMap["level"] != "info" {
		t.Fatalf("log.level = %v, want info", logMap["level"])
	}
}
