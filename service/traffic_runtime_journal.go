package service

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/alireza0/s-ui/config"
	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/logger"
	"gorm.io/gorm"
)

const (
	trafficRuntimeJournalSchema          = 1
	trafficRuntimeJournalFlushThreshold  = 3 * 1024 * 1024
	trafficRuntimeJournalMaxBytes        = 9 * 1024 * 1024
	trafficRuntimeJournalFilename        = "kwor-traffic-runtime.v1.json"
	trafficRuntimeJournalBackupSuffix    = ".bak"
	trafficRuntimeArchiveFilename        = "kwor-traffic-runtime.v2.jsonl"
	trafficRuntimeArchiveRotateBytes     = 16 * 1024 * 1024
	trafficRuntimeArchiveFlushBytes      = 64 * 1024
	trafficRuntimeArchiveFlushInterval   = time.Minute
	trafficRuntimeJournalMetaTable       = "traffic_runtime_journal_meta"
	trafficRuntimeJournalCheckpointTable = "traffic_runtime_journal_checkpoints"
	trafficRuntimeJournalStagingTable    = "traffic_runtime_journal_staging"
	trafficRuntimeJournalEntryTable      = "traffic_runtime_journal_entries"
	trafficRuntimeJournalPendingLimit    = 16384
	// The entry table is part of the crash-recovery path, so it must remain
	// bounded even when a later SQLite flush keeps failing.  The in-memory tail
	// limit alone is not enough because one committed sample creates one durable
	// row before the next flush attempt.
	trafficRuntimeJournalEntryMaxRows    = 4096
	trafficRuntimeJournalEntryMaxBytes   = 16 * 1024 * 1024
	trafficRuntimeArchiveMaxPendingBytes = 512 * 1024
	trafficRuntimeArchiveKeyTable        = "traffic_runtime_archive_keys"
)

var errTrafficRuntimeJournalTooLarge = errors.New("traffic runtime journal exceeds its bounded capacity")
var errTrafficRuntimeJournalStageCorrupt = errors.New("traffic runtime journal staging record is corrupt")
var errTrafficRuntimeJournalEntryCapacity = errors.New("traffic runtime journal entry table is full")

var trafficRuntimeJournalPathFn = func() string {
	return filepath.Join(filepath.Dir(config.GetDBPath()), trafficRuntimeJournalFilename)
}

var trafficRuntimeArchivePathFn = func() string {
	return filepath.Join(filepath.Dir(config.GetDBPath()), trafficRuntimeArchiveFilename)
}

var quarantineTrafficRuntimeJournalFilesFn = quarantineTrafficRuntimeJournalFiles

type trafficRuntimeJournalPayload struct {
	Schema     int           `json:"schema"`
	Generation string        `json:"generation"`
	Sequence   uint64        `json:"sequence"`
	Samples    []model.Stats `json:"samples"`
	Checksum   string        `json:"checksum"`
}

type trafficRuntimeStatsJournal struct {
	initMu sync.Mutex

	mu                   sync.Mutex
	initialized          bool
	generation           string
	archiveKey           string
	checkpoint           uint64
	sequence             uint64
	version              uint64
	pending              map[string]model.Stats
	archivePending       []trafficRuntimeArchiveRecord
	archiveBytes         int
	archiveInFlightBytes int
	archiveLastFlush     time.Time
	archiveFlushing      bool
	archiveDropWarned    bool
}

var runtimeTrafficStats = &trafficRuntimeStatsJournal{
	pending: make(map[string]model.Stats),
}

// trafficRuntimeJournalTransactionMu covers the small SQLite transaction that
// advances nft counter baselines together with the journal staging snapshot.
// Keeping flushes outside that transaction prevents a flush from observing a
// sidecar written for a transaction that has not committed yet.
var trafficRuntimeJournalTransactionMu sync.Mutex

type trafficRuntimeJournalStage struct {
	payload  trafficRuntimeJournalPayload
	raw      []byte
	pending  map[string]model.Stats
	flushNow bool
	samples  []model.Stats
}

// trafficRuntimeArchiveRecord is an optional long-lived, append-only audit
// record. It deliberately carries an opaque entity token instead of a tag.
// SQLite staging remains the recovery authority for unflushed traffic.
type trafficRuntimeArchiveRecord struct {
	Sequence  uint64 `json:"seq"`
	DateTime  int64  `json:"t"`
	Namespace string `json:"ns"`
	Entity    string `json:"e"`
	Direction bool   `json:"d"`
	Traffic   int64  `json:"v"`
}

func lockTrafficRuntimeJournalTransaction() func() {
	trafficRuntimeJournalTransactionMu.Lock()
	return trafficRuntimeJournalTransactionMu.Unlock
}

func init() {
	database.RegisterDBResetHook(func() {
		runtimeTrafficStats.resetForDatabaseReload()
	})
	database.RegisterDBBeforeRestoreHook(PauseRuntimeSamplerForDatabaseRestore)
	database.RegisterDBRestoreAbortHook(ResumeRuntimeSamplerAfterDatabaseRestoreFailure)
	database.RegisterDBBeforeBackupHook(func() error {
		return FlushTrafficRuntimeJournal()
	})
	database.RegisterDBBeforeRestoreHook(func() error {
		return FlushTrafficRuntimeJournal()
	})
	database.RegisterDBBeforeRestoreHook(func() error {
		return QuarantineTrafficRuntimeJournalBeforeDatabaseRestore()
	})
	database.RegisterDBAfterRestoreHook(func() {
		if err := InvalidateTrafficRuntimeJournalForDatabaseRestore(); err != nil {
			// Restore has already replaced the database at this point. The
			// generation rotation still prevents replay even if file quarantine
			// or a best-effort cleanup failed.
			logger.Warning("invalidate traffic runtime journal after database restore failed: ", err)
		}
	})
}

// PrepareTrafficRuntimeJournalOnStartup restores a durable, not-yet-flushed
// history tail before the runtime sampler starts collecting new counters.
func PrepareTrafficRuntimeJournalOnStartup() error {
	unlock := lockTrafficRuntimeJournalTransaction()
	defer unlock()
	if err := runtimeTrafficStats.ensureReady(); err != nil {
		return err
	}
	if err := runtimeTrafficStats.restore(); err != nil {
		return err
	}
	return runtimeTrafficStats.flush()
}

