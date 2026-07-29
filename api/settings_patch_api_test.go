package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/service"
	"github.com/gin-gonic/gin"
)

func TestSettingsPatchCASUpsertNoopAndCompactAudit(t *testing.T) {
	setupSettingsPatchAPITestDB(t)
	apiService := &ApiService{}
	snapshot, err := apiService.SettingService.GetSettingsSnapshot()
	if err != nil {
		t.Fatalf("load snapshot: %v", err)
	}
	if snapshot.Revision != 1 || snapshot.Defaults.JSONExt == "" || snapshot.Defaults.ClashExt == "" {
		t.Fatalf("unexpected snapshot: %#v", snapshot)
	}

	if err := database.GetDB().Where("key = ?", "subURI").Delete(&model.Setting{}).Error; err != nil {
		t.Fatalf("delete setting row: %v", err)
	}
	_, first := performSettingsPatchJSONPost(t, apiService.SaveSettingsPatch, `{"expectedRevision":1,"changes":{"subURI":"https://example.com/sub"}}`)
	if !first.Success {
		t.Fatalf("patch failed: %s", first.Msg)
	}
	firstResult := decodeSettingsPatchResult(t, first.Obj)
	if firstResult.Revision != 2 || len(firstResult.ChangedKeys) != 1 || firstResult.ChangedKeys[0] != "subURI" {
		t.Fatalf("unexpected first patch result: %#v", firstResult)
	}
	var setting model.Setting
	if err := database.GetDB().Where("key = ?", "subURI").First(&setting).Error; err != nil || setting.Value != "https://example.com/sub/" {
		t.Fatalf("missing UPSERT value: setting=%#v err=%v", setting, err)
	}

	var auditCount int64
	if err := database.GetDB().Model(&model.Changes{}).Where("key = ? AND action = ?", "settings", "patch").Count(&auditCount).Error; err != nil {
		t.Fatalf("count audit rows: %v", err)
	}
	_, noop := performSettingsPatchJSONPost(t, apiService.SaveSettingsPatch, `{"expectedRevision":2,"changes":{"subURI":"https://example.com/sub"}}`)
	if !noop.Success || decodeSettingsPatchResult(t, noop.Obj).Revision != 2 {
		t.Fatalf("no-op patch failed or changed revision: %#v", noop)
	}
	var afterNoopCount int64
	_ = database.GetDB().Model(&model.Changes{}).Where("key = ? AND action = ?", "settings", "patch").Count(&afterNoopCount).Error
	if afterNoopCount != auditCount {
		t.Fatalf("no-op patch wrote audit: before=%d after=%d", auditCount, afterNoopCount)
	}

	extension := `{"latency_test_interval":"10m","latency_tolerance":50}`
	body, _ := json.Marshal(map[string]interface{}{
		"expectedRevision": 2,
		"changes":          map[string]string{"subJsonExt": extension},
	})
	_, extensionResponse := performSettingsPatchJSONPost(t, apiService.SaveSettingsPatch, string(body))
	if !extensionResponse.Success {
		t.Fatalf("extension patch failed: %s", extensionResponse.Msg)
	}
	var audit model.Changes
	if err := database.GetDB().Where("key = ? AND action = ?", "settings", "patch").Order("id DESC").First(&audit).Error; err != nil {
		t.Fatalf("load extension audit: %v", err)
	}
	auditText := string(audit.Obj)
	if strings.Contains(auditText, extension) || !strings.Contains(auditText, "sha256") || !strings.Contains(auditText, "bytes") {
		t.Fatalf("extension audit is not compact metadata: %s", auditText)
	}

	_, conflict := performSettingsPatchJSONPost(t, apiService.SaveSettingsPatch, `{"expectedRevision":2,"changes":{"subURI":"https://stale.example"}}`)
	if conflict.Success {
		t.Fatal("stale settings patch unexpectedly succeeded")
	}
	conflictObject, ok := conflict.Obj.(map[string]interface{})
	if !ok || conflictObject["code"] != "revision_conflict" || conflictObject["currentRevision"] != float64(3) {
		t.Fatalf("unexpected conflict response: %#v", conflict.Obj)
	}
}

