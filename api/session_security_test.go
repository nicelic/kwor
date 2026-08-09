package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"

	"github.com/gin-contrib/sessions"
	"github.com/gin-contrib/sessions/cookie"
	"github.com/gin-gonic/gin"
	"github.com/gorilla/securecookie"
	gorillaSessions "github.com/gorilla/sessions"
)

type loginSessionTestState struct {
	now     time.Time
	version string
}

type sessionAPIResponse struct {
	Success bool   `json:"success"`
	Msg     string `json:"msg"`
	Obj     struct {
		Reason         string `json:"reason"`
		DeadlineAt     int64  `json:"deadlineAt"`
		IdleDeadlineAt int64  `json:"idleDeadlineAt"`
	} `json:"obj"`
}

type failingSessionCookieStore struct {
	*gorillaSessions.CookieStore
}

func (s *failingSessionCookieStore) Options(options sessions.Options) {
	s.CookieStore.Options = options.ToGorillaOptions()
}

type failingSessionCookieCodec struct{}

func (failingSessionCookieCodec) Encode(_ string, _ interface{}) (string, error) {
	return "", errors.New("forced session cookie write failure")
}

func (failingSessionCookieCodec) Decode(_ string, _ string, _ interface{}) error {
	return errors.New("forced session cookie decode failure")
}

func isolateLoginSessionState(t *testing.T) *loginSessionTestState {
	t.Helper()

	state := &loginSessionTestState{
		now:     time.Date(2030, time.January, 1, 12, 0, 0, 0, time.UTC),
		version: "1.6.1",
	}
	previousNow := loginSessionNow
	previousVersionFn := loginSessionVersionFn
	loginSessionNow = func() time.Time { return state.now }
	loginSessionVersionFn = func() string { return state.version }

	loginSessionRegistry.Lock()
	previousSessions := loginSessionRegistry.sessions
	previousEpoch := loginSessionRegistry.epoch
	loginSessionRegistry.sessions = make(map[string]loginSessionRecord)
	loginSessionRegistry.epoch = "test-live-epoch"
	loginSessionRegistry.Unlock()

	t.Cleanup(func() {
		loginSessionNow = previousNow
		loginSessionVersionFn = previousVersionFn
		loginSessionRegistry.Lock()
		loginSessionRegistry.sessions = previousSessions
		loginSessionRegistry.epoch = previousEpoch
		loginSessionRegistry.Unlock()
	})
	return state
}

func newLoginSessionTestRouter(t *testing.T, sessionMaxAgeMinutes int) (*gin.Engine, sessions.Store) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	store := cookie.NewStore([]byte("session-test-secret"))
	router := gin.New()
	router.Use(sessions.Sessions("kwor", store))
	router.GET("/login", func(c *gin.Context) {
		if err := SetLoginUser(c, "tester", sessionMaxAgeMinutes); err != nil {
			c.String(http.StatusInternalServerError, err.Error())
			return
		}
		c.Status(http.StatusNoContent)
	})
	router.GET("/session", (&ApiService{}).Session)
	router.POST("/session", (&ApiService{}).SessionActivity)
	return router, store
}

func loginSessionCookie(t *testing.T, router *gin.Engine) *http.Cookie {
	t.Helper()
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/login", nil))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("login status=%d body=%q", recorder.Code, recorder.Body.String())
	}
	return responseSessionCookie(t, recorder)
}

func requestSession(t *testing.T, router *gin.Engine, method string, sessionCookie *http.Cookie) (*httptest.ResponseRecorder, sessionAPIResponse) {
	t.Helper()
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(method, "/session", nil)
	if sessionCookie != nil {
		request.AddCookie(sessionCookie)
	}
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("session status=%d body=%q", recorder.Code, recorder.Body.String())
	}
	var response sessionAPIResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode session response: %v; body=%q", err, recorder.Body.String())
	}
	return recorder, response
}

func responseSessionCookie(t *testing.T, recorder *httptest.ResponseRecorder) *http.Cookie {
	return responseCookieByName(t, recorder, "kwor")
}

