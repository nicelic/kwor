package util

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strings"

	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/util/common"
)

var InboundTypeWithLink = []string{"socks", "http", "snell", "shadowsocks", "naive", "hysteria", "hysteria2", "anytls", "tuic", "vless", "trojan", "vmess", "mieru"}

type LinkParam struct {
	Key   string
	Value string
}

func LinkGenerator(clientConfig json.RawMessage, i *model.Inbound, hostname string) []string {
	if i == nil || IsSubscriptionServerOnlyInboundType(i.Type) {
		return []string{}
	}
	inbound, err := i.MarshalFull()
	if err != nil {
		return []string{}
	}

	var tls map[string]interface{}
	if i.TlsId > 0 {
		tls = prepareTls(i.Tls)
	}

	var userConfig map[string]map[string]interface{}
	if err := json.Unmarshal(clientConfig, &userConfig); err != nil {
		return []string{}
	}

	var Addrs []map[string]interface{}
	if err := json.Unmarshal(i.Addrs, &Addrs); err != nil {
		return []string{}
	}
	if len(Addrs) == 0 {
		Addrs = append(Addrs, map[string]interface{}{
			"server":      hostname,
			"server_port": (*inbound)["listen_port"],
			"remark":      i.Tag,
		})
		if i.TlsId > 0 {
			Addrs[0]["tls"] = tls
		}
	} else {
		for index, addr := range Addrs {
			addrRemark, _ := addr["remark"].(string)
			Addrs[index]["remark"] = i.Tag + addrRemark
			if i.TlsId > 0 {
				newTls := map[string]interface{}{}
				for k, v := range tls {
					newTls[k] = v
				}

				// Override tls
				if addrTls, ok := addr["tls"].(map[string]interface{}); ok {
					for k, v := range addrTls {
						newTls[k] = v
					}
				}
				Addrs[index]["tls"] = newTls
			}
		}
	}

	switch i.Type {
	case "socks":
		return socksLink(userConfig["socks"], Addrs)
	case "http":
		return httpLink(userConfig["http"], Addrs)
	case "snell":
		return snellLink(userConfig["snell"], *inbound, Addrs)
	case "shadowsocks":
		return shadowsocksLink(userConfig, *inbound, Addrs)
	case "naive":
		return naiveLink(userConfig["naive"], *inbound, Addrs)
	case "hysteria":
		return hysteriaLink(userConfig["hysteria"], *inbound, Addrs)
	case "hysteria2":
		return hysteria2Link(userConfig["hysteria2"], *inbound, Addrs)
	case "tuic":
		return tuicLink(userConfig["tuic"], *inbound, Addrs)
	case "vless":
		return vlessLink(userConfig["vless"], *inbound, Addrs)
	case "anytls":
		return anytlsLink(userConfig["anytls"], Addrs)
	case "mieru":
		return mieruLink(userConfig["mieru"], *inbound, Addrs)
	case "trojan":
		return trojanLink(userConfig["trojan"], *inbound, Addrs)
	case "vmess":
		return vmessLink(userConfig["vmess"], *inbound, Addrs)
	}

	return []string{}
}

func prepareTls(t *model.Tls) map[string]interface{} {
	if t == nil {
		return nil
	}

	var iTls, oTls map[string]interface{}
	if err := json.Unmarshal(t.Client, &oTls); err != nil {
		return nil
	}
	if err := json.Unmarshal(t.Server, &iTls); err != nil {
		return nil
	}
	if iTls == nil {
		iTls = map[string]interface{}{}
	}
	if oTls == nil {
		oTls = map[string]interface{}{}
	}

	for k, v := range iTls {
		switch k {
		case "enabled", "server_name", "alpn":
			oTls[k] = v
		case "shadow_tls", "res_tls", "jls_config":
			// Mihomo wrapper modes are needed by Shadowsocks plugin links. Other
			// URI builders simply ignore these internal fields.
			oTls[k] = v
		case "reality":
			reality, ok := v.(map[string]interface{})
			if !ok {
				continue
			}
			clientReality, ok := oTls["reality"].(map[string]interface{})
			if !ok || clientReality == nil {
				clientReality = map[string]interface{}{}
			}
			if enabled, exists := reality["enabled"]; exists {
				clientReality["enabled"] = enabled
			}
			if shortIDs, hasSIds := reality["short_id"].([]interface{}); hasSIds && len(shortIDs) > 0 {
				clientReality["short_id"] = shortIDs[common.RandomInt(len(shortIDs))]
			}
			oTls["reality"] = clientReality
		}
	}
	return oTls
}

