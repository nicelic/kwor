package service

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

// NftablesCapabilities is the startup-cached feature profile used to choose
// the managed nftables layout and grammar. Unknown or unsupported versions
// are reported explicitly and must not be used for ruleset rendering.
type NftablesCapabilities struct {
	NftVersion              string
	KernelVersion           string
	CompatibilityMode       string
	SupportsJSON            bool
	SupportsNamedCounters   bool
	SupportsMeters          bool
	SupportsInetNAT         bool
	SupportsTransportHeader bool
	SupportsTableComments   bool
	RequiresNatBasePair     bool
	RendererSupported       bool
	CapabilityError         string
	VersionProbeError       string
	JSONProbeError          string
	MeterProbeError         string
}

type nftVersionNumber struct {
	major    int
	minor    int
	patch    int
	hasPatch bool
}

// nftRulesetRendererRevision identifies the command renderer rather than the
// observed nft userspace or kernel release. Bump it only when a future change
// alters the panel-owned ruleset grammar within an existing layout.
const nftRulesetRendererRevision = "v3"

const (
	nftMinimumMajor = 0
	nftMinimumMinor = 7
	nftMinimumPatch = 0

	nftMinimumKernelMajor = 4
	nftMinimumKernelMinor = 9
	nftMinimumKernelPatch = 0
)

var (
	nftVersionOutputRe = regexp.MustCompile(`(?i)\bnftables\s+v?(\d+)\.(\d+)(?:\.(\d+))?`)
	nftKernelVersionRe = regexp.MustCompile(`^(\d+)\.(\d+)(?:\.(\d+))?`)

	nftCapabilitiesRunVersionFn = func(binaryPath string) (string, error) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		output, err := exec.CommandContext(ctx, binaryPath, "--version").CombinedOutput()
		return strings.TrimSpace(string(output)), err
	}
	nftCapabilitiesRunJSONProbeFn = func(binaryPath string) error {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		output, err := exec.CommandContext(ctx, binaryPath, "-j", "list", "tables").CombinedOutput()
		if err != nil {
			return fmt.Errorf("nft -j list tables failed: %w: %s", err, strings.TrimSpace(string(output)))
		}
		return nil
	}

	nftCapabilitiesSnapshot = struct {
		mu    sync.RWMutex
		value NftablesCapabilities
	}{
		value: NftablesCapabilities{CompatibilityMode: "conservative", RequiresNatBasePair: true},
	}

	// nftCapabilityLayoutState remembers the ruleset renderer profile for which
	// the managed NAT layout was last rebuilt. It is intentionally process-local:
	// the rules are runtime-only and every cold start must reconcile any tables
	// left by an interrupted previous process before restoring them from SQLite.
	nftCapabilityLayoutState = struct {
		mu               sync.RWMutex
		appliedSignature string
		lastApplyError   string
	}{}
)

func parseNftablesVersion(raw string) (nftVersionNumber, bool) {
	match := nftVersionOutputRe.FindStringSubmatch(strings.TrimSpace(raw))
	if len(match) != 4 {
		return nftVersionNumber{}, false
	}
	major, majorErr := strconv.Atoi(match[1])
	minor, minorErr := strconv.Atoi(match[2])
	patch := 0
	var patchErr error
	if strings.TrimSpace(match[3]) != "" {
		patch, patchErr = strconv.Atoi(match[3])
	}
	if majorErr != nil || minorErr != nil || patchErr != nil {
		return nftVersionNumber{}, false
	}
	return nftVersionNumber{major: major, minor: minor, patch: patch, hasPatch: strings.TrimSpace(match[3]) != ""}, true
}

func parseNftKernelVersion(raw string) (nftVersionNumber, bool) {
	match := nftKernelVersionRe.FindStringSubmatch(strings.TrimSpace(raw))
	if len(match) != 4 {
		return nftVersionNumber{}, false
	}
	major, majorErr := strconv.Atoi(match[1])
	minor, minorErr := strconv.Atoi(match[2])
	patch := 0
	var patchErr error
	if strings.TrimSpace(match[3]) != "" {
		patch, patchErr = strconv.Atoi(match[3])
	}
	if majorErr != nil || minorErr != nil || patchErr != nil {
		return nftVersionNumber{}, false
	}
	return nftVersionNumber{major: major, minor: minor, patch: patch, hasPatch: strings.TrimSpace(match[3]) != ""}, true
}

func (v nftVersionNumber) atLeastVersion(major int, minor int, patch int) bool {
	if v.major != major {
		return v.major > major
	}
	if v.minor != minor {
		return v.minor > minor
	}
	return v.patch >= patch
}

func (v nftVersionNumber) String() string {
	if !v.hasPatch {
		return strconv.Itoa(v.major) + "." + strconv.Itoa(v.minor)
	}
	return strconv.Itoa(v.major) + "." + strconv.Itoa(v.minor) + "." + strconv.Itoa(v.patch)
}

