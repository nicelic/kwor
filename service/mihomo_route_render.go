package service

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/logger"
)

type mihomoRouteRenderResult struct {
	Rules          []string
	SubRules       map[string][]string
	ValidationErrs []string
}

type mihomoInboundRouteRef struct {
	RuleName      string
	DefaultTarget string
	ProxyTarget   string
}

func renderMihomoRoutes(route map[string]interface{}, providerTags map[string]struct{}, ipRuleProviderTags map[string]struct{}, targets *mihomoProxyConversionResult, globalFinal string, inboundRefs map[string]mihomoInboundRouteRef, inboundAlias map[string]string) *mihomoRouteRenderResult {
	result := &mihomoRouteRenderResult{
		Rules:          []string{},
		SubRules:       map[string][]string{},
		ValidationErrs: []string{},
	}

	globalFinal = strings.TrimSpace(globalFinal)
	if globalFinal == "" {
		resolvedFinal, ok := resolveMihomoGlobalFinalTarget(route, targets, nil)
		if !ok {
			rawFinal := strings.TrimSpace(firstString(route["final"]))
			if rawFinal != "" {
				result.ValidationErrs = append(result.ValidationErrs, fmt.Sprintf("route.final references unknown outbound %q", rawFinal))
			}
			globalFinal = "DIRECT"
		} else {
			globalFinal = resolvedFinal
		}
	}

	rawRules, ok := route["rules"].([]interface{})
	if !ok || len(rawRules) == 0 {
		result.Rules = append(result.Rules, "MATCH,"+globalFinal)
		for _, ref := range inboundRefs {
			if ref.RuleName == "" {
				continue
			}
			finalTarget := ref.DefaultTarget
			if finalTarget == "" {
				finalTarget = globalFinal
			}
			result.SubRules[ref.RuleName] = []string{"MATCH," + finalTarget}
		}
		if len(result.SubRules) == 0 {
			result.SubRules = nil
		}
		return result
	}

	subRuleOrder := make([]string, 0, len(inboundRefs))
	seenSubRules := map[string]struct{}{}
	for _, ref := range inboundRefs {
		if ref.RuleName == "" {
			continue
		}
		if _, exists := seenSubRules[ref.RuleName]; exists {
			continue
		}
		seenSubRules[ref.RuleName] = struct{}{}
		subRuleOrder = append(subRuleOrder, ref.RuleName)
		result.SubRules[ref.RuleName] = []string{}
	}

	noResolveEnabled := isMihomoRouteNoResolveEnabled(route)

	for _, rawRule := range rawRules {
		rule, ok := rawRule.(map[string]interface{})
		if !ok || rule == nil {
			continue
		}

		ruleStrings, ok := buildMihomoRuleStrings(rule, providerTags, ipRuleProviderTags, targets, noResolveEnabled)
		if !ok || len(ruleStrings) == 0 {
			if msg := describeInvalidMihomoRule(rule, providerTags, targets); msg != "" {
				result.ValidationErrs = append(result.ValidationErrs, msg)
			}
			continue
		}

		inbounds := normalizeMihomoInboundRules(rule["inbound"], inboundAlias)
		if len(inbounds) == 0 {
			result.Rules = append(result.Rules, ruleStrings...)
			for _, ruleName := range subRuleOrder {
				result.SubRules[ruleName] = append(result.SubRules[ruleName], ruleStrings...)
			}
			continue
		}

		for _, inboundTag := range inbounds {
			ref, exists := inboundRefs[inboundTag]
			if !exists || ref.RuleName == "" {
				continue
			}
			result.SubRules[ref.RuleName] = append(result.SubRules[ref.RuleName], ruleStrings...)
		}
	}

	result.Rules = appendMihomoTerminalMatch(result.Rules, globalFinal)
	for _, ruleName := range subRuleOrder {
		ref := inboundRefs[ruleName]
		finalTarget := ref.DefaultTarget
		if finalTarget == "" {
			finalTarget = globalFinal
		}
		result.SubRules[ruleName] = appendMihomoTerminalMatch(result.SubRules[ruleName], finalTarget)
	}

	if len(result.SubRules) == 0 {
		result.SubRules = nil
	}

	return result
}

