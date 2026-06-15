package service

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"gorm.io/gorm"
)

func TestBuildAcmeSecretEnvKeySetIncludesSavedKeys(t *testing.T) {
	keys := buildAcmeSecretEnvKeySet([]string{
		"Ali_Key=abc",
		"CF_Token=xyz",
		"invalid line",
	})
	if _, ok := keys["Ali_Key"]; !ok {
		t.Fatalf("expected Ali_Key in key set")
	}
	if _, ok := keys["SAVED_Ali_Key"]; !ok {
		t.Fatalf("expected SAVED_Ali_Key in key set")
	}
	if _, ok := keys["CF_Token"]; !ok {
		t.Fatalf("expected CF_Token in key set")
	}
	if _, ok := keys["SAVED_CF_Token"]; !ok {
		t.Fatalf("expected SAVED_CF_Token in key set")
	}
	if _, ok := keys["invalid line"]; ok {
		t.Fatalf("unexpected invalid key in key set")
	}
}

func TestMergeAcmeDNSAccountEnvKeepsStoredValueWhenMasked(t *testing.T) {
	existing := map[string]string{
		"Ali_Key":    "old-key",
		"Ali_Secret": "old-secret",
	}
	incoming := map[string]string{
		"Ali_Key":    acmeMaskedEnvValue,
		"Ali_Secret": "new-secret",
	}
	merged := mergeAcmeDNSAccountEnv(existing, incoming)
	if got := merged["Ali_Key"]; got != "old-key" {
		t.Fatalf("expected masked field to keep old value, got=%q", got)
	}
	if got := merged["Ali_Secret"]; got != "new-secret" {
		t.Fatalf("expected non-masked field to be updated, got=%q", got)
	}
}

func TestSanitizeAcmeEnvMap(t *testing.T) {
	sanitized := sanitizeAcmeEnvMap(map[string]string{
		"CF_Account_ID": "account-id",
		"Ali_Key":       "abc",
		"Ali_Secret":    "def",
	})
	if got := sanitized["CF_Account_ID"]; got != "account-id" {
		t.Fatalf("expected CF_Account_ID preserved, got=%q", got)
	}
	if got := sanitized["Ali_Key"]; got != acmeMaskedEnvValue {
		t.Fatalf("expected Ali_Key masked, got=%q", got)
	}
	if got := sanitized["Ali_Secret"]; got != acmeMaskedEnvValue {
		t.Fatalf("expected Ali_Secret masked, got=%q", got)
	}
}

func TestStripAcmeAccountConfSecretsRemovesKeysAndSavedKeys(t *testing.T) {
	homeDir := t.TempDir()
	confPath := filepath.Join(homeDir, "account.conf")
	content := strings.Join([]string{
		"USER_PATH=/usr/bin:/bin",
		"Ali_Key='keep-me-out'",
		"SAVED_Ali_Key='saved-value'",
		"CF_Token=\"token-value\"",
		"SAVED_CF_Token='saved-token'",
		"LE_WORKING_DIR='/tmp/acme'",
		"",
	}, "\n")
	if err := os.WriteFile(confPath, []byte(content), 0o600); err != nil {
		t.Fatalf("write account.conf failed: %v", err)
	}

	removed, err := stripAcmeAccountConfSecrets(homeDir, []string{
		"Ali_Key=new-key",
		"CF_Token=new-token",
	})
	if err != nil {
		t.Fatalf("strip account.conf secrets failed: %v", err)
	}
	if removed != 4 {
		t.Fatalf("unexpected removed count: got=%d want=4", removed)
	}

	afterRaw, err := os.ReadFile(confPath)
	if err != nil {
		t.Fatalf("read account.conf after strip failed: %v", err)
	}
	after := string(afterRaw)
	if strings.Contains(after, "Ali_Key=") {
		t.Fatalf("Ali_Key still exists after cleanup: %s", after)
	}
	if strings.Contains(after, "CF_Token=") {
		t.Fatalf("CF_Token still exists after cleanup: %s", after)
	}
	if !strings.Contains(after, "USER_PATH=/usr/bin:/bin") {
		t.Fatalf("non-secret line should be kept: %s", after)
	}
	if !strings.Contains(after, "LE_WORKING_DIR='/tmp/acme'") {
		t.Fatalf("non-secret line should be kept: %s", after)
	}
}

