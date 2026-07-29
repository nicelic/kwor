package service

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/util/common"
	"gorm.io/gorm"
)

// migrateLegacyAcmeRuntimeOnStartup performs the one-time transition away
// from acme.sh's shared account.conf/config directory.  Account state from
// that layout cannot be attributed safely to one database account, so those
// accounts deliberately become pending-registration accounts.  Certificate
// material stays in certificate_records, while automatic renewal is disabled
// until an operator explicitly reissues it with a selected account.
func (s *AcmeService) migrateLegacyAcmeRuntimeOnStartup() error {
	if strings.TrimSpace(s.readSettingWithDefault(acmeRuntimeSchemaV2Key, "")) == "1" {
		return nil
	}

	acmeOperationMu.Lock()
	defer acmeOperationMu.Unlock()

	if strings.TrimSpace(s.readSettingWithDefault(acmeRuntimeSchemaV2Key, "")) == "1" {
		return nil
	}

	// DNS credentials are the only recoverable secrets in the old shared
	// account.conf. Persist them first, then remove the shared runtime below.
	if err := s.migrateLegacyDNSSecretsFromAccountConf(); err != nil {
		return err
	}
	if err := s.migrateLegacyAcmeCertificateMirror(); err != nil {
		return err
	}
	if err := s.markLegacyAcmeAccountsPendingRegistration(); err != nil {
		return err
	}
	if err := s.clearLegacyAcmeRuntimeFiles(); err != nil {
		return err
	}
	if err := s.setString(acmeRuntimeSchemaV2Key, "1"); err != nil {
		return err
	}
	return nil
}

// migrateLegacyAcmeCertificateMirror preserves rows that existed only in the
// retired acme_certificates mirror. Existing inventory rows always win: after
// this migration certificate_records is the single source of truth.
func (s *AcmeService) migrateLegacyAcmeCertificateMirror() error {
	db := database.GetDB()
	if db == nil || !db.Migrator().HasTable(&model.AcmeCertificate{}) {
		return nil
	}
	legacyRows := make([]model.AcmeCertificate, 0)
	if err := db.Order("id ASC").Find(&legacyRows).Error; err != nil {
		return err
	}
	for i := range legacyRows {
		legacy := &legacyRows[i]
		legacyRef := fmt.Sprintf("legacy-%d", legacy.Id)
		oldRef := strconv.FormatUint(uint64(legacy.Id), 10)
		existing := &model.CertificateRecord{}
		err := db.Where("source_type = ? AND source_ref IN ?", CertificateSourceACME, []string{legacyRef, oldRef}).First(existing).Error
		if err == nil {
			continue
		}
		if !database.IsNotFound(err) {
			return err
		}

		certificateType := normalizeAcmeCertificateType(legacy.CertificateType)
		acmeAccountID := legacy.AcmeAccountID
		acmeAccountName := strings.TrimSpace(legacy.AcmeAccountName)
		dnsAccountID := legacy.DNSAccountID
		dnsAccountName := strings.TrimSpace(legacy.DNSAccountName)
		runtimeProfile := ""
		if certificateType == acmeCertificateTypeIP {
			acmeAccountID = 0
			acmeAccountName = ""
			dnsAccountID = 0
			dnsAccountName = ""
			runtimeProfile = acmeIPRuntimeProfile
		}

		if _, err := certificateInventory.Upsert(CertificateUpsertPayload{
			SourceType:         CertificateSourceACME,
			SourceRef:          legacyRef,
			MainDomain:         strings.TrimSpace(legacy.MainDomain),
			Domains:            decodeCertificateDomains(legacy.DomainSet),
			CertificateType:    certificateType,
			CertProfile:        strings.TrimSpace(legacy.CertProfile),
			Challenge:          strings.TrimSpace(legacy.Challenge),
			KeyLength:          strings.TrimSpace(legacy.KeyLength),
			CAServer:           strings.TrimSpace(legacy.CAServer),
			UseECC:             legacy.UseECC,
			AutoRenew:          legacy.AutoRenew,
			AcmeAccountID:      acmeAccountID,
			AcmeAccountName:    acmeAccountName,
			DNSAccountID:       dnsAccountID,
			DNSAccountName:     dnsAccountName,
			AcmeRuntimeProfile: runtimeProfile,
			ApplyTarget:        strings.TrimSpace(legacy.ApplyTarget),
			PushDir:            strings.TrimSpace(legacy.PushDir),
			PushFiles:          strings.TrimSpace(legacy.PushFiles),
			Remark:             strings.TrimSpace(legacy.Remark),
			Webroot:            strings.TrimSpace(legacy.Webroot),
			DNSProvider:        strings.TrimSpace(legacy.DNSProvider),
			CustomArgs:         strings.TrimSpace(legacy.CustomArgs),
			CertPEM:            append([]byte(nil), legacy.CertPEM...),
			KeyPEM:             append([]byte(nil), legacy.KeyPEM...),
			FullchainPEM:       append([]byte(nil), legacy.FullchainPEM...),
			ChainPEM:           append([]byte(nil), legacy.ChainPEM...),
			Fingerprint:        strings.TrimSpace(legacy.Fingerprint),
			NotBefore:          legacy.NotBefore,
			NotAfter:           legacy.NotAfter,
			LastIssuedAt:       legacy.LastIssuedAt,
			LastRenewedAt:      legacy.LastRenewedAt,
			LastError:          strings.TrimSpace(legacy.LastError),
			LastOutput:         strings.TrimSpace(legacy.LastOutput),
			ListOrderAt:        legacy.CreatedAt.Unix(),
		}); err != nil {
			return err
		}
	}

	// Keeping the unused table schema is harmless for an in-place SQLite
	// upgrade and avoids a destructive table rebuild. Its rows must disappear,
	// though: no service path is allowed to read it after this point.
	if len(legacyRows) > 0 {
		if err := db.Where("1 = 1").Delete(&model.AcmeCertificate{}).Error; err != nil {
			return err
		}
	}
	return nil
}

