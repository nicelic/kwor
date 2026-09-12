package util

import (
	"strings"
)

type RealmOptsParsed struct {
	Enable         bool
	ServerURL      string
	Token          string
	RealmID        string
	STUNServers    []string
	SNI            string
	SkipCertVerify bool
	NameCertVerify string
	Fingerprint    string
	Certificate    string
	PrivateKey     string
	ALPN           []string
	Proxy          string
}

func parseRawRealmOpts(raw interface{}) (*RealmOptsParsed, bool) {
	if raw == nil {
		return nil, false
	}
	m, ok := raw.(map[string]interface{})
	if !ok || m == nil || len(m) == 0 {
		return nil, false
	}

	getVal := func(keys ...string) interface{} {
		for _, k := range keys {
			if v, exists := m[k]; exists && v != nil {
				return v
			}
		}
		return nil
	}

	getString := func(keys ...string) string {
		v := getVal(keys...)
		if v == nil {
			return ""
		}
		if s, ok := v.(string); ok {
			return strings.TrimSpace(s)
		}
		return strings.TrimSpace(readStringValue(v))
	}

	getBool := func(keys ...string) bool {
		v := getVal(keys...)
		if v == nil {
			return false
		}
		if b, ok := v.(bool); ok {
			return b
		}
		if s, ok := v.(string); ok {
			s = strings.TrimSpace(strings.ToLower(s))
			return s == "true" || s == "1"
		}
		return false
	}

	getStringSlice := func(keys ...string) []string {
		v := getVal(keys...)
		if v == nil {
			return nil
		}
		switch val := v.(type) {
		case []string:
			res := make([]string, 0, len(val))
			for _, item := range val {
				if s := strings.TrimSpace(item); s != "" {
					res = append(res, s)
				}
			}
			return res
		case []interface{}:
			res := make([]string, 0, len(val))
			for _, item := range val {
				if s := strings.TrimSpace(readStringValue(item)); s != "" {
					res = append(res, s)
				}
			}
			return res
		case string:
			parts := strings.Split(val, ",")
			res := make([]string, 0, len(parts))
			for _, p := range parts {
				if s := strings.TrimSpace(p); s != "" {
					res = append(res, s)
				}
			}
			return res
		}
		return nil
	}

	enable := getBool("enable")
	serverURL := getString("server_url", "server-url", "serverUrl")
	token := getString("token")
	realmID := getString("realm_id", "realm-id", "realmId")
	stunServers := getStringSlice("stun_servers", "stun-servers", "stunServers")
	sni := getString("sni")
	skipCertVerify := getBool("skip_cert_verify", "skip-cert-verify", "skipCertVerify")
	nameCertVerify := getString("name_cert_verify", "name-cert-verify", "nameCertVerify")
	fingerprint := getString("fingerprint")
	certificate := getString("certificate")
	privateKey := getString("private_key", "private-key", "privateKey")
	alpn := getStringSlice("alpn")
	proxy := getString("proxy")

	if !enable && serverURL == "" && token == "" && realmID == "" && len(stunServers) == 0 {
		return nil, false
	}

	return &RealmOptsParsed{
		Enable:         enable,
		ServerURL:      serverURL,
		Token:          token,
		RealmID:        realmID,
		STUNServers:    stunServers,
		SNI:            sni,
		SkipCertVerify: skipCertVerify,
		NameCertVerify: nameCertVerify,
		Fingerprint:    fingerprint,
		Certificate:    certificate,
		PrivateKey:     privateKey,
		ALPN:           alpn,
		Proxy:          proxy,
	}, true
}

