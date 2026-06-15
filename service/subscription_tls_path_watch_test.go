package service

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/alireza0/s-ui/database/model"
)

func resetSubscriptionTLSPathWatchStateForTest() {
	subscriptionTLSPathWatchMu.Lock()
	defer subscriptionTLSPathWatchMu.Unlock()
	subscriptionTLSPathWatchInitialized = false
	subscriptionTLSPathWatchLastDigest = ""
	subscriptionTLSPathWatchLastEntries = nil
}

func TestBuildSubscriptionTLSPathDigest_ChangesWhenCertificateFileContentChanges(t *testing.T) {
	db := setupMihomoSyncTestDB(t, "tls-path-watch-change.db")

	certPath := filepath.Join(t.TempDir(), "server.pem")
	if err := os.WriteFile(certPath, []byte("CERT-A\n"), 0o644); err != nil {
		t.Fatalf("write cert A failed: %v", err)
	}

	record := &model.Tls{
		Name: "watch-default",
		Server: mustJSONRaw(t, map[string]interface{}{
			"certificate_path": certPath,
		}),
		Client: mustJSONRaw(t, map[string]interface{}{}),
	}
	if err := db.Create(record).Error; err != nil {
		t.Fatalf("create tls record failed: %v", err)
	}

	digestA, filesA, err := buildSubscriptionTLSPathDigest()
	if err != nil {
		t.Fatalf("build digest A failed: %v", err)
	}
	if filesA == 0 || digestA == "" {
		t.Fatalf("expected non-empty digest and watched file count, got digest=%q files=%d", digestA, filesA)
	}

	if err := os.WriteFile(certPath, []byte("CERT-B\n"), 0o644); err != nil {
		t.Fatalf("write cert B failed: %v", err)
	}

	digestB, filesB, err := buildSubscriptionTLSPathDigest()
	if err != nil {
		t.Fatalf("build digest B failed: %v", err)
	}
	if filesB != filesA {
		t.Fatalf("expected watched file count unchanged, got %d vs %d", filesB, filesA)
	}
	if digestB == digestA {
		t.Fatalf("expected digest to change after certificate content update")
	}
}

func TestCheckAndSyncAutoManagedSubscriptionsOnTLSPathChange_DetectsAndCachesDigest(t *testing.T) {
	resetSubscriptionTLSPathWatchStateForTest()
	t.Cleanup(resetSubscriptionTLSPathWatchStateForTest)

	db := setupMihomoSyncTestDB(t, "tls-path-watch-sync.db")

	certPath := filepath.Join(t.TempDir(), "server.pem")
	if err := os.WriteFile(certPath, []byte("SYNC-A\n"), 0o644); err != nil {
		t.Fatalf("write cert A failed: %v", err)
	}

	record := &model.Tls{
		Name: "watch-sync",
		Server: mustJSONRaw(t, map[string]interface{}{
			"certificate_path": certPath,
		}),
		Client: mustJSONRaw(t, map[string]interface{}{}),
	}
	if err := db.Create(record).Error; err != nil {
		t.Fatalf("create tls record failed: %v", err)
	}

	changed, err := CheckAndSyncAutoManagedSubscriptionsOnTLSPathChange("")
	if err != nil {
		t.Fatalf("first check failed: %v", err)
	}
	if !changed {
		t.Fatalf("expected first check to be treated as changed baseline")
	}

	changed, err = CheckAndSyncAutoManagedSubscriptionsOnTLSPathChange("")
	if err != nil {
		t.Fatalf("second check failed: %v", err)
	}
	if changed {
		t.Fatalf("expected second check to be unchanged")
	}

	if err := os.WriteFile(certPath, []byte("SYNC-B\n"), 0o644); err != nil {
		t.Fatalf("write cert B failed: %v", err)
	}

	changed, err = CheckAndSyncAutoManagedSubscriptionsOnTLSPathChange("")
	if err != nil {
		t.Fatalf("third check failed: %v", err)
	}
	if !changed {
		t.Fatalf("expected third check to detect certificate content change")
	}
}

func TestBuildSubscriptionTLSPathDigest_IncludesMihomoTLSPaths(t *testing.T) {
	db := setupMihomoSyncTestDB(t, "tls-path-watch-mihomo.db")

	certPath := filepath.Join(t.TempDir(), "mihomo-server.pem")
	if err := os.WriteFile(certPath, []byte("MIHOMO-CERT\n"), 0o644); err != nil {
		t.Fatalf("write mihomo cert failed: %v", err)
	}

	record := &model.MihomoTls{
		Name: "watch-mihomo",
		Server: mustJSONRaw(t, map[string]interface{}{
			"certificate_path": certPath,
		}),
		Client: mustJSONRaw(t, map[string]interface{}{}),
	}
	if err := db.Create(record).Error; err != nil {
		t.Fatalf("create mihomo tls record failed: %v", err)
	}

	digest, files, err := buildSubscriptionTLSPathDigest()
	if err != nil {
		t.Fatalf("build digest failed: %v", err)
	}
	if digest == "" || files == 0 {
		t.Fatalf("expected mihomo tls path to be included, got digest=%q files=%d", digest, files)
	}
}

