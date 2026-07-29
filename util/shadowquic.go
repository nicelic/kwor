package util

import (
	"fmt"
	"math"
	"net"
	"strconv"
	"strings"
)

// ValidateMihomoShadowQUICJLSUpstreamAddr validates the only required
// ShadowQUIC listener protocol field. net.SplitHostPort deliberately keeps
// IPv6 bracket rules aligned with the address syntax Mihomo accepts.
func ValidateMihomoShadowQUICJLSUpstreamAddr(raw string) error {
	addr := strings.TrimSpace(raw)
	if addr == "" {
		return fmt.Errorf("shadowquic jls-upstream.addr is required")
	}

	host, portText, err := net.SplitHostPort(addr)
	if err != nil || strings.TrimSpace(host) == "" {
		return fmt.Errorf("shadowquic jls-upstream.addr must be host:port or [IPv6]:port")
	}
	if strings.HasPrefix(addr, "[") {
		ip := net.ParseIP(host)
		if ip == nil || !strings.Contains(host, ":") {
			return fmt.Errorf("shadowquic jls-upstream.addr must use a valid bracketed IPv6 address")
		}
	}
	if portText == "" || strings.Trim(portText, "0123456789") != "" {
		return fmt.Errorf("shadowquic jls-upstream.addr port must be between 1 and 65535")
	}
	port, err := strconv.Atoi(portText)
	if err != nil || port < 1 || port > 65535 {
		return fmt.Errorf("shadowquic jls-upstream.addr port must be between 1 and 65535")
	}
	return nil
}

// SanitizeMihomoShadowQUICOutbound keeps the internal raw outbound limited to
// official ShadowQUIC client fields plus the supported Mihomo common fields.
// It intentionally has no defaults: a missing optional field remains missing
// all the way to the rendered YAML.
func SanitizeMihomoShadowQUICOutbound(outbound map[string]interface{}) {
	if outbound == nil {
		return
	}

	clean := map[string]interface{}{
		"type": "shadowquic",
	}
	copyShadowQUICString(outbound, clean, "tag", "tag", "name")
	copyShadowQUICString(outbound, clean, "server", "server")
	if port, ok := shadowQUICNonNegativeInt(firstShadowQUICValue(outbound, "server_port", "server-port", "port")); ok && port > 0 {
		clean["server_port"] = port
	}
	copyShadowQUICString(outbound, clean, "username", "username")
	copyShadowQUICString(outbound, clean, "password", "password")

	copyShadowQUICString(outbound, clean, "sni", "sni")
	copyShadowQUICStringSlice(outbound, clean, "alpn", "alpn")
	copyShadowQUICStringSlice(outbound, clean, "quic_versions", "quic_versions", "quic-versions")
	copyShadowQUICBool(outbound, clean, "udp_over_stream", "udp_over_stream", "udp-over-stream")
	copyShadowQUICBool(outbound, clean, "zero_rtt", "zero_rtt", "zero-rtt")
	copyShadowQUICNonNegativeInt(outbound, clean, "keep_alive_interval", "keep_alive_interval", "keep-alive-interval")
	copyShadowQUICString(outbound, clean, "congestion_controller", "congestion_controller", "congestion-controller")
	copyShadowQUICBandwidth(outbound, clean, "up", "up")
	copyShadowQUICBandwidth(outbound, clean, "down", "down")
	copyShadowQUICNonNegativeInt(outbound, clean, "cwnd", "cwnd")

	if bbrProfile, hasBBRProfile := NormalizeMihomoBBRProfile(firstShadowQUICValue(outbound, "bbr_profile", "bbr-profile")); hasBBRProfile {
		clean["bbr_profile"] = bbrProfile
	}
	copyShadowQUICNonNegativeInt(outbound, clean, "max_datagram_frame_size", "max_datagram_frame_size", "max-datagram-frame-size")
	copyShadowQUICNonNegativeInt(outbound, clean, "max_open_streams", "max_open_streams", "max-open-streams")
	copyShadowQUICNonNegativeInt(outbound, clean, "recv_window_conn", "recv_window_conn", "recv-window-conn")
	copyShadowQUICNonNegativeInt(outbound, clean, "recv_window", "recv_window", "recv-window")
	copyShadowQUICBool(outbound, clean, "disable_mtu_discovery", "disable_mtu_discovery", "disable-mtu-discovery")
	if common := sanitizeMihomoShadowQUICCommonFields(outbound["mihomo_common"]); len(common) > 0 {
		clean["mihomo_common"] = common
	}

	for key := range outbound {
		delete(outbound, key)
	}
	for key, value := range clean {
		outbound[key] = value
	}
}

