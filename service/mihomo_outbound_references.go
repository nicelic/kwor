package service

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/alireza0/s-ui/database/model"
	"gorm.io/gorm"
)

// validateMihomoOutboundRemovalReferences prevents a save from committing a
// configuration whose remaining records still point at a deleted outbound.
// The renderer would reject that state only after commit, leaving server.yaml
// stale while SQLite already contains the broken data.
func validateMihomoOutboundRemovalReferences(tx *gorm.DB, removedTags []string, skippedGroupNames []string, shadowQUICTargets []string) error {
	if tx == nil {
		return fmt.Errorf("mihomo outbound reference validation requires a database transaction")
	}

	removed := normalizedMihomoReferenceSet(removedTags)
	if len(removed) == 0 && len(shadowQUICTargets) == 0 {
		return nil
	}

	references := make([]string, 0)
	addReference := func(owner, target string) {
		target = strings.TrimSpace(target)
		if target == "" {
			return
		}
		if _, exists := removed[target]; exists {
			references = append(references, fmt.Sprintf("%s -> %s", owner, target))
		}
	}

	var outbounds []model.MihomoOutbound
	if err := tx.Model(&model.MihomoOutbound{}).Order("id ASC").Find(&outbounds).Error; err != nil {
		return err
	}
	for index := range outbounds {
		outbound := &outbounds[index]
		if _, deleting := removed[strings.TrimSpace(outbound.Tag)]; deleting {
			continue
		}
		raw, err := resolveMihomoOutboundJSON(outbound)
		if err != nil {
			return fmt.Errorf("decode mihomo outbound %q references: %w", outbound.Tag, err)
		}
		payload := map[string]interface{}{}
		if err := json.Unmarshal(raw, &payload); err != nil {
			return fmt.Errorf("decode mihomo outbound %q references: %w", outbound.Tag, err)
		}

		owner := fmt.Sprintf("outbound %q", strings.TrimSpace(outbound.Tag))
		outboundType := strings.ToLower(strings.TrimSpace(firstString(payload["type"])))
		if outboundType == "selector" || outboundType == "urltest" {
			for _, member := range toStringSlice(payload["outbounds"]) {
				addReference(owner, member)
			}
		}
		addReference(owner, firstString(payload["detour"]))
		addReference(owner, firstString(payload["dialer-proxy"]))
	}

	var inbounds []model.MihomoInbound
	if err := tx.Model(&model.MihomoInbound{}).Select("id", "tag", "type", "options").Find(&inbounds).Error; err != nil {
		return err
	}
	for _, inbound := range inbounds {
		if strings.EqualFold(strings.TrimSpace(inbound.Type), "shadowquic") {
			continue
		}
		options := map[string]interface{}{}
		if len(inbound.Options) == 0 || json.Unmarshal(inbound.Options, &options) != nil {
			continue
		}
		addReference(fmt.Sprintf("listener %q", strings.TrimSpace(inbound.Tag)), extractDetourFromOptions(options))
	}

	var tlsConfigs []model.MihomoTls
	if err := tx.Model(&model.MihomoTls{}).Find(&tlsConfigs).Error; err != nil {
		return err
	}
	for index := range tlsConfigs {
		tlsConfig := &tlsConfigs[index]
		references, err := collectMihomoTLSOutboundReferences(tlsConfig)
		if err != nil {
			return err
		}
		name := strings.TrimSpace(tlsConfig.Name)
		if name == "" {
			name = fmt.Sprintf("id=%d", tlsConfig.Id)
		}
		for _, reference := range references {
			addReference(fmt.Sprintf("mihomo TLS %q %s", name, reference.Path), reference.Target)
		}
	}

	if len(removed) > 0 {
		skippedGroups := normalizedMihomoReferenceSet(skippedGroupNames)
		var groups []model.MihomoOutboundGroup
		if err := tx.Model(&model.MihomoOutboundGroup{}).Select("name", "outbounds").Find(&groups).Error; err != nil {
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
	}

	configText, err := (&MihomoConfigService{}).GetConfigWithDB(tx)
	if err != nil {
		return err
	}
	config := map[string]interface{}{}
	if strings.TrimSpace(configText) != "" && json.Unmarshal([]byte(configText), &config) == nil {
		route, _ := config["route"].(map[string]interface{})
		if route != nil {
			addReference("route.final", firstString(route["final"]))
			for _, item := range toInterfaceSlice(route["rules"]) {
				rule, _ := item.(map[string]interface{})
				if strings.EqualFold(strings.TrimSpace(firstString(rule["action"])), "route") {
					addReference("route rule", firstString(rule["outbound"]))
				}
			}
			for _, item := range toInterfaceSlice(route["rule_set"]) {
				provider, _ := item.(map[string]interface{})
				if provider == nil {
					continue
				}
				name := strings.TrimSpace(firstString(provider["tag"]))
				if name == "" {
					name = "unnamed"
				}
				addReference(fmt.Sprintf("rule provider %q", name), firstString(provider["proxy"]))
				addReference(fmt.Sprintf("rule provider %q", name), firstString(provider["download_detour"]))
			}
		}
	}

	if len(shadowQUICTargets) > 0 {
		inboundTags, err := findMihomoShadowQUICJLSProxyInboundTags(tx, shadowQUICTargets...)
		if err != nil {
			return err
		}
		for _, tag := range inboundTags {
			references = append(references, fmt.Sprintf("shadowquic listener %q", tag))
		}
	}

	if len(references) == 0 {
		return nil
	}
	sort.Strings(references)
	return fmt.Errorf("cannot remove mihomo outbound target because it is still referenced by %s", strings.Join(references, "; "))
}

func normalizedMihomoReferenceSet(values []string) map[string]struct{} {
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

func replaceMihomoOutboundTagInPanelGroups(tx *gorm.DB, oldTag string, newTag string) error {
	if tx == nil {
		return fmt.Errorf("mihomo panel group update requires a database transaction")
	}
	oldTag = strings.TrimSpace(oldTag)
	newTag = strings.TrimSpace(newTag)
	if oldTag == "" || oldTag == newTag {
		return nil
	}

	var groups []model.MihomoOutboundGroup
	if err := tx.Model(&model.MihomoOutboundGroup{}).Select("id", "outbounds").Find(&groups).Error; err != nil {
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
		if err := tx.Model(&model.MihomoOutboundGroup{}).Where("id = ?", group.Id).Update("outbounds", string(encoded)).Error; err != nil {
			return err
		}
	}
	return nil
}

func removeMihomoOutboundTagFromPanelGroups(tx *gorm.DB, tag string) error {
	return replaceMihomoOutboundTagInPanelGroups(tx, tag, "")
}
