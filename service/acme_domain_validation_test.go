package service

import (
	"fmt"
	"strings"
	"testing"
)

func TestValidateAcmeIssueIdentifiersNormalizesIDNSeparatorsAndDuplicates(t *testing.T) {
	domains, err := validateAcmeIssueIdentifiers("  example.com, 例子.公司\nwww.example.com\tEXAMPLE.COM ", acmeCertificateTypeDomain)
	if err != nil {
		t.Fatalf("validate domains failed: %v", err)
	}
	want := []string{"example.com", "xn--fsqu00a.xn--55qx5d", "www.example.com"}
	if len(domains) != len(want) {
		t.Fatalf("unexpected domains: %#v", domains)
	}
	for index := range want {
		if domains[index] != want[index] {
			t.Fatalf("domain %d: got %q want %q", index, domains[index], want[index])
		}
	}
}

func TestValidateAcmeIssueIdentifiersRejectsInvalidPublicNames(t *testing.T) {
	for _, input := range []string{"127.0.0.1", "localhost", "bad_domain.example", "*.*.example.com"} {
		if _, err := validateAcmeIssueIdentifiers(input, acmeCertificateTypeDomain); err == nil {
			t.Fatalf("expected invalid ACME domain to be rejected: %q", input)
		}
	}
}

func TestValidateAcmeIssueIdentifiersEnforces2048DomainLimit(t *testing.T) {
	identifiers := make([]string, 0, acmeDomainCertificateMaxNames+1)
	for index := 0; index < acmeDomainCertificateMaxNames+1; index++ {
		identifiers = append(identifiers, fmt.Sprintf("host-%04d.example.com", index))
	}

	accepted, err := validateAcmeIssueIdentifiers(strings.Join(identifiers[:acmeDomainCertificateMaxNames], " "), acmeCertificateTypeDomain)
	if err != nil {
		t.Fatalf("expected 2048 domains to be accepted: %v", err)
	}
	if len(accepted) != acmeDomainCertificateMaxNames {
		t.Fatalf("accepted domain count = %d, want %d", len(accepted), acmeDomainCertificateMaxNames)
	}

	_, err = validateAcmeIssueIdentifiers(strings.Join(identifiers, " "), acmeCertificateTypeDomain)
	if err == nil || !strings.Contains(err.Error(), "2048") {
		t.Fatalf("expected 2049 domains to be rejected with the 2048 limit, got %v", err)
	}
}

func TestValidateAcmeIssueIdentifiersEnforces2048IPLimit(t *testing.T) {
	identifiers := make([]string, 0, acmeIPCertificateMaxIPs+1)
	for index := 0; index < acmeIPCertificateMaxIPs+1; index++ {
		identifiers = append(identifiers, fmt.Sprintf("2001:db8::%x", index+1))
	}

	accepted, err := validateAcmeIssueIdentifiers(strings.Join(identifiers[:acmeIPCertificateMaxIPs], " "), acmeCertificateTypeIP)
	if err != nil {
		t.Fatalf("expected 2048 IP identifiers to be accepted: %v", err)
	}
	if len(accepted) != acmeIPCertificateMaxIPs {
		t.Fatalf("accepted IP count = %d, want %d", len(accepted), acmeIPCertificateMaxIPs)
	}

	_, err = validateAcmeIssueIdentifiers(strings.Join(identifiers, " "), acmeCertificateTypeIP)
	if err == nil || !strings.Contains(err.Error(), "2048") {
		t.Fatalf("expected 2049 IP identifiers to be rejected with the 2048 limit, got %v", err)
	}
}

func TestAcmeWildcardRequiresDNSChallenge(t *testing.T) {
	if !hasAcmeWildcardDomain([]string{"*.example.com", "api.example.com"}) {
		t.Fatal("expected wildcard detection")
	}
	if hasAcmeWildcardDomain([]string{"api.example.com"}) {
		t.Fatal("did not expect wildcard detection")
	}
}

func TestSelfSignedIdentifierCompatibilityIsRetained(t *testing.T) {
	identifiers := normalizeAcmeDomains("localhost, 127.0.0.1 internal-host")
	if len(identifiers) != 3 {
		t.Fatalf("expected self-signed compatible identifiers, got %#v", identifiers)
	}
}
