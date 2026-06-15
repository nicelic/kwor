package util

import (
	"encoding/json"
	"net"
	"strconv"
	"strings"
)

// ShadowTLSVersion normalizes a loosely-typed version value from JSON/UI payloads.
func ShadowTLSVersion(raw interface{}) int {
	switch value := raw.(type) {
	case int:
		return value
	case int32:
		return int(value)
	case int64:
		return int(value)
	case float32:
		return int(value)
	case float64:
		return int(value)
	case json.Number:
		if parsed, err := value.Int64(); err == nil {
			return int(parsed)
		}
	case string:
		value = strings.TrimSpace(value)
		if value == "" {
			return 0
		}
		if parsed, err := strconv.Atoi(value); err == nil {
			return parsed
		}
	}
	return 0
}

// ResolveShadowTLSClientPassword follows ShadowTLS version semantics:
// - v1: no password
// - v2: use inbound global password
// - v3: use client-scoped password
func ResolveShadowTLSClientPassword(version int, clientConfigs map[string]interface{}, inboundOptions json.RawMessage) (string, bool) {
	switch version {
	case 1:
		return "", false
	case 2:
		if password, ok := shadowTLSPasswordFromInboundOptions(inboundOptions); ok {
			return password, true
		}
	}

	return shadowTLSPasswordFromClientConfigs(clientConfigs)
}

// BuildShadowTLSClientPair converts one UI-facing ShadowTLS outbound into the runtime pair used by subscriptions.
func BuildShadowTLSClientPair(outJSON map[string]interface{}, clientConfigs map[string]interface{}, inboundOptions json.RawMessage) (map[string]interface{}, map[string]interface{}) {
	tag, _ := outJSON["tag"].(string)
	if strings.TrimSpace(tag) == "" {
		return nil, nil
	}

	version := ShadowTLSVersion(outJSON["version"])
	ssConfig, hasSSConfig := outJSON["ss_config"].(map[string]interface{})
	stlsTag := tag + "-out"

	stlsOutbound := map[string]interface{}{
		"type":        "shadowtls",
		"tag":         stlsTag,
		"server":      outJSON["server"],
		"server_port": outJSON["server_port"],
		"version":     outJSON["version"],
	}
	if password, ok := ResolveShadowTLSClientPassword(version, clientConfigs, inboundOptions); ok {
		stlsOutbound["password"] = password
	}
	if tls, ok := outJSON["tls"]; ok {
		stlsOutbound["tls"] = tls
	}

	if !hasSSConfig || ssConfig == nil {
		return nil, stlsOutbound
	}

	ssOutbound := map[string]interface{}{
		"type":   "shadowsocks",
		"tag":    tag,
		"detour": stlsTag,
	}
	copyShadowTLSSSConfig(ssOutbound, ssConfig, true)

	return ssOutbound, stlsOutbound
}

// BuildShadowTLSRuntimeOutboundPairMap converts one stored ShadowTLS outbound into runtime outbounds.
func BuildShadowTLSRuntimeOutboundPairMap(outboundData map[string]interface{}, preserveDisabledMultiplex bool) (map[string]interface{}, map[string]interface{}) {
	if outboundData == nil {
		return nil, nil
	}

	stlsOutbound := cloneShadowTLSMap(outboundData)
	delete(stlsOutbound, "ss_config")
	StripShadowTLSInboundOnlyFields(stlsOutbound)

	ssConfig, hasSSConfig := outboundData["ss_config"].(map[string]interface{})
	if !hasSSConfig || ssConfig == nil {
		return nil, stlsOutbound
	}

	tag, _ := stlsOutbound["tag"].(string)
	if strings.TrimSpace(tag) == "" {
		return nil, stlsOutbound
	}

	stlsTag := tag + "-out"
	stlsOutbound["tag"] = stlsTag

	ssOutbound := map[string]interface{}{
		"type":   "shadowsocks",
		"tag":    tag,
		"detour": stlsTag,
	}
	copyShadowTLSSSConfig(ssOutbound, ssConfig, preserveDisabledMultiplex)

	return ssOutbound, stlsOutbound
}

// BuildShadowTLSRuntimeOutboundPairJSON is the JSON counterpart of BuildShadowTLSRuntimeOutboundPairMap.
func BuildShadowTLSRuntimeOutboundPairJSON(outboundJSON []byte, preserveDisabledMultiplex bool) (json.RawMessage, json.RawMessage, error) {
	outboundData := map[string]interface{}{}
	if err := json.Unmarshal(outboundJSON, &outboundData); err != nil {
		return nil, nil, err
	}

	ssOutbound, stlsOutbound := BuildShadowTLSRuntimeOutboundPairMap(outboundData, preserveDisabledMultiplex)

	var ssJSON json.RawMessage
	if ssOutbound != nil {
		raw, err := json.Marshal(ssOutbound)
		if err != nil {
			return nil, nil, err
		}
		ssJSON = raw
	}

	var stlsJSON json.RawMessage
	if stlsOutbound != nil {
		raw, err := json.Marshal(stlsOutbound)
		if err != nil {
			return nil, nil, err
		}
		stlsJSON = raw
	}

	return ssJSON, stlsJSON, nil
}

