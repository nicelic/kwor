package service

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alireza0/s-ui/util/common"
)

const (
	firewallNftReasonNotLinux           = "not_linux"
	firewallNftReasonMissingBinary      = "missing_binary"
	firewallNftReasonPermissionDenied   = "permission_denied"
	firewallNftReasonUnsupportedManager = "unsupported_manager"
	firewallNftReasonImmutableOS        = "immutable_os"
	firewallNftReasonInstallFailed      = "install_failed"
	firewallNftReasonReady              = "ready"
)

type FirewallNftablesStatus struct {
	Supported            bool     `json:"supported"`
	Installed            bool     `json:"installed"`
	AutoInstallSupported bool     `json:"autoInstallSupported"`
	BinaryPath           string   `json:"binaryPath,omitempty"`
	SystemFamily         string   `json:"systemFamily,omitempty"`
	DistributionID       string   `json:"distributionId,omitempty"`
	VersionID            string   `json:"versionId,omitempty"`
	Codename             string   `json:"codename,omitempty"`
	PackageManager       string   `json:"packageManager,omitempty"`
	ManualCommands       []string `json:"manualCommands"`
	Reason               string   `json:"reason"`
}

type firewallNftInstallPlan struct {
	Name            string
	SystemFamily    string
	DistributionID  string
	VersionID       string
	Codename        string
	InstallPlan     [][]string
	PostInstallPlan [][]string
	ManualCommands  []string
	Immutable       bool
}

type firewallPrivilegeContext struct {
	IsRoot   bool
	SudoPath string
}

type firewallLinuxDistribution struct {
	ID       string
	IDLike   string
	Version  string
	Codename string
	Major    int
	Minor    int
}

var (
	firewallRuntimeGOOS      = runtime.GOOS
	firewallCommandLookPath  = exec.LookPath
	firewallReadFile         = os.ReadFile
	firewallGeteuid          = os.Geteuid
	firewallRunInstall       = runFirewallNftInstallCommand
	firewallOSReleasePaths   = []string{"/etc/os-release", "/usr/lib/os-release"}
	firewallNftInstallStateM sync.Mutex
	firewallNftInstallState  = struct {
		lastFailure string
	}{}
)

var (
	firewallDebianCodenameByMajor = map[int]string{
		9:  "stretch",
		10: "buster",
		11: "bullseye",
		12: "bookworm",
		13: "trixie",
		14: "forky",
		15: "duke",
	}
	firewallUbuntuCodenameByVersion = map[string]string{
		"18.04": "bionic",
		"18.10": "cosmic",
		"19.04": "disco",
		"19.10": "eoan",
		"20.04": "focal",
		"20.10": "groovy",
		"21.04": "hirsute",
		"21.10": "impish",
		"22.04": "jammy",
		"22.10": "kinetic",
		"23.04": "lunar",
		"23.10": "mantic",
		"24.04": "noble",
		"24.10": "oracular",
		"25.04": "plucky",
		"25.10": "questing",
		"26.04": "resolute",
		"26.10": "stonking",
	}
)

func (ctx firewallPrivilegeContext) canAutoInstall() bool {
	return ctx.IsRoot || strings.TrimSpace(ctx.SudoPath) != ""
}

func readFirewallOsReleaseFields() map[string]string {
	for _, path := range firewallOSReleasePaths {
		content, err := firewallReadFile(path)
		if err != nil {
			continue
		}
		return parseOsReleaseFields(string(content))
	}
	return map[string]string{}
}

