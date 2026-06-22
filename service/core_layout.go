package service

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/alireza0/s-ui/config"
	"github.com/alireza0/s-ui/logger"
)

const (
	singboxCoreSubdir = "singbox"
	mihomoCoreSubdir  = "mihomo"
)

var managedCoreLayoutMu sync.Mutex

func GetManagedCoreRootDir() string {
	return filepath.Join(config.GetDataDir(), "core")
}

func GetSingboxCoreDir() string {
	return filepath.Join(GetManagedCoreRootDir(), singboxCoreSubdir)
}

func GetMihomoCoreDir() string {
	return filepath.Join(GetManagedCoreRootDir(), mihomoCoreSubdir)
}

func GetSingboxConfigPath() string {
	return filepath.Join(GetSingboxCoreDir(), "config.json")
}

func GetMihomoConfigPath() string {
	return filepath.Join(GetMihomoCoreDir(), "server.yaml")
}

func GetMihomoInboundMetaPath() string {
	return filepath.Join(GetManagedCoreRootDir(), mihomoInboundMetaFilename)
}

// EnsureManagedCoreLayout guarantees the managed core layout exists.
func EnsureManagedCoreLayout() error {
	managedCoreLayoutMu.Lock()
	defer managedCoreLayoutMu.Unlock()

	root := GetManagedCoreRootDir()
	if err := os.MkdirAll(root, 0o755); err != nil {
		return fmt.Errorf("create core root directory failed: %w", err)
	}
	singboxCoreDir := GetSingboxCoreDir()
	singboxUsesRoot := sameManagedCorePath(singboxCoreDir, root)
	if singboxUsesRoot {
		if err := migrateSingboxSubdirArtifactsToRoot(root); err != nil {
			return fmt.Errorf("migrate singbox subdir artifacts to core root failed: %w", err)
		}
	} else {
		if err := migrateLegacyCoreDirectoryNameConflict(root, singboxCoreSubdir, "sing-box"); err != nil {
			return fmt.Errorf("migrate legacy singbox directory-name conflict failed: %w", err)
		}
	}
	if err := migrateLegacyCoreDirectoryNameConflict(root, mihomoCoreSubdir, "mihomo"); err != nil {
		return fmt.Errorf("migrate legacy mihomo directory-name conflict failed: %w", err)
	}
	if !singboxUsesRoot {
		if err := os.MkdirAll(singboxCoreDir, 0o755); err != nil {
			return fmt.Errorf("create singbox core directory failed: %w", err)
		}
	}
	if err := os.MkdirAll(GetMihomoCoreDir(), 0o755); err != nil {
		return fmt.Errorf("create mihomo core directory failed: %w", err)
	}

	if err := migrateLegacyCoreArtifacts(root); err != nil {
		return err
	}
	if err := ensureManagedCoreRuntimeScaffold(root); err != nil {
		return err
	}
	return nil
}

func ensureManagedCoreRuntimeScaffold(root string) error {
	if err := ensureManagedCoreDirScaffold(GetMihomoCoreDir()); err != nil {
		return fmt.Errorf("prepare mihomo runtime scaffold failed: %w", err)
	}
	singboxCoreDir := GetSingboxCoreDir()
	if !sameManagedCorePath(singboxCoreDir, root) {
		if err := ensureManagedCoreDirScaffold(singboxCoreDir); err != nil {
			return fmt.Errorf("prepare singbox runtime scaffold failed: %w", err)
		}
	}
	return nil
}

func ensureManagedCoreDirScaffold(coreDir string) error {
	for _, child := range []string{".config", ".cache"} {
		if err := os.MkdirAll(filepath.Join(coreDir, child), 0o755); err != nil {
			return err
		}
	}
	return nil
}

