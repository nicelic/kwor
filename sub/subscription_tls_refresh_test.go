package sub

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/util"
	"gopkg.in/yaml.v3"
)

type testCertificateMaterial struct {
	pemText               string
	publicKeySHA256Base64 string
	fingerprintWithColons string
}

func buildLeafCertificateMaterial(t *testing.T, commonName string, serial int64) testCertificateMaterial {
	t.Helper()

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate ECDSA key: %v", err)
	}

	template := &x509.Certificate{
		SerialNumber: big.NewInt(serial),
		Subject:      pkix.Name{CommonName: commonName},
		NotBefore:    time.Now().Add(-1 * time.Hour),
		NotAfter:     time.Now().Add(24 * time.Hour),
		DNSNames:     []string{commonName},
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}

	certDER, err := x509.CreateCertificate(rand.Reader, template, template, &privateKey.PublicKey, privateKey)
	if err != nil {
		t.Fatalf("failed to create certificate: %v", err)
	}

	publicKeyDER, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		t.Fatalf("failed to marshal public key: %v", err)
	}

	pubKeySum := sha256.Sum256(publicKeyDER)
	fingerprintSum := sha256.Sum256(certDER)
	fingerprintHex := strings.ToUpper(hex.EncodeToString(fingerprintSum[:]))
	parts := make([]string, 0, len(fingerprintHex)/2)
	for i := 0; i < len(fingerprintHex); i += 2 {
		parts = append(parts, fingerprintHex[i:i+2])
	}

	return testCertificateMaterial{
		pemText:               strings.TrimSpace(string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER}))),
		publicKeySHA256Base64: base64.StdEncoding.EncodeToString(pubKeySum[:]),
		fingerprintWithColons: strings.Join(parts, ":"),
	}
}

func setupSubscriptionTestDB(t *testing.T, dbName string) {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), dbName)
	if err := database.InitDB(dbPath); err != nil {
		t.Fatalf("InitDB failed: %v", err)
	}

	db := database.GetDB()
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("db.DB failed: %v", err)
	}
	t.Cleanup(func() {
		_ = sqlDB.Close()
	})
}

