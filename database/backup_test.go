package database

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/alireza0/s-ui/config"
	"github.com/alireza0/s-ui/database/model"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func buildBackupArchiveForTest(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for name, content := range entries {
		entry, err := writer.Create(name)
		if err != nil {
			t.Fatalf("create archive entry failed: %v", err)
		}
		if _, err := entry.Write([]byte(content)); err != nil {
			t.Fatalf("write archive entry failed: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close archive writer failed: %v", err)
	}
	return buffer.Bytes()
}

func TestReadDBBackupArchiveEntriesWithLimits(t *testing.T) {
	limits := dbBackupArchiveLimits{maxEntries: 2, maxEntryBytes: 8, maxTotalBytes: 12}
	archive := buildBackupArchiveForTest(t, map[string]string{
		"first.db":  "first",
		"second.db": "second",
	})

	entries, err := readDBBackupArchiveEntriesWithLimits(archive, limits)
	if err != nil {
		t.Fatalf("read archive failed: %v", err)
	}
	if string(entries["first.db"]) != "first" || string(entries["second.db"]) != "second" {
		t.Fatalf("unexpected archive entries: %#v", entries)
	}
}

func TestReadDBBackupArchiveEntriesRejectsExpandedPayload(t *testing.T) {
	limits := dbBackupArchiveLimits{maxEntries: 1, maxEntryBytes: 4, maxTotalBytes: 4}
	archive := buildBackupArchiveForTest(t, map[string]string{"kwor.db": "12345"})

	if _, err := readDBBackupArchiveEntriesWithLimits(archive, limits); err == nil {
		t.Fatal("expected oversized archive entry to be rejected")
	}
}

func TestReadReaderWithLimitRejectsOversizedInput(t *testing.T) {
	if _, err := readReaderWithLimit(strings.NewReader("12345"), 4, "test input"); err == nil {
		t.Fatal("expected oversized input to be rejected")
	}
}

func TestValidateSQLiteDatabaseFileRejectsHeaderOnlyInput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "invalid.db")
	if err := os.WriteFile(path, []byte("SQLite format 3\x00"), 0o600); err != nil {
		t.Fatalf("write fake database failed: %v", err)
	}
	if err := validateSQLiteDatabaseFile(path); err == nil {
		t.Fatal("expected header-only SQLite input to be rejected")
	}
}

func TestBoundedBytesBufferRejectsOversizedOutput(t *testing.T) {
	buffer := &boundedBytesBuffer{maxBytes: 4}
	if _, err := buffer.Write([]byte("1234")); err != nil {
		t.Fatalf("write within limit failed: %v", err)
	}
	if _, err := buffer.Write([]byte("5")); err == nil {
		t.Fatal("expected oversized output to be rejected")
	}
	if got := string(buffer.Bytes()); got != "1234" {
		t.Fatalf("buffer contents = %q, want %q", got, "1234")
	}
}