// ValidateMihomoShadowQUICOutbound checks the required fields of a
// ShadowQUIC client payload after applying the protocol's closed schema.
// fallbackTag is used by persisted records whose tag is stored outside the
// raw payload.
func ValidateMihomoShadowQUICOutbound(source map[string]interface{}, fallbackTag string) error {
	if source == nil {
		return fmt.Errorf("shadowquic outbound is required")
	}

	normalized := cloneShadowQUICMap(source)
	SanitizeMihomoShadowQUICOutbound(normalized)
	return validateMihomoShadowQUICOutbound(normalized, fallbackTag)
}

// SanitizeMihomoShadowQUICInboundTemplate limits an inbound's reusable
// client template to transport fields. Authentication is client-specific and
// must be supplied from mihomo_clients.config.shadowquic, never inherited
// from an inbound out_json value.
func SanitizeMihomoShadowQUICInboundTemplate(outbound map[string]interface{}) {
	SanitizeMihomoShadowQUICOutbound(outbound)
	if outbound == nil {
		return
	}
	delete(outbound, "username")
	delete(outbound, "password")
}

// BuildMihomoShadowQUICClashProxy converts a raw outbound into the exact
// Mihomo proxy schema. It never emits TLS or generic dial fields.
func BuildMihomoShadowQUICClashProxy(source map[string]interface{}, tag string) (map[string]interface{}, bool) {
	if source == nil {
		return nil, false
	}

	normalized := cloneShadowQUICMap(source)
	SanitizeMihomoShadowQUICOutbound(normalized)
	if err := validateMihomoShadowQUICOutbound(normalized, tag); err != nil {
		return nil, false
	}

	tag = strings.TrimSpace(tag)
	if tag == "" {
		tag = strings.TrimSpace(shadowQUICString(normalized["tag"]))
	}
	server := strings.TrimSpace(shadowQUICString(normalized["server"]))
	serverPort, _ := shadowQUICNonNegativeInt(normalized["server_port"])
	username := strings.TrimSpace(shadowQUICString(normalized["username"]))
	password := strings.TrimSpace(shadowQUICString(normalized["password"]))

	proxy := map[string]interface{}{
		"name":     tag,
		"type":     "shadowquic",
		"server":   server,
		"port":     serverPort,
		"username": username,
		"password": password,
	}

	copyShadowQUICProxyString(normalized, proxy, "sni", "sni")
	copyShadowQUICProxyStringSlice(normalized, proxy, "alpn", "alpn")
	copyShadowQUICProxyStringSlice(normalized, proxy, "quic-versions", "quic_versions")
	copyShadowQUICProxyBool(normalized, proxy, "udp-over-stream", "udp_over_stream")
	copyShadowQUICProxyBool(normalized, proxy, "zero-rtt", "zero_rtt")
	copyShadowQUICProxyNonNegativeInt(normalized, proxy, "keep-alive-interval", "keep_alive_interval")
	copyShadowQUICProxyString(normalized, proxy, "congestion-controller", "congestion_controller")
	copyShadowQUICProxyString(normalized, proxy, "up", "up")
	copyShadowQUICProxyString(normalized, proxy, "down", "down")
	copyShadowQUICProxyNonNegativeInt(normalized, proxy, "cwnd", "cwnd")
	copyShadowQUICProxyString(normalized, proxy, "bbr-profile", "bbr_profile")
	copyShadowQUICProxyNonNegativeInt(normalized, proxy, "max-datagram-frame-size", "max_datagram_frame_size")
	copyShadowQUICProxyNonNegativeInt(normalized, proxy, "max-open-streams", "max_open_streams")
	copyShadowQUICProxyNonNegativeInt(normalized, proxy, "recv-window-conn", "recv_window_conn")
	copyShadowQUICProxyNonNegativeInt(normalized, proxy, "recv-window", "recv_window")
	copyShadowQUICProxyBool(normalized, proxy, "disable-mtu-discovery", "disable_mtu_discovery")
	applyMihomoShadowQUICCommonFields(proxy, normalized["mihomo_common"])

	return proxy, true
}

