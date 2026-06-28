package service

import (
	"os"
	"runtime"
	"strings"
	"sync"

	"github.com/alireza0/s-ui/util/common"
)

const (
	systemLinuxDNSContentKey          = "systemLinuxDnsContent"
	systemLinuxDNSPathKey             = "systemLinuxDnsPath"
	systemLinuxDNSNameServersInputKey = "systemLinuxDnsNameServersInput"

	defaultSystemLinuxDNSPath = "/etc/resolv.conf"
)

var systemLinuxDNSOptimizationMu sync.Mutex

type SystemLinuxDNSOptimizationService struct {
	SettingService
}

type SystemLinuxDNSOptimizationOverview struct {
	Supported         bool     `json:"supported"`
	ConfigPath        string   `json:"configPath"`
	Content           string   `json:"content"`
	NameServers       []string `json:"nameServers"`
	NameServersInput  string   `json:"nameServersInput"`
	ActiveNameServers []string `json:"activeNameServers"`
	Immutable         bool     `json:"immutable"`
	Error             string   `json:"error,omitempty"`
}

func (s *SystemLinuxDNSOptimizationService) GetOverview() (*SystemLinuxDNSOptimizationOverview, error) {
	overview := &SystemLinuxDNSOptimizationOverview{
		Supported: runtime.GOOS == "linux",
	}

	if !overview.Supported {
		overview.Error = "Linux DNS 修改仅支持 Linux"
		return overview, nil
	}

	path := s.resolveLinuxDNSConfigPath()
	overview.ConfigPath = path

	content, err := s.getString(systemLinuxDNSContentKey)
	if err != nil {
		return nil, err
	}
	content = normalizeManagedLinuxDNSContent(content)

	activeNameServers := make([]string, 0)
	if pathEntryExists(path) {
		raw, readErr := os.ReadFile(path)
		if readErr != nil {
			overview.Error = common.NewError("读取 resolv.conf 失败: ", readErr).Error()
		} else {
			activeNameServers = extractActiveLinuxNameServers(string(raw))
			content = normalizeManagedLinuxDNSContent(string(raw))
			if content != "" {
				if err := s.setString(systemLinuxDNSContentKey, content); err != nil {
					return nil, err
				}
			}
		}
	} else if strings.TrimSpace(content) == "" {
		overview.Error = "未找到 /etc/resolv.conf"
	}

	overview.Content = content
	overview.NameServers = extractActiveLinuxNameServers(content)
	nameServersInput, inputErr := s.resolveManagedLinuxDNSNameServersInput(content, overview.NameServers)
	if inputErr != nil {
		return nil, inputErr
	}
	overview.NameServersInput = nameServersInput
	overview.ActiveNameServers = activeNameServers

	if pathEntryExists(path) {
		immutable, immutableErr := detectFileImmutable(path)
		if immutableErr == nil {
			overview.Immutable = immutable
		}
	} else if strings.TrimSpace(overview.Error) == "" {
		overview.Error = "未找到 /etc/resolv.conf"
	}

	return overview, nil
}

func (s *SystemLinuxDNSOptimizationService) SaveContent(content string) error {
	systemLinuxDNSOptimizationMu.Lock()
	defer systemLinuxDNSOptimizationMu.Unlock()

	if runtime.GOOS != "linux" {
		return common.NewError("Linux DNS 修改仅支持 Linux")
	}

	normalized := normalizeManagedLinuxDNSContent(content)
	if strings.TrimSpace(normalized) == "" {
		return common.NewError("resolv.conf 内容不能为空")
	}

	path, err := s.applyManagedLinuxDNSContentLocked(normalized)
	if err != nil {
		return err
	}
	if err := s.setString(systemLinuxDNSContentKey, normalized); err != nil {
		return err
	}
	nameServers := extractActiveLinuxNameServers(normalized)
	nameServersInput, inputErr := s.resolveManagedLinuxDNSNameServersInput(normalized, nameServers)
	if inputErr != nil {
		return inputErr
	}
	if err := s.persistManagedLinuxDNSNameServersInput(normalized, nameServers, nameServersInput); err != nil {
		return err
	}
	return s.setString(systemLinuxDNSPathKey, path)
}

