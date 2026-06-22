package cmd

import (
	"testing"
	"time"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/service"
)

func TestDockerBootstrapShouldRepairPanelCertificateWhenMissingMaterial(t *testing.T) {
	settingService := initDockerBootstrapTestDB(t)

	if !dockerBootstrapShouldRepairPanelCertificate(settingService, service.PanelSelfSignedTargetPanel) {
		t.Fatal("expected missing panel tls material to require repair")
	}
}

func TestDockerBootstrapShouldRepairPanelCertificateWhenMaterialExists(t *testing.T) {
	settingService := initDockerBootstrapTestDB(t)
	_ = settingService
	if _, err := service.GenerateAndStorePanelSQLiteCertificate(service.PanelSelfSignedTargetPanel, time.Now()); err != nil {
		t.Fatalf("generate bootstrap certificate failed: %v", err)
	}

	if dockerBootstrapShouldRepairPanelCertificate(settingService, service.PanelSelfSignedTargetPanel) {
		t.Fatal("expected existing panel tls material to skip repair")
	}
}

func initDockerBootstrapTestDB(t *testing.T) *service.SettingService {
	t.Helper()

	dbPath := t.TempDir() + "\\docker-bootstrap.db"
	if err := database.InitDB(dbPath); err != nil {
		t.Fatalf("init db failed: %v", err)
	}
	if sqlDB, err := database.GetDB().DB(); err == nil && sqlDB != nil {
		t.Cleanup(func() {
			_ = sqlDB.Close()
		})
	}
	settingService := &service.SettingService{}
	if _, err := settingService.GetAllSetting(); err != nil {
		t.Fatalf("seed default settings failed: %v", err)
	}
	return settingService
}
