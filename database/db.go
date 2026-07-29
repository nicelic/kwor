package database

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/alireza0/s-ui/config"
	"github.com/alireza0/s-ui/database/model"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var db *gorm.DB

// GetPersistedSystemPlatformOS returns the OS captured during the latest
// panel start. It intentionally never probes the running host.
func GetPersistedSystemPlatformOS() string {
	if db == nil {
		return ""
	}
	platform := &model.SystemPlatform{}
	if err := db.First(platform, 1).Error; err != nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(platform.OS))
}

// GetPersistedSystemPlatformArchitecture returns the architecture captured
// during the latest panel start. It intentionally never probes the host.
func GetPersistedSystemPlatformArchitecture() string {
	if db == nil {
		return ""
	}
	platform := &model.SystemPlatform{}
	if err := db.First(platform, 1).Error; err != nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(platform.Architecture))
}

func initUser() error {
	// Let service.UserService handle first login as registration
	return nil
}

func sqliteDSNWithPragmas(dbPath string) string {
	base := dbPath
	rawQuery := ""
	if idx := strings.Index(dbPath, "?"); idx >= 0 {
		base = dbPath[:idx]
		rawQuery = dbPath[idx+1:]
	}

	values, err := url.ParseQuery(rawQuery)
	if err != nil {
		values = url.Values{}
	}
	values.Add("_pragma", "secure_delete(1)")

	encoded := values.Encode()
	if encoded == "" {
		return base
	}
	return base + "?" + encoded
}

func OpenDB(dbPath string) error {
	dir := filepath.Dir(dbPath)
	err := os.MkdirAll(dir, 01740)
	if err != nil {
		return err
	}

	var gormLogger logger.Interface

	if config.IsDebug() {
		gormLogger = logger.Default
	} else {
		gormLogger = logger.Discard
	}

	c := &gorm.Config{
		Logger: gormLogger,
	}
	db, err = gorm.Open(sqlite.Open(sqliteDSNWithPragmas(dbPath)), c)
	if err != nil {
		return err
	}

	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	// This project uses a single local SQLite file as the source of truth.
	// Serializing access through one pooled connection reduces lock churn and
	// keeps connection-level PRAGMA state uniform.
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)

	if config.IsDebug() {
		db = db.Debug()
	}

	runDBResetHooks()
	return nil
}

func InitDB(dbPath string) error {
	err := OpenDB(dbPath)
	if err != nil {
		return err
	}

	// Default Outbounds
	if !db.Migrator().HasTable(&model.Outbound{}) {
		db.Migrator().CreateTable(&model.Outbound{})
		defaultOutbound := []model.Outbound{
			{Type: "direct", Tag: "direct", Options: json.RawMessage(`{}`)},
		}
		db.Create(&defaultOutbound)
	}
	if !db.Migrator().HasTable(&model.MihomoOutbound{}) {
		db.Migrator().CreateTable(&model.MihomoOutbound{})
		defaultOutbound := []model.MihomoOutbound{
			{Type: "direct", Tag: "direct", Options: json.RawMessage(`{}`)},
		}
		db.Create(&defaultOutbound)
	}

	err = db.AutoMigrate(
		&model.Setting{},
		&model.SettingsState{},
		&model.SystemPlatform{},
		&model.PanelCertificate{},
		&model.SelfSignedAuthority{},
		&model.AcmeAccount{},
		&model.AcmeDNSAccount{},
		&model.CertificateRecord{},
		&model.Tls{},
		&model.MihomoTls{},
		&model.Inbound{},
		&model.MihomoInbound{},
		&model.Outbound{},
		&model.MihomoOutbound{},
		&model.MihomoOutboundGroup{},
		&model.OutboundGroup{},
		&model.SubOutbound{},
		&model.SubSyncBlock{},
		&model.SubGroup{},
		&model.Service{},
		&model.Endpoint{},
		&model.User{},
		&model.Tokens{},
		&model.Stats{},
		&model.InboundTrafficState{},
		&model.ClientPortLimitState{},
		&model.ClientPortBlockState{},
		&model.FirewallRule{},
		&model.FirewallGeoRule{},
		&model.PortForwardRule{},
		&model.PortForwardKernelForwardState{},
		&model.ReverseProxyRule{},
		&model.ReverseProxySettings{},
		&model.ReverseProxyCertificateBalanceState{},
		&model.PanelCertificateBalanceState{},
		&model.PortForwardLimitState{},
		&model.PortForwardRuleTrafficState{},
		&model.PortForwardOverviewTrafficState{},
		&model.MihomoClientPortLimitState{},
		&model.MihomoClientPortBlockState{},
		&model.ClientInboundTrafficState{},
		&model.MihomoInboundRedirectState{},
		&model.MihomoClientInboundTrafficState{},
		&model.Client{},
		&model.MihomoClient{},
		&model.Changes{},
		&managedRuntimeFileBackupEntry{},
	)
	if err != nil {
		return err
	}
	if err := ensureSettingsStorage(db); err != nil {
		return err
	}
	if err := ensureReverseProxySettingsSingleton(db); err != nil {
		return err
	}
	if err := ensureCertificateRecordIndexes(db); err != nil {
		return err
	}
	if err := ensureAcmeAccountIndexes(db); err != nil {
		return err
	}
	if err := ensureReverseProxyCertificateBalanceIndexes(db); err != nil {
		return err
	}
	if err := ensurePanelCertificateBalanceIndexes(db); err != nil {
		return err
	}
	err = initUser()
	if err != nil {
		return err
	}

	return nil
}

