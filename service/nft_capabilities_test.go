package service

import (
	"errors"
	"strings"
	"testing"

	"github.com/alireza0/s-ui/database/model"
)

func withNftCapabilitiesTestGlobals(t *testing.T) {
	t.Helper()

	originalRunNft := runNftFn
	originalRunNftScript := runNftScriptFn
	originalSupported := nftSupportedFn
	originalIsLinux := nftIsLinuxHost
	originalLookPath := nftLookPathFn
	originalVersionCommand := nftCapabilitiesRunVersionFn
	originalJSONProbe := nftCapabilitiesRunJSONProbeFn
	nftCapabilitiesSnapshot.mu.RLock()
	originalCapabilities := nftCapabilitiesSnapshot.value
	nftCapabilitiesSnapshot.mu.RUnlock()
	nftCapabilityLayoutState.mu.RLock()
	originalAppliedLayout := nftCapabilityLayoutState.appliedSignature
	originalApplyError := nftCapabilityLayoutState.lastApplyError
	nftCapabilityLayoutState.mu.RUnlock()
	nftLifecycleRestoreState.mu.RLock()
	originalBaseSignature := nftLifecycleRestoreState.baseSignature
	originalBaseRestoreError := nftLifecycleRestoreState.baseRestoreError
	nftLifecycleRestoreState.mu.RUnlock()

	t.Cleanup(func() {
		runNftFn = originalRunNft
		runNftScriptFn = originalRunNftScript
		nftSupportedFn = originalSupported
		nftIsLinuxHost = originalIsLinux
		nftLookPathFn = originalLookPath
		nftCapabilitiesRunVersionFn = originalVersionCommand
		nftCapabilitiesRunJSONProbeFn = originalJSONProbe
		nftCapabilitiesSnapshot.mu.Lock()
		nftCapabilitiesSnapshot.value = originalCapabilities
		nftCapabilitiesSnapshot.mu.Unlock()
		nftCapabilityLayoutState.mu.Lock()
		nftCapabilityLayoutState.appliedSignature = originalAppliedLayout
		nftCapabilityLayoutState.lastApplyError = originalApplyError
		nftCapabilityLayoutState.mu.Unlock()
		nftLifecycleRestoreState.mu.Lock()
		nftLifecycleRestoreState.baseSignature = originalBaseSignature
		nftLifecycleRestoreState.baseRestoreError = originalBaseRestoreError
		nftLifecycleRestoreState.mu.Unlock()
	})

	nftCapabilitiesRunJSONProbeFn = func(string) error { return nil }
	runNftScriptFn = func(script string) ([]byte, error) {
		var output []byte
		for _, line := range strings.Split(script, "\n") {
			args := strings.Fields(strings.TrimSpace(line))
			if len(args) == 0 {
				continue
			}
			current, err := runNftFn(args...)
			if err != nil {
				return nil, err
			}
			output = current
		}
		return output, nil
	}
}

func setNftCapabilitiesForTest(capabilities NftablesCapabilities) {
	nftCapabilitiesSnapshot.mu.Lock()
	nftCapabilitiesSnapshot.value = capabilities
	nftCapabilitiesSnapshot.mu.Unlock()
}

func TestBuildNftablesCapabilitiesDebianBaselineMatrix(t *testing.T) {
	testCases := []struct {
		name          string
		versionOutput string
		kernelRelease string
		mode          string
		json          bool
		namedCounters bool
		meters        bool
		inetNAT       bool
		transport     bool
		tableComments bool
		natBasePair   bool
		renderer      bool
		capabilityErr bool
	}{
		{
			name:          "debian 9",
			versionOutput: "nftables v0.7 (Scrooge McDuck)",
			kernelRelease: "4.9.0-18-amd64",
			mode:          "compatibility",
			natBasePair:   true,
			renderer:      true,
		},
		{
			name:          "debian 10",
			versionOutput: "nftables v0.9.0 (Topsy)",
			kernelRelease: "4.19.0-26-amd64",
			mode:          "compatibility",
			json:          true,
			namedCounters: true,
			renderer:      true,
		},
		{
			name:          "nft 0.9.2 transport header boundary",
			versionOutput: "nftables v0.9.2",
			kernelRelease: "5.10.0",
			mode:          "compatibility",
			json:          true,
			namedCounters: true,
			meters:        true,
			inetNAT:       true,
			transport:     true,
			renderer:      true,
		},
		{
			name:          "nft 0.9.7 table comment boundary",
			versionOutput: "nftables v0.9.7",
			kernelRelease: "5.10.0",
			mode:          "native",
			json:          true,
			namedCounters: true,
			meters:        true,
			inetNAT:       true,
			transport:     true,
			tableComments: true,
			renderer:      true,
		},
		{
			name:          "debian 11",
			versionOutput: "nftables v0.9.8 (E.D.S.)",
			kernelRelease: "5.10.0-32-amd64",
			mode:          "native",
			json:          true,
			namedCounters: true,
			inetNAT:       true,
			meters:        true,
			transport:     true,
			tableComments: true,
			renderer:      true,
		},
		{
			name:          "unknown versions stay conservative",
			versionOutput: "unexpected output",
			kernelRelease: "unknown",
			mode:          "conservative",
			natBasePair:   true,
			capabilityErr: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			capabilities := buildNftablesCapabilities(testCase.versionOutput, testCase.kernelRelease, nil)
			if capabilities.CompatibilityMode != testCase.mode {
				t.Fatalf("mode=%q want=%q", capabilities.CompatibilityMode, testCase.mode)
			}
			if capabilities.SupportsJSON != testCase.json {
				t.Fatalf("supports JSON=%v want=%v", capabilities.SupportsJSON, testCase.json)
			}
			if capabilities.SupportsNamedCounters != testCase.namedCounters {
				t.Fatalf("supports named counters=%v want=%v", capabilities.SupportsNamedCounters, testCase.namedCounters)
			}
			if capabilities.SupportsMeters != testCase.meters {
				t.Fatalf("supports meters=%v want=%v", capabilities.SupportsMeters, testCase.meters)
			}
			if capabilities.SupportsInetNAT != testCase.inetNAT {
				t.Fatalf("supports inet NAT=%v want=%v", capabilities.SupportsInetNAT, testCase.inetNAT)
			}
			if capabilities.SupportsTransportHeader != testCase.transport {
				t.Fatalf("supports transport header=%v want=%v", capabilities.SupportsTransportHeader, testCase.transport)
			}
			if capabilities.SupportsTableComments != testCase.tableComments {
				t.Fatalf("supports table comments=%v want=%v", capabilities.SupportsTableComments, testCase.tableComments)
			}
			if capabilities.RequiresNatBasePair != testCase.natBasePair {
				t.Fatalf("requires NAT pair=%v want=%v", capabilities.RequiresNatBasePair, testCase.natBasePair)
			}
			if capabilities.RendererSupported != testCase.renderer {
				t.Fatalf("renderer supported=%v want=%v", capabilities.RendererSupported, testCase.renderer)
			}
			if (strings.TrimSpace(capabilities.CapabilityError) != "") != testCase.capabilityErr {
				t.Fatalf("capability error=%q wantError=%v", capabilities.CapabilityError, testCase.capabilityErr)
			}
		})
	}
}

