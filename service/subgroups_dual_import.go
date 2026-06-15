package service

import (
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"github.com/alireza0/s-ui/util"
	"gopkg.in/yaml.v3"
)

type subscriptionImportNode struct {
	Tag          string
	JSONOutbound map[string]interface{}
	JSONRaw      json.RawMessage
	ClashProxy   map[string]interface{}
	ClashRawYAML []byte
}

func extractClashProxies(yamlData []byte) ([]map[string]interface{}, error) {
	var doc map[string]interface{}
	if err := yaml.Unmarshal(yamlData, &doc); err != nil {
		return nil, fmt.Errorf("failed to parse clash yaml: %v", err)
	}

	raw, ok := doc["proxies"]
	if !ok {
		return nil, fmt.Errorf("clash subscription has no proxies field")
	}

	proxies, ok := raw.([]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid clash proxies format")
	}

	result := make([]map[string]interface{}, 0, len(proxies))
	for _, item := range proxies {
		proxy, ok := item.(map[string]interface{})
		if !ok || proxy == nil {
			continue
		}
		name, _ := proxy["name"].(string)
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}

		result = append(result, cloneMap(proxy))
	}
	return result, nil
}

func mergeImportedSubscriptionNodes(
	jsonOutbounds []map[string]interface{},
	clashProxies []map[string]interface{},
) []subscriptionImportNode {
	nodes := make([]subscriptionImportNode, 0, len(jsonOutbounds)+len(clashProxies))
	byTag := make(map[string]int, len(jsonOutbounds)+len(clashProxies))

	for _, outbound := range jsonOutbounds {
		if outbound == nil {
			continue
		}
		tag, _ := outbound["tag"].(string)
		tag = strings.TrimSpace(tag)
		if tag == "" {
			continue
		}
		if _, exists := byTag[tag]; exists {
			continue
		}
		nodes = append(nodes, subscriptionImportNode{
			Tag:          tag,
			JSONOutbound: cloneMap(outbound),
		})
		byTag[tag] = len(nodes) - 1
	}

	for _, proxy := range clashProxies {
		if proxy == nil {
			continue
		}
		name, _ := proxy["name"].(string)
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}

		convertedOutbound, converted := convertClashProxyToSubOutbound(proxy)
		index, exists := byTag[name]
		if !exists {
			if !converted {
				continue
			}
			nodes = append(nodes, subscriptionImportNode{
				Tag:          name,
				JSONOutbound: convertedOutbound,
			})
			index = len(nodes) - 1
			byTag[name] = index
		} else if converted && nodes[index].JSONOutbound == nil {
			nodes[index].JSONOutbound = convertedOutbound
		}

		nodes[index].ClashProxy = cloneMap(proxy)
	}

	for i := range nodes {
		tag := strings.TrimSpace(nodes[i].Tag)
		if tag == "" {
			continue
		}
		if nodes[i].JSONOutbound != nil {
			nodes[i].JSONOutbound["tag"] = tag
		}
		if nodes[i].ClashProxy != nil {
			nodes[i].ClashProxy["name"] = tag
		}
	}

	return nodes
}

func mergeMissingOutboundFields(dst map[string]interface{}, src map[string]interface{}) {
	if dst == nil || src == nil {
		return
	}

	for key, value := range src {
		if isEmptyMergedFieldValue(value) {
			continue
		}

		existing, exists := dst[key]
		if !exists || isEmptyMergedFieldValue(existing) {
			dst[key] = value
			continue
		}

		srcMap, srcIsMap := value.(map[string]interface{})
		if !srcIsMap {
			continue
		}
		dstMap, dstIsMap := existing.(map[string]interface{})
		if !dstIsMap || dstMap == nil {
			continue
		}

		mergeMissingOutboundFields(dstMap, srcMap)
	}
}

func isEmptyMergedFieldValue(value interface{}) bool {
	if value == nil {
		return true
	}

	switch typed := value.(type) {
	case string:
		return strings.TrimSpace(typed) == ""
	case []interface{}:
		return len(typed) == 0
	case []string:
		return len(typed) == 0
	case map[string]interface{}:
		return len(typed) == 0
	}

	return false
}

