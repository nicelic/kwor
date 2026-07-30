package api

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/alireza0/s-ui/config"
	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/logger"

	"github.com/gin-contrib/sessions"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

const (
	loginSessionToken            = "LOGIN_SESSION_TOKEN"
	legacyLoginSessionCookieName = "kwor"

	// The page-presence lease is separate from the configurable user-operation
	// idle limit. A visible page renews this lease every minute; a hidden or
	// closed page stops renewing it and loses the cookie after ten minutes.
	loginSessionLifetime            = 10 * time.Minute
	loginSessionCookieMaxAge        = int(loginSessionLifetime / time.Second)
	loginSessionCleanupInterval     = time.Minute
	loginSessionTokenBytes          = 32
	maxInMemoryLoginSessions        = 25
	maxLoginSessionIdleLimitMinutes = 365 * 24 * 60
	loginSessionVersionFallback     = "unknown"
	loginSessionReasonMissing       = "missing_or_legacy_cookie"
	loginSessionReasonVersion       = "version_mismatch"
	loginSessionReasonPageInactive  = "page_inactive_timeout"
	loginSessionReasonUserIdle      = "user_idle_timeout"
	loginSessionReasonRestarted     = "panel_restarted"
	loginSessionReasonLiveMissing   = "live_session_missing"
	loginSessionReasonLiveChanged   = "live_session_mismatch"
	loginSessionReasonStoreFailed   = "session_store_unavailable"
	loginSessionReasonWriteFailed   = "cookie_write_failed"
)

type loginSessionRecord struct {
	userName         string
	version          string
	epoch            string
	expiresAt        time.Time
	lastActivityAt   time.Time
	idleLimitMinutes int
}

type loginSessionStorage uint8

const (
	loginSessionStorageMemory loginSessionStorage = iota
	loginSessionStorageDatabase
)

type loginSessionValidation struct {
	token     string
	tokenHash string
	userName  string
	reason    string
	err       error
}

type loginSessionStatus struct {
	IdleDeadlineAt int64 `json:"idleDeadlineAt,omitempty"`
}

func (v loginSessionValidation) valid() bool {
	return v.err == nil && v.reason == "" && v.userName != "" && v.tokenHash != ""
}

var loginSessionRegistry = struct {
	sync.Mutex
	epoch    string
	sessions map[string]loginSessionRecord
}{
	epoch:    newLoginSessionEpoch(),
	sessions: make(map[string]loginSessionRecord),
}

var (
	loginSessionNow       = time.Now
	loginSessionVersionFn = config.GetVersion
	loginSessionCleanup   sync.Once
)

// InitializeLoginSessionStore runs only after SQLite is ready. Persisted rows
// are runtime overflow records, never restart-resumable sessions, so a fresh
// panel process always clears them and rotates the live epoch.
func InitializeLoginSessionStore() error {
	loginSessionRegistry.Lock()
	memoryCount := len(loginSessionRegistry.sessions)
	loginSessionRegistry.sessions = make(map[string]loginSessionRecord)
	loginSessionRegistry.epoch = newLoginSessionEpoch()
	persistedCount, err := clearPersistedLoginSessionsLocked()
	loginSessionRegistry.Unlock()
	if err != nil {
		return err
	}

	loginSessionCleanup.Do(startLoginSessionCleanup)
	logger.Infof("login session store initialized: memory=%d database=%d", memoryCount, persistedCount)
	return nil
}

