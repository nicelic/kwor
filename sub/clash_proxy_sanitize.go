package sub

import "strings"

func sanitizeMihomoClashProxy(proxy map[string]interface{}) (map[string]interface{}, bool) {
	if proxy == nil {
		return nil, false
	}

	sanitized := cloneClashProxyMap(proxy)
	changed := false

	proxyType := strings.ToLower(strings.TrimSpace(firstString(sanitized["type"])))
	switch proxyType {
	case "tuic":
		// Preserve proxy fields exactly as the Clash renderer emitted them.
		// Only strip keys that are still known to be invalid for Mihomo YAML.
		if _, exists := sanitized["network"]; exists {
			delete(sanitized, "network")
			changed = true
		}
	case "hysteria2":
		if up, ok := toInt(sanitized["up"]); ok && up <= 0 {
			delete(sanitized, "up")
			changed = true
		}
		if down, ok := toInt(sanitized["down"]); ok && down <= 0 {
			delete(sanitized, "down")
			changed = true
		}
	}

	return sanitized, changed
}

func cloneClashProxyMap(src map[string]interface{}) map[string]interface{} {
	if src == nil {
		return nil
	}

	dst := make(map[string]interface{}, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}