func convertClashProxyToSubOutbound(proxy map[string]interface{}) (map[string]interface{}, bool) {
	name, _ := proxy["name"].(string)
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, false
	}

	clashType, _ := proxy["type"].(string)
	clashType = strings.ToLower(strings.TrimSpace(clashType))
	outType := clashTypeToSubType(clashType)
	if outType == "" {
		return nil, false
	}

	outbound := map[string]interface{}{
		"type": outType,
		"tag":  name,
	}

	if server, ok := proxy["server"].(string); ok && strings.TrimSpace(server) != "" {
		outbound["server"] = strings.TrimSpace(server)
	}
	if port, ok := toIntValue(proxy["port"]); ok && port > 0 {
		outbound["server_port"] = port
	}

	if outType == "shadowsocks" && isClashShadowTLSProxy(proxy) {
		return convertClashShadowTLSProxyToSubOutbound(proxy, name)
	}

	switch outType {
	case "shadowsocks":
		if cipher, ok := proxy["cipher"].(string); ok && cipher != "" {
			outbound["method"] = cipher
		}
		if password, ok := proxy["password"].(string); ok && password != "" {
			outbound["password"] = password
		}
		if udpOverTCP := buildClashUDPOverTCPOptions(proxy); udpOverTCP != nil {
			outbound["udp_over_tcp"] = udpOverTCP
		}
		if plugin, ok := proxy["plugin"].(string); ok && strings.TrimSpace(plugin) != "" {
			outbound["plugin"] = strings.TrimSpace(plugin)
		}
		if pluginOpts, ok := proxy["plugin-opts"]; ok && pluginOpts != nil {
			outbound["plugin_opts"] = pluginOpts
		}
	case "socks":
		if username, ok := proxy["username"].(string); ok && username != "" {
			outbound["username"] = username
		}
		if password, ok := proxy["password"].(string); ok && password != "" {
			outbound["password"] = password
		}
	case "http":
		if username, ok := proxy["username"].(string); ok && username != "" {
			outbound["username"] = username
		}
		if password, ok := proxy["password"].(string); ok && password != "" {
			outbound["password"] = password
		}
		if headers, ok := proxy["headers"].(map[string]interface{}); ok && len(headers) > 0 {
			outbound["headers"] = headers
		}
	case "vmess":
		if uuid, ok := proxy["uuid"].(string); ok && uuid != "" {
			outbound["uuid"] = uuid
		}
		if alterID, ok := toIntValue(proxy["alterId"]); ok {
			outbound["alter_id"] = alterID
		}
		if security, ok := proxy["cipher"].(string); ok && security != "" {
			outbound["security"] = security
		}
		if packetEncoding, ok := proxy["packet-encoding"].(string); ok && packetEncoding != "" {
			outbound["packet_encoding"] = packetEncoding
		}
	case "vless":
		if uuid, ok := proxy["uuid"].(string); ok && uuid != "" {
			outbound["uuid"] = uuid
		}
		if flow, ok := proxy["flow"].(string); ok && flow != "" {
			outbound["flow"] = flow
		}
		if packetEncoding, ok := proxy["packet-encoding"].(string); ok && packetEncoding != "" {
			outbound["packet_encoding"] = packetEncoding
		}
		if encryption, ok := proxy["encryption"].(string); ok && encryption != "" {
			outbound["encryption"] = encryption
		}
	case "trojan":
		if password, ok := proxy["password"].(string); ok && password != "" {
			outbound["password"] = password
		}
	case "snell":
		if psk, ok := proxy["psk"].(string); ok && strings.TrimSpace(psk) != "" {
			outbound["psk"] = strings.TrimSpace(psk)
		}
		if version, ok := toIntValue(proxy["version"]); ok && version > 0 {
			outbound["version"] = version
		}
		if reuse, ok := toBoolValue(proxy["reuse"]); ok {
			outbound["reuse"] = reuse
		}
		if obfsOpts, ok := proxy["obfs-opts"].(map[string]interface{}); ok && obfsOpts != nil {
			mode := strings.TrimSpace(firstStringValue(obfsOpts["mode"]))
			if mode != "" {
				host := strings.TrimSpace(firstStringValue(obfsOpts["host"]))
				if host == "" {
					host = "www.bing.com"
				}
				outbound["obfs_opts"] = map[string]interface{}{
					"mode": mode,
					"host": host,
				}
			}
		}
	case "tuic":
		if token, ok := proxy["token"].(string); ok && token != "" {
			outbound["token"] = token
		}
		if uuid, ok := proxy["uuid"].(string); ok && uuid != "" {
			outbound["uuid"] = uuid
		}
		if password, ok := proxy["password"].(string); ok && password != "" {
			outbound["password"] = password
		}
		if cc, ok := proxy["congestion-controller"].(string); ok && cc != "" {
			outbound["congestion_control"] = cc
		}
		if relayMode, ok := proxy["udp-relay-mode"].(string); ok && relayMode != "" {
			outbound["udp_relay_mode"] = relayMode
		}
		if reduceRTT, ok := toBoolValue(proxy["reduce-rtt"]); ok && reduceRTT {
			outbound["zero_rtt_handshake"] = true
		}
		if heartbeatMS, ok := toIntValue(proxy["heartbeat-interval"]); ok && heartbeatMS > 0 {
			outbound["heartbeat"] = fmt.Sprintf("%ds", heartbeatMS/1000)
		}
	case "hysteria":
		if auth, ok := proxy["auth-str"].(string); ok && auth != "" {
			outbound["auth_str"] = auth
		}
		if obfs, ok := proxy["obfs"].(string); ok && obfs != "" {
			outbound["obfs"] = obfs
		}
		if up, ok := toIntValue(proxy["up"]); ok && up > 0 {
			outbound["up_mbps"] = up
		}
		if down, ok := toIntValue(proxy["down"]); ok && down > 0 {
			outbound["down_mbps"] = down
		}
		if streamReceiveWindow, ok := toIntValue(proxy["recv-window-conn"]); ok && streamReceiveWindow > 0 {
			outbound["stream_receive_window"] = streamReceiveWindow
		}
		if connectionReceiveWindow, ok := toIntValue(proxy["recv-window"]); ok && connectionReceiveWindow > 0 {
			outbound["connection_receive_window"] = connectionReceiveWindow
		}
		if disablePathMTU, ok := toBoolValue(proxy["disable-mtu-discovery"]); ok {
			outbound["disable_path_mtu_discovery"] = disablePathMTU
		}
		if ports, ok := proxy["ports"].(string); ok && strings.TrimSpace(ports) != "" {
			serverPorts := parseClashPortsString(ports)
			if len(serverPorts) > 0 {
				outbound["server_ports"] = serverPorts
			}
		}
	case "hysteria2":
		if password, ok := proxy["password"].(string); ok && password != "" {
			outbound["password"] = password
		}
		if up, ok := toIntValue(proxy["up"]); ok && up > 0 {
			outbound["up_mbps"] = up
		}
		if down, ok := toIntValue(proxy["down"]); ok && down > 0 {
			outbound["down_mbps"] = down
		}
		if obfsType, ok := proxy["obfs"].(string); ok && obfsType != "" {
			obfs := map[string]interface{}{"type": obfsType}
			if obfsPassword, ok := proxy["obfs-password"].(string); ok && obfsPassword != "" {
				obfs["password"] = obfsPassword
			}
			outbound["obfs"] = obfs
		}
		if ports, ok := proxy["ports"].(string); ok && strings.TrimSpace(ports) != "" {
			serverPorts := parseClashPortsString(ports)
			if len(serverPorts) > 0 {
				outbound["server_ports"] = serverPorts
			}
		}
		if hopInterval, ok := toIntValue(proxy["hop-interval"]); ok && hopInterval > 0 {
			outbound["hop_interval"] = fmt.Sprintf("%ds", hopInterval)
		}
	case "anytls":
		if password, ok := proxy["password"].(string); ok && password != "" {
			outbound["password"] = password
		}
		if checkInterval, ok := toIntValue(proxy["idle-session-check-interval"]); ok && checkInterval > 0 {
			outbound["idle_session_check_interval"] = fmt.Sprintf("%ds", checkInterval)
		}
		if timeout, ok := toIntValue(proxy["idle-session-timeout"]); ok && timeout > 0 {
			outbound["idle_session_timeout"] = fmt.Sprintf("%ds", timeout)
		}
		if minIdle, ok := toIntValue(proxy["min-idle-session"]); ok && minIdle >= 0 {
			outbound["min_idle_session"] = minIdle
		}
	case "mieru":
		if username, ok := proxy["username"].(string); ok && strings.TrimSpace(username) != "" {
			outbound["username"] = strings.TrimSpace(username)
		}
		if password, ok := proxy["password"].(string); ok && strings.TrimSpace(password) != "" {
			outbound["password"] = strings.TrimSpace(password)
		}
		outbound["transport"] = util.NormalizeMieruTransport(firstStringValue(proxy["transport"]))
		if udp, ok := toBoolValue(proxy["udp"]); ok && udp {
			outbound["udp"] = true
		}
		if value := strings.TrimSpace(firstStringValue(proxy["multiplexing"])); value != "" {
			outbound["multiplexing"] = util.NormalizeMieruMultiplexing(value)
		}
		if value := strings.TrimSpace(firstStringValue(proxy["handshake-mode"])); value != "" {
			outbound["handshake_mode"] = util.NormalizeMieruHandshakeMode(value)
		}
		if portRange, ok := proxy["port-range"].(string); ok && strings.TrimSpace(portRange) != "" {
			normalized, valid := util.NormalizeMieruPortBinding(portRange)
			if valid {
				outbound["port_range"] = normalized
				if _, exists := outbound["server_port"]; !exists {
					if port, ok := util.MieruPrimaryPortFromBinding(normalized); ok {
						outbound["server_port"] = port
					}
				}
			}
		}
	case "sudoku":
		if key := util.NormalizeSudokuKeyValue(proxy["key"]); key != "" {
			outbound["key"] = key
		}
		outbound["aead_method"] = util.NormalizeSudokuAEADMethod(firstStringValue(proxy["aead-method"]))
		if value, ok := toIntValue(proxy["padding-min"]); ok && value > 0 {
			outbound["padding_min"] = value
		}
		if value, ok := toIntValue(proxy["padding-max"]); ok && value > 0 {
			outbound["padding_max"] = value
		}
		customTable := util.NormalizeSudokuCustomTable(proxy["custom-table"])
		customTables := util.NormalizeSudokuCustomTables(proxy["custom-tables"])
		outbound["table_type"] = util.NormalizeSudokuTableTypeForCustom(
			firstStringValue(proxy["table-type"]),
			customTable != "" || len(customTables) > 0,
		)
		if customTable != "" {
			outbound["custom_table"] = customTable
		}
		if len(customTables) > 0 {
			outbound["custom_tables"] = customTables
		}
		if value, ok := toBoolValue(proxy["enable-pure-downlink"]); ok {
			outbound["enable_pure_downlink"] = value
		}
		if httpmask := buildSudokuJSONHTTPMaskFromProxy(proxy["httpmask"]); len(httpmask) > 0 {
			outbound["httpmask"] = httpmask
		}
	case "trusttunnel":
		if username, ok := proxy["username"].(string); ok && strings.TrimSpace(username) != "" {
			outbound["username"] = strings.TrimSpace(username)
		}
		if password, ok := proxy["password"].(string); ok && strings.TrimSpace(password) != "" {
			outbound["password"] = strings.TrimSpace(password)
		}
		if udp, ok := toBoolValue(proxy["udp"]); ok {
			outbound["udp"] = udp
		}
		if quic, ok := toBoolValue(proxy["quic"]); ok && quic {
			outbound["quic"] = true
		}
		if healthCheck, ok := toBoolValue(proxy["health-check"]); ok {
			outbound["health_check"] = healthCheck
		} else if healthCheck, ok := toBoolValue(proxy["health_check"]); ok {
			outbound["health_check"] = healthCheck
		}
		if cc, ok := proxy["congestion-controller"].(string); ok && strings.TrimSpace(cc) != "" {
			outbound["congestion_controller"] = strings.TrimSpace(cc)
		}
		if maxConnections, ok := toIntValue(proxy["max-connections"]); ok && maxConnections >= 0 {
			outbound["max_connections"] = maxConnections
		} else if maxConnections, ok := toIntValue(proxy["max_connections"]); ok && maxConnections >= 0 {
			outbound["max_connections"] = maxConnections
		}
		if minStreams, ok := toIntValue(proxy["min-streams"]); ok && minStreams >= 0 {
			outbound["min_streams"] = minStreams
		} else if minStreams, ok := toIntValue(proxy["min_streams"]); ok && minStreams >= 0 {
			outbound["min_streams"] = minStreams
		}
		if maxStreams, ok := toIntValue(proxy["max-streams"]); ok && maxStreams >= 0 {
			outbound["max_streams"] = maxStreams
		} else if maxStreams, ok := toIntValue(proxy["max_streams"]); ok && maxStreams >= 0 {
			outbound["max_streams"] = maxStreams
		}
	}

	tlsMap := map[string]interface{}{}
	if tlsEnabled, ok := toBoolValue(proxy["tls"]); ok && tlsEnabled {
		tlsMap["enabled"] = true
	}
	if sni, ok := proxy["servername"].(string); ok && sni != "" {
		tlsMap["server_name"] = sni
	}
	if sni, ok := proxy["sni"].(string); ok && sni != "" {
		tlsMap["server_name"] = sni
	}
	if alpn := toStringSliceValue(proxy["alpn"]); len(alpn) > 0 {
		tlsMap["alpn"] = alpn
	}
	if insecure, ok := toBoolValue(proxy["skip-cert-verify"]); ok && insecure {
		tlsMap["insecure"] = true
	}
	if disableSNI, ok := toBoolValue(proxy["disable-sni"]); ok && disableSNI {
		tlsMap["disable_sni"] = true
	}
	if fingerprint, ok := proxy["fingerprint"].(string); ok && strings.TrimSpace(fingerprint) != "" {
		tlsMap["fingerprint"] = strings.TrimSpace(fingerprint)
	}
	if clientFP, ok := proxy["client-fingerprint"].(string); ok && clientFP != "" {
		tlsMap["utls"] = map[string]interface{}{
			"enabled":     true,
			"fingerprint": clientFP,
		}
	}
	if len(tlsMap) > 0 {
		outbound["tls"] = tlsMap
	}

	network, _ := proxy["network"].(string)
	network = strings.ToLower(strings.TrimSpace(network))
	switch network {
	case "ws":
		transport := map[string]interface{}{"type": "ws"}
		if wsOpts, ok := proxy["ws-opts"].(map[string]interface{}); ok {
			if path, ok := wsOpts["path"].(string); ok && path != "" {
				transport["path"] = path
			}
			if headers, ok := wsOpts["headers"].(map[string]interface{}); ok && len(headers) > 0 {
				transport["headers"] = headers
			}
			if maxEarlyData, ok := toIntValue(wsOpts["max-early-data"]); ok && maxEarlyData > 0 {
				transport["max_early_data"] = maxEarlyData
			}
			if earlyName, ok := wsOpts["early-data-header-name"].(string); ok && earlyName != "" {
				transport["early_data_header_name"] = earlyName
			}
			if enabled, ok := toBoolValue(wsOpts["v2ray-http-upgrade"]); ok && enabled {
				transport["v2ray_http_upgrade"] = true
			}
			if enabled, ok := toBoolValue(wsOpts["v2ray-http-upgrade-fast-open"]); ok && enabled {
				transport["v2ray_http_upgrade_fast_open"] = true
			}
		}
		outbound["transport"] = transport
	case "grpc":
		transport := map[string]interface{}{"type": "grpc"}
		if grpcOpts, ok := proxy["grpc-opts"].(map[string]interface{}); ok {
			if serviceName, ok := grpcOpts["grpc-service-name"].(string); ok && serviceName != "" {
				transport["service_name"] = serviceName
			}
			if userAgent, ok := grpcOpts["grpc-user-agent"].(string); ok && userAgent != "" {
				transport["grpc_user_agent"] = userAgent
			}
			if pingInterval, ok := toIntValue(grpcOpts["ping-interval"]); ok && pingInterval > 0 {
				transport["ping_interval"] = pingInterval
			}
			if maxConnections, ok := toIntValue(grpcOpts["max-connections"]); ok && maxConnections > 0 {
				transport["max_connections"] = maxConnections
			}
			if minStreams, ok := toIntValue(grpcOpts["min-streams"]); ok && minStreams >= 0 {
				transport["min_streams"] = minStreams
			}
			if maxStreams, ok := toIntValue(grpcOpts["max-streams"]); ok && maxStreams >= 0 {
				transport["max_streams"] = maxStreams
			}
		}
		outbound["transport"] = transport
	case "http":
		transport := map[string]interface{}{"type": "http"}
		if httpOpts, ok := proxy["http-opts"].(map[string]interface{}); ok {
			if method, ok := httpOpts["method"].(string); ok && method != "" {
				transport["method"] = method
			}
			switch pathList := httpOpts["path"].(type) {
			case []interface{}:
				if len(pathList) > 0 {
					if first, ok := pathList[0].(string); ok && first != "" {
						transport["path"] = first
					}
				}
			case []string:
				if len(pathList) > 0 && strings.TrimSpace(pathList[0]) != "" {
					transport["path"] = strings.TrimSpace(pathList[0])
				}
			}
			if headers, ok := httpOpts["headers"].(map[string]interface{}); ok && len(headers) > 0 {
				transport["headers"] = headers
			}
		}
		outbound["transport"] = transport
	case "h2":
		transport := map[string]interface{}{"type": "h2"}
		if h2Opts, ok := proxy["h2-opts"].(map[string]interface{}); ok {
			if path, ok := h2Opts["path"].(string); ok && path != "" {
				transport["path"] = path
			}
			if hosts := toStringSliceValue(h2Opts["host"]); len(hosts) > 0 {
				transport["host"] = hosts
			}
		}
		outbound["transport"] = transport
	case "xhttp":
		if outType != "vless" {
			break
		}
		transport := map[string]interface{}{"type": "xhttp"}
		if xhttpOpts, ok := proxy["xhttp-opts"].(map[string]interface{}); ok {
			if path, ok := xhttpOpts["path"].(string); ok && strings.TrimSpace(path) != "" {
				transport["path"] = strings.TrimSpace(path)
			}
			if host, ok := xhttpOpts["host"].(string); ok && strings.TrimSpace(host) != "" {
				transport["host"] = strings.TrimSpace(host)
			}
			if mode, ok := xhttpOpts["mode"].(string); ok && strings.TrimSpace(mode) != "" {
				transport["mode"] = strings.TrimSpace(mode)
			}
			if headers, ok := xhttpOpts["headers"].(map[string]interface{}); ok && len(headers) > 0 {
				transport["headers"] = headers
			}
			if noGRPCHeader, ok := toBoolValue(xhttpOpts["no-grpc-header"]); ok {
				transport["no_grpc_header"] = noGRPCHeader
			}
			if padding, ok := xhttpOpts["x-padding-bytes"].(string); ok && strings.TrimSpace(padding) != "" {
				transport["x_padding_bytes"] = strings.TrimSpace(padding)
			}
			if postBytes, ok := toIntValue(xhttpOpts["sc-max-each-post-bytes"]); ok && postBytes > 0 {
				transport["sc_max_each_post_bytes"] = postBytes
			}
			if reuse, ok := xhttpOpts["reuse-settings"].(map[string]interface{}); ok && len(reuse) > 0 {
				if converted := convertClashXHTTPReuseSettings(reuse); len(converted) > 0 {
					transport["reuse_settings"] = converted
				}
			}
			if download, ok := xhttpOpts["download-settings"].(map[string]interface{}); ok && len(download) > 0 {
				if converted := convertClashXHTTPDownloadSettings(download); len(converted) > 0 {
					transport["download_settings"] = converted
				}
			}
		}
		outbound["transport"] = transport
	}

	if multiplex := buildClashMultiplexOptions(proxy); len(multiplex) > 0 {
		outbound["multiplex"] = multiplex
	}
	if outType != "mieru" && outType != "sudoku" && outType != "trusttunnel" {
		applyClashUDPNetwork(outbound, proxy)
	}

	return outbound, true
}