func TestParseAcmeEnvLineKey(t *testing.T) {
	if got := parseAcmeEnvLineKey("Ali_Key=abc"); got != "Ali_Key" {
		t.Fatalf("unexpected key parse result: %q", got)
	}
	if got := parseAcmeEnvLineKey("export CF_Token='abc'"); got != "CF_Token" {
		t.Fatalf("unexpected export key parse result: %q", got)
	}
	if got := parseAcmeEnvLineKey("  # comment "); got != "" {
		t.Fatalf("expected empty key for comment, got=%q", got)
	}
	if got := parseAcmeEnvLineKey("1INVALID=abc"); got != "" {
		t.Fatalf("expected invalid key to be ignored, got=%q", got)
	}
}

func TestSanitizeDNSAccountEnvForProviderDropsOtherProviderFields(t *testing.T) {
	provider, ok := lookupAcmeDNSProvider("dns_cf")
	if !ok {
		t.Fatal("dns_cf provider not found")
	}
	sanitized := sanitizeDNSAccountEnvForProvider(provider, map[string]string{
		"CF_Token":      "token",
		"CF_Account_ID": "account-id",
		"Ali_Key":       "should-drop",
		"Ali_Secret":    "should-drop",
		"CUSTOM_ENV":    "keep",
	})
	if got := sanitized["CF_Token"]; got != "token" {
		t.Fatalf("expected CF_Token kept, got=%q", got)
	}
	if got := sanitized["CF_Account_ID"]; got != "account-id" {
		t.Fatalf("expected CF_Account_ID kept, got=%q", got)
	}
	if _, ok := sanitized["Ali_Key"]; ok {
		t.Fatalf("expected Ali_Key dropped: %#v", sanitized)
	}
	if _, ok := sanitized["Ali_Secret"]; ok {
		t.Fatalf("expected Ali_Secret dropped: %#v", sanitized)
	}
	if got := sanitized["CUSTOM_ENV"]; got != "keep" {
		t.Fatalf("expected CUSTOM_ENV preserved, got=%q", got)
	}
}

func TestSaveDNSAccountProviderChangeReplacesOldSecrets(t *testing.T) {
	db := setupAcmeDNSTestDB(t, "acme-dns-provider-change.db")

	svc := &AcmeService{}
	first, err := svc.SaveDNSAccount(AcmeDNSAccountPayload{
		Name:         "dns-account",
		ProviderCode: "dns_ali",
		EnvJSON:      `{"Ali_Key":"old-key","Ali_Secret":"old-secret"}`,
	})
	if err != nil {
		t.Fatalf("save ali dns account failed: %v", err)
	}
	if first == nil || first.Overview == nil {
		t.Fatalf("unexpected first save result: %#v", first)
	}

	row := &model.AcmeDNSAccount{}
	if err := db.Where("name = ?", "dns-account").First(row).Error; err != nil {
		t.Fatalf("load saved dns account failed: %v", err)
	}

	if _, err := svc.SaveDNSAccount(AcmeDNSAccountPayload{
		ID:           row.Id,
		Name:         "dns-account",
		ProviderCode: "dns_cf",
		EnvJSON:      `{"CF_Token":"cf-token","CF_Account_ID":"cf-account","Ali_Key":"stale","Ali_Secret":"stale"}`,
	}); err != nil {
		t.Fatalf("save dns account after provider change failed: %v", err)
	}

	if err := db.Where("id = ?", row.Id).First(row).Error; err != nil {
		t.Fatalf("reload dns account failed: %v", err)
	}
	envMap, err := parseAcmeEnvJSON(row.EnvJSON)
	if err != nil {
		t.Fatalf("parse env json failed: %v", err)
	}
	if got := envMap["CF_Token"]; got != "cf-token" {
		t.Fatalf("expected CF_Token updated, got=%q", got)
	}
	if _, ok := envMap["Ali_Key"]; ok {
		t.Fatalf("expected stale Ali_Key removed: %#v", envMap)
	}
	if _, ok := envMap["Ali_Secret"]; ok {
		t.Fatalf("expected stale Ali_Secret removed: %#v", envMap)
	}
}

