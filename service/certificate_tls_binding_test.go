package service

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alireza0/s-ui/database/model"
)

func TestCertificateListBlocksDeleteWhenDefaultTLSUsesCertificate(t *testing.T) {
	db := setupMihomoSyncTestDB(t, "cert-default-tls-usage.db")
	record := upsertTestCertificateRecord(t, "default-tls.example.com")

	tlsRow := &model.Tls{
		Name:                "default-listener",
		CertificateRecordID: record.Id,
		Server:              mustJSONRaw(t, map[string]interface{}{}),
		Client:              mustJSONRaw(t, map[string]interface{}{}),
	}
	if err := db.Create(tlsRow).Error; err != nil {
		t.Fatalf("create tls row failed: %v", err)
	}

	views, err := certificateInventory.List()
	if err != nil {
		t.Fatalf("list certificates failed: %v", err)
	}
	view := findCertificateRecordView(t, views, record.Id)
	if !view.DeleteBlocked || !view.InUseByTLS {
		t.Fatalf("expected default TLS usage to block delete, got %#v", view)
	}
	if !strings.Contains(view.UsageLabel, "default-listener") {
		t.Fatalf("expected usage label to include tls name, got %q", view.UsageLabel)
	}

	if _, err := (&AcmeService{}).Delete(AcmeDeletePayload{ID: record.Id}); err == nil {
		t.Fatalf("expected delete to fail while certificate is used by default TLS")
	}
}

func TestCertificateListBlocksDeleteWhenMihomoTLSUsesCertificate(t *testing.T) {
	db := setupMihomoSyncTestDB(t, "cert-mihomo-tls-usage.db")
	record := upsertTestCertificateRecord(t, "mihomo-tls.example.com")

	tlsRow := &model.MihomoTls{
		Name:                "mihomo-listener",
		CertificateRecordID: record.Id,
		Server:              mustJSONRaw(t, map[string]interface{}{}),
		Client:              mustJSONRaw(t, map[string]interface{}{}),
	}
	if err := db.Create(tlsRow).Error; err != nil {
		t.Fatalf("create mihomo tls row failed: %v", err)
	}

	views, err := certificateInventory.List()
	if err != nil {
		t.Fatalf("list certificates failed: %v", err)
	}
	view := findCertificateRecordView(t, views, record.Id)
	if !view.DeleteBlocked || !view.InUseByMihomo {
		t.Fatalf("expected mihomo TLS usage to block delete, got %#v", view)
	}
	if !strings.Contains(view.UsageLabel, "mihomo-listener") {
		t.Fatalf("expected usage label to include mihomo tls name, got %q", view.UsageLabel)
	}

	if _, err := (&AcmeService{}).Delete(AcmeDeletePayload{ID: record.Id}); err == nil {
		t.Fatalf("expected delete to fail while certificate is used by mihomo TLS")
	}
}

func TestCertificateDeleteAllowedAfterTLSBindingCleared(t *testing.T) {
	db := setupMihomoSyncTestDB(t, "cert-tls-usage-cleared.db")
	record := upsertTestCertificateRecord(t, "cleared-tls.example.com")

	tlsRow := &model.Tls{
		Name:                "cleared-listener",
		CertificateRecordID: record.Id,
		Server:              mustJSONRaw(t, map[string]interface{}{}),
		Client:              mustJSONRaw(t, map[string]interface{}{}),
	}
	if err := db.Create(tlsRow).Error; err != nil {
		t.Fatalf("create tls row failed: %v", err)
	}
	if err := db.Model(&model.Tls{}).Where("id = ?", tlsRow.Id).Update("certificate_record_id", 0).Error; err != nil {
		t.Fatalf("clear tls binding failed: %v", err)
	}

	views, err := certificateInventory.List()
	if err != nil {
		t.Fatalf("list certificates failed: %v", err)
	}
	view := findCertificateRecordView(t, views, record.Id)
	if view.DeleteBlocked || view.InUseByTLS {
		t.Fatalf("expected delete block to clear, got %#v", view)
	}

	if _, err := (&AcmeService{}).Delete(AcmeDeletePayload{ID: record.Id}); err != nil {
		t.Fatalf("delete after binding cleared failed: %v", err)
	}
}

