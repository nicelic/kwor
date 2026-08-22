package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/logger"
	"github.com/alireza0/s-ui/util/common"
	"gorm.io/gorm"
)

// LastUpdate is the default-chain data revision used by sing-box pages.
// Mihomo keeps a separate revision so an update in one independent core does
// not force the other page to rebuild and ship a full snapshot.
var LastUpdate int64
var MihomoLastUpdate int64

func markLastUpdate(_ int64) {
	advanceConfigRevision(&LastUpdate)
}

func markMihomoLastUpdate(_ int64) {
	advanceConfigRevision(&MihomoLastUpdate)
}

func markBothLastUpdates(_ int64) {
	advanceConfigRevision(&LastUpdate)
	advanceConfigRevision(&MihomoLastUpdate)
}

func advanceConfigRevision(revision *int64) {
	// This is an in-memory synchronization revision, not a persisted wall-clock
	// field. Millisecond precision plus a CAS increment prevents two saves in the
	// same second from being invisible to browser polling.
	now := time.Now().UnixMilli()
	for {
		current := atomic.LoadInt64(revision)
		next := now
		if next <= current {
			next = current + 1
		}
		if atomic.CompareAndSwapInt64(revision, current, next) {
			return
		}
	}
}

func currentLastUpdate() int64 {
	return atomic.LoadInt64(&LastUpdate)
}

// CurrentConfigRevisionForPolling exposes the in-memory monotonic revision to
// API snapshots without coupling callers to the internal atomic state.
func CurrentConfigRevisionForPolling() int64 {
	return currentLastUpdate()
}

func CurrentMihomoConfigRevisionForPolling() int64 {
	return atomic.LoadInt64(&MihomoLastUpdate)
}

// MihomoConfigRevisionConflictError prevents a stale route or DNS draft from
// replacing a newer Mihomo configuration saved by another page or session.
// Mihomo's polling revision is monotonic for the running panel process and is
// sufficient for the compact editor snapshots that own only part of config.
type MihomoConfigRevisionConflictError struct {
	ExpectedRevision int64
	CurrentRevision  int64
}

func (e *MihomoConfigRevisionConflictError) Error() string {
	if e == nil {
		return "mihomo configuration revision conflict"
	}
	return fmt.Sprintf("mihomo configuration revision conflict: expected %d, current %d", e.ExpectedRevision, e.CurrentRevision)
}

func ensureMihomoConfigRevision(expected *int64) error {
	if expected == nil {
		return nil
	}
	current := CurrentMihomoConfigRevisionForPolling()
	if *expected != current {
		return &MihomoConfigRevisionConflictError{
			ExpectedRevision: *expected,
			CurrentRevision:  current,
		}
	}
	return nil
}

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

// CommittedSaveError preserves the existing post-commit failure signal while
// allowing callers that coordinated an external reversible change to tell it
// apart from a database rollback. The SQLite transaction has already committed.
type CommittedSaveError struct {
	Err                 error
	RetrySingboxRuntime bool
}

func (e *CommittedSaveError) Error() string {
	if e == nil || e.Err == nil {
		return "设置已提交后的运行时处理失败"
	}
	return e.Err.Error()
}