func (s *SystemLinuxDNSOptimizationService) SaveNameServers(nameServersText string) error {
	systemLinuxDNSOptimizationMu.Lock()
	defer systemLinuxDNSOptimizationMu.Unlock()

	if runtime.GOOS != "linux" {
		return common.NewError("Linux DNS 修改仅支持 Linux")
	}

	path := s.resolveLinuxDNSConfigPath()
	baseContent, err := s.loadCurrentLinuxDNSContent(path)
	if err != nil {
		return err
	}

	nameServers := normalizeLinuxNameServerInput(nameServersText)
	nextContent := replaceActiveLinuxNameServers(baseContent, nameServers)

	appliedPath, err := s.applyManagedLinuxDNSContentLocked(nextContent)
	if err != nil {
		return err
	}
	if err := s.setString(systemLinuxDNSContentKey, nextContent); err != nil {
		return err
	}
	if err := s.persistManagedLinuxDNSNameServersInput(nextContent, nameServers, nameServersText); err != nil {
		return err
	}
	return s.setString(systemLinuxDNSPathKey, appliedPath)
}

func (s *SystemLinuxDNSOptimizationService) applyManagedLinuxDNSContentLocked(content string) (string, error) {
	path := s.resolveLinuxDNSConfigPath()
	if strings.TrimSpace(path) == "" {
		return "", common.NewError("resolv.conf 路径为空")
	}

	content = normalizeManagedLinuxDNSContent(content)
	if err := rewriteManagedFileWithImmutable(path, content, managedFileRewriteOptions{
		DisplayName:                      "resolv.conf",
		IgnoreUnsupportedUnlockOnSymlink: true,
	}); err != nil {
		return "", err
	}

	if err := s.setString(systemLinuxDNSPathKey, path); err != nil {
		return "", err
	}
	return path, nil
}

func (s *SystemLinuxDNSOptimizationService) resolveLinuxDNSConfigPath() string {
	savedPath, err := s.getString(systemLinuxDNSPathKey)
	if err == nil {
		savedPath = strings.TrimSpace(savedPath)
		if savedPath != "" {
			return savedPath
		}
	}
	return defaultSystemLinuxDNSPath
}

func normalizeManagedLinuxDNSContent(content string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")
	if strings.TrimSpace(content) == "" {
		return ""
	}
	content = strings.TrimRight(content, "\n")
	return content + "\n"
}

func extractActiveLinuxNameServers(content string) []string {
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	lines := strings.Split(normalized, "\n")
	servers := make([]string, 0)
	for _, line := range lines {
		server, ok := parseActiveLinuxNameServerLine(line)
		if ok {
			servers = append(servers, server)
		}
	}
	return servers
}

func parseActiveLinuxNameServerLine(line string) (string, bool) {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") {
		return "", false
	}

	withoutComment := trimmed
	if idx := strings.Index(withoutComment, "#"); idx >= 0 {
		withoutComment = strings.TrimSpace(withoutComment[:idx])
	}
	if withoutComment == "" {
		return "", false
	}

	fields := strings.Fields(withoutComment)
	if len(fields) < 2 {
		return "", false
	}
	if !strings.EqualFold(fields[0], "nameserver") {
		return "", false
	}

	server := strings.TrimSpace(fields[1])
	if server == "" {
		return "", false
	}
	return server, true
}

