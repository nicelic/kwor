package api

import (
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestCoreRequestContracts(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("version query keeps AMD64 level Mihomo-only", func(t *testing.T) {
		singboxContext, _ := gin.CreateTestContext(httptest.NewRecorder())
		singboxContext.Request = httptest.NewRequest(
			"GET",
			"/?channel=stable&target_os=linux&target_arch=amd64&target_libc=musl&target_amd64_level=v3",
			nil,
		)
		singboxRequest := parseSingboxCoreVersionWindowQuery(singboxContext)
		singboxTargetJSON, err := json.Marshal(singboxRequest.Target)
		if err != nil {
			t.Fatalf("marshal sing-box target: %v", err)
		}
		if strings.Contains(string(singboxTargetJSON), "amd64Level") {
			t.Fatalf("sing-box query accepted AMD64 level: %s", singboxTargetJSON)
		}
		if singboxRequest.Target.Libc != "musl" {
			t.Fatalf("sing-box libc was not parsed: %+v", singboxRequest.Target)
		}

		mihomoContext, _ := gin.CreateTestContext(httptest.NewRecorder())
		mihomoContext.Request = httptest.NewRequest(
			"GET",
			"/?channel=stable&target_os=linux&target_arch=amd64&target_libc=musl&target_amd64_level=v3",
			nil,
		)
		mihomoRequest := parseMihomoCoreVersionWindowQuery(mihomoContext)
		if mihomoRequest.Target.Amd64Level != "v3" {
			t.Fatalf("Mihomo AMD64 level was not parsed: %+v", mihomoRequest.Target)
		}
	})

	t.Run("download form ignores AMD64 level for sing-box", func(t *testing.T) {
		form := url.Values{
			"version":            {"v1.14.0"},
			"target_os":          {"linux"},
			"target_arch":        {"amd64"},
			"target_libc":        {"glibc"},
			"target_amd64_level": {"v2"},
			"downloadSessionId":  {"singbox-session"},
		}
		singboxContext, _ := gin.CreateTestContext(httptest.NewRecorder())
		singboxContext.Request = httptest.NewRequest("POST", "/", strings.NewReader(form.Encode()))
		singboxContext.Request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		singboxRequest := parseSingboxCoreDownloadRequest(singboxContext)
		targetJSON, err := json.Marshal(singboxRequest.Target)
		if err != nil {
			t.Fatalf("marshal sing-box download target: %v", err)
		}
		if strings.Contains(string(targetJSON), "amd64Level") {
			t.Fatalf("sing-box download form accepted AMD64 level: %s", targetJSON)
		}

		mihomoContext, _ := gin.CreateTestContext(httptest.NewRecorder())
		mihomoContext.Request = httptest.NewRequest("POST", "/", strings.NewReader(form.Encode()))
		mihomoContext.Request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		mihomoRequest := parseMihomoCoreDownloadRequest(mihomoContext)
		if mihomoRequest.Target.Amd64Level != "v2" {
			t.Fatalf("Mihomo download form lost AMD64 level: %+v", mihomoRequest.Target)
		}
	})

	t.Run("download preference keeps AMD64 level Mihomo-only", func(t *testing.T) {
		form := url.Values{
			"custom_url":         {"https://example.com/core.tar.gz"},
			"target_os":          {"linux"},
			"target_arch":        {"amd64"},
			"target_libc":        {"musl"},
			"target_amd64_level": {"v3"},
		}

		singboxContext, _ := gin.CreateTestContext(httptest.NewRecorder())
		singboxContext.Request = httptest.NewRequest("POST", "/", strings.NewReader(form.Encode()))
		singboxContext.Request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		singboxRequest := parseSingboxCorePreferenceRequest(singboxContext)
		singboxTargetJSON, err := json.Marshal(singboxRequest.Target)
		if err != nil {
			t.Fatalf("marshal sing-box preference target: %v", err)
		}
		if strings.Contains(string(singboxTargetJSON), "amd64Level") {
			t.Fatalf("sing-box preference accepted AMD64 level: %s", singboxTargetJSON)
		}
		if !singboxRequest.HasOS || !singboxRequest.HasArch || !singboxRequest.HasLibc || singboxRequest.Target.Libc != "musl" {
			t.Fatalf("sing-box preference fields were not parsed: %+v", singboxRequest)
		}

		mihomoContext, _ := gin.CreateTestContext(httptest.NewRecorder())
		mihomoContext.Request = httptest.NewRequest("POST", "/", strings.NewReader(form.Encode()))
		mihomoContext.Request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		mihomoRequest := parseMihomoCorePreferenceRequest(mihomoContext)
		if !mihomoRequest.HasOS || !mihomoRequest.HasArch || !mihomoRequest.HasAMD64Level || mihomoRequest.Target.Amd64Level != "v3" {
			t.Fatalf("Mihomo preference lost AMD64 level: %+v", mihomoRequest)
		}
	})

	t.Run("sing-box update settings parse auto update switch", func(t *testing.T) {
		form := url.Values{
			"enabled":             {"true"},
			"interval":            {"6"},
			"auto_update_enabled": {"1"},
		}
		ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
		ctx.Request = httptest.NewRequest("POST", "/", strings.NewReader(form.Encode()))
		ctx.Request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		request, err := parseSingboxCoreUpdateSettingsRequest(ctx)
		if err != nil {
			t.Fatalf("parse sing-box update settings: %v", err)
		}
		if !request.Enabled || request.IntervalHours != 6 || !request.HasAutoUpdate || !request.AutoUpdate {
			t.Fatalf("unexpected sing-box update settings request: %+v", request)
		}
	})

	t.Run("Mihomo update settings parse auto update switch", func(t *testing.T) {
		form := url.Values{
			"enabled":             {"true"},
			"interval":            {"8"},
			"auto_update_enabled": {"true"},
		}
		ctx, _ := gin.CreateTestContext(httptest.NewRecorder())
		ctx.Request = httptest.NewRequest("POST", "/", strings.NewReader(form.Encode()))
		ctx.Request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		request, err := parseMihomoCoreUpdateSettingsRequest(ctx)
		if err != nil {
			t.Fatalf("parse Mihomo update settings: %v", err)
		}
		if !request.Enabled || request.IntervalHours != 8 || !request.HasAutoUpdate || !request.AutoUpdate {
			t.Fatalf("unexpected Mihomo update settings request: %+v", request)
		}
	})
}
