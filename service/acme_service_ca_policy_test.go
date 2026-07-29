package service

import (
	"strings"
	"testing"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
)

func TestSaveAcmeAccountRejectsUnsupportedServer(t *testing.T) {
	setupAcmeDNSTestDB(t, "acme-account-ca-reject.db")
	svc := &AcmeService{}

	_, err := svc.SaveAcmeAccount(AcmeAccountPayload{
		Name:      "acc-ca-reject",
		Email:     "acc@example.com",
		Server:    "https://example.com/acme/directory",
		KeyLength: "ec-256",
	})
	if err == nil {
		t.Fatal("expected unsupported CA server to be rejected")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "letsencrypt") || !strings.Contains(strings.ToLower(err.Error()), "zerossl") {
		t.Fatalf("unexpected error for unsupported CA: %v", err)
	}
}

func TestSaveAcmeAccountNormalizesLetsEncryptURLAlias(t *testing.T) {
	setupAcmeDNSTestDB(t, "acme-account-ca-normalize.db")
	svc := &AcmeService{}

	_, err := svc.SaveAcmeAccount(AcmeAccountPayload{
		Name:      "acc-ca-normalize",
		Email:     "acc@example.com",
		Server:    acmeLEProductionDirectory,
		KeyLength: "ec-256",
	})
	if err != nil {
		t.Fatalf("save account failed: %v", err)
	}

	row := &model.AcmeAccount{}
	if err := database.GetDB().Where("name = ?", "acc-ca-normalize").First(row).Error; err != nil {
		t.Fatalf("query saved account failed: %v", err)
	}
	if row.Server != "letsencrypt" {
		t.Fatalf("expected normalized server letsencrypt, got %q", row.Server)
	}
}

func TestGetOverviewOnlyReturnsLetAndZeroCAOptions(t *testing.T) {
	setupAcmeDNSTestDB(t, "acme-overview-ca-options.db")
	svc := &AcmeService{}

	overview, err := svc.GetOverview()
	if err != nil {
		t.Fatalf("GetOverview failed: %v", err)
	}
	if len(overview.CAOptions) != 2 {
		t.Fatalf("expected 2 ca options, got %d: %#v", len(overview.CAOptions), overview.CAOptions)
	}

	got := map[string]bool{}
	for _, item := range overview.CAOptions {
		got[strings.TrimSpace(strings.ToLower(item.Value))] = true
	}
	if !got["letsencrypt"] || !got["zerossl"] {
		t.Fatalf("expected ca options letsencrypt+zerossl, got %#v", overview.CAOptions)
	}
}
