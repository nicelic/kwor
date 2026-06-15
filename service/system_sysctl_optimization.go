package service

import (
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/alireza0/s-ui/util/common"
)

const (
	systemSysctlEnabledKey = "systemSysctlEnabled"
	systemSysctlContentKey = "systemSysctlContent"
	systemSysctlPathKey    = "systemSysctlPath"

	sysctlManagedDropInPath = "/etc/sysctl.d/99-s-ui-optimize.conf"
	sysctlManagedMainPath   = "/etc/sysctl.conf"
)

const defaultSystemSysctlContent = `net.core.default_qdisc=cake
net.ipv4.tcp_congestion_control=bbr

net.ipv4.tcp_window_scaling=1

# 3.6MB, 36MB, 360MB
net.ipv4.tcp_rmem=3600000 36000000 360000000
net.ipv4.tcp_wmem=3600000 36000000 360000000

vm.swappiness=100
`

var (
	systemSysctlOptimizationMu sync.Mutex

	sysctlManagedPaths = []string{
		sysctlManagedDropInPath,
		sysctlManagedMainPath,
	}
)

type SystemSysctlOptimizationService struct {
	SettingService
}

type SystemSysctlOptimizationOverview struct {
	Supported  bool   `json:"supported"`
	Enabled    bool   `json:"enabled"`
	ConfigPath string `json:"configPath"`
	Content    string `json:"content"`
	Immutable  bool   `json:"immutable"`
	Error      string `json:"error,omitempty"`
}

func (s *SystemSysctlOptimizationService) GetOverview() (*SystemSysctlOptimizationOverview, error) {
	content, err := s.getString(systemSysctlContentKey)
	if err != nil {
		return nil, err
	}
	enabled, err := s.getBool(systemSysctlEnabledKey)
	if err != nil {
		return nil, err
	}

	overview := &SystemSysctlOptimizationOverview{
		Supported: runtime.GOOS == "linux",
		Enabled:   enabled,
		Content:   content,
	}

	if !overview.Supported {
		overview.Error = "sysctl 优化仅支持 Linux"
		return overview, nil
	}

	paths := resolveSysctlManagedPaths()
	overview.ConfigPath = formatManagedSysctlPathList(paths)

	lockedAll := true
	missingPaths := make([]string, 0)
	for _, path := range paths {
		if !pathEntryExists(path) {
			lockedAll = false
			missingPaths = append(missingPaths, path)
			continue
		}
		immutable, immutableErr := detectFileImmutable(path)
		if immutableErr != nil || !immutable {
			lockedAll = false
		}
	}
	overview.Immutable = lockedAll

	if enabled && len(missingPaths) > 0 {
		overview.Error = "sysctl 托管文件缺失: " + strings.Join(missingPaths, ", ")
	}

	return overview, nil
}

func (s *SystemSysctlOptimizationService) SetEnabled(enabled bool) error {
	systemSysctlOptimizationMu.Lock()
	defer systemSysctlOptimizationMu.Unlock()

	if runtime.GOOS != "linux" {
		return common.NewError("sysctl 优化仅支持 Linux")
	}

	if enabled {
		content, err := s.getString(systemSysctlContentKey)
		if err != nil {
			return err
		}
		content = normalizeManagedSysctlContent(content)
		paths, err := s.applyManagedSysctlContentLocked(content)
		if err != nil {
			return err
		}
		if err := applySysctlFromManagedFiles(paths); err != nil {
			return err
		}
		if err := s.setString(systemSysctlEnabledKey, "true"); err != nil {
			return err
		}
		return s.setString(systemSysctlPathKey, formatManagedSysctlPathList(paths))
	}

	if err := unlockManagedSysctlFiles(resolveSysctlManagedPaths()); err != nil {
		return err
	}

	if err := s.setString(systemSysctlPathKey, formatManagedSysctlPathList(resolveSysctlManagedPaths())); err != nil {
		return err
	}
	return s.setString(systemSysctlEnabledKey, "false")
}

