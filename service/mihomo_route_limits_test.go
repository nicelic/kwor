package service

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/alireza0/s-ui/database/model"
)

func TestValidateMihomoConfigRouteBoundsRejectsRuleCombinations(t *testing.T) {
	db := initMihomoManagerFinalFallbackTestDB(t, "mihomo-route-combination-limit.db")
	domains := make([]string, 0, 23)
	ports := make([]int, 0, 23)
	for index := 0; index < 23; index++ {
		domains = append(domains, fmt.Sprintf("node-%d.example.test", index))
		ports = append(ports, 10000+index)
	}

	config, err := json.Marshal(map[string]interface{}{
		"route": map[string]interface{}{
			"final":    "DIRECT",
			"rule_set": []interface{}{},
			"rules": []interface{}{map[string]interface{}{
				"action":   "route",
				"outbound": "DIRECT",
				"domain":   domains,
				"port":     ports,
			}},
		},
	})
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}

	err = validateMihomoConfigRouteBounds(config, db)
	if err == nil || !strings.Contains(err.Error(), "combination safety limit") {
		t.Fatalf("expected combination limit error, got %v", err)
	}
}

func TestValidateMihomoConfigRouteBoundsRejectsUnsupportedTarget(t *testing.T) {
	db := initMihomoManagerFinalFallbackTestDB(t, "mihomo-route-target-limit.db")
	config := json.RawMessage(`{
		"route": {
			"final": "DIRECT",
			"rule_set": [],
			"rules": [{"action":"route","outbound":"tor-only"}]
		}
	}`)

	err := validateMihomoConfigRouteBounds(config, db)
	if err == nil || !strings.Contains(err.Error(), "unsupported Mihomo target") {
		t.Fatalf("expected unsupported target error, got %v", err)
	}
}

func TestValidateMihomoConfigRouteBoundsCountsTerminalMatchRule(t *testing.T) {
	db := initMihomoManagerFinalFallbackTestDB(t, "mihomo-route-terminal-match-limit.db")
	if err := db.Create(&model.MihomoInbound{Type: "http", Tag: "route-limit-inbound"}).Error; err != nil {
		t.Fatalf("create mihomo inbound: %v", err)
	}

	domains := make([]string, 0, 8)
	ports := make([]int, 0, 64)
	for index := 0; index < 8; index++ {
		domains = append(domains, fmt.Sprintf("terminal-%d.example.test", index))
	}
	for index := 0; index < 64; index++ {
		ports = append(ports, 10000+index)
	}
	rules := make([]interface{}, 0, 8)
	for index := 0; index < 8; index++ {
		rules = append(rules, map[string]interface{}{
			"action":   "route",
			"outbound": "DIRECT",
			"domain":   domains,
			"port":     ports,
		})
	}

	config, err := json.Marshal(map[string]interface{}{
		"route": map[string]interface{}{
			"final":    "DIRECT",
			"rule_set": []interface{}{},
			"rules":    rules,
		},
	})
	if err != nil {
		t.Fatalf("marshal config: %v", err)
	}

	err = validateMihomoConfigRouteBounds(config, db)
	if err == nil || !strings.Contains(err.Error(), "generated-rule safety limit") {
		t.Fatalf("expected terminal MATCH limit error, got %v", err)
	}
}

func TestBuildMihomoConfigChangeAuditDoesNotStoreFullConfig(t *testing.T) {
	config := json.RawMessage(`{
		"sniffer": {"enable": true},
		"route": {
			"final": "DIRECT",
			"rule_set": [{"tag":"provider-a"}],
			"rules": [{"action":"route","outbound":"DIRECT","domain":["secret.example.test"]}]
		}
	}`)

	audit := buildMihomoConfigChangeAudit(config)
	if strings.Contains(string(audit), "secret.example.test") {
		t.Fatalf("audit must not contain the full route config: %s", audit)
	}
	if !strings.Contains(string(audit), `"route_rules":1`) || !strings.Contains(string(audit), `"sniff":true`) {
		t.Fatalf("audit summary missing expected values: %s", audit)
	}
}

func TestValidateMihomoRouteRuleBoundsRejectsMalformedNumericMatchers(t *testing.T) {
	baseRule := func() map[string]interface{} {
		return map[string]interface{}{
			"action":   "route",
			"outbound": "DIRECT",
		}
	}

	tests := []struct {
		name  string
		field string
		value interface{}
	}{
		{name: "fractional port", field: "port", value: []interface{}{80.5}},
		{name: "string port", field: "port", value: []interface{}{"80"}},
		{name: "truncated port text", field: "port", value: []interface{}{"80x"}},
		{name: "zero port", field: "port", value: []interface{}{0}},
		{name: "large port", field: "port", value: []interface{}{65536}},
		{name: "fractional source port", field: "source_port", value: []interface{}{1.5}},
		{name: "malformed port range", field: "port_range", value: []interface{}{"80:90"}},
		{name: "reversed port range", field: "port_range", value: []interface{}{"90-80"}},
		{name: "out of bounds port range", field: "source_port_range", value: []interface{}{"0-80"}},
		{name: "string uid", field: "user_id", value: []interface{}{"1000"}},
		{name: "fractional uid", field: "user_id", value: []interface{}{1000.5}},
		{name: "negative uid", field: "user_id", value: []interface{}{-1}},
		{name: "unsafe uid", field: "user_id", value: []interface{}{float64(maxMihomoUID + 1)}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rule := baseRule()
			rule[tt.field] = tt.value
			if err := validateMihomoRouteRuleBounds(rule, map[string]struct{}{}, nil); err == nil || !strings.Contains(err.Error(), tt.field) {
				t.Fatalf("expected %s validation error, got %v", tt.field, err)
			}
		})
	}
}

func TestValidateMihomoRouteRuleBoundsAcceptsCanonicalNumericMatchers(t *testing.T) {
	rule := map[string]interface{}{
		"action":            "route",
		"outbound":          "DIRECT",
		"port":              []interface{}{float64(80), float64(443)},
		"source_port":       []interface{}{uint16(53)},
		"port_range":        []interface{}{"1000-2000"},
		"source_port_range": []interface{}{"3000-3010"},
		"user_id":           []interface{}{float64(0), int64(1000)},
	}

	if err := validateMihomoRouteRuleBounds(rule, map[string]struct{}{}, nil); err != nil {
		t.Fatalf("canonical numeric matchers were rejected: %v", err)
	}
}

func TestMihomoRouteRendererDoesNotTruncateFractionalPorts(t *testing.T) {
	rule := map[string]interface{}{
		"action":   "route",
		"outbound": "DIRECT",
		"port":     []float64{80.5},
	}
	if _, ok := buildMihomoRuleStrings(rule, nil, nil, nil, false); ok {
		t.Fatal("fractional port must not be rendered as a truncated integer")
	}

	rule["port"] = []float64{80}
	rules, ok := buildMihomoRuleStrings(rule, nil, nil, nil, false)
	if !ok || len(rules) != 1 || rules[0] != "DST-PORT,80,DIRECT" {
		t.Fatalf("canonical port rendered incorrectly: ok=%v rules=%#v", ok, rules)
	}
}
