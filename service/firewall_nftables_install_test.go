package service

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func withFirewallNftablesTestGlobals(t *testing.T) {
	t.Helper()

	originalFirewallRuntimeGOOS := firewallRuntimeGOOS
	originalFirewallCommandLookPath := firewallCommandLookPath
	originalFirewallReadFile := firewallReadFile
	originalFirewallGeteuid := firewallGeteuid
	originalFirewallRunInstall := firewallRunInstall
	originalFirewallSupportedFn := firewallSupportedFn
	originalFirewallState := firewallState
	originalNftRuntimeGOOS := nftRuntimeGOOS
	originalNftLookPathFn := nftLookPathFn
	originalNftStatFn := nftStatFn
	originalNftCandidates := append([]string{}, nftCandidates...)
	originalInstallFailure := getFirewallNftInstallFailure()

	t.Cleanup(func() {
		firewallRuntimeGOOS = originalFirewallRuntimeGOOS
		firewallCommandLookPath = originalFirewallCommandLookPath
		firewallReadFile = originalFirewallReadFile
		firewallGeteuid = originalFirewallGeteuid
		firewallRunInstall = originalFirewallRunInstall
		firewallSupportedFn = originalFirewallSupportedFn
		firewallState = originalFirewallState
		nftRuntimeGOOS = originalNftRuntimeGOOS
		nftLookPathFn = originalNftLookPathFn
		nftStatFn = originalNftStatFn
		nftCandidates = originalNftCandidates
		setFirewallNftInstallFailure(originalInstallFailure)
	})

	clearFirewallNftInstallFailure()
	firewallState.lastRenderHash = ""
	firewallState.lastRuntimeHash = ""
	firewallState.lastReconcile = time.Time{}
}

func firewallTestLookPath(paths map[string]string) func(string) (string, error) {
	return func(name string) (string, error) {
		if path, ok := paths[name]; ok {
			return path, nil
		}
		return "", exec.ErrNotFound
	}
}

func firewallTestLinuxOsRelease(id string, idLike string) []byte {
	return firewallTestLinuxOsReleaseVersion(id, idLike, "", "")
}

func firewallTestLinuxOsReleaseVersion(id string, idLike string, version string, codename string) []byte {
	lines := []string{}
	if strings.TrimSpace(id) != "" {
		lines = append(lines, "ID="+id)
	}
	if strings.TrimSpace(idLike) != "" {
		lines = append(lines, `ID_LIKE="`+idLike+`"`)
	}
	if strings.TrimSpace(version) != "" {
		lines = append(lines, `VERSION_ID="`+version+`"`)
	}
	if strings.TrimSpace(codename) != "" {
		lines = append(lines, "VERSION_CODENAME="+codename)
	}
	return []byte(strings.Join(lines, "\n"))
}

func TestResolveNftBinaryPathFallsBackToKnownLocations(t *testing.T) {
	withFirewallNftablesTestGlobals(t)

	tempFile, err := os.CreateTemp(t.TempDir(), "nft-*")
	if err != nil {
		t.Fatalf("create temp nft binary failed: %v", err)
	}
	if err := tempFile.Close(); err != nil {
		t.Fatalf("close temp nft binary failed: %v", err)
	}

	nftRuntimeGOOS = "linux"
	nftLookPathFn = func(string) (string, error) {
		return "", exec.ErrNotFound
	}
	nftCandidates = []string{tempFile.Name()}
	nftStatFn = os.Stat

	path, err := resolveNftBinaryPath()
	if err != nil {
		t.Fatalf("resolveNftBinaryPath returned error: %v", err)
	}
	if path != tempFile.Name() {
		t.Fatalf("resolved path mismatch: got %q want %q", path, tempFile.Name())
	}
}

