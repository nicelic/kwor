package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alireza0/s-ui/config"
	"github.com/alireza0/s-ui/database"
)

func TestActivateManagedAcmeInstallKeepsOldOnIncompleteStage(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "acme-install-incomplete.db")
	if err := database.InitDB(dbPath); err != nil {
		t.Fatalf("init db failed: %v", err)
	}
	if sqlDB, err := database.GetDB().DB(); err == nil && sqlDB != nil {
		t.Cleanup(func() {
			_ = sqlDB.Close()
		})
	}

	root := managedAcmeHomeDir()
	_ = os.RemoveAll(root)
	t.Cleanup(func() {
		_ = os.RemoveAll(filepath.Join(config.GetDataDir(), "acme"))
	})

	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir managed home failed: %v", err)
	}
	oldScriptPath := filepath.Join(root, "acme.sh")
	if err := os.WriteFile(oldScriptPath, []byte("old-script"), 0o644); err != nil {
		t.Fatalf("write old script failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "account.conf"), []byte("ACCOUNT_EMAIL='old@example.com'\n"), 0o644); err != nil {
		t.Fatalf("write old account.conf failed: %v", err)
	}

	stageDir, cleanupStage, err := createManagedAcmeInstallWorkspace("acme-stage-test-*")
	if err != nil {
		t.Fatalf("create staged home failed: %v", err)
	}
	defer cleanupStage()
	if err := os.WriteFile(filepath.Join(stageDir, "account.conf"), []byte("ACCOUNT_EMAIL='new@example.com'\n"), 0o644); err != nil {
		t.Fatalf("write staged account.conf failed: %v", err)
	}

	svc := &AcmeService{}
	_, err = svc.activateManagedAcmeInstallLocked(stageDir)
	if err == nil {
		t.Fatal("expected activateManagedAcmeInstallLocked to fail when staged script is missing")
	}
	if !strings.Contains(err.Error(), "staged acme.sh script was not found") {
		t.Fatalf("unexpected error: %v", err)
	}

	gotScript, readErr := os.ReadFile(oldScriptPath)
	if readErr != nil {
		t.Fatalf("read old script after failed activation failed: %v", readErr)
	}
	if string(gotScript) != "old-script" {
		t.Fatalf("old script changed after failed activation: %q", string(gotScript))
	}
	gotAccount, readErr := os.ReadFile(filepath.Join(root, "account.conf"))
	if readErr != nil {
		t.Fatalf("read old account.conf after failed activation failed: %v", readErr)
	}
	if !strings.Contains(string(gotAccount), "old@example.com") {
		t.Fatalf("old account.conf changed after failed activation: %q", string(gotAccount))
	}
}

