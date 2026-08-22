package service

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/util/common"
	"gorm.io/gorm"
)

// validateSingboxServiceReferences checks the tags exposed by the service
// editor while the service mutation is still inside its transaction. Without
// this pass, a service could be written successfully and fail only during the
// next generated-core refresh.
func validateSingboxServiceReferences(tx *gorm.DB, payload *singboxEntityPayload) error {
	if tx == nil {
		return common.NewError("sing-box service reference validation requires a database transaction")
	}
	if payload == nil {
		return common.NewError("sing-box service payload is required")
	}

	fields := make(map[string]any, len(payload.Fields))
	for key, raw := range payload.Fields {
		var value any
		if err := json.Unmarshal(raw, &value); err != nil {
			return common.NewErrorf("sing-box service %s is invalid: %v", key, err)
		}
		fields[key] = value
	}

	inboundTags, err := loadSingboxRuntimeInboundReferenceTags(tx)
	if err != nil {
		return err
	}
	inboundSet := stringSet(inboundTags)
	outboundTags, err := loadSingboxRouteOutboundTags(tx)
	if err != nil {
		return err
	}
	outboundSet := stringSet(outboundTags)
	dnsTags, err := loadSingboxEffectiveDNSServerTags(tx)
	if err != nil {
		return err
	}
	dnsSet := stringSet(dnsTags)

	validateReference := func(raw any, known map[string]struct{}, field string) error {
		if raw == nil {
			return nil
		}
		value, ok := raw.(string)
		if !ok {
			return common.NewErrorf("sing-box service %s must be a string", field)
		}
		value = strings.TrimSpace(value)
		if value == "" {
			return nil
		}
		if _, exists := known[value]; !exists {
			return common.NewErrorf("sing-box service %s references unknown target %q", field, value)
		}
		return nil
	}

	validateList := func(raw any, known map[string]struct{}, field string) error {
		values, err := stringValues(raw)
		if err != nil {
			return common.NewErrorf("sing-box service %s must be a string list", field)
		}
		for _, value := range values {
			if _, exists := known[value]; !exists {
				return common.NewErrorf("sing-box service %s references unknown target %q", field, value)
			}
		}
		return nil
	}

	validateDial := func(raw any, owner string) error {
		if raw == nil {
			return nil
		}
		dial, ok := raw.(map[string]any)
		if !ok || dial == nil {
			return common.NewErrorf("sing-box service %s must be an object", owner)
		}
		if err := validateReference(dial["detour"], outboundSet, owner+".detour"); err != nil {
			return err
		}
		return validateReference(dial["domain_resolver"], dnsSet, owner+".domain_resolver")
	}

	if err := validateReference(fields["detour"], inboundSet, "detour"); err != nil {
		return err
	}

	switch payload.Type {
	case "derp":
		var endpointTags []string
		if err := tx.Model(&model.Endpoint{}).
			Where("type = ?", "tailscale").Order("id ASC").Pluck("tag", &endpointTags).Error; err != nil {
			return err
		}
		if err := validateList(fields["verify_client_endpoint"], stringSet(endpointTags), "verify_client_endpoint"); err != nil {
			return err
		}
		if raw, exists := fields["verify_client_url"]; exists && raw != nil {
			urls, ok := raw.([]any)
			if !ok {
				return common.NewError("sing-box service verify_client_url must be an array")
			}
			for index, item := range urls {
				if err := validateDial(item, fmt.Sprintf("verify_client_url[%d]", index)); err != nil {
					return err
				}
			}
		}
		if raw, exists := fields["mesh_with"]; exists && raw != nil {
			mesh, ok := raw.([]any)
			if !ok {
				return common.NewError("sing-box service mesh_with must be an array")
			}
			for index, item := range mesh {
				if err := validateDial(item, fmt.Sprintf("mesh_with[%d]", index)); err != nil {
					return err
				}
			}
		}
		if stun, ok := fields["stun"].(map[string]any); ok && stun != nil {
			if err := validateReference(stun["detour"], inboundSet, "stun.detour"); err != nil {
				return err
			}
		}
	case "ssm-api":
		if raw, exists := fields["servers"]; exists && raw != nil {
			servers, ok := raw.(map[string]any)
			if !ok {
				return common.NewError("sing-box service servers must be an object")
			}
			for path, target := range servers {
				if err := validateList(target, inboundSet, "servers."+path); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func collectSingboxServiceInboundReferences(tx *gorm.DB) ([]singboxInboundReference, error) {
	if tx == nil {
		return nil, common.NewError("sing-box service reference collection requires a database transaction")
	}
	services := make([]model.Service, 0)
	if err := tx.Select("type", "tag", "options").Order("id ASC").Find(&services).Error; err != nil {
		return nil, err
	}
	references := make([]singboxInboundReference, 0)
	for _, service := range services {
		options := map[string]any{}
		if len(service.Options) > 0 && string(service.Options) != "null" {
			if err := json.Unmarshal(service.Options, &options); err != nil {
				return nil, fmt.Errorf("decode sing-box service %q references: %w", service.Tag, err)
			}
		}
		owner := fmt.Sprintf("service %q", strings.TrimSpace(service.Tag))
		add := func(field string, raw any) error {
			values, err := stringValues(raw)
			if err != nil {
				return fmt.Errorf("%s.%s has invalid inbound references", owner, field)
			}
			for _, value := range values {
				references = append(references, singboxInboundReference{
					Owner: owner + "." + field,
					Tag:   value,
				})
			}
			return nil
		}
		if raw, exists := options["detour"]; exists {
			if err := add("detour", raw); err != nil {
				return nil, err
			}
		}
		if stun, ok := options["stun"].(map[string]any); ok && stun != nil {
			if err := add("stun.detour", stun["detour"]); err != nil {
				return nil, err
			}
		}
		if raw, exists := options["verify_client_endpoint"]; exists {
			if err := add("verify_client_endpoint", raw); err != nil {
				return nil, err
			}
		}
		if raw, exists := options["servers"]; exists && raw != nil {
			servers, ok := raw.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("%s.servers has invalid inbound references", owner)
			}
			for path, target := range servers {
				if err := add("servers."+path, target); err != nil {
					return nil, err
				}
			}
		}
	}
	return references, nil
}

func collectSingboxServiceOutboundReferences(tx *gorm.DB, addReference func(string, string)) error {
	if tx == nil {
		return common.NewError("sing-box service reference collection requires a database transaction")
	}
	if addReference == nil {
		return nil
	}
	services := make([]model.Service, 0)
	if err := tx.Select("type", "tag", "options").Order("id ASC").Find(&services).Error; err != nil {
		return err
	}
	for _, service := range services {
		options := map[string]any{}
		if len(service.Options) > 0 && string(service.Options) != "null" {
			if err := json.Unmarshal(service.Options, &options); err != nil {
				return fmt.Errorf("decode sing-box service %q references: %w", service.Tag, err)
			}
		}
		owner := fmt.Sprintf("service %q", strings.TrimSpace(service.Tag))
		for _, field := range []string{"verify_client_url", "mesh_with"} {
			raw, exists := options[field]
			if !exists || raw == nil {
				continue
			}
			items, ok := raw.([]any)
			if !ok {
				return fmt.Errorf("%s.%s has invalid outbound references", owner, field)
			}
			for index, item := range items {
				entry, ok := item.(map[string]any)
				if !ok || entry == nil {
					return fmt.Errorf("%s.%s[%d] has invalid outbound references", owner, field, index)
				}
				if target, ok := entry["detour"].(string); ok {
					addReference(fmt.Sprintf("%s.%s[%d]", owner, field, index), target)
				} else if entry["detour"] != nil {
					return fmt.Errorf("%s.%s[%d].detour must be a string", owner, field, index)
				}
			}
		}
	}
	return nil
}

func validateSingboxServiceRemovalReferences(tx *gorm.DB, removedTags []string) error {
	removed := stringSet(removedTags)
	if len(removed) == 0 {
		return nil
	}
	servers := make([]model.DnsServer, 0)
	if err := tx.Select("tag", "type", "options").Order("id ASC").Find(&servers).Error; err != nil {
		return err
	}
	for _, server := range servers {
		if server.Type != "resolved" {
			continue
		}
		options := map[string]any{}
		if len(server.Options) > 0 && string(server.Options) != "null" {
			if err := json.Unmarshal(server.Options, &options); err != nil {
				return fmt.Errorf("decode DNS server %q references: %w", server.Tag, err)
			}
		}
		serviceTag, _ := options["service"].(string)
		serviceTag = strings.TrimSpace(serviceTag)
		if _, exists := removed[serviceTag]; exists {
			return fmt.Errorf("cannot remove sing-box service %q because DNS server %q still references it", serviceTag, server.Tag)
		}
	}
	return nil
}

func validateSingboxEndpointRemovalReferences(tx *gorm.DB, removedTags []string) error {
	removed := stringSet(removedTags)
	if len(removed) == 0 {
		return nil
	}
	servers := make([]model.DnsServer, 0)
	if err := tx.Select("tag", "type", "options").Order("id ASC").Find(&servers).Error; err != nil {
		return err
	}
	for _, server := range servers {
		if server.Type != "tailscale" {
			continue
		}
		options := map[string]any{}
		if len(server.Options) > 0 && string(server.Options) != "null" {
			if err := json.Unmarshal(server.Options, &options); err != nil {
				return fmt.Errorf("decode DNS server %q references: %w", server.Tag, err)
			}
		}
		endpointTag, _ := options["endpoint"].(string)
		endpointTag = strings.TrimSpace(endpointTag)
		if _, exists := removed[endpointTag]; exists {
			return fmt.Errorf("cannot remove sing-box endpoint %q because DNS server %q still references it", endpointTag, server.Tag)
		}
	}
	return nil
}
