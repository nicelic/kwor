package service

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/alireza0/s-ui/database/model"
)

func testMihomoProxyTargets() *mihomoProxyConversionResult {
	return &mihomoProxyConversionResult{
		SupportedTags: map[string]struct{}{
			"DIRECT":      {},
			"REJECT":      {},
			"REJECT-DROP": {},
			"proxy-a":     {},
		},
		DirectTags: map[string]struct{}{},
	}
}

func testMihomoTLSWithWrapperProxy(mode string, proxy string) *model.MihomoTls {
	server := map[string]interface{}{}
	switch mode {
	case model.MihomoTlsModeShadowTLS:
		server["shadow_tls"] = map[string]interface{}{
			"version":   3,
			"users":     []interface{}{map[string]interface{}{"name": "alice", "password": "shadow-pass"}},
			"handshake": map[string]interface{}{"dest": "edge.example.com:443", "proxy": proxy},
			"handshake_for_server_name": map[string]interface{}{
				"cdn.example.com": map[string]interface{}{"dest": "cdn.example.com:443", "proxy": proxy},
			},
		}
	case model.MihomoTlsModeRestls:
		server["res_tls"] = map[string]interface{}{"dest": "edge.example.com:443", "password": "restls-pass", "proxy": proxy}
	case model.MihomoTlsModeJLS:
		server["jls_config"] = map[string]interface{}{
			"users": []interface{}{map[string]interface{}{"username": "alice", "password": "jls-pass"}},
			"dest":  "edge.example.com:443",
			"proxy": proxy,
		}
	}
	encoded, _ := json.Marshal(server)
	return &model.MihomoTls{Mode: mode, Server: encoded, Client: json.RawMessage(`{}`)}
}

func TestValidateMihomoTLSOutboundReferencesCoversAllWrapperProxyFields(t *testing.T) {
	for _, mode := range []string{model.MihomoTlsModeShadowTLS, model.MihomoTlsModeRestls, model.MihomoTlsModeJLS} {
		t.Run(mode, func(t *testing.T) {
			tls := testMihomoTLSWithWrapperProxy(mode, "proxy-a")
			if err := validateMihomoTLSOutboundReferences(tls, testMihomoProxyTargets()); err != nil {
				t.Fatalf("valid %s wrapper proxy was rejected: %v", mode, err)
			}

			tls = testMihomoTLSWithWrapperProxy(mode, "missing-proxy")
			err := validateMihomoTLSOutboundReferences(tls, testMihomoProxyTargets())
			if err == nil || !strings.Contains(err.Error(), "missing-proxy") {
				t.Fatalf("expected invalid %s wrapper proxy error, got %v", mode, err)
			}
		})
	}
}

func TestMihomoTlsSaveRejectsUnknownWrapperProxy(t *testing.T) {
	db := initMihomoManagerFinalFallbackTestDB(t, "mihomo-tls-unknown-wrapper-proxy.db")
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("begin transaction failed: %v", tx.Error)
	}
	defer tx.Rollback()

	tls := testMihomoTLSWithWrapperProxy(model.MihomoTlsModeJLS, "missing-proxy")
	tls.Client = json.RawMessage(`{"jls_opts":{"username":"alice","password":"jls-pass"}}`)
	payload, err := json.Marshal(tls)
	if err != nil {
		t.Fatalf("marshal TLS payload failed: %v", err)
	}
	err = (&MihomoTlsService{}).Save(tx, "new", payload, "panel.example.com")
	if err == nil || !strings.Contains(err.Error(), "missing-proxy") {
		t.Fatalf("expected unknown wrapper proxy rejection, got %v", err)
	}
}

func TestMihomoTlsSaveNormalizesWrapperDirectProxy(t *testing.T) {
	db := initMihomoManagerFinalFallbackTestDB(t, "mihomo-tls-wrapper-direct-proxy.db")
	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("begin transaction failed: %v", tx.Error)
	}

	tls := testMihomoTLSWithWrapperProxy(model.MihomoTlsModeJLS, "direct")
	tls.Client = json.RawMessage(`{"jls_opts":{"username":"alice","password":"jls-pass"}}`)
	payload, err := json.Marshal(tls)
	if err != nil {
		tx.Rollback()
		t.Fatalf("marshal TLS payload failed: %v", err)
	}
	if err := (&MihomoTlsService{}).Save(tx, "new", payload, "panel.example.com"); err != nil {
		tx.Rollback()
		t.Fatalf("save TLS failed: %v", err)
	}
	if err := tx.Commit().Error; err != nil {
		t.Fatalf("commit TLS transaction failed: %v", err)
	}

	var stored model.MihomoTls
	if err := db.Last(&stored).Error; err != nil {
		t.Fatalf("load saved TLS failed: %v", err)
	}
	var server map[string]interface{}
	if err := json.Unmarshal(stored.Server, &server); err != nil {
		t.Fatalf("parse saved TLS server configuration failed: %v", err)
	}
	wrapper, _ := server["jls_config"].(map[string]interface{})
	if wrapper["proxy"] != "DIRECT" {
		t.Fatalf("saved JLS proxy = %#v, want DIRECT", wrapper["proxy"])
	}
}

