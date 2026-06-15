package service

import (
	"strconv"
	"strings"
)

var mihomoSupportedDNSListKeys = []string{
	"direct-nameserver",
	"proxy-server-nameserver",
	"nameserver",
	"default-nameserver",
	"fallback",
}

func sanitizeMihomoDNSConfig(raw interface{}) map[string]interface{} {
	dnsMap, ok := raw.(map[string]interface{})
	if !ok || dnsMap == nil {
		return nil
	}

	sanitized := make(map[string]interface{})
	for _, key := range mihomoSupportedDNSListKeys {
		values := sanitizeMihomoDNSStringList(dnsMap[key])
		if len(values) == 0 {
			continue
		}
		sanitized[key] = values
	}

	if len(sanitized) == 0 {
		return nil
	}

	if ipv6, ok := toBool(dnsMap["ipv6"]); ok {
		sanitized["ipv6"] = ipv6
	} else {
		sanitized["ipv6"] = false
	}

	if preferH3, ok := toBool(dnsMap["prefer-h3"]); ok {
		sanitized["prefer-h3"] = preferH3
	} else {
		sanitized["prefer-h3"] = false
	}

	if sanitized["ipv6"] == true {
		if ipv6Timeout, ok := sanitizeMihomoDNSIPv6Timeout(dnsMap["ipv6-timeout"]); ok {
			sanitized["ipv6-timeout"] = ipv6Timeout
		}
	}

	return sanitized
}

func buildMihomoDNSDocument(raw interface{}) map[string]interface{} {
	dns := sanitizeMihomoDNSConfig(raw)
	if len(dns) == 0 {
		return nil
	}
	dns["enable"] = true
	return dns
}

func sanitizeMihomoDNSStringList(raw interface{}) []string {
	var source []string

	switch value := raw.(type) {
	case []string:
		source = value
	case []interface{}:
		source = make([]string, 0, len(value))
		for _, item := range value {
			if text, ok := item.(string); ok {
				source = append(source, text)
			}
		}
	case string:
		source = []string{value}
	default:
		return nil
	}

	result := make([]string, 0, len(source))
	seen := make(map[string]struct{}, len(source))
	for _, item := range source {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}

	if len(result) == 0 {
		return nil
	}
	return result
}

func sanitizeMihomoDNSIPv6Timeout(raw interface{}) (int, bool) {
	switch value := raw.(type) {
	case int:
		if value > 0 {
			return value, true
		}
	case int32:
		if value > 0 {
			return int(value), true
		}
	case int64:
		if value > 0 {
			return int(value), true
		}
	case float32:
		normalized := int(value)
		if float32(normalized) == value && normalized > 0 {
			return normalized, true
		}
	case float64:
		normalized := int(value)
		if float64(normalized) == value && normalized > 0 {
			return normalized, true
		}
	case string:
		normalized := strings.TrimSpace(strings.ToLower(value))
		if normalized == "" {
			return 0, false
		}
		normalized = strings.ReplaceAll(normalized, " ", "")
		if strings.HasSuffix(normalized, "ms") {
			normalized = strings.TrimSuffix(normalized, "ms")
		}
		if normalized == "" {
			return 0, false
		}
		parsed, err := strconv.Atoi(normalized)
		if err == nil && parsed > 0 {
			return parsed, true
		}
	}

	return 0, false
}
