package service

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/alireza0/s-ui/config"
	"github.com/alireza0/s-ui/logger"
)

const panelSelfSignedTimeLayout = "2006-01-02-15-04-05"

type PanelSelfSignedTarget string

const (
	PanelSelfSignedTargetPanel PanelSelfSignedTarget = "panel"
	PanelSelfSignedTargetSub   PanelSelfSignedTarget = "sub"
)

func (t PanelSelfSignedTarget) isValid() bool {
	return t == PanelSelfSignedTargetPanel || t == PanelSelfSignedTargetSub
}

func ResolvePanelSelfSignedCertificatePaths(settingService *SettingService, target PanelSelfSignedTarget, now time.Time) (string, string, error) {
	if settingService == nil {
		return "", "", fmt.Errorf("setting service is nil")
	}
	if !target.isValid() {
		return "", "", fmt.Errorf("invalid panel self-signed target: %q", target)
	}
	if now.IsZero() {
		now = time.Now()
	}

	baseName, err := resolvePanelSelfSignedBaseName(settingService, target, now)
	if err != nil {
		return "", "", err
	}

	certDir := filepath.Join(config.GetDataDir(), "cert", baseName+"_"+string(target))
	certPath := filepath.Join(certDir, "fullchain.pem")
	keyPath := filepath.Join(certDir, "privkey.pem")
	return certPath, keyPath, nil
}

func GeneratePanelSelfSignedCertificateForTarget(settingService *SettingService, target PanelSelfSignedTarget, now time.Time) (*PanelSelfSignedResult, error) {
	if now.IsZero() {
		now = time.Now()
	}
	certPath, keyPath, err := ResolvePanelSelfSignedCertificatePaths(settingService, target, now)
	if err != nil {
		return nil, err
	}
	return generatePanelSelfSignedCertificateAt(certPath, keyPath, now)
}

// SplitLegacySharedPanelSelfSignedCertificate migrates old shared self-signed paths:
// Promanager_data/cert/fullchain.pem + privkey.pem -> separate panel/sub directories.
func SplitLegacySharedPanelSelfSignedCertificate(settingService *SettingService) (bool, error) {
	if settingService == nil {
		return false, fmt.Errorf("setting service is nil")
	}

	webCert, err := settingService.GetCertFile()
	if err != nil {
		return false, err
	}
	webKey, err := settingService.GetKeyFile()
	if err != nil {
		return false, err
	}
	subCert, err := settingService.GetSubCertFile()
	if err != nil {
		return false, err
	}
	subKey, err := settingService.GetSubKeyFile()
	if err != nil {
		return false, err
	}

	webCert = strings.TrimSpace(webCert)
	webKey = strings.TrimSpace(webKey)
	subCert = strings.TrimSpace(subCert)
	subKey = strings.TrimSpace(subKey)

	if webCert == "" || webKey == "" || subCert == "" || subKey == "" {
		return false, nil
	}
	if !sameFilePath(webCert, subCert) || !sameFilePath(webKey, subKey) {
		return false, nil
	}

	legacyCertPath := filepath.Join(config.GetDataDir(), "cert", "fullchain.pem")
	legacyKeyPath := filepath.Join(config.GetDataDir(), "cert", "privkey.pem")
	if !sameFilePath(webCert, legacyCertPath) || !sameFilePath(webKey, legacyKeyPath) {
		return false, nil
	}

	selfSigned, err := isSelfSignedCertificatePair(webCert, webKey)
	if err != nil || !selfSigned {
		return false, nil
	}

	now := time.Now()
	panelResult, err := GeneratePanelSelfSignedCertificateForTarget(settingService, PanelSelfSignedTargetPanel, now)
	if err != nil {
		return false, fmt.Errorf("generate panel self-signed certificate failed: %w", err)
	}
	subResult, err := GeneratePanelSelfSignedCertificateForTarget(settingService, PanelSelfSignedTargetSub, now)
	if err != nil {
		return false, fmt.Errorf("generate sub self-signed certificate failed: %w", err)
	}

	if err := settingService.SaveSetting("webCertFile", panelResult.CertPath); err != nil {
		return false, fmt.Errorf("set web cert path failed: %w", err)
	}
	if err := settingService.SaveSetting("webKeyFile", panelResult.KeyPath); err != nil {
		return false, fmt.Errorf("set web key path failed: %w", err)
	}
	if err := settingService.SaveSetting("subCertFile", subResult.CertPath); err != nil {
		return false, fmt.Errorf("set sub cert path failed: %w", err)
	}
	if err := settingService.SaveSetting("subKeyFile", subResult.KeyPath); err != nil {
		return false, fmt.Errorf("set sub key path failed: %w", err)
	}

	logger.Infof(
		"[PanelCert] migrated legacy shared self-signed cert paths: web=%s sub=%s",
		panelResult.CertPath,
		subResult.CertPath,
	)
	return true, nil
}

