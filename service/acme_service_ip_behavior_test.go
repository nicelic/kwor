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

func TestCleanupManagedAcmeWorktreesRemovesLegacyWorktreeAndKeepsSupportFiles(t *testing.T) {
	root := filepath.Join(t.TempDir(), "acme")
	worktree := filepath.Join(root, "2400-f880-dbf-8a82--213_ecc")
	if err := os.MkdirAll(filepath.Join(worktree, "backup"), 0o755); err != nil {
		t.Fatalf("mkdir worktree failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(worktree, "cert.pem"), []byte("cert"), 0o644); err != nil {
		t.Fatalf("write cert.pem failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "account.conf"), []byte("ACCOUNT_EMAIL='a@example.com'"), 0o600); err != nil {
		t.Fatalf("write account.conf failed: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "ca"), 0o755); err != nil {
		t.Fatalf("mkdir ca failed: %v", err)
	}
	if err := cleanupManagedAcmeWorktrees(root); err != nil {
		t.Fatalf("cleanupManagedAcmeWorktrees failed: %v", err)
	}

	if _, err := os.Stat(worktree); !os.IsNotExist(err) {
		t.Fatalf("expected worktree removed, stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "account.conf")); err != nil {
		t.Fatalf("expected account.conf kept, stat err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "ca")); err != nil {
		t.Fatalf("expected ca directory kept, stat err=%v", err)
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

func TestApplyIssuePostActionsOnlyUsesExplicitPushDir(t *testing.T) {
	setupAcmeIPBehaviorTestDB(t, "acme-ip-behavior-pushdir.db")
	svc := &AcmeService{}

	entry := &model.AcmeCertificate{
		Id:              11,
		MainDomain:      "test.example.com",
		DomainSet:       `["test.example.com"]`,
		Challenge:       "standalone",
		KeyLength:       "ec-384",
		CAServer:        acmeLEProductionDirectory,
		CertificateType: acmeCertificateTypeDomain,
		CertPEM:         []byte("test-cert"),
		KeyPEM:          []byte("test-key"),
		FullchainPEM:    []byte("test-fullchain"),
		ChainPEM:        []byte("test-chain"),
	}
	if err := database.GetDB().Create(entry).Error; err != nil {
		t.Fatalf("create acme certificate failed: %v", err)
	}
	record, err := upsertInventoryFromAcme(entry)
	if err != nil {
		t.Fatalf("upsert inventory failed: %v", err)
	}

	targetDir := filepath.Join(t.TempDir(), "bundle")
	if err := svc.applyIssuePostActions(entry, "", targetDir, true); err != nil {
		t.Fatalf("applyIssuePostActions failed: %v", err)
	}

	if got := strings.TrimSpace(entry.PushDir); got != targetDir {
		t.Fatalf("expected entry push dir updated: got=%q want=%q", got, targetDir)
	}

	record, err = certificateInventory.GetRecordByID(record.Id)
	if err != nil {
		t.Fatalf("load inventory record failed: %v", err)
	}
	if got := strings.TrimSpace(record.PushDir); got != targetDir {
		t.Fatalf("expected inventory push dir updated: got=%q want=%q", got, targetDir)
	}
	if got := strings.TrimSpace(record.PushFiles); got == "" {
		t.Fatal("expected inventory push files to be recorded")
	}
}

func TestApplyIssuePostActionsSkipsStoredPushDirWhenPushNotExplicit(t *testing.T) {
	setupAcmeIPBehaviorTestDB(t, "acme-ip-behavior-pushdir-skip.db")
	svc := &AcmeService{}

	oldDir := filepath.Join(t.TempDir(), "old-push")
	if err := os.MkdirAll(oldDir, 0o755); err != nil {
		t.Fatalf("create old push dir failed: %v", err)
	}

	entry := &model.AcmeCertificate{
		Id:              12,
		MainDomain:      "test-skip.example.com",
		DomainSet:       `["test-skip.example.com"]`,
		Challenge:       "standalone",
		KeyLength:       "ec-384",
		CAServer:        acmeLEProductionDirectory,
		CertificateType: acmeCertificateTypeDomain,
		PushDir:         oldDir,
		CertPEM:         []byte("test-cert"),
		KeyPEM:          []byte("test-key"),
		FullchainPEM:    []byte("test-fullchain"),
		ChainPEM:        []byte("test-chain"),
	}
	if err := database.GetDB().Create(entry).Error; err != nil {
		t.Fatalf("create acme certificate failed: %v", err)
	}
	record, err := upsertInventoryFromAcme(entry)
	if err != nil {
		t.Fatalf("upsert inventory failed: %v", err)
	}
	record.PushDir = oldDir
	record.PushFiles = `["cert.pem","key.pem","fullchain.pem"]`
	if err := database.GetDB().Save(record).Error; err != nil {
		t.Fatalf("save inventory push state failed: %v", err)
	}

	if err := svc.applyIssuePostActions(entry, "", "", false); err != nil {
		t.Fatalf("applyIssuePostActions failed: %v", err)
	}

	if got := strings.TrimSpace(entry.PushDir); got != oldDir {
		t.Fatalf("expected entry push dir unchanged: got=%q want=%q", got, oldDir)
	}
	reloadedRecord, err := certificateInventory.GetRecordByID(record.Id)
	if err != nil {
		t.Fatalf("reload inventory record failed: %v", err)
	}
	if got := strings.TrimSpace(reloadedRecord.PushDir); got != oldDir {
		t.Fatalf("expected inventory push dir unchanged: got=%q want=%q", got, oldDir)
	}
	if _, statErr := os.Stat(filepath.Join(oldDir, "cert.pem")); !os.IsNotExist(statErr) {
		t.Fatalf("expected cert.pem not created when push is not explicit, stat err=%v", statErr)
	}
}

func TestPushSyncsAcmeSourceRecordAndPreservesTrackedFiles(t *testing.T) {
	setupAcmeIPBehaviorTestDB(t, "acme-push-sync-source.db")
	svc := &AcmeService{}

	sourceEntry := &model.AcmeCertificate{
		Id:              21,
		MainDomain:      "push.example.com",
		DomainSet:       `["push.example.com"]`,
		Challenge:       "standalone",
		KeyLength:       "ec-256",
		CAServer:        acmeLEProductionDirectory,
		CertificateType: acmeCertificateTypeDomain,
		PushDir:         "",
		PushFiles:       "",
		CertPEM:         []byte("cert-a"),
		KeyPEM:          []byte("key-a"),
		FullchainPEM:    []byte("fullchain-a"),
		ChainPEM:        nil,
	}
	if err := database.GetDB().Create(sourceEntry).Error; err != nil {
		t.Fatalf("create acme source entry failed: %v", err)
	}

	record, err := upsertInventoryFromAcme(sourceEntry)
	if err != nil {
		t.Fatalf("upsert inventory failed: %v", err)
	}

	oldDir := filepath.Join(t.TempDir(), "old-push")
	if _, err := replaceCertificateInDirectoryWithTrackedFiles(oldDir, nil, sourceEntry.CertPEM, sourceEntry.KeyPEM, sourceEntry.FullchainPEM, sourceEntry.ChainPEM); err != nil {
		t.Fatalf("seed old push dir failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(oldDir, "keep.txt"), []byte("keep"), 0o644); err != nil {
		t.Fatalf("write keep.txt failed: %v", err)
	}

	sourceEntry.PushDir = oldDir
	sourceEntry.PushFiles = `["cert.pem","fullchain.pem","key.pem"]`
	if err := database.GetDB().Save(sourceEntry).Error; err != nil {
		t.Fatalf("save acme source entry failed: %v", err)
	}
	record.PushDir = oldDir
	record.PushFiles = sourceEntry.PushFiles
	if err := database.GetDB().Save(record).Error; err != nil {
		t.Fatalf("save inventory record failed: %v", err)
	}

	newDir := filepath.Join(t.TempDir(), "new-push")
	result, err := svc.Push(AcmePushPayload{ID: record.Id, TargetDir: newDir})
	if err != nil {
		t.Fatalf("push failed: %v", err)
	}
	if result == nil || result.Certificate == nil {
		t.Fatal("expected push result certificate")
	}

	reloadedSource := &model.AcmeCertificate{}
	if err := database.GetDB().Where("id = ?", sourceEntry.Id).First(reloadedSource).Error; err != nil {
		t.Fatalf("reload source entry failed: %v", err)
	}
	if got := strings.TrimSpace(reloadedSource.PushDir); got != newDir {
		t.Fatalf("expected source push dir synced: got=%q want=%q", got, newDir)
	}
	if got := strings.TrimSpace(reloadedSource.PushFiles); got == "" {
		t.Fatal("expected source push files recorded")
	}

	reloadedRecord, err := certificateInventory.GetRecordByID(record.Id)
	if err != nil {
		t.Fatalf("reload inventory record failed: %v", err)
	}
	if got := strings.TrimSpace(reloadedRecord.PushDir); got != newDir {
		t.Fatalf("expected inventory push dir synced: got=%q want=%q", got, newDir)
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

func TestUpsertCertificateFromPathsCreatesNewRowsForSameIP(t *testing.T) {
	setupAcmeIPBehaviorTestDB(t, "acme-same-ip-new-rows.db")

	now := time.Now()
	certPEM, keyPEM, fullchainPEM, chainPEM, err := generateManagedBundleForTest(t, "149.104.4.229", now)
	if err != nil {
		t.Fatalf("generate bundle failed: %v", err)
	}
	paths1 := writeManagedBundleForTest(t, "149.104.4.229-a", certPEM, keyPEM, fullchainPEM, chainPEM)
	paths2 := writeManagedBundleForTest(t, "149.104.4.229-b", certPEM, keyPEM, fullchainPEM, chainPEM)

	svc := &AcmeService{}
	first, err := svc.upsertCertificateFromPaths(0, []string{"149.104.4.229"}, acmeCertificateTypeIP, acmeCertProfileForType(acmeCertificateTypeIP), "standalone", "ec-384", acmeLEProductionDirectory, true, "/tmp/acme-a", "", "", "", "", paths1, 111)
	if err != nil {
		t.Fatalf("upsert first failed: %v", err)
	}
	second, err := svc.upsertCertificateFromPaths(0, []string{"149.104.4.229"}, acmeCertificateTypeIP, acmeCertProfileForType(acmeCertificateTypeIP), "standalone", "ec-384", acmeLEProductionDirectory, true, "/tmp/acme-b", "", "", "", "", paths2, 222)
	if err != nil {
		t.Fatalf("upsert second failed: %v", err)
	}

	if first.Id == 0 || second.Id == 0 || first.Id == second.Id {
		t.Fatalf("expected distinct acme rows, got first=%d second=%d", first.Id, second.Id)
	}

	var count int64
	if err := database.GetDB().Model(&model.AcmeCertificate{}).Where("main_domain = ?", "149.104.4.229").Count(&count).Error; err != nil {
		t.Fatalf("count same ip rows failed: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected 2 acme rows for same ip, got=%d", count)
	}
}

func TestSyncInventoryFromAcmeDBRepairsMissingInventoryAndDisplayID(t *testing.T) {
	setupAcmeIPBehaviorTestDB(t, "acme-sync-repair.db")

	entry := &model.AcmeCertificate{
		MainDomain:      "repair.example.com",
		DomainSet:       `["repair.example.com"]`,
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
		AcmeAccountName: "default",
		DNSAccountName:  "",
		AutoRenew:       true,
	}
	if err := database.GetDB().Create(entry).Error; err != nil {
		t.Fatalf("create acme entry failed: %v", err)
	}

	svc := &AcmeService{}
	if err := svc.syncInventoryFromAcmeDB(); err != nil {
		t.Fatalf("syncInventoryFromAcmeDB failed: %v", err)
	}

	row := &model.CertificateRecord{}
	if err := database.GetDB().Where("source_type = ? AND source_ref = ?", CertificateSourceACME, "1").First(row).Error; err != nil {
		t.Fatalf("expected inventory row synced: %v", err)
	}
	if row.DisplayID != 1 {
		t.Fatalf("expected display id repaired to 1, got=%d", row.DisplayID)
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

	if err := svc.syncInventoryFromAcmeDB(); err != nil {
		t.Fatalf("second syncInventoryFromAcmeDB failed: %v", err)
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
		SourceType:   CertificateSourceSelfSigned,
		SourceRef:    "self-delete-1",
		MainDomain:   "self.example.com",
		Domains:      []string{"self.example.com"},
		PushDir:      pushDir,
		PushFiles:    `["cert.pem","key.pem"]`,
		CertPEM:      []byte("cert"),
		KeyPEM:       []byte("key"),
		LastIssuedAt: 100,
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
		SourceType:   CertificateSourceACME,
		SourceRef:    "999",
		MainDomain:   "orphan.example.com",
		Domains:      []string{"orphan.example.com"},
		Challenge:    "standalone",
		KeyLength:    "ec-256",
		CAServer:     acmeLEProductionDirectory,
		PushDir:      pushDir,
		PushFiles:    `["fullchain.pem","key.pem"]`,
		CertPEM:      []byte("cert"),
		KeyPEM:       []byte("key"),
		LastIssuedAt: 100,
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

func TestSyncInventoryFromAcmeDBRemovesOrphanAcmeInventoryRows(t *testing.T) {
	setupAcmeIPBehaviorTestDB(t, "acme-sync-remove-orphan.db")

	validEntry := &model.AcmeCertificate{
		MainDomain:      "valid.example.com",
		DomainSet:       `["valid.example.com"]`,
		CertificateType: acmeCertificateTypeDomain,
		Challenge:       "standalone",
		KeyLength:       "ec-256",
		CAServer:        acmeLEProductionDirectory,
		UseECC:          true,
		CertPEM:         []byte("cert-valid"),
		KeyPEM:          []byte("key-valid"),
		FullchainPEM:    []byte("fullchain-valid"),
		ChainPEM:        []byte("chain-valid"),
		Fingerprint:     "fp-valid",
		NotBefore:       1000,
		NotAfter:        2000,
		LastIssuedAt:    1000,
		LastRenewedAt:   1000,
	}
	if err := database.GetDB().Create(validEntry).Error; err != nil {
		t.Fatalf("create valid acme entry failed: %v", err)
	}

	orphan, err := certificateInventory.Upsert(CertificateUpsertPayload{
		SourceType:   CertificateSourceACME,
		SourceRef:    "999",
		MainDomain:   "orphan.example.com",
		Domains:      []string{"orphan.example.com"},
		Challenge:    "standalone",
		KeyLength:    "ec-256",
		CAServer:     acmeLEProductionDirectory,
		CertPEM:      []byte("cert-orphan"),
		KeyPEM:       []byte("key-orphan"),
		LastIssuedAt: 100,
	})
	if err != nil {
		t.Fatalf("create orphan inventory row failed: %v", err)
	}

	svc := &AcmeService{}
	if err := svc.syncInventoryFromAcmeDB(); err != nil {
		t.Fatalf("syncInventoryFromAcmeDB failed: %v", err)
	}

	if err := database.GetDB().Where("id = ?", orphan.Id).First(&model.CertificateRecord{}).Error; !database.IsNotFound(err) {
		t.Fatalf("expected orphan inventory removed, got err=%v", err)
	}

	validRow := &model.CertificateRecord{}
	if err := database.GetDB().Where("source_type = ? AND source_ref = ?", CertificateSourceACME, "1").First(validRow).Error; err != nil {
		t.Fatalf("expected valid inventory row kept: %v", err)
	}
	if strings.TrimSpace(validRow.MainDomain) != "valid.example.com" {
		t.Fatalf("unexpected valid inventory main domain: %q", validRow.MainDomain)
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
