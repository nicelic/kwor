package service

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/pem"
	"fmt"
	"strings"
	"time"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/util/common"
)

const (
	defaultSelfSignedDurationValue = 90
	defaultSelfSignedDurationUnit  = "d"
	defaultSelfSignedAlgorithm     = "ecc256"
	selfSignedUnnamedMainDomain    = "no-sni"
)

type SelfSignedService struct {
	ServerService
}

type SelfSignedAuthorityView struct {
	Id           uint   `json:"id"`
	Name         string `json:"name"`
	PlatformCode string `json:"platformCode"`
	PlatformName string `json:"platformName"`
	SubjectCN    string `json:"subjectCn"`
	Organization string `json:"organization"`
	Department   string `json:"department"`
	Country      string `json:"country"`
	Province     string `json:"province"`
	City         string `json:"city"`
	KeyAlgorithm string `json:"keyAlgorithm"`
	IssuerName   string `json:"issuerName"`
	IssuerOrg    string `json:"issuerOrg"`
	CAURL        string `json:"caUrl"`
	OCSPURL      string `json:"ocspUrl"`
	CRLURL       string `json:"crlUrl"`
	KeyUsage     string `json:"keyUsage"`
	ExtKeyUsage  string `json:"extKeyUsage"`
	SignAlgo     string `json:"signAlgo"`
	Brand        string `json:"brand"`
	Notes        string `json:"notes"`
	Builtin      bool   `json:"builtin"`
	NotBefore    int64  `json:"notBefore"`
	NotAfter     int64  `json:"notAfter"`
	CreatedAt    int64  `json:"createdAt"`
	UpdatedAt    int64  `json:"updatedAt"`
}

type SelfSignedIssuePayload struct {
	ExistingRecordID   uint
	PreferredSourceRef string

	AuthorityID uint

	AuthorityName string
	PlatformCode  string
	PlatformName  string
	SubjectCN     string
	Organization  string
	Department    string
	Country       string
	Province      string
	City          string
	SaveAuthority bool

	DomainsText        string
	AllowEmptyNames    bool
	KeyAlgorithm       string
	SignatureAlgorithm string
	DurationValue      int
	DurationUnit       string
	Remark             string
	ApplyTarget        string
	PushDir            string
	PushExplicit       bool
	TrackedPushFiles   string
}

type selfSignedBuiltinAuthority struct {
	Name         string
	PlatformCode string
	PlatformName string
	SubjectCN    string
	Organization string
	Department   string
	Country      string
	Province     string
	City         string
	KeyAlgorithm string
	IssuerName   string
	IssuerOrg    string
	CAURL        string
	OCSPURL      string
	CRLURL       string
	KeyUsage     string
	ExtKeyUsage  string
	SignAlgo     string
	Brand        string
	Notes        string
}

var defaultSelfSignedAuthorities = []selfSignedBuiltinAuthority{
	{
		Name:         "ZeroSSL",
		PlatformCode: "zerossl",
		PlatformName: "ZeroSSL",
		SubjectCN:    "ZeroSSL ECC DV SSL CA 2",
		Organization: "ZeroSSL GmbH",
		Department:   "DV TLS Issuing CA",
		Country:      "AT",
		Province:     "Vienna",
		City:         "Vienna",
		KeyAlgorithm: "ecc256",
		IssuerName:   "ZeroSSL ECC DV SSL CA 2",
		IssuerOrg:    "ZeroSSL GmbH",
		CAURL:        "http://crt.sectigo.com/ZeroSLECCDVSSLCA2.crt",
		OCSPURL:      "http://ocsp.sectigo.com",
		CRLURL:       "",
		KeyUsage:     "Digital Signature",
		ExtKeyUsage:  "Server Auth",
		SignAlgo:     "SHA256-ECDSA",
		Brand:        "ZeroSSL",
		Notes:        "ZeroSSL 证书默认使用 OCSP，官方文档说明当前不提供 CRL 扩展。",
	},
	{
		Name:         "Let's Encrypt",
		PlatformCode: "letsencrypt",
		PlatformName: "Let's Encrypt",
		SubjectCN:    "Let's Encrypt, CN = R13",
		Organization: "Let's Encrypt",
		Department:   "DV TLS Issuing CA",
		Country:      "US",
		Province:     "California",
		City:         "San Francisco",
		KeyAlgorithm: "ecc256",
		IssuerName:   "Let's Encrypt R13",
		IssuerOrg:    "Let's Encrypt",
		CAURL:        "https://letsencrypt.org/certs/2024/r13.pem",
		OCSPURL:      "",
		CRLURL:       "r13.c.lencr.org",
		KeyUsage:     "Digital Signature",
		ExtKeyUsage:  "Server Auth",
		SignAlgo:     "SHA256-ECDSA",
		Brand:        "Let's Encrypt",
		Notes:        "Let's Encrypt 已在 2025-05-07 移除证书 OCSP URL，并在 2025-08-06 关闭 OCSP 响应服务，现以 CRL 为主。",
	},
}