func buildMihomoRuleStrings(rule map[string]interface{}, providerTags map[string]struct{}, ipRuleProviderTags map[string]struct{}, targets *mihomoProxyConversionResult, noResolveEnabled bool) ([]string, bool) {
	target, ok := resolveMihomoRuleTarget(rule, targets)
	if !ok {
		return nil, false
	}

	matcherGroups := make([][]string, 0, 16)
	hasTargetIPMatcher := false
	if values := buildStringMatcherAtoms("DOMAIN", rule["domain"]); len(values) > 0 {
		matcherGroups = append(matcherGroups, values)
	}
	if values := buildStringMatcherAtoms("DOMAIN-SUFFIX", rule["domain_suffix"]); len(values) > 0 {
		matcherGroups = append(matcherGroups, values)
	}
	if values := buildStringMatcherAtoms("DOMAIN-KEYWORD", rule["domain_keyword"]); len(values) > 0 {
		matcherGroups = append(matcherGroups, values)
	}
	if values := buildStringMatcherAtoms("DOMAIN-REGEX", rule["domain_regex"]); len(values) > 0 {
		matcherGroups = append(matcherGroups, values)
	}
	if values := buildIPCIDRAtoms(rule["ip_cidr"]); len(values) > 0 {
		matcherGroups = append(matcherGroups, values)
		hasTargetIPMatcher = true
	}
	if values := buildMihomoPrivateIPAtoms(rule["ip_is_private"]); len(values) > 0 {
		matcherGroups = append(matcherGroups, values)
		hasTargetIPMatcher = true
	}
	if values := buildNetworkAtoms(rule["network"]); len(values) > 0 {
		matcherGroups = append(matcherGroups, values)
	}
	if values := buildStringMatcherAtoms("IN-USER", rule["auth_user"]); len(values) > 0 {
		matcherGroups = append(matcherGroups, values)
	}
	if values := buildIPCIDRMatcherAtoms("SRC-IP-CIDR", rule["source_ip_cidr"]); len(values) > 0 {
		matcherGroups = append(matcherGroups, values)
	}
	if values := buildMihomoSourcePrivateIPAtoms(rule["source_ip_is_private"]); len(values) > 0 {
		matcherGroups = append(matcherGroups, values)
	}
	if values := buildPortMatcherAtoms("SRC-PORT", rule["source_port"], rule["source_port_range"]); len(values) > 0 {
		matcherGroups = append(matcherGroups, values)
	}
	if values := buildPortMatcherAtoms("DST-PORT", rule["port"], rule["port_range"]); len(values) > 0 {
		matcherGroups = append(matcherGroups, values)
	}
	if values := buildStringMatcherAtoms("PROCESS-NAME", rule["process_name"]); len(values) > 0 {
		matcherGroups = append(matcherGroups, values)
	}
	if values := buildStringMatcherAtoms("PROCESS-PATH", rule["process_path"]); len(values) > 0 {
		matcherGroups = append(matcherGroups, values)
	}
	if values := buildStringMatcherAtoms("PROCESS-PATH-REGEX", rule["process_path_regex"]); len(values) > 0 {
		matcherGroups = append(matcherGroups, values)
	}
	if values := buildIntegerMatcherAtoms("UID", rule["user_id"]); len(values) > 0 {
		matcherGroups = append(matcherGroups, values)
	}
	if refs := normalizeMihomoRuleSetRefs(rule["rule_set"]); len(refs) > 0 {
		if missing := collectMissingMihomoRuleProviderTags(refs, providerTags); len(missing) > 0 {
			return nil, false
		}
		if hasMihomoTargetIPRuleSetMatcher(refs, ipRuleProviderTags) {
			hasTargetIPMatcher = true
		}
		if values := buildRuleSetAtoms(refs, rule["rule_set_ip_cidr_match_source"]); len(values) > 0 {
			matcherGroups = append(matcherGroups, values)
		}
	}

	if len(matcherGroups) == 0 {
		return []string{"MATCH," + target}, true
	}

	combinations := expandMihomoMatcherGroups(matcherGroups)
	if len(combinations) == 0 {
		return nil, false
	}

	rendered := make([]string, 0, len(combinations))
	for _, atoms := range combinations {
		if len(atoms) == 0 {
			continue
		}
		if len(atoms) == 1 {
			ruleString := atoms[0] + "," + target
			if noResolveEnabled && hasTargetIPMatcher {
				ruleString += ",no-resolve"
			}
			rendered = append(rendered, ruleString)
			continue
		}

		parts := make([]string, 0, len(atoms))
		for _, atom := range atoms {
			parts = append(parts, "("+atom+")")
		}
		ruleString := "AND,(" + strings.Join(parts, ",") + ")," + target
		if noResolveEnabled && hasTargetIPMatcher {
			ruleString += ",no-resolve"
		}
		rendered = append(rendered, ruleString)
	}
	return rendered, len(rendered) > 0
}

