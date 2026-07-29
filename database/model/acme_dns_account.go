package model

import "time"

type AcmeDNSAccount struct {
	Id uint `json:"id" gorm:"primaryKey;autoIncrement"`

	// DisplayID is the recyclable number rendered as dns_N in the UI. Id stays
	// as the permanent relation key used by certificate records.
	DisplayID uint64 `json:"displayId" gorm:"column:display_id;not null;default:0"`

	Name         string `json:"name" gorm:"size:128;not null;default:''"`
	ProviderName string `json:"providerName" gorm:"size:128;not null;default:''"`
	ProviderCode string `json:"providerCode" gorm:"size:64;not null;default:''"`
	EnvJSON      string `json:"envJson" gorm:"type:text;not null;default:'{}'"`
	Remark       string `json:"remark" gorm:"type:text;not null;default:''"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}
