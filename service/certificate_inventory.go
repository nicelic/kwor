package service

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/util/common"
)

const (
	CertificateSourceACME       = "acme"
	CertificateSourceSelfSigned = "self_signed"
	CertificateSourceImported   = "imported"

	certificateDisplayIDMin       uint64 = 1
	certificateDisplayIDMax       uint64 = 100000000000
	certificateListDefaultPerPage        = 20
	certificateListMaxPerPage            = 100
	certificateMaterialMaxBytes          = 512 * 1024 * 1024
)

type CertificateRecordView struct {
	Id         uint   `json:"id"`
	DisplayID  uint64 `json:"displayId"`
	ResourceID string `json:"resourceId"`

	SourceType string `json:"sourceType"`
	SourceRef  string `json:"sourceRef"`

	MainDomain string   `json:"mainDomain"`
	Domains    []string `json:"domains"`

	CertificateType          string `json:"certificateType"`
	CertProfile              string `json:"certProfile"`
	Challenge                string `json:"challenge"`
	KeyLength                string `json:"keyLength"`
	IssuedKeyAlgorithm       string `json:"issuedKeyAlgorithm"`
	IssuedSignatureAlgorithm string `json:"issuedSignatureAlgorithm"`
	CAServer                 string `json:"caServer"`
	UseECC                   bool   `json:"useEcc"`
	AutoRenew                bool   `json:"autoRenew"`
	AutoRenewRetryPhase      string `json:"autoRenewRetryPhase"`
	AutoRenewRetryCount      int    `json:"autoRenewRetryCount"`
	AutoRenewNextRetryAt     int64  `json:"autoRenewNextRetryAt"`
	AutoRenewLastAttemptAt   int64  `json:"autoRenewLastAttemptAt"`

	AcmeAccountID   uint              `json:"acmeAccountId"`
	AcmeAccountName string            `json:"acmeAccountName"`
	DNSAccountID    uint              `json:"dnsAccountId"`
	DNSAccountName  string            `json:"dnsAccountName"`
	ApplyTarget     string            `json:"applyTarget"`
	PushEnabled     bool              `json:"pushEnabled"`
	PushDir         string            `json:"pushDir"`
	PushFilePaths   map[string]string `json:"pushFilePaths"`
	PushFiles       string            `json:"pushFiles"`
	Remark          string            `json:"remark"`
	RenewConfig     string            `json:"renewConfig"`

	AcmeHome    string `json:"acmeHome"`
	Webroot     string `json:"webroot"`
	DNSProvider string `json:"dnsProvider"`
	DNSEnvText  string `json:"dnsEnvText"`
	CustomArgs  string `json:"customArgs"`

	CertPath      string `json:"certPath"`
	KeyPath       string `json:"keyPath"`
	FullchainPath string `json:"fullchainPath"`
	ChainPath     string `json:"chainPath"`

	Fingerprint string `json:"fingerprint"`
	NotBefore   int64  `json:"notBefore"`
	NotAfter    int64  `json:"notAfter"`

	LastIssuedAt    int64  `json:"lastIssuedAt"`
	LastRenewedAt   int64  `json:"lastRenewedAt"`
	ListOrderAt     int64  `json:"listOrderAt"`
	UpdatedAt       int64  `json:"updatedAt"`
	CreatedAt       int64  `json:"createdAt"`
	LastError       string `json:"lastError"`
	PostActionError string `json:"postActionError"`
	Status          string `json:"status"`
	InUseByPanel    bool   `json:"inUseByPanel"`
	InUseBySub      bool   `json:"inUseBySub"`
	InUseByTLS      bool   `json:"inUseByTls"`
	InUseByMihomo   bool   `json:"inUseByMihomo"`
	UsageLabel      string `json:"usageLabel"`
	// DeleteBlocked is kept for API compatibility. Certificate deletion now
	// detaches active consumers atomically, so a usage reference is informative
	// rather than a hard delete block.
	DeleteBlocked bool `json:"deleteBlocked"`
}

type CertificateMaterialView struct {
	Id uint `json:"id"`

	MainDomain string `json:"mainDomain"`
	SourceType string `json:"sourceType"`
	SourceRef  string `json:"sourceRef"`

	CertPath      string `json:"certPath"`
	KeyPath       string `json:"keyPath"`
	FullchainPath string `json:"fullchainPath"`
	ChainPath     string `json:"chainPath"`

	CertPEM                  string `json:"certPem"`
	KeyPEM                   string `json:"keyPem"`
	FullchainPEM             string `json:"fullchainPem"`
	ChainPEM                 string `json:"chainPem"`
	Fingerprint              string `json:"fingerprint"`
	IssuedKeyAlgorithm       string `json:"issuedKeyAlgorithm"`
	IssuedSignatureAlgorithm string `json:"issuedSignatureAlgorithm"`
}

type CertificateListResult struct {
	Items              []CertificateRecordView `json:"items"`
	Page               int                     `json:"page"`
	PerPage            int                     `json:"perPage"`
	Total              int64                   `json:"total"`
	HasMore            bool                    `json:"hasMore"`
	PanelAssignedCount int                     `json:"panelAssignedCount"`
	SubAssignedCount   int                     `json:"subAssignedCount"`
}

// TLSCertificateOption is the compact certificate selector model used by the
// TLS editor. It deliberately avoids usage aggregation and certificate
// material so opening the selector stays proportional to the number of rows.
type TLSCertificateOption struct {
	ID          uint     `json:"id"`
	DisplayID   uint64   `json:"displayId"`
	ListOrderAt int64    `json:"listOrderAt"`
	SourceType  string   `json:"sourceType"`
	MainDomain  string   `json:"mainDomain"`
	Domains     []string `json:"domains"`
	Status      string   `json:"status"`
}

// CertificateRecordLogView is loaded only when the operator explicitly opens
// a certificate's historical issue or renewal output.
type CertificateRecordLogView struct {
	Id              uint   `json:"id"`
	MainDomain      string `json:"mainDomain"`
	LastError       string `json:"lastError"`
	PostActionError string `json:"postActionError"`
	LastOutput      string `json:"lastOutput"`
}

type CertificateInventoryService struct{}

