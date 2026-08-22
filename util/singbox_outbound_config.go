package util

import "strings"

func ShouldSkipSingboxOutboundClientConfigKey(protocol string, key string, hasTLS bool) bool {
	return !allowsSingboxOutboundClientConfigKey(protocol, key, hasTLS)
}

func ShouldSkipMihomoOutboundClientConfigKey(protocol string, key string, hasTLS bool) bool {
	return !allowsMihomoOutboundClientConfigKey(protocol, key, hasTLS)
}

// ShouldSkipClashSubscriptionClientConfigKey applies the field boundary for
// Clash-only rendering. It intentionally differs from sing-box JSON for
// protocols such as Mieru that Mihomo supports but sing-box does not.
func ShouldSkipClashSubscriptionClientConfigKey(protocol string, key string, hasTLS bool) bool {
	return !allowsClashSubscriptionClientConfigKey(protocol, key, hasTLS)
}

// Client configuration is panel data, not a free-form outbound overlay.
// Keep this closed so UI metadata, legacy aliases, and unknown future fields
// cannot become invalid sing-box or Mihomo subscription parameters.
func allowsSingboxOutboundClientConfigKey(protocol string, key string, hasTLS bool) bool {
	protocol = normalizeSubscriptionType(protocol)
	key = strings.TrimSpace(key)

	switch protocol {
	case "mixed", "socks", "socks5", "http":
		return key == "username" || key == "password"
	case "vmess":
		return key == "uuid"
	case "vless":
		return key == "uuid" || (key == "flow" && hasTLS)
	case "trojan", "anytls", "hysteria2":
		return key == "password"
	case "naive":
		return key == "username" || key == "password"
	case "hysteria":
		return key == "auth_str"
	case "tuic":
		return key == "uuid" || key == "password"
	default:
		return false
	}
}

func allowsClashSubscriptionClientConfigKey(protocol string, key string, hasTLS bool) bool {
	if normalizeSubscriptionType(protocol) == "mieru" {
		key = strings.TrimSpace(key)
		return key == "username" || key == "password"
	}
	return allowsSingboxOutboundClientConfigKey(protocol, key, hasTLS)
}

func allowsMihomoOutboundClientConfigKey(protocol string, key string, hasTLS bool) bool {
	protocol = normalizeSubscriptionType(protocol)
	key = strings.TrimSpace(key)

	switch protocol {
	case "mixed", "socks", "socks5", "http", "naive", "anytls", "mieru", "trojan", "hysteria2", "trusttunnel", "shadowquic":
		return key == "username" || key == "password"
	case "vmess":
		return key == "username" || key == "uuid"
	case "vless":
		return key == "username" || key == "uuid" || (key == "flow" && hasTLS)
	case "hysteria":
		return key == "auth_str"
	case "tuic":
		return key == "uuid" || key == "password"
	case "snell":
		return key == "psk"
	default:
		return false
	}
}

func supportsSingboxOutboundUsername(protocol string) bool {
	return allowsSingboxOutboundClientConfigKey(protocol, "username", true)
}

func supportsMihomoOutboundUsername(protocol string) bool {
	return allowsMihomoOutboundClientConfigKey(protocol, "username", true)
}

// SubscriptionClientConfigUsername returns the canonical client username while
// accepting the historical name field used by older client records.
func SubscriptionClientConfigUsername(config map[string]interface{}) string {
	if config == nil {
		return ""
	}

	for _, key := range []string{"username", "name"} {
		if value := strings.TrimSpace(readStringValue(config[key])); value != "" {
			return value
		}
	}
	return ""
}

// StripSubscriptionOutboundPanelFields removes values that belong to panel
// records or list projections, never to a client outbound. It is intentionally
// small and explicit: protocol-specific outbound fields are handled by their
// own renderers instead of being accidentally discarded here.
func StripSubscriptionOutboundPanelFields(outbound map[string]interface{}) {
	if outbound == nil {
		return
	}

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
	} {
		delete(outbound, key)
	}
}

func SanitizeSingboxSubscriptionOutbound(outbound map[string]interface{}) {
	if outbound == nil {
		return
	}

	StripSubscriptionOutboundPanelFields(outbound)
	protocol, _ := outbound["type"].(string)
	PromoteHysteria2ReceiveWindowsToSingbox(outbound)
	if normalizeSubscriptionType(protocol) == "hysteria" {
		NormalizeHysteriaSubscriptionOutbound(outbound)
	}
	if !supportsSingboxOutboundUsername(protocol) {
		delete(outbound, "username")
	}
	delete(outbound, "name")
	delete(outbound, "alterId")
	stripMihomoOnlySubscriptionFields(outbound)
	sanitizeSingboxSubscriptionTransport(outbound)
}

func stripMihomoOnlySubscriptionFields(outbound map[string]interface{}) {
	if outbound == nil {
		return
	}

	for _, key := range []string{
		"mihomo_common",
		"mihomo_hy2",
		"mihomo_fast_open",
		"fast_open",
		"fast-open",
	} {
		delete(outbound, key)
	}

	tlsMap, ok := outbound["tls"].(map[string]interface{})
	if !ok || tlsMap == nil {
		return
	}
	for _, key := range []string{
		"mihomo_use_fingerprint",
		"fingerprint",
		"include_server_certificate",
		"include_server_fingerprint",
	} {
		delete(tlsMap, key)
	}
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
