package service

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/util/common"
	"gorm.io/gorm"
)

const (
	SingboxRouteMaxBytes           = 1 * 1024 * 1024
	SingboxRouteMaxRules           = 512
	SingboxRouteMaxRuleSets        = 256
	SingboxRouteMaxRuleBytes       = 32 * 1024
	SingboxRouteMaxRulesBytes      = 512 * 1024
	SingboxRouteMaxRuleSetBytes    = 32 * 1024
	SingboxRouteMaxLogicalDepth    = 6
	SingboxRouteMaxLogicalChildren = 32
	SingboxRouteMaxValueDepth      = 16
	SingboxRouteMaxArrayItems      = 512
	SingboxRouteMaxObjectEntries   = 128
	SingboxRouteMaxStringBytes     = 4096
)

type SingboxRouteRevisionConflictError struct {
	CurrentRevision uint64
}

func (e *SingboxRouteRevisionConflictError) Error() string {
	return "sing-box route revision conflict"
}

type SingboxRouteEditorContext struct {
	Revision     uint64          `json:"revision"`
	Route        json.RawMessage `json:"route"`
	InboundTags  []string        `json:"inboundTags"`
	OutboundTags []string        `json:"outboundTags"`
	ClientNames  []string        `json:"clientNames"`
}

type SingboxRouteSaveRequest struct {
	ExpectedRevision uint64          `json:"expectedRevision"`
	Route            json.RawMessage `json:"route"`
	RetryRuntime     bool            `json:"retryRuntime,omitempty"`
}

type SingboxRouteSaveResult struct {
	Revision uint64          `json:"revision"`
	Route    json.RawMessage `json:"route"`
	Changed  bool            `json:"changed"`
}

var regenerateSingboxRouteRuntimeConfig = func(configService *ConfigService) error {
	return GetProManagerService(configService).RegenerateCoreConfig()
}

