package service

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/alireza0/s-ui/database/model"
)

type runtimeDrainCall struct {
	target      PanelSelfSignedTarget
	fingerprint string
	gracePeriod time.Duration
}

type mockPanelTLSRuntimeApplier struct {
	mu           sync.Mutex
	applyTargets []PanelSelfSignedTarget
	drainCalls   []runtimeDrainCall
}

func (m *mockPanelTLSRuntimeApplier) ApplyPanelTLSSettings(target PanelSelfSignedTarget) error {
	m.mu.Lock()
	m.applyTargets = append(m.applyTargets, target)
	m.mu.Unlock()
	return nil
}

func (m *mockPanelTLSRuntimeApplier) DrainPanelTLSConnectionsByFingerprint(target PanelSelfSignedTarget, fingerprint string, gracePeriod time.Duration) error {
	m.mu.Lock()
	m.drainCalls = append(m.drainCalls, runtimeDrainCall{
		target:      target,
		fingerprint: strings.TrimSpace(fingerprint),
		gracePeriod: gracePeriod,
	})
	m.mu.Unlock()
	return nil
}

func (m *mockPanelTLSRuntimeApplier) snapshot() ([]PanelSelfSignedTarget, []runtimeDrainCall) {
	m.mu.Lock()
	defer m.mu.Unlock()
	applyTargets := append([]PanelSelfSignedTarget(nil), m.applyTargets...)
	drains := append([]runtimeDrainCall(nil), m.drainCalls...)
	return applyTargets, drains
}

func TestAcmeApplyAppendsAndMovesToFront(t *testing.T) {
	settingService := initPanelSQLiteSettingTestDB(t)
	_ = settingService

	mockRuntime := &mockPanelTLSRuntimeApplier{}
	RegisterPanelTLSRuntimeApplier(mockRuntime)
	defer RegisterPanelTLSRuntimeApplier(nil)

	first := upsertAcmeApplyTestCertificateRecord(t, "apply-first", "fp-apply-first")
	second := upsertAcmeApplyTestCertificateRecord(t, "apply-second", "fp-apply-second")

	svc := &AcmeService{}
	if _, err := svc.Apply(AcmeApplyPayload{ID: first.Id, Target: "panel"}); err != nil {
		t.Fatalf("apply first failed: %v", err)
	}
	if _, err := svc.Apply(AcmeApplyPayload{ID: second.Id, Target: "panel"}); err != nil {
		t.Fatalf("apply second failed: %v", err)
	}
	if _, err := svc.Apply(AcmeApplyPayload{ID: first.Id, Target: "panel"}); err != nil {
		t.Fatalf("re-apply first failed: %v", err)
	}

	ids, err := GetAssignedCertificateRecordIDs(&SettingService{}, PanelSelfSignedTargetPanel)
	if err != nil {
		t.Fatalf("read panel assigned ids failed: %v", err)
	}
	if len(ids) != 2 || ids[0] != first.Id || ids[1] != second.Id {
		t.Fatalf("panel assigned ids mismatch: got=%v want=[%d %d]", ids, first.Id, second.Id)
	}

	targets, err := assignedTargetsForCertificateRecord(first.Id)
	if err != nil {
		t.Fatalf("read assigned targets for first cert failed: %v", err)
	}
	if len(targets) != 1 || targets[0] != PanelSelfSignedTargetPanel {
		t.Fatalf("assigned targets mismatch: got=%v want=[panel]", targets)
	}

	views, err := certificateInventory.List()
	if err != nil {
		t.Fatalf("list certificate views failed: %v", err)
	}
	firstView := findCertificateViewByID(t, views, first.Id)
	secondView := findCertificateViewByID(t, views, second.Id)
	if !firstView.InUseByPanel || !secondView.InUseByPanel {
		t.Fatalf("expected both certs in-use by panel, got first=%v second=%v", firstView.InUseByPanel, secondView.InUseByPanel)
	}

	applyTargets, drainCalls := mockRuntime.snapshot()
	if len(applyTargets) != 3 {
		t.Fatalf("apply runtime calls mismatch: got=%d want=3", len(applyTargets))
	}
	for i, target := range applyTargets {
		if target != PanelSelfSignedTargetPanel {
			t.Fatalf("apply runtime target[%d] mismatch: got=%q want=%q", i, target, PanelSelfSignedTargetPanel)
		}
	}
	if len(drainCalls) != 0 {
		t.Fatalf("unexpected drain calls during apply: %#v", drainCalls)
	}
}

func TestAcmeUnapplyLastCertificateIsRejected(t *testing.T) {
	settingService := initPanelSQLiteSettingTestDB(t)
	_ = settingService

	mockRuntime := &mockPanelTLSRuntimeApplier{}
	RegisterPanelTLSRuntimeApplier(mockRuntime)
	defer RegisterPanelTLSRuntimeApplier(nil)

	only := upsertAcmeApplyTestCertificateRecord(t, "unapply-last", "fp-unapply-last")
	svc := &AcmeService{}
	if _, err := svc.Apply(AcmeApplyPayload{ID: only.Id, Target: "panel"}); err != nil {
		t.Fatalf("apply only cert failed: %v", err)
	}

	_, err := svc.Unapply(AcmeUnapplyPayload{ID: only.Id, Target: "panel"})
	if err == nil {
		t.Fatalf("expected unapply last certificate to fail")
	}
	if !strings.Contains(err.Error(), "at least one certificate must remain for target") {
		t.Fatalf("unexpected unapply error: %v", err)
	}

	ids, readErr := GetAssignedCertificateRecordIDs(&SettingService{}, PanelSelfSignedTargetPanel)
	if readErr != nil {
		t.Fatalf("read panel assigned ids failed: %v", readErr)
	}
	if len(ids) != 1 || ids[0] != only.Id {
		t.Fatalf("panel assigned ids should remain unchanged: got=%v want=[%d]", ids, only.Id)
	}

	_, drainCalls := mockRuntime.snapshot()
	if len(drainCalls) != 0 {
		t.Fatalf("unexpected drain calls when unapply was rejected: %#v", drainCalls)
	}
}

