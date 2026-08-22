package service

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/alireza0/s-ui/database/model"
	"gorm.io/gorm"
)

// validateSingboxRuntimeTaggedObjects prevents a generated config from
// carrying duplicate tags after panel records are expanded into runtime objects.
func validateSingboxRuntimeTaggedObjects(kind string, objects []json.RawMessage) error {
	owners := make(map[string]int, len(objects))
	for index, raw := range objects {
		object := map[string]interface{}{}
		if err := json.Unmarshal(raw, &object); err != nil {
			return fmt.Errorf("decode sing-box runtime %s #%d: %w", kind, index+1, err)
		}
		tag := strings.TrimSpace(firstString(object["tag"]))
		if tag == "" {
			continue
		}
		if previous, exists := owners[tag]; exists {
			return fmt.Errorf("sing-box runtime %s tag %q is duplicated by entries %d and %d", kind, tag, previous+1, index+1)
		}
		owners[tag] = index
	}
	return nil
}

func singboxRuntimeOutboundTags(outbound *model.Outbound) ([]string, error) {
	if outbound == nil {
		return nil, fmt.Errorf("sing-box outbound is nil")
	}
	raw, err := resolveOutboundJSON(outbound)
	if err != nil {
		return nil, err
	}
	payload := map[string]interface{}{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	return singboxRuntimeOutboundTagsForType(payload, outbound.Tag, outbound.Type), nil
}

func singboxRuntimeOutboundTagsFromMap(payload map[string]interface{}, fallbackTag string) []string {
	return singboxRuntimeOutboundTagsForType(payload, fallbackTag, firstString(payload["type"]))
}

func singboxRuntimeOutboundTagsForType(payload map[string]interface{}, fallbackTag string, runtimeType string) []string {
	tag := strings.TrimSpace(firstString(payload["tag"]))
	if tag == "" {
		tag = strings.TrimSpace(fallbackTag)
	}
	if tag == "" {
		return nil
	}

	tags := []string{tag}
	if runtimeType == "shadowtls" && hasShadowTLSSSConfig(payload) {
		tags = append(tags, tag+"-out")
	}
	return compactUniqueStrings(tags)
}

func singboxRuntimeInboundTags(inbound *model.Inbound) []string {
	if inbound == nil || inbound.Type == "ssh" {
		return nil
	}

	tag := strings.TrimSpace(inbound.Tag)
	if tag == "" {
		return nil
	}

	tags := []string{tag}
	if inbound.Type == "shadowtls" {
		options := map[string]interface{}{}
		if len(inbound.Options) > 0 && json.Unmarshal(inbound.Options, &options) == nil && hasShadowTLSSSConfig(options) {
			tags = append(tags, tag+"-in")
		}
	}
	return compactUniqueStrings(tags)
}

// singboxRouteInboundReferenceTags returns every inbound label that can be
// matched by route and DNS rules for one stored inbound. It keeps the actual
// runtime tags and the panel's effective detour tag together.
func singboxRouteInboundReferenceTags(inbound *model.Inbound) []string {
	runtimeTags := singboxRuntimeInboundTags(inbound)
	if len(runtimeTags) == 0 {
		return nil
	}

	effectiveTag := deriveEffectiveInboundRouteTagFromRaw(inbound.Tag, inbound.Type, inbound.Options)
	return compactUniqueStrings(append(runtimeTags, effectiveTag))
}

func singboxRouteEndpointReferenceTags(endpoint *model.Endpoint) []string {
	if endpoint == nil {
		return nil
	}

	options := map[string]interface{}{}
	if len(endpoint.Options) == 0 || json.Unmarshal(endpoint.Options, &options) != nil {
		return nil
	}
	port, ok := asPositiveInteger(options["listen_port"])
	if !ok || port == 0 {
		return nil
	}

	tag := deriveEffectiveEndpointRouteTagFromRaw(endpoint.Tag, endpoint.Options)
	return compactUniqueStrings([]string{tag})
}

func removedSingboxRuntimeTags(previous []string, current []string) []string {
	currentSet := make(map[string]struct{}, len(current))
	for _, tag := range current {
		if tag = strings.TrimSpace(tag); tag != "" {
			currentSet[tag] = struct{}{}
		}
	}

	removed := make([]string, 0, len(previous))
	seen := make(map[string]struct{}, len(previous))
	for _, tag := range previous {
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		if _, exists := currentSet[tag]; exists {
			continue
		}
		if _, exists := seen[tag]; exists {
			continue
		}
		seen[tag] = struct{}{}
		removed = append(removed, tag)
	}
	return removed
}

func validateSingboxStoredRuntimeOutboundTags(tx *gorm.DB) error {
	if tx == nil {
		return fmt.Errorf("sing-box outbound tag validation requires a database transaction")
	}

	var outbounds []model.Outbound
	if err := tx.Model(&model.Outbound{}).Order("id ASC").Find(&outbounds).Error; err != nil {
		return err
	}

	owners := make(map[string]string, len(outbounds))
	for index := range outbounds {
		outbound := &outbounds[index]
		tags, err := singboxRuntimeOutboundTags(outbound)
		if err != nil {
			return fmt.Errorf("decode sing-box outbound %q runtime tags: %w", outbound.Tag, err)
		}
		for _, tag := range tags {
			if owner, exists := owners[tag]; exists {
				return fmt.Errorf("sing-box runtime outbound tag %q conflicts between %q and %q", tag, owner, outbound.Tag)
			}
			owners[tag] = outbound.Tag
		}
	}
	return nil
}

func validateSingboxStoredRuntimeInboundTags(tx *gorm.DB) error {
	if tx == nil {
		return fmt.Errorf("sing-box inbound tag validation requires a database transaction")
	}

	var inbounds []model.Inbound
	if err := tx.Model(&model.Inbound{}).Order("id ASC").Find(&inbounds).Error; err != nil {
		return err
	}

	owners := make(map[string]string, len(inbounds))
	for index := range inbounds {
		inbound := &inbounds[index]
		for _, tag := range singboxRuntimeInboundTags(inbound) {
			if owner, exists := owners[tag]; exists {
				return fmt.Errorf("sing-box runtime inbound tag %q conflicts between %q and %q", tag, owner, inbound.Tag)
			}
			owners[tag] = inbound.Tag
		}
	}
	return nil
}

func expandSingboxRuntimeOutboundTags(tx *gorm.DB, tags []string) ([]string, error) {
	result := compactUniqueStrings(tags)
	if tx == nil || len(result) == 0 {
		return result, nil
	}

	var outbounds []model.Outbound
	if err := tx.Model(&model.Outbound{}).Where("tag IN ?", result).Find(&outbounds).Error; err != nil {
		return nil, err
	}
	for index := range outbounds {
		runtimeTags, err := singboxRuntimeOutboundTags(&outbounds[index])
		if err != nil {
			return nil, fmt.Errorf("decode sing-box outbound %q runtime tags: %w", outbounds[index].Tag, err)
		}
		result = append(result, runtimeTags...)
	}
	return compactUniqueStrings(result), nil
}

func collectSingboxRuntimeOutboundTagsByTags(tx *gorm.DB, tags []string) ([]string, error) {
	if tx == nil || len(tags) == 0 {
		return nil, nil
	}

	var outbounds []model.Outbound
	if err := tx.Model(&model.Outbound{}).Where("tag IN ?", compactUniqueStrings(tags)).Find(&outbounds).Error; err != nil {
		return nil, err
	}

	result := make([]string, 0, len(outbounds)*2)
	for index := range outbounds {
		runtimeTags, err := singboxRuntimeOutboundTags(&outbounds[index])
		if err != nil {
			return nil, fmt.Errorf("decode sing-box outbound %q runtime tags: %w", outbounds[index].Tag, err)
		}
		result = append(result, runtimeTags...)
	}
	return compactUniqueStrings(result), nil
}

func collectSingboxRuntimeOutboundTagsFromMaps(outbounds []map[string]interface{}) []string {
	result := make([]string, 0, len(outbounds)*2)
	for _, outbound := range outbounds {
		result = append(result, singboxRuntimeOutboundTagsFromMap(outbound, "")...)
	}
	return compactUniqueStrings(result)
}

func loadSingboxRuntimeInboundReferenceTags(tx *gorm.DB) ([]string, error) {
	if tx == nil {
		return nil, fmt.Errorf("sing-box inbound reference validation requires a database transaction")
	}

	var inbounds []model.Inbound
	if err := tx.Select("type", "tag", "options").Order("id ASC").Find(&inbounds).Error; err != nil {
		return nil, err
	}

	tags := make([]string, 0, len(inbounds)*2)
	for index := range inbounds {
		tags = append(tags, singboxRouteInboundReferenceTags(&inbounds[index])...)
	}

	var endpoints []model.Endpoint
	if err := tx.Select("tag", "options").Order("id ASC").Find(&endpoints).Error; err != nil {
		return nil, err
	}
	for index := range endpoints {
		tags = append(tags, singboxRouteEndpointReferenceTags(&endpoints[index])...)
	}

	return compactUniqueStrings(tags), nil
}
