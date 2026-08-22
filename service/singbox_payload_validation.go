package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/alireza0/s-ui/database/model"
)

// singboxEntityPayload keeps the identity fields in their original JSON form
// until they have passed strict validation. Model unmarshalling uses generic
// maps for compatibility, which otherwise turns values such as 443.5 into a
// float64 before a later conversion can accidentally truncate them.
type singboxEntityPayload struct {
	Fields map[string]json.RawMessage
	ID     uint
	Type   string
	Tag    string
}

const maxSingboxJSONSafeInteger = uint64(9007199254740991)

func parseSingboxEntityPayload(data json.RawMessage, action string, entity string) (*singboxEntityPayload, error) {
	fields := map[string]json.RawMessage{}
	if err := json.Unmarshal(data, &fields); err != nil || fields == nil {
		if err != nil {
			return nil, fmt.Errorf("invalid sing-box %s payload: %w", entity, err)
		}
		return nil, fmt.Errorf("sing-box %s payload must be an object", entity)
	}

	typeValue, err := requiredSingboxPayloadString(fields, "type", entity)
	if err != nil {
		return nil, err
	}
	tagValue, err := requiredSingboxPayloadString(fields, "tag", entity)
	if err != nil {
		return nil, err
	}

	identity := &singboxEntityPayload{
		Fields: fields,
		Type:   strings.ToLower(strings.TrimSpace(typeValue)),
		Tag:    strings.TrimSpace(tagValue),
	}

	if rawID, exists := fields["id"]; exists && !isSingboxJSONNull(rawID) {
		id, parseErr := parseStrictSingboxUnsigned(rawID, 0, uint64(^uint(0)))
		if parseErr != nil {
			return nil, fmt.Errorf("sing-box %s id: %w", entity, parseErr)
		}
		identity.ID = uint(id)
	}
	if action == "edit" && identity.ID == 0 {
		return nil, fmt.Errorf("sing-box %s id is required for edit", entity)
	}
	if action == "new" {
		// New/import payloads may be copies of an existing object. Never let a
		// client-supplied primary key select or overwrite a database row; the
		// server allocates the id and the persisted raw payload omits it.
		identity.ID = 0
		delete(fields, "id")
	}

	return identity, nil
}

func requiredSingboxPayloadString(fields map[string]json.RawMessage, key string, entity string) (string, error) {
	raw, exists := fields[key]
	if !exists || isSingboxJSONNull(raw) {
		return "", fmt.Errorf("sing-box %s %s is required", entity, key)
	}
	var value string
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", fmt.Errorf("sing-box %s %s must be a string", entity, key)
	}
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("sing-box %s %s is required", entity, key)
	}
	return value, nil
}

func isSingboxJSONNull(raw json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}

func parseStrictSingboxUnsigned(raw json.RawMessage, min uint64, max uint64) (uint64, error) {
	value := bytes.TrimSpace(raw)
	if len(value) == 0 || bytes.Equal(value, []byte("null")) {
		return 0, fmt.Errorf("must be a complete decimal integer")
	}

	text := string(value)
	if value[0] == '"' {
		var quoted string
		if err := json.Unmarshal(value, &quoted); err != nil {
			return 0, fmt.Errorf("must be a complete decimal integer")
		}
		text = strings.TrimSpace(quoted)
	}
	if text == "" {
		return 0, fmt.Errorf("must be a complete decimal integer")
	}
	for _, char := range text {
		if char < '0' || char > '9' {
			return 0, fmt.Errorf("must be a complete decimal integer")
		}
	}

	parsed, err := strconv.ParseUint(text, 10, 64)
	if err != nil || parsed < min || parsed > max {
		return 0, fmt.Errorf("must be an integer from %d to %d", min, max)
	}
	return parsed, nil
}

func parseStrictSingboxPort(raw json.RawMessage) (int, error) {
	parsed, err := parseStrictSingboxUnsigned(raw, 1, 65535)
	if err != nil {
		return 0, err
	}
	return int(parsed), nil
}

