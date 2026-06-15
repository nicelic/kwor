package model

import "time"

// SelfSignedAuthority stores a local certificate authority used to issue
// managed certificates inside the panel. It is intentionally separate from the
// TLS config table so certificate issuance leaves no residue in TLS presets.
type SelfSignedAuthority struct {
	Id uint `json:"id" gorm:"primaryKey;autoIncrement"`

	Name          string `json:"name" gorm:"size:128;not null;default:'';uniqueIndex:idx_self_signed_authority_name"`
	PlatformCode  string `json:"platformCode" gorm:"size:64;not null;default:'';index"`
	PlatformName  string `json:"platformName" gorm:"size:128;not null;default:''"`
	SubjectCN     string `json:"subjectCn" gorm:"size:255;not null;default:''"`
	Organization  string `json:"organization" gorm:"size:255;not null;default:''"`
	Department    string `json:"department" gorm:"size:255;not null;default:''"`
	Country       string `json:"country" gorm:"size:8;not null;default:''"`
	Province      string `json:"province" gorm:"size:128;not null;default:''"`
	City          string `json:"city" gorm:"size:128;not null;default:''"`
	KeyAlgorithm  string `json:"keyAlgorithm" gorm:"size:32;not null;default:''"`
	IssuerName    string `json:"issuerName" gorm:"size:255;not null;default:''"`
	IssuerOrg     string `json:"issuerOrg" gorm:"size:255;not null;default:''"`
	CAURL         string `json:"caUrl" gorm:"size:1024;not null;default:''"`
	OCSPURL       string `json:"ocspUrl" gorm:"size:1024;not null;default:''"`
	CRLURL        string `json:"crlUrl" gorm:"size:1024;not null;default:''"`
	KeyUsage      string `json:"keyUsage" gorm:"size:255;not null;default:''"`
	ExtKeyUsage   string `json:"extKeyUsage" gorm:"size:255;not null;default:''"`
	SignAlgo      string `json:"signAlgo" gorm:"size:255;not null;default:''"`
	Brand         string `json:"brand" gorm:"size:255;not null;default:''"`
	Notes         string `json:"notes" gorm:"size:2048;not null;default:''"`
	Builtin       bool   `json:"builtin" gorm:"not null;default:false"`
	RootCertPEM   []byte `json:"-" gorm:"column:root_cert_pem;type:blob;not null"`
	IssuerCertPEM []byte `json:"-" gorm:"column:issuer_cert_pem;type:blob;not null"`
	IssuerKeyPEM  []byte `json:"-" gorm:"column:issuer_key_pem;type:blob;not null"`

	RootFingerprint   string `json:"rootFingerprint" gorm:"size:128;not null;default:''"`
	IssuerFingerprint string `json:"issuerFingerprint" gorm:"size:128;not null;default:''"`
	NotBefore         int64  `json:"notBefore" gorm:"not null;default:0"`
	NotAfter          int64  `json:"notAfter" gorm:"not null;default:0"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}
