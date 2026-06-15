package service

import (
	"encoding/json"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"gorm.io/gorm"
)

func TestClientRateLimitCollectDesiredPortLimits(t *testing.T) {
	t.Run("multiple inbounds and finite-only min aggregation", func(t *testing.T) {
		db := initClientLimitTestDB(t, "client-limit-collect.db")
		createDefaultInbound(t, db, 1, "hy2-a", map[string]interface{}{
			"listen_port":    30000,
			"port_hop_range": "30010-30012",
		})
		createDefaultInbound(t, db, 2, "mieru-b", map[string]interface{}{
			"listen_port":   31000,
			"port_bindings": "31010,31020-31021",
		})

		mustCreateDefaultClient(t, db, model.Client{
			Enable:         true,
			Name:           "limited-a",
			Inbounds:       mustRawJSONForLimitTest(t, []uint{1, 2}),
			SpeedLimitMbps: 200,
		})
		mustCreateDefaultClient(t, db, model.Client{
			Enable:         true,
			Name:           "limited-b",
			Inbounds:       mustRawJSONForLimitTest(t, []uint{1}),
			SpeedLimitMbps: 100,
		})
		mustCreateDefaultClient(t, db, model.Client{
			Enable:         true,
			Name:           "unlimited-c",
			Inbounds:       mustRawJSONForLimitTest(t, []uint{1}),
			SpeedLimitMbps: 0,
		})

		desired, desiredTags, err := (&ClientRateLimitService{}).collectDesiredPortLimits(db)
		if err != nil {
			t.Fatalf("collectDesiredPortLimits failed: %v", err)
		}

		wantDesired := map[int]int{
			30000: 100,
			30010: 100,
			30011: 100,
			30012: 100,
			31000: 200,
			31010: 200,
			31020: 200,
			31021: 200,
		}
		if !reflect.DeepEqual(desired, wantDesired) {
			t.Fatalf("unexpected desired limits: got=%v want=%v", desired, wantDesired)
		}
		if desiredTags[30010] != "hy2-a" || desiredTags[31020] != "mieru-b" {
			t.Fatalf("unexpected desired tags: %#v", desiredTags)
		}
	})

	t.Run("delete user removes all finite sources from desired map", func(t *testing.T) {
		db := initClientLimitTestDB(t, "client-limit-delete.db")
		createDefaultInbound(t, db, 1, "hy2-a", map[string]interface{}{
			"listen_port":    30000,
			"port_hop_range": "30010-30011",
		})

		client := mustCreateDefaultClient(t, db, model.Client{
			Enable:         true,
			Name:           "limited-a",
			Inbounds:       mustRawJSONForLimitTest(t, []uint{1}),
			SpeedLimitMbps: 100,
		})

		svc := &ClientRateLimitService{}
		desired, _, err := svc.collectDesiredPortLimits(db)
		if err != nil {
			t.Fatalf("collectDesiredPortLimits failed: %v", err)
		}
		if len(desired) == 0 {
			t.Fatalf("expected desired limits before delete")
		}

		if err := db.Delete(&client).Error; err != nil {
			t.Fatalf("delete client failed: %v", err)
		}

		desired, _, err = svc.collectDesiredPortLimits(db)
		if err != nil {
			t.Fatalf("collectDesiredPortLimits after delete failed: %v", err)
		}
		if len(desired) != 0 {
			t.Fatalf("expected empty desired limits after delete, got %v", desired)
		}
	})

	t.Run("100 to 200 to 0 updates desired map", func(t *testing.T) {
		db := initClientLimitTestDB(t, "client-limit-update.db")
		createDefaultInbound(t, db, 1, "hy2-a", map[string]interface{}{
			"listen_port":    30000,
			"port_hop_range": "30010-30010",
		})

		client := mustCreateDefaultClient(t, db, model.Client{
			Enable:         true,
			Name:           "limited-a",
			Inbounds:       mustRawJSONForLimitTest(t, []uint{1}),
			SpeedLimitMbps: 100,
		})

		svc := &ClientRateLimitService{}
		desired, _, err := svc.collectDesiredPortLimits(db)
		if err != nil {
			t.Fatalf("collectDesiredPortLimits failed: %v", err)
		}
		if desired[30000] != 100 || desired[30010] != 100 {
			t.Fatalf("unexpected desired limits at 100: %v", desired)
		}

		if err := db.Model(&client).Update("speed_limit_mbps", 200).Error; err != nil {
			t.Fatalf("update client speed limit to 200 failed: %v", err)
		}
		desired, _, err = svc.collectDesiredPortLimits(db)
		if err != nil {
			t.Fatalf("collectDesiredPortLimits at 200 failed: %v", err)
		}
		if desired[30000] != 200 || desired[30010] != 200 {
			t.Fatalf("unexpected desired limits at 200: %v", desired)
		}

		if err := db.Model(&client).Update("speed_limit_mbps", 0).Error; err != nil {
			t.Fatalf("update client speed limit to 0 failed: %v", err)
		}
		desired, _, err = svc.collectDesiredPortLimits(db)
		if err != nil {
			t.Fatalf("collectDesiredPortLimits at 0 failed: %v", err)
		}
		if len(desired) != 0 {
			t.Fatalf("expected no desired limits at 0, got %v", desired)
		}
	})
}

