package service

import (
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alireza0/s-ui/database"
)

func TestResolvePanelTLSMaterialUsesAssignedRecordOnly(t *testing.T) {
	settingService := initPanelSQLiteSettingTestDB(t)

	now := time.Now()
	if _, err := GenerateAndStorePanelSQLiteCertificate(PanelSelfSignedTargetPanel, now); err != nil {
		t.Fatalf("generate panel bootstrap self-signed certificate failed: %v", err)
	}

	assignedID, err := GetAssignedCertificateRecordID(settingService, PanelSelfSignedTargetPanel)
	if err != nil {
		t.Fatalf("read assigned certificate id failed: %v", err)
	}
	if assignedID == 0 {
		t.Fatal("expected assigned certificate id after bootstrap generation")
	}

	material, err := ResolvePanelTLSMaterial(settingService, PanelSelfSignedTargetPanel)
	if err != nil {
		t.Fatalf("resolve panel tls material failed: %v", err)
	}
	if material == nil {
		t.Fatal("expected tls material")
	}
	if material.SourceType != PanelTLSSourceInventoryRecord {
		t.Fatalf("unexpected source type: %s", material.SourceType)
	}
	if material.Record == nil {
		t.Fatal("expected inventory record on resolved tls material")
	}
	if material.Record.Id != assignedID {
		t.Fatalf("resolved record id mismatch: got=%d want=%d", material.Record.Id, assignedID)
	}
	if material.Record.SourceType != CertificateSourceSelfSigned {
		t.Fatalf("unexpected inventory source type: %s", material.Record.SourceType)
	}
	if !strings.HasPrefix(material.Record.SourceRef, "bootstrap:") {
		t.Fatalf("unexpected inventory source ref: %s", material.Record.SourceRef)
	}
	if !material.Record.AutoRenew {
		t.Fatal("expected bootstrap self-signed certificate auto renew to be enabled")
	}
}

func TestResolvePanelTLSMaterialClearsMissingAssignedRecord(t *testing.T) {
	settingService := initPanelSQLiteSettingTestDB(t)

	if err := SetAssignedCertificateRecordID(settingService, PanelSelfSignedTargetPanel, 999999); err != nil {
		t.Fatalf("set assigned certificate id failed: %v", err)
	}

	material, err := ResolvePanelTLSMaterial(settingService, PanelSelfSignedTargetPanel)
	if err != nil {
		t.Fatalf("resolve panel tls material failed: %v", err)
	}
	if material != nil {
		t.Fatal("expected nil material for missing assigned record")
	}

	clearedID, err := GetAssignedCertificateRecordID(settingService, PanelSelfSignedTargetPanel)
	if err != nil {
		t.Fatalf("read assigned certificate id failed: %v", err)
	}
	if clearedID != 0 {
		t.Fatalf("expected assigned certificate id to be cleared, got=%d", clearedID)
	}
}

func initPanelSQLiteSettingTestDB(t *testing.T) *SettingService {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "panel-sqlite-settings.db")
	if err := database.InitDB(dbPath); err != nil {
		t.Fatalf("init db failed: %v", err)
	}
	if sqlDB, err := database.GetDB().DB(); err == nil && sqlDB != nil {
		t.Cleanup(func() {
			_ = sqlDB.Close()
		})
	}

	settingService := &SettingService{}
	if _, err := settingService.GetAllSetting(); err != nil {
		t.Fatalf("seed default settings failed: %v", err)
	}
	return settingService
}