func migrateLegacyCoreArtifacts(root string) error {
	if err := migrateLegacyManagedCoreConfigFiles(root); err != nil {
		return fmt.Errorf("migrate legacy managed core config files failed: %w", err)
	}
	singboxCoreDir := GetSingboxCoreDir()
	if !sameManagedCorePath(singboxCoreDir, root) {
		if err := migrateLegacyCoreEntries(root, singboxCoreDir, legacySingboxArtifactRules()); err != nil {
			return fmt.Errorf("migrate legacy singbox artifacts failed: %w", err)
		}
	}
	if err := migrateLegacyCoreEntries(root, GetMihomoCoreDir(), legacyMihomoArtifactRules()); err != nil {
		return fmt.Errorf("migrate legacy mihomo artifacts failed: %w", err)
	}
	if !sameManagedCorePath(singboxCoreDir, root) {
		if err := migrateLegacySingboxConfigFragments(root, singboxCoreDir); err != nil {
			return fmt.Errorf("migrate legacy singbox config fragments failed: %w", err)
		}
	}
	if err := migrateLegacyMihomoConfigFragments(root, GetMihomoCoreDir()); err != nil {
		return fmt.Errorf("migrate legacy mihomo config fragments failed: %w", err)
	}
	if err := migrateLegacyMihomoHomeArtifacts(root, GetMihomoCoreDir()); err != nil {
		return fmt.Errorf("migrate legacy mihomo home artifacts failed: %w", err)
	}
	if err := ensureManagedCoreCompatibilityLinks(root); err != nil {
		return fmt.Errorf("create core compatibility links failed: %w", err)
	}
	if err := cleanupBrokenManagedCoreSymlinks(root); err != nil {
		return fmt.Errorf("cleanup broken core compatibility links failed: %w", err)
	}
	return nil
}

func sameManagedCorePath(left, right string) bool {
	left = filepath.Clean(left)
	right = filepath.Clean(right)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(left, right)
	}
	return left == right
}

func migrateSingboxSubdirArtifactsToRoot(root string) error {
	sourcePath := filepath.Join(root, singboxCoreSubdir)
	info, err := os.Lstat(sourcePath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}

	if !info.IsDir() {
		if info.Mode()&os.ModeSymlink != 0 {
			if targetInfo, statErr := os.Stat(sourcePath); statErr == nil && targetInfo.IsDir() {
				return migrateSingboxSubdirDirectoryContentsToRoot(sourcePath, root)
			}
		}
		return movePathWithMerge(sourcePath, filepath.Join(root, "sing-box"))
	}

	return migrateSingboxSubdirDirectoryContentsToRoot(sourcePath, root)
}

func migrateSingboxSubdirDirectoryContentsToRoot(sourceDir, root string) error {
	entries, err := os.ReadDir(sourceDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		name := strings.TrimSpace(entry.Name())
		if name == "" {
			continue
		}
		if err := movePathWithMerge(filepath.Join(sourceDir, name), filepath.Join(root, name)); err != nil {
			return err
		}
	}
	if err := os.Remove(sourceDir); err != nil && !errors.Is(err, os.ErrNotExist) {
		if emptyErr := removeDirIfEmpty(sourceDir); emptyErr != nil && !errors.Is(emptyErr, os.ErrNotExist) {
			return emptyErr
		}
	}
	return nil
}

func migrateLegacyManagedCoreConfigFiles(root string) error {
	legacyPairs := []struct {
		src string
		dst string
	}{
		{
			src: filepath.Join(root, "config.json"),
			dst: filepath.Join(root, singboxCoreSubdir, "config.json"),
		},
		{
			src: filepath.Join(root, "server.yaml"),
			dst: filepath.Join(root, mihomoCoreSubdir, "server.yaml"),
		},
	}

	for _, pair := range legacyPairs {
		if err := movePathWithMerge(pair.src, pair.dst); err != nil {
			return err
		}
	}
	return nil
}

func migrateLegacyCoreDirectoryNameConflict(root, subdirName, binName string) error {
	conflictPath := filepath.Join(root, subdirName)
	info, err := os.Lstat(conflictPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if info.IsDir() {
		return nil
	}
	if info.Mode()&os.ModeSymlink != 0 {
		if targetInfo, statErr := os.Stat(conflictPath); statErr == nil && targetInfo.IsDir() {
			return nil
		}
	}

	tempPath := filepath.Join(root, fmt.Sprintf(".%s.legacy-core-%d", subdirName, time.Now().UnixNano()))
	for index := 1; ; index++ {
		if _, statErr := os.Lstat(tempPath); errors.Is(statErr, os.ErrNotExist) {
			break
		} else if statErr != nil {
			return statErr
		}
		tempPath = filepath.Join(root, fmt.Sprintf(".%s.legacy-core-%d-%d", subdirName, time.Now().UnixNano(), index))
	}

	if err := os.Rename(conflictPath, tempPath); err != nil {
		return err
	}
	targetDir := filepath.Join(root, subdirName)
	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return err
	}
	targetPath := filepath.Join(targetDir, binName)
	if err := movePathWithMerge(tempPath, targetPath); err != nil {
		return err
	}
	return nil
}

