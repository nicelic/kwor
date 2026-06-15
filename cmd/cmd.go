package cmd

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/alireza0/s-ui/cmd/migration"
	"github.com/alireza0/s-ui/config"
	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/service"
)

const (
	kworServiceName  = "kwor"
	defaultPanelPort = 8888
	defaultPanelPath = "/app/"
)

// getServiceFilePath returns the systemd service file path for kwor.
func getServiceFilePath() string {
	return "/etc/systemd/system/" + kworServiceName + ".service"
}

// getBinDir returns the directory of the currently running binary.
func getBinDir() string {
	execPath, err := os.Executable()
	if err != nil {
		dir, _ := os.Getwd()
		return dir
	}
	realPath, err := filepath.EvalSymlinks(execPath)
	if err != nil {
		realPath = execPath
	}
	return filepath.Dir(realPath)
}

// getBinPath returns the absolute path of the currently running binary.
func getBinPath() string {
	execPath, err := os.Executable()
	if err != nil {
		return "./kwor"
	}
	realPath, err := filepath.EvalSymlinks(execPath)
	if err != nil {
		return execPath
	}
	return realPath
}

// isProcessRunning checks whether a kwor process is running (excluding self).
func isProcessRunning() bool {
	out, err := exec.Command("pgrep", "-x", kworServiceName).Output()
	if err != nil {
		return false
	}
	pids := strings.TrimSpace(string(out))
	if pids == "" {
		return false
	}

	selfPid := strconv.Itoa(os.Getpid())
	for _, pid := range strings.Split(pids, "\n") {
		pid = strings.TrimSpace(pid)
		if pid != "" && pid != selfPid {
			return true
		}
	}
	return false
}

func isSystemdServiceExists() bool {
	_, err := os.Stat(getServiceFilePath())
	return err == nil
}

func isSystemdServiceActive() bool {
	return exec.Command("systemctl", "is-active", "--quiet", kworServiceName).Run() == nil
}

func isSingboxServiceActive() bool {
	singboxName := service.GetSingboxSystemdName()
	return exec.Command("systemctl", "is-active", "--quiet", singboxName).Run() == nil
}

func isSingboxServiceExists() bool {
	singboxName := service.GetSingboxSystemdName()
	_, err := os.Stat("/etc/systemd/system/" + singboxName + ".service")
	return err == nil
}

func isMihomoServiceActive() bool {
	mihomoName := service.GetMihomoSystemdName()
	return exec.Command("systemctl", "is-active", "--quiet", mihomoName).Run() == nil
}

func isMihomoServiceExists() bool {
	mihomoName := service.GetMihomoSystemdName()
	_, err := os.Stat("/etc/systemd/system/" + mihomoName + ".service")
	return err == nil
}

func isNamedProcessRunning(processName string) bool {
	out, err := exec.Command("pgrep", "-x", processName).Output()
	if err != nil {
		return false
	}
	return len(strings.TrimSpace(string(out))) > 0
}

func stopManagedChildService(serviceName string, processName string, label string) bool {
	stoppedSomething := false

	if exec.Command("systemctl", "is-active", "--quiet", serviceName).Run() == nil {
		fmt.Printf("[kwor] stopping %s child service via systemd...\n", label)
		if err := exec.Command("systemctl", "stop", serviceName).Run(); err != nil {
			fmt.Printf("[kwor] failed to stop %s with systemd: %v, fallback to kill\n", label, err)
			exec.Command("pkill", "-TERM", "-x", processName).Run()
			time.Sleep(2 * time.Second)
			exec.Command("pkill", "-KILL", "-x", processName).Run()
		}
		fmt.Printf("[kwor] %s stopped\n", label)
		stoppedSomething = true
	} else if isNamedProcessRunning(processName) {
		fmt.Printf("[kwor] force stopping %s process...\n", label)
		exec.Command("pkill", "-TERM", "-x", processName).Run()
		time.Sleep(2 * time.Second)
		if isNamedProcessRunning(processName) {
			exec.Command("pkill", "-KILL", "-x", processName).Run()
		}
		fmt.Printf("[kwor] %s process stopped\n", label)
		stoppedSomething = true
	}

	return stoppedSomething
}