func resolveBuiltinAuthorityProfile(platformCode string) *selfSignedBuiltinAuthority {
	code := strings.ToLower(strings.TrimSpace(platformCode))
	for i := range defaultSelfSignedAuthorities {
		row := defaultSelfSignedAuthorities[i]
		if strings.ToLower(strings.TrimSpace(row.PlatformCode)) == code {
			return &row
		}
	}
	return nil
}

func (s *SelfSignedService) ListAuthorities() ([]SelfSignedAuthorityView, error) {
	if err := s.ensureBuiltinAuthorities(); err != nil {
		return nil, err
	}
	rows := make([]model.SelfSignedAuthority, 0)
	if err := database.GetDB().
		Order("builtin DESC, updated_at DESC, id DESC").
		Find(&rows).Error; err != nil {
		return nil, err
	}

	result := make([]SelfSignedAuthorityView, 0, len(rows))
	for i := range rows {
		row := rows[i]
		profile := resolveBuiltinAuthorityProfile(row.PlatformCode)
		issuerName := ""
		issuerOrg := ""
		caURL := ""
		ocspURL := ""
		crlURL := ""
		keyUsage := ""
		extKeyUsage := ""
		signAlgo := ""
		brand := ""
		notes := ""
		if profile != nil {
			issuerName = strings.TrimSpace(profile.IssuerName)
			issuerOrg = strings.TrimSpace(profile.IssuerOrg)
			caURL = strings.TrimSpace(profile.CAURL)
			ocspURL = strings.TrimSpace(profile.OCSPURL)
			crlURL = strings.TrimSpace(profile.CRLURL)
			keyUsage = strings.TrimSpace(profile.KeyUsage)
			extKeyUsage = strings.TrimSpace(profile.ExtKeyUsage)
			signAlgo = strings.TrimSpace(profile.SignAlgo)
			brand = strings.TrimSpace(profile.Brand)
			notes = strings.TrimSpace(profile.Notes)
		}
		result = append(result, SelfSignedAuthorityView{
			Id:           row.Id,
			Name:         strings.TrimSpace(row.Name),
			PlatformCode: strings.TrimSpace(row.PlatformCode),
			PlatformName: strings.TrimSpace(row.PlatformName),
			SubjectCN:    strings.TrimSpace(row.SubjectCN),
			Organization: strings.TrimSpace(row.Organization),
			Department:   strings.TrimSpace(row.Department),
			Country:      strings.TrimSpace(row.Country),
			Province:     strings.TrimSpace(row.Province),
			City:         strings.TrimSpace(row.City),
			KeyAlgorithm: strings.TrimSpace(row.KeyAlgorithm),
			IssuerName:   issuerName,
			IssuerOrg:    issuerOrg,
			CAURL:        caURL,
			OCSPURL:      ocspURL,
			CRLURL:       crlURL,
			KeyUsage:     keyUsage,
			ExtKeyUsage:  extKeyUsage,
			SignAlgo:     signAlgo,
			Brand:        brand,
			Notes:        notes,
			Builtin:      row.Builtin,
			NotBefore:    row.NotBefore,
			NotAfter:     row.NotAfter,
			CreatedAt:    row.CreatedAt.Unix(),
			UpdatedAt:    row.UpdatedAt.Unix(),
		})
		if !row.Builtin {
			result[len(result)-1].IssuerName = strings.TrimSpace(row.IssuerName)
			result[len(result)-1].IssuerOrg = strings.TrimSpace(row.IssuerOrg)
			result[len(result)-1].CAURL = strings.TrimSpace(row.CAURL)
			result[len(result)-1].OCSPURL = strings.TrimSpace(row.OCSPURL)
			result[len(result)-1].CRLURL = strings.TrimSpace(row.CRLURL)
			result[len(result)-1].KeyUsage = strings.TrimSpace(row.KeyUsage)
			result[len(result)-1].ExtKeyUsage = strings.TrimSpace(row.ExtKeyUsage)
			result[len(result)-1].SignAlgo = strings.TrimSpace(row.SignAlgo)
			result[len(result)-1].Brand = strings.TrimSpace(row.Brand)
			result[len(result)-1].Notes = strings.TrimSpace(row.Notes)
		}
	}
	return result, nil
}