func TestBuildNftablesCapabilitiesRejectsProbeAndMinimumVersionFailures(t *testing.T) {
	testCases := []struct {
		name          string
		versionOutput string
		kernelRelease string
		probeErr      error
		wantError     string
	}{
		{name: "version probe failure", versionOutput: "nftables v1.0.0", kernelRelease: "6.1.0", probeErr: errors.New("permission denied"), wantError: "version probe failed"},
		{name: "nft below minimum", versionOutput: "nftables v0.6", kernelRelease: "4.9.0", wantError: "minimum supported version"},
		{name: "kernel below minimum", versionOutput: "nftables v0.7", kernelRelease: "4.8.0", wantError: "minimum supported version"},
	}
	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			capabilities := buildNftablesCapabilities(testCase.versionOutput, testCase.kernelRelease, testCase.probeErr)
			if capabilities.RendererSupported {
				t.Fatal("failed or unsupported capability probe must not enable the renderer")
			}
			if !strings.Contains(capabilities.CapabilityError, testCase.wantError) {
				t.Fatalf("capability error=%q want substring=%q", capabilities.CapabilityError, testCase.wantError)
			}
		})
	}
}

func TestRefreshNftablesCapabilitiesCachesVersionAndKernel(t *testing.T) {
	withNftCapabilitiesTestGlobals(t)

	originalPlatform, originalPlatformErr := GetSystemPlatform()
	t.Cleanup(func() {
		if originalPlatformErr != nil || originalPlatform == nil {
			clearSystemPlatformSnapshot()
			return
		}
		setSystemPlatformSnapshot(originalPlatform)
	})
	setSystemPlatformSnapshot(&model.SystemPlatform{OS: "linux", KernelRelease: "4.19.0-26-amd64"})
	nftIsLinuxHost = func() bool { return true }
	nftLookPathFn = func(string) (string, error) { return "/test/nft", nil }
	nftCapabilitiesRunVersionFn = func(binaryPath string) (string, error) {
		if binaryPath != "/test/nft" {
			t.Fatalf("unexpected binary path: %s", binaryPath)
		}
		return "nftables v0.9.0 (Topsy)", nil
	}

	capabilities := RefreshNftablesCapabilities()
	if capabilities.NftVersion != "0.9.0" {
		t.Fatalf("nft version=%q", capabilities.NftVersion)
	}
	if capabilities.KernelVersion != "4.19.0-26-amd64" {
		t.Fatalf("kernel version=%q", capabilities.KernelVersion)
	}
	if capabilities.CompatibilityMode != "compatibility" {
		t.Fatalf("mode=%q", capabilities.CompatibilityMode)
	}
	if GetNftablesCapabilities() != capabilities {
		t.Fatal("capability snapshot was not cached")
	}

	setSystemPlatformSnapshot(&model.SystemPlatform{OS: "linux", KernelRelease: "5.10.0-32-amd64"})
	nftCapabilitiesRunVersionFn = func(string) (string, error) {
		return "nftables v0.9.8 (E.D.S.)", nil
	}
	refreshed := RefreshNftablesCapabilities()
	if refreshed.CompatibilityMode != "native" || refreshed.NftVersion != "0.9.8" {
		t.Fatalf("refresh did not replace cached profile: %+v", refreshed)
	}
}

func TestRefreshNftablesCapabilitiesDowngradesOnlyJSONReaderAfterProbeFailure(t *testing.T) {
	withNftCapabilitiesTestGlobals(t)

	originalPlatform, originalPlatformErr := GetSystemPlatform()
	t.Cleanup(func() {
		if originalPlatformErr != nil || originalPlatform == nil {
			clearSystemPlatformSnapshot()
			return
		}
		setSystemPlatformSnapshot(originalPlatform)
	})
	setSystemPlatformSnapshot(&model.SystemPlatform{OS: "linux", KernelRelease: "5.10.0"})
	nftIsLinuxHost = func() bool { return true }
	nftLookPathFn = func(string) (string, error) { return "/test/nft", nil }
	nftCapabilitiesRunVersionFn = func(string) (string, error) { return "nftables v0.9.8", nil }
	nftCapabilitiesRunJSONProbeFn = func(string) error { return errors.New("json rejected") }

	capabilities := RefreshNftablesCapabilities()
	if !capabilities.RendererSupported {
		t.Fatalf("JSON reader failure must not disable ruleset rendering: %+v", capabilities)
	}
	if capabilities.SupportsJSON {
		t.Fatal("failed JSON probe must select the text reader")
	}
	if !strings.Contains(capabilities.JSONProbeError, "json rejected") {
		t.Fatalf("missing JSON probe diagnostic: %+v", capabilities)
	}
	if nftUsesCompatibilityLayout() {
		t.Fatal("JSON reader fallback alone must not change the NAT layout")
	}
}

func TestRefreshNftablesCapabilitiesUpdatesObservedVersionWithoutChangingRenderer(t *testing.T) {
	withNftCapabilitiesTestGlobals(t)

	originalPlatform, originalPlatformErr := GetSystemPlatform()
	t.Cleanup(func() {
		if originalPlatformErr != nil || originalPlatform == nil {
			clearSystemPlatformSnapshot()
			return
		}
		setSystemPlatformSnapshot(originalPlatform)
	})
	nftIsLinuxHost = func() bool { return true }
	nftLookPathFn = func(string) (string, error) { return "/test/nft", nil }

	setSystemPlatformSnapshot(&model.SystemPlatform{OS: "linux", KernelRelease: "5.10.0-32-amd64"})
	nftCapabilitiesRunVersionFn = func(string) (string, error) {
		return "nftables v0.9.8 (E.D.S.)", nil
	}
	before := RefreshNftablesCapabilities()
	beforeSignature := nftCapabilityLayoutSignature()

	setSystemPlatformSnapshot(&model.SystemPlatform{OS: "linux", KernelRelease: "6.12.0-1-amd64"})
	nftCapabilitiesRunVersionFn = func(string) (string, error) {
		return "nftables v1.1.1", nil
	}
	after := RefreshNftablesCapabilities()
	afterSignature := nftCapabilityLayoutSignature()

	if before.NftVersion != "0.9.8" || before.KernelVersion != "5.10.0-32-amd64" {
		t.Fatalf("unexpected initial observed versions: %+v", before)
	}
	if after.NftVersion != "1.1.1" || after.KernelVersion != "6.12.0-1-amd64" {
		t.Fatalf("refresh did not retain new observed versions: %+v", after)
	}
	if beforeSignature != afterSignature {
		t.Fatalf("same renderer must retain its layout signature: before=%q after=%q", beforeSignature, afterSignature)
	}
}

