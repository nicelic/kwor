package api

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/gob"
	"net"
	"net/http"
	"sync"
	"time"

	"github.com/alireza0/s-ui/config"
	"github.com/alireza0/s-ui/database/model"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
)

const (
	loginUser             = "LOGIN_USER"
	loginSessionID        = "LOGIN_SESSION_ID"
	loginSessionExpiresAt = "LOGIN_SESSION_EXPIRES_AT"

	// A browser-session cookie still needs a bounded server-side lifetime.
	// Browsers do not notify the server when their session ends, so this also
	// prevents abandoned session records from growing without bound.
	defaultBrowserSessionLifetime = 24 * time.Hour
	maxSessionCookieCodecLifetime = 365 * 24 * time.Hour
	loginSessionCleanupInterval   = 15 * time.Minute
)

type loginSessionRecord struct {
	userName  string
	expiresAt time.Time
}

var loginSessionRegistry = struct {
	sync.Mutex
	sessions map[string]loginSessionRecord
}{
	sessions: make(map[string]loginSessionRecord),
}

var loginSessionNow = time.Now

func init() {
	gob.Register(model.User{})
	go func() {
		ticker := time.NewTicker(loginSessionCleanupInterval)
		defer ticker.Stop()
		for now := range ticker.C {
			loginSessionRegistry.Lock()
			pruneExpiredLoginSessionsLocked(now)
			loginSessionRegistry.Unlock()
		}
	}()
}

// isLocalDebugHTTPRequest is the narrowly scoped HTTP exception for the
// VSCode F5 workflow. RemoteAddr is used deliberately: forwarded headers can
// be supplied by an untrusted client.
func isLocalDebugHTTPRequest(c *gin.Context) bool {
	if !config.IsDebug() || c == nil || c.Request == nil || c.Request.TLS != nil {
		return false
	}
	ip := net.ParseIP(getRemoteIp(c))
	return ip != nil && ip.IsLoopback()
}

func isPanelLoginTransportAllowed(c *gin.Context) bool {
	return c != nil && c.Request != nil && (c.Request.TLS != nil || isLocalDebugHTTPRequest(c))
}

func SetLoginUser(c *gin.Context, userName string, maxAge int) error {
	maxAge = normalizeLoginSessionMaxAge(maxAge)
	s := sessions.Default(c)
	previousID, _ := s.Get(loginSessionID).(string)
	if previousID != "" {
		removeLoginSession(previousID)
	}

	sessionID, err := newLoginSessionID()
	if err != nil {
		return err
	}
	now := loginSessionNow()
	expiresAt := loginSessionExpiry(now, maxAge).Truncate(time.Second)
	storeLoginSession(sessionID, loginSessionRecord{
		userName:  userName,
		expiresAt: expiresAt,
	})
	s.Set(loginUser, userName)
	s.Set(loginSessionID, sessionID)
	s.Set(loginSessionExpiresAt, expiresAt.Unix())
	s.Options(sessionCookieOptions(c, maxAge))

	return s.Save()
}

func SetMaxAge(c *gin.Context) error {
	s := sessions.Default(c)
	if sessionID, _ := s.Get(loginSessionID).(string); sessionID != "" {
		now := loginSessionNow()
		expiresAt := now.Add(defaultBrowserSessionLifetime).Truncate(time.Second)
		refreshLoginSession(sessionID, expiresAt)
		s.Set(loginSessionExpiresAt, expiresAt.Unix())
	}
	s.Options(sessionCookieOptions(c, 0))
	return s.Save()
}

func GetLoginUser(c *gin.Context) string {
	userName, _ := getValidLoginUser(c)
	return userName
}

func IsLogin(c *gin.Context) bool {
	userName, valid := getValidLoginUser(c)
	if valid && userName != "" {
		return true
	}
	// Delete expired or legacy cookies as soon as they are presented. This is
	// intentionally best-effort: authentication has already failed even when
	// the response cannot carry the deletion cookie (for example, on a closed
	// client connection).
	clearSessionCookie(c)
	return false
}

func ClearSession(c *gin.Context) {
	if c != nil {
		s := sessions.Default(c)
		if sessionID, _ := s.Get(loginSessionID).(string); sessionID != "" {
			removeLoginSession(sessionID)
		}
	}
	clearSessionCookie(c)
}