func isClashShadowTLSProxy(proxy map[string]interface{}) bool {
	plugin, _ := proxy["plugin"].(string)
	plugin = strings.ToLower(strings.TrimSpace(plugin))
	return plugin == "shadow-tls" || plugin == "shadowtls"
}

func convertClashShadowTLSProxyToSubOutbound(proxy map[string]interface{}, name string) (map[string]interface{}, bool) {
	server, _ := proxy["server"].(string)
	server = strings.TrimSpace(server)
	port, ok := toIntValue(proxy["port"])
	if name == "" || server == "" || !ok || port <= 0 {
		return nil, false
	}

	outbound := map[string]interface{}{
		"type":        "shadowtls",
		"tag":         name,
		"server":      server,
		"server_port": port,
		"version":     3,
	}

	tlsMap := map[string]interface{}{}
	if clientFP, ok := proxy["client-fingerprint"].(string); ok && strings.TrimSpace(clientFP) != "" {
		tlsMap["utls"] = map[string]interface{}{
			"enabled":     true,
			"fingerprint": strings.TrimSpace(clientFP),
		}
	}

	if pluginOpts, ok := proxy["plugin-opts"].(map[string]interface{}); ok && pluginOpts != nil {
		if version, ok := toIntValue(pluginOpts["version"]); ok && version > 0 {
			outbound["version"] = version
		}
		if password, ok := pluginOpts["password"].(string); ok && strings.TrimSpace(password) != "" {
			outbound["password"] = strings.TrimSpace(password)
		}
		if host, ok := pluginOpts["host"].(string); ok && strings.TrimSpace(host) != "" {
			tlsMap["enabled"] = true
			tlsMap["server_name"] = strings.TrimSpace(host)
		}
		if alpn := toStringSliceValue(pluginOpts["alpn"]); len(alpn) > 0 {
			tlsMap["enabled"] = true
			tlsMap["alpn"] = alpn
		}
		if insecure, ok := toBoolValue(pluginOpts["skip-cert-verify"]); ok && insecure {
			tlsMap["enabled"] = true
			tlsMap["insecure"] = true
		}
		if fingerprint, ok := pluginOpts["fingerprint"].(string); ok && strings.TrimSpace(fingerprint) != "" {
			tlsMap["enabled"] = true
			tlsMap["fingerprint"] = strings.TrimSpace(fingerprint)
		}
	}

	if len(tlsMap) > 0 {
		outbound["tls"] = tlsMap
	}

	ssConfig := map[string]interface{}{}
	if cipher, ok := proxy["cipher"].(string); ok && strings.TrimSpace(cipher) != "" {
		ssConfig["method"] = strings.TrimSpace(cipher)
	}
	if password, ok := proxy["password"].(string); ok && password != "" {
		ssConfig["password"] = password
	}
	applyClashUDPNetwork(ssConfig, proxy)
	if udpOverTCP := buildClashUDPOverTCPOptions(proxy); udpOverTCP != nil {
		ssConfig["udp_over_tcp"] = udpOverTCP
	}
	if multiplex := buildClashMultiplexOptions(proxy); len(multiplex) > 0 {
		ssConfig["multiplex"] = multiplex
	}
	if len(ssConfig) > 0 {
		outbound["ss_config"] = ssConfig
	}

	return outbound, true
}