func startLoginSessionCleanup() {
	go func() {
		ticker := time.NewTicker(loginSessionCleanupInterval)
		defer ticker.Stop()
		for now := range ticker.C {
			loginSessionRegistry.Lock()
			pruneExpiredLoginSessionsLocked(now)
			err := prunePersistedLoginSessionsLocked(now)
			if err == nil {
				err = refillInMemoryLoginSessionsLocked(now)
			}
			loginSessionRegistry.Unlock()
			if err != nil {
				logger.Warning("login session cleanup failed: ", err)
			}
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

// SetLoginUser creates an opaque 256-bit token. The browser Cookie contains
// only that token; the username and every policy field stay server-side.
func SetLoginUser(c *gin.Context, userName string, idleLimitMinutes int) error {
	if c == nil {
		return fmt.Errorf("login session context is nil")
	}

	s := sessions.Default(c)
	previousToken, _ := s.Get(loginSessionToken).(string)
	token, err := newLoginSessionToken()
	if err != nil {
		return err
	}
	tokenHash, validToken := loginSessionTokenHash(token)
	if !validToken {
		return fmt.Errorf("generated login session token is invalid")
	}

	now := loginSessionNow().Truncate(time.Second)
	if _, err := storeLoginSession(
		tokenHash,
		userName,
		currentLoginSessionVersion(),
		loginSessionExpiry(now),
		now,
		normalizeLoginSessionIdleLimit(idleLimitMinutes),
	); err != nil {
		return err
	}

	// A new cookie shape deliberately has no compatibility path. Clearing first
	// ensures legacy LOGIN_USER/version/epoch values cannot survive a login.
	s.Clear()
	s.Set(loginSessionToken, token)
	s.Options(sessionCookieOptions(c))
	if err := s.Save(); err != nil {
		removeLoginSession(tokenHash)
		return err
	}
	if previousHash, ok := loginSessionTokenHash(previousToken); ok && previousHash != tokenHash {
		removeLoginSession(previousHash)
	}
	return nil
}

// RefreshLoginSession renews the page-presence lease. Only the explicit
// activity endpoint passes userActivity=true, so periodic polling and Cookie
// keepalive can never extend the configurable user-operation idle limit.
func RefreshLoginSession(c *gin.Context, userActivity bool) (loginSessionStatus, bool, string, error) {
	validation := validateLoginSession(c)
	if !validation.valid() {
		clearSessionCookie(c)
		return loginSessionStatus{}, false, validation.reason, validation.err
	}

	now := loginSessionNow().Truncate(time.Second)
	record, refreshed, reason, err := refreshLoginSession(
		validation.tokenHash,
		loginSessionExpiry(now),
		now,
		userActivity,
	)
	if !refreshed {
		clearSessionCookie(c)
		return loginSessionStatus{}, false, reason, err
	}

	s := sessions.Default(c)
	s.Clear()
	s.Set(loginSessionToken, validation.token)
	s.Options(sessionCookieOptions(c))
	if err := s.Save(); err != nil {
		removeLoginSession(validation.tokenHash)
		clearSessionCookie(c)
		return loginSessionStatus{}, false, loginSessionReasonWriteFailed, err
	}
	return loginSessionStatusFromRecord(record), true, "", nil
}

func GetLoginUser(c *gin.Context) string {
	validation := validateLoginSession(c)
	if !validation.valid() {
		clearSessionCookie(c)
		return ""
	}
	return validation.userName
}

func IsLogin(c *gin.Context) bool {
	validation := validateLoginSession(c)
	if validation.valid() {
		return true
	}
	// Delete expired, legacy, or otherwise incompatible cookies as soon as they
	// are presented. Authentication has already failed even if deletion cannot
	// be written back to the browser.
	clearSessionCookie(c)
	return false
}

func ClearSession(c *gin.Context) {
	if c != nil {
		s := sessions.Default(c)
		if token, _ := s.Get(loginSessionToken).(string); token != "" {
			if tokenHash, ok := loginSessionTokenHash(token); ok {
				removeLoginSession(tokenHash)
			}
		}
	}
	clearSessionCookie(c)
}

// InvalidateAllLoginSessions is called only for a full panel restart. TLS
// listener reloads deliberately do not call it, so certificate changes do not
// become authentication changes.
func InvalidateAllLoginSessions(reason string) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "panel_restart"
	}

	loginSessionRegistry.Lock()
	memoryCount := len(loginSessionRegistry.sessions)
	loginSessionRegistry.sessions = make(map[string]loginSessionRecord)
	loginSessionRegistry.epoch = newLoginSessionEpoch()
	persistedCount, err := clearPersistedLoginSessionsLocked()
	loginSessionRegistry.Unlock()
	if err != nil {
		logger.Warningf("clear persisted login sessions failed: reason=%s error=%v", reason, err)
	}
	logger.Infof("login session live state reset: reason=%s memory=%d database=%d", reason, memoryCount, persistedCount)
}

func clearSessionCookie(c *gin.Context) {
	if c == nil {
		return
	}
	s := sessions.Default(c)
	s.Clear()
	options := sessionCookieOptions(c)
	options.MaxAge = -1
	s.Options(options)
	_ = s.Save()
}

// ClearLegacyLoginSessionCookie removes the pre-instance-scoped Cookie name
// when a browser still sends it. The Web server owns the current cookie name,
// so this exact legacy name can be deleted without touching another current
// panel instance's kwor_<instance> Cookie.
func ClearLegacyLoginSessionCookie(c *gin.Context) {
	if c == nil || c.Request == nil {
		return
	}
	if _, err := c.Request.Cookie(legacyLoginSessionCookieName); err != nil {
		return
	}
	options := sessionCookieOptions(c)
	http.SetCookie(c.Writer, &http.Cookie{
		Name:     legacyLoginSessionCookieName,
		Value:    "",
		Path:     options.Path,
		MaxAge:   -1,
		HttpOnly: options.HttpOnly,
		Secure:   options.Secure,
		SameSite: options.SameSite,
		Expires:  time.Unix(1, 0),
	})
}

func sessionCookieOptions(c *gin.Context) sessions.Options {
	return sessions.Options{
		Path:     "/",
		MaxAge:   loginSessionCookieMaxAge,
		Secure:   !isLocalDebugHTTPRequest(c),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
	}
}

// SessionCookieCodecMaxAgeSeconds bounds securecookie decoding to the same
// ten-minute page-presence lifetime enforced by the browser cookie.
func SessionCookieCodecMaxAgeSeconds() int {
	return loginSessionCookieMaxAge
}

func validateLoginSession(c *gin.Context) loginSessionValidation {
	if c == nil {
		return loginSessionValidation{reason: loginSessionReasonMissing}
	}

	s := sessions.Default(c)
	token, tokenOK := s.Get(loginSessionToken).(string)
	tokenHash, tokenValid := loginSessionTokenHash(token)
	if !tokenOK || !tokenValid {
		return loginSessionValidation{reason: loginSessionReasonMissing}
	}

	now := loginSessionNow()
	loginSessionRegistry.Lock()
	record, _, reason, err := loadLoginSessionLocked(tokenHash, now)
	loginSessionRegistry.Unlock()
	if err != nil {
		return loginSessionValidation{reason: loginSessionReasonStoreFailed, err: err}
	}
	if reason != "" {
		return loginSessionValidation{reason: reason}
	}
	return loginSessionValidation{
		token:     token,
		tokenHash: tokenHash,
		userName:  record.userName,
	}
}

func loginSessionExpiry(now time.Time) time.Time {
	return now.Add(loginSessionLifetime)
}

func loginSessionIdleDeadline(lastActivityAt time.Time, idleLimitMinutes int) time.Time {
	return lastActivityAt.Add(time.Duration(idleLimitMinutes) * time.Minute)
}

func normalizeLoginSessionIdleLimit(value int) int {
	if value <= 0 {
		return 0
	}
	if value > maxLoginSessionIdleLimitMinutes {
		return maxLoginSessionIdleLimitMinutes
	}
	return value
}

func loginSessionStatusFromRecord(record loginSessionRecord) loginSessionStatus {
	if record.idleLimitMinutes <= 0 {
		return loginSessionStatus{}
	}
	return loginSessionStatus{
		IdleDeadlineAt: loginSessionIdleDeadline(record.lastActivityAt, record.idleLimitMinutes).Unix(),
	}
}

func currentLoginSessionVersion() string {
	version := strings.TrimSpace(loginSessionVersionFn())
	if version == "" {
		return loginSessionVersionFallback
	}
	return version
}

func newLoginSessionToken() (string, error) {
	bytes := make([]byte, loginSessionTokenBytes)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(bytes), nil
}

func loginSessionTokenHash(token string) (string, bool) {
	bytes, err := base64.RawURLEncoding.DecodeString(strings.TrimSpace(token))
	if err != nil || len(bytes) != loginSessionTokenBytes {
		return "", false
	}
	sum := sha256.Sum256(bytes)
	return hex.EncodeToString(sum[:]), true
}

func newLoginSessionEpoch() string {
	epoch, err := newLoginSessionToken()
	if err == nil && epoch != "" {
		return epoch
	}
	return fmt.Sprintf("fallback-%d", time.Now().UnixNano())
}

func storeLoginSession(tokenHash string, userName string, version string, expiresAt time.Time, lastActivityAt time.Time, idleLimitMinutes int) (loginSessionRecord, error) {
	loginSessionRegistry.Lock()
	defer loginSessionRegistry.Unlock()
	now := loginSessionNow()
	pruneExpiredLoginSessionsLocked(now)
	// A freed memory slot belongs to an already-valid overflow session before a
	// brand-new login is placed. This keeps the hot set at its fixed capacity
	// and prevents a released slot from being needlessly left idle in SQLite.
	if err := refillInMemoryLoginSessionsLocked(now); err != nil {
		logger.Warning("refill in-memory login sessions before login failed: ", err)
	}
	record := loginSessionRecord{
		userName:         userName,
		version:          version,
		epoch:            loginSessionRegistry.epoch,
		expiresAt:        expiresAt,
		lastActivityAt:   lastActivityAt,
		idleLimitMinutes: idleLimitMinutes,
	}
	if len(loginSessionRegistry.sessions) < maxInMemoryLoginSessions {
		loginSessionRegistry.sessions[tokenHash] = record
		return record, nil
	}
	if err := savePersistedLoginSessionLocked(tokenHash, record); err != nil {
		return loginSessionRecord{}, err
	}
	return record, nil
}

func refreshLoginSession(tokenHash string, expiresAt time.Time, now time.Time, userActivity bool) (loginSessionRecord, bool, string, error) {
	loginSessionRegistry.Lock()
	defer loginSessionRegistry.Unlock()
	record, storage, reason, err := loadLoginSessionLocked(tokenHash, now)
	if err != nil {
		return loginSessionRecord{}, false, loginSessionReasonStoreFailed, err
	}
	if reason != "" {
		return loginSessionRecord{}, false, reason, nil
	}

	record.expiresAt = expiresAt
	if userActivity {
		record.lastActivityAt = now
	}
	if storage == loginSessionStorageMemory {
		loginSessionRegistry.sessions[tokenHash] = record
	} else if err := updatePersistedLoginSessionLocked(tokenHash, record); err != nil {
		return loginSessionRecord{}, false, loginSessionReasonStoreFailed, err
	}
	return record, true, "", nil
}

func loadLoginSessionLocked(tokenHash string, now time.Time) (loginSessionRecord, loginSessionStorage, string, error) {
	if record, exists := loginSessionRegistry.sessions[tokenHash]; exists {
		if reason := loginSessionRecordReasonLocked(record, now); reason != "" {
			delete(loginSessionRegistry.sessions, tokenHash)
			if err := refillInMemoryLoginSessionsLocked(now); err != nil {
				logger.Warning("refill in-memory login sessions failed: ", err)
			}
			return loginSessionRecord{}, loginSessionStorageMemory, reason, nil
		}
		return record, loginSessionStorageMemory, "", nil
	}

	db := database.GetDB()
	if db == nil {
		return loginSessionRecord{}, loginSessionStorageDatabase, loginSessionReasonLiveMissing, nil
	}
	var persisted model.LoginSession
	err := db.Where("token_hash = ?", tokenHash).Take(&persisted).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return loginSessionRecord{}, loginSessionStorageDatabase, loginSessionReasonLiveMissing, nil
		}
		return loginSessionRecord{}, loginSessionStorageDatabase, loginSessionReasonStoreFailed, err
	}
	record := loginSessionRecordFromModel(persisted)
	if reason := loginSessionRecordReasonLocked(record, now); reason != "" {
		if err := deletePersistedLoginSessionLocked(tokenHash); err != nil {
			return loginSessionRecord{}, loginSessionStorageDatabase, loginSessionReasonStoreFailed, err
		}
		return loginSessionRecord{}, loginSessionStorageDatabase, reason, nil
	}
	if len(loginSessionRegistry.sessions) < maxInMemoryLoginSessions {
		if err := deletePersistedLoginSessionLocked(tokenHash); err == nil {
			loginSessionRegistry.sessions[tokenHash] = record
			return record, loginSessionStorageMemory, "", nil
		} else {
			logger.Warning("promote login session into memory failed: ", err)
		}
	}
	return record, loginSessionStorageDatabase, "", nil
}