func createSystemdService() error {
	binPath := getBinPath()
	binDir := getBinDir()

	serviceContent := fmt.Sprintf(`[Unit]
Description=kwor Service
After=network.target nss-lookup.target

[Service]
Type=simple
WorkingDirectory=%s
ExecStart=%s
Restart=on-failure
RestartSec=5s
LimitNOFILE=infinity

[Install]
WantedBy=multi-user.target
`, binDir, binPath)

	if err := os.WriteFile(getServiceFilePath(), []byte(serviceContent), 0o644); err != nil {
		return fmt.Errorf("failed to create systemd service file: %v", err)
	}

	if err := verifySystemdServiceFile(); err != nil {
		return err
	}

	if err := exec.Command("systemctl", "daemon-reload").Run(); err != nil {
		return fmt.Errorf("failed to reload systemd daemon: %v", err)
	}

	return nil
}

func verifySystemdServiceFile() error {
	out, err := exec.Command("systemd-analyze", "verify", getServiceFilePath()).CombinedOutput()
	if err == nil {
		return nil
	}
	if errors.Is(err, exec.ErrNotFound) {
		// systemd-analyze not available on this distro; skip strict verify.
		return nil
	}
	detail := strings.TrimSpace(string(out))
	if detail == "" {
		return fmt.Errorf("systemd unit verify failed: %v", err)
	}
	return fmt.Errorf("systemd unit verify failed: %v: %s", err, detail)
}

func enableAndStartService() error {
	if err := exec.Command("systemctl", "enable", kworServiceName).Run(); err != nil {
		return fmt.Errorf("failed to enable service: %v", err)
	}
	if err := exec.Command("systemctl", "start", kworServiceName).Run(); err != nil {
		printSystemdDiagnostics(kworServiceName)
		return fmt.Errorf("failed to start service: %v", err)
	}
	if !waitSystemdServiceActive(kworServiceName, 12*time.Second) {
		state := systemdActiveState(kworServiceName)
		if state == "" {
			state = "unknown"
		}
		printSystemdDiagnostics(kworServiceName)
		return fmt.Errorf("service state after start is %q", state)
	}
	return nil
}

