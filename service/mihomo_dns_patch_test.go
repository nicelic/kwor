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

func TestSaveMihomoDNSPatchRejectsStaleRevision(t *testing.T) {
	previousRevision := atomic.LoadInt64(&MihomoLastUpdate)
	t.Cleanup(func() { atomic.StoreInt64(&MihomoLastUpdate, previousRevision) })
	atomic.StoreInt64(&MihomoLastUpdate, 200)
	expectedRevision := CurrentMihomoConfigRevisionForPolling()
	markMihomoLastUpdate(0)

	_, err := (&ConfigService{}).SaveMihomoDNSPatch(MihomoDNSPatchRequest{
		ExpectedRevision: &expectedRevision,
		DNS:              json.RawMessage(`{"nameserver":["udp://1.1.1.1"]}`),
	}, "tester")
	var conflict *MihomoConfigRevisionConflictError
	if !errors.As(err, &conflict) {
		t.Fatalf("expected Mihomo DNS revision conflict, got %v", err)
	}
	if conflict.CurrentRevision != CurrentMihomoConfigRevisionForPolling() {
		t.Fatalf("unexpected current revision: %#v", conflict)
	}
}

func TestApplyMihomoDNSPatchKeepsUnrelatedConfig(t *testing.T) {
	current := json.RawMessage(`{
  "route":{"rules":[{"action":"route","outbound":"direct"}],"no_resolve":false},
  "sniffer":{"enable":true},
  "dns":{"nameserver":["udp://1.1.1.1"]}
}`)
	tcpConcurrent := true
	updated, err := applyMihomoDNSPatch(current, MihomoDNSPatchRequest{
		TCPConcurrent: &tcpConcurrent,
		DNS:           json.RawMessage(`{"nameserver":["tls://8.8.8.8"]}`),
	})
	if err != nil {
		t.Fatalf("apply DNS patch: %v", err)
	}
	var document map[string]interface{}
	if err := json.Unmarshal(updated, &document); err != nil {
		t.Fatalf("decode patch result: %v", err)
	}
	if _, ok := document["route"]; !ok {
		t.Fatalf("route was unexpectedly removed: %s", updated)
	}
	if _, ok := document["sniffer"]; !ok {
		t.Fatalf("sniffer was unexpectedly removed: %s", updated)
	}
	dns, _ := document["dns"].(map[string]interface{})
	if dns == nil || len(dns["nameserver"].([]interface{})) != 1 {
		t.Fatalf("DNS was not replaced as expected: %#v", document["dns"])
	}
}

func TestApplyMihomoDNSPatchRemovesUnselectedOptionalSwitches(t *testing.T) {
	current := json.RawMessage(`{
  "ipv6":true,
  "tcp-concurrent":false,
  "dns":{"nameserver":["udp://1.1.1.1"],"ipv6":false,"prefer-h3":true}
}`)
	updated, err := applyMihomoDNSPatch(current, MihomoDNSPatchRequest{
		DNS: json.RawMessage(`{"nameserver":["udp://8.8.8.8"]}`),
	})
	if err != nil {
		t.Fatalf("apply DNS patch: %v", err)
	}

	var document map[string]interface{}
	if err := json.Unmarshal(updated, &document); err != nil {
		t.Fatalf("decode patch result: %v", err)
	}
	if _, exists := document["tcp-concurrent"]; exists {
		t.Fatalf("unselected tcp-concurrent was retained: %s", updated)
	}
	if _, exists := document["ipv6"]; exists {
		t.Fatalf("unselected top-level ipv6 was retained: %s", updated)
	}
	dns, _ := document["dns"].(map[string]interface{})
	if dns == nil {
		t.Fatalf("DNS was unexpectedly removed: %s", updated)
	}
	if _, exists := dns["ipv6"]; exists {
		t.Fatalf("unselected dns.ipv6 was retained: %#v", dns)
	}
	if _, exists := dns["prefer-h3"]; exists {
		t.Fatalf("unselected dns.prefer-h3 was retained: %#v", dns)
	}
}

