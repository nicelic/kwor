package service

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/alireza0/s-ui/database/model"
	"gorm.io/gorm"
)

// loadMihomoRouteInboundRuleTags returns only listeners that can actually
// attach a Mihomo rule. A listener with a native proxy detour is terminal at
// the listener level, so it deliberately has no rule name and cannot safely
// appear in route.rules[].inbound.
func loadMihomoRouteInboundRuleTags(db *gorm.DB, targets *mihomoProxyConversionResult) (map[string]struct{}, error) {
	if db == nil {
		return nil, fmt.Errorf("mihomo route inbound validation requires a database")
	}

	var inbounds []model.MihomoInbound
	if err := db.Model(model.MihomoInbound{}).
		Select("tag", "type", "options").
		Find(&inbounds).Error; err != nil {
		return nil, fmt.Errorf("load mihomo route inbounds failed: %w", err)
	}

	tags := make(map[string]struct{}, len(inbounds))
	for _, inbound := range inbounds {
		if !isSupportedMihomoInboundType(inbound.Type) {
			continue
		}

		ref, err := buildMihomoInboundRouteRef(inbound, targets, "DIRECT")
		if err != nil {
			return nil, err
		}
		if tag := strings.TrimSpace(ref.RuleName); tag != "" {
			tags[tag] = struct{}{}
		}
	}

	return tags, nil
}

func sortedMihomoRouteInboundRuleTags(tags map[string]struct{}) []string {
	result := make([]string, 0, len(tags))
	for tag := range tags {
		result = append(result, tag)
	}
	sort.Strings(result)
	return result
}

func normalizeMihomoRouteInboundRuleTags(raw interface{}) ([]string, error) {
	if raw == nil {
		return nil, nil
	}

	values := make([]string, 0)
	switch value := raw.(type) {
	case string:
		values = append(values, value)
	case []string:
		values = append(values, value...)
	case []interface{}:
		values = make([]string, 0, len(value))
		for _, item := range value {
			text, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("must be a string or an array of strings")
			}
			values = append(values, text)
		}
	default:
		return nil, fmt.Errorf("must be a string or an array of strings")
	}

	return normalizeMihomoInboundRules(values, nil), nil
}

// validateMihomoConfigInboundReferences rejects route rules that would be
// silently discarded because their selected listener is absent or uses a
// native proxy detour.
func validateMihomoConfigInboundReferences(tx *gorm.DB, config json.RawMessage) error {
	if tx == nil || len(config) == 0 {
		return nil
	}

	var document map[string]interface{}
	if err := json.Unmarshal(config, &document); err != nil {
		return err
	}
	route, _ := document["route"].(map[string]interface{})
	if route == nil {
		return nil
	}

	rawRules, ok := route["rules"].([]interface{})
	if !ok || len(rawRules) == 0 {
		return nil
	}

	targets, err := loadMihomoRouteTargets(tx)
	if err != nil {
		return err
	}
	available, err := loadMihomoRouteInboundRuleTags(tx, targets)
	if err != nil {
		return err
	}

	for index, rawRule := range rawRules {
		rule, ok := rawRule.(map[string]interface{})
		if !ok || rule == nil {
			continue
		}

		inboundTags, err := normalizeMihomoRouteInboundRuleTags(rule["inbound"])
		if err != nil {
			return fmt.Errorf("mihomo route rule #%d inbound %w", index+1, err)
		}

		missing := make([]string, 0)
		for _, tag := range inboundTags {
			if _, exists := available[tag]; !exists {
				missing = append(missing, tag)
			}
		}
		if len(missing) > 0 {
			sort.Strings(missing)
			return fmt.Errorf("mihomo route rule #%d references unavailable inbound tag(s): %s", index+1, strings.Join(missing, ", "))
		}
	}

	return nil
}

func validateMihomoStoredInboundReferences(tx *gorm.DB) error {
	config, err := (&MihomoConfigService{}).GetConfigWithDB(tx)
	if err != nil {
		return err
	}
	return validateMihomoConfigInboundReferences(tx, json.RawMessage(config))
}
