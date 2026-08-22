package service

import (
	"context"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/alireza0/s-ui/config"
	"github.com/alireza0/s-ui/database"
	"github.com/alireza0/s-ui/database/model"
	"github.com/alireza0/s-ui/util/common"
	"gorm.io/gorm"
)

const (
	systemMTUEnabledKey    = "systemMTUEnabled"
	systemMTUValueKey      = "systemMTUValue"
	systemMTUScriptPathKey = "systemMTUScriptPath"
	systemMTUInterfaceKey  = "systemMTUInterface"
	systemMTUOriginalKey   = "systemMTUOriginalValue"

	defaultSystemMTUValue = 1500
	minAllowedMTUValue    = 576
	maxAllowedMTUValue    = 9500

	managedMTUScriptFileName = "_set_mtu_.sh"
	managedMTUServiceUnit    = "kwor-mtu-opt.service"
	managedMTUServicePath    = "/etc/systemd/system/" + managedMTUServiceUnit
	managedMTUScriptOwnerID  = "mtu-script"
	managedMTUServiceOwnerID = "mtu-systemd"
)

var (
	systemMTUOptimizationMu sync.Mutex
	mtuPattern              = regexp.MustCompile(`\bmtu\s+(\d+)\b`)
	mtuInterfaceNamePattern = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,64}$`)
)

type SystemMTUOptimizationService struct {
	SettingService
}

type systemMTUPersistedState struct {
	Enabled     bool
	MTU         int
	OriginalMTU int
	Interface   string
	ScriptPath  string
}

type systemMTUFileSnapshot struct {
	exists bool
	data   []byte
	mode   os.FileMode
}

type SystemMTUOptimizationOverview struct {
	Supported         bool   `json:"supported"`
	Enabled           bool   `json:"enabled"`
	Interface         string `json:"interface"`
	CurrentMTU        int    `json:"currentMtu"`
	MTU               int    `json:"mtu"`
	OriginalMTU       int    `json:"originalMtu"`
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
	return s.GetOverviewContext(context.Background())
}

// GetOverviewContext only reads the current host state. It does not acquire the
// mutation mutex, otherwise a routine page refresh could wait behind an MTU
// rollback, a systemd reload, or another privileged write operation.
func (s *SystemMTUOptimizationService) GetOverviewContext(ctx context.Context) (*SystemMTUOptimizationOverview, error) {
	enabled, err := s.getBool(systemMTUEnabledKey)
	if err != nil {
		return nil, err
	}

	storedMTU, mtuErr := s.getStoredMTU()
	if mtuErr != nil {
		storedMTU = defaultSystemMTUValue
	}
	originalMTU, originalErr := s.getStoredOriginalMTU()
	storedInterface, interfaceErr := s.getStoredInterface()

	scriptPath := s.resolveMTUScriptPath()
	overview := &SystemMTUOptimizationOverview{
		Supported:         IsSystemPlatformLinux(),
		Enabled:           enabled,
		MTU:               storedMTU,
		OriginalMTU:       originalMTU,
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

	issues := make([]string, 0, 6)
	if mtuErr != nil {
		issues = append(issues, strings.TrimSpace(mtuErr.Error()))
	}
	if originalErr != nil {
		issues = append(issues, strings.TrimSpace(originalErr.Error()))
	}
	if interfaceErr != nil {
		issues = append(issues, strings.TrimSpace(interfaceErr.Error()))
	}

	iface := ""
	if enabled {
		iface = sanitizeInterfaceName(storedInterface)
		if iface != "" {
			if _, lookupErr := net.InterfaceByName(iface); lookupErr != nil {
				issues = append(issues, "保存的 MTU 网卡已不可用，将回退到当前默认网卡")
				iface = ""
			}
		}
	}
	var detectErr error
	if iface == "" {
		iface, detectErr = detectDefaultInterfaceNameContext(ctx)
	}
	if detectErr != nil {
		overview.Supported = false
		issues = append(issues, "默认网卡检测失败: "+strings.TrimSpace(detectErr.Error()))
	} else {
		overview.Interface = iface
		currentMTU, currentErr := detectInterfaceMTUValueContext(ctx, iface)
		if currentErr != nil {
			issues = append(issues, "读取网卡 MTU 失败: "+strings.TrimSpace(currentErr.Error()))
		} else {
			overview.CurrentMTU = currentMTU
		}
	}

	if iface != "" {
		if _, capabilityErr := resolveInterfaceMTUMutator(iface); capabilityErr != nil {
			overview.Supported = false
			issues = append(issues, strings.TrimSpace(capabilityErr.Error()))
		}
	}

	systemctlPath, systemctlErr := resolveOperationalSystemctlContext(ctx)
	if systemctlErr != nil {
		overview.Supported = false
		issues = append(issues, strings.TrimSpace(systemctlErr.Error()))
	} else {
		unitState, activeState, stateErr := readSystemdUnitStatusContext(ctx, systemctlPath, managedMTUServiceUnit)
		if stateErr != nil {
			issues = append(issues, "读取 systemd 状态失败: "+strings.TrimSpace(stateErr.Error()))
		} else {
			overview.ServiceEnabled = strings.EqualFold(unitState, "enabled")
			overview.ServiceRegistered = overview.ServiceRegistered || (unitState != "" && !strings.EqualFold(unitState, "not-found"))
			overview.ServiceActive = activeState
		}
	}

	if enabled {
		if overview.OriginalMTU <= 0 {
			issues = append(issues, "旧版本未记录原始 MTU，关闭时将回退到 1500")
		}
		if !overview.ScriptExists {
			issues = append(issues, "MTU 脚本不存在，请重新保存 MTU 或重新开启开关")
		}
		if !overview.ServiceRegistered || !overview.ServiceEnabled {
			issues = append(issues, "systemd 开机自启未注册，将在下次保存或重新开启时自动补注册")
		}
	} else if overview.ServiceRegistered || overview.ServiceEnabled {
		issues = append(issues, "检测到已关闭功能残留的 MTU systemd 服务，请重新切换开关完成清理")
	}

	if len(issues) > 0 {
		overview.Error = strings.Join(issues, " | ")
	}
	return overview, nil
}

func (s *SystemMTUOptimizationService) SetEnabled(enabled bool, requestedMTU *int) error {
	systemMTUOptimizationMu.Lock()
	defer systemMTUOptimizationMu.Unlock()

	if !IsSystemPlatformLinux() {
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

	if !IsSystemPlatformLinux() {
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
	previous, err := s.loadMTUPersistedState()
	if err != nil {
		return err
	}

	iface := ""
	previousInterface := sanitizeInterfaceName(previous.Interface)
	if previous.Enabled {
		iface = previousInterface
		if iface != "" {
			if _, lookupErr := net.InterfaceByName(iface); lookupErr != nil {
				iface = ""
			}
		}
	}
	if iface == "" {
		iface, err = detectDefaultInterfaceName()
		if err != nil {
			return common.NewError("默认网卡检测失败: ", err)
		}
	}
	currentMTU, err := detectInterfaceMTUValue(iface)
	if err != nil {
		return common.NewError("读取网卡原始 MTU 失败: ", err)
	}
	if _, err := resolveInterfaceMTUMutator(iface); err != nil {
		return err
	}
	if _, err := resolveOperationalSystemctl(); err != nil {
		return err
	}

	originalMTU := currentMTU
	if previous.Enabled && previous.OriginalMTU > 0 && iface == previousInterface {
		originalMTU = previous.OriginalMTU
	}
	scriptPath, err := s.rebuildManagedMTUScriptLocked(mtu, iface)
	if err != nil {
		return err
	}
	rollback := func(cause error) error {
		return s.rollbackMTUEnable(previous, iface, currentMTU, scriptPath, cause)
	}
	if err := ensureSystemdMTUService(scriptPath); err != nil {
		return rollback(err)
	}
	if err := runManagedMTUScript(scriptPath, mtu); err != nil {
		return rollback(common.NewError("执行 MTU 脚本失败: ", err))
	}
	verifiedMTU, err := detectInterfaceMTUValue(iface)
	if err != nil || verifiedMTU != mtu {
		if err == nil {
			err = common.NewError("读取值为 ", verifiedMTU, "，期望值为 ", mtu)
		}
		return rollback(common.NewError("校验 MTU 生效结果失败: ", err))
	}

	next := systemMTUPersistedState{
		Enabled:     true,
		MTU:         mtu,
		OriginalMTU: originalMTU,
		Interface:   iface,
		ScriptPath:  scriptPath,
	}
	if err := s.saveMTUPersistedState(next); err != nil {
		return rollback(common.NewError("保存 MTU 状态失败: ", err))
	}
	return nil
}

func (s *SystemMTUOptimizationService) disableMTULocked() error {
	previous, err := s.loadMTUPersistedState()
	if err != nil {
		return err
	}
	previous.ScriptPath = strings.TrimSpace(previous.ScriptPath)
	if previous.ScriptPath == "" {
		previous.ScriptPath = s.resolveMTUScriptPath()
	}
	if !previous.Enabled {
		errs := make([]error, 0, 2)
		if err := removeSystemdMTUService(); err != nil {
			errs = append(errs, err)
		}
		if err := removeManagedMTUScript(previous.ScriptPath); err != nil {
			errs = append(errs, err)
		}
		if err := joinMTUErrors(errs); err != nil {
			return err
		}

		previous.Interface = ""
		previous.OriginalMTU = 0
		return s.saveMTUPersistedState(previous)
	}
	previousInterface := sanitizeInterfaceName(previous.Interface)
	iface := previousInterface
	if iface != "" {
		if _, lookupErr := net.InterfaceByName(iface); lookupErr != nil {
			iface = ""
		}
	}
	if iface == "" {
		iface, err = detectDefaultInterfaceName()
		if err != nil {
			return common.NewError("默认网卡检测失败: ", err)
		}
	}
	restoreMTU := previous.OriginalMTU
	if restoreMTU <= 0 || iface != previousInterface {
		restoreMTU = defaultSystemMTUValue
	}
	if _, err := resolveInterfaceMTUMutator(iface); err != nil {
		return err
	}
	rollback := func(cause error) error {
		return s.rollbackMTUDisable(previous, iface, cause)
	}
	if err := setInterfaceMTUValue(iface, restoreMTU); err != nil {
		return rollback(common.NewError("恢复原始 MTU=", restoreMTU, " 失败: ", err))
	}
	if err := removeSystemdMTUService(); err != nil {
		return rollback(err)
	}
	if err := removeManagedMTUScript(previous.ScriptPath); err != nil {
		return rollback(err)
	}
	next := systemMTUPersistedState{
		Enabled:    false,
		MTU:        restoreMTU,
		ScriptPath: previous.ScriptPath,
	}
	if err := s.saveMTUPersistedState(next); err != nil {
		return rollback(common.NewError("保存 MTU 关闭状态失败: ", err))
	}
	return nil
}

func (s *SystemMTUOptimizationService) rollbackMTUDisable(previous systemMTUPersistedState, iface string, cause error) error {
	errs := []error{cause}
	restoredPath, rebuildErr := s.rebuildManagedMTUScriptLocked(previous.MTU, iface)
	if rebuildErr != nil {
		errs = append(errs, common.NewError("恢复原 MTU 脚本失败: ", rebuildErr))
	} else if serviceErr := ensureSystemdMTUService(restoredPath); serviceErr != nil {
		errs = append(errs, common.NewError("恢复原 MTU systemd 服务失败: ", serviceErr))
	}
	if restoreErr := setInterfaceMTUValue(iface, previous.MTU); restoreErr != nil {
		errs = append(errs, common.NewError("恢复关闭前 MTU 失败: ", restoreErr))
	}
	return joinMTUErrors(errs)
}

func (s *SystemMTUOptimizationService) rollbackMTUEnable(previous systemMTUPersistedState, iface string, previousCurrentMTU int, scriptPath string, cause error) error {
	errs := []error{cause}
	if err := setInterfaceMTUValue(iface, previousCurrentMTU); err != nil {
		errs = append(errs, common.NewError("恢复变更前 MTU 失败: ", err))
	}
	if previous.Enabled {
		restoreInterface := sanitizeInterfaceName(previous.Interface)
		if restoreInterface == "" {
			restoreInterface = iface
		}
		restoredPath, rebuildErr := s.rebuildManagedMTUScriptLocked(previous.MTU, restoreInterface)
		if rebuildErr != nil {
			errs = append(errs, common.NewError("恢复原 MTU 脚本失败: ", rebuildErr))
		} else if serviceErr := ensureSystemdMTUService(restoredPath); serviceErr != nil {
			errs = append(errs, common.NewError("恢复原 MTU systemd 服务失败: ", serviceErr))
		}
	} else {
		if err := removeSystemdMTUService(); err != nil {
			errs = append(errs, err)
		}
		if err := removeManagedMTUScript(scriptPath); err != nil {
			errs = append(errs, err)
		}
	}
	if err := s.saveMTUPersistedState(previous); err != nil {
		errs = append(errs, common.NewError("恢复原 MTU 数据状态失败: ", err))
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

func (s *SystemMTUOptimizationService) getStoredOriginalMTU() (int, error) {
	raw, err := s.getString(systemMTUOriginalKey)
	if err != nil {
		return 0, err
	}
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" || trimmed == "0" {
		return 0, nil
	}
	value, err := strconv.Atoi(trimmed)
	if err != nil {
		return 0, common.NewError("保存的原始 MTU 不是整数")
	}
	if err := validateMTUValue(value); err != nil {
		return 0, common.NewError("保存的原始 MTU 无效: ", err)
	}
	return value, nil
}

func (s *SystemMTUOptimizationService) getStoredInterface() (string, error) {
	raw, err := s.getString(systemMTUInterfaceKey)
	if err != nil {
		return "", err
	}
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", nil
	}
	normalized := sanitizeInterfaceName(trimmed)
	if normalized == "" || normalized != trimmed {
		return "", common.NewError("保存的 MTU 网卡名称无效")
	}
	return normalized, nil
}

func (s *SystemMTUOptimizationService) loadMTUPersistedState() (systemMTUPersistedState, error) {
	enabled, err := s.getBool(systemMTUEnabledKey)
	if err != nil {
		return systemMTUPersistedState{}, err
	}
	mtu, err := s.getStoredMTU()
	if err != nil {
		return systemMTUPersistedState{}, err
	}
	originalMTU, err := s.getStoredOriginalMTU()
	if err != nil {
		return systemMTUPersistedState{}, err
	}
	iface, err := s.getStoredInterface()
	if err != nil {
		return systemMTUPersistedState{}, err
	}
	return systemMTUPersistedState{
		Enabled:     enabled,
		MTU:         mtu,
		OriginalMTU: originalMTU,
		Interface:   iface,
		ScriptPath:  s.resolveMTUScriptPath(),
	}, nil
}

func (s *SystemMTUOptimizationService) saveMTUPersistedState(state systemMTUPersistedState) error {
	db := database.GetDB()
	if db == nil {
		return common.NewError("database is not ready")
	}
	iface := sanitizeInterfaceName(state.Interface)
	if state.Enabled && iface == "" {
		return common.NewError("MTU 网卡名称为空")
	}
	values := map[string]string{
		systemMTUEnabledKey:    strconv.FormatBool(state.Enabled),
		systemMTUValueKey:      strconv.Itoa(state.MTU),
		systemMTUScriptPathKey: strings.TrimSpace(state.ScriptPath),
		systemMTUInterfaceKey:  iface,
		systemMTUOriginalKey:   strconv.Itoa(state.OriginalMTU),
	}
	return db.Transaction(func(tx *gorm.DB) error {
		for key, value := range values {
			setting := &model.Setting{}
			err := tx.Model(model.Setting{}).Where("key = ?", key).Order("id DESC").First(setting).Error
			if database.IsNotFound(err) {
				if err := tx.Create(&model.Setting{Key: key, Value: value}).Error; err != nil {
					return err
				}
				continue
			}
			if err != nil {
				return err
			}
			setting.Value = value
			if err := tx.Save(setting).Error; err != nil {
				return err
			}
		}
		return nil
	})
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

func (s *SystemMTUOptimizationService) rebuildManagedMTUScriptLocked(mtu int, iface string) (string, error) {
	iface = sanitizeInterfaceName(iface)
	if iface == "" {
		return "", common.NewError("MTU 网卡名称为空")
	}
	scriptPath := s.resolveMTUScriptPath()
	scriptPath = strings.TrimSpace(scriptPath)
	if scriptPath == "" {
		return "", common.NewError("MTU 脚本路径为空")
	}
	if err := os.MkdirAll(filepath.Dir(scriptPath), 0o755); err != nil {
		return "", common.NewError("创建 MTU 脚本目录失败: ", err)
	}
	previousFile, err := captureSystemMTUFile(scriptPath)
	if err != nil {
		return "", common.NewError("读取旧 MTU 脚本失败: ", err)
	}
	ownership, err := BeginHostFileOwnership(managedMTUScriptOwnerID, []string{scriptPath}, HostCleanupDelete)
	if err != nil {
		return "", common.NewError("记录待写入 MTU 脚本所有权失败: ", err)
	}

	content := buildManagedMTUScriptContent(mtu, iface)
	if err := writeSystemMTUFileAtomic(scriptPath, []byte(content), 0o755); err != nil {
		if restoreErr := rollbackSystemMTUFile(ownership.ID, scriptPath, previousFile); restoreErr != nil {
			return "", common.NewError("原子替换 MTU 脚本失败: ", err, "；恢复旧脚本失败: ", restoreErr)
		}
		return "", common.NewError("原子替换 MTU 脚本失败: ", err)
	}
	if err := VerifyAndActivateHostResource(ownership.ID); err != nil {
		if restoreErr := rollbackSystemMTUFile(ownership.ID, scriptPath, previousFile); restoreErr != nil {
			return "", common.NewError("确认 MTU 脚本所有权失败: ", err, "；恢复旧脚本失败: ", restoreErr)
		}
		return "", common.NewError("确认 MTU 脚本所有权失败: ", err)
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
	if err := RemoveHostResource(managedMTUScriptOwnerID); err != nil {
		return common.NewError("删除 MTU 脚本所有权记录失败: ", err)
	}

	scriptDir := filepath.Dir(scriptPath)
	entries, err := os.ReadDir(scriptDir)
	if err == nil && len(entries) == 0 {
		_ = os.Remove(scriptDir)
	}
	return nil
}

func captureSystemMTUFile(path string) (systemMTUFileSnapshot, error) {
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return systemMTUFileSnapshot{}, nil
	}
	if err != nil {
		return systemMTUFileSnapshot{}, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return systemMTUFileSnapshot{}, err
	}
	return systemMTUFileSnapshot{exists: true, data: data, mode: info.Mode().Perm()}, nil
}

func restoreSystemMTUFile(path string, snapshot systemMTUFileSnapshot) error {
	if !snapshot.exists {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	return writeSystemMTUFileAtomic(path, snapshot.data, snapshot.mode)
}

func restoreMTUFileOwnership(id string, snapshot systemMTUFileSnapshot) error {
	if snapshot.exists {
		return VerifyAndActivateHostResource(id)
	}
	return RemoveHostResource(id)
}

func rollbackSystemMTUFile(id string, path string, snapshot systemMTUFileSnapshot) error {
	if err := restoreSystemMTUFile(path, snapshot); err != nil {
		return err
	}
	return restoreMTUFileOwnership(id, snapshot)
}

func writeSystemMTUFileAtomic(path string, data []byte, mode os.FileMode) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return err
	}
	tempFile, err := os.CreateTemp(directory, ".kwor-mtu-*")
	if err != nil {
		return err
	}
	tempPath := tempFile.Name()
	defer os.Remove(tempPath)
	if _, err := tempFile.Write(data); err != nil {
		_ = tempFile.Close()
		return err
	}
	if err := tempFile.Sync(); err != nil {
		_ = tempFile.Close()
		return err
	}
	if err := tempFile.Chmod(mode); err != nil {
		_ = tempFile.Close()
		return err
	}
	if err := tempFile.Close(); err != nil {
		return err
	}
	if err := os.Rename(tempPath, path); err != nil {
		return err
	}
	return syncHostOwnershipDirectory(directory)
}

func buildManagedMTUScriptContent(mtu int, iface string) string {
	iface = sanitizeInterfaceName(iface)
	return strings.TrimSpace(
		`#!/bin/sh
