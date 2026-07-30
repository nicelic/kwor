package model

import "time"

// LoginSession is an overflow-only runtime record. TokenHash is derived from
// the opaque browser token; the raw bearer token is never written to SQLite.
// The table is cleared on every full panel start, so it cannot restore a login
// across a process restart.
type LoginSession struct {
	TokenHash        string    `gorm:"primaryKey;size:64"`
	UserName         string    `gorm:"not null"`
	Version          string    `gorm:"not null;size:128"`
	Epoch            string    `gorm:"not null;size:128;index"`
	ExpiresAt        time.Time `gorm:"not null;index"`
	LastActivityAt   time.Time `gorm:"not null"`
	IdleLimitMinutes int       `gorm:"not null"`
	CreatedAt        time.Time
	UpdatedAt        time.Time `gorm:"index"`
}

func (LoginSession) TableName() string {
	return "login_sessions"
}
