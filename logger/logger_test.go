package logger

import (
	"strings"
	"sync"
	"testing"
	"unicode/utf8"
)

func resetLogBufferForTest() {
	logBufferMu.Lock()
	logBuffer = nil
	logBufferBytes = 0
	logBufferMu.Unlock()
}

func TestLogBufferIsBoundedAndTruncatesEntries(t *testing.T) {
	resetLogBufferForTest()
	t.Cleanup(resetLogBufferForTest)

	addToBuffer("WARNING", strings.Repeat("界", logEntryMaxBytes))
	logBufferMu.RLock()
	entryCount := len(logBuffer)
	bufferBytes := logBufferBytes
	entryBytes := 0
	valid := false
	if entryCount > 0 {
		entryBytes = len(logBuffer[0].log)
		valid = utf8.ValidString(logBuffer[0].log)
	}
	logBufferMu.RUnlock()
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
