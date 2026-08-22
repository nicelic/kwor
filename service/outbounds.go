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

type OutboundService struct{}

func normalizeOutboundRawPayload(data json.RawMessage) json.RawMessage {
	if len(data) == 0 {
		return nil
	}

	var payload map[string]interface{}
	if err := json.Unmarshal(data, &payload); err != nil {
		return append(json.RawMessage(nil), data...)
	}

	delete(payload, "id")
	if value, ok := payload["type"].(string); ok {
		payload["type"] = strings.ToLower(strings.TrimSpace(value))
	}
	if value, ok := payload["tag"].(string); ok {
		payload["tag"] = strings.TrimSpace(value)
	}
	normalized, err := json.Marshal(payload)
	if err != nil {
		return append(json.RawMessage(nil), data...)
	}
	return normalized
}

func resolveOutboundJSON(outbound *model.Outbound) ([]byte, error) {
	if outbound == nil {
		return nil, common.NewError("outbound is nil")
	}

	if len(outbound.RawOutbound) > 0 {
		var payload map[string]interface{}
		if err := json.Unmarshal(outbound.RawOutbound, &payload); err == nil && payload != nil {
			return append(json.RawMessage(nil), outbound.RawOutbound...), nil
		}
	}

	return outbound.MarshalJSON()
}

func (o *OutboundService) GetAll() (*[]map[string]interface{}, error) {
	db := database.GetDB()
	outbounds := []*model.Outbound{}
	err := db.Model(model.Outbound{}).Scan(&outbounds).Error
	if err != nil {
		return nil, err
	}
	data := make([]map[string]interface{}, 0, len(outbounds))
	for _, outbound := range outbounds {
		outboundJSON, err := resolveOutboundJSON(outbound)
		if err != nil {
			return nil, err
		}
		outData := map[string]interface{}{}
		if err := json.Unmarshal(outboundJSON, &outData); err != nil {
			return nil, err
		}
		outData["id"] = outbound.Id
		data = append(data, outData)
	}
	return &data, nil
}

func (o *OutboundService) GetAllConfig(db *gorm.DB) ([]json.RawMessage, error) {
	var outboundsJson []json.RawMessage
	var outbounds []*model.Outbound
	err := db.Model(model.Outbound{}).Scan(&outbounds).Error
	if err != nil {
		return nil, err
	}
	for _, outbound := range outbounds {
		outboundJson, err := resolveOutboundJSON(outbound)
		if err != nil {
			return nil, err
		}

		// 处理 ShadowTLS 组合出站
		if outbound.Type == "shadowtls" {
			ssJson, shadowtlsJson, err := o.processShadowTLSOutbound(outboundJson, outbound)
			if err != nil {
				return nil, err
			}
			// 注意：shadowsocks 出站需要在 shadowtls 之前添加
			if ssJson != nil {
				outboundsJson = append(outboundsJson, ssJson)
			}
			outboundsJson = append(outboundsJson, shadowtlsJson)
		} else {
			outboundsJson = append(outboundsJson, outboundJson)
		}
	}
	return outboundsJson, nil
}

// processShadowTLSOutbound 处理 ShadowTLS 出站，如果有 ss_config，生成组合的出站配置
// 按照 sing-box 标准格式生成（图b）：
// 1. Shadowsocks 出站: type, tag, method, password, detour, udp_over_tcp, multiplex
// 2. ShadowTLS 出站: type, tag(-out), server, server_port, version, password, tls
// 返回: ssOutboundJson, shadowtlsJson, error
func (o *OutboundService) processShadowTLSOutbound(outboundJson []byte, outbound *model.Outbound) (json.RawMessage, json.RawMessage, error) {
	return util.BuildShadowTLSRuntimeOutboundPairJSON(outboundJson, true)
}

