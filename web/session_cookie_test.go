package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/alireza0/s-ui/api"
	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/service"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

func TestSessionCookieNameIsStableAndInstanceScoped(t *testing.T) {
	first := sessionCookieName([]byte("first-instance-secret"))
	if first != sessionCookieName([]byte("first-instance-secret")) {
		t.Fatal("session cookie name is not stable")
	}
	if first == sessionCookieName([]byte("second-instance-secret")) {
		t.Fatal("different instance secrets produced the same cookie name")
	}
	if len(first) != len("kwor_")+12 {
		t.Fatalf("unexpected session cookie name: %q", first)
	}
}

func TestTLSListenerRouterRebuildKeepsLiveLoginSession(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "web-session-rebuild.db")); err != nil {
		t.Fatalf("init db failed: %v", err)
	}
	if sqlDB, err := database.GetDB().DB(); err == nil && sqlDB != nil {
		t.Cleanup(func() { _ = sqlDB.Close() })
	}
	settingService := &service.SettingService{}
	if _, err := settingService.GetAllSetting(); err != nil {
		t.Fatalf("seed default settings failed: %v", err)
	}
	secret, err := settingService.GetSecret()
	if err != nil {
		t.Fatalf("load session secret failed: %v", err)
	}

	gin.SetMode(gin.TestMode)
	loginRouter := gin.New()
	loginRouter.Use(sessions.Sessions(sessionCookieName(secret), newSessionCookieStore(secret)))
	loginRouter.GET("/login", func(c *gin.Context) {
		if err := api.SetLoginUser(c, "tls-reload-test", 0); err != nil {
			c.String(http.StatusInternalServerError, err.Error())
			return
		}
		c.Status(http.StatusNoContent)
	})

	loginRecorder := httptest.NewRecorder()
	loginRouter.ServeHTTP(loginRecorder, httptest.NewRequest(http.MethodGet, "/login", nil))
	if loginRecorder.Code != http.StatusNoContent {
		t.Fatalf("login status=%d body=%q", loginRecorder.Code, loginRecorder.Body.String())
	}
	sessionCookie := sessionCookieFromResponse(t, loginRecorder, sessionCookieName(secret))
	t.Cleanup(func() { api.InvalidateAllLoginSessions("web_test_cleanup") })

	server := NewServer()
	firstRouter, err := server.initRouter()
	if err != nil {
		t.Fatalf("init first web router failed: %v", err)
	}
	firstRecorder := httptest.NewRecorder()
	firstRequest := httptest.NewRequest(http.MethodGet, "/app/api/session", nil)
	firstRequest.AddCookie(sessionCookie)
	firstRequest.AddCookie(&http.Cookie{Name: "kwor", Value: "legacy-fixed-name"})
	firstRouter.ServeHTTP(firstRecorder, firstRequest)
	assertSuccessfulSessionResponse(t, firstRecorder)
	assertCookieDeleted(t, firstRecorder, "kwor")
	sessionCookie = sessionCookieFromResponse(t, firstRecorder, sessionCookieName(secret))

	// web.Server.Restart creates a new router and CookieStore with the same
	// database secret. That TLS listener boundary must not reset api's live map.
	secondRouter, err := server.initRouter()
	if err != nil {
		t.Fatalf("init rebuilt web router failed: %v", err)
	}
	secondRecorder := httptest.NewRecorder()
	secondRequest := httptest.NewRequest(http.MethodGet, "/app/api/session", nil)
	secondRequest.AddCookie(sessionCookie)
	secondRouter.ServeHTTP(secondRecorder, secondRequest)
	assertSuccessfulSessionResponse(t, secondRecorder)
}

func sessionCookieFromResponse(t *testing.T, recorder *httptest.ResponseRecorder, name string) *http.Cookie {
	t.Helper()
	for _, sessionCookie := range recorder.Result().Cookies() {
		if sessionCookie.Name == name {
			return sessionCookie
		}
	}
	t.Fatalf("response did not include session cookie %q: %#v", name, recorder.Result().Cookies())
	return nil
}

func assertSuccessfulSessionResponse(t *testing.T, recorder *httptest.ResponseRecorder) {
	t.Helper()
	if recorder.Code != http.StatusOK {
		t.Fatalf("session status=%d body=%q", recorder.Code, recorder.Body.String())
	}
	var payload struct {
		Success bool `json:"success"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode session response: %v; body=%q", err, recorder.Body.String())
	}
	if !payload.Success {
		t.Fatalf("rebuilt web router rejected live session: %q", recorder.Body.String())
	}
}

func assertCookieDeleted(t *testing.T, recorder *httptest.ResponseRecorder, name string) {
	t.Helper()
	for _, cookie := range recorder.Result().Cookies() {
		if cookie.Name == name {
			if cookie.MaxAge >= 0 {
				t.Fatalf("cookie %q MaxAge=%d, want deletion", name, cookie.MaxAge)
			}
			return
		}
	}
	t.Fatalf("response did not delete cookie %q: %#v", name, recorder.Result().Cookies())
}
