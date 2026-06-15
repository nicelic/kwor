package sub

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/service"
)

func TestDefaultShortSubscriptionUsesClientIPv6Override(t *testing.T) {
	setupSubscriptionTestDB(t, "default-short-link-ipv6.db")

	db := database.GetDB()
	inbound := createOrderedDefaultInbound(t, "default-short-link", "default.example.com", 443)
	if err := db.Create(inbound).Error; err != nil {
		t.Fatalf("create inbound failed: %v", err)
	}

	client := model.Client{
		Enable:   true,
		Name:     "default-short-user",
		ServerIp: "[2001:db8::8]",
		Config: mustRawJSON(t, map[string]interface{}{
			"trojan": map[string]interface{}{
				"password": "secret-pass",
			},
		}),
		Inbounds: mustRawJSON(t, []uint{inbound.Id}),
		Links:    mustRawJSON(t, []map[string]string{}),
	}

	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("begin tx failed: %v", tx.Error)
	}
	defer tx.Rollback()

	payload, err := json.Marshal(client)
	if err != nil {
		t.Fatalf("marshal client failed: %v", err)
	}
	if _, err := (&service.ClientService{}).Save(tx, "new", payload, "panel.example.com"); err != nil {
		t.Fatalf("save client failed: %v", err)
	}
	if err := tx.Commit().Error; err != nil {
		t.Fatalf("commit tx failed: %v", err)
	}

	var stored model.Client
	if err := db.Where("name = ?", client.Name).First(&stored).Error; err != nil {
		t.Fatalf("reload client failed: %v", err)
	}
	if stored.ServerIp != "2001:db8::8" {
		t.Fatalf("stored ServerIp = %q, want %q", stored.ServerIp, "2001:db8::8")
	}

	result, _, err := (&SubService{}).GetSubs(client.Name)
	if err != nil {
		t.Fatalf("GetSubs failed: %v", err)
	}
	decoded := decodeSubscriptionPayload(t, *result)
	if !strings.Contains(decoded, "trojan://secret-pass@[2001:db8::8]:443") {
		t.Fatalf("expected IPv6 short link in subscription, got %q", decoded)
	}
	if strings.Contains(decoded, "default.example.com") {
		t.Fatalf("expected subscription to use client override instead of default host, got %q", decoded)
	}
}

func TestMihomoShortSubscriptionUsesClientDomainOverride(t *testing.T) {
	setupSubscriptionTestDB(t, "mihomo-short-link-domain.db")

	db := database.GetDB()
	inbound := createOrderedMihomoInbound(t, "mihomo-short-link", "default.example.com", 443)
	if err := db.Create(inbound).Error; err != nil {
		t.Fatalf("create mihomo inbound failed: %v", err)
	}

	client := model.MihomoClient{
		Enable:   true,
		Name:     "mihomo-short-user",
		ServerIp: "sub.example.com",
		Config: mustRawJSON(t, map[string]interface{}{
			"trojan": map[string]interface{}{
				"password": "secret-pass",
			},
		}),
		Inbounds: mustRawJSON(t, []uint{inbound.Id}),
		Links:    mustRawJSON(t, []map[string]string{}),
	}

	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("begin tx failed: %v", tx.Error)
	}
	defer tx.Rollback()

	payload, err := json.Marshal(client)
	if err != nil {
		t.Fatalf("marshal mihomo client failed: %v", err)
	}
	if _, err := (&service.MihomoClientService{}).Save(tx, "new", payload, "panel.example.com"); err != nil {
		t.Fatalf("save mihomo client failed: %v", err)
	}
	if err := tx.Commit().Error; err != nil {
		t.Fatalf("commit tx failed: %v", err)
	}

	result, _, err := (&SubService{}).GetMihomoSubs(client.Name)
	if err != nil {
		t.Fatalf("GetMihomoSubs failed: %v", err)
	}
	decoded := decodeSubscriptionPayload(t, *result)
	if !strings.Contains(decoded, "trojan://secret-pass@sub.example.com:443") {
		t.Fatalf("expected domain short link in mihomo subscription, got %q", decoded)
	}
	if strings.Contains(decoded, "@[sub.example.com]:443") {
		t.Fatalf("domain host should not be wrapped in brackets, got %q", decoded)
	}
}

func TestDefaultShortSubscriptionRefreshesStaleStoredLocalLinks(t *testing.T) {
	setupSubscriptionTestDB(t, "default-short-link-stale.db")

	db := database.GetDB()
	inbound := createOrderedDefaultInbound(t, "default-short-link-stale", "legacy.example.com", 443)
	if err := db.Create(inbound).Error; err != nil {
		t.Fatalf("create inbound failed: %v", err)
	}

	client := model.Client{
		Enable:   true,
		Name:     "default-stale-user",
		ServerIp: "fresh.example.com",
		Config: mustRawJSON(t, map[string]interface{}{
			"trojan": map[string]interface{}{
				"password": "secret-pass",
			},
		}),
		Inbounds: mustRawJSON(t, []uint{inbound.Id}),
		Links: mustRawJSON(t, []Link{
			{
				Type:   "local",
				Remark: inbound.Tag,
				Uri:    "trojan://secret-pass@legacy.example.com:443?type=tcp#default-short-link-stale",
			},
		}),
	}
	if err := db.Create(&client).Error; err != nil {
		t.Fatalf("create client failed: %v", err)
	}

	result, _, err := (&SubService{}).GetSubs(client.Name)
	if err != nil {
		t.Fatalf("GetSubs failed: %v", err)
	}
	decoded := decodeSubscriptionPayload(t, *result)
	if !strings.Contains(decoded, "trojan://secret-pass@fresh.example.com:443") {
		t.Fatalf("expected subscription to rebuild stale local link, got %q", decoded)
	}
	if strings.Contains(decoded, "legacy.example.com") {
		t.Fatalf("expected stale stored local link host to be replaced, got %q", decoded)
	}
}

func decodeSubscriptionPayload(t *testing.T, payload string) string {
	t.Helper()

	decoded, err := base64.StdEncoding.DecodeString(payload)
	if err == nil {
		return string(decoded)
	}
	return payload
}
