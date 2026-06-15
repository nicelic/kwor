package service

import (
	"encoding/json"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/logger"
	"gorm.io/gorm"
)

// MihomoClientRateLimitService manages mihomo per-port nftables bandwidth limiting rules.
//
// Rule model (independent from traffic monitor/port-hop rules):
// - input : meta l4proto {tcp,udp} th dport <port> limit rate over <bytes>/second drop
// - output: meta l4proto {tcp,udp} th sport <port> limit rate over <bytes>/second drop
//
// Effective limit source:
// - enabled clients only
// - bound inbound tags only
// - per inbound port use the MIN speedLimitMbps across all bound clients
// - port hopping ranges are not used; only listen_port is limited
type MihomoClientRateLimitService struct{}

func (s *MihomoClientRateLimitService) IsNftTableReady() bool {
	return nftTableExists()
}

func (s *MihomoClientRateLimitService) InitOnStartup() {
	if runtime.GOOS != "linux" || !nftSupported() {
		return
	}
	if err := s.Reconcile(true); err != nil {
		logger.Warning("init mihomo client rate limit rules failed: ", err)
	}
}

func (s *MihomoClientRateLimitService) EnsureRuleIntegrity() error {
	if runtime.GOOS != "linux" || !nftSupported() {
		return nil
	}
	if !(&MihomoCoreManagerService{}).IsRunning() {
		return nil
	}
	return s.Reconcile(true)
}

// Reconcile recomputes effective limits from DB and syncs nft rules/state.
func (s *MihomoClientRateLimitService) Reconcile(applyRules bool) error {
	db := database.GetDB()
	tx := db.Begin()
	if tx.Error != nil {
		return tx.Error
	}

	desired, desiredTags, err := s.collectDesiredPortLimits(tx)
	if err != nil {
		tx.Rollback()
		return err
	}

	if applyRules {
		err = s.reconcileWithRules(tx, desired, desiredTags)
	} else {
		err = s.reconcileStateOnly(tx, desired, desiredTags)
	}
	if err != nil {
		tx.Rollback()
		return err
	}
	if commitErr := tx.Commit().Error; commitErr != nil {
		tx.Rollback()
		return commitErr
	}
	return nil
}

func (s *MihomoClientRateLimitService) CleanupOnShutdown() {
	if runtime.GOOS == "linux" && nftSupported() {
		if err := deleteRulesByCommentPrefix(mihomoLimitNftRuleComments.prefix); err != nil {
			logger.Warning("failed to cleanup mihomo client rate limit nft rules by prefix: ", err)
		}
	}

	db := database.GetDB()
	if err := db.Session(&gorm.Session{AllowGlobalUpdate: true}).
		Model(&model.MihomoClientPortLimitState{}).
		Updates(map[string]interface{}{
			"in_handle":  0,
			"out_handle": 0,
			"updated_at": time.Now(),
		}).Error; err != nil {
		logger.Warning("failed to reset mihomo client rate limit handles after nft cleanup: ", err)
	}
}

func (s *MihomoClientRateLimitService) reconcileStateOnly(tx *gorm.DB, desired map[int]int, desiredTags map[int]string) error {
	if err := deleteRulesByCommentPrefix(mihomoLimitNftRuleComments.prefix); err != nil {
		return err
	}

	var states []model.MihomoClientPortLimitState
	if err := tx.Find(&states).Error; err != nil {
		return err
	}
	existing := make(map[int]*model.MihomoClientPortLimitState, len(states))
	for i := range states {
		existing[states[i].Port] = &states[i]
	}

	for _, state := range states {
		if _, ok := desired[state.Port]; ok {
			continue
		}
		if err := tx.Delete(&state).Error; err != nil {
			return err
		}
	}

	ports := sortedPortsFromMap(desired)
	for _, port := range ports {
		limit := desired[port]
		tag := desiredTags[port]
		if st, ok := existing[port]; ok {
			if err := tx.Model(st).Updates(map[string]interface{}{
				"tag":        tag,
				"limit_mbps": limit,
				"in_handle":  0,
				"out_handle": 0,
				"updated_at": time.Now(),
			}).Error; err != nil {
				return err
			}
			continue
		}
		now := time.Now()
		next := model.MihomoClientPortLimitState{
			Port:      port,
			Tag:       tag,
			LimitMbps: limit,
			InHandle:  0,
			OutHandle: 0,
			UpdatedAt: now,
			CreatedAt: now,
		}
		if err := tx.Create(&next).Error; err != nil {
			return err
		}
	}

	return nil
}

