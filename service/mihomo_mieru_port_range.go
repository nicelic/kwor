package service

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/util"
)

const mihomoMieruPortRangeOptionKey = "port_range"

func normalizeMihomoMieruPortRangeValue(raw string) (string, error) {
	value := strings.TrimSpace(raw)
	if value == "" {
		return "", nil
	}
	value = strings.ReplaceAll(value, "\uFF0C", ",")
	if strings.Contains(value, ",") {
		return "", fmt.Errorf("mieru port range must be a single range segment")
	}
	normalized, ok := util.NormalizeMieruPortRange(value)
	if !ok {
		return "", fmt.Errorf("invalid mieru port range, expected one range like 400-500")
	}
	return normalized, nil
}

func decodeJSONMap(raw json.RawMessage) (map[string]interface{}, error) {
	if len(raw) == 0 {
		return map[string]interface{}{}, nil
	}
	payload := map[string]interface{}{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	if payload == nil {
		return map[string]interface{}{}, nil
	}
	return payload, nil
}

func encodeJSONMap(payload map[string]interface{}) (json.RawMessage, error) {
	if payload == nil {
		payload = map[string]interface{}{}
	}
	encoded, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return nil, err
	}
	return json.RawMessage(encoded), nil
}

func extractMihomoMieruPortRange(options json.RawMessage) string {
	payload, err := decodeJSONMap(options)
	if err != nil {
		return ""
	}
	raw := firstString(payload[mihomoMieruPortRangeOptionKey])
	normalized, err := normalizeMihomoMieruPortRangeValue(raw)
	if err != nil {
		return ""
	}
	return normalized
}

func extractMihomoMieruPortRangeFromOutJSON(outJSON json.RawMessage) string {
	payload, err := decodeJSONMap(outJSON)
	if err != nil {
		return ""
	}
	raw := firstString(payload[mihomoMieruPortRangeOptionKey])
	normalized, err := normalizeMihomoMieruPortRangeValue(raw)
	if err != nil {
		return ""
	}
	return normalized
}

func sanitizeMihomoMieruInboundPortRange(inbound *model.MihomoInbound) (string, error) {
	if inbound == nil || inbound.Type != "mieru" {
		return "", nil
	}

	options, err := decodeJSONMap(inbound.Options)
	if err != nil {
		return "", err
	}

	outJSON, err := decodeJSONMap(inbound.OutJson)
	if err != nil {
		return "", err
	}

	candidate := firstString(outJSON[mihomoMieruPortRangeOptionKey])
	if strings.TrimSpace(candidate) == "" {
		candidate = firstString(options[mihomoMieruPortRangeOptionKey])
	}

	normalizedRange, err := normalizeMihomoMieruPortRangeValue(candidate)
	if err != nil {
		return "", err
	}

	delete(options, "port_bindings")
	if normalizedRange == "" {
		delete(options, mihomoMieruPortRangeOptionKey)
		delete(outJSON, mihomoMieruPortRangeOptionKey)
	} else {
		options[mihomoMieruPortRangeOptionKey] = normalizedRange
		outJSON[mihomoMieruPortRangeOptionKey] = normalizedRange
	}

	if inbound.Options, err = encodeJSONMap(options); err != nil {
		return "", err
	}
	if inbound.OutJson, err = encodeJSONMap(outJSON); err != nil {
		return "", err
	}
	return normalizedRange, nil
}

func resolveMihomoInboundRedirectSpec(inbound *model.MihomoInbound) (string, bool) {
	if inbound == nil {
		return "", false
	}
	if inbound.Type == "mieru" {
		if portRange := extractMihomoMieruPortRange(inbound.Options); strings.TrimSpace(portRange) != "" {
			return portRange, true
		}
		return extractMihomoMieruPortRangeFromOutJSON(inbound.OutJson), true
	}
	return extractPortHopRange(inbound.Options), false
}