func clashTypeToSubType(clashType string) string {
	switch clashType {
	case "ss":
		return "shadowsocks"
	case "socks5":
		return "socks"
	case "vmess", "vless", "trojan", "snell", "tuic", "hysteria", "hysteria2", "anytls", "http":
		return clashType
	case "mieru":
		return "mieru"
	case "sudoku":
		return "sudoku"
	case "trusttunnel":
		return "trusttunnel"
	default:
		return ""
	}
}

func firstStringValue(raw interface{}) string {
	switch value := raw.(type) {
	case string:
		return strings.TrimSpace(value)
	case []string:
		for _, item := range value {
			item = strings.TrimSpace(item)
			if item != "" {
				return item
			}
		}
	case []interface{}:
		for _, item := range value {
			itemStr, ok := item.(string)
			if !ok {
				continue
			}
			itemStr = strings.TrimSpace(itemStr)
			if itemStr != "" {
				return itemStr
			}
		}
	}
	return ""
}

func parseClashPortsString(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		p := strings.TrimSpace(part)
		if p == "" {
			continue
		}
		p = strings.ReplaceAll(p, "-", ":")
		result = append(result, p)
	}
	return result
}

func applyClashUDPNetwork(outbound map[string]interface{}, proxy map[string]interface{}) {
	if outbound == nil || proxy == nil {
		return
	}
	udpEnabled, ok := toBoolValue(proxy["udp"])
	if ok && udpEnabled {
		outbound["network"] = "udp"
	}
}

