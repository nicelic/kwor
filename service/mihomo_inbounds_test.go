package service

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
)

func TestMihomoInboundServiceGetAllIncludesSelectableClientAssignments(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "mihomo-inbounds.db")
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

	shadowsocksInbound := model.MihomoInbound{
		Type: "shadowsocks",
		Tag:  "ss-40010",
		Options: json.RawMessage(`{
			"listen": "::",
			"listen_port": 40010,
			"method": "2022-blake3-aes-128-gcm",
			"password": "server-pass"
		}`),
	}
	if err := db.Create(&shadowsocksInbound).Error; err != nil {
		t.Fatalf("create mihomo shadowsocks inbound failed: %v", err)
	}

	shadowTLSInbound := model.MihomoInbound{
		Type: "shadowtls",
		Tag:  "stls-40443",
		Options: json.RawMessage(`{
			"listen": "::",
			"listen_port": 40443,
			"version": 2,
			"password": "shadowtls-pass",
			"handshake": {
				"server": "addons.mozilla.org",
				"server_port": 443
			}
		}`),
	}
	if err := db.Create(&shadowTLSInbound).Error; err != nil {
		t.Fatalf("create mihomo shadowtls inbound failed: %v", err)
	}

	clients := []model.MihomoClient{
		{
			Enable:   true,
			Name:     "ss-user",
			Config:   json.RawMessage(`{"shadowsocks":{"name":"ss-user","password":"client-pass"}}`),
			Inbounds: json.RawMessage(fmt.Sprintf("[%d]", shadowsocksInbound.Id)),
			Links:    json.RawMessage(`[]`),
		},
		{
			Enable:   true,
			Name:     "stls-user",
			Config:   json.RawMessage(`{"shadowtls":{"name":"stls-user","password":"client-pass"}}`),
			Inbounds: json.RawMessage(fmt.Sprintf("[%d]", shadowTLSInbound.Id)),
			Links:    json.RawMessage(`[]`),
		},
	}
	if err := db.Create(&clients).Error; err != nil {
		t.Fatalf("create mihomo clients failed: %v", err)
	}

	data, err := (&MihomoInboundService{}).GetAll()
	if err != nil {
		t.Fatalf("GetAll failed: %v", err)
	}

	gotByTag := map[string]map[string]interface{}{}
	for _, item := range *data {
		tag, _ := item["tag"].(string)
		gotByTag[tag] = item
	}

	assertSelectableUsers := func(tag, user string) {
		item := gotByTag[tag]
		if item == nil {
			t.Fatalf("missing inbound %s in GetAll result", tag)
		}

		userManagement, ok := item["user_management"].(MihomoInboundUserManagement)
		if !ok {
			t.Fatalf("inbound %s user_management has unexpected type: %#v", tag, item["user_management"])
		}
		if !userManagement.Selectable {
			t.Fatalf("inbound %s selectable = false, want true", tag)
		}

		users, ok := item["users"].([]string)
		if !ok {
			t.Fatalf("inbound %s users has unexpected type: %#v", tag, item["users"])
		}
		if len(users) != 1 || users[0] != user {
			t.Fatalf("inbound %s users = %#v, want [%q]", tag, users, user)
		}
	}

	assertSelectableUsers("ss-40010", "ss-user")
	assertSelectableUsers("stls-40443", "stls-user")
}

