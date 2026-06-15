package service

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"gorm.io/gorm"
)

func TestBuildMihomoClashOptions_PreservesMihomoTLSFields(t *testing.T) {
	outbound := map[string]interface{}{
		"type":        "vless",
		"tag":         "node-a",
		"server":      "1.1.1.1",
		"server_port": 443,
		"uuid":        "11111111-1111-1111-1111-111111111111",
		"tls": map[string]interface{}{
			"enabled":     true,
			"server_name": "example.com",
			"utls": map[string]interface{}{
				"enabled":     true,
				"fingerprint": "chrome",
			},
			"reality": map[string]interface{}{
				"enabled":    true,
				"public_key": "pub-key",
				"short_id":   "short-id",
			},
			"ech": map[string]interface{}{
				"enabled":           true,
				"config":            []interface{}{"-----BEGIN ECH CONFIGS-----", "ABC", "-----END ECH CONFIGS-----"},
				"query_server_name": "ech.example.com",
			},
		},
		"transport": map[string]interface{}{
			"type": "ws",
			"path": "/ws",
		},
	}

	raw, err := buildMihomoClashOptions(outbound, "mihomo_node-a")
	if err != nil {
		t.Fatalf("buildMihomoClashOptions returned error: %v", err)
	}
	if len(raw) == 0 {
		t.Fatal("expected clash options to be generated")
	}

	var proxy map[string]interface{}
	if err := json.Unmarshal(raw, &proxy); err != nil {
		t.Fatalf("failed to decode clash options: %v", err)
	}

	if got, _ := proxy["name"].(string); got != "mihomo_node-a" {
		t.Fatalf("expected proxy name to be rewritten, got %#v", proxy["name"])
	}
	if got, _ := proxy["client-fingerprint"].(string); got != "chrome" {
		t.Fatalf("unexpected client-fingerprint: %#v", proxy["client-fingerprint"])
	}

	realityOpts, ok := proxy["reality-opts"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected reality-opts map, got %#v", proxy["reality-opts"])
	}
	if got, _ := realityOpts["public-key"].(string); got != "pub-key" {
		t.Fatalf("unexpected reality public-key: %#v", realityOpts["public-key"])
	}
	if got, _ := realityOpts["short-id"].(string); got != "short-id" {
		t.Fatalf("unexpected reality short-id: %#v", realityOpts["short-id"])
	}

	echOpts, ok := proxy["ech-opts"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected ech-opts map, got %#v", proxy["ech-opts"])
	}
	if got, _ := echOpts["config"].(string); got != "ABC" {
		t.Fatalf("unexpected ech config: %#v", echOpts["config"])
	}
	if got, _ := echOpts["query-server-name"].(string); got != "ech.example.com" {
		t.Fatalf("unexpected ech query-server-name: %#v", echOpts["query-server-name"])
	}
}

func TestBuildMihomoClashOptions_Hysteria2OmitsFastOpenAndUnsetBandwidth(t *testing.T) {
	outbound := map[string]interface{}{
		"type":             "hysteria2",
		"tag":              "node-hy2",
		"server":           "1.1.1.1",
		"server_port":      443,
		"password":         "pwd",
		"mihomo_fast_open": true,
	}

	raw, err := buildMihomoClashOptions(outbound, "mihomo_node-hy2")
	if err != nil {
		t.Fatalf("buildMihomoClashOptions returned error: %v", err)
	}

	var proxy map[string]interface{}
	if err := json.Unmarshal(raw, &proxy); err != nil {
		t.Fatalf("failed to decode clash options: %v", err)
	}
	if _, exists := proxy["fast-open"]; exists {
		t.Fatalf("expected hysteria2 fast-open to be omitted, got %#v", proxy["fast-open"])
	}
	if _, exists := proxy["up"]; exists {
		t.Fatalf("expected hysteria2 up to be omitted when unset, got %#v", proxy["up"])
	}
	if _, exists := proxy["down"]; exists {
		t.Fatalf("expected hysteria2 down to be omitted when unset, got %#v", proxy["down"])
	}
}

func TestBuildMihomoLegacySubTags(t *testing.T) {
	tags := buildMihomoLegacySubTags([]string{"hk2", "hk2", "hk3"}, "hy2_hk2")
	lookup := map[string]bool{}
	for _, tag := range tags {
		lookup[tag] = true
	}

	expected := []string{
		"m_hy2_hk2_hk2",
		"m_hk2-hy2_hk2",
		"m_hk2",
		"m_hy2_hk2_hk3",
		"m_hk3-hy2_hk2",
		"m_hk3",
		"m_hy2_hk2",
		"mihomo_hy2_hk2_hk2",
		"mihomo_hk2-hy2_hk2",
		"mihomo_hk2",
		"mihomo_hy2_hk2_hk3",
		"mihomo_hk3-hy2_hk2",
		"mihomo_hk3",
		"mihomo_hy2_hk2",
	}
	for _, tag := range expected {
		if !lookup[tag] {
			t.Fatalf("expected legacy tag %s not found in %#v", tag, tags)
		}
	}
}

