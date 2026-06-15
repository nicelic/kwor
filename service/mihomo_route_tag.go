package service

import "encoding/json"

func deriveEffectiveMihomoInboundRouteTag(tag string, _ string, options map[string]interface{}) string {
	if detour := extractDetourFromOptions(options); detour != "" {
		return detour
	}
	return tag
}

func deriveEffectiveMihomoInboundRouteTagFromRaw(tag string, inboundType string, rawOptions json.RawMessage) string {
	if len(rawOptions) == 0 {
		return tag
	}

	var options map[string]interface{}
	if err := json.Unmarshal(rawOptions, &options); err != nil {
		return tag
	}

	return deriveEffectiveMihomoInboundRouteTag(tag, inboundType, options)
}
