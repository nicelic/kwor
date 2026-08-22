package util

import (
	"encoding/json"
	"io"
	"math"
	"net"
	"os"
	"strings"

	"github.com/alireza0/s-ui/logger"
	"github.com/alireza0/s-ui/util/common"

	"github.com/alireza0/s-ui/database/model"
)

const maxTLSPEMFileBytes int64 = 2 * 1024 * 1024

// readPemFile 读取 PEM 文件并返回行数组
func readPemFile(path string) []string {
	file, err := os.Open(path)
	if err != nil {
		logger.Warningf("failed to read PEM file %s: %v", path, err)
		return nil
	}
	defer file.Close()

	data, err := io.ReadAll(io.LimitReader(file, maxTLSPEMFileBytes+1))
	if err != nil {
		logger.Warningf("failed to read PEM file %s: %v", path, err)
		return nil
	}
	if int64(len(data)) > maxTLSPEMFileBytes {
		logger.Warningf("PEM file %s exceeds the %d byte limit", path, maxTLSPEMFileBytes)
		return nil
	}

	content := strings.TrimSpace(string(data))
	if content == "" {
		return nil
	}
	return strings.Split(content, "\n")
}

func hasNonEmptySlice(value interface{}) bool {
	switch v := value.(type) {
	case []interface{}:
		return len(v) > 0
	case []string:
		return len(v) > 0
	default:
		return false
	}
}

func positiveIntFromAny(value interface{}) (int, bool) {
	maxInt := int64(^uint(0) >> 1)
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
		return int(v), v > 0 && v <= maxInt
	case float32:
		value := float64(v)
		if value <= 0 || math.IsNaN(value) || math.IsInf(value, 0) || math.Trunc(value) != value || value > float64(maxInt) {
			return 0, false
		}
		return int(value), true
	case float64:
		if v <= 0 || math.IsNaN(v) || math.IsInf(v, 0) || math.Trunc(v) != v || v > float64(maxInt) {
			return 0, false
		}
		return int(v), true
	default:
		return 0, false
	}
}

func SanitizeOptionalNetworkField(outbound map[string]interface{}) {
	if outbound == nil {
		return
	}
	if value, ok := outbound["network"].(string); ok && strings.TrimSpace(value) == "" {
		delete(outbound, "network")
	}
}

// Fill Inbound's out_json
func FillOutJson(i *model.Inbound, hostname string) error {
	switch i.Type {
	case "direct", "tun", "redirect", "tproxy":
		return nil
	}
	var outJson map[string]interface{}
	err := json.Unmarshal(i.OutJson, &outJson)
	if err != nil {
		return err
	}

	if outJson == nil {
		outJson = make(map[string]interface{})
	}
	// Older panel builds could leave list-view metadata in the reusable
	// outbound template. It must not survive into subscriptions.
	StripSubscriptionOutboundPanelFields(outJson)

	if i.Type == "shadowquic" {
		// ShadowQUIC has native sni/alpn fields and must never inherit mihomo_TLS.
		delete(outJson, "tls")
	} else if i.TlsId > 0 {
		addTls(&outJson, i.Tls, i.Type)
	} else {
		delete(outJson, "tls")
		if strings.EqualFold(strings.TrimSpace(i.Type), "shadowsocks") {
			clearMihomoShadowsocksPlugin(outJson)
		}
	}

	inbound, err := i.MarshalFull()
	if err != nil {
		return err
	}

	outJson["type"] = i.Type
	outJson["tag"] = i.Tag
	outJson["server"] = NormalizeSubscriptionServerHost(hostname)
	outJson["server_port"] = (*inbound)["listen_port"]

	switch i.Type {
	case "http", "socks", "mixed", "anytls":
	case "snell":
		snellOut(&outJson, *inbound)
	case "trusttunnel":
		trustTunnelOut(&outJson, *inbound)
	case "ssh":
		sshOut(&outJson, *inbound)
	case "naive":
		naiveOut(&outJson, *inbound)
	case "shadowsocks":
		shadowsocksOut(&outJson, *inbound)
	case "shadowtls":
		shadowTlsOut(&outJson, *inbound)
	case "shadowquic":
		shadowQUICOut(&outJson, *inbound)
	case "hysteria":
		hysteriaOut(&outJson, *inbound)
	case "hysteria2":
		hysteria2Out(&outJson, *inbound)
	case "mieru":
		mieruOut(&outJson, *inbound)
	case "sudoku":
		sudokuOut(&outJson, *inbound)
	case "tuic":
		tuicOut(&outJson, *inbound)
	case "vless":
		vlessOut(&outJson, *inbound)
	case "trojan":
		trojanOut(&outJson, *inbound)
	case "vmess":
		vmessOut(&outJson, *inbound)
	default:
		for key := range outJson {
			delete(outJson, key)
		}
	}

	i.OutJson, err = json.MarshalIndent(outJson, "", "  ")
	if err != nil {
		return err
	}

	return nil
}