func (s *MihomoClientRateLimitService) reconcileWithRules(tx *gorm.DB, desired map[int]int, desiredTags map[int]string) error {
	var states []model.MihomoClientPortLimitState
	if err := tx.Find(&states).Error; err != nil {
		return err
	}
	existing := make(map[int]*model.MihomoClientPortLimitState, len(states))
	for i := range states {
		existing[states[i].Port] = &states[i]
	}

	// Remove obsolete states/rules.
	for _, state := range states {
		if _, ok := desired[state.Port]; ok {
			continue
		}
		if err := s.removeRulesFromState(&state); err != nil {
			logger.Warning("failed to remove obsolete mihomo client limit nft rules for port ", state.Port, ": ", err)
		}
		if err := tx.Delete(&state).Error; err != nil {
			return err
		}
	}

	validComments := make(map[string]struct{}, len(desired)*2)
	ports := sortedPortsFromMap(desired)
	for _, port := range ports {
		limit := desired[port]
		tag := desiredTags[port]
		portTag := strconv.Itoa(port)
		validComments[mihomoLimitNftRuleComments.in(portTag)] = struct{}{}
		validComments[mihomoLimitNftRuleComments.out(portTag)] = struct{}{}

		if st, ok := existing[port]; ok {
			if st.LimitMbps != limit {
				if err := s.removeRulesFromState(st); err != nil {
					logger.Warning("failed to remove stale mihomo client limit rules for port ", port, ": ", err)
				}
				inHandle, outHandle, ensureErr := s.ensureLimitRules(port, limit)
				if err := tx.Model(st).Updates(map[string]interface{}{
					"tag":        tag,
					"limit_mbps": limit,
					"in_handle":  inHandle,
					"out_handle": outHandle,
					"updated_at": time.Now(),
				}).Error; err != nil {
					return err
				}
				if ensureErr != nil {
					return ensureErr
				}
				continue
			}

			st.Tag = tag
			if err := s.tryRecoverHandles(tx, st); err != nil {
				logger.Warning("recover mihomo client limit handles failed for port ", st.Port, ": ", err)
			}
			inOk := ruleHandleExists(nftChainIn, st.InHandle)
			outOk := ruleHandleExists(nftChainOut, st.OutHandle)
			if !inOk || !outOk {
				if err := s.removeRulesFromState(st); err != nil {
					logger.Warning("failed to clear broken mihomo client limit rules for port ", st.Port, ": ", err)
				}
				inHandle, outHandle, ensureErr := s.ensureLimitRules(port, limit)
				if err := tx.Model(st).Updates(map[string]interface{}{
					"tag":        tag,
					"in_handle":  inHandle,
					"out_handle": outHandle,
					"updated_at": time.Now(),
				}).Error; err != nil {
					return err
				}
				if ensureErr != nil {
					return ensureErr
				}
				continue
			}

			if err := tx.Model(st).Updates(map[string]interface{}{
				"tag":        tag,
				"updated_at": time.Now(),
			}).Error; err != nil {
				return err
			}
			continue
		}

		inHandle, outHandle, ensureErr := s.ensureLimitRules(port, limit)
		now := time.Now()
		next := model.MihomoClientPortLimitState{
			Port:      port,
			Tag:       tag,
			LimitMbps: limit,
			InHandle:  inHandle,
			OutHandle: outHandle,
			UpdatedAt: now,
			CreatedAt: now,
		}
		if err := tx.Create(&next).Error; err != nil {
			return err
		}
		if ensureErr != nil {
			return ensureErr
		}
	}

	if err := s.cleanupOrphanRules(validComments); err != nil {
		logger.Warning("cleanup orphan mihomo client limit nft rules failed: ", err)
	}
	return nil
}

func (s *MihomoClientRateLimitService) ensureLimitRules(port int, limitMbps int) (int, int, error) {
	rateBytes := mbpsToBytesPerSecond(limitMbps)
	commentIn := mihomoLimitNftRuleComments.in(strconv.Itoa(port))
	commentOut := mihomoLimitNftRuleComments.out(strconv.Itoa(port))

	inHandle, inErr := addPortRateLimitRule(nftChainIn, port, "dport", rateBytes, commentIn)
	if inErr != nil {
		logger.Warning("failed to add mihomo client limit input rule for port ", port, ": ", inErr)
	}
	outHandle, outErr := addPortRateLimitRule(nftChainOut, port, "sport", rateBytes, commentOut)
	if outErr != nil {
		logger.Warning("failed to add mihomo client limit output rule for port ", port, ": ", outErr)
	}

	if inErr != nil {
		return inHandle, outHandle, inErr
	}
	if outErr != nil {
		return inHandle, outHandle, outErr
	}
	return inHandle, outHandle, nil
}

