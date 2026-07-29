package api

import (
	"strings"
	"testing"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
)

func TestSaveAcmeAccountRoutePreservesOmittedFieldsAndAllowsEmptyLetsEncryptEmail(t *testing.T) {
	initAcmeRouteTestDB(t)
	db := database.GetDB()

	zerossl := &model.AcmeAccount{
		Name:             "zero-old",
		Email:            "ops@example.com",
		Server:           "zerossl",
		KeyLength:        "rsa-4096",
		AccountKeyLength: "rsa-4096",
		Remark:           "keep this remark",
		Registered:       true,
	}
	if err := db.Create(zerossl).Error; err != nil {
		t.Fatalf("create ZeroSSL account failed: %v", err)
	}

	_, msg := performAcmeRouteJSONPost(t, (&ApiService{}).SaveAcmeAccount, `{"id":`+uintJSON(zerossl.Id)+`,"name":"zero-renamed"}`)
	if !msg.Success {
		t.Fatalf("partial ACME account update failed: %#v", msg)
	}
	if err := db.Where("id = ?", zerossl.Id).First(zerossl).Error; err != nil {
		t.Fatalf("reload ZeroSSL account failed: %v", err)
	}
	if zerossl.Email != "ops@example.com" || zerossl.Server != "zerossl" || zerossl.AccountKeyLength != "rsa-4096" || zerossl.Remark != "keep this remark" {
		t.Fatalf("omitted account fields were lost: %#v", zerossl)
	}

	letsEncrypt := &model.AcmeAccount{
		Name:             "le-old",
		Email:            "contact@example.com",
		Server:           "letsencrypt",
		KeyLength:        "ec-256",
		AccountKeyLength: "ec-256",
	}
	if err := db.Create(letsEncrypt).Error; err != nil {
		t.Fatalf("create Let's Encrypt account failed: %v", err)
	}
	_, msg = performAcmeRouteJSONPost(t, (&ApiService{}).SaveAcmeAccount, `{"id":`+uintJSON(letsEncrypt.Id)+`,"name":"le-old","email":""}`)
	if !msg.Success {
		t.Fatalf("explicit empty Let's Encrypt email should be accepted: %#v", msg)
	}
	if err := db.Where("id = ?", letsEncrypt.Id).First(letsEncrypt).Error; err != nil {
		t.Fatalf("reload Let's Encrypt account failed: %v", err)
	}
	if letsEncrypt.Email != "" {
		t.Fatalf("explicit empty email was not stored: %#v", letsEncrypt)
	}
}

func TestSaveAcmeDNSAccountRoutePreservesOmittedCredentials(t *testing.T) {
	initAcmeRouteTestDB(t)
	db := database.GetDB()
	dnsAccount := &model.AcmeDNSAccount{
		Name:         "cloudflare-old",
		ProviderName: "Cloudflare",
		ProviderCode: "dns_cf",
		EnvJSON:      `{"CF_Token":"stored-token","CF_Zone_ID":"zone-id"}`,
		Remark:       "keep dns remark",
	}
	if err := db.Create(dnsAccount).Error; err != nil {
		t.Fatalf("create DNS account failed: %v", err)
	}

	_, msg := performAcmeRouteJSONPost(t, (&ApiService{}).SaveAcmeDNSAccount, `{"id":`+uintJSON(dnsAccount.Id)+`,"name":"cloudflare-renamed","providerCode":"dns_cf"}`)
	if !msg.Success {
		t.Fatalf("partial DNS account update failed: %#v", msg)
	}
	if err := db.Where("id = ?", dnsAccount.Id).First(dnsAccount).Error; err != nil {
		t.Fatalf("reload DNS account failed: %v", err)
	}
	if dnsAccount.EnvJSON != `{"CF_Token":"stored-token","CF_Zone_ID":"zone-id"}` || dnsAccount.Remark != "keep dns remark" {
		t.Fatalf("omitted DNS fields were lost: %#v", dnsAccount)
	}
}

func TestReissueIPRouteDoesNotRequireCertificateTypeOrAccount(t *testing.T) {
	initAcmeRouteTestDB(t)

	_, msg := performAcmeRouteJSONPost(t, (&ApiService{}).ReissueAcmeCertificate, `{"existingRecordId":999}`)
	if strings.Contains(msg.Msg, "acmeAccountId is required for domain certificate") {
		t.Fatalf("reissue with omitted certificateType must defer account validation to the existing record: %#v", msg)
	}
}