// addTls function
func addTls(out *map[string]interface{}, tls *model.Tls, inboundType ...string) {
	if out == nil || tls == nil {
		return
	}

	var tlsServer, tlsConfig map[string]interface{}
	err := json.Unmarshal(tls.Server, &tlsServer)
	if err != nil {
		return
	}
	err = json.Unmarshal(tls.Client, &tlsConfig)
	if err != nil {
		return
	}
	if tlsServer == nil {
		tlsServer = map[string]interface{}{}
	}
	if tlsConfig == nil {
		tlsConfig = map[string]interface{}{}
	}

	mode := tls.Mode
	if mode == "" {
		mode = model.NormalizeMihomoTlsMode("", tls.Server, tls.Client)
	}
	if len(inboundType) > 0 && strings.EqualFold(strings.TrimSpace(inboundType[0]), "shadowsocks") {
		clearMihomoShadowsocksPlugin(*out)
	}
	if mode == model.MihomoTlsModeShadowTLS || mode == model.MihomoTlsModeRestls || mode == model.MihomoTlsModeJLS {
		if len(inboundType) > 0 && strings.EqualFold(strings.TrimSpace(inboundType[0]), "shadowsocks") {
			addMihomoShadowsocksWrapper(out, tlsConfig, tlsServer, mode)
			return
		}
		addMihomoWrapperTLS(tlsConfig, tlsServer, mode)
		(*out)["tls"] = tlsConfig
		return
	}

	// Backward compatible default: keep adding server certificate/SHA256 unless explicitly disabled.
	includeServerCertificate := true
	if include, ok := tlsConfig["include_server_certificate"].(bool); ok {
		includeServerCertificate = include
	}
	delete(tlsConfig, "include_server_certificate")
	delete(tlsConfig, "include_server_fingerprint")

	if enabled, ok := tlsServer["enabled"]; ok {
		tlsConfig["enabled"] = enabled
	}
	if serverName, ok := tlsServer["server_name"]; ok {
		tlsConfig["server_name"] = serverName
	}
	if alpn, ok := tlsServer["alpn"]; ok {
		tlsConfig["alpn"] = alpn
	}
	if minVersion, ok := tlsServer["min_version"]; ok {
		tlsConfig["min_version"] = minVersion
	}
	if maxVersion, ok := tlsServer["max_version"]; ok {
		tlsConfig["max_version"] = maxVersion
	}
	if includeServerCertificate {
		// certificate_public_key_sha256 has higher priority than certificate chain verification.
		// When hash list is empty or not set, fall back to certificate PEM verification.
		hasServerCertificateSHA256 := hasNonEmptySlice(tlsConfig["certificate_public_key_sha256"])
		if hasServerCertificateSHA256 {
			delete(tlsConfig, "certificate")
			delete(tlsConfig, "certificate_path")
		} else {
			if certificate, ok := tlsServer["certificate"]; ok {
				tlsConfig["certificate"] = certificate
			} else if certPath, ok := tlsServer["certificate_path"].(string); ok && certPath != "" {
				lines := readPemFile(certPath)
				if lines != nil {
					tlsConfig["certificate"] = lines
				}
			}
		}
	} else {
		// Explicitly disable server-side certificate pinning material in generated client JSON.
		delete(tlsConfig, "certificate_public_key_sha256")
		delete(tlsConfig, "certificate")
		delete(tlsConfig, "certificate_path")
	}
	if cipherSuites, ok := tlsServer["cipher_suites"]; ok {
		tlsConfig["cipher_suites"] = cipherSuites
	}
	if reality, ok := tlsServer["reality"].(map[string]interface{}); ok {
		realityEnabled, enabledOK := reality["enabled"].(bool)
		if enabledOK && realityEnabled {
			realityConfig, configOK := tlsConfig["reality"].(map[string]interface{})
			if !configOK || realityConfig == nil {
				realityConfig = map[string]interface{}{}
			}
			realityConfig["enabled"] = true
			if shortIDs, ok := reality["short_id"].([]interface{}); ok && len(shortIDs) > 0 {
				realityConfig["short_id"] = shortIDs[common.RandomInt(len(shortIDs))]
			}
			tlsConfig["reality"] = realityConfig
		}
	}

	if ech, ok := tlsServer["ech"].(map[string]interface{}); ok {
		echEnabled, enabledOK := ech["enabled"].(bool)
		if enabledOK && echEnabled {
			echConfig, configOK := tlsConfig["ech"].(map[string]interface{})
			if !configOK || echConfig == nil {
				echConfig = map[string]interface{}{}
			}
			echConfig["enabled"] = true
			echConfig["pq_signature_schemes_enabled"] = ech["pq_signature_schemes_enabled"]
			echConfig["dynamic_record_sizing_disabled"] = ech["dynamic_record_sizing_disabled"]
			tlsConfig["ech"] = echConfig
		}
	}

	// mTLS: 当 client_authentication 不为 "no" 时，从 tls.Client 中读取客户端证书/密钥
	// 正确的 mTLS 配置：
	//   服务端(inbound): certificate + key (服务器证书), client_certificate (客户端CA证书，用于验证客户端)
	//   客户端(outbound): certificate (服务端CA证书，用于验证服务端), client_certificate + client_key (客户端证书+私钥)
	// tls.Client 中已经存储了独立的客户端证书和私钥，直接使用即可
	if clientAuth, ok := tlsServer["client_authentication"].(string); ok && clientAuth != "" && clientAuth != "no" {
		// 从 tls.Client 读取客户端证书（已由前端独立生成/设置）
		if cert, ok := tlsConfig["client_certificate"]; ok && cert != nil {
			// 已经在 tlsConfig 中，保持不变
		} else if certPath, ok := tlsConfig["client_certificate_path"].(string); ok && certPath != "" {
			// 路径模式：读取文件内容内联到配置中
			lines := readPemFile(certPath)
			if lines != nil {
				tlsConfig["client_certificate"] = lines
			}
			delete(tlsConfig, "client_certificate_path")
		}

		// 从 tls.Client 读取客户端私钥（已由前端独立生成/设置）
		if key, ok := tlsConfig["client_key"]; ok && key != nil {
			// 已经在 tlsConfig 中，保持不变
		} else if keyPath, ok := tlsConfig["client_key_path"].(string); ok && keyPath != "" {
			// 路径模式：读取文件内容内联到配置中
			lines := readPemFile(keyPath)
			if lines != nil {
				tlsConfig["client_key"] = lines
			}
			delete(tlsConfig, "client_key_path")
		}
	}

	// 清理路径字段：outbound 配置中不应包含路径字段，只使用内联内容
	delete(tlsConfig, "client_certificate_path")
	delete(tlsConfig, "client_key_path")

	// Certificate store must live at top-level "certificate.store", not inside outbound tls.
	delete(tlsConfig, "tls_store")
	delete(tlsConfig, "store")

	(*out)["tls"] = tlsConfig
}

