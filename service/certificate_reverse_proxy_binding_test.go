package service

import (
	"strings"
	"testing"

	"github.com/alireza0/s-ui/database/model"
	"gorm.io/gorm"
)

func TestCertificateDeleteBlockedByReverseProxyCertificateList(t *testing.T) {
	db := setupMihomoSyncTestDB(t, "cert-reverse-proxy-list-usage.db")
	record := upsertTestCertificateRecord(t, "reverse-proxy-list.example.com")

	rule := &model.ReverseProxyRule{
		Name:                  "rp-listener-a",
		Enabled:               true,
		ListenProtocol:        "https",
		CertificateRecordList: encodeReverseProxyUintList([]uint{record.Id}),
	}
	if err := db.Create(rule).Error; err != nil {
		t.Fatalf("create reverse proxy rule failed: %v", err)
	}
	if err := RefreshAllCertificateBindingUsageFlags(); err != nil {
		t.Fatalf("refresh certificate binding flags failed: %v", err)
	}

	views, err := certificateInventory.List()
	if err != nil {
		t.Fatalf("list certificates failed: %v", err)
	}
	view := findCertificateRecordView(t, views, record.Id)
	if !view.DeleteBlocked || !strings.Contains(view.DeleteBlockedReason, "反向代理") {
		t.Fatalf("expected reverse proxy usage to block deletion, got %#v", view)
	}
	if !strings.Contains(view.UsageLabel, "反向代理使用中") || !strings.Contains(view.UsageLabel, "rp-listener-a") {
		t.Fatalf("expected usage label to include reverse proxy rule name, got %q", view.UsageLabel)
	}
	if !strings.Contains(view.Remark, "反向代理使用中") {
		t.Fatalf("expected remark to include reverse proxy usage marker, got %q", view.Remark)
	}

	if _, err := (&AcmeService{}).Delete(AcmeDeletePayload{ID: record.Id}); err == nil || !strings.Contains(err.Error(), "反向代理") {
		t.Fatalf("expected reverse proxy delete rejection, got %v", err)
	}
	if err := db.Where("id = ?", record.Id).First(&model.CertificateRecord{}).Error; err != nil {
		t.Fatalf("certificate must remain after rejected deletion: %v", err)
	}
	if err := db.Where("id = ?", rule.Id).First(rule).Error; err != nil {
		t.Fatalf("reload reverse proxy rule failed: %v", err)
	}
	if len(reverseProxyRuleStoredCertificateIDs(rule)) != 1 || reverseProxyRuleStoredCertificateIDs(rule)[0] != record.Id {
		t.Fatalf("reverse proxy binding must remain unchanged: %#v", rule)
	}
}

func TestCertificateListReportsDisabledReverseProxyLegacyCertificateField(t *testing.T) {
	db := setupMihomoSyncTestDB(t, "cert-reverse-proxy-legacy-usage.db")
	record := upsertTestCertificateRecord(t, "reverse-proxy-legacy.example.com")

	rule := &model.ReverseProxyRule{
		Name:                "rp-disabled-legacy",
		Enabled:             false,
		ListenProtocol:      "https",
		CertificateRecordID: record.Id,
	}
	if err := db.Create(rule).Error; err != nil {
		t.Fatalf("create reverse proxy legacy rule failed: %v", err)
	}
	if err := RefreshAllCertificateBindingUsageFlags(); err != nil {
		t.Fatalf("refresh certificate binding flags failed: %v", err)
	}

	views, err := certificateInventory.List()
	if err != nil {
		t.Fatalf("list certificates failed: %v", err)
	}
	view := findCertificateRecordView(t, views, record.Id)
	if !view.DeleteBlocked || !strings.Contains(view.DeleteBlockedReason, "反向代理") {
		t.Fatalf("disabled reverse proxy must still block deletion, got %#v", view)
	}
	if !strings.Contains(view.UsageLabel, "rp-disabled-legacy") {
		t.Fatalf("expected usage label to include disabled reverse proxy rule, got %q", view.UsageLabel)
	}
}

func TestCertificateReverseProxyUsageLabelTruncatesAfterThreeRules(t *testing.T) {
	db := setupMihomoSyncTestDB(t, "cert-reverse-proxy-usage-truncate.db")
	record := upsertTestCertificateRecord(t, "reverse-proxy-truncate.example.com")

	names := []string{"rp-1", "rp-2", "rp-3", "rp-4"}
	for _, name := range names {
		rule := &model.ReverseProxyRule{
			Name:                  name,
			Enabled:               true,
			ListenProtocol:        "https",
			CertificateRecordList: encodeReverseProxyUintList([]uint{record.Id}),
		}
		if err := db.Create(rule).Error; err != nil {
			t.Fatalf("create reverse proxy rule %s failed: %v", name, err)
		}
	}

	views, err := certificateInventory.List()
	if err != nil {
		t.Fatalf("list certificates failed: %v", err)
	}
	view := findCertificateRecordView(t, views, record.Id)
	if !strings.Contains(view.UsageLabel, "rp-1, rp-2, rp-3 等 4 项") {
		t.Fatalf("expected truncated reverse proxy usage label, got %q", view.UsageLabel)
	}
}

func TestCertificateDeleteAllowedAfterReverseProxyBindingCleared(t *testing.T) {
	db := setupMihomoSyncTestDB(t, "cert-reverse-proxy-usage-cleared.db")
	record := upsertTestCertificateRecord(t, "reverse-proxy-cleared.example.com")

	rule := &model.ReverseProxyRule{
		Name:                  "rp-cleared",
		Enabled:               true,
		ListenProtocol:        "https",
		CertificateRecordList: encodeReverseProxyUintList([]uint{record.Id}),
	}
	if err := db.Create(rule).Error; err != nil {
		t.Fatalf("create reverse proxy rule failed: %v", err)
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.ReverseProxyRule{}).Where("id = ?", rule.Id).Updates(map[string]interface{}{
			"certificate_record_id":   0,
			"certificate_record_list": "",
		}).Error; err != nil {
			return err
		}
		return RefreshCertificateBindingUsageFlagsTx(tx, []uint{record.Id})
	}); err != nil {
		t.Fatalf("clear reverse proxy binding failed: %v", err)
	}

	views, err := certificateInventory.List()
	if err != nil {
		t.Fatalf("list certificates failed: %v", err)
	}
	view := findCertificateRecordView(t, views, record.Id)
	if view.DeleteBlocked {
		t.Fatalf("expected delete block to clear after reverse proxy binding removal, got %#v", view)
	}
	if strings.Contains(view.UsageLabel, "反向代理使用中") {
		t.Fatalf("expected reverse proxy usage label to clear, got %q", view.UsageLabel)
	}

	if _, err := (&AcmeService{}).Delete(AcmeDeletePayload{ID: record.Id}); err != nil {
		t.Fatalf("delete after reverse proxy binding cleared failed: %v", err)
	}
}
