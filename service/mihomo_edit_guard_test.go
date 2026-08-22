package service

import (
	"testing"

	"github.com/alireza0/s-ui/database/model"
)

func TestMihomoEditRejectsMissingRecordsWithoutCreatingThem(t *testing.T) {
	db := initMihomoManagerFinalFallbackTestDB(t, "mihomo-edit-missing-records.db")

	tests := []struct {
		name  string
		save  func() error
		count func() (int64, error)
	}{
		{
			name: "inbound",
			save: func() error {
				_, err := (&MihomoInboundService{}).Save(db, "edit", mustJSONRaw(t, map[string]interface{}{
					"id":          9101,
					"type":        "socks",
					"tag":         "missing-inbound",
					"listen":      "::",
					"listen_port": 19101,
				}), "", "panel.example.com")
				return err
			},
			count: func() (int64, error) {
				var count int64
				err := db.Model(&model.MihomoInbound{}).Where("id = ?", 9101).Count(&count).Error
				return count, err
			},
		},
		{
			name: "outbound",
			save: func() error {
				return (&MihomoOutboundService{}).Save(db, "edit", mustJSONRaw(t, map[string]interface{}{
					"id":   9102,
					"type": "direct",
					"tag":  "missing-outbound",
				}))
			},
			count: func() (int64, error) {
				var count int64
				err := db.Model(&model.MihomoOutbound{}).Where("id = ?", 9102).Count(&count).Error
				return count, err
			},
		},
		{
			name: "tls",
			save: func() error {
				return (&MihomoTlsService{}).Save(db, "edit", mustJSONRaw(t, map[string]interface{}{
					"id":     9103,
					"name":   "missing-tls",
					"mode":   "tls",
					"server": map[string]interface{}{"enabled": true},
					"client": map[string]interface{}{"enabled": true},
				}), "panel.example.com")
			},
			count: func() (int64, error) {
				var count int64
				err := db.Model(&model.MihomoTls{}).Where("id = ?", 9103).Count(&count).Error
				return count, err
			},
		},
		{
			name: "outbound group",
			save: func() error {
				_, err := (&MihomoOutboundGroupService{}).SaveWithRuntimeImpact(db, "edit", mustJSONRaw(t, map[string]interface{}{
					"id":        9104,
					"name":      "missing-group",
					"outbounds": "[]",
				}))
				return err
			},
			count: func() (int64, error) {
				var count int64
				err := db.Model(&model.MihomoOutboundGroup{}).Where("id = ?", 9104).Count(&count).Error
				return count, err
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if err := test.save(); err == nil {
				t.Fatal("expected edit of missing record to fail")
			}
			count, err := test.count()
			if err != nil {
				t.Fatalf("count persisted records failed: %v", err)
			}
			if count != 0 {
				t.Fatalf("missing record was unexpectedly created, count=%d", count)
			}
		})
	}
}
