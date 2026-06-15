package model

import "time"

// ReverseProxyCertificateBalanceState stores certificate balancing counters per
// reverse-proxy listener and SNI bucket.
type ReverseProxyCertificateBalanceState struct {
	Id uint `json:"id" gorm:"primaryKey;autoIncrement"`

	ListenerKey         string `json:"listenerKey" gorm:"column:listener_key;size:128;not null;default:'';index:idx_rp_cert_balance_unique,unique"`
	SNIBucket           string `json:"sniBucket" gorm:"column:sni_bucket;size:255;not null;default:'';index:idx_rp_cert_balance_unique,unique"`
	CertificateRecordID uint   `json:"certificateRecordId" gorm:"column:certificate_record_id;not null;default:0;index:idx_rp_cert_balance_unique,unique"`

	ActiveConn     int64 `json:"activeConn" gorm:"column:active_conn;not null;default:0"`
	SelectedTotal  int64 `json:"selectedTotal" gorm:"column:selected_total;not null;default:0"`
	LastSelectedAt int64 `json:"lastSelectedAt" gorm:"column:last_selected_at;not null;default:0;index"`
	UpdatedAtUnix  int64 `json:"updatedAtUnix" gorm:"column:updated_at_unix;not null;default:0;index"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
}
