package service

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/util/common"
)

const (
	sshDirectivePort               = "port"
	sshDirectiveAllowTcpForwarding = "allowtcpforwarding"
	sshDirectivePermitOpen         = "permitopen"
	sshDirectiveGatewayPorts       = "gatewayports"
)

var sshMainConfigCandidates = []string{
	"/etc/ssh/sshd_config",
	"/etc/sshd_config",
	"/usr/local/etc/sshd_config",
}

var sshServiceCandidates = []string{"sshd", "ssh"}

var sshDirectiveCanonicalName = map[string]string{
	sshDirectivePort:               "Port",
	sshDirectiveAllowTcpForwarding: "AllowTcpForwarding",
	sshDirectivePermitOpen:         "PermitOpen",
	sshDirectiveGatewayPorts:       "GatewayPorts",
}

var sshDirectiveStableOrder = []string{
	sshDirectivePort,
	sshDirectiveAllowTcpForwarding,
	sshDirectivePermitOpen,
	sshDirectiveGatewayPorts,
}

type FirewallSSHConfigStatus struct {
	Supported          bool   `json:"supported"`
	ConfigPath         string `json:"configPath"`
	Ports              []int  `json:"ports"`
	Port               int    `json:"port"`
	ProxyEnabled       bool   `json:"proxyEnabled"`
	AllowTcpForwarding string `json:"allowTcpForwarding"`
	PermitOpen         string `json:"permitOpen"`
	GatewayPorts       string `json:"gatewayPorts"`
	Error              string `json:"error,omitempty"`
}

type sshDirectiveProbe struct {
	Ports map[int]struct{}

	PrimaryPort    int
	HasPrimaryPort bool

	AllowTcpForwarding    string
	HasAllowTcpForwarding bool

	PermitOpen    string
	HasPermitOpen bool

	GatewayPorts    string
	HasGatewayPorts bool
}

func detectSSHConfigMainPath() string {
	for _, candidate := range sshMainConfigCandidates {
		if pathExists(candidate) {
			return candidate
		}
	}
	return sshMainConfigCandidates[0]
}

func resolveExistingSSHConfigMainPath() (string, error) {
	for _, candidate := range sshMainConfigCandidates {
		if pathExists(candidate) {
			return candidate, nil
		}
	}
	return "", common.NewError("ssh config file not found; checked: ", strings.Join(sshMainConfigCandidates, ", "))
}

func resolveFirewallSSHConfigStatus() FirewallSSHConfigStatus {
	status := FirewallSSHConfigStatus{
		Supported: runtime.GOOS == "linux",
		ConfigPath: func() string {
			if runtime.GOOS != "linux" {
				return ""
			}
			return detectSSHConfigMainPath()
		}(),
		Ports: []int{22},
		Port:  22,
	}
	if runtime.GOOS != "linux" {
		status.Error = "ssh config management is available on Linux only"
		return status
	}

	probe, err := probeSSHConfig(status.ConfigPath)
	if err != nil {
		status.Error = err.Error()
		return status
	}

	if len(probe.Ports) > 0 {
		status.Ports = probe.Ports
	}
	if probe.Port > 0 {
		status.Port = probe.Port
	} else if len(status.Ports) > 0 {
		status.Port = status.Ports[0]
	}

	status.AllowTcpForwarding = probe.AllowTcpForwarding
	status.PermitOpen = probe.PermitOpen
	status.GatewayPorts = probe.GatewayPorts
	status.ProxyEnabled = strings.EqualFold(status.AllowTcpForwarding, "yes") &&
		strings.EqualFold(status.PermitOpen, "any") &&
		strings.EqualFold(status.GatewayPorts, "no")
	return status
}

func (s *FirewallService) UpdateSSHPort(port int) error {
	if port < 1 || port > 65535 {
		return common.NewError("ssh port must be in range 1-65535")
	}
	if runtime.GOOS != "linux" {
		return common.NewError("ssh config update is available on Linux only")
	}

	firewallStateMu.Lock()
	defer firewallStateMu.Unlock()

	configPath, err := resolveExistingSSHConfigMainPath()
	if err != nil {
		return err
	}
	if err := applyAndRestartSSHConfig(configPath, map[string]string{
		sshDirectivePort: strconv.Itoa(port),
	}); err != nil {
		return err
	}
	return s.syncFirewallAfterSSHConfigChangeLocked()
}

