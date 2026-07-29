package service

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
)

func TestBuildAcmeIssueCommandArgsAddsIPFamilyListenFlags(t *testing.T) {
	ipv4Args := buildAcmeIssueCommandArgs(
		[]string{"149.104.4.229"},
		"standalone",
		"",
		"",
		"ec-384",
		acmeLEProductionDirectory,
		"",
		true,
		acmeIPFamilyIPv4,
	)
	assertArgIncluded(t, ipv4Args, "--listen-v4")
	assertArgIncluded(t, ipv4Args, "--cert-profile")
	assertArgIncluded(t, ipv4Args, "shortlived")
	assertArgIncluded(t, ipv4Args, "--server")
	assertArgIncluded(t, ipv4Args, acmeLEProductionDirectory)

	ipv6Args := buildAcmeIssueCommandArgs(
		[]string{"2400:f880:dbf:8a82::38b"},
		"standalone",
		"",
		"",
		"ec-384",
		acmeLEProductionDirectory,
		"",
		true,
		acmeIPFamilyIPv6,
	)
	assertArgIncluded(t, ipv6Args, "--listen-v6")

	dualArgs := buildAcmeIssueCommandArgs(
		[]string{"149.104.4.229", "2400:f880:dbf:8a82::38b"},
		"standalone",
		"",
		"",
		"ec-384",
		acmeLEProductionDirectory,
		"",
		true,
		acmeIPFamilyDual,
	)
	assertArgNotIncluded(t, dualArgs, "--listen-v4")
	assertArgNotIncluded(t, dualArgs, "--listen-v6")
}

func TestDetectAcmeIPFamilyMode(t *testing.T) {
	if got := detectAcmeIPFamilyMode([]string{"149.104.4.229"}); got != acmeIPFamilyIPv4 {
		t.Fatalf("ipv4 mode = %q", got)
	}
	if got := detectAcmeIPFamilyMode([]string{"2400:f880:dbf:8a82::38b"}); got != acmeIPFamilyIPv6 {
		t.Fatalf("ipv6 mode = %q", got)
	}
	if got := detectAcmeIPFamilyMode([]string{"149.104.4.229", "2400:f880:dbf:8a82::38b"}); got != acmeIPFamilyDual {
		t.Fatalf("dual mode = %q", got)
	}
}

