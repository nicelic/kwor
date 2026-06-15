package service

import (
	"encoding/json"
	"strings"
)

type outboundMergeSchemaNode struct {
	Children map[string]*outboundMergeSchemaNode
}

func mergeLeaf() *outboundMergeSchemaNode {
	return &outboundMergeSchemaNode{}
}

func mergeBranch(children map[string]*outboundMergeSchemaNode) *outboundMergeSchemaNode {
	return &outboundMergeSchemaNode{Children: cloneMergeSchema(children)}
}

func cloneMergeSchema(src map[string]*outboundMergeSchemaNode) map[string]*outboundMergeSchemaNode {
	if len(src) == 0 {
		return nil
	}

	dst := make(map[string]*outboundMergeSchemaNode, len(src))
	for key, child := range src {
		if child == nil {
			dst[key] = nil
			continue
		}
		dst[key] = &outboundMergeSchemaNode{Children: cloneMergeSchema(child.Children)}
	}
	return dst
}

func mergeSchemaMaps(parts ...map[string]*outboundMergeSchemaNode) map[string]*outboundMergeSchemaNode {
	merged := make(map[string]*outboundMergeSchemaNode)
	for _, part := range parts {
		for key, node := range part {
			if node == nil {
				merged[key] = nil
				continue
			}
			merged[key] = &outboundMergeSchemaNode{Children: cloneMergeSchema(node.Children)}
		}
	}
	if len(merged) == 0 {
		return nil
	}
	return merged
}

func mergeSchemaLeaves(keys ...string) map[string]*outboundMergeSchemaNode {
	schema := make(map[string]*outboundMergeSchemaNode, len(keys))
	for _, key := range keys {
		schema[key] = mergeLeaf()
	}
	return schema
}

func mergeSchemaWithout(src map[string]*outboundMergeSchemaNode, keys ...string) map[string]*outboundMergeSchemaNode {
	cloned := cloneMergeSchema(src)
	for _, key := range keys {
		delete(cloned, key)
	}
	return cloned
}

var outboundTLSMergeSchema = mergeSchemaMaps(
	mergeSchemaLeaves(
		"enabled",
		"disable_sni",
		"server_name",
		"insecure",
		"alpn",
		"min_version",
		"max_version",
		"cipher_suites",
		"certificate",
		"certificate_path",
		"fragment",
		"fragment_fallback_delay",
		"record_fragment",
	),
	map[string]*outboundMergeSchemaNode{
		"utls":    mergeBranch(mergeSchemaLeaves("enabled", "fingerprint")),
		"reality": mergeBranch(mergeSchemaLeaves("enabled", "public_key", "short_id")),
		"ech": mergeBranch(mergeSchemaLeaves(
			"enabled",
			"pq_signature_schemes_enabled",
			"dynamic_record_sizing_disabled",
			"config",
			"config_path",
		)),
	},
)

var outboundTransportMergeSchema = mergeSchemaMaps(
	mergeSchemaLeaves(
		"type",
		"host",
		"path",
		"method",
		"headers",
		"mode",
		"idle_timeout",
		"ping_timeout",
		"ping_interval",
		"max_connections",
		"min_streams",
		"max_streams",
		"max_early_data",
		"early_data_header_name",
		"v2ray_http_upgrade",
		"v2ray_http_upgrade_fast_open",
		"service_name",
		"grpc_user_agent",
		"permit_without_stream",
		"no_grpc_header",
		"x_padding_bytes",
		"sc_max_each_post_bytes",
	),
	map[string]*outboundMergeSchemaNode{
		"reuse_settings": mergeBranch(mergeSchemaLeaves(
			"max_connections",
			"max_concurrency",
			"c_max_reuse_times",
			"h_max_request_times",
			"h_max_reusable_secs",
		)),
		"download_settings": mergeBranch(mergeSchemaMaps(
			mergeSchemaLeaves(
				"path",
				"host",
				"headers",
				"no_grpc_header",
				"x_padding_bytes",
				"sc_max_each_post_bytes",
				"server",
				"port",
				"tls",
				"alpn",
				"ech_opts",
				"reality_opts",
				"skip_cert_verify",
				"fingerprint",
				"certificate",
				"private_key",
				"servername",
				"client_fingerprint",
			),
			map[string]*outboundMergeSchemaNode{
				"reuse_settings": mergeBranch(mergeSchemaLeaves(
					"max_connections",
					"max_concurrency",
					"c_max_reuse_times",
					"h_max_request_times",
					"h_max_reusable_secs",
				)),
			},
		)),
	},
)

