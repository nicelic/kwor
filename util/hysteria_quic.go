package util

import (
	"encoding/json"

	"github.com/alireza0/s-ui/database/model"
)

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
