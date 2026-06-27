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

func (s *MihomoClientService) Get(id string) (*[]model.MihomoClient, error) {
	if id == "" {
		return s.GetAll()
	}
	return s.getById(id)
}

func (s *MihomoClientService) getById(id string) (*[]model.MihomoClient, error) {
	db := database.GetDB()
	var clients []model.MihomoClient
	err := db.Model(model.MihomoClient{}).Where("id in ?", strings.Split(id, ",")).Scan(&clients).Error
	if err != nil {
		return nil, err
	}
	return &clients, nil
}

func (s *MihomoClientService) GetAll() (*[]model.MihomoClient, error) {
	db := database.GetDB()
	var clients []model.MihomoClient
	err := db.Model(model.MihomoClient{}).
		Select("`id`, `enable`, `name`, `desc`, `group`, `inbounds`, `up`, `down`, `volume`, `expiry`, `speed_limit_mbps`").
		Order("id ASC").
		Scan(&clients).Error
	if err != nil {
		return nil, err
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
		if err = validateMihomoSnellClientBindings(tx, &client); err != nil {
			return nil, err
		}
		if err = s.updateLinksWithFixedInbounds(tx, []*model.MihomoClient{&client}, hostname); err != nil {
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
			if err = json.Unmarshal(client.Inbounds, &inboundIDs); err != nil {
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
				inboundIDs = common.UnionUintArray(oldInboundIDs, newInboundIDs)
			}
		}
		if act == "new" && client.LastReset == 0 {
			client.LastReset = nowUnix
		}
		client.SpeedLimitMbps = normalizeClientSpeedLimitMbps(client.SpeedLimitMbps)
		if err = tx.Save(&client).Error; err != nil {
			return nil, err
		}

		var clientInboundIDs []uint
		if jsonErr := json.Unmarshal(client.Inbounds, &clientInboundIDs); jsonErr == nil {
			if syncErr := s.MihomoNftTrafficService.SyncClientBindings(tx, client.Id, clientInboundIDs); syncErr != nil {
				logger.Warning("failed to sync mihomo client traffic bindings for ", client.Name, ": ", syncErr)
			}
		}

		if manualTrafficReset {
			if resetErr := s.MihomoNftTrafficService.ResetClientTraffic(tx, client.Id); resetErr != nil {
				logger.Warning("failed to reset mihomo client nft traffic baseline for ", client.Name, ": ", resetErr)
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
		for _, client := range clients {
			if client == nil {
				continue
			}
			client.ServerIp = util.NormalizeSubscriptionServerHost(client.ServerIp)
		}
		if err = json.Unmarshal(clients[0].Inbounds, &inboundIDs); err != nil {
			return nil, err
		}
		if _, err = synchronizeMihomoSudokuBindings(tx, clients, nil, nil); err != nil {
			return nil, err
		}
		if err = validateMihomoSnellClientBindingsBatch(tx, clients); err != nil {
			return nil, err
		}
		if err = s.updateLinksWithFixedInbounds(tx, clients, hostname); err != nil {
			return nil, err
		}
		for _, client := range clients {
			client.SpeedLimitMbps = normalizeClientSpeedLimitMbps(client.SpeedLimitMbps)
		}
		if err = tx.Save(clients).Error; err != nil {
			return nil, err
		}

		for _, client := range clients {
			var clientInboundIDs []uint
			if jsonErr := json.Unmarshal(client.Inbounds, &clientInboundIDs); jsonErr == nil {
				if syncErr := s.MihomoNftTrafficService.SyncClientBindings(tx, client.Id, clientInboundIDs); syncErr != nil {
					logger.Warning("failed to sync mihomo client traffic bindings for ", client.Name, ": ", syncErr)
				}
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
		if err = json.Unmarshal(client.Inbounds, &inboundIDs); err != nil {
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

func validateMihomoSnellClientBindingsBatch(tx *gorm.DB, clients []*model.MihomoClient) error {
	claimedInboundOwner := map[uint]string{}
	for _, client := range clients {
		if client == nil {
			continue
		}
		var inboundIDs []uint
		if err := json.Unmarshal(client.Inbounds, &inboundIDs); err != nil {
			return err
		}
		inboundIDs = dedupeUintIDs(inboundIDs)
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

	var inboundIDs []uint
	if err := json.Unmarshal(client.Inbounds, &inboundIDs); err != nil {
		return err
	}
	inboundIDs = dedupeUintIDs(inboundIDs)
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

	for _, inbound := range snellInbounds {
		var conflictClientID uint
		query := tx.Model(model.MihomoClient{}).
			Select("id").
			Where("id <> ?", client.Id).
			Where("EXISTS (SELECT 1 FROM json_each(mihomo_clients.inbounds) WHERE json_each.value = ?)", inbound.Id).
			Limit(1)
		if err := query.Scan(&conflictClientID).Error; err != nil {
			return err
		}
		if conflictClientID != 0 {
			return fmt.Errorf("snell inbound %s can bind only one user", inbound.Tag)
		}
	}

	return nil
}

func (s *MihomoClientService) updateLinksWithFixedInbounds(tx *gorm.DB, clients []*model.MihomoClient, hostname string) error {
	if len(clients) == 0 {
		return nil
	}

	var inboundIDs []uint
	if err := json.Unmarshal(clients[0].Inbounds, &inboundIDs); err != nil {
		return err
	}

	var inbounds []model.MihomoInbound
	if len(inboundIDs) > 0 {
		err := tx.Model(model.MihomoInbound{}).
			Preload("Tls").
			Where("id in ? and type in ?", inboundIDs, util.InboundTypeWithLink).
			Find(&inbounds).Error
		if err != nil {
			return err
		}
		inbounds = util.OrderMihomoInboundValuesByIDs(inboundIDs, inbounds)
	}

	for index, client := range clients {
		var clientLinks []map[string]string
		if err := json.Unmarshal(client.Links, &clientLinks); err != nil {
			return err
		}

		newClientLinks := []map[string]string{}
		for _, inbound := range inbounds {
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
			return err
		}
		clients[index].Links = links
	}

	return nil
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
		var clientInbounds []uint
		_ = json.Unmarshal(client.Inbounds, &clientInbounds)
		clientInbounds = append(clientInbounds, inboundID)
		inboundsRaw, err := json.MarshalIndent(clientInbounds, "", "  ")
		if err != nil {
			return err
		}
		client.Inbounds = inboundsRaw

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
			if clientLink["remark"] != inbound.Tag {
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
		if syncErr := s.MihomoNftTrafficService.SyncClientBindings(tx, client.Id, clientInbounds); syncErr != nil {
			logger.Warning("failed to sync mihomo client traffic bindings for ", client.Name, " after inbound add: ", syncErr)
		}
	}

	return nil
}

func (s *MihomoClientService) UpdateClientsOnInboundDelete(tx *gorm.DB, id uint, tag string) error {
	var clients []model.MihomoClient
	err := tx.Table("mihomo_clients").
		Where("EXISTS (SELECT 1 FROM json_each(mihomo_clients.inbounds) WHERE json_each.value = ?)", id).
		Find(&clients).Error
	if err != nil {
		return err
	}

	for _, client := range clients {
		var clientInbounds, newClientInbounds []uint
		_ = json.Unmarshal(client.Inbounds, &clientInbounds)
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
			if clientLink["remark"] != tag {
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
		if syncErr := s.MihomoNftTrafficService.SyncClientBindings(tx, client.Id, newClientInbounds); syncErr != nil {
			logger.Warning("failed to sync mihomo client traffic bindings for ", client.Name, " after inbound delete: ", syncErr)
		}
	}

	return nil
}

func (s *MihomoClientService) UpdateLinksByInboundChange(tx *gorm.DB, inbounds *[]model.MihomoInbound, hostname string, oldTag string) error {
	if inbounds == nil {
		return nil
	}

	for _, inbound := range *inbounds {
		var clients []model.MihomoClient
		err := tx.Table("mihomo_clients").
			Where("EXISTS (SELECT 1 FROM json_each(mihomo_clients.inbounds) WHERE json_each.value = ?)", inbound.Id).
			Find(&clients).Error
		if err != nil {
			return err
		}

		base := inbound.ToBase()
		for _, client := range clients {
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
				if clientLink["type"] != "local" || (clientLink["remark"] != inbound.Tag && clientLink["remark"] != oldTag) {
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
		}
	}

	return nil
}

// ResetTrafficBySchedule resets mihomo client traffic by configured monthly reset days.
func (s *MihomoClientService) ResetTrafficBySchedule() error {
	db := database.GetDB()
	now := time.Now().In(getClientAccessPolicyLocation())

	var clients []model.MihomoClient
	err := db.Model(model.MihomoClient{}).
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
		logger.Info("Resetting traffic for mihomo client ", client.Name, " (reset days: ", client.Extra, ")")
		if resetErr := s.MihomoNftTrafficService.ResetClientTraffic(tx, client.Id); resetErr != nil {
			logger.Warning("failed to reset traffic for mihomo client ", client.Name, ": ", resetErr)
		}
	}

	if err = tx.Commit().Error; err != nil {
		tx.Rollback()
		return err
	}
	return nil
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
		Where("enable = true AND ((volume > 0 AND up + down > volume) OR (expiry > 0 AND expiry < ?))", now).
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

		var clientInbounds []uint
		_ = json.Unmarshal(client.Inbounds, &clientInbounds)
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
		Where("enable = true AND ((volume > 0 AND up + down > volume) OR (expiry > 0 AND expiry < ?))", now).
		Update("enable", false).Error; err != nil {
		tx.Rollback()
		return nil, err
	}

	if len(changes) > 0 {
		if err = recordChanges(tx, changes); err != nil {
			tx.Rollback()
			return nil, err
		}
		LastUpdate = dt
	}

	if err = tx.Commit().Error; err != nil {
		tx.Rollback()
		return nil, err
	}

	return inboundIDs, nil
}

func (s *MihomoClientService) findInboundsChanges(tx *gorm.DB, client model.MihomoClient) ([]uint, error) {
	var oldClient model.MihomoClient
	if err := tx.Model(model.MihomoClient{}).Where("id = ?", client.Id).First(&oldClient).Error; err != nil {
		return nil, err
	}

	var oldInboundIDs []uint
	var newInboundIDs []uint
	if err := json.Unmarshal(oldClient.Inbounds, &oldInboundIDs); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(client.Inbounds, &newInboundIDs); err != nil {
		return nil, err
	}

	if !bytes.Equal(oldClient.Config, client.Config) || oldClient.Name != client.Name || oldClient.Enable != client.Enable {
		return common.UnionUintArray(oldInboundIDs, newInboundIDs), nil
	}

	return common.DiffUintArray(oldInboundIDs, newInboundIDs), nil
}
