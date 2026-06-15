package web

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"path/filepath"
	"testing"
	"time"

	"github.com/alireza0/s-ui/database"
)

func TestCollectSNIMatchingTLSRuntimeCertificates_StrictMatch(t *testing.T) {
	materials := []*tlsRuntimeCertificate{
		mustTLSRuntimeCertificate(t, []string{"example.com"}, nil, "fp-1"),
		mustTLSRuntimeCertificate(t, []string{"api.example.com"}, nil, "fp-2"),
	}

	matched := collectSNIMatchingTLSRuntimeCertificates(materials, "api.example.com")
	if len(matched) != 1 || matched[0] != materials[1] {
		t.Fatalf("unexpected sni matched set: got=%d", len(matched))
	}
	if got := collectSNIMatchingTLSRuntimeCertificates(materials, "no-match.example.net"); len(got) != 0 {
		t.Fatalf("expected no match, got=%d", len(got))
	}
}

func TestCollectSNIMatchingTLSRuntimeCertificatesSkipsExpired(t *testing.T) {
	expired := mustTLSRuntimeCertificate(t, []string{"example.com"}, nil, "fp-expired")
	expired.notAfter = time.Now().Add(-time.Minute)
	valid := mustTLSRuntimeCertificate(t, []string{"example.com"}, nil, "fp-valid")
	materials := []*tlsRuntimeCertificate{expired, valid}

	matched := collectSNIMatchingTLSRuntimeCertificates(materials, "example.com")
	if len(matched) != 1 || matched[0] != valid {
		t.Fatalf("expected only valid certificate, got=%#v", matched)
	}
	if selected := selectTLSRuntimeCertificate(materials, ""); selected != valid {
		t.Fatalf("expected no-sni fallback to skip expired certificate")
	}
}

func TestSplitNoSNITLSRuntimeCertificateCandidates_PrefersIPCertAndFallsBack(t *testing.T) {
	ipCert := mustTLSRuntimeCertificate(t, nil, []string{"127.0.0.1"}, "fp-ip")
	domainCert := mustTLSRuntimeCertificate(t, []string{"example.com"}, nil, "fp-domain")
	otherIPCert := mustTLSRuntimeCertificate(t, nil, []string{"10.0.0.1"}, "fp-ip-other")
	materials := []*tlsRuntimeCertificate{domainCert, otherIPCert, ipCert}

	ipPreferred, others := splitNoSNITLSRuntimeCertificateCandidates(materials, "127.0.0.1")
	if len(ipPreferred) != 1 || ipPreferred[0] != ipCert {
		t.Fatalf("expected local ip matching ip cert first, got=%d", len(ipPreferred))
	}
	if len(others) != 2 {
		t.Fatalf("expected 2 fallback certs, got=%d", len(others))
	}

	ipPreferred, others = splitNoSNITLSRuntimeCertificateCandidates(materials, "192.168.10.10")
	if len(ipPreferred) != 0 {
		t.Fatalf("expected no local-ip ip cert match, got=%d", len(ipPreferred))
	}
	if len(others) != 3 {
		t.Fatalf("expected all certs in fallback set, got=%d", len(others))
	}
}

func TestSplitNoSNITLSRuntimeCertificateCandidatesSkipsExpired(t *testing.T) {
	expiredIPCert := mustTLSRuntimeCertificate(t, nil, []string{"127.0.0.1"}, "fp-expired-ip")
	expiredIPCert.notAfter = time.Now().Add(-time.Minute)
	validIPCert := mustTLSRuntimeCertificate(t, nil, []string{"127.0.0.1"}, "fp-valid-ip")

	ipPreferred, others := splitNoSNITLSRuntimeCertificateCandidates(
		[]*tlsRuntimeCertificate{expiredIPCert, validIPCert},
		"127.0.0.1",
	)
	if len(ipPreferred) != 1 || ipPreferred[0] != validIPCert {
		t.Fatalf("expected expired ip cert to be skipped, got=%#v", ipPreferred)
	}
	if len(others) != 0 {
		t.Fatalf("expected no fallback certs, got=%d", len(others))
	}
}

func TestSelectBalancedTLSRuntimeCertificateFallsBackWithoutSelectionOnDBError(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "web-balance-fallback.db")
	if err := database.OpenDB(dbPath); err != nil {
		t.Fatalf("open fallback db failed: %v", err)
	}
	if sqlDB, err := database.GetDB().DB(); err == nil && sqlDB != nil {
		t.Cleanup(func() {
			_ = sqlDB.Close()
		})
	}

	server := NewServer()
	first := mustTLSRuntimeCertificate(t, []string{"example.com"}, nil, "fp-first")
	second := mustTLSRuntimeCertificate(t, []string{"example.com"}, nil, "fp-second")
	first.certRecordID = 11
	second.certRecordID = 22

	selected, selection := server.selectBalancedTLSRuntimeCertificate(
		[]*tlsRuntimeCertificate{first, second},
		"listener|panel|9443",
		"example.com",
	)
	if selected != first {
		t.Fatalf("expected fallback to first candidate on db error, got %#v", selected)
	}
	if selection.CertificateRecordID != 0 || selection.ListenerKey != "" || selection.SNIBucket != "" {
		t.Fatalf("expected empty selection on fallback, got %#v", selection)
	}
}

func mustTLSRuntimeCertificate(t *testing.T, dnsNames []string, ipNames []string, fingerprint string) *tlsRuntimeCertificate {
	t.Helper()

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key failed: %v", err)
	}
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject: pkix.Name{
			CommonName: "test.local",
		},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		DNSNames:              dnsNames,
	}
	for _, rawIP := range ipNames {
		parsed := netParseIP(rawIP)
		if parsed != nil {
			template.IPAddresses = append(template.IPAddresses, parsed)
		}
	}

	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		t.Fatalf("create certificate failed: %v", err)
	}
	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse certificate failed: %v", err)
	}
	return &tlsRuntimeCertificate{
		leaf:        leaf,
		fingerprint: fingerprint,
		notAfter:    leaf.NotAfter,
	}
}

func netParseIP(raw string) net.IP {
	return net.ParseIP(raw)
}