# kwor-owner:v1 resource=mtu-script
set -eu

MTU_VALUE="`+strconv.Itoa(mtu)+`"
PREFERRED_IFACE="`+iface+`"
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
  if [ -n "${PREFERRED_IFACE:-}" ] && [ "${PREFERRED_IFACE}" != "lo" ] && [ -e "/sys/class/net/${PREFERRED_IFACE}" ]; then
    echo "${PREFERRED_IFACE}"
    return 0
  fi

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

  for iface_path in /sys/class/net/*; do
    [ -e "$iface_path" ] || continue
    iface="${iface_path##*/}"
    [ "$iface" = "lo" ] && continue
    if [ -r "$iface_path/operstate" ] && [ "$(cat "$iface_path/operstate" 2>/dev/null || true)" = "up" ]; then
      echo "$iface"
      return 0
    fi
  done

  for iface_path in /sys/class/net/*; do
    [ -e "$iface_path" ] || continue
    iface="${iface_path##*/}"
    [ "$iface" = "lo" ] && continue
    echo "$iface"
    return 0
  done

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
elif [ -w "/sys/class/net/$IFACE/mtu" ]; then
  printf '%s\n' "$MTU_VALUE" > "/sys/class/net/$IFACE/mtu"
else
  echo "missing MTU write capability: ip, ifconfig and writable sysfs are unavailable" >&2
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
	systemctlPath, err := resolveOperationalSystemctl()
	if err != nil {
		return err
	}

	serviceContent, err := buildManagedMTUServiceContent(scriptPath)
	if err != nil {
		return err
	}
	previousFile, err := captureSystemMTUFile(managedMTUServicePath)
	if err != nil {
		return common.NewError("读取旧 MTU systemd 服务失败: ", err)
	}
	previousUnitState, _, _ := readSystemdUnitStatus(systemctlPath, managedMTUServiceUnit)
	previousUnitEnabled := strings.EqualFold(previousUnitState, "enabled")
	ownership, err := BeginSystemdHostOwnership(managedMTUServiceOwnerID, managedMTUServiceUnit, []string{managedMTUServicePath}, map[string]string{
		"script": scriptPath,
	})
	if err != nil {
		return common.NewError("记录待写入 MTU systemd 所有权失败: ", err)
	}
	if err := writeSystemMTUFileAtomic(managedMTUServicePath, []byte(serviceContent), 0o644); err != nil {
		if restoreErr := rollbackSystemMTUFile(ownership.ID, managedMTUServicePath, previousFile); restoreErr != nil {
			return common.NewError("原子替换 MTU systemd 服务失败: ", err, "；恢复旧服务失败: ", restoreErr)
		}
		return common.NewError("原子替换 MTU systemd 服务失败: ", err)
	}
	restorePreviousFile := func() error {
		errs := make([]error, 0, 4)
		if err := runCommandWithTimeout(12*time.Second, systemctlPath, "disable", managedMTUServiceUnit); err != nil && !isMissingSystemdUnitError(err) {
			errs = append(errs, err)
		}
		fileRestored := true
		if err := restoreSystemMTUFile(managedMTUServicePath, previousFile); err != nil {
			fileRestored = false
			errs = append(errs, err)
		}
		if err := runCommandWithTimeout(12*time.Second, systemctlPath, "daemon-reload"); err != nil {
			errs = append(errs, err)
		}
		if previousUnitEnabled {
			if err := runCommandWithTimeout(12*time.Second, systemctlPath, "enable", managedMTUServiceUnit); err != nil {
				errs = append(errs, err)
			}
		}
		if fileRestored {
			if err := restoreMTUFileOwnership(ownership.ID, previousFile); err != nil {
				errs = append(errs, err)
			}
		}
		return joinMTUErrors(errs)
	}
	if err := verifySystemdUnitFile(managedMTUServicePath); err != nil {
		if restoreErr := restorePreviousFile(); restoreErr != nil {
			return common.NewError("校验 MTU systemd 服务失败: ", err, "；恢复旧服务失败: ", restoreErr)
		}
		return common.NewError("校验 MTU systemd 服务失败: ", err)
	}

	if err := runCommandWithTimeout(12*time.Second, systemctlPath, "daemon-reload"); err != nil {
		if restoreErr := restorePreviousFile(); restoreErr != nil {
			return common.NewError("重新加载 systemd 失败: ", err, "；恢复旧服务失败: ", restoreErr)
		}
		return common.NewError("重新加载 systemd 失败: ", err)
	}
	if err := runCommandWithTimeout(12*time.Second, systemctlPath, "enable", managedMTUServiceUnit); err != nil {
		if restoreErr := restorePreviousFile(); restoreErr != nil {
			return common.NewError("注册 MTU systemd 开机自启失败: ", err, "；恢复旧服务失败: ", restoreErr)
		}
		return common.NewError("注册 MTU systemd 开机自启失败: ", err)
	}
	if err := VerifyAndActivateHostResource(ownership.ID); err != nil {
		if restoreErr := restorePreviousFile(); restoreErr != nil {
			return common.NewError("确认 MTU systemd 所有权失败: ", err, "；恢复旧服务失败: ", restoreErr)
		}
		return common.NewError("确认 MTU systemd 所有权失败: ", err)
	}
	return nil
}