func TestGetNftablesCapabilitiesNeverReprobesCachedVersion(t *testing.T) {
	withNftCapabilitiesTestGlobals(t)

	setNftCapabilitiesForTest(buildNftablesCapabilities("nftables v0.9.0", "4.19.0", nil))
	probes := 0
	nftCapabilitiesRunVersionFn = func(string) (string, error) {
		probes++
		return "nftables v9.9.9", nil
	}

	for i := 0; i < 4; i++ {
		capabilities := GetNftablesCapabilities()
		if capabilities.NftVersion != "0.9.0" {
			t.Fatalf("cached capability changed during read: %+v", capabilities)
		}
	}
	if probes != 0 {
		t.Fatalf("cached reads must not execute nft --version, probes=%d", probes)
	}
}

func TestNftCapabilityLayoutSignatureTracksRulesetRendererOnly(t *testing.T) {
	withNftCapabilitiesTestGlobals(t)

	testCases := []struct {
		name  string
		left  NftablesCapabilities
		right NftablesCapabilities
		same  bool
	}{
		{
			name:  "modern nft and kernel upgrades retain native renderer",
			left:  buildNftablesCapabilities("nftables v0.9.8", "5.10.0-32-amd64", nil),
			right: buildNftablesCapabilities("nftables v1.1.1", "6.12.0-1-amd64", nil),
			same:  true,
		},
		{
			name:  "old kernel keeps nft userspace upgrade in compatibility renderer",
			left:  buildNftablesCapabilities("nftables v0.7", "4.9.0-18-amd64", nil),
			right: buildNftablesCapabilities("nftables v0.9.0", "4.19.0-26-amd64", nil),
			same:  true,
		},
		{
			name:  "inet nat capability changes renderer",
			left:  buildNftablesCapabilities("nftables v0.9.0", "4.19.0-26-amd64", nil),
			right: buildNftablesCapabilities("nftables v0.9.8", "5.10.0-32-amd64", nil),
			same:  false,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			setNftCapabilitiesForTest(testCase.left)
			leftSignature := nftCapabilityLayoutSignature()
			setNftCapabilitiesForTest(testCase.right)
			rightSignature := nftCapabilityLayoutSignature()
			if (leftSignature == rightSignature) != testCase.same {
				t.Fatalf("renderer signatures left=%q right=%q same=%v want=%v", leftSignature, rightSignature, leftSignature == rightSignature, testCase.same)
			}
		})
	}
}

func TestNftCapabilitySameRendererProfileSkipsLayoutMigration(t *testing.T) {
	testCases := []struct {
		name string
		old  NftablesCapabilities
		new  NftablesCapabilities
	}{
		{
			name: "native package and kernel upgrade",
			old:  buildNftablesCapabilities("nftables v0.9.8", "5.10.0", nil),
			new:  buildNftablesCapabilities("nftables v1.1.1", "6.12.0", nil),
		},
		{
			name: "compatibility package and kernel upgrade",
			old:  buildNftablesCapabilities("nftables v0.7", "4.9.0", nil),
			new:  buildNftablesCapabilities("nftables v0.9.0", "4.19.0", nil),
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			withNftCapabilitiesTestGlobals(t)
			nftSupportedFn = func() bool { return true }
			setNftCapabilitiesForTest(testCase.old)
			markNftCapabilityLayoutApplied()
			setNftCapabilitiesForTest(testCase.new)

			commands := make([]string, 0)
			runNftFn = func(args ...string) ([]byte, error) {
				commands = append(commands, strings.Join(args, " "))
				return nil, nil
			}

			transitioned, err := prepareNftablesCapabilityLayoutTransition()
			if err != nil || transitioned {
				t.Fatalf("same renderer profile must not migrate: transitioned=%v err=%v", transitioned, err)
			}
			if nftCapabilityLayoutReconcilePending() {
				t.Fatal("same renderer profile must remain applied after observed version refresh")
			}
			if len(commands) != 0 {
				t.Fatalf("same renderer profile must not execute nft cleanup commands: %v", commands)
			}
		})
	}
}

func TestNftCapabilityLayoutTransitionCleansBothLayoutsBeforeMarkingApplied(t *testing.T) {
	withNftCapabilitiesTestGlobals(t)
	nftSupportedFn = func() bool { return true }

	oldCapabilities := buildNftablesCapabilities("nftables v0.9.0", "4.19.0", nil)
	setNftCapabilitiesForTest(oldCapabilities)
	nftCapabilityLayoutState.mu.Lock()
	nftCapabilityLayoutState.appliedSignature = nftCapabilityLayoutSignature()
	nftCapabilityLayoutState.mu.Unlock()
	setNftCapabilitiesForTest(buildNftablesCapabilities("nftables v0.9.8", "5.10.0", nil))

	commands := make([]string, 0)
	runNftFn = func(args ...string) ([]byte, error) {
		command := strings.Join(args, " ")
		commands = append(commands, command)
		if strings.HasPrefix(command, "list table ") {
			return []byte(`table inet managed { comment "kwor-owner:v1" }`), nil
		}
		return nil, nil
	}

	transitioned, err := prepareNftablesCapabilityLayoutTransition()
	if err != nil || !transitioned {
		t.Fatalf("prepare transition transitioned=%v err=%v", transitioned, err)
	}
	if !nftCapabilityLayoutReconcilePending() {
		t.Fatal("layout must remain pending until all managed rules are restored")
	}
	for _, expected := range []string{
		"delete chain inet " + nftTable + " " + nftChainPrerouting,
		"delete table ip " + nftTable,
		"delete table ip6 " + nftTable,
	} {
		if !containsNftCommand(commands, expected) {
			t.Fatalf("missing layout cleanup command %q: %v", expected, commands)
		}
	}

	markNftCapabilityLayoutApplied()
	if nftCapabilityLayoutReconcilePending() {
		t.Fatal("layout should be applied only after explicit successful restore")
	}
}

func TestNftCapabilityLayoutTransitionCleansNativeBeforeCompatibilityRestore(t *testing.T) {
	withNftCapabilitiesTestGlobals(t)
	nftSupportedFn = func() bool { return true }
	originalPlatform, originalPlatformErr := GetSystemPlatform()
	t.Cleanup(func() {
		if originalPlatformErr != nil || originalPlatform == nil {
			clearSystemPlatformSnapshot()
			return
		}
		setSystemPlatformSnapshot(originalPlatform)
	})
	setSystemPlatformSnapshot(&model.SystemPlatform{OS: "linux", KernelRelease: "4.9.0"})

	setNftCapabilitiesForTest(buildNftablesCapabilities("nftables v0.9.8", "5.10.0", nil))
	nftCapabilityLayoutState.mu.Lock()
	nftCapabilityLayoutState.appliedSignature = nftCapabilityLayoutSignature()
	nftCapabilityLayoutState.mu.Unlock()
	setNftCapabilitiesForTest(buildNftablesCapabilities("nftables v0.7", "4.9.0", nil))

	commands := make([]string, 0)
	runNftFn = func(args ...string) ([]byte, error) {
		command := strings.Join(args, " ")
		commands = append(commands, command)
		if strings.HasPrefix(command, "list table ") {
			return []byte(`table inet managed { comment "kwor-owner:v1" }`), nil
		}
		return nil, nil
	}

	transitioned, err := prepareNftablesCapabilityLayoutTransition()
	if err != nil || !transitioned {
		t.Fatalf("prepare reverse transition transitioned=%v err=%v", transitioned, err)
	}
	for _, expected := range []string{
		"delete chain inet " + nftTable + " " + nftChainPrerouting,
		"delete table ip " + nftTable,
		"delete table ip6 " + nftTable,
		"delete table inet " + portForwardNftTable,
		"delete table ip " + portForwardNftTable,
		"delete table ip6 " + portForwardNftTable,
	} {
		if !containsNftCommand(commands, expected) {
			t.Fatalf("missing reverse layout cleanup command %q: %v", expected, commands)
		}
	}
	if !nftCapabilityLayoutReconcilePending() {
		t.Fatal("reverse transition must remain pending until compatibility rules are restored")
	}
}