func buildNftablesCapabilities(versionOutput string, kernelRelease string, probeErr error) NftablesCapabilities {
	capabilities := NftablesCapabilities{
		KernelVersion:       strings.TrimSpace(kernelRelease),
		CompatibilityMode:   "conservative",
		RequiresNatBasePair: true,
	}
	if probeErr != nil {
		capabilities.VersionProbeError = strings.TrimSpace(probeErr.Error())
	}

	nftVersion, nftVersionOK := parseNftablesVersion(versionOutput)
	if nftVersionOK {
		capabilities.NftVersion = nftVersion.String()
		capabilities.SupportsJSON = nftVersion.atLeastVersion(0, 9, 0)
		capabilities.SupportsNamedCounters = nftVersion.atLeastVersion(0, 9, 0)
		capabilities.SupportsTransportHeader = nftVersion.atLeastVersion(0, 9, 2)
	} else if capabilities.VersionProbeError == "" {
		raw := strings.TrimSpace(versionOutput)
		if raw == "" {
			raw = "empty output"
		}
		capabilities.VersionProbeError = fmt.Sprintf("unrecognized nftables version output: %s", raw)
	}

	kernelVersion, kernelVersionOK := parseNftKernelVersion(kernelRelease)
	if kernelVersionOK {
		capabilities.RequiresNatBasePair = !kernelVersion.atLeastVersion(4, 18, 0)
		capabilities.SupportsInetNAT = kernelVersion.atLeastVersion(5, 2, 0)
		capabilities.SupportsTableComments = nftVersionOK &&
			nftVersion.atLeastVersion(0, 9, 7) &&
			kernelVersion.atLeastVersion(5, 10, 0)
		// Meter keys containing conntrack original tuples are only advertised
		// on a conservative modern baseline. Older hosts keep forwarding but
		// explicitly report a degraded rate-limit state.
		capabilities.SupportsMeters = nftVersionOK &&
			nftVersion.atLeastVersion(0, 9, 0) &&
			kernelVersion.atLeastVersion(5, 1, 0)
	}

	switch {
	case capabilities.VersionProbeError != "":
		capabilities.CapabilityError = "nftables version probe failed: " + capabilities.VersionProbeError
	case !nftVersionOK:
		capabilities.CapabilityError = "nftables version is unavailable"
	case !kernelVersionOK:
		capabilities.CapabilityError = "kernel version is unavailable or unrecognized: " + strings.TrimSpace(kernelRelease)
	case !nftVersion.atLeastVersion(nftMinimumMajor, nftMinimumMinor, nftMinimumPatch):
		capabilities.CapabilityError = fmt.Sprintf(
			"nftables %s is unsupported; minimum supported version is %d.%d.%d",
			capabilities.NftVersion,
			nftMinimumMajor,
			nftMinimumMinor,
			nftMinimumPatch,
		)
	case !kernelVersion.atLeastVersion(nftMinimumKernelMajor, nftMinimumKernelMinor, nftMinimumKernelPatch):
		capabilities.CapabilityError = fmt.Sprintf(
			"kernel %s is unsupported; minimum supported version is %d.%d.%d",
			capabilities.KernelVersion,
			nftMinimumKernelMajor,
			nftMinimumKernelMinor,
			nftMinimumKernelPatch,
		)
	default:
		capabilities.RendererSupported = true
		if capabilities.SupportsTransportHeader &&
			capabilities.SupportsTableComments &&
			capabilities.SupportsNamedCounters &&
			capabilities.SupportsInetNAT {
			capabilities.CompatibilityMode = "native"
		} else {
			capabilities.CompatibilityMode = "compatibility"
		}
	}
	return capabilities
}

// RefreshNftablesCapabilities is intentionally called only by the panel
// lifecycle and after a successful nftables installation. It must never be
// called from overview polling, cron jobs, or a database transaction.
func RefreshNftablesCapabilities() NftablesCapabilities {
	kernelRelease := ""
	if platform, err := GetSystemPlatform(); err == nil && platform != nil {
		kernelRelease = platform.KernelRelease
	}

	versionOutput := ""
	var probeErr error
	binaryPath := ""
	if resolvedPath, err := resolveNftBinaryPath(); err == nil {
		binaryPath = resolvedPath
		versionOutput, probeErr = nftCapabilitiesRunVersionFn(binaryPath)
	} else {
		probeErr = err
	}

	capabilities := buildNftablesCapabilities(versionOutput, kernelRelease, probeErr)
	if capabilities.RendererSupported && capabilities.SupportsJSON && strings.TrimSpace(binaryPath) != "" {
		if err := nftCapabilitiesRunJSONProbeFn(binaryPath); err != nil {
			capabilities.SupportsJSON = false
			capabilities.JSONProbeError = strings.TrimSpace(err.Error())
		}
	}
	nftCapabilitiesSnapshot.mu.Lock()
	nftCapabilitiesSnapshot.value = capabilities
	nftCapabilitiesSnapshot.mu.Unlock()
	return capabilities
}

