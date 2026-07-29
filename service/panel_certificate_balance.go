package service

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	panelCertificateBalanceNoSNIBucket    = "_nosni"
	panelCertificateBalanceCleanupGap     = 5 * time.Minute
	panelCertificateBalanceDecayGap       = 24 * time.Hour
	panelCertificateBalanceStaleTTL       = 24 * time.Hour
	panelCertificateBalanceMinUpdatedUnix = int64(0)
)

type PanelCertificateBalanceSelection struct {
	ListenerKey         string
	SNIBucket           string
	CertificateRecordID uint
}

type PanelCertificateBalanceService struct{}

type panelCertificateBalanceRuntimeKey struct {
	listenerKey         string
	sniBucket           string
	certificateRecordID uint
}

type panelCertificateBalanceRuntimeState struct {
	activeConn      int64
	pendingSelected int64
	lastSelectedAt  int64
	updatedAtUnix   int64
}

type panelCertificateBalancePersistRow struct {
	key             panelCertificateBalanceRuntimeKey
	activeConn      int64
	pendingSelected int64
	lastSelectedAt  int64
	updatedAtUnix   int64
}

var (
	panelCertificateBalanceRuntimeMu sync.Mutex
	panelCertificateBalanceRuntime   = make(map[panelCertificateBalanceRuntimeKey]*panelCertificateBalanceRuntimeState)

	panelCertificateBalanceMaintenanceMu   sync.Mutex
	panelCertificateBalanceLastCleanupUnix int64 = panelCertificateBalanceMinUpdatedUnix
	panelCertificateBalanceLastDecayUnix   int64 = panelCertificateBalanceMinUpdatedUnix
)

func init() {
	database.RegisterDBResetHook(resetPanelCertificateBalanceRuntime)
}

func resetPanelCertificateBalanceRuntime() {
	panelCertificateBalanceRuntimeMu.Lock()
	panelCertificateBalanceRuntime = make(map[panelCertificateBalanceRuntimeKey]*panelCertificateBalanceRuntimeState)
	panelCertificateBalanceRuntimeMu.Unlock()

	panelCertificateBalanceMaintenanceMu.Lock()
	panelCertificateBalanceLastCleanupUnix = panelCertificateBalanceMinUpdatedUnix
	panelCertificateBalanceLastDecayUnix = panelCertificateBalanceMinUpdatedUnix
	panelCertificateBalanceMaintenanceMu.Unlock()
}

func PanelCertificateBalanceListenerKey(target PanelSelfSignedTarget, port int) string {
	name := "panel"
	if target == PanelSelfSignedTargetSub {
		name = "sub"
	}
	if port < 0 {
		port = 0
	}
	return "listener|" + name + "|" + strconv.Itoa(port)
}

func NormalizePanelCertificateBalanceSNIBucket(raw string) string {
	value := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(raw), "."))
	if value == "" {
		return panelCertificateBalanceNoSNIBucket
	}
	return value
}

func panelCertificateBalanceCandidateBucket(ids []uint) string {
	sorted := append([]uint(nil), ids...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	parts := make([]string, 0, len(sorted))
	for _, id := range sorted {
		parts = append(parts, strconv.FormatUint(uint64(id), 10))
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, ",")))
	return "certset:" + hex.EncodeToString(sum[:12])
}

// Reserve selects a certificate entirely in memory. TLS handshakes must never
// wait for the single SQLite connection; the low-frequency maintenance job
// persists diagnostic counters independently.
func (s *PanelCertificateBalanceService) Reserve(listenerKey string, _ string, candidateRecordIDs []uint) (uint, PanelCertificateBalanceSelection, error) {
	listenerKey = strings.TrimSpace(listenerKey)
	ids := normalizePanelCertificateBalanceRecordIDs(candidateRecordIDs)
	if listenerKey == "" || len(ids) == 0 {
		return 0, PanelCertificateBalanceSelection{}, nil
	}

	bucket := panelCertificateBalanceCandidateBucket(ids)
	nowUnix := time.Now().Unix()
	selectedID := uint(0)
	selectedActive := int64(0)
	selectedLast := int64(0)

	panelCertificateBalanceRuntimeMu.Lock()
	for i, id := range ids {
		key := panelCertificateBalanceRuntimeKey{
			listenerKey:         listenerKey,
			sniBucket:           bucket,
			certificateRecordID: id,
		}
		state := panelCertificateBalanceRuntime[key]
		activeConn := int64(0)
		lastSelectedAt := int64(0)
		if state != nil {
			activeConn = state.activeConn
			lastSelectedAt = state.lastSelectedAt
		}
		if i == 0 || panelBalanceCandidateLess(activeConn, lastSelectedAt, id, selectedActive, selectedLast, selectedID) {
			selectedID = id
			selectedActive = activeConn
			selectedLast = lastSelectedAt
		}
	}
	selectedKey := panelCertificateBalanceRuntimeKey{
		listenerKey:         listenerKey,
		sniBucket:           bucket,
		certificateRecordID: selectedID,
	}
	state := panelCertificateBalanceRuntime[selectedKey]
	if state == nil {
		state = &panelCertificateBalanceRuntimeState{}
		panelCertificateBalanceRuntime[selectedKey] = state
	}
	state.activeConn++
	state.pendingSelected++
	state.lastSelectedAt = nowUnix
	state.updatedAtUnix = nowUnix
	panelCertificateBalanceRuntimeMu.Unlock()

	return selectedID, PanelCertificateBalanceSelection{
		ListenerKey:         listenerKey,
		SNIBucket:           bucket,
		CertificateRecordID: selectedID,
	}, nil
}

