package service

import (
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
)

func createSingboxOutboundForSafetyTest(t *testing.T, tag string, outboundType string, payload map[string]interface{}) model.Outbound {
	t.Helper()
	payload["tag"] = tag
	payload["type"] = outboundType
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal sing-box outbound payload failed: %v", err)
	}
	record := model.Outbound{
		Tag:         tag,
		Type:        outboundType,
		RawOutbound: raw,
	}
	if err := database.GetDB().Create(&record).Error; err != nil {
		t.Fatalf("create sing-box outbound %q failed: %v", tag, err)
	}
	return record
}

func TestExtractSingboxOutboundGroupSubscriptionRejectsTooManyEntries(t *testing.T) {
	entries := make([]map[string]interface{}, 0, singboxOutboundSubscriptionImportMaxNodes+1)
	for index := 0; index <= singboxOutboundSubscriptionImportMaxNodes; index++ {
		entries = append(entries, map[string]interface{}{
			"type": "socks",
			"tag":  "node-" + strconv.Itoa(index),
		})
	}
	payload, err := json.Marshal(map[string]interface{}{"outbounds": entries})
	if err != nil {
		t.Fatalf("marshal subscription fixture: %v", err)
	}

	_, _, err = extractSingboxOutboundGroupSubscription(payload)
	if err == nil || !strings.Contains(err.Error(), "more than") {
		t.Fatalf("expected oversized subscription rejection, got %v", err)
	}
}

func TestSingboxOutboundGroupImportRejectsExistingManualTag(t *testing.T) {
	openOutboundGroupOrderTestDB(t)
	db := database.GetDB()
	group := model.OutboundGroup{Name: "imported", Outbounds: "[]", SortOrder: 1}
	if err := db.Create(&group).Error; err != nil {
		t.Fatalf("create outbound group: %v", err)
	}
	manual := model.Outbound{
		Type:        "socks",
		Tag:         "manual-node",
		RawOutbound: json.RawMessage(`{"type":"socks","tag":"manual-node","server":"127.0.0.1","server_port":1080}`),
	}
	if err := db.Create(&manual).Error; err != nil {
		t.Fatalf("create manual outbound: %v", err)
	}

	imported := map[string]interface{}{
		"type":        "socks",
		"tag":         "manual-node",
		"server":      "127.0.0.2",
		"server_port": 1081,
	}
	raw, err := json.Marshal(imported)
	if err != nil {
		t.Fatalf("marshal imported outbound: %v", err)
	}

	err = (&OutboundGroupService{}).applyImportedSubscription(
		db,
		group.Name,
		"https://example.invalid/sub.json",
		false,
		[]map[string]interface{}{imported},
		map[string]json.RawMessage{"manual-node": raw},
		true,
	)
	if err == nil || !strings.Contains(err.Error(), "owned outside group") {
		t.Fatalf("expected manual tag ownership rejection, got %v", err)
	}

	stored := &model.Outbound{}
	if err := db.Where("tag = ?", "manual-node").First(stored).Error; err != nil {
		t.Fatalf("reload manual outbound: %v", err)
	}
	if !strings.Contains(string(stored.RawOutbound), "127.0.0.1") {
		t.Fatalf("manual outbound was overwritten: %s", stored.RawOutbound)
	}
}

func TestSingboxOutboundGroupImportPreflightsWithTransactionSettings(t *testing.T) {
	openOutboundGroupOrderTestDB(t)
	db := database.GetDB()
	group := model.OutboundGroup{Name: "imported", Outbounds: "[]", SortOrder: 1}
	if err := db.Create(&group).Error; err != nil {
		t.Fatalf("create outbound group: %v", err)
	}
	config := model.Setting{Key: "config", Value: `{"log":{"level":"info"},"dns":{"rules":[]},"route":{"rules":[]}}`}
	if err := db.Create(&config).Error; err != nil {
		t.Fatalf("create sing-box config fixture: %v", err)
	}
	imported := map[string]interface{}{
		"type":        "socks",
		"tag":         "imported-node",
		"server":      "127.0.0.1",
		"server_port": 1080,
	}
	raw, err := json.Marshal(imported)
	if err != nil {
		t.Fatalf("marshal imported outbound: %v", err)
	}

	if err := (&OutboundGroupService{}).applyImportedSubscription(
		db,
		group.Name,
		"https://example.invalid/sub.json",
		false,
		[]map[string]interface{}{imported},
		map[string]json.RawMessage{"imported-node": raw},
		true,
	); err != nil {
		t.Fatalf("transactional subscription import failed: %v", err)
	}

	var stored model.Outbound
	if err := db.Where("tag = ?", "imported-node").First(&stored).Error; err != nil {
		t.Fatalf("load imported outbound: %v", err)
	}
	if string(stored.RawOutbound) == "" {
		t.Fatal("imported outbound raw payload was not persisted")
	}
}

