package service

import (
	"encoding/json"
	"strconv"
	"time"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/logger"
	"github.com/alireza0/s-ui/util/common"
	"gorm.io/gorm"
)

var (
	LastUpdate int64
	corePtr    = &dummyCore{}
)

type dummyCore struct{}

func (c *dummyCore) IsRunning() bool                          { return false }
func (c *dummyCore) Start(config []byte) error                { return nil }
func (c *dummyCore) Stop() error                              { return nil }
func (c *dummyCore) AddInbound(config json.RawMessage) error  { return nil }
func (c *dummyCore) RemoveInbound(tag string) error           { return nil }
func (c *dummyCore) AddOutbound(config json.RawMessage) error { return nil }
func (c *dummyCore) RemoveOutbound(tag string) error          { return nil }
func (c *dummyCore) AddEndpoint(config json.RawMessage) error { return nil }
func (c *dummyCore) RemoveEndpoint(tag string) error          { return nil }
func (c *dummyCore) AddService(config json.RawMessage) error  { return nil }
func (c *dummyCore) RemoveService(tag string) error           { return nil }
func (c *dummyCore) GetInstance() *dummyCore                  { return c }
func (c *dummyCore) StatsTracker() *dummyCore                 { return c }
func (c *dummyCore) ConnTracker() *dummyCore                  { return c }
func (c *dummyCore) GetStats() *[]model.Stats                 { return &[]model.Stats{} }
func (c *dummyCore) CloseConnByInbound(tag string)            {}
func (c *dummyCore) Uptime() uint32                           { return 0 }

type ConfigService struct {
	ClientService
	SyncService
	TlsService
	SettingService
	InboundService
	OutboundService
	MihomoConfigService
	MihomoClientService
	MihomoSyncService
	MihomoTlsService
	MihomoInboundService
	MihomoOutboundService
	MihomoOutboundGroupService
	OutboundGroupService
	SubOutboundService
	SubGroupService
	ServicesService
	EndpointService
}

type SingBoxConfig struct {
	Log          json.RawMessage   `json:"log"`
	Dns          json.RawMessage   `json:"dns"`
	Ntp          json.RawMessage   `json:"ntp"`
	Inbounds     []json.RawMessage `json:"inbounds"`
	Outbounds    []json.RawMessage `json:"outbounds"`
	Services     []json.RawMessage `json:"services"`
	Endpoints    []json.RawMessage `json:"endpoints"`
	Route        json.RawMessage   `json:"route"`
	Certificate  json.RawMessage   `json:"certificate,omitempty"`
	Experimental json.RawMessage   `json:"experimental"`
}

func NewConfigService(core interface{}) *ConfigService {
	return &ConfigService{}
}

func (s *ConfigService) GetConfig(data string) (*SingBoxConfig, error) {
	var err error
	if len(data) == 0 {
		data, err = s.SettingService.GetConfig()
		if err != nil {
			return nil, err
		}
	}

	configRaw := json.RawMessage(data)
	aliasMap, err := buildInboundTagAliasMap(database.GetDB())
	if err != nil {
		return nil, err
	}
	if len(aliasMap) > 0 {
		configRaw, _, err = normalizeConfigInboundRuleTags(configRaw, aliasMap)
		if err != nil {
			return nil, err
		}
	}

	singboxConfig := SingBoxConfig{}
	err = json.Unmarshal(configRaw, &singboxConfig)
	if err != nil {
		return nil, err
	}

	singboxConfig.Inbounds, err = s.InboundService.GetAllConfig(database.GetDB())
	if err != nil {
		return nil, err
	}
	singboxConfig.Outbounds, err = s.OutboundService.GetAllConfig(database.GetDB())
	if err != nil {
		return nil, err
	}
	singboxConfig.Outbounds, err = stripOutboundsTLSStore(singboxConfig.Outbounds)
	if err != nil {
		return nil, err
	}
	singboxConfig.Services, err = s.ServicesService.GetAllConfig(database.GetDB())
	if err != nil {
		return nil, err
	}
	singboxConfig.Endpoints, err = s.EndpointService.GetAllConfig(database.GetDB())
	if err != nil {
		return nil, err
	}
	return &singboxConfig, nil
}

func (s *ConfigService) IsCoreRunning() bool {
	return false
}

func (s *ConfigService) StartCore(defaultConfig string) error {
	return nil
}

func (s *ConfigService) RestartCore() error {
	return nil
}

func (s *ConfigService) restartCoreWithConfig(config json.RawMessage) error {
	return nil
}

func (s *ConfigService) StopCore() error {
	return nil
}

