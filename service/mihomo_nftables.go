package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/logger"
	"github.com/alireza0/s-ui/util"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type MihomoNftTrafficService struct{}

var mihomoClientBindingRepairNeeded atomic.Bool

func init() {
	mihomoClientBindingRepairNeeded.Store(true)
	database.RegisterDBResetHook(func() { mihomoClientBindingRepairNeeded.Store(true) })
}

type mihomoInboundTrafficSample struct {
	state                  model.MihomoInboundRedirectState
	originalInBytes        int64
	originalOutBytes       int64
	originalInHandle       int
	originalOutHandle      int
	originalRedirectHandle int
	stateChanged           bool
	delta                  inboundDelta
}

var mihomoPortHopRefreshState = struct {
	mu   sync.Mutex
	last map[uint]time.Time
}{
	last: map[uint]time.Time{},
}

// Mihomo has an independent rule/state model, but its nft commands still need
// one local mutation boundary. Without this lock an API save, port-hop refresh
// and sampler read could replace a handle between the counter snapshot and the
// SQLite baseline update, causing a missed or duplicated delta.
var mihomoInboundNftMutationMu sync.Mutex

func runMihomoInboundNftMutation(fn func() error) error {
	if fn == nil {
		return nil
	}
	mihomoInboundNftMutationMu.Lock()
	defer mihomoInboundNftMutationMu.Unlock()
	return fn()
}

func (s *MihomoNftTrafficService) IsNftTableReady() bool {
	return nftTableExists()
}

// EnsureRuleIntegrity verifies mihomo inbound nftables rules and recreates
// missing ones when rules are externally removed.
func (s *MihomoNftTrafficService) EnsureRuleIntegrity() error {
	if !IsSystemPlatformLinux() || !nftSupported() {
		return nil
	}

	coreSvc := &MihomoCoreManagerService{}
	if !coreSvc.IsRunning() {
		return nil
	}
	return s.EnsureRuleIntegrityWhenRunning()
}

// EnsureRuleIntegrityWhenRunning is for callers that already checked the
// Mihomo Core state in the current synchronization pass.
func (s *MihomoNftTrafficService) EnsureRuleIntegrityWhenRunning() error {
	return runMihomoInboundNftMutation(s.ensureRuleIntegrityWhenRunning)
}

