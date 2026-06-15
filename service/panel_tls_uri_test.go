package service

import (
	"strconv"
	"strings"
	"testing"
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

func strconvUint(value uint) string {
	return strconv.FormatUint(uint64(value), 10)
}