var outboundMultiplexMergeSchema = mergeSchemaMaps(
	mergeSchemaLeaves(
		"enabled",
		"protocol",
		"max_connections",
		"min_streams",
		"max_streams",
		"padding",
	),
	map[string]*outboundMergeSchemaNode{
		"brutal": mergeBranch(mergeSchemaLeaves("enabled", "up_mbps", "down_mbps")),
	},
)

var shadowTLSSSConfigMergeSchema = mergeSchemaMaps(
	mergeSchemaLeaves("method", "password", "network", "udp_over_tcp"),
	map[string]*outboundMergeSchemaNode{
		"multiplex": mergeBranch(outboundMultiplexMergeSchema),
	},
)

var hysteria2ObfsMergeSchema = mergeSchemaLeaves("type", "password")

var mihomoHY2MergeSchema = mergeSchemaLeaves(
	"initial_stream_receive_window",
	"max_stream_receive_window",
	"initial_connection_receive_window",
	"max_connection_receive_window",
)

var defaultDialMergeSchema = mergeSchemaLeaves(
	"detour",
	"bind_interface",
	"inet4_bind_address",
	"inet6_bind_address",
	"routing_mark",
	"reuse_addr",
	"connect_timeout",
	"tcp_fast_open",
	"tcp_multi_path",
	"udp_fragment",
	"fallback_delay",
	"domain_resolver",
)

var mihomoDialMergeSchema = mergeSchemaLeaves(
	"detour",
	"bind_interface",
	"routing_mark",
	"tcp_fast_open",
	"tcp_multi_path",
	"fallback_delay",
)

func outboundEditableMergeSchema(namespace string, outType string) map[string]*outboundMergeSchemaNode {
	schema := mergeSchemaLeaves("type", "tag")

	switch strings.TrimSpace(namespace) {
	case "mihomo":
		schema = mergeSchemaMaps(schema, mihomoEditableMergeSchema(outType))
	default:
		schema = mergeSchemaMaps(schema, defaultEditableMergeSchema(outType))
	}

	return schema
}

func defaultEditableMergeSchema(outType string) map[string]*outboundMergeSchemaNode {
	schema := protocolEditableMergeSchema(outType)
	if supportsDialMergeSchema(outType) {
		schema = mergeSchemaMaps(schema, defaultDialMergeSchema)
	}
	if supportsTLSMergeSchema(outType) {
		schema = mergeSchemaMaps(schema, map[string]*outboundMergeSchemaNode{
			"tls": mergeBranch(outboundTLSMergeSchema),
		})
	}
	return schema
}

func mihomoEditableMergeSchema(outType string) map[string]*outboundMergeSchemaNode {
	schema := protocolEditableMergeSchema(outType)
	if supportsDialMergeSchema(outType) {
		schema = mergeSchemaMaps(schema, mihomoDialMergeSchema)
	}
	if supportsTLSMergeSchema(outType) {
		tlsSchema := cloneMergeSchema(outboundTLSMergeSchema)
		delete(tlsSchema, "utls")
		if supportsMihomoClientFingerprint(outType) {
			tlsSchema["utls"] = mergeBranch(mergeSchemaLeaves("enabled", "fingerprint"))
		}
		if outType == "anytls" {
			delete(tlsSchema, "reality")
		}
		schema = mergeSchemaMaps(schema, map[string]*outboundMergeSchemaNode{
			"tls": mergeBranch(tlsSchema),
		})
	}

	switch strings.TrimSpace(outType) {
	case "selector":
		delete(schema, "default")
		delete(schema, "interrupt_exist_connections")
	case "urltest":
		delete(schema, "idle_timeout")
		delete(schema, "interrupt_exist_connections")
	}

	return schema
}