func addMihomoShadowsocksWrapper(out *map[string]interface{}, client, server map[string]interface{}, mode string) {
	if out == nil {
		return
	}
	pluginOpts := map[string]interface{}{}
	wrapperHost := ""
	if value := strings.TrimSpace(firstStringFromAny(client["server_name"])); value != "" {
		wrapperHost = value
	}
	var wrapper map[string]interface{}
	switch mode {
	case model.MihomoTlsModeShadowTLS:
		wrapper, _ = server["shadow_tls"].(map[string]interface{})
	case model.MihomoTlsModeRestls:
		wrapper, _ = server["res_tls"].(map[string]interface{})
	case model.MihomoTlsModeJLS:
		wrapper, _ = server["jls_config"].(map[string]interface{})
	}
	if wrapper != nil {
		if wrapperHost == "" {
			wrapperHost = strings.TrimSpace(firstStringFromAny(wrapper["sni"]))
		}
		if wrapperHost == "" {
			wrapperHost = mihomoWrapperHost(firstStringFromAny(wrapper["dest"]))
		}
		if wrapperHost == "" {
			if handshake, ok := wrapper["handshake"].(map[string]interface{}); ok && handshake != nil {
				wrapperHost = mihomoWrapperHost(firstStringFromAny(handshake["dest"]))
			}
		}
	}
	if wrapperHost != "" {
		pluginOpts["host"] = wrapperHost
	}
	switch mode {
	case model.MihomoTlsModeShadowTLS:
		if wrapper != nil {
			if version, ok := positiveIntFromAny(wrapper["version"]); ok {
				pluginOpts["version"] = version
			}
		}
		if opts, ok := client["shadow_tls_opts"].(map[string]interface{}); ok && opts != nil {
			if password := strings.TrimSpace(firstStringFromAny(opts["password"])); password != "" {
				pluginOpts["password"] = password
			}
			if version, ok := positiveIntFromAny(opts["version"]); ok {
				pluginOpts["version"] = version
			}
		}
		(*out)["plugin"] = "shadow-tls"
	case model.MihomoTlsModeRestls:
		if opts, ok := client["restls_opts"].(map[string]interface{}); ok && opts != nil {
			if password := strings.TrimSpace(firstStringFromAny(opts["password"])); password != "" {
				pluginOpts["password"] = password
			}
			if hint := strings.TrimSpace(firstStringFromAny(opts["version_hint"])); hint != "" {
				pluginOpts["version_hint"] = hint
			}
			if script := firstStringFromAny(opts["restls_script"]); script != "" {
				pluginOpts["restls_script"] = script
			}
		}
		if script := firstStringFromAnyMap(wrapper, "restls_script"); script != "" {
			pluginOpts["restls_script"] = script
		}
		(*out)["plugin"] = "restls"
	case model.MihomoTlsModeJLS:
		if opts, ok := client["jls_opts"].(map[string]interface{}); ok && opts != nil {
			for _, key := range []string{"username", "password"} {
				if value := strings.TrimSpace(firstStringFromAny(opts[key])); value != "" {
					pluginOpts[key] = value
				}
			}
		}
		if wrapper != nil {
			if alpn, ok := wrapper["alpn"]; ok {
				pluginOpts["alpn"] = alpn
			}
		}
		(*out)["plugin"] = "jls"
	}
	if len(pluginOpts) > 0 {
		(*out)["plugin_opts"] = pluginOpts
	}
	if utls, ok := client["utls"].(map[string]interface{}); ok && utls != nil {
		if fingerprint := strings.TrimSpace(firstStringFromAny(utls["fingerprint"])); fingerprint != "" {
			(*out)["client_fingerprint"] = fingerprint
		}
	}
	delete(*out, "tls")
}

func firstStringFromAnyMap(value map[string]interface{}, key string) string {
	if value == nil {
		return ""
	}
	return firstStringFromAny(value[key])
}