func buildManagedMTUServiceContent(scriptPath string) (string, error) {
	shellPath, err := resolveManagedScriptShell()
	if err != nil {
		return "", err
	}
	return `# kwor-owner:v1 resource=mtu-systemd
[Unit]
Description=kwor managed default interface MTU
Wants=network-online.target
After=network-online.target

[Service]
Type=oneshot
ExecStartPre=/bin/sleep 10
ExecStart=` + buildSystemdExecCommand(shellPath, scriptPath) + `
RemainAfterExit=yes

[Install]
WantedBy=multi-user.target
`, nil
}

func removeSystemdMTUService() error {
	systemctlPath, systemctlErr := resolveOperationalSystemctl()
	errs := make([]error, 0, 4)
	if systemctlErr == nil {
		unitState, _, stateErr := readSystemdUnitStatus(systemctlPath, managedMTUServiceUnit)
		unitMissing := stateErr == nil && strings.EqualFold(unitState, "not-found") && !pathEntryExists(managedMTUServicePath)
		if !unitMissing {
			if err := runCommandWithTimeout(12*time.Second, systemctlPath, "disable", "--now", managedMTUServiceUnit); err != nil && !isMissingSystemdUnitError(err) {
				return common.NewError("停用 MTU systemd 服务失败: ", err)
			}
		}
	} else if pathEntryExists(managedMTUServicePath) {
		return systemctlErr
	}

	if pathEntryExists(managedMTUServicePath) {
		if err := os.Remove(managedMTUServicePath); err != nil && !os.IsNotExist(err) {
			errs = append(errs, common.NewError("删除 MTU systemd 服务文件失败: ", err))
		}
	}

	if systemctlErr == nil {
		if err := runCommandWithTimeout(12*time.Second, systemctlPath, "daemon-reload"); err != nil {
			errs = append(errs, common.NewError("删除 MTU 服务后重新加载 systemd 失败: ", err))
		}
		_ = runCommandWithTimeout(8*time.Second, systemctlPath, "reset-failed", managedMTUServiceUnit)
	}
	if len(errs) == 0 {
		if err := RemoveHostResource(managedMTUServiceOwnerID); err != nil {
			errs = append(errs, common.NewError("删除 MTU systemd 所有权记录失败: ", err))
		}
	}
	return joinMTUErrors(errs)
}

