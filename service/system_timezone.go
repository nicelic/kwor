package service

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// SystemTimeZoneStatus is intentionally separate from the settings table.
// TimeLocation is blank when it must not be shown in the selector (lack of
// permission or an IANA location outside the curated list).
type SystemTimeZoneStatus struct {
	TimeLocation string `json:"timeLocation"`
	Displayable  bool   `json:"displayable"`
	CanModify    bool   `json:"canModify"`
	Reason       string `json:"reason,omitempty"`
}

type systemTimeZonePathSet struct {
	ZoneinfoRoot string
	Localtime    string
	TimezoneFile string
}

var (
	systemTimeZoneIsLinux  = IsSystemPlatformLinux
	systemTimeZoneGeteuid  = os.Geteuid
	systemTimeZoneDetector = detectSystemTimeLocationName
	systemTimeZonePaths    = systemTimeZonePathSet{
		ZoneinfoRoot: "/usr/share/zoneinfo",
		Localtime:    "/etc/localtime",
		TimezoneFile: "/etc/timezone",
	}
	systemTimeZoneCommandRunner = defaultSystemTimeZoneCommandRunner
	systemTimeZoneFileApplier   = setSystemTimeZoneByFiles
)

func defaultSystemTimeZoneCommandRunner(ctx context.Context, name string, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, name, args...)
	output, err := command.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		return output, fmt.Errorf("%s timed out after %s", name, systemCommandTimeout)
	}
	return output, err
}

func systemTimeZonePermission() (bool, string) {
	if !systemTimeZoneIsLinux() {
		return false, "系统时区仅支持 Linux 平台"
	}
	if systemTimeZoneGeteuid() != 0 {
		return false, "当前面板进程没有修改系统时区的权限"
	}
	return true, ""
}

func currentSystemTimeLocation() string {
	return normalizeTimeLocationName(systemTimeZoneDetector())
}

// GetCurrentSystemTimeLocation exposes the raw, valid IANA value for the save
// rollback path. It intentionally does not decide whether that value may be
// shown in the UI.
func GetCurrentSystemTimeLocation() string {
	return currentSystemTimeLocation()
}

// GetSystemTimeZoneStatus does not cache host state. It is requested only
// when the settings page opens or after a save, so the UI always reflects a
// user-initiated Linux timezone change without a resident watcher.
func GetSystemTimeZoneStatus() SystemTimeZoneStatus {
	allowed, reason := systemTimeZonePermission()
	if !allowed {
		return SystemTimeZoneStatus{CanModify: false, Reason: reason}
	}

	name := currentSystemTimeLocation()
	if name == "" {
		return SystemTimeZoneStatus{
			CanModify: false,
			Reason:    "未能识别 Linux 系统时区",
		}
	}
	if !IsSelectableTimeLocation(name) {
		return SystemTimeZoneStatus{
			CanModify: true,
			Reason:    "当前系统时区不在面板可选列表中",
		}
	}
	return SystemTimeZoneStatus{
		TimeLocation: name,
		Displayable:  true,
		CanModify:    true,
	}
}

// SetSystemTimeLocation changes only the Linux timezone configuration. It
// never adjusts the Linux clock. The caller must have completed remote time
// source validation before invoking this method.
func SetSystemTimeLocation(value string) error {
	return setSystemTimeLocation(value, true)
}

// RestoreSystemTimeLocation is reserved for the transactional rollback path.
// A previously detected, valid IANA zone may be outside the curated selector;
// restoring it must not be blocked by a UI-only list restriction.
func RestoreSystemTimeLocation(value string) error {
	return setSystemTimeLocation(value, false)
}

