package service

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/logger"
	"gorm.io/gorm"
)

const (
	statsBucketSeconds            int64   = 60
	statsBucketUniqueIndex                = "idx_stats_bucket_unique"
	statsLookupIndex                      = "idx_stats_resource_tag_time"
	changesDateTimeIndex                  = "idx_changes_date_time"
	changesRetentionDays          int     = 180
	changesMaxRows                int64   = 10000
	changesMinKeepRows            int64   = 1000
	sqliteCompactMinFreelistPages int64   = 1024
	sqliteCompactMinFreelistRatio float64 = 0.10
)

var (
	historyStorageStateMu  sync.Mutex
	historyStorageInitOnce sync.Once
	historyStorageInitErr  error
)

func init() {
	database.RegisterDBResetHook(func() {
		resetHistoryStorageState()
	})
}

func resetHistoryStorageState() {
	historyStorageStateMu.Lock()
	defer historyStorageStateMu.Unlock()

	historyStorageInitOnce = sync.Once{}
	historyStorageInitErr = nil
}

func EnsureHistoryStorageReady() error {
	historyStorageStateMu.Lock()
	defer historyStorageStateMu.Unlock()

	historyStorageInitOnce.Do(func() {
		historyStorageInitErr = prepareHistoryStorage(database.GetDB())
	})
	return historyStorageInitErr
}

func PrepareHistoryStorageOnStartup() error {
	if err := EnsureHistoryStorageReady(); err != nil {
		return err
	}

	db := database.GetDB()
	if db == nil {
		return fmt.Errorf("database is not initialized")
	}

	deleted, err := pruneChangesHistory(db)
	if err != nil {
		return err
	}
	if deleted > 0 {
		if err := compactMainSQLiteDB(db, false); err != nil {
			logger.Warning("compact sqlite after pruning changes failed: ", err)
		}
	}

	return nil
}

func prepareHistoryStorage(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("database is not initialized")
	}
	if err := migrateLegacyStatsTableIfNeeded(db); err != nil {
		return err
	}
	if err := ensureStatsTableIndexes(db); err != nil {
		return err
	}
	if err := ensureChangesTableIndexes(db); err != nil {
		return err
	}
	return nil
}

func migrateLegacyStatsTableIfNeeded(db *gorm.DB) error {
	if db == nil || !db.Migrator().HasTable(&model.Stats{}) {
		return nil
	}

	needsMigration, err := statsTableNeedsCompaction(db)
	if err != nil {
		return err
	}
	if !needsMigration {
		if err := ensureStatsTableIndexes(db); err != nil {
			return err
		}
		return nil
	}

	tx := db.Begin()
	if tx.Error != nil {
		return tx.Error
	}

	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
			panic(r)
		}
	}()

	statements := []struct {
		sql  string
		args []interface{}
	}{
		{
			sql: "DROP TABLE IF EXISTS stats_compact_tmp",
		},
		{
			sql: `CREATE TABLE stats_compact_tmp (
				id INTEGER PRIMARY KEY AUTOINCREMENT,
				date_time INTEGER NOT NULL,
				resource TEXT NOT NULL,
				tag TEXT NOT NULL,
				direction NUMERIC NOT NULL DEFAULT 0,
				traffic INTEGER NOT NULL DEFAULT 0
			)`,
		},
		{
			sql: `INSERT INTO stats_compact_tmp (date_time, resource, tag, direction, traffic)
				SELECT
					bucket_start,
					resource,
					tag,
					direction,
					SUM(traffic) AS traffic
				FROM (
					SELECT
						CAST((date_time / ?) * ? AS INTEGER) AS bucket_start,
						CASE
							WHEN LOWER(TRIM(resource)) = 'user' THEN 'client'
							WHEN LOWER(TRIM(resource)) = 'mihomo_user' THEN 'mihomo_client'
							ELSE resource
						END AS resource,
						tag,
						direction,
						traffic
					FROM stats
					WHERE tag IS NOT NULL AND TRIM(tag) <> '' AND traffic > 0
				)
				GROUP BY bucket_start, resource, tag, direction`,
			args: []interface{}{statsBucketSeconds, statsBucketSeconds},
		},
		{
			sql: "DROP TABLE stats",
		},
		{
			sql: "ALTER TABLE stats_compact_tmp RENAME TO stats",
		},
	}

	for _, statement := range statements {
		if err := tx.Exec(statement.sql, statement.args...).Error; err != nil {
			tx.Rollback()
			return err
		}
	}

	if err := tx.Commit().Error; err != nil {
		tx.Rollback()
		return err
	}

	if err := ensureStatsTableIndexes(db); err != nil {
		return err
	}
	if err := compactMainSQLiteDB(db, true); err != nil {
		logger.Warning("compact sqlite after stats migration failed: ", err)
	}

	return nil
}

