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

func TestRefreshMihomoClashProxyTLSUpdatesWrapperProjection(t *testing.T) {
	tests := []struct {
		name   string
		proxy  map[string]interface{}
		tls    *model.Tls
		assert func(*testing.T, map[string]interface{})
	}{
		{
			name: "vless restls",
			proxy: map[string]interface{}{
				"name":         "node",
				"type":         "vless",
				"tls":          true,
				"servername":   "old.example.com",
				"reality-opts": map[string]interface{}{"short-id": "old"},
			},
			tls: &model.Tls{
				Mode:   model.MihomoTlsModeRestls,
				Server: mustRawJSON(t, map[string]interface{}{"res_tls": map[string]interface{}{"dest": "edge.example.com:443", "password": "restls-pass", "restls_script": "300?100"}}),
				Client: mustRawJSON(t, map[string]interface{}{"server_name": "cdn.example.com", "restls_opts": map[string]interface{}{"password": "restls-pass", "version_hint": "tls13", "restls_script": "300?100"}}),
			},
			assert: func(t *testing.T, proxy map[string]interface{}) {
				if proxy["tls"] != true || proxy["servername"] != "cdn.example.com" {
					t.Fatalf("unexpected TLS/SNI projection: %#v", proxy)
				}
				if _, exists := proxy["reality-opts"]; exists {
					t.Fatalf("stale Reality options retained: %#v", proxy)
				}
				opts, ok := proxy["restls-opts"].(map[string]interface{})
				if !ok || opts["version-hint"] != "tls13" || opts["restls-script"] != "300?100" {
					t.Fatalf("unexpected Restls options: %#v", proxy["restls-opts"])
				}
			},
		},
		{
			name: "shadowsocks jls",
			proxy: map[string]interface{}{
				"name":               "ss-node",
				"type":               "shadowsocks",
				"tls":                true,
				"plugin":             "restls",
				"plugin-opts":        map[string]interface{}{"password": "old"},
				"restls-opts":        map[string]interface{}{"password": "old"},
				"client-fingerprint": "old-fingerprint",
			},
			tls: &model.Tls{
				Mode:   model.MihomoTlsModeJLS,
				Server: mustRawJSON(t, map[string]interface{}{"jls_config": map[string]interface{}{"dest": "edge.example.com:443", "sni": "jls.example.com", "alpn": []interface{}{"h2"}}}),
				Client: mustRawJSON(t, map[string]interface{}{"jls_opts": map[string]interface{}{"username": "alice", "password": "jls-pass"}}),
			},
			assert: func(t *testing.T, proxy map[string]interface{}) {
				if proxy["plugin"] != "jls" {
					t.Fatalf("plugin = %#v", proxy["plugin"])
				}
				if _, exists := proxy["tls"]; exists {
					t.Fatalf("Shadowsocks plugin proxy must not keep tls: %#v", proxy)
				}
				opts, ok := proxy["plugin-opts"].(map[string]interface{})
				if !ok || opts["host"] != "jls.example.com" || opts["username"] != "alice" || opts["password"] != "jls-pass" {
					t.Fatalf("unexpected JLS plugin options: %#v", proxy["plugin-opts"])
				}
				if _, exists := proxy["restls-opts"]; exists {
					t.Fatalf("stale Restls options retained: %#v", proxy)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			refreshMihomoClashProxyTLS(tt.proxy, tt.tls)
			tt.assert(t, tt.proxy)
		})
	}
}

func TestSubManagerRuntimeRefreshesMihomoAnyTLSReality(t *testing.T) {
	initSubManagerTLSRefreshTestLogger()
	setupSubscriptionTestDB(t, "submanager-mihomo-anytls-reality-refresh.db")

	db := database.GetDB()
	tlsConfig := model.MihomoTls{
		Name: "mihomo-anytls-reality",
		Mode: model.MihomoTlsModeReality,
		Server: mustRawJSON(t, map[string]interface{}{
			"server_name": "anytls.example.com",
			"reality":     map[string]interface{}{"enabled": true},
		}),
		Client: mustRawJSON(t, map[string]interface{}{
			"server_name": "anytls.example.com",
			"reality": map[string]interface{}{
				"enabled":    true,
				"public_key": "public-key",
				"short_id":   "abcd",
			},
		}),
	}
	if err := db.Create(&tlsConfig).Error; err != nil {
		t.Fatalf("create Mihomo TLS failed: %v", err)
	}
	inbound := &model.MihomoInbound{
		Type:  "anytls",
		Tag:   "anytls-reality-source",
		TlsId: tlsConfig.Id,
		Options: mustRawJSON(t, map[string]interface{}{
			"listen_port": 443,
			"password":    "anytls-password",
		}),
	}
	if err := db.Create(inbound).Error; err != nil {
		t.Fatalf("create Mihomo inbound failed: %v", err)
	}

	subOutbound := &model.SubOutbound{
		Type:            "anytls",
		SourceType:      subManagerSourceMihomoClient,
		SourceInboundId: inbound.Id,
	}
	proxy := map[string]interface{}{
		"type":         "anytls",
		"tls":          true,
		"sni":          "old.example.com",
		"reality-opts": map[string]interface{}{"short-id": "old"},
	}
	(&SubManagerSubService{}).refreshSubOutboundClashProxyTLS(proxy, subOutbound)

	if proxy["tls"] != true || proxy["sni"] != "anytls.example.com" {
		t.Fatalf("unexpected AnyTLS TLS/SNI projection: %#v", proxy)
	}
	realityOpts, ok := proxy["reality-opts"].(map[string]interface{})
	if !ok || realityOpts["public-key"] != "public-key" || realityOpts["short-id"] != "abcd" {
		t.Fatalf("unexpected AnyTLS Reality projection: %#v", proxy["reality-opts"])
	}
}

func TestSubManagerRuntimeRefreshesMihomoShadowsocksPluginWithoutTLSMap(t *testing.T) {
	initSubManagerTLSRefreshTestLogger()
	setupSubscriptionTestDB(t, "submanager-mihomo-ss-wrapper-refresh.db")

	db := database.GetDB()
	tlsConfig := model.MihomoTls{
		Name: "mihomo-ss-jls",
		Mode: model.MihomoTlsModeJLS,
		Server: mustRawJSON(t, map[string]interface{}{
			"jls_config": map[string]interface{}{
				"enable": true,
				"users":  []interface{}{map[string]interface{}{"username": "alice", "password": "jls-pass"}},
				"dest":   "edge.example.com:443",
				"sni":    "jls.example.com",
			},
		}),
		Client: mustRawJSON(t, map[string]interface{}{
			"jls_opts": map[string]interface{}{"username": "alice", "password": "jls-pass"},
		}),
	}
	if err := db.Create(&tlsConfig).Error; err != nil {
		t.Fatalf("create Mihomo TLS failed: %v", err)
	}
	inbound := &model.MihomoInbound{
		Type:  "shadowsocks",
		Tag:   "ss-jls-source",
		TlsId: tlsConfig.Id,
		Options: mustRawJSON(t, map[string]interface{}{
			"listen_port": 443,
			"method":      "2022-blake3-aes-128-gcm",
			"password":    "ss-pass",
		}),
		Tls: &tlsConfig,
	}
	if err := db.Create(inbound).Error; err != nil {
		t.Fatalf("create Mihomo inbound failed: %v", err)
	}

	subOutbound := &model.SubOutbound{
		Type:            "shadowsocks",
		SourceType:      subManagerSourceMihomoClient,
		SourceInboundId: inbound.Id,
	}
	outbound := map[string]interface{}{
		"type":        "shadowsocks",
		"plugin":      "restls",
		"plugin_opts": map[string]interface{}{"password": "old"},
	}
	(&SubManagerSubService{}).refreshSubOutboundTLS(outbound, subOutbound)

	if outbound["plugin"] != "jls" {
		t.Fatalf("plugin = %#v", outbound["plugin"])
	}
	if _, exists := outbound["tls"]; exists {
		t.Fatalf("Shadowsocks plugin refresh must not add tls: %#v", outbound)
	}
	pluginOpts, ok := outbound["plugin_opts"].(map[string]interface{})
	if !ok || pluginOpts["host"] != "jls.example.com" || pluginOpts["username"] != "alice" || pluginOpts["password"] != "jls-pass" {
		t.Fatalf("unexpected plugin options: %#v", outbound["plugin_opts"])
	}
}

func TestSubManagerSubscriptionsRenderMihomoShadowsocksShadowTLS(t *testing.T) {
	initSubManagerTLSRefreshTestLogger()
	setupSubscriptionTestDB(t, "submanager-mihomo-ss-shadowtls-subscriptions.db")

	db := database.GetDB()
	tlsConfig := model.MihomoTls{
		Name: "mihomo-ss-shadowtls",
		Mode: model.MihomoTlsModeShadowTLS,
		Server: mustRawJSON(t, map[string]interface{}{
			"shadow_tls": map[string]interface{}{
				"enable":    true,
				"version":   3,
				"users":     []interface{}{map[string]interface{}{"name": "alice", "password": "shadow-pass"}},
				"handshake": map[string]interface{}{"dest": "addons.mozilla.org:443"},
			},
		}),
		Client: mustRawJSON(t, map[string]interface{}{
			"shadow_tls_opts": map[string]interface{}{"version": 3, "password": "shadow-pass"},
			"utls":            map[string]interface{}{"enabled": true, "fingerprint": "chrome"},
		}),
	}
	if err := db.Create(&tlsConfig).Error; err != nil {
		t.Fatalf("create Mihomo ShadowTLS config failed: %v", err)
	}

	inbound := model.MihomoInbound{
		Type:  "shadowsocks",
		Tag:   "ss-shadowtls-source",
		TlsId: tlsConfig.Id,
		Options: mustRawJSON(t, map[string]interface{}{
			"listen_port": 443,
			"method":      "2022-blake3-aes-128-gcm",
			"password":    "ss-pass",
		}),
	}
	if err := db.Create(&inbound).Error; err != nil {
		t.Fatalf("create Mihomo Shadowsocks inbound failed: %v", err)
	}

	const subTag = "ss-shadowtls-source_sync_default"
	createSubOutboundFromMap(
		t,
		map[string]interface{}{
			"type":        "shadowsocks",
			"tag":         subTag,
			"server":      "203.0.113.10",
			"server_port": 443,
			"method":      "2022-blake3-aes-128-gcm",
			"password":    "ss-pass",
			"plugin":      "restls",
			"plugin_opts": map[string]interface{}{"password": "stale"},
		},
		subManagerSourceMihomoClient,
		9001,
		inbound.Id,
		nil,
	)

	jsonSub, err := (&SubManagerSubService{}).GetSubManagerJson(subTag)
	if err != nil {
		t.Fatalf("GetSubManagerJson failed: %v", err)
	}
	jsonDoc := map[string]interface{}{}
	if err := json.Unmarshal([]byte(*jsonSub), &jsonDoc); err != nil {
		t.Fatalf("decode SubManager JSON subscription failed: %v", err)
	}
	ssOutbound := findTaggedOutbound(t, jsonDoc["outbounds"], subTag)
	if ssOutbound["type"] != "shadowsocks" || ssOutbound["detour"] != subTag+"-out" {
		t.Fatalf("unexpected SubManager Shadowsocks outbound: %#v", ssOutbound)
	}
	if _, exists := ssOutbound["plugin"]; exists {
		t.Fatalf("SubManager sing-box JSON must not retain the Mihomo plugin: %#v", ssOutbound)
	}
	shadowTLSOutbound := findTaggedOutbound(t, jsonDoc["outbounds"], subTag+"-out")
	if shadowTLSOutbound["type"] != "shadowtls" || shadowTLSOutbound["password"] != "shadow-pass" {
		t.Fatalf("unexpected SubManager ShadowTLS outbound: %#v", shadowTLSOutbound)
	}
	shadowTLSTLS := asMap(t, shadowTLSOutbound["tls"])
	if shadowTLSTLS["server_name"] != "addons.mozilla.org" {
		t.Fatalf("unexpected SubManager ShadowTLS SNI: %#v", shadowTLSTLS)
	}

	clashSub, err := (&SubManagerSubService{}).GetSubManagerClash(subTag)
	if err != nil {
		t.Fatalf("GetSubManagerClash failed: %v", err)
	}
	clashDoc := map[string]interface{}{}
	if err := yaml.Unmarshal([]byte(*clashSub), &clashDoc); err != nil {
		t.Fatalf("decode SubManager Clash subscription failed: %v", err)
	}
	clashProxy := findNamedProxy(t, clashDoc["proxies"], subTag)
	if clashProxy["type"] != "ss" || clashProxy["plugin"] != "shadow-tls" {
		t.Fatalf("unexpected SubManager Clash proxy: %#v", clashProxy)
	}
	pluginOpts := asMap(t, clashProxy["plugin-opts"])
	if pluginOpts["host"] != "addons.mozilla.org" || pluginOpts["password"] != "shadow-pass" {
		t.Fatalf("unexpected SubManager Clash plugin options: %#v", pluginOpts)
	}
}

func TestSubManagerRuntimeRefreshesMihomoWrapperWithoutTLSMap(t *testing.T) {
	initSubManagerTLSRefreshTestLogger()
	setupSubscriptionTestDB(t, "submanager-mihomo-wrapper-refresh.db")

	db := database.GetDB()
	tlsConfig := model.MihomoTls{
		Name: "mihomo-trojan-restls",
		Mode: model.MihomoTlsModeRestls,
		Server: mustRawJSON(t, map[string]interface{}{
			"enabled": true,
			"res_tls": map[string]interface{}{
				"enable":   true,
				"dest":     "edge.example.com:443",
				"password": "restls-pass",
			},
		}),
		Client: mustRawJSON(t, map[string]interface{}{
			"server_name": "restls.example.com",
			"restls_opts": map[string]interface{}{
				"password":     "restls-pass",
				"version_hint": "tls13",
			},
		}),
	}
	if err := db.Create(&tlsConfig).Error; err != nil {
		t.Fatalf("create Mihomo TLS failed: %v", err)
	}
	inbound := &model.MihomoInbound{
		Type:  "trojan",
		Tag:   "trojan-restls-source",
		TlsId: tlsConfig.Id,
		Tls:   &tlsConfig,
		Options: mustRawJSON(t, map[string]interface{}{
			"listen_port": 443,
			"password":    "trojan-pass",
		}),
	}
	if err := db.Create(inbound).Error; err != nil {
		t.Fatalf("create Mihomo inbound failed: %v", err)
	}

	subOutbound := &model.SubOutbound{
		Type:            "trojan",
		SourceType:      subManagerSourceMihomoClient,
		SourceInboundId: inbound.Id,
	}
	outbound := map[string]interface{}{"type": "trojan"}
	(&SubManagerSubService{}).refreshSubOutboundTLS(outbound, subOutbound)

	tlsMap, ok := outbound["tls"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected managed wrapper refresh to create tls map: %#v", outbound)
	}
	if tlsMap["enabled"] != true || tlsMap["server_name"] != "restls.example.com" {
		t.Fatalf("unexpected refreshed managed TLS envelope: %#v", tlsMap)
	}
	if opts, ok := tlsMap["restls_opts"].(map[string]interface{}); !ok || opts["password"] != "restls-pass" {
		t.Fatalf("unexpected refreshed Restls options: %#v", tlsMap["restls_opts"])
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