func setSystemTimeLocation(value string, requireSelectable bool) error {
	allowed, reason := systemTimeZonePermission()
	if !allowed {
		return fmt.Errorf("%s", reason)
	}

	name, err := NormalizePanelTimeLocation(value)
	if err != nil {
		return err
	}
	if requireSelectable && !IsSelectableTimeLocation(name) {
		return fmt.Errorf("系统时区只能选择面板提供的时区")
	}
	previous := currentSystemTimeLocation()
	if previous == "" {
		return fmt.Errorf("无法读取当前 Linux 系统时区，为保证失败可回滚，已拒绝修改")
	}
	if previous == name {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), systemCommandTimeout)
	defer cancel()
	output, timedatectlErr := systemTimeZoneCommandRunner(ctx, "timedatectl", "set-timezone", name)
	if timedatectlErr == nil {
		return nil
	}

	if fallbackErr := systemTimeZoneFileApplier(name); fallbackErr == nil {
		return nil
	} else {
		restoreErr := systemTimeZoneFileApplier(previous)
		commandDetail := strings.TrimSpace(string(output))
		if commandDetail == "" {
			commandDetail = timedatectlErr.Error()
		}
		if restoreErr != nil {
			return fmt.Errorf("timedatectl 修改失败（%s）；/etc/localtime 回退失败：%v；恢复原系统时区失败：%w", commandDetail, fallbackErr, restoreErr)
		}
		return fmt.Errorf("timedatectl 修改失败（%s）；/etc/localtime 回退失败：%w", commandDetail, fallbackErr)
	}
}

func setSystemTimeZoneByFiles(name string) error {
	target, err := systemTimeZoneFileTarget(name)
	if err != nil {
		return err
	}

	if err := replaceSystemLocaltimeSymlink(target); err != nil {
		return err
	}
	if err := updateExistingSystemTimezoneFile(name); err != nil {
		return err
	}
	return nil
}

func systemTimeZoneFileTarget(name string) (string, error) {
	root := filepath.Clean(systemTimeZonePaths.ZoneinfoRoot)
	relative := filepath.Clean(filepath.FromSlash(name))
	if relative == "." || filepath.IsAbs(relative) {
		return "", fmt.Errorf("无效的时区文件路径")
	}
	target := filepath.Join(root, relative)
	contained, err := filepath.Rel(root, target)
	if err != nil || contained == ".." || strings.HasPrefix(contained, ".."+string(os.PathSeparator)) {
		return "", fmt.Errorf("时区文件路径越界")
	}
	info, err := os.Stat(target)
	if err != nil {
		return "", fmt.Errorf("时区文件不存在：%w", err)
	}
	if info.IsDir() {
		return "", fmt.Errorf("时区文件不是有效文件")
	}
	return target, nil
}

func replaceSystemLocaltimeSymlink(target string) error {
	directory := filepath.Dir(systemTimeZonePaths.Localtime)
	temporary, err := os.CreateTemp(directory, ".kwor-localtime-")
	if err != nil {
		return fmt.Errorf("创建系统时区临时文件失败：%w", err)
	}
	temporaryName := temporary.Name()
	if closeErr := temporary.Close(); closeErr != nil {
		_ = os.Remove(temporaryName)
		return fmt.Errorf("关闭系统时区临时文件失败：%w", closeErr)
	}
	if err := os.Remove(temporaryName); err != nil {
		return fmt.Errorf("准备系统时区临时链接失败：%w", err)
	}
	defer os.Remove(temporaryName)

	if err := os.Symlink(target, temporaryName); err != nil {
		return fmt.Errorf("创建系统时区链接失败：%w", err)
	}
	if err := os.Rename(temporaryName, systemTimeZonePaths.Localtime); err != nil {
		return fmt.Errorf("原子替换 /etc/localtime 失败：%w", err)
	}
	return nil
}

func updateExistingSystemTimezoneFile(name string) error {
	path := systemTimeZonePaths.TimezoneFile
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("读取 /etc/timezone 状态失败：%w", err)
	}
	if !info.Mode().IsRegular() {
		return nil
	}

	directory := filepath.Dir(path)
	temporary, err := os.CreateTemp(directory, ".kwor-timezone-")
	if err != nil {
		return fmt.Errorf("创建 /etc/timezone 临时文件失败：%w", err)
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
	if err := temporary.Chmod(info.Mode().Perm()); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("设置 /etc/timezone 临时文件权限失败：%w", err)
	}
	if _, err := temporary.WriteString(name + "\n"); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("写入 /etc/timezone 临时文件失败：%w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("关闭 /etc/timezone 临时文件失败：%w", err)
	}
	if err := os.Rename(temporaryName, path); err != nil {
		return fmt.Errorf("原子替换 /etc/timezone 失败：%w", err)
	}
	return nil
}