func TestSingboxOutboundDeleteRejectsSelectorReference(t *testing.T) {
	openOutboundGroupOrderTestDB(t)
	db := database.GetDB()
	node := model.Outbound{
		Type:        "socks",
		Tag:         "node-a",
		RawOutbound: json.RawMessage(`{"type":"socks","tag":"node-a","server":"127.0.0.1","server_port":1080}`),
	}
	selector := model.Outbound{
		Type:        "selector",
		Tag:         "node-selector",
		RawOutbound: json.RawMessage(`{"type":"selector","tag":"node-selector","outbounds":["node-a"]}`),
	}
	if err := db.Create(&node).Error; err != nil {
		t.Fatalf("create referenced outbound: %v", err)
	}
	if err := db.Create(&selector).Error; err != nil {
		t.Fatalf("create selector outbound: %v", err)
	}

	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("begin transaction: %v", tx.Error)
	}
	err := (&OutboundService{}).Save(tx, "del", json.RawMessage(`"node-a"`))
	if err == nil || !strings.Contains(err.Error(), "node-selector") {
		tx.Rollback()
		t.Fatalf("expected selector reference rejection, got %v", err)
	}
	tx.Rollback()
}

func TestSingboxOutboundDeleteRejectsDNSDetourReference(t *testing.T) {
	openOutboundGroupOrderTestDB(t)
	db := database.GetDB()
	node := model.Outbound{
		Type:        "socks",
		Tag:         "dns-node",
		RawOutbound: json.RawMessage(`{"type":"socks","tag":"dns-node","server":"127.0.0.1","server_port":1080}`),
	}
	if err := db.Create(&node).Error; err != nil {
		t.Fatalf("create DNS detour outbound: %v", err)
	}
	var config model.Setting
	if err := db.Where("key = ?", "config").First(&config).Error; database.IsNotFound(err) {
		config = model.Setting{Key: "config"}
	} else if err != nil {
		t.Fatalf("load sing-box config: %v", err)
	}
	config.Value = `{"dns":{"servers":[{"type":"udp","tag":"dns-main","server":"1.1.1.1","detour":"dns-node"}]},"route":{"rules":[]}}`
	if err := db.Save(&config).Error; err != nil {
		t.Fatalf("save sing-box config fixture: %v", err)
	}

	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("begin transaction: %v", tx.Error)
	}
	err := (&OutboundService{}).Save(tx, "del", json.RawMessage(`"dns-node"`))
	if err == nil || !strings.Contains(err.Error(), "dns server") {
		tx.Rollback()
		t.Fatalf("expected DNS detour reference rejection, got %v", err)
	}
	tx.Rollback()
}

func TestConfigSaveRejectsSingboxOutboundWriteWhileSubscriptionImportIsRunning(t *testing.T) {
	if !singboxOutboundSubscriptionImportMu.TryLock() {
		t.Fatal("expected test subscription lock to be available")
	}
	defer singboxOutboundSubscriptionImportMu.Unlock()

	_, err := (&ConfigService{}).Save("outbounds", "new", json.RawMessage(`{}`), "", "", "")
	if !errors.Is(err, ErrSingboxOutboundSubscriptionImportBusy) {
		t.Fatalf("expected sing-box outbound save to reject during subscription import, got %v", err)
	}
}

