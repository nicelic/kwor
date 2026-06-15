package service

import (
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/logger"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	reverseProxyCertBalanceNoSNIBucket    = "_nosni"
	reverseProxyCertBalanceCleanupGap     = 5 * time.Minute
	reverseProxyCertBalanceDecayGap       = 24 * time.Hour
	reverseProxyCertBalanceStaleTTL       = 24 * time.Hour
	reverseProxyCertBalanceMinUpdatedUnix = int64(0)
)

type ReverseProxyCertificateBalanceDiagnostic struct {
	CertificateRecordID uint   `json:"certificateRecordId"`
	SNIBucket           string `json:"sniBucket"`
	ActiveConn          int64  `json:"activeConn"`
	SelectedTotal       int64  `json:"selectedTotal"`
	LastSelectedAt      int64  `json:"lastSelectedAt"`
	UpdatedAtUnix       int64  `json:"updatedAtUnix"`
}

var (
	reverseProxyCertBalanceMaintenanceMu   sync.Mutex
	reverseProxyCertBalanceLastCleanupUnix int64 = reverseProxyCertBalanceMinUpdatedUnix
	reverseProxyCertBalanceLastDecayUnix   int64 = reverseProxyCertBalanceMinUpdatedUnix
)

func reverseProxyNormalizeSNIBucket(raw string) string {
	value := reverseProxyNormalizeServerName(raw)
	if value == "" {
		return reverseProxyCertBalanceNoSNIBucket
	}
	return value
}

func (g *reverseProxyListenerGroup) selectBalancedCertificate(candidates []*reverseProxyRuleCertificateBinding, sniBucket string) (*reverseProxyRuleCertificateBinding, reverseProxyCertificateSelection, error) {
	if g == nil {
		return nil, reverseProxyCertificateSelection{}, nil
	}
	filtered := reverseProxyUniqueCertificateBindings(candidates)
	if len(filtered) == 0 {
		return nil, reverseProxyCertificateSelection{}, nil
	}
	bucket := reverseProxyNormalizeSNIBucket(sniBucket)
	if g.service == nil {
		selected := reverseProxyFallbackCertificateBinding(filtered)
		if selected == nil {
			return nil, reverseProxyCertificateSelection{}, nil
		}
		return selected, reverseProxyCertificateSelection{}, nil
	}
	selected, selection, err := g.service.reserveCertificateBalanceSelection(g.key, bucket, filtered)
	if err != nil {
		logger.Warning("reverse proxy certificate balancing fallback due to db error: ", err)
		selected = reverseProxyFallbackCertificateBinding(filtered)
		if selected == nil {
			return nil, reverseProxyCertificateSelection{}, err
		}
		return selected, reverseProxyCertificateSelection{}, nil
	}
	return selected, selection, nil
}

func reverseProxyFallbackCertificateBinding(candidates []*reverseProxyRuleCertificateBinding) *reverseProxyRuleCertificateBinding {
	for _, candidate := range candidates {
		if reverseProxyCertificateBindingUsable(candidate, time.Now()) {
			return candidate
		}
	}
	return nil
}

func reverseProxyUniqueCertificateBindings(candidates []*reverseProxyRuleCertificateBinding) []*reverseProxyRuleCertificateBinding {
	out := make([]*reverseProxyRuleCertificateBinding, 0, len(candidates))
	seen := make(map[uint]struct{}, len(candidates))
	now := time.Now()
	for _, item := range candidates {
		if !reverseProxyCertificateBindingUsable(item, now) || item.CertificateRecordID == 0 {
			continue
		}
		if _, exists := seen[item.CertificateRecordID]; exists {
			continue
		}
		seen[item.CertificateRecordID] = struct{}{}
		out = append(out, item)
	}
	return out
}

func reverseProxyCertificateBindingUsable(binding *reverseProxyRuleCertificateBinding, now time.Time) bool {
	if binding == nil || binding.Certificate == nil || binding.Leaf == nil || binding.Leaf.Leaf == nil {
		return false
	}
	if now.IsZero() {
		now = time.Now()
	}
	notAfter := binding.Leaf.NotAfter
	if notAfter.IsZero() {
		notAfter = binding.Leaf.Leaf.NotAfter
	}
	return notAfter.IsZero() || now.Before(notAfter)
}