// NormalizeMihomoShadowQUICALPN keeps only the ALPN values exposed by the
// ShadowQUIC listener editor.
func NormalizeMihomoShadowQUICALPN(raw interface{}) []string {
	allowed := map[string]struct{}{
		"h3":       {},
		"h2":       {},
		"http/1.1": {},
	}
	values := shadowQUICStringSlice(raw)
	result := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := allowed[value]; ok {
			result = append(result, value)
		}
	}
	return result
}

// NormalizeMihomoShadowQUICVersions keeps the supported QUIC versions in
// source order. Mihomo accepts both v1 and v2 in the same listener profile.
func NormalizeMihomoShadowQUICVersions(raw interface{}) []string {
	values := shadowQUICStringSlice(raw)
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		switch value {
		case "v1", "v2":
			if _, exists := seen[value]; exists {
				continue
			}
			seen[value] = struct{}{}
			result = append(result, value)
		}
	}
	return result
}

// NormalizeMihomoShadowQUICCongestionController accepts the algorithms
// documented by Mihomo for ShadowQUIC.
func NormalizeMihomoShadowQUICCongestionController(raw interface{}) (string, bool) {
	value := strings.ToLower(strings.TrimSpace(shadowQUICString(raw)))
	switch value {
	case "cubic", "new_reno", "bbr":
		return value, true
	default:
		return "", false
	}
}

func validateMihomoShadowQUICOutbound(normalized map[string]interface{}, fallbackTag string) error {
	tag := strings.TrimSpace(fallbackTag)
	if tag == "" {
		tag = strings.TrimSpace(shadowQUICString(normalized["tag"]))
	}
	if tag == "" {
		return fmt.Errorf("shadowquic tag is required")
	}

	if server := strings.TrimSpace(shadowQUICString(normalized["server"])); server == "" {
		return fmt.Errorf("shadowquic server is required")
	}

	serverPort, portOK := shadowQUICNonNegativeInt(normalized["server_port"])
	if !portOK || serverPort < 1 || serverPort > 65535 {
		return fmt.Errorf("shadowquic server_port must be between 1 and 65535")
	}

	if username := strings.TrimSpace(shadowQUICString(normalized["username"])); username == "" {
		return fmt.Errorf("shadowquic username is required")
	}
	if password := strings.TrimSpace(shadowQUICString(normalized["password"])); password == "" {
		return fmt.Errorf("shadowquic password is required")
	}
	return nil
}

