package service

import (
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	"github.com/alireza0/s-ui/network"
)

// MigrateLegacySettingsPathCertificatesToInventory migrates legacy
// webCertFile/webKeyFile/subCertFile/subKeyFile based certificates into
// certificate inventory so panel/sub only consume inventory assignments.
func MigrateLegacySettingsPathCertificatesToInventory(settingService *SettingService) error {
	if settingService == nil {
		return nil
	}

	type getterPair struct {
		getCert func() (string, error)
		getKey  func() (string, error)
	}

	getters := map[PanelSelfSignedTarget]getterPair{
		PanelSelfSignedTargetPanel: {getCert: settingService.GetCertFile, getKey: settingService.GetKeyFile},
		PanelSelfSignedTargetSub:   {getCert: settingService.GetSubCertFile, getKey: settingService.GetSubKeyFile},
	}

	for _, target := range []PanelSelfSignedTarget{PanelSelfSignedTargetPanel, PanelSelfSignedTargetSub} {
		pair := getters[target]
		certPath, err := pair.getCert()
		if err != nil {
			return err
		}
		keyPath, err := pair.getKey()
		if err != nil {
			return err
		}
		certPath = strings.TrimSpace(certPath)
		keyPath = strings.TrimSpace(keyPath)
		if certPath == "" || keyPath == "" {
			continue
		}

		certPEM, keyPEM, readErr := LoadPanelCertificatePEMFromPaths(certPath, keyPath)
		if readErr != nil {
			continue
		}

		tlsPair, pairErr := tls.X509KeyPair(certPEM, keyPEM)
		if pairErr != nil {
			continue
		}
		leaf, leafErr := network.ParseLeafCertificate(&tlsPair)
		if leafErr != nil || leaf == nil {
			continue
		}

		domains := collectLegacyLeafDomains(leaf)
		mainDomain := panelTargetFallbackDomain(target)
		if len(domains) > 0 {
			mainDomain = domains[0]
		}

		fingerprint := ""
		if len(leaf.Raw) > 0 {
			sum := sha256.Sum256(leaf.Raw)
			fingerprint = hex.EncodeToString(sum[:])
		}

		now := time.Now().Unix()
		row, upsertErr := certificateInventory.Upsert(CertificateUpsertPayload{
			SourceType: CertificateSourceImported,
			SourceRef:  BuildImportedSourceRef(target),

			MainDomain: mainDomain,
			Domains:    domains,

			CertificateType: "domain",
			AutoRenew:       false,
			Remark:          "legacy settings path certificate migrated",

			CertPath:      certPath,
			KeyPath:       keyPath,
			FullchainPath: certPath,

			CertPEM:      certPEM,
			KeyPEM:       keyPEM,
			FullchainPEM: certPEM,

			Fingerprint:   fingerprint,
			NotBefore:     leaf.NotBefore.Unix(),
			NotAfter:      leaf.NotAfter.Unix(),
			LastIssuedAt:  now,
			LastRenewedAt: now,
		})
		if upsertErr != nil {
			return upsertErr
		}
		if row == nil {
			continue
		}

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
	return nil
}

func clearLegacySettingsPathCertificateSource(settingService *SettingService, sourceRef string) error {
	if settingService == nil {
		return nil
	}

	target, ok := parseImportedSourceRefTarget(sourceRef)
	if !ok {
		return nil
	}

	switch target {
	case PanelSelfSignedTargetPanel:
		if err := settingService.SaveSetting("webCertFile", ""); err != nil {
			return err
		}
		if err := settingService.SaveSetting("webKeyFile", ""); err != nil {
			return err
		}
	case PanelSelfSignedTargetSub:
		if err := settingService.SaveSetting("subCertFile", ""); err != nil {
			return err
		}
		if err := settingService.SaveSetting("subKeyFile", ""); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported imported certificate target: %q", target)
	}

	return nil
}

func parseImportedSourceRefTarget(sourceRef string) (PanelSelfSignedTarget, bool) {
	switch strings.TrimSpace(sourceRef) {
	case BuildImportedSourceRef(PanelSelfSignedTargetPanel):
		return PanelSelfSignedTargetPanel, true
	case BuildImportedSourceRef(PanelSelfSignedTargetSub):
		return PanelSelfSignedTargetSub, true
	default:
		return "", false
	}
}

func collectLegacyLeafDomains(leaf *x509.Certificate) []string {
	if leaf == nil {
		return []string{}
	}
	seen := map[string]struct{}{}
	result := make([]string, 0)
	add := func(raw string) {
		value := strings.TrimSpace(strings.Trim(raw, "."))
		if value == "" {
			return
		}
		if _, ok := seen[value]; ok {
			return
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	for _, dns := range leaf.DNSNames {
		add(strings.ToLower(strings.TrimSpace(dns)))
	}
	for _, ip := range leaf.IPAddresses {
		add(strings.TrimSpace(ip.String()))
	}
	add(strings.TrimSpace(leaf.Subject.CommonName))
	return result
}