func TestNftCapabilityLayoutTransitionKeepsPendingOnCleanupFailure(t *testing.T) {
	withNftCapabilitiesTestGlobals(t)
	nftSupportedFn = func() bool { return true }

	setNftCapabilitiesForTest(buildNftablesCapabilities("nftables v0.9.0", "4.19.0", nil))
	nftCapabilityLayoutState.mu.Lock()
	nftCapabilityLayoutState.appliedSignature = nftCapabilityLayoutSignature()
	nftCapabilityLayoutState.mu.Unlock()
	setNftCapabilitiesForTest(buildNftablesCapabilities("nftables v0.9.8", "5.10.0", nil))

	runNftFn = func(args ...string) ([]byte, error) {
		command := strings.Join(args, " ")
		if strings.HasPrefix(command, "list table ") {
			return []byte(`table inet managed { comment "kwor-owner:v1" }`), nil
		}
		if command == "delete table ip "+nftTable {
			return nil, errors.New("delete failed")
		}
		return nil, nil
	}

	transitioned, err := prepareNftablesCapabilityLayoutTransition()
	if !transitioned || err == nil {
		t.Fatalf("expected failed transition to remain pending: transitioned=%v err=%v", transitioned, err)
	}
	if !nftCapabilityLayoutReconcilePending() {
		t.Fatal("failed cleanup must not mark the new layout as applied")
	}
}

func TestNftCapabilityColdStartKeepsMatchingLayoutAndCleansOppositeLayout(t *testing.T) {
	testCases := []struct {
		name            string
		profile         NftablesCapabilities
		existingCommand string
		wantFullCleanup bool
	}{
		{
			name:    "native cold start keeps native counters",
			profile: buildNftablesCapabilities("nftables v0.9.8", "5.10.0", nil),
		},
		{
			name:            "native cold start removes compatibility residue",
			profile:         buildNftablesCapabilities("nftables v0.9.8", "5.10.0", nil),
			existingCommand: "list table ip " + nftTable,
			wantFullCleanup: true,
		},
		{
			name:            "compatibility cold start removes native residue",
			profile:         buildNftablesCapabilities("nftables v0.7", "4.9.0", nil),
			existingCommand: "list chain inet " + nftTable + " " + nftChainPrerouting,
			wantFullCleanup: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			withNftCapabilitiesTestGlobals(t)
			nftSupportedFn = func() bool { return true }
			setNftCapabilitiesForTest(testCase.profile)

			commands := make([]string, 0)
			fullCleanupStarted := false
			runNftFn = func(args ...string) ([]byte, error) {
				command := strings.Join(args, " ")
				commands = append(commands, command)
				if command == testCase.existingCommand {
					fullCleanupStarted = true
					if strings.HasPrefix(command, "list table ") {
						return []byte(`table inet managed { comment "kwor-owner:v1" }`), nil
					}
					return nil, nil
				}
				if strings.HasPrefix(testCase.existingCommand, "list chain inet ") && command == "list table inet "+nftTable {
					return []byte(`table inet managed { comment "kwor-owner:v1" }`), nil
				}
				if fullCleanupStarted && strings.HasPrefix(command, "list table ") {
					return []byte(`table inet managed { comment "kwor-owner:v1" }`), nil
				}
				if strings.HasPrefix(command, "list ") {
					return nil, errors.New("No such file or directory")
				}
				return nil, nil
			}

			transitioned, err := prepareNftablesCapabilityLayoutTransition()
			if err != nil || !transitioned {
				t.Fatalf("prepare cold-start transition transitioned=%v err=%v", transitioned, err)
			}
			gotFullCleanup := false
			for _, command := range commands {
				if strings.HasPrefix(command, "delete ") {
					gotFullCleanup = true
					break
				}
			}
			if gotFullCleanup != testCase.wantFullCleanup {
				t.Fatalf("full cleanup=%v want=%v, commands=%v", gotFullCleanup, testCase.wantFullCleanup, commands)
			}
			if !testCase.wantFullCleanup {
				for _, command := range commands {
					if strings.HasPrefix(command, "delete ") {
						t.Fatalf("matching cold-start layout must not be deleted: %v", commands)
					}
				}
			}
			if !nftCapabilityLayoutReconcilePending() {
				t.Fatal("cold-start restore must remain pending until lifecycle verification succeeds")
			}
		})
	}
}

func TestPortForwardLayoutMigrationRequiresKnownPriorLayout(t *testing.T) {
	if portForwardLayoutMigrationRequired("", "v1:native") {
		t.Fatal("empty previous signature represents a cold start, not a migration")
	}
	if portForwardLayoutMigrationRequired("v1:native", "v1:native") {
		t.Fatal("identical layouts must not migrate")
	}
	if !portForwardLayoutMigrationRequired("v1:compatibility", "v1:native") {
		t.Fatal("known layout changes must migrate")
	}
}

func TestEnsureNftCompatibilityNatChainsRollsBackCreatedIPv4BaseOnIPv6Failure(t *testing.T) {
	withNftCapabilitiesTestGlobals(t)
	setNftCapabilitiesForTest(buildNftablesCapabilities("nftables v0.7", "4.9.0", nil))

	commands := make([]string, 0)
	createdTables := make(map[string]bool)
	runNftFn = func(args ...string) ([]byte, error) {
		command := strings.Join(args, " ")
		commands = append(commands, command)
		switch {
		case strings.HasPrefix(command, "list table "):
			if createdTables[command] {
				return []byte(`table ip kwor { comment "kwor-owner:v1" }`), nil
			}
			return nil, errors.New("No such file or directory")
		case strings.HasPrefix(command, "list chain "):
			return nil, errors.New("No such file or directory")
		case strings.HasPrefix(command, "add table ip6 "+nftTable):
			return nil, errors.New("ip6 table rejected")
		case strings.HasPrefix(command, "add table ip "+nftTable):
			createdTables["list table ip "+nftTable] = true
			return nil, nil
		default:
			return nil, nil
		}
	}

	if err := ensureNftCompatibilityNatChains(); err == nil {
		t.Fatal("expected ip6 base creation failure")
	}
	if !containsNftCommand(commands, "delete table ip "+nftTable) {
		t.Fatalf("missing IPv4 base rollback: %v", commands)
	}
}

