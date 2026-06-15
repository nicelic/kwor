package service

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

const (
	panelSelfSignedValidity = 30 * 24 * time.Hour
)

const panelSelfSignedLetsEncryptAuthorityCode = "letsencrypt"

type PanelSelfSignedResult struct {
	CertPath        string
	KeyPath         string
	CertPEM         []byte
	KeyPEM          []byte
	Identity        string
	IdentityKind    string
	DetectionReason string
	NotAfter        time.Time
}

func (r *PanelSelfSignedResult) withInspectionFallback(inspect PanelCertificateInspection) *PanelSelfSignedResult {
	if r == nil {
		return nil
	}
	if r.NotAfter.IsZero() && !inspect.NotAfter.IsZero() {
		r.NotAfter = inspect.NotAfter
	}
	return r
}

func GeneratePanelSelfSignedCertificate(certPath string, keyPath string) (*PanelSelfSignedResult, error) {
	return generatePanelSelfSignedCertificateAt(certPath, keyPath, time.Now())
}

func GeneratePanelSelfSignedCertificateInDir(certDir string) (*PanelSelfSignedResult, error) {
	certPath := filepath.Join(certDir, "fullchain.pem")
	keyPath := filepath.Join(certDir, "privkey.pem")
	return generatePanelSelfSignedCertificateAt(certPath, keyPath, time.Now())
}

func GeneratePanelSelfSignedCertificatePEM(now time.Time) (*PanelSelfSignedResult, error) {
	if now.IsZero() {
		now = time.Now()
	}
	return generatePanelSelfSignedCertificatePEM(now)
}

func BuildPanelSelfSignedRenewConfig(result *PanelSelfSignedResult) string {
	cfg := SelfSignedRenewConfig{
		Mode:               "panel_bootstrap",
		CertificateType:    "domain",
		KeyAlgorithm:       defaultSelfSignedAlgorithm,
		SignatureAlgorithm: defaultSelfSignedAlgorithm,
		DurationValue:      defaultSelfSignedDurationValue,
		DurationUnit:       defaultSelfSignedDurationUnit,
		PlatformCode:       panelSelfSignedLetsEncryptAuthorityCode,
		PlatformName:       "Let's Encrypt",
	}
	if result != nil {
		if strings.TrimSpace(result.Identity) != "" {
			cfg.Domains = []string{strings.TrimSpace(result.Identity)}
		}
		cfg.Identity = strings.TrimSpace(result.Identity)
		cfg.IdentityKind = strings.TrimSpace(result.IdentityKind)
		cfg.DetectionReason = strings.TrimSpace(result.DetectionReason)
	}
	return marshalSelfSignedRenewConfig(cfg)
}

func generatePanelSelfSignedCertificateAt(certPath string, keyPath string, now time.Time) (*PanelSelfSignedResult, error) {
	certPath = strings.TrimSpace(certPath)
	keyPath = strings.TrimSpace(keyPath)
	if certPath == "" || keyPath == "" {
		return nil, fmt.Errorf("certificate/key path is empty")
	}

	result, err := generatePanelSelfSignedCertificatePEM(now)
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(filepath.Dir(certPath), 0o755); err != nil {
		return nil, fmt.Errorf("failed to create certificate directory: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(keyPath), 0o755); err != nil {
		return nil, fmt.Errorf("failed to create key directory: %w", err)
	}

	if err := os.WriteFile(certPath, result.CertPEM, 0o644); err != nil {
		return nil, fmt.Errorf("failed to write certificate: %w", err)
	}

	if err := os.WriteFile(keyPath, result.KeyPEM, 0o600); err != nil {
		return nil, fmt.Errorf("failed to write private key: %w", err)
	}

	result.CertPath = certPath
	result.KeyPath = keyPath
	return result, nil
}

func generatePanelSelfSignedCertificatePEM(now time.Time) (*PanelSelfSignedResult, error) {
	detect := detectPublicIPForPanelCert()

	privateKey, err := ecdsa.GenerateKey(elliptic.P384(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate private key: %w", err)
	}

	serialNumber, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, fmt.Errorf("failed to generate serial number: %w", err)
	}

	notBefore := now.Add(-5 * time.Minute)
	notAfter := now.Add(panelSelfSignedValidity)

	template := x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			CommonName: "kwor-self-signed",
		},
		NotBefore:             notBefore,
		NotAfter:              notAfter,
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		SignatureAlgorithm:    x509.ECDSAWithSHA384,
	}

	if detect.ip != nil {
		template.Subject.CommonName = detect.ip.String()
		template.IPAddresses = []net.IP{detect.ip}
	}

	certDER, err := x509.CreateCertificate(rand.Reader, &template, &template, &privateKey.PublicKey, privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create certificate: %w", err)
	}

	privateKeyBytes, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal private key: %w", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateKeyBytes})

	return &PanelSelfSignedResult{
		CertPEM:         certPEM,
		KeyPEM:          keyPEM,
		Identity:        detect.identity,
		IdentityKind:    detect.kind,
		DetectionReason: detect.reason,
		NotAfter:        notAfter,
	}, nil
}

