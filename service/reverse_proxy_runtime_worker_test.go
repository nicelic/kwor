package service

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestReverseProxyRuntimeWorkerPausesAndResumesSafely(t *testing.T) {
	worker := &reverseProxyRuntimeWorker{}
	worker.Start(&ReverseProxyService{})
	waitForReverseProxyRuntimeWorkerState(t, worker, true)

	if err := worker.PauseForDatabaseRestore(); err != nil {
		t.Fatalf("pause worker for database restore: %v", err)
	}
	worker.mu.Lock()
	paused := worker.databasePaused
	running := worker.running || worker.stopping
	worker.mu.Unlock()
	if !paused {
		t.Fatal("worker must remember the database restore pause")
	}
	if running {
		t.Fatal("worker must stop before database replacement")
	}

	worker.Start(&ReverseProxyService{})
	worker.mu.Lock()
	running = worker.running || worker.stopping
	worker.mu.Unlock()
	if running {
		t.Fatal("starting while restore is paused must not revive the worker")
	}

	worker.ResumeAfterDatabaseRestoreFailure()
	waitForReverseProxyRuntimeWorkerState(t, worker, true)
	worker.StopAndWait()
	worker.StopAndWait()
}

func TestReverseProxyRuntimeWorkerContinuesAfterSyncPanic(t *testing.T) {
	var calls atomic.Int32
	worker := &reverseProxyRuntimeWorker{
		syncFunc: func(*ReverseProxyService) error {
			if calls.Add(1) == 1 {
				panic("fixture panic")
			}
			return errors.New("fixture error")
		},
	}
	worker.Start(&ReverseProxyService{})
	worker.Wake()
	deadline := time.Now().Add(time.Second)
	for calls.Load() < 1 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if calls.Load() < 1 {
		t.Fatal("worker did not execute the first sync")
	}
	worker.Wake()
	deadline = time.Now().Add(time.Second)
	for calls.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if calls.Load() < 2 {
		t.Fatal("worker stopped after a sync panic")
	}
	worker.StopAndWait()
}

func waitForReverseProxyRuntimeWorkerState(t *testing.T, worker *reverseProxyRuntimeWorker, wantRunning bool) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		worker.mu.Lock()
		running := worker.running
		worker.mu.Unlock()
		if running == wantRunning {
			return
		}
		time.Sleep(time.Millisecond)
	}
	worker.mu.Lock()
	running := worker.running
	worker.mu.Unlock()
	t.Fatalf("worker running=%v, want %v", running, wantRunning)
}