func (s *SelfSignedService) Issue(payload SelfSignedIssuePayload) (*AcmeActionResult, error) {
	domains := normalizeAcmeDomains(payload.DomainsText)
	if len(domains) == 0 && !payload.AllowEmptyNames {
		return nil, common.NewError("domain list is required")
	}

	authority, err := s.resolveAuthorityForIssue(payload)
	if err != nil {
		return nil, err
	}

	keyAlgorithm := normalizeSelfSignedAlgorithm(payload.KeyAlgorithm)
	if keyAlgorithm == "" {
		keyAlgorithm = normalizeSelfSignedAlgorithm(authority.KeyAlgorithm)
	}
	if keyAlgorithm == "" {
		keyAlgorithm = defaultSelfSignedAlgorithm
	}

	signatureAlgorithm := normalizeSelfSignedAlgorithm(payload.SignatureAlgorithm)
	if signatureAlgorithm == "" {
		signatureAlgorithm = keyAlgorithm
	}

	durationValue := payload.DurationValue
	if durationValue <= 0 {
		durationValue = defaultSelfSignedDurationValue
	}
	durationUnit := normalizeSelfSignedDurationUnit(payload.DurationUnit)
	if durationUnit == "" {
		durationUnit = defaultSelfSignedDurationUnit
	}

	serverName := ""
	if len(domains) > 0 {
		serverName = domains[0]
	}

	options := fmt.Sprintf("%s,%d,%s,%s,%s", serverName, durationValue, durationUnit, keyAlgorithm, signatureAlgorithm)
	keypair := s.ServerService.GenKeypair("tls", options)
	if len(keypair) == 0 {
		return nil, common.NewError("generate keypair failed: empty output")
	}
	if strings.HasPrefix(strings.ToLower(strings.TrimSpace(keypair[0])), "failed to generate") {
		return nil, common.NewError(strings.TrimSpace(keypair[0]))
	}

	keyPEM, fullchainPEM, err := parseTLSPEMFromKeypairLines(keypair)
	if err != nil {
		return nil, err
	}
	certPEM, chainPEM := splitLeafAndChainPEM(fullchainPEM)
	if len(certPEM) == 0 {
		return nil, common.NewError("generate keypair failed: leaf certificate is missing")
	}

	fingerprint, notBefore, notAfter, err := inspectCertificateFingerprint(certPEM, keyPEM)
	if err != nil {
		return nil, err
	}

	applyTarget := ""
	if normalizedTarget, ok := normalizeAcmeApplyTarget(payload.ApplyTarget); ok {
		applyTarget = string(normalizedTarget)
	}
	renewConfig := marshalSelfSignedRenewConfig(SelfSignedRenewConfig{
		Mode:               "certificate_center",
		CertificateType:    "domain",
		Domains:            domains,
		AllowEmptyNames:    payload.AllowEmptyNames,
		KeyAlgorithm:       keyAlgorithm,
		SignatureAlgorithm: signatureAlgorithm,
		DurationValue:      durationValue,
		DurationUnit:       durationUnit,
		AuthorityID:        authority.Id,
		AuthorityName:      strings.TrimSpace(authority.Name),
		PlatformCode:       strings.TrimSpace(authority.PlatformCode),
		PlatformName:       strings.TrimSpace(authority.PlatformName),
	})

	platformCode := strings.ToLower(strings.TrimSpace(authority.PlatformCode))
	if platformCode == "" {
		platformCode = "custom"
	}

	sourceRef := makeSelfSignedSourceRef()
	if strings.TrimSpace(payload.PreferredSourceRef) != "" {
		sourceRef = strings.TrimSpace(payload.PreferredSourceRef)
	}
	if payload.ExistingRecordID > 0 {
		existingRecord, err := certificateInventory.GetRecordByID(payload.ExistingRecordID)
		if err == nil && existingRecord != nil && strings.TrimSpace(existingRecord.SourceRef) != "" {
			sourceRef = strings.TrimSpace(existingRecord.SourceRef)
		}
	}

	mainDomain := selfSignedUnnamedMainDomain
	if len(domains) > 0 {
		mainDomain = domains[0]
	}

	row, err := certificateInventory.Upsert(CertificateUpsertPayload{
		SourceType: CertificateSourceSelfSigned,
		SourceRef:  sourceRef,

		MainDomain: mainDomain,
		Domains:    domains,

		Challenge: "self_signed",
		KeyLength: keyAlgorithm,
		CAServer:  platformCode,
		UseECC:    strings.HasPrefix(keyAlgorithm, "ecc"),
		AutoRenew: true,

		AcmeAccountName: strings.TrimSpace(authority.Name),
		ApplyTarget:     applyTarget,
		PushDir:         strings.TrimSpace(payload.PushDir),
		PushFiles:       strings.TrimSpace(payload.TrackedPushFiles),
		Remark:          strings.TrimSpace(payload.Remark),
		RenewConfig:     renewConfig,

		CertPath:      "",
		KeyPath:       "",
		FullchainPath: "",
		ChainPath:     "",

		CertPEM:      certPEM,
		KeyPEM:       keyPEM,
		FullchainPEM: fullchainPEM,
		ChainPEM:     chainPEM,

		Fingerprint: fingerprint,
		NotBefore:   notBefore.Unix(),
		NotAfter:    notAfter.Unix(),

		LastIssuedAt:  time.Now().Unix(),
		LastRenewedAt: time.Now().Unix(),
		LastOutput:    "issued by local sing-box tls generator",
	})
	if err != nil {
		return nil, err
	}

	if row == nil {
		return nil, common.NewError("issue self-signed certificate failed: empty inventory record")
	}

	if payload.PushExplicit && strings.TrimSpace(row.PushDir) != "" {
		pushState, pushErr := syncCertificateDirectoryPushState(
			row.PushDir,
			row.PushDir,
			row.PushFiles,
			certPEM,
			keyPEM,
			fullchainPEM,
			chainPEM,
		)
		if pushErr != nil {
			return nil, pushErr
		}
		row.PushDir = pushState.PushDir
		row.PushFiles = pushState.PushFiles
	}

	if applyTarget != "" {
		target, _ := normalizeAcmeApplyTarget(applyTarget)
		acmeSvc := &AcmeService{}
		if err := acmeSvc.applyInventoryRecordToTarget(row, target); err != nil {
			return nil, err
		}
		row.ApplyTarget = applyTarget
	}

	if err := database.GetDB().Save(row).Error; err != nil {
		return nil, err
	}
	if err := ApplyPanelTLSRuntimeSettingsForRecord(row.Id); err != nil {
		return nil, err
	}
	if _, err := ForceSyncTLSBindingsForCertificateRecord(row.Id, ""); err != nil {
		return nil, err
	}
	if err := (&AcmeService{}).EnsureOverviewRuntimeConsistency(true); err != nil {
		return nil, err
	}

	overview, err := (&AcmeService{}).GetOverview()
	if err != nil {
		return nil, err
	}
	view := convertCertificateRecord(row)
	return &AcmeActionResult{
		Overview:    overview,
		Certificate: &view,
		Msg:         "self-signed certificate issued",
	}, nil
}

