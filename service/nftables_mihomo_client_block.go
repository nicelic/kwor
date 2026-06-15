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

// MihomoClientPortBlockService manages mihomo port-level nftables access blocks.
// It blocks all effective inbound-facing ports for depleted users,
// including listen ports and redirect ranges (HY1/HY2/Mieru).
type MihomoClientPortBlockService struct{}

func (s *MihomoClientPortBlockService) IsNftTableReady() bool {
	return nftTableExists()
}

func (s *MihomoClientPortBlockService) InitOnStartup() {
	if runtime.GOOS != "linux" || !nftSupported() {
		return
	}
	if err := s.Reconcile(true); err != nil {
		logger.Warning("init mihomo client block nft rules failed: ", err)
	}
}

func (s *MihomoClientPortBlockService) EnsureRuleIntegrity() error {
	if runtime.GOOS != "linux" || !nftSupported() {
		return nil
	}
	if !(&MihomoCoreManagerService{}).IsRunning() {
		return nil
	}
	return s.Reconcile(true)
}

func (s *MihomoClientPortBlockService) Reconcile(applyRules bool) error {
	db := database.GetDB()
	tx := db.Begin()
	if tx.Error != nil {
		return tx.Error
	}

	desired, err := s.collectDesiredBlockedPorts(tx)
	if err != nil {
		tx.Rollback()
		return err
	}

	if applyRules {
		err = s.reconcileWithRules(tx, desired)
	} else {
		err = s.reconcileStateOnly(tx, desired)
	}
	if err != nil {
		tx.Rollback()
		return err
	}
	if commitErr := tx.Commit().Error; commitErr != nil {
		tx.Rollback()
		return commitErr
	}

	if applyRules && len(desired) > 0 {
		if err := flushConntrackTable(); err != nil {
			logger.Warning("flush conntrack after mihomo client block apply failed: ", err)
		}
	}
	return nil
}

func (s *MihomoClientPortBlockService) CleanupOnShutdown() {
	if runtime.GOOS == "linux" && nftSupported() {
		if err := deleteRulesByCommentPrefix(mihomoBlockNftRuleComments.prefix); err != nil {
			logger.Warning("failed to cleanup mihomo client block nft rules by prefix: ", err)
		}
	}

	db := database.GetDB()
	if err := db.Session(&gorm.Session{AllowGlobalUpdate: true}).
		Model(&model.MihomoClientPortBlockState{}).
		Updates(map[string]interface{}{
			"in_handle":  0,
			"out_handle": 0,
			"updated_at": time.Now(),
		}).Error; err != nil {
		logger.Warning("failed to reset mihomo client block handles after nft cleanup: ", err)
	}
}

func (s *MihomoClientPortBlockService) reconcileStateOnly(tx *gorm.DB, desired map[int]clientBlockedPortSpec) error {
	if err := deleteRulesByCommentPrefix(mihomoBlockNftRuleComments.prefix); err != nil {
		return err
	}

	var states []model.MihomoClientPortBlockState
	if err := tx.Find(&states).Error; err != nil {
		return err
	}
	existing := make(map[int]*model.MihomoClientPortBlockState, len(states))
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

	ports := sortedBlockedPortsFromSpecs(desired)
	for _, port := range ports {
		spec := desired[port]
		encodedRanges := encodePortRangesJSON(spec.Ranges)
		if st, ok := existing[port]; ok {
			if err := tx.Model(st).Updates(map[string]interface{}{
				"tag":         spec.Tag,
				"port_ranges": encodedRanges,
				"in_handle":   0,
				"out_handle":  0,
				"updated_at":  time.Now(),
			}).Error; err != nil {
				return err
			}
			continue
		}
		now := time.Now()
		next := model.MihomoClientPortBlockState{
			Port:       port,
			Tag:        spec.Tag,
			PortRanges: encodedRanges,
			InHandle:   0,
			OutHandle:  0,
			UpdatedAt:  now,
			CreatedAt:  now,
		}
		if err := tx.Create(&next).Error; err != nil {
			return err
		}
	}

	return nil
}

