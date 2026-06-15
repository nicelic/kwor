package model

import (
	"encoding/json"
	"strconv"
	"strings"
)

const (
	defaultHysteriaServerBandwidthMbps  = 2000
	fallbackHysteriaServerBandwidthMbps = 10000
)

var inboundViewOnlyFields = map[string]struct{}{
	"route_tag":       {},
	"user_management": {},
}

var inboundRuntimeExcludedFields = map[string]struct{}{
	"port_hop_range":                     {},
	"port_hop_interval":                  {},
	"port_hop_interval_max":              {},
	"port_range":                         {},
	"naive_quic_congestion_control_omit": {},
}

func NormalizeHysteriaInboundOptionsMap(options map[string]interface{}) {
	if options == nil {
		return
	}

	moveInboundLegacyField(options, "stream_receive_window", "recv_window_conn")
	moveInboundLegacyField(options, "connection_receive_window", "recv_window_client")
	moveInboundLegacyField(options, "max_concurrent_streams", "max_conn_client")
	moveInboundLegacyField(options, "disable_path_mtu_discovery", "disable_mtu_discovery")
	moveInboundLegacyField(options, "server_up_mbps", "up_mbps")
	moveInboundLegacyField(options, "server_down_mbps", "down_mbps")
}

func normalizeInboundOptionsMap(options map[string]interface{}, inboundType string) {
	if options == nil {
		return
	}

	if inboundType == "hysteria" {
		NormalizeHysteriaInboundOptionsMap(options)
	}
	sanitizeHysteriaServerBandwidthFields(options, inboundType)
}

func moveInboundLegacyField(options map[string]interface{}, newKey string, oldKey string) {
	if _, exists := options[newKey]; !exists {
		if value, ok := options[oldKey]; ok {
			options[newKey] = value
		}
	}
	delete(options, oldKey)
}

func sanitizeOptionalPositiveIntField(options map[string]interface{}, inboundType string, key string) {
	if inboundType != "hysteria" && inboundType != "hysteria2" {
		return
	}

	value, exists := options[key]
	if !exists {
		return
	}

	if normalized, ok := normalizePositiveIntValue(value); ok {
		options[key] = normalized
		return
	}

	delete(options, key)
}

func sanitizeHysteriaServerBandwidthFields(options map[string]interface{}, inboundType string) {
	switch inboundType {
	case "hysteria":
		options["server_up_mbps"] = resolveHysteriaStoredBandwidthField(options, "server_up_mbps", "up_mbps")
		options["server_down_mbps"] = resolveHysteriaStoredBandwidthField(options, "server_down_mbps", "down_mbps")
	case "hysteria2":
		if normalized, ok := resolveOptionalStoredBandwidthField(options, "server_up_mbps", "up_mbps"); ok {
			options["server_up_mbps"] = normalized
		} else {
			delete(options, "server_up_mbps")
		}
		if normalized, ok := resolveOptionalStoredBandwidthField(options, "server_down_mbps", "down_mbps"); ok {
			options["server_down_mbps"] = normalized
		} else {
			delete(options, "server_down_mbps")
		}
	default:
		return
	}

	delete(options, "up_mbps")
	delete(options, "down_mbps")
}

func resolveHysteriaStoredBandwidthField(options map[string]interface{}, primaryKey string, legacyKey string) int {
	if options == nil {
		return defaultHysteriaServerBandwidthMbps
	}

	if value, exists := options[primaryKey]; exists {
		if normalized, ok := normalizeStoredBandwidthValue(value); ok {
			return normalized
		}
	}

	if value, exists := options[legacyKey]; exists {
		if normalized, ok := normalizeStoredBandwidthValue(value); ok {
			return normalized
		}
	}

	return defaultHysteriaServerBandwidthMbps
}

func resolveOptionalStoredBandwidthField(options map[string]interface{}, primaryKey string, legacyKey string) (int, bool) {
	if options == nil {
		return 0, false
	}

	if value, exists := options[primaryKey]; exists {
		if normalized, ok := normalizeStoredBandwidthValue(value); ok {
			return normalized, true
		}
	}

	if value, exists := options[legacyKey]; exists {
		if normalized, ok := normalizeStoredBandwidthValue(value); ok {
			return normalized, true
		}
	}

	return 0, false
}

func normalizeStoredBandwidthValue(value interface{}) (int, bool) {
	switch v := value.(type) {
	case int:
		return v, true
	case int8:
		return int(v), true
	case int16:
		return int(v), true
	case int32:
		return int(v), true
	case int64:
		return int(v), true
	case uint:
		return int(v), true
	case uint8:
		return int(v), true
	case uint16:
		return int(v), true
	case uint32:
		return int(v), true
	case uint64:
		return int(v), true
	case float32:
		return int(v), true
	case float64:
		return int(v), true
	case string:
		text := strings.TrimSpace(v)
		if text == "" {
			return 0, false
		}
		n, err := strconv.Atoi(text)
		if err != nil {
			return 0, false
		}
		return n, true
	default:
		return 0, false
	}
}

