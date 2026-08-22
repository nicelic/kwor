package service

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
)

func stubSingboxDNSRuntimeRegeneration(t *testing.T) {
	t.Helper()
	original := regenerateSingboxDNSRuntimeConfig
	regenerateSingboxDNSRuntimeConfig = func(*ConfigService) error { return nil }
	t.Cleanup(func() {
		regenerateSingboxDNSRuntimeConfig = original
	})
}

func TestSaveSingboxDNSOnlyChangesDNSAndKeepsMihomoIndependent(t *testing.T) {
	stubSingboxDNSRuntimeRegeneration(t)
	settingService := initPanelSQLiteSettingTestDB(t)
	initialConfig := `{
		"log":{"level":"info"},
		"route":{"rules":[{"action":"sniff"}],"rule_set":[{"tag":"route-set"}]},
		"dns":{"rules":[]}
	}`
	if err := settingService.SaveSetting("config", initialConfig); err != nil {
		t.Fatalf("seed sing-box config: %v", err)
	}
	initialMihomo := `{"dns":{"nameserver":["tls://1.1.1.1"]},"route":{"rules":[{"type":"match","payload":"keep"}]}}`
	if err := settingService.SaveSetting(mihomoConfigSettingKey, initialMihomo); err != nil {
		t.Fatalf("seed mihomo config: %v", err)
	}
	server := model.DnsServer{
		Type:    "tls",
		Tag:     "dns-main",
		Options: json.RawMessage(`{"server":"1.1.1.1","server_port":853,"tls":{"enabled":true}}`),
	}
	if err := database.GetDB().Create(&server).Error; err != nil {
		t.Fatalf("create DNS server: %v", err)
	}
	if err := database.GetDB().Create(&model.Outbound{
		Type:    "direct",
		Tag:     "dns-detour",
		Options: json.RawMessage(`{"server":"127.0.0.1","server_port":1}`),
	}).Error; err != nil {
		t.Fatalf("create DNS detour outbound: %v", err)
	}

	configService := &ConfigService{}
	before, err := configService.GetSingboxDNSSnapshot()
	if err != nil {
		t.Fatalf("load DNS snapshot: %v", err)
	}
	if !containsString(before.DialTags, "dns-detour") {
		t.Fatalf("DNS snapshot omitted default-chain Dial tag: %#v", before.DialTags)
	}
	result, err := configService.SaveSingboxDNS(SingboxDNSMutationRequest{
		ExpectedRevision: before.Revision,
		DNS:              json.RawMessage(`{"final":"dns-main","cache_capacity":1024,"rules":[]}`),
	}, "test")
	if err != nil {
		t.Fatalf("save focused DNS: %v", err)
	}
	if !result.Changed || result.Snapshot == nil || result.Snapshot.Revision != before.Revision+1 {
		t.Fatalf("unexpected DNS save result: %#v", result)
	}

	var stored model.Setting
	if err := database.GetDB().Where("key = ?", "config").First(&stored).Error; err != nil {
		t.Fatalf("load sing-box config: %v", err)
	}
	var config map[string]interface{}
	if err := json.Unmarshal([]byte(stored.Value), &config); err != nil {
		t.Fatalf("decode sing-box config: %v", err)
	}
	if _, exists := config["log"]; exists {
		t.Fatalf("DNS save retained legacy log: %#v", config["log"])
	}
	route, _ := config["route"].(map[string]interface{})
	if _, ok := route["rule_set"]; !ok {
		t.Fatalf("DNS save removed route.rule_set: %#v", route)
	}
	dns, _ := config["dns"].(map[string]interface{})
	if dns["final"] != "dns-main" || dns["cache_capacity"] != float64(1024) {
		t.Fatalf("DNS section was not updated: %#v", dns)
	}
	if _, exists := dns["servers"]; exists {
		t.Fatalf("DNS cards must not be embedded in config: %#v", dns)
	}

	var mihomo model.Setting
	if err := database.GetDB().Where("key = ?", mihomoConfigSettingKey).First(&mihomo).Error; err != nil {
		t.Fatalf("load Mihomo config: %v", err)
	}
	if mihomo.Value != initialMihomo {
		t.Fatalf("focused sing-box DNS save touched Mihomo config: %s", mihomo.Value)
	}
}

