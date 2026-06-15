package service

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/alireza0/s-ui/util/common"
)

const (
	systemLogDisableEnabledKey  = "systemLogDisableEnabled"
	systemLogJournaldContentKey = "systemLogJournaldContent"
	systemLogJournaldPathKey    = "systemLogJournaldPath"
)

const defaultSystemLogJournaldContent = `#/etc/systemd/journald.conf
[Journal]
Storage=none
SystemMaxUse=0
RuntimeMaxUse=0
RateLimitIntervalSec=30s
RateLimitBurst=0
ReadKMsg=no
ForwardToKMsg=no
`

var (
	systemLogOptimizationMu sync.Mutex

	journaldConfigCandidates = []string{
		"/etc/systemd/journald.conf",
		"/usr/local/etc/systemd/journald.conf",
		"/usr/lib/systemd/journald.conf",
		"/lib/systemd/journald.conf",
	}

	journaldServiceCandidates = []string{
		"systemd-journald",
		"journald",
	}
)

type SystemLogOptimizationService struct {
	SettingService
}

type SystemLogOptimizationOverview struct {
	Supported  bool   `json:"supported"`
	Enabled    bool   `json:"enabled"`
	ConfigPath string `json:"configPath"`
	Content    string `json:"content"`
	Immutable  bool   `json:"immutable"`
	Error      string `json:"error,omitempty"`
}

func (s *SystemLogOptimizationService) GetOverview() (*SystemLogOptimizationOverview, error) {
	content, err := s.getString(systemLogJournaldContentKey)
	if err != nil {
		return nil, err
	}
	enabled, err := s.getBool(systemLogDisableEnabledKey)
	if err != nil {
		return nil, err
	}

	overview := &SystemLogOptimizationOverview{
		Supported: runtime.GOOS == "linux",
		Enabled:   enabled,
		Content:   content,
	}

	if !overview.Supported {
		overview.Error = "系统日志优化仅支持 Linux"
		return overview, nil
	}

	path, pathErr := s.resolveJournaldConfigPath(false)
	if pathErr == nil {
		overview.ConfigPath = path
		immutable, immutableErr := detectFileImmutable(path)
		if immutableErr == nil {
			overview.Immutable = immutable
		}
	} else if enabled {
		overview.Error = strings.TrimSpace(pathErr.Error())
	}

	return overview, nil
}

func (s *SystemLogOptimizationService) SetDisabled(enabled bool) error {
	systemLogOptimizationMu.Lock()
	defer systemLogOptimizationMu.Unlock()

	if runtime.GOOS != "linux" {
		return common.NewError("系统日志优化仅支持 Linux")
	}

	if enabled {
		content, err := s.getString(systemLogJournaldContentKey)
		if err != nil {
			return err
		}
		content = normalizeManagedJournaldContent(content)
		path, err := s.applyManagedJournaldContentLocked(content)
		if err != nil {
			return err
		}
		if err := restartJournaldService(); err != nil {
			return err
		}
		if err := s.setString(systemLogDisableEnabledKey, "true"); err != nil {
			return err
		}
		return s.setString(systemLogJournaldPathKey, path)
	}

	path, pathErr := s.resolveJournaldConfigPath(false)
	if pathErr == nil && pathEntryExists(path) {
		if err := clearManagedFileImmutableFlag(path, "journald 配置", managedFileRewriteOptions{}); err != nil {
			return err
		}
	}

	return s.setString(systemLogDisableEnabledKey, "false")
}

func (s *SystemLogOptimizationService) SaveContent(content string) error {
	systemLogOptimizationMu.Lock()
	defer systemLogOptimizationMu.Unlock()

	if runtime.GOOS != "linux" {
		return common.NewError("系统日志优化仅支持 Linux")
	}

	normalized := normalizeManagedJournaldContent(content)
	if strings.TrimSpace(normalized) == "" {
		return common.NewError("journald 配置内容不能为空")
	}

	path, err := s.applyManagedJournaldContentLocked(normalized)
	if err != nil {
		return err
	}
	if err := restartJournaldService(); err != nil {
		return err
	}
	if err := s.setString(systemLogJournaldContentKey, normalized); err != nil {
		return err
	}
	if err := s.setString(systemLogDisableEnabledKey, "true"); err != nil {
		return err
	}
	return s.setString(systemLogJournaldPathKey, path)
}

func (s *SystemLogOptimizationService) ResetContent() error {
	return s.SaveContent(defaultSystemLogJournaldContent)
}

