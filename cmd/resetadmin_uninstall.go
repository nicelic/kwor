package cmd

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
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
	return isProcessRunning() || isSystemdServiceActive()
}

func killProcessesByNameExceptSelf(processName string) bool {
	out, err := exec.Command("pgrep", "-x", processName).Output()
	if err != nil {
		return false
	}

	selfPid := strconv.Itoa(os.Getpid())
	killed := false
	for _, pid := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		pid = strings.TrimSpace(pid)
		if pid == "" || pid == selfPid {
			continue
		}
		exec.Command("kill", "-TERM", pid).Run()
		killed = true
	}
	if !killed {
		return false
	}

	time.Sleep(2 * time.Second)

	out, err = exec.Command("pgrep", "-x", processName).Output()
	if err != nil {
		return true
	}
	for _, pid := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		pid = strings.TrimSpace(pid)
		if pid == "" || pid == selfPid {
			continue
		}
		exec.Command("kill", "-KILL", pid).Run()
	}

	return true
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

func handleUninstallCommand() {
	confirm := strings.TrimSpace(readInput("是否停止kwor运行并卸载、删除其创建的全部文件？(y/N)", ""))
	if confirm != "y" && confirm != "Y" {
		fmt.Println("[kwor] \u5df2\u53d6\u6d88\u5378\u8f7d")
		return
	}

	verifyWord := strings.TrimSpace(readInput("\u8bf7\u8f93\u5165kwor", ""))
	if verifyWord != kworServiceName {
		fmt.Println("[kwor] \u8f93\u5165\u9519\u8bef\uff0c\u5df2\u53d6\u6d88\u5378\u8f7d")
		return
	}

	if isKworRunning() || isSingboxServiceActive() || isMihomoServiceActive() || isNamedProcessRunning("sing-box") || isNamedProcessRunning("mihomo") {
		fmt.Println("[kwor] service running, execute ./kwor stop first")
		handleStop()
	} else {
		fmt.Println("[kwor] \u7a0b\u5e8f\u672a\u8fd0\u884c\uff0c\u5f00\u59cb\u6e05\u7406")
	}

	if err := database.InitDB(config.GetDBPath()); err == nil {
		if vnstatErr := (&service.TrafficOverviewService{}).RemoveManagedVnstatForUninstall(); vnstatErr != nil {
			fmt.Println("[kwor] cleanup vnstat failed:", vnstatErr)
		}
		if _, removeErr := (&service.AcmeService{}).RemoveManagedAcmeForUninstall(); removeErr != nil {
			fmt.Println("[kwor] cleanup acme failed:", removeErr)
		}
	} else {
		fmt.Println("[kwor] init db for acme cleanup failed:", err)
		if vnstatErr := (&service.TrafficOverviewService{}).RemoveManagedVnstatForUninstall(); vnstatErr != nil {
			fmt.Println("[kwor] cleanup vnstat failed:", vnstatErr)
		}
	}

	cleanupSystemdArtifacts()
	cleanupRuntimeArtifacts()

	fmt.Println("[kwor] uninstall completed")
	os.Exit(0)
}

func cleanupSystemdArtifacts() {
	if runtime.GOOS == "linux" {
		serviceNames := []string{
			kworServiceName,
			service.GetSingboxSystemdName(),
			service.GetMihomoSystemdName(),
		}
		serviceNames = append(serviceNames, uninstallLegacyManagedSystemdServices...)
		for _, serviceName := range uniqueStrings(serviceNames) {
			removeSystemdServiceArtifacts(serviceName)
		}
		exec.Command("systemctl", "daemon-reload").Run()
		exec.Command("systemctl", "reset-failed").Run()
	}
	service.CleanupManagedNftablesOnShutdown()
}

func cleanupRuntimeArtifacts() {
	cleanupLinuxManagedArtifacts()

	binDir := getBinDir()
	dbPath := config.GetDBPath()
	legacyDBPath := filepath.Join(binDir, "db", config.GetName()+".db")
	dataDir := config.GetDataDir()

	paths := []string{
		dataDir,
		dbPath,
		dbPath + "-wal",
		dbPath + "-shm",
		dbPath + ".backup",
		dbPath + ".temp",
		legacyDBPath,
		legacyDBPath + "-wal",
		legacyDBPath + "-shm",
		legacyDBPath + ".backup",
		legacyDBPath + ".temp",
		filepath.Join(binDir, "kwor.service"),
		filepath.Join(binDir, "s-ui.service"),
		"/usr/bin/kwor",
		"/usr/local/bin/kwor",
	}
	paths = append(paths, executablePathsForCleanup()...)

	for _, path := range uniquePaths(paths) {
		removePathIfExists(path)
	}

	removeIfEmpty(filepath.Dir(legacyDBPath))
	cleanupDBDirIfSafe(filepath.Dir(dbPath))
}