func (o *OutboundService) processShadowTLSOutboundLegacy(outboundJson []byte, outbound *model.Outbound) (json.RawMessage, json.RawMessage, error) {
	var outboundData map[string]interface{}
	if err := json.Unmarshal(outboundJson, &outboundData); err != nil {
		return nil, nil, err
	}

	// 检查是否有 ss_config
	ssConfig, hasSsConfig := outboundData["ss_config"].(map[string]interface{})
	if !hasSsConfig || ssConfig == nil {
		stripShadowTLSInboundOnlyFields(outboundData)
		sanitizedJson, err := json.Marshal(outboundData)
		if err != nil {
			return nil, nil, err
		}
		return nil, sanitizedJson, nil
	}

	// 删除 ss_config，不需要在最终的 shadowtls 配置中
	delete(outboundData, "ss_config")
	stripShadowTLSInboundOnlyFields(outboundData)

	// 安全获取 tag
	tag, ok := outboundData["tag"].(string)
	if !ok || tag == "" {
		shadowtlsJson, err := json.Marshal(outboundData)
		if err != nil {
			return nil, nil, err
		}
		return nil, shadowtlsJson, nil
	}

	// 生成内部 shadowtls 出站的 tag
	shadowtlsTag := tag + "-out"

	// 修改 shadowtls 的 tag 为 xxx-out
	outboundData["tag"] = shadowtlsTag

	// 生成 shadowtls 出站配置
	shadowtlsJson, err := json.Marshal(outboundData)
	if err != nil {
		return nil, nil, err
	}

	// 生成 shadowsocks 出站配置（主出站，使用原始 tag）
	// 按图b格式: type, tag, method, password, detour, udp_over_tcp, multiplex
	ssOutbound := map[string]interface{}{
		"type":   "shadowsocks",
		"tag":    tag,
		"detour": shadowtlsTag,
	}

	// 添加 method
	if method, ok := ssConfig["method"]; ok && method != nil {
		ssOutbound["method"] = method
	}
	// 添加 network
	if network, ok := ssConfig["network"]; ok && network != nil && network != "" {
		ssOutbound["network"] = network
	}
	// 添加 password（直接字符串）
	if password, ok := ssConfig["password"]; ok && password != nil {
		ssOutbound["password"] = password
	}
	// 添加 udp_over_tcp
	if udpOverTcp, ok := ssConfig["udp_over_tcp"]; ok && udpOverTcp != nil {
		ssOutbound["udp_over_tcp"] = udpOverTcp
	}

	// 添加多路复用配置（包含所有字段，不仅仅是 enabled 的）
	if multiplex, ok := ssConfig["multiplex"].(map[string]interface{}); ok && multiplex != nil {
		ssOutbound["multiplex"] = multiplex
	}

	ssOutboundJson, err := json.Marshal(ssOutbound)
	if err != nil {
		return nil, nil, err
	}

	return ssOutboundJson, shadowtlsJson, nil
}

