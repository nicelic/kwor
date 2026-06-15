package service

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alireza0/s-ui/database/model"
)

func TestMergeClientProtocolConfigPreservesExistingPortRange(t *testing.T) {
	outbound := map[string]interface{}{
		"type":         "hysteria",
		"server_port":  float64(38855),
		"server_ports": []interface{}{"31185:35650"},
		"hop_interval": "30s",
	}
	config := map[string]interface{}{
		"name":         "alice",
		"auth_str":     "secret",
		"server_port":  float64(443),
		"server_ports": []interface{}{"41000:45000"},
		"hop_interval": "10s",
	}

	mergeClientProtocolConfig(outbound, config, &model.Inbound{TlsId: 1})

	if got, _ := outbound["auth_str"].(string); got != "secret" {
		t.Fatalf("expected auth_str to be merged, got %v", outbound["auth_str"])
	}

	if got, _ := outbound["server_port"].(float64); got != 38855 {
		t.Fatalf("expected existing server_port=38855, got %v", outbound["server_port"])
	}

	serverPorts, ok := outbound["server_ports"].([]interface{})
	if !ok || len(serverPorts) != 1 || serverPorts[0] != "31185:35650" {
		t.Fatalf("expected existing server_ports kept, got %v", outbound["server_ports"])
	}

	if got, _ := outbound["hop_interval"].(string); got != "30s" {
		t.Fatalf("expected existing hop_interval=30s, got %v", outbound["hop_interval"])
	}
}

func TestMergeClientProtocolConfigSkipsUnsupportedUsernameFields(t *testing.T) {
	outbound := map[string]interface{}{
		"type": "vmess",
	}
	config := map[string]interface{}{
		"username": "client",
		"uuid":     "1234",
	}

	mergeClientProtocolConfig(outbound, config, &model.Inbound{TlsId: 1})

	if _, exists := outbound["username"]; exists {
		t.Fatalf("expected vmess username to be skipped, got %#v", outbound["username"])
	}
	if got, _ := outbound["uuid"].(string); got != "1234" {
		t.Fatalf("expected uuid to be merged, got %#v", outbound["uuid"])
	}
}

func TestMergeClientProtocolConfigForNamespaceKeepsMihomoUsernameFields(t *testing.T) {
	outbound := map[string]interface{}{
		"type": "vmess",
	}
	config := map[string]interface{}{
		"username": "client",
		"uuid":     "1234",
	}

	mergeClientProtocolConfigForNamespace(outbound, config, &model.Inbound{TlsId: 1}, "mihomo")

	if got, _ := outbound["username"].(string); got != "client" {
		t.Fatalf("expected mihomo vmess username to be merged, got %#v", outbound["username"])
	}
	if got, _ := outbound["uuid"].(string); got != "1234" {
		t.Fatalf("expected uuid to be merged, got %#v", outbound["uuid"])
	}
}

func TestMergeClientProtocolConfigForNamespaceKeepsMihomoMieruUsername(t *testing.T) {
	outbound := map[string]interface{}{
		"type": "mieru",
	}
	config := map[string]interface{}{
		"username": "alice",
		"password": "secret",
	}

	mergeClientProtocolConfigForNamespace(outbound, config, &model.Inbound{TlsId: 1}, "mihomo")

	if got, _ := outbound["username"].(string); got != "alice" {
		t.Fatalf("expected mihomo mieru username to be merged, got %#v", outbound["username"])
	}
	if got, _ := outbound["password"].(string); got != "secret" {
		t.Fatalf("expected mihomo mieru password to be merged, got %#v", outbound["password"])
	}
}

func TestMergeClientProtocolConfigForNamespaceKeepsMihomoSnellPSK(t *testing.T) {
	outbound := map[string]interface{}{
		"type": "snell",
	}
	config := map[string]interface{}{
		"psk": "secret-pass",
	}

	mergeClientProtocolConfigForNamespace(outbound, config, &model.Inbound{TlsId: 0}, "mihomo")

	if got, _ := outbound["psk"].(string); got != "secret-pass" {
		t.Fatalf("expected mihomo snell psk to be merged, got %#v", outbound["psk"])
	}
}

