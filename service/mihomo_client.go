package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/logger"
	"github.com/alireza0/s-ui/util"
	"github.com/alireza0/s-ui/util/common"
	"gorm.io/gorm"
)

type MihomoClientService struct {
	MihomoNftTrafficService
}

func normalizeMihomoClientArrayFields(client *model.MihomoClient) {
	if client == nil {
		return
	}
	if len(bytes.TrimSpace(client.Inbounds)) == 0 || bytes.Equal(bytes.TrimSpace(client.Inbounds), []byte("null")) {
		client.Inbounds = json.RawMessage("[]")
	}
	if len(bytes.TrimSpace(client.Links)) == 0 || bytes.Equal(bytes.TrimSpace(client.Links), []byte("null")) {
		client.Links = json.RawMessage("[]")
	}
}

func (s *MihomoClientService) Get(id string) (*[]model.MihomoClient, error) {
	if id == "" {
		return s.GetAll()
	}
	return s.getById(id)
}

func (s *MihomoClientService) getById(id string) (*[]model.MihomoClient, error) {
	db := database.GetDB()
	clients := make([]model.MihomoClient, 0)
	err := db.Model(model.MihomoClient{}).Where("id in ?", strings.Split(id, ",")).Scan(&clients).Error
	if err != nil {
		return nil, err
	}
	autoSyncIDs, err := (&SettingService{}).GetSubManagerAutoSyncMihomoClientIDs()
	if err != nil {
		return nil, err
	}
	autoSyncSet := make(map[uint]struct{}, len(autoSyncIDs))
	for _, id := range autoSyncIDs {
		autoSyncSet[id] = struct{}{}
	}
	for i := range clients {
		normalizeMihomoClientArrayFields(&clients[i])
		_, clients[i].AutoSync = autoSyncSet[clients[i].Id]
	}
	return &clients, nil
}

func (s *MihomoClientService) GetAll() (*[]model.MihomoClient, error) {
	db := database.GetDB()
	clients := make([]model.MihomoClient, 0)
	err := db.Model(model.MihomoClient{}).
		Select("`id`, `enable`, `name`, `desc`, `group`, `inbounds`, `up`, `down`, `volume`, `expiry`, `speed_limit_mbps`").
		Order("id ASC").
		Scan(&clients).Error
	if err != nil {
		return nil, err
	}
	autoSyncIDs, err := (&SettingService{}).GetSubManagerAutoSyncMihomoClientIDs()
	if err != nil {
		return nil, err
	}
	autoSyncSet := make(map[uint]struct{}, len(autoSyncIDs))
	for _, id := range autoSyncIDs {
		autoSyncSet[id] = struct{}{}
	}
	for i := range clients {
		normalizeMihomoClientArrayFields(&clients[i])
		_, clients[i].AutoSync = autoSyncSet[clients[i].Id]
	}
	return &clients, nil
}