func parseFirewallLinuxDistribution(fields map[string]string) firewallLinuxDistribution {
	id := normalizeLinuxReleaseToken(fields["ID"])
	version := strings.TrimSpace(strings.Trim(fields["VERSION_ID"], `"'`))
	codename := firstNonEmpty(
		normalizeLinuxReleaseToken(fields["VERSION_CODENAME"]),
		normalizeLinuxReleaseToken(fields["UBUNTU_CODENAME"]),
	)
	major, minor := parseLinuxVersionParts(version)
	if codename == "" {
		switch id {
		case "debian":
			codename = firewallDebianCodenameByMajor[major]
		case "ubuntu":
			codename = firewallUbuntuCodenameByVersion[normalizeUbuntuVersionKey(major, minor)]
		}
	}
	return firewallLinuxDistribution{
		ID:       id,
		IDLike:   strings.ToLower(strings.TrimSpace(fields["ID_LIKE"])),
		Version:  version,
		Codename: codename,
		Major:    major,
		Minor:    minor,
	}
}

func parseLinuxVersionParts(version string) (int, int) {
	version = strings.TrimSpace(strings.Trim(version, `"'`))
	if version == "" {
		return 0, 0
	}
	parts := strings.Split(version, ".")
	major, _ := strconv.Atoi(firstNumericPrefix(parts[0]))
	minor := 0
	if len(parts) > 1 {
		minor, _ = strconv.Atoi(firstNumericPrefix(parts[1]))
	}
	return major, minor
}

func firstNumericPrefix(value string) string {
	value = strings.TrimSpace(value)
	var builder strings.Builder
	for _, r := range value {
		if r < '0' || r > '9' {
			break
		}
		builder.WriteRune(r)
	}
	return builder.String()
}

func normalizeUbuntuVersionKey(major int, minor int) string {
	if major <= 0 {
		return ""
	}
	return fmt.Sprintf("%d.%02d", major, minor)
}

func normalizeLinuxReleaseToken(value string) string {
	value = strings.ToLower(strings.TrimSpace(strings.Trim(value, `"'`)))
	if value == "" {
		return ""
	}
	for _, r := range value {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		case r == '-' || r == '_' || r == '.':
		default:
			return ""
		}
	}
	return value
}

func isUbuntuVersionAtLeast(major int, minor int, minMajor int, minMinor int) bool {
	if major != minMajor {
		return major > minMajor
	}
	return minor >= minMinor
}

func detectFirewallLinuxSystemFamily(fields map[string]string) string {
	idLike := strings.ToLower(strings.TrimSpace(fields["ID_LIKE"]))
	id := strings.ToLower(strings.TrimSpace(fields["ID"]))
	switch {
	case strings.Contains(idLike, "debian") || id == "debian" || id == "ubuntu":
		return "debian"
	case strings.Contains(idLike, "rhel") || strings.Contains(idLike, "fedora") || id == "fedora" || id == "rhel" || id == "centos" || id == "rocky" || id == "almalinux" || id == "ol" || id == "oracle" || id == "amzn":
		return "rhel"
	case strings.Contains(idLike, "suse") || id == "sles" || id == "opensuse" || id == "opensuse-leap" || id == "opensuse-tumbleweed":
		return "suse"
	case strings.Contains(idLike, "arch") || id == "arch" || id == "manjaro":
		return "arch"
	case id == "alpine":
		return "alpine"
	default:
		return "unknown"
	}
}

func supportedDebianOrUbuntuNftInstall(distribution firewallLinuxDistribution) bool {
	switch distribution.ID {
	case "debian":
		return distribution.Major >= 9 && distribution.Codename != ""
	case "ubuntu":
		return distribution.Codename != "" && isUbuntuVersionAtLeast(distribution.Major, distribution.Minor, 18, 4)
	default:
		return false
	}
}

func buildDebianNftablesSourcesList(distribution firewallLinuxDistribution) string {
	codename := strings.TrimSpace(distribution.Codename)
	if codename == "" {
		return ""
	}

	var lines []string
	components := "main contrib non-free"
	if distribution.Major >= 12 || distribution.Major == 0 {
		components += " non-free-firmware"
	}
	base := "http://deb.debian.org/debian"
	securityBase := "http://security.debian.org/debian-security"
	lines = append(lines,
		fmt.Sprintf("deb %s %s %s", base, codename, components),
		fmt.Sprintf("deb %s %s-updates %s", base, codename, components),
		fmt.Sprintf("deb %s %s-security %s", securityBase, codename, components),
	)
	return strings.Join(lines, "\n") + "\n"
}