func (s *MihomoClientRateLimitService) removeRulesFromState(state *model.MihomoClientPortLimitState) error {
	if state == nil {
		return nil
	}
	portTag := strconv.Itoa(state.Port)
	var firstErr error

	if state.InHandle > 0 {
		if err := deleteRuleByHandle(nftChainIn, state.InHandle); err != nil {
			firstErr = err
		}
	} else if err := deleteRuleByComment(nftChainIn, mihomoLimitNftRuleComments.in(portTag)); err != nil && firstErr == nil {
		firstErr = err
	}

	if state.OutHandle > 0 {
		if err := deleteRuleByHandle(nftChainOut, state.OutHandle); err != nil && firstErr == nil {
			firstErr = err
		}
	} else if err := deleteRuleByComment(nftChainOut, mihomoLimitNftRuleComments.out(portTag)); err != nil && firstErr == nil {
		firstErr = err
	}

	return firstErr
}

func (s *MihomoClientRateLimitService) cleanupOrphanRules(validComments map[string]struct{}) error {
	if !nftSupported() || !nftTableExists() {
		return nil
	}

	chains := []string{nftChainIn, nftChainOut}
	var firstErr error
	for _, chain := range chains {
		rules, err := listRuleCommentsByPrefix(chain, mihomoLimitNftRuleComments.prefix)
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
			if err := deleteRuleByHandle(chain, rule.handle); err != nil {
				logger.Warning("failed to delete orphan mihomo client limit nft rule ", rule.comment, " handle ", rule.handle, ": ", err)
				if firstErr == nil {
					firstErr = err
				}
			}
		}
	}
	return firstErr
}

func (s *MihomoClientRateLimitService) tryRecoverHandles(tx *gorm.DB, state *model.MihomoClientPortLimitState) error {
	if tx == nil || state == nil {
		return nil
	}
	changed := false
	portTag := strconv.Itoa(state.Port)

	if handle := findHandleByComment(nftChainIn, mihomoLimitNftRuleComments.in(portTag)); handle > 0 && handle != state.InHandle {
		state.InHandle = handle
		changed = true
	}
	if handle := findHandleByComment(nftChainOut, mihomoLimitNftRuleComments.out(portTag)); handle > 0 && handle != state.OutHandle {
		state.OutHandle = handle
		changed = true
	}

	if !changed {
		return nil
	}
	return tx.Model(state).Updates(map[string]interface{}{
		"in_handle":  state.InHandle,
		"out_handle": state.OutHandle,
		"updated_at": time.Now(),
	}).Error
}

func (s *MihomoClientRateLimitService) collectDesiredPortLimits(tx *gorm.DB) (map[int]int, map[int]string, error) {
	type inboundEntry struct {
		Id      uint
		Type    string
		Tag     string
		Options json.RawMessage
	}
	var inbounds []inboundEntry
	if err := tx.Model(&model.MihomoInbound{}).Select("id, type, tag, options").Find(&inbounds).Error; err != nil {
		return nil, nil, err
	}

	type inboundPortInfo struct {
		ports []int
		tag   string
	}
	inboundPorts := make(map[uint]inboundPortInfo, len(inbounds))
	for _, inbound := range inbounds {
		baseInbound := model.MihomoInbound{
			Id:      inbound.Id,
			Type:    inbound.Type,
			Tag:     inbound.Tag,
			Options: inbound.Options,
		}
		ports := expandPortRangesToPorts(collectMihomoInboundLimitRanges(&baseInbound))
		if len(ports) == 0 {
			continue
		}
		inboundPorts[inbound.Id] = inboundPortInfo{
			ports: ports,
			tag:   strings.TrimSpace(inbound.Tag),
		}
	}

	var clients []model.MihomoClient
	if err := tx.Model(&model.MihomoClient{}).
		Select("enable, inbounds, speed_limit_mbps").
		Where("enable = ? AND speed_limit_mbps > 0", true).
		Find(&clients).Error; err != nil {
		return nil, nil, err
	}

	desired := make(map[int]int)
	desiredTags := make(map[int]string)
	for _, client := range clients {
		limit := normalizeLimitMbps(client.SpeedLimitMbps)
		if limit <= 0 {
			continue
		}
		ids := parseClientInboundIDsForLimit(client.Inbounds)
		if len(ids) == 0 {
			continue
		}

		for _, inboundID := range ids {
			info, ok := inboundPorts[inboundID]
			if !ok || len(info.ports) == 0 {
				continue
			}

			for _, port := range info.ports {
				current, exists := desired[port]
				if !exists || limit < current {
					desired[port] = limit
					desiredTags[port] = info.tag
					continue
				}
				if limit == current {
					existingTag := strings.TrimSpace(desiredTags[port])
					if existingTag == "" || (info.tag != "" && info.tag < existingTag) {
						desiredTags[port] = info.tag
					}
				}
			}
		}
	}

	return desired, desiredTags, nil
}
