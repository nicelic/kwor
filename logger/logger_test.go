package logger

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"
)

func resetLogBufferForTest() {
	globalRingBuffer.mu.Lock()
	globalRingBuffer.entries = [logBufferMaxEntries]logBufferEntry{}
	globalRingBuffer.head = 0
	globalRingBuffer.tail = 0
	globalRingBuffer.count = 0
	globalRingBuffer.curBytes = 0
	logBufferBytes = 0
	globalRingBuffer.mu.Unlock()
}

func TestLogBufferIsBoundedAndTruncatesEntries(t *testing.T) {
	resetLogBufferForTest()
	t.Cleanup(resetLogBufferForTest)

	addToBuffer("WARNING", strings.Repeat("界", logEntryMaxBytes))
	globalRingBuffer.mu.RLock()
	entryCount := globalRingBuffer.count
	bufferBytes := globalRingBuffer.curBytes
	entryBytes := 0
	valid := false
	if entryCount > 0 {
		entryBytes = len(globalRingBuffer.entries[globalRingBuffer.head].log)
		valid = utf8.ValidString(globalRingBuffer.entries[globalRingBuffer.head].log)
	}
	globalRingBuffer.mu.RUnlock()
	if entryCount != 1 || bufferBytes > logBufferMaxBytes || entryBytes > logEntryMaxBytes {
		t.Fatalf("log buffer bounds violated: entries=%d bytes=%d entryBytes=%d", entryCount, bufferBytes, entryBytes)
	}
	if !valid {
		t.Fatal("truncated log entry is not valid UTF-8")
	}
}

func TestLogBufferConcurrentReadWrite(t *testing.T) {
	resetLogBufferForTest()
	t.Cleanup(resetLogBufferForTest)

	var group sync.WaitGroup
	for i := 0; i < 8; i++ {
		group.Add(1)
		go func(index int) {
			defer group.Done()
			for count := 0; count < 500; count++ {
				addToBuffer("INFO", "worker="+string(rune('a'+index)))
				_ = GetLogs(10, "DEBUG")
			}
		}(i)
	}
	group.Wait()

	logs := GetLogs(10, "DEBUG")
	if len(logs) != 10 {
		t.Fatalf("GetLogs returned %d entries, want 10", len(logs))
	}
}

func TestGetLogsClampsCallerRequestedCapacity(t *testing.T) {
	resetLogBufferForTest()
	t.Cleanup(resetLogBufferForTest)
	addToBuffer("INFO", "bounded result")

	logs := GetLogs(logBufferMaxEntries*1024, "DEBUG")
	if len(logs) != 1 {
		t.Fatalf("GetLogs returned %d entries for a one-entry buffer, want 1", len(logs))
	}
}

func TestDiskLogManagerPruneOverLimit(t *testing.T) {
	tempDir := t.TempDir()
	mgr := &diskLogManager{
		dir: tempDir,
	}

	// 模拟创建若干个历史日志文件
	for i := 0; i < 6; i++ {
		p := filepath.Join(tempDir, fmt.Sprintf("kwor-20260901-%02d0000.log", i))
		_ = os.WriteFile(p, make([]byte, 10*1024*1024), 0640) // 10MB each, total 60MB > 50MB
	}

	mgr.mu.Lock()
	mgr.recalculateAndPruneLocked()
	mgr.mu.Unlock()

	if mgr.totalBytes > diskLogMaxTotalBytes {
		t.Fatalf("totalBytes %d exceeds limit %d", mgr.totalBytes, diskLogMaxTotalBytes)
	}
}