func (s *OutboundService) Save(tx *gorm.DB, act string, data json.RawMessage) error {
	var err error

	switch act {
	case "new", "edit":
		normalizedData, identity, err := validateAndNormalizeSingboxOutboundPayload(data, act)
		if err != nil {
			return err
		}
		var outbound model.Outbound
		err = outbound.UnmarshalJSON(normalizedData)
		if err != nil {
			return err
		}
		outbound.Id = identity.ID
		outbound.Type = identity.Type
		outbound.Tag = identity.Tag
		incomingRaw := normalizeOutboundRawPayload(normalizedData)
		oldTag := ""
		var previousRuntimeTags []string
		if act == "edit" {
			existing := &model.Outbound{}
			if err := tx.Model(model.Outbound{}).Where("id = ?", outbound.Id).First(existing).Error; err != nil {
				return err
			}
			oldTag = strings.TrimSpace(existing.Tag)
			previousRuntimeTags, err = singboxRuntimeOutboundTags(existing)
			if err != nil {
				return err
			}
			if existing.Type == outbound.Type {
				if baseRaw, resolveErr := resolveOutboundJSON(existing); resolveErr == nil {
					outbound.RawOutbound = mergeEditableOutboundRawPayload(baseRaw, incomingRaw, "default", outbound.Type)
				} else {
					outbound.RawOutbound = incomingRaw
				}
			} else {
				outbound.RawOutbound = incomingRaw
			}
		} else {
			outbound.RawOutbound = incomingRaw
		}
		if len(outbound.RawOutbound) == 0 {
			outbound.RawOutbound = incomingRaw
		}
		// RawOutbound is the authoritative editable/runtime payload. Keeping the
		// same options JSON in both columns doubles memory and SQLite I/O for
		// imported node collections; Options remains only for legacy rows without
		// RawOutbound.
		outbound.Options = nil

		err = tx.Save(&outbound).Error
		if err != nil {
			return err
		}
		currentRuntimeTags, err := singboxRuntimeOutboundTags(&outbound)
		if err != nil {
			return err
		}
		resolved, err := resolveOutboundJSON(&outbound)
		if err != nil {
			return err
		}
		payload := map[string]interface{}{}
		if err := json.Unmarshal(resolved, &payload); err != nil {
			return err
		}
		if err := validateSingboxOutboundPayloadReferences(tx, &outbound, payload); err != nil {
			return err
		}
		if err := validateSingboxStoredRuntimeOutboundTags(tx); err != nil {
			return err
		}
		if oldTag != "" && oldTag != strings.TrimSpace(outbound.Tag) {
			if err := replaceSingboxOutboundTagInPanelGroups(tx, oldTag, outbound.Tag); err != nil {
				return err
			}
		}
		removedRuntimeTags := removedSingboxRuntimeTags(previousRuntimeTags, currentRuntimeTags)
		if len(removedRuntimeTags) > 0 {
			if err := validateSingboxOutboundRemovalReferences(tx, removedRuntimeTags, nil); err != nil {
				return err
			}
		}
	case "del":
		var tag string
		err = json.Unmarshal(data, &tag)
		if err != nil {
			return err
		}
		tag = strings.TrimSpace(tag)
		var existing model.Outbound
		if err := tx.Where("tag = ?", tag).First(&existing).Error; err != nil {
			return err
		}
		removedRuntimeTags, err := singboxRuntimeOutboundTags(&existing)
		if err != nil {
			return err
		}
		if err := removeSingboxOutboundTagFromPanelGroups(tx, tag); err != nil {
			return err
		}
		if err := validateSingboxOutboundRemovalReferences(tx, removedRuntimeTags, nil); err != nil {
			return err
		}
		err = tx.Where("tag = ?", tag).Delete(model.Outbound{}).Error
		if err != nil {
			return err
		}
	default:
		return common.NewErrorf("unknown action: %s", act)
	}
	return nil
}

func stripOutboundsTLSStore(outbounds []json.RawMessage) ([]json.RawMessage, error) {
	normalized, _, err := normalizeSingboxRuntimeOutbounds(outbounds)
	if err != nil {
		return nil, err
	}
	return normalized, nil
}

func stripOutboundTLSStore(outbound map[string]interface{}) {
	tlsRaw, ok := outbound["tls"]
	if !ok {
		return
	}
	tlsMap, ok := tlsRaw.(map[string]interface{})
	if !ok || tlsMap == nil {
		return
	}

	delete(tlsMap, "tls_store")
	delete(tlsMap, "store")

	if len(tlsMap) == 0 {
		delete(outbound, "tls")
	}
}

func stripShadowTLSInboundOnlyFields(outbound map[string]interface{}) {
	util.StripShadowTLSInboundOnlyFields(outbound)
}

func sanitizeShadowTLSOutboundJSON(raw []byte) ([]byte, error) {
	outboundData := map[string]interface{}{}
	if err := json.Unmarshal(raw, &outboundData); err != nil {
		return nil, err
	}
	stripShadowTLSInboundOnlyFields(outboundData)
	return json.Marshal(outboundData)
}

func normalizeCertificateStoreValue(raw interface{}) string {
	store, ok := raw.(string)
	if !ok {
		return ""
	}
	store = strings.ToLower(strings.TrimSpace(store))
	switch store {
	case "system", "mozilla", "chrome", "none":
		return store
	default:
		return ""
	}
}
