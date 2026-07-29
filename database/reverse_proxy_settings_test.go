package database

import (
	"path/filepath"
	"testing"

	"github.com/alireza0/s-ui/database/model"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestInitDBCreatesReverseProxySettingsAndMigratesLegacyRuleLimits(t *testing.T) {
	previousDB := db
	path := filepath.Join(t.TempDir(), "legacy-reverse-proxy.db")

	legacyDB, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		t.Fatalf("open legacy database: %v", err)
	}
	legacySQL, err := legacyDB.DB()
	if err != nil {
		t.Fatalf("access legacy database: %v", err)
	}
	if err := legacyDB.AutoMigrate(&model.ReverseProxyRule{}); err != nil {
		_ = legacySQL.Close()
		t.Fatalf("migrate legacy reverse proxy rule: %v", err)
	}
	legacyRule := model.ReverseProxyRule{Name: "legacy"}
	if err := legacyDB.Create(&legacyRule).Error; err != nil {
		_ = legacySQL.Close()
		t.Fatalf("create legacy reverse proxy rule: %v", err)
	}
	if err := legacyDB.Model(&model.ReverseProxyRule{}).Where("id = ?", legacyRule.Id).Updates(map[string]interface{}{
		"max_concurrent_requests":    0,
		"dns_max_concurrent_queries": 0,
	}).Error; err != nil {
		_ = legacySQL.Close()
		t.Fatalf("prepare legacy zero limits: %v", err)
	}
	if err := legacySQL.Close(); err != nil {
		t.Fatalf("close legacy database: %v", err)
	}

	if err := InitDB(path); err != nil {
		t.Fatalf("initialize legacy database: %v", err)
	}
	current := GetDB()
	currentSQL, err := current.DB()
	if err != nil {
		t.Fatalf("access initialized database: %v", err)
	}
	t.Cleanup(func() {
		_ = currentSQL.Close()
		db = previousDB
	})

	var settings model.ReverseProxySettings
	if err := current.Where("id = ?", model.ReverseProxySettingsSingletonID).First(&settings).Error; err != nil {
		t.Fatalf("load initialized reverse proxy settings: %v", err)
	}
	want := model.DefaultReverseProxySettings()
	if settings.Id != want.Id ||
		settings.Revision != want.Revision ||
		settings.ListenerConnectionLimit != want.ListenerConnectionLimit ||
		settings.GlobalHTTPMaxConcurrent != want.GlobalHTTPMaxConcurrent ||
		settings.GlobalDNSMaxConcurrent != want.GlobalDNSMaxConcurrent ||
		settings.HTTP2MaxConcurrentStreams != want.HTTP2MaxConcurrentStreams ||
		settings.QUICMaxIncomingStreams != want.QUICMaxIncomingStreams ||
		settings.DefaultUpstreamMaxIdleConnections != want.DefaultUpstreamMaxIdleConnections ||
		settings.MemoryPoolBytes != want.MemoryPoolBytes ||
		settings.DefaultRuleMemoryLimitBytes != want.DefaultRuleMemoryLimitBytes ||
		settings.ResponseRewriteInputBytes != want.ResponseRewriteInputBytes ||
		settings.ResponseRewriteOutputBytes != want.ResponseRewriteOutputBytes ||
		settings.ResponseRewriteMaxConcurrent != want.ResponseRewriteMaxConcurrent {
		t.Fatalf("unexpected initialized reverse proxy settings: got=%#v want=%#v", settings, want)
	}

	var migrated model.ReverseProxyRule
	if err := current.Where("id = ?", legacyRule.Id).First(&migrated).Error; err != nil {
		t.Fatalf("load migrated reverse proxy rule: %v", err)
	}
	if migrated.MaxConcurrentRequests != model.ReverseProxyLegacyMaxConcurrentRequests ||
		migrated.DNSMaxConcurrentQueries != model.ReverseProxyLegacyDNSMaxConcurrentQueries {
		t.Fatalf("legacy zero limits were not migrated once: got request=%d dns=%d", migrated.MaxConcurrentRequests, migrated.DNSMaxConcurrentQueries)
	}
	if err := current.Model(&model.ReverseProxyRule{}).Where("id = ?", legacyRule.Id).Updates(map[string]interface{}{
		"max_concurrent_requests":    0,
		"dns_max_concurrent_queries": 0,
	}).Error; err != nil {
		t.Fatalf("set intentional unlimited limits: %v", err)
	}
	if err := ensureReverseProxySettingsSingleton(current); err != nil {
		t.Fatalf("re-check existing reverse proxy singleton: %v", err)
	}
	if err := current.Where("id = ?", legacyRule.Id).First(&migrated).Error; err != nil {
		t.Fatalf("reload intentionally unlimited reverse proxy rule: %v", err)
	}
	if migrated.MaxConcurrentRequests != 0 || migrated.DNSMaxConcurrentQueries != 0 {
		t.Fatalf("existing singleton must not remigrate intentional unlimited limits: request=%d dns=%d", migrated.MaxConcurrentRequests, migrated.DNSMaxConcurrentQueries)
	}
}