func (s *FirewallService) SetSSHProxyEnabled(enabled bool) error {
	if runtime.GOOS != "linux" {
		return common.NewError("ssh config update is available on Linux only")
	}

	firewallStateMu.Lock()
	defer firewallStateMu.Unlock()

	configPath, err := resolveExistingSSHConfigMainPath()
	if err != nil {
		return err
	}

	directives := map[string]string{
		sshDirectiveAllowTcpForwarding: "no",
	}
	if enabled {
		directives[sshDirectiveAllowTcpForwarding] = "yes"
		directives[sshDirectivePermitOpen] = "any"
		directives[sshDirectiveGatewayPorts] = "no"
	}

	return applyAndRestartSSHConfig(configPath, directives)
}

func (s *FirewallService) syncFirewallAfterSSHConfigChangeLocked() error {
	defaults := resolveFirewallDefaultPorts()
	if err := upsertFirewallSystemRulesLocked(database.GetDB(), defaults); err != nil {
		return err
	}

	enabled, err := s.getFirewallEnabledLocked()
	if err != nil {
		return err
	}
	if !enabled || !firewallSupported() {
		return nil
	}
	return s.reconcileLocked(0)
}

func probeSSHConfig(mainPath string) (FirewallSSHConfigStatus, error) {
	state := &sshDirectiveProbe{
		Ports: make(map[int]struct{}),
	}
	visited := map[string]struct{}{}
	if err := collectSSHDirectiveProbeFromFile(mainPath, visited, state, true); err != nil {
		return FirewallSSHConfigStatus{}, err
	}

	ports := make([]int, 0, len(state.Ports))
	for value := range state.Ports {
		ports = append(ports, value)
	}
	sort.Ints(ports)

	return FirewallSSHConfigStatus{
		Ports:              ports,
		Port:               state.PrimaryPort,
		AllowTcpForwarding: state.AllowTcpForwarding,
		PermitOpen:         state.PermitOpen,
		GatewayPorts:       state.GatewayPorts,
		Supported:          true,
		ConfigPath:         mainPath,
	}, nil
}

func collectSSHDirectiveProbeFromFile(path string, visited map[string]struct{}, probe *sshDirectiveProbe, required bool) error {
	path = strings.TrimSpace(path)
	if path == "" {
		if required {
			return common.NewError("ssh config path is empty")
		}
		return nil
	}

	cleanPath := filepath.Clean(path)
	if _, exists := visited[cleanPath]; exists {
		return nil
	}
	visited[cleanPath] = struct{}{}

	content, err := os.ReadFile(cleanPath)
	if err != nil {
		if required {
			return common.NewError("failed to read ssh config: ", err)
		}
		return nil
	}

	lines := strings.Split(string(content), "\n")
	inMatchBlock := false
	for _, rawLine := range lines {
		key, values, _, ok := parseSSHDirectiveLine(rawLine)
		if !ok {
			continue
		}
		if key == "match" {
			inMatchBlock = true
			continue
		}
		if inMatchBlock {
			continue
		}

		switch key {
		case "include":
			for _, includePattern := range values {
				for _, includeFile := range expandSSHIncludePattern(includePattern, cleanPath) {
					if err := collectSSHDirectiveProbeFromFile(includeFile, visited, probe, false); err != nil {
						return err
					}
				}
			}
		case sshDirectivePort:
			for _, rawPort := range values {
				port, parseErr := strconv.Atoi(normalizeSSHDirectiveToken(rawPort))
				if parseErr != nil || port < 1 || port > 65535 {
					continue
				}
				probe.Ports[port] = struct{}{}
				if !probe.HasPrimaryPort {
					probe.PrimaryPort = port
					probe.HasPrimaryPort = true
				}
			}
		case sshDirectiveAllowTcpForwarding:
			if !probe.HasAllowTcpForwarding && len(values) > 0 {
				probe.AllowTcpForwarding = strings.ToLower(normalizeSSHDirectiveToken(values[0]))
				probe.HasAllowTcpForwarding = true
			}
		case sshDirectivePermitOpen:
			if !probe.HasPermitOpen && len(values) > 0 {
				probe.PermitOpen = strings.ToLower(normalizeSSHDirectiveToken(values[0]))
				probe.HasPermitOpen = true
			}
		case sshDirectiveGatewayPorts:
			if !probe.HasGatewayPorts && len(values) > 0 {
				probe.GatewayPorts = strings.ToLower(normalizeSSHDirectiveToken(values[0]))
				probe.HasGatewayPorts = true
			}
		}
	}
	return nil
}