func (s *ConfigService) Save(obj string, act string, data json.RawMessage, initUsers string, loginUser string, hostname string) (objs []string, err error) {
	objs = []string{obj}
	postCommitHooks := make([]func() error, 0)
	compactStatsAfterCommit := false

	db := database.GetDB()
	tx := db.Begin()
	if tx.Error != nil {
		return nil, tx.Error
	}
	BeginManagedRuntimeHookScope(tx)
	defer func() {
		if err != nil {
			DiscardManagedRuntimeHookScope(tx)
			tx.Rollback()
			return
		}

		if commitErr := tx.Commit().Error; commitErr != nil {
			err = commitErr
			DiscardManagedRuntimeHookScope(tx)
			tx.Rollback()
			return
		}

		LastUpdate = time.Now().Unix()
		managedRuntimeErr := RunManagedRuntimeHookScope(tx)

		proManager := GetProManagerService(s)

		switch obj {
		case "inbounds":
			proManager.regenerateInboundConfigs()
			proManager.regenerateCoreConfig()
			proManager.regenerateSubJsonConfigs()
			postCommitHooks = append(postCommitHooks, func() error {
				return s.syncAutoManagedDefaultClients(hostname)
			})
		case "outbounds":
			proManager.regenerateOutboundConfigs()
			proManager.regenerateCoreConfig()
			postCommitHooks = append(postCommitHooks, func() error {
				return s.syncAutoManagedDefaultClients(hostname)
			})
		case "outboundgroups":
			proManager.regenerateOutboundConfigs()
			proManager.regenerateCoreConfig()
			postCommitHooks = append(postCommitHooks, func() error {
				return s.syncAutoManagedDefaultClients(hostname)
			})
		case "suboutbounds":
			// sub_json is shared by client subscriptions, suboutbounds, and subgroups.
			// Regenerate the whole directory here so client files are preserved.
			proManager.regenerateSubJsonConfigs()
		case "subgroups":
			// SubGroupService.Save already updates group files.
		case "clients":
			proManager.regenerateInboundConfigs()
			proManager.regenerateCoreConfig()
			proManager.regenerateSubJsonConfigs()
			postCommitHooks = append(postCommitHooks, func() error {
				return s.syncAutoManagedDefaultClients(hostname)
			})
		case "tls":
			proManager.regenerateInboundConfigs()
			proManager.regenerateCoreConfig()
			proManager.regenerateSubJsonConfigs()
			postCommitHooks = append(postCommitHooks, func() error {
				return s.syncAutoManagedDefaultClients(hostname)
			})
		case "services", "endpoints":
			proManager.regenerateCoreConfig()
			postCommitHooks = append(postCommitHooks, func() error {
				return s.syncAutoManagedDefaultClients(hostname)
			})
		case "config", "settings":
			proManager.regenerateInboundConfigs()
			proManager.regenerateOutboundConfigs()
			proManager.regenerateCoreConfig()
			proManager.regenerateSubJsonConfigs()
			s.SubOutboundService.RegenerateAllSubOutboundConfigs()
			s.SubGroupService.RegenerateAllGroupConfigs()
			postCommitHooks = append(postCommitHooks, func() error {
				if err := s.syncAutoManagedDefaultClients(hostname); err != nil {
					return err
				}
				return s.syncAutoManagedMihomoClients(hostname)
			})
		case "mihomo_inbounds", "mihomo_outbounds", "mihomo_outboundgroups", "mihomo_clients", "mihomo_tls", "mihomo_config":
			if err := NewMihomoManagerService().RegenerateServerConfig(); err != nil {
				logger.Warning("regenerate mihomo server config failed: ", err)
			}
			postCommitHooks = append(postCommitHooks, func() error {
				return s.syncAutoManagedMihomoClients(hostname)
			})
		}

		for _, hook := range postCommitHooks {
			if hookErr := hook(); hookErr != nil {
				logger.Warning("post-commit hook failed: ", hookErr)
			}
		}
		if compactStatsAfterCommit {
			if compactErr := compactMainSQLiteDB(db, true); compactErr != nil {
				logger.Warning("compact sqlite after disabling stats history failed: ", compactErr)
			}
		}

		if managedRuntimeErr != nil {
			err = managedRuntimeErr
		}
	}()

	switch obj {
	case "clients":
		var newClient model.Client
		hasNewClient := false
		var oldClient *model.Client
		if act == "new" || act == "edit" {
			if jsonErr := json.Unmarshal(data, &newClient); jsonErr != nil {
				return nil, jsonErr
			}
			hasNewClient = true
		}
		if act == "edit" && hasNewClient && newClient.Id > 0 {
			var previousClient model.Client
			if queryErr := tx.Model(model.Client{}).Where("id = ?", newClient.Id).First(&previousClient).Error; queryErr != nil {
				return nil, queryErr
			}
			oldClient = &previousClient
		}
		if act == "del" {
			var deleteClientID uint
			if jsonErr := json.Unmarshal(data, &deleteClientID); jsonErr != nil {
				return nil, jsonErr
			}
			if deleteClientID > 0 {
				var previousClient model.Client
				if queryErr := tx.Model(model.Client{}).Where("id = ?", deleteClientID).First(&previousClient).Error; queryErr != nil {
					return nil, queryErr
				}
				oldClient = &previousClient
			}
		}

		if err == nil && act == "del" && oldClient != nil {
			if syncErr := s.SyncService.CleanupClientSubOutboundsOnDelete(tx, oldClient); syncErr != nil {
				return nil, common.NewErrorf("failed to cleanup client suboutbounds on delete: %v", syncErr)
			}
			objs = append(objs, "suboutbounds", "subgroups")
			deletedClientID := oldClient.Id
			postCommitHooks = append(postCommitHooks, func() error {
				return s.SettingService.SetSubManagerAutoSyncClient(deletedClientID, false)
			})
		}

		var inboundIds []uint
		inboundIds, err = s.ClientService.Save(tx, act, data, hostname)
		if err == nil && len(inboundIds) > 0 {
			objs = append(objs, "inbounds")
			err = s.InboundService.RestartInbounds(tx, inboundIds)
			if err != nil {
				return nil, common.NewErrorf("failed to update users for inbounds: %v", err)
			}
		}

		// Keep suboutbounds in sync when editing a client that already has synced records.
		if err == nil && act == "edit" && hasNewClient {
			if syncErr := s.SyncService.SyncClientOnSave(tx, oldClient, &newClient, hostname); syncErr != nil {
				return nil, common.NewErrorf("failed to sync client suboutbounds: %v", syncErr)
			}
			objs = append(objs, "suboutbounds", "subgroups")
		}
		if err == nil {
			postCommitHooks = append(postCommitHooks, func() error {
				return s.applyDefaultClientNftPolicies()
			})
		}
	case "tls":
		err = s.TlsService.Save(tx, act, data, hostname)
		objs = append(objs, "clients", "inbounds")
	case "inbounds":
		nftAction, nftPlanErr := s.InboundService.Save(tx, act, data, initUsers, hostname)
		err = nftPlanErr
		objs = append(objs, "clients")
		if err == nil && nftAction != nil {
			actionCopy := *nftAction
			postCommitHooks = append(postCommitHooks, func() error {
				return s.applyInboundNftAction(&actionCopy)
			})
		}
		if err == nil {
			postCommitHooks = append(postCommitHooks, func() error {
				return s.applyDefaultClientNftPolicies()
			})
		}
	case "outbounds":
		err = s.OutboundService.Save(tx, act, data)
	case "outboundgroups":
		err = s.OutboundGroupService.Save(tx, act, data)
		objs = append(objs, "outbounds")
	case "suboutbounds":
		err = s.SubOutboundService.Save(tx, act, data)
		// SubOutboundService.Save already handles file writes/deletes.
		// Global regeneration is handled in the deferred post-commit hook.
		objs = append(objs, "subgroups")
	case "subgroups":
		err = s.SubGroupService.Save(tx, act, data)
		// SubGroupService.Save already handles group file writes/deletes.
	case "services":
		err = s.ServicesService.Save(tx, act, data)
	case "endpoints":
		err = s.EndpointService.Save(tx, act, data)
	case "config":
		normalizedConfig := data
		aliasMap, aliasErr := buildInboundTagAliasMap(tx)
		if aliasErr != nil {
			return nil, aliasErr
		}
		if len(aliasMap) > 0 {
			normalizedConfig, _, err = normalizeConfigInboundRuleTags(data, aliasMap)
			if err != nil {
				return nil, err
			}
		}
		data = normalizedConfig

		err = s.SettingService.SaveConfig(tx, normalizedConfig)
		if err != nil {
			return nil, err
		}
		err = s.restartCoreWithConfig(normalizedConfig)
	case "settings":
		err = s.SettingService.Save(tx, data)
		if err == nil {
			compactStatsAfterCommit = shouldCompactStatsAfterSettingsSave(data)
		}
		if err == nil {
			postCommitHooks = append(postCommitHooks, func() error {
				if err := ApplyPanelTLSRuntimeSettings(PanelSelfSignedTargetPanel); err != nil {
					logger.Warning("apply web tls runtime settings failed: ", err)
				}
				if err := ApplyPanelTLSRuntimeSettings(PanelSelfSignedTargetSub); err != nil {
					logger.Warning("apply sub tls runtime settings failed: ", err)
				}
				if err := (&FirewallService{}).SyncIfNeeded(0); err != nil {
					logger.Warning("apply firewall sync after settings update failed: ", err)
				}
				return nil
			})
		}
	case "mihomo_clients":
		var newClient model.MihomoClient
		hasNewClient := false
		var oldClient *model.MihomoClient
		if act == "new" || act == "edit" {
			if jsonErr := json.Unmarshal(data, &newClient); jsonErr != nil {
				return nil, jsonErr
			}
			hasNewClient = true
		}
		if act == "edit" && hasNewClient && newClient.Id > 0 {
			var previousClient model.MihomoClient
			if queryErr := tx.Model(model.MihomoClient{}).Where("id = ?", newClient.Id).First(&previousClient).Error; queryErr != nil {
				return nil, queryErr
			}
			oldClient = &previousClient
		}
		if act == "del" {
			var deleteClientID uint
			if jsonErr := json.Unmarshal(data, &deleteClientID); jsonErr != nil {
				return nil, jsonErr
			}
			if deleteClientID > 0 {
				var previousClient model.MihomoClient
				if queryErr := tx.Model(model.MihomoClient{}).Where("id = ?", deleteClientID).First(&previousClient).Error; queryErr != nil {
					return nil, queryErr
				}
				oldClient = &previousClient
			}
		}

		if err == nil && act == "del" && oldClient != nil {
			if syncErr := s.MihomoSyncService.CleanupClientSubOutboundsOnDelete(tx, oldClient); syncErr != nil {
				return nil, common.NewErrorf("failed to cleanup mihomo client suboutbounds on delete: %v", syncErr)
			}
			objs = append(objs, "suboutbounds", "subgroups")
			deletedClientID := oldClient.Id
			postCommitHooks = append(postCommitHooks, func() error {
				return s.SettingService.SetSubManagerAutoSyncMihomoClient(deletedClientID, false)
			})
		}

		var inboundIDs []uint
		inboundIDs, err = s.MihomoClientService.Save(tx, act, data, hostname)
		if err == nil && len(inboundIDs) > 0 {
			objs = append(objs, "mihomo_inbounds")
		}

		if err == nil && act == "edit" && hasNewClient {
			if syncErr := s.MihomoSyncService.SyncClientOnSave(tx, oldClient, &newClient, hostname); syncErr != nil {
				return nil, common.NewErrorf("failed to sync mihomo client suboutbounds: %v", syncErr)
			}
			objs = append(objs, "suboutbounds", "subgroups")
		}
		if err == nil {
			skipIDs := make([]uint, 0, 1)
			if act == "edit" && hasNewClient && newClient.Id > 0 {
				skipIDs = append(skipIDs, newClient.Id)
			}
			if syncErr := s.syncManagedMihomoClients(tx, hostname, skipIDs...); syncErr != nil {
				return nil, common.NewErrorf("failed to sync related mihomo client suboutbounds: %v", syncErr)
			}
			objs = append(objs, "suboutbounds", "subgroups")
		}
		if err == nil {
			postCommitHooks = append(postCommitHooks, func() error {
				return s.applyMihomoClientNftPolicies()
			})
		}
	case "mihomo_tls":
		err = s.MihomoTlsService.Save(tx, act, data, hostname)
		objs = append(objs, "mihomo_clients", "mihomo_inbounds")
	case "mihomo_inbounds":
		nftAction, nftPlanErr := s.MihomoInboundService.Save(tx, act, data, initUsers, hostname)
		err = nftPlanErr
		objs = append(objs, "mihomo_clients")
		if err == nil && nftAction != nil {
			actionCopy := *nftAction
			postCommitHooks = append(postCommitHooks, func() error {
				return s.applyMihomoInboundNftAction(&actionCopy)
			})
		}
		if err == nil {
			if syncErr := s.syncManagedMihomoClients(tx, hostname); syncErr != nil {
				return nil, common.NewErrorf("failed to sync mihomo client suboutbounds: %v", syncErr)
			}
			objs = append(objs, "suboutbounds", "subgroups")
		}
		if err == nil {
			postCommitHooks = append(postCommitHooks, func() error {
				return s.applyMihomoClientNftPolicies()
			})
		}
	case "mihomo_outbounds":
		err = s.MihomoOutboundService.Save(tx, act, data)
	case "mihomo_outboundgroups":
		err = s.MihomoOutboundGroupService.Save(tx, act, data)
		objs = append(objs, "mihomo_outbounds")
	case "mihomo_config":
		err = s.MihomoConfigService.SaveConfig(tx, data)
	default:
		return nil, common.NewError("unknown object: ", obj)
	}
	if err != nil {
		return nil, err
	}
	if obj == "clients" || obj == "inbounds" {
		if err := validateManagedSubJSONFileNames(tx); err != nil {
			return nil, err
		}
	}

	dt := time.Now().Unix()
	err = recordChange(tx, model.Changes{
		DateTime: dt,
		Actor:    loginUser,
		Key:      obj,
		Action:   act,
		Obj:      data,
	})
	if err != nil {
		return nil, err
	}

	return objs, nil
}