func TestCleanupStaleManagedAcmeInstallWorkspacesRemovesOnlyManagedWorkspaceDirs(t *testing.T) {
	parentDir := t.TempDir()
	stageDir := filepath.Join(parentDir, acmeManagedWorkspaceStagePrefix+"old")
	backupDir := filepath.Join(parentDir, acmeManagedWorkspaceBackupPrefix+"old")
	keepDir := filepath.Join(parentDir, "acme")
	if err := os.MkdirAll(filepath.Join(stageDir, "dnsapi"), 0o755); err != nil {
		t.Fatalf("mkdir stage workspace failed: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(backupDir, "dnsapi"), 0o755); err != nil {
		t.Fatalf("mkdir backup workspace failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stageDir, "acme.sh"), []byte("stage"), 0o644); err != nil {
		t.Fatalf("write stage acme.sh failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(backupDir, "account.conf"), []byte("ACCOUNT_EMAIL='backup@example.com'"), 0o600); err != nil {
		t.Fatalf("write backup account.conf failed: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(keepDir, "ca"), 0o755); err != nil {
		t.Fatalf("mkdir keep ca failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(keepDir, "account.conf"), []byte("ACCOUNT_EMAIL='keep@example.com'"), 0o600); err != nil {
		t.Fatalf("write keep account.conf failed: %v", err)
	}

	if err := cleanupStaleManagedAcmeInstallWorkspaces(parentDir); err != nil {
		t.Fatalf("cleanupStaleManagedAcmeInstallWorkspaces failed: %v", err)
	}

	if _, err := os.Stat(stageDir); !os.IsNotExist(err) {
		t.Fatalf("expected stage workspace removed, stat err=%v", err)
	}
	if _, err := os.Stat(backupDir); !os.IsNotExist(err) {
		t.Fatalf("expected backup workspace removed, stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(keepDir, "account.conf")); err != nil {
		t.Fatalf("expected keep account.conf preserved, stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(keepDir, "ca")); err != nil {
		t.Fatalf("expected keep ca preserved, stat err=%v", err)
	}
}

func TestConvertCertificateRecordIncludesIssuedAlgorithms(t *testing.T) {
	setupAcmeIPBehaviorTestDB(t, "acme-ip-behavior-view.db")
	_, fullchainPEM, err := (&ServerService{}).generateCertWithAlgorithm(
		"test.example.com",
		"ecc256",
		"ecc384",
		tlsCertificateUsageServer,
		time.Now().Add(-time.Hour),
		time.Now().Add(24*time.Hour),
	)
	if err != nil {
		t.Fatalf("generate cert failed: %v", err)
	}
	row := &model.CertificateRecord{
		Id:           7,
		SourceType:   CertificateSourceACME,
		SourceRef:    "7",
		MainDomain:   "test.example.com",
		DomainSet:    `["test.example.com"]`,
		Challenge:    "standalone",
		KeyLength:    "ec-384",
		CAServer:     acmeLEProductionDirectory,
		FullchainPEM: fullchainPEM,
	}

	view := convertCertificateRecord(row)
	if strings.TrimSpace(view.IssuedKeyAlgorithm) == "" {
		t.Fatal("expected issued key algorithm")
	}
	if strings.TrimSpace(view.IssuedSignatureAlgorithm) == "" {
		t.Fatal("expected issued signature algorithm")
	}
}

func TestApplyCertificateRecordPostActionsOnlyUsesExplicitPushDir(t *testing.T) {
	setupAcmeIPBehaviorTestDB(t, "acme-ip-behavior-pushdir.db")
	svc := &AcmeService{}

	record, err := certificateInventory.Upsert(CertificateUpsertPayload{
		SourceType:      CertificateSourceACME,
		SourceRef:       "post-action-push",
		MainDomain:      "test.example.com",
		Domains:         []string{"test.example.com"},
		CertificateType: acmeCertificateTypeDomain,
		Challenge:       "standalone",
		KeyLength:       "ec-384",
		CAServer:        acmeLEProductionDirectory,
		CertPEM:         []byte("test-cert"),
		KeyPEM:          []byte("test-key"),
		FullchainPEM:    []byte("test-fullchain"),
		ChainPEM:        []byte("test-chain"),
	})
	if err != nil {
		t.Fatalf("create certificate record failed: %v", err)
	}

	targetDir := filepath.Join(t.TempDir(), "bundle")
	if warnings := svc.applyCertificateRecordPostActions(record, "", targetDir, true); len(warnings) > 0 {
		t.Fatalf("applyCertificateRecordPostActions returned warnings: %v", warnings)
	}

	record, err = certificateInventory.GetRecordByID(record.Id)
	if err != nil {
		t.Fatalf("load inventory record failed: %v", err)
	}
	if got := strings.TrimSpace(record.PushDir); got != targetDir {
		t.Fatalf("expected inventory push dir updated: got=%q want=%q", got, targetDir)
	}
	if !record.PushEnabled {
		t.Fatal("expected inventory push state to be marked verified")
	}
	if got := strings.TrimSpace(record.PushFilePaths); got == "" {
		t.Fatal("expected inventory full push file paths to be recorded")
	}
}

func TestApplyCertificateRecordPostActionsRewritesVerifiedPushWhenNotExplicit(t *testing.T) {
	setupAcmeIPBehaviorTestDB(t, "acme-ip-behavior-pushdir-rewrite.db")
	svc := &AcmeService{}

	oldDir := filepath.Join(t.TempDir(), "old-push")

	record, err := certificateInventory.Upsert(CertificateUpsertPayload{
		SourceType:      CertificateSourceACME,
		SourceRef:       "post-action-rewrite",
		MainDomain:      "test-rewrite.example.com",
		Domains:         []string{"test-rewrite.example.com"},
		CertificateType: acmeCertificateTypeDomain,
		Challenge:       "standalone",
		KeyLength:       "ec-384",
		CAServer:        acmeLEProductionDirectory,
		CertPEM:         []byte("first-cert"),
		KeyPEM:          []byte("first-key"),
		FullchainPEM:    []byte("first-fullchain"),
		ChainPEM:        []byte("first-chain"),
	})
	if err != nil {
		t.Fatalf("create certificate record failed: %v", err)
	}
	if _, err := svc.Push(AcmePushPayload{ID: record.Id, TargetDir: oldDir}); err != nil {
		t.Fatalf("seed verified directory push failed: %v", err)
	}
	record, err = certificateInventory.GetRecordByID(record.Id)
	if err != nil {
		t.Fatalf("reload verified inventory record failed: %v", err)
	}
	record.CertPEM = []byte("renewed-cert")
	record.KeyPEM = []byte("renewed-key")
	record.FullchainPEM = []byte("renewed-fullchain")
	record.ChainPEM = nil
	if err := database.GetDB().Save(record).Error; err != nil {
		t.Fatalf("save renewed certificate material failed: %v", err)
	}

	if warnings := svc.applyCertificateRecordPostActions(record, "", "", false); len(warnings) > 0 {
		t.Fatalf("applyCertificateRecordPostActions returned warnings: %v", warnings)
	}
	reloadedRecord, err := certificateInventory.GetRecordByID(record.Id)
	if err != nil {
		t.Fatalf("reload inventory record failed: %v", err)
	}
	if got := strings.TrimSpace(reloadedRecord.PushDir); got != oldDir {
		t.Fatalf("expected inventory push dir unchanged: got=%q want=%q", got, oldDir)
	}
	if !reloadedRecord.PushEnabled {
		t.Fatal("expected verified directory push state to remain enabled")
	}
	if actual, readErr := os.ReadFile(filepath.Join(oldDir, "cert.pem")); readErr != nil || string(actual) != "renewed-cert" {
		t.Fatalf("expected cert.pem rewritten for verified push, content=%q err=%v", string(actual), readErr)
	}
	if _, statErr := os.Stat(filepath.Join(oldDir, "chain.pem")); !os.IsNotExist(statErr) {
		t.Fatalf("expected obsolete chain.pem removed during verified push rewrite, stat err=%v", statErr)
	}
}

func TestPostActionWarningsPersistWithoutChangingCertificateStatus(t *testing.T) {
	setupAcmeIPBehaviorTestDB(t, "acme-post-action-warning.db")
	record, err := certificateInventory.Upsert(CertificateUpsertPayload{
		SourceType:      CertificateSourceACME,
		SourceRef:       "post-action-warning",
		MainDomain:      "warning.example.com",
		Domains:         []string{"warning.example.com"},
		CertificateType: acmeCertificateTypeDomain,
		Challenge:       "standalone",
		NotAfter:        time.Now().Add(24 * time.Hour).Unix(),
		CertPEM:         []byte("test-cert"),
		KeyPEM:          []byte("test-key"),
		FullchainPEM:    []byte("test-fullchain"),
	})
	if err != nil {
		t.Fatalf("create certificate record failed: %v", err)
	}

	warnings := persistCertificatePostActionWarnings(record, []string{"推送证书目录失败: permission denied"})
	if len(warnings) != 1 {
		t.Fatalf("unexpected persisted warnings: %v", warnings)
	}
	reloaded, err := certificateInventory.GetRecordByID(record.Id)
	if err != nil {
		t.Fatalf("reload certificate record failed: %v", err)
	}
	view := convertCertificateRecord(reloaded)
	if view.Status != "normal" {
		t.Fatalf("post-action warning must not change certificate validity status: %q", view.Status)
	}
	if !strings.Contains(view.PostActionError, "permission denied") {
		t.Fatalf("expected post-action warning in inventory view: %q", view.PostActionError)
	}

	if err := clearCertificatePostActionError(reloaded); err != nil {
		t.Fatalf("clear post-action warning failed: %v", err)
	}
	reloaded, err = certificateInventory.GetRecordByID(record.Id)
	if err != nil {
		t.Fatalf("reload cleared certificate record failed: %v", err)
	}
	if strings.TrimSpace(reloaded.PostActionError) != "" {
		t.Fatalf("expected post-action warning to be cleared: %q", reloaded.PostActionError)
	}
}

func TestPushUpdatesCertificateRecordAndPreservesTrackedFiles(t *testing.T) {
	setupAcmeIPBehaviorTestDB(t, "acme-push-sync-source.db")
	svc := &AcmeService{}

	record, err := certificateInventory.Upsert(CertificateUpsertPayload{
		SourceType:      CertificateSourceACME,
		SourceRef:       "push-record",
		MainDomain:      "push.example.com",
		Domains:         []string{"push.example.com"},
		CertificateType: acmeCertificateTypeDomain,
		Challenge:       "standalone",
		KeyLength:       "ec-256",
		CAServer:        acmeLEProductionDirectory,
		CertPEM:         []byte("cert-a"),
		KeyPEM:          []byte("key-a"),
		FullchainPEM:    []byte("fullchain-a"),
	})
	if err != nil {
		t.Fatalf("create certificate record failed: %v", err)
	}

	oldDir := filepath.Join(t.TempDir(), "old-push")
	if _, err := svc.Push(AcmePushPayload{ID: record.Id, TargetDir: oldDir}); err != nil {
		t.Fatalf("seed verified old push dir failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(oldDir, "keep.txt"), []byte("keep"), 0o644); err != nil {
		t.Fatalf("write keep.txt failed: %v", err)
	}

	newDir := filepath.Join(t.TempDir(), "new-push")
	result, err := svc.Push(AcmePushPayload{ID: record.Id, TargetDir: newDir})
	if err != nil {
		t.Fatalf("push failed: %v", err)
	}
	if result == nil || result.Certificate == nil {
		t.Fatal("expected push result certificate")
	}

	reloadedRecord, err := certificateInventory.GetRecordByID(record.Id)
	if err != nil {
		t.Fatalf("reload inventory record failed: %v", err)
	}
	if got := strings.TrimSpace(reloadedRecord.PushDir); got != newDir {
		t.Fatalf("expected inventory push dir synced: got=%q want=%q", got, newDir)
	}
	if !reloadedRecord.PushEnabled {
		t.Fatal("expected certificate record push state marked verified")
	}
	if got := strings.TrimSpace(reloadedRecord.PushFilePaths); got == "" {
		t.Fatal("expected certificate record full push file paths recorded")
	}

	for _, name := range []string{"cert.pem", "key.pem", "fullchain.pem"} {
		if _, statErr := os.Stat(filepath.Join(oldDir, name)); !os.IsNotExist(statErr) {
			t.Fatalf("expected old %s removed, stat err=%v", name, statErr)
		}
		if _, statErr := os.Stat(filepath.Join(newDir, name)); statErr != nil {
			t.Fatalf("expected new %s created, stat err=%v", name, statErr)
		}
	}
	if _, statErr := os.Stat(filepath.Join(oldDir, "keep.txt")); statErr != nil {
		t.Fatalf("expected keep.txt preserved, stat err=%v", statErr)
	}
}

func TestPushWithoutChainRecordsOnlyWrittenPaths(t *testing.T) {
	setupAcmeIPBehaviorTestDB(t, "acme-push-without-chain.db")
	svc := &AcmeService{}
	record, err := certificateInventory.Upsert(CertificateUpsertPayload{
		SourceType:      CertificateSourceACME,
		SourceRef:       "push-no-chain",
		MainDomain:      "no-chain.example.com",
		Domains:         []string{"no-chain.example.com"},
		CertificateType: acmeCertificateTypeDomain,
		Challenge:       "standalone",
		KeyLength:       "ec-256",
		CAServer:        acmeLEProductionDirectory,
		CertPEM:         []byte("cert"),
		KeyPEM:          []byte("key"),
		FullchainPEM:    []byte("fullchain"),
	})
	if err != nil {
		t.Fatalf("create certificate record failed: %v", err)
	}

	targetDir := filepath.Join(t.TempDir(), "bundle")
	if _, err := svc.Push(AcmePushPayload{ID: record.Id, TargetDir: targetDir}); err != nil {
		t.Fatalf("push failed: %v", err)
	}
	reloaded, err := certificateInventory.GetRecordByID(record.Id)
	if err != nil {
		t.Fatalf("reload pushed record failed: %v", err)
	}
	paths := decodeCertificatePushFilePaths(reloaded.PushFilePaths)
	if len(paths) != 3 || paths["chain.pem"] != "" {
		t.Fatalf("expected only three pushed file paths, got=%#v", paths)
	}
	if _, statErr := os.Stat(filepath.Join(targetDir, "chain.pem")); !os.IsNotExist(statErr) {
		t.Fatalf("expected chain.pem absent when bundle has no chain, stat err=%v", statErr)
	}
}

func TestPushFailureDoesNotMarkUnverifiedDirectoryState(t *testing.T) {
	setupAcmeIPBehaviorTestDB(t, "acme-push-failure-state.db")
	svc := &AcmeService{}
	record, err := certificateInventory.Upsert(CertificateUpsertPayload{
		SourceType:      CertificateSourceACME,
		SourceRef:       "push-failure",
		MainDomain:      "failure.example.com",
		Domains:         []string{"failure.example.com"},
		CertificateType: acmeCertificateTypeDomain,
		Challenge:       "standalone",
		KeyLength:       "ec-256",
		CAServer:        acmeLEProductionDirectory,
		CertPEM:         []byte("cert"),
		KeyPEM:          []byte("key"),
		FullchainPEM:    []byte("fullchain"),
	})
	if err != nil {
		t.Fatalf("create certificate record failed: %v", err)
	}

	targetDir := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(targetDir, []byte("file"), 0o644); err != nil {
		t.Fatalf("prepare invalid target path failed: %v", err)
	}
	if _, err := svc.Push(AcmePushPayload{ID: record.Id, TargetDir: targetDir}); err == nil {
		t.Fatal("expected directory push failure")
	}
	reloaded, err := certificateInventory.GetRecordByID(record.Id)
	if err != nil {
		t.Fatalf("reload failed push record failed: %v", err)
	}
	if reloaded.PushEnabled || strings.TrimSpace(reloaded.PushDir) != "" || strings.TrimSpace(reloaded.PushFilePaths) != "" {
		t.Fatalf("failed push must not mark an unverified directory state: %#v", reloaded)
	}
}

func TestClearPushDeletesOnlyVerifiedCertificateFiles(t *testing.T) {
	setupAcmeIPBehaviorTestDB(t, "acme-push-clear.db")
	svc := &AcmeService{}
	record, err := certificateInventory.Upsert(CertificateUpsertPayload{
		SourceType:      CertificateSourceACME,
		SourceRef:       "push-clear",
		MainDomain:      "clear.example.com",
		Domains:         []string{"clear.example.com"},
		CertificateType: acmeCertificateTypeDomain,
		Challenge:       "standalone",
		KeyLength:       "ec-256",
		CAServer:        acmeLEProductionDirectory,
		CertPEM:         []byte("cert"),
		KeyPEM:          []byte("key"),
		FullchainPEM:    []byte("fullchain"),
	})
	if err != nil {
		t.Fatalf("create certificate record failed: %v", err)
	}

	targetDir := filepath.Join(t.TempDir(), "bundle")
	if _, err := svc.Push(AcmePushPayload{ID: record.Id, TargetDir: targetDir}); err != nil {
		t.Fatalf("seed push failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(targetDir, "keep.txt"), []byte("keep"), 0o644); err != nil {
		t.Fatalf("write unrelated file failed: %v", err)
	}
	if _, err := svc.Push(AcmePushPayload{ID: record.Id, Clear: true}); err != nil {
		t.Fatalf("clear push failed: %v", err)
	}
	reloaded, err := certificateInventory.GetRecordByID(record.Id)
	if err != nil {
		t.Fatalf("reload cleared record failed: %v", err)
	}
	if reloaded.PushEnabled || strings.TrimSpace(reloaded.PushDir) != "" || strings.TrimSpace(reloaded.PushFilePaths) != "" {
		t.Fatalf("expected push state cleared, got %#v", reloaded)
	}
	for _, name := range []string{"cert.pem", "key.pem", "fullchain.pem"} {
		if _, statErr := os.Stat(filepath.Join(targetDir, name)); !os.IsNotExist(statErr) {
			t.Fatalf("expected %s deleted, stat err=%v", name, statErr)
		}
	}
	if _, statErr := os.Stat(targetDir); statErr != nil {
		t.Fatalf("expected target directory preserved, stat err=%v", statErr)
	}
	if content, readErr := os.ReadFile(filepath.Join(targetDir, "keep.txt")); readErr != nil || string(content) != "keep" {
		t.Fatalf("expected unrelated file preserved, content=%q err=%v", string(content), readErr)
	}
}

func TestGetOverviewRemovesLegacyDefaultPushSetting(t *testing.T) {
	setupAcmeIPBehaviorTestDB(t, "acme-ip-behavior-legacy-setting.db")
	svc := &AcmeService{}
	if err := svc.setString("acmeDefaultPushDir", "/legacy/default"); err != nil {
		t.Fatalf("set legacy default push dir failed: %v", err)
	}

	if _, err := svc.GetOverview(); err != nil {
		t.Fatalf("GetOverview failed: %v", err)
	}

	var count int64
	if err := database.GetDB().Model(&model.Setting{}).Where("key = ?", "acmeDefaultPushDir").Count(&count).Error; err != nil {
		t.Fatalf("count legacy setting failed: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected legacy default push dir setting removed, count=%d", count)
	}
}

func TestCleanupLegacyCertificateManagedDirKeepsUnknownFiles(t *testing.T) {
	root := filepath.Join(t.TempDir(), "legacy-live")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("create root failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "cert.pem"), []byte("cert"), 0o644); err != nil {
		t.Fatalf("write cert failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "note.txt"), []byte("keep"), 0o644); err != nil {
		t.Fatalf("write note failed: %v", err)
	}

	if err := cleanupLegacyCertificateManagedDir(root, map[string]struct{}{
		"cert.pem":      {},
		"key.pem":       {},
		"fullchain.pem": {},
		"chain.pem":     {},
	}, false); err != nil {
		t.Fatalf("cleanup failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(root, "cert.pem")); !os.IsNotExist(err) {
		t.Fatalf("expected cert.pem removed, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "note.txt")); err != nil {
		t.Fatalf("expected note.txt kept, err=%v", err)
	}
}

func TestDeleteImportedSettingsPathCertificateClearsLegacySourceSettings(t *testing.T) {
	setupAcmeIPBehaviorTestDB(t, "acme-delete-imported-settings.db")

	settingService := &SettingService{}
	if _, err := settingService.GetAllSetting(); err != nil {
		t.Fatalf("seed default settings failed: %v", err)
	}
	if err := settingService.SaveSetting("webCertFile", "/tmp/panel/fullchain.pem"); err != nil {
		t.Fatalf("set web cert path failed: %v", err)
	}
	if err := settingService.SaveSetting("webKeyFile", "/tmp/panel/privkey.pem"); err != nil {
		t.Fatalf("set web key path failed: %v", err)
	}

	row, err := (&CertificateInventoryService{}).Upsert(CertificateUpsertPayload{
		SourceType:    CertificateSourceImported,
		SourceRef:     BuildImportedSourceRef(PanelSelfSignedTargetPanel),
		MainDomain:    "149.104.4.229",
		Domains:       []string{"149.104.4.229"},
		CertPath:      "/tmp/panel/fullchain.pem",
		KeyPath:       "/tmp/panel/privkey.pem",
		FullchainPath: "/tmp/panel/fullchain.pem",
		CertPEM:       []byte("cert"),
		KeyPEM:        []byte("key"),
		FullchainPEM:  []byte("cert"),
		LastIssuedAt:  100,
		LastRenewedAt: 100,
	})
	if err != nil {
		t.Fatalf("upsert imported certificate failed: %v", err)
	}

	result, err := (&AcmeService{}).Delete(AcmeDeletePayload{ID: row.Id})
	if err != nil {
		t.Fatalf("delete imported certificate failed: %v", err)
	}
	if result == nil {
		t.Fatal("expected delete result")
	}

	var count int64
	if err := database.GetDB().Model(&model.CertificateRecord{}).Where("id = ?", row.Id).Count(&count).Error; err != nil {
		t.Fatalf("count certificate record failed: %v", err)
	}
	if count != 0 {
		t.Fatalf("expected certificate record removed, count=%d", count)
	}

	webCertFile, err := settingService.GetCertFile()
	if err != nil {
		t.Fatalf("read web cert path failed: %v", err)
	}
	if strings.TrimSpace(webCertFile) != "" {
		t.Fatalf("expected web cert path cleared, got=%q", webCertFile)
	}

	webKeyFile, err := settingService.GetKeyFile()
	if err != nil {
		t.Fatalf("read web key path failed: %v", err)
	}
	if strings.TrimSpace(webKeyFile) != "" {
		t.Fatalf("expected web key path cleared, got=%q", webKeyFile)
	}
}

func TestMigrateLegacySettingsPathCertificatesToInventoryClearsLegacySettings(t *testing.T) {
	setupAcmeIPBehaviorTestDB(t, "acme-migrate-legacy-settings.db")

	settingService := &SettingService{}
	if _, err := settingService.GetAllSetting(); err != nil {
		t.Fatalf("seed default settings failed: %v", err)
	}
	certDir := t.TempDir()
	generated, err := GeneratePanelSelfSignedCertificateInDir(certDir)
	if err != nil {
		t.Fatalf("generate legacy panel certificate failed: %v", err)
	}
	if err := settingService.SaveSetting("webCertFile", generated.CertPath); err != nil {
		t.Fatalf("set web cert path failed: %v", err)
	}
	if err := settingService.SaveSetting("webKeyFile", generated.KeyPath); err != nil {
		t.Fatalf("set web key path failed: %v", err)
	}

	if err := MigrateLegacySettingsPathCertificatesToInventory(settingService); err != nil {
		t.Fatalf("migrate legacy settings-path certificates failed: %v", err)
	}

	webCertFile, err := settingService.GetCertFile()
	if err != nil {
		t.Fatalf("read web cert path failed: %v", err)
	}
	if strings.TrimSpace(webCertFile) != "" {
		t.Fatalf("expected web cert path cleared, got=%q", webCertFile)
	}

	webKeyFile, err := settingService.GetKeyFile()
	if err != nil {
		t.Fatalf("read web key path failed: %v", err)
	}
	if strings.TrimSpace(webKeyFile) != "" {
		t.Fatalf("expected web key path cleared, got=%q", webKeyFile)
	}

	assignedID, err := GetAssignedCertificateRecordID(settingService, PanelSelfSignedTargetPanel)
	if err != nil {
		t.Fatalf("read assigned certificate id failed: %v", err)
	}
	if assignedID == 0 {
		t.Fatal("expected assigned certificate id after migration")
	}
	row, err := certificateInventory.GetRecordByID(assignedID)
	if err != nil {
		t.Fatalf("read migrated inventory row failed: %v", err)
	}
	if row == nil {
		t.Fatal("expected migrated inventory row")
	}
	if row.SourceRef != BuildImportedSourceRef(PanelSelfSignedTargetPanel) {
		t.Fatalf("unexpected source ref: %s", row.SourceRef)
	}
}

func TestCertificateInventoryDisplayIDReusesSmallestGap(t *testing.T) {
	setupAcmeIPBehaviorTestDB(t, "acme-display-id-gap.db")

	svc := &CertificateInventoryService{}
	first, err := svc.Upsert(CertificateUpsertPayload{
		SourceType:   CertificateSourceACME,
		SourceRef:    "1",
		MainDomain:   "one.example.com",
		Domains:      []string{"one.example.com"},
		Challenge:    "standalone",
		KeyLength:    "ec-256",
		CAServer:     acmeLEProductionDirectory,
		CertPEM:      []byte("cert-1"),
		KeyPEM:       []byte("key-1"),
		LastIssuedAt: 100,
	})
	if err != nil {
		t.Fatalf("upsert first failed: %v", err)
	}
	second, err := svc.Upsert(CertificateUpsertPayload{
		SourceType:   CertificateSourceACME,
		SourceRef:    "2",
		MainDomain:   "two.example.com",
		Domains:      []string{"two.example.com"},
		Challenge:    "standalone",
		KeyLength:    "ec-256",
		CAServer:     acmeLEProductionDirectory,
		CertPEM:      []byte("cert-2"),
		KeyPEM:       []byte("key-2"),
		LastIssuedAt: 200,
	})
	if err != nil {
		t.Fatalf("upsert second failed: %v", err)
	}
	third, err := svc.Upsert(CertificateUpsertPayload{
		SourceType:   CertificateSourceACME,
		SourceRef:    "3",
		MainDomain:   "three.example.com",
		Domains:      []string{"three.example.com"},
		Challenge:    "standalone",
		KeyLength:    "ec-256",
		CAServer:     acmeLEProductionDirectory,
		CertPEM:      []byte("cert-3"),
		KeyPEM:       []byte("key-3"),
		LastIssuedAt: 300,
	})
	if err != nil {
		t.Fatalf("upsert third failed: %v", err)
	}

	if first.DisplayID != 1 || second.DisplayID != 2 || third.DisplayID != 3 {
		t.Fatalf("unexpected initial display ids: %d %d %d", first.DisplayID, second.DisplayID, third.DisplayID)
	}

	if err := svc.DeleteByID(second.Id); err != nil {
		t.Fatalf("delete second failed: %v", err)
	}

	fourth, err := svc.Upsert(CertificateUpsertPayload{
		SourceType:   CertificateSourceACME,
		SourceRef:    "4",
		MainDomain:   "four.example.com",
		Domains:      []string{"four.example.com"},
		Challenge:    "standalone",
		KeyLength:    "ec-256",
		CAServer:     acmeLEProductionDirectory,
		CertPEM:      []byte("cert-4"),
		KeyPEM:       []byte("key-4"),
		LastIssuedAt: 400,
	})
	if err != nil {
		t.Fatalf("upsert fourth failed: %v", err)
	}
	if fourth.DisplayID != 2 {
		t.Fatalf("expected display id 2 reused, got=%d", fourth.DisplayID)
	}
}

func TestUpsertCertificateRecordFromPathsCreatesNewRowsForSameIP(t *testing.T) {
	setupAcmeIPBehaviorTestDB(t, "acme-same-ip-new-rows.db")

	now := time.Now()
	certPEM, keyPEM, fullchainPEM, chainPEM, err := generateManagedBundleForTest(t, "149.104.4.229", now)
	if err != nil {
		t.Fatalf("generate bundle failed: %v", err)
	}
	paths1 := writeManagedBundleForTest(t, "149.104.4.229-a", certPEM, keyPEM, fullchainPEM, chainPEM)
	paths2 := writeManagedBundleForTest(t, "149.104.4.229-b", certPEM, keyPEM, fullchainPEM, chainPEM)

	svc := &AcmeService{}
	first, err := svc.upsertAcmeCertificateRecordFromPaths(
		0,
		[]string{"149.104.4.229"},
		acmeCertificateTypeIP,
		"standalone",
		"ec-384",
		acmeLEProductionDirectory,
		true,
		"",
		"",
		"",
		nil,
		nil,
		paths1,
		true,
		"",
		"",
		"",
	)
	if err != nil {
		t.Fatalf("upsert first failed: %v", err)
	}
	second, err := svc.upsertAcmeCertificateRecordFromPaths(
		0,
		[]string{"149.104.4.229"},
		acmeCertificateTypeIP,
		"standalone",
		"ec-384",
		acmeLEProductionDirectory,
		true,
		"",
		"",
		"",
		nil,
		nil,
		paths2,
		true,
		"",
		"",
		"",
	)
	if err != nil {
		t.Fatalf("upsert second failed: %v", err)
	}

	if first.Id == 0 || second.Id == 0 || first.Id == second.Id {
		t.Fatalf("expected distinct certificate records, got first=%d second=%d", first.Id, second.Id)
	}

	var count int64
	if err := database.GetDB().Model(&model.CertificateRecord{}).Where("source_type = ? AND main_domain = ?", CertificateSourceACME, "149.104.4.229").Count(&count).Error; err != nil {
		t.Fatalf("count same ip rows failed: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 certificate records for same ip, got=%d", count)
	}
}

func TestCertificateInventoryRepairsDisplayIDAndListOrder(t *testing.T) {
	setupAcmeIPBehaviorTestDB(t, "acme-sync-repair.db")

	row, err := certificateInventory.Upsert(CertificateUpsertPayload{
		SourceType:      CertificateSourceACME,
		SourceRef:       "repair-display-id",
		MainDomain:      "repair.example.com",
		Domains:         []string{"repair.example.com"},
		CertificateType: acmeCertificateTypeDomain,
		Challenge:       "standalone",
		KeyLength:       "ec-256",
		CAServer:        acmeLEProductionDirectory,
		UseECC:          true,
		CertPEM:         []byte("cert"),
		KeyPEM:          []byte("key"),
		FullchainPEM:    []byte("fullchain"),
		ChainPEM:        []byte("chain"),
		Fingerprint:     "fp-repair",
		NotBefore:       1000,
		NotAfter:        2000,
		LastIssuedAt:    1000,
		LastRenewedAt:   1000,
		AutoRenew:       true,
	})
	if err != nil {
		t.Fatalf("create certificate record failed: %v", err)
	}
	if row.DisplayID != 1 {
		t.Fatalf("expected display id allocated as 1, got=%d", row.DisplayID)
	}
	if row.ListOrderAt <= 0 {
		t.Fatalf("expected positive listOrderAt, got=%d", row.ListOrderAt)
	}

	if err := database.GetDB().Model(&model.CertificateRecord{}).Where("id = ?", row.Id).Updates(map[string]interface{}{
		"display_id":    0,
		"list_order_at": 0,
	}).Error; err != nil {
		t.Fatalf("reset display id/list order failed: %v", err)
	}

	if err := certificateInventory.RepairDisplayIDs(); err != nil {
		t.Fatalf("RepairDisplayIDs failed: %v", err)
	}

	if err := database.GetDB().Where("id = ?", row.Id).First(row).Error; err != nil {
		t.Fatalf("reload repaired row failed: %v", err)
	}
	if row.DisplayID != 1 {
		t.Fatalf("expected display id repaired again to 1, got=%d", row.DisplayID)
	}
	if row.ListOrderAt <= 0 {
		t.Fatalf("expected listOrderAt repaired again, got=%d", row.ListOrderAt)
	}
}

func TestDeleteRemovesTrackedPushedFilesForNonAcmeRecord(t *testing.T) {
	setupAcmeIPBehaviorTestDB(t, "acme-delete-nonacme-keep-pushdir.db")

	pushDir := filepath.Join(t.TempDir(), "push-self")
	if err := os.MkdirAll(pushDir, 0o755); err != nil {
		t.Fatalf("mkdir push dir failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pushDir, "cert.pem"), []byte("cert"), 0o644); err != nil {
		t.Fatalf("write cert.pem failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pushDir, "key.pem"), []byte("key"), 0o600); err != nil {
		t.Fatalf("write key.pem failed: %v", err)
	}

	record, err := certificateInventory.Upsert(CertificateUpsertPayload{
		SourceType:        CertificateSourceSelfSigned,
		SourceRef:         "self-delete-1",
		MainDomain:        "self.example.com",
		Domains:           []string{"self.example.com"},
		PushStateProvided: true,
		PushEnabled:       true,
		PushDir:           pushDir,
		PushFilePaths:     encodeCertificatePushFilePaths(map[string]string{"cert.pem": filepath.Join(pushDir, "cert.pem"), "key.pem": filepath.Join(pushDir, "key.pem")}),
		CertPEM:           []byte("cert"),
		KeyPEM:            []byte("key"),
		LastIssuedAt:      100,
	})
	if err != nil {
		t.Fatalf("upsert self-signed record failed: %v", err)
	}

	svc := &AcmeService{}
	if _, err := svc.Delete(AcmeDeletePayload{ID: record.Id}); err != nil {
		t.Fatalf("delete self-signed record failed: %v", err)
	}

	if _, statErr := os.Stat(filepath.Join(pushDir, "cert.pem")); !os.IsNotExist(statErr) {
		t.Fatalf("expected pushed cert.pem deleted, stat err=%v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(pushDir, "key.pem")); !os.IsNotExist(statErr) {
		t.Fatalf("expected pushed key.pem deleted, stat err=%v", statErr)
	}
	if err := database.GetDB().Where("id = ?", record.Id).First(&model.CertificateRecord{}).Error; !database.IsNotFound(err) {
		t.Fatalf("expected inventory record deleted, got err=%v", err)
	}
}

func TestDeleteRemovesTrackedPushedFilesAndRemovesOrphanAcmeInventoryRecord(t *testing.T) {
	setupAcmeIPBehaviorTestDB(t, "acme-delete-orphan-keep-pushdir.db")

	pushDir := filepath.Join(t.TempDir(), "push-acme")
	if err := os.MkdirAll(pushDir, 0o755); err != nil {
		t.Fatalf("mkdir push dir failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pushDir, "fullchain.pem"), []byte("fullchain"), 0o644); err != nil {
		t.Fatalf("write fullchain.pem failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(pushDir, "key.pem"), []byte("key"), 0o600); err != nil {
		t.Fatalf("write key.pem failed: %v", err)
	}

	record, err := certificateInventory.Upsert(CertificateUpsertPayload{
		SourceType:        CertificateSourceACME,
		SourceRef:         "999",
		MainDomain:        "orphan.example.com",
		Domains:           []string{"orphan.example.com"},
		Challenge:         "standalone",
		KeyLength:         "ec-256",
		CAServer:          acmeLEProductionDirectory,
		PushStateProvided: true,
		PushEnabled:       true,
		PushDir:           pushDir,
		PushFilePaths:     encodeCertificatePushFilePaths(map[string]string{"fullchain.pem": filepath.Join(pushDir, "fullchain.pem"), "key.pem": filepath.Join(pushDir, "key.pem")}),
		CertPEM:           []byte("cert"),
		KeyPEM:            []byte("key"),
		LastIssuedAt:      100,
	})
	if err != nil {
		t.Fatalf("upsert orphan acme inventory failed: %v", err)
	}

	svc := &AcmeService{}
	if _, err := svc.Delete(AcmeDeletePayload{ID: record.Id}); err != nil {
		t.Fatalf("delete orphan acme inventory failed: %v", err)
	}

	if _, statErr := os.Stat(filepath.Join(pushDir, "fullchain.pem")); !os.IsNotExist(statErr) {
		t.Fatalf("expected pushed fullchain.pem deleted, stat err=%v", statErr)
	}
	if _, statErr := os.Stat(filepath.Join(pushDir, "key.pem")); !os.IsNotExist(statErr) {
		t.Fatalf("expected pushed key.pem deleted, stat err=%v", statErr)
	}
	if err := database.GetDB().Where("id = ?", record.Id).First(&model.CertificateRecord{}).Error; !database.IsNotFound(err) {
		t.Fatalf("expected orphan inventory record deleted, got err=%v", err)
	}
}

func TestRemoveTrackedCertificateFilesFromDirectoryOnlyTouchesTrackedFiles(t *testing.T) {
	targetDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(targetDir, "cert.pem"), []byte("cert"), 0o644); err != nil {
		t.Fatalf("write cert.pem failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(targetDir, "key.pem"), []byte("key"), 0o600); err != nil {
		t.Fatalf("write key.pem failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(targetDir, "fullchain.pem"), []byte("fullchain"), 0o644); err != nil {
		t.Fatalf("write fullchain.pem failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(targetDir, "chain.pem"), []byte("chain"), 0o644); err != nil {
		t.Fatalf("write chain.pem failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(targetDir, "keep.txt"), []byte("keep"), 0o644); err != nil {
		t.Fatalf("write keep.txt failed: %v", err)
	}

	if err := removeTrackedCertificateFilesFromDirectory(targetDir, []string{"cert.pem", "key.pem", "fullchain.pem"}); err != nil {
		t.Fatalf("remove tracked files failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(targetDir, "cert.pem")); !os.IsNotExist(err) {
		t.Fatalf("expected cert.pem removed, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(targetDir, "key.pem")); !os.IsNotExist(err) {
		t.Fatalf("expected key.pem removed, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(targetDir, "fullchain.pem")); !os.IsNotExist(err) {
		t.Fatalf("expected fullchain.pem removed, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(targetDir, "chain.pem")); err != nil {
		t.Fatalf("expected chain.pem kept, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(targetDir, "keep.txt")); err != nil {
		t.Fatalf("expected keep.txt kept, err=%v", err)
	}
}

func TestOverviewMaintenanceKeepsACMEInventoryRecordWithoutMirror(t *testing.T) {
	setupAcmeIPBehaviorTestDB(t, "acme-sync-remove-orphan.db")

	record, err := certificateInventory.Upsert(CertificateUpsertPayload{
		SourceType:   CertificateSourceACME,
		SourceRef:    "inventory-only",
		MainDomain:   "inventory-only.example.com",
		Domains:      []string{"inventory-only.example.com"},
		Challenge:    "standalone",
		KeyLength:    "ec-256",
		CAServer:     acmeLEProductionDirectory,
		CertPEM:      []byte("cert-orphan"),
		KeyPEM:       []byte("key-orphan"),
		LastIssuedAt: 100,
	})
	if err != nil {
		t.Fatalf("create certificate record failed: %v", err)
	}

	svc := &AcmeService{}
	if err := svc.EnsureOverviewRuntimeConsistency(true); err != nil {
		t.Fatalf("EnsureOverviewRuntimeConsistency failed: %v", err)
	}

	if err := database.GetDB().Where("id = ?", record.Id).First(&model.CertificateRecord{}).Error; err != nil {
		t.Fatalf("ACME certificate record should not require a mirror row: %v", err)
	}
}

func generateManagedBundleForTest(t *testing.T, domain string, now time.Time) ([]byte, []byte, []byte, []byte, error) {
	t.Helper()
	keyPEM, fullchainPEM, err := (&ServerService{}).generateCertWithAlgorithm(
		domain,
		"ecc256",
		"ecc384",
		tlsCertificateUsageServer,
		now.Add(-time.Hour),
		now.Add(24*time.Hour),
	)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	certPEM, chainPEM := splitLeafAndChainPEM(fullchainPEM)
	return certPEM, keyPEM, fullchainPEM, chainPEM, nil
}

func writeManagedBundleForTest(t *testing.T, name string, certPEM []byte, keyPEM []byte, fullchainPEM []byte, chainPEM []byte) *acmeManagedCertPaths {
	t.Helper()
	dir := filepath.Join(t.TempDir(), name)
	return mustWriteManagedBundle(t, dir, certPEM, keyPEM, fullchainPEM, chainPEM)
}

func mustWriteManagedBundle(t *testing.T, dir string, certPEM []byte, keyPEM []byte, fullchainPEM []byte, chainPEM []byte) *acmeManagedCertPaths {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir bundle dir failed: %v", err)
	}
	paths := &acmeManagedCertPaths{
		CertPath:      filepath.Join(dir, "cert.pem"),
		KeyPath:       filepath.Join(dir, "key.pem"),
		FullchainPath: filepath.Join(dir, "fullchain.pem"),
		ChainPath:     filepath.Join(dir, "chain.pem"),
	}
	if err := os.WriteFile(paths.CertPath, certPEM, 0o644); err != nil {
		t.Fatalf("write cert.pem failed: %v", err)
	}
	if err := os.WriteFile(paths.KeyPath, keyPEM, 0o600); err != nil {
		t.Fatalf("write key.pem failed: %v", err)
	}
	if err := os.WriteFile(paths.FullchainPath, fullchainPEM, 0o644); err != nil {
		t.Fatalf("write fullchain.pem failed: %v", err)
	}
	if err := os.WriteFile(paths.ChainPath, chainPEM, 0o644); err != nil {
		t.Fatalf("write chain.pem failed: %v", err)
	}
	return paths
}

func setupAcmeIPBehaviorTestDB(t *testing.T, dbName string) {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), dbName)
	if err := database.InitDB(dbPath); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}
	sqlDB, err := database.GetDB().DB()
	if err != nil {
		t.Fatalf("db.DB failed: %v", err)
	}
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})
}

func assertArgIncluded(t *testing.T, args []string, expected string) {
	t.Helper()
	for _, arg := range args {
		if arg == expected {
			return
		}
	}
	t.Fatalf("expected arg %q in %#v", expected, args)
}

func assertArgNotIncluded(t *testing.T, args []string, unexpected string) {
	t.Helper()
	for _, arg := range args {
		if arg == unexpected {
			t.Fatalf("did not expect arg %q in %#v", unexpected, args)
		}
	}
}
