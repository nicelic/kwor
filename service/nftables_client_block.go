package service

import (
	"encoding/json"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/logger"
	"gorm.io/gorm"
)

// ClientPortBlockService manages nftables drop rules for depleted or expired client ports.
// It blocks both traffic directions for all effective inbound-facing ports
// (listen port + port-hop/redirect ranges) bound to depleted users.
type ClientPortBlockService struct{}

func (s *ClientPortBlockService) IsNftTableReady() bool {
	return nftTableExists()
}

func (s *ClientPortBlockService) InitOnStartup() {
	if runtime.GOOS != "linux" || !nftSupported() {
		return
	}
	if err := s.Reconcile(true); err != nil {
		logger.Warning("init client block nft rules failed: ", err)
	}
}

func (s *ClientPortBlockService) EnsureRuleIntegrity() error {
	if runtime.GOOS != "linux" || !nftSupported() {
		return nil
	}
	if !(&CoreManagerService{}).IsRunning() {
		return nil
	}
	return s.Reconcile(true)
}

func (s *ClientPortBlockService) Reconcile(applyRules bool) error {
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
			logger.Warning("flush conntrack after client block apply failed: ", err)
		}
	}
	return nil
}

func (s *ClientPortBlockService) CleanupOnShutdown() {
	if runtime.GOOS == "linux" && nftSupported() {
		if err := deleteRulesByCommentPrefix(singboxBlockNftRuleComments.prefix); err != nil {
			logger.Warning("failed to cleanup client block nft rules by prefix: ", err)
		}
	}

	db := database.GetDB()
	if err := db.Session(&gorm.Session{AllowGlobalUpdate: true}).
		Model(&model.ClientPortBlockState{}).
		Updates(map[string]interface{}{
			"in_handle":  0,
			"out_handle": 0,
			"updated_at": time.Now(),
		}).Error; err != nil {
		logger.Warning("failed to reset client block handles after nft cleanup: ", err)
	}
}