func socksLink(userConfig map[string]interface{}, addrs []map[string]interface{}) []string {
	var links []string
	for _, addr := range addrs {
		port, _ := addr["server_port"].(float64)
		hostPort := FormatSubscriptionLinkHostPort(firstStringFromInterface(addr["server"]), int(port))
		if hostPort == "" {
			continue
		}
		links = append(links, fmt.Sprintf("socks5://%s:%s@%s", userConfig["username"], userConfig["password"], hostPort))
	}
	return links
}

func httpLink(userConfig map[string]interface{}, addrs []map[string]interface{}) []string {
	var links []string
	for _, addr := range addrs {
		protocol := "http"
		if addr["tls"] != nil {
			protocol = "https"
		}
		port, _ := addr["server_port"].(float64)
		hostPort := FormatSubscriptionLinkHostPort(firstStringFromInterface(addr["server"]), int(port))
		if hostPort == "" {
			continue
		}
		links = append(links, fmt.Sprintf("%s://%s:%s@%s", protocol, userConfig["username"], userConfig["password"], hostPort))
	}
	return links
}

func shadowsocksLink(
	userConfig map[string]map[string]interface{},
	inbound map[string]interface{},
	addrs []map[string]interface{}) []string {

	method, _ := inbound["method"].(string)
	// 服务端始终运行在单用户模式（无 users 数组），客户端直接使用入站密码即可。
	// 不需要拼接 serverPassword:userPassword 的多用户格式。
	inbPass, _ := inbound["password"].(string)

	uriBase := fmt.Sprintf("ss://%s", toBase64([]byte(fmt.Sprintf("%s:%s", method, inbPass))))

	var links []string
	for _, addr := range addrs {
		var params []LinkParam
		if tls, ok := addr["tls"].(map[string]interface{}); ok {
			if plugin := mihomoShadowsocksPluginLink(tls); plugin != "" {
				params = append(params, LinkParam{"plugin", plugin})
			}
		}
		port, _ := addr["server_port"].(float64)
		hostPort := FormatSubscriptionLinkHostPort(firstStringFromInterface(addr["server"]), int(port))
		if hostPort == "" {
			continue
		}
		uri := fmt.Sprintf("%s@%s", uriBase, hostPort)
		remark, _ := addr["remark"].(string)
		links = append(links, addParams(uri, params, remark))
	}
	return links
}

func mihomoShadowsocksPluginLink(tls map[string]interface{}) string {
	if tls == nil {
		return ""
	}
	var plugin string
	var opts []string
	var wrapper map[string]interface{}
	if value, ok := tls["shadow_tls"].(map[string]interface{}); ok && value != nil {
		plugin, wrapper = "shadow-tls", value
		appendMihomoPluginOption(&opts, "version", tlsMapString(tls["shadow_tls_opts"], "version"))
		appendMihomoPluginOption(&opts, "password", tlsMapString(tls["shadow_tls_opts"], "password"))
	} else if value, ok := tls["res_tls"].(map[string]interface{}); ok && value != nil {
		plugin, wrapper = "restls", value
		appendMihomoPluginOption(&opts, "password", tlsMapString(tls["restls_opts"], "password"))
		appendMihomoPluginOption(&opts, "version-hint", tlsMapString(tls["restls_opts"], "version_hint"))
		script := tlsMapString(tls["restls_opts"], "restls_script")
		if script == "" {
			script = tlsMapString(wrapper, "restls_script")
		}
		appendMihomoPluginOption(&opts, "restls-script", script)
	} else if value, ok := tls["jls_config"].(map[string]interface{}); ok && value != nil {
		plugin, wrapper = "jls", value
		appendMihomoPluginOption(&opts, "username", tlsMapString(tls["jls_opts"], "username"))
		appendMihomoPluginOption(&opts, "password", tlsMapString(tls["jls_opts"], "password"))
		if alpn := tlsStringList(wrapper["alpn"]); len(alpn) > 0 {
			appendMihomoPluginOption(&opts, "alpn", strings.Join(alpn, ","))
		}
	}
	if plugin == "" {
		return ""
	}
	host := firstStringFromInterface(tls["server_name"])
	if host == "" && wrapper != nil {
		host = firstStringFromInterface(wrapper["sni"])
		if host == "" {
			host = mihomoWrapperHostForLink(firstStringFromInterface(wrapper["dest"]))
		}
		if host == "" {
			if handshake, ok := wrapper["handshake"].(map[string]interface{}); ok {
				host = mihomoWrapperHostForLink(firstStringFromInterface(handshake["dest"]))
			}
		}
	}
	appendMihomoPluginOption(&opts, "host", host)
	return plugin + ";" + strings.Join(opts, ";")
}

