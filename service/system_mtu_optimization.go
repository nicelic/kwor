package service

import (
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alireza0/s-ui/config"
	"github.com/alireza0/s-ui/util/common"
)

const (
	systemMTUEnabledKey    = "systemMTUEnabled"
	systemMTUValueKey      = "systemMTUValue"
	systemMTUScriptPathKey = "systemMTUScriptPath"

	defaultSystemMTUValue = 1500
	minAllowedMTUValue    = 576
	maxAllowedMTUValue    = 9500

	managedMTUScriptFileName = "_set_mtu_.sh"
	managedMTUServiceUnit    = "kwor-mtu-opt.service"
	managedMTUServicePath    = "/etc/systemd/system/" + managedMTUServiceUnit
)

var (
	systemMTUOptimizationMu sync.Mutex
	mtuPattern              = regexp.MustCompile(`\bmtu\s+(\d+)\b`)
)

type SystemMTUOptimizationService struct {
	SettingService
}

type SystemMTUOptimizationOverview struct {
	Supported         bool   `json:"supported"`
	Enabled           bool   `json:"enabled"`
	Interface         string `json:"interface"`
	CurrentMTU        int    `json:"currentMtu"`
	MTU               int    `json:"mtu"`
	ScriptPath        string `json:"scriptPath"`
	ScriptExists      bool   `json:"scriptExists"`
	ServiceName       string `json:"serviceName"`
	ServicePath       string `json:"servicePath"`
	ServiceRegistered bool   `json:"serviceRegistered"`
	ServiceEnabled    bool   `json:"serviceEnabled"`
	ServiceActive     string `json:"serviceActive"`
	Error             string `json:"error,omitempty"`
}

func (s *SystemMTUOptimizationService) GetOverview() (*SystemMTUOptimizationOverview, error) {
	enabled, err := s.getBool(systemMTUEnabledKey)
	if err != nil {
		return nil, err
	}

	storedMTU, mtuErr := s.getStoredMTU()
	if mtuErr != nil {
		storedMTU = defaultSystemMTUValue
	}

	scriptPath := s.resolveMTUScriptPath()
	overview := &SystemMTUOptimizationOverview{
		Supported:         runtime.GOOS == "linux",
		Enabled:           enabled,
		MTU:               storedMTU,
		ScriptPath:        scriptPath,
		ScriptExists:      pathEntryExists(scriptPath),
		ServiceName:       managedMTUServiceUnit,
		ServicePath:       managedMTUServicePath,
		ServiceRegistered: pathEntryExists(managedMTUServicePath),
	}

	if !overview.Supported {
		overview.Error = "MTU 优化仅支持 Linux"
		return overview, nil
	}

	issues := make([]string, 0, 4)
	if mtuErr != nil {
		issues = append(issues, strings.TrimSpace(mtuErr.Error()))
	}

	iface, detectErr := detectDefaultInterfaceName()
	if detectErr != nil {
		issues = append(issues, "默认网卡检测失败: "+strings.TrimSpace(detectErr.Error()))
	} else {
		overview.Interface = iface
		currentMTU, currentErr := detectInterfaceMTUValue(iface)
		if currentErr != nil {
			issues = append(issues, "读取网卡 MTU 失败: "+strings.TrimSpace(currentErr.Error()))
		} else {
			overview.CurrentMTU = currentMTU
		}
	}

	systemctlPath, systemctlErr := exec.LookPath("systemctl")
	if systemctlErr == nil {
		state, stateErr := readSystemdUnitFileState(systemctlPath, managedMTUServiceUnit)
		if stateErr != nil {
			issues = append(issues, "读取 systemd 注册状态失败: "+strings.TrimSpace(stateErr.Error()))
		} else {
			overview.ServiceEnabled = strings.EqualFold(state, "enabled")
			if strings.TrimSpace(state) != "" {
				overview.ServiceRegistered = overview.ServiceRegistered || !strings.EqualFold(state, "not-found")
			}
		}

		activeState, activeErr := readSystemdUnitActiveState(systemctlPath, managedMTUServiceUnit)
		if activeErr == nil {
			overview.ServiceActive = activeState
		}
	} else if enabled {
		issues = append(issues, "未找到 systemctl，无法检测开机启动注册状态")
	}

	if enabled {
		if !overview.ScriptExists {
			issues = append(issues, "MTU 脚本不存在，请重新保存 MTU 或重新开启开关")
		}
		if !overview.ServiceRegistered || !overview.ServiceEnabled {
			issues = append(issues, "systemd 开机自启未注册，将在下次保存或重新开启时自动补注册")
		}
	}

	if len(issues) > 0 {
		overview.Error = strings.Join(issues, " | ")
	}
	return overview, nil
}

