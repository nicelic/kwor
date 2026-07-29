package service

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/alireza0/s-ui/database/model"
)

type fakeCertificateCoreRestarter struct {
	running      bool
	restartErr   error
	restartCount int
}

func (f *fakeCertificateCoreRestarter) IsRunning() bool {
	return f.running
}

func (f *fakeCertificateCoreRestarter) RestartCore() error {
	f.restartCount++
	return f.restartErr
}

func TestCertificateCoreRestartRunsOnceForMergedPendingUpdates(t *testing.T) {
	setupMihomoSyncTestDB(t, "certificate-core-restart-once.db")
	resetAutoRenewBatchStateForTest(t)
	fake := &fakeCertificateCoreRestarter{running: true}
	now := time.Now().Unix()
	resetCertificateCoreRestartCoordinatorForTest(t, fake, nil, certificateCoreRestartPersistentState{
		Singbox: certificateCoreRestartState{
			Pending:         true,
			WindowStartedAt: now - 3601,
			WindowEndsAt:    now - 1,
			Generation:      2,
		},
	})

	if err := processCertificateCoreRestart(certificateCoreKindSingbox, now); err != nil {
		t.Fatalf("process merged restart failed: %v", err)
	}
	if err := processCertificateCoreRestart(certificateCoreKindSingbox, now+60); err != nil {
		t.Fatalf("process cleared restart failed: %v", err)
	}
	if fake.restartCount != 1 {
		t.Fatalf("restart count=%d, want 1", fake.restartCount)
	}
}

func TestCertificateCoreRestartFailureUsesThreeRapidRetriesThenPeriodic(t *testing.T) {
	setupMihomoSyncTestDB(t, "certificate-core-restart-retry.db")
	resetAutoRenewBatchStateForTest(t)
	fake := &fakeCertificateCoreRestarter{running: true, restartErr: errors.New("restart failed")}
	now := time.Now().Unix()
	resetCertificateCoreRestartCoordinatorForTest(t, fake, nil, certificateCoreRestartPersistentState{
		Singbox: certificateCoreRestartState{
			Pending:         true,
			WindowStartedAt: now - 3601,
			WindowEndsAt:    now - 1,
		},
	})

	for attempt := 0; attempt < 4; attempt++ {
		err := processCertificateCoreRestart(certificateCoreKindSingbox, now)
		if err == nil {
			t.Fatalf("attempt %d unexpectedly succeeded", attempt+1)
		}
		state := certificateCoreRestartStateSnapshotForTest(certificateCoreKindSingbox)
		if attempt < 3 && state.RetryPhase != certificateCoreRestartRetryRapid {
			t.Fatalf("attempt %d phase=%q, want rapid", attempt+1, state.RetryPhase)
		}
		if attempt == 3 {
			if state.RetryPhase != certificateCoreRestartRetryPeriodic || state.RetryCount != 3 {
				t.Fatalf("unexpected periodic retry state: %#v", state)
			}
			if state.NextRetryAt != now+int64(6*time.Hour/time.Second) {
				t.Fatalf("periodic next retry=%d, want %d", state.NextRetryAt, now+int64(6*time.Hour/time.Second))
			}
			break
		}
		now = state.NextRetryAt
	}
	if fake.restartCount != 4 {
		t.Fatalf("restart attempts=%d, want 4", fake.restartCount)
	}
}

func TestStoppedCertificateCoreIsNotStartedByRestartQueue(t *testing.T) {
	setupMihomoSyncTestDB(t, "certificate-core-restart-stopped.db")
	resetAutoRenewBatchStateForTest(t)
	fake := &fakeCertificateCoreRestarter{running: false}
	now := time.Now().Unix()
	resetCertificateCoreRestartCoordinatorForTest(t, fake, nil, certificateCoreRestartPersistentState{
		Singbox: certificateCoreRestartState{Pending: true, WindowEndsAt: now - 1},
	})

	if err := processCertificateCoreRestart(certificateCoreKindSingbox, now); err != nil {
		t.Fatalf("process stopped Core state failed: %v", err)
	}
	if fake.restartCount != 0 {
		t.Fatalf("stopped Core restart count=%d, want 0", fake.restartCount)
	}
	if state := certificateCoreRestartStateSnapshotForTest(certificateCoreKindSingbox); state.Pending {
		t.Fatalf("stopped Core pending state was not cleared: %#v", state)
	}
}