func TestMihomoTlsSaveNormalizesWrapperDestinationBeforeValidation(t *testing.T) {
	db := initMihomoManagerFinalFallbackTestDB(t, "mihomo-tls-wrapper-destination.db")
	tls := &model.MihomoTls{
		Name:   "restls-host-only",
		Mode:   model.MihomoTlsModeRestls,
		Server: json.RawMessage(`{"res_tls":{"dest":"edge.example.com","password":"restls-pass"}}`),
		Client: json.RawMessage(`{"restls_opts":{"password":"restls-pass","version_hint":"tls13"}}`),
	}
	payload, err := json.Marshal(tls)
	if err != nil {
		t.Fatalf("marshal TLS payload failed: %v", err)
	}
	if err := (&MihomoTlsService{}).Save(db, "new", payload, "panel.example.com"); err != nil {
		t.Fatalf("save TLS failed: %v", err)
	}

	var stored model.MihomoTls
	if err := db.Where("name = ?", tls.Name).First(&stored).Error; err != nil {
		t.Fatalf("load saved TLS failed: %v", err)
	}
	var server, client map[string]interface{}
	if err := json.Unmarshal(stored.Server, &server); err != nil {
		t.Fatalf("parse saved TLS server configuration failed: %v", err)
	}
	if err := json.Unmarshal(stored.Client, &client); err != nil {
		t.Fatalf("parse saved TLS client configuration failed: %v", err)
	}
	wrapper, ok := server["res_tls"].(map[string]interface{})
	if !ok || wrapper["dest"] != "edge.example.com:443" {
		t.Fatalf("stored Restls destination = %#v, want edge.example.com:443", wrapper)
	}
	if client["server_name"] != "edge.example.com" {
		t.Fatalf("stored Restls SNI = %#v, want edge.example.com", client["server_name"])
	}
}

func TestMihomoOutboundDeleteRejectsTLSWrapperProxyReference(t *testing.T) {
	db := initMihomoManagerFinalFallbackTestDB(t, "mihomo-tls-wrapper-proxy-delete.db")
	insertMihomoOutboundForFallbackTest(t, db, `{"type":"socks","tag":"proxy-a","server":"127.0.0.1","server_port":1080}`)

	tls := testMihomoTLSWithWrapperProxy(model.MihomoTlsModeJLS, "proxy-a")
	tls.Name = "wrapper-tls"
	tls.Client = json.RawMessage(`{"jls_opts":{"username":"alice","password":"jls-pass"}}`)
	if err := db.Create(tls).Error; err != nil {
		t.Fatalf("create Mihomo TLS failed: %v", err)
	}

	tx := db.Begin()
	if tx.Error != nil {
		t.Fatalf("begin transaction failed: %v", tx.Error)
	}
	defer tx.Rollback()
	err := (&MihomoOutboundService{}).Save(tx, "del", json.RawMessage(`"proxy-a"`))
	if err == nil || !strings.Contains(err.Error(), `mihomo TLS "wrapper-tls" jls-config.proxy -> proxy-a`) {
		t.Fatalf("expected TLS wrapper reference rejection, got %v", err)
	}
}

func TestMihomoGeneratorRejectsBoundTLSWrapperUnknownProxy(t *testing.T) {
	db := initMihomoManagerFinalFallbackTestDB(t, "mihomo-tls-wrapper-proxy-generator.db")
	tls := testMihomoTLSWithWrapperProxy(model.MihomoTlsModeJLS, "missing-proxy")
	tls.Client = json.RawMessage(`{"jls_opts":{"username":"alice","password":"jls-pass"}}`)
	if err := db.Create(tls).Error; err != nil {
		t.Fatalf("create Mihomo TLS failed: %v", err)
	}
	inbound := model.MihomoInbound{
		Type:  "shadowsocks",
		Tag:   "ss-wrapper",
		TlsId: tls.Id,
		Options: json.RawMessage(`{
			"listen":"::",
			"listen_port":18443,
			"method":"2022-blake3-aes-128-gcm",
			"password":"server-pass"
		}`),
	}
	if err := db.Create(&inbound).Error; err != nil {
		t.Fatalf("create Mihomo inbound failed: %v", err)
	}

	_, err := NewMihomoManagerService().GenerateServerDocument()
	if err == nil || !strings.Contains(err.Error(), "missing-proxy") {
		t.Fatalf("expected generator to reject unknown TLS wrapper proxy, got %v", err)
	}
}