func TestFirewallNftablesOverviewStates(t *testing.T) {
	openFirewallSystemRuleTestDB(t)

	withFirewallNftablesTestGlobals(t)
	firewallReadFile = func(string) ([]byte, error) {
		return firewallTestLinuxOsReleaseVersion("debian", "debian", "10", "buster"), nil
	}
	firewallCommandLookPath = firewallTestLookPath(map[string]string{
		"apt-get": "/usr/bin/apt-get",
	})
	nftStatFn = func(string) (os.FileInfo, error) {
		return nil, os.ErrNotExist
	}

	testCases := []struct {
		name              string
		setup             func()
		wantAvailable     bool
		wantReason        string
		wantInstalled     bool
		wantAutoInstall   bool
		wantBinaryPath    string
		wantErrorContains string
	}{
		{
			name: "not linux",
			setup: func() {
				firewallRuntimeGOOS = "windows"
				nftRuntimeGOOS = "windows"
				firewallSupportedFn = func() bool { return false }
				nftLookPathFn = func(string) (string, error) { return "", exec.ErrNotFound }
			},
			wantAvailable:     false,
			wantReason:        firewallNftReasonNotLinux,
			wantInstalled:     false,
			wantAutoInstall:   false,
			wantErrorContains: "Linux only",
		},
		{
			name: "linux missing binary",
			setup: func() {
				firewallRuntimeGOOS = "linux"
				nftRuntimeGOOS = "linux"
				firewallSupportedFn = func() bool { return false }
				firewallGeteuid = func() int { return 0 }
				nftLookPathFn = func(string) (string, error) { return "", exec.ErrNotFound }
			},
			wantAvailable:     false,
			wantReason:        firewallNftReasonMissingBinary,
			wantInstalled:     false,
			wantAutoInstall:   true,
			wantErrorContains: "archive.debian.org",
		},
		{
			name: "linux installed but permission denied",
			setup: func() {
				firewallRuntimeGOOS = "linux"
				nftRuntimeGOOS = "linux"
				firewallSupportedFn = func() bool { return false }
				firewallGeteuid = func() int { return 0 }
				nftLookPathFn = func(name string) (string, error) {
					if name == "nft" {
						return "/usr/sbin/nft", nil
					}
					return "", exec.ErrNotFound
				}
			},
			wantAvailable:     false,
			wantReason:        firewallNftReasonPermissionDenied,
			wantInstalled:     true,
			wantAutoInstall:   true,
			wantBinaryPath:    "/usr/sbin/nft",
			wantErrorContains: "lacks permission to execute nft",
		},
		{
			name: "linux installed and available",
			setup: func() {
				firewallRuntimeGOOS = "linux"
				nftRuntimeGOOS = "linux"
				firewallSupportedFn = func() bool { return true }
				firewallGeteuid = func() int { return 0 }
				nftLookPathFn = func(name string) (string, error) {
					if name == "nft" {
						return "/usr/sbin/nft", nil
					}
					return "", exec.ErrNotFound
				}
			},
			wantAvailable:   true,
			wantReason:      firewallNftReasonReady,
			wantInstalled:   true,
			wantAutoInstall: true,
			wantBinaryPath:  "/usr/sbin/nft",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			withFirewallNftablesTestGlobals(t)
			openFirewallSystemRuleTestDB(t)

			firewallReadFile = func(string) ([]byte, error) {
				return firewallTestLinuxOsReleaseVersion("debian", "debian", "10", "buster"), nil
			}
			firewallCommandLookPath = firewallTestLookPath(map[string]string{
				"apt-get": "/usr/bin/apt-get",
			})
			nftStatFn = func(string) (os.FileInfo, error) {
				return nil, os.ErrNotExist
			}

			testCase.setup()

			overview, err := (&FirewallService{}).GetOverview()
			if err != nil {
				t.Fatalf("GetOverview returned error: %v", err)
			}
			if overview.Available != testCase.wantAvailable {
				t.Fatalf("overview.Available mismatch: got=%v want=%v", overview.Available, testCase.wantAvailable)
			}
			if overview.Nftables.Reason != testCase.wantReason {
				t.Fatalf("nft reason mismatch: got=%q want=%q", overview.Nftables.Reason, testCase.wantReason)
			}
			if overview.Nftables.Installed != testCase.wantInstalled {
				t.Fatalf("nft installed mismatch: got=%v want=%v", overview.Nftables.Installed, testCase.wantInstalled)
			}
			if overview.Nftables.AutoInstallSupported != testCase.wantAutoInstall {
				t.Fatalf("nft autoInstallSupported mismatch: got=%v want=%v", overview.Nftables.AutoInstallSupported, testCase.wantAutoInstall)
			}
			if overview.Nftables.BinaryPath != testCase.wantBinaryPath {
				t.Fatalf("binary path mismatch: got=%q want=%q", overview.Nftables.BinaryPath, testCase.wantBinaryPath)
			}
			if testCase.wantErrorContains == "" {
				if overview.Error != "" {
					t.Fatalf("expected empty overview error, got %q", overview.Error)
				}
			} else if !strings.Contains(overview.Error, testCase.wantErrorContains) {
				t.Fatalf("overview error mismatch: got=%q want substring=%q", overview.Error, testCase.wantErrorContains)
			}
		})
	}
}