func systemdActiveState(serviceName string) string {
	out, err := exec.Command("systemctl", "is-active", serviceName).Output()
	if err != nil && len(out) == 0 {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func waitSystemdServiceActive(serviceName string, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		state := systemdActiveState(serviceName)
		switch state {
		case "active":
			return true
		case "failed", "inactive":
			return false
		}
		time.Sleep(500 * time.Millisecond)
	}
	return systemdActiveState(serviceName) == "active"
}

func systemdStatusOutput(serviceName string) string {
	out, err := exec.Command("systemctl", "status", serviceName, "--no-pager", "-l").CombinedOutput()
	if err != nil && len(out) == 0 {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func systemdJournalOutput(serviceName string, lines int) string {
	out, err := exec.Command("journalctl", "-u", serviceName, "-n", strconv.Itoa(lines), "--no-pager").CombinedOutput()
	if err != nil && len(out) == 0 {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func printSystemdDiagnostics(serviceName string) {
	statusOut := systemdStatusOutput(serviceName)
	if statusOut != "" {
		fmt.Printf("[kwor] systemctl status %s:\n%s\n", serviceName, statusOut)
	}

	journalOut := systemdJournalOutput(serviceName, 40)
	if journalOut != "" {
		fmt.Printf("[kwor] journalctl -u %s -n 40:\n%s\n", serviceName, journalOut)
		printLikelyFailureHints(journalOut)
	}
}

func printLikelyFailureHints(journalText string) {
	j := strings.ToLower(journalText)
	switch {
	case strings.Contains(j, "address already in use"):
		fmt.Println("[kwor] possible cause: port conflict. Check webPort/subPort.")
	case strings.Contains(j, "unknown time zone"), strings.Contains(j, "unknown timezone"):
		fmt.Println("[kwor] possible cause: timezone data missing. Install tzdata or set timeLocation=Local.")
	case strings.Contains(j, "permission denied"):
		fmt.Println("[kwor] possible cause: permission denied. Check executable and directory permissions.")
	case strings.Contains(j, "no such file"), strings.Contains(j, "not found"):
		fmt.Println("[kwor] possible cause: missing file/path. Check cert files, working directory, and binary path.")
	}
}

func isInternalSystemdCommandAllowed() bool {
	value := strings.TrimSpace(os.Getenv(service.InternalSystemdCommandEnv))
	if value == "1" || strings.EqualFold(value, "true") {
		return true
	}
	if strings.TrimSpace(os.Getenv("INVOCATION_ID")) != "" {
		return true
	}
	return strings.TrimSpace(os.Getenv("SYSTEMD_EXEC_PID")) != ""
}

func printUnsupportedSubcommand(name string) {
	fmt.Printf("[kwor] unsupported subcommand: %s\n", name)
	fmt.Println("[kwor] available subcommands: start, stop, uninstall, migrate, uri, setting, admin")
}

func isFirstRun() bool {
	_, err := os.Stat(config.GetDBPath())
	return os.IsNotExist(err)
}

// readInput reads a line from stdin, returns defaultVal when input is empty.
func readInput(prompt string, defaultVal string) string {
	reader := bufio.NewReader(os.Stdin)
	if defaultVal != "" {
		fmt.Printf("%s [default: %s]: ", prompt, defaultVal)
	} else {
		fmt.Printf("%s: ", prompt)
	}
	input, _ := reader.ReadString('\n')
	input = strings.TrimSpace(input)
	if input == "" {
		return defaultVal
	}
	return input
}

func normalizePanelDisplayHost(domain string, listen string) string {
	domain = strings.TrimSpace(domain)
	if domain != "" {
		return domain
	}

	listen = strings.TrimSpace(strings.Trim(listen, "[]"))
	switch listen {
	case "", "0.0.0.0", "::":
		return ""
	default:
		return listen
	}
}

func buildPanelURL(proto string, host string, port int, webPath string) string {
	if !strings.HasPrefix(webPath, "/") {
		webPath = "/" + webPath
	}
	if !strings.HasSuffix(webPath, "/") {
		webPath += "/"
	}
	if strings.Contains(host, ":") && !strings.HasPrefix(host, "[") {
		host = "[" + host + "]"
	}

	portText := fmt.Sprintf(":%d", port)
	if (proto == "https" && port == 443) || (proto == "http" && port == 80) {
		portText = ""
	}
	return fmt.Sprintf("%s://%s%s%s", proto, host, portText, webPath)
}

func collectLocalPanelIPs() ([]string, []string) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil, nil
	}

	ipv4List := make([]string, 0, 8)
	ipv6List := make([]string, 0, 8)
	seen4 := make(map[string]struct{})
	seen6 := make(map[string]struct{})

	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			var ip net.IP
			switch value := addr.(type) {
			case *net.IPNet:
				ip = value.IP
			case *net.IPAddr:
				ip = value.IP
			}
			if ip == nil || ip.IsLoopback() {
				continue
			}

			if ip4 := ip.To4(); ip4 != nil {
				text := ip4.String()
				if _, exists := seen4[text]; exists {
					continue
				}
				seen4[text] = struct{}{}
				ipv4List = append(ipv4List, text)
				continue
			}

			if ip.To16() == nil || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsMulticast() {
				continue
			}
			text := ip.String()
			if zoneIndex := strings.Index(text, "%"); zoneIndex > 0 {
				text = text[:zoneIndex]
			}
			if _, exists := seen6[text]; exists {
				continue
			}
			seen6[text] = struct{}{}
			ipv6List = append(ipv6List, text)
		}
	}

	return ipv4List, ipv6List
}

func collectPanelDisplayHosts(listen string, maxCount int) []string {
	hosts := make([]string, 0, maxCount)
	seen := make(map[string]struct{})
	addHost := func(host string) {
		host = strings.TrimSpace(strings.Trim(host, "[]"))
		if host == "" {
			return
		}
		if zoneIndex := strings.Index(host, "%"); zoneIndex > 0 {
			host = host[:zoneIndex]
		}
		if _, exists := seen[host]; exists {
			return
		}
		seen[host] = struct{}{}
		hosts = append(hosts, host)
	}

	normalizedListen := strings.TrimSpace(strings.Trim(listen, "[]"))
	isWildcardListen := normalizedListen == "" || normalizedListen == "0.0.0.0" || normalizedListen == "::"
	if !isWildcardListen {
		addHost(normalizedListen)
		return hosts
	}

	ipv4List, ipv6List := collectLocalPanelIPs()
	for _, ip := range ipv4List {
		addHost(ip)
		if len(hosts) >= maxCount {
			return hosts
		}
	}
	for _, ip := range ipv6List {
		addHost(ip)
		if len(hosts) >= maxCount {
			return hosts
		}
	}

	if len(hosts) < maxCount {
		addHost(strings.TrimSpace(getPublicIP()))
	}
	if len(hosts) > maxCount {
		return hosts[:maxCount]
	}
	return hosts
}

func printFirstRunPanelURLs() {
	if err := database.InitDB(config.GetDBPath()); err != nil {
		fmt.Printf("[kwor] read panel URL info failed: %v\n", err)
		return
	}

	settingService := service.SettingService{}

	port, err := settingService.GetPort()
	if err != nil || port <= 0 || port > 65535 {
		port = defaultPanelPort
	}
	webPath, err := settingService.GetWebPath()
	if err != nil || strings.TrimSpace(webPath) == "" {
		webPath = defaultPanelPath
	}
	domain, _ := settingService.GetWebDomain()
	listen, _ := settingService.GetListen()
	panelAssignedRecordIDs, _ := service.GetAssignedCertificateRecordIDs(&settingService, service.PanelSelfSignedTargetPanel)

	proto := "http"
	if len(panelAssignedRecordIDs) > 0 {
		proto = "https"
	}

	hosts := collectPanelDisplayHosts(listen, 10)
	if len(hosts) == 0 {
		fallback := normalizePanelDisplayHost(domain, listen)
		if fallback != "" {
			hosts = append(hosts, fallback)
		}
	}
	if len(hosts) == 0 {
		hosts = append(hosts, "<server-ip-or-domain>")
	}

	fmt.Println("[kwor] panel URL(s):")
	for _, host := range hosts {
		fmt.Printf("[kwor] %s\n", buildPanelURL(proto, host, port, webPath))
	}
}

// keep compatibility for internal callers while avoiding username/password output
func printBasicLoginInfo() {
	printFirstRunPanelURLs()
}

func firstRunSetup() {
	fmt.Println("============================================")
	fmt.Println("        kwor first-run setup")
	fmt.Println("============================================")
	fmt.Println()

	portStr := readInput("Enter panel port", strconv.Itoa(defaultPanelPort))
	port, err := strconv.Atoi(portStr)
	if err != nil || port <= 0 || port > 65535 {
		port = defaultPanelPort
		fmt.Printf("[kwor] invalid port, using default: %d\n", port)
	}

	webPath := readInput("Enter panel URL path", defaultPanelPath)
	if !strings.HasPrefix(webPath, "/") {
		webPath = "/" + webPath
	}
	if !strings.HasSuffix(webPath, "/") {
		webPath += "/"
	}

	username := strings.TrimSpace(readInput("Enter admin username", "admin"))
	if username == "" {
		username = "admin"
	}
	password := strings.TrimSpace(readInput("Enter admin password", "admin"))
	if password == "" {
		password = "admin"
	}

	fmt.Println()
	fmt.Println("============================================")
	fmt.Printf("  Port: %d\n", port)
	fmt.Printf("  Path: %s\n", webPath)
	fmt.Println("============================================")
	fmt.Println()

	if err = database.InitDB(config.GetDBPath()); err != nil {
		fmt.Printf("[kwor] init database failed: %v\n", err)
		return
	}

	settingService := service.SettingService{}
	if err = settingService.SetPort(port); err != nil {
		fmt.Printf("[kwor] set port failed: %v\n", err)
	}
	if err = settingService.SetWebPath(webPath); err != nil {
		fmt.Printf("[kwor] set web path failed: %v\n", err)
	}

	userService := service.UserService{}
	if err = userService.UpdateFirstUser(username, password); err != nil {
		fmt.Printf("[kwor] set admin credentials failed: %v\n", err)
	}

	now := time.Now()
	panelResult, panelErr := service.GenerateAndAssignPanelBootstrapCertificate(service.PanelSelfSignedTargetPanel, now)
	subResult, subErr := service.GenerateAndAssignPanelBootstrapCertificate(service.PanelSelfSignedTargetSub, now)

	if panelErr != nil {
		fmt.Printf("[kwor] generate panel self-signed certificate failed: %v\n", panelErr)
	} else {
		if strings.TrimSpace(panelResult.Identity) != "" {
			fmt.Printf("[kwor] panel self-signed identity: %s (%s)\n", panelResult.Identity, panelResult.IdentityKind)
		} else {
			fmt.Printf("[kwor] panel self-signed identity: none (%s)\n", panelResult.DetectionReason)
		}
		fmt.Println("[kwor] panel self-signed certificate issued by certificate center and delegated to managed runtime storage")
		if assignedIDs, assignErr := service.GetAssignedCertificateRecordIDs(&settingService, service.PanelSelfSignedTargetPanel); assignErr != nil {
			fmt.Printf("[kwor] read panel tls assignment failed: %v\n", assignErr)
		} else if len(assignedIDs) == 0 {
			fmt.Println("[kwor] warning: panel certificate issued but assignment is still empty")
		}
	}

	if subErr != nil {
		fmt.Printf("[kwor] generate sub self-signed certificate failed: %v\n", subErr)
	} else {
		if strings.TrimSpace(subResult.Identity) != "" {
			fmt.Printf("[kwor] sub self-signed identity: %s (%s)\n", subResult.Identity, subResult.IdentityKind)
		} else {
			fmt.Printf("[kwor] sub self-signed identity: none (%s)\n", subResult.DetectionReason)
		}
		fmt.Println("[kwor] sub self-signed certificate issued by certificate center and delegated to managed runtime storage")
		if assignedIDs, assignErr := service.GetAssignedCertificateRecordIDs(&settingService, service.PanelSelfSignedTargetSub); assignErr != nil {
			fmt.Printf("[kwor] read sub tls assignment failed: %v\n", assignErr)
		} else if len(assignedIDs) == 0 {
			fmt.Println("[kwor] warning: subscription certificate issued but assignment is still empty")
		}
	}

	if panelErr != nil && subErr != nil {
		fmt.Println("[kwor] continue in HTTP mode")
	} else {
		fmt.Println("[kwor] HTTPS certificate configured")
		if panelErr != nil || subErr != nil {
			fmt.Println("[kwor] one service still uses HTTP until its cert path is fixed")
		}
	}

	fmt.Println("[kwor] first-run setup completed")
	fmt.Println()
}

func handleStart() {
	firstRunInitialized := false

	if isProcessRunning() {
		if isSystemdServiceExists() && isSystemdServiceActive() {
			fmt.Println("[kwor] program already running, systemd service is active")
		} else if isSystemdServiceExists() && !isSystemdServiceActive() {
			fmt.Println("[kwor] program running but systemd service is inactive, starting service...")
			if err := exec.Command("systemctl", "start", kworServiceName).Run(); err != nil {
				fmt.Printf("[kwor] activate systemd service failed: %v\n", err)
				printSystemdDiagnostics(kworServiceName)
			} else if !waitSystemdServiceActive(kworServiceName, 8*time.Second) {
				fmt.Println("[kwor] systemd service did not become active")
				printSystemdDiagnostics(kworServiceName)
			} else {
				fmt.Println("[kwor] systemd service activated")
			}
		} else {
			fmt.Println("[kwor] program running but systemd file missing, creating service...")
			if err := createSystemdService(); err != nil {
				fmt.Printf("[kwor] create systemd service failed: %v\n", err)
				return
			}
			if err := enableAndStartService(); err != nil {
				fmt.Printf("[kwor] register systemd auto-start failed: %v\n", err)
				return
			}
			fmt.Println("[kwor] systemd service created and registered")
		}
		return
	}

	if isFirstRun() {
		firstRunSetup()
		firstRunInitialized = true
	}

	fmt.Println("[kwor] creating systemd service...")
	if err := createSystemdService(); err != nil {
		fmt.Printf("[kwor] create systemd service failed: %v\n", err)
		return
	}

	fmt.Println("[kwor] enabling auto-start and starting service...")
	if err := enableAndStartService(); err != nil {
		fmt.Printf("[kwor] start failed: %v\n", err)
		return
	}

	fmt.Println("[kwor] started successfully, systemd auto-start is registered")
	fmt.Println("[kwor] use 'systemctl status kwor' to check running status")
	if firstRunInitialized {
		printFirstRunPanelURLs()
	}
}

func handleStop() {
	stoppedSomething := false

	panelRunning := isSystemdServiceActive() || isProcessRunning()
	if panelRunning {
		if err := service.MarkPanelStopOnly(); err != nil {
			fmt.Printf("[kwor] prepare panel-only stop failed: %v\n", err)
			return
		}
	}

	if isSystemdServiceActive() {
		fmt.Println("[kwor] stopping kwor service...")
		if err := exec.Command("systemctl", "stop", kworServiceName).Run(); err != nil {
			fmt.Printf("[kwor] stop kwor failed: %v\n", err)
		} else {
			fmt.Println("[kwor] kwor stopped")
			stoppedSomething = true
		}
	} else if isProcessRunning() {
		fmt.Println("[kwor] force stopping kwor process...")
		killProcessesByNameExceptSelf(kworServiceName)
		fmt.Println("[kwor] kwor process stopped")
		stoppedSomething = true
	}

	if isSystemdServiceExists() {
		fmt.Println("[kwor] removing kwor systemd service...")
		exec.Command("systemctl", "disable", kworServiceName).Run()
		os.Remove(getServiceFilePath())
		exec.Command("systemctl", "daemon-reload").Run()
		exec.Command("systemctl", "reset-failed").Run()
		fmt.Println("[kwor] kwor systemd service removed")
		stoppedSomething = true
	}

	service.ClearPanelStopOnlyMarker()

	if !stoppedSomething {
		fmt.Println("[kwor] program is not running, nothing to do")
	} else {
		fmt.Println("[kwor] panel stopped")
	}
}

func resolveCoreManagedConfigPath(coreName string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(coreName)) {
	case "singbox", "sing-box", "default":
		return service.GetSingboxConfigPath(), nil
	case "mihomo":
		return service.GetMihomoConfigPath(), nil
	default:
		return "", fmt.Errorf("unsupported core name: %s", coreName)
	}
}