func runFindMihomoSyncTargetSubOutboundAllowsUnownedDesiredTag(t *testing.T) {
	db := setupMihomoSyncTestDB(t, "mihomo-sync-desired.db")

	inbound := model.MihomoInbound{Type: "http", Tag: "hy2_hk2"}
	if err := db.Create(&inbound).Error; err != nil {
		t.Fatalf("create inbound failed: %v", err)
	}

	client := &model.MihomoClient{Id: 1, Name: "hk2"}
	desiredTag := buildMihomoClientSubTag(inbound.Tag, client.Name)
	record := &model.SubOutbound{
		Type:    "http",
		Tag:     desiredTag,
		Options: mustJSONRaw(t, map[string]interface{}{"server": "1.1.1.1"}),
	}
	if err := db.Create(record).Error; err != nil {
		t.Fatalf("create suboutbound failed: %v", err)
	}

	target, err := (&MihomoSyncService{}).findSyncTargetSubOutbound(
		db,
		client,
		nil,
		&inbound,
		desiredTag,
		map[uint]struct{}{},
	)
	if err != nil {
		t.Fatalf("findSyncTargetSubOutbound returned error: %v", err)
	}
	if target == nil || target.Id != record.Id {
		t.Fatalf("expected desired tag record to be reused, got %#v", target)
	}
}

func TestFindMihomoSyncTargetSubOutbound_AllowsUnownedDesiredTag(t *testing.T) {
	t.Helper()
	runFindMihomoSyncTargetSubOutboundAllowsUnownedDesiredTag(t)
}

// Keep a compatibility alias for historical typo-based invocations.
func TestFindHimomoSyncTargetSubOutbound_AllowsUnownedDesiredTag(t *testing.T) {
	t.Helper()
	runFindMihomoSyncTargetSubOutboundAllowsUnownedDesiredTag(t)
}

func TestHasManagedMihomoSubOutbounds_DetectsLegacyTag(t *testing.T) {
	db := setupMihomoSyncTestDB(t, "mihomo-sync-managed.db")

	inbound := model.MihomoInbound{Type: "http", Tag: "hy2_hk2"}
	if err := db.Create(&inbound).Error; err != nil {
		t.Fatalf("create inbound failed: %v", err)
	}

	oldClient := &model.MihomoClient{
		Id:       1,
		Name:     "hk2",
		Inbounds: mustJSONRaw(t, []uint{inbound.Id}),
	}
	newClient := &model.MihomoClient{
		Id:       1,
		Name:     "hk3",
		Inbounds: mustJSONRaw(t, []uint{inbound.Id}),
	}

	legacyTag := "mihomo_hk2-" + inbound.Tag
	record := &model.SubOutbound{
		Type:    "http",
		Tag:     legacyTag,
		Options: mustJSONRaw(t, map[string]interface{}{"server": "1.1.1.1"}),
	}
	if err := db.Create(record).Error; err != nil {
		t.Fatalf("create legacy suboutbound failed: %v", err)
	}

	hasManaged, err := (&MihomoSyncService{}).hasManagedSubOutbounds(
		db,
		newClient,
		oldClient,
		map[uint]*model.MihomoInbound{inbound.Id: &inbound},
		[]uint{inbound.Id},
		[]uint{inbound.Id},
	)
	if err != nil {
		t.Fatalf("hasManagedSubOutbounds returned error: %v", err)
	}
	if !hasManaged {
		t.Fatalf("expected legacy tag to be detected as managed")
	}
}

func TestCleanupClientSubOutboundsOnDelete_RemovesLegacyTag(t *testing.T) {
	db := setupMihomoSyncTestDB(t, "mihomo-sync-cleanup.db")
	svc := &MihomoSyncService{}

	inbound := model.MihomoInbound{Type: "http", Tag: "hy2_hk2"}
	if err := db.Create(&inbound).Error; err != nil {
		t.Fatalf("create inbound failed: %v", err)
	}

	oldClient := &model.MihomoClient{
		Id:       1,
		Name:     "hk2",
		Inbounds: mustJSONRaw(t, []uint{inbound.Id}),
	}
	legacyTag := "mihomo_hk2-" + inbound.Tag
	record := &model.SubOutbound{
		Type:    "http",
		Tag:     legacyTag,
		Options: mustJSONRaw(t, map[string]interface{}{"server": "1.1.1.1"}),
	}
	if err := db.Create(record).Error; err != nil {
		t.Fatalf("create legacy suboutbound failed: %v", err)
	}

	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("begin tx failed: %v", tx.Error)
	}
	if err := svc.CleanupClientSubOutboundsOnDelete(tx, oldClient); err != nil {
		tx.Rollback()
		t.Fatalf("CleanupClientSubOutboundsOnDelete returned error: %v", err)
	}
	if err := tx.Commit().Error; err != nil {
		t.Fatalf("commit tx failed: %v", err)
	}

	var count int64
	if err := db.Model(model.SubOutbound{}).Where("tag = ?", legacyTag).Count(&count).Error; err != nil {
		t.Fatalf("count suboutbound failed: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected legacy suboutbound to be removed, remaining=%d", count)
	}
}

func setupMihomoSyncTestDB(t *testing.T, dbName string) *gorm.DB {
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

func mustJSONRaw(t *testing.T, value interface{}) json.RawMessage {
	t.Helper()

	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal failed: %v", err)
	}
	return json.RawMessage(raw)
}