func statsTableNeedsCompaction(db *gorm.DB) (bool, error) {
	type statsCompactionCheckRow struct {
		Count int64 `gorm:"column:count"`
	}

	row := statsCompactionCheckRow{}
	if err := db.Raw(`
		SELECT COUNT(*) AS count
		FROM (
			SELECT id
			FROM stats
			WHERE traffic <= 0
				OR tag IS NULL
				OR TRIM(tag) = ''
				OR LOWER(TRIM(resource)) IN ('user', 'mihomo_user')
				OR (date_time % ?) <> 0
			LIMIT 1
		)
	`, statsBucketSeconds).Scan(&row).Error; err != nil {
		return false, err
	}
	if row.Count > 0 {
		return true, nil
	}

	hasUniqueIndex, err := sqliteIndexExists(db, statsBucketUniqueIndex)
	if err != nil {
		return false, err
	}
	if hasUniqueIndex {
		return false, nil
	}

	row = statsCompactionCheckRow{}
	if err := db.Raw(`
		SELECT COUNT(*) AS count
		FROM (
			SELECT CAST((date_time / ?) * ? AS INTEGER), resource, tag, direction
			FROM stats
			GROUP BY CAST((date_time / ?) * ? AS INTEGER), resource, tag, direction
			HAVING COUNT(*) > 1
			LIMIT 1
		)
	`, statsBucketSeconds, statsBucketSeconds, statsBucketSeconds, statsBucketSeconds).Scan(&row).Error; err != nil {
		return false, err
	}
	return row.Count > 0, nil
}

func ensureStatsTableIndexes(db *gorm.DB) error {
	if db == nil || !db.Migrator().HasTable(&model.Stats{}) {
		return nil
	}

	statements := []string{
		fmt.Sprintf("CREATE UNIQUE INDEX IF NOT EXISTS %s ON stats(date_time, resource, tag, direction)", statsBucketUniqueIndex),
		fmt.Sprintf("CREATE INDEX IF NOT EXISTS %s ON stats(resource, tag, date_time DESC)", statsLookupIndex),
	}
	for _, statement := range statements {
		if err := db.Exec(statement).Error; err != nil {
			return err
		}
	}
	return nil
}

func ensureChangesTableIndexes(db *gorm.DB) error {
	if db == nil || !db.Migrator().HasTable(&model.Changes{}) {
		return nil
	}
	statement := fmt.Sprintf("CREATE INDEX IF NOT EXISTS %s ON changes(date_time DESC)", changesDateTimeIndex)
	return db.Exec(statement).Error
}

func sqliteIndexExists(db *gorm.DB, name string) (bool, error) {
	if db == nil {
		return false, fmt.Errorf("database is not initialized")
	}

	type indexCount struct {
		Count int64 `gorm:"column:count"`
	}

	row := indexCount{}
	if err := db.Raw(
		"SELECT COUNT(*) AS count FROM sqlite_master WHERE type = 'index' AND name = ?",
		name,
	).Scan(&row).Error; err != nil {
		return false, err
	}
	return row.Count > 0, nil
}

func statsBucketStart(unixSec int64) int64 {
	if unixSec <= 0 {
		unixSec = time.Now().Unix()
	}
	return (unixSec / statsBucketSeconds) * statsBucketSeconds
}

func normalizeStatsResource(resource string) string {
	switch strings.ToLower(strings.TrimSpace(resource)) {
	case "user":
		return "client"
	case "mihomo_user":
		return "mihomo_client"
	default:
		return strings.TrimSpace(resource)
	}
}

func upsertStatsTraffic(tx *gorm.DB, sample model.Stats) error {
	if tx == nil {
		return fmt.Errorf("stats upsert requires transaction")
	}

	resource := normalizeStatsResource(sample.Resource)
	tag := strings.TrimSpace(sample.Tag)
	if resource == "" || tag == "" || sample.Traffic <= 0 {
		return nil
	}

	return tx.Exec(`
		INSERT INTO stats (date_time, resource, tag, direction, traffic)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(date_time, resource, tag, direction) DO UPDATE SET
			traffic = traffic + excluded.traffic
	`, statsBucketStart(sample.DateTime), resource, tag, sample.Direction, sample.Traffic).Error
}

func upsertStatsTrafficBatch(tx *gorm.DB, samples []model.Stats) error {
	if err := EnsureHistoryStorageReady(); err != nil {
		return err
	}
	for _, sample := range samples {
		if err := upsertStatsTraffic(tx, sample); err != nil {
			return err
		}
	}
	return nil
}

