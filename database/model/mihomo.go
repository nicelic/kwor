package model

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"
	"time"
)

func copyRawMessage(raw json.RawMessage) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	cloned := make([]byte, len(raw))
	copy(cloned, raw)
	return json.RawMessage(cloned)
}

var mihomoServerTLSKeysToStrip = []string{
	"min_version",
	"max_version",
	"cipher_suites",
	"client_authentication",
	"client_certificate",
	"client_certificate_path",
	"client_certificate_public_key_sha256",
}

var mihomoClientTLSKeysToStrip = []string{
	"store",
	"tls_store",
	"mihomo_use_fingerprint",
	"certificate",
	"certificate_path",
	"client_certificate",
	"client_certificate_path",
	"client_key",
	"client_key_path",
	"fragment",
	"fragment_fallback_delay",
	"record_fragment",
}

var mihomoOutboundKeysToStrip = []string{
	"inet4_bind_address",
	"inet6_bind_address",
	"reuse_addr",
	"udp_fragment",
	"connect_timeout",
	"domain_resolver",
}

var mihomoOutboundUTLSSupportedTypes = map[string]struct{}{
	"vmess":       {},
	"vless":       {},
	"trojan":      {},
	"anytls":      {},
	"shadowtls":   {},
	"trusttunnel": {},
}

const (
	MihomoTlsModeTLS       = "tls"
	MihomoTlsModeReality   = "reality"
	MihomoTlsModeShadowTLS = "shadow-tls"
	MihomoTlsModeRestls    = "restls"
	MihomoTlsModeJLS       = "jls"
)

var mihomoTlsModes = map[string]struct{}{
	MihomoTlsModeTLS:       {},
	MihomoTlsModeReality:   {},
	MihomoTlsModeShadowTLS: {},
	MihomoTlsModeRestls:    {},
	MihomoTlsModeJLS:       {},
}

func IsMihomoTlsMode(mode string) bool {
	normalized := strings.ToLower(strings.TrimSpace(mode))
	_, ok := mihomoTlsModes[normalized]
	return ok
}

func NormalizeMihomoTlsMode(mode string, server, client json.RawMessage) string {
	normalized := strings.ToLower(strings.TrimSpace(mode))
	if _, ok := mihomoTlsModes[normalized]; ok {
		return normalized
	}
	_ = server
	_ = client
	return MihomoTlsModeTLS
}

func sanitizeMihomoTLSRaw(raw json.RawMessage, keys ...string) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return copyRawMessage(raw)
	}

	for _, key := range keys {
		delete(payload, key)
	}
	sanitizeMihomoTLSOptionalFields(payload)

	if len(payload) == 0 {
		return json.RawMessage([]byte("{}"))
	}

	sanitized, err := json.Marshal(payload)
	if err != nil {
		return copyRawMessage(raw)
	}

	return json.RawMessage(sanitized)
}

func sanitizeMihomoTLSOptionalFields(payload map[string]interface{}) {
	if payload == nil {
		return
	}
	if raw, exists := payload["server_name"]; exists {
		value, ok := raw.(string)
		if !ok {
			delete(payload, "server_name")
		} else {
			payload["server_name"] = strings.TrimSpace(value)
		}
	}
	if raw, exists := payload["alpn"]; exists {
		values, ok := raw.([]interface{})
		if !ok {
			delete(payload, "alpn")
			return
		}
		clean := make([]interface{}, 0, len(values))
		for _, item := range values {
			value, ok := item.(string)
			value = strings.TrimSpace(value)
			if ok && value != "" {
				clean = append(clean, value)
			}
		}
		if len(clean) == 0 {
			delete(payload, "alpn")
		} else {
			payload["alpn"] = clean
		}
	}
}

