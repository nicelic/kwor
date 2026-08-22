package model

import "time"

// ReverseProxyRule stores panel-managed reverse proxy rules.
// Runtime matching order follows ListOrder first, then database ID.
type ReverseProxyRule struct {
	Id uint `json:"id" gorm:"primaryKey;autoIncrement"`

	DisplayID uint64 `json:"displayId" gorm:"column:display_id;not null;default:0;index"`
	ListOrder int64  `json:"listOrder" gorm:"column:list_order;not null;default:0;index"`

	Name    string `json:"name" gorm:"size:255;not null;default:''"`
	Enabled bool   `json:"enabled" gorm:"not null;default:true"`

	ListenProtocol      string `json:"listenProtocol" gorm:"size:16;not null;default:'http';index"`
	ListenProtocolAlias string `json:"listenProtocolAlias" gorm:"column:listen_protocol_alias;size:16;not null;default:''"`
	ListenPort          int    `json:"listenPort" gorm:"not null;default:0;index"`
	// Compression settings are scoped independently to the local response
	// negotiation and the target request header. The algorithm lists are JSON
	// arrays stored as text so old SQLite rows can be upgraded in place.
	ListenCompressionEnabled    bool   `json:"listenCompressionEnabled" gorm:"column:listen_compression_enabled;not null;default:true"`
	ListenCompressionAlgorithms string `json:"listenCompressionAlgorithms" gorm:"column:listen_compression_algorithms;type:text;not null;default:''"`

	HostList      string `json:"hostList" gorm:"type:text;not null;default:''"`
	PathPrefix    string `json:"pathPrefix" gorm:"size:1024;not null;default:'/'"`
	ListenDNSPath string `json:"listenDnsPath" gorm:"column:listen_dns_path;size:1024;not null;default:''"`

	TargetProtocol              string `json:"targetProtocol" gorm:"size:16;not null;default:'http'"`
	TargetProtocolAlias         string `json:"targetProtocolAlias" gorm:"column:target_protocol_alias;size:16;not null;default:''"`
	TargetAddresses             string `json:"targetAddresses" gorm:"type:text;not null;default:''"`
	TargetPort                  int    `json:"targetPort" gorm:"not null;default:0"`
	TargetCompressionEnabled    bool   `json:"targetCompressionEnabled" gorm:"column:target_compression_enabled;not null;default:true"`
	TargetCompressionAlgorithms string `json:"targetCompressionAlgorithms" gorm:"column:target_compression_algorithms;type:text;not null;default:''"`
	TargetPath                  string `json:"targetPath" gorm:"size:1024;not null;default:''"`
	TargetDNSPath               string `json:"targetDnsPath" gorm:"column:target_dns_path;size:1024;not null;default:''"`
	FallbackDNSUpstreams        string `json:"fallbackDnsUpstreams" gorm:"column:fallback_dns_upstreams;type:text;not null;default:''"`
	DNSUpstreamTimeoutSeconds   int    `json:"dnsUpstreamTimeoutSeconds" gorm:"column:dns_upstream_timeout_seconds;not null;default:12"`
	DNSCacheEnabled             bool   `json:"dnsCacheEnabled" gorm:"column:dns_cache_enabled;not null;default:false"`
	DNSCacheSizeBytes           int    `json:"dnsCacheSizeBytes" gorm:"column:dns_cache_size_bytes;not null;default:4194304"`
	DNSCacheMinTTL              int    `json:"dnsCacheMinTtl" gorm:"column:dns_cache_min_ttl;not null;default:0"`
	DNSCacheMaxTTL              int    `json:"dnsCacheMaxTtl" gorm:"column:dns_cache_max_ttl;not null;default:0"`
	DNSAllowedCIDRs             string `json:"dnsAllowedCidrs" gorm:"column:dns_allowed_cidrs;type:text;not null;default:''"`
	DNSRateLimitQPS             int    `json:"dnsRateLimitQps" gorm:"column:dns_rate_limit_qps;not null;default:50"`
	DNSMaxConcurrentQueries     int    `json:"dnsMaxConcurrentQueries" gorm:"column:dns_max_concurrent_queries;not null;default:128"`

	EDNSEnabled            bool   `json:"ednsEnabled" gorm:"column:edns_enabled;not null;default:false"`
	EDNSMode               string `json:"ednsMode" gorm:"column:edns_mode;size:32;not null;default:'auto'"`
	EDNSCustomIP           string `json:"ednsCustomIp" gorm:"column:edns_custom_ip;size:255;not null;default:''"`
	EDNSClientSubnetPolicy string `json:"ednsClientSubnetPolicy" gorm:"column:edns_client_subnet_policy;size:32;not null;default:'client_ip'"`
	DisableIPv4Answer      bool   `json:"disableIpv4Answer" gorm:"column:disable_ipv4_answer;not null;default:false"`
	DisableIPv6Answer      bool   `json:"disableIpv6Answer" gorm:"column:disable_ipv6_answer;not null;default:false"`

	CertificateRecordID       uint   `json:"certificateRecordId" gorm:"not null;default:0"`
	CertificateRecordList     string `json:"certificateRecordList" gorm:"column:certificate_record_list;type:text;not null;default:''"`
	ListenHTTPVersionStrategy string `json:"listenHttpVersionStrategy" gorm:"column:listen_http_version_strategy;size:32;not null;default:''"`
	IPStrategy                string `json:"ipStrategy" gorm:"size:32;not null;default:'prefer_ipv4'"`
	HTTPVersionStrategy       string `json:"httpVersionStrategy" gorm:"size:32;not null;default:''"`
	UpstreamTLSVerify         bool   `json:"upstreamTlsVerify" gorm:"not null;default:true"`
	// A zero rule-level limit deliberately means that the rule relies on the
	// panel-wide guard.  This lets operators tune a single shared ceiling
	// without silently inheriting an unrelated hard-coded per-rule cap.
	MaxConcurrentConnections   int   `json:"maxConcurrentConnections" gorm:"column:max_concurrent_connections;not null;default:0"`
	MaxConcurrentRequests      int   `json:"maxConcurrentRequests" gorm:"column:max_concurrent_requests;not null;default:0"`
	UpstreamMaxConnections     int   `json:"upstreamMaxConnections" gorm:"column:upstream_max_connections;not null;default:0"`
	UpstreamMaxIdleConnections int   `json:"upstreamMaxIdleConnections" gorm:"column:upstream_max_idle_connections;not null;default:0"`
	MemoryLimitBytes           int64 `json:"memoryLimitBytes" gorm:"column:memory_limit_bytes;not null;default:0"`
	ApiPassthrough             bool  `json:"apiPassthrough" gorm:"not null;default:false"`
	AdvertiseHTTP3             bool  `json:"advertiseHttp3" gorm:"column:advertise_http3;not null;default:false"`

	Remark        string `json:"remark" gorm:"type:text;not null;default:''"`
	LastError     string `json:"lastError" gorm:"type:text;not null;default:''"`
	RuntimeStatus string `json:"runtimeStatus" gorm:"size:64;not null;default:''"`

	UpdatedAt time.Time `json:"updatedAt"`
	CreatedAt time.Time `json:"createdAt"`
}