func queryStatsHistory(resource string, tag string, limitHours int) ([]model.Stats, error) {
	if err := EnsureHistoryStorageReady(); err != nil {
		return nil, err
	}

	db := database.GetDB()
	if db == nil {
		return nil, fmt.Errorf("database is not initialized")
	}

	resource = strings.TrimSpace(resource)
	tag = strings.TrimSpace(tag)
	if resource == "" || tag == "" {
		return []model.Stats{}, nil
	}
	if limitHours <= 0 {
		limitHours = 1
	}

	startUnix := time.Now().Add(-time.Duration(limitHours) * time.Hour).Unix()
	bucketSeconds := statsQueryBucketSeconds(limitHours)
	resources := statsQueryResources(resource)
	placeholders := make([]string, 0, len(resources))
	args := make([]interface{}, 0, len(resources)+5)
	args = append(args, resource, bucketSeconds, bucketSeconds)
	for _, value := range resources {
		placeholders = append(placeholders, "?")
		args = append(args, value)
	}
	args = append(args, tag, startUnix)

	query := fmt.Sprintf(`
		SELECT
			bucket_start AS date_time,
			? AS resource,
			tag,
			direction,
			SUM(traffic) AS traffic
		FROM (
			SELECT
				CAST((date_time / ?) * ? AS INTEGER) AS bucket_start,
				tag,
				direction,
				traffic
			FROM stats
			WHERE resource IN (%s) AND tag = ? AND date_time >= ?
		)
		GROUP BY bucket_start, tag, direction
		ORDER BY bucket_start ASC, direction ASC
	`, strings.Join(placeholders, ","))

	rows := make([]model.Stats, 0)
	if err := db.Raw(query, args...).Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func statsQueryResources(resource string) []string {
	switch strings.ToLower(strings.TrimSpace(resource)) {
	case "endpoint":
		return []string{"inbound", "outbound"}
	case "client":
		return []string{"client", "user"}
	case "mihomo_client":
		return []string{"mihomo_client", "mihomo_user"}
	default:
		return []string{strings.TrimSpace(resource)}
	}
}

func statsQueryBucketSeconds(limitHours int) int64 {
	switch {
	case limitHours <= 6:
		return 60
	case limitHours <= 24:
		return 300
	case limitHours <= 72:
		return 900
	case limitHours <= 240:
		return 1800
	case limitHours <= 720:
		return 10800
	default:
		return 21600
	}
}

func recordChange(tx *gorm.DB, change model.Changes) error {
	return recordChanges(tx, []model.Changes{change})
}

func recordChanges(tx *gorm.DB, changes []model.Changes) error {
	if tx == nil {
		return fmt.Errorf("changes insert requires transaction")
	}
	if len(changes) == 0 {
		return nil
	}
	if err := tx.Model(model.Changes{}).Create(&changes).Error; err != nil {
		return err
	}
	_, err := pruneChangesHistory(tx)
	return err
}

func pruneChangesHistory(db *gorm.DB) (int64, error) {
	if db == nil || !db.Migrator().HasTable(&model.Changes{}) {
		return 0, nil
	}

	var deleted int64
	cutoff := time.Now().AddDate(0, 0, -changesRetentionDays).Unix()
	result := db.Exec(
		`DELETE FROM changes
		WHERE date_time < ?
			AND id NOT IN (
				SELECT id FROM changes ORDER BY id DESC LIMIT ?
			)`,
		cutoff,
		changesMinKeepRows,
	)
	if result.Error != nil {
		return deleted, result.Error
	}
	deleted += result.RowsAffected

	var count int64
	if err := db.Model(model.Changes{}).Count(&count).Error; err != nil {
		return deleted, err
	}
	if count <= changesMaxRows {
		return deleted, nil
	}

	excess := count - changesMaxRows
	trim := db.Exec(
		`DELETE FROM changes
		WHERE id IN (
			SELECT id
			FROM changes
			ORDER BY id ASC
			LIMIT ?
		)`,
		excess,
	)
	if trim.Error != nil {
		return deleted, trim.Error
	}
	deleted += trim.RowsAffected
	return deleted, nil
}

func compactMainSQLiteDB(db *gorm.DB, force bool) error {
	if db == nil {
		return nil
	}

	if !force {
		pageCount, freelistCount, err := readSQLitePageStats(db)
		if err != nil {
			return err
		}
		if pageCount <= 0 || freelistCount < sqliteCompactMinFreelistPages {
			return nil
		}
		if float64(freelistCount)/float64(pageCount) < sqliteCompactMinFreelistRatio {
			return nil
		}
	}

	if err := db.Exec("PRAGMA wal_checkpoint(TRUNCATE)").Error; err != nil {
		logger.Warning("main sqlite wal checkpoint failed: ", err)
	}
	return db.Exec("VACUUM").Error
}

func readSQLitePageStats(db *gorm.DB) (int64, int64, error) {
	if db == nil {
		return 0, 0, fmt.Errorf("database is not initialized")
	}

	type pageCountRow struct {
		PageCount int64 `gorm:"column:page_count"`
	}
	type freelistCountRow struct {
		FreelistCount int64 `gorm:"column:freelist_count"`
	}

	pageCount := pageCountRow{}
	if err := db.Raw("PRAGMA page_count").Scan(&pageCount).Error; err != nil {
		return 0, 0, err
	}

	freelistCount := freelistCountRow{}
	if err := db.Raw("PRAGMA freelist_count").Scan(&freelistCount).Error; err != nil {
		return 0, 0, err
	}

	return pageCount.PageCount, freelistCount.FreelistCount, nil
}
