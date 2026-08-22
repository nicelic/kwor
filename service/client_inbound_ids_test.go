package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
)

func TestClientServiceSaveRejectsMissingAndNonSelectableInboundTargets(t *testing.T) {
	initPanelSQLiteSettingTestDB(t)
	db := database.GetDB()
	service := &ClientService{}

	direct := model.Inbound{
		Type:    "direct",
		Tag:     "direct-listener",
		Options: json.RawMessage(`{"listen":"::","listen_port":18080}`),
	}
	if err := db.Create(&direct).Error; err != nil {
		t.Fatalf("create direct inbound: %v", err)
	}

	missingPayload := json.RawMessage(`{
  "enable": true,
  "name": "missing-target",
  "config": {},
  "inbounds": [999999],
  "links": []
}`)
	if _, err := service.Save(db, "new", missingPayload, "panel.example.com"); err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("missing inbound client binding was accepted: %v", err)
	}

	directPayload := json.RawMessage(`{
  "enable": true,
  "name": "direct-target",
  "config": {},
  "inbounds": [` + string(mustMarshalClientInboundID(t, direct.Id)) + `],
  "links": []
}`)
	if _, err := service.Save(db, "new", directPayload, "panel.example.com"); err == nil || !strings.Contains(err.Error(), "cannot be assigned") {
		t.Fatalf("non-selectable inbound client binding was accepted: %v", err)
	}
}

func TestClientServiceBulkNormalizesFieldsAndUsesEachInboundSelection(t *testing.T) {
	initPanelSQLiteSettingTestDB(t)
	db := database.GetDB()
	service := &ClientService{}

	first := model.Inbound{
		Type:    "socks",
		Tag:     "socks-first",
		Addrs:   json.RawMessage(`[]`),
		Options: json.RawMessage(`{"listen":"::","listen_port":18081}`),
	}
	second := model.Inbound{
		Type:    "socks",
		Tag:     "socks-second",
		Addrs:   json.RawMessage(`[]`),
		Options: json.RawMessage(`{"listen":"::","listen_port":18082}`),
	}
	if err := db.Create(&first).Error; err != nil {
		t.Fatalf("create first inbound: %v", err)
	}
	if err := db.Create(&second).Error; err != nil {
		t.Fatalf("create second inbound: %v", err)
	}

	payload := json.RawMessage(`[
  {
    "enable": true,
    "name": "bulk-first",
    "config": {"socks":{"username":"first","password":"secret"}},
    "inbounds": [` + string(mustMarshalClientInboundID(t, first.Id)) + `],
    "links": [],
    "volume": -1,
    "expiry": -1,
    "extra": 99,
    "up": -1,
    "down": -1,
    "speedLimitMbps": -1
  },
  {
    "enable": true,
    "name": "bulk-second",
    "config": {"socks":{"username":"second","password":"secret"}},
    "inbounds": [` + string(mustMarshalClientInboundID(t, second.Id)) + `],
    "links": []
  }
]`)
	if _, err := service.Save(db, "addbulk", payload, "panel.example.com"); err != nil {
		t.Fatalf("bulk client save failed: %v", err)
	}

	var firstClient, secondClient model.Client
	if err := database.GetDB().Where("name = ?", "bulk-first").First(&firstClient).Error; err != nil {
		t.Fatalf("load first bulk client: %v", err)
	}
	if err := database.GetDB().Where("name = ?", "bulk-second").First(&secondClient).Error; err != nil {
		t.Fatalf("load second bulk client: %v", err)
	}
	if firstClient.Volume != 0 || firstClient.Expiry != 0 || firstClient.Up != 0 || firstClient.Down != 0 {
		t.Fatalf("bulk client statistics were not normalized: %#v", firstClient)
	}
	if firstClient.Extra != 31 || firstClient.SpeedLimitMbps != 0 || firstClient.LastReset <= 0 {
		t.Fatalf("bulk client defaults were not normalized: %#v", firstClient)
	}
	assertOnlyLocalClientLinkRemark(t, firstClient.Links, first.Tag)
	assertOnlyLocalClientLinkRemark(t, secondClient.Links, second.Tag)
}

func mustMarshalClientInboundID(t *testing.T, id uint) []byte {
	t.Helper()
	value, err := json.Marshal(id)
	if err != nil {
		t.Fatalf("marshal inbound id: %v", err)
	}
	return value
}

func assertOnlyLocalClientLinkRemark(t *testing.T, raw json.RawMessage, want string) {
	t.Helper()
	links := []map[string]string{}
	if err := json.Unmarshal(raw, &links); err != nil {
		t.Fatalf("decode client links: %v", err)
	}
	if len(links) == 0 {
		t.Fatalf("client links are empty, want a local link for %q", want)
	}
	for _, link := range links {
		if link["type"] == "local" && link["remark"] == want {
			return
		}
	}
	t.Fatalf("local link remarks = %#v, want %q", links, want)
}
