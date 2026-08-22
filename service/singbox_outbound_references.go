package service

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/alireza0/s-ui/database/model"
	"gorm.io/gorm"
)

// validateSingboxOutboundPayloadReferences runs while the edited outbound is
// still inside the caller's transaction. Removal checks alone cannot catch a
// new bad detour, selector member, or dialer-proxy target until a later core
// regeneration attempt.
func validateSingboxOutboundPayloadReferences(tx *gorm.DB, outbound *model.Outbound, payload map[string]interface{}) error {
	if tx == nil {
		return fmt.Errorf("sing-box outbound reference validation requires a database transaction")
	}
	if outbound == nil || payload == nil {
		return fmt.Errorf("sing-box outbound reference payload is invalid")
	}
	targets, err := loadSingboxRouteOutboundTags(tx)
	if err != nil {
		return err
	}
	known := normalizedSingboxOutboundReferenceSet(targets)
	ownerTags := normalizedSingboxOutboundReferenceSet(singboxRuntimeOutboundTagsFromMap(payload, outbound.Tag))
	owner := strings.TrimSpace(outbound.Tag)
	if owner == "" {
		owner = "unnamed"
	}
	if err := validateSingboxDNSResolverFields(tx, payload, fmt.Sprintf("sing-box outbound %q", owner)); err != nil {
		return err
	}

	validateTarget := func(field string, value interface{}) error {
		if value == nil {
			return nil
		}
		target, ok := value.(string)
		if !ok {
			return fmt.Errorf("sing-box outbound %q %s must be a string", owner, field)
		}
		target = strings.TrimSpace(target)
		if target == "" {
			return nil
		}
		if _, exists := known[target]; !exists {
			return fmt.Errorf("sing-box outbound %q references unknown %s %q", owner, field, target)
		}
		if _, self := ownerTags[target]; self {
			return fmt.Errorf("sing-box outbound %q cannot reference itself through %s", owner, field)
		}
		return nil
	}

	if err := validateTarget("detour", payload["detour"]); err != nil {
		return err
	}
	if err := validateTarget("dialer-proxy", payload["dialer-proxy"]); err != nil {
		return err
	}
	typeValue := strings.ToLower(strings.TrimSpace(firstString(payload["type"])))
	if typeValue != "selector" && typeValue != "urltest" {
		return nil
	}
	members, err := stringValues(payload["outbounds"])
	if err != nil {
		return fmt.Errorf("sing-box outbound %q has invalid selector members", owner)
	}
	for _, member := range members {
		if _, exists := known[member]; !exists {
			return fmt.Errorf("sing-box outbound %q references unknown member %q", owner, member)
		}
		if _, self := ownerTags[member]; self {
			return fmt.Errorf("sing-box outbound %q cannot include itself as a member", owner)
		}
	}
	return validateTarget("default", payload["default"])
}

// validateSingboxDNSResolverFields validates the DNS-card references exposed
// by Dial controls. The same field can appear in nested handshake/mesh
// objects, so walk only the explicitly named resolver keys while preserving
// all other extension data.
func validateSingboxDNSResolverFields(tx *gorm.DB, raw map[string]interface{}, owner string) error {
	if tx == nil || raw == nil {
		return nil
	}

	var known map[string]struct{}
	loadKnown := func() error {
		if known != nil {
			return nil
		}
		tags, err := loadSingboxEffectiveDNSServerTags(tx)
		if err != nil {
			return err
		}
		known = stringSet(tags)
		return nil
	}

	var walk func(interface{}, string) error
	walk = func(value interface{}, path string) error {
		switch typed := value.(type) {
		case map[string]interface{}:
			for key, child := range typed {
				childPath := key
				if path != "" {
					childPath = path + "." + key
				}
				if key == "domain_resolver" || key == "default_domain_resolver" {
					if child == nil {
						continue
					}
					resolver, ok := child.(string)
					if !ok {
						return fmt.Errorf("%s %s must be a string", owner, childPath)
					}
					resolver = strings.TrimSpace(resolver)
					if resolver == "" {
						continue
					}
					if err := loadKnown(); err != nil {
						return err
					}
					if _, exists := known[resolver]; !exists {
						return fmt.Errorf("%s %s references unknown DNS server %q", owner, childPath, resolver)
					}
					continue
				}
				if err := walk(child, childPath); err != nil {
					return err
				}
			}
		case []interface{}:
			for index, child := range typed {
				childPath := fmt.Sprintf("%s[%d]", path, index)
				if err := walk(child, childPath); err != nil {
					return err
				}
			}
		}
		return nil
	}

	return walk(raw, "")
}

