package sub

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"net"
	"os"
	"strings"

	"github.com/alireza0/s-ui/database/model"
)

func refreshSubscriptionOutboundTLS(outbound map[string]interface{}, tlsConfig *model.Tls) {
	if outbound == nil {
		return
	}

	serverTLS := map[string]interface{}{}
	clientTLS := map[string]interface{}{}
	if tlsConfig != nil {
		serverTLS = decodeSubscriptionTLSRaw(tlsConfig.Server)
		clientTLS = decodeSubscriptionTLSRaw(tlsConfig.Client)
	}
	mode := model.MihomoTlsModeTLS
	if tlsConfig != nil {
		mode = model.NormalizeMihomoTlsMode(tlsConfig.Mode, tlsConfig.Server, tlsConfig.Client)
	}
	if strings.EqualFold(strings.TrimSpace(firstStringValue(outbound["type"])), "shadowsocks") {
		refreshMihomoShadowsocksPlugin(outbound, serverTLS, clientTLS, mode)
		return
	}
	if tlsConfig == nil {
		return
	}

	outboundTLS, ok := outbound["tls"].(map[string]interface{})
	if !ok || outboundTLS == nil {
		outboundTLS = map[string]interface{}{}
		outbound["tls"] = outboundTLS
	}
	existingServerCertificateSHA256 := outboundTLS["certificate_public_key_sha256"]
	if enabled, ok := toBool(serverTLS["enabled"]); ok {
		outboundTLS["enabled"] = enabled
	} else if enabled, ok := toBool(clientTLS["enabled"]); ok {
		outboundTLS["enabled"] = enabled
	} else if value, exists := outboundTLS["enabled"]; !exists || value == nil {
		outboundTLS["enabled"] = true
	}

	// Wrapper options are part of the client subscription template. Clear the
	// mode-specific projection first so a Reality/wrapper switch cannot leave
	// stale mutually-exclusive fields behind.
	for _, key := range []string{
		"shadow_tls_opts", "restls_opts", "jls_opts",
		"server_name", "alpn", "insecure", "disable_sni", "utls", "reality", "ech",
		"certificate", "certificate_path", "certificate_public_key_sha256", "fingerprint",
		"client_certificate", "client_certificate_path", "client_key", "client_key_path",
	} {
		delete(outboundTLS, key)
	}
	for _, key := range []string{"server_name", "alpn", "insecure", "disable_sni", "utls", "reality", "ech"} {
		if value, exists := clientTLS[key]; exists {
			outboundTLS[key] = value
		}
	}
	if _, exists := clientTLS["alpn"]; !exists {
		if value, exists := serverTLS["alpn"]; exists {
			outboundTLS["alpn"] = value
		}
	}
	if strings.TrimSpace(firstStringValue(outboundTLS["server_name"])) == "" {
		if serverName := strings.TrimSpace(firstStringValue(serverTLS["server_name"])); serverName != "" {
			outboundTLS["server_name"] = serverName
		}
	}
	if mode == model.MihomoTlsModeShadowTLS || mode == model.MihomoTlsModeRestls || mode == model.MihomoTlsModeJLS {
		wrapper := mihomoTLSWrapper(serverTLS, mode)
		if strings.TrimSpace(firstStringValue(outboundTLS["server_name"])) == "" && wrapper != nil {
			serverName := strings.TrimSpace(firstStringValue(wrapper["sni"]))
			if serverName == "" {
				serverName = subscriptionTLSWrapperHost(firstStringValue(wrapper["dest"]))
			}
			if serverName == "" {
				if handshake, ok := wrapper["handshake"].(map[string]interface{}); ok && handshake != nil {
					serverName = subscriptionTLSWrapperHost(firstStringValue(handshake["dest"]))
				}
			}
			if serverName != "" {
				outboundTLS["server_name"] = serverName
			}
		}
		if mode == model.MihomoTlsModeJLS {
			if _, exists := outboundTLS["alpn"]; !exists && wrapper != nil && wrapper["alpn"] != nil {
				outboundTLS["alpn"] = wrapper["alpn"]
			}
		}
	}
	switch mode {
	case model.MihomoTlsModeShadowTLS:
		if opts, ok := clientTLS["shadow_tls_opts"].(map[string]interface{}); ok && opts != nil {
			outboundTLS["shadow_tls_opts"] = opts
		}
	case model.MihomoTlsModeRestls:
		if opts, ok := clientTLS["restls_opts"].(map[string]interface{}); ok && opts != nil {
			if strings.TrimSpace(firstStringValue(opts["restls_script"])) == "" {
				if wrapper := mihomoTLSWrapper(serverTLS, mode); wrapper != nil {
					if script := strings.TrimSpace(firstStringValue(wrapper["restls_script"])); script != "" {
						opts["restls_script"] = script
					}
				}
			}
			outboundTLS["restls_opts"] = opts
		}
	case model.MihomoTlsModeJLS:
		if opts, ok := clientTLS["jls_opts"].(map[string]interface{}); ok && opts != nil {
			outboundTLS["jls_opts"] = opts
		}
	}
	if mode == model.MihomoTlsModeShadowTLS || mode == model.MihomoTlsModeRestls || mode == model.MihomoTlsModeJLS {
		outboundTLS["enabled"] = true
		for _, key := range []string{"certificate", "certificate_path", "certificate_public_key_sha256", "fingerprint"} {
			delete(outboundTLS, key)
		}
		return
	}

	includeServerCertificate := true
	if include, ok := clientTLS["include_server_certificate"].(bool); ok {
		includeServerCertificate = include
	}
	includeServerFingerprint := shouldIncludeSubscriptionClashFingerprint(clientTLS)
	useServerCertificateSHA256 := hasNonEmptyTLSHash(clientTLS["certificate_public_key_sha256"])
	if !useServerCertificateSHA256 {
		useServerCertificateSHA256 = hasNonEmptyTLSHash(existingServerCertificateSHA256)
	}

	serverCertLines, serverCertPEM, hasServerCert := loadSubscriptionPEM(serverTLS["certificate"], serverTLS["certificate_path"], "CERTIFICATE")
	if includeServerCertificate {
		if hasServerCert {
			if useServerCertificateSHA256 {
				if sha256Value, ok := calculateSubscriptionTLSPublicKeySHA256(serverCertPEM); ok {
					outboundTLS["certificate_public_key_sha256"] = []string{sha256Value}
				} else {
					delete(outboundTLS, "certificate_public_key_sha256")
				}
				delete(outboundTLS, "certificate")
			} else {
				outboundTLS["certificate"] = serverCertLines
				delete(outboundTLS, "certificate_public_key_sha256")
			}
		} else {
			delete(outboundTLS, "certificate")
			delete(outboundTLS, "certificate_public_key_sha256")
		}
	} else {
		delete(outboundTLS, "certificate")
		delete(outboundTLS, "certificate_public_key_sha256")
	}
	if includeServerFingerprint && hasServerCert {
		if fingerprint, ok := calculateSubscriptionTLSFingerprint(serverCertPEM); ok {
			outboundTLS["fingerprint"] = fingerprint
		} else {
			delete(outboundTLS, "fingerprint")
		}
	} else {
		delete(outboundTLS, "fingerprint")
	}

	if clientCertLines, _, ok := loadSubscriptionPEM(clientTLS["client_certificate"], clientTLS["client_certificate_path"], "CERTIFICATE"); ok {
		outboundTLS["client_certificate"] = clientCertLines
	}
	if clientKeyLines, ok := loadSubscriptionTextLines(clientTLS["client_key"], clientTLS["client_key_path"]); ok {
		outboundTLS["client_key"] = clientKeyLines
	}
}