func buildClashUDPOverTCPOptions(proxy map[string]interface{}) interface{} {
	if proxy == nil {
		return nil
	}
	enabled, ok := toBoolValue(proxy["udp-over-tcp"])
	if !ok || !enabled {
		return nil
	}
	if version, ok := toIntValue(proxy["udp-over-tcp-version"]); ok && version > 0 {
		return map[string]interface{}{
			"enabled": true,
			"version": version,
		}
	}
	return true
}

func buildClashMultiplexOptions(proxy map[string]interface{}) map[string]interface{} {
	smux, ok := proxy["smux"].(map[string]interface{})
	if !ok || len(smux) == 0 {
		return nil
	}

	multiplex := map[string]interface{}{}
	if enabled, ok := toBoolValue(smux["enabled"]); ok {
		multiplex["enabled"] = enabled
	}
	if protocol, ok := smux["protocol"].(string); ok && protocol != "" {
		multiplex["protocol"] = protocol
	}
	if maxConnections, ok := toIntValue(smux["max-connections"]); ok && maxConnections > 0 {
		multiplex["max_connections"] = maxConnections
	}
	if minStreams, ok := toIntValue(smux["min-streams"]); ok && minStreams > 0 {
		multiplex["min_streams"] = minStreams
	}
	if maxStreams, ok := toIntValue(smux["max-streams"]); ok && maxStreams > 0 {
		multiplex["max_streams"] = maxStreams
	}
	if padding, ok := toBoolValue(smux["padding"]); ok {
		multiplex["padding"] = padding
	}
	if len(multiplex) == 0 {
		return nil
	}
	return multiplex
}