func materializeCoreConfigForSystemd(coreName string) error {
	configPath, err := resolveCoreManagedConfigPath(coreName)
	if err != nil {
		return err
	}
	normalizedCore := strings.ToLower(strings.TrimSpace(coreName))

	if err = database.InitDB(config.GetDBPath()); err != nil {
		return fmt.Errorf("init database failed: %v", err)
	}
	if err = service.InitManagedRuntimeFileStore(); err != nil {
		return fmt.Errorf("init managed runtime file store failed: %v", err)
	}
	if err = service.EnsureManagedCoreLayout(); err != nil {
		return fmt.Errorf("init managed core layout failed: %v", err)
	}

	// Regenerate latest managed config from DB before materializing to disk.
	// Do not hard-fail on regenerate errors: fall back to last managed config if present.
	var regenerateErr error
	switch normalizedCore {
	case "singbox", "sing-box", "default":
		cfgSvc := service.NewConfigService(nil)
		func() {
			defer func() {
				if recoverErr := recover(); recoverErr != nil {
					regenerateErr = fmt.Errorf("panic during singbox regenerate: %v", recoverErr)
				}
			}()
			service.NewProManagerService(cfgSvc).SaveInboundJson()
		}()
	case "mihomo":
		if regenErr := service.NewMihomoManagerService().RegenerateServerConfig(); regenErr != nil {
			regenerateErr = fmt.Errorf("regenerate mihomo config failed: %v", regenErr)
		}
	}
	if regenerateErr != nil {
		fmt.Printf("[kwor] warning: %v\n", regenerateErr)
	}

	exists, err := service.ManagedRuntimeFileExists(configPath)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("managed config not found: %s", configPath)
	}

	if err = service.MaterializeManagedRuntimeCoreFile(configPath, 15*time.Second); err != nil {
		return fmt.Errorf("materialize core config failed: %v", err)
	}
	if normalizedCore == "singbox" || normalizedCore == "sing-box" || normalizedCore == "default" {
		binName := "sing-box"
		if runtime.GOOS == "windows" {
			binName = "sing-box.exe"
		}
		binPath := filepath.Join(service.GetSingboxCoreDir(), binName)
		if _, statErr := os.Stat(binPath); statErr == nil {
			if checkErr := service.CheckSingboxRuntimeConfig(binPath, configPath, service.GetSingboxCoreDir()); checkErr != nil {
				return checkErr
			}
		}
	}
	return nil
}