func TestAcmeUnapplyIdempotentAndDrainsRemovedFingerprint(t *testing.T) {
	settingService := initPanelSQLiteSettingTestDB(t)
	_ = settingService

	mockRuntime := &mockPanelTLSRuntimeApplier{}
	RegisterPanelTLSRuntimeApplier(mockRuntime)
	defer RegisterPanelTLSRuntimeApplier(nil)

	first := upsertAcmeApplyTestCertificateRecord(t, "unapply-first", "fp-unapply-first")
	second := upsertAcmeApplyTestCertificateRecord(t, "unapply-second", "fp-unapply-second")

	svc := &AcmeService{}
	if _, err := svc.Apply(AcmeApplyPayload{ID: first.Id, Target: "panel"}); err != nil {
		t.Fatalf("apply first failed: %v", err)
	}
	if _, err := svc.Apply(AcmeApplyPayload{ID: second.Id, Target: "panel"}); err != nil {
		t.Fatalf("apply second failed: %v", err)
	}

	if _, err := svc.Unapply(AcmeUnapplyPayload{ID: first.Id, Target: "panel"}); err != nil {
		t.Fatalf("unapply first failed: %v", err)
	}
	if _, err := svc.Unapply(AcmeUnapplyPayload{ID: first.Id, Target: "panel"}); err != nil {
		t.Fatalf("idempotent unapply should succeed, got: %v", err)
	}

	ids, err := GetAssignedCertificateRecordIDs(&SettingService{}, PanelSelfSignedTargetPanel)
	if err != nil {
		t.Fatalf("read panel assigned ids failed: %v", err)
	}
	if len(ids) != 1 || ids[0] != second.Id {
		t.Fatalf("panel assigned ids mismatch after unapply: got=%v want=[%d]", ids, second.Id)
	}

	views, err := certificateInventory.List()
	if err != nil {
		t.Fatalf("list certificate views failed: %v", err)
	}
	firstView := findCertificateViewByID(t, views, first.Id)
	secondView := findCertificateViewByID(t, views, second.Id)
	if firstView.InUseByPanel {
		t.Fatalf("first certificate should no longer be in-use by panel")
	}
	if !secondView.InUseByPanel {
		t.Fatalf("second certificate should remain in-use by panel")
	}

	row, err := certificateInventory.GetRecordByID(first.Id)
	if err != nil {
		t.Fatalf("get first certificate record failed: %v", err)
	}
	if strings.TrimSpace(row.ApplyTarget) != "" {
		t.Fatalf("first certificate apply target should be empty after unapply, got=%q", row.ApplyTarget)
	}

	_, drainCalls := mockRuntime.snapshot()
	if len(drainCalls) != 1 {
		t.Fatalf("drain call count mismatch: got=%d want=1", len(drainCalls))
	}
	drain := drainCalls[0]
	if drain.target != PanelSelfSignedTargetPanel {
		t.Fatalf("drain target mismatch: got=%q want=%q", drain.target, PanelSelfSignedTargetPanel)
	}
	if drain.fingerprint != "fp-unapply-first" {
		t.Fatalf("drain fingerprint mismatch: got=%q want=%q", drain.fingerprint, "fp-unapply-first")
	}
	if drain.gracePeriod != PanelTLSUnapplyDrainGracePeriod() {
		t.Fatalf("drain grace period mismatch: got=%s want=%s", drain.gracePeriod, PanelTLSUnapplyDrainGracePeriod())
	}
}

func upsertAcmeApplyTestCertificateRecord(t *testing.T, sourceRef string, fingerprint string) *model.CertificateRecord {
	t.Helper()
	record, err := certificateInventory.Upsert(CertificateUpsertPayload{
		SourceType:   CertificateSourceImported,
		SourceRef:    "acme-apply:" + sourceRef,
		MainDomain:   sourceRef + ".example.com",
		Domains:      []string{sourceRef + ".example.com"},
		CertPEM:      []byte("test-cert-" + sourceRef),
		KeyPEM:       []byte("test-key-" + sourceRef),
		FullchainPEM: []byte("test-cert-" + sourceRef),
		Fingerprint:  fingerprint,
	})
	if err != nil {
		t.Fatalf("upsert certificate record failed: %v", err)
	}
	return record
}

func findCertificateViewByID(t *testing.T, views []CertificateRecordView, id uint) CertificateRecordView {
	t.Helper()
	for _, item := range views {
		if item.Id == id {
			return item
		}
	}
	t.Fatalf("certificate view not found for id=%d", id)
	return CertificateRecordView{}
}