func TestSingboxOutboundDeleteRejectsDerivedShadowTLSReference(t *testing.T) {
	openOutboundGroupOrderTestDB(t)
	db := database.GetDB()
	createSingboxOutboundForSafetyTest(t, "stls-node", "shadowtls", map[string]interface{}{
		"server":      "203.0.113.10",
		"server_port": 443,
		"version":     3,
		"password":    "shadow-secret",
		"ss_config": map[string]interface{}{
			"method":   "2022-blake3-aes-128-gcm",
			"password": "ss-secret",
		},
	})
	if err := db.Save(&model.Setting{Key: "config", Value: `{"route":{"final":"stls-node-out","rules":[]}}`}).Error; err != nil {
		t.Fatalf("save route fixture: %v", err)
	}

	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("begin transaction: %v", tx.Error)
	}
	err := (&OutboundService{}).Save(tx, "del", json.RawMessage(`"stls-node"`))
	tx.Rollback()
	if err == nil || !strings.Contains(err.Error(), "route.final") || !strings.Contains(err.Error(), "stls-node-out") {
		t.Fatalf("expected derived ShadowTLS reference rejection, got %v", err)
	}
}

func TestSingboxOutboundDeleteRejectsNestedRouteReference(t *testing.T) {
	openOutboundGroupOrderTestDB(t)
	db := database.GetDB()
	createSingboxOutboundForSafetyTest(t, "nested-node", "vless", map[string]interface{}{
		"server":      "edge.example.com",
		"server_port": 443,
		"uuid":        "11111111-1111-1111-1111-111111111111",
	})
	if err := db.Save(&model.Setting{Key: "config", Value: `{
		"route":{"rules":[{
			"type":"logical",
			"mode":"and",
			"rules":[{"action":"route","outbound":"nested-node"}]
		}]}
	}`}).Error; err != nil {
		t.Fatalf("save nested route fixture: %v", err)
	}

	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("begin transaction: %v", tx.Error)
	}
	err := (&OutboundService{}).Save(tx, "del", json.RawMessage(`"nested-node"`))
	tx.Rollback()
	if err == nil || !strings.Contains(err.Error(), "route rule #1 #1") || !strings.Contains(err.Error(), "nested-node") {
		t.Fatalf("expected nested route reference rejection, got %v", err)
	}
}

func TestSingboxOutboundDeleteRejectsSelectedDNSDetourReference(t *testing.T) {
	openOutboundGroupOrderTestDB(t)
	db := database.GetDB()
	createSingboxOutboundForSafetyTest(t, "dns-detour", "vless", map[string]interface{}{
		"server":      "edge.example.com",
		"server_port": 443,
		"uuid":        "22222222-2222-2222-2222-222222222222",
	})
	if err := db.Create(&model.DnsServer{
		Type:    "https",
		Tag:     "selected-dns",
		Options: json.RawMessage(`{"server":"dns.example.com","server_port":443,"detour":"dns-detour"}`),
	}).Error; err != nil {
		t.Fatalf("create selected DNS server: %v", err)
	}
	if err := db.Save(&model.Setting{Key: "config", Value: `{"dns":{"final":"selected-dns","rules":[]}}`}).Error; err != nil {
		t.Fatalf("save DNS fixture: %v", err)
	}

	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("begin transaction: %v", tx.Error)
	}
	err := (&OutboundService{}).Save(tx, "del", json.RawMessage(`"dns-detour"`))
	tx.Rollback()
	if err == nil || !strings.Contains(err.Error(), `dns server "selected-dns"`) || !strings.Contains(err.Error(), "dns-detour") {
		t.Fatalf("expected selected DNS detour reference rejection, got %v", err)
	}
}

func TestSingboxOutboundEditRejectsRemovedShadowTLSPairTag(t *testing.T) {
	openOutboundGroupOrderTestDB(t)
	db := database.GetDB()
	node := createSingboxOutboundForSafetyTest(t, "stls-node", "shadowtls", map[string]interface{}{
		"server":      "203.0.113.10",
		"server_port": 443,
		"version":     3,
		"password":    "shadow-secret",
		"ss_config": map[string]interface{}{
			"method":   "2022-blake3-aes-128-gcm",
			"password": "ss-secret",
		},
	})
	if err := db.Save(&model.Setting{Key: "config", Value: `{"route":{"final":"stls-node-out","rules":[]}}`}).Error; err != nil {
		t.Fatalf("save route fixture: %v", err)
	}

	payload := json.RawMessage(`{
		"id": ` + strconv.FormatUint(uint64(node.Id), 10) + `,
		"type": "shadowtls",
		"tag": "stls-node",
		"server": "203.0.113.10",
		"server_port": 443,
		"version": 3,
		"password": "shadow-secret"
	}`)
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("begin transaction: %v", tx.Error)
	}
	err := (&OutboundService{}).Save(tx, "edit", payload)
	tx.Rollback()
	if err == nil || !strings.Contains(err.Error(), "route.final") || !strings.Contains(err.Error(), "stls-node-out") {
		t.Fatalf("expected ShadowTLS pair removal rejection, got %v", err)
	}
}

