package database

import (
	"os"
	"path/filepath"
	"sync"

	"github.com/alireza0/s-ui/config"
	"github.com/alireza0/s-ui/database/model"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

var (
	ddnsDB   *gorm.DB
	ddnsDBMu sync.RWMutex
)

func GetDDNSDBPath() string {
	return filepath.Join(config.GetDBFolderPath(), "ddns.db")
}

func InitDDNSDB() error {
	ddnsDBMu.Lock()
	defer ddnsDBMu.Unlock()

	if ddnsDB != nil {
		return nil
	}

	dbPath := GetDDNSDBPath()
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 01740); err != nil {
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
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)

	if config.IsDebug() {
		openedDB = openedDB.Debug()
	}

	// Auto-migrate DDNS and DNS tables directly in ddns.db
	if err := openedDB.AutoMigrate(
		&model.DDNSAccount{},
		&model.DDNSRule{},
		&model.DNSAccount{},
		&model.DNSDomainFavorite{},
	); err != nil {
		return err
	}

	// Smooth initial migration: if dns_accounts is newly created and empty,
	// clone existing accounts from ddns_accounts once to isolate both datasets smoothly.
	var dnsAccountCount int64
	if err := openedDB.Model(&model.DNSAccount{}).Count(&dnsAccountCount).Error; err == nil && dnsAccountCount == 0 {
		var ddnsAccounts []model.DDNSAccount
		if err := openedDB.Find(&ddnsAccounts).Error; err == nil && len(ddnsAccounts) > 0 {
			for _, dAcc := range ddnsAccounts {
				dnsAcc := model.DNSAccount{
					Id:           dAcc.Id,
					DisplayID:    dAcc.DisplayID,
					Name:         dAcc.Name,
					ProviderCode: dAcc.ProviderCode,
					EnvJSON:      dAcc.EnvJSON,
					Remark:       dAcc.Remark,
					CreatedAt:    dAcc.CreatedAt,
					UpdatedAt:    dAcc.UpdatedAt,
				}
				_ = openedDB.Create(&dnsAcc).Error
			}
		}
	}

	ddnsDB = openedDB
	return nil
}

func GetDDNSDB() *gorm.DB {
	ddnsDBMu.RLock()
	db := ddnsDB
	ddnsDBMu.RUnlock()

	if db != nil {
		return db
	}

	// Lazy initialization if not initialized yet
	if err := InitDDNSDB(); err != nil {
		return nil
	}

	ddnsDBMu.RLock()
	defer ddnsDBMu.RUnlock()
	return ddnsDB
}
