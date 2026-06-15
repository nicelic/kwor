package model

import "time"

// InboundTrafficState keeps the last seen cumulative nftables counters for an inbound port.

// Counters are cumulative since the nftables rule was created/reset.

// We store them to calculate deltas (for Stats table) and to support client bind baselines.

type InboundTrafficState struct {
	Id        uint `json:"id" gorm:"primaryKey;autoIncrement"`
	InboundId uint `json:"inboundId" gorm:"uniqueIndex"`

	// Tag is denormalized for convenience/debug (real source of truth is inbounds.tag)
	Tag  string `json:"tag"`
	Port int    `json:"port"`

	// Nftables rule handles (for delete/update). We use 1 rule per chain per port:
	// - input:  meta l4proto {tcp,udp} th dport <port> counter
	// - output: meta l4proto {tcp,udp} th sport <port> counter
	InHandle  int `json:"inHandle"`
	OutHandle int `json:"outHandle"`

	// Port hopping support (Hysteria2): REDIRECT rule in nat/prerouting
	PortHopRange   string `json:"portHopRange"`   // stored port_hop_range for change detection
	RedirectHandle int    `json:"redirectHandle"` // nftables handle for REDIRECT rule

	// Cumulative bytes reported by nftables counters (since rule creation/reset).
	// Used to calculate deltas for inbound Stats graphs.
	InBytes  int64 `json:"inBytes"`  // input chain bytes (client -> server port) => upload
	OutBytes int64 `json:"outBytes"` // output chain bytes (server port -> client) => download

	UpdatedAt time.Time `json:"updatedAt"`
	CreatedAt time.Time `json:"createdAt"`
}

// ClientPortLimitState tracks nftables rate-limit rule state by listen port.
// LimitMbps <= 0 means no active limit and should not have nft handles.
type ClientPortLimitState struct {
	Id   uint   `json:"id" gorm:"primaryKey;autoIncrement"`
	Port int    `json:"port" gorm:"uniqueIndex"`
	Tag  string `json:"tag"`

	LimitMbps int `json:"limitMbps"`

	// One drop-over-limit rule per direction.
	InHandle  int `json:"inHandle"`
	OutHandle int `json:"outHandle"`

	UpdatedAt time.Time `json:"updatedAt"`
	CreatedAt time.Time `json:"createdAt"`
}

// ClientPortBlockState tracks nftables access-block rules by listen port.
// When present, both inbound and outbound traffic for the port are dropped.
type ClientPortBlockState struct {
	Id   uint   `json:"id" gorm:"primaryKey;autoIncrement"`
	Port int    `json:"port" gorm:"uniqueIndex"`
	Tag  string `json:"tag"`
	// PortRanges tracks all blocked inbound-facing ports for this logical listen port.
	// JSON format: [{ "start": 21000, "end": 25000 }, { "start": 31100, "end": 31100 }]
	PortRanges string `json:"portRanges" gorm:"type:text"`

	InHandle  int `json:"inHandle"`
	OutHandle int `json:"outHandle"`

	UpdatedAt time.Time `json:"updatedAt"`
	CreatedAt time.Time `json:"createdAt"`
}

// MihomoClientPortLimitState tracks mihomo nftables rate-limit rule state by listen port.
// LimitMbps <= 0 means no active limit and should not have nft handles.
type MihomoClientPortLimitState struct {
	Id   uint   `json:"id" gorm:"primaryKey;autoIncrement"`
	Port int    `json:"port" gorm:"uniqueIndex"`
	Tag  string `json:"tag"`

	LimitMbps int `json:"limitMbps"`

	// One drop-over-limit rule per direction.
	InHandle  int `json:"inHandle"`
	OutHandle int `json:"outHandle"`

	UpdatedAt time.Time `json:"updatedAt"`
	CreatedAt time.Time `json:"createdAt"`
}

// MihomoClientPortBlockState tracks mihomo nftables access-block rules by port.
type MihomoClientPortBlockState struct {
	Id   uint   `json:"id" gorm:"primaryKey;autoIncrement"`
	Port int    `json:"port" gorm:"uniqueIndex"`
	Tag  string `json:"tag"`
	// PortRanges tracks all blocked inbound-facing ports for this logical listen port.
	PortRanges string `json:"portRanges" gorm:"type:text"`

	InHandle  int `json:"inHandle"`
	OutHandle int `json:"outHandle"`

	UpdatedAt time.Time `json:"updatedAt"`
	CreatedAt time.Time `json:"createdAt"`
}

