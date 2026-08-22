package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
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
	result := make([]map[string]interface{}, 0, len(inbound))
	err := db.Model(model.MihomoInbound{}).Where("id in ?", strings.Split(ids, ",")).Scan(&inbound).Error
	if err != nil {
		return nil, err
	}
	for _, inb := range inbound {
		if !isSupportedMihomoInboundType(inb.Type) {
			continue
		}
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

	result := make([]map[string]interface{}, 0, len(inbounds))
	for _, inbound := range inbounds {
		if !isSupportedMihomoInboundType(inbound.Type) {
			continue
		}
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

	selectableInboundIDs := make([]uint, 0, len(inbounds))
	for _, inbound := range inbounds {
		if !isSupportedMihomoInboundType(inbound.Type) {
			continue
		}
		selectableInboundIDs = append(selectableInboundIDs, inbound.Id)
	}
	usersByInboundID, err := loadMihomoInboundUsers(db, selectableInboundIDs)
	if err != nil {
		return nil, err
	}

	// The panel store distinguishes an empty list from a missing/malformed
	// response. Keep the JSON contract stable after the last inbound is deleted.
	data := make([]map[string]interface{}, 0, len(inbounds))
	for _, inbound := range inbounds {
		if !isSupportedMihomoInboundType(inbound.Type) {
			continue
		}
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
			if strings.EqualFold(inbound.Type, "hysteria2") {
				if rangeValue := extractPortHopRange(inbound.Options); rangeValue != "" {
					inbData["port_hop_range"] = rangeValue
				}
			}
		}
		userManagement := attachMihomoInboundUserManagementView(inbData, inbound)
		if userManagement.Selectable {
			inbData["users"] = usersByInboundID[inbound.Id]
		}
		data = append(data, inbData)
	}

	return &data, nil
}

func loadMihomoInboundUsers(db *gorm.DB, inboundIDs []uint) (map[uint][]string, error) {
	result := make(map[uint][]string, len(inboundIDs))
	if db == nil || len(inboundIDs) == 0 {
		return result, nil
	}

	type clientBindings struct {
		Name     string
		Inbounds json.RawMessage
	}
	var clients []clientBindings
	if err := db.Model(&model.MihomoClient{}).Select("name, inbounds").Find(&clients).Error; err != nil {
		return nil, err
	}
	wanted := make(map[uint]struct{}, len(inboundIDs))
	for _, id := range inboundIDs {
		wanted[id] = struct{}{}
		result[id] = []string{}
	}
	for _, client := range clients {
		name := strings.TrimSpace(client.Name)
		if name == "" {
			continue
		}
		for _, inboundID := range parseMihomoInboundIDs(client.Inbounds) {
			if _, ok := wanted[inboundID]; ok {
				result[inboundID] = append(result[inboundID], name)
			}
		}
	}
	for inboundID := range result {
		sort.Strings(result[inboundID])
	}
	return result, nil
}