func cleanupCoreConfigForSystemd(coreName string) error {
	configPath, err := resolveCoreManagedConfigPath(coreName)
	if err != nil {
		return err
	}
	service.DiscardMaterializedManagedRuntimeCoreFile(configPath)
	return nil
}

func handleSettingSubcommand(args []string) {
	settingFlags := flag.NewFlagSet("setting", flag.ContinueOnError)
	settingFlags.SetOutput(os.Stdout)

	show := settingFlags.Bool("show", false, "show current panel and subscription settings")
	reset := settingFlags.Bool("reset", false, "reset settings to defaults")
	port := settingFlags.Int("port", 0, "set panel port")
	path := settingFlags.String("path", "", "set panel path")
	subPort := settingFlags.Int("subPort", 0, "set subscription port")
	subPath := settingFlags.String("subPath", "", "set subscription path")

	if err := settingFlags.Parse(args); err != nil {
		return
	}

	switch {
	case *show:
		showSetting()
	case *reset:
		resetSetting()
	case settingFlags.NFlag() == 0:
		settingFlags.Usage()
	default:
		updateSetting(*port, *path, *subPort, *subPath)
	}
}

func handleAdminSubcommand(args []string) {
	adminFlags := flag.NewFlagSet("admin", flag.ContinueOnError)
	adminFlags.SetOutput(os.Stdout)

	show := adminFlags.Bool("show", false, "show current admin credentials")
	reset := adminFlags.Bool("reset", false, "reset admin credentials to admin/admin")
	username := adminFlags.String("username", "", "set admin username")
	password := adminFlags.String("password", "", "set admin password")

	if err := adminFlags.Parse(args); err != nil {
		return
	}

	switch {
	case *show:
		showAdmin()
	case *reset:
		resetAdmin()
	case adminFlags.NFlag() == 0:
		adminFlags.Usage()
	default:
		updateAdmin(*username, *password)
	}
}

