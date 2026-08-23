package database

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/alireza0/s-ui/config"
	"github.com/alireza0/s-ui/database/model"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var (
	db   *gorm.DB
	dbMu sync.RWMutex
)

// GetPersistedSystemPlatformOS returns the OS captured during the latest
// panel start. It intentionally never probes the running host.
func GetPersistedSystemPlatformOS() string {
	currentDB := GetDB()
	if currentDB == nil {
		return ""
	}
	platform := &model.SystemPlatform{}
	if err := currentDB.First(platform, 1).Error; err != nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(platform.OS))
}

// GetPersistedSystemPlatformArchitecture returns the architecture captured
// during the latest panel start. It intentionally never probes the host.
func GetPersistedSystemPlatformArchitecture() string {
	currentDB := GetDB()
	if currentDB == nil {
		return ""
	}
	platform := &model.SystemPlatform{}
	if err := currentDB.First(platform, 1).Error; err != nil {
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
	openedDB, err := gorm.Open(sqlite.Open(sqliteDSNWithPragmas(dbPath)), c)
	if err != nil {
		return err
	}

	sqlDB, err := openedDB.DB()
	if err != nil {
		return err
	}
	// This project uses a single local SQLite file as the source of truth.
	// Serializing access through one pooled connection reduces lock churn and
	// keeps connection-level PRAGMA state uniform.
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)

	if config.IsDebug() {
		openedDB = openedDB.Debug()
	}
	dbMu.Lock()
	db = openedDB
	dbMu.Unlock()

	runDBResetHooks()
	return nil
}