func mihomoWrapperHost(raw string) string {
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

func addMihomoWrapperTLS(client map[string]interface{}, server map[string]interface{}, mode string) {
	if client == nil {
		return
	}
	client["enabled"] = true
	serverName := strings.TrimSpace(firstStringFromAny(server["server_name"]))
	var wrapper map[string]interface{}
	switch mode {
	case model.MihomoTlsModeShadowTLS:
		wrapper, _ = server["shadow_tls"].(map[string]interface{})
	case model.MihomoTlsModeRestls:
		wrapper, _ = server["res_tls"].(map[string]interface{})
	case model.MihomoTlsModeJLS:
		wrapper, _ = server["jls_config"].(map[string]interface{})
	}
	if serverName == "" && wrapper != nil {
		serverName = strings.TrimSpace(firstStringFromAny(wrapper["sni"]))
		if serverName == "" {
			serverName = mihomoWrapperHost(firstStringFromAny(wrapper["dest"]))
		}
		if serverName == "" {
			if handshake, ok := wrapper["handshake"].(map[string]interface{}); ok && handshake != nil {
				serverName = mihomoWrapperHost(firstStringFromAny(handshake["dest"]))
			}
		}
	}
	if serverName != "" && strings.TrimSpace(firstStringFromAny(client["server_name"])) == "" {
		client["server_name"] = serverName
	}
	if mode == model.MihomoTlsModeJLS && wrapper != nil {
		if _, exists := client["alpn"]; !exists {
			if alpn := wrapper["alpn"]; alpn != nil {
				client["alpn"] = alpn
			}
		}
	}

	// The internal snake_case fields stay inside the reusable TLS template.
	// Clash/Mihomo projection converts them to the official hyphenated keys.
	switch mode {
	case model.MihomoTlsModeShadowTLS:
		if opts, ok := client["shadow_tls_opts"].(map[string]interface{}); ok && opts != nil {
			client["shadow_tls_opts"] = opts
		}
	case model.MihomoTlsModeRestls:
		if opts, ok := client["restls_opts"].(map[string]interface{}); ok && opts != nil {
			if strings.TrimSpace(firstStringFromAny(opts["restls_script"])) == "" && wrapper != nil {
				if script := strings.TrimSpace(firstStringFromAny(wrapper["restls_script"])); script != "" {
					opts["restls_script"] = script
				}
			}
			client["restls_opts"] = opts
		}
	case model.MihomoTlsModeJLS:
		if opts, ok := client["jls_opts"].(map[string]interface{}); ok && opts != nil {
			client["jls_opts"] = opts
		}
	}
}

func clearMihomoShadowsocksPlugin(out map[string]interface{}) {
	if out == nil {
		return
	}
	plugin := strings.ToLower(strings.TrimSpace(firstStringFromAny(out["plugin"])))
	if plugin != "shadow-tls" && plugin != "shadowtls" && plugin != "restls" && plugin != "res-tls" && plugin != "jls" {
		return
	}
	for _, key := range []string{"plugin", "plugin_opts", "plugin-opts", "client_fingerprint", "client-fingerprint"} {
		delete(out, key)
	}
}

// Protocol-specific functions
func shadowsocksOut(out *map[string]interface{}, inbound map[string]interface{}) {
	if method, ok := inbound["method"].(string); ok {
		(*out)["method"] = method
	}
}

func shadowTlsOut(out *map[string]interface{}, inbound map[string]interface{}) {
	// 支持版本 1、2、3
	version := 0
	if v, ok := inbound["version"].(float64); ok {
		version = int(v)
	}

	// 如果版本无效（不是 1、2、3），清空输出并返回
	if version < 1 || version > 3 {
		for key := range *out {
			delete(*out, key)
		}
		return
	}

	(*out)["version"] = version

	// 保留用户已有的 TLS 配置，如果没有则创建默认值
	existingTls, hasExistingTls := (*out)["tls"].(map[string]interface{})
	if !hasExistingTls {
		existingTls = map[string]interface{}{}
	}

	// 确保 enabled 为 true
	existingTls["enabled"] = true

	// 从 handshake.server 获取 server_name（如果用户没有手动设置）
	if _, hasServerName := existingTls["server_name"]; !hasServerName {
		if handshake, ok := inbound["handshake"].(map[string]interface{}); ok {
			if server, ok := handshake["server"].(string); ok && server != "" {
				existingTls["server_name"] = server
			}
		}
	}

	// 不再强制注入默认 utls。
	// 若前端关闭了 utls（删除 utls 字段），这里应保持关闭状态，避免保存后自动恢复为开启。

	(*out)["tls"] = existingTls

	// 如果有 ss_config，在 out_json 中维护 ss_config 信息
	// 注意：不要每次都覆盖 out_json.ss_config，否则会把前端“客户端侧”编辑的
	// multiplex / udp_over_tcp / network 等值重置掉。
	// 仅在 out_json.ss_config 缺失时初始化，或对缺失基础字段做兜底。
	if ssConfig, ok := inbound["ss_config"].(map[string]interface{}); ok && ssConfig != nil {
		existingSsConfig, hasExisting := (*out)["ss_config"].(map[string]interface{})
		if !hasExisting || existingSsConfig == nil {
			(*out)["ss_config"] = ssConfig
		} else {
			if _, ok := existingSsConfig["method"]; !ok {
				if method, ok := ssConfig["method"]; ok {
					existingSsConfig["method"] = method
				}
			}
			if _, ok := existingSsConfig["password"]; !ok {
				if password, ok := ssConfig["password"]; ok {
					existingSsConfig["password"] = password
				}
			}
			if _, ok := existingSsConfig["network"]; !ok {
				if network, ok := ssConfig["network"]; ok {
					existingSsConfig["network"] = network
				}
			}
			(*out)["ss_config"] = existingSsConfig
		}
	}
}

func shadowQUICOut(out *map[string]interface{}, inbound map[string]interface{}) {
	if out == nil || *out == nil {
		return
	}

	// The listener owns these shared protocol fields. Client up/down bandwidth
	// values remain in out_json and are not overwritten by listener settings.
	for _, key := range []string{
		"sni",
		"alpn",
		"quic_versions",
		"quic-versions",
		"zero_rtt",
		"zero-rtt",
		"congestion_controller",
		"congestion-controller",
		"cwnd",
		"bbr_profile",
		"bbr-profile",
		"max_datagram_frame_size",
		"max-datagram-frame-size",
		"recv_window_conn",
		"recv-window-conn",
		"recv_window",
		"recv-window",
		"disable_mtu_discovery",
		"disable-mtu-discovery",
	} {
		delete(*out, key)
	}
	delete(*out, "tls")
	delete(*out, "jls_upstream")
	delete(*out, "jls-upstream")
	delete(*out, "routing_mark")
	delete(*out, "routing-mark")
	delete(*out, "rule")
	delete(*out, "proxy")
	delete(*out, "detour")

	if upstream := shadowQUICInboundUpstream(inbound); upstream != nil {
		if sni := strings.TrimSpace(shadowQUICString(upstream["sni"])); sni != "" {
			(*out)["sni"] = sni
		}
	}
	if alpn := NormalizeMihomoShadowQUICALPN(inbound["alpn"]); len(alpn) > 0 {
		(*out)["alpn"] = alpn
	}
	if versions := NormalizeMihomoShadowQUICVersions(firstShadowQUICValue(inbound, "quic_versions", "quic-versions")); len(versions) > 0 {
		(*out)["quic_versions"] = versions
	}
	if zeroRTT, ok := firstShadowQUICValue(inbound, "zero_rtt", "zero-rtt").(bool); ok {
		(*out)["zero_rtt"] = zeroRTT
	}
	if controller, ok := NormalizeMihomoShadowQUICCongestionController(firstShadowQUICValue(inbound, "congestion_controller", "congestion-controller")); ok {
		(*out)["congestion_controller"] = controller
	}
	copyShadowQUICNonNegativeInt(inbound, *out, "cwnd", "cwnd")
	if profile, ok := NormalizeMihomoBBRProfile(firstShadowQUICValue(inbound, "bbr_profile", "bbr-profile")); ok {
		(*out)["bbr_profile"] = profile
	}
	copyShadowQUICNonNegativeInt(inbound, *out, "max_datagram_frame_size", "max_datagram_frame_size", "max-datagram-frame-size")
	copyShadowQUICNonNegativeInt(inbound, *out, "recv_window_conn", "recv_window_conn", "recv-window-conn")
	copyShadowQUICNonNegativeInt(inbound, *out, "recv_window", "recv_window")
	if disableMTUDiscovery, ok := firstShadowQUICValue(inbound, "disable_mtu_discovery", "disable-mtu-discovery").(bool); ok {
		(*out)["disable_mtu_discovery"] = disableMTUDiscovery
	}

	SanitizeMihomoShadowQUICInboundTemplate(*out)
	if _, exists := (*out)["udp_over_stream"]; !exists {
		(*out)["udp_over_stream"] = false
	}
	if _, exists := (*out)["keep_alive_interval"]; !exists {
		(*out)["keep_alive_interval"] = 10000
	}
	if _, exists := (*out)["max_open_streams"]; !exists {
		(*out)["max_open_streams"] = 1024
	}
}

func shadowQUICInboundUpstream(inbound map[string]interface{}) map[string]interface{} {
	if inbound == nil {
		return nil
	}
	upstream, _ := inbound["jls-upstream"].(map[string]interface{})
	if canonical, ok := inbound["jls_upstream"].(map[string]interface{}); ok && canonical != nil {
		if upstream == nil {
			upstream = map[string]interface{}{}
		}
		for key, value := range canonical {
			upstream[key] = value
		}
	}
	return upstream
}

func hysteriaOut(out *map[string]interface{}, inbound map[string]interface{}) {
	clientUpMbps, hasClientUpMbps := positiveIntFromAny((*out)["up_mbps"])
	clientDownMbps, hasClientDownMbps := positiveIntFromAny((*out)["down_mbps"])

	delete(*out, "server_up_mbps")
	delete(*out, "server_down_mbps")
	delete(*out, "port_hop_range")
	delete(*out, "port_hop_interval")
	delete(*out, "port_hop_interval_max")
	delete(*out, "down_mbps")
	delete(*out, "up_mbps")
	delete(*out, "obfs")
	delete(*out, "recv_window_conn")
	delete(*out, "recv_window")
	delete(*out, "disable_mtu_discovery")
	delete(*out, "stream_receive_window")
	delete(*out, "connection_receive_window")
	delete(*out, "max_concurrent_streams")
	delete(*out, "disable_path_mtu_discovery")

	model.NormalizeHysteriaInboundOptionsMap(inbound)

	if hasClientUpMbps {
		(*out)["up_mbps"] = clientUpMbps
	}
	if hasClientDownMbps {
		(*out)["down_mbps"] = clientDownMbps
	}
	if obfs, ok := inbound["obfs"]; ok {
		(*out)["obfs"] = obfs
	}
	syncHysteriaOutboundField(*out, inbound, "stream_receive_window")
	syncHysteriaOutboundField(*out, inbound, "connection_receive_window")
	syncHysteriaOutboundField(*out, inbound, "max_concurrent_streams")
	syncHysteriaOutboundField(*out, inbound, "disable_path_mtu_discovery")

	// 端口跳跃: 从服务端自定义字段 port_hop_range 解析为客户端 server_ports
	if portHopRange, ok := inbound["port_hop_range"].(string); ok && portHopRange != "" {
		serverPorts := ParsePortHopRange(portHopRange)
		if len(serverPorts) > 0 {
			(*out)["server_ports"] = serverPorts
		}
	} else {
		delete(*out, "server_ports")
	}

	// 端口跳跃间隔
	if hopInterval, ok := inbound["port_hop_interval"].(string); ok && hopInterval != "" {
		(*out)["hop_interval"] = hopInterval
	} else {
		delete(*out, "hop_interval")
	}
}

func hysteria2Out(out *map[string]interface{}, inbound map[string]interface{}) {
	clientUpMbps, hasClientUpMbps := positiveIntFromAny((*out)["up_mbps"])
	clientDownMbps, hasClientDownMbps := positiveIntFromAny((*out)["down_mbps"])

	delete(*out, "server_up_mbps")
	delete(*out, "server_down_mbps")
	delete(*out, "port_hop_range")
	delete(*out, "port_hop_interval")
	delete(*out, "port_hop_interval_max")
	delete(*out, "down_mbps")
	delete(*out, "up_mbps")
	delete(*out, "obfs")
	SanitizeOptionalNetworkField(*out)

	if hasClientUpMbps {
		(*out)["up_mbps"] = clientUpMbps
	}
	if hasClientDownMbps {
		(*out)["down_mbps"] = clientDownMbps
	}
	if obfs, ok := inbound["obfs"]; ok {
		(*out)["obfs"] = obfs
	}
	if bbrProfile, ok := normalizeHysteria2BBRProfile(inbound["bbr_profile"]); ok {
		(*out)["bbr_profile"] = bbrProfile
	} else {
		delete(*out, "bbr_profile")
	}

	// 端口跳跃: 从服务端自定义字段 port_hop_range 解析为客户端 server_ports
	if portHopRange, ok := inbound["port_hop_range"].(string); ok && portHopRange != "" {
		serverPorts := ParsePortHopRange(portHopRange)
		if len(serverPorts) > 0 {
			(*out)["server_ports"] = serverPorts
		}
	} else {
		delete(*out, "server_ports")
	}

	// 端口跳跃间隔
	if hopInterval, ok := inbound["port_hop_interval"].(string); ok && hopInterval != "" {
		(*out)["hop_interval"] = hopInterval
	} else {
		delete(*out, "hop_interval")
	}
	if hopIntervalMax, ok := inbound["port_hop_interval_max"].(string); ok && hopIntervalMax != "" {
		(*out)["hop_interval_max"] = hopIntervalMax
	} else {
		delete(*out, "hop_interval_max")
	}
	SanitizeOptionalNetworkField(*out)
}

func normalizeHysteria2BBRProfile(raw interface{}) (string, bool) {
	value, ok := raw.(string)
	if !ok {
		return "", false
	}

	switch strings.ToLower(strings.TrimSpace(value)) {
	case "conservative":
		return "conservative", true
	case "standard":
		return "standard", true
	case "aggressive":
		return "aggressive", true
	default:
		return "", false
	}
}

func NormalizeMihomoBBRProfile(raw interface{}) (string, bool) {
	return normalizeHysteria2BBRProfile(raw)
}

func SupportsMihomoBBRProfileProtocol(protocol string) bool {
	switch strings.ToLower(strings.TrimSpace(protocol)) {
	case "hysteria2", "tuic", "trusttunnel", "shadowquic", "masque":
		return true
	default:
		return false
	}
}

func mieruOut(out *map[string]interface{}, inbound map[string]interface{}) {
	delete(*out, "port_bindings")
	delete(*out, "port_range")
	delete(*out, "user_hint_is_mandatory")
	delete(*out, "user-hint-is-mandatory")

	transport := NormalizeMieruTransport(firstStringFromAny(inbound["transport"]))
	(*out)["transport"] = transport

	if transport == "TCP" {
		if udp, ok := (*out)["udp"].(bool); ok {
			(*out)["udp"] = udp
		} else {
			(*out)["udp"] = true
		}
	} else {
		delete(*out, "udp")
	}

	(*out)["multiplexing"] = NormalizeMieruMultiplexing(firstStringFromAny((*out)["multiplexing"]))
	(*out)["handshake_mode"] = NormalizeMieruHandshakeMode(firstStringFromAny((*out)["handshake_mode"]))

	if rangeValue, ok := NormalizeMieruPortRange(firstStringFromAny(inbound["port_range"])); ok {
		if port, ok := MieruPrimaryPortFromBinding(rangeValue); ok {
			(*out)["server_port"] = port
		}
		(*out)["port_range"] = rangeValue
		return
	}

	bindings := NormalizeMieruPortBindings(firstStringFromAny(inbound["port_bindings"]))
	switch len(bindings) {
	case 0:
		if port, ok := toIntValue(inbound["listen_port"]); ok && port > 0 {
			(*out)["server_port"] = port
		}
	case 1:
		if port, ok := MieruPrimaryPortFromBinding(bindings[0]); ok {
			(*out)["server_port"] = port
		}
		if strings.Contains(bindings[0], "-") {
			(*out)["port_range"] = bindings[0]
		}
	default:
		if port, ok := MieruPrimaryPortFromBinding(bindings[0]); ok {
			(*out)["server_port"] = port
		}
		(*out)["port_bindings"] = bindings
	}
}

func sudokuOut(out *map[string]interface{}, inbound map[string]interface{}) {
	delete(*out, "key")
	delete(*out, "handshake_timeout")
	delete(*out, "fallback")
	delete(*out, "disable_http_mask")

	(*out)["aead_method"] = NormalizeSudokuAEADMethod(firstStringFromAny(inbound["aead_method"]))

	if value, ok := sudokuToInt(inbound["padding_min"]); ok && value > 0 {
		(*out)["padding_min"] = value
	} else {
		(*out)["padding_min"] = 1
	}
	if value, ok := sudokuToInt(inbound["padding_max"]); ok && value > 0 {
		(*out)["padding_max"] = value
	} else {
		(*out)["padding_max"] = 15
	}

	customTable := NormalizeSudokuCustomTable(inbound["custom_table"])
	customTables := NormalizeSudokuCustomTables(inbound["custom_tables"])
	(*out)["table_type"] = NormalizeSudokuTableTypeForCustom(
		firstStringFromAny(inbound["table_type"]),
		customTable != "" || len(customTables) > 0,
	)

	if customTable != "" {
		(*out)["custom_table"] = customTable
	} else {
		delete(*out, "custom_table")
	}
	if len(customTables) > 0 {
		(*out)["custom_tables"] = customTables
	} else {
		delete(*out, "custom_tables")
	}

	if value, ok := sudokuToBool(inbound["enable_pure_downlink"]); ok {
		(*out)["enable_pure_downlink"] = value
	} else {
		(*out)["enable_pure_downlink"] = false
	}

	httpmask := map[string]interface{}{}
	if existing, ok := (*out)["httpmask"].(map[string]interface{}); ok && existing != nil {
		if value, ok := sudokuToBool(existing["tls"]); ok {
			httpmask["tls"] = value
		}
		if value := NormalizeSudokuStringValue(existing["host"]); value != "" {
			httpmask["host"] = value
		}
		if value := NormalizeSudokuHTTPMaskMultiplex(firstStringFromAny(existing["multiplex"])); value != "" {
			httpmask["multiplex"] = value
		}
	}

	if inboundHTTPMask, ok := inbound["httpmask"].(map[string]interface{}); ok && inboundHTTPMask != nil {
		if value, ok := sudokuToBool(inboundHTTPMask["disable"]); ok {
			httpmask["disable"] = value
		} else {
			httpmask["disable"] = false
		}
		httpmask["mode"] = NormalizeSudokuHTTPMaskMode(firstStringFromAny(inboundHTTPMask["mode"]))
		if pathRoot := NormalizeSudokuStringValue(inboundHTTPMask["path_root"]); pathRoot != "" {
			httpmask["path_root"] = pathRoot
		} else {
			delete(httpmask, "path_root")
		}
	} else {
		httpmask["disable"] = false
		httpmask["mode"] = "legacy"
		delete(httpmask, "path_root")
	}

	if value, ok := sudokuToBool(httpmask["tls"]); ok {
		httpmask["tls"] = value
	} else {
		httpmask["tls"] = true
	}
	if value := NormalizeSudokuStringValue(httpmask["host"]); value != "" {
		httpmask["host"] = value
	} else {
		delete(httpmask, "host")
	}
	if value := NormalizeSudokuHTTPMaskMultiplex(firstStringFromAny(httpmask["multiplex"])); value != "" {
		httpmask["multiplex"] = value
	}
	if len(httpmask) > 0 {
		(*out)["httpmask"] = httpmask
	} else {
		delete(*out, "httpmask")
	}
}

// ParsePortHopRange 解析用户输入的端口范围字符串为 sing-box server_ports 数组
// 支持格式:
//   - 单端口: "55100"
//   - 端口范围: "2080-3000" 或 "2080:3000"
//   - 混合: "500-800, 2080:3000, 55100"
//   - 中英文逗号均可
func ParsePortHopRange(input string) []string {
	if input == "" {
		return nil
	}

	// Replace full-width comma with normal comma.
	input = strings.ReplaceAll(input, "\uFF0C", ",")

	parts := strings.Split(input, ",")
	var result []string
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		// Sing-box uses ":" for range syntax; normalize "-" to ":".
		part = strings.ReplaceAll(part, "-", ":")
		result = append(result, part)
	}
	return result
}