func refreshMihomoShadowsocksPlugin(outbound map[string]interface{}, serverTLS, clientTLS map[string]interface{}, mode string) {
	if outbound == nil {
		return
	}
	knownPlugin := map[string]struct{}{"shadow-tls": {}, "restls": {}, "jls": {}}
	currentPlugin := strings.TrimSpace(firstStringValue(outbound["plugin"]))
	if mode != model.MihomoTlsModeShadowTLS && mode != model.MihomoTlsModeRestls && mode != model.MihomoTlsModeJLS {
		if _, known := knownPlugin[currentPlugin]; known {
			delete(outbound, "plugin")
			delete(outbound, "plugin_opts")
			delete(outbound, "plugin-opts")
			delete(outbound, "client_fingerprint")
			delete(outbound, "client-fingerprint")
		}
		return
	}

	delete(outbound, "plugin_opts")
	delete(outbound, "plugin-opts")
	delete(outbound, "client_fingerprint")
	delete(outbound, "client-fingerprint")
	pluginOpts := map[string]interface{}{}
	wrapper := mihomoTLSWrapper(serverTLS, mode)
	host := strings.TrimSpace(firstStringValue(clientTLS["server_name"]))
	if host == "" && wrapper != nil {
		host = strings.TrimSpace(firstStringValue(wrapper["sni"]))
		if host == "" {
			host = subscriptionTLSWrapperHost(firstStringValue(wrapper["dest"]))
		}
		if host == "" {
			if handshake, ok := wrapper["handshake"].(map[string]interface{}); ok && handshake != nil {
				host = subscriptionTLSWrapperHost(firstStringValue(handshake["dest"]))
			}
		}
	}
	if host != "" {
		pluginOpts["host"] = host
	}

	switch mode {
	case model.MihomoTlsModeShadowTLS:
		outbound["plugin"] = "shadow-tls"
		if opts, ok := clientTLS["shadow_tls_opts"].(map[string]interface{}); ok && opts != nil {
			copyStringOrNumber(pluginOpts, opts, "version")
			copyStringOrNumber(pluginOpts, opts, "password")
		}
		if _, exists := pluginOpts["version"]; !exists && wrapper != nil {
			copyStringOrNumber(pluginOpts, wrapper, "version")
		}
	case model.MihomoTlsModeRestls:
		outbound["plugin"] = "restls"
		if opts, ok := clientTLS["restls_opts"].(map[string]interface{}); ok && opts != nil {
			copyStringOrNumber(pluginOpts, opts, "password")
			copyStringOrNumber(pluginOpts, opts, "version_hint")
			copyStringOrNumber(pluginOpts, opts, "restls_script")
		}
		if _, exists := pluginOpts["restls_script"]; !exists && wrapper != nil {
			copyStringOrNumber(pluginOpts, wrapper, "restls_script")
		}
	case model.MihomoTlsModeJLS:
		outbound["plugin"] = "jls"
		if opts, ok := clientTLS["jls_opts"].(map[string]interface{}); ok && opts != nil {
			copyStringOrNumber(pluginOpts, opts, "username")
			copyStringOrNumber(pluginOpts, opts, "password")
		}
		if wrapper != nil {
			if alpn := wrapper["alpn"]; alpn != nil {
				pluginOpts["alpn"] = alpn
			}
		}
	}
	if len(pluginOpts) > 0 {
		outbound["plugin_opts"] = pluginOpts
	}
	if utls, ok := clientTLS["utls"].(map[string]interface{}); ok && utls != nil {
		if fingerprint := strings.TrimSpace(firstStringValue(utls["fingerprint"])); fingerprint != "" {
			outbound["client_fingerprint"] = fingerprint
		}
	}
}