func TestDefaultSubscriptionsRefreshCertificatePathMaterial(t *testing.T) {
	setupSubscriptionTestDB(t, "default-subscription-refresh.db")

	oldCert := buildLeafCertificateMaterial(t, "old.example.com", 1)
	newCert := buildLeafCertificateMaterial(t, "new.example.com", 2)
	certPath := filepath.Join(t.TempDir(), "server.pem")
	if err := os.WriteFile(certPath, []byte(oldCert.pemText+"\n"), 0o644); err != nil {
		t.Fatalf("failed to write old certificate: %v", err)
	}

	db := database.GetDB()
	tlsConfig := model.Tls{
		Name: "default-tls",
		Server: mustRawJSON(t, map[string]interface{}{
			"enabled":          true,
			"server_name":      "edge.example.com",
			"certificate_path": certPath,
		}),
		Client: mustRawJSON(t, map[string]interface{}{
			"include_server_certificate": true,
		}),
	}
	if err := db.Create(&tlsConfig).Error; err != nil {
		t.Fatalf("create tls failed: %v", err)
	}

	inbound := model.Inbound{
		Type:    "trojan",
		Tag:     "trojan-443",
		TlsId:   tlsConfig.Id,
		Tls:     &tlsConfig,
		Addrs:   mustRawJSON(t, []interface{}{}),
		OutJson: mustRawJSON(t, map[string]interface{}{}),
		Options: mustRawJSON(t, map[string]interface{}{
			"listen_port": 443,
		}),
	}
	if err := util.FillOutJson(&inbound, "panel.example.com"); err != nil {
		t.Fatalf("FillOutJson failed: %v", err)
	}
	if err := db.Create(&inbound).Error; err != nil {
		t.Fatalf("create inbound failed: %v", err)
	}

	if err := os.WriteFile(certPath, []byte(newCert.pemText+"\n"), 0o644); err != nil {
		t.Fatalf("failed to replace certificate: %v", err)
	}

	client := model.Client{
		Enable: true,
		Name:   "default-user",
		Config: mustRawJSON(t, map[string]interface{}{
			"trojan": map[string]interface{}{
				"password": "secret-pass",
			},
		}),
		Inbounds: mustRawJSON(t, []uint{inbound.Id}),
		Links:    mustRawJSON(t, []map[string]string{}),
	}
	if err := db.Create(&client).Error; err != nil {
		t.Fatalf("create client failed: %v", err)
	}

	jsonSub, _, err := (&JsonService{}).GetJson(client.Name, "json")
	if err != nil {
		t.Fatalf("GetJson failed: %v", err)
	}

	var jsonDoc map[string]interface{}
	if err := json.Unmarshal([]byte(*jsonSub), &jsonDoc); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	jsonOutbound := findTaggedOutbound(t, jsonDoc["outbounds"], inbound.Tag)
	jsonTLS := asMap(t, jsonOutbound["tls"])
	certificateLines := asStringSliceValue(t, jsonTLS["certificate"])
	if strings.Join(certificateLines, "\n") != newCert.pemText {
		t.Fatalf("expected refreshed certificate PEM from path, got %#v", jsonTLS["certificate"])
	}
	if _, hasSHA256 := jsonTLS["certificate_public_key_sha256"]; hasSHA256 {
		t.Fatalf("expected default JSON subscription to prefer PEM without certificate_public_key_sha256, got %#v", jsonTLS["certificate_public_key_sha256"])
	}

	clashSub, _, err := (&ClashService{}).GetClash(client.Name)
	if err != nil {
		t.Fatalf("GetClash failed: %v", err)
	}

	var clashDoc map[string]interface{}
	if err := yaml.Unmarshal([]byte(*clashSub), &clashDoc); err != nil {
		t.Fatalf("yaml.Unmarshal failed: %v", err)
	}
	clashProxy := findNamedProxy(t, clashDoc["proxies"], inbound.Tag)
	if got, _ := clashProxy["fingerprint"].(string); got != newCert.fingerprintWithColons {
		t.Fatalf("expected refreshed clash fingerprint %q, got %v", newCert.fingerprintWithColons, clashProxy["fingerprint"])
	}
}

