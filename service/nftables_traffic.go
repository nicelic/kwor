package service

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/logger"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// NftTrafficService manages nftables-based port traffic monitoring.
//
// Lifecycle:
//   - Inbound created  -> SetupInboundRules (adds nftables counter rules + InboundTrafficState row)
//   - Inbound deleted  -> RemoveInboundRules (removes nftables rules + deletes InboundTrafficState + ClientInboundTrafficState)
//   - Client saved     -> SyncClientBindings (adds/removes ClientInboundTrafficState with proper baseline)
//   - Every 10s        -> CollectAndSaveTraffic (reads counters, calculates deltas, writes Stats records)
//
// Port hopping (Hysteria2):
//   - When port_hop_range is set, a REDIRECT rule is created in nat/prerouting
//     to forward hop port UDP traffic to listen_port.
//   - Counter rules still monitor listen_port only (REDIRECT rewrites dport before filter/input).
type NftTrafficService struct{}

// A full client-binding repair is needed only for legacy state or after a
// failed targeted sync. Normal client/inbound saves already queue an exact
// post-commit binding update, so repeating a full client JSON scan on every
// traffic delta only burns CPU and SQLite time.
var defaultClientBindingRepairNeeded atomic.Bool

func init() {
	defaultClientBindingRepairNeeded.Store(true)
	database.RegisterDBResetHook(func() { defaultClientBindingRepairNeeded.Store(true) })
}

type inboundCounterSnapshot struct {
	inBytes  int64
	outBytes int64
}

type inboundTrafficSample struct {
	state                  model.InboundTrafficState
	originalInBytes        int64
	originalOutBytes       int64
	originalInHandle       int
	originalOutHandle      int
	originalRedirectHandle int
	stateChanged           bool
	delta                  inboundDelta
}

var portHopRefreshState = struct {
	mu   sync.Mutex
	last map[uint]time.Time
}{
	last: map[uint]time.Time{},
}

// defaultInboundNftMutationMu serializes default-chain inbound rule changes.
// It intentionally does not cover Mihomo or other nft users: both core chains
// retain independent lifecycle and state models.
var defaultInboundNftMutationMu sync.Mutex

func runDefaultInboundNftMutation(fn func() error) error {
	defaultInboundNftMutationMu.Lock()
	defer defaultInboundNftMutationMu.Unlock()
	return fn()
}

func (s *NftTrafficService) IsNftTableReady() bool {
	return nftTableExists()
}

// ApplyInboundNftAction keeps the core-state decision and the matching rule
// mutation under one default-chain lock. A Core transition cannot otherwise
// slip between a save seeing "stopped" and its cleanup of old rules.
func (s *NftTrafficService) ApplyInboundNftAction(db *gorm.DB, action *InboundNftAction) error {
	if action == nil {
		return nil
	}
	if db == nil {
		return errors.New("default inbound nft action requires a database")
	}
	return runDefaultInboundNftMutation(func() error {
		coreRunning := (&CoreManagerService{}).IsRunning()
		var err error
		switch action.Kind {
		case "upsert":
			if coreRunning {
				err = s.updateInboundRules(db, action.InboundID, action.Tag, action.Port, action.PortHopRange)
			} else {
				err = s.UpsertInboundStateOnly(db, action.InboundID, action.Tag, action.Port, action.PortHopRange)
			}
		case "remove":
			if coreRunning {
				err = s.removeInboundRules(db, action.InboundID)
			} else {
				err = s.RemoveInboundStateOnly(db, action.InboundID)
			}
		default:
			return errors.New("unknown inbound nft action: " + action.Kind)
		}
		if err != nil {
			return err
		}
		if !coreRunning {
			s.cleanupOnShutdown()
		}
		return nil
	})
}

// EnsureRuleIntegrity verifies inbound nftables rules are still present and
// recreates missing ones when rules are externally removed.
func (s *NftTrafficService) EnsureRuleIntegrity() error {
	if !IsSystemPlatformLinux() || !nftSupported() {
		return nil
	}

	coreSvc := &CoreManagerService{}
	if !coreSvc.IsRunning() {
		return nil
	}
	return s.EnsureRuleIntegrityWhenRunning()
}

// EnsureRuleIntegrityWhenRunning is for callers that already checked the core
// state in the current synchronization pass.
func (s *NftTrafficService) EnsureRuleIntegrityWhenRunning() error {
	return runDefaultInboundNftMutation(func() error {
		return s.ensureRuleIntegrityWhenRunning()
	})
}

