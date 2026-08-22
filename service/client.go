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

type ClientService struct {
	NftTrafficService
}

func normalizeClientArrayFields(client *model.Client) {
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

const maxClientBulkCount = 100

func (s *ClientService) Get(id string) (*[]model.Client, error) {
	if id == "" {
		return s.GetAll()
	}
	return s.getById(id)
}

func (s *ClientService) getById(id string) (*[]model.Client, error) {
	db := database.GetDB()
	client := make([]model.Client, 0)
	err := db.Model(model.Client{}).Where("id in ?", strings.Split(id, ",")).Scan(&client).Error
	if err != nil {
		return nil, err
	}
	for i := range client {
		normalizeClientArrayFields(&client[i])
	}

	return &client, nil
}

func (s *ClientService) GetAll() (*[]model.Client, error) {
	db := database.GetDB()
	clients := make([]model.Client, 0)
	err := db.Model(model.Client{}).
		Select("`id`, `enable`, `name`, `desc`, `group`, `inbounds`, `up`, `down`, `volume`, `expiry`, `speed_limit_mbps`").
		Order("id ASC").
		Scan(&clients).Error
	if err != nil {
		return nil, err
	}
	for i := range clients {
		normalizeClientArrayFields(&clients[i])
	}
	autoSyncIDs, err := (&SettingService{}).GetSubManagerAutoSyncClientIDs()
	if err != nil {
		return nil, err
	}
	autoSyncSet := make(map[uint]struct{}, len(autoSyncIDs))
	for _, id := range autoSyncIDs {
		autoSyncSet[id] = struct{}{}
	}
	for i := range clients {
		_, clients[i].AutoSync = autoSyncSet[clients[i].Id]
	}
	return &clients, nil
}

func (s *ClientService) Save(tx *gorm.DB, act string, data json.RawMessage, hostname string) ([]uint, error) {
	var err error
	var inboundIds []uint

	switch act {
	case "new", "edit":
		var client model.Client
		err = json.Unmarshal(data, &client)
		if err != nil {
			return nil, err
		}
		normalizeClientArrayFields(&client)
		if act == "new" {
			client.Id = 0
		} else if client.Id == 0 {
			return nil, common.NewError("client id is required for edit")
		}
		editingID := uint(0)
		if act == "edit" {
			editingID = client.Id
		}
		if err = validateClientNames(tx, []*model.Client{&client}, editingID); err != nil {
			return nil, err
		}
		clientInboundSelections, normalizeErr := normalizeClientInboundSelections([]*model.Client{&client})
		if normalizeErr != nil {
			return nil, normalizeErr
		}
		clientInboundIDs := clientInboundSelections[0]
		if err = validateClientInboundTargets(tx, clientInboundIDs); err != nil {
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
		err = s.updateLinksWithFixedInbounds(tx, []*model.Client{&client}, hostname)
		if err != nil {
			return nil, err
		}
		var oldClient *model.Client
		if act == "edit" {
			// Find changed inbounds
			inboundIds, err = s.findInboundsChanges(tx, client)
			if err != nil {
				return nil, err
			}
			var previousClient model.Client
			if txErr := tx.Model(model.Client{}).Where("id = ?", client.Id).First(&previousClient).Error; txErr == nil {
				oldClient = &previousClient
			} else {
				return nil, txErr
			}
		} else {
			inboundIds = clientInboundIDs
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
				var oldInboundIDs []uint
				var newInboundIDs []uint
				_ = json.Unmarshal(oldClient.Inbounds, &oldInboundIDs)
				_ = json.Unmarshal(client.Inbounds, &newInboundIDs)
				inboundIds = common.UnionUintArray(oldInboundIDs, newInboundIDs)
			}
		}
		if act == "new" && client.LastReset == 0 {
			client.LastReset = nowUnix
		}
		client.SpeedLimitMbps = normalizeClientSpeedLimitMbps(client.SpeedLimitMbps)

		err = tx.Save(&client).Error
		if err != nil {
			return nil, err
		}

		// Sync client-inbound traffic bindings (for nftables-based per-client stats)
		if queueErr := s.NftTrafficService.QueueSyncClientBindings(tx, client.Id, clientInboundIDs); queueErr != nil {
			logger.Warning("failed to queue client traffic binding sync for ", client.Name, ": ", queueErr)
		}

		// Manual reset keeps the current nft baselines aligned with the UI counters.
		if manualTrafficReset {
			if queueErr := s.NftTrafficService.QueueClientTrafficReset(tx, client.Id); queueErr != nil {
				logger.Warning("failed to queue client nft traffic reset for ", client.Name, ": ", queueErr)
			}
		}

	case "addbulk":
		var clients []*model.Client
		err = json.Unmarshal(data, &clients)
		if err != nil {
			return nil, err
		}
		if len(clients) == 0 {
			return nil, common.NewError("at least one client is required")
		}
		if len(clients) > maxClientBulkCount {
			return nil, common.NewErrorf("bulk client count must be between 1 and %d", maxClientBulkCount)
		}
		if err = validateClientNames(tx, clients, 0); err != nil {
			return nil, err
		}
		clientInboundSelections, normalizeErr := normalizeClientInboundSelections(clients)
		if normalizeErr != nil {
			return nil, normalizeErr
		}
		for _, selection := range clientInboundSelections {
			inboundIds = common.UnionUintArray(inboundIds, selection)
		}
		if err = validateClientInboundTargets(tx, inboundIds); err != nil {
			return nil, err
		}
		for _, client := range clients {
			normalizeClientArrayFields(client)
			client.Id = 0
			client.ServerIp = util.NormalizeSubscriptionServerHost(client.ServerIp)
		}
		err = s.updateLinksWithFixedInbounds(tx, clients, hostname)
		if err != nil {
			return nil, err
		}
		nowUnix := time.Now().Unix()
		for _, client := range clients {
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
		err = tx.Save(clients).Error
		if err != nil {
			return nil, err
		}

		// Sync traffic bindings for all bulk-added clients
		for index, client := range clients {
			if queueErr := s.NftTrafficService.QueueSyncClientBindings(tx, client.Id, clientInboundSelections[index]); queueErr != nil {
				logger.Warning("failed to queue client traffic binding sync for ", client.Name, ": ", queueErr)
			}
		}

	case "del":
		var id uint
		err = json.Unmarshal(data, &id)
		if err != nil {
			return nil, err
		}
		var client model.Client
		err = tx.Where("id = ?", id).First(&client).Error
		if err != nil {
			return nil, err
		}
		err = json.Unmarshal(client.Inbounds, &inboundIds)
		if err != nil {
			return nil, err
		}

		// Delete client traffic bindings before deleting the client
		if delErr := s.NftTrafficService.DeleteClientBindings(tx, id); delErr != nil {
			logger.Warning("failed to delete client traffic bindings for client id ", id, ": ", delErr)
		}

		err = tx.Where("id = ?", id).Delete(model.Client{}).Error
		if err != nil {
			return nil, err
		}
	default:
		return nil, common.NewErrorf("unknown action: %s", act)
	}

	return inboundIds, nil
}

func validateClientNames(tx *gorm.DB, clients []*model.Client, editingID uint) error {
	if tx == nil {
		return common.NewError("client transaction is nil")
	}

	seen := make(map[string]struct{}, len(clients))
	for _, client := range clients {
		if client == nil {
			return common.NewError("client is nil")
		}
		client.Name = strings.TrimSpace(client.Name)
		if client.Name == "" {
			return common.NewError("client name is required")
		}
		if _, ok := seen[client.Name]; ok {
			return common.NewErrorf("client name %q is duplicated in this request", client.Name)
		}
		seen[client.Name] = struct{}{}
	}

	for name := range seen {
		query := tx.Model(&model.Client{}).Where("name = ?", name)
		if editingID > 0 {
			query = query.Where("id <> ?", editingID)
		}
		var count int64
		if err := query.Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return common.NewErrorf("client name %q already exists", name)
		}
	}
	return nil
}

func normalizeClientInboundSelections(clients []*model.Client) ([][]uint, error) {
	selections := make([][]uint, len(clients))
	for index, client := range clients {
		if client == nil {
			return nil, common.NewError("client is nil")
		}
		inboundIDs, err := util.ParseInboundIDs(client.Inbounds)
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

// validateClientInboundTargets mirrors the client-page selector at the final
// database boundary. It blocks stale selections from another browser tab and
// direct API calls from persisting absent or non-selectable default inbounds.
func validateClientInboundTargets(tx *gorm.DB, inboundIDs []uint) error {
	if tx == nil {
		return common.NewError("client transaction is nil")
	}
	inboundIDs = dedupeUintIDs(inboundIDs)
	if len(inboundIDs) == 0 {
		return nil
	}

	var inbounds []model.Inbound
	if err := tx.Model(model.Inbound{}).
		Select("id", "tag", "type", "options").
		Where("id IN ?", inboundIDs).
		Find(&inbounds).Error; err != nil {
		return err
	}
	byID := make(map[uint]model.Inbound, len(inbounds))
	for _, inbound := range inbounds {
		byID[inbound.Id] = inbound
	}
	for _, inboundID := range inboundIDs {
		inbound, exists := byID[inboundID]
		if !exists {
			return fmt.Errorf("sing-box inbound id %d does not exist", inboundID)
		}
		shadowTLSVersion := uint(0)
		if inbound.Type == "shadowtls" {
			options := map[string]interface{}{}
			if json.Unmarshal(inbound.Options, &options) == nil {
				shadowTLSVersion = uint(util.ShadowTLSVersion(options["version"]))
			}
		}
		if !buildSingboxInboundUserManagement(inbound.Type, shadowTLSVersion).Selectable {
			return fmt.Errorf("sing-box inbound %q (%s) cannot be assigned to a client", inbound.Tag, inbound.Type)
		}
	}
	return nil
}

func (s *ClientService) updateLinksWithFixedInbounds(tx *gorm.DB, clients []*model.Client, hostname string) error {
	if len(clients) == 0 {
		return nil
	}

	clientInboundIDs, err := normalizeClientInboundSelections(clients)
	if err != nil {
		return err
	}
	inboundIDs := make([]uint, 0)
	for _, selection := range clientInboundIDs {
		inboundIDs = common.UnionUintArray(inboundIDs, selection)
	}

	var inbounds []model.Inbound
	if len(inboundIDs) > 0 {
		if err := tx.Model(model.Inbound{}).Preload("Tls").Where("id in ? and type in ?", inboundIDs, util.InboundTypeWithLink).Find(&inbounds).Error; err != nil {
			return err
		}
	}
	inboundByID := make(map[uint]model.Inbound, len(inbounds))
	for _, inbound := range inbounds {
		inboundByID[inbound.Id] = inbound
	}

	for index, client := range clients {
		selectedInbounds := make([]model.Inbound, 0, len(clientInboundIDs[index]))
		for _, inboundID := range clientInboundIDs[index] {
			if inbound, exists := inboundByID[inboundID]; exists {
				selectedInbounds = append(selectedInbounds, inbound)
			}
		}
		var clientLinks []map[string]string
		err = json.Unmarshal(client.Links, &clientLinks)
		if err != nil {
			return err
		}

		newClientLinks := []map[string]string{}
		for _, inbound := range selectedInbounds {
			serverHost := util.ResolveSubscriptionServerHost(client.ServerIp, &inbound, hostname)
			newLinks := util.LinkGenerator(client.Config, &inbound, serverHost)
			for _, newLink := range newLinks {
				newClientLinks = append(newClientLinks, map[string]string{
					"remark": inbound.Tag,
					"type":   "local",
					"uri":    newLink,
				})
			}
		}

		// Add non local links
		for _, clientLink := range clientLinks {
			if clientLink["type"] != "local" {
				newClientLinks = append(newClientLinks, clientLink)
			}
		}

		clients[index].Links, err = json.MarshalIndent(newClientLinks, "", "  ")
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *ClientService) UpdateClientsOnInboundAdd(tx *gorm.DB, initIds string, inboundId uint, hostname string) error {
	clientIds := strings.Split(initIds, ",")
	var clients []model.Client
	err := tx.Model(model.Client{}).Where("id in ?", clientIds).Find(&clients).Error
	if err != nil {
		return err
	}
	var inbound model.Inbound
	err = tx.Model(model.Inbound{}).Preload("Tls").Where("id = ?", inboundId).Find(&inbound).Error
	if err != nil {
		return err
	}
	for _, client := range clients {
		// Add inbounds
		var clientInbounds []uint
		json.Unmarshal(client.Inbounds, &clientInbounds)
		clientInbounds = append(clientInbounds, inboundId)
		client.Inbounds, err = json.MarshalIndent(clientInbounds, "", "  ")
		if err != nil {
			return err
		}
		// Add links
		var clientLinks, newClientLinks []map[string]string
		json.Unmarshal(client.Links, &clientLinks)
		serverHost := util.ResolveSubscriptionServerHost(client.ServerIp, &inbound, hostname)
		newLinks := util.LinkGenerator(client.Config, &inbound, serverHost)
		for _, newLink := range newLinks {
			newClientLinks = append(newClientLinks, map[string]string{
				"remark": inbound.Tag,
				"type":   "local",
				"uri":    newLink,
			})
		}
		for _, clientLink := range clientLinks {
			if clientLink["remark"] != inbound.Tag {
				newClientLinks = append(newClientLinks, clientLink)
			}
		}

		client.Links, err = json.MarshalIndent(newClientLinks, "", "  ")
		if err != nil {
			return err
		}
		err = tx.Save(&client).Error
		if err != nil {
			return err
		}
		if queueErr := s.NftTrafficService.QueueSyncClientBindings(tx, client.Id, clientInbounds); queueErr != nil {
			logger.Warning("failed to queue client traffic binding sync for ", client.Name, " after inbound add: ", queueErr)
		}
	}
	return nil
}

func (s *ClientService) UpdateClientsOnInboundDelete(tx *gorm.DB, id uint, tag string) error {
	var clients []model.Client
	err := tx.Table("clients").
		Where("EXISTS (SELECT 1 FROM json_each(clients.inbounds) WHERE json_each.value = ?)", id).
		Find(&clients).Error
	if err != nil {
		return err
	}
	for _, client := range clients {
		// Delete inbounds
		var clientInbounds, newClientInbounds []uint
		json.Unmarshal(client.Inbounds, &clientInbounds)
		for _, clientInbound := range clientInbounds {
			if clientInbound != id {
				newClientInbounds = append(newClientInbounds, clientInbound)
			}
		}
		client.Inbounds, err = json.MarshalIndent(newClientInbounds, "", "  ")
		if err != nil {
			return err
		}
		// Delete links
		var clientLinks, newClientLinks []map[string]string
		json.Unmarshal(client.Links, &clientLinks)
		for _, clientLink := range clientLinks {
			if clientLink["remark"] != tag {
				newClientLinks = append(newClientLinks, clientLink)
			}
		}
		client.Links, err = json.MarshalIndent(newClientLinks, "", "  ")
		if err != nil {
			return err
		}
		err = tx.Save(&client).Error
		if err != nil {
			return err
		}
		if queueErr := s.NftTrafficService.QueueSyncClientBindings(tx, client.Id, newClientInbounds); queueErr != nil {
			logger.Warning("failed to queue client traffic binding sync for ", client.Name, " after inbound delete: ", queueErr)
		}
	}
	return nil
}

func (s *ClientService) UpdateLinksByInboundChange(tx *gorm.DB, inbounds *[]model.Inbound, hostname string, oldTag string) error {
	var err error
	for _, inbound := range *inbounds {
		var clients []model.Client
		err = tx.Table("clients").
			Where("EXISTS (SELECT 1 FROM json_each(clients.inbounds) WHERE json_each.value = ?)", inbound.Id).
			Find(&clients).Error
		if err != nil {
			return err
		}
		for _, client := range clients {
			var clientLinks, newClientLinks []map[string]string
			json.Unmarshal(client.Links, &clientLinks)
			serverHost := util.ResolveSubscriptionServerHost(client.ServerIp, &inbound, hostname)
			newLinks := util.LinkGenerator(client.Config, &inbound, serverHost)
			for _, newLink := range newLinks {
				newClientLinks = append(newClientLinks, map[string]string{
					"remark": inbound.Tag,
					"type":   "local",
					"uri":    newLink,
				})
			}
			for _, clientLink := range clientLinks {
				if clientLink["type"] != "local" || (clientLink["remark"] != inbound.Tag && clientLink["remark"] != oldTag) {
					newClientLinks = append(newClientLinks, clientLink)
				}
			}

			client.Links, err = json.MarshalIndent(newClientLinks, "", "  ")
			if err != nil {
				return err
			}
			err = tx.Save(&client).Error
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// ResetTrafficBySchedule checks clients with Extra > 0 (traffic reset days)
// and resets their traffic when the configured monthly reset boundary is reached.
func (s *ClientService) ResetTrafficBySchedule() (bool, error) {
	db := database.GetDB()
	now := PanelNow()

	var clients []model.Client
	err := db.Model(model.Client{}).
		Where("extra > 0").
		Find(&clients).Error
	if err != nil {
		return false, err
	}

	if len(clients) == 0 {
		return false, nil
	}

	resetClients := make([]model.Client, 0, len(clients))
	for _, client := range clients {
		if shouldResetClientTrafficMonthly(client.LastReset, client.Extra, now) {
			resetClients = append(resetClients, client)
		}
	}
	if len(resetClients) == 0 {
		return false, nil
	}
	if err := FlushTrafficRuntimeJournal(); err != nil {
		return false, fmt.Errorf("scheduled client traffic reset requires traffic journal flush: %w", err)
	}

	changed := false
	for _, client := range resetClients {
		logger.Info("Resetting traffic for client ", client.Name, " (reset days: ", client.Extra, ")")
		if resetErr := s.NftTrafficService.ResetClientTraffic(db, client.Id); resetErr != nil {
			logger.Warning("failed to reset traffic for client ", client.Name, ": ", resetErr)
			continue
		}
		changed = true
		if client.Depleted && (client.Expiry <= 0 || client.Expiry > now.Unix()) {
			if err := db.Model(&model.Client{}).Where("id = ? AND depleted = ?", client.Id, true).Updates(map[string]interface{}{
				"enable":   true,
				"depleted": false,
			}).Error; err != nil {
				logger.Warning("failed to re-enable reset client ", client.Name, ": ", err)
			}
		}
	}
	return changed, nil
}

func (s *ClientService) DepleteClients() ([]uint, error) {
	var clients []model.Client
	var changes []model.Changes
	var inboundIds []uint

	now := time.Now().Unix()
	db := database.GetDB()

	tx := db.Begin()
	if tx.Error != nil {
		return nil, tx.Error
	}

	if err := tx.Model(model.Client{}).Where("enable = true AND ((volume > 0 AND up + down >= volume) OR (expiry > 0 AND expiry <= ?))", now).Scan(&clients).Error; err != nil {
		tx.Rollback()
		return nil, err
	}

	dt := time.Now().Unix()
	for _, client := range clients {
		logger.Debug("Client ", client.Name, " is going to be disabled")
		var userInbounds []uint
		_ = json.Unmarshal(client.Inbounds, &userInbounds)
		// Find changed inbounds
		inboundIds = common.UnionUintArray(inboundIds, userInbounds)
		changes = append(changes, model.Changes{
			DateTime: dt,
			Actor:    "DepleteJob",
			Key:      "clients",
			Action:   "disable",
			Obj:      json.RawMessage("\"" + client.Name + "\""),
		})
	}

	// Save changes
	if len(changes) > 0 {
		if err := tx.Model(model.Client{}).Where("enable = true AND ((volume > 0 AND up + down >= volume) OR (expiry > 0 AND expiry <= ?))", now).Updates(map[string]interface{}{"enable": false, "depleted": true}).Error; err != nil {
			tx.Rollback()
			return nil, err
		}
		if err := recordChanges(tx, changes); err != nil {
			tx.Rollback()
			return nil, err
		}
	}

	if err := tx.Commit().Error; err != nil {
		tx.Rollback()
		return nil, err
	}
	if len(changes) > 0 {
		markLastUpdate(dt)
	}

	return inboundIds, nil
}

func (s *ClientService) findInboundsChanges(tx *gorm.DB, client model.Client) ([]uint, error) {
	var err error
	var oldClient model.Client
	var oldInboundIds, newInboundIds []uint
	err = tx.Model(model.Client{}).Where("id = ?", client.Id).First(&oldClient).Error
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(oldClient.Inbounds, &oldInboundIds)
	if err != nil {
		return nil, err
	}
	err = json.Unmarshal(client.Inbounds, &newInboundIds)
	if err != nil {
		return nil, err
	}

	// Check client.Config changes
	if !bytes.Equal(oldClient.Config, client.Config) ||
		oldClient.Name != client.Name ||
		oldClient.Enable != client.Enable {
		return common.UnionUintArray(oldInboundIds, newInboundIds), nil
	}

	// Check client.Inbounds changes
	diffInbounds := common.DiffUintArray(oldInboundIds, newInboundIds)

	return diffInbounds, nil
}
