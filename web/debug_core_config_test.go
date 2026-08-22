package web

import (
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alireza0/s-ui/api"
	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/service"
	"github.com/gin-gonic/gin"
)

func setupDebugCoreConfigRouter(t *testing.T) (*gin.Engine, *http.Cookie) {
	t.Helper()
	t.Setenv("KWOR_DEBUG", "true")
	if err := database.InitDB(filepath.Join(t.TempDir(), "web-debug-core-config.db")); err != nil {
		t.Fatalf("init db failed: %v", err)
	}
	db := database.GetDB()
	if sqlDB, err := db.DB(); err == nil && sqlDB != nil {
		t.Cleanup(func() { _ = sqlDB.Close() })
	}
	if _, err := (&service.SettingService{}).GetAllSetting(); err != nil {
		t.Fatalf("seed default settings failed: %v", err)
	}
	if err := service.InitManagedRuntimeFileStore(); err != nil {
		t.Fatalf("init managed runtime file store failed: %v", err)
	}
	if err := service.ManagedRuntimeWriteFile(service.GetSingboxConfigPath(), []byte(`{"log":{"level":"debug"}}`)); err != nil {
		t.Fatalf("write sing-box config fixture failed: %v", err)
	}
	if err := service.ManagedRuntimeWriteFile(service.GetMihomoConfigPath(), []byte("mixed-port: 7890\n")); err != nil {
		t.Fatalf("write Mihomo config fixture failed: %v", err)
	}

	previousGate := debugCoreEnvironmentCheck
	debugCoreEnvironmentCheck = func() bool { return true }
	t.Cleanup(func() {
		debugCoreEnvironmentCheck = previousGate
		api.InvalidateAllLoginSessions("debug_core_config_test_cleanup")
	})

	server := NewServer()
	router, err := server.initRouter()
	if err != nil {
		t.Fatalf("init web router failed: %v", err)
	}
	settingService := &service.SettingService{}
	secret, err := settingService.GetSecret()
	if err != nil {
		t.Fatalf("load session secret failed: %v", err)
	}
	router.GET("/test-debug-login", func(c *gin.Context) {
		if err := api.SetLoginUser(c, "debug-core-test", 30); err != nil {
			c.String(http.StatusInternalServerError, err.Error())
			return
		}
		c.Status(http.StatusNoContent)
	})

	loginRecorder := httptest.NewRecorder()
	loginRequest := httptest.NewRequest(http.MethodGet, "/test-debug-login", nil)
	loginRequest.Host = "127.0.0.1:8888"
	loginRequest.RemoteAddr = "127.0.0.1:50001"
	router.ServeHTTP(loginRecorder, loginRequest)
	if loginRecorder.Code != http.StatusNoContent {
		t.Fatalf("debug test login status=%d body=%q", loginRecorder.Code, loginRecorder.Body.String())
	}
	for _, cookie := range loginRecorder.Result().Cookies() {
		if cookie.Name == sessionCookieName(secret) {
			return router, cookie
		}
	}
	t.Fatalf("debug test login did not return session cookie")
	return nil, nil
}

func newDebugCoreConfigRequest(method string, path string, cookie *http.Cookie) *http.Request {
	request := httptest.NewRequest(method, path, nil)
	request.Host = "127.0.0.1:8888"
	request.RemoteAddr = "127.0.0.1:50002"
	if cookie != nil {
		request.AddCookie(cookie)
	}
	return request
}