func (s *MihomoNftTrafficService) ensureRuleIntegrityWhenRunning() error {
	if !IsSystemPlatformLinux() || !nftSupported() {
		return nil
	}

	db := database.GetDB()
	var inbounds []model.MihomoInbound
	if err := db.Find(&inbounds).Error; err != nil {
		return err
	}
	if len(inbounds) == 0 {
		return nil
	}
	unsupportedInboundIDs := make([]uint, 0)
	supportedInbounds := make([]model.MihomoInbound, 0, len(inbounds))
	for _, inbound := range inbounds {
		if !isSupportedMihomoInboundType(inbound.Type) {
			unsupportedInboundIDs = append(unsupportedInboundIDs, inbound.Id)
			continue
		}
		supportedInbounds = append(supportedInbounds, inbound)
	}
	if len(unsupportedInboundIDs) > 0 {
		if err := s.removeUnsupportedInboundStates(db, unsupportedInboundIDs); err != nil {
			return err
		}
	}
	inbounds = supportedInbounds
	if len(inbounds) == 0 {
		return nil
	}
	inHandles, err := snapshotManagedRuleHandles(nftChainIn, mihomoNftRuleComments.prefix)
	if err != nil {
		return err
	}
	outHandles, err := snapshotManagedRuleHandles(nftChainOut, mihomoNftRuleComments.prefix)
	if err != nil {
		return err
	}
	redirectRules, err := snapshotManagedRedirectRules(mihomoNftRuleComments.prefix)
	if err != nil {
		return err
	}

	validComments := make(map[string]struct{}, len(inbounds)*3)
	for _, inbound := range inbounds {
		port := extractPort(inbound.Options)
		if port <= 0 {
			continue
		}
		validComments[mihomoNftRuleComments.in(inbound.Tag)] = struct{}{}
		validComments[mihomoNftRuleComments.out(inbound.Tag)] = struct{}{}
		redirectRange, _ := resolveMihomoInboundRedirectSpec(&inbound)
		if strings.TrimSpace(redirectRange) != "" {
			validComments[mihomoNftRuleComments.redirect(inbound.Tag)] = struct{}{}
		}
	}

	var firstErr error
	for _, inbound := range inbounds {
		port := extractPort(inbound.Options)
		if port <= 0 {
			continue
		}
		redirectRange, redirectTCP := resolveMihomoInboundRedirectSpec(&inbound)
		if err := s.ensureInboundRuleIntegrity(db, &inbound, port, redirectRange, redirectTCP, inHandles, outHandles, redirectRules); err != nil {
			logger.Warning("mihomo nft integrity check failed for inbound ", inbound.Tag, ": ", err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}

	if err := s.cleanupOrphanInboundRules(validComments, inHandles, outHandles, redirectRules.comments); err != nil {
		logger.Warning("cleanup orphan mihomo nft rules failed: ", err)
		if firstErr == nil {
			firstErr = err
		}
	}

	return firstErr
}

func (s *MihomoNftTrafficService) cleanupOrphanInboundRules(validComments map[string]struct{}, inHandles map[string]int, outHandles map[string]int, redirectComments map[string]struct{}) error {
	var firstErr error
	cleanupChain := func(chain string, handles map[string]int) {
		for comment, handle := range handles {
			if _, ok := validComments[comment]; ok {
				continue
			}
			if err := deleteRuleByHandle(chain, handle); err != nil && !nftObjectMissing(err) {
				logger.Warning("failed to delete orphan mihomo nft rule ", comment, " handle ", handle, ": ", err)
				if firstErr == nil {
					firstErr = err
				}
			}
		}
	}
	cleanupChain(nftChainIn, inHandles)
	cleanupChain(nftChainOut, outHandles)
	for comment := range redirectComments {
		if _, ok := validComments[comment]; ok {
			continue
		}
		if err := deleteNftRedirectRulesByComment(comment); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (s *MihomoNftTrafficService) ensureInboundRuleIntegrity(tx *gorm.DB, inbound *model.MihomoInbound, port int, portHopRange string, redirectTCP bool, inHandles map[string]int, outHandles map[string]int, redirectRules *managedRedirectRuleSnapshot) error {
	if inbound == nil || inbound.Id == 0 || port <= 0 {
		return nil
	}

	var state model.MihomoInboundRedirectState
	result := tx.Where("inbound_id = ?", inbound.Id).First(&state)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return s.SetupInboundRules(tx, inbound.Id, inbound.Tag, port, portHopRange, redirectTCP)
	}
	if result.Error != nil {
		return result.Error
	}

	if state.Tag != inbound.Tag || state.Port != port || state.PortHopRange != portHopRange {
		return s.UpdateInboundRules(tx, inbound.Id, inbound.Tag, port, portHopRange, redirectTCP)
	}

	originalInHandle := state.InHandle
	originalOutHandle := state.OutHandle
	missing := false

	if handle, ok := inHandles[mihomoNftRuleComments.in(inbound.Tag)]; ok {
		state.InHandle = handle
	} else {
		missing = true
	}

	if handle, ok := outHandles[mihomoNftRuleComments.out(inbound.Tag)]; ok {
		state.OutHandle = handle
	} else {
		missing = true
	}

	if strings.TrimSpace(portHopRange) != "" {
		comment := mihomoNftRuleComments.redirect(inbound.Tag)
		if redirectRules == nil || !redirectRules.valid[comment] {
			missing = true
		}
	} else if state.RedirectHandle > 0 || redirectRules.hasComment(mihomoNftRuleComments.redirect(inbound.Tag)) {
		if err := deleteNftRedirectRulesByComment(mihomoNftRuleComments.redirect(inbound.Tag)); err != nil {
			logger.Warning("failed to delete stale mihomo redirect rule for inbound ", inbound.Tag, ": ", err)
		}
		if err := tx.Model(&state).Updates(map[string]interface{}{
			"redirect_handle": 0,
			"updated_at":      time.Now(),
		}).Error; err != nil {
			return err
		}
		state.RedirectHandle = 0
	}

	if !missing {
		if state.InHandle != originalInHandle || state.OutHandle != originalOutHandle {
			return tx.Model(&state).Updates(map[string]interface{}{
				"in_handle":  state.InHandle,
				"out_handle": state.OutHandle,
				"updated_at": time.Now(),
			}).Error
		}
		return nil
	}

	if err := s.removeRulesFromState(&state); err != nil {
		logger.Warning("failed to remove stale mihomo nft rules for inbound ", inbound.Tag, ": ", err)
	}
	if err := tx.Delete(&state).Error; err != nil {
		return err
	}
	return s.SetupInboundRules(tx, inbound.Id, inbound.Tag, port, portHopRange, redirectTCP)
}

func (s *MihomoNftTrafficService) SetupInboundRules(tx *gorm.DB, inboundID uint, tag string, port int, portHopRange string, redirectTCP bool) error {
	if inboundID == 0 {
		return nil
	}
	if port <= 0 {
		return s.RemoveInboundRules(tx, inboundID)
	}

	var existing model.MihomoInboundRedirectState
	result := tx.Where("inbound_id = ?", inboundID).First(&existing)
	if result.Error == nil {
		if existing.Tag == tag && existing.Port == port && existing.PortHopRange == portHopRange {
			return nil
		}
		if err := s.removeRulesFromState(&existing); err != nil {
			logger.Warning("failed to remove old mihomo nft rules for inbound ", tag, ": ", err)
		}
		_ = tx.Delete(&existing).Error
	}

	inHandle, inErr := addPortCounterRule(nftChainIn, port, "dport", mihomoNftRuleComments.in(tag))
	if inErr != nil {
		logger.Warning("failed to add mihomo nft input counter rule for inbound ", tag, ": ", inErr)
	}
	outHandle, outErr := addPortCounterRule(nftChainOut, port, "sport", mihomoNftRuleComments.out(tag))
	if outErr != nil {
		logger.Warning("failed to add mihomo nft output counter rule for inbound ", tag, ": ", outErr)
	}

	redirectHandle := 0
	hopNft, skipped, sample := portHopRangeToNftWithExclusions(portHopRange, port)
	if skipped > 0 {
		if len(sample) > 0 {
			logger.Info("mihomo port hop range for inbound ", tag, ": skipped ", skipped, " UDP ports (sample ", sample, ")")
		} else {
			logger.Info("mihomo port hop range for inbound ", tag, ": skipped ", skipped, " UDP ports")
		}
	}
	if hopNft != "" {
		handle, err := addRedirectRuleWithProtocols(hopNft, port, mihomoNftRuleComments.redirect(tag), redirectTCP)
		if err != nil {
			logger.Warning("failed to add mihomo REDIRECT rule for inbound ", tag, ": ", err)
		} else {
			redirectHandle = handle
		}
	}

	now := time.Now()
	state := model.MihomoInboundRedirectState{
		InboundId:      inboundID,
		Tag:            tag,
		Port:           port,
		PortHopRange:   portHopRange,
		InHandle:       inHandle,
		OutHandle:      outHandle,
		RedirectHandle: redirectHandle,
		InBytes:        0,
		OutBytes:       0,
		UpdatedAt:      now,
		CreatedAt:      now,
	}
	clearMihomoPortHopRefresh(inboundID)
	return tx.Create(&state).Error
}

// Backward-compatible wrapper.
func (s *MihomoNftTrafficService) SetupInboundRedirect(tx *gorm.DB, inboundID uint, tag string, port int, portHopRange string) error {
	return s.SetupInboundRules(tx, inboundID, tag, port, portHopRange, false)
}

func (s *MihomoNftTrafficService) UpdateInboundRules(tx *gorm.DB, inboundID uint, tag string, newPort int, portHopRange string, redirectTCP bool) error {
	if inboundID == 0 {
		return nil
	}
	if newPort <= 0 {
		return s.RemoveInboundRules(tx, inboundID)
	}

	var existing model.MihomoInboundRedirectState
	result := tx.Where("inbound_id = ?", inboundID).First(&existing)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return s.SetupInboundRules(tx, inboundID, tag, newPort, portHopRange, redirectTCP)
	}
	if result.Error != nil {
		return result.Error
	}

	if existing.Port == newPort && existing.PortHopRange == portHopRange {
		if existing.Tag == tag {
			return s.reconcileRedirectRuleSpec(tx, &existing, redirectTCP)
		}
		return s.updateInboundTag(tx, &existing, tag, redirectTCP)
	}

	if err := s.removeRulesFromState(&existing); err != nil {
		logger.Warning("failed to remove old mihomo nft rules for inbound ", existing.Tag, ": ", err)
	}
	if err := tx.Delete(&existing).Error; err != nil {
		return err
	}
	return s.SetupInboundRules(tx, inboundID, tag, newPort, portHopRange, redirectTCP)
}

// Backward-compatible wrapper.
func (s *MihomoNftTrafficService) UpdateInboundRedirect(tx *gorm.DB, inboundID uint, tag string, port int, portHopRange string) error {
	return s.UpdateInboundRules(tx, inboundID, tag, port, portHopRange, false)
}

// reconcileRedirectRuleSpec keeps redirect protocol behavior aligned with the
// current inbound type even when tag/port/range remain unchanged.
func (s *MihomoNftTrafficService) reconcileRedirectRuleSpec(tx *gorm.DB, state *model.MihomoInboundRedirectState, redirectTCP bool) error {
	if tx == nil || state == nil {
		return nil
	}

	if strings.TrimSpace(state.PortHopRange) == "" || state.Port <= 0 {
		comment := mihomoNftRuleComments.redirect(state.Tag)
		if state.RedirectHandle <= 0 && !nftRedirectRuleExistsInAnyLayout(comment) {
			return nil
		}
		if err := s.removeRedirectRule(state); err != nil {
			logger.Warning("failed to remove stale mihomo REDIRECT rule for inbound ", state.Tag, ": ", err)
		}
		state.RedirectHandle = 0
		return tx.Model(state).Updates(map[string]interface{}{
			"redirect_handle": 0,
			"updated_at":      time.Now(),
		}).Error
	}

	if err := s.removeRedirectRule(state); err != nil {
		logger.Warning("failed to delete existing mihomo REDIRECT rule for inbound ", state.Tag, ": ", err)
	}

	hopNft, skipped, sample := portHopRangeToNftWithExclusions(state.PortHopRange, state.Port)
	if skipped > 0 {
		if len(sample) > 0 {
			logger.Info("mihomo port hop range for inbound ", state.Tag, ": skipped ", skipped, " UDP ports (sample ", sample, ")")
		} else {
			logger.Info("mihomo port hop range for inbound ", state.Tag, ": skipped ", skipped, " UDP ports")
		}
	}

	redirectHandle := 0
	if hopNft != "" {
		handle, err := addRedirectRuleWithProtocols(hopNft, state.Port, mihomoNftRuleComments.redirect(state.Tag), redirectTCP)
		if err != nil {
			return err
		}
		redirectHandle = handle
	}

	state.RedirectHandle = redirectHandle
	return tx.Model(state).Updates(map[string]interface{}{
		"redirect_handle": redirectHandle,
		"updated_at":      time.Now(),
	}).Error
}

func (s *MihomoNftTrafficService) updateInboundTag(tx *gorm.DB, state *model.MihomoInboundRedirectState, newTag string, redirectTCP bool) error {
	oldTag := state.Tag
	if oldTag == newTag {
		return nil
	}

	if state.InHandle <= 0 {
		if handle := findHandleByComment(nftChainIn, mihomoNftRuleComments.in(oldTag)); handle > 0 {
			state.InHandle = handle
		}
	}
	if state.OutHandle <= 0 {
		if handle := findHandleByComment(nftChainOut, mihomoNftRuleComments.out(oldTag)); handle > 0 {
			state.OutHandle = handle
		}
	}
	if state.InHandle <= 0 || state.OutHandle <= 0 {
		logger.Warning("mihomo tag change for inbound ", oldTag, " -> ", newTag, " requires rule recreation (missing nft handles)")
		if err := s.removeRulesFromState(state); err != nil {
			logger.Warning("failed to remove old mihomo nft rules during tag change: ", err)
		}
		if err := tx.Delete(state).Error; err != nil {
			return err
		}
		return s.SetupInboundRules(tx, state.InboundId, newTag, state.Port, state.PortHopRange, redirectTCP)
	}

	if strings.TrimSpace(state.PortHopRange) != "" {
		if err := deleteNftRedirectRulesByComment(mihomoNftRuleComments.redirect(oldTag)); err != nil {
			logger.Warning("failed to delete old mihomo REDIRECT rule for inbound ", oldTag, ": ", err)
		}

		hopNft, skipped, sample := portHopRangeToNftWithExclusions(state.PortHopRange, state.Port)
		if skipped > 0 {
			if len(sample) > 0 {
				logger.Info("mihomo port hop range for inbound ", newTag, ": skipped ", skipped, " UDP ports (sample ", sample, ")")
			} else {
				logger.Info("mihomo port hop range for inbound ", newTag, ": skipped ", skipped, " UDP ports")
			}
		}
		if hopNft != "" {
			redirectHandle, err := addRedirectRuleWithProtocols(hopNft, state.Port, mihomoNftRuleComments.redirect(newTag), redirectTCP)
			if err != nil {
				logger.Warning("failed to add mihomo REDIRECT rule for inbound ", newTag, ": ", err)
			}
			state.RedirectHandle = redirectHandle
		} else {
			state.RedirectHandle = 0
		}
	}

	state.Tag = newTag
	return tx.Model(state).Updates(map[string]interface{}{
		"tag":             newTag,
		"in_handle":       state.InHandle,
		"out_handle":      state.OutHandle,
		"redirect_handle": state.RedirectHandle,
		"updated_at":      time.Now(),
	}).Error
}

func (s *MihomoNftTrafficService) UpsertInboundStateOnly(tx *gorm.DB, inboundID uint, tag string, port int, portHopRange string) error {
	if inboundID == 0 {
		return nil
	}
	if port <= 0 {
		return s.RemoveInboundStateOnly(tx, inboundID)
	}

	now := time.Now()
	var state model.MihomoInboundRedirectState
	result := tx.Where("inbound_id = ?", inboundID).First(&state)
	if result.Error == nil {
		clearMihomoPortHopRefresh(inboundID)
		return tx.Model(&state).Updates(map[string]interface{}{
			"tag":             tag,
			"port":            port,
			"port_hop_range":  portHopRange,
			"in_handle":       0,
			"out_handle":      0,
			"redirect_handle": 0,
			"in_bytes":        0,
			"out_bytes":       0,
			"updated_at":      now,
		}).Error
	}
	if !errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return result.Error
	}

	clearMihomoPortHopRefresh(inboundID)
	return tx.Create(&model.MihomoInboundRedirectState{
		InboundId:      inboundID,
		Tag:            tag,
		Port:           port,
		PortHopRange:   portHopRange,
		InHandle:       0,
		OutHandle:      0,
		RedirectHandle: 0,
		InBytes:        0,
		OutBytes:       0,
		UpdatedAt:      now,
		CreatedAt:      now,
	}).Error
}

func (s *MihomoNftTrafficService) RemoveInboundStateOnly(tx *gorm.DB, inboundID uint) error {
	if inboundID == 0 {
		return nil
	}
	if err := tx.Where("inbound_id = ?", inboundID).Delete(&model.MihomoClientInboundTrafficState{}).Error; err != nil {
		return err
	}
	if err := tx.Where("inbound_id = ?", inboundID).Delete(&model.MihomoInboundRedirectState{}).Error; err != nil {
		return err
	}
	clearMihomoPortHopRefresh(inboundID)
	return nil
}

func (s *MihomoNftTrafficService) RemoveInboundRules(tx *gorm.DB, inboundID uint) error {
	var state model.MihomoInboundRedirectState
	if err := tx.Where("inbound_id = ?", inboundID).First(&state).Error; err != nil {
		return nil
	}

	if err := s.removeRulesFromState(&state); err != nil {
		logger.Warning("failed to remove mihomo nft rules for inbound ", state.Tag, ": ", err)
	}
	if err := tx.Where("inbound_id = ?", inboundID).Delete(&model.MihomoClientInboundTrafficState{}).Error; err != nil {
		return err
	}
	if err := tx.Delete(&state).Error; err != nil {
		return err
	}
	clearMihomoPortHopRefresh(inboundID)
	return nil
}

// Backward-compatible wrapper.
func (s *MihomoNftTrafficService) RemoveInboundRedirect(tx *gorm.DB, inboundID uint) error {
	return s.RemoveInboundRules(tx, inboundID)
}

func (s *MihomoNftTrafficService) removeRulesFromState(state *model.MihomoInboundRedirectState) error {
	if state == nil {
		return nil
	}

	var firstErr error
	if state.InHandle > 0 {
		if err := deleteRuleByHandle(nftChainIn, state.InHandle); err != nil {
			firstErr = err
		}
	} else if err := deleteRuleByComment(nftChainIn, mihomoNftRuleComments.in(state.Tag)); err != nil && firstErr == nil {
		firstErr = err
	}

	if state.OutHandle > 0 {
		if err := deleteRuleByHandle(nftChainOut, state.OutHandle); err != nil && firstErr == nil {
			firstErr = err
		}
	} else if err := deleteRuleByComment(nftChainOut, mihomoNftRuleComments.out(state.Tag)); err != nil && firstErr == nil {
		firstErr = err
	}

	if state.RedirectHandle > 0 || strings.TrimSpace(state.PortHopRange) != "" {
		if err := deleteNftRedirectRulesByComment(mihomoNftRuleComments.redirect(state.Tag)); err != nil && firstErr == nil {
			firstErr = err
		}
	}

	return firstErr
}

func (s *MihomoNftTrafficService) removeRedirectRule(state *model.MihomoInboundRedirectState) error {
	if state == nil {
		return nil
	}
	return deleteNftRedirectRulesByComment(mihomoNftRuleComments.redirect(state.Tag))
}

// SyncClientBindings synchronizes MihomoClientInboundTrafficState rows for one client.
// New or re-activated bindings start from current nft counters (zero residue).
func (s *MihomoNftTrafficService) SyncClientBindings(tx *gorm.DB, clientID uint, newInboundIDs []uint) error {
	return s.syncClientBindings(tx, clientID, newInboundIDs, nil)
}

// QueueSyncClientBindings delays nft counter reads until after the caller
// commits, keeping SQLite's only connection available while nft is running.
func (s *MihomoNftTrafficService) QueueSyncClientBindings(tx *gorm.DB, clientID uint, inboundIDs []uint) error {
	ids := append([]uint(nil), inboundIDs...)
	return QueueManagedRuntimeHook(tx, func() error {
		if err := s.SyncClientBindings(database.GetDB(), clientID, ids); err != nil {
			mihomoClientBindingRepairNeeded.Store(true)
			logger.Warning("failed to sync mihomo client traffic bindings for client id ", clientID, ": ", err)
		}
		return nil
	})
}

func (s *MihomoNftTrafficService) syncClientBindings(tx *gorm.DB, clientID uint, newInboundIDs []uint, snapshots map[uint]inboundCounterSnapshot) error {
	if clientID == 0 {
		return nil
	}

	var existingBindings []model.MihomoClientInboundTrafficState
	if err := tx.Where("client_id = ?", clientID).Find(&existingBindings).Error; err != nil {
		return err
	}

	existingMap := make(map[uint]*model.MihomoClientInboundTrafficState, len(existingBindings))
	for i := range existingBindings {
		existingMap[existingBindings[i].InboundId] = &existingBindings[i]
	}

	newSet := make(map[uint]struct{}, len(newInboundIDs))
	for _, inboundID := range newInboundIDs {
		newSet[inboundID] = struct{}{}
	}

	for inboundID, binding := range existingMap {
		if _, ok := newSet[inboundID]; ok {
			continue
		}
		if !binding.Active {
			continue
		}
		binding.Active = false
		binding.UpdatedAt = time.Now()
		if err := tx.Save(binding).Error; err != nil {
			return err
		}
	}

	now := time.Now()
	for _, inboundID := range newInboundIDs {
		if existing, ok := existingMap[inboundID]; ok {
			if existing.Active {
				continue
			}
			currentIn, currentOut := s.currentInboundBytesForBinding(tx, inboundID, snapshots)
			existing.Active = true
			existing.LastInBytes = currentIn
			existing.LastOutBytes = currentOut
			existing.UsedInBytes = 0
			existing.UsedOutBytes = 0
			existing.UpdatedAt = now
			if err := tx.Save(existing).Error; err != nil {
				return err
			}
			continue
		}

		currentIn, currentOut := s.currentInboundBytesForBinding(tx, inboundID, snapshots)
		binding := model.MihomoClientInboundTrafficState{
			ClientId:     clientID,
			InboundId:    inboundID,
			Active:       true,
			LastInBytes:  currentIn,
			LastOutBytes: currentOut,
			UsedInBytes:  0,
			UsedOutBytes: 0,
			CreatedAt:    now,
			UpdatedAt:    now,
		}
		if err := tx.Create(&binding).Error; err != nil {
			return err
		}
	}

	return nil
}

func (s *MihomoNftTrafficService) currentInboundBytesForBinding(tx *gorm.DB, inboundID uint, snapshots map[uint]inboundCounterSnapshot) (int64, int64) {
	if snapshot, ok := snapshots[inboundID]; ok {
		return snapshot.inBytes, snapshot.outBytes
	}
	// During periodic collection this map is non-nil and the caller already
	// holds an explicit SQLite transaction. Do not fall back to nft there when
	// an inbound was created after the pre-transaction snapshot was read.
	if snapshots != nil {
		return s.getPersistedInboundBytes(tx, inboundID)
	}
	return s.getCurrentInboundBytes(tx, inboundID)
}

func (s *MihomoNftTrafficService) getPersistedInboundBytes(db *gorm.DB, inboundID uint) (int64, int64) {
	if db == nil || inboundID == 0 {
		return 0, 0
	}

	var state model.MihomoInboundRedirectState
	if err := db.Select("in_bytes", "out_bytes").Where("inbound_id = ?", inboundID).First(&state).Error; err != nil {
		return 0, 0
	}
	return state.InBytes, state.OutBytes
}

func (s *MihomoNftTrafficService) DeleteClientBindings(tx *gorm.DB, clientID uint) error {
	if clientID == 0 {
		return nil
	}
	return tx.Where("client_id = ?", clientID).Delete(&model.MihomoClientInboundTrafficState{}).Error
}

// ResetClientTraffic resets mihomo client up/down and active binding accumulators.
func (s *MihomoNftTrafficService) ResetClientTraffic(tx *gorm.DB, clientID uint) error {
	return runMihomoInboundNftMutation(func() error {
		return s.resetClientTraffic(tx, clientID)
	})
}

func (s *MihomoNftTrafficService) resetClientTraffic(tx *gorm.DB, clientID uint) error {
	if clientID == 0 {
		return nil
	}

	if err := tx.Model(&model.MihomoClient{}).Where("id = ?", clientID).Updates(map[string]interface{}{
		"up":         0,
		"down":       0,
		"last_reset": time.Now().Unix(),
	}).Error; err != nil {
		return err
	}

	var bindings []model.MihomoClientInboundTrafficState
	if err := tx.Where("client_id = ? AND active = ?", clientID, true).Find(&bindings).Error; err != nil {
		return err
	}

	now := time.Now()
	for i := range bindings {
		b := &bindings[i]
		currentIn, currentOut := s.getCurrentInboundBytes(tx, b.InboundId)
		b.LastInBytes = currentIn
		b.LastOutBytes = currentOut
		b.UsedInBytes = 0
		b.UsedOutBytes = 0
		b.UpdatedAt = now
		if err := tx.Save(b).Error; err != nil {
			return err
		}
	}

	return nil
}

// QueueClientTrafficReset keeps nft counter reads outside the transaction that
// changed the mihomo client record.
func (s *MihomoNftTrafficService) QueueClientTrafficReset(tx *gorm.DB, clientID uint) error {
	return QueueManagedRuntimeHook(tx, func() error {
		if err := s.ResetClientTraffic(database.GetDB(), clientID); err != nil {
			logger.Warning("failed to reset mihomo client nft traffic baseline for client id ", clientID, ": ", err)
		}
		return nil
	})
}

func (s *MihomoNftTrafficService) getCurrentInboundBytes(tx *gorm.DB, inboundID uint) (int64, int64) {
	if inboundID == 0 {
		return 0, 0
	}

	var state model.MihomoInboundRedirectState
	if err := tx.Where("inbound_id = ?", inboundID).First(&state).Error; err != nil {
		return 0, 0
	}

	inBytes, inErr := getChainRuleBytesByHandle(nftChainIn, state.InHandle)
	if inErr != nil {
		s.tryRecoverHandles(tx, &state)
		inBytes, inErr = getChainRuleBytesByHandle(nftChainIn, state.InHandle)
		if inErr != nil {
			inBytes = state.InBytes
		}
	}

	outBytes, outErr := getChainRuleBytesByHandle(nftChainOut, state.OutHandle)
	if outErr != nil {
		s.tryRecoverHandles(tx, &state)
		outBytes, outErr = getChainRuleBytesByHandle(nftChainOut, state.OutHandle)
		if outErr != nil {
			outBytes = state.OutBytes
		}
	}

	return inBytes, outBytes
}

// RefreshPortHopRedirects refreshes REDIRECT rules according to port_hop_interval.
func (s *MihomoNftTrafficService) RefreshPortHopRedirects() error {
	return runMihomoInboundNftMutation(s.refreshPortHopRedirects)
}

func (s *MihomoNftTrafficService) refreshPortHopRedirects() error {
	if !IsSystemPlatformLinux() || !nftSupported() {
		return nil
	}

	coreSvc := &MihomoCoreManagerService{}
	if !coreSvc.IsRunning() {
		return nil
	}

	db := database.GetDB()
	var states []model.MihomoInboundRedirectState
	if err := db.Where("port_hop_range <> ''").Find(&states).Error; err != nil {
		return err
	}
	if len(states) == 0 {
		return nil
	}

	inboundIDs := make([]uint, 0, len(states))
	for _, state := range states {
		if state.InboundId > 0 {
			inboundIDs = append(inboundIDs, state.InboundId)
		}
	}
	if len(inboundIDs) == 0 {
		return nil
	}

	var inbounds []model.MihomoInbound
	if err := db.Model(&model.MihomoInbound{}).Select("id, type, options").Where("id IN ?", inboundIDs).Find(&inbounds).Error; err != nil {
		return err
	}
	inboundsByID := make(map[uint]model.MihomoInbound, len(inbounds))
	for _, inbound := range inbounds {
		inboundsByID[inbound.Id] = inbound
	}

	var firstErr error
	for i := range states {
		state := states[i]
		if state.Port <= 0 || strings.TrimSpace(state.PortHopRange) == "" {
			continue
		}

		inbound, ok := inboundsByID[state.InboundId]
		if !ok {
			clearMihomoPortHopRefresh(state.InboundId)
			logger.Warning("skip mihomo port hop refresh because inbound no longer exists: ", state.Tag)
			continue
		}
		if !isSupportedMihomoInboundType(inbound.Type) {
			if err := s.RemoveInboundRules(db, state.InboundId); err != nil {
				logger.Warning("failed to remove unsupported mihomo inbound nft state for ", state.Tag, ": ", err)
			}
			continue
		}
		normalizedRange, rangeErr := normalizeMihomoPortHopRange(state.PortHopRange)
		if rangeErr != nil {
			clearMihomoPortHopRefresh(state.InboundId)
			if err := s.removeRedirectRule(&state); err != nil {
				logger.Warning("failed to remove invalid mihomo port hop redirect for inbound ", state.Tag, ": ", err)
			}
			if err := db.Model(&model.MihomoInboundRedirectState{}).Where("id = ?", state.Id).Updates(map[string]interface{}{
				"port_hop_range":  "",
				"redirect_handle": 0,
				"updated_at":      time.Now(),
			}).Error; err != nil {
				logger.Warning("failed to clear invalid mihomo port hop range for inbound ", state.Tag, ": ", err)
			}
			logger.Warning("skip invalid mihomo port hop range for inbound ", state.Tag, ": ", rangeErr)
			continue
		}
		originalRange := state.PortHopRange
		interval, ok := parseMihomoPortHopInterval(extractPortHopInterval(inbound.Options))
		if !ok {
			clearMihomoPortHopRefresh(state.InboundId)
			continue
		}

		now := time.Now()
		if !shouldRefreshMihomoPortHop(state.InboundId, now, interval) {
			continue
		}

		// Run the potentially slow nft commands before the short SQLite update.
		if err := s.removeRedirectRule(&state); err != nil {
			logger.Warning("failed to delete existing mihomo REDIRECT rule for inbound ", state.Tag, ": ", err)
		}

		hopNft, skipped, sample := portHopRangeToNftWithExclusions(normalizedRange, state.Port)
		if skipped > 0 {
			if len(sample) > 0 {
				logger.Info("mihomo port hop interval refresh for inbound ", state.Tag, ": skipped ", skipped, " UDP ports (sample ", sample, ")")
			} else {
				logger.Info("mihomo port hop interval refresh for inbound ", state.Tag, ": skipped ", skipped, " UDP ports")
			}
		}

		redirectHandle := 0
		if hopNft != "" {
			handle, err := addRedirectRuleWithProtocols(hopNft, state.Port, mihomoNftRuleComments.redirect(state.Tag), inbound.Type == "mieru")
			if err != nil {
				logger.Warning("failed to refresh mihomo port hop redirect for inbound ", state.Tag, ": ", err)
				if firstErr == nil {
					firstErr = err
				}
				continue
			}
			redirectHandle = handle
		}

		result := db.Model(&model.MihomoInboundRedirectState{}).
			Where("id = ? AND tag = ? AND port = ? AND port_hop_range = ? AND redirect_handle = ?",
				state.Id, state.Tag, state.Port, originalRange, state.RedirectHandle).
			Updates(map[string]interface{}{
				"port_hop_range":  normalizedRange,
				"redirect_handle": redirectHandle,
				"updated_at":      now,
			})
		if result.Error != nil {
			logger.Warning("failed to save mihomo port hop redirect handle for inbound ", state.Tag, ": ", result.Error)
			if firstErr == nil {
				firstErr = result.Error
			}
			if err := deleteNftRedirectRulesByComment(mihomoNftRuleComments.redirect(state.Tag)); err != nil {
				logger.Warning("failed to remove unsaved mihomo REDIRECT rule for inbound ", state.Tag, ": ", err)
			}
			continue
		}
		if result.RowsAffected == 0 {
			logger.Warning("skip stale mihomo port hop redirect result for inbound ", state.Tag)
			if err := deleteNftRedirectRulesByComment(mihomoNftRuleComments.redirect(state.Tag)); err != nil {
				logger.Warning("failed to remove stale mihomo REDIRECT rule for inbound ", state.Tag, ": ", err)
			}
			continue
		}
		markMihomoPortHopRefreshed(state.InboundId, now)
	}

	return firstErr
}

// CollectAndSaveTraffic reads mihomo nft counters, writes inbound/client stats,
// and updates cumulative counters in MihomoInboundRedirectState. Its boolean
// result reports whether a traffic delta was committed.
func (s *MihomoNftTrafficService) CollectAndSaveTraffic() (bool, error) {
	return s.collectAndSaveTraffic(nil)
}

// CollectAndSaveTrafficWithHistory lets the serialized runtime sampler reuse
// its one trafficAge read for both Core chains without changing the legacy
// no-argument method contract.
func (s *MihomoNftTrafficService) CollectAndSaveTrafficWithHistory(saveTraffic bool) (bool, error) {
	return s.collectAndSaveTraffic(&saveTraffic)
}

func (s *MihomoNftTrafficService) collectAndSaveTraffic(saveTrafficOverride *bool) (bool, error) {
	var changed bool
	err := runMihomoInboundNftMutation(func() error {
		var collectErr error
		changed, collectErr = s.collectAndSaveTrafficLocked(saveTrafficOverride)
		return collectErr
	})
	return changed, err
}

func (s *MihomoNftTrafficService) collectAndSaveTrafficLocked(saveTrafficOverride *bool) (bool, error) {
	if !IsSystemPlatformLinux() || !nftSupported() {
		return false, nil
	}
	samplingEpoch, samplingAllowed := captureRuntimeTrafficSamplingEpoch()
	if !samplingAllowed {
		return false, nil
	}

	coreSvc := &MihomoCoreManagerService{}
	if !coreSvc.IsRunning() {
		setMihomoOnlines(nil, nil)
		return false, nil
	}

	db := database.GetDB()
	var states []model.MihomoInboundRedirectState
	if err := db.Find(&states).Error; err != nil {
		return false, err
	}

	if len(states) == 0 {
		// Legacy self-heal: old deployments may have mihomo inbounds but no nft state rows yet.
		s.initOnStartup()
		if err := db.Find(&states).Error; err != nil {
			return false, err
		}
		if len(states) == 0 {
			setMihomoOnlines(nil, nil)
			return false, nil
		}
	}

	saveTraffic := true
	if saveTrafficOverride != nil {
		saveTraffic = *saveTrafficOverride
	} else if trafficAge, err := (&SettingService{}).GetTrafficAge(); err == nil {
		saveTraffic = trafficAge > 0
	} else {
		logger.Warning("failed to load trafficAge for mihomo nft collection: ", err)
	}
	if saveTraffic {
		if err := EnsureHistoryStorageReady(); err != nil {
			return false, err
		}
		if err := runtimeTrafficStats.ensureReady(); err != nil {
			return false, err
		}
	}

	// Read nft counters before opening SQLite's single connection transaction.
	samples := s.readInboundTrafficSamples(states)
	snapshots := make(map[uint]inboundCounterSnapshot, len(states))
	for _, state := range states {
		snapshots[state.InboundId] = inboundCounterSnapshot{
			inBytes:  state.InBytes,
			outBytes: state.OutBytes,
		}
	}
	hasStateUpdates := false
	for _, sample := range samples {
		if sample.stateChanged {
			hasStateUpdates = true
		}
		snapshots[sample.state.InboundId] = inboundCounterSnapshot{
			inBytes:  sample.delta.currentIn,
			outBytes: sample.delta.currentOut,
		}
	}
	if !hasStateUpdates {
		setMihomoOnlines(nil, nil)
		return false, nil
	}
	if !runtimeTrafficSamplingEpochUnchanged(samplingEpoch) {
		return false, nil
	}

	unlockJournalTransaction := lockTrafficRuntimeJournalTransaction()
	defer unlockJournalTransaction()
	tx := db.Begin()
	if tx.Error != nil {
		return false, tx.Error
	}
	committed := false
	defer func() {
		if !committed {
			tx.Rollback()
		}
	}()

	now := time.Now().Unix()
	deltas := make([]inboundDelta, 0, len(samples))
	historySamples := make([]model.Stats, 0, len(samples)*2)
	inboundOnlineSet := make(map[string]struct{})
	for _, sample := range samples {
		if !sample.stateChanged {
			continue
		}
		result := tx.Model(&model.MihomoInboundRedirectState{}).
			Where("id = ? AND tag = ? AND port = ? AND port_hop_range = ? AND in_handle = ? AND out_handle = ? AND redirect_handle = ? AND in_bytes = ? AND out_bytes = ?",
				sample.state.Id,
				sample.state.Tag,
				sample.state.Port,
				sample.state.PortHopRange,
				sample.originalInHandle,
				sample.originalOutHandle,
				sample.originalRedirectHandle,
				sample.originalInBytes,
				sample.originalOutBytes,
			).
			Updates(map[string]interface{}{
				"in_handle":       sample.state.InHandle,
				"out_handle":      sample.state.OutHandle,
				"redirect_handle": sample.state.RedirectHandle,
				"in_bytes":        sample.delta.currentIn,
				"out_bytes":       sample.delta.currentOut,
				"updated_at":      time.Now(),
			})
		if result.Error != nil {
			return false, result.Error
		}
		if result.RowsAffected == 0 {
			continue
		}

		delta := sample.delta
		if delta.deltaIn <= 0 && delta.deltaOut <= 0 {
			continue
		}
		deltas = append(deltas, delta)
		inboundOnlineSet[delta.tag] = struct{}{}
		if saveTraffic && delta.deltaIn > 0 {
			historySamples = append(historySamples, model.Stats{
				DateTime:  now,
				Resource:  "mihomo_inbound",
				Tag:       delta.tag,
				Direction: true,
				Traffic:   delta.deltaIn,
			})
		}
		if saveTraffic && delta.deltaOut > 0 {
			historySamples = append(historySamples, model.Stats{
				DateTime:  now,
				Resource:  "mihomo_inbound",
				Tag:       delta.tag,
				Direction: false,
				Traffic:   delta.deltaOut,
			})
		}
	}

	userOnlines := []string{}
	if len(deltas) > 0 {
		var err error
		userOnlines, err = s.writeClientStats(tx, deltas, now, saveTraffic, &historySamples)
		if err != nil {
			return false, err
		}
	}
	journalStage, journalErr := stageTrafficRuntimeStatsForTransaction(tx, historySamples)
	if journalErr != nil {
		return false, journalErr
	}
	if !runtimeTrafficSamplingEpochUnchanged(samplingEpoch) {
		discardStagedTrafficRuntimeStats(journalStage)
		return false, nil
	}

	if err := tx.Commit().Error; err != nil {
		discardStagedTrafficRuntimeStats(journalStage)
		return false, err
	}
	committed = true
	if commitStagedTrafficRuntimeStats(journalStage) {
		if err := runtimeTrafficStats.flush(); err != nil {
			logger.Warning("flush mihomo traffic runtime journal at capacity threshold failed: ", err)
		}
	}

	setMihomoOnlines(tagsFromSet(inboundOnlineSet), userOnlines)
	return len(deltas) > 0, nil
}

// readInboundTrafficSamples batches normal counter reads into one nft command
// per direction. A failed snapshot falls back to the existing single-inbound
// recovery path so externally removed rules still self-heal correctly.
func (s *MihomoNftTrafficService) readInboundTrafficSamples(states []model.MihomoInboundRedirectState) []mihomoInboundTrafficSample {
	if len(states) == 0 {
		return nil
	}

	prepared := make([]model.MihomoInboundRedirectState, len(states))
	originalByInboundID := make(map[uint]model.MihomoInboundRedirectState, len(states))
	inputHandles := make([]int, 0, len(states))
	outputHandles := make([]int, 0, len(states))
	for i := range states {
		prepared[i] = states[i]
		originalByInboundID[states[i].InboundId] = states[i]
		if prepared[i].InHandle <= 0 || prepared[i].OutHandle <= 0 {
			s.recoverHandles(&prepared[i])
		}
		inputHandles = append(inputHandles, prepared[i].InHandle)
		outputHandles = append(outputHandles, prepared[i].OutHandle)
	}

	inputBytes, inputErr := getChainRuleBytesByHandles(nftChainIn, inputHandles)
	outputBytes, outputErr := getChainRuleBytesByHandles(nftChainOut, outputHandles)
	if inputErr != nil || outputErr != nil {
		if inputErr != nil {
			logger.Warning("failed to batch-read mihomo nft input counters: ", inputErr)
		}
		if outputErr != nil {
			logger.Warning("failed to batch-read mihomo nft output counters: ", outputErr)
		}
		return s.readInboundTrafficSamplesIndividually(states)
	}

	samples := make([]mihomoInboundTrafficSample, 0, len(prepared))
	for _, state := range prepared {
		currentIn, inOK := inputBytes[state.InHandle]
		currentOut, outOK := outputBytes[state.OutHandle]
		if !inOK || !outOK {
			return s.readInboundTrafficSamplesIndividually(states)
		}
		samples = append(samples, newMihomoInboundTrafficSample(state, originalByInboundID[state.InboundId], currentIn, currentOut))
	}
	return samples
}

func (s *MihomoNftTrafficService) readInboundTrafficSamplesIndividually(states []model.MihomoInboundRedirectState) []mihomoInboundTrafficSample {
	samples := make([]mihomoInboundTrafficSample, 0, len(states))
	for _, state := range states {
		sample, ok := s.readInboundTrafficSample(state)
		if ok {
			samples = append(samples, sample)
		}
	}
	return samples
}

func newMihomoInboundTrafficSample(state model.MihomoInboundRedirectState, original model.MihomoInboundRedirectState, currentIn int64, currentOut int64) mihomoInboundTrafficSample {
	deltaIn := currentIn - original.InBytes
	deltaOut := currentOut - original.OutBytes
	if deltaIn < 0 {
		deltaIn = currentIn
	}
	if deltaOut < 0 {
		deltaOut = currentOut
	}
	return mihomoInboundTrafficSample{
		state:                  state,
		originalInBytes:        original.InBytes,
		originalOutBytes:       original.OutBytes,
		originalInHandle:       original.InHandle,
		originalOutHandle:      original.OutHandle,
		originalRedirectHandle: original.RedirectHandle,
		stateChanged:           state.InHandle != original.InHandle || state.OutHandle != original.OutHandle || state.RedirectHandle != original.RedirectHandle || currentIn != original.InBytes || currentOut != original.OutBytes,
		delta: inboundDelta{
			inboundId:  state.InboundId,
			tag:        state.Tag,
			deltaIn:    deltaIn,
			deltaOut:   deltaOut,
			currentIn:  currentIn,
			currentOut: currentOut,
		},
	}
}

func (s *MihomoNftTrafficService) readInboundTrafficSample(state model.MihomoInboundRedirectState) (mihomoInboundTrafficSample, bool) {
	sample := mihomoInboundTrafficSample{
		state:                  state,
		originalInBytes:        state.InBytes,
		originalOutBytes:       state.OutBytes,
		originalInHandle:       state.InHandle,
		originalOutHandle:      state.OutHandle,
		originalRedirectHandle: state.RedirectHandle,
	}

	if state.InHandle <= 0 || state.OutHandle <= 0 {
		s.recoverHandles(&state)
	}
	currentIn, errIn := getChainRuleBytesByHandle(nftChainIn, state.InHandle)
	if errIn != nil {
		s.recoverHandles(&state)
		currentIn, errIn = getChainRuleBytesByHandle(nftChainIn, state.InHandle)
	}
	if errIn != nil {
		logger.Warning("failed to read mihomo nft input counter for inbound ", state.Tag, ": ", errIn)
		return sample, false
	}

	currentOut, errOut := getChainRuleBytesByHandle(nftChainOut, state.OutHandle)
	if errOut != nil {
		s.recoverHandles(&state)
		currentOut, errOut = getChainRuleBytesByHandle(nftChainOut, state.OutHandle)
	}
	if errOut != nil {
		logger.Warning("failed to read mihomo nft output counter for inbound ", state.Tag, ": ", errOut)
		return sample, false
	}

	deltaIn := currentIn - state.InBytes
	deltaOut := currentOut - state.OutBytes
	if deltaIn < 0 {
		deltaIn = currentIn
	}
	if deltaOut < 0 {
		deltaOut = currentOut
	}
	sample.state = state
	sample.stateChanged = state.InHandle != sample.originalInHandle ||
		state.OutHandle != sample.originalOutHandle ||
		state.RedirectHandle != sample.originalRedirectHandle ||
		currentIn != sample.originalInBytes ||
		currentOut != sample.originalOutBytes
	sample.delta = inboundDelta{
		inboundId:  state.InboundId,
		tag:        state.Tag,
		deltaIn:    deltaIn,
		deltaOut:   deltaOut,
		currentIn:  currentIn,
		currentOut: currentOut,
	}
	return sample, true
}

func (s *MihomoNftTrafficService) writeClientStats(tx *gorm.DB, deltas []inboundDelta, now int64, saveTraffic bool, historySamples *[]model.Stats) ([]string, error) {
	deltaMap := make(map[uint]*inboundDelta, len(deltas))
	for i := range deltas {
		deltaMap[deltas[i].inboundId] = &deltas[i]
	}

	// Restrict the binding read to inbounds that actually changed in this
	// sampling round. This keeps Mihomo traffic cost proportional to the
	// affected topology instead of every active client binding.
	inboundIDs := make([]uint, 0, len(deltaMap))
	for inboundID := range deltaMap {
		inboundIDs = append(inboundIDs, inboundID)
	}
	sort.Slice(inboundIDs, func(i, j int) bool { return inboundIDs[i] < inboundIDs[j] })

	var bindings []model.MihomoClientInboundTrafficState
	if err := tx.Where("active = ? AND inbound_id IN ?", true, inboundIDs).Find(&bindings).Error; err != nil {
		return nil, err
	}
	if len(bindings) == 0 {
		return nil, nil
	}

	clientIDSet := make(map[uint]struct{})
	for _, binding := range bindings {
		clientIDSet[binding.ClientId] = struct{}{}
	}
	clientIDs := make([]uint, 0, len(clientIDSet))
	for id := range clientIDSet {
		clientIDs = append(clientIDs, id)
	}

	var clients []model.MihomoClient
	if err := tx.Model(model.MihomoClient{}).
		Where("id IN ?", clientIDs).
		Select("id, name").
		Find(&clients).Error; err != nil {
		return nil, err
	}
	clientNames := make(map[uint]string, len(clients))
	for _, client := range clients {
		clientNames[client.Id] = client.Name
	}

	type clientAgg struct {
		upTotal   int64
		downTotal int64
	}
	aggs := make(map[uint]*clientAgg)
	changedBindings := make([]model.MihomoClientInboundTrafficState, 0, len(bindings))

	for i := range bindings {
		b := &bindings[i]
		delta, ok := deltaMap[b.InboundId]
		if !ok || (delta.deltaIn == 0 && delta.deltaOut == 0) {
			continue
		}

		agg, ok := aggs[b.ClientId]
		if !ok {
			agg = &clientAgg{}
			aggs[b.ClientId] = agg
		}
		agg.upTotal += delta.deltaIn
		agg.downTotal += delta.deltaOut

		b.UsedInBytes += delta.deltaIn
		b.UsedOutBytes += delta.deltaOut
		b.LastInBytes = delta.currentIn
		b.LastOutBytes = delta.currentOut
		b.UpdatedAt = time.Now()
		changedBindings = append(changedBindings, *b)
	}
	if err := saveMihomoClientInboundTrafficBindingsBatch(tx, changedBindings); err != nil {
		return nil, err
	}

	userOnlineSet := make(map[string]struct{}, len(aggs))
	upDeltas := make(map[uint]int64)
	downDeltas := make(map[uint]int64)
	for clientID, agg := range aggs {
		name := strings.TrimSpace(clientNames[clientID])
		if name == "" {
			continue
		}

		if agg.upTotal > 0 {
			if saveTraffic {
				*historySamples = append(*historySamples, model.Stats{
					DateTime:  now,
					Resource:  "mihomo_client",
					Tag:       name,
					Direction: true,
					Traffic:   agg.upTotal,
				})
			}
			upDeltas[clientID] = agg.upTotal
		}

		if agg.downTotal > 0 {
			if saveTraffic {
				*historySamples = append(*historySamples, model.Stats{
					DateTime:  now,
					Resource:  "mihomo_client",
					Tag:       name,
					Direction: false,
					Traffic:   agg.downTotal,
				})
			}
			downDeltas[clientID] = agg.downTotal
		}

		if agg.upTotal > 0 || agg.downTotal > 0 {
			userOnlineSet[name] = struct{}{}
		}
	}
	if err := applyMihomoClientTrafficDeltasBatch(tx, upDeltas, downDeltas); err != nil {
		return nil, err
	}

	return tagsFromSet(userOnlineSet), nil
}

func saveMihomoClientInboundTrafficBindingsBatch(tx *gorm.DB, bindings []model.MihomoClientInboundTrafficState) error {
	if tx == nil || len(bindings) == 0 {
		return nil
	}
	return tx.Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"client_id", "inbound_id", "active", "last_in_bytes", "last_out_bytes",
			"used_in_bytes", "used_out_bytes", "updated_at",
		}),
	}).Create(&bindings).Error
}