// validateSingboxDNSServerRemovalReferences protects older databases that
// may still contain a resolver reference outside the current editor's
// candidate list. A selected final card is checked separately; this pass also
// catches stale references to non-final cards before they are deleted.
func validateSingboxDNSServerRemovalReferences(tx *gorm.DB, tag string) error {
	if tx == nil {
		return fmt.Errorf("sing-box DNS server reference validation requires a database transaction")
	}
	tag = strings.TrimSpace(tag)
	if tag == "" {
		return nil
	}

	references := make([]string, 0)
	add := func(owner string, raw interface{}) {
		resolver, ok := raw.(string)
		if ok && strings.TrimSpace(resolver) == tag {
			references = append(references, owner)
		}
	}
	var collect func(interface{}, string)
	collect = func(value interface{}, path string) {
		switch typed := value.(type) {
		case map[string]interface{}:
			for key, child := range typed {
				childPath := key
				if path != "" {
					childPath = path + "." + key
				}
				if key == "domain_resolver" || key == "default_domain_resolver" {
					add(childPath, child)
				}
				collect(child, childPath)
			}
		case []interface{}:
			for index, child := range typed {
				collect(child, fmt.Sprintf("%s[%d]", path, index))
			}
		}
	}

	var outbounds []model.Outbound
	if err := tx.Model(&model.Outbound{}).Select("tag", "type", "options", "raw_outbound").Order("id ASC").Find(&outbounds).Error; err != nil {
		return err
	}
	for index := range outbounds {
		raw, err := resolveOutboundJSON(&outbounds[index])
		if err != nil {
			return err
		}
		var payload interface{}
		if err := json.Unmarshal(raw, &payload); err != nil {
			return err
		}
		collect(payload, fmt.Sprintf("outbound %q", strings.TrimSpace(outbounds[index].Tag)))
	}

	var endpoints []model.Endpoint
	if err := tx.Model(&model.Endpoint{}).Select("tag", "options").Order("id ASC").Find(&endpoints).Error; err != nil {
		return err
	}
	for _, endpoint := range endpoints {
		var payload interface{}
		if len(endpoint.Options) > 0 {
			if err := json.Unmarshal(endpoint.Options, &payload); err != nil {
				return err
			}
		}
		collect(payload, fmt.Sprintf("endpoint %q", strings.TrimSpace(endpoint.Tag)))
	}

	var services []model.Service
	if err := tx.Model(&model.Service{}).Select("tag", "options").Order("id ASC").Find(&services).Error; err != nil {
		return err
	}
	for _, service := range services {
		var payload interface{}
		if len(service.Options) > 0 {
			if err := json.Unmarshal(service.Options, &payload); err != nil {
				return err
			}
		}
		collect(payload, fmt.Sprintf("service %q", strings.TrimSpace(service.Tag)))
	}

	configRaw, err := loadSingboxConfigForDNS(tx)
	if err != nil {
		return err
	}
	var config interface{}
	if len(configRaw) > 0 {
		if err := json.Unmarshal(configRaw, &config); err != nil {
			return err
		}
	}
	collect(config, "config")

	var servers []model.DnsServer
	if err := tx.Model(&model.DnsServer{}).Select("tag", "options").Order("id ASC").Find(&servers).Error; err != nil {
		return err
	}
	for _, server := range servers {
		var payload interface{}
		if len(server.Options) > 0 {
			if err := json.Unmarshal(server.Options, &payload); err != nil {
				return err
			}
		}
		collect(payload, fmt.Sprintf("DNS server %q", strings.TrimSpace(server.Tag)))
	}

	if len(references) == 0 {
		return nil
	}
	sort.Strings(references)
	return fmt.Errorf("cannot remove sing-box DNS server %q because it is referenced by %s", tag, strings.Join(references, "; "))
}

