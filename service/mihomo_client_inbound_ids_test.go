package service

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	"github.com/alireza0/s-ui/database/model"
)

func TestMihomoInboundClientBindingMutationsNormalizeLegacyInboundIDs(t *testing.T) {
	db := initMihomoManagerFinalFallbackTestDB(t, "mihomo-client-legacy-inbound-ids.db")
	createInbound := func(tag string, port int) model.MihomoInbound {
		inbound := model.MihomoInbound{
			Type:    "socks",
			Tag:     tag,
			Addrs:   json.RawMessage(`[]`),
			Options: json.RawMessage(fmt.Sprintf(`{"listen":"::","listen_port":%d}`, port)),
		}
		if err := db.Create(&inbound).Error; err != nil {
			t.Fatalf("create Mihomo inbound %s failed: %v", tag, err)
		}
		return inbound
	}
	inboundA := createInbound("legacy-a", 18101)
	inboundB := createInbound("legacy-b", 18102)

	legacyIDs, err := json.Marshal([]string{
		fmt.Sprintf("%d", inboundA.Id),
		fmt.Sprintf("%d", inboundB.Id),
	})
	if err != nil {
		t.Fatalf("marshal legacy inbound ids failed: %v", err)
	}
	client := model.MihomoClient{
		Enable:   true,
		Name:     "legacy-client",
		Config:   json.RawMessage(`{"socks":{"username":"legacy","password":"pass"}}`),
		Inbounds: legacyIDs,
		Links:    json.RawMessage(`[]`),
	}
	if err := db.Create(&client).Error; err != nil {
		t.Fatalf("create Mihomo client failed: %v", err)
	}

	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("begin transaction failed: %v", tx.Error)
	}
	defer tx.Rollback()

	service := &MihomoClientService{}
	if err := service.UpdateClientsOnInboundDelete(tx, inboundA.Id, inboundA.Tag); err != nil {
		t.Fatalf("delete inbound binding failed: %v", err)
	}
	var afterDelete model.MihomoClient
	if err := tx.First(&afterDelete, client.Id).Error; err != nil {
		t.Fatalf("load client after delete failed: %v", err)
	}
	assertMihomoClientInboundIDs(t, afterDelete.Inbounds, []uint{inboundB.Id})

	if err := service.UpdateClientsOnInboundAdd(tx, fmt.Sprintf("%d", client.Id), inboundA.Id, "panel.example.com"); err != nil {
		t.Fatalf("add inbound binding failed: %v", err)
	}
	var afterAdd model.MihomoClient
	if err := tx.First(&afterAdd, client.Id).Error; err != nil {
		t.Fatalf("load client after add failed: %v", err)
	}
	assertMihomoClientInboundIDs(t, afterAdd.Inbounds, []uint{inboundB.Id, inboundA.Id})
}

func TestMihomoClientBulkSaveNormalizesSharedClientFields(t *testing.T) {
	db := initMihomoManagerFinalFallbackTestDB(t, "mihomo-client-bulk-normalize.db")
	payload := json.RawMessage(`[
  {
    "enable": true,
    "name": "bulk-normalized",
    "config": {},
    "inbounds": [],
    "links": [],
    "volume": -1,
    "expiry": -1,
    "up": -100,
    "down": -200,
    "extra": 99,
    "speedLimitMbps": -50
  }
]`)

	if _, err := (&MihomoClientService{}).Save(db, "addbulk", payload, "panel.example.com"); err != nil {
		t.Fatalf("bulk client save failed: %v", err)
	}

	var client model.MihomoClient
	if err := db.Where("name = ?", "bulk-normalized").First(&client).Error; err != nil {
		t.Fatalf("load bulk client failed: %v", err)
	}
	if client.Volume != 0 || client.Expiry != 0 || client.Up != 0 || client.Down != 0 {
		t.Fatalf("bulk client statistics were not normalized: %#v", client)
	}
	if client.Extra != 31 {
		t.Fatalf("bulk client reset day = %d, want 31", client.Extra)
	}
	if client.SpeedLimitMbps != 0 {
		t.Fatalf("bulk client speed limit = %d, want 0", client.SpeedLimitMbps)
	}
	if client.LastReset <= 0 {
		t.Fatalf("bulk client last reset was not initialized: %d", client.LastReset)
	}
}

