package service

import (
	"testing"
	"time"

	"github.com/alireza0/s-ui/database/model"
)

func TestAutoRenewReadsCertificateRecordByItsOwnID(t *testing.T) {
	db := setupMihomoSyncTestDB(t, "acme-auto-renew-inventory-id.db")
	_ = upsertTestCertificateRecord(t, "dummy-before-acme.example.com")

	record := &model.CertificateRecord{
		SourceType:      CertificateSourceACME,
		SourceRef:       "managed-auto-renew-id",
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
	if err := db.Create(record).Error; err != nil {
		t.Fatalf("create certificate record failed: %v", err)
	}

	got, err := certificateInventory.GetRecordByID(record.Id)
	if err != nil {
		t.Fatalf("load certificate record by id failed: %v", err)
	}
	if got.Id != record.Id {
		t.Fatalf("record id=%d, want=%d", got.Id, record.Id)
	}
	if got.SourceType != CertificateSourceACME || got.SourceRef != "managed-auto-renew-id" {
		t.Fatalf("unexpected ACME inventory source: %#v", got)
	}
}
