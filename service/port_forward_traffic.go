package service

import (
	"errors"
	"math"
	"strings"
	"time"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/util/common"
	"gorm.io/gorm"
)

const (
	portForwardTrafficBlockQuota  = "quota"
	portForwardTrafficBlockExpiry = "expiry"

	portForwardTrafficOverviewStateID uint  = 1
	portForwardTrafficGiBBytes        int64 = 1024 * 1024 * 1024
	portForwardTrafficMaxBytes        int64 = 1<<63 - 1
)

// portForwardTrafficRuleRuntime is the presentation and rendering result for
// one rule. It is derived from persisted counters and current configuration,
// rather than from a transient nftables counter reading.
type portForwardTrafficRuleRuntime struct {
	UsedUpBytes   int64
	UsedDownBytes int64
	LastResetAt   int64
	NextResetAt   int64
	LimitReached  bool
	Expired       bool
	BlockReason   string
}

type portForwardTrafficRuntime struct {
	Rules        map[uint]portForwardTrafficRuleRuntime
	OverviewUp   int64
	OverviewDown int64
}

func normalizePortForwardTrafficLimitGiB(value float64) (int64, error) {
	if !isFiniteFloat(value) || value < 0 {
		return 0, common.NewError("traffic limit must not be negative")
	}
	rounded := math.Round(value*100) / 100
	if rounded > 0 && rounded < trafficOverviewMinDisplayGiB {
		rounded = trafficOverviewMinDisplayGiB
	}
	if rounded <= 0 {
		return 0, nil
	}
	maxGiB := float64(portForwardTrafficMaxBytes) / float64(portForwardTrafficGiBBytes)
	if rounded > maxGiB {
		return 0, common.NewError("traffic limit exceeds the maximum supported value")
	}
	return int64(rounded * float64(portForwardTrafficGiBBytes)), nil
}

func portForwardTrafficLimitGiB(bytes int64) float64 {
	if bytes <= 0 {
		return 0
	}
	return math.Round(float64(bytes)*100/float64(portForwardTrafficGiBBytes)) / 100
}

func normalizePortForwardTrafficResetDay(value int) (int, error) {
	if value < 0 || value > 31 {
		return 0, common.NewError("traffic reset day must be between 0 and 31")
	}
	return value, nil
}

func normalizePortForwardTrafficExpiryDate(value string) (string, error) {
	normalized, err := normalizeTrafficOverviewExpiryDate(value)
	if err != nil {
		return "", common.NewError("invalid traffic expiry date: ", err.Error())
	}
	return normalized, nil
}

func portForwardTrafficCounterDelta(current int64, previous int64) int64 {
	if current < 0 {
		current = 0
	}
	if previous < 0 {
		previous = 0
	}
	if current >= previous {
		return current - previous
	}
	// A named counter may have been recreated, or an inline compatibility
	// counter may have been reset by an atomic forwarding redraw. In that case
	// only the new counter value is unseen traffic.
	return current
}

func addPortForwardTrafficBytes(left int64, right int64) int64 {
	if left < 0 {
		left = 0
	}
	if right <= 0 {
		return left
	}
	if left > portForwardTrafficMaxBytes-right {
		return portForwardTrafficMaxBytes
	}
	return left + right
}

func normalizePortForwardTrafficState(state *model.PortForwardRuleTrafficState) bool {
	if state == nil {
		return false
	}
	changed := false
	normalize := func(value *int64) {
		if *value < 0 {
			*value = 0
			changed = true
		}
	}
	normalize(&state.LastUpBytes)
	normalize(&state.LastDownBytes)
	normalize(&state.UsedUpBytes)
	normalize(&state.UsedDownBytes)
	normalize(&state.OverviewLastUpBytes)
	normalize(&state.OverviewLastDownBytes)
	normalize(&state.LastResetAt)
	if state.AppliedResetDay < 0 || state.AppliedResetDay > 31 {
		state.AppliedResetDay = 0
		changed = true
	}
	trimmedTag := strings.TrimSpace(state.ResetPeriodTag)
	if state.ResetPeriodTag != trimmedTag {
		state.ResetPeriodTag = trimmedTag
		changed = true
	}
	return changed
}

