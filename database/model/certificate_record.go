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

	AcmeAccountID   uint   `json:"acmeAccountId" gorm:"not null;default:0"`
	AcmeAccountName string `json:"acmeAccountName" gorm:"size:128;not null;default:''"`
	DNSAccountID    uint   `json:"dnsAccountId" gorm:"not null;default:0"`
	DNSAccountName  string `json:"dnsAccountName" gorm:"size:128;not null;default:''"`
	ApplyTarget     string `json:"applyTarget" gorm:"size:16;not null;default:''"`
	PushDir         string `json:"pushDir" gorm:"size:1024;not null;default:''"`
	PushFiles       string `json:"pushFiles" gorm:"type:text;not null;default:''"`
	Remark          string `json:"remark" gorm:"type:text;not null;default:''"`
	RenewConfig     string `json:"renewConfig" gorm:"type:text;not null;default:''"`

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

	LastIssuedAt  int64  `json:"lastIssuedAt" gorm:"not null;default:0"`
	LastRenewedAt int64  `json:"lastRenewedAt" gorm:"not null;default:0"`
	LastError     string `json:"lastError" gorm:"type:text;not null;default:''"`
	LastOutput    string `json:"lastOutput" gorm:"type:text;not null;default:''"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}