func isMissingSystemdUnitError(err error) bool {
	if err == nil {
		return false
	}
	lower := strings.ToLower(err.Error())
	for _, fragment := range []string{"not found", "not loaded", "does not exist", "no such file"} {
		if strings.Contains(lower, fragment) {
			return true
		}
	}
	return false
}

func resolveOperationalSystemctl() (string, error) {
	return resolveOperationalSystemctlContext(context.Background())
}

func resolveOperationalSystemctlContext(ctx context.Context) (string, error) {
	if !pathEntryExists("/run/systemd/system") {
		return "", common.NewError("当前环境没有运行中的 systemd 管理器，MTU 持久化不可用")
	}
	systemctlPath, err := exec.LookPath("systemctl")
	if err != nil {
		return "", common.NewError("未找到 systemctl，MTU 持久化不可用")
	}
	output, err := runOptimizationCommandOutputWithTimeout(ctx, 5*time.Second, systemctlPath, "show", "--property=Version", "--value")
	if err != nil {
		return "", common.NewError("无法连接 systemd 管理器: ", err)
	}
	if strings.TrimSpace(output) == "" {
		return "", common.NewError("systemd 管理器未返回版本信息")
	}
	return systemctlPath, nil
}

func readSystemdUnitStatus(systemctlPath string, unit string) (string, string, error) {
	return readSystemdUnitStatusContext(context.Background(), systemctlPath, unit)
}