func TestMihomoSubscriptionsRefreshCertificatePathMaterial(t *testing.T) {
	setupSubscriptionTestDB(t, "mihomo-subscription-refresh.db")

	oldCert := buildLeafCertificateMaterial(t, "mihomo-old.example.com", 11)
	newCert := buildLeafCertificateMaterial(t, "mihomo-new.example.com", 12)
	certPath := filepath.Join(t.TempDir(), "mihomo-server.pem")
	if err := os.WriteFile(certPath, []byte(oldCert.pemText+"\n"), 0o644); err != nil {
		t.Fatalf("failed to write old certificate: %v", err)
	}

	db := database.GetDB()
	tlsConfig := model.MihomoTls{
		Name: "mihomo-tls",
		Server: mustRawJSON(t, map[string]interface{}{
			"enabled":          true,
			"server_name":      "edge.example.com",
			"certificate_path": certPath,
		}),
		Client: mustRawJSON(t, map[string]interface{}{
			"include_server_certificate": true,
		}),
	}
	if err := db.Create(&tlsConfig).Error; err != nil {
		t.Fatalf("create mihomo tls failed: %v", err)
	}

	inbound := model.MihomoInbound{
		Type:    "trojan",
		Tag:     "mihomo-trojan-443",
		TlsId:   tlsConfig.Id,
		Tls:     &tlsConfig,
		Addrs:   mustRawJSON(t, []interface{}{}),
		OutJson: mustRawJSON(t, map[string]interface{}{}),
		Options: mustRawJSON(t, map[string]interface{}{
			"listen_port": 443,
		}),
	}

	baseInbound := inbound.ToBase()
	if err := util.FillOutJson(&baseInbound, "panel.example.com"); err != nil {
		t.Fatalf("FillOutJson failed: %v", err)
	}
	inbound.OutJson = baseInbound.OutJson
	if err := db.Create(&inbound).Error; err != nil {
		t.Fatalf("create mihomo inbound failed: %v", err)
	}

	if err := os.WriteFile(certPath, []byte(newCert.pemText+"\n"), 0o644); err != nil {
		t.Fatalf("failed to replace certificate: %v", err)
	}

	client := model.MihomoClient{
		Enable: true,
		Name:   "mihomo-refresh-user",
		Config: mustRawJSON(t, map[string]interface{}{
			"trojan": map[string]interface{}{
				"password": "secret-pass",
			},
		}),
		Inbounds: mustRawJSON(t, []uint{inbound.Id}),
		Links:    mustRawJSON(t, []map[string]string{}),
	}
	if err := db.Create(&client).Error; err != nil {
		t.Fatalf("create mihomo client failed: %v", err)
	}

	jsonSub, _, err := (&JsonService{}).GetMihomoJson(client.Name, "json")
	if err != nil {
		t.Fatalf("GetMihomoJson failed: %v", err)
	}

	var jsonDoc map[string]interface{}
	if err := json.Unmarshal([]byte(*jsonSub), &jsonDoc); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	jsonOutbound := findTaggedOutbound(t, jsonDoc["outbounds"], inbound.Tag)
	jsonTLS := asMap(t, jsonOutbound["tls"])
	certificateLines := asStringSliceValue(t, jsonTLS["certificate"])
	if strings.Join(certificateLines, "\n") != newCert.pemText {
		t.Fatalf("expected refreshed mihomo certificate PEM from path, got %#v", jsonTLS["certificate"])
	}
	if _, hasSHA256 := jsonTLS["certificate_public_key_sha256"]; hasSHA256 {
		t.Fatalf("expected mihomo JSON subscription to prefer PEM without certificate_public_key_sha256, got %#v", jsonTLS["certificate_public_key_sha256"])
	}

	clashSub, _, err := (&ClashService{}).GetMihomoClash(client.Name)
	if err != nil {
		t.Fatalf("GetMihomoClash failed: %v", err)
	}

	var clashDoc map[string]interface{}
	if err := yaml.Unmarshal([]byte(*clashSub), &clashDoc); err != nil {
		t.Fatalf("yaml.Unmarshal failed: %v", err)
	}
	clashProxy := findNamedProxy(t, clashDoc["proxies"], inbound.Tag)
	if got, _ := clashProxy["fingerprint"].(string); got != newCert.fingerprintWithColons {
		t.Fatalf("expected refreshed mihomo clash fingerprint %q, got %v", newCert.fingerprintWithColons, clashProxy["fingerprint"])
	}
}

func TestRefreshSubscriptionOutboundTLS_UsesSHA256ModeFromTLSSettings(t *testing.T) {
	cert := buildLeafCertificateMaterial(t, "sha-mode.example.com", 101)
	certPath := filepath.Join(t.TempDir(), "sha-mode-server.pem")
	if err := os.WriteFile(certPath, []byte(cert.pemText+"\n"), 0o644); err != nil {
		t.Fatalf("failed to write certificate: %v", err)
	}

	outbound := map[string]interface{}{
		"type": "trojan",
		"tag":  "sha-mode-node",
		"tls": map[string]interface{}{
			"enabled":                       true,
			"server_name":                   "edge.example.com",
			"certificate_public_key_sha256": []interface{}{"legacy-hash"},
			"certificate":                   []interface{}{"legacy-cert"},
		},
	}
	tlsConfig := &model.Tls{
		Server: mustRawJSON(t, map[string]interface{}{
			"enabled":          true,
			"server_name":      "edge.example.com",
			"certificate_path": certPath,
		}),
		Client: mustRawJSON(t, map[string]interface{}{
			"include_server_certificate":    true,
			"certificate_public_key_sha256": []string{"configured"},
		}),
	}

	refreshSubscriptionOutboundTLS(outbound, tlsConfig)

	outboundTLS := asMap(t, outbound["tls"])
	serverSHA256 := asStringSliceValue(t, outboundTLS["certificate_public_key_sha256"])
	if len(serverSHA256) != 1 || serverSHA256[0] != cert.publicKeySHA256Base64 {
		t.Fatalf("expected refreshed certificate_public_key_sha256 %q, got %#v", cert.publicKeySHA256Base64, outboundTLS["certificate_public_key_sha256"])
	}
	if _, hasCert := outboundTLS["certificate"]; hasCert {
		t.Fatalf("expected sha256 mode to omit certificate PEM, got %#v", outboundTLS["certificate"])
	}
}