func (s *SelfSignedService) DeleteAuthority(id uint) (*AcmeActionResult, error) {
	if id == 0 {
		return nil, common.NewError("authority id is required")
	}
	row := &model.SelfSignedAuthority{}
	if err := database.GetDB().Where("id = ?", id).First(row).Error; err != nil {
		return nil, err
	}
	if row.Builtin {
		return nil, common.NewError("builtin authority cannot be deleted")
	}
	if err := database.GetDB().Delete(row).Error; err != nil {
		return nil, err
	}
	if err := (&AcmeService{}).EnsureOverviewRuntimeConsistency(true); err != nil {
		return nil, err
	}
	overview, err := (&AcmeService{}).GetOverview()
	if err != nil {
		return nil, err
	}
	return &AcmeActionResult{
		Overview: overview,
		Msg:      "authority deleted",
	}, nil
}

func (s *SelfSignedService) SaveAuthority(entry *model.SelfSignedAuthority) (*AcmeActionResult, error) {
	if err := s.ensureBuiltinAuthorities(); err != nil {
		return nil, err
	}
	if entry == nil {
		return nil, common.NewError("authority is required")
	}

	normalized := &model.SelfSignedAuthority{
		Name:         strings.TrimSpace(entry.Name),
		PlatformCode: strings.ToLower(strings.TrimSpace(entry.PlatformCode)),
		PlatformName: strings.TrimSpace(entry.PlatformName),
		SubjectCN:    strings.TrimSpace(entry.SubjectCN),
		Organization: strings.TrimSpace(entry.Organization),
		Department:   strings.TrimSpace(entry.Department),
		Country:      strings.ToUpper(strings.TrimSpace(entry.Country)),
		Province:     strings.TrimSpace(entry.Province),
		City:         strings.TrimSpace(entry.City),
		KeyAlgorithm: normalizeSelfSignedAlgorithm(entry.KeyAlgorithm),
		IssuerName:   strings.TrimSpace(entry.IssuerName),
		IssuerOrg:    strings.TrimSpace(entry.IssuerOrg),
		CAURL:        strings.TrimSpace(entry.CAURL),
		OCSPURL:      strings.TrimSpace(entry.OCSPURL),
		CRLURL:       strings.TrimSpace(entry.CRLURL),
		KeyUsage:     strings.TrimSpace(entry.KeyUsage),
		ExtKeyUsage:  strings.TrimSpace(entry.ExtKeyUsage),
		SignAlgo:     strings.TrimSpace(entry.SignAlgo),
		Brand:        strings.TrimSpace(entry.Brand),
		Notes:        strings.TrimSpace(entry.Notes),
	}
	if normalized.Name == "" {
		return nil, common.NewError("authority name is required")
	}
	if normalized.SubjectCN == "" {
		return nil, common.NewError("subject cn is required")
	}
	if normalized.Organization == "" {
		return nil, common.NewError("organization is required")
	}
	if normalized.Country == "" {
		return nil, common.NewError("country is required")
	}
	if len(normalized.Country) != 2 {
		return nil, common.NewError("country must be a 2-letter code")
	}
	if normalized.PlatformCode == "" {
		normalized.PlatformCode = "custom"
	}
	if normalized.PlatformName == "" {
		normalized.PlatformName = normalized.Name
	}
	if normalized.KeyAlgorithm == "" {
		normalized.KeyAlgorithm = defaultSelfSignedAlgorithm
	}
	if normalized.IssuerName == "" {
		normalized.IssuerName = normalized.SubjectCN
	}
	if normalized.IssuerOrg == "" {
		normalized.IssuerOrg = normalized.Organization
	}
	if normalized.Brand == "" {
		normalized.Brand = normalized.PlatformName
	}
	if normalized.SignAlgo == "" {
		normalized.SignAlgo = "SHA256-ECDSA"
	}
	if normalized.KeyUsage == "" {
		normalized.KeyUsage = "Digital Signature"
	}
	if normalized.ExtKeyUsage == "" {
		normalized.ExtKeyUsage = "Server Auth"
	}

	if entry.Id > 0 {
		existing := &model.SelfSignedAuthority{}
		if err := database.GetDB().Where("id = ?", entry.Id).First(existing).Error; err != nil {
			return nil, err
		}
		if existing.Builtin {
			return nil, common.NewError("builtin authority cannot be edited")
		}
		existing.Name = normalized.Name
		existing.PlatformCode = normalized.PlatformCode
		existing.PlatformName = normalized.PlatformName
		existing.SubjectCN = normalized.SubjectCN
		existing.Organization = normalized.Organization
		existing.Department = normalized.Department
		existing.Country = normalized.Country
		existing.Province = normalized.Province
		existing.City = normalized.City
		if strings.TrimSpace(entry.KeyAlgorithm) != "" {
			existing.KeyAlgorithm = normalized.KeyAlgorithm
			if existing.KeyAlgorithm == "" {
				existing.KeyAlgorithm = defaultSelfSignedAlgorithm
			}
		} else if strings.TrimSpace(existing.KeyAlgorithm) == "" {
			existing.KeyAlgorithm = defaultSelfSignedAlgorithm
		}
		existing.IssuerName = normalized.IssuerName
		existing.IssuerOrg = normalized.IssuerOrg
		existing.CAURL = normalized.CAURL
		existing.OCSPURL = normalized.OCSPURL
		existing.CRLURL = normalized.CRLURL
		existing.KeyUsage = normalized.KeyUsage
		existing.ExtKeyUsage = normalized.ExtKeyUsage
		if strings.TrimSpace(entry.SignAlgo) != "" {
			existing.SignAlgo = normalized.SignAlgo
			if existing.SignAlgo == "" {
				existing.SignAlgo = "SHA256-ECDSA"
			}
		} else if strings.TrimSpace(existing.SignAlgo) == "" {
			existing.SignAlgo = "SHA256-ECDSA"
		}
		existing.Brand = normalized.Brand
		existing.Notes = normalized.Notes
		if existing.RootCertPEM == nil {
			existing.RootCertPEM = []byte{}
		}
		if existing.IssuerCertPEM == nil {
			existing.IssuerCertPEM = []byte{}
		}
		if existing.IssuerKeyPEM == nil {
			existing.IssuerKeyPEM = []byte{}
		}
		if err := database.GetDB().Save(existing).Error; err != nil {
			return nil, err
		}
		if err := (&AcmeService{}).EnsureOverviewRuntimeConsistency(true); err != nil {
			return nil, err
		}
		overview, err := (&AcmeService{}).GetOverview()
		if err != nil {
			return nil, err
		}
		return &AcmeActionResult{
			Overview: overview,
			Msg:      "authority saved",
		}, nil
	}

	saved, err := s.saveCustomAuthority(normalized)
	if err != nil {
		return nil, err
	}
	if err := (&AcmeService{}).EnsureOverviewRuntimeConsistency(true); err != nil {
		return nil, err
	}
	overview, err := (&AcmeService{}).GetOverview()
	if err != nil {
		return nil, err
	}
	_ = saved
	return &AcmeActionResult{
		Overview: overview,
		Msg:      "authority saved",
	}, nil
}

