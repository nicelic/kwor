package util

import (
	"fmt"
	"strconv"
	"strings"
)

const (
	DefaultMieruTransport     = "TCP"
	DefaultMieruMultiplexing  = "MULTIPLEXING_LOW"
	DefaultMieruHandshakeMode = "HANDSHAKE_STANDARD"
)

var mieruMultiplexingLevels = map[string]struct{}{
	"MULTIPLEXING_OFF":    {},
	"MULTIPLEXING_LOW":    {},
	"MULTIPLEXING_MIDDLE": {},
	"MULTIPLEXING_HIGH":   {},
}

var mieruHandshakeModes = map[string]struct{}{
	"HANDSHAKE_STANDARD": {},
	"HANDSHAKE_NO_WAIT":  {},
}

func NormalizeMieruTransport(raw string) string {
	switch strings.ToUpper(strings.TrimSpace(raw)) {
	case "UDP":
		return "UDP"
	default:
		return DefaultMieruTransport
	}
}

func NormalizeMieruMultiplexing(raw string) string {
	value := strings.ToUpper(strings.TrimSpace(raw))
	if _, ok := mieruMultiplexingLevels[value]; ok {
		return value
	}
	return DefaultMieruMultiplexing
}

func NormalizeMieruHandshakeMode(raw string) string {
	value := strings.ToUpper(strings.TrimSpace(raw))
	if _, ok := mieruHandshakeModes[value]; ok {
		return value
	}
	return DefaultMieruHandshakeMode
}

func NormalizeMieruPortBinding(raw string) (string, bool) {
	value := strings.TrimSpace(raw)
	value = strings.ReplaceAll(value, "\uFF1A", ":")
	value = strings.ReplaceAll(value, ":", "-")
	if value == "" {
		return "", false
	}

	if strings.Contains(value, "-") {
		parts := strings.Split(value, "-")
		if len(parts) != 2 {
			return "", false
		}
		begin, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil || begin < 1 || begin > 65535 {
			return "", false
		}
		end, err := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil || end < 1 || end > 65535 || begin > end {
			return "", false
		}
		if begin == end {
			return strconv.Itoa(begin), true
		}
		return fmt.Sprintf("%d-%d", begin, end), true
	}

	port, err := strconv.Atoi(value)
	if err != nil || port < 1 || port > 65535 {
		return "", false
	}
	return strconv.Itoa(port), true
}

// NormalizeMieruPortRange normalizes one mieru port range segment.
// Valid examples: "2090-2099", "2090:2099", "2090：2099".
// Single ports are rejected.
func NormalizeMieruPortRange(raw string) (string, bool) {
	normalized, ok := NormalizeMieruPortBinding(raw)
	if !ok || !strings.Contains(normalized, "-") {
		return "", false
	}
	return normalized, true
}

func NormalizeMieruPortBindings(raw string) []string {
	if raw == "" {
		return nil
	}

	raw = strings.ReplaceAll(raw, "\uFF0C", ",")
	parts := strings.Split(raw, ",")
	normalized := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, part := range parts {
		binding, ok := NormalizeMieruPortBinding(part)
		if !ok {
			continue
		}
		if _, exists := seen[binding]; exists {
			continue
		}
		seen[binding] = struct{}{}
		normalized = append(normalized, binding)
	}
	return normalized
}

func NormalizeMieruPortBindingsValue(raw interface{}) []string {
	switch value := raw.(type) {
	case string:
		return NormalizeMieruPortBindings(value)
	case []string:
		return NormalizeMieruPortBindings(strings.Join(value, ","))
	case []interface{}:
		parts := make([]string, 0, len(value))
		for _, item := range value {
			itemStr, ok := item.(string)
			if !ok {
				continue
			}
			parts = append(parts, itemStr)
		}
		return NormalizeMieruPortBindings(strings.Join(parts, ","))
	default:
		return nil
	}
}

func MieruPrimaryPortFromBinding(binding string) (int, bool) {
	normalized, ok := NormalizeMieruPortBinding(binding)
	if !ok {
		return 0, false
	}
	first := normalized
	if idx := strings.IndexByte(normalized, '-'); idx >= 0 {
		first = normalized[:idx]
	}
	port, err := strconv.Atoi(first)
	if err != nil || port < 1 || port > 65535 {
		return 0, false
	}
	return port, true
}

func NormalizeMieruOutboundBindings(outbound map[string]interface{}) []string {
	if outbound == nil {
		return nil
	}

	if bindings := NormalizeMieruPortBindingsValue(outbound["port_bindings"]); len(bindings) > 0 {
		return bindings
	}

	if portRange, ok := outbound["port_range"].(string); ok {
		if binding, ok := NormalizeMieruPortBinding(portRange); ok {
			return []string{binding}
		}
	}

	if port, ok := toIntValue(outbound["server_port"]); ok && port > 0 {
		return []string{strconv.Itoa(port)}
	}

	return nil
}

func BuildMieruBindingQueryValue(binding string) string {
	normalized, ok := NormalizeMieruPortBinding(binding)
	if !ok {
		return ""
	}
	return normalized
}

func toIntValue(raw interface{}) (int, bool) {
	switch value := raw.(type) {
	case int:
		return value, true
	case int32:
		return int(value), true
	case int64:
		return int(value), true
	case float32:
		return int(value), true
	case float64:
		return int(value), true
	case string:
		value = strings.TrimSpace(value)
		if value == "" {
			return 0, false
		}
		port, err := strconv.Atoi(value)
		if err == nil {
			return port, true
		}
	}
	return 0, false
}
