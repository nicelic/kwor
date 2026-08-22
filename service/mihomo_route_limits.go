package service

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"

	"github.com/alireza0/s-ui/database/model"
	"gorm.io/gorm"
)

const (
	maxMihomoConfigBytes            = 2 * 1024 * 1024
	maxMihomoRouteRules             = 256
	maxMihomoRuleProviders          = 128
	maxMihomoRuleValuesPerMatcher   = 64
	maxMihomoRuleValueBytes         = 1024
	maxMihomoRuleCombinations       = 512
	maxMihomoRouteBaseRenderedRules = 4096
	maxMihomoRouteRenderedRules     = 8192
	maxMihomoGeneratedYAMLBytes     = 4 * 1024 * 1024
	maxMihomoUID                    = int64(9007199254740991) // JavaScript Number.MAX_SAFE_INTEGER
)

var mihomoRouteMatcherFields = []string{
	"domain",
	"domain_suffix",
	"domain_keyword",
	"domain_regex",
	"ip_cidr",
	"network",
	"auth_user",
	"source_ip_cidr",
	"process_name",
	"process_path",
	"process_path_regex",
	"user_id",
	"rule_set",
}

type MihomoRouteEditorContext struct {
	InboundTags  []string
	RouteTargets []string
}

// GetMihomoRouteEditorContext returns only the data the route editor needs.
// It deliberately avoids the full Mihomo dashboard payload, whose client,
// TLS and subscription collections are unrelated to editing route rules.
func GetMihomoRouteEditorContext(db *gorm.DB) (*MihomoRouteEditorContext, error) {
	targets, err := loadMihomoRouteTargets(db)
	if err != nil {
		return nil, err
	}

	inboundTags, err := loadMihomoRouteInboundRuleTags(db, targets)
	if err != nil {
		return nil, err
	}

	routeTargets := make([]string, 0, len(targets.SupportedTags))
	for tag := range targets.SupportedTags {
		routeTargets = append(routeTargets, tag)
	}
	sort.Strings(routeTargets)

	return &MihomoRouteEditorContext{
		InboundTags:  sortedMihomoRouteInboundRuleTags(inboundTags),
		RouteTargets: routeTargets,
	}, nil
}

func loadMihomoRouteTargets(db *gorm.DB) (*mihomoProxyConversionResult, error) {
	if db == nil {
		return nil, fmt.Errorf("mihomo route target validation requires a database")
	}

	var outbounds []model.MihomoOutbound
	if err := db.Model(model.MihomoOutbound{}).Order("id ASC").Find(&outbounds).Error; err != nil {
		return nil, fmt.Errorf("load mihomo outbounds failed: %w", err)
	}

	rawOutbounds := make([]map[string]interface{}, 0, len(outbounds))
	for index := range outbounds {
		rawJSON, err := resolveMihomoOutboundJSON(&outbounds[index])
		if err != nil {
			return nil, fmt.Errorf("decode mihomo outbound %s failed: %w", outbounds[index].Tag, err)
		}
		rawMap, err := marshalJSONMap(rawJSON)
		if err != nil {
			return nil, fmt.Errorf("decode mihomo outbound %s failed: %w", outbounds[index].Tag, err)
		}
		rawOutbounds = append(rawOutbounds, rawMap)
	}

	result := convertMihomoOutboundsToClash(rawOutbounds)
	if len(result.ValidationErrs) > 0 {
		return nil, fmt.Errorf("invalid mihomo outbound config: %s", strings.Join(result.ValidationErrs, "; "))
	}
	return result, nil
}

// GetMihomoRouteTargets returns only targets that the current Mihomo renderer
// can actually emit. The UI must not offer database records that cannot become
// a proxy or proxy group in server.yaml.
func GetMihomoRouteTargets(db *gorm.DB) ([]string, error) {
	targets, err := loadMihomoRouteTargets(db)
	if err != nil {
		return nil, err
	}

	items := make([]string, 0, len(targets.SupportedTags))
	for tag := range targets.SupportedTags {
		items = append(items, tag)
	}
	sort.Strings(items)
	return items, nil
}

