package service

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"gorm.io/gorm"
)

type singboxInboundReference struct {
	Owner string
	Tag   string
}

func validateSingboxInboundRemovalReferences(tx *gorm.DB, removedTags []string) error {
	removed := normalizedSingboxOutboundReferenceSet(removedTags)
	if len(removed) == 0 {
		return nil
	}
	availableTags, err := loadSingboxRuntimeInboundReferenceTags(tx)
	if err != nil {
		return err
	}
	available := normalizedSingboxOutboundReferenceSet(availableTags)
	for tag := range available {
		delete(removed, tag)
	}
	if len(removed) == 0 {
		return nil
	}

	references, err := collectSingboxInboundReferences(tx)
	if err != nil {
		return err
	}

	blocked := make([]string, 0)
	for _, reference := range references {
		if _, exists := removed[reference.Tag]; exists {
			blocked = append(blocked, fmt.Sprintf("%s -> %s", reference.Owner, reference.Tag))
		}
	}
	if len(blocked) == 0 {
		return nil
	}

	sort.Strings(blocked)
	return fmt.Errorf("cannot remove sing-box inbound because it is still referenced by %s", strings.Join(blocked, "; "))
}

func validateSingboxConfiguredInboundReferences(tx *gorm.DB, config json.RawMessage) error {
	allowedTags, err := loadSingboxRuntimeInboundReferenceTags(tx)
	if err != nil {
		return err
	}
	allowed := normalizedSingboxOutboundReferenceSet(allowedTags)
	references, err := collectSingboxInboundReferencesFromConfig(config)
	if err != nil {
		return err
	}

	for _, reference := range references {
		if _, exists := allowed[reference.Tag]; !exists {
			return fmt.Errorf("sing-box %s references unknown inbound %q", reference.Owner, reference.Tag)
		}
	}
	return nil
}

func collectSingboxInboundReferences(tx *gorm.DB) ([]singboxInboundReference, error) {
	if tx == nil {
		return nil, fmt.Errorf("sing-box inbound reference validation requires a database transaction")
	}
	config, err := loadSingboxConfigForDNS(tx)
	if err != nil {
		return nil, err
	}
	references, err := collectSingboxInboundReferencesFromConfig(config)
	if err != nil {
		return nil, err
	}
	serviceReferences, err := collectSingboxServiceInboundReferences(tx)
	if err != nil {
		return nil, err
	}
	return append(references, serviceReferences...), nil
}

func collectSingboxInboundReferencesFromConfig(config json.RawMessage) ([]singboxInboundReference, error) {
	if len(config) == 0 {
		return nil, nil
	}

	root := map[string]interface{}{}
	if err := json.Unmarshal(config, &root); err != nil {
		return nil, err
	}

	references := make([]singboxInboundReference, 0)
	for _, sectionName := range []string{"route", "dns"} {
		section, _ := root[sectionName].(map[string]interface{})
		if section == nil {
			continue
		}
		if err := collectSingboxInboundRuleReferences(section["rules"], sectionName, &references); err != nil {
			return nil, err
		}
	}
	if experimental, _ := root["experimental"].(map[string]interface{}); experimental != nil {
		if v2rayAPI, _ := experimental["v2ray_api"].(map[string]interface{}); v2rayAPI != nil {
			if stats, _ := v2rayAPI["stats"].(map[string]interface{}); stats != nil {
				values, err := stringValues(stats["inbounds"])
				if err != nil {
					return nil, fmt.Errorf("sing-box experimental.v2ray_api.stats has invalid inbound references")
				}
				for _, tag := range values {
					references = append(references, singboxInboundReference{Owner: "experimental.v2ray_api.stats.inbounds", Tag: tag})
				}
			}
		}
	}
	return references, nil
}

func collectSingboxInboundRuleReferences(raw interface{}, ownerPrefix string, references *[]singboxInboundReference) error {
	if raw == nil {
		return nil
	}
	rules, ok := raw.([]interface{})
	if !ok {
		return fmt.Errorf("sing-box %s rules must be an array", ownerPrefix)
	}

	for index, rawRule := range rules {
		rule, ok := rawRule.(map[string]interface{})
		if !ok || rule == nil {
			return fmt.Errorf("sing-box %s rule #%d is invalid", ownerPrefix, index+1)
		}
		owner := fmt.Sprintf("%s rule #%d", ownerPrefix, index+1)
		values, err := stringValues(rule["inbound"])
		if err != nil {
			return fmt.Errorf("sing-box %s has invalid inbound references", owner)
		}
		for _, tag := range values {
			*references = append(*references, singboxInboundReference{Owner: owner, Tag: tag})
		}
		if nested, exists := rule["rules"]; exists {
			if err := collectSingboxInboundRuleReferences(nested, owner, references); err != nil {
				return err
			}
		}
	}
	return nil
}
