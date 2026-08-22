package service

import (
	"encoding/json"
	"strings"

	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/util"
)

func fillMihomoOutJson(inbound *model.MihomoInbound, hostname string) error {
	if inbound == nil {
		return nil
	}
	base := inbound.ToBase()
	if err := util.FillOutJson(&base, hostname); err != nil {
		return err
	}
	inbound.OutJson = base.OutJson
	return sanitizeMihomoClientCommonFields(inbound)
}

// sanitizeMihomoClientCommonFields keeps the supported client-template common
// fields canonical. They are subscription-only and may live at the outbound
// root or inside a nested protocol template store.
func sanitizeMihomoClientCommonFields(inbound *model.MihomoInbound) error {
	if inbound == nil || len(inbound.OutJson) == 0 {
		return nil
	}

	var outbound map[string]interface{}
	if err := json.Unmarshal(inbound.OutJson, &outbound); err != nil {
		return err
	}
	if outbound == nil {
		return nil
	}
	stripUnsupportedMihomoFastOpenFields(outbound, inbound.Type)
	if strings.EqualFold(strings.TrimSpace(inbound.Type), "hysteria2") {
		util.EnsureHysteria2MihomoReceiveWindows(outbound)
		sanitizeMihomoHysteria2ClientReceiveWindows(outbound)
	} else {
		delete(outbound, "mihomo_hy2")
	}
	stripUnsupportedMihomoTLSFields(outbound)
	sanitizeMihomoGRPCNumericFields(outbound)
	sanitizeMihomoMultiplexFields(outbound, "multiplex")

	sanitizeStore := func(root map[string]interface{}) {
		common, ok := root["mihomo_common"].(map[string]interface{})
		if !ok || common == nil {
			return
		}
		if _, exists := common["smux"].(map[string]interface{}); !exists {
			if legacyMux, ok := common["mux"].(map[string]interface{}); ok && legacyMux != nil {
				common["smux"] = legacyMux
			}
		}
		delete(common, "mux")
		if value, exists := common["udp"]; exists {
			if _, ok := value.(bool); !ok {
				delete(common, "udp")
			}
		}
		if value, exists := common["ip_version"]; exists {
			if normalized, ok := util.NormalizeMihomoClientIPVersion(value); ok {
				common["ip_version"] = normalized
			} else {
				delete(common, "ip_version")
			}
		}
		if value, exists := common["routing_mark"]; exists {
			if normalized, ok := toMihomoStrictInt(value); ok && normalized >= 0 {
				common["routing_mark"] = normalized
			} else {
				delete(common, "routing_mark")
			}
		}
		for _, key := range []string{"tcp_fast_open", "tcp_multi_path"} {
			if value, exists := common[key]; exists {
				if _, ok := value.(bool); !ok {
					delete(common, key)
				}
			}
		}
		if util.SupportsMihomoBBRProfileProtocol(inbound.Type) {
			if profile, ok := util.NormalizeMihomoBBRProfile(common["bbr_profile"]); ok {
				common["bbr_profile"] = profile
			} else {
				delete(common, "bbr_profile")
			}
		} else {
			delete(common, "bbr_profile")
		}
		delete(common, "bbr-profile")
		sanitizeMihomoSMuxNumericFields(common)
		if len(common) == 0 {
			delete(root, "mihomo_common")
		}
	}

	sanitizeStore(outbound)
	if ssConfig, ok := outbound["ss_config"].(map[string]interface{}); ok && ssConfig != nil {
		sanitizeMihomoMultiplexFields(ssConfig, "multiplex")
		sanitizeStore(ssConfig)
	}

	normalized, err := json.MarshalIndent(outbound, "", "  ")
	if err != nil {
		return err
	}
	inbound.OutJson = normalized
	return nil
}

func sanitizeMihomoSMuxNumericFields(common map[string]interface{}) {
	sanitizeMihomoMultiplexFields(common, "smux")
}

func sanitizeMihomoMultiplexFields(root map[string]interface{}, key string) {
	if root == nil || key == "" {
		return
	}
	rawMux, exists := root[key]
	if !exists {
		return
	}
	mux, ok := rawMux.(map[string]interface{})
	if !ok || mux == nil {
		delete(root, key)
		return
	}

	if value, exists := mux["enabled"]; exists {
		if normalized, ok := value.(bool); ok {
			if !normalized {
				delete(root, key)
				return
			}
			mux["enabled"] = normalized
		} else {
			delete(mux, "enabled")
		}
	}
	if value, exists := mux["protocol"]; exists {
		protocol, ok := value.(string)
		protocol = strings.ToLower(strings.TrimSpace(protocol))
		if !ok || (protocol != "smux" && protocol != "yamux" && protocol != "h2mux") {
			delete(mux, "protocol")
		} else {
			mux["protocol"] = protocol
		}
	}
	for _, key := range []string{"max_connections", "min_streams", "max_streams"} {
		if value, exists := mux[key]; exists {
			if normalized, ok := toMihomoStrictInt(value); ok && normalized >= 0 {
				mux[key] = normalized
			} else {
				delete(mux, key)
			}
		}
	}
	for _, key := range []string{"statistic", "only_tcp", "padding"} {
		if value, exists := mux[key]; exists {
			if normalized, ok := value.(bool); ok {
				mux[key] = normalized
			} else {
				delete(mux, key)
			}
		}
	}
	if value, exists := mux["only-tcp"]; exists {
		if _, normalizedExists := mux["only_tcp"]; !normalizedExists {
			if normalized, ok := value.(bool); ok {
				mux["only_tcp"] = normalized
			}
		}
		delete(mux, "only-tcp")
	}
	rawBrutal, exists := mux["brutal"]
	if exists {
		brutal, ok := rawBrutal.(map[string]interface{})
		if !ok || brutal == nil {
			delete(mux, "brutal")
		} else {
			if enabled, ok := brutal["enabled"].(bool); !ok || !enabled {
				delete(mux, "brutal")
			} else {
				brutal["enabled"] = true
				for _, key := range []string{"up_mbps", "down_mbps"} {
					if value, exists := brutal[key]; exists {
						if normalized, ok := toMihomoStrictInt(value); ok && normalized >= 0 {
							brutal[key] = normalized
						} else {
							delete(brutal, key)
						}
					}
				}
			}
		}
	}
	if _, exists := mux["enabled"]; !exists && len(mux) > 0 {
		mux["enabled"] = true
	}
	if len(mux) == 0 {
		delete(root, key)
	}
}

