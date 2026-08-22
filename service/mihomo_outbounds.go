package service

import (
	"encoding/json"
	"strings"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/util"
	"github.com/alireza0/s-ui/util/common"
	"gorm.io/gorm"
)

type MihomoOutboundService struct{}

func normalizeMihomoOutboundRawPayload(data json.RawMessage) json.RawMessage {
	if len(data) == 0 {
		return nil
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(data, &payload); err != nil {
		return append(json.RawMessage(nil), data...)
	}

	delete(payload, "id")
	outboundType, _ := payload["type"].(string)
	outboundType = strings.ToLower(strings.TrimSpace(outboundType))
	stripUnsupportedMihomoFastOpenFields(payload, outboundType)
	stripUnsupportedMihomoTLSFields(payload)
	normalizeMihomoHysteriaOutboundFields(payload, outboundType)
	stripUnsupportedMihomoManualOutboundFields(payload, outboundType)
	sanitizeMihomoOutboundRoutingMark(payload)
	sanitizeMihomoGRPCNumericFields(payload)
	sanitizeMihomoMultiplexFields(payload, "multiplex")
	if ssConfig, ok := payload["ss_config"].(map[string]interface{}); ok && ssConfig != nil {
		sanitizeMihomoMultiplexFields(ssConfig, "multiplex")
	}
	if outboundType == "hysteria2" {
		util.EnsureHysteria2MihomoReceiveWindows(payload)
		sanitizeMihomoHysteria2ClientReceiveWindows(payload)
	} else {
		delete(payload, "mihomo_hy2")
	}
	if outboundType == "shadowquic" {
		util.SanitizeMihomoShadowQUICOutbound(payload)
	}
	if outboundType == "trusttunnel" {
		if tls, ok := payload["tls"].(map[string]interface{}); ok && tls != nil {
			delete(tls, "reality")
			delete(tls, "ech")
		}
	}
	normalized, err := json.Marshal(payload)
	if err != nil {
		return append(json.RawMessage(nil), data...)
	}
	return normalized
}

func normalizeMihomoHysteriaOutboundFields(payload map[string]interface{}, outboundType string) {
	if outboundType != "hysteria" && outboundType != "hysteria2" {
		return
	}

	for canonicalKey, legacyKey := range map[string]string{
		"up_mbps":   "server_up_mbps",
		"down_mbps": "server_down_mbps",
	} {
		legacyValue, exists := payload[legacyKey]
		if !exists {
			continue
		}
		if normalized, ok := toInt(legacyValue); ok && normalized >= 0 {
			// These server_* keys were mistakenly written by the shared outbound
			// editor. They have no Mihomo proxy meaning, so preserve the user's
			// bandwidth choice under the actual outbound keys.
			payload[canonicalKey] = normalized
		}
		delete(payload, legacyKey)
	}

	if outboundType == "hysteria" {
		delete(payload, "network")
	}
}

func stripUnsupportedMihomoManualOutboundFields(payload map[string]interface{}, outboundType string) {
	switch outboundType {
	case "socks", "socks5":
		delete(payload, "version")
		delete(payload, "udp_over_tcp")
	case "http":
		delete(payload, "path")
	case "vmess", "vless", "trojan", "hysteria2", "tuic":
		delete(payload, "network")
	}
}

func resolveMihomoOutboundJSON(outbound *model.MihomoOutbound) ([]byte, error) {
	if outbound == nil {
		return nil, common.NewError("mihomo outbound is nil")
	}

	if len(outbound.RawOutbound) > 0 {
		var payload map[string]interface{}
		if err := json.Unmarshal(outbound.RawOutbound, &payload); err == nil && payload != nil {
			return normalizeMihomoOutboundRawPayload(outbound.RawOutbound), nil
		}
	}

	return outbound.MarshalJSON()
}

func preserveExistingMihomoImportedRawProxy(raw json.RawMessage, existing json.RawMessage) json.RawMessage {
	if len(raw) == 0 || len(existing) == 0 {
		return raw
	}

	payload := map[string]interface{}{}
	if err := json.Unmarshal(raw, &payload); err != nil || payload == nil {
		return raw
	}
	if _, exists := payload[mihomoImportedClashProxyKey]; exists {
		return raw
	}

	existingPayload := map[string]interface{}{}
	if err := json.Unmarshal(existing, &existingPayload); err != nil || existingPayload == nil {
		return raw
	}
	rawProxy, exists := existingPayload[mihomoImportedClashProxyKey]
	if !exists || rawProxy == nil {
		return raw
	}

	payload[mihomoImportedClashProxyKey] = rawProxy
	merged, err := json.Marshal(payload)
	if err != nil {
		return raw
	}
	return merged
}

func (s *MihomoOutboundService) GetAll() (*[]map[string]interface{}, error) {
	db := database.GetDB()
	outbounds := []*model.MihomoOutbound{}
	if err := db.Model(model.MihomoOutbound{}).Order("id ASC").Scan(&outbounds).Error; err != nil {
		return nil, err
	}

	data := make([]map[string]interface{}, 0, len(outbounds))
	for _, outbound := range outbounds {
		outboundJSON, err := resolveMihomoOutboundJSON(outbound)
		if err != nil {
			return nil, err
		}

		outData := map[string]interface{}{}
		if err := json.Unmarshal(outboundJSON, &outData); err != nil {
			return nil, err
		}
		outData["id"] = outbound.Id
		outData = normalizeLegacyMihomoImportedOutboundForDisplay(outData)
		data = append(data, outData)
	}

	return &data, nil
}

func normalizeLegacyMihomoImportedOutboundForDisplay(outbound map[string]interface{}) map[string]interface{} {
	if outbound == nil {
		return nil
	}
	if _, exists := outbound[mihomoImportedClashProxyKey]; exists {
		return outbound
	}

	name, _ := outbound["name"].(string)
	name = strings.TrimSpace(name)
	if name == "" {
		return outbound
	}
	if _, hasServerPort := outbound["server_port"]; hasServerPort {
		return outbound
	}
	if _, hasPort := outbound["port"]; !hasPort {
		portRange := strings.TrimSpace(firstString(outbound["port-range"]))
		if portRange == "" {
			return outbound
		}
	}

	tag, _ := outbound["tag"].(string)
	tag = strings.TrimSpace(tag)
	outType, _ := outbound["type"].(string)
	outType = strings.TrimSpace(outType)
	normalized := normalizeMihomoImportedOutbound(outbound, tag, outType)
	if normalized == nil {
		return outbound
	}
	if id, exists := outbound["id"]; exists {
		normalized["id"] = id
	}
	return normalized
}

func (s *MihomoOutboundService) GetAllConfig(db *gorm.DB) ([]json.RawMessage, error) {
	var outboundsJSON []json.RawMessage
	var outbounds []*model.MihomoOutbound
	if err := db.Model(model.MihomoOutbound{}).Order("id ASC").Scan(&outbounds).Error; err != nil {
		return nil, err
	}

	for _, outbound := range outbounds {
		outboundJSON, err := resolveMihomoOutboundJSON(outbound)
		if err != nil {
			return nil, err
		}
		if outbound.Type == "shadowtls" {
			ssJSON, shadowtlsJSON, err := s.processShadowTLSOutbound(outboundJSON)
			if err != nil {
				return nil, err
			}
			if ssJSON != nil {
				outboundsJSON = append(outboundsJSON, ssJSON)
			}
			outboundsJSON = append(outboundsJSON, shadowtlsJSON)
		} else {
			outboundsJSON = append(outboundsJSON, outboundJSON)
		}
	}

	return outboundsJSON, nil
}

func (s *MihomoOutboundService) processShadowTLSOutbound(outboundJSON []byte) (json.RawMessage, json.RawMessage, error) {
	return util.BuildShadowTLSRuntimeOutboundPairJSON(outboundJSON, true)
}

func (s *MihomoOutboundService) processShadowTLSOutboundLegacy(outboundJSON []byte) (json.RawMessage, json.RawMessage, error) {
	var outboundData map[string]interface{}
	if err := json.Unmarshal(outboundJSON, &outboundData); err != nil {
		return nil, nil, err
	}

	ssConfig, hasSSConfig := outboundData["ss_config"].(map[string]interface{})
	if !hasSSConfig || ssConfig == nil {
		stripShadowTLSInboundOnlyFields(outboundData)
		sanitizedJSON, err := json.Marshal(outboundData)
		if err != nil {
			return nil, nil, err
		}
		return nil, sanitizedJSON, nil
	}

	delete(outboundData, "ss_config")
	stripShadowTLSInboundOnlyFields(outboundData)

	tag, ok := outboundData["tag"].(string)
	if !ok || tag == "" {
		shadowtlsJSON, err := json.Marshal(outboundData)
		if err != nil {
			return nil, nil, err
		}
		return nil, shadowtlsJSON, nil
	}

	shadowtlsTag := tag + "-out"
	outboundData["tag"] = shadowtlsTag

	shadowtlsJSON, err := json.Marshal(outboundData)
	if err != nil {
		return nil, nil, err
	}

	ssOutbound := map[string]interface{}{
		"type":   "shadowsocks",
		"tag":    tag,
		"detour": shadowtlsTag,
	}
	if method, ok := ssConfig["method"]; ok && method != nil {
		ssOutbound["method"] = method
	}
	if network, ok := ssConfig["network"]; ok && network != nil && network != "" {
		ssOutbound["network"] = network
	}
	if password, ok := ssConfig["password"]; ok && password != nil {
		ssOutbound["password"] = password
	}
	if udpOverTCP, ok := ssConfig["udp_over_tcp"]; ok && udpOverTCP != nil {
		ssOutbound["udp_over_tcp"] = udpOverTCP
	}
	if multiplex, ok := ssConfig["multiplex"].(map[string]interface{}); ok && multiplex != nil {
		ssOutbound["multiplex"] = multiplex
	}

	ssOutboundJSON, err := json.Marshal(ssOutbound)
	if err != nil {
		return nil, nil, err
	}

	return ssOutboundJSON, shadowtlsJSON, nil
}

func (s *MihomoOutboundService) Save(tx *gorm.DB, act string, data json.RawMessage) error {
	switch act {
	case "new", "edit":
		var outbound model.MihomoOutbound
		if err := outbound.UnmarshalJSON(data); err != nil {
			return err
		}
		if act == "new" {
			// Do not allow a cloned editor payload to turn a create into an
			// overwrite through GORM Save.
			outbound.Id = 0
		} else if outbound.Id == 0 {
			return common.NewError("mihomo outbound id is required for edit")
		}
		incomingRaw := normalizeMihomoOutboundRawPayload(data)
		outbound.RawOutbound = incomingRaw
		outbound.RawClashYAML = nil
		oldTag := ""
		if act == "edit" {
			var existing model.MihomoOutbound
			if err := tx.Model(model.MihomoOutbound{}).Where("id = ?", outbound.Id).Take(&existing).Error; err != nil {
				return err
			}
			oldTag = strings.TrimSpace(existing.Tag)
			if len(outbound.RawOutbound) > 0 && existing.Type == outbound.Type && !strings.EqualFold(strings.TrimSpace(outbound.Type), "shadowquic") {
				if baseRaw, resolveErr := resolveMihomoOutboundJSON(&existing); resolveErr == nil {
					outbound.RawOutbound = mergeEditableOutboundRawPayload(baseRaw, data, "mihomo", outbound.Type)
				}
				outbound.RawOutbound = preserveExistingMihomoImportedRawProxy(outbound.RawOutbound, existing.RawOutbound)
			}
			if len(outbound.RawOutbound) == 0 {
				outbound.RawOutbound = incomingRaw
			}
		}
		if len(outbound.RawOutbound) == 0 {
			outbound.RawOutbound = incomingRaw
		}
		outbound.RawOutbound = normalizeMihomoOutboundRawPayload(outbound.RawOutbound)
		outbound.RawOutbound = sanitizeMihomoShadowQUICOutboundRaw(outbound.RawOutbound, outbound.Type)
		if strings.EqualFold(strings.TrimSpace(outbound.Type), "shadowquic") {
			shadowQUICRaw := map[string]interface{}{}
			if err := json.Unmarshal(outbound.RawOutbound, &shadowQUICRaw); err != nil {
				return err
			}
			if err := util.ValidateMihomoShadowQUICOutbound(shadowQUICRaw, outbound.Tag); err != nil {
				return err
			}
		}
		if err := tx.Save(&outbound).Error; err != nil {
			return err
		}
		if oldTag != "" && oldTag != strings.TrimSpace(outbound.Tag) {
			if err := replaceMihomoOutboundTagInPanelGroups(tx, oldTag, outbound.Tag); err != nil {
				return err
			}
			if err := validateMihomoOutboundRemovalReferences(tx, []string{oldTag}, nil, nil); err != nil {
				return err
			}
			return replaceMihomoShadowQUICJLSProxyReferences(tx, oldTag, outbound.Tag)
		}
		return nil
	case "del":
		var tag string
		if err := json.Unmarshal(data, &tag); err != nil {
			return err
		}
		tag = strings.TrimSpace(tag)
		if err := removeMihomoOutboundTagFromPanelGroups(tx, tag); err != nil {
			return err
		}
		if err := validateMihomoOutboundRemovalReferences(tx, []string{tag}, nil, []string{tag}); err != nil {
			return err
		}
		return tx.Where("tag = ?", tag).Delete(model.MihomoOutbound{}).Error
	default:
		return common.NewErrorf("unknown action: %s", act)
	}
}