func StripShadowTLSInboundOnlyFields(outbound map[string]interface{}) {
	if outbound == nil {
		return
	}
	delete(outbound, "handshake")
	delete(outbound, "handshake_for_server_name")
	delete(outbound, "strict_mode")
	delete(outbound, "wildcard_sni")
}

func DeriveShadowTLSPluginHost(shadowTLSOutbound map[string]interface{}) string {
	if tlsMap, ok := shadowTLSOutbound["tls"].(map[string]interface{}); ok && tlsMap != nil {
		if serverName, ok := tlsMap["server_name"].(string); ok && strings.TrimSpace(serverName) != "" {
			return strings.TrimSpace(serverName)
		}
	}

	if handshakeMap, ok := shadowTLSOutbound["handshake"].(map[string]interface{}); ok && handshakeMap != nil {
		if server, ok := handshakeMap["server"].(string); ok && strings.TrimSpace(server) != "" {
			return strings.TrimSpace(server)
		}
		if dest, ok := handshakeMap["dest"].(string); ok && strings.TrimSpace(dest) != "" {
			if host, ok := splitHostFromAddress(strings.TrimSpace(dest)); ok {
				return host
			}
			return strings.TrimSpace(dest)
		}
	}

	if server, ok := shadowTLSOutbound["server"].(string); ok && strings.TrimSpace(server) != "" {
		return strings.TrimSpace(server)
	}
	return ""
}

func shadowTLSPasswordFromInboundOptions(inboundOptions json.RawMessage) (string, bool) {
	if len(inboundOptions) == 0 {
		return "", false
	}

	options := map[string]interface{}{}
	if err := json.Unmarshal(inboundOptions, &options); err != nil {
		return "", false
	}

	password, _ := options["password"].(string)
	password = strings.TrimSpace(password)
	if password == "" {
		return "", false
	}

	return password, true
}

func shadowTLSPasswordFromClientConfigs(clientConfigs map[string]interface{}) (string, bool) {
	shadowTLSConfig, _ := clientConfigs["shadowtls"].(map[string]interface{})
	if shadowTLSConfig == nil {
		return "", false
	}

	password, _ := shadowTLSConfig["password"].(string)
	password = strings.TrimSpace(password)
	if password == "" {
		return "", false
	}

	return password, true
}

func copyShadowTLSSSConfig(dst map[string]interface{}, ssConfig map[string]interface{}, preserveDisabledMultiplex bool) {
	if dst == nil || ssConfig == nil {
		return
	}

	if method, ok := ssConfig["method"]; ok && method != nil {
		dst["method"] = method
	}
	if network, ok := ssConfig["network"]; ok && network != nil && network != "" {
		dst["network"] = network
	}
	if password, ok := ssConfig["password"]; ok && password != nil {
		dst["password"] = password
	}
	if udp, ok := ssConfig["udp"]; ok {
		dst["udp"] = udp
	}
	if ipVersion, ok := ssConfig["ip_version"]; ok && ipVersion != nil && ipVersion != "" {
		dst["ip_version"] = ipVersion
	}
	if routingMark, ok := ssConfig["routing_mark"]; ok {
		dst["routing_mark"] = routingMark
	}
	if tcpFastOpen, ok := ssConfig["tcp_fast_open"]; ok {
		dst["tcp_fast_open"] = tcpFastOpen
	}
	if tcpMultiPath, ok := ssConfig["tcp_multi_path"]; ok {
		dst["tcp_multi_path"] = tcpMultiPath
	}
	if mihomoCommon, ok := ssConfig["mihomo_common"].(map[string]interface{}); ok && mihomoCommon != nil {
		dst["mihomo_common"] = cloneShadowTLSMap(mihomoCommon)
	}
	if udpOverTCP, ok := ssConfig["udp_over_tcp"]; ok && udpOverTCP != nil {
		dst["udp_over_tcp"] = udpOverTCP
	}
	if multiplex, ok := ssConfig["multiplex"].(map[string]interface{}); ok && multiplex != nil {
		if preserveDisabledMultiplex || shadowTLSMultiplexEnabled(multiplex) {
			dst["multiplex"] = multiplex
		}
	}
}

func shadowTLSMultiplexEnabled(multiplex map[string]interface{}) bool {
	enabled, ok := multiplex["enabled"].(bool)
	return ok && enabled
}

func cloneShadowTLSMap(src map[string]interface{}) map[string]interface{} {
	dst := make(map[string]interface{}, len(src))
	for key, value := range src {
		dst[key] = value
	}
	return dst
}

func splitHostFromAddress(address string) (string, bool) {
	if address == "" {
		return "", false
	}

	if host, _, err := net.SplitHostPort(address); err == nil {
		host = strings.Trim(host, "[]")
		host = strings.TrimSpace(host)
		if host != "" {
			return host, true
		}
	}

	if strings.Count(address, ":") == 1 {
		if host, _, ok := strings.Cut(address, ":"); ok {
			host = strings.TrimSpace(host)
			if host != "" {
				return host, true
			}
		}
	}

	return "", false
}