func TestGetDbExcludesPortForwardKernelForwardState(t *testing.T) {
	previousDB := db
	if err := InitDB(config.GetDBPath()); err != nil {
		t.Fatalf("init source database: %v", err)
	}
	sourceSQL, err := GetDB().DB()
	if err == nil && sourceSQL != nil {
		t.Cleanup(func() {
			_ = sourceSQL.Close()
			db = previousDB
		})
	} else {
		t.Cleanup(func() { db = previousDB })
	}
	state := model.PortForwardKernelForwardState{
		Id:              1,
		HostFingerprint: "test-host",
		IPv4Modified:    true,
		IPv4Original:    "0\n",
	}
	if err := GetDB().Save(&state).Error; err != nil {
		t.Fatalf("save host-local state: %v", err)
	}

	contents, err := GetDb("changes,stats")
	if err != nil {
		t.Fatalf("create lightweight backup: %v", err)
	}
	backupPath := filepath.Join(t.TempDir(), "lightweight-backup.db")
	if err := os.WriteFile(backupPath, contents, 0o600); err != nil {
		t.Fatalf("write backup database: %v", err)
	}
	backupDB, err := gorm.Open(sqlite.Open(backupPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open backup database: %v", err)
	}
	backupSQL, err := backupDB.DB()
	if err != nil {
		t.Fatalf("access backup database handle: %v", err)
	}
	defer backupSQL.Close()
	if backupDB.Migrator().HasTable(&model.PortForwardKernelForwardState{}) {
		t.Fatal("host-local forwarding sysctl state must not be exported")
	}
}

func TestGetDbPreservesReverseProxySettingsSingleton(t *testing.T) {
	previousDB := db
	sourcePath := filepath.Join(t.TempDir(), "reverse-proxy-settings-source.db")
	if err := InitDB(sourcePath); err != nil {
		t.Fatalf("init source database: %v", err)
	}
	sourceSQL, err := GetDB().DB()
	if err != nil {
		t.Fatalf("access source database handle: %v", err)
	}
	t.Cleanup(func() {
		_ = sourceSQL.Close()
		db = previousDB
	})

	expected := model.ReverseProxySettings{
		Id:                                1,
		Revision:                          27,
		ListenerConnectionLimit:           4321,
		GlobalHTTPMaxConcurrent:           3210,
		GlobalDNSMaxConcurrent:            2109,
		HTTP2MaxConcurrentStreams:         251,
		QUICMaxIncomingStreams:            257,
		DefaultUpstreamMaxIdleConnections: 33,
		MemoryPoolBytes:                   9 * 1024 * 1024,
		DefaultRuleMemoryLimitBytes:       1024 * 1024,
		ResponseRewriteInputBytes:         512 * 1024,
		ResponseRewriteOutputBytes:        512 * 1024,
		ResponseRewriteMaxConcurrent:      31,
	}
	if err := GetDB().Save(&expected).Error; err != nil {
		t.Fatalf("save reverse proxy settings: %v", err)
	}

	contents, err := GetDb("changes,stats")
	if err != nil {
		t.Fatalf("create reverse proxy settings backup: %v", err)
	}
	backupPath := filepath.Join(t.TempDir(), "reverse-proxy-settings-backup.db")
	if err := os.WriteFile(backupPath, contents, 0o600); err != nil {
		t.Fatalf("write backup database: %v", err)
	}
	backupDB, err := gorm.Open(sqlite.Open(backupPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open backup database: %v", err)
	}
	backupSQL, err := backupDB.DB()
	if err != nil {
		t.Fatalf("access backup database handle: %v", err)
	}
	defer backupSQL.Close()

	var restored model.ReverseProxySettings
	if err := backupDB.Where("id = ?", expected.Id).First(&restored).Error; err != nil {
		t.Fatalf("load reverse proxy settings from backup: %v", err)
	}
	if restored.Revision != expected.Revision ||
		restored.ListenerConnectionLimit != expected.ListenerConnectionLimit ||
		restored.GlobalHTTPMaxConcurrent != expected.GlobalHTTPMaxConcurrent ||
		restored.GlobalDNSMaxConcurrent != expected.GlobalDNSMaxConcurrent ||
		restored.HTTP2MaxConcurrentStreams != expected.HTTP2MaxConcurrentStreams ||
		restored.QUICMaxIncomingStreams != expected.QUICMaxIncomingStreams ||
		restored.DefaultUpstreamMaxIdleConnections != expected.DefaultUpstreamMaxIdleConnections ||
		restored.MemoryPoolBytes != expected.MemoryPoolBytes ||
		restored.DefaultRuleMemoryLimitBytes != expected.DefaultRuleMemoryLimitBytes ||
		restored.ResponseRewriteInputBytes != expected.ResponseRewriteInputBytes ||
		restored.ResponseRewriteOutputBytes != expected.ResponseRewriteOutputBytes ||
		restored.ResponseRewriteMaxConcurrent != expected.ResponseRewriteMaxConcurrent {
		t.Fatalf("reverse proxy settings changed in backup: got=%#v want=%#v", restored, expected)
	}
}

func TestGetDbPreservesDnsServers(t *testing.T) {
	previousDB := db
	sourcePath := filepath.Join(t.TempDir(), "dns-servers-source.db")
	if err := InitDB(sourcePath); err != nil {
		t.Fatalf("init source database: %v", err)
	}
	sourceSQL, err := GetDB().DB()
	if err != nil {
		t.Fatalf("access source database handle: %v", err)
	}
	t.Cleanup(func() {
		_ = sourceSQL.Close()
		db = previousDB
	})

	expected := model.DnsServer{
		Type:    "https",
		Tag:     "dns-backup",
		Options: []byte("{\"server\":\"1.1.1.1\",\"server_port\":443,\"path\":\"/dns-query\"}"),
	}
	if err := GetDB().Create(&expected).Error; err != nil {
		t.Fatalf("save DNS server: %v", err)
	}

	contents, err := GetDb("changes,stats")
	if err != nil {
		t.Fatalf("create DNS server backup: %v", err)
	}
	backupPath := filepath.Join(t.TempDir(), "dns-servers-backup.db")
	if err := os.WriteFile(backupPath, contents, 0o600); err != nil {
		t.Fatalf("write backup database: %v", err)
	}
	backupDB, err := gorm.Open(sqlite.Open(backupPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open backup database: %v", err)
	}
	backupSQL, err := backupDB.DB()
	if err != nil {
		t.Fatalf("access backup database handle: %v", err)
	}
	defer backupSQL.Close()

	var restored model.DnsServer
	if err := backupDB.Where("tag = ?", expected.Tag).First(&restored).Error; err != nil {
		t.Fatalf("load DNS server from backup: %v", err)
	}
	if restored.Type != expected.Type || string(restored.Options) != string(expected.Options) {
		t.Fatalf("DNS server changed in backup: got=%#v want=%#v", restored, expected)
	}
}

func TestGetDbPreservesSettingsStateSingleton(t *testing.T) {
	previousDB := db
	sourcePath := filepath.Join(t.TempDir(), "settings-state-source.db")
	if err := InitDB(sourcePath); err != nil {
		t.Fatalf("init source database: %v", err)
	}
	sourceSQL, err := GetDB().DB()
	if err != nil {
		t.Fatalf("access source database handle: %v", err)
	}
	t.Cleanup(func() {
		_ = sourceSQL.Close()
		db = previousDB
	})

	expected := model.SettingsState{Id: 1, Revision: 27}
	if err := GetDB().Save(&expected).Error; err != nil {
		t.Fatalf("save settings state: %v", err)
	}
	contents, err := GetDb("changes,stats")
	if err != nil {
		t.Fatalf("create settings state backup: %v", err)
	}
	backupPath := filepath.Join(t.TempDir(), "settings-state-backup.db")
	if err := os.WriteFile(backupPath, contents, 0o600); err != nil {
		t.Fatalf("write backup database: %v", err)
	}
	backupDB, err := gorm.Open(sqlite.Open(backupPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open backup database: %v", err)
	}
	backupSQL, err := backupDB.DB()
	if err != nil {
		t.Fatalf("access backup database handle: %v", err)
	}
	defer backupSQL.Close()

	var restored model.SettingsState
	if err := backupDB.First(&restored, 1).Error; err != nil {
		t.Fatalf("load settings state from backup: %v", err)
	}
	if restored != expected {
		t.Fatalf("settings state backup = %#v, want %#v", restored, expected)
	}
}

func TestCreateSQLiteSnapshotExcludesPortForwardKernelForwardState(t *testing.T) {
	sourcePath := filepath.Join(t.TempDir(), "source.db")
	sourceDB, err := gorm.Open(sqlite.Open(sourcePath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open source database: %v", err)
	}
	sourceSQL, err := sourceDB.DB()
	if err != nil {
		t.Fatalf("access source database handle: %v", err)
	}
	defer sourceSQL.Close()
	if err := sourceDB.AutoMigrate(&model.PortForwardKernelForwardState{}); err != nil {
		t.Fatalf("migrate source database: %v", err)
	}
	if err := sourceDB.Create(&model.PortForwardKernelForwardState{Id: 1, HostFingerprint: "source-host", IPv4Modified: true, IPv4Original: "0\n"}).Error; err != nil {
		t.Fatalf("create source host-local state: %v", err)
	}

	snapshotPath := filepath.Join(t.TempDir(), "snapshot.db")
	if err := createSQLiteSnapshot(sourcePath, snapshotPath); err != nil {
		t.Fatalf("create archive snapshot: %v", err)
	}
	snapshotDB, err := gorm.Open(sqlite.Open(snapshotPath), &gorm.Config{})
	if err != nil {
		t.Fatalf("open archive snapshot: %v", err)
	}
	snapshotSQL, err := snapshotDB.DB()
	if err != nil {
		t.Fatalf("access archive snapshot handle: %v", err)
	}
	defer snapshotSQL.Close()
	if snapshotDB.Migrator().HasTable(&model.PortForwardKernelForwardState{}) {
		t.Fatal("host-local forwarding sysctl state must not be present in archive snapshots")
	}
}