func applyMihomoClientTrafficDeltasBatch(tx *gorm.DB, up map[uint]int64, down map[uint]int64) error {
	if tx == nil {
		return nil
	}
	if err := applyMihomoClientTrafficDeltaColumn(tx, "up", up); err != nil {
		return err
	}
	return applyMihomoClientTrafficDeltaColumn(tx, "down", down)
}

func applyMihomoClientTrafficDeltaColumn(tx *gorm.DB, column string, deltas map[uint]int64) error {
	if len(deltas) == 0 {
		return nil
	}
	keys := make([]uint, 0, len(deltas))
	for id, delta := range deltas {
		if id > 0 && delta > 0 {
			keys = append(keys, id)
		}
	}
	if len(keys) == 0 {
		return nil
	}
	sort.Slice(keys, func(i, j int) bool { return keys[i] < keys[j] })
	args := make([]interface{}, 0, len(keys)*3)
	caseSQL := "CASE id"
	placeholders := make([]string, 0, len(keys))
	for _, id := range keys {
		caseSQL += " WHEN ? THEN ?"
		args = append(args, id, deltas[id])
		placeholders = append(placeholders, "?")
	}
	caseSQL += " ELSE 0 END"
	for _, id := range keys {
		args = append(args, id)
	}
	query := fmt.Sprintf("UPDATE mihomo_clients SET %s = %s + %s WHERE id IN (%s)", column, column, caseSQL, strings.Join(placeholders, ","))
	return tx.Exec(query, args...).Error
}

