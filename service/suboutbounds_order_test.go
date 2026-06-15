package service

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"gorm.io/gorm"
)

func TestSubOutboundGetAllOrdersDefaultManagedItemsByInboundThenClient(t *testing.T) {
	db := initSubOutboundOrderTestDB(t, "suboutbounds-default-order.db")

	firstClient := &model.Client{
		Name:     "USv6",
		Inbounds: mustSubOutboundOrderJSON(t, []uint{101, 202}),
	}
	secondClient := &model.Client{
		Name:     "USv4",
		Inbounds: mustSubOutboundOrderJSON(t, []uint{101, 202}),
	}
	if err := db.Create(firstClient).Error; err != nil {
		t.Fatalf("create first client failed: %v", err)
	}
	if err := db.Create(secondClient).Error; err != nil {
		t.Fatalf("create second client failed: %v", err)
	}

	createManagedOrderSubOutbound(t, db, "s_hy2_USv6", subOutboundSourceClient, firstClient.Id, 101)
	createManagedOrderSubOutbound(t, db, "s_hy2_tg_USv6", subOutboundSourceClient, firstClient.Id, 202)
	createManagedOrderSubOutbound(t, db, "s_hy2_USv4", subOutboundSourceClient, secondClient.Id, 101)
	createManagedOrderSubOutbound(t, db, "s_hy2_tg_USv4", subOutboundSourceClient, secondClient.Id, 202)

	items, err := (&SubOutboundService{}).GetAll()
	if err != nil {
		t.Fatalf("GetAll failed: %v", err)
	}

	got := collectSubOutboundTags(*items)
	want := []string{"s_hy2_USv6", "s_hy2_USv4", "s_hy2_tg_USv6", "s_hy2_tg_USv4"}
	assertSubOutboundTagOrder(t, got, want)
}

func TestSubOutboundGetAllOrdersMihomoManagedItemsByInboundThenClient(t *testing.T) {
	db := initSubOutboundOrderTestDB(t, "suboutbounds-mihomo-order.db")

	firstClient := &model.MihomoClient{
		Name:     "USv6",
		Inbounds: mustSubOutboundOrderJSON(t, []uint{301, 404}),
	}
	secondClient := &model.MihomoClient{
		Name:     "USv4",
		Inbounds: mustSubOutboundOrderJSON(t, []uint{301, 404}),
	}
	if err := db.Create(firstClient).Error; err != nil {
		t.Fatalf("create first mihomo client failed: %v", err)
	}
	if err := db.Create(secondClient).Error; err != nil {
		t.Fatalf("create second mihomo client failed: %v", err)
	}

	createManagedOrderSubOutbound(t, db, "m_hy2_USv6", subOutboundSourceMihomoClient, firstClient.Id, 301)
	createManagedOrderSubOutbound(t, db, "m_hy2_tg_USv6", subOutboundSourceMihomoClient, firstClient.Id, 404)
	createManagedOrderSubOutbound(t, db, "m_hy2_USv4", subOutboundSourceMihomoClient, secondClient.Id, 301)
	createManagedOrderSubOutbound(t, db, "m_hy2_tg_USv4", subOutboundSourceMihomoClient, secondClient.Id, 404)

	items, err := (&SubOutboundService{}).GetAll()
	if err != nil {
		t.Fatalf("GetAll failed: %v", err)
	}

	got := collectSubOutboundTags(*items)
	want := []string{"m_hy2_USv6", "m_hy2_USv4", "m_hy2_tg_USv6", "m_hy2_tg_USv4"}
	assertSubOutboundTagOrder(t, got, want)
}

func initSubOutboundOrderTestDB(t *testing.T, filename string) *gorm.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), filename)
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

func mustSubOutboundOrderJSON(t *testing.T, value interface{}) json.RawMessage {
	t.Helper()

	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal JSON failed: %v", err)
	}
	return data
}

func createManagedOrderSubOutbound(
	t *testing.T,
	db *gorm.DB,
	tag string,
	sourceType string,
	sourceClientID uint,
	sourceInboundID uint,
) {
	t.Helper()

	record := &model.SubOutbound{
		Type:            "trojan",
		Tag:             tag,
		SourceType:      sourceType,
		SourceClientId:  sourceClientID,
		SourceInboundId: sourceInboundID,
		RawOutbound: mustSubOutboundOrderJSON(t, map[string]interface{}{
			"type":        "trojan",
			"tag":         tag,
			"server":      "example.com",
			"server_port": 443,
			"password":    "test-pass",
		}),
	}
	if err := db.Create(record).Error; err != nil {
		t.Fatalf("create suboutbound %s failed: %v", tag, err)
	}
}

func collectSubOutboundTags(items []map[string]interface{}) []string {
	result := make([]string, 0, len(items))
	for _, item := range items {
		tag, _ := item["tag"].(string)
		result = append(result, tag)
	}
	return result
}

func assertSubOutboundTagOrder(t *testing.T, got []string, want []string) {
	t.Helper()

	if len(got) < len(want) {
		t.Fatalf("expected at least %d tags, got %v", len(want), got)
	}
	for index, tag := range want {
		if got[index] != tag {
			t.Fatalf("expected order %v, got %v", want, got)
		}
	}
}
