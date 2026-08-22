package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
)

func TestSingboxServiceSaveRejectsUnknownEditorReferences(t *testing.T) {
	settingService := initPanelSQLiteSettingTestDB(t)
	if err := settingService.SaveSetting("config", `{"dns":{"rules":[]}}`); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	db := database.GetDB()
	if err := db.Create(&model.Outbound{
		Type:        "direct",
		Tag:         "service-direct",
		RawOutbound: json.RawMessage(`{"type":"direct","tag":"service-direct"}`),
	}).Error; err != nil {
		t.Fatalf("create outbound: %v", err)
	}
	if err := db.Create(&model.Inbound{
		Type:    "mixed",
		Tag:     "listener",
		Options: json.RawMessage(`{"listen":"::","listen_port":1080}`),
	}).Error; err != nil {
		t.Fatalf("create inbound: %v", err)
	}
	if err := db.Create(&model.Endpoint{
		Type:    "tailscale",
		Tag:     "ts-endpoint",
		Options: json.RawMessage(`{"listen_port":8443}`),
	}).Error; err != nil {
		t.Fatalf("create endpoint: %v", err)
	}

	tests := []struct {
		name    string
		payload string
		wantErr string
	}{
		{
			name:    "listen detour",
			payload: `{"type":"resolved","tag":"resolved-bad","listen":"::","listen_port":53,"detour":"missing-inbound"}`,
			wantErr: "detour references unknown target",
		},
		{
			name:    "derp endpoint",
			payload: `{"type":"derp","tag":"derp-bad","listen":"::","listen_port":443,"verify_client_endpoint":["missing-endpoint"]}`,
			wantErr: "verify_client_endpoint references unknown target",
		},
		{
			name:    "ssm api inbound",
			payload: `{"type":"ssm-api","tag":"ssm-bad","listen":"::","listen_port":443,"servers":{"/ss":"missing-inbound"}}`,
			wantErr: "servers./ss references unknown target",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			tx := db.Begin()
			if tx.Error != nil {
				t.Fatalf("begin transaction: %v", tx.Error)
			}
			err := (&ServicesService{}).Save(tx, "new", json.RawMessage(test.payload))
			tx.Rollback()
			if err == nil || !strings.Contains(err.Error(), test.wantErr) {
				t.Fatalf("service reference error=%v, want %q", err, test.wantErr)
			}
		})
	}

	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("begin valid transaction: %v", tx.Error)
	}
	if err := (&ServicesService{}).Save(tx, "new", json.RawMessage(`{"type":"resolved","tag":"resolved-good","listen":"::","listen_port":53,"detour":"listener"}`)); err != nil {
		t.Fatalf("valid service reference rejected: %v", err)
	}
	tx.Rollback()
}