func loginSessionRecordReasonLocked(record loginSessionRecord, now time.Time) string {
	if strings.TrimSpace(record.userName) == "" {
		return loginSessionReasonLiveChanged
	}
	if record.version != currentLoginSessionVersion() {
		return loginSessionReasonVersion
	}
	if record.epoch != loginSessionRegistry.epoch {
		return loginSessionReasonRestarted
	}
	if !record.expiresAt.After(now) {
		return loginSessionReasonPageInactive
	}
	if record.idleLimitMinutes > 0 && !loginSessionIdleDeadline(record.lastActivityAt, record.idleLimitMinutes).After(now) {
		return loginSessionReasonUserIdle
	}
	return ""
}

func removeLoginSession(tokenHash string) {
	if tokenHash == "" {
		return
	}
	loginSessionRegistry.Lock()
	defer loginSessionRegistry.Unlock()
	delete(loginSessionRegistry.sessions, tokenHash)
	if err := deletePersistedLoginSessionLocked(tokenHash); err != nil {
		logger.Warning("delete persisted login session failed: ", err)
		return
	}
	if err := refillInMemoryLoginSessionsLocked(loginSessionNow()); err != nil {
		logger.Warning("refill in-memory login sessions failed: ", err)
	}
}

func pruneExpiredLoginSessionsLocked(now time.Time) {
	for tokenHash, record := range loginSessionRegistry.sessions {
		if loginSessionRecordReasonLocked(record, now) != "" {
			delete(loginSessionRegistry.sessions, tokenHash)
		}
	}
}

