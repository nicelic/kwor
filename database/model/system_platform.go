package model

// SystemPlatform stores the one host-platform snapshot collected while the
// panel starts. Runtime features read this record instead of probing the host
// repeatedly.
type SystemPlatform struct {
	Id              uint   `json:"id" gorm:"primaryKey;autoIncrement:false"`
	OS              string `json:"os" gorm:"not null"`
	Architecture    string `json:"architecture" gorm:"not null"`
	SystemID        string `json:"systemId"`
	SystemIDLike    string `json:"systemIdLike"`
	SystemFamily    string `json:"systemFamily"`
	Libc            string `json:"libc"`
	KernelRelease   string `json:"kernelRelease"`
	VersionID       string `json:"versionId"`
	VersionCodename string `json:"versionCodename"`
	DetectedAt      int64  `json:"detectedAt" gorm:"not null"`
}
