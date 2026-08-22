package service

import (
	"sync"
	"sync/atomic"
)

var runtimeSamplerSignal = struct {
	sync.RWMutex
	wake   func()
	pause  func() error
	resume func()
}{}

var runtimeTrafficMutationState = struct {
	active atomic.Int32
	epoch  atomic.Uint64
}{}

// BeginRuntimeTrafficMutation marks a panel-side change that can replace nft
// handles or traffic bindings. Samplers use the epoch as a seqlock-like
// boundary: a pass that overlaps the mutation rolls back its baseline update
// and retries from nft's cumulative counters on the next pass.
func BeginRuntimeTrafficMutation() func() {
	runtimeTrafficMutationState.active.Add(1)
	runtimeTrafficMutationState.epoch.Add(1)
	var once sync.Once
	return func() {
		once.Do(func() {
			runtimeTrafficMutationState.epoch.Add(1)
			remaining := runtimeTrafficMutationState.active.Add(-1)
			if remaining == 0 {
				// A lifecycle/config path may have queued its wake while the
				// mutation was still active. Send one more coalesced wake after
				// the gate closes so that pass is not lost to the seqlock check.
				WakeRuntimeSampler()
			}
		})
	}
}

func captureRuntimeTrafficSamplingEpoch() (uint64, bool) {
	if runtimeTrafficMutationState.active.Load() != 0 {
		return 0, false
	}
	epoch := runtimeTrafficMutationState.epoch.Load()
	if runtimeTrafficMutationState.active.Load() != 0 {
		return 0, false
	}
	return epoch, true
}

func runtimeTrafficSamplingEpochUnchanged(epoch uint64) bool {
	return runtimeTrafficMutationState.active.Load() == 0 &&
		runtimeTrafficMutationState.epoch.Load() == epoch
}

// RegisterRuntimeSamplerWake connects the application-owned scheduler without
// making service depend on the cronjob package.  A nil callback cleanly
// disconnects it during panel shutdown.
func RegisterRuntimeSamplerWake(wake func()) {
	runtimeSamplerSignal.Lock()
	runtimeSamplerSignal.wake = wake
	runtimeSamplerSignal.Unlock()
}

// RegisterRuntimeSamplerDatabaseBarrier lets database replacement pause the
// sampler before closing SQLite and resume it only when a restore rolls back.
// The callbacks are optional so service packages and focused tests can run
// without a live scheduler.
func RegisterRuntimeSamplerDatabaseBarrier(pause func() error, resume func()) {
	runtimeSamplerSignal.Lock()
	runtimeSamplerSignal.pause = pause
	runtimeSamplerSignal.resume = resume
	runtimeSamplerSignal.Unlock()
}

func PauseRuntimeSamplerForDatabaseRestore() error {
	runtimeSamplerSignal.RLock()
	pause := runtimeSamplerSignal.pause
	runtimeSamplerSignal.RUnlock()
	if pause == nil {
		return nil
	}
	return pause()
}

func ResumeRuntimeSamplerAfterDatabaseRestoreFailure() {
	runtimeSamplerSignal.RLock()
	resume := runtimeSamplerSignal.resume
	runtimeSamplerSignal.RUnlock()
	if resume != nil {
		resume()
	}
}

func WakeRuntimeSampler() {
	runtimeSamplerSignal.RLock()
	wake := runtimeSamplerSignal.wake
	runtimeSamplerSignal.RUnlock()
	if wake != nil {
		wake()
	}
}
