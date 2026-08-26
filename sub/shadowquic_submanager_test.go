package sub

import (
	"encoding/json"
	"path/filepath"
	"strings"
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
          "up":"500",
          "down":600,
		  "udp":true,
		  "ip-version":"ipv6",
		  "routing-mark":100,
		  "routing_mark":101,
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
	for key, want := range map[string]int{"up": 500, "down": 600} {
		if got, ok := proxy[key].(int); !ok || got != want {
			t.Fatalf("expected numeric stored ShadowQUIC %s=%d, got %#v", key, want, proxy[key])
		}
	}
	if strings.Contains(*clashSub, `up: "500"`) || strings.Contains(*clashSub, `down: "600"`) {
		t.Fatalf("stored ShadowQUIC bandwidth must not be emitted as quoted YAML strings: %s", *clashSub)
	}
	if proxy["udp"] != true || proxy["ip-version"] != "ipv6" || proxy["routing-mark"] != 100 {
		t.Fatalf("expected final Mihomo common fields to remain, got %#v", proxy)
	}
	for _, key := range []string{"tls", "dialer-proxy", "routing_mark", "rule", "proxy"} {
		if _, exists := proxy[key]; exists {
			t.Fatalf("unexpected field %s in sanitized ShadowQUIC output: %#v", key, proxy)
		}
	}

	group := &model.SubGroup{Name: "shadowquic-group", Outbounds: `["sq-sub"]`}
	if err := db.Create(group).Error; err != nil {
		t.Fatalf("create ShadowQUIC subgroup failed: %v", err)
	}
	groupClash, err := (&SubManagerSubService{}).GetSubGroupClash(group.Name)
	if err != nil {
		t.Fatalf("GetSubGroupClash failed: %v", err)
	}
	var groupDoc map[string]interface{}
	if err := yaml.Unmarshal([]byte(*groupClash), &groupDoc); err != nil {
		t.Fatalf("decode subgroup Clash output failed: %v", err)
	}
	groupProxy := findNamedProxy(t, groupDoc["proxies"], record.Tag)
	if groupProxy["udp"] != true || groupProxy["ip-version"] != "ipv6" || groupProxy["routing-mark"] != 100 {
		t.Fatalf("expected subgroup Clash output to retain final Mihomo common fields, got %#v", groupProxy)
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
