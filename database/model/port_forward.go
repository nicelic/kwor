package model

import "time"

// PortForwardRule stores panel-managed nftables port forwarding rules.
// The rule forwards traffic that enters the local port spec to a remote IP:port
// with optional per-rule bandwidth limiting.
type PortForwardRule struct {
	Id uint `json:"id" gorm:"primaryKey;autoIncrement"`

	Name        string `json:"name"`
	Description string `json:"description" gorm:"type:text"`
	Enabled     bool   `json:"enabled"`

	// Family is the forwarded network family: ipv4, ipv6 or dual.
	Family string `json:"family" gorm:"index"`

	// Protocol is limited to tcp, udp or tcp_udp for forwarding rules.
	Protocol string `json:"protocol" gorm:"index"`

	// LocalPortMode keeps the original UI mode: single, count or range.
	LocalPortMode string `json:"localPortMode"`

	// LocalPortSpec is the normalized nftables-compatible local port expression.
	// Current UI stores either one port or a contiguous range such as "3000-3099".
	LocalPortSpec string `json:"localPortSpec" gorm:"index"`

	LocalPortStart int `json:"localPortStart"`
	LocalPortCount int `json:"localPortCount"`
	LocalPortEnd   int `json:"localPortEnd"`

	TargetIP   string `json:"targetIP"`
	TargetPort int    `json:"targetPort"`

	// RateLimitMbps <= 0 means unlimited.
	RateLimitMbps int `json:"rateLimitMbps"`

	// TrafficLimitBytes <= 0 means unlimited. The UI accepts GiB with two
	// decimal places, while persistence keeps an exact integer byte cap.
	TrafficLimitBytes int64 `json:"trafficLimitBytes"`

	// TrafficResetDay is 0 when the monthly traffic reset is disabled; otherwise
	// it is a panel-time calendar day from 1 through 31.
	TrafficResetDay int `json:"trafficResetDay"`

	// TrafficExpiryDate is empty when no expiry applies, otherwise YYYY-MM-DD
	// interpreted at 00:00 in the panel time zone.
	TrafficExpiryDate string `json:"trafficExpiryDate" gorm:"size:10"`

	UpdatedAt time.Time `json:"updatedAt"`
	CreatedAt time.Time `json:"createdAt"`
}

// PortForwardKernelForwardState is host-local runtime ownership evidence for
// forwarding sysctls. It is intentionally kept out of database backups: a
// copied database must never restore forwarding values that belonged to a
// different host.
//
// A non-empty Original value together with the corresponding Modified flag
// means this module changed that sysctl from the recorded value to enabled.
// Values are stored verbatim so restoration preserves the host's original
// newline and formatting.
type PortForwardKernelForwardState struct {
	Id uint `json:"id" gorm:"primaryKey;autoIncrement"`

	HostFingerprint string `json:"hostFingerprint" gorm:"size:128;not null;default:'';index"`

	IPv4Modified bool   `json:"ipv4Modified" gorm:"not null;default:false"`
	IPv4Original string `json:"ipv4Original" gorm:"type:text;not null;default:''"`
	IPv6Modified bool   `json:"ipv6Modified" gorm:"not null;default:false"`
	IPv6Original string `json:"ipv6Original" gorm:"type:text;not null;default:''"`

	UpdatedAt time.Time `json:"updatedAt"`
	CreatedAt time.Time `json:"createdAt"`
}