func TestMergeClientProtocolConfigSkipsFlowWhenTLSDisabled(t *testing.T) {
	outbound := map[string]interface{}{
		"type": "vless",
	}
	config := map[string]interface{}{
		"flow": "xtls-rprx-vision",
		"uuid": "1234",
	}

	mergeClientProtocolConfig(outbound, config, &model.Inbound{TlsId: 0})

	if _, exists := outbound["flow"]; exists {
		t.Fatalf("expected flow to be skipped when tls is disabled, got %v", outbound["flow"])
	}
	if got, _ := outbound["uuid"].(string); got != "1234" {
		t.Fatalf("expected uuid to be merged, got %v", outbound["uuid"])
	}
}

func TestMergeClientProtocolConfigFillsEmptyOutboundFields(t *testing.T) {
	outbound := map[string]interface{}{
		"type":         "hysteria2",
		"server_ports": []interface{}{},
	}
	config := map[string]interface{}{
		"server_ports": []interface{}{"41000:45000"},
	}

	mergeClientProtocolConfig(outbound, config, &model.Inbound{TlsId: 1})

	serverPorts, ok := outbound["server_ports"].([]interface{})
	if !ok || len(serverPorts) != 1 || serverPorts[0] != "41000:45000" {
		t.Fatalf("expected empty outbound server_ports to be filled, got %v", outbound["server_ports"])
	}
}

func TestMergeClientProtocolConfigPreservesExistingHysteria2Network(t *testing.T) {
	outbound := map[string]interface{}{
		"type":    "hysteria2",
		"network": "udp",
	}
	config := map[string]interface{}{
		"network": "tcp",
	}

	mergeClientProtocolConfig(outbound, config, &model.Inbound{TlsId: 1})

	if got, _ := outbound["network"].(string); got != "udp" {
		t.Fatalf("expected existing hysteria2 network=udp to be preserved, got %#v", outbound["network"])
	}
}

func TestMergeClientProtocolConfigRemovesEmptyHysteria2Network(t *testing.T) {
	outbound := map[string]interface{}{
		"type":    "hysteria2",
		"network": "  ",
	}

	mergeClientProtocolConfig(outbound, map[string]interface{}{}, &model.Inbound{TlsId: 1})

	if _, exists := outbound["network"]; exists {
		t.Fatalf("expected empty hysteria2 network to be removed, got %#v", outbound["network"])
	}
}

func TestBuildSyncedOutboundOverridesServerHost(t *testing.T) {
	svc := &SyncService{}
	inbound := &model.Inbound{
		OutJson: json.RawMessage(`{
			"type": "vless",
			"tag": "in_v6",
			"server": "149.104.4.31",
			"server_port": 443
		}`),
		Options: json.RawMessage(`{}`),
	}

	outbound, _, err := svc.buildSyncedOutbound(nil, inbound, map[string]interface{}{}, "", "2400:f680:dbf:8a82:be24:11ff:fe95:78b6", false)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if got, _ := outbound["server"].(string); got != "2400:f680:dbf:8a82:be24:11ff:fe95:78b6" {
		t.Fatalf("expected server to be overridden, got %v", outbound["server"])
	}
}

func TestBuildSyncedOutboundKeepsServerWhenHostEmpty(t *testing.T) {
	svc := &SyncService{}
	inbound := &model.Inbound{
		OutJson: json.RawMessage(`{
			"type": "vless",
			"tag": "in_v4",
			"server": "149.104.4.31",
			"server_port": 443
		}`),
		Options: json.RawMessage(`{}`),
	}

	outbound, _, err := svc.buildSyncedOutbound(nil, inbound, map[string]interface{}{}, "", "   ", false)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if got, _ := outbound["server"].(string); got != "149.104.4.31" {
		t.Fatalf("expected original server to be preserved, got %v", outbound["server"])
	}
}