func (s *NftTrafficService) ensureRuleIntegrityWhenRunning() error {
	if !IsSystemPlatformLinux() || !nftSupported() {
		return nil
	}

	db := database.GetDB()
	var inbounds []model.Inbound
	if err := db.Find(&inbounds).Error; err != nil {
		return err
	}
	if len(inbounds) == 0 {
		return nil
	}
	managedInboundIDs := make([]uint, 0, len(inbounds))
	for _, inbound := range inbounds {
		if inbound.Id > 0 && extractPort(inbound.Options) > 0 {
			managedInboundIDs = append(managedInboundIDs, inbound.Id)
		}
	}
	var states []model.InboundTrafficState
	if len(managedInboundIDs) > 0 {
		if err := db.Where("inbound_id IN ?", managedInboundIDs).Find(&states).Error; err != nil {
			return err
		}
	}
	statesByInboundID := make(map[uint]model.InboundTrafficState, len(states))
	for _, state := range states {
		statesByInboundID[state.InboundId] = state
	}
	inHandles, err := snapshotManagedRuleHandles(nftChainIn, singboxNftRuleComments.prefix)
	if err != nil {
		return err
	}
	outHandles, err := snapshotManagedRuleHandles(nftChainOut, singboxNftRuleComments.prefix)
	if err != nil {
		return err
	}
	redirectRules, err := snapshotManagedRedirectRules(singboxNftRuleComments.prefix)
	if err != nil {
		return err
	}

	validComments := make(map[string]struct{}, len(inbounds)*3)
	for _, inbound := range inbounds {
		port := extractPort(inbound.Options)
		if port <= 0 {
			continue
		}
		validComments[singboxNftRuleComments.in(inbound.Tag)] = struct{}{}
		validComments[singboxNftRuleComments.out(inbound.Tag)] = struct{}{}
		if strings.TrimSpace(extractPortHopRange(inbound.Options)) != "" {
			validComments[singboxNftRuleComments.redirect(inbound.Tag)] = struct{}{}
		}
	}

	var firstErr error
	for _, inbound := range inbounds {
		port := extractPort(inbound.Options)
		if port <= 0 {
			continue
		}
		portHopRange := extractPortHopRange(inbound.Options)
		state, stateFound := statesByInboundID[inbound.Id]
		if err := s.ensureInboundRuleIntegrity(db, &inbound, port, portHopRange, state, stateFound, inHandles, outHandles, redirectRules); err != nil {
			logger.Warning("nft rule integrity check failed for inbound ", inbound.Tag, ": ", err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}

	if err := s.cleanupOrphanInboundRules(validComments, inHandles, outHandles, redirectRules.comments); err != nil {
		logger.Warning("cleanup orphan inbound nft rules failed: ", err)
		if firstErr == nil {
			firstErr = err
		}
	}

	return firstErr
}

type chainRuleComment struct {
	handle  int
	comment string
}

func (s *NftTrafficService) cleanupOrphanInboundRules(validComments map[string]struct{}, inHandles map[string]int, outHandles map[string]int, redirectComments map[string]struct{}) error {
	var firstErr error
	cleanupChain := func(chain string, handles map[string]int) {
		for comment, handle := range handles {
			if _, ok := validComments[comment]; ok {
				continue
			}
			if err := deleteRuleByHandle(chain, handle); err != nil && !nftObjectMissing(err) {
				logger.Warning("failed to delete orphan inbound nft rule ", comment, " handle ", handle, ": ", err)
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

func (s *NftTrafficService) ensureInboundRuleIntegrity(tx *gorm.DB, inbound *model.Inbound, port int, portHopRange string, state model.InboundTrafficState, stateFound bool, inHandles map[string]int, outHandles map[string]int, redirectRules *managedRedirectRuleSnapshot) error {
	if inbound == nil || inbound.Id == 0 || port <= 0 {
		return nil
	}

	if !stateFound {
		return s.SetupInboundRules(tx, inbound.Id, inbound.Tag, port, portHopRange)
	}

	// Keep DB state aligned to the current inbound definition.
	if state.Tag != inbound.Tag || state.Port != port || state.PortHopRange != portHopRange {
		return s.updateInboundRules(tx, inbound.Id, inbound.Tag, port, portHopRange)
	}

	originalInHandle := state.InHandle
	originalOutHandle := state.OutHandle
	missing := false

	if handle, ok := inHandles[singboxNftRuleComments.in(inbound.Tag)]; ok {
		state.InHandle = handle
	} else {
		missing = true
	}

	if handle, ok := outHandles[singboxNftRuleComments.out(inbound.Tag)]; ok {
		state.OutHandle = handle
	} else {
		missing = true
	}

	if portHopRange != "" {
		comment := singboxNftRuleComments.redirect(inbound.Tag)
		if redirectRules == nil || !redirectRules.valid[comment] {
			missing = true
		}
	} else if state.RedirectHandle > 0 || redirectRules.hasComment(singboxNftRuleComments.redirect(inbound.Tag)) {
		if err := deleteNftRedirectRulesByComment(singboxNftRuleComments.redirect(inbound.Tag)); err != nil {
			logger.Warning("failed to delete stale redirect rule for inbound ", inbound.Tag, ": ", err)
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
		logger.Warning("failed to remove stale nft rules for inbound ", inbound.Tag, ": ", err)
	}
	if err := tx.Delete(&state).Error; err != nil {
		return err
	}
	return s.SetupInboundRules(tx, inbound.Id, inbound.Tag, port, portHopRange)
}

// ---------------------------------------------------------------------------
// Inbound rule lifecycle
// ---------------------------------------------------------------------------

// SetupInboundRules creates nftables counter rules for the given inbound port
// and persists an InboundTrafficState row.
// If portHopRange is non-empty, also creates a REDIRECT rule for port hopping.
// Call after the inbound is saved to the DB (so we have its ID).
func (s *NftTrafficService) SetupInboundRules(tx *gorm.DB, inboundId uint, tag string, port int, portHopRange string) error {
	if port <= 0 {
		return s.removeInboundRules(tx, inboundId)
	}

	// Check if rules already exist for this inbound
	var existing model.InboundTrafficState
	result := tx.Where("inbound_id = ?", inboundId).First(&existing)
	if result.Error == nil {
		// Already exists - check if port or port_hop_range changed
		if existing.Port == port && existing.PortHopRange == portHopRange {
			return nil // no change needed
		}
		// Changed: remove old rules first
		if err := s.removeRulesFromState(&existing); err != nil {
			logger.Warning("failed to remove old nftables rules for inbound ", tag, ": ", err)
		}
		// Delete old state
		tx.Delete(&existing)
	}

	// Create nftables counter rules (monitor listen_port only)
	inHandle, err := addPortCounterRule(nftChainIn, port, "dport", singboxNftRuleComments.in(tag))
	if err != nil {
		logger.Warning("failed to add nftables input rule for port ", port, ": ", err)
	}

	outHandle, err := addPortCounterRule(nftChainOut, port, "sport", singboxNftRuleComments.out(tag))
	if err != nil {
		logger.Warning("failed to add nftables output rule for port ", port, ": ", err)
	}

	// Create REDIRECT rule for port hopping if needed
	var redirectHandle int
	if portHopRange != "" {
		hopNft, skipped, sample := portHopRangeToNftWithExclusions(portHopRange, port)
		if skipped > 0 {
			if len(sample) > 0 {
				logger.Info("port hop range for inbound ", tag, ": skipped ", skipped, " UDP ports (sample ", sample, ")")
			} else {
				logger.Info("port hop range for inbound ", tag, ": skipped ", skipped, " UDP ports")
			}
		}
		if hopNft != "" {
			redirectHandle, err = addRedirectRule(hopNft, port, singboxNftRuleComments.redirect(tag))
			if err != nil {
				logger.Warning("failed to add nftables REDIRECT rule for port hopping (", hopNft, " -> ", port, "): ", err)
			} else if redirectHandle > 0 {
				logger.Info("nftables REDIRECT rule created: UDP ", hopNft, " -> :", port, " (handle ", redirectHandle, ")")
			}
		} else {
			logger.Warning("port hop range for inbound ", tag, " has no available UDP ports after exclusion")
		}
	}

	state := model.InboundTrafficState{
		InboundId:      inboundId,
		Tag:            tag,
		Port:           port,
		InHandle:       inHandle,
		OutHandle:      outHandle,
		PortHopRange:   portHopRange,
		RedirectHandle: redirectHandle,
		InBytes:        0,
		OutBytes:       0,
		UpdatedAt:      time.Now(),
		CreatedAt:      time.Now(),
	}

	return tx.Create(&state).Error
}

// RemoveInboundRules deletes nftables rules and the InboundTrafficState for the given inbound.
// Also removes all ClientInboundTrafficState records for this inbound.
func (s *NftTrafficService) RemoveInboundRules(tx *gorm.DB, inboundId uint) error {
	return runDefaultInboundNftMutation(func() error {
		return s.removeInboundRules(tx, inboundId)
	})
}

func (s *NftTrafficService) removeInboundRules(tx *gorm.DB, inboundId uint) error {
	var state model.InboundTrafficState
	result := tx.Where("inbound_id = ?", inboundId).First(&state)
	if result.Error != nil {
		return nil // no state found, nothing to do
	}

	// Remove nftables rules
	if err := s.removeRulesFromState(&state); err != nil {
		logger.Warning("failed to remove nftables rules for inbound ", state.Tag, ": ", err)
	}

	// Delete all client bindings for this inbound
	tx.Where("inbound_id = ?", inboundId).Delete(&model.ClientInboundTrafficState{})

	// Delete the inbound traffic state
	if err := tx.Delete(&state).Error; err != nil {
		return err
	}
	clearPortHopRefresh(inboundId)
	return nil
}

// UpsertInboundStateOnly updates/creates InboundTrafficState without touching nftables rules.
// Use this while core is stopped to keep DB state in sync with inbound changes.
func (s *NftTrafficService) UpsertInboundStateOnly(tx *gorm.DB, inboundId uint, tag string, port int, portHopRange string) error {
	if inboundId == 0 {
		return nil
	}
	if port <= 0 {
		return s.RemoveInboundStateOnly(tx, inboundId)
	}

	var state model.InboundTrafficState
	result := tx.Where("inbound_id = ?", inboundId).First(&state)
	now := time.Now()

	if result.Error == nil {
		clearPortHopRefresh(inboundId)
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

	clearPortHopRefresh(inboundId)
	state = model.InboundTrafficState{
		InboundId:      inboundId,
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
	}
	return tx.Create(&state).Error
}

// RemoveInboundStateOnly deletes traffic state rows without touching nftables rules.
// Use this while core is stopped to avoid noisy nft command errors.
func (s *NftTrafficService) RemoveInboundStateOnly(tx *gorm.DB, inboundId uint) error {
	if inboundId == 0 {
		return nil
	}
	if err := tx.Where("inbound_id = ?", inboundId).Delete(&model.ClientInboundTrafficState{}).Error; err != nil {
		return err
	}
	if err := tx.Where("inbound_id = ?", inboundId).Delete(&model.InboundTrafficState{}).Error; err != nil {
		return err
	}
	clearPortHopRefresh(inboundId)
	return nil
}

// UpdateInboundRules handles inbound edits (port, tag, port_hop_range).
// Port/port_hop_range changes recreate rules; tag-only changes update mappings when possible.
func (s *NftTrafficService) UpdateInboundRules(tx *gorm.DB, inboundId uint, tag string, newPort int, portHopRange string) error {
	return runDefaultInboundNftMutation(func() error {
		return s.updateInboundRules(tx, inboundId, tag, newPort, portHopRange)
	})
}

func (s *NftTrafficService) updateInboundRules(tx *gorm.DB, inboundId uint, tag string, newPort int, portHopRange string) error {
	if newPort <= 0 {
		return s.removeInboundRules(tx, inboundId)
	}

	var existing model.InboundTrafficState
	result := tx.Where("inbound_id = ?", inboundId).First(&existing)
	if result.Error != nil {
		// No existing rules, just create new ones
		return s.SetupInboundRules(tx, inboundId, tag, newPort, portHopRange)
	}

	if existing.Port == newPort && existing.PortHopRange == portHopRange {
		if existing.Tag == tag {
			return nil // no change
		}
		// Tag-only change: keep rules, update mapping if possible
		return s.updateInboundTag(tx, &existing, tag)
	}

	// Port or port_hop_range changed: remove old, create new
	if err := s.removeRulesFromState(&existing); err != nil {
		logger.Warning("failed to remove old nftables rules: ", err)
	}
	tx.Delete(&existing)

	return s.SetupInboundRules(tx, inboundId, tag, newPort, portHopRange)
}

// updateInboundTag updates tag mapping; keeps counter rules if handles are known.
// Port-hopping REDIRECT is recreated on tag change; missing counter handles trigger full recreate.
func (s *NftTrafficService) updateInboundTag(tx *gorm.DB, state *model.InboundTrafficState, newTag string) error {
	oldTag := state.Tag
	if oldTag == newTag {
		return nil
	}

	// Try to recover missing handles using the old tag comments.
	if state.InHandle <= 0 {
		if handle := findHandleByComment(nftChainIn, singboxNftRuleComments.in(oldTag)); handle > 0 {
			state.InHandle = handle
		}
	}
	if state.OutHandle <= 0 {
		if handle := findHandleByComment(nftChainOut, singboxNftRuleComments.out(oldTag)); handle > 0 {
			state.OutHandle = handle
		}
	}
	if state.InHandle <= 0 || state.OutHandle <= 0 {
		logger.Warning("tag change for inbound ", oldTag, " -> ", newTag, " requires rule recreation (missing nft handles)")
		if err := s.removeRulesFromState(state); err != nil {
			logger.Warning("failed to remove old nftables rules during tag change: ", err)
		}
		tx.Delete(state)
		return s.SetupInboundRules(tx, state.InboundId, newTag, state.Port, state.PortHopRange)
	}

	// Recreate REDIRECT rule (port hopping) on tag change.
	if state.PortHopRange != "" {
		comment := singboxNftRuleComments.redirect(oldTag)
		if err := deleteNftRedirectRulesByComment(comment); err != nil {
			logger.Warning("failed to delete old nftables REDIRECT rule for inbound ", oldTag, ": ", err)
		}

		hopNft, skipped, sample := portHopRangeToNftWithExclusions(state.PortHopRange, state.Port)
		if skipped > 0 {
			if len(sample) > 0 {
				logger.Info("port hop range for inbound ", newTag, ": skipped ", skipped, " UDP ports (sample ", sample, ")")
			} else {
				logger.Info("port hop range for inbound ", newTag, ": skipped ", skipped, " UDP ports")
			}
		}
		if hopNft != "" {
			comment := singboxNftRuleComments.redirect(newTag)
			redirectHandle, err := addRedirectRule(hopNft, state.Port, comment)
			if err != nil {
				logger.Warning("failed to add nftables REDIRECT rule for port hopping (", hopNft, " -> ", state.Port, "): ", err)
			} else if redirectHandle > 0 {
				logger.Info("nftables REDIRECT rule created: UDP ", hopNft, " -> :", state.Port, " (handle ", redirectHandle, ")")
			}
			state.RedirectHandle = redirectHandle
		} else {
			state.RedirectHandle = 0
			logger.Warning("port hop range for inbound ", newTag, " has no available UDP ports after exclusion")
		}
	}

	// Keep rules and counters, just update the state tag mapping.
	state.Tag = newTag
	return tx.Model(state).Updates(map[string]interface{}{
		"tag":             newTag,
		"in_handle":       state.InHandle,
		"out_handle":      state.OutHandle,
		"redirect_handle": state.RedirectHandle,
		"updated_at":      time.Now(),
	}).Error
}

func (s *NftTrafficService) removeRulesFromState(state *model.InboundTrafficState) error {
	var firstErr error

	// Delete input rule: try by handle first, fallback to comment
	if state.InHandle > 0 {
		if err := deleteRuleByHandle(nftChainIn, state.InHandle); err != nil {
			firstErr = err
		}
	} else {
		// Handle unknown, try to delete by comment
		comment := singboxNftRuleComments.in(state.Tag)
		if err := deleteRuleByComment(nftChainIn, comment); err != nil && firstErr == nil {
			firstErr = err
		}
	}

	// Delete output rule: try by handle first, fallback to comment
	if state.OutHandle > 0 {
		if err := deleteRuleByHandle(nftChainOut, state.OutHandle); err != nil && firstErr == nil {
			firstErr = err
		}
	} else {
		comment := singboxNftRuleComments.out(state.Tag)
		if err := deleteRuleByComment(nftChainOut, comment); err != nil && firstErr == nil {
			firstErr = err
		}
	}

	// A compatibility REDIRECT is dual-stack and cannot be represented by one
	// handle, so always remove it by its stable comment across NAT tables.
	if state.PortHopRange != "" || state.RedirectHandle > 0 {
		comment := singboxNftRuleComments.redirect(state.Tag)
		if err := deleteNftRedirectRulesByComment(comment); err != nil && firstErr == nil {
			firstErr = err
		}
	}

	return firstErr
}

// ---------------------------------------------------------------------------
// Client-Inbound binding management
// ---------------------------------------------------------------------------

// SyncClientBindings synchronizes the ClientInboundTrafficState records for a client.
// It creates baselines for newly bound inbounds and deactivates removed ones.
//
// IMPORTANT: For newly bound inbounds, the baseline (LastInBytes/LastOutBytes) is set
// to the CURRENT cumulative nftables counter value, so the client starts from 0.
func (s *NftTrafficService) SyncClientBindings(tx *gorm.DB, clientId uint, newInboundIds []uint) error {
	return s.syncClientBindings(tx, clientId, newInboundIds, nil)
}

// QueueSyncClientBindings delays counter reads until after the caller commits.
// nft can be slow, so it must not run while SQLite's single connection is held.
func (s *NftTrafficService) QueueSyncClientBindings(tx *gorm.DB, clientId uint, inboundIds []uint) error {
	ids := append([]uint(nil), inboundIds...)
	return QueueManagedRuntimeHook(tx, func() error {
		if err := s.SyncClientBindings(database.GetDB(), clientId, ids); err != nil {
			defaultClientBindingRepairNeeded.Store(true)
			logger.Warning("failed to sync client traffic bindings for client id ", clientId, ": ", err)
		}
		return nil
	})
}

func (s *NftTrafficService) syncClientBindings(tx *gorm.DB, clientId uint, newInboundIds []uint, snapshots map[uint]inboundCounterSnapshot) error {
	// Get existing bindings
	var existingBindings []model.ClientInboundTrafficState
	if err := tx.Where("client_id = ?", clientId).Find(&existingBindings).Error; err != nil {
		return err
	}

	existingMap := make(map[uint]*model.ClientInboundTrafficState)
	for i := range existingBindings {
		existingMap[existingBindings[i].InboundId] = &existingBindings[i]
	}

	newSet := make(map[uint]bool)
	for _, id := range newInboundIds {
		newSet[id] = true
	}

	// Deactivate bindings that are no longer in the new set
	for inboundId, binding := range existingMap {
		if !newSet[inboundId] {
			if binding.Active {
				binding.Active = false
				binding.UpdatedAt = time.Now()
				if err := tx.Save(binding).Error; err != nil {
					return err
				}
			}
		}
	}

	// Activate or create bindings for new inbound IDs
	for _, inboundId := range newInboundIds {
		if existing, ok := existingMap[inboundId]; ok {
			if !existing.Active {
				// Re-activating: reset baseline to current nftables counter
				currentIn, currentOut := s.currentInboundBytesForBinding(tx, inboundId, snapshots)
				existing.Active = true
				existing.LastInBytes = currentIn
				existing.LastOutBytes = currentOut
				existing.UsedInBytes = 0
				existing.UsedOutBytes = 0
				existing.UpdatedAt = time.Now()
				if err := tx.Save(existing).Error; err != nil {
					return err
				}
			}
			// Already active: no change needed
		} else {
			// New binding: create with baseline = current nftables counter
			currentIn, currentOut := s.currentInboundBytesForBinding(tx, inboundId, snapshots)
			binding := model.ClientInboundTrafficState{
				ClientId:     clientId,
				InboundId:    inboundId,
				Active:       true,
				LastInBytes:  currentIn,
				LastOutBytes: currentOut,
				UsedInBytes:  0,
				UsedOutBytes: 0,
				CreatedAt:    time.Now(),
				UpdatedAt:    time.Now(),
			}
			if err := tx.Create(&binding).Error; err != nil {
				return err
			}
		}
	}

	return nil
}

func (s *NftTrafficService) currentInboundBytesForBinding(tx *gorm.DB, inboundId uint, snapshots map[uint]inboundCounterSnapshot) (int64, int64) {
	if snapshot, ok := snapshots[inboundId]; ok {
		return snapshot.inBytes, snapshot.outBytes
	}
	// CollectAndSaveTraffic passes a non-nil snapshot map while holding an
	// explicit transaction. A concurrently-created inbound can be absent from
	// that map; use its persisted counter instead of issuing nft commands while
	// the sole SQLite connection is checked out.
	if snapshots != nil {
		return s.getPersistedInboundBytes(tx, inboundId)
	}
	return s.getCurrentInboundBytes(tx, inboundId)
}

func (s *NftTrafficService) getPersistedInboundBytes(db *gorm.DB, inboundId uint) (int64, int64) {
	if db == nil || inboundId == 0 {
		return 0, 0
	}

	var state model.InboundTrafficState
	if err := db.Select("in_bytes", "out_bytes").Where("inbound_id = ?", inboundId).First(&state).Error; err != nil {
		return 0, 0
	}
	return state.InBytes, state.OutBytes
}

// DeleteClientBindings removes all ClientInboundTrafficState records for a client.
func (s *NftTrafficService) DeleteClientBindings(tx *gorm.DB, clientId uint) error {
	return tx.Where("client_id = ?", clientId).Delete(&model.ClientInboundTrafficState{}).Error
}

// ResetClientTraffic resets the client's up/down counters and all active binding accumulators.
// Used for explicit traffic resets and scheduled monthly resets.
func (s *NftTrafficService) ResetClientTraffic(tx *gorm.DB, clientId uint) error {
	// Reset client up/down
	if err := tx.Model(&model.Client{}).Where("id = ?", clientId).Updates(map[string]interface{}{
		"up":         0,
		"down":       0,
		"last_reset": time.Now().Unix(),
	}).Error; err != nil {
		return err
	}

	// Reset all active binding accumulators and set baseline to current nftables counter
	var bindings []model.ClientInboundTrafficState
	if err := tx.Where("client_id = ? AND active = ?", clientId, true).Find(&bindings).Error; err != nil {
		return err
	}
	for i := range bindings {
		b := &bindings[i]
		currentIn, currentOut := s.getCurrentInboundBytes(tx, b.InboundId)
		b.LastInBytes = currentIn
		b.LastOutBytes = currentOut
		b.UsedInBytes = 0
		b.UsedOutBytes = 0
		b.UpdatedAt = time.Now()
		if err := tx.Save(b).Error; err != nil {
			return err
		}
	}

	return nil
}

// QueueClientTrafficReset keeps nft counter reads outside the transaction that
// changed the client record.
func (s *NftTrafficService) QueueClientTrafficReset(tx *gorm.DB, clientId uint) error {
	return QueueManagedRuntimeHook(tx, func() error {
		if err := s.ResetClientTraffic(database.GetDB(), clientId); err != nil {
			logger.Warning("failed to reset client nft traffic baseline for client id ", clientId, ": ", err)
		}
		return nil
	})
}

// getCurrentInboundBytes returns the current cumulative nftables counter bytes
// for the given inbound. Used to set baselines.
func (s *NftTrafficService) getCurrentInboundBytes(tx *gorm.DB, inboundId uint) (int64, int64) {
	var state model.InboundTrafficState
	result := tx.Where("inbound_id = ?", inboundId).First(&state)
	if result.Error != nil {
		return 0, 0
	}

	// Read current counter values from nftables
	inBytes, err := getChainRuleBytesByHandle(nftChainIn, state.InHandle)
	if err != nil {
		s.tryRecoverHandles(tx, &state)
		inBytes, err = getChainRuleBytesByHandle(nftChainIn, state.InHandle)
		if err != nil {
			inBytes = state.InBytes // fallback to last known
		}
	}

	outBytes, err := getChainRuleBytesByHandle(nftChainOut, state.OutHandle)
	if err != nil {
		s.tryRecoverHandles(tx, &state)
		outBytes, err = getChainRuleBytesByHandle(nftChainOut, state.OutHandle)
		if err != nil {
			outBytes = state.OutBytes // fallback to last known
		}
	}

	return inBytes, outBytes
}

// ---------------------------------------------------------------------------
// Periodic traffic collection
// ---------------------------------------------------------------------------

// RefreshPortHopRedirects refreshes REDIRECT rules based on each inbound's port_hop_interval.
// This runs independently from traffic statistics collection.
func (s *NftTrafficService) RefreshPortHopRedirects() error {
	return runDefaultInboundNftMutation(func() error {
		return s.refreshPortHopRedirects()
	})
}

func (s *NftTrafficService) refreshPortHopRedirects() error {
	if !IsSystemPlatformLinux() || !nftSupported() {
		return nil
	}

	coreSvc := &CoreManagerService{}
	if !coreSvc.IsRunning() {
		return nil
	}

	db := database.GetDB()
	var states []model.InboundTrafficState
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

	var inbounds []model.Inbound
	if err := db.Model(&model.Inbound{}).Select("id, options").Where("id IN ?", inboundIDs).Find(&inbounds).Error; err != nil {
		return err
	}
	inboundsByID := make(map[uint]model.Inbound, len(inbounds))
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
			clearPortHopRefresh(state.InboundId)
			logger.Warning("skip port hop refresh because inbound no longer exists: ", state.Tag)
			continue
		}
		interval, ok := parsePortHopInterval(extractPortHopInterval(inbound.Options))
		if !ok {
			clearPortHopRefresh(state.InboundId)
			continue
		}

		now := time.Now()
		if !shouldRefreshPortHop(state.InboundId, now, interval) {
			continue
		}

		// nft can block for seconds on a busy host. Keep it outside the SQLite write.
		if err := deleteNftRedirectRulesByComment(singboxNftRuleComments.redirect(state.Tag)); err != nil {
			logger.Warning("failed to delete existing nftables REDIRECT rule by comment for inbound ", state.Tag, ": ", err)
		}

		hopNft, skipped, sample := portHopRangeToNftWithExclusions(state.PortHopRange, state.Port)
		if skipped > 0 {
			if len(sample) > 0 {
				logger.Info("port hop interval refresh for inbound ", state.Tag, ": skipped ", skipped, " UDP ports (sample ", sample, ")")
			} else {
				logger.Info("port hop interval refresh for inbound ", state.Tag, ": skipped ", skipped, " UDP ports")
			}
		}

		redirectHandle := 0
		if hopNft != "" {
			handle, err := addRedirectRule(hopNft, state.Port, singboxNftRuleComments.redirect(state.Tag))
			if err != nil {
				logger.Warning("failed to refresh port hop redirect for inbound ", state.Tag, ": ", err)
				if firstErr == nil {
					firstErr = err
				}
				continue
			}
			redirectHandle = handle
		}

		result := db.Model(&model.InboundTrafficState{}).
			Where("id = ? AND tag = ? AND port = ? AND port_hop_range = ? AND redirect_handle = ?",
				state.Id, state.Tag, state.Port, state.PortHopRange, state.RedirectHandle).
			Updates(map[string]interface{}{
				"redirect_handle": redirectHandle,
				"updated_at":      now,
			})
		if result.Error != nil {
			logger.Warning("failed to save port hop redirect handle for inbound ", state.Tag, ": ", result.Error)
			if firstErr == nil {
				firstErr = result.Error
			}
			if err := deleteNftRedirectRulesByComment(singboxNftRuleComments.redirect(state.Tag)); err != nil {
				logger.Warning("failed to remove unsaved nftables REDIRECT rule for inbound ", state.Tag, ": ", err)
			}
			continue
		}
		if result.RowsAffected == 0 {
			// A newer save may already have installed a rule with the same stable
			// comment. Do not delete by comment here: that would erase the newer
			// rule and leave the inbound unreachable until a later reconciliation.
			logger.Warning("skip stale port hop redirect result for inbound ", state.Tag)
			continue
		}
		markPortHopRefreshed(state.InboundId, now)
	}

	return firstErr
}

// CollectAndSaveTraffic reads nftables counters, computes deltas, and writes
// Stats records for both inbounds (resource="inbound") and clients (resource="client").
func (s *NftTrafficService) CollectAndSaveTraffic() (bool, error) {
	return s.collectAndSaveTraffic(nil)
}

// CollectAndSaveTrafficWithHistory lets the serialized runtime sampler reuse
// its one trafficAge read for both Core chains without changing the legacy
// no-argument method contract.
func (s *NftTrafficService) CollectAndSaveTrafficWithHistory(saveTraffic bool) (bool, error) {
	return s.collectAndSaveTraffic(&saveTraffic)
}

func (s *NftTrafficService) collectAndSaveTraffic(saveTrafficOverride *bool) (bool, error) {
	if !IsSystemPlatformLinux() || !nftSupported() {
		setOnlines(nil, nil, nil)
		return false, nil
	}
	samplingEpoch, samplingAllowed := captureRuntimeTrafficSamplingEpoch()
	if !samplingAllowed {
		return false, nil
	}

	coreSvc := &CoreManagerService{}
	if !coreSvc.IsRunning() {
		setOnlines(nil, nil, nil)
		return false, nil
	}

	db := database.GetDB()

	var states []model.InboundTrafficState
	if err := db.Find(&states).Error; err != nil {
		return false, err
	}

	if len(states) == 0 {
		// Legacy self-heal: old deployments may have inbounds but no nft state rows yet.
		s.InitOnStartup()
		if err := db.Find(&states).Error; err != nil {
			return false, err
		}
		if len(states) == 0 {
			setOnlines(nil, nil, nil)
			return false, nil
		}
	}

	saveTraffic := true
	if saveTrafficOverride != nil {
		saveTraffic = *saveTrafficOverride
	} else if trafficAge, err := (&SettingService{}).GetTrafficAge(); err == nil {
		saveTraffic = trafficAge > 0
	} else {
		logger.Warning("failed to load trafficAge for nft collection: ", err)
	}
	if saveTraffic {
		if err := EnsureHistoryStorageReady(); err != nil {
			return false, err
		}
		if err := runtimeTrafficStats.ensureReady(); err != nil {
			return false, err
		}
	}

	// nft commands can take seconds on a busy host. Read all counters before opening
	// the SQLite transaction so the single database connection remains available.
	// Keep the two chain snapshots out of the same window as rule replacement.
	// A concurrent save, restart or integrity repair otherwise makes a single
	// missing handle fall back to per-inbound nft reads for the whole batch.
	var samples []inboundTrafficSample
	if err := runDefaultInboundNftMutation(func() error {
		samples = s.readInboundTrafficSamples(states)
		return nil
	}); err != nil {
		return false, err
	}
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
		setOnlines(nil, nil, nil)
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
		// A batch snapshot contains every managed inbound, but only counter or
		// handle changes need a SQLite write. Updating unchanged rows every ten
		// seconds needlessly dirties the database and extends the single-connection
		// transaction.
		if !sample.stateChanged {
			continue
		}
		result := tx.Model(&model.InboundTrafficState{}).
			Where("id = ? AND tag = ? AND port = ? AND in_handle = ? AND out_handle = ? AND redirect_handle = ? AND in_bytes = ? AND out_bytes = ?",
				sample.state.Id,
				sample.state.Tag,
				sample.state.Port,
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
				Resource:  "inbound",
				Tag:       delta.tag,
				Direction: true,
				Traffic:   delta.deltaIn,
			})
		}
		if saveTraffic && delta.deltaOut > 0 {
			historySamples = append(historySamples, model.Stats{
				DateTime:  now,
				Resource:  "inbound",
				Tag:       delta.tag,
				Direction: false,
				Traffic:   delta.deltaOut,
			})
		}
	}

	userOnlines := []string{}
	if len(deltas) > 0 {
		if err := s.ensureClientBindings(tx, snapshots); err != nil {
			return false, err
		}
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
			logger.Warning("flush traffic runtime journal at capacity threshold failed: ", err)
		}
	}
	setOnlines(tagsFromSet(inboundOnlineSet), userOnlines, nil)
	return len(deltas) > 0, nil
}

// readInboundTrafficSamples batches normal counter reads into one nft command
// per direction. A failed snapshot falls back to the existing per-inbound
// recovery path so externally removed rules still self-heal correctly.
func (s *NftTrafficService) readInboundTrafficSamples(states []model.InboundTrafficState) []inboundTrafficSample {
	if len(states) == 0 {
		return nil
	}

	prepared := make([]model.InboundTrafficState, len(states))
	originalByInboundID := make(map[uint]model.InboundTrafficState, len(states))
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
			logger.Warning("failed to batch-read nft input counters: ", inputErr)
		}
		if outputErr != nil {
			logger.Warning("failed to batch-read nft output counters: ", outputErr)
		}
		return s.readInboundTrafficSamplesIndividually(states)
	}

	samples := make([]inboundTrafficSample, 0, len(prepared))
	for _, state := range prepared {
		currentIn, inOK := inputBytes[state.InHandle]
		currentOut, outOK := outputBytes[state.OutHandle]
		if !inOK || !outOK {
			return s.readInboundTrafficSamplesIndividually(states)
		}
		samples = append(samples, newInboundTrafficSample(state, originalByInboundID[state.InboundId], currentIn, currentOut))
	}
	return samples
}