func TestEnsureCompatibilityPortForwardBaseRollsBackAllCreatedTablesOnIPv6Failure(t *testing.T) {
	withNftCapabilitiesTestGlobals(t)
	setNftCapabilitiesForTest(buildNftablesCapabilities("nftables v0.7", "4.9.0", nil))

	commands := make([]string, 0)
	createdTables := make(map[string]bool)
	runNftFn = func(args ...string) ([]byte, error) {
		command := strings.Join(args, " ")
		commands = append(commands, command)
		switch {
		case strings.HasPrefix(command, "list table "):
			if createdTables[command] {
				return []byte(`table inet kwor_forward { comment "kwor-owner:v1" }`), nil
			}
			return nil, errors.New("No such file or directory")
		case strings.HasPrefix(command, "list chain "):
			return nil, errors.New("No such file or directory")
		case strings.HasPrefix(command, "add table ip6 "+portForwardNftTable):
			return nil, errors.New("ip6 table rejected")
		case strings.HasPrefix(command, "add table "):
			parts := strings.Fields(command)
			if len(parts) >= 4 {
				createdTables["list table "+parts[2]+" "+parts[3]] = true
			}
			return nil, nil
		default:
			return nil, nil
		}
	}

	if err := ensureCompatibilityPortForwardBase(); err == nil {
		t.Fatal("expected compatibility forwarding base creation to fail")
	}
	for _, expected := range []string{
		"delete table ip " + portForwardNftTable,
		"delete table inet " + portForwardNftTable,
	} {
		if !containsNftCommand(commands, expected) {
			t.Fatalf("missing forwarding base rollback command %q: %v", expected, commands)
		}
	}
}

func TestPortHopNatBaseChainsUseNumericPrioritiesAcrossLayouts(t *testing.T) {
	testCases := []struct {
		name     string
		profile  NftablesCapabilities
		expected []string
	}{
		{
			name:    "debian 9 compatibility layout",
			profile: buildNftablesCapabilities("nftables v0.7", "4.9.0", nil),
			expected: []string{
				"add chain ip " + nftTable + " " + nftChainPrerouting + " { type nat hook prerouting priority -100 ; policy accept ; }",
				"add chain ip " + nftTable + " " + nftChainPostrouting + " { type nat hook postrouting priority 100 ; policy accept ; }",
				"add chain ip6 " + nftTable + " " + nftChainPrerouting + " { type nat hook prerouting priority -100 ; policy accept ; }",
				"add chain ip6 " + nftTable + " " + nftChainPostrouting + " { type nat hook postrouting priority 100 ; policy accept ; }",
			},
		},
		{
			name:    "nft 0.9 native layout",
			profile: buildNftablesCapabilities("nftables v0.9.0", "5.2.0", nil),
			expected: []string{
				"add chain inet " + nftTable + " " + nftChainPrerouting + " { type nat hook prerouting priority -100 ; policy accept ; }",
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			withNftCapabilitiesTestGlobals(t)
			nftSupportedFn = func() bool { return true }
			setNftCapabilitiesForTest(testCase.profile)

			commands := make([]string, 0)
			createdTables := make(map[string]bool)
			runNftFn = func(args ...string) ([]byte, error) {
				command := strings.Join(args, " ")
				commands = append(commands, command)
				if strings.HasPrefix(command, "list table ") {
					if createdTables[command] {
						return []byte("table x y { chain " + nftOwnershipChain + " { counter comment \"" + nftTableOwnershipMarker + "\" } }"), nil
					}
					return nil, errors.New("No such file or directory")
				}
				if strings.HasPrefix(command, "add table ") {
					parts := strings.Fields(command)
					createdTables["list table "+parts[2]+" "+parts[3]] = true
					return nil, nil
				}
				if strings.HasPrefix(command, "list chain ") {
					return nil, errors.New("No such file or directory")
				}
				return nil, nil
			}

			if err := ensureNftNatChain(); err != nil {
				t.Fatalf("ensure NAT base chain failed: %v", err)
			}
			for _, expected := range testCase.expected {
				if !containsNftCommand(commands, expected) {
					t.Fatalf("missing numeric-priority command %q: %v", expected, commands)
				}
			}
			for _, command := range commands {
				if strings.Contains(command, "priority dstnat") || strings.Contains(command, "priority srcnat") {
					t.Fatalf("legacy-compatible NAT command must not use a symbolic priority: %s", command)
				}
			}
		})
	}
}

func TestPortForwardNatBaseChainsUseNumericPrioritiesAcrossLayouts(t *testing.T) {
	testCases := []struct {
		name     string
		profile  NftablesCapabilities
		ensure   func() error
		expected []string
	}{
		{
			name:    "debian 9 compatibility layout",
			profile: buildNftablesCapabilities("nftables v0.7", "4.9.0", nil),
			ensure:  ensureCompatibilityPortForwardBase,
			expected: []string{
				"add chain ip " + portForwardNftTable + " " + portForwardPreroutingChain + " { type nat hook prerouting priority -100 ; policy accept ; }",
				"add chain ip " + portForwardNftTable + " " + portForwardPostroutingChain + " { type nat hook postrouting priority 100 ; policy accept ; }",
				"add chain ip6 " + portForwardNftTable + " " + portForwardPreroutingChain + " { type nat hook prerouting priority -100 ; policy accept ; }",
				"add chain ip6 " + portForwardNftTable + " " + portForwardPostroutingChain + " { type nat hook postrouting priority 100 ; policy accept ; }",
			},
		},
		{
			name:    "nft 0.9 native layout",
			profile: buildNftablesCapabilities("nftables v0.9.0", "5.2.0", nil),
			ensure:  ensureNativePortForwardBase,
			expected: []string{
				"add chain inet " + portForwardNftTable + " " + portForwardPreroutingChain + " { type nat hook prerouting priority -100 ; policy accept ; }",
				"add chain inet " + portForwardNftTable + " " + portForwardPostroutingChain + " { type nat hook postrouting priority 100 ; policy accept ; }",
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			withNftCapabilitiesTestGlobals(t)
			setNftCapabilitiesForTest(testCase.profile)

			commands := make([]string, 0)
			createdTables := make(map[string]bool)
			runNftFn = func(args ...string) ([]byte, error) {
				command := strings.Join(args, " ")
				commands = append(commands, command)
				if strings.HasPrefix(command, "list table ") {
					if createdTables[command] {
						return []byte("table x y { chain " + nftOwnershipChain + " { counter comment \"" + nftTableOwnershipMarker + "\" } }"), nil
					}
					return nil, errors.New("No such file or directory")
				}
				if strings.HasPrefix(command, "add table ") {
					parts := strings.Fields(command)
					createdTables["list table "+parts[2]+" "+parts[3]] = true
					return nil, nil
				}
				if strings.HasPrefix(command, "list chain ") {
					return nil, errors.New("No such file or directory")
				}
				return nil, nil
			}

			if err := testCase.ensure(); err != nil {
				t.Fatalf("ensure port-forward NAT base chains failed: %v", err)
			}
			for _, expected := range testCase.expected {
				if !containsNftCommand(commands, expected) {
					t.Fatalf("missing numeric-priority command %q: %v", expected, commands)
				}
			}
			for _, command := range commands {
				if strings.Contains(command, "priority dstnat") || strings.Contains(command, "priority srcnat") {
					t.Fatalf("legacy-compatible NAT command must not use a symbolic priority: %s", command)
				}
			}
		})
	}
}