func (s *SelfSignedService) resolveAuthorityForIssue(payload SelfSignedIssuePayload) (*model.SelfSignedAuthority, error) {
	if err := s.ensureBuiltinAuthorities(); err != nil {
		return nil, err
	}

	if payload.AuthorityID > 0 {
		entry := &model.SelfSignedAuthority{}
		if err := database.GetDB().Where("id = ?", payload.AuthorityID).First(entry).Error; err != nil {
			return nil, err
		}
		return entry, nil
	}

	platformCode := strings.ToLower(strings.TrimSpace(payload.PlatformCode))
	if platformCode != "" {
		entry := &model.SelfSignedAuthority{}
		if err := database.GetDB().Where("platform_code = ?", platformCode).Order("builtin DESC, id DESC").First(entry).Error; err == nil {
			return entry, nil
		}
	}

	custom := buildCustomAuthorityFromPayload(payload)
	if custom == nil {
		entry := &model.SelfSignedAuthority{}
		if err := database.GetDB().Where("platform_code = ?", "zerossl").Order("builtin DESC, id DESC").First(entry).Error; err != nil {
			return nil, err
		}
		return entry, nil
	}
	if payload.SaveAuthority {
		saved, err := s.saveCustomAuthority(custom)
		if err != nil {
			return nil, err
		}
		return saved, nil
	}
	return custom, nil
}