type CertificateUpsertPayload struct {
	SourceType string
	SourceRef  string

	MainDomain string
	Domains    []string

	CertificateType string
	CertProfile     string
	Challenge       string
	KeyLength       string
	CAServer        string
	UseECC          bool
	AutoRenew       bool

	AcmeAccountID      uint
	AcmeAccountName    string
	DNSAccountID       uint
	DNSAccountName     string
	AcmeRuntimeProfile string
	ApplyTarget        string
	PushEnabled        bool
	PushDir            string
	PushFilePaths      string
	PushFiles          string
	// PushStateProvided avoids overwriting a verified directory-push state while
	// an issuance or renewal updates the certificate material.
	PushStateProvided bool
	Remark            string
	RenewConfig       string

	AcmeHome    string
	Webroot     string
	DNSProvider string
	DNSEnvText  string
	CustomArgs  string

	CertPath      string
	KeyPath       string
	FullchainPath string
	ChainPath     string

	CertPEM      []byte
	KeyPEM       []byte
	FullchainPEM []byte
	ChainPEM     []byte

	Fingerprint string
	NotBefore   int64
	NotAfter    int64

	LastIssuedAt    int64
	LastRenewedAt   int64
	LastError       string
	PostActionError string
	LastOutput      string
	ListOrderAt     int64
}

func (s *CertificateInventoryService) List() ([]CertificateRecordView, error) {
	rows := make([]model.CertificateRecord, 0)
	if err := database.GetDB().
		Select(certificateRecordListProjectionColumns()).
		Order("list_order_at DESC, id DESC").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	return buildCertificateRecordViews(rows)
}

func (s *CertificateInventoryService) ListTLSOptions() ([]TLSCertificateOption, error) {
	rows := make([]model.CertificateRecord, 0)
	if err := database.GetDB().
		Select("id", "display_id", "list_order_at", "source_type", "main_domain", "domain_set", "last_error", "not_after").
		Order("list_order_at DESC, id DESC").
		Find(&rows).Error; err != nil {
		return nil, err
	}

	result := make([]TLSCertificateOption, 0, len(rows))
	for i := range rows {
		row := &rows[i]
		domains := decodeCertificateDomains(row.DomainSet)
		if len(domains) == 0 && strings.TrimSpace(row.MainDomain) != "" {
			domains = []string{strings.TrimSpace(row.MainDomain)}
		}
		result = append(result, TLSCertificateOption{
			ID:          row.Id,
			DisplayID:   row.DisplayID,
			ListOrderAt: row.ListOrderAt,
			SourceType:  strings.TrimSpace(row.SourceType),
			MainDomain:  strings.TrimSpace(row.MainDomain),
			Domains:     domains,
			Status:      certificateStatus(row),
		})
	}
	return result, nil
}

func (s *CertificateInventoryService) ListPage(page int, perPage int, search string) (*CertificateListResult, error) {
	if page < 1 {
		page = 1
	}
	if perPage <= 0 {
		perPage = certificateListDefaultPerPage
	}
	if perPage > certificateListMaxPerPage {
		perPage = certificateListMaxPerPage
	}

	db := database.GetDB().Model(&model.CertificateRecord{})
	search = strings.ToLower(strings.TrimSpace(search))
	if search != "" {
		like := "%" + search + "%"
		db = db.Where(
			"LOWER(main_domain) LIKE ? OR LOWER(domain_set) LIKE ? OR LOWER(acme_account_name) LIKE ? OR LOWER(dns_account_name) LIKE ? OR LOWER(remark) LIKE ? OR LOWER(ca_server) LIKE ? OR LOWER(challenge) LIKE ? OR CAST(display_id AS TEXT) LIKE ?",
			like, like, like, like, like, like, like, like,
		)
	}

	total := int64(0)
	if err := db.Count(&total).Error; err != nil {
		return nil, err
	}
	if total > 0 && int64(page-1)*int64(perPage) >= total {
		page = int((total-1)/int64(perPage)) + 1
	}

	rows := make([]model.CertificateRecord, 0, perPage)
	if err := db.Select(certificateRecordListProjectionColumns()).
		Order("list_order_at DESC, id DESC").
		Limit(perPage).
		Offset((page - 1) * perPage).
		Find(&rows).Error; err != nil {
		return nil, err
	}
	items, err := buildCertificateRecordViews(rows)
	if err != nil {
		return nil, err
	}

	settingService := &SettingService{}
	panelAssignedIDs, err := readAssignedCertificateRecordIDSet(settingService, PanelSelfSignedTargetPanel)
	if err != nil {
		return nil, err
	}
	subAssignedIDs, err := readAssignedCertificateRecordIDSet(settingService, PanelSelfSignedTargetSub)
	if err != nil {
		return nil, err
	}
	return &CertificateListResult{
		Items:              items,
		Page:               page,
		PerPage:            perPage,
		Total:              total,
		HasMore:            int64(page*perPage) < total,
		PanelAssignedCount: len(panelAssignedIDs),
		SubAssignedCount:   len(subAssignedIDs),
	}, nil
}

func buildCertificateRecordViews(rows []model.CertificateRecord) ([]CertificateRecordView, error) {
	if err := hydrateCertificateAccountNames(rows); err != nil {
		return nil, err
	}
	usageSnapshot, err := collectCertificateUsageSnapshot(rows)
	if err != nil {
		return nil, err
	}
	result := make([]CertificateRecordView, 0, len(rows))
	for i := range rows {
		result = append(result, convertCertificateRecordWithUsage(&rows[i], usageSnapshot))
	}
	return result, nil
}

func certificateRecordListProjectionColumns() []string {
	return []string{
		"id", "display_id", "list_order_at",
		"source_type", "source_ref", "main_domain", "domain_set",
		"certificate_type", "cert_profile", "challenge", "key_length",
		"issued_key_algorithm", "issued_signature_algorithm", "ca_server", "use_ecc", "auto_renew",
		"auto_renew_retry_phase", "auto_renew_retry_count", "auto_renew_next_retry_at", "auto_renew_last_attempt_at",
		"acme_account_id", "acme_account_name", "dns_account_id", "dns_account_name",
		"apply_target", "push_enabled", "push_dir", "push_file_paths", "push_files", "remark", "renew_config",
		"webroot", "dns_provider", "custom_args",
		"fingerprint", "not_before", "not_after",
		"last_issued_at", "last_renewed_at", "last_error", "post_action_error",
		"created_at", "updated_at",
	}
}