func responseCookieByName(t *testing.T, recorder *httptest.ResponseRecorder, name string) *http.Cookie {
	t.Helper()
	for _, sessionCookie := range recorder.Result().Cookies() {
		if sessionCookie.Name == name {
			return sessionCookie
		}
	}
	t.Fatalf("response did not include cookie %q: %#v", name, recorder.Result().Cookies())
	return nil
}

func writeSessionCookie(t *testing.T, store sessions.Store, values map[interface{}]interface{}) *http.Cookie {
	t.Helper()
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(sessions.Sessions("kwor", store))
	router.GET("/write", func(c *gin.Context) {
		s := sessions.Default(c)
		for key, value := range values {
			s.Set(key, value)
		}
		s.Options(sessionCookieOptions(c, 0))
		if err := s.Save(); err != nil {
			c.String(http.StatusInternalServerError, err.Error())
			return
		}
		c.Status(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/write", nil))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("write cookie status=%d body=%q", recorder.Code, recorder.Body.String())
	}
	return responseSessionCookie(t, recorder)
}

func decodeSessionCookieValues(t *testing.T, store sessions.Store, sessionCookie *http.Cookie) map[interface{}]interface{} {
	t.Helper()
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.AddCookie(sessionCookie)
	s, err := store.Get(request, "kwor")
	if err != nil {
		t.Fatalf("decode session cookie: %v", err)
	}
	return s.Values
}

func assertDeletedSessionCookie(t *testing.T, recorder *httptest.ResponseRecorder) {
	t.Helper()
	if deletedCookie := responseSessionCookie(t, recorder); deletedCookie.MaxAge >= 0 {
		t.Fatalf("session cookie MaxAge=%d, want deletion", deletedCookie.MaxAge)
	}
}

func inMemoryLoginSessionCount() int {
	loginSessionRegistry.Lock()
	defer loginSessionRegistry.Unlock()
	return len(loginSessionRegistry.sessions)
}

func createStoredLoginSession(t *testing.T, state *loginSessionTestState, userName string, sessionMaxAgeMinutes int) (string, string) {
	t.Helper()
	token, err := newLoginSessionToken()
	if err != nil {
		t.Fatalf("create login token: %v", err)
	}
	tokenHash, ok := loginSessionTokenHash(token)
	if !ok {
		t.Fatal("generated login token did not produce a valid hash")
	}
	effectiveSessionMinutes := normalizeStoredSessionTimeoutMinutes(sessionMaxAgeMinutes)
	if _, err := storeLoginSession(
		tokenHash,
		userName,
		currentLoginSessionVersion(),
		loginSessionExpiry(state.now, effectiveSessionMinutes),
		state.now,
		effectiveSessionMinutes,
	); err != nil {
		t.Fatalf("store login session: %v", err)
	}
	return token, tokenHash
}

func prepareLoginSessionDatabase(t *testing.T) {
	t.Helper()
	if err := database.InitDB(filepath.Join(t.TempDir(), "login-session-test.db")); err != nil {
		t.Fatalf("init login session database: %v", err)
	}
	db := database.GetDB()
	if db == nil {
		t.Fatal("login session database is nil")
	}
	if err := db.Where("1 = 1").Delete(&model.LoginSession{}).Error; err != nil {
		t.Fatalf("clear login session database: %v", err)
	}
	t.Cleanup(func() {
		loginSessionRegistry.Lock()
		_, _ = clearPersistedLoginSessionsLocked()
		loginSessionRegistry.Unlock()
		if sqlDB, err := db.DB(); err == nil && sqlDB != nil {
			_ = sqlDB.Close()
		}
	})
}

func persistedLoginSessionCount(t *testing.T) int64 {
	t.Helper()
	var count int64
	if err := database.GetDB().Model(&model.LoginSession{}).Count(&count).Error; err != nil {
		t.Fatalf("count persisted login sessions: %v", err)
	}
	return count
}

func TestSessionCookieOptionsAreSecureAndBoundedToSeventyTwoHours(t *testing.T) {
	options := sessionCookieOptions(nil, 0)
	if !options.Secure || !options.HttpOnly || options.SameSite != http.SameSiteLaxMode {
		t.Fatalf("unexpected session cookie security options: %#v", options)
	}
	if options.MaxAge != 72*60*60 {
		t.Fatalf("session max age = %d, want %d", options.MaxAge, 72*60*60)
	}
	if codecMaxAge := SessionCookieCodecMaxAgeSeconds(); codecMaxAge != 72*60*60 {
		t.Fatalf("session cookie codec max age = %d, want %d", codecMaxAge, 72*60*60)
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
		if options := sessionCookieOptions(ctx, 0); options.Secure {
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
	if options := sessionCookieOptions(ctx, 0); !options.Secure {
		t.Fatal("non-loopback address must retain a secure session cookie")
	}
}

func TestLoginCookieContainsOnlyOpaqueRandomToken(t *testing.T) {
	t.Setenv("KWOR_DEBUG", "false")
	isolateLoginSessionState(t)
	gin.SetMode(gin.TestMode)
	store := cookie.NewStore([]byte("opaque-login-cookie-test-secret"))
	router := gin.New()
	router.Use(sessions.Sessions("kwor", store))
	router.GET("/login", func(c *gin.Context) {
		if err := SetLoginUser(c, "tester", 0); err != nil {
			c.String(http.StatusInternalServerError, err.Error())
			return
		}
		c.Status(http.StatusNoContent)
	})

	legacyCookie := writeSessionCookie(t, store, map[interface{}]interface{}{
		"LOGIN_USER":            "legacy-user",
		"LOGIN_PASSWORD":        "legacy-password",
		"LOGIN_SESSION_ID":      "legacy-session-id",
		"LOGIN_SESSION_VERSION": "1.6.0",
		"LOGIN_SESSION_EPOCH":   "legacy-epoch",
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/login", nil)
	request.AddCookie(legacyCookie)
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("login status=%d body=%q", recorder.Code, recorder.Body.String())
	}

	sessionCookie := responseSessionCookie(t, recorder)
	if !sessionCookie.Secure || !sessionCookie.HttpOnly || sessionCookie.SameSite != http.SameSiteLaxMode || sessionCookie.MaxAge != SessionCookieCodecMaxAgeSeconds() {
		t.Fatalf("unexpected opaque login cookie options: %#v", sessionCookie)
	}
	values := decodeSessionCookieValues(t, store, sessionCookie)
	if len(values) != 1 {
		t.Fatalf("login cookie values=%#v, want only opaque token", values)
	}
	token, ok := values[loginSessionToken].(string)
	if !ok {
		t.Fatalf("opaque login token is missing or not a string: %#v", values)
	}
	if _, ok := loginSessionTokenHash(token); !ok {
		t.Fatalf("login cookie token is not a valid %d-byte random token", loginSessionTokenBytes)
	}
	for _, legacyKey := range []string{"LOGIN_USER", "LOGIN_PASSWORD", "LOGIN_SESSION_ID", "LOGIN_SESSION_VERSION", "LOGIN_SESSION_EPOCH"} {
		if _, exists := values[legacyKey]; exists {
			t.Fatalf("legacy value %q survived the new login cookie: %#v", legacyKey, values)
		}
	}
}

func TestFixedLegacyCookieNameIsDeletedWhenPresented(t *testing.T) {
	t.Setenv("KWOR_DEBUG", "false")
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(sessions.Sessions("kwor_current_instance", cookie.NewStore([]byte("legacy-fixed-name-test-secret"))))
	router.GET("/session", func(c *gin.Context) {
		ClearLegacyLoginSessionCookie(c)
		c.Status(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/session", nil)
	request.AddCookie(&http.Cookie{Name: legacyLoginSessionCookieName, Value: "obsolete"})
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("legacy cookie cleanup status=%d body=%q", recorder.Code, recorder.Body.String())
	}
	deleted := responseCookieByName(t, recorder, legacyLoginSessionCookieName)
	if deleted.MaxAge >= 0 || !deleted.Secure || !deleted.HttpOnly || deleted.SameSite != http.SameSiteLaxMode {
		t.Fatalf("legacy cookie was not securely deleted: %#v", deleted)
	}
}

func TestConfiguredSessionKeepaliveRenewsConfiguredLease(t *testing.T) {
	state := isolateLoginSessionState(t)
	start := state.now
	router, _ := newLoginSessionTestRouter(t, 1)
	sessionCookie := loginSessionCookie(t, router)

	state.now = start.Add(30 * time.Second)
	refreshRecorder, refreshResponse := requestSession(t, router, http.MethodGet, sessionCookie)
	if !refreshResponse.Success {
		t.Fatalf("keepalive rejected active session: %#v", refreshResponse)
	}
	if want := state.now.Add(time.Minute).Unix(); refreshResponse.Obj.DeadlineAt != want || refreshResponse.Obj.IdleDeadlineAt != want {
		t.Fatalf("keepalive deadline=%d/%d, want %d", refreshResponse.Obj.DeadlineAt, refreshResponse.Obj.IdleDeadlineAt, want)
	}
	renewedCookie := responseSessionCookie(t, refreshRecorder)
	if renewedCookie.MaxAge != 60 {
		t.Fatalf("renewed cookie MaxAge=%d, want %d", renewedCookie.MaxAge, 60)
	}

	state.now = start.Add(80 * time.Second)
	secondRefreshRecorder, secondRefreshResponse := requestSession(t, router, http.MethodGet, renewedCookie)
	if !secondRefreshResponse.Success {
		t.Fatalf("second keepalive rejected active session: %#v", secondRefreshResponse)
	}
	renewedCookie = responseSessionCookie(t, secondRefreshRecorder)

	state.now = start.Add(141 * time.Second)
	expiredRecorder, expiredResponse := requestSession(t, router, http.MethodGet, renewedCookie)
	if expiredResponse.Success || expiredResponse.Obj.Reason != loginSessionReasonTimeout {
		t.Fatalf("expired keepalive response=%#v, want reason=%q", expiredResponse, loginSessionReasonTimeout)
	}
	assertDeletedSessionCookie(t, expiredRecorder)
}

func TestSessionActivityRoutePreservesSessionTimeoutReason(t *testing.T) {
	state := isolateLoginSessionState(t)
	start := state.now
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(sessions.Sessions("kwor", cookie.NewStore([]byte("session-activity-route-test-secret"))))
	NewAPIHandler(router.Group("/api"), &APIv2Handler{})
	router.GET("/login", func(c *gin.Context) {
		if err := SetLoginUser(c, "tester", 1); err != nil {
			c.String(http.StatusInternalServerError, err.Error())
			return
		}
		c.Status(http.StatusNoContent)
	})

	sessionCookie := loginSessionCookie(t, router)
	state.now = start.Add(time.Minute + time.Second)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/session", nil)
	request.AddCookie(sessionCookie)
	router.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("session activity route status=%d body=%q", recorder.Code, recorder.Body.String())
	}
	var response sessionAPIResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode activity route response: %v; body=%q", err, recorder.Body.String())
	}
	if response.Success || response.Msg != "Invalid login" || response.Obj.Reason != loginSessionReasonTimeout {
		t.Fatalf("activity route timeout response=%#v, want reason=%q", response, loginSessionReasonTimeout)
	}
	assertDeletedSessionCookie(t, recorder)
}

func TestZeroSessionMaxAgeUsesSeventyTwoHourLease(t *testing.T) {
	state := isolateLoginSessionState(t)
	start := state.now
	router, _ := newLoginSessionTestRouter(t, 0)
	sessionCookie := loginSessionCookie(t, router)

	for iteration := 1; iteration <= 3; iteration++ {
		state.now = start.Add(time.Duration(iteration) * 36 * time.Hour)
		recorder, response := requestSession(t, router, http.MethodGet, sessionCookie)
		if !response.Success {
			t.Fatalf("keepalive %d rejected zero-limit session: %#v", iteration, response)
		}
		if want := state.now.Add(72 * time.Hour).Unix(); response.Obj.DeadlineAt != want || response.Obj.IdleDeadlineAt != want {
			t.Fatalf("72-hour keepalive deadline=%d/%d, want %d", response.Obj.DeadlineAt, response.Obj.IdleDeadlineAt, want)
		}
		sessionCookie = responseSessionCookie(t, recorder)
		if sessionCookie.MaxAge != 72*60*60 {
			t.Fatalf("zero session max age cookie MaxAge=%d, want %d", sessionCookie.MaxAge, 72*60*60)
		}
	}
}

func TestSessionActivityCompatibilityRefreshesUnifiedDeadline(t *testing.T) {
	state := isolateLoginSessionState(t)
	start := state.now
	router, _ := newLoginSessionTestRouter(t, 1)
	sessionCookie := loginSessionCookie(t, router)

	state.now = start.Add(50 * time.Second)
	activityRecorder, activityResponse := requestSession(t, router, http.MethodPost, sessionCookie)
	if !activityResponse.Success {
		t.Fatalf("compatibility POST refresh rejected session: %#v", activityResponse)
	}
	if want := state.now.Add(time.Minute).Unix(); activityResponse.Obj.DeadlineAt != want || activityResponse.Obj.IdleDeadlineAt != want {
		t.Fatalf("compatibility POST deadline=%d/%d, want %d", activityResponse.Obj.DeadlineAt, activityResponse.Obj.IdleDeadlineAt, want)
	}

	state.now = start.Add(100 * time.Second)
	_, followupResponse := requestSession(t, router, http.MethodGet, responseSessionCookie(t, activityRecorder))
	if !followupResponse.Success {
		t.Fatalf("compatibility POST did not renew session timeout: %#v", followupResponse)
	}
}

func TestLegacyCookieFormatIsRejectedAndDeleted(t *testing.T) {
	state := isolateLoginSessionState(t)
	gin.SetMode(gin.TestMode)
	store := cookie.NewStore([]byte("legacy-session-test-secret"))
	router := gin.New()
	router.Use(sessions.Sessions("kwor", store))
	router.GET("/session", (&ApiService{}).Session)

	// This is deliberately the pre-token shape. It is not migrated or read,
	// even if a coincidental old in-memory entry still exists.
	loginSessionRegistry.Lock()
	loginSessionRegistry.sessions["legacy-session-id"] = loginSessionRecord{
		userName:       "tester",
		version:        currentLoginSessionVersion(),
		epoch:          loginSessionRegistry.epoch,
		expiresAt:      loginSessionExpiry(state.now, 0),
		lastActivityAt: state.now,
	}
	loginSessionRegistry.Unlock()
	legacyCookie := writeSessionCookie(t, store, map[interface{}]interface{}{
		"LOGIN_USER":            "tester",
		"LOGIN_SESSION_ID":      "legacy-session-id",
		"LOGIN_SESSION_VERSION": currentLoginSessionVersion(),
		"LOGIN_SESSION_EPOCH":   "test-live-epoch",
	})

	recorder, response := requestSession(t, router, http.MethodGet, legacyCookie)
	if response.Success || response.Obj.Reason != loginSessionReasonMissing {
		t.Fatalf("legacy cookie response=%#v, want reason=%q", response, loginSessionReasonMissing)
	}
	assertDeletedSessionCookie(t, recorder)
}

func TestVersionMismatchAndLiveEpochResetRejectSession(t *testing.T) {
	t.Run("version mismatch", func(t *testing.T) {
		state := isolateLoginSessionState(t)
		router, _ := newLoginSessionTestRouter(t, 0)
		sessionCookie := loginSessionCookie(t, router)

		state.version = "1.6.2"
		recorder, response := requestSession(t, router, http.MethodGet, sessionCookie)
		if response.Success || response.Obj.Reason != loginSessionReasonVersion {
			t.Fatalf("version mismatch response=%#v, want reason=%q", response, loginSessionReasonVersion)
		}
		assertDeletedSessionCookie(t, recorder)
	})

	t.Run("live epoch mismatch", func(t *testing.T) {
		isolateLoginSessionState(t)
		router, store := newLoginSessionTestRouter(t, 0)
		sessionCookie := loginSessionCookie(t, router)
		values := decodeSessionCookieValues(t, store, sessionCookie)
		token, _ := values[loginSessionToken].(string)
		tokenHash, ok := loginSessionTokenHash(token)
		if !ok {
			t.Fatal("could not hash current login token")
		}

		loginSessionRegistry.Lock()
		record := loginSessionRegistry.sessions[tokenHash]
		record.epoch = "previous-live-epoch"
		loginSessionRegistry.sessions[tokenHash] = record
		loginSessionRegistry.Unlock()

		recorder, response := requestSession(t, router, http.MethodGet, sessionCookie)
		if response.Success || response.Obj.Reason != loginSessionReasonRestarted {
			t.Fatalf("epoch mismatch response=%#v, want reason=%q", response, loginSessionReasonRestarted)
		}
		assertDeletedSessionCookie(t, recorder)
	})

	t.Run("complete panel reset", func(t *testing.T) {
		isolateLoginSessionState(t)
		router, _ := newLoginSessionTestRouter(t, 0)
		sessionCookie := loginSessionCookie(t, router)

		InvalidateAllLoginSessions("panel_restart")
		recorder, response := requestSession(t, router, http.MethodGet, sessionCookie)
		if response.Success {
			t.Fatalf("complete panel reset retained a login: %#v", response)
		}
		assertDeletedSessionCookie(t, recorder)
	})
}

func TestLoginCookieWriteFailureRollsBackLiveSession(t *testing.T) {
	isolateLoginSessionState(t)
	gin.SetMode(gin.TestMode)
	store := &failingSessionCookieStore{CookieStore: gorillaSessions.NewCookieStore([]byte("failing-session-test-secret"))}
	store.Codecs = []securecookie.Codec{failingSessionCookieCodec{}}
	router := gin.New()
	router.Use(sessions.Sessions("kwor", store))
	router.GET("/login", func(c *gin.Context) {
		if err := SetLoginUser(c, "cookie-write-failure", 0); err == nil {
			c.String(http.StatusInternalServerError, "expected cookie write failure")
			return
		}
		c.Status(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/login", nil))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("login failure handler status=%d body=%q", recorder.Code, recorder.Body.String())
	}

	loginSessionRegistry.Lock()
	defer loginSessionRegistry.Unlock()
	for _, record := range loginSessionRegistry.sessions {
		if record.userName == "cookie-write-failure" {
			t.Fatalf("failed cookie write retained live session: %#v", record)
		}
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

func TestLoginSessionDatabaseOverflowPromotesAndRecycles(t *testing.T) {
	state := isolateLoginSessionState(t)
	prepareLoginSessionDatabase(t)

	tokens := make([]string, 0, maxInMemoryLoginSessions+2)
	hashes := make([]string, 0, maxInMemoryLoginSessions+2)
	for index := 0; index < maxInMemoryLoginSessions+1; index++ {
		token, tokenHash := createStoredLoginSession(t, state, "overflow-user", 0)
		tokens = append(tokens, token)
		hashes = append(hashes, tokenHash)
	}
	if got := inMemoryLoginSessionCount(); got != maxInMemoryLoginSessions {
		t.Fatalf("in-memory session count=%d, want %d", got, maxInMemoryLoginSessions)
	}
	if got := persistedLoginSessionCount(t); got != 1 {
		t.Fatalf("persisted overflow count=%d, want 1", got)
	}

	var overflow model.LoginSession
	if err := database.GetDB().Where("token_hash = ?", hashes[maxInMemoryLoginSessions]).Take(&overflow).Error; err != nil {
		t.Fatalf("load persisted overflow session: %v", err)
	}
	if overflow.TokenHash != hashes[maxInMemoryLoginSessions] || overflow.TokenHash == tokens[maxInMemoryLoginSessions] || strings.Contains(overflow.TokenHash, tokens[maxInMemoryLoginSessions]) {
		t.Fatalf("database stored an unexpected token representation: %#v", overflow)
	}

	// Simulate a just-freed slot before a request for the overflow record. The
	// request must promote that record into memory rather than keep using SQLite.
	loginSessionRegistry.Lock()
	delete(loginSessionRegistry.sessions, hashes[0])
	loginSessionRegistry.Unlock()
	if _, refreshed, reason, err := refreshLoginSession(hashes[maxInMemoryLoginSessions], state.now); err != nil || !refreshed || reason != "" {
		t.Fatalf("promote overflow session refreshed=%v reason=%q err=%v", refreshed, reason, err)
	}
	if got := inMemoryLoginSessionCount(); got != maxInMemoryLoginSessions {
		t.Fatalf("promoted in-memory session count=%d, want %d", got, maxInMemoryLoginSessions)
	}
	if got := persistedLoginSessionCount(t); got != 0 {
		t.Fatalf("persisted count after promotion=%d, want 0", got)
	}

	_, refillHash := createStoredLoginSession(t, state, "overflow-user", 0)
	if got := persistedLoginSessionCount(t); got != 1 {
		t.Fatalf("persisted overflow count before automatic refill=%d, want 1", got)
	}
	removeLoginSession(hashes[1])
	if got := inMemoryLoginSessionCount(); got != maxInMemoryLoginSessions {
		t.Fatalf("in-memory count after automatic refill=%d, want %d", got, maxInMemoryLoginSessions)
	}
	if got := persistedLoginSessionCount(t); got != 0 {
		t.Fatalf("persisted count after automatic refill=%d, want 0", got)
	}
	loginSessionRegistry.Lock()
	_, refilled := loginSessionRegistry.sessions[refillHash]
	loginSessionRegistry.Unlock()
	if !refilled {
		t.Fatal("released memory slot did not refill from the database overflow session")
	}

	for _, record := range []loginSessionRecord{
		{
			userName:       "expired",
			version:        currentLoginSessionVersion(),
			epoch:          loginSessionRegistry.epoch,
			expiresAt:      state.now.Add(-time.Second),
			lastActivityAt: state.now,
		},
		{
			userName:       "old-epoch",
			version:        currentLoginSessionVersion(),
			epoch:          "expired-live-epoch",
			expiresAt:      loginSessionExpiry(state.now, 0),
			lastActivityAt: state.now,
		},
		{
			userName:       "old-version",
			version:        "1.6.0",
			epoch:          loginSessionRegistry.epoch,
			expiresAt:      loginSessionExpiry(state.now, 0),
			lastActivityAt: state.now,
		},
	} {
		token, err := newLoginSessionToken()
		if err != nil {
			t.Fatalf("create stale login token: %v", err)
		}
		tokenHash, ok := loginSessionTokenHash(token)
		if !ok {
			t.Fatal("create stale login token produced an invalid hash")
		}
		if err := database.GetDB().Create(loginSessionModel(tokenHash, record)).Error; err != nil {
			t.Fatalf("create stale persisted session: %v", err)
		}
	}
	loginSessionRegistry.Lock()
	err := prunePersistedLoginSessionsLocked(state.now)
	loginSessionRegistry.Unlock()
	if err != nil {
		t.Fatalf("prune persisted login sessions: %v", err)
	}
	if got := persistedLoginSessionCount(t); got != 0 {
		t.Fatalf("stale persisted sessions remained after cleanup: %d", got)
	}
}

func TestLoginCookieWriteFailureRollsBackDatabaseOverflow(t *testing.T) {
	state := isolateLoginSessionState(t)
	prepareLoginSessionDatabase(t)
	for index := 0; index < maxInMemoryLoginSessions; index++ {
		createStoredLoginSession(t, state, "overflow-user", 0)
	}
	if got := inMemoryLoginSessionCount(); got != maxInMemoryLoginSessions {
		t.Fatalf("in-memory session count=%d, want %d", got, maxInMemoryLoginSessions)
	}

	gin.SetMode(gin.TestMode)
	store := &failingSessionCookieStore{CookieStore: gorillaSessions.NewCookieStore([]byte("failing-overflow-session-test-secret"))}
	store.Codecs = []securecookie.Codec{failingSessionCookieCodec{}}
	router := gin.New()
	router.Use(sessions.Sessions("kwor", store))
	router.GET("/login", func(c *gin.Context) {
		if err := SetLoginUser(c, "database-cookie-write-failure", 0); err == nil {
			c.String(http.StatusInternalServerError, "expected cookie write failure")
			return
		}
		c.Status(http.StatusNoContent)
	})

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/login", nil))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("login failure handler status=%d body=%q", recorder.Code, recorder.Body.String())
	}
	if got := persistedLoginSessionCount(t); got != 0 {
		t.Fatalf("failed cookie write left %d database overflow sessions", got)
	}
}
