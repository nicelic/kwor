package util

import "strings"

func ShouldSkipSingboxOutboundClientConfigKey(protocol string, key string, hasTLS bool) bool {
	key = strings.TrimSpace(key)
	switch key {
	case "name", "alterId":
		return true
	case "flow":
		return !hasTLS
	case "username":
		return !supportsSingboxOutboundUsername(protocol)
	default:
		return false
	}
}

func ShouldSkipMihomoOutboundClientConfigKey(protocol string, key string, hasTLS bool) bool {
	key = strings.TrimSpace(key)
	switch key {
	case "name", "alterId":
		return true
	case "flow":
		return normalizeSubscriptionType(protocol) == "vless" && !hasTLS
	case "username":
		return !supportsMihomoOutboundUsername(protocol)
	default:
		return false
	}
}

func supportsSingboxOutboundUsername(protocol string) bool {
	switch normalizeSubscriptionType(protocol) {
	case "mixed", "socks", "http", "naive":
		return true
	default:
		return false
	}
}

func supportsMihomoOutboundUsername(protocol string) bool {
	switch normalizeSubscriptionType(protocol) {
	case "mixed", "socks", "http", "vmess", "vless", "trojan", "naive", "anytls", "hysteria2", "mieru", "trusttunnel":
		return true
	default:
		return false
	}
}

func SanitizeSingboxSubscriptionOutbound(outbound map[string]interface{}) {
	if outbound == nil {
		return
	}

	protocol, _ := outbound["type"].(string)
	if normalizeSubscriptionType(protocol) == "hysteria" {
		NormalizeHysteriaSubscriptionOutbound(outbound)
	}
	if !supportsSingboxOutboundUsername(protocol) {
		delete(outbound, "username")
	}
	delete(outbound, "name")
	sanitizeSingboxSubscriptionTransport(outbound)
}

func sanitizeSingboxSubscriptionTransport(outbound map[string]interface{}) {
	if outbound == nil {
		return
	}

	rawTransport, exists := outbound["transport"]
	if !exists {
		return
	}

	transport, ok := rawTransport.(map[string]interface{})
	if !ok || transport == nil {
		delete(outbound, "transport")
		return
	}

	transportType := strings.ToLower(strings.TrimSpace(readStringValue(transport["type"])))
	canonicalType := transportType
	switch transportType {
	case "":
		delete(outbound, "transport")
		return
	case "h2":
		// sing-box uses v2ray http transport for h2-style settings.
		canonicalType = "http"
	case "http", "ws", "grpc", "quic", "httpupgrade":
		// keep supported v2ray transport types
	case "xhttp":
		// xhttp is mihomo-only and is not accepted by sing-box transport options.
		delete(outbound, "transport")
		return
	default:
		delete(outbound, "transport")
		return
	}
	transport["type"] = canonicalType

	// Remove mihomo-only transport keys that are invalid in sing-box.
	delete(transport, "v2ray_http_upgrade")
	delete(transport, "v2ray_http_upgrade_fast_open")
	delete(transport, "grpc_user_agent")
	delete(transport, "ping_interval")
	delete(transport, "max_connections")
	delete(transport, "min_streams")
	delete(transport, "max_streams")
	delete(transport, "mode")
	delete(transport, "no_grpc_header")
	delete(transport, "no_sse_header")
	delete(transport, "x_padding_bytes")
	delete(transport, "sc_max_each_post_bytes")
	delete(transport, "sc_stream_up_server_secs")
	delete(transport, "reuse_settings")
	delete(transport, "download_settings")

	if strings.TrimSpace(readStringValue(transport["type"])) == "" {
		delete(outbound, "transport")
		return
	}
	outbound["transport"] = transport
}

func readStringValue(raw interface{}) string {
	value, ok := raw.(string)
	if !ok {
		return ""
	}
	return value
}
