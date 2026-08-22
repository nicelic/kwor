package service

import "testing"

func TestRuntimeTrafficMutationEpochRejectsOverlappingSamplingPass(t *testing.T) {
	runtimeTrafficMutationState.active.Store(0)
	runtimeTrafficMutationState.epoch.Store(0)
	t.Cleanup(func() {
		runtimeTrafficMutationState.active.Store(0)
		runtimeTrafficMutationState.epoch.Store(0)
	})

	epoch, ok := captureRuntimeTrafficSamplingEpoch()
	if !ok {
		t.Fatal("sampling epoch should be stable before a mutation")
	}
	finish := BeginRuntimeTrafficMutation()
	if _, ok := captureRuntimeTrafficSamplingEpoch(); ok {
		t.Fatal("sampling epoch should be unavailable while a mutation is active")
	}
	if runtimeTrafficSamplingEpochUnchanged(epoch) {
		t.Fatal("old sampling epoch was accepted during a mutation")
	}
	finish()
	if runtimeTrafficSamplingEpochUnchanged(epoch) {
		t.Fatal("old sampling epoch was accepted after a mutation")
	}
	if _, ok := captureRuntimeTrafficSamplingEpoch(); !ok {
		t.Fatal("sampling epoch should be stable after the mutation closes")
	}
}