func TestCertificateCoreRestartWaitsForPersistedRapidRenewRetry(t *testing.T) {
	db := setupMihomoSyncTestDB(t, "certificate-core-waits-rapid.db")
	resetAutoRenewBatchStateForTest(t)
	fake := &fakeCertificateCoreRestarter{running: true}
	now := time.Now().Unix()
	row := &model.CertificateRecord{
		SourceType:             CertificateSourceSelfSigned,
		SourceRef:              "rapid-retry-block",
		MainDomain:             "rapid-retry.example.com",
		DomainSet:              `["rapid-retry.example.com"]`,
		AutoRenew:              true,
		AutoRenewRetryPhase:    acmeAutoRenewRetryPhaseRapid,
		AutoRenewNextRetryAt:   now + 60,
		AutoRenewLastAttemptAt: now - 540,
		NotAfter:               now + 86400,
		CertPEM:                []byte("test-cert"),
		KeyPEM:                 []byte("test-key"),
	}
	if err := db.Create(row).Error; err != nil {
		t.Fatalf("create rapid retry certificate failed: %v", err)
	}
	resetCertificateCoreRestartCoordinatorForTest(t, fake, nil, certificateCoreRestartPersistentState{
		Singbox: certificateCoreRestartState{Pending: true, WindowEndsAt: now - 1},
	})

	if err := processCertificateCoreRestart(certificateCoreKindSingbox, now); err != nil {
		t.Fatalf("process deferred restart failed: %v", err)
	}
	if fake.restartCount != 0 {
		t.Fatalf("Core restarted while rapid certificate retry was pending: %d", fake.restartCount)
	}
	if err := db.Model(row).Update("auto_renew_retry_phase", acmeAutoRenewRetryPhasePeriodic).Error; err != nil {
		t.Fatalf("move certificate to periodic retry failed: %v", err)
	}
	if err := processCertificateCoreRestart(certificateCoreKindSingbox, now); err != nil {
		t.Fatalf("process restart after rapid retries converged failed: %v", err)
	}
	if fake.restartCount != 1 {
		t.Fatalf("Core restart count=%d, want 1 after rapid retries converged", fake.restartCount)
	}
}

func TestCertificateFingerprintCheckOnlyUsesSelectedFinalConfigSections(t *testing.T) {
	keyPEM, fullchainPEM, err := (&ServerService{}).generateCertWithAlgorithm(
		"fingerprint.example.com",
		"ecc256",
		"ecc256",
		tlsCertificateUsageServer,
		time.Now().Add(-time.Hour),
		time.Now().Add(24*time.Hour),
	)
	if err != nil {
		t.Fatalf("generate certificate failed: %v", err)
	}
	fingerprint, _, _, err := inspectCertificateFingerprint(fullchainPEM, keyPEM)
	if err != nil {
		t.Fatalf("inspect certificate failed: %v", err)
	}
	certificateLines := splitCertificateRecordPEMLines(fullchainPEM)
	document := map[string]interface{}{
		"outbounds": []interface{}{map[string]interface{}{
			"tls": map[string]interface{}{"certificate": certificateLines},
		}},
		"inbounds": []interface{}{},
		"services": []interface{}{},
	}
	if certificateConfigValueContainsFingerprint(document["inbounds"], fingerprint) ||
		certificateConfigValueContainsFingerprint(document["services"], fingerprint) {
		t.Fatal("outbound-only certificate must not satisfy sing-box server config verification")
	}

	document["inbounds"] = []interface{}{map[string]interface{}{
		"tls": map[string]interface{}{"certificate": certificateLines},
	}}
	if !certificateConfigValueContainsFingerprint(document["inbounds"], fingerprint) {
		t.Fatal("inbound certificate fingerprint was not detected")
	}

	listeners := []interface{}{map[string]interface{}{"certificate": string(fullchainPEM)}}
	if !certificateConfigValueContainsFingerprint(listeners, fingerprint) {
		t.Fatal("Mihomo listener certificate fingerprint was not detected")
	}
}