func protocolEditableMergeSchema(outType string) map[string]*outboundMergeSchemaNode {
	switch strings.TrimSpace(outType) {
	case "direct":
		return nil
	case "socks":
		return mergeSchemaLeaves("server", "server_port", "version", "username", "password", "network")
	case "http":
		return mergeSchemaLeaves("server", "server_port", "username", "password", "path", "headers")
	case "shadowsocks":
		return mergeSchemaMaps(
			mergeSchemaLeaves("server", "server_port", "method", "password", "network"),
			map[string]*outboundMergeSchemaNode{
				"multiplex": mergeBranch(outboundMultiplexMergeSchema),
			},
		)
	case "vmess":
		return mergeSchemaMaps(
			mergeSchemaLeaves(
				"server",
				"server_port",
				"uuid",
				"security",
				"alter_id",
				"global_padding",
				"authenticated_length",
				"network",
				"packet_encoding",
			),
			map[string]*outboundMergeSchemaNode{
				"multiplex": mergeBranch(outboundMultiplexMergeSchema),
				"transport": mergeBranch(outboundTransportMergeSchema),
			},
		)
	case "trojan":
		return mergeSchemaMaps(
			mergeSchemaLeaves("server", "server_port", "password", "network"),
			map[string]*outboundMergeSchemaNode{
				"multiplex": mergeBranch(outboundMultiplexMergeSchema),
				"transport": mergeBranch(outboundTransportMergeSchema),
			},
		)
	case "hysteria":
		return mergeSchemaLeaves(
			"server",
			"server_port",
			"up_mbps",
			"down_mbps",
			"obfs",
			"auth_str",
			"stream_receive_window",
			"connection_receive_window",
			"max_concurrent_streams",
			"disable_path_mtu_discovery",
			"network",
			"mihomo_fast_open",
			"port_hop_interval",
		)
	case "shadowtls":
		return mergeSchemaMaps(
			mergeSchemaLeaves("server", "server_port", "version", "password", "strict_mode", "wildcard_sni"),
			map[string]*outboundMergeSchemaNode{
				"handshake": mergeBranch(mergeSchemaLeaves("server", "server_port")),
				"ss_config": mergeBranch(shadowTLSSSConfigMergeSchema),
			},
		)
	case "vless":
		return mergeSchemaMaps(
			mergeSchemaLeaves("server", "server_port", "uuid", "flow", "network", "packet_encoding"),
			map[string]*outboundMergeSchemaNode{
				"multiplex": mergeBranch(outboundMultiplexMergeSchema),
				"transport": mergeBranch(outboundTransportMergeSchema),
			},
		)
	case "tuic":
		return mergeSchemaLeaves(
			"server",
			"server_port",
			"uuid",
			"password",
			"mihomo_fast_open",
			"congestion_control",
			"udp_relay_mode",
			"zero_rtt_handshake",
			"heartbeat",
			"network",
		)
	case "hysteria2":
		return mergeSchemaMaps(
			mergeSchemaLeaves(
				"server",
				"server_port",
				"server_ports",
				"hop_interval",
				"hop_interval_max",
				"mihomo_fast_open",
				"up_mbps",
				"down_mbps",
				"password",
				"network",
				"bbr_profile",
				"brutal_debug",
				"port_hop_interval",
			),
			map[string]*outboundMergeSchemaNode{
				"obfs":       mergeBranch(hysteria2ObfsMergeSchema),
				"mihomo_hy2": mergeBranch(mihomoHY2MergeSchema),
			},
		)
	case "anytls":
		return mergeSchemaLeaves(
			"server",
			"server_port",
			"password",
			"idle_session_check_interval",
			"idle_session_timeout",
			"min_idle_session",
		)
	case "mieru":
		return mergeSchemaLeaves(
			"server",
			"server_port",
			"port_range",
			"transport",
			"udp",
			"username",
			"password",
			"multiplexing",
			"handshake_mode",
		)
	case "sudoku":
		return mergeSchemaMaps(
			mergeSchemaLeaves(
				"server",
				"server_port",
				"key",
				"aead_method",
				"padding_min",
				"padding_max",
				"table_type",
				"custom_table",
				"custom_tables",
				"enable_pure_downlink",
			),
			map[string]*outboundMergeSchemaNode{
				"httpmask": mergeBranch(mergeSchemaLeaves(
					"disable",
					"mode",
					"tls",
					"host",
					"path_root",
					"multiplex",
				)),
			},
		)
	case "trusttunnel":
		return mergeSchemaLeaves(
			"server",
			"server_port",
			"username",
			"password",
			"udp",
			"health_check",
			"congestion_controller",
			"quic",
			"max_connections",
			"min_streams",
			"max_streams",
		)
	case "tor":
		return mergeSchemaLeaves("executable_path", "extra_args", "data_directory", "torrc")
	case "ssh":
		return mergeSchemaLeaves(
			"server",
			"server_port",
			"user",
			"password",
			"private_key",
			"private_key_path",
			"private_key_passphrase",
			"host_key",
			"host_key_algorithms",
			"client_version",
		)
	case "selector":
		return mergeSchemaLeaves("outbounds", "default", "interrupt_exist_connections")
	case "urltest":
		return mergeSchemaLeaves("outbounds", "url", "interval", "tolerance", "idle_timeout", "interrupt_exist_connections")
	default:
		return nil
	}
}