type reverseProxyBalanceCandidateState struct {
	Binding        *reverseProxyRuleCertificateBinding
	ActiveConn     int64
	LastSelectedAt int64
}

func reverseProxyBalanceCandidateLess(a reverseProxyBalanceCandidateState, b reverseProxyBalanceCandidateState) bool {
	if a.ActiveConn != b.ActiveConn {
		return a.ActiveConn < b.ActiveConn
	}
	if a.LastSelectedAt != b.LastSelectedAt {
		return a.LastSelectedAt < b.LastSelectedAt
	}
	return a.Binding.CertificateRecordID < b.Binding.CertificateRecordID
}

func (s *ReverseProxyService) reserveCertificateBalanceSelection(listenerKey string, sniBucket string, candidates []*reverseProxyRuleCertificateBinding) (*reverseProxyRuleCertificateBinding, reverseProxyCertificateSelection, error) {
	db := database.GetDB()
	if db == nil {
		return nil, reverseProxyCertificateSelection{}, nil
	}
	if len(candidates) == 0 {
		return nil, reverseProxyCertificateSelection{}, nil
	}
	listenerKey = strings.TrimSpace(listenerKey)
	sniBucket = reverseProxyNormalizeSNIBucket(sniBucket)
	if listenerKey == "" {
		return nil, reverseProxyCertificateSelection{}, nil
	}

	certIDs := make([]uint, 0, len(candidates))
	byID := make(map[uint]*reverseProxyRuleCertificateBinding, len(candidates))
	now := time.Now()
	for _, item := range candidates {
		if !reverseProxyCertificateBindingUsable(item, now) || item.CertificateRecordID == 0 {
			continue
		}
		certIDs = append(certIDs, item.CertificateRecordID)
		byID[item.CertificateRecordID] = item
	}
	if len(certIDs) == 0 {
		return nil, reverseProxyCertificateSelection{}, nil
	}

	var selectedBinding *reverseProxyRuleCertificateBinding
	var selectedRecordID uint
	var nowUnix int64
	if err := db.Transaction(func(tx *gorm.DB) error {
		rows := make([]model.ReverseProxyCertificateBalanceState, 0)
		if err := tx.
			Select("certificate_record_id", "active_conn", "last_selected_at").
			Where("listener_key = ? AND sni_bucket = ? AND certificate_record_id IN ?", listenerKey, sniBucket, certIDs).
			Find(&rows).Error; err != nil {
			return err
		}

		stats := make(map[uint]model.ReverseProxyCertificateBalanceState, len(rows))
		for i := range rows {
			stats[rows[i].CertificateRecordID] = rows[i]
		}

		candidateStates := make([]reverseProxyBalanceCandidateState, 0, len(certIDs))
		for _, certID := range certIDs {
			binding := byID[certID]
			if binding == nil {
				continue
			}
			row := stats[certID]
			candidateStates = append(candidateStates, reverseProxyBalanceCandidateState{
				Binding:        binding,
				ActiveConn:     row.ActiveConn,
				LastSelectedAt: row.LastSelectedAt,
			})
		}
		if len(candidateStates) == 0 {
			return nil
		}

		selected := candidateStates[0]
		for i := 1; i < len(candidateStates); i++ {
			if reverseProxyBalanceCandidateLess(candidateStates[i], selected) {
				selected = candidateStates[i]
			}
		}
		selectedBinding = selected.Binding
		selectedRecordID = selected.Binding.CertificateRecordID
		nowUnix = time.Now().Unix()

		insertRow := &model.ReverseProxyCertificateBalanceState{
			ListenerKey:         listenerKey,
			SNIBucket:           sniBucket,
			CertificateRecordID: selectedRecordID,
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
		return nil, reverseProxyCertificateSelection{}, err
	}

	if selectedBinding == nil || selectedRecordID == 0 {
		return nil, reverseProxyCertificateSelection{}, nil
	}
	return selectedBinding, reverseProxyCertificateSelection{
		ListenerKey:         listenerKey,
		SNIBucket:           sniBucket,
		CertificateRecordID: selectedRecordID,
	}, nil
}

func (s *ReverseProxyService) releaseCertificateBalanceSelection(selection reverseProxyCertificateSelection) error {
	db := database.GetDB()
	if db == nil {
		return nil
	}
	listenerKey := strings.TrimSpace(selection.ListenerKey)
	sniBucket := reverseProxyNormalizeSNIBucket(selection.SNIBucket)
	if listenerKey == "" || selection.CertificateRecordID == 0 {
		return nil
	}
	nowUnix := time.Now().Unix()
	return db.Exec(
		"UPDATE reverse_proxy_certificate_balance_states SET active_conn = CASE WHEN active_conn > 0 THEN active_conn - 1 ELSE 0 END, updated_at_unix = ? WHERE listener_key = ? AND sni_bucket = ? AND certificate_record_id = ?",
		nowUnix,
		listenerKey,
		sniBucket,
		selection.CertificateRecordID,
	).Error
}

func (s *ReverseProxyService) MaintainCertificateBalance(force bool) error {
	nowUnix := time.Now().Unix()
	reverseProxyCertBalanceMaintenanceMu.Lock()
	needCleanup := force
	needDecay := force
	if !needCleanup {
		needCleanup = nowUnix-reverseProxyCertBalanceLastCleanupUnix >= int64(reverseProxyCertBalanceCleanupGap/time.Second)
	}
	if !needDecay {
		needDecay = nowUnix-reverseProxyCertBalanceLastDecayUnix >= int64(reverseProxyCertBalanceDecayGap/time.Second)
	}
	reverseProxyCertBalanceMaintenanceMu.Unlock()
	if !needCleanup && !needDecay {
		return nil
	}

	db := database.GetDB()
	if db == nil {
		return nil
	}

	if err := db.Model(&model.ReverseProxyCertificateBalanceState{}).
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
		"DELETE FROM reverse_proxy_certificate_balance_states WHERE certificate_record_id NOT IN (SELECT id FROM certificate_records)",
	).Error; err != nil {
		return err
	}
	if err := s.cleanupUnboundCertificateBalanceRows(); err != nil {
		return err
	}
	if needCleanup {
		staleBefore := nowUnix - int64(reverseProxyCertBalanceStaleTTL/time.Second)
		if err := db.Where("active_conn = 0 AND (updated_at_unix <= 0 OR updated_at_unix < ?)", staleBefore).
			Delete(&model.ReverseProxyCertificateBalanceState{}).Error; err != nil {
			return err
		}
	}
	if needDecay {
		if err := db.Model(&model.ReverseProxyCertificateBalanceState{}).
			Where("selected_total > 0").
			Update("selected_total", gormExpr("selected_total / 2")).Error; err != nil {
			return err
		}
	}

	reverseProxyCertBalanceMaintenanceMu.Lock()
	if needCleanup {
		reverseProxyCertBalanceLastCleanupUnix = nowUnix
	}
	if needDecay {
		reverseProxyCertBalanceLastDecayUnix = nowUnix
	}
	reverseProxyCertBalanceMaintenanceMu.Unlock()
	return nil
}

func (s *ReverseProxyService) cleanupUnboundCertificateBalanceRows() error {
	db := database.GetDB()
	if db == nil {
		return nil
	}
	rows := make([]model.ReverseProxyRule, 0)
	if err := db.Select("certificate_record_id", "certificate_record_list").Find(&rows).Error; err != nil {
		return err
	}
	bound := make(map[uint]struct{})
	for i := range rows {
		for _, certID := range reverseProxyRuleCertificateIDs(&rows[i]) {
			bound[certID] = struct{}{}
		}
	}
	if len(bound) == 0 {
		return db.Session(&gormSessionAllowAll).Delete(&model.ReverseProxyCertificateBalanceState{}).Error
	}
	ids := make([]uint, 0, len(bound))
	for certID := range bound {
		ids = append(ids, certID)
	}
	return db.Where("certificate_record_id NOT IN ?", ids).Delete(&model.ReverseProxyCertificateBalanceState{}).Error
}

func (s *ReverseProxyService) loadRuleCertificateBalanceDiagnostics(rows []model.ReverseProxyRule) (map[uint][]ReverseProxyCertificateBalanceDiagnostic, error) {
	result := make(map[uint][]ReverseProxyCertificateBalanceDiagnostic)
	if len(rows) == 0 {
		return result, nil
	}
	db := database.GetDB()
	if db == nil {
		return result, nil
	}

	listenerKeys := make(map[string]struct{})
	byRule := make(map[uint]map[uint]struct{})
	certIDs := make(map[uint]struct{})
	for i := range rows {
		row := rows[i]
		if !row.Enabled || !strings.EqualFold(strings.TrimSpace(row.ListenProtocol), reverseProxyProtocolHTTPS) {
			continue
		}
		ids := reverseProxyRuleCertificateIDs(&row)
		if len(ids) == 0 {
			continue
		}
		key := reverseProxyListenerKey(row.ListenProtocol, row.ListenPort)
		listenerKeys[key] = struct{}{}
		if byRule[row.Id] == nil {
			byRule[row.Id] = make(map[uint]struct{})
		}
		for _, certID := range ids {
			certIDs[certID] = struct{}{}
			byRule[row.Id][certID] = struct{}{}
		}
	}
	if len(listenerKeys) == 0 || len(certIDs) == 0 {
		return result, nil
	}

	listenerKeyList := make([]string, 0, len(listenerKeys))
	for key := range listenerKeys {
		listenerKeyList = append(listenerKeyList, key)
	}
	certIDList := make([]uint, 0, len(certIDs))
	for certID := range certIDs {
		certIDList = append(certIDList, certID)
	}

	balanceRows := make([]model.ReverseProxyCertificateBalanceState, 0)
	if err := db.
		Select("listener_key", "sni_bucket", "certificate_record_id", "active_conn", "selected_total", "last_selected_at", "updated_at_unix").
		Where("listener_key IN ? AND certificate_record_id IN ?", listenerKeyList, certIDList).
		Order("certificate_record_id ASC, active_conn DESC, selected_total DESC, last_selected_at DESC").
		Find(&balanceRows).Error; err != nil {
		return nil, err
	}

	listenerCertRows := make(map[string][]model.ReverseProxyCertificateBalanceState)
	for i := range balanceRows {
		row := balanceRows[i]
		key := strings.TrimSpace(row.ListenerKey) + "|" + strings.TrimSpace(strconvFormatUint(row.CertificateRecordID))
		listenerCertRows[key] = append(listenerCertRows[key], row)
	}

	for i := range rows {
		row := rows[i]
		ids := byRule[row.Id]
		if len(ids) == 0 {
			continue
		}
		key := reverseProxyListenerKey(row.ListenProtocol, row.ListenPort)
		diags := make([]ReverseProxyCertificateBalanceDiagnostic, 0)
		for certID := range ids {
			rowsForCert := listenerCertRows[strings.TrimSpace(key)+"|"+strings.TrimSpace(strconvFormatUint(certID))]
			for _, item := range rowsForCert {
				diags = append(diags, ReverseProxyCertificateBalanceDiagnostic{
					CertificateRecordID: item.CertificateRecordID,
					SNIBucket:           item.SNIBucket,
					ActiveConn:          item.ActiveConn,
					SelectedTotal:       item.SelectedTotal,
					LastSelectedAt:      item.LastSelectedAt,
					UpdatedAtUnix:       item.UpdatedAtUnix,
				})
			}
		}
		result[row.Id] = diags
	}
	return result, nil
}

func gormExpr(sql string) clause.Expr {
	return clause.Expr{SQL: sql}
}

func strconvFormatUint(v uint) string {
	return strings.TrimSpace(strconv.FormatUint(uint64(v), 10))
}