func TestBuildSyncedOutboundUsesSubscriptionTLSPayload(t *testing.T) {
	cert := buildSyncTestCertificateMaterial(t, "sync.example.com", 101)
	certPath := filepath.Join(t.TempDir(), "sync-server.pem")
	if err := os.WriteFile(certPath, []byte(cert.pemText+"\n"), 0o644); err != nil {
		t.Fatalf("write certificate failed: %v", err)
	}

	svc := &SyncService{}
	tlsConfig := &model.Tls{
		Server: mustMarshalJSON(t, map[string]interface{}{
			"enabled":          true,
			"server_name":      "sync.example.com",
			"certificate_path": certPath,
		}),
		Client: mustMarshalJSON(t, map[string]interface{}{
			"include_server_certificate":    true,
			"certificate_public_key_sha256": []string{"stale-hash"},
			"tls_store":                     "system",
		}),
	}
	inbound := &model.Inbound{
		TlsId: 1,
		Tls:   tlsConfig,
		OutJson: mustMarshalJSON(t, map[string]interface{}{
			"type":        "hysteria",
			"tag":         "hy1-source",
			"server":      "1.1.1.1",
			"server_port": 443,
			"auth_str":    "secret",
			"tls": map[string]interface{}{
				"enabled": true,
			},
		}),
		Options: mustMarshalJSON(t, map[string]interface{}{}),
	}

	outbound, clashSource, err := svc.buildSyncedOutbound(nil, inbound, map[string]interface{}{}, "", "", true)
	if err != nil {
		t.Fatalf("buildSyncedOutbound returned error: %v", err)
	}

	tlsMap, ok := outbound["tls"].(map[string]interface{})
	if !ok || tlsMap == nil {
		t.Fatalf("expected synced outbound tls map, got %#v", outbound["tls"])
	}
	hashes := stringSliceForSyncTest(t, tlsMap["certificate_public_key_sha256"])
	if len(hashes) != 1 || hashes[0] != cert.publicKeySHA256Base64 {
		t.Fatalf("expected refreshed certificate_public_key_sha256 %q, got %#v", cert.publicKeySHA256Base64, tlsMap["certificate_public_key_sha256"])
	}
	if _, exists := tlsMap["certificate"]; exists {
		t.Fatalf("expected JSON raw node to keep SHA256 mode without PEM, got %#v", tlsMap["certificate"])
	}
	if _, exists := tlsMap["fingerprint"]; exists {
		t.Fatalf("expected JSON raw node to strip Clash fingerprint, got %#v", tlsMap["fingerprint"])
	}
	if _, exists := tlsMap["include_server_certificate"]; exists {
		t.Fatalf("expected JSON raw node to strip include_server_certificate, got %#v", tlsMap["include_server_certificate"])
	}
	if got, _ := tlsMap["tls_store"].(string); got != "system" {
		t.Fatalf("expected synced raw node to keep tls_store for root certificate.store rendering, got %#v", tlsMap["tls_store"])
	}

	clashTLS, ok := clashSource["tls"].(map[string]interface{})
	if !ok || clashTLS == nil {
		t.Fatalf("expected clash source tls map, got %#v", clashSource["tls"])
	}
	if got, _ := clashTLS["fingerprint"].(string); got == "" {
		t.Fatalf("expected clash source to keep refreshed fingerprint for ClashOptions, got %#v", clashTLS["fingerprint"])
	}
}

