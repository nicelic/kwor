package service

import (
	"fmt"
	"strings"

	"github.com/alireza0/s-ui/util"
)

func normalizeMihomoListenerCompatFields(listener map[string]interface{}, listenerType string) {
	if listener == nil {
		return
	}

	switch listenerType {
	case "anytls":
		normalizeMihomoAnyTLSListener(listener)
	case "hysteria2":
		normalizeMihomoHysteria2Listener(listener)
	case "snell":
		normalizeMihomoSnellListener(listener)
	case "tuic":
		normalizeMihomoTUICListener(listener)
	case "shadowtls":
		normalizeMihomoShadowTLSListener(listener)
	case "shadowsocks":
		normalizeMihomoShadowsocksListener(listener)
	case "vmess":
		normalizeMihomoVMessListener(listener)
	case "vless":
		normalizeMihomoVLESSListener(listener)
	case "trojan":
		normalizeMihomoTrojanListener(listener)
	case "mieru":
		normalizeMihomoMieruListener(listener)
	case "sudoku":
		normalizeMihomoSudokuListener(listener)
	case "trusttunnel":
		normalizeMihomoTrustTunnelListener(listener)
	case "tun":
		normalizeMihomoTunListener(listener)
	case "tproxy":
		normalizeMihomoTProxyListener(listener)
	}

	switch listenerType {
	case "vmess", "vless", "trojan", "shadowsocks", "hysteria2", "tuic":
		normalizeMihomoListenerMuxOption(listener)
	}

	if listenerType != "tun" {
		delete(listener, "udp_timeout")
	}
	delete(listener, "tcp_fast_open")
	delete(listener, "tcp_multi_path")
	delete(listener, "udp_fragment")
	delete(listener, "managed")
	if listenerType != "sudoku" {
		delete(listener, "fallback")
	}
}

func normalizeMihomoAnyTLSListener(listener map[string]interface{}) {
	rawPadding := listener["padding-scheme"]
	if rawPadding == nil {
		rawPadding = listener["padding_scheme"]
	}
	if normalized := normalizeAnyTLSPaddingScheme(rawPadding); normalized != "" {
		listener["padding-scheme"] = normalized
	}
	delete(listener, "padding_scheme")
}

func normalizeMihomoSnellListener(listener map[string]interface{}) {
	if version, ok := toInt(listener["version"]); ok && version >= 4 && version <= 5 {
		listener["version"] = version
	} else {
		listener["version"] = 5
	}

	if udp, ok := toBool(listener["udp"]); ok {
		listener["udp"] = udp
	} else {
		listener["udp"] = true
	}

	rawObfs := listener["obfs_opts"]
	if rawObfs == nil {
		rawObfs = listener["obfs-opts"]
	}

	obfsOpts, ok := rawObfs.(map[string]interface{})
	if !ok || obfsOpts == nil {
		delete(listener, "obfs_opts")
		delete(listener, "obfs-opts")
		return
	}

	mode := strings.TrimSpace(firstString(obfsOpts["mode"]))
	if mode == "" {
		delete(listener, "obfs_opts")
		delete(listener, "obfs-opts")
		return
	}

	host := strings.TrimSpace(firstString(obfsOpts["host"]))
	if host == "" {
		host = "www.bing.com"
	}

	listener["obfs-opts"] = map[string]interface{}{
		"mode": mode,
		"host": host,
	}
	delete(listener, "obfs_opts")
}

