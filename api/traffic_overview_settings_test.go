package api

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/service"
	"github.com/gin-gonic/gin"
)

func TestSaveTrafficOverviewSettingsPersistsResetDay(t *testing.T) {
	initTrafficOverviewAPITestDB(t)

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	body := "limit_gib=0&reset_day=13"
	req := httptest.NewRequest("POST", "/api/traffic-overview-settings", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	ctx.Request = req

	apiSvc := &ApiService{}
	apiSvc.SaveTrafficOverviewSettings(ctx)

	msg := decodeAPIMessage(t, rec.Body.String())
	if !msg.Success {
		t.Fatalf("expected success response, got error: %s", msg.Msg)
	}

	settings, err := (&service.SettingService{}).GetAllSetting()
	if err != nil {
		t.Fatalf("load settings failed: %v", err)
	}
	if (*settings)["trafficOverviewResetDay"] != "13" {
		t.Fatalf("trafficOverviewResetDay=%q, want %q", (*settings)["trafficOverviewResetDay"], "13")
	}
	if (*settings)["trafficOverviewExpiryDate"] != "" {
		t.Fatalf("trafficOverviewExpiryDate=%q, want empty", (*settings)["trafficOverviewExpiryDate"])
	}
}

func TestSaveTrafficOverviewSettingsAcceptsJSONPayload(t *testing.T) {
	initTrafficOverviewAPITestDB(t)

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	body := `{"limit_gib":64.25,"reset_day":17,"expiry_date":"2027-05-04"}`
	req := httptest.NewRequest("POST", "/api/traffic-overview-settings", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx.Request = req

	apiSvc := &ApiService{}
	apiSvc.SaveTrafficOverviewSettings(ctx)

	msg := decodeAPIMessage(t, rec.Body.String())
	if !msg.Success {
		t.Fatalf("expected success response, got error: %s", msg.Msg)
	}

	settings, err := (&service.SettingService{}).GetAllSetting()
	if err != nil {
		t.Fatalf("load settings failed: %v", err)
	}
	if (*settings)["trafficOverviewLimitGiB"] != "64.25" {
		t.Fatalf("trafficOverviewLimitGiB=%q, want %q", (*settings)["trafficOverviewLimitGiB"], "64.25")
	}
	if (*settings)["trafficOverviewResetDay"] != "17" {
		t.Fatalf("trafficOverviewResetDay=%q, want %q", (*settings)["trafficOverviewResetDay"], "17")
	}
	if (*settings)["trafficOverviewExpiryDate"] != "2027-05-04" {
		t.Fatalf("trafficOverviewExpiryDate=%q, want %q", (*settings)["trafficOverviewExpiryDate"], "2027-05-04")
	}
}

func TestSaveTrafficOverviewSettingsAcceptsMultipartPayload(t *testing.T) {
	initTrafficOverviewAPITestDB(t)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("limit_gib", "128.50"); err != nil {
		t.Fatalf("write limit_gib field failed: %v", err)
	}
	if err := writer.WriteField("reset_day", "22"); err != nil {
		t.Fatalf("write reset_day field failed: %v", err)
	}
	if err := writer.WriteField("expiry_date", "2028-07-18"); err != nil {
		t.Fatalf("write expiry_date field failed: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close multipart writer failed: %v", err)
	}

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	req := httptest.NewRequest("POST", "/api/traffic-overview-settings", &body)
	req.Header.Set("Content-Type", writer.FormDataContentType())
	ctx.Request = req

	apiSvc := &ApiService{}
	apiSvc.SaveTrafficOverviewSettings(ctx)

	msg := decodeAPIMessage(t, rec.Body.String())
	if !msg.Success {
		t.Fatalf("expected success response, got error: %s", msg.Msg)
	}

	settings, err := (&service.SettingService{}).GetAllSetting()
	if err != nil {
		t.Fatalf("load settings failed: %v", err)
	}
	if (*settings)["trafficOverviewLimitGiB"] != "128.50" {
		t.Fatalf("trafficOverviewLimitGiB=%q, want %q", (*settings)["trafficOverviewLimitGiB"], "128.50")
	}
	if (*settings)["trafficOverviewResetDay"] != "22" {
		t.Fatalf("trafficOverviewResetDay=%q, want %q", (*settings)["trafficOverviewResetDay"], "22")
	}
	if (*settings)["trafficOverviewExpiryDate"] != "2028-07-18" {
		t.Fatalf("trafficOverviewExpiryDate=%q, want %q", (*settings)["trafficOverviewExpiryDate"], "2028-07-18")
	}
}

func TestSaveTrafficOverviewSettingsRejectsMissingField(t *testing.T) {
	initTrafficOverviewAPITestDB(t)

	if err := (&service.TrafficOverviewService{}).UpdateTrafficOverviewSettings(0, 13, "", false); err != nil {
		t.Fatalf("seed reset day failed: %v", err)
	}

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	body := "limit_gib=0"
	req := httptest.NewRequest("POST", "/api/traffic-overview-settings", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	ctx.Request = req

	apiSvc := &ApiService{}
	apiSvc.SaveTrafficOverviewSettings(ctx)

	msg := decodeAPIMessage(t, rec.Body.String())
	if msg.Success {
		t.Fatalf("expected failure response when reset_day missing")
	}

	settings, err := (&service.SettingService{}).GetAllSetting()
	if err != nil {
		t.Fatalf("load settings failed: %v", err)
	}
	if (*settings)["trafficOverviewResetDay"] != "13" {
		t.Fatalf("reset day should stay unchanged, got %q", (*settings)["trafficOverviewResetDay"])
	}
}

func TestSaveTrafficOverviewSettingsOmittedExpiryDateKeepsPreviousValue(t *testing.T) {
	initTrafficOverviewAPITestDB(t)

	if err := (&service.TrafficOverviewService{}).UpdateTrafficOverviewSettings(32, 9, "2027-05-04", true); err != nil {
		t.Fatalf("seed expiry date failed: %v", err)
	}

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	body := `{"limit_gib":48.5,"reset_day":10}`
	req := httptest.NewRequest("POST", "/api/traffic-overview-settings", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx.Request = req

	apiSvc := &ApiService{}
	apiSvc.SaveTrafficOverviewSettings(ctx)

	msg := decodeAPIMessage(t, rec.Body.String())
	if !msg.Success {
		t.Fatalf("expected success response, got error: %s", msg.Msg)
	}

	settings, err := (&service.SettingService{}).GetAllSetting()
	if err != nil {
		t.Fatalf("load settings failed: %v", err)
	}
	if (*settings)["trafficOverviewExpiryDate"] != "2027-05-04" {
		t.Fatalf("trafficOverviewExpiryDate=%q, want %q", (*settings)["trafficOverviewExpiryDate"], "2027-05-04")
	}
}

func TestSaveTrafficOverviewSettingsEmptyExpiryDateClearsValue(t *testing.T) {
	initTrafficOverviewAPITestDB(t)

	if err := (&service.TrafficOverviewService{}).UpdateTrafficOverviewSettings(32, 9, "2027-05-04", true); err != nil {
		t.Fatalf("seed expiry date failed: %v", err)
	}

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	body := `{"limit_gib":32,"reset_day":9,"expiry_date":""}`
	req := httptest.NewRequest("POST", "/api/traffic-overview-settings", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx.Request = req

	apiSvc := &ApiService{}
	apiSvc.SaveTrafficOverviewSettings(ctx)

	msg := decodeAPIMessage(t, rec.Body.String())
	if !msg.Success {
		t.Fatalf("expected success response, got error: %s", msg.Msg)
	}

	settings, err := (&service.SettingService{}).GetAllSetting()
	if err != nil {
		t.Fatalf("load settings failed: %v", err)
	}
	if (*settings)["trafficOverviewExpiryDate"] != "" {
		t.Fatalf("trafficOverviewExpiryDate=%q, want empty", (*settings)["trafficOverviewExpiryDate"])
	}
}

func TestSaveTrafficOverviewSettingsAcceptsCompatibilityExpiryDateField(t *testing.T) {
	initTrafficOverviewAPITestDB(t)

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	body := `{"limitGiB":16,"resetDay":6,"expiryDate":"2029-08-01"}`
	req := httptest.NewRequest("POST", "/api/traffic-overview-settings", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	ctx.Request = req

	apiSvc := &ApiService{}
	apiSvc.SaveTrafficOverviewSettings(ctx)

	msg := decodeAPIMessage(t, rec.Body.String())
	if !msg.Success {
		t.Fatalf("expected success response, got error: %s", msg.Msg)
	}

	settings, err := (&service.SettingService{}).GetAllSetting()
	if err != nil {
		t.Fatalf("load settings failed: %v", err)
	}
	if (*settings)["trafficOverviewExpiryDate"] != "2029-08-01" {
		t.Fatalf("trafficOverviewExpiryDate=%q, want %q", (*settings)["trafficOverviewExpiryDate"], "2029-08-01")
	}
}

func TestSaveTrafficOverviewSwitchPersistsEnabled(t *testing.T) {
	initTrafficOverviewAPITestDB(t)

	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	req := httptest.NewRequest("POST", "/api/traffic-overview-switch", strings.NewReader(`{"enabled":false}`))
	req.Header.Set("Content-Type", "application/json")
	ctx.Request = req

	apiSvc := &ApiService{}
	apiSvc.SaveTrafficOverviewSwitch(ctx)

	msg := decodeAPIMessage(t, rec.Body.String())
	if !msg.Success {
		t.Fatalf("expected success response, got error: %s", msg.Msg)
	}

	settings, err := (&service.SettingService{}).GetAllSetting()
	if err != nil {
		t.Fatalf("load settings failed: %v", err)
	}
	if (*settings)["trafficOverviewEnabled"] != "false" {
		t.Fatalf("trafficOverviewEnabled=%q, want false", (*settings)["trafficOverviewEnabled"])
	}
}

func TestGetTrafficOverviewVnstatVersionsReturnsBothSources(t *testing.T) {
	rec := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(rec)
	ctx.Request = httptest.NewRequest("GET", "/api/traffic-overview-vnstat-versions", nil)

	apiSvc := &ApiService{}
	apiSvc.GetTrafficOverviewVnstatVersions(ctx)

	msg := decodeAPIMessage(t, rec.Body.String())
	if !msg.Success {
		t.Fatalf("expected success response, got error: %s", msg.Msg)
	}
	payload, ok := msg.Obj.(map[string]interface{})
	if !ok {
		t.Fatalf("unexpected obj payload: %#v", msg.Obj)
	}
	versions, ok := payload["versions"].([]interface{})
	if !ok || len(versions) == 0 {
		t.Fatalf("expected at least one vnstat version option, got %#v", payload["versions"])
	}
	values := make([]string, 0, len(versions))
	for _, rawVersion := range versions {
		item, ok := rawVersion.(map[string]interface{})
		if !ok {
			t.Fatalf("unexpected version option payload: %#v", rawVersion)
		}
		value, _ := item["value"].(string)
		values = append(values, value)
	}
	if len(values) != 2 {
		t.Fatalf("version option count=%d, want 2; values=%v", len(values), values)
	}
	if values[0] != "system-package" || values[1] != "github-release" {
		t.Fatalf("version option values=%v, want [system-package github-release]", values)
	}
}

func initTrafficOverviewAPITestDB(t *testing.T) {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "traffic-overview-api.db")
	if err := database.InitDB(dbPath); err != nil {
		t.Fatalf("init db failed: %v", err)
	}
	if sqlDB, err := database.GetDB().DB(); err == nil && sqlDB != nil {
		t.Cleanup(func() {
			_ = sqlDB.Close()
		})
	}

	if _, err := (&service.SettingService{}).GetAllSetting(); err != nil {
		t.Fatalf("seed default settings failed: %v", err)
	}
}

func decodeAPIMessage(t *testing.T, raw string) Msg {
	t.Helper()

	var msg Msg
	if err := json.Unmarshal([]byte(raw), &msg); err != nil {
		t.Fatalf("decode api response failed: %v", err)
	}
	return msg
}
