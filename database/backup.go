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

type pendingDBRestoreMarker struct {
	StageDir  string `json:"stageDir"`
	BackupDir string `json:"backupDir,omitempty"`
	Applied   bool   `json:"applied,omitempty"`
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

	bdb, _ := backupDb.DB()
	_ = bdb.Close()

	file, err := os.Open(dbPath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	fileContents, err := io.ReadAll(file)
	if err != nil {
		return nil, err
	}

	return fileContents, nil
}

func BuildDBBackupArchive() (*DBBackupArchive, error) {
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

	var buffer bytes.Buffer
	zipWriter := zip.NewWriter(&buffer)

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
		_, copyErr := io.Copy(writer, file)
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

func ImportDB(file multipart.File) error {
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
	defer tempFile.Close()
	defer os.Remove(tempPath)

	if err := closeMainDatabase(); err != nil {
		return common.NewErrorf("Error closing existing db: %v", err)
	}

	if _, err = io.Copy(tempFile, file); err != nil {
		return common.NewErrorf("Error saving db: %v", err)
	}

	newDb, err := gorm.Open(sqlite.Open(tempPath), &gorm.Config{})
	if err != nil {
		return common.NewErrorf("Error checking db: %v", err)
	}
	newDBHandle, _ := newDb.DB()
	_ = newDBHandle.Close()

	fallbackPath := fmt.Sprintf("%s.backup", config.GetDBPath())
	if err := removeIfExists(fallbackPath); err != nil {
		return common.NewErrorf("Error removing existing fallback db file: %v", err)
	}

	if err := os.Rename(config.GetDBPath(), fallbackPath); err != nil {
		return common.NewErrorf("Error backing up temporary db file: %v", err)
	}
	defer os.Remove(fallbackPath)

	if err := os.Rename(tempPath, config.GetDBPath()); err != nil {
		if errRename := os.Rename(fallbackPath, config.GetDBPath()); errRename != nil {
			return common.NewErrorf("Error moving db file and restoring fallback: %v", errRename)
		}
		return common.NewErrorf("Error moving db file: %v", err)
	}

	if err := migration.MigrateDbWithError(); err != nil {
		if errRename := os.Rename(fallbackPath, config.GetDBPath()); errRename != nil {
			return common.NewErrorf("Error migrating db and restoring fallback: %v", errRename)
		}
		return common.NewErrorf("Error migrating db: %v", err)
	}
	if err := InitDB(config.GetDBPath()); err != nil {
		if errRename := os.Rename(fallbackPath, config.GetDBPath()); errRename != nil {
			return common.NewErrorf("Error migrating db and restoring fallback: %v", errRename)
		}
		return common.NewErrorf("Error migrating db: %v", err)
	}

	if err := SendSighup(); err != nil {
		return common.NewErrorf("Error restarting app: %v", err)
	}

	return nil
}

func RestoreDBBackupArchive(file multipart.File, panelRestarter func() error, stopRunningCores func() error) error {
	if file == nil {
		return common.NewError("未选择备份文件")
	}
	if panelRestarter == nil {
		return common.NewError("面板重启回调不可用")
	}
	if HasPendingDBRestore() {
		return common.NewError("已有待处理的备份恢复任务，请等待当前恢复完成后再试")
	}

	archiveData, err := io.ReadAll(file)
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
	if err := runBeforeDBRestoreHooks(); err != nil {
		clearPendingDBRestoreMarker()
		_ = os.RemoveAll(stageDir)
		cleanupPendingDBRestoreBaseDir()
		_ = InitDB(config.GetDBPath())
		_ = runAfterDBRestoreHooks()
		return common.NewErrorf("执行恢复前清理失败: %v", err)
	}

	if currentExists {
		if err := os.Rename(dbDir, backupDir); err != nil {
			clearPendingDBRestoreMarker()
			_ = os.RemoveAll(stageDir)
			cleanupPendingDBRestoreBaseDir()
			_ = InitDB(config.GetDBPath())
			_ = runAfterDBRestoreHooks()
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
	if err := runAfterDBRestoreHooks(); err != nil {
		clearPendingDBRestoreMarker()
		cleanupPendingDBRestoreBaseDir()
		if rollbackErr := rollbackPendingDBRestore(currentExists, dbDir, backupDir); rollbackErr != nil {
			return common.NewErrorf("执行恢复后初始化失败: %v；回滚失败: %v", err, rollbackErr)
		}
		return common.NewErrorf("执行恢复后初始化失败: %v", err)
	}

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
	return sourceDB.Exec(sql).Error
}

func escapeSQLiteLiteral(value string) string {
	return strings.ReplaceAll(value, "'", "''")
}

func closeMainDatabase() error {
	if db == nil {
		return nil
	}
	sqlDB, err := db.DB()
	if err != nil {
		db = nil
		return err
	}
	db = nil
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
		if err := runAfterDBRestoreHooks(); err != nil {
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
	reader, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, common.NewErrorf("备份文件不是有效的 zip 压缩包: %v", err)
	}

	entries := make(map[string][]byte)
	seenNames := make(map[string]struct{})
	for _, file := range reader.File {
		if file.FileInfo().IsDir() {
			continue
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

		rc, err := file.Open()
		if err != nil {
			return nil, common.NewErrorf("读取压缩包文件失败 %s: %v", name, err)
		}
		content, readErr := io.ReadAll(rc)
		closeErr := rc.Close()
		if readErr != nil {
			return nil, common.NewErrorf("读取压缩包文件失败 %s: %v", name, readErr)
		}
		if closeErr != nil {
			return nil, common.NewErrorf("关闭压缩包文件失败 %s: %v", name, closeErr)
		}
		entries[name] = content
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

	tempDB, err := gorm.Open(sqlite.Open(mainDBPath), &gorm.Config{})
	if err != nil {
		return common.NewErrorf("备份中的主数据库无法打开: %v", err)
	}
	sqlDB, dbErr := tempDB.DB()
	if dbErr == nil {
		_ = sqlDB.Close()
	}

	monitorDBPath := filepath.Join(stageDir, filepath.Base(config.GetSystemMonitorDBPath()))
	if fileExists(monitorDBPath) {
		monitorFile, err := os.Open(monitorDBPath)
		if err != nil {
			return common.NewErrorf("打开监控数据库失败: %v", err)
		}
		monitorSQLite, checkErr := IsSQLiteDB(monitorFile)
		closeErr := monitorFile.Close()
		if checkErr != nil {
			return common.NewErrorf("校验监控数据库失败: %v", checkErr)
		}
		if closeErr != nil {
			return common.NewErrorf("关闭监控数据库失败: %v", closeErr)
		}
		if !monitorSQLite {
			return common.NewError("备份中的监控数据库不是有效的 SQLite 文件")
		}
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
