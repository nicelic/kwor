package service

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/util"
	"github.com/alireza0/s-ui/util/common"

	"gorm.io/gorm"
)

type InboundService struct {
	ClientService
	NftTrafficService
}

type InboundNftAction struct {
	Kind         string
	InboundID    uint
	Tag          string
	Port         int
	PortHopRange string
	RedirectTCP  bool
}

func (s *InboundService) Get(ids string) (*[]map[string]interface{}, error) {
	if ids == "" {
		return s.GetAll()
	}
	return s.getById(ids)
}

func (s *InboundService) getById(ids string) (*[]map[string]interface{}, error) {
	var inbound []model.Inbound
	var result []map[string]interface{}
	db := database.GetDB()
	err := db.Model(model.Inbound{}).Where("id in ?", strings.Split(ids, ",")).Scan(&inbound).Error
	if err != nil {
		return nil, err
	}
	for _, inb := range inbound {
		inbData, err := inb.MarshalFull()
		if err != nil {
			return nil, err
		}
		result = append(result, *inbData)
	}
	return &result, nil
}

// GetOutJsonIPs 获取指定入站的 out_json 中的 server IP 列表
// 返回格式: [{inbound_id: id, tag: tag, server: ip}, ...]
func (s *InboundService) GetOutJsonIPs(ids string) ([]map[string]interface{}, error) {
	db := database.GetDB()
	var inbounds []model.Inbound

	if ids == "" {
		// 如果没有指定ID，返回所有入站的IP
		err := db.Model(model.Inbound{}).Find(&inbounds).Error
		if err != nil {
			return nil, err
		}
	} else {
		// 根据指定的ID列表查询
		err := db.Model(model.Inbound{}).Where("id in ?", strings.Split(ids, ",")).Find(&inbounds).Error
		if err != nil {
			return nil, err
		}
	}

	var result []map[string]interface{}
	for _, inbound := range inbounds {
		if len(inbound.OutJson) < 5 {
			continue
		}

		var outJson map[string]interface{}
		err := json.Unmarshal(inbound.OutJson, &outJson)
		if err != nil {
			continue
		}

		server, ok := outJson["server"].(string)
		if !ok || server == "" {
			continue
		}

		result = append(result, map[string]interface{}{
			"id":     inbound.Id,
			"tag":    inbound.Tag,
			"server": util.NormalizeSubscriptionServerHost(server),
		})
	}

	return result, nil
}

func (s *InboundService) GetAll() (*[]map[string]interface{}, error) {
	db := database.GetDB()
	inbounds := []model.Inbound{}
	err := db.Model(model.Inbound{}).Scan(&inbounds).Error
	if err != nil {
		return nil, err
	}
	var data []map[string]interface{}
	for _, inbound := range inbounds {
		var shadowtls_version uint
		ss_managed := false
		routeTag := deriveEffectiveInboundRouteTagFromRaw(inbound.Tag, inbound.Type, inbound.Options)
		inbData := map[string]interface{}{
			"id":        inbound.Id,
			"type":      inbound.Type,
			"tag":       inbound.Tag,
			"route_tag": routeTag,
			"tls_id":    inbound.TlsId,
		}
		if inbound.Options != nil {
			var restFields map[string]json.RawMessage
			if err := json.Unmarshal(inbound.Options, &restFields); err != nil {
				return nil, err
			}
			inbData["listen"] = restFields["listen"]
			inbData["listen_port"] = restFields["listen_port"]
			if inbound.Type == "shadowtls" {
				json.Unmarshal(restFields["version"], &shadowtls_version)
			}
			if inbound.Type == "shadowsocks" {
				// 开发者要求隐藏并默认关闭 SS API 专用能力，读取列表时统一按 managed=false 处理。
				// Developer requirement: hide and default-disable SS API-only capability; always treat managed as false in list view.
				ss_managed = false
			}
		}
		if inbound.Type == "ssh" {
			inbData["user_management"] = map[string]interface{}{
				"selectable":       true,
				"uses_users_field": false,
				"mode":             "shared_credentials",
				"identity_type":    "type_tag",
				"reason":           "ssh_subscription_outbound_only",
			}
		}
		if s.hasUser(inbound.Type) &&
			!(inbound.Type == "shadowtls" && shadowtls_version < 3) &&
			!(inbound.Type == "shadowsocks" && ss_managed) {
			users := []string{}
			err = db.Raw("SELECT clients.name FROM clients, json_each(clients.inbounds) as je WHERE je.value = ?", inbound.Id).Scan(&users).Error
			if err != nil {
				return nil, err
			}
			inbData["users"] = users
		}

		data = append(data, inbData)
	}
	return &data, nil
}

