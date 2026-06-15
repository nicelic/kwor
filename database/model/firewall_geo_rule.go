package model

import "time"

// FirewallGeoRule stores panel-managed source IP country/region rules that
// are resolved from remote rule-set files and rendered into the managed
// nftables firewall chain.
type FirewallGeoRule struct {
	Id uint `json:"id" gorm:"primaryKey;autoIncrement"`

	Name        string `json:"name"`
	Description string `json:"description" gorm:"type:text"`
	Enabled     bool   `json:"enabled"`

	// Family: dual, ipv4, ipv6
	Family string `json:"family"`

	// Protocol: tcp, udp, tcp_udp
	Protocol string `json:"protocol"`

	// PortSpec keeps the normalized port/range expression, e.g. "443" or
	// "80, 443, 10000-10010".
	PortSpec string `json:"portSpec"`

	// Action: allow, block
	Action string `json:"action" gorm:"index"`

	// CountryCode keeps the normalized geoip name used to build rule-set URLs
	// when custom URLs are not provided, e.g. "us", "jp", "private".
	CountryCode string `json:"countryCode" gorm:"index"`

	// SourceProviders stores the ordered source provider keys as JSON.
	SourceProviders string `json:"sourceProviders" gorm:"type:text"`

	// CustomSourceURLs stores the ordered explicit source URLs as JSON.
	CustomSourceURLs string `json:"customSourceUrls" gorm:"type:text"`

	// ResolvedSources stores the active URLs used by the latest successful
	// refresh. For provider-based rules this usually contains a single URL;
	// for explicit URLs it contains all successfully merged sources.
	ResolvedSources string `json:"resolvedSources" gorm:"type:text"`

	// CachedFiles stores file names under Promanager_data/geoip as JSON.
	CachedFiles string `json:"cachedFiles" gorm:"type:text"`

	// ContentHash tracks the latest successfully parsed normalized prefix set.
	ContentHash string `json:"contentHash" gorm:"index"`

	// PrefixCount stores the normalized prefix count after merge.
	PrefixCount int `json:"prefixCount"`

	// LastRefreshAt is the last successful refresh timestamp.
	LastRefreshAt int64 `json:"lastRefreshAt"`

	// LastRefreshError stores the last refresh failure message while keeping
	// the previous successful cache active.
	LastRefreshError string `json:"lastRefreshError" gorm:"type:text"`

	UpdatedAt time.Time `json:"updatedAt"`
	CreatedAt time.Time `json:"createdAt"`
}