func TestRemoveNativePortForwardNatChainsStopsOnUnexpectedErrors(t *testing.T) {
	withNftCapabilitiesTestGlobals(t)

	commands := make([]string, 0)
	runNftFn = func(args ...string) ([]byte, error) {
		command := strings.Join(args, " ")
		commands = append(commands, command)
		switch command {
		case "list table inet " + portForwardNftTable:
			return []byte(`table inet kwor_forward { comment "kwor-owner:v1" }`), nil
		case "list chain inet " + portForwardNftTable + " " + portForwardPreroutingChain:
			return nil, errors.New("permission denied")
		case "list chain inet " + portForwardNftTable + " " + portForwardPostroutingChain:
			return nil, nil
		case "flush chain inet " + portForwardNftTable + " " + portForwardPostroutingChain:
			return nil, errors.New("flush rejected")
		default:
			return nil, nil
		}
	}

	if err := removeNativePortForwardNatChains(); err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("unexpected list failure must be returned, got %v", err)
	}
	if containsNftCommand(commands, "delete chain inet "+portForwardNftTable+" "+portForwardPostroutingChain) {
		t.Fatalf("must not delete a chain after its flush failed: %v", commands)
	}
}

func TestRemoveNativeNftNatChainsDoesNotDeleteAfterFlushFailure(t *testing.T) {
	withNftCapabilitiesTestGlobals(t)

	commands := make([]string, 0)
	runNftFn = func(args ...string) ([]byte, error) {
		command := strings.Join(args, " ")
		commands = append(commands, command)
		switch command {
		case "list table inet " + nftTable:
			return []byte(`table inet kwor { comment "kwor-owner:v1" }`), nil
		case "list chain inet " + nftTable + " " + nftChainPrerouting:
			return nil, errors.New("permission denied")
		case "list chain inet " + nftTable + " " + nftChainPostrouting:
			return nil, nil
		case "flush chain inet " + nftTable + " " + nftChainPostrouting:
			return nil, errors.New("flush rejected")
		default:
			return nil, nil
		}
	}

	if err := removeNativeNftNatChain(); err == nil || !strings.Contains(err.Error(), "permission denied") {
		t.Fatalf("unexpected native NAT cleanup error: %v", err)
	}
	if containsNftCommand(commands, "delete chain inet "+nftTable+" "+nftChainPostrouting) {
		t.Fatalf("must not delete a native chain after its flush failed: %v", commands)
	}
}

func TestRollbackManagedPortForwardRenderCleansEveryLayout(t *testing.T) {
	withNftCapabilitiesTestGlobals(t)
	nftSupportedFn = func() bool { return true }

	originalPlatform, originalPlatformErr := GetSystemPlatform()
	originalState := portForwardState
	t.Cleanup(func() {
		portForwardState = originalState
		if originalPlatformErr != nil || originalPlatform == nil {
			clearSystemPlatformSnapshot()
			return
		}
		setSystemPlatformSnapshot(originalPlatform)
	})
	setSystemPlatformSnapshot(&model.SystemPlatform{OS: "linux"})

	commands := make([]string, 0)
	runNftFn = func(args ...string) ([]byte, error) {
		command := strings.Join(args, " ")
		commands = append(commands, command)
		switch command {
		case "list table inet " + portForwardNftTable:
			return []byte(`table inet kwor_forward { comment "kwor-owner:v1" }`), nil
		case "list table ip " + portForwardNftTable:
			return []byte(`table ip kwor_forward { comment "kwor-owner:v1" }`), nil
		case "list table ip6 " + portForwardNftTable:
			return []byte(`table ip6 kwor_forward { comment "kwor-owner:v1" }`), nil
		}
		return nil, nil
	}
	portForwardState.lastRenderHash = "old-hash"
	portForwardState.lastLayout = "old-layout"
	portForwardState.warnings = []string{"old warning"}

	baseErr := errors.New("ip6 base rejected")
	if err := rollbackManagedPortForwardRender(baseErr); err != baseErr {
		t.Fatalf("rollback returned %v, want original base error", err)
	}
	for _, expected := range []string{
		"delete table inet " + portForwardNftTable,
		"delete table ip " + portForwardNftTable,
		"delete table ip6 " + portForwardNftTable,
	} {
		if !containsNftCommand(commands, expected) {
			t.Fatalf("missing render rollback command %q: %v", expected, commands)
		}
	}
	if portForwardState.lastRenderHash != "" || portForwardState.lastLayout != "" || len(portForwardState.warnings) != 0 {
		t.Fatalf("rollback must clear render state: %+v", portForwardState)
	}
}

func TestPortForwardRenderIntegrityRejectsManagedTableResidueAndDuplicateRules(t *testing.T) {
	withNftCapabilitiesTestGlobals(t)
	nftSupportedFn = func() bool { return true }
	setNftCapabilitiesForTest(buildNftablesCapabilities("nftables v0.9.8", "5.10.0", nil))

	residue := true
	runNftFn = func(args ...string) ([]byte, error) {
		command := strings.Join(args, " ")
		if command == "list table inet "+portForwardNftTable && residue {
			return nil, nil
		}
		return nil, errors.New("No such file or directory")
	}
	if portForwardRenderIntact(nil) {
		t.Fatal("zero active rules must reject a residual managed forwarding table")
	}
	residue = false
	if !portForwardRenderIntact(nil) {
		t.Fatal("zero active rules should be intact after every managed forwarding table is removed")
	}

	row := model.PortForwardRule{
		Id:            17,
		Family:        portForwardFamilyIPv4,
		Protocol:      portForwardProtocolTCP,
		LocalPortSpec: "8443",
		TargetIP:      portForwardLoopbackIPv4,
		TargetPort:    9443,
	}
	runNftFn = func(args ...string) ([]byte, error) {
		command := strings.Join(args, " ")
		switch {
		case command == "list table inet "+portForwardNftTable:
			return nil, nil
		case strings.HasPrefix(command, "list table ip ") || strings.HasPrefix(command, "list table ip6 "):
			return nil, errors.New("No such file or directory")
		case strings.HasPrefix(command, "list chain inet "+portForwardNftTable+" "):
			return nil, nil
		case strings.HasPrefix(command, "--handle --numeric list chain inet "+portForwardNftTable+" "+portForwardPreroutingChain):
			comment := portForwardRuleComment(row.Id, "dnat")
			return []byte("counter comment \"" + comment + "\" # handle 1\n" +
				"counter comment \"" + comment + "\" # handle 2\n"), nil
		case strings.HasPrefix(command, "--handle --numeric list chain "):
			return []byte{}, nil
		default:
			return nil, nil
		}
	}
	if portForwardRenderIntact([]model.PortForwardRule{row}) {
		t.Fatal("duplicate managed DNAT comment must fail strict forwarding integrity")
	}
}