// StageTrafficRuntimeStats records compact history deltas in the durable tail.
// SQLite receives those minute buckets on the sampler's flush cadence.
func StageTrafficRuntimeStats(samples []model.Stats) error {
	if len(samples) == 0 {
		return nil
	}

	unlock := lockTrafficRuntimeJournalTransaction()
	defer unlock()

	if err := runtimeTrafficStats.ensureReady(); err != nil {
		return err
	}
	if err := EnsureHistoryStorageReady(); err != nil {
		return err
	}
	db := database.GetDB()
	if db == nil {
		return fmt.Errorf("traffic runtime journal database is not initialized")
	}
	tx := db.Begin()
	if tx.Error != nil {
		return tx.Error
	}
	stage, err := stageTrafficRuntimeStatsForTransaction(tx, samples)
	if err != nil {
		tx.Rollback()
		return err
	}
	if err := tx.Commit().Error; err != nil {
		discardStagedTrafficRuntimeStats(stage)
		return err
	}
	if commitStagedTrafficRuntimeStats(stage) {
		_ = runtimeTrafficStats.flush()
	}
	return nil
}

// stageTrafficRuntimeStatsForTransaction persists a sidecar candidate before
// the counter baseline transaction commits, then stores that exact candidate
// in the same SQLite transaction. The caller must hold
// lockTrafficRuntimeJournalTransaction and finalize the returned stage only
// after Commit succeeds. On restart the SQLite staging snapshot is the source
// of truth, so a pre-commit sidecar can never be replayed as traffic.
func stageTrafficRuntimeStatsForTransaction(tx *gorm.DB, samples []model.Stats) (*trafficRuntimeJournalStage, error) {
	if len(samples) == 0 {
		return nil, nil
	}
	stage, err := runtimeTrafficStats.stage(samples)
	if err != nil {
		if tx == nil {
			return nil, err
		}
		if fallbackErr := upsertStatsTrafficBatch(tx, samples); fallbackErr != nil {
			return nil, fmt.Errorf("traffic runtime journal failed: %v; synchronous history fallback failed: %w", err, fallbackErr)
		}
		return nil, nil
	}
	if stage == nil {
		return nil, nil
	}
	if err := persistTrafficRuntimeJournalStage(tx, stage); err != nil {
		if errors.Is(err, errTrafficRuntimeJournalEntryCapacity) {
			// The journal is optional once the counter-baseline transaction is
			// already open.  Write this bounded batch directly to stats rather
			// than allowing durable recovery rows to grow without limit while a
			// later flush is failing.
			if fallbackErr := upsertStatsTrafficBatch(tx, samples); fallbackErr != nil {
				return nil, fmt.Errorf("traffic runtime journal is full; synchronous history fallback failed: %w", fallbackErr)
			}
			return nil, nil
		}
		discardStagedTrafficRuntimeStats(stage)
		return nil, err
	}
	return stage, nil
}

func commitStagedTrafficRuntimeStats(stage *trafficRuntimeJournalStage) bool {
	if stage == nil {
		return false
	}
	runtimeTrafficStats.commitStage(stage)
	return stage.flushNow
}

func discardStagedTrafficRuntimeStats(stage *trafficRuntimeJournalStage) {
	if stage == nil {
		return
	}
	_ = runtimeTrafficStats.abortStage()
}

func FlushTrafficRuntimeJournal() error {
	unlock := lockTrafficRuntimeJournalTransaction()
	defer unlock()
	return runtimeTrafficStats.flush()
}

// QuarantineTrafficRuntimeJournalBeforeDatabaseRestore separates the old
// database's filesystem mirror before SQLite is replaced. The normal restore
// hook rotates the database generation afterward as a second protection.
func QuarantineTrafficRuntimeJournalBeforeDatabaseRestore() error {
	unlock := lockTrafficRuntimeJournalTransaction()
	defer unlock()
	return errors.Join(
		quarantineTrafficRuntimeJournalFilesFn(),
		quarantineTrafficRuntimeArchiveFiles(),
	)
}

// InvalidateTrafficRuntimeJournalForDatabaseRestore removes the old database's
// sidecar after a successful import/recovery.  It is intentionally separate
// from the normal DB reset hook, which also runs during ordinary startup.
func InvalidateTrafficRuntimeJournalForDatabaseRestore() error {
	unlock := lockTrafficRuntimeJournalTransaction()
	defer unlock()
	runtimeTrafficStats.resetForDatabaseReloadLocked()
	// A restored database may contain the same journal metadata generation as
	// the source database. Rotate it before the next sampler pass so a stale
	// sidecar can never become valid merely because file quarantine was delayed
	// or failed. The file is still quarantined on a best-effort basis below.
	rotateErr := runtimeTrafficStats.rotateGenerationForDatabaseRestore()
	quarantineErr := errors.Join(
		quarantineTrafficRuntimeJournalFilesFn(),
		quarantineTrafficRuntimeArchiveFiles(),
	)
	return errors.Join(rotateErr, quarantineErr)
}

func (s *trafficRuntimeStatsJournal) resetForDatabaseReload() {
	unlock := lockTrafficRuntimeJournalTransaction()
	defer unlock()
	s.resetForDatabaseReloadLocked()
}

func (s *trafficRuntimeStatsJournal) resetForDatabaseReloadLocked() {
	s.initMu.Lock()
	s.initialized = false
	s.generation = ""
	s.archiveKey = ""
	s.checkpoint = 0
	s.sequence = 0
	s.initMu.Unlock()

	s.mu.Lock()
	s.pending = make(map[string]model.Stats)
	s.archivePending = nil
	s.archiveBytes = 0
	s.archiveInFlightBytes = 0
	s.archiveLastFlush = time.Time{}
	s.archiveFlushing = false
	s.archiveDropWarned = false
	s.version++
	s.mu.Unlock()
}