func TestMihomoInboundLinkMutationsKeepExternalSameRemark(t *testing.T) {
	db := initMihomoManagerFinalFallbackTestDB(t, "mihomo-client-external-link-remark.db")
	inbound := model.MihomoInbound{
		Type:    "socks",
		Tag:     "shared-remark",
		Addrs:   json.RawMessage(`[]`),
		Options: json.RawMessage(`{"listen":"::","listen_port":18111}`),
	}
	if err := db.Create(&inbound).Error; err != nil {
		t.Fatalf("create Mihomo inbound failed: %v", err)
	}
	client := model.MihomoClient{
		Enable:   true,
		Name:     "external-link-client",
		Config:   json.RawMessage(`{"socks":{"username":"user","password":"pass"}}`),
		Inbounds: json.RawMessage(fmt.Sprintf(`[%d]`, inbound.Id)),
		Links: json.RawMessage(`[
			{"type":"local","remark":"shared-remark","uri":"socks://local"},
			{"type":"external","remark":"shared-remark","uri":"https://example.com/external"}
		]`),
	}
	if err := db.Create(&client).Error; err != nil {
		t.Fatalf("create Mihomo client failed: %v", err)
	}

	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("begin transaction failed: %v", tx.Error)
	}
	defer tx.Rollback()

	service := &MihomoClientService{}
	if err := service.UpdateClientsOnInboundDelete(tx, inbound.Id, inbound.Tag); err != nil {
		t.Fatalf("delete inbound binding failed: %v", err)
	}
	var afterDelete model.MihomoClient
	if err := tx.First(&afterDelete, client.Id).Error; err != nil {
		t.Fatalf("load client after delete failed: %v", err)
	}
	assertMihomoClientInboundIDs(t, afterDelete.Inbounds, []uint{})
	assertExternalLinkWithRemark(t, afterDelete.Links, inbound.Tag)

	if err := service.UpdateClientsOnInboundAdd(tx, fmt.Sprintf("%d", client.Id), inbound.Id, "panel.example.com"); err != nil {
		t.Fatalf("add inbound binding failed: %v", err)
	}
	var afterAdd model.MihomoClient
	if err := tx.First(&afterAdd, client.Id).Error; err != nil {
		t.Fatalf("load client after add failed: %v", err)
	}
	assertMihomoClientInboundIDs(t, afterAdd.Inbounds, []uint{inbound.Id})
	assertExternalLinkWithRemark(t, afterAdd.Links, inbound.Tag)
}

func TestMihomoDepleteClientsReturnsLegacyStringInboundIDs(t *testing.T) {
	db := initMihomoManagerFinalFallbackTestDB(t, "mihomo-deplete-legacy-inbound-ids.db")
	client := model.MihomoClient{
		Enable:   true,
		Name:     "legacy-depleted-client",
		Inbounds: json.RawMessage(`["18121"]`),
		Volume:   100,
		Up:       100,
	}
	if err := db.Create(&client).Error; err != nil {
		t.Fatalf("create Mihomo client failed: %v", err)
	}

	ids, err := (&MihomoClientService{}).DepleteClients()
	if err != nil {
		t.Fatalf("deplete Mihomo clients failed: %v", err)
	}
	if !reflect.DeepEqual(ids, []uint{18121}) {
		t.Fatalf("depleted inbound ids = %#v, want []uint{18121}", ids)
	}
}