func cloneShadowQUICMap(src map[string]interface{}) map[string]interface{} {
	dst := make(map[string]interface{}, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func firstShadowQUICValue(source map[string]interface{}, keys ...string) interface{} {
	for _, key := range keys {
		if value, ok := source[key]; ok {
			return value
		}
	}
	return nil
}

func shadowQUICString(raw interface{}) string {
	value, _ := raw.(string)
	return value
}

func shadowQUICStringSlice(raw interface{}) []string {
	values := make([]string, 0)
	switch typed := raw.(type) {
	case []string:
		values = append(values, typed...)
	case []interface{}:
		for _, item := range typed {
			if value, ok := item.(string); ok {
				values = append(values, value)
			}
		}
	case string:
		values = append(values, strings.FieldsFunc(typed, func(r rune) bool { return r == ',' || r == '\n' })...)
	}

	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func shadowQUICNonNegativeInt(raw interface{}) (int, bool) {
	var value int64
	switch typed := raw.(type) {
	case int:
		value = int64(typed)
	case int8:
		value = int64(typed)
	case int16:
		value = int64(typed)
	case int32:
		value = int64(typed)
	case int64:
		value = typed
	case uint:
		if uint64(typed) > uint64(math.MaxInt) {
			return 0, false
		}
		value = int64(typed)
	case uint32:
		value = int64(typed)
	case uint64:
		if typed > uint64(math.MaxInt) {
			return 0, false
		}
		value = int64(typed)
	case float64:
		if math.Trunc(typed) != typed || typed > float64(math.MaxInt) || typed < 0 {
			return 0, false
		}
		value = int64(typed)
	case float32:
		if math.Trunc(float64(typed)) != float64(typed) || typed < 0 {
			return 0, false
		}
		value = int64(typed)
	case string:
		parsed, err := strconv.ParseInt(strings.TrimSpace(typed), 10, 0)
		if err != nil {
			return 0, false
		}
		value = parsed
	default:
		return 0, false
	}
	if value < 0 || value > int64(math.MaxInt) {
		return 0, false
	}
	return int(value), true
}

func copyShadowQUICString(source, target map[string]interface{}, targetKey string, sourceKeys ...string) {
	if value := strings.TrimSpace(shadowQUICString(firstShadowQUICValue(source, sourceKeys...))); value != "" {
		target[targetKey] = value
	}
}

func copyShadowQUICStringSlice(source, target map[string]interface{}, targetKey string, sourceKeys ...string) {
	if values := shadowQUICStringSlice(firstShadowQUICValue(source, sourceKeys...)); len(values) > 0 {
		target[targetKey] = values
	}
}

func copyShadowQUICBool(source, target map[string]interface{}, targetKey string, sourceKeys ...string) {
	if value, ok := firstShadowQUICValue(source, sourceKeys...).(bool); ok {
		target[targetKey] = value
	}
}

func copyShadowQUICNonNegativeInt(source, target map[string]interface{}, targetKey string, sourceKeys ...string) {
	if value, ok := shadowQUICNonNegativeInt(firstShadowQUICValue(source, sourceKeys...)); ok {
		target[targetKey] = value
	}
}

func copyShadowQUICBandwidth(source, target map[string]interface{}, targetKey string, sourceKeys ...string) {
	raw := firstShadowQUICValue(source, sourceKeys...)
	var value string
	switch typed := raw.(type) {
	case string:
		value = strings.TrimSpace(typed)
	case int, int8, int16, int32, int64, uint, uint32, uint64, float32, float64:
		value = strings.TrimSpace(fmt.Sprint(typed))
	}
	if value != "" {
		target[targetKey] = value
	}
}

func sanitizeMihomoShadowQUICCommonFields(raw interface{}) map[string]interface{} {
	common, ok := raw.(map[string]interface{})
	if !ok || common == nil {
		return nil
	}

	clean := map[string]interface{}{}
	if udp, ok := common["udp"].(bool); ok {
		clean["udp"] = udp
	}
	if ipVersion := strings.TrimSpace(shadowQUICString(common["ip_version"])); ipVersion != "" {
		clean["ip_version"] = ipVersion
	}
	if routingMark, ok := shadowQUICNonNegativeInt(common["routing_mark"]); ok {
		clean["routing_mark"] = routingMark
	}
	return clean
}

func applyMihomoShadowQUICCommonFields(proxy map[string]interface{}, raw interface{}) {
	if proxy == nil {
		return
	}
	common, ok := raw.(map[string]interface{})
	if !ok || common == nil {
		return
	}
	if udp, ok := common["udp"].(bool); ok {
		proxy["udp"] = udp
	}
	if ipVersion := strings.TrimSpace(shadowQUICString(common["ip_version"])); ipVersion != "" {
		proxy["ip-version"] = ipVersion
	}
	if routingMark, ok := shadowQUICNonNegativeInt(common["routing_mark"]); ok {
		proxy["routing-mark"] = routingMark
	}
}

func copyShadowQUICProxyString(source, target map[string]interface{}, targetKey string, sourceKeys ...string) {
	copyShadowQUICString(source, target, targetKey, sourceKeys...)
}

func copyShadowQUICProxyStringSlice(source, target map[string]interface{}, targetKey string, sourceKeys ...string) {
	copyShadowQUICStringSlice(source, target, targetKey, sourceKeys...)
}

func copyShadowQUICProxyBool(source, target map[string]interface{}, targetKey string, sourceKeys ...string) {
	copyShadowQUICBool(source, target, targetKey, sourceKeys...)
}

func copyShadowQUICProxyNonNegativeInt(source, target map[string]interface{}, targetKey string, sourceKeys ...string) {
	copyShadowQUICNonNegativeInt(source, target, targetKey, sourceKeys...)
}
