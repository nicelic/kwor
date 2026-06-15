package config

import (
	_ "embed"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

//go:embed version
var version string

//go:embed name
var name string

type LogLevel string

const (
	Debug LogLevel = "debug"
	Info  LogLevel = "info"
	Warn  LogLevel = "warn"
	Error LogLevel = "error"
)

var (
	dbPathOnce        sync.Once
	dbPath            string
	monitorDBPathOnce sync.Once
	monitorDBPath     string
)

func GetVersion() string {
	return strings.TrimSpace(version)
}

func GetName() string {
	return strings.TrimSpace(name)
}

func GetLogLevel() LogLevel {
	if IsDebug() {
		return Debug
	}
	logLevel := strings.ToLower(strings.TrimSpace(os.Getenv("KWOR_LOG_LEVEL")))
	if logLevel == "" {
		return Info
	}
	switch LogLevel(logLevel) {
	case Debug, Info, Warn, Error:
		return LogLevel(logLevel)
	default:
		return Info
	}
}

func IsDebug() bool {
	return strings.EqualFold(strings.TrimSpace(os.Getenv("KWOR_DEBUG")), "true")
}

// GetBinDir returns the directory where the running binary is located (relative path safe)
func GetBinDir() string {
	execPath, err := os.Executable()
	if err != nil {
		// Fallback to current working directory
		dir, err2 := os.Getwd()
		if err2 != nil {
			return "."
		}
		return dir
	}
	// Resolve symlinks
	realPath, err := filepath.EvalSymlinks(execPath)
	if err != nil {
		realPath = execPath
	}
	return filepath.Dir(realPath)
}

func GetDataDir() string {
	return filepath.Join(GetBinDir(), "Promanager_data")
}

func GetDBFolderPath() string {
	defaultPath := filepath.Join(GetDataDir(), "db")
	dbFolderPath := strings.TrimSpace(os.Getenv("KWOR_DB_FOLDER"))
	if dbFolderPath == "" {
		return defaultPath
	}

	candidate := filepath.Clean(dbFolderPath)
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(GetDataDir(), candidate)
	}
	if !isSubPath(candidate, GetDataDir()) {
		return defaultPath
	}
	return candidate
}

func GetDBPath() string {
	dbPathOnce.Do(func() {
		dbPath = filepath.Join(GetDBFolderPath(), GetName()+".db")
		// Always migrate old ./db data into Promanager_data/db (or its subdirectory) when possible.
		migrateLegacyDBArtifacts(dbPath)
	})
	return dbPath
}

func GetSystemMonitorDBPath() string {
	monitorDBPathOnce.Do(func() {
		monitorDBPath = filepath.Join(GetDBFolderPath(), "monitor.db")
	})
	return monitorDBPath
}

func isSubPath(path, parent string) bool {
	cleanPath := filepath.Clean(path)
	cleanParent := filepath.Clean(parent)

	rel, err := filepath.Rel(cleanParent, cleanPath)
	if err != nil {
		return false
	}
	if rel == "." {
		return true
	}
	if strings.HasPrefix(rel, ".."+string(os.PathSeparator)) || rel == ".." {
		return false
	}
	return true
}

func getLegacyDBPath() string {
	return filepath.Join(GetBinDir(), "db", GetName()+".db")
}

func migrateLegacyDBArtifacts(newDBPath string) {
	oldDBPath := getLegacyDBPath()
	if filepath.Clean(oldDBPath) == filepath.Clean(newDBPath) {
		return
	}

	// No legacy db file, nothing to migrate.
	if !pathExists(oldDBPath) {
		return
	}

	if err := os.MkdirAll(filepath.Dir(newDBPath), 0o740); err != nil {
		return
	}

	// Always prefer the new path; copy old data into it when needed.
	if !pathExists(newDBPath) {
		if err := copyFile(oldDBPath, newDBPath); err != nil {
			return
		}
	}

	// Copy sidecar files best-effort.
	copyLegacyDBSidecars(oldDBPath, newDBPath)

	// After copy succeeded and new db exists, clean old artifacts.
	if pathExists(newDBPath) {
		removeLegacyDBArtifacts(oldDBPath)
		_ = removeDirIfEmpty(filepath.Dir(oldDBPath))
	}
}

func copyLegacyDBSidecars(oldDBPath, newDBPath string) {
	suffixes := []string{"-wal", "-shm", ".backup", ".temp"}
	for _, suffix := range suffixes {
		oldPath := oldDBPath + suffix
		if !pathExists(oldPath) {
			continue
		}

		newPath := newDBPath + suffix
		if pathExists(newPath) {
			continue
		}

		if err := copyFile(oldPath, newPath); err != nil {
			continue
		}
	}
}

func removeLegacyDBArtifacts(oldDBPath string) {
	suffixes := []string{"", "-wal", "-shm", ".backup", ".temp"}
	for _, suffix := range suffixes {
		_ = os.Remove(oldDBPath + suffix)
	}
}

func pathExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func copyFile(oldPath, newPath string) error {
	in, err := os.Open(oldPath)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(newPath)
	if err != nil {
		return err
	}

	if _, err = io.Copy(out, in); err != nil {
		out.Close()
		_ = os.Remove(newPath)
		return err
	}

	if err = out.Close(); err != nil {
		_ = os.Remove(newPath)
		return err
	}

	return nil
}

func removeDirIfEmpty(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	if len(entries) > 0 {
		return nil
	}
	return os.Remove(dir)
}