func buildCustomAuthorityFromPayload(payload SelfSignedIssuePayload) *model.SelfSignedAuthority {
	name := strings.TrimSpace(payload.AuthorityName)
	platformCode := strings.ToLower(strings.TrimSpace(payload.PlatformCode))
	platformName := strings.TrimSpace(payload.PlatformName)
	if name == "" && platformCode == "" && platformName == "" {
		return nil
	}

	if name == "" {
		if platformName != "" {
			name = platformName
		} else {
			name = platformCode
		}
	}
	if platformCode == "" {
		platformCode = "custom"
	}
	if platformName == "" {
		platformName = name
	}

	return &model.SelfSignedAuthority{
		Name:          name,
		PlatformCode:  platformCode,
		PlatformName:  platformName,
		SubjectCN:     strings.TrimSpace(payload.SubjectCN),
		Organization:  strings.TrimSpace(payload.Organization),
		Department:    strings.TrimSpace(payload.Department),
		Country:       strings.TrimSpace(payload.Country),
		Province:      strings.TrimSpace(payload.Province),
		City:          strings.TrimSpace(payload.City),
		KeyAlgorithm:  normalizeSelfSignedAlgorithm(payload.KeyAlgorithm),
		Builtin:       false,
		RootCertPEM:   []byte{},
		IssuerCertPEM: []byte{},
		IssuerKeyPEM:  []byte{},
	}
}

func (s *SelfSignedService) saveCustomAuthority(entry *model.SelfSignedAuthority) (*model.SelfSignedAuthority, error) {
	if entry == nil {
		return nil, common.NewError("authority is required")
	}
	entry.Name = strings.TrimSpace(entry.Name)
	if entry.Name == "" {
		return nil, common.NewError("authority name is required")
	}

	exists := &model.SelfSignedAuthority{}
	err := database.GetDB().Where("name = ?", entry.Name).First(exists).Error
	switch {
	case err == nil:
		if exists.Builtin {
			return nil, common.NewError("builtin authority name is reserved")
		}
		exists.PlatformCode = strings.TrimSpace(entry.PlatformCode)
		exists.PlatformName = strings.TrimSpace(entry.PlatformName)
		exists.SubjectCN = strings.TrimSpace(entry.SubjectCN)
		exists.Organization = strings.TrimSpace(entry.Organization)
		exists.Department = strings.TrimSpace(entry.Department)
		exists.Country = strings.TrimSpace(entry.Country)
		exists.Province = strings.TrimSpace(entry.Province)
		exists.City = strings.TrimSpace(entry.City)
		exists.KeyAlgorithm = normalizeSelfSignedAlgorithm(entry.KeyAlgorithm)
		if exists.KeyAlgorithm == "" {
			exists.KeyAlgorithm = defaultSelfSignedAlgorithm
		}
		if exists.RootCertPEM == nil {
			exists.RootCertPEM = []byte{}
		}
		if exists.IssuerCertPEM == nil {
			exists.IssuerCertPEM = []byte{}
		}
		if exists.IssuerKeyPEM == nil {
			exists.IssuerKeyPEM = []byte{}
		}
		if err := database.GetDB().Save(exists).Error; err != nil {
			return nil, err
		}
		return exists, nil
	case database.IsNotFound(err):
		if entry.KeyAlgorithm == "" {
			entry.KeyAlgorithm = defaultSelfSignedAlgorithm
		}
		if entry.RootCertPEM == nil {
			entry.RootCertPEM = []byte{}
		}
		if entry.IssuerCertPEM == nil {
			entry.IssuerCertPEM = []byte{}
		}
		if entry.IssuerKeyPEM == nil {
			entry.IssuerKeyPEM = []byte{}
		}
		if err := database.GetDB().Save(entry).Error; err != nil {
			return nil, err
		}
		return entry, nil
	default:
		return nil, err
	}
}