func TestLocalDebugCoreConfigRoutesReturnPersistedConfigs(t *testing.T) {
	router, cookie := setupDebugCoreConfigRouter(t)

	singboxRecorder := httptest.NewRecorder()
	router.ServeHTTP(singboxRecorder, newDebugCoreConfigRequest(http.MethodGet, "/app/s", cookie))
	if singboxRecorder.Code != http.StatusOK {
		t.Fatalf("sing-box debug route status=%d body=%q", singboxRecorder.Code, singboxRecorder.Body.String())
	}
	if got := singboxRecorder.Body.String(); got != `{"log":{"level":"debug"}}` {
		t.Fatalf("unexpected sing-box debug config: %q", got)
	}
	if got := singboxRecorder.Header().Get("Content-Type"); got != "text/plain; charset=utf-8" {
		t.Fatalf("sing-box Content-Type=%q", got)
	}
	if got := singboxRecorder.Header().Get("Content-Disposition"); got != `inline; filename="config.json"` {
		t.Fatalf("sing-box Content-Disposition=%q", got)
	}
	if got := singboxRecorder.Header().Get("Cache-Control"); got != "no-store, no-cache, must-revalidate, private" {
		t.Fatalf("sing-box Cache-Control=%q", got)
	}

	mihomoRecorder := httptest.NewRecorder()
	router.ServeHTTP(mihomoRecorder, newDebugCoreConfigRequest(http.MethodGet, "/app/m", cookie))
	if mihomoRecorder.Code != http.StatusOK || mihomoRecorder.Body.String() != "mixed-port: 7890\n" {
		t.Fatalf("unexpected Mihomo debug response status=%d body=%q", mihomoRecorder.Code, mihomoRecorder.Body.String())
	}
}

func TestLocalDebugCoreConfigRoutesRejectUnsafeRequests(t *testing.T) {
	router, cookie := setupDebugCoreConfigRouter(t)

	cases := []struct {
		name       string
		method     string
		path       string
		cookie     *http.Cookie
		host       string
		remoteAddr string
	}{
		{name: "missing login", method: http.MethodGet, path: "/app/s", host: "127.0.0.1:8888", remoteAddr: "127.0.0.1:50002"},
		{name: "wrong host", method: http.MethodGet, path: "/app/s", cookie: cookie, host: "localhost:8888", remoteAddr: "127.0.0.1:50002"},
		{name: "remote peer", method: http.MethodGet, path: "/app/s", cookie: cookie, host: "127.0.0.1:8888", remoteAddr: "192.168.1.10:50002"},
		{name: "non get", method: http.MethodPost, path: "/app/s", cookie: cookie, host: "127.0.0.1:8888", remoteAddr: "127.0.0.1:50002"},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			request := newDebugCoreConfigRequest(testCase.method, testCase.path, testCase.cookie)
			request.Host = testCase.host
			request.RemoteAddr = testCase.remoteAddr
			recorder := httptest.NewRecorder()
			router.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusNotFound {
				t.Fatalf("unsafe request status=%d body=%q", recorder.Code, recorder.Body.String())
			}
			if strings.Contains(recorder.Body.String(), "mixed-port") || strings.Contains(recorder.Body.String(), "log") {
				t.Fatalf("unsafe request leaked core config: %q", recorder.Body.String())
			}
		})
	}
}

func TestLocalDebugCoreConfigRoutesRequireF5EnvironmentGate(t *testing.T) {
	router, cookie := setupDebugCoreConfigRouter(t)
	debugCoreEnvironmentCheck = func() bool { return false }

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, newDebugCoreConfigRequest(http.MethodGet, "/app/s", cookie))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("non-F5 environment status=%d body=%q", recorder.Code, recorder.Body.String())
	}
}

func TestLocalDebugCoreConfigRoutesHideMissingStoredConfig(t *testing.T) {
	router, cookie := setupDebugCoreConfigRouter(t)
	if err := database.GetDB().Exec("DELETE FROM managed_runtime_files WHERE path = ?", "core/mihomo/server.yaml").Error; err != nil {
		t.Fatalf("remove Mihomo stored config failed: %v", err)
	}

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, newDebugCoreConfigRequest(http.MethodGet, "/app/m", cookie))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("missing stored config status=%d body=%q", recorder.Code, recorder.Body.String())
	}
}

func TestVSCodeF5EnvironmentGateRejectsNonF5Executable(t *testing.T) {
	t.Setenv("KWOR_DEBUG", "false")
	if isVSCodeF5LocalDebugEnvironment() {
		t.Fatal("non-debug environment unexpectedly passed the VS Code F5 gate")
	}
	t.Setenv("KWOR_DEBUG", "true")
	if isVSCodeF5LocalDebugEnvironment() {
		t.Fatal("test executable unexpectedly passed the VS Code F5 gate")
	}
}