func clearSessionCookie(c *gin.Context) {
	if c == nil {
		return
	}
	s := sessions.Default(c)
	s.Clear()
	options := sessionCookieOptions(c, 0)
	options.MaxAge = -1
	s.Options(options)
	_ = s.Save()
}

func sessionCookieOptions(c *gin.Context, maxAge int) sessions.Options {
	maxAge = normalizeLoginSessionMaxAge(maxAge)
	options := sessions.Options{
		Path:     "/",
		Secure:   !isLocalDebugHTTPRequest(c),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}
	if maxAge > 0 {
		options.MaxAge = maxAge * 60
	}
	return options
}

// SessionCookieCodecMaxAgeSeconds bounds securecookie decoding on the server.
// Per-login registry records enforce the configured (often shorter) duration.
func SessionCookieCodecMaxAgeSeconds() int {
	return int(maxSessionCookieCodecLifetime / time.Second)
}

func getValidLoginUser(c *gin.Context) (string, bool) {
	if c == nil {
		return "", false
	}
	s := sessions.Default(c)
	userName, userOK := s.Get(loginUser).(string)
	sessionID, sessionOK := s.Get(loginSessionID).(string)
	expiresAtUnix, expiresOK := sessionExpiryUnix(s.Get(loginSessionExpiresAt))
	if !userOK || userName == "" || !sessionOK || sessionID == "" || !expiresOK {
		return "", false
	}

	now := loginSessionNow()
	expiresAt := time.Unix(expiresAtUnix, 0)
	if !expiresAt.After(now) {
		removeLoginSession(sessionID)
		return "", false
	}

	loginSessionRegistry.Lock()
	defer loginSessionRegistry.Unlock()
	pruneExpiredLoginSessionsLocked(now)
	record, exists := loginSessionRegistry.sessions[sessionID]
	if !exists || record.userName != userName || !record.expiresAt.Equal(expiresAt) || !record.expiresAt.After(now) {
		return "", false
	}
	return userName, true
}

func sessionExpiryUnix(value interface{}) (int64, bool) {
	switch typed := value.(type) {
	case int64:
		return typed, typed > 0
	case int:
		return int64(typed), typed > 0
	case float64:
		parsed := int64(typed)
		return parsed, typed == float64(parsed) && parsed > 0
	default:
		return 0, false
	}
}

func loginSessionExpiry(now time.Time, maxAge int) time.Time {
	maxAge = normalizeLoginSessionMaxAge(maxAge)
	if maxAge > 0 {
		return now.Add(time.Duration(maxAge) * time.Minute)
	}
	return now.Add(defaultBrowserSessionLifetime)
}

func normalizeLoginSessionMaxAge(maxAge int) int {
	if maxAge <= 0 {
		return 0
	}
	maximum := int(maxSessionCookieCodecLifetime / time.Minute)
	if maxAge > maximum {
		return maximum
	}
	return maxAge
}

func newLoginSessionID() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

func storeLoginSession(sessionID string, record loginSessionRecord) {
	loginSessionRegistry.Lock()
	pruneExpiredLoginSessionsLocked(loginSessionNow())
	loginSessionRegistry.sessions[sessionID] = record
	loginSessionRegistry.Unlock()
}

func refreshLoginSession(sessionID string, expiresAt time.Time) {
	loginSessionRegistry.Lock()
	record, exists := loginSessionRegistry.sessions[sessionID]
	if !exists {
		loginSessionRegistry.Unlock()
		return
	}
	record.expiresAt = expiresAt
	loginSessionRegistry.sessions[sessionID] = record
	loginSessionRegistry.Unlock()
}

func removeLoginSession(sessionID string) {
	if sessionID == "" {
		return
	}
	loginSessionRegistry.Lock()
	delete(loginSessionRegistry.sessions, sessionID)
	loginSessionRegistry.Unlock()
}

func pruneExpiredLoginSessionsLocked(now time.Time) {
	for sessionID, record := range loginSessionRegistry.sessions {
		if !record.expiresAt.After(now) {
			delete(loginSessionRegistry.sessions, sessionID)
		}
	}
}
