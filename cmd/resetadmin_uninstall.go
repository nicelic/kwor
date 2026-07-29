package cmd

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/alireza0/s-ui/config"
	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/service"
)

var uninstallLegacyManagedSystemdServices = []string{
	"sing-box",
	"singbox",
	"s-ui-singbox",
	"sui-singbox",
	"mihomo",
	"metacubex-mihomo",
	"s-ui-mihomo",
	"sui-mihomo",
	"kwor-mtu-opt",
}

var uninstallSysctlManagedFiles = []string{
	"/etc/sysctl.d/99-s-ui-optimize.conf",
	"/etc/sysctl.d/99-kwor-optimize.conf",
}

var uninstallJournaldManagedCandidates = []string{
	"/etc/systemd/journald.conf",
	"/usr/local/etc/systemd/journald.conf",
	"/usr/lib/systemd/journald.conf",
	"/lib/systemd/journald.conf",
}

func isKworRunning() bool {
	return isProcessRunning() || isManagedPanelSystemdServiceActive()
}

func readPortWithDefault(defaultPort int) int {
	defaultPortText := strconv.Itoa(defaultPort)
	for {
		portText := readInput("\u8bf7\u8f93\u5165\u9762\u677f\u7aef\u53e3(\u56de\u8f66\u4fdd\u7559\u539f\u503c)", defaultPortText)
		port, err := strconv.Atoi(strings.TrimSpace(portText))
		if err == nil && port >= 1 && port <= 65535 {
			return port
		}
		fmt.Println("[kwor] invalid port, please input 1-65535")
	}
}

func normalizeWebPath(path string) string {
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	if !strings.HasSuffix(path, "/") {
		path += "/"
	}
	return path
}

func handleResetAdminCommand() {
	if !isKworRunning() {
		fmt.Println("[kwor] \u7a0b\u5e8f\u672a\u8fd0\u884c\uff0c\u8bf7\u5148\u6267\u884c ./kwor start")
		return
	}

	confirm := strings.TrimSpace(readInput("\u4f60\u662f\u5426\u8981\u91cd\u7f6e\u767b\u5f55\u72b6\u6001\uff1f(y/N)", ""))
	if confirm != "y" && confirm != "Y" {
		fmt.Println("[kwor] \u5df2\u53d6\u6d88\u91cd\u7f6e")
		return
	}

	verifyWord := strings.TrimSpace(readInput("\u8bf7\u8f93\u5165kwor", ""))
	if verifyWord != kworServiceName {
		fmt.Println("[kwor] \u8f93\u5165\u9519\u8bef\uff0c\u5df2\u53d6\u6d88\u91cd\u7f6e")
		return
	}

	if err := database.InitDB(config.GetDBPath()); err != nil {
		fmt.Println("[kwor] init db failed:", err)
		return
	}

	settingService := service.SettingService{}
	userService := service.UserService{}

	currentPort, err := settingService.GetPort()
	if err != nil || currentPort <= 0 || currentPort > 65535 {
		currentPort = defaultPanelPort
	}

	currentWebPath, err := settingService.GetWebPath()
	if err != nil || strings.TrimSpace(currentWebPath) == "" {
		currentWebPath = defaultPanelPath
	}

	currentUsername := "admin"
	currentPassword := "admin"
	user, err := userService.GetFirstUser()
	if err == nil && user != nil {
		if strings.TrimSpace(user.Username) != "" {
			currentUsername = strings.TrimSpace(user.Username)
		}
		if strings.TrimSpace(user.Password) != "" {
			currentPassword = user.Password
		}
	} else if err != nil && !database.IsNotFound(err) {
		fmt.Println("[kwor] read current admin failed:", err)
		return
	}

	fmt.Println("[kwor] Enter keeps current value.")
	newPort := readPortWithDefault(currentPort)
	newWebPath := normalizeWebPath(readInput("\u8bf7\u8f93\u5165\u9762\u677fURL\u8def\u5f84(\u56de\u8f66\u4fdd\u7559\u539f\u503c)", currentWebPath))
	newUsername := strings.TrimSpace(readInput("\u8bf7\u8f93\u5165\u7ba1\u7406\u5458\u7528\u6237\u540d(\u56de\u8f66\u4fdd\u7559\u539f\u503c)", currentUsername))
	if newUsername == "" {
		newUsername = currentUsername
	}

	newPassword := strings.TrimSpace(readInput("\u8bf7\u8f93\u5165\u7ba1\u7406\u5458\u5bc6\u7801(\u56de\u8f66\u4fdd\u7559\u539f\u503c)", ""))
	if newPassword == "" {
		newPassword = currentPassword
	}

	if err = settingService.SetPort(newPort); err != nil {
		fmt.Println("[kwor] reset port failed:", err)
		return
	}
	if err = settingService.SetWebPath(newWebPath); err != nil {
		fmt.Println("[kwor] reset path failed:", err)
		return
	}
	if err = userService.UpdateFirstUser(newUsername, newPassword); err != nil {
		fmt.Println("[kwor] reset admin failed:", err)
		return
	}

	fmt.Println("[kwor] \u91cd\u7f6e\u5b8c\u6210")
	fmt.Printf("[kwor] port: %d\n", newPort)
	fmt.Printf("[kwor] path: %s\n", newWebPath)
	fmt.Printf("[kwor] username: %s\n", newUsername)
	fmt.Printf("[kwor] password: %s\n", newPassword)
	printBasicLoginInfo()

	if newPort != currentPort || newWebPath != currentWebPath {
		fmt.Println("[kwor] \u7aef\u53e3\u6216URL\u8def\u5f84\u5df2\u66f4\u6539\uff0c\u91cd\u542f\u540e\u751f\u6548")
	}
}