func isMihomoWrapperTLSMode(mode string) bool {
	return mode == model.MihomoTlsModeShadowTLS || mode == model.MihomoTlsModeRestls || mode == model.MihomoTlsModeJLS
}

func clearMihomoClashWrapperProjection(proxy map[string]interface{}) {
	if proxy == nil {
		return
	}
	for _, key := range []string{"shadow-tls-opts", "restls-opts", "jls-opts"} {
		delete(proxy, key)
	}
	plugin := strings.ToLower(strings.TrimSpace(firstStringValue(proxy["plugin"])))
	if plugin == "shadow-tls" || plugin == "shadowtls" || plugin == "restls" || plugin == "res-tls" || plugin == "jls" {
		delete(proxy, "plugin")
		delete(proxy, "plugin-opts")
		delete(proxy, "client-fingerprint")
	}
}

func refreshMihomoClashProxyTLS(proxy map[string]interface{}, tlsConfig *model.Tls) {
	if proxy == nil || tlsConfig == nil {
		return
	}
	serverTLS := decodeSubscriptionTLSRaw(tlsConfig.Server)
	clientTLS := decodeSubscriptionTLSRaw(tlsConfig.Client)
	mode := model.NormalizeMihomoTlsMode(tlsConfig.Mode, tlsConfig.Server, tlsConfig.Client)
	proxyType := strings.ToLower(strings.TrimSpace(firstStringValue(proxy["type"])))

	if proxyType != "shadowsocks" {
		clearMihomoClashWrapperProjection(proxy)
		if isMihomoWrapperTLSMode(mode) {
			for _, key := range []string{"reality-opts", "ech-opts", "fingerprint", "skip-cert-verify", "disable-sni", "alpn", "client-fingerprint"} {
				delete(proxy, key)
			}
			delete(proxy, "sni")
			delete(proxy, "servername")
		}
		proxy["tls"] = true
		serverName := strings.TrimSpace(firstStringValue(clientTLS["server_name"]))
		if serverName == "" {
			serverName = strings.TrimSpace(firstStringValue(serverTLS["server_name"]))
		}
		if serverName == "" {
			if wrapper := mihomoTLSWrapper(serverTLS, mode); wrapper != nil {
				serverName = strings.TrimSpace(firstStringValue(wrapper["sni"]))
				if serverName == "" {
					serverName = subscriptionTLSWrapperHost(firstStringValue(wrapper["dest"]))
				}
				if serverName == "" {
					if handshake, ok := wrapper["handshake"].(map[string]interface{}); ok && handshake != nil {
						serverName = subscriptionTLSWrapperHost(firstStringValue(handshake["dest"]))
					}
				}
			}
		}
		if serverName != "" {
			switch proxyType {
			case "vmess", "vless":
				proxy["servername"] = serverName
				delete(proxy, "sni")
			default:
				proxy["sni"] = serverName
				delete(proxy, "servername")
			}
		}
		if utls, ok := clientTLS["utls"].(map[string]interface{}); ok && utls != nil {
			if fingerprint := strings.TrimSpace(firstStringValue(utls["fingerprint"])); fingerprint != "" {
				proxy["client-fingerprint"] = fingerprint
			}
		}
		if isMihomoWrapperTLSMode(mode) {
			switch mode {
			case model.MihomoTlsModeShadowTLS:
				if opts, ok := clientTLS["shadow_tls_opts"].(map[string]interface{}); ok && opts != nil {
					proxy["shadow-tls-opts"] = normalizeMihomoClashTLSOptionMap(opts, nil)
				}
			case model.MihomoTlsModeRestls:
				if opts, ok := clientTLS["restls_opts"].(map[string]interface{}); ok && opts != nil {
					restlsOpts := normalizeMihomoClashTLSOptionMap(opts, map[string]string{
						"version_hint": "version-hint", "restls_script": "restls-script",
					})
					if _, exists := restlsOpts["restls-script"]; !exists {
						if wrapper := mihomoTLSWrapper(serverTLS, mode); wrapper != nil {
							if script := strings.TrimSpace(firstStringValue(wrapper["restls_script"])); script != "" {
								restlsOpts["restls-script"] = script
							}
						}
					}
					proxy["restls-opts"] = restlsOpts
				}
			case model.MihomoTlsModeJLS:
				if opts, ok := clientTLS["jls_opts"].(map[string]interface{}); ok && opts != nil {
					proxy["jls-opts"] = normalizeMihomoClashTLSOptionMap(opts, nil)
				}
				if wrapper := mihomoTLSWrapper(serverTLS, mode); wrapper != nil {
					if alpn := wrapper["alpn"]; alpn != nil {
						proxy["alpn"] = alpn
					}
				}
			}
		}
		return
	}

	clearMihomoClashWrapperProjection(proxy)
	if !isMihomoWrapperTLSMode(mode) {
		return
	}
	for _, key := range []string{"tls", "reality-opts", "ech-opts", "fingerprint", "skip-cert-verify", "disable-sni", "sni", "servername", "alpn", "client-fingerprint"} {
		delete(proxy, key)
	}

	wrapper := mihomoTLSWrapper(serverTLS, mode)
	host := strings.TrimSpace(firstStringValue(clientTLS["server_name"]))
	if host == "" && wrapper != nil {
		host = strings.TrimSpace(firstStringValue(wrapper["sni"]))
		if host == "" {
			host = subscriptionTLSWrapperHost(firstStringValue(wrapper["dest"]))
		}
		if host == "" {
			if handshake, ok := wrapper["handshake"].(map[string]interface{}); ok && handshake != nil {
				host = subscriptionTLSWrapperHost(firstStringValue(handshake["dest"]))
			}
		}
	}
	pluginOpts := map[string]interface{}{}
	if host != "" {
		pluginOpts["host"] = host
	}
	switch mode {
	case model.MihomoTlsModeShadowTLS:
		proxy["plugin"] = "shadow-tls"
		if opts, ok := clientTLS["shadow_tls_opts"].(map[string]interface{}); ok && opts != nil {
			copyStringOrNumber(pluginOpts, opts, "version")
			copyStringOrNumber(pluginOpts, opts, "password")
		}
		if _, exists := pluginOpts["version"]; !exists && wrapper != nil {
			copyStringOrNumber(pluginOpts, wrapper, "version")
		}
	case model.MihomoTlsModeRestls:
		proxy["plugin"] = "restls"
		if opts, ok := clientTLS["restls_opts"].(map[string]interface{}); ok && opts != nil {
			copyStringOrNumber(pluginOpts, opts, "password")
			copyStringOrNumber(pluginOpts, opts, "version_hint")
			copyStringOrNumber(pluginOpts, opts, "restls_script")
		}
		if _, exists := pluginOpts["restls_script"]; !exists && wrapper != nil {
			copyStringOrNumber(pluginOpts, wrapper, "restls_script")
		}
	case model.MihomoTlsModeJLS:
		proxy["plugin"] = "jls"
		if opts, ok := clientTLS["jls_opts"].(map[string]interface{}); ok && opts != nil {
			copyStringOrNumber(pluginOpts, opts, "username")
			copyStringOrNumber(pluginOpts, opts, "password")
		}
		if wrapper != nil && wrapper["alpn"] != nil {
			pluginOpts["alpn"] = wrapper["alpn"]
		}
	}
	if len(pluginOpts) > 0 {
		proxy["plugin-opts"] = normalizeMihomoSSPluginOpts(pluginOpts)
	}
	if utls, ok := clientTLS["utls"].(map[string]interface{}); ok && utls != nil {
		if fingerprint := strings.TrimSpace(firstStringValue(utls["fingerprint"])); fingerprint != "" {
			proxy["client-fingerprint"] = fingerprint
		}
	}
}

