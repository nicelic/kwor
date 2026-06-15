package model

// SubSyncBlock records manual delete intent from Sub Manager.
// When present, auto-push sync will skip recreating that client+inbound node.
type SubSyncBlock struct {
	Id uint `json:"id" form:"id" gorm:"primaryKey;autoIncrement"`

	SourceType      string `json:"source_type" form:"source_type" gorm:"uniqueIndex:idx_sub_sync_block,priority:1"`
	SourceClientId  uint   `json:"source_client_id" form:"source_client_id" gorm:"uniqueIndex:idx_sub_sync_block,priority:2"`
	SourceInboundId uint   `json:"source_inbound_id" form:"source_inbound_id" gorm:"uniqueIndex:idx_sub_sync_block,priority:3"`
}