func TestActivateManagedAcmeInstallReplacesOnlyManagedArtifacts(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "acme-install-activate.db")
	if err := database.InitDB(dbPath); err != nil {
		t.Fatalf("init db failed: %v", err)
	}
	if sqlDB, err := database.GetDB().DB(); err == nil && sqlDB != nil {
		t.Cleanup(func() {
			_ = sqlDB.Close()
		})
	}

	acmeRoot := filepath.Join(config.GetDataDir(), "acme")
	root := managedAcmeHomeDir()
	_ = os.RemoveAll(acmeRoot)
	t.Cleanup(func() {
		_ = os.RemoveAll(acmeRoot)
	})

	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir managed home failed: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "dnsapi"), 0o755); err != nil {
		t.Fatalf("mkdir old dnsapi failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "acme.sh"), []byte("old-script"), 0o644); err != nil {
		t.Fatalf("write old script failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "account.conf"), []byte("ACCOUNT_EMAIL='old@example.com'\n"), 0o644); err != nil {
		t.Fatalf("write old account.conf failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "dnsapi", "old.sh"), []byte("old-dns"), 0o644); err != nil {
		t.Fatalf("write old dnsapi file failed: %v", err)
	}
	liveDir := filepath.Join(acmeRoot, "live", "cert-a")
	if err := os.MkdirAll(liveDir, 0o755); err != nil {
		t.Fatalf("mkdir live dir failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(liveDir, "fullchain.pem"), []byte("live-cert"), 0o644); err != nil {
		t.Fatalf("write live cert failed: %v", err)
	}

	stageDir, cleanupStage, err := createManagedAcmeInstallWorkspace("acme-stage-test-*")
	if err != nil {
		t.Fatalf("create staged home failed: %v", err)
	}
	defer cleanupStage()
	if err := os.WriteFile(filepath.Join(stageDir, "acme.sh"), []byte("new-script"), 0o755); err != nil {
		t.Fatalf("write staged script failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stageDir, "account.conf"), []byte("ACCOUNT_EMAIL='new@example.com'\n"), 0o644); err != nil {
		t.Fatalf("write staged account.conf failed: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(stageDir, "dnsapi"), 0o755); err != nil {
		t.Fatalf("mkdir staged dnsapi failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stageDir, "dnsapi", "new.sh"), []byte("new-dns"), 0o644); err != nil {
		t.Fatalf("write staged dnsapi file failed: %v", err)
	}

	svc := &AcmeService{}
	scriptPath, err := svc.activateManagedAcmeInstallLocked(stageDir)
	if err != nil {
		t.Fatalf("activateManagedAcmeInstallLocked failed: %v", err)
	}
	if filepath.Clean(scriptPath) != filepath.Clean(filepath.Join(root, "acme.sh")) {
		t.Fatalf("unexpected script path: got=%q", scriptPath)
	}
	if err := svc.persistManagedAcmeManifestLocked(root); err != nil {
		t.Fatalf("persistManagedAcmeManifestLocked failed: %v", err)
	}

	gotScript, err := os.ReadFile(filepath.Join(root, "acme.sh"))
	if err != nil {
		t.Fatalf("read activated script failed: %v", err)
	}
	if string(gotScript) != "new-script" {
		t.Fatalf("activated script content = %q, want new-script", string(gotScript))
	}
	gotAccount, err := os.ReadFile(filepath.Join(root, "account.conf"))
	if err != nil {
		t.Fatalf("read activated account.conf failed: %v", err)
	}
	if !strings.Contains(string(gotAccount), "new@example.com") {
		t.Fatalf("activated account.conf = %q", string(gotAccount))
	}
	if pathExists(filepath.Join(root, "dnsapi", "old.sh")) {
		t.Fatalf("old dnsapi artifact still exists after activation")
	}
	if !pathExists(filepath.Join(root, "dnsapi", "new.sh")) {
		t.Fatalf("new dnsapi artifact missing after activation")
	}
	liveCert, err := os.ReadFile(filepath.Join(liveDir, "fullchain.pem"))
	if err != nil {
		t.Fatalf("read live cert failed: %v", err)
	}
	if string(liveCert) != "live-cert" {
		t.Fatalf("live cert changed unexpectedly: %q", string(liveCert))
	}

	manifestRaw := strings.TrimSpace(svc.readSettingWithDefault(acmeManagedPathManifestKey, ""))
	if manifestRaw == "" {
		t.Fatal("expected managed manifest to be saved")
	}

	manifestPaths := []string{}
	if err := json.Unmarshal([]byte(manifestRaw), &manifestPaths); err != nil {
		t.Fatalf("unmarshal managed manifest failed: %v", err)
	}
	for _, item := range manifestPaths {
		cleaned := filepath.Clean(strings.TrimSpace(item))
		if cleaned == filepath.Clean(liveDir) || strings.HasPrefix(filepath.ToSlash(cleaned), filepath.ToSlash(liveDir)+"/") {
			t.Fatalf("managed manifest should not include live directory: %#v", manifestPaths)
		}
		if cleaned == filepath.Clean(filepath.Join(root, "dnsapi")) {
			t.Fatalf("managed manifest should track dnsapi files precisely instead of storing only the directory: %#v", manifestPaths)
		}
		if strings.HasSuffix(cleaned, string(filepath.Separator)+"old.sh") || filepath.Base(cleaned) == "old.sh" {
			t.Fatalf("managed manifest should not include old dnsapi artifact: %#v", manifestPaths)
		}
	}
	if len(manifestPaths) == 0 {
		t.Fatalf("managed manifest should contain install artifacts")
	}
	if !strings.Contains(manifestRaw, "acme.sh") || !strings.Contains(manifestRaw, "dnsapi") {
		t.Fatalf("managed manifest missing expected install artifacts: %s", manifestRaw)
	}
}

func TestRemoveManagedAcmeSkipsLegacyLivePathsInManifest(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "acme-remove-legacy-manifest.db")
	if err := database.InitDB(dbPath); err != nil {
		t.Fatalf("init db failed: %v", err)
	}
	if sqlDB, err := database.GetDB().DB(); err == nil && sqlDB != nil {
		t.Cleanup(func() {
			_ = sqlDB.Close()
		})
	}

	acmeRoot := filepath.Join(config.GetDataDir(), "acme")
	root := managedAcmeHomeDir()
	_ = os.RemoveAll(acmeRoot)
	t.Cleanup(func() {
		_ = os.RemoveAll(acmeRoot)
	})

	if err := os.MkdirAll(filepath.Join(root, "dnsapi"), 0o755); err != nil {
		t.Fatalf("mkdir dnsapi failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "acme.sh"), []byte("managed-script"), 0o644); err != nil {
		t.Fatalf("write managed script failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "dnsapi", "dns_cf.sh"), []byte("dns"), 0o644); err != nil {
		t.Fatalf("write dnsapi file failed: %v", err)
	}
	liveDir := filepath.Join(acmeRoot, "live", "cert-a")
	if err := os.MkdirAll(liveDir, 0o755); err != nil {
		t.Fatalf("mkdir live dir failed: %v", err)
	}
	liveCertPath := filepath.Join(liveDir, "kept.txt")
	if err := os.WriteFile(liveCertPath, []byte("live-cert"), 0o644); err != nil {
		t.Fatalf("write live marker failed: %v", err)
	}

	manifestPaths := []string{
		filepath.Join(root, "acme.sh"),
		filepath.Join(root, "dnsapi", "dns_cf.sh"),
		liveDir,
		liveCertPath,
	}
	rawManifest, err := json.Marshal(manifestPaths)
	if err != nil {
		t.Fatalf("marshal manifest failed: %v", err)
	}

	svc := &AcmeService{}
	if err := svc.setString(acmeManagedPathManifestKey, string(rawManifest)); err != nil {
		t.Fatalf("save manifest failed: %v", err)
	}
	if err := svc.setString(acmeScriptPathKey, filepath.Join(root, "acme.sh")); err != nil {
		t.Fatalf("save script path failed: %v", err)
	}

	if _, err := svc.removeManagedAcmeWithOptionsLocked(acmeRemoveOptions{}); err != nil {
		t.Fatalf("removeManagedAcmeWithOptionsLocked failed: %v", err)
	}

	if pathExists(filepath.Join(root, "acme.sh")) {
		t.Fatal("managed acme.sh still exists after removal")
	}
	if pathExists(filepath.Join(root, "dnsapi", "dns_cf.sh")) {
		t.Fatal("managed dnsapi artifact still exists after removal")
	}
	if !pathExists(liveDir) {
		t.Fatalf("live directory should not be removed by managed acme cleanup: %q", liveDir)
	}
	liveMarker, err := os.ReadFile(liveCertPath)
	if err != nil {
		t.Fatalf("read live marker after managed removal failed: %v", err)
	}
	if string(liveMarker) != "live-cert" {
		t.Fatalf("live marker changed unexpectedly: %q", string(liveMarker))
	}
}

func TestRemoveManagedAcmePreservesCustomManagedDirFiles(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "acme-remove-custom-managed-dir.db")
	if err := database.InitDB(dbPath); err != nil {
		t.Fatalf("init db failed: %v", err)
	}
	if sqlDB, err := database.GetDB().DB(); err == nil && sqlDB != nil {
		t.Cleanup(func() {
			_ = sqlDB.Close()
		})
	}

	acmeRoot := filepath.Join(config.GetDataDir(), "acme")
	root := managedAcmeHomeDir()
	_ = os.RemoveAll(acmeRoot)
	t.Cleanup(func() {
		_ = os.RemoveAll(acmeRoot)
	})

	if err := os.MkdirAll(filepath.Join(root, "dnsapi"), 0o755); err != nil {
		t.Fatalf("mkdir dnsapi failed: %v", err)
	}
	managedScriptPath := filepath.Join(root, "acme.sh")
	managedDNSPath := filepath.Join(root, "dnsapi", "dns_cf.sh")
	customDNSPath := filepath.Join(root, "dnsapi", "custom_provider.sh")
	if err := os.WriteFile(managedScriptPath, []byte("managed-script"), 0o644); err != nil {
		t.Fatalf("write managed script failed: %v", err)
	}
	if err := os.WriteFile(managedDNSPath, []byte("managed-dns"), 0o644); err != nil {
		t.Fatalf("write managed dnsapi file failed: %v", err)
	}
	if err := os.WriteFile(customDNSPath, []byte("custom-dns"), 0o644); err != nil {
		t.Fatalf("write custom dnsapi file failed: %v", err)
	}

	manifestPaths := []string{
		managedScriptPath,
		managedDNSPath,
	}
	rawManifest, err := json.Marshal(manifestPaths)
	if err != nil {
		t.Fatalf("marshal manifest failed: %v", err)
	}

	svc := &AcmeService{}
	if err := svc.setString(acmeManagedPathManifestKey, string(rawManifest)); err != nil {
		t.Fatalf("save manifest failed: %v", err)
	}
	if err := svc.setString(acmeScriptPathKey, managedScriptPath); err != nil {
		t.Fatalf("save managed script path failed: %v", err)
	}

	if _, err := svc.removeManagedAcmeWithOptionsLocked(acmeRemoveOptions{}); err != nil {
		t.Fatalf("removeManagedAcmeWithOptionsLocked failed: %v", err)
	}

	if pathExists(managedScriptPath) {
		t.Fatal("managed acme.sh should be removed")
	}
	if pathExists(managedDNSPath) {
		t.Fatal("managed dnsapi file should be removed")
	}
	if !pathExists(customDNSPath) {
		t.Fatal("custom dnsapi file should be preserved")
	}
}

func TestRemoveManagedAcmePreservesExternalScriptSetting(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "acme-remove-external-script.db")
	if err := database.InitDB(dbPath); err != nil {
		t.Fatalf("init db failed: %v", err)
	}
	if sqlDB, err := database.GetDB().DB(); err == nil && sqlDB != nil {
		t.Cleanup(func() {
			_ = sqlDB.Close()
		})
	}

	acmeRoot := filepath.Join(config.GetDataDir(), "acme")
	root := managedAcmeHomeDir()
	_ = os.RemoveAll(acmeRoot)
	t.Cleanup(func() {
		_ = os.RemoveAll(acmeRoot)
	})

	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("mkdir managed home failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "acme.sh"), []byte("managed-script"), 0o644); err != nil {
		t.Fatalf("write managed script failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "account.conf"), []byte("ACCOUNT_EMAIL='managed@example.com'\n"), 0o644); err != nil {
		t.Fatalf("write managed account failed: %v", err)
	}

	externalRoot := filepath.Join(t.TempDir(), "external-acme")
	if err := os.MkdirAll(externalRoot, 0o755); err != nil {
		t.Fatalf("mkdir external root failed: %v", err)
	}
	externalScript := filepath.Join(externalRoot, "acme.sh")
	if err := os.WriteFile(externalScript, []byte("external-script"), 0o644); err != nil {
		t.Fatalf("write external script failed: %v", err)
	}

	svc := &AcmeService{}
	if err := svc.persistManagedAcmeManifestLocked(root); err != nil {
		t.Fatalf("persist managed manifest failed: %v", err)
	}
	if err := svc.setString(acmeScriptPathKey, externalScript); err != nil {
		t.Fatalf("save external script path failed: %v", err)
	}

	if _, err := svc.removeManagedAcmeWithOptionsLocked(acmeRemoveOptions{}); err != nil {
		t.Fatalf("removeManagedAcmeWithOptionsLocked failed: %v", err)
	}

	saved := strings.TrimSpace(svc.readSettingWithDefault(acmeScriptPathKey, ""))
	if filepath.Clean(saved) != filepath.Clean(externalScript) {
		t.Fatalf("expected external script path preserved, got %q want %q", saved, externalScript)
	}
	if !pathExists(externalScript) {
		t.Fatal("external acme.sh should not be removed")
	}
	if pathExists(filepath.Join(root, "acme.sh")) {
		t.Fatal("managed acme.sh should be removed")
	}
}

func TestCleanupObsoleteLegacyManagedAcmeInstallRoot(t *testing.T) {
	acmeRoot := filepath.Join(config.GetDataDir(), "acme")
	currentRoot := managedAcmeHomeDir()
	legacyRoot := legacyManagedAcmeHomeDir()
	_ = os.RemoveAll(acmeRoot)
	t.Cleanup(func() {
		_ = os.RemoveAll(acmeRoot)
	})

	if err := os.MkdirAll(filepath.Join(currentRoot, "dnsapi"), 0o755); err != nil {
		t.Fatalf("mkdir current dnsapi failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(currentRoot, "acme.sh"), []byte("current-script"), 0o644); err != nil {
		t.Fatalf("write current script failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(currentRoot, "dnsapi", "current.sh"), []byte("current-dns"), 0o644); err != nil {
		t.Fatalf("write current dnsapi failed: %v", err)
	}

	if err := os.MkdirAll(filepath.Join(legacyRoot, "dnsapi"), 0o755); err != nil {
		t.Fatalf("mkdir legacy dnsapi failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacyRoot, "acme.sh"), []byte("legacy-script"), 0o644); err != nil {
		t.Fatalf("write legacy script failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(legacyRoot, "dnsapi", "legacy.sh"), []byte("legacy-dns"), 0o644); err != nil {
		t.Fatalf("write legacy dnsapi failed: %v", err)
	}

	if err := cleanupObsoleteLegacyManagedAcmeInstallRoot(); err != nil {
		t.Fatalf("cleanupObsoleteLegacyManagedAcmeInstallRoot failed: %v", err)
	}

	if pathExists(filepath.Join(legacyRoot, "acme.sh")) || pathExists(filepath.Join(legacyRoot, "dnsapi", "legacy.sh")) {
		t.Fatal("legacy managed acme install artifacts still exist after cleanup")
	}
	if pathExists(legacyRoot) {
		t.Fatalf("legacy managed root should be removed when empty: %q", legacyRoot)
	}
	currentScript, err := os.ReadFile(filepath.Join(currentRoot, "acme.sh"))
	if err != nil {
		t.Fatalf("read current script failed: %v", err)
	}
	if string(currentScript) != "current-script" {
		t.Fatalf("current script changed unexpectedly: %q", string(currentScript))
	}
	if !pathExists(filepath.Join(currentRoot, "dnsapi", "current.sh")) {
		t.Fatal("current managed dnsapi artifact should remain")
	}
}