func (s *MihomoClientService) Save(tx *gorm.DB, act string, data json.RawMessage, hostname string) ([]uint, error) {
	var (
		err        error
		inboundIDs []uint
	)

	switch act {
	case "new", "edit":
		var client model.MihomoClient
		if err = json.Unmarshal(data, &client); err != nil {
			return nil, err
		}
		if act == "new" {
			client.Id = 0
		} else if client.Id == 0 {
			return nil, common.NewError("mihomo client id is required for edit")
		}
		normalizeMihomoClientArrayFields(&client)
		editingID := uint(0)
		if act == "edit" {
			editingID = client.Id
		}
		if err = validateMihomoClientNames(tx, []*model.MihomoClient{&client}, editingID); err != nil {
			return nil, err
		}
		clientInboundSelections, normalizeErr := normalizeMihomoClientInboundSelections([]*model.MihomoClient{&client})
		if normalizeErr != nil {
			return nil, normalizeErr
		}
		clientInboundIDs := clientInboundSelections[0]
		if err = validateMihomoClientInboundTargets(tx, clientInboundIDs); err != nil {
			return nil, err
		}
		client.Volume = normalizeClientVolume(client.Volume)
		client.Expiry = normalizeClientExpiry(client.Expiry)
		client.Extra = normalizeClientResetDay(client.Extra)
		if client.Up < 0 {
			client.Up = 0
		}
		if client.Down < 0 {
			client.Down = 0
		}
		client.ServerIp = util.NormalizeSubscriptionServerHost(client.ServerIp)
		if _, err = synchronizeMihomoSudokuBindings(tx, []*model.MihomoClient{&client}, nil, nil); err != nil {
			return nil, err
		}
		if _, err = ensureMihomoShadowQUICCredentialsForClientBindings(tx, &client); err != nil {
			return nil, err
		}
		if err = validateMihomoSnellClientBindings(tx, &client); err != nil {
			return nil, err
		}
		if _, err = s.updateLinksForClients(tx, []*model.MihomoClient{&client}, hostname); err != nil {
			return nil, err
		}
		var oldClient *model.MihomoClient
		if act == "edit" {
			inboundIDs, err = s.findInboundsChanges(tx, client)
			if err != nil {
				return nil, err
			}
			var previousClient model.MihomoClient
			if txErr := tx.Model(model.MihomoClient{}).Where("id = ?", client.Id).First(&previousClient).Error; txErr == nil {
				oldClient = &previousClient
			} else {
				return nil, txErr
			}
		} else {
			inboundIDs = clientInboundIDs
		}
		nowUnix := time.Now().Unix()
		manualTrafficReset := client.TrafficResetRequested && act == "edit"
		if oldClient != nil {
			client.Depleted = oldClient.Depleted
			if manualTrafficReset {
				client.Up = 0
				client.Down = 0
			} else {
				client.Up = oldClient.Up
				client.Down = oldClient.Down
			}

			if oldClient.Extra != client.Extra {
				client.LastReset = nowUnix
			} else {
				client.LastReset = oldClient.LastReset
			}

			evaluation := evaluateClientAccess(true, client.Up+client.Down, client.Volume, client.Expiry, nowUnix)
			if oldClient.Depleted {
				if evaluation.Blocked {
					client.Enable = false
					client.Depleted = true
				} else {
					client.Enable = true
					client.Depleted = false
				}
			} else {
				client.Depleted = false
			}
			if oldClient.Enable != client.Enable {
				oldInboundIDs, parseErr := parseClientInboundIDs(oldClient.Inbounds)
				if parseErr != nil {
					return nil, parseErr
				}
				inboundIDs = common.UnionUintArray(oldInboundIDs, clientInboundIDs)
			}
		}
		if act == "new" && client.LastReset == 0 {
			client.LastReset = nowUnix
		}
		client.SpeedLimitMbps = normalizeClientSpeedLimitMbps(client.SpeedLimitMbps)
		if err = tx.Save(&client).Error; err != nil {
			return nil, err
		}

		if queueErr := s.MihomoNftTrafficService.QueueSyncClientBindings(tx, client.Id, clientInboundIDs); queueErr != nil {
			logger.Warning("failed to queue mihomo client traffic binding sync for ", client.Name, ": ", queueErr)
		}

		if manualTrafficReset {
			if queueErr := s.MihomoNftTrafficService.QueueClientTrafficReset(tx, client.Id); queueErr != nil {
				logger.Warning("failed to queue mihomo client nft traffic reset for ", client.Name, ": ", queueErr)
			}
		}
	case "addbulk":
		var clients []*model.MihomoClient
		if err = json.Unmarshal(data, &clients); err != nil {
			return nil, err
		}
		if len(clients) == 0 {
			return nil, nil
		}
		if err = validateMihomoClientNames(tx, clients, 0); err != nil {
			return nil, err
		}
		clientInboundSelections, normalizeErr := normalizeMihomoClientInboundSelections(clients)
		if normalizeErr != nil {
			return nil, normalizeErr
		}
		for _, selection := range clientInboundSelections {
			inboundIDs = mergeInboundIDs(inboundIDs, selection)
		}
		if err = validateMihomoClientInboundTargets(tx, inboundIDs); err != nil {
			return nil, err
		}
		for _, client := range clients {
			if client == nil {
				continue
			}
			client.Id = 0
			client.ServerIp = util.NormalizeSubscriptionServerHost(client.ServerIp)
		}
		if _, err = synchronizeMihomoSudokuBindings(tx, clients, nil, nil); err != nil {
			return nil, err
		}
		for _, client := range clients {
			if _, err = ensureMihomoShadowQUICCredentialsForClientBindings(tx, client); err != nil {
				return nil, err
			}
		}
		if err = validateMihomoSnellClientBindingsBatch(tx, clients); err != nil {
			return nil, err
		}
		if _, err = s.updateLinksForClients(tx, clients, hostname); err != nil {
			return nil, err
		}
		nowUnix := time.Now().Unix()
		for _, client := range clients {
			if client == nil {
				continue
			}
			client.Volume = normalizeClientVolume(client.Volume)
			client.Expiry = normalizeClientExpiry(client.Expiry)
			client.Extra = normalizeClientResetDay(client.Extra)
			if client.Up < 0 {
				client.Up = 0
			}
			if client.Down < 0 {
				client.Down = 0
			}
			if client.LastReset == 0 {
				client.LastReset = nowUnix
			}
			client.SpeedLimitMbps = normalizeClientSpeedLimitMbps(client.SpeedLimitMbps)
		}
		if err = tx.Save(clients).Error; err != nil {
			return nil, err
		}

		for index, client := range clients {
			if queueErr := s.MihomoNftTrafficService.QueueSyncClientBindings(tx, client.Id, clientInboundSelections[index]); queueErr != nil {
				logger.Warning("failed to queue mihomo client traffic binding sync for ", client.Name, ": ", queueErr)
			}
		}
	case "del":
		var id uint
		if err = json.Unmarshal(data, &id); err != nil {
			return nil, err
		}
		var client model.MihomoClient
		if err = tx.Where("id = ?", id).First(&client).Error; err != nil {
			return nil, err
		}
		if inboundIDs, err = parseClientInboundIDs(client.Inbounds); err != nil {
			return nil, err
		}
		if delErr := s.MihomoNftTrafficService.DeleteClientBindings(tx, id); delErr != nil {
			logger.Warning("failed to delete mihomo client traffic bindings for client id ", id, ": ", delErr)
		}
		if err = tx.Where("id = ?", id).Delete(model.MihomoClient{}).Error; err != nil {
			return nil, err
		}
	default:
		return nil, common.NewErrorf("unknown action: %s", act)
	}

	return inboundIDs, nil
}

