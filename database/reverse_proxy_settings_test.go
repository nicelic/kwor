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
	if want.MemoryPoolBytes != 32*1024*1024*1024 ||
		want.DefaultRuleMemoryLimitBytes != 1024*1024*1024 ||
		want.ResponseRewriteInputBytes != 128*1024*1024 ||
		want.ResponseRewriteOutputBytes != 256*1024*1024 ||
		want.ResponseRewriteMaxConcurrent != 512 {
		t.Fatalf("unexpected reverse proxy resource defaults: %#v", want)
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

func TestEnsureReverseProxySettingsSingletonMigratesUntouchedResourceDefaults(t *testing.T) {
	legacyDB, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "legacy-resource-defaults.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open legacy resource database: %v", err)
	}
	legacySQL, err := legacyDB.DB()
	if err != nil {
		t.Fatalf("access legacy resource database: %v", err)
	}
	t.Cleanup(func() { _ = legacySQL.Close() })
	if err := legacyDB.AutoMigrate(&model.ReverseProxyRule{}, &model.ReverseProxySettings{}); err != nil {
		t.Fatalf("migrate legacy resource database: %v", err)
	}
	legacy := model.DefaultReverseProxySettings()
	legacy.Revision = 7
	legacy.ListenerConnectionLimit = 1234
	legacy.MemoryPoolBytes = model.ReverseProxyLegacyMemoryPoolBytes
	legacy.DefaultRuleMemoryLimitBytes = model.ReverseProxyLegacyDefaultRuleMemoryLimitBytes
	legacy.ResponseRewriteInputBytes = model.ReverseProxyLegacyResponseRewriteInputBytes
	legacy.ResponseRewriteOutputBytes = model.ReverseProxyLegacyResponseRewriteOutputBytes
	legacy.ResponseRewriteMaxConcurrent = model.ReverseProxyLegacyResponseRewriteMaxConcurrent
	if err := legacyDB.Create(&legacy).Error; err != nil {
		t.Fatalf("create legacy resource settings: %v", err)
	}

	if err := ensureReverseProxySettingsSingleton(legacyDB); err != nil {
		t.Fatalf("migrate legacy resource defaults: %v", err)
	}
	var migrated model.ReverseProxySettings
	if err := legacyDB.First(&migrated, model.ReverseProxySettingsSingletonID).Error; err != nil {
		t.Fatalf("load migrated resource settings: %v", err)
	}
	want := model.DefaultReverseProxySettings()
	if migrated.Revision != 8 || migrated.ListenerConnectionLimit != legacy.ListenerConnectionLimit ||
		migrated.MemoryPoolBytes != want.MemoryPoolBytes ||
		migrated.DefaultRuleMemoryLimitBytes != want.DefaultRuleMemoryLimitBytes ||
		migrated.ResponseRewriteInputBytes != want.ResponseRewriteInputBytes ||
		migrated.ResponseRewriteOutputBytes != want.ResponseRewriteOutputBytes ||
		migrated.ResponseRewriteMaxConcurrent != want.ResponseRewriteMaxConcurrent {
		t.Fatalf("unexpected migrated resource settings: got=%#v want defaults=%#v", migrated, want)
	}
	if err := ensureReverseProxySettingsSingleton(legacyDB); err != nil {
		t.Fatalf("repeat resource default migration: %v", err)
	}
	var repeated model.ReverseProxySettings
	if err := legacyDB.First(&repeated, model.ReverseProxySettingsSingletonID).Error; err != nil {
		t.Fatalf("reload migrated resource settings: %v", err)
	}
	if repeated.Revision != migrated.Revision {
		t.Fatalf("resource default migration is not idempotent: first=%d second=%d", migrated.Revision, repeated.Revision)
	}
}

func TestEnsureReverseProxySettingsSingletonPreservesCustomizedResourceDefaults(t *testing.T) {
	customDB, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "custom-resource-defaults.db")), &gorm.Config{})
	if err != nil {
		t.Fatalf("open customized resource database: %v", err)
	}
	customSQL, err := customDB.DB()
	if err != nil {
		t.Fatalf("access customized resource database: %v", err)
	}
	t.Cleanup(func() { _ = customSQL.Close() })
	if err := customDB.AutoMigrate(&model.ReverseProxyRule{}, &model.ReverseProxySettings{}); err != nil {
		t.Fatalf("migrate customized resource database: %v", err)
	}
	custom := model.DefaultReverseProxySettings()
	custom.Revision = 9
	custom.MemoryPoolBytes = model.ReverseProxyLegacyMemoryPoolBytes
	custom.DefaultRuleMemoryLimitBytes = model.ReverseProxyLegacyDefaultRuleMemoryLimitBytes
	custom.ResponseRewriteInputBytes = model.ReverseProxyLegacyResponseRewriteInputBytes
	custom.ResponseRewriteOutputBytes = model.ReverseProxyLegacyResponseRewriteOutputBytes
	custom.ResponseRewriteMaxConcurrent = model.ReverseProxyLegacyResponseRewriteMaxConcurrent + 1
	if err := customDB.Create(&custom).Error; err != nil {
		t.Fatalf("create customized resource settings: %v", err)
	}

	if err := ensureReverseProxySettingsSingleton(customDB); err != nil {
		t.Fatalf("check customized resource defaults: %v", err)
	}
	var preserved model.ReverseProxySettings
	if err := customDB.First(&preserved, model.ReverseProxySettingsSingletonID).Error; err != nil {
		t.Fatalf("load customized resource settings: %v", err)
	}
	if preserved.Id != custom.Id ||
		preserved.Revision != custom.Revision ||
		preserved.ListenerConnectionLimit != custom.ListenerConnectionLimit ||
		preserved.GlobalHTTPMaxConcurrent != custom.GlobalHTTPMaxConcurrent ||
		preserved.GlobalDNSMaxConcurrent != custom.GlobalDNSMaxConcurrent ||
		preserved.HTTP2MaxConcurrentStreams != custom.HTTP2MaxConcurrentStreams ||
		preserved.QUICMaxIncomingStreams != custom.QUICMaxIncomingStreams ||
		preserved.DefaultUpstreamMaxIdleConnections != custom.DefaultUpstreamMaxIdleConnections ||
		preserved.MemoryPoolBytes != custom.MemoryPoolBytes ||
		preserved.DefaultRuleMemoryLimitBytes != custom.DefaultRuleMemoryLimitBytes ||
		preserved.ResponseRewriteInputBytes != custom.ResponseRewriteInputBytes ||
		preserved.ResponseRewriteOutputBytes != custom.ResponseRewriteOutputBytes ||
		preserved.ResponseRewriteMaxConcurrent != custom.ResponseRewriteMaxConcurrent ||
		!preserved.UpdatedAt.Equal(custom.UpdatedAt) ||
		!preserved.CreatedAt.Equal(custom.CreatedAt) {
		t.Fatalf("customized resource settings were overwritten: got=%#v want=%#v", preserved, custom)
	}
}