func (s *ConfigService) GetSingboxRouteEditorContext() (*SingboxRouteEditorContext, error) {
	db := database.GetDB()
	if db == nil {
		return nil, common.NewError("database is not ready")
	}
	context := &SingboxRouteEditorContext{}
	err := db.Transaction(func(tx *gorm.DB) error {
		revision, err := ensureSingboxConfigRevisionState(tx)
		if err != nil {
			return err
		}
		config, err := loadSingboxConfigForDNS(tx)
		if err != nil {
			return err
		}
		routeRaw, err := extractSingboxRoute(config)
		if err != nil {
			return err
		}
		inboundTags, err := loadSingboxDNSInboundTags(tx)
		if err != nil {
			return err
		}
		outboundTags, err := loadSingboxRouteOutboundTags(tx)
		if err != nil {
			return err
		}
		clientNames := []string{}
		if err := tx.Model(&model.Client{}).Order("id ASC").Pluck("name", &clientNames).Error; err != nil {
			return err
		}
		context.Revision = revision
		context.Route = routeRaw
		context.InboundTags = compactUniqueStrings(inboundTags)
		context.OutboundTags = compactUniqueStrings(outboundTags)
		context.ClientNames = compactUniqueStrings(clientNames)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return context, nil
}

func (s *ConfigService) SaveSingboxRoute(request SingboxRouteSaveRequest, actor string) (*SingboxRouteSaveResult, error) {
	retryOnly := request.RetryRuntime && len(request.Route) == 0
	if request.ExpectedRevision == 0 && !retryOnly {
		return nil, common.NewError("expectedRevision is required")
	}
	if len(request.Route) == 0 && !request.RetryRuntime {
		return nil, common.NewError("route is required")
	}
	db := database.GetDB()
	if db == nil {
		return nil, common.NewError("database is not ready")
	}
	result := &SingboxRouteSaveResult{}
	err := db.Transaction(func(tx *gorm.DB) error {
		currentRevision, err := ensureSingboxConfigRevisionState(tx)
		if err != nil {
			return err
		}
		if !retryOnly && currentRevision != request.ExpectedRevision {
			return &SingboxRouteRevisionConflictError{CurrentRevision: currentRevision}
		}
		if request.RetryRuntime && len(request.Route) == 0 {
			currentConfig, err := loadSingboxConfigForDNS(tx)
			if err != nil {
				return err
			}
			routeRaw, err := extractSingboxRoute(currentConfig)
			if err != nil {
				return err
			}
			result.Revision = currentRevision
			result.Route = routeRaw
			return nil
		}
		currentConfig, err := loadSingboxConfigForDNS(tx)
		if err != nil {
			return err
		}
		route, err := normalizeSingboxRouteInboundTags(request.Route, tx)
		if err != nil {
			return err
		}
		route, err = normalizeAndValidateSingboxRoute(route, tx)
		if err != nil {
			return err
		}
		oldRoute, err := extractSingboxRoute(currentConfig)
		if err != nil {
			return err
		}
		if bytes.Equal(oldRoute, route) {
			result.Revision = currentRevision
			result.Route = route
			return nil
		}
		updated, err := replaceSingboxRoute(currentConfig, route)
		if err != nil {
			return err
		}
		if err := (&SettingService{}).SaveConfig(tx, updated); err != nil {
			return err
		}
		newRevision, err := bumpSingboxConfigRevision(tx, currentRevision)
		if err != nil {
			return err
		}
		result.Revision = newRevision
		result.Route = route
		result.Changed = true
		return recordChange(tx, model.Changes{
			DateTime: time.Now().Unix(),
			Actor:    actor,
			Key:      "singbox_route",
			Action:   "set",
			Obj:      buildSingboxRouteChangeAudit(route),
		})
	})
	if err != nil {
		return nil, err
	}
	if result.Changed {
		markLastUpdate(time.Now().Unix())
	}
	if result.Changed || request.RetryRuntime {
		if err := regenerateSingboxRouteRuntimeConfig(s); err != nil {
			return result, &CommittedSaveError{Err: fmt.Errorf("regenerate sing-box config after route save failed: %w", err)}
		}
	}
	return result, nil
}

func extractSingboxRoute(config json.RawMessage) (json.RawMessage, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(config, &root); err != nil {
		return nil, err
	}
	if raw, ok := root["route"]; ok && len(raw) > 0 && string(raw) != "null" {
		var route map[string]any
		if err := json.Unmarshal(raw, &route); err != nil {
			return nil, err
		}
		if route != nil {
			return json.Marshal(route)
		}
	}
	return json.RawMessage(`{"rules":[],"rule_set":[]}`), nil
}

func replaceSingboxRoute(config, route json.RawMessage) (json.RawMessage, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(config, &root); err != nil {
		return nil, err
	}
	root["route"] = append(json.RawMessage(nil), route...)
	return json.Marshal(root)
}

func normalizeSingboxRouteInboundTags(raw json.RawMessage, tx *gorm.DB) (json.RawMessage, error) {
	aliasMap, err := buildInboundTagAliasMap(tx)
	if err != nil {
		return nil, err
	}
	if len(aliasMap) == 0 {
		return append(json.RawMessage(nil), raw...), nil
	}
	var route map[string]any
	if err := json.Unmarshal(raw, &route); err != nil {
		return nil, err
	}
	if route == nil {
		return nil, common.NewError("sing-box route must be an object")
	}
	if rules, ok := route["rules"].([]any); ok {
		normalizeRuleListInboundTags(rules, aliasMap)
	}
	return json.Marshal(route)
}

func loadSingboxRouteOutboundTags(tx *gorm.DB) ([]string, error) {
	tags := make([]string, 0)
	add := func(tag string) {
		tag = strings.TrimSpace(tag)
		if tag != "" {
			tags = append(tags, tag)
		}
	}
	var outbounds []model.Outbound
	if err := tx.Model(&model.Outbound{}).
		Select("type", "tag", "options", "raw_outbound").
		Order("id ASC").Find(&outbounds).Error; err != nil {
		return nil, err
	}
	for index := range outbounds {
		runtimeTags, err := singboxRuntimeOutboundTags(&outbounds[index])
		if err != nil {
			return nil, fmt.Errorf("decode sing-box outbound %q runtime tags: %w", outbounds[index].Tag, err)
		}
		for _, tag := range runtimeTags {
			add(tag)
		}
	}
	var endpoints []string
	if err := tx.Model(&model.Endpoint{}).Order("id ASC").Pluck("tag", &endpoints).Error; err != nil {
		return nil, err
	}
	for _, endpoint := range endpoints {
		add(endpoint)
	}

	return compactUniqueStrings(tags), nil
}

// validateSingboxConfigRouteBounds protects every default sing-box config
// write, including retained compatibility callers of the generic config API.
// The focused route API still owns the CAS write path; this is a validation
// backstop, not a second route persistence contract.
func validateSingboxConfigRouteBounds(config json.RawMessage, tx *gorm.DB) error {
	if len(config) == 0 {
		return nil
	}
	root := map[string]json.RawMessage{}
	if err := json.Unmarshal(config, &root); err != nil {
		return err
	}
	rawRoute, ok := root["route"]
	if !ok || len(rawRoute) == 0 || bytes.Equal(bytes.TrimSpace(rawRoute), []byte("null")) {
		if err := validateSingboxConfiguredInboundReferences(tx, config); err != nil {
			return err
		}
		return validateSingboxConfigBasicsReferences(config, tx)
	}
	routeRaw, err := extractSingboxRoute(config)
	if err != nil {
		return err
	}
	route := map[string]any{}
	if err := json.Unmarshal(routeRaw, &route); err != nil {
		return err
	}
	// Older exports can keep rule_set at the top level until the runtime
	// generator moves it into route.rule_set. Validate that form too.
	if _, hasRuleSets := route["rule_set"]; !hasRuleSets {
		if legacyRuleSets, exists := root["rule_set"]; exists {
			var legacyValue any
			if err := json.Unmarshal(legacyRuleSets, &legacyValue); err != nil {
				return err
			}
			route["rule_set"] = legacyValue
		}
	}
	rawRoute, err = json.Marshal(route)
	if err != nil {
		return err
	}
	if _, err = normalizeAndValidateSingboxRoute(rawRoute, tx); err != nil {
		return err
	}
	if err := validateSingboxConfiguredInboundReferences(tx, config); err != nil {
		return err
	}
	return validateSingboxConfigBasicsReferences(config, tx)
}

func normalizeAndValidateSingboxRoute(raw json.RawMessage, db *gorm.DB) (json.RawMessage, error) {
	if len(raw) > SingboxRouteMaxBytes {
		return nil, common.NewErrorf("sing-box route exceeds %d bytes", SingboxRouteMaxBytes)
	}
	if !utf8.Valid(raw) {
		return nil, common.NewError("sing-box route is not valid UTF-8")
	}
	var route map[string]any
	if err := json.Unmarshal(raw, &route); err != nil || route == nil {
		if err == nil {
			err = errors.New("route must be an object")
		}
		return nil, common.NewErrorf("invalid sing-box route: %v", err)
	}
	if err := validateSingboxRouteValue(route, 0); err != nil {
		return nil, err
	}
	if err := validateSingboxDNSResolverFields(db, route, "sing-box route"); err != nil {
		return nil, err
	}
	if err := validateSingboxRouteNumericFields(route); err != nil {
		return nil, err
	}
	targets, err := loadSingboxRouteOutboundTags(db)
	if err != nil {
		return nil, err
	}
	targetSet := make(map[string]struct{}, len(targets))
	for _, target := range targets {
		targetSet[target] = struct{}{}
	}
	inboundTargets, err := loadSingboxRuntimeInboundReferenceTags(db)
	if err != nil {
		return nil, err
	}
	inboundSet := make(map[string]struct{}, len(inboundTargets))
	for _, target := range inboundTargets {
		inboundSet[target] = struct{}{}
	}
	final, finalExists, err := readSingboxRouteString(route, "final")
	if err != nil {
		return nil, common.NewError("sing-box route.final must be a string")
	}
	if finalExists && final != "" {
		if _, ok := targetSet[final]; !ok {
			return nil, common.NewErrorf("route.final references unknown outbound %q", final)
		}
	}
	ruleSets, err := validateSingboxRouteRuleSets(route["rule_set"], targetSet)
	if err != nil {
		return nil, err
	}
	budget := &singboxRouteRuleBudget{}
	if err := validateSingboxRouteRules(route["rules"], ruleSets, targetSet, inboundSet, 1, budget); err != nil {
		return nil, err
	}
	return json.Marshal(route)
}

func validateSingboxRouteNumericFields(route map[string]any) error {
	if raw, exists := route["default_mark"]; exists && raw != nil {
		value, ok := strictSingboxRouteInteger(raw)
		if !ok || value < 0 {
			return common.NewError("sing-box route.default_mark must be a non-negative integer")
		}
		route["default_mark"] = value
	}
	return validateSingboxRouteRulesNumericFields(route["rules"])
}

func validateSingboxRouteRulesNumericFields(raw any) error {
	items, ok := raw.([]any)
	if !ok {
		return nil
	}
	for _, item := range items {
		rule, ok := item.(map[string]any)
		if !ok || rule == nil {
			continue
		}
		if rawVersion, exists := rule["ip_version"]; exists && rawVersion != nil {
			value, ok := strictSingboxRouteInteger(rawVersion)
			if !ok || (value != 4 && value != 6) {
				return common.NewError("sing-box route rule ip_version must be 4 or 6")
			}
			rule["ip_version"] = value
		}
		for _, field := range []string{"port", "source_port"} {
			if err := validateSingboxRouteIntegerList(rule[field], field, 1, 65535); err != nil {
				return err
			}
		}
		if err := validateSingboxRouteIntegerList(rule["user_id"], "user_id", 0, singboxRouteMaxJSONInteger); err != nil {
			return err
		}
		for _, field := range []string{"port_range", "source_port_range"} {
			if err := validateSingboxRoutePortRanges(rule[field], field); err != nil {
				return err
			}
		}
		if rawPort, exists := rule["override_port"]; exists && rawPort != nil {
			value, ok := strictSingboxRouteInteger(rawPort)
			if !ok || value < 1 || value > 65535 {
				return common.NewError("sing-box route rule override_port must be an integer from 1 to 65535")
			}
			rule["override_port"] = value
		}
		if err := validateSingboxRouteRulesNumericFields(rule["rules"]); err != nil {
			return err
		}
	}
	return nil
}

const singboxRouteMaxJSONInteger int64 = 1<<53 - 1

func validateSingboxRouteIntegerList(value any, field string, min, max int64) error {
	if value == nil {
		return nil
	}
	values := []any{value}
	if list, ok := value.([]any); ok {
		values = list
	}
	for index, item := range values {
		parsed, ok := strictSingboxRouteInteger(item)
		if !ok || parsed < min || parsed > max {
			return common.NewErrorf("sing-box route rule %s[%d] must be an integer from %d to %d", field, index+1, min, max)
		}
	}
	return nil
}

func validateSingboxRoutePortRanges(value any, field string) error {
	if value == nil {
		return nil
	}
	values := []any{value}
	if list, ok := value.([]any); ok {
		values = list
	}
	for index, item := range values {
		text, ok := item.(string)
		if !ok {
			return common.NewErrorf("sing-box route rule %s[%d] must be a port range", field, index+1)
		}
		text = strings.TrimSpace(text)
		separator := ":"
		if !strings.Contains(text, separator) {
			separator = "-"
		}
		parts := strings.Split(text, separator)
		if len(parts) != 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
			return common.NewErrorf("sing-box route rule %s[%d] must use start:end", field, index+1)
		}
		start, startErr := parseSingboxRoutePort(parts[0])
		end, endErr := parseSingboxRoutePort(parts[1])
		if startErr != nil || endErr != nil || start > end {
			return common.NewErrorf("sing-box route rule %s[%d] must be between 1 and 65535", field, index+1)
		}
	}
	return nil
}

func parseSingboxRoutePort(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, errors.New("empty port")
	}
	var parsed int64
	for _, char := range value {
		if char < '0' || char > '9' {
			return 0, errors.New("invalid port")
		}
		if parsed > 65535/10 {
			return 0, errors.New("port overflow")
		}
		parsed = parsed*10 + int64(char-'0')
	}
	if parsed < 1 || parsed > 65535 {
		return 0, errors.New("port out of range")
	}
	return parsed, nil
}