func TestDetectFirewallNftInstallPlanSelectsExpectedCommands(t *testing.T) {
	withFirewallNftablesTestGlobals(t)

	testCases := []struct {
		name                string
		fields              map[string]string
		paths               map[string]string
		wantName            string
		wantSystemFamily    string
		wantImmutable       bool
		wantFirstCommand    string
		wantFirstManualStep string
	}{
		{
			name:             "apt-get",
			fields:           map[string]string{"ID": "debian", "ID_LIKE": "debian", "VERSION_ID": "10", "VERSION_CODENAME": "buster"},
			paths:            map[string]string{"apt-get": "/usr/bin/apt-get"},
			wantName:         "apt-get",
			wantSystemFamily: "debian",
			wantFirstCommand: "archive.debian.org",
		},
		{
			name:             "dnf",
			fields:           map[string]string{"ID": "fedora", "ID_LIKE": "fedora"},
			paths:            map[string]string{"dnf": "/usr/bin/dnf"},
			wantName:         "dnf",
			wantSystemFamily: "rhel",
			wantFirstCommand: "dnf install -y nftables",
		},
		{
			name:             "yum",
			fields:           map[string]string{"ID": "centos", "ID_LIKE": "rhel fedora"},
			paths:            map[string]string{"yum": "/usr/bin/yum"},
			wantName:         "yum",
			wantSystemFamily: "rhel",
			wantFirstCommand: "yum install -y nftables",
		},
		{
			name:             "zypper",
			fields:           map[string]string{"ID": "opensuse-leap", "ID_LIKE": "suse opensuse"},
			paths:            map[string]string{"zypper": "/usr/bin/zypper"},
			wantName:         "zypper",
			wantSystemFamily: "suse",
			wantFirstCommand: "zypper --non-interactive install nftables",
		},
		{
			name:             "pacman",
			fields:           map[string]string{"ID": "arch", "ID_LIKE": "archlinux"},
			paths:            map[string]string{"pacman": "/usr/bin/pacman"},
			wantName:         "pacman",
			wantSystemFamily: "arch",
			wantFirstCommand: "pacman -S --needed --noconfirm nftables",
		},
		{
			name:             "apk",
			fields:           map[string]string{"ID": "alpine"},
			paths:            map[string]string{"apk": "/sbin/apk"},
			wantName:         "apk",
			wantSystemFamily: "alpine",
			wantFirstCommand: "apk add --no-cache nftables",
		},
		{
			name:                "rpm-ostree",
			fields:              map[string]string{"ID": "fedora", "ID_LIKE": "fedora"},
			paths:               map[string]string{"rpm-ostree": "/usr/bin/rpm-ostree"},
			wantName:            "rpm-ostree",
			wantSystemFamily:    "rhel",
			wantImmutable:       true,
			wantFirstManualStep: "rpm-ostree install nftables",
		},
		{
			name:                "transactional-update",
			fields:              map[string]string{"ID": "opensuse-microos", "ID_LIKE": "suse opensuse"},
			paths:               map[string]string{"transactional-update": "/usr/sbin/transactional-update"},
			wantName:            "transactional-update",
			wantSystemFamily:    "suse",
			wantImmutable:       true,
			wantFirstManualStep: "transactional-update pkg install nftables",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			firewallCommandLookPath = firewallTestLookPath(testCase.paths)

			plan := detectFirewallNftInstallPlan(testCase.fields)
			if plan == nil {
				t.Fatal("expected install plan, got nil")
			}
			if plan.Name != testCase.wantName {
				t.Fatalf("plan name mismatch: got=%q want=%q", plan.Name, testCase.wantName)
			}
			if plan.SystemFamily != testCase.wantSystemFamily {
				t.Fatalf("plan system family mismatch: got=%q want=%q", plan.SystemFamily, testCase.wantSystemFamily)
			}
			if plan.Immutable != testCase.wantImmutable {
				t.Fatalf("plan immutable mismatch: got=%v want=%v", plan.Immutable, testCase.wantImmutable)
			}
			if testCase.wantFirstCommand != "" {
				if len(plan.InstallPlan) == 0 {
					t.Fatalf("expected install commands for %s", testCase.wantName)
				}
				if got := strings.Join(plan.InstallPlan[0], " "); !strings.Contains(got, testCase.wantFirstCommand) {
					t.Fatalf("first command mismatch: got=%q want substring=%q", got, testCase.wantFirstCommand)
				}
			}
			if testCase.wantFirstManualStep != "" {
				if len(plan.ManualCommands) == 0 {
					t.Fatalf("expected manual commands for %s", testCase.wantName)
				}
				if plan.ManualCommands[0] != testCase.wantFirstManualStep {
					t.Fatalf("first manual command mismatch: got=%q want=%q", plan.ManualCommands[0], testCase.wantFirstManualStep)
				}
			}
		})
	}
}