func (s *MihomoInboundService) Save(tx *gorm.DB, act string, data json.RawMessage, initUserIDs string, hostname string) (*InboundNftAction, error) {
	var nftAction *InboundNftAction

	switch act {
	case "new", "edit":
		var inbound model.MihomoInbound
		if err := inbound.UnmarshalJSON(data); err != nil {
			return nil, err
		}
		inbound.Type = strings.ToLower(strings.TrimSpace(inbound.Type))
		oldTag := ""
		if act == "new" {
			// A copied create payload must never reuse an existing primary key.
			// GORM Save treats a non-zero key as an update.
			inbound.Id = 0
		} else if act == "edit" {
			if inbound.Id == 0 {
				return nil, common.NewError("mihomo inbound id is required for edit")
			}
			if tx == nil {
				return nil, common.NewError("mihomo inbound transaction is nil")
			}
			loaded := &model.MihomoInbound{}
			if err := tx.Model(model.MihomoInbound{}).Select("id", "tag", "type", "options").Where("id = ?", inbound.Id).First(loaded).Error; err != nil {
				return nil, err
			}
			if err := validateMihomoInboundTypeChangeClientBindings(tx, loaded, &inbound); err != nil {
				return nil, err
			}
			oldTag = loaded.Tag
		}
		if strings.EqualFold(strings.TrimSpace(inbound.Type), "shadowtls") {
			return nil, fmt.Errorf("mihomo ShadowTLS must be configured as a Shadowsocks inbound with mihomo TLS mode")
		}
		if isRemovedMihomoInboundType(inbound.Type) {
			return nil, fmt.Errorf("mihomo does not support Hysteria v1 inbound")
		}
		if !isSupportedMihomoInboundType(inbound.Type) {
			return nil, fmt.Errorf("mihomo does not support %s inbound", strings.TrimSpace(inbound.Type))
		}
		if err := validateMihomoInboundPayload(data, &inbound); err != nil {
			return nil, err
		}
		if err := sanitizeMihomoHysteria2PortHop(&inbound); err != nil {
			return nil, err
		}
		if _, err := sanitizeMihomoMieruInboundPortRange(&inbound); err != nil {
			return nil, err
		}
		if err := sanitizeMihomoShadowQUICInboundOptions(&inbound); err != nil {
			return nil, err
		}
		if err := validateMihomoShadowQUICJLSUpstreamProxy(tx, &inbound); err != nil {
			return nil, err
		}
		if err := validateMihomoSnellInitBindings(tx, &inbound, parseIDList(initUserIDs)); err != nil {
			return nil, err
		}
		if _, err := synchronizeMihomoSudokuBindings(tx, nil, []*model.MihomoInbound{&inbound}, parseIDList(initUserIDs)); err != nil {
			return nil, err
		}
		if inbound.TlsId > 0 {
			if tx == nil {
				return nil, fmt.Errorf("mihomo inbound TLS validation requires a database transaction")
			}
			if err := tx.Model(model.MihomoTls{}).Where("id = ?", inbound.TlsId).Take(&inbound.Tls).Error; err != nil {
				return nil, err
			}
			if err := validateMihomoInboundTLSMode(&inbound); err != nil {
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
		if err := validateMihomoStoredInboundReferences(tx); err != nil {
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
		if err := validateMihomoStoredInboundReferences(tx); err != nil {
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

// validateMihomoInboundPayload applies the final API boundary checks that
// cannot safely be delegated to model.Inbound.UnmarshalJSON. That model uses
// map[string]interface{} and therefore turns JSON numbers into float64; this
// helper inspects the original raw field so values such as 443.5 can never be
// truncated by a later toInt conversion.
func validateMihomoInboundPayload(data json.RawMessage, inbound *model.MihomoInbound) error {
	if inbound == nil {
		return fmt.Errorf("mihomo inbound is required")
	}

	inbound.Tag = strings.TrimSpace(inbound.Tag)
	if inbound.Tag == "" {
		return fmt.Errorf("mihomo inbound tag is required")
	}

	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return fmt.Errorf("invalid Mihomo inbound payload: %w", err)
	}

	portRaw, hasPort := raw["listen_port"]
	if !strings.EqualFold(inbound.Type, "tun") && hasPort {
		if _, err := parseStrictMihomoListenPort(portRaw); err != nil {
			return fmt.Errorf("mihomo %s listen_port: %w", inbound.Type, err)
		}
	} else if !strings.EqualFold(inbound.Type, "tun") {
		return fmt.Errorf("mihomo %s listen_port is required", inbound.Type)
	}

	if mihomoInboundRequiresTLS(inbound.Type) && inbound.TlsId == 0 {
		return fmt.Errorf("mihomo %s inbound requires a TLS configuration", inbound.Type)
	}

	return nil
}

func mihomoInboundRequiresTLS(inboundType string) bool {
	switch strings.ToLower(strings.TrimSpace(inboundType)) {
	case "anytls", "hysteria2", "trusttunnel", "tuic":
		return true
	default:
		return false
	}
}

func parseStrictMihomoListenPort(raw json.RawMessage) (int, error) {
	value := bytes.TrimSpace(raw)
	if len(value) == 0 || bytes.Equal(value, []byte("null")) {
		return 0, fmt.Errorf("must be an integer between 1 and 65535")
	}

	text := string(value)
	if value[0] == '"' {
		var quoted string
		if err := json.Unmarshal(value, &quoted); err != nil {
			return 0, fmt.Errorf("must be an integer between 1 and 65535")
		}
		text = strings.TrimSpace(quoted)
	}
	if text == "" {
		return 0, fmt.Errorf("must be an integer between 1 and 65535")
	}
	for _, char := range text {
		if char < '0' || char > '9' {
			return 0, fmt.Errorf("must be a complete decimal integer")
		}
	}

	parsed, err := strconv.ParseUint(text, 10, 16)
	if err != nil || parsed < 1 || parsed > 65535 {
		return 0, fmt.Errorf("must be an integer between 1 and 65535")
	}
	return int(parsed), nil
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
	return s.updateOutJSONsForLoadedInbounds(tx, inbounds, hostname)
}

func (s *MihomoInboundService) updateOutJSONsForLoadedInbounds(tx *gorm.DB, inbounds []model.MihomoInbound, hostname string) error {
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
		if !isSupportedMihomoInboundType(inbound.Type) {
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

		if inbound.Type == "snell" {
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

	var clients []model.MihomoClient
	if err := db.Model(model.MihomoClient{}).
		Select("name", "config", "inbounds").
		Where("enable = ?", true).
		Find(&clients).Error; err != nil {
		return "", err
	}
	clients = filterMihomoClientsBoundToInboundIDs(clients, []uint{inboundID})

	unique := map[string]struct{}{}
	for _, client := range clients {
		rawConfig := strings.TrimSpace(string(client.Config))
		if rawConfig == "" || strings.EqualFold(rawConfig, "null") {
			continue
		}

		config := map[string]json.RawMessage{}
		if err := json.Unmarshal(client.Config, &config); err != nil {
			return "", fmt.Errorf("parse mihomo snell client %q config failed: %w", client.Name, err)
		}
		rawUser := strings.TrimSpace(string(config["snell"]))
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
	case "mixed", "socks", "http", "snell", "vmess", "trojan", "naive", "hysteria", "shadowquic", "tuic", "hysteria2", "vless", "anytls", "mieru", "sudoku", "trusttunnel":
		return true
	}
	return false
}

func (s *MihomoInboundService) fetchUsers(db *gorm.DB, inboundType string, condition string, inbound map[string]interface{}) (interface{}, error) {
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

	return normalizeMihomoFetchedUsers(inboundType, users, inbound)
}

func (s *MihomoInboundService) fetchUsersForInbound(db *gorm.DB, inboundType string, inboundID uint, inbound map[string]interface{}) (interface{}, error) {
	if inboundID == 0 {
		return nil, nil
	}
	if inboundType == "shadowsocks" {
		method, _ := inbound["method"].(string)
		if method == "2022-blake3-aes-128-gcm" {
			inboundType = "shadowsocks16"
		}
	}

	var clients []model.MihomoClient
	if err := db.Model(model.MihomoClient{}).
		Select("config", "inbounds").
		Where("enable = ?", true).
		Find(&clients).Error; err != nil {
		return nil, err
	}
	clients = filterMihomoClientsBoundToInboundIDs(clients, []uint{inboundID})

	users := make([]string, 0, len(clients))
	for _, client := range clients {
		rawConfig := strings.TrimSpace(string(client.Config))
		if rawConfig == "" || strings.EqualFold(rawConfig, "null") {
			continue
		}

		config := map[string]json.RawMessage{}
		if err := json.Unmarshal(client.Config, &config); err != nil {
			return nil, fmt.Errorf("parse mihomo client config failed: %w", err)
		}
		rawUser := strings.TrimSpace(string(config[inboundType]))
		if rawUser == "" || strings.EqualFold(rawUser, "null") {
			continue
		}
		users = append(users, rawUser)
	}

	return normalizeMihomoFetchedUsers(inboundType, users, inbound)
}

func normalizeMihomoFetchedUsers(inboundType string, users []string, inbound map[string]interface{}) (interface{}, error) {
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
	// Runtime users are derived exclusively from bound Mihomo clients. This
	// also prevents legacy Options or view metadata from reaching server.yaml.
	delete(inbound, "users")

	users, err := s.fetchUsersForInbound(db, inboundType, inboundID, inbound)
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
	delete(inbound, "users")

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
		case "mixed", "socks", "http":
			username := strings.TrimSpace(firstString(user["username"]))
			if username == "" {
				username = strings.TrimSpace(firstString(user["name"]))
			}
			password := strings.TrimSpace(firstString(user["password"]))
			if username == "" || password == "" {
				return nil, fmt.Errorf("mihomo %s user missing username/password", inboundType)
			}
			// Proxy-auth listeners accept only this credential pair. Rebuild the
			// entry so client-side metadata or legacy aliases cannot leak into YAML.
			user = map[string]interface{}{
				"username": username,
				"password": password,
			}
		case "vmess", "vless", "trojan":
			username := strings.TrimSpace(firstString(user["username"]))
			if username == "" {
				username = strings.TrimSpace(firstString(user["name"]))
			}

			switch inboundType {
			case "vmess":
				uuid := strings.TrimSpace(firstString(user["uuid"]))
				if uuid == "" {
					return nil, fmt.Errorf("mihomo vmess user missing uuid")
				}
				normalizedUser := map[string]interface{}{"uuid": uuid}
				if username != "" {
					normalizedUser["username"] = username
				}
				if alterID, ok := toInt(firstNonNil(user["alterId"], user["alter_id"])); ok && alterID >= 0 {
					normalizedUser["alterId"] = alterID
				}
				user = normalizedUser
			case "vless":
				uuid := strings.TrimSpace(firstString(user["uuid"]))
				if uuid == "" {
					return nil, fmt.Errorf("mihomo vless user missing uuid")
				}
				normalizedUser := map[string]interface{}{"uuid": uuid}
				if username != "" {
					normalizedUser["username"] = username
				}
				if flow := strings.TrimSpace(firstString(user["flow"])); flow != "" && inbound["tls"] != nil {
					normalizedUser["flow"] = flow
				}
				user = normalizedUser
			case "trojan":
				password := strings.TrimSpace(firstString(user["password"]))
				if password == "" {
					return nil, fmt.Errorf("mihomo trojan user missing password")
				}
				normalizedUser := map[string]interface{}{"password": password}
				if username != "" {
					normalizedUser["username"] = username
				}
				user = normalizedUser
			}
		case "shadowquic":
			username := strings.TrimSpace(firstString(user["username"]))
			if username == "" {
				username = strings.TrimSpace(firstString(user["name"]))
			}
			password := strings.TrimSpace(firstString(user["password"]))
			if username == "" || password == "" {
				return nil, fmt.Errorf("mihomo %s user missing username/password", inboundType)
			}
			// The Mihomo listener schema only accepts these two user fields.
			// Rebuild instead of deleting selected legacy keys so a future UI
			// addition cannot accidentally reach the runtime users list.
			user = map[string]interface{}{
				"username": username,
				"password": password,
			}
		case "trusttunnel":
			username, password := util.ResolveTrustTunnelCredentials(user)
			if username == "" || password == "" {
				return nil, fmt.Errorf("mihomo %s user missing username/password", inboundType)
			}
			user = map[string]interface{}{
				"username": username,
				"password": password,
			}
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