func TestMihomoLegacyInboundIDsPreserveTrafficAndSubscriptionOrder(t *testing.T) {
	raw := json.RawMessage(`["18131",18130,"18131"]`)
	if got := parseMihomoInboundIDs(raw); !reflect.DeepEqual(got, []uint{18131, 18130}) {
		t.Fatalf("mihomo traffic inbound ids = %#v, want []uint{18131, 18130}", got)
	}

	rank := buildSubOutboundInboundRank(raw)
	if rank[18131] != 0 || rank[18130] != 1 || len(rank) != 2 {
		t.Fatalf("subscription inbound rank = %#v, want 18131:0 and 18130:1", rank)
	}
}

func TestMihomoBindingQueriesIgnoreLossyLegacyInboundIDs(t *testing.T) {
	db := initMihomoManagerFinalFallbackTestDB(t, "mihomo-lossy-legacy-inbound-id.db")
	inbound := model.MihomoInbound{
		Type:    "socks",
		Tag:     "strict-binding",
		Addrs:   json.RawMessage(`[]`),
		Options: json.RawMessage(`{"listen":"::","listen_port":18141}`),
	}
	if err := db.Create(&inbound).Error; err != nil {
		t.Fatalf("create Mihomo inbound failed: %v", err)
	}

	valid := model.MihomoClient{
		Enable:   true,
		Name:     "strict-valid",
		Config:   json.RawMessage(`{"socks":{"username":"valid","password":"pass"}}`),
		Inbounds: json.RawMessage(fmt.Sprintf(`["%d"]`, inbound.Id)),
		Links:    json.RawMessage(`[]`),
	}
	invalid := model.MihomoClient{
		Enable:   true,
		Name:     "strict-invalid",
		Config:   json.RawMessage(`{"socks":{"username":"invalid","password":"pass"}}`),
		Inbounds: json.RawMessage(fmt.Sprintf(`["%dx"]`, inbound.Id)),
		Links:    json.RawMessage(`[{"type":"external","remark":"keep","uri":"https://example.com"}]`),
	}
	if err := db.Create(&valid).Error; err != nil {
		t.Fatalf("create valid Mihomo client failed: %v", err)
	}
	if err := db.Create(&invalid).Error; err != nil {
		t.Fatalf("create invalid Mihomo client failed: %v", err)
	}

	rendered, err := (&MihomoInboundService{}).addUsers(db, json.RawMessage(`{"type":"socks","tag":"strict-binding"}`), inbound.Id, inbound.Type)
	if err != nil {
		t.Fatalf("render Mihomo inbound users failed: %v", err)
	}
	var payload map[string]interface{}
	if err := json.Unmarshal(rendered, &payload); err != nil {
		t.Fatalf("parse rendered Mihomo inbound failed: %v", err)
	}
	users, ok := payload["users"].([]interface{})
	if !ok || len(users) != 1 {
		t.Fatalf("rendered users = %#v, want only the valid binding", payload["users"])
	}
	user, ok := users[0].(map[string]interface{})
	if !ok || user["username"] != "valid" {
		t.Fatalf("rendered user = %#v, want strict-valid", users[0])
	}

	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("begin transaction failed: %v", tx.Error)
	}
	defer tx.Rollback()
	if err := (&MihomoClientService{}).UpdateClientsOnInboundDelete(tx, inbound.Id, inbound.Tag); err != nil {
		t.Fatalf("remove inbound binding failed: %v", err)
	}

	var afterValid, afterInvalid model.MihomoClient
	if err := tx.First(&afterValid, valid.Id).Error; err != nil {
		t.Fatalf("load valid client after delete failed: %v", err)
	}
	if err := tx.First(&afterInvalid, invalid.Id).Error; err != nil {
		t.Fatalf("load invalid client after delete failed: %v", err)
	}
	assertMihomoClientInboundIDs(t, afterValid.Inbounds, []uint{})
	if !bytes.Equal(afterInvalid.Inbounds, invalid.Inbounds) {
		t.Fatalf("lossy legacy binding was modified: got %s, want %s", afterInvalid.Inbounds, invalid.Inbounds)
	}
	if !bytes.Equal(afterInvalid.Links, invalid.Links) {
		t.Fatalf("lossy legacy binding links were modified: got %s, want %s", afterInvalid.Links, invalid.Links)
	}
}