func TestSingboxOutboundGroupRefreshRejectsRemovedShadowTLSPairTag(t *testing.T) {
	openOutboundGroupOrderTestDB(t)
	db := database.GetDB()
	group := model.OutboundGroup{Name: "remote", Outbounds: `["stls-node"]`, SortOrder: 1}
	if err := db.Create(&group).Error; err != nil {
		t.Fatalf("create outbound group: %v", err)
	}
	createSingboxOutboundForSafetyTest(t, "stls-node", "shadowtls", map[string]interface{}{
		"server":      "203.0.113.10",
		"server_port": 443,
		"version":     3,
		"password":    "shadow-secret",
		"ss_config": map[string]interface{}{
			"method":   "2022-blake3-aes-128-gcm",
			"password": "ss-secret",
		},
	})
	if err := db.Save(&model.Setting{Key: "config", Value: `{"route":{"final":"stls-node-out","rules":[]}}`}).Error; err != nil {
		t.Fatalf("save route fixture: %v", err)
	}

	updated := map[string]interface{}{
		"type":        "socks",
		"tag":         "stls-node",
		"server":      "203.0.113.10",
		"server_port": 1080,
	}
	raw, err := json.Marshal(updated)
	if err != nil {
		t.Fatalf("marshal refreshed outbound: %v", err)
	}
	err = (&OutboundGroupService{}).applyImportedSubscription(
		db,
		group.Name,
		"https://example.invalid/sub.json",
		false,
		[]map[string]interface{}{updated},
		map[string]json.RawMessage{"stls-node": raw},
		true,
	)
	if err == nil || !strings.Contains(err.Error(), "route.final") || !strings.Contains(err.Error(), "stls-node-out") {
		t.Fatalf("expected subscription refresh rejection, got %v", err)
	}

	var stored model.Outbound
	if err := db.Where("tag = ?", "stls-node").First(&stored).Error; err != nil {
		t.Fatalf("reload stored outbound: %v", err)
	}
	if !strings.Contains(string(stored.RawOutbound), "ss_config") {
		t.Fatalf("rejected refresh must retain prior ShadowTLS payload: %s", stored.RawOutbound)
	}
}

func TestSingboxOutboundSaveRejectsShadowTLSRuntimeTagCollision(t *testing.T) {
	openOutboundGroupOrderTestDB(t)
	db := database.GetDB()
	createSingboxOutboundForSafetyTest(t, "stls-node", "shadowtls", map[string]interface{}{
		"server":      "203.0.113.10",
		"server_port": 443,
		"version":     3,
		"password":    "shadow-secret",
		"ss_config": map[string]interface{}{
			"method":   "2022-blake3-aes-128-gcm",
			"password": "ss-secret",
		},
	})

	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("begin transaction: %v", tx.Error)
	}
	err := (&OutboundService{}).Save(tx, "new", json.RawMessage(`{
		"type":"socks",
		"tag":"stls-node-out",
		"server":"127.0.0.1",
		"server_port":1080
	}`))
	tx.Rollback()
	if err == nil || !strings.Contains(err.Error(), "runtime outbound tag") || !strings.Contains(err.Error(), "stls-node-out") {
		t.Fatalf("expected runtime tag collision rejection, got %v", err)
	}
}