type panelIPDetectResult struct {
	ip       net.IP
	identity string
	kind     string
	reason   string
}

func detectPublicIPForPanelCert() panelIPDetectResult {
	commands := [][]string{
		{"ip", "-4", "addr"},
		{"ip", "a"},
		{"ip", "addr"},
	}

	var firstPublicIPv6 net.IP
	var anyCommandSucceeded bool

	for _, command := range commands {
		output, err := exec.Command(command[0], command[1:]...).Output()
		if err != nil {
			continue
		}
		anyCommandSucceeded = true

		ipv4List, ipv6List := parsePublicIPsFromIPCommandOutput(string(output))
		if len(ipv4List) > 0 {
			ip := ipv4List[0]
			return panelIPDetectResult{
				ip:       ip,
				identity: ip.String(),
				kind:     "ipv4",
				reason:   "public ipv4 detected from ip command output",
			}
		}
		if firstPublicIPv6 == nil && len(ipv6List) > 0 {
			firstPublicIPv6 = ipv6List[0]
		}
	}

	if firstPublicIPv6 != nil {
		return panelIPDetectResult{
			ip:       firstPublicIPv6,
			identity: firstPublicIPv6.String(),
			kind:     "ipv6",
			reason:   "public ipv6 detected from ip command output",
		}
	}

	if !anyCommandSucceeded {
		return panelIPDetectResult{
			kind:   "none",
			reason: "ip command execution failed; issuing self-signed certificate without sni identity",
		}
	}

	return panelIPDetectResult{
		kind:   "none",
		reason: "no public ip found in ip command output; issuing self-signed certificate without sni identity",
	}
}

func parsePublicIPsFromIPCommandOutput(output string) ([]net.IP, []net.IP) {
	lines := strings.Split(output, "\n")
	ipv4List := make([]net.IP, 0)
	ipv6List := make([]net.IP, 0)
	seen := map[string]struct{}{}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || !strings.Contains(trimmed, "scope global") {
			continue
		}

		fields := strings.Fields(trimmed)
		if len(fields) < 2 {
			continue
		}

		addr := fields[1]
		slash := strings.Index(addr, "/")
		if slash <= 0 {
			continue
		}
		ipText := addr[:slash]
		ip := net.ParseIP(ipText)
		if ip == nil {
			continue
		}
		if _, exists := seen[ip.String()]; exists {
			continue
		}

		if ipv4 := ip.To4(); ipv4 != nil {
			if !isPublicIPv4(ipv4) {
				continue
			}
			seen[ipv4.String()] = struct{}{}
			ipv4List = append(ipv4List, ipv4)
			continue
		}

		if !isPublicIPv6(ip) {
			continue
		}
		seen[ip.String()] = struct{}{}
		ipv6List = append(ipv6List, ip)
	}

	return ipv4List, ipv6List
}

func isPublicIPv4(ip net.IP) bool {
	ip = ip.To4()
	if ip == nil {
		return false
	}
	if !ip.IsGlobalUnicast() || ip.IsLoopback() || ip.IsPrivate() || ip.IsMulticast() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() {
		return false
	}

	for _, blocked := range blockedIPv4Networks {
		if blocked.Contains(ip) {
			return false
		}
	}
	return true
}

func isPublicIPv6(ip net.IP) bool {
	if ip == nil || ip.To4() != nil {
		return false
	}
	if !ip.IsGlobalUnicast() || ip.IsLoopback() || ip.IsPrivate() || ip.IsMulticast() || ip.IsUnspecified() || ip.IsLinkLocalUnicast() {
		return false
	}

	for _, blocked := range blockedIPv6Networks {
		if blocked.Contains(ip) {
			return false
		}
	}
	return true
}

func mustCIDR(cidr string) *net.IPNet {
	_, network, err := net.ParseCIDR(cidr)
	if err != nil {
		panic(err)
	}
	return network
}

var blockedIPv4Networks = []*net.IPNet{
	mustCIDR("0.0.0.0/8"),
	mustCIDR("10.0.0.0/8"),
	mustCIDR("100.64.0.0/10"),
	mustCIDR("127.0.0.0/8"),
	mustCIDR("169.254.0.0/16"),
	mustCIDR("172.16.0.0/12"),
	mustCIDR("192.0.0.0/24"),
	mustCIDR("192.0.2.0/24"),
	mustCIDR("192.168.0.0/16"),
	mustCIDR("198.18.0.0/15"),
	mustCIDR("198.51.100.0/24"),
	mustCIDR("203.0.113.0/24"),
	mustCIDR("224.0.0.0/4"),
	mustCIDR("240.0.0.0/4"),
}

var blockedIPv6Networks = []*net.IPNet{
	mustCIDR("::/128"),
	mustCIDR("::1/128"),
	mustCIDR("fc00::/7"),
	mustCIDR("fe80::/10"),
	mustCIDR("ff00::/8"),
	mustCIDR("2001:db8::/32"),
	mustCIDR("fec0::/10"),
	mustCIDR("2001:10::/28"),
}