func TestSettingsPatchRejectsUnknownInvalidAndOversizedValues(t *testing.T) {
	setupSettingsPatchAPITestDB(t)
	apiService := &ApiService{}

	tests := []struct {
		name string
		body string
	}{
		{name: "unknown key", body: `{"expectedRevision":1,"changes":{"secret":"x"}}`},
		{name: "invalid json", body: `{"expectedRevision":1,"changes":{"subJsonExt":"{"}}`},
		{name: "duplicate yaml key", body: `{"expectedRevision":1,"changes":{"subClashExt":"mixed-port: 7890\nmixed-port: 7891\n"}}`},
		{name: "JSON ruleset format mismatch", body: `{"expectedRevision":1,"changes":{"subJsonExt":"{\"rule_set\":[{\"tag\":\"x\",\"format\":\"source\",\"url\":\"https://example.com/x.srs\"}]}"}}`},
		{name: "Clash ruleset format mismatch", body: `{"expectedRevision":1,"changes":{"subClashExt":"rule-providers:\n  x:\n    type: http\n    behavior: domain\n    format: yaml\n    url: https://example.com/x.mrs\n"}}`},
		{name: "custom name type conflict", body: `{"expectedRevision":1,"changes":{"subJsonExt":"{\"_uiConfig\":{\"ruleSetSource\":\"karingx_github\",\"ruleRows\":[{\"kind\":\"custom\",\"name\":\"same\",\"customType\":\"domain\"},{\"kind\":\"custom\",\"name\":\"same\",\"customType\":\"domain_suffix\"}]}}"}}`},
		{name: "invalid subscription URI scheme", body: `{"expectedRevision":1,"changes":{"subURI":"ftp://example.com/sub"}}`},
		{name: "subscription URI query", body: `{"expectedRevision":1,"changes":{"subURI":"https://example.com/sub?token=x"}}`},
		{name: "subscription URI invalid port", body: `{"expectedRevision":1,"changes":{"subURI":"https://example.com:invalid/sub"}}`},
		{name: "invalid listener", body: `{"expectedRevision":1,"changes":{"subListen":"not-a-listen-address"}}`},
		{name: "unsafe panel path", body: `{"expectedRevision":1,"changes":{"webPath":"/api/:route/"}}`},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, response := performSettingsPatchJSONPost(t, apiService.SaveSettingsPatch, test.body)
			if response.Success {
				t.Fatalf("invalid patch unexpectedly succeeded: %#v", response)
			}
		})
	}

	oversized := `{"x":"` + strings.Repeat("a", service.SubscriptionExtensionMaxBytes) + `"}`
	body, _ := json.Marshal(map[string]interface{}{
		"expectedRevision": 1,
		"changes":          map[string]string{"subJsonExt": oversized},
	})
	_, response := performSettingsPatchJSONPost(t, apiService.SaveSettingsPatch, string(body))
	if response.Success || !strings.Contains(response.Msg, "超过") {
		t.Fatalf("oversized extension was not rejected: %#v", response)
	}
}

func TestSettingsPatchRequiresTrafficHistoryClearConfirmation(t *testing.T) {
	setupSettingsPatchAPITestDB(t)
	_, response := performSettingsPatchJSONPost(t, (&ApiService{}).SaveSettingsPatch, `{"expectedRevision":1,"changes":{"trafficAge":"0"}}`)
	if response.Success || !strings.Contains(response.Msg, "确认") {
		t.Fatalf("traffic history clear without confirmation was accepted: %#v", response)
	}
}