func appendMihomoPluginOption(opts *[]string, key, value string) {
	if opts == nil || strings.TrimSpace(key) == "" || strings.TrimSpace(value) == "" {
		return
	}
	*opts = append(*opts, key+"="+value)
}

func tlsMapString(raw interface{}, key string) string {
	value, ok := raw.(map[string]interface{})
	if !ok || value == nil {
		return ""
	}
	return firstStringFromInterface(value[key])
}

func tlsStringList(raw interface{}) []string {
	var result []string
	switch values := raw.(type) {
	case []string:
		for _, value := range values {
			if value = strings.TrimSpace(value); value != "" {
				result = append(result, value)
			}
		}
	case []interface{}:
		for _, value := range values {
			if text := strings.TrimSpace(firstStringFromInterface(value)); text != "" {
				result = append(result, text)
			}
		}
	}
	return result
}

func mihomoWrapperHostForLink(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	if host, _, err := net.SplitHostPort(value); err == nil {
		return strings.Trim(host, "[]")
	}
	if strings.HasPrefix(value, "[") {
		if end := strings.Index(value, "]"); end > 1 {
			return value[1:end]
		}
	}
	if strings.Count(value, ":") == 1 {
		return strings.TrimSpace(strings.SplitN(value, ":", 2)[0])
	}
	return value
}

func naiveLink(
	userConfig map[string]interface{},
	inbound map[string]interface{},
	addrs []map[string]interface{}) []string {

	password, _ := userConfig["password"].(string)
	username, _ := userConfig["username"].(string)

	baseUri := "http2://"
	var links []string

	for _, addr := range addrs {
		var params []LinkParam
		params = append(params, LinkParam{"padding", "1"})
		if tls, ok := addr["tls"].(map[string]interface{}); ok {
			if sni, ok := tls["server_name"].(string); ok {
				params = append(params, LinkParam{"peer", sni})
			}
			if alpn, ok := tls["alpn"].([]interface{}); ok {
				alpnList := make([]string, len(alpn))
				for i, v := range alpn {
					alpnList[i] = v.(string)
				}
				params = append(params, LinkParam{"alpn", strings.Join(alpnList, ",")})
			}
			if insecure, ok := tls["insecure"].(bool); ok && insecure {
				params = append(params, LinkParam{"insecure", "1"})
			}
		}
		if tfo, ok := inbound["tcp_fast_open"].(bool); ok && tfo {
			params = append(params, LinkParam{"tfo", "1"})
		} else {
			params = append(params, LinkParam{"tfo", "0"})
		}

		port, _ := addr["server_port"].(float64)
		hostPort := FormatSubscriptionLinkHostPort(firstStringFromInterface(addr["server"]), int(port))
		if hostPort == "" {
			continue
		}
		uri := baseUri + toBase64([]byte(fmt.Sprintf("%s:%s@%s", username, password, hostPort)))
		links = append(links, addParams(uri, params, addr["remark"].(string)))
	}
	return links
}

func hysteriaLink(
	userConfig map[string]interface{},
	inbound map[string]interface{},
	addrs []map[string]interface{}) []string {

	baseUri := "hysteria://"
	var links []string

	for _, addr := range addrs {
		var params []LinkParam
		var outJson map[string]interface{}
		if rawOutJSON, ok := inbound["out_json"].(json.RawMessage); ok && len(rawOutJSON) > 0 {
			_ = json.Unmarshal(rawOutJSON, &outJson)
		}
		if upmbps, ok := outJson["up_mbps"].(float64); ok && upmbps > 0 {
			params = append(params, LinkParam{"upmbps", fmt.Sprintf("%.0f", upmbps)})
		}
		if downmbps, ok := outJson["down_mbps"].(float64); ok && downmbps > 0 {
			params = append(params, LinkParam{"downmbps", fmt.Sprintf("%.0f", downmbps)})
		}
		if auth, ok := userConfig["auth_str"].(string); ok {
			params = append(params, LinkParam{"auth", auth})
		}
		if tls, ok := addr["tls"].(map[string]interface{}); ok {
			getTlsParams(&params, tls, "insecure")
		}
		if obfs, ok := inbound["obfs"].(string); ok {
			params = append(params, LinkParam{"obfs", obfs})
		}
		if tfo, ok := inbound["tcp_fast_open"].(bool); ok && tfo {
			params = append(params, LinkParam{"fastopen", "1"})
		} else {
			params = append(params, LinkParam{"fastopen", "0"})
		}
		if err := json.Unmarshal(inbound["out_json"].(json.RawMessage), &outJson); err != nil {
			return []string{} // Handle error
		}
		if mport, ok := outJson["server_ports"].([]interface{}); ok {
			mportList := make([]string, len(mport))
			for i, v := range mport {
				mportList[i] = v.(string)
			}
			params = append(params, LinkParam{"mport", strings.Join(mportList, ",")})
		}

		port, _ := addr["server_port"].(float64)
		hostPort := FormatSubscriptionLinkHostPort(firstStringFromInterface(addr["server"]), int(port))
		if hostPort == "" {
			continue
		}
		uri := fmt.Sprintf("%s%s", baseUri, hostPort)
		links = append(links, addParams(uri, params, addr["remark"].(string)))
	}

	return links
}

