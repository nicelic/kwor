package service

import (
	"testing"

	"github.com/alireza0/s-ui/database/model"
)

func TestApplyAcmeReissueDefaultsUsesExistingCertificateRecord(t *testing.T) {
	existing := &model.CertificateRecord{
		Id:                 42,
		SourceType:         CertificateSourceACME,
		SourceRef:          "managed-reissue-defaults",
		MainDomain:         "example.com",
		DomainSet:          `["example.com","www.example.com"]`,
		CertificateType:    acmeCertificateTypeDomain,
		Challenge:          "dns",
		Webroot:            "/srv/www/example.com",
		DNSProvider:        "dns_cf",
		KeyLength:          "ec-384",
		CustomArgs:         "--ocsp",
		AcmeAccountID:      8,
		AcmeAccountName:    "primary",
		DNSAccountID:       9,
		DNSAccountName:     "cloudflare",
		AutoRenew:          true,
		Remark:             "existing remark",
		ApplyTarget:        "panel,sub",
		PushEnabled:        true,
		PushDir:            "/etc/ssl/example.com",
		AcmeRuntimeProfile: "",
	}
	payload := &AcmeIssuePayload{ExistingRecordID: existing.Id}

	applyAcmeReissueDefaults(payload, existing)

	if payload.CertificateType != acmeCertificateTypeDomain || payload.DomainsText != "example.com,www.example.com" {
		t.Fatalf("certificate identity defaults were not restored: %#v", payload)
	}
	if payload.Challenge != "dns" || payload.Webroot != "/srv/www/example.com" || payload.DNSProvider != "dns_cf" {
		t.Fatalf("challenge defaults were not restored: %#v", payload)
	}
	if payload.KeyLength != "ec-384" || payload.CustomArgs != "--ocsp" {
		t.Fatalf("certificate options were not restored: %#v", payload)
	}
	if payload.AcmeAccountID != 8 || payload.DNSAccountID != 9 || !payload.AutoRenew {
		t.Fatalf("account bindings or renew state were not restored: %#v", payload)
	}
	if payload.Remark != "existing remark" || payload.ApplyTarget != "panel,sub" || payload.PushDir != "/etc/ssl/example.com" {
		t.Fatalf("post-issue defaults were not restored: %#v", payload)
	}
	if payload.PushExplicit {
		t.Fatal("an omitted pushDir must not push the certificate again")
	}
	if payload.DNSEnvText != "" {
		t.Fatalf("old manual DNS env must not be restored: %q", payload.DNSEnvText)
	}
}

func TestApplyAcmeReissueDefaultsKeepsExplicitOverrides(t *testing.T) {
	existing := &model.CertificateRecord{
		MainDomain:      "example.com",
		DomainSet:       `["example.com"]`,
		CertificateType: acmeCertificateTypeDomain,
		Challenge:       "dns",
		DNSProvider:     "dns_cf",
		KeyLength:       "ec-256",
		AcmeAccountID:   8,
		DNSAccountID:    9,
		AutoRenew:       true,
		Remark:          "old",
		ApplyTarget:     "panel",
		PushDir:         "/old/push",
	}
	payload := &AcmeIssuePayload{
		ExistingRecordID:        42,
		CertificateType:         acmeCertificateTypeDomain,
		CertificateTypeProvided: true,
		KeyLength:               "rsa-4096",
		KeyLengthProvided:       true,
		AcmeAccountID:           18,
		AcmeAccountProvided:     true,
		DNSAccountID:            19,
		DNSAccountProvided:      true,
		AutoRenew:               false,
		AutoRenewProvided:       true,
		Remark:                  "new",
		RemarkProvided:          true,
		ApplyTarget:             "sub",
		ApplyTargetProvided:     true,
		PushDir:                 "/new/push",
		PushDirProvided:         true,
	}

	applyAcmeReissueDefaults(payload, existing)

	if payload.KeyLength != "rsa-4096" || payload.AcmeAccountID != 18 || payload.DNSAccountID != 19 {
		t.Fatalf("explicit certificate/account overrides were lost: %#v", payload)
	}
	if payload.AutoRenew || payload.Remark != "new" || payload.ApplyTarget != "sub" || payload.PushDir != "/new/push" {
		t.Fatalf("explicit post-issue overrides were lost: %#v", payload)
	}
	if !payload.PushExplicit {
		t.Fatal("a submitted non-empty pushDir must remain explicit")
	}
}

func TestApplyAcmeReissueDefaultsKeepsIPCertificateTypeAndNoUserBindings(t *testing.T) {
	existing := &model.CertificateRecord{
		MainDomain:         "192.0.2.10",
		DomainSet:          `["192.0.2.10"]`,
		CertificateType:    acmeCertificateTypeIP,
		Challenge:          "alpn",
		KeyLength:          "ec-384",
		AutoRenew:          true,
		AcmeRuntimeProfile: acmeIPRuntimeProfile,
	}
	payload := &AcmeIssuePayload{ExistingRecordID: 77}

	applyAcmeReissueDefaults(payload, existing)

	if payload.CertificateType != acmeCertificateTypeIP || payload.DomainsText != "192.0.2.10" || payload.Challenge != "alpn" {
		t.Fatalf("IP reissue defaults were not restored: %#v", payload)
	}
	if payload.AcmeAccountID != 0 || payload.DNSAccountID != 0 {
		t.Fatalf("IP reissue must not acquire user account bindings: %#v", payload)
	}
}