func (e *CommittedSaveError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

// regenerateCommittedSingboxRuntime is replaceable in focused tests. It is
// deliberately separate from ConfigService.Save so retrying a post-commit
// failure cannot run the original database mutation a second time.
var regenerateCommittedSingboxRuntime = func(configService *ConfigService) error {
	return GetProManagerService(configService).RegenerateCoreConfig()
}

// RetrySingboxRuntime rebuilds only the rendered sing-box runtime file after a
// mutation has already committed. Callers must not replay the original save.
func (s *ConfigService) RetrySingboxRuntime() error {
	if s == nil {
		return errors.New("config service is nil")
	}
	return regenerateCommittedSingboxRuntime(s)
}

type SingBoxConfig struct {
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

func NewConfigService() *ConfigService {
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

func (s *ConfigService) Save(obj string, act string, data json.RawMessage, initUsers string, loginUser string, hostname string) (objs []string, err error) {
	objs = []string{obj}
	// A Clash import holds this lock while downloading outside SQLite and then
	// applies a short transaction. Reject concurrent Mihomo writes so an import
	// result cannot overwrite a later save made from another tab.
	if strings.HasPrefix(obj, "mihomo_") {
		if !mihomoOutboundSubscriptionImportMu.TryLock() {
			return nil, ErrMihomoSubscriptionImportBusy
		}
		defer mihomoOutboundSubscriptionImportMu.Unlock()
	}
	postCommitHooks := make([]func() error, 0)
	defaultTLSSaveImpact := TlsSaveImpact{}
	compactStatsAfterCommit := false
	panelTimeLocationChanged := false
	singboxOutboundGroupsRuntimeChanged := false
	retrySingboxRuntime := false
	mihomoRuntimeChanged := false
	var mihomoRenderedServerConfig []byte
	mihomoServerConfigLockHeld := false
	singboxOutboundWriteLockHeld := false
	// Mihomo group create/edit/reorder operations only change panel metadata.
	// A group delete can remove actual outbounds, so it takes the same snapshot
	// lock as other runtime-affecting Mihomo saves.
	mihomoRuntimeSnapshotSave := strings.HasPrefix(obj, "mihomo_") && (obj != "mihomo_outboundgroups" || act == "del")
	if obj == "settings" {
		panelTimeLocationChanged, err = s.SettingService.WillChangePanelTimeLocation(data)
		if err != nil {
			return nil, err
		}
	}
	if obj == "endpoints" {
		data, err = s.EndpointService.PrepareSave(act, data)
		if err != nil {
			return nil, err
		}
	}
	if obj == "outbounds" || obj == "outboundgroups" {
		// Take the import lock before SQLite's only connection. A subscription
		// refresh downloads outside the database and then applies a short
		// transaction; this prevents a panel outbound save from racing that
		// commit and being overwritten by the imported payload.
		if !singboxOutboundSubscriptionImportMu.TryLock() {
			return nil, ErrSingboxOutboundSubscriptionImportBusy
		}
		singboxOutboundWriteLockHeld = true
	}
	if mihomoRuntimeSnapshotSave {
		// Take the generator lock before checking out SQLite's only connection.
		// RegenerateServerConfig takes this lock before querying the database too;
		// reversing that order can deadlock a save against a concurrent rebuild.
		mihomoServerConfigRegenerationMu.Lock()
		mihomoServerConfigLockHeld = true
	}
	defer func() {
		if singboxOutboundWriteLockHeld {
			singboxOutboundSubscriptionImportMu.Unlock()
		}
		if mihomoServerConfigLockHeld {
			mihomoServerConfigRegenerationMu.Unlock()
		}
	}()

	// Configuration changes can invalidate traffic baselines and nft comments.
	// Drain the durable history tail before the mutation so the next sampler pass
	// never replays bytes against a newly rendered inbound/client set.
	finishTrafficMutation := BeginRuntimeTrafficMutation()
	defer finishTrafficMutation()
	if flushErr := FlushTrafficRuntimeJournal(); flushErr != nil {
		return nil, fmt.Errorf("保存配置前刷新流量账本失败: %w", flushErr)
	}

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

		// Invalidate only after the transaction has committed. Invalidating while
		// SettingService.Save is still inside the transaction can allow a parallel
		// reader to cache the old database value and make the subsequent cron
		// reload use the wrong calendar location.
		if panelTimeLocationChanged {
			InvalidatePanelTimeLocationCache()
		}
		if obj == "settings" {
			InvalidateSessionMaxAgeCache()
			InvalidateTrafficAgeCache()
			// ConfigService.Save is still used by legacy/internal settings
			// callers. Keep subscription rendering in sync with that path too.
			invalidateSubscriptionRuntimeSettings()
		}
		if obj == "tls" && defaultTLSSaveImpact.PathBindingsChanged {
			InvalidateSubscriptionTLSPathWatchBindings()
		}
		if obj == "mihomo_tls" {
			InvalidateSubscriptionTLSPathWatchBindings()
		}

		managedRuntimeErr := RunManagedRuntimeHookScope(tx)

		proManager := GetProManagerService(s)
		regenerateSingboxRuntime := func() {
			if regenerateErr := proManager.RegenerateCoreConfig(); regenerateErr != nil {
				retrySingboxRuntime = true
				managedRuntimeErr = errors.Join(managedRuntimeErr, fmt.Errorf("regenerate sing-box core config failed: %w", regenerateErr))
			}
		}

		switch obj {
		case "inbounds":
			regenerateSingboxRuntime()
			postCommitHooks = append(postCommitHooks, func() error {
				return s.syncAutoManagedDefaultClients(hostname)
			})
		case "outbounds":
			regenerateSingboxRuntime()
			postCommitHooks = append(postCommitHooks, func() error {
				return s.syncAutoManagedDefaultClients(hostname)
			})
		case "outboundgroups":
			if singboxOutboundGroupsRuntimeChanged {
				regenerateSingboxRuntime()
				postCommitHooks = append(postCommitHooks, func() error {
					return s.syncAutoManagedDefaultClients(hostname)
				})
			}
		case "suboutbounds", "subgroups":
			// Subscription payloads are rendered from SQLite on request.
		case "clients":
			regenerateSingboxRuntime()
			postCommitHooks = append(postCommitHooks, func() error {
				return s.syncAutoManagedDefaultClients(hostname)
			})
		case "tls":
			if defaultTLSSaveImpact.RuntimeConfigChanged {
				regenerateSingboxRuntime()
			}
			if defaultTLSSaveImpact.SubscriptionProjectionChanged && defaultTLSSaveImpact.TLSID > 0 {
				tlsID := defaultTLSSaveImpact.TLSID
				postCommitHooks = append(postCommitHooks, func() error {
					return s.syncAutoManagedDefaultClientsForCertificateBinding(hostname, []uint{tlsID})
				})
			}
		case "services", "endpoints":
			regenerateSingboxRuntime()
			postCommitHooks = append(postCommitHooks, func() error {
				return s.syncAutoManagedDefaultClients(hostname)
			})
		case "config", "settings":
			regenerateSingboxRuntime()
			postCommitHooks = append(postCommitHooks, func() error {
				if err := s.syncAutoManagedDefaultClients(hostname); err != nil {
					return err
				}
				return s.syncAutoManagedMihomoClients(hostname)
			})
		case "mihomo_inbounds", "mihomo_outbounds", "mihomo_clients", "mihomo_tls", "mihomo_config":
			mihomoRuntimeChanged = true
			fallthrough
		case "mihomo_outboundgroups":
			if obj != "mihomo_outboundgroups" || mihomoRuntimeChanged {
				mihomoManager := NewMihomoManagerService()
				var regenerateErr error
				if mihomoServerConfigLockHeld {
					regenerateErr = mihomoManager.writeRenderedServerConfig(mihomoRenderedServerConfig)
				} else {
					regenerateErr = mihomoManager.RegenerateServerConfig()
				}
				if regenerateErr != nil {
					managedRuntimeErr = errors.Join(managedRuntimeErr, fmt.Errorf("regenerate mihomo server config failed: %w", regenerateErr))
				} else {
					if obj == "mihomo_tls" {
						tlsID := mihomoTLSIDFromSavePayload(data)
						if tlsID > 0 {
							postCommitHooks = append(postCommitHooks, func() error {
								return s.syncAutoManagedMihomoClientsForCertificateBinding(hostname, []uint{tlsID})
							})
						}
					} else {
						postCommitHooks = append(postCommitHooks, func() error {
							return s.syncAutoManagedMihomoClients(hostname)
						})
					}
				}
				if mihomoServerConfigLockHeld {
					mihomoServerConfigRegenerationMu.Unlock()
					mihomoServerConfigLockHeld = false
				}
			}
		}

		for _, hook := range postCommitHooks {
			if hookErr := hook(); hookErr != nil {
				logger.Warning("post-commit hook failed: ", hookErr)
			}
		}
		// Advance polling revisions only after every post-commit hook has
		// finished. Several hooks reconcile managed subscription nodes in their
		// own transactions; publishing a revision first could let another page
		// load the old projection and then skip its next refresh.
		//
		// Some default-chain saves reconcile auto-managed client projections.
		// Those projections are shared with Mihomo, so both polling revisions
		// must advance even though the initiating object belongs to one core.
		// Pure group metadata/order changes stay local.
		sharedSubManagerMutation := obj == "clients" || obj == "inbounds" || obj == "tls" ||
			obj == "outbounds" || obj == "services" || obj == "endpoints" ||
			obj == "config" || obj == "settings" ||
			(obj == "outboundgroups" && singboxOutboundGroupsRuntimeChanged) ||
			obj == "suboutbounds" || obj == "subgroups" || obj == "mihomo_clients" ||
			obj == "mihomo_inbounds" || obj == "mihomo_tls" || obj == "mihomo_outbounds" ||
			(obj == "mihomo_outboundgroups" && mihomoRuntimeChanged) || obj == "mihomo_config"
		if sharedSubManagerMutation {
			markBothLastUpdates(time.Now().Unix())
		} else if strings.HasPrefix(obj, "mihomo_") {
			markMihomoLastUpdate(time.Now().Unix())
		} else {
			markLastUpdate(time.Now().Unix())
		}
		if compactStatsAfterCommit {
			requestMainSQLiteCompaction(db, true)
		}

		if managedRuntimeErr != nil {
			err = &CommittedSaveError{
				Err:                 managedRuntimeErr,
				RetrySingboxRuntime: retrySingboxRuntime,
			}
		}
		InvalidateDashboardRuntimeCache()
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

		var inboundIDs []uint
		inboundIDs, err = s.ClientService.Save(tx, act, data, hostname)
		if err == nil && len(inboundIDs) > 0 {
			objs = append(objs, "inbounds")
		}

		// Keep suboutbounds in sync when editing a client that already has synced records.
		if err == nil && act == "edit" && hasNewClient {
			if syncErr := s.SyncService.SyncClientOnSave(tx, oldClient, &newClient, hostname); syncErr != nil {
				return nil, common.NewErrorf("failed to sync client suboutbounds: %v", syncErr)
			}
		}
		// The post-commit auto-sync hook can also rebuild already-managed users.
		// Always return the shared collections for a successful client mutation.
		if err == nil {
			objs = append(objs, "suboutbounds", "subgroups")
		}
		if err == nil {
			postCommitHooks = append(postCommitHooks, func() error {
				return s.applyDefaultClientNftPolicies()
			})
		}
	case "tls":
		defaultTLSSaveImpact, err = s.TlsService.Save(tx, act, data, hostname)
		if err == nil && defaultTLSSaveImpact.RefreshClientAndInboundData {
			objs = append(objs, "clients", "inbounds")
		}
		// TLS edits can rebuild managed subscription nodes through the
		// certificate-binding hook. Return both projection collections so the
		// subscription manager does not keep stale cards until a full reload.
		if err == nil && defaultTLSSaveImpact.SubscriptionProjectionChanged {
			objs = append(objs, "suboutbounds", "subgroups")
		}
	case "inbounds":
		nftAction, nftPlanErr := s.InboundService.Save(tx, act, data, initUsers, hostname)
		err = nftPlanErr
		// Inbound mutations also update client links and may create/remove
		// managed subscription outbounds. Return the shared collections on every
		// action so both default and Mihomo-facing subscription views converge.
		objs = append(objs, "clients", "suboutbounds", "subgroups")
		if err == nil {
			affectedClientIDs, syncErr := loadAffectedDefaultClientIDsForInboundSave(tx, act, data, initUsers)
			if syncErr != nil {
				return nil, common.NewErrorf("failed to identify affected client suboutbounds: %v", syncErr)
			}
			if syncErr := s.syncManagedDefaultClientsForIDs(tx, hostname, affectedClientIDs); syncErr != nil {
				return nil, common.NewErrorf("failed to sync affected client suboutbounds: %v", syncErr)
			}
		}
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
		// Outbound rename/delete operations rewrite references in panel groups.
		// The group editor must receive the updated tags in the same response.
		if err == nil {
			objs = append(objs, "outboundgroups", "suboutbounds", "subgroups")
		}
	case "outboundgroups":
		singboxOutboundGroupsRuntimeChanged, err = s.OutboundGroupService.saveWithRuntimeImpactLocked(tx, act, data)
		if singboxOutboundGroupsRuntimeChanged {
			objs = append(objs, "outbounds", "suboutbounds", "subgroups")
		}
	case "suboutbounds":
		err = s.SubOutboundService.Save(tx, act, data)
		// Save keeps subscription rendering validation in the post-commit hook.
		objs = append(objs, "subgroups")
	case "subgroups":
		err = s.SubGroupService.Save(tx, act, data)
		// Deleting an imported group also deletes its managed suboutbounds. Return
		// both collections for every group mutation so an empty result can clear
		// both default and Mihomo subscription views immediately.
		if err == nil {
			objs = append(objs, "suboutbounds")
		}
	case "services":
		err = s.ServicesService.Save(tx, act, data)
		if err == nil {
			objs = append(objs, "suboutbounds", "subgroups")
		}
	case "endpoints":
		err = s.EndpointService.Save(tx, act, data)
		if err == nil {
			objs = append(objs, "suboutbounds", "subgroups")
		}
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
		if err = validateSingboxConfigRouteBounds(normalizedConfig, tx); err != nil {
			return nil, err
		}

		err = s.SettingService.SaveConfig(tx, normalizedConfig)
		if err != nil {
			return nil, err
		}
		objs = append(objs, "suboutbounds", "subgroups")
	case "settings":
		err = s.SettingService.Save(tx, data)
		if err == nil {
			compactStatsAfterCommit = shouldCompactStatsAfterSettingsSave(data)
			objs = append(objs, "suboutbounds", "subgroups")
		}
		if err == nil {
			postCommitHooks = append(postCommitHooks, func() error {
				if panelTimeLocationChanged {
					if err := ReloadPanelTimeSchedule(); err != nil {
						logger.Warning("reload panel timezone schedule failed: ", err)
					}
					if err := ReschedulePendingCoreAutoChecksForPanelTimeZone(); err != nil {
						logger.Warning("reschedule pending core auto checks after panel timezone update failed: ", err)
					}
				}
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
		// TLS edits can rewrite inbound out_json, client links and their
		// Mihomo-managed subscription outbounds. Return every affected list so
		// all seven related pages converge without a full-page reload.
		objs = append(objs, "mihomo_inbounds", "mihomo_clients", "suboutbounds", "subgroups")
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
			affectedClientIDs, syncErr := loadAffectedMihomoClientIDsForInboundSave(tx, act, data, initUsers)
			if syncErr != nil {
				return nil, common.NewErrorf("failed to identify affected mihomo client suboutbounds: %v", syncErr)
			}
			if syncErr := s.syncManagedMihomoClientsForIDs(tx, hostname, affectedClientIDs); syncErr != nil {
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
		mihomoRuntimeChanged = err == nil
		// Renames and deletes also update panel group references and ShadowQUIC
		// proxy references, so the group selector must receive the same response.
		objs = append(objs, "mihomo_outboundgroups", "suboutbounds", "subgroups")
	case "mihomo_outboundgroups":
		mihomoRuntimeChanged, err = s.MihomoOutboundGroupService.saveWithRuntimeImpactLocked(tx, act, data)
		if mihomoRuntimeChanged {
			objs = append(objs, "mihomo_outbounds", "suboutbounds", "subgroups")
		}
	case "mihomo_config":
		err = s.MihomoConfigService.SaveConfig(tx, data)
		mihomoRuntimeChanged = err == nil
		if err == nil {
			objs = append(objs, "suboutbounds", "subgroups")
		}
	default:
		return nil, common.NewError("unknown object: ", obj)
	}
	if err != nil {
		return nil, err
	}
	if strings.HasPrefix(obj, "mihomo_") && obj != "mihomo_outboundgroups" {
		mihomoRuntimeChanged = true
	}
	if obj == "settings" || obj == "inbounds" || obj == "mihomo_inbounds" {
		if err := validatePortForwardListenerClaimsAgainstActiveRules(tx); err != nil {
			return nil, err
		}
	}
	if obj == "clients" || obj == "inbounds" {
		if err := validateManagedSubJSONFileNames(tx); err != nil {
			return nil, err
		}
	}
	// Group labels/order are panel metadata only. They must not invalidate the
	// route/DNS optimistic-concurrency revision unless the mutation actually
	// removes or changes runtime outbounds.
	singboxConfigMutation := isSingboxConfigMutationObject(obj) &&
		!(obj == "outboundgroups" && !singboxOutboundGroupsRuntimeChanged)
	if singboxConfigMutation {
		currentRevision, revisionErr := ensureSingboxConfigRevisionState(tx)
		if revisionErr != nil {
			return nil, revisionErr
		}
		if _, revisionErr = bumpSingboxConfigRevision(tx, currentRevision); revisionErr != nil {
			return nil, revisionErr
		}
	}
	dt := time.Now().Unix()
	auditData := data
	if obj == "config" {
		auditData = buildSingboxConfigChangeAudit(data)
	} else if obj == "mihomo_config" {
		auditData = buildMihomoConfigChangeAudit(data)
	}
	err = recordChange(tx, model.Changes{
		DateTime: dt,
		Actor:    loginUser,
		Key:      obj,
		Action:   act,
		Obj:      auditData,
	})
	if err != nil {
		return nil, err
	}
	if mihomoRuntimeChanged {
		mihomoManager := NewMihomoManagerService()
		if mihomoServerConfigLockHeld {
			// Keep the pre-acquired generator lock across commit and the matching
			// runtime-file write. The YAML is rendered from this transaction, but is
			// never written until the database state is durable.
			mihomoRenderedServerConfig, err = mihomoManager.renderServerConfig(tx)
			if err != nil {
				return nil, fmt.Errorf("invalid mihomo runtime config: %w", err)
			}
		} else if err := mihomoManager.ValidateServerConfig(tx); err != nil {
			return nil, fmt.Errorf("invalid mihomo runtime config: %w", err)
		}
	}

	return uniqueConfigSaveObjects(objs), nil
}

func uniqueConfigSaveObjects(objs []string) []string {
	if len(objs) < 2 {
		return objs
	}
	unique := make([]string, 0, len(objs))
	seen := make(map[string]struct{}, len(objs))
	for _, obj := range objs {
		if _, exists := seen[obj]; exists {
			continue
		}
		seen[obj] = struct{}{}
		unique = append(unique, obj)
	}
	return unique
}

func mihomoTLSIDFromSavePayload(data json.RawMessage) uint {
	var payload struct {
		ID uint `json:"id"`
	}
	if err := json.Unmarshal(data, &payload); err != nil {
		return 0
	}
	return payload.ID
}

func (s *ConfigService) applyInboundNftAction(action *InboundNftAction) error {
	if action == nil {
		return nil
	}
	return (&NftTrafficService{}).ApplyInboundNftAction(database.GetDB(), action)
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
	return runMihomoInboundNftMutation(func() error {
		return s.applyMihomoInboundNftActionLocked(action)
	})
}

func (s *ConfigService) applyMihomoInboundNftActionLocked(action *InboundNftAction) error {
	if action == nil {
		return nil
	}

	db := database.GetDB()
	nftSvc := &MihomoNftTrafficService{}
	coreSvc := &MihomoCoreManagerService{}
	coreRunning := coreSvc.IsRunning()

	var err error
	switch action.Kind {
	case "upsert":
		if coreRunning {
			err = nftSvc.UpdateInboundRules(db, action.InboundID, action.Tag, action.Port, action.PortHopRange, action.RedirectTCP)
		} else {
			err = nftSvc.UpsertInboundStateOnly(db, action.InboundID, action.Tag, action.Port, action.PortHopRange)
		}
	case "remove":
		if coreRunning {
			err = nftSvc.RemoveInboundRules(db, action.InboundID)
		} else {
			err = nftSvc.RemoveInboundStateOnly(db, action.InboundID)
		}
	default:
		err = common.NewError("unknown mihomo inbound nft action: ", action.Kind)
	}
	if err != nil {
		return err
	}

	if !coreRunning {
		nftSvc.cleanupOnShutdown()
	}

	return nil
}

func (s *ConfigService) CheckChanges(lu string) (bool, error) {
	return checkConfigRevisionChanges(lu, currentLastUpdate, func() { markLastUpdate(time.Now().Unix()) })
}

func (s *ConfigService) CheckMihomoChanges(lu string) (bool, error) {
	return checkConfigRevisionChanges(lu, func() int64 {
		return atomic.LoadInt64(&MihomoLastUpdate)
	}, func() { markMihomoLastUpdate(time.Now().Unix()) })
}

func checkConfigRevisionChanges(lu string, current func() int64, initialize func()) (bool, error) {
	if lu == "" {
		return true, nil
	}
	intLu, err := strconv.ParseInt(lu, 10, 64)
	if err != nil {
		return true, nil
	}
	lastUpdate := current()
	if lastUpdate == 0 {
		// After a panel restart the in-memory revision has no ordering relation to
		// an already-open browser. Force one full snapshot instead of comparing a
		// millisecond revision to the historical seconds-based audit table.
		initialize()
		return true, nil
	}
	// A browser can retain a revision from a previous process lifetime. Treat
	// both directions as changed so a newer process cannot be mistaken for an
	// already-synchronized snapshot.
	return lastUpdate != intLu, nil
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

// loadAffectedDefaultClientIDsForInboundSave returns clients whose managed
// subscription projections may have changed because an inbound was written.
// The inbound save itself updates local links and removes deleted-inbound
// records; this helper covers the remaining managed subscription rebuild.
func loadAffectedDefaultClientIDsForInboundSave(tx *gorm.DB, act string, data json.RawMessage, initUserIDs string) ([]uint, error) {
	if tx == nil {
		return nil, common.NewError("client transaction is nil")
	}
	switch act {
	case "new":
		return dedupeUintIDs(parseIDList(initUserIDs)), nil
	case "del":
		return []uint{}, nil
	case "edit":
		var inbound model.Inbound
		if err := inbound.UnmarshalJSON(data); err != nil {
			return nil, err
		}
		if inbound.Id == 0 {
			return []uint{}, nil
		}
		var clients []model.Client
		if err := tx.Model(model.Client{}).Select("id", "inbounds").Find(&clients).Error; err != nil {
			return nil, err
		}
		ids := make([]uint, 0, len(clients))
		for _, client := range clients {
			bound, err := parseClientInboundIDs(client.Inbounds)
			if err != nil {
				continue
			}
			if anyUintInSet(bound, uintSetFromSlice([]uint{inbound.Id})) {
				ids = append(ids, client.Id)
			}
		}
		managed, err := loadManagedClientIDsForInboundIDs(tx, subOutboundSourceClient, []uint{inbound.Id})
		if err != nil {
			return nil, err
		}
		ids = append(ids, managed...)
		return dedupeUintIDs(ids), nil
	default:
		return []uint{}, nil
	}
}

func (s *ConfigService) syncManagedDefaultClientsForIDs(tx *gorm.DB, hostname string, clientIDs []uint) error {
	if tx == nil {
		return nil
	}
	clientIDs = compactPositiveUintList(clientIDs)
	if len(clientIDs) == 0 {
		return nil
	}
	var clients []model.Client
	if err := tx.Model(model.Client{}).Where("id IN ?", clientIDs).Find(&clients).Error; err != nil {
		return err
	}
	for i := range clients {
		client := clients[i]
		if err := s.SyncService.SyncClientOnSave(tx, &client, &client, hostname); err != nil {
			return err
		}
	}
	return nil
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
		if id > 0 {
			skip[id] = struct{}{}
		}
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

func (s *ConfigService) syncManagedMihomoClientsForIDs(tx *gorm.DB, hostname string, clientIDs []uint) error {
	if tx == nil {
		return nil
	}
	clientIDs = compactPositiveUintList(clientIDs)
	if len(clientIDs) == 0 {
		return nil
	}

	var clients []model.MihomoClient
	if err := tx.Model(model.MihomoClient{}).Where("id IN ?", clientIDs).Find(&clients).Error; err != nil {
		return err
	}
	for _, client := range clients {
		oldClient := client
		newClient := client
		if err := s.MihomoSyncService.SyncClientOnSave(tx, &oldClient, &newClient, hostname); err != nil {
			return err
		}
	}
	return nil
}

func loadAffectedMihomoClientIDsForInboundSave(tx *gorm.DB, act string, data json.RawMessage, initUserIDs string) ([]uint, error) {
	if tx == nil {
		return nil, nil
	}
	var inboundID uint
	switch act {
	case "new":
		return dedupeUintIDs(parseIDList(initUserIDs)), nil
	case "edit":
		var inbound model.MihomoInbound
		if err := inbound.UnmarshalJSON(data); err != nil {
			return nil, err
		}
		inboundID = inbound.Id
	case "del":
		return []uint{}, nil
	}
	if inboundID == 0 {
		return []uint{}, nil
	}
	var clients []model.MihomoClient
	if err := tx.Model(model.MihomoClient{}).Select("id", "inbounds").Find(&clients).Error; err != nil {
		return nil, err
	}
	clients = filterMihomoClientsBoundToInboundIDs(clients, []uint{inboundID})
	clientIDs := make([]uint, 0, len(clients))
	for _, client := range clients {
		clientIDs = append(clientIDs, client.Id)
	}
	return compactPositiveUintList(clientIDs), nil
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

	var syncErr error
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
			syncErr = err
			break
		}
	}
	if syncErr != nil {
		DiscardManagedRuntimeHookScope(tx)
		tx.Rollback()
		return nil, syncErr
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
		markLastUpdate(time.Now().Unix())
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

	var syncErr error
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
			syncErr = err
			break
		}
	}
	if syncErr != nil {
		DiscardManagedRuntimeHookScope(tx)
		tx.Rollback()
		return nil, syncErr
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
		markBothLastUpdates(time.Now().Unix())
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