func buildUbuntuNftablesSourcesList(distribution firewallLinuxDistribution) string {
	codename := strings.TrimSpace(distribution.Codename)
	if codename == "" {
		return ""
	}
	base := "http://archive.ubuntu.com/ubuntu"
	securityBase := "http://security.ubuntu.com/ubuntu"
	lines := []string{
		fmt.Sprintf("deb %s %s main restricted universe multiverse", base, codename),
		fmt.Sprintf("deb %s %s-updates main restricted universe multiverse", base, codename),
		fmt.Sprintf("deb %s %s-backports main restricted universe multiverse", base, codename),
		fmt.Sprintf("deb %s %s-security main restricted universe multiverse", securityBase, codename),
	}
	return strings.Join(lines, "\n") + "\n"
}

func buildAptUpdateCommand(managerName string, distribution firewallLinuxDistribution) []string {
	command := []string{managerName}
	command = append(command, "update")
	return command
}

func buildNftablesServiceStartCommand() []string {
	script := "if command -v systemctl >/dev/null 2>&1; then systemctl enable --now nftables || systemctl start nftables || true; elif command -v service >/dev/null 2>&1; then service nftables start || true; fi"
	return []string{"sh", "-c", script}
}

func buildNftablesServiceManualCommand() string {
	return "sh -c " + shellSingleQuote("if command -v systemctl >/dev/null 2>&1; then systemctl enable --now nftables || systemctl start nftables || true; elif command -v service >/dev/null 2>&1; then service nftables start || true; fi")
}

func shellSingleQuote(value string) string {
	return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'"
}

func buildDebianUbuntuFirewallNftInstallPlan(fields map[string]string, managerName string) *firewallNftInstallPlan {
	distribution := parseFirewallLinuxDistribution(fields)
	if !supportedDebianOrUbuntuNftInstall(distribution) {
		return nil
	}
	updateCommand := buildAptUpdateCommand(managerName, distribution)
	installCommand := []string{managerName, "install", "-y", "nftables"}
	return &firewallNftInstallPlan{
		Name:           managerName,
		SystemFamily:   "debian",
		DistributionID: distribution.ID,
		VersionID:      distribution.Version,
		Codename:       distribution.Codename,
		InstallPlan: [][]string{
			updateCommand,
			installCommand,
		},
		PostInstallPlan: [][]string{
			buildNftablesServiceStartCommand(),
		},
		ManualCommands: []string{
			strings.Join(updateCommand, " "),
			strings.Join(installCommand, " "),
			buildNftablesServiceManualCommand(),
		},
	}
}

