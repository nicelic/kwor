package service

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"os"
	"strings"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/util"
)

func renderManagedSingboxSubscriptionJSON(
	rawOutbounds []map[string]interface{},
	othersStr string,
	resolveTLSStore func(string) string,
) ([]byte, error) {
	outbounds, outTags := expandSubOutboundsForSubscription(cloneManagedSubscriptionOutbounds(rawOutbounds))
	if outbounds == nil {
		outbounds = []map[string]interface{}{}
	}
	if outTags == nil {
		outTags = []string{}
	}

	outbounds, outTags = util.FilterTaggedSubscriptionOutbounds(
		outbounds,
		outTags,
		util.SupportsSingboxSubscriptionOutboundType,
	)
	if outbounds == nil {
		outbounds = []map[string]interface{}{}
	}
	if outTags == nil {
		outTags = []string{}
	}
	for _, outbound := range outbounds {
		util.SanitizeSingboxSubscriptionOutbound(outbound)
	}

	latencyURL := "http://www.gstatic.com/generate_204"
	latencyInterval := "10m"
	latencyTolerance := 50
	var extJSON map[string]interface{}
	if len(othersStr) > 0 {
		if err := json.Unmarshal([]byte(othersStr), &extJSON); err == nil {
			if value, ok := extJSON["latency_test_url"].(string); ok && value != "" {
				latencyURL = value
			}
			if value, ok := extJSON["latency_test_interval"].(string); ok && value != "" {
				if normalized, ok := normalizeManagedSingboxLatencyInterval(value); ok {
					latencyInterval = normalized
				}
			}
			if value, ok := extJSON["latency_tolerance"].(float64); ok && value > 0 {
				latencyTolerance = int(value)
			}
		}
	}

	selectorGroups := parseSelectorGroupsFromExt(extJSON)
	stripMihomoSubscriptionFields(outbounds)
	outbounds = append(buildManagedSubscriptionDefaultOutbounds(outTags, latencyURL, latencyInterval, latencyTolerance, selectorGroups), outbounds...)

	tlsStore := extractTlsStoreFromSubOutbounds(outbounds)
	if resolveTLSStore != nil {
		tlsStore = resolveTLSStore(tlsStore)
	}

	jsonConfig := buildSubJsonFullConfig(outbounds, othersStr)
	applyCertificateStoreToSubConfig(jsonConfig, tlsStore)
	return json.MarshalIndent(jsonConfig, "", "  ")
}

func buildManagedSubscriptionDefaultOutbounds(
	outTags []string,
	latencyURL string,
	latencyInterval string,
	latencyTolerance int,
	selectorGroups []selectorGroupConfig,
) []map[string]interface{} {
	defaultOutbounds := []map[string]interface{}{
		{
			"outbounds":                   append([]string{autoSelectorTag}, outTags...),
			"tag":                         nodeSelectorTag,
			"type":                        "selector",
			"interrupt_exist_connections": true,
		},
		{
			"tag":                         autoSelectorTag,
			"type":                        "urltest",
			"outbounds":                   outTags,
			"url":                         latencyURL,
			"interval":                    latencyInterval,
			"tolerance":                   latencyTolerance,
			"interrupt_exist_connections": true,
		},
		{
			"outbounds":                   append([]string{"direct", "block"}, outTags...),
			"tag":                         globalDirectSelectorTag,
			"type":                        "selector",
			"interrupt_exist_connections": true,
		},
		{
			"outbounds":                   append([]string{"block", "direct"}, outTags...),
			"tag":                         globalBlockSelectorTag,
			"type":                        "selector",
			"interrupt_exist_connections": true,
		},
		{
			"outbounds":                   append([]string{nodeSelectorTag, globalDirectSelectorTag}, outTags...),
			"tag":                         finalSelectorTag,
			"type":                        "selector",
			"interrupt_exist_connections": true,
		},
		{
			"outbounds":                   append([]string{nodeSelectorTag, autoSelectorTag, globalDirectSelectorTag, globalBlockSelectorTag, finalSelectorTag}, outTags...),
			"tag":                         globalSelectorTag,
			"type":                        "selector",
			"interrupt_exist_connections": true,
		},
	}
	defaultOutbounds = append(defaultOutbounds, buildNamedSelectorOutbounds(selectorGroups, outTags)...)
	defaultOutbounds = append(defaultOutbounds,
		map[string]interface{}{"type": "direct", "tag": "direct"},
		map[string]interface{}{"type": "block", "tag": "block"},
	)
	return defaultOutbounds
}