func TestSaveSingboxDNSCanonicalizesDoHPaths(t *testing.T) {
	stubSingboxDNSRuntimeRegeneration(t)
	settingService := initPanelSQLiteSettingTestDB(t)
	if err := settingService.SaveSetting("config", `{"dns":{"rules":[]}}`); err != nil {
		t.Fatalf("seed sing-box config: %v", err)
	}

	configService := &ConfigService{}
	before, err := configService.GetSingboxDNSSnapshot()
	if err != nil {
		t.Fatalf("load initial DNS snapshot: %v", err)
	}
	result, err := configService.SaveSingboxDNS(SingboxDNSMutationRequest{
		ExpectedRevision: before.Revision,
		ServerAction:     "new",
		Server:           json.RawMessage(`{"type":"https","tag":"google-doh","server":"dns.google","server_port":443,"path":"dns-query","tls":{"enabled":true}}`),
	}, "test")
	if err != nil {
		t.Fatalf("save DoH server: %v", err)
	}
	if result.Snapshot == nil {
		t.Fatal("DoH save did not return a snapshot")
	}
	var dohPath string
	for _, server := range result.Snapshot.Servers {
		if server["tag"] == "google-doh" {
			dohPath, _ = server["path"].(string)
		}
	}
	if dohPath != "/dns-query" {
		t.Fatalf("DoH path = %q, want /dns-query", dohPath)
	}

	result, err = configService.SaveSingboxDNS(SingboxDNSMutationRequest{
		ExpectedRevision: result.Snapshot.Revision,
		ServerAction:     "new",
		Server:           json.RawMessage(`{"type":"h3","tag":"google-doh3","server":"dns.google","server_port":443,"path":"/dns-query","tls":{"enabled":true}}`),
	}, "test")
	if err != nil {
		t.Fatalf("save DoH3 server: %v", err)
	}
	if result.Snapshot == nil {
		t.Fatal("DoH3 save did not return a snapshot")
	}
	var doh3Path string
	for _, server := range result.Snapshot.Servers {
		if server["tag"] == "google-doh3" {
			doh3Path, _ = server["path"].(string)
		}
	}
	if doh3Path != "/dns-query" {
		t.Fatalf("DoH3 path = %q, want /dns-query", doh3Path)
	}

	result, err = configService.SaveSingboxDNS(SingboxDNSMutationRequest{
		ExpectedRevision: result.Snapshot.Revision,
		DNS:              json.RawMessage(`{"final":"google-doh","rules":[]}`),
	}, "test")
	if err != nil {
		t.Fatalf("select canonicalized DoH server: %v", err)
	}
	generated, err := (&ProManagerService{ConfigService: configService}).GenerateFullConfig()
	if err != nil {
		t.Fatalf("generate full config: %v", err)
	}
	generatedDNS := map[string]interface{}{}
	if err := json.Unmarshal(generated.Dns, &generatedDNS); err != nil {
		t.Fatalf("decode generated DNS config: %v", err)
	}
	servers, _ := generatedDNS["servers"].([]interface{})
	if len(servers) != 1 {
		t.Fatalf("generated DNS server count = %d, want 1", len(servers))
	}
	server, _ := servers[0].(map[string]interface{})
	if path, _ := server["path"].(string); path != "/dns-query" {
		t.Fatalf("generated DoH path = %q, want /dns-query", path)
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func TestSaveSingboxDNSRejectsStaleRevisionAndMihomoDoesNotAdvanceIt(t *testing.T) {
	stubSingboxDNSRuntimeRegeneration(t)
	settingService := initPanelSQLiteSettingTestDB(t)
	if err := settingService.SaveSetting("config", `{"dns":{"rules":[]},"route":{"rules":[]}}`); err != nil {
		t.Fatalf("seed sing-box config: %v", err)
	}
	configService := &ConfigService{}
	before, err := configService.GetSingboxDNSSnapshot()
	if err != nil {
		t.Fatalf("load first snapshot: %v", err)
	}

	tx := database.GetDB().Begin()
	if tx.Error != nil {
		t.Fatalf("begin Mihomo transaction: %v", tx.Error)
	}
	if err := (&MihomoConfigService{}).SaveConfig(tx, json.RawMessage(`{"dns":{"nameserver":["tls://8.8.8.8"]}}`)); err != nil {
		tx.Rollback()
		t.Fatalf("save Mihomo config: %v", err)
	}
	if err := tx.Commit().Error; err != nil {
		t.Fatalf("commit Mihomo config: %v", err)
	}
	afterMihomo, err := configService.GetSingboxDNSSnapshot()
	if err != nil {
		t.Fatalf("load snapshot after Mihomo save: %v", err)
	}
	if afterMihomo.Revision != before.Revision {
		t.Fatalf("Mihomo save changed sing-box DNS revision: before=%d after=%d", before.Revision, afterMihomo.Revision)
	}

	first, err := configService.SaveSingboxDNS(SingboxDNSMutationRequest{
		ExpectedRevision: before.Revision,
		DNS:              json.RawMessage(`{"strategy":"prefer_ipv4","rules":[]}`),
	}, "test")
	if err != nil || !first.Changed {
		t.Fatalf("first DNS mutation failed: result=%#v err=%v", first, err)
	}
	_, err = configService.SaveSingboxDNS(SingboxDNSMutationRequest{
		ExpectedRevision: before.Revision,
		DNS:              json.RawMessage(`{"strategy":"ipv4_only","rules":[]}`),
	}, "test")
	if !IsSingboxDNSRevisionConflict(err) {
		t.Fatalf("expected stale revision error, got %v", err)
	}
	if current := SingboxDNSRevisionFromConflict(err); current != before.Revision+1 {
		t.Fatalf("unexpected current revision: %d", current)
	}
}

func TestSingboxDNSResourceBoundsAndLegacyCardBypass(t *testing.T) {
	settingService := initPanelSQLiteSettingTestDB(t)
	if err := settingService.SaveSetting("config", `{"dns":{"rules":[]}}`); err != nil {
		t.Fatalf("seed config: %v", err)
	}

	tooManyRules := make([]interface{}, SingboxDNSMaxRules+1)
	for index := range tooManyRules {
		tooManyRules[index] = map[string]interface{}{"action": "reject"}
	}
	if err := validateSingboxDNSConfigMap(map[string]interface{}{"rules": tooManyRules}); err == nil || !strings.Contains(err.Error(), "DNS") {
		t.Fatalf("expected DNS rule count limit, got %v", err)
	}
	if err := validateSingboxDNSConfigMap(map[string]interface{}{"cache_capacity": SingboxDNSMaxCacheCapacity + 1, "rules": []interface{}{}}); err == nil || !strings.Contains(err.Error(), "cache_capacity") {
		t.Fatalf("expected cache capacity limit, got %v", err)
	}

	headers := make(map[string]interface{}, SingboxDNSMaxHeaders+1)
	for index := 0; index <= SingboxDNSMaxHeaders; index++ {
		headers["X-Test-"+string(rune('A'+index))] = "value"
	}
	if err := validateSingboxDNSServerPayload(&model.DnsServer{
		Type:    "https",
		Tag:     "too-many-headers",
		Options: mustMarshalDNSJSON(t, map[string]interface{}{"headers": headers}),
	}); err == nil || !strings.Contains(err.Error(), "headers") {
		t.Fatalf("expected DNS header limit, got %v", err)
	}

	_, err := (&ConfigService{}).Save("dnsservers", "new", json.RawMessage(`{"type":"local","tag":"bypass"}`), "", "test", "")
	if err == nil {
		t.Fatal("removed DNS card object unexpectedly remained a valid generic save target")
	}
}

func TestValidateSingboxDNSServerPayloadRejectsUnknownTypeAndInvalidPort(t *testing.T) {
	unknown := &model.DnsServer{Type: "bogus", Tag: "bad", Options: json.RawMessage(`{}`)}
	if err := validateSingboxDNSServerPayload(unknown); err == nil || !strings.Contains(err.Error(), "not supported") {
		t.Fatalf("unknown DNS server type was accepted: %v", err)
	}
	missingAddress := &model.DnsServer{Type: "udp", Tag: "missing", Options: json.RawMessage(`{"server_port":53}`)}
	if err := validateSingboxDNSServerPayload(missingAddress); err == nil || !strings.Contains(err.Error(), "server address") {
		t.Fatalf("DNS server without address was accepted: %v", err)
	}
	badPort := &model.DnsServer{Type: "udp", Tag: "fractional", Options: json.RawMessage(`{"server":"1.1.1.1","server_port":53.5}`)}
	if err := validateSingboxDNSServerPayload(badPort); err == nil || !strings.Contains(err.Error(), "server_port") {
		t.Fatalf("fractional DNS server port was accepted: %v", err)
	}
}

func TestValidateSingboxDNSRuleRejectsTruncatedNumericValues(t *testing.T) {
	cases := []map[string]any{
		{"action": "route", "port": []any{"80x"}},
		{"action": "route", "source_port": []any{80.5}},
		{"action": "route", "ip_version": 5},
		{"action": "route", "rewrite_ttl": 1.5},
	}
	for index, rule := range cases {
		if err := validateSingboxDNSRule(rule, 1); err == nil {
			t.Fatalf("invalid DNS rule #%d was accepted: %#v", index+1, rule)
		}
	}
}

func mustMarshalDNSJSON(t *testing.T, value interface{}) json.RawMessage {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal DNS fixture: %v", err)
	}
	return data
}

func TestSaveSingboxDNSResourceErrorDoesNotWriteState(t *testing.T) {
	stubSingboxDNSRuntimeRegeneration(t)
	settingService := initPanelSQLiteSettingTestDB(t)
	if err := settingService.SaveSetting("config", `{"dns":{"rules":[]}}`); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	configService := &ConfigService{}
	before, err := configService.GetSingboxDNSSnapshot()
	if err != nil {
		t.Fatalf("load snapshot: %v", err)
	}
	_, err = configService.SaveSingboxDNS(SingboxDNSMutationRequest{
		ExpectedRevision: before.Revision,
		DNS:              json.RawMessage(`{"cache_capacity":999999999,"rules":[]}`),
	}, "test")
	if err == nil {
		t.Fatal("oversized cache capacity was accepted")
	}
	var conflict *SingboxDNSRevisionConflictError
	if errors.As(err, &conflict) {
		t.Fatalf("resource limit must not be reported as revision conflict: %v", err)
	}
	after, err := configService.GetSingboxDNSSnapshot()
	if err != nil {
		t.Fatalf("load post-error snapshot: %v", err)
	}
	if after.Revision != before.Revision {
		t.Fatalf("rejected DNS mutation advanced revision: before=%d after=%d", before.Revision, after.Revision)
	}
}

func TestSaveSingboxDNSRuntimeRetryDoesNotMutateDatabase(t *testing.T) {
	settingService := initPanelSQLiteSettingTestDB(t)
	if err := settingService.SaveSetting("config", `{"dns":{"rules":[]},"route":{"rules":[]}}`); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	configService := &ConfigService{}
	before, err := configService.GetSingboxDNSSnapshot()
	if err != nil {
		t.Fatalf("load snapshot: %v", err)
	}
	var setting model.Setting
	if err := database.GetDB().Where("key = ?", "config").First(&setting).Error; err != nil {
		t.Fatalf("load config before retry: %v", err)
	}
	var auditsBefore int64
	if err := database.GetDB().Model(&model.Changes{}).Where("key = ?", "singbox_dns").Count(&auditsBefore).Error; err != nil {
		t.Fatalf("count audits before retry: %v", err)
	}

	original := regenerateSingboxDNSRuntimeConfig
	calls := 0
	regenerateSingboxDNSRuntimeConfig = func(*ConfigService) error {
		calls++
		return nil
	}
	t.Cleanup(func() {
		regenerateSingboxDNSRuntimeConfig = original
	})

	result, err := configService.SaveSingboxDNS(SingboxDNSMutationRequest{
		ExpectedRevision: before.Revision,
		RetryRuntime:     true,
	}, "test")
	if err != nil || result == nil || result.Changed || calls != 1 {
		t.Fatalf("runtime retry result=%#v calls=%d err=%v", result, calls, err)
	}
	if result.Snapshot == nil || result.Snapshot.Revision != before.Revision {
		t.Fatalf("runtime retry changed snapshot revision: %#v", result.Snapshot)
	}
	var afterSetting model.Setting
	if err := database.GetDB().Where("key = ?", "config").First(&afterSetting).Error; err != nil {
		t.Fatalf("load config after retry: %v", err)
	}
	if afterSetting.Value != setting.Value {
		t.Fatalf("runtime retry changed config: before=%s after=%s", setting.Value, afterSetting.Value)
	}
	var auditsAfter int64
	if err := database.GetDB().Model(&model.Changes{}).Where("key = ?", "singbox_dns").Count(&auditsAfter).Error; err != nil {
		t.Fatalf("count audits after retry: %v", err)
	}
	if auditsAfter != auditsBefore {
		t.Fatalf("runtime retry wrote an audit record: before=%d after=%d", auditsBefore, auditsAfter)
	}
}

func TestNormalizeDNSRuleServerRecursesIntoLogicalChildren(t *testing.T) {
	rules := []interface{}{map[string]interface{}{
		"type": "logical",
		"mode": "and",
		"rules": []interface{}{
			map[string]interface{}{"action": "route", "server": "stale"},
			map[string]interface{}{
				"type": "logical",
				"mode": "or",
				"rules": []interface{}{
					map[string]interface{}{"action": "route", "server": "another-stale"},
				},
			},
		},
	}}
	for _, rule := range rules {
		normalizeDNSRuleServer(rule, "dns-final")
	}
	outer := rules[0].(map[string]interface{})
	children := outer["rules"].([]interface{})
	if got := children[0].(map[string]interface{})["server"]; got != "dns-final" {
		t.Fatalf("top-level logical child server = %#v, want dns-final", got)
	}
	nested := children[1].(map[string]interface{})["rules"].([]interface{})
	if got := nested[0].(map[string]interface{})["server"]; got != "dns-final" {
		t.Fatalf("nested logical child server = %#v, want dns-final", got)
	}
}

func TestSaveSingboxDNSValidatesRuntimeReferences(t *testing.T) {
	stubSingboxDNSRuntimeRegeneration(t)
	settingService := initPanelSQLiteSettingTestDB(t)
	if err := settingService.SaveSetting("config", `{"dns":{"rules":[]}}`); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	configService := &ConfigService{}
	context, err := configService.GetSingboxDNSSnapshot()
	if err != nil {
		t.Fatalf("load snapshot: %v", err)
	}

	_, err = configService.SaveSingboxDNS(SingboxDNSMutationRequest{
		ExpectedRevision: context.Revision,
		ServerAction:     "new",
		Server:           json.RawMessage(`{"type":"udp","tag":"bad-detour","server":"1.1.1.1","detour":"missing"}`),
	}, "test")
	if err == nil || !strings.Contains(err.Error(), "unknown detour") {
		t.Fatalf("missing DNS detour was accepted: %v", err)
	}

	_, err = configService.SaveSingboxDNS(SingboxDNSMutationRequest{
		ExpectedRevision: context.Revision,
		ServerAction:     "new",
		Server:           json.RawMessage(`{"type":"tailscale","tag":"bad-tailscale"}`),
	}, "test")
	if err == nil || !strings.Contains(err.Error(), "requires an endpoint") {
		t.Fatalf("tailscale DNS card without endpoint was accepted: %v", err)
	}

	_, err = configService.SaveSingboxDNS(SingboxDNSMutationRequest{
		ExpectedRevision: context.Revision,
		ServerAction:     "new",
		Server:           json.RawMessage(`{"type":"resolved","tag":"bad-resolved"}`),
	}, "test")
	if err == nil || !strings.Contains(err.Error(), "requires a service") {
		t.Fatalf("resolved DNS card without service was accepted: %v", err)
	}

	if err := database.GetDB().Create(&model.Outbound{Type: "direct", Tag: "dns-detour"}).Error; err != nil {
		t.Fatalf("create DNS detour: %v", err)
	}
	if err := database.GetDB().Create(&model.Endpoint{Type: "tailscale", Tag: "ts-endpoint", Options: json.RawMessage(`{}`)}).Error; err != nil {
		t.Fatalf("create tailscale endpoint: %v", err)
	}
	if err := database.GetDB().Create(&model.Service{Type: "resolved", Tag: "resolved-service", Options: json.RawMessage(`{}`)}).Error; err != nil {
		t.Fatalf("create resolved service: %v", err)
	}

	result, err := configService.SaveSingboxDNS(SingboxDNSMutationRequest{
		ExpectedRevision: context.Revision,
		ServerAction:     "new",
		Server:           json.RawMessage(`{"type":"udp","tag":"valid-detour","server":"1.1.1.1","detour":"dns-detour"}`),
	}, "test")
	if err != nil || result == nil || !result.Changed || result.Snapshot == nil {
		t.Fatalf("valid DNS detour mutation failed: result=%#v err=%v", result, err)
	}

	result, err = configService.SaveSingboxDNS(SingboxDNSMutationRequest{
		ExpectedRevision: result.Snapshot.Revision,
		ServerAction:     "new",
		Server:           json.RawMessage(`{"type":"tailscale","tag":"valid-tailscale","endpoint":"ts-endpoint"}`),
	}, "test")
	if err != nil || result == nil || !result.Changed || result.Snapshot == nil {
		t.Fatalf("valid tailscale DNS mutation failed: result=%#v err=%v", result, err)
	}

	result, err = configService.SaveSingboxDNS(SingboxDNSMutationRequest{
		ExpectedRevision: result.Snapshot.Revision,
		ServerAction:     "new",
		Server:           json.RawMessage(`{"type":"resolved","tag":"valid-resolved","service":"resolved-service"}`),
	}, "test")
	if err != nil || result == nil || !result.Changed {
		t.Fatalf("valid resolved DNS mutation failed: result=%#v err=%v", result, err)
	}
}

func TestSaveSingboxDNSRetryIgnoresStaleRevision(t *testing.T) {
	settingService := initPanelSQLiteSettingTestDB(t)
	if err := settingService.SaveSetting("config", `{"dns":{"rules":[]},"route":{"rules":[]}}`); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	calls := 0
	original := regenerateSingboxDNSRuntimeConfig
	regenerateSingboxDNSRuntimeConfig = func(*ConfigService) error { calls++; return nil }
	t.Cleanup(func() { regenerateSingboxDNSRuntimeConfig = original })

	configService := &ConfigService{}
	before, err := configService.GetSingboxDNSSnapshot()
	if err != nil {
		t.Fatalf("load DNS snapshot: %v", err)
	}
	first, err := configService.SaveSingboxDNS(SingboxDNSMutationRequest{
		ExpectedRevision: before.Revision,
		DNS:              json.RawMessage(`{"strategy":"prefer_ipv4","rules":[]}`),
	}, "test")
	if err != nil || first == nil || !first.Changed || first.Snapshot == nil {
		t.Fatalf("advance DNS revision: result=%#v err=%v", first, err)
	}
	result, err := configService.SaveSingboxDNS(SingboxDNSMutationRequest{
		ExpectedRevision: before.Revision,
		RetryRuntime:     true,
	}, "test")
	if err != nil || result == nil || result.Changed || result.Snapshot == nil || result.Snapshot.Revision != first.Snapshot.Revision {
		t.Fatalf("stale runtime retry result=%#v err=%v", result, err)
	}
	if calls != 2 {
		t.Fatalf("runtime regeneration calls=%d, want 2", calls)
	}
}