func firstStringFromAny(raw interface{}) string {
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

func stringSliceFromAny(raw interface{}) []string {
	switch value := raw.(type) {
	case []string:
		result := make([]string, 0, len(value))
		for _, item := range value {
			item = strings.TrimSpace(item)
			if item != "" {
				result = append(result, item)
			}
		}
		return result
	case []interface{}:
		result := make([]string, 0, len(value))
		for _, item := range value {
			itemStr, ok := item.(string)
			if !ok {
				continue
			}
			itemStr = strings.TrimSpace(itemStr)
			if itemStr != "" {
				result = append(result, itemStr)
			}
		}
		return result
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

func sshOut(out *map[string]interface{}, inbound map[string]interface{}) {
	delete(*out, "username")
	delete(*out, "user")
	delete(*out, "password")
	delete(*out, "private_key")
	delete(*out, "private_key_path")
	delete(*out, "private_key_passphrase")
	delete(*out, "host_key")
	delete(*out, "host_key_algorithms")
	delete(*out, "client_version")
	delete(*out, "cipher")
	delete(*out, "mac")
	delete(*out, "kex_algorithm")

	username := firstStringFromAny(inbound["username"])
	if username == "" {
		username = firstStringFromAny(inbound["user"])
	}
	if username != "" {
		(*out)["user"] = username
		(*out)["username"] = username
	}

	if password := firstStringFromAny(inbound["password"]); password != "" {
		(*out)["password"] = password
	}
	if privateKey := firstStringFromAny(inbound["private_key"]); privateKey != "" {
		(*out)["private_key"] = privateKey
	}
	if privateKeyPath := firstStringFromAny(inbound["private_key_path"]); privateKeyPath != "" {
		(*out)["private_key_path"] = privateKeyPath
	}
	if passphrase := firstStringFromAny(inbound["private_key_passphrase"]); passphrase != "" {
		(*out)["private_key_passphrase"] = passphrase
	}
	if hostKey := stringSliceFromAny(inbound["host_key"]); len(hostKey) > 0 {
		(*out)["host_key"] = hostKey
	}
	if algorithms := stringSliceFromAny(inbound["host_key_algorithms"]); len(algorithms) > 0 {
		(*out)["host_key_algorithms"] = algorithms
	}
	if clientVersion := firstStringFromAny(inbound["client_version"]); clientVersion != "" {
		(*out)["client_version"] = clientVersion
	}
	if cipher := stringSliceFromAny(inbound["cipher"]); len(cipher) > 0 {
		(*out)["cipher"] = cipher
	}
	if mac := stringSliceFromAny(inbound["mac"]); len(mac) > 0 {
		(*out)["mac"] = mac
	}
	if kex := stringSliceFromAny(inbound["kex_algorithm"]); len(kex) > 0 {
		(*out)["kex_algorithm"] = kex
	}
}

func naiveOut(out *map[string]interface{}, inbound map[string]interface{}) {
	delete(*out, "network")

	quicCongestionControl := strings.ToLower(strings.TrimSpace(firstStringFromAny((*out)["quic_congestion_control"])))
	switch quicCongestionControl {
	case "bbr", "bbr2", "cubic", "reno":
		(*out)["quic_congestion_control"] = quicCongestionControl
	default:
		delete(*out, "quic_congestion_control")
	}
}

func snellOut(out *map[string]interface{}, inbound map[string]interface{}) {
	if version, ok := positiveIntFromAny((*out)["version"]); ok && version > 0 {
		(*out)["version"] = version
	} else if version, ok := positiveIntFromAny(inbound["version"]); ok && version > 0 {
		(*out)["version"] = version
	} else {
		(*out)["version"] = 5
	}

	if reuse, ok := (*out)["reuse"].(bool); ok {
		(*out)["reuse"] = reuse
	} else {
		(*out)["reuse"] = false
	}

	rawObfs := inbound["obfs_opts"]
	if rawObfs == nil {
		rawObfs = inbound["obfs-opts"]
	}
	obfsMap, _ := rawObfs.(map[string]interface{})
	if obfsMap == nil {
		delete(*out, "obfs_opts")
		return
	}

	mode := strings.TrimSpace(firstStringFromAny(obfsMap["mode"]))
	if mode == "" {
		delete(*out, "obfs_opts")
		return
	}

	host := strings.TrimSpace(firstStringFromAny(obfsMap["host"]))
	if host == "" {
		host = "www.bing.com"
	}

	(*out)["obfs_opts"] = map[string]interface{}{
		"mode": mode,
		"host": host,
	}
}

func trustTunnelOut(out *map[string]interface{}, inbound map[string]interface{}) {
	if udp, ok := ResolveTrustTunnelUDP(*out); ok {
		(*out)["udp"] = udp
	} else {
		(*out)["udp"] = HasStringValue(inbound["network"], "udp")
	}
	delete(*out, "network")
}

func tuicOut(out *map[string]interface{}, inbound map[string]interface{}) {
	delete(*out, "zero_rtt_handshake")
	delete(*out, "heartbeat")
	if congestionControl, ok := inbound["congestion_control"].(string); ok {
		(*out)["congestion_control"] = congestionControl
	} else {
		(*out)["congestion_control"] = "cubic"
	}
	if zeroRTT, ok := inbound["zero_rtt_handshake"].(bool); ok {
		(*out)["zero_rtt_handshake"] = zeroRTT
	}
	if heartbeat, ok := inbound["heartbeat"]; ok {
		(*out)["heartbeat"] = heartbeat
	}
}

func vlessOut(out *map[string]interface{}, inbound map[string]interface{}) {
	delete(*out, "transport")
	if transport, ok := inbound["transport"]; ok {
		(*out)["transport"] = transport
	}

	enabled, hasEnabled := VLESSInboundEncryptionEnabled(inbound)
	if !hasEnabled {
		return
	}
	if !enabled {
		delete(*out, "encryption")
		return
	}

	if encryption, ok := BuildVLESSMihomoEncryption(inbound); ok {
		(*out)["encryption"] = encryption
	} else {
		delete(*out, "encryption")
	}
}

func trojanOut(out *map[string]interface{}, inbound map[string]interface{}) {
	delete(*out, "transport")
	if transport, ok := inbound["transport"]; ok {
		(*out)["transport"] = transport
	}
}

func vmessOut(out *map[string]interface{}, inbound map[string]interface{}) {
	delete(*out, "alterId")
	(*out)["alter_id"] = 0
	delete(*out, "transport")
	if transport, ok := inbound["transport"]; ok {
		(*out)["transport"] = transport
	}
}