func TestBuildDebianUbuntuFirewallNftInstallPlanUsesOfficialSourcesAndServiceStart(t *testing.T) {
	withFirewallNftablesTestGlobals(t)

	testCases := []struct {
		name                string
		fields              map[string]string
		manager             string
		wantRewriteContains []string
		wantUpdateCommand   string
		wantInstallCommand  string
		wantManualUpdate    string
		wantManualInstall   string
		wantServiceContains string
		wantSourceListPath  string
	}{
		{
			name: "debian 10 archive sources",
			fields: map[string]string{
				"ID":               "debian",
				"ID_LIKE":          "debian",
				"VERSION_ID":       "10",
				"VERSION_CODENAME": "buster",
			},
			manager:             "apt-get",
			wantRewriteContains: []string{"archive.debian.org/debian", "archive.debian.org/debian-security", "buster"},
			wantUpdateCommand:   "apt-get -o Acquire::Check-Valid-Until=false update",
			wantInstallCommand:  "apt-get install -y nftables",
			wantManualUpdate:    "apt-get -o Acquire::Check-Valid-Until=false update",
			wantManualInstall:   "apt-get install -y nftables",
			wantServiceContains: "systemctl enable --now nftables",
			wantSourceListPath:  "/etc/apt/sources.list",
		},
		{
			name: "debian 12 primary and security sources",
			fields: map[string]string{
				"ID":               "debian",
				"ID_LIKE":          "debian",
				"VERSION_ID":       "12",
				"VERSION_CODENAME": "bookworm",
			},
			manager:             "apt-get",
			wantRewriteContains: []string{"deb.debian.org/debian", "security.debian.org/debian-security", "bookworm-security", "non-free-firmware"},
			wantUpdateCommand:   "apt-get update",
			wantInstallCommand:  "apt-get install -y nftables",
			wantManualUpdate:    "apt-get update",
			wantManualInstall:   "apt-get install -y nftables",
			wantServiceContains: "systemctl enable --now nftables",
			wantSourceListPath:  "/etc/apt/sources.list",
		},
		{
			name: "ubuntu 18.04 official archive and security",
			fields: map[string]string{
				"ID":               "ubuntu",
				"ID_LIKE":          "debian",
				"VERSION_ID":       "18.04",
				"VERSION_CODENAME": "bionic",
			},
			manager:             "apt-get",
			wantRewriteContains: []string{"archive.ubuntu.com/ubuntu", "security.ubuntu.com/ubuntu", "bionic-security", "multiverse"},
			wantUpdateCommand:   "apt-get update",
			wantInstallCommand:  "apt-get install -y nftables",
			wantManualUpdate:    "apt-get update",
			wantManualInstall:   "apt-get install -y nftables",
			wantServiceContains: "systemctl enable --now nftables",
			wantSourceListPath:  "/etc/apt/sources.list",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			plan := buildDebianUbuntuFirewallNftInstallPlan(testCase.fields, testCase.manager)
			if plan == nil {
				t.Fatal("expected install plan, got nil")
			}
			if plan.SourceListPath != testCase.wantSourceListPath {
				t.Fatalf("source list path mismatch: got=%q want=%q", plan.SourceListPath, testCase.wantSourceListPath)
			}
			if len(plan.InstallPlan) != 3 {
				t.Fatalf("unexpected install plan length: %d", len(plan.InstallPlan))
			}
			rewrite := strings.Join(plan.InstallPlan[0], " ")
			for _, want := range testCase.wantRewriteContains {
				if !strings.Contains(rewrite, want) {
					t.Fatalf("rewrite command mismatch: got=%q want substring=%q", rewrite, want)
				}
			}
			if got := strings.Join(plan.InstallPlan[1], " "); got != testCase.wantUpdateCommand {
				t.Fatalf("update command mismatch: got=%q want=%q", got, testCase.wantUpdateCommand)
			}
			if got := strings.Join(plan.InstallPlan[2], " "); got != testCase.wantInstallCommand {
				t.Fatalf("install command mismatch: got=%q want=%q", got, testCase.wantInstallCommand)
			}
			if len(plan.PostInstallPlan) != 1 {
				t.Fatalf("unexpected post-install plan length: %d", len(plan.PostInstallPlan))
			}
			if got := strings.Join(plan.PostInstallPlan[0], " "); !strings.Contains(got, testCase.wantServiceContains) {
				t.Fatalf("post-install command mismatch: got=%q want substring=%q", got, testCase.wantServiceContains)
			}
			if len(plan.ManualCommands) != 4 {
				t.Fatalf("unexpected manual command count: %d", len(plan.ManualCommands))
			}
			if got := plan.ManualCommands[0]; !strings.Contains(got, testCase.wantRewriteContains[0]) {
				t.Fatalf("manual rewrite command mismatch: got=%q want substring=%q", got, testCase.wantRewriteContains[0])
			}
			if plan.ManualCommands[1] != testCase.wantManualUpdate {
				t.Fatalf("manual update command mismatch: got=%q want=%q", plan.ManualCommands[1], testCase.wantManualUpdate)
			}
			if plan.ManualCommands[2] != testCase.wantManualInstall {
				t.Fatalf("manual install command mismatch: got=%q want=%q", plan.ManualCommands[2], testCase.wantManualInstall)
			}
			if got := plan.ManualCommands[3]; !strings.Contains(got, testCase.wantServiceContains) {
				t.Fatalf("manual service command mismatch: got=%q want substring=%q", got, testCase.wantServiceContains)
			}
		})
	}
}