func (s *SystemMTUOptimizationService) SetEnabled(enabled bool, requestedMTU *int) error {
	systemMTUOptimizationMu.Lock()
	defer systemMTUOptimizationMu.Unlock()

	if runtime.GOOS != "linux" {
		return common.NewError("MTU 优化仅支持 Linux")
	}

	if enabled {
		targetMTU := defaultSystemMTUValue
		if requestedMTU != nil {
			targetMTU = *requestedMTU
		} else if iface, detectErr := detectDefaultInterfaceName(); detectErr == nil {
			if currentMTU, currentErr := detectInterfaceMTUValue(iface); currentErr == nil {
				targetMTU = currentMTU
			} else if storedMTU, storedErr := s.getStoredMTU(); storedErr == nil {
				targetMTU = storedMTU
			}
		} else if storedMTU, storedErr := s.getStoredMTU(); storedErr == nil {
			targetMTU = storedMTU
		}
		return s.enableMTULocked(targetMTU)
	}

	return s.disableMTULocked()
}

func (s *SystemMTUOptimizationService) SaveMTU(mtu int) error {
	systemMTUOptimizationMu.Lock()
	defer systemMTUOptimizationMu.Unlock()

	if runtime.GOOS != "linux" {
		return common.NewError("MTU 优化仅支持 Linux")
	}
	if err := validateMTUValue(mtu); err != nil {
		return err
	}

	enabled, err := s.getBool(systemMTUEnabledKey)
	if err != nil {
		return err
	}
	if !enabled {
		return s.setString(systemMTUValueKey, strconv.Itoa(mtu))
	}

	return s.enableMTULocked(mtu)
}

func (s *SystemMTUOptimizationService) enableMTULocked(mtu int) error {
	if err := validateMTUValue(mtu); err != nil {
		return err
	}

	scriptPath, err := s.rebuildManagedMTUScriptLocked(mtu)
	if err != nil {
		return err
	}
	if err := runManagedMTUScript(scriptPath, mtu); err != nil {
		return common.NewError("执行 MTU 脚本失败: ", err)
	}
	if err := ensureSystemdMTUService(scriptPath); err != nil {
		return err
	}

	if err := s.setString(systemMTUValueKey, strconv.Itoa(mtu)); err != nil {
		return err
	}
	if err := s.setString(systemMTUEnabledKey, "true"); err != nil {
		return err
	}
	return s.setString(systemMTUScriptPathKey, scriptPath)
}

func (s *SystemMTUOptimizationService) disableMTULocked() error {
	errs := make([]error, 0, 4)

	iface, ifaceErr := detectDefaultInterfaceName()
	if ifaceErr == nil {
		if err := setInterfaceMTUValue(iface, defaultSystemMTUValue); err != nil {
			errs = append(errs, common.NewError("回退 MTU=1500 失败: ", err))
		}
	} else {
		errs = append(errs, common.NewError("默认网卡检测失败: ", ifaceErr))
	}

	if err := removeSystemdMTUService(); err != nil {
		errs = append(errs, err)
	}
	scriptPath := s.resolveMTUScriptPath()
	if err := removeManagedMTUScript(scriptPath); err != nil {
		errs = append(errs, err)
	}

	if err := s.setString(systemMTUEnabledKey, "false"); err != nil {
		errs = append(errs, err)
	}
	if err := s.setString(systemMTUValueKey, strconv.Itoa(defaultSystemMTUValue)); err != nil {
		errs = append(errs, err)
	}
	if err := s.setString(systemMTUScriptPathKey, scriptPath); err != nil {
		errs = append(errs, err)
	}

	return joinMTUErrors(errs)
}

func (s *SystemMTUOptimizationService) getStoredMTU() (int, error) {
	raw, err := s.getString(systemMTUValueKey)
	if err != nil {
		return 0, err
	}
	return parseAndValidateMTU(raw)
}

func (s *SystemMTUOptimizationService) resolveMTUScriptPath() string {
	savedPath, err := s.getString(systemMTUScriptPathKey)
	if err == nil {
		savedPath = strings.TrimSpace(savedPath)
		if savedPath != "" {
			return savedPath
		}
	}
	return filepath.Join(config.GetDataDir(), "mtu", managedMTUScriptFileName)
}