func normalizeMihomoHysteria2Listener(listener map[string]interface{}) {
	if up := normalizeBandwidthString(listener["up"]); up != "" {
		listener["up"] = up
	} else if up := normalizeBandwidthString(listener["up_mbps"]); up != "" {
		listener["up"] = up
	}
	if down := normalizeBandwidthString(listener["down"]); down != "" {
		listener["down"] = down
	} else if down := normalizeBandwidthString(listener["down_mbps"]); down != "" {
		listener["down"] = down
	}
	if value, ok := toBool(listener["ignore-client-bandwidth"]); ok {
		listener["ignore-client-bandwidth"] = value
	} else if value, ok := toBool(listener["ignore_client_bandwidth"]); ok {
		listener["ignore-client-bandwidth"] = value
	}

	if obfsMap, ok := listener["obfs"].(map[string]interface{}); ok && obfsMap != nil {
		if value := strings.TrimSpace(firstString(obfsMap["type"])); value != "" {
			listener["obfs"] = value
		} else {
			delete(listener, "obfs")
		}
		if value := strings.TrimSpace(firstString(obfsMap["password"])); value != "" {
			listener["obfs-password"] = value
		} else {
			delete(listener, "obfs-password")
		}
	}

	if ms, ok := durationToMilliseconds(listener["max_idle_time"]); ok && ms > 0 {
		listener["max-idle-time"] = ms
	}

	if masquerade := normalizeMihomoMasquerade(listener["masquerade"]); masquerade != "" {
		listener["masquerade"] = masquerade
	} else if _, isMap := listener["masquerade"].(map[string]interface{}); isMap {
		delete(listener, "masquerade")
	}

	normalizeMihomoHysteria2ReceiveWindows(listener, listener)
	if legacy, ok := listener["mihomo_hy2"].(map[string]interface{}); ok && legacy != nil {
		normalizeMihomoHysteria2ReceiveWindows(listener, legacy)
	}

	delete(listener, "up_mbps")
	delete(listener, "down_mbps")
	delete(listener, "ignore_client_bandwidth")
	delete(listener, "max_idle_time")
	delete(listener, "mihomo_hy2")
}

func normalizeMihomoTUICListener(listener map[string]interface{}) {
	if value := strings.TrimSpace(firstString(listener["congestion-controller"])); value != "" {
		listener["congestion-controller"] = value
	} else if value := strings.TrimSpace(firstString(listener["congestion_control"])); value != "" {
		listener["congestion-controller"] = value
	}
	if ms, ok := durationToMilliseconds(listener["authentication-timeout"]); ok && ms > 0 {
		listener["authentication-timeout"] = ms
	} else if ms, ok := durationToMilliseconds(listener["auth_timeout"]); ok && ms > 0 {
		listener["authentication-timeout"] = ms
	}
	if ms, ok := durationToMilliseconds(listener["max_idle_time"]); ok && ms > 0 {
		listener["max-idle-time"] = ms
	}
	if packetSize, ok := toInt(listener["max_udp_relay_packet_size"]); ok && packetSize > 0 {
		listener["max-udp-relay-packet-size"] = packetSize
	}

	delete(listener, "congestion_control")
	delete(listener, "auth_timeout")
	delete(listener, "heartbeat")
	delete(listener, "zero_rtt_handshake")
	delete(listener, "network")
	delete(listener, "udp_relay_mode")
	delete(listener, "udp_over_stream")
	delete(listener, "max_idle_time")
	delete(listener, "max_udp_relay_packet_size")
}

func normalizeMihomoTrustTunnelListener(listener map[string]interface{}) {
	if value := strings.TrimSpace(firstString(listener["congestion-controller"])); value != "" {
		listener["congestion-controller"] = value
	} else if value := strings.TrimSpace(firstString(listener["congestion_controller"])); value != "" {
		listener["congestion-controller"] = value
	} else if value := strings.TrimSpace(firstString(listener["congestion_control"])); value != "" {
		listener["congestion-controller"] = value
	}

	network := make([]string, 0, 2)
	seen := map[string]struct{}{}
	for _, value := range toStringSlice(listener["network"]) {
		value = strings.ToLower(strings.TrimSpace(value))
		if value != "tcp" && value != "udp" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		network = append(network, value)
	}
	if len(network) > 0 {
		listener["network"] = network
	} else {
		delete(listener, "network")
	}

	delete(listener, "congestion_controller")
	delete(listener, "congestion_control")
}

func normalizeMihomoShadowsocksListener(listener map[string]interface{}) {
	if cipher := strings.TrimSpace(firstString(listener["cipher"])); cipher != "" {
		listener["cipher"] = cipher
	} else if method := strings.TrimSpace(firstString(listener["method"])); method != "" {
		listener["cipher"] = method
	}

	network := strings.ToLower(strings.TrimSpace(firstString(listener["network"])))
	switch network {
	case "tcp":
		listener["udp"] = false
	case "udp", "":
		if network != "" {
			listener["udp"] = true
		}
	}

	delete(listener, "method")
	delete(listener, "network")
}

