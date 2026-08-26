package sub

import (
	"strings"

	"github.com/alireza0/s-ui/util"
)

func sanitizeMihomoClashProxy(proxy map[string]interface{}) (map[string]interface{}, bool) {
	if proxy == nil {
		return nil, false
	}

	sanitized := cloneClashProxyMap(proxy)
	changed := stripSubscriptionClashProxyPanelFields(sanitized)

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
	case "shadowquic":
		// ShadowQUIC only supports its official native fields. Rebuild every
		// stored/imported Clash proxy so TLS and generic dial fields cannot be
		// replayed from a raw YAML snippet. This is already a final Clash proxy,
		// so use the Clash-specific sanitizer instead of the raw-outbound one.
		name := strings.TrimSpace(firstString(sanitized["name"]))
		normalized, ok := util.SanitizeMihomoShadowQUICClashProxy(sanitized, name)
		if !ok {
			return nil, true
		}
		return normalized, true
	}

	return sanitized, changed
}

func stripSubscriptionClashProxyPanelFields(proxy map[string]interface{}) bool {
	if proxy == nil {
		return false
	}

	changed := false
	for _, key := range []string{
		"id",
		"tls_id",
		"route_tag",
		"user_management",
		"metadata",
		"users",
		"inbounds",
		"addrs",
		"out_json",
		"options",
		"source_type",
		"source_client_id",
		"source_inbound_id",
		"server_port",
		"alter_id",
		"tls_store",
		"mihomo_common",
		"mihomo_hy2",
		"mihomo_fast_open",
		"fast_open",
	} {
		if _, exists := proxy[key]; exists {
			delete(proxy, key)
			changed = true
		}
	}
	return changed
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
