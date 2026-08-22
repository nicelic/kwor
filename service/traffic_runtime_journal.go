package service

import (
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
	trafficRuntimeJournalMetaTable       = "traffic_runtime_journal_meta"
	trafficRuntimeJournalCheckpointTable = "traffic_runtime_journal_checkpoints"
	trafficRuntimeJournalStagingTable    = "traffic_runtime_journal_staging"
)

var errTrafficRuntimeJournalTooLarge = errors.New("traffic runtime journal exceeds its bounded capacity")
var errTrafficRuntimeJournalStageCorrupt = errors.New("traffic runtime journal staging record is corrupt")

var trafficRuntimeJournalPathFn = func() string {
	return filepath.Join(filepath.Dir(config.GetDBPath()), trafficRuntimeJournalFilename)
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

	mu          sync.Mutex
	initialized bool
	generation  string
	checkpoint  uint64
	sequence    uint64
	version     uint64
	pending     map[string]model.Stats
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
	return quarantineTrafficRuntimeJournalFilesFn()
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
	quarantineErr := quarantineTrafficRuntimeJournalFilesFn()
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
	s.checkpoint = 0
	s.sequence = 0
	s.initMu.Unlock()

	s.mu.Lock()
	s.pending = make(map[string]model.Stats)
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

	var checkpoint uint64
	if err := db.Raw(fmt.Sprintf("SELECT sequence FROM %s WHERE id = 1", trafficRuntimeJournalCheckpointTable)).Scan(&checkpoint).Error; err != nil {
		return err
	}
	s.generation = generation
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
	return tx.Exec(fmt.Sprintf(`INSERT INTO %s (id, generation, sequence, payload) VALUES (1, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			generation = excluded.generation,
			sequence = excluded.sequence,
			payload = excluded.payload`, trafficRuntimeJournalStagingTable),
		stage.payload.Generation,
		stage.payload.Sequence,
		stage.raw,
	).Error
}

func deleteAllTrafficRuntimeJournalStages(db *gorm.DB) error {
	if db == nil {
		return fmt.Errorf("traffic runtime journal database is not initialized")
	}
	return db.Exec(fmt.Sprintf("DELETE FROM %s", trafficRuntimeJournalStagingTable)).Error
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
	return db.Exec(fmt.Sprintf("DELETE FROM %s", trafficRuntimeJournalStagingTable)).Error
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
		return tx.Exec(fmt.Sprintf("DELETE FROM %s", trafficRuntimeJournalStagingTable)).Error
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
	s.version++
	s.mu.Unlock()
	return nil
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
	candidate := cloneTrafficRuntimePendingStats(s.pending)
	accepted := false
	for _, sample := range samples {
		if addTrafficRuntimePendingStat(candidate, sample) {
			accepted = true
		}
	}
	if !accepted {
		return nil, nil
	}
	sequence := s.sequence + 1
	payload, raw, err := s.payloadForPendingLocked(candidate, sequence)
	if err != nil {
		return nil, err
	}
	if len(raw) > trafficRuntimeJournalMaxBytes {
		return nil, errTrafficRuntimeJournalTooLarge
	}
	if err := writeTrafficRuntimeJournalFile(raw); err != nil {
		return nil, err
	}
	return &trafficRuntimeJournalStage{
		payload:  payload,
		raw:      raw,
		pending:  candidate,
		flushNow: len(raw) >= trafficRuntimeJournalFlushThreshold,
	}, nil
}

func (s *trafficRuntimeStatsJournal) commitStage(stage *trafficRuntimeJournalStage) {
	if stage == nil {
		return
	}
	s.mu.Lock()
	s.pending = stage.pending
	s.sequence = stage.payload.Sequence
	s.version++
	s.mu.Unlock()
}

func (s *trafficRuntimeStatsJournal) abortStage() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.pending) == 0 {
		return removeTrafficRuntimeJournalFiles()
	}
	_, raw, err := s.payloadLocked()
	if err != nil {
		return err
	}
	if err := writeTrafficRuntimeJournalFile(raw); err != nil {
		return err
	}
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
	defer s.mu.Unlock()
	if len(s.pending) == 0 {
		return nil
	}

	sequence := s.sequence
	samples := s.sortedPendingLocked()
	s.initMu.Lock()
	generation := s.generation
	s.initMu.Unlock()
	db := database.GetDB()
	if db == nil {
		return fmt.Errorf("traffic runtime journal database is not initialized")
	}
	if err := db.Transaction(func(tx *gorm.DB) error {
		var applied uint64
		if err := tx.Raw(fmt.Sprintf("SELECT sequence FROM %s WHERE id = 1", trafficRuntimeJournalCheckpointTable)).Scan(&applied).Error; err != nil {
			return err
		}
		if applied >= sequence {
			return deleteTrafficRuntimeJournalStage(tx, generation, sequence)
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
		return deleteTrafficRuntimeJournalStage(tx, generation, sequence)
	}); err != nil {
		return err
	}

	s.pending = make(map[string]model.Stats)
	s.version++
	s.initMu.Lock()
	if sequence > s.checkpoint {
		s.checkpoint = sequence
	}
	s.initMu.Unlock()
	return removeTrafficRuntimeJournalFiles()
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
	_, samples := runtimeTrafficStats.snapshotPending()
	return pendingTrafficRuntimeStatsFromSamples(samples, resource, tag, startUnix, bucketSeconds)
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
	result := make([]model.Stats, 0, len(combined))
	for _, sample := range combined {
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