// applyPortForwardTrafficMonthlyReset mirrors the traffic-management monthly
// reset behavior: enabling or changing a date keeps current usage, while a
// later calendar boundary resets only this rule's own total.
func applyPortForwardTrafficMonthlyReset(state *model.PortForwardRuleTrafficState, resetDay int, now time.Time) bool {
	if state == nil {
		return false
	}
	resetDay = normalizeResetDay(resetDay)
	if resetDay <= 0 {
		changed := state.AppliedResetDay != 0 || state.ResetPeriodTag != ""
		state.AppliedResetDay = 0
		state.ResetPeriodTag = ""
		return changed
	}

	expectedTag := computePeriodTag(resetDay, now)
	if state.AppliedResetDay != resetDay {
		state.AppliedResetDay = resetDay
		state.ResetPeriodTag = expectedTag
		return true
	}
	if state.ResetPeriodTag == "" {
		state.ResetPeriodTag = expectedTag
		return true
	}
	if state.ResetPeriodTag == expectedTag {
		return false
	}

	state.ResetPeriodTag = expectedTag
	state.UsedUpBytes = 0
	state.UsedDownBytes = 0
	state.LastResetAt = now.Unix()
	return true
}

func buildPortForwardTrafficRuleRuntime(row model.PortForwardRule, state model.PortForwardRuleTrafficState, now time.Time) (portForwardTrafficRuleRuntime, error) {
	result := portForwardTrafficRuleRuntime{
		UsedUpBytes:   maxInt64(state.UsedUpBytes, 0),
		UsedDownBytes: maxInt64(state.UsedDownBytes, 0),
		LastResetAt:   maxInt64(state.LastResetAt, 0),
	}
	if next, ok := nextClientMonthlyResetBoundary(normalizeResetDay(row.TrafficResetDay), now); ok && !next.IsZero() {
		result.NextResetAt = next.Unix()
	}

	_, expiryBoundary, err := parseTrafficOverviewExpiryDate(row.TrafficExpiryDate, now.Location())
	if err != nil {
		return result, err
	}
	result.Expired = isTrafficOverviewExpired(expiryBoundary, now)
	limit := maxInt64(row.TrafficLimitBytes, 0)
	result.LimitReached = limit > 0 && addPortForwardTrafficBytes(result.UsedUpBytes, result.UsedDownBytes) >= limit
	switch {
	case result.Expired:
		result.BlockReason = portForwardTrafficBlockExpiry
	case result.LimitReached:
		result.BlockReason = portForwardTrafficBlockQuota
	}
	return result, nil
}