func TestBoundCertificateQueuesRestartOnlyWhenFinalConfigUsesFingerprint(t *testing.T) {
	db := setupMihomoSyncTestDB(t, "certificate-core-config-usage.db")
	resetAutoRenewBatchStateForTest(t)
	fake := &fakeCertificateCoreRestarter{running: true}
	resetCertificateCoreRestartCoordinatorForTest(t, fake, nil, certificateCoreRestartPersistentState{})

	keyPEM, fullchainPEM, err := (&ServerService{}).generateCertWithAlgorithm(
		"queue.example.com",
		"ecc256",
		"ecc256",
		tlsCertificateUsageServer,
		time.Now().Add(-time.Hour),
		time.Now().Add(24*time.Hour),
	)
	if err != nil {
		t.Fatalf("generate certificate failed: %v", err)
	}
	fingerprint, notBefore, notAfter, err := inspectCertificateFingerprint(fullchainPEM, keyPEM)
	if err != nil {
		t.Fatalf("inspect certificate failed: %v", err)
	}
	record, err := certificateInventory.Upsert(CertificateUpsertPayload{
		SourceType:   CertificateSourceSelfSigned,
		SourceRef:    "queue-final-config",
		MainDomain:   "queue.example.com",
		Domains:      []string{"queue.example.com"},
		CertPEM:      fullchainPEM,
		KeyPEM:       keyPEM,
		FullchainPEM: fullchainPEM,
		Fingerprint:  fingerprint,
		NotBefore:    notBefore.Unix(),
		NotAfter:     notAfter.Unix(),
	})
	if err != nil {
		t.Fatalf("create certificate inventory record failed: %v", err)
	}
	if err := db.Create(&model.Tls{
		Name:                "queue-tls",
		CertificateRecordID: record.Id,
		Server:              mustJSONRaw(t, map[string]interface{}{}),
		Client:              mustJSONRaw(t, map[string]interface{}{}),
	}).Error; err != nil {
		t.Fatalf("create TLS binding failed: %v", err)
	}

	certificateLines := splitCertificateRecordPEMLines(fullchainPEM)
	writeSingboxFinalConfigForTest(t, map[string]interface{}{
		"inbounds": []interface{}{},
		"services": []interface{}{},
		"outbounds": []interface{}{map[string]interface{}{
			"tls": map[string]interface{}{"certificate": certificateLines},
		}},
	})
	if err := verifyAndQueueCertificateCoreRestart(certificateCoreKindSingbox, record); err != nil {
		t.Fatalf("verify outbound-only certificate failed: %v", err)
	}
	if state := certificateCoreRestartStateSnapshotForTest(certificateCoreKindSingbox); state.Pending {
		t.Fatalf("outbound-only certificate unexpectedly queued restart: %#v", state)
	}

	writeSingboxFinalConfigForTest(t, map[string]interface{}{
		"inbounds": []interface{}{map[string]interface{}{
			"tls": map[string]interface{}{"certificate": certificateLines},
		}},
		"services": []interface{}{},
	})
	if err := verifyAndQueueCertificateCoreRestart(certificateCoreKindSingbox, record); err != nil {
		t.Fatalf("verify inbound certificate failed: %v", err)
	}
	if state := certificateCoreRestartStateSnapshotForTest(certificateCoreKindSingbox); !state.Pending {
		t.Fatal("inbound certificate did not queue running sing-box restart")
	}
}

func TestCertificateCoordinatorInternalSettingsAreNotReturnedToSettingsPage(t *testing.T) {
	setupMihomoSyncTestDB(t, "certificate-internal-settings-hidden.db")
	settingService := &SettingService{}
	if err := settingService.SaveSetting(certificateCoreRestartStateSettingKey, `{"singbox":{"pending":true}}`); err != nil {
		t.Fatalf("save Core restart state failed: %v", err)
	}
	if err := settingService.SaveSetting(certificateAutoRenewBatchStateSettingKey, `{"startedAt":1,"endsAt":2}`); err != nil {
		t.Fatalf("save auto-renew batch state failed: %v", err)
	}
	settings, err := settingService.GetAllSetting()
	if err != nil {
		t.Fatalf("load settings failed: %v", err)
	}
	if _, exists := (*settings)[certificateCoreRestartStateSettingKey]; exists {
		t.Fatal("Core restart coordinator state leaked into settings response")
	}
	if _, exists := (*settings)[certificateAutoRenewBatchStateSettingKey]; exists {
		t.Fatal("auto-renew batch state leaked into settings response")
	}
}

func writeSingboxFinalConfigForTest(t *testing.T, document map[string]interface{}) {
	t.Helper()
	raw, err := json.Marshal(document)
	if err != nil {
		t.Fatalf("marshal sing-box config failed: %v", err)
	}
	if err := ManagedRuntimeWriteFile(GetSingboxConfigPath(), raw); err != nil {
		t.Fatalf("write sing-box config failed: %v", err)
	}
}

func resetCertificateCoreRestartCoordinatorForTest(
	t *testing.T,
	singbox certificateCoreRestarter,
	mihomo certificateCoreRestarter,
	persisted certificateCoreRestartPersistentState,
) {
	t.Helper()
	reset := func() {
		coordinator := certificateCoreRestartCoordinator
		coordinator.mu.Lock()
		defer coordinator.mu.Unlock()
		coordinator.loaded = true
		coordinator.singbox = singbox
		coordinator.mihomo = mihomo
		coordinator.persisted = persisted
	}
	reset()
	t.Cleanup(func() {
		coordinator := certificateCoreRestartCoordinator
		coordinator.mu.Lock()
		defer coordinator.mu.Unlock()
		coordinator.loaded = false
		coordinator.singbox = nil
		coordinator.mihomo = nil
		coordinator.persisted = certificateCoreRestartPersistentState{}
	})
}

func certificateCoreRestartStateSnapshotForTest(kind string) certificateCoreRestartState {
	coordinator := certificateCoreRestartCoordinator
	coordinator.mu.Lock()
	defer coordinator.mu.Unlock()
	state := coordinator.stateLocked(kind)
	if state == nil {
		return certificateCoreRestartState{}
	}
	return *state
}
