package service

import (
	"encoding/json"
	"strings"

	"github.com/alireza0/s-ui/database/model"

	"gorm.io/gorm"
)

// deriveEffectiveInboundRouteTag returns the actual inbound tag used by routing.
// For shadowtls with ss_config, data is handled by an internal shadowsocks inbound (<tag>-in).
// For generic detour-enabled inbounds/endpoints, traffic is handled by the detour target.
func deriveEffectiveInboundRouteTag(tag string, inboundType string, options map[string]interface{}) string {
	if detour := extractDetourFromOptions(options); detour != "" {
		return detour
	}
	if inboundType == "shadowtls" && hasShadowTLSSSConfig(options) {
		return tag + "-in"
	}
	return tag
}

func deriveEffectiveInboundRouteTagFromRaw(tag string, inboundType string, rawOptions json.RawMessage) string {
	if len(rawOptions) == 0 {
		return tag
	}

	var options map[string]interface{}
	if err := json.Unmarshal(rawOptions, &options); err != nil {
		return tag
	}

	return deriveEffectiveInboundRouteTag(tag, inboundType, options)
}

func deriveEffectiveEndpointRouteTagFromRaw(tag string, rawOptions json.RawMessage) string {
	if len(rawOptions) == 0 {
		return tag
	}

	var options map[string]interface{}
	if err := json.Unmarshal(rawOptions, &options); err != nil {
		return tag
	}

	if detour := extractDetourFromOptions(options); detour != "" {
		return detour
	}
	return tag
}

func extractDetourFromOptions(options map[string]interface{}) string {
	raw, ok := options["detour"]
	if !ok || raw == nil {
		return ""
	}

	detour, ok := raw.(string)
	if !ok {
		return ""
	}

	return strings.TrimSpace(detour)
}

func hasShadowTLSSSConfig(options map[string]interface{}) bool {
	ssConfig, ok := options["ss_config"]
	if !ok || ssConfig == nil {
		return false
	}

	if asMap, ok := ssConfig.(map[string]interface{}); ok {
		return len(asMap) > 0
	}

	return true
}

func buildInboundTagAliasMap(db *gorm.DB) (map[string]string, error) {
	aliasMap := make(map[string]string)

	var inbounds []model.Inbound
	if err := db.Model(model.Inbound{}).Find(&inbounds).Error; err != nil {
		return nil, err
	}
	for _, inbound := range inbounds {
		effectiveTag := deriveEffectiveInboundRouteTagFromRaw(inbound.Tag, inbound.Type, inbound.Options)
		if effectiveTag != "" && effectiveTag != inbound.Tag {
			aliasMap[inbound.Tag] = effectiveTag
		}
	}

	var endpoints []model.Endpoint
	if err := db.Model(model.Endpoint{}).Find(&endpoints).Error; err != nil {
		return nil, err
	}
	for _, endpoint := range endpoints {
		effectiveTag := deriveEffectiveEndpointRouteTagFromRaw(endpoint.Tag, endpoint.Options)
		if effectiveTag != "" && effectiveTag != endpoint.Tag {
			aliasMap[endpoint.Tag] = effectiveTag
		}
	}

	return aliasMap, nil
}

func normalizeConfigInboundRuleTags(configRaw json.RawMessage, aliasMap map[string]string) (json.RawMessage, bool, error) {
	if len(aliasMap) == 0 || len(configRaw) == 0 {
		return configRaw, false, nil
	}

	var config map[string]interface{}
	if err := json.Unmarshal(configRaw, &config); err != nil {
		return nil, false, err
	}

	changed := false

	if route, ok := config["route"].(map[string]interface{}); ok {
		if rules, ok := route["rules"].([]interface{}); ok {
			if normalizeRuleListInboundTags(rules, aliasMap) {
				route["rules"] = rules
				changed = true
			}
		}
	}

	if dns, ok := config["dns"].(map[string]interface{}); ok {
		if rules, ok := dns["rules"].([]interface{}); ok {
			if normalizeRuleListInboundTags(rules, aliasMap) {
				dns["rules"] = rules
				changed = true
			}
		}
	}

	if !changed {
		return configRaw, false, nil
	}

	normalized, err := json.Marshal(config)
	if err != nil {
		return nil, false, err
	}

	return normalized, true, nil
}

func normalizeRuleListInboundTags(rules []interface{}, aliasMap map[string]string) bool {
	changed := false

	for i, rawRule := range rules {
		ruleMap, ok := rawRule.(map[string]interface{})
		if !ok {
			continue
		}

		if normalizeRuleInboundTags(ruleMap, aliasMap) {
			rules[i] = ruleMap
			changed = true
		}
	}

	return changed
}

func normalizeRuleInboundTags(rule map[string]interface{}, aliasMap map[string]string) bool {
	changed := false

	if inbound, ok := rule["inbound"]; ok {
		normalized, fieldChanged := normalizeInboundField(inbound, aliasMap)
		if fieldChanged {
			rule["inbound"] = normalized
			changed = true
		}
	}

	if nestedRules, ok := rule["rules"].([]interface{}); ok {
		if normalizeRuleListInboundTags(nestedRules, aliasMap) {
			rule["rules"] = nestedRules
			changed = true
		}
	}

	return changed
}

func normalizeInboundField(rawInbound interface{}, aliasMap map[string]string) (interface{}, bool) {
	switch inbound := rawInbound.(type) {
	case string:
		normalized := resolveInboundTagAlias(inbound, aliasMap)
		return normalized, normalized != inbound
	case []string:
		changed := false
		seen := make(map[string]struct{}, len(inbound))
		normalized := make([]string, 0, len(inbound))

		for _, tag := range inbound {
			resolved := resolveInboundTagAlias(tag, aliasMap)
			if resolved != tag {
				changed = true
			}
			if _, exists := seen[resolved]; exists {
				changed = true
				continue
			}
			seen[resolved] = struct{}{}
			normalized = append(normalized, resolved)
		}

		if !changed {
			return rawInbound, false
		}
		return normalized, true
	case []interface{}:
		changed := false
		seen := make(map[string]struct{}, len(inbound))
		normalized := make([]interface{}, 0, len(inbound))

		for _, entry := range inbound {
			tag, ok := entry.(string)
			if !ok {
				normalized = append(normalized, entry)
				continue
			}

			resolved := resolveInboundTagAlias(tag, aliasMap)
			if resolved != tag {
				changed = true
			}
			if _, exists := seen[resolved]; exists {
				changed = true
				continue
			}
			seen[resolved] = struct{}{}
			normalized = append(normalized, resolved)
		}

		if !changed {
			return rawInbound, false
		}
		return normalized, true
	default:
		return rawInbound, false
	}
}

func resolveInboundTagAlias(tag string, aliasMap map[string]string) string {
	current := strings.TrimSpace(tag)
	if current == "" {
		return current
	}

	visited := map[string]struct{}{}
	for {
		next, exists := aliasMap[current]
		if !exists {
			return current
		}

		next = strings.TrimSpace(next)
		if next == "" || next == current {
			return current
		}

		if _, seen := visited[current]; seen {
			return current
		}
		visited[current] = struct{}{}
		current = next
	}
}
