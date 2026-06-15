package service

import (
	"encoding/json"
	"strconv"
	"testing"

	"github.com/alireza0/s-ui/database/model"
)

func TestGetAssignedCertificateRecordIDsFallbacksToLegacyAndWriteback(t *testing.T) {
	settingService := initPanelSQLiteSettingTestDB(t)
	record := upsertAssignmentTestCertificateRecord(t, "panel-legacy")

	if err := settingService.SaveSetting(panelAssignedCertificateRecordIDsPanelKey, "[]"); err != nil {
		t.Fatalf("set panel multi key failed: %v", err)
	}
	if err := settingService.SaveSetting(panelAssignedCertificateRecordIDPanelKey, strconv.FormatUint(uint64(record.Id), 10)); err != nil {
		t.Fatalf("set panel legacy key failed: %v", err)
	}

	ids, err := GetAssignedCertificateRecordIDs(settingService, PanelSelfSignedTargetPanel)
	if err != nil {
		t.Fatalf("get assigned ids failed: %v", err)
	}
	if len(ids) != 1 || ids[0] != record.Id {
		t.Fatalf("assigned ids mismatch: got=%v want=[%d]", ids, record.Id)
	}

	stored, err := readAssignedIDListFromSetting(settingService, panelAssignedCertificateRecordIDsPanelKey)
	if err != nil {
		t.Fatalf("read stored panel multi key failed: %v", err)
	}
	if len(stored) != 1 || stored[0] != record.Id {
		t.Fatalf("stored panel multi key mismatch: got=%v want=[%d]", stored, record.Id)
	}
}

func TestGetAssignedCertificateRecordIDsCleansInvalidAndDedup(t *testing.T) {
	settingService := initPanelSQLiteSettingTestDB(t)
	first := upsertAssignmentTestCertificateRecord(t, "panel-clean-1")
	second := upsertAssignmentTestCertificateRecord(t, "panel-clean-2")

	raw := "[" +
		"0," +
		strconv.FormatUint(uint64(first.Id), 10) + "," +
		strconv.FormatUint(uint64(first.Id), 10) + "," +
		"999999," +
		strconv.FormatUint(uint64(second.Id), 10) + "]"
	if err := settingService.SaveSetting(panelAssignedCertificateRecordIDsPanelKey, raw); err != nil {
		t.Fatalf("set dirty panel multi key failed: %v", err)
	}
	if err := settingService.SaveSetting(panelAssignedCertificateRecordIDPanelKey, "0"); err != nil {
		t.Fatalf("set panel legacy key failed: %v", err)
	}

	ids, err := GetAssignedCertificateRecordIDs(settingService, PanelSelfSignedTargetPanel)
	if err != nil {
		t.Fatalf("get assigned ids failed: %v", err)
	}
	if len(ids) != 2 || ids[0] != first.Id || ids[1] != second.Id {
		t.Fatalf("cleaned ids mismatch: got=%v want=[%d %d]", ids, first.Id, second.Id)
	}

	legacyRaw, err := settingService.getString(panelAssignedCertificateRecordIDPanelKey)
	if err != nil {
		t.Fatalf("read panel legacy key failed: %v", err)
	}
	if legacyRaw != strconv.FormatUint(uint64(first.Id), 10) {
		t.Fatalf("legacy mirror mismatch: got=%q want=%d", legacyRaw, first.Id)
	}
}

