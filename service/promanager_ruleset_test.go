package service

import (
	"encoding/json"
	"testing"
)

func TestNormalizeRouteRuleSetPlacementMigratesLegacyDownloadDetour(t *testing.T) {
	config := &ProManagerSingBoxConfig{
		Route: json.RawMessage(`{
			"rule_set": [
				{"type":"remote","tag":"legacy","format":"binary","url":"https://example.com/legacy.srs","download_detour":"direct"},
				{"type":"remote","tag":"current","format":"binary","url":"https://example.com/current.srs","http_client":{"detour":"proxy"}},
				{"type":"local","tag":"local","format":"binary","path":"./local.srs","download_detour":"keep"}
			]
		}`),
	}

	if err := normalizeRouteRuleSetPlacement(config); err != nil {
		t.Fatalf("normalizeRouteRuleSetPlacement() error = %v", err)
	}

	routeMap := decodeRouteMapForTest(t, config.Route)
	items, ok := routeMap["rule_set"].([]interface{})
	if !ok || len(items) != 3 {
		t.Fatalf("route.rule_set = %#v, want three items", routeMap["rule_set"])
	}
	legacy := items[0].(map[string]interface{})
	client, ok := legacy["http_client"].(map[string]interface{})
	if !ok || client["detour"] != "direct" {
		t.Fatalf("legacy http_client = %#v, want detour direct", legacy["http_client"])
	}
	if _, exists := legacy["download_detour"]; exists {
		t.Fatal("legacy download_detour should be removed")
	}
	current := items[1].(map[string]interface{})
	if current["http_client"].(map[string]interface{})["detour"] != "proxy" {
		t.Fatalf("existing http_client.detour was overwritten: %#v", current["http_client"])
	}
	local := items[2].(map[string]interface{})
	if _, exists := local["download_detour"]; !exists {
		t.Fatal("non-remote rule set should not be rewritten by remote migration")
	}
}

func decodeRouteMapForTest(t *testing.T, raw json.RawMessage) map[string]interface{} {
	t.Helper()
	route := map[string]interface{}{}
	if err := json.Unmarshal(raw, &route); err != nil {
		t.Fatalf("decode route: %v", err)
	}
	return route
}