func strictSingboxRouteInteger(value any) (int64, bool) {
	switch typed := value.(type) {
	case float64:
		if math.IsNaN(typed) || math.IsInf(typed, 0) || typed != math.Trunc(typed) || typed < 0 || typed > float64(singboxRouteMaxJSONInteger) {
			return 0, false
		}
		return int64(typed), true
	case int:
		return int64(typed), typed >= 0 && int64(typed) <= singboxRouteMaxJSONInteger
	case int64:
		return typed, typed >= 0 && typed <= singboxRouteMaxJSONInteger
	case uint64:
		if typed > uint64(singboxRouteMaxJSONInteger) {
			return 0, false
		}
		return int64(typed), true
	default:
		return 0, false
	}
}

func validateSingboxRouteRuleSets(raw any, targets map[string]struct{}) (map[string]struct{}, error) {
	result := map[string]struct{}{}
	if raw == nil {
		return result, nil
	}
	items, ok := raw.([]any)
	if !ok || len(items) > SingboxRouteMaxRuleSets {
		return nil, common.NewErrorf("sing-box route rule_set must contain at most %d items", SingboxRouteMaxRuleSets)
	}
	total := 0
	for index, item := range items {
		value, ok := item.(map[string]any)
		if !ok || value == nil {
			return nil, common.NewErrorf("sing-box rule_set #%d is invalid", index+1)
		}
		payload, _ := json.Marshal(value)
		if len(payload) > SingboxRouteMaxRuleSetBytes {
			return nil, common.NewErrorf("sing-box rule_set #%d exceeds %d bytes", index+1, SingboxRouteMaxRuleSetBytes)
		}
		total += len(payload)
		tag := strings.TrimSpace(firstString(value["tag"]))
		if tag == "" || !utf8.ValidString(tag) {
			return nil, common.NewErrorf("sing-box rule_set #%d has an invalid tag", index+1)
		}
		if _, exists := result[tag]; exists {
			return nil, common.NewErrorf("sing-box rule_set tag %q is duplicated", tag)
		}
		if err := validateSingboxRouteRuleSet(value, index, targets); err != nil {
			return nil, err
		}
		result[tag] = struct{}{}
	}
	if total > SingboxRouteMaxRulesBytes {
		return nil, common.NewErrorf("sing-box route rule_set data exceeds %d bytes", SingboxRouteMaxRulesBytes)
	}
	return result, nil
}