func (s *ConfigService) applyInboundNftAction(action *InboundNftAction) error {
	if action == nil {
		return nil
	}

	db := database.GetDB()
	tx := db.Begin()
	if tx.Error != nil {
		return tx.Error
	}

	nftSvc := &NftTrafficService{}
	coreSvc := &CoreManagerService{}
	coreRunning := coreSvc.IsRunning()

	var err error
	switch action.Kind {
	case "upsert":
		if coreRunning {
			err = nftSvc.UpdateInboundRules(tx, action.InboundID, action.Tag, action.Port, action.PortHopRange)
		} else {
			err = nftSvc.UpsertInboundStateOnly(tx, action.InboundID, action.Tag, action.Port, action.PortHopRange)
		}
	case "remove":
		if coreRunning {
			err = nftSvc.RemoveInboundRules(tx, action.InboundID)
		} else {
			err = nftSvc.RemoveInboundStateOnly(tx, action.InboundID)
		}
	default:
		err = common.NewError("unknown inbound nft action: ", action.Kind)
	}
	if err != nil {
		tx.Rollback()
		return err
	}
	if commitErr := tx.Commit().Error; commitErr != nil {
		tx.Rollback()
		return commitErr
	}

	if !coreRunning {
		nftSvc.CleanupOnShutdown()
	}

	return nil
}

