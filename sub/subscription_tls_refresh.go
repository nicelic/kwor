package sub

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"os"
	"strings"

	"github.com/alireza0/s-ui/database/model"
)

func refreshSubscriptionOutboundTLS(outbound map[string]interface{}, tlsConfig *model.Tls) {
	if outbound == nil || tlsConfig == nil {
		return
	}

	outboundTLS, ok := outbound["tls"].(map[string]interface{})
	if !ok || outboundTLS == nil {
		return
	}

	serverTLS := decodeSubscriptionTLSRaw(tlsConfig.Server)
	clientTLS := decodeSubscriptionTLSRaw(tlsConfig.Client)

	includeServerCertificate := true
	if include, ok := clientTLS["include_server_certificate"].(bool); ok {
		includeServerCertificate = include
	}
	includeServerFingerprint := shouldIncludeSubscriptionClashFingerprint(clientTLS)
	useServerCertificateSHA256 := hasNonEmptyTLSHash(clientTLS["certificate_public_key_sha256"])
	if !useServerCertificateSHA256 {
		useServerCertificateSHA256 = hasNonEmptyTLSHash(outboundTLS["certificate_public_key_sha256"])
	}

	serverCertLines, serverCertPEM, hasServerCert := loadSubscriptionPEM(serverTLS["certificate"], serverTLS["certificate_path"], "CERTIFICATE")
	if includeServerCertificate {
		if hasServerCert {
			if useServerCertificateSHA256 {
				if sha256Value, ok := calculateSubscriptionTLSPublicKeySHA256(serverCertPEM); ok {
					outboundTLS["certificate_public_key_sha256"] = []string{sha256Value}
				} else {
					delete(outboundTLS, "certificate_public_key_sha256")
				}
				delete(outboundTLS, "certificate")
			} else {
				outboundTLS["certificate"] = serverCertLines
				delete(outboundTLS, "certificate_public_key_sha256")
			}
		} else {
			delete(outboundTLS, "certificate")
			delete(outboundTLS, "certificate_public_key_sha256")
		}
	} else {
		delete(outboundTLS, "certificate")
		delete(outboundTLS, "certificate_public_key_sha256")
	}
	if includeServerFingerprint && hasServerCert {
		if fingerprint, ok := calculateSubscriptionTLSFingerprint(serverCertPEM); ok {
			outboundTLS["fingerprint"] = fingerprint
		} else {
			delete(outboundTLS, "fingerprint")
		}
	} else {
		delete(outboundTLS, "fingerprint")
	}

	if clientCertLines, _, ok := loadSubscriptionPEM(clientTLS["client_certificate"], clientTLS["client_certificate_path"], "CERTIFICATE"); ok {
		outboundTLS["client_certificate"] = clientCertLines
	}
	if clientKeyLines, ok := loadSubscriptionTextLines(clientTLS["client_key"], clientTLS["client_key_path"]); ok {
		outboundTLS["client_key"] = clientKeyLines
	}
}

func shouldIncludeSubscriptionClashFingerprint(clientTLS map[string]interface{}) bool {
	if include, ok := clientTLS["include_server_fingerprint"].(bool); ok {
		return include
	}
	return true
}

func hasNonEmptyTLSHash(raw interface{}) bool {
	switch value := raw.(type) {
	case []string:
		for _, item := range value {
			if strings.TrimSpace(item) != "" {
				return true
			}
		}
	case []interface{}:
		for _, item := range value {
			text, ok := item.(string)
			if !ok {
				continue
			}
			if strings.TrimSpace(text) != "" {
				return true
			}
		}
	case string:
		return strings.TrimSpace(value) != ""
	}
	return false
}

func decodeSubscriptionTLSRaw(raw json.RawMessage) map[string]interface{} {
	if len(raw) == 0 {
		return nil
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil
	}
	return decoded
}

func loadSubscriptionPEM(contentRaw interface{}, pathRaw interface{}, requiredBlock string) ([]string, []byte, bool) {
	lines, rawBytes, ok := loadSubscriptionRawBytes(contentRaw, pathRaw)
	if !ok {
		return nil, nil, false
	}
	if !strings.Contains(string(rawBytes), "BEGIN "+requiredBlock) {
		return nil, nil, false
	}
	return lines, rawBytes, true
}