func GetNftablesCapabilities() NftablesCapabilities {
	nftCapabilitiesSnapshot.mu.RLock()
	capabilities := nftCapabilitiesSnapshot.value
	nftCapabilitiesSnapshot.mu.RUnlock()
	return capabilities
}

func nftUsesCompatibilityLayout() bool {
	return nftCapabilitiesUseCompatibilityLayout(GetNftablesCapabilities())
}

func nftCapabilitiesUseCompatibilityLayout(capabilities NftablesCapabilities) bool {
	return !capabilities.RendererSupported || !capabilities.SupportsNamedCounters || !capabilities.SupportsInetNAT
}

// nftCapabilityLayoutSignature deliberately describes only the ruleset
// grammar selected by this panel. NftVersion and KernelVersion remain useful
// diagnostic values, but a same-layout package or kernel update must not
// delete managed tables or reset native named counters. JSON support alone
// changes a reader path, not the persisted ruleset grammar.
func nftCapabilityLayoutSignature() string {
	capabilities := GetNftablesCapabilities()
	if !capabilities.RendererSupported {
		return nftRulesetRendererRevision + ":unsupported"
	}
	layout := "native"
	if nftCapabilitiesUseCompatibilityLayout(capabilities) {
		layout = "compatibility"
	}
	return nftRulesetRendererRevision + ":" + layout
}

// nftCapabilityLayoutReconcilePending reports whether the managed NAT layout
// still belongs to a different ruleset renderer profile. It only compares
// cached values and never executes a host command.
func nftCapabilityLayoutReconcilePending() bool {
	current := nftCapabilityLayoutSignature()
	nftCapabilityLayoutState.mu.RLock()
	applied := nftCapabilityLayoutState.appliedSignature
	nftCapabilityLayoutState.mu.RUnlock()
	return current != applied
}

// nftCapabilityLayoutHasAppliedSignature distinguishes a real in-process
// capability transition from a cold start. The signature deliberately remains
// process-local, so a cold start must inspect the existing ruleset before it
// decides whether the current layout is stale.
func nftCapabilityLayoutHasAppliedSignature() bool {
	nftCapabilityLayoutState.mu.RLock()
	applied := strings.TrimSpace(nftCapabilityLayoutState.appliedSignature)
	nftCapabilityLayoutState.mu.RUnlock()
	return applied != ""
}

func markNftCapabilityLayoutApplied() {
	signature := nftCapabilityLayoutSignature()
	nftCapabilityLayoutState.mu.Lock()
	nftCapabilityLayoutState.appliedSignature = signature
	nftCapabilityLayoutState.lastApplyError = ""
	nftCapabilityLayoutState.mu.Unlock()
}

func clearNftCapabilityLayoutApplied() {
	nftCapabilityLayoutState.mu.Lock()
	nftCapabilityLayoutState.appliedSignature = ""
	nftCapabilityLayoutState.lastApplyError = ""
	nftCapabilityLayoutState.mu.Unlock()
}

func setNftCapabilityLayoutApplyError(err error) {
	message := ""
	if err != nil {
		message = strings.TrimSpace(err.Error())
	}
	nftCapabilityLayoutState.mu.Lock()
	nftCapabilityLayoutState.lastApplyError = message
	nftCapabilityLayoutState.mu.Unlock()
}

func markNftCapabilityLayoutPending(err error) {
	message := ""
	if err != nil {
		message = strings.TrimSpace(err.Error())
	}
	nftCapabilityLayoutState.mu.Lock()
	nftCapabilityLayoutState.appliedSignature = ""
	nftCapabilityLayoutState.lastApplyError = message
	nftCapabilityLayoutState.mu.Unlock()
}

func nftCapabilityLayoutLastApplyError() string {
	nftCapabilityLayoutState.mu.RLock()
	message := nftCapabilityLayoutState.lastApplyError
	nftCapabilityLayoutState.mu.RUnlock()
	return message
}

func ensureNftRendererSupported() error {
	capabilities := GetNftablesCapabilities()
	if capabilities.RendererSupported {
		return nil
	}
	message := strings.TrimSpace(capabilities.CapabilityError)
	if message == "" {
		message = "nftables capability profile is not ready"
	}
	return fmt.Errorf("nftables renderer is unavailable: %s", message)
}

func markNftMetersUnsupported(cause error) {
	message := ""
	if cause != nil {
		message = strings.TrimSpace(cause.Error())
	}
	nftCapabilitiesSnapshot.mu.Lock()
	capabilities := nftCapabilitiesSnapshot.value
	capabilities.SupportsMeters = false
	capabilities.MeterProbeError = message
	nftCapabilitiesSnapshot.value = capabilities
	nftCapabilitiesSnapshot.mu.Unlock()
}