func mihomoTLSWrapper(serverTLS map[string]interface{}, mode string) map[string]interface{} {
	if serverTLS == nil {
		return nil
	}
	key := ""
	switch mode {
	case model.MihomoTlsModeShadowTLS:
		key = "shadow_tls"
	case model.MihomoTlsModeRestls:
		key = "res_tls"
	case model.MihomoTlsModeJLS:
		key = "jls_config"
	}
	value, _ := serverTLS[key].(map[string]interface{})
	return value
}

func copyStringOrNumber(target map[string]interface{}, source map[string]interface{}, key string) {
	if target == nil || source == nil {
		return
	}
	value, exists := source[key]
	if !exists {
		return
	}
	switch value.(type) {
	case string, float64, float32, int, int8, int16, int32, int64, uint, uint8, uint16, uint32, uint64:
		target[key] = value
	}
}

func subscriptionTLSWrapperHost(raw string) string {
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

func firstStringValue(raw interface{}) string {
	switch value := raw.(type) {
	case string:
		return strings.TrimSpace(value)
	case []string:
		for _, item := range value {
			if item = strings.TrimSpace(item); item != "" {
				return item
			}
		}
	case []interface{}:
		for _, item := range value {
			if text, ok := item.(string); ok && strings.TrimSpace(text) != "" {
				return strings.TrimSpace(text)
			}
		}
	}
	return ""
}

func shouldIncludeSubscriptionClashFingerprint(clientTLS map[string]interface{}) bool {
	if include, ok := clientTLS["include_server_fingerprint"].(bool); ok {
		return include
	}
	return true
}

func hasNonEmptyTLSHash(raw interface{}) bool {
	switch value := raw.(type) {
	case []string:
		for _, item := range value {
			if strings.TrimSpace(item) != "" {
				return true
			}
		}
	case []interface{}:
		for _, item := range value {
			text, ok := item.(string)
			if !ok {
				continue
			}
			if strings.TrimSpace(text) != "" {
				return true
			}
		}
	case string:
		return strings.TrimSpace(value) != ""
	}
	return false
}

func decodeSubscriptionTLSRaw(raw json.RawMessage) map[string]interface{} {
	if len(raw) == 0 {
		return nil
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil
	}
	return decoded
}

func loadSubscriptionPEM(contentRaw interface{}, pathRaw interface{}, requiredBlock string) ([]string, []byte, bool) {
	lines, rawBytes, ok := loadSubscriptionRawBytes(contentRaw, pathRaw)
	if !ok {
		return nil, nil, false
	}
	if !strings.Contains(string(rawBytes), "BEGIN "+requiredBlock) {
		return nil, nil, false
	}
	return lines, rawBytes, true
}

func loadSubscriptionTextLines(contentRaw interface{}, pathRaw interface{}) ([]string, bool) {
	lines, _, ok := loadSubscriptionRawBytes(contentRaw, pathRaw)
	return lines, ok
}

func loadSubscriptionRawBytes(contentRaw interface{}, pathRaw interface{}) ([]string, []byte, bool) {
	if lines, rawBytes, ok := loadSubscriptionRawBytesFromPath(pathRaw); ok {
		return lines, rawBytes, true
	}

	if lines, ok := normalizeSubscriptionLines(contentRaw); ok {
		pemText := strings.Join(lines, "\n")
		return lines, []byte(pemText + "\n"), true
	}

	return nil, nil, false
}

func loadSubscriptionRawBytesFromPath(pathRaw interface{}) ([]string, []byte, bool) {
	path, ok := pathRaw.(string)
	if !ok {
		return nil, nil, false
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, nil, false
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, false
	}

	normalized := strings.TrimSpace(strings.ReplaceAll(string(data), "\r\n", "\n"))
	if normalized == "" {
		return nil, nil, false
	}

	lines := strings.Split(normalized, "\n")
	return lines, []byte(normalized + "\n"), true
}

func normalizeSubscriptionLines(raw interface{}) ([]string, bool) {
	switch typed := raw.(type) {
	case []string:
		lines := filterSubscriptionLines(typed)
		return lines, len(lines) > 0
	case []interface{}:
		lines := make([]string, 0, len(typed))
		for _, item := range typed {
			value, ok := item.(string)
			if !ok {
				return nil, false
			}
			lines = append(lines, strings.TrimSpace(strings.TrimSuffix(value, "\r")))
		}
		lines = filterSubscriptionLines(lines)
		return lines, len(lines) > 0
	case string:
		normalized := strings.TrimSpace(strings.ReplaceAll(typed, "\r\n", "\n"))
		if normalized == "" {
			return nil, false
		}
		lines := filterSubscriptionLines(strings.Split(normalized, "\n"))
		return lines, len(lines) > 0
	default:
		return nil, false
	}
}

func filterSubscriptionLines(lines []string) []string {
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if trimmed == "" {
			continue
		}
		filtered = append(filtered, trimmed)
	}
	return filtered
}