func TestSaveSyncedSubOutboundOverwritesCachedRawAndClash(t *testing.T) {
	db := initOutboundEditMergeTestDB(t, "sync-suboutbound-exact-cache.db")

	subTag := "s_hy1_sync_user"
	seed := map[string]interface{}{
		"type":        "hysteria",
		"tag":         subTag,
		"server":      "1.1.1.1",
		"server_port": float64(443),
		"auth_str":    "old-secret",
		"raw_only":    "should-be-removed",
	}
	target := &model.SubOutbound{}
	if err := target.UnmarshalJSON(mustMarshalJSON(t, seed)); err != nil {
		t.Fatalf("SubOutbound.UnmarshalJSON failed: %v", err)
	}
	target.RawOutbound = mustMarshalJSON(t, seed)
	target.ClashOptions = mustMarshalJSON(t, map[string]interface{}{
		"name":     subTag,
		"type":     "hysteria",
		"server":   "1.1.1.1",
		"port":     float64(443),
		"auth-str": "old-secret",
	})
	target.SourceType = subOutboundSourceClient
	target.SourceClientId = 11
	target.SourceInboundId = 20
	if err := db.Create(target).Error; err != nil {
		t.Fatalf("create seed suboutbound failed: %v", err)
	}
	oldID := target.Id
	blocker := &model.SubOutbound{
		Type: "direct",
		Tag:  "zz_sync_recreate_blocker",
	}
	if err := db.Create(blocker).Error; err != nil {
		t.Fatalf("create blocker suboutbound failed: %v", err)
	}

	outbound := map[string]interface{}{
		"type":        "hysteria",
		"tag":         "hy1-source",
		"server":      "2.2.2.2",
		"server_port": 8443,
		"auth_str":    "new-secret",
	}
	clashSource := map[string]interface{}{
		"type":        "hysteria",
		"tag":         "hy1-source",
		"server":      "2.2.2.2",
		"server_port": 8443,
		"auth_str":    "new-secret",
	}

	var subOutboundService SubOutboundService
	BeginManagedRuntimeHookScope(db)
	err := (&SyncService{}).saveSyncedSubOutbound(db, &subOutboundService, target, outbound, clashSource, subTag, 11, 22)
	DiscardManagedRuntimeHookScope(db)
	if err != nil {
		t.Fatalf("saveSyncedSubOutbound returned error: %v", err)
	}

	reloaded := &model.SubOutbound{}
	if err := db.Where("tag = ?", subTag).First(reloaded).Error; err != nil {
		t.Fatalf("reload suboutbound failed: %v", err)
	}
	if reloaded.Id == oldID {
		t.Fatalf("expected synced suboutbound to be recreated with a new id, still got id=%d", reloaded.Id)
	}
	var oldRecord model.SubOutbound
	if err := db.Where("id = ?", oldID).First(&oldRecord).Error; err == nil {
		t.Fatalf("expected old suboutbound id=%d to be deleted before recreate", oldID)
	}

	rawMap := mustDecodeJSONMap(t, reloaded.RawOutbound)
	if _, exists := rawMap["raw_only"]; exists {
		t.Fatalf("expected synced RawOutbound to overwrite stale raw_only field, got %#v", rawMap["raw_only"])
	}
	if got, _ := rawMap["server"].(string); got != "2.2.2.2" {
		t.Fatalf("expected synced RawOutbound server=2.2.2.2, got %#v", rawMap["server"])
	}
	if got, _ := rawMap["tag"].(string); got != subTag {
		t.Fatalf("expected synced RawOutbound tag %q, got %#v", subTag, rawMap["tag"])
	}

	proxy := mustDecodeJSONMap(t, reloaded.ClashOptions)
	if got, _ := proxy["auth-str"].(string); got != "new-secret" {
		t.Fatalf("expected ClashOptions to be overwritten with new auth-str, got %#v", proxy["auth-str"])
	}
	if got, _ := proxy["server"].(string); got != "2.2.2.2" {
		t.Fatalf("expected ClashOptions server=2.2.2.2, got %#v", proxy["server"])
	}
	if reloaded.SourceType != subOutboundSourceClient || reloaded.SourceClientId != 11 || reloaded.SourceInboundId != 22 {
		t.Fatalf("unexpected synced source metadata: %#v", reloaded)
	}
}

