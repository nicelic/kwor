package service

import (
	"testing"
	"time"

	"github.com/alireza0/s-ui/database/model"
)

func TestAutoRenewBatchUsesFixedOneHourCandidateWindow(t *testing.T) {
	setupMihomoSyncTestDB(t, "auto-renew-fixed-window.db")
	resetAutoRenewBatchStateForTest(t)
	now := int64(1_800_000_000)
	window := int64(30 * 24 * time.Hour / time.Second)
	rows := []model.CertificateRecord{
		newAutoRenewBatchTestRecord(1, now+window, now),
		newAutoRenewBatchTestRecord(2, now+window+int64(59*time.Minute/time.Second), now),
		newAutoRenewBatchTestRecord(3, now+window+int64(61*time.Minute/time.Second), now),
	}

	ids := collectAutoRenewBatchCandidates(rows, now)
	assertUintListEqual(t, ids, []uint{1, 2})
	startedAt, endsAt, active := currentCertificateAutoRenewBatchWindow()
	if !active || startedAt != now || endsAt != now+int64(time.Hour/time.Second) {
		t.Fatalf("unexpected batch window: active=%v start=%d end=%d", active, startedAt, endsAt)
	}

	simulateAutoRenewBatchProcessRestartForTest()
	rows = append(rows, newAutoRenewBatchTestRecord(4, now+window+int64(45*time.Minute/time.Second), now))
	ids = collectAutoRenewBatchCandidates(rows, now+int64(30*time.Minute/time.Second))
	assertUintListEqual(t, ids, []uint{1, 2, 4})
	startedAtAfter, endsAtAfter, _ := currentCertificateAutoRenewBatchWindow()
	if startedAtAfter != startedAt || endsAtAfter != endsAt {
		t.Fatalf("candidate addition extended batch: before=%d/%d after=%d/%d", startedAt, endsAt, startedAtAfter, endsAtAfter)
	}
}

func TestCertificateAutoRenewFailureTransitionsToPeriodicAfterThreeRapidRetries(t *testing.T) {
	now := int64(1_800_000_000)
	phase, count, next := nextCertificateAutoRenewFailureState("", 0, now)
	if phase != acmeAutoRenewRetryPhaseRapid || count != 0 || next != now+int64(10*time.Minute/time.Second) {
		t.Fatalf("unexpected initial failure state: %s %d %d", phase, count, next)
	}

	for completed := 0; completed < 2; completed++ {
		now = next
		phase, count, next = nextCertificateAutoRenewFailureState(phase, count, now)
		if phase != acmeAutoRenewRetryPhaseRapid || count != completed+1 || next != now+int64(10*time.Minute/time.Second) {
			t.Fatalf("unexpected rapid retry %d state: %s %d %d", completed+1, phase, count, next)
		}
	}

	now = next
	phase, count, next = nextCertificateAutoRenewFailureState(phase, count, now)
	if phase != acmeAutoRenewRetryPhasePeriodic || count != 3 || next != now+int64(6*time.Hour/time.Second) {
		t.Fatalf("unexpected periodic retry state: %s %d %d", phase, count, next)
	}
}

func TestExpiredCertificateDisablesAutoRenewAndExposesRetryMetadata(t *testing.T) {
	db := setupMihomoSyncTestDB(t, "expired-auto-renew-state.db")
	now := time.Now().Unix()
	row := &model.CertificateRecord{
		SourceType: CertificateSourceSelfSigned,
		SourceRef:  "expired-auto-renew",
		MainDomain: "expired.example.com",
		DomainSet:  `["expired.example.com"]`,
		AutoRenew:  true,
		NotBefore:  now - 7200,
		NotAfter:   now - 1,
		CertPEM:    []byte("test-cert"),
		KeyPEM:     []byte("test-key"),
	}
	if err := db.Create(row).Error; err != nil {
		t.Fatalf("create certificate failed: %v", err)
	}
	if err := disableExpiredCertificateAutoRenew(row.Id, now); err != nil {
		t.Fatalf("disable expired auto-renew failed: %v", err)
	}

	views, err := certificateInventory.List()
	if err != nil {
		t.Fatalf("list certificate metadata failed: %v", err)
	}
	view := findCertificateRecordView(t, views, row.Id)
	if view.AutoRenew || view.AutoRenewRetryPhase != acmeAutoRenewRetryPhaseExpiredDisabled || view.AutoRenewNextRetryAt != 0 {
		t.Fatalf("unexpected expired auto-renew view: %#v", view)
	}
}

func newAutoRenewBatchTestRecord(id uint, notAfter int64, now int64) model.CertificateRecord {
	return model.CertificateRecord{
		Id:         id,
		SourceType: CertificateSourceACME,
		AutoRenew:  true,
		NotBefore:  now - int64(90*24*time.Hour/time.Second),
		NotAfter:   notAfter,
	}
}

func resetAutoRenewBatchStateForTest(t *testing.T) {
	t.Helper()
	reset := func() {
		acmeAutoRenewBatch.mu.Lock()
		defer acmeAutoRenewBatch.mu.Unlock()
		acmeAutoRenewBatch.loaded = true
		acmeAutoRenewBatch.startedAt = 0
		acmeAutoRenewBatch.endsAt = 0
		acmeAutoRenewBatch.candidateIDs = nil
		acmeAutoRenewBatch.completedIDs = nil
	}
	reset()
	t.Cleanup(reset)
}

func simulateAutoRenewBatchProcessRestartForTest() {
	acmeAutoRenewBatch.mu.Lock()
	defer acmeAutoRenewBatch.mu.Unlock()
	acmeAutoRenewBatch.loaded = false
	acmeAutoRenewBatch.startedAt = 0
	acmeAutoRenewBatch.endsAt = 0
	acmeAutoRenewBatch.candidateIDs = nil
	acmeAutoRenewBatch.completedIDs = nil
}

func assertUintListEqual(t *testing.T, got []uint, want []uint) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("unexpected ids: got=%v want=%v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("unexpected ids: got=%v want=%v", got, want)
		}
	}
}
