package sub

import (
	"encoding/json"
	"math"
	"strings"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/util"
)

func mihomoClientToBase(client *model.MihomoClient) *model.Client {
	if client == nil {
		return nil
	}

	return &model.Client{
		Id:        client.Id,
		Enable:    client.Enable,
		Name:      client.Name,
		Config:    client.Config,
		Inbounds:  client.Inbounds,
		Links:     client.Links,
		Volume:    client.Volume,
		Expiry:    client.Expiry,
		Down:      client.Down,
		Up:        client.Up,
		Desc:      client.Desc,
		Group:     client.Group,
		ServerIp:  client.ServerIp,
		Extra:     client.Extra,
		LastReset: client.LastReset,
	}
}

func loadMihomoClientBySubID(subID string) (*model.Client, error) {
	db := database.GetDB()
	client := &model.MihomoClient{}
	if err := db.Model(model.MihomoClient{}).Where("enable = true and name = ?", subID).First(client).Error; err != nil {
		return nil, err
	}
	return mihomoClientToBase(client), nil
}

func loadMihomoSubscriptionData(subID string) (*model.Client, []*model.Inbound, error) {
	db := database.GetDB()
	mihomoClient := &model.MihomoClient{}
	if err := db.Model(model.MihomoClient{}).Where("enable = true and name = ?", subID).First(mihomoClient).Error; err != nil {
		return nil, nil, err
	}

	inboundIDs, err := util.ParseInboundIDs(mihomoClient.Inbounds)
	if err != nil {
		return nil, nil, err
	}

	mihomoInbounds := make([]*model.MihomoInbound, 0, len(inboundIDs))
	if len(inboundIDs) > 0 {
		if err := db.Model(model.MihomoInbound{}).Preload("Tls").Where("id in ?", inboundIDs).Find(&mihomoInbounds).Error; err != nil {
			return nil, nil, err
		}
	}
	mihomoInbounds = util.OrderMihomoInboundPtrsByIDs(inboundIDs, mihomoInbounds)

	inbounds := make([]*model.Inbound, 0, len(mihomoInbounds))
	for _, mihomoInbound := range mihomoInbounds {
		if mihomoInbound == nil {
			continue
		}
		// The user binding can outlive an old protocol record. Only emit
		// inbounds that Mihomo can still materialize as listeners; proxy-only
		// protocol types such as SSH belong to manually configured outbounds,
		// not a server-side client subscription.
		if !util.SupportsMihomoRuntimeListenerType(mihomoInbound.Type) {
			continue
		}

		baseInbound := mihomoInbound.ToBase()
		if len(baseInbound.OutJson) < 5 {
			if host := resolveMihomoSubscriptionHost(mihomoClient, &baseInbound); host != "" {
				if err := util.FillOutJson(&baseInbound, host); err != nil {
					return nil, nil, err
				}
			}
		}
		if err := normalizeMihomoSubscriptionOutJSON(&baseInbound); err != nil {
			return nil, nil, err
		}

		inbounds = append(inbounds, &baseInbound)
	}

	return mihomoClientToBase(mihomoClient), inbounds, nil
}

func resolveMihomoSubscriptionHost(client *model.MihomoClient, inbound *model.Inbound) string {
	override := ""
	if client != nil {
		override = client.ServerIp
	}
	return util.ResolveSubscriptionServerHost(override, inbound, "")
}

