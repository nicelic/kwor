package service

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
)

func stubSingboxBasicsRuntimeRegeneration(t *testing.T) {
	t.Helper()
	original := regenerateSingboxBasicsRuntimeConfig
	regenerateSingboxBasicsRuntimeConfig = func(*ConfigService) error { return nil }
	t.Cleanup(func() {
		regenerateSingboxBasicsRuntimeConfig = original
	})
}

func TestSaveSingboxBasicsOnlyChangesOwnedSections(t *testing.T) {
	stubSingboxBasicsRuntimeRegeneration(t)
	settingService := initPanelSQLiteSettingTestDB(t)
	initialConfig := `{
		"dns":{"strategy":"prefer_ipv4","rules":[]},
		"route":{"rules":[{"action":"sniff"}],"rule_set":[]},
		"experimental":{"future_feature":{"keep":true}}
	}`
	if err := settingService.SaveSetting("config", initialConfig); err != nil {
		t.Fatalf("seed sing-box config: %v", err)
	}
	initialMihomo := `{"route":{"rules":[{"type":"match","payload":"keep"}]}}`
	if err := settingService.SaveSetting(mihomoConfigSettingKey, initialMihomo); err != nil {
		t.Fatalf("seed Mihomo config: %v", err)
	}
	if err := database.GetDB().Create(&model.Outbound{Type: "socks", Tag: "basic-node", Options: json.RawMessage(`{"server":"127.0.0.1","server_port":1080}`)}).Error; err != nil {
		t.Fatalf("create basic outbound: %v", err)
	}

	configService := &ConfigService{}
	before, err := configService.GetSingboxBasicsEditorContext()
	if err != nil {
		t.Fatalf("load basics context: %v", err)
	}
	result, err := configService.SaveSingboxBasics(SingboxBasicsSaveRequest{
		ExpectedRevision: before.Revision,
		Basics: json.RawMessage(`{
			"ntp":{"enabled":true,"server":"time.apple.com","server_port":123,"interval":"45m","detour":"basic-node","routing_mark":7},
			"experimental":{
				"future_feature":{"keep":true},
				"clash_api":{"external_controller":"127.0.0.1:9090","external_ui_download_detour":"basic-node","access_control_allow_origin":["https://panel.example"]},
				"v2ray_api":{"listen":"127.0.0.1:8080","stats":{"enabled":false,"inbounds":[],"outbounds":[],"users":[]}}
			}
		}`),
	}, "test")
	if err != nil {
		t.Fatalf("save basics: %v", err)
	}
	if !result.Changed || result.Revision != before.Revision+1 {
		t.Fatalf("unexpected basics save result: %#v", result)
	}

	var stored model.Setting
	if err := database.GetDB().Where("key = ?", "config").First(&stored).Error; err != nil {
		t.Fatalf("load saved config: %v", err)
	}
	config := map[string]any{}
	if err := json.Unmarshal([]byte(stored.Value), &config); err != nil {
		t.Fatalf("decode saved config: %v", err)
	}
	if dns, _ := config["dns"].(map[string]any); dns["strategy"] != "prefer_ipv4" {
		t.Fatalf("basics save changed DNS: %#v", dns)
	}
	if route, _ := config["route"].(map[string]any); len(route["rules"].([]any)) != 1 {
		t.Fatalf("basics save changed route: %#v", route)
	}
	if _, exists := config["log"]; exists {
		t.Fatalf("legacy log unexpectedly remained in basic config: %#v", config["log"])
	}
	if ntp, _ := config["ntp"].(map[string]any); ntp["detour"] != "basic-node" || ntp["routing_mark"] != float64(7) {
		t.Fatalf("NTP was not updated: %#v", ntp)
	}
	experimental, _ := config["experimental"].(map[string]any)
	if future, _ := experimental["future_feature"].(map[string]any); future["keep"] != true {
		t.Fatalf("unowned experimental fields were not preserved: %#v", experimental)
	}

	var mihomo model.Setting
	if err := database.GetDB().Where("key = ?", mihomoConfigSettingKey).First(&mihomo).Error; err != nil {
		t.Fatalf("load Mihomo config: %v", err)
	}
	if mihomo.Value != initialMihomo {
		t.Fatalf("focused sing-box basics save touched Mihomo config: %s", mihomo.Value)
	}
}