func isMihomoRouteNoResolveEnabled(route map[string]interface{}) bool {
	if route == nil {
		return true
	}
	if enabled, ok := toBool(route["no_resolve"]); ok {
		return enabled
	}
	if enabled, ok := toBool(route["no-resolve"]); ok {
		return enabled
	}
	if enabled, ok := toBool(route["noResolve"]); ok {
		return enabled
	}
	return true
}

func resolveMihomoRuleTarget(rule map[string]interface{}, targets *mihomoProxyConversionResult) (string, bool) {
	action, _ := rule["action"].(string)
	switch action {
	case "reject":
		method, _ := rule["method"].(string)
		if strings.EqualFold(strings.TrimSpace(method), "drop") {
			return "REJECT-DROP", true
		}
		return "REJECT", true
	case "route":
		return normalizeMihomoRouteTarget(firstString(rule["outbound"]), targets)
	default:
		return "", false
	}
}

func describeInvalidMihomoRule(rule map[string]interface{}, providerTags map[string]struct{}, targets *mihomoProxyConversionResult) string {
	action := strings.TrimSpace(firstString(rule["action"]))
	if missing := collectMissingMihomoRuleProviderTags(normalizeMihomoRuleSetRefs(rule["rule_set"]), providerTags); len(missing) > 0 {
		return fmt.Sprintf("route rule references unknown rule_set provider(s): %s", strings.Join(missing, ", "))
	}
	switch action {
	case "reject":
		return ""
	case "route":
		outbound := strings.TrimSpace(firstString(rule["outbound"]))
		if outbound == "" {
			return "route rule is missing an outbound target"
		}
		if _, ok := normalizeMihomoRouteTarget(outbound, targets); !ok {
			return fmt.Sprintf("route rule references unknown outbound %q", outbound)
		}
		return ""
	case "":
		return "route rule is missing an action"
	default:
		return fmt.Sprintf("route rule uses unsupported action %q", action)
	}
}

func normalizeMihomoRouteTarget(raw string, targets *mihomoProxyConversionResult) (string, bool) {
	if targets == nil {
		switch strings.ToUpper(strings.TrimSpace(raw)) {
		case "DIRECT":
			return "DIRECT", true
		case "REJECT", "BLOCK":
			return "REJECT", true
		case "REJECT-DROP":
			return "REJECT-DROP", true
		}
		return "", false
	}
	return normalizeMihomoTarget(raw, targets)
}

func resolveMihomoGlobalFinalTarget(route map[string]interface{}, targets *mihomoProxyConversionResult, fallbackCandidates []string) (string, bool) {
	rawFinal := strings.TrimSpace(firstString(route["final"]))
	if rawFinal != "" {
		return normalizeMihomoRouteTarget(rawFinal, targets)
	}

	for _, candidate := range fallbackCandidates {
		normalized, ok := normalizeMihomoRouteTarget(candidate, targets)
		if ok {
			return normalized, true
		}
	}

	return "DIRECT", true
}

func buildStringMatcherAtoms(matcher string, raw interface{}) []string {
	values := toStringSlice(raw)
	if len(values) == 0 {
		return nil
	}
	atoms := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		atoms = append(atoms, matcher+","+value)
	}
	return atoms
}

func buildIPCIDRAtoms(raw interface{}) []string {
	values := toStringSlice(raw)
	if len(values) == 0 {
		return nil
	}
	atoms := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		matcher := "IP-CIDR"
		if strings.Contains(value, ":") {
			matcher = "IP-CIDR6"
		}
		atoms = append(atoms, matcher+","+value)
	}
	return atoms
}