func readSystemdUnitStatusContext(ctx context.Context, systemctlPath string, unit string) (string, string, error) {
	output, err := runOptimizationCommandOutputWithTimeout(ctx, 5*time.Second, systemctlPath, "show", "-p", "LoadState", "-p", "UnitFileState", "-p", "ActiveState", unit)
	if err != nil {
		return "", "", err
	}
	unitFileState, activeState := parseSystemdUnitStatus(output)
	return unitFileState, activeState, nil
}

func parseSystemdUnitStatus(output string) (string, string) {
	loadState := ""
	unitFileState := ""
	activeState := ""
	for _, line := range strings.Split(strings.ReplaceAll(output, "\r\n", "\n"), "\n") {
		key, value, found := strings.Cut(strings.TrimSpace(line), "=")
		if !found {
			continue
		}
		switch strings.TrimSpace(key) {
		case "LoadState":
			loadState = strings.TrimSpace(value)
		case "UnitFileState":
			unitFileState = strings.TrimSpace(value)
		case "ActiveState":
			activeState = strings.TrimSpace(value)
		}
	}
	if strings.EqualFold(loadState, "not-found") {
		unitFileState = "not-found"
	}
	if unitFileState == "" {
		unitFileState = "unknown"
	}
	if activeState == "" {
		activeState = "unknown"
	}
	return unitFileState, activeState
}