func TestSetAssignedCertificateRecordIDsMirrorsLegacyHead(t *testing.T) {
	settingService := initPanelSQLiteSettingTestDB(t)
	first := upsertAssignmentTestCertificateRecord(t, "panel-set-1")
	second := upsertAssignmentTestCertificateRecord(t, "panel-set-2")

	if err := SetAssignedCertificateRecordIDs(settingService, PanelSelfSignedTargetPanel, []uint{
		second.Id, first.Id, second.Id, 0,
	}); err != nil {
		t.Fatalf("set assigned ids failed: %v", err)
	}

	ids, err := GetAssignedCertificateRecordIDs(settingService, PanelSelfSignedTargetPanel)
	if err != nil {
		t.Fatalf("get assigned ids failed: %v", err)
	}
	if len(ids) != 2 || ids[0] != second.Id || ids[1] != first.Id {
		t.Fatalf("stored ids mismatch: got=%v want=[%d %d]", ids, second.Id, first.Id)
	}

	legacyRaw, err := settingService.getString(panelAssignedCertificateRecordIDPanelKey)
	if err != nil {
		t.Fatalf("read panel legacy key failed: %v", err)
	}
	if legacyRaw != strconv.FormatUint(uint64(second.Id), 10) {
		t.Fatalf("legacy mirror mismatch: got=%q want=%d", legacyRaw, second.Id)
	}
}

func TestSyncPanelTLSAssignmentsMigratesLegacyForBothTargets(t *testing.T) {
	settingService := initPanelSQLiteSettingTestDB(t)
	panelRecord := upsertAssignmentTestCertificateRecord(t, "sync-panel")
	subRecord := upsertAssignmentTestCertificateRecord(t, "sync-sub")

	if err := settingService.SaveSetting(panelAssignedCertificateRecordIDsPanelKey, "[]"); err != nil {
		t.Fatalf("set panel multi key failed: %v", err)
	}
	if err := settingService.SaveSetting(panelAssignedCertificateRecordIDPanelKey, strconv.FormatUint(uint64(panelRecord.Id), 10)); err != nil {
		t.Fatalf("set panel legacy key failed: %v", err)
	}
	if err := settingService.SaveSetting(panelAssignedCertificateRecordIDsSubKey, "[]"); err != nil {
		t.Fatalf("set sub multi key failed: %v", err)
	}
	if err := settingService.SaveSetting(panelAssignedCertificateRecordIDSubKey, strconv.FormatUint(uint64(subRecord.Id), 10)); err != nil {
		t.Fatalf("set sub legacy key failed: %v", err)
	}

	if err := SyncPanelTLSAssignments(settingService); err != nil {
		t.Fatalf("sync panel tls assignments failed: %v", err)
	}

	panelIDs, err := GetAssignedCertificateRecordIDs(settingService, PanelSelfSignedTargetPanel)
	if err != nil {
		t.Fatalf("get panel ids failed: %v", err)
	}
	if len(panelIDs) != 1 || panelIDs[0] != panelRecord.Id {
		t.Fatalf("panel ids mismatch: got=%v want=[%d]", panelIDs, panelRecord.Id)
	}

	subIDs, err := GetAssignedCertificateRecordIDs(settingService, PanelSelfSignedTargetSub)
	if err != nil {
		t.Fatalf("get sub ids failed: %v", err)
	}
	if len(subIDs) != 1 || subIDs[0] != subRecord.Id {
		t.Fatalf("sub ids mismatch: got=%v want=[%d]", subIDs, subRecord.Id)
	}
}

func upsertAssignmentTestCertificateRecord(t *testing.T, sourceRef string) *model.CertificateRecord {
	t.Helper()
	record, err := certificateInventory.Upsert(CertificateUpsertPayload{
		SourceType:   CertificateSourceImported,
		SourceRef:    "assignment:" + sourceRef,
		MainDomain:   sourceRef + ".example.com",
		Domains:      []string{sourceRef + ".example.com"},
		CertPEM:      []byte("test-cert-" + sourceRef),
		KeyPEM:       []byte("test-key-" + sourceRef),
		FullchainPEM: []byte("test-cert-" + sourceRef),
		Fingerprint:  "fp-" + sourceRef,
	})
	if err != nil {
		t.Fatalf("upsert certificate record failed: %v", err)
	}
	return record
}

func readAssignedIDListFromSetting(settingService *SettingService, key string) ([]uint, error) {
	raw, err := settingService.getString(key)
	if err != nil {
		return nil, err
	}
	parsed := make([]uint, 0)
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil, err
	}
	return parsed, nil
}
