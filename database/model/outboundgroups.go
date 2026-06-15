package model

import "time"

// OutboundGroup stores outbound groups imported from subscription links.
type OutboundGroup struct {
	Id              uint      `json:"id" gorm:"primaryKey;autoIncrement"`
	Name            string    `json:"name" gorm:"unique;not null"`
	SortOrder       int       `json:"sort_order" gorm:"default:0;index"`
	Outbounds       string    `json:"outbounds" gorm:"type:text"`          // JSON array string of outbound tags
	SubscriptionUrl string    `json:"subscription_url" gorm:"type:text"`   // Optional subscription URL
	AllowInsecure   bool      `json:"allow_insecure" gorm:"default:false"` // Allow insecure HTTPS
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

func (OutboundGroup) TableName() string {
	return "outbound_groups"
}
