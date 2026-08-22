package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"net"
	"strconv"
	"strings"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/util/common"
	"gorm.io/gorm"
)

type MihomoTlsService struct {
	MihomoInboundService
}

const maxMihomoTLSJSONBytes = 2 * 1024 * 1024

func (s *MihomoTlsService) GetAll() ([]model.MihomoTls, error) {
	db := database.GetDB()
	tlsConfig := []model.MihomoTls{}
	err := db.Model(model.MihomoTls{}).Scan(&tlsConfig).Error
	if err != nil {
		return nil, err
	}
	for i := range tlsConfig {
		tlsConfig[i].Sanitize()
	}
	return tlsConfig, nil
}

func (s *MihomoTlsService) Save(tx *gorm.DB, action string, data json.RawMessage, hostname string) error {
	switch action {
	case "new", "edit":
		if len(data) > maxMihomoTLSJSONBytes {
			return common.NewErrorf("mihomo TLS payload exceeds the %d byte limit", maxMihomoTLSJSONBytes)
		}
		var tls model.MihomoTls
		if err := json.Unmarshal(data, &tls); err != nil {
			return err
		}
		if action == "new" {
			// A copied TLS template must receive a fresh database identity.
			tls.Id = 0
		} else if action == "edit" {
			if tls.Id == 0 {
				return common.NewError("mihomo TLS id is required for edit")
			}
			var existing model.MihomoTls
			if err := tx.Model(model.MihomoTls{}).Select("id").Where("id = ?", tls.Id).First(&existing).Error; err != nil {
				return err
			}
		}
		if err := validateMihomoTLSJSONShape(tls.Server, "server"); err != nil {
			return err
		}
		if err := validateMihomoTLSJSONShape(tls.Client, "client"); err != nil {
			return err
		}
		if rawMode := strings.TrimSpace(tls.Mode); rawMode != "" && !model.IsMihomoTlsMode(rawMode) {
			return common.NewErrorf("unsupported mihomo TLS mode: %s", rawMode)
		}
		// Match the editor's host-only shorthand before strict validation. Keep
		// the validation before and after Sanitize so malformed active fields are
		// still rejected rather than silently discarded by cleanup.
		tls.NormalizeWrapperDestinations()
		if err := validateMihomoTLSMode(&tls); err != nil {
			return err
		}
		tls.Sanitize()
		if err := validateMihomoTLSMode(&tls); err != nil {
			return err
		}
		if err := validateAndNormalizeMihomoTLSOutboundReferences(tx, &tls); err != nil {
			return err
		}
		if err := validateMihomoTLSRecordSize(&tls); err != nil {
			return err
		}
		if err := tx.Save(&tls).Error; err != nil {
			return err
		}
		if action == "edit" {
			var inbounds []model.MihomoInbound
			err := tx.Model(model.MihomoInbound{}).Preload("Tls").Where("tls_id = ?", tls.Id).Find(&inbounds).Error
			if err != nil {
				return err
			}
			if len(inbounds) > 0 {
				for index := range inbounds {
					inbounds[index].Tls = &tls
					if err := validateMihomoInboundTLSMode(&inbounds[index]); err != nil {
						return err
					}
				}
				if err := s.MihomoClientService.UpdateLinksByInboundChange(tx, &inbounds, hostname, ""); err != nil {
					return err
				}
				if err := s.MihomoInboundService.updateOutJSONsForLoadedInbounds(tx, inbounds, hostname); err != nil {
					return common.NewError("unable to update out_json of mihomo inbounds: ", err.Error())
				}
			}
		}
	case "del":
		var id uint
		if err := json.Unmarshal(data, &id); err != nil {
			return err
		}
		var inboundCount int64
		if err := tx.Model(model.MihomoInbound{}).Where("tls_id = ?", id).Count(&inboundCount).Error; err != nil {
			return err
		}
		if inboundCount > 0 {
			return common.NewError("tls in use")
		}
		if err := tx.Where("id = ?", id).Delete(model.MihomoTls{}).Error; err != nil {
			return err
		}
	default:
		return common.NewErrorf("unknown action: %s", action)
	}

	return nil
}

