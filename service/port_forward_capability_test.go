package service

import (
	"errors"
	"testing"

	"github.com/alireza0/s-ui/database/model"
)

func TestPortForwardCapabilityStatusSeparatesSupportAndReadiness(t *testing.T) {
	withNftCapabilitiesTestGlobals(t)
	originalPlatform, originalPlatformErr := GetSystemPlatform()
	originalSupported := nftSupportedFn
	originalRunNft := runNftFn
	t.Cleanup(func() {
		nftSupportedFn = originalSupported
		runNftFn = originalRunNft
		if originalPlatformErr != nil || originalPlatform == nil {
			clearSystemPlatformSnapshot()
			return
		}
		setSystemPlatformSnapshot(originalPlatform)
	})

	setSystemPlatformSnapshot(&model.SystemPlatform{OS: "linux"})
	setNftCapabilitiesForTest(buildNftablesCapabilities("nftables v0.9.8", "5.10.0", nil))
	markNftCapabilityLayoutApplied()
	nftSupportedFn = func() bool { return true }
	runNftFn = func(args ...string) ([]byte, error) {
		if len(args) == 2 && args[0] == "list" && args[1] == "tables" {
			return nil, errors.New("operation not permitted")
		}
		return nil, nil
	}

	supported, ready, available, reason := portForwardCapabilityStatus()
	if !supported || ready || available || reason == "" {
		t.Fatalf("unexpected unavailable runtime state: supported=%v ready=%v available=%v reason=%q", supported, ready, available, reason)
	}

	runNftFn = func(args ...string) ([]byte, error) {
		if len(args) != 2 || args[0] != "list" || args[1] != "tables" {
			t.Fatalf("unexpected readiness command: %v", args)
		}
		return nil, nil
	}
	supported, ready, available, reason = portForwardCapabilityStatus()
	if !supported || !ready || !available || reason != "" {
		t.Fatalf("unexpected ready runtime state: supported=%v ready=%v available=%v reason=%q", supported, ready, available, reason)
	}
}

func TestPortForwardCapabilityStatusRejectsUnknownAndPendingRendererProfiles(t *testing.T) {
	withNftCapabilitiesTestGlobals(t)

	originalPlatform, originalPlatformErr := GetSystemPlatform()
	t.Cleanup(func() {
		if originalPlatformErr != nil || originalPlatform == nil {
			clearSystemPlatformSnapshot()
			return
		}
		setSystemPlatformSnapshot(originalPlatform)
	})
	setSystemPlatformSnapshot(&model.SystemPlatform{OS: "linux"})
	nftSupportedFn = func() bool { return true }
	runNftFn = func(args ...string) ([]byte, error) { return nil, nil }

	setNftCapabilitiesForTest(buildNftablesCapabilities("unexpected", "5.10.0", nil))
	supported, ready, available, reason := portForwardCapabilityStatus()
	if !supported || ready || available || reason == "" {
		t.Fatalf("unknown version must not be ready: supported=%v ready=%v available=%v reason=%q", supported, ready, available, reason)
	}

	setNftCapabilitiesForTest(buildNftablesCapabilities("nftables v0.9.8", "5.10.0", nil))
	clearNftCapabilityLayoutApplied()
	supported, ready, available, reason = portForwardCapabilityStatus()
	if !supported || ready || !available || reason == "" {
		t.Fatalf("pending layout must be executable but not ready: supported=%v ready=%v available=%v reason=%q", supported, ready, available, reason)
	}
}