func TestCompactAndSubscriptionSettingsSnapshotsShareAtomicRevision(t *testing.T) {
	setupSettingsPatchAPITestDB(t)
	settingService := &service.SettingService{}
	if err := settingService.SaveSetting("subJsonExt", `{"latency_test_interval":"10m"}`); err != nil {
		t.Fatalf("save json extension: %v", err)
	}
	compact, err := settingService.GetSettingsSnapshot(false)
	if err != nil {
		t.Fatalf("load compact snapshot: %v", err)
	}
	if compact.ExtensionsIncluded || compact.Values["subJsonExt"] != "" || compact.Values["subClashExt"] != "" {
		t.Fatalf("compact snapshot exposed extensions: %#v", compact)
	}
	extension, err := settingService.GetSubscriptionSettingsSnapshot("json")
	if err != nil {
		t.Fatalf("load json extension snapshot: %v", err)
	}
	if extension.Revision != compact.Revision || extension.Value == "" || extension.Default == "" {
		t.Fatalf("extension snapshot is inconsistent: compact=%#v extension=%#v", compact, extension)
	}
}

func TestSystemOnlyPatchForceRevisionAdvancesCAS(t *testing.T) {
	setupSettingsPatchAPITestDB(t)
	result, err := (&service.SettingService{}).ApplySettingsPatch(service.SettingsPatchRequest{
		ExpectedRevision:  1,
		Changes:           map[string]string{},
		ForceRevision:     true,
		SystemTimeChanged: true,
	}, "tester")
	if err != nil {
		t.Fatalf("apply system-only revision: %v", err)
	}
	if result.Revision != 2 || !result.SystemTimeChanged {
		t.Fatalf("system-only patch did not advance revision: %#v", result)
	}
	if err := (&service.SettingService{}).CheckSettingsRevision(1); err == nil {
		t.Fatal("stale revision still passed after system-only patch")
	}
}

func TestSettingsSnapshotPairsValuesWithRevisionDuringConcurrentPatches(t *testing.T) {
	setupSettingsPatchAPITestDB(t)
	settingService := &service.SettingService{}
	done := make(chan struct{})
	writeErr := make(chan error, 1)
	const patches = 12
	go func() {
		defer close(done)
		revision := uint64(1)
		for index := 1; index <= patches; index++ {
			result, err := settingService.ApplySettingsPatch(service.SettingsPatchRequest{
				ExpectedRevision: revision,
				Changes: map[string]string{
					"subURI": fmt.Sprintf("https://snapshot.example/%d", index),
				},
			}, "writer")
			if err != nil {
				writeErr <- err
				return
			}
			revision = result.Revision
		}
	}()

	completed := false
	for !completed {
		snapshot, err := settingService.GetSettingsSnapshot(false)
		if err != nil {
			t.Fatalf("load concurrent snapshot: %v", err)
		}
		if snapshot.Revision > 1 {
			expected := fmt.Sprintf("https://snapshot.example/%d/", snapshot.Revision-1)
			if snapshot.Values["subURI"] != expected {
				t.Fatalf("torn snapshot: revision=%d URI=%q want %q", snapshot.Revision, snapshot.Values["subURI"], expected)
			}
		}
		select {
		case err := <-writeErr:
			t.Fatalf("writer patch failed: %v", err)
		case <-done:
			completed = true
		default:
		}
	}
	select {
	case err := <-writeErr:
		t.Fatalf("writer patch failed: %v", err)
	default:
	}
}

func TestSubscriptionRuntimeSettingsCacheInvalidatesAfterPatch(t *testing.T) {
	setupSettingsPatchAPITestDB(t)
	settingService := &service.SettingService{}
	before, err := settingService.GetSubUpdates()
	if err != nil || before != 12 {
		t.Fatalf("load cached subscription update interval: value=%d err=%v", before, err)
	}
	result, err := settingService.ApplySettingsPatch(service.SettingsPatchRequest{
		ExpectedRevision: 1,
		Changes:          map[string]string{"subUpdates": "24"},
	}, "tester")
	if err != nil || result.Revision != 2 {
		t.Fatalf("patch subscription update interval: result=%#v err=%v", result, err)
	}
	after, err := settingService.GetSubUpdates()
	if err != nil || after != 24 {
		t.Fatalf("subscription runtime cache was stale: value=%d err=%v", after, err)
	}
}