func normalizeMihomoShadowTLSListener(listener map[string]interface{}) {
	shadowTLSPassword := strings.TrimSpace(firstString(listener["password"]))
	ssConfig, _ := listener["ss_config"].(map[string]interface{})
	if ssConfig != nil {
		if cipher := strings.TrimSpace(firstString(ssConfig["cipher"])); cipher != "" {
			listener["cipher"] = cipher
		} else if method := strings.TrimSpace(firstString(ssConfig["method"])); method != "" {
			listener["method"] = method
		}
		if password := strings.TrimSpace(firstString(ssConfig["password"])); password != "" {
			listener["password"] = password
		}
		if multiplex, ok := ssConfig["multiplex"].(map[string]interface{}); ok && multiplex != nil {
			listener["multiplex"] = multiplex
		}
	}

	shadowTLS := map[string]interface{}{
		"enable": true,
	}
	if version, ok := toInt(listener["version"]); ok && version > 0 {
		shadowTLS["version"] = version
	}
	if shadowTLSPassword != "" {
		shadowTLS["password"] = shadowTLSPassword
	}
	if users, ok := listener["users"].([]interface{}); ok && len(users) > 0 {
		shadowTLS["users"] = users
	}
	if handshake := normalizeMihomoShadowTLSHandshake(listener["handshake"]); len(handshake) > 0 {
		shadowTLS["handshake"] = handshake
	}

	listener["type"] = "shadowsocks"
	listener["shadow-tls"] = shadowTLS
	delete(listener, "version")
	delete(listener, "users")
	delete(listener, "handshake")
	delete(listener, "handshake_for_server_name")
	delete(listener, "strict_mode")
	delete(listener, "wildcard_sni")
	delete(listener, "ss_config")

	normalizeMihomoShadowsocksListener(listener)
	normalizeMihomoListenerMuxOption(listener)
}

func normalizeMihomoShadowTLSHandshake(raw interface{}) map[string]interface{} {
	handshake, ok := raw.(map[string]interface{})
	if !ok || handshake == nil {
		return nil
	}

	normalized := map[string]interface{}{}
	if dest := strings.TrimSpace(firstString(handshake["dest"])); dest != "" {
		normalized["dest"] = dest
	} else {
		server := strings.TrimSpace(firstString(handshake["server"]))
		serverPort, _ := toInt(handshake["server_port"])
		if server != "" && serverPort > 0 {
			normalized["dest"] = fmt.Sprintf("%s:%d", server, serverPort)
		} else if server != "" {
			normalized["dest"] = server
		}
	}

	if len(normalized) == 0 {
		return nil
	}
	return normalized
}

func normalizeMihomoShadowTLSHandshakeForServerName(raw interface{}) map[string]interface{} {
	handshakeMap, ok := raw.(map[string]interface{})
	if !ok || handshakeMap == nil {
		return nil
	}

	normalized := map[string]interface{}{}
	for serverName, value := range handshakeMap {
		handshake := normalizeMihomoShadowTLSHandshake(value)
		if len(handshake) == 0 {
			continue
		}
		normalized[serverName] = handshake
	}

	if len(normalized) == 0 {
		return nil
	}
	return normalized
}

func normalizeMihomoVMessListener(listener map[string]interface{}) {
	normalizeMihomoListenerTransport(listener)
	synthesizeVMessUsers(listener)
	delete(listener, "uuid")
	delete(listener, "alter_id")
	delete(listener, "username")
	delete(listener, "security")
	delete(listener, "packet_encoding")
	delete(listener, "network")
	delete(listener, "global_padding")
	delete(listener, "authenticated_length")
}

func normalizeMihomoVLESSListener(listener map[string]interface{}) {
	applyMihomoVLESSDecryptionCompat(listener)
	normalizeMihomoListenerTransport(listener)
	synthesizeVLESSUsers(listener)
	delete(listener, "uuid")
	delete(listener, "username")
	delete(listener, "flow")
	delete(listener, "packet_encoding")
	delete(listener, "network")
}

func applyMihomoVLESSDecryptionCompat(listener map[string]interface{}) {
	if listener == nil {
		return
	}

	enabled, hasEnabled := util.VLESSInboundEncryptionEnabled(listener)
	if hasEnabled {
		if enabled {
			if decryption, ok := util.BuildVLESSMihomoDecryption(listener); ok {
				listener["decryption"] = decryption
			} else {
				delete(listener, "decryption")
			}
		} else {
			delete(listener, "decryption")
		}
	}

	for _, key := range util.VLESSInboundEncryptionHelperKeys {
		delete(listener, key)
	}
}