func convertClashXHTTPReuseSettings(raw map[string]interface{}) map[string]interface{} {
	if raw == nil {
		return nil
	}

	reuse := map[string]interface{}{}
	if value, ok := raw["max-connections"]; ok {
		reuse["max_connections"] = value
	}
	if value, ok := raw["max-concurrency"]; ok {
		reuse["max_concurrency"] = value
	}
	if value, ok := raw["c-max-reuse-times"]; ok {
		reuse["c_max_reuse_times"] = value
	}
	if value, ok := raw["h-max-request-times"]; ok {
		reuse["h_max_request_times"] = value
	}
	if value, ok := raw["h-max-reusable-secs"]; ok {
		reuse["h_max_reusable_secs"] = value
	}

	if len(reuse) == 0 {
		return nil
	}
	return reuse
}

func convertClashXHTTPDownloadSettings(raw map[string]interface{}) map[string]interface{} {
	if raw == nil {
		return nil
	}

	download := map[string]interface{}{}
	if path, ok := raw["path"].(string); ok && strings.TrimSpace(path) != "" {
		download["path"] = strings.TrimSpace(path)
	}
	if host, ok := raw["host"].(string); ok && strings.TrimSpace(host) != "" {
		download["host"] = strings.TrimSpace(host)
	}
	if headers, ok := raw["headers"].(map[string]interface{}); ok && len(headers) > 0 {
		download["headers"] = headers
	}
	if noGRPCHeader, ok := toBoolValue(raw["no-grpc-header"]); ok {
		download["no_grpc_header"] = noGRPCHeader
	}
	if padding, ok := raw["x-padding-bytes"].(string); ok && strings.TrimSpace(padding) != "" {
		download["x_padding_bytes"] = strings.TrimSpace(padding)
	}
	if postBytes, ok := toIntValue(raw["sc-max-each-post-bytes"]); ok && postBytes > 0 {
		download["sc_max_each_post_bytes"] = postBytes
	}
	if reuse, ok := raw["reuse-settings"].(map[string]interface{}); ok && len(reuse) > 0 {
		download["reuse_settings"] = convertClashXHTTPReuseSettings(reuse)
	}

	if server, ok := raw["server"].(string); ok && strings.TrimSpace(server) != "" {
		download["server"] = strings.TrimSpace(server)
	}
	if port, ok := toIntValue(raw["port"]); ok && port > 0 {
		download["port"] = port
	}
	if tls, ok := toBoolValue(raw["tls"]); ok {
		download["tls"] = tls
	}
	if alpn := toStringSliceValue(raw["alpn"]); len(alpn) > 0 {
		download["alpn"] = alpn
	}
	if echOpts, ok := raw["ech-opts"].(map[string]interface{}); ok && len(echOpts) > 0 {
		download["ech_opts"] = cloneMap(echOpts)
	}
	if realityOpts, ok := raw["reality-opts"].(map[string]interface{}); ok && len(realityOpts) > 0 {
		download["reality_opts"] = cloneMap(realityOpts)
	}
	if skipCertVerify, ok := toBoolValue(raw["skip-cert-verify"]); ok {
		download["skip_cert_verify"] = skipCertVerify
	}
	if fingerprint, ok := raw["fingerprint"].(string); ok && strings.TrimSpace(fingerprint) != "" {
		download["fingerprint"] = strings.TrimSpace(fingerprint)
	}
	if certificate, ok := raw["certificate"].(string); ok && strings.TrimSpace(certificate) != "" {
		download["certificate"] = strings.TrimSpace(certificate)
	}
	if privateKey, ok := raw["private-key"].(string); ok && strings.TrimSpace(privateKey) != "" {
		download["private_key"] = strings.TrimSpace(privateKey)
	}
	if serverName, ok := raw["servername"].(string); ok && strings.TrimSpace(serverName) != "" {
		download["servername"] = strings.TrimSpace(serverName)
	}
	if clientFingerprint, ok := raw["client-fingerprint"].(string); ok && strings.TrimSpace(clientFingerprint) != "" {
		download["client_fingerprint"] = strings.TrimSpace(clientFingerprint)
	}

	if len(download) == 0 {
		return nil
	}
	return download
}