func firewallNftInstallPlans() []firewallNftInstallPlan {
	return []firewallNftInstallPlan{
		{
			Name:           "rpm-ostree",
			SystemFamily:   "rhel",
			Immutable:      true,
			ManualCommands: []string{"rpm-ostree install nftables", "reboot"},
		},
		{
			Name:           "transactional-update",
			SystemFamily:   "suse",
			Immutable:      true,
			ManualCommands: []string{"transactional-update pkg install nftables", "reboot"},
		},
		{
			Name:         "apt-get",
			SystemFamily: "debian",
			InstallPlan: [][]string{
				{"apt-get", "update"},
				{"apt-get", "install", "-y", "nftables"},
			},
		},
		{
			Name:         "apt",
			SystemFamily: "debian",
			InstallPlan: [][]string{
				{"apt", "update"},
				{"apt", "install", "-y", "nftables"},
			},
		},
		{
			Name:         "dnf5",
			SystemFamily: "rhel",
			InstallPlan:  [][]string{{"dnf5", "install", "-y", "nftables"}},
		},
		{
			Name:         "dnf",
			SystemFamily: "rhel",
			InstallPlan:  [][]string{{"dnf", "install", "-y", "nftables"}},
		},
		{
			Name:         "microdnf",
			SystemFamily: "rhel",
			InstallPlan:  [][]string{{"microdnf", "install", "-y", "nftables"}},
		},
		{
			Name:         "yum",
			SystemFamily: "rhel",
			InstallPlan:  [][]string{{"yum", "install", "-y", "nftables"}},
		},
		{
			Name:         "zypper",
			SystemFamily: "suse",
			InstallPlan:  [][]string{{"zypper", "--non-interactive", "install", "nftables"}},
		},
		{
			Name:         "pacman",
			SystemFamily: "arch",
			InstallPlan:  [][]string{{"pacman", "-S", "--needed", "--noconfirm", "nftables"}},
		},
		{
			Name:         "apk",
			SystemFamily: "alpine",
			InstallPlan:  [][]string{{"apk", "add", "--no-cache", "nftables"}},
		},
	}
}

func detectFirewallNftInstallPlan(fields map[string]string) *firewallNftInstallPlan {
	systemFamily := detectFirewallLinuxSystemFamily(fields)
	if systemFamily == "debian" {
		for _, managerName := range []string{"apt-get", "apt"} {
			if _, err := firewallCommandLookPath(managerName); err == nil {
				if plan := buildDebianUbuntuFirewallNftInstallPlan(fields, managerName); plan != nil {
					return plan
				}
			}
		}
		return nil
	}
	for _, candidate := range firewallNftInstallPlans() {
		if systemFamily != "" && systemFamily != "unknown" && candidate.SystemFamily != "" && candidate.SystemFamily != systemFamily {
			continue
		}
		if _, err := firewallCommandLookPath(candidate.Name); err == nil {
			plan := candidate
			plan.SystemFamily = firstNonEmpty(plan.SystemFamily, systemFamily, "unknown")
			plan.ManualCommands = buildDefaultNftablesManualCommands(plan)
			return &plan
		}
	}
	for _, candidate := range firewallNftInstallPlans() {
		if _, err := firewallCommandLookPath(candidate.Name); err == nil {
			plan := candidate
			plan.SystemFamily = firstNonEmpty(plan.SystemFamily, systemFamily, "unknown")
			plan.ManualCommands = buildDefaultNftablesManualCommands(plan)
			return &plan
		}
	}
	return nil
}

func buildDefaultNftablesManualCommands(plan firewallNftInstallPlan) []string {
	if len(plan.ManualCommands) > 0 {
		return append([]string{}, plan.ManualCommands...)
	}
	commands := make([]string, 0, len(plan.InstallPlan)+len(plan.PostInstallPlan))
	for _, command := range plan.InstallPlan {
		commands = append(commands, strings.Join(command, " "))
	}
	for _, command := range plan.PostInstallPlan {
		commands = append(commands, strings.Join(command, " "))
	}
	return commands
}

func detectFirewallPrivilegeContext() firewallPrivilegeContext {
	ctx := firewallPrivilegeContext{
		IsRoot: firewallGeteuid() == 0,
	}
	if sudoPath, err := firewallCommandLookPath("sudo"); err == nil {
		ctx.SudoPath = strings.TrimSpace(sudoPath)
	}
	return ctx
}

func buildFirewallManualCommands(plan *firewallNftInstallPlan, privilege firewallPrivilegeContext) []string {
	if plan == nil {
		return nil
	}
	commands := append([]string{}, plan.ManualCommands...)
	if len(commands) == 0 {
		for _, command := range plan.InstallPlan {
			commands = append(commands, strings.Join(command, " "))
		}
		for _, command := range plan.PostInstallPlan {
			commands = append(commands, strings.Join(command, " "))
		}
	}
	if len(commands) == 0 {
		return nil
	}
	if privilege.IsRoot {
		return commands
	}
	if strings.TrimSpace(privilege.SudoPath) != "" {
		prefixed := make([]string, 0, len(commands))
		for _, command := range commands {
			prefixed = append(prefixed, "sudo "+command)
		}
		return prefixed
	}
	return commands
}

