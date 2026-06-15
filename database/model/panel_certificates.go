package model

type PanelCertificate struct {
	Target      string `json:"target" gorm:"primaryKey;size:32"`
	CertPEM     []byte `json:"-" gorm:"column:cert_pem;type:blob;not null"`
	KeyPEM      []byte `json:"-" gorm:"column:key_pem;type:blob;not null"`
	Fingerprint string `json:"fingerprint" gorm:"column:fingerprint;size:128;not null;default:''"`
	NotBefore   int64  `json:"notBefore" gorm:"column:not_before;not null;default:0"`
	NotAfter    int64  `json:"notAfter" gorm:"column:not_after;not null;default:0"`
	UpdatedAt   int64  `json:"updatedAt" gorm:"column:updated_at;not null;default:0"`
}
