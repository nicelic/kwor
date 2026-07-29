package model

import "time"

type AcmeAccount struct {
	Id uint `json:"id" gorm:"primaryKey;autoIncrement"`

	// DisplayID is the user-facing, recyclable account number. The database
	// primary key remains permanent so a stale browser request can never point
	// to a newly-created account that reused the same display number.
	DisplayID uint64 `json:"displayId" gorm:"column:display_id;not null;default:0"`

	Name   string `json:"name" gorm:"size:128;not null;default:''"`
	Email  string `json:"email" gorm:"size:255;not null;default:''"`
	Server string `json:"server" gorm:"size:512;not null;default:''"`

	// KeyLength is retained only for a one-time schema transition from the old
	// account form. New code must use AccountKeyLength; certificate key length
	// belongs to CertificateRecord instead of an ACME account.
	KeyLength        string `json:"keyLength" gorm:"size:32;not null;default:'ec-256'"`
	AccountKeyLength string `json:"accountKeyLength" gorm:"column:account_key_length;size:32;not null;default:'ec-256'"`
	Remark           string `json:"remark" gorm:"type:text;not null;default:''"`

	// RuntimeState stores only acme.sh account/CA state (account key, account
	// JSON and CA config). It is reconstructed into a per-operation temporary
	// --config-home and is never returned by an API response.
	RuntimeState []byte `json:"-" gorm:"column:runtime_state;type:blob"`
	Registered   bool   `json:"registered" gorm:"not null;default:false"`
	System       bool   `json:"system" gorm:"not null;default:false"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}