func ParseCmd() {
	var showVersion bool
	flag.BoolVar(&showVersion, "v", false, "show version")

	oldUsage := flag.Usage
	flag.Usage = func() {
		oldUsage()
		fmt.Println()
		fmt.Println("Commands:")
		fmt.Println("    start          start kwor and register systemd auto-start")
		fmt.Println("    stop           stop kwor panel and remove its systemd auto-start")
		fmt.Println("    resetadmin     interactively reset panel login state")
		fmt.Println("    uninstall      uninstall kwor and cleanup project files")
		fmt.Println("    migrate        migrate the database to the current version")
		fmt.Println("    uri            print current panel access URL(s)")
		fmt.Println("    setting        show or update panel/subscription settings")
		fmt.Println("    admin          show or update admin credentials")
	}

	flag.Parse()
	if showVersion {
		fmt.Println(config.GetName(), "\t", config.GetVersion())
		return
	}

	if len(os.Args) < 2 {
		flag.Usage()
		return
	}

	switch os.Args[1] {
	case "materialize-core-config":
		if !isInternalSystemdCommandAllowed() {
			printUnsupportedSubcommand(os.Args[1])
			os.Exit(2)
		}
		if len(os.Args) < 3 {
			fmt.Println("usage: kwor materialize-core-config <singbox|mihomo>")
			os.Exit(2)
		}
		if err := materializeCoreConfigForSystemd(os.Args[2]); err != nil {
			fmt.Printf("[kwor] materialize core config failed: %v\n", err)
			os.Exit(1)
		}
	case "cleanup-core-config":
		if !isInternalSystemdCommandAllowed() {
			printUnsupportedSubcommand(os.Args[1])
			os.Exit(2)
		}
		if len(os.Args) < 3 {
			fmt.Println("usage: kwor cleanup-core-config <singbox|mihomo>")
			os.Exit(2)
		}
		if err := cleanupCoreConfigForSystemd(os.Args[2]); err != nil {
			fmt.Printf("[kwor] cleanup core config failed: %v\n", err)
			os.Exit(1)
		}
	case "start":
		handleStart()
	case "stop":
		handleStop()
	case "resetadmin":
		handleResetAdminCommand()
	case "uninstall":
		handleUninstallCommand()
	case "migrate":
		migration.MigrateDb()
	case "uri":
		getPanelURI()
	case "setting":
		handleSettingSubcommand(os.Args[2:])
	case "admin":
		handleAdminSubcommand(os.Args[2:])
	default:
		printUnsupportedSubcommand(os.Args[1])
		flag.Usage()
	}
}