func normalizeMihomoClientInboundSelections(clients []*model.MihomoClient) ([][]uint, error) {
	selections := make([][]uint, len(clients))
	for index, client := range clients {
		if client == nil {
			return nil, common.NewError("mihomo client is nil")
		}
		inboundIDs, err := parseClientInboundIDs(client.Inbounds)
		if err != nil {
			return nil, err
		}
		encoded, err := json.Marshal(inboundIDs)
		if err != nil {
			return nil, err
		}
		client.Inbounds = encoded
		selections[index] = inboundIDs
	}
	return selections, nil
}

// validateMihomoClientInboundTargets makes the database enforce the same
// binding boundary as the client-page selector. This also covers a stale
// client editor whose selected inbound was deleted in another window.
func validateMihomoClientInboundTargets(tx *gorm.DB, inboundIDs []uint) error {
	if tx == nil {
		return common.NewError("mihomo client transaction is nil")
	}
	inboundIDs = dedupeUintIDs(inboundIDs)
	if len(inboundIDs) == 0 {
		return nil
	}

	var inbounds []model.MihomoInbound
	if err := tx.Model(model.MihomoInbound{}).
		Select("id", "tag", "type", "options").
		Where("id IN ?", inboundIDs).
		Find(&inbounds).Error; err != nil {
		return err
	}
	byID := make(map[uint]model.MihomoInbound, len(inbounds))
	for _, inbound := range inbounds {
		byID[inbound.Id] = inbound
	}
	for _, inboundID := range inboundIDs {
		if _, exists := byID[inboundID]; !exists {
			return fmt.Errorf("mihomo inbound id %d does not exist", inboundID)
		}
	}
	for _, inboundID := range inboundIDs {
		inbound := byID[inboundID]
		if !isSupportedMihomoInboundType(inbound.Type) {
			// These are known listener types without user authentication. Keep
			// the historical binding error for direct-like records, while still
			// distinguishing genuinely unsupported legacy protocols (for example
			// SSH) from a non-selectable listener.
			switch strings.ToLower(strings.TrimSpace(inbound.Type)) {
			case "direct", "redirect", "tproxy", "tun":
				return fmt.Errorf("mihomo inbound %q (%s) cannot be assigned to a client", inbound.Tag, inbound.Type)
			}
			return fmt.Errorf("mihomo inbound %q (%s) is not supported by Mihomo", inbound.Tag, inbound.Type)
		}
		if buildMihomoInboundUserManagementFromOptions(inbound.Type, inbound.Options).Selectable {
			continue
		}
		return fmt.Errorf("mihomo inbound %q (%s) cannot be assigned to a client", inbound.Tag, inbound.Type)
	}
	return nil
}

