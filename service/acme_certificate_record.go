package service

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/util/common"
)

// createManualDNSAccountLocked persists a one-off issue-form credential only
// after acme.sh has successfully produced certificate material. Future
// renewals then resolve the credential exclusively from the database.
func (s *AcmeService) createManualDNSAccountLocked(providerCode string, envText string) (*model.AcmeDNSAccount, error) {
	providerMeta, ok := lookupAcmeDNSProvider(strings.TrimSpace(providerCode))
	if !ok {
		return nil, common.NewError("不支持的 dns 提供商: ", providerCode)
	}
	envPairs, err := normalizeAcmeEnvAssignments(envText)
	if err != nil {
		return nil, err
	}
	envMap := envPairsToEnvMap(envPairs)
	envMap = sanitizeDNSAccountEnvForProvider(providerMeta, envMap)
	if err := validateDNSProviderEnv(providerMeta, envMap); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(envMap)
	if err != nil {
		return nil, err
	}
	entry := &model.AcmeDNSAccount{
		ProviderName: providerMeta.Name,
		ProviderCode: providerMeta.ProviderCode,
		EnvJSON:      string(raw),
		Remark:       "由证书签发表单自动创建",
	}
	db := database.GetDB()
	if err := ensureDNSAccountDisplayID(db, entry); err != nil {
		return nil, err
	}
	entry.Name = "手工 DNS " + dnsAccountResourceID(entry.DisplayID)
	if err := db.Create(entry).Error; err != nil {
		return nil, err
	}
	return entry, nil
}

// applyAcmeReissueDefaults restores configuration that an edit-and-reissue
// request omitted. CertificateRecord is the only durable ACME certificate
// source, so no legacy mirror lookup is involved here.
func applyAcmeReissueDefaults(payload *AcmeIssuePayload, existing *model.CertificateRecord) {
	if payload == nil || existing == nil {
		return
	}

	stringProvided := func(value string, provided bool) bool {
		return provided || strings.TrimSpace(value) != ""
	}
	uintProvided := func(value uint, provided bool) bool {
		return provided || value != 0
	}

	if !stringProvided(payload.CertificateType, payload.CertificateTypeProvided) {
		payload.CertificateType = existing.CertificateType
	}
	if !stringProvided(payload.DomainsText, payload.DomainsProvided) {
		domains := decodeCertificateDomains(existing.DomainSet)
		if len(domains) == 0 && strings.TrimSpace(existing.MainDomain) != "" {
			domains = []string{strings.TrimSpace(existing.MainDomain)}
		}
		payload.DomainsText = strings.Join(domains, ",")
	}
	if !stringProvided(payload.Challenge, payload.ChallengeProvided) {
		payload.Challenge = existing.Challenge
	}
	if !stringProvided(payload.Webroot, payload.WebrootProvided) {
		payload.Webroot = existing.Webroot
	}
	if !stringProvided(payload.DNSProvider, payload.DNSProviderProvided) {
		payload.DNSProvider = existing.DNSProvider
	}
	if !stringProvided(payload.KeyLength, payload.KeyLengthProvided) {
		payload.KeyLength = existing.KeyLength
	}
	if !stringProvided(payload.CustomArgs, payload.CustomArgsProvided) {
		payload.CustomArgs = existing.CustomArgs
	}
	if !uintProvided(payload.AcmeAccountID, payload.AcmeAccountProvided) {
		payload.AcmeAccountID = existing.AcmeAccountID
	}
	if !uintProvided(payload.DNSAccountID, payload.DNSAccountProvided) {
		payload.DNSAccountID = existing.DNSAccountID
	}
	if !payload.AutoRenewProvided {
		payload.AutoRenew = existing.AutoRenew
	}
	if !stringProvided(payload.Remark, payload.RemarkProvided) {
		payload.Remark = existing.Remark
	}
	if !stringProvided(payload.ApplyTarget, payload.ApplyTargetProvided) {
		payload.ApplyTarget = existing.ApplyTarget
	}

	pushProvided := stringProvided(payload.PushDir, payload.PushDirProvided)
	if !pushProvided {
		if existing.PushEnabled {
			payload.PushDir = existing.PushDir
		} else {
			payload.PushDir = ""
		}
		// The verified push state itself remains on the record. Post actions use
		// it to rewrite the directory after the replacement bundle is persisted.
		payload.PushExplicit = false
	} else if strings.TrimSpace(payload.PushDir) != "" {
		payload.PushExplicit = true
	}
	// DNSEnvText deliberately remains empty when omitted. DNS credentials live
	// in the selected DNS account and must never be revived from old records.
}

