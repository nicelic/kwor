package service

import (
	"testing"
	"time"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
)

func TestCertificateRecordIDForACMEEntryUsesInventoryIDWhenIDsDiffer(t *testing.T) {
	db := setupMihomoSyncTestDB(t, "acme-auto-renew-inventory-id.db")
	_ = upsertTestCertificateRecord(t, "dummy-before-acme.example.com")

	entry := &model.AcmeCertificate{
		MainDomain:      "auto-renew-id.example.com",
		DomainSet:       `["auto-renew-id.example.com"]`,
		CertificateType: "domain",
		Challenge:       "standalone",
		KeyLength:       "ec-256",
		AutoRenew:       true,
		CertPEM:         []byte("test-cert"),
		KeyPEM:          []byte("test-key"),
		FullchainPEM:    []byte("test-cert"),
		NotBefore:       time.Now().Add(-time.Hour).Unix(),
		NotAfter:        time.Now().Add(24 * time.Hour).Unix(),
	}
	if err := db.Create(entry).Error; err != nil {
		t.Fatalf("create acme certificate failed: %v", err)
	}

	record, err := upsertInventoryFromAcme(entry)
	if err != nil {
		t.Fatalf("upsert inventory from acme failed: %v", err)
	}
	if record.Id == entry.Id {
		t.Fatalf("test setup failed: expected inventory id to differ from acme id, both=%d", record.Id)
	}

	got, err := certificateRecordIDForACMEEntry(entry)
	if err != nil {
		t.Fatalf("certificateRecordIDForACMEEntry failed: %v", err)
	}
	if got != record.Id {
		t.Fatalf("record id = %d, want inventory id %d (acme id %d)", got, record.Id, entry.Id)
	}

	stored := &model.CertificateRecord{}
	if err := database.GetDB().Where("id = ?", got).First(stored).Error; err != nil {
		t.Fatalf("load resolved inventory record failed: %v", err)
	}
	if stored.SourceType != CertificateSourceACME || stored.SourceRef != "1" {
		t.Fatalf("unexpected resolved inventory source: %#v", stored)
	}
}
