package service

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/alireza0/s-ui/logger"
	"github.com/alireza0/s-ui/util/common"
)

type managedFileRewriteOptions struct {
	DisplayName                      string
	IgnoreUnsupportedUnlockOnSymlink bool
}

func rewriteManagedFileWithImmutable(path string, content string, opts managedFileRewriteOptions) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return common.NewError("配置路径为空")
	}
	displayName := normalizeManagedFileDisplayName(opts.DisplayName)

	if err := removeManagedFile(path, opts); err != nil {
		return err
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return common.NewError("创建", displayName, "目录失败: ", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return common.NewError("写入", displayName, "失败: ", err)
	}

	readBack, err := os.ReadFile(path)
	if err != nil {
		return common.NewError("校验", displayName, "读取失败: ", err)
	}
	if string(readBack) != content {
		return common.NewError("校验", displayName, "失败: 写入内容与文件内容不一致")
	}

	if err := setManagedFileImmutableFlag(path, displayName); err != nil {
		return err
	}
	immutable, immutableErr := detectFileImmutable(path)
	if immutableErr == nil && !immutable {
		return common.NewError("校验", displayName, "失败: immutable 标记未生效")
	}

	logger.Infof("[SystemOptimize] rebuilt and locked %s: %s", displayName, path)
	return nil
}

func removeManagedFile(path string, opts managedFileRewriteOptions) error {
	path = strings.TrimSpace(path)
	if path == "" || !pathEntryExists(path) {
		return nil
	}

	info, statErr := os.Lstat(path)
	if statErr != nil {
		if os.IsNotExist(statErr) {
			return nil
		}
		return common.NewError("读取配置文件状态失败: ", statErr)
	}
	if info.IsDir() {
		return common.NewError("拒绝删除目录路径: ", path)
	}

	displayName := normalizeManagedFileDisplayName(opts.DisplayName)
	immutable, immutableErr := detectFileImmutable(path)
	if immutableErr == nil && immutable {
		logger.Infof("[SystemOptimize] detected immutable lock on %s: %s", displayName, path)
	}

	if err := clearManagedFileImmutableFlag(path, displayName, opts); err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return buildManagedFileDeleteError(displayName, err)
	}
	if pathEntryExists(path) {
		return common.NewError(displayName, "删除后仍存在: ", path)
	}

	logger.Infof("[SystemOptimize] removed old %s: %s", displayName, path)
	return nil
}

func clearManagedFileImmutableFlag(path string, displayName string, opts managedFileRewriteOptions) error {
	path = strings.TrimSpace(path)
	if path == "" || !pathEntryExists(path) {
		return nil
	}

	chattrPath, err := exec.LookPath("chattr")
	if err != nil {
		logger.Warningf("[SystemOptimize] chattr not found while unlocking %s: %s", displayName, path)
		return nil
	}
	if err := runCommandWithTimeout(8*time.Second, chattrPath, "-i", path); err != nil {
		if opts.IgnoreUnsupportedUnlockOnSymlink && isPathSymlink(path) && isImmutableUnsupportedError(err) {
			return nil
		}
		if isImmutableUnsupportedError(err) {
			logger.Warningf("[SystemOptimize] immutable unlock unsupported for %s: %s", displayName, path)
			return nil
		}
		return common.NewError("解除", displayName, " immutable 失败: ", err)
	}
	return nil
}

func setManagedFileImmutableFlag(path string, displayName string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return common.NewError(displayName, "路径为空")
	}

	chattrPath, err := exec.LookPath("chattr")
	if err != nil {
		return common.NewError("未找到 chattr 命令，无法锁定", displayName)
	}
	if err := runCommandWithTimeout(8*time.Second, chattrPath, "+i", path); err != nil {
		if isImmutableUnsupportedError(err) {
			return common.NewError("设置", displayName, " immutable 失败: 当前文件系统不支持 chattr +i")
		}
		return common.NewError("设置", displayName, " immutable 失败: ", err)
	}
	return nil
}

func buildManagedFileDeleteError(displayName string, removeErr error) error {
	text := strings.ToLower(removeErr.Error())
	if os.IsPermission(removeErr) || strings.Contains(text, "operation not permitted") || strings.Contains(text, "permission denied") {
		if _, err := exec.LookPath("chattr"); err != nil {
			return common.NewError("删除旧", displayName, "失败: ", removeErr, "。可能文件已被 immutable(+i) 锁定且系统未安装 chattr，无法自动解锁。")
		}
		return common.NewError("删除旧", displayName, "失败: ", removeErr, "。可能文件仍处于 immutable(+i) 锁定状态，请检查权限或手动执行 chattr -i 后重试。")
	}
	return common.NewError("删除旧", displayName, "失败: ", removeErr)
}

func normalizeManagedFileDisplayName(displayName string) string {
	displayName = strings.TrimSpace(displayName)
	if displayName == "" {
		return "配置文件"
	}
	return displayName
}