func TestDeleteDNSAccountClearsCertificateReferences(t *testing.T) {
	db := setupAcmeDNSTestDB(t, "acme-dns-delete-reference.db")

	dnsRow := &model.AcmeDNSAccount{
		Name:         "dns-account",
		ProviderName: "Cloudflare",
		ProviderCode: "dns_cf",
		EnvJSON:      `{"CF_Token":"token","CF_Account_ID":"account"}`,
	}
	if err := db.Create(dnsRow).Error; err != nil {
		t.Fatalf("create dns account failed: %v", err)
	}

	acmeCert := &model.AcmeCertificate{
		MainDomain:      "example.com",
		DomainSet:       `["example.com"]`,
		Challenge:       "dns",
		KeyLength:       "ec-256",
		CAServer:        "letsencrypt",
		UseECC:          true,
		DNSAccountID:    dnsRow.Id,
		DNSAccountName:  dnsRow.Name,
		CertPEM:         []byte("cert"),
		KeyPEM:          []byte("key"),
		FullchainPEM:    []byte("fullchain"),
		Fingerprint:     "fp",
		LastIssuedAt:    1,
		LastRenewedAt:   1,
		CertificateType: "domain",
	}
	if err := db.Create(acmeCert).Error; err != nil {
		t.Fatalf("create acme certificate failed: %v", err)
	}

	inventory := &model.CertificateRecord{
		SourceType:      CertificateSourceACME,
		SourceRef:       "1",
		MainDomain:      "example.com",
		DomainSet:       `["example.com"]`,
		Challenge:       "dns",
		KeyLength:       "ec-256",
		CAServer:        "letsencrypt",
		UseECC:          true,
		DNSAccountID:    dnsRow.Id,
		DNSAccountName:  dnsRow.Name,
		CertPEM:         []byte("cert"),
		KeyPEM:          []byte("key"),
		FullchainPEM:    []byte("fullchain"),
		Fingerprint:     "fp",
		LastIssuedAt:    1,
		LastRenewedAt:   1,
		CertificateType: "domain",
	}
	if err := db.Create(inventory).Error; err != nil {
		t.Fatalf("create inventory record failed: %v", err)
	}

	if _, err := (&AcmeService{}).DeleteDNSAccount(dnsRow.Id); err != nil {
		t.Fatalf("delete dns account failed: %v", err)
	}

	if err := db.Where("id = ?", acmeCert.Id).First(acmeCert).Error; err != nil {
		t.Fatalf("reload acme certificate failed: %v", err)
	}
	if acmeCert.DNSAccountID != 0 || acmeCert.DNSAccountName != "" {
		t.Fatalf("expected acme certificate dns reference cleared: %#v", acmeCert)
	}

	if err := db.Where("id = ?", inventory.Id).First(inventory).Error; err != nil {
		t.Fatalf("reload inventory record failed: %v", err)
	}
	if inventory.DNSAccountID != 0 || inventory.DNSAccountName != "" {
		t.Fatalf("expected inventory dns reference cleared: %#v", inventory)
	}
}

