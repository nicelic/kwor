package model

import "time"

// DDNSAccount stores authentication credentials for DNS providers (Cloudflare, Aliyun, Tencent, Webhook, etc.)
type DDNSAccount struct {
	Id           uint      `json:"id" gorm:"primaryKey;autoIncrement"`
	DisplayID    uint64    `json:"displayId" gorm:"column:display_id;not null;default:0;index"`
	Name         string    `json:"name" gorm:"size:128;not null;default:''"`
	ProviderCode string    `json:"providerCode" gorm:"size:64;not null;default:''"` // cloudflare, aliyun, tencent, webhook
	EnvJSON      string    `json:"envJson" gorm:"type:text;not null;default:'{}'"`   // credentials key-value pairs
	Remark       string    `json:"remark" gorm:"type:text;not null;default:''"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

// DDNSRule represents a dynamic DNS resolution rule
type DDNSRule struct {
	Id               uint       `json:"id" gorm:"primaryKey;autoIncrement"`
	DisplayID        uint64     `json:"displayId" gorm:"column:display_id;not null;default:0;index"`
	ListOrder        int64      `json:"listOrder" gorm:"column:list_order;not null;default:0;index"`
	Name             string     `json:"name" gorm:"size:255;not null;default:''"`
	Enabled          bool       `json:"enabled" gorm:"not null;default:true"`
	AccountID        uint       `json:"accountId" gorm:"column:account_id;not null;default:0;index"`
	Domains          string     `json:"domains" gorm:"type:text;not null;default:''"` // comma or newline separated domain list
	IPType           string     `json:"ipType" gorm:"column:ip_type;size:16;not null;default:'ipv4'"` // ipv4, ipv6, dual

	// IPv4 Settings
	IPV4Source       string     `json:"ipv4Source" gorm:"column:ip_v4_source;size:32;not null;default:'api'"` // api, interface, disabled
	IPV4URL          string     `json:"ipv4Url" gorm:"column:ip_v4_url;size:512;not null;default:''"`
	IPV4Interface    string     `json:"ipv4Interface" gorm:"column:ip_v4_interface;size:64;not null;default:''"`

	// IPv6 Settings
	IPV6Source       string     `json:"ipv6Source" gorm:"column:ip_v6_source;size:32;not null;default:'disabled'"` // api, interface, disabled
	IPV6URL          string     `json:"ipv6Url" gorm:"column:ip_v6_url;size:512;not null;default:''"`
	IPV6Interface    string     `json:"ipv6Interface" gorm:"column:ip_v6_interface;size:64;not null;default:''"`

	IntervalMinutes       int        `json:"intervalMinutes" gorm:"column:interval_minutes;not null;default:1"`
	LocalIntervalSeconds  int        `json:"localIntervalSeconds" gorm:"column:local_interval_seconds;not null;default:10"`
	TTL                   int        `json:"ttl" gorm:"column:ttl;not null;default:60"`
	CloudflareProxy  bool       `json:"cloudflareProxy" gorm:"column:cloudflare_proxy;not null;default:false"`

	// Runtime status fields
	LastIPV4         string     `json:"lastIpv4" gorm:"column:last_ip_v4;size:64;not null;default:''"`
	LastIPV6         string     `json:"lastIpv6" gorm:"column:last_ip_v6;size:128;not null;default:''"`
	LastSyncTime     *time.Time `json:"lastSyncTime" gorm:"column:last_sync_time"`
	LastStatus       string     `json:"lastStatus" gorm:"column:last_status;size:32;not null;default:'pending'"` // success, error, pending, syncing
	LastError        string     `json:"lastError" gorm:"column:last_error;type:text;not null;default:''"`

	CreatedAt        time.Time  `json:"createdAt"`
	UpdatedAt        time.Time  `json:"updatedAt"`
}

// DNSDomainFavorite stores commonly used root domains and preferences in ddns.db
type DNSDomainFavorite struct {
	Id        uint      `json:"id" gorm:"primaryKey;autoIncrement"`
	AccountID uint      `json:"accountId" gorm:"column:account_id;not null;default:0;index"`
	Domain    string    `json:"domain" gorm:"size:255;not null;default:'';index"`
	Remark    string    `json:"remark" gorm:"size:255;not null;default:''"`
	IsDefault bool      `json:"isDefault" gorm:"column:is_default;not null;default:false"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}

// DNSAccount stores authentication credentials exclusively for cloud DNS management (isolated from DDNS)
type DNSAccount struct {
	Id           uint      `json:"id" gorm:"primaryKey;autoIncrement"`
	DisplayID    uint64    `json:"displayId" gorm:"column:display_id;not null;default:0;index"`
	Name         string    `json:"name" gorm:"size:128;not null;default:''"`
	ProviderCode string    `json:"providerCode" gorm:"size:64;not null;default:''"` // cloudflare, aliyun, tencent, spaceship, godaddy, porkbun
	EnvJSON      string    `json:"envJson" gorm:"type:text;not null;default:'{}'"`   // credentials key-value pairs
	Remark       string    `json:"remark" gorm:"type:text;not null;default:''"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}