func normalizeMihomoPort(raw interface{}) (int, bool) {
	const maxPort = 65535
	fromInt := func(value int64) (int, bool) {
		if value < 1 || value > maxPort {
			return 0, false
		}
		return int(value), true
	}

	switch value := raw.(type) {
	case int:
		return fromInt(int64(value))
	case int8:
		return fromInt(int64(value))
	case int16:
		return fromInt(int64(value))
	case int32:
		return fromInt(int64(value))
	case int64:
		return fromInt(value)
	case uint:
		if uint64(value) > maxPort {
			return 0, false
		}
		return fromInt(int64(value))
	case uint8:
		return fromInt(int64(value))
	case uint16:
		return fromInt(int64(value))
	case uint32:
		if value > maxPort {
			return 0, false
		}
		return fromInt(int64(value))
	case uint64:
		if value > maxPort {
			return 0, false
		}
		return fromInt(int64(value))
	case float32:
		return normalizeMihomoPort(float64(value))
	case float64:
		if math.IsNaN(value) || math.IsInf(value, 0) || math.Trunc(value) != value {
			return 0, false
		}
		if value < 1 || value > maxPort {
			return 0, false
		}
		return int(value), true
	case string:
		value = strings.TrimSpace(value)
		if value == "" {
			return 0, false
		}
		parsed, err := strconv.ParseUint(value, 10, 16)
		if err != nil || parsed > maxPort {
			return 0, false
		}
		return fromInt(int64(parsed))
	default:
		return 0, false
	}
}

func sanitizeMihomoRealityHandshakePort(server map[string]interface{}) {
	if server == nil {
		return
	}
	reality, ok := server["reality"].(map[string]interface{})
	if !ok || reality == nil {
		return
	}
	handshake, ok := reality["handshake"].(map[string]interface{})
	if !ok || handshake == nil {
		return
	}
	rawPort, exists := handshake["server_port"]
	if !exists {
		return
	}
	if port, ok := normalizeMihomoPort(rawPort); ok {
		handshake["server_port"] = port
		return
	}
	delete(handshake, "server_port")
}

func sanitizeMihomoOutboundRaw(raw json.RawMessage, outType string) json.RawMessage {
	if len(raw) == 0 {
		return nil
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return copyRawMessage(raw)
	}

	for _, key := range mihomoOutboundKeysToStrip {
		delete(payload, key)
	}

	switch strings.ToLower(strings.TrimSpace(outType)) {
	case "shadowquic":
		// ShadowQUIC uses native SNI/ALPN and never has a mihomo_TLS block.
		delete(payload, "tls")
		delete(payload, "detour")
		delete(payload, "routing_mark")
		delete(payload, "rule")
		delete(payload, "proxy")
	case "tuic":
		delete(payload, "mihomo_fast_open")
		delete(payload, "fast_open")
		delete(payload, "fast-open")
	case "selector":
		delete(payload, "default")
		delete(payload, "url")
		delete(payload, "interval")
		delete(payload, "tolerance")
		delete(payload, "idle_timeout")
		delete(payload, "interrupt_exist_connections")
	case "urltest":
		delete(payload, "default")
		delete(payload, "idle_timeout")
		delete(payload, "interrupt_exist_connections")
	}

	if tlsMap, ok := payload["tls"].(map[string]interface{}); ok && tlsMap != nil {
		sanitizeMihomoOutboundTLSMap(tlsMap, outType)
	}

	if len(payload) == 0 {
		return json.RawMessage([]byte("{}"))
	}

	sanitized, err := json.Marshal(payload)
	if err != nil {
		return copyRawMessage(raw)
	}

	return json.RawMessage(sanitized)
}

func sanitizeMihomoOutboundTLSMap(tlsMap map[string]interface{}, outType string) {
	if tlsMap == nil {
		return
	}

	// TLS fragmentation is a sing-box client option and has no Mihomo
	// proxy/listener projection. Do not keep an editable value that never
	// reaches the generated Mihomo configuration.
	delete(tlsMap, "fragment")
	delete(tlsMap, "fragment_fallback_delay")
	delete(tlsMap, "record_fragment")

	if _, ok := mihomoOutboundUTLSSupportedTypes[outType]; !ok {
		delete(tlsMap, "utls")
	}
	if outType == "trusttunnel" {
		// Mihomo's TrustTunnel proxy schema accepts a client fingerprint but
		// has no Reality or ECH projection. Keep persisted UI data aligned
		// with the generated Clash proxy instead of silently dropping it later.
		delete(tlsMap, "reality")
		delete(tlsMap, "ech")
	}
}