func InitDB(dbPath string) error {
	err := OpenDB(dbPath)
	if err != nil {
		return err
	}
	target := GetDB()
	if target == nil {
		return fmt.Errorf("database is not initialized")
	}

	// Default Outbounds
	if !target.Migrator().HasTable(&model.Outbound{}) {
		target.Migrator().CreateTable(&model.Outbound{})
		defaultOutbound := []model.Outbound{
			{Type: "direct", Tag: "direct", Options: json.RawMessage(`{}`)},
		}
		target.Create(&defaultOutbound)
	}
	if !target.Migrator().HasTable(&model.MihomoOutbound{}) {
		target.Migrator().CreateTable(&model.MihomoOutbound{})
		defaultOutbound := []model.MihomoOutbound{
			{Type: "direct", Tag: "direct", Options: json.RawMessage(`{}`)},
		}
		target.Create(&defaultOutbound)
	}

	hadMihomoTLSMode := target.Migrator().HasColumn(&model.MihomoTls{}, "mode")
	err = target.AutoMigrate(
		&model.Setting{},
		&model.SettingsState{},
		&model.SingboxConfigState{},
		&model.SubscriptionInitialState{},
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
		&model.DnsServer{},
		&model.Endpoint{},
		&model.User{},
		&model.Tokens{},
		&model.LoginSession{},
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
	if !hadMihomoTLSMode {
		if err := backfillMihomoRealityModes(target); err != nil {
			return err
		}
	}
	if err := ensureClientNameUniqueness(target); err != nil {
		return err
	}
	if err := ensureMihomoClientNameUniqueness(target); err != nil {
		return err
	}
	if err := ensureSettingsStorage(target); err != nil {
		return err
	}
	if err := ensureSingboxConfigStorage(target); err != nil {
		return err
	}
	if err := ensureReverseProxySettingsSingleton(target); err != nil {
		return err
	}
	if err := ensureCertificateRecordIndexes(target); err != nil {
		return err
	}
	if err := ensureAcmeAccountIndexes(target); err != nil {
		return err
	}
	if err := ensureReverseProxyCertificateBalanceIndexes(target); err != nil {
		return err
	}
	if err := ensurePanelCertificateBalanceIndexes(target); err != nil {
		return err
	}
	err = initUser()
	if err != nil {
		return err
	}

	return nil
}

// backfillMihomoRealityModes preserves pre-mode Mihomo TLS records when the
// mode column is introduced. It intentionally handles only the existing
// Reality shape; standalone ShadowTLS rows are not migrated or resurrected.
func backfillMihomoRealityModes(target *gorm.DB) error {
	if target == nil {
		return nil
	}

	type mihomoTLSRow struct {
		ID     uint            `gorm:"column:id"`
		Mode   string          `gorm:"column:mode"`
		Server json.RawMessage `gorm:"column:server"`
	}
	var rows []mihomoTLSRow
	if err := target.Table("mihomo_tls").Select("id, mode, server").Find(&rows).Error; err != nil {
		return err
	}

	for _, row := range rows {
		if !strings.EqualFold(strings.TrimSpace(row.Mode), model.MihomoTlsModeTLS) {
			continue
		}
		var server map[string]interface{}
		if err := json.Unmarshal(row.Server, &server); err != nil || server == nil {
			continue
		}
		if reality, ok := server["reality"].(map[string]interface{}); !ok || reality == nil {
			continue
		}
		if err := target.Model(&model.MihomoTls{}).Where("id = ?", row.ID).Update("mode", model.MihomoTlsModeReality).Error; err != nil {
			return err
		}
	}
	return nil
}

// ensureClientNameUniqueness upgrades legacy databases without making startup
// fail when historical users share a subscription name. The oldest row keeps
// its name; later duplicates receive a deterministic ID suffix before the
// database-level unique index is created.
func ensureClientNameUniqueness(target *gorm.DB) error {
	if target == nil {
		return nil
	}

	type duplicateName struct {
		Name  string
		Count int64
	}
	type clientID struct {
		Id uint
	}

	return target.Transaction(func(tx *gorm.DB) error {
		var duplicates []duplicateName
		if err := tx.Table("clients").
			Select("name, COUNT(*) AS count").
			Group("name").
			Having("COUNT(*) > 1").
			Find(&duplicates).Error; err != nil {
			return err
		}

		for _, duplicate := range duplicates {
			var clients []clientID
			if err := tx.Table("clients").
				Select("id").
				Where("name = ?", duplicate.Name).
				Order("id ASC").
				Find(&clients).Error; err != nil {
				return err
			}
			for index, client := range clients {
				if index == 0 {
					continue
				}
				candidateBase := strings.TrimSpace(duplicate.Name)
				if candidateBase == "" {
					candidateBase = "client"
				}
				candidate := fmt.Sprintf("%s-%d", candidateBase, client.Id)
				for suffix := 2; ; suffix++ {
					var count int64
					if err := tx.Table("clients").Where("name = ?", candidate).Count(&count).Error; err != nil {
						return err
					}
					if count == 0 {
						break
					}
					candidate = fmt.Sprintf("%s-%d-%d", candidateBase, client.Id, suffix)
				}
				if err := tx.Model(&model.Client{}).
					Where("id = ? AND name = ?", client.Id, duplicate.Name).
					Update("name", candidate).Error; err != nil {
					return err
				}
			}
		}

		return tx.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_clients_name_unique ON clients(name)").Error
	})
}

// ensureMihomoClientNameUniqueness upgrades legacy databases without making
// startup fail when historical users share a subscription name. The oldest row
// keeps its name; later duplicates receive a deterministic ID suffix before
// the database-level unique index is created.
func ensureMihomoClientNameUniqueness(target *gorm.DB) error {
	if target == nil {
		return nil
	}

	type duplicateName struct {
		Name  string
		Count int64
	}
	type clientID struct {
		Id uint
	}

	return target.Transaction(func(tx *gorm.DB) error {
		var duplicates []duplicateName
		if err := tx.Table("mihomo_clients").
			Select("name, COUNT(*) AS count").
			Group("name").
			Having("COUNT(*) > 1").
			Find(&duplicates).Error; err != nil {
			return err
		}

		for _, duplicate := range duplicates {
			var clients []clientID
			if err := tx.Table("mihomo_clients").
				Select("id").
				Where("name = ?", duplicate.Name).
				Order("id ASC").
				Find(&clients).Error; err != nil {
				return err
			}
			for index, client := range clients {
				if index == 0 {
					continue
				}
				candidateBase := strings.TrimSpace(duplicate.Name)
				if candidateBase == "" {
					candidateBase = "mihomo-client"
				}
				candidate := fmt.Sprintf("%s-%d", candidateBase, client.Id)
				for suffix := 2; ; suffix++ {
					var count int64
					if err := tx.Table("mihomo_clients").Where("name = ?", candidate).Count(&count).Error; err != nil {
						return err
					}
					if count == 0 {
						break
					}
					candidate = fmt.Sprintf("%s-%d-%d", candidateBase, client.Id, suffix)
				}
				if err := tx.Model(&model.MihomoClient{}).
					Where("id = ? AND name = ?", client.Id, duplicate.Name).
					Update("name", candidate).Error; err != nil {
					return err
				}
			}
		}

		return tx.Exec("CREATE UNIQUE INDEX IF NOT EXISTS idx_mihomo_clients_name_unique ON mihomo_clients(name)").Error
	})
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

func ensureSingboxConfigStorage(target *gorm.DB) error {
	if target == nil {
		return nil
	}

	return target.Transaction(func(tx *gorm.DB) error {
		state := model.SingboxConfigState{Id: 1, Revision: 1}
		return tx.Where("id = ?", state.Id).FirstOrCreate(&state).Error
	})
}

func GetDB() *gorm.DB {
	dbMu.RLock()
	currentDB := db
	dbMu.RUnlock()
	return currentDB
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
			return migrateReverseProxyResourceDefaults(tx, existing)
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

// migrateReverseProxyResourceDefaults upgrades only an untouched installation
// that still has the complete previous built-in resource tuple.  A partial
// match is treated as a user-customized policy and is preserved as-is.
func migrateReverseProxyResourceDefaults(tx *gorm.DB, existing model.ReverseProxySettings) error {
	if existing.MemoryPoolBytes != model.ReverseProxyLegacyMemoryPoolBytes ||
		existing.DefaultRuleMemoryLimitBytes != model.ReverseProxyLegacyDefaultRuleMemoryLimitBytes ||
		existing.ResponseRewriteInputBytes != model.ReverseProxyLegacyResponseRewriteInputBytes ||
		existing.ResponseRewriteOutputBytes != model.ReverseProxyLegacyResponseRewriteOutputBytes ||
		existing.ResponseRewriteMaxConcurrent != model.ReverseProxyLegacyResponseRewriteMaxConcurrent {
		return nil
	}

	defaults := model.DefaultReverseProxySettings()
	nextRevision := existing.Revision + 1
	if nextRevision == 0 {
		nextRevision = defaults.Revision
	}
	updates := map[string]interface{}{
		"revision":                        nextRevision,
		"memory_pool_bytes":               defaults.MemoryPoolBytes,
		"default_rule_memory_limit_bytes": defaults.DefaultRuleMemoryLimitBytes,
		"response_rewrite_input_bytes":    defaults.ResponseRewriteInputBytes,
		"response_rewrite_output_bytes":   defaults.ResponseRewriteOutputBytes,
		"response_rewrite_max_concurrent": defaults.ResponseRewriteMaxConcurrent,
	}
	return tx.Model(&model.ReverseProxySettings{}).
		Where("id = ?", existing.Id).
		Updates(updates).Error
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
