package web

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/service"
)

func TestPanelHTMLIsNoStoreAndStaticAssetsRemainCacheable(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "web-cache-control.db")); err != nil {
		t.Fatalf("init db failed: %v", err)
	}
	if sqlDB, err := database.GetDB().DB(); err == nil && sqlDB != nil {
		t.Cleanup(func() { _ = sqlDB.Close() })
	}
	if _, err := (&service.SettingService{}).GetAllSetting(); err != nil {
		t.Fatalf("seed default settings failed: %v", err)
	}

	router, err := NewServer().initRouter()
	if err != nil {
		t.Fatalf("init router failed: %v", err)
	}

	pageRecorder := httptest.NewRecorder()
	pageRequest := httptest.NewRequest(http.MethodGet, "/app/login", nil)
	pageRequest.Host = "localhost"
	router.ServeHTTP(pageRecorder, pageRequest)
	if pageRecorder.Code != http.StatusOK {
		t.Fatalf("panel HTML status=%d want %d", pageRecorder.Code, http.StatusOK)
	}
	if got, want := pageRecorder.Header().Get("Cache-Control"), "no-store, no-cache, must-revalidate, private"; got != want {
		t.Fatalf("panel HTML Cache-Control=%q want %q", got, want)
	}
	if got := pageRecorder.Header().Get("Pragma"); got != "no-cache" {
		t.Fatalf("panel HTML Pragma=%q want no-cache", got)
	}
	if got := pageRecorder.Header().Get("Expires"); got != "0" {
		t.Fatalf("panel HTML Expires=%q want 0", got)
	}

	assetRecorder := httptest.NewRecorder()
	assetRequest := httptest.NewRequest(http.MethodGet, "/app/assets/favicon.ico", nil)
	assetRequest.Host = "localhost"
	router.ServeHTTP(assetRecorder, assetRequest)
	if assetRecorder.Code != http.StatusOK {
		t.Fatalf("asset status=%d want %d", assetRecorder.Code, http.StatusOK)
	}
	if got, want := assetRecorder.Header().Get("Cache-Control"), "max-age=31536000"; got != want {
		t.Fatalf("asset Cache-Control=%q want %q", got, want)
	}
}
