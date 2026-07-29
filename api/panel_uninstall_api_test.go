package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/alireza0/s-ui/service"
	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
)

func TestPanelUninstallRouteRequiresLoginSession(t *testing.T) {
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	engine.Use(sessions.Sessions("kwor", cookie.NewStore([]byte("panel-uninstall-route-test-secret"))))
	NewAPIHandler(engine.Group("/api"), &APIv2Handler{})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/panel-uninstall", nil)
	request.Header.Set("X-Requested-With", "XMLHttpRequest")
	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	message := decodeAPIMessage(t, recorder.Body.String())
	if message.Success || message.Msg != "Invalid login" {
		t.Fatalf("unauthenticated panel uninstall response = %#v", message)
	}
}

func TestPanelUninstallRouteDispatchesAuthenticatedRequest(t *testing.T) {
	gin.SetMode(gin.TestMode)
	previousScheduler := panelUninstallScheduler
	panelUninstallScheduler = func(uninstallService *service.PanelUninstallService) (*service.PanelUninstallResult, error) {
		return &service.PanelUninstallResult{
			Started:    true,
			BinaryPath: "/opt/kwor/kwor_amd64",
			Message:    "卸载任务已启动，面板连接即将断开",
		}, nil
	}
	t.Cleanup(func() {
		panelUninstallScheduler = previousScheduler
	})

	store := cookie.NewStore([]byte("panel-uninstall-route-test-secret"))
	engine := gin.New()
	engine.Use(sessions.Sessions("kwor", store))
	engine.GET("/test-login", func(c *gin.Context) {
		if err := SetLoginUser(c, "panel-uninstall-test", 30); err != nil {
			c.String(http.StatusInternalServerError, err.Error())
			return
		}
		c.Status(http.StatusNoContent)
	})
	NewAPIHandler(engine.Group("/api"), &APIv2Handler{})

	loginRecorder := httptest.NewRecorder()
	engine.ServeHTTP(loginRecorder, httptest.NewRequest(http.MethodGet, "/test-login", nil))
	if loginRecorder.Code != http.StatusNoContent {
		t.Fatalf("login setup status = %d", loginRecorder.Code)
	}
	cookies := loginRecorder.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("login setup cookies = %#v", cookies)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/panel-uninstall", nil)
	request.AddCookie(cookies[0])
	engine.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	message := decodeAPIMessage(t, recorder.Body.String())
	if !message.Success {
		t.Fatalf("authenticated panel uninstall response failed: %s", message.Msg)
	}
	payload, ok := message.Obj.(map[string]interface{})
	payloadMessage, messageOK := payload["message"].(string)
	if !ok || !messageOK || payload["started"] != true || !strings.Contains(payloadMessage, "任务已启动") {
		t.Fatalf("unexpected panel uninstall payload: %#v", message.Obj)
	}
}

func TestPanelUninstallIsNotExposedByAPIV2(t *testing.T) {
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(recorder)
	context.Request = httptest.NewRequest(http.MethodPost, "/apiv2/panel-uninstall", nil)
	context.Params = gin.Params{{Key: "postAction", Value: "panel-uninstall"}}

	(&APIv2Handler{}).postHandler(context)
	message := decodeAPIMessage(t, recorder.Body.String())
	if message.Success || !strings.Contains(message.Msg, "unknown action") {
		t.Fatalf("API v2 panel uninstall response = %#v", message)
	}
}
