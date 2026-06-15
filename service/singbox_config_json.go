package service

import (
	"encoding/json"
	"fmt"
)

func sanitizeSingboxConfigJSON(config json.RawMessage) (json.RawMessage, error) {
	return normalizeSingboxConfigJSON(config, false)
}

func sanitizeAndValidateSingboxConfigJSON(config json.RawMessage) (json.RawMessage, error) {
	return normalizeSingboxConfigJSON(config, true)
}

func normalizeSingboxConfigJSON(config json.RawMessage, validateRules bool) (json.RawMessage, error) {
	if len(config) == 0 {
		return config, nil
	}

	root := map[string]interface{}{}
	if err := json.Unmarshal(config, &root); err != nil {
		return nil, err
	}

	dnsMap, ok := root["dns"].(map[string]interface{})
	if !ok || dnsMap == nil {
		return config, nil
	}

	changed, err := sanitizeSingboxDNSMap(dnsMap, validateRules)
	if err != nil {
		return nil, err
	}
	if !changed {
		return config, nil
	}

	root["dns"] = dnsMap
	return json.Marshal(root)
}

func sanitizeSingboxDNSMap(dnsMap map[string]interface{}, validateRules bool) (bool, error) {
	if dnsMap == nil {
		return false, nil
	}

	changed := false

	if _, hasIndependentCache := dnsMap["independent_cache"]; hasIndependentCache {
		delete(dnsMap, "independent_cache")
		changed = true
	}

	if servers, ok := dnsMap["servers"].([]interface{}); ok {
		for _, item := range servers {
			serverMap, ok := item.(map[string]interface{})
			if !ok || serverMap == nil {
				continue
			}
			if _, hasStrategy := serverMap["strategy"]; !hasStrategy {
				continue
			}
			delete(serverMap, "strategy")
			changed = true
		}
	}

	if validateRules {
		if err := validateSingboxDNSRulesCompatibility(dnsMap); err != nil {
			return changed, err
		}
	}

	return changed, nil
}

type singboxDNSRuleCompatibilityState struct {
	hasIPVersionOrQueryType           bool
	hasLegacyAddressFilter            bool
	hasLegacyRuleActionStrategy       bool
	hasLegacyRuleSetIPCIDRAcceptEmpty bool
}

func validateSingboxDNSRulesCompatibility(dnsMap map[string]interface{}) error {
	rules, ok := dnsMap["rules"].([]interface{})
	if !ok || len(rules) == 0 {
		return nil
	}

	state := singboxDNSRuleCompatibilityState{}
	collectSingboxDNSRuleCompatibilityState(rules, &state)

	if !state.hasIPVersionOrQueryType {
		return nil
	}
	if !state.hasLegacyAddressFilter && !state.hasLegacyRuleActionStrategy && !state.hasLegacyRuleSetIPCIDRAcceptEmpty {
		return nil
	}

	return fmt.Errorf(
		"dns rule compatibility: ip_version/query_type cannot be combined with legacy dns rule strategy, legacy address filter fields (ip_cidr/ip_is_private without match_response), or rule_set_ip_cidr_accept_empty; migrate to evaluate + match_response or move strategy to dns.strategy / a dedicated dns server",
	)
}

func collectSingboxDNSRuleCompatibilityState(rules []interface{}, state *singboxDNSRuleCompatibilityState) {
	for _, rawRule := range rules {
		ruleMap, ok := rawRule.(map[string]interface{})
		if !ok || ruleMap == nil {
			continue
		}

		if hasSingboxDNSRuleIPVersionOrQueryType(ruleMap) {
			state.hasIPVersionOrQueryType = true
		}
		if hasSingboxDNSRuleActionStrategy(ruleMap) {
			state.hasLegacyRuleActionStrategy = true
		}
		if hasSingboxLegacyDNSRuleAddressFilter(ruleMap) {
			state.hasLegacyAddressFilter = true
		}
		if _, hasLegacyRuleSetItem := ruleMap["rule_set_ip_cidr_accept_empty"]; hasLegacyRuleSetItem {
			state.hasLegacyRuleSetIPCIDRAcceptEmpty = true
		}

		childRules, ok := ruleMap["rules"].([]interface{})
		if ok && len(childRules) > 0 {
			collectSingboxDNSRuleCompatibilityState(childRules, state)
		}
	}
}

func hasSingboxDNSRuleIPVersionOrQueryType(rule map[string]interface{}) bool {
	if hasNonEmptyJSONValue(rule["ip_version"]) {
		return true
	}
	return hasNonEmptyJSONValue(rule["query_type"])
}

func hasSingboxDNSRuleActionStrategy(rule map[string]interface{}) bool {
	return hasNonEmptyJSONValue(rule["strategy"])
}

func hasSingboxLegacyDNSRuleAddressFilter(rule map[string]interface{}) bool {
	matchResponse, _ := rule["match_response"].(bool)
	if matchResponse {
		return false
	}

	if hasNonEmptyJSONValue(rule["ip_cidr"]) {
		return true
	}
	_, hasIPIsPrivate := rule["ip_is_private"]
	return hasIPIsPrivate
}

func hasNonEmptyJSONValue(value interface{}) bool {
	switch typed := value.(type) {
	case nil:
		return false
	case string:
		return typed != ""
	case bool:
		return true
	case float64:
		return typed != 0
	case int:
		return typed != 0
	case int32:
		return typed != 0
	case int64:
		return typed != 0
	case []interface{}:
		return len(typed) > 0
	case []string:
		return len(typed) > 0
	case map[string]interface{}:
		return len(typed) > 0
	default:
		return true
	}
}