func (s *PanelCertificateBalanceService) Release(selection PanelCertificateBalanceSelection) error {
	listenerKey := strings.TrimSpace(selection.ListenerKey)
	bucket := strings.TrimSpace(selection.SNIBucket)
	if listenerKey == "" || bucket == "" || selection.CertificateRecordID == 0 {
		return nil
	}

	key := panelCertificateBalanceRuntimeKey{
		listenerKey:         listenerKey,
		sniBucket:           bucket,
		certificateRecordID: selection.CertificateRecordID,
	}
	nowUnix := time.Now().Unix()
	panelCertificateBalanceRuntimeMu.Lock()
	if state := panelCertificateBalanceRuntime[key]; state != nil {
		if state.activeConn > 0 {
			state.activeConn--
		}
		state.updatedAtUnix = nowUnix
	}
	panelCertificateBalanceRuntimeMu.Unlock()
	return nil
}

func snapshotPanelCertificateBalanceRuntime() []panelCertificateBalancePersistRow {
	panelCertificateBalanceRuntimeMu.Lock()
	defer panelCertificateBalanceRuntimeMu.Unlock()

	rows := make([]panelCertificateBalancePersistRow, 0, len(panelCertificateBalanceRuntime))
	for key, state := range panelCertificateBalanceRuntime {
		if state == nil {
			continue
		}
		rows = append(rows, panelCertificateBalancePersistRow{
			key:             key,
			activeConn:      state.activeConn,
			pendingSelected: state.pendingSelected,
			lastSelectedAt:  state.lastSelectedAt,
			updatedAtUnix:   state.updatedAtUnix,
		})
	}
	return rows
}

func acknowledgePanelCertificateBalancePersist(rows []panelCertificateBalancePersistRow, nowUnix int64) {
	panelCertificateBalanceRuntimeMu.Lock()
	defer panelCertificateBalanceRuntimeMu.Unlock()

	staleBefore := nowUnix - int64(panelCertificateBalanceStaleTTL/time.Second)
	for _, row := range rows {
		state := panelCertificateBalanceRuntime[row.key]
		if state == nil {
			continue
		}
		state.pendingSelected -= row.pendingSelected
		if state.pendingSelected < 0 {
			state.pendingSelected = 0
		}
	}
	for key, state := range panelCertificateBalanceRuntime {
		if state != nil && state.activeConn == 0 && state.pendingSelected == 0 && state.updatedAtUnix < staleBefore {
			delete(panelCertificateBalanceRuntime, key)
		}
	}
}

func assignedPanelCertificateBalanceRecordIDs() ([]uint, error) {
	settingService := &SettingService{}
	panelIDs, err := GetAssignedCertificateRecordIDs(settingService, PanelSelfSignedTargetPanel)
	if err != nil {
		return nil, err
	}
	subIDs, err := GetAssignedCertificateRecordIDs(settingService, PanelSelfSignedTargetSub)
	if err != nil {
		return nil, err
	}
	return normalizePanelCertificateBalanceRecordIDs(append(panelIDs, subIDs...)), nil
}

func discardUnassignedPanelCertificateBalanceRuntime(boundIDs []uint) {
	bound := make(map[uint]struct{}, len(boundIDs))
	for _, id := range boundIDs {
		bound[id] = struct{}{}
	}
	panelCertificateBalanceRuntimeMu.Lock()
	for key := range panelCertificateBalanceRuntime {
		if _, exists := bound[key.certificateRecordID]; !exists {
			delete(panelCertificateBalanceRuntime, key)
		}
	}
	panelCertificateBalanceRuntimeMu.Unlock()
}