func TestResolveSnellSharedPSK(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "mihomo-snell-psk.db")
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

	createInbound := func(tag string) model.MihomoInbound {
		inbound := model.MihomoInbound{
			Type: "snell",
			Tag:  tag,
			Options: json.RawMessage(`{
				"listen": "::",
				"listen_port": 8443,
				"version": 5
			}`),
		}
		if err := db.Create(&inbound).Error; err != nil {
			t.Fatalf("create mihomo snell inbound failed: %v", err)
		}
		return inbound
	}

	createClient := func(name string, inboundID uint, psk string) {
		config := map[string]interface{}{}
		if psk != "" {
			config["snell"] = map[string]interface{}{
				"name": name,
				"psk":  psk,
			}
		} else {
			config["snell"] = map[string]interface{}{
				"name": name,
			}
		}
		rawConfig, err := json.Marshal(config)
		if err != nil {
			t.Fatalf("marshal config failed: %v", err)
		}
		client := model.MihomoClient{
			Enable:   true,
			Name:     name,
			Config:   rawConfig,
			Inbounds: json.RawMessage(fmt.Sprintf("[%d]", inboundID)),
			Links:    json.RawMessage(`[]`),
		}
		if err := db.Create(&client).Error; err != nil {
			t.Fatalf("create mihomo snell client failed: %v", err)
		}
	}

	svc := &MihomoInboundService{}

	t.Run("single shared psk", func(t *testing.T) {
		inbound := createInbound("snell-single")
		createClient("snell-user-1", inbound.Id, "shared-pass")

		psk, err := svc.resolveSnellSharedPSK(db, inbound.Id)
		if err != nil {
			t.Fatalf("resolveSnellSharedPSK failed: %v", err)
		}
		if psk != "shared-pass" {
			t.Fatalf("expected shared-pass, got %q", psk)
		}
	})

	t.Run("multiple matching psk", func(t *testing.T) {
		inbound := createInbound("snell-matching")
		createClient("snell-user-2", inbound.Id, "same-pass")
		createClient("snell-user-3", inbound.Id, "same-pass")

		psk, err := svc.resolveSnellSharedPSK(db, inbound.Id)
		if err != nil {
			t.Fatalf("resolveSnellSharedPSK failed: %v", err)
		}
		if psk != "same-pass" {
			t.Fatalf("expected same-pass, got %q", psk)
		}
	})

	t.Run("missing psk errors", func(t *testing.T) {
		inbound := createInbound("snell-missing")
		createClient("snell-user-4", inbound.Id, "")

		_, err := svc.resolveSnellSharedPSK(db, inbound.Id)
		if err == nil || err.Error() != "snell inbound has no bound client psk" {
			t.Fatalf("expected missing psk error, got %v", err)
		}
	})

	t.Run("mismatched psk errors", func(t *testing.T) {
		inbound := createInbound("snell-mismatch")
		createClient("snell-user-5", inbound.Id, "pass-a")
		createClient("snell-user-6", inbound.Id, "pass-b")

		_, err := svc.resolveSnellSharedPSK(db, inbound.Id)
		if err == nil || err.Error() != "snell inbound has multiple different client psk values" {
			t.Fatalf("expected mismatched psk error, got %v", err)
		}
	})
}

func TestProcessSnellInboundPreservesAndDefaultsUDP(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "mihomo-snell-udp.db")
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

	createInbound := func(tag string) *model.MihomoInbound {
		inbound := &model.MihomoInbound{
			Type: "snell",
			Tag:  tag,
			Options: json.RawMessage(`{
				"listen": "::",
				"listen_port": 8443,
				"version": 5
			}`),
		}
		if err := db.Create(inbound).Error; err != nil {
			t.Fatalf("create mihomo snell inbound failed: %v", err)
		}
		return inbound
	}

	bindClient := func(name string, inboundID uint, psk string) {
		config := map[string]interface{}{
			"snell": map[string]interface{}{
				"name": name,
				"psk":  psk,
			},
		}
		rawConfig, err := json.Marshal(config)
		if err != nil {
			t.Fatalf("marshal config failed: %v", err)
		}
		client := &model.MihomoClient{
			Enable:   true,
			Name:     name,
			Config:   rawConfig,
			Inbounds: json.RawMessage(fmt.Sprintf("[%d]", inboundID)),
			Links:    json.RawMessage(`[]`),
		}
		if err := db.Create(client).Error; err != nil {
			t.Fatalf("create mihomo snell client failed: %v", err)
		}
	}

	svc := &MihomoInboundService{}

	t.Run("explicit false", func(t *testing.T) {
		inbound := createInbound("snell-udp-false")
		bindClient("snell-udp-false-user", inbound.Id, "shared-pass")

		processed, err := svc.processSnellInbound(db, []byte(`{
			"type": "snell",
			"version": 5,
			"udp": false
		}`), inbound)
		if err != nil {
			t.Fatalf("processSnellInbound failed: %v", err)
		}

		payload := map[string]interface{}{}
		if err := json.Unmarshal(processed, &payload); err != nil {
			t.Fatalf("unmarshal processed payload failed: %v", err)
		}
		if got, ok := payload["udp"].(bool); !ok || got {
			t.Fatalf("expected udp=false, got %#v", payload["udp"])
		}
	})

	t.Run("default true", func(t *testing.T) {
		inbound := createInbound("snell-udp-default")
		bindClient("snell-udp-default-user", inbound.Id, "shared-pass")

		processed, err := svc.processSnellInbound(db, []byte(`{
			"type": "snell",
			"version": 5
		}`), inbound)
		if err != nil {
			t.Fatalf("processSnellInbound failed: %v", err)
		}

		payload := map[string]interface{}{}
		if err := json.Unmarshal(processed, &payload); err != nil {
			t.Fatalf("unmarshal processed payload failed: %v", err)
		}
		if got, ok := payload["udp"].(bool); !ok || !got {
			t.Fatalf("expected udp=true by default, got %#v", payload["udp"])
		}
	})
}

