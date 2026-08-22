package service

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
)

func setupTrafficRuntimeJournalTest(t *testing.T) string {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "traffic-runtime.db")
	if err := database.InitDB(dbPath); err != nil {
		t.Fatalf("init journal database failed: %v", err)
	}
	if sqlDB, err := database.GetDB().DB(); err == nil && sqlDB != nil {
		t.Cleanup(func() { _ = sqlDB.Close() })
	}

	previousPathFn := trafficRuntimeJournalPathFn
	journalPath := filepath.Join(filepath.Dir(dbPath), trafficRuntimeJournalFilename)
	trafficRuntimeJournalPathFn = func() string { return journalPath }
	resetHistoryStorageState()
	runtimeTrafficStats.resetForDatabaseReload()
	t.Cleanup(func() {
		trafficRuntimeJournalPathFn = previousPathFn
		runtimeTrafficStats.resetForDatabaseReload()
		resetHistoryStorageState()
	})
	return journalPath
}

func TestTrafficRuntimeJournalMergesPendingAndFlushes(t *testing.T) {
	journalPath := setupTrafficRuntimeJournalTest(t)
	now := time.Now().Unix()
	samples := []model.Stats{
		{DateTime: now, Resource: "client", Tag: "alice", Direction: true, Traffic: 40},
		{DateTime: now, Resource: "client", Tag: "alice", Direction: true, Traffic: 2},
	}
	if err := StageTrafficRuntimeStats(samples); err != nil {
		t.Fatalf("stage samples failed: %v", err)
	}
	if _, err := os.Stat(journalPath); err != nil {
		t.Fatalf("journal sidecar was not written: %v", err)
	}

	rows, err := queryStatsHistory("client", "alice", 1)
	if err != nil {
		t.Fatalf("query pending stats failed: %v", err)
	}
	if len(rows) != 1 || rows[0].Traffic != 42 {
		t.Fatalf("pending stats = %#v, want one 42-byte row", rows)
	}

	if err := FlushTrafficRuntimeJournal(); err != nil {
		t.Fatalf("flush journal failed: %v", err)
	}
	if _, err := os.Stat(journalPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("flushed journal sidecar still exists: %v", err)
	}
	rows, err = queryStatsHistory("client", "alice", 1)
	if err != nil {
		t.Fatalf("query flushed stats failed: %v", err)
	}
	if len(rows) != 1 || rows[0].Traffic != 42 {
		t.Fatalf("flushed stats = %#v, want one 42-byte row", rows)
	}
}

func TestTrafficRuntimeJournalCheckpointPreventsReplay(t *testing.T) {
	journalPath := setupTrafficRuntimeJournalTest(t)
	sample := model.Stats{DateTime: time.Now().Unix(), Resource: "client", Tag: "checkpoint", Direction: false, Traffic: 17}
	if err := StageTrafficRuntimeStats([]model.Stats{sample}); err != nil {
		t.Fatalf("stage sample failed: %v", err)
	}
	raw, err := os.ReadFile(journalPath)
	if err != nil {
		t.Fatalf("read staged journal failed: %v", err)
	}
	if err := FlushTrafficRuntimeJournal(); err != nil {
		t.Fatalf("flush journal failed: %v", err)
	}
	if err := os.WriteFile(journalPath, raw, 0o600); err != nil {
		t.Fatalf("restore committed sidecar fixture failed: %v", err)
	}

	runtimeTrafficStats.resetForDatabaseReload()
	if err := PrepareTrafficRuntimeJournalOnStartup(); err != nil {
		t.Fatalf("restore journal failed: %v", err)
	}
	rows, err := queryStatsHistory("client", "checkpoint", 1)
	if err != nil {
		t.Fatalf("query restored stats failed: %v", err)
	}
	if len(rows) != 1 || rows[0].Traffic != 17 {
		t.Fatalf("checkpoint replay changed stats: %#v", rows)
	}
	if _, err := os.Stat(journalPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("already-applied sidecar was not removed: %v", err)
	}
}