func hysteria2Link(
	userConfig map[string]interface{},
	inbound map[string]interface{},
	addrs []map[string]interface{}) []string {

	password, _ := userConfig["password"].(string)
	baseUri := fmt.Sprintf("%s%s@", "hysteria2://", password)
	var links []string

	for _, addr := range addrs {
		var params []LinkParam
		var outJson map[string]interface{}
		if rawOutJSON, ok := inbound["out_json"].(json.RawMessage); ok && len(rawOutJSON) > 0 {
			_ = json.Unmarshal(rawOutJSON, &outJson)
		}
		if upmbps, ok := outJson["up_mbps"].(float64); ok && upmbps > 0 {
			params = append(params, LinkParam{"upmbps", fmt.Sprintf("%.0f", upmbps)})
		}
		if downmbps, ok := outJson["down_mbps"].(float64); ok && downmbps > 0 {
			params = append(params, LinkParam{"downmbps", fmt.Sprintf("%.0f", downmbps)})
		}
		if tls, ok := addr["tls"].(map[string]interface{}); ok {
			getTlsParams(&params, tls, "insecure")
		}
		if obfs, ok := inbound["obfs"].(map[string]interface{}); ok {
			if obfsType, ok := obfs["type"].(string); ok {
				params = append(params, LinkParam{"obfs", obfsType})
			}
			if obfsPassword, ok := obfs["password"].(string); ok {
				params = append(params, LinkParam{"obfs-password", obfsPassword})
			}
		}
		if tfo, ok := inbound["tcp_fast_open"].(bool); ok && tfo {
			params = append(params, LinkParam{"fastopen", "1"})
		} else {
			params = append(params, LinkParam{"fastopen", "0"})
		}
		if err := json.Unmarshal(inbound["out_json"].(json.RawMessage), &outJson); err != nil {
			return []string{} // Handle error
		}
		if mport, ok := outJson["server_ports"].([]interface{}); ok {
			mportList := make([]string, len(mport))
			for i, v := range mport {
				mportList[i] = v.(string)
			}
			params = append(params, LinkParam{"mport", strings.Join(mportList, ",")})
		}

		port, _ := addr["server_port"].(float64)
		hostPort := FormatSubscriptionLinkHostPort(firstStringFromInterface(addr["server"]), int(port))
		if hostPort == "" {
			continue
		}
		uri := fmt.Sprintf("%s%s", baseUri, hostPort)
		links = append(links, addParams(uri, params, addr["remark"].(string)))
	}

	return links
}

func anytlsLink(
	userConfig map[string]interface{},
	addrs []map[string]interface{}) []string {

	password, _ := userConfig["password"].(string)
	baseUri := fmt.Sprintf("%s%s@", "anytls://", password)
	var links []string

	for _, addr := range addrs {
		var params []LinkParam
		if tls, ok := addr["tls"].(map[string]interface{}); ok {
			getTlsParams(&params, tls, "insecure")
		}

		port, _ := addr["server_port"].(float64)
		hostPort := FormatSubscriptionLinkHostPort(firstStringFromInterface(addr["server"]), int(port))
		if hostPort == "" {
			continue
		}
		uri := fmt.Sprintf("%s%s", baseUri, hostPort)
		links = append(links, addParams(uri, params, addr["remark"].(string)))
	}

	return links
}

