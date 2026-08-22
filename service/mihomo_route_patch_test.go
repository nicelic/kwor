package service

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
)

func TestSaveMihomoRoutePatchRejectsStaleRevision(t *testing.T) {
	previousRevision := atomic.LoadInt64(&MihomoLastUpdate)
	t.Cleanup(func() { atomic.StoreInt64(&MihomoLastUpdate, previousRevision) })
	atomic.StoreInt64(&MihomoLastUpdate, 100)
	expectedRevision := CurrentMihomoConfigRevisionForPolling()
	markMihomoLastUpdate(0)

	_, err := (&ConfigService{}).SaveMihomoRoutePatch(MihomoRoutePatchRequest{
		ExpectedRevision: &expectedRevision,
		Route:            json.RawMessage(`{"rules":[],"rule_set":[]}`),
	}, "tester", "panel.example.com")
	var conflict *MihomoConfigRevisionConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("expected Mihomo route revision conflict, got %v", err)
	}
	if conflict.CurrentRevision != CurrentMihomoConfigRevisionForPolling() {
		t.Fatalf("unexpected current revision: %#v", conflict)
	}
}

func TestApplyMihomoRoutePatchKeepsUnrelatedConfig(t *testing.T) {
	current := json.RawMessage(`{
  "dns":{"nameserver":["udp://1.1.1.1"]},
  "ipv6":true,
  "tcp-concurrent":true,
  "sniffer":{"enable":true,"parse-pure-ip":true},
  "route":{"final":"DIRECT","rules":[]}
}`)
	updated, err := applyMihomoRoutePatch(current, MihomoRoutePatchRequest{
		Route: json.RawMessage(`{"final":"REJECT","rules":[],"rule_set":[]}`),
	})
	if err != nil {
		t.Fatalf("apply route patch: %v", err)
	}

	var document map[string]interface{}
	if err := json.Unmarshal(updated, &document); err != nil {
		t.Fatalf("decode route patch result: %v", err)
	}
	if document["ipv6"] != true || document["tcp-concurrent"] != true {
		t.Fatalf("route patch changed unrelated top-level settings: %#v", document)
	}
	dns, _ := document["dns"].(map[string]interface{})
	if dns == nil || len(dns["nameserver"].([]interface{})) != 1 {
		t.Fatalf("route patch removed DNS: %#v", document["dns"])
	}
	sniffer, _ := document["sniffer"].(map[string]interface{})
	if sniffer == nil || sniffer["enable"] != true || sniffer["parse-pure-ip"] != true {
		t.Fatalf("route patch removed untouched sniffer data: %#v", document["sniffer"])
	}
	route, _ := document["route"].(map[string]interface{})
	if route == nil || route["final"] != "REJECT" {
		t.Fatalf("route was not replaced: %#v", document["route"])
	}
}

func TestApplyMihomoRoutePatchCanClearSnifferWithoutTouchingDNS(t *testing.T) {
	current := json.RawMessage(`{
  "dns":{"nameserver":["udp://1.1.1.1"]},
  "sniffer":{"enable":true},
  "route":{"final":"DIRECT","rules":[]}
}`)
	updated, err := applyMihomoRoutePatch(current, MihomoRoutePatchRequest{
		Route:   json.RawMessage(`{"final":"DIRECT","rules":[],"rule_set":[]}`),
		Sniffer: json.RawMessage(`null`),
	})
	if err != nil {
		t.Fatalf("apply route patch with sniffer clear: %v", err)
	}
	var document map[string]interface{}
	if err := json.Unmarshal(updated, &document); err != nil {
		t.Fatalf("decode route patch result: %v", err)
	}
	if _, exists := document["sniffer"]; exists {
		t.Fatalf("explicit sniffer clear did not remove it: %#v", document["sniffer"])
	}
	if _, exists := document["dns"]; !exists {
		t.Fatalf("sniffer clear removed DNS: %s", updated)
	}
}

