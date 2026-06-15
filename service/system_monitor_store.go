package service

import (
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/alireza0/s-ui/config"
	"github.com/alireza0/s-ui/logger"

	sqliteDriver "github.com/glebarez/sqlite"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

type systemMonitorCollectorState struct {
	ReadBytes        uint64                                    `json:"readBytes"`
	WriteBytes       uint64                                    `json:"writeBytes"`
	PhysicalNetStats map[string]systemMonitorInterfaceCounters `json:"physicalNetStats,omitempty"`
	SampledAt        int64                                     `json:"sampledAt"`
}

type systemMonitorInterfaceCounters struct {
	SentBytes uint64 `json:"sentBytes"`
	RecvBytes uint64 `json:"recvBytes"`
}

type systemMonitorRollupDefinition struct {
	RangeKey          string
	TableName         string
	BucketDuration    time.Duration
	RetentionDuration time.Duration
}

type systemMonitorRollupRow struct {
	BucketStart    int64 `gorm:"column:bucket_start"`
	SampleCount    int64 `gorm:"column:sample_count"`
	CPUAvg         int64 `gorm:"column:cpu_avg"`
	CPUMax         int64 `gorm:"column:cpu_max"`
	MemoryAvg      int64 `gorm:"column:memory_avg"`
	MemoryMax      int64 `gorm:"column:memory_max"`
	DiskReadAvg    int64 `gorm:"column:disk_read_avg"`
	DiskReadMax    int64 `gorm:"column:disk_read_max"`
	DiskWriteAvg   int64 `gorm:"column:disk_write_avg"`
	DiskWriteMax   int64 `gorm:"column:disk_write_max"`
	NetworkUpAvg   int64 `gorm:"column:network_up_avg"`
	NetworkUpMax   int64 `gorm:"column:network_up_max"`
	NetworkDownAvg int64 `gorm:"column:network_down_avg"`
	NetworkDownMax int64 `gorm:"column:network_down_max"`
}

type systemMonitorScaledPoint struct {
	CPUPercentScaled    int64
	MemoryPercentScaled int64
	DiskReadBps         int64
	DiskWriteBps        int64
	NetworkUpBps        int64
	NetworkDownBps      int64
}

const (
	systemMonitorMetaCollectorStateKey = "collector_state"
)

var (
	systemMonitorDBInitOnce sync.Once
	systemMonitorDBInitErr  error
	systemMonitorDB         *gorm.DB
)

var systemMonitorRollupDefinitions = []systemMonitorRollupDefinition{
	{
		RangeKey:          "8s",
		TableName:         "system_monitor_rollup_8s",
		BucketDuration:    8 * time.Second,
		RetentionDuration: 8 * time.Hour,
	},
	{
		RangeKey:          "1m",
		TableName:         "system_monitor_rollup_1m",
		BucketDuration:    time.Minute,
		RetentionDuration: 48 * time.Hour,
	},
	{
		RangeKey:          "30m",
		TableName:         "system_monitor_rollup_30m",
		BucketDuration:    30 * time.Minute,
		RetentionDuration: 120 * 24 * time.Hour,
	},
}

func InitSystemMonitorStore() error {
	systemMonitorDBInitOnce.Do(func() {
		systemMonitorDBInitErr = openSystemMonitorStore()
	})
	return systemMonitorDBInitErr
}

func GetSystemMonitorStorePath() string {
	return config.GetSystemMonitorDBPath()
}

func openSystemMonitorStore() error {
	dbPath := config.GetSystemMonitorDBPath()
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o740); err != nil {
		return err
	}

	var gormLog gormlogger.Interface
	if config.IsDebug() {
		gormLog = gormlogger.Default
	} else {
		gormLog = gormlogger.Discard
	}

	db, err := gorm.Open(sqliteDriver.Open(systemMonitorSQLiteDSN(dbPath)), &gorm.Config{
		Logger: gormLog,
	})
	if err != nil {
		return err
	}

	sqlDB, err := db.DB()
	if err != nil {
		return err
	}
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)

	if err := ensureSystemMonitorSchema(db); err != nil {
		return err
	}

	systemMonitorDB = db
	return nil
}