type MihomoTls struct {
	Id                  uint            `json:"id" form:"id" gorm:"primaryKey;autoIncrement"`
	Name                string          `json:"name" form:"name"`
	Mode                string          `json:"mode" form:"mode" gorm:"column:mode;not null;default:tls;index"`
	CertificateRecordID uint            `json:"certificateRecordId" form:"certificateRecordId" gorm:"column:certificate_record_id;not null;default:0;index"`
	Server              json.RawMessage `json:"server" form:"server"`
	Client              json.RawMessage `json:"client" form:"client"`
}

func (MihomoTls) TableName() string {
	return "mihomo_tls"
}

func (t *MihomoTls) Sanitize() {
	if t == nil {
		return
	}
	t.Mode = NormalizeMihomoTlsMode(t.Mode, t.Server, t.Client)
	t.Server = sanitizeMihomoTLSRaw(t.Server, mihomoServerTLSKeysToStrip...)
	t.Client = sanitizeMihomoTLSRaw(t.Client, mihomoClientTLSKeysToStrip...)
	sanitizeMihomoTLSModePayload(t)
}

// NormalizeWrapperDestinations applies the user-facing host:port shorthand to
// the three Mihomo TLS wrappers without altering any other submitted fields.
// It is intentionally separate from Sanitize so the service can normalize a
// destination before its first strict validation pass.
func (t *MihomoTls) NormalizeWrapperDestinations() {
	if t == nil {
		return
	}

	mode := NormalizeMihomoTlsMode(t.Mode, t.Server, t.Client)
	var server map[string]interface{}
	if json.Unmarshal(t.Server, &server) != nil || server == nil {
		return
	}

	normalizeMihomoWrapperDestinations(mode, server)
	encoded, err := json.Marshal(server)
	if err != nil {
		return
	}
	t.Server = json.RawMessage(encoded)
}