func parseSSHDirectiveLine(line string) (key string, values []string, indent string, ok bool) {
	trimmedLeft := strings.TrimLeft(line, " \t")
	if trimmedLeft == "" {
		return "", nil, "", false
	}
	indent = line[:len(line)-len(trimmedLeft)]

	stripped := stripSSHConfigComment(line)
	if stripped == "" {
		return "", nil, "", false
	}
	fields := strings.Fields(stripped)
	if len(fields) == 0 {
		return "", nil, "", false
	}
	key = strings.ToLower(strings.TrimSpace(fields[0]))
	if len(fields) > 1 {
		values = fields[1:]
	}
	return key, values, indent, true
}

func normalizeSSHDirectiveToken(raw string) string {
	return strings.Trim(strings.TrimSpace(raw), "\"'")
}

func applyAndRestartSSHConfig(configPath string, directives map[string]string) error {
	previous, changed, err := applySSHDirectiveChanges(configPath, directives)
	if err != nil {
		return err
	}
	if !changed {
		return nil
	}

	if err := validateSSHConfig(configPath); err != nil {
		_ = restoreSSHConfigFile(configPath, previous)
		return common.NewError("updated ssh config is invalid: ", err)
	}

	if err := restartSSHService(); err != nil {
		restoreErr := restoreSSHConfigFile(configPath, previous)
		if restoreErr == nil {
			_ = restartSSHService()
		}
		if restoreErr != nil {
			return common.NewError("failed to restart ssh service and rollback failed: ", err, "; rollback: ", restoreErr)
		}
		return common.NewError("failed to restart ssh service: ", err)
	}
	return nil
}

func applySSHDirectiveChanges(configPath string, directives map[string]string) ([]byte, bool, error) {
	original, err := os.ReadFile(configPath)
	if err != nil {
		return nil, false, common.NewError("failed to read ssh config: ", err)
	}

	updated, changed := renderSSHConfigWithDirectiveOverrides(original, directives)
	if !changed {
		return original, false, nil
	}

	fileMode := os.FileMode(0o600)
	if stat, statErr := os.Stat(configPath); statErr == nil {
		fileMode = stat.Mode().Perm()
		if fileMode == 0 {
			fileMode = 0o600
		}
	}
	if err := os.WriteFile(configPath, updated, fileMode); err != nil {
		return nil, false, common.NewError("failed to write ssh config: ", err)
	}
	return original, true, nil
}

func restoreSSHConfigFile(configPath string, content []byte) error {
	fileMode := os.FileMode(0o600)
	if stat, err := os.Stat(configPath); err == nil {
		fileMode = stat.Mode().Perm()
		if fileMode == 0 {
			fileMode = 0o600
		}
	}
	return os.WriteFile(configPath, content, fileMode)
}

func renderSSHConfigWithDirectiveOverrides(content []byte, directives map[string]string) ([]byte, bool) {
	normalized := normalizeSSHDirectiveOverrides(directives)
	if len(normalized) == 0 {
		return content, false
	}

	raw := string(content)
	lineEnding := "\n"
	if strings.Contains(raw, "\r\n") {
		lineEnding = "\r\n"
		raw = strings.ReplaceAll(raw, "\r\n", "\n")
	}
	hadTrailingNewline := strings.HasSuffix(raw, "\n")
	raw = strings.TrimSuffix(raw, "\n")

	lines := []string{}
	if raw != "" {
		lines = strings.Split(raw, "\n")
	}

	globalEnd := len(lines)
	for index, line := range lines {
		key, _, _, ok := parseSSHDirectiveLine(line)
		if !ok {
			continue
		}
		if key == "match" {
			globalEnd = index
			break
		}
	}

	changed := false
	applied := make(map[string]bool, len(normalized))
	for index := 0; index < globalEnd; index++ {
		key, _, indent, ok := parseSSHDirectiveLine(lines[index])
		if !ok {
			continue
		}
		value, exists := normalized[key]
		if !exists {
			continue
		}
		next := indent + sshDirectiveCanonicalName[key] + " " + value
		if lines[index] != next {
			lines[index] = next
			changed = true
		}
		applied[key] = true
	}

	insertLines := make([]string, 0)
	for _, key := range sshDirectiveStableOrder {
		value, exists := normalized[key]
		if !exists || applied[key] {
			continue
		}
		insertLines = append(insertLines, sshDirectiveCanonicalName[key]+" "+value)
		applied[key] = true
	}

	extraKeys := make([]string, 0)
	for key := range normalized {
		if applied[key] {
			continue
		}
		extraKeys = append(extraKeys, key)
	}
	sort.Strings(extraKeys)
	for _, key := range extraKeys {
		insertLines = append(insertLines, sshDirectiveCanonicalName[key]+" "+normalized[key])
	}

	if len(insertLines) > 0 {
		prefix := append([]string{}, lines[:globalEnd]...)
		prefix = append(prefix, insertLines...)
		lines = append(prefix, lines[globalEnd:]...)
		changed = true
	}

	if !changed {
		return content, false
	}

	updated := strings.Join(lines, "\n")
	if hadTrailingNewline {
		updated += "\n"
	}
	if lineEnding == "\r\n" {
		updated = strings.ReplaceAll(updated, "\n", "\r\n")
	}
	return []byte(updated), true
}