func (s *InboundService) FromIds(ids []uint) ([]*model.Inbound, error) {
	db := database.GetDB()
	inbounds := []*model.Inbound{}
	err := db.Model(model.Inbound{}).Where("id in ?", ids).Scan(&inbounds).Error
	if err != nil {
		return nil, err
	}
	return inbounds, nil
}

func (s *InboundService) Save(tx *gorm.DB, act string, data json.RawMessage, initUserIds string, hostname string) (*InboundNftAction, error) {
	var err error
	var nftAction *InboundNftAction

	switch act {
	case "new", "edit":
		var inbound model.Inbound
		err = inbound.UnmarshalJSON(data)
		if err != nil {
			return nil, err
		}
		if inbound.TlsId > 0 {
			err = tx.Model(model.Tls{}).Where("id = ?", inbound.TlsId).Find(&inbound.Tls).Error
			if err != nil {
				return nil, err
			}
		}
		var oldTag string
		if act == "edit" {
			err = tx.Model(model.Inbound{}).Select("tag").Where("id = ?", inbound.Id).Find(&oldTag).Error
			if err != nil {
				return nil, err
			}
		}

		if corePtr.IsRunning() {
			if act == "edit" {
				err = corePtr.RemoveInbound(oldTag)
				if err != nil && err != os.ErrInvalid {
					return nil, err
				}
			}

			inboundConfig, err := inbound.MarshalJSON()
			if err != nil {
				return nil, err
			}

			if act == "edit" {
				inboundConfig, err = s.addUsers(tx, inboundConfig, inbound.Id, inbound.Type)
			} else {
				inboundConfig, err = s.initUsers(tx, inboundConfig, initUserIds, inbound.Type)
			}
			if err != nil {
				return nil, err
			}

			err = corePtr.AddInbound(inboundConfig)
			if err != nil {
				return nil, err
			}
		}

		err = util.FillOutJson(&inbound, hostname)
		if err != nil {
			return nil, err
		}

		err = tx.Save(&inbound).Error
		if err != nil {
			return nil, err
		}

		switch act {
		case "new":
			err = s.ClientService.UpdateClientsOnInboundAdd(tx, initUserIds, inbound.Id, hostname)
		case "edit":
			err = s.ClientService.UpdateLinksByInboundChange(tx, &[]model.Inbound{inbound}, hostname, oldTag)
		}
		if err != nil {
			return nil, err
		}
		nftAction = &InboundNftAction{
			Kind:         "upsert",
			InboundID:    inbound.Id,
			Tag:          inbound.Tag,
			Port:         extractPort(inbound.Options),
			PortHopRange: extractPortHopRange(inbound.Options),
		}
	case "del":
		var tag string
		err = json.Unmarshal(data, &tag)
		if err != nil {
			return nil, err
		}
		if corePtr.IsRunning() {
			err = corePtr.RemoveInbound(tag)
			if err != nil && err != os.ErrInvalid {
				return nil, err
			}
		}
		var id uint
		err = tx.Model(model.Inbound{}).Select("id").Where("tag = ?", tag).Scan(&id).Error
		if err != nil {
			return nil, err
		}
		err = s.ClientService.UpdateClientsOnInboundDelete(tx, id, tag)
		if err != nil {
			return nil, err
		}
		var syncSvc SyncService
		if err := syncSvc.CleanupSubOutboundsByInboundID(tx, subOutboundSourceClient, id); err != nil {
			return nil, err
		}

		err = tx.Where("tag = ?", tag).Delete(model.Inbound{}).Error
		if err != nil {
			return nil, err
		}
		nftAction = &InboundNftAction{
			Kind:      "remove",
			InboundID: id,
			Tag:       tag,
		}
	default:
		return nil, common.NewErrorf("unknown action: %s", act)
	}
	return nftAction, nil
}

