package api

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alireza0/s-ui/service"
	"github.com/gin-gonic/gin"
)

func TestGetAndSaveSettingsLanguage(t *testing.T) {
	initTrafficOverviewAPITestDB(t)

	apiSvc := &ApiService{}

	// Test GET default language
	{
		rec := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(rec)
		ctx.Request = httptest.NewRequest("GET", "/api/settings-language", nil)
		apiSvc.GetSettingsLanguage(ctx)

		var resp struct {
			Success bool `json:"success"`
			Obj     struct {
				DefaultLanguage string `json:"defaultLanguage"`
			} `json:"obj"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if !resp.Success {
			t.Fatalf("expected success, got failure")
		}
		if resp.Obj.DefaultLanguage != "zhHans" {
			t.Fatalf("defaultLanguage = %q, want zhHans", resp.Obj.DefaultLanguage)
		}
	}

	// Test POST save language
	{
		rec := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(rec)
		body := `{"language":"en"}`
		ctx.Request = httptest.NewRequest("POST", "/api/settings-language", strings.NewReader(body))
		ctx.Request.Header.Set("Content-Type", "application/json")
		apiSvc.SaveSettingsLanguage(ctx)

		var resp struct {
			Success bool `json:"success"`
			Obj     struct {
				DefaultLanguage string `json:"defaultLanguage"`
			} `json:"obj"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if !resp.Success {
			t.Fatalf("expected success, got failure")
		}
		if resp.Obj.DefaultLanguage != "en" {
			t.Fatalf("saved defaultLanguage = %q, want en", resp.Obj.DefaultLanguage)
		}
	}

	// Verify persistence in DB
	lang, err := (&service.SettingService{}).GetDefaultLanguage()
	if err != nil {
		t.Fatalf("failed to get default language: %v", err)
	}
	if lang != "en" {
		t.Fatalf("persisted language = %q, want en", lang)
	}
}
