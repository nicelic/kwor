package service

import (
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

var (
	panelCertificateBalanceMaintenanceMu   sync.Mutex
	panelCertificateBalanceLastCleanupUnix int64 = panelCertificateBalanceMinUpdatedUnix
	panelCertificateBalanceLastDecayUnix   int64 = panelCertificateBalanceMinUpdatedUnix
)

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

func (s *PanelCertificateBalanceService) Reserve(listenerKey string, sniBucket string, candidateRecordIDs []uint) (uint, PanelCertificateBalanceSelection, error) {
	if err := s.Maintain(false); err != nil {
		return 0, PanelCertificateBalanceSelection{}, err
	}
	db := database.GetDB()
	if db == nil {
		return 0, PanelCertificateBalanceSelection{}, nil
	}

	listenerKey = strings.TrimSpace(listenerKey)
	sniBucket = NormalizePanelCertificateBalanceSNIBucket(sniBucket)
	if listenerKey == "" {
		return 0, PanelCertificateBalanceSelection{}, nil
	}

	ids := normalizePanelCertificateBalanceRecordIDs(candidateRecordIDs)
	if len(ids) == 0 {
		return 0, PanelCertificateBalanceSelection{}, nil
	}

	selectedID := uint(0)
	nowUnix := int64(0)
	if err := db.Transaction(func(tx *gorm.DB) error {
		rows := make([]model.PanelCertificateBalanceState, 0, len(ids))
		if err := tx.
			Select("certificate_record_id", "active_conn", "last_selected_at").
			Where("listener_key = ? AND sni_bucket = ? AND certificate_record_id IN ?", listenerKey, sniBucket, ids).
			Find(&rows).Error; err != nil {
			return err
		}

		stats := make(map[uint]model.PanelCertificateBalanceState, len(rows))
		for i := range rows {
			stats[rows[i].CertificateRecordID] = rows[i]
		}

		selectedActive := int64(0)
		selectedLast := int64(0)
		for i, id := range ids {
			row := stats[id]
			if i == 0 || panelBalanceCandidateLess(row.ActiveConn, row.LastSelectedAt, id, selectedActive, selectedLast, selectedID) {
				selectedID = id
				selectedActive = row.ActiveConn
				selectedLast = row.LastSelectedAt
			}
		}
		if selectedID == 0 {
			return nil
		}

		nowUnix = time.Now().Unix()
		insertRow := &model.PanelCertificateBalanceState{
			ListenerKey:         listenerKey,
			SNIBucket:           sniBucket,
			CertificateRecordID: selectedID,
			ActiveConn:          1,
			SelectedTotal:       1,
			LastSelectedAt:      nowUnix,
			UpdatedAtUnix:       nowUnix,
		}
		return tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "listener_key"},
				{Name: "sni_bucket"},
				{Name: "certificate_record_id"},
			},
			DoUpdates: clause.Assignments(map[string]interface{}{
				"active_conn":      gormExpr("active_conn + 1"),
				"selected_total":   gormExpr("selected_total + 1"),
				"last_selected_at": nowUnix,
				"updated_at_unix":  nowUnix,
			}),
		}).Create(insertRow).Error
	}); err != nil {
		return 0, PanelCertificateBalanceSelection{}, err
	}

	if selectedID == 0 {
		return 0, PanelCertificateBalanceSelection{}, nil
	}
	return selectedID, PanelCertificateBalanceSelection{
		ListenerKey:         listenerKey,
		SNIBucket:           sniBucket,
		CertificateRecordID: selectedID,
	}, nil
}

func (s *PanelCertificateBalanceService) Release(selection PanelCertificateBalanceSelection) error {
	db := database.GetDB()
	if db == nil {
		return nil
	}
	listenerKey := strings.TrimSpace(selection.ListenerKey)
	sniBucket := NormalizePanelCertificateBalanceSNIBucket(selection.SNIBucket)
	if listenerKey == "" || selection.CertificateRecordID == 0 {
		return nil
	}
	nowUnix := time.Now().Unix()
	return db.Exec(
		"UPDATE panel_certificate_balance_states SET active_conn = CASE WHEN active_conn > 0 THEN active_conn - 1 ELSE 0 END, updated_at_unix = ? WHERE listener_key = ? AND sni_bucket = ? AND certificate_record_id = ?",
		nowUnix,
		listenerKey,
		sniBucket,
		selection.CertificateRecordID,
	).Error
}

