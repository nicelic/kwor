package service

import (
	"crypto/x509"
	"encoding/pem"
	"testing"
	"time"
)

func parseLeafCertificateFromPEM(t *testing.T, pemData []byte) *x509.Certificate {
	t.Helper()
	block, _ := pem.Decode(pemData)
	if block == nil {
		t.Fatal("failed to decode certificate PEM")
	}
	cert, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("ParseCertificate failed: %v", err)
	}
	return cert
}

func parseCertificatesFromPEM(t *testing.T, pemData []byte) []*x509.Certificate {
	t.Helper()
	certs, err := parseCertificates(pemData)
	if err != nil {
		t.Fatalf("parseCertificates failed: %v", err)
	}
	return certs
}

func hasExtKeyUsage(usages []x509.ExtKeyUsage, target x509.ExtKeyUsage) bool {
	for _, usage := range usages {
		if usage == target {
			return true
		}
	}
	return false
}

func TestGenerateCertWithAlgorithm_DefaultsToServerAuth(t *testing.T) {
	service := &ServerService{}
	_, certPEM, err := service.generateCertWithAlgorithm(
		"example.com",
		"ecc256",
		"ecc256",
		tlsCertificateUsageServer,
		time.Now(),
		time.Now().Add(24*time.Hour),
	)
	if err != nil {
		t.Fatalf("generateCertWithAlgorithm failed: %v", err)
	}

	cert := parseLeafCertificateFromPEM(t, certPEM)
	if !hasExtKeyUsage(cert.ExtKeyUsage, x509.ExtKeyUsageServerAuth) {
		t.Fatalf("expected ServerAuth usage, got %#v", cert.ExtKeyUsage)
	}
	if hasExtKeyUsage(cert.ExtKeyUsage, x509.ExtKeyUsageClientAuth) {
		t.Fatalf("did not expect ClientAuth usage, got %#v", cert.ExtKeyUsage)
	}
	if len(cert.DNSNames) != 1 || cert.DNSNames[0] != "example.com" {
		t.Fatalf("expected DNSNames to contain example.com, got %#v", cert.DNSNames)
	}
}

func TestGenerateCertWithAlgorithm_ClientUsageUsesClientAuth(t *testing.T) {
	service := &ServerService{}
	_, certPEM, err := service.generateCertWithAlgorithm(
		"client",
		"ecc256",
		"ecc256",
		tlsCertificateUsageClient,
		time.Now(),
		time.Now().Add(24*time.Hour),
	)
	if err != nil {
		t.Fatalf("generateCertWithAlgorithm failed: %v", err)
	}

	cert := parseLeafCertificateFromPEM(t, certPEM)
	if !hasExtKeyUsage(cert.ExtKeyUsage, x509.ExtKeyUsageClientAuth) {
		t.Fatalf("expected ClientAuth usage, got %#v", cert.ExtKeyUsage)
	}
	if hasExtKeyUsage(cert.ExtKeyUsage, x509.ExtKeyUsageServerAuth) {
		t.Fatalf("did not expect ServerAuth usage, got %#v", cert.ExtKeyUsage)
	}
	if len(cert.DNSNames) != 0 {
		t.Fatalf("expected client certificate to omit DNSNames, got %#v", cert.DNSNames)
	}
}