func validateMihomoConfigRouteBounds(config json.RawMessage, db *gorm.DB) error {
	if len(config) == 0 {
		return nil
	}
	if len(config) > maxMihomoConfigBytes {
		return fmt.Errorf("mihomo configuration exceeds the %d byte safety limit", maxMihomoConfigBytes)
	}

	var document map[string]interface{}
	if err := json.Unmarshal(config, &document); err != nil {
		return err
	}
	route, ok := document["route"].(map[string]interface{})
	if !ok || route == nil {
		return nil
	}

	targets, err := loadMihomoRouteTargets(db)
	if err != nil {
		return err
	}
	if final := strings.TrimSpace(firstString(route["final"])); final != "" {
		if _, ok := normalizeMihomoRouteTarget(final, targets); !ok {
			return fmt.Errorf("route.final references unsupported Mihomo target %q", final)
		}
	}

	providerTags, err := validateMihomoRuleProviders(route["rule_set"], targets)
	if err != nil {
		return err
	}

	rawRules, ok := route["rules"].([]interface{})
	if !ok {
		return nil
	}
	if len(rawRules) > maxMihomoRouteRules {
		return fmt.Errorf("mihomo route rules exceed the %d rule safety limit", maxMihomoRouteRules)
	}

	totalCombinations := 0
	inboundRouteCount, err := countMihomoRouteRefs(db, targets)
	if err != nil {
		return err
	}
	// The global list and every retained inbound sub-rule always receive a
	// terminal MATCH rule. Include them in the same limit as expanded rules.
	renderedRuleCount := inboundRouteCount + 1
	if renderedRuleCount > maxMihomoRouteRenderedRules {
		return fmt.Errorf("mihomo route terminal rules exceed the %d generated-rule safety limit", maxMihomoRouteRenderedRules)
	}
	for index, rawRule := range rawRules {
		rule, ok := rawRule.(map[string]interface{})
		if !ok || rule == nil {
			return fmt.Errorf("mihomo route rule #%d is invalid", index+1)
		}
		if err := validateMihomoRouteRuleBounds(rule, providerTags, targets); err != nil {
			return fmt.Errorf("mihomo route rule #%d: %w", index+1, err)
		}

		combinations, err := mihomoRuleCombinationCount(rule)
		if err != nil {
			return fmt.Errorf("mihomo route rule #%d: %w", index+1, err)
		}
		totalCombinations += combinations
		if totalCombinations > maxMihomoRouteBaseRenderedRules {
			return fmt.Errorf("mihomo route rules expand beyond the %d generated-rule safety limit", maxMihomoRouteBaseRenderedRules)
		}

		multiplier := 1
		if len(normalizeMihomoInboundRules(rule["inbound"], nil)) == 0 {
			multiplier += inboundRouteCount
		} else {
			multiplier = len(normalizeMihomoInboundRules(rule["inbound"], nil))
		}
		if combinations > maxMihomoRouteRenderedRules/multiplier {
			return fmt.Errorf("mihomo route rule output exceeds the %d generated-rule safety limit", maxMihomoRouteRenderedRules)
		}
		renderedRuleCount += combinations * multiplier
		if renderedRuleCount > maxMihomoRouteRenderedRules {
			return fmt.Errorf("mihomo route rules expand beyond the %d generated-rule safety limit", maxMihomoRouteRenderedRules)
		}
	}

	return nil
}

func countMihomoRouteRefs(db *gorm.DB, targets *mihomoProxyConversionResult) (int, error) {
	var inbounds []model.MihomoInbound
	if err := db.Model(model.MihomoInbound{}).Preload("Tls").Find(&inbounds).Error; err != nil {
		return 0, fmt.Errorf("load mihomo inbounds for route bounds failed: %w", err)
	}
	refs := map[string]struct{}{}
	for _, inbound := range filterSupportedMihomoListeners(inbounds) {
		ref, err := buildMihomoInboundRouteRef(inbound, targets, "DIRECT")
		if err != nil || strings.TrimSpace(ref.RuleName) == "" {
			continue
		}
		refs[ref.RuleName] = struct{}{}
	}
	return len(refs), nil
}