// sanitizeMihomoTLSModePayload keeps the persisted server/client halves
// mutually exclusive. A TLS record is a reusable template, so stale fields
// from a previous mode must not leak into the generated listener/subscription.
func sanitizeMihomoTLSModePayload(t *MihomoTls) {
	if t == nil {
		return
	}
	var server, client map[string]interface{}
	if json.Unmarshal(t.Server, &server) != nil || server == nil {
		server = map[string]interface{}{}
	}
	if json.Unmarshal(t.Client, &client) != nil || client == nil {
		client = map[string]interface{}{}
	}

	if t.Mode != MihomoTlsModeShadowTLS {
		delete(server, "shadow_tls")
	}
	if t.Mode != MihomoTlsModeRestls {
		delete(server, "res_tls")
	}
	if t.Mode != MihomoTlsModeJLS {
		delete(server, "jls_config")
	}
	normalizeMihomoWrapperDestinations(t.Mode, server)
	if t.Mode == MihomoTlsModeTLS {
		delete(server, "reality")
	} else if t.Mode == MihomoTlsModeReality {
		for _, key := range []string{"certificate", "certificate_path", "key", "key_path", "acme", "ech"} {
			delete(server, key)
		}
		sanitizeMihomoRealityHandshakePort(server)
		t.CertificateRecordID = 0
	} else if t.Mode == MihomoTlsModeShadowTLS || t.Mode == MihomoTlsModeRestls || t.Mode == MihomoTlsModeJLS {
		for _, key := range []string{"certificate", "certificate_path", "key", "key_path", "client_authentication", "client_certificate", "client_certificate_path", "client_certificate_public_key_sha256", "reality", "acme", "ech"} {
			delete(server, key)
		}
		t.CertificateRecordID = 0
	}
	if t.Mode == MihomoTlsModeRestls {
		if wrapper, ok := server["res_tls"].(map[string]interface{}); ok && wrapper != nil {
			// Restls and JLS both support rate-limit. Omit only an explicit null
			// value so the listener renderer can project valid values to YAML.
			if value, exists := wrapper["rate_limit"]; exists && value == nil {
				delete(wrapper, "rate_limit")
			}
			if value, exists := wrapper["min_record_len"]; exists && value == nil {
				delete(wrapper, "min_record_len")
			}
		}
		if wrapper, ok := server["res_tls"].(map[string]interface{}); ok && wrapper != nil {
			if opts, ok := client["restls_opts"].(map[string]interface{}); ok && opts != nil {
				serverScript := firstStringValue(wrapper["restls_script"])
				clientScript := firstStringValue(opts["restls_script"])
				script := serverScript
				if script == "" {
					script = clientScript
				}
				if script == "" {
					delete(wrapper, "restls_script")
					delete(opts, "restls_script")
				} else {
					wrapper["restls_script"] = script
					opts["restls_script"] = script
				}
			}
		}
	}
	activeWrapperKey := ""
	switch t.Mode {
	case MihomoTlsModeShadowTLS:
		activeWrapperKey = "shadow_tls_opts"
	case MihomoTlsModeRestls:
		activeWrapperKey = "restls_opts"
	case MihomoTlsModeJLS:
		activeWrapperKey = "jls_opts"
	}
	for _, key := range []string{"shadow_tls_opts", "restls_opts", "jls_opts"} {
		if opts, ok := client[key].(map[string]interface{}); ok && opts != nil {
			if key == activeWrapperKey {
				_, hasClientSNI := client["server_name"]
				if !hasClientSNI {
					if nestedSNI := firstStringValue(opts["server_name"]); nestedSNI != "" {
						client["server_name"] = nestedSNI
					}
				}
			}
			delete(opts, "server_name")
		}
	}
	if activeWrapperKey != "" {
		if _, hasClientSNI := client["server_name"]; !hasClientSNI {
			if legacySNI := firstStringValue(server["server_name"]); legacySNI != "" {
				client["server_name"] = legacySNI
			}
		}
		delete(server, "server_name")
	}
	var wrapper map[string]interface{}
	var destination interface{}
	switch t.Mode {
	case MihomoTlsModeShadowTLS:
		wrapper, _ = server["shadow_tls"].(map[string]interface{})
		if handshake, ok := wrapper["handshake"].(map[string]interface{}); ok && handshake != nil {
			destination = handshake["dest"]
		}
	case MihomoTlsModeRestls:
		wrapper, _ = server["res_tls"].(map[string]interface{})
		if wrapper != nil {
			destination = wrapper["dest"]
		}
	case MihomoTlsModeJLS:
		wrapper, _ = server["jls_config"].(map[string]interface{})
		if wrapper != nil {
			destination = wrapper["dest"]
		}
	}
	if activeWrapperKey != "" {
		rawClientSNI, hasClientSNI := client["server_name"]
		clientSNI, clientSNIIsString := rawClientSNI.(string)
		clientSNI = strings.TrimSpace(clientSNI)
		derivedSNI := mihomoWrapperSNIFromDestination(destination)
		if t.Mode == MihomoTlsModeJLS && wrapper != nil {
			serverSNI := firstStringValue(wrapper["sni"])
			fallbackSNI := derivedSNI
			// A missing outer SNI can be migrated from the historical JLS listener
			// field. An explicitly blank (including whitespace-only) outer SNI
			// instead means automatic derivation from dest.
			if !hasClientSNI || !clientSNIIsString {
				fallbackSNI = firstNonEmptyString(serverSNI, derivedSNI)
			}
			if sni := firstNonEmptyString(clientSNI, fallbackSNI); sni != "" {
				client["server_name"] = sni
				wrapper["sni"] = sni
			} else {
				delete(client, "server_name")
				delete(wrapper, "sni")
			}
		} else if sni := firstNonEmptyString(clientSNI, derivedSNI); sni != "" {
			client["server_name"] = sni
		} else {
			delete(client, "server_name")
		}
	}
	switch t.Mode {
	case MihomoTlsModeShadowTLS:
		if wrapper, ok := server["shadow_tls"].(map[string]interface{}); ok && wrapper != nil {
			sanitizeMihomoShadowTLSWrapper(wrapper)
			wrapper["enable"] = true
			if opts, ok := client["shadow_tls_opts"].(map[string]interface{}); ok && opts != nil {
				if mihomoShadowTLSVersion(wrapper) == 1 {
					// ShadowTLS v1 has no password in the official client
					// options. Do not carry a v2/v3 credential across a
					// version switch.
					delete(opts, "password")
				}
			}
		}
	case MihomoTlsModeRestls:
		if wrapper, ok := server["res_tls"].(map[string]interface{}); ok && wrapper != nil {
			wrapper["enable"] = true
		}
	case MihomoTlsModeJLS:
		if wrapper, ok := server["jls_config"].(map[string]interface{}); ok && wrapper != nil {
			sanitizeMihomoSingleUserList(wrapper)
			wrapper["enable"] = true
			if value, exists := wrapper["rate_limit"]; exists && value == nil {
				delete(wrapper, "rate_limit")
			}
		}
	}

	for _, key := range []string{"shadow_tls_opts", "restls_opts", "jls_opts", "reality", "ech"} {
		keep := (t.Mode == MihomoTlsModeShadowTLS && key == "shadow_tls_opts") ||
			(t.Mode == MihomoTlsModeRestls && key == "restls_opts") ||
			(t.Mode == MihomoTlsModeJLS && key == "jls_opts") ||
			(t.Mode == MihomoTlsModeReality && key == "reality") ||
			(t.Mode == MihomoTlsModeTLS && key == "ech")
		if !keep {
			delete(client, key)
		}
	}
	removeEmptyMihomoTLSSNI(server)
	removeEmptyMihomoTLSSNI(client)

	serverJSON, _ := json.Marshal(server)
	clientJSON, _ := json.Marshal(client)
	t.Server = json.RawMessage(serverJSON)
	t.Client = json.RawMessage(clientJSON)
}