func TestSingboxServiceDetourUsesEffectiveEndpointRouteTag(t *testing.T) {
	openOutboundGroupOrderTestDB(t)
	db := database.GetDB()
	if err := db.Create(&model.Endpoint{
		Type:    "tailscale",
		Tag:     "endpoint-panel-tag",
		Options: json.RawMessage(`{"listen_port":8443,"detour":"runtime-inbound"}`),
	}).Error; err != nil {
		t.Fatalf("create endpoint: %v", err)
	}
	if err := db.Create(&model.Inbound{
		Type:    "mixed",
		Tag:     "runtime-inbound",
		Options: json.RawMessage(`{"listen":"::","listen_port":1080}`),
	}).Error; err != nil {
		t.Fatalf("create inbound: %v", err)
	}

	for _, test := range []struct {
		name    string
		detour  string
		wantErr bool
	}{
		{name: "panel endpoint tag is rejected", detour: "endpoint-panel-tag", wantErr: true},
		{name: "effective route tag is accepted", detour: "runtime-inbound", wantErr: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			tx := db.Begin()
			if tx.Error != nil {
				t.Fatalf("begin transaction: %v", tx.Error)
			}
			err := (&ServicesService{}).Save(tx, "new", json.RawMessage(`{"type":"resolved","tag":"resolved-`+test.detour+`","listen":"::","listen_port":53,"detour":"`+test.detour+`"}`))
			tx.Rollback()
			if test.wantErr {
				if err == nil || !strings.Contains(err.Error(), "detour references unknown target") {
					t.Fatalf("detour validation error=%v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("effective route tag rejected: %v", err)
			}
		})
	}
}

func TestSingboxServiceReferencesBlockEndpointAndOutboundRemoval(t *testing.T) {
	openOutboundGroupOrderTestDB(t)
	db := database.GetDB()
	if err := db.Create(&model.Endpoint{
		Type:    "tailscale",
		Tag:     "service-endpoint",
		Options: json.RawMessage(`{"listen_port":8443}`),
	}).Error; err != nil {
		t.Fatalf("create endpoint: %v", err)
	}
	if err := db.Create(&model.Outbound{
		Type:        "direct",
		Tag:         "service-probe",
		RawOutbound: json.RawMessage(`{"type":"direct","tag":"service-probe"}`),
	}).Error; err != nil {
		t.Fatalf("create outbound: %v", err)
	}
	if err := db.Create(&model.Service{
		Type: "derp",
		Tag:  "service-references",
		Options: json.RawMessage(`{
			"listen":"::",
			"listen_port":443,
			"verify_client_endpoint":["service-endpoint"],
			"verify_client_url":[{"url":"https://example.com","detour":"service-probe"}]
		}`),
	}).Error; err != nil {
		t.Fatalf("create referencing service: %v", err)
	}

	t.Run("endpoint", func(t *testing.T) {
		tx := db.Begin()
		if tx.Error != nil {
			t.Fatalf("begin endpoint transaction: %v", tx.Error)
		}
		err := (&EndpointService{}).Save(tx, "del", json.RawMessage(`"service-endpoint"`))
		tx.Rollback()
		if err == nil || !strings.Contains(err.Error(), "service") || !strings.Contains(err.Error(), "service-endpoint") {
			t.Fatalf("endpoint removal error=%v", err)
		}
	})

	t.Run("outbound", func(t *testing.T) {
		tx := db.Begin()
		if tx.Error != nil {
			t.Fatalf("begin outbound transaction: %v", tx.Error)
		}
		err := (&OutboundService{}).Save(tx, "del", json.RawMessage(`"service-probe"`))
		tx.Rollback()
		if err == nil || !strings.Contains(err.Error(), "service") || !strings.Contains(err.Error(), "service-probe") {
			t.Fatalf("outbound removal error=%v", err)
		}
	})
}

func TestSingboxDNSCardsBlockServiceAndEndpointRemoval(t *testing.T) {
	openOutboundGroupOrderTestDB(t)
	db := database.GetDB()
	if err := db.Create(&model.Service{Type: "resolved", Tag: "dns-service", Options: json.RawMessage(`{}`)}).Error; err != nil {
		t.Fatalf("create service: %v", err)
	}
	if err := db.Create(&model.Endpoint{Type: "tailscale", Tag: "dns-endpoint", Options: json.RawMessage(`{"listen_port":443}`)}).Error; err != nil {
		t.Fatalf("create endpoint: %v", err)
	}
	if err := db.Create(&model.DnsServer{Type: "resolved", Tag: "dns-resolved", Options: json.RawMessage(`{"service":"dns-service"}`)}).Error; err != nil {
		t.Fatalf("create resolved DNS server: %v", err)
	}
	if err := db.Create(&model.DnsServer{Type: "tailscale", Tag: "dns-tailscale", Options: json.RawMessage(`{"endpoint":"dns-endpoint"}`)}).Error; err != nil {
		t.Fatalf("create tailscale DNS server: %v", err)
	}

	t.Run("service", func(t *testing.T) {
		tx := db.Begin()
		if tx.Error != nil {
			t.Fatalf("begin service transaction: %v", tx.Error)
		}
		err := (&ServicesService{}).Save(tx, "del", json.RawMessage(`"dns-service"`))
		tx.Rollback()
		if err == nil || !strings.Contains(err.Error(), "DNS server") || !strings.Contains(err.Error(), "dns-resolved") {
			t.Fatalf("service removal error=%v", err)
		}
	})

	t.Run("endpoint", func(t *testing.T) {
		tx := db.Begin()
		if tx.Error != nil {
			t.Fatalf("begin endpoint transaction: %v", tx.Error)
		}
		err := (&EndpointService{}).Save(tx, "del", json.RawMessage(`"dns-endpoint"`))
		tx.Rollback()
		if err == nil || !strings.Contains(err.Error(), "DNS server") || !strings.Contains(err.Error(), "dns-tailscale") {
			t.Fatalf("endpoint removal error=%v", err)
		}
	})
}