// validateMihomoInboundTypeChangeClientBindings rejects an edit that would
// leave existing client bindings pointing at a listener without user support.
// Clearing those bindings must remain an explicit action in client management.
func validateMihomoInboundTypeChangeClientBindings(tx *gorm.DB, existing, updated *model.MihomoInbound) error {
	if tx == nil || existing == nil || updated == nil || existing.Id == 0 {
		return nil
	}
	oldManagement := buildMihomoInboundUserManagementFromOptions(existing.Type, existing.Options)
	newManagement := buildMihomoInboundUserManagementFromOptions(updated.Type, updated.Options)
	if !oldManagement.Selectable || newManagement.Selectable {
		return nil
	}

	var clients []model.MihomoClient
	if err := tx.Model(model.MihomoClient{}).Select("id", "name", "inbounds").Find(&clients).Error; err != nil {
		return err
	}
	boundClients := filterMihomoClientsBoundToInboundIDs(clients, []uint{existing.Id})
	if len(boundClients) == 0 {
		return nil
	}
	names := make([]string, 0, len(boundClients))
	for _, client := range boundClients {
		name := strings.TrimSpace(client.Name)
		if name != "" {
			names = append(names, name)
		}
	}
	if len(names) == 0 {
		names = append(names, fmt.Sprintf("id=%d", existing.Id))
	}
	return fmt.Errorf("mihomo inbound %q still has bound clients (%s); remove those bindings before changing it to %s", existing.Tag, strings.Join(names, ", "), updated.Type)
}

// filterMihomoClientsBoundToInboundIDs keeps only clients whose stored
// selection strictly contains at least one requested inbound ID. Historical
// string IDs are accepted by parseClientInboundIDs, but lossy SQLite CAST
// matching must never turn values such as "12x" into a binding for ID 12.
func filterMihomoClientsBoundToInboundIDs(clients []model.MihomoClient, inboundIDs []uint) []model.MihomoClient {
	if len(clients) == 0 || len(inboundIDs) == 0 {
		return []model.MihomoClient{}
	}

	wanted := make(map[uint]struct{}, len(inboundIDs))
	for _, inboundID := range inboundIDs {
		if inboundID > 0 {
			wanted[inboundID] = struct{}{}
		}
	}
	if len(wanted) == 0 {
		return []model.MihomoClient{}
	}

	matched := make([]model.MihomoClient, 0, len(clients))
	for _, client := range clients {
		clientInboundIDs, err := parseClientInboundIDs(client.Inbounds)
		if err != nil {
			continue
		}
		for _, inboundID := range clientInboundIDs {
			if _, exists := wanted[inboundID]; exists {
				matched = append(matched, client)
				break
			}
		}
	}

	return matched
}

func validateMihomoClientNames(tx *gorm.DB, clients []*model.MihomoClient, editingID uint) error {
	if tx == nil {
		return common.NewError("mihomo client transaction is nil")
	}

	seen := make(map[string]struct{}, len(clients))
	for _, client := range clients {
		if client == nil {
			return common.NewError("mihomo client is nil")
		}
		client.Name = strings.TrimSpace(client.Name)
		if client.Name == "" {
			return common.NewError("mihomo client name is required")
		}
		if _, ok := seen[client.Name]; ok {
			return common.NewErrorf("mihomo client name %q is duplicated in this request", client.Name)
		}
		seen[client.Name] = struct{}{}
	}

	for name := range seen {
		query := tx.Model(&model.MihomoClient{}).Where("name = ?", name)
		if editingID > 0 {
			query = query.Where("id <> ?", editingID)
		}
		var count int64
		if err := query.Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return common.NewErrorf("mihomo client name %q already exists", name)
		}
	}
	return nil
}