func (s *MihomoNftTrafficService) ensureClientBindings(tx *gorm.DB, snapshots map[uint]inboundCounterSnapshot) error {
	if tx == nil {
		return nil
	}
	if !mihomoClientBindingRepairNeeded.Load() {
		return nil
	}

	var clients []model.MihomoClient
	if err := tx.Model(model.MihomoClient{}).
		Select("id, inbounds").
		Find(&clients).Error; err != nil {
		return err
	}

	for i := range clients {
		client := &clients[i]
		if client.Id == 0 {
			continue
		}
		if err := s.syncClientBindings(tx, client.Id, parseMihomoInboundIDs(client.Inbounds), snapshots); err != nil {
			return err
		}
	}
	mihomoClientBindingRepairNeeded.Store(false)
	return nil
}

func parseMihomoInboundIDs(raw json.RawMessage) []uint {
	ids, err := util.ParseInboundIDs(raw)
	if err != nil || len(ids) == 0 {
		return []uint{}
	}
	return ids
}

func deduplicateInboundIDs(ids []uint) []uint {
	if len(ids) == 0 {
		return []uint{}
	}

	seen := make(map[uint]struct{}, len(ids))
	result := make([]uint, 0, len(ids))
	for _, id := range ids {
		if id == 0 {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		result = append(result, id)
	}
	return result
}

// InitOnStartup restores mihomo nft rules for existing inbounds.
func (s *MihomoNftTrafficService) InitOnStartup() {
	_ = runMihomoInboundNftMutation(func() error {
		s.initOnStartup()
		return nil
	})
}

func (s *MihomoNftTrafficService) initOnStartup() {
	if !IsSystemPlatformLinux() || !nftSupported() {
		return
	}

	db := database.GetDB()
	var inbounds []model.MihomoInbound
	if err := db.Find(&inbounds).Error; err != nil {
		logger.Warning("failed to load mihomo inbounds for nft init: ", err)
		return
	}
	unsupportedInboundIDs := make([]uint, 0)
	for _, inbound := range inbounds {
		if !isSupportedMihomoInboundType(inbound.Type) {
			unsupportedInboundIDs = append(unsupportedInboundIDs, inbound.Id)
		}
	}
	if err := s.removeUnsupportedInboundStates(db, unsupportedInboundIDs); err != nil {
		logger.Warning("failed to remove unsupported mihomo inbound nft states on startup: ", err)
	}

	for _, inbound := range inbounds {
		if !isSupportedMihomoInboundType(inbound.Type) {
			if err := s.RemoveInboundRules(db, inbound.Id); err != nil {
				logger.Warning("failed to remove unsupported mihomo inbound nft state for ", inbound.Tag, ": ", err)
			}
			continue
		}
		port := extractPort(inbound.Options)
		if port <= 0 {
			continue
		}

		redirectRange, redirectTCP := resolveMihomoInboundRedirectSpec(&inbound)

		var state model.MihomoInboundRedirectState
		result := db.Where("inbound_id = ?", inbound.Id).First(&state)
		if result.Error == nil {
			if state.InHandle > 0 || state.OutHandle > 0 || state.RedirectHandle > 0 || strings.TrimSpace(state.PortHopRange) != "" {
				if rmErr := s.removeRulesFromState(&state); rmErr != nil {
					logger.Warning("failed to cleanup old mihomo nft rules on startup for inbound ", inbound.Tag, ": ", rmErr)
				}
			}

			inHandle, inErr := addPortCounterRule(nftChainIn, port, "dport", mihomoNftRuleComments.in(inbound.Tag))
			if inErr != nil {
				logger.Warning("failed to restore mihomo nft input rule for inbound ", inbound.Tag, ": ", inErr)
			}
			outHandle, outErr := addPortCounterRule(nftChainOut, port, "sport", mihomoNftRuleComments.out(inbound.Tag))
			if outErr != nil {
				logger.Warning("failed to restore mihomo nft output rule for inbound ", inbound.Tag, ": ", outErr)
			}

			redirectHandle := 0
			hopNft, skipped, sample := portHopRangeToNftWithExclusions(redirectRange, port)
			if skipped > 0 {
				if len(sample) > 0 {
					logger.Info("mihomo port hop range for inbound ", inbound.Tag, ": skipped ", skipped, " UDP ports (sample ", sample, ")")
				} else {
					logger.Info("mihomo port hop range for inbound ", inbound.Tag, ": skipped ", skipped, " UDP ports")
				}
			}
			if hopNft != "" {
				handle, redirectErr := addRedirectRuleWithProtocols(hopNft, port, mihomoNftRuleComments.redirect(inbound.Tag), redirectTCP)
				if redirectErr != nil {
					logger.Warning("failed to restore mihomo REDIRECT rule for inbound ", inbound.Tag, ": ", redirectErr)
				}
				redirectHandle = handle
			}

			if updateErr := db.Model(&state).Updates(map[string]interface{}{
				"tag":             inbound.Tag,
				"port":            port,
				"port_hop_range":  redirectRange,
				"in_handle":       inHandle,
				"out_handle":      outHandle,
				"redirect_handle": redirectHandle,
				"in_bytes":        0,
				"out_bytes":       0,
				"updated_at":      time.Now(),
			}).Error; updateErr != nil {
				logger.Warning("failed to update mihomo inbound nft state on startup for inbound ", inbound.Tag, ": ", updateErr)
			}

			if bindingErr := db.Model(&model.MihomoClientInboundTrafficState{}).
				Where("inbound_id = ? AND active = ?", inbound.Id, true).
				Updates(map[string]interface{}{
					"last_in_bytes":  0,
					"last_out_bytes": 0,
					"updated_at":     time.Now(),
				}).Error; bindingErr != nil {
				logger.Warning("failed to reset mihomo client inbound baselines on startup for inbound ", inbound.Tag, ": ", bindingErr)
			}
			continue
		}

		if !errors.Is(result.Error, gorm.ErrRecordNotFound) {
			logger.Warning("failed to query mihomo inbound nft state on startup for inbound ", inbound.Tag, ": ", result.Error)
			continue
		}

		if setupErr := s.SetupInboundRules(db, inbound.Id, inbound.Tag, port, redirectRange, redirectTCP); setupErr != nil {
			logger.Warning("failed to setup startup mihomo nft rules for inbound ", inbound.Tag, ": ", setupErr)
			continue
		}
	}
}

func (s *MihomoNftTrafficService) removeUnsupportedInboundStates(db *gorm.DB, inboundIDs []uint) error {
	if db == nil || len(inboundIDs) == 0 {
		return nil
	}
	var firstErr error
	for _, inboundID := range inboundIDs {
		if err := s.RemoveInboundRules(db, inboundID); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// CleanupOnShutdown removes mihomo nft rules and clears volatile handles/baselines.
func (s *MihomoNftTrafficService) CleanupOnShutdown() {
	_ = runMihomoInboundNftMutation(func() error {
		s.cleanupOnShutdown()
		return nil
	})
}

func (s *MihomoNftTrafficService) cleanupOnShutdown() {
	mihomoPortHopRefreshState.mu.Lock()
	mihomoPortHopRefreshState.last = map[uint]time.Time{}
	mihomoPortHopRefreshState.mu.Unlock()

	if IsSystemPlatformLinux() && nftSupported() {
		if err := deleteRulesByCommentPrefix(mihomoNftRuleComments.prefix); err != nil {
			logger.Warning("failed to cleanup mihomo nft rules by prefix: ", err)
		}
	}

	db := database.GetDB()
	now := time.Now()
	updates := map[string]interface{}{
		"in_handle":       0,
		"out_handle":      0,
		"redirect_handle": 0,
		"in_bytes":        0,
		"out_bytes":       0,
		"updated_at":      now,
	}
	if err := db.Session(&gorm.Session{AllowGlobalUpdate: true}).
		Model(&model.MihomoInboundRedirectState{}).
		Updates(updates).Error; err != nil {
		logger.Warning("failed to reset mihomo inbound nft state after cleanup: ", err)
	}

	if err := db.Model(&model.MihomoClientInboundTrafficState{}).
		Where("active = ?", true).
		Updates(map[string]interface{}{
			"last_in_bytes":  0,
			"last_out_bytes": 0,
			"updated_at":     now,
		}).Error; err != nil {
		logger.Warning("failed to reset mihomo active client baselines after nft cleanup: ", err)
	}

	setMihomoOnlines(nil, nil)
}

// recoverHandles tries to recover missing nft rule handles by comments.
// It only updates the supplied snapshot, so callers can run nft reads outside a DB transaction.
func (s *MihomoNftTrafficService) recoverHandles(st *model.MihomoInboundRedirectState) bool {
	if st == nil {
		return false
	}

	changed := false

	if handle := findHandleByComment(nftChainIn, mihomoNftRuleComments.in(st.Tag)); handle > 0 && handle != st.InHandle {
		st.InHandle = handle
		changed = true
		logger.Info("recovered mihomo nft input handle for ", st.Tag, ": ", handle)
	}

	if handle := findHandleByComment(nftChainOut, mihomoNftRuleComments.out(st.Tag)); handle > 0 && handle != st.OutHandle {
		st.OutHandle = handle
		changed = true
		logger.Info("recovered mihomo nft output handle for ", st.Tag, ": ", handle)
	}

	if strings.TrimSpace(st.PortHopRange) != "" {
		if handle := findNftRedirectHandleByComment(mihomoNftRuleComments.redirect(st.Tag)); handle != st.RedirectHandle {
			st.RedirectHandle = handle
			changed = true
			if handle > 0 {
				logger.Info("recovered mihomo nft redirect handle for ", st.Tag, ": ", handle)
			}
		}
	}

	return changed
}

// tryRecoverHandles persists recovered handles for call paths that already own a transaction.
func (s *MihomoNftTrafficService) tryRecoverHandles(tx *gorm.DB, st *model.MihomoInboundRedirectState) {
	if tx == nil || !s.recoverHandles(st) {
		return
	}
	if err := tx.Model(st).Updates(map[string]interface{}{
		"in_handle":       st.InHandle,
		"out_handle":      st.OutHandle,
		"redirect_handle": st.RedirectHandle,
		"updated_at":      time.Now(),
	}).Error; err != nil {
		logger.Warning("failed to persist recovered mihomo nft handles for inbound ", st.Tag, ": ", err)
	}
}

func shouldRefreshMihomoPortHop(inboundID uint, now time.Time, interval time.Duration) bool {
	mihomoPortHopRefreshState.mu.Lock()
	defer mihomoPortHopRefreshState.mu.Unlock()

	last, ok := mihomoPortHopRefreshState.last[inboundID]
	if !ok {
		return true
	}
	return now.Sub(last) >= interval
}

func markMihomoPortHopRefreshed(inboundID uint, now time.Time) {
	mihomoPortHopRefreshState.mu.Lock()
	defer mihomoPortHopRefreshState.mu.Unlock()
	mihomoPortHopRefreshState.last[inboundID] = now
}

func clearMihomoPortHopRefresh(inboundID uint) {
	mihomoPortHopRefreshState.mu.Lock()
	defer mihomoPortHopRefreshState.mu.Unlock()
	delete(mihomoPortHopRefreshState.last, inboundID)
}

func tagsFromSet(set map[string]struct{}) []string {
	if len(set) == 0 {
		return []string{}
	}

	tags := make([]string, 0, len(set))
	for tag := range set {
		if strings.TrimSpace(tag) == "" {
			continue
		}
		tags = append(tags, tag)
	}
	sort.Strings(tags)
	return tags
}