func normalizeMihomoSubscriptionOutJSON(inbound *model.Inbound) error {
	if inbound == nil {
		return nil
	}

	outbound := map[string]interface{}{}
	if len(inbound.OutJson) > 0 {
		if err := json.Unmarshal(inbound.OutJson, &outbound); err != nil {
			return err
		}
	}
	if outbound == nil {
		outbound = map[string]interface{}{}
	}
	util.StripSubscriptionOutboundPanelFields(outbound)

	migrateLegacyMihomoCommonFields(outbound, inbound.Type)
	sanitizeMihomoSubscriptionTLSFields(outbound)
	sanitizeMihomoSubscriptionGRPCFields(outbound)
	sanitizeMihomoSubscriptionReceiveWindows(outbound, inbound.Type)
	if strings.EqualFold(strings.TrimSpace(inbound.Type), "hysteria2") {
		util.EnsureHysteria2MihomoReceiveWindows(outbound)
	}
	sanitizeMihomoSubscriptionCommonFields(outbound)
	if ssConfig, ok := outbound["ss_config"].(map[string]interface{}); ok && ssConfig != nil {
		sanitizeMihomoSubscriptionMultiplexFields(ssConfig, "multiplex")
		sanitizeMihomoSubscriptionCommonFields(ssConfig)
	}
	if inbound.Type == "shadowquic" {
		// ShadowQUIC never uses mihomo_TLS. Clear legacy relation state before
		// callers refresh subscription TLS fields from the inbound model.
		inbound.TlsId = 0
		inbound.Tls = nil
		util.SanitizeMihomoShadowQUICInboundTemplate(outbound)
	}
	if inbound.Type == "hysteria2" {
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
	if inbound.Type == "tuic" {
		delete(outbound, "mihomo_fast_open")
		delete(outbound, "fast_open")
		delete(outbound, "fast-open")
	}

	if inbound.Type == "tuic" {
		delete(outbound, "auth_timeout")
		delete(outbound, "authentication_timeout")
		delete(outbound, "max_idle_time")
		delete(outbound, "max-idle-time")
		delete(outbound, "zero_rtt_handshake")
		delete(outbound, "reduce_rtt")
		delete(outbound, "heartbeat")
		delete(outbound, "heartbeat_interval")
		delete(outbound, "network")
		fullInbound, err := inbound.MarshalFull()
		if err != nil {
			return err
		}
		if fullInbound != nil {
			if value, ok := normalizePositiveIntValue((*fullInbound)["max_udp_relay_packet_size"]); ok {
				if _, exists := outbound["max_udp_relay_packet_size"]; !exists {
					outbound["max_udp_relay_packet_size"] = value
				}
			}
			if value, ok := normalizePositiveIntValue((*fullInbound)["cwnd"]); ok {
				if _, exists := outbound["cwnd"]; !exists {
					outbound["cwnd"] = value
				}
			}
		}
	}

	normalized, err := json.MarshalIndent(outbound, "", "  ")
	if err != nil {
		return err
	}
	inbound.OutJson = normalized
	return nil
}

func migrateLegacyMihomoCommonFields(outbound map[string]interface{}, inboundType string) {
	if outbound == nil {
		return
	}

	migrateLegacyMihomoCommonStore(outbound, inboundType)
}

func sanitizeMihomoSubscriptionTLSFields(outbound map[string]interface{}) {
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

func sanitizeMihomoSubscriptionGRPCFields(outbound map[string]interface{}) {
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
			if normalized, ok := normalizePositiveIntValue(value); ok {
				transport[key] = normalized
			} else {
				delete(transport, key)
			}
		}
	}
	for _, key := range []string{"min_streams", "max_streams"} {
		if value, exists := transport[key]; exists {
			if normalized, ok := normalizeNonNegativeIntValue(value); ok {
				transport[key] = normalized
			} else {
				delete(transport, key)
			}
		}
	}
}

func sanitizeMihomoSubscriptionReceiveWindows(outbound map[string]interface{}, inboundType string) {
	if outbound == nil {
		return
	}
	if !strings.EqualFold(strings.TrimSpace(inboundType), "hysteria2") {
		delete(outbound, "mihomo_hy2")
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
		if value, ok := normalizePositiveIntValue(options[key]); ok {
			clean[key] = value
		}
	}
	if len(clean) == 0 {
		delete(outbound, "mihomo_hy2")
		return
	}
	outbound["mihomo_hy2"] = clean
}

func sanitizeMihomoSubscriptionCommonFields(root map[string]interface{}) {
	if root == nil {
		return
	}
	common, ok := root["mihomo_common"].(map[string]interface{})
	if !ok || common == nil {
		return
	}
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
		if normalized, ok := normalizeNonNegativeIntValue(value); ok {
			common["routing_mark"] = normalized
		} else {
			delete(common, "routing_mark")
		}
	}
	sanitizeMihomoSubscriptionMultiplexFields(common, "smux")
	if len(common) == 0 {
		delete(root, "mihomo_common")
	}
}