func snellLink(
	userConfig map[string]interface{},
	inbound map[string]interface{},
	addrs []map[string]interface{},
) []string {
	psk, _ := userConfig["psk"].(string)
	psk = strings.TrimSpace(psk)
	if psk == "" {
		return nil
	}

	settings := extractLinkOutboundSettings(inbound["out_json"])

	version := "5"
	if clientVersion, ok := positiveSnellVersionFromAny(settings["version"]); ok {
		version = fmt.Sprintf("%d", clientVersion)
	} else if inboundVersion, ok := positiveSnellVersionFromAny(inbound["version"]); ok {
		version = fmt.Sprintf("%d", inboundVersion)
	}

	obfsOpts, _ := settings["obfs_opts"].(map[string]interface{})
	if obfsOpts == nil {
		obfsOpts, _ = inbound["obfs_opts"].(map[string]interface{})
	}
	if obfsOpts == nil {
		obfsOpts, _ = inbound["obfs-opts"].(map[string]interface{})
	}

	links := make([]string, 0, len(addrs))
	for _, addr := range addrs {
		var params []LinkParam
		params = append(params, LinkParam{Key: "version", Value: version})
		appendBooleanLinkParam(&params, settings, "reuse")
		if obfsOpts != nil {
			if mode, ok := obfsOpts["mode"].(string); ok && strings.TrimSpace(mode) != "" {
				params = append(params, LinkParam{Key: "obfs", Value: strings.TrimSpace(mode)})
				host := "www.bing.com"
				if customHost, ok := obfsOpts["host"].(string); ok && strings.TrimSpace(customHost) != "" {
					host = strings.TrimSpace(customHost)
				}
				params = append(params, LinkParam{Key: "host", Value: host})
			}
		}

		port, _ := addr["server_port"].(float64)
		hostPort := FormatSubscriptionLinkHostPort(firstStringFromInterface(addr["server"]), int(port))
		if hostPort == "" {
			continue
		}
		uri := fmt.Sprintf("snell://%s@%s", url.User(psk).String(), hostPort)
		remark, _ := addr["remark"].(string)
		links = append(links, addParams(uri, params, remark))
	}

	return links
}

func positiveSnellVersionFromAny(value interface{}) (int, bool) {
	switch v := value.(type) {
	case int:
		return v, v > 0
	case int8:
		return int(v), v > 0
	case int16:
		return int(v), v > 0
	case int32:
		return int(v), v > 0
	case int64:
		return int(v), v > 0
	case float32:
		return int(v), v > 0
	case float64:
		return int(v), v > 0
	case json.Number:
		parsed, err := v.Int64()
		if err == nil {
			return int(parsed), parsed > 0
		}
		return 0, false
	default:
		return 0, false
	}
}

func mieruLink(
	userConfig map[string]interface{},
	inbound map[string]interface{},
	addrs []map[string]interface{}) []string {

	username, _ := userConfig["username"].(string)
	password, _ := userConfig["password"].(string)
	if username == "" || password == "" {
		return nil
	}

	transport := NormalizeMieruTransport(firstStringFromInterface(inbound["transport"]))
	profile := strings.TrimSpace(firstStringFromInterface(inbound["tag"]))
	if profile == "" {
		profile = "default"
	}

	var outJSON map[string]interface{}
	if raw, ok := inbound["out_json"].(json.RawMessage); ok && len(raw) > 0 {
		_ = json.Unmarshal(raw, &outJSON)
	}

	multiplexing := NormalizeMieruMultiplexing(firstStringFromInterface(outJSON["multiplexing"]))
	handshakeMode := NormalizeMieruHandshakeMode(firstStringFromInterface(outJSON["handshake_mode"]))

	links := make([]string, 0, len(addrs))
	for _, addr := range addrs {
		host := NormalizeSubscriptionServerHost(firstStringFromInterface(addr["server"]))
		remark, _ := addr["remark"].(string)
		if host == "" {
			continue
		}

		bindings := NormalizeMieruPortBindings(firstStringFromInterface(inbound["port_bindings"]))
		if len(bindings) == 0 {
			if portRange, ok := NormalizeMieruPortRange(firstStringFromInterface(inbound["port_range"])); ok {
				bindings = []string{portRange}
			}
		}
		if len(bindings) == 0 {
			if port, ok := addr["server_port"].(float64); ok && port > 0 {
				bindings = []string{fmt.Sprintf("%.0f", port)}
			} else if port, ok := inbound["listen_port"].(float64); ok && port > 0 {
				bindings = []string{fmt.Sprintf("%.0f", port)}
			}
		}
		if len(bindings) == 0 {
			continue
		}

		for _, binding := range bindings {
			u := &url.URL{
				Scheme:   "mierus",
				User:     url.UserPassword(username, password),
				Host:     FormatSubscriptionLinkHost(host),
				Fragment: remark,
			}
			query := url.Values{}
			query.Set("profile", profile)
			query.Add("port", BuildMieruBindingQueryValue(binding))
			query.Add("protocol", transport)
			if multiplexing != "" {
				query.Set("multiplexing", multiplexing)
			}
			if handshakeMode != "" {
				query.Set("handshake-mode", handshakeMode)
			}
			u.RawQuery = query.Encode()
			links = append(links, u.String())
		}
	}

	return links
}