func sanitizeMihomoGRPCNumericFields(outbound map[string]interface{}) {
	if outbound == nil {
		return
	}

	transport, ok := outbound["transport"].(map[string]interface{})
	if !ok || transport == nil {
		return
	}

	grpcKeys := []string{"grpc_user_agent", "ping_interval", "max_connections", "min_streams", "max_streams"}
	transportType, _ := transport["type"].(string)
	if !strings.EqualFold(strings.TrimSpace(transportType), "grpc") {
		for _, key := range grpcKeys {
			delete(transport, key)
		}
		return
	}

	for _, key := range []string{"ping_interval", "max_connections"} {
		if value, exists := transport[key]; exists {
			if normalized, ok := toMihomoStrictInt(value); ok && normalized > 0 {
				transport[key] = normalized
			} else {
				delete(transport, key)
			}
		}
	}
	for _, key := range []string{"min_streams", "max_streams"} {
		if value, exists := transport[key]; exists {
			if normalized, ok := toMihomoStrictInt(value); ok && normalized >= 0 {
				transport[key] = normalized
			} else {
				delete(transport, key)
			}
		}
	}
}

// sanitizeMihomoHysteria2ClientReceiveWindows limits the panel-only storage
// block to the four integer values that Mihomo can project into a proxy or
// listener. This also repairs historic API/SQLite values before they reach a
// generated configuration.
func sanitizeMihomoHysteria2ClientReceiveWindows(outbound map[string]interface{}) {
	if outbound == nil {
		return
	}

	raw, exists := outbound["mihomo_hy2"]
	if !exists {
		return
	}
	options, ok := raw.(map[string]interface{})
	if !ok || options == nil {
		delete(outbound, "mihomo_hy2")
		return
	}

	clean := make(map[string]interface{}, 4)
	for _, key := range []string{
		"initial_stream_receive_window",
		"max_stream_receive_window",
		"initial_connection_receive_window",
		"max_connection_receive_window",
	} {
		if value, ok := toMihomoStrictInt(options[key]); ok && value > 0 {
			clean[key] = value
		}
	}
	if len(clean) == 0 {
		delete(outbound, "mihomo_hy2")
		return
	}
	outbound["mihomo_hy2"] = clean
}

func sanitizeMihomoOutboundRoutingMark(outbound map[string]interface{}) {
	if outbound == nil {
		return
	}
	value, exists := outbound["routing_mark"]
	if !exists {
		return
	}
	if normalized, ok := toMihomoStrictInt(value); ok && normalized >= 0 {
		outbound["routing_mark"] = normalized
		return
	}
	delete(outbound, "routing_mark")
}

// toMihomoStrictInt accepts concrete numeric JSON values only. The panel's
// number controls submit numbers, so textual numbers are treated as stale or
// malformed data rather than silently gaining a different type on save.
func toMihomoStrictInt(value interface{}) (int, bool) {
	if _, isString := value.(string); isString {
		return 0, false
	}
	return toInt(value)
}

// stripUnsupportedMihomoFastOpenFields removes the protocol-template setting
// only for protocols whose Mihomo proxy schema does not support it.
func stripUnsupportedMihomoFastOpenFields(outbound map[string]interface{}, outboundType string) {
	switch strings.ToLower(strings.TrimSpace(outboundType)) {
	case "tuic":
		delete(outbound, "mihomo_fast_open")
		delete(outbound, "fast_open")
		delete(outbound, "fast-open")
	case "hysteria", "hysteria2":
		if _, exists := outbound["mihomo_fast_open"]; !exists {
			if value, ok := outbound["fast_open"].(bool); ok {
				outbound["mihomo_fast_open"] = value
			} else if value, ok := outbound["fast-open"].(bool); ok {
				outbound["mihomo_fast_open"] = value
			}
		}
		delete(outbound, "fast_open")
		delete(outbound, "fast-open")
	}
}

func stripUnsupportedMihomoTLSFields(outbound map[string]interface{}) {
	if outbound == nil {
		return
	}
	tls, ok := outbound["tls"].(map[string]interface{})
	if !ok || tls == nil {
		return
	}
	delete(tls, "fragment")
	delete(tls, "fragment_fallback_delay")
	delete(tls, "record_fragment")
}

func mihomoInboundSliceToBase(inbounds []model.MihomoInbound) []model.Inbound {
	result := make([]model.Inbound, 0, len(inbounds))
	for _, inbound := range inbounds {
		result = append(result, inbound.ToBase())
	}
	return result
}

func mihomoInboundPtrsToBase(inbounds []*model.MihomoInbound) []*model.Inbound {
	result := make([]*model.Inbound, 0, len(inbounds))
	for _, inbound := range inbounds {
		if inbound == nil {
			continue
		}
		base := inbound.ToBase()
		result = append(result, &base)
	}
	return result
}