func (s *ClientPortBlockService) reconcileStateOnly(tx *gorm.DB, desired map[int]clientBlockedPortSpec) error {
	if err := deleteRulesByCommentPrefix(singboxBlockNftRuleComments.prefix); err != nil {
		return err
	}

	var states []model.ClientPortBlockState
	if err := tx.Find(&states).Error; err != nil {
		return err
	}
	existing := make(map[int]*model.ClientPortBlockState, len(states))
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
		next := model.ClientPortBlockState{
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

func (s *ClientPortBlockService) reconcileWithRules(tx *gorm.DB, desired map[int]clientBlockedPortSpec) error {
	var states []model.ClientPortBlockState
	if err := tx.Find(&states).Error; err != nil {
		return err
	}
	existing := make(map[int]*model.ClientPortBlockState, len(states))
	for i := range states {
		existing[states[i].Port] = &states[i]
	}

	for _, state := range states {
		if _, ok := desired[state.Port]; ok {
			continue
		}
		if err := s.removeRulesFromState(&state); err != nil {
			logger.Warning("failed to remove obsolete client block nft rules for port ", state.Port, ": ", err)
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
		validComments[singboxBlockNftRuleComments.in(portTag)] = struct{}{}
		validComments[singboxBlockNftRuleComments.out(portTag)] = struct{}{}
		encodedRanges := encodePortRangesJSON(spec.Ranges)

		if st, ok := existing[port]; ok {
			st.Tag = spec.Tag
			desiredRanges := normalizeNftPortRanges(spec.Ranges)
			currentRanges := decodePortRangesJSON(st.PortRanges)
			rangesChanged := !portRangeSlicesEqual(currentRanges, desiredRanges)

			if err := s.tryRecoverHandles(tx, st); err != nil {
				logger.Warning("recover client block handles failed for port ", st.Port, ": ", err)
			}
			inOk := ruleHandleExists(nftChainIn, st.InHandle)
			outOk := ruleHandleExists(nftChainOut, st.OutHandle)
			if rangesChanged || !inOk || !outOk {
				if err := s.removeRulesFromState(st); err != nil {
					logger.Warning("failed to clear broken client block rules for port ", st.Port, ": ", err)
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
		next := model.ClientPortBlockState{
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
		logger.Warning("cleanup orphan client block nft rules failed: ", err)
	}
	return nil
}

func (s *ClientPortBlockService) ensureBlockRules(port int, ranges []portRange) (int, int, error) {
	commentIn := singboxBlockNftRuleComments.in(strconv.Itoa(port))
	commentOut := singboxBlockNftRuleComments.out(strconv.Itoa(port))
	normalized := normalizeNftPortRanges(ranges)

	inHandle, inErr := addPortRangeDropRule(nftChainIn, "dport", normalized, commentIn)
	if inErr != nil {
		logger.Warning("failed to add client block input rule for port ", port, ": ", inErr)
	}
	outHandle, outErr := addPortRangeDropRule(nftChainOut, "sport", normalized, commentOut)
	if outErr != nil {
		logger.Warning("failed to add client block output rule for port ", port, ": ", outErr)
	}

	if inErr != nil {
		return inHandle, outHandle, inErr
	}
	if outErr != nil {
		return inHandle, outHandle, outErr
	}
	return inHandle, outHandle, nil
}

func (s *ClientPortBlockService) removeRulesFromState(state *model.ClientPortBlockState) error {
	if state == nil {
		return nil
	}
	portTag := strconv.Itoa(state.Port)
	var firstErr error

	if state.InHandle > 0 {
		if err := deleteRuleByHandle(nftChainIn, state.InHandle); err != nil {
			firstErr = err
		}
	} else if err := deleteRuleByComment(nftChainIn, singboxBlockNftRuleComments.in(portTag)); err != nil && firstErr == nil {
		firstErr = err
	}

	if state.OutHandle > 0 {
		if err := deleteRuleByHandle(nftChainOut, state.OutHandle); err != nil && firstErr == nil {
			firstErr = err
		}
	} else if err := deleteRuleByComment(nftChainOut, singboxBlockNftRuleComments.out(portTag)); err != nil && firstErr == nil {
		firstErr = err
	}

	return firstErr
}

func (s *ClientPortBlockService) cleanupOrphanRules(validComments map[string]struct{}) error {
	if !nftSupported() || !nftTableExists() {
		return nil
	}

	chains := []string{nftChainIn, nftChainOut}
	var firstErr error
	for _, chain := range chains {
		rules, err := listRuleCommentsByPrefix(chain, singboxBlockNftRuleComments.prefix)
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
				logger.Warning("failed to delete orphan client block nft rule ", rule.comment, " handle ", rule.handle, ": ", err)
				if firstErr == nil {
					firstErr = err
				}
			}
		}
	}
	return firstErr
}

func (s *ClientPortBlockService) tryRecoverHandles(tx *gorm.DB, state *model.ClientPortBlockState) error {
	if tx == nil || state == nil {
		return nil
	}
	changed := false
	portTag := strconv.Itoa(state.Port)

	if handle := findHandleByComment(nftChainIn, singboxBlockNftRuleComments.in(portTag)); handle > 0 && handle != state.InHandle {
		state.InHandle = handle
		changed = true
	}
	if handle := findHandleByComment(nftChainOut, singboxBlockNftRuleComments.out(portTag)); handle > 0 && handle != state.OutHandle {
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

type clientBlockedPortSpec struct {
	Tag    string
	Ranges []portRange
}

func (s *ClientPortBlockService) collectDesiredBlockedPorts(tx *gorm.DB) (map[int]clientBlockedPortSpec, error) {
	type inboundEntry struct {
		Id      uint
		Tag     string
		Type    string
		Options json.RawMessage
		OutJson json.RawMessage
	}
	var inbounds []inboundEntry
	if err := tx.Model(&model.Inbound{}).Select("id, tag, type, options, out_json").Find(&inbounds).Error; err != nil {
		return nil, err
	}

	type inboundPortInfo struct {
		port   int
		tag    string
		ranges []portRange
	}
	inboundPorts := make(map[uint]inboundPortInfo, len(inbounds))
	for _, inbound := range inbounds {
		mockInbound := model.Inbound{
			Id:      inbound.Id,
			Tag:     inbound.Tag,
			Type:    inbound.Type,
			Options: inbound.Options,
			OutJson: inbound.OutJson,
		}
		ranges := collectInboundBlockRanges(&mockInbound)
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
	var clients []model.Client
	if err := tx.Model(&model.Client{}).
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

func sortedBlockedPortsFromSpecs(values map[int]clientBlockedPortSpec) []int {
	if len(values) == 0 {
		return []int{}
	}
	ports := make([]int, 0, len(values))
	for port := range values {
		ports = append(ports, port)
	}
	sort.Ints(ports)
	return ports
}

func encodePortRangesJSON(ranges []portRange) string {
	normalized := normalizeNftPortRanges(ranges)
	if len(normalized) == 0 {
		return ""
	}

	type persistedPortRange struct {
		Start int `json:"start"`
		End   int `json:"end"`
	}
	payload := make([]persistedPortRange, 0, len(normalized))
	for _, item := range normalized {
		payload = append(payload, persistedPortRange{
			Start: item.start,
			End:   item.end,
		})
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		return ""
	}
	return string(raw)
}

func decodePortRangesJSON(raw string) []portRange {
	text := strings.TrimSpace(raw)
	if text == "" {
		return nil
	}

	type persistedPortRange struct {
		Start int `json:"start"`
		End   int `json:"end"`
	}
	var payload []persistedPortRange
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		return nil
	}

	ranges := make([]portRange, 0, len(payload))
	for _, item := range payload {
		ranges = append(ranges, portRange{
			start: item.Start,
			end:   item.End,
		})
	}
	return normalizeNftPortRanges(ranges)
}

func portRangeSlicesEqual(left []portRange, right []portRange) bool {
	left = normalizeNftPortRanges(left)
	right = normalizeNftPortRanges(right)
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index].start != right[index].start || left[index].end != right[index].end {
			return false
		}
	}
	return true
}