func TestSettingsPatchAllowsSameCustomNameWithSameMatchType(t *testing.T) {
	setupSettingsPatchAPITestDB(t)
	extension := `{"_uiConfig":{"ruleSetSource":"karingx_github","ruleRows":[{"kind":"custom","name":"same","customType":"domain","values":["a.example"]},{"kind":"custom","name":"same","customType":"domain","values":["b.example"]}]}}`
	body, _ := json.Marshal(map[string]interface{}{
		"expectedRevision": 1,
		"changes":          map[string]string{"subJsonExt": extension},
	})
	_, response := performSettingsPatchJSONPost(t, (&ApiService{}).SaveSettingsPatch, string(body))
	if !response.Success {
		t.Fatalf("same-name custom rows with matching types were rejected: %#v", response)
	}
}

func TestLegacySettingsSaveRequiresExpectedRevision(t *testing.T) {
	setupSettingsPatchAPITestDB(t)
	values := url.Values{
		"object": {"settings"},
		"action": {"set"},
		"data":   {`{"subURI":"https://example.com"}`},
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("POST", "/api/save", strings.NewReader(values.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	context, _ := gin.CreateTestContext(recorder)
	context.Request = request
	(&ApiService{}).Save(context, "tester")
	message := Msg{}
	if err := json.Unmarshal(recorder.Body.Bytes(), &message); err != nil {
		t.Fatalf("decode legacy response: %v", err)
	}
	if message.Success || !strings.Contains(message.Msg, "expectedRevision") {
		t.Fatalf("legacy save did not reject missing revision: %#v", message)
	}
}

func TestSettingsPatchValidatesProposedListenerValues(t *testing.T) {
	setupSettingsPatchAPITestDB(t)
	rule := model.PortForwardRule{
		Name:           "patch-conflict",
		Enabled:        true,
		Family:         "dual",
		Protocol:       "tcp",
		LocalPortMode:  "single",
		LocalPortSpec:  "32123",
		LocalPortStart: 32123,
		LocalPortCount: 1,
		LocalPortEnd:   32123,
		TargetIP:       "203.0.113.10",
		TargetPort:     443,
	}
	if err := database.GetDB().Create(&rule).Error; err != nil {
		t.Fatalf("create forwarding rule: %v", err)
	}

	_, response := performSettingsPatchJSONPost(t, (&ApiService{}).SaveSettingsPatch, `{"expectedRevision":1,"changes":{"webPort":"32123"}}`)
	if response.Success || !strings.Contains(response.Msg, "patch-conflict") {
		t.Fatalf("proposed listener conflict was not rejected: %#v", response)
	}
	snapshot, err := (&service.SettingService{}).GetSettingsSnapshot()
	if err != nil {
		t.Fatalf("load snapshot after rejected patch: %v", err)
	}
	if snapshot.Revision != 1 || snapshot.Values["webPort"] == "32123" {
		t.Fatalf("rejected listener patch was not rolled back: %#v", snapshot)
	}
}

func TestDirectEditableSettingWriteBumpsRevisionOnlyOnChange(t *testing.T) {
	setupSettingsPatchAPITestDB(t)
	settingService := &service.SettingService{}
	if err := settingService.SaveSetting("subURI", "https://direct.example/sub"); err != nil {
		t.Fatalf("save direct editable setting: %v", err)
	}
	first, err := settingService.GetSettingsSnapshot()
	if err != nil {
		t.Fatalf("load first snapshot: %v", err)
	}
	if first.Revision != 2 || first.Values["subURI"] != "https://direct.example/sub" {
		t.Fatalf("unexpected first direct setting snapshot: %#v", first)
	}
	if err := settingService.SaveSetting("subURI", "https://direct.example/sub"); err != nil {
		t.Fatalf("repeat direct editable setting: %v", err)
	}
	second, err := settingService.GetSettingsSnapshot()
	if err != nil {
		t.Fatalf("load second snapshot: %v", err)
	}
	if second.Revision != first.Revision {
		t.Fatalf("unchanged direct write bumped revision: first=%d second=%d", first.Revision, second.Revision)
	}
	if err := settingService.ResetSettings(); err != nil {
		t.Fatalf("reset settings: %v", err)
	}
	afterReset, err := settingService.GetSettingsSnapshot()
	if err != nil {
		t.Fatalf("load snapshot after reset: %v", err)
	}
	if afterReset.Revision != second.Revision+1 || afterReset.Values["subURI"] != "" {
		t.Fatalf("settings reset did not bump revision or restore defaults: %#v", afterReset)
	}
}

func TestSubscriptionRuleSetProbeRejectsPrivateAndOversizedBatch(t *testing.T) {
	apiService := &ApiService{}
	blockedTargets := []string{
		"http://127.0.0.1/rules.json",
		"http://100.64.0.1/rules.json",
		"http://169.254.169.254/rules.json",
		"http://[fc00::1]/rules.json",
		"http://[64:ff9b::7f00:1]/rules.json",
		"http://[fec0::1]/rules.json",
	}
	for _, target := range blockedTargets {
		body, _ := json.Marshal(map[string]interface{}{
			"items": []map[string]interface{}{{"id": target, "kind": "json", "scope": "domain", "url": target}},
		})
		_, privateResponse := performSettingsPatchJSONPost(t, func(c *gin.Context, _ string) {
			apiService.ProbeSubscriptionRuleSets(c)
		}, string(body))
		if !privateResponse.Success {
			t.Fatalf("probe request itself failed for %s: %s", target, privateResponse.Msg)
		}
		privateResults, ok := privateResponse.Obj.([]interface{})
		if !ok || len(privateResults) != 1 {
			t.Fatalf("unexpected private probe response for %s: %#v", target, privateResponse.Obj)
		}
		privateResult := privateResults[0].(map[string]interface{})
		if privateResult["valid"] != false || !strings.Contains(privateResult["error"].(string), "公网") {
			t.Fatalf("private target was not rejected for %s: %#v", target, privateResult)
		}
	}

	items := make([]map[string]interface{}, service.RuleSetProbeMaxBatch+1)
	for index := range items {
		items[index] = map[string]interface{}{"id": index, "kind": "json", "scope": "domain", "url": "https://example.com/rules.json"}
	}
	body, _ := json.Marshal(map[string]interface{}{"items": items})
	_, oversizedResponse := performSettingsPatchJSONPost(t, func(c *gin.Context, _ string) {
		apiService.ProbeSubscriptionRuleSets(c)
	}, string(body))
	if oversizedResponse.Success {
		t.Fatal("oversized probe batch unexpectedly succeeded")
	}
}

func performSettingsPatchJSONPost(t *testing.T, handler func(*gin.Context, string), body string) (*httptest.ResponseRecorder, Msg) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest("POST", "/api/settings-patch", bytes.NewBufferString(body))
	request.Header.Set("Content-Type", "application/json")
	context, _ := gin.CreateTestContext(recorder)
	context.Request = request
	handler(context, "tester")
	message := Msg{}
	if err := json.Unmarshal(recorder.Body.Bytes(), &message); err != nil {
		t.Fatalf("decode response %q: %v", recorder.Body.String(), err)
	}
	return recorder, message
}

func decodeSettingsPatchResult(t *testing.T, raw interface{}) service.SettingsPatchResult {
	t.Helper()
	encoded, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("marshal patch result: %v", err)
	}
	result := service.SettingsPatchResult{}
	if err := json.Unmarshal(encoded, &result); err != nil {
		t.Fatalf("decode patch result: %v", err)
	}
	return result
}

func setupSettingsPatchAPITestDB(t *testing.T) {
	t.Helper()
	if err := database.InitDB(filepath.Join(t.TempDir(), "settings-patch.db")); err != nil {
		t.Fatalf("init settings test database: %v", err)
	}
	sqlDB, _ := database.GetDB().DB()
	t.Cleanup(func() {
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
	})
	if _, err := (&service.SettingService{}).GetAllSetting(); err != nil {
		t.Fatalf("seed settings: %v", err)
	}
}