func TestMihomoSnellAndShadowQUICStrictlyMatchLegacyInboundIDs(t *testing.T) {
	db := initMihomoManagerFinalFallbackTestDB(t, "mihomo-strict-legacy-credential-bindings.db")
	snell := model.MihomoInbound{Type: "snell", Tag: "strict-snell"}
	if err := db.Create(&snell).Error; err != nil {
		t.Fatalf("create Snell inbound failed: %v", err)
	}
	validSnell := model.MihomoClient{
		Enable:   true,
		Name:     "strict-snell-valid",
		Config:   json.RawMessage(`{"snell":{"psk":"valid-psk"}}`),
		Inbounds: json.RawMessage(fmt.Sprintf(`["%d"]`, snell.Id)),
	}
	invalidSnell := model.MihomoClient{
		Enable:   true,
		Name:     "strict-snell-invalid",
		Config:   json.RawMessage(`{"snell":{"psk":"invalid-psk"}}`),
		Inbounds: json.RawMessage(fmt.Sprintf(`["%d.5"]`, snell.Id)),
	}
	if err := db.Create(&validSnell).Error; err != nil {
		t.Fatalf("create valid Snell client failed: %v", err)
	}
	if err := db.Create(&invalidSnell).Error; err != nil {
		t.Fatalf("create invalid Snell client failed: %v", err)
	}

	psk, err := (&MihomoInboundService{}).resolveSnellSharedPSK(db, snell.Id)
	if err != nil {
		t.Fatalf("resolve Snell psk failed: %v", err)
	}
	if psk != "valid-psk" {
		t.Fatalf("Snell psk = %q, want valid-psk", psk)
	}

	shadowQUIC := model.MihomoInbound{Type: "shadowquic", Tag: "strict-shadowquic"}
	if err := db.Create(&shadowQUIC).Error; err != nil {
		t.Fatalf("create ShadowQUIC inbound failed: %v", err)
	}
	validShadowQUIC := model.MihomoClient{
		Enable:   true,
		Name:     "strict-shadowquic-valid",
		Config:   json.RawMessage(`{}`),
		Inbounds: json.RawMessage(fmt.Sprintf(`["%d"]`, shadowQUIC.Id)),
	}
	invalidShadowQUIC := model.MihomoClient{
		Enable:   true,
		Name:     "strict-shadowquic-invalid",
		Config:   json.RawMessage(`{"keep":"unrelated"}`),
		Inbounds: json.RawMessage(fmt.Sprintf(`["%dx"]`, shadowQUIC.Id)),
	}
	if err := db.Create(&validShadowQUIC).Error; err != nil {
		t.Fatalf("create valid ShadowQUIC client failed: %v", err)
	}
	if err := db.Create(&invalidShadowQUIC).Error; err != nil {
		t.Fatalf("create invalid ShadowQUIC client failed: %v", err)
	}

	if err := repairMihomoShadowQUICInboundClientCredentials(db, shadowQUIC.Id); err != nil {
		t.Fatalf("repair ShadowQUIC credentials failed: %v", err)
	}
	var afterValid, afterInvalid model.MihomoClient
	if err := db.First(&afterValid, validShadowQUIC.Id).Error; err != nil {
		t.Fatalf("load valid ShadowQUIC client failed: %v", err)
	}
	if err := db.First(&afterInvalid, invalidShadowQUIC.Id).Error; err != nil {
		t.Fatalf("load invalid ShadowQUIC client failed: %v", err)
	}
	var validConfig map[string]interface{}
	if err := json.Unmarshal(afterValid.Config, &validConfig); err != nil {
		t.Fatalf("parse repaired ShadowQUIC config failed: %v", err)
	}
	if _, exists := validConfig["shadowquic"]; !exists {
		t.Fatalf("valid ShadowQUIC client did not receive credentials: %#v", validConfig)
	}
	if !bytes.Equal(afterInvalid.Config, invalidShadowQUIC.Config) {
		t.Fatalf("lossy ShadowQUIC binding config was modified: got %s, want %s", afterInvalid.Config, invalidShadowQUIC.Config)
	}
}