func resolvePanelSelfSignedBaseName(settingService *SettingService, target PanelSelfSignedTarget, now time.Time) (string, error) {
	candidates, err := panelSelfSignedSNICandidates(settingService, target)
	if err != nil {
		return "", err
	}
	for _, candidate := range candidates {
		if normalized, ok := normalizePanelSelfSignedName(candidate); ok {
			return normalized, nil
		}
	}

	detected := detectPublicIPForPanelCert()
	if detected.kind == "ipv4" && detected.ip != nil {
		if normalized, ok := normalizePanelSelfSignedName(detected.ip.String()); ok {
			return normalized, nil
		}
	}

	return panelSelfSignedTimestamp(now), nil
}

func panelSelfSignedSNICandidates(settingService *SettingService, target PanelSelfSignedTarget) ([]string, error) {
	switch target {
	case PanelSelfSignedTargetPanel:
		domain, err := settingService.GetWebDomain()
		if err != nil {
			return nil, err
		}
		listen, err := settingService.GetListen()
		if err != nil {
			return nil, err
		}
		return []string{domain, listen}, nil
	case PanelSelfSignedTargetSub:
		domain, err := settingService.GetSubDomain()
		if err != nil {
			return nil, err
		}
		listen, err := settingService.GetSubListen()
		if err != nil {
			return nil, err
		}
		return []string{domain, listen}, nil
	default:
		return nil, fmt.Errorf("invalid panel self-signed target: %q", target)
	}
}

func normalizePanelSelfSignedName(raw string) (string, bool) {
	host := normalizePanelSelfSignedHost(raw)
	if host == "" {
		return "", false
	}

	if parsedIP := net.ParseIP(host); parsedIP != nil {
		if ipv4 := parsedIP.To4(); ipv4 != nil {
			if ipv4.IsUnspecified() {
				return "", false
			}
			return ipv4.String(), true
		}
		// IPv6 path must use timestamp fallback.
		return "", false
	}

	normalized := strings.ToLower(host)
	normalized = strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '.' || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, normalized)
	normalized = strings.Trim(normalized, "._-")
	if normalized == "" {
		return "", false
	}
	return normalized, true
}

func normalizePanelSelfSignedHost(raw string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		return ""
	}
	value = strings.TrimSpace(strings.TrimSuffix(value, "."))
	if value == "" {
		return ""
	}

	// Bracketed IPv6 can include port, for example [2001:db8::1]:443.
	if strings.HasPrefix(value, "[") {
		if right := strings.Index(value, "]"); right > 0 {
			host := value[1:right]
			remainder := strings.TrimSpace(value[right+1:])
			if remainder == "" || strings.HasPrefix(remainder, ":") {
				value = host
			}
		}
	}

	if host, port, err := net.SplitHostPort(value); err == nil && port != "" {
		value = host
	} else if strings.Count(value, ":") == 1 {
		// Non-bracket host:port form.
		index := strings.LastIndex(value, ":")
		if index > 0 && index < len(value)-1 {
			if _, err := strconv.Atoi(value[index+1:]); err == nil {
				value = value[:index]
			}
		}
	}

	value = strings.TrimSpace(strings.Trim(value, "[]"))
	value = strings.TrimSpace(strings.TrimSuffix(value, "."))
	if isWildcardHost(value) {
		return ""
	}
	return value
}

func panelSelfSignedTimestamp(now time.Time) string {
	if now.IsZero() {
		now = time.Now()
	}
	stamp := now.Format(panelSelfSignedTimeLayout)
	if strings.TrimSpace(stamp) != "" {
		return stamp
	}
	return strconv.FormatInt(now.Unix(), 10)
}

func isSelfSignedCertificatePair(certPath string, keyPath string) (bool, error) {
	certPEM, err := os.ReadFile(certPath)
	if err != nil {
		return false, err
	}
	keyPEM, err := os.ReadFile(keyPath)
	if err != nil {
		return false, err
	}

	pair, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return false, err
	}
	if len(pair.Certificate) == 0 {
		return false, fmt.Errorf("empty certificate chain")
	}

	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		return false, err
	}
	if !bytes.Equal(leaf.RawIssuer, leaf.RawSubject) {
		return false, nil
	}
	return leaf.CheckSignature(leaf.SignatureAlgorithm, leaf.RawTBSCertificate, leaf.Signature) == nil, nil
}

func sameFilePath(pathA string, pathB string) bool {
	cleanA := filepath.Clean(strings.TrimSpace(pathA))
	cleanB := filepath.Clean(strings.TrimSpace(pathB))
	if runtime.GOOS == "windows" {
		return strings.EqualFold(cleanA, cleanB)
	}
	return cleanA == cleanB
}

func isWildcardHost(host string) bool {
	switch strings.TrimSpace(strings.Trim(host, "[]")) {
	case "", "0.0.0.0", "::":
		return true
	default:
		return false
	}
}
