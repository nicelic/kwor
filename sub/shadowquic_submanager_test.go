package sub

import (
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"gopkg.in/yaml.v3"
)

func TestShadowQUICSubManagerUsesStoredClashOptionsAndFiltersJSON(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "shadowquic-submanager.db")
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

	record := model.SubOutbound{
		Type: "shadowquic",
		Tag:  "sq-sub",
		RawOutbound: json.RawMessage(`{
          "type":"shadowquic",
          "tag":"sq-sub",
          "server":"raw.example.com",
          "server_port":10443,
          "username":"alice",
          "password":"secret",
          "sni":"raw.example.com",
          "tls":{"enabled":true}
        }`),
		ClashOptions: json.RawMessage(`{
          "name":"sq-sub",
          "type":"shadowquic",
          "server":"saved.example.com",
          "port":443,
          "username":"alice",
          "password":"secret",
          "sni":"saved.example.com",
          "zero-rtt":false,
          "tls":true,
          "dialer-proxy":"must-not-survive"
        }`),
	}
	if err := db.Create(&record).Error; err != nil {
		t.Fatalf("create ShadowQUIC suboutbound failed: %v", err)
	}

	jsonSub, err := (&SubManagerSubService{}).GetSubManagerJson(record.Tag)
	if err != nil {
		t.Fatalf("GetSubManagerJson failed: %v", err)
	}
	var jsonDoc map[string]interface{}
	if err := json.Unmarshal([]byte(*jsonSub), &jsonDoc); err != nil {
		t.Fatalf("decode sub-manager JSON failed: %v", err)
	}
	if hasTaggedOutbound(jsonDoc["outbounds"], record.Tag) {
		t.Fatalf("sub-manager sing-box JSON must filter ShadowQUIC: %#v", jsonDoc["outbounds"])
	}

	clashSub, err := (&SubManagerSubService{}).GetSubManagerClash(record.Tag)
	if err != nil {
		t.Fatalf("GetSubManagerClash failed: %v", err)
	}
	var clashDoc map[string]interface{}
	if err := yaml.Unmarshal([]byte(*clashSub), &clashDoc); err != nil {
		t.Fatalf("decode sub-manager Clash output failed: %v", err)
	}
	proxy := findNamedProxy(t, clashDoc["proxies"], record.Tag)
	if proxy["server"] != "saved.example.com" || proxy["sni"] != "saved.example.com" {
		t.Fatalf("expected stored ClashOptions to take precedence, got %#v", proxy)
	}
	if _, exists := proxy["zero-rtt"]; !exists {
		t.Fatalf("expected stored explicit zero-rtt field, got %#v", proxy)
	}
	for _, key := range []string{"tls", "dialer-proxy", "routing-mark", "rule", "proxy"} {
		if _, exists := proxy[key]; exists {
			t.Fatalf("unexpected field %s in sanitized ShadowQUIC output: %#v", key, proxy)
		}
	}
}

func TestShadowQUICSubManagerDoesNotReinjectManagedTLS(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "shadowquic-submanager-tls.db")
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

	tls := &model.MihomoTls{
		Name:   "legacy-tls",
		Server: json.RawMessage(`{"server_name":"tls.example.com"}`),
		Client: json.RawMessage(`{"insecure":true,"utls":{"fingerprint":"chrome"}}`),
	}
	if err := db.Create(tls).Error; err != nil {
		t.Fatalf("create Mihomo TLS failed: %v", err)
	}
	inbound := &model.MihomoInbound{
		Type:  "shadowquic",
		Tag:   "sq-source",
		TlsId: tls.Id,
	}
	if err := db.Create(inbound).Error; err != nil {
		t.Fatalf("create ShadowQUIC source inbound failed: %v", err)
	}

	proxy := map[string]interface{}{
		"name":     "sq-sub",
		"type":     "shadowquic",
		"server":   "edge.example.com",
		"port":     443,
		"username": "alice",
		"password": "secret",
		"sni":      "native.example.com",
	}
	subOutbound := &model.SubOutbound{
		Type:            "shadowquic",
		SourceType:      subManagerSourceMihomoClient,
		SourceInboundId: inbound.Id,
	}

	(&SubManagerSubService{}).refreshSubOutboundClashProxyTLS(proxy, subOutbound)
	if proxy["sni"] != "native.example.com" {
		t.Fatalf("managed TLS must not overwrite ShadowQUIC SNI: %#v", proxy)
	}
	for _, key := range []string{"servername", "skip-cert-verify", "disable-sni", "client-fingerprint", "fingerprint"} {
		if _, exists := proxy[key]; exists {
			t.Fatalf("managed TLS field %s must not reach ShadowQUIC Clash output: %#v", key, proxy)
		}
	}
}