type legacyCoreArtifactRules struct {
	exactNames  map[string]struct{}
	prefixNames []string
}

func legacySingboxArtifactRules() legacyCoreArtifactRules {
	return legacyCoreArtifactRules{
		exactNames: map[string]struct{}{
			"sing-box":     {},
			"sing-box.exe": {},
		},
		prefixNames: []string{
			"sing-box-download",
			"sing-box-custom-download",
			"sing-box-",
		},
	}
}

func legacyMihomoArtifactRules() legacyCoreArtifactRules {
	return legacyCoreArtifactRules{
		exactNames: map[string]struct{}{
			"mihomo":     {},
			"mihomo.exe": {},
		},
		prefixNames: []string{
			"mihomo-download",
			"mihomo-custom-download",
			"mihomo-",
		},
	}
}

func legacyMihomoHomeArtifactNames() map[string]struct{} {
	return map[string]struct{}{
		"cache.db":     {},
		"country.mmdb": {},
		"geoip.db":     {},
		"geoip.metadb": {},
		"asn.mmdb":     {},
		"geoip.dat":    {},
		"geosite.dat":  {},
		"ui":           {},
		"rules":        {},
		"proxies":      {},
	}
}

func migrateLegacySingboxConfigFragments(root, targetDir string) error {
	candidates := []struct {
		sourceParent string
		fragmentName string
	}{
		{sourceParent: filepath.Join(root, ".config"), fragmentName: "sing-box"},
		{sourceParent: filepath.Join(root, ".cache"), fragmentName: "sing-box"},
	}

	for _, candidate := range candidates {
		srcFragment := filepath.Join(candidate.sourceParent, candidate.fragmentName)
		if _, err := os.Stat(srcFragment); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return err
		}

		relParent := filepath.Base(candidate.sourceParent)
		dstFragment := filepath.Join(targetDir, relParent, candidate.fragmentName)
		if err := movePathWithMerge(srcFragment, dstFragment); err != nil {
			return err
		}
		_ = removeDirIfEmpty(candidate.sourceParent)
	}

	return nil
}

func migrateLegacyMihomoHomeArtifacts(root, targetDir string) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}

	legacyNames := legacyMihomoHomeArtifactNames()
	for _, entry := range entries {
		name := strings.TrimSpace(entry.Name())
		if name == "" {
			continue
		}
		if _, ok := legacyNames[strings.ToLower(name)]; !ok {
			continue
		}

		srcPath := filepath.Join(root, name)
		dstPath := filepath.Join(targetDir, name)
		if err := movePathWithCompatibilityLink(srcPath, dstPath); err != nil {
			return err
		}
	}

	return nil
}

func migrateLegacyCoreEntries(root, targetDir string, rules legacyCoreArtifactRules) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		name := strings.TrimSpace(entry.Name())
		if name == "" {
			continue
		}
		if name == singboxCoreSubdir || name == mihomoCoreSubdir {
			continue
		}
		if !shouldMigrateLegacyEntry(name, rules) {
			continue
		}

		srcPath := filepath.Join(root, name)
		dstPath := filepath.Join(targetDir, name)
		if err := movePathWithMerge(srcPath, dstPath); err != nil {
			return err
		}
	}

	return nil
}