func resolveHysteriaServerBandwidthMbps(options map[string]interface{}, key string) int {
	if options == nil {
		return defaultHysteriaServerBandwidthMbps
	}

	if value, exists := options[key]; exists {
		switch v := value.(type) {
		case int:
			if v > 0 {
				return v
			}
			return fallbackHysteriaServerBandwidthMbps
		case int8:
			if v > 0 {
				return int(v)
			}
			return fallbackHysteriaServerBandwidthMbps
		case int16:
			if v > 0 {
				return int(v)
			}
			return fallbackHysteriaServerBandwidthMbps
		case int32:
			if v > 0 {
				return int(v)
			}
			return fallbackHysteriaServerBandwidthMbps
		case int64:
			if v > 0 {
				return int(v)
			}
			return fallbackHysteriaServerBandwidthMbps
		case uint:
			if v > 0 {
				return int(v)
			}
			return fallbackHysteriaServerBandwidthMbps
		case uint8:
			if v > 0 {
				return int(v)
			}
			return fallbackHysteriaServerBandwidthMbps
		case uint16:
			if v > 0 {
				return int(v)
			}
			return fallbackHysteriaServerBandwidthMbps
		case uint32:
			if v > 0 {
				return int(v)
			}
			return fallbackHysteriaServerBandwidthMbps
		case uint64:
			if v > 0 {
				return int(v)
			}
			return fallbackHysteriaServerBandwidthMbps
		case float32:
			if v > 0 {
				return int(v)
			}
			return fallbackHysteriaServerBandwidthMbps
		case float64:
			if v > 0 {
				return int(v)
			}
			return fallbackHysteriaServerBandwidthMbps
		case string:
			text := strings.TrimSpace(v)
			if text == "" {
				return defaultHysteriaServerBandwidthMbps
			}
			n, err := strconv.Atoi(text)
			if err != nil {
				return defaultHysteriaServerBandwidthMbps
			}
			if n > 0 {
				return n
			}
			return fallbackHysteriaServerBandwidthMbps
		}
	}

	return defaultHysteriaServerBandwidthMbps
}

func resolveOptionalRuntimeBandwidthMbps(options map[string]interface{}, key string) (int, bool) {
	if options == nil {
		return 0, false
	}

	value, exists := options[key]
	if !exists {
		return 0, false
	}

	normalized, ok := normalizeStoredBandwidthValue(value)
	if !ok || normalized <= 0 {
		return 0, false
	}

	return normalized, true
}

func normalizePositiveIntValue(value interface{}) (int, bool) {
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
	case uint:
		return int(v), v > 0
	case uint8:
		return int(v), v > 0
	case uint16:
		return int(v), v > 0
	case uint32:
		return int(v), v > 0
	case uint64:
		return int(v), v > 0
	case float32:
		return int(v), v > 0
	case float64:
		return int(v), v > 0
	case string:
		text := strings.TrimSpace(v)
		if text == "" {
			return 0, false
		}
		n, err := strconv.Atoi(text)
		if err != nil {
			return 0, false
		}
		return n, n > 0
	default:
		return 0, false
	}
}

func hasNonEmptySliceValue(value interface{}) bool {
	switch v := value.(type) {
	case []interface{}:
		return len(v) > 0
	case []string:
		return len(v) > 0
	default:
		return false
	}
}

type Inbound struct {
	Id   uint   `json:"id" form:"id" gorm:"primaryKey;autoIncrement"`
	Type string `json:"type" form:"type"`
	Tag  string `json:"tag" form:"tag" gorm:"unique"`

	// Foreign key to tls table
	TlsId uint `json:"tls_id" form:"tls_id"`
	Tls   *Tls `json:"tls" form:"tls" gorm:"foreignKey:TlsId;references:Id"`

	Addrs   json.RawMessage `json:"addrs" form:"addrs"`
	OutJson json.RawMessage `json:"out_json" form:"out_json"`
	Options json.RawMessage `json:"-" form:"-"`
}