func buildSudokuJSONHTTPMaskFromProxy(raw interface{}) map[string]interface{} {
	httpmaskRaw, _ := raw.(map[string]interface{})
	if httpmaskRaw == nil {
		httpmaskRaw = map[string]interface{}{}
	}

	httpmask := map[string]interface{}{
		"disable":   false,
		"mode":      "legacy",
		"tls":       true,
		"multiplex": "off",
	}
	if value, ok := toBoolValue(httpmaskRaw["disable"]); ok {
		httpmask["disable"] = value
	}
	if value := util.NormalizeSudokuHTTPMaskMode(firstStringValue(httpmaskRaw["mode"])); value != "" {
		httpmask["mode"] = value
	}
	if value, ok := toBoolValue(httpmaskRaw["tls"]); ok {
		httpmask["tls"] = value
	}
	if host := util.NormalizeSudokuStringValue(httpmaskRaw["mask-host"]); host != "" {
		httpmask["host"] = host
	} else if host := util.NormalizeSudokuStringValue(httpmaskRaw["host"]); host != "" {
		httpmask["host"] = host
	}
	if pathRoot := util.NormalizeSudokuStringValue(httpmaskRaw["path-root"]); pathRoot != "" {
		httpmask["path_root"] = pathRoot
	} else if pathRoot := util.NormalizeSudokuStringValue(httpmaskRaw["path_root"]); pathRoot != "" {
		httpmask["path_root"] = pathRoot
	}
	if value := util.NormalizeSudokuHTTPMaskMultiplex(firstStringValue(httpmaskRaw["multiplex"])); value != "" {
		httpmask["multiplex"] = value
	}
	return httpmask
}

