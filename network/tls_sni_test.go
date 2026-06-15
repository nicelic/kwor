package network

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"testing"
	"time"
)

func buildTestLeafCertificate(t *testing.T, dnsNames []string) (*x509.Certificate, []byte) {
	t.Helper()

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
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

	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}

	leaf, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatalf("parse certificate: %v", err)
	}

	return leaf, der
}

func TestParseLeafCertificate(t *testing.T) {
	wantLeaf, der := buildTestLeafCertificate(t, []string{"example.com"})
	cert := &tls.Certificate{
		Certificate: [][]byte{der},
	}

	gotLeaf, err := ParseLeafCertificate(cert)
	if err != nil {
		t.Fatalf("ParseLeafCertificate returned error: %v", err)
	}
	if gotLeaf == nil {
		t.Fatal("ParseLeafCertificate returned nil leaf")
	}
	if cert.Leaf == nil {
		t.Fatal("ParseLeafCertificate should set cert.Leaf")
	}
	if gotLeaf.Subject.CommonName != wantLeaf.Subject.CommonName {
		t.Fatalf("leaf subject mismatch: got %q want %q", gotLeaf.Subject.CommonName, wantLeaf.Subject.CommonName)
	}
}

func TestStrictSNIVerifier(t *testing.T) {
	leaf, _ := buildTestLeafCertificate(t, []string{"example.com", "*.example.org"})
	verify := StrictSNIVerifier(leaf)

	tests := []struct {
		name    string
		sni     string
		wantErr bool
	}{
		{name: "empty sni", sni: "", wantErr: false},
		{name: "exact match", sni: "example.com", wantErr: false},
		{name: "wildcard match", sni: "api.example.org", wantErr: false},
		{name: "trailing dot match", sni: "example.com.", wantErr: false},
		{name: "wildcard root mismatch", sni: "example.org", wantErr: true},
		{name: "unrelated domain mismatch", sni: "example.net", wantErr: true},
		{name: "ip sni ignored", sni: "192.0.2.1", wantErr: false},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			err := verify(tls.ConnectionState{
				ServerName: tc.sni,
			})
			if tc.wantErr && err == nil {
				t.Fatalf("expected error for sni %q", tc.sni)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error for sni %q: %v", tc.sni, err)
			}
		})
	}
}