func savePersistedLoginSessionLocked(tokenHash string, record loginSessionRecord) error {
	db := database.GetDB()
	if db == nil {
		return fmt.Errorf("login session database is unavailable")
	}
	return db.Create(loginSessionModel(tokenHash, record)).Error
}

func updatePersistedLoginSessionLocked(tokenHash string, record loginSessionRecord) error {
	db := database.GetDB()
	if db == nil {
		return fmt.Errorf("login session database is unavailable")
	}
	result := db.Model(&model.LoginSession{}).Where("token_hash = ?", tokenHash).Updates(map[string]interface{}{
		"expires_at":       record.expiresAt,
		"last_activity_at": record.lastActivityAt,
	})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected != 0 {
		return nil
	}

	// SQLite can report zero changed rows when a second-resolution refresh
	// writes identical values. Verify existence before treating that as a
	// concurrent logout or restart, otherwise a valid database overflow session
	// could be rejected only because its refresh landed in the same second.
	var count int64
	if err := db.Model(&model.LoginSession{}).Where("token_hash = ?", tokenHash).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return fmt.Errorf("login session no longer exists")
	}
	return nil
}

func deletePersistedLoginSessionLocked(tokenHash string) error {
	db := database.GetDB()
	if db == nil {
		return nil
	}
	return db.Where("token_hash = ?", tokenHash).Delete(&model.LoginSession{}).Error
}

