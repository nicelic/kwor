package service

import (
	"os"
	"path/filepath"
	"runtime"
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

func TestAcmeDNSProviderCatalogMatchesOfficialDefinitions(t *testing.T) {
	expectedFields := map[string][]string{
		"dns_ali":         {"Ali_Key", "Ali_Secret"},
		"dns_tencent":     {"Tencent_SecretId", "Tencent_SecretKey"},
		"dns_cf":          {"CF_Token", "CF_Account_ID", "CF_Zone_ID", "CF_Email", "CF_Key"},
		"dns_aws":         {"AWS_ACCESS_KEY_ID", "AWS_SECRET_ACCESS_KEY", "AWS_DNS_SLOWRATE"},
		"dns_huaweicloud": {"HUAWEICLOUD_Username", "HUAWEICLOUD_Password", "HUAWEICLOUD_DomainName", "HUAWEICLOUD_Region"},
		"dns_gd":          {"GD_Key", "GD_Secret"},
		"dns_vercel":      {"VERCEL_TOKEN"},
	}
	expectedRequired := map[string]map[string]bool{
		"dns_ali": {
			"Ali_Key":    true,
			"Ali_Secret": true,
		},
		"dns_tencent": {
			"Tencent_SecretId":  true,
			"Tencent_SecretKey": true,
		},
		"dns_cf": {
			"CF_Token":      false,
			"CF_Account_ID": false,
			"CF_Zone_ID":    false,
			"CF_Email":      false,
			"CF_Key":        false,
		},
		"dns_aws": {
			"AWS_ACCESS_KEY_ID":     false,
			"AWS_SECRET_ACCESS_KEY": false,
			"AWS_DNS_SLOWRATE":      false,
		},
		"dns_huaweicloud": {
			"HUAWEICLOUD_Username":   true,
			"HUAWEICLOUD_Password":   true,
			"HUAWEICLOUD_DomainName": true,
			"HUAWEICLOUD_Region":     false,
		},
		"dns_gd": {
			"GD_Key":    true,
			"GD_Secret": true,
		},
		"dns_vercel": {
			"VERCEL_TOKEN": true,
		},
	}
	if len(defaultAcmeDNSProviderCatalog) != len(expectedFields) {
		t.Fatalf("unexpected DNS provider catalog size: got=%d want=%d", len(defaultAcmeDNSProviderCatalog), len(expectedFields))
	}
	for providerCode, wantFields := range expectedFields {
		provider, ok := lookupAcmeDNSProvider(providerCode)
		if !ok {
			t.Fatalf("provider %q missing from catalog", providerCode)
		}
		if len(provider.Fields) != len(wantFields) {
			t.Fatalf("provider %q fields: got=%d want=%d", providerCode, len(provider.Fields), len(wantFields))
		}
		for index, wantKey := range wantFields {
			field := provider.Fields[index]
			if gotKey := field.Key; gotKey != wantKey {
				t.Fatalf("provider %q field %d: got=%q want=%q", providerCode, index, gotKey, wantKey)
			}
			if wantRequired := expectedRequired[providerCode][wantKey]; field.Required != wantRequired {
				t.Fatalf("provider %q field %q required: got=%t want=%t", providerCode, wantKey, field.Required, wantRequired)
			}
		}
	}

	huawei, _ := lookupAcmeDNSProvider("dns_huaweicloud")
	region := huawei.Fields[len(huawei.Fields)-1]
	if region.Required || region.Placeholder != "cn-north-4" {
		t.Fatalf("unexpected HuaweiCloud Region definition: %#v", region)
	}
}

func TestBuildLegacyDNSCandidatesIncludesHuaweiRegion(t *testing.T) {
	candidates := buildLegacyDNSCandidatesFromEnvMap(map[string]string{
		"HUAWEICLOUD_Username":   "user",
		"HUAWEICLOUD_Password":   "password",
		"HUAWEICLOUD_DomainName": "account-domain",
		"HUAWEICLOUD_Region":     "cn-north-4",
	})
	for _, candidate := range candidates {
		if candidate.provider.ProviderCode != "dns_huaweicloud" {
			continue
		}
		if got := candidate.env["HUAWEICLOUD_Region"]; got != "cn-north-4" {
			t.Fatalf("expected HuaweiCloud Region migrated, got=%q", got)
		}
		return
	}
	t.Fatal("expected HuaweiCloud legacy DNS candidate")
}

func TestResolveAcmeDNSRuntimeConfigValidatesKnownProvider(t *testing.T) {
	runtimeConfig, err := resolveAcmeDNSRuntimeConfig(0, "dns_cf", "CF_Token=token-only")
	if err != nil {
		t.Fatalf("expected Cloudflare token-only runtime config valid, got err=%v", err)
	}
	if runtimeConfig.ProviderCode != "dns_cf" || runtimeConfig.EnvPairs[0] != "CF_Token=token-only" {
		t.Fatalf("unexpected Cloudflare runtime config: %#v", runtimeConfig)
	}

	if _, err := resolveAcmeDNSRuntimeConfig(0, "dns_cf", ""); err == nil {
		t.Fatal("expected missing Cloudflare credentials rejected before acme.sh execution")
	}

	if _, err := resolveAcmeDNSRuntimeConfig(0, "dns_custom", "CUSTOM_TOKEN=value"); err != nil {
		t.Fatalf("expected unknown manual provider compatibility preserved, got err=%v", err)
	}
}

func TestResolveAcmeDNSRuntimeConfigUsesBoundAccountProvider(t *testing.T) {
	db := setupAcmeDNSTestDB(t, "acme-dns-runtime-account.db")
	dnsAccount := &model.AcmeDNSAccount{
		Name:         "cloudflare-account",
		ProviderName: "Cloudflare",
		ProviderCode: "dns_cf",
		EnvJSON:      `{"CF_Token":"stored-token"}`,
	}
	if err := db.Create(dnsAccount).Error; err != nil {
		t.Fatalf("create DNS account failed: %v", err)
	}

	runtimeConfig, err := resolveAcmeDNSRuntimeConfig(dnsAccount.Id, "dns_ali", "CF_Zone_ID=zone-id")
	if err != nil {
		t.Fatalf("resolve bound DNS account failed: %v", err)
	}
	if runtimeConfig.ProviderCode != "dns_cf" || runtimeConfig.AccountName != dnsAccount.Name {
		t.Fatalf("expected bound account provider to win, got=%#v", runtimeConfig)
	}
	env := envPairsToEnvMap(runtimeConfig.EnvPairs)
	if env["CF_Token"] != "stored-token" || env["CF_Zone_ID"] != "zone-id" {
		t.Fatalf("expected stored and manual DNS env merged, got=%#v", env)
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

func TestSaveDNSAccountProviderChangeRejectsReferencedAccountAndAllowsCredentialRotation(t *testing.T) {
	db := setupAcmeDNSTestDB(t, "acme-dns-provider-lock.db")
	dnsAccount := &model.AcmeDNSAccount{
		Name:         "ali-account",
		ProviderName: "阿里云",
		ProviderCode: "dns_ali",
		EnvJSON:      `{"Ali_Key":"old-key","Ali_Secret":"old-secret"}`,
	}
	if err := db.Create(dnsAccount).Error; err != nil {
		t.Fatalf("create DNS account failed: %v", err)
	}
	certificate := &model.CertificateRecord{
		SourceType:      CertificateSourceACME,
		SourceRef:       "dns-provider-lock",
		MainDomain:      "locked.example.com",
		DomainSet:       `["locked.example.com"]`,
		CertificateType: acmeCertificateTypeDomain,
		Challenge:       "dns",
		KeyLength:       "ec-256",
		CAServer:        "letsencrypt",
		UseECC:          true,
		DNSAccountID:    dnsAccount.Id,
		DNSAccountName:  dnsAccount.Name,
		AutoRenew:       true,
		CertPEM:         []byte("cert"),
		KeyPEM:          []byte("key"),
		FullchainPEM:    []byte("fullchain"),
		Fingerprint:     "lock-fingerprint",
		LastIssuedAt:    1,
		LastRenewedAt:   1,
	}
	if err := db.Create(certificate).Error; err != nil {
		t.Fatalf("create referenced certificate failed: %v", err)
	}

	svc := &AcmeService{}
	if _, err := svc.SaveDNSAccount(AcmeDNSAccountPayload{
		ID:           dnsAccount.Id,
		Name:         dnsAccount.Name,
		ProviderCode: "dns_cf",
		EnvJSON:      `{"CF_Token":"token"}`,
	}); err == nil {
		t.Fatal("expected provider change for referenced DNS account rejected")
	}
	if err := db.Where("id = ?", dnsAccount.Id).First(dnsAccount).Error; err != nil {
		t.Fatalf("reload locked DNS account failed: %v", err)
	}
	if dnsAccount.ProviderCode != "dns_ali" {
		t.Fatalf("expected provider unchanged after rejection, got=%q", dnsAccount.ProviderCode)
	}

	accounts, err := svc.listDNSAccounts()
	if err != nil {
		t.Fatalf("list DNS accounts failed: %v", err)
	}
	if len(accounts) != 1 || !accounts[0].ProviderLocked {
		t.Fatalf("expected referenced DNS account provider lock, got=%#v", accounts)
	}

	if _, err := svc.SaveDNSAccount(AcmeDNSAccountPayload{
		ID:           dnsAccount.Id,
		Name:         dnsAccount.Name,
		ProviderCode: "dns_ali",
		EnvJSON:      `{"Ali_Key":"rotated-key","Ali_Secret":"rotated-secret"}`,
	}); err != nil {
		t.Fatalf("expected same-provider credential rotation allowed, got err=%v", err)
	}
	if err := db.Where("id = ?", certificate.Id).First(certificate).Error; err != nil {
		t.Fatalf("reload referenced certificate failed: %v", err)
	}
	if certificate.DNSAccountID != dnsAccount.Id || !certificate.AutoRenew {
		t.Fatalf("expected credential rotation to preserve binding and auto renew: %#v", certificate)
	}
}

func TestListDNSAccountsLocksInventoryReference(t *testing.T) {
	db := setupAcmeDNSTestDB(t, "acme-dns-inventory-lock.db")
	dnsAccount := &model.AcmeDNSAccount{
		Name:         "inventory-account",
		ProviderName: "Cloudflare",
		ProviderCode: "dns_cf",
		EnvJSON:      `{"CF_Token":"token"}`,
	}
	if err := db.Create(dnsAccount).Error; err != nil {
		t.Fatalf("create DNS account failed: %v", err)
	}
	if err := db.Create(&model.CertificateRecord{
		SourceType:      CertificateSourceACME,
		SourceRef:       "inventory-only",
		MainDomain:      "inventory.example.com",
		DomainSet:       `["inventory.example.com"]`,
		CertificateType: acmeCertificateTypeDomain,
		Challenge:       "dns",
		KeyLength:       "ec-256",
		CAServer:        "letsencrypt",
		UseECC:          true,
		DNSAccountID:    dnsAccount.Id,
		DNSAccountName:  dnsAccount.Name,
		CertPEM:         []byte("cert"),
		KeyPEM:          []byte("key"),
		FullchainPEM:    []byte("fullchain"),
		Fingerprint:     "inventory-lock-fingerprint",
	}).Error; err != nil {
		t.Fatalf("create inventory record failed: %v", err)
	}

	accounts, err := (&AcmeService{}).listDNSAccounts()
	if err != nil {
		t.Fatalf("list DNS accounts failed: %v", err)
	}
	if len(accounts) != 1 || !accounts[0].ProviderLocked {
		t.Fatalf("expected inventory reference to lock DNS account, got=%#v", accounts)
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

	inventory := &model.CertificateRecord{
		SourceType:      CertificateSourceACME,
		SourceRef:       "dns-delete-reference",
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
		"CF_Token": "token-only",
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
		"CF_Account_ID": "account-only",
	}); err == nil {
		t.Fatal("expected Cloudflare account ID without authentication rejected")
	}
}

func TestDNSRuntimeEnvironmentRejectsReservedKeysAndKeepsSafeExtensions(t *testing.T) {
	provider, ok := lookupAcmeDNSProvider("dns_cf")
	if !ok {
		t.Fatal("dns_cf provider not found")
	}
	if err := validateDNSProviderEnv(provider, map[string]string{
		"CF_Token":           "token",
		"AWS_DEFAULT_REGION": "us-east-1",
	}); err != nil {
		t.Fatalf("safe provider extension should be allowed: %v", err)
	}

	for _, raw := range []string{
		"1INVALID=value",
		"LE_CONFIG_HOME=/tmp/override",
		"PATH=/tmp/override",
		"LD_PRELOAD=/tmp/preload.so",
	} {
		if _, err := normalizeAcmeEnvAssignments(raw); err == nil {
			t.Fatalf("expected runtime env %q rejected", raw)
		}
	}
	if err := validateDNSProviderEnv(provider, map[string]string{
		"CF_Token":       "token",
		"LE_WORKING_DIR": "/tmp/override",
	}); err == nil {
		t.Fatal("reserved runtime variable should be rejected for saved DNS accounts")
	}

	db := setupAcmeDNSTestDB(t, "acme-dns-reserved-runtime.db")
	account := &model.AcmeDNSAccount{
		Name:         "invalid-runtime-env",
		ProviderName: "Cloudflare",
		ProviderCode: "dns_cf",
		EnvJSON:      `{"CF_Token":"token","DYLD_INSERT_LIBRARIES":"/tmp/inject"}`,
	}
	if err := db.Create(account).Error; err != nil {
		t.Fatalf("create DNS account fixture failed: %v", err)
	}
	if _, err := resolveAcmeDNSRuntimeConfig(account.Id, "", ""); err == nil {
		t.Fatal("stored reserved runtime variable should be rejected before execution")
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

	nonDNSInventory := &model.CertificateRecord{
		SourceType:      CertificateSourceACME,
		SourceRef:       "nondns-cleanup-standalone",
		MainDomain:      "standalone.example.com",
		DomainSet:       `["standalone.example.com"]`,
		CertificateType: acmeCertificateTypeDomain,
		Challenge:       "standalone",
		KeyLength:       "ec-256",
		CAServer:        "letsencrypt",
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
		SourceRef:       "nondns-cleanup-dns",
		MainDomain:      "dns.example.com",
		DomainSet:       `["dns.example.com"]`,
		CertificateType: acmeCertificateTypeDomain,
		Challenge:       "dns",
		KeyLength:       "ec-256",
		CAServer:        "letsencrypt",
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
	if err := database.GetDB().Create(&model.CertificateRecord{
		SourceType:      CertificateSourceACME,
		SourceRef:       "overview-record",
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
