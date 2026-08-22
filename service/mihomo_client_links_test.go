package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/alireza0/s-ui/database/model"
)

func TestUpdateMihomoClientLinksUsesEachClientInboundSelection(t *testing.T) {
	db := initMihomoManagerFinalFallbackTestDB(t, "mihomo-client-link-selections.db")
	inboundA := model.MihomoInbound{
		Type:    "socks",
		Tag:     "socks-a",
		Addrs:   json.RawMessage(`[]`),
		Options: json.RawMessage(`{"listen":"::","listen_port":18090}`),
	}
	inboundB := model.MihomoInbound{
		Type:    "socks",
		Tag:     "socks-b",
		Addrs:   json.RawMessage(`[]`),
		Options: json.RawMessage(`{"listen":"::","listen_port":18091}`),
	}
	if err := db.Create(&inboundA).Error; err != nil {
		t.Fatalf("create first mihomo inbound failed: %v", err)
	}
	if err := db.Create(&inboundB).Error; err != nil {
		t.Fatalf("create second mihomo inbound failed: %v", err)
	}

	clients := []*model.MihomoClient{
		{
			Name:     "alpha",
			Config:   json.RawMessage(`{"socks":{"username":"alpha","password":"alpha-pass"}}`),
			Inbounds: json.RawMessage(`[` + jsonNumber(inboundA.Id) + `]`),
			Links:    json.RawMessage(`[]`),
		},
		{
			Name:     "bravo",
			Config:   json.RawMessage(`{"socks":{"username":"bravo","password":"bravo-pass"}}`),
			Inbounds: json.RawMessage(`[` + jsonNumber(inboundB.Id) + `]`),
			Links:    json.RawMessage(`[]`),
		},
	}

	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("begin transaction failed: %v", tx.Error)
	}
	defer tx.Rollback()

	inboundIDs, err := (&MihomoClientService{}).updateLinksForClients(tx, clients, "panel.example.com")
	if err != nil {
		t.Fatalf("update client links failed: %v", err)
	}
	if len(inboundIDs) != 2 || inboundIDs[0] != inboundA.Id || inboundIDs[1] != inboundB.Id {
		t.Fatalf("affected inbound ids = %#v, want [%d %d]", inboundIDs, inboundA.Id, inboundB.Id)
	}

	assertSingleSocksLinkPort := func(client *model.MihomoClient, wantPort string) {
		t.Helper()
		var links []map[string]string
		if err := json.Unmarshal(client.Links, &links); err != nil {
			t.Fatalf("parse links for %s failed: %v", client.Name, err)
		}
		if len(links) != 1 || !strings.Contains(links[0]["uri"], wantPort) {
			t.Fatalf("links for %s = %#v, want only port %s", client.Name, links, wantPort)
		}
	}
	assertSingleSocksLinkPort(clients[0], ":18090")
	assertSingleSocksLinkPort(clients[1], ":18091")
}

func jsonNumber(value uint) string {
	raw, _ := json.Marshal(value)
	return string(raw)
}
