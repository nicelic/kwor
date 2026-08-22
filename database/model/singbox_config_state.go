package model

// SingboxConfigState tracks the revision of data that can change the default
// sing-box configuration. It deliberately excludes every Mihomo table and
// setting so the two core configuration domains remain independent.
type SingboxConfigState struct {
	Id       uint   `json:"id" gorm:"primaryKey;autoIncrement:false"`
	Revision uint64 `json:"revision" gorm:"not null;default:1"`
}
