package sub

import (
	"encoding/json"

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

	var inboundIDs []uint
	if err := json.Unmarshal(mihomoClient.Inbounds, &inboundIDs); err != nil {
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

	migrateLegacyMihomoCommonFields(outbound, inbound.Type)
	if inbound.Type == "shadowtls" {
		sanitizeMihomoShadowTLSSubscriptionOutJSON(outbound)
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
		if _, exists := outbound["mihomo_fast_open"]; !exists {
			if fastOpen, existsFastOpen := outbound["fast_open"]; existsFastOpen {
				outbound["mihomo_fast_open"] = fastOpen
			}
		}
		delete(outbound, "fast_open")

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

func sanitizeMihomoShadowTLSSubscriptionOutJSON(outbound map[string]interface{}) {
	if outbound == nil {
		return
	}
	ssConfig, ok := outbound["ss_config"].(map[string]interface{})
	if !ok || ssConfig == nil {
		return
	}
	delete(ssConfig, "network")
}

func migrateLegacyMihomoCommonFields(outbound map[string]interface{}, inboundType string) {
	if outbound == nil {
		return
	}

	if inboundType == "shadowtls" {
		ssConfig, ok := outbound["ss_config"].(map[string]interface{})
		if !ok || ssConfig == nil {
			return
		}
		migrateLegacyMihomoCommonStore(ssConfig, inboundType)
		return
	}

	migrateLegacyMihomoCommonStore(outbound, inboundType)
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
	switch value := raw.(type) {
	case int:
		return value, value > 0
	case int32:
		return int(value), value > 0
	case int64:
		return int(value), value > 0
	case float32:
		return int(value), value > 0
	case float64:
		return int(value), value > 0
	default:
		return 0, false
	}
}
