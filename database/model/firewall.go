package model

import "time"

// FirewallRule stores panel-managed inbound firewall rules that are mirrored
// into a dedicated nftables table when the firewall switch is enabled.
type FirewallRule struct {
	Id uint `json:"id" gorm:"primaryKey;autoIncrement"`

	Name        string `json:"name"`
	Description string `json:"description" gorm:"type:text"`
	Enabled     bool   `json:"enabled"`

	// Origin:
	// - system: reserved default rules for SSH/panel/sub ports
	// - manual: created by panel users
	// - temporary: internal temporary rules created by managed flows such as ACME
	// - external: observed from other nftables chains for panel visibility/cleanup
	Origin string `json:"origin" gorm:"index"`

	// SystemKey is only used when Origin=system.
	SystemKey string `json:"systemKey" gorm:"index"`

	// TemporaryType is only used when Origin=temporary, for example "acme".
	TemporaryType string `json:"temporaryType" gorm:"index"`

	// TemporaryExpireAt is only used when Origin=temporary and stores the
	// unix timestamp when the internal temporary rule should be cleaned up.
	TemporaryExpireAt int64 `json:"temporaryExpireAt" gorm:"index"`

	// Current implementation manages inbound rules only.
	Direction string `json:"direction"`

	// Family: dual, ipv4, ipv6
	Family string `json:"family"`

	// Protocol: tcp, udp, tcp_udp, icmp, icmp_v4, icmp_v6
	Protocol string `json:"protocol"`

	// PortSpec keeps the normalized port/range expression, e.g. "22" or "80, 443, 10000-10010".
	PortSpec string `json:"portSpec"`

	// SourceSpec keeps the normalized source IP/CIDR expression.
	SourceSpec string `json:"sourceSpec" gorm:"type:text"`

	// Observed* fields are only used when Origin=external so the panel can
	// track and optionally remove the original nftables rule. These rows are
	// not auto-applied into the managed allowlist.
	ObservedFamily  string `json:"observedFamily"`
	ObservedTable   string `json:"observedTable"`
	ObservedChain   string `json:"observedChain"`
	ObservedHandle  int    `json:"observedHandle"`
	ObservedComment string `json:"observedComment" gorm:"type:text"`
	LastSeenAt      int64  `json:"lastSeenAt"`

	UpdatedAt time.Time `json:"updatedAt"`
	CreatedAt time.Time `json:"createdAt"`
}