func ensureSettingsStorage(target *gorm.DB) error {
	if target == nil {
		return nil
	}

	return target.Transaction(func(tx *gorm.DB) error {
		// Legacy databases may contain duplicate keys. GetAllSetting historically
		// resolved those by ascending id, so the largest id is the effective row.
		if err := tx.Exec(`DELETE FROM settings
			WHERE id NOT IN (
				SELECT MAX(id) FROM settings GROUP BY key
			)`).Error; err != nil {
			return err
		}
		if err := tx.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_settings_key_unique ON settings(key)").Error; err != nil {
			return err
		}

		state := model.SettingsState{Id: 1, Revision: 1}
		return tx.Where("id = ?", state.Id).FirstOrCreate(&state).Error
	})
}

func GetDB() *gorm.DB {
	return db
}

func IsNotFound(err error) bool {
	return err == gorm.ErrRecordNotFound
}

// ensureReverseProxySettingsSingleton creates the resource-policy singleton
// during database initialization.  Its first creation also migrates legacy
// zero rule values that previously meant the old fixed defaults; later saves
// keep zero as the deliberate "no additional rule limit" value.
func ensureReverseProxySettingsSingleton(target *gorm.DB) error {
	if target == nil {
		return nil
	}
	return target.Transaction(func(tx *gorm.DB) error {
		settings := model.DefaultReverseProxySettings()
		var existing model.ReverseProxySettings
		err := tx.Where("id = ?", settings.Id).First(&existing).Error
		if err == nil {
			return nil
		}
		if err != nil && !IsNotFound(err) {
			return err
		}
		if err := tx.Create(&settings).Error; err != nil {
			return err
		}
		if err := tx.Model(&model.ReverseProxyRule{}).
			Where("max_concurrent_requests = ?", 0).
			Update("max_concurrent_requests", model.ReverseProxyLegacyMaxConcurrentRequests).Error; err != nil {
			return err
		}
		return tx.Model(&model.ReverseProxyRule{}).
			Where("dns_max_concurrent_queries = ?", 0).
			Update("dns_max_concurrent_queries", model.ReverseProxyLegacyDNSMaxConcurrentQueries).Error
	})
}

func ensureCertificateRecordIndexes(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	if !db.Migrator().HasTable(&model.CertificateRecord{}) {
		return nil
	}
	createSQL := "CREATE UNIQUE INDEX IF NOT EXISTS idx_certificate_records_display_id_nonzero ON certificate_records(display_id) WHERE display_id > 0"
	return db.Exec(createSQL).Error
}

func ensureAcmeAccountIndexes(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	if db.Migrator().HasTable(&model.AcmeAccount{}) {
		if err := db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_acme_accounts_display_id_nonzero ON acme_accounts(display_id) WHERE display_id > 0").Error; err != nil {
			return err
		}
	}
	if db.Migrator().HasTable(&model.AcmeDNSAccount{}) {
		if err := db.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_acme_dns_accounts_display_id_nonzero ON acme_dns_accounts(display_id) WHERE display_id > 0").Error; err != nil {
			return err
		}
	}
	return nil
}

func ensureReverseProxyCertificateBalanceIndexes(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	if !db.Migrator().HasTable(&model.ReverseProxyCertificateBalanceState{}) {
		return nil
	}
	createSQL := "CREATE UNIQUE INDEX IF NOT EXISTS idx_rp_cert_balance_listener_sni_cert ON reverse_proxy_certificate_balance_states(listener_key, sni_bucket, certificate_record_id)"
	return db.Exec(createSQL).Error
}

func ensurePanelCertificateBalanceIndexes(db *gorm.DB) error {
	if db == nil {
		return nil
	}
	if !db.Migrator().HasTable(&model.PanelCertificateBalanceState{}) {
		return nil
	}
	createSQL := "CREATE UNIQUE INDEX IF NOT EXISTS idx_panel_cert_balance_listener_sni_cert ON panel_certificate_balance_states(listener_key, sni_bucket, certificate_record_id)"
	return db.Exec(createSQL).Error
}