func (s *SystemMTUOptimizationService) rebuildManagedMTUScriptLocked(mtu int) (string, error) {
	scriptPath := s.resolveMTUScriptPath()
	scriptPath = strings.TrimSpace(scriptPath)
	if scriptPath == "" {
		return "", common.NewError("MTU 脚本路径为空")
	}

	if err := os.MkdirAll(filepath.Dir(scriptPath), 0o755); err != nil {
		return "", common.NewError("创建 MTU 脚本目录失败: ", err)
	}
	if pathEntryExists(scriptPath) {
		if err := os.Remove(scriptPath); err != nil && !os.IsNotExist(err) {
			return "", common.NewError("删除旧 MTU 脚本失败: ", err)
		}
	}

	content := buildManagedMTUScriptContent(mtu)
	if err := os.WriteFile(scriptPath, []byte(content), 0o755); err != nil {
		return "", common.NewError("写入 MTU 脚本失败: ", err)
	}
	if err := os.Chmod(scriptPath, 0o755); err != nil {
		return "", common.NewError("设置 MTU 脚本执行权限失败: ", err)
	}
	if err := s.setString(systemMTUScriptPathKey, scriptPath); err != nil {
		return "", err
	}
	return scriptPath, nil
}

func removeManagedMTUScript(scriptPath string) error {
	scriptPath = strings.TrimSpace(scriptPath)
	if scriptPath == "" {
		return nil
	}

	if pathEntryExists(scriptPath) {
		if err := os.Remove(scriptPath); err != nil && !os.IsNotExist(err) {
			return common.NewError("删除 MTU 脚本失败: ", err)
		}
	}

	scriptDir := filepath.Dir(scriptPath)
	entries, err := os.ReadDir(scriptDir)
	if err == nil && len(entries) == 0 {
		_ = os.Remove(scriptDir)
	}
	return nil
}

func buildManagedMTUScriptContent(mtu int) string {
	return strings.TrimSpace(
		`#!/bin/sh
set -eu

MTU_VALUE="`+strconv.Itoa(mtu)+`"
if [ "${1:-}" != "" ]; then
  MTU_VALUE="$1"
fi

case "$MTU_VALUE" in
  ''|*[!0-9]*)
    echo "invalid MTU value: $MTU_VALUE" >&2
    exit 1
    ;;
esac

detect_default_iface() {
  iface=""
  if command -v ip >/dev/null 2>&1; then
    iface="$(ip -o route show to default 2>/dev/null | awk '{for(i=1;i<=NF;i++){if($i=="dev"){print $(i+1); exit}}}')"
    iface="${iface%%@*}"
    if [ -n "${iface:-}" ]; then
      echo "$iface"
      return 0
    fi

    iface="$(ip -o link show up 2>/dev/null | awk -F': ' '$2 != "lo" {print $2; exit}')"
    iface="${iface%%@*}"
    if [ -n "${iface:-}" ]; then
      echo "$iface"
      return 0
    fi
  fi

  if [ -r /proc/net/route ]; then
    iface="$(awk 'NR>1 && $2=="00000000" {print $1; exit}' /proc/net/route)"
    iface="${iface%%@*}"
    if [ -n "${iface:-}" ]; then
      echo "$iface"
      return 0
    fi
  fi

  return 1
}

IFACE="$(detect_default_iface || true)"
if [ -z "${IFACE:-}" ]; then
  echo "failed to detect default network interface" >&2
  exit 1
fi

if command -v ip >/dev/null 2>&1; then
  ip link set dev "$IFACE" mtu "$MTU_VALUE"
elif command -v ifconfig >/dev/null 2>&1; then
  ifconfig "$IFACE" mtu "$MTU_VALUE" up
else
  echo "missing network command: neither ip nor ifconfig was found" >&2
  exit 1
fi
`,
	) + "\n"
}

func runManagedMTUScript(scriptPath string, mtu int) error {
	shellPath, err := resolveManagedScriptShell()
	if err != nil {
		return err
	}
	return runCommandWithTimeout(20*time.Second, shellPath, scriptPath, strconv.Itoa(mtu))
}

func resolveManagedScriptShell() (string, error) {
	candidates := []string{"/bin/bash", "/bin/sh"}
	for _, candidate := range candidates {
		if pathExists(candidate) {
			return candidate, nil
		}
	}
	if bashPath, err := exec.LookPath("bash"); err == nil {
		return bashPath, nil
	}
	if shPath, err := exec.LookPath("sh"); err == nil {
		return shPath, nil
	}
	return "", common.NewError("未找到可用 shell（bash/sh）")
}

