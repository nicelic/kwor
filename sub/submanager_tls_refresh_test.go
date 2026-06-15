package sub

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/logger"
	"github.com/alireza0/s-ui/util"
	logging "github.com/op/go-logging"
	"gopkg.in/yaml.v3"
)

func createSubOutboundFromMap(
	t *testing.T,
	outbound map[string]interface{},
	sourceType string,
	sourceClientID uint,
	sourceInboundID uint,
	clashOptions map[string]interface{},
) *model.SubOutbound {
	t.Helper()

	raw := mustRawJSON(t, outbound)
	record := &model.SubOutbound{}
	if err := record.UnmarshalJSON(raw); err != nil {
		t.Fatalf("SubOutbound.UnmarshalJSON failed: %v", err)
	}
	record.SourceType = sourceType
	record.SourceClientId = sourceClientID
	record.SourceInboundId = sourceInboundID
	if clashOptions != nil {
		record.ClashOptions = mustRawJSON(t, clashOptions)
	}

	if err := database.GetDB().Create(record).Error; err != nil {
		t.Fatalf("create suboutbound failed: %v", err)
	}
	return record
}

func initSubManagerTLSRefreshTestLogger() {
	if logger.GetLogger() == nil {
		logger.InitLogger(logging.INFO)
	}
}