func (s *SystemSysctlOptimizationService) SaveContent(content string) error {
	systemSysctlOptimizationMu.Lock()
	defer systemSysctlOptimizationMu.Unlock()

	if runtime.GOOS != "linux" {
		return common.NewError("sysctl 优化仅支持 Linux")
	}

	normalized := normalizeManagedSysctlContent(content)
	if strings.TrimSpace(normalized) == "" {
		return common.NewError("sysctl 配置内容不能为空")
	}

	paths, err := s.applyManagedSysctlContentLocked(normalized)
	if err != nil {
		return err
	}
	if err := applySysctlFromManagedFiles(paths); err != nil {
		return err
	}
	if err := s.setString(systemSysctlContentKey, normalized); err != nil {
		return err
	}
	if err := s.setString(systemSysctlEnabledKey, "true"); err != nil {
		return err
	}
	return s.setString(systemSysctlPathKey, formatManagedSysctlPathList(paths))
}

func (s *SystemSysctlOptimizationService) ResetContent() error {
	return s.SaveContent(defaultSystemSysctlContent)
}

func (s *SystemSysctlOptimizationService) ReconcileOnStartup() error {
	systemSysctlOptimizationMu.Lock()
	defer systemSysctlOptimizationMu.Unlock()

	if runtime.GOOS != "linux" {
		return nil
	}

	enabled, err := s.getBool(systemSysctlEnabledKey)
	if err != nil {
		return err
	}
	paths := resolveSysctlManagedPaths()

	if !enabled {
		if err := unlockManagedSysctlFiles(paths); err != nil {
			return err
		}
		return s.setString(systemSysctlEnabledKey, "false")
	}

	if allManagedSysctlPathsLocked(paths) {
		return nil
	}

	content, err := s.getString(systemSysctlContentKey)
	if err != nil {
		return err
	}
	content = normalizeManagedSysctlContent(content)

	appliedPaths, err := s.applyManagedSysctlContentLocked(content)
	if err != nil {
		return err
	}
	if err := applySysctlFromManagedFiles(appliedPaths); err != nil {
		return err
	}
	if err := s.setString(systemSysctlPathKey, formatManagedSysctlPathList(appliedPaths)); err != nil {
		return err
	}
	return s.setString(systemSysctlEnabledKey, "true")
}

func (s *SystemSysctlOptimizationService) applyManagedSysctlContentLocked(content string) ([]string, error) {
	content = normalizeManagedSysctlContent(content)
	if strings.TrimSpace(content) == "" {
		return nil, common.NewError("sysctl 配置内容不能为空")
	}

	paths := resolveSysctlManagedPaths()
	for _, path := range paths {
		if err := rewriteManagedFileWithImmutable(path, content, managedFileRewriteOptions{
			DisplayName: "sysctl 配置",
		}); err != nil {
			return nil, err
		}
	}

	pathValue := formatManagedSysctlPathList(paths)
	if err := s.setString(systemSysctlPathKey, pathValue); err != nil {
		return nil, err
	}
	return paths, nil
}

func resolveSysctlManagedPaths() []string {
	result := make([]string, 0, len(sysctlManagedPaths))
	seen := make(map[string]struct{}, len(sysctlManagedPaths))
	for _, rawPath := range sysctlManagedPaths {
		path := strings.TrimSpace(rawPath)
		if path == "" {
			continue
		}
		path = filepath.Clean(path)
		if _, exists := seen[path]; exists {
			continue
		}
		seen[path] = struct{}{}
		result = append(result, path)
	}
	return result
}

func formatManagedSysctlPathList(paths []string) string {
	return strings.Join(paths, ", ")
}

func unlockManagedSysctlFiles(paths []string) error {
	for _, path := range paths {
		if !pathEntryExists(path) {
			continue
		}
		if err := clearManagedFileImmutableFlag(path, "sysctl 配置", managedFileRewriteOptions{}); err != nil {
			return err
		}
	}
	return nil
}

func allManagedSysctlPathsLocked(paths []string) bool {
	if len(paths) == 0 {
		return false
	}
	for _, path := range paths {
		if !pathEntryExists(path) {
			return false
		}
		immutable, immutableErr := detectFileImmutable(path)
		if immutableErr != nil || !immutable {
			return false
		}
	}
	return true
}

func normalizeManagedSysctlContent(content string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")
	if strings.TrimSpace(content) == "" {
		content = defaultSystemSysctlContent
	}
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	return content
}