func validateSingboxRouteRuleSet(value map[string]any, index int, targets map[string]struct{}) error {
	prefix := fmt.Sprintf("sing-box rule_set #%d", index+1)
	typeValue, typeExists, err := readSingboxRouteString(value, "type")
	if err != nil {
		return common.NewErrorf("%s has an invalid type", prefix)
	}
	// Older stored routes may keep only a rule-set tag and let the runtime
	// generator or Core-side defaults supply the remaining fields.
	if !typeExists {
		return nil
	}
	if typeValue == "" {
		return common.NewErrorf("%s has an invalid type", prefix)
	}

	format, formatExists, err := readSingboxRouteString(value, "format")
	if err != nil {
		return common.NewErrorf("%s has an invalid format", prefix)
	}
	if formatExists && format != "" && format != "source" && format != "binary" {
		return common.NewErrorf("%s has an unsupported format %q", prefix, format)
	}

	path, pathExists, err := readSingboxRouteString(value, "path")
	if err != nil {
		return common.NewErrorf("%s has an invalid path", prefix)
	}
	urlValue, urlExists, err := readSingboxRouteString(value, "url")
	if err != nil {
		return common.NewErrorf("%s has an invalid URL", prefix)
	}
	detour, detourExists, err := readSingboxRouteString(value, "download_detour")
	if err != nil {
		return common.NewErrorf("%s has an invalid download_detour", prefix)
	}
	httpClientDetour, httpClientDetourExists, err := readSingboxRouteHTTPClientDetour(value)
	if err != nil {
		return common.NewErrorf("%s has an invalid http_client", prefix)
	}
	if detourExists && httpClientDetourExists && detour != "" && httpClientDetour != "" && detour != httpClientDetour {
		return common.NewErrorf("%s contains conflicting download_detour and http_client.detour", prefix)
	}
	effectiveDetour := detour
	if httpClientDetour != "" {
		effectiveDetour = httpClientDetour
	}
	interval, intervalExists, err := readSingboxRouteString(value, "update_interval")
	if err != nil {
		return common.NewErrorf("%s has an invalid update_interval", prefix)
	}

	switch strings.ToLower(typeValue) {
	case "local":
		if !pathExists || path == "" {
			return common.NewErrorf("%s requires a path", prefix)
		}
		if urlExists && urlValue != "" {
			return common.NewErrorf("%s cannot contain a URL", prefix)
		}
		if effectiveDetour != "" {
			return common.NewErrorf("%s cannot contain download_detour", prefix)
		}
		if intervalExists && interval != "" {
			return common.NewErrorf("%s cannot contain update_interval", prefix)
		}
		if raw, exists := value["initial_path"]; exists && raw != nil {
			return common.NewErrorf("%s cannot contain initial_path", prefix)
		}
		if raw, exists := value["http_client"]; exists && raw != nil {
			return common.NewErrorf("%s cannot contain http_client", prefix)
		}
	case "remote":
		if !urlExists || urlValue == "" {
			return common.NewErrorf("%s requires a URL", prefix)
		}
		if pathExists && path != "" {
			return common.NewErrorf("%s cannot contain a path", prefix)
		}
		if intervalExists && interval != "" && !isValidSingboxDuration(interval) {
			return common.NewErrorf("%s has an invalid update_interval", prefix)
		}
		if effectiveDetour != "" {
			if _, ok := targets[effectiveDetour]; !ok {
				if httpClientDetour != "" {
					return common.NewErrorf("%s references unknown http_client.detour %q", prefix, effectiveDetour)
				}
				return common.NewErrorf("%s references unknown download_detour %q", prefix, effectiveDetour)
			}
		}
	case "inline":
		if formatExists && format != "" {
			return common.NewErrorf("%s cannot contain format", prefix)
		}
		if pathExists && path != "" || urlExists && urlValue != "" || effectiveDetour != "" || intervalExists && interval != "" {
			return common.NewErrorf("%s contains fields that are not valid for inline rule sets", prefix)
		}
		if raw, exists := value["initial_path"]; exists && raw != nil {
			return common.NewErrorf("%s cannot contain initial_path", prefix)
		}
		if raw, exists := value["http_client"]; exists && raw != nil {
			return common.NewErrorf("%s cannot contain http_client", prefix)
		}
		inlineRules, ok := value["rules"].([]any)
		if !ok || len(inlineRules) == 0 {
			return common.NewErrorf("%s requires inline rules", prefix)
		}
		for ruleIndex, rawRule := range inlineRules {
			if rule, ok := rawRule.(map[string]any); !ok || rule == nil {
				return common.NewErrorf("%s inline rule #%d is invalid", prefix, ruleIndex+1)
			}
		}
	default:
		return common.NewErrorf("%s uses unsupported type %q", prefix, typeValue)
	}
	return nil
}