func (s *SelfSignedService) ensureBuiltinAuthorities() error {
	for _, item := range defaultSelfSignedAuthorities {
		row := &model.SelfSignedAuthority{}
		findErr := database.GetDB().
			Where("platform_code = ? AND builtin = ?", strings.TrimSpace(item.PlatformCode), true).
			Order("id ASC").
			First(row).Error
		if database.IsNotFound(findErr) {
			legacyNames := []string{
				fmt.Sprintf("%s (Local Simulation)", strings.TrimSpace(item.PlatformName)),
				fmt.Sprintf("%s (Local Simulation)", strings.TrimSpace(item.Name)),
			}
			for _, legacy := range legacyNames {
				legacy = strings.TrimSpace(legacy)
				if legacy == "" {
					continue
				}
				candidate := &model.SelfSignedAuthority{}
				err := database.GetDB().
					Where("name = ? AND platform_code = ?", legacy, strings.TrimSpace(item.PlatformCode)).
					Order("id ASC").
					First(candidate).Error
				if err == nil {
					findErr = nil
					row = candidate
					break
				}
			}
		}
		switch {
		case findErr == nil:
			nextKeyAlgorithm := normalizeSelfSignedAlgorithm(item.KeyAlgorithm)
			if nextKeyAlgorithm == "" {
				nextKeyAlgorithm = defaultSelfSignedAlgorithm
			}
			needsSave := false
			if strings.TrimSpace(row.Name) != strings.TrimSpace(item.Name) {
				row.Name = strings.TrimSpace(item.Name)
				needsSave = true
			}
			if strings.TrimSpace(row.PlatformCode) != strings.TrimSpace(item.PlatformCode) {
				row.PlatformCode = strings.TrimSpace(item.PlatformCode)
				needsSave = true
			}
			if strings.TrimSpace(row.PlatformName) != strings.TrimSpace(item.PlatformName) {
				row.PlatformName = strings.TrimSpace(item.PlatformName)
				needsSave = true
			}
			if strings.TrimSpace(row.SubjectCN) != strings.TrimSpace(item.SubjectCN) {
				row.SubjectCN = strings.TrimSpace(item.SubjectCN)
				needsSave = true
			}
			if strings.TrimSpace(row.Organization) != strings.TrimSpace(item.Organization) {
				row.Organization = strings.TrimSpace(item.Organization)
				needsSave = true
			}
			if strings.TrimSpace(row.Department) != strings.TrimSpace(item.Department) {
				row.Department = strings.TrimSpace(item.Department)
				needsSave = true
			}
			if strings.TrimSpace(row.Country) != strings.TrimSpace(item.Country) {
				row.Country = strings.TrimSpace(item.Country)
				needsSave = true
			}
			if strings.TrimSpace(row.Province) != strings.TrimSpace(item.Province) {
				row.Province = strings.TrimSpace(item.Province)
				needsSave = true
			}
			if strings.TrimSpace(row.City) != strings.TrimSpace(item.City) {
				row.City = strings.TrimSpace(item.City)
				needsSave = true
			}
			if strings.TrimSpace(row.KeyAlgorithm) != nextKeyAlgorithm {
				row.KeyAlgorithm = nextKeyAlgorithm
				needsSave = true
			}
			if !row.Builtin {
				row.Builtin = true
				needsSave = true
			}
			if row.RootCertPEM == nil {
				row.RootCertPEM = []byte{}
				needsSave = true
			}
			if row.IssuerCertPEM == nil {
				row.IssuerCertPEM = []byte{}
				needsSave = true
			}
			if row.IssuerKeyPEM == nil {
				row.IssuerKeyPEM = []byte{}
				needsSave = true
			}
			if needsSave {
				if err := database.GetDB().Save(row).Error; err != nil {
					return err
				}
			}
		case database.IsNotFound(findErr):
			row = &model.SelfSignedAuthority{
				Name:          strings.TrimSpace(item.Name),
				PlatformCode:  strings.TrimSpace(item.PlatformCode),
				PlatformName:  strings.TrimSpace(item.PlatformName),
				SubjectCN:     strings.TrimSpace(item.SubjectCN),
				Organization:  strings.TrimSpace(item.Organization),
				Department:    strings.TrimSpace(item.Department),
				Country:       strings.TrimSpace(item.Country),
				Province:      strings.TrimSpace(item.Province),
				City:          strings.TrimSpace(item.City),
				KeyAlgorithm:  normalizeSelfSignedAlgorithm(item.KeyAlgorithm),
				Builtin:       true,
				RootCertPEM:   []byte{},
				IssuerCertPEM: []byte{},
				IssuerKeyPEM:  []byte{},
			}
			if row.KeyAlgorithm == "" {
				row.KeyAlgorithm = defaultSelfSignedAlgorithm
			}
			if err := database.GetDB().Save(row).Error; err != nil {
				return err
			}
		default:
			return findErr
		}
	}
	return nil
}

