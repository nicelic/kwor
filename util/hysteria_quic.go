package util

import (
	"encoding/json"
	"strings"

	"github.com/alireza0/s-ui/database/model"
)

var hysteria2ReceiveWindowKeys = [...]string{
	"initial_stream_receive_window",
	"max_stream_receive_window",
	"initial_connection_receive_window",
	"max_connection_receive_window",
}

func NormalizeHysteriaSubscriptionOutbound(outbound map[string]interface{}) {
	if outbound == nil {
		return
	}

	moveHysteriaOutboundLegacyField(outbound, "stream_receive_window", "recv_window_conn")
	moveHysteriaOutboundLegacyField(outbound, "connection_receive_window", "recv_window")
	moveHysteriaOutboundLegacyField(outbound, "disable_path_mtu_discovery", "disable_mtu_discovery")
}

func NormalizeHysteriaOutboundOptionsMap(outbound map[string]interface{}) {
	NormalizeHysteriaSubscriptionOutbound(outbound)
}

// StripMihomoHysteria2ReceiveWindowsForSingbox removes the four quic-go
// receive-window fields reserved for Mihomo/Clash Hysteria2 client proxies.
// The fields may appear in legacy root form or under mihomo_hy2. The wrapper
// itself is removed by the surrounding sing-box sanitizers.
func StripMihomoHysteria2ReceiveWindowsForSingbox(outbound map[string]interface{}) {
	if outbound == nil || !strings.EqualFold(strings.TrimSpace(firstStringValue(outbound["type"])), "hysteria2") {
		return
	}
	for _, key := range hysteria2ReceiveWindowKeys {
		delete(outbound, key)
	}
}

// EnsureHysteria2MihomoReceiveWindows restores the legacy Mihomo storage
// wrapper when a payload only contains the shared root fields. This keeps
// older Mihomo editors and Clash conversion compatible with sing-box-created
// nodes.
func EnsureHysteria2MihomoReceiveWindows(outbound map[string]interface{}) {
	if outbound == nil || !strings.EqualFold(strings.TrimSpace(firstStringValue(outbound["type"])), "hysteria2") {
		return
	}
	nested, _ := outbound["mihomo_hy2"].(map[string]interface{})
	if nested == nil {
		nested = map[string]interface{}{}
	}
	for _, key := range hysteria2ReceiveWindowKeys {
		if _, exists := nested[key]; exists {
			if _, valid := positiveIntFromAny(nested[key]); valid {
				continue
			}
		}
		if value, ok := positiveIntFromAny(outbound[key]); ok {
			nested[key] = value
		}
	}
	if len(nested) > 0 {
		outbound["mihomo_hy2"] = nested
	}
}

func Hysteria2ReceiveWindowValue(outbound map[string]interface{}, key string) (int, bool) {
	if outbound == nil {
		return 0, false
	}
	if value, ok := positiveIntFromAny(outbound[key]); ok {
		return value, true
	}
	nested, _ := outbound["mihomo_hy2"].(map[string]interface{})
	if nested == nil {
		return 0, false
	}
	return positiveIntFromAny(nested[key])
}

func firstStringValue(value interface{}) string {
	switch v := value.(type) {
	case string:
		return v
	case []string:
		if len(v) > 0 {
			return v[0]
		}
	case []interface{}:
		if len(v) > 0 {
			if s, ok := v[0].(string); ok {
				return s
			}
		}
	}
	return ""
}

func ApplyHysteriaInboundQUICToOutbound(outbound map[string]interface{}, inboundOptions json.RawMessage) {
	if outbound == nil {
		return
	}

	NormalizeHysteriaSubscriptionOutbound(outbound)

	if len(inboundOptions) == 0 {
		return
	}

	var options map[string]interface{}
	if err := json.Unmarshal(inboundOptions, &options); err != nil {
		return
	}

	model.NormalizeHysteriaInboundOptionsMap(options)
	syncHysteriaOutboundField(outbound, options, "stream_receive_window")
	syncHysteriaOutboundField(outbound, options, "connection_receive_window")
	syncHysteriaOutboundField(outbound, options, "max_concurrent_streams")
	syncHysteriaOutboundField(outbound, options, "disable_path_mtu_discovery")
}

func moveHysteriaOutboundLegacyField(outbound map[string]interface{}, newKey string, oldKey string) {
	if _, exists := outbound[newKey]; !exists {
		if value, ok := outbound[oldKey]; ok {
			outbound[newKey] = value
		}
	}
	delete(outbound, oldKey)
}

func syncHysteriaOutboundField(outbound map[string]interface{}, inbound map[string]interface{}, key string) {
	if value, ok := inbound[key]; ok {
		outbound[key] = value
		return
	}
	delete(outbound, key)
}