// NormalizeMihomoHysteria2RealmOpts produces official Mihomo kebab-case map.
func NormalizeMihomoHysteria2RealmOpts(raw interface{}) (map[string]interface{}, bool) {
	parsed, ok := parseRawRealmOpts(raw)
	if !ok {
		return nil, false
	}

	out := make(map[string]interface{})
	out["enable"] = parsed.Enable
	if parsed.ServerURL != "" {
		out["server-url"] = parsed.ServerURL
	}
	if parsed.Token != "" {
		out["token"] = parsed.Token
	}
	if parsed.RealmID != "" {
		out["realm-id"] = parsed.RealmID
	}
	if len(parsed.STUNServers) > 0 {
		out["stun-servers"] = parsed.STUNServers
	}
	if parsed.SNI != "" {
		out["sni"] = parsed.SNI
	}
	if parsed.SkipCertVerify {
		out["skip-cert-verify"] = true
	}
	if parsed.NameCertVerify != "" {
		out["name-cert-verify"] = parsed.NameCertVerify
	}
	if parsed.Fingerprint != "" {
		out["fingerprint"] = parsed.Fingerprint
	}
	if parsed.Certificate != "" {
		out["certificate"] = parsed.Certificate
	}
	if parsed.PrivateKey != "" {
		out["private-key"] = parsed.PrivateKey
	}
	if len(parsed.ALPN) > 0 {
		out["alpn"] = parsed.ALPN
	}
	if parsed.Proxy != "" {
		out["proxy"] = parsed.Proxy
	}

	return out, true
}

// NormalizeMihomoHysteria2RealmOptsSnake produces internal snake_case map (for out_json / UI storage).
func NormalizeMihomoHysteria2RealmOptsSnake(raw interface{}) (map[string]interface{}, bool) {
	parsed, ok := parseRawRealmOpts(raw)
	if !ok {
		return nil, false
	}

	out := make(map[string]interface{})
	out["enable"] = parsed.Enable
	if parsed.ServerURL != "" {
		out["server_url"] = parsed.ServerURL
	}
	if parsed.Token != "" {
		out["token"] = parsed.Token
	}
	if parsed.RealmID != "" {
		out["realm_id"] = parsed.RealmID
	}
	if len(parsed.STUNServers) > 0 {
		out["stun_servers"] = parsed.STUNServers
	}
	if parsed.SNI != "" {
		out["sni"] = parsed.SNI
	}
	if parsed.SkipCertVerify {
		out["skip_cert_verify"] = true
	}
	if parsed.NameCertVerify != "" {
		out["name_cert_verify"] = parsed.NameCertVerify
	}
	if parsed.Fingerprint != "" {
		out["fingerprint"] = parsed.Fingerprint
	}
	if parsed.Certificate != "" {
		out["certificate"] = parsed.Certificate
	}
	if parsed.PrivateKey != "" {
		out["private_key"] = parsed.PrivateKey
	}
	if len(parsed.ALPN) > 0 {
		out["alpn"] = parsed.ALPN
	}
	if parsed.Proxy != "" {
		out["proxy"] = parsed.Proxy
	}

	return out, true
}

// BuildMihomoRealmOptsForClash extracts and formats realm-opts for Clash/Mihomo proxies.
// Returns non-nil only when realm-opts is enabled and has valid configuration.
func BuildMihomoRealmOptsForClash(outbound map[string]interface{}) map[string]interface{} {
	if outbound == nil {
		return nil
	}

	var rawOpts interface{}
	if v, ok := outbound["realm_opts"]; ok && v != nil {
		rawOpts = v
	} else if v, ok := outbound["realm-opts"]; ok && v != nil {
		rawOpts = v
	}

	if rawOpts == nil {
		return nil
	}

	norm, ok := NormalizeMihomoHysteria2RealmOpts(rawOpts)
	if !ok {
		return nil
	}

	enable, _ := norm["enable"].(bool)
	if !enable {
		return nil
	}

	serverURL, _ := norm["server-url"].(string)
	if strings.TrimSpace(serverURL) == "" {
		return nil
	}

	return norm
}

// StripMihomoRealmOpts removes realm-opts fields from outbound/inbound maps.
func StripMihomoRealmOpts(m map[string]interface{}) {
	if m == nil {
		return
	}
	delete(m, "realm_opts")
	delete(m, "realm-opts")
}