func TestTrafficRuntimeJournalRestoresCommittedSQLiteStageWhenMirrorIsMissing(t *testing.T) {
	journalPath := setupTrafficRuntimeJournalTest(t)
	sample := model.Stats{DateTime: time.Now().Unix(), Resource: "client", Tag: "committed-stage", Direction: true, Traffic: 23}
	if err := StageTrafficRuntimeStats([]model.Stats{sample}); err != nil {
		t.Fatalf("stage committed sample failed: %v", err)
	}
	if err := os.Remove(journalPath); err != nil {
		t.Fatalf("remove primary mirror fixture failed: %v", err)
	}
	_ = os.Remove(journalPath + trafficRuntimeJournalBackupSuffix)

	runtimeTrafficStats.resetForDatabaseReload()
	if err := PrepareTrafficRuntimeJournalOnStartup(); err != nil {
		t.Fatalf("restore committed sqlite stage failed: %v", err)
	}
	rows, err := queryStatsHistory("client", "committed-stage", 1)
	if err != nil {
		t.Fatalf("query restored committed stage failed: %v", err)
	}
	if len(rows) != 1 || rows[0].Traffic != 23 {
		t.Fatalf("committed sqlite stage stats = %#v, want one 23-byte row", rows)
	}
}

func TestTrafficRuntimeJournalDoesNotReplayPreCommitMirror(t *testing.T) {
	journalPath := setupTrafficRuntimeJournalTest(t)
	if err := runtimeTrafficStats.ensureReady(); err != nil {
		t.Fatalf("prepare journal metadata failed: %v", err)
	}

	unlock := lockTrafficRuntimeJournalTransaction()
	tx := database.GetDB().Begin()
	if tx.Error != nil {
		unlock()
		t.Fatalf("begin pre-commit fixture transaction failed: %v", tx.Error)
	}
	stage, err := stageTrafficRuntimeStatsForTransaction(tx, []model.Stats{{
		DateTime: time.Now().Unix(), Resource: "client", Tag: "pre-commit", Direction: false, Traffic: 31,
	}})
	if err != nil {
		tx.Rollback()
		unlock()
		t.Fatalf("stage pre-commit fixture failed: %v", err)
	}
	if stage == nil {
		tx.Rollback()
		unlock()
		t.Fatal("pre-commit fixture unexpectedly bypassed the journal")
	}
	if err := tx.Rollback().Error; err != nil {
		unlock()
		t.Fatalf("rollback pre-commit fixture failed: %v", err)
	}
	unlock()

	if _, err := os.Stat(journalPath); err != nil {
		t.Fatalf("pre-commit mirror was not written: %v", err)
	}
	runtimeTrafficStats.resetForDatabaseReload()
	if err := PrepareTrafficRuntimeJournalOnStartup(); err != nil {
		t.Fatalf("recover pre-commit mirror failed: %v", err)
	}
	rows, err := queryStatsHistory("client", "pre-commit", 1)
	if err != nil {
		t.Fatalf("query pre-commit mirror failed: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("pre-commit mirror was replayed: %#v", rows)
	}
	invalidFiles, err := filepath.Glob(journalPath + ".invalid-*")
	if err != nil || len(invalidFiles) == 0 {
		t.Fatalf("pre-commit mirror was not quarantined: files=%v err=%v", invalidFiles, err)
	}
}

func TestTrafficRuntimeJournalRestoresLastValidBackupAfterPrimaryDamage(t *testing.T) {
	journalPath := setupTrafficRuntimeJournalTest(t)
	first := model.Stats{DateTime: time.Now().Unix(), Resource: "client", Tag: "backup", Direction: true, Traffic: 11}
	second := model.Stats{DateTime: time.Now().Unix(), Resource: "client", Tag: "backup", Direction: true, Traffic: 5}
	if err := StageTrafficRuntimeStats([]model.Stats{first}); err != nil {
		t.Fatalf("stage first backup sample failed: %v", err)
	}
	if err := StageTrafficRuntimeStats([]model.Stats{second}); err != nil {
		t.Fatalf("stage second backup sample failed: %v", err)
	}
	if _, err := os.Stat(journalPath + trafficRuntimeJournalBackupSuffix); err != nil {
		t.Fatalf("journal backup was not retained: %v", err)
	}
	if err := os.WriteFile(journalPath, []byte("damaged-primary"), 0o600); err != nil {
		t.Fatalf("damage primary fixture failed: %v", err)
	}

	runtimeTrafficStats.resetForDatabaseReload()
	if err := PrepareTrafficRuntimeJournalOnStartup(); err != nil {
		t.Fatalf("recover from backup failed: %v", err)
	}
	rows, err := queryStatsHistory("client", "backup", 1)
	if err != nil {
		t.Fatalf("query backup-restored stats failed: %v", err)
	}
	if len(rows) != 1 || rows[0].Traffic != 16 {
		t.Fatalf("staging restore stats = %#v, want one 16-byte row", rows)
	}
	invalidFiles, err := filepath.Glob(journalPath + ".invalid-*")
	if err != nil || len(invalidFiles) == 0 {
		t.Fatalf("damaged primary was not quarantined: files=%v err=%v", invalidFiles, err)
	}
}

func TestTrafficRuntimeJournalDoesNotReplaceValidBackupWithCorruptPrimary(t *testing.T) {
	journalPath := setupTrafficRuntimeJournalTest(t)
	if err := StageTrafficRuntimeStats([]model.Stats{{
		DateTime: time.Now().Unix(), Resource: "client", Tag: "backup-preserve", Direction: true, Traffic: 11,
	}}); err != nil {
		t.Fatalf("stage initial backup sample failed: %v", err)
	}
	if err := StageTrafficRuntimeStats([]model.Stats{{
		DateTime: time.Now().Unix(), Resource: "client", Tag: "backup-preserve", Direction: true, Traffic: 5,
	}}); err != nil {
		t.Fatalf("stage backup sample failed: %v", err)
	}
	validBackup, err := os.ReadFile(journalPath + trafficRuntimeJournalBackupSuffix)
	if err != nil {
		t.Fatalf("read valid backup fixture failed: %v", err)
	}
	if err := os.WriteFile(journalPath, []byte("corrupt-primary"), 0o600); err != nil {
		t.Fatalf("damage primary fixture failed: %v", err)
	}

	if err := StageTrafficRuntimeStats([]model.Stats{{
		DateTime: time.Now().Unix(), Resource: "client", Tag: "backup-preserve", Direction: true, Traffic: 3,
	}}); err != nil {
		t.Fatalf("stage after primary damage failed: %v", err)
	}
	gotBackup, err := os.ReadFile(journalPath + trafficRuntimeJournalBackupSuffix)
	if err != nil {
		t.Fatalf("read preserved backup failed: %v", err)
	}
	if !bytes.Equal(gotBackup, validBackup) {
		t.Fatalf("valid backup was replaced by damaged primary")
	}
}

func TestTrafficRuntimeJournalIgnoresInvalidSamplesWithoutAdvancingSequence(t *testing.T) {
	journalPath := setupTrafficRuntimeJournalTest(t)
	if err := StageTrafficRuntimeStats([]model.Stats{
		{DateTime: time.Now().Unix(), Resource: "", Tag: "invalid-resource", Direction: true, Traffic: 1},
		{DateTime: time.Now().Unix(), Resource: "client", Tag: "zero", Direction: true, Traffic: 0},
	}); err != nil {
		t.Fatalf("invalid samples should be ignored: %v", err)
	}
	runtimeTrafficStats.mu.Lock()
	pending := len(runtimeTrafficStats.pending)
	sequence := runtimeTrafficStats.sequence
	runtimeTrafficStats.mu.Unlock()
	if pending != 0 || sequence != 0 {
		t.Fatalf("invalid samples changed journal state: pending=%d sequence=%d", pending, sequence)
	}
	if _, err := os.Stat(journalPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("invalid samples created a mirror: %v", err)
	}
}

func TestTrafficRuntimeJournalDiscardsCorruptSQLiteStage(t *testing.T) {
	setupTrafficRuntimeJournalTest(t)
	if err := runtimeTrafficStats.ensureReady(); err != nil {
		t.Fatalf("prepare journal metadata failed: %v", err)
	}
	runtimeTrafficStats.initMu.Lock()
	generation := runtimeTrafficStats.generation
	runtimeTrafficStats.initMu.Unlock()
	if err := database.GetDB().Exec(
		fmt.Sprintf("INSERT INTO %s (id, generation, sequence, payload) VALUES (1, ?, 1, ?)", trafficRuntimeJournalStagingTable),
		generation, []byte("{corrupt-stage"),
	).Error; err != nil {
		t.Fatalf("insert corrupt staging fixture failed: %v", err)
	}

	if err := PrepareTrafficRuntimeJournalOnStartup(); err != nil {
		t.Fatalf("corrupt staging should be discarded during startup: %v", err)
	}
	var count int64
	if err := database.GetDB().Table(trafficRuntimeJournalStagingTable).Count(&count).Error; err != nil {
		t.Fatalf("count staging rows after corruption cleanup failed: %v", err)
	}
	if count != 0 {
		t.Fatalf("corrupt staging row was not discarded: %d rows remain", count)
	}
}

func TestTrafficRuntimeJournalQuarantinesOldMirrorBeforeDatabaseRestore(t *testing.T) {
	journalPath := setupTrafficRuntimeJournalTest(t)
	if err := StageTrafficRuntimeStats([]model.Stats{{
		DateTime: time.Now().Unix(), Resource: "client", Tag: "before-restore", Direction: true, Traffic: 7,
	}}); err != nil {
		t.Fatalf("stage restore mirror failed: %v", err)
	}
	if err := QuarantineTrafficRuntimeJournalBeforeDatabaseRestore(); err != nil {
		t.Fatalf("quarantine before restore failed: %v", err)
	}
	if _, err := os.Stat(journalPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("old primary mirror still exists after restore quarantine: %v", err)
	}
	invalidFiles, err := filepath.Glob(journalPath + ".invalid-*")
	if err != nil || len(invalidFiles) == 0 {
		t.Fatalf("old mirror was not retained as a quarantine file: files=%v err=%v", invalidFiles, err)
	}
}

func TestTrafficRuntimeJournalQuarantinesCorruptAndWrongGenerationFiles(t *testing.T) {
	journalPath := setupTrafficRuntimeJournalTest(t)
	if err := runtimeTrafficStats.ensureReady(); err != nil {
		t.Fatalf("prepare journal metadata failed: %v", err)
	}
	if err := os.WriteFile(journalPath, []byte("{not-json"), 0o600); err != nil {
		t.Fatalf("write corrupt fixture failed: %v", err)
	}
	if err := runtimeTrafficStats.restore(); err != nil {
		t.Fatalf("corrupt journal must be isolated, got: %v", err)
	}
	invalidFiles, err := filepath.Glob(journalPath + ".invalid-*")
	if err != nil || len(invalidFiles) != 1 {
		t.Fatalf("corrupt journal was not quarantined: files=%v err=%v", invalidFiles, err)
	}

	payload := trafficRuntimeJournalPayload{
		Schema:     trafficRuntimeJournalSchema,
		Generation: "old-database-generation",
		Sequence:   1,
		Samples: []model.Stats{{
			DateTime: time.Now().Unix(), Resource: "client", Tag: "old", Direction: true, Traffic: 1,
		}},
	}
	raw, err := marshalTrafficRuntimeJournalPayload(payload)
	if err != nil {
		t.Fatalf("marshal generation fixture failed: %v", err)
	}
	if err := os.WriteFile(journalPath, raw, 0o600); err != nil {
		t.Fatalf("write generation fixture failed: %v", err)
	}
	if err := runtimeTrafficStats.restore(); err != nil {
		t.Fatalf("wrong generation journal must be isolated, got: %v", err)
	}
	invalidFiles, err = filepath.Glob(journalPath + ".invalid-*")
	if err != nil || len(invalidFiles) < 2 {
		t.Fatalf("wrong generation journal was not quarantined: files=%v err=%v", invalidFiles, err)
	}
}

func TestTrafficRuntimeJournalRestoreRotatesGenerationBeforeFileQuarantine(t *testing.T) {
	journalPath := setupTrafficRuntimeJournalTest(t)
	sample := model.Stats{DateTime: time.Now().Unix(), Resource: "client", Tag: "restored-generation", Direction: true, Traffic: 29}
	if err := StageTrafficRuntimeStats([]model.Stats{sample}); err != nil {
		t.Fatalf("stage restore-generation sample failed: %v", err)
	}
	if _, err := os.Stat(journalPath); err != nil {
		t.Fatalf("restore-generation sidecar was not written: %v", err)
	}

	runtimeTrafficStats.initMu.Lock()
	oldGeneration := runtimeTrafficStats.generation
	runtimeTrafficStats.initMu.Unlock()
	previousQuarantine := quarantineTrafficRuntimeJournalFilesFn
	quarantineTrafficRuntimeJournalFilesFn = func() error {
		return errors.New("simulated quarantine failure")
	}
	t.Cleanup(func() { quarantineTrafficRuntimeJournalFilesFn = previousQuarantine })
	if err := InvalidateTrafficRuntimeJournalForDatabaseRestore(); err == nil {
		t.Fatal("restore invalidation unexpectedly succeeded despite quarantine failure")
	}

	runtimeTrafficStats.initMu.Lock()
	newGeneration := runtimeTrafficStats.generation
	runtimeTrafficStats.initMu.Unlock()
	if newGeneration == "" || newGeneration == oldGeneration {
		t.Fatalf("database restore did not rotate generation: old=%q new=%q", oldGeneration, newGeneration)
	}

	// The mirror is deliberately still present. Startup must reject it by the
	// new generation, then quarantine it, rather than replaying the old tail.
	quarantineTrafficRuntimeJournalFilesFn = quarantineTrafficRuntimeJournalFiles
	if err := PrepareTrafficRuntimeJournalOnStartup(); err != nil {
		t.Fatalf("prepare after restore-generation rotation failed: %v", err)
	}
	rows, err := queryStatsHistory("client", "restored-generation", 1)
	if err != nil {
		t.Fatalf("query after restore-generation rotation failed: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("stale restored tail was replayed: %#v", rows)
	}
}

func TestTrafficRuntimeJournalCapacityFallsBackWithoutGrowingMemory(t *testing.T) {
	setupTrafficRuntimeJournalTest(t)
	tooLargeTag := strings.Repeat("x", trafficRuntimeJournalMaxBytes)
	err := StageTrafficRuntimeStats([]model.Stats{{
		DateTime: time.Now().Unix(), Resource: "client", Tag: tooLargeTag, Direction: true, Traffic: 1,
	}})
	if err != nil {
		t.Fatalf("oversized journal fallback failed: %v", err)
	}
	runtimeTrafficStats.mu.Lock()
	pending := len(runtimeTrafficStats.pending)
	runtimeTrafficStats.mu.Unlock()
	if pending != 0 {
		t.Fatalf("oversized sample remained in memory: %d pending rows", pending)
	}
	var stored int64
	if err := database.GetDB().Model(&model.Stats{}).Where("tag = ?", tooLargeTag).Count(&stored).Error; err != nil {
		t.Fatalf("count oversized fallback stats failed: %v", err)
	}
	if stored != 1 {
		t.Fatalf("oversized fallback stats rows = %d, want 1", stored)
	}
}

func TestTrafficRuntimeJournalUsesCurrentTransactionFallback(t *testing.T) {
	journalPath := setupTrafficRuntimeJournalTest(t)
	blockedParent := filepath.Join(filepath.Dir(journalPath), "not-a-directory")
	if err := os.WriteFile(blockedParent, []byte("blocked"), 0o600); err != nil {
		t.Fatalf("create blocked journal parent failed: %v", err)
	}
	trafficRuntimeJournalPathFn = func() string { return filepath.Join(blockedParent, trafficRuntimeJournalFilename) }
	if err := runtimeTrafficStats.ensureReady(); err != nil {
		t.Fatalf("prepare journal metadata failed: %v", err)
	}
	if err := EnsureHistoryStorageReady(); err != nil {
		t.Fatalf("prepare history storage failed: %v", err)
	}

	unlock := lockTrafficRuntimeJournalTransaction()
	tx := database.GetDB().Begin()
	if tx.Error != nil {
		t.Fatalf("begin fallback transaction failed: %v", tx.Error)
	}
	sample := model.Stats{DateTime: time.Now().Unix(), Resource: "client", Tag: "fallback", Direction: true, Traffic: 9}
	staged, err := stageTrafficRuntimeStatsForTransaction(tx, []model.Stats{sample})
	if err != nil {
		tx.Rollback()
		t.Fatalf("transaction fallback failed: %v", err)
	}
	if staged != nil {
		tx.Rollback()
		t.Fatal("sidecar unexpectedly staged through a blocked directory")
	}
	if err := tx.Commit().Error; err != nil {
		unlock()
		t.Fatalf("commit fallback transaction failed: %v", err)
	}
	unlock()
	rows, err := queryStatsHistory("client", "fallback", 1)
	if err != nil {
		t.Fatalf("query fallback stats failed: %v", err)
	}
	if len(rows) != 1 || rows[0].Traffic != 9 {
		t.Fatalf("fallback stats = %#v, want one 9-byte row", rows)
	}
}

func TestQueryStatsHistoryWaitsForJournalTransactionBoundary(t *testing.T) {
	setupTrafficRuntimeJournalTest(t)
	if err := StageTrafficRuntimeStats([]model.Stats{{
		DateTime: time.Now().Unix(), Resource: "client", Tag: "query-boundary", Direction: true, Traffic: 7,
	}}); err != nil {
		t.Fatalf("stage query boundary sample: %v", err)
	}

	unlock := lockTrafficRuntimeJournalTransaction()
	result := make(chan []model.Stats, 1)
	go func() {
		rows, err := queryStatsHistory("client", "query-boundary", 1)
		if err != nil {
			result <- nil
			return
		}
		result <- rows
	}()
	select {
	case <-result:
		unlock()
		t.Fatal("stats query crossed the journal transaction boundary")
	case <-time.After(40 * time.Millisecond):
	}
	unlock()
	select {
	case rows := <-result:
		if len(rows) != 1 || rows[0].Traffic != 7 {
			t.Fatalf("query boundary rows = %#v, want one 7-byte row", rows)
		}
	case <-time.After(time.Second):
		t.Fatal("stats query did not resume after journal transaction boundary")
	}
}
