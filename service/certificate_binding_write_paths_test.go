package service

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"gorm.io/gorm"
)

func TestTLSWritePathsRefreshCertificateBindingFlags(t *testing.T) {
	db := setupMihomoSyncTestDB(t, "certificate-binding-tls-write-paths.db")
	first := upsertTestCertificateRecord(t, "tls-write-first.example.com")
	second := upsertTestCertificateRecord(t, "tls-write-second.example.com")

	defaultService := &TlsService{}
	defaultTLS := model.Tls{
		Name:                "binding-default-tls",
		CertificateRecordID: first.Id,
		Server:              json.RawMessage(`{}`),
		Client:              json.RawMessage(`{}`),
	}
	if err := saveDefaultTLSForBindingTest(db, defaultService, "new", defaultTLS); err != nil {
		t.Fatalf("create sing-box TLS failed: %v", err)
	}
	if err := assertCertificateBindingFlags(t, db, first.Id, false, true, false); err != nil {
		t.Fatal(err)
	}

	if err := db.Where("name = ?", defaultTLS.Name).First(&defaultTLS).Error; err != nil {
		t.Fatalf("load created sing-box TLS failed: %v", err)
	}
	defaultTLS.CertificateRecordID = second.Id
	if err := saveDefaultTLSForBindingTest(db, defaultService, "edit", defaultTLS); err != nil {
		t.Fatalf("replace sing-box TLS certificate failed: %v", err)
	}
	if err := assertCertificateBindingFlags(t, db, first.Id, false, false, false); err != nil {
		t.Fatal(err)
	}
	if err := assertCertificateBindingFlags(t, db, second.Id, false, true, false); err != nil {
		t.Fatal(err)
	}

	if err := deleteDefaultTLSForBindingTest(db, defaultService, defaultTLS.Id); err != nil {
		t.Fatalf("delete sing-box TLS failed: %v", err)
	}
	if err := assertCertificateBindingFlags(t, db, second.Id, false, false, false); err != nil {
		t.Fatal(err)
	}

	mihomoService := &MihomoTlsService{}
	mihomoTLS := model.MihomoTls{
		Name:                "binding-mihomo-tls",
		Mode:                model.MihomoTlsModeTLS,
		CertificateRecordID: first.Id,
		Server:              json.RawMessage(`{}`),
		Client:              json.RawMessage(`{}`),
	}
	if err := saveMihomoTLSForBindingTest(db, mihomoService, "new", mihomoTLS); err != nil {
		t.Fatalf("create Mihomo TLS failed: %v", err)
	}
	if err := assertCertificateBindingFlags(t, db, first.Id, false, false, true); err != nil {
		t.Fatal(err)
	}

	if err := db.Where("name = ?", mihomoTLS.Name).First(&mihomoTLS).Error; err != nil {
		t.Fatalf("load created Mihomo TLS failed: %v", err)
	}
	mihomoTLS.CertificateRecordID = second.Id
	if err := saveMihomoTLSForBindingTest(db, mihomoService, "edit", mihomoTLS); err != nil {
		t.Fatalf("replace Mihomo TLS certificate failed: %v", err)
	}
	if err := assertCertificateBindingFlags(t, db, first.Id, false, false, false); err != nil {
		t.Fatal(err)
	}
	if err := assertCertificateBindingFlags(t, db, second.Id, false, false, true); err != nil {
		t.Fatal(err)
	}

	if err := deleteMihomoTLSForBindingTest(db, mihomoService, mihomoTLS.Id); err != nil {
		t.Fatalf("delete Mihomo TLS failed: %v", err)
	}
	if err := assertCertificateBindingFlags(t, db, second.Id, false, false, false); err != nil {
		t.Fatal(err)
	}
}

