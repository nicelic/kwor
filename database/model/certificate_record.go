package model

import "time"

// CertificateRecord is the unified certificate inventory used by the
// certificate management page. It stores ACME/self-signed/imported
// certificate materials as original data for later push/apply/view actions.
type CertificateRecord struct {
	Id uint `json:"id" gorm:"primaryKey;autoIncrement"`

	DisplayID   uint64 `json:"displayId" gorm:"column:display_id;not null;default:0"`
	ListOrderAt int64  `json:"listOrderAt" gorm:"column:list_order_at;index;not null;default:0"`

	SourceType string `json:"sourceType" gorm:"size:32;not null;index:idx_certificate_source,unique"`
	SourceRef  string `json:"sourceRef" gorm:"size:255;not null;index:idx_certificate_source,unique"`

	MainDomain string `json:"mainDomain" gorm:"index;size:255;not null;default:''"`
	DomainSet  string `json:"domainSet" gorm:"type:text;not null;default:''"`

	CertificateType string `json:"certificateType" gorm:"size:32;not null;default:'domain'"`
	CertProfile     string `json:"certProfile" gorm:"size:64;not null;default:''"`
	Challenge       string `json:"challenge" gorm:"size:32;not null;default:''"`
	KeyLength       string `json:"keyLength" gorm:"size:32;not null;default:''"`
	CAServer        string `json:"caServer" gorm:"size:128;not null;default:''"`
	UseECC          bool   `json:"useEcc" gorm:"not null;default:false"`
	AutoRenew       bool   `json:"autoRenew" gorm:"not null;default:false"`
	// Auto-renew retry state is persisted so short and periodic retries survive
	// panel restarts without turning manual renew failures into background work.
	AutoRenewRetryPhase    string `json:"autoRenewRetryPhase" gorm:"size:32;not null;default:''"`
	AutoRenewRetryCount    int    `json:"autoRenewRetryCount" gorm:"not null;default:0"`
	AutoRenewNextRetryAt   int64  `json:"autoRenewNextRetryAt" gorm:"not null;default:0"`
	AutoRenewLastAttemptAt int64  `json:"autoRenewLastAttemptAt" gorm:"not null;default:0"`

	AcmeAccountID   uint   `json:"acmeAccountId" gorm:"not null;default:0"`
	AcmeAccountName string `json:"acmeAccountName" gorm:"size:128;not null;default:''"`
	DNSAccountID    uint   `json:"dnsAccountId" gorm:"not null;default:0"`
	DNSAccountName  string `json:"dnsAccountName" gorm:"size:128;not null;default:''"`
	// AcmeRuntimeProfile is empty for normal domain accounts and "ip_acme" for
	// IP certificates. The latter intentionally has no user-visible account ID.
	AcmeRuntimeProfile string `json:"acmeRuntimeProfile" gorm:"size:64;not null;default:''"`
	ApplyTarget        string `json:"applyTarget" gorm:"size:16;not null;default:''"`
	// PushEnabled is true only after the current certificate bundle has been
	// written to PushDir and its on-disk contents have been verified.
	PushEnabled bool   `json:"pushEnabled" gorm:"not null;default:false"`
	PushDir     string `json:"pushDir" gorm:"size:1024;not null;default:''"`
	// PushFilePaths stores a JSON object of fixed certificate filenames to their
	// verified full paths, for example {"fullchain.pem":"/etc/ssl/fullchain.pem"}.
	PushFilePaths string `json:"pushFilePaths" gorm:"type:text;not null;default:''"`
	// PushFiles is the retired filename-only tracking field. New directory push
	// behavior uses PushEnabled and PushFilePaths exclusively.
	PushFiles   string `json:"pushFiles" gorm:"type:text;not null;default:''"`
	Remark      string `json:"remark" gorm:"type:text;not null;default:''"`
	RenewConfig string `json:"renewConfig" gorm:"type:text;not null;default:''"`

	// Persistent binding flags are the deletion authority for managed runtime
	// consumers. They are maintained by the TLS and reverse-proxy write paths
	// so certificate deletion never needs to scan every consumer table.
	BoundByReverseProxy bool `json:"boundByReverseProxy" gorm:"column:bound_by_reverse_proxy;not null;default:false"`
	BoundBySingboxTLS   bool `json:"boundBySingboxTls" gorm:"column:bound_by_singbox_tls;not null;default:false"`
	BoundByMihomoTLS    bool `json:"boundByMihomoTls" gorm:"column:bound_by_mihomo_tls;not null;default:false"`

	AcmeHome    string `json:"acmeHome" gorm:"size:1024;not null;default:''"`
	Webroot     string `json:"webroot" gorm:"size:1024;not null;default:''"`
	DNSProvider string `json:"dnsProvider" gorm:"size:128;not null;default:''"`
	DNSEnvText  string `json:"dnsEnvText" gorm:"type:text;not null;default:''"`
	CustomArgs  string `json:"customArgs" gorm:"type:text;not null;default:''"`

	CertPath      string `json:"certPath" gorm:"size:1024;not null;default:''"`
	KeyPath       string `json:"keyPath" gorm:"size:1024;not null;default:''"`
	FullchainPath string `json:"fullchainPath" gorm:"size:1024;not null;default:''"`
	ChainPath     string `json:"chainPath" gorm:"size:1024;not null;default:''"`

	CertPEM      []byte `json:"-" gorm:"column:cert_pem;type:blob;not null"`
	KeyPEM       []byte `json:"-" gorm:"column:key_pem;type:blob;not null"`
	FullchainPEM []byte `json:"-" gorm:"column:fullchain_pem;type:blob"`
	ChainPEM     []byte `json:"-" gorm:"column:chain_pem;type:blob"`

	Fingerprint string `json:"fingerprint" gorm:"size:128;not null;default:''"`
	NotBefore   int64  `json:"notBefore" gorm:"not null;default:0"`
	NotAfter    int64  `json:"notAfter" gorm:"not null;default:0"`
	// Issued algorithm metadata is stored separately so list and selector
	// queries never need to load certificate or private-key BLOBs.
	IssuedKeyAlgorithm       string `json:"issuedKeyAlgorithm" gorm:"size:128;not null;default:''"`
	IssuedSignatureAlgorithm string `json:"issuedSignatureAlgorithm" gorm:"size:128;not null;default:''"`

	LastIssuedAt  int64  `json:"lastIssuedAt" gorm:"not null;default:0"`
	LastRenewedAt int64  `json:"lastRenewedAt" gorm:"not null;default:0"`
	LastError     string `json:"lastError" gorm:"type:text;not null;default:''"`
	// PostActionError records a non-fatal failure after certificate material has
	// already been stored, such as a directory push or runtime binding refresh.
	// It must not be folded into LastError because the certificate itself remains
	// valid and usable from the inventory.
	PostActionError string `json:"postActionError" gorm:"type:text;not null;default:''"`
	LastOutput      string `json:"lastOutput" gorm:"type:text;not null;default:''"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}