func removeEmptyMihomoTLSSNI(payload map[string]interface{}) {
	if payload == nil {
		return
	}
	if raw, exists := payload["server_name"]; exists {
		value, ok := raw.(string)
		if !ok || strings.TrimSpace(value) == "" {
			delete(payload, "server_name")
		}
	}
}

func sanitizeMihomoShadowTLSWrapper(wrapper map[string]interface{}) {
	if wrapper == nil {
		return
	}
	version := mihomoShadowTLSVersion(wrapper)
	if version != 3 {
		delete(wrapper, "strict_mode")
		delete(wrapper, "wildcard_sni")
	}
	if version <= 1 {
		delete(wrapper, "handshake_for_server_name")
	}
	if version > 0 && version < 3 {
		delete(wrapper, "wildcard_sni")
	}
	if version == 3 {
		sanitizeMihomoSingleUserList(wrapper)
		delete(wrapper, "password")
	} else if version == 2 {
		delete(wrapper, "users")
	} else if version == 1 {
		delete(wrapper, "password")
		delete(wrapper, "users")
	}

	if raw, exists := wrapper["wildcard_sni"]; exists {
		value, ok := raw.(string)
		value = strings.ToLower(strings.TrimSpace(value))
		if !ok || (value != "" && value != "off" && value != "authed" && value != "all") {
			delete(wrapper, "wildcard_sni")
		} else if value == "" {
			delete(wrapper, "wildcard_sni")
		} else {
			wrapper["wildcard_sni"] = value
		}
	}

	rawMappings, exists := wrapper["handshake_for_server_name"]
	if !exists {
		return
	}
	if version == 1 {
		delete(wrapper, "handshake_for_server_name")
		return
	}
	mappings, ok := rawMappings.(map[string]interface{})
	if !ok || mappings == nil {
		delete(wrapper, "handshake_for_server_name")
		return
	}
	clean := make(map[string]interface{}, len(mappings))
	for rawName, rawHandshake := range mappings {
		name := strings.TrimSpace(rawName)
		handshake, ok := rawHandshake.(map[string]interface{})
		if !ok || handshake == nil || name == "" {
			continue
		}
		dest := strings.TrimSpace(firstStringValue(handshake["dest"]))
		if dest == "" {
			continue
		}
		entry := map[string]interface{}{"dest": dest}
		if proxy := strings.TrimSpace(firstStringValue(handshake["proxy"])); proxy != "" {
			entry["proxy"] = proxy
		}
		clean[name] = entry
	}
	if len(clean) == 0 {
		delete(wrapper, "handshake_for_server_name")
	} else {
		wrapper["handshake_for_server_name"] = clean
	}
}

