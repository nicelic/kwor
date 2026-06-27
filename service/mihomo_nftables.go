package service

import (
	"encoding/json"
	"errors"
	"math"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/logger"
	"gorm.io/gorm"
)

type MihomoNftTrafficService struct{}

var mihomoPortHopRefreshState = struct {
	mu   sync.Mutex
	last map[uint]time.Time
}{
	last: map[uint]time.Time{},
}

func (s *MihomoNftTrafficService) IsNftTableReady() bool {
	return nftTableExists()
}

// EnsureRuleIntegrity verifies mihomo inbound nftables rules and recreates
// missing ones when rules are externally removed.
func (s *MihomoNftTrafficService) EnsureRuleIntegrity() error {
	if runtime.GOOS != "linux" || !nftSupported() {
		return nil
	}

	coreSvc := &MihomoCoreManagerService{}
	if !coreSvc.IsRunning() {
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
		redirectRange, redirectTCP := resolveMihomoInboundRedirectSpec(&inbound)
		if err := s.ensureInboundRuleIntegrity(tx, &inbound, port, redirectRange, redirectTCP); err != nil {
			logger.Warning("mihomo nft integrity check failed for inbound ", inbound.Tag, ": ", err)
			if firstErr == nil {
				firstErr = err
			}
		}
	}

	if err := s.cleanupOrphanInboundRules(validComments); err != nil {
		logger.Warning("cleanup orphan mihomo nft rules failed: ", err)
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

func (s *MihomoNftTrafficService) cleanupOrphanInboundRules(validComments map[string]struct{}) error {
	if !nftSupported() || !nftTableExists() {
		return nil
	}

	chains := []string{nftChainIn, nftChainOut, nftChainPrerouting}
	var firstErr error
	for _, chain := range chains {
		rules, err := listRuleCommentsByPrefix(chain, mihomoNftRuleComments.prefix)
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
				logger.Warning("failed to delete orphan mihomo nft rule ", rule.comment, " handle ", rule.handle, ": ", err)
				if firstErr == nil {
					firstErr = err
				}
			}
		}
	}
	return firstErr
}

