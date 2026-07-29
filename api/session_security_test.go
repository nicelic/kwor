package api

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
)

func TestSessionCookieOptionsAreSecureAndHttpOnly(t *testing.T) {
	options := sessionCookieOptions(nil, 30)
	if !options.Secure || !options.HttpOnly || options.SameSite != http.SameSiteLaxMode {
		t.Fatalf("unexpected session cookie security options: %#v", options)
	}
	if options.MaxAge != 30*60 {
		t.Fatalf("session max age = %d, want %d", options.MaxAge, 30*60)
	}
}

func TestLocalDebugHTTPWhitelistAllowsLoopbackOnly(t *testing.T) {
	t.Setenv("KWOR_DEBUG", "true")

	for _, remoteAddr := range []string{"127.0.0.1:12345", "[::1]:12345"} {
		recorder := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(recorder)
		ctx.Request = httptest.NewRequest(http.MethodPost, "http://127.0.0.1/api/login", nil)
		ctx.Request.RemoteAddr = remoteAddr

		if !isPanelLoginTransportAllowed(ctx) {
			t.Fatalf("loopback address %q must be allowed in local debug mode", remoteAddr)
		}
		if options := sessionCookieOptions(ctx, 30); options.Secure {
			t.Fatalf("loopback address %q must receive an HTTP-compatible session cookie", remoteAddr)
		}
	}

	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "http://192.0.2.10/api/login", nil)
	ctx.Request.RemoteAddr = "192.0.2.10:12345"

	if isPanelLoginTransportAllowed(ctx) {
		t.Fatal("non-loopback address must not be allowed in local debug mode")
	}
	if options := sessionCookieOptions(ctx, 30); !options.Secure {
		t.Fatal("non-loopback address must retain a secure session cookie")
	}
}

func TestExpiredSessionCookieReplayIsRejectedServerSide(t *testing.T) {
	previousNow := loginSessionNow
	loginSessionRegistry.Lock()
	previousSessions := loginSessionRegistry.sessions
	loginSessionRegistry.sessions = make(map[string]loginSessionRecord)
	loginSessionRegistry.Unlock()
	t.Cleanup(func() {
		loginSessionNow = previousNow
		loginSessionRegistry.Lock()
		loginSessionRegistry.sessions = previousSessions
		loginSessionRegistry.Unlock()
	})

	now := time.Date(2026, time.July, 29, 12, 0, 0, 0, time.UTC)
	loginSessionNow = func() time.Time { return now }

	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(sessions.Sessions("kwor", cookie.NewStore([]byte("session-replay-test-secret"))))
	router.GET("/login", func(c *gin.Context) {
		if err := SetLoginUser(c, "tester", 30); err != nil {
			t.Fatalf("set login session: %v", err)
		}
		c.Status(http.StatusNoContent)
	})
	router.GET("/session", func(c *gin.Context) {
		c.String(http.StatusOK, "%t", IsLogin(c))
	})

	loginRecorder := httptest.NewRecorder()
	router.ServeHTTP(loginRecorder, httptest.NewRequest(http.MethodGet, "/login", nil))
	cookies := loginRecorder.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("login returned %d cookies, want 1", len(cookies))
	}

	now = now.Add(31 * time.Minute)
	replayRequest := httptest.NewRequest(http.MethodGet, "/session", nil)
	replayRequest.AddCookie(cookies[0])
	replayRecorder := httptest.NewRecorder()
	router.ServeHTTP(replayRecorder, replayRequest)
	if got := replayRecorder.Body.String(); got != "false" {
		t.Fatalf("expired signed cookie replay returned %q, want false", got)
	}
}

func TestGetRemoteIpUsesDirectPeerInsteadOfForwardedHeader(t *testing.T) {
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodGet, "https://panel.example/", nil)
	ctx.Request.RemoteAddr = "192.0.2.10:54321"
	ctx.Request.Header.Set("X-Forwarded-For", "198.51.100.10")
	ctx.Request.Header.Set("X-Real-IP", "198.51.100.11")
	if got := getRemoteIp(ctx); got != "192.0.2.10" {
		t.Fatalf("remote ip = %q, want direct peer", got)
	}
}

func TestLoginRejectsPlainHTTPBeforeCredentialLookup(t *testing.T) {
	t.Setenv("KWOR_DEBUG", "false")
	recorder := httptest.NewRecorder()
	ctx, _ := gin.CreateTestContext(recorder)
	ctx.Request = httptest.NewRequest(http.MethodPost, "http://panel.example/api/login", strings.NewReader("user=admin&pass=secret"))
	ctx.Request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	(&ApiService{}).Login(ctx)
	if recorder.Code != http.StatusOK {
		t.Fatalf("plain http login status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if !strings.Contains(recorder.Body.String(), "panel login requires https") {
		t.Fatalf("plain http login must be rejected, got %q", recorder.Body.String())
	}
}