func normalizeOptionalSingboxPayloadUnsigned(fields map[string]json.RawMessage, key string, min uint64, max uint64) (uint64, bool, error) {
	raw, exists := fields[key]
	if !exists || isSingboxJSONNull(raw) {
		delete(fields, key)
		return 0, false, nil
	}

	value, err := parseStrictSingboxUnsigned(raw, min, max)
	if err != nil {
		return 0, true, fmt.Errorf("%s: %w", key, err)
	}
	fields[key] = json.RawMessage(strconv.FormatUint(value, 10))
	return value, true, nil
}

func normalizeSingboxPayloadIdentity(payload *singboxEntityPayload) error {
	if payload == nil {
		return fmt.Errorf("sing-box payload is required")
	}

	typeValue, err := json.Marshal(payload.Type)
	if err != nil {
		return err
	}
	tagValue, err := json.Marshal(payload.Tag)
	if err != nil {
		return err
	}
	payload.Fields["type"] = typeValue
	payload.Fields["tag"] = tagValue
	if rawID, exists := payload.Fields["id"]; exists && !isSingboxJSONNull(rawID) {
		payload.Fields["id"] = json.RawMessage(strconv.FormatUint(uint64(payload.ID), 10))
	}
	return nil
}

type singboxIntegerOptionRange struct {
	min uint64
	max uint64
}

var singboxVisibleIntegerOptions = map[string]singboxIntegerOptionRange{
	"alter_id":                      {min: 0, max: maxSingboxJSONSafeInteger},
	"connection_receive_window":     {min: 0, max: maxSingboxJSONSafeInteger},
	"cwnd":                          {min: 0, max: maxSingboxJSONSafeInteger},
	"down_mbps":                     {min: 0, max: maxSingboxJSONSafeInteger},
	"handshake_timeout":             {min: 1, max: maxSingboxJSONSafeInteger},
	"insecure_concurrency":          {min: 0, max: maxSingboxJSONSafeInteger},
	"max_concurrent_streams":        {min: 0, max: maxSingboxJSONSafeInteger},
	"max_connections":               {min: 0, max: maxSingboxJSONSafeInteger},
	"max_datagram_frame_size":       {min: 0, max: maxSingboxJSONSafeInteger},
	"max_early_data":                {min: 0, max: maxSingboxJSONSafeInteger},
	"max_open_streams":              {min: 0, max: maxSingboxJSONSafeInteger},
	"max_streams":                   {min: 0, max: maxSingboxJSONSafeInteger},
	"max_udp_relay_packet_size":     {min: 0, max: maxSingboxJSONSafeInteger},
	"min_idle_session":              {min: 0, max: maxSingboxJSONSafeInteger},
	"min_streams":                   {min: 0, max: maxSingboxJSONSafeInteger},
	"mtu":                           {min: 0, max: 65535},
	"padding_max":                   {min: 0, max: maxSingboxJSONSafeInteger},
	"padding_min":                   {min: 0, max: maxSingboxJSONSafeInteger},
	"ping_interval":                 {min: 0, max: maxSingboxJSONSafeInteger},
	"persistent_keepalive_interval": {min: 0, max: 65535},
	"recv_window":                   {min: 0, max: maxSingboxJSONSafeInteger},
	"recv_window_conn":              {min: 0, max: maxSingboxJSONSafeInteger},
	"sc_max_each_post_bytes":        {min: 1, max: maxSingboxJSONSafeInteger},
	"server_port":                   {min: 1, max: 65535},
	"server_down_mbps":              {min: 0, max: maxSingboxJSONSafeInteger},
	"server_up_mbps":                {min: 0, max: maxSingboxJSONSafeInteger},
	"status_code":                   {min: 100, max: 599},
	"stream_receive_window":         {min: 0, max: maxSingboxJSONSafeInteger},
	"tolerance":                     {min: 0, max: maxSingboxJSONSafeInteger},
	"udp_over_stream_version":       {min: 0, max: maxSingboxJSONSafeInteger},
	"up_mbps":                       {min: 0, max: maxSingboxJSONSafeInteger},
	"workers":                       {min: 1, max: maxSingboxJSONSafeInteger},
}

// normalizeSingboxVisibleIntegerOptions walks only object/array containers and
// touches the panel's documented integer fields. Unknown extension data stays
// byte-for-byte semantic JSON, while decimals and unsafe values cannot be
// silently converted by model.UnmarshalJSON's generic float64 path.
func normalizeSingboxVisibleIntegerOptions(fields map[string]json.RawMessage, path string) error {
	return normalizeSingboxIntegerOptions(fields, path, singboxVisibleIntegerOptions)
}