func TestFirewallNftablesPrivilegeStrategies(t *testing.T) {
	plan := &firewallNftInstallPlan{
		Name: "apt-get",
		InstallPlan: [][]string{
			{"apt-get", "update"},
			{"apt-get", "install", "-y", "nftables"},
		},
	}

	t.Run("root direct install", func(t *testing.T) {
		privilege := firewallPrivilegeContext{IsRoot: true}
		commands := buildFirewallAutomaticInstallCommands(plan, privilege)
		if len(commands) != 2 {
			t.Fatalf("unexpected command count: %d", len(commands))
		}
		if got := strings.Join(commands[0], " "); got != "apt-get update" {
			t.Fatalf("root install command mismatch: %q", got)
		}
	})

	t.Run("sudo non interactive prefix", func(t *testing.T) {
		privilege := firewallPrivilegeContext{SudoPath: "/usr/bin/sudo"}
		commands := buildFirewallAutomaticInstallCommands(plan, privilege)
		if len(commands) != 2 {
			t.Fatalf("unexpected command count: %d", len(commands))
		}
		if got := strings.Join(commands[0], " "); got != "sudo -n apt-get update" {
			t.Fatalf("sudo install command mismatch: %q", got)
		}
		manual := buildFirewallManualCommands(plan, privilege)
		if len(manual) != 2 || manual[0] != "sudo apt-get update" {
			t.Fatalf("sudo manual commands mismatch: %v", manual)
		}
	})

	t.Run("no sudo returns raw manual commands", func(t *testing.T) {
		privilege := firewallPrivilegeContext{}
		if privilege.canAutoInstall() {
			t.Fatal("expected auto install to be disabled without root or sudo")
		}
		manual := buildFirewallManualCommands(plan, privilege)
		if len(manual) != 2 {
			t.Fatalf("unexpected manual command count: %d", len(manual))
		}
		if manual[0] != "apt-get update" || manual[1] != "apt-get install -y nftables" {
			t.Fatalf("manual commands mismatch: %v", manual)
		}
	})
}