func (s *PanelCertificateBalanceService) Maintain(force bool) error {
	panelCertificateBalanceMaintenanceMu.Lock()
	defer panelCertificateBalanceMaintenanceMu.Unlock()

	nowUnix := time.Now().Unix()
	needCleanup := force || nowUnix-panelCertificateBalanceLastCleanupUnix >= int64(panelCertificateBalanceCleanupGap/time.Second)
	needDecay := force || nowUnix-panelCertificateBalanceLastDecayUnix >= int64(panelCertificateBalanceDecayGap/time.Second)
	if !needCleanup && !needDecay {
		return nil
	}

	db := database.GetDB()
	if db == nil {
		return nil
	}
	boundIDs, err := assignedPanelCertificateBalanceRecordIDs()
	if err != nil {
		return err
	}
	discardUnassignedPanelCertificateBalanceRuntime(boundIDs)
	runtimeRows := snapshotPanelCertificateBalanceRuntime()

	err = db.Transaction(func(tx *gorm.DB) error {
		// The runtime map is authoritative. Reset leaked counts left by older
		// versions before publishing the current process snapshot.
		if err := tx.Model(&model.PanelCertificateBalanceState{}).
			Where("active_conn <> 0").
			Update("active_conn", 0).Error; err != nil {
			return err
		}
		if err := tx.Exec(
			"DELETE FROM panel_certificate_balance_states WHERE certificate_record_id NOT IN (SELECT id FROM certificate_records)",
		).Error; err != nil {
			return err
		}
		if err := tx.Where("sni_bucket NOT LIKE ?", "certset:%").Delete(&model.PanelCertificateBalanceState{}).Error; err != nil {
			return err
		}
		if len(boundIDs) == 0 {
			if err := tx.Session(&gormSessionAllowAll).Delete(&model.PanelCertificateBalanceState{}).Error; err != nil {
				return err
			}
		} else if err := tx.Where("certificate_record_id NOT IN ?", boundIDs).Delete(&model.PanelCertificateBalanceState{}).Error; err != nil {
			return err
		}

		for _, runtimeRow := range runtimeRows {
			row := &model.PanelCertificateBalanceState{
				ListenerKey:         runtimeRow.key.listenerKey,
				SNIBucket:           runtimeRow.key.sniBucket,
				CertificateRecordID: runtimeRow.key.certificateRecordID,
				ActiveConn:          runtimeRow.activeConn,
				SelectedTotal:       runtimeRow.pendingSelected,
				LastSelectedAt:      runtimeRow.lastSelectedAt,
				UpdatedAtUnix:       runtimeRow.updatedAtUnix,
			}
			if err := tx.Clauses(clause.OnConflict{
				Columns: []clause.Column{
					{Name: "listener_key"},
					{Name: "sni_bucket"},
					{Name: "certificate_record_id"},
				},
				DoUpdates: clause.Assignments(map[string]interface{}{
					"active_conn":      runtimeRow.activeConn,
					"selected_total":   clause.Expr{SQL: "selected_total + ?", Vars: []interface{}{runtimeRow.pendingSelected}},
					"last_selected_at": runtimeRow.lastSelectedAt,
					"updated_at_unix":  runtimeRow.updatedAtUnix,
				}),
			}).Create(row).Error; err != nil {
				return err
			}
		}

		if needCleanup {
			staleBefore := nowUnix - int64(panelCertificateBalanceStaleTTL/time.Second)
			if err := tx.Where("active_conn = 0 AND (updated_at_unix <= 0 OR updated_at_unix < ?)", staleBefore).
				Delete(&model.PanelCertificateBalanceState{}).Error; err != nil {
				return err
			}
		}
		if needDecay {
			if err := tx.Model(&model.PanelCertificateBalanceState{}).
				Where("selected_total > 0").
				Update("selected_total", gormExpr("selected_total / 2")).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return err
	}

	acknowledgePanelCertificateBalancePersist(runtimeRows, nowUnix)
	if needCleanup {
		panelCertificateBalanceLastCleanupUnix = nowUnix
	}
	if needDecay {
		panelCertificateBalanceLastDecayUnix = nowUnix
	}
	return nil
}

func panelBalanceCandidateLess(activeA int64, lastA int64, idA uint, activeB int64, lastB int64, idB uint) bool {
	if activeA != activeB {
		return activeA < activeB
	}
	if lastA != lastB {
		return lastA < lastB
	}
	return idA < idB
}

func normalizePanelCertificateBalanceRecordIDs(ids []uint) []uint {
	if len(ids) == 0 {
		return nil
	}
	out := make([]uint, 0, len(ids))
	seen := make(map[uint]struct{}, len(ids))
	for _, id := range ids {
		if id == 0 {
			continue
		}
		if _, exists := seen[id]; exists {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}