func supportsTLSMergeSchema(outType string) bool {
	switch strings.TrimSpace(outType) {
	case "http", "vmess", "trojan", "hysteria", "shadowtls", "vless", "tuic", "hysteria2", "anytls", "trusttunnel":
		return true
	default:
		return false
	}
}

func supportsDialMergeSchema(outType string) bool {
	switch strings.TrimSpace(outType) {
	case "selector", "urltest":
		return false
	default:
		return true
	}
}

func supportsMihomoClientFingerprint(outType string) bool {
	switch strings.TrimSpace(outType) {
	case "vmess", "vless", "trojan", "anytls", "shadowtls":
		return true
	default:
		return false
	}
}

func mergeEditableOutboundRawPayload(existingRaw json.RawMessage, incomingRaw json.RawMessage, namespace string, outType string) json.RawMessage {
	editedPayload, ok := decodeOutboundPayloadMap(incomingRaw)
	if !ok {
		return normalizeRawOutboundPayloadWithoutID(incomingRaw)
	}

	if len(existingRaw) == 0 {
		normalized, err := json.Marshal(editedPayload)
		if err != nil {
			return normalizeRawOutboundPayloadWithoutID(incomingRaw)
		}
		return normalized
	}

	basePayload, ok := decodeOutboundPayloadMap(existingRaw)
	if !ok {
		normalized, err := json.Marshal(editedPayload)
		if err != nil {
			return normalizeRawOutboundPayloadWithoutID(incomingRaw)
		}
		return normalized
	}

	merged := mergeEditableOutboundMap(basePayload, editedPayload, outboundEditableMergeSchema(namespace, outType))
	normalized, err := json.Marshal(merged)
	if err != nil {
		return normalizeRawOutboundPayloadWithoutID(incomingRaw)
	}
	return normalized
}

func normalizeRawOutboundPayloadWithoutID(raw json.RawMessage) json.RawMessage {
	payload, ok := decodeOutboundPayloadMap(raw)
	if !ok {
		return append(json.RawMessage(nil), raw...)
	}
	normalized, err := json.Marshal(payload)
	if err != nil {
		return append(json.RawMessage(nil), raw...)
	}
	return normalized
}