func parseSubscriptionCertificates(certPEM []byte) ([]*x509.Certificate, bool) {
	rest := certPEM
	certs := make([]*x509.Certificate, 0)

	for len(rest) > 0 {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, false
		}
		certs = append(certs, cert)
	}

	return certs, len(certs) > 0
}

func calculateSubscriptionTLSPublicKeySHA256(certPEM []byte) (string, bool) {
	certs, ok := parseSubscriptionCertificates(certPEM)
	if !ok {
		return "", false
	}

	publicKeyDER, err := x509.MarshalPKIXPublicKey(certs[0].PublicKey)
	if err != nil {
		return "", false
	}

	sum := sha256.Sum256(publicKeyDER)
	return base64.StdEncoding.EncodeToString(sum[:]), true
}

func calculateSubscriptionTLSFingerprint(certPEM []byte) (string, bool) {
	certs, ok := parseSubscriptionCertificates(certPEM)
	if !ok {
		return "", false
	}

	sum := sha256.Sum256(certs[0].Raw)
	hexStr := strings.ToUpper(hex.EncodeToString(sum[:]))
	parts := make([]string, 0, len(hexStr)/2)
	for i := 0; i < len(hexStr); i += 2 {
		parts = append(parts, hexStr[i:i+2])
	}
	return strings.Join(parts, ":"), true
}