// ClientInboundTrafficState stores per-client baseline/accumulator for an inbound.

// It is used to ensure:

// - binding an inbound to a new client starts from 0 (no historical residue)

// - unbinding stops counting

// - re-binding can start from 0 again (by resetting baseline + accumulators)

type ClientInboundTrafficState struct {
	Id        uint `json:"id" gorm:"primaryKey;autoIncrement"`
	ClientId  uint `json:"clientId" gorm:"uniqueIndex:idx_client_inbound"`
	InboundId uint `json:"inboundId" gorm:"uniqueIndex:idx_client_inbound"`
	Active    bool `json:"active" gorm:"index"`

	// IMPORTANT: This is the key to "no residue".
	// When a client binds an inbound, we snapshot the CURRENT nftables cumulative bytes into Last*.
	// On each poll, we only count (current - last) and then update last=current.
	// So a new client binding to an old inbound always starts from 0.
	LastInBytes  int64 `json:"lastInBytes"`
	LastOutBytes int64 `json:"lastOutBytes"`

	// Accumulated bytes while this binding is active (since last bind).
	UsedInBytes  int64 `json:"usedInBytes"`
	UsedOutBytes int64 `json:"usedOutBytes"`

	UpdatedAt time.Time `json:"updatedAt"`
	CreatedAt time.Time `json:"createdAt"`
}

// MihomoInboundRedirectState keeps nftables state for mihomo inbounds.
// It tracks both:
// - Counter rule handles/cumulative bytes (for traffic accounting)
// - REDIRECT handle (for port hopping)
//
// The type name is kept for backward compatibility with existing DB backups.
type MihomoInboundRedirectState struct {
	Id        uint `json:"id" gorm:"primaryKey;autoIncrement"`
	InboundId uint `json:"inboundId" gorm:"uniqueIndex"`

	Tag          string `json:"tag"`
	Port         int    `json:"port"`
	PortHopRange string `json:"portHopRange"`

	InHandle  int `json:"inHandle"`
	OutHandle int `json:"outHandle"`

	RedirectHandle int   `json:"redirectHandle"`
	InBytes        int64 `json:"inBytes"`
	OutBytes       int64 `json:"outBytes"`

	UpdatedAt time.Time `json:"updatedAt"`
	CreatedAt time.Time `json:"createdAt"`
}

// MihomoClientInboundTrafficState stores per-client baseline/accumulator for mihomo.
// Semantics match ClientInboundTrafficState in the default namespace.
type MihomoClientInboundTrafficState struct {
	Id        uint `json:"id" gorm:"primaryKey;autoIncrement"`
	ClientId  uint `json:"clientId" gorm:"uniqueIndex:idx_mihomo_client_inbound"`
	InboundId uint `json:"inboundId" gorm:"uniqueIndex:idx_mihomo_client_inbound"`
	Active    bool `json:"active" gorm:"index"`

	LastInBytes  int64 `json:"lastInBytes"`
	LastOutBytes int64 `json:"lastOutBytes"`

	UsedInBytes  int64 `json:"usedInBytes"`
	UsedOutBytes int64 `json:"usedOutBytes"`

	UpdatedAt time.Time `json:"updatedAt"`
	CreatedAt time.Time `json:"createdAt"`
}

// PortForwardLimitState stores the latest runtime limit result for a forwarding rule.
// The configured value remains on PortForwardRule.RateLimitMbps.
type PortForwardLimitState struct {
	Id     uint `json:"id" gorm:"primaryKey;autoIncrement"`
	RuleId uint `json:"ruleId" gorm:"uniqueIndex"`

	EffectiveRateLimitMbps int    `json:"effectiveRateLimitMbps"`
	Status                 string `json:"status"`
	Warning                string `json:"warning" gorm:"type:text"`

	UpdatedAt time.Time `json:"updatedAt"`
	CreatedAt time.Time `json:"createdAt"`
}