func normalizeSSHDirectiveOverrides(directives map[string]string) map[string]string {
	normalized := make(map[string]string, len(directives))
	for rawKey, rawValue := range directives {
		key := strings.ToLower(strings.TrimSpace(rawKey))
		if _, known := sshDirectiveCanonicalName[key]; !known {
			continue
		}
		value := strings.TrimSpace(rawValue)
		if value == "" {
			continue
		}
		normalized[key] = value
	}
	return normalized
}

func validateSSHConfig(configPath string) error {
	if runtime.GOOS != "linux" {
		return nil
	}

	sshdPath := resolveSSHDExecutablePath()
	if sshdPath == "" {
		return nil
	}

	if err := runCommandWithTimeout(8*time.Second, sshdPath, "-t", "-f", configPath); err != nil {
		return err
	}
	return nil
}

func resolveSSHDExecutablePath() string {
	if path, err := exec.LookPath("sshd"); err == nil {
		return path
	}
	fallbacks := []string{
		"/usr/sbin/sshd",
		"/usr/local/sbin/sshd",
	}
	for _, candidate := range fallbacks {
		if pathExists(candidate) {
			return candidate
		}
	}
	return ""
}

func restartSSHService() error {
	if runtime.GOOS != "linux" {
		return common.NewError("ssh service restart is available on Linux only")
	}

	attempts := make([]string, 0)
	appendAttempt := func(prefix string, err error) {
		if err == nil {
			return
		}
		attempts = append(attempts, prefix+": "+strings.TrimSpace(err.Error()))
	}

	if systemctlPath, err := exec.LookPath("systemctl"); err == nil {
		for _, serviceName := range sshServiceCandidates {
			commandErr := runCommandWithTimeout(12*time.Second, systemctlPath, "restart", serviceName)
			if commandErr == nil {
				return nil
			}
			appendAttempt("systemctl restart "+serviceName, commandErr)
		}
	}

	if servicePath, err := exec.LookPath("service"); err == nil {
		for _, serviceName := range sshServiceCandidates {
			commandErr := runCommandWithTimeout(12*time.Second, servicePath, serviceName, "restart")
			if commandErr == nil {
				return nil
			}
			appendAttempt("service "+serviceName+" restart", commandErr)
		}
	}

	if openrcPath, err := exec.LookPath("rc-service"); err == nil {
		for _, serviceName := range sshServiceCandidates {
			commandErr := runCommandWithTimeout(12*time.Second, openrcPath, serviceName, "restart")
			if commandErr == nil {
				return nil
			}
			appendAttempt("rc-service "+serviceName+" restart", commandErr)
		}
	}

	if runitPath, err := exec.LookPath("sv"); err == nil {
		for _, serviceName := range sshServiceCandidates {
			commandErr := runCommandWithTimeout(12*time.Second, runitPath, "restart", serviceName)
			if commandErr == nil {
				return nil
			}
			appendAttempt("sv restart "+serviceName, commandErr)
		}
	}

	for _, serviceName := range sshServiceCandidates {
		initScript := filepath.Join("/etc/init.d", serviceName)
		if !pathExists(initScript) {
			continue
		}
		commandErr := runCommandWithTimeout(12*time.Second, initScript, "restart")
		if commandErr == nil {
			return nil
		}
		appendAttempt(initScript+" restart", commandErr)
	}

	if len(attempts) == 0 {
		return common.NewError("no supported ssh service manager found (expected one of systemctl/service/rc-service/sv)")
	}
	return common.NewError("all ssh restart attempts failed: ", strings.Join(attempts, " | "))
}

func runCommandWithTimeout(timeout time.Duration, command string, args ...string) error {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, command, args...)
	output, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return fmt.Errorf("command timed out (%s %s)", command, strings.Join(args, " "))
	}
	if err != nil {
		trimmed := strings.TrimSpace(string(output))
		if trimmed == "" {
			return err
		}
		return fmt.Errorf("%w: %s", err, trimmed)
	}
	return nil
}

func pathExists(path string) bool {
	path = strings.TrimSpace(path)
	if path == "" {
		return false
	}
	if _, err := os.Stat(path); err != nil {
		return false
	}
	return true
}