func TestSingboxOutboundSaveValidatesNewRuntimeReferences(t *testing.T) {
	openOutboundGroupOrderTestDB(t)
	db := database.GetDB()
	if err := db.Create(&model.Outbound{
		Type:        "direct",
		Tag:         "known-target",
		RawOutbound: json.RawMessage(`{"type":"direct","tag":"known-target"}`),
	}).Error; err != nil {
		t.Fatalf("create target outbound: %v", err)
	}

	trySave := func(raw string) error {
		t.Helper()
		tx := db.Begin()
		if tx.Error != nil {
			t.Fatalf("begin transaction: %v", tx.Error)
		}
		err := (&OutboundService{}).Save(tx, "new", json.RawMessage(raw))
		tx.Rollback()
		return err
	}

	if err := trySave(`{"type":"socks","tag":"bad-detour","server":"127.0.0.1","server_port":1080,"detour":"missing"}`); err == nil || !strings.Contains(err.Error(), "unknown detour") {
		t.Fatalf("unknown outbound detour was accepted: %v", err)
	}
	if err := trySave(`{"type":"selector","tag":"bad-selector","outbounds":["missing"]}`); err == nil || !strings.Contains(err.Error(), "unknown member") {
		t.Fatalf("unknown selector member was accepted: %v", err)
	}
	if err := trySave(`{"type":"selector","tag":"self-selector","outbounds":["self-selector"]}`); err == nil || !strings.Contains(err.Error(), "itself") {
		t.Fatalf("self selector member was accepted: %v", err)
	}
	if err := trySave(`{"type":"socks","tag":"valid-detour","server":"127.0.0.1","server_port":1080,"detour":"known-target"}`); err != nil {
		t.Fatalf("valid outbound detour was rejected: %v", err)
	}
}

func TestConfigChangeRevisionIsMonotonicAndForcesRestartSnapshot(t *testing.T) {
	previous := atomic.LoadInt64(&LastUpdate)
	t.Cleanup(func() {
		atomic.StoreInt64(&LastUpdate, previous)
	})
	atomic.StoreInt64(&LastUpdate, 0)

	updated, err := (&ConfigService{}).CheckChanges("123")
	if err != nil || !updated {
		t.Fatalf("first check after reset must require full snapshot: updated=%v err=%v", updated, err)
	}
	first := CurrentConfigRevisionForPolling()
	markLastUpdate(0)
	second := CurrentConfigRevisionForPolling()
	if second <= first {
		t.Fatalf("revision did not advance: first=%d second=%d", first, second)
	}

	updated, err = (&ConfigService{}).CheckChanges(strconv.FormatInt(first, 10))
	if err != nil || !updated {
		t.Fatalf("newer same-process revision was not detected: updated=%v err=%v", updated, err)
	}
}

func TestPollingRevisionsAreIndependentAcrossCores(t *testing.T) {
	previousDefault := atomic.LoadInt64(&LastUpdate)
	previousMihomo := atomic.LoadInt64(&MihomoLastUpdate)
	t.Cleanup(func() {
		atomic.StoreInt64(&LastUpdate, previousDefault)
		atomic.StoreInt64(&MihomoLastUpdate, previousMihomo)
	})
	atomic.StoreInt64(&LastUpdate, 0)
	atomic.StoreInt64(&MihomoLastUpdate, 0)

	configService := &ConfigService{}
	if updated, err := configService.CheckChanges("1"); err != nil || !updated {
		t.Fatalf("default revision should require an initial snapshot: updated=%v err=%v", updated, err)
	}
	if updated, err := configService.CheckMihomoChanges("1"); err != nil || !updated {
		t.Fatalf("mihomo revision should require an initial snapshot: updated=%v err=%v", updated, err)
	}
	defaultRevision := CurrentConfigRevisionForPolling()
	mihomoRevision := CurrentMihomoConfigRevisionForPolling()

	markMihomoLastUpdate(0)
	if updated, err := configService.CheckChanges(strconv.FormatInt(defaultRevision, 10)); err != nil || updated {
		t.Fatalf("mihomo-only update should not invalidate default snapshot: updated=%v err=%v", updated, err)
	}
	if updated, err := configService.CheckMihomoChanges(strconv.FormatInt(mihomoRevision, 10)); err != nil || !updated {
		t.Fatalf("mihomo-only update was not visible to mihomo polling: updated=%v err=%v", updated, err)
	}
}