func sanitizeMihomoSingleUserList(wrapper map[string]interface{}) {
	if wrapper == nil {
		return
	}
	users, ok := wrapper["users"].([]interface{})
	if !ok {
		return
	}
	if len(users) == 0 {
		wrapper["users"] = []interface{}{}
		return
	}
	wrapper["users"] = []interface{}{users[0]}
}

func mihomoShadowTLSVersion(wrapper map[string]interface{}) int {
	if wrapper == nil {
		return 0
	}
	switch value := wrapper["version"].(type) {
	case int:
		return value
	case int8:
		return int(value)
	case int16:
		return int(value)
	case int32:
		return int(value)
	case int64:
		return int(value)
	case uint:
		return int(value)
	case uint8:
		return int(value)
	case uint16:
		return int(value)
	case uint32:
		return int(value)
	case uint64:
		return int(value)
	case float64:
		return int(value)
	case float32:
		return int(value)
	default:
		return 0
	}
}

func firstStringValue(raw interface{}) string {
	switch value := raw.(type) {
	case string:
		return strings.TrimSpace(value)
	default:
		return ""
	}
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if normalized := strings.TrimSpace(value); normalized != "" {
			return normalized
		}
	}
	return ""
}

func normalizeMihomoWrapperDestinations(mode string, server map[string]interface{}) {
	if server == nil {
		return
	}

	normalizeDestination := func(payload map[string]interface{}, key string) {
		if payload == nil {
			return
		}
		if destination, ok := payload[key].(string); ok {
			payload[key] = normalizeMihomoWrapperDestination(destination)
		}
	}

	switch mode {
	case MihomoTlsModeShadowTLS:
		if wrapper, ok := server["shadow_tls"].(map[string]interface{}); ok && wrapper != nil {
			if handshake, ok := wrapper["handshake"].(map[string]interface{}); ok && handshake != nil {
				normalizeDestination(handshake, "dest")
			}
		}
	case MihomoTlsModeRestls:
		if wrapper, ok := server["res_tls"].(map[string]interface{}); ok && wrapper != nil {
			normalizeDestination(wrapper, "dest")
		}
	case MihomoTlsModeJLS:
		if wrapper, ok := server["jls_config"].(map[string]interface{}); ok && wrapper != nil {
			normalizeDestination(wrapper, "dest")
		}
	}
}

func normalizeMihomoWrapperDestination(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}

	if strings.HasPrefix(value, "[") {
		closing := strings.Index(value, "]")
		if closing <= 1 {
			return value
		}
		host := strings.TrimSpace(value[1:closing])
		suffix := value[closing+1:]
		if host == "" {
			return value
		}
		if suffix == "" {
			return "[" + host + "]:443"
		}
		if strings.HasPrefix(suffix, ":") && isMihomoWrapperPortText(suffix[1:]) {
			return "[" + host + "]:" + suffix[1:]
		}
		return value
	}

	if strings.ContainsAny(value, " \t\r\n") {
		return value
	}
	if strings.Count(value, ":") == 0 {
		return value + ":443"
	}
	if strings.Count(value, ":") == 1 {
		host, port, found := strings.Cut(value, ":")
		if found && host != "" && isMihomoWrapperPortText(port) {
			return host + ":" + port
		}
	}
	return value
}