func tuicLink(
	userConfig map[string]interface{},
	inbound map[string]interface{},
	addrs []map[string]interface{}) []string {

	settings := extractLinkOutboundSettings(inbound["out_json"])
	password := firstLinkString(userConfig, settings, "password")
	uuid := firstLinkString(userConfig, settings, "uuid")
	token := firstLinkString(userConfig, settings, "token")
	var links []string

	for _, addr := range addrs {
		var params []LinkParam
		if tls, ok := addr["tls"].(map[string]interface{}); ok {
			getTlsParams(&params, tls, "insecure")
		}
		if congestionControl := firstLinkString(settings, inbound, "congestion_control"); congestionControl != "" {
			params = append(params, LinkParam{"congestion_control", congestionControl})
		}
		if relayMode := firstLinkString(settings, inbound, "udp_relay_mode"); relayMode != "" {
			params = append(params, LinkParam{"udp_relay_mode", relayMode})
		}
		if heartbeat := firstLinkString(settings, inbound, "heartbeat"); heartbeat != "" {
			params = append(params, LinkParam{"heartbeat", heartbeat})
		}
		if requestTimeout := firstLinkString(settings, inbound, "request_timeout"); requestTimeout != "" {
			params = append(params, LinkParam{"request_timeout", requestTimeout})
		}
		if ip := firstLinkString(settings, inbound, "ip"); ip != "" {
			params = append(params, LinkParam{"ip", ip})
		}
		appendNumericLinkParam(&params, settings, "max_open_streams")
		appendNumericLinkParam(&params, settings, "max_udp_relay_packet_size")
		appendNumericLinkParam(&params, settings, "cwnd")
		appendNumericLinkParam(&params, settings, "udp_over_stream_version")
		appendNumericLinkParam(&params, settings, "max_datagram_frame_size")
		appendBooleanLinkParam(&params, settings, "zero_rtt_handshake")
		appendBooleanLinkParamAs(&params, settings, "mihomo_fast_open", "fast_open")
		appendBooleanLinkParam(&params, settings, "udp_over_stream")
		appendBooleanLinkParam(&params, settings, "disable_mtu_discovery")

		port, _ := addr["server_port"].(float64)
		userInfo := buildTUICUserInfo(token, uuid, password)
		if userInfo == "" {
			continue
		}
		hostPort := FormatSubscriptionLinkHostPort(firstStringFromInterface(addr["server"]), int(port))
		if hostPort == "" {
			continue
		}
		uri := fmt.Sprintf("tuic://%s@%s", userInfo, hostPort)
		links = append(links, addParams(uri, params, addr["remark"].(string)))
	}

	return links
}

func extractLinkOutboundSettings(raw interface{}) map[string]interface{} {
	switch value := raw.(type) {
	case map[string]interface{}:
		return value
	case json.RawMessage:
		var payload map[string]interface{}
		if err := json.Unmarshal(value, &payload); err == nil {
			return payload
		}
	case []byte:
		var payload map[string]interface{}
		if err := json.Unmarshal(value, &payload); err == nil {
			return payload
		}
	}
	return nil
}

func firstLinkString(sources ...interface{}) string {
	if len(sources) < 2 {
		return ""
	}

	key, ok := sources[len(sources)-1].(string)
	if !ok || key == "" {
		return ""
	}

	for _, source := range sources[:len(sources)-1] {
		valueMap, ok := source.(map[string]interface{})
		if !ok || valueMap == nil {
			continue
		}
		if value, ok := valueMap[key].(string); ok {
			value = strings.TrimSpace(value)
			if value != "" {
				return value
			}
		}
	}
	return ""
}

func appendNumericLinkParam(params *[]LinkParam, source map[string]interface{}, key string) {
	if params == nil || source == nil || key == "" {
		return
	}
	switch value := source[key].(type) {
	case int:
		if value > 0 {
			*params = append(*params, LinkParam{key, fmt.Sprintf("%d", value)})
		}
	case int32:
		if value > 0 {
			*params = append(*params, LinkParam{key, fmt.Sprintf("%d", value)})
		}
	case int64:
		if value > 0 {
			*params = append(*params, LinkParam{key, fmt.Sprintf("%d", value)})
		}
	case float32:
		if value > 0 {
			*params = append(*params, LinkParam{key, fmt.Sprintf("%d", int(value))})
		}
	case float64:
		if value > 0 {
			*params = append(*params, LinkParam{key, fmt.Sprintf("%d", int(value))})
		}
	}
}

func appendBooleanLinkParam(params *[]LinkParam, source map[string]interface{}, key string) {
	appendBooleanLinkParamAs(params, source, key, key)
}

func appendBooleanLinkParamAs(params *[]LinkParam, source map[string]interface{}, sourceKey string, queryKey string) {
	if params == nil || source == nil {
		return
	}
	if sourceKey == "" || queryKey == "" {
		return
	}
	if value, ok := source[sourceKey].(bool); ok && value {
		*params = append(*params, LinkParam{queryKey, "1"})
	}
}

