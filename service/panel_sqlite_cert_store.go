package service

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
)

type PanelTLSMaterial struct {
	Target     PanelSelfSignedTarget
	SourceType string
	SourceName string
	CertPath   string
	KeyPath    string
	CertPEM    []byte
	KeyPEM     []byte
	Record     *model.CertificateRecord
}

const (
	PanelTLSSourceInventoryRecord = "inventory_record"
)

func LoadPanelSQLiteCertificate(target PanelSelfSignedTarget) (*model.PanelCertificate, error) {
	if !target.isValid() {
		return nil, fmt.Errorf("invalid panel certificate target: %q", target)
	}

	entry := &model.PanelCertificate{}
	err := database.GetDB().Where("target = ?", string(target)).First(entry).Error
	if err != nil {
		return nil, err
	}
	return entry, nil
}

func SavePanelSQLiteCertificate(target PanelSelfSignedTarget, certPEM []byte, keyPEM []byte, now time.Time) (*model.PanelCertificate, error) {
	if !target.isValid() {
		return nil, fmt.Errorf("invalid panel certificate target: %q", target)
	}
	if now.IsZero() {
		now = time.Now()
	}

	inspect := InspectPanelCertificatePEM(certPEM, keyPEM, "sqlite:"+string(target), now)
	if inspect.Class != PanelCertificateSelfSigned {
		return nil, fmt.Errorf("sqlite self-signed certificate inspection failed: %s", inspect.Reason)
	}

	entry := &model.PanelCertificate{
		Target:      string(target),
		CertPEM:     append([]byte(nil), certPEM...),
		KeyPEM:      append([]byte(nil), keyPEM...),
		Fingerprint: inspect.Fingerprint,
		NotBefore:   inspect.NotBefore.Unix(),
		NotAfter:    inspect.NotAfter.Unix(),
		UpdatedAt:   now.Unix(),
	}
	if err := database.GetDB().Save(entry).Error; err != nil {
		return nil, err
	}
	if _, err := upsertInventoryFromPanelSQLite(target, entry); err != nil {
		return nil, err
	}
	return entry, nil
}

func GenerateAndAssignPanelBootstrapCertificate(target PanelSelfSignedTarget, now time.Time) (*PanelSelfSignedResult, error) {
	selfSignedService := &SelfSignedService{}
	detect := detectPublicIPForPanelCert()
	domainsText := strings.TrimSpace(detect.identity)
	allowEmptyNames := false
	if domainsText == "" {
		allowEmptyNames = true
	}

	record, issueErr := selfSignedService.Issue(SelfSignedIssuePayload{
		PreferredSourceRef: "bootstrap:" + strings.TrimSpace(string(target)),
		PlatformCode:       panelSelfSignedLetsEncryptAuthorityCode,
		PlatformName:       "Let's Encrypt",
		DomainsText:        domainsText,
		AllowEmptyNames:    allowEmptyNames,
		KeyAlgorithm:       "ecc384",
		SignatureAlgorithm: "ecc384",
		DurationValue:      defaultSelfSignedDurationValue,
		DurationUnit:       defaultSelfSignedDurationUnit,
		Remark:             "\u754c\u9762/\u8ba2\u9605\u9996\u6b21\u81ea\u7b7e\u8bc1\u4e66",
		ApplyTarget:        string(target),
	})
	if issueErr != nil {
		return nil, issueErr
	}
	if record == nil || record.Certificate == nil {
		return nil, fmt.Errorf("issue self-signed certificate failed: empty certificate result")
	}
	row, err := certificateInventory.GetRecordByID(record.Certificate.Id)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, fmt.Errorf("self-signed inventory record not found")
	}
	inspect := InspectPanelCertificatePEM(row.FullchainPEM, row.KeyPEM, row.SourceRef, now)
	result := &PanelSelfSignedResult{
		CertPEM:         append([]byte(nil), row.FullchainPEM...),
		KeyPEM:          append([]byte(nil), row.KeyPEM...),
		Identity:        strings.TrimSpace(detect.identity),
		IdentityKind:    strings.TrimSpace(detect.kind),
		DetectionReason: strings.TrimSpace(detect.reason),
		NotAfter:        time.Unix(row.NotAfter, 0),
		CertPath:        strings.TrimSpace(row.FullchainPath),
		KeyPath:         strings.TrimSpace(row.KeyPath),
	}
	if result.IdentityKind == "" {
		result.IdentityKind = "none"
	}
	if result.DetectionReason == "" {
		result.DetectionReason = "issued by certificate center self-signed flow"
	}
	if result.Identity == "" && strings.TrimSpace(record.Certificate.MainDomain) != "" && strings.TrimSpace(record.Certificate.MainDomain) != selfSignedUnnamedMainDomain {
		result.Identity = strings.TrimSpace(record.Certificate.MainDomain)
		if result.IdentityKind == "none" {
			result.IdentityKind = "inventory"
		}
	}
	return result.withInspectionFallback(inspect), nil
}