func readSingboxRouteHTTPClientDetour(value map[string]any) (string, bool, error) {
	raw, exists := value["http_client"]
	if !exists || raw == nil {
		return "", false, nil
	}
	client, ok := raw.(map[string]any)
	if !ok || client == nil {
		return "", true, errors.New("http_client is not an object")
	}
	rawDetour, present := client["detour"]
	if !present || rawDetour == nil {
		return "", false, nil
	}
	detour, ok := rawDetour.(string)
	if !ok {
		return "", true, errors.New("http_client.detour is not a string")
	}
	return strings.TrimSpace(detour), true, nil
}

func readSingboxRouteString(value map[string]any, key string) (string, bool, error) {
	raw, exists := value[key]
	if !exists || raw == nil {
		return "", exists, nil
	}
	text, ok := raw.(string)
	if !ok {
		return "", true, errors.New("not a string")
	}
	return strings.TrimSpace(text), true, nil
}

func isValidSingboxDuration(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	for index := 0; index < len(value); {
		start := index
		digits := false
		dots := 0
		for index < len(value) {
			char := value[index]
			switch {
			case char >= '0' && char <= '9':
				digits = true
				index++
			case char == '.':
				dots++
				index++
			default:
				goto unit
			}
		}
	unit:
		if index == start || !digits || dots > 1 {
			return false
		}
		unitLength := 1
		if index+1 < len(value) {
			candidate := value[index : index+2]
			if candidate == "ns" || candidate == "us" || candidate == "ms" {
				unitLength = 2
			}
		}
		if unitLength == 2 {
			index += unitLength
			continue
		}
		if index >= len(value) || !strings.Contains("smhdw", value[index:index+1]) {
			return false
		}
		index++
	}
	return true
}