func systemMonitorSQLiteDSN(dbPath string) string {
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

func ensureSystemMonitorSchema(db *gorm.DB) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS system_monitor_meta (
			key TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			updated_at INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS system_monitor_rollup_8s (
			bucket_start INTEGER PRIMARY KEY,
			sample_count INTEGER NOT NULL DEFAULT 0,
			cpu_avg INTEGER NOT NULL DEFAULT 0,
			cpu_min INTEGER NOT NULL DEFAULT 0,
			cpu_max INTEGER NOT NULL DEFAULT 0,
			memory_avg INTEGER NOT NULL DEFAULT 0,
			memory_min INTEGER NOT NULL DEFAULT 0,
			memory_max INTEGER NOT NULL DEFAULT 0,
			disk_read_avg INTEGER NOT NULL DEFAULT 0,
			disk_read_max INTEGER NOT NULL DEFAULT 0,
			disk_write_avg INTEGER NOT NULL DEFAULT 0,
			disk_write_max INTEGER NOT NULL DEFAULT 0,
			network_up_avg INTEGER NOT NULL DEFAULT 0,
			network_up_max INTEGER NOT NULL DEFAULT 0,
			network_down_avg INTEGER NOT NULL DEFAULT 0,
			network_down_max INTEGER NOT NULL DEFAULT 0,
			updated_at INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS system_monitor_rollup_1m (
			bucket_start INTEGER PRIMARY KEY,
			sample_count INTEGER NOT NULL DEFAULT 0,
			cpu_avg INTEGER NOT NULL DEFAULT 0,
			cpu_min INTEGER NOT NULL DEFAULT 0,
			cpu_max INTEGER NOT NULL DEFAULT 0,
			memory_avg INTEGER NOT NULL DEFAULT 0,
			memory_min INTEGER NOT NULL DEFAULT 0,
			memory_max INTEGER NOT NULL DEFAULT 0,
			disk_read_avg INTEGER NOT NULL DEFAULT 0,
			disk_read_max INTEGER NOT NULL DEFAULT 0,
			disk_write_avg INTEGER NOT NULL DEFAULT 0,
			disk_write_max INTEGER NOT NULL DEFAULT 0,
			network_up_avg INTEGER NOT NULL DEFAULT 0,
			network_up_max INTEGER NOT NULL DEFAULT 0,
			network_down_avg INTEGER NOT NULL DEFAULT 0,
			network_down_max INTEGER NOT NULL DEFAULT 0,
			updated_at INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE IF NOT EXISTS system_monitor_rollup_30m (
			bucket_start INTEGER PRIMARY KEY,
			sample_count INTEGER NOT NULL DEFAULT 0,
			cpu_avg INTEGER NOT NULL DEFAULT 0,
			cpu_min INTEGER NOT NULL DEFAULT 0,
			cpu_max INTEGER NOT NULL DEFAULT 0,
			memory_avg INTEGER NOT NULL DEFAULT 0,
			memory_min INTEGER NOT NULL DEFAULT 0,
			memory_max INTEGER NOT NULL DEFAULT 0,
			disk_read_avg INTEGER NOT NULL DEFAULT 0,
			disk_read_max INTEGER NOT NULL DEFAULT 0,
			disk_write_avg INTEGER NOT NULL DEFAULT 0,
			disk_write_max INTEGER NOT NULL DEFAULT 0,
			network_up_avg INTEGER NOT NULL DEFAULT 0,
			network_up_max INTEGER NOT NULL DEFAULT 0,
			network_down_avg INTEGER NOT NULL DEFAULT 0,
			network_down_max INTEGER NOT NULL DEFAULT 0,
			updated_at INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE INDEX IF NOT EXISTS idx_system_monitor_rollup_8s_updated_at ON system_monitor_rollup_8s(updated_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_system_monitor_rollup_1m_updated_at ON system_monitor_rollup_1m(updated_at DESC)`,
		`CREATE INDEX IF NOT EXISTS idx_system_monitor_rollup_30m_updated_at ON system_monitor_rollup_30m(updated_at DESC)`,
	}
	for _, statement := range statements {
		if err := db.Exec(statement).Error; err != nil {
			return err
		}
	}
	for _, tableName := range []string{"system_monitor_rollup_8s", "system_monitor_rollup_1m", "system_monitor_rollup_30m"} {
		if err := ensureSystemMonitorRollupColumns(db, tableName); err != nil {
			return err
		}
	}
	return nil
}

func ensureSystemMonitorRollupColumns(db *gorm.DB, tableName string) error {
	type tableInfoRow struct {
		Name string `gorm:"column:name"`
	}
	rows := make([]tableInfoRow, 0)
	if err := db.Raw(fmt.Sprintf("PRAGMA table_info(%s)", tableName)).Scan(&rows).Error; err != nil {
		return err
	}
	existing := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		existing[strings.TrimSpace(row.Name)] = struct{}{}
	}

	columns := []string{
		"network_up_avg INTEGER NOT NULL DEFAULT 0",
		"network_up_max INTEGER NOT NULL DEFAULT 0",
		"network_down_avg INTEGER NOT NULL DEFAULT 0",
		"network_down_max INTEGER NOT NULL DEFAULT 0",
	}
	for _, columnDef := range columns {
		columnName := strings.TrimSpace(strings.SplitN(columnDef, " ", 2)[0])
		if _, ok := existing[columnName]; ok {
			continue
		}
		if err := db.Exec(fmt.Sprintf("ALTER TABLE %s ADD COLUMN %s", tableName, columnDef)).Error; err != nil {
			return err
		}
	}
	return nil
}

func systemMonitorDBHandle() (*gorm.DB, error) {
	if err := InitSystemMonitorStore(); err != nil {
		return nil, err
	}
	if systemMonitorDB == nil {
		return nil, fmt.Errorf("system monitor db is not initialized")
	}
	return systemMonitorDB, nil
}

func loadSystemMonitorCollectorState() (systemMonitorCollectorState, error) {
	state := systemMonitorCollectorState{}
	db, err := systemMonitorDBHandle()
	if err != nil {
		return state, err
	}

	type metaRow struct {
		Value string `gorm:"column:value"`
	}
	row := metaRow{}
	err = db.Table("system_monitor_meta").Select("value").Where("key = ?", systemMonitorMetaCollectorStateKey).Take(&row).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return state, nil
		}
		return state, err
	}
	if strings.TrimSpace(row.Value) == "" {
		return state, nil
	}
	if err := json.Unmarshal([]byte(row.Value), &state); err != nil {
		return state, err
	}
	return state, nil
}

func saveSystemMonitorCollectorState(state systemMonitorCollectorState) error {
	db, err := systemMonitorDBHandle()
	if err != nil {
		return err
	}
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return db.Exec(`
		INSERT INTO system_monitor_meta (key, value, updated_at)
		VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET
			value = excluded.value,
			updated_at = excluded.updated_at
	`, systemMonitorMetaCollectorStateKey, string(data), state.SampledAt).Error
}

func upsertSystemMonitorRollup(def systemMonitorRollupDefinition, sampledAt time.Time, point systemMonitorScaledPoint) error {
	db, err := systemMonitorDBHandle()
	if err != nil {
		return err
	}

	bucketStart := sampledAt.Truncate(def.BucketDuration).Unix()
	updatedAt := sampledAt.Unix()
	statement := fmt.Sprintf(`
		INSERT INTO %s (
			bucket_start,
			sample_count,
			cpu_avg,
			cpu_min,
			cpu_max,
			memory_avg,
			memory_min,
			memory_max,
			disk_read_avg,
			disk_read_max,
			disk_write_avg,
			disk_write_max,
			network_up_avg,
			network_up_max,
			network_down_avg,
			network_down_max,
			updated_at
		) VALUES (?, 1, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(bucket_start) DO UPDATE SET
			sample_count = sample_count + 1,
			cpu_avg = ((cpu_avg * sample_count) + excluded.cpu_avg) / (sample_count + 1),
			cpu_min = MIN(cpu_min, excluded.cpu_min),
			cpu_max = MAX(cpu_max, excluded.cpu_max),
			memory_avg = ((memory_avg * sample_count) + excluded.memory_avg) / (sample_count + 1),
			memory_min = MIN(memory_min, excluded.memory_min),
			memory_max = MAX(memory_max, excluded.memory_max),
			disk_read_avg = ((disk_read_avg * sample_count) + excluded.disk_read_avg) / (sample_count + 1),
			disk_read_max = MAX(disk_read_max, excluded.disk_read_max),
			disk_write_avg = ((disk_write_avg * sample_count) + excluded.disk_write_avg) / (sample_count + 1),
			disk_write_max = MAX(disk_write_max, excluded.disk_write_max),
			network_up_avg = ((network_up_avg * sample_count) + excluded.network_up_avg) / (sample_count + 1),
			network_up_max = MAX(network_up_max, excluded.network_up_max),
			network_down_avg = ((network_down_avg * sample_count) + excluded.network_down_avg) / (sample_count + 1),
			network_down_max = MAX(network_down_max, excluded.network_down_max),
			updated_at = excluded.updated_at
	`, def.TableName)

	return db.Exec(
		statement,
		bucketStart,
		point.CPUPercentScaled,
		point.CPUPercentScaled,
		point.CPUPercentScaled,
		point.MemoryPercentScaled,
		point.MemoryPercentScaled,
		point.MemoryPercentScaled,
		point.DiskReadBps,
		point.DiskReadBps,
		point.DiskWriteBps,
		point.DiskWriteBps,
		point.NetworkUpBps,
		point.NetworkUpBps,
		point.NetworkDownBps,
		point.NetworkDownBps,
		updatedAt,
	).Error
}

func pruneSystemMonitorRollups(now time.Time) error {
	db, err := systemMonitorDBHandle()
	if err != nil {
		return err
	}
	settings := currentSystemMonitorSettings()
	nowUnix := now.Unix()
	for _, def := range systemMonitorRollupDefinitions {
		cutoff := now.Add(-systemMonitorRetentionForRollup(settings, def)).Unix()
		statement := fmt.Sprintf("DELETE FROM %s WHERE bucket_start < ?", def.TableName)
		if err := db.Exec(statement, cutoff).Error; err != nil {
			return err
		}
		logger.Debugf("system monitor prune %s keep since %d now=%d", def.TableName, cutoff, nowUnix)
	}
	return nil
}

func clearSystemMonitorHistoryAndCompact() error {
	db, err := systemMonitorDBHandle()
	if err != nil {
		return err
	}

	for _, def := range systemMonitorRollupDefinitions {
		statement := fmt.Sprintf("DELETE FROM %s", def.TableName)
		if err := db.Exec(statement).Error; err != nil {
			return err
		}
	}

	if err := db.Exec("PRAGMA wal_checkpoint(TRUNCATE)").Error; err != nil {
		logger.Warning("system monitor wal checkpoint failed:", err)
	}

	if err := db.Exec("VACUUM").Error; err != nil {
		return err
	}
	return nil
}

func querySystemMonitorHistory(def systemMonitorRollupDefinition, start time.Time, end time.Time) ([]systemMonitorRollupRow, error) {
	db, err := systemMonitorDBHandle()
	if err != nil {
		return nil, err
	}
	rows := make([]systemMonitorRollupRow, 0)
	statement := fmt.Sprintf(`
		SELECT
			bucket_start,
			sample_count,
			cpu_avg,
			cpu_max,
			memory_avg,
			memory_max,
			disk_read_avg,
			disk_read_max,
			disk_write_avg,
			disk_write_max,
			network_up_avg,
			network_up_max,
			network_down_avg,
			network_down_max
		FROM %s
		WHERE bucket_start >= ? AND bucket_start <= ?
		ORDER BY bucket_start ASC
	`, def.TableName)
	if err := db.Raw(statement, start.Unix(), end.Unix()).Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func getSystemMonitorDatabaseSizeBytes() uint64 {
	info, err := os.Stat(config.GetSystemMonitorDBPath())
	if err != nil {
		return 0
	}
	if info.Size() <= 0 {
		return 0
	}
	return uint64(info.Size())
}

func scalePercentToInt(value float64) int64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}
	if value < 0 {
		value = 0
	}
	if value > 100 {
		value = 100
	}
	return int64(math.Round(value * 100))
}

func unscalePercentToFloat(value int64) float64 {
	return float64(value) / 100
}
