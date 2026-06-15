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
		&model.PanelCertificate{},
		&model.SelfSignedAuthority{},
		&model.AcmeAccount{},
		&model.AcmeDNSAccount{},
		&model.AcmeCertificate{},
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
		&model.ReverseProxyRule{},
		&model.ReverseProxyCertificateBalanceState{},
		&model.PanelCertificateBalanceState{},
		&model.PortForwardLimitState{},
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
	if err := ensureCertificateRecordIndexes(db); err != nil {
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

func GetDB() *gorm.DB {
	return db
}

func IsNotFound(err error) bool {
	return err == gorm.ErrRecordNotFound
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
