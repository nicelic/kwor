package service

import (
	"encoding/json"
	"reflect"
	"testing"
)

func inboundFieldToStringSlice(t *testing.T, value interface{}) []string {
	t.Helper()

	switch inbound := value.(type) {
	case []string:
		return inbound
	case []interface{}:
		result := make([]string, 0, len(inbound))
		for _, entry := range inbound {
			tag, ok := entry.(string)
			if !ok {
				t.Fatalf("expected inbound entry string, got %T", entry)
			}
			result = append(result, tag)
		}
		return result
	default:
		t.Fatalf("expected inbound slice, got %T", value)
		return nil
	}
}

func TestDeriveEffectiveInboundRouteTag(t *testing.T) {
	tests := []struct {
		name       string
		tag        string
		inboundTyp string
		options    map[string]interface{}
		want       string
	}{
		{
			name:       "shadowtls with ss_config maps to internal shadowsocks inbound",
			tag:        "stls_hk1",
			inboundTyp: "shadowtls",
			options: map[string]interface{}{
				"ss_config": map[string]interface{}{"method": "2022-blake3-aes-256-gcm"},
			},
			want: "stls_hk1-in",
		},
		{
			name:       "detour has higher priority than shadowtls internal mapping",
			tag:        "stls_hk1",
			inboundTyp: "shadowtls",
			options: map[string]interface{}{
				"detour":    "custom-detour-in",
				"ss_config": map[string]interface{}{"method": "2022-blake3-aes-256-gcm"},
			},
			want: "custom-detour-in",
		},
		{
			name:       "generic detour inbound maps to detour target",
			tag:        "vless_hk1",
			inboundTyp: "vless",
			options: map[string]interface{}{
				"detour": "vless_hk1_inner",
			},
			want: "vless_hk1_inner",
		},
		{
			name:       "no detour and no shadowtls split keeps original tag",
			tag:        "hy1_hk1",
			inboundTyp: "hysteria",
			options:    map[string]interface{}{},
			want:       "hy1_hk1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := deriveEffectiveInboundRouteTag(tt.tag, tt.inboundTyp, tt.options)
			if got != tt.want {
				t.Fatalf("deriveEffectiveInboundRouteTag() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestNormalizeConfigInboundRuleTags(t *testing.T) {
	rawConfigMap := map[string]interface{}{
		"route": map[string]interface{}{
			"rules": []interface{}{
				map[string]interface{}{
					"action":   "route",
					"inbound":  []interface{}{"hy1_hk1", "stls_hk1", "stls_hk1"},
					"outbound": "hy1_sg",
				},
				map[string]interface{}{
					"type": "logical",
					"mode": "and",
					"rules": []interface{}{
						map[string]interface{}{
							"action":   "route",
							"inbound":  []interface{}{"outer", "plain"},
							"outbound": "direct",
						},
					},
				},
			},
		},
		"dns": map[string]interface{}{
			"rules": []interface{}{
				map[string]interface{}{
					"action":  "route",
					"inbound": []interface{}{"stls_hk1"},
					"server":  "dns-hiB",
				},
			},
		},
	}

	rawConfig, err := json.Marshal(rawConfigMap)
	if err != nil {
		t.Fatalf("marshal raw config failed: %v", err)
	}

	aliasMap := map[string]string{
		"stls_hk1": "stls_hk1-in",
		"outer":    "middle",
		"middle":   "inner",
	}

	normalized, changed, err := normalizeConfigInboundRuleTags(rawConfig, aliasMap)
	if err != nil {
		t.Fatalf("normalizeConfigInboundRuleTags failed: %v", err)
	}
	if !changed {
		t.Fatalf("expected changed=true")
	}

	var normalizedConfig map[string]interface{}
	if err := json.Unmarshal(normalized, &normalizedConfig); err != nil {
		t.Fatalf("unmarshal normalized config failed: %v", err)
	}

	routeRules := normalizedConfig["route"].(map[string]interface{})["rules"].([]interface{})
	firstInbound := inboundFieldToStringSlice(t, routeRules[0].(map[string]interface{})["inbound"])
	if !reflect.DeepEqual(firstInbound, []string{"hy1_hk1", "stls_hk1-in"}) {
		t.Fatalf("route first inbound = %#v, want %#v", firstInbound, []string{"hy1_hk1", "stls_hk1-in"})
	}

	nestedRules := routeRules[1].(map[string]interface{})["rules"].([]interface{})
	nestedInbound := inboundFieldToStringSlice(t, nestedRules[0].(map[string]interface{})["inbound"])
	if !reflect.DeepEqual(nestedInbound, []string{"inner", "plain"}) {
		t.Fatalf("nested inbound = %#v, want %#v", nestedInbound, []string{"inner", "plain"})
	}

	dnsRules := normalizedConfig["dns"].(map[string]interface{})["rules"].([]interface{})
	dnsInbound := inboundFieldToStringSlice(t, dnsRules[0].(map[string]interface{})["inbound"])
	if !reflect.DeepEqual(dnsInbound, []string{"stls_hk1-in"}) {
		t.Fatalf("dns inbound = %#v, want %#v", dnsInbound, []string{"stls_hk1-in"})
	}
}

func TestResolveInboundTagAlias_CycleSafeAndTransitive(t *testing.T) {
	if got := resolveInboundTagAlias("a", map[string]string{"a": "b", "b": "a"}); got != "a" {
		t.Fatalf("cycle alias should resolve to original tag, got %q", got)
	}

	if got := resolveInboundTagAlias("a", map[string]string{"a": "b", "b": "c"}); got != "c" {
		t.Fatalf("transitive alias should resolve to final tag, got %q", got)
	}
}