func isMihomoWrapperPortText(value string) bool {
	if value == "" {
		return false
	}
	for _, r := range value {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func mihomoWrapperSNIFromDestination(raw interface{}) string {
	value := firstStringValue(raw)
	if value == "" {
		return ""
	}
	if strings.HasPrefix(value, "[") {
		if closing := strings.Index(value, "]"); closing > 1 {
			return value[1:closing]
		}
		return ""
	}
	if colon := strings.LastIndex(value, ":"); colon > 0 && strings.Count(value, ":") == 1 {
		return strings.TrimSpace(value[:colon])
	}
	if !strings.Contains(value, ":") {
		return value
	}
	return ""
}

func (t *MihomoTls) ToBase() *Tls {
	if t == nil {
		return nil
	}
	cloned := &MihomoTls{
		Id:                  t.Id,
		Name:                t.Name,
		Mode:                t.Mode,
		CertificateRecordID: t.CertificateRecordID,
		Server:              copyRawMessage(t.Server),
		Client:              copyRawMessage(t.Client),
	}
	cloned.Sanitize()
	return &Tls{
		Id:                  cloned.Id,
		Name:                cloned.Name,
		CertificateRecordID: cloned.CertificateRecordID,
		Server:              copyRawMessage(cloned.Server),
		Client:              copyRawMessage(cloned.Client),
		Mode:                cloned.Mode,
	}
}

func mihomoTlsFromBase(base *Tls) *MihomoTls {
	if base == nil {
		return nil
	}
	tls := &MihomoTls{
		Id:                  base.Id,
		Name:                base.Name,
		Mode:                base.Mode,
		CertificateRecordID: base.CertificateRecordID,
		Server:              copyRawMessage(base.Server),
		Client:              copyRawMessage(base.Client),
	}
	tls.Sanitize()
	return tls
}

type MihomoClient struct {
	Id                    uint            `json:"id" form:"id" gorm:"primaryKey;autoIncrement"`
	Enable                bool            `json:"enable" form:"enable"`
	Depleted              bool            `json:"-" form:"-" gorm:"not null;default:false"`
	Name                  string          `json:"name" form:"name"`
	Config                json.RawMessage `json:"config,omitempty" form:"config"`
	Inbounds              json.RawMessage `json:"inbounds" form:"inbounds"`
	Links                 json.RawMessage `json:"links,omitempty" form:"links"`
	Volume                int64           `json:"volume" form:"volume"`
	Expiry                int64           `json:"expiry" form:"expiry"`
	Down                  int64           `json:"down" form:"down"`
	Up                    int64           `json:"up" form:"up"`
	Desc                  string          `json:"desc" form:"desc"`
	Group                 string          `json:"group" form:"group"`
	ServerIp              string          `json:"serverIp" form:"serverIp"`
	SpeedLimitMbps        int             `json:"speedLimitMbps" form:"speedLimitMbps"`
	Extra                 int             `json:"extra" form:"extra"`
	LastReset             int64           `json:"lastReset" form:"lastReset"`
	TrafficResetRequested bool            `json:"trafficResetRequested" form:"trafficResetRequested" gorm:"-"`
	AutoSync              bool            `json:"autoSync" form:"-" gorm:"-"`
}

func (MihomoClient) TableName() string {
	return "mihomo_clients"
}

type MihomoInbound struct {
	Id   uint   `json:"id" form:"id" gorm:"primaryKey;autoIncrement"`
	Type string `json:"type" form:"type"`
	Tag  string `json:"tag" form:"tag" gorm:"unique"`

	TlsId uint       `json:"tls_id" form:"tls_id"`
	Tls   *MihomoTls `json:"tls" form:"tls" gorm:"foreignKey:TlsId;references:Id"`

	Addrs   json.RawMessage `json:"addrs" form:"addrs"`
	OutJson json.RawMessage `json:"out_json" form:"out_json"`
	Options json.RawMessage `json:"-" form:"-"`
}

func (MihomoInbound) TableName() string {
	return "mihomo_inbounds"
}

func (i *MihomoInbound) UnmarshalJSON(data []byte) error {
	var base Inbound
	if err := base.UnmarshalJSON(data); err != nil {
		return err
	}
	i.FromBase(base)
	return nil
}

func (i MihomoInbound) MarshalJSON() ([]byte, error) {
	base := i.ToBase()
	return base.MarshalJSON()
}

func (i MihomoInbound) MarshalFull() (*map[string]interface{}, error) {
	base := i.ToBase()
	return base.MarshalFull()
}

func (i MihomoInbound) ToBase() Inbound {
	base := Inbound{
		Id:      i.Id,
		Type:    i.Type,
		Tag:     i.Tag,
		TlsId:   i.TlsId,
		Addrs:   copyRawMessage(i.Addrs),
		OutJson: copyRawMessage(i.OutJson),
		Options: copyRawMessage(i.Options),
	}
	base.Tls = i.Tls.ToBase()
	return base
}

func (i *MihomoInbound) FromBase(base Inbound) {
	i.Id = base.Id
	i.Type = base.Type
	i.Tag = base.Tag
	i.TlsId = base.TlsId
	i.Tls = mihomoTlsFromBase(base.Tls)
	i.Addrs = copyRawMessage(base.Addrs)
	i.OutJson = copyRawMessage(base.OutJson)
	i.Options = copyRawMessage(base.Options)
}

type MihomoOutbound struct {
	Id           uint            `json:"id" form:"id" gorm:"primaryKey;autoIncrement"`
	Type         string          `json:"type" form:"type"`
	Tag          string          `json:"tag" form:"tag" gorm:"unique"`
	Options      json.RawMessage `json:"-" form:"-"`
	RawOutbound  json.RawMessage `json:"-" form:"-" gorm:"type:text"`
	RawClashYAML []byte          `json:"-" form:"-" gorm:"type:blob"`
}

func (MihomoOutbound) TableName() string {
	return "mihomo_outbounds"
}

func (o *MihomoOutbound) UnmarshalJSON(data []byte) error {
	var base Outbound
	if err := base.UnmarshalJSON(data); err != nil {
		return err
	}
	o.FromBase(base)
	return nil
}

func (o MihomoOutbound) MarshalJSON() ([]byte, error) {
	base := o.ToBase()
	return base.MarshalJSON()
}

func (o MihomoOutbound) ToBase() Outbound {
	return Outbound{
		Id:      o.Id,
		Type:    o.Type,
		Tag:     o.Tag,
		Options: sanitizeMihomoOutboundRaw(copyRawMessage(o.Options), o.Type),
	}
}

func (o *MihomoOutbound) FromBase(base Outbound) {
	o.Id = base.Id
	o.Type = base.Type
	o.Tag = base.Tag
	o.Options = sanitizeMihomoOutboundRaw(copyRawMessage(base.Options), base.Type)
}

type MihomoOutboundGroup struct {
	Id              uint      `json:"id" gorm:"primaryKey;autoIncrement"`
	Name            string    `json:"name" gorm:"unique;not null"`
	SortOrder       int       `json:"sort_order" gorm:"default:0;index"`
	Outbounds       string    `json:"outbounds" gorm:"type:text"`
	SubscriptionUrl string    `json:"subscription_url" gorm:"type:text"`
	AllowInsecure   bool      `json:"allow_insecure" gorm:"default:false"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

func (MihomoOutboundGroup) TableName() string {
	return "mihomo_outbound_groups"
}