func TestRefreshSubscriptionOutboundTLS_DisabledServerCertificate_RemovesCertificateAndSHA256(t *testing.T) {
	cert := buildLeafCertificateMaterial(t, "disabled-cert.example.com", 201)
	certPath := filepath.Join(t.TempDir(), "disabled-cert-server.pem")
	if err := os.WriteFile(certPath, []byte(cert.pemText+"\n"), 0o644); err != nil {
		t.Fatalf("failed to write certificate: %v", err)
	}

	outbound := map[string]interface{}{
		"type": "trojan",
		"tag":  "disabled-cert-node",
		"tls": map[string]interface{}{
			"enabled":                       true,
			"server_name":                   "edge.example.com",
			"certificate_public_key_sha256": []interface{}{"legacy-hash"},
			"certificate":                   []interface{}{"legacy-cert"},
			"fingerprint":                   "AA:BB:CC",
		},
	}
	tlsConfig := &model.Tls{
		Server: mustRawJSON(t, map[string]interface{}{
			"enabled":          true,
			"server_name":      "edge.example.com",
			"certificate_path": certPath,
		}),
		Client: mustRawJSON(t, map[string]interface{}{
			"include_server_certificate":    false,
			"certificate_public_key_sha256": []string{"configured"},
		}),
	}

	refreshSubscriptionOutboundTLS(outbound, tlsConfig)

	outboundTLS := asMap(t, outbound["tls"])
	if _, exists := outboundTLS["certificate"]; exists {
		t.Fatalf("expected certificate to be removed when include_server_certificate is false, got %#v", outboundTLS["certificate"])
	}
	if _, exists := outboundTLS["certificate_public_key_sha256"]; exists {
		t.Fatalf("expected certificate_public_key_sha256 to be removed when include_server_certificate is false, got %#v", outboundTLS["certificate_public_key_sha256"])
	}
	if got, _ := outboundTLS["fingerprint"].(string); got != cert.fingerprintWithColons {
		t.Fatalf("expected fingerprint to remain when include_server_certificate is false, got %#v", outboundTLS["fingerprint"])
	}
}