func applySysctlFromManagedFiles(paths []string) error {
	sysctlPath, err := exec.LookPath("sysctl")
	if err != nil {
		return common.NewError("未找到 sysctl 命令")
	}

	pathsByPriority := make([]string, 0, len(paths))
	pathSet := make(map[string]struct{}, len(paths))
	for _, path := range paths {
		normalized := filepath.Clean(strings.TrimSpace(path))
		if normalized == "" {
			continue
		}
		pathSet[normalized] = struct{}{}
	}
	if _, ok := pathSet[sysctlManagedMainPath]; ok {
		pathsByPriority = append(pathsByPriority, sysctlManagedMainPath)
	}
	if _, ok := pathSet[sysctlManagedDropInPath]; ok {
		pathsByPriority = append(pathsByPriority, sysctlManagedDropInPath)
	}
	for _, path := range paths {
		normalized := filepath.Clean(strings.TrimSpace(path))
		if normalized == "" {
			continue
		}
		if normalized == sysctlManagedMainPath || normalized == sysctlManagedDropInPath {
			continue
		}
		pathsByPriority = append(pathsByPriority, normalized)
	}

	attempts := make([]string, 0)
	appendAttempt := func(prefix string, attemptErr error) {
		if attemptErr == nil {
			return
		}
		attempts = append(attempts, prefix+": "+strings.TrimSpace(attemptErr.Error()))
	}

	allDirectSucceeded := true
	for _, path := range pathsByPriority {
		if attemptErr := runCommandWithTimeout(12*time.Second, sysctlPath, "-p", path); attemptErr != nil {
			allDirectSucceeded = false
			appendAttempt("sysctl -p "+path, attemptErr)
		}
	}
	if allDirectSucceeded {
		return nil
	}

	if attemptErr := runCommandWithTimeout(18*time.Second, sysctlPath, "--system"); attemptErr == nil {
		return nil
	} else {
		appendAttempt("sysctl --system", attemptErr)
	}

	if attemptErr := restartSysctlService(); attemptErr == nil {
		return nil
	} else {
		appendAttempt("restart sysctl service", attemptErr)
	}

	if len(attempts) == 0 {
		return common.NewError("应用 sysctl 配置失败: 请检查参数是否受当前内核支持")
	}
	return common.NewError("应用 sysctl 配置失败: ", strings.Join(attempts, " | "))
}

func resolveSysctlServiceCandidates() []string {
	family := strings.TrimSpace(detectLinuxSystemFamily())
	switch family {
	case "debian":
		return []string{"systemd-sysctl", "procps"}
	case "rhel", "suse", "arch":
		return []string{"systemd-sysctl"}
	case "alpine":
		return []string{"procps", "systemd-sysctl"}
	default:
		return []string{"systemd-sysctl", "procps"}
	}
}

func restartSysctlService() error {
	attempts := make([]string, 0)
	appendAttempt := func(prefix string, attemptErr error) {
		if attemptErr == nil {
			return
		}
		attempts = append(attempts, prefix+": "+strings.TrimSpace(attemptErr.Error()))
	}

	serviceNames := resolveSysctlServiceCandidates()
	if len(serviceNames) == 0 {
		serviceNames = []string{"systemd-sysctl", "procps"}
	}

	if systemctlPath, err := exec.LookPath("systemctl"); err == nil {
		for _, serviceName := range serviceNames {
			commandErr := runCommandWithTimeout(12*time.Second, systemctlPath, "restart", serviceName)
			if commandErr == nil {
				return nil
			}
			appendAttempt("systemctl restart "+serviceName, commandErr)
		}
	}

	if servicePath, err := exec.LookPath("service"); err == nil {
		for _, serviceName := range serviceNames {
			commandErr := runCommandWithTimeout(12*time.Second, servicePath, serviceName, "restart")
			if commandErr == nil {
				return nil
			}
			appendAttempt("service "+serviceName+" restart", commandErr)
		}
	}

	if openrcPath, err := exec.LookPath("rc-service"); err == nil {
		for _, serviceName := range serviceNames {
			commandErr := runCommandWithTimeout(12*time.Second, openrcPath, serviceName, "restart")
			if commandErr == nil {
				return nil
			}
			appendAttempt("rc-service "+serviceName+" restart", commandErr)
		}
	}

	if runitPath, err := exec.LookPath("sv"); err == nil {
		for _, serviceName := range serviceNames {
			commandErr := runCommandWithTimeout(12*time.Second, runitPath, "restart", serviceName)
			if commandErr == nil {
				return nil
			}
			appendAttempt("sv restart "+serviceName, commandErr)
		}
	}

	for _, serviceName := range serviceNames {
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
		return common.NewError("未找到可用的 sysctl 服务管理命令（systemctl/service/rc-service/sv）")
	}
	return common.NewError("重启 sysctl 服务失败: ", strings.Join(attempts, " | "))
}