func sanitizeMihomoSubscriptionMultiplexFields(root map[string]interface{}, key string) {
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
			if normalized, ok := normalizeNonNegativeIntValue(value); ok {
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
	if rawBrutal, exists := mux["brutal"]; exists {
		brutal, ok := rawBrutal.(map[string]interface{})
		if !ok || brutal == nil {
			delete(mux, "brutal")
		} else {
			if value, exists := brutal["enabled"]; exists {
				if normalized, ok := value.(bool); ok {
					brutal["enabled"] = normalized
				} else {
					delete(brutal, "enabled")
				}
			}
			for _, key := range []string{"up_mbps", "down_mbps"} {
				if value, exists := brutal[key]; exists {
					if normalized, ok := normalizeNonNegativeIntValue(value); ok {
						brutal[key] = normalized
					} else {
						delete(brutal, key)
					}
				}
			}
			if len(brutal) == 0 {
				delete(mux, "brutal")
			}
		}
	}
	if len(mux) == 0 {
		delete(root, key)
	}
}

func migrateLegacyMihomoCommonStore(root map[string]interface{}, inboundType string) {
	if root == nil {
		return
	}

	common, ok := root["mihomo_common"].(map[string]interface{})
	if !ok || common == nil {
		common = map[string]interface{}{}
		root["mihomo_common"] = common
	}

	for _, key := range []string{"udp", "ip_version", "routing_mark", "tcp_fast_open", "tcp_multi_path"} {
		if value, exists := root[key]; exists {
			if _, hasCommon := common[key]; !hasCommon {
				common[key] = value
			}
			delete(root, key)
		}
	}

	if mux, ok := root["multiplex"].(map[string]interface{}); ok && mux != nil {
		if _, exists := common["smux"]; !exists {
			common["smux"] = mux
		}
		delete(root, "multiplex")
	}

	if util.SupportsMihomoBBRProfileProtocol(inboundType) {
		if profile, ok := util.NormalizeMihomoBBRProfile(common["bbr_profile"]); ok {
			common["bbr_profile"] = profile
		} else if profile, ok := util.NormalizeMihomoBBRProfile(common["bbr-profile"]); ok {
			common["bbr_profile"] = profile
		} else if profile, ok := util.NormalizeMihomoBBRProfile(root["bbr_profile"]); ok {
			common["bbr_profile"] = profile
		} else if profile, ok := util.NormalizeMihomoBBRProfile(root["bbr-profile"]); ok {
			common["bbr_profile"] = profile
		} else {
			delete(common, "bbr_profile")
		}
	} else {
		delete(common, "bbr_profile")
	}
	delete(common, "bbr-profile")
	delete(root, "bbr_profile")
	delete(root, "bbr-profile")

	if len(common) == 0 {
		delete(root, "mihomo_common")
	}
}

func normalizePositiveIntValue(raw interface{}) (int, bool) {
	value, ok := normalizeNonNegativeIntValue(raw)
	return value, ok && value > 0
}

func normalizeNonNegativeIntValue(raw interface{}) (int, bool) {
	maxInt := int64(^uint(0) >> 1)
	switch value := raw.(type) {
	case int:
		return value, value >= 0
	case int32:
		return int(value), value >= 0
	case int64:
		return int(value), value >= 0 && value <= maxInt
	case float32:
		parsed := float64(value)
		if parsed < 0 || math.IsNaN(parsed) || math.IsInf(parsed, 0) || math.Trunc(parsed) != parsed || parsed > float64(maxInt) {
			return 0, false
		}
		return int(parsed), true
	case float64:
		if value < 0 || math.IsNaN(value) || math.IsInf(value, 0) || math.Trunc(value) != value || value > float64(maxInt) {
			return 0, false
		}
		return int(value), true
	default:
		return 0, false
	}
}