func TestMihomoClientRateLimitCollectDesiredPortLimits(t *testing.T) {
	db := initClientLimitTestDB(t, "mihomo-client-limit-collect.db")
	createMihomoInbound(t, db, 1, "hy2-a", map[string]interface{}{
		"listen_port":    32000,
		"port_hop_range": "32010-32012",
	}, nil)
	createMihomoInbound(t, db, 2, "mieru-b", map[string]interface{}{
		"listen_port": 33000,
		"port_range":  "33010-33011",
	}, nil)

	mustCreateMihomoClient(t, db, model.MihomoClient{
		Enable:         true,
		Name:           "limited-a",
		Inbounds:       mustRawJSONForLimitTest(t, []uint{1, 2}),
		SpeedLimitMbps: 200,
	})
	mustCreateMihomoClient(t, db, model.MihomoClient{
		Enable:         true,
		Name:           "limited-b",
		Inbounds:       mustRawJSONForLimitTest(t, []uint{1}),
		SpeedLimitMbps: 100,
	})
	mustCreateMihomoClient(t, db, model.MihomoClient{
		Enable:         true,
		Name:           "unlimited-c",
		Inbounds:       mustRawJSONForLimitTest(t, []uint{2}),
		SpeedLimitMbps: 0,
	})

	desired, desiredTags, err := (&MihomoClientRateLimitService{}).collectDesiredPortLimits(db)
	if err != nil {
		t.Fatalf("collectDesiredPortLimits failed: %v", err)
	}

	wantDesired := map[int]int{
		32000: 100,
		32010: 100,
		32011: 100,
		32012: 100,
		33000: 200,
		33010: 200,
		33011: 200,
	}
	if !reflect.DeepEqual(desired, wantDesired) {
		t.Fatalf("unexpected desired limits: got=%v want=%v", desired, wantDesired)
	}
	if desiredTags[32010] != "hy2-a" || desiredTags[33010] != "mieru-b" {
		t.Fatalf("unexpected desired tags: %#v", desiredTags)
	}
}

func initClientLimitTestDB(t *testing.T, dbName string) *gorm.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), dbName)
	if err := database.InitDB(dbPath); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}

	db := database.GetDB()
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db.DB failed: %v", err)
	}
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})
	return db
}

func createDefaultInbound(t *testing.T, db *gorm.DB, id uint, tag string, options map[string]interface{}) {
	t.Helper()
	inbound := model.Inbound{
		Id:      id,
		Tag:     tag,
		Type:    "hysteria2",
		Options: mustRawJSONForLimitTest(t, options),
	}
	if tag == "mieru-b" {
		inbound.Type = "mieru"
	}
	if err := db.Create(&inbound).Error; err != nil {
		t.Fatalf("create inbound failed: %v", err)
	}
}

func createMihomoInbound(t *testing.T, db *gorm.DB, id uint, tag string, options map[string]interface{}, outJSON map[string]interface{}) {
	t.Helper()
	inbound := model.MihomoInbound{
		Id:      id,
		Tag:     tag,
		Type:    "hysteria2",
		Options: mustRawJSONForLimitTest(t, options),
	}
	if outJSON != nil {
		inbound.OutJson = mustRawJSONForLimitTest(t, outJSON)
	}
	if tag == "mieru-b" {
		inbound.Type = "mieru"
	}
	if err := db.Create(&inbound).Error; err != nil {
		t.Fatalf("create mihomo inbound failed: %v", err)
	}
}

func mustCreateDefaultClient(t *testing.T, db *gorm.DB, client model.Client) model.Client {
	t.Helper()
	if len(client.Links) == 0 {
		client.Links = json.RawMessage(`[]`)
	}
	if err := db.Create(&client).Error; err != nil {
		t.Fatalf("create client failed: %v", err)
	}
	return client
}

func mustCreateMihomoClient(t *testing.T, db *gorm.DB, client model.MihomoClient) model.MihomoClient {
	t.Helper()
	if len(client.Links) == 0 {
		client.Links = json.RawMessage(`[]`)
	}
	if err := db.Create(&client).Error; err != nil {
		t.Fatalf("create mihomo client failed: %v", err)
	}
	return client
}

func mustRawJSONForLimitTest(t *testing.T, value interface{}) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	return json.RawMessage(raw)
}