func (s *AcmeService) upsertAcmeCertificateRecordFromPaths(
	existingRecordID uint,
	domains []string,
	certificateType string,
	challenge string,
	keyLength string,
	caServer string,
	useECC bool,
	webroot string,
	dnsProvider string,
	customArgs string,
	account *model.AcmeAccount,
	dnsAccount *model.AcmeDNSAccount,
	paths *acmeManagedCertPaths,
	autoRenew bool,
	remark string,
	applyTarget string,
	pushDir string,
) (*model.CertificateRecord, error) {
	if len(domains) == 0 {
		return nil, common.NewError("domains are empty")
	}
	if paths == nil {
		return nil, common.NewError("paths are empty")
	}
	certPEM, keyPEM, fullchainPEM, chainPEM, err := readCertificateBundle(paths)
	if err != nil {
		return nil, err
	}
	fingerprint, notBefore, notAfter, err := inspectCertificateFingerprint(certPEM, keyPEM)
	if err != nil {
		return nil, err
	}

	existing := (*model.CertificateRecord)(nil)
	sourceRef := fmt.Sprintf("managed-%d", time.Now().UnixNano())
	if existingRecordID > 0 {
		row, err := certificateInventory.GetRecordByID(existingRecordID)
		if err != nil {
			return nil, err
		}
		if strings.TrimSpace(row.SourceType) != CertificateSourceACME {
			return nil, common.NewError("只有 ACME 证书可以重新签发")
		}
		existing = row
		sourceRef = strings.TrimSpace(row.SourceRef)
		if sourceRef == "" {
			sourceRef = fmt.Sprintf("managed-%d", time.Now().UnixNano())
		}
	}

	accountID := uint(0)
	accountName := ""
	runtimeProfile := ""
	if normalizeAcmeCertificateType(certificateType) == acmeCertificateTypeIP {
		runtimeProfile = acmeIPRuntimeProfile
	} else if account != nil {
		accountID = account.Id
		accountName = strings.TrimSpace(account.Name)
	}
	dnsAccountID := uint(0)
	dnsAccountName := ""
	if dnsAccount != nil && shouldUseAcmeDNSChallenge(certificateType, challenge) {
		dnsAccountID = dnsAccount.Id
		dnsAccountName = strings.TrimSpace(dnsAccount.Name)
	}

	if existing != nil {
		if strings.TrimSpace(applyTarget) == "" {
			applyTarget = strings.TrimSpace(existing.ApplyTarget)
		}
		if strings.TrimSpace(pushDir) == "" {
			pushDir = strings.TrimSpace(existing.PushDir)
		}
	}
	lastIssuedAt := time.Now().Unix()
	lastRenewedAt := lastIssuedAt
	pushFiles := ""
	listOrderAt := int64(0)
	if existing != nil {
		pushFiles = strings.TrimSpace(existing.PushFiles)
		listOrderAt = existing.ListOrderAt
	}

	return certificateInventory.Upsert(CertificateUpsertPayload{
		SourceType:         CertificateSourceACME,
		SourceRef:          sourceRef,
		MainDomain:         domains[0],
		Domains:            domains,
		CertificateType:    normalizeAcmeCertificateType(certificateType),
		CertProfile:        acmeCertProfileForType(certificateType),
		Challenge:          strings.TrimSpace(challenge),
		KeyLength:          strings.TrimSpace(keyLength),
		CAServer:           strings.TrimSpace(caServer),
		UseECC:             useECC,
		AutoRenew:          autoRenew,
		AcmeAccountID:      accountID,
		AcmeAccountName:    accountName,
		DNSAccountID:       dnsAccountID,
		DNSAccountName:     dnsAccountName,
		AcmeRuntimeProfile: runtimeProfile,
		ApplyTarget:        strings.TrimSpace(applyTarget),
		PushDir:            strings.TrimSpace(pushDir),
		PushFiles:          pushFiles,
		Remark:             strings.TrimSpace(remark),
		AcmeHome:           "",
		Webroot:            strings.TrimSpace(webroot),
		DNSProvider:        strings.TrimSpace(dnsProvider),
		DNSEnvText:         "",
		CustomArgs:         strings.TrimSpace(customArgs),
		CertPath:           "",
		KeyPath:            "",
		FullchainPath:      "",
		ChainPath:          "",
		CertPEM:            certPEM,
		KeyPEM:             keyPEM,
		FullchainPEM:       fullchainPEM,
		ChainPEM:           chainPEM,
		Fingerprint:        fingerprint,
		NotBefore:          notBefore.Unix(),
		NotAfter:           notAfter.Unix(),
		LastIssuedAt:       lastIssuedAt,
		LastRenewedAt:      lastRenewedAt,
		LastOutput:         "",
		ListOrderAt:        listOrderAt,
	})
}