func (s *ConfigService) applyClientRateLimitNft() error {
	nftSvc := &ClientRateLimitService{}
	coreRunning := (&CoreManagerService{}).IsRunning()
	return nftSvc.Reconcile(coreRunning)
}

func (s *ConfigService) applyClientPortBlockNft() error {
	nftSvc := &ClientPortBlockService{}
	coreRunning := (&CoreManagerService{}).IsRunning()
	return nftSvc.Reconcile(coreRunning)
}

func (s *ConfigService) applyDefaultClientNftPolicies() error {
	if err := s.applyClientRateLimitNft(); err != nil {
		return err
	}
	return s.applyClientPortBlockNft()
}

func (s *ConfigService) applyMihomoClientRateLimitNft() error {
	nftSvc := &MihomoClientRateLimitService{}
	coreRunning := (&MihomoCoreManagerService{}).IsRunning()
	return nftSvc.Reconcile(coreRunning)
}

func (s *ConfigService) applyMihomoClientPortBlockNft() error {
	nftSvc := &MihomoClientPortBlockService{}
	coreRunning := (&MihomoCoreManagerService{}).IsRunning()
	return nftSvc.Reconcile(coreRunning)
}

func (s *ConfigService) applyMihomoClientNftPolicies() error {
	if err := s.applyMihomoClientRateLimitNft(); err != nil {
		return err
	}
	return s.applyMihomoClientPortBlockNft()
}