func TestGetChainRuleBytesByHandleParsesJSONAndLegacyText(t *testing.T) {
	jsonOutput := []byte(`{"nftables":[{"rule":{"handle":12,"expr":[{"counter":{"packets":4,"bytes":2048}}]}}]}`)
	jsonBytes, jsonErr := getChainRuleBytesByHandleFromJSON(jsonOutput, nftChainIn, 12)
	if jsonErr != nil || jsonBytes != 2048 {
		t.Fatalf("json parser bytes=%d err=%v", jsonBytes, jsonErr)
	}

	textOutput := []byte(`table inet kwor {
  chain input {
    meta l4proto { tcp, udp } th dport 443 counter packets 7 bytes 4096 comment "kwor_inbound_demo_in" # handle 12
  }
}
`)
	textBytes, textErr := getChainRuleBytesByHandleFromText(textOutput, nftChainIn, 12)
	if textErr != nil || textBytes != 4096 {
		t.Fatalf("text parser bytes=%d err=%v", textBytes, textErr)
	}
}

func TestParseCompatibilityPortForwardCounterBytesAggregatesDualStackRules(t *testing.T) {
	output := []byte(strings.Join([]string{
		`meta nfproto ipv4 counter packets 2 bytes 120 comment "kwor_pf_rule_9_up"`,
		`meta nfproto ipv6 counter packets 3 bytes 180 comment "kwor_pf_rule_9_up"`,
		`meta nfproto ipv4 counter packets 1 bytes 70 comment "kwor_pf_rule_9_down"`,
		`meta nfproto ipv6 counter packets 4 bytes 230 comment "kwor_pf_rule_9_down"`,
	}, "\n"))
	result := parseCompatibilityPortForwardCounterBytes(output)
	if result[portForwardCounterName(9, "up")] != 300 {
		t.Fatalf("up bytes=%d", result[portForwardCounterName(9, "up")])
	}
	if result[portForwardCounterName(9, "down")] != 300 {
		t.Fatalf("down bytes=%d", result[portForwardCounterName(9, "down")])
	}
}

func TestCompatibilityRedirectRollsBackFirstFamilyWhenSecondFamilyFails(t *testing.T) {
	withNftCapabilitiesTestGlobals(t)
	nftSupportedFn = func() bool { return true }
	setNftCapabilitiesForTest(buildNftablesCapabilities("nftables v0.7", "4.9.0", nil))

	commands := make([]string, 0)
	runNftFn = func(args ...string) ([]byte, error) {
		command := strings.Join(args, " ")
		commands = append(commands, command)
		if strings.HasPrefix(command, "--handle --numeric list chain ip "+nftTable+" "+nftChainPrerouting) {
			return []byte(`udp dport 4000 redirect to :443 comment "kwor_inbound_demo_redirect" # handle 41`), nil
		}
		if strings.HasPrefix(command, "add rule ip6 "+nftTable+" "+nftChainPrerouting) {
			return nil, errors.New("ip6 rule rejected")
		}
		if strings.HasPrefix(command, "list ") || strings.HasPrefix(command, "--handle ") {
			return nil, errors.New("No such file or directory")
		}
		return nil, nil
	}

	if _, err := addRedirectRule("4000", 443, "kwor_inbound_demo_redirect"); err == nil {
		t.Fatal("expected the ip6 rule failure")
	}
	if !containsNftCommand(commands, "add rule ip "+nftTable+" "+nftChainPrerouting) {
		t.Fatalf("missing ipv4 redirect command: %v", commands)
	}
	if !containsNftCommand(commands, "add rule ip6 "+nftTable+" "+nftChainPrerouting) {
		t.Fatalf("missing ipv6 redirect command: %v", commands)
	}
	if !containsNftCommand(commands, "delete rule ip "+nftTable+" "+nftChainPrerouting+" handle 41") {
		t.Fatalf("missing ipv4 rollback command: %v", commands)
	}
}

func TestCompatibilityRedirectIntegrityAndCleanupCoverBothFamilies(t *testing.T) {
	withNftCapabilitiesTestGlobals(t)
	nftSupportedFn = func() bool { return true }
	setNftCapabilitiesForTest(buildNftablesCapabilities("nftables v0.7", "4.9.0", nil))

	comment := "kwor_inbound_demo_redirect"
	commands := make([]string, 0)
	includeIPv6 := false
	includeNative := true
	duplicateIPv4 := false
	runNftFn = func(args ...string) ([]byte, error) {
		command := strings.Join(args, " ")
		commands = append(commands, command)
		if strings.HasPrefix(command, "--handle --numeric list chain ip "+nftTable+" "+nftChainPrerouting) {
			if duplicateIPv4 {
				return []byte("udp dport 4000 redirect to :443 comment \"kwor_inbound_demo_redirect\" # handle 31\n" +
					"udp dport 4001 redirect to :443 comment \"kwor_inbound_demo_redirect\" # handle 33"), nil
			}
			return []byte(`udp dport 4000 redirect to :443 comment "kwor_inbound_demo_redirect" # handle 31`), nil
		}
		if strings.HasPrefix(command, "--handle --numeric list chain ip6 "+nftTable+" "+nftChainPrerouting) {
			if includeIPv6 {
				return []byte(`udp dport 4000 redirect to :443 comment "kwor_inbound_demo_redirect" # handle 32`), nil
			}
			return []byte(""), nil
		}
		if strings.HasPrefix(command, "--handle --numeric list chain inet "+nftTable+" "+nftChainPrerouting) {
			if includeNative {
				return []byte(`udp dport 4000 redirect to :443 comment "kwor_inbound_demo_redirect" # handle 30`), nil
			}
			return []byte(""), nil
		}
		return nil, nil
	}

	if nftRedirectRuleExistsByComment(comment) {
		t.Fatal("one-family redirect must not pass compatibility integrity")
	}
	includeIPv6 = true
	if nftRedirectRuleExistsByComment(comment) {
		t.Fatal("cross-layout redirect residue must not pass compatibility integrity")
	}
	includeNative = false
	if !nftRedirectRuleExistsByComment(comment) {
		t.Fatal("dual-stack redirect should pass compatibility integrity")
	}
	duplicateIPv4 = true
	if nftRedirectRuleExistsByComment(comment) {
		t.Fatal("duplicate compatibility redirect must not pass integrity")
	}
	duplicateIPv4 = false
	includeNative = true
	if err := deleteNftRedirectRulesByComment(comment); err != nil {
		t.Fatalf("cleanup failed: %v", err)
	}
	for _, expected := range []string{
		"delete rule inet " + nftTable + " " + nftChainPrerouting + " handle 30",
		"delete rule ip " + nftTable + " " + nftChainPrerouting + " handle 31",
		"delete rule ip6 " + nftTable + " " + nftChainPrerouting + " handle 32",
	} {
		if !containsNftCommand(commands, expected) {
			t.Fatalf("missing cleanup command %q: %v", expected, commands)
		}
	}
}