func shouldMigrateLegacyEntry(name string, rules legacyCoreArtifactRules) bool {
	if _, ok := rules.exactNames[name]; ok {
		return true
	}
	for _, prefix := range rules.prefixNames {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

// Some deployments may have created per-user-like runtime folders under core/.config
// and core/.cache. Migrate only obviously Mihomo-related fragments.
func migrateLegacyMihomoConfigFragments(root, targetDir string) error {
	candidates := []struct {
		sourceParent string
		fragmentName string
	}{
		{sourceParent: filepath.Join(root, ".config"), fragmentName: "mihomo"},
		{sourceParent: filepath.Join(root, ".config"), fragmentName: "clash"},
		{sourceParent: filepath.Join(root, ".cache"), fragmentName: "mihomo"},
		{sourceParent: filepath.Join(root, ".cache"), fragmentName: "clash"},
	}

	for _, candidate := range candidates {
		srcFragment := filepath.Join(candidate.sourceParent, candidate.fragmentName)
		if _, err := os.Stat(srcFragment); err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return err
		}

		relParent := filepath.Base(candidate.sourceParent)
		dstFragment := filepath.Join(targetDir, relParent, candidate.fragmentName)
		if err := movePathWithMerge(srcFragment, dstFragment); err != nil {
			return err
		}
		_ = removeDirIfEmpty(candidate.sourceParent)
	}

	return nil
}

func movePathWithMerge(srcPath, dstPath string) error {
	srcInfo, err := os.Lstat(srcPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if srcInfo.Mode()&os.ModeSymlink != 0 && symlinkPointsToPath(srcPath, dstPath) {
		if err := os.Remove(srcPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		logger.Infof("removed stale legacy core compatibility link: %s -> %s", srcPath, dstPath)
		return nil
	}

	dstInfo, dstErr := os.Lstat(dstPath)
	if dstErr == nil {
		if srcInfo.IsDir() && dstInfo.IsDir() {
			return mergeDirectoryContents(srcPath, dstPath)
		}
		return preserveLegacyConflict(srcPath, dstPath, srcInfo)
	}
	if !errors.Is(dstErr, os.ErrNotExist) {
		return dstErr
	}

	if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
		return err
	}
	if err := os.Rename(srcPath, dstPath); err == nil {
		logger.Infof("migrated legacy core path: %s -> %s", srcPath, dstPath)
		return nil
	}

	if srcInfo.IsDir() {
		if err := copyDirRecursive(srcPath, dstPath); err != nil {
			return err
		}
		if err := os.RemoveAll(srcPath); err != nil {
			return err
		}
		logger.Infof("copied legacy core directory across devices: %s -> %s", srcPath, dstPath)
		return nil
	}

	if err := copyFile(srcPath, dstPath); err != nil {
		return err
	}
	if err := os.Remove(srcPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	logger.Infof("copied legacy core file across devices: %s -> %s", srcPath, dstPath)
	return nil
}

func movePathWithCompatibilityLink(srcPath, dstPath string) error {
	if err := movePathWithMerge(srcPath, dstPath); err != nil {
		return err
	}
	if err := createLegacyCompatibilitySymlink(srcPath, dstPath); err != nil {
		logger.Warningf("create legacy compatibility symlink failed: %s -> %s: %v", srcPath, dstPath, err)
	}
	return nil
}

func mergeDirectoryContents(srcDir, dstDir string) error {
	if err := os.MkdirAll(dstDir, 0o755); err != nil {
		return err
	}

	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		srcChild := filepath.Join(srcDir, entry.Name())
		dstChild := filepath.Join(dstDir, entry.Name())
		if err := movePathWithMerge(srcChild, dstChild); err != nil {
			return err
		}
	}

	if err := os.Remove(srcDir); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func preserveLegacyConflict(srcPath, dstPath string, srcInfo os.FileInfo) error {
	// If two same-name files already exist, preserve source as a timestamped
	// legacy artifact under target directory to avoid silent data loss.
	conflictName := filepath.Base(dstPath)
	legacyName := fmt.Sprintf("%s.legacy-%d", conflictName, time.Now().Unix())
	legacyPath := filepath.Join(filepath.Dir(dstPath), legacyName)

	// Ensure we never overwrite an existing fallback file.
	for index := 1; ; index++ {
		if _, err := os.Lstat(legacyPath); errors.Is(err, os.ErrNotExist) {
			break
		} else if err != nil {
			return err
		}
		legacyName = fmt.Sprintf("%s.legacy-%d-%d", conflictName, time.Now().Unix(), index)
		legacyPath = filepath.Join(filepath.Dir(dstPath), legacyName)
	}

	if err := os.MkdirAll(filepath.Dir(legacyPath), 0o755); err != nil {
		return err
	}
	if err := os.Rename(srcPath, legacyPath); err == nil {
		logger.Warningf("legacy core conflict preserved: %s -> %s (target kept: %s)", srcPath, legacyPath, dstPath)
		return nil
	}

	if srcInfo.IsDir() {
		if err := copyDirRecursive(srcPath, legacyPath); err != nil {
			return err
		}
		if err := os.RemoveAll(srcPath); err != nil {
			return err
		}
		logger.Warningf("legacy core conflict copied: %s -> %s (target kept: %s)", srcPath, legacyPath, dstPath)
		return nil
	}

	if err := copyFile(srcPath, legacyPath); err != nil {
		return err
	}
	if err := os.Remove(srcPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	logger.Warningf("legacy core conflict file copied: %s -> %s (target kept: %s)", srcPath, legacyPath, dstPath)
	return nil
}

func copyDirRecursive(srcDir, dstDir string) error {
	srcInfo, err := os.Stat(srcDir)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dstDir, srcInfo.Mode().Perm()); err != nil {
		return err
	}

	entries, err := os.ReadDir(srcDir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		srcChild := filepath.Join(srcDir, entry.Name())
		dstChild := filepath.Join(dstDir, entry.Name())
		if entry.IsDir() {
			if err := copyDirRecursive(srcChild, dstChild); err != nil {
				return err
			}
			continue
		}
		if err := copyFile(srcChild, dstChild); err != nil {
			return err
		}
	}
	return nil
}

func copyFile(srcPath, dstPath string) error {
	srcFile, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	srcInfo, err := srcFile.Stat()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
		return err
	}

	dstFile, err := os.Create(dstPath)
	if err != nil {
		return err
	}
	if _, err := io.Copy(dstFile, srcFile); err != nil {
		dstFile.Close()
		return err
	}
	if err := dstFile.Close(); err != nil {
		return err
	}
	if err := os.Chmod(dstPath, srcInfo.Mode().Perm()); err != nil {
		return err
	}
	return nil
}

func removeDirIfEmpty(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	if len(entries) > 0 {
		return nil
	}
	return os.Remove(dir)
}

func ensureManagedCoreCompatibilityLinks(root string) error {
	if runtime.GOOS == "windows" {
		return nil
	}

	targetDirs := []string{
		GetMihomoCoreDir(),
	}
	if singboxCoreDir := GetSingboxCoreDir(); !sameManagedCorePath(singboxCoreDir, root) {
		targetDirs = append([]string{singboxCoreDir}, targetDirs...)
	}

	entries, err := os.ReadDir(root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		name := strings.TrimSpace(entry.Name())
		if name == "" {
			continue
		}
		if name == singboxCoreSubdir || name == mihomoCoreSubdir {
			continue
		}
		if isSharedManagedCoreRootName(name) {
			continue
		}
		if name == ".config" || name == ".cache" {
			continue
		}
		if sameManagedCorePath(GetSingboxCoreDir(), root) && shouldMigrateLegacyEntry(name, legacySingboxArtifactRules()) {
			continue
		}

		srcPath := filepath.Join(root, name)
		if isSymlinkPath(srcPath) {
			continue
		}
		for _, targetDir := range targetDirs {
			if err := ensureCompatibilityLinkForLegacyEntry(srcPath, filepath.Join(targetDir, name)); err != nil {
				return err
			}
		}
	}

	for _, hiddenDir := range []string{".config", ".cache"} {
		sourceParent := filepath.Join(root, hiddenDir)
		for _, targetDir := range targetDirs {
			if err := ensureCompatibilityLinksForLegacyParent(sourceParent, filepath.Join(targetDir, hiddenDir)); err != nil {
				return err
			}
		}
	}

	return nil
}

func isSharedManagedCoreRootName(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "config.json", "server.yaml", strings.ToLower(mihomoInboundMetaFilename):
		return true
	default:
		return false
	}
}

func ensureCompatibilityLinksForLegacyParent(sourceParent, linkParent string) error {
	if runtime.GOOS == "windows" {
		return nil
	}

	entries, err := os.ReadDir(sourceParent)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}

	if err := os.MkdirAll(linkParent, 0o755); err != nil {
		return err
	}

	for _, entry := range entries {
		name := strings.TrimSpace(entry.Name())
		if name == "" {
			continue
		}
		sourceChild := filepath.Join(sourceParent, name)
		if isSymlinkPath(sourceChild) {
			continue
		}
		linkChild := filepath.Join(linkParent, name)
		if err := ensureCompatibilityLinkForLegacyEntry(sourceChild, linkChild); err != nil {
			return err
		}
	}

	return nil
}