func normalizeMihomoTrojanListener(listener map[string]interface{}) {
	normalizeMihomoListenerTransport(listener)
	synthesizeTrojanUsers(listener)
	delete(listener, "password")
	delete(listener, "username")
	delete(listener, "network")
}

func normalizeMihomoMieruListener(listener map[string]interface{}) {
	listener["transport"] = util.NormalizeMieruTransport(firstString(listener["transport"]))
	if value, ok := toBool(listener["user-hint-is-mandatory"]); ok {
		listener["user-hint-is-mandatory"] = value
	} else if value, ok := toBool(listener["user_hint_is_mandatory"]); ok {
		listener["user-hint-is-mandatory"] = value
	} else {
		listener["user-hint-is-mandatory"] = true
	}
	delete(listener, "user_hint_is_mandatory")
	delete(listener, "port_bindings")
	delete(listener, "port_range")
}

func normalizeMihomoSudokuListener(listener map[string]interface{}) {
	if key := util.NormalizeSudokuKeyValue(listener["key"]); key != "" {
		listener["key"] = key
	} else {
		delete(listener, "key")
	}

	listener["aead-method"] = util.NormalizeSudokuAEADMethod(firstString(listener["aead-method"]))
	if legacy := util.NormalizeSudokuAEADMethod(firstString(listener["aead_method"])); legacy != "" {
		listener["aead-method"] = legacy
	}

	if value, ok := toInt(listener["padding-min"]); ok && value > 0 {
		listener["padding-min"] = value
	} else if value, ok := toInt(listener["padding_min"]); ok && value > 0 {
		listener["padding-min"] = value
	}
	if value, ok := toInt(listener["padding-max"]); ok && value > 0 {
		listener["padding-max"] = value
	} else if value, ok := toInt(listener["padding_max"]); ok && value > 0 {
		listener["padding-max"] = value
	}

	customTable := util.NormalizeSudokuCustomTable(listener["custom-table"])
	if customTable == "" {
		customTable = util.NormalizeSudokuCustomTable(listener["custom_table"])
	}
	customTables := util.NormalizeSudokuCustomTables(listener["custom-tables"])
	if len(customTables) == 0 {
		customTables = util.NormalizeSudokuCustomTables(listener["custom_tables"])
	}
	hasCustomTables := customTable != "" || len(customTables) > 0

	listener["table-type"] = util.NormalizeSudokuTableTypeForCustom(
		firstString(listener["table-type"]),
		hasCustomTables,
	)
	if legacyRaw := strings.TrimSpace(firstString(listener["table_type"])); legacyRaw != "" {
		listener["table-type"] = util.NormalizeSudokuTableTypeForCustom(legacyRaw, hasCustomTables)
	}

	if customTable != "" {
		listener["custom-table"] = customTable
	} else {
		delete(listener, "custom-table")
	}

	if len(customTables) > 0 {
		listener["custom-tables"] = customTables
	} else {
		delete(listener, "custom-tables")
	}

	if value, ok := toInt(listener["handshake-timeout"]); ok && value > 0 {
		listener["handshake-timeout"] = value
	} else if value, ok := toInt(listener["handshake_timeout"]); ok && value > 0 {
		listener["handshake-timeout"] = value
	}

	if value, ok := toBool(listener["enable-pure-downlink"]); ok {
		listener["enable-pure-downlink"] = value
	} else if value, ok := toBool(listener["enable_pure_downlink"]); ok {
		listener["enable-pure-downlink"] = value
	}

	if fallback := util.NormalizeSudokuStringValue(listener["fallback"]); fallback != "" {
		listener["fallback"] = fallback
	} else {
		delete(listener, "fallback")
	}
	if value, ok := toBool(listener["disable-http-mask"]); ok {
		listener["disable-http-mask"] = value
	} else if value, ok := toBool(listener["disable_http_mask"]); ok {
		listener["disable-http-mask"] = value
	}

	httpmaskInput, _ := listener["httpmask"].(map[string]interface{})
	httpmask := map[string]interface{}{}
	if httpmaskInput != nil {
		if value, ok := toBool(httpmaskInput["disable"]); ok {
			httpmask["disable"] = value
		}
		httpmask["mode"] = util.NormalizeSudokuHTTPMaskMode(firstString(httpmaskInput["mode"]))
		if pathRoot := util.NormalizeSudokuStringValue(httpmaskInput["path_root"]); pathRoot != "" {
			httpmask["path_root"] = pathRoot
		} else if pathRoot := util.NormalizeSudokuStringValue(httpmaskInput["path-root"]); pathRoot != "" {
			httpmask["path_root"] = pathRoot
		}
	}
	if _, exists := httpmask["disable"]; !exists {
		httpmask["disable"] = false
	}
	if _, exists := httpmask["mode"]; !exists {
		if value := util.NormalizeSudokuHTTPMaskMode(firstString(listener["http-mask-mode"])); value != "" {
			httpmask["mode"] = value
		} else {
			httpmask["mode"] = "legacy"
		}
	}
	if pathRoot := util.NormalizeSudokuStringValue(listener["path_root"]); pathRoot != "" {
		httpmask["path_root"] = pathRoot
	} else if pathRoot := util.NormalizeSudokuStringValue(listener["path-root"]); pathRoot != "" {
		httpmask["path_root"] = pathRoot
	}
	if len(httpmask) > 0 {
		listener["httpmask"] = httpmask
	} else {
		delete(listener, "httpmask")
	}

	delete(listener, "aead_method")
	delete(listener, "padding_min")
	delete(listener, "padding_max")
	delete(listener, "table_type")
	delete(listener, "custom_table")
	delete(listener, "custom_tables")
	delete(listener, "handshake_timeout")
	delete(listener, "enable_pure_downlink")
	delete(listener, "disable_http_mask")
	delete(listener, "http-mask-mode")
	delete(listener, "path_root")
	delete(listener, "path-root")
}

