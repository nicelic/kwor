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

	certificateDisplayIDMin uint64 = 1
	certificateDisplayIDMax uint64 = 100000000000
)

type CertificateRecordView struct {
	Id        uint   `json:"id"`
	DisplayID uint64 `json:"displayId"`

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

	AcmeAccountID   uint   `json:"acmeAccountId"`
	AcmeAccountName string `json:"acmeAccountName"`
	DNSAccountID    uint   `json:"dnsAccountId"`
	DNSAccountName  string `json:"dnsAccountName"`
	ApplyTarget     string `json:"applyTarget"`
	PushDir         string `json:"pushDir"`
	PushFiles       string `json:"pushFiles"`
	Remark          string `json:"remark"`
	RenewConfig     string `json:"renewConfig"`

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

	LastIssuedAt  int64  `json:"lastIssuedAt"`
	LastRenewedAt int64  `json:"lastRenewedAt"`
	ListOrderAt   int64  `json:"listOrderAt"`
	UpdatedAt     int64  `json:"updatedAt"`
	CreatedAt     int64  `json:"createdAt"`
	LastError     string `json:"lastError"`
	LastOutput    string `json:"lastOutput"`
	Status        string `json:"status"`
	InUseByPanel  bool   `json:"inUseByPanel"`
	InUseBySub    bool   `json:"inUseBySub"`
	InUseByTLS    bool   `json:"inUseByTls"`
	InUseByMihomo bool   `json:"inUseByMihomo"`
	UsageLabel    string `json:"usageLabel"`
	DeleteBlocked bool   `json:"deleteBlocked"`
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

	FullchainPEM             string `json:"fullchainPem"`
	KeyPEM                   string `json:"keyPem"`
	Fingerprint              string `json:"fingerprint"`
	IssuedKeyAlgorithm       string `json:"issuedKeyAlgorithm"`
	IssuedSignatureAlgorithm string `json:"issuedSignatureAlgorithm"`
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

	AcmeAccountID   uint
	AcmeAccountName string
	DNSAccountID    uint
	DNSAccountName  string
	ApplyTarget     string
	PushDir         string
	PushFiles       string
	Remark          string
	RenewConfig     string

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

	LastIssuedAt  int64
	LastRenewedAt int64
	LastError     string
	LastOutput    string
	ListOrderAt   int64
}