func (s *ConfigService) applyMihomoInboundNftAction(action *InboundNftAction) error {
	if action == nil {
		return nil
	}

	db := database.GetDB()
	tx := db.Begin()
	if tx.Error != nil {
		return tx.Error
	}

	nftSvc := &MihomoNftTrafficService{}
	coreSvc := &MihomoCoreManagerService{}
	coreRunning := coreSvc.IsRunning()

	var err error
	switch action.Kind {
	case "upsert":
		if coreRunning {
			err = nftSvc.UpdateInboundRules(tx, action.InboundID, action.Tag, action.Port, action.PortHopRange, action.RedirectTCP)
		} else {
			err = nftSvc.UpsertInboundStateOnly(tx, action.InboundID, action.Tag, action.Port, action.PortHopRange)
		}
	case "remove":
		if coreRunning {
			err = nftSvc.RemoveInboundRules(tx, action.InboundID)
		} else {
			err = nftSvc.RemoveInboundStateOnly(tx, action.InboundID)
		}
	default:
		err = common.NewError("unknown mihomo inbound nft action: ", action.Kind)
	}
	if err != nil {
		tx.Rollback()
		return err
	}
	if commitErr := tx.Commit().Error; commitErr != nil {
		tx.Rollback()
		return commitErr
	}

	if !coreRunning {
		nftSvc.CleanupOnShutdown()
	}

	return nil
}

func (s *ConfigService) CheckChanges(lu string) (bool, error) {
	if lu == "" {
		return true, nil
	}
	intLu, err := strconv.ParseInt(lu, 10, 64)
	if err != nil {
		return true, nil
	}
	if LastUpdate == 0 {
		db := database.GetDB()
		var count int64
		err := db.Model(model.Changes{}).Where("date_time > ?", intLu).Count(&count).Error
		if err == nil {
			LastUpdate = time.Now().Unix()
		}
		return count > 0, err
	} else {
		return LastUpdate > intLu, err
	}
}

func (s *ConfigService) GetChanges(actor string, chngKey string, count string) []model.Changes {
	c, _ := strconv.Atoi(count)
	if c <= 0 {
		c = 10
	}
	if c > 100 {
		c = 100
	}

	db := database.GetDB()
	var chngs []model.Changes
	query := db.Model(model.Changes{})
	if actor != "" {
		query = query.Where("actor = ?", actor)
	}
	if chngKey != "" {
		query = query.Where("key = ?", chngKey)
	}
	err := query.Order("`id` desc").Limit(c).Scan(&chngs).Error
	if err != nil {
		logger.Warning(err)
	}
	return chngs
}

func compactAutoSyncClientIDs(ids []uint, existing map[uint]struct{}) []uint {
	if len(ids) == 0 {
		return []uint{}
	}
	filtered := make([]uint, 0, len(ids))
	for _, id := range ids {
		if _, ok := existing[id]; !ok {
			continue
		}
		filtered = append(filtered, id)
	}
	return filtered
}

func cleanAutoSyncClientIDsAfterPartialSync(ids []uint, attempted map[uint]struct{}, existing map[uint]struct{}) []uint {
	if len(ids) == 0 {
		return []uint{}
	}
	filtered := make([]uint, 0, len(ids))
	for _, id := range ids {
		if _, wasAttempted := attempted[id]; !wasAttempted {
			filtered = append(filtered, id)
			continue
		}
		if _, ok := existing[id]; ok {
			filtered = append(filtered, id)
		}
	}
	return filtered
}

func equalAutoSyncClientIDs(a []uint, b []uint) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func mergeAutoManagedCandidateIDs(parts ...[]uint) []uint {
	result := make([]uint, 0)
	seen := make(map[uint]struct{})
	for _, part := range parts {
		for _, id := range part {
			if id == 0 {
				continue
			}
			if _, exists := seen[id]; exists {
				continue
			}
			seen[id] = struct{}{}
			result = append(result, id)
		}
	}
	return result
}

func filterIDsBySet(ids []uint, allowed map[uint]struct{}) []uint {
	if len(ids) == 0 || len(allowed) == 0 {
		return []uint{}
	}
	result := make([]uint, 0, len(ids))
	for _, id := range ids {
		if _, ok := allowed[id]; ok {
			result = append(result, id)
		}
	}
	return result
}