func (s *AcmeService) markLegacyAcmeAccountsPendingRegistration() error {
	db := database.GetDB()
	return db.Transaction(func(tx *gorm.DB) error {
		accounts := make([]model.AcmeAccount, 0)
		if err := tx.Where("system = ?", false).Find(&accounts).Error; err != nil {
			return err
		}
		pendingIDs := make([]uint, 0, len(accounts))
		for i := range accounts {
			account := &accounts[i]
			if len(account.RuntimeState) > 0 {
				continue
			}
			keyLength := effectiveAcmeAccountKeyLength(account)
			if err := tx.Model(&model.AcmeAccount{}).Where("id = ?", account.Id).Updates(map[string]interface{}{
				"account_key_length": keyLength,
				"key_length":         keyLength,
				"registered":         false,
			}).Error; err != nil {
				return err
			}
			pendingIDs = append(pendingIDs, account.Id)
		}
		if len(pendingIDs) == 0 {
			return nil
		}
		return tx.Model(&model.CertificateRecord{}).
			Where("source_type = ? AND acme_account_id IN ?", CertificateSourceACME, pendingIDs).
			Update("auto_renew", false).Error
	})
}

func (s *AcmeService) clearLegacyAcmeRuntimeFiles() error {
	legacyRows := make([]model.CertificateRecord, 0)
	if err := database.GetDB().Where("source_type = ?", CertificateSourceACME).Find(&legacyRows).Error; err != nil {
		return err
	}
	for _, homeDir := range s.legacyAcmeRuntimeHomes() {
		if err := clearOneLegacyAcmeRuntimeHome(homeDir, legacyRows); err != nil {
			return err
		}
	}
	return nil
}

func (s *AcmeService) legacyAcmeRuntimeHomes() []string {
	// Only clean directories historically created by this panel. A configured
	// or auto-detected acme.sh executable may belong to another application or
	// an operator, so it must never become a migration target.
	candidates := []string{
		managedAcmeHomeDir(),
		legacyManagedAcmeHomeDir(),
	}
	seen := map[string]struct{}{}
	result := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		candidate = filepath.Clean(candidate)
		if candidate == "" || candidate == "." || candidate == string(filepath.Separator) {
			continue
		}
		if _, exists := seen[candidate]; exists {
			continue
		}
		seen[candidate] = struct{}{}
		result = append(result, candidate)
	}
	return result
}

func clearOneLegacyAcmeRuntimeHome(homeDir string, records []model.CertificateRecord) error {
	homeDir = filepath.Clean(strings.TrimSpace(homeDir))
	if homeDir == "" || homeDir == "." || homeDir == string(filepath.Separator) || !pathExists(homeDir) {
		return nil
	}
	for _, name := range []string{"account.conf", "ca.conf", "http.header", "http.header.bak"} {
		if err := os.Remove(filepath.Join(homeDir, name)); err != nil && !os.IsNotExist(err) {
			return common.NewError("clear legacy acme runtime file failed: ", err)
		}
	}
	caDir := filepath.Join(homeDir, "ca")
	if err := os.RemoveAll(caDir); err != nil && !os.IsNotExist(err) {
		return common.NewError("clear legacy acme CA runtime failed: ", err)
	}
	for i := range records {
		mainDomain := strings.TrimSpace(records[i].MainDomain)
		if mainDomain == "" {
			continue
		}
		// Try both old layout suffixes. The database certificate material was
		// already retained, and this only targets direct children of homeDir.
		cleanupAcmeWorkingTree(homeDir, mainDomain, false)
		cleanupAcmeWorkingTree(homeDir, mainDomain, true)
	}
	return nil
}