func buildIPCIDRMatcherAtoms(matcher string, raw interface{}) []string {
	values := toStringSlice(raw)
	if len(values) == 0 {
		return nil
	}
	atoms := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		atoms = append(atoms, matcher+","+value)
	}
	return atoms
}

func buildNetworkAtoms(raw interface{}) []string {
	values := toStringSlice(raw)
	if len(values) == 0 {
		return nil
	}
	atoms := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.ToUpper(strings.TrimSpace(value))
		if value != "TCP" && value != "UDP" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		atoms = append(atoms, "NETWORK,"+value)
	}
	return atoms
}

func buildMihomoPrivateIPAtoms(raw interface{}) []string {
	enabled, ok := toBool(raw)
	if !ok || !enabled {
		return nil
	}
	return []string{"GEOIP,private"}
}

func buildMihomoSourcePrivateIPAtoms(raw interface{}) []string {
	enabled, ok := toBool(raw)
	if !ok || !enabled {
		return nil
	}
	return []string{"SRC-GEOIP,private"}
}

func buildPortMatcherAtoms(matcher string, portsRaw interface{}, rangesRaw interface{}) []string {
	atoms := []string{}
	seen := map[string]struct{}{}
	add := func(value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		atom := matcher + "," + value
		if _, exists := seen[atom]; exists {
			return
		}
		seen[atom] = struct{}{}
		atoms = append(atoms, atom)
	}

	for _, port := range toIntList(portsRaw) {
		if port < 0 {
			continue
		}
		add(fmt.Sprintf("%d", port))
	}
	for _, portRange := range toStringSlice(rangesRaw) {
		add(portRange)
	}

	return atoms
}

func buildIntegerMatcherAtoms(matcher string, raw interface{}) []string {
	values := toIntList(raw)
	if len(values) == 0 {
		return nil
	}
	atoms := make([]string, 0, len(values))
	seen := map[int]struct{}{}
	for _, value := range values {
		if value < 0 {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		atoms = append(atoms, fmt.Sprintf("%s,%d", matcher, value))
	}
	return atoms
}

func toIntList(raw interface{}) []int {
	switch value := raw.(type) {
	case []int:
		return append([]int{}, value...)
	case []string:
		result := make([]int, 0, len(value))
		for _, item := range value {
			if intValue, ok := toInt(item); ok {
				result = append(result, intValue)
			}
		}
		return result
	case []int32:
		result := make([]int, 0, len(value))
		for _, item := range value {
			result = append(result, int(item))
		}
		return result
	case []int64:
		result := make([]int, 0, len(value))
		for _, item := range value {
			result = append(result, int(item))
		}
		return result
	case []float64:
		result := make([]int, 0, len(value))
		for _, item := range value {
			result = append(result, int(item))
		}
		return result
	case []interface{}:
		result := make([]int, 0, len(value))
		for _, item := range value {
			if intValue, ok := toInt(item); ok {
				result = append(result, intValue)
			}
		}
		return result
	default:
		if intValue, ok := toInt(raw); ok {
			return []int{intValue}
		}
		return nil
	}
}

func normalizeMihomoRuleSetRefs(raw interface{}) []string {
	values := toStringSlice(raw)
	if len(values) == 0 {
		return nil
	}

	normalized := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		normalized = append(normalized, value)
	}
	return normalized
}

func collectMissingMihomoRuleProviderTags(values []string, providerTags map[string]struct{}) []string {
	if len(values) == 0 {
		return nil
	}
	if providerTags == nil {
		providerTags = map[string]struct{}{}
	}

	missing := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := providerTags[value]; exists {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		missing = append(missing, value)
	}
	return missing
}

func buildRuleSetAtoms(values []string, rawMatchSource interface{}) []string {
	if len(values) == 0 {
		return nil
	}

	matchSource, _ := toBool(rawMatchSource)
	atoms := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		atom := "RULE-SET," + value
		if matchSource {
			atom += ",src"
		}
		atoms = append(atoms, atom)
	}
	return atoms
}

func hasMihomoTargetIPRuleSetMatcher(values []string, ipRuleProviderTags map[string]struct{}) bool {
	if len(values) == 0 || len(ipRuleProviderTags) == 0 {
		return false
	}
	for _, value := range values {
		tag := strings.TrimSpace(value)
		if tag == "" {
			continue
		}
		if _, exists := ipRuleProviderTags[tag]; exists {
			return true
		}
	}
	return false
}