func validateMihomoSnellClientBindingsBatch(tx *gorm.DB, clients []*model.MihomoClient) error {
	claimedInboundOwner := map[uint]string{}
	for _, client := range clients {
		if client == nil {
			continue
		}
		inboundIDs, err := parseClientInboundIDs(client.Inbounds)
		if err != nil {
			return err
		}
		if len(inboundIDs) > 0 {
			var snellInboundIDs []uint
			if err := tx.Model(model.MihomoInbound{}).
				Where("id in ? AND type = ?", inboundIDs, "snell").
				Pluck("id", &snellInboundIDs).Error; err != nil {
				return err
			}
			for _, inboundID := range snellInboundIDs {
				if owner, exists := claimedInboundOwner[inboundID]; exists && owner != client.Name {
					return fmt.Errorf("snell inbound id %d can bind only one user", inboundID)
				}
				claimedInboundOwner[inboundID] = client.Name
			}
		}
		if err := validateMihomoSnellClientBindings(tx, client); err != nil {
			return err
		}
	}
	return nil
}

func validateMihomoSnellClientBindings(tx *gorm.DB, client *model.MihomoClient) error {
	if tx == nil || client == nil {
		return nil
	}

	inboundIDs, err := parseClientInboundIDs(client.Inbounds)
	if err != nil {
		return err
	}
	if len(inboundIDs) == 0 {
		return nil
	}

	var snellInbounds []model.MihomoInbound
	if err := tx.Model(model.MihomoInbound{}).
		Select("id", "tag", "type").
		Where("id in ? AND type = ?", inboundIDs, "snell").
		Find(&snellInbounds).Error; err != nil {
		return err
	}
	if len(snellInbounds) == 0 {
		return nil
	}

	var existingClients []model.MihomoClient
	if err := tx.Model(model.MihomoClient{}).
		Select("id", "inbounds").
		Where("id <> ?", client.Id).
		Find(&existingClients).Error; err != nil {
		return err
	}

	for _, inbound := range snellInbounds {
		if len(filterMihomoClientsBoundToInboundIDs(existingClients, []uint{inbound.Id})) > 0 {
			return fmt.Errorf("snell inbound %s can bind only one user", inbound.Tag)
		}
	}

	return nil
}

func (s *MihomoClientService) updateLinksForClients(tx *gorm.DB, clients []*model.MihomoClient, hostname string) ([]uint, error) {
	if len(clients) == 0 {
		return nil, nil
	}

	clientInboundIDs := make([][]uint, len(clients))
	inboundIDs := make([]uint, 0)
	for index, client := range clients {
		if client == nil {
			return nil, common.NewError("mihomo client is nil")
		}
		ids, err := parseClientInboundIDs(client.Inbounds)
		if err != nil {
			return nil, err
		}
		clientInboundIDs[index] = ids
		inboundIDs = mergeInboundIDs(inboundIDs, ids)
	}

	var inbounds []model.MihomoInbound
	if len(inboundIDs) > 0 {
		err := tx.Model(model.MihomoInbound{}).
			Preload("Tls").
			Where("id in ? and type in ?", inboundIDs, util.InboundTypeWithLink).
			Find(&inbounds).Error
		if err != nil {
			return nil, err
		}
	}
	inboundByID := make(map[uint]model.MihomoInbound, len(inbounds))
	for _, inbound := range inbounds {
		inboundByID[inbound.Id] = inbound
	}

	for index, client := range clients {
		selectedInbounds := make([]model.MihomoInbound, 0, len(clientInboundIDs[index]))
		for _, inboundID := range clientInboundIDs[index] {
			if inbound, exists := inboundByID[inboundID]; exists {
				selectedInbounds = append(selectedInbounds, inbound)
			}
		}
		var clientLinks []map[string]string
		if err := json.Unmarshal(client.Links, &clientLinks); err != nil {
			return nil, err
		}

		newClientLinks := []map[string]string{}
		for _, inbound := range selectedInbounds {
			base := inbound.ToBase()
			serverHost := util.ResolveSubscriptionServerHost(client.ServerIp, &base, hostname)
			newLinks := util.LinkGenerator(client.Config, &base, serverHost)
			for _, newLink := range newLinks {
				newClientLinks = append(newClientLinks, map[string]string{
					"remark": inbound.Tag,
					"type":   "local",
					"uri":    newLink,
				})
			}
		}

		for _, clientLink := range clientLinks {
			if clientLink["type"] != "local" {
				newClientLinks = append(newClientLinks, clientLink)
			}
		}

		links, err := json.MarshalIndent(newClientLinks, "", "  ")
		if err != nil {
			return nil, err
		}
		clients[index].Links = links
	}

	return inboundIDs, nil
}