func cloneManagedSubscriptionOutbounds(rawOutbounds []map[string]interface{}) []map[string]interface{} {
	cloned := make([]map[string]interface{}, 0, len(rawOutbounds))
	for _, outbound := range rawOutbounds {
		if outbound == nil {
			continue
		}

		data, err := json.Marshal(outbound)
		if err != nil {
			cloned = append(cloned, cloneSubOutboundMap(outbound))
			continue
		}

		var copied map[string]interface{}
		if err := json.Unmarshal(data, &copied); err != nil {
			cloned = append(cloned, cloneSubOutboundMap(outbound))
			continue
		}
		cloned = append(cloned, copied)
	}
	return cloned
}

func normalizeManagedSingboxLatencyInterval(raw string) (string, bool) {
	value := strings.TrimSpace(strings.ToLower(raw))
	if len(value) < 2 {
		return "", false
	}

	unit := value[len(value)-1]
	if unit != 's' && unit != 'm' && unit != 'h' && unit != 'd' {
		return "", false
	}

	numberPart := strings.TrimSpace(value[:len(value)-1])
	if numberPart == "" {
		return "", false
	}

	var interval int
	if _, err := fmt.Sscanf(numberPart, "%d", &interval); err != nil || interval <= 0 {
		return "", false
	}

	return fmt.Sprintf("%d%c", interval, unit), true
}

func stripMihomoSubscriptionFields(outbounds []map[string]interface{}) {
	for _, outbound := range outbounds {
		delete(outbound, "mihomo_common")
		delete(outbound, "mihomo_hy2")
		delete(outbound, "mihomo_fast_open")
		delete(outbound, "fast_open")
		if tlsMap, ok := outbound["tls"].(map[string]interface{}); ok {
			delete(tlsMap, "mihomo_use_fingerprint")
			delete(tlsMap, "fingerprint")
			delete(tlsMap, "include_server_certificate")
			delete(tlsMap, "include_server_fingerprint")
		}
	}
}

func refreshManagedSubOutboundTLS(outbound map[string]interface{}, subOutbound *model.SubOutbound) {
	if outbound == nil || subOutbound == nil {
		return
	}
	if _, ok := outbound["tls"].(map[string]interface{}); !ok {
		return
	}

	if tlsConfig, ok := loadManagedSourceTLSForSubOutbound(subOutbound); ok && tlsConfig != nil {
		refreshManagedSubscriptionOutboundTLS(outbound, tlsConfig)
		return
	}

	if !shouldFallbackRefreshManagedSubOutboundTLS(subOutbound) {
		return
	}

	if tlsConfig, ok := buildManagedFallbackTLSConfigFromOutbound(outbound); ok && tlsConfig != nil {
		refreshManagedSubscriptionOutboundTLS(outbound, tlsConfig)
	}
}

func shouldFallbackRefreshManagedSubOutboundTLS(subOutbound *model.SubOutbound) bool {
	if subOutbound == nil {
		return true
	}
	if strings.TrimSpace(subOutbound.SourceType) == "" {
		return true
	}
	return false
}

func loadManagedSourceTLSForSubOutbound(subOutbound *model.SubOutbound) (*model.Tls, bool) {
	if subOutbound == nil || subOutbound.SourceInboundId == 0 {
		return nil, false
	}

	db := database.GetDB()
	switch strings.TrimSpace(subOutbound.SourceType) {
	case subOutboundSourceClient:
		inbound := &model.Inbound{}
		if err := db.Model(model.Inbound{}).
			Preload("Tls").
			Where("id = ?", subOutbound.SourceInboundId).
			First(inbound).Error; err != nil {
			return nil, false
		}
		if inbound.Tls == nil {
			return nil, false
		}
		return inbound.Tls, true
	case subOutboundSourceMihomoClient:
		inbound := &model.MihomoInbound{}
		if err := db.Model(model.MihomoInbound{}).
			Preload("Tls").
			Where("id = ?", subOutbound.SourceInboundId).
			First(inbound).Error; err != nil {
			return nil, false
		}
		if inbound.Tls == nil {
			return nil, false
		}
		return inbound.Tls.ToBase(), true
	default:
		return nil, false
	}
}