// syncPortForwardTrafficStateRows consumes one nft snapshot worth of counters
// and persists independent rule and overview cursors. It intentionally does
// not invoke nftables while its SQLite transaction is open.
func syncPortForwardTrafficStateRows(rows []model.PortForwardRule, counterBytes map[string]int64, now time.Time) (portForwardTrafficRuntime, error) {
	result := portForwardTrafficRuntime{Rules: make(map[uint]portForwardTrafficRuleRuntime, len(rows))}
	db := database.GetDB()
	if db == nil {
		return result, common.NewError("database is not ready")
	}
	if now.IsZero() {
		now = PanelNow()
	}
	if counterBytes == nil {
		counterBytes = map[string]int64{}
	}

	err := db.Transaction(func(tx *gorm.DB) error {
		states := make([]model.PortForwardRuleTrafficState, 0, len(rows))
		if err := tx.Find(&states).Error; err != nil {
			return err
		}
		stateByRule := make(map[uint]model.PortForwardRuleTrafficState, len(states))
		for _, state := range states {
			stateByRule[state.RuleId] = state
		}

		overview := model.PortForwardOverviewTrafficState{}
		overviewFound := true
		if err := tx.Where("id = ?", portForwardTrafficOverviewStateID).First(&overview).Error; err != nil {
			if !errors.Is(err, gorm.ErrRecordNotFound) {
				return err
			}
			overviewFound = false
			overview.Id = portForwardTrafficOverviewStateID
		}
		overview.UsedUpBytes = maxInt64(overview.UsedUpBytes, 0)
		overview.UsedDownBytes = maxInt64(overview.UsedDownBytes, 0)

		overviewDeltaUp := int64(0)
		overviewDeltaDown := int64(0)
		changedStates := make(map[uint]bool, len(rows))
		createdStates := make(map[uint]bool, len(rows))

		for _, row := range rows {
			rawUp := maxInt64(counterBytes[portForwardCounterName(row.Id, "up")], 0)
			rawDown := maxInt64(counterBytes[portForwardCounterName(row.Id, "down")], 0)
			state, exists := stateByRule[row.Id]
			if !exists {
				// Existing rules gain their visible nft counter values during the
				// first migration. New rules are inserted with a zero state by
				// UpsertRule before their counters are created.
				state = model.PortForwardRuleTrafficState{
					RuleId:                row.Id,
					LastUpBytes:           rawUp,
					LastDownBytes:         rawDown,
					UsedUpBytes:           rawUp,
					UsedDownBytes:         rawDown,
					OverviewLastUpBytes:   rawUp,
					OverviewLastDownBytes: rawDown,
				}
				createdStates[row.Id] = true
			} else {
				changed := normalizePortForwardTrafficState(&state)
				counterChanged := rawUp != state.LastUpBytes || rawDown != state.LastDownBytes ||
					rawUp != state.OverviewLastUpBytes || rawDown != state.OverviewLastDownBytes
				ruleDeltaUp := portForwardTrafficCounterDelta(rawUp, state.LastUpBytes)
				ruleDeltaDown := portForwardTrafficCounterDelta(rawDown, state.LastDownBytes)
				overviewDeltaUp = addPortForwardTrafficBytes(overviewDeltaUp, portForwardTrafficCounterDelta(rawUp, state.OverviewLastUpBytes))
				overviewDeltaDown = addPortForwardTrafficBytes(overviewDeltaDown, portForwardTrafficCounterDelta(rawDown, state.OverviewLastDownBytes))
				state.UsedUpBytes = addPortForwardTrafficBytes(state.UsedUpBytes, ruleDeltaUp)
				state.UsedDownBytes = addPortForwardTrafficBytes(state.UsedDownBytes, ruleDeltaDown)
				state.LastUpBytes = rawUp
				state.LastDownBytes = rawDown
				state.OverviewLastUpBytes = rawUp
				state.OverviewLastDownBytes = rawDown
				changedStates[row.Id] = changed || counterChanged || ruleDeltaUp > 0 || ruleDeltaDown > 0
			}

			if applyPortForwardTrafficMonthlyReset(&state, row.TrafficResetDay, now) {
				changedStates[row.Id] = true
			}
			stateByRule[row.Id] = state
		}

		if !overviewFound {
			for _, row := range rows {
				state := stateByRule[row.Id]
				overview.UsedUpBytes = addPortForwardTrafficBytes(overview.UsedUpBytes, state.UsedUpBytes)
				overview.UsedDownBytes = addPortForwardTrafficBytes(overview.UsedDownBytes, state.UsedDownBytes)
			}
		} else {
			overview.UsedUpBytes = addPortForwardTrafficBytes(overview.UsedUpBytes, overviewDeltaUp)
			overview.UsedDownBytes = addPortForwardTrafficBytes(overview.UsedDownBytes, overviewDeltaDown)
		}

		for _, row := range rows {
			state := stateByRule[row.Id]
			if createdStates[row.Id] {
				if err := tx.Create(&state).Error; err != nil {
					return err
				}
			} else if changedStates[row.Id] {
				if err := tx.Save(&state).Error; err != nil {
					return err
				}
			}
			runtime, err := buildPortForwardTrafficRuleRuntime(row, state, now)
			if err != nil {
				return err
			}
			result.Rules[row.Id] = runtime
		}

		if overviewFound {
			if overviewDeltaUp > 0 || overviewDeltaDown > 0 {
				if err := tx.Save(&overview).Error; err != nil {
					return err
				}
			}
		} else if err := tx.Create(&overview).Error; err != nil {
			return err
		}
		result.OverviewUp = overview.UsedUpBytes
		result.OverviewDown = overview.UsedDownBytes
		return nil
	})
	return result, err
}