func (s *MihomoClientService) UpdateClientsOnInboundAdd(tx *gorm.DB, initIDs string, inboundID uint, hostname string) error {
	clientIDs := strings.Split(initIDs, ",")
	var clients []model.MihomoClient
	if err := tx.Model(model.MihomoClient{}).Where("id in ?", clientIDs).Find(&clients).Error; err != nil {
		return err
	}

	var inbound model.MihomoInbound
	if err := tx.Model(model.MihomoInbound{}).Preload("Tls").Where("id = ?", inboundID).Find(&inbound).Error; err != nil {
		return err
	}
	base := inbound.ToBase()

	for _, client := range clients {
		clientInbounds, err := parseClientInboundIDs(client.Inbounds)
		if err != nil {
			return err
		}
		clientInbounds = mergeInboundIDs(clientInbounds, []uint{inboundID})
		inboundsRaw, err := json.MarshalIndent(clientInbounds, "", "  ")
		if err != nil {
			return err
		}
		client.Inbounds = inboundsRaw
		if strings.EqualFold(strings.TrimSpace(inbound.Type), "shadowquic") {
			if _, err := ensureMihomoShadowQUICClientCredentials(&client); err != nil {
				return err
			}
		}

		var clientLinks, newClientLinks []map[string]string
		_ = json.Unmarshal(client.Links, &clientLinks)
		serverHost := util.ResolveSubscriptionServerHost(client.ServerIp, &base, hostname)
		newLinks := util.LinkGenerator(client.Config, &base, serverHost)
		for _, newLink := range newLinks {
			newClientLinks = append(newClientLinks, map[string]string{
				"remark": inbound.Tag,
				"type":   "local",
				"uri":    newLink,
			})
		}
		for _, clientLink := range clientLinks {
			if clientLink["type"] != "local" || clientLink["remark"] != inbound.Tag {
				newClientLinks = append(newClientLinks, clientLink)
			}
		}

		linksRaw, err := json.MarshalIndent(newClientLinks, "", "  ")
		if err != nil {
			return err
		}
		client.Links = linksRaw
		if err := tx.Save(&client).Error; err != nil {
			return err
		}
		if queueErr := s.MihomoNftTrafficService.QueueSyncClientBindings(tx, client.Id, clientInbounds); queueErr != nil {
			logger.Warning("failed to queue mihomo client traffic binding sync for ", client.Name, " after inbound add: ", queueErr)
		}
	}

	return nil
}

func (s *MihomoClientService) UpdateClientsOnInboundDelete(tx *gorm.DB, id uint, tag string) error {
	var clients []model.MihomoClient
	err := tx.Table("mihomo_clients").Find(&clients).Error
	if err != nil {
		return err
	}
	clients = filterMihomoClientsBoundToInboundIDs(clients, []uint{id})

	for _, client := range clients {
		clientInbounds, err := parseClientInboundIDs(client.Inbounds)
		if err != nil {
			return err
		}
		newClientInbounds := make([]uint, 0, len(clientInbounds))
		for _, clientInbound := range clientInbounds {
			if clientInbound != id {
				newClientInbounds = append(newClientInbounds, clientInbound)
			}
		}
		inboundsRaw, err := json.MarshalIndent(newClientInbounds, "", "  ")
		if err != nil {
			return err
		}
		client.Inbounds = inboundsRaw

		var clientLinks, newClientLinks []map[string]string
		_ = json.Unmarshal(client.Links, &clientLinks)
		for _, clientLink := range clientLinks {
			if clientLink["type"] != "local" || clientLink["remark"] != tag {
				newClientLinks = append(newClientLinks, clientLink)
			}
		}
		linksRaw, err := json.MarshalIndent(newClientLinks, "", "  ")
		if err != nil {
			return err
		}
		client.Links = linksRaw

		if err := tx.Save(&client).Error; err != nil {
			return err
		}
		if queueErr := s.MihomoNftTrafficService.QueueSyncClientBindings(tx, client.Id, newClientInbounds); queueErr != nil {
			logger.Warning("failed to queue mihomo client traffic binding sync for ", client.Name, " after inbound delete: ", queueErr)
		}
	}

	return nil
}