func normalizeSingboxListenerPortOptions(fields map[string]json.RawMessage, path string) error {
	return normalizeSingboxIntegerOptions(fields, path, map[string]singboxIntegerOptionRange{
		"listen_port": {min: 1, max: 65535},
	})
}

func normalizeSingboxIntegerOptions(fields map[string]json.RawMessage, path string, integerOptions map[string]singboxIntegerOptionRange) error {
	for key, raw := range fields {
		fieldPath := key
		if path != "" {
			fieldPath = path + "." + key
		}
		if limits, isInteger := integerOptions[key]; isInteger {
			if _, _, err := normalizeOptionalSingboxPayloadUnsigned(fields, key, limits.min, limits.max); err != nil {
				return fmt.Errorf("%s: %w", fieldPath, err)
			}
			raw = fields[key]
		}

		trimmed := bytes.TrimSpace(raw)
		if len(trimmed) == 0 || isSingboxJSONNull(trimmed) {
			continue
		}
		switch trimmed[0] {
		case '{':
			child := map[string]json.RawMessage{}
			if err := json.Unmarshal(trimmed, &child); err != nil {
				return fmt.Errorf("%s: must be a JSON object: %w", fieldPath, err)
			}
			if err := normalizeSingboxIntegerOptions(child, fieldPath, integerOptions); err != nil {
				return err
			}
			normalized, err := json.Marshal(child)
			if err != nil {
				return err
			}
			fields[key] = normalized
		case '[':
			children := []json.RawMessage{}
			if err := json.Unmarshal(trimmed, &children); err != nil {
				return fmt.Errorf("%s: must be a JSON array: %w", fieldPath, err)
			}
			changed := false
			for index, childRaw := range children {
				childTrimmed := bytes.TrimSpace(childRaw)
				if len(childTrimmed) == 0 || childTrimmed[0] != '{' {
					continue
				}
				child := map[string]json.RawMessage{}
				if err := json.Unmarshal(childTrimmed, &child); err != nil {
					return fmt.Errorf("%s[%d]: must be a JSON object: %w", fieldPath, index, err)
				}
				if err := normalizeSingboxIntegerOptions(child, fmt.Sprintf("%s[%d]", fieldPath, index), integerOptions); err != nil {
					return err
				}
				normalized, err := json.Marshal(child)
				if err != nil {
					return err
				}
				children[index] = normalized
				changed = true
			}
			if changed {
				normalized, err := json.Marshal(children)
				if err != nil {
					return err
				}
				fields[key] = normalized
			}
		}
	}
	return nil
}

func normalizeSingboxReservedBytes(fields map[string]json.RawMessage) error {
	raw, exists := fields["reserved"]
	if !exists || isSingboxJSONNull(raw) {
		delete(fields, "reserved")
		return nil
	}

	items := []json.RawMessage{}
	if err := json.Unmarshal(raw, &items); err != nil {
		return fmt.Errorf("reserved must be an array of exactly three bytes")
	}
	if len(items) != 3 {
		return fmt.Errorf("reserved must contain exactly three bytes")
	}
	values := make([]uint64, len(items))
	for index, item := range items {
		value, err := parseStrictSingboxUnsigned(item, 0, 255)
		if err != nil {
			return fmt.Errorf("reserved[%d]: %w", index, err)
		}
		values[index] = value
	}
	normalized, err := json.Marshal(values)
	if err != nil {
		return err
	}
	fields["reserved"] = normalized
	return nil
}

