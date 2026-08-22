package database

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
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

type DBBackupArchive struct {
	FileName string
	Data     []byte
}

type boundedBytesBuffer struct {
	buffer   bytes.Buffer
	maxBytes int64
}

func (b *boundedBytesBuffer) Write(data []byte) (int, error) {
	if b.maxBytes <= 0 {
		return 0, fmt.Errorf("buffer size limit must be positive")
	}
	if int64(b.buffer.Len())+int64(len(data)) > b.maxBytes {
		return 0, fmt.Errorf("output exceeds %d bytes", b.maxBytes)
	}
	return b.buffer.Write(data)
}

func (b *boundedBytesBuffer) Bytes() []byte {
	return b.buffer.Bytes()
}

type pendingDBRestoreMarker struct {
	StageDir  string `json:"stageDir"`
	BackupDir string `json:"backupDir,omitempty"`
	Applied   bool   `json:"applied,omitempty"`
}

const (
	databaseImportMaxBytes               int64 = 256 * 1024 * 1024
	dbBackupArchiveMaxBytes              int64 = 256 * 1024 * 1024
	dbBackupArchiveMaxEntryBytes         int64 = 256 * 1024 * 1024
	dbBackupArchiveMaxEntries                  = 16
	databaseUploadMultipartOverheadBytes int64 = 1024 * 1024
)

// MaxDatabaseImportUploadBytes leaves room for multipart boundaries and field metadata.
func MaxDatabaseImportUploadBytes() int64 {
	return databaseImportMaxBytes + databaseUploadMultipartOverheadBytes
}

// MaxDBBackupArchiveUploadBytes leaves room for multipart boundaries and field metadata.
func MaxDBBackupArchiveUploadBytes() int64 {
	return dbBackupArchiveMaxBytes + databaseUploadMultipartOverheadBytes
}

type dbBackupArchiveLimits struct {
	maxEntries    int
	maxEntryBytes int64
	maxTotalBytes int64
}

var defaultDBBackupArchiveLimits = dbBackupArchiveLimits{
	maxEntries:    dbBackupArchiveMaxEntries,
	maxEntryBytes: dbBackupArchiveMaxEntryBytes,
	maxTotalBytes: dbBackupArchiveMaxBytes,
}