func normalizeMihomoTunListener(listener map[string]interface{}) {
	if value := strings.TrimSpace(firstString(listener["device"])); value != "" {
		listener["device"] = value
	} else if value := strings.TrimSpace(firstString(listener["interface_name"])); value != "" {
		listener["device"] = value
	}

	if rawAddresses := toStringSlice(listener["address"]); len(rawAddresses) > 0 {
		inet4, inet6 := splitMihomoTunAddresses(rawAddresses)
		if len(inet4) > 0 {
			listener["inet4-address"] = inet4
		}
		if len(inet6) > 0 {
			listener["inet6-address"] = inet6
		}
	}

	if seconds, ok := durationToSeconds(listener["udp_timeout"]); ok && seconds > 0 {
		listener["udp-timeout"] = seconds
	}

	delete(listener, "interface_name")
	delete(listener, "address")
	delete(listener, "udp_timeout")
}

func normalizeMihomoTProxyListener(listener map[string]interface{}) {
	switch strings.ToLower(strings.TrimSpace(firstString(listener["network"]))) {
	case "tcp":
		listener["udp"] = false
	case "udp", "":
		if firstString(listener["network"]) != "" {
			listener["udp"] = true
		}
	}
	delete(listener, "network")
}

func normalizeMihomoListenerTransport(listener map[string]interface{}) {
	transport, ok := listener["transport"].(map[string]interface{})
	if !ok || transport == nil {
		delete(listener, "transport")
		return
	}

	switch strings.ToLower(strings.TrimSpace(firstString(transport["type"]))) {
	case "ws":
		if path := strings.TrimSpace(firstString(transport["path"])); path != "" {
			listener["ws-path"] = path
		}
	case "grpc":
		if serviceName := strings.TrimSpace(firstString(transport["service_name"])); serviceName != "" {
			listener["grpc-service-name"] = serviceName
		}
	case "xhttp":
		xhttpConfig := map[string]interface{}{}
		if path := strings.TrimSpace(firstString(transport["path"])); path != "" {
			xhttpConfig["path"] = path
		}
		if host := strings.TrimSpace(firstString(transport["host"])); host != "" {
			xhttpConfig["host"] = host
		}
		if mode := strings.TrimSpace(firstString(transport["mode"])); mode != "" {
			xhttpConfig["mode"] = mode
		}
		if noSSEHeader, ok := toBool(transport["no_sse_header"]); ok {
			xhttpConfig["no-sse-header"] = noSSEHeader
		} else if noGRPCHeader, ok := toBool(transport["no_grpc_header"]); ok {
			xhttpConfig["no-sse-header"] = noGRPCHeader
		}
		if streamUpSecs := strings.TrimSpace(firstString(transport["sc_stream_up_server_secs"])); streamUpSecs != "" {
			xhttpConfig["sc-stream-up-server-secs"] = streamUpSecs
		}
		if postBytes, ok := toInt(transport["sc_max_each_post_bytes"]); ok && postBytes > 0 {
			xhttpConfig["sc-max-each-post-bytes"] = postBytes
		}
		if len(xhttpConfig) > 0 {
			listener["xhttp-config"] = xhttpConfig
		}
	}

	delete(listener, "transport")
}

