package service

import (
	"encoding/json"
	"errors"
	"math"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/logger"

	"gorm.io/gorm"
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

var portHopRefreshState = struct {
	mu   sync.Mutex
	last map[uint]time.Time
}{
	last: map[uint]time.Time{},
}

func (s *NftTrafficService) IsNftTableReady() bool {
	return nftTableExists()
}

// EnsureRuleIntegrity verifies inbound nftables rules are still present and
// recreates missing ones when rules are externally removed.
func (s *NftTrafficService) EnsureRuleIntegrity() error {
	if runtime.GOOS != "linux" || !nftSupported() {
		return nil
	}

	coreSvc := &CoreManagerService{}
	if !coreSvc.IsRunning() {
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

	tx := db.Begin()
	if tx.Error != nil {
		return tx.Error
	}

	var firstErr error
	for _, inbound := range inbounds {
		port := extractPort(inbound.Options)
		if port <= 0 {
			continue
		}
		portHopRange := extractPortHopRange(inbound.Options)
		if err := s.ensureInboundRuleIntegrity(tx, &inbound, port, portHopRange); err != nil {
			logger.Warning("nft rule integrity check failed for inbound ", inbound.Tag, ": ", err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}

	if err := s.cleanupOrphanInboundRules(validComments); err != nil {
		logger.Warning("cleanup orphan inbound nft rules failed: ", err)
		if firstErr == nil {
			firstErr = err
		}
	}

	if err := tx.Commit().Error; err != nil {
		tx.Rollback()
		return err
	}
	return firstErr
}

type chainRuleComment struct {
	handle  int
	comment string
}

func (s *NftTrafficService) cleanupOrphanInboundRules(validComments map[string]struct{}) error {
	if !nftSupported() || !nftTableExists() {
		return nil
	}

	chains := []string{nftChainIn, nftChainOut, nftChainPrerouting}
	var firstErr error
	for _, chain := range chains {
		rules, err := listRuleCommentsByPrefix(chain, singboxNftRuleComments.prefix)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		for _, rule := range rules {
			if _, ok := validComments[rule.comment]; ok {
				continue
			}
			if err = deleteRuleByHandle(chain, rule.handle); err != nil {
				logger.Warning("failed to delete orphan inbound nft rule ", rule.comment, " handle ", rule.handle, ": ", err)
				if firstErr == nil {
					firstErr = err
				}
			}
		}
	}
	return firstErr
}

func (s *NftTrafficService) ensureInboundRuleIntegrity(tx *gorm.DB, inbound *model.Inbound, port int, portHopRange string) error {
	if inbound == nil || inbound.Id == 0 || port <= 0 {
		return nil
	}

	var state model.InboundTrafficState
	result := tx.Where("inbound_id = ?", inbound.Id).First(&state)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return s.SetupInboundRules(tx, inbound.Id, inbound.Tag, port, portHopRange)
	}
	if result.Error != nil {
		return result.Error
	}

	// Keep DB state aligned to the current inbound definition.
	if state.Tag != inbound.Tag || state.Port != port || state.PortHopRange != portHopRange {
		return s.UpdateInboundRules(tx, inbound.Id, inbound.Tag, port, portHopRange)
	}

	s.tryRecoverHandles(tx, &state)
	missing := false

	if state.InHandle <= 0 {
		missing = true
	} else if _, err := getChainRuleBytesByHandle(nftChainIn, state.InHandle); err != nil {
		missing = true
	}

	if state.OutHandle <= 0 {
		missing = true
	} else if _, err := getChainRuleBytesByHandle(nftChainOut, state.OutHandle); err != nil {
		missing = true
	}

	if portHopRange != "" {
		comment := singboxNftRuleComments.redirect(inbound.Tag)
		if state.RedirectHandle <= 0 {
			if handle := findHandleByComment(nftChainPrerouting, comment); handle > 0 {
				state.RedirectHandle = handle
				if err := tx.Model(&state).Updates(map[string]interface{}{
					"redirect_handle": handle,
					"updated_at":      time.Now(),
				}).Error; err != nil {
					return err
				}
			} else {
				missing = true
			}
		} else if findHandleByComment(nftChainPrerouting, comment) <= 0 {
			missing = true
		}
	} else if state.RedirectHandle > 0 {
		if err := deleteRuleByHandle(nftChainPrerouting, state.RedirectHandle); err != nil {
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
		return nil
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
	if newPort <= 0 {
		return s.RemoveInboundRules(tx, inboundId)
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
	if state.PortHopRange != "" && state.RedirectHandle <= 0 {
		if handle := findHandleByComment(nftChainPrerouting, singboxNftRuleComments.redirect(oldTag)); handle > 0 {
			state.RedirectHandle = handle
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
		if state.RedirectHandle > 0 {
			if err := deleteRuleByHandle(nftChainPrerouting, state.RedirectHandle); err != nil {
				logger.Warning("failed to delete old nftables REDIRECT rule for inbound ", oldTag, ": ", err)
			}
		} else {
			comment := singboxNftRuleComments.redirect(oldTag)
			if err := deleteRuleByComment(nftChainPrerouting, comment); err != nil {
				logger.Warning("failed to delete old nftables REDIRECT rule for inbound ", oldTag, ": ", err)
			}
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

	// Delete REDIRECT rule (port hopping): try by handle first, fallback to comment
	if state.RedirectHandle > 0 {
		if err := deleteRuleByHandle(nftChainPrerouting, state.RedirectHandle); err != nil && firstErr == nil {
			firstErr = err
		}
	} else if state.PortHopRange != "" {
		comment := singboxNftRuleComments.redirect(state.Tag)
		if err := deleteRuleByComment(nftChainPrerouting, comment); err != nil && firstErr == nil {
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
	// Get existing bindings
	var existingBindings []model.ClientInboundTrafficState
	tx.Where("client_id = ?", clientId).Find(&existingBindings)

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
				tx.Save(binding)
			}
		}
	}

	// Activate or create bindings for new inbound IDs
	for _, inboundId := range newInboundIds {
		if existing, ok := existingMap[inboundId]; ok {
			if !existing.Active {
				// Re-activating: reset baseline to current nftables counter
				currentIn, currentOut := s.getCurrentInboundBytes(tx, inboundId)
				existing.Active = true
				existing.LastInBytes = currentIn
				existing.LastOutBytes = currentOut
				existing.UsedInBytes = 0
				existing.UsedOutBytes = 0
				existing.UpdatedAt = time.Now()
				tx.Save(existing)
			}
			// Already active: no change needed
		} else {
			// New binding: create with baseline = current nftables counter
			currentIn, currentOut := s.getCurrentInboundBytes(tx, inboundId)
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
			tx.Create(&binding)
		}
	}

	return nil
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
	tx.Where("client_id = ? AND active = ?", clientId, true).Find(&bindings)
	for i := range bindings {
		b := &bindings[i]
		currentIn, currentOut := s.getCurrentInboundBytes(tx, b.InboundId)
		b.LastInBytes = currentIn
		b.LastOutBytes = currentOut
		b.UsedInBytes = 0
		b.UsedOutBytes = 0
		b.UpdatedAt = time.Now()
		tx.Save(b)
	}

	return nil
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
	if runtime.GOOS != "linux" || !nftSupported() {
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

	tx := db.Begin()
	if tx.Error != nil {
		return tx.Error
	}

	var firstErr error
	for i := range states {
		st := &states[i]
		if st.Port <= 0 || strings.TrimSpace(st.PortHopRange) == "" {
			continue
		}
		if err := s.maybeRefreshPortHopRedirect(tx, st); err != nil {
			logger.Warning("failed to refresh port hop redirect for inbound ", st.Tag, ": ", err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}

	if err := tx.Commit().Error; err != nil {
		tx.Rollback()
		return err
	}
	return firstErr
}

// CollectAndSaveTraffic reads nftables counters, computes deltas, and writes
// Stats records for both inbounds (resource="inbound") and clients (resource="client").
func (s *NftTrafficService) CollectAndSaveTraffic() error {
	if runtime.GOOS != "linux" || !nftSupported() {
		setOnlines(nil, nil, nil)
		return nil
	}

	coreSvc := &CoreManagerService{}
	if !coreSvc.IsRunning() {
		setOnlines(nil, nil, nil)
		return nil
	}

	db := database.GetDB()

	var states []model.InboundTrafficState
	if err := db.Find(&states).Error; err != nil {
		return err
	}

	if len(states) == 0 {
		// Legacy self-heal: old deployments may have inbounds but no nft state rows yet.
		s.InitOnStartup()
		if err := db.Find(&states).Error; err != nil {
			return err
		}
		if len(states) == 0 {
			setOnlines(nil, nil, nil)
			return nil
		}
	}

	now := time.Now().Unix()

	tx := db.Begin()
	var txErr error
	defer func() {
		if txErr == nil {
			tx.Commit()
		} else {
			tx.Rollback()
		}
	}()

	// Collect per-inbound deltas
	deltas := make([]inboundDelta, 0, len(states))
	inboundOnlineSet := make(map[string]struct{})

	for i := range states {
		st := &states[i]

		// If handle is 0 (was unknown at creation time), try to recover it by comment
		if st.InHandle <= 0 || st.OutHandle <= 0 {
			s.tryRecoverHandles(tx, st)
		}

		currentIn, errIn := getChainRuleBytesByHandle(nftChainIn, st.InHandle)
		if errIn != nil {
			// Handle may be stale (rule was recreated externally). Try to recover and retry once.
			s.tryRecoverHandles(tx, st)
			currentIn, errIn = getChainRuleBytesByHandle(nftChainIn, st.InHandle)
		}
		if errIn != nil {
			logger.Warning("failed to read nftables input counter for inbound ", st.Tag, ": ", errIn)
			continue
		}
		currentOut, errOut := getChainRuleBytesByHandle(nftChainOut, st.OutHandle)
		if errOut != nil {
			s.tryRecoverHandles(tx, st)
			currentOut, errOut = getChainRuleBytesByHandle(nftChainOut, st.OutHandle)
		}
		if errOut != nil {
			logger.Warning("failed to read nftables output counter for inbound ", st.Tag, ": ", errOut)
			continue
		}

		deltaIn := currentIn - st.InBytes
		deltaOut := currentOut - st.OutBytes

		// Handle counter reset (e.g., nftables rule was recreated)
		if deltaIn < 0 {
			deltaIn = currentIn
		}
		if deltaOut < 0 {
			deltaOut = currentOut
		}

		if deltaIn > 0 || deltaOut > 0 {
			deltas = append(deltas, inboundDelta{
				inboundId:  st.InboundId,
				tag:        st.Tag,
				deltaIn:    deltaIn,
				deltaOut:   deltaOut,
				currentIn:  currentIn,
				currentOut: currentOut,
			})
			inboundOnlineSet[st.Tag] = struct{}{}

			// Write inbound Stats records
			if deltaIn > 0 {
				txErr = tx.Create(&model.Stats{
					DateTime:  now,
					Resource:  "inbound",
					Tag:       st.Tag,
					Direction: true, // upload
					Traffic:   deltaIn,
				}).Error
				if txErr != nil {
					return txErr
				}
			}
			if deltaOut > 0 {
				txErr = tx.Create(&model.Stats{
					DateTime:  now,
					Resource:  "inbound",
					Tag:       st.Tag,
					Direction: false, // download
					Traffic:   deltaOut,
				}).Error
				if txErr != nil {
					return txErr
				}
			}
		}

		// Update cumulative bytes in InboundTrafficState
		txErr = tx.Model(st).Updates(map[string]interface{}{
			"in_bytes":   currentIn,
			"out_bytes":  currentOut,
			"updated_at": time.Now(),
		}).Error
		if txErr != nil {
			return txErr
		}
	}

	// Now compute client deltas from their active bindings
	userOnlines := []string{}
	if len(deltas) > 0 {
		txErr = s.ensureClientBindings(tx)
		if txErr != nil {
			return txErr
		}
		userOnlines, txErr = s.writeClientStats(tx, deltas, now)
		if txErr != nil {
			return txErr
		}
	}

	setOnlines(tagsFromSet(inboundOnlineSet), userOnlines, nil)
	return nil
}

// writeClientStats aggregates inbound deltas for each client's active bindings
// and writes Stats records with resource="client".
func (s *NftTrafficService) writeClientStats(tx *gorm.DB, deltas []inboundDelta, now int64) ([]string, error) {
	// Build inbound delta map
	deltaMap := make(map[uint]*inboundDelta)
	for i := range deltas {
		deltaMap[deltas[i].inboundId] = &deltas[i]
	}

	// Get all active client bindings
	var bindings []model.ClientInboundTrafficState
	if err := tx.Where("active = ?", true).Find(&bindings).Error; err != nil {
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
		if err := tx.Save(b).Error; err != nil {
			return nil, err
		}
	}

	userOnlineSet := make(map[string]struct{}, len(clientAggs))

	// Write client Stats records and update client up/down
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
			if err := tx.Create(&model.Stats{
				DateTime:  now,
				Resource:  "client",
				Tag:       name,
				Direction: true,
				Traffic:   agg.upTotal,
			}).Error; err != nil {
				return nil, err
			}
			// Update client.up
			if err := tx.Model(&model.Client{}).Where("id = ?", clientId).
				UpdateColumn("up", gorm.Expr("up + ?", agg.upTotal)).Error; err != nil {
				return nil, err
			}
		}
		if agg.downTotal > 0 {
			if err := tx.Create(&model.Stats{
				DateTime:  now,
				Resource:  "client",
				Tag:       name,
				Direction: false,
				Traffic:   agg.downTotal,
			}).Error; err != nil {
				return nil, err
			}
			// Update client.down
			if err := tx.Model(&model.Client{}).Where("id = ?", clientId).
				UpdateColumn("down", gorm.Expr("down + ?", agg.downTotal)).Error; err != nil {
				return nil, err
			}
		}
	}

	return tagsFromSet(userOnlineSet), nil
}

func (s *NftTrafficService) ensureClientBindings(tx *gorm.DB) error {
	if tx == nil {
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
		if err := s.SyncClientBindings(tx, client.Id, parseNftClientInboundIDs(client.Inbounds)); err != nil {
			return err
		}
	}

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

func (s *NftTrafficService) maybeRefreshPortHopRedirect(tx *gorm.DB, st *model.InboundTrafficState) error {
	if st == nil || st.InboundId == 0 || st.PortHopRange == "" {
		if st != nil {
			clearPortHopRefresh(st.InboundId)
		}
		return nil
	}

	var inbound model.Inbound
	if err := tx.Model(model.Inbound{}).Select("id, options").Where("id = ?", st.InboundId).First(&inbound).Error; err != nil {
		return err
	}
	intervalRaw := extractPortHopInterval(inbound.Options)
	interval, ok := parsePortHopInterval(intervalRaw)
	if !ok {
		clearPortHopRefresh(st.InboundId)
		return nil
	}

	now := time.Now()
	if !shouldRefreshPortHop(st.InboundId, now, interval) {
		return nil
	}

	if st.RedirectHandle > 0 {
		if err := deleteRuleByHandle(nftChainPrerouting, st.RedirectHandle); err != nil {
			logger.Warning("failed to delete existing nftables REDIRECT rule for inbound ", st.Tag, ": ", err)
		}
	} else {
		comment := singboxNftRuleComments.redirect(st.Tag)
		if err := deleteRuleByComment(nftChainPrerouting, comment); err != nil {
			logger.Warning("failed to delete existing nftables REDIRECT rule by comment for inbound ", st.Tag, ": ", err)
		}
	}

	hopNft, skipped, sample := portHopRangeToNftWithExclusions(st.PortHopRange, st.Port)
	if skipped > 0 {
		if len(sample) > 0 {
			logger.Info("port hop interval refresh for inbound ", st.Tag, ": skipped ", skipped, " UDP ports (sample ", sample, ")")
		} else {
			logger.Info("port hop interval refresh for inbound ", st.Tag, ": skipped ", skipped, " UDP ports")
		}
	}

	redirectHandle := 0
	if hopNft != "" {
		comment := singboxNftRuleComments.redirect(st.Tag)
		handle, err := addRedirectRule(hopNft, st.Port, comment)
		if err != nil {
			return err
		}
		redirectHandle = handle
	}

	st.RedirectHandle = redirectHandle
	if err := tx.Model(st).Updates(map[string]interface{}{
		"redirect_handle": redirectHandle,
		"updated_at":      now,
	}).Error; err != nil {
		return err
	}

	markPortHopRefreshed(st.InboundId, now)
	return nil
}

// ---------------------------------------------------------------------------
// Startup initialization
// ---------------------------------------------------------------------------

// InitOnStartup restores nftables rules for all existing inbounds that have
// InboundTrafficState records. Also creates rules for inbounds that don't have
// state records yet.
func (s *NftTrafficService) InitOnStartup() {
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
			// State exists - recreate nftables rules (they don't survive reboot)
			// Remove old rules only when we have stored handles.
			// This avoids noisy delete attempts after table-wide cleanup.
			if state.InHandle > 0 || state.OutHandle > 0 || state.RedirectHandle > 0 {
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
			tx := db.Begin()
			if tx.Error != nil {
				logger.Warning("failed to open startup transaction for inbound ", inbound.Tag, ": ", tx.Error)
				continue
			}
			if setupErr := s.SetupInboundRules(tx, inbound.Id, inbound.Tag, port, portHopRange); setupErr != nil {
				tx.Rollback()
				logger.Warning("failed to setup startup nftables rules for inbound ", inbound.Tag, ": ", setupErr)
				continue
			}
			if commitErr := tx.Commit().Error; commitErr != nil {
				tx.Rollback()
				logger.Warning("failed to commit startup nftables state for inbound ", inbound.Tag, ": ", commitErr)
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

// tryRecoverHandles attempts to find missing handles by searching nftables rules by comment.
// Updates the database if handles are found.
func (s *NftTrafficService) tryRecoverHandles(tx *gorm.DB, st *model.InboundTrafficState) {
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

	// Also try to recover REDIRECT handle if port hopping is configured
	if st.PortHopRange != "" {
		comment = singboxNftRuleComments.redirect(st.Tag)
		handle = findHandleByComment(nftChainPrerouting, comment)
		if handle > 0 && handle != st.RedirectHandle {
			st.RedirectHandle = handle
			changed = true
			logger.Info("recovered nftables REDIRECT handle for ", st.Tag, ": ", handle)
		}
	}

	if changed {
		tx.Model(st).Updates(map[string]interface{}{
			"in_handle":       st.InHandle,
			"out_handle":      st.OutHandle,
			"redirect_handle": st.RedirectHandle,
			"updated_at":      time.Now(),
		})
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