func validateMihomoTLSMode(tls *model.MihomoTls) error {
	if tls == nil {
		return common.NewError("mihomo TLS is required")
	}
	mode := model.NormalizeMihomoTlsMode(tls.Mode, tls.Server, tls.Client)
	if mode == model.MihomoTlsModeTLS || mode == model.MihomoTlsModeReality {
		return nil
	}

	var server, client map[string]interface{}
	if err := json.Unmarshal(tls.Server, &server); err != nil || server == nil {
		return common.NewErrorf("mihomo %s server configuration is required", mode)
	}
	if err := json.Unmarshal(tls.Client, &client); err != nil || client == nil {
		return common.NewErrorf("mihomo %s client configuration is required", mode)
	}

	getMap := func(root map[string]interface{}, key string) (map[string]interface{}, error) {
		value, ok := root[key].(map[string]interface{})
		if !ok || value == nil {
			return nil, common.NewErrorf("mihomo %s %s is required", mode, key)
		}
		return value, nil
	}
	getString := func(root map[string]interface{}, key string) string {
		value, _ := root[key].(string)
		return strings.TrimSpace(value)
	}
	getRawString := func(root map[string]interface{}, key string) string {
		value, _ := root[key].(string)
		return value
	}
	getOptionalString := func(root map[string]interface{}, key, label string) (string, error) {
		if root == nil {
			return "", nil
		}
		raw, exists := root[key]
		if !exists || raw == nil {
			return "", nil
		}
		value, ok := raw.(string)
		if !ok {
			return "", common.NewErrorf("%s must be a string", label)
		}
		return strings.TrimSpace(value), nil
	}
	getStringList := func(root map[string]interface{}, key, label string) ([]string, error) {
		if root == nil {
			return nil, nil
		}
		raw, exists := root[key]
		if !exists || raw == nil {
			return nil, nil
		}
		values := make([]interface{}, 0)
		switch value := raw.(type) {
		case []interface{}:
			values = value
		case []string:
			for _, item := range value {
				values = append(values, item)
			}
		default:
			return nil, common.NewErrorf("%s must be an array of strings", label)
		}
		result := make([]string, 0, len(values))
		for _, rawValue := range values {
			value, ok := rawValue.(string)
			if !ok || strings.TrimSpace(value) == "" {
				return nil, common.NewErrorf("%s must contain only non-empty strings", label)
			}
			result = append(result, strings.TrimSpace(value))
		}
		return result, nil
	}
	getStrictInt := func(root map[string]interface{}, key, label string) (int, bool, error) {
		raw, exists := root[key]
		if !exists || raw == nil {
			return 0, false, nil
		}
		switch value := raw.(type) {
		case int:
			return value, true, nil
		case int64:
			return int(value), true, nil
		case float64:
			if math.IsNaN(value) || math.IsInf(value, 0) || value != math.Trunc(value) {
				return 0, false, common.NewErrorf("%s must be an integer", label)
			}
			return int(value), true, nil
		default:
			return 0, false, common.NewErrorf("%s must be an integer", label)
		}
	}
	if _, err := getOptionalString(client, "server_name", "Mihomo TLS client SNI"); err != nil {
		return err
	}

	switch mode {
	case model.MihomoTlsModeShadowTLS:
		config, err := getMap(server, "shadow_tls")
		if err != nil {
			return err
		}
		opts, err := getMap(client, "shadow_tls_opts")
		if err != nil {
			return err
		}
		version, _, err := getStrictInt(config, "version", "ShadowTLS version")
		if err != nil {
			return err
		}
		if version < 1 || version > 3 {
			return common.NewError("ShadowTLS version must be 1, 2 or 3")
		}
		clientVersion, _, err := getStrictInt(opts, "version", "ShadowTLS client version")
		if err != nil {
			return err
		}
		if clientVersion != version {
			return common.NewError("ShadowTLS client/server versions must match")
		}
		if version == 3 {
			if rawStrict, exists := config["strict_mode"]; exists {
				if _, ok := rawStrict.(bool); !ok {
					return common.NewError("ShadowTLS strict-mode must be a boolean")
				}
			}
		}
		serverPassword := getRawString(config, "password")
		clientPassword := getRawString(opts, "password")
		if version == 2 && strings.TrimSpace(serverPassword) == "" {
			return common.NewError("ShadowTLS password is required")
		}
		if raw, exists := config["handshake"]; exists && raw != nil {
			handshakeConfig, ok := raw.(map[string]interface{})
			if !ok || handshakeConfig == nil {
				return common.NewError("ShadowTLS handshake must be an object")
			}
			if _, err := getOptionalString(handshakeConfig, "dest", "ShadowTLS handshake destination"); err != nil {
				return err
			}
			if _, err := getOptionalString(handshakeConfig, "proxy", "ShadowTLS handshake proxy"); err != nil {
				return err
			}
		}
		if version == 2 {
			if strings.TrimSpace(clientPassword) == "" || clientPassword != serverPassword {
				return common.NewError("ShadowTLS v2 client/server passwords must match")
			}
		} else if version == 3 {
			if strings.TrimSpace(clientPassword) == "" {
				return common.NewError("ShadowTLS v3 client password is required")
			}
			users, ok := config["users"].([]interface{})
			if !ok || len(users) != 1 {
				return common.NewError("ShadowTLS v3 requires exactly one user")
			}
			matched := false
			for _, raw := range users {
				user, ok := raw.(map[string]interface{})
				userName := getRawString(user, "name")
				userPassword := getRawString(user, "password")
				if !ok || strings.TrimSpace(userName) == "" || strings.TrimSpace(userPassword) == "" {
					return common.NewError("ShadowTLS v3 users require name and password")
				}
				if userPassword == clientPassword {
					matched = true
				}
			}
			if !matched {
				return common.NewError("ShadowTLS client password must match a v3 user")
			}
		}
		wildcardSNI := "off"
		if version == 3 {
			if raw, exists := config["wildcard_sni"]; exists {
				value, ok := raw.(string)
				if !ok {
					return common.NewError("ShadowTLS wildcard-sni must be off, authed or all")
				}
				wildcardSNI = strings.ToLower(strings.TrimSpace(value))
				if wildcardSNI != "off" && wildcardSNI != "authed" && wildcardSNI != "all" {
					return common.NewError("ShadowTLS wildcard-sni must be off, authed or all")
				}
				if version < 3 && wildcardSNI != "off" {
					return common.NewError("ShadowTLS wildcard-sni is only supported by version 3")
				}
			}
		}
		handshake, _ := config["handshake"].(map[string]interface{})
		handshakeDest := getString(handshake, "dest")
		if version < 3 || wildcardSNI != "all" {
			if err := validateMihomoTLSDestination(handshakeDest, "ShadowTLS handshake destination"); err != nil {
				return err
			}
		} else if handshakeDest != "" {
			if err := validateMihomoTLSDestination(handshakeDest, "ShadowTLS handshake destination"); err != nil {
				return err
			}
		}
		if version >= 2 {
			if raw, exists := config["handshake_for_server_name"]; exists {
				mappings, ok := raw.(map[string]interface{})
				if !ok || mappings == nil {
					return common.NewError("ShadowTLS handshake-for-server-name must be an object")
				}
				for name, rawHandshake := range mappings {
					if strings.TrimSpace(name) == "" {
						return common.NewError("ShadowTLS handshake-for-server-name contains an empty SNI")
					}
					handshake, ok := rawHandshake.(map[string]interface{})
					if !ok || handshake == nil {
						return common.NewError("ShadowTLS handshake-for-server-name entries must be objects")
					}
					if err := validateMihomoTLSDestination(getString(handshake, "dest"), "ShadowTLS SNI handshake destination"); err != nil {
						return err
					}
					if _, err := getOptionalString(handshake, "proxy", "ShadowTLS SNI handshake proxy"); err != nil {
						return err
					}
				}
			}
		}
	case model.MihomoTlsModeRestls:
		config, err := getMap(server, "res_tls")
		if err != nil {
			return err
		}
		opts, err := getMap(client, "restls_opts")
		if err != nil {
			return err
		}
		if err := validateMihomoTLSDestination(getString(config, "dest"), "Restls destination"); err != nil {
			return err
		}
		serverPassword := getRawString(config, "password")
		clientPassword := getRawString(opts, "password")
		if strings.TrimSpace(serverPassword) == "" {
			return common.NewError("Restls destination and password are required")
		}
		for _, field := range []struct {
			root  map[string]interface{}
			key   string
			label string
		}{
			{root: config, key: "restls_script", label: "Restls server script"},
			{root: config, key: "proxy", label: "Restls server proxy"},
			{root: opts, key: "restls_script", label: "Restls client script"},
		} {
			if _, err := getOptionalString(field.root, field.key, field.label); err != nil {
				return err
			}
		}
		minRecordLen, hasMinRecordLen, err := getStrictInt(config, "min_record_len", "Restls min-record-len")
		if err != nil {
			return err
		}
		if hasMinRecordLen && minRecordLen < 0 {
			return common.NewError("Restls min-record-len must be non-negative")
		}
		rateLimit, hasRateLimit, err := getStrictInt(config, "rate_limit", "Restls rate-limit")
		if err != nil {
			return err
		}
		if hasRateLimit && rateLimit < 0 {
			return common.NewError("Restls rate-limit must be non-negative")
		}
		if strings.TrimSpace(clientPassword) == "" || clientPassword != serverPassword {
			return common.NewError("Restls client/server passwords must match")
		}
		versionHint := strings.ToLower(getString(opts, "version_hint"))
		if versionHint != "tls12" && versionHint != "tls13" {
			return common.NewError("Restls version hint must be tls12 or tls13")
		}
	case model.MihomoTlsModeJLS:
		config, err := getMap(server, "jls_config")
		if err != nil {
			return err
		}
		opts, err := getMap(client, "jls_opts")
		if err != nil {
			return err
		}
		if err := validateMihomoTLSDestination(getString(config, "dest"), "JLS destination"); err != nil {
			return err
		}
		if _, err := getOptionalString(config, "sni", "JLS SNI"); err != nil {
			return err
		}
		if _, err := getOptionalString(config, "proxy", "JLS proxy"); err != nil {
			return err
		}
		if _, err := getStringList(config, "alpn", "JLS ALPN"); err != nil {
			return err
		}
		username := getRawString(opts, "username")
		password := getRawString(opts, "password")
		if strings.TrimSpace(username) == "" || strings.TrimSpace(password) == "" {
			return common.NewError("JLS username and password are required")
		}
		users, ok := config["users"].([]interface{})
		if !ok || len(users) != 1 {
			return common.NewError("JLS requires exactly one user")
		}
		matched := false
		for _, raw := range users {
			user, ok := raw.(map[string]interface{})
			userName := getRawString(user, "username")
			userPassword := getRawString(user, "password")
			if !ok || strings.TrimSpace(userName) == "" || strings.TrimSpace(userPassword) == "" {
				return common.NewError("JLS users require username and password")
			}
			if userName == username && userPassword == password {
				matched = true
			}
		}
		if !matched {
			return common.NewError("JLS client credentials must match a server user")
		}
		rateLimit, hasRateLimit, err := getStrictInt(config, "rate_limit", "JLS rate-limit")
		if err != nil {
			return err
		}
		if hasRateLimit && rateLimit < 0 {
			return common.NewError("JLS rate-limit must be non-negative")
		}
	default:
		return common.NewErrorf("unsupported Mihomo TLS mode: %s", mode)
	}

	return nil
}