func TestMergeClientProtocolConfigTrustTunnelUsesClientName(t *testing.T) {
	outbound := map[string]interface{}{
		"type":    "trusttunnel",
		"network": []interface{}{"tcp", "udp"},
	}
	config := map[string]interface{}{
		"uuid":    "tt-secret",
		"network": []interface{}{"udp"},
	}

	mergeClientProtocolConfig(outbound, config, &model.Inbound{TlsId: 1}, "alice")

	if got, _ := outbound["username"].(string); got != "alice" {
		t.Fatalf("expected trusttunnel username alice, got %v", outbound["username"])
	}
	if got, _ := outbound["password"].(string); got != "tt-secret" {
		t.Fatalf("expected trusttunnel password tt-secret, got %v", outbound["password"])
	}
	if got, _ := outbound["udp"].(bool); !got {
		t.Fatalf("expected trusttunnel udp=true from legacy network, got %#v", outbound["udp"])
	}
	if _, exists := outbound["network"]; exists {
		t.Fatalf("expected trusttunnel network to be removed, got %#v", outbound["network"])
	}
}

func TestMergeClientProtocolConfigSudokuMapsUUIDToKey(t *testing.T) {
	outbound := map[string]interface{}{
		"type": "sudoku",
	}
	config := map[string]interface{}{
		"uuid": "12345678-1234-1234-1234-1234567890ab",
	}

	mergeClientProtocolConfig(outbound, config, &model.Inbound{TlsId: 0})

	if got, _ := outbound["key"].(string); got != "12345678-1234-1234-1234-1234567890ab" {
		t.Fatalf("expected sudoku key to come from client uuid, got %#v", outbound["key"])
	}
	if _, exists := outbound["uuid"]; exists {
		t.Fatalf("expected sudoku uuid field to be skipped, got %#v", outbound["uuid"])
	}
}

func TestMergeClientProtocolConfigSudokuPrefersClientUUIDOverInboundHiddenKey(t *testing.T) {
	outbound := map[string]interface{}{
		"type": "sudoku",
	}
	config := map[string]interface{}{
		"uuid": "12345678-1234-1234-1234-1234567890ab",
	}
	inbound := &model.Inbound{
		Options: json.RawMessage(`{"key":"aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee"}`),
	}

	mergeClientProtocolConfig(outbound, config, inbound)

	if got, _ := outbound["key"].(string); got != "12345678-1234-1234-1234-1234567890ab" {
		t.Fatalf("expected sudoku key to prefer client uuid, got %#v", outbound["key"])
	}
}

type syncTestCertificateMaterial struct {
	pemText               string
	publicKeySHA256Base64 string
}

func buildSyncTestCertificateMaterial(t *testing.T, commonName string, serial int64) syncTestCertificateMaterial {
	t.Helper()

	privateKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key failed: %v", err)
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
		t.Fatalf("create certificate failed: %v", err)
	}

	publicKeyDER, err := x509.MarshalPKIXPublicKey(&privateKey.PublicKey)
	if err != nil {
		t.Fatalf("marshal public key failed: %v", err)
	}
	pubKeySum := sha256.Sum256(publicKeyDER)

	return syncTestCertificateMaterial{
		pemText:               strings.TrimSpace(string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certDER}))),
		publicKeySHA256Base64: base64.StdEncoding.EncodeToString(pubKeySum[:]),
	}
}

func stringSliceForSyncTest(t *testing.T, raw interface{}) []string {
	t.Helper()

	switch value := raw.(type) {
	case []string:
		return append([]string(nil), value...)
	case []interface{}:
		result := make([]string, 0, len(value))
		for _, item := range value {
			text, ok := item.(string)
			if !ok {
				t.Fatalf("expected string slice item, got %#v", item)
			}
			result = append(result, text)
		}
		return result
	default:
		t.Fatalf("expected string slice, got %#v", raw)
		return nil
	}
}