// hydrateCertificateAccountNames keeps the UI label in sync with an account
// rename while the permanent account IDs remain the only relation source.
func hydrateCertificateAccountNames(rows []model.CertificateRecord) error {
	if len(rows) == 0 {
		return nil
	}
	acmeIDs := make([]uint, 0)
	dnsIDs := make([]uint, 0)
	seenAcme := map[uint]struct{}{}
	seenDNS := map[uint]struct{}{}
	for i := range rows {
		if rows[i].AcmeAccountID > 0 {
			if _, exists := seenAcme[rows[i].AcmeAccountID]; !exists {
				seenAcme[rows[i].AcmeAccountID] = struct{}{}
				acmeIDs = append(acmeIDs, rows[i].AcmeAccountID)
			}
		}
		if rows[i].DNSAccountID > 0 {
			if _, exists := seenDNS[rows[i].DNSAccountID]; !exists {
				seenDNS[rows[i].DNSAccountID] = struct{}{}
				dnsIDs = append(dnsIDs, rows[i].DNSAccountID)
			}
		}
	}
	acmeNames := map[uint]string{}
	if len(acmeIDs) > 0 {
		accounts := make([]model.AcmeAccount, 0)
		if err := database.GetDB().Select("id", "name").Where("id IN ? AND system = ?", acmeIDs, false).Find(&accounts).Error; err != nil {
			return err
		}
		for i := range accounts {
			acmeNames[accounts[i].Id] = strings.TrimSpace(accounts[i].Name)
		}
	}
	dnsNames := map[uint]string{}
	if len(dnsIDs) > 0 {
		accounts := make([]model.AcmeDNSAccount, 0)
		if err := database.GetDB().Select("id", "name").Where("id IN ?", dnsIDs).Find(&accounts).Error; err != nil {
			return err
		}
		for i := range accounts {
			dnsNames[accounts[i].Id] = strings.TrimSpace(accounts[i].Name)
		}
	}
	for i := range rows {
		if name, exists := acmeNames[rows[i].AcmeAccountID]; exists {
			rows[i].AcmeAccountName = name
		}
		if name, exists := dnsNames[rows[i].DNSAccountID]; exists {
			rows[i].DNSAccountName = name
		}
	}
	return nil
}

func (s *CertificateInventoryService) GetMaterial(id uint) (*CertificateMaterialView, error) {
	if id == 0 {
		return nil, common.NewError("certificate id is required")
	}
	lengths := struct {
		CertPEMLength      int `gorm:"column:cert_pem_length"`
		KeyPEMLength       int `gorm:"column:key_pem_length"`
		FullchainPEMLength int `gorm:"column:fullchain_pem_length"`
		ChainPEMLength     int `gorm:"column:chain_pem_length"`
	}{}
	if err := database.GetDB().Model(&model.CertificateRecord{}).
		Select("COALESCE(length(cert_pem), 0) AS cert_pem_length", "COALESCE(length(key_pem), 0) AS key_pem_length", "COALESCE(length(fullchain_pem), 0) AS fullchain_pem_length", "COALESCE(length(chain_pem), 0) AS chain_pem_length").
		Where("id = ?", id).
		First(&lengths).Error; err != nil {
		return nil, err
	}
	if err := validateCertificateMaterialTotal(lengths.CertPEMLength, lengths.KeyPEMLength, lengths.FullchainPEMLength, lengths.ChainPEMLength); err != nil {
		return nil, err
	}
	row := &model.CertificateRecord{}
	if err := database.GetDB().Where("id = ?", id).First(row).Error; err != nil {
		return nil, err
	}
	issuedKeyAlgorithm, issuedSignatureAlgorithm := certificateIssuedAlgorithms(row)
	return &CertificateMaterialView{
		Id:                       row.Id,
		MainDomain:               strings.TrimSpace(row.MainDomain),
		SourceType:               strings.TrimSpace(row.SourceType),
		SourceRef:                strings.TrimSpace(row.SourceRef),
		CertPath:                 strings.TrimSpace(row.CertPath),
		KeyPath:                  strings.TrimSpace(row.KeyPath),
		FullchainPath:            strings.TrimSpace(row.FullchainPath),
		ChainPath:                strings.TrimSpace(row.ChainPath),
		CertPEM:                  strings.TrimSpace(string(row.CertPEM)),
		KeyPEM:                   strings.TrimSpace(string(row.KeyPEM)),
		FullchainPEM:             strings.TrimSpace(string(row.FullchainPEM)),
		ChainPEM:                 strings.TrimSpace(string(row.ChainPEM)),
		Fingerprint:              strings.TrimSpace(row.Fingerprint),
		IssuedKeyAlgorithm:       issuedKeyAlgorithm,
		IssuedSignatureAlgorithm: issuedSignatureAlgorithm,
	}, nil
}

func (s *CertificateInventoryService) GetLog(id uint) (*CertificateRecordLogView, error) {
	if id == 0 {
		return nil, common.NewError("certificate id is required")
	}
	row := &model.CertificateRecord{}
	if err := database.GetDB().Select("id", "main_domain", "last_error", "post_action_error", "last_output").Where("id = ?", id).First(row).Error; err != nil {
		return nil, err
	}
	return &CertificateRecordLogView{
		Id:              row.Id,
		MainDomain:      strings.TrimSpace(row.MainDomain),
		LastError:       strings.TrimSpace(row.LastError),
		PostActionError: strings.TrimSpace(row.PostActionError),
		LastOutput:      truncateAcmeStoredOutput(row.LastOutput),
	}, nil
}

func (s *CertificateInventoryService) GetRecordByID(id uint) (*model.CertificateRecord, error) {
	if id == 0 {
		return nil, common.NewError("certificate id is required")
	}
	row := &model.CertificateRecord{}
	if err := database.GetDB().Where("id = ?", id).First(row).Error; err != nil {
		return nil, err
	}
	return row, nil
}

