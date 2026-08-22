package model

import "time"

const (
	// ReverseProxySettingsSingletonID is the only permitted primary key for
	// the resource-policy record.
	ReverseProxySettingsSingletonID uint64 = 1

	// These values preserve the effective per-rule limits used before zero
	// became the explicit "no additional rule limit" setting.  They are used
	// only by the one-time database migration that creates the singleton.
	ReverseProxyLegacyMaxConcurrentRequests   = 128
	ReverseProxyLegacyDNSMaxConcurrentQueries = 128
)

// ReverseProxySettings is the singleton, persisted resource policy for the
// reverse-proxy runtime.  Id is always 1.  Revision guards every rule and
// resource mutation so concurrent settings tabs cannot silently overwrite one
// another in SQLite.
type ReverseProxySettings struct {
	Id       uint64 `json:"id" gorm:"primaryKey;autoIncrement:false"`
	Revision uint64 `json:"revision" gorm:"not null;default:1"`

	ListenerConnectionLimit           int    `json:"listenerConnectionLimit" gorm:"column:listener_connection_limit;not null;default:4096"`
	GlobalHTTPMaxConcurrent           int    `json:"globalHttpMaxConcurrent" gorm:"column:global_http_max_concurrent;not null;default:4096"`
	GlobalDNSMaxConcurrent            int    `json:"globalDnsMaxConcurrent" gorm:"column:global_dns_max_concurrent;not null;default:4096"`
	HTTP2MaxConcurrentStreams         uint32 `json:"http2MaxConcurrentStreams" gorm:"column:http2_max_concurrent_streams;not null;default:250"`
	QUICMaxIncomingStreams            int64  `json:"quicMaxIncomingStreams" gorm:"column:quic_max_incoming_streams;not null;default:256"`
	DefaultUpstreamMaxIdleConnections int    `json:"defaultUpstreamMaxIdleConnections" gorm:"column:default_upstream_max_idle_connections;not null;default:32"`

	// MemoryPoolBytes is an admission ceiling for reverse-proxy cache and body
	// rewrite buffers. The 8 GiB default is intentional and is not eagerly
	// allocated when the panel starts.
	MemoryPoolBytes              int64 `json:"memoryPoolBytes" gorm:"column:memory_pool_bytes;not null;default:8589934592"`
	DefaultRuleMemoryLimitBytes  int64 `json:"defaultRuleMemoryLimitBytes" gorm:"column:default_rule_memory_limit_bytes;not null;default:402653184"`
	ResponseRewriteInputBytes    int64 `json:"responseRewriteInputBytes" gorm:"column:response_rewrite_input_bytes;not null;default:4194304"`
	ResponseRewriteOutputBytes   int64 `json:"responseRewriteOutputBytes" gorm:"column:response_rewrite_output_bytes;not null;default:8388608"`
	ResponseRewriteMaxConcurrent int   `json:"responseRewriteMaxConcurrent" gorm:"column:response_rewrite_max_concurrent;not null;default:32"`

	UpdatedAt time.Time `json:"updatedAt"`
	CreatedAt time.Time `json:"createdAt"`
}

// DefaultReverseProxySettings returns a fully populated singleton rather than
// relying on SQLite column defaults.  Database initialization and the runtime
// both use this constructor so new installations cannot drift from the panel
// defaults shown in the resource-control UI.
func DefaultReverseProxySettings() ReverseProxySettings {
	return ReverseProxySettings{
		Id:                                ReverseProxySettingsSingletonID,
		Revision:                          1,
		ListenerConnectionLimit:           4096,
		GlobalHTTPMaxConcurrent:           4096,
		GlobalDNSMaxConcurrent:            4096,
		HTTP2MaxConcurrentStreams:         250,
		QUICMaxIncomingStreams:            256,
		DefaultUpstreamMaxIdleConnections: 32,
		// This is a deliberate capacity limit, not an eager 8 GiB allocation.
		MemoryPoolBytes:              8 * 1024 * 1024 * 1024,
		DefaultRuleMemoryLimitBytes:  384 * 1024 * 1024,
		ResponseRewriteInputBytes:    4 * 1024 * 1024,
		ResponseRewriteOutputBytes:   8 * 1024 * 1024,
		ResponseRewriteMaxConcurrent: 32,
	}
}