func TestReverseProxyWritePathsRefreshCertificateBindingFlags(t *testing.T) {
	openReverseProxyTestDB(t)
	db := database.GetDB()
	firstID := createReverseProxyTestCertificateRecord(t, "binding-proxy-first.example.com")
	secondID := createReverseProxyTestCertificateRecord(t, "binding-proxy-second.example.com")
	service := &ReverseProxyService{}

	payload := func(id uint) ReverseProxyRulePayload {
		return ReverseProxyRulePayload{
			Name:                "binding-reverse-proxy",
			Enabled:             false,
			ListenProtocol:      reverseProxyProtocolHTTPS,
			ListenPort:          reserveReverseProxyTestPort(t),
			Hosts:               map[uint]string{firstID: "binding-proxy-first.example.com", secondID: "binding-proxy-second.example.com"}[id],
			TargetProtocol:      reverseProxyProtocolHTTP,
			TargetAddresses:     "127.0.0.1",
			TargetPort:          reserveReverseProxyTestPort(t),
			CertificateRecordID: id,
			IPStrategy:          reverseProxyIPStrategyPreferIPv4,
		}
	}
	if err := service.UpsertRule(payload(firstID)); err != nil {
		t.Fatalf("create reverse proxy rule failed: %v", err)
	}
	if err := assertCertificateBindingFlags(t, db, firstID, true, false, false); err != nil {
		t.Fatal(err)
	}

	rule := &model.ReverseProxyRule{}
	if err := db.Where("name = ?", "binding-reverse-proxy").First(rule).Error; err != nil {
		t.Fatalf("load created reverse proxy rule failed: %v", err)
	}
	updated := payload(secondID)
	updated.ID = rule.Id
	if err := service.UpsertRule(updated); err != nil {
		t.Fatalf("replace reverse proxy certificate failed: %v", err)
	}
	if err := assertCertificateBindingFlags(t, db, firstID, false, false, false); err != nil {
		t.Fatal(err)
	}
	if err := assertCertificateBindingFlags(t, db, secondID, true, false, false); err != nil {
		t.Fatal(err)
	}

	if err := service.DeleteRule(rule.Id); err != nil {
		t.Fatalf("delete reverse proxy rule failed: %v", err)
	}
	if err := assertCertificateBindingFlags(t, db, secondID, false, false, false); err != nil {
		t.Fatal(err)
	}
}

func saveDefaultTLSForBindingTest(db *gorm.DB, service *TlsService, action string, tls model.Tls) error {
	raw, err := json.Marshal(tls)
	if err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		_, err := service.Save(tx, action, raw, "")
		return err
	})
}

func deleteDefaultTLSForBindingTest(db *gorm.DB, service *TlsService, id uint) error {
	raw, err := json.Marshal(id)
	if err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		_, err := service.Save(tx, "del", raw, "")
		return err
	})
}

func saveMihomoTLSForBindingTest(db *gorm.DB, service *MihomoTlsService, action string, tls model.MihomoTls) error {
	raw, err := json.Marshal(tls)
	if err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		return service.Save(tx, action, raw, "")
	})
}

func deleteMihomoTLSForBindingTest(db *gorm.DB, service *MihomoTlsService, id uint) error {
	raw, err := json.Marshal(id)
	if err != nil {
		return err
	}
	return db.Transaction(func(tx *gorm.DB) error {
		return service.Save(tx, "del", raw, "")
	})
}

func assertCertificateBindingFlags(t *testing.T, db *gorm.DB, id uint, reverseProxy bool, singboxTLS bool, mihomoTLS bool) error {
	t.Helper()
	row := &model.CertificateRecord{}
	if err := db.Where("id = ?", id).First(row).Error; err != nil {
		return err
	}
	if row.BoundByReverseProxy != reverseProxy || row.BoundBySingboxTLS != singboxTLS || row.BoundByMihomoTLS != mihomoTLS {
		return fmt.Errorf("certificate %d flags = reverse_proxy:%t singbox_tls:%t mihomo_tls:%t, want reverse_proxy:%t singbox_tls:%t mihomo_tls:%t", id, row.BoundByReverseProxy, row.BoundBySingboxTLS, row.BoundByMihomoTLS, reverseProxy, singboxTLS, mihomoTLS)
	}
	return nil
}