func (s *CertificateInventoryService) DeleteByID(id uint) error {
	if id == 0 {
		return common.NewError("certificate id is required")
	}
	result := database.GetDB().Where("id = ?", id).Delete(&model.CertificateRecord{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected > 0 {
		noteReverseProxyCertificateInventoryChanged()
	}
	return nil
}

func (s *CertificateInventoryService) DeleteBySource(sourceType string, sourceRef string) error {
	sourceType = strings.TrimSpace(strings.ToLower(sourceType))
	sourceRef = strings.TrimSpace(sourceRef)
	if sourceType == "" || sourceRef == "" {
		return nil
	}
	result := database.GetDB().
		Where("source_type = ? AND source_ref = ?", sourceType, sourceRef).
		Delete(&model.CertificateRecord{})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected > 0 {
		noteReverseProxyCertificateInventoryChanged()
	}
	return nil
}

func (s *CertificateInventoryService) Upsert(payload CertificateUpsertPayload) (*model.CertificateRecord, error) {
	sourceType := strings.TrimSpace(strings.ToLower(payload.SourceType))
	sourceRef := strings.TrimSpace(payload.SourceRef)
	if sourceType == "" {
		return nil, common.NewError("source type is required")
	}
	if sourceRef == "" {
		return nil, common.NewError("source ref is required")
	}
	if err := validateCertificateMaterialTotal(len(payload.CertPEM), len(payload.KeyPEM), len(payload.FullchainPEM), len(payload.ChainPEM)); err != nil {
		return nil, err
	}

	mainDomain := strings.TrimSpace(payload.MainDomain)
	domains := normalizeCertificateDomains(payload.Domains, mainDomain)
	if mainDomain == "" && len(domains) > 0 {
		mainDomain = domains[0]
	}
	if mainDomain == "" {
		mainDomain = "unknown"
	}
	if len(domains) == 0 {
		domains = []string{mainDomain}
	}

	domainJSON, err := json.Marshal(domains)
	if err != nil {
		return nil, err
	}

	now := time.Now().Unix()
	entry := &model.CertificateRecord{}
	db := database.GetDB()
	findErr := db.Where("source_type = ? AND source_ref = ?", sourceType, sourceRef).First(entry).Error
	isNewEntry := false
	if findErr != nil {
		if !database.IsNotFound(findErr) {
			return nil, findErr
		}
		entry = &model.CertificateRecord{}
		isNewEntry = true
	}

	entry.SourceType = sourceType
	entry.SourceRef = sourceRef
	entry.MainDomain = mainDomain
	entry.DomainSet = string(domainJSON)

	entry.CertificateType = strings.TrimSpace(payload.CertificateType)
	if entry.CertificateType == "" {
		entry.CertificateType = "domain"
	}
	entry.CertProfile = strings.TrimSpace(payload.CertProfile)
	entry.Challenge = strings.TrimSpace(payload.Challenge)
	entry.KeyLength = strings.TrimSpace(payload.KeyLength)
	entry.CAServer = strings.TrimSpace(payload.CAServer)
	entry.UseECC = payload.UseECC
	entry.AutoRenew = payload.AutoRenew
	entry.AutoRenewRetryPhase = ""
	entry.AutoRenewRetryCount = 0
	entry.AutoRenewNextRetryAt = 0

	entry.AcmeAccountID = payload.AcmeAccountID
	entry.AcmeAccountName = strings.TrimSpace(payload.AcmeAccountName)
	entry.DNSAccountID = payload.DNSAccountID
	entry.DNSAccountName = strings.TrimSpace(payload.DNSAccountName)
	entry.AcmeRuntimeProfile = strings.TrimSpace(payload.AcmeRuntimeProfile)
	entry.ApplyTarget = strings.TrimSpace(payload.ApplyTarget)
	if payload.PushStateProvided {
		entry.PushEnabled = payload.PushEnabled
		entry.PushDir = strings.TrimSpace(payload.PushDir)
		entry.PushFilePaths = strings.TrimSpace(payload.PushFilePaths)
		entry.PushFiles = strings.TrimSpace(payload.PushFiles)
	}
	entry.Remark = strings.TrimSpace(payload.Remark)
	entry.RenewConfig = strings.TrimSpace(payload.RenewConfig)

	entry.AcmeHome = strings.TrimSpace(payload.AcmeHome)
	entry.Webroot = strings.TrimSpace(payload.Webroot)
	entry.DNSProvider = strings.TrimSpace(payload.DNSProvider)
	entry.DNSEnvText = strings.TrimSpace(payload.DNSEnvText)
	entry.CustomArgs = strings.TrimSpace(payload.CustomArgs)

	entry.CertPath = strings.TrimSpace(payload.CertPath)
	entry.KeyPath = strings.TrimSpace(payload.KeyPath)
	entry.FullchainPath = strings.TrimSpace(payload.FullchainPath)
	entry.ChainPath = strings.TrimSpace(payload.ChainPath)

	entry.CertPEM = append([]byte(nil), payload.CertPEM...)
	entry.KeyPEM = append([]byte(nil), payload.KeyPEM...)
	entry.FullchainPEM = append([]byte(nil), payload.FullchainPEM...)
	entry.ChainPEM = append([]byte(nil), payload.ChainPEM...)
	entry.IssuedKeyAlgorithm, entry.IssuedSignatureAlgorithm = inspectIssuedCertificateAlgorithms(entry.FullchainPEM)

	entry.Fingerprint = strings.TrimSpace(payload.Fingerprint)
	entry.NotBefore = payload.NotBefore
	entry.NotAfter = payload.NotAfter

	if payload.LastIssuedAt > 0 {
		entry.LastIssuedAt = payload.LastIssuedAt
	} else if entry.LastIssuedAt == 0 {
		entry.LastIssuedAt = now
	}
	if payload.LastRenewedAt > 0 {
		entry.LastRenewedAt = payload.LastRenewedAt
	} else if entry.LastRenewedAt == 0 {
		entry.LastRenewedAt = now
	}

	entry.LastError = strings.TrimSpace(payload.LastError)
	entry.PostActionError = strings.TrimSpace(payload.PostActionError)
	entry.LastOutput = truncateAcmeStoredOutput(payload.LastOutput)
	if entry.ListOrderAt <= 0 {
		entry.ListOrderAt = payload.ListOrderAt
	}
	if entry.ListOrderAt <= 0 {
		entry.ListOrderAt = now
	}
	if entry.DisplayID == 0 {
		nextDisplayID, nextErr := s.allocateNextDisplayID()
		if nextErr != nil {
			return nil, nextErr
		}
		entry.DisplayID = nextDisplayID
	}
	if isNewEntry && entry.ListOrderAt <= 0 {
		entry.ListOrderAt = now
	}

	if err := db.Save(entry).Error; err != nil {
		return nil, err
	}
	noteReverseProxyCertificateInventoryChanged()
	return entry, nil
}

func validateCertificateMaterialTotal(lengths ...int) error {
	total := 0
	for _, length := range lengths {
		if length <= 0 {
			continue
		}
		if total > certificateMaterialMaxBytes-length {
			return common.NewError("证书材料总大小超过 512 MiB 限制")
		}
		total += length
	}
	return nil
}

func (s *CertificateInventoryService) RepairDisplayIDs() error {
	db := database.GetDB()
	rows := make([]model.CertificateRecord, 0)
	if err := db.Select("id", "display_id", "list_order_at", "created_at", "last_issued_at", "not_before", "updated_at").Order("id ASC").Find(&rows).Error; err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}

	needsRepair := false
	usedDisplayIDs := make(map[uint64]struct{}, len(rows))
	for i := range rows {
		row := &rows[i]
		if row.DisplayID == 0 {
			needsRepair = true
		} else {
			if _, exists := usedDisplayIDs[row.DisplayID]; exists {
				needsRepair = true
			}
			usedDisplayIDs[row.DisplayID] = struct{}{}
		}
		if row.ListOrderAt <= 0 {
			needsRepair = true
		}
	}
	if !needsRepair {
		return nil
	}

	sort.Slice(rows, func(i, j int) bool {
		ti := certificateRecordListOrderSeed(&rows[i])
		tj := certificateRecordListOrderSeed(&rows[j])
		if ti == tj {
			return rows[i].Id < rows[j].Id
		}
		return ti < tj
	})

	usedDisplayIDs = make(map[uint64]struct{}, len(rows))
	for i := range rows {
		row := &rows[i]
		if row.DisplayID > 0 && row.DisplayID >= certificateDisplayIDMin && row.DisplayID <= certificateDisplayIDMax {
			if _, exists := usedDisplayIDs[row.DisplayID]; !exists {
				usedDisplayIDs[row.DisplayID] = struct{}{}
			} else {
				row.DisplayID = 0
			}
		} else {
			row.DisplayID = 0
		}
		if row.ListOrderAt <= 0 {
			row.ListOrderAt = certificateRecordListOrderSeed(row)
		}
	}

	for i := range rows {
		row := &rows[i]
		if row.DisplayID != 0 {
			continue
		}
		next, err := allocateDisplayIDFromUsedSet(usedDisplayIDs)
		if err != nil {
			return err
		}
		row.DisplayID = next
		usedDisplayIDs[next] = struct{}{}
	}

	for i := range rows {
		row := &rows[i]
		if err := db.Model(&model.CertificateRecord{}).
			Where("id = ?", row.Id).
			Updates(map[string]interface{}{
				"display_id":    row.DisplayID,
				"list_order_at": row.ListOrderAt,
			}).Error; err != nil {
			return err
		}
	}
	return nil
}

func (s *CertificateInventoryService) allocateNextDisplayID() (uint64, error) {
	rows := make([]model.CertificateRecord, 0)
	if err := database.GetDB().
		Where("display_id > 0").
		Order("display_id ASC").
		Find(&rows).Error; err != nil {
		return 0, err
	}
	used := make(map[uint64]struct{}, len(rows))
	for i := range rows {
		used[rows[i].DisplayID] = struct{}{}
	}
	return allocateDisplayIDFromUsedSet(used)
}

func allocateDisplayIDFromUsedSet(used map[uint64]struct{}) (uint64, error) {
	if used == nil {
		used = map[uint64]struct{}{}
	}
	for candidate := certificateDisplayIDMin; candidate <= certificateDisplayIDMax; candidate++ {
		if _, exists := used[candidate]; exists {
			continue
		}
		return candidate, nil
	}
	return 0, common.NewError("certificate display id exhausted")
}

func certificateRecordListOrderSeed(row *model.CertificateRecord) int64 {
	if row == nil {
		return time.Now().Unix()
	}
	if !row.CreatedAt.IsZero() {
		return row.CreatedAt.Unix()
	}
	if row.LastIssuedAt > 0 {
		return row.LastIssuedAt
	}
	if row.NotBefore > 0 {
		return row.NotBefore
	}
	if !row.UpdatedAt.IsZero() {
		return row.UpdatedAt.Unix()
	}
	return int64(row.Id)
}

func normalizeCertificateDomains(raw []string, fallbackMain string) []string {
	seen := map[string]struct{}{}
	result := make([]string, 0, len(raw)+1)
	add := func(value string) {
		value = strings.TrimSpace(strings.ToLower(value))
		value = strings.Trim(value, ".")
		if value == "" {
			return
		}
		if _, ok := seen[value]; ok {
			return
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	add(fallbackMain)
	for _, item := range raw {
		add(item)
	}
	return result
}

func decodeCertificateDomains(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []string{}
	}
	result := make([]string, 0)
	if err := json.Unmarshal([]byte(raw), &result); err != nil {
		return []string{}
	}
	return normalizeCertificateDomains(result, "")
}

func certificateRecordTypeOrDefault(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "domain"
	}
	return value
}

func certificateStatus(row *model.CertificateRecord) string {
	if row == nil {
		return "unknown"
	}
	if strings.TrimSpace(row.LastError) != "" {
		return "error"
	}
	nowUnix := time.Now().Unix()
	if row.NotAfter > 0 && row.NotAfter <= nowUnix {
		return "expired"
	}
	return "normal"
}

func convertCertificateRecord(entry *model.CertificateRecord) CertificateRecordView {
	snapshot := certificateUsageSnapshot{
		panelAssignedIDs: map[uint]struct{}{},
		subAssignedIDs:   map[uint]struct{}{},
		tlsUsageByID:     map[uint]certificateTLSUsage{},
		reverseUsageByID: map[uint]certificateReverseProxyUsage{},
	}
	if entry != nil && entry.Id > 0 {
		if certificateAssignedRecordMatches(PanelSelfSignedTargetPanel, entry.Id) {
			snapshot.panelAssignedIDs[entry.Id] = struct{}{}
		}
		if certificateAssignedRecordMatches(PanelSelfSignedTargetSub, entry.Id) {
			snapshot.subAssignedIDs[entry.Id] = struct{}{}
		}
		snapshot.tlsUsageByID[entry.Id] = collectCertificateTLSUsage(entry.Id)
		snapshot.reverseUsageByID[entry.Id] = collectCertificateReverseProxyUsage(entry.Id)
	}
	return convertCertificateRecordWithUsage(entry, snapshot)
}

func convertCertificateRecordWithUsage(entry *model.CertificateRecord, snapshot certificateUsageSnapshot) CertificateRecordView {
	if entry == nil {
		return CertificateRecordView{}
	}
	_, inUseByPanel := snapshot.panelAssignedIDs[entry.Id]
	_, inUseBySub := snapshot.subAssignedIDs[entry.Id]
	tlsUsage := snapshot.tlsUsageByID[entry.Id]
	reverseProxyUsage := snapshot.reverseUsageByID[entry.Id]
	inUseByTLS := len(tlsUsage.DefaultTLSNames) > 0
	inUseByMihomo := len(tlsUsage.MihomoTLSNames) > 0
	usageLabel := buildCertificateUsageLabel(inUseByPanel, inUseBySub, tlsUsage, reverseProxyUsage)
	applyTarget := strings.TrimSpace(entry.ApplyTarget)
	switch {
	case inUseByPanel && inUseBySub:
		applyTarget = "panel,sub"
	case inUseByPanel:
		applyTarget = "panel"
	case inUseBySub:
		applyTarget = "sub"
	}
	domains := decodeCertificateDomains(entry.DomainSet)
	if len(domains) == 0 && strings.TrimSpace(entry.MainDomain) != "" {
		domains = []string{strings.TrimSpace(entry.MainDomain)}
	}
	issuedKeyAlgorithm, issuedSignatureAlgorithm := certificateIssuedAlgorithms(entry)
	pushDir := ""
	pushFilePaths := map[string]string{}
	if entry.PushEnabled {
		pushDir = strings.TrimSpace(entry.PushDir)
		pushFilePaths = decodeCertificatePushFilePaths(entry.PushFilePaths)
	}
	return CertificateRecordView{
		Id:         entry.Id,
		DisplayID:  entry.DisplayID,
		ResourceID: certificateResourceID(entry.DisplayID),

		SourceType: strings.TrimSpace(entry.SourceType),
		SourceRef:  strings.TrimSpace(entry.SourceRef),

		MainDomain: strings.TrimSpace(entry.MainDomain),
		Domains:    domains,

		CertificateType:          certificateRecordTypeOrDefault(entry.CertificateType),
		CertProfile:              strings.TrimSpace(entry.CertProfile),
		Challenge:                strings.TrimSpace(entry.Challenge),
		KeyLength:                strings.TrimSpace(entry.KeyLength),
		IssuedKeyAlgorithm:       issuedKeyAlgorithm,
		IssuedSignatureAlgorithm: issuedSignatureAlgorithm,
		CAServer:                 strings.TrimSpace(entry.CAServer),
		UseECC:                   entry.UseECC,
		AutoRenew:                entry.AutoRenew,
		AutoRenewRetryPhase:      strings.TrimSpace(entry.AutoRenewRetryPhase),
		AutoRenewRetryCount:      entry.AutoRenewRetryCount,
		AutoRenewNextRetryAt:     entry.AutoRenewNextRetryAt,
		AutoRenewLastAttemptAt:   entry.AutoRenewLastAttemptAt,

		AcmeAccountID:   entry.AcmeAccountID,
		AcmeAccountName: strings.TrimSpace(entry.AcmeAccountName),
		DNSAccountID:    entry.DNSAccountID,
		DNSAccountName:  strings.TrimSpace(entry.DNSAccountName),
		ApplyTarget:     applyTarget,
		PushEnabled:     entry.PushEnabled,
		PushDir:         pushDir,
		PushFilePaths:   pushFilePaths,
		PushFiles:       strings.TrimSpace(entry.PushFiles),
		Remark:          mergeCertificateRemark(strings.TrimSpace(entry.Remark), inUseByPanel, inUseBySub, tlsUsage, reverseProxyUsage),
		RenewConfig:     strings.TrimSpace(entry.RenewConfig),

		// Runtime paths and ad-hoc DNS variables are implementation details and
		// may contain legacy sensitive data. Certificate records retain only the
		// challenge metadata needed for an explicit reissue.
		AcmeHome:    "",
		Webroot:     strings.TrimSpace(entry.Webroot),
		DNSProvider: strings.TrimSpace(entry.DNSProvider),
		DNSEnvText:  "",
		CustomArgs:  strings.TrimSpace(entry.CustomArgs),

		CertPath:      "",
		KeyPath:       "",
		FullchainPath: "",
		ChainPath:     "",

		Fingerprint: strings.TrimSpace(entry.Fingerprint),
		NotBefore:   entry.NotBefore,
		NotAfter:    entry.NotAfter,

		LastIssuedAt:    entry.LastIssuedAt,
		LastRenewedAt:   entry.LastRenewedAt,
		ListOrderAt:     entry.ListOrderAt,
		UpdatedAt:       entry.UpdatedAt.Unix(),
		CreatedAt:       entry.CreatedAt.Unix(),
		LastError:       strings.TrimSpace(entry.LastError),
		PostActionError: strings.TrimSpace(entry.PostActionError),
		Status:          certificateStatus(entry),
		InUseByPanel:    inUseByPanel,
		InUseBySub:      inUseBySub,
		InUseByTLS:      inUseByTLS,
		InUseByMihomo:   inUseByMihomo,
		UsageLabel:      usageLabel,
		DeleteBlocked:   false,
	}
}

func inspectIssuedCertificateAlgorithms(fullchainPEM []byte) (string, string) {
	if len(fullchainPEM) == 0 {
		return "", ""
	}
	algorithmInfo, err := (&ServerService{}).DetectTLSCertificateAlgorithm("pem", "", string(fullchainPEM))
	if err != nil {
		return "", ""
	}
	return strings.TrimSpace(algorithmInfo["key_algorithm"]), strings.TrimSpace(algorithmInfo["signature_algorithm"])
}

func certificateIssuedAlgorithms(entry *model.CertificateRecord) (string, string) {
	if entry == nil {
		return "", ""
	}
	keyAlgorithm := strings.TrimSpace(entry.IssuedKeyAlgorithm)
	signatureAlgorithm := strings.TrimSpace(entry.IssuedSignatureAlgorithm)
	if (keyAlgorithm == "" || signatureAlgorithm == "") && len(entry.FullchainPEM) > 0 {
		parsedKey, parsedSignature := inspectIssuedCertificateAlgorithms(entry.FullchainPEM)
		if keyAlgorithm == "" {
			keyAlgorithm = parsedKey
		}
		if signatureAlgorithm == "" {
			signatureAlgorithm = parsedSignature
		}
	}
	return keyAlgorithm, signatureAlgorithm
}

// BackfillIssuedAlgorithms updates one bounded startup batch of legacy records.
// It intentionally loads PEM only here, never in list or selector queries.
func (s *CertificateInventoryService) BackfillIssuedAlgorithms(limit int) error {
	if limit <= 0 {
		limit = 100
	}
	db := database.GetDB()
	if db == nil {
		return nil
	}
	rows := make([]model.CertificateRecord, 0, limit)
	if err := db.Select("id", "fullchain_pem", "issued_key_algorithm", "issued_signature_algorithm").
		Where("(issued_key_algorithm = '' OR issued_signature_algorithm = '') AND length(fullchain_pem) > 0").
		Order("id ASC").
		Limit(limit).
		Find(&rows).Error; err != nil {
		return err
	}
	for i := range rows {
		keyAlgorithm, signatureAlgorithm := certificateIssuedAlgorithms(&rows[i])
		if keyAlgorithm == "" && signatureAlgorithm == "" {
			continue
		}
		if err := db.Model(&model.CertificateRecord{}).Where("id = ?", rows[i].Id).Updates(map[string]interface{}{
			"issued_key_algorithm":       keyAlgorithm,
			"issued_signature_algorithm": signatureAlgorithm,
		}).Error; err != nil {
			return err
		}
	}
	return nil
}

func BuildImportedSourceRef(target PanelSelfSignedTarget) string {
	return fmt.Sprintf("settings:%s_path", strings.TrimSpace(string(target)))
}

type certificateTLSUsage struct {
	DefaultTLSNames []string
	MihomoTLSNames  []string
}

func (u certificateTLSUsage) inUse() bool {
	return len(u.DefaultTLSNames) > 0 || len(u.MihomoTLSNames) > 0
}

type certificateReverseProxyUsage struct {
	RuleNames []string
}

func (u certificateReverseProxyUsage) inUse() bool {
	return len(u.RuleNames) > 0
}

type certificateUsageSnapshot struct {
	panelAssignedIDs map[uint]struct{}
	subAssignedIDs   map[uint]struct{}
	tlsUsageByID     map[uint]certificateTLSUsage
	reverseUsageByID map[uint]certificateReverseProxyUsage
}

func collectCertificateUsageSnapshot(rows []model.CertificateRecord) (certificateUsageSnapshot, error) {
	snapshot := certificateUsageSnapshot{
		panelAssignedIDs: map[uint]struct{}{},
		subAssignedIDs:   map[uint]struct{}{},
		tlsUsageByID:     map[uint]certificateTLSUsage{},
		reverseUsageByID: map[uint]certificateReverseProxyUsage{},
	}
	if len(rows) == 0 {
		return snapshot, nil
	}

	recordIDs := make([]uint, 0, len(rows))
	recordIDSet := make(map[uint]struct{}, len(rows))
	for i := range rows {
		if rows[i].Id == 0 {
			continue
		}
		recordIDs = append(recordIDs, rows[i].Id)
		recordIDSet[rows[i].Id] = struct{}{}
	}
	if len(recordIDs) == 0 {
		return snapshot, nil
	}

	settingService := &SettingService{}
	panelAssignedIDs, err := readAssignedCertificateRecordIDSet(settingService, PanelSelfSignedTargetPanel)
	if err != nil {
		return snapshot, err
	}
	subAssignedIDs, err := readAssignedCertificateRecordIDSet(settingService, PanelSelfSignedTargetSub)
	if err != nil {
		return snapshot, err
	}
	snapshot.panelAssignedIDs = panelAssignedIDs
	snapshot.subAssignedIDs = subAssignedIDs

	db := database.GetDB()
	defaultRows := make([]model.Tls, 0)
	if err := db.Select("id", "name", "certificate_record_id").Where("certificate_record_id IN ?", recordIDs).Find(&defaultRows).Error; err != nil {
		return snapshot, err
	}
	for i := range defaultRows {
		row := defaultRows[i]
		usage := snapshot.tlsUsageByID[row.CertificateRecordID]
		name := strings.TrimSpace(row.Name)
		if name == "" {
			name = fmt.Sprintf("#%d", row.Id)
		}
		usage.DefaultTLSNames = append(usage.DefaultTLSNames, name)
		snapshot.tlsUsageByID[row.CertificateRecordID] = usage
	}

	mihomoRows := make([]model.MihomoTls, 0)
	if err := db.Select("id", "name", "certificate_record_id").Where("certificate_record_id IN ?", recordIDs).Find(&mihomoRows).Error; err != nil {
		return snapshot, err
	}
	for i := range mihomoRows {
		row := mihomoRows[i]
		usage := snapshot.tlsUsageByID[row.CertificateRecordID]
		name := strings.TrimSpace(row.Name)
		if name == "" {
			name = fmt.Sprintf("#%d", row.Id)
		}
		usage.MihomoTLSNames = append(usage.MihomoTLSNames, name)
		snapshot.tlsUsageByID[row.CertificateRecordID] = usage
	}

	reverseRows := make([]model.ReverseProxyRule, 0)
	if err := db.Select("id", "name", "certificate_record_id", "certificate_record_list", "list_order").
		Order("list_order ASC, id ASC").
		Find(&reverseRows).Error; err != nil {
		return snapshot, err
	}
	seenNamesByID := make(map[uint]map[string]struct{})
	for i := range reverseRows {
		row := reverseRows[i]
		certIDs := reverseProxyRuleCertificateIDs(&row)
		if len(certIDs) == 0 {
			continue
		}
		name := strings.TrimSpace(row.Name)
		if name == "" {
			name = fmt.Sprintf("#%d", row.Id)
		}
		for _, certID := range certIDs {
			if _, ok := recordIDSet[certID]; !ok {
				continue
			}
			if seenNamesByID[certID] == nil {
				seenNamesByID[certID] = map[string]struct{}{}
			}
			if _, exists := seenNamesByID[certID][name]; exists {
				continue
			}
			seenNamesByID[certID][name] = struct{}{}
			usage := snapshot.reverseUsageByID[certID]
			usage.RuleNames = append(usage.RuleNames, name)
			snapshot.reverseUsageByID[certID] = usage
		}
	}

	return snapshot, nil
}

func readAssignedCertificateRecordIDSet(settingService *SettingService, target PanelSelfSignedTarget) (map[uint]struct{}, error) {
	result := map[uint]struct{}{}
	if settingService == nil {
		return result, nil
	}
	rawList, err := settingService.getString(panelAssignedCertificateRecordIDsKey(target))
	if err != nil {
		return nil, err
	}
	parsedFromList, _ := parseAssignedCertificateRecordIDs(rawList)
	filteredFromList, err := filterExistingCertificateRecordIDs(parsedFromList)
	if err != nil {
		return nil, err
	}
	resolved := filteredFromList
	if len(resolved) == 0 {
		legacyID, err := readLegacyAssignedCertificateRecordID(settingService, target)
		if err != nil {
			return nil, err
		}
		if legacyID > 0 {
			filteredLegacy, legacyFilterErr := filterExistingCertificateRecordIDs([]uint{legacyID})
			if legacyFilterErr != nil {
				return nil, legacyFilterErr
			}
			resolved = filteredLegacy
		}
	}
	for _, id := range resolved {
		if id == 0 {
			continue
		}
		result[id] = struct{}{}
	}
	return result, nil
}

func collectCertificateTLSUsage(recordID uint) certificateTLSUsage {
	if recordID == 0 {
		return certificateTLSUsage{}
	}

	db := database.GetDB()
	result := certificateTLSUsage{}

	defaultRows := make([]model.Tls, 0)
	if err := db.Select("id", "name").Where("certificate_record_id = ?", recordID).Find(&defaultRows).Error; err == nil {
		for i := range defaultRows {
			name := strings.TrimSpace(defaultRows[i].Name)
			if name == "" {
				name = fmt.Sprintf("#%d", defaultRows[i].Id)
			}
			result.DefaultTLSNames = append(result.DefaultTLSNames, name)
		}
	}

	mihomoRows := make([]model.MihomoTls, 0)
	if err := db.Select("id", "name").Where("certificate_record_id = ?", recordID).Find(&mihomoRows).Error; err == nil {
		for i := range mihomoRows {
			name := strings.TrimSpace(mihomoRows[i].Name)
			if name == "" {
				name = fmt.Sprintf("#%d", mihomoRows[i].Id)
			}
			result.MihomoTLSNames = append(result.MihomoTLSNames, name)
		}
	}

	return result
}

func collectCertificateReverseProxyUsage(recordID uint) certificateReverseProxyUsage {
	if recordID == 0 {
		return certificateReverseProxyUsage{}
	}

	db := database.GetDB()
	rows := make([]model.ReverseProxyRule, 0)
	if err := db.Select("id", "name", "certificate_record_id", "certificate_record_list", "list_order").
		Order("list_order ASC, id ASC").
		Find(&rows).Error; err != nil {
		return certificateReverseProxyUsage{}
	}

	result := certificateReverseProxyUsage{}
	seen := make(map[string]struct{})
	for i := range rows {
		certIDs := reverseProxyRuleCertificateIDs(&rows[i])
		matched := false
		for _, certID := range certIDs {
			if certID == recordID {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		name := strings.TrimSpace(rows[i].Name)
		if name == "" {
			name = fmt.Sprintf("#%d", rows[i].Id)
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		result.RuleNames = append(result.RuleNames, name)
	}

	return result
}

func buildReverseProxyUsageMarker(usage certificateReverseProxyUsage) string {
	if len(usage.RuleNames) == 0 {
		return ""
	}
	return "反向代理使用中: " + formatCertificateUsageNames(usage.RuleNames)
}

func buildCertificateUsageMarkers(inUseByPanel bool, inUseBySub bool, tlsUsage certificateTLSUsage, reverseProxyUsage certificateReverseProxyUsage) []string {
	markers := make([]string, 0, 5)
	if inUseByPanel {
		markers = append(markers, "面板入口使用中")
	}
	if inUseBySub {
		markers = append(markers, "订阅入口使用中")
	}
	if len(tlsUsage.DefaultTLSNames) > 0 {
		markers = append(markers, "sing-box TLS 使用中: "+formatCertificateUsageNames(tlsUsage.DefaultTLSNames))
	}
	if len(tlsUsage.MihomoTLSNames) > 0 {
		markers = append(markers, "mihomo TLS 使用中: "+formatCertificateUsageNames(tlsUsage.MihomoTLSNames))
	}
	if marker := buildReverseProxyUsageMarker(reverseProxyUsage); marker != "" {
		markers = append(markers, marker)
	}
	return markers
}

func buildCertificateUsageLabel(inUseByPanel bool, inUseBySub bool, tlsUsage certificateTLSUsage, reverseProxyUsage certificateReverseProxyUsage) string {
	return strings.Join(buildCertificateUsageMarkers(inUseByPanel, inUseBySub, tlsUsage, reverseProxyUsage), " / ")
}

func mergeCertificateRemark(base string, inUseByPanel bool, inUseBySub bool, tlsUsage certificateTLSUsage, reverseProxyUsage certificateReverseProxyUsage) string {
	base = strings.TrimSpace(base)
	markers := buildCertificateUsageMarkers(inUseByPanel, inUseBySub, tlsUsage, reverseProxyUsage)
	if len(markers) == 0 {
		return base
	}
	if base == "" {
		return strings.Join(markers, " / ")
	}
	merged := base
	for _, marker := range markers {
		if strings.Contains(merged, marker) {
			continue
		}
		merged += " / " + marker
	}
	return merged
}

func formatCertificateUsageNames(names []string) string {
	filtered := make([]string, 0, len(names))
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		filtered = append(filtered, name)
		if len(filtered) >= 3 {
			break
		}
	}
	if len(filtered) == 0 {
		return "-"
	}
	suffix := ""
	if len(names) > len(filtered) {
		suffix = fmt.Sprintf(" 等 %d 项", len(names))
	}
	return strings.Join(filtered, ", ") + suffix
}

func certificateUsageLabel(inUseByPanel bool, inUseBySub bool) string {
	switch {
	case inUseByPanel && inUseBySub:
		return "界面、订阅使用中"
	case inUseByPanel:
		return "界面使用中"
	case inUseBySub:
		return "订阅使用中"
	default:
		return ""
	}
}