func normalizeSingboxWireguardPeers(fields map[string]json.RawMessage) error {
	raw, exists := fields["peers"]
	if !exists || isSingboxJSONNull(raw) {
		delete(fields, "peers")
		return nil
	}

	peers := []json.RawMessage{}
	if err := json.Unmarshal(raw, &peers); err != nil {
		return fmt.Errorf("peers must be an array")
	}
	for index, rawPeer := range peers {
		peer := map[string]json.RawMessage{}
		if err := json.Unmarshal(rawPeer, &peer); err != nil || peer == nil {
			if err != nil {
				return fmt.Errorf("peers[%d]: must be an object: %w", index, err)
			}
			return fmt.Errorf("peers[%d]: must be an object", index)
		}
		if _, _, err := normalizeOptionalSingboxPayloadUnsigned(peer, "port", 1, 65535); err != nil {
			return fmt.Errorf("peers[%d]: %w", index, err)
		}
		if _, _, err := normalizeOptionalSingboxPayloadUnsigned(peer, "persistent_keepalive_interval", 0, 65535); err != nil {
			return fmt.Errorf("peers[%d]: %w", index, err)
		}
		if err := normalizeSingboxReservedBytes(peer); err != nil {
			return fmt.Errorf("peers[%d]: %w", index, err)
		}
		normalized, err := json.Marshal(peer)
		if err != nil {
			return err
		}
		peers[index] = normalized
	}

	normalized, err := json.Marshal(peers)
	if err != nil {
		return err
	}
	fields["peers"] = normalized
	return nil
}

func normalizeSingboxWireguardPayload(fields map[string]json.RawMessage) error {
	if err := normalizeSingboxReservedBytes(fields); err != nil {
		return err
	}
	if err := normalizeSingboxWireguardPeers(fields); err != nil {
		return err
	}
	return nil
}

func marshalNormalizedSingboxPayload(payload *singboxEntityPayload) (json.RawMessage, error) {
	if err := normalizeSingboxPayloadIdentity(payload); err != nil {
		return nil, err
	}
	normalized, err := json.Marshal(payload.Fields)
	if err != nil {
		return nil, err
	}
	return normalized, nil
}

func validateAndNormalizeSingboxEndpointPayload(data json.RawMessage, action string) (json.RawMessage, *singboxEntityPayload, error) {
	payload, err := parseSingboxEntityPayload(data, action, "endpoint")
	if err != nil {
		return nil, nil, err
	}
	if err := normalizeSingboxVisibleIntegerOptions(payload.Fields, ""); err != nil {
		return nil, nil, fmt.Errorf("sing-box endpoint %w", err)
	}
	if payload.Type == "wireguard" || payload.Type == "warp" {
		if _, _, err := normalizeOptionalSingboxPayloadUnsigned(payload.Fields, "listen_port", 0, 65535); err != nil {
			return nil, nil, fmt.Errorf("sing-box endpoint %w", err)
		}
		if err := normalizeSingboxWireguardPayload(payload.Fields); err != nil {
			return nil, nil, fmt.Errorf("sing-box endpoint %w", err)
		}
	}
	normalized, err := marshalNormalizedSingboxPayload(payload)
	if err != nil {
		return nil, nil, err
	}
	return normalized, payload, nil
}

func validateAndNormalizeSingboxServicePayload(data json.RawMessage, action string) (json.RawMessage, *singboxEntityPayload, error) {
	payload, err := parseSingboxEntityPayload(data, action, "service")
	if err != nil {
		return nil, nil, err
	}
	if err := normalizeSingboxVisibleIntegerOptions(payload.Fields, ""); err != nil {
		return nil, nil, fmt.Errorf("sing-box service %w", err)
	}
	if _, exists, err := normalizeOptionalSingboxPayloadUnsigned(payload.Fields, "listen_port", 1, 65535); err != nil {
		return nil, nil, fmt.Errorf("sing-box service %w", err)
	} else if !exists {
		return nil, nil, fmt.Errorf("sing-box service listen_port is required")
	}
	if err := normalizeSingboxListenerPortOptions(payload.Fields, ""); err != nil {
		return nil, nil, fmt.Errorf("sing-box service %w", err)
	}
	normalized, err := marshalNormalizedSingboxPayload(payload)
	if err != nil {
		return nil, nil, err
	}
	return normalized, payload, nil
}

func validateAndNormalizeSingboxOutboundPayload(data json.RawMessage, action string) (json.RawMessage, *singboxEntityPayload, error) {
	payload, err := parseSingboxEntityPayload(data, action, "outbound")
	if err != nil {
		return nil, nil, err
	}
	if err := normalizeSingboxVisibleIntegerOptions(payload.Fields, ""); err != nil {
		return nil, nil, fmt.Errorf("sing-box outbound %w", err)
	}
	if payload.Type == "wireguard" {
		if err := normalizeSingboxWireguardPayload(payload.Fields); err != nil {
			return nil, nil, fmt.Errorf("sing-box outbound %w", err)
		}
	}
	normalized, err := marshalNormalizedSingboxPayload(payload)
	if err != nil {
		return nil, nil, err
	}
	return normalized, payload, nil
}