// Database replacement changes the live SQLite file. Keep the legacy import
// and archive restore paths mutually exclusive inside one panel process.
var databaseImportRestoreMu sync.Mutex

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
	// Export and replacement both read or move the live SQLite file. Keep them
	// mutually exclusive so an export can never copy from a connection that a
	// concurrent import has just closed.
	databaseImportRestoreMu.Lock()
	defer databaseImportRestoreMu.Unlock()

	if err := runDBBeforeBackupHooks(); err != nil {
		return nil, common.NewErrorf("备份前刷新运行态账本失败: %v", err)
	}
	db := GetDB()
	if db == nil {
		return nil, common.NewError("数据库尚未初始化")
	}
	excludeChanges, excludeStats := false, false
	for _, table := range strings.Split(exclude, ",") {
		if table == "changes" {
			excludeChanges = true
		} else if table == "stats" {
			excludeStats = true
		}
	}

	dbDir := filepath.Dir(config.GetDBPath())
	if err := os.MkdirAll(dbDir, 0o740); err != nil {
		return nil, err
	}
	dbPath := filepath.Join(dbDir, fmt.Sprintf("%s_%s.db", config.GetName(), time.Now().Format("20060102-150405")))

	backupDb, err := gorm.Open(sqlite.Open(dbPath), &gorm.Config{})
	if err != nil {
		return nil, err
	}
	backupSQLDB, err := backupDb.DB()
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = backupSQLDB.Close()
	}()
	defer os.Remove(dbPath)

	err = backupDb.AutoMigrate(
		&model.Setting{},
		&model.SettingsState{},
		&model.SingboxConfigState{},
		&model.SubscriptionInitialState{},
		// ACME/DNS account state and certificate inventory are all database
		// source data. Keep them together in the lightweight database export as
		// well as the full archive backup.
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
		&model.SubGroup{},
		&model.Service{},
		&model.DnsServer{},
		&model.Endpoint{},
		&model.User{},
		&model.Tokens{},
		&model.Client{},
		&model.MihomoClient{},
		&model.InboundTrafficState{},
		&model.ClientPortLimitState{},
		&model.MihomoClientPortLimitState{},
		&model.PortForwardRule{},
		&model.PortForwardRuleTrafficState{},
		&model.PortForwardOverviewTrafficState{},
		// PortForwardKernelForwardState is deliberately excluded: it records
		// host-local sysctl ownership and must never cross a database backup.
		&model.ReverseProxyRule{},
		&model.ReverseProxySettings{},
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
		func() error { return copyBackupTable[model.SettingsState](db, backupDb) },
		func() error { return copyBackupTable[model.SingboxConfigState](db, backupDb) },
		func() error { return copyBackupTable[model.SubscriptionInitialState](db, backupDb) },
		func() error { return copyBackupTable[model.AcmeAccount](db, backupDb) },
		func() error { return copyBackupTable[model.AcmeDNSAccount](db, backupDb) },
		func() error { return copyBackupTable[model.CertificateRecord](db, backupDb) },
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
		func() error { return copyBackupTable[model.DnsServer](db, backupDb) },
		func() error { return copyBackupTable[model.Endpoint](db, backupDb) },
		func() error { return copyBackupTable[model.User](db, backupDb) },
		func() error { return copyBackupTable[model.Tokens](db, backupDb) },
		func() error { return copyBackupTable[model.Client](db, backupDb) },
		func() error { return copyBackupTable[model.MihomoClient](db, backupDb) },
		func() error { return copyBackupTable[model.InboundTrafficState](db, backupDb) },
		func() error { return copyBackupTable[model.ClientPortLimitState](db, backupDb) },
		func() error { return copyBackupTable[model.MihomoClientPortLimitState](db, backupDb) },
		func() error { return copyBackupTable[model.PortForwardRule](db, backupDb) },
		func() error { return copyBackupTable[model.PortForwardRuleTrafficState](db, backupDb) },
		func() error { return copyBackupTable[model.PortForwardOverviewTrafficState](db, backupDb) },
		// Do not copy model.PortForwardKernelForwardState; see AutoMigrate list.
		func() error { return copyBackupTable[model.ReverseProxyRule](db, backupDb) },
		func() error { return copyBackupTable[model.ReverseProxySettings](db, backupDb) },
		func() error { return copyBackupTable[model.ReverseProxyCertificateBalanceState](db, backupDb) },
		func() error { return copyBackupTable[model.PanelCertificateBalanceState](db, backupDb) },
		func() error { return copyBackupTable[model.ClientInboundTrafficState](db, backupDb) },
		func() error { return copyBackupTable[model.MihomoInboundRedirectState](db, backupDb) },
		func() error { return copyBackupTable[model.MihomoClientInboundTrafficState](db, backupDb) },
		func() error {
			if excludeStats {
				return nil
			}
			return copyBackupTable[model.Stats](db, backupDb)
		},
		func() error {
			if excludeChanges {
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

	if err := backupDb.Exec("PRAGMA wal_checkpoint;").Error; err != nil {
		return nil, err
	}

	if err := backupSQLDB.Close(); err != nil {
		return nil, err
	}

	file, err := os.Open(dbPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	fileContents, err := readReaderWithLimit(file, databaseImportMaxBytes, "数据库备份")
	if err != nil {
		return nil, err
	}

	return fileContents, nil
}

func BuildDBBackupArchive() (*DBBackupArchive, error) {
	// Archive snapshots must see one stable database directory. Imports and
	// restores use the same gate before replacing that directory.
	databaseImportRestoreMu.Lock()
	defer databaseImportRestoreMu.Unlock()

	if err := runDBBeforeBackupHooks(); err != nil {
		return nil, common.NewErrorf("归档前刷新运行态账本失败: %v", err)
	}
	dbDir := config.GetDBFolderPath()
	if err := os.MkdirAll(dbDir, 0o740); err != nil {
		return nil, err
	}

	snapshotDir, items, err := createDBBackupSnapshots(dbDir)
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(snapshotDir)
	if len(items) == 0 {
		return nil, common.NewError("db 目录中没有可备份文件")
	}

	buffer := &boundedBytesBuffer{maxBytes: dbBackupArchiveMaxBytes}
	zipWriter := zip.NewWriter(buffer)
	var totalSnapshotBytes int64

	for _, item := range items {
		rel, err := filepath.Rel(snapshotDir, item)
		if err != nil {
			_ = zipWriter.Close()
			return nil, err
		}
		rel = filepath.ToSlash(rel)

		info, err := os.Stat(item)
		if err != nil {
			_ = zipWriter.Close()
			return nil, err
		}
		remainingBytes := dbBackupArchiveMaxBytes - totalSnapshotBytes
		if info.Size() < 0 || info.Size() > remainingBytes {
			_ = zipWriter.Close()
			return nil, common.NewErrorf("数据库备份内容超过 %d 字节限制", dbBackupArchiveMaxBytes)
		}

		header, err := zip.FileInfoHeader(info)
		if err != nil {
			_ = zipWriter.Close()
			return nil, err
		}
		header.Name = rel
		header.Method = zip.Deflate

		writer, err := zipWriter.CreateHeader(header)
		if err != nil {
			_ = zipWriter.Close()
			return nil, err
		}

		file, err := os.Open(item)
		if err != nil {
			_ = zipWriter.Close()
			return nil, err
		}
		var copyErr error
		if info.Size() > 0 {
			var written int64
			written, copyErr = copyReaderWithLimit(writer, file, remainingBytes)
			totalSnapshotBytes += written
		}
		closeErr := file.Close()
		if copyErr != nil {
			_ = zipWriter.Close()
			return nil, copyErr
		}
		if closeErr != nil {
			_ = zipWriter.Close()
			return nil, closeErr
		}
	}

	if err := zipWriter.Close(); err != nil {
		return nil, err
	}

	fileName := fmt.Sprintf("%s_db_backup_%s.zip", config.GetName(), time.Now().Format("20060102-150405"))
	return &DBBackupArchive{
		FileName: fileName,
		Data:     buffer.Bytes(),
	}, nil
}

func ImportDB(file multipart.File) (err error) {
	if file == nil {
		return common.NewError("未选择数据库文件")
	}
	databaseImportRestoreMu.Lock()
	defer databaseImportRestoreMu.Unlock()
	defer func() {
		if err != nil {
			runDBRestoreAbortHooks()
		}
	}()
	if err := runDBBeforeRestoreHooks(); err != nil {
		return err
	}
	if err := ensureNoPendingDBRestore(); err != nil {
		return err
	}

	isValidDb, err := IsSQLiteDB(file)
	if err != nil {
		return common.NewErrorf("Error checking db file format: %v", err)
	}
	if !isValidDb {
		return common.NewError("Invalid db file format")
	}

	if _, err = file.Seek(0, 0); err != nil {
		return common.NewErrorf("Error resetting file reader: %v", err)
	}

	tempPath := fmt.Sprintf("%s.temp", config.GetDBPath())
	if err := removeIfExists(tempPath); err != nil {
		return common.NewErrorf("Error removing existing temporary db file: %v", err)
	}

	tempFile, err := os.Create(tempPath)
	if err != nil {
		return common.NewErrorf("Error creating temporary db file: %v", err)
	}
	defer os.Remove(tempPath)

	if _, err = copyReaderWithLimit(tempFile, file, databaseImportMaxBytes); err != nil {
		_ = tempFile.Close()
		return common.NewErrorf("Error saving db: %v", err)
	}
	if err := tempFile.Close(); err != nil {
		return common.NewErrorf("Error closing temporary db: %v", err)
	}

	if err := validateSQLiteDatabaseFile(tempPath); err != nil {
		return common.NewErrorf("Error checking db: %v", err)
	}

	return replaceMainDatabaseWithImportedFile(tempPath)
}

func replaceMainDatabaseWithImportedFile(tempPath string) error {
	dbPath := config.GetDBPath()
	fallbackPath := fmt.Sprintf("%s.backup", dbPath)
	if err := closeMainDatabase(); err != nil {
		return common.NewErrorf("Error closing existing db: %v", err)
	}

	reopenOriginal := func(cause error) error {
		if reopenErr := InitDB(dbPath); reopenErr != nil {
			return common.NewErrorf("%v; Error reopening original db: %v", cause, reopenErr)
		}
		return cause
	}

	if err := removeIfExists(fallbackPath); err != nil {
		return reopenOriginal(common.NewErrorf("Error removing existing fallback db file: %v", err))
	}

	if err := os.Rename(dbPath, fallbackPath); err != nil {
		return reopenOriginal(common.NewErrorf("Error backing up original db file: %v", err))
	}

	restoreOriginal := func(cause error) error {
		if closeErr := closeMainDatabase(); closeErr != nil {
			return common.NewErrorf("%v; Error closing failed imported db: %v", cause, closeErr)
		}
		if removeErr := removeIfExists(dbPath); removeErr != nil {
			return common.NewErrorf("%v; Error removing failed imported db: %v", cause, removeErr)
		}
		if restoreErr := os.Rename(fallbackPath, dbPath); restoreErr != nil {
			return common.NewErrorf("%v; Error restoring original db: %v", cause, restoreErr)
		}
		if reopenErr := InitDB(dbPath); reopenErr != nil {
			return common.NewErrorf("%v; Error reopening restored db: %v", cause, reopenErr)
		}
		return cause
	}

	if err := os.Rename(tempPath, dbPath); err != nil {
		return restoreOriginal(common.NewErrorf("Error moving imported db file: %v", err))
	}

	if err := migration.MigrateDbWithError(); err != nil {
		return restoreOriginal(common.NewErrorf("Error migrating db: %v", err))
	}
	if err := InitDB(dbPath); err != nil {
		return restoreOriginal(common.NewErrorf("Error initializing imported db: %v", err))
	}
	runDBAfterRestoreHooks()
	if err := os.Remove(fallbackPath); err != nil && !os.IsNotExist(err) {
		logger.Warning("remove imported db fallback file failed: ", err)
	}

	if err := SendSighup(); err != nil {
		return common.NewErrorf("Error restarting app: %v", err)
	}

	return nil
}

func RestoreDBBackupArchive(file multipart.File, panelRestarter func() error, stopRunningCores func() error) (err error) {
	if file == nil {
		return common.NewError("未选择备份文件")
	}
	if panelRestarter == nil {
		return common.NewError("面板重启回调不可用")
	}
	databaseImportRestoreMu.Lock()
	defer databaseImportRestoreMu.Unlock()
	defer func() {
		if err != nil {
			runDBRestoreAbortHooks()
		}
	}()
	if err := runDBBeforeRestoreHooks(); err != nil {
		return err
	}
	if err := ensureNoPendingDBRestore(); err != nil {
		return err
	}

	archiveData, err := readReaderWithLimit(file, dbBackupArchiveMaxBytes, "备份文件")
	if err != nil {
		return common.NewErrorf("读取备份文件失败: %v", err)
	}

	entries, err := readDBBackupArchiveEntries(archiveData)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return common.NewError("备份压缩包中没有 db 文件")
	}

	stageRoot := pendingDBRestoreBaseDir()
	stageDir := filepath.Join(stageRoot, fmt.Sprintf("stage-%d", time.Now().UnixNano()))
	cleanupStage := func() {
		_ = os.RemoveAll(stageDir)
		cleanupPendingDBRestoreBaseDir()
	}

	if err := os.MkdirAll(stageDir, 0o740); err != nil {
		return common.NewErrorf("创建恢复临时目录失败: %v", err)
	}

	if err := extractDBArchiveEntries(entries, stageDir); err != nil {
		cleanupStage()
		return err
	}

	if err := validateRestoredDatabaseSet(stageDir); err != nil {
		cleanupStage()
		return err
	}

	if stopRunningCores != nil {
		if err := stopRunningCores(); err != nil {
			cleanupStage()
			return err
		}
	}

	if err := writePendingDBRestoreMarker(&pendingDBRestoreMarker{StageDir: stageDir}); err != nil {
		cleanupStage()
		return common.NewErrorf("写入恢复任务失败: %v", err)
	}
	if err := panelRestarter(); err != nil {
		clearPendingDBRestoreMarker()
		cleanupStage()
		return common.NewErrorf("重启面板失败: %v", err)
	}

	return nil
}

func HasPendingDBRestore() bool {
	marker, err := readPendingDBRestoreMarker()
	return err == nil && marker != nil
}

func HasPendingDBRestoreToApply() bool {
	marker, err := readPendingDBRestoreMarker()
	return err == nil && marker != nil && !marker.Applied
}

func HasPendingDBRestoreToFinalize() bool {
	marker, err := readPendingDBRestoreMarker()
	return err == nil && marker != nil && marker.Applied
}

func ApplyPendingDBRestore() error {
	databaseImportRestoreMu.Lock()
	defer databaseImportRestoreMu.Unlock()

	marker, err := readPendingDBRestoreMarker()
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	baseDir := filepath.Clean(pendingDBRestoreBaseDir())
	stageDir := filepath.Clean(strings.TrimSpace(marker.StageDir))
	if !isPathWithinBase(stageDir, baseDir, false) {
		clearPendingDBRestoreMarker()
		cleanupPendingDBRestoreStage(stageDir, baseDir)
		cleanupPendingDBRestoreBaseDir()
		return common.NewError("恢复任务目录非法")
	}
	if err := validateRestoredDatabaseSet(stageDir); err != nil {
		clearPendingDBRestoreMarker()
		cleanupPendingDBRestoreStage(stageDir, baseDir)
		cleanupPendingDBRestoreBaseDir()
		return err
	}

	dbDir := config.GetDBFolderPath()
	backupDir := filepath.Join(baseDir, fmt.Sprintf("backup-%d", time.Now().UnixNano()))
	currentExists := fileExists(dbDir)

	if err := closeMainDatabase(); err != nil {
		clearPendingDBRestoreMarker()
		_ = os.RemoveAll(stageDir)
		cleanupPendingDBRestoreBaseDir()
		return common.NewErrorf("关闭主数据库失败: %v", err)
	}

	if currentExists {
		if err := os.Rename(dbDir, backupDir); err != nil {
			clearPendingDBRestoreMarker()
			_ = os.RemoveAll(stageDir)
			cleanupPendingDBRestoreBaseDir()
			_ = InitDB(config.GetDBPath())
			return common.NewErrorf("备份当前 db 目录失败: %v", err)
		}
	}

	if err := os.RemoveAll(dbDir); err != nil {
		clearPendingDBRestoreMarker()
		_ = os.RemoveAll(stageDir)
		cleanupPendingDBRestoreBaseDir()
		if rollbackErr := rollbackPendingDBRestore(currentExists, dbDir, backupDir); rollbackErr != nil {
			return common.NewErrorf("清理旧 db 目录失败: %v；回滚失败: %v", err, rollbackErr)
		}
		return common.NewErrorf("清理旧 db 目录失败: %v", err)
	}
	if err := os.Rename(stageDir, dbDir); err != nil {
		clearPendingDBRestoreMarker()
		_ = os.RemoveAll(stageDir)
		cleanupPendingDBRestoreBaseDir()
		if rollbackErr := rollbackPendingDBRestore(currentExists, dbDir, backupDir); rollbackErr != nil {
			return common.NewErrorf("替换 db 目录失败: %v；回滚失败: %v", err, rollbackErr)
		}
		return common.NewErrorf("替换 db 目录失败: %v", err)
	}

	if err := migration.MigrateDbWithError(); err != nil {
		clearPendingDBRestoreMarker()
		cleanupPendingDBRestoreBaseDir()
		if rollbackErr := rollbackPendingDBRestore(currentExists, dbDir, backupDir); rollbackErr != nil {
			return common.NewErrorf("迁移恢复后的数据库失败: %v；回滚失败: %v", err, rollbackErr)
		}
		return common.NewErrorf("迁移恢复后的数据库失败: %v", err)
	}
	if err := InitDB(config.GetDBPath()); err != nil {
		clearPendingDBRestoreMarker()
		cleanupPendingDBRestoreBaseDir()
		if rollbackErr := rollbackPendingDBRestore(currentExists, dbDir, backupDir); rollbackErr != nil {
			return common.NewErrorf("重新初始化主数据库失败: %v；回滚失败: %v", err, rollbackErr)
		}
		return common.NewErrorf("重新初始化主数据库失败: %v", err)
	}
	// The staged database is active only after InitDB succeeds. Run restore
	// hooks here, not before the panel restart request, so runtime sidecars are
	// invalidated against the restored database generation rather than the old
	// one that is about to be replaced.
	runDBAfterRestoreHooks()

	marker.StageDir = dbDir
	marker.BackupDir = backupDir
	marker.Applied = true
	if err := writePendingDBRestoreMarker(marker); err != nil {
		clearPendingDBRestoreMarker()
		if rollbackErr := rollbackPendingDBRestore(currentExists, dbDir, backupDir); rollbackErr != nil {
			return common.NewErrorf("写入恢复完成标记失败: %v；回滚失败: %v", err, rollbackErr)
		}
		return common.NewErrorf("写入恢复完成标记失败: %v", err)
	}
	return nil
}

func FinalizePendingDBRestore() error {
	databaseImportRestoreMu.Lock()
	defer databaseImportRestoreMu.Unlock()

	marker, err := readPendingDBRestoreMarker()
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if marker == nil || !marker.Applied {
		return nil
	}
	return finalizePendingDBRestore(marker, filepath.Clean(pendingDBRestoreBaseDir()))
}

func ensureNoPendingDBRestore() error {
	marker, err := readPendingDBRestoreMarker()
	if err == nil && marker != nil {
		return common.NewError("已有待处理的备份恢复任务，请等待当前恢复完成后再试")
	}
	if err != nil && !os.IsNotExist(err) {
		return common.NewErrorf("读取待处理备份恢复任务失败: %v", err)
	}
	return nil
}

func readReaderWithLimit(reader io.Reader, maxBytes int64, label string) ([]byte, error) {
	if reader == nil {
		return nil, fmt.Errorf("%s is empty", label)
	}
	if maxBytes <= 0 {
		return nil, fmt.Errorf("%s size limit must be positive", label)
	}
	data, err := io.ReadAll(io.LimitReader(reader, maxBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxBytes {
		return nil, fmt.Errorf("%s exceeds %d bytes", label, maxBytes)
	}
	return data, nil
}

func copyReaderWithLimit(dst io.Writer, src io.Reader, maxBytes int64) (int64, error) {
	if dst == nil || src == nil {
		return 0, fmt.Errorf("source or destination is nil")
	}
	if maxBytes <= 0 {
		return 0, fmt.Errorf("copy size limit must be positive")
	}
	written, err := io.Copy(dst, io.LimitReader(src, maxBytes+1))
	if err != nil {
		return written, err
	}
	if written > maxBytes {
		return written, fmt.Errorf("input exceeds %d bytes", maxBytes)
	}
	return written, nil
}

func validateSQLiteDatabaseFile(path string) error {
	tempDB, err := gorm.Open(sqlite.Open(path), &gorm.Config{})
	if err != nil {
		return err
	}
	sqlDB, err := tempDB.DB()
	if err != nil {
		return err
	}
	defer sqlDB.Close()
	if err := sqlDB.Ping(); err != nil {
		return err
	}
	return tempDB.Exec("PRAGMA schema_version").Error
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
	process, err := os.FindProcess(os.Getpid())
	if err != nil {
		return err
	}

	go func() {
		time.Sleep(3 * time.Second)
		// The process target OS is fixed by the running binary. Do not acquire
		// the single SQLite connection during database recovery merely to choose
		// how this process is restarted.
		var signalErr error
		if runtime.GOOS == "windows" {
			signalErr = process.Kill()
		} else {
			signalErr = process.Signal(syscall.SIGHUP)
		}
		if signalErr != nil {
			logger.Error("send panel restart signal failed:", signalErr)
		}
	}()
	return nil
}

func createDBBackupSnapshots(dbDir string) (string, []string, error) {
	runtimeDir := filepath.Join(config.GetDataDir(), "runtime")
	if err := os.MkdirAll(runtimeDir, 0o740); err != nil {
		return "", nil, err
	}

	snapshotDir, err := os.MkdirTemp(runtimeDir, "db-backup-")
	if err != nil {
		return "", nil, err
	}

	items, err := collectBackupSnapshotFiles(dbDir, snapshotDir)
	if err != nil {
		_ = os.RemoveAll(snapshotDir)
		return "", nil, err
	}
	return snapshotDir, items, nil
}

func collectBackupSourceFiles(dbDir string) ([]string, error) {
	entries, err := os.ReadDir(dbDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := strings.TrimSpace(entry.Name())
		if name == "" {
			continue
		}
		lower := strings.ToLower(name)
		if !strings.HasSuffix(lower, ".db") && !strings.HasSuffix(lower, "-wal") && !strings.HasSuffix(lower, "-shm") {
			continue
		}
		files = append(files, filepath.Join(dbDir, name))
	}
	return files, nil
}

func collectBackupSnapshotFiles(dbDir string, snapshotDir string) ([]string, error) {
	sourceFiles, err := collectBackupSourceFiles(dbDir)
	if err != nil {
		return nil, err
	}

	files := make([]string, 0, len(sourceFiles))
	for _, sourcePath := range sourceFiles {
		name := strings.TrimSpace(filepath.Base(sourcePath))
		if name == "" {
			continue
		}

		lower := strings.ToLower(name)
		if strings.HasSuffix(lower, "-wal") || strings.HasSuffix(lower, "-shm") {
			continue
		}

		targetPath := filepath.Join(snapshotDir, name)
		if strings.HasSuffix(lower, ".db") {
			if err := createSQLiteSnapshot(sourcePath, targetPath); err != nil {
				return nil, common.NewErrorf("创建数据库快照失败 %s: %v", name, err)
			}
			files = append(files, targetPath)
		}
	}
	return files, nil
}

func createSQLiteSnapshot(sourcePath string, targetPath string) error {
	sourceDB, err := gorm.Open(sqlite.Open(sqliteDSNWithPragmas(sourcePath)), &gorm.Config{})
	if err != nil {
		return err
	}
	sqlDB, dbErr := sourceDB.DB()
	if dbErr == nil {
		defer sqlDB.Close()
	}

	if err := os.MkdirAll(filepath.Dir(targetPath), 0o740); err != nil {
		return err
	}
	sql := fmt.Sprintf("VACUUM INTO '%s'", escapeSQLiteLiteral(filepath.ToSlash(targetPath)))
	if err := sourceDB.Exec(sql).Error; err != nil {
		return err
	}
	// This is a copy-only operation on the snapshot. The forwarding sysctl
	// ownership record is host-local runtime evidence, so it must not survive
	// either lightweight exports or full archive backups.
	return removePortForwardKernelForwardStateFromSnapshot(targetPath)
}

func removePortForwardKernelForwardStateFromSnapshot(path string) error {
	snapshotDB, err := gorm.Open(sqlite.Open(sqliteDSNWithPragmas(path)), &gorm.Config{})
	if err != nil {
		return err
	}
	sqlDB, err := snapshotDB.DB()
	if err != nil {
		return err
	}
	defer sqlDB.Close()
	if !snapshotDB.Migrator().HasTable(&model.PortForwardKernelForwardState{}) {
		return nil
	}
	return snapshotDB.Migrator().DropTable(&model.PortForwardKernelForwardState{})
}

func escapeSQLiteLiteral(value string) string {
	return strings.ReplaceAll(value, "'", "''")
}

func closeMainDatabase() error {
	currentDB := GetDB()
	if currentDB == nil {
		return nil
	}
	sqlDB, err := currentDB.DB()
	if err != nil {
		return err
	}
	// Keep the closed handle published until InitDB swaps in the replacement.
	// Concurrent readers then receive the driver's normal "database is closed"
	// error instead of dereferencing a transient nil global pointer.
	return sqlDB.Close()
}

func removeIfExists(path string) error {
	if _, err := os.Stat(path); err == nil {
		return os.Remove(path)
	} else if os.IsNotExist(err) {
		return nil
	} else {
		return err
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func isPathWithinBase(targetPath string, basePath string, allowBase bool) bool {
	targetPath = filepath.Clean(strings.TrimSpace(targetPath))
	basePath = filepath.Clean(strings.TrimSpace(basePath))
	if targetPath == "" || basePath == "" {
		return false
	}

	rel, err := filepath.Rel(basePath, targetPath)
	if err != nil {
		return false
	}
	rel = filepath.Clean(rel)
	if rel == "." {
		return allowBase
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}

func cleanupPendingDBRestoreStage(stageDir string, baseDir string) {
	if !isPathWithinBase(stageDir, baseDir, false) {
		return
	}
	_ = os.RemoveAll(stageDir)
}

func rollbackPendingDBRestore(currentExists bool, dbDir string, backupDir string) error {
	if currentExists && fileExists(backupDir) {
		_ = os.RemoveAll(dbDir)
		if err := os.Rename(backupDir, dbDir); err != nil {
			return err
		}
	}
	if fileExists(dbDir) {
		if err := InitDB(config.GetDBPath()); err != nil {
			return err
		}
	}
	return nil
}

func finalizePendingDBRestore(marker *pendingDBRestoreMarker, baseDir string) error {
	backupDir := filepath.Clean(strings.TrimSpace(marker.BackupDir))
	if backupDir == "" {
		clearPendingDBRestoreMarker()
		cleanupPendingDBRestoreBaseDir()
		return nil
	}
	if !isPathWithinBase(backupDir, baseDir, false) {
		clearPendingDBRestoreMarker()
		return common.NewError("恢复备份目录非法")
	}
	if err := os.RemoveAll(backupDir); err != nil {
		return common.NewErrorf("清理旧数据库备份失败: %v", err)
	}
	clearPendingDBRestoreMarker()
	cleanupPendingDBRestoreBaseDir()
	return nil
}

func readDBBackupArchiveEntries(data []byte) (map[string][]byte, error) {
	return readDBBackupArchiveEntriesWithLimits(data, defaultDBBackupArchiveLimits)
}

func readDBBackupArchiveEntriesWithLimits(data []byte, limits dbBackupArchiveLimits) (map[string][]byte, error) {
	if limits.maxEntries <= 0 || limits.maxEntryBytes <= 0 || limits.maxTotalBytes <= 0 {
		return nil, common.NewError("备份压缩包大小限制无效")
	}
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, common.NewErrorf("备份文件不是有效的 zip 压缩包: %v", err)
	}

	entries := make(map[string][]byte)
	seenNames := make(map[string]struct{})
	entryCount := 0
	totalBytes := int64(0)
	for _, file := range reader.File {
		if file.FileInfo().IsDir() {
			continue
		}
		if entryCount >= limits.maxEntries {
			return nil, common.NewErrorf("备份压缩包文件数量超过上限 %d", limits.maxEntries)
		}

		name, err := normalizeDBBackupArchiveEntryName(file.Name)
		if err != nil {
			return nil, err
		}
		lowerName := strings.ToLower(name)
		if _, exists := seenNames[lowerName]; exists {
			return nil, common.NewErrorf("备份压缩包包含重复文件: %s", name)
		}
		seenNames[lowerName] = struct{}{}
		if file.UncompressedSize64 > uint64(limits.maxEntryBytes) {
			return nil, common.NewErrorf("备份压缩包文件过大: %s", name)
		}
		if totalBytes > limits.maxTotalBytes-int64(file.UncompressedSize64) {
			return nil, common.NewErrorf("备份压缩包解压后总大小超过上限 %d", limits.maxTotalBytes)
		}

		rc, err := file.Open()
		if err != nil {
			return nil, common.NewErrorf("读取压缩包文件失败 %s: %v", name, err)
		}
		remainingBytes := limits.maxTotalBytes - totalBytes
		entryLimit := limits.maxEntryBytes
		if entryLimit > remainingBytes {
			entryLimit = remainingBytes
		}
		content, readErr := readReaderWithLimit(rc, entryLimit, "备份压缩包文件")
		closeErr := rc.Close()
		if readErr != nil {
			return nil, common.NewErrorf("读取压缩包文件失败 %s: %v", name, readErr)
		}
		if closeErr != nil {
			return nil, common.NewErrorf("关闭压缩包文件失败 %s: %v", name, closeErr)
		}
		entries[name] = content
		entryCount++
		totalBytes += int64(len(content))
	}

	return entries, nil
}

func normalizeDBBackupArchiveEntryName(rawName string) (string, error) {
	name := filepath.ToSlash(strings.TrimSpace(rawName))
	if name == "" {
		return "", common.NewError("备份压缩包包含空文件名")
	}
	if strings.HasPrefix(name, "/") || strings.Contains(name, "../") {
		return "", common.NewErrorf("备份压缩包包含非法路径: %s", name)
	}
	if strings.Contains(name, "/") || strings.Contains(name, ":") {
		return "", common.NewErrorf("备份压缩包只允许包含 db 目录根层级文件: %s", name)
	}
	if !strings.HasSuffix(strings.ToLower(name), ".db") {
		return "", common.NewErrorf("备份压缩包包含不支持的文件类型: %s", name)
	}
	return name, nil
}

func extractDBArchiveEntries(entries map[string][]byte, stageDir string) error {
	baseDir := filepath.Clean(stageDir)
	for name, content := range entries {
		targetPath := filepath.Join(stageDir, filepath.FromSlash(name))
		cleanTarget := filepath.Clean(targetPath)
		if !isPathWithinBase(cleanTarget, baseDir, false) {
			return common.NewErrorf("备份压缩包包含越界路径: %s", name)
		}
		if err := os.MkdirAll(filepath.Dir(cleanTarget), 0o740); err != nil {
			return common.NewErrorf("创建恢复目录失败 %s: %v", name, err)
		}
		if err := os.WriteFile(cleanTarget, content, 0o640); err != nil {
			return common.NewErrorf("写入恢复文件失败 %s: %v", name, err)
		}
	}
	return nil
}

func validateRestoredDatabaseSet(stageDir string) error {
	mainDBPath := filepath.Join(stageDir, filepath.Base(config.GetDBPath()))
	if !fileExists(mainDBPath) {
		return common.NewErrorf("备份压缩包缺少主数据库文件 %s", filepath.Base(config.GetDBPath()))
	}

	file, err := os.Open(mainDBPath)
	if err != nil {
		return common.NewErrorf("打开主数据库失败: %v", err)
	}
	isSQLite, checkErr := IsSQLiteDB(file)
	closeErr := file.Close()
	if checkErr != nil {
		return common.NewErrorf("校验主数据库失败: %v", checkErr)
	}
	if closeErr != nil {
		return common.NewErrorf("关闭主数据库失败: %v", closeErr)
	}
	if !isSQLite {
		return common.NewError("备份中的主数据库不是有效的 SQLite 文件")
	}

	if err := validateSQLiteDatabaseFile(mainDBPath); err != nil {
		return common.NewErrorf("备份中的主数据库无法打开: %v", err)
	}

	return nil
}

func pendingDBRestoreBaseDir() string {
	return filepath.Join(config.GetDataDir(), "runtime", "db-restore")
}

func pendingDBRestoreMarkerPath() string {
	return filepath.Join(pendingDBRestoreBaseDir(), "pending.json")
}

func writePendingDBRestoreMarker(marker *pendingDBRestoreMarker) error {
	if marker == nil {
		return common.NewError("恢复任务标记为空")
	}
	if err := os.MkdirAll(pendingDBRestoreBaseDir(), 0o740); err != nil {
		return err
	}
	raw, err := json.Marshal(marker)
	if err != nil {
		return err
	}
	return os.WriteFile(pendingDBRestoreMarkerPath(), raw, 0o640)
}

func readPendingDBRestoreMarker() (*pendingDBRestoreMarker, error) {
	raw, err := os.ReadFile(pendingDBRestoreMarkerPath())
	if err != nil {
		return nil, err
	}
	var payload pendingDBRestoreMarker
	if err := json.Unmarshal(raw, &payload); err != nil {
		return nil, err
	}
	return &payload, nil
}

func clearPendingDBRestoreMarker() {
	_ = os.Remove(pendingDBRestoreMarkerPath())
}

func cleanupPendingDBRestoreBaseDir() {
	entries, err := os.ReadDir(pendingDBRestoreBaseDir())
	if err != nil {
		return
	}
	if len(entries) == 0 {
		_ = os.Remove(pendingDBRestoreBaseDir())
	}
}
