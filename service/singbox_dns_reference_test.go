package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"gorm.io/gorm"
)

func seedSingboxDNSResolverReferenceTest(t *testing.T, db *gorm.DB) {
	t.Helper()
	setting := model.Setting{}
	err := db.Where("key = ?", "config").First(&setting).Error
	if database.IsNotFound(err) {
		setting = model.Setting{Key: "config"}
	} else if err != nil {
		t.Fatalf("load config setting: %v", err)
	}
	setting.Value = `{"dns":{"final":"dns-main","rules":[]},"route":{"rules":[]}}`
	if err := db.Save(&setting).Error; err != nil {
		t.Fatalf("save config setting: %v", err)
	}
	if err := db.Create(&model.DnsServer{
		Type:    "udp",
		Tag:     "dns-main",
		Options: json.RawMessage(`{"server":"1.1.1.1","server_port":53}`),
	}).Error; err != nil {
		t.Fatalf("create DNS server: %v", err)
	}
}

func TestSingboxDialDNSReferencesAreValidated(t *testing.T) {
	db := initOutboundEditMergeTestDB(t, "singbox-dial-dns-references.db")
	seedSingboxDNSResolverReferenceTest(t, db)

	tryOutbound := func(resolver string) error {
		tx := db.Begin()
		if tx.Error != nil {
			t.Fatalf("begin outbound transaction: %v", tx.Error)
		}
		err := (&OutboundService{}).Save(tx, "new", json.RawMessage(`{"type":"direct","tag":"dns-ref-out","domain_resolver":"`+resolver+`"}`))
		tx.Rollback()
		return err
	}
	if err := tryOutbound("missing-dns"); err == nil || !strings.Contains(err.Error(), "unknown DNS server") {
		t.Fatalf("unknown outbound DNS resolver was accepted: %v", err)
	}
	if err := tryOutbound("dns-main"); err != nil {
		t.Fatalf("valid outbound DNS resolver was rejected: %v", err)
	}

	tryEndpoint := func(resolver string) error {
		tx := db.Begin()
		if tx.Error != nil {
			t.Fatalf("begin endpoint transaction: %v", tx.Error)
		}
		err := (&EndpointService{}).Save(tx, "new", json.RawMessage(`{"type":"tailscale","tag":"dns-ref-endpoint","domain_resolver":"`+resolver+`"}`))
		tx.Rollback()
		return err
	}
	if err := tryEndpoint("missing-dns"); err == nil || !strings.Contains(err.Error(), "unknown DNS server") {
		t.Fatalf("unknown endpoint DNS resolver was accepted: %v", err)
	}
	if err := tryEndpoint("dns-main"); err != nil {
		t.Fatalf("valid endpoint DNS resolver was rejected: %v", err)
	}

	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("begin route transaction: %v", tx.Error)
	}
	_, err := normalizeAndValidateSingboxRoute(json.RawMessage(`{"default_domain_resolver":"missing-dns","rules":[]}`), tx)
	tx.Rollback()
	if err == nil || !strings.Contains(err.Error(), "unknown DNS server") {
		t.Fatalf("unknown route DNS resolver was accepted: %v", err)
	}
}

func TestSingboxDNSServerDeleteRejectsStaleResolverReferences(t *testing.T) {
	db := initOutboundEditMergeTestDB(t, "singbox-dns-delete-references.db")
	seedSingboxDNSResolverReferenceTest(t, db)
	idle := model.DnsServer{
		Type:    "udp",
		Tag:     "dns-idle",
		Options: json.RawMessage(`{"server":"9.9.9.9","server_port":53}`),
	}
	if err := db.Create(&idle).Error; err != nil {
		t.Fatalf("create idle DNS server: %v", err)
	}
	if err := db.Create(&model.Outbound{
		Type:        "direct",
		Tag:         "legacy-resolver-reference",
		RawOutbound: json.RawMessage(`{"type":"direct","tag":"legacy-resolver-reference","domain_resolver":"dns-idle"}`),
	}).Error; err != nil {
		t.Fatalf("create stale resolver outbound: %v", err)
	}

	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("begin DNS delete transaction: %v", tx.Error)
	}
	payload, err := json.Marshal(idle.Id)
	if err != nil {
		t.Fatalf("marshal DNS id: %v", err)
	}
	err = (&DnsServerService{}).Save(tx, "del", payload)
	tx.Rollback()
	if err == nil || !strings.Contains(err.Error(), "legacy-resolver-reference") {
		t.Fatalf("stale DNS resolver reference was not rejected: %v", err)
	}
}