func loadPortForwardTrafficRuntime(rows []model.PortForwardRule, now time.Time) (portForwardTrafficRuntime, error) {
	result := portForwardTrafficRuntime{Rules: make(map[uint]portForwardTrafficRuleRuntime, len(rows))}
	db := database.GetDB()
	if db == nil {
		return result, common.NewError("database is not ready")
	}
	if now.IsZero() {
		now = PanelNow()
	}

	states := make([]model.PortForwardRuleTrafficState, 0, len(rows))
	if err := db.Find(&states).Error; err != nil {
		return result, err
	}
	stateByRule := make(map[uint]model.PortForwardRuleTrafficState, len(states))
	for _, state := range states {
		normalizePortForwardTrafficState(&state)
		stateByRule[state.RuleId] = state
	}
	for _, row := range rows {
		runtime, err := buildPortForwardTrafficRuleRuntime(row, stateByRule[row.Id], now)
		if err != nil {
			return result, err
		}
		result.Rules[row.Id] = runtime
	}

	var overview model.PortForwardOverviewTrafficState
	if err := db.Where("id = ?", portForwardTrafficOverviewStateID).First(&overview).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return result, err
	}
	result.OverviewUp = maxInt64(overview.UsedUpBytes, 0)
	result.OverviewDown = maxInt64(overview.UsedDownBytes, 0)
	return result, nil
}

func portForwardTrafficBlockReasons(rows []model.PortForwardRule, traffic portForwardTrafficRuntime) map[uint]string {
	result := make(map[uint]string)
	for _, row := range rows {
		if !row.Enabled {
			continue
		}
		if reason := strings.TrimSpace(traffic.Rules[row.Id].BlockReason); reason != "" {
			result[row.Id] = reason
		}
	}
	return result
}

func (s *PortForwardService) capturePortForwardTrafficLocked(rows []model.PortForwardRule) (portForwardTrafficRuntime, error) {
	if !portForwardSupported() {
		return syncPortForwardTrafficStateRows(rows, nil, PanelNow())
	}
	snapshot, err := loadPortForwardNftSnapshot()
	if err != nil {
		return portForwardTrafficRuntime{Rules: make(map[uint]portForwardTrafficRuleRuntime)}, err
	}
	portForwardState.nftSnapshot = &snapshot
	return syncPortForwardTrafficStateRows(rows, readPortForwardCounterBytesFromSnapshot(snapshot), PanelNow())
}

