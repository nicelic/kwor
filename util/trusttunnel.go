package util

import (
	"math"
	"strconv"
	"strings"
)

// ResolveTrustTunnelCredentials keeps compatibility with current and legacy configs.
func ResolveTrustTunnelCredentials(config map[string]interface{}, fallbackUsername ...string) (string, string) {
	if config == nil {
		return "", ""
	}

	firstNonEmpty := func(keys ...string) string {
		for _, key := range keys {
			value, _ := config[key].(string)
			value = strings.TrimSpace(value)
			if value != "" {
				return value
			}
		}
		return ""
	}

	fallback := ""
	if len(fallbackUsername) > 0 {
		fallback = strings.TrimSpace(fallbackUsername[0])
	}

	username := firstNonEmpty("username", "name")
	if username == "" {
		username = fallback
	}
	if username == "" {
		username = firstNonEmpty("uuid", "password")
	}
	password := firstNonEmpty("password", "uuid")
	if username == "" {
		username = password
	}
	if password == "" {
		password = username
	}

	return username, password
}

func ApplyTrustTunnelCredentials(outbound map[string]interface{}, config map[string]interface{}, fallbackUsername ...string) {
	if outbound == nil || config == nil {
		return
	}

	username, password := ResolveTrustTunnelCredentials(config, fallbackUsername...)
	if username == "" || password == "" {
		return
	}

	if existing, _ := outbound["username"].(string); strings.TrimSpace(existing) == "" {
		outbound["username"] = username
	}
	if existing, _ := outbound["password"].(string); strings.TrimSpace(existing) == "" {
		outbound["password"] = password
	}
}

func ResolveTrustTunnelUDP(config map[string]interface{}) (bool, bool) {
	if config == nil {
		return false, false
	}

	switch value := config["udp"].(type) {
	case bool:
		return value, true
	case string:
		parsed, err := strconv.ParseBool(strings.TrimSpace(value))
		if err == nil {
			return parsed, true
		}
	}

	if HasStringValue(config["network"], "udp") {
		return true, true
	}

	return false, false
}

func ApplyTrustTunnelReuseOptions(outbound map[string]interface{}, config map[string]interface{}) {
	if outbound == nil || config == nil {
		return
	}

	if value, ok := ResolveTrustTunnelReuseOption(config, "max-connections", "max_connections"); ok {
		outbound["max-connections"] = value
	}
	if value, ok := ResolveTrustTunnelReuseOption(config, "min-streams", "min_streams"); ok {
		outbound["min-streams"] = value
	}
	if value, ok := ResolveTrustTunnelReuseOption(config, "max-streams", "max_streams"); ok {
		outbound["max-streams"] = value
	}
}

func ResolveTrustTunnelReuseOption(config map[string]interface{}, keys ...string) (int, bool) {
	if config == nil || len(keys) == 0 {
		return 0, false
	}

	for _, key := range keys {
		parsed, ok := parseTrustTunnelNonNegativeInt(config[key])
		if ok {
			return parsed, true
		}
	}
	return 0, false
}

func parseTrustTunnelNonNegativeInt(raw interface{}) (int, bool) {
	switch value := raw.(type) {
	case int:
		if value >= 0 {
			return value, true
		}
	case int32:
		if value >= 0 {
			return int(value), true
		}
	case int64:
		if value >= 0 {
			return int(value), true
		}
	case float32:
		number := float64(value)
		if number >= 0 && number == math.Trunc(number) && number <= float64(math.MaxInt64) {
			return int(number), true
		}
	case float64:
		if value >= 0 && value == math.Trunc(value) && value <= float64(math.MaxInt64) {
			return int(value), true
		}
	case string:
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return 0, false
		}
		parsed, err := strconv.Atoi(trimmed)
		if err == nil && parsed >= 0 {
			return parsed, true
		}
	}

	return 0, false
}

func SanitizeTrustTunnelOutbound(outbound map[string]interface{}) {
	if outbound == nil {
		return
	}

	if udp, ok := ResolveTrustTunnelUDP(outbound); ok {
		outbound["udp"] = udp
	}
	if _, exists := outbound["health_check"]; !exists {
		switch value := outbound["health-check"].(type) {
		case bool:
			outbound["health_check"] = value
		case string:
			parsed, err := strconv.ParseBool(strings.TrimSpace(value))
			if err == nil {
				outbound["health_check"] = parsed
			}
		}
	}

	delete(outbound, "network")
	delete(outbound, "uuid")
	delete(outbound, "health-check")
}

func HasStringValue(raw interface{}, target string) bool {
	target = strings.ToLower(strings.TrimSpace(target))
	if target == "" {
		return false
	}

	switch value := raw.(type) {
	case string:
		return strings.ToLower(strings.TrimSpace(value)) == target
	case []string:
		for _, item := range value {
			if strings.ToLower(strings.TrimSpace(item)) == target {
				return true
			}
		}
	case []interface{}:
		for _, item := range value {
			itemString, ok := item.(string)
			if ok && strings.ToLower(strings.TrimSpace(itemString)) == target {
				return true
			}
		}
	}

	return false
}
