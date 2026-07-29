package service

import (
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
)

func TestPanelCertificateBalanceReservePrefersLeastActive(t *testing.T) {
	openPanelCertificateBalanceTestDB(t)
	svc := &PanelCertificateBalanceService{}
	certA := uint(11)
	certB := uint(22)

	listenerKey := PanelCertificateBalanceListenerKey(PanelSelfSignedTargetPanel, 8888)
	bucket := NormalizePanelCertificateBalanceSNIBucket("api.example.com")
	firstID, _, err := svc.Reserve(listenerKey, bucket, []uint{certA, certB})
	if err != nil {
		t.Fatalf("reserve panel certificate failed: %v", err)
	}
	if firstID != certA {
		t.Fatalf("first selected cert mismatch: got=%d want=%d", firstID, certA)
	}
	secondID, secondSelection, err := svc.Reserve(listenerKey, bucket, []uint{certA, certB})
	if err != nil {
		t.Fatalf("reserve second panel certificate failed: %v", err)
	}
	if secondID != certB {
		t.Fatalf("second selected cert mismatch: got=%d want=%d", secondID, certB)
	}
	if err := svc.Release(secondSelection); err != nil {
		t.Fatalf("release second selection failed: %v", err)
	}
	thirdID, _, err := svc.Reserve(listenerKey, bucket, []uint{certA, certB})
	if err != nil {
		t.Fatalf("reserve third panel certificate failed: %v", err)
	}
	if thirdID != certB {
		t.Fatalf("least-active selected cert mismatch: got=%d want=%d", thirdID, certB)
	}
}

func TestPanelCertificateBalanceReleaseAndCleanup(t *testing.T) {
	openPanelCertificateBalanceTestDB(t)
	svc := &PanelCertificateBalanceService{}
	settingService := &SettingService{}

	certID := createPanelBalanceTestCertificateRecord(t, "panel-balance-cleanup")
	if err := SetAssignedCertificateRecordIDs(settingService, PanelSelfSignedTargetPanel, []uint{certID}); err != nil {
		t.Fatalf("assign panel certificate id failed: %v", err)
	}

	secondCertID := createPanelBalanceTestCertificateRecord(t, "panel-balance-cleanup-second")
	if err := SetAssignedCertificateRecordIDs(settingService, PanelSelfSignedTargetPanel, []uint{certID, secondCertID}); err != nil {
		t.Fatalf("assign second panel certificate id failed: %v", err)
	}

	listenerKey := PanelCertificateBalanceListenerKey(PanelSelfSignedTargetPanel, 9443)
	_, selection, err := svc.Reserve(listenerKey, "", []uint{certID, secondCertID})
	if err != nil {
		t.Fatalf("reserve panel certificate selection failed: %v", err)
	}
	if err := svc.Maintain(true); err != nil {
		t.Fatalf("persist panel certificate selection failed: %v", err)
	}

	if err := svc.Release(selection); err != nil {
		t.Fatalf("release panel certificate selection failed: %v", err)
	}
	if err := svc.Maintain(true); err != nil {
		t.Fatalf("persist released panel certificate selection failed: %v", err)
	}

	row := &model.PanelCertificateBalanceState{}
	if err := database.GetDB().
		Where("listener_key = ? AND sni_bucket = ? AND certificate_record_id = ?", listenerKey, selection.SNIBucket, selection.CertificateRecordID).
		First(row).Error; err != nil {
		t.Fatalf("read released balance row failed: %v", err)
	}
	if row.ActiveConn != 0 {
		t.Fatalf("active_conn after release mismatch: got=%d want=%d", row.ActiveConn, 0)
	}

	if err := SetAssignedCertificateRecordIDs(settingService, PanelSelfSignedTargetPanel, []uint{}); err != nil {
		t.Fatalf("clear assigned panel certificate ids failed: %v", err)
	}
	if err := svc.Maintain(true); err != nil {
		t.Fatalf("maintain panel certificate balance failed: %v", err)
	}

	count := int64(0)
	if err := database.GetDB().Model(&model.PanelCertificateBalanceState{}).Count(&count).Error; err != nil {
		t.Fatalf("count panel balance rows failed: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected cleanup to remove all rows, got=%d", count)
	}
}