func TestCompatibilityPortForwardUsesDualNatTablesAndInlineCounters(t *testing.T) {
	withNftCapabilitiesTestGlobals(t)
	setNftCapabilitiesForTest(buildNftablesCapabilities("nftables v0.7", "4.9.0", nil))

	commands := make([]string, 0)
	runNftFn = func(args ...string) ([]byte, error) {
		commands = append(commands, strings.Join(args, " "))
		return nil, nil
	}

	row := model.PortForwardRule{
		Id:            7,
		Name:          "compat",
		Family:        portForwardFamilyDual,
		Protocol:      portForwardProtocolTCPUDP,
		LocalPortSpec: "8443",
		TargetIP:      portForwardLoopbackIPv4,
		TargetPort:    9443,
		RateLimitMbps: 20,
	}
	if _, err := addCompatibilityManagedPortForwardRule(row); err != nil {
		t.Fatalf("add compatibility rule failed: %v", err)
	}
	if !containsNftCommand(commands, "add rule ip "+portForwardNftTable+" "+portForwardPreroutingChain) {
		t.Fatalf("missing ipv4 NAT rule: %v", commands)
	}
	if !containsNftCommand(commands, "add rule ip6 "+portForwardNftTable+" "+portForwardPreroutingChain) {
		t.Fatalf("missing ipv6 NAT rule: %v", commands)
	}
	if !containsNftCommand(commands, "add rule inet "+portForwardNftTable+" "+portForwardInputChain) {
		t.Fatalf("missing inet filter rule: %v", commands)
	}
	for _, command := range commands {
		if strings.Contains(command, "counter name") || strings.Contains(command, " meter ") || strings.Contains(command, "th dport") {
			t.Fatalf("legacy compatibility command contains unsupported grammar: %s", command)
		}
	}
	if !containsNftCommand(commands, "add rule ip "+portForwardNftTable+" "+portForwardPreroutingChain+" meta l4proto") {
		t.Fatalf("legacy compatibility rules were not emitted: %v", commands)
	}
}

func TestNftRendererUsesVersionSpecificOwnershipAndTransportGrammar(t *testing.T) {
	testCases := []struct {
		name                 string
		profile              NftablesCapabilities
		wantOwnership        string
		wantTransport        string
		forbiddenOwnership   string
		forbiddenTransport   string
		wantCompatibilityNAT bool
	}{
		{
			name:                 "debian 9 legacy grammar",
			profile:              buildNftablesCapabilities("nftables v0.7", "4.9.0", nil),
			wantOwnership:        "add chain inet " + firewallNftTable + " " + nftOwnershipChain,
			wantTransport:        "@th,16,16 443",
			forbiddenOwnership:   "add table inet " + firewallNftTable + " { comment",
			forbiddenTransport:   "th dport",
			wantCompatibilityNAT: true,
		},
		{
			name:               "debian 11 modern grammar",
			profile:            buildNftablesCapabilities("nftables v0.9.8", "5.10.0", nil),
			wantOwnership:      "add table inet " + firewallNftTable + " { comment",
			wantTransport:      "th dport 443",
			forbiddenTransport: "@th,16,16",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			withNftCapabilitiesTestGlobals(t)
			setNftCapabilitiesForTest(testCase.profile)

			firewallScript, err := buildManagedFirewallScript([]model.FirewallRule{{
				Id:       31,
				Enabled:  true,
				Origin:   firewallOriginManual,
				Family:   firewallFamilyIPv4,
				Protocol: firewallProtocolTCP,
				PortSpec: "443",
			}}, nil, false)
			if err != nil {
				t.Fatalf("build firewall script: %v", err)
			}
			if !strings.Contains(firewallScript, testCase.wantOwnership) || !strings.Contains(firewallScript, testCase.wantTransport) {
				t.Fatalf("missing version-specific firewall grammar:\n%s", firewallScript)
			}
			if !strings.Contains(firewallScript, "add chain inet "+firewallNftTable+" "+nftOwnershipChain) {
				t.Fatalf("missing downgrade-safe owner-chain marker:\n%s", firewallScript)
			}
			if (testCase.forbiddenOwnership != "" && strings.Contains(firewallScript, testCase.forbiddenOwnership)) ||
				strings.Contains(firewallScript, testCase.forbiddenTransport) {
				t.Fatalf("firewall script contains unsupported grammar:\n%s", firewallScript)
			}

			row := model.PortForwardRule{
				Id:            32,
				Name:          "grammar",
				Enabled:       true,
				Family:        portForwardFamilyIPv4,
				Protocol:      portForwardProtocolTCP,
				LocalPortSpec: "8443",
				TargetIP:      portForwardLoopbackIPv4,
				TargetPort:    9443,
				RateLimitMbps: 10,
			}
			_, commands := buildManagedPortForwardRuleCommands(row)
			lines := make([]string, 0, len(commands))
			for _, command := range commands {
				lines = append(lines, portForwardNftScriptLine(command))
			}
			forwardScript := strings.Join(lines, "\n")
			if !strings.Contains(forwardScript, strings.Replace(testCase.wantTransport, "443", "8443", 1)) {
				t.Fatalf("missing version-specific forwarding grammar:\n%s", forwardScript)
			}
			if strings.Contains(forwardScript, testCase.forbiddenTransport) {
				t.Fatalf("forwarding script contains unsupported transport grammar:\n%s", forwardScript)
			}
			if testCase.wantCompatibilityNAT {
				if strings.Contains(forwardScript, "counter name") || strings.Contains(forwardScript, " meter ") {
					t.Fatalf("legacy forwarding script contains unsupported stateful objects:\n%s", forwardScript)
				}
			}
		})
	}
}

func TestNftOwnershipMarkerAcceptsOnlyTableCommentOrDedicatedOwnerRule(t *testing.T) {
	legacy := []byte(`table inet kwor {
  chain kwor_owner {
    counter packets 0 bytes 0 comment "kwor-owner:v1"
  }
}`)
	if !nftTableHasOwnershipMarker(legacy) {
		t.Fatal("dedicated legacy owner rule was not recognized")
	}
	wrongChain := []byte(`table inet kwor {
  chain input {
    counter packets 0 bytes 0 comment "kwor-owner:v1"
  }
}`)
	if nftTableHasOwnershipMarker(wrongChain) {
		t.Fatal("marker rule outside the dedicated owner chain must not confer table ownership")
	}
	ownerChainComment := []byte(`table inet kwor {
  chain kwor_owner {
    comment "kwor-owner:v1"
  }
}`)
	if nftTableHasOwnershipMarker(ownerChainComment) {
		t.Fatal("an owner-chain comment without the marker counter rule must not confer ownership")
	}
}

func containsNftCommand(commands []string, prefix string) bool {
	for _, command := range commands {
		if strings.HasPrefix(command, prefix) {
			return true
		}
	}
	return false
}
