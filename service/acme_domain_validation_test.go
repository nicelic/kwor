package service

import "testing"

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
