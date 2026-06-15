package service

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/util"
	"github.com/alireza0/s-ui/util/common"
	"gorm.io/gorm"
)

type MihomoInboundService struct {
	MihomoClientService
}

func (s *MihomoInboundService) Get(ids string) (*[]map[string]interface{}, error) {
	if ids == "" {
		return s.GetAll()
	}
	return s.getById(ids)
}

func (s *MihomoInboundService) getById(ids string) (*[]map[string]interface{}, error) {
	db := database.GetDB()
	var inbound []model.MihomoInbound
	var result []map[string]interface{}
	err := db.Model(model.MihomoInbound{}).Where("id in ?", strings.Split(ids, ",")).Scan(&inbound).Error
	if err != nil {
		return nil, err
	}
	for _, inb := range inbound {
		inbData, err := inb.MarshalFull()
		if err != nil {
			return nil, err
		}
		view := *inbData
		attachMihomoInboundUserManagementView(view, inb)
		result = append(result, view)
	}
	return &result, nil
}

func (s *MihomoInboundService) GetOutJsonIPs(ids string) ([]map[string]interface{}, error) {
	db := database.GetDB()
	var inbounds []model.MihomoInbound

	if ids == "" {
		if err := db.Model(model.MihomoInbound{}).Find(&inbounds).Error; err != nil {
			return nil, err
		}
	} else {
		if err := db.Model(model.MihomoInbound{}).Where("id in ?", strings.Split(ids, ",")).Find(&inbounds).Error; err != nil {
			return nil, err
		}
	}

	var result []map[string]interface{}
	for _, inbound := range inbounds {
		if len(inbound.OutJson) < 5 {
			continue
		}
		var outJson map[string]interface{}
		if err := json.Unmarshal(inbound.OutJson, &outJson); err != nil {
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

func (s *MihomoInboundService) GetAll() (*[]map[string]interface{}, error) {
	db := database.GetDB()
	inbounds := []model.MihomoInbound{}
	err := db.Model(model.MihomoInbound{}).Scan(&inbounds).Error
	if err != nil {
		return nil, err
	}

	var data []map[string]interface{}
	for _, inbound := range inbounds {
		routeTag := deriveEffectiveMihomoInboundRouteTagFromRaw(inbound.Tag, inbound.Type, inbound.Options)
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
		}
		userManagement := attachMihomoInboundUserManagementView(inbData, inbound)
		if userManagement.Selectable {
			users := []string{}
			err = db.Raw("SELECT mihomo_clients.name FROM mihomo_clients, json_each(mihomo_clients.inbounds) as je WHERE je.value = ?", inbound.Id).Scan(&users).Error
			if err != nil {
				return nil, err
			}
			inbData["users"] = users
		}
		data = append(data, inbData)
	}

	return &data, nil
}

func (s *MihomoInboundService) Save(tx *gorm.DB, act string, data json.RawMessage, initUserIDs string, hostname string) (*InboundNftAction, error) {
	var nftAction *InboundNftAction

	switch act {
	case "new", "edit":
		var inbound model.MihomoInbound
		if err := inbound.UnmarshalJSON(data); err != nil {
			return nil, err
		}
		if _, err := sanitizeMihomoMieruInboundPortRange(&inbound); err != nil {
			return nil, err
		}
		sanitizeMihomoShadowTLSInboundOptions(&inbound)
		if err := validateMihomoSnellInitBindings(tx, &inbound, parseIDList(initUserIDs)); err != nil {
			return nil, err
		}
		if _, err := synchronizeMihomoSudokuBindings(tx, nil, []*model.MihomoInbound{&inbound}, parseIDList(initUserIDs)); err != nil {
			return nil, err
		}
		if inbound.TlsId > 0 {
			if err := tx.Model(model.MihomoTls{}).Where("id = ?", inbound.TlsId).Find(&inbound.Tls).Error; err != nil {
				return nil, err
			}
		}

		oldTag := ""
		if act == "edit" {
			if err := tx.Model(model.MihomoInbound{}).Select("tag").Where("id = ?", inbound.Id).Find(&oldTag).Error; err != nil {
				return nil, err
			}
		}

		if strings.EqualFold(inbound.Type, "vless") {
			fullInbound, err := inbound.MarshalFull()
			if err != nil {
				return nil, err
			}
			if fullInbound != nil {
				if err := util.ValidateVLESSMihomoEncryptionSource(*fullInbound); err != nil {
					return nil, err
				}
			}
		}

		if err := fillMihomoOutJson(&inbound, hostname); err != nil {
			return nil, err
		}
		if err := tx.Save(&inbound).Error; err != nil {
			return nil, err
		}

		switch act {
		case "new":
			if err := s.MihomoClientService.UpdateClientsOnInboundAdd(tx, initUserIDs, inbound.Id, hostname); err != nil {
				return nil, err
			}
		case "edit":
			if err := s.MihomoClientService.UpdateLinksByInboundChange(tx, &[]model.MihomoInbound{inbound}, hostname, oldTag); err != nil {
				return nil, err
			}
		}
		redirectRange, redirectTCP := resolveMihomoInboundRedirectSpec(&inbound)
		nftAction = &InboundNftAction{
			Kind:         "upsert",
			InboundID:    inbound.Id,
			Tag:          inbound.Tag,
			Port:         extractPort(inbound.Options),
			PortHopRange: redirectRange,
			RedirectTCP:  redirectTCP,
		}
	case "del":
		var tag string
		if err := json.Unmarshal(data, &tag); err != nil {
			return nil, err
		}
		var id uint
		if err := tx.Model(model.MihomoInbound{}).Select("id").Where("tag = ?", tag).Scan(&id).Error; err != nil {
			return nil, err
		}
		if err := s.MihomoClientService.UpdateClientsOnInboundDelete(tx, id, tag); err != nil {
			return nil, err
		}
		var syncSvc SyncService
		if err := syncSvc.CleanupSubOutboundsByInboundID(tx, subOutboundSourceMihomoClient, id); err != nil {
			return nil, err
		}
		if err := tx.Where("tag = ?", tag).Delete(model.MihomoInbound{}).Error; err != nil {
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

func validateMihomoSnellInitBindings(tx *gorm.DB, inbound *model.MihomoInbound, initUserIDs []uint) error {
	if tx == nil || inbound == nil || !strings.EqualFold(strings.TrimSpace(inbound.Type), "snell") {
		return nil
	}

	initUserIDs = dedupeUintIDs(initUserIDs)
	if len(initUserIDs) <= 1 {
		return nil
	}

	return fmt.Errorf("snell inbound can bind only one user")
}

func (s *MihomoInboundService) UpdateOutJsons(tx *gorm.DB, inboundIDs []uint, hostname string) error {
	var inbounds []model.MihomoInbound
	err := tx.Model(model.MihomoInbound{}).Preload("Tls").Where("id in ?", inboundIDs).Find(&inbounds).Error
	if err != nil {
		return err
	}
	for _, inbound := range inbounds {
		current := inbound
		if err := fillMihomoOutJson(&current, effectiveOutJSONHostname(current.OutJson, hostname)); err != nil {
			return err
		}
		if err := tx.Model(model.MihomoInbound{}).Where("tag = ?", current.Tag).Update("out_json", current.OutJson).Error; err != nil {
			return err
		}
	}
	return nil
}

func (s *MihomoInboundService) GetAllConfig(db *gorm.DB) ([]json.RawMessage, error) {
	var inboundsJSON []json.RawMessage
	var inbounds []*model.MihomoInbound
	err := db.Model(model.MihomoInbound{}).Preload("Tls").Find(&inbounds).Error
	if err != nil {
		return nil, err
	}

	for _, inbound := range inbounds {
		if inbound.Type == "ssh" {
			continue
		}
		inboundJSON, err := inbound.MarshalJSON()
		if err != nil {
			return nil, err
		}
		inboundJSON, err = s.addUsers(db, inboundJSON, inbound.Id, inbound.Type)
		if err != nil {
			return nil, err
		}

	if inbound.Type == "shadowtls" {
		shadowtlsJSON, ssJSON, err := s.processShadowTLSInbound(db, inboundJSON, inbound)
		if err != nil {
			return nil, err
		}
		inboundsJSON = append(inboundsJSON, shadowtlsJSON)
		if ssJSON != nil {
			inboundsJSON = append(inboundsJSON, ssJSON)
		}
	} else if inbound.Type == "snell" {
		snellJSON, err := s.processSnellInbound(db, inboundJSON, inbound)
		if err != nil {
			return nil, err
		}
		inboundsJSON = append(inboundsJSON, snellJSON)
	} else {
		inboundsJSON = append(inboundsJSON, inboundJSON)
	}
	}

	return inboundsJSON, nil
}

func (s *MihomoInboundService) processShadowTLSInbound(db *gorm.DB, inboundJSON []byte, inbound *model.MihomoInbound) (json.RawMessage, json.RawMessage, error) {
	var inboundData map[string]interface{}
	if err := json.Unmarshal(inboundJSON, &inboundData); err != nil {
		return nil, nil, err
	}

	ssConfig, hasSSConfig := inboundData["ss_config"].(map[string]interface{})
	if !hasSSConfig || ssConfig == nil {
		return inboundJSON, nil, nil
	}

	delete(inboundData, "ss_config")
	if handshake, ok := inboundData["handshake"].(map[string]interface{}); ok && handshake != nil {
		delete(handshake, "proxy")
		delete(handshake, "detour")
	}
	delete(inboundData, "handshake_for_server_name")
	delete(inboundData, "strict_mode")
	delete(inboundData, "wildcard_sni")

	tag, ok := inboundData["tag"].(string)
	if !ok || tag == "" {
		shadowtlsJSON, err := json.Marshal(inboundData)
		if err != nil {
			return nil, nil, err
		}
		return shadowtlsJSON, nil, nil
	}

	ssTag := tag + "-in"
	inboundData["detour"] = ssTag

	shadowtlsJSON, err := json.Marshal(inboundData)
	if err != nil {
		return nil, nil, err
	}

	ssInbound := map[string]interface{}{
		"type":   "shadowsocks",
		"tag":    ssTag,
		"listen": "127.0.0.1",
	}
	if method, ok := ssConfig["method"]; ok && method != nil {
		ssInbound["method"] = method
	}
	if password, ok := ssConfig["password"]; ok && password != nil {
		ssInbound["password"] = password
	}
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

	ssInboundJSON, err := json.Marshal(ssInbound)
	if err != nil {
		return nil, nil, err
	}

	return shadowtlsJSON, ssInboundJSON, nil
}

func sanitizeMihomoShadowTLSInboundOptions(inbound *model.MihomoInbound) {
	if inbound == nil || !strings.EqualFold(strings.TrimSpace(inbound.Type), "shadowtls") {
		return
	}
	if len(inbound.Options) == 0 {
		return
	}

	var options map[string]interface{}
	if err := json.Unmarshal(inbound.Options, &options); err != nil || options == nil {
		return
	}

	delete(options, "detour")
	delete(options, "tcp_fast_open")
	delete(options, "tcp_multi_path")
	delete(options, "udp_fragment")
	delete(options, "udp_timeout")
	delete(options, "strict_mode")
	delete(options, "wildcard_sni")
	delete(options, "handshake_for_server_name")

	if handshake, ok := options["handshake"].(map[string]interface{}); ok && handshake != nil {
		delete(handshake, "proxy")
		delete(handshake, "detour")
	}
	if ssConfig, ok := options["ss_config"].(map[string]interface{}); ok && ssConfig != nil {
		delete(ssConfig, "network")
	}

	sanitized, err := json.Marshal(options)
	if err != nil {
		return
	}
	inbound.Options = json.RawMessage(sanitized)
}

func (s *MihomoInboundService) processSnellInbound(db *gorm.DB, inboundJSON []byte, inbound *model.MihomoInbound) (json.RawMessage, error) {
	var inboundData map[string]interface{}
	if err := json.Unmarshal(inboundJSON, &inboundData); err != nil {
		return nil, err
	}

	psk, err := s.resolveSnellSharedPSK(db, inbound.Id)
	if err != nil {
		return nil, err
	}
	inboundData["psk"] = psk

	version, ok := toInt(inboundData["version"])
	if !ok || version < 4 || version > 5 {
		inboundData["version"] = 5
	}

	if udp, ok := toBool(inboundData["udp"]); ok {
		inboundData["udp"] = udp
	} else {
		inboundData["udp"] = true
	}

	if obfsOpts, ok := inboundData["obfs_opts"].(map[string]interface{}); ok && obfsOpts != nil {
		mode := strings.TrimSpace(firstString(obfsOpts["mode"]))
		if mode == "" {
			delete(inboundData, "obfs_opts")
		} else {
			host := strings.TrimSpace(firstString(obfsOpts["host"]))
			if host == "" {
				host = "www.bing.com"
			}
			inboundData["obfs_opts"] = map[string]interface{}{
				"mode": mode,
				"host": host,
			}
		}
	}

	return json.Marshal(inboundData)
}

func (s *MihomoInboundService) resolveSnellSharedPSK(db *gorm.DB, inboundID uint) (string, error) {
	if inboundID == 0 {
		return "", fmt.Errorf("snell inbound missing id")
	}

	var users []string
	err := db.Raw(
		`SELECT json_extract(mihomo_clients.config, "$.snell")
		FROM mihomo_clients
		WHERE enable = true
		  AND EXISTS (SELECT 1 FROM json_each(mihomo_clients.inbounds) WHERE json_each.value = ?)`,
		inboundID,
	).Scan(&users).Error
	if err != nil {
		return "", err
	}

	unique := map[string]struct{}{}
	for _, rawUser := range users {
		rawUser = strings.TrimSpace(rawUser)
		if rawUser == "" || strings.EqualFold(rawUser, "null") {
			continue
		}

		var user map[string]interface{}
		if err := json.Unmarshal([]byte(rawUser), &user); err != nil {
			return "", fmt.Errorf("parse mihomo snell user failed: %w", err)
		}
		psk := strings.TrimSpace(firstString(user["psk"]))
		if psk == "" {
			continue
		}
		unique[psk] = struct{}{}
	}

	switch len(unique) {
	case 0:
		return "", fmt.Errorf("snell inbound has no bound client psk")
	case 1:
		for psk := range unique {
			return psk, nil
		}
	}

	return "", fmt.Errorf("snell inbound has multiple different client psk values")
}

func (s *MihomoInboundService) hasUser(inboundType string) bool {
	switch inboundType {
	case "mixed", "socks", "http", "snell", "vmess", "trojan", "naive", "hysteria", "shadowtls", "tuic", "hysteria2", "vless", "anytls", "mieru", "sudoku", "trusttunnel":
		return true
	}
	return false
}

func (s *MihomoInboundService) fetchUsers(db *gorm.DB, inboundType string, condition string, inbound map[string]interface{}) (interface{}, error) {
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
		fmt.Sprintf(`SELECT json_extract(mihomo_clients.config, "$.%s")
		FROM mihomo_clients WHERE enable = true AND %s`,
			inboundType, condition)).Scan(&users).Error
	if err != nil {
		return nil, err
	}

	switch inboundType {
	case "anytls", "hysteria2":
		return normalizeMihomoUsersForMap(inboundType, users, []string{"username", "name"})
	case "tuic":
		return normalizeMihomoUsersForMap(inboundType, users, []string{"uuid"})
	case "mieru":
		return normalizeMihomoUsersForMap(inboundType, users, []string{"username", "name"})
	default:
		return normalizeMihomoUsersForList(inboundType, users, inbound)
	}
}

func (s *MihomoInboundService) addUsers(db *gorm.DB, inboundJSON []byte, inboundID uint, inboundType string) ([]byte, error) {
	if !s.hasUser(inboundType) || inboundType == "shadowsocks" || inboundType == "sudoku" || inboundType == "snell" {
		return inboundJSON, nil
	}

	var inbound map[string]interface{}
	if err := json.Unmarshal(inboundJSON, &inbound); err != nil {
		return nil, err
	}

	condition := fmt.Sprintf("%d IN (SELECT json_each.value FROM json_each(mihomo_clients.inbounds))", inboundID)
	users, err := s.fetchUsers(db, inboundType, condition, inbound)
	if err != nil {
		return nil, err
	}
	if users != nil {
		inbound["users"] = users
	}

	return json.Marshal(inbound)
}

func (s *MihomoInboundService) initUsers(db *gorm.DB, inboundJSON []byte, clientIDs string, inboundType string) ([]byte, error) {
	if strings.TrimSpace(clientIDs) == "" {
		return inboundJSON, nil
	}
	if !s.hasUser(inboundType) || inboundType == "shadowsocks" || inboundType == "sudoku" || inboundType == "snell" {
		return inboundJSON, nil
	}

	clientIDList := strings.Split(clientIDs, ",")
	var inbound map[string]interface{}
	if err := json.Unmarshal(inboundJSON, &inbound); err != nil {
		return nil, err
	}

	condition := fmt.Sprintf("id IN (%s)", strings.Join(clientIDList, ","))
	users, err := s.fetchUsers(db, inboundType, condition, inbound)
	if err != nil {
		return nil, err
	}
	if users != nil {
		inbound["users"] = users
	}

	return json.Marshal(inbound)
}

func normalizeMihomoUsersForList(inboundType string, users []string, inbound map[string]interface{}) ([]json.RawMessage, error) {
	usersJSON := make([]json.RawMessage, 0, len(users))
	for _, rawUser := range users {
		rawUser = strings.TrimSpace(rawUser)
		if rawUser == "" || strings.EqualFold(rawUser, "null") {
			continue
		}

		var user map[string]interface{}
		if err := json.Unmarshal([]byte(rawUser), &user); err != nil {
			return nil, fmt.Errorf("parse mihomo %s user failed: %w", inboundType, err)
		}

		switch inboundType {
		case "vmess", "vless", "trojan":
			if username := strings.TrimSpace(firstString(user["username"])); username == "" {
				if legacyName := strings.TrimSpace(firstString(user["name"])); legacyName != "" {
					user["username"] = legacyName
				}
			}
			delete(user, "name")
		case "trusttunnel":
			username, password := util.ResolveTrustTunnelCredentials(user)
			if username == "" || password == "" {
				return nil, fmt.Errorf("mihomo %s user missing username/password", inboundType)
			}
			user["username"] = username
			user["password"] = password
			delete(user, "name")
			delete(user, "uuid")
		}

		if inboundType == "vless" && inbound["tls"] == nil {
			delete(user, "flow")
		}

		normalized, err := json.Marshal(user)
		if err != nil {
			return nil, fmt.Errorf("marshal mihomo %s user failed: %w", inboundType, err)
		}
		usersJSON = append(usersJSON, json.RawMessage(normalized))
	}

	if len(usersJSON) == 0 {
		return nil, nil
	}
	return usersJSON, nil
}

func normalizeMihomoUsersForMap(inboundType string, users []string, identityKeys []string) (map[string]string, error) {
	usersMap := make(map[string]string, len(users))
	for _, rawUser := range users {
		rawUser = strings.TrimSpace(rawUser)
		if rawUser == "" || strings.EqualFold(rawUser, "null") {
			continue
		}

		var user map[string]interface{}
		if err := json.Unmarshal([]byte(rawUser), &user); err != nil {
			return nil, fmt.Errorf("parse mihomo %s user failed: %w", inboundType, err)
		}

		identity := ""
		for _, key := range identityKeys {
			identity = strings.TrimSpace(firstString(user[key]))
			if identity != "" {
				break
			}
		}
		if identity == "" {
			return nil, fmt.Errorf("mihomo %s user missing identity field", inboundType)
		}

		password := strings.TrimSpace(firstString(user["password"]))
		if password == "" {
			return nil, fmt.Errorf("mihomo %s user %q missing password", inboundType, identity)
		}

		usersMap[identity] = password
	}

	if len(usersMap) == 0 {
		return nil, nil
	}
	return usersMap, nil
}