func normalizeMihomoListenerMuxOption(listener map[string]interface{}) {
	mux, ok := listener["multiplex"].(map[string]interface{})
	if !ok || mux == nil {
		delete(listener, "multiplex")
		return
	}

	enabled, hasEnabled := toBool(mux["enabled"])
	if hasEnabled && !enabled {
		delete(listener, "multiplex")
		return
	}

	muxOption := map[string]interface{}{}
	if padding, ok := toBool(mux["padding"]); ok {
		muxOption["padding"] = padding
	}
	if brutal, ok := mux["brutal"].(map[string]interface{}); ok && brutal != nil {
		brutalOption := map[string]interface{}{}
		if enabled, ok := toBool(brutal["enabled"]); ok {
			brutalOption["enabled"] = enabled
		}
		if up := normalizeBandwidthString(brutal["up"]); up != "" {
			brutalOption["up"] = up
		} else if up := normalizeBandwidthString(brutal["up_mbps"]); up != "" {
			brutalOption["up"] = up
		}
		if down := normalizeBandwidthString(brutal["down"]); down != "" {
			brutalOption["down"] = down
		} else if down := normalizeBandwidthString(brutal["down_mbps"]); down != "" {
			brutalOption["down"] = down
		}
		if len(brutalOption) > 0 {
			muxOption["brutal"] = brutalOption
		}
	}

	if len(muxOption) > 0 {
		listener["mux-option"] = muxOption
	}
	delete(listener, "multiplex")
}

func synthesizeVMessUsers(listener map[string]interface{}) {
	if !mihomoUsersEmpty(listener["users"]) {
		return
	}
	uuid := strings.TrimSpace(firstString(listener["uuid"]))
	if uuid == "" {
		return
	}

	user := map[string]interface{}{
		"uuid": uuid,
	}
	if username := strings.TrimSpace(firstString(listener["username"])); username != "" {
		user["username"] = username
	}
	if alterID, ok := toInt(listener["alter_id"]); ok && alterID >= 0 {
		user["alterId"] = alterID
	} else {
		user["alterId"] = 0
	}
	listener["users"] = []interface{}{user}
}

func synthesizeVLESSUsers(listener map[string]interface{}) {
	if !mihomoUsersEmpty(listener["users"]) {
		return
	}
	uuid := strings.TrimSpace(firstString(listener["uuid"]))
	if uuid == "" {
		return
	}

	user := map[string]interface{}{
		"uuid": uuid,
	}
	if username := strings.TrimSpace(firstString(listener["username"])); username != "" {
		user["username"] = username
	}
	if flow := strings.TrimSpace(firstString(listener["flow"])); flow != "" && listener["tls"] != nil {
		user["flow"] = flow
	}
	listener["users"] = []interface{}{user}
}

func synthesizeTrojanUsers(listener map[string]interface{}) {
	if !mihomoUsersEmpty(listener["users"]) {
		return
	}
	password := strings.TrimSpace(firstString(listener["password"]))
	if password == "" {
		return
	}

	user := map[string]interface{}{
		"password": password,
	}
	if username := strings.TrimSpace(firstString(listener["username"])); username != "" {
		user["username"] = username
	}
	listener["users"] = []interface{}{user}
}

func normalizeAnyTLSPaddingScheme(raw interface{}) string {
	switch value := raw.(type) {
	case string:
		value = strings.ReplaceAll(value, "\r\n", "\n")
		value = strings.ReplaceAll(value, "\r", "\n")
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			return ""
		}
		if !strings.Contains(trimmed, "\n") && strings.Contains(trimmed, ",") {
			trimmed = strings.Join(splitAnyTLSPaddingScheme(trimmed), "\n")
		}
		return normalizeAnyTLSPaddingLines(strings.Split(trimmed, "\n"))
	case []string:
		return normalizeAnyTLSPaddingLines(value)
	case []interface{}:
		return normalizeAnyTLSPaddingLines(toStringSlice(value))
	default:
		return ""
	}
}

