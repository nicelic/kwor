package service

import (
	"bytes"
	"encoding/json"
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

func (s *ClientService) Get(id string) (*[]model.Client, error) {
	if id == "" {
		return s.GetAll()
	}
	return s.getById(id)
}

func (s *ClientService) getById(id string) (*[]model.Client, error) {
	db := database.GetDB()
	var client []model.Client
	err := db.Model(model.Client{}).Where("id in ?", strings.Split(id, ",")).Scan(&client).Error
	if err != nil {
		return nil, err
	}

	return &client, nil
}

func (s *ClientService) GetAll() (*[]model.Client, error) {
	db := database.GetDB()
	var clients []model.Client
	err := db.Model(model.Client{}).
		Select("`id`, `enable`, `name`, `desc`, `group`, `inbounds`, `up`, `down`, `volume`, `expiry`, `speed_limit_mbps`").
		Order("id ASC").
		Scan(&clients).Error
	if err != nil {
		return nil, err
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
			err = json.Unmarshal(client.Inbounds, &inboundIds)
			if err != nil {
				return nil, err
			}
		}
		nowUnix := time.Now().Unix()
		manualTrafficReset := client.TrafficResetRequested && act == "edit"
		if oldClient != nil {
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

			accessSettingsChanged := manualTrafficReset ||
				oldClient.Volume != client.Volume ||
				oldClient.Expiry != client.Expiry ||
				oldClient.Extra != client.Extra
			oldEvaluation := evaluateClientAccess(true, oldClient.Up+oldClient.Down, oldClient.Volume, oldClient.Expiry, nowUnix)
			evaluation := evaluateClientAccess(true, client.Up+client.Down, client.Volume, client.Expiry, nowUnix)
			if accessSettingsChanged && !oldClient.Enable && !client.Enable && oldEvaluation.Blocked && !evaluation.Blocked {
				client.Enable = true
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
		var clientInboundIds []uint
		if jsonErr := json.Unmarshal(client.Inbounds, &clientInboundIds); jsonErr == nil {
			if syncErr := s.NftTrafficService.SyncClientBindings(tx, client.Id, clientInboundIds); syncErr != nil {
				logger.Warning("failed to sync client traffic bindings for ", client.Name, ": ", syncErr)
			}
		}

		// Manual reset keeps the current nft baselines aligned with the UI counters.
		if manualTrafficReset {
			if resetErr := s.NftTrafficService.ResetClientTraffic(tx, client.Id); resetErr != nil {
				logger.Warning("failed to reset client nft traffic baseline for ", client.Name, ": ", resetErr)
			}
		}

	case "addbulk":
		var clients []*model.Client
		err = json.Unmarshal(data, &clients)
		if err != nil {
			return nil, err
		}
		for _, client := range clients {
			if client == nil {
				continue
			}
			client.ServerIp = util.NormalizeSubscriptionServerHost(client.ServerIp)
		}
		err = json.Unmarshal(clients[0].Inbounds, &inboundIds)
		if err != nil {
			return nil, err
		}
		err = s.updateLinksWithFixedInbounds(tx, clients, hostname)
		if err != nil {
			return nil, err
		}
		for _, client := range clients {
			client.SpeedLimitMbps = normalizeClientSpeedLimitMbps(client.SpeedLimitMbps)
		}
		err = tx.Save(clients).Error
		if err != nil {
			return nil, err
		}

		// Sync traffic bindings for all bulk-added clients
		for _, client := range clients {
			var clientInboundIds []uint
			if jsonErr := json.Unmarshal(client.Inbounds, &clientInboundIds); jsonErr == nil {
				if syncErr := s.NftTrafficService.SyncClientBindings(tx, client.Id, clientInboundIds); syncErr != nil {
					logger.Warning("failed to sync client traffic bindings for ", client.Name, ": ", syncErr)
				}
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

func (s *ClientService) updateLinksWithFixedInbounds(tx *gorm.DB, clients []*model.Client, hostname string) error {
	var err error
	var inbounds []model.Inbound
	var inboundIds []uint

	err = json.Unmarshal(clients[0].Inbounds, &inboundIds)
	if err != nil {
		return err
	}

	// Zero inbounds means removing local links only
	if len(inboundIds) > 0 {
		err = tx.Model(model.Inbound{}).Preload("Tls").Where("id in ? and type in ?", inboundIds, util.InboundTypeWithLink).Find(&inbounds).Error
		if err != nil {
			return err
		}
		inbounds = util.OrderBaseInboundValuesByIDs(inboundIds, inbounds)
	}
	for index, client := range clients {
		var clientLinks []map[string]string
		err = json.Unmarshal(client.Links, &clientLinks)
		if err != nil {
			return err
		}

		newClientLinks := []map[string]string{}
		for _, inbound := range inbounds {
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
		if syncErr := s.NftTrafficService.SyncClientBindings(tx, client.Id, clientInbounds); syncErr != nil {
			logger.Warning("failed to sync client traffic bindings for ", client.Name, " after inbound add: ", syncErr)
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
		if syncErr := s.NftTrafficService.SyncClientBindings(tx, client.Id, newClientInbounds); syncErr != nil {
			logger.Warning("failed to sync client traffic bindings for ", client.Name, " after inbound delete: ", syncErr)
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
func (s *ClientService) ResetTrafficBySchedule() error {
	db := database.GetDB()
	now := time.Now().In(getClientAccessPolicyLocation())

	var clients []model.Client
	err := db.Model(model.Client{}).
		Where("extra > 0").
		Find(&clients).Error
	if err != nil {
		return err
	}

	if len(clients) == 0 {
		return nil
	}

	tx := db.Begin()
	if tx.Error != nil {
		return tx.Error
	}

	for _, client := range clients {
		if !shouldResetClientTrafficMonthly(client.LastReset, client.Extra, now) {
			continue
		}
		logger.Info("Resetting traffic for client ", client.Name, " (reset days: ", client.Extra, ")")
		if resetErr := s.NftTrafficService.ResetClientTraffic(tx, client.Id); resetErr != nil {
			logger.Warning("failed to reset traffic for client ", client.Name, ": ", resetErr)
		}
	}

	if err = tx.Commit().Error; err != nil {
		tx.Rollback()
		return err
	}
	return nil
}

func (s *ClientService) DepleteClients() ([]uint, error) {
	var err error
	var clients []model.Client
	var changes []model.Changes
	var users []string
	var inboundIds []uint

	now := time.Now().Unix()
	db := database.GetDB()

	tx := db.Begin()
	defer func() {
		if err == nil {
			tx.Commit()
		} else {
			tx.Rollback()
		}
	}()

	err = tx.Model(model.Client{}).Where("enable = true AND ((volume >0 AND up+down > volume) OR (expiry > 0 AND expiry < ?))", now).Scan(&clients).Error
	if err != nil {
		return nil, err
	}

	dt := time.Now().Unix()
	for _, client := range clients {
		logger.Debug("Client ", client.Name, " is going to be disabled")
		users = append(users, client.Name)
		var userInbounds []uint
		json.Unmarshal(client.Inbounds, &userInbounds)
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
		err = tx.Model(model.Client{}).Where("enable = true AND ((volume >0 AND up+down > volume) OR (expiry > 0 AND expiry < ?))", now).Update("enable", false).Error
		if err != nil {
			return nil, err
		}
		err = recordChanges(tx, changes)
		if err != nil {
			return nil, err
		}
		LastUpdate = dt
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
