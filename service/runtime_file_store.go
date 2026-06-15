package service

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/alireza0/s-ui/config"
	"github.com/alireza0/s-ui/database"
)

const managedRuntimeFileTable = "managed_runtime_files"

type managedRuntimeFileEntry struct {
	Path      string
	DirPath   string
	FileName  string
	Ext       string
	Content   []byte
	Size      int64
	UpdatedAt int64
}

type managedRuntimeFileStore struct {
	initMu         sync.Mutex
	initialized    bool
	migratedLegacy bool

	cacheMu sync.RWMutex
	cache   map[string]*managedRuntimeFileEntry

	timerMu sync.Mutex
	timers  map[string]*time.Timer
}

var runtimeManagedFiles = &managedRuntimeFileStore{
	cache:  make(map[string]*managedRuntimeFileEntry),
	timers: make(map[string]*time.Timer),
}

func init() {
	database.RegisterDBResetHook(func() {
		runtimeManagedFiles.resetForDatabaseReload()
	})
}

func InitManagedRuntimeFileStore() error {
	return runtimeManagedFiles.ensureReady()
}

func ManagedRuntimeWriteFile(filePath string, data []byte) error {
	if err := runtimeManagedFiles.ensureReady(); err != nil {
		return err
	}

	canonical, managed := canonicalManagedRuntimePath(filePath)
	if !managed {
		return os.WriteFile(filePath, data, 0o644)
	}

	return runtimeManagedFiles.put(canonical, data)
}

func ManagedRuntimeReadFile(filePath string) ([]byte, error) {
	if err := runtimeManagedFiles.ensureReady(); err != nil {
		return nil, err
	}

	canonical, managed := canonicalManagedRuntimePath(filePath)
	if !managed {
		return os.ReadFile(filePath)
	}

	return runtimeManagedFiles.read(canonical)
}

func ManagedRuntimeFileExists(filePath string) (bool, error) {
	if err := runtimeManagedFiles.ensureReady(); err != nil {
		return false, err
	}

	canonical, managed := canonicalManagedRuntimePath(filePath)
	if !managed {
		_, err := os.Stat(filePath)
		if err == nil {
			return true, nil
		}
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}

	return runtimeManagedFiles.exists(canonical)
}

func ManagedRuntimeDeleteFile(filePath string) error {
	if err := runtimeManagedFiles.ensureReady(); err != nil {
		return err
	}

	canonical, managed := canonicalManagedRuntimePath(filePath)
	if !managed {
		err := os.Remove(filePath)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}

	return runtimeManagedFiles.delete(canonical)
}

func ManagedRuntimeClearDirJSONFiles(dirPath string, keepFiles ...string) error {
	if err := runtimeManagedFiles.ensureReady(); err != nil {
		return err
	}

	canonicalDir, managed := canonicalManagedRuntimePath(dirPath)
	if !managed {
		if err := os.MkdirAll(dirPath, 0o755); err != nil {
			return err
		}
		entries, err := os.ReadDir(dirPath)
		if err != nil {
			return err
		}
		keep := make(map[string]struct{}, len(keepFiles))
		for _, name := range keepFiles {
			keep[name] = struct{}{}
		}
		for _, entry := range entries {
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
				continue
			}
			if _, skip := keep[entry.Name()]; skip {
				continue
			}
			_ = os.Remove(filepath.Join(dirPath, entry.Name()))
		}
		return nil
	}

	return runtimeManagedFiles.clearDirJSONFiles(canonicalDir, keepFiles...)
}