func (s *PortForwardService) ResetRuleTraffic(id uint) error {
	portForwardStateMu.Lock()
	defer portForwardStateMu.Unlock()

	db := database.GetDB()
	if db == nil {
		return common.NewError("database is not ready")
	}
	rows, err := loadPortForwardRulesLocked()
	if err != nil {
		return err
	}
	var rule model.PortForwardRule
	found := false
	for _, row := range rows {
		if row.Id == id {
			rule = row
			found = true
			break
		}
	}
	if !found {
		return common.NewError("forwarding rule not found")
	}
	if _, err := s.capturePortForwardTrafficLocked(rows); err != nil {
		return err
	}

	counterBytes := map[string]int64{}
	if portForwardState.nftSnapshot != nil {
		counterBytes = readPortForwardCounterBytesFromSnapshot(*portForwardState.nftSnapshot)
	}
	now := PanelNow()
	var previous model.PortForwardRuleTrafficState
	previousFound := false
	if err := db.Where("rule_id = ?", id).First(&previous).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	} else if err == nil {
		previousFound = true
	}

	err = db.Transaction(func(tx *gorm.DB) error {
		state := previous
		if !previousFound {
			state = model.PortForwardRuleTrafficState{RuleId: id}
		}
		state.LastUpBytes = maxInt64(counterBytes[portForwardCounterName(id, "up")], 0)
		state.LastDownBytes = maxInt64(counterBytes[portForwardCounterName(id, "down")], 0)
		state.OverviewLastUpBytes = state.LastUpBytes
		state.OverviewLastDownBytes = state.LastDownBytes
		state.UsedUpBytes = 0
		state.UsedDownBytes = 0
		state.AppliedResetDay = normalizeResetDay(rule.TrafficResetDay)
		state.ResetPeriodTag = computePeriodTag(state.AppliedResetDay, now)
		state.LastResetAt = now.Unix()
		if previousFound {
			return tx.Save(&state).Error
		}
		return tx.Create(&state).Error
	})
	if err != nil {
		return err
	}
	if err := s.renderLocked(false); err == nil {
		return nil
	} else {
		actionErr := err
		notes := make([]string, 0, 2)
		if previousFound {
			if restoreErr := db.Save(&previous).Error; restoreErr != nil {
				notes = append(notes, "restore previous traffic state failed: "+restoreErr.Error())
			} else {
				notes = append(notes, "restored previous traffic state")
			}
		} else if deleteErr := db.Where("rule_id = ?", id).Delete(&model.PortForwardRuleTrafficState{}).Error; deleteErr != nil {
			notes = append(notes, "remove newly created traffic state failed: "+deleteErr.Error())
		}
		if restoreErr := s.renderLocked(false); restoreErr != nil {
			notes = append(notes, "restore forwarding render failed: "+restoreErr.Error())
		}
		return wrapPortForwardRollbackError(actionErr, notes...)
	}
}

func (s *PortForwardService) ResetOverviewTraffic() error {
	portForwardStateMu.Lock()
	defer portForwardStateMu.Unlock()

	db := database.GetDB()
	if db == nil {
		return common.NewError("database is not ready")
	}
	rows, err := loadPortForwardRulesLocked()
	if err != nil {
		return err
	}
	if _, err := s.capturePortForwardTrafficLocked(rows); err != nil {
		return err
	}

	previous := model.PortForwardOverviewTrafficState{}
	previousFound := false
	if err := db.Where("id = ?", portForwardTrafficOverviewStateID).First(&previous).Error; err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	} else if err == nil {
		previousFound = true
	}
	reset := model.PortForwardOverviewTrafficState{Id: portForwardTrafficOverviewStateID}
	if previousFound {
		reset.CreatedAt = previous.CreatedAt
		err = db.Save(&reset).Error
	} else {
		err = db.Create(&reset).Error
	}
	if err != nil {
		return err
	}
	if err := s.renderLocked(false); err == nil {
		return nil
	} else {
		actionErr := err
		notes := make([]string, 0, 2)
		if previousFound {
			if restoreErr := db.Save(&previous).Error; restoreErr != nil {
				notes = append(notes, "restore previous overview traffic failed: "+restoreErr.Error())
			}
		} else if deleteErr := db.Where("id = ?", portForwardTrafficOverviewStateID).Delete(&model.PortForwardOverviewTrafficState{}).Error; deleteErr != nil {
			notes = append(notes, "remove newly created overview traffic state failed: "+deleteErr.Error())
		}
		if restoreErr := s.renderLocked(false); restoreErr != nil {
			notes = append(notes, "restore forwarding render failed: "+restoreErr.Error())
		}
		return wrapPortForwardRollbackError(actionErr, notes...)
	}
}