func TestSaveSingboxBasicsRejectsStaleRevisionAndBadRuntimeReferences(t *testing.T) {
	stubSingboxBasicsRuntimeRegeneration(t)
	settingService := initPanelSQLiteSettingTestDB(t)
	if err := settingService.SaveSetting("config", `{"dns":{"rules":[]},"route":{"rules":[]},"experimental":{}}`); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	if err := database.GetDB().Create(&model.Outbound{Type: "socks", Tag: "basic-node", Options: json.RawMessage(`{"server":"127.0.0.1","server_port":1080}`)}).Error; err != nil {
		t.Fatalf("create basic outbound: %v", err)
	}

	configService := &ConfigService{}
	before, err := configService.GetSingboxBasicsEditorContext()
	if err != nil {
		t.Fatalf("load basics context: %v", err)
	}
	first, err := configService.SaveSingboxBasics(SingboxBasicsSaveRequest{
		ExpectedRevision: before.Revision,
		Basics:           json.RawMessage(`{"ntp":{"enabled":true,"server":"time.apple.com"},"experimental":{}}`),
	}, "test")
	if err != nil || !first.Changed {
		t.Fatalf("first basics mutation failed: result=%#v err=%v", first, err)
	}
	_, err = configService.SaveSingboxBasics(SingboxBasicsSaveRequest{
		ExpectedRevision: before.Revision,
		Basics:           json.RawMessage(`{"experimental":{"cache_file":{"enabled":true}}}`),
	}, "test")
	var conflict *SingboxBasicsRevisionConflictError
	if !errors.As(err, &conflict) || conflict.CurrentRevision != first.Revision {
		t.Fatalf("expected stale basics conflict at revision %d, got %v", first.Revision, err)
	}

	_, err = configService.SaveSingboxBasics(SingboxBasicsSaveRequest{
		ExpectedRevision: first.Revision,
		Basics:           json.RawMessage(`{"ntp":{"enabled":true,"server_port":123.5,"detour":"missing-node"},"experimental":{}}`),
	}, "test")
	if err == nil || (!strings.Contains(err.Error(), "server_port") && !strings.Contains(err.Error(), "detour")) {
		t.Fatalf("invalid NTP data was accepted: %v", err)
	}

	_, err = configService.SaveSingboxBasics(SingboxBasicsSaveRequest{
		ExpectedRevision: first.Revision,
		Basics:           json.RawMessage(`{"experimental":{"v2ray_api":{"stats":{"enabled":true,"inbounds":[],"outbounds":["missing-node"],"users":[]}}}}`),
	}, "test")
	if err == nil || !strings.Contains(err.Error(), "outbounds") {
		t.Fatalf("unknown V2Ray outbound reference was accepted: %v", err)
	}

	after, err := configService.GetSingboxBasicsEditorContext()
	if err != nil {
		t.Fatalf("reload basics context: %v", err)
	}
	if after.Revision != first.Revision {
		t.Fatalf("rejected basics payload advanced revision: before=%d after=%d", first.Revision, after.Revision)
	}
}

func TestSaveSingboxBasicsValidatesAllVisibleNTPDialControls(t *testing.T) {
	stubSingboxBasicsRuntimeRegeneration(t)
	settingService := initPanelSQLiteSettingTestDB(t)
	if err := settingService.SaveSetting("config", `{"dns":{"final":"dns-main","rules":[]},"route":{"rules":[]},"experimental":{}}`); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	if err := database.GetDB().Create(&model.Outbound{Type: "socks", Tag: "basic-node", Options: json.RawMessage(`{"server":"127.0.0.1","server_port":1080}`)}).Error; err != nil {
		t.Fatalf("create outbound: %v", err)
	}
	if err := database.GetDB().Create(&model.DnsServer{Type: "udp", Tag: "dns-main", Options: json.RawMessage(`{"server":"1.1.1.1","server_port":53}`)}).Error; err != nil {
		t.Fatalf("create DNS server: %v", err)
	}

	configService := &ConfigService{}
	context, err := configService.GetSingboxBasicsEditorContext()
	if err != nil {
		t.Fatalf("load basics context: %v", err)
	}
	valid := json.RawMessage(`{
		"ntp":{"enabled":true,"server":"time.apple.com","detour":"basic-node","domain_resolver":"dns-main","bind_interface":"eth0","inet4_bind_address":"192.0.2.1","inet6_bind_address":"2001:db8::1","routing_mark":9,"reuse_addr":true,"tcp_fast_open":false,"tcp_multi_path":true,"udp_fragment":false,"connect_timeout":"1.5s"},
		"experimental":{}
	}`)
	if _, err := configService.SaveSingboxBasics(SingboxBasicsSaveRequest{ExpectedRevision: context.Revision, Basics: valid}, "test"); err != nil {
		t.Fatalf("valid NTP dial controls were rejected: %v", err)
	}

	latest, err := configService.GetSingboxBasicsEditorContext()
	if err != nil {
		t.Fatalf("reload basics context: %v", err)
	}
	for _, payload := range []json.RawMessage{
		json.RawMessage(`{"ntp":{"enabled":true,"domain_resolver":"missing-dns"},"experimental":{}}`),
		json.RawMessage(`{"ntp":{"enabled":true,"tcp_fast_open":"yes"},"experimental":{}}`),
		json.RawMessage(`{"ntp":{"enabled":true,"connect_timeout":"1.5sx"},"experimental":{}}`),
		json.RawMessage(`{"unknown":{},"experimental":{}}`),
	} {
		if _, err := configService.SaveSingboxBasics(SingboxBasicsSaveRequest{ExpectedRevision: latest.Revision, Basics: payload}, "test"); err == nil {
			t.Fatalf("invalid basics payload was accepted: %s", payload)
		}
	}
}