func ensureSystemdMTUService(scriptPath string) error {
	systemctlPath, err := exec.LookPath("systemctl")
	if err != nil {
		return common.NewError("未找到 systemctl，无法注册 MTU 开机自启")
	}

	serviceContent, err := buildManagedMTUServiceContent(scriptPath)
	if err != nil {
		return err
	}
	if err := os.WriteFile(managedMTUServicePath, []byte(serviceContent), 0o644); err != nil {
		return common.NewError("写入 MTU systemd 服务失败: ", err)
	}

	if err := runCommandWithTimeout(12*time.Second, systemctlPath, "daemon-reload"); err != nil {
		return common.NewError("重新加载 systemd 失败: ", err)
	}
	if err := runCommandWithTimeout(12*time.Second, systemctlPath, "enable", managedMTUServiceUnit); err != nil {
		return common.NewError("注册 MTU systemd 开机自启失败: ", err)
	}
	return nil
}

func buildManagedMTUServiceContent(scriptPath string) (string, error) {
	shellPath, err := resolveManagedScriptShell()
	if err != nil {
		return "", err
	}
	return `[Unit]
Description=kwor managed default interface MTU
Wants=network-online.target
After=network-online.target

[Service]
Type=oneshot
ExecStartPre=/bin/sleep 10
ExecStart=` + shellPath + ` "` + scriptPath + `"
RemainAfterExit=yes

[Install]
WantedBy=multi-user.target
`, nil
}

func removeSystemdMTUService() error {
	systemctlPath, systemctlErr := exec.LookPath("systemctl")
	if systemctlErr == nil {
		_ = runCommandWithTimeout(12*time.Second, systemctlPath, "disable", "--now", managedMTUServiceUnit)
	}

	if pathEntryExists(managedMTUServicePath) {
		if err := os.Remove(managedMTUServicePath); err != nil && !os.IsNotExist(err) {
			return common.NewError("删除 MTU systemd 服务文件失败: ", err)
		}
	}

	if systemctlErr == nil {
		if err := runCommandWithTimeout(12*time.Second, systemctlPath, "daemon-reload"); err != nil {
			return common.NewError("删除 MTU 服务后重新加载 systemd 失败: ", err)
		}
		_ = runCommandWithTimeout(8*time.Second, systemctlPath, "reset-failed", managedMTUServiceUnit)
	}
	return nil
}

func readSystemdUnitFileState(systemctlPath string, unit string) (string, error) {
	output, err := runCommandOutputWithTimeout(8*time.Second, systemctlPath, "show", "-p", "UnitFileState", "--value", unit)
	if err != nil {
		return "", err
	}
	state := strings.TrimSpace(output)
	if state == "" {
		state = "unknown"
	}
	return state, nil
}

func readSystemdUnitActiveState(systemctlPath string, unit string) (string, error) {
	output, err := runCommandOutputWithTimeout(8*time.Second, systemctlPath, "show", "-p", "ActiveState", "--value", unit)
	if err != nil {
		return "", err
	}
	state := strings.TrimSpace(output)
	if state == "" {
		state = "unknown"
	}
	return state, nil
}

func detectDefaultInterfaceName() (string, error) {
	if ipPath, err := exec.LookPath("ip"); err == nil {
		output, routeErr := runCommandOutputWithTimeout(8*time.Second, ipPath, "-o", "route", "show", "to", "default")
		if routeErr == nil {
			iface := parseMTUDefaultInterfaceFromIPRouteOutput(output)
			if iface != "" {
				return iface, nil
			}
		}
	}

	if raw, err := os.ReadFile("/proc/net/route"); err == nil {
		iface := parseMTUDefaultInterfaceFromProcRoute(string(raw))
		if iface != "" {
			return iface, nil
		}
	}

	ifaces, err := net.Interfaces()
	if err != nil {
		return "", err
	}
	for _, iface := range ifaces {
		name := strings.TrimSpace(iface.Name)
		if name == "" || name == "lo" {
			continue
		}
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		return name, nil
	}
	for _, iface := range ifaces {
		name := strings.TrimSpace(iface.Name)
		if name == "" || name == "lo" {
			continue
		}
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		return name, nil
	}

	return "", common.NewError("未检测到可用默认网卡")
}