func TestPanelCertificateBalanceReserveConcurrentStaysBalanced(t *testing.T) {
	openPanelCertificateBalanceTestDB(t)
	svc := &PanelCertificateBalanceService{}
	settingService := &SettingService{}

	certA := createPanelBalanceTestCertificateRecord(t, "panel-concurrent-a")
	certB := createPanelBalanceTestCertificateRecord(t, "panel-concurrent-b")
	if err := SetAssignedCertificateRecordIDs(settingService, PanelSelfSignedTargetPanel, []uint{certA, certB}); err != nil {
		t.Fatalf("assign panel certificate ids failed: %v", err)
	}

	listenerKey := PanelCertificateBalanceListenerKey(PanelSelfSignedTargetPanel, 9443)
	bucket := NormalizePanelCertificateBalanceSNIBucket("api.example.com")
	const runs = 40

	errCh := make(chan error, runs)
	var wg sync.WaitGroup
	for i := 0; i < runs; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			selectedID, selection, err := svc.Reserve(listenerKey, bucket, []uint{certA, certB})
			if err != nil {
				errCh <- err
				return
			}
			if selectedID == 0 || selection.CertificateRecordID == 0 {
				errCh <- testingError("unexpected empty selection")
				return
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		if err != nil {
			t.Fatalf("reserve concurrent failed: %v", err)
		}
	}
	if err := svc.Maintain(true); err != nil {
		t.Fatalf("persist concurrent balance state failed: %v", err)
	}

	rows := make([]model.PanelCertificateBalanceState, 0)
	if err := database.GetDB().
		Where("listener_key = ? AND sni_bucket = ?", listenerKey, panelCertificateBalanceCandidateBucket([]uint{certA, certB})).
		Order("certificate_record_id asc").
		Find(&rows).Error; err != nil {
		t.Fatalf("query panel balance rows failed: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected two cert rows, got %d (%#v)", len(rows), rows)
	}
	total := rows[0].ActiveConn + rows[1].ActiveConn
	if total != runs {
		t.Fatalf("active_conn total mismatch: got=%d want=%d", total, runs)
	}
	diff := rows[0].ActiveConn - rows[1].ActiveConn
	if diff < 0 {
		diff = -diff
	}
	if diff > 1 {
		t.Fatalf("expected near-even distribution, got diff=%d rows=%#v", diff, rows)
	}
}

func TestPanelCertificateBalanceMaintainClearsLeakedActiveStaleRows(t *testing.T) {
	openPanelCertificateBalanceTestDB(t)
	svc := &PanelCertificateBalanceService{}
	settingService := &SettingService{}

	certID := createPanelBalanceTestCertificateRecord(t, "panel-stale-active")
	if err := SetAssignedCertificateRecordIDs(settingService, PanelSelfSignedTargetPanel, []uint{certID}); err != nil {
		t.Fatalf("assign panel certificate id failed: %v", err)
	}

	listenerKey := PanelCertificateBalanceListenerKey(PanelSelfSignedTargetPanel, 9443)
	staleUnix := time.Now().Unix() - int64((panelCertificateBalanceStaleTTL/time.Second)+10)
	rows := []model.PanelCertificateBalanceState{
		{
			ListenerKey:         listenerKey,
			SNIBucket:           NormalizePanelCertificateBalanceSNIBucket("stale-inactive.example.com"),
			CertificateRecordID: certID,
			ActiveConn:          0,
			SelectedTotal:       6,
			LastSelectedAt:      staleUnix,
			UpdatedAtUnix:       staleUnix,
		},
		{
			ListenerKey:         listenerKey,
			SNIBucket:           NormalizePanelCertificateBalanceSNIBucket("stale-active.example.com"),
			CertificateRecordID: certID,
			ActiveConn:          2,
			SelectedTotal:       6,
			LastSelectedAt:      staleUnix,
			UpdatedAtUnix:       staleUnix,
		},
	}
	if err := database.GetDB().Create(&rows).Error; err != nil {
		t.Fatalf("seed panel stale rows failed: %v", err)
	}

	if err := svc.Maintain(true); err != nil {
		t.Fatalf("maintain panel balance failed: %v", err)
	}

	remaining := make([]model.PanelCertificateBalanceState, 0)
	if err := database.GetDB().
		Where("listener_key = ?", listenerKey).
		Find(&remaining).Error; err != nil {
		t.Fatalf("query panel remaining rows failed: %v", err)
	}
	if len(remaining) != 0 {
		t.Fatalf("expected leaked stale rows to be removed, got %d (%#v)", len(remaining), remaining)
	}
}

func openPanelCertificateBalanceTestDB(t *testing.T) {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "panel-certificate-balance.db")
	if err := database.InitDB(dbPath); err != nil {
		t.Fatalf("init db failed: %v", err)
	}
	sqlDB, err := database.GetDB().DB()
	if err == nil && sqlDB != nil {
		t.Cleanup(func() {
			_ = sqlDB.Close()
		})
	}
}

func createPanelBalanceTestCertificateRecord(t *testing.T, suffix string) uint {
	t.Helper()

	now := time.Now().Unix()
	row := &model.CertificateRecord{
		SourceType:   "imported",
		SourceRef:    "test-panel-balance:" + suffix,
		MainDomain:   "example.com",
		DomainSet:    `["example.com"]`,
		CertPEM:      []byte("cert-" + suffix),
		KeyPEM:       []byte("key-" + suffix),
		FullchainPEM: []byte("fullchain-" + suffix),
		NotBefore:    now - 3600,
		NotAfter:     now + 3600,
	}
	if err := database.GetDB().Create(row).Error; err != nil {
		t.Fatalf("create certificate record failed: %v", err)
	}
	return row.Id
}

type testingError string

func (e testingError) Error() string { return string(e) }