func ensureCompatibilityLinkForLegacyEntry(sourcePath, linkPath string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	if sourcePath == "" || linkPath == "" {
		return nil
	}

	if info, err := os.Lstat(sourcePath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	} else if info.Mode()&os.ModeSymlink != 0 {
		return nil
	}

	if _, err := os.Lstat(linkPath); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(linkPath), 0o755); err != nil {
		return err
	}

	relativeTarget, err := filepath.Rel(filepath.Dir(linkPath), sourcePath)
	if err != nil || strings.TrimSpace(relativeTarget) == "" {
		relativeTarget = sourcePath
	}
	if err := os.Symlink(relativeTarget, linkPath); err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil
		}
		return err
	}
	return nil
}

func cleanupBrokenManagedCoreSymlinks(root string) error {
	if runtime.GOOS == "windows" {
		return nil
	}

	dirs := []string{
		filepath.Join(root, mihomoCoreSubdir),
		filepath.Join(root, mihomoCoreSubdir, ".config"),
		filepath.Join(root, mihomoCoreSubdir, ".cache"),
	}
	if singboxCoreDir := GetSingboxCoreDir(); !sameManagedCorePath(singboxCoreDir, root) {
		dirs = append([]string{
			filepath.Join(root, singboxCoreSubdir),
			filepath.Join(root, singboxCoreSubdir, ".config"),
			filepath.Join(root, singboxCoreSubdir, ".cache"),
		}, dirs...)
	}
	for _, dir := range dirs {
		if err := cleanupBrokenSymlinksInDir(dir); err != nil {
			return err
		}
	}
	return nil
}

func cleanupBrokenSymlinksInDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}

	for _, entry := range entries {
		linkPath := filepath.Join(dir, entry.Name())
		if !isSymlinkPath(linkPath) {
			continue
		}
		targetPath, err := resolveSymlinkPath(linkPath)
		if err != nil {
			return err
		}
		if _, statErr := os.Stat(targetPath); statErr == nil {
			continue
		} else if !errors.Is(statErr, os.ErrNotExist) {
			return statErr
		}
		if err := os.Remove(linkPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		logger.Infof("removed broken managed core compatibility link: %s", linkPath)
	}

	return nil
}

func isSymlinkPath(filePath string) bool {
	info, err := os.Lstat(filePath)
	return err == nil && info.Mode()&os.ModeSymlink != 0
}

func symlinkPointsToPath(linkPath, targetPath string) bool {
	resolvedLink, err := resolveSymlinkPath(linkPath)
	if err != nil {
		return false
	}
	absResolved, err := filepath.Abs(resolvedLink)
	if err != nil {
		absResolved = resolvedLink
	}
	absTarget, err := filepath.Abs(targetPath)
	if err != nil {
		absTarget = targetPath
	}
	return filepath.Clean(absResolved) == filepath.Clean(absTarget)
}

func symlinkPointsInsideDir(linkPath, dirPath string) bool {
	resolvedLink, err := resolveSymlinkPath(linkPath)
	if err != nil {
		return false
	}
	absResolved, err := filepath.Abs(resolvedLink)
	if err != nil {
		absResolved = resolvedLink
	}
	absDir, err := filepath.Abs(dirPath)
	if err != nil {
		absDir = dirPath
	}

	rel, err := filepath.Rel(filepath.Clean(absDir), filepath.Clean(absResolved))
	if err != nil {
		return false
	}
	return rel == "." || (!strings.HasPrefix(rel, ".."+string(filepath.Separator)) && rel != "..")
}

func resolveSymlinkPath(linkPath string) (string, error) {
	target, err := os.Readlink(linkPath)
	if err != nil {
		return "", err
	}
	if filepath.IsAbs(target) {
		return filepath.Clean(target), nil
	}
	return filepath.Clean(filepath.Join(filepath.Dir(linkPath), target)), nil
}

func createLegacyCompatibilitySymlink(legacyPath, targetPath string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	if legacyPath == "" || targetPath == "" {
		return nil
	}

	if _, err := os.Lstat(legacyPath); err == nil {
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}

	if _, err := os.Lstat(targetPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}

	relativeTarget, err := filepath.Rel(filepath.Dir(legacyPath), targetPath)
	if err != nil || strings.TrimSpace(relativeTarget) == "" {
		relativeTarget = targetPath
	}
	return os.Symlink(relativeTarget, legacyPath)
}