func parseTLSPEMFromKeypairLines(lines []string) ([]byte, []byte, error) {
	keyLines := make([]string, 0, 128)
	certLines := make([]string, 0, 256)
	inKey := false
	inCert := false

	for _, raw := range lines {
		line := strings.TrimRight(raw, "\r")
		switch line {
		case "-----BEGIN PRIVATE KEY-----":
			inKey = true
			inCert = false
			keyLines = append(keyLines, line)
			continue
		case "-----END PRIVATE KEY-----":
			if inKey {
				keyLines = append(keyLines, line)
			}
			inKey = false
			continue
		case "-----BEGIN CERTIFICATE-----":
			inCert = true
			inKey = false
			certLines = append(certLines, line)
			continue
		case "-----END CERTIFICATE-----":
			if inCert {
				certLines = append(certLines, line)
			}
			inCert = false
			continue
		}

		if inKey {
			keyLines = append(keyLines, line)
		} else if inCert {
			certLines = append(certLines, line)
		}
	}

	if len(keyLines) == 0 {
		return nil, nil, common.NewError("generate keypair failed: private key PEM not found")
	}
	if len(certLines) == 0 {
		return nil, nil, common.NewError("generate keypair failed: certificate PEM not found")
	}

	keyPEM := []byte(strings.Join(keyLines, "\n") + "\n")
	fullchainPEM := []byte(strings.Join(certLines, "\n") + "\n")
	return keyPEM, fullchainPEM, nil
}

func splitLeafAndChainPEM(fullchainPEM []byte) ([]byte, []byte) {
	rest := fullchainPEM
	blocks := make([][]byte, 0, 4)
	for {
		block, next := pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type == "CERTIFICATE" {
			blocks = append(blocks, pem.EncodeToMemory(block))
		}
		rest = next
		if len(rest) == 0 {
			break
		}
	}
	if len(blocks) == 0 {
		return nil, nil
	}
	leaf := blocks[0]
	if len(blocks) == 1 {
		return leaf, nil
	}
	chain := []byte{}
	for i := 1; i < len(blocks); i++ {
		chain = append(chain, blocks[i]...)
	}
	return leaf, chain
}

func normalizeSelfSignedAlgorithm(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "ec-256":
		return "ecc256"
	case "ec-384":
		return "ecc384"
	case "ec-521":
		return "ecc521"
	case "2048":
		return "rsa2048"
	case "3072":
		return "rsa3072"
	case "4096":
		return "rsa4096"
	case "8192":
		return "rsa8192"
	case "ecc224", "ecc256", "ecc384", "ecc521", "rsa1024", "rsa2048", "rsa3072", "rsa4096", "rsa8192":
		return value
	default:
		return ""
	}
}

func normalizeSelfSignedDurationUnit(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "y":
		return "y"
	case "m":
		return "m"
	case "d":
		return "d"
	default:
		return ""
	}
}

func makeSelfSignedSourceRef() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return fmt.Sprintf("selfsigned-%d", time.Now().UnixNano())
	}
	return "selfsigned-" + hex.EncodeToString(buf)
}