func TestTLSPathChangeForceRebuildsManagedDefaultSubOutbound(t *testing.T) {
	resetSubscriptionTLSPathWatchStateForTest()
	t.Cleanup(resetSubscriptionTLSPathWatchStateForTest)

	db := setupMihomoSyncTestDB(t, "tls-path-force-default.db")
	oldKeyPEM, oldFullchainPEM, err := (&ServerService{}).generateCertWithAlgorithm(
		"path-force-old.example.com",
		"ecc256",
		"ecc256",
		tlsCertificateUsageServer,
		time.Now().Add(-time.Hour),
		time.Now().Add(24*time.Hour),
	)
	if err != nil {
		t.Fatalf("generate old certificate failed: %v", err)
	}
	newKeyPEM, newFullchainPEM, err := (&ServerService{}).generateCertWithAlgorithm(
		"path-force-new.example.com",
		"ecc256",
		"ecc256",
		tlsCertificateUsageServer,
		time.Now().Add(-time.Hour),
		time.Now().Add(48*time.Hour),
	)
	if err != nil {
		t.Fatalf("generate new certificate failed: %v", err)
	}

	dir := t.TempDir()
	fullchainPath := filepath.Join(dir, "fullchain.pem")
	keyPath := filepath.Join(dir, "key.pem")
	if err := os.WriteFile(fullchainPath, oldFullchainPEM, 0o644); err != nil {
		t.Fatalf("write old fullchain failed: %v", err)
	}
	if err := os.WriteFile(keyPath, oldKeyPEM, 0o600); err != nil {
		t.Fatalf("write old key failed: %v", err)
	}

	tlsRow := &model.Tls{
		Name: "external-path-tls",
		Server: mustJSONRawIndent(t, map[string]interface{}{
			"certificate_path": fullchainPath,
			"key_path":         keyPath,
		}),
		Client: mustJSONRawIndent(t, map[string]interface{}{
			"include_server_certificate": true,
		}),
	}
	if err := db.Create(tlsRow).Error; err != nil {
		t.Fatalf("create tls row failed: %v", err)
	}

	changed, err := CheckAndSyncAutoManagedSubscriptionsOnTLSPathChange("")
	if err != nil {
		t.Fatalf("baseline path check failed: %v", err)
	}
	if !changed {
		t.Fatalf("expected baseline path check to initialize as changed")
	}

	inbound := &model.Inbound{
		Type:    "trojan",
		Tag:     "path-force-trojan",
		TlsId:   tlsRow.Id,
		OutJson: mustJSONRaw(t, map[string]interface{}{}),
		Options: mustJSONRaw(t, map[string]interface{}{
			"listen_port": 443,
		}),
	}
	if err := db.Create(inbound).Error; err != nil {
		t.Fatalf("create inbound failed: %v", err)
	}

	client := &model.Client{
		Enable:   true,
		Name:     "path-force-user",
		Config:   mustJSONRaw(t, map[string]interface{}{"trojan": map[string]interface{}{"password": "secret"}}),
		Inbounds: mustJSONRaw(t, []uint{inbound.Id}),
	}
	if err := db.Create(client).Error; err != nil {
		t.Fatalf("create client failed: %v", err)
	}
	if err := (&SettingService{}).SaveSubManagerAutoSyncClientIDs([]uint{client.Id}); err != nil {
		t.Fatalf("save auto sync ids failed: %v", err)
	}
	if err := blockSubSyncInbound(db, subOutboundSourceClient, client.Id, inbound.Id); err != nil {
		t.Fatalf("block sub sync inbound failed: %v", err)
	}

	if err := os.WriteFile(fullchainPath, newFullchainPEM, 0o644); err != nil {
		t.Fatalf("write new fullchain failed: %v", err)
	}
	if err := os.WriteFile(keyPath, newKeyPEM, 0o600); err != nil {
		t.Fatalf("write new key failed: %v", err)
	}

	changed, err = CheckAndSyncAutoManagedSubscriptionsOnTLSPathChange("")
	if err != nil {
		t.Fatalf("changed path check failed: %v", err)
	}
	if !changed {
		t.Fatalf("expected certificate path change to trigger sync")
	}

	var blockCount int64
	if err := db.Model(model.SubSyncBlock{}).
		Where("source_type = ? AND source_client_id = ? AND source_inbound_id = ?", subOutboundSourceClient, client.Id, inbound.Id).
		Count(&blockCount).Error; err != nil {
		t.Fatalf("count sync blocks failed: %v", err)
	}
	if blockCount != 0 {
		t.Fatalf("expected path force sync to clear manual delete block, count=%d", blockCount)
	}

	subTag := buildManagedClientSubTag(inbound.Tag, client.Name)
	saved := &model.SubOutbound{}
	if err := db.Where("tag = ?", subTag).First(saved).Error; err != nil {
		t.Fatalf("load synced suboutbound failed: %v", err)
	}
	if saved.SourceType != subOutboundSourceClient || saved.SourceClientId != client.Id || saved.SourceInboundId != inbound.Id {
		t.Fatalf("unexpected sync source metadata: %#v", saved)
	}
	assertSubOutboundCertificatePEM(t, saved, newFullchainPEM)
}