func buildManagedFallbackTLSConfigFromOutbound(outbound map[string]interface{}) (*model.Tls, bool) {
	if outbound == nil {
		return nil, false
	}

	tlsMap, ok := outbound["tls"].(map[string]interface{})
	if !ok || tlsMap == nil {
		return nil, false
	}

	raw, err := json.Marshal(tlsMap)
	if err != nil {
		return nil, false
	}

	server := append(json.RawMessage(nil), raw...)
	client := append(json.RawMessage(nil), raw...)
	return &model.Tls{Server: server, Client: client}, true
}

func refreshManagedSubscriptionOutboundTLS(outbound map[string]interface{}, tlsConfig *model.Tls) {
	if outbound == nil || tlsConfig == nil {
		return
	}

	outboundTLS, ok := outbound["tls"].(map[string]interface{})
	if !ok || outboundTLS == nil {
		return
	}

	serverTLS := decodeManagedSubscriptionTLSRaw(tlsConfig.Server)
	clientTLS := decodeManagedSubscriptionTLSRaw(tlsConfig.Client)

	includeServerCertificate := true
	if include, ok := clientTLS["include_server_certificate"].(bool); ok {
		includeServerCertificate = include
	}
	includeServerFingerprint := shouldIncludeManagedClashFingerprint(clientTLS)
	useServerCertificateSHA256 := hasNonEmptyManagedTLSHash(clientTLS["certificate_public_key_sha256"])
	if !useServerCertificateSHA256 {
		useServerCertificateSHA256 = hasNonEmptyManagedTLSHash(outboundTLS["certificate_public_key_sha256"])
	}

	serverCertLines, serverCertPEM, hasServerCert := loadManagedSubscriptionPEM(serverTLS["certificate"], serverTLS["certificate_path"], "CERTIFICATE")
	if includeServerCertificate {
		if hasServerCert {
			if useServerCertificateSHA256 {
				if sha256Value, ok := calculateManagedSubscriptionTLSPublicKeySHA256(serverCertPEM); ok {
					outboundTLS["certificate_public_key_sha256"] = []string{sha256Value}
				} else if !restoreManagedTLSHash(outboundTLS, clientTLS["certificate_public_key_sha256"]) {
					delete(outboundTLS, "certificate_public_key_sha256")
				}
				delete(outboundTLS, "certificate")
			} else {
				outboundTLS["certificate"] = serverCertLines
				delete(outboundTLS, "certificate_public_key_sha256")
			}
		} else {
			delete(outboundTLS, "certificate")
			if useServerCertificateSHA256 {
				if !restoreManagedTLSHash(outboundTLS, clientTLS["certificate_public_key_sha256"]) {
					delete(outboundTLS, "certificate_public_key_sha256")
				}
			} else {
				delete(outboundTLS, "certificate_public_key_sha256")
			}
		}
	} else {
		delete(outboundTLS, "certificate")
		delete(outboundTLS, "certificate_public_key_sha256")
	}
	if includeServerFingerprint && hasServerCert {
		if fingerprint, ok := calculateManagedSubscriptionTLSFingerprint(serverCertPEM); ok {
			outboundTLS["fingerprint"] = fingerprint
		} else {
			delete(outboundTLS, "fingerprint")
		}
	} else {
		delete(outboundTLS, "fingerprint")
	}

	if clientCertLines, _, ok := loadManagedSubscriptionPEM(clientTLS["client_certificate"], clientTLS["client_certificate_path"], "CERTIFICATE"); ok {
		outboundTLS["client_certificate"] = clientCertLines
	}
	if clientKeyLines, ok := loadManagedSubscriptionTextLines(clientTLS["client_key"], clientTLS["client_key_path"]); ok {
		outboundTLS["client_key"] = clientKeyLines
	}
}