func loadDefaultClientIDsUsingTLSIDs(tlsIDs []uint) (map[uint]struct{}, error) {
	result := make(map[uint]struct{})
	tlsIDs = compactPositiveUintList(tlsIDs)
	if len(tlsIDs) == 0 {
		return result, nil
	}

	db := database.GetDB()
	inboundIDs, err := loadInboundIDsForDefaultTLSIDs(db, tlsIDs)
	if err != nil {
		return nil, err
	}
	if len(inboundIDs) == 0 {
		return result, nil
	}
	inboundSet := uintSetFromSlice(inboundIDs)

	var clients []model.Client
	if err := db.Model(model.Client{}).Select("id", "inbounds").Find(&clients).Error; err != nil {
		return nil, err
	}
	for i := range clients {
		ids, err := parseClientInboundIDs(clients[i].Inbounds)
		if err != nil {
			continue
		}
		if anyUintInSet(ids, inboundSet) {
			result[clients[i].Id] = struct{}{}
		}
	}

	managedClientIDs, err := loadManagedClientIDsForInboundIDs(db, subOutboundSourceClient, inboundIDs)
	if err != nil {
		return nil, err
	}
	for _, id := range managedClientIDs {
		result[id] = struct{}{}
	}
	return result, nil
}

func loadMihomoClientIDsUsingTLSIDs(tlsIDs []uint) (map[uint]struct{}, error) {
	result := make(map[uint]struct{})
	tlsIDs = compactPositiveUintList(tlsIDs)
	if len(tlsIDs) == 0 {
		return result, nil
	}

	db := database.GetDB()
	inboundIDs, err := loadInboundIDsForMihomoTLSIDs(db, tlsIDs)
	if err != nil {
		return nil, err
	}
	if len(inboundIDs) == 0 {
		return result, nil
	}
	inboundSet := uintSetFromSlice(inboundIDs)

	var clients []model.MihomoClient
	if err := db.Model(model.MihomoClient{}).Select("id", "inbounds").Find(&clients).Error; err != nil {
		return nil, err
	}
	for i := range clients {
		ids, err := parseClientInboundIDs(clients[i].Inbounds)
		if err != nil {
			continue
		}
		if anyUintInSet(ids, inboundSet) {
			result[clients[i].Id] = struct{}{}
		}
	}

	managedClientIDs, err := loadManagedClientIDsForInboundIDs(db, subOutboundSourceMihomoClient, inboundIDs)
	if err != nil {
		return nil, err
	}
	for _, id := range managedClientIDs {
		result[id] = struct{}{}
	}
	return result, nil
}

func loadInboundIDsForDefaultTLSIDs(db *gorm.DB, tlsIDs []uint) ([]uint, error) {
	var inboundIDs []uint
	if err := db.Model(model.Inbound{}).
		Where("tls_id IN ?", tlsIDs).
		Pluck("id", &inboundIDs).Error; err != nil {
		return nil, err
	}
	return compactPositiveUintList(inboundIDs), nil
}

func loadInboundIDsForMihomoTLSIDs(db *gorm.DB, tlsIDs []uint) ([]uint, error) {
	var inboundIDs []uint
	if err := db.Model(model.MihomoInbound{}).
		Where("tls_id IN ?", tlsIDs).
		Pluck("id", &inboundIDs).Error; err != nil {
		return nil, err
	}
	return compactPositiveUintList(inboundIDs), nil
}

func loadManagedClientIDsForInboundIDs(db *gorm.DB, sourceType string, inboundIDs []uint) ([]uint, error) {
	inboundIDs = compactPositiveUintList(inboundIDs)
	if len(inboundIDs) == 0 {
		return []uint{}, nil
	}
	var clientIDs []uint
	if err := db.Model(model.SubOutbound{}).
		Distinct("source_client_id").
		Where("source_type = ? AND source_client_id > 0 AND source_inbound_id IN ?", sourceType, inboundIDs).
		Pluck("source_client_id", &clientIDs).Error; err != nil {
		return nil, err
	}
	return compactPositiveUintList(clientIDs), nil
}

func uintSetFromSlice(ids []uint) map[uint]struct{} {
	result := make(map[uint]struct{}, len(ids))
	for _, id := range ids {
		if id == 0 {
			continue
		}
		result[id] = struct{}{}
	}
	return result
}

func anyUintInSet(ids []uint, set map[uint]struct{}) bool {
	if len(ids) == 0 || len(set) == 0 {
		return false
	}
	for _, id := range ids {
		if _, ok := set[id]; ok {
			return true
		}
	}
	return false
}

func (s *ConfigService) syncManagedMihomoClients(tx *gorm.DB, hostname string, skipClientIDs ...uint) error {
	if tx == nil {
		return nil
	}

	var clients []model.MihomoClient
	if err := tx.Model(model.MihomoClient{}).Find(&clients).Error; err != nil {
		return err
	}

	skip := make(map[uint]struct{}, len(skipClientIDs))
	for _, id := range skipClientIDs {
		if id == 0 {
			continue
		}
		skip[id] = struct{}{}
	}

	for _, client := range clients {
		if _, ignored := skip[client.Id]; ignored {
			continue
		}
		oldClient := client
		newClient := client
		if err := s.MihomoSyncService.SyncClientOnSave(tx, &oldClient, &newClient, hostname); err != nil {
			return err
		}
	}

	return nil
}

func discoverAutoManagedClientIDsBySource(sourceType string) ([]uint, error) {
	if sourceType == "" {
		return []uint{}, nil
	}
	db := database.GetDB()
	var ids []uint
	if err := db.Model(model.SubOutbound{}).
		Distinct("source_client_id").
		Where("source_type = ? AND source_client_id > 0", sourceType).
		Pluck("source_client_id", &ids).Error; err != nil {
		return nil, err
	}
	return ids, nil
}