// validateSingboxOutboundRemovalReferences keeps an outbound mutation from
// committing a default-chain configuration that still references a removed tag.
// The sing-box process is independent and only consumes generated config later,
// so rejecting the mutation here prevents a delayed restart failure.
func validateSingboxOutboundRemovalReferences(tx *gorm.DB, removedTags []string, skippedGroupNames []string) error {
	if tx == nil {
		return fmt.Errorf("sing-box outbound reference validation requires a database transaction")
	}

	expandedTags, err := expandSingboxRuntimeOutboundTags(tx, removedTags)
	if err != nil {
		return err
	}
	removed := normalizedSingboxOutboundReferenceSet(expandedTags)
	if len(removed) == 0 {
		return nil
	}

	references := make([]string, 0)
	addReference := func(owner string, target string) {
		target = strings.TrimSpace(target)
		if target == "" {
			return
		}
		if _, exists := removed[target]; exists {
			references = append(references, fmt.Sprintf("%s -> %s", owner, target))
		}
	}

	var outbounds []model.Outbound
	if err := tx.Model(&model.Outbound{}).Order("id ASC").Find(&outbounds).Error; err != nil {
		return err
	}
	for index := range outbounds {
		outbound := &outbounds[index]
		if _, deleting := removed[strings.TrimSpace(outbound.Tag)]; deleting {
			continue
		}
		raw, err := resolveOutboundJSON(outbound)
		if err != nil {
			return fmt.Errorf("decode sing-box outbound %q references: %w", outbound.Tag, err)
		}
		payload := map[string]interface{}{}
		if err := json.Unmarshal(raw, &payload); err != nil {
			return fmt.Errorf("decode sing-box outbound %q references: %w", outbound.Tag, err)
		}

		owner := fmt.Sprintf("outbound %q", strings.TrimSpace(outbound.Tag))
		outboundType := strings.ToLower(strings.TrimSpace(firstString(payload["type"])))
		if outboundType == "selector" || outboundType == "urltest" {
			for _, member := range toStringSlice(payload["outbounds"]) {
				addReference(owner, member)
			}
			addReference(owner, firstString(payload["default"]))
		}
		addReference(owner, firstString(payload["detour"]))
		addReference(owner, firstString(payload["dialer-proxy"]))
	}

	skippedGroups := normalizedSingboxOutboundReferenceSet(skippedGroupNames)
	var groups []model.OutboundGroup
	if err := tx.Model(&model.OutboundGroup{}).Select("name", "outbounds").Find(&groups).Error; err != nil {
		return err
	}
	for _, group := range groups {
		if _, skipped := skippedGroups[strings.TrimSpace(group.Name)]; skipped {
			continue
		}
		for _, tag := range parseOutboundGroupTags(group.Outbounds) {
			addReference(fmt.Sprintf("panel group %q", strings.TrimSpace(group.Name)), tag)
		}
	}
	if err := collectSingboxServiceOutboundReferences(tx, addReference); err != nil {
		return err
	}

	configRaw, err := loadSingboxConfigForDNS(tx)
	if err != nil {
		return err
	}
	config := map[string]interface{}{}
	if len(configRaw) > 0 && json.Unmarshal(configRaw, &config) == nil {
		if dns, _ := config["dns"].(map[string]interface{}); dns != nil {
			finalTag := firstString(dns["final"])
			rawServer, found, err := (&DnsServerService{}).GetSelectedConfig(tx, finalTag)
			if err != nil {
				return err
			}
			if found {
				server := map[string]interface{}{}
				if err := json.Unmarshal(rawServer, &server); err != nil {
					return fmt.Errorf("decode selected DNS server references: %w", err)
				}
				serverTag := firstString(server["tag"])
				if serverTag == "" {
					serverTag = "unnamed"
				}
				addReference(fmt.Sprintf("dns server %q", serverTag), firstString(server["detour"]))
			} else if legacyServer, legacyFound := legacySelectedDNSConfig(dns, finalTag); legacyFound {
				serverTag := firstString(legacyServer["tag"])
				if serverTag == "" {
					serverTag = "unnamed"
				}
				addReference(fmt.Sprintf("dns server %q", serverTag), firstString(legacyServer["detour"]))
			}
		}
		if ntp, _ := config["ntp"].(map[string]interface{}); ntp != nil {
			addReference("ntp", firstString(ntp["detour"]))
		}
		if experimental, _ := config["experimental"].(map[string]interface{}); experimental != nil {
			if clashAPI, _ := experimental["clash_api"].(map[string]interface{}); clashAPI != nil {
				addReference("experimental.clash_api", firstString(clashAPI["external_ui_download_detour"]))
			}
			if v2rayAPI, _ := experimental["v2ray_api"].(map[string]interface{}); v2rayAPI != nil {
				if stats, _ := v2rayAPI["stats"].(map[string]interface{}); stats != nil {
					for _, target := range toStringSlice(stats["outbounds"]) {
						addReference("experimental.v2ray_api.stats.outbounds", target)
					}
				}
			}
		}
		route, _ := config["route"].(map[string]interface{})
		if route != nil {
			addReference("route.final", firstString(route["final"]))
			collectSingboxRouteOutboundReferences(route["rules"], "route rule", addReference)
			for _, item := range toInterfaceSlice(route["rule_set"]) {
				ruleSet, _ := item.(map[string]interface{})
				if ruleSet == nil {
					continue
				}
				name := strings.TrimSpace(firstString(ruleSet["tag"]))
				if name == "" {
					name = "unnamed"
				}
				detour := firstString(ruleSet["download_detour"])
				if client, _ := ruleSet["http_client"].(map[string]interface{}); client != nil {
					if current := firstString(client["detour"]); current != "" {
						detour = current
					}
				}
				addReference(fmt.Sprintf("rule_set %q", name), detour)
			}
		}
	}

	if len(references) == 0 {
		return nil
	}
	sort.Strings(references)
	return fmt.Errorf("cannot remove sing-box outbound because it is still referenced by %s", strings.Join(references, "; "))
}

