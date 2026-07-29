package service

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"gorm.io/gorm"
)

func setupCurrentAcmeRuntimeSchemaTestDB(t *testing.T, dbName string) *gorm.DB {
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

func TestCurrentDatabaseDoesNotCreateRetiredAcmeMirror(t *testing.T) {
	db := setupCurrentAcmeRuntimeSchemaTestDB(t, "acme-current-schema.db")
	if db.Migrator().HasTable(&model.AcmeCertificate{}) {
		t.Fatal("new database must not create the retired acme_certificates mirror table")
	}
}

func TestLegacyAcmeMirrorMigratesIntoCertificateRecords(t *testing.T) {
	db := setupCurrentAcmeRuntimeSchemaTestDB(t, "acme-legacy-mirror-migration.db")
	if err := db.AutoMigrate(&model.AcmeCertificate{}); err != nil {
		t.Fatalf("create legacy mirror fixture failed: %v", err)
	}
	legacy := &model.AcmeCertificate{
		MainDomain:      "legacy-mirror.example.com",
		DomainSet:       `["legacy-mirror.example.com"]`,
		CertificateType: acmeCertificateTypeDomain,
		Challenge:       "dns",
		KeyLength:       "ec-256",
		CAServer:        "letsencrypt",
		AutoRenew:       true,
		CertPEM:         []byte("legacy-cert"),
		KeyPEM:          []byte("legacy-key"),
		FullchainPEM:    []byte("legacy-fullchain"),
		ChainPEM:        []byte("legacy-chain"),
	}
	if err := db.Create(legacy).Error; err != nil {
		t.Fatalf("create legacy mirror row failed: %v", err)
	}

	if err := (&AcmeService{}).migrateLegacyAcmeCertificateMirror(); err != nil {
		t.Fatalf("migrate legacy mirror failed: %v", err)
	}
	record := &model.CertificateRecord{}
	if err := db.Where("source_type = ? AND source_ref = ?", CertificateSourceACME, fmt.Sprintf("legacy-%d", legacy.Id)).First(record).Error; err != nil {
		t.Fatalf("load converted certificate record failed: %v", err)
	}
	if record.MainDomain != legacy.MainDomain || string(record.CertPEM) != "legacy-cert" || string(record.KeyPEM) != "legacy-key" {
		t.Fatalf("legacy certificate data was not preserved: %#v", record)
	}
	var remaining int64
	if err := db.Model(&model.AcmeCertificate{}).Count(&remaining).Error; err != nil {
		t.Fatalf("count legacy mirror rows failed: %v", err)
	}
	if remaining != 0 {
		t.Fatalf("legacy mirror rows must be removed after conversion, remaining=%d", remaining)
	}
}

func TestAcmeOperationRuntimeSnapshotsCAStateAndCleansUp(t *testing.T) {
	db := setupAcmeDNSTestDB(t, "acme-runtime-snapshot.db")
	account := &model.AcmeAccount{
		Name:             "runtime-account",
		Server:           "letsencrypt",
		KeyLength:        "ec-256",
		AccountKeyLength: "ec-256",
	}
	if err := db.Create(account).Error; err != nil {
		t.Fatalf("create account failed: %v", err)
	}

	runtime, err := newAcmeOperationRuntime(account)
	if err != nil {
		t.Fatalf("create operation runtime failed: %v", err)
	}
	t.Cleanup(runtime.cleanup)

	caDir := filepath.Join(runtime.configHome, "ca", "letsencrypt")
	if err := os.MkdirAll(caDir, 0o700); err != nil {
		t.Fatalf("create temporary CA directory failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(caDir, "account.key"), []byte("account-private-key"), 0o600); err != nil {
		t.Fatalf("write account key failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(caDir, "account.conf"), []byte("ACCOUNT_EMAIL='ops@example.com'\n"), 0o600); err != nil {
		t.Fatalf("write account config failed: %v", err)
	}
	// A root account.conf is intentionally outside the saved ca/ tree because
	// acme.sh DNS scripts may cache credentials in it.
	if err := os.WriteFile(filepath.Join(runtime.configHome, "account.conf"), []byte("CF_Token='must-not-persist'\n"), 0o600); err != nil {
		t.Fatalf("write root account config failed: %v", err)
	}

	if err := runtime.snapshot(); err != nil {
		t.Fatalf("snapshot runtime failed: %v", err)
	}

	reloaded := &model.AcmeAccount{}
	if err := db.Where("id = ?", account.Id).First(reloaded).Error; err != nil {
		t.Fatalf("reload account failed: %v", err)
	}
	if !reloaded.Registered || len(reloaded.RuntimeState) == 0 {
		t.Fatalf("expected registered account runtime state, got registered=%v bytes=%d", reloaded.Registered, len(reloaded.RuntimeState))
	}

	state := acmeRuntimeState{}
	if err := json.Unmarshal(reloaded.RuntimeState, &state); err != nil {
		t.Fatalf("decode runtime state failed: %v", err)
	}
	if _, exists := state.Files["account.conf"]; exists {
		t.Fatalf("root account.conf must not be included in runtime state: %#v", state.Files)
	}
	for relative, encoded := range state.Files {
		content, decodeErr := base64.StdEncoding.DecodeString(encoded)
		if decodeErr != nil {
			t.Fatalf("decode saved file %q failed: %v", relative, decodeErr)
		}
		if strings.Contains(string(content), "CF_Token") {
			t.Fatalf("DNS credential leaked into saved runtime file %q", relative)
		}
	}

	runtimeRoot := runtime.root
	runtime.cleanup()
	if _, statErr := os.Stat(runtimeRoot); !os.IsNotExist(statErr) {
		t.Fatalf("temporary runtime directory should be removed, stat err=%v", statErr)
	}

	restored, err := newAcmeOperationRuntime(reloaded)
	if err != nil {
		t.Fatalf("restore operation runtime failed: %v", err)
	}
	defer restored.cleanup()
	key, err := os.ReadFile(filepath.Join(restored.configHome, "ca", "letsencrypt", "account.key"))
	if err != nil {
		t.Fatalf("read restored account key failed: %v", err)
	}
	if string(key) != "account-private-key" {
		t.Fatalf("unexpected restored account key: %q", key)
	}
	if _, statErr := os.Stat(filepath.Join(restored.configHome, "account.conf")); !os.IsNotExist(statErr) {
		t.Fatalf("root account.conf must not be restored, stat err=%v", statErr)
	}
}

func TestRestoreAcmeRuntimeStateRejectsOversizedAggregate(t *testing.T) {
	files := make(map[string]string, 5)
	content := strings.Repeat("x", acmeRuntimeStateMaxFileLen)
	for i := 0; i < 5; i++ {
		files[fmt.Sprintf("ca/letsencrypt/runtime-%d.conf", i)] = base64.StdEncoding.EncodeToString([]byte(content))
	}
	raw, err := json.Marshal(acmeRuntimeState{Files: files})
	if err != nil {
		t.Fatalf("marshal runtime state failed: %v", err)
	}

	err = restoreAcmeRuntimeState(t.TempDir(), raw)
	if err == nil || !strings.Contains(err.Error(), "runtime is too large") {
		t.Fatalf("expected aggregate runtime size rejection, got %v", err)
	}
}

func TestEnsureOperationRuntimeAccountClearsEmptyLEContact(t *testing.T) {
	state := acmeRuntimeState{Files: map[string]string{
		"ca/letsencrypt/account.key": base64.StdEncoding.EncodeToString([]byte("private-account-key")),
		"ca/letsencrypt/ca.conf":     base64.StdEncoding.EncodeToString([]byte("CA_EMAIL='old@example.com'\nSAVED_CA_EMAIL='old@example.com'\nCA_SERVER='https://acme-v02.api.letsencrypt.org/directory'\n")),
	}}
	raw, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal runtime state failed: %v", err)
	}
	account := &model.AcmeAccount{
		Name:         "empty-contact",
		Email:        "",
		Server:       "letsencrypt",
		Registered:   true,
		RuntimeState: raw,
	}
	runtime, err := newAcmeOperationRuntime(account)
	if err != nil {
		t.Fatalf("create runtime failed: %v", err)
	}
	defer runtime.cleanup()

	calls := make([][]string, 0, 1)
	runner := func(timeout time.Duration, command string, args []string, envPairs []string, logSession *acmeLogSession) (string, error) {
		calls = append(calls, append([]string{}, args...))
		return "", nil
	}
	if err := (&AcmeService{}).ensureOperationRuntimeAccountWithRunner("/tmp/acme.sh", "/tmp/acme-home", runtime, account, "letsencrypt", nil, runner); err != nil {
		t.Fatalf("ensure operation runtime account failed: %v", err)
	}
	if len(calls) != 1 || !containsToken(calls[0], "--update-account") || containsToken(calls[0], "--register-account") {
		t.Fatalf("expected one update-account call, got %#v", calls)
	}
	emptyEmailArgument := false
	for i, arg := range calls[0] {
		if arg == "-m" && i+1 < len(calls[0]) && calls[0][i+1] == "" {
			emptyEmailArgument = true
			break
		}
	}
	if !emptyEmailArgument {
		t.Fatalf("expected an explicit empty -m argument, got %#v", calls[0])
	}
	caConf, err := os.ReadFile(filepath.Join(runtime.configHome, "ca", "letsencrypt", "ca.conf"))
	if err != nil {
		t.Fatalf("read temporary ca.conf failed: %v", err)
	}
	if strings.Contains(string(caConf), "CA_EMAIL") {
		t.Fatalf("temporary CA contact must be removed before empty update: %s", caConf)
	}
}

func TestEnsureOverviewScrubsLegacyAcmeCertificateRuntimeFields(t *testing.T) {
	db := setupAcmeDNSTestDB(t, "acme-legacy-record-scrub.db")
	record := &model.CertificateRecord{
		SourceType:      CertificateSourceACME,
		SourceRef:       "legacy-runtime-scrub",
		MainDomain:      "legacy-scrub.example.com",
		DomainSet:       `["legacy-scrub.example.com"]`,
		CertificateType: acmeCertificateTypeDomain,
		Challenge:       "dns",
		KeyLength:       "ec-256",
		CAServer:        "letsencrypt",
		AcmeHome:        "/legacy/acme",
		DNSEnvText:      "CF_Token=legacy-secret",
		RenewConfig:     "--renew-hook legacy",
		CertPath:        "/legacy/cert.pem",
		KeyPath:         "/legacy/key.pem",
		FullchainPath:   "/legacy/fullchain.pem",
		ChainPath:       "/legacy/chain.pem",
		CertPEM:         []byte("cert-data"),
		KeyPEM:          []byte("key-data"),
		FullchainPEM:    []byte("fullchain-data"),
		LastOutput:      "CF_Token=legacy-secret",
		LastError:       "legacy failure",
	}
	if err := db.Create(record).Error; err != nil {
		t.Fatalf("create legacy certificate record failed: %v", err)
	}

	if err := (&AcmeService{}).EnsureOverviewRuntimeConsistency(true); err != nil {
		t.Fatalf("ensure overview consistency failed: %v", err)
	}
	stored := &model.CertificateRecord{}
	if err := db.Where("id = ?", record.Id).First(stored).Error; err != nil {
		t.Fatalf("reload scrubbed certificate record failed: %v", err)
	}
	if stored.AcmeHome != "" || stored.DNSEnvText != "" || stored.RenewConfig != "" ||
		stored.CertPath != "" || stored.KeyPath != "" || stored.FullchainPath != "" || stored.ChainPath != "" {
		t.Fatalf("legacy runtime fields were not cleared: %#v", stored)
	}
	if stored.LastOutput != "" || stored.LastError != "" {
		t.Fatalf("legacy command output must be cleared: %#v", stored)
	}
	if string(stored.CertPEM) != "cert-data" || string(stored.KeyPEM) != "key-data" || string(stored.FullchainPEM) != "fullchain-data" {
		t.Fatalf("certificate material must be preserved: %#v", stored)
	}
}

func TestLegacyAcmeRuntimeHomesExcludeConfiguredExternalScript(t *testing.T) {
	setupAcmeDNSTestDB(t, "acme-runtime-home-scope.db")
	externalHome := t.TempDir()
	externalScript := filepath.Join(externalHome, "acme.sh")
	if err := (&AcmeService{}).setString(acmeScriptPathKey, externalScript); err != nil {
		t.Fatalf("save configured external script path failed: %v", err)
	}

	for _, home := range (&AcmeService{}).legacyAcmeRuntimeHomes() {
		if filepath.Clean(home) == filepath.Clean(externalHome) {
			t.Fatalf("external acme.sh directory must not be a legacy migration target: %#v", home)
		}
	}
}

func TestAcmeAndDNSDisplayIDsReuseSmallestGap(t *testing.T) {
	db := setupAcmeDNSTestDB(t, "acme-display-id-reuse.db")

	firstACME := &model.AcmeAccount{Name: "acme-first", Server: "letsencrypt", AccountKeyLength: "ec-256"}
	if err := ensureAcmeAccountDisplayID(db, firstACME); err != nil {
		t.Fatalf("allocate first ACME display id failed: %v", err)
	}
	if err := db.Create(firstACME).Error; err != nil {
		t.Fatalf("create first ACME account failed: %v", err)
	}
	secondACME := &model.AcmeAccount{Name: "acme-second", Server: "letsencrypt", AccountKeyLength: "ec-256"}
	if err := ensureAcmeAccountDisplayID(db, secondACME); err != nil {
		t.Fatalf("allocate second ACME display id failed: %v", err)
	}
	if err := db.Create(secondACME).Error; err != nil {
		t.Fatalf("create second ACME account failed: %v", err)
	}
	if firstACME.DisplayID != 1 || secondACME.DisplayID != 2 {
		t.Fatalf("unexpected ACME display ids: first=%d second=%d", firstACME.DisplayID, secondACME.DisplayID)
	}
	if err := db.Delete(firstACME).Error; err != nil {
		t.Fatalf("delete first ACME account failed: %v", err)
	}
	reusedACME := &model.AcmeAccount{Name: "acme-reused", Server: "letsencrypt", AccountKeyLength: "ec-256"}
	if err := ensureAcmeAccountDisplayID(db, reusedACME); err != nil {
		t.Fatalf("reuse ACME display id failed: %v", err)
	}
	if err := db.Create(reusedACME).Error; err != nil {
		t.Fatalf("create reused ACME account failed: %v", err)
	}
	if reusedACME.DisplayID != 1 || reusedACME.Id == firstACME.Id {
		t.Fatalf("expected reusable display id with permanent new primary key: first=%#v reused=%#v", firstACME, reusedACME)
	}

	firstDNS := &model.AcmeDNSAccount{Name: "dns-first", ProviderCode: "dns_cf", EnvJSON: `{"CF_Token":"one"}`}
	if err := ensureDNSAccountDisplayID(db, firstDNS); err != nil {
		t.Fatalf("allocate first DNS display id failed: %v", err)
	}
	if err := db.Create(firstDNS).Error; err != nil {
		t.Fatalf("create first DNS account failed: %v", err)
	}
	secondDNS := &model.AcmeDNSAccount{Name: "dns-second", ProviderCode: "dns_cf", EnvJSON: `{"CF_Token":"two"}`}
	if err := ensureDNSAccountDisplayID(db, secondDNS); err != nil {
		t.Fatalf("allocate second DNS display id failed: %v", err)
	}
	if err := db.Create(secondDNS).Error; err != nil {
		t.Fatalf("create second DNS account failed: %v", err)
	}
	if firstDNS.DisplayID != 1 || secondDNS.DisplayID != 2 {
		t.Fatalf("unexpected DNS display ids: first=%d second=%d", firstDNS.DisplayID, secondDNS.DisplayID)
	}
	if err := db.Delete(firstDNS).Error; err != nil {
		t.Fatalf("delete first DNS account failed: %v", err)
	}
	reusedDNS := &model.AcmeDNSAccount{Name: "dns-reused", ProviderCode: "dns_cf", EnvJSON: `{"CF_Token":"three"}`}
	if err := ensureDNSAccountDisplayID(db, reusedDNS); err != nil {
		t.Fatalf("reuse DNS display id failed: %v", err)
	}
	if err := db.Create(reusedDNS).Error; err != nil {
		t.Fatalf("create reused DNS account failed: %v", err)
	}
	if reusedDNS.DisplayID != 1 || reusedDNS.Id == firstDNS.Id {
		t.Fatalf("expected reusable DNS display id with permanent new primary key: first=%#v reused=%#v", firstDNS, reusedDNS)
	}
}

func TestDeleteAccountsClearsRecordBindingsAndDisablesAutoRenew(t *testing.T) {
	db := setupAcmeDNSTestDB(t, "acme-account-delete-bindings.db")
	acmeAccount := &model.AcmeAccount{Name: "acme-bound", Server: "letsencrypt", AccountKeyLength: "ec-256"}
	if err := db.Create(acmeAccount).Error; err != nil {
		t.Fatalf("create ACME account failed: %v", err)
	}
	dnsAccount := &model.AcmeDNSAccount{Name: "dns-bound", ProviderCode: "dns_cf", EnvJSON: `{"CF_Token":"token"}`}
	if err := db.Create(dnsAccount).Error; err != nil {
		t.Fatalf("create DNS account failed: %v", err)
	}
	record := &model.CertificateRecord{
		SourceType:      CertificateSourceACME,
		SourceRef:       "managed-delete-bindings",
		MainDomain:      "delete-bindings.example.com",
		DomainSet:       `["delete-bindings.example.com"]`,
		CertificateType: acmeCertificateTypeDomain,
		Challenge:       "dns",
		KeyLength:       "ec-256",
		CAServer:        "letsencrypt",
		AutoRenew:       true,
		AcmeAccountID:   acmeAccount.Id,
		AcmeAccountName: acmeAccount.Name,
		DNSAccountID:    dnsAccount.Id,
		DNSAccountName:  dnsAccount.Name,
		CertPEM:         []byte("cert"),
		KeyPEM:          []byte("key"),
		FullchainPEM:    []byte("fullchain"),
	}
	if err := db.Create(record).Error; err != nil {
		t.Fatalf("create certificate record failed: %v", err)
	}

	svc := &AcmeService{}
	if _, err := svc.DeleteAcmeAccount(acmeAccount.Id); err != nil {
		t.Fatalf("delete ACME account failed: %v", err)
	}
	if err := db.Where("id = ?", record.Id).First(record).Error; err != nil {
		t.Fatalf("reload certificate after ACME account deletion failed: %v", err)
	}
	if record.AcmeAccountID != 0 || record.AcmeAccountName != "" || record.AutoRenew {
		t.Fatalf("ACME account deletion must clear only ACME binding and disable renewal: %#v", record)
	}
	if record.DNSAccountID != dnsAccount.Id || record.DNSAccountName != dnsAccount.Name {
		t.Fatalf("DNS binding should remain until DNS account is deleted: %#v", record)
	}

	if _, err := svc.DeleteDNSAccount(dnsAccount.Id); err != nil {
		t.Fatalf("delete DNS account failed: %v", err)
	}
	if err := db.Where("id = ?", record.Id).First(record).Error; err != nil {
		t.Fatalf("reload certificate after DNS account deletion failed: %v", err)
	}
	if record.DNSAccountID != 0 || record.DNSAccountName != "" || record.AutoRenew {
		t.Fatalf("DNS account deletion must clear DNS binding and keep renewal disabled: %#v", record)
	}
}

func TestDeleteCertificateDetachesApplicationsButKeepsAccounts(t *testing.T) {
	db := setupAcmeDNSTestDB(t, "acme-certificate-delete-detach.db")
	acmeAccount := &model.AcmeAccount{Name: "acme-keep-after-cert-delete", Server: "letsencrypt", AccountKeyLength: "ec-256"}
	if err := db.Create(acmeAccount).Error; err != nil {
		t.Fatalf("create ACME account failed: %v", err)
	}
	dnsAccount := &model.AcmeDNSAccount{Name: "dns-keep-after-cert-delete", ProviderCode: "dns_cf", EnvJSON: `{"CF_Token":"token"}`}
	if err := db.Create(dnsAccount).Error; err != nil {
		t.Fatalf("create DNS account failed: %v", err)
	}
	record := &model.CertificateRecord{
		SourceType:      CertificateSourceACME,
		SourceRef:       "managed-delete-detach",
		MainDomain:      "delete-detach.example.com",
		DomainSet:       `["delete-detach.example.com"]`,
		CertificateType: acmeCertificateTypeDomain,
		Challenge:       "dns",
		KeyLength:       "ec-256",
		CAServer:        "letsencrypt",
		AcmeAccountID:   acmeAccount.Id,
		AcmeAccountName: acmeAccount.Name,
		DNSAccountID:    dnsAccount.Id,
		DNSAccountName:  dnsAccount.Name,
		CertPEM:         []byte("cert"),
		KeyPEM:          []byte("key"),
		FullchainPEM:    []byte("fullchain"),
	}
	if err := db.Create(record).Error; err != nil {
		t.Fatalf("create certificate record failed: %v", err)
	}
	if err := SetAssignedCertificateRecordIDs(&SettingService{}, PanelSelfSignedTargetPanel, []uint{record.Id}); err != nil {
		t.Fatalf("assign certificate to panel failed: %v", err)
	}
	tlsRow := &model.Tls{Name: "default-tls", CertificateRecordID: record.Id, Server: json.RawMessage(`{}`), Client: json.RawMessage(`{}`)}
	if err := db.Create(tlsRow).Error; err != nil {
		t.Fatalf("create default TLS binding failed: %v", err)
	}
	mihomoTLSRow := &model.MihomoTls{Name: "mihomo-tls", CertificateRecordID: record.Id, Server: json.RawMessage(`{}`), Client: json.RawMessage(`{}`)}
	if err := db.Create(mihomoTLSRow).Error; err != nil {
		t.Fatalf("create Mihomo TLS binding failed: %v", err)
	}
	reverseProxy := &model.ReverseProxyRule{
		Name:                  "reverse-proxy-binding",
		CertificateRecordID:   record.Id,
		CertificateRecordList: encodeReverseProxyUintList([]uint{record.Id}),
	}
	if err := db.Create(reverseProxy).Error; err != nil {
		t.Fatalf("create reverse proxy binding failed: %v", err)
	}
	if err := db.Create(&model.PanelCertificateBalanceState{ListenerKey: "panel-listener", SNIBucket: "delete-detach.example.com", CertificateRecordID: record.Id}).Error; err != nil {
		t.Fatalf("create panel balance state failed: %v", err)
	}
	if err := db.Create(&model.ReverseProxyCertificateBalanceState{ListenerKey: "reverse-listener", SNIBucket: "delete-detach.example.com", CertificateRecordID: record.Id}).Error; err != nil {
		t.Fatalf("create reverse proxy balance state failed: %v", err)
	}

	if _, err := (&AcmeService{}).Delete(AcmeDeletePayload{ID: record.Id}); err != nil {
		t.Fatalf("delete certificate with bindings failed: %v", err)
	}
	if err := db.Where("id = ?", record.Id).First(&model.CertificateRecord{}).Error; !database.IsNotFound(err) {
		t.Fatalf("certificate record should be deleted, got err=%v", err)
	}
	if err := db.Where("id = ?", acmeAccount.Id).First(&model.AcmeAccount{}).Error; err != nil {
		t.Fatalf("ACME account must remain after certificate deletion: %v", err)
	}
	if err := db.Where("id = ?", dnsAccount.Id).First(&model.AcmeDNSAccount{}).Error; err != nil {
		t.Fatalf("DNS account must remain after certificate deletion: %v", err)
	}
	assigned, err := GetAssignedCertificateRecordIDs(&SettingService{}, PanelSelfSignedTargetPanel)
	if err != nil {
		t.Fatalf("read panel assignments failed: %v", err)
	}
	if len(assigned) != 0 {
		t.Fatalf("panel assignment should be cleared after certificate deletion: %#v", assigned)
	}
	if err := db.Where("id = ?", tlsRow.Id).First(tlsRow).Error; err != nil || tlsRow.CertificateRecordID != 0 {
		t.Fatalf("default TLS binding should be cleared: row=%#v err=%v", tlsRow, err)
	}
	if err := db.Where("id = ?", mihomoTLSRow.Id).First(mihomoTLSRow).Error; err != nil || mihomoTLSRow.CertificateRecordID != 0 {
		t.Fatalf("Mihomo TLS binding should be cleared: row=%#v err=%v", mihomoTLSRow, err)
	}
	if err := db.Where("id = ?", reverseProxy.Id).First(reverseProxy).Error; err != nil {
		t.Fatalf("reload reverse proxy binding failed: %v", err)
	}
	if reverseProxy.CertificateRecordID != 0 || reverseProxy.CertificateRecordList != "" {
		t.Fatalf("reverse proxy certificate binding should be cleared: %#v", reverseProxy)
	}
	var balanceCount int64
	if err := db.Model(&model.PanelCertificateBalanceState{}).Where("certificate_record_id = ?", record.Id).Count(&balanceCount).Error; err != nil || balanceCount != 0 {
		t.Fatalf("panel balance state should be cleared: count=%d err=%v", balanceCount, err)
	}
	if err := db.Model(&model.ReverseProxyCertificateBalanceState{}).Where("certificate_record_id = ?", record.Id).Count(&balanceCount).Error; err != nil || balanceCount != 0 {
		t.Fatalf("reverse proxy balance state should be cleared: count=%d err=%v", balanceCount, err)
	}
}

func TestRemoveAcmeCertificatesAndInventoryDetachesReverseProxyWithSingleRevisionBump(t *testing.T) {
	db := setupAcmeDNSTestDB(t, "acme-remove-all-certificate-bindings.db")
	proxyService := &ReverseProxyService{}
	_ = proxyService.StopRuntime()
	t.Cleanup(func() {
		_ = proxyService.StopRuntime()
	})

	first := &model.CertificateRecord{
		SourceType:   CertificateSourceACME,
		SourceRef:    "remove-all-first",
		MainDomain:   "first.example.com",
		DomainSet:    `["first.example.com"]`,
		CertPEM:      []byte("first-cert"),
		KeyPEM:       []byte("first-key"),
		FullchainPEM: []byte("first-fullchain"),
	}
	second := &model.CertificateRecord{
		SourceType:   CertificateSourceSelfSigned,
		SourceRef:    "remove-all-second",
		MainDomain:   "second.example.com",
		DomainSet:    `["second.example.com"]`,
		CertPEM:      []byte("second-cert"),
		KeyPEM:       []byte("second-key"),
		FullchainPEM: []byte("second-fullchain"),
	}
	if err := db.Create(first).Error; err != nil {
		t.Fatalf("create first certificate failed: %v", err)
	}
	if err := db.Create(second).Error; err != nil {
		t.Fatalf("create second certificate failed: %v", err)
	}

	rule := &model.ReverseProxyRule{
		Name:                  "remove-all-reverse-proxy-binding",
		Enabled:               false,
		CertificateRecordID:   first.Id,
		CertificateRecordList: encodeReverseProxyUintList([]uint{first.Id, second.Id}),
	}
	if err := db.Create(rule).Error; err != nil {
		t.Fatalf("create reverse proxy binding failed: %v", err)
	}

	settings := &model.ReverseProxySettings{}
	if err := db.Where("id = ?", model.ReverseProxySettingsSingletonID).First(settings).Error; err != nil {
		t.Fatalf("load reverse proxy settings failed: %v", err)
	}
	initialRevision := settings.Revision

	if err := (&AcmeService{}).removeAcmeCertificatesAndInventoryLocked(); err != nil {
		t.Fatalf("remove all certificate inventory failed: %v", err)
	}

	if err := db.Where("id = ?", rule.Id).First(rule).Error; err != nil {
		t.Fatalf("reload reverse proxy rule failed: %v", err)
	}
	if rule.CertificateRecordID != 0 || rule.CertificateRecordList != "" {
		t.Fatalf("bulk certificate deletion must clear reverse proxy bindings: %#v", rule)
	}
	if err := db.Where("id = ?", model.ReverseProxySettingsSingletonID).First(settings).Error; err != nil {
		t.Fatalf("reload reverse proxy settings failed: %v", err)
	}
	if settings.Revision != initialRevision+1 {
		t.Fatalf("bulk certificate deletion must bump revision once: before=%d after=%d", initialRevision, settings.Revision)
	}
	var certificateCount int64
	if err := db.Model(&model.CertificateRecord{}).Count(&certificateCount).Error; err != nil {
		t.Fatalf("count certificate inventory failed: %v", err)
	}
	if certificateCount != 0 {
		t.Fatalf("certificate inventory should be empty after bulk deletion, got %d", certificateCount)
	}
}

func TestManualDNSIssueCredentialsBecomeManagedAccount(t *testing.T) {
	setupAcmeDNSTestDB(t, "acme-manual-dns-account.db")
	svc := &AcmeService{}

	cloudflare, err := svc.createManualDNSAccountLocked("dns_cf", "CF_Token=token-only\nCF_Zone_ID=zone-id")
	if err != nil {
		t.Fatalf("create Cloudflare manual DNS account failed: %v", err)
	}
	if cloudflare.DisplayID != 1 || cloudflare.Name != "手工 DNS dns_1" {
		t.Fatalf("unexpected manual DNS account identity: %#v", cloudflare)
	}
	cloudflareEnv, err := parseAcmeEnvJSON(cloudflare.EnvJSON)
	if err != nil {
		t.Fatalf("parse Cloudflare manual DNS env failed: %v", err)
	}
	if cloudflareEnv["CF_Token"] != "token-only" || cloudflareEnv["CF_Zone_ID"] != "zone-id" {
		t.Fatalf("manual Cloudflare DNS credential was not persisted: %#v", cloudflareEnv)
	}

	roleBasedAWS, err := svc.createManualDNSAccountLocked("dns_aws", "")
	if err != nil {
		t.Fatalf("create IAM-role DNS account failed: %v", err)
	}
	if roleBasedAWS.DisplayID != 2 || roleBasedAWS.Name != "手工 DNS dns_2" {
		t.Fatalf("unexpected IAM-role DNS account identity: %#v", roleBasedAWS)
	}
	roleEnv, err := parseAcmeEnvJSON(roleBasedAWS.EnvJSON)
	if err != nil {
		t.Fatalf("parse IAM-role DNS env failed: %v", err)
	}
	if len(roleEnv) != 0 {
		t.Fatalf("IAM-role DNS account should keep empty static credentials: %#v", roleEnv)
	}
}

func TestDNSProviderLifecycleUsesUnifiedCertificateRecords(t *testing.T) {
	db := setupAcmeDNSTestDB(t, "acme-dns-provider-lifecycle.db")
	svc := &AcmeService{}

	if _, err := svc.SaveDNSAccount(AcmeDNSAccountPayload{
		Name:         "dns-lifecycle",
		ProviderCode: "dns_ali",
		EnvJSON:      `{"Ali_Key":"old-key","Ali_Secret":"old-secret"}`,
	}); err != nil {
		t.Fatalf("create unbound DNS account failed: %v", err)
	}
	dnsAccount := &model.AcmeDNSAccount{}
	if err := db.Where("name = ?", "dns-lifecycle").First(dnsAccount).Error; err != nil {
		t.Fatalf("load DNS account failed: %v", err)
	}

	if _, err := svc.SaveDNSAccount(AcmeDNSAccountPayload{
		ID:           dnsAccount.Id,
		Name:         dnsAccount.Name,
		ProviderCode: "dns_cf",
		EnvJSON:      `{"CF_Token":"token-before-binding"}`,
	}); err != nil {
		t.Fatalf("unbound DNS account should allow provider change: %v", err)
	}
	if err := db.Where("id = ?", dnsAccount.Id).First(dnsAccount).Error; err != nil {
		t.Fatalf("reload changed DNS account failed: %v", err)
	}
	if dnsAccount.ProviderCode != "dns_cf" {
		t.Fatalf("expected unbound provider change to persist, got=%q", dnsAccount.ProviderCode)
	}

	record := &model.CertificateRecord{
		SourceType:      CertificateSourceACME,
		SourceRef:       "managed-provider-lifecycle",
		MainDomain:      "provider-lifecycle.example.com",
		DomainSet:       `["provider-lifecycle.example.com"]`,
		CertificateType: acmeCertificateTypeDomain,
		Challenge:       "dns",
		KeyLength:       "ec-256",
		CAServer:        "letsencrypt",
		AutoRenew:       true,
		DNSAccountID:    dnsAccount.Id,
		DNSAccountName:  dnsAccount.Name,
		CertPEM:         []byte("cert"),
		KeyPEM:          []byte("key"),
		FullchainPEM:    []byte("fullchain"),
	}
	if err := db.Create(record).Error; err != nil {
		t.Fatalf("create unified certificate record failed: %v", err)
	}

	if _, err := svc.SaveDNSAccount(AcmeDNSAccountPayload{
		ID:           dnsAccount.Id,
		Name:         dnsAccount.Name,
		ProviderCode: "dns_ali",
		EnvJSON:      `{"Ali_Key":"should-not-save","Ali_Secret":"should-not-save"}`,
	}); err == nil {
		t.Fatal("bound DNS account provider change should be rejected")
	}
	if err := db.Where("id = ?", dnsAccount.Id).First(dnsAccount).Error; err != nil {
		t.Fatalf("reload DNS account after rejected provider change failed: %v", err)
	}
	if dnsAccount.ProviderCode != "dns_cf" {
		t.Fatalf("provider must remain unchanged after rejection, got=%q", dnsAccount.ProviderCode)
	}

	accounts, err := svc.listDNSAccounts()
	if err != nil {
		t.Fatalf("list DNS accounts failed: %v", err)
	}
	if len(accounts) != 1 || !accounts[0].ProviderLocked {
		t.Fatalf("bound unified record should lock provider selection: %#v", accounts)
	}

	if _, err := svc.SaveDNSAccount(AcmeDNSAccountPayload{
		ID:           dnsAccount.Id,
		Name:         dnsAccount.Name,
		ProviderCode: "dns_cf",
		EnvJSON:      `{"CF_Token":"token-after-rotation"}`,
	}); err != nil {
		t.Fatalf("same-provider credential rotation should be allowed: %v", err)
	}
	if err := db.Where("id = ?", record.Id).First(record).Error; err != nil {
		t.Fatalf("reload bound record after credential rotation failed: %v", err)
	}
	if record.DNSAccountID != dnsAccount.Id || !record.AutoRenew {
		t.Fatalf("same-provider credential rotation must retain binding and renewal: %#v", record)
	}
}

func TestIPCertificateRecordUsesHiddenRuntimeWithoutAccountBindings(t *testing.T) {
	db := setupAcmeDNSTestDB(t, "acme-ip-hidden-runtime.db")
	acmeAccount := &model.AcmeAccount{Name: "visible-domain-account", Server: "letsencrypt", AccountKeyLength: "ec-256"}
	if err := db.Create(acmeAccount).Error; err != nil {
		t.Fatalf("create ACME account failed: %v", err)
	}
	dnsAccount := &model.AcmeDNSAccount{Name: "visible-dns-account", ProviderCode: "dns_cf", EnvJSON: `{"CF_Token":"token"}`}
	if err := db.Create(dnsAccount).Error; err != nil {
		t.Fatalf("create DNS account failed: %v", err)
	}
	certPEM, keyPEM, fullchainPEM, chainPEM, err := generateManagedBundleForTest(t, "192.0.2.10", time.Now())
	if err != nil {
		t.Fatalf("generate IP certificate bundle failed: %v", err)
	}
	paths := writeManagedBundleForTest(t, "ip-hidden-runtime", certPEM, keyPEM, fullchainPEM, chainPEM)

	record, err := (&AcmeService{}).upsertAcmeCertificateRecordFromPaths(
		0,
		[]string{"192.0.2.10"},
		acmeCertificateTypeIP,
		"standalone",
		"ec-256",
		acmeLEProductionDirectory,
		true,
		"",
		"",
		"",
		acmeAccount,
		dnsAccount,
		paths,
		true,
		"IP hidden runtime",
		"",
		"",
	)
	if err != nil {
		t.Fatalf("create IP certificate record failed: %v", err)
	}
	if record.AcmeAccountID != 0 || record.AcmeAccountName != "" || record.DNSAccountID != 0 || record.DNSAccountName != "" {
		t.Fatalf("IP certificate must not persist user account bindings: %#v", record)
	}
	if record.AcmeRuntimeProfile != acmeIPRuntimeProfile {
		t.Fatalf("IP certificate must use hidden runtime profile, got=%q", record.AcmeRuntimeProfile)
	}
}

func TestReissueKeepsCertificateRecordAndApplicationAssignment(t *testing.T) {
	db := setupAcmeDNSTestDB(t, "acme-reissue-record.db")
	acmeAccount := &model.AcmeAccount{Name: "acme-reissue", Server: "letsencrypt", AccountKeyLength: "ec-256"}
	if err := db.Create(acmeAccount).Error; err != nil {
		t.Fatalf("create ACME account failed: %v", err)
	}
	dnsAccount := &model.AcmeDNSAccount{Name: "dns-reissue", ProviderCode: "dns_cf", EnvJSON: `{"CF_Token":"token"}`}
	if err := db.Create(dnsAccount).Error; err != nil {
		t.Fatalf("create DNS account failed: %v", err)
	}

	certPEM, keyPEM, fullchainPEM, chainPEM, err := generateManagedBundleForTest(t, "reissue.example.com", time.Now())
	if err != nil {
		t.Fatalf("generate certificate bundle failed: %v", err)
	}
	initial, err := certificateInventory.Upsert(CertificateUpsertPayload{
		SourceType:        CertificateSourceACME,
		SourceRef:         "managed-reissue-record",
		MainDomain:        "reissue.example.com",
		Domains:           []string{"reissue.example.com"},
		CertificateType:   acmeCertificateTypeDomain,
		Challenge:         "dns",
		KeyLength:         "ec-256",
		CAServer:          "letsencrypt",
		UseECC:            true,
		AutoRenew:         true,
		AcmeAccountID:     acmeAccount.Id,
		AcmeAccountName:   acmeAccount.Name,
		DNSAccountID:      dnsAccount.Id,
		DNSAccountName:    dnsAccount.Name,
		ApplyTarget:       "panel",
		PushStateProvided: true,
		PushEnabled:       true,
		PushDir:           "C:/cert-push",
		PushFilePaths:     `{"cert.pem":"C:/cert-push/cert.pem","key.pem":"C:/cert-push/key.pem","fullchain.pem":"C:/cert-push/fullchain.pem"}`,
		Remark:            "before reissue",
		CertPEM:           certPEM,
		KeyPEM:            keyPEM,
		FullchainPEM:      fullchainPEM,
		ChainPEM:          chainPEM,
	})
	if err != nil {
		t.Fatalf("create initial inventory record failed: %v", err)
	}
	if err := SetAssignedCertificateRecordIDs(&SettingService{}, PanelSelfSignedTargetPanel, []uint{initial.Id}); err != nil {
		t.Fatalf("assign initial certificate to panel failed: %v", err)
	}

	newCertPEM, newKeyPEM, newFullchainPEM, newChainPEM, err := generateManagedBundleForTest(t, "reissue.example.com", time.Now().Add(time.Minute))
	if err != nil {
		t.Fatalf("generate reissue certificate bundle failed: %v", err)
	}
	paths := writeManagedBundleForTest(t, "reissued", newCertPEM, newKeyPEM, newFullchainPEM, newChainPEM)
	updated, err := (&AcmeService{}).upsertAcmeCertificateRecordFromPaths(
		initial.Id,
		[]string{"reissue.example.com", "www.reissue.example.com"},
		acmeCertificateTypeDomain,
		"dns",
		"3072",
		"letsencrypt",
		false,
		"",
		"dns_cf",
		"--debug 2",
		acmeAccount,
		dnsAccount,
		paths,
		true,
		"after reissue",
		"",
		"",
	)
	if err != nil {
		t.Fatalf("reissue record upsert failed: %v", err)
	}
	if updated.Id != initial.Id || updated.DisplayID != initial.DisplayID || updated.SourceRef != initial.SourceRef {
		t.Fatalf("reissue must update the same certificate record: before=%#v after=%#v", initial, updated)
	}
	if updated.ApplyTarget != "panel" || !updated.PushEnabled || updated.PushDir != "C:/cert-push" || updated.PushFilePaths != initial.PushFilePaths {
		t.Fatalf("reissue must preserve application and push relationship: %#v", updated)
	}
	if updated.AcmeAccountID != acmeAccount.Id || updated.DNSAccountID != dnsAccount.Id || !updated.AutoRenew {
		t.Fatalf("reissue must preserve selected account bindings and renewal state: %#v", updated)
	}
	assigned, err := GetAssignedCertificateRecordIDs(&SettingService{}, PanelSelfSignedTargetPanel)
	if err != nil {
		t.Fatalf("read panel assignment failed: %v", err)
	}
	if len(assigned) != 1 || assigned[0] != updated.Id {
		t.Fatalf("panel assignment must keep the same record id after reissue: %#v", assigned)
	}
}

func TestACMEAccountEmailAndKeyLengthPolicies(t *testing.T) {
	if normalized, err := validateAcmeAccountEmailForServer("", "letsencrypt"); err != nil || normalized != "" {
		t.Fatalf("Let's Encrypt should allow an empty contact: normalized=%q err=%v", normalized, err)
	}
	if normalized, err := validateAcmeAccountEmailForServer("one@example.com, two@example.com,one@example.com", "letsencrypt"); err != nil || normalized != "one@example.com,two@example.com" {
		t.Fatalf("expected normalized comma-separated contacts, got=%q err=%v", normalized, err)
	}
	if _, err := validateAcmeAccountEmailForServer("", "zerossl"); err == nil {
		t.Fatal("ZeroSSL must require an email contact")
	}

	for _, keyLength := range []string{"2048", "3072", "4096", "8192", "ec-256", "ec-384", "ec-521"} {
		if normalized := normalizeAcmeKeyLength(keyLength); normalized != keyLength {
			t.Fatalf("account key length %q should be supported, got %q", keyLength, normalized)
		}
	}
	if normalized := normalizeAcmeKeyLength("rsa-2048"); normalized != "" {
		t.Fatalf("unexpected non-acme.sh account key alias accepted: %q", normalized)
	}

}

func TestRegisteredACMEAccountRequiresDedicatedKeyRotation(t *testing.T) {
	db := setupAcmeDNSTestDB(t, "acme-account-key-rotation-policy.db")
	account := &model.AcmeAccount{
		Name:             "registered-account",
		Server:           "letsencrypt",
		KeyLength:        "ec-256",
		AccountKeyLength: "ec-256",
		Registered:       true,
		RuntimeState:     []byte(`{"files":{"ca/letsencrypt/account.key":"cHJpdmF0ZS1rZXk="}}`),
	}
	if err := db.Create(account).Error; err != nil {
		t.Fatalf("create registered account failed: %v", err)
	}

	if _, err := (&AcmeService{}).SaveAcmeAccount(AcmeAccountPayload{
		ID:               account.Id,
		Name:             account.Name,
		Server:           account.Server,
		AccountKeyLength: "4096",
	}); err == nil {
		t.Fatal("registered account key length must require the dedicated rotation action")
	}
	reloaded := &model.AcmeAccount{}
	if err := db.Where("id = ?", account.Id).First(reloaded).Error; err != nil {
		t.Fatalf("reload registered account failed: %v", err)
	}
	if reloaded.AccountKeyLength != "ec-256" {
		t.Fatalf("regular account save must not rotate the registered key, got=%q", reloaded.AccountKeyLength)
	}
}