func TestGenerateCertWithTemplate_UsesTemplateChainAndPreservesLeafIdentity(t *testing.T) {
	service := &ServerService{}
	template := resolveTLSSelfSignedTemplate("letsencrypt")
	if template == nil {
		t.Fatal("expected letsencrypt template")
	}

	_, certPEM, err := service.generateCertWithTemplate(
		"edge.example.com",
		"ecc256",
		"ecc256",
		tlsCertificateUsageServer,
		time.Now(),
		time.Now().Add(24*time.Hour),
		template,
	)
	if err != nil {
		t.Fatalf("generateCertWithTemplate failed: %v", err)
	}

	certs := parseCertificatesFromPEM(t, certPEM)
	if len(certs) != 3 {
		t.Fatalf("expected fullchain with 3 certificates, got %d", len(certs))
	}

	leaf := certs[0]
	intermediate := certs[1]
	root := certs[2]

	if leaf.Subject.CommonName != "edge.example.com" {
		t.Fatalf("expected leaf CN edge.example.com, got %q", leaf.Subject.CommonName)
	}
	if len(leaf.DNSNames) != 1 || leaf.DNSNames[0] != "edge.example.com" {
		t.Fatalf("expected leaf DNSNames edge.example.com, got %#v", leaf.DNSNames)
	}
	for _, extension := range leaf.Extensions {
		if extension.Id.String() == "1.3.6.1.4.1.55555.1.1" {
			t.Fatalf("did not expect private template marker extension in leaf certificate")
		}
	}
	if intermediate.Subject.CommonName != template.Intermediate.CommonName {
		t.Fatalf("expected intermediate CN %q, got %q", template.Intermediate.CommonName, intermediate.Subject.CommonName)
	}
	if root.Subject.CommonName != template.Root.CommonName {
		t.Fatalf("expected root CN %q, got %q", template.Root.CommonName, root.Subject.CommonName)
	}
	if len(intermediate.IssuingCertificateURL) != 1 || intermediate.IssuingCertificateURL[0] != template.CAURL {
		t.Fatalf("expected issuing URL %q, got %#v", template.CAURL, intermediate.IssuingCertificateURL)
	}
	if detectTLSSelfSignedTemplateCode(certs) != template.Code {
		t.Fatalf("expected detected template %q", template.Code)
	}
}

func TestDetectTLSSelfSignedTemplate_BlanksLegacyAndPartialChains(t *testing.T) {
	service := &ServerService{}

	_, legacyPEM, err := service.generateCertWithAlgorithm(
		"legacy.example.com",
		"ecc256",
		"ecc256",
		tlsCertificateUsageServer,
		time.Now(),
		time.Now().Add(24*time.Hour),
	)
	if err != nil {
		t.Fatalf("generateCertWithAlgorithm failed: %v", err)
	}

	templateCode, err := service.DetectTLSSelfSignedTemplate("pem", "", string(legacyPEM))
	if err != nil {
		t.Fatalf("DetectTLSSelfSignedTemplate failed: %v", err)
	}
	if templateCode != "" {
		t.Fatalf("expected legacy chain to remain blank, got %q", templateCode)
	}

	template := resolveTLSSelfSignedTemplate("zerossl")
	if template == nil {
		t.Fatal("expected zerossl template")
	}
	_, templatedPEM, err := service.generateCertWithTemplate(
		"edge.example.com",
		"ecc256",
		"ecc256",
		tlsCertificateUsageServer,
		time.Now(),
		time.Now().Add(24*time.Hour),
		template,
	)
	if err != nil {
		t.Fatalf("generateCertWithTemplate failed: %v", err)
	}

	leaf := parseLeafCertificateFromPEM(t, templatedPEM)
	leafOnlyPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: leaf.Raw})
	templateCode, err = service.DetectTLSSelfSignedTemplate("pem", "", string(leafOnlyPEM))
	if err != nil {
		t.Fatalf("DetectTLSSelfSignedTemplate failed on leaf-only certificate: %v", err)
	}
	if templateCode != "" {
		t.Fatalf("expected partial chain detection to stay blank, got %q", templateCode)
	}
}

func TestGenKeypairWithTemplate_RejectsUnknownTLSTemplate(t *testing.T) {
	service := &ServerService{}

	keypair := service.GenKeypairWithTemplate("tls", "ss.cc,1,y,ecc256,ecc256", "unknown-template")
	if len(keypair) != 1 {
		t.Fatalf("expected a single error line, got %#v", keypair)
	}
	if keypair[0] != "Failed to generate TLS keypair: unknown tls self-signed template: unknown-template" {
		t.Fatalf("unexpected error line: %q", keypair[0])
	}
}