func expandMihomoMatcherGroups(groups [][]string) [][]string {
	if len(groups) == 0 {
		return nil
	}

	combinations := [][]string{{}}
	for _, group := range groups {
		if len(group) == 0 {
			continue
		}
		next := make([][]string, 0, len(combinations)*len(group))
		for _, combination := range combinations {
			for _, atom := range group {
				cloned := append([]string{}, combination...)
				cloned = append(cloned, atom)
				next = append(next, cloned)
			}
		}
		combinations = next
	}
	return combinations
}

func normalizeMihomoInboundRules(raw interface{}, aliasMap map[string]string) []string {
	values := toStringSlice(raw)
	if len(values) == 0 {
		return nil
	}

	normalized := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if alias, ok := aliasMap[value]; ok && alias != "" {
			value = alias
		}
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		normalized = append(normalized, value)
	}
	return normalized
}

func normalizeMihomoRuleProviderBehavior(rawBehavior string, format string) string {
	behavior := strings.ToLower(strings.TrimSpace(rawBehavior))
	if format == "mrs" {
		switch behavior {
		case "domain", "ipcidr":
			return behavior
		default:
			return "domain"
		}
	}

	switch behavior {
	case "domain", "ipcidr", "classical":
		return behavior
	default:
		return "classical"
	}
}

func buildMihomoRuleProviders(raw interface{}, targets *mihomoProxyConversionResult) (map[string]interface{}, map[string]struct{}) {
	providers := map[string]interface{}{}
	tags := map[string]struct{}{}

	items, ok := raw.([]interface{})
	if !ok || len(items) == 0 {
		return nil, tags
	}

	for _, item := range items {
		entry, ok := item.(map[string]interface{})
		if !ok || entry == nil {
			continue
		}

		tag := strings.TrimSpace(firstString(entry["tag"]))
		if tag == "" {
			continue
		}

		provider := map[string]interface{}{}

		entryType := strings.ToLower(strings.TrimSpace(firstString(entry["type"])))
		switch entryType {
		case "local", "file":
			provider["type"] = "file"
			path := strings.TrimSpace(firstString(entry["path"]))
			if path == "" {
				continue
			}
			provider["path"] = path
		case "remote", "http":
			provider["type"] = "http"
			url := strings.TrimSpace(firstString(entry["url"]))
			if url == "" {
				continue
			}
			provider["url"] = url
			if interval, ok := durationToSeconds(entry["update_interval"]); ok && interval > 0 {
				provider["interval"] = interval
			}
			proxySource := firstString(entry["proxy"])
			if proxySource == "" {
				proxySource = firstString(entry["download_detour"])
			}
			if proxy, ok := normalizeMihomoRouteTarget(proxySource, targets); ok {
				provider["proxy"] = proxy
			}
		case "inline":
			provider["type"] = "inline"
			payload := normalizeMihomoInlineRuleProviderPayload(entry["payload"])
			if len(payload) == 0 {
				continue
			}
			provider["payload"] = payload
			provider["behavior"] = normalizeMihomoRuleProviderBehavior(firstString(entry["behavior"]), "")
			providers[tag] = provider
			tags[tag] = struct{}{}
			continue
		default:
			continue
		}

		format := strings.ToLower(strings.TrimSpace(firstString(entry["format"])))
		switch format {
		case "binary":
			format = "mrs"
		case "source":
			format = "yaml"
		}
		if format == "" {
			rawBehavior := strings.ToLower(strings.TrimSpace(firstString(entry["behavior"])))
			behavior := normalizeMihomoRuleProviderBehavior(rawBehavior, "")
			if behavior == "classical" {
				format = "yaml"
			} else {
				format = "text"
			}
		}

		behavior := normalizeMihomoRuleProviderBehavior(firstString(entry["behavior"]), format)
		provider["behavior"] = behavior
		provider["format"] = format

		providers[tag] = provider
		tags[tag] = struct{}{}
	}

	if len(providers) == 0 {
		return nil, tags
	}
	return providers, tags
}