type singboxRouteRuleBudget struct {
	count int
	bytes int
}

func validateSingboxRouteRules(raw any, ruleSets, targets, inboundTargets map[string]struct{}, depth int, budget *singboxRouteRuleBudget) error {
	if budget == nil {
		return common.NewError("sing-box route rule budget is not ready")
	}
	if raw == nil {
		return nil
	}
	items, ok := raw.([]any)
	if !ok || len(items) > SingboxRouteMaxRules {
		return common.NewErrorf("sing-box route rules must contain at most %d items", SingboxRouteMaxRules)
	}
	for index, item := range items {
		rule, ok := item.(map[string]any)
		if !ok || rule == nil {
			return common.NewErrorf("sing-box route rule #%d is invalid", index+1)
		}
		payload, _ := json.Marshal(rule)
		if len(payload) > SingboxRouteMaxRuleBytes {
			return common.NewErrorf("sing-box route rule #%d exceeds %d bytes", index+1, SingboxRouteMaxRuleBytes)
		}
		budget.count++
		if budget.count > SingboxRouteMaxRules {
			return common.NewErrorf("sing-box route rules exceed the %d rule safety limit", SingboxRouteMaxRules)
		}
		budget.bytes += len(payload)
		if budget.bytes > SingboxRouteMaxRulesBytes {
			return common.NewErrorf("sing-box route rules exceed %d bytes", SingboxRouteMaxRulesBytes)
		}
		if depth > SingboxRouteMaxLogicalDepth {
			return common.NewErrorf("sing-box logical route nesting exceeds %d levels", SingboxRouteMaxLogicalDepth)
		}
		action, actionExists, err := readSingboxRouteString(rule, "action")
		if err != nil {
			return common.NewErrorf("sing-box route rule #%d has an invalid action", index+1)
		}
		outbound, outboundExists, err := readSingboxRouteString(rule, "outbound")
		if err != nil {
			return common.NewErrorf("sing-box route rule #%d has an invalid outbound", index+1)
		}
		if (actionExists && action == "route") || (outboundExists && outbound != "") {
			if outbound == "" {
				return common.NewErrorf("sing-box route rule #%d requires an outbound", index+1)
			}
			if _, ok := targets[outbound]; !ok {
				return common.NewErrorf("sing-box route rule #%d references unknown outbound %q", index+1, outbound)
			}
		}
		inbounds, err := stringValues(rule["inbound"])
		if err != nil {
			return common.NewErrorf("sing-box route rule #%d has invalid inbound references", index+1)
		}
		for _, inbound := range inbounds {
			if _, ok := inboundTargets[inbound]; !ok {
				return common.NewErrorf("sing-box route rule #%d references unknown inbound %q", index+1, inbound)
			}
		}
		references, err := stringValues(rule["rule_set"])
		if err != nil {
			return common.NewErrorf("sing-box route rule #%d has invalid rule_set references", index+1)
		}
		for _, ref := range references {
			if _, ok := ruleSets[ref]; !ok {
				return common.NewErrorf("sing-box route rule #%d references unknown rule_set %q", index+1, ref)
			}
		}
		if children, exists := rule["rules"]; exists {
			list, ok := children.([]any)
			if !ok || len(list) == 0 || len(list) > SingboxRouteMaxLogicalChildren {
				return common.NewErrorf("sing-box logical route rule #%d must contain 1 to %d children", index+1, SingboxRouteMaxLogicalChildren)
			}
			if err := validateSingboxRouteRules(list, ruleSets, targets, inboundTargets, depth+1, budget); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateSingboxRouteValue(value any, depth int) error {
	if depth > SingboxRouteMaxValueDepth {
		return common.NewError("sing-box route data nesting is too deep")
	}
	switch typed := value.(type) {
	case nil, bool, float64:
		return nil
	case string:
		if !utf8.ValidString(typed) || len(typed) > SingboxRouteMaxStringBytes {
			return common.NewErrorf("sing-box route text value exceeds %d bytes or is not valid UTF-8", SingboxRouteMaxStringBytes)
		}
	case []any:
		if len(typed) > SingboxRouteMaxArrayItems {
			return common.NewErrorf("sing-box route array exceeds %d items", SingboxRouteMaxArrayItems)
		}
		for _, item := range typed {
			if err := validateSingboxRouteValue(item, depth+1); err != nil {
				return err
			}
		}
	case map[string]any:
		if len(typed) > SingboxRouteMaxObjectEntries {
			return common.NewErrorf("sing-box route object exceeds %d fields", SingboxRouteMaxObjectEntries)
		}
		for key, item := range typed {
			if !utf8.ValidString(key) || len(key) > SingboxRouteMaxStringBytes {
				return common.NewError("sing-box route field name is invalid")
			}
			if err := validateSingboxRouteValue(item, depth+1); err != nil {
				return err
			}
		}
	default:
		return common.NewError("sing-box route contains an unsupported value")
	}
	return nil
}

func stringValues(value any) ([]string, error) {
	switch typed := value.(type) {
	case string:
		if strings.TrimSpace(typed) != "" {
			return []string{strings.TrimSpace(typed)}, nil
		}
		return nil, nil
	case []any:
		values := make([]string, 0, len(typed))
		for _, item := range typed {
			text, ok := item.(string)
			if !ok || strings.TrimSpace(text) == "" {
				return nil, errors.New("string list contains a non-string or empty value")
			}
			values = append(values, strings.TrimSpace(text))
		}
		return values, nil
	case nil:
		return nil, nil
	default:
		return nil, errors.New("expected a string or string list")
	}
}

func buildSingboxRouteChangeAudit(route json.RawMessage) json.RawMessage {
	audit := map[string]any{"bytes": len(route)}
	var document map[string]any
	if json.Unmarshal(route, &document) == nil {
		if rules, ok := document["rules"].([]any); ok {
			audit["ruleCount"] = len(rules)
		}
		if ruleSets, ok := document["rule_set"].([]any); ok {
			audit["ruleSetCount"] = len(ruleSets)
		}
		if final := strings.TrimSpace(firstString(document["final"])); final != "" {
			audit["final"] = final
		}
	}
	data, err := json.Marshal(audit)
	if err != nil {
		return json.RawMessage(`{"summary":"singbox_route"}`)
	}
	return data
}

func buildSingboxConfigChangeAudit(config json.RawMessage) json.RawMessage {
	audit := map[string]any{"bytes": len(config)}
	var document map[string]any
	if json.Unmarshal(config, &document) == nil {
		if route, ok := document["route"].(map[string]any); ok && route != nil {
			var routeRaw json.RawMessage
			if encoded, err := json.Marshal(route); err == nil {
				routeRaw = encoded
			}
			var routeAudit map[string]any
			if json.Unmarshal(buildSingboxRouteChangeAudit(routeRaw), &routeAudit) == nil {
				for key, value := range routeAudit {
					audit["route_"+key] = value
				}
			}
		}
	}
	data, err := json.Marshal(audit)
	if err != nil {
		return json.RawMessage(`{"summary":"config"}`)
	}
	return data
}