func splitAnyTLSPaddingScheme(value string) []string {
	segments := make([]string, 0, 8)
	start := 0
	for i := 0; i < len(value); i++ {
		if value[i] != ',' {
			continue
		}

		next := i + 1
		for next < len(value) && (value[next] == ' ' || value[next] == '\t') {
			next++
		}
		if !isAnyTLSPaddingDirective(value, next) {
			continue
		}

		segments = append(segments, value[start:i])
		start = next
	}
	segments = append(segments, value[start:])
	return segments
}

func isAnyTLSPaddingDirective(value string, start int) bool {
	if start >= len(value) {
		return false
	}
	if strings.HasPrefix(value[start:], "stop=") {
		return true
	}
	end := start
	for end < len(value) && value[end] >= '0' && value[end] <= '9' {
		end++
	}
	return end > start && end < len(value) && value[end] == '='
}

func normalizeAnyTLSPaddingLines(lines []string) string {
	normalized := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			normalized = append(normalized, line)
		}
	}
	return strings.Join(normalized, "\n")
}

func normalizeBandwidthString(raw interface{}) string {
	switch value := raw.(type) {
	case string:
		value = strings.TrimSpace(value)
		if value == "" {
			return ""
		}
		return value
	default:
		if n, ok := toInt(raw); ok && n > 0 {
			return fmt.Sprintf("%d Mbps", n)
		}
		return ""
	}
}

func normalizeMihomoMasquerade(raw interface{}) string {
	switch value := raw.(type) {
	case string:
		return strings.TrimSpace(value)
	case map[string]interface{}:
		switch strings.ToLower(strings.TrimSpace(firstString(value["type"]))) {
		case "file":
			return normalizeMihomoFileMasquerade(value["directory"])
		case "proxy":
			return strings.TrimSpace(firstString(value["url"]))
		default:
			if url := strings.TrimSpace(firstString(value["url"])); url != "" {
				return url
			}
			if directory := normalizeMihomoFileMasquerade(value["directory"]); directory != "" {
				return directory
			}
			return ""
		}
	default:
		return ""
	}
}

func normalizeMihomoFileMasquerade(raw interface{}) string {
	directory := strings.TrimSpace(firstString(raw))
	if directory == "" {
		return ""
	}
	if strings.HasPrefix(directory, "file://") {
		return directory
	}
	if strings.HasPrefix(directory, "/") {
		return "file://" + directory
	}
	return "file:///" + strings.TrimLeft(directory, "/")
}

func splitMihomoTunAddresses(addresses []string) ([]string, []string) {
	inet4 := make([]string, 0, len(addresses))
	inet6 := make([]string, 0, len(addresses))
	for _, address := range addresses {
		address = strings.TrimSpace(address)
		if address == "" {
			continue
		}
		if strings.Contains(address, ":") {
			inet6 = append(inet6, address)
			continue
		}
		inet4 = append(inet4, address)
	}
	return inet4, inet6
}

func normalizeMihomoHysteria2ReceiveWindows(dst map[string]interface{}, src map[string]interface{}) {
	if dst == nil || src == nil {
		return
	}

	copyIfMissingMapped(dst, src, "initial-stream-receive-window", "initial-stream-receive-window", "initial_stream_receive_window")
	copyIfMissingMapped(dst, src, "max-stream-receive-window", "max-stream-receive-window", "max_stream_receive_window")
	copyIfMissingMapped(dst, src, "initial-connection-receive-window", "initial-connection-receive-window", "initial_connection_receive_window")
	copyIfMissingMapped(dst, src, "max-connection-receive-window", "max-connection-receive-window", "max_connection_receive_window")

	delete(dst, "initial_stream_receive_window")
	delete(dst, "max_stream_receive_window")
	delete(dst, "initial_connection_receive_window")
	delete(dst, "max_connection_receive_window")
}

func copyIfMissingMapped(dst map[string]interface{}, src map[string]interface{}, dstKey string, srcKeys ...string) {
	if _, exists := dst[dstKey]; exists {
		return
	}
	for _, srcKey := range srcKeys {
		if value, ok := src[srcKey]; ok && value != nil {
			dst[dstKey] = value
			return
		}
	}
}

func mihomoUsersEmpty(raw interface{}) bool {
	switch value := raw.(type) {
	case nil:
		return true
	case []interface{}:
		return len(value) == 0
	case []string:
		return len(value) == 0
	case map[string]interface{}:
		return len(value) == 0
	case map[string]string:
		return len(value) == 0
	default:
		return false
	}
}
