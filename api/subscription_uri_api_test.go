package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/service"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
)

func initSubscriptionURIAPITestDB(t *testing.T) *service.SettingService {
	t.Helper()

	if err := database.InitDB(filepath.Join(t.TempDir(), "subscription-uri-api.db")); err != nil {
		t.Fatalf("init db failed: %v", err)
	}
	if sqlDB, err := database.GetDB().DB(); err == nil && sqlDB != nil {
		t.Cleanup(func() { _ = sqlDB.Close() })
	}

	settingService := &service.SettingService{}
	if _, err := settingService.GetAllSetting(); err != nil {
		t.Fatalf("seed default settings failed: %v", err)
	}
	return settingService
}

func requestSubscriptionURI(t *testing.T, host string) string {
	t.Helper()

	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	request := httptest.NewRequest(http.MethodGet, "/api/subscription-uri", nil)
	request.Host = host
	context.Request = request
	(&ApiService{}).GetSubscriptionURI(context)

	var response Msg
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode response failed: %v", err)
	}
	if !response.Success {
		t.Fatalf("endpoint failed: %s", response.Msg)
	}
	body, ok := response.Obj.(map[string]interface{})
	if !ok {
		t.Fatalf("unexpected response object: %#v", response.Obj)
	}
	uri, ok := body["subURI"].(string)
	if !ok || uri == "" {
		t.Fatalf("invalid subscription URI: %#v", body["subURI"])
	}
	return uri
}

func TestGetSubscriptionURIUsesRequestHostDomainPortPathAndTLS(t *testing.T) {
	settingService := initSubscriptionURIAPITestDB(t)
	if err := settingService.SaveSetting("subURI", ""); err != nil {
		t.Fatalf("clear fixed subscription URI failed: %v", err)
	}
	if err := settingService.SaveSetting("subDomain", ""); err != nil {
		t.Fatalf("clear subscription domain failed: %v", err)
	}
	if err := settingService.SaveSetting("subPort", "35844"); err != nil {
		t.Fatalf("save subscription port failed: %v", err)
	}
	if err := settingService.SaveSetting("subPath", "/edge-sub/"); err != nil {
		t.Fatalf("save subscription path failed: %v", err)
	}
	if got, want := requestSubscriptionURI(t, "panel.example.test:8443"), "http://panel.example.test:35844/edge-sub/"; got != want {
		t.Fatalf("request host URI=%q want %q", got, want)
	}
	if got, want := requestSubscriptionURI(t, "[2001:db8::10]"), "http://[2001:db8::10]:35844/edge-sub/"; got != want {
		t.Fatalf("IPv6 request host URI=%q want %q", got, want)
	}
	if got, want := requestSubscriptionURI(t, "[2001:db8::11]:8443"), "http://[2001:db8::11]:35844/edge-sub/"; got != want {
		t.Fatalf("IPv6 request host with port URI=%q want %q", got, want)
	}

	if err := settingService.SaveSetting("subDomain", "sub.example.test"); err != nil {
		t.Fatalf("save subscription domain failed: %v", err)
	}
	if err := settingService.SaveSetting("subPort", "80"); err != nil {
		t.Fatalf("save default HTTP subscription port failed: %v", err)
	}
	if got, want := requestSubscriptionURI(t, "ignored.example.test:8443"), "http://sub.example.test/edge-sub/"; got != want {
		t.Fatalf("subscription domain URI=%q want %q", got, want)
	}

	inventory := &service.CertificateInventoryService{}
	record, err := inventory.Upsert(service.CertificateUpsertPayload{
		SourceType:   "imported",
		SourceRef:    "subscription-uri-api-test",
		MainDomain:   "sub.example.test",
		Domains:      []string{"sub.example.test"},
		CertPEM:      []byte("test-cert"),
		KeyPEM:       []byte("test-key"),
		FullchainPEM: []byte("test-cert"),
		Fingerprint:  "subscription-uri-api-test",
	})
	if err != nil {
		t.Fatalf("create certificate inventory record failed: %v", err)
	}
	if err := service.SetAssignedCertificateRecordID(settingService, service.PanelSelfSignedTargetSub, record.Id); err != nil {
		t.Fatalf("assign subscription certificate failed: %v", err)
	}
	if err := settingService.SaveSetting("subPort", "443"); err != nil {
		t.Fatalf("save default HTTPS subscription port failed: %v", err)
	}
	if got, want := requestSubscriptionURI(t, "ignored.example.test:8443"), "https://sub.example.test/edge-sub/"; got != want {
		t.Fatalf("TLS subscription URI=%q want %q", got, want)
	}

	if err := settingService.SaveSetting("subURI", "  https://custom.example.test/custom-prefix  "); err != nil {
		t.Fatalf("save custom subscription URI failed: %v", err)
	}
	if got, want := requestSubscriptionURI(t, "ignored.example.test:8443"), "https://custom.example.test/custom-prefix/"; got != want {
		t.Fatalf("custom subscription URI=%q want %q", got, want)
	}
}

func TestNormalizeSubscriptionBaseURIRejectsNonBaseURLs(t *testing.T) {
	for _, value := range []string{
		"",
		"ftp://sub.example.test/path/",
		"https://sub.example.test/path/?token=secret",
		"https://sub.example.test/path/#fragment",
	} {
		if normalized, err := normalizeSubscriptionBaseURI(value); err == nil {
			t.Fatalf("normalizeSubscriptionBaseURI(%q)=%q, want error", value, normalized)
		}
	}
}

func TestDynamicAPIHandlersUseNoStoreResponseHeader(t *testing.T) {
	initSubscriptionURIAPITestDB(t)
	router := gin.New()
	router.Use(sessions.Sessions("kwor", cookie.NewStore([]byte("subscription-uri-api-test-secret"))))
	apiv2 := NewAPIv2Handler(router.Group("/apiv2"))
	NewAPIHandler(router.Group("/api"), apiv2)

	for _, path := range []string{"/api/settings", "/apiv2/settings"} {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodGet, path, nil)
		request.Header.Set("X-Requested-With", "XMLHttpRequest")
		router.ServeHTTP(recorder, request)
		if got, want := recorder.Header().Get("Cache-Control"), "no-store, no-cache, must-revalidate, private"; got != want {
			t.Fatalf("%s Cache-Control=%q want %q", path, got, want)
		}
		if got := recorder.Header().Get("Pragma"); got != "no-cache" {
			t.Fatalf("%s Pragma=%q want no-cache", path, got)
		}
		if got := recorder.Header().Get("Expires"); got != "0" {
			t.Fatalf("%s Expires=%q want 0", path, got)
		}
	}
}