func (s *MihomoClientPortBlockService) reconcileWithRules(tx *gorm.DB, desired map[int]clientBlockedPortSpec) error {
	var states []model.MihomoClientPortBlockState
	if err := tx.Find(&states).Error; err != nil {
		return err
	}
	existing := make(map[int]*model.MihomoClientPortBlockState, len(states))
	for i := range states {
		existing[states[i].Port] = &states[i]
	}

	for _, state := range states {
		if _, ok := desired[state.Port]; ok {
			continue
		}
		if err := s.removeRulesFromState(&state); err != nil {
			logger.Warning("failed to remove obsolete mihomo client block nft rules for port ", state.Port, ": ", err)
		}
		if err := tx.Delete(&state).Error; err != nil {
			return err
		}
	}

	validComments := make(map[string]struct{}, len(desired)*2)
	ports := sortedBlockedPortsFromSpecs(desired)
	for _, port := range ports {
		spec := desired[port]
		portTag := strconv.Itoa(port)
		validComments[mihomoBlockNftRuleComments.in(portTag)] = struct{}{}
		validComments[mihomoBlockNftRuleComments.out(portTag)] = struct{}{}
		encodedRanges := encodePortRangesJSON(spec.Ranges)

		if st, ok := existing[port]; ok {
			st.Tag = spec.Tag
			desiredRanges := normalizeNftPortRanges(spec.Ranges)
			currentRanges := decodePortRangesJSON(st.PortRanges)
			rangesChanged := !portRangeSlicesEqual(currentRanges, desiredRanges)

			if err := s.tryRecoverHandles(tx, st); err != nil {
				logger.Warning("recover mihomo client block handles failed for port ", st.Port, ": ", err)
			}
			inOk := ruleHandleExists(nftChainIn, st.InHandle)
			outOk := ruleHandleExists(nftChainOut, st.OutHandle)
			if rangesChanged || !inOk || !outOk {
				if err := s.removeRulesFromState(st); err != nil {
					logger.Warning("failed to clear broken mihomo client block rules for port ", st.Port, ": ", err)
				}
				inHandle, outHandle, ensureErr := s.ensureBlockRules(port, desiredRanges)
				if err := tx.Model(st).Updates(map[string]interface{}{
					"tag":         spec.Tag,
					"port_ranges": encodedRanges,
					"in_handle":   inHandle,
					"out_handle":  outHandle,
					"updated_at":  time.Now(),
				}).Error; err != nil {
					return err
				}
				if ensureErr != nil {
					return ensureErr
				}
				continue
			}

			if err := tx.Model(st).Updates(map[string]interface{}{
				"tag":         spec.Tag,
				"port_ranges": encodedRanges,
				"updated_at":  time.Now(),
			}).Error; err != nil {
				return err
			}
			continue
		}

		inHandle, outHandle, ensureErr := s.ensureBlockRules(port, spec.Ranges)
		now := time.Now()
		next := model.MihomoClientPortBlockState{
			Port:       port,
			Tag:        spec.Tag,
			PortRanges: encodedRanges,
			InHandle:   inHandle,
			OutHandle:  outHandle,
			UpdatedAt:  now,
			CreatedAt:  now,
		}
		if err := tx.Create(&next).Error; err != nil {
			return err
		}
		if ensureErr != nil {
			return ensureErr
		}
	}

	if err := s.cleanupOrphanRules(validComments); err != nil {
		logger.Warning("cleanup orphan mihomo client block nft rules failed: ", err)
	}
	return nil
}

func (s *MihomoClientPortBlockService) ensureBlockRules(port int, ranges []portRange) (int, int, error) {
	commentIn := mihomoBlockNftRuleComments.in(strconv.Itoa(port))
	commentOut := mihomoBlockNftRuleComments.out(strconv.Itoa(port))
	normalized := normalizeNftPortRanges(ranges)

	inHandle, inErr := addPortRangeDropRule(nftChainIn, "dport", normalized, commentIn)
	if inErr != nil {
		logger.Warning("failed to add mihomo client block input rule for port ", port, ": ", inErr)
	}
	outHandle, outErr := addPortRangeDropRule(nftChainOut, "sport", normalized, commentOut)
	if outErr != nil {
		logger.Warning("failed to add mihomo client block output rule for port ", port, ": ", outErr)
	}

	if inErr != nil {
		return inHandle, outHandle, inErr
	}
	if outErr != nil {
		return inHandle, outHandle, outErr
	}
	return inHandle, outHandle, nil
}