// GenerateAndStorePanelSQLiteCertificate is kept as a compatibility wrapper.
// The implementation now delegates to certificate center inventory issuance.
func GenerateAndStorePanelSQLiteCertificate(target PanelSelfSignedTarget, now time.Time) (*PanelSelfSignedResult, error) {
	return GenerateAndAssignPanelBootstrapCertificate(target, now)
}

func ResolvePanelTLSMaterial(settingService *SettingService, target PanelSelfSignedTarget) (*PanelTLSMaterial, error) {
	materials, err := ResolvePanelTLSMaterials(settingService, target)
	if err != nil {
		return nil, err
	}
	if len(materials) == 0 {
		return nil, nil
	}
	return materials[0], nil
}

func ResolvePanelTLSMaterials(settingService *SettingService, target PanelSelfSignedTarget) ([]*PanelTLSMaterial, error) {
	if settingService == nil {
		return nil, fmt.Errorf("setting service is nil")
	}
	if !target.isValid() {
		return nil, fmt.Errorf("invalid panel certificate target: %q", target)
	}

	rows, err := resolveAssignedPanelTLSRecords(settingService, target)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return []*PanelTLSMaterial{}, nil
	}
	materials := make([]*PanelTLSMaterial, 0, len(rows))
	for _, row := range rows {
		material := buildPanelTLSMaterialFromRecord(target, row)
		if material == nil {
			continue
		}
		materials = append(materials, material)
	}
	return materials, nil
}

func SyncPanelTLSAssignments(settingService *SettingService) error {
	if settingService == nil {
		return nil
	}
	if _, err := resolveAssignedPanelTLSRecords(settingService, PanelSelfSignedTargetPanel); err != nil {
		return err
	}
	if _, err := resolveAssignedPanelTLSRecords(settingService, PanelSelfSignedTargetSub); err != nil {
		return err
	}
	return nil
}

func resolveAssignedPanelTLSRecord(settingService *SettingService, target PanelSelfSignedTarget) (*model.CertificateRecord, error) {
	rows, err := resolveAssignedPanelTLSRecords(settingService, target)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, nil
	}
	return rows[0], nil
}

func resolveAssignedPanelTLSRecords(settingService *SettingService, target PanelSelfSignedTarget) ([]*model.CertificateRecord, error) {
	assignedRecordIDs, err := GetAssignedCertificateRecordIDs(settingService, target)
	if err != nil {
		return nil, err
	}
	if len(assignedRecordIDs) == 0 {
		return []*model.CertificateRecord{}, nil
	}

	rows := make([]*model.CertificateRecord, 0, len(assignedRecordIDs))
	validIDs := make([]uint, 0, len(assignedRecordIDs))
	needsWriteback := false
	for _, assignedRecordID := range assignedRecordIDs {
		row, getErr := certificateInventory.GetRecordByID(assignedRecordID)
		if getErr != nil {
			if database.IsNotFound(getErr) {
				needsWriteback = true
				continue
			}
			return nil, getErr
		}
		if row == nil {
			needsWriteback = true
			continue
		}
		if len(row.FullchainPEM) == 0 || len(row.KeyPEM) == 0 {
			return nil, fmt.Errorf("assigned certificate %d has empty tls material", assignedRecordID)
		}
		validIDs = append(validIDs, row.Id)
		rows = append(rows, row)
	}

	if needsWriteback || len(validIDs) != len(assignedRecordIDs) {
		if setErr := SetAssignedCertificateRecordIDs(settingService, target, validIDs); setErr != nil {
			return nil, setErr
		}
	}
	return rows, nil
}

func buildPanelTLSMaterialFromRecord(target PanelSelfSignedTarget, row *model.CertificateRecord) *PanelTLSMaterial {
	if row == nil {
		return nil
	}
	return &PanelTLSMaterial{
		Target:     target,
		SourceType: PanelTLSSourceInventoryRecord,
		SourceName: fmt.Sprintf("inventory:%d", row.Id),
		CertPath:   strings.TrimSpace(row.FullchainPath),
		KeyPath:    strings.TrimSpace(row.KeyPath),
		CertPEM:    append([]byte(nil), row.FullchainPEM...),
		KeyPEM:     append([]byte(nil), row.KeyPEM...),
		Record:     row,
	}
}

func EnsurePanelTLSMaterial(settingService *SettingService, target PanelSelfSignedTarget, now time.Time) (*PanelTLSMaterial, *PanelSelfSignedResult, error) {
	material, err := ResolvePanelTLSMaterial(settingService, target)
	if err != nil {
		return nil, nil, err
	}
	return material, nil, nil
}

func EnsurePanelTLSMaterials(settingService *SettingService, target PanelSelfSignedTarget, now time.Time) ([]*PanelTLSMaterial, *PanelSelfSignedResult, error) {
	materials, err := ResolvePanelTLSMaterials(settingService, target)
	if err != nil {
		return nil, nil, err
	}
	return materials, nil, nil
}

