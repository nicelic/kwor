package service

import (
	"encoding/json"
	"os"
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
	var data []map[string]interface{}
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
		var outbound model.Outbound
		err = outbound.UnmarshalJSON(data)
		if err != nil {
			return err
		}
		incomingRaw := normalizeOutboundRawPayload(data)
		oldTag := ""
		oldType := ""
		if act == "edit" {
			existing := &model.Outbound{}
			query := tx.Model(model.Outbound{})
			if outbound.Id != 0 {
				query = query.Where("id = ?", outbound.Id)
			} else {
				query = query.Where("tag = ?", outbound.Tag)
			}
			if err := query.First(existing).Error; err == nil {
				oldTag = existing.Tag
				oldType = existing.Type
				if existing.Type == outbound.Type {
					if baseRaw, resolveErr := resolveOutboundJSON(existing); resolveErr == nil {
						outbound.RawOutbound = mergeEditableOutboundRawPayload(baseRaw, data, "default", outbound.Type)
					} else {
						outbound.RawOutbound = incomingRaw
					}
				} else {
					outbound.RawOutbound = incomingRaw
				}
			} else if !database.IsNotFound(err) {
				return err
			} else {
				outbound.RawOutbound = incomingRaw
			}
		} else {
			outbound.RawOutbound = incomingRaw
		}
		if len(outbound.RawOutbound) == 0 {
			outbound.RawOutbound = incomingRaw
		}

		if corePtr.IsRunning() {
			configData, err := resolveOutboundJSON(&outbound)
			if err != nil {
				return err
			}
			runtimePayloads, err := buildRuntimeOutboundPayloads(configData, outbound.Type)
			if err != nil {
				return err
			}

			if act == "edit" {
				if oldTag == "" {
					return gorm.ErrRecordNotFound
				}
				if err := removeRuntimeOutboundFromCore(oldTag, oldType); err != nil {
					return err
				}
			}

			for _, payload := range runtimePayloads {
				err = corePtr.AddOutbound(payload)
				if err != nil {
					return err
				}
			}
		}

		err = tx.Save(&outbound).Error
		if err != nil {
			return err
		}
	case "del":
		var tag string
		err = json.Unmarshal(data, &tag)
		if err != nil {
			return err
		}
		if corePtr.IsRunning() {
			outboundType := ""
			var old struct {
				Type string
			}
			findErr := tx.Model(model.Outbound{}).Select("type").Where("tag = ?", tag).Take(&old).Error
			if findErr == nil {
				outboundType = old.Type
			}
			err = removeRuntimeOutboundFromCore(tag, outboundType)
			if err != nil && err != os.ErrInvalid {
				return err
			}
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

func buildRuntimeOutboundPayloads(configData json.RawMessage, outboundType string) ([]json.RawMessage, error) {
	sanitizePayloads := func(payloads []json.RawMessage) ([]json.RawMessage, error) {
		if len(payloads) == 0 {
			return payloads, nil
		}
		sanitized, _, err := normalizeSingboxRuntimeOutbounds(payloads)
		if err != nil {
			return nil, err
		}
		return sanitized, nil
	}

	if outboundType != "shadowtls" {
		return sanitizePayloads([]json.RawMessage{configData})
	}

	var outboundSvc OutboundService
	ssJson, stlsJson, err := outboundSvc.processShadowTLSOutbound(configData, nil)
	if err != nil {
		return nil, err
	}

	payloads := make([]json.RawMessage, 0, 2)
	if ssJson != nil {
		payloads = append(payloads, ssJson)
	}
	if stlsJson != nil {
		payloads = append(payloads, stlsJson)
	}
	return sanitizePayloads(payloads)
}

func removeRuntimeOutboundFromCore(tag string, outboundType string) error {
	if tag == "" {
		return nil
	}
	if err := corePtr.RemoveOutbound(tag); err != nil && err != os.ErrInvalid {
		return err
	}

	if outboundType == "shadowtls" {
		stlsTag := tag + "-out"
		if err := corePtr.RemoveOutbound(stlsTag); err != nil && err != os.ErrInvalid {
			return err
		}
	}
	return nil
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