func validateMihomoTLSDestination(raw, label string) error {
	value := strings.TrimSpace(raw)
	if value == "" {
		return common.NewErrorf("%s is required", label)
	}
	host, port, err := net.SplitHostPort(value)
	if err != nil || strings.TrimSpace(host) == "" {
		return common.NewErrorf("%s must be a host:port address", label)
	}
	portNumber, err := strconv.Atoi(port)
	if err != nil || portNumber < 1 || portNumber > 65535 {
		return common.NewErrorf("%s must use a port between 1 and 65535", label)
	}
	return nil
}

func validateMihomoInboundTLSMode(inbound *model.MihomoInbound) error {
	if inbound == nil || inbound.Tls == nil {
		return nil
	}
	mode := model.NormalizeMihomoTlsMode(inbound.Tls.Mode, inbound.Tls.Server, inbound.Tls.Client)
	typeName := strings.ToLower(strings.TrimSpace(inbound.Type))
	supportedWrapper := typeName == "vmess" || typeName == "vless" || typeName == "trojan" || typeName == "anytls" || typeName == "shadowsocks"
	if typeName == "shadowsocks" && (mode == model.MihomoTlsModeTLS || mode == model.MihomoTlsModeReality) {
		return common.NewError("Mihomo Shadowsocks inbound only supports ShadowTLS, Restls or JLS TLS modes")
	}
	if (mode == model.MihomoTlsModeShadowTLS || mode == model.MihomoTlsModeRestls || mode == model.MihomoTlsModeJLS) && !supportedWrapper {
		return common.NewErrorf("mihomo %s TLS is only supported by Shadowsocks, VMess, VLESS, Trojan and AnyTLS inbounds", mode)
	}
	return nil
}

func validateMihomoTLSRecordSize(tls *model.MihomoTls) error {
	if tls == nil {
		return common.NewError("mihomo TLS is required")
	}
	if len(tls.Server) > maxMihomoTLSJSONBytes || len(tls.Client) > maxMihomoTLSJSONBytes {
		return common.NewErrorf("mihomo TLS server/client configuration exceeds the %d byte limit", maxMihomoTLSJSONBytes)
	}
	if len(tls.Server)+len(tls.Client) > maxMihomoTLSJSONBytes {
		return fmt.Errorf("mihomo TLS configuration exceeds the %d byte limit", maxMihomoTLSJSONBytes)
	}
	return nil
}

func validateMihomoTLSJSONShape(raw json.RawMessage, field string) error {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 || bytes.Equal(trimmed, []byte("null")) {
		return nil
	}

	var value interface{}
	if err := json.Unmarshal(trimmed, &value); err != nil {
		return common.NewErrorf("mihomo TLS %s JSON is invalid: %v", field, err)
	}
	if _, ok := value.(map[string]interface{}); !ok {
		return common.NewErrorf("mihomo TLS %s must be a JSON object", field)
	}
	return nil
}