func TestPersistLegacyDNSCandidatesCreatesDatabaseRowsAndCleansAccountConf(t *testing.T) {
	db := setupAcmeDNSTestDB(t, "acme-dns-legacy-migrate.db")
	_ = db

	homeDir := t.TempDir()
	confPath := filepath.Join(homeDir, "account.conf")
	content := strings.Join([]string{
		"USER_PATH=/usr/bin:/bin",
		"SAVED_CF_Token='cf-token'",
		"CF_Account_ID='cf-account'",
		"Ali_Key='ali-key'",
		"Ali_Secret='ali-secret'",
		"",
	}, "\n")
	if err := os.WriteFile(confPath, []byte(content), 0o600); err != nil {
		t.Fatalf("write account.conf failed: %v", err)
	}

	candidates, err := loadLegacyDNSCandidatesFromAccountConf(homeDir)
	if err != nil {
		t.Fatalf("load legacy dns candidates failed: %v", err)
	}
	if len(candidates) != 2 {
		t.Fatalf("expected two candidates, got=%d", len(candidates))
	}

	if err := (&AcmeService{}).persistLegacyDNSCandidates(homeDir, candidates); err != nil {
		t.Fatalf("persist legacy dns candidates failed: %v", err)
	}

	rows := make([]model.AcmeDNSAccount, 0)
	if err := db.Order("provider_code ASC").Find(&rows).Error; err != nil {
		t.Fatalf("query migrated dns accounts failed: %v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("expected two migrated dns accounts, got=%d", len(rows))
	}

	envByProvider := map[string]map[string]string{}
	for _, row := range rows {
		envMap, err := parseAcmeEnvJSON(row.EnvJSON)
		if err != nil {
			t.Fatalf("parse migrated env json failed: %v", err)
		}
		envByProvider[row.ProviderCode] = envMap
	}
	if got := envByProvider["dns_cf"]["CF_Token"]; got != "cf-token" {
		t.Fatalf("expected migrated CF_Token, got=%q", got)
	}
	if got := envByProvider["dns_ali"]["Ali_Key"]; got != "ali-key" {
		t.Fatalf("expected migrated Ali_Key, got=%q", got)
	}

	afterRaw, err := os.ReadFile(confPath)
	if err != nil {
		t.Fatalf("read account.conf after migration failed: %v", err)
	}
	after := string(afterRaw)
	if strings.Contains(after, "CF_Token=") || strings.Contains(after, "Ali_Key=") || strings.Contains(after, "Ali_Secret=") {
		t.Fatalf("expected legacy dns secrets removed from account.conf: %s", after)
	}
	if !strings.Contains(after, "USER_PATH=/usr/bin:/bin") {
		t.Fatalf("expected unrelated line preserved: %s", after)
	}
}

func TestValidateDNSProviderEnvCloudflareCompatibilityModes(t *testing.T) {
	provider, ok := lookupAcmeDNSProvider("dns_cf")
	if !ok {
		t.Fatal("dns_cf provider not found")
	}

	if err := validateDNSProviderEnv(provider, map[string]string{
		"CF_Token":      "token",
		"CF_Account_ID": "account",
	}); err != nil {
		t.Fatalf("expected Cloudflare token mode valid, got err=%v", err)
	}

	if err := validateDNSProviderEnv(provider, map[string]string{
		"CF_Email": "user@example.com",
		"CF_Key":   "global-key",
	}); err != nil {
		t.Fatalf("expected Cloudflare global key mode valid, got err=%v", err)
	}

	if err := validateDNSProviderEnv(provider, map[string]string{
		"CF_Token": "token-only",
	}); err == nil {
		t.Fatal("expected token-only Cloudflare config to be rejected")
	}
}

func TestValidateDNSProviderEnvAWSRoleAndStaticModes(t *testing.T) {
	provider, ok := lookupAcmeDNSProvider("dns_aws")
	if !ok {
		t.Fatal("dns_aws provider not found")
	}

	if err := validateDNSProviderEnv(provider, map[string]string{}); err != nil {
		t.Fatalf("expected empty aws env valid for role mode, got err=%v", err)
	}

	if err := validateDNSProviderEnv(provider, map[string]string{
		"AWS_ACCESS_KEY_ID":     "ak",
		"AWS_SECRET_ACCESS_KEY": "sk",
	}); err != nil {
		t.Fatalf("expected static aws key pair valid, got err=%v", err)
	}

	if err := validateDNSProviderEnv(provider, map[string]string{
		"AWS_ACCESS_KEY_ID": "ak-only",
	}); err == nil {
		t.Fatal("expected incomplete aws static credentials to be rejected")
	}
}

func TestShouldUseAcmeDNSChallenge(t *testing.T) {
	if !shouldUseAcmeDNSChallenge(acmeCertificateTypeDomain, "dns") {
		t.Fatal("expected domain + dns challenge to use dns flow")
	}
	if shouldUseAcmeDNSChallenge(acmeCertificateTypeDomain, "standalone") {
		t.Fatal("did not expect standalone challenge to use dns flow")
	}
	if shouldUseAcmeDNSChallenge(acmeCertificateTypeIP, "dns") {
		t.Fatal("did not expect ip certificate to use dns flow")
	}
}

func TestCleanupNonDNSCertificateDNSReferences(t *testing.T) {
	db := setupAcmeDNSTestDB(t, "acme-nondns-cleanup.db")
	svc := &AcmeService{}

	nonDNSAcme := &model.AcmeCertificate{
		MainDomain:      "standalone.example.com",
		DomainSet:       `["standalone.example.com"]`,
		CertificateType: acmeCertificateTypeDomain,
		Challenge:       "standalone",
		KeyLength:       "ec-256",
		CAServer:        "letsencrypt",
		UseECC:          true,
		DNSAccountID:    11,
		DNSAccountName:  "dns-standalone",
		CertPEM:         []byte("cert-standalone"),
		KeyPEM:          []byte("key-standalone"),
		FullchainPEM:    []byte("fullchain-standalone"),
		Fingerprint:     "fp-standalone",
		LastIssuedAt:    1,
		LastRenewedAt:   1,
	}
	dnsAcme := &model.AcmeCertificate{
		MainDomain:      "dns.example.com",
		DomainSet:       `["dns.example.com"]`,
		CertificateType: acmeCertificateTypeDomain,
		Challenge:       "dns",
		KeyLength:       "ec-256",
		CAServer:        "letsencrypt",
		UseECC:          true,
		DNSAccountID:    22,
		DNSAccountName:  "dns-valid",
		CertPEM:         []byte("cert-dns"),
		KeyPEM:          []byte("key-dns"),
		FullchainPEM:    []byte("fullchain-dns"),
		Fingerprint:     "fp-dns",
		LastIssuedAt:    1,
		LastRenewedAt:   1,
	}
	if err := db.Create(nonDNSAcme).Error; err != nil {
		t.Fatalf("create non-dns acme row failed: %v", err)
	}
	if err := db.Create(dnsAcme).Error; err != nil {
		t.Fatalf("create dns acme row failed: %v", err)
	}

	nonDNSInventory := &model.CertificateRecord{
		SourceType:      CertificateSourceACME,
		SourceRef:       strconv.FormatUint(uint64(nonDNSAcme.Id), 10),
		MainDomain:      nonDNSAcme.MainDomain,
		DomainSet:       nonDNSAcme.DomainSet,
		CertificateType: nonDNSAcme.CertificateType,
		Challenge:       nonDNSAcme.Challenge,
		KeyLength:       nonDNSAcme.KeyLength,
		CAServer:        nonDNSAcme.CAServer,
		UseECC:          true,
		DNSAccountID:    11,
		DNSAccountName:  "dns-standalone",
		CertPEM:         []byte("inv-cert-standalone"),
		KeyPEM:          []byte("inv-key-standalone"),
		FullchainPEM:    []byte("inv-fullchain-standalone"),
		Fingerprint:     "inv-fp-standalone",
		LastIssuedAt:    1,
		LastRenewedAt:   1,
	}
	dnsInventory := &model.CertificateRecord{
		SourceType:      CertificateSourceACME,
		SourceRef:       strconv.FormatUint(uint64(dnsAcme.Id), 10),
		MainDomain:      dnsAcme.MainDomain,
		DomainSet:       dnsAcme.DomainSet,
		CertificateType: dnsAcme.CertificateType,
		Challenge:       dnsAcme.Challenge,
		KeyLength:       dnsAcme.KeyLength,
		CAServer:        dnsAcme.CAServer,
		UseECC:          true,
		DNSAccountID:    22,
		DNSAccountName:  "dns-valid",
		CertPEM:         []byte("inv-cert-dns"),
		KeyPEM:          []byte("inv-key-dns"),
		FullchainPEM:    []byte("inv-fullchain-dns"),
		Fingerprint:     "inv-fp-dns",
		LastIssuedAt:    1,
		LastRenewedAt:   1,
	}
	if err := db.Create(nonDNSInventory).Error; err != nil {
		t.Fatalf("create non-dns inventory row failed: %v", err)
	}
	if err := db.Create(dnsInventory).Error; err != nil {
		t.Fatalf("create dns inventory row failed: %v", err)
	}

	if err := svc.cleanupNonDNSCertificateDNSReferences(); err != nil {
		t.Fatalf("cleanup non-dns dns references failed: %v", err)
	}

	if err := db.Where("id = ?", nonDNSAcme.Id).First(nonDNSAcme).Error; err != nil {
		t.Fatalf("reload non-dns acme row failed: %v", err)
	}
	if nonDNSAcme.DNSAccountID != 0 || nonDNSAcme.DNSAccountName != "" {
		t.Fatalf("expected non-dns acme row dns refs cleared: %#v", nonDNSAcme)
	}
	if err := db.Where("id = ?", dnsAcme.Id).First(dnsAcme).Error; err != nil {
		t.Fatalf("reload dns acme row failed: %v", err)
	}
	if dnsAcme.DNSAccountID != 22 || dnsAcme.DNSAccountName != "dns-valid" {
		t.Fatalf("expected dns acme row refs kept: %#v", dnsAcme)
	}

	if err := db.Where("id = ?", nonDNSInventory.Id).First(nonDNSInventory).Error; err != nil {
		t.Fatalf("reload non-dns inventory row failed: %v", err)
	}
	if nonDNSInventory.DNSAccountID != 0 || nonDNSInventory.DNSAccountName != "" {
		t.Fatalf("expected non-dns inventory row dns refs cleared: %#v", nonDNSInventory)
	}
	if err := db.Where("id = ?", dnsInventory.Id).First(dnsInventory).Error; err != nil {
		t.Fatalf("reload dns inventory row failed: %v", err)
	}
	if dnsInventory.DNSAccountID != 22 || dnsInventory.DNSAccountName != "dns-valid" {
		t.Fatalf("expected dns inventory row refs kept: %#v", dnsInventory)
	}
}

func TestGetOverviewReturnsAccountsAndCertificatesWhenUnsupportedOS(t *testing.T) {
	db := setupAcmeDNSTestDB(t, "acme-overview-unsupported-os.db")
	_ = db
	svc := &AcmeService{}

	if err := database.GetDB().Create(&model.AcmeAccount{
		Name:      "acc-1",
		Email:     "acc1@example.com",
		Server:    "letsencrypt",
		KeyLength: "ec-256",
		Remark:    "test",
	}).Error; err != nil {
		t.Fatalf("create acme account failed: %v", err)
	}
	if err := database.GetDB().Create(&model.AcmeDNSAccount{
		Name:         "dns-1",
		ProviderName: "Cloudflare",
		ProviderCode: "dns_cf",
		EnvJSON:      `{"CF_Token":"token","CF_Account_ID":"acc"}`,
		Remark:       "test",
	}).Error; err != nil {
		t.Fatalf("create dns account failed: %v", err)
	}
	if err := database.GetDB().Create(&model.AcmeCertificate{
		MainDomain:      "overview.example.com",
		DomainSet:       `["overview.example.com"]`,
		CertificateType: acmeCertificateTypeDomain,
		Challenge:       "dns",
		KeyLength:       "ec-256",
		CAServer:        "letsencrypt",
		UseECC:          true,
		CertPEM:         []byte("cert-overview"),
		KeyPEM:          []byte("key-overview"),
		FullchainPEM:    []byte("fullchain-overview"),
		Fingerprint:     "fp-overview",
		LastIssuedAt:    1,
		LastRenewedAt:   1,
	}).Error; err != nil {
		t.Fatalf("create acme certificate failed: %v", err)
	}

	overview, err := svc.GetOverview()
	if err != nil {
		t.Fatalf("GetOverview failed: %v", err)
	}
	if len(overview.AcmeAccounts) == 0 {
		t.Fatal("expected acme accounts in overview")
	}
	if len(overview.DNSAccounts) == 0 {
		t.Fatal("expected dns accounts in overview")
	}
	if len(overview.Certificates) == 0 {
		t.Fatal("expected certificates in overview")
	}
	if runtime.GOOS != "linux" {
		if overview.Supported {
			t.Fatal("expected unsupported flag on non-linux")
		}
		if strings.TrimSpace(overview.Error) == "" {
			t.Fatal("expected non-linux overview error message")
		}
	}
}

func setupAcmeDNSTestDB(t *testing.T, dbName string) *gorm.DB {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), dbName)
	if err := database.InitDB(dbPath); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}

	db := database.GetDB()
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db.DB failed: %v", err)
	}
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})

	return db
}