func clearPersistedLoginSessionsLocked() (int64, error) {
	db := database.GetDB()
	if db == nil {
		return 0, nil
	}
	var count int64
	if err := db.Model(&model.LoginSession{}).Count(&count).Error; err != nil {
		return 0, err
	}
	if err := db.Where("1 = 1").Delete(&model.LoginSession{}).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func prunePersistedLoginSessionsLocked(now time.Time) error {
	db := database.GetDB()
	if db == nil {
		return nil
	}
	if err := db.Where("expires_at <= ? OR version <> ? OR epoch <> ?", now, currentLoginSessionVersion(), loginSessionRegistry.epoch).
		Delete(&model.LoginSession{}).Error; err != nil {
		return err
	}
	return db.Where("idle_limit_minutes > 0 AND julianday(last_activity_at) <= julianday(?) - (idle_limit_minutes / 1440.0)", now).
		Delete(&model.LoginSession{}).Error
}

func refillInMemoryLoginSessionsLocked(now time.Time) error {
	db := database.GetDB()
	if db == nil {
		return nil
	}
	for len(loginSessionRegistry.sessions) < maxInMemoryLoginSessions {
		var candidate model.LoginSession
		err := db.Order("updated_at DESC").Take(&candidate).Error
		if err != nil {
			if err == gorm.ErrRecordNotFound {
				return nil
			}
			return err
		}
		record := loginSessionRecordFromModel(candidate)
		if loginSessionRecordReasonLocked(record, now) != "" {
			if err := db.Where("token_hash = ?", candidate.TokenHash).Delete(&model.LoginSession{}).Error; err != nil {
				return err
			}
			continue
		}
		if err := db.Where("token_hash = ?", candidate.TokenHash).Delete(&model.LoginSession{}).Error; err != nil {
			return err
		}
		loginSessionRegistry.sessions[candidate.TokenHash] = record
	}
	return nil
}

func loginSessionModel(tokenHash string, record loginSessionRecord) *model.LoginSession {
	return &model.LoginSession{
		TokenHash:        tokenHash,
		UserName:         record.userName,
		Version:          record.version,
		Epoch:            record.epoch,
		ExpiresAt:        record.expiresAt,
		LastActivityAt:   record.lastActivityAt,
		IdleLimitMinutes: record.idleLimitMinutes,
	}
}

func loginSessionRecordFromModel(persisted model.LoginSession) loginSessionRecord {
	return loginSessionRecord{
		userName:         persisted.UserName,
		version:          persisted.Version,
		epoch:            persisted.Epoch,
		expiresAt:        persisted.ExpiresAt,
		lastActivityAt:   persisted.LastActivityAt,
		idleLimitMinutes: normalizeLoginSessionIdleLimit(persisted.IdleLimitMinutes),
	}
}