type PanelTLSRuntimeApplier interface {
	ApplyPanelTLSSettings(target PanelSelfSignedTarget) error
	DrainPanelTLSConnectionsByFingerprint(target PanelSelfSignedTarget, fingerprint string, gracePeriod time.Duration) error
}

var (
	panelTLSRuntimeMu      sync.RWMutex
	panelTLSRuntimeApplier PanelTLSRuntimeApplier
)

func RegisterPanelTLSRuntimeApplier(applier PanelTLSRuntimeApplier) {
	panelTLSRuntimeMu.Lock()
	panelTLSRuntimeApplier = applier
	panelTLSRuntimeMu.Unlock()
}

func ApplyPanelTLSRuntimeSettings(target PanelSelfSignedTarget) error {
	panelTLSRuntimeMu.RLock()
	applier := panelTLSRuntimeApplier
	panelTLSRuntimeMu.RUnlock()
	if applier == nil {
		return nil
	}
	return applier.ApplyPanelTLSSettings(target)
}

func DrainPanelTLSRuntimeConnectionsByFingerprint(target PanelSelfSignedTarget, fingerprint string, gracePeriod time.Duration) error {
	panelTLSRuntimeMu.RLock()
	applier := panelTLSRuntimeApplier
	panelTLSRuntimeMu.RUnlock()
	if applier == nil {
		return nil
	}
	return applier.DrainPanelTLSConnectionsByFingerprint(target, fingerprint, gracePeriod)
}

func nowUnix(now time.Time) int64 {
	if now.IsZero() {
		return time.Now().Unix()
	}
	return now.Unix()
}

func panelTargetLabel(target PanelSelfSignedTarget) string {
	if target == PanelSelfSignedTargetSub {
		return "\u8ba2\u9605\u5165\u53e3"
	}
	return "\u9762\u677f\u5165\u53e3"
}

func panelTargetFallbackDomain(target PanelSelfSignedTarget) string {
	if target == PanelSelfSignedTargetSub {
		return "sub.local"
	}
	return "panel.local"
}

func upsertInventoryFromPanelSQLite(target PanelSelfSignedTarget, entry *model.PanelCertificate) (*model.CertificateRecord, error) {
	if entry == nil {
		return nil, nil
	}
	sourceRef := "sqlite:" + strings.TrimSpace(string(target))
	mainDomain := panelTargetLabel(target)
	return certificateInventory.Upsert(CertificateUpsertPayload{
		SourceType: CertificateSourceSelfSigned,
		SourceRef:  sourceRef,

		MainDomain: mainDomain,
		Domains:    []string{mainDomain},

		AutoRenew: false,
		Remark:    "\u81ea\u7b7e\u8bc1\u4e66\uff08SQLite\uff09",

		CertPath:      "sqlite:" + strings.TrimSpace(string(target)) + "/fullchain.pem",
		KeyPath:       "sqlite:" + strings.TrimSpace(string(target)) + "/key.pem",
		FullchainPath: "sqlite:" + strings.TrimSpace(string(target)) + "/fullchain.pem",

		FullchainPEM: entry.CertPEM,
		CertPEM:      entry.CertPEM,
		KeyPEM:       entry.KeyPEM,

		Fingerprint:   entry.Fingerprint,
		NotBefore:     entry.NotBefore,
		NotAfter:      entry.NotAfter,
		LastIssuedAt:  entry.UpdatedAt,
		LastRenewedAt: entry.UpdatedAt,
	})
}

// MigrateLegacyPanelSQLiteCertificatesToInventory migrates certificates from
// legacy table panel_certificates into certificate inventory once, then removes
// legacy rows to prevent startup rehydration after user deletion.
func MigrateLegacyPanelSQLiteCertificatesToInventory() error {
	settingService := &SettingService{}
	migrateOne := func(target PanelSelfSignedTarget) error {
		entry, err := LoadPanelSQLiteCertificate(target)
		if err != nil {
			if database.IsNotFound(err) {
				return nil
			}
			return err
		}

		row, upsertErr := upsertInventoryFromPanelSQLite(target, entry)
		if upsertErr != nil {
			return upsertErr
		}
		if row != nil {
			assignedID, getErr := GetAssignedCertificateRecordID(settingService, target)
			if getErr != nil {
				return getErr
			}
			if assignedID == 0 {
				if setErr := SetAssignedCertificateRecordID(settingService, target, row.Id); setErr != nil {
					return setErr
				}
			}
		}

		return database.GetDB().Where("target = ?", string(target)).Delete(&model.PanelCertificate{}).Error
	}
	if err := migrateOne(PanelSelfSignedTargetPanel); err != nil {
		return err
	}
	if err := migrateOne(PanelSelfSignedTargetSub); err != nil {
		return err
	}
	return nil
}