func cloneMap(src map[string]interface{}) map[string]interface{} {
	if src == nil {
		return nil
	}
	dst := make(map[string]interface{}, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
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
		n, err := strconv.Atoi(value)
		if err == nil {
			return n, true
		}
	}
	return 0, false
}

func toBoolValue(raw interface{}) (bool, bool) {
	switch value := raw.(type) {
	case bool:
		return value, true
	case int:
		return value != 0, true
	case int32:
		return value != 0, true
	case int64:
		return value != 0, true
	case float32:
		return value != 0, true
	case float64:
		return value != 0, true
	case string:
		value = strings.TrimSpace(strings.ToLower(value))
		if value == "" {
			return false, false
		}
		switch value {
		case "1", "true", "yes", "on":
			return true, true
		case "0", "false", "no", "off":
			return false, true
		}
	}
	return false, false
}

func toStringSliceValue(raw interface{}) []string {
	switch value := raw.(type) {
	case []string:
		out := make([]string, 0, len(value))
		for _, item := range value {
			item = strings.TrimSpace(item)
			if item != "" {
				out = append(out, item)
			}
		}
		return out
	case []interface{}:
		out := make([]string, 0, len(value))
		for _, item := range value {
			s, ok := item.(string)
			if !ok {
				continue
			}
			s = strings.TrimSpace(s)
			if s != "" {
				out = append(out, s)
			}
		}
		return out
	case string:
		value = strings.TrimSpace(value)
		if value == "" {
			return nil
		}
		return []string{value}
	default:
		return nil
	}
}

func encodeClashProxyOptions(proxy map[string]interface{}) json.RawMessage {
	if proxy == nil {
		return nil
	}

	if normalized, ok := normalizeClashNumberTypes(proxy).(map[string]interface{}); ok && normalized != nil {
		proxy = normalized
	}
	data, err := json.MarshalIndent(proxy, "", "  ")
	if err != nil {
		return nil
	}
	return data
}

func normalizeClashNumberTypes(value interface{}) interface{} {
	switch typed := value.(type) {
	case map[string]interface{}:
		out := make(map[string]interface{}, len(typed))
		for key, item := range typed {
			out[key] = normalizeClashNumberTypes(item)
		}
		return out
	case []interface{}:
		out := make([]interface{}, 0, len(typed))
		for _, item := range typed {
			out = append(out, normalizeClashNumberTypes(item))
		}
		return out
	case float64:
		return normalizeClashFloat(typed)
	case float32:
		return normalizeClashFloat(float64(typed))
	default:
		return typed
	}
}

func normalizeClashFloat(value float64) interface{} {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return value
	}
	if value == math.Trunc(value) && value >= math.MinInt64 && value <= math.MaxInt64 {
		return int64(value)
	}
	return value
}