func (s *ConfigService) syncAutoManagedDefaultClientsForCertificateBinding(hostname string, tlsIDs []uint) error {
	settingsIDs, err := s.SettingService.GetSubManagerAutoSyncClientIDs()
	if err != nil {
		return err
	}
	legacyIDs, err := discoverAutoManagedClientIDsBySource(subOutboundSourceClient)
	if err != nil {
		return err
	}
	affectedClientIDs, err := loadDefaultClientIDsUsingTLSIDs(tlsIDs)
	if err != nil {
		return err
	}
	if len(affectedClientIDs) == 0 {
		return nil
	}

	ids := mergeAutoManagedCandidateIDs(
		filterIDsBySet(settingsIDs, affectedClientIDs),
		filterIDsBySet(legacyIDs, affectedClientIDs),
	)
	existing, err := s.forceSyncDefaultClientIDsToSubManager(hostname, ids)
	if err != nil {
		return err
	}

	cleaned := cleanAutoSyncClientIDsAfterPartialSync(settingsIDs, affectedClientIDs, existing)
	if !equalAutoSyncClientIDs(settingsIDs, cleaned) {
		if err := s.SettingService.SaveSubManagerAutoSyncClientIDs(cleaned); err != nil {
			logger.Warning("save default auto sync client ids failed: ", err)
		}
	}
	return nil
}

func (s *ConfigService) syncAutoManagedMihomoClientsForCertificateBinding(hostname string, tlsIDs []uint) error {
	settingsIDs, err := s.SettingService.GetSubManagerAutoSyncMihomoClientIDs()
	if err != nil {
		return err
	}
	legacyIDs, err := discoverAutoManagedClientIDsBySource(subOutboundSourceMihomoClient)
	if err != nil {
		return err
	}
	affectedClientIDs, err := loadMihomoClientIDsUsingTLSIDs(tlsIDs)
	if err != nil {
		return err
	}
	if len(affectedClientIDs) == 0 {
		return nil
	}

	ids := mergeAutoManagedCandidateIDs(
		filterIDsBySet(settingsIDs, affectedClientIDs),
		filterIDsBySet(legacyIDs, affectedClientIDs),
	)
	existing, err := s.forceSyncMihomoClientIDsToSubManager(hostname, ids)
	if err != nil {
		return err
	}

	cleaned := cleanAutoSyncClientIDsAfterPartialSync(settingsIDs, affectedClientIDs, existing)
	if !equalAutoSyncClientIDs(settingsIDs, cleaned) {
		if err := s.SettingService.SaveSubManagerAutoSyncMihomoClientIDs(cleaned); err != nil {
			logger.Warning("save mihomo auto sync client ids failed: ", err)
		}
	}
	return nil
}

func (s *ConfigService) syncAutoManagedDefaultClients(hostname string) error {
	ids, err := s.SettingService.GetSubManagerAutoSyncClientIDs()
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		// Respect explicit settings only: no implicit auto-discovery/auto-enable.
		return nil
	}

	db := database.GetDB()
	tx := db.Begin()
	if tx.Error != nil {
		return tx.Error
	}
	BeginManagedRuntimeHookScope(tx)

	var clients []model.Client
	if err := tx.Model(model.Client{}).Where("id in ?", ids).Find(&clients).Error; err != nil {
		DiscardManagedRuntimeHookScope(tx)
		tx.Rollback()
		return err
	}

	byID := make(map[uint]*model.Client, len(clients))
	for i := range clients {
		client := &clients[i]
		byID[client.Id] = client
	}

	existing := make(map[uint]struct{}, len(clients))
	for _, id := range ids {
		client, ok := byID[id]
		if !ok {
			continue
		}
		existing[id] = struct{}{}
		if err := s.SyncService.SyncClientOnAutoPush(tx, client, hostname); err != nil {
			logger.Warningf("auto sync default client %s failed: %v", client.Name, err)
		}
	}

	if err := tx.Commit().Error; err != nil {
		DiscardManagedRuntimeHookScope(tx)
		tx.Rollback()
		return err
	}

	if err := RunManagedRuntimeHookScope(tx); err != nil {
		return err
	}

	cleaned := compactAutoSyncClientIDs(ids, existing)
	if !equalAutoSyncClientIDs(ids, cleaned) {
		if err := s.SettingService.SaveSubManagerAutoSyncClientIDs(cleaned); err != nil {
			logger.Warning("save default auto sync client ids failed: ", err)
		}
	}

	return nil
}

func (s *ConfigService) forceSyncDefaultClientIDsToSubManager(hostname string, ids []uint) (map[uint]struct{}, error) {
	ids = compactPositiveUintList(ids)
	existing := make(map[uint]struct{})
	if len(ids) == 0 {
		return existing, nil
	}

	db := database.GetDB()
	tx := db.Begin()
	if tx.Error != nil {
		return nil, tx.Error
	}
	BeginManagedRuntimeHookScope(tx)

	var clients []model.Client
	if err := tx.Model(model.Client{}).Where("id in ?", ids).Find(&clients).Error; err != nil {
		DiscardManagedRuntimeHookScope(tx)
		tx.Rollback()
		return nil, err
	}

	byID := make(map[uint]*model.Client, len(clients))
	for i := range clients {
		client := &clients[i]
		byID[client.Id] = client
	}

	for _, id := range ids {
		client, ok := byID[id]
		if !ok {
			continue
		}
		existing[id] = struct{}{}
		if err := clearBlockedSubSyncInboundsForClient(tx, subOutboundSourceClient, client.Id); err != nil {
			DiscardManagedRuntimeHookScope(tx)
			tx.Rollback()
			return nil, err
		}
		if _, err := s.SyncService.syncClientSubOutbounds(tx, nil, client, hostname, true, true); err != nil {
			logger.Warningf("force sync default client %s failed: %v", client.Name, err)
		}
	}

	if err := tx.Commit().Error; err != nil {
		DiscardManagedRuntimeHookScope(tx)
		tx.Rollback()
		return nil, err
	}
	if err := RunManagedRuntimeHookScope(tx); err != nil {
		return nil, err
	}
	if len(existing) > 0 {
		LastUpdate = time.Now().Unix()
	}
	return existing, nil
}

