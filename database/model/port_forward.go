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

	UpdatedAt time.Time `json:"updatedAt"`
	CreatedAt time.Time `json:"createdAt"`
}