func decodeOutboundPayloadMap(raw json.RawMessage) (map[string]interface{}, bool) {
	if len(raw) == 0 {
		return nil, false
	}

	payload := map[string]interface{}{}
	if err := json.Unmarshal(raw, &payload); err != nil || payload == nil {
		return nil, false
	}
	delete(payload, "id")
	return payload, true
}

func mergeEditableOutboundMap(
	base map[string]interface{},
	edited map[string]interface{},
	schema map[string]*outboundMergeSchemaNode,
) map[string]interface{} {
	result := cloneJSONObject(base)

	for key, value := range edited {
		if key == "id" {
			continue
		}
		if _, exists := schema[key]; exists {
			continue
		}
		result[key] = cloneJSONValue(value)
	}

	for key, node := range schema {
		editedValue, hasEdited := edited[key]
		if node == nil || len(node.Children) == 0 {
			if hasEdited {
				result[key] = cloneJSONValue(editedValue)
			} else {
				delete(result, key)
			}
			continue
		}

		if !hasEdited {
			delete(result, key)
			continue
		}

		editedMap, ok := editedValue.(map[string]interface{})
		if !ok || editedMap == nil {
			result[key] = cloneJSONValue(editedValue)
			continue
		}

		baseMap, _ := base[key].(map[string]interface{})
		mergedChild := mergeEditableOutboundMap(baseMap, editedMap, node.Children)
		if len(mergedChild) == 0 {
			delete(result, key)
			continue
		}
		result[key] = mergedChild
	}

	return result
}

func cloneJSONObject(src map[string]interface{}) map[string]interface{} {
	if src == nil {
		return map[string]interface{}{}
	}

	dst := make(map[string]interface{}, len(src))
	for key, value := range src {
		if key == "id" {
			continue
		}
		dst[key] = cloneJSONValue(value)
	}
	return dst
}

func cloneJSONValue(value interface{}) interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		return cloneJSONObject(typed)
	case []interface{}:
		cloned := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			cloned = append(cloned, cloneJSONValue(item))
		}
		return cloned
	case []string:
		cloned := make([]string, len(typed))
		copy(cloned, typed)
		return cloned
	case json.RawMessage:
		return append(json.RawMessage(nil), typed...)
	default:
		return typed
	}
}

func buildMergedClashProxyOptions(
	mergedOutboundRaw json.RawMessage,
	existingClashOptions json.RawMessage,
	tag string,
) json.RawMessage {
	existingProxy, _ := decodeArbitraryJSONMap(existingClashOptions)

	outboundMap, ok := decodeArbitraryJSONMap(mergedOutboundRaw)
	if !ok || outboundMap == nil {
		if existingProxy == nil {
			return nil
		}
		existingProxy["name"] = strings.TrimSpace(tag)
		return encodeClashProxyOptions(existingProxy)
	}

	result := convertMihomoOutboundsToClash([]map[string]interface{}{outboundMap})
	if result == nil || len(result.Proxies) == 0 {
		if existingProxy == nil {
			return nil
		}
		existingProxy["name"] = strings.TrimSpace(tag)
		return encodeClashProxyOptions(existingProxy)
	}

	proxy := cloneMap(result.Proxies[0])
	if proxy == nil {
		return nil
	}
	if existingProxy != nil {
		mergeImportedMihomoRawClashProxy(proxy, existingProxy)
	}
	if strings.TrimSpace(tag) != "" {
		proxy["name"] = strings.TrimSpace(tag)
	}
	return encodeClashProxyOptions(proxy)
}

func decodeArbitraryJSONMap(raw json.RawMessage) (map[string]interface{}, bool) {
	if len(raw) == 0 {
		return nil, false
	}

	payload := map[string]interface{}{}
	if err := json.Unmarshal(raw, &payload); err != nil || payload == nil {
		return nil, false
	}
	return payload, true
}