func TestSaveMihomoRoutePatchSkipsNoopAuditAndKeepsDNS(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "mihomo-route-patch.db")); err != nil {
		t.Fatalf("init database: %v", err)
	}
	sqlDB, _ := database.GetDB().DB()
	t.Cleanup(func() {
		if sqlDB != nil {
			_ = sqlDB.Close()
		}
	})
	if _, err := (&SettingService{}).GetAllSetting(); err != nil {
		t.Fatalf("seed settings: %v", err)
	}
	initial := `{"dns":{"nameserver":["udp://1.1.1.1"]},"sniffer":{"enable":true},"route":{"final":"DIRECT","rules":[]}}`
	if err := (&SettingService{}).SaveSetting(mihomoConfigSettingKey, initial); err != nil {
		t.Fatalf("save initial config: %v", err)
	}

	request := MihomoRoutePatchRequest{
		Route: json.RawMessage(`{"final":"DIRECT","rules":[],"rule_set":[],"no_resolve":false}`),
	}
	result, err := (&ConfigService{}).SaveMihomoRoutePatch(request, "tester", "panel.example.com")
	if err != nil || !result.Changed {
		t.Fatalf("save route patch: result=%#v err=%v", result, err)
	}
	var setting model.Setting
	if err := database.GetDB().Where("key = ?", mihomoConfigSettingKey).First(&setting).Error; err != nil {
		t.Fatalf("load config: %v", err)
	}
	if !strings.Contains(setting.Value, `udp://1.1.1.1`) || !strings.Contains(setting.Value, `"sniffer"`) || !strings.Contains(setting.Value, `"no_resolve": false`) {
		t.Fatalf("route patch overwrote unrelated config: %s", setting.Value)
	}
	var before int64
	if err := database.GetDB().Model(&model.Changes{}).Where("key = ? AND action = ?", mihomoConfigSettingKey, "route-patch").Count(&before).Error; err != nil {
		t.Fatalf("count route patch audit: %v", err)
	}
	result, err = (&ConfigService{}).SaveMihomoRoutePatch(request, "tester", "panel.example.com")
	if err != nil || result.Changed {
		t.Fatalf("no-op route patch changed state: result=%#v err=%v", result, err)
	}
	var after int64
	if err := database.GetDB().Model(&model.Changes{}).Where("key = ? AND action = ?", mihomoConfigSettingKey, "route-patch").Count(&after).Error; err != nil {
		t.Fatalf("count route patch audit after no-op: %v", err)
	}
	if after != before {
		t.Fatalf("no-op route patch wrote audit: before=%d after=%d", before, after)
	}
	if err := ManagedRuntimeDeleteFile(GetMihomoConfigPath()); err != nil {
		t.Fatalf("remove Mihomo runtime config before retry: %v", err)
	}
	if exists, err := ManagedRuntimeFileExists(GetMihomoConfigPath()); err != nil || exists {
		t.Fatalf("runtime config should be absent before retry: exists=%v err=%v", exists, err)
	}

	retryRequest := request
	retryRequest.RetryRuntime = true
	result, err = (&ConfigService{}).SaveMihomoRoutePatch(retryRequest, "tester", "panel.example.com")
	if err != nil || result.Changed {
		t.Fatalf("runtime retry should regenerate without another DB update: result=%#v err=%v", result, err)
	}
	if exists, err := ManagedRuntimeFileExists(GetMihomoConfigPath()); err != nil || !exists {
		t.Fatalf("runtime retry did not recreate server config: exists=%v err=%v", exists, err)
	}
	var afterRetry int64
	if err := database.GetDB().Model(&model.Changes{}).Where("key = ? AND action = ?", mihomoConfigSettingKey, "route-patch").Count(&afterRetry).Error; err != nil {
		t.Fatalf("count route patch audit after runtime retry: %v", err)
	}
	if afterRetry != before {
		t.Fatalf("runtime retry wrote audit: before=%d after=%d", before, afterRetry)
	}
}