func parseOptionalSingboxTLSID(fields map[string]json.RawMessage) (uint, error) {
	raw, exists := fields["tls_id"]
	if !exists || isSingboxJSONNull(raw) {
		return 0, nil
	}
	parsed, err := parseStrictSingboxUnsigned(raw, 0, uint64(^uint(0)))
	if err != nil {
		return 0, fmt.Errorf("tls_id: %w", err)
	}
	return uint(parsed), nil
}

func normalizeSingboxInboundIntegerOptions(inbound *model.Inbound, values map[string]int, removeKeys ...string) error {
	if inbound == nil {
		return fmt.Errorf("sing-box inbound is required")
	}
	options := map[string]interface{}{}
	if len(inbound.Options) > 0 && !isSingboxJSONNull(inbound.Options) {
		if err := json.Unmarshal(inbound.Options, &options); err != nil || options == nil {
			if err != nil {
				return fmt.Errorf("decode sing-box inbound options: %w", err)
			}
			options = map[string]interface{}{}
		}
	}
	for key, value := range values {
		options[key] = value
	}
	for _, key := range removeKeys {
		delete(options, key)
	}
	encoded, err := json.MarshalIndent(options, "", "  ")
	if err != nil {
		return err
	}
	inbound.Options = encoded
	return nil
}

func validateSingboxInboundPayload(data json.RawMessage, inbound *model.Inbound, action string) error {
	if inbound == nil {
		return fmt.Errorf("sing-box inbound is required")
	}

	payload, err := parseSingboxEntityPayload(data, action, "inbound")
	if err != nil {
		return err
	}
	if err := normalizeSingboxVisibleIntegerOptions(payload.Fields, ""); err != nil {
		return fmt.Errorf("sing-box inbound %w", err)
	}
	if payload.Type == "tun" {
		if _, _, err := normalizeOptionalSingboxPayloadUnsigned(payload.Fields, "mtu", 576, 65535); err != nil {
			return fmt.Errorf("sing-box tun %w", err)
		}
	}
	normalizedData, err := marshalNormalizedSingboxPayload(payload)
	if err != nil {
		return err
	}
	if err := inbound.UnmarshalJSON(normalizedData); err != nil {
		return err
	}
	inbound.Id = payload.ID
	inbound.Type = payload.Type
	inbound.Tag = payload.Tag

	tlsID, err := parseOptionalSingboxTLSID(payload.Fields)
	if err != nil {
		return fmt.Errorf("sing-box inbound %w", err)
	}
	inbound.TlsId = tlsID

	values := map[string]int{}
	removeKeys := make([]string, 0, 1)
	if inbound.Type != "tun" {
		rawPort, exists := payload.Fields["listen_port"]
		if !exists || isSingboxJSONNull(rawPort) {
			return fmt.Errorf("sing-box %s listen_port is required", inbound.Type)
		}
		port, parseErr := parseStrictSingboxPort(rawPort)
		if parseErr != nil {
			return fmt.Errorf("sing-box %s listen_port: %w", inbound.Type, parseErr)
		}
		values["listen_port"] = port
	}
	if rawOverridePort, exists := payload.Fields["override_port"]; exists {
		if isSingboxJSONNull(rawOverridePort) {
			removeKeys = append(removeKeys, "override_port")
		} else {
			port, parseErr := parseStrictSingboxPort(rawOverridePort)
			if parseErr != nil {
				return fmt.Errorf("sing-box %s override_port: %w", inbound.Type, parseErr)
			}
			values["override_port"] = port
		}
	}
	if err := normalizeSingboxInboundIntegerOptions(inbound, values, removeKeys...); err != nil {
		return err
	}

	if singboxInboundRequiresTLS(inbound.Type) && inbound.TlsId == 0 {
		return fmt.Errorf("sing-box %s inbound requires a TLS configuration", inbound.Type)
	}
	return nil
}

func singboxInboundRequiresTLS(inboundType string) bool {
	switch strings.ToLower(strings.TrimSpace(inboundType)) {
	case "anytls", "hysteria", "hysteria2", "naive", "trusttunnel", "tuic":
		return true
	default:
		return false
	}
}