func loadSubscriptionTextLines(contentRaw interface{}, pathRaw interface{}) ([]string, bool) {
	lines, _, ok := loadSubscriptionRawBytes(contentRaw, pathRaw)
	return lines, ok
}

func loadSubscriptionRawBytes(contentRaw interface{}, pathRaw interface{}) ([]string, []byte, bool) {
	if lines, rawBytes, ok := loadSubscriptionRawBytesFromPath(pathRaw); ok {
		return lines, rawBytes, true
	}

	if lines, ok := normalizeSubscriptionLines(contentRaw); ok {
		pemText := strings.Join(lines, "\n")
		return lines, []byte(pemText + "\n"), true
	}

	return nil, nil, false
}

func loadSubscriptionRawBytesFromPath(pathRaw interface{}) ([]string, []byte, bool) {
	path, ok := pathRaw.(string)
	if !ok {
		return nil, nil, false
	}
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, nil, false
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, false
	}

	normalized := strings.TrimSpace(strings.ReplaceAll(string(data), "\r\n", "\n"))
	if normalized == "" {
		return nil, nil, false
	}

	lines := strings.Split(normalized, "\n")
	return lines, []byte(normalized + "\n"), true
}

func normalizeSubscriptionLines(raw interface{}) ([]string, bool) {
	switch typed := raw.(type) {
	case []string:
		lines := filterSubscriptionLines(typed)
		return lines, len(lines) > 0
	case []interface{}:
		lines := make([]string, 0, len(typed))
		for _, item := range typed {
			value, ok := item.(string)
			if !ok {
				return nil, false
			}
			lines = append(lines, strings.TrimSpace(strings.TrimSuffix(value, "\r")))
		}
		lines = filterSubscriptionLines(lines)
		return lines, len(lines) > 0
	case string:
		normalized := strings.TrimSpace(strings.ReplaceAll(typed, "\r\n", "\n"))
		if normalized == "" {
			return nil, false
		}
		lines := filterSubscriptionLines(strings.Split(normalized, "\n"))
		return lines, len(lines) > 0
	default:
		return nil, false
	}
}

func filterSubscriptionLines(lines []string) []string {
	filtered := make([]string, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if trimmed == "" {
			continue
		}
		filtered = append(filtered, trimmed)
	}
	return filtered
}

func parseSubscriptionCertificates(certPEM []byte) ([]*x509.Certificate, bool) {
	rest := certPEM
	certs := make([]*x509.Certificate, 0)

	for len(rest) > 0 {
		var block *pem.Block
		block, rest = pem.Decode(rest)
		if block == nil {
			break
		}
		if block.Type != "CERTIFICATE" {
			continue
		}
		cert, err := x509.ParseCertificate(block.Bytes)
		if err != nil {
			return nil, false
		}
		certs = append(certs, cert)
	}

	return certs, len(certs) > 0
}

func calculateSubscriptionTLSPublicKeySHA256(certPEM []byte) (string, bool) {
	certs, ok := parseSubscriptionCertificates(certPEM)
	if !ok {
		return "", false
	}

	publicKeyDER, err := x509.MarshalPKIXPublicKey(certs[0].PublicKey)
	if err != nil {
		return "", false
	}

	sum := sha256.Sum256(publicKeyDER)
	return base64.StdEncoding.EncodeToString(sum[:]), true
}

func calculateSubscriptionTLSFingerprint(certPEM []byte) (string, bool) {
	certs, ok := parseSubscriptionCertificates(certPEM)
	if !ok {
		return "", false
	}

	sum := sha256.Sum256(certs[0].Raw)
	hexStr := strings.ToUpper(hex.EncodeToString(sum[:]))
	parts := make([]string, 0, len(hexStr)/2)
	for i := 0; i < len(hexStr); i += 2 {
		parts = append(parts, hexStr[i:i+2])
	}
	return strings.Join(parts, ":"), true
}