func buildFirewallAutomaticInstallCommands(plan *firewallNftInstallPlan, privilege firewallPrivilegeContext) [][]string {
	if plan == nil || len(plan.InstallPlan) == 0 {
		return nil
	}
	commands := make([][]string, 0, len(plan.InstallPlan))
	for _, command := range plan.InstallPlan {
		next := append([]string{}, command...)
		if !privilege.IsRoot {
			next = append([]string{"sudo", "-n"}, next...)
		}
		commands = append(commands, next)
	}
	for _, command := range plan.PostInstallPlan {
		next := append([]string{}, command...)
		if !privilege.IsRoot {
			next = append([]string{"sudo", "-n"}, next...)
		}
		commands = append(commands, next)
	}
	return commands
}

func setFirewallNftInstallFailure(message string) {
	firewallNftInstallStateM.Lock()
	defer firewallNftInstallStateM.Unlock()
	firewallNftInstallState.lastFailure = strings.TrimSpace(message)
}

func clearFirewallNftInstallFailure() {
	firewallNftInstallStateM.Lock()
	defer firewallNftInstallStateM.Unlock()
	firewallNftInstallState.lastFailure = ""
}

func getFirewallNftInstallFailure() string {
	firewallNftInstallStateM.Lock()
	defer firewallNftInstallStateM.Unlock()
	return firewallNftInstallState.lastFailure
}

func buildFirewallNftablesStatus(available bool) FirewallNftablesStatus {
	status := FirewallNftablesStatus{
		Supported: firewallRuntimeGOOS == "linux",
		Reason:    firewallNftReasonNotLinux,
	}
	if !status.Supported {
		return status
	}

	fields := readFirewallOsReleaseFields()
	systemFamily := firstNonEmpty(detectFirewallLinuxSystemFamily(fields), "unknown")
	distribution := parseFirewallLinuxDistribution(fields)
	plan := detectFirewallNftInstallPlan(fields)
	privilege := detectFirewallPrivilegeContext()

	status.SystemFamily = systemFamily
	status.DistributionID = distribution.ID
	status.VersionID = distribution.Version
	status.Codename = distribution.Codename
		if plan != nil {
			status.PackageManager = plan.Name
			status.SystemFamily = firstNonEmpty(plan.SystemFamily, systemFamily, "unknown")
			status.DistributionID = firstNonEmpty(plan.DistributionID, status.DistributionID)
			status.VersionID = firstNonEmpty(plan.VersionID, status.VersionID)
			status.Codename = firstNonEmpty(plan.Codename, status.Codename)
			status.ManualCommands = buildFirewallManualCommands(plan, privilege)
		}
	status.AutoInstallSupported = plan != nil && !plan.Immutable && privilege.canAutoInstall()

	if binaryPath, err := resolveNftBinaryPath(); err == nil {
		status.Installed = true
		status.BinaryPath = binaryPath
		if available {
			status.Reason = firewallNftReasonReady
			clearFirewallNftInstallFailure()
			return status
		}
		status.Reason = firewallNftReasonPermissionDenied
		return status
	}

	lastFailure := getFirewallNftInstallFailure()
	switch {
	case plan != nil && plan.Immutable:
		status.Reason = firewallNftReasonImmutableOS
	case plan == nil:
		status.Reason = firewallNftReasonUnsupportedManager
	case !privilege.canAutoInstall():
		status.Reason = firewallNftReasonPermissionDenied
	case lastFailure != "":
		status.Reason = firewallNftReasonInstallFailed
	default:
		status.Reason = firewallNftReasonMissingBinary
	}

	return status
}