func TestMihomoSubscriptionsDisabledServerCertificate_RemovesCertificateAndSHA256(t *testing.T) {
	setupSubscriptionTestDB(t, "mihomo-subscription-disabled-server-cert.db")

	cert := buildLeafCertificateMaterial(t, "mihomo-disabled.example.com", 301)
	certPath := filepath.Join(t.TempDir(), "mihomo-disabled-server.pem")
	if err := os.WriteFile(certPath, []byte(cert.pemText+"\n"), 0o644); err != nil {
		t.Fatalf("failed to write certificate: %v", err)
	}

	db := database.GetDB()
	tlsConfig := model.MihomoTls{
		Name: "mihomo-disabled-tls",
		Server: mustRawJSON(t, map[string]interface{}{
			"enabled":          true,
			"server_name":      "edge.example.com",
			"certificate_path": certPath,
		}),
		Client: mustRawJSON(t, map[string]interface{}{
			"include_server_certificate":    false,
			"certificate_public_key_sha256": []string{"configured"},
		}),
	}
	if err := db.Create(&tlsConfig).Error; err != nil {
		t.Fatalf("create mihomo tls failed: %v", err)
	}

	inbound := model.MihomoInbound{
		Type:    "trojan",
		Tag:     "mihomo-disabled-trojan-443",
		TlsId:   tlsConfig.Id,
		Tls:     &tlsConfig,
		Addrs:   mustRawJSON(t, []interface{}{}),
		OutJson: mustRawJSON(t, map[string]interface{}{}),
		Options: mustRawJSON(t, map[string]interface{}{
			"listen_port": 443,
		}),
	}
	baseInbound := inbound.ToBase()
	if err := util.FillOutJson(&baseInbound, "panel.example.com"); err != nil {
		t.Fatalf("FillOutJson failed: %v", err)
	}
	inbound.OutJson = baseInbound.OutJson
	if err := db.Create(&inbound).Error; err != nil {
		t.Fatalf("create mihomo inbound failed: %v", err)
	}

	client := model.MihomoClient{
		Enable: true,
		Name:   "mihomo-disabled-user",
		Config: mustRawJSON(t, map[string]interface{}{
			"trojan": map[string]interface{}{
				"password": "secret-pass",
			},
		}),
		Inbounds: mustRawJSON(t, []uint{inbound.Id}),
		Links:    mustRawJSON(t, []map[string]string{}),
	}
	if err := db.Create(&client).Error; err != nil {
		t.Fatalf("create mihomo client failed: %v", err)
	}

	jsonSub, _, err := (&JsonService{}).GetMihomoJson(client.Name, "json")
	if err != nil {
		t.Fatalf("GetMihomoJson failed: %v", err)
	}

	var jsonDoc map[string]interface{}
	if err := json.Unmarshal([]byte(*jsonSub), &jsonDoc); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	jsonOutbound := findTaggedOutbound(t, jsonDoc["outbounds"], inbound.Tag)
	jsonTLS := asMap(t, jsonOutbound["tls"])
	if _, exists := jsonTLS["certificate"]; exists {
		t.Fatalf("expected certificate to be removed when include_server_certificate is false, got %#v", jsonTLS["certificate"])
	}
	if _, exists := jsonTLS["certificate_public_key_sha256"]; exists {
		t.Fatalf("expected certificate_public_key_sha256 to be removed when include_server_certificate is false, got %#v", jsonTLS["certificate_public_key_sha256"])
	}

	clashSub, _, err := (&ClashService{}).GetMihomoClash(client.Name)
	if err != nil {
		t.Fatalf("GetMihomoClash failed: %v", err)
	}
	var clashDoc map[string]interface{}
	if err := yaml.Unmarshal([]byte(*clashSub), &clashDoc); err != nil {
		t.Fatalf("yaml.Unmarshal failed: %v", err)
	}
	clashProxy := findNamedProxy(t, clashDoc["proxies"], inbound.Tag)
	if got, _ := clashProxy["fingerprint"].(string); got != cert.fingerprintWithColons {
		t.Fatalf("expected clash fingerprint to remain when include_server_certificate is false, got %#v", clashProxy["fingerprint"])
	}
}

func TestRefreshSubscriptionOutboundTLS_DisabledServerFingerprint_RemovesFingerprint(t *testing.T) {
	cert := buildLeafCertificateMaterial(t, "disabled-fp.example.com", 401)
	certPath := filepath.Join(t.TempDir(), "disabled-fp-server.pem")
	if err := os.WriteFile(certPath, []byte(cert.pemText+"\n"), 0o644); err != nil {
		t.Fatalf("failed to write certificate: %v", err)
	}

	outbound := map[string]interface{}{
		"type": "trojan",
		"tag":  "disabled-fp-node",
		"tls": map[string]interface{}{
			"enabled":     true,
			"server_name": "edge.example.com",
			"fingerprint": "AA:BB:CC",
		},
	}
	tlsConfig := &model.Tls{
		Server: mustRawJSON(t, map[string]interface{}{
			"enabled":          true,
			"server_name":      "edge.example.com",
			"certificate_path": certPath,
		}),
		Client: mustRawJSON(t, map[string]interface{}{
			"include_server_certificate": true,
			"include_server_fingerprint": false,
		}),
	}

	refreshSubscriptionOutboundTLS(outbound, tlsConfig)

	outboundTLS := asMap(t, outbound["tls"])
	if _, exists := outboundTLS["certificate"]; !exists {
		t.Fatalf("expected certificate to remain when include_server_certificate is true, got %#v", outboundTLS["certificate"])
	}
	if _, exists := outboundTLS["fingerprint"]; exists {
		t.Fatalf("expected fingerprint to be removed when include_server_fingerprint is false, got %#v", outboundTLS["fingerprint"])
	}
}