func (s *MihomoClientPortBlockService) removeRulesFromState(state *model.MihomoClientPortBlockState) error {
	if state == nil {
		return nil
	}
	portTag := strconv.Itoa(state.Port)
	var firstErr error

	if state.InHandle > 0 {
		if err := deleteRuleByHandle(nftChainIn, state.InHandle); err != nil {
			firstErr = err
		}
	} else if err := deleteRuleByComment(nftChainIn, mihomoBlockNftRuleComments.in(portTag)); err != nil && firstErr == nil {
		firstErr = err
	}

	if state.OutHandle > 0 {
		if err := deleteRuleByHandle(nftChainOut, state.OutHandle); err != nil && firstErr == nil {
			firstErr = err
		}
	} else if err := deleteRuleByComment(nftChainOut, mihomoBlockNftRuleComments.out(portTag)); err != nil && firstErr == nil {
		firstErr = err
	}

	return firstErr
}

func (s *MihomoClientPortBlockService) cleanupOrphanRules(validComments map[string]struct{}) error {
	if !nftSupported() || !nftTableExists() {
		return nil
	}

	chains := []string{nftChainIn, nftChainOut}
	var firstErr error
	for _, chain := range chains {
		rules, err := listRuleCommentsByPrefix(chain, mihomoBlockNftRuleComments.prefix)
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
				logger.Warning("failed to delete orphan mihomo client block nft rule ", rule.comment, " handle ", rule.handle, ": ", err)
				if firstErr == nil {
					firstErr = err
				}
			}
		}
	}
	return firstErr
}

func (s *MihomoClientPortBlockService) tryRecoverHandles(tx *gorm.DB, state *model.MihomoClientPortBlockState) error {
	if tx == nil || state == nil {
		return nil
	}
	changed := false
	portTag := strconv.Itoa(state.Port)

	if handle := findHandleByComment(nftChainIn, mihomoBlockNftRuleComments.in(portTag)); handle > 0 && handle != state.InHandle {
		state.InHandle = handle
		changed = true
	}
	if handle := findHandleByComment(nftChainOut, mihomoBlockNftRuleComments.out(portTag)); handle > 0 && handle != state.OutHandle {
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

func (s *MihomoClientPortBlockService) collectDesiredBlockedPorts(tx *gorm.DB) (map[int]clientBlockedPortSpec, error) {
	type inboundEntry struct {
		Id      uint
		Tag     string
		Type    string
		Options json.RawMessage
		OutJson json.RawMessage
	}
	var inbounds []inboundEntry
	if err := tx.Model(&model.MihomoInbound{}).Select("id, tag, type, options, out_json").Find(&inbounds).Error; err != nil {
		return nil, err
	}

	type inboundPortInfo struct {
		port   int
		tag    string
		ranges []portRange
	}
	inboundPorts := make(map[uint]inboundPortInfo, len(inbounds))
	for _, inbound := range inbounds {
		mockInbound := model.MihomoInbound{
			Id:      inbound.Id,
			Tag:     inbound.Tag,
			Type:    inbound.Type,
			Options: inbound.Options,
			OutJson: inbound.OutJson,
		}
		ranges := collectMihomoInboundBlockRanges(&mockInbound)
		listenPort := extractPort(inbound.Options)
		if listenPort <= 0 || len(ranges) == 0 {
			continue
		}
		inboundPorts[inbound.Id] = inboundPortInfo{
			port:   listenPort,
			tag:    strings.TrimSpace(inbound.Tag),
			ranges: ranges,
		}
	}

	nowUnix := time.Now().Unix()
	var clients []model.MihomoClient
	if err := tx.Model(&model.MihomoClient{}).
		Select("enable, inbounds, volume, expiry, up, down").
		Where("enable = ?", true).
		Find(&clients).Error; err != nil {
		return nil, err
	}

	desired := make(map[int]clientBlockedPortSpec)
	for _, client := range clients {
		used := client.Up + client.Down
		evaluation := evaluateClientAccess(client.Enable, used, client.Volume, client.Expiry, nowUnix)
		if !evaluation.Blocked {
			continue
		}

		ids := parseClientInboundIDsForLimit(client.Inbounds)
		for _, inboundID := range ids {
			info, ok := inboundPorts[inboundID]
			if !ok || info.port <= 0 {
				continue
			}
			spec, exists := desired[info.port]
			if !exists {
				spec = clientBlockedPortSpec{
					Tag:    info.tag,
					Ranges: append([]portRange{}, info.ranges...),
				}
			} else {
				spec.Ranges = append(spec.Ranges, info.ranges...)
				existingTag := strings.TrimSpace(spec.Tag)
				if existingTag == "" || (info.tag != "" && info.tag < existingTag) {
					spec.Tag = info.tag
				}
			}
			spec.Ranges = normalizeNftPortRanges(spec.Ranges)
			desired[info.port] = spec
		}
	}

	return desired, nil
}