func (s *SystemLogOptimizationService) ReconcileOnStartup() error {
	systemLogOptimizationMu.Lock()
	defer systemLogOptimizationMu.Unlock()

	if runtime.GOOS != "linux" {
		return nil
	}

	enabled, err := s.getBool(systemLogDisableEnabledKey)
	if err != nil {
		return err
	}

	path, resolveErr := s.resolveJournaldConfigPath(true)
	if resolveErr != nil {
		if enabled {
			return resolveErr
		}
		return s.setString(systemLogDisableEnabledKey, "false")
	}

	if !enabled {
		if pathEntryExists(path) {
			if err := clearManagedFileImmutableFlag(path, "journald 配置", managedFileRewriteOptions{}); err != nil {
				return err
			}
		}
		return s.setString(systemLogDisableEnabledKey, "false")
	}

	locked := false
	if pathEntryExists(path) {
		immutable, immutableErr := detectFileImmutable(path)
		locked = immutableErr == nil && immutable
	}
	if locked {
		return nil
	}

	content, err := s.getString(systemLogJournaldContentKey)
	if err != nil {
		return err
	}
	content = normalizeManagedJournaldContent(content)

	appliedPath, err := s.applyManagedJournaldContentLocked(content)
	if err != nil {
		return err
	}
	if err := restartJournaldService(); err != nil {
		return err
	}
	if err := s.setString(systemLogJournaldPathKey, appliedPath); err != nil {
		return err
	}
	return s.setString(systemLogDisableEnabledKey, "true")
}

func (s *SystemLogOptimizationService) applyManagedJournaldContentLocked(content string) (string, error) {
	path, err := s.resolveJournaldConfigPath(true)
	if err != nil {
		return "", err
	}

	content = normalizeManagedJournaldContent(content)
	if strings.TrimSpace(content) == "" {
		return "", common.NewError("journald 配置内容不能为空")
	}

	if err := rewriteManagedFileWithImmutable(path, content, managedFileRewriteOptions{
		DisplayName: "journald 配置",
	}); err != nil {
		return "", err
	}

	if err := s.setString(systemLogJournaldPathKey, path); err != nil {
		return "", err
	}
	return path, nil
}

func (s *SystemLogOptimizationService) resolveJournaldConfigPath(writeIntent bool) (string, error) {
	savedPath, err := s.getString(systemLogJournaldPathKey)
	if err == nil {
		savedPath = strings.TrimSpace(savedPath)
		if savedPath != "" {
			if pathExists(savedPath) {
				return savedPath, nil
			}
			if writeIntent {
				dir := filepath.Dir(savedPath)
				if dir != "" {
					return savedPath, nil
				}
			}
		}
	}

	for _, candidate := range journaldConfigCandidates {
		if pathExists(candidate) {
			return candidate, nil
		}
	}

	if writeIntent {
		for _, candidate := range journaldConfigCandidates {
			dir := filepath.Dir(candidate)
			if pathExists(dir) {
				return candidate, nil
			}
		}
		return journaldConfigCandidates[0], nil
	}

	return "", common.NewError("未找到 journald 配置文件路径")
}

func normalizeManagedJournaldContent(content string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")
	if strings.TrimSpace(content) == "" {
		content = defaultSystemLogJournaldContent
	}
	if !strings.HasSuffix(content, "\n") {
		content += "\n"
	}
	return content
}

func detectFileImmutable(path string) (bool, error) {
	if !pathExists(path) {
		return false, nil
	}
	lsattrPath, err := exec.LookPath("lsattr")
	if err != nil {
		return false, nil
	}
	output, err := runCommandOutputWithTimeout(8*time.Second, lsattrPath, path)
	if err != nil {
		return false, nil
	}
	fields := strings.Fields(output)
	if len(fields) == 0 {
		return false, nil
	}
	return strings.Contains(fields[0], "i"), nil
}

func runCommandOutputWithTimeout(timeout time.Duration, command string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, command, args...)
	output, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return "", fmt.Errorf("command timed out (%s %s)", command, strings.Join(args, " "))
	}
	if err != nil {
		trimmed := strings.TrimSpace(string(output))
		if trimmed == "" {
			return "", err
		}
		return "", fmt.Errorf("%w: %s", err, trimmed)
	}
	return string(output), nil
}

func restartJournaldService() error {
	attempts := make([]string, 0)
	appendAttempt := func(prefix string, err error) {
		if err == nil {
			return
		}
		attempts = append(attempts, prefix+": "+strings.TrimSpace(err.Error()))
	}

	serviceNames := resolveJournaldServiceCandidates()
	if len(serviceNames) == 0 {
		serviceNames = journaldServiceCandidates
	}

	if systemctlPath, err := exec.LookPath("systemctl"); err == nil {
		actions := []string{"restart", "reload-or-restart", "try-restart"}
		for _, action := range actions {
			for _, serviceName := range serviceNames {
				commandErr := runCommandWithTimeout(12*time.Second, systemctlPath, action, serviceName)
				if commandErr == nil {
					return nil
				}
				appendAttempt("systemctl "+action+" "+serviceName, commandErr)
			}
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
		return common.NewError("未找到可用的 journald 服务管理命令（systemctl/service/rc-service/sv）")
	}
	return common.NewError("重启 journald 失败: ", strings.Join(attempts, " | "))
}

func resolveJournaldServiceCandidates() []string {
	family := strings.TrimSpace(detectLinuxSystemFamily())
	switch family {
	case "debian", "rhel", "suse", "arch":
		return []string{"systemd-journald", "journald"}
	case "alpine":
		return []string{"journald", "systemd-journald"}
	default:
		return []string{"systemd-journald", "journald"}
	}
}