func TestMihomoClientSaveRejectsMissingAndNonSelectableInboundTargets(t *testing.T) {
	db := initMihomoManagerFinalFallbackTestDB(t, "mihomo-client-invalid-targets.db")
	service := &MihomoClientService{}

	missingPayload := json.RawMessage(`{
  "enable": true,
  "name": "missing-target",
  "config": {},
  "inbounds": [999999],
  "links": []
}`)
	if _, err := service.Save(db, "new", missingPayload, "panel.example.com"); err == nil || !strings.Contains(err.Error(), "does not exist") {
		t.Fatalf("expected missing inbound to be rejected, got %v", err)
	}

	direct := model.MihomoInbound{Type: "direct", Tag: "no-user-listener"}
	if err := db.Create(&direct).Error; err != nil {
		t.Fatalf("create non-selectable inbound: %v", err)
	}
	nonSelectablePayload := json.RawMessage(fmt.Sprintf(`{
  "enable": true,
  "name": "non-selectable-target",
  "config": {},
  "inbounds": [%d],
  "links": []
}`, direct.Id))
	if _, err := service.Save(db, "new", nonSelectablePayload, "panel.example.com"); err == nil || !strings.Contains(err.Error(), "cannot be assigned") {
		t.Fatalf("expected non-selectable inbound to be rejected, got %v", err)
	}
}

func TestMihomoInboundSaveRejectsTypeChangeThatOrphansClientBindings(t *testing.T) {
	db := initMihomoManagerFinalFallbackTestDB(t, "mihomo-inbound-orphan-bindings.db")
	inbound := model.MihomoInbound{
		Type:    "socks",
		Tag:     "bound-socks",
		Addrs:   json.RawMessage(`[]`),
		Options: json.RawMessage(`{"listen":"::","listen_port":18151}`),
	}
	if err := db.Create(&inbound).Error; err != nil {
		t.Fatalf("create selectable inbound: %v", err)
	}
	client := model.MihomoClient{
		Enable:   true,
		Name:     "bound-client",
		Config:   json.RawMessage(`{"socks":{"username":"bound-client","password":"secret"}}`),
		Inbounds: json.RawMessage(fmt.Sprintf(`[%d]`, inbound.Id)),
		Links:    json.RawMessage(`[]`),
	}
	if err := db.Create(&client).Error; err != nil {
		t.Fatalf("create bound client: %v", err)
	}

	payload := json.RawMessage(fmt.Sprintf(`{
  "id": %d,
  "type": "direct",
  "tag": "bound-socks",
  "listen": "::",
  "listen_port": 18151
}`, inbound.Id))
	if _, err := (&MihomoInboundService{}).Save(db, "edit", payload, "", "panel.example.com"); err == nil || !strings.Contains(err.Error(), "still has bound clients") {
		t.Fatalf("expected incompatible inbound type change to be rejected, got %v", err)
	}
}

func assertMihomoClientInboundIDs(t *testing.T, raw json.RawMessage, want []uint) {
	t.Helper()
	var strict []uint
	if err := json.Unmarshal(raw, &strict); err != nil {
		t.Fatalf("inbound ids are not canonical numeric JSON: %s: %v", raw, err)
	}
	if !reflect.DeepEqual(strict, want) {
		t.Fatalf("inbound ids = %#v, want %#v", strict, want)
	}
}

func assertExternalLinkWithRemark(t *testing.T, raw json.RawMessage, remark string) {
	t.Helper()
	var links []map[string]string
	if err := json.Unmarshal(raw, &links); err != nil {
		t.Fatalf("parse client links failed: %v", err)
	}
	for _, link := range links {
		if link["type"] == "external" && link["remark"] == remark && link["uri"] == "https://example.com/external" {
			return
		}
	}
	t.Fatalf("external link with remark %q was removed: %#v", remark, links)
}