func TestInstallNftablesReturnsRefreshedOverviewAfterSuccess(t *testing.T) {
	openFirewallSystemRuleTestDB(t)
	withFirewallNftablesTestGlobals(t)

	firewallRuntimeGOOS = "linux"
	nftRuntimeGOOS = "linux"
	firewallReadFile = func(string) ([]byte, error) {
		return firewallTestLinuxOsReleaseVersion("debian", "debian", "10", "buster"), nil
	}
	firewallCommandLookPath = firewallTestLookPath(map[string]string{
		"apt-get": "/usr/bin/apt-get",
	})
	firewallGeteuid = func() int { return 0 }

	installed := false
	nftLookPathFn = func(name string) (string, error) {
		if name == "nft" && installed {
			return "/usr/sbin/nft", nil
		}
		return "", exec.ErrNotFound
	}
	nftStatFn = func(string) (os.FileInfo, error) {
		return nil, os.ErrNotExist
	}
	firewallSupportedFn = func() bool {
		return installed
	}

	executed := make([]string, 0, 4)
	firewallRunInstall = func(command []string) error {
		executed = append(executed, strings.Join(command, " "))
		if len(command) >= 2 && command[0] == "apt-get" && command[1] == "install" {
			installed = true
		}
		return nil
	}

	if err := (&SettingService{}).setString(firewallEnabledKey, "true"); err != nil {
		t.Fatalf("set firewallEnabled setting failed: %v", err)
	}
	firewallState.lastReconcile = time.Now()

	overview, err := (&FirewallService{}).InstallNftables()
	if err != nil {
		t.Fatalf("InstallNftables returned error: %v", err)
	}
	if len(executed) != 4 {
		t.Fatalf("unexpected install command count: %d (%v)", len(executed), executed)
	}
	if !strings.Contains(executed[0], "archive.debian.org/debian") || !strings.Contains(executed[0], "/etc/apt/sources.list") {
		t.Fatalf("first install command mismatch: %q", executed[0])
	}
	if executed[1] != "apt-get -o Acquire::Check-Valid-Until=false update" {
		t.Fatalf("second install command mismatch: %q", executed[1])
	}
	if executed[2] != "apt-get install -y nftables" {
		t.Fatalf("third install command mismatch: %q", executed[2])
	}
	if !strings.Contains(executed[3], "systemctl enable --now nftables") {
		t.Fatalf("fourth install command mismatch: %q", executed[3])
	}
	if !overview.Nftables.Installed {
		t.Fatal("expected nftables to be marked installed after install")
	}
	if !overview.Available {
		t.Fatal("expected overview to be available after install")
	}
	if overview.Nftables.Reason != firewallNftReasonReady {
		t.Fatalf("unexpected nft status after install: %q", overview.Nftables.Reason)
	}
	if overview.Error != "" {
		t.Fatalf("expected empty overview error after install, got %q", overview.Error)
	}
}

func TestInstallNftablesWithoutSudoReturnsManualCommands(t *testing.T) {
	withFirewallNftablesTestGlobals(t)

	firewallRuntimeGOOS = "linux"
	nftRuntimeGOOS = "linux"
	firewallReadFile = func(string) ([]byte, error) {
		return firewallTestLinuxOsReleaseVersion("debian", "debian", "10", "buster"), nil
	}
	firewallCommandLookPath = firewallTestLookPath(map[string]string{
		"apt-get": "/usr/bin/apt-get",
	})
	firewallGeteuid = func() int { return 1000 }
	firewallSupportedFn = func() bool { return false }
	nftLookPathFn = func(string) (string, error) { return "", exec.ErrNotFound }
	nftStatFn = func(string) (os.FileInfo, error) {
		return nil, os.ErrNotExist
	}
	firewallRunInstall = func([]string) error {
		return errors.New("install command should not be called without sudo")
	}

	_, err := (&FirewallService{}).InstallNftables()
	if err == nil {
		t.Fatal("expected InstallNftables to fail without root or sudo")
	}
	message := err.Error()
	if !strings.Contains(message, "requires root or passwordless sudo") {
		t.Fatalf("unexpected permission error: %q", message)
	}
	if !strings.Contains(message, "archive.debian.org/debian") {
		t.Fatalf("manual rewrite command missing from error: %q", message)
	}
	if !strings.Contains(message, "apt-get -o Acquire::Check-Valid-Until=false update") {
		t.Fatalf("manual update command missing from error: %q", message)
	}
	if !strings.Contains(message, "apt-get install -y nftables") {
		t.Fatalf("manual commands missing from error: %q", message)
	}
	if !strings.Contains(message, "systemctl enable --now nftables") {
		t.Fatalf("manual service command missing from error: %q", message)
	}
}
