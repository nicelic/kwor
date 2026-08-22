package cronjob

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/robfig/cron/v3"
)

func TestRuntimeSamplerSerializesWakePassesAndRestartsCleanly(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	var active atomic.Int32
	var maxActive atomic.Int32

	sampler := &RuntimeSampler{
		taskOverrides: &runtimeSamplerTaskOverrides{
			integrity: func(bool) {
				current := active.Add(1)
				for {
					previous := maxActive.Load()
					if current <= previous || maxActive.CompareAndSwap(previous, current) {
						break
					}
				}
				if calls.Add(1) == 1 {
					close(started)
					<-release
				}
				active.Add(-1)
			},
			traffic:     func() {},
			portForward: func() {},
			reverse:     func() {},
			deplete:     func() {},
			flush:       func() error { return nil },
		},
	}

	sampler.Start()
	sampler.Wake()
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("runtime sampler did not start its wake pass")
	}
	sampler.Wake()
	sampler.Wake()
	close(release)

	deadline := time.After(time.Second)
	for calls.Load() < 2 {
		select {
		case <-deadline:
			t.Fatalf("coalesced wake did not run after first pass: calls=%d", calls.Load())
		case <-time.After(time.Millisecond):
		}
	}
	if got := maxActive.Load(); got != 1 {
		t.Fatalf("runtime sampler overlapped passes: max active=%d", got)
	}

	sampler.StopAndFlush()
	sampler.Start()
	sampler.Wake()
	deadline = time.After(time.Second)
	for calls.Load() < 3 {
		select {
		case <-deadline:
			t.Fatalf("runtime sampler did not restart: calls=%d", calls.Load())
		case <-time.After(time.Millisecond):
		}
	}
	sampler.StopAndFlush()
}

func TestRuntimeSamplerContinuesAfterTaskPanic(t *testing.T) {
	var calls atomic.Int32
	sampler := &RuntimeSampler{
		taskOverrides: &runtimeSamplerTaskOverrides{
			integrity: func(bool) {
				if calls.Add(1) == 1 {
					panic("fixture panic")
				}
			},
			traffic:     func() {},
			portForward: func() {},
			reverse:     func() {},
			deplete:     func() {},
			flush:       func() error { return nil },
		},
	}

	sampler.Start()
	sampler.Wake()
	deadline := time.After(time.Second)
	for calls.Load() < 1 {
		select {
		case <-deadline:
			t.Fatal("runtime sampler did not execute the initial pass")
		case <-time.After(time.Millisecond):
		}
	}
	sampler.Wake()
	deadline = time.After(time.Second)
	for calls.Load() < 2 {
		select {
		case <-deadline:
			t.Fatal("runtime sampler stopped after a task panic")
		case <-time.After(time.Millisecond):
		}
	}
	if err := sampler.StopAndFlush(); err != nil {
		t.Fatalf("stop sampler after panic: %v", err)
	}
}

func TestRuntimeSamplerStaggersCleanStart(t *testing.T) {
	trafficStarted := make(chan struct{}, 1)
	sampler := &RuntimeSampler{
		taskOverrides: &runtimeSamplerTaskOverrides{
			traffic: func() {
				select {
				case trafficStarted <- struct{}{}:
				default:
				}
			},
			flush: func() error { return nil },
		},
	}

	sampler.Start()
	select {
	case <-trafficStarted:
		t.Fatal("clean sampler start burst into the traffic task")
	case <-time.After(100 * time.Millisecond):
	}

	sampler.Wake()
	select {
	case <-trafficStarted:
	case <-time.After(time.Second):
		t.Fatal("explicit sampler wake did not run the traffic task")
	}
	if err := sampler.StopAndFlush(); err != nil {
		t.Fatalf("stop staggered sampler: %v", err)
	}
}

func TestRuntimeSamplerStartWaitsForFinalFlush(t *testing.T) {
	flushStarted := make(chan struct{})
	releaseFlush := make(chan struct{})
	var flushCalls atomic.Int32
	var integrityCalls atomic.Int32
	sampler := &RuntimeSampler{
		taskOverrides: &runtimeSamplerTaskOverrides{
			integrity: func(bool) { integrityCalls.Add(1) },
			flush: func() error {
				// The explicit wake below performs the first pass, including a
				// flush. Block the second call, which is StopAndFlush's final barrier.
				if flushCalls.Add(1) == 2 {
					close(flushStarted)
					<-releaseFlush
				}
				return nil
			},
		},
	}
	sampler.Start()
	sampler.Wake()
	waitForRuntimeSamplerCalls(t, &integrityCalls, 1)

	stopDone := make(chan error, 1)
	go func() { stopDone <- sampler.StopAndFlush() }()
	select {
	case <-flushStarted:
	case <-time.After(time.Second):
		t.Fatal("sampler did not enter its final flush")
	}

	restartDone := make(chan struct{})
	go func() {
		sampler.Start()
		sampler.Wake()
		close(restartDone)
	}()
	select {
	case <-restartDone:
		t.Fatal("sampler restarted before its final flush completed")
	case <-time.After(40 * time.Millisecond):
	}

	close(releaseFlush)
	if err := <-stopDone; err != nil {
		t.Fatalf("final sampler flush failed: %v", err)
	}
	select {
	case <-restartDone:
	case <-time.After(time.Second):
		t.Fatal("sampler did not restart after final flush completed")
	}
	waitForRuntimeSamplerCalls(t, &integrityCalls, 2)
	if err := sampler.StopAndFlush(); err != nil {
		t.Fatalf("stop restarted sampler: %v", err)
	}
}

func TestDatabaseRestoreAbortRestartsSamplerAfterFinalFlushFailure(t *testing.T) {
	var integrityCalls atomic.Int32
	flushErr := errors.New("fixture flush failure")
	sampler := &RuntimeSampler{
		taskOverrides: &runtimeSamplerTaskOverrides{
			integrity: func(bool) { integrityCalls.Add(1) },
			flush:     func() error { return flushErr },
		},
	}
	sampler.Start()
	sampler.Wake()
	waitForRuntimeSamplerCalls(t, &integrityCalls, 1)

	job := &CronJob{
		cron:           cron.New(),
		runtimeSampler: sampler,
	}
	if err := job.PauseRuntimeSamplerForDatabaseRestore(); !errors.Is(err, flushErr) {
		t.Fatalf("pause database restore error = %v, want flush failure", err)
	}
	job.mu.Lock()
	paused := job.runtimeSamplerPaused
	job.mu.Unlock()
	if !paused {
		t.Fatal("sampler must remain marked paused until the restore-abort hook resumes it")
	}

	job.ResumeRuntimeSamplerAfterDatabaseRestoreFailure()
	waitForRuntimeSamplerCalls(t, &integrityCalls, 2)

	if err := sampler.StopAndFlush(); !errors.Is(err, flushErr) {
		t.Fatalf("final sampler stop error = %v, want flush failure", err)
	}
}

func waitForRuntimeSamplerCalls(t *testing.T, calls *atomic.Int32, want int32) {
	t.Helper()
	deadline := time.After(time.Second)
	for calls.Load() < want {
		select {
		case <-deadline:
			t.Fatalf("runtime sampler calls=%d, want at least %d", calls.Load(), want)
		case <-time.After(time.Millisecond):
		}
	}
}
