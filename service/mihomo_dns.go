package service

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	maxMihomoDNSAddressesPerList = 8
	maxMihomoDNSAddressesTotal   = 32
	maxMihomoDNSAddressBytes     = 1024
	maxMihomoDNSAddressesBytes   = 16 * 1024
)

var mihomoSupportedDNSListKeys = []string{
	"direct-nameserver",
	"proxy-server-nameserver",
	"nameserver",
	"default-nameserver",
	"fallback",
}

// validateMihomoDNSConfig keeps DNS input bounded before it reaches config
// rendering. It deliberately accepts all Mihomo URI schemes so newer Core
// versions are not rejected by the panel, while still rejecting malformed URI
// shapes that are known to make a generated configuration unusable.
func validateMihomoDNSConfig(raw interface{}) error {
	dnsMap, ok := raw.(map[string]interface{})
	if !ok || dnsMap == nil {
		return fmt.Errorf("DNS 配置必须是对象")
	}

	totalAddresses := 0
	totalBytes := 0
	for _, key := range mihomoSupportedDNSListKeys {
		values, err := validatedMihomoDNSStringList(key, dnsMap[key])
		if err != nil {
			return err
		}
		if len(values) > maxMihomoDNSAddressesPerList {
			return fmt.Errorf("%s 最多允许 %d 个 DNS 地址", key, maxMihomoDNSAddressesPerList)
		}
		for _, value := range values {
			totalAddresses++
			totalBytes += len(value)
			if totalAddresses > maxMihomoDNSAddressesTotal {
				return fmt.Errorf("DNS 地址总数不能超过 %d", maxMihomoDNSAddressesTotal)
			}
			if totalBytes > maxMihomoDNSAddressesBytes {
				return fmt.Errorf("DNS 地址总大小不能超过 %d 字节", maxMihomoDNSAddressesBytes)
			}
		}
	}
	return nil
}

func validatedMihomoDNSStringList(key string, raw interface{}) ([]string, error) {
	if raw == nil {
		return nil, nil
	}

	var source []string
	switch value := raw.(type) {
	case []string:
		source = value
	case []interface{}:
		source = make([]string, 0, len(value))
		for index, item := range value {
			text, ok := item.(string)
			if !ok {
				return nil, fmt.Errorf("%s 的第 %d 项必须是字符串", key, index+1)
			}
			source = append(source, text)
		}
	case string:
		source = []string{value}
	default:
		return nil, fmt.Errorf("%s 必须是字符串或字符串数组", key)
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
		if err := validateMihomoDNSAddress(trimmed); err != nil {
			return nil, fmt.Errorf("%s: %w", key, err)
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	return result, nil
}

func validateMihomoDNSAddress(value string) error {
	if !utf8.ValidString(value) {
		return fmt.Errorf("DNS 地址必须是有效 UTF-8 文本")
	}
	if len(value) > maxMihomoDNSAddressBytes {
		return fmt.Errorf("单个 DNS 地址不能超过 %d 字节", maxMihomoDNSAddressBytes)
	}
	if strings.IndexFunc(value, func(r rune) bool {
		return unicode.IsControl(r) || unicode.IsSpace(r)
	}) >= 0 {
		return fmt.Errorf("DNS 地址不能包含空白或控制字符")
	}

	if !strings.Contains(value, "://") {
		return nil
	}
	parsed, err := url.Parse(value)
	if err != nil || strings.TrimSpace(parsed.Scheme) == "" || strings.TrimSpace(parsed.Host) == "" {
		return fmt.Errorf("DNS URI 格式无效")
	}
	return nil
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
	}

	if preferH3, ok := toBool(dnsMap["prefer-h3"]); ok {
		sanitized["prefer-h3"] = preferH3
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
	if value, ok := toInt(raw); ok && value > 0 {
		return value, true
	}

	value, ok := raw.(string)
	if !ok {
		return 0, false
	}
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

	return 0, false
}