func TestSaveMihomoDNSPatchSkipsNoopAuditAndKeepsRoute(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "mihomo-dns-patch.db")); err != nil {
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
	initial := `{"route":{"final":"DIRECT","rules":[{"action":"route","outbound":"direct"}]},"dns":{"nameserver":["udp://1.1.1.1"]}}`
	if err := (&SettingService{}).SaveSetting(mihomoConfigSettingKey, initial); err != nil {
		t.Fatalf("save initial config: %v", err)
	}

	tcpConcurrent := false
	request := MihomoDNSPatchRequest{
		TCPConcurrent: &tcpConcurrent,
		DNS:           json.RawMessage(`{"nameserver":["udp://8.8.8.8"]}`),
	}
	result, err := (&ConfigService{}).SaveMihomoDNSPatch(request, "tester")
	if err != nil || !result.Changed {
		t.Fatalf("save DNS patch: result=%#v err=%v", result, err)
	}
	var setting model.Setting
	if err := database.GetDB().Where("key = ?", mihomoConfigSettingKey).First(&setting).Error; err != nil {
		t.Fatalf("load config: %v", err)
	}
	if !strings.Contains(setting.Value, `"route"`) || !strings.Contains(setting.Value, `udp://8.8.8.8`) {
		t.Fatalf("DNS patch overwrote unrelated config: %s", setting.Value)
	}
	var before int64
	if err := database.GetDB().Model(&model.Changes{}).Where("key = ? AND action = ?", mihomoConfigSettingKey, "dns-patch").Count(&before).Error; err != nil {
		t.Fatalf("count audit: %v", err)
	}
	result, err = (&ConfigService{}).SaveMihomoDNSPatch(request, "tester")
	if err != nil || result.Changed {
		t.Fatalf("no-op patch changed state: result=%#v err=%v", result, err)
	}
	var after int64
	if err := database.GetDB().Model(&model.Changes{}).Where("key = ? AND action = ?", mihomoConfigSettingKey, "dns-patch").Count(&after).Error; err != nil {
		t.Fatalf("count audit after no-op: %v", err)
	}
	if after != before {
		t.Fatalf("no-op patch wrote audit: before=%d after=%d", before, after)
	}
	if err := ManagedRuntimeDeleteFile(GetMihomoConfigPath()); err != nil {
		t.Fatalf("remove Mihomo runtime config before retry: %v", err)
	}
	if exists, err := ManagedRuntimeFileExists(GetMihomoConfigPath()); err != nil || exists {
		t.Fatalf("runtime config should be absent before retry: exists=%v err=%v", exists, err)
	}

	retryRequest := request
	retryRequest.RetryRuntime = true
	result, err = (&ConfigService{}).SaveMihomoDNSPatch(retryRequest, "tester")
	if err != nil || result.Changed {
		t.Fatalf("runtime retry should regenerate without another DB update: result=%#v err=%v", result, err)
	}
	if exists, err := ManagedRuntimeFileExists(GetMihomoConfigPath()); err != nil || !exists {
		t.Fatalf("runtime retry did not recreate server config: exists=%v err=%v", exists, err)
	}
	var afterRetry int64
	if err := database.GetDB().Model(&model.Changes{}).Where("key = ? AND action = ?", mihomoConfigSettingKey, "dns-patch").Count(&afterRetry).Error; err != nil {
		t.Fatalf("count DNS patch audit after runtime retry: %v", err)
	}
	if afterRetry != before {
		t.Fatalf("runtime retry wrote audit: before=%d after=%d", before, afterRetry)
	}
}

func TestSaveMihomoDNSPatchRejectsInvalidRenderedConfigBeforeCommit(t *testing.T) {
	if err := database.InitDB(filepath.Join(t.TempDir(), "mihomo-dns-invalid-runtime.db")); err != nil {
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
	initial := `{"route":{"final":"DIRECT","rules":[{"action":"route","outbound":"missing-target"}]},"dns":{"nameserver":["udp://1.1.1.1"]}}`
	if err := (&SettingService{}).SaveSetting(mihomoConfigSettingKey, initial); err != nil {
		t.Fatalf("save initial config: %v", err)
	}

	tcpConcurrent := true
	_, err := (&ConfigService{}).SaveMihomoDNSPatch(MihomoDNSPatchRequest{
		TCPConcurrent: &tcpConcurrent,
		DNS:           json.RawMessage(`{"nameserver":["udp://8.8.8.8"]}`),
	}, "tester")
	if err == nil || !strings.Contains(err.Error(), "invalid mihomo runtime config") {
		t.Fatalf("expected runtime config validation failure, got %v", err)
	}

	var setting model.Setting
	if err := database.GetDB().Where("key = ?", mihomoConfigSettingKey).First(&setting).Error; err != nil {
		t.Fatalf("load config after rejected patch: %v", err)
	}
	if setting.Value != initial {
		t.Fatalf("invalid DNS patch was committed: got %s, want %s", setting.Value, initial)
	}
	var audits int64
	if err := database.GetDB().Model(&model.Changes{}).Where("key = ? AND action = ?", mihomoConfigSettingKey, "dns-patch").Count(&audits).Error; err != nil {
		t.Fatalf("count audit after rejected patch: %v", err)
	}
	if audits != 0 {
		t.Fatalf("rejected DNS patch wrote audit records: %d", audits)
	}
}

func TestSaveMihomoDNSPatchRejectsWhileSubscriptionImportIsRunning(t *testing.T) {
	if !mihomoOutboundSubscriptionImportMu.TryLock() {
		t.Fatal("expected test subscription lock to be available")
	}
	t.Cleanup(mihomoOutboundSubscriptionImportMu.Unlock)

	tcpConcurrent := false
	_, err := (&ConfigService{}).SaveMihomoDNSPatch(MihomoDNSPatchRequest{
		TCPConcurrent: &tcpConcurrent,
		DNS:           json.RawMessage(`{"nameserver":["udp://8.8.8.8"]}`),
	}, "tester")
	if !errors.Is(err, ErrMihomoSubscriptionImportBusy) {
		t.Fatalf("expected subscription import busy error, got %v", err)
	}
}