func detectDefaultInterfaceName() (string, error) {
	return detectDefaultInterfaceNameContext(context.Background())
}

func detectDefaultInterfaceNameContext(ctx context.Context) (string, error) {
	if ipPath, err := exec.LookPath("ip"); err == nil {
		output, routeErr := runOptimizationCommandOutputWithTimeout(ctx, 8*time.Second, ipPath, "-o", "route", "show", "to", "default")
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
	name = strings.TrimSpace(name)
	if !mtuInterfaceNamePattern.MatchString(name) {
		return ""
	}
	return name
}

func detectInterfaceMTUValue(iface string) (int, error) {
	return detectInterfaceMTUValueContext(context.Background(), iface)
}

func detectInterfaceMTUValueContext(ctx context.Context, iface string) (int, error) {
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
		output, cmdErr := runOptimizationCommandOutputWithTimeout(ctx, 8*time.Second, ipPath, "link", "show", "dev", iface)
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
	errs := make([]error, 0, 3)
	if ipPath, err := exec.LookPath("ip"); err == nil {
		if setErr := runCommandWithTimeout(12*time.Second, ipPath, "link", "set", "dev", iface, "mtu", mtuStr); setErr == nil {
			return nil
		} else {
			errs = append(errs, setErr)
		}
	}
	if ifconfigPath, err := exec.LookPath("ifconfig"); err == nil {
		if setErr := runCommandWithTimeout(12*time.Second, ifconfigPath, iface, "mtu", mtuStr, "up"); setErr == nil {
			return nil
		} else {
			errs = append(errs, setErr)
		}
	}
	if setErr := writeInterfaceMTUValueSysfs(iface, mtuStr); setErr == nil {
		return nil
	} else {
		errs = append(errs, setErr)
	}
	return common.NewError("设置网卡 MTU 失败: ", joinMTUErrors(errs))
}

func resolveInterfaceMTUMutator(iface string) (string, error) {
	iface = sanitizeInterfaceName(iface)
	if iface == "" {
		return "", common.NewError("网卡名称为空")
	}
	if path, err := exec.LookPath("ip"); err == nil {
		return path, nil
	}
	if path, err := exec.LookPath("ifconfig"); err == nil {
		return path, nil
	}
	path := filepath.Join("/sys/class/net", iface, "mtu")
	file, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err == nil {
		_ = file.Close()
		return path, nil
	}
	return "", common.NewError("缺少 MTU 写入能力：未找到 ip/ifconfig，且 sysfs 不可写")
}

func writeInterfaceMTUValueSysfs(iface string, mtu string) error {
	path := filepath.Join("/sys/class/net", iface, "mtu")
	file, err := os.OpenFile(path, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	defer file.Close()
	if _, err := file.WriteString(mtu + "\n"); err != nil {
		return err
	}
	return nil
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