func MaterializeManagedRuntimeCoreFile(filePath string, ttl time.Duration) error {
	if ttl <= 0 {
		ttl = 5 * time.Second
	}

	if err := runtimeManagedFiles.ensureReady(); err != nil {
		return err
	}

	canonical, managed := canonicalManagedRuntimePath(filePath)
	if !managed || !isManagedRuntimeTempCoreFile(canonical) {
		_, err := os.Stat(filePath)
		return err
	}

	content, err := runtimeManagedFiles.read(canonical)
	if err != nil {
		return err
	}

	diskPath := managedRuntimeDiskPath(canonical)
	if err := os.MkdirAll(filepath.Dir(diskPath), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(diskPath, content, 0o600); err != nil {
		return err
	}

	runtimeManagedFiles.scheduleCleanup(canonical, ttl)
	return nil
}

func DiscardMaterializedManagedRuntimeCoreFile(filePath string) {
	canonical, managed := canonicalManagedRuntimePath(filePath)
	if !managed || !isManagedRuntimeTempCoreFile(canonical) {
		_ = os.Remove(filePath)
		return
	}

	runtimeManagedFiles.cancelCleanup(canonical)
	_ = os.Remove(managedRuntimeDiskPath(canonical))
}

func (s *managedRuntimeFileStore) ensureReady() error {
	s.initMu.Lock()
	defer s.initMu.Unlock()

	if s.initialized && s.migratedLegacy {
		return nil
	}

	db := database.GetDB()
	if db == nil {
		return fmt.Errorf("managed runtime file store requires initialized database")
	}

	if !s.initialized {
		statements := []string{
			fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
				path TEXT PRIMARY KEY,
				dir_path TEXT NOT NULL,
				file_name TEXT NOT NULL,
				ext TEXT NOT NULL,
				content BLOB NOT NULL,
				size INTEGER NOT NULL DEFAULT 0,
				updated_at INTEGER NOT NULL
			)`, managedRuntimeFileTable),
			fmt.Sprintf(`CREATE INDEX IF NOT EXISTS idx_%s_dir_path ON %s (dir_path)`, managedRuntimeFileTable, managedRuntimeFileTable),
		}
		for _, stmt := range statements {
			if err := db.Exec(stmt).Error; err != nil {
				return err
			}
		}
		s.initialized = true
	}

	if !s.migratedLegacy {
		if err := EnsureManagedCoreLayout(); err != nil {
			return err
		}
		if err := s.migrateLegacyFiles(); err != nil {
			return err
		}
		s.migratedLegacy = true
	}

	return nil
}

func (s *managedRuntimeFileStore) resetForDatabaseReload() {
	s.initMu.Lock()
	s.initialized = false
	s.migratedLegacy = false
	s.initMu.Unlock()

	s.cacheMu.Lock()
	s.cache = make(map[string]*managedRuntimeFileEntry)
	s.cacheMu.Unlock()

	s.timerMu.Lock()
	timers := s.timers
	s.timers = make(map[string]*time.Timer)
	s.timerMu.Unlock()

	for canonical, timer := range timers {
		if timer != nil {
			timer.Stop()
		}
		if isManagedRuntimeTempCoreFile(canonical) {
			_ = os.Remove(managedRuntimeDiskPath(canonical))
		}
	}
}

func (s *managedRuntimeFileStore) put(canonical string, data []byte) error {
	entry := newManagedRuntimeFileEntry(canonical, data)
	db := database.GetDB()
	if db == nil {
		return fmt.Errorf("database is not initialized")
	}

	stmt := fmt.Sprintf(`INSERT INTO %s (path, dir_path, file_name, ext, content, size, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(path) DO UPDATE SET
			dir_path = excluded.dir_path,
			file_name = excluded.file_name,
			ext = excluded.ext,
			content = excluded.content,
			size = excluded.size,
			updated_at = excluded.updated_at`, managedRuntimeFileTable)

	if err := db.Exec(
		stmt,
		entry.Path,
		entry.DirPath,
		entry.FileName,
		entry.Ext,
		entry.Content,
		entry.Size,
		entry.UpdatedAt,
	).Error; err != nil {
		return err
	}

	s.cacheMu.Lock()
	s.cache[canonical] = cloneManagedRuntimeEntry(entry)
	s.cacheMu.Unlock()

	if s.hasActiveCleanup(canonical) && isManagedRuntimeTempCoreFile(canonical) {
		diskPath := managedRuntimeDiskPath(canonical)
		if err := os.MkdirAll(filepath.Dir(diskPath), 0o755); err == nil {
			_ = os.WriteFile(diskPath, data, 0o600)
		}
		return nil
	}

	_ = os.Remove(managedRuntimeDiskPath(canonical))
	return nil
}

func (s *managedRuntimeFileStore) read(canonical string) ([]byte, error) {
	s.cacheMu.RLock()
	if cached, ok := s.cache[canonical]; ok && cached != nil {
		content := append([]byte(nil), cached.Content...)
		s.cacheMu.RUnlock()
		return content, nil
	}
	s.cacheMu.RUnlock()

	db := database.GetDB()
	if db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}

	entry := &managedRuntimeFileEntry{}
	row := db.Raw(
		fmt.Sprintf(`SELECT path, dir_path, file_name, ext, content, size, updated_at FROM %s WHERE path = ?`, managedRuntimeFileTable),
		canonical,
	).Row()
	if err := row.Scan(
		&entry.Path,
		&entry.DirPath,
		&entry.FileName,
		&entry.Ext,
		&entry.Content,
		&entry.Size,
		&entry.UpdatedAt,
	); err == nil {
		s.cacheMu.Lock()
		s.cache[canonical] = cloneManagedRuntimeEntry(entry)
		s.cacheMu.Unlock()
		return append([]byte(nil), entry.Content...), nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	diskPath := managedRuntimeDiskPath(canonical)
	data, err := os.ReadFile(diskPath)
	if err != nil {
		return nil, fmt.Errorf("managed runtime file %s not found", canonical)
	}

	if putErr := s.put(canonical, data); putErr != nil {
		return nil, putErr
	}
	_ = os.Remove(diskPath)
	return append([]byte(nil), data...), nil
}

func (s *managedRuntimeFileStore) exists(canonical string) (bool, error) {
	s.cacheMu.RLock()
	_, ok := s.cache[canonical]
	s.cacheMu.RUnlock()
	if ok {
		return true, nil
	}

	exists, err := s.existsInDB(canonical)
	if err != nil {
		return false, err
	}
	if exists {
		return true, nil
	}

	diskPath := managedRuntimeDiskPath(canonical)
	_, err = os.Stat(diskPath)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return false, err
}

func (s *managedRuntimeFileStore) existsInDB(canonical string) (bool, error) {
	db := database.GetDB()
	if db == nil {
		return false, fmt.Errorf("database is not initialized")
	}

	var found int
	if err := db.Raw(
		fmt.Sprintf(`SELECT 1 FROM %s WHERE path = ? LIMIT 1`, managedRuntimeFileTable),
		canonical,
	).Scan(&found).Error; err != nil {
		return false, err
	}
	return found == 1, nil
}

func (s *managedRuntimeFileStore) delete(canonical string) error {
	db := database.GetDB()
	if db == nil {
		return fmt.Errorf("database is not initialized")
	}

	if err := db.Exec(
		fmt.Sprintf(`DELETE FROM %s WHERE path = ?`, managedRuntimeFileTable),
		canonical,
	).Error; err != nil {
		return err
	}

	s.cacheMu.Lock()
	delete(s.cache, canonical)
	s.cacheMu.Unlock()

	s.cancelCleanup(canonical)
	err := os.Remove(managedRuntimeDiskPath(canonical))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func (s *managedRuntimeFileStore) clearDirJSONFiles(canonicalDir string, keepFiles ...string) error {
	db := database.GetDB()
	if db == nil {
		return fmt.Errorf("database is not initialized")
	}

	keep := make(map[string]struct{}, len(keepFiles))
	for _, name := range keepFiles {
		keep[name] = struct{}{}
	}

	rows, err := db.Raw(
		fmt.Sprintf(`SELECT path, file_name FROM %s WHERE dir_path = ?`, managedRuntimeFileTable),
		canonicalDir,
	).Rows()
	if err != nil {
		return err
	}

	var pathsToDelete []string
	for rows.Next() {
		var filePath string
		var fileName string
		if scanErr := rows.Scan(&filePath, &fileName); scanErr != nil {
			_ = rows.Close()
			return scanErr
		}
		if path.Ext(fileName) != ".json" {
			continue
		}
		if _, skip := keep[fileName]; skip {
			continue
		}
		pathsToDelete = append(pathsToDelete, filePath)
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		_ = rows.Close()
		return rowsErr
	}
	// Close the result set before issuing DELETE statements. The SQLite pool is
	// intentionally limited to one connection, so keeping Rows open here can
	// block the follow-up writes indefinitely.
	if closeErr := rows.Close(); closeErr != nil {
		return closeErr
	}

	for _, filePath := range pathsToDelete {
		if err := s.delete(filePath); err != nil {
			return err
		}
	}

	diskDir := managedRuntimeDiskPath(canonicalDir)
	entries, err := os.ReadDir(diskDir)
	if err == nil {
		for _, entry := range entries {
			if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
				continue
			}
			if _, skip := keep[entry.Name()]; skip {
				continue
			}
			_ = os.Remove(filepath.Join(diskDir, entry.Name()))
		}
	}

	return nil
}

func (s *managedRuntimeFileStore) scheduleCleanup(canonical string, ttl time.Duration) {
	s.timerMu.Lock()
	defer s.timerMu.Unlock()

	if timer, ok := s.timers[canonical]; ok {
		timer.Stop()
	}

	diskPath := managedRuntimeDiskPath(canonical)
	s.timers[canonical] = time.AfterFunc(ttl, func() {
		_ = os.Remove(diskPath)
		s.timerMu.Lock()
		delete(s.timers, canonical)
		s.timerMu.Unlock()
	})
}

func (s *managedRuntimeFileStore) cancelCleanup(canonical string) {
	s.timerMu.Lock()
	defer s.timerMu.Unlock()

	if timer, ok := s.timers[canonical]; ok {
		timer.Stop()
		delete(s.timers, canonical)
	}
}

func (s *managedRuntimeFileStore) hasActiveCleanup(canonical string) bool {
	s.timerMu.Lock()
	defer s.timerMu.Unlock()
	_, ok := s.timers[canonical]
	return ok
}

func (s *managedRuntimeFileStore) migrateLegacyFiles() error {
	managedDirs := []string{"Inbound", "outbound", "sub_json", "sub_manager"}
	for _, dirName := range managedDirs {
		if err := s.migrateLegacyDir(dirName); err != nil {
			return err
		}
	}

	managedCoreFiles := []string{
		"core/singbox/config.json",
		"core/mihomo/server.yaml",
		"core/" + mihomoInboundMetaFilename,
	}
	for _, canonical := range managedCoreFiles {
		if err := s.migrateLegacyFile(canonical); err != nil {
			return err
		}
	}
	if err := s.migrateLegacyCoreCanonicalAlias("core/config.json", "core/singbox/config.json"); err != nil {
		return err
	}
	if err := s.migrateLegacyCoreCanonicalAlias("core/server.yaml", "core/mihomo/server.yaml"); err != nil {
		return err
	}

	return nil
}

func (s *managedRuntimeFileStore) migrateLegacyCoreCanonicalAlias(sourceCanonical, targetCanonical string) error {
	if sourceCanonical == "" || targetCanonical == "" || sourceCanonical == targetCanonical {
		return nil
	}

	targetExists, err := s.exists(targetCanonical)
	if err != nil {
		return err
	}

	sourceExistsInDB, err := s.existsInDB(sourceCanonical)
	if err != nil {
		return err
	}
	sourceDiskPath := managedRuntimeDiskPath(sourceCanonical)
	sourceDataOnDisk, readErr := os.ReadFile(sourceDiskPath)
	sourceExistsOnDisk := readErr == nil
	if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
		return readErr
	}

	if targetExists {
		if sourceExistsInDB {
			if err := s.delete(sourceCanonical); err != nil {
				return err
			}
		}
		if sourceExistsOnDisk {
			if err := os.Remove(sourceDiskPath); err != nil && !errors.Is(err, os.ErrNotExist) {
				return err
			}
		}
		return nil
	}

	if sourceExistsInDB {
		data, err := s.read(sourceCanonical)
		if err != nil {
			return err
		}
		if err := s.put(targetCanonical, data); err != nil {
			return err
		}
		if err := s.delete(sourceCanonical); err != nil {
			return err
		}
		return nil
	}

	if sourceExistsOnDisk {
		if err := s.put(targetCanonical, sourceDataOnDisk); err != nil {
			return err
		}
		if err := os.Remove(sourceDiskPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}

	return nil
}

func (s *managedRuntimeFileStore) migrateLegacyDir(dirName string) error {
	dirPath := filepath.Join(config.GetDataDir(), dirName)
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		fullPath := filepath.Join(dirPath, entry.Name())
		canonical, managed := canonicalManagedRuntimePath(fullPath)
		if !managed {
			continue
		}
		if err := s.migrateLegacyFile(canonical); err != nil {
			return err
		}
	}

	_ = os.Remove(dirPath)
	return nil
}

func (s *managedRuntimeFileStore) migrateLegacyFile(canonical string) error {
	existsInStore, err := s.existsInDB(canonical)
	if err != nil {
		return err
	}

	diskPath := managedRuntimeDiskPath(canonical)
	if existsInStore {
		err = os.Remove(diskPath)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		return nil
	}

	data, err := os.ReadFile(diskPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}

	if err := s.put(canonical, data); err != nil {
		return err
	}

	err = os.Remove(diskPath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func newManagedRuntimeFileEntry(canonical string, data []byte) *managedRuntimeFileEntry {
	return &managedRuntimeFileEntry{
		Path:      canonical,
		DirPath:   path.Dir(canonical),
		FileName:  path.Base(canonical),
		Ext:       path.Ext(canonical),
		Content:   append([]byte(nil), data...),
		Size:      int64(len(data)),
		UpdatedAt: time.Now().Unix(),
	}
}

func cloneManagedRuntimeEntry(entry *managedRuntimeFileEntry) *managedRuntimeFileEntry {
	if entry == nil {
		return nil
	}
	cloned := *entry
	cloned.Content = append([]byte(nil), entry.Content...)
	return &cloned
}

func canonicalManagedRuntimePath(rawPath string) (string, bool) {
	rawPath = strings.TrimSpace(rawPath)
	if rawPath == "" {
		return "", false
	}

	cleaned := filepath.Clean(rawPath)
	if filepath.IsAbs(cleaned) {
		dataDir := filepath.Clean(config.GetDataDir())
		rel, err := filepath.Rel(dataDir, cleaned)
		if err != nil || rel == "." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || rel == ".." {
			return "", false
		}
		cleaned = rel
	}

	parts := splitManagedPath(cleaned)
	if len(parts) == 0 {
		return "", false
	}

	root, ok := canonicalManagedRoot(parts[0])
	if !ok {
		return "", false
	}

	switch root {
	case "Inbound", "outbound", "sub_json", "sub_manager":
		if len(parts) == 1 {
			return root, true
		}
		return path.Join(append([]string{root}, parts[1:]...)...), true
	case "core":
		corePath, ok := canonicalManagedCorePath(parts[1:]...)
		if !ok {
			return "", false
		}
		return path.Join(root, corePath), true
	default:
		return "", false
	}
}

func canonicalManagedRoot(raw string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "inbound":
		return "Inbound", true
	case "outbound":
		return "outbound", true
	case "sub_json":
		return "sub_json", true
	case "sub_manager":
		return "sub_manager", true
	case "core":
		return "core", true
	default:
		return "", false
	}
}

func canonicalManagedCoreFile(raw string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "config.json":
		return path.Join("singbox", "config.json"), true
	case "server.yaml":
		return path.Join("mihomo", "server.yaml"), true
	case strings.ToLower(mihomoInboundMetaFilename):
		return mihomoInboundMetaFilename, true
	default:
		return "", false
	}
}

func canonicalManagedCorePath(parts ...string) (string, bool) {
	if len(parts) == 0 {
		return "", false
	}
	if len(parts) == 1 {
		return canonicalManagedCoreFile(parts[0])
	}
	if len(parts) == 2 {
		subdir := strings.ToLower(strings.TrimSpace(parts[0]))
		fileName := strings.ToLower(strings.TrimSpace(parts[1]))
		switch {
		case subdir == "singbox" && fileName == "config.json":
			return path.Join("singbox", "config.json"), true
		case subdir == "mihomo" && fileName == "server.yaml":
			return path.Join("mihomo", "server.yaml"), true
		case subdir == "core":
			return canonicalManagedCoreFile(parts[1])
		}
	}
	return "", false
}

func isManagedRuntimeCoreConfigFile(canonical string) bool {
	return canonical == "core/singbox/config.json" || canonical == "core/mihomo/server.yaml"
}

func isManagedRuntimeTempCoreFile(canonical string) bool {
	return canonical == "core/singbox/config.json" || canonical == "core/mihomo/server.yaml"
}

func managedRuntimeDiskPath(canonical string) string {
	if canonical == "" {
		return config.GetDataDir()
	}
	parts := strings.Split(canonical, "/")
	items := make([]string, 0, len(parts)+1)
	items = append(items, config.GetDataDir())
	items = append(items, parts...)
	return filepath.Join(items...)
}

func splitManagedPath(raw string) []string {
	normalized := filepath.ToSlash(filepath.Clean(raw))
	normalized = strings.TrimPrefix(normalized, "./")
	normalized = strings.TrimPrefix(normalized, "/")
	if normalized == "" || normalized == "." {
		return nil
	}

	parts := strings.Split(normalized, "/")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" || part == "." {
			continue
		}
		result = append(result, part)
	}
	return result
}
