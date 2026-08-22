package service

import (
	"strconv"
	"strings"
	"testing"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
)

func TestGetFinalSubURIPrefersMultiKeyThenLegacyFallback(t *testing.T) {
	settingService := initPanelSQLiteSettingTestDB(t)
	record := upsertAssignmentTestCertificateRecord(t, "sub-uri")

	if err := settingService.SaveSetting("subURI", ""); err != nil {
		t.Fatalf("clear subURI failed: %v", err)
	}
	if err := settingService.SaveSetting(panelAssignedCertificateRecordIDsSubKey, "[]"); err != nil {
		t.Fatalf("set empty sub multi key failed: %v", err)
	}
	if err := settingService.SaveSetting(panelAssignedCertificateRecordIDSubKey, "0"); err != nil {
		t.Fatalf("set empty sub legacy key failed: %v", err)
	}

	httpURI, err := settingService.GetFinalSubURI("example.com")
	if err != nil {
		t.Fatalf("get final sub uri failed: %v", err)
	}
	if !strings.HasPrefix(httpURI, "http://") {
		t.Fatalf("unexpected default sub uri scheme: %q", httpURI)
	}

	if err := settingService.SaveSetting(panelAssignedCertificateRecordIDsSubKey, "["+strconvUint(record.Id)+"]"); err != nil {
		t.Fatalf("set sub multi key failed: %v", err)
	}
	httpsURI, err := settingService.GetFinalSubURI("example.com")
	if err != nil {
		t.Fatalf("get https final sub uri failed: %v", err)
	}
	if !strings.HasPrefix(httpsURI, "https://") {
		t.Fatalf("unexpected multi-key sub uri scheme: %q", httpsURI)
	}

	if err := settingService.SaveSetting(panelAssignedCertificateRecordIDsSubKey, "[]"); err != nil {
		t.Fatalf("reset sub multi key failed: %v", err)
	}
	if err := settingService.SaveSetting(panelAssignedCertificateRecordIDSubKey, strconvUint(record.Id)); err != nil {
		t.Fatalf("set sub legacy key failed: %v", err)
	}
	legacyURI, err := settingService.GetFinalSubURI("example.com")
	if err != nil {
		t.Fatalf("get legacy final sub uri failed: %v", err)
	}
	if !strings.HasPrefix(legacyURI, "https://") {
		t.Fatalf("unexpected legacy fallback sub uri scheme: %q", legacyURI)
	}
}

func TestGetFinalSubURIUsesCustomURIWithoutLoadingAllSettings(t *testing.T) {
	settingService := initSubPathSettingTestDB(t)
	if err := settingService.SaveSetting("subURI", "https://custom.example.test/sub"); err != nil {
		t.Fatalf("save custom subscription URI failed: %v", err)
	}

	uri, err := settingService.GetFinalSubURI("panel.example.test")
	if err != nil {
		t.Fatalf("get final sub URI failed: %v", err)
	}
	if uri != "https://custom.example.test/sub/" {
		t.Fatalf("custom subscription URI = %q", uri)
	}

	var count int64
	if err := database.GetDB().Model(&model.Setting{}).
		Where("key IN ?", []string{"subPort", "subPath", "subJsonExt", "subClashExt"}).
		Count(&count).Error; err != nil {
		t.Fatalf("count unrelated settings failed: %v", err)
	}
	if count != 0 {
		t.Fatalf("custom URI should not initialize unrelated settings, found %d", count)
	}
}

func strconvUint(value uint) string {
	return strconv.FormatUint(uint64(value), 10)
}