func (s *trafficRuntimeStatsJournal) ensureReady() error {
	s.initMu.Lock()
	defer s.initMu.Unlock()
	if s.initialized {
		return nil
	}

	db := database.GetDB()
	if db == nil {
		return fmt.Errorf("traffic runtime journal requires initialized database")
	}
	if err := db.Exec(fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		generation TEXT NOT NULL
	)`, trafficRuntimeJournalMetaTable)).Error; err != nil {
		return err
	}
	if err := db.Exec(fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		sequence INTEGER NOT NULL DEFAULT 0
	)`, trafficRuntimeJournalCheckpointTable)).Error; err != nil {
		return err
	}
	if err := db.Exec(fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		generation TEXT NOT NULL,
		sequence INTEGER NOT NULL,
		payload BLOB NOT NULL
	)`, trafficRuntimeJournalStagingTable)).Error; err != nil {
		return err
	}
	if err := db.Exec(fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
		generation TEXT NOT NULL,
		sequence INTEGER NOT NULL,
		payload BLOB NOT NULL,
		PRIMARY KEY (generation, sequence)
	)`, trafficRuntimeJournalEntryTable)).Error; err != nil {
		return err
	}
	if err := db.Exec(fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		archive_key TEXT NOT NULL
	)`, trafficRuntimeArchiveKeyTable)).Error; err != nil {
		return err
	}

	var generation string
	if err := db.Raw(fmt.Sprintf("SELECT generation FROM %s WHERE id = 1", trafficRuntimeJournalMetaTable)).Scan(&generation).Error; err != nil {
		return err
	}
	if strings.TrimSpace(generation) == "" {
		var err error
		generation, err = newTrafficRuntimeJournalGeneration()
		if err != nil {
			return err
		}
		if err := db.Exec(fmt.Sprintf(`INSERT INTO %s (id, generation) VALUES (1, ?)
			ON CONFLICT(id) DO UPDATE SET generation = excluded.generation`, trafficRuntimeJournalMetaTable), generation).Error; err != nil {
			return err
		}
	}
	var archiveKey string
	if err := db.Raw(fmt.Sprintf("SELECT archive_key FROM %s WHERE id = 1", trafficRuntimeArchiveKeyTable)).Scan(&archiveKey).Error; err != nil {
		return err
	}
	if strings.TrimSpace(archiveKey) == "" {
		key, err := newTrafficRuntimeJournalGeneration()
		if err != nil {
			return err
		}
		archiveKey = key
		if err := db.Exec(fmt.Sprintf(`INSERT INTO %s (id, archive_key) VALUES (1, ?)
			ON CONFLICT(id) DO UPDATE SET archive_key = excluded.archive_key`, trafficRuntimeArchiveKeyTable), archiveKey).Error; err != nil {
			return err
		}
	}

	var checkpoint uint64
	if err := db.Raw(fmt.Sprintf("SELECT sequence FROM %s WHERE id = 1", trafficRuntimeJournalCheckpointTable)).Scan(&checkpoint).Error; err != nil {
		return err
	}
	s.generation = generation
	s.archiveKey = archiveKey
	s.checkpoint = checkpoint
	s.sequence = checkpoint
	s.initialized = true
	return nil
}

func (s *trafficRuntimeStatsJournal) restore() error {
	db := database.GetDB()
	if db == nil {
		return fmt.Errorf("traffic runtime journal database is not initialized")
	}

	if restored, err := s.restoreIncrementalEntries(db); err != nil {
		return err
	} else if restored {
		// v2 entries are the committed recovery authority. Any remaining v1
		// mirrors belong to an older write strategy and must never be replayed
		// alongside the same SQLite tail.
		return quarantineTrafficRuntimeJournalFiles()
	}

	staged, hasStaged, stagedErr := readTrafficRuntimeJournalStage(db)
	if stagedErr != nil {
		if errors.Is(stagedErr, errTrafficRuntimeJournalStageCorrupt) {
			cleanupErr := errors.Join(
				deleteAllTrafficRuntimeJournalStages(db),
				quarantineTrafficRuntimeJournalFiles(),
			)
			if cleanupErr != nil {
				return errors.Join(stagedErr, cleanupErr)
			}
			logger.Warning("discarded corrupt traffic runtime journal staging record: ", stagedErr)
			return nil
		}
		return stagedErr
	}

	fileResult, fileErr := readTrafficRuntimeJournalFileDetailed()
	quarantineErr := quarantineInvalidTrafficRuntimeJournalFiles(fileResult.invalidPaths)
	if quarantineErr != nil {
		return quarantineErr
	}
	if fileResult.sourcePath == "" && len(fileResult.invalidPaths) > 0 {
		fileErr = nil
	}

	s.initMu.Lock()
	generation := s.generation
	checkpoint := s.checkpoint
	s.initMu.Unlock()

	if hasStaged {
		if staged.Generation != generation || staged.Sequence <= checkpoint {
			if err := deleteTrafficRuntimeJournalStage(db, staged.Generation, staged.Sequence); err != nil {
				return err
			}
			return removeTrafficRuntimeJournalFiles()
		}

		s.mu.Lock()
		s.pending = make(map[string]model.Stats, len(staged.Samples))
		for _, sample := range staged.Samples {
			s.addPendingLocked(sample)
		}
		s.sequence = staged.Sequence
		s.version++
		s.mu.Unlock()

		fileMatchesStage := fileResult.sourcePath != "" &&
			fileResult.payload.Generation == staged.Generation &&
			fileResult.payload.Sequence == staged.Sequence &&
			fileResult.payload.Checksum == staged.Checksum
		needsMirrorRewrite := !fileMatchesStage || fileResult.sourcePath != trafficRuntimeJournalPath()
		if needsMirrorRewrite && fileResult.sourcePath != "" {
			// Rewrite from the SQLite staging source after quarantining both
			// filesystem copies. Otherwise a stale or damaged primary could be
			// copied over a valid backup immediately before the repaired primary is
			// published.
			if err := quarantineTrafficRuntimeJournalFiles(); err != nil {
				return err
			}
		}
		if needsMirrorRewrite {
			if err := writeTrafficRuntimeJournalFile(staged.raw); err != nil {
				return err
			}
		}
		return nil
	}

	if fileErr != nil {
		if errors.Is(fileErr, os.ErrNotExist) {
			return nil
		}
		return fileErr
	}
	if fileResult.sourcePath == "" {
		return nil
	}
	if fileResult.payload.Generation != generation {
		return quarantineTrafficRuntimeJournalFiles()
	}
	if fileResult.payload.Sequence <= checkpoint {
		return removeTrafficRuntimeJournalFiles()
	}

	// A sidecar without the matching SQLite staging record was produced before
	// its counter-baseline transaction committed. Never replay it on startup:
	// doing so would double count the next cumulative nft delta.
	return quarantineTrafficRuntimeJournalFiles()
}

type trafficRuntimeJournalStagingRecord struct {
	Generation string
	Sequence   uint64
	Payload    []byte

	trafficRuntimeJournalPayload
	raw []byte
}

func persistTrafficRuntimeJournalStage(tx *gorm.DB, stage *trafficRuntimeJournalStage) error {
	if tx == nil {
		return fmt.Errorf("traffic runtime journal staging requires a database transaction")
	}
	if stage == nil || len(stage.raw) == 0 || len(stage.raw) > trafficRuntimeJournalMaxBytes {
		return errTrafficRuntimeJournalTooLarge
	}
	var usage struct {
		Rows  int64 `gorm:"column:rows"`
		Bytes int64 `gorm:"column:bytes"`
	}
	if err := tx.Raw(fmt.Sprintf(`SELECT COUNT(*) AS rows,
		COALESCE(SUM(length(payload)), 0) AS bytes
		FROM %s WHERE generation = ?`, trafficRuntimeJournalEntryTable), stage.payload.Generation).Scan(&usage).Error; err != nil {
		return err
	}
	if usage.Rows >= trafficRuntimeJournalEntryMaxRows ||
		usage.Bytes > int64(trafficRuntimeJournalEntryMaxBytes)-int64(len(stage.raw)) {
		return errTrafficRuntimeJournalEntryCapacity
	}
	return tx.Exec(fmt.Sprintf(`INSERT INTO %s (generation, sequence, payload) VALUES (?, ?, ?)
		ON CONFLICT(generation, sequence) DO UPDATE SET payload = excluded.payload`, trafficRuntimeJournalEntryTable),
		stage.payload.Generation,
		stage.payload.Sequence,
		stage.raw,
	).Error
}

func deleteAllTrafficRuntimeJournalStages(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("traffic runtime journal database is not initialized")
	}
	return errors.Join(
		db.Exec(fmt.Sprintf("DELETE FROM %s", trafficRuntimeJournalStagingTable)).Error,
		db.Exec(fmt.Sprintf("DELETE FROM %s", trafficRuntimeJournalEntryTable)).Error,
	)
}

func readTrafficRuntimeJournalStage(db *gorm.DB) (trafficRuntimeJournalStagingRecord, bool, error) {
	result := trafficRuntimeJournalStagingRecord{}
	if db == nil {
		return result, false, fmt.Errorf("traffic runtime journal database is not initialized")
	}
	query := db.Raw(fmt.Sprintf("SELECT generation, sequence, payload FROM %s WHERE id = 1", trafficRuntimeJournalStagingTable)).Scan(&result)
	if query.Error != nil {
		return result, false, query.Error
	}
	if query.RowsAffected == 0 {
		return result, false, nil
	}
	if len(result.Payload) == 0 || len(result.Payload) > trafficRuntimeJournalMaxBytes {
		return result, false, fmt.Errorf("%w: payload size is invalid", errTrafficRuntimeJournalStageCorrupt)
	}
	payload, err := unmarshalTrafficRuntimeJournalPayload(result.Payload)
	if err != nil {
		return result, false, fmt.Errorf("%w: %v", errTrafficRuntimeJournalStageCorrupt, err)
	}
	if payload.Generation != result.Generation || payload.Sequence != result.Sequence {
		return result, false, fmt.Errorf("%w: metadata does not match payload", errTrafficRuntimeJournalStageCorrupt)
	}
	result.trafficRuntimeJournalPayload = payload
	result.raw = append([]byte(nil), result.Payload...)
	return result, true, nil
}

func deleteTrafficRuntimeJournalStage(db *gorm.DB, generation string, sequence uint64) error {
	if db == nil {
		return fmt.Errorf("traffic runtime journal database is not initialized")
	}
	return db.Exec(fmt.Sprintf("DELETE FROM %s WHERE id = 1 AND generation = ? AND sequence <= ?", trafficRuntimeJournalStagingTable), generation, sequence).Error
}

func (s *trafficRuntimeStatsJournal) clearStagingForDatabaseRestore() error {
	if err := s.ensureReady(); err != nil {
		return err
	}
	db := database.GetDB()
	if db == nil {
		return fmt.Errorf("traffic runtime journal database is not initialized")
	}
	return db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(fmt.Sprintf("DELETE FROM %s", trafficRuntimeJournalStagingTable)).Error; err != nil {
			return err
		}
		return tx.Exec(fmt.Sprintf("DELETE FROM %s", trafficRuntimeJournalEntryTable)).Error
	})
}

// rotateGenerationForDatabaseRestore creates a fresh accounting generation in
// the database that is now active. The generation, checkpoint and staging row
// are changed in one short transaction; an old mirror therefore fails the
// generation check even if its filesystem quarantine cannot complete.
func (s *trafficRuntimeStatsJournal) rotateGenerationForDatabaseRestore() error {
	if err := s.ensureReady(); err != nil {
		return err
	}
	db := database.GetDB()
	if db == nil {
		return fmt.Errorf("traffic runtime journal database is not initialized")
	}
	generation, err := newTrafficRuntimeJournalGeneration()
	if err != nil {
		return err
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec(fmt.Sprintf("UPDATE %s SET generation = ? WHERE id = 1", trafficRuntimeJournalMetaTable), generation).Error; err != nil {
			return err
		}
		if err := tx.Exec(fmt.Sprintf("INSERT INTO %s (id, sequence) VALUES (1, 0) ON CONFLICT(id) DO UPDATE SET sequence = 0", trafficRuntimeJournalCheckpointTable)).Error; err != nil {
			return err
		}
		if err := tx.Exec(fmt.Sprintf("DELETE FROM %s", trafficRuntimeJournalStagingTable)).Error; err != nil {
			return err
		}
		return tx.Exec(fmt.Sprintf("DELETE FROM %s", trafficRuntimeJournalEntryTable)).Error
	}); err != nil {
		return err
	}
	s.initMu.Lock()
	s.generation = generation
	s.checkpoint = 0
	s.sequence = 0
	s.initialized = true
	s.initMu.Unlock()
	s.mu.Lock()
	s.pending = make(map[string]model.Stats)
	s.archivePending = nil
	s.archiveBytes = 0
	s.archiveInFlightBytes = 0
	s.archiveLastFlush = time.Time{}
	s.archiveFlushing = false
	s.archiveDropWarned = false
	s.version++
	s.mu.Unlock()
	return nil
}

type trafficRuntimeJournalEntry struct {
	Generation string
	Sequence   uint64
	Payload    []byte
}

// restoreIncrementalEntries restores v2's append-only SQLite tail. Unlike the
// legacy singleton snapshot, each committed sampling transaction contributes a
// small independent payload, so startup never needs a sidecar file to decide
// whether a counter delta was committed.
func (s *trafficRuntimeStatsJournal) restoreIncrementalEntries(db *gorm.DB) (bool, error) {
	if db == nil {
		return false, fmt.Errorf("traffic runtime journal database is not initialized")
	}
	s.initMu.Lock()
	generation := s.generation
	checkpoint := s.checkpoint
	s.initMu.Unlock()
	if err := db.Exec(
		fmt.Sprintf("DELETE FROM %s WHERE generation = ? AND sequence <= ?", trafficRuntimeJournalEntryTable),
		generation,
		checkpoint,
	).Error; err != nil {
		return false, err
	}
	entries := make([]trafficRuntimeJournalEntry, 0)
	if err := db.Raw(fmt.Sprintf(`SELECT generation, sequence, payload FROM %s
		WHERE generation = ? AND sequence > ? ORDER BY sequence ASC`, trafficRuntimeJournalEntryTable), generation, checkpoint).Scan(&entries).Error; err != nil {
		return false, err
	}
	if len(entries) == 0 {
		return false, nil
	}
	pending := make(map[string]model.Stats)
	sequence := checkpoint
	hadEntries := len(entries) > 0
	for _, entry := range entries {
		if entry.Sequence > sequence && entry.Sequence-sequence > 1 {
			// Entries are independently recoverable, but a gap means at least one
			// committed tail record is missing or was quarantined. Keep restoring
			// later valid records while making the potential accounting loss
			// explicit instead of silently treating the sequence as contiguous.
			logger.Warning("traffic runtime journal sequence gap before ", entry.Sequence, "; previous sequence ", sequence)
		}
		if entry.Sequence <= sequence || len(entry.Payload) == 0 || len(entry.Payload) > trafficRuntimeJournalMaxBytes {
			if err := deleteTrafficRuntimeJournalEntry(db, entry.Generation, entry.Sequence); err != nil {
				return false, fmt.Errorf("discard invalid traffic runtime journal entry %d: %w", entry.Sequence, err)
			}
			logger.Warning("discarded invalid traffic runtime journal entry sequence ", entry.Sequence)
			continue
		}
		var samples []model.Stats
		if err := json.Unmarshal(entry.Payload, &samples); err != nil {
			if deleteErr := deleteTrafficRuntimeJournalEntry(db, entry.Generation, entry.Sequence); deleteErr != nil {
				return false, fmt.Errorf("discard corrupt traffic runtime journal entry %d: %w", entry.Sequence, deleteErr)
			}
			logger.Warning("discarded corrupt traffic runtime journal entry sequence ", entry.Sequence, ": ", err)
			continue
		}
		accepted := false
		for _, sample := range samples {
			if addTrafficRuntimePendingStat(pending, sample) {
				accepted = true
			}
		}
		if !accepted {
			if err := deleteTrafficRuntimeJournalEntry(db, entry.Generation, entry.Sequence); err != nil {
				return false, fmt.Errorf("discard empty traffic runtime journal entry %d: %w", entry.Sequence, err)
			}
			logger.Warning("discarded empty traffic runtime journal entry sequence ", entry.Sequence)
			continue
		}
		sequence = entry.Sequence
	}
	if !hadEntries {
		return false, nil
	}
	s.mu.Lock()
	s.pending = pending
	s.sequence = sequence
	s.version++
	s.mu.Unlock()
	return true, nil
}

func deleteTrafficRuntimeJournalEntry(db *gorm.DB, generation string, sequence uint64) error {
	if db == nil {
		return fmt.Errorf("traffic runtime journal database is not initialized")
	}
	return db.Exec(
		fmt.Sprintf("DELETE FROM %s WHERE generation = ? AND sequence = ?", trafficRuntimeJournalEntryTable),
		generation,
		sequence,
	).Error
}

func quarantineInvalidTrafficRuntimeJournalFiles(paths []string) error {
	var errs []error
	for _, path := range paths {
		if err := quarantineTrafficRuntimeJournalFile(path); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func (s *trafficRuntimeStatsJournal) stage(samples []model.Stats) (*trafficRuntimeJournalStage, error) {
	if err := s.ensureReady(); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	acceptedSamples := compactTrafficRuntimeSamples(samples)
	accepted := len(acceptedSamples) > 0
	if !accepted {
		return nil, nil
	}
	sequence := s.sequence + 1
	s.initMu.Lock()
	generation := s.generation
	s.initMu.Unlock()
	raw, err := json.Marshal(acceptedSamples)
	if err != nil {
		return nil, err
	}
	if len(raw) > trafficRuntimeJournalMaxBytes {
		return nil, errTrafficRuntimeJournalTooLarge
	}
	return &trafficRuntimeJournalStage{
		payload:  trafficRuntimeJournalPayload{Schema: trafficRuntimeJournalSchema, Generation: generation, Sequence: sequence},
		raw:      raw,
		flushNow: len(s.pending)+len(acceptedSamples) >= trafficRuntimeJournalPendingLimit,
		samples:  acceptedSamples,
	}, nil
}

func (s *trafficRuntimeStatsJournal) commitStage(stage *trafficRuntimeJournalStage) {
	if stage == nil {
		return
	}
	s.mu.Lock()
	for _, sample := range stage.samples {
		s.addPendingLocked(sample)
	}
	s.sequence = stage.payload.Sequence
	s.queueArchiveLocked(stage.samples, stage.payload.Sequence)
	s.version++
	s.mu.Unlock()
}

func (s *trafficRuntimeStatsJournal) abortStage() error {
	return nil
}

func cloneTrafficRuntimePendingStats(source map[string]model.Stats) map[string]model.Stats {
	cloned := make(map[string]model.Stats, len(source))
	for key, value := range source {
		cloned[key] = value
	}
	return cloned
}

func (s *trafficRuntimeStatsJournal) flush() error {
	if err := s.ensureReady(); err != nil {
		return err
	}
	if err := EnsureHistoryStorageReady(); err != nil {
		return err
	}

	s.mu.Lock()
	if len(s.pending) == 0 {
		s.mu.Unlock()
		return s.flushArchive(true)
	}

	sequence := s.sequence
	samples := s.sortedPendingLocked()
	s.initMu.Lock()
	generation := s.generation
	s.initMu.Unlock()
	db := database.GetDB()
	if db == nil {
		s.mu.Unlock()
		return fmt.Errorf("traffic runtime journal database is not initialized")
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		var applied uint64
		if err := tx.Raw(fmt.Sprintf("SELECT sequence FROM %s WHERE id = 1", trafficRuntimeJournalCheckpointTable)).Scan(&applied).Error; err != nil {
			return err
		}
		if applied >= sequence {
			if err := deleteTrafficRuntimeJournalStage(tx, generation, sequence); err != nil {
				return err
			}
			return tx.Exec(fmt.Sprintf("DELETE FROM %s WHERE generation = ? AND sequence <= ?", trafficRuntimeJournalEntryTable), generation, sequence).Error
		}
		if err := upsertStatsTrafficBatch(tx, samples); err != nil {
			return err
		}
		if err := tx.Exec(
			fmt.Sprintf(`INSERT INTO %s (id, sequence) VALUES (1, ?)
			ON CONFLICT(id) DO UPDATE SET sequence = excluded.sequence`, trafficRuntimeJournalCheckpointTable),
			sequence,
		).Error; err != nil {
			return err
		}
		if err := deleteTrafficRuntimeJournalStage(tx, generation, sequence); err != nil {
			return err
		}
		return tx.Exec(fmt.Sprintf("DELETE FROM %s WHERE generation = ? AND sequence <= ?", trafficRuntimeJournalEntryTable), generation, sequence).Error
	}); err != nil {
		s.mu.Unlock()
		return err
	}

	s.pending = make(map[string]model.Stats)
	s.version++
	s.initMu.Lock()
	if sequence > s.checkpoint {
		s.checkpoint = sequence
	}
	s.initMu.Unlock()
	s.mu.Unlock()
	archiveErr := s.flushArchive(true)
	removeErr := removeTrafficRuntimeJournalFiles()
	return errors.Join(archiveErr, removeErr)
}

func compactTrafficRuntimeSamples(samples []model.Stats) []model.Stats {
	compact := make(map[string]model.Stats, len(samples))
	for _, sample := range samples {
		if !addTrafficRuntimePendingStat(compact, sample) {
			continue
		}
	}
	return sortedTrafficRuntimePendingStats(compact)
}

func (s *trafficRuntimeStatsJournal) addPendingLocked(sample model.Stats) {
	addTrafficRuntimePendingStat(s.pending, sample)
}

func addTrafficRuntimePendingStat(pending map[string]model.Stats, sample model.Stats) bool {
	if pending == nil {
		return false
	}
	resource := normalizeStatsResource(sample.Resource)
	tag := strings.TrimSpace(sample.Tag)
	if resource == "" || tag == "" || sample.Traffic <= 0 {
		return false
	}
	sample.Resource = resource
	sample.Tag = tag
	sample.DateTime = statsBucketStart(sample.DateTime)
	key := trafficRuntimeJournalSampleKey(sample)
	if current, ok := pending[key]; ok {
		current.Traffic += sample.Traffic
		pending[key] = current
		return true
	}
	pending[key] = sample
	return true
}

func (s *trafficRuntimeStatsJournal) sortedPendingLocked() []model.Stats {
	return sortedTrafficRuntimePendingStats(s.pending)
}

func sortedTrafficRuntimePendingStats(pending map[string]model.Stats) []model.Stats {
	samples := make([]model.Stats, 0, len(pending))
	for _, sample := range pending {
		samples = append(samples, sample)
	}
	sort.Slice(samples, func(i, j int) bool {
		if samples[i].DateTime != samples[j].DateTime {
			return samples[i].DateTime < samples[j].DateTime
		}
		if samples[i].Resource != samples[j].Resource {
			return samples[i].Resource < samples[j].Resource
		}
		if samples[i].Tag != samples[j].Tag {
			return samples[i].Tag < samples[j].Tag
		}
		return !samples[i].Direction && samples[j].Direction
	})
	return samples
}

func (s *trafficRuntimeStatsJournal) payloadLocked() (trafficRuntimeJournalPayload, []byte, error) {
	return s.payloadForPendingLocked(s.pending, s.sequence)
}

func (s *trafficRuntimeStatsJournal) payloadForPendingLocked(pending map[string]model.Stats, sequence uint64) (trafficRuntimeJournalPayload, []byte, error) {
	s.initMu.Lock()
	generation := s.generation
	s.initMu.Unlock()
	payload := trafficRuntimeJournalPayload{
		Schema:     trafficRuntimeJournalSchema,
		Generation: generation,
		Sequence:   sequence,
		Samples:    sortedTrafficRuntimePendingStats(pending),
	}
	raw, err := marshalTrafficRuntimeJournalPayload(payload)
	return payload, raw, err
}

func trafficRuntimeJournalSampleKey(sample model.Stats) string {
	return fmt.Sprintf("%d\x00%s\x00%s\x00%t", sample.DateTime, sample.Resource, sample.Tag, sample.Direction)
}

func marshalTrafficRuntimeJournalPayload(payload trafficRuntimeJournalPayload) ([]byte, error) {
	payload.Checksum = ""
	unsigned, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(unsigned)
	payload.Checksum = hex.EncodeToString(sum[:])
	return json.Marshal(payload)
}

func unmarshalTrafficRuntimeJournalPayload(raw []byte) (trafficRuntimeJournalPayload, error) {
	payload := trafficRuntimeJournalPayload{}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return payload, err
	}
	if payload.Schema != trafficRuntimeJournalSchema || strings.TrimSpace(payload.Generation) == "" || payload.Sequence == 0 {
		return payload, fmt.Errorf("traffic runtime journal metadata is invalid")
	}
	checksum := strings.TrimSpace(payload.Checksum)
	if checksum == "" {
		return payload, fmt.Errorf("traffic runtime journal checksum is missing")
	}
	payload.Checksum = ""
	unsigned, err := json.Marshal(payload)
	if err != nil {
		return payload, err
	}
	sum := sha256.Sum256(unsigned)
	if !strings.EqualFold(checksum, hex.EncodeToString(sum[:])) {
		return payload, fmt.Errorf("traffic runtime journal checksum does not match")
	}
	payload.Checksum = checksum
	if !trafficRuntimeJournalPayloadHasUsableSamples(payload) {
		return payload, fmt.Errorf("traffic runtime journal samples are empty or invalid")
	}
	return payload, nil
}

func trafficRuntimeJournalPayloadHasUsableSamples(payload trafficRuntimeJournalPayload) bool {
	for _, sample := range payload.Samples {
		if normalizeStatsResource(sample.Resource) != "" &&
			strings.TrimSpace(sample.Tag) != "" && sample.Traffic > 0 {
			return true
		}
	}
	return false
}

func trafficRuntimeJournalPath() string {
	return trafficRuntimeJournalPathFn()
}

func trafficRuntimeArchivePath() string {
	return trafficRuntimeArchivePathFn()
}

func (s *trafficRuntimeStatsJournal) queueArchiveLocked(samples []model.Stats, sequence uint64) {
	if len(samples) == 0 {
		return
	}
	for _, sample := range samples {
		record := trafficRuntimeArchiveRecord{
			Sequence:  sequence,
			DateTime:  sample.DateTime,
			Namespace: trafficRuntimeArchiveNamespace(sample.Resource),
			Entity:    s.archiveEntityTokenLocked(sample.Resource, sample.Tag),
			Direction: sample.Direction,
			Traffic:   sample.Traffic,
		}
		if record.Namespace == "" || record.Entity == "" || record.Traffic <= 0 {
			continue
		}
		raw, err := json.Marshal(record)
		if err != nil {
			continue
		}
		if s.archiveInFlightBytes+s.archiveBytes+len(raw)+1 > trafficRuntimeArchiveMaxPendingBytes {
			if !s.archiveDropWarned {
				logger.Warning("traffic runtime archive queue reached its bounded capacity; dropping optional archive records until the next successful flush")
				s.archiveDropWarned = true
			}
			continue
		}
		s.archivePending = append(s.archivePending, record)
		s.archiveBytes += len(raw) + 1
	}
}

func trafficRuntimeArchiveNamespace(resource string) string {
	switch normalizeStatsResource(resource) {
	case "client":
		return "singbox_client"
	case "inbound":
		return "singbox_inbound"
	case "mihomo_client":
		return "mihomo_client"
	case "mihomo_inbound":
		return "mihomo_inbound"
	default:
		return ""
	}
}

func (s *trafficRuntimeStatsJournal) archiveEntityTokenLocked(resource string, tag string) string {
	s.initMu.Lock()
	key := s.archiveKey
	s.initMu.Unlock()
	namespace := trafficRuntimeArchiveNamespace(resource)
	if key == "" || namespace == "" || strings.TrimSpace(tag) == "" {
		return ""
	}
	mac := hmac.New(sha256.New, []byte(key))
	_, _ = io.WriteString(mac, namespace)
	_, _ = io.WriteString(mac, "\x00")
	_, _ = io.WriteString(mac, strings.TrimSpace(tag))
	return hex.EncodeToString(mac.Sum(nil)[:12])
}

func (s *trafficRuntimeStatsJournal) flushArchive(force bool) error {
	s.mu.Lock()
	if len(s.archivePending) == 0 || s.archiveFlushing ||
		(!force && s.archiveBytes < trafficRuntimeArchiveFlushBytes && time.Since(s.archiveLastFlush) < trafficRuntimeArchiveFlushInterval) {
		s.mu.Unlock()
		return nil
	}
	records := append([]trafficRuntimeArchiveRecord(nil), s.archivePending...)
	queuedBytes := s.archiveBytes
	s.archivePending = nil
	s.archiveBytes = 0
	s.archiveInFlightBytes = queuedBytes
	s.archiveFlushing = true
	s.mu.Unlock()

	err := appendTrafficRuntimeArchive(records)
	s.mu.Lock()
	s.archiveFlushing = false
	s.archiveInFlightBytes = 0
	if err != nil {
		// Preserve sequence order when a newly committed sample arrived while
		// the disk write was in flight. The archive is optional, but losing its
		// in-memory tail before the bounded retry path runs would be needless.
		s.archivePending = append(records, s.archivePending...)
		s.archiveBytes += queuedBytes
		for s.archiveBytes > trafficRuntimeArchiveMaxPendingBytes && len(s.archivePending) > 0 {
			last := s.archivePending[len(s.archivePending)-1]
			s.archivePending = s.archivePending[:len(s.archivePending)-1]
			if raw, marshalErr := json.Marshal(last); marshalErr == nil {
				s.archiveBytes -= len(raw) + 1
			}
		}
		s.mu.Unlock()
		return err
	}
	s.archiveLastFlush = time.Now()
	s.archiveDropWarned = false
	s.mu.Unlock()
	return nil
}

func appendTrafficRuntimeArchive(records []trafficRuntimeArchiveRecord) error {
	if len(records) == 0 {
		return nil
	}
	path := trafficRuntimeArchivePath()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("traffic runtime archive path must not be a symbolic link")
	}
	archiveBytes := 0
	for _, record := range records {
		raw, err := json.Marshal(record)
		if err != nil {
			return err
		}
		archiveBytes += len(raw) + 1
	}
	if info, err := os.Stat(path); err == nil && info.Size()+int64(archiveBytes) > trafficRuntimeArchiveRotateBytes {
		previous := path + ".1"
		if info, statErr := os.Lstat(previous); statErr == nil && info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("traffic runtime archive rotation path must not be a symbolic link")
		}
		_ = os.Remove(previous)
		if err := os.Rename(path, previous); err != nil {
			return err
		}
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return err
	}
	for _, record := range records {
		raw, marshalErr := json.Marshal(record)
		if marshalErr != nil {
			continue
		}
		if _, err := file.Write(append(raw, '\n')); err != nil {
			_ = file.Close()
			return err
		}
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return nil
}

func readTrafficRuntimeJournalFile() (trafficRuntimeJournalPayload, string, error) {
	result, err := readTrafficRuntimeJournalFileDetailed()
	return result.payload, result.sourcePath, err
}

type trafficRuntimeJournalReadResult struct {
	payload      trafficRuntimeJournalPayload
	sourcePath   string
	invalidPaths []string
}

func readTrafficRuntimeJournalFileDetailed() (trafficRuntimeJournalReadResult, error) {
	result := trafficRuntimeJournalReadResult{}
	paths := []string{trafficRuntimeJournalPath(), trafficRuntimeJournalPath() + trafficRuntimeJournalBackupSuffix}
	var lastErr error
	for _, path := range paths {
		raw, err := os.ReadFile(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			lastErr = err
			continue
		}
		if len(raw) > trafficRuntimeJournalMaxBytes {
			lastErr = errTrafficRuntimeJournalTooLarge
			result.invalidPaths = append(result.invalidPaths, path)
			continue
		}
		payload, decodeErr := unmarshalTrafficRuntimeJournalPayload(raw)
		if decodeErr == nil {
			result.payload = payload
			result.sourcePath = path
			return result, nil
		}
		lastErr = decodeErr
		result.invalidPaths = append(result.invalidPaths, path)
	}
	if lastErr != nil {
		return result, lastErr
	}
	return result, os.ErrNotExist
}

func quarantineTrafficRuntimeJournalFiles() error {
	var errs []error
	for _, path := range []string{trafficRuntimeJournalPath(), trafficRuntimeJournalPath() + trafficRuntimeJournalBackupSuffix} {
		if err := quarantineTrafficRuntimeJournalFile(path); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func quarantineTrafficRuntimeArchiveFiles() error {
	var errs []error
	for _, path := range []string{trafficRuntimeArchivePath(), trafficRuntimeArchivePath() + ".1"} {
		if err := quarantineTrafficRuntimeJournalFile(path); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func writeTrafficRuntimeJournalFile(raw []byte) error {
	if len(raw) > trafficRuntimeJournalMaxBytes {
		return errTrafficRuntimeJournalTooLarge
	}
	path := trafficRuntimeJournalPath()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}

	temp, err := os.CreateTemp(dir, ".kwor-traffic-runtime-*.tmp")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	cleanup := func() {
		_ = temp.Close()
		_ = os.Remove(tempPath)
	}
	if err := temp.Chmod(0o600); err != nil {
		cleanup()
		return err
	}
	if _, err := temp.Write(raw); err != nil {
		cleanup()
		return err
	}
	if err := temp.Sync(); err != nil {
		cleanup()
		return err
	}
	if err := temp.Close(); err != nil {
		_ = os.Remove(tempPath)
		return err
	}
	if err := copyTrafficRuntimeJournalBackup(path, path+trafficRuntimeJournalBackupSuffix); err != nil {
		_ = os.Remove(tempPath)
		return err
	}
	if err := os.Rename(tempPath, path); err != nil {
		_ = os.Remove(tempPath)
		return err
	}
	syncTrafficRuntimeJournalDirectory(dir)
	return nil
}

func copyTrafficRuntimeJournalBackup(sourcePath string, backupPath string) error {
	source, err := os.Open(sourcePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	defer source.Close()

	raw, err := io.ReadAll(io.LimitReader(source, trafficRuntimeJournalMaxBytes+1))
	if err != nil {
		return err
	}
	if len(raw) > trafficRuntimeJournalMaxBytes {
		// An oversized primary is invalid mirror data, not a reason to discard
		// the new bounded candidate or overwrite the existing backup.
		return nil
	}
	// A corrupt primary must never replace the last known-good backup. The
	// caller will still publish the new, already validated mirror as primary.
	if _, err := unmarshalTrafficRuntimeJournalPayload(raw); err != nil {
		return nil
	}

	temp, err := os.CreateTemp(filepath.Dir(backupPath), ".kwor-traffic-runtime-backup-*.tmp")
	if err != nil {
		return err
	}
	tempPath := temp.Name()
	cleanup := func() {
		_ = temp.Close()
		_ = os.Remove(tempPath)
	}
	if err := temp.Chmod(0o600); err != nil {
		cleanup()
		return err
	}
	if _, err := temp.Write(raw); err != nil {
		cleanup()
		return err
	}
	if err := temp.Sync(); err != nil {
		cleanup()
		return err
	}
	if err := temp.Close(); err != nil {
		_ = os.Remove(tempPath)
		return err
	}
	if err := os.Rename(tempPath, backupPath); err != nil {
		_ = os.Remove(tempPath)
		return err
	}
	return nil
}

func removeTrafficRuntimeJournalFiles() error {
	var errorsList []error
	for _, path := range []string{trafficRuntimeJournalPath(), trafficRuntimeJournalPath() + trafficRuntimeJournalBackupSuffix} {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			errorsList = append(errorsList, err)
		}
	}
	return errors.Join(errorsList...)
}

func quarantineTrafficRuntimeJournalFile(path string) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	base := fmt.Sprintf("%s.invalid-%d", path, time.Now().UnixNano())
	quarantinePath := base
	for suffix := 1; ; suffix++ {
		_, err := os.Lstat(quarantinePath)
		if errors.Is(err, os.ErrNotExist) {
			break
		}
		if err != nil {
			return err
		}
		quarantinePath = fmt.Sprintf("%s-%d", base, suffix)
	}
	if err := os.Rename(path, quarantinePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func syncTrafficRuntimeJournalDirectory(dir string) {
	handle, err := os.Open(dir)
	if err != nil {
		return
	}
	defer handle.Close()
	_ = handle.Sync()
}

func newTrafficRuntimeJournalGeneration() (string, error) {
	bytes := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func pendingTrafficRuntimeStatsForQuery(resource string, tag string, startUnix int64, bucketSeconds int64) []model.Stats {
	unlock := lockTrafficRuntimeJournalTransaction()
	defer unlock()
	return runtimeTrafficStats.snapshotPendingForQuery(resource, tag, startUnix, bucketSeconds)
}

func (s *trafficRuntimeStatsJournal) currentVersion() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.version
}

func (s *trafficRuntimeStatsJournal) snapshotPending() (uint64, []model.Stats) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.version, s.sortedPendingLocked()
}

// snapshotPendingForQuery filters while holding the journal lock so a history
// request does not allocate and sort every unflushed client/inbound bucket just
// to return one entity's timeline.
func (s *trafficRuntimeStatsJournal) snapshotPendingForQuery(resource string, tag string, startUnix int64, bucketSeconds int64) []model.Stats {
	resource = normalizeStatsResource(resource)
	tag = strings.TrimSpace(tag)
	if resource == "" || tag == "" || bucketSeconds <= 0 {
		return nil
	}
	allowed := make(map[string]struct{}, len(statsQueryResources(resource)))
	for _, candidate := range statsQueryResources(resource) {
		allowed[normalizeStatsResource(candidate)] = struct{}{}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	combined := make(map[string]model.Stats)
	for _, sample := range s.pending {
		if _, ok := allowed[normalizeStatsResource(sample.Resource)]; !ok || sample.Tag != tag || sample.DateTime < startUnix {
			continue
		}
		sample.DateTime = (sample.DateTime / bucketSeconds) * bucketSeconds
		sample.Resource = resource
		key := fmt.Sprintf("%d\x00%t", sample.DateTime, sample.Direction)
		if current, exists := combined[key]; exists {
			current.Traffic += sample.Traffic
			combined[key] = current
			continue
		}
		combined[key] = sample
	}
	return sortedPendingTrafficStats(combined)
}

func pendingTrafficRuntimeStatsFromSamples(samples []model.Stats, resource string, tag string, startUnix int64, bucketSeconds int64) []model.Stats {
	resource = normalizeStatsResource(resource)
	tag = strings.TrimSpace(tag)
	if resource == "" || tag == "" {
		return nil
	}
	allowed := make(map[string]struct{}, len(statsQueryResources(resource)))
	for _, candidate := range statsQueryResources(resource) {
		allowed[normalizeStatsResource(candidate)] = struct{}{}
	}

	combined := make(map[string]model.Stats)
	for _, sample := range samples {
		if _, ok := allowed[normalizeStatsResource(sample.Resource)]; !ok || sample.Tag != tag || sample.DateTime < startUnix {
			continue
		}
		sample.DateTime = (sample.DateTime / bucketSeconds) * bucketSeconds
		sample.Resource = resource
		key := fmt.Sprintf("%d\x00%t", sample.DateTime, sample.Direction)
		if current, exists := combined[key]; exists {
			current.Traffic += sample.Traffic
			combined[key] = current
			continue
		}
		combined[key] = sample
	}
	return sortedPendingTrafficStats(combined)
}

func sortedPendingTrafficStats(samples map[string]model.Stats) []model.Stats {
	result := make([]model.Stats, 0, len(samples))
	for _, sample := range samples {
		result = append(result, sample)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].DateTime != result[j].DateTime {
			return result[i].DateTime < result[j].DateTime
		}
		return !result[i].Direction && result[j].Direction
	})
	return result
}