func validateMihomoRuleProviders(raw interface{}, targets *mihomoProxyConversionResult) (map[string]struct{}, error) {
	items, ok := raw.([]interface{})
	if raw == nil {
		return map[string]struct{}{}, nil
	}
	if !ok {
		return nil, fmt.Errorf("mihomo route.rule_set must be an array")
	}
	if len(items) > maxMihomoRuleProviders {
		return nil, fmt.Errorf("mihomo rule providers exceed the %d provider safety limit", maxMihomoRuleProviders)
	}

	tags := make(map[string]struct{}, len(items))
	for index, item := range items {
		provider, ok := item.(map[string]interface{})
		if !ok || provider == nil {
			return nil, fmt.Errorf("mihomo rule provider #%d is invalid", index+1)
		}
		tag := strings.TrimSpace(firstString(provider["tag"]))
		if tag == "" || len(tag) > maxMihomoRuleValueBytes {
			return nil, fmt.Errorf("mihomo rule provider #%d has an invalid tag", index+1)
		}
		if _, exists := tags[tag]; exists {
			return nil, fmt.Errorf("mihomo rule provider tag %q is duplicated", tag)
		}
		tags[tag] = struct{}{}

		typeName := strings.ToLower(strings.TrimSpace(firstString(provider["type"])))
		switch typeName {
		case "file", "local":
			path := strings.TrimSpace(firstString(provider["path"]))
			if path == "" {
				return nil, fmt.Errorf("mihomo rule provider %q requires a path", tag)
			}
			if len(path) > maxMihomoRuleValueBytes {
				return nil, fmt.Errorf("mihomo rule provider %q path exceeds the %d byte safety limit", tag, maxMihomoRuleValueBytes)
			}
		case "http", "remote":
			url := strings.TrimSpace(firstString(provider["url"]))
			if url == "" {
				return nil, fmt.Errorf("mihomo rule provider %q requires a URL", tag)
			}
			if len(url) > maxMihomoRuleValueBytes {
				return nil, fmt.Errorf("mihomo rule provider %q URL exceeds the %d byte safety limit", tag, maxMihomoRuleValueBytes)
			}
			proxy := strings.TrimSpace(firstString(provider["proxy"]))
			if proxy == "" {
				proxy = strings.TrimSpace(firstString(provider["download_detour"]))
			}
			if proxy != "" {
				if _, ok := normalizeMihomoRouteTarget(proxy, targets); !ok {
					return nil, fmt.Errorf("mihomo rule provider %q references unsupported proxy %q", tag, proxy)
				}
			}
		case "inline":
			if err := validateMihomoStringList(provider["payload"], "payload"); err != nil {
				return nil, fmt.Errorf("mihomo rule provider %q: %w", tag, err)
			}
			if len(toStringSlice(provider["payload"])) == 0 {
				return nil, fmt.Errorf("mihomo rule provider %q requires payload entries", tag)
			}
		default:
			return nil, fmt.Errorf("mihomo rule provider %q uses unsupported type %q", tag, typeName)
		}
	}

	return tags, nil
}

func validateMihomoRouteRuleBounds(rule map[string]interface{}, providerTags map[string]struct{}, targets *mihomoProxyConversionResult) error {
	action := strings.TrimSpace(firstString(rule["action"]))
	switch action {
	case "route":
		target := strings.TrimSpace(firstString(rule["outbound"]))
		if target == "" {
			return fmt.Errorf("route action requires an outbound target")
		}
		if _, ok := normalizeMihomoRouteTarget(target, targets); !ok {
			return fmt.Errorf("route action references unsupported Mihomo target %q", target)
		}
	case "reject":
	default:
		return fmt.Errorf("uses unsupported action %q", action)
	}

	for _, field := range mihomoRouteMatcherFields {
		if err := validateMihomoStringList(rule[field], field); err != nil {
			return err
		}
	}
	if err := validateMihomoRouteNumericMatchers(rule); err != nil {
		return err
	}
	if err := validateMihomoRuleValues(rule["inbound"], "inbound"); err != nil {
		return err
	}

	for _, tag := range normalizeMihomoRuleSetRefs(rule["rule_set"]) {
		if _, exists := providerTags[tag]; !exists {
			return fmt.Errorf("references unknown rule provider %q", tag)
		}
	}
	_, err := mihomoRuleCombinationCount(rule)
	return err
}

func validateMihomoStringList(raw interface{}, field string) error {
	values := toStringSlice(raw)
	if len(values) > maxMihomoRuleValuesPerMatcher {
		return fmt.Errorf("%s exceeds the %d value safety limit", field, maxMihomoRuleValuesPerMatcher)
	}
	for _, value := range values {
		if len(value) > maxMihomoRuleValueBytes {
			return fmt.Errorf("%s contains a value longer than %d bytes", field, maxMihomoRuleValueBytes)
		}
	}
	return nil
}

func validateMihomoRuleValues(raw interface{}, field string) error {
	if raw == nil {
		return nil
	}
	values := asMihomoRuleValues(raw)
	if len(values) > maxMihomoRuleValuesPerMatcher {
		return fmt.Errorf("%s exceeds the %d value safety limit", field, maxMihomoRuleValuesPerMatcher)
	}
	for _, value := range values {
		if text, ok := value.(string); ok && len(text) > maxMihomoRuleValueBytes {
			return fmt.Errorf("%s contains a value longer than %d bytes", field, maxMihomoRuleValueBytes)
		}
	}
	return nil
}