func TestSyncTLSBindingsForCertificateRecordRefreshesBoundTLSMaterial(t *testing.T) {
	db := setupMihomoSyncTestDB(t, "cert-tls-sync.db")
	_, fullchainPEM, err := (&ServerService{}).generateCertWithAlgorithm(
		"sync.example.com",
		"ecc256",
		"ecc256",
		tlsCertificateUsageServer,
		time.Now().Add(-time.Hour),
		time.Now().Add(24*time.Hour),
	)
	if err != nil {
		t.Fatalf("generate certificate failed: %v", err)
	}
	keyPEM := []byte("-----BEGIN PRIVATE KEY-----\nMC4CAQAwBQYDK2VwBCIEIAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA\n-----END PRIVATE KEY-----\n")

	dir := t.TempDir()
	fullchainPath := filepath.Join(dir, "fullchain.pem")
	keyPath := filepath.Join(dir, "key.pem")
	if err := os.WriteFile(fullchainPath, fullchainPEM, 0o644); err != nil {
		t.Fatalf("write fullchain failed: %v", err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		t.Fatalf("write key failed: %v", err)
	}

	record, err := certificateInventory.Upsert(CertificateUpsertPayload{
		SourceType:    CertificateSourceSelfSigned,
		SourceRef:     "sync-bound",
		MainDomain:    "sync.example.com",
		Domains:       []string{"sync.example.com"},
		CertPath:      fullchainPath,
		KeyPath:       keyPath,
		FullchainPath: fullchainPath,
		CertPEM:       fullchainPEM,
		KeyPEM:        keyPEM,
		NotBefore:     time.Now().Add(-time.Hour).Unix(),
		NotAfter:      time.Now().Add(24 * time.Hour).Unix(),
	})
	if err != nil {
		t.Fatalf("upsert certificate failed: %v", err)
	}

	tlsRow := &model.Tls{
		Name:                "sync-listener",
		CertificateRecordID: record.Id,
		Server: mustJSONRaw(t, map[string]interface{}{
			"certificate": []string{"OLD-CERT"},
			"key":         []string{"OLD-KEY"},
		}),
		Client: mustJSONRaw(t, map[string]interface{}{
			"certificate_public_key_sha256": []string{"old-sha"},
			"fingerprint":                   "OLD",
		}),
	}
	if err := db.Create(tlsRow).Error; err != nil {
		t.Fatalf("create tls row failed: %v", err)
	}

	changed, err := SyncTLSBindingsForCertificateRecord(record.Id, "")
	if err != nil {
		t.Fatalf("sync tls bindings failed: %v", err)
	}
	if !changed {
		t.Fatalf("expected sync to report changes")
	}

	updated := &model.Tls{}
	if err := db.Where("id = ?", tlsRow.Id).First(updated).Error; err != nil {
		t.Fatalf("load updated tls failed: %v", err)
	}

	var server map[string]interface{}
	if err := json.Unmarshal(updated.Server, &server); err != nil {
		t.Fatalf("decode server tls failed: %v", err)
	}
	if _, exists := server["certificate_path"]; exists {
		t.Fatalf("expected certificate_path removed, got %#v", server["certificate_path"])
	}
	if _, exists := server["key_path"]; exists {
		t.Fatalf("expected key_path removed, got %#v", server["key_path"])
	}
	certLines, certOK := server["certificate"].([]interface{})
	if !certOK || len(certLines) == 0 {
		t.Fatalf("expected inline certificate lines, got %#v", server["certificate"])
	}
	keyLines, keyOK := server["key"].([]interface{})
	if !keyOK || len(keyLines) == 0 {
		t.Fatalf("expected inline key lines, got %#v", server["key"])
	}

	var client map[string]interface{}
	if err := json.Unmarshal(updated.Client, &client); err != nil {
		t.Fatalf("decode client tls failed: %v", err)
	}
	shaValues, ok := client["certificate_public_key_sha256"].([]interface{})
	if !ok || len(shaValues) != 1 || shaValues[0] == "old-sha" {
		t.Fatalf("expected refreshed sha256, got %#v", client["certificate_public_key_sha256"])
	}
	if got, _ := client["fingerprint"].(string); got == "" || got == "OLD" {
		t.Fatalf("expected refreshed fingerprint, got %#v", client["fingerprint"])
	}
}

func TestSyncTLSBindingsForCertificateRecordRefreshesMihomoTLSHashes(t *testing.T) {
	db := setupMihomoSyncTestDB(t, "cert-mihomo-tls-sync.db")
	_, fullchainPEM, err := (&ServerService{}).generateCertWithAlgorithm(
		"sync-mihomo.example.com",
		"ecc256",
		"ecc256",
		tlsCertificateUsageServer,
		time.Now().Add(-time.Hour),
		time.Now().Add(24*time.Hour),
	)
	if err != nil {
		t.Fatalf("generate certificate failed: %v", err)
	}
	keyPEM := []byte("-----BEGIN PRIVATE KEY-----\nMC4CAQAwBQYDK2VwBCIEIAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA\n-----END PRIVATE KEY-----\n")

	dir := t.TempDir()
	fullchainPath := filepath.Join(dir, "fullchain.pem")
	keyPath := filepath.Join(dir, "key.pem")
	if err := os.WriteFile(fullchainPath, fullchainPEM, 0o644); err != nil {
		t.Fatalf("write fullchain failed: %v", err)
	}
	if err := os.WriteFile(keyPath, keyPEM, 0o600); err != nil {
		t.Fatalf("write key failed: %v", err)
	}

	record, err := certificateInventory.Upsert(CertificateUpsertPayload{
		SourceType:    CertificateSourceSelfSigned,
		SourceRef:     "sync-mihomo-bound",
		MainDomain:    "sync-mihomo.example.com",
		Domains:       []string{"sync-mihomo.example.com"},
		CertPath:      fullchainPath,
		KeyPath:       keyPath,
		FullchainPath: fullchainPath,
		CertPEM:       fullchainPEM,
		KeyPEM:        keyPEM,
		NotBefore:     time.Now().Add(-time.Hour).Unix(),
		NotAfter:      time.Now().Add(24 * time.Hour).Unix(),
	})
	if err != nil {
		t.Fatalf("upsert certificate failed: %v", err)
	}

	tlsRow := &model.MihomoTls{
		Name:                "sync-mihomo-listener",
		CertificateRecordID: record.Id,
		Server: mustJSONRaw(t, map[string]interface{}{
			"certificate":                          []string{"OLD-CERT"},
			"key":                                  []string{"OLD-KEY"},
			"client_certificate_public_key_sha256": []string{"old-client-sha"},
		}),
		Client: mustJSONRaw(t, map[string]interface{}{
			"certificate_public_key_sha256": []string{"old-sha"},
			"fingerprint":                   "OLD",
		}),
	}
	if err := db.Create(tlsRow).Error; err != nil {
		t.Fatalf("create mihomo tls row failed: %v", err)
	}

	changed, err := SyncTLSBindingsForCertificateRecord(record.Id, "")
	if err != nil {
		t.Fatalf("sync mihomo tls bindings failed: %v", err)
	}
	if !changed {
		t.Fatalf("expected mihomo sync to report changes")
	}

	updated := &model.MihomoTls{}
	if err := db.Where("id = ?", tlsRow.Id).First(updated).Error; err != nil {
		t.Fatalf("load updated mihomo tls failed: %v", err)
	}

	var server map[string]interface{}
	if err := json.Unmarshal(updated.Server, &server); err != nil {
		t.Fatalf("decode mihomo server tls failed: %v", err)
	}
	if _, exists := server["certificate_path"]; exists {
		t.Fatalf("expected certificate_path removed, got %#v", server["certificate_path"])
	}
	if _, exists := server["key_path"]; exists {
		t.Fatalf("expected key_path removed, got %#v", server["key_path"])
	}
	certLines, certOK := server["certificate"].([]interface{})
	if !certOK || len(certLines) == 0 {
		t.Fatalf("expected inline certificate lines, got %#v", server["certificate"])
	}
	keyLines, keyOK := server["key"].([]interface{})
	if !keyOK || len(keyLines) == 0 {
		t.Fatalf("expected inline key lines, got %#v", server["key"])
	}
	if _, exists := server["client_certificate_public_key_sha256"]; exists {
		t.Fatalf("expected mihomo server-side client_certificate_public_key_sha256 to be stripped, got %#v", server["client_certificate_public_key_sha256"])
	}

	var client map[string]interface{}
	if err := json.Unmarshal(updated.Client, &client); err != nil {
		t.Fatalf("decode mihomo client tls failed: %v", err)
	}
	shaValues, ok := client["certificate_public_key_sha256"].([]interface{})
	if !ok || len(shaValues) != 1 || shaValues[0] == "old-sha" {
		t.Fatalf("expected refreshed mihomo sha256, got %#v", client["certificate_public_key_sha256"])
	}
	if got, _ := client["fingerprint"].(string); got == "" || got == "OLD" {
		t.Fatalf("expected refreshed mihomo fingerprint, got %#v", client["fingerprint"])
	}
}

func TestForceSyncTLSBindingsForCertificateRecordRebuildsDefaultManagedClientWhenPathsUnchanged(t *testing.T) {
	db := setupMihomoSyncTestDB(t, "cert-tls-force-default.db")
	oldKeyPEM, oldFullchainPEM, err := (&ServerService{}).generateCertWithAlgorithm(
		"force-default-old.example.com",
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
		"force-default-new.example.com",
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

	record, err := certificateInventory.Upsert(CertificateUpsertPayload{
		SourceType:    CertificateSourceSelfSigned,
		SourceRef:     "force-default",
		MainDomain:    "force-default.example.com",
		Domains:       []string{"force-default.example.com"},
		CertPath:      fullchainPath,
		KeyPath:       keyPath,
		FullchainPath: fullchainPath,
		CertPEM:       oldFullchainPEM,
		KeyPEM:        oldKeyPEM,
		FullchainPEM:  oldFullchainPEM,
		NotBefore:     time.Now().Add(-time.Hour).Unix(),
		NotAfter:      time.Now().Add(24 * time.Hour).Unix(),
	})
	if err != nil {
		t.Fatalf("upsert certificate failed: %v", err)
	}

	tlsRow := &model.Tls{
		Name:                "force-default-tls",
		CertificateRecordID: record.Id,
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

	inbound := &model.Inbound{
		Type:    "trojan",
		Tag:     "force-trojan",
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
		Name:     "force-user",
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
	record.CertPEM = append([]byte(nil), newFullchainPEM...)
	record.FullchainPEM = append([]byte(nil), newFullchainPEM...)
	record.KeyPEM = append([]byte(nil), newKeyPEM...)
	if err := db.Save(record).Error; err != nil {
		t.Fatalf("save renewed certificate record failed: %v", err)
	}

	changed, err := ForceSyncTLSBindingsForCertificateRecord(record.Id, "")
	if err != nil {
		t.Fatalf("force sync tls bindings failed: %v", err)
	}
	if !changed {
		t.Fatalf("expected force sync to report broadcast")
	}

	var blockCount int64
	if err := db.Model(model.SubSyncBlock{}).
		Where("source_type = ? AND source_client_id = ? AND source_inbound_id = ?", subOutboundSourceClient, client.Id, inbound.Id).
		Count(&blockCount).Error; err != nil {
		t.Fatalf("count sync blocks failed: %v", err)
	}
	if blockCount != 0 {
		t.Fatalf("expected force sync to clear manual delete block, count=%d", blockCount)
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

func TestForceSyncTLSBindingsForCertificateRecordRebuildsMihomoManagedClientWhenPathsUnchanged(t *testing.T) {
	db := setupMihomoSyncTestDB(t, "cert-tls-force-mihomo.db")
	oldKeyPEM, oldFullchainPEM, err := (&ServerService{}).generateCertWithAlgorithm(
		"force-mihomo-old.example.com",
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
		"force-mihomo-new.example.com",
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

	record, err := certificateInventory.Upsert(CertificateUpsertPayload{
		SourceType:    CertificateSourceSelfSigned,
		SourceRef:     "force-mihomo",
		MainDomain:    "force-mihomo.example.com",
		Domains:       []string{"force-mihomo.example.com"},
		CertPath:      fullchainPath,
		KeyPath:       keyPath,
		FullchainPath: fullchainPath,
		CertPEM:       oldFullchainPEM,
		KeyPEM:        oldKeyPEM,
		FullchainPEM:  oldFullchainPEM,
		NotBefore:     time.Now().Add(-time.Hour).Unix(),
		NotAfter:      time.Now().Add(24 * time.Hour).Unix(),
	})
	if err != nil {
		t.Fatalf("upsert certificate failed: %v", err)
	}

	tlsRow := &model.MihomoTls{
		Name:                "force-mihomo-tls",
		CertificateRecordID: record.Id,
		Server: mustJSONRawIndent(t, map[string]interface{}{
			"certificate_path": fullchainPath,
			"key_path":         keyPath,
		}),
		Client: mustJSONRawIndent(t, map[string]interface{}{
			"include_server_certificate": true,
		}),
	}
	if err := db.Create(tlsRow).Error; err != nil {
		t.Fatalf("create mihomo tls row failed: %v", err)
	}

	inbound := &model.MihomoInbound{
		Type:    "trojan",
		Tag:     "force-mihomo-trojan",
		TlsId:   tlsRow.Id,
		OutJson: mustJSONRaw(t, map[string]interface{}{}),
		Options: mustJSONRaw(t, map[string]interface{}{
			"listen_port": 443,
		}),
	}
	if err := db.Create(inbound).Error; err != nil {
		t.Fatalf("create mihomo inbound failed: %v", err)
	}

	client := &model.MihomoClient{
		Enable:   true,
		Name:     "force-mihomo-user",
		Config:   mustJSONRaw(t, map[string]interface{}{"trojan": map[string]interface{}{"password": "secret"}}),
		Inbounds: mustJSONRaw(t, []uint{inbound.Id}),
	}
	if err := db.Create(client).Error; err != nil {
		t.Fatalf("create mihomo client failed: %v", err)
	}
	if err := (&SettingService{}).SaveSubManagerAutoSyncMihomoClientIDs([]uint{client.Id}); err != nil {
		t.Fatalf("save mihomo auto sync ids failed: %v", err)
	}
	if err := blockSubSyncInbound(db, subOutboundSourceMihomoClient, client.Id, inbound.Id); err != nil {
		t.Fatalf("block mihomo sub sync inbound failed: %v", err)
	}

	if err := os.WriteFile(fullchainPath, newFullchainPEM, 0o644); err != nil {
		t.Fatalf("write new fullchain failed: %v", err)
	}
	if err := os.WriteFile(keyPath, newKeyPEM, 0o600); err != nil {
		t.Fatalf("write new key failed: %v", err)
	}
	record.CertPEM = append([]byte(nil), newFullchainPEM...)
	record.FullchainPEM = append([]byte(nil), newFullchainPEM...)
	record.KeyPEM = append([]byte(nil), newKeyPEM...)
	if err := db.Save(record).Error; err != nil {
		t.Fatalf("save renewed certificate record failed: %v", err)
	}

	changed, err := ForceSyncTLSBindingsForCertificateRecord(record.Id, "")
	if err != nil {
		t.Fatalf("force sync mihomo tls bindings failed: %v", err)
	}
	if !changed {
		t.Fatalf("expected force sync to report broadcast")
	}

	var blockCount int64
	if err := db.Model(model.SubSyncBlock{}).
		Where("source_type = ? AND source_client_id = ? AND source_inbound_id = ?", subOutboundSourceMihomoClient, client.Id, inbound.Id).
		Count(&blockCount).Error; err != nil {
		t.Fatalf("count mihomo sync blocks failed: %v", err)
	}
	if blockCount != 0 {
		t.Fatalf("expected force sync to clear mihomo manual delete block, count=%d", blockCount)
	}

	subTag := buildMihomoClientSubTag(inbound.Tag, client.Name)
	saved := &model.SubOutbound{}
	if err := db.Where("tag = ?", subTag).First(saved).Error; err != nil {
		t.Fatalf("load synced mihomo suboutbound failed: %v", err)
	}
	if saved.SourceType != subOutboundSourceMihomoClient || saved.SourceClientId != client.Id || saved.SourceInboundId != inbound.Id {
		t.Fatalf("unexpected mihomo sync source metadata: %#v", saved)
	}
	assertSubOutboundCertificatePEM(t, saved, newFullchainPEM)
}

func upsertTestCertificateRecord(t *testing.T, domain string) *model.CertificateRecord {
	t.Helper()
	row, err := certificateInventory.Upsert(CertificateUpsertPayload{
		SourceType:   CertificateSourceImported,
		SourceRef:    "test:" + domain,
		MainDomain:   domain,
		Domains:      []string{domain},
		CertPEM:      []byte("test-cert"),
		KeyPEM:       []byte("test-key"),
		FullchainPEM: []byte("test-cert"),
	})
	if err != nil {
		t.Fatalf("upsert certificate record failed: %v", err)
	}
	return row
}

func mustJSONRawIndent(t *testing.T, value interface{}) json.RawMessage {
	t.Helper()

	raw, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatalf("json.MarshalIndent failed: %v", err)
	}
	return json.RawMessage(raw)
}

func assertSubOutboundCertificatePEM(t *testing.T, subOutbound *model.SubOutbound, expectedPEM []byte) {
	t.Helper()

	raw, err := resolveSubOutboundJSON(subOutbound)
	if err != nil {
		t.Fatalf("resolve suboutbound json failed: %v", err)
	}
	var outbound map[string]interface{}
	if err := json.Unmarshal(raw, &outbound); err != nil {
		t.Fatalf("decode suboutbound json failed: %v", err)
	}
	tlsMap, ok := outbound["tls"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected tls map in suboutbound: %#v", outbound["tls"])
	}
	certificateLines, ok := tlsMap["certificate"].([]interface{})
	if !ok || len(certificateLines) == 0 {
		t.Fatalf("expected inline certificate lines, got %#v", tlsMap["certificate"])
	}
	lines := make([]string, 0, len(certificateLines))
	for _, line := range certificateLines {
		text, ok := line.(string)
		if !ok {
			t.Fatalf("expected certificate line string, got %#v", line)
		}
		lines = append(lines, text)
	}
	if strings.TrimSpace(strings.Join(lines, "\n")) != strings.TrimSpace(string(expectedPEM)) {
		t.Fatalf("synced certificate PEM was not refreshed")
	}
}

func findCertificateRecordView(t *testing.T, views []CertificateRecordView, id uint) CertificateRecordView {
	t.Helper()
	for _, view := range views {
		if view.Id == id {
			return view
		}
	}
	t.Fatalf("certificate view %d not found in %#v", id, views)
	return CertificateRecordView{}
}