func buildTUICUserInfo(token string, uuid string, password string) string {
	token = strings.TrimSpace(token)
	uuid = strings.TrimSpace(uuid)
	password = strings.TrimSpace(password)
	if token != "" {
		return url.User(token).String()
	}
	if uuid == "" {
		return ""
	}
	return url.UserPassword(uuid, password).String()
}

func vlessLink(
	userConfig map[string]interface{},
	inbound map[string]interface{},
	addrs []map[string]interface{}) []string {

	uuid, _ := userConfig["uuid"].(string)
	baseParams := getTransportParams(inbound["transport"])
	settings := extractLinkOutboundSettings(inbound["out_json"])
	var links []string

	for _, addr := range addrs {
		params := make([]LinkParam, len(baseParams))
		copy(params, baseParams)
		if encryption := firstLinkString(settings, "encryption"); encryption != "" {
			params = append(params, LinkParam{"encryption", encryption})
		}
		if tls, ok := addr["tls"].(map[string]interface{}); ok {
			if enabled, _ := tls["enabled"].(bool); enabled {
				getTlsParams(&params, tls, "allowInsecure")
				if flow, ok := userConfig["flow"].(string); ok {
					params = append(params, LinkParam{"flow", flow})
				}
			}
		}
		port, _ := addr["server_port"].(float64)
		hostPort := FormatSubscriptionLinkHostPort(firstStringFromInterface(addr["server"]), int(port))
		if hostPort == "" {
			continue
		}
		uri := fmt.Sprintf("vless://%s@%s", uuid, hostPort)
		uri = addParams(uri, params, addr["remark"].(string))
		links = append(links, uri)
	}

	return links
}

func trojanLink(
	userConfig map[string]interface{},
	inbound map[string]interface{},
	addrs []map[string]interface{}) []string {
	password, _ := userConfig["password"].(string)
	baseParams := getTransportParams(inbound["transport"])
	var links []string

	for _, addr := range addrs {
		params := make([]LinkParam, len(baseParams))
		copy(params, baseParams)
		if tls, ok := addr["tls"].(map[string]interface{}); ok {
			if enabled, _ := tls["enabled"].(bool); enabled {
				getTlsParams(&params, tls, "allowInsecure")
			}
		}
		port, _ := addr["server_port"].(float64)
		hostPort := FormatSubscriptionLinkHostPort(firstStringFromInterface(addr["server"]), int(port))
		if hostPort == "" {
			continue
		}
		uri := fmt.Sprintf("trojan://%s@%s", password, hostPort)
		uri = addParams(uri, params, addr["remark"].(string))
		links = append(links, uri)
	}

	return links
}

func vmessLink(
	userConfig map[string]interface{},
	inbound map[string]interface{},
	addrs []map[string]interface{}) []string {

	uuid, _ := userConfig["uuid"].(string)
	transportParams := getTransportParams(inbound["transport"])
	var links []string

	baseParams := map[string]interface{}{
		"v":   "2",
		"id":  uuid,
		"aid": 0,
	}

	var net, typ, host, path string
	for _, p := range transportParams {
		switch p.Key {
		case "type":
			net = p.Value
		case "host":
			host = p.Value
		case "path":
			path = p.Value
		}
	}

	if net == "http" || net == "tcp" {
		baseParams["net"] = "tcp"
		if net == "http" {
			typ = "http"
		}
	} else {
		baseParams["net"] = net
	}

	for _, addr := range addrs {
		obj := make(map[string]interface{})
		for k, v := range baseParams {
			obj[k] = v
		}

		obj["add"] = NormalizeSubscriptionServerHost(firstStringFromInterface(addr["server"]))
		port, _ := addr["server_port"].(float64)
		obj["port"] = fmt.Sprintf("%.0f", port)
		obj["ps"], _ = addr["remark"].(string)
		if typ != "" {
			obj["type"] = typ
		}
		if host != "" {
			obj["host"] = host
		}
		if path != "" {
			obj["path"] = path
		}
		populateVmessTlsParams(obj, addr["tls"])

		jsonStr, _ := json.Marshal(obj)

		uri := fmt.Sprintf("vmess://%s", toBase64(jsonStr))
		links = append(links, uri)
	}
	return links
}

