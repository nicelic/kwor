package database

import (
	"bytes"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"github.com/alireza0/s-ui/cmd/migration"
	"github.com/alireza0/s-ui/config"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/logger"
	"github.com/alireza0/s-ui/util/common"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

type managedRuntimeFileBackupEntry struct {
	Path      string `gorm:"column:path;primaryKey"`
	DirPath   string `gorm:"column:dir_path;not null;index:idx_managed_runtime_files_dir_path"`
	FileName  string `gorm:"column:file_name;not null"`
	Ext       string `gorm:"column:ext;not null"`
	Content   []byte `gorm:"column:content;not null"`
	Size      int64  `gorm:"column:size;not null;default:0"`
	UpdatedAt int64  `gorm:"column:updated_at;not null"`
}

func (managedRuntimeFileBackupEntry) TableName() string {
	return "managed_runtime_files"
}

func copyBackupTable[T any](src *gorm.DB, dst *gorm.DB) error {
	var entity T
	if !src.Migrator().HasTable(&entity) {
		return nil
	}

	rows := make([]T, 0)
	if err := src.Model(&entity).Scan(&rows).Error; err != nil {
		return err
	}
	if len(rows) == 0 {
		return nil
	}
	return dst.Save(rows).Error
}

func GetDb(exclude string) ([]byte, error) {
	exclude_changes, exclude_stats := false, false
	for _, table := range strings.Split(exclude, ",") {
		if table == "changes" {
			exclude_changes = true
		} else if table == "stats" {
			exclude_stats = true
		}
	}

	dbDir := filepath.Dir(config.GetDBPath())
	if err := os.MkdirAll(dbDir, 01740); err != nil {
		return nil, err
	}
	dbPath := filepath.Join(dbDir, fmt.Sprintf("%s_%s.db", config.GetName(), time.Now().Format("20060102-150405")))

	backupDb, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		return nil, err
	}
	defer os.Remove(dbPath)

	err = backupDb.AutoMigrate(
		&model.Setting{},
		&model.Tls{},
		&model.MihomoTls{},
		&model.Inbound{},
		&model.MihomoInbound{},
		&model.Outbound{},
		&model.MihomoOutbound{},
		&model.MihomoOutboundGroup{},
		&model.OutboundGroup{},
		&model.SubOutbound{},
		&model.SubGroup{},
		&model.Service{},
		&model.Endpoint{},
		&model.User{},
		&model.Tokens{},
		&model.Client{},
		&model.MihomoClient{},
		&model.InboundTrafficState{},
		&model.ClientPortLimitState{},
		&model.MihomoClientPortLimitState{},
		&model.PortForwardRule{},
		&model.ReverseProxyRule{},
		&model.ReverseProxyCertificateBalanceState{},
		&model.PanelCertificateBalanceState{},
		&model.ClientInboundTrafficState{},
		&model.MihomoInboundRedirectState{},
		&model.MihomoClientInboundTrafficState{},
		&model.Stats{},
		&model.Changes{},
		&managedRuntimeFileBackupEntry{},
	)
	if err != nil {
		return nil, err
	}

	copySteps := []func() error{
		func() error { return copyBackupTable[model.Setting](db, backupDb) },
		func() error { return copyBackupTable[model.Tls](db, backupDb) },
		func() error { return copyBackupTable[model.MihomoTls](db, backupDb) },
		func() error { return copyBackupTable[model.Inbound](db, backupDb) },
		func() error { return copyBackupTable[model.MihomoInbound](db, backupDb) },
		func() error { return copyBackupTable[model.Outbound](db, backupDb) },
		func() error { return copyBackupTable[model.MihomoOutbound](db, backupDb) },
		func() error { return copyBackupTable[model.MihomoOutboundGroup](db, backupDb) },
		func() error { return copyBackupTable[model.OutboundGroup](db, backupDb) },
		func() error { return copyBackupTable[model.SubOutbound](db, backupDb) },
		func() error { return copyBackupTable[model.SubGroup](db, backupDb) },
		func() error { return copyBackupTable[model.Service](db, backupDb) },
		func() error { return copyBackupTable[model.Endpoint](db, backupDb) },
		func() error { return copyBackupTable[model.User](db, backupDb) },
		func() error { return copyBackupTable[model.Tokens](db, backupDb) },
		func() error { return copyBackupTable[model.Client](db, backupDb) },
		func() error { return copyBackupTable[model.MihomoClient](db, backupDb) },
		func() error { return copyBackupTable[model.InboundTrafficState](db, backupDb) },
		func() error { return copyBackupTable[model.ClientPortLimitState](db, backupDb) },
		func() error { return copyBackupTable[model.MihomoClientPortLimitState](db, backupDb) },
		func() error { return copyBackupTable[model.PortForwardRule](db, backupDb) },
		func() error { return copyBackupTable[model.ReverseProxyRule](db, backupDb) },
		func() error { return copyBackupTable[model.ReverseProxyCertificateBalanceState](db, backupDb) },
		func() error { return copyBackupTable[model.PanelCertificateBalanceState](db, backupDb) },
		func() error { return copyBackupTable[model.ClientInboundTrafficState](db, backupDb) },
		func() error { return copyBackupTable[model.MihomoInboundRedirectState](db, backupDb) },
		func() error { return copyBackupTable[model.MihomoClientInboundTrafficState](db, backupDb) },
		func() error {
			if exclude_stats {
				return nil
			}
			return copyBackupTable[model.Stats](db, backupDb)
		},
		func() error {
			if exclude_changes {
				return nil
			}
			return copyBackupTable[model.Changes](db, backupDb)
		},
		func() error { return copyBackupTable[managedRuntimeFileBackupEntry](db, backupDb) },
	}
	for _, step := range copySteps {
		if err := step(); err != nil {
			return nil, err
		}
	}

	// Update WAL
	err = backupDb.Exec("PRAGMA wal_checkpoint;").Error
	if err != nil {
		return nil, err
	}

	bdb, _ := backupDb.DB()
	bdb.Close()

	// Open the file for reading
	file, err := os.Open(dbPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	// Read the file contents
	fileContents, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}

	return fileContents, nil
}