func executablePathsForCleanup() []string {
	candidates := make([]string, 0, 4)
	if execPath, err := os.Executable(); err == nil {
		candidates = append(candidates, execPath)
		if realPath, err := filepath.EvalSymlinks(execPath); err == nil {
			candidates = append(candidates, realPath)
		}
	}
	candidates = append(candidates, getBinPath())
	return candidates
}

func uniquePaths(paths []string) []string {
	seen := make(map[string]struct{})
	result := make([]string, 0, len(paths))
	for _, p := range paths {
		cleaned := filepath.Clean(strings.TrimSpace(p))
		if cleaned == "" || cleaned == "." {
			continue
		}
		if _, ok := seen[cleaned]; ok {
			continue
		}
		seen[cleaned] = struct{}{}
		result = append(result, cleaned)
	}
	return result
}

func uniqueStrings(values []string) []string {
	seen := make(map[string]struct{})
	result := make([]string, 0, len(values))
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed == "" {
			continue
		}
		if _, ok := seen[trimmed]; ok {
			continue
		}
		seen[trimmed] = struct{}{}
		result = append(result, trimmed)
	}
	return result
}

func cleanupLinuxManagedArtifacts() {
	if runtime.GOOS != "linux" {
		return
	}

	unlockOnly := []string{
		"/etc/resolv.conf",
		"/etc/sysctl.conf",
	}
	unlockOnly = append(unlockOnly, uninstallJournaldManagedCandidates...)
	unlockOnly = append(unlockOnly, uninstallSysctlManagedFiles...)
	for _, path := range uniquePaths(unlockOnly) {
		clearImmutableBeforeRemoval(path)
	}

	for _, path := range uniquePaths(uninstallSysctlManagedFiles) {
		removePathIfExists(path)
	}

	removePathIfExists(filepath.Join(config.GetDataDir(), "mtu", "_set_mtu_.sh"))
}

func removeSystemdServiceArtifacts(serviceName string) {
	serviceName = strings.TrimSpace(serviceName)
	if serviceName == "" {
		return
	}

	exec.Command("systemctl", "stop", serviceName).Run()
	exec.Command("systemctl", "disable", serviceName).Run()

	for _, servicePath := range systemdServiceFileCandidates(serviceName) {
		removePathIfExists(servicePath)
	}
}

func systemdServiceFileCandidates(serviceName string) []string {
	fileName := strings.TrimSpace(serviceName) + ".service"
	if fileName == ".service" {
		return nil
	}
	return []string{
		filepath.Join("/etc/systemd/system", fileName),
		filepath.Join("/lib/systemd/system", fileName),
		filepath.Join("/usr/lib/systemd/system", fileName),
	}
}

func clearImmutableBeforeRemoval(path string) {
	if runtime.GOOS != "linux" {
		return
	}

	path = strings.TrimSpace(path)
	if path == "" || !pathExists(path) {
		return
	}

	chattrPath, err := exec.LookPath("chattr")
	if err != nil {
		return
	}

	info, err := os.Lstat(path)
	if err != nil {
		return
	}

	args := []string{"-i", path}
	if info.IsDir() {
		args = []string{"-R", "-i", path}
	}
	exec.Command(chattrPath, args...).Run()
}

func removePathIfExists(path string) {
	if path == "" || !pathExists(path) {
		return
	}
	clearImmutableBeforeRemoval(path)
	if err := os.RemoveAll(path); err != nil {
		fmt.Printf("[kwor] remove failed: %s (%v)\n", path, err)
		return
	}
	fmt.Printf("[kwor] removed: %s\n", path)
}

func pathExists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

func removeIfEmpty(dir string) {
	if dir == "" || !pathExists(dir) {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		return
	}
	if err = os.Remove(dir); err == nil {
		fmt.Printf("[kwor] removed empty dir: %s\n", dir)
	}
}

func cleanupDBDirIfSafe(dir string) {
	if dir == "" || !pathExists(dir) {
		return
	}
	cleaned := filepath.Clean(dir)
	if isProtectedInstallDir(cleaned) {
		return
	}
	dataDir := filepath.Clean(config.GetDataDir())
	cleanedSlash := filepath.ToSlash(cleaned)
	dataDirSlash := filepath.ToSlash(dataDir)
	if strings.HasPrefix(cleanedSlash, dataDirSlash+"/") {
		removeIfEmpty(cleaned)
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