func parseMTUDefaultInterfaceFromIPRouteOutput(output string) string {
	lines := strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n")
	for _, line := range lines {
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 3 {
			continue
		}
		for i := 0; i < len(fields)-1; i++ {
			if fields[i] != "dev" {
				continue
			}
			iface := sanitizeInterfaceName(fields[i+1])
			if iface != "" && iface != "lo" {
				return iface
			}
		}
	}
	return ""
}

func parseMTUDefaultInterfaceFromProcRoute(raw string) string {
	lines := strings.Split(strings.ReplaceAll(raw, "\r\n", "\n"), "\n")
	for i, line := range lines {
		if i == 0 {
			continue
		}
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) < 2 {
			continue
		}
		if fields[1] != "00000000" {
			continue
		}
		iface := sanitizeInterfaceName(fields[0])
		if iface != "" && iface != "lo" {
			return iface
		}
	}
	return ""
}

func sanitizeInterfaceName(name string) string {
	name = strings.TrimSpace(name)
	name = strings.Trim(name, "\"'")
	if idx := strings.Index(name, "@"); idx > 0 {
		name = name[:idx]
	}
	return strings.TrimSpace(name)
}

func detectInterfaceMTUValue(iface string) (int, error) {
	iface = sanitizeInterfaceName(iface)
	if iface == "" {
		return 0, common.NewError("网卡名称为空")
	}

	ifaceInfo, err := net.InterfaceByName(iface)
	if err == nil && ifaceInfo.MTU > 0 {
		return ifaceInfo.MTU, nil
	}

	raw, readErr := os.ReadFile(filepath.Join("/sys/class/net", iface, "mtu"))
	if readErr == nil {
		mtu, parseErr := strconv.Atoi(strings.TrimSpace(string(raw)))
		if parseErr == nil && mtu > 0 {
			return mtu, nil
		}
	}

	if ipPath, pathErr := exec.LookPath("ip"); pathErr == nil {
		output, cmdErr := runCommandOutputWithTimeout(8*time.Second, ipPath, "link", "show", "dev", iface)
		if cmdErr == nil {
			match := mtuPattern.FindStringSubmatch(output)
			if len(match) == 2 {
				mtu, parseErr := strconv.Atoi(match[1])
				if parseErr == nil && mtu > 0 {
					return mtu, nil
				}
			}
		}
	}

	return 0, common.NewError("无法读取网卡 ", iface, " 的 MTU")
}

func setInterfaceMTUValue(iface string, mtu int) error {
	iface = sanitizeInterfaceName(iface)
	if iface == "" {
		return common.NewError("网卡名称为空")
	}
	if err := validateMTUValue(mtu); err != nil {
		return err
	}

	mtuStr := strconv.Itoa(mtu)
	if ipPath, err := exec.LookPath("ip"); err == nil {
		if setErr := runCommandWithTimeout(12*time.Second, ipPath, "link", "set", "dev", iface, "mtu", mtuStr); setErr == nil {
			return nil
		}
	}
	if ifconfigPath, err := exec.LookPath("ifconfig"); err == nil {
		if setErr := runCommandWithTimeout(12*time.Second, ifconfigPath, iface, "mtu", mtuStr, "up"); setErr == nil {
			return nil
		}
	}

	return common.NewError("未找到可用命令设置 MTU（ip/ifconfig）")
}

func parseAndValidateMTU(raw string) (int, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return defaultSystemMTUValue, nil
	}
	value, err := strconv.Atoi(trimmed)
	if err != nil {
		return 0, common.NewError("MTU 必须为整数")
	}
	if err := validateMTUValue(value); err != nil {
		return 0, err
	}
	return value, nil
}

func validateMTUValue(mtu int) error {
	if mtu < minAllowedMTUValue || mtu > maxAllowedMTUValue {
		return common.NewError("MTU 取值范围必须在 ", minAllowedMTUValue, " - ", maxAllowedMTUValue, " 之间")
	}
	return nil
}

func joinMTUErrors(errs []error) error {
	if len(errs) == 0 {
		return nil
	}
	texts := make([]string, 0, len(errs))
	for _, err := range errs {
		if err == nil {
			continue
		}
		text := strings.TrimSpace(err.Error())
		if text == "" {
			continue
		}
		texts = append(texts, text)
	}
	if len(texts) == 0 {
		return nil
	}
	return common.NewError(strings.Join(texts, " | "))
}