func TestSubManagerSubscriptionsRefreshManagedCertificatePathMaterial_Default(t *testing.T) {
	initSubManagerTLSRefreshTestLogger()
	setupSubscriptionTestDB(t, "submanager-refresh-default.db")

	oldCert := buildLeafCertificateMaterial(t, "sm-old.example.com", 21)
	newCert := buildLeafCertificateMaterial(t, "sm-new.example.com", 22)
	certPath := filepath.Join(t.TempDir(), "sm-default-server.pem")
	if err := os.WriteFile(certPath, []byte(oldCert.pemText+"\n"), 0o644); err != nil {
		t.Fatalf("failed to write old certificate: %v", err)
	}

	db := database.GetDB()
	tlsConfig := model.Tls{
		Name: "sm-default-tls",
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

	subTag := "trojan-443_sync_default"
	var outboundMap map[string]interface{}
	if err := json.Unmarshal(inbound.OutJson, &outboundMap); err != nil {
		t.Fatalf("json.Unmarshal outbound failed: %v", err)
	}
	outboundMap["tag"] = subTag
	if tlsMap, ok := outboundMap["tls"].(map[string]interface{}); ok {
		delete(tlsMap, "certificate_public_key_sha256")
	}
	createSubOutboundFromMap(t, outboundMap, subManagerSourceClient, 1001, inbound.Id, nil)

	if err := os.WriteFile(certPath, []byte(newCert.pemText+"\n"), 0o644); err != nil {
		t.Fatalf("failed to replace certificate: %v", err)
	}

	jsonSub, err := (&SubManagerSubService{}).GetSubManagerJson(subTag)
	if err != nil {
		t.Fatalf("GetSubManagerJson failed: %v", err)
	}

	var jsonDoc map[string]interface{}
	if err := json.Unmarshal([]byte(*jsonSub), &jsonDoc); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	jsonOutbound := findTaggedOutbound(t, jsonDoc["outbounds"], subTag)
	jsonTLS := asMap(t, jsonOutbound["tls"])
	certificateLines := asStringSliceValue(t, jsonTLS["certificate"])
	if strings.Join(certificateLines, "\n") != newCert.pemText {
		t.Fatalf("expected refreshed certificate PEM from path, got %#v", jsonTLS["certificate"])
	}
	if _, hasSHA256 := jsonTLS["certificate_public_key_sha256"]; hasSHA256 {
		t.Fatalf("expected default JSON subscription to prefer PEM without certificate_public_key_sha256, got %#v", jsonTLS["certificate_public_key_sha256"])
	}

	clashSub, err := (&SubManagerSubService{}).GetSubManagerClash(subTag)
	if err != nil {
		t.Fatalf("GetSubManagerClash failed: %v", err)
	}

	var clashDoc map[string]interface{}
	if err := yaml.Unmarshal([]byte(*clashSub), &clashDoc); err != nil {
		t.Fatalf("yaml.Unmarshal failed: %v", err)
	}
	clashProxy := findNamedProxy(t, clashDoc["proxies"], subTag)
	if got, _ := clashProxy["fingerprint"].(string); got != newCert.fingerprintWithColons {
		t.Fatalf("expected refreshed clash fingerprint %q, got %v", newCert.fingerprintWithColons, clashProxy["fingerprint"])
	}
}

func TestSubManagerSubscriptionsRefreshManagedCertificatePathMaterial_Mihomo(t *testing.T) {
	initSubManagerTLSRefreshTestLogger()
	setupSubscriptionTestDB(t, "submanager-refresh-mihomo.db")

	oldCert := buildLeafCertificateMaterial(t, "sm-mihomo-old.example.com", 31)
	newCert := buildLeafCertificateMaterial(t, "sm-mihomo-new.example.com", 32)
	certPath := filepath.Join(t.TempDir(), "sm-mihomo-server.pem")
	if err := os.WriteFile(certPath, []byte(oldCert.pemText+"\n"), 0o644); err != nil {
		t.Fatalf("failed to write old certificate: %v", err)
	}

	db := database.GetDB()
	tlsConfig := model.MihomoTls{
		Name: "sm-mihomo-tls",
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

	subTag := "mihomo-trojan-443_sync_default"
	var outboundMap map[string]interface{}
	if err := json.Unmarshal(inbound.OutJson, &outboundMap); err != nil {
		t.Fatalf("json.Unmarshal outbound failed: %v", err)
	}
	outboundMap["tag"] = subTag
	if tlsMap, ok := outboundMap["tls"].(map[string]interface{}); ok {
		delete(tlsMap, "certificate_public_key_sha256")
	}
	createSubOutboundFromMap(
		t,
		outboundMap,
		subManagerSourceMihomoClient,
		2001,
		inbound.Id,
		map[string]interface{}{
			"name":        subTag,
			"type":        "trojan",
			"server":      "legacy.example.com",
			"port":        443,
			"password":    "legacy",
			"fingerprint": oldCert.fingerprintWithColons,
		},
	)

	if err := os.WriteFile(certPath, []byte(newCert.pemText+"\n"), 0o644); err != nil {
		t.Fatalf("failed to replace certificate: %v", err)
	}

	jsonSub, err := (&SubManagerSubService{}).GetSubManagerJson(subTag)
	if err != nil {
		t.Fatalf("GetSubManagerJson failed: %v", err)
	}

	var jsonDoc map[string]interface{}
	if err := json.Unmarshal([]byte(*jsonSub), &jsonDoc); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	jsonOutbound := findTaggedOutbound(t, jsonDoc["outbounds"], subTag)
	jsonTLS := asMap(t, jsonOutbound["tls"])
	certificateLines := asStringSliceValue(t, jsonTLS["certificate"])
	if strings.Join(certificateLines, "\n") != newCert.pemText {
		t.Fatalf("expected refreshed mihomo certificate PEM from path, got %#v", jsonTLS["certificate"])
	}
	if _, hasSHA256 := jsonTLS["certificate_public_key_sha256"]; hasSHA256 {
		t.Fatalf("expected mihomo JSON subscription to prefer PEM without certificate_public_key_sha256, got %#v", jsonTLS["certificate_public_key_sha256"])
	}

	clashSub, err := (&SubManagerSubService{}).GetSubManagerClash(subTag)
	if err != nil {
		t.Fatalf("GetSubManagerClash failed: %v", err)
	}

	var clashDoc map[string]interface{}
	if err := yaml.Unmarshal([]byte(*clashSub), &clashDoc); err != nil {
		t.Fatalf("yaml.Unmarshal failed: %v", err)
	}
	clashProxy := findNamedProxy(t, clashDoc["proxies"], subTag)
	if got, _ := clashProxy["fingerprint"].(string); got != newCert.fingerprintWithColons {
		t.Fatalf("expected refreshed clash fingerprint %q, got %v", newCert.fingerprintWithColons, clashProxy["fingerprint"])
	}
}

func TestSubManagerClash_DisabledServerFingerprint_RemovesFingerprint_Default(t *testing.T) {
	initSubManagerTLSRefreshTestLogger()
	setupSubscriptionTestDB(t, "submanager-disabled-server-fingerprint-default.db")

	cert := buildLeafCertificateMaterial(t, "sm-disabled-fp.example.com", 61)
	certPath := filepath.Join(t.TempDir(), "sm-disabled-fp-server.pem")
	if err := os.WriteFile(certPath, []byte(cert.pemText+"\n"), 0o644); err != nil {
		t.Fatalf("failed to write certificate: %v", err)
	}

	db := database.GetDB()
	tlsConfig := model.Tls{
		Name: "sm-disabled-fp-tls",
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
		t.Fatalf("create tls failed: %v", err)
	}

	inbound := model.Inbound{
		Type:    "trojan",
		Tag:     "trojan-disabled-fp-443",
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

	subTag := "trojan-disabled-fp-443_sync_default"
	var outboundMap map[string]interface{}
	if err := json.Unmarshal(inbound.OutJson, &outboundMap); err != nil {
		t.Fatalf("json.Unmarshal outbound failed: %v", err)
	}
	outboundMap["tag"] = subTag
	createSubOutboundFromMap(t, outboundMap, subManagerSourceClient, 5001, inbound.Id, nil)

	clashSub, err := (&SubManagerSubService{}).GetSubManagerClash(subTag)
	if err != nil {
		t.Fatalf("GetSubManagerClash failed: %v", err)
	}

	var clashDoc map[string]interface{}
	if err := yaml.Unmarshal([]byte(*clashSub), &clashDoc); err != nil {
		t.Fatalf("yaml.Unmarshal failed: %v", err)
	}
	clashProxy := findNamedProxy(t, clashDoc["proxies"], subTag)
	if _, exists := clashProxy["fingerprint"]; exists {
		t.Fatalf("expected clash fingerprint to be removed when include_server_fingerprint is false, got %#v", clashProxy["fingerprint"])
	}
}

func TestSubManagerClash_DisabledServerFingerprint_RemovesFingerprint_Mihomo(t *testing.T) {
	initSubManagerTLSRefreshTestLogger()
	setupSubscriptionTestDB(t, "submanager-disabled-server-fingerprint-mihomo.db")

	cert := buildLeafCertificateMaterial(t, "sm-mihomo-disabled-fp.example.com", 71)
	certPath := filepath.Join(t.TempDir(), "sm-mihomo-disabled-fp-server.pem")
	if err := os.WriteFile(certPath, []byte(cert.pemText+"\n"), 0o644); err != nil {
		t.Fatalf("failed to write certificate: %v", err)
	}

	db := database.GetDB()
	tlsConfig := model.MihomoTls{
		Name: "sm-mihomo-disabled-fp-tls",
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

	subTag := "mihomo-disabled-fp-trojan-443_sync_default"
	var outboundMap map[string]interface{}
	if err := json.Unmarshal(inbound.OutJson, &outboundMap); err != nil {
		t.Fatalf("json.Unmarshal outbound failed: %v", err)
	}
	outboundMap["tag"] = subTag
	createSubOutboundFromMap(t, outboundMap, subManagerSourceMihomoClient, 6001, inbound.Id, nil)

	clashSub, err := (&SubManagerSubService{}).GetSubManagerClash(subTag)
	if err != nil {
		t.Fatalf("GetSubManagerClash failed: %v", err)
	}

	var clashDoc map[string]interface{}
	if err := yaml.Unmarshal([]byte(*clashSub), &clashDoc); err != nil {
		t.Fatalf("yaml.Unmarshal failed: %v", err)
	}
	clashProxy := findNamedProxy(t, clashDoc["proxies"], subTag)
	if _, exists := clashProxy["fingerprint"]; exists {
		t.Fatalf("expected clash fingerprint to be removed when include_server_fingerprint is false, got %#v", clashProxy["fingerprint"])
	}
}

func TestSubManagerJson_DisabledServerCertificate_RemovesCertificateAndSHA256(t *testing.T) {
	initSubManagerTLSRefreshTestLogger()
	setupSubscriptionTestDB(t, "submanager-disabled-server-cert.db")

	cert := buildLeafCertificateMaterial(t, "sm-disabled.example.com", 41)
	certPath := filepath.Join(t.TempDir(), "sm-disabled-server.pem")
	if err := os.WriteFile(certPath, []byte(cert.pemText+"\n"), 0o644); err != nil {
		t.Fatalf("failed to write certificate: %v", err)
	}

	db := database.GetDB()
	tlsConfig := model.Tls{
		Name: "sm-disabled-tls",
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
		t.Fatalf("create tls failed: %v", err)
	}

	inbound := model.Inbound{
		Type:    "trojan",
		Tag:     "trojan-disabled-443",
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

	subTag := "trojan-disabled-443_sync_default"
	var outboundMap map[string]interface{}
	if err := json.Unmarshal(inbound.OutJson, &outboundMap); err != nil {
		t.Fatalf("json.Unmarshal outbound failed: %v", err)
	}
	outboundMap["tag"] = subTag
	createSubOutboundFromMap(t, outboundMap, subManagerSourceClient, 3001, inbound.Id, nil)

	jsonSub, err := (&SubManagerSubService{}).GetSubManagerJson(subTag)
	if err != nil {
		t.Fatalf("GetSubManagerJson failed: %v", err)
	}

	var jsonDoc map[string]interface{}
	if err := json.Unmarshal([]byte(*jsonSub), &jsonDoc); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	jsonOutbound := findTaggedOutbound(t, jsonDoc["outbounds"], subTag)
	jsonTLS := asMap(t, jsonOutbound["tls"])
	if _, exists := jsonTLS["certificate"]; exists {
		t.Fatalf("expected certificate to be removed when include_server_certificate is false, got %#v", jsonTLS["certificate"])
	}
	if _, exists := jsonTLS["certificate_public_key_sha256"]; exists {
		t.Fatalf("expected certificate_public_key_sha256 to be removed when include_server_certificate is false, got %#v", jsonTLS["certificate_public_key_sha256"])
	}
}

func TestSubManagerJson_PreservesExistingSHA256ModeFromStoredOutbound(t *testing.T) {
	initSubManagerTLSRefreshTestLogger()
	setupSubscriptionTestDB(t, "submanager-preserve-stored-sha256.db")

	cert := buildLeafCertificateMaterial(t, "sm-sha.example.com", 71)
	certPath := filepath.Join(t.TempDir(), "sm-sha-server.pem")
	if err := os.WriteFile(certPath, []byte(cert.pemText+"\n"), 0o644); err != nil {
		t.Fatalf("failed to write certificate: %v", err)
	}

	db := database.GetDB()
	tlsConfig := model.Tls{
		Name: "sm-sha-tls",
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
		Tag:     "trojan-sha-443",
		TlsId:   tlsConfig.Id,
		Tls:     &tlsConfig,
		Addrs:   mustRawJSON(t, []interface{}{}),
		OutJson: mustRawJSON(t, map[string]interface{}{}),
		Options: mustRawJSON(t, map[string]interface{}{"listen_port": 443}),
	}
	if err := util.FillOutJson(&inbound, "panel.example.com"); err != nil {
		t.Fatalf("FillOutJson failed: %v", err)
	}
	if err := db.Create(&inbound).Error; err != nil {
		t.Fatalf("create inbound failed: %v", err)
	}

	subTag := "trojan-sha-443_sync_default"
	var outboundMap map[string]interface{}
	if err := json.Unmarshal(inbound.OutJson, &outboundMap); err != nil {
		t.Fatalf("json.Unmarshal outbound failed: %v", err)
	}
	outboundMap["tag"] = subTag
	if tlsMap, ok := outboundMap["tls"].(map[string]interface{}); ok {
		tlsMap["certificate_public_key_sha256"] = []string{"legacy-hash"}
		delete(tlsMap, "certificate")
	}
	createSubOutboundFromMap(t, outboundMap, subManagerSourceClient, 7001, inbound.Id, nil)

	jsonSub, err := (&SubManagerSubService{}).GetSubManagerJson(subTag)
	if err != nil {
		t.Fatalf("GetSubManagerJson failed: %v", err)
	}

	var jsonDoc map[string]interface{}
	if err := json.Unmarshal([]byte(*jsonSub), &jsonDoc); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	jsonOutbound := findTaggedOutbound(t, jsonDoc["outbounds"], subTag)
	jsonTLS := asMap(t, jsonOutbound["tls"])
	hashes := asStringSliceValue(t, jsonTLS["certificate_public_key_sha256"])
	if len(hashes) != 1 || hashes[0] != cert.publicKeySHA256Base64 {
		t.Fatalf("expected stored sha256 mode to be preserved and refreshed, got %#v", jsonTLS["certificate_public_key_sha256"])
	}
	if _, exists := jsonTLS["certificate"]; exists {
		t.Fatalf("expected certificate PEM to stay omitted in sha256 mode, got %#v", jsonTLS["certificate"])
	}
}

func TestSubManagerJson_SubgroupImportedNodePreservesStoredSHA256(t *testing.T) {
	initSubManagerTLSRefreshTestLogger()
	setupSubscriptionTestDB(t, "submanager-subgroup-preserve-stored-sha256.db")

	subTag := "imported-hy1-node"
	createSubOutboundFromMap(
		t,
		map[string]interface{}{
			"type":        "hysteria",
			"tag":         subTag,
			"server":      "1.2.3.4",
			"server_port": 443,
			"auth_str":    "secret",
			"tls": map[string]interface{}{
				"enabled": true,
				"certificate_public_key_sha256": []string{
					"stored-sha256-value",
				},
			},
		},
		subManagerSourceSubGroup,
		8001,
		0,
		nil,
	)

	jsonSub, err := (&SubManagerSubService{}).GetSubManagerJson(subTag)
	if err != nil {
		t.Fatalf("GetSubManagerJson failed: %v", err)
	}

	var jsonDoc map[string]interface{}
	if err := json.Unmarshal([]byte(*jsonSub), &jsonDoc); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	jsonOutbound := findTaggedOutbound(t, jsonDoc["outbounds"], subTag)
	jsonTLS := asMap(t, jsonOutbound["tls"])
	hashes := asStringSliceValue(t, jsonTLS["certificate_public_key_sha256"])
	if len(hashes) != 1 || hashes[0] != "stored-sha256-value" {
		t.Fatalf("expected imported subgroup node to preserve stored sha256, got %#v", jsonTLS["certificate_public_key_sha256"])
	}
	if _, exists := jsonTLS["certificate"]; exists {
		t.Fatalf("expected imported subgroup node to keep original sha256 mode without PEM injection, got %#v", jsonTLS["certificate"])
	}
}

func TestSubManagerJsonMihomo_DisabledServerCertificate_RemovesCertificateAndSHA256(t *testing.T) {
	initSubManagerTLSRefreshTestLogger()
	setupSubscriptionTestDB(t, "submanager-mihomo-disabled-server-cert.db")

	cert := buildLeafCertificateMaterial(t, "sm-mihomo-disabled.example.com", 51)
	certPath := filepath.Join(t.TempDir(), "sm-mihomo-disabled-server.pem")
	if err := os.WriteFile(certPath, []byte(cert.pemText+"\n"), 0o644); err != nil {
		t.Fatalf("failed to write certificate: %v", err)
	}

	db := database.GetDB()
	tlsConfig := model.MihomoTls{
		Name: "sm-mihomo-disabled-tls",
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

	subTag := "mihomo-disabled-trojan-443_sync_default"
	var outboundMap map[string]interface{}
	if err := json.Unmarshal(inbound.OutJson, &outboundMap); err != nil {
		t.Fatalf("json.Unmarshal outbound failed: %v", err)
	}
	outboundMap["tag"] = subTag
	createSubOutboundFromMap(
		t,
		outboundMap,
		subManagerSourceMihomoClient,
		4001,
		inbound.Id,
		nil,
	)

	jsonSub, err := (&SubManagerSubService{}).GetSubManagerJson(subTag)
	if err != nil {
		t.Fatalf("GetSubManagerJson failed: %v", err)
	}

	var jsonDoc map[string]interface{}
	if err := json.Unmarshal([]byte(*jsonSub), &jsonDoc); err != nil {
		t.Fatalf("json.Unmarshal failed: %v", err)
	}
	jsonOutbound := findTaggedOutbound(t, jsonDoc["outbounds"], subTag)
	jsonTLS := asMap(t, jsonOutbound["tls"])
	if _, exists := jsonTLS["certificate"]; exists {
		t.Fatalf("expected certificate to be removed when include_server_certificate is false, got %#v", jsonTLS["certificate"])
	}
	if _, exists := jsonTLS["certificate_public_key_sha256"]; exists {
		t.Fatalf("expected certificate_public_key_sha256 to be removed when include_server_certificate is false, got %#v", jsonTLS["certificate_public_key_sha256"])
	}
}