// SyncAutoManagedClientsForRuntime runs both default and mihomo auto-managed
// client sync pipelines. It is intended for runtime tasks (for example, file
// content watchers) that need to trigger incremental subscription refreshes.
func (s *ConfigService) SyncAutoManagedClientsForRuntime(hostname string) error {
	if err := s.syncAutoManagedDefaultClients(hostname); err != nil {
		return err
	}
	return s.syncAutoManagedMihomoClients(hostname)
}

func (s *ConfigService) syncAutoManagedMihomoClients(hostname string) error {
	ids, err := s.SettingService.GetSubManagerAutoSyncMihomoClientIDs()
	if err != nil {
		return err
	}
	if len(ids) == 0 {
		// Respect explicit settings only: no implicit auto-discovery/auto-enable.
		return nil
	}

	db := database.GetDB()
	tx := db.Begin()
	if tx.Error != nil {
		return tx.Error
	}
	BeginManagedRuntimeHookScope(tx)

	var clients []model.MihomoClient
	if err := tx.Model(model.MihomoClient{}).Where("id in ?", ids).Find(&clients).Error; err != nil {
		DiscardManagedRuntimeHookScope(tx)
		tx.Rollback()
		return err
	}

	byID := make(map[uint]*model.MihomoClient, len(clients))
	for i := range clients {
		client := &clients[i]
		byID[client.Id] = client
	}

	existing := make(map[uint]struct{}, len(clients))
	for _, id := range ids {
		client, ok := byID[id]
		if !ok {
			continue
		}
		existing[id] = struct{}{}
		if err := s.MihomoSyncService.SyncClientOnAutoPush(tx, client, hostname); err != nil {
			logger.Warningf("auto sync mihomo client %s failed: %v", client.Name, err)
		}
	}

	if err := tx.Commit().Error; err != nil {
		DiscardManagedRuntimeHookScope(tx)
		tx.Rollback()
		return err
	}

	if err := RunManagedRuntimeHookScope(tx); err != nil {
		return err
	}

	cleaned := compactAutoSyncClientIDs(ids, existing)
	if !equalAutoSyncClientIDs(ids, cleaned) {
		if err := s.SettingService.SaveSubManagerAutoSyncMihomoClientIDs(cleaned); err != nil {
			logger.Warning("save mihomo auto sync client ids failed: ", err)
		}
	}

	return nil
}

func (s *ConfigService) forceSyncMihomoClientIDsToSubManager(hostname string, ids []uint) (map[uint]struct{}, error) {
	ids = compactPositiveUintList(ids)
	existing := make(map[uint]struct{})
	if len(ids) == 0 {
		return existing, nil
	}

	db := database.GetDB()
	tx := db.Begin()
	if tx.Error != nil {
		return nil, tx.Error
	}
	BeginManagedRuntimeHookScope(tx)

	var clients []model.MihomoClient
	if err := tx.Model(model.MihomoClient{}).Where("id in ?", ids).Find(&clients).Error; err != nil {
		DiscardManagedRuntimeHookScope(tx)
		tx.Rollback()
		return nil, err
	}

	byID := make(map[uint]*model.MihomoClient, len(clients))
	for i := range clients {
		client := &clients[i]
		byID[client.Id] = client
	}

	for _, id := range ids {
		client, ok := byID[id]
		if !ok {
			continue
		}
		existing[id] = struct{}{}
		if err := clearBlockedSubSyncInboundsForClient(tx, subOutboundSourceMihomoClient, client.Id); err != nil {
			DiscardManagedRuntimeHookScope(tx)
			tx.Rollback()
			return nil, err
		}
		if _, err := s.MihomoSyncService.syncClientSubOutbounds(tx, nil, client, hostname, true, true); err != nil {
			logger.Warningf("force sync mihomo client %s failed: %v", client.Name, err)
		}
	}

	if err := tx.Commit().Error; err != nil {
		DiscardManagedRuntimeHookScope(tx)
		tx.Rollback()
		return nil, err
	}
	if err := RunManagedRuntimeHookScope(tx); err != nil {
		return nil, err
	}
	if len(existing) > 0 {
		LastUpdate = time.Now().Unix()
	}
	return existing, nil
}

func shouldCompactStatsAfterSettingsSave(data json.RawMessage) bool {
	settings := map[string]interface{}{}
	if err := json.Unmarshal(data, &settings); err != nil {
		return false
	}
	value, ok := settings["trafficAge"]
	if !ok || value == nil {
		return false
	}
	switch typed := value.(type) {
	case string:
		return typed == "0"
	case float64:
		return typed == 0
	default:
		return false
	}
}