func (s *MihomoNftTrafficService) ensureInboundRuleIntegrity(tx *gorm.DB, inbound *model.MihomoInbound, port int, portHopRange string, redirectTCP bool) error {
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

	if strings.TrimSpace(portHopRange) != "" {
		comment := mihomoNftRuleComments.redirect(inbound.Tag)
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
		if state.RedirectHandle <= 0 {
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
	if state.PortHopRange != "" && state.RedirectHandle <= 0 {
		if handle := findHandleByComment(nftChainPrerouting, mihomoNftRuleComments.redirect(oldTag)); handle > 0 {
			state.RedirectHandle = handle
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
		if state.RedirectHandle > 0 {
			if err := deleteRuleByHandle(nftChainPrerouting, state.RedirectHandle); err != nil {
				logger.Warning("failed to delete old mihomo REDIRECT rule for inbound ", oldTag, ": ", err)
			}
		} else {
			_ = deleteRuleByComment(nftChainPrerouting, mihomoNftRuleComments.redirect(oldTag))
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

	if state.RedirectHandle > 0 {
		if err := deleteRuleByHandle(nftChainPrerouting, state.RedirectHandle); err != nil && firstErr == nil {
			firstErr = err
		}
	} else if strings.TrimSpace(state.PortHopRange) != "" {
		if err := deleteRuleByComment(nftChainPrerouting, mihomoNftRuleComments.redirect(state.Tag)); err != nil && firstErr == nil {
			firstErr = err
		}
	}

	return firstErr
}

func (s *MihomoNftTrafficService) removeRedirectRule(state *model.MihomoInboundRedirectState) error {
	if state == nil {
		return nil
	}
	if state.RedirectHandle > 0 {
		return deleteRuleByHandle(nftChainPrerouting, state.RedirectHandle)
	}
	return deleteRuleByComment(nftChainPrerouting, mihomoNftRuleComments.redirect(state.Tag))
}

// SyncClientBindings synchronizes MihomoClientInboundTrafficState rows for one client.
// New or re-activated bindings start from current nft counters (zero residue).
func (s *MihomoNftTrafficService) SyncClientBindings(tx *gorm.DB, clientID uint, newInboundIDs []uint) error {
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
			currentIn, currentOut := s.getCurrentInboundBytes(tx, inboundID)
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

		currentIn, currentOut := s.getCurrentInboundBytes(tx, inboundID)
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

func (s *MihomoNftTrafficService) DeleteClientBindings(tx *gorm.DB, clientID uint) error {
	if clientID == 0 {
		return nil
	}
	return tx.Where("client_id = ?", clientID).Delete(&model.MihomoClientInboundTrafficState{}).Error
}

// ResetClientTraffic resets mihomo client up/down and active binding accumulators.
func (s *MihomoNftTrafficService) ResetClientTraffic(tx *gorm.DB, clientID uint) error {
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
	if runtime.GOOS != "linux" || !nftSupported() {
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
			logger.Warning("failed to refresh mihomo port hop redirect for inbound ", st.Tag, ": ", err)
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

func (s *MihomoNftTrafficService) maybeRefreshPortHopRedirect(tx *gorm.DB, st *model.MihomoInboundRedirectState) error {
	if st == nil || st.InboundId == 0 || strings.TrimSpace(st.PortHopRange) == "" {
		if st != nil {
			clearMihomoPortHopRefresh(st.InboundId)
		}
		return nil
	}

	var inbound model.MihomoInbound
	if err := tx.Model(model.MihomoInbound{}).Select("id, type, options").Where("id = ?", st.InboundId).First(&inbound).Error; err != nil {
		return err
	}

	intervalRaw := extractPortHopInterval(inbound.Options)
	interval, ok := parsePortHopInterval(intervalRaw)
	if !ok {
		clearMihomoPortHopRefresh(st.InboundId)
		return nil
	}

	now := time.Now()
	if !shouldRefreshMihomoPortHop(st.InboundId, now, interval) {
		return nil
	}

	if err := s.removeRedirectRule(st); err != nil {
		logger.Warning("failed to delete existing mihomo REDIRECT rule for inbound ", st.Tag, ": ", err)
	}

	hopNft, skipped, sample := portHopRangeToNftWithExclusions(st.PortHopRange, st.Port)
	if skipped > 0 {
		if len(sample) > 0 {
			logger.Info("mihomo port hop interval refresh for inbound ", st.Tag, ": skipped ", skipped, " UDP ports (sample ", sample, ")")
		} else {
			logger.Info("mihomo port hop interval refresh for inbound ", st.Tag, ": skipped ", skipped, " UDP ports")
		}
	}

	redirectHandle := 0
	if hopNft != "" {
		handle, err := addRedirectRuleWithProtocols(hopNft, st.Port, mihomoNftRuleComments.redirect(st.Tag), inbound.Type == "mieru")
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

	markMihomoPortHopRefreshed(st.InboundId, now)
	return nil
}

// CollectAndSaveTraffic reads mihomo nft counters, writes inbound/client stats,
// and updates cumulative counters in MihomoInboundRedirectState.
func (s *MihomoNftTrafficService) CollectAndSaveTraffic() error {
	if runtime.GOOS != "linux" || !nftSupported() {
		return nil
	}

	coreSvc := &MihomoCoreManagerService{}
	if !coreSvc.IsRunning() {
		setMihomoOnlines(nil, nil)
		return nil
	}

	db := database.GetDB()
	var states []model.MihomoInboundRedirectState
	if err := db.Find(&states).Error; err != nil {
		return err
	}

	if len(states) == 0 {
		// Legacy self-heal: old deployments may have mihomo inbounds but no nft state rows yet.
		s.InitOnStartup()
		if err := db.Find(&states).Error; err != nil {
			return err
		}
		if len(states) == 0 {
			setMihomoOnlines(nil, nil)
			return nil
		}
	}

	now := time.Now().Unix()
	saveTraffic := true
	if trafficAge, err := (&SettingService{}).GetTrafficAge(); err == nil {
		saveTraffic = trafficAge > 0
	} else {
		logger.Warning("failed to load trafficAge for mihomo nft collection: ", err)
	}
	if saveTraffic {
		if err := EnsureHistoryStorageReady(); err != nil {
			return err
		}
	}

	tx := db.Begin()
	if tx.Error != nil {
		return tx.Error
	}

	deltas := make([]inboundDelta, 0, len(states))
	inboundOnlineSet := make(map[string]struct{})

	for i := range states {
		st := &states[i]
		if st.InHandle <= 0 || st.OutHandle <= 0 {
			s.tryRecoverHandles(tx, st)
		}

		currentIn, errIn := getChainRuleBytesByHandle(nftChainIn, st.InHandle)
		if errIn != nil {
			s.tryRecoverHandles(tx, st)
			currentIn, errIn = getChainRuleBytesByHandle(nftChainIn, st.InHandle)
		}
		if errIn != nil {
			logger.Warning("failed to read mihomo nft input counter for inbound ", st.Tag, ": ", errIn)
			continue
		}

		currentOut, errOut := getChainRuleBytesByHandle(nftChainOut, st.OutHandle)
		if errOut != nil {
			s.tryRecoverHandles(tx, st)
			currentOut, errOut = getChainRuleBytesByHandle(nftChainOut, st.OutHandle)
		}
		if errOut != nil {
			logger.Warning("failed to read mihomo nft output counter for inbound ", st.Tag, ": ", errOut)
			continue
		}

		deltaIn := currentIn - st.InBytes
		deltaOut := currentOut - st.OutBytes
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

			if saveTraffic && deltaIn > 0 {
				if err := upsertStatsTraffic(tx, model.Stats{
					DateTime:  now,
					Resource:  "mihomo_inbound",
					Tag:       st.Tag,
					Direction: true,
					Traffic:   deltaIn,
				}); err != nil {
					tx.Rollback()
					return err
				}
			}
			if saveTraffic && deltaOut > 0 {
				if err := upsertStatsTraffic(tx, model.Stats{
					DateTime:  now,
					Resource:  "mihomo_inbound",
					Tag:       st.Tag,
					Direction: false,
					Traffic:   deltaOut,
				}); err != nil {
					tx.Rollback()
					return err
				}
			}
		}

		if err := tx.Model(st).Updates(map[string]interface{}{
			"in_bytes":   currentIn,
			"out_bytes":  currentOut,
			"updated_at": time.Now(),
		}).Error; err != nil {
			tx.Rollback()
			return err
		}
	}

	userOnlines := []string{}
	if len(deltas) > 0 {
		if err := s.ensureClientBindings(tx); err != nil {
			tx.Rollback()
			return err
		}
		var err error
		userOnlines, err = s.writeClientStats(tx, deltas, now, saveTraffic)
		if err != nil {
			tx.Rollback()
			return err
		}
	}

	if err := tx.Commit().Error; err != nil {
		tx.Rollback()
		return err
	}

	setMihomoOnlines(tagsFromSet(inboundOnlineSet), userOnlines)
	return nil
}

func (s *MihomoNftTrafficService) writeClientStats(tx *gorm.DB, deltas []inboundDelta, now int64, saveTraffic bool) ([]string, error) {
	deltaMap := make(map[uint]*inboundDelta, len(deltas))
	for i := range deltas {
		deltaMap[deltas[i].inboundId] = &deltas[i]
	}

	var bindings []model.MihomoClientInboundTrafficState
	if err := tx.Where("active = ?", true).Find(&bindings).Error; err != nil {
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
		if err := tx.Save(b).Error; err != nil {
			return nil, err
		}
	}

	userOnlineSet := make(map[string]struct{}, len(aggs))
	for clientID, agg := range aggs {
		name := strings.TrimSpace(clientNames[clientID])
		if name == "" {
			continue
		}

		if agg.upTotal > 0 {
			if saveTraffic {
				if err := upsertStatsTraffic(tx, model.Stats{
					DateTime:  now,
					Resource:  "mihomo_client",
					Tag:       name,
					Direction: true,
					Traffic:   agg.upTotal,
				}); err != nil {
					return nil, err
				}
			}
			if err := tx.Model(&model.MihomoClient{}).Where("id = ?", clientID).
				UpdateColumn("up", gorm.Expr("up + ?", agg.upTotal)).Error; err != nil {
				return nil, err
			}
		}

		if agg.downTotal > 0 {
			if saveTraffic {
				if err := upsertStatsTraffic(tx, model.Stats{
					DateTime:  now,
					Resource:  "mihomo_client",
					Tag:       name,
					Direction: false,
					Traffic:   agg.downTotal,
				}); err != nil {
					return nil, err
				}
			}
			if err := tx.Model(&model.MihomoClient{}).Where("id = ?", clientID).
				UpdateColumn("down", gorm.Expr("down + ?", agg.downTotal)).Error; err != nil {
				return nil, err
			}
		}

		if agg.upTotal > 0 || agg.downTotal > 0 {
			userOnlineSet[name] = struct{}{}
		}
	}

	return tagsFromSet(userOnlineSet), nil
}

func (s *MihomoNftTrafficService) ensureClientBindings(tx *gorm.DB) error {
	if tx == nil {
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
		if err := s.SyncClientBindings(tx, client.Id, parseMihomoInboundIDs(client.Inbounds)); err != nil {
			return err
		}
	}

	return nil
}

func parseMihomoInboundIDs(raw json.RawMessage) []uint {
	if len(raw) == 0 {
		return []uint{}
	}

	var ids []uint
	if err := json.Unmarshal(raw, &ids); err == nil {
		return deduplicateInboundIDs(ids)
	}

	// Backward-compatible fallback for historical malformed payloads.
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
	if runtime.GOOS != "linux" || !nftSupported() {
		return
	}

	db := database.GetDB()
	var inbounds []model.MihomoInbound
	if err := db.Find(&inbounds).Error; err != nil {
		logger.Warning("failed to load mihomo inbounds for nft init: ", err)
		return
	}

	for _, inbound := range inbounds {
		port := extractPort(inbound.Options)
		if port <= 0 {
			continue
		}

		redirectRange, redirectTCP := resolveMihomoInboundRedirectSpec(&inbound)

		var state model.MihomoInboundRedirectState
		result := db.Where("inbound_id = ?", inbound.Id).First(&state)
		if result.Error == nil {
			if state.InHandle > 0 || state.OutHandle > 0 || state.RedirectHandle > 0 {
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

		tx := db.Begin()
		if tx.Error != nil {
			logger.Warning("failed to open startup transaction for mihomo inbound ", inbound.Tag, ": ", tx.Error)
			continue
		}
		if setupErr := s.SetupInboundRules(tx, inbound.Id, inbound.Tag, port, redirectRange, redirectTCP); setupErr != nil {
			tx.Rollback()
			logger.Warning("failed to setup startup mihomo nft rules for inbound ", inbound.Tag, ": ", setupErr)
			continue
		}
		if commitErr := tx.Commit().Error; commitErr != nil {
			tx.Rollback()
			logger.Warning("failed to commit startup mihomo nft state for inbound ", inbound.Tag, ": ", commitErr)
			continue
		}
	}
}

// CleanupOnShutdown removes mihomo nft rules and clears volatile handles/baselines.
func (s *MihomoNftTrafficService) CleanupOnShutdown() {
	mihomoPortHopRefreshState.mu.Lock()
	mihomoPortHopRefreshState.last = map[uint]time.Time{}
	mihomoPortHopRefreshState.mu.Unlock()

	if runtime.GOOS == "linux" && nftSupported() {
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

// tryRecoverHandles tries to recover missing nft rule handles by comments.
func (s *MihomoNftTrafficService) tryRecoverHandles(tx *gorm.DB, st *model.MihomoInboundRedirectState) {
	if tx == nil || st == nil {
		return
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
		if handle := findHandleByComment(nftChainPrerouting, mihomoNftRuleComments.redirect(st.Tag)); handle > 0 && handle != st.RedirectHandle {
			st.RedirectHandle = handle
			changed = true
			logger.Info("recovered mihomo nft redirect handle for ", st.Tag, ": ", handle)
		}
	}

	if changed {
		_ = tx.Model(st).Updates(map[string]interface{}{
			"in_handle":       st.InHandle,
			"out_handle":      st.OutHandle,
			"redirect_handle": st.RedirectHandle,
			"updated_at":      time.Now(),
		}).Error
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