func (s *MihomoClientService) UpdateLinksByInboundChange(tx *gorm.DB, inbounds *[]model.MihomoInbound, hostname string, oldTag string) error {
	if inbounds == nil {
		return nil
	}

	inboundByID := make(map[uint]model.MihomoInbound, len(*inbounds))
	inboundIDs := make([]uint, 0, len(*inbounds))
	for _, inbound := range *inbounds {
		if inbound.Id == 0 {
			continue
		}
		if _, exists := inboundByID[inbound.Id]; exists {
			continue
		}
		inboundByID[inbound.Id] = inbound
		inboundIDs = append(inboundIDs, inbound.Id)
	}
	if len(inboundIDs) == 0 {
		return nil
	}

	var clients []model.MihomoClient
	if err := tx.Table("mihomo_clients").Find(&clients).Error; err != nil {
		return err
	}
	clients = filterMihomoClientsBoundToInboundIDs(clients, inboundIDs)

	for _, client := range clients {
		clientInboundIDs, err := parseClientInboundIDs(client.Inbounds)
		if err != nil {
			return err
		}
		linkedInbounds := make([]model.MihomoInbound, 0, len(clientInboundIDs))
		for _, inboundID := range clientInboundIDs {
			if inbound, exists := inboundByID[inboundID]; exists {
				linkedInbounds = append(linkedInbounds, inbound)
			}
		}
		if len(linkedInbounds) == 0 {
			continue
		}

		linkedTags := make(map[string]struct{}, len(linkedInbounds))
		for _, inbound := range linkedInbounds {
			linkedTags[inbound.Tag] = struct{}{}
		}
		originalConfig := append(json.RawMessage(nil), client.Config...)
		var clientLinks []map[string]string
		_ = json.Unmarshal(client.Links, &clientLinks)
		newClientLinks := make([]map[string]string, 0, len(clientLinks)+len(linkedInbounds))
		for _, clientLink := range clientLinks {
			if clientLink["type"] != "local" {
				newClientLinks = append(newClientLinks, clientLink)
				continue
			}
			if clientLink["remark"] == oldTag {
				continue
			}
			if _, replaced := linkedTags[clientLink["remark"]]; !replaced {
				newClientLinks = append(newClientLinks, clientLink)
			}
		}

		for _, inbound := range linkedInbounds {
			if strings.EqualFold(strings.TrimSpace(inbound.Type), "shadowquic") {
				if _, err := ensureMihomoShadowQUICClientCredentials(&client); err != nil {
					return err
				}
			}
			base := inbound.ToBase()
			serverHost := util.ResolveSubscriptionServerHost(client.ServerIp, &base, hostname)
			for _, newLink := range util.LinkGenerator(client.Config, &base, serverHost) {
				newClientLinks = append(newClientLinks, map[string]string{
					"remark": inbound.Tag,
					"type":   "local",
					"uri":    newLink,
				})
			}
		}

		linksRaw, err := json.MarshalIndent(newClientLinks, "", "  ")
		if err != nil {
			return err
		}
		if bytes.Equal(client.Links, linksRaw) && bytes.Equal(client.Config, originalConfig) {
			continue
		}
		updates := map[string]interface{}{"links": linksRaw}
		if !bytes.Equal(client.Config, originalConfig) {
			updates["config"] = client.Config
		}
		if err := tx.Model(model.MihomoClient{}).Where("id = ?", client.Id).Updates(updates).Error; err != nil {
			return err
		}
	}

	return nil
}