func ImportDB(file multipart.File) error {
	// Check if the file is a SQLite database
	isValidDb, err := IsSQLiteDB(file)
	if err != nil {
		return common.NewErrorf("Error checking db file format: %v", err)
	}
	if !isValidDb {
		return common.NewError("Invalid db file format")
	}

	// Reset the file reader to the beginning
	_, err = file.Seek(0, 0)
	if err != nil {
		return common.NewErrorf("Error resetting file reader: %v", err)
	}

	// Save the file as temporary file
	tempPath := fmt.Sprintf("%s.temp", config.GetDBPath())
	// Remove the existing fallback file (if any) before creating one
	_, err = os.Stat(tempPath)
	if err == nil {
		errRemove := os.Remove(tempPath)
		if errRemove != nil {
			return common.NewErrorf("Error removing existing temporary db file: %v", errRemove)
		}
	}
	// Create the temporary file
	tempFile, err := os.Create(tempPath)
	if err != nil {
		return common.NewErrorf("Error creating temporary db file: %v", err)
	}
	defer tempFile.Close()

	// Remove temp file before returning
	defer os.Remove(tempPath)

	// Close old DB
	old_db, _ := db.DB()
	old_db.Close()

	// Save uploaded file to temporary file
	_, err = io.Copy(tempFile, file)
	if err != nil {
		return common.NewErrorf("Error saving db: %v", err)
	}

	// Check if we can init db or not
	newDb, err := gorm.Open(sqlite.Open(tempPath), &gorm.Config{})
	if err != nil {
		return common.NewErrorf("Error checking db: %v", err)
	}
	newDb_db, _ := newDb.DB()
	newDb_db.Close()

	// Backup the current database for fallback
	fallbackPath := fmt.Sprintf("%s.backup", config.GetDBPath())
	// Remove the existing fallback file (if any)
	_, err = os.Stat(fallbackPath)
	if err == nil {
		errRemove := os.Remove(fallbackPath)
		if errRemove != nil {
			return common.NewErrorf("Error removing existing fallback db file: %v", errRemove)
		}
	}
	// Move the current database to the fallback location
	err = os.Rename(config.GetDBPath(), fallbackPath)
	if err != nil {
		return common.NewErrorf("Error backing up temporary db file: %v", err)
	}

	// Remove the temporary file before returning
	defer os.Remove(fallbackPath)

	// Move temp to DB path
	err = os.Rename(tempPath, config.GetDBPath())
	if err != nil {
		errRename := os.Rename(fallbackPath, config.GetDBPath())
		if errRename != nil {
			return common.NewErrorf("Error moving db file and restoring fallback: %v", errRename)
		}
		return common.NewErrorf("Error moving db file: %v", err)
	}

	// Migrate DB
	migration.MigrateDb()
	err = InitDB(config.GetDBPath())
	if err != nil {
		errRename := os.Rename(fallbackPath, config.GetDBPath())
		if errRename != nil {
			return common.NewErrorf("Error migrating db and restoring fallback: %v", errRename)
		}
		return common.NewErrorf("Error migrating db: %v", err)
	}

	// Restart app
	err = SendSighup()
	if err != nil {
		return common.NewErrorf("Error restarting app: %v", err)
	}

	return nil
}

func IsSQLiteDB(file io.Reader) (bool, error) {
	signature := []byte("SQLite format 3\x00")
	buf := make([]byte, len(signature))
	_, err := file.Read(buf)
	if err != nil {
		return false, err
	}
	return bytes.Equal(buf, signature), nil
}

func SendSighup() error {
	// Get the current process
	process, err := os.FindProcess(os.Getpid())
	if err != nil {
		return err
	}

	// Send SIGHUP to the current process
	go func() {
		time.Sleep(3 * time.Second)
		if runtime.GOOS == "windows" {
			err = process.Kill()
		} else {
			err = process.Signal(syscall.SIGHUP)
		}
		if err != nil {
			logger.Error("send signal SIGHUP failed:", err)
		}
	}()
	return nil
}