func normalizeLinuxNameServerInput(raw string) []string {
	normalized := strings.ReplaceAll(raw, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	normalized = strings.ReplaceAll(normalized, ",", " ")
	fields := strings.Fields(normalized)
	result := make([]string, 0, len(fields))
	for _, item := range fields {
		v := strings.TrimSpace(item)
		if v == "" {
			continue
		}
		result = append(result, v)
	}
	return result
}

func normalizeLinuxNameServerInputText(raw string) string {
	normalized := strings.ReplaceAll(raw, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	normalized = strings.ReplaceAll(normalized, ",", " ")
	lines := strings.Split(normalized, "\n")
	rebuilt := make([]string, 0, len(lines))
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		rebuilt = append(rebuilt, strings.Join(fields, " "))
	}
	return strings.Join(rebuilt, "\n")
}

func buildLinuxDNSNameServersInputFromContent(content string, nameServers []string) string {
	if len(nameServers) == 0 {
		return ""
	}

	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	lines := strings.Split(normalized, "\n")

	resultLines := make([]string, 0, len(nameServers))
	seen := 0
	for _, line := range lines {
		server, ok := parseActiveLinuxNameServerLine(line)
		if !ok {
			continue
		}

		resultLines = append(resultLines, server)
		seen++
		if seen >= len(nameServers) {
			break
		}
	}

	if len(resultLines) == 0 {
		return strings.Join(nameServers, " ")
	}
	return strings.Join(resultLines, "\n")
}

func areLinuxNameServerListsEqual(left []string, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func (s *SystemLinuxDNSOptimizationService) resolveManagedLinuxDNSNameServersInput(content string, nameServers []string) (string, error) {
	savedInput, err := s.getString(systemLinuxDNSNameServersInputKey)
	if err != nil {
		return "", err
	}

	normalizedSavedInput := normalizeLinuxNameServerInputText(savedInput)
	if normalizedSavedInput != "" {
		if areLinuxNameServerListsEqual(normalizeLinuxNameServerInput(normalizedSavedInput), nameServers) {
			return normalizedSavedInput, nil
		}
		if err := s.setString(systemLinuxDNSNameServersInputKey, ""); err != nil {
			return "", err
		}
	}

	rebuilt := buildLinuxDNSNameServersInputFromContent(content, nameServers)
	if err := s.persistManagedLinuxDNSNameServersInput(content, nameServers, rebuilt); err != nil {
		return "", err
	}
	return rebuilt, nil
}

func (s *SystemLinuxDNSOptimizationService) persistManagedLinuxDNSNameServersInput(content string, nameServers []string, rawInput string) error {
	normalizedInput := normalizeLinuxNameServerInputText(rawInput)
	if len(nameServers) == 0 {
		return s.setString(systemLinuxDNSNameServersInputKey, "")
	}

	if !areLinuxNameServerListsEqual(normalizeLinuxNameServerInput(normalizedInput), nameServers) {
		normalizedInput = buildLinuxDNSNameServersInputFromContent(content, nameServers)
	}

	return s.setString(systemLinuxDNSNameServersInputKey, normalizedInput)
}

func replaceActiveLinuxNameServers(content string, nameServers []string) string {
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	normalized = strings.ReplaceAll(normalized, "\r", "\n")
	lines := strings.Split(normalized, "\n")

	rebuilt := make([]string, 0, len(lines)+len(nameServers))
	inserted := false
	for _, line := range lines {
		if _, ok := parseActiveLinuxNameServerLine(line); ok {
			if !inserted {
				for _, server := range nameServers {
					rebuilt = append(rebuilt, "nameserver "+server)
				}
				inserted = true
			}
			continue
		}
		rebuilt = append(rebuilt, line)
	}

	if !inserted {
		for len(rebuilt) > 0 && strings.TrimSpace(rebuilt[len(rebuilt)-1]) == "" {
			rebuilt = rebuilt[:len(rebuilt)-1]
		}
		for _, server := range nameServers {
			rebuilt = append(rebuilt, "nameserver "+server)
		}
	}

	return normalizeManagedLinuxDNSContent(strings.Join(rebuilt, "\n"))
}

func (s *SystemLinuxDNSOptimizationService) loadCurrentLinuxDNSContent(path string) (string, error) {
	if pathEntryExists(path) {
		raw, err := os.ReadFile(path)
		if err != nil {
			return "", common.NewError("读取 resolv.conf 失败: ", err)
		}
		return normalizeManagedLinuxDNSContent(string(raw)), nil
	}

	content, err := s.getString(systemLinuxDNSContentKey)
	if err != nil {
		return "", err
	}
	return normalizeManagedLinuxDNSContent(content), nil
}

func pathEntryExists(path string) bool {
	path = strings.TrimSpace(path)
	if path == "" {
		return false
	}
	_, err := os.Lstat(path)
	return err == nil
}

func isPathSymlink(path string) bool {
	info, err := os.Lstat(path)
	if err != nil {
		return false
	}
	return info.Mode()&os.ModeSymlink != 0
}

func isImmutableUnsupportedError(err error) bool {
	if err == nil {
		return false
	}
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "operation not supported") || strings.Contains(text, "inappropriate ioctl")
}