func TestSaveSingboxBasicsRetriesRuntimeWithoutRewritingConfig(t *testing.T) {
	settingService := initPanelSQLiteSettingTestDB(t)
	if err := settingService.SaveSetting("config", `{"dns":{"rules":[]},"route":{"rules":[]},"experimental":{}}`); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	calls := 0
	original := regenerateSingboxBasicsRuntimeConfig
	regenerateSingboxBasicsRuntimeConfig = func(*ConfigService) error {
		calls++
		return nil
	}
	t.Cleanup(func() { regenerateSingboxBasicsRuntimeConfig = original })

	configService := &ConfigService{}
	before, err := configService.GetSingboxBasicsEditorContext()
	if err != nil {
		t.Fatalf("load basics context: %v", err)
	}
	result, err := configService.SaveSingboxBasics(SingboxBasicsSaveRequest{
		ExpectedRevision: before.Revision,
		RetryRuntime:     true,
	}, "test")
	if err != nil || result == nil || result.Changed || result.Revision != before.Revision || calls != 1 {
		t.Fatalf("runtime retry result=%#v calls=%d err=%v", result, calls, err)
	}
}

func TestSingboxRemovalReferenceChecksIncludeV2RayStats(t *testing.T) {
	settingService := initPanelSQLiteSettingTestDB(t)
	if err := database.GetDB().Create(&model.Outbound{Type: "socks", Tag: "node-a", Options: json.RawMessage(`{"server":"127.0.0.1","server_port":1080}`)}).Error; err != nil {
		t.Fatalf("create outbound: %v", err)
	}
	if err := database.GetDB().Create(&model.Inbound{Type: "mixed", Tag: "listener-a", Options: json.RawMessage(`{"listen":"::","listen_port":1080}`)}).Error; err != nil {
		t.Fatalf("create inbound: %v", err)
	}
	if err := settingService.SaveSetting("config", `{
		"dns":{"rules":[]},"route":{"rules":[]},
		"experimental":{"v2ray_api":{"stats":{"enabled":true,"inbounds":["listener-a"],"outbounds":["node-a"],"users":[]}}}
	}`); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	tx := database.GetDB().Begin()
	defer tx.Rollback()
	if err := validateSingboxOutboundRemovalReferences(tx, []string{"node-a"}, nil); err == nil || !strings.Contains(err.Error(), "v2ray_api.stats.outbounds") {
		t.Fatalf("V2Ray outbound reference did not block removal: %v", err)
	}
	if err := tx.Where("tag = ?", "listener-a").Delete(&model.Inbound{}).Error; err != nil {
		t.Fatalf("stage inbound removal: %v", err)
	}
	if err := validateSingboxInboundRemovalReferences(tx, []string{"listener-a"}); err == nil || !strings.Contains(err.Error(), "v2ray_api.stats.inbounds") {
		t.Fatalf("V2Ray inbound reference did not block removal: %v", err)
	}
}

func TestSaveSingboxBasicsRetryIgnoresStaleRevision(t *testing.T) {
	settingService := initPanelSQLiteSettingTestDB(t)
	if err := settingService.SaveSetting("config", `{"dns":{"rules":[]},"route":{"rules":[]},"experimental":{}}`); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	calls := 0
	original := regenerateSingboxBasicsRuntimeConfig
	regenerateSingboxBasicsRuntimeConfig = func(*ConfigService) error { calls++; return nil }
	t.Cleanup(func() { regenerateSingboxBasicsRuntimeConfig = original })

	configService := &ConfigService{}
	before, err := configService.GetSingboxBasicsEditorContext()
	if err != nil {
		t.Fatalf("load basics context: %v", err)
	}
	first, err := configService.SaveSingboxBasics(SingboxBasicsSaveRequest{
		ExpectedRevision: before.Revision,
		Basics:           json.RawMessage(`{"experimental":{"cache_file":{"enabled":true}}}`),
	}, "test")
	if err != nil || first == nil || !first.Changed {
		t.Fatalf("advance basics revision: result=%#v err=%v", first, err)
	}
	result, err := configService.SaveSingboxBasics(SingboxBasicsSaveRequest{
		ExpectedRevision: before.Revision,
		RetryRuntime:     true,
	}, "test")
	if err != nil || result == nil || result.Changed || result.Revision != first.Revision {
		t.Fatalf("stale runtime retry result=%#v err=%v", result, err)
	}
	if calls != 2 {
		t.Fatalf("runtime regeneration calls=%d, want 2", calls)
	}
}