func TestMihomoSubscriptionsDisabledServerFingerprint_RemovesClashFingerprint(t *testing.T) {
	setupSubscriptionTestDB(t, "mihomo-subscription-disabled-server-fingerprint.db")

	cert := buildLeafCertificateMaterial(t, "mihomo-disabled-fp.example.com", 501)
	certPath := filepath.Join(t.TempDir(), "mihomo-disabled-fp-server.pem")
	if err := os.WriteFile(certPath, []byte(cert.pemText+"\n"), 0o644); err != nil {
		t.Fatalf("failed to write certificate: %v", err)
	}

	db := database.GetDB()
	tlsConfig := model.MihomoTls{
		Name: "mihomo-disabled-fp-tls",
		Server: mustRawJSON(t, map[string]interface{}{
			"enabled":          true,
			"server_name":      "edge.example.com",
			"certificate_path": certPath,
		}),
		Client: mustRawJSON(t, map[string]interface{}{
			"include_server_certificate": true,
			"include_server_fingerprint": false,
		}),
	}
	if err := db.Create(&tlsConfig).Error; err != nil {
		t.Fatalf("create mihomo tls failed: %v", err)
	}

	inbound := model.MihomoInbound{
		Type:    "trojan",
		Tag:     "mihomo-disabled-fp-trojan-443",
		TlsId:   tlsConfig.Id,
		Tls:     &tlsConfig,
		Addrs:   mustRawJSON(t, []interface{}{}),
		OutJson: mustRawJSON(t, map[string]interface{}{}),
		Options: mustRawJSON(t, map[string]interface{}{
			"listen_port": 443,
		}),
	}
	baseInbound := inbound.ToBase()
	if err := util.FillOutJson(&baseInbound, "panel.example.com"); err != nil {
		t.Fatalf("FillOutJson failed: %v", err)
	}
	inbound.OutJson = baseInbound.OutJson
	if err := db.Create(&inbound).Error; err != nil {
		t.Fatalf("create mihomo inbound failed: %v", err)
	}

	client := model.MihomoClient{
		Enable: true,
		Name:   "mihomo-disabled-fp-user",
		Config: mustRawJSON(t, map[string]interface{}{
			"trojan": map[string]interface{}{
				"password": "secret-pass",
			},
		}),
		Inbounds: mustRawJSON(t, []uint{inbound.Id}),
		Links:    mustRawJSON(t, []map[string]string{}),
	}
	if err := db.Create(&client).Error; err != nil {
		t.Fatalf("create mihomo client failed: %v", err)
	}

	clashSub, _, err := (&ClashService{}).GetMihomoClash(client.Name)
	if err != nil {
		t.Fatalf("GetMihomoClash failed: %v", err)
	}
	var clashDoc map[string]interface{}
	if err := yaml.Unmarshal([]byte(*clashSub), &clashDoc); err != nil {
		t.Fatalf("yaml.Unmarshal failed: %v", err)
	}
	clashProxy := findNamedProxy(t, clashDoc["proxies"], inbound.Tag)
	if _, exists := clashProxy["fingerprint"]; exists {
		t.Fatalf("expected clash fingerprint to be removed when include_server_fingerprint is false, got %#v", clashProxy["fingerprint"])
	}
}