var waitBeforePanelConfirmedUninstall = time.Sleep
var executeKworUninstall = performKworUninstall

func handleUninstallCommand(args []string) error {
	panelConfirmed := false
	switch {
	case len(args) == 0:
		// Keep the terminal flow unchanged for normal SSH usage.
	case len(args) == 1 && args[0] == service.PanelUninstallConfirmedArgument:
		panelConfirmed = true
	default:
		return fmt.Errorf("usage: kwor uninstall")
	}

	if service.RunningInsideDocker() {
		printDockerUninstallGuide()
		return nil
	}
	if service.RunningInsideContainer() {
		fmt.Println("[kwor] uninstall is disabled inside generic containers.")
		fmt.Println("[kwor] Stop and remove the container from its host. This command will not remove container resources.")
		return nil
	}

	if panelConfirmed {
		return runPanelConfirmedUninstall()
	}

	return runTerminalUninstall(readInput)
}

func runTerminalUninstall(input func(prompt string, defaultVal string) string) error {
	confirm := strings.TrimSpace(input("是否停止kwor运行并卸载、删除其创建的全部文件？(y/N)", ""))
	if confirm != "y" && confirm != "Y" {
		fmt.Println("[kwor] \u5df2\u53d6\u6d88\u5378\u8f7d")
		return nil
	}

	verifyWord := strings.TrimSpace(input("\u8bf7\u8f93\u5165kwor", ""))
	if verifyWord != kworServiceName {
		fmt.Println("[kwor] \u8f93\u5165\u9519\u8bef\uff0c\u5df2\u53d6\u6d88\u5378\u8f7d")
		return nil
	}

	return executeKworUninstall()
}

func runPanelConfirmedUninstall() error {
	// The HTTP response is sent before this independent process reaches the
	// established CLI cleanup path.
	waitBeforePanelConfirmedUninstall(2 * time.Second)
	return executeKworUninstall()
}

// performKworUninstall is the sole implementation for both terminal and
// panel-confirmed requests. The service layer owns ordering, verification and
// the persistent retry state so the CLI cannot delete data before cores stop.
func performKworUninstall() error {
	options := buildKworUninstallOptions()
	report, err := service.PerformKworUninstall(options)
	if report != nil {
		for _, warning := range report.Warnings {
			fmt.Println("[kwor] uninstall warning:", warning)
		}
		for _, failure := range report.Failures {
			fmt.Println("[kwor] uninstall failure:", failure)
		}
	}
	if err != nil {
		fmt.Println("[kwor] uninstall did not finish; ownership state has been kept for retry:", err)
		return err
	}
	fmt.Println("[kwor] uninstall completed")
	return nil
}

func printDockerUninstallGuide() {
	fmt.Println("[kwor] Docker deployment must be removed on the host. The container never receives Docker daemon access.")
	for _, instruction := range service.DockerUninstallHostCommands() {
		fmt.Printf("[kwor] host uninstall command (%s):\n%s\n", instruction.ID, instruction.Command)
	}
}

func buildKworUninstallOptions() service.KworUninstallOptions {
	binDir := getBinDir()
	return service.KworUninstallOptions{
		PanelBinaryPath:    getBinPath(),
		PanelBinDir:        binDir,
		DataDir:            config.GetDataDir(),
		DatabasePath:       config.GetDBPath(),
		LegacyDatabasePath: filepath.Join(binDir, "db", config.GetName()+".db"),
		PanelServiceName:   kworServiceName,
		RuntimePaths: []string{
			filepath.Join(binDir, "install.sh"),
			filepath.Join(binDir, "kwor.service"),
			config.GetRuntimeInstallScriptPath(),
			config.GetRuntimeServiceFilePath(),
			filepath.Join(config.GetRuntimeSupportDir(), "panel-update-last.log"),
			filepath.Join(binDir, "s-ui.service"),
		},
	}
}

func shouldRemoveInstallDirAfterUninstall(dir string) bool {
	// Never remove the binary's parent directory during uninstall.
	// Uninstall should only remove project-created files, not the container dir
	// that may also hold user-managed files or deployment artifacts.
	_ = dir
	return false
}

func isProtectedInstallDir(dir string) bool {
	normalized := filepath.ToSlash(filepath.Clean(dir))
	protectedDirs := map[string]struct{}{
		"/":                {},
		"/bin":             {},
		"/sbin":            {},
		"/usr/bin":         {},
		"/usr/sbin":        {},
		"/usr/local/bin":   {},
		"/usr/local/sbin":  {},
		"/etc":             {},
		"/var":             {},
		"/home":            {},
		"/root":            {},
		"C:/":              {},
		"C:/Windows":       {},
		"C:/Program Files": {},
	}
	_, ok := protectedDirs[normalized]
	return ok
}
