package service

import (
	"crypto/x509"
	"net"
	"testing"
	"time"

	"github.com/alireza0/s-ui/database/model"
)

func TestSelfSignedAuthorityTemplateControlsCertificateChainAndSANs(t *testing.T) {
	authority := &model.SelfSignedAuthority{
		PlatformCode: "example-local-ca",
		PlatformName: "Example Local CA",
		SubjectCN:    "Example Issuing CA",
		Organization: "Example Issuer Org",
		Department:   "TLS Issuing",
		Country:      "CN",
		Province:     "Zhejiang",
		City:         "Hangzhou",
		IssuerName:   "Example Root CA",
		IssuerOrg:    "Example Root Org",
		CAURL:        "https://ca.example.test/issuer.pem",
		OCSPURL:      "https://ocsp.example.test",
		CRLURL:       "https://crl.example.test/issuer.crl",
		KeyUsage:     "Digital Signature, Key Encipherment",
		ExtKeyUsage:  "Server Auth, Client Auth",
	}
	template := buildSelfSignedAuthorityTemplate(authority)
	if template == nil {
		t.Fatal("expected authority template")
	}

	service := &ServerService{}
	_, fullchainPEM, err := service.generateCertWithTemplateForNames(
		[]string{"edge.example.test", "api.example.test", "2001:db8::7"},
		"ecc256",
		"ecc256",
		tlsCertificateUsageServer,
		time.Now(),
		time.Now().Add(24*time.Hour),
		template,
	)
	if err != nil {
		t.Fatalf("generate certificate failed: %v", err)
	}

	certs := parseCertificatesFromPEM(t, fullchainPEM)
	if len(certs) != 3 {
		t.Fatalf("expected fullchain with 3 certificates, got %d", len(certs))
	}
	leaf, intermediate, root := certs[0], certs[1], certs[2]
	if leaf.Subject.CommonName != "edge.example.test" {
		t.Fatalf("unexpected leaf CN: %q", leaf.Subject.CommonName)
	}
	if len(leaf.DNSNames) != 2 || leaf.DNSNames[0] != "edge.example.test" || leaf.DNSNames[1] != "api.example.test" {
		t.Fatalf("expected both DNS SANs, got %#v", leaf.DNSNames)
	}
	if len(leaf.IPAddresses) != 1 || !leaf.IPAddresses[0].Equal(net.ParseIP("2001:db8::7")) {
		t.Fatalf("expected IPv6 SAN, got %#v", leaf.IPAddresses)
	}
	if leaf.KeyUsage&x509.KeyUsageDigitalSignature == 0 || leaf.KeyUsage&x509.KeyUsageKeyEncipherment == 0 {
		t.Fatalf("leaf key usages did not use authority settings: %#v", leaf.KeyUsage)
	}
	if !hasExtKeyUsage(leaf.ExtKeyUsage, x509.ExtKeyUsageServerAuth) || !hasExtKeyUsage(leaf.ExtKeyUsage, x509.ExtKeyUsageClientAuth) {
		t.Fatalf("leaf extended key usages did not use authority settings: %#v", leaf.ExtKeyUsage)
	}
	if intermediate.Subject.CommonName != authority.SubjectCN || len(intermediate.Subject.Organization) != 1 || intermediate.Subject.Organization[0] != authority.Organization {
		t.Fatalf("intermediate did not use authority subject: %#v", intermediate.Subject)
	}
	if intermediate.Subject.OrganizationalUnit[0] != authority.Department {
		t.Fatalf("unexpected intermediate organizational unit: %#v", intermediate.Subject.OrganizationalUnit)
	}
	if root.Subject.CommonName != authority.IssuerName || len(root.Subject.Organization) != 1 || root.Subject.Organization[0] != authority.IssuerOrg {
		t.Fatalf("root did not use authority issuer: %#v", root.Subject)
	}
	if len(intermediate.IssuingCertificateURL) != 1 || intermediate.IssuingCertificateURL[0] != authority.CAURL {
		t.Fatalf("unexpected issuing certificate URL: %#v", intermediate.IssuingCertificateURL)
	}
	if len(intermediate.OCSPServer) != 1 || intermediate.OCSPServer[0] != authority.OCSPURL {
		t.Fatalf("unexpected OCSP server: %#v", intermediate.OCSPServer)
	}
	if len(intermediate.CRLDistributionPoints) != 1 || intermediate.CRLDistributionPoints[0] != authority.CRLURL {
		t.Fatalf("unexpected CRL distribution points: %#v", intermediate.CRLDistributionPoints)
	}
}

func TestSelfSignedAuthorityUsageValidationRejectsUnknownValues(t *testing.T) {
	if err := validateSelfSignedAuthorityKeyUsage("Digital Signature, Unknown Usage"); err == nil {
		t.Fatal("expected unknown key usage to be rejected")
	}
	if err := validateSelfSignedAuthorityExtKeyUsage("Server Auth, Unknown Usage"); err == nil {
		t.Fatal("expected unknown extended key usage to be rejected")
	}
}