func (s *InboundService) UpdateOutJsons(tx *gorm.DB, inboundIds []uint, hostname string) error {
	var inbounds []model.Inbound
	err := tx.Model(model.Inbound{}).Preload("Tls").Where("id in ?", inboundIds).Find(&inbounds).Error
	if err != nil {
		return err
	}
	for _, inbound := range inbounds {
		err = util.FillOutJson(&inbound, effectiveOutJSONHostname(inbound.OutJson, hostname))
		if err != nil {
			return err
		}
		err = tx.Model(model.Inbound{}).Where("tag = ?", inbound.Tag).Update("out_json", inbound.OutJson).Error
		if err != nil {
			return err
		}
	}

	return nil
}

func effectiveOutJSONHostname(outJSON json.RawMessage, hostname string) string {
	if util.NormalizeSubscriptionServerHost(hostname) != "" {
		return hostname
	}
	if len(outJSON) == 0 {
		return hostname
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(outJSON, &payload); err != nil {
		return hostname
	}
	if server, ok := payload["server"].(string); ok && util.NormalizeSubscriptionServerHost(server) != "" {
		return server
	}
	return hostname
}

// UpdateOutJsonServerIP 更新指定入站的 out_json 中的 server IP
// 用于客户端保存时更新其关联入站的出站IP
func (s *InboundService) UpdateOutJsonServerIP(tx *gorm.DB, inboundIds []uint, serverIP string) error {
	if serverIP == "" || len(inboundIds) == 0 {
		return nil
	}

	var inbounds []model.Inbound
	err := tx.Model(model.Inbound{}).Where("id in ?", inboundIds).Find(&inbounds).Error
	if err != nil {
		return err
	}

	for _, inbound := range inbounds {
		if len(inbound.OutJson) < 5 {
			continue
		}

		var outJson map[string]interface{}
		err := json.Unmarshal(inbound.OutJson, &outJson)
		if err != nil {
			continue
		}

		// 只更新有 server 字段的出站配置
		if _, hasServer := outJson["server"]; hasServer {
			outJson["server"] = serverIP
			newOutJson, err := json.MarshalIndent(outJson, "", "  ")
			if err != nil {
				continue
			}
			err = tx.Model(model.Inbound{}).Where("id = ?", inbound.Id).Update("out_json", newOutJson).Error
			if err != nil {
				return err
			}
		}
	}

	return nil
}

func (s *InboundService) GetAllConfig(db *gorm.DB) ([]json.RawMessage, error) {
	var inboundsJson []json.RawMessage
	var inbounds []*model.Inbound
	err := db.Model(model.Inbound{}).Preload("Tls").Find(&inbounds).Error
	if err != nil {
		return nil, err
	}
	for _, inbound := range inbounds {
		if inbound.Type == "ssh" {
			continue
		}
		inboundJson, err := inbound.MarshalJSON()
		if err != nil {
			return nil, err
		}
		inboundJson, err = s.addUsers(db, inboundJson, inbound.Id, inbound.Type)
		if err != nil {
			return nil, err
		}

		// 处理 ShadowTLS 组合入站
		if inbound.Type == "shadowtls" {
			shadowtlsJson, ssJson, err := s.processShadowTLSInbound(db, inboundJson, inbound)
			if err != nil {
				return nil, err
			}
			inboundsJson = append(inboundsJson, shadowtlsJson)
			if ssJson != nil {
				inboundsJson = append(inboundsJson, ssJson)
			}
		} else {
			inboundsJson = append(inboundsJson, inboundJson)
		}
	}
	return inboundsJson, nil
}

// processShadowTLSInbound 处理 ShadowTLS 入站，如果有 ss_config，生成组合的入站配置
// 按照 sing-box 标准格式生成：
// 1. ShadowTLS 入站: type, tag, listen, listen_port, detour, version, users, handshake, strict_mode
// 2. Shadowsocks 入站: type, tag, listen(127.0.0.1), network, method, password, multiplex
func (s *InboundService) processShadowTLSInbound(db *gorm.DB, inboundJson []byte, inbound *model.Inbound) (json.RawMessage, json.RawMessage, error) {
	var inboundData map[string]interface{}
	if err := json.Unmarshal(inboundJson, &inboundData); err != nil {
		return nil, nil, err
	}

	// 检查是否有 ss_config
	ssConfig, hasSsConfig := inboundData["ss_config"].(map[string]interface{})
	if !hasSsConfig || ssConfig == nil {
		// 没有 ss_config，返回原始配置
		return inboundJson, nil, nil
	}

	// 删除 ss_config，不需要在最终的 shadowtls 配置中
	delete(inboundData, "ss_config")

	// 清理空的 handshake_for_server_name
	if hfsn, ok := inboundData["handshake_for_server_name"].(map[string]interface{}); ok && len(hfsn) == 0 {
		delete(inboundData, "handshake_for_server_name")
	}

	// 清理空的或 "off" 的 wildcard_sni
	if wSni, ok := inboundData["wildcard_sni"].(string); ok && (wSni == "" || wSni == "off") {
		delete(inboundData, "wildcard_sni")
	}

	// 安全获取 tag
	tag, ok := inboundData["tag"].(string)
	if !ok || tag == "" {
		// 没有 tag，返回原始配置（删除 ss_config 后）
		shadowtlsJson, err := json.Marshal(inboundData)
		if err != nil {
			return nil, nil, err
		}
		return shadowtlsJson, nil, nil
	}

	// 生成内部 shadowsocks 入站的 tag
	ssTag := tag + "-in"

	// 设置 shadowtls 的 detour 指向内部 shadowsocks
	inboundData["detour"] = ssTag

	// 生成 shadowtls 入站配置
	shadowtlsJson, err := json.Marshal(inboundData)
	if err != nil {
		return nil, nil, err
	}

	// 生成内部 shadowsocks 入站配置
	// password 直接使用 ss_config.password，不使用 users 多用户模式
	ssInbound := map[string]interface{}{
		"type":   "shadowsocks",
		"tag":    ssTag,
		"listen": "127.0.0.1",
	}

	// 按顺序添加字段: network, method, password, multiplex
	if network, ok := ssConfig["network"]; ok && network != nil && network != "" {
		ssInbound["network"] = network
	}
	if method, ok := ssConfig["method"]; ok && method != nil {
		ssInbound["method"] = method
	}
	if password, ok := ssConfig["password"]; ok && password != nil {
		ssInbound["password"] = password
	}

	// 添加多路复用配置（仅保留服务端需要的字段：enabled, padding, brutal）
	if multiplex, ok := ssConfig["multiplex"].(map[string]interface{}); ok && multiplex != nil {
		serverMux := map[string]interface{}{}
		if enabled, ok := multiplex["enabled"]; ok {
			serverMux["enabled"] = enabled
		}
		if padding, ok := multiplex["padding"]; ok {
			serverMux["padding"] = padding
		}
		if brutal, ok := multiplex["brutal"]; ok {
			serverMux["brutal"] = brutal
		}
		ssInbound["multiplex"] = serverMux
	}

	ssInboundJson, err := json.Marshal(ssInbound)
	if err != nil {
		return nil, nil, err
	}

	// 注意：不调用 addUsers，password 直接来自 ss_config.password

	return shadowtlsJson, ssInboundJson, nil
}

func (s *InboundService) hasUser(inboundType string) bool {
	switch inboundType {
	case "mixed", "socks", "http", "shadowsocks", "vmess", "trojan", "naive", "hysteria", "shadowtls", "tuic", "hysteria2", "vless", "anytls":
		return true
	}
	return false
}

func (s *InboundService) fetchUsers(db *gorm.DB, inboundType string, condition string, inbound map[string]interface{}) ([]json.RawMessage, error) {
	if inboundType == "shadowtls" {
		version, _ := inbound["version"].(float64)
		if int(version) < 3 {
			return nil, nil
		}
	}
	if inboundType == "shadowsocks" {
		method, _ := inbound["method"].(string)
		if method == "2022-blake3-aes-128-gcm" {
			inboundType = "shadowsocks16"
		}
	}

	var users []string

	err := db.Raw(
		fmt.Sprintf(`SELECT json_extract(clients.config, "$.%s")
		FROM clients WHERE enable = true AND %s`,
			inboundType, condition)).Scan(&users).Error
	if err != nil {
		return nil, err
	}

	usersJSON, err := normalizeSingboxUsersForList(inboundType, users, inbound["tls"] != nil)
	if err != nil {
		return nil, err
	}
	return usersJSON, nil
}

func (s *InboundService) addUsers(db *gorm.DB, inboundJson []byte, inboundId uint, inboundType string) ([]byte, error) {
	if !s.hasUser(inboundType) {
		return inboundJson, nil
	}

	// 屏蔽 shadowsocks 的 users 字段（shadowsocks 使用 password 而非 users 数组）
	if inboundType == "shadowsocks" {
		return inboundJson, nil
	}

	var inbound map[string]interface{}
	err := json.Unmarshal(inboundJson, &inbound)
	if err != nil {
		return nil, err
	}

	condition := fmt.Sprintf("%d IN (SELECT json_each.value FROM json_each(clients.inbounds))", inboundId)
	users, err := s.fetchUsers(db, inboundType, condition, inbound)
	if err != nil {
		return nil, err
	}

	// 只有获取到用户时才设置 users 字段
	// 避免 ShadowTLS v1/v2 等不需要 users 的入站出现 "users": null
	if users != nil {
		inbound["users"] = users
	}

	return json.Marshal(inbound)
}

func (s *InboundService) initUsers(db *gorm.DB, inboundJson []byte, clientIds string, inboundType string) ([]byte, error) {
	ClientIds := strings.Split(clientIds, ",")
	if len(ClientIds) == 0 {
		return inboundJson, nil
	}

	if !s.hasUser(inboundType) {
		return inboundJson, nil
	}

	// 屏蔽 shadowsocks 的 users 字段（shadowsocks 使用 password 而非 users 数组）
	if inboundType == "shadowsocks" {
		return inboundJson, nil
	}

	var inbound map[string]interface{}
	err := json.Unmarshal(inboundJson, &inbound)
	if err != nil {
		return nil, err
	}

	condition := fmt.Sprintf("id IN (%s)", strings.Join(ClientIds, ","))
	users, err := s.fetchUsers(db, inboundType, condition, inbound)
	if err != nil {
		return nil, err
	}

	// 只有获取到用户时才设置 users 字段
	// 避免 ShadowTLS v1/v2 等不需要 users 的入站出现 "users": null
	if users != nil {
		inbound["users"] = users
	}

	return json.Marshal(inbound)
}

func (s *InboundService) RestartInbounds(tx *gorm.DB, ids []uint) error {
	if !corePtr.IsRunning() {
		return nil
	}
	var inbounds []*model.Inbound
	err := tx.Model(model.Inbound{}).Preload("Tls").Where("id in ?", ids).Find(&inbounds).Error
	if err != nil {
		return err
	}
	for _, inbound := range inbounds {
		err = corePtr.RemoveInbound(inbound.Tag)
		if err != nil && err != os.ErrInvalid {
			return err
		}
		// Close all existing connections
		corePtr.GetInstance().ConnTracker().CloseConnByInbound(inbound.Tag)

		inboundConfig, err := inbound.MarshalJSON()
		if err != nil {
			return err
		}
		inboundConfig, err = s.addUsers(tx, inboundConfig, inbound.Id, inbound.Type)
		if err != nil {
			return err
		}
		err = corePtr.AddInbound(inboundConfig)
		if err != nil {
			return err
		}
	}
	return nil
}