// ResetTrafficBySchedule resets mihomo client traffic by configured monthly reset days.
func (s *MihomoClientService) ResetTrafficBySchedule() (bool, error) {
	db := database.GetDB()
	now := PanelNow()

	var clients []model.MihomoClient
	err := db.Model(model.MihomoClient{}).
		Where("extra > 0").
		Find(&clients).Error
	if err != nil {
		return false, err
	}
	if len(clients) == 0 {
		return false, nil
	}

	resetClients := make([]model.MihomoClient, 0, len(clients))
	for _, client := range clients {
		if shouldResetClientTrafficMonthly(client.LastReset, client.Extra, now) {
			resetClients = append(resetClients, client)
		}
	}
	if len(resetClients) == 0 {
		return false, nil
	}
	if err := FlushTrafficRuntimeJournal(); err != nil {
		return false, fmt.Errorf("scheduled Mihomo traffic reset requires traffic journal flush: %w", err)
	}

	changed := false
	for _, client := range resetClients {
		logger.Info("Resetting traffic for mihomo client ", client.Name, " (reset days: ", client.Extra, ")")
		if resetErr := s.MihomoNftTrafficService.ResetClientTraffic(db, client.Id); resetErr != nil {
			logger.Warning("failed to reset traffic for mihomo client ", client.Name, ": ", resetErr)
			continue
		}
		changed = true
		if client.Depleted && (client.Expiry <= 0 || client.Expiry > now.Unix()) {
			if err := db.Model(&model.MihomoClient{}).Where("id = ? AND depleted = ?", client.Id, true).Updates(map[string]interface{}{
				"enable":   true,
				"depleted": false,
			}).Error; err != nil {
				logger.Warning("failed to re-enable reset mihomo client ", client.Name, ": ", err)
			}
		}
	}
	return changed, nil
}

// DepleteClients disables mihomo clients that exceed volume or expiry.
// Returns changed inbound IDs so callers can restart corresponding inbounds.
func (s *MihomoClientService) DepleteClients() ([]uint, error) {
	now := time.Now().Unix()
	db := database.GetDB()

	tx := db.Begin()
	if tx.Error != nil {
		return nil, tx.Error
	}

	var clients []model.MihomoClient
	err := tx.Model(model.MihomoClient{}).
		Where("enable = true AND ((volume > 0 AND up + down >= volume) OR (expiry > 0 AND expiry <= ?))", now).
		Find(&clients).Error
	if err != nil {
		tx.Rollback()
		return nil, err
	}

	if len(clients) == 0 {
		if err = tx.Commit().Error; err != nil {
			tx.Rollback()
			return nil, err
		}
		return nil, nil
	}

	inboundIDs := make([]uint, 0)
	changes := make([]model.Changes, 0, len(clients))
	dt := time.Now().Unix()

	for _, client := range clients {
		logger.Debug("Mihomo client ", client.Name, " is going to be disabled")

		clientInbounds, parseErr := parseClientInboundIDs(client.Inbounds)
		if parseErr != nil {
			logger.Warning("skip malformed Mihomo inbound bindings while depleting client ", client.Name, ": ", parseErr)
			continue
		}
		inboundIDs = common.UnionUintArray(inboundIDs, clientInbounds)

		changes = append(changes, model.Changes{
			DateTime: dt,
			Actor:    "DepleteJob",
			Key:      "mihomo_clients",
			Action:   "disable",
			Obj:      json.RawMessage("\"" + client.Name + "\""),
		})
	}

	if err = tx.Model(model.MihomoClient{}).
		Where("enable = true AND ((volume > 0 AND up + down >= volume) OR (expiry > 0 AND expiry <= ?))", now).
		Updates(map[string]interface{}{"enable": false, "depleted": true}).Error; err != nil {
		tx.Rollback()
		return nil, err
	}

	if len(changes) > 0 {
		if err = recordChanges(tx, changes); err != nil {
			tx.Rollback()
			return nil, err
		}
	}

	if err = tx.Commit().Error; err != nil {
		tx.Rollback()
		return nil, err
	}
	if len(changes) > 0 {
		markMihomoLastUpdate(dt)
	}

	return inboundIDs, nil
}

func (s *MihomoClientService) findInboundsChanges(tx *gorm.DB, client model.MihomoClient) ([]uint, error) {
	var oldClient model.MihomoClient
	if err := tx.Model(model.MihomoClient{}).Where("id = ?", client.Id).First(&oldClient).Error; err != nil {
		return nil, err
	}

	oldInboundIDs, err := parseClientInboundIDs(oldClient.Inbounds)
	if err != nil {
		return nil, err
	}
	newInboundIDs, err := parseClientInboundIDs(client.Inbounds)
	if err != nil {
		return nil, err
	}

	if !bytes.Equal(oldClient.Config, client.Config) || oldClient.Name != client.Name || oldClient.Enable != client.Enable {
		return common.UnionUintArray(oldInboundIDs, newInboundIDs), nil
	}

	return common.DiffUintArray(oldInboundIDs, newInboundIDs), nil
}