func validateMihomoRouteNumericMatchers(rule map[string]interface{}) error {
	checks := []struct {
		raw   interface{}
		field string
		min   int64
		max   int64
	}{
		{rule["port"], "port", 1, 65535},
		{rule["source_port"], "source_port", 1, 65535},
		{rule["user_id"], "user_id", 0, maxMihomoUID},
	}
	for _, check := range checks {
		if err := validateMihomoRuleValues(check.raw, check.field); err != nil {
			return err
		}
		if err := validateMihomoIntegerValues(check.raw, check.field, check.min, check.max); err != nil {
			return err
		}
	}
	for _, check := range []struct {
		raw   interface{}
		field string
	}{
		{rule["port_range"], "port_range"},
		{rule["source_port_range"], "source_port_range"},
	} {
		if err := validateMihomoRuleValues(check.raw, check.field); err != nil {
			return err
		}
		if err := validateMihomoPortRanges(check.raw, check.field); err != nil {
			return err
		}
	}
	return nil
}

// validateMihomoIntegerValues deliberately rejects numeric strings. The UI
// and JSON API must preserve the distinction between an integer and malformed
// text such as "80x" instead of allowing the renderer to truncate it.
func validateMihomoIntegerValues(raw interface{}, field string, min, max int64) error {
	if raw == nil {
		return nil
	}
	for index, value := range asMihomoRuleValues(raw) {
		parsed, ok := strictMihomoInteger(value)
		if !ok || parsed < min || parsed > max {
			return fmt.Errorf("%s[%d] must be an integer from %d to %d", field, index+1, min, max)
		}
	}
	return nil
}

func validateMihomoPortRanges(raw interface{}, field string) error {
	if raw == nil {
		return nil
	}
	for index, value := range asMihomoRuleValues(raw) {
		text, ok := value.(string)
		if !ok {
			return fmt.Errorf("%s[%d] must use the form start-end", field, index+1)
		}
		if _, ok := normalizeMihomoPortRangeValue(text); !ok {
			return fmt.Errorf("%s[%d] must use the form start-end", field, index+1)
		}
	}
	return nil
}

func normalizeMihomoPortRangeValue(value string) (string, bool) {
	text := strings.TrimSpace(value)
	parts := strings.Split(text, "-")
	if len(parts) != 2 || !isMihomoDecimal(parts[0]) || !isMihomoDecimal(parts[1]) {
		return "", false
	}
	start, startErr := strconv.ParseInt(parts[0], 10, 64)
	end, endErr := strconv.ParseInt(parts[1], 10, 64)
	if startErr != nil || endErr != nil || start < 1 || end > 65535 || start > end {
		return "", false
	}
	return fmt.Sprintf("%d-%d", start, end), true
}