func collectMihomoIPRuleProviderTags(providers map[string]interface{}) map[string]struct{} {
	if len(providers) == 0 {
		return nil
	}

	tags := map[string]struct{}{}
	for tag, rawProvider := range providers {
		provider, ok := rawProvider.(map[string]interface{})
		if !ok || provider == nil {
			continue
		}
		behavior := strings.ToLower(strings.TrimSpace(firstString(provider["behavior"])))
		if behavior != "ipcidr" {
			continue
		}
		normalizedTag := strings.TrimSpace(tag)
		if normalizedTag == "" {
			continue
		}
		tags[normalizedTag] = struct{}{}
	}

	if len(tags) == 0 {
		return nil
	}
	return tags
}

func normalizeMihomoInlineRuleProviderPayload(raw interface{}) []string {
	values := toStringSlice(raw)
	if len(values) == 0 {
		return nil
	}

	payload := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		payload = append(payload, value)
	}
	return payload
}

func buildMihomoInboundAliasMap(inbounds []model.MihomoInbound) map[string]string {
	aliasMap := make(map[string]string)
	for _, inbound := range inbounds {
		effectiveTag := deriveEffectiveMihomoInboundRouteTagFromRaw(inbound.Tag, inbound.Type, inbound.Options)
		if effectiveTag != "" && effectiveTag != inbound.Tag {
			aliasMap[inbound.Tag] = effectiveTag
		}
	}
	return aliasMap
}

func marshalJSONMap(raw []byte) (map[string]interface{}, error) {
	var value map[string]interface{}
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, err
	}
	if value == nil {
		return map[string]interface{}{}, nil
	}
	return value, nil
}

func buildMihomoListener(inbound model.MihomoInbound, payload map[string]interface{}, ref mihomoInboundRouteRef) map[string]interface{} {
	mergeMihomoInboundOutJSONListenerCompat(payload, inbound)

	listener := map[string]interface{}{}
	for key, value := range payload {
		switch key {
		case "tag":
			continue
		case "listen_port":
			listener["port"] = value
		case "detour":
			continue
		default:
			listener[key] = value
		}
	}

	name := strings.TrimSpace(inbound.Tag)
	if name == "" {
		name = ref.RuleName
	}
	if name != "" {
		listener["name"] = name
	}
	if ref.ProxyTarget != "" {
		listener["proxy"] = ref.ProxyTarget
	}
	if ref.RuleName != "" {
		// Listener rule names select a same-named entry in top-level sub-rules.
		listener["rule"] = ref.RuleName
	}
	normalizeMihomoListenerPayload(listener)
	return listener
}

func mergeMihomoInboundOutJSONListenerCompat(payload map[string]interface{}, inbound model.MihomoInbound) {
	if payload == nil {
		return
	}

	switch strings.ToLower(strings.TrimSpace(inbound.Type)) {
	case "hysteria2":
		mergeMihomoHysteria2OutJSONListenerCompat(payload, inbound.OutJson)
	}
}

func mergeMihomoHysteria2OutJSONListenerCompat(payload map[string]interface{}, raw json.RawMessage) {
	if payload == nil || len(raw) == 0 {
		return
	}
	if _, exists := payload["mihomo_hy2"]; exists {
		return
	}

	var outbound map[string]interface{}
	if err := json.Unmarshal(raw, &outbound); err != nil || outbound == nil {
		return
	}

	mihomoHY2, ok := outbound["mihomo_hy2"].(map[string]interface{})
	if !ok || len(mihomoHY2) == 0 {
		return
	}

	payload["mihomo_hy2"] = cloneMihomoListenerCompatMap(mihomoHY2)
}

