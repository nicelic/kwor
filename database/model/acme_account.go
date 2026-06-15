package model

import "time"

type AcmeAccount struct {
	Id uint `json:"id" gorm:"primaryKey;autoIncrement"`

	Name      string `json:"name" gorm:"size:128;not null;default:''"`
	Email     string `json:"email" gorm:"size:255;not null;default:''"`
	Server    string `json:"server" gorm:"size:512;not null;default:''"`
	KeyLength string `json:"keyLength" gorm:"size:32;not null;default:'ec-256'"`
	Remark    string `json:"remark" gorm:"type:text;not null;default:''"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}
