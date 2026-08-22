package service

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
)

func TestInboundServiceSaveRejectsRemovedShadowTLSRuntimeTagReference(t *testing.T) {
	openOutboundGroupOrderTestDB(t)
	db := database.GetDB()
	inbound := model.Inbound{
		Type:    "shadowtls",
		Tag:     "stls-listener",
		Addrs:   json.RawMessage(`[]`),
		OutJson: json.RawMessage(`{}`),
		Options: json.RawMessage(`{
			"listen":"::",
			"listen_port":443,
			"version":3,
			"handshake":{"server":"example.com","server_port":443},
			"ss_config":{"method":"2022-blake3-aes-128-gcm","password":"ss-secret"}
		}`),
	}
	if err := db.Create(&inbound).Error; err != nil {
		t.Fatalf("create ShadowTLS inbound: %v", err)
	}
	if err := db.Save(&model.Setting{Key: "config", Value: `{"route":{"rules":[{"inbound":["stls-listener-in"],"action":"sniff"}]}}`}).Error; err != nil {
		t.Fatalf("save route fixture: %v", err)
	}

	payload := json.RawMessage(`{
		"id": ` + strconv.FormatUint(uint64(inbound.Id), 10) + `,
		"type":"shadowtls",
		"tag":"stls-listener",
		"addrs":[],
		"out_json":{},
		"listen":"::",
		"listen_port":443,
		"version":3,
		"handshake":{"server":"example.com","server_port":443}
	}`)
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("begin transaction: %v", tx.Error)
	}
	_, err := (&InboundService{}).Save(tx, "edit", payload, "", "panel.example.com")
	tx.Rollback()
	if err == nil || !strings.Contains(err.Error(), "route rule #1") || !strings.Contains(err.Error(), "stls-listener-in") {
		t.Fatalf("expected removed ShadowTLS inbound reference rejection, got %v", err)
	}
}

func TestInboundServiceSaveRejectsRemovedDetourRouteReference(t *testing.T) {
	openOutboundGroupOrderTestDB(t)
	db := database.GetDB()
	inbound := model.Inbound{
		Type:    "direct",
		Tag:     "front-listener",
		Addrs:   json.RawMessage(`[]`),
		OutJson: json.RawMessage(`{}`),
		Options: json.RawMessage(`{"listen":"::","listen_port":8443,"detour":"inner-listener"}`),
	}
	if err := db.Create(&inbound).Error; err != nil {
		t.Fatalf("create detour inbound: %v", err)
	}
	if err := db.Save(&model.Setting{Key: "config", Value: `{"route":{"rules":[{"inbound":["inner-listener"],"action":"sniff"}]}}`}).Error; err != nil {
		t.Fatalf("save route fixture: %v", err)
	}

	payload := json.RawMessage(`{
		"id": ` + strconv.FormatUint(uint64(inbound.Id), 10) + `,
		"type":"direct",
		"tag":"front-listener",
		"addrs":[],
		"out_json":{},
		"listen":"::",
		"listen_port":8443
	}`)
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("begin transaction: %v", tx.Error)
	}
	_, err := (&InboundService{}).Save(tx, "edit", payload, "", "panel.example.com")
	tx.Rollback()
	if err == nil || !strings.Contains(err.Error(), "route rule #1") || !strings.Contains(err.Error(), "inner-listener") {
		t.Fatalf("expected removed detour route reference rejection, got %v", err)
	}
}

func TestEndpointServiceDeleteRejectsRemovedRouteTagReference(t *testing.T) {
	openOutboundGroupOrderTestDB(t)
	db := database.GetDB()
	endpoint := model.Endpoint{
		Type:    "direct",
		Tag:     "endpoint-listener",
		Options: json.RawMessage(`{"listen_port":9443,"detour":"endpoint-inner"}`),
	}
	if err := db.Create(&endpoint).Error; err != nil {
		t.Fatalf("create endpoint: %v", err)
	}
	if err := db.Save(&model.Setting{Key: "config", Value: `{"route":{"rules":[{"inbound":["endpoint-inner"],"action":"sniff"}]}}`}).Error; err != nil {
		t.Fatalf("save route fixture: %v", err)
	}

	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("begin transaction: %v", tx.Error)
	}
	err := (&EndpointService{}).Save(tx, "del", json.RawMessage(`"endpoint-listener"`))
	tx.Rollback()
	if err == nil || !strings.Contains(err.Error(), "route rule #1") || !strings.Contains(err.Error(), "endpoint-inner") {
		t.Fatalf("expected removed endpoint route reference rejection, got %v", err)
	}
}

func TestSingboxRuntimeInboundTagCollisionIsRejected(t *testing.T) {
	openOutboundGroupOrderTestDB(t)
	db := database.GetDB()
	if err := db.Create(&model.Inbound{
		Type:    "trojan",
		Tag:     "stls-listener-in",
		Addrs:   json.RawMessage(`[]`),
		OutJson: json.RawMessage(`{}`),
		Options: json.RawMessage(`{"listen":"::","listen_port":444}`),
	}).Error; err != nil {
		t.Fatalf("create colliding inbound: %v", err)
	}
	if err := db.Create(&model.Inbound{
		Type:    "shadowtls",
		Tag:     "stls-listener",
		Addrs:   json.RawMessage(`[]`),
		OutJson: json.RawMessage(`{}`),
		Options: json.RawMessage(`{"listen":"::","listen_port":443,"version":3,"ss_config":{"method":"2022-blake3-aes-128-gcm","password":"ss-secret"}}`),
	}).Error; err != nil {
		t.Fatalf("create ShadowTLS inbound: %v", err)
	}

	if err := validateSingboxStoredRuntimeInboundTags(db); err == nil || !strings.Contains(err.Error(), "runtime inbound tag") || !strings.Contains(err.Error(), "stls-listener-in") {
		t.Fatalf("expected runtime inbound tag collision, got %v", err)
	}
	if _, err := (&ProManagerService{ConfigService: &ConfigService{}}).GenerateFullConfig(); err == nil || !strings.Contains(err.Error(), "runtime inbounds") {
		t.Fatalf("final config generation must reject the collision, got %v", err)
	}
}

func TestSaveSingboxRouteRejectsUnknownInbound(t *testing.T) {
	stubSingboxRouteRuntimeRegeneration(t)
	settingService := initPanelSQLiteSettingTestDB(t)
	if err := settingService.SaveSetting("config", `{"route":{"rules":[],"rule_set":[]}}`); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	configService := &ConfigService{}
	context, err := configService.GetSingboxRouteEditorContext()
	if err != nil {
		t.Fatalf("load route context: %v", err)
	}
	_, err = configService.SaveSingboxRoute(SingboxRouteSaveRequest{
		ExpectedRevision: context.Revision,
		Route:            json.RawMessage(`{"rules":[{"inbound":["missing-listener"],"action":"sniff"}],"rule_set":[]}`),
	}, "test")
	if err == nil || !strings.Contains(err.Error(), "unknown inbound") || !strings.Contains(err.Error(), "missing-listener") {
		t.Fatalf("expected unknown inbound rejection, got %v", err)
	}
}