func shouldIncludeManagedClashFingerprint(clientTLS map[string]interface{}) bool {
	if include, ok := clientTLS["include_server_fingerprint"].(bool); ok {
		return include
	}
	return true
}

func hasNonEmptyManagedTLSHash(raw interface{}) bool {
	_, ok := managedTLSHashStrings(raw)
	return ok
}

func restoreManagedTLSHash(outboundTLS map[string]interface{}, preferred interface{}) bool {
	if outboundTLS == nil {
		return false
	}
	if hashes, ok := managedTLSHashStrings(preferred); ok {
		outboundTLS["certificate_public_key_sha256"] = hashes
		return true
	}
	if hashes, ok := managedTLSHashStrings(outboundTLS["certificate_public_key_sha256"]); ok {
		outboundTLS["certificate_public_key_sha256"] = hashes
		return true
	}
	return false
}

func managedTLSHashStrings(raw interface{}) ([]string, bool) {
	result := make([]string, 0)
	switch value := raw.(type) {
	case []string:
		for _, item := range value {
			item = strings.TrimSpace(item)
			if item != "" {
				result = append(result, item)
			}
		}
	case []interface{}:
		for _, item := range value {
			text, ok := item.(string)
			if !ok {
				continue
			}
			text = strings.TrimSpace(text)
			if text != "" {
				result = append(result, text)
			}
		}
	case string:
		value = strings.TrimSpace(value)
		if value != "" {
			result = append(result, value)
		}
	}
	return result, len(result) > 0
}

func decodeManagedSubscriptionTLSRaw(raw json.RawMessage) map[string]interface{} {
	if len(raw) == 0 {
		return nil
	}

	var decoded map[string]interface{}
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil
	}
	return decoded
}

func loadManagedSubscriptionPEM(contentRaw interface{}, pathRaw interface{}, requiredBlock string) ([]string, []byte, bool) {
	lines, rawBytes, ok := loadManagedSubscriptionRawBytes(contentRaw, pathRaw)
	if !ok {
		return nil, nil, false
	}
	if !strings.Contains(string(rawBytes), "BEGIN "+requiredBlock) {
		return nil, nil, false
	}
	return lines, rawBytes, true
}

func loadManagedSubscriptionTextLines(contentRaw interface{}, pathRaw interface{}) ([]string, bool) {
	lines, _, ok := loadManagedSubscriptionRawBytes(contentRaw, pathRaw)
	return lines, ok
}

func loadManagedSubscriptionRawBytes(contentRaw interface{}, pathRaw interface{}) ([]string, []byte, bool) {
	if lines, rawBytes, ok := loadManagedSubscriptionRawBytesFromPath(pathRaw); ok {
		return lines, rawBytes, true
	}

	if lines, ok := normalizeManagedSubscriptionLines(contentRaw); ok {
		pemText := strings.Join(lines, "\n")
		return lines, []byte(pemText + "\n"), true
	}

	return nil, nil, false
}

func loadManagedSubscriptionRawBytesFromPath(pathRaw interface{}) ([]string, []byte, bool) {
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

func normalizeManagedSubscriptionLines(raw interface{}) ([]string, bool) {
	switch typed := raw.(type) {
	case []string:
		lines := filterManagedSubscriptionLines(typed)
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
		lines = filterManagedSubscriptionLines(lines)
		return lines, len(lines) > 0
	case string:
		normalized := strings.TrimSpace(strings.ReplaceAll(typed, "\r\n", "\n"))
		if normalized == "" {
			return nil, false
		}
		lines := filterManagedSubscriptionLines(strings.Split(normalized, "\n"))
		return lines, len(lines) > 0
	default:
		return nil, false
	}
}

func filterManagedSubscriptionLines(lines []string) []string {
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

func parseManagedSubscriptionCertificates(certPEM []byte) ([]*x509.Certificate, bool) {
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

func calculateManagedSubscriptionTLSPublicKeySHA256(certPEM []byte) (string, bool) {
	certs, ok := parseManagedSubscriptionCertificates(certPEM)
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

func calculateManagedSubscriptionTLSFingerprint(certPEM []byte) (string, bool) {
	certs, ok := parseManagedSubscriptionCertificates(certPEM)
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