func populateVmessTlsParams(obj map[string]interface{}, tlsConfig interface{}) {
	if tlsMap, ok := tlsConfig.(map[string]interface{}); ok {
		enabled, _ := tlsMap["enabled"].(bool)
		if !enabled {
			obj["tls"] = "none"
			return
		}
		obj["tls"] = "tls"
		var tlsParams []LinkParam
		getTlsParams(&tlsParams, tlsMap, "allowInsecure")
		for _, p := range tlsParams {
			switch p.Key {
			case "security":
				// ignore, as "tls" is already set
			case "allowInsecure":
				obj["allowInsecure"] = 1
			case "sni":
				obj["sni"] = p.Value
			case "fp":
				obj["fp"] = p.Value
			case "alpn":
				obj["alpn"] = p.Value
			}
		}
		return
	}
	obj["tls"] = "none"
}

func toBase64(d []byte) string {
	return base64.StdEncoding.EncodeToString(d)
}

func addParams(uri string, params []LinkParam, remark string) string {
	URL, _ := url.Parse(uri)
	var q []string
	for _, p := range params {
		switch p.Key {
		case "mport", "alpn":
			q = append(q, fmt.Sprintf("%s=%s", p.Key, p.Value))
		default:
			q = append(q, fmt.Sprintf("%s=%s", p.Key, url.QueryEscape(p.Value)))
		}
	}
	URL.RawQuery = strings.Join(q, "&")
	URL.Fragment = remark
	return URL.String()
}

func firstStringFromInterface(raw interface{}) string {
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

func getTransportParams(t interface{}) []LinkParam {
	var params []LinkParam
	trasport, _ := t.(map[string]interface{})
	var transportType string
	if tt, ok := trasport["type"].(string); ok {
		transportType = tt
	} else {
		transportType = "tcp"
	}
	params = append(params, LinkParam{"type", transportType})
	if transportType == "tcp" {
		return params
	}

	switch transportType {
	case "http":
		if host, ok := trasport["host"].([]interface{}); ok {
			var hosts []string
			for _, v := range host {
				hosts = append(hosts, v.(string))
			}
			params = append(params, LinkParam{"host", strings.Join(hosts, ",")})
		}
		if path, ok := trasport["path"].(string); ok {
			params = append(params, LinkParam{"path", path})
		}
	case "ws":
		if path, ok := trasport["path"].(string); ok {
			params = append(params, LinkParam{"path", path})
		}
		if headers, ok := trasport["headers"].(map[string]interface{}); ok {
			if host, ok := headers["Host"].(string); ok {
				params = append(params, LinkParam{"host", host})
			}
		}
	case "grpc":
		if serviceName, ok := trasport["service_name"].(string); ok {
			params = append(params, LinkParam{"serviceName", serviceName})
		}
	case "httpupgrade":
		if host, ok := trasport["host"].(string); ok {
			params = append(params, LinkParam{"host", host})
		}
		if path, ok := trasport["path"].(string); ok {
			params = append(params, LinkParam{"path", path})
		}
	}
	return params
}

func getTlsParams(params *[]LinkParam, tls map[string]interface{}, insecureKey string) {
	realityEnabled := false
	if reality, ok := tls["reality"].(map[string]interface{}); ok {
		if enabled, _ := reality["enabled"].(bool); enabled {
			realityEnabled = true
			*params = append(*params, LinkParam{"security", "reality"})
			if pbk, ok := reality["public_key"].(string); ok {
				*params = append(*params, LinkParam{"pbk", pbk})
			}
			if sid, ok := reality["short_id"].(string); ok {
				*params = append(*params, LinkParam{"sid", sid})
			}
		}
	}
	if !realityEnabled {
		*params = append(*params, LinkParam{"security", "tls"})
		if insecure, ok := tls["insecure"].(bool); ok && insecure {
			*params = append(*params, LinkParam{insecureKey, "1"})
		}
		if disableSni, ok := tls["disable_sni"].(bool); ok && disableSni {
			*params = append(*params, LinkParam{"disable_sni", "1"})
		}
	}
	if utls, ok := tls["utls"].(map[string]interface{}); ok {
		if fingerprint, ok := utls["fingerprint"].(string); ok {
			*params = append(*params, LinkParam{"fp", fingerprint})
		}
	}
	if sni, ok := tls["server_name"].(string); ok {
		*params = append(*params, LinkParam{"sni", sni})
	}
	var alpnList []string
	switch alpn := tls["alpn"].(type) {
	case []interface{}:
		for _, rawValue := range alpn {
			value, ok := rawValue.(string)
			if !ok {
				continue
			}
			value = strings.TrimSpace(value)
			if value != "" {
				alpnList = append(alpnList, value)
			}
		}
	case []string:
		for _, rawValue := range alpn {
			value := strings.TrimSpace(rawValue)
			if value != "" {
				alpnList = append(alpnList, value)
			}
		}
	}
	if len(alpnList) > 0 {
		*params = append(*params, LinkParam{"alpn", strings.Join(alpnList, ",")})
	}
}