func isMihomoDecimal(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func strictMihomoInteger(raw interface{}) (int64, bool) {
	const maxInt64 = int64(^uint64(0) >> 1)
	const minInt64 = -maxInt64 - 1
	switch value := raw.(type) {
	case int:
		return int64(value), true
	case int8:
		return int64(value), true
	case int16:
		return int64(value), true
	case int32:
		return int64(value), true
	case int64:
		return value, true
	case uint:
		if uint64(value) > uint64(maxInt64) {
			return 0, false
		}
		return int64(value), true
	case uint8:
		return int64(value), true
	case uint16:
		return int64(value), true
	case uint32:
		return int64(value), true
	case uint64:
		if value > uint64(maxInt64) {
			return 0, false
		}
		return int64(value), true
	case float32:
		return strictMihomoFloatInteger(float64(value), minInt64, maxInt64)
	case float64:
		return strictMihomoFloatInteger(value, minInt64, maxInt64)
	default:
		return 0, false
	}
}

func strictMihomoFloatInteger(value float64, min, max int64) (int64, bool) {
	if math.IsNaN(value) || math.IsInf(value, 0) || math.Trunc(value) != value {
		return 0, false
	}
	if value < float64(min) || value >= float64(max) {
		return 0, false
	}
	return int64(value), true
}

func asMihomoRuleValues(raw interface{}) []interface{} {
	switch value := raw.(type) {
	case []interface{}:
		return value
	case []string:
		result := make([]interface{}, 0, len(value))
		for _, item := range value {
			result = append(result, item)
		}
		return result
	case []int:
		result := make([]interface{}, 0, len(value))
		for _, item := range value {
			result = append(result, item)
		}
		return result
	case []int8:
		result := make([]interface{}, 0, len(value))
		for _, item := range value {
			result = append(result, item)
		}
		return result
	case []int16:
		result := make([]interface{}, 0, len(value))
		for _, item := range value {
			result = append(result, item)
		}
		return result
	case []int32:
		result := make([]interface{}, 0, len(value))
		for _, item := range value {
			result = append(result, item)
		}
		return result
	case []int64:
		result := make([]interface{}, 0, len(value))
		for _, item := range value {
			result = append(result, item)
		}
		return result
	case []uint:
		result := make([]interface{}, 0, len(value))
		for _, item := range value {
			result = append(result, item)
		}
		return result
	case []uint8:
		result := make([]interface{}, 0, len(value))
		for _, item := range value {
			result = append(result, item)
		}
		return result
	case []uint16:
		result := make([]interface{}, 0, len(value))
		for _, item := range value {
			result = append(result, item)
		}
		return result
	case []uint32:
		result := make([]interface{}, 0, len(value))
		for _, item := range value {
			result = append(result, item)
		}
		return result
	case []uint64:
		result := make([]interface{}, 0, len(value))
		for _, item := range value {
			result = append(result, item)
		}
		return result
	case []float32:
		result := make([]interface{}, 0, len(value))
		for _, item := range value {
			result = append(result, item)
		}
		return result
	case []float64:
		result := make([]interface{}, 0, len(value))
		for _, item := range value {
			result = append(result, item)
		}
		return result
	default:
		return []interface{}{raw}
	}
}

func mihomoRuleCombinationCount(rule map[string]interface{}) (int, error) {
	if rule == nil {
		return 1, nil
	}
	groups := make([]int, 0, len(mihomoRouteMatcherFields)+4)
	for _, field := range mihomoRouteMatcherFields {
		if field == "user_id" {
			continue
		}
		if count := len(toStringSlice(rule[field])); count > 0 {
			groups = append(groups, count)
		}
	}
	if count := len(toIntList(rule["port"])) + len(toStringSlice(rule["port_range"])); count > 0 {
		groups = append(groups, count)
	}
	if count := len(toIntList(rule["source_port"])) + len(toStringSlice(rule["source_port_range"])); count > 0 {
		groups = append(groups, count)
	}
	if enabled, _ := toBool(rule["ip_is_private"]); enabled {
		groups = append(groups, 1)
	}
	if enabled, _ := toBool(rule["source_ip_is_private"]); enabled {
		groups = append(groups, 1)
	}
	if count := len(toIntList(rule["user_id"])); count > 0 {
		groups = append(groups, count)
	}

	combinations := 1
	for _, count := range groups {
		if count <= 0 {
			continue
		}
		if combinations > maxMihomoRuleCombinations/count {
			return 0, fmt.Errorf("matcher values expand beyond the %d combination safety limit", maxMihomoRuleCombinations)
		}
		combinations *= count
	}
	return combinations, nil
}

func buildMihomoConfigChangeAudit(config json.RawMessage) json.RawMessage {
	audit := map[string]interface{}{
		"bytes": len(config),
	}
	var document map[string]interface{}
	if err := json.Unmarshal(config, &document); err != nil {
		return mustMarshalMihomoConfigAudit(audit)
	}
	route, _ := document["route"].(map[string]interface{})
	if route != nil {
		audit["route_rules"] = len(toInterfaceSlice(route["rules"]))
		audit["rule_providers"] = len(toInterfaceSlice(route["rule_set"]))
		if final := strings.TrimSpace(firstString(route["final"])); final != "" {
			audit["final"] = final
		}
	}
	if sniffer, ok := document["sniffer"].(map[string]interface{}); ok {
		if enabled, _ := toBool(sniffer["enable"]); enabled {
			audit["sniff"] = true
		}
	}
	return mustMarshalMihomoConfigAudit(audit)
}

func mustMarshalMihomoConfigAudit(audit map[string]interface{}) json.RawMessage {
	data, err := json.Marshal(audit)
	if err != nil {
		return json.RawMessage(`{"summary":"mihomo_config"}`)
	}
	return data
}

func toInterfaceSlice(raw interface{}) []interface{} {
	values, _ := raw.([]interface{})
	return values
}