func TestMihomoSnellInboundInitBindingsAllowOnlyOneUser(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "mihomo-snell-init-bindings.db")
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

	clients := []model.MihomoClient{
		{
			Enable:   true,
			Name:     "snell-user-a",
			Config:   json.RawMessage(`{"snell":{"name":"snell-user-a","psk":"pass-a"}}`),
			Inbounds: json.RawMessage(`[]`),
			Links:    json.RawMessage(`[]`),
		},
		{
			Enable:   true,
			Name:     "snell-user-b",
			Config:   json.RawMessage(`{"snell":{"name":"snell-user-b","psk":"pass-b"}}`),
			Inbounds: json.RawMessage(`[]`),
			Links:    json.RawMessage(`[]`),
		},
	}
	if err := db.Create(&clients).Error; err != nil {
		t.Fatalf("create mihomo clients failed: %v", err)
	}

	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("begin tx failed: %v", tx.Error)
	}
	t.Cleanup(func() {
		tx.Rollback()
	})

	rawInbound := json.RawMessage(`{
		"type": "snell",
		"tag": "snell-only-one-user",
		"listen": "::",
		"listen_port": 9443,
		"version": 5
	}`)

	_, err = (&MihomoInboundService{}).Save(tx, "new", rawInbound, fmt.Sprintf("%d,%d", clients[0].Id, clients[1].Id), "example.com")
	if err == nil || err.Error() != "snell inbound can bind only one user" {
		t.Fatalf("expected single-user snell binding error, got %v", err)
	}
}

func TestMihomoSnellClientBindingsRejectSecondUser(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "mihomo-snell-client-binding-limit.db")
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

	inbound := model.MihomoInbound{
		Type: "snell",
		Tag:  "snell-occupied",
		Options: json.RawMessage(`{
			"listen": "::",
			"listen_port": 9443,
			"version": 5
		}`),
	}
	if err := db.Create(&inbound).Error; err != nil {
		t.Fatalf("create mihomo snell inbound failed: %v", err)
	}

	existingClient := model.MihomoClient{
		Enable:   true,
		Name:     "snell-user-a",
		Config:   json.RawMessage(`{"snell":{"name":"snell-user-a","psk":"pass-a"}}`),
		Inbounds: json.RawMessage(fmt.Sprintf("[%d]", inbound.Id)),
		Links:    json.RawMessage(`[]`),
	}
	if err := db.Create(&existingClient).Error; err != nil {
		t.Fatalf("create existing mihomo client failed: %v", err)
	}

	conflictingClient := model.MihomoClient{
		Enable:   true,
		Name:     "snell-user-b",
		Config:   json.RawMessage(`{"snell":{"name":"snell-user-b","psk":"pass-b"}}`),
		Inbounds: json.RawMessage(fmt.Sprintf("[%d]", inbound.Id)),
		Links:    json.RawMessage(`[]`),
	}
	rawClient, err := json.Marshal(conflictingClient)
	if err != nil {
		t.Fatalf("marshal conflicting client failed: %v", err)
	}

	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("begin tx failed: %v", tx.Error)
	}
	t.Cleanup(func() {
		tx.Rollback()
	})

	_, err = (&MihomoClientService{}).Save(tx, "new", rawClient, "example.com")
	expected := "snell inbound snell-occupied can bind only one user"
	if err == nil || err.Error() != expected {
		t.Fatalf("expected %q, got %v", expected, err)
	}
}