func (s *NftTrafficService) readInboundTrafficSamplesIndividually(states []model.InboundTrafficState) []inboundTrafficSample {
	samples := make([]inboundTrafficSample, 0, len(states))
	for _, state := range states {
		sample, ok := s.readInboundTrafficSample(state)
		if ok {
			samples = append(samples, sample)
		}
	}
	return samples
}

func newInboundTrafficSample(state model.InboundTrafficState, original model.InboundTrafficState, currentIn int64, currentOut int64) inboundTrafficSample {
	deltaIn := currentIn - original.InBytes
	deltaOut := currentOut - original.OutBytes
	if deltaIn < 0 {
		deltaIn = currentIn
	}
	if deltaOut < 0 {
		deltaOut = currentOut
	}
	return inboundTrafficSample{
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

func (s *NftTrafficService) readInboundTrafficSample(state model.InboundTrafficState) (inboundTrafficSample, bool) {
	sample := inboundTrafficSample{
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
		logger.Warning("failed to read nftables input counter for inbound ", state.Tag, ": ", errIn)
		return sample, false
	}

	currentOut, errOut := getChainRuleBytesByHandle(nftChainOut, state.OutHandle)
	if errOut != nil {
		s.recoverHandles(&state)
		currentOut, errOut = getChainRuleBytesByHandle(nftChainOut, state.OutHandle)
	}
	if errOut != nil {
		logger.Warning("failed to read nftables output counter for inbound ", state.Tag, ": ", errOut)
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

// writeClientStats aggregates inbound deltas for each client's active bindings
// and writes Stats records with resource="client".
func (s *NftTrafficService) writeClientStats(tx *gorm.DB, deltas []inboundDelta, now int64, saveTraffic bool, historySamples *[]model.Stats) ([]string, error) {
	// Build inbound delta map
	deltaMap := make(map[uint]*inboundDelta)
	for i := range deltas {
		deltaMap[deltas[i].inboundId] = &deltas[i]
	}

	// Only bindings for inbounds that changed in this sampling round can
	// contribute traffic. Querying every active binding made the hot path scale
	// with the total client topology even when one inbound was the only one
	// carrying traffic.
	inboundIDs := make([]uint, 0, len(deltaMap))
	for inboundID := range deltaMap {
		inboundIDs = append(inboundIDs, inboundID)
	}
	sort.Slice(inboundIDs, func(i, j int) bool { return inboundIDs[i] < inboundIDs[j] })

	var bindings []model.ClientInboundTrafficState
	if err := tx.Where("active = ? AND inbound_id IN ?", true, inboundIDs).Find(&bindings).Error; err != nil {
		return nil, err
	}

	if len(bindings) == 0 {
		return nil, nil
	}

	// Get client names
	clientIds := make([]uint, 0)
	for _, b := range bindings {
		clientIds = append(clientIds, b.ClientId)
	}
	// Deduplicate
	uniqueClientIds := make(map[uint]bool)
	for _, id := range clientIds {
		uniqueClientIds[id] = true
	}

	clientNames := make(map[uint]string)
	var clients []model.Client
	dedupIds := make([]uint, 0, len(uniqueClientIds))
	for id := range uniqueClientIds {
		dedupIds = append(dedupIds, id)
	}
	if err := tx.Model(model.Client{}).Where("id in ?", dedupIds).Select("id, name").Find(&clients).Error; err != nil {
		return nil, err
	}
	for _, c := range clients {
		clientNames[c.Id] = c.Name
	}

	// Aggregate deltas per client
	type clientAgg struct {
		upTotal   int64
		downTotal int64
	}
	clientAggs := make(map[uint]*clientAgg)
	changedBindings := make([]model.ClientInboundTrafficState, 0, len(bindings))

	for i := range bindings {
		b := &bindings[i]
		d, ok := deltaMap[b.InboundId]
		if !ok || (d.deltaIn == 0 && d.deltaOut == 0) {
			continue
		}

		agg, ok := clientAggs[b.ClientId]
		if !ok {
			agg = &clientAgg{}
			clientAggs[b.ClientId] = agg
		}
		agg.upTotal += d.deltaIn
		agg.downTotal += d.deltaOut

		// Update binding accumulators
		b.UsedInBytes += d.deltaIn
		b.UsedOutBytes += d.deltaOut
		b.LastInBytes = d.currentIn
		b.LastOutBytes = d.currentOut
		b.UpdatedAt = time.Now()
		changedBindings = append(changedBindings, *b)
	}
	if err := saveClientInboundTrafficBindingsBatch(tx, changedBindings); err != nil {
		return nil, err
	}

	userOnlineSet := make(map[string]struct{}, len(clientAggs))

	// Write client Stats records and update client up/down
	upDeltas := make(map[uint]int64)
	downDeltas := make(map[uint]int64)
	for clientId, agg := range clientAggs {
		name, ok := clientNames[clientId]
		if !ok {
			continue
		}
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		userOnlineSet[name] = struct{}{}
		if agg.upTotal > 0 {
			if saveTraffic {
				*historySamples = append(*historySamples, model.Stats{
					DateTime:  now,
					Resource:  "client",
					Tag:       name,
					Direction: true,
					Traffic:   agg.upTotal,
				})
			}
			// Update client.up
			upDeltas[clientId] = agg.upTotal
		}
		if agg.downTotal > 0 {
			if saveTraffic {
				*historySamples = append(*historySamples, model.Stats{
					DateTime:  now,
					Resource:  "client",
					Tag:       name,
					Direction: false,
					Traffic:   agg.downTotal,
				})
			}
			// Update client.down
			downDeltas[clientId] = agg.downTotal
		}
	}
	if err := applyClientTrafficDeltasBatch(tx, upDeltas, downDeltas); err != nil {
		return nil, err
	}

	return tagsFromSet(userOnlineSet), nil
}

func saveClientInboundTrafficBindingsBatch(tx *gorm.DB, bindings []model.ClientInboundTrafficState) error {
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

func applyClientTrafficDeltasBatch(tx *gorm.DB, up map[uint]int64, down map[uint]int64) error {
	if tx == nil {
		return nil
	}
	if err := applyClientTrafficDeltaColumn(tx, "up", up); err != nil {
		return err
	}
	return applyClientTrafficDeltaColumn(tx, "down", down)
}

func applyClientTrafficDeltaColumn(tx *gorm.DB, column string, deltas map[uint]int64) error {
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
	query := fmt.Sprintf("UPDATE clients SET %s = %s + %s WHERE id IN (%s)", column, column, caseSQL, strings.Join(placeholders, ","))
	return tx.Exec(query, args...).Error
}

func (s *NftTrafficService) ensureClientBindings(tx *gorm.DB, snapshots map[uint]inboundCounterSnapshot) error {
	if tx == nil {
		return nil
	}
	if !defaultClientBindingRepairNeeded.Load() {
		return nil
	}

	var clients []model.Client
	if err := tx.Model(model.Client{}).
		Select("id, inbounds").
		Find(&clients).Error; err != nil {
		return err
	}

	for i := range clients {
		client := &clients[i]
		if client.Id == 0 {
			continue
		}
		if err := s.syncClientBindings(tx, client.Id, parseNftClientInboundIDs(client.Inbounds), snapshots); err != nil {
			return err
		}
	}
	defaultClientBindingRepairNeeded.Store(false)
	return nil
}

func parseNftClientInboundIDs(raw json.RawMessage) []uint {
	if len(raw) == 0 {
		return []uint{}
	}

	var ids []uint
	if err := json.Unmarshal(raw, &ids); err == nil {
		return deduplicateInboundIDs(ids)
	}

	var mixed []interface{}
	if err := json.Unmarshal(raw, &mixed); err != nil {
		return []uint{}
	}

	parsed := make([]uint, 0, len(mixed))
	for _, item := range mixed {
		switch value := item.(type) {
		case float64:
			if value > 0 && math.Trunc(value) == value {
				parsed = append(parsed, uint(value))
			}
		case int:
			if value > 0 {
				parsed = append(parsed, uint(value))
			}
		case string:
			numeric, err := strconv.ParseUint(strings.TrimSpace(value), 10, 64)
			if err == nil && numeric > 0 {
				parsed = append(parsed, uint(numeric))
			}
		case json.Number:
			numeric, err := value.Int64()
			if err == nil && numeric > 0 {
				parsed = append(parsed, uint(numeric))
			}
		}
	}

	return deduplicateInboundIDs(parsed)
}

func shouldRefreshPortHop(inboundID uint, now time.Time, interval time.Duration) bool {
	portHopRefreshState.mu.Lock()
	defer portHopRefreshState.mu.Unlock()

	last, ok := portHopRefreshState.last[inboundID]
	if !ok {
		return true
	}
	return now.Sub(last) >= interval
}

func markPortHopRefreshed(inboundID uint, now time.Time) {
	portHopRefreshState.mu.Lock()
	defer portHopRefreshState.mu.Unlock()
	portHopRefreshState.last[inboundID] = now
}

func clearPortHopRefresh(inboundID uint) {
	portHopRefreshState.mu.Lock()
	defer portHopRefreshState.mu.Unlock()
	delete(portHopRefreshState.last, inboundID)
}

func parsePortHopInterval(raw string) (time.Duration, bool) {
	trimmed := strings.TrimSpace(strings.ToLower(raw))
	if trimmed == "" {
		return 0, false
	}
	duration, err := time.ParseDuration(trimmed)
	if err != nil || duration <= 0 {
		return 0, false
	}
	return duration, true
}

// ---------------------------------------------------------------------------
// Startup initialization
// ---------------------------------------------------------------------------

// InitOnStartup restores nftables rules for all existing inbounds that have
// InboundTrafficState records. Also creates rules for inbounds that don't have
// state records yet.
func (s *NftTrafficService) InitOnStartup() {
	_ = runDefaultInboundNftMutation(func() error {
		s.initOnStartup()
		return nil
	})
}

func (s *NftTrafficService) initOnStartup() {
	if !nftSupported() {
		logger.Info("nftables not supported on this platform, skipping traffic rule initialization")
		return
	}

	db := database.GetDB()

	// Get all inbounds
	var inbounds []model.Inbound
	if err := db.Find(&inbounds).Error; err != nil {
		logger.Warning("failed to load inbounds for nftables init: ", err)
		return
	}

	for _, inbound := range inbounds {
		port := extractPort(inbound.Options)
		if port <= 0 {
			continue
		}

		portHopRange := extractPortHopRange(inbound.Options)

		// Check if state already exists
		var state model.InboundTrafficState
		result := db.Where("inbound_id = ?", inbound.Id).First(&state)

		if result.Error == nil {
			// State exists - recreate nftables rules (they don't survive reboot).
			// Port-hop compatibility rules have no single stored handle, so their
			// configured range must also trigger comment-based cleanup.
			if state.InHandle > 0 || state.OutHandle > 0 || state.RedirectHandle > 0 || strings.TrimSpace(state.PortHopRange) != "" {
				if rmErr := s.removeRulesFromState(&state); rmErr != nil {
					logger.Warning("failed to cleanup old nftables rules on startup for inbound ", inbound.Tag, ": ", rmErr)
				}
			}

			inHandle, inErr := addPortCounterRule(nftChainIn, port, "dport", singboxNftRuleComments.in(inbound.Tag))
			if inErr != nil {
				logger.Warning("failed to restore nftables input rule for inbound ", inbound.Tag, ": ", inErr)
			}
			outHandle, outErr := addPortCounterRule(nftChainOut, port, "sport", singboxNftRuleComments.out(inbound.Tag))
			if outErr != nil {
				logger.Warning("failed to restore nftables output rule for inbound ", inbound.Tag, ": ", outErr)
			}

			// Recreate REDIRECT rule for port hopping if needed
			var redirectHandle int
			if portHopRange != "" {
				hopNft, skipped, sample := portHopRangeToNftWithExclusions(portHopRange, port)
				if skipped > 0 {
					if len(sample) > 0 {
						logger.Info("port hop range for inbound ", inbound.Tag, ": skipped ", skipped, " UDP ports (sample ", sample, ")")
					} else {
						logger.Info("port hop range for inbound ", inbound.Tag, ": skipped ", skipped, " UDP ports")
					}
				}
				if hopNft != "" {
					var redirectErr error
					redirectHandle, redirectErr = addRedirectRule(hopNft, port, singboxNftRuleComments.redirect(inbound.Tag))
					if redirectErr != nil {
						logger.Warning("failed to restore nftables REDIRECT rule for inbound ", inbound.Tag, ": ", redirectErr)
					}
					if redirectHandle > 0 {
						logger.Info("nftables REDIRECT rule restored: UDP ", hopNft, " -> :", port)
					}
				} else {
					logger.Warning("port hop range for inbound ", inbound.Tag, " has no available UDP ports after exclusion")
				}
			}

			// Reset counter baselines (rules are new, counters start at 0)
			if updateErr := db.Model(&state).Updates(map[string]interface{}{
				"in_handle":       inHandle,
				"out_handle":      outHandle,
				"redirect_handle": redirectHandle,
				"port_hop_range":  portHopRange,
				"in_bytes":        0,
				"out_bytes":       0,
				"port":            port,
				"tag":             inbound.Tag,
				"updated_at":      time.Now(),
			}).Error; updateErr != nil {
				logger.Warning("failed to update inbound traffic state on startup for inbound ", inbound.Tag, ": ", updateErr)
			}

			// Also reset client binding baselines to 0 (since nftables counters are reset)
			if bindingErr := db.Model(&model.ClientInboundTrafficState{}).
				Where("inbound_id = ? AND active = ?", inbound.Id, true).
				Updates(map[string]interface{}{
					"last_in_bytes":  0,
					"last_out_bytes": 0,
					"updated_at":     time.Now(),
				}).Error; bindingErr != nil {
				logger.Warning("failed to reset client inbound baselines on startup for inbound ", inbound.Tag, ": ", bindingErr)
			}
		} else if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			// No state - create new
			if setupErr := s.SetupInboundRules(db, inbound.Id, inbound.Tag, port, portHopRange); setupErr != nil {
				logger.Warning("failed to setup startup nftables rules for inbound ", inbound.Tag, ": ", setupErr)
				continue
			}
		} else {
			logger.Warning("failed to query inbound traffic state on startup for inbound ", inbound.Tag, ": ", result.Error)
		}
	}

	logger.Info("nftables traffic rules initialized for ", len(inbounds), " inbounds")
}

// CleanupOnShutdown removes all nftables rules created by this service.
// Should be called when the program is stopping.
func (s *NftTrafficService) CleanupOnShutdown() {
	_ = runDefaultInboundNftMutation(func() error {
		s.cleanupOnShutdown()
		return nil
	})
}

func (s *NftTrafficService) cleanupOnShutdown() {
	portHopRefreshState.mu.Lock()
	portHopRefreshState.last = map[uint]time.Time{}
	portHopRefreshState.mu.Unlock()

	if err := deleteRulesByCommentPrefix(singboxNftRuleComments.prefix); err != nil {
		logger.Warning("failed to cleanup sing-box nft rules by prefix: ", err)
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
		Model(&model.InboundTrafficState{}).
		Updates(updates).Error; err != nil {
		logger.Warning("failed to reset inbound nft state after cleanup: ", err)
	}

	if err := db.Model(&model.ClientInboundTrafficState{}).
		Where("active = ?", true).
		Updates(map[string]interface{}{
			"last_in_bytes":  0,
			"last_out_bytes": 0,
			"updated_at":     now,
		}).Error; err != nil {
		logger.Warning("failed to reset active client baselines after nft cleanup: ", err)
	}
}

// recoverHandles attempts to find missing handles by searching nftables rules by comment.
// It only updates the supplied snapshot, so callers can perform nft reads without a DB transaction.
func (s *NftTrafficService) recoverHandles(st *model.InboundTrafficState) bool {
	if st == nil {
		return false
	}
	changed := false

	comment := singboxNftRuleComments.in(st.Tag)
	handle := findHandleByComment(nftChainIn, comment)
	if handle > 0 && handle != st.InHandle {
		st.InHandle = handle
		changed = true
		logger.Info("recovered nftables input handle for ", st.Tag, ": ", handle)
	}

	comment = singboxNftRuleComments.out(st.Tag)
	handle = findHandleByComment(nftChainOut, comment)
	if handle > 0 && handle != st.OutHandle {
		st.OutHandle = handle
		changed = true
		logger.Info("recovered nftables output handle for ", st.Tag, ": ", handle)
	}

	// Compatibility REDIRECT rules are tracked by comment rather than a single
	// handle. Reset a stale modern handle when the active layout is compatible.
	if st.PortHopRange != "" {
		comment = singboxNftRuleComments.redirect(st.Tag)
		handle = findNftRedirectHandleByComment(comment)
		if handle != st.RedirectHandle {
			st.RedirectHandle = handle
			changed = true
			if handle > 0 {
				logger.Info("recovered nftables REDIRECT handle for ", st.Tag, ": ", handle)
			}
		}
	}

	return changed
}

// tryRecoverHandles persists recovered handles for call paths that already own a transaction.
func (s *NftTrafficService) tryRecoverHandles(tx *gorm.DB, st *model.InboundTrafficState) {
	if tx == nil || !s.recoverHandles(st) {
		return
	}
	if err := tx.Model(st).Updates(map[string]interface{}{
		"in_handle":       st.InHandle,
		"out_handle":      st.OutHandle,
		"redirect_handle": st.RedirectHandle,
		"updated_at":      time.Now(),
	}).Error; err != nil {
		logger.Warning("failed to persist recovered nftables handles for inbound ", st.Tag, ": ", err)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// extractPort extracts the listen_port from inbound Options JSON.
func extractPort(options json.RawMessage) int {
	if options == nil {
		return 0
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(options, &fields); err != nil {
		return 0
	}
	portRaw, ok := fields["listen_port"]
	if !ok {
		return 0
	}
	var port int
	if err := json.Unmarshal(portRaw, &port); err != nil {
		return 0
	}
	return port
}

// extractPortHopRange extracts the port_hop_range string from inbound Options JSON.
// Returns empty string if not present.
func extractPortHopRange(options json.RawMessage) string {
	if options == nil {
		return ""
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(options, &fields); err != nil {
		return ""
	}
	raw, ok := fields["port_hop_range"]
	if !ok {
		return ""
	}
	var portHopRange string
	if err := json.Unmarshal(raw, &portHopRange); err != nil {
		return ""
	}
	return portHopRange
}

// extractPortHopInterval extracts port_hop_interval from inbound Options JSON.
// Returns empty string if not present.
func extractPortHopInterval(options json.RawMessage) string {
	if options == nil {
		return ""
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(options, &fields); err != nil {
		return ""
	}
	raw, ok := fields["port_hop_interval"]
	if !ok {
		return ""
	}
	var interval string
	if err := json.Unmarshal(raw, &interval); err != nil {
		return ""
	}
	return interval
}

// inboundDelta is used internally for traffic collection.
type inboundDelta struct {
	inboundId  uint
	tag        string
	deltaIn    int64
	deltaOut   int64
	currentIn  int64
	currentOut int64
}