func (s *PanelCertificateBalanceService) Maintain(force bool) error {
	nowUnix := time.Now().Unix()
	panelCertificateBalanceMaintenanceMu.Lock()
	needCleanup := force
	needDecay := force
	if !needCleanup {
		needCleanup = nowUnix-panelCertificateBalanceLastCleanupUnix >= int64(panelCertificateBalanceCleanupGap/time.Second)
	}
	if !needDecay {
		needDecay = nowUnix-panelCertificateBalanceLastDecayUnix >= int64(panelCertificateBalanceDecayGap/time.Second)
	}
	panelCertificateBalanceMaintenanceMu.Unlock()
	if !needCleanup && !needDecay {
		return nil
	}

	db := database.GetDB()
	if db == nil {
		return nil
	}

	if err := db.Model(&model.PanelCertificateBalanceState{}).
		Where("active_conn < 0 OR selected_total < 0 OR last_selected_at < 0 OR updated_at_unix < 0").
		Updates(map[string]interface{}{
			"active_conn":      gormExpr("CASE WHEN active_conn < 0 THEN 0 ELSE active_conn END"),
			"selected_total":   gormExpr("CASE WHEN selected_total < 0 THEN 0 ELSE selected_total END"),
			"last_selected_at": gormExpr("CASE WHEN last_selected_at < 0 THEN 0 ELSE last_selected_at END"),
			"updated_at_unix":  gormExpr("CASE WHEN updated_at_unix < 0 THEN 0 ELSE updated_at_unix END"),
		}).Error; err != nil {
		return err
	}
	if err := db.Exec(
		"DELETE FROM panel_certificate_balance_states WHERE certificate_record_id NOT IN (SELECT id FROM certificate_records)",
	).Error; err != nil {
		return err
	}
	if err := s.cleanupUnassignedRows(); err != nil {
		return err
	}
	if needCleanup {
		staleBefore := nowUnix - int64(panelCertificateBalanceStaleTTL/time.Second)
		if err := db.Where("active_conn = 0 AND (updated_at_unix <= 0 OR updated_at_unix < ?)", staleBefore).
			Delete(&model.PanelCertificateBalanceState{}).Error; err != nil {
			return err
		}
	}
	if needDecay {
		if err := db.Model(&model.PanelCertificateBalanceState{}).
			Where("selected_total > 0").
			Update("selected_total", gormExpr("selected_total / 2")).Error; err != nil {
			return err
		}
	}

	panelCertificateBalanceMaintenanceMu.Lock()
	if needCleanup {
		panelCertificateBalanceLastCleanupUnix = nowUnix
	}
	if needDecay {
		panelCertificateBalanceLastDecayUnix = nowUnix
	}
	panelCertificateBalanceMaintenanceMu.Unlock()
	return nil
}

func (s *PanelCertificateBalanceService) cleanupUnassignedRows() error {
	db := database.GetDB()
	if db == nil {
		return nil
	}
	settingService := &SettingService{}
	panelIDs, err := GetAssignedCertificateRecordIDs(settingService, PanelSelfSignedTargetPanel)
	if err != nil {
		return err
	}
	subIDs, err := GetAssignedCertificateRecordIDs(settingService, PanelSelfSignedTargetSub)
	if err != nil {
		return err
	}

	bound := make(map[uint]struct{}, len(panelIDs)+len(subIDs))
	for _, id := range panelIDs {
		if id > 0 {
			bound[id] = struct{}{}
		}
	}
	for _, id := range subIDs {
		if id > 0 {
			bound[id] = struct{}{}
		}
	}
	if len(bound) == 0 {
		return db.Session(&gormSessionAllowAll).Delete(&model.PanelCertificateBalanceState{}).Error
	}
	ids := make([]uint, 0, len(bound))
	for id := range bound {
		ids = append(ids, id)
	}
	return db.Where("certificate_record_id NOT IN ?", ids).Delete(&model.PanelCertificateBalanceState{}).Error
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