func (s *CertificateInventoryService) List() ([]CertificateRecordView, error) {
	rows := make([]model.CertificateRecord, 0)
	if err := database.GetDB().Order("list_order_at DESC, id DESC").Find(&rows).Error; err != nil {
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

func (s *CertificateInventoryService) GetMaterial(id uint) (*CertificateMaterialView, error) {
	if id == 0 {
		return nil, common.NewError("certificate id is required")
	}
	row := &model.CertificateRecord{}
	if err := database.GetDB().Where("id = ?", id).First(row).Error; err != nil {
		return nil, err
	}
	issuedKeyAlgorithm, issuedSignatureAlgorithm := inspectIssuedCertificateAlgorithms(row.FullchainPEM)
	return &CertificateMaterialView{
		Id:                       row.Id,
		MainDomain:               strings.TrimSpace(row.MainDomain),
		SourceType:               strings.TrimSpace(row.SourceType),
		SourceRef:                strings.TrimSpace(row.SourceRef),
		CertPath:                 strings.TrimSpace(row.CertPath),
		KeyPath:                  strings.TrimSpace(row.KeyPath),
		FullchainPath:            strings.TrimSpace(row.FullchainPath),
		ChainPath:                strings.TrimSpace(row.ChainPath),
		FullchainPEM:             strings.TrimSpace(string(row.FullchainPEM)),
		KeyPEM:                   strings.TrimSpace(string(row.KeyPEM)),
		Fingerprint:              strings.TrimSpace(row.Fingerprint),
		IssuedKeyAlgorithm:       issuedKeyAlgorithm,
		IssuedSignatureAlgorithm: issuedSignatureAlgorithm,
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
	return database.GetDB().Where("id = ?", id).Delete(&model.CertificateRecord{}).Error
}

func (s *CertificateInventoryService) DeleteBySource(sourceType string, sourceRef string) error {
	sourceType = strings.TrimSpace(strings.ToLower(sourceType))
	sourceRef = strings.TrimSpace(sourceRef)
	if sourceType == "" || sourceRef == "" {
		return nil
	}
	return database.GetDB().
		Where("source_type = ? AND source_ref = ?", sourceType, sourceRef).
		Delete(&model.CertificateRecord{}).Error
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

	entry.AcmeAccountID = payload.AcmeAccountID
	entry.AcmeAccountName = strings.TrimSpace(payload.AcmeAccountName)
	entry.DNSAccountID = payload.DNSAccountID
	entry.DNSAccountName = strings.TrimSpace(payload.DNSAccountName)
	entry.ApplyTarget = strings.TrimSpace(payload.ApplyTarget)
	entry.PushDir = strings.TrimSpace(payload.PushDir)
	entry.PushFiles = strings.TrimSpace(payload.PushFiles)
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
	entry.LastOutput = strings.TrimSpace(payload.LastOutput)
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
	return entry, nil
}

func (s *CertificateInventoryService) RepairDisplayIDs() error {
	db := database.GetDB()
	rows := make([]model.CertificateRecord, 0)
	if err := db.Order("id ASC").Find(&rows).Error; err != nil {
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
	issuedKeyAlgorithm, issuedSignatureAlgorithm := inspectIssuedCertificateAlgorithms(entry.FullchainPEM)
	return CertificateRecordView{
		Id:        entry.Id,
		DisplayID: entry.DisplayID,

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

		AcmeAccountID:   entry.AcmeAccountID,
		AcmeAccountName: strings.TrimSpace(entry.AcmeAccountName),
		DNSAccountID:    entry.DNSAccountID,
		DNSAccountName:  strings.TrimSpace(entry.DNSAccountName),
		ApplyTarget:     applyTarget,
		PushDir:         strings.TrimSpace(entry.PushDir),
		PushFiles:       strings.TrimSpace(entry.PushFiles),
		Remark:          mergeCertificateRemark(strings.TrimSpace(entry.Remark), inUseByPanel, inUseBySub, tlsUsage, reverseProxyUsage),
		RenewConfig:     strings.TrimSpace(entry.RenewConfig),

		AcmeHome:    strings.TrimSpace(entry.AcmeHome),
		Webroot:     strings.TrimSpace(entry.Webroot),
		DNSProvider: strings.TrimSpace(entry.DNSProvider),
		DNSEnvText:  strings.TrimSpace(entry.DNSEnvText),
		CustomArgs:  strings.TrimSpace(entry.CustomArgs),

		CertPath:      strings.TrimSpace(entry.CertPath),
		KeyPath:       strings.TrimSpace(entry.KeyPath),
		FullchainPath: strings.TrimSpace(entry.FullchainPath),
		ChainPath:     strings.TrimSpace(entry.ChainPath),

		Fingerprint: strings.TrimSpace(entry.Fingerprint),
		NotBefore:   entry.NotBefore,
		NotAfter:    entry.NotAfter,

		LastIssuedAt:  entry.LastIssuedAt,
		LastRenewedAt: entry.LastRenewedAt,
		ListOrderAt:   entry.ListOrderAt,
		UpdatedAt:     entry.UpdatedAt.Unix(),
		CreatedAt:     entry.CreatedAt.Unix(),
		LastError:     strings.TrimSpace(entry.LastError),
		LastOutput:    strings.TrimSpace(entry.LastOutput),
		Status:        certificateStatus(entry),
		InUseByPanel:  inUseByPanel,
		InUseBySub:    inUseBySub,
		InUseByTLS:    inUseByTLS,
		InUseByMihomo: inUseByMihomo,
		UsageLabel:    usageLabel,
		DeleteBlocked: inUseByPanel || inUseBySub || inUseByTLS || inUseByMihomo || reverseProxyUsage.inUse(),
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