func (i *Inbound) UnmarshalJSON(data []byte) error {
	var err error
	var raw map[string]interface{}
	if err = json.Unmarshal(data, &raw); err != nil {
		return err
	}

	// Extract fixed fields and store the rest in Options
	if val, exists := raw["id"].(float64); exists {
		i.Id = uint(val)
	}
	delete(raw, "id")
	i.Type, _ = raw["type"].(string)
	delete(raw, "type")
	i.Tag, _ = raw["tag"].(string)
	delete(raw, "tag")

	// 开发者要求隐藏并默认关闭 SS API 专用能力，保存时强制 managed=false。
	// Developer requirement: hide and default-disable SS API-only capability; force managed=false on save.
	// 说明 / Note: 不影响常规 SS/SS2022 节点创建与使用 / does not affect regular SS/SS2022 node creation or usage.
	if i.Type == "shadowsocks" {
		raw["managed"] = false
	}

	// TlsId
	if val, exists := raw["tls_id"].(float64); exists {
		i.TlsId = uint(val)
	}
	delete(raw, "tls_id")
	delete(raw, "tls")
	delete(raw, "users")
	for key := range inboundViewOnlyFields {
		delete(raw, key)
	}

	// Addrs
	i.Addrs, _ = json.MarshalIndent(raw["addrs"], "", "  ")
	delete(raw, "addrs")

	// OutJson
	i.OutJson, _ = json.MarshalIndent(raw["out_json"], "", "  ")
	delete(raw, "out_json")

	normalizeInboundOptionsMap(raw, i.Type)

	// Remaining fields
	i.Options, err = json.MarshalIndent(raw, "", "  ")
	return err
}

// MarshalJSON customizes marshalling
func (i Inbound) MarshalJSON() ([]byte, error) {
	// Combine fixed fields and dynamic fields into one map
	combined := make(map[string]interface{})
	combined["type"] = i.Type
	combined["tag"] = i.Tag
	if i.Tls != nil {
		var tlsConfig map[string]interface{}
		if err := json.Unmarshal(i.Tls.Server, &tlsConfig); err == nil {
			if hasNonEmptySliceValue(tlsConfig["client_certificate_public_key_sha256"]) {
				delete(tlsConfig, "client_certificate")
				delete(tlsConfig, "client_certificate_path")
			}
			combined["tls"] = tlsConfig
		} else {
			combined["tls"] = i.Tls.Server
		}
	}

	if i.Options != nil {
		var restFields map[string]interface{}
		if err := json.Unmarshal(i.Options, &restFields); err != nil {
			return nil, err
		}

		// 开发者要求隐藏并默认关闭 SS API 专用能力，运行配置统一 managed=false。
		// Developer requirement: hide and default-disable SS API-only capability; enforce managed=false in runtime config.
		if i.Type == "shadowsocks" {
			restFields["managed"] = false
		}

		normalizeInboundOptionsMap(restFields, i.Type)

		for k, v := range restFields {
			if _, blocked := inboundViewOnlyFields[k]; blocked {
				continue
			}
			if _, blocked := inboundRuntimeExcludedFields[k]; blocked {
				continue
			}
			switch i.Type {
			case "hysteria":
				switch k {
				case "server_up_mbps":
					combined["up_mbps"] = resolveHysteriaServerBandwidthMbps(restFields, "server_up_mbps")
					continue
				case "server_down_mbps":
					combined["down_mbps"] = resolveHysteriaServerBandwidthMbps(restFields, "server_down_mbps")
					continue
				}
			case "hysteria2":
				switch k {
				case "server_up_mbps":
					if upMbps, ok := resolveOptionalRuntimeBandwidthMbps(restFields, "server_up_mbps"); ok {
						combined["up_mbps"] = upMbps
					}
					continue
				case "server_down_mbps":
					if downMbps, ok := resolveOptionalRuntimeBandwidthMbps(restFields, "server_down_mbps"); ok {
						combined["down_mbps"] = downMbps
					}
					continue
				}
			}
			combined[k] = v
		}
	}

	return json.Marshal(combined)
}

func (i Inbound) MarshalFull() (*map[string]interface{}, error) {
	combined := make(map[string]interface{})
	combined["id"] = i.Id
	combined["type"] = i.Type
	combined["tag"] = i.Tag
	combined["tls_id"] = i.TlsId
	combined["addrs"] = i.Addrs
	combined["out_json"] = i.OutJson

	if i.Options != nil {
		var restFields map[string]interface{}
		if err := json.Unmarshal(i.Options, &restFields); err != nil {
			return nil, err
		}

		// 开发者要求隐藏并默认关闭 SS API 专用能力，完整视图统一 managed=false。
		// Developer requirement: hide and default-disable SS API-only capability; enforce managed=false in full view.
		if i.Type == "shadowsocks" {
			restFields["managed"] = false
		}

		normalizeInboundOptionsMap(restFields, i.Type)

		for k, v := range restFields {
			combined[k] = v
		}
	}
	return &combined, nil
}
