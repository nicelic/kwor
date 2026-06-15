package model

import (
	"time"
)

// SubGroup stores subscription-manager groups.
type SubGroup struct {
	Id                      uint      `json:"id" gorm:"primaryKey;autoIncrement"`
	Name                    string    `json:"name" gorm:"unique;not null"`
	SortOrder               int       `json:"sort_order" gorm:"default:0;index"`
	Outbounds               string    `json:"outbounds" gorm:"type:text"`
	SubscriptionUrl         string    `json:"subscription_url" gorm:"type:text"`
	SubscriptionUrlClash    string    `json:"subscription_url_clash" gorm:"type:text"`
	AllowInsecure           bool      `json:"allow_insecure" gorm:"default:false"`
	AutoUpdateLastAt        int64     `json:"auto_update_last_at" gorm:"default:0"`
	AutoUpdateFailedSources string    `json:"auto_update_failed_sources" gorm:"type:text"`
	AutoUpdateError         string    `json:"auto_update_error" gorm:"type:text"`
	CreatedAt               time.Time `json:"created_at"`
	UpdatedAt               time.Time `json:"updated_at"`
}

func (SubGroup) TableName() string {
	return "sub_groups"
}