func runFirewallNftInstallCommand(command []string) error {
	if len(command) == 0 {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, command[0], command[1:]...)
	output, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("nftables install command timed out: %s", strings.Join(command, " "))
	}
	if err != nil {
		return fmt.Errorf("nftables install command failed (%s): %w: %s", strings.Join(command, " "), err, strings.TrimSpace(string(output)))
	}
	return nil
}

func formatFirewallManualCommands(commands []string) string {
	if len(commands) == 0 {
		return ""
	}
	filtered := make([]string, 0, len(commands))
	for _, command := range commands {
		trimmed := strings.TrimSpace(command)
		if trimmed == "" {
			continue
		}
		filtered = append(filtered, trimmed)
	}
	return strings.Join(filtered, " ; ")
}

func buildFirewallNftablesOverviewError(status FirewallNftablesStatus) string {
	manualText := formatFirewallManualCommands(status.ManualCommands)
	switch status.Reason {
	case firewallNftReasonReady:
		return ""
	case firewallNftReasonNotLinux:
		return "nftables firewall is unavailable on this host (Linux only)"
	case firewallNftReasonMissingBinary:
		message := "nftables is not installed on this host."
		if manualText != "" {
			message += " Run manually: " + manualText
		}
		return message
	case firewallNftReasonPermissionDenied:
		if status.Installed {
			return "nftables is installed, but kwor lacks permission to execute nft. Restart kwor as root or grant passwordless sudo."
		}
		message := "nftables is not installed and automatic installation requires root or passwordless sudo."
		if manualText != "" {
			message += " Run manually: " + manualText
		}
		return message
	case firewallNftReasonUnsupportedManager:
		return "nftables is not installed and no supported Linux package manager was found."
	case firewallNftReasonImmutableOS:
		message := "nftables must be installed through the system's immutable package workflow."
		if manualText != "" {
			message += " Run manually: " + manualText
		}
		return message
	case firewallNftReasonInstallFailed:
		message := "automatic nftables install failed"
		if lastFailure := getFirewallNftInstallFailure(); lastFailure != "" {
			message += ": " + lastFailure
		}
		if manualText != "" {
			message += ". Run manually: " + manualText
		}
		return message
	default:
		return "nftables status is unavailable"
	}
}

func (s *FirewallService) InstallNftables() (*FirewallOverview, error) {
	if firewallRuntimeGOOS != "linux" {
		return nil, common.NewError("nftables install is supported on Linux only")
	}

	available := firewallSupportedFn()
	status := buildFirewallNftablesStatus(available)
	if status.Installed {
		if available {
			clearFirewallNftInstallFailure()
			return s.GetOverview()
		}
		return nil, common.NewError(buildFirewallNftablesOverviewError(status))
	}

	fields := readFirewallOsReleaseFields()
	plan := detectFirewallNftInstallPlan(fields)
	privilege := detectFirewallPrivilegeContext()
	if plan == nil || plan.Immutable || !privilege.canAutoInstall() {
		return nil, common.NewError(buildFirewallNftablesOverviewError(status))
	}

	commands := buildFirewallAutomaticInstallCommands(plan, privilege)
	for _, command := range commands {
		if err := firewallRunInstall(command); err != nil {
			message := strings.TrimSpace(err.Error())
			setFirewallNftInstallFailure(message)
			return nil, common.NewError(buildFirewallNftablesOverviewError(buildFirewallNftablesStatus(false)))
		}
	}

	clearFirewallNftInstallFailure()
	overview, err := s.GetOverview()
	if err != nil {
		return nil, err
	}
	if !overview.Nftables.Installed {
		setFirewallNftInstallFailure("nftables install finished, but nft binary is still missing")
		return nil, common.NewError(buildFirewallNftablesOverviewError(buildFirewallNftablesStatus(false)))
	}
	return overview, nil
}