func collectSingboxRouteOutboundReferences(raw interface{}, ownerPrefix string, addReference func(string, string)) {
	for index, item := range toInterfaceSlice(raw) {
		rule, _ := item.(map[string]interface{})
		if rule == nil {
			continue
		}
		owner := fmt.Sprintf("%s #%d", ownerPrefix, index+1)
		if strings.EqualFold(strings.TrimSpace(firstString(rule["action"])), "route") || strings.TrimSpace(firstString(rule["outbound"])) != "" {
			addReference(owner, firstString(rule["outbound"]))
		}
		if nested, exists := rule["rules"]; exists {
			collectSingboxRouteOutboundReferences(nested, owner, addReference)
		}
	}
}

func normalizedSingboxOutboundReferenceSet(values []string) map[string]struct{} {
	result := make(map[string]struct{}, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		result[value] = struct{}{}
	}
	return result
}

func replaceSingboxOutboundTagInPanelGroups(tx *gorm.DB, oldTag string, newTag string) error {
	if tx == nil {
		return fmt.Errorf("sing-box panel group update requires a database transaction")
	}
	oldTag = strings.TrimSpace(oldTag)
	newTag = strings.TrimSpace(newTag)
	if oldTag == "" || oldTag == newTag {
		return nil
	}

	var groups []model.OutboundGroup
	if err := tx.Model(&model.OutboundGroup{}).Select("id", "outbounds").Find(&groups).Error; err != nil {
		return err
	}
	for _, group := range groups {
		tags := parseOutboundGroupTags(group.Outbounds)
		updated := make([]string, 0, len(tags))
		seen := make(map[string]struct{}, len(tags))
		changed := false
		for _, tag := range tags {
			if strings.TrimSpace(tag) == oldTag {
				changed = true
				tag = newTag
			}
			tag = strings.TrimSpace(tag)
			if tag == "" {
				continue
			}
			if _, exists := seen[tag]; exists {
				changed = true
				continue
			}
			seen[tag] = struct{}{}
			updated = append(updated, tag)
		}
		if !changed {
			continue
		}
		encoded, err := json.Marshal(updated)
		if err != nil {
			return err
		}
		if err := tx.Model(&model.OutboundGroup{}).Where("id = ?", group.Id).Update("outbounds", string(encoded)).Error; err != nil {
			return err
		}
	}
	return nil
}

func removeSingboxOutboundTagFromPanelGroups(tx *gorm.DB, tag string) error {
	return replaceSingboxOutboundTagInPanelGroups(tx, tag, "")
}