func cloneMihomoListenerCompatMap(src map[string]interface{}) map[string]interface{} {
	if src == nil {
		return nil
	}

	dst := make(map[string]interface{}, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func appendMihomoTerminalMatch(rules []string, target string) []string {
	target = strings.TrimSpace(target)
	if target == "" {
		return rules
	}
	if len(rules) > 0 {
		lastRule := strings.ToUpper(strings.TrimSpace(rules[len(rules)-1]))
		if strings.HasPrefix(lastRule, "MATCH,") {
			return rules
		}
	}
	return append(rules, "MATCH,"+target)
}

func normalizeMihomoListenerPayload(listener map[string]interface{}) {
	if listener == nil {
		return
	}

	listenerType := strings.ToLower(strings.TrimSpace(firstString(listener["type"])))
	if listenerType == "redirect" {
		listener["type"] = "redir"
		listenerType = "redir"
	}

	normalizeMihomoListenerCompatFields(listener, listenerType)

	tlsMap, ok := listener["tls"].(map[string]interface{})
	if !ok || tlsMap == nil {
		return
	}

	if value := normalizeMihomoListenerTLSValue(tlsMap["certificate"], tlsMap["certificate_path"]); value != "" {
		listener["certificate"] = value
	}
	if value := normalizeMihomoListenerTLSValue(tlsMap["key"], tlsMap["key_path"]); value != "" {
		listener["private-key"] = value
	}
	if value := strings.TrimSpace(firstString(tlsMap["client_authentication"])); value != "" {
		listener["client-auth-type"] = value
	}
	if value := normalizeMihomoListenerTLSValue(tlsMap["client_certificate"], tlsMap["client_certificate_path"]); value != "" {
		listener["client-auth-cert"] = value
	}
	if value := extractMihomoListenerECHKey(tlsMap); value != "" {
		listener["ech-key"] = value
	}
	if realityConfig := buildMihomoListenerRealityConfig(tlsMap); len(realityConfig) > 0 {
		listener["reality-config"] = realityConfig
	}
	if (listenerType == "hysteria2" || listenerType == "tuic") && listener["alpn"] == nil {
		if alpn := toStringSlice(tlsMap["alpn"]); len(alpn) > 0 {
			listener["alpn"] = alpn
		}
	}

	delete(listener, "tls")
}

func normalizeMihomoListenerTLSValue(content interface{}, path interface{}) string {
	if filePath := strings.TrimSpace(firstString(path)); filePath != "" {
		return filePath
	}
	if lines := toStringSlice(content); len(lines) > 0 {
		return strings.Join(lines, "\n")
	}
	return strings.TrimSpace(firstString(content))
}

func extractMihomoListenerECHKey(tlsMap map[string]interface{}) string {
	echMap, ok := tlsMap["ech"].(map[string]interface{})
	if !ok || echMap == nil {
		return ""
	}
	if enabled, ok := toBool(echMap["enabled"]); ok && !enabled {
		return ""
	}
	return normalizeMihomoListenerTLSValue(echMap["key"], echMap["key_path"])
}

func buildMihomoListenerRealityConfig(tlsMap map[string]interface{}) map[string]interface{} {
	realityMap, ok := tlsMap["reality"].(map[string]interface{})
	if !ok || realityMap == nil {
		return nil
	}
	if enabled, ok := toBool(realityMap["enabled"]); ok && !enabled {
		return nil
	}

	realityConfig := map[string]interface{}{}

	handshake, _ := realityMap["handshake"].(map[string]interface{})
	handshakeServer := strings.TrimSpace(firstString(handshake["server"]))
	handshakePort, _ := toInt(handshake["server_port"])
	if handshakeServer != "" && handshakePort > 0 {
		realityConfig["dest"] = fmt.Sprintf("%s:%d", handshakeServer, handshakePort)
	}

	if privateKey := strings.TrimSpace(firstString(realityMap["private_key"])); privateKey != "" {
		realityConfig["private-key"] = privateKey
	}
	if shortIDs := toStringSlice(realityMap["short_id"]); len(shortIDs) > 0 {
		realityConfig["short-id"] = shortIDs
	}

	serverName := strings.TrimSpace(firstString(tlsMap["server_name"]))
	if serverName == "" {
		serverName = handshakeServer
	}
	if serverName != "" {
		realityConfig["server-names"] = []string{serverName}
	}

	if maxTimeDifference, ok := durationToMicroseconds(realityMap["max_time_difference"]); ok && maxTimeDifference > 0 {
		realityConfig["max-time-difference"] = maxTimeDifference
	}

	if len(realityConfig) == 0 {
		return nil
	}
	return realityConfig
}

func buildMihomoInboundRouteRef(inbound model.MihomoInbound, targets *mihomoProxyConversionResult, globalFinal string) (mihomoInboundRouteRef, error) {
	ref := mihomoInboundRouteRef{
		DefaultTarget: globalFinal,
	}

	var options map[string]interface{}
	if len(inbound.Options) > 0 && json.Unmarshal(inbound.Options, &options) == nil {
		if detour := extractDetourFromOptions(options); detour != "" {
			if target, ok := normalizeMihomoRouteTarget(detour, targets); ok {
				ref.ProxyTarget = target
			} else {
				return mihomoInboundRouteRef{}, fmt.Errorf("listener %s detour references unknown outbound %q", strings.TrimSpace(inbound.Tag), strings.TrimSpace(detour))
			}
		}
	}
	if ref.ProxyTarget == "" {
		ref.RuleName = deriveEffectiveMihomoInboundRouteTagFromRaw(inbound.Tag, inbound.Type, inbound.Options)
	}
	return ref, nil
}

func filterSupportedMihomoListeners(inbounds []model.MihomoInbound) []model.MihomoInbound {
	supported := make([]model.MihomoInbound, 0, len(inbounds))
	for _, inbound := range inbounds {
		switch inbound.Type {
		case "mixed", "socks", "http", "redirect", "tproxy", "tun", "snell", "shadowsocks", "shadowtls", "vmess", "vless", "trojan", "anytls", "tuic", "hysteria2", "mieru", "sudoku", "trusttunnel":
			supported = append(supported, inbound)
		default:
			logger.Warning("skip unsupported mihomo listener type: ", inbound.Type, " tag=", inbound.Tag)
		}
	}
	return supported
}

func normalizeMihomoSniffer(raw interface{}) map[string]interface{} {
	value, ok := raw.(map[string]interface{})
	if !ok || value == nil {
		return nil
	}

	enabled, _ := toBool(value["enable"])
	if !enabled {
		return nil
	}

	forceDNSMapping, ok := toBool(value["force-dns-mapping"])
	if !ok {
		forceDNSMapping = true
	}
	overrideDestination, ok := toBool(value["override-destination"])
	if !ok {
		overrideDestination = false
	}
	parsePureIP, ok := toBool(value["parse-pure-ip"])
	if !ok {
		parsePureIP = true
	}

	sniffer := map[string]interface{}{
		"enable":               true,
		"force-dns-mapping":    forceDNSMapping,
		"override-destination": overrideDestination,
		"parse-pure-ip":        parsePureIP,
	}

	sniffMap, _ := value["sniff"].(map[string]interface{})
	normalizedSniff := map[string]interface{}{}
	for _, protocol := range []string{"HTTP", "TLS", "QUIC"} {
		ports := []string{"1-65535"}
		if sniffMap != nil {
			if entry, ok := sniffMap[protocol].(map[string]interface{}); ok {
				if customPorts := toStringSlice(entry["ports"]); len(customPorts) > 0 {
					ports = customPorts
				}
			}
		}
		normalizedSniff[protocol] = map[string]interface{}{"ports": ports}
	}
	sniffer["sniff"] = normalizedSniff

	return sniffer
}

func copyMihomoGeneralConfig(base map[string]interface{}) map[string]interface{} {
	result := map[string]interface{}{}
	for key, value := range base {
		switch key {
		case "log", "route", "inbounds", "outbounds", "services", "endpoints", "certificate", "experimental", "ntp", "dns":
			continue
		default:
			result[key] = value
		}
	}

	if logMap, ok := base["log"].(map[string]interface{}); ok {
		if level, ok := logMap["level"].(string); ok && strings.TrimSpace(level) != "" {
			result["log-level"] = strings.TrimSpace(level)
		}
	}

	return result
}

func applyMihomoRouteGeneralConfig(document map[string]interface{}, route map[string]interface{}) {
	if document == nil || route == nil {
		return
	}

	document["mode"] = "rule"

	if value := strings.TrimSpace(firstString(route["default_interface"])); value != "" {
		document["interface-name"] = value
	}
	if value, ok := toInt(route["default_mark"]); ok && value > 0 {
		document["routing-mark"] = value
	}
	if value, ok := toBool(route["auto_detect_interface"]); ok {
		document["auto-detect-interface"] = value
	}
}

func normalizeMihomoDocument(document map[string]interface{}) map[string]interface{} {
	normalized, ok := normalizeNumericTypesForYAML(document).(map[string]interface{})
	if !ok || normalized == nil {
		return document
	}
	return normalized
}

func requireRouteMap(base map[string]interface{}) map[string]interface{} {
	raw, ok := base["route"].(map[string]interface{})
	if !ok || raw == nil {
		return map[string]interface{}{}
	}
	return raw
}
