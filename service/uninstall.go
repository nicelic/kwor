package service

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/alireza0/s-ui/config"
	"github.com/alireza0/s-ui/database"
	"github.com/shirou/gopsutil/v4/process"
)

const kworOwnershipMarker = "kwor-owner:v1"

var uninstallLegacySystemdUnits = []string{
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

var ownedHostFileImmutableDetectFn = detectFileImmutable

var uninstallDedicatedSysctlFiles = []string{
	"/etc/sysctl.d/99-s-ui-optimize.conf",
	"/etc/sysctl.d/99-kwor-optimize.conf",
}

type KworUninstallOptions struct {
	PanelBinaryPath    string
	PanelBinDir        string
	DataDir            string
	DatabasePath       string
	LegacyDatabasePath string
	PanelServiceName   string
	PanelProcessPaths  []string
	RuntimePaths       []string
	// PreservePanelRuntime is used only by a fresh start recovering a prior
	// incomplete uninstall. The new binary is already executing and must stay
	// available after it clears old host-side residue.
	PreservePanelRuntime bool
}

type UninstallReport struct {
	Removed   []string
	Preserved []string
	Warnings  []string
	Failures  []string
}

func (r *UninstallReport) addRemoved(value string) {
	if r == nil {
		return
	}
	r.Removed = appendUniqueUninstallString(r.Removed, value)
}

func (r *UninstallReport) addPreserved(value string) {
	if r == nil {
		return
	}
	r.Preserved = appendUniqueUninstallString(r.Preserved, value)
}

func (r *UninstallReport) addWarning(value string) {
	if r == nil {
		return
	}
	r.Warnings = appendUniqueUninstallString(r.Warnings, value)
}

func (r *UninstallReport) addFailure(value string) {
	if r == nil {
		return
	}
	r.Failures = appendUniqueUninstallString(r.Failures, value)
}

func (r *UninstallReport) failureError() error {
	if r == nil || len(r.Failures) == 0 {
		return nil
	}
	return errors.New(strings.Join(r.Failures, "; "))
}

func appendUniqueUninstallString(values []string, value string) []string {
	value = strings.TrimSpace(value)
	if value == "" {
		return values
	}
	for _, current := range values {
		if current == value {
			return values
		}
	}
	return append(values, value)
}

func normalizeKworUninstallOptions(options KworUninstallOptions) KworUninstallOptions {
	if strings.TrimSpace(options.PanelBinaryPath) == "" {
		if path, err := os.Executable(); err == nil {
			options.PanelBinaryPath = path
		}
	}
	for index, path := range []string{options.PanelBinaryPath, options.PanelBinDir, options.DataDir, options.DatabasePath, options.LegacyDatabasePath} {
		if strings.TrimSpace(path) == "" {
			continue
		}
		cleaned := filepath.Clean(path)
		switch index {
		case 0:
			options.PanelBinaryPath = cleaned
		case 1:
			options.PanelBinDir = cleaned
		case 2:
			options.DataDir = cleaned
		case 3:
			options.DatabasePath = cleaned
		case 4:
			options.LegacyDatabasePath = cleaned
		}
	}
	if options.PanelBinDir == "" && options.PanelBinaryPath != "" {
		options.PanelBinDir = filepath.Dir(options.PanelBinaryPath)
	}
	if options.DataDir == "" {
		options.DataDir = config.GetDataDir()
	}
	if options.DatabasePath == "" {
		options.DatabasePath = config.GetDBPath()
	}
	if options.LegacyDatabasePath == "" && options.PanelBinDir != "" {
		options.LegacyDatabasePath = filepath.Join(options.PanelBinDir, "db", config.GetName()+".db")
	}
	if options.PanelServiceName == "" {
		options.PanelServiceName = "kwor"
	}
	options.PanelProcessPaths = normalizeOwnershipStrings(append(options.PanelProcessPaths, options.PanelBinaryPath), true)
	options.RuntimePaths = normalizeOwnershipStrings(options.RuntimePaths, true)
	return options
}

// PerformKworUninstall is the one cleanup implementation used by the SSH CLI
// and the independent worker created from the panel UI. It keeps destructive
// actions constrained to known kwor resources, but does not let one stale or
// modified resource prevent removal of every other confirmed project file.
func PerformKworUninstall(options KworUninstallOptions) (report *UninstallReport, resultErr error) {
	options = normalizeKworUninstallOptions(options)
	report = &UninstallReport{}
	if runtime.GOOS != "linux" {
		return report, fmt.Errorf("kwor uninstall is supported on Linux hosts only")
	}
	if os.Geteuid() != 0 {
		return report, fmt.Errorf("kwor uninstall requires root privileges")
	}
	if runningInsideContainer() {
		return report, fmt.Errorf("kwor uninstall is disabled inside Docker containers")
	}

	if err := BeginKworUninstallLifecycle("正在建立卸载停工状态"); err != nil {
		return report, err
	}
	defer func() {
		if resultErr != nil {
			FailKworUninstallLifecycleWithReport(resultErr, report)
		}
	}()
	recordFailure := func(stage string, err error) {
		if err == nil {
			return
		}
		stage = strings.TrimSpace(stage)
		if stage == "" {
			report.addFailure(err.Error())
			return
		}
		report.addFailure(stage + ": " + strings.TrimSpace(err.Error()))
	}
	updatePhase := func(phase string) {
		if err := UpdateKworUninstallLifecycle(phase); err != nil {
			recordFailure("更新卸载状态", err)
		}
	}

	updatePhase("正在停止并核验受管后台任务")
	// 长任务可能正持有生命周期锁。先停工取消它们，再取得该锁；任务的
	// 延迟清理会释放锁，因此卸载不会在发出取消请求前直接放弃。
	if err := RequestKworLifecycleQuiesce(); err != nil {
		report.addWarning("停止面板后台任务未确认完成，继续按精确进程路径强制清理: " + err.Error())
	}
	lock, err := AcquireKworLifecycleLock()
	if err != nil {
		return report, err
	}
	lockReleased := false
	defer func() {
		if !lockReleased {
			_ = lock.Release()
		}
	}()

	store := defaultHostOwnershipStore()
	manifest, found, err := store.Load()
	if err != nil {
		return report, err
	}
	hostIDMismatch := found && manifest != nil && strings.TrimSpace(manifest.HostID) != "" && manifest.HostID != hostOwnershipHostID()
	if hostIDMismatch {
		report.addWarning("host ownership manifest belongs to a different host; skip forwarding baseline restore")
	}
	if !found {
		if err := bootstrapKworUninstallOwnership(store, options); err != nil {
			return report, err
		}
		manifest, _, err = store.Load()
		if err != nil {
			return report, err
		}
	}
	if err := store.SetUninstalling(true); err != nil {
		recordFailure("写入卸载恢复状态", err)
	}
	manifest, _, err = store.Load()
	if err != nil {
		return report, err
	}
	if manifest == nil {
		return report, errors.New("host ownership manifest disappeared before cleanup")
	}
	reloadManifest := func(stage string) bool {
		current, _, loadErr := store.Load()
		if loadErr != nil {
			recordFailure(stage, loadErr)
			return false
		}
		if current == nil {
			recordFailure(stage, errors.New("host ownership manifest is missing"))
			return false
		}
		manifest = current
		return true
	}

	if err := stopKworUpdateWorkersForUninstall(); err != nil {
		recordFailure("停止面板更新 worker", err)
	}
	updatePhase("正在停止并核验受管 Core")
	if err := stopManagedCoresForUninstall(manifest, options); err != nil {
		recordFailure("停止受管 Core", err)
	}

	databaseReady := false
	databasePath := ""
	for _, candidate := range []string{options.DatabasePath, options.LegacyDatabasePath} {
		if uninstallPathExists(candidate) {
			databasePath = candidate
			break
		}
	}
	if databasePath != "" {
		if initErr := database.InitDB(databasePath); initErr != nil {
			report.addWarning("database unavailable; database-backed add-on cleanup will use only ownership records: " + initErr.Error())
		} else {
			databaseReady = true
		}
	}

	if databaseReady {
		if err := (&TrafficOverviewService{}).RemoveManagedVnstatForUninstall(); err != nil {
			recordFailure("清理受管 vnStat", err)
		}
		if _, err := (&AcmeService{}).RemoveManagedAcmeForUninstall(); err != nil {
			recordFailure("清理受管 ACME", err)
		}
	}
	if err := cleanupOwnedAcmeCronForUninstall(options, report); err != nil {
		recordFailure("清理 ACME cron", err)
	}
	// ACME/vnStat cleanup can remove their own ownership records. Reload before
	// the next stage so stale in-memory entries cannot act on a newly reused
	// unit name.
	reloadManifest("重新读取 ACME/vnStat 清理后的所有权状态")

	updatePhase("正在清理受管 nftables 与外部服务")
	if err := cleanupOwnedNftablesForUninstall(store, manifest, report, !hostIDMismatch); err != nil {
		recordFailure("清理受管 nftables", err)
	}
	if err := cleanupOwnedSystemdForUninstall(store, manifest, options, report, true); err != nil {
		recordFailure("清理受管 systemd 服务", err)
	}
	if err := cleanupOwnedHostFilesForUninstall(store, manifest, report); err != nil {
		recordFailure("清理受管宿主机文件", err)
	}

	// Refresh after systemd/nft/file stages because entries can have been
	// removed from the manifest as each stage verifies its own result.
	reloadManifest("重新读取外部资源清理后的所有权状态")
	if err := cleanupRegisteredExternalPathsForUninstall(store, manifest, options, report); err != nil {
		recordFailure("清理登记的外部资源", err)
	}
	updatePhase("正在停止面板并清理最终运行文件")
	reloadManifest("重新读取面板停止前的所有权状态")
	if err := stopPanelForUninstall(manifest, options); err != nil {
		recordFailure("停止面板进程", err)
	}
	reloadManifest("重新读取面板服务清理前的所有权状态")
	if err := cleanupOwnedSystemdForUninstall(store, manifest, options, report, false); err != nil {
		recordFailure("删除面板 systemd 服务", err)
	}
	if err := cleanupPanelRuntimeForUninstall(store, options, report); err != nil {
		recordFailure("删除面板运行文件", err)
	}
	// Release the coarse lifecycle lock while the blocking state still exists,
	// then remove its on-disk lock file as part of /run/kwor cleanup. The empty
	// ownership manifest remains Uninstalling=true until runtime cleanup also
	// succeeds, so a late failure still blocks resource recreation.
	if err := lock.Release(); err != nil {
		recordFailure("释放卸载生命周期锁", err)
	}
	lockReleased = true
	if failureErr := report.failureError(); failureErr != nil {
		return report, failureErr
	}
	if err := ClearKworUninstallLifecycle(); err != nil {
		return report, err
	}
	if err := store.RemoveManifestIfEmpty(); err != nil {
		return report, err
	}
	return report, nil
}

func bootstrapKworUninstallOwnership(store *hostOwnershipStore, options KworUninstallOptions) error {
	if store == nil {
		return errors.New("host ownership store is nil")
	}
	panelPaths := normalizeOwnershipStrings(append([]string{options.PanelBinaryPath, options.DataDir, options.DatabasePath, options.LegacyDatabasePath}, options.RuntimePaths...), true)
	panel, err := store.Upsert(HostResource{
		ID:            "panel-runtime",
		Kind:          HostResourcePanelRuntime,
		State:         hostResourceStateActive,
		CleanupPolicy: HostCleanupDelete,
		Paths:         panelPaths,
		Before:        ownershipPathsAssumedNew(panelPaths),
		Units:         []string{options.PanelServiceName},
	})
	if err != nil {
		return err
	}
	if err := VerifyAndActivateHostResourceForStore(store, panel.ID); err != nil {
		return err
	}
	for _, core := range []struct {
		id   string
		name string
		path string
		unit string
	}{
		{id: "core-singbox", name: "singbox", path: filepath.Join(GetSingboxCoreDir(), "sing-box"), unit: GetSingboxSystemdName()},
		{id: "core-mihomo", name: "mihomo", path: filepath.Join(GetMihomoCoreDir(), "mihomo"), unit: GetMihomoSystemdName()},
	} {
		resource, upsertErr := store.Upsert(HostResource{
			ID:            core.id,
			Kind:          HostResourceManagedCore,
			State:         hostResourceStateActive,
			CleanupPolicy: HostCleanupDelete,
			Paths:         []string{core.path},
			Before:        ownershipPathsAssumedNew([]string{core.path}),
			Units:         []string{core.unit},
			Metadata:      map[string]string{"core": core.name},
		})
		if upsertErr != nil {
			return upsertErr
		}
		if markErr := VerifyAndActivateHostResourceForStore(store, resource.ID); markErr != nil {
			return markErr
		}
	}
	return nil
}

func stopPanelForUninstall(manifest *HostOwnershipManifest, options KworUninstallOptions) error {
	_ = MarkPanelStopOnly()
	defer ClearPanelStopOnlyMarker()
	var failures []error
	panelSystemdOwned := legacySystemdUnitLooksOwned(options.PanelServiceName, options)
	for _, resource := range manifestResources(manifest, HostResourceSystemdUnit) {
		for _, unit := range resource.Units {
			if unit != options.PanelServiceName {
				continue
			}
			if len(ownedSystemdUnitArtifactPaths(unit, options, resource.Paths, resource.Hashes, resource.Before, false)) > 0 ||
				(hasRegisteredSystemdArtifactPath(unit, resource.Paths) && !hasPreexistingSystemdArtifactPath(unit, resource.Paths, resource.Before)) {
				panelSystemdOwned = true
			}
		}
	}
	if panelSystemdOwned {
		if err := stopSystemdUnitForUninstall(options.PanelServiceName, false); err != nil {
			failures = append(failures, fmt.Errorf("stop panel systemd service: %w", err))
		}
	}
	for _, path := range panelProcessPathsForUninstall(manifest, options) {
		if err := TerminateManagedProcessesByBinaryPathExceptPIDs(path, 5*time.Second, []int{os.Getpid()}); err != nil {
			failures = append(failures, fmt.Errorf("stop panel process %s: %w", path, err))
			continue
		}
		if ManagedProcessesRunningByBinaryPath(path, []int{os.Getpid()}) {
			failures = append(failures, fmt.Errorf("panel process is still running: %s", path))
		}
	}
	return errors.Join(failures...)
}

// panelProcessPathsForUninstall 只返回已登记或当前明确可识别的面板二进制
// 路径。这样可以覆盖升级遗留的 kwor/kwor_amd64/kwor_arm64 别名和
// " (deleted)" 进程，同时不会把数据目录等普通运行文件当成进程目标。
func panelProcessPathsForUninstall(manifest *HostOwnershipManifest, options KworUninstallOptions) []string {
	paths := append([]string{}, options.PanelProcessPaths...)
	paths = append(paths, options.PanelBinaryPath)
	for _, resource := range manifestResources(manifest, HostResourcePanelRuntime) {
		if !hostResourceHasVerifiedPostWriteState(resource) {
			continue
		}
		for _, path := range resource.Paths {
			if isKnownPanelBinaryAliasPath(path) || sameUninstallPath(path, options.PanelBinaryPath) {
				paths = append(paths, path)
			}
		}
	}
	return normalizeOwnershipStrings(paths, true)
}

func isKnownPanelBinaryAliasPath(path string) bool {
	switch strings.ToLower(filepath.Base(filepath.Clean(strings.TrimSpace(path)))) {
	case "kwor", "kwor_amd64", "kwor_arm64":
		return true
	default:
		return false
	}
}

func stopManagedCoresForUninstall(manifest *HostOwnershipManifest, options KworUninstallOptions) error {
	var failures []error
	ownedPaths := make([]string, 0)
	for _, resource := range manifestResources(manifest, HostResourceManagedCore) {
		ownedPaths = append(ownedPaths, resource.Paths...)
	}
	singboxPath := filepath.Join(GetSingboxCoreDir(), "sing-box")
	mihomoPath := filepath.Join(GetMihomoCoreDir(), "mihomo")
	ownedPaths = append(ownedPaths, singboxPath, mihomoPath)
	confirmedUnits := make(map[string]struct{})
	for _, resource := range manifestResources(manifest, HostResourceSystemdUnit) {
		for _, unit := range resource.Units {
			if !isManagedCoreSystemdUnitForUninstall(unit) {
				continue
			}
			if len(ownedSystemdUnitArtifactPaths(unit, options, resource.Paths, resource.Hashes, resource.Before, false)) > 0 ||
				(hasRegisteredSystemdArtifactPath(unit, resource.Paths) && !hasPreexistingSystemdArtifactPath(unit, resource.Paths, resource.Before)) {
				confirmedUnits[unit] = struct{}{}
			}
		}
	}
	for _, unit := range append(append([]string{}, uninstallLegacySystemdUnits...), GetSingboxSystemdName(), GetMihomoSystemdName()) {
		if !isManagedCoreSystemdUnitForUninstall(unit) {
			continue
		}
		if legacySystemdUnitLooksOwned(unit, options) {
			confirmedUnits[unit] = struct{}{}
		}
	}

	units := make([]string, 0, len(confirmedUnits))
	for unit := range confirmedUnits {
		units = append(units, unit)
	}
	sort.Strings(units)
	for _, unit := range units {
		if err := stopSystemdUnitForUninstall(unit, false); err != nil {
			failures = append(failures, fmt.Errorf("stop managed core systemd unit %s: %w", unit, err))
		}
	}
	for _, path := range normalizeOwnershipStrings(ownedPaths, true) {
		if err := TerminateManagedProcessesByBinaryPathExceptPIDs(path, 5*time.Second, nil); err != nil {
			failures = append(failures, fmt.Errorf("stop managed core process %s: %w", path, err))
			continue
		}
		if ManagedProcessesRunningByBinaryPath(path, nil) {
			failures = append(failures, fmt.Errorf("managed core process is still running: %s", path))
		}
	}
	return errors.Join(failures...)
}

func isManagedCoreSystemdUnitForUninstall(unit string) bool {
	return legacySystemdUnitIsSingbox(unit) || legacySystemdUnitIsMihomo(unit)
}

func stopKworUpdateWorkersForUninstall() error {
	var failures []error
	if isKworSystemdHost() {
		systemctlPath, err := exec.LookPath("systemctl")
		if err != nil {
			failures = append(failures, fmt.Errorf("find systemctl while listing panel update workers: %w", err))
		} else {
			output, listErr := runCommandOutputWithTimeout(systemCommandTimeout, systemctlPath, "list-units", "--all", "--no-legend", "--plain", "kwor-panel-update-*")
			if listErr != nil {
				failures = append(failures, fmt.Errorf("list panel update transient units: %w", listErr))
			} else {
				for _, line := range strings.Split(output, "\n") {
					fields := strings.Fields(line)
					if len(fields) == 0 || !panelUpdateSystemdUnitNamePattern.MatchString(fields[0]) {
						continue
					}
					unit := fields[0]
					owned, verifyErr := verifyKworPanelUpdateSystemdUnit(systemctlPath, unit, "", false)
					if verifyErr != nil {
						failures = append(failures, fmt.Errorf("verify panel update worker %s: %w", unit, verifyErr))
						continue
					}
					if !owned {
						continue
					}
					if err := stopSystemdUnitForUninstall(unit, false); err != nil {
						failures = append(failures, fmt.Errorf("stop panel update worker %s: %w", unit, err))
					}
				}
			}
		}
	}

	processes, processErr := process.Processes()
	if processErr != nil {
		failures = append(failures, processErr)
		return errors.Join(failures...)
	}
	workers := make([]panelUpdateWorkerProcessIdentity, 0)
	for _, proc := range processes {
		if proc == nil || int(proc.Pid) == os.Getpid() {
			continue
		}
		cmdline, cmdErr := proc.CmdlineSlice()
		if cmdErr != nil {
			continue
		}
		scriptPath := panelUpdateScriptPathFromCommandText(strings.Join(cmdline, " "))
		if scriptPath == "" || !panelUpdateWorkspaceScriptOwned(scriptPath) {
			continue
		}
		cwd, cwdErr := proc.Cwd()
		if cwdErr != nil || filepath.Clean(cwd) != filepath.Dir(scriptPath) {
			continue
		}
		createTime, createErr := proc.CreateTime()
		if createErr != nil || createTime <= 0 {
			continue
		}
		workers = append(workers, panelUpdateWorkerProcessIdentity{pid: proc.Pid, createTime: createTime, scriptPath: scriptPath})
	}
	if len(workers) == 0 {
		return errors.Join(failures...)
	}
	for _, worker := range workers {
		if err := signalPanelUpdateWorkerProcess(worker, false); err != nil {
			failures = append(failures, fmt.Errorf("stop panel update worker %d: %w", worker.pid, err))
		}
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		allExited := true
		for _, worker := range workers {
			if panelUpdateWorkerProcessMatches(worker) {
				allExited = false
				break
			}
		}
		if allExited {
			return errors.Join(failures...)
		}
		time.Sleep(120 * time.Millisecond)
	}
	for _, worker := range workers {
		if !panelUpdateWorkerProcessMatches(worker) {
			continue
		}
		if err := signalPanelUpdateWorkerProcess(worker, true); err != nil {
			failures = append(failures, fmt.Errorf("kill panel update worker %d: %w", worker.pid, err))
		}
	}
	for _, worker := range workers {
		if panelUpdateWorkerProcessMatches(worker) {
			failures = append(failures, fmt.Errorf("panel update worker is still running: %d", worker.pid))
		}
	}
	return errors.Join(failures...)
}

type panelUpdateWorkerProcessIdentity struct {
	pid        int32
	createTime int64
	scriptPath string
}

func panelUpdateWorkerProcessMatches(identity panelUpdateWorkerProcessIdentity) bool {
	if identity.pid <= 0 || identity.createTime <= 0 || identity.scriptPath == "" {
		return false
	}
	proc, err := process.NewProcess(identity.pid)
	if err != nil || !managedCoreProcessRunning(proc) {
		return false
	}
	createTime, err := proc.CreateTime()
	if err != nil || createTime != identity.createTime {
		return false
	}
	cmdline, err := proc.CmdlineSlice()
	if err != nil || panelUpdateScriptPathFromCommandText(strings.Join(cmdline, " ")) != identity.scriptPath {
		return false
	}
	cwd, err := proc.Cwd()
	return err == nil && filepath.Clean(cwd) == filepath.Dir(identity.scriptPath)
}

func signalPanelUpdateWorkerProcess(identity panelUpdateWorkerProcessIdentity, force bool) error {
	if !panelUpdateWorkerProcessMatches(identity) {
		return nil
	}
	proc, err := process.NewProcess(identity.pid)
	if err != nil {
		return nil
	}
	if force {
		err = proc.Kill()
	} else {
		err = proc.Terminate()
	}
	if err != nil && panelUpdateWorkerProcessMatches(identity) {
		return err
	}
	return nil
}

func cleanupOwnedAcmeCronForUninstall(options KworUninstallOptions, report *UninstallReport) error {
	crontabPath, err := exec.LookPath("crontab")
	if err != nil {
		return nil
	}
	current, listErr := runCommandOutputWithTimeout(systemCommandTimeout, crontabPath, "-l")
	if listErr != nil {
		lower := strings.ToLower(current + " " + listErr.Error())
		if strings.Contains(lower, "no crontab") {
			return nil
		}
		return fmt.Errorf("read root crontab while checking managed acme jobs: %w", listErr)
	}
	updated, removed := removeOwnedAcmeCronLines(current, options.DataDir)
	if !removed {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), systemCommandTimeout)
	defer cancel()
	command := exec.CommandContext(ctx, crontabPath, "-")
	command.Stdin = strings.NewReader(updated)
	output, applyErr := command.CombinedOutput()
	if ctx.Err() != nil {
		return fmt.Errorf("replace root crontab while removing managed acme jobs: %w", ctx.Err())
	}
	if applyErr != nil {
		return fmt.Errorf("replace root crontab while removing managed acme jobs: %w: %s", applyErr, strings.TrimSpace(string(output)))
	}
	verified, verifyErr := runCommandOutputWithTimeout(systemCommandTimeout, crontabPath, "-l")
	if verifyErr != nil {
		return fmt.Errorf("verify root crontab after removing managed acme jobs: %w", verifyErr)
	}
	_, stillPresent := removeOwnedAcmeCronLines(verified, options.DataDir)
	if stillPresent {
		return errors.New("managed acme cron entry remains after removal")
	}
	report.addRemoved("root crontab managed acme renewal")
	return nil
}

func removeOwnedAcmeCronLines(content string, dataDir string) (string, bool) {
	acmeRoot := filepath.ToSlash(filepath.Join(strings.TrimSpace(dataDir), "acme"))
	if acmeRoot == "" || acmeRoot == "." || acmeRoot == "/acme" {
		return content, false
	}
	acmeScript := strings.TrimSuffix(acmeRoot, "/") + "/acme.sh"
	lines := strings.Split(content, "\n")
	kept := make([]string, 0, len(lines))
	removed := false
	for _, line := range lines {
		normalized := filepath.ToSlash(line)
		if shellCommandLineReferencesExactPath(normalized, acmeScript) {
			removed = true
			continue
		}
		kept = append(kept, line)
	}
	updated := strings.Join(kept, "\n")
	if removed && strings.TrimSpace(updated) != "" && !strings.HasSuffix(updated, "\n") {
		updated += "\n"
	}
	return updated, removed
}

func shellCommandLineReferencesExactPath(line string, path string) bool {
	if path == "" {
		return false
	}
	searchFrom := 0
	for searchFrom <= len(line)-len(path) {
		relative := strings.Index(line[searchFrom:], path)
		if relative < 0 {
			return false
		}
		start := searchFrom + relative
		end := start + len(path)
		beforeOK := start == 0 || shellPathBoundary(line[start-1])
		afterOK := end == len(line) || shellPathBoundary(line[end])
		if beforeOK && afterOK {
			return true
		}
		searchFrom = start + 1
	}
	return false
}

func shellPathBoundary(value byte) bool {
	switch value {
	case ' ', '\t', '\r', '\n', '\'', '"', '=', '(', ')', ';', '&', '|':
		return true
	default:
		return false
	}
}

func cleanupOwnedNftablesForUninstall(store *hostOwnershipStore, manifest *HostOwnershipManifest, report *UninstallReport, restoreForwardingOptions ...bool) error {
	restoreForwarding := true
	if len(restoreForwardingOptions) > 0 {
		restoreForwarding = restoreForwardingOptions[0]
	}
	var failures []error
	resources := manifestResources(manifest, HostResourceNftTable)
	registered := make(map[string]HostResource)
	registeredTables := make(map[string]struct{})
	for _, resource := range resources {
		registered[resource.ID] = resource
		for _, table := range resource.NftTables {
			registeredTables[table.Family+"\x00"+table.Name] = struct{}{}
		}
	}
	legacyTables, err := detectLegacyOwnedNftTables()
	if err != nil {
		failures = append(failures, fmt.Errorf("detect legacy kwor nftables tables: %w", err))
	}
	missingLegacyTables := make([]HostNftTable, 0, len(legacyTables))
	for _, table := range legacyTables {
		if _, exists := registeredTables[table.Family+"\x00"+table.Name]; !exists {
			missingLegacyTables = append(missingLegacyTables, table)
		}
	}
	if len(missingLegacyTables) > 0 {
		resource, registerErr := store.Upsert(HostResource{
			ID:            "legacy-nftables",
			Kind:          HostResourceNftTable,
			State:         hostResourceStateActive,
			CleanupPolicy: HostCleanupDelete,
			NftTables:     missingLegacyTables,
		})
		if registerErr != nil {
			failures = append(failures, fmt.Errorf("register legacy kwor nftables tables: %w", registerErr))
		} else {
			registered[resource.ID] = resource
		}
	}
	resourceIDs := make([]string, 0, len(registered))
	for resourceID := range registered {
		resourceIDs = append(resourceIDs, resourceID)
	}
	sort.Strings(resourceIDs)
	for _, resourceID := range resourceIDs {
		resource := registered[resourceID]
		if err := store.MarkState(resource.ID, hostResourceStateCleanupPending); err != nil {
			failures = append(failures, fmt.Errorf("mark nftables resource %s for cleanup: %w", resource.ID, err))
			continue
		}
		resourceFailed := false
		for _, table := range resource.NftTables {
			if err := deleteOwnedNftTable(table); err != nil {
				failures = append(failures, fmt.Errorf("delete owned nft table %s %s: %w", table.Family, table.Name, err))
				resourceFailed = true
				continue
			}
			report.addRemoved("nft table " + table.Family + " " + table.Name)
		}
		if resourceFailed {
			continue
		}
		if err := store.Remove(resource.ID); err != nil {
			failures = append(failures, fmt.Errorf("remove nftables ownership resource %s: %w", resource.ID, err))
		}
	}
	if err := restoreOwnedKernelForwardingFromManifest(store, manifest, report, restoreForwarding); err != nil {
		failures = append(failures, err)
	}
	return errors.Join(failures...)
}

func restoreOwnedKernelForwardingFromManifest(store *hostOwnershipStore, manifest *HostOwnershipManifest, report *UninstallReport, restoreForwardingOptions ...bool) error {
	restoreForwarding := true
	if len(restoreForwardingOptions) > 0 {
		restoreForwarding = restoreForwardingOptions[0]
	}
	var failures []error
	if current, found, err := store.Load(); err != nil {
		failures = append(failures, err)
	} else if found {
		manifest = current
	}
	resources := manifestResources(manifest, HostResourceKernelForward)
	if len(resources) == 0 {
		return nil
	}
	hostFingerprint := ""
	if restoreForwarding {
		var err error
		hostFingerprint, err = portForwardKernelHostFingerprintFn()
		if err != nil || strings.TrimSpace(hostFingerprint) == "" {
			if err == nil {
				err = errors.New("empty host fingerprint")
			}
			failures = append(failures, fmt.Errorf("read forwarding host fingerprint from ownership manifest: %w", err))
		}
	}
	for _, resource := range resources {
		if err := store.MarkState(resource.ID, hostResourceStateCleanupPending); err != nil {
			failures = append(failures, fmt.Errorf("mark forwarding resource %s for cleanup: %w", resource.ID, err))
			continue
		}
		if !restoreForwarding {
			report.addPreserved("skip forwarding restore due to host fingerprint mismatch")
			if err := store.Remove(resource.ID); err != nil {
				failures = append(failures, fmt.Errorf("remove forwarding ownership resource %s: %w", resource.ID, err))
			}
			continue
		}
		if strings.TrimSpace(hostFingerprint) == "" {
			continue
		}
		if strings.TrimSpace(resource.Metadata["hostFingerprint"]) != strings.TrimSpace(hostFingerprint) {
			failures = append(failures, fmt.Errorf("refuse to restore forwarding from a different host for resource %s", resource.ID))
			continue
		}
		resourceFailed := false
		for _, family := range []struct {
			name        string
			path        string
			modifiedKey string
			originalKey string
		}{
			{name: "ipv4", path: portForwardIPv4ForwardPath, modifiedKey: "ipv4Modified", originalKey: "ipv4Original"},
			{name: "ipv6", path: portForwardIPv6ForwardPath, modifiedKey: "ipv6Modified", originalKey: "ipv6Original"},
		} {
			modified, parseErr := strconv.ParseBool(strings.TrimSpace(resource.Metadata[family.modifiedKey]))
			if parseErr != nil || !modified {
				continue
			}
			original := resource.Metadata[family.originalKey]
			if original == "" {
				failures = append(failures, fmt.Errorf("owned %s forwarding baseline is missing", family.name))
				resourceFailed = true
				continue
			}
			if err := restorePortForwardKernelValueIfUnchanged(family.path, original); err != nil {
				failures = append(failures, fmt.Errorf("restore %s forwarding from ownership manifest: %w", family.name, err))
				resourceFailed = true
				continue
			}
			actual, readErr := portForwardKernelReadFileFn(family.path)
			if readErr != nil {
				failures = append(failures, fmt.Errorf("verify restored %s forwarding from ownership manifest: %w", family.name, readErr))
				resourceFailed = true
				continue
			}
			if string(actual) != original {
				failures = append(failures, fmt.Errorf("%s forwarding value remains different after restore", family.name))
				resourceFailed = true
				continue
			}
			report.addRemoved("restored " + family.path)
		}
		if resourceFailed {
			continue
		}
		if err := store.Remove(resource.ID); err != nil {
			failures = append(failures, fmt.Errorf("remove forwarding ownership resource %s: %w", resource.ID, err))
		}
	}
	return errors.Join(failures...)
}

func detectLegacyOwnedNftTables() ([]HostNftTable, error) {
	if _, err := exec.LookPath("nft"); err != nil {
		return nil, nil
	}
	candidates := []HostNftTable{
		{Family: "inet", Name: nftTable},
		{Family: "ip", Name: nftTable},
		{Family: "ip6", Name: nftTable},
		{Family: "inet", Name: firewallNftTable},
		{Family: "inet", Name: portForwardNftTable},
		{Family: "ip", Name: portForwardNftTable},
		{Family: "ip6", Name: portForwardNftTable},
	}
	result := make([]HostNftTable, 0, len(candidates))
	for _, table := range candidates {
		output, exists, err := listNftTableForUninstall(table)
		if err != nil {
			return nil, err
		}
		if !exists {
			continue
		}
		owned, ownershipErr := nftTableIsSafeToDelete(table.Family, table.Name, []byte(output))
		if ownershipErr != nil {
			return nil, ownershipErr
		}
		if owned {
			result = append(result, table)
		}
	}
	return result, nil
}

func deleteOwnedNftTable(table HostNftTable) error {
	output, exists, err := listNftTableForUninstall(table)
	if err != nil {
		return err
	}
	if !exists {
		return nil
	}
	owned, ownershipErr := nftTableIsSafeToDelete(table.Family, table.Name, []byte(output))
	if ownershipErr != nil {
		return ownershipErr
	}
	if !owned {
		return fmt.Errorf("refuse to delete nft table without kwor ownership proof: %s %s", table.Family, table.Name)
	}
	nftPath, err := exec.LookPath("nft")
	if err != nil {
		return fmt.Errorf("nft is unavailable while owned table remains %s %s: %w", table.Family, table.Name, err)
	}
	if err := runCommandWithTimeout(systemCommandTimeout, nftPath, "delete", "table", table.Family, table.Name); err != nil {
		return fmt.Errorf("delete owned nft table %s %s: %w", table.Family, table.Name, err)
	}
	_, stillExists, listErr := listNftTableForUninstall(table)
	if listErr != nil {
		return listErr
	}
	if stillExists {
		return fmt.Errorf("owned nft table remains after delete: %s %s", table.Family, table.Name)
	}
	return nil
}

func listNftTableForUninstall(table HostNftTable) (string, bool, error) {
	nftPath, err := exec.LookPath("nft")
	if err != nil {
		return "", false, fmt.Errorf("find nft command: %w", err)
	}
	output, commandErr := runCommandOutputWithTimeout(systemCommandTimeout, nftPath, "list", "table", table.Family, table.Name)
	if commandErr == nil {
		return output, true, nil
	}
	lower := strings.ToLower(output + " " + commandErr.Error())
	if strings.Contains(lower, "no such file") || strings.Contains(lower, "not found") {
		return "", false, nil
	}
	return "", false, fmt.Errorf("list nft table %s %s: %w", table.Family, table.Name, commandErr)
}

type ownedSystemdUninstallTarget struct {
	resourceIDs          map[string]struct{}
	paths                []string
	hashes               map[string]string
	before               map[string]HostPathBeforeState
	legacy               bool
	packageManagedVnstat bool
}

type uninstallSystemdUnitStatus struct {
	FragmentPath  string
	DropInPaths   []string
	ActiveState   string
	UnitFileState string
	LoadState     string
}

var uninstallSystemdSearchRoots = []string{
	"/etc/systemd/system",
	"/run/systemd/system",
	"/lib/systemd/system",
	"/usr/lib/systemd/system",
}

var uninstallSystemdEnableLinkRoots = []string{
	"/etc/systemd/system",
	"/run/systemd/system",
}

var uninstallSystemdWalkDirFn = filepath.WalkDir

var uninstallSystemdShowFn = func(systemctlPath string, unit string) (uninstallSystemdUnitStatus, error) {
	output, err := runCommandOutputWithTimeout(
		systemCommandTimeout,
		systemctlPath,
		"show",
		unit,
		"--property=FragmentPath",
		"--property=DropInPaths",
		"--property=ActiveState",
		"--property=UnitFileState",
		"--property=LoadState",
	)
	status := uninstallSystemdUnitStatus{}
	for _, line := range strings.Split(output, "\n") {
		key, value, found := strings.Cut(strings.TrimSpace(line), "=")
		if !found {
			continue
		}
		switch key {
		case "FragmentPath":
			if path := strings.TrimSpace(value); path != "" {
				status.FragmentPath = filepath.Clean(path)
			}
		case "DropInPaths":
			for _, path := range strings.Fields(value) {
				path = filepath.Clean(strings.TrimSpace(path))
				if path != "" && path != "." {
					status.DropInPaths = append(status.DropInPaths, path)
				}
			}
		case "ActiveState":
			status.ActiveState = strings.TrimSpace(value)
		case "UnitFileState":
			status.UnitFileState = strings.TrimSpace(value)
		case "LoadState":
			status.LoadState = strings.TrimSpace(value)
		}
	}
	if err != nil {
		lower := strings.ToLower(output + " " + err.Error())
		if strings.Contains(lower, "not found") || strings.Contains(lower, "not loaded") {
			status.LoadState = "not-found"
			return status, nil
		}
		return status, err
	}
	return status, nil
}

func cleanupOwnedSystemdForUninstall(store *hostOwnershipStore, manifest *HostOwnershipManifest, options KworUninstallOptions, report *UninstallReport, deferPanelUnitOptions ...bool) error {
	deferPanelUnit := false
	if len(deferPanelUnitOptions) > 0 {
		deferPanelUnit = deferPanelUnitOptions[0]
	}
	targets, resourceRemaining := collectOwnedSystemdUninstallTargets(manifest, options, deferPanelUnit)

	units := make([]string, 0, len(targets))
	for unit := range targets {
		units = append(units, unit)
	}
	sort.Strings(units)
	markedResources := make(map[string]struct{})
	var failures []error
	for _, unit := range units {
		target := targets[unit]
		if target.packageManagedVnstat && len(target.resourceIDs) == 0 {
			if err := stopSystemdUnitForUninstall(unit, true); err != nil {
				failures = append(failures, fmt.Errorf("stop package-managed vnstat unit %s: %w", unit, err))
				continue
			}
			report.addRemoved("stopped/disabled systemd " + unit)
			continue
		}
		for resourceID := range target.resourceIDs {
			if _, marked := markedResources[resourceID]; marked {
				continue
			}
			if err := store.MarkState(resourceID, hostResourceStateCleanupPending); err != nil {
				failures = append(failures, err)
				continue
			}
			markedResources[resourceID] = struct{}{}
		}
		if err := removeOwnedSystemdUnit(unit, options, target.paths, target.hashes, target.before, target.legacy); err != nil {
			failures = append(failures, fmt.Errorf("remove systemd unit %s: %w", unit, err))
			continue
		}
		report.addRemoved("systemd " + unit)
		for resourceID := range target.resourceIDs {
			resourceRemaining[resourceID]--
			if resourceRemaining[resourceID] > 0 {
				continue
			}
			if err := store.Remove(resourceID); err != nil {
				failures = append(failures, err)
			}
		}
	}
	if isKworSystemdHost() {
		systemctlPath, err := exec.LookPath("systemctl")
		if err != nil {
			failures = append(failures, fmt.Errorf("find systemctl after owned unit cleanup: %w", err))
			return errors.Join(failures...)
		}
		if err := runCommandWithTimeout(systemCommandTimeout, systemctlPath, "daemon-reload"); err != nil {
			failures = append(failures, fmt.Errorf("reload systemd after owned unit cleanup: %w", err))
		}
		_ = runCommandWithTimeout(systemCommandTimeout, systemctlPath, "reset-failed")
	}
	return errors.Join(failures...)
}

func collectOwnedSystemdUninstallTargets(manifest *HostOwnershipManifest, options KworUninstallOptions, deferPanelUnit bool) (map[string]*ownedSystemdUninstallTarget, map[string]int) {
	targets := make(map[string]*ownedSystemdUninstallTarget)
	resourceRemaining := make(map[string]int)
	addTarget := func(unit string, resource *HostResource, legacy bool, trackResource bool) {
		unit = strings.TrimSpace(unit)
		if unit == "" {
			return
		}
		target := targets[unit]
		if target == nil {
			target = &ownedSystemdUninstallTarget{resourceIDs: make(map[string]struct{}), hashes: make(map[string]string), before: make(map[string]HostPathBeforeState)}
			targets[unit] = target
		}
		target.legacy = target.legacy || legacy
		if resource == nil {
			return
		}
		target.paths = append(target.paths, resource.Paths...)
		for path, hash := range resource.Hashes {
			target.hashes[filepath.Clean(path)] = hash
		}
		target.before = mergeHostPathBeforeStates(target.before, resource.Before)
		if trackResource {
			if _, exists := target.resourceIDs[resource.ID]; exists {
				return
			}
			target.resourceIDs[resource.ID] = struct{}{}
			resourceRemaining[resource.ID]++
		}
	}
	for _, resource := range manifestResources(manifest, HostResourceSystemdUnit) {
		for _, unit := range resource.Units {
			if deferPanelUnit && unit == options.PanelServiceName {
				continue
			}
			copy := resource
			addTarget(unit, &copy, false, true)
		}
	}
	for _, resource := range manifestResources(manifest, HostResourceManagedCore) {
		for _, unit := range resource.Units {
			copy := resource
			addTarget(unit, &copy, false, true)
		}
	}
	for _, resource := range manifestResources(manifest, HostResourceACME) {
		for _, unit := range resource.Units {
			// The ACME record owns its other files as a single resource and is
			// removed only by its later dedicated cleanup stage.
			copy := resource
			addTarget(unit, &copy, false, false)
		}
	}
	for _, resource := range manifestResources(manifest, HostResourceVnStat) {
		for _, unit := range resource.Units {
			copy := resource
			addTarget(unit, &copy, false, false)
			if normalizeVnstatInstallMethod(resource.Metadata["installMethod"], resource.Metadata["packageManager"]) == vnstatInstallMethodSystemPackage {
				targets[unit].packageManagedVnstat = true
			}
		}
	}

	// Historic installations have no durable ledger. Their unit content must
	// still prove its relation to kwor before the unit is stopped or deleted.
	for _, unit := range append(append([]string{}, uninstallLegacySystemdUnits...), acmeSystemdUnitCandidates...) {
		if deferPanelUnit && unit == options.PanelServiceName {
			continue
		}
		if _, exists := targets[unit]; !exists && legacySystemdUnitLooksOwned(unit, options) {
			addTarget(unit, nil, true, false)
		}
	}
	for _, unit := range []string{options.PanelServiceName, GetSingboxSystemdName(), GetMihomoSystemdName(), "kwor-mtu-opt", "kwor-vnstat"} {
		if deferPanelUnit && unit == options.PanelServiceName {
			continue
		}
		if _, exists := targets[unit]; !exists && legacySystemdUnitLooksOwned(unit, options) {
			addTarget(unit, nil, true, false)
		}
	}
	return targets, resourceRemaining
}

func removeOwnedSystemdUnit(unit string, options KworUninstallOptions, registeredPaths []string, hashes map[string]string, before map[string]HostPathBeforeState, legacy bool) error {
	artifacts := ownedSystemdUnitArtifactPaths(unit, options, registeredPaths, hashes, before, legacy)
	preserveExisting := hasPreexistingSystemdArtifactPath(unit, registeredPaths, before)
	systemdHost := isKworSystemdHost()
	if len(artifacts) == 0 {
		if hasRegisteredSystemdArtifactPath(unit, registeredPaths) {
			if registeredSystemdArtifactExists(unit, registeredPaths) {
				if preserveExisting {
					return nil
				}
				return fmt.Errorf("registered systemd artifact can no longer be verified as kwor-owned: %s", unit)
			}
			if preserveExisting {
				return nil
			}
			if systemdHost {
				if err := verifySystemdUnitAbsentForUninstall(unit); err != nil {
					return fmt.Errorf("registered systemd unit cannot be verified as removed: %s: %w", unit, err)
				}
			}
			return nil
		}
		if systemdHost {
			if err := verifySystemdUnitAbsentForUninstall(unit); err != nil {
				return fmt.Errorf("systemd unit has no verified kwor artifact and is not absent: %s: %w", unit, err)
			}
		}
		return nil
	}
	if !preserveExisting {
		if systemdHost {
			if err := stopSystemdUnitForUninstall(unit, true); err != nil {
				return err
			}
		}
		enableLinks, err := ownedSystemdEnableLinksForUninstall(unit, artifacts)
		if err != nil {
			return err
		}
		for _, path := range enableLinks {
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return fmt.Errorf("remove systemd enable link %s: %w", path, err)
			}
		}
	}
	for _, path := range artifacts {
		if err := removeOwnedUninstallPath(path); err != nil {
			return fmt.Errorf("remove owned systemd artifact %s: %w", path, err)
		}
	}
	for _, directory := range systemdDropInDirectoriesForUninstall(unit) {
		entries, err := os.ReadDir(directory)
		if err == nil && len(entries) == 0 {
			_ = os.Remove(directory)
		}
	}
	if systemdHost && !preserveExisting {
		if systemctlPath, err := exec.LookPath("systemctl"); err == nil {
			if err := runCommandWithTimeout(systemCommandTimeout, systemctlPath, "daemon-reload"); err != nil {
				return fmt.Errorf("reload systemd while verifying %s removal: %w", unit, err)
			}
		} else {
			return fmt.Errorf("find systemctl while verifying %s removal: %w", unit, err)
		}
	}
	if preserveExisting {
		return nil
	}
	if !systemdHost {
		return nil
	}
	return verifySystemdUnitAbsentForUninstall(unit)
}

func hasRegisteredSystemdArtifactPath(unit string, paths []string) bool {
	fileName := uninstallSystemdUnitFileName(unit)
	if fileName == "" {
		return false
	}
	for _, path := range paths {
		path = filepath.Clean(path)
		for _, root := range uninstallSystemdSearchRoots {
			basePath := filepath.Join(root, fileName)
			if path == basePath || strings.HasPrefix(path, basePath+".d"+string(os.PathSeparator)) || path == basePath+".d" {
				return true
			}
		}
	}
	return false
}

func registeredSystemdArtifactExists(unit string, paths []string) bool {
	fileName := uninstallSystemdUnitFileName(unit)
	if fileName == "" {
		return false
	}
	for _, path := range paths {
		path = filepath.Clean(path)
		for _, root := range uninstallSystemdSearchRoots {
			basePath := filepath.Join(root, fileName)
			if path == basePath || strings.HasPrefix(path, basePath+".d"+string(os.PathSeparator)) || path == basePath+".d" {
				if uninstallPathExists(path) {
					return true
				}
			}
		}
	}
	return false
}

func ownedSystemdUnitArtifactPaths(unit string, options KworUninstallOptions, registeredPaths []string, hashes map[string]string, before map[string]HostPathBeforeState, legacy bool) []string {
	registered := make(map[string]struct{}, len(registeredPaths))
	for _, path := range registeredPaths {
		registered[filepath.Clean(path)] = struct{}{}
	}
	result := make([]string, 0)
	seen := make(map[string]struct{})
	add := func(path string) {
		path = filepath.Clean(path)
		if _, exists := seen[path]; exists || !uninstallPathExists(path) || !isSystemdArtifactPathForUninstall(unit, path) {
			return
		}
		if !systemdArtifactBelongsToKwor(unit, path, options, registered, hashes, before, legacy) {
			return
		}
		seen[path] = struct{}{}
		result = append(result, path)
	}
	for _, path := range uninstallSystemdUnitPaths(unit) {
		if strings.HasSuffix(path, ".d") {
			entries, err := os.ReadDir(path)
			if err != nil {
				continue
			}
			for _, entry := range entries {
				if !entry.IsDir() {
					add(filepath.Join(path, entry.Name()))
				}
			}
			continue
		}
		add(path)
	}
	if isKworSystemdHost() {
		systemctlPath, err := exec.LookPath("systemctl")
		if err != nil {
			return result
		}
		if status, showErr := uninstallSystemdShowFn(systemctlPath, unit); showErr == nil {
			add(status.FragmentPath)
			for _, path := range status.DropInPaths {
				add(path)
			}
		}
	}
	for path := range registered {
		add(path)
	}
	sort.Strings(result)
	return result
}

func isSystemdArtifactPathForUninstall(unit string, path string) bool {
	path = filepath.Clean(strings.TrimSpace(path))
	fileName := uninstallSystemdUnitFileName(unit)
	if path == "" || path == "." || fileName == "" {
		return false
	}
	for _, root := range uninstallSystemdSearchRoots {
		basePath := filepath.Join(root, fileName)
		if path == basePath || strings.HasPrefix(path, basePath+".d"+string(os.PathSeparator)) {
			return true
		}
	}
	return false
}

func hasPreexistingSystemdArtifactPath(unit string, paths []string, before map[string]HostPathBeforeState) bool {
	for _, path := range paths {
		path = filepath.Clean(path)
		if !isSystemdArtifactPathForUninstall(unit, path) {
			continue
		}
		if state, exists := before[path]; exists && state.Existed {
			return true
		}
	}
	return false
}

func systemdArtifactBelongsToKwor(unit string, path string, options KworUninstallOptions, registered map[string]struct{}, hashes map[string]string, before map[string]HostPathBeforeState, legacy bool) bool {
	path = filepath.Clean(path)
	if state, exists := before[path]; exists && state.Existed {
		return false
	}
	if _, exists := registered[path]; exists && !legacy {
		// The exact systemd artifact was created by kwor. Its content may have
		// changed during an upgrade or a local edit, but the registered path is
		// still inside the systemd unit boundary for this known service.
		return true
	}
	if expectedHash := strings.TrimSpace(hashes[path]); expectedHash != "" {
		return ownershipPathHashes([]string{path})[path] == expectedHash
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	text := strings.ToLower(string(content))
	if strings.Contains(text, kworOwnershipMarker) {
		return true
	}
	return legacy && legacySystemdUnitHasTrustedSignature(unit, text, options)
}

func legacySystemdUnitHasTrustedSignature(unit string, content string, options KworUninstallOptions) bool {
	unit = strings.ToLower(strings.TrimSpace(uninstallSystemdUnitFileName(unit)))
	if unit == "" || !legacySystemdUnitNameIsKnown(unit, options) {
		return false
	}
	content = normalizeSystemdOwnershipContent(content)
	execStarts := systemdExecStartLinesForOwnership(content)
	if len(execStarts) == 0 {
		return false
	}
	containsPath := func(value string) bool {
		value = normalizeSystemdOwnershipContent(value)
		if value == "" {
			return false
		}
		for _, line := range execStarts {
			if strings.Contains(line, value) {
				return true
			}
		}
		return false
	}
	switch {
	case unit == strings.ToLower(uninstallSystemdUnitFileName(options.PanelServiceName)):
		return containsPath(options.PanelBinaryPath)
	case legacySystemdUnitIsSingbox(unit):
		return containsPath(filepath.Join(GetSingboxCoreDir(), "sing-box"))
	case legacySystemdUnitIsMihomo(unit):
		return containsPath(filepath.Join(GetMihomoCoreDir(), "mihomo"))
	case unit == strings.ToLower(managedMTUServiceUnit):
		return containsPath(filepath.Join(options.DataDir, "mtu", managedMTUScriptFileName))
	case unit == strings.ToLower(uninstallSystemdUnitFileName("kwor-vnstat")):
		// A generic vnstatd ExecStart is not enough to establish ownership.
		// Historic panel units without the explicit marker are intentionally kept.
		return false
	case legacySystemdUnitIsAcme(unit):
		return containsPath(filepath.Join(options.DataDir, "acme", "acme.sh"))
	default:
		return false
	}
}

func normalizeSystemdOwnershipContent(value string) string {
	value = strings.ToLower(filepath.ToSlash(value))
	value = strings.ReplaceAll(value, `\x20`, " ")
	value = strings.ReplaceAll(value, `\x09`, "\t")
	value = strings.ReplaceAll(value, `\x0a`, "\n")
	return value
}

func systemdExecStartLinesForOwnership(content string) []string {
	lines := strings.Split(content, "\n")
	result := make([]string, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(strings.ToLower(line), "execstart=") {
			continue
		}
		value := strings.TrimSpace(line[len("ExecStart="):])
		value = strings.TrimLeft(value, "-@:+! ")
		if value != "" {
			result = append(result, normalizeSystemdOwnershipContent(value))
		}
	}
	return result
}

func legacySystemdUnitNameIsKnown(unit string, options KworUninstallOptions) bool {
	if unit == strings.ToLower(uninstallSystemdUnitFileName(options.PanelServiceName)) ||
		unit == strings.ToLower(managedMTUServiceUnit) ||
		unit == strings.ToLower(uninstallSystemdUnitFileName("kwor-vnstat")) ||
		legacySystemdUnitIsSingbox(unit) || legacySystemdUnitIsMihomo(unit) || legacySystemdUnitIsAcme(unit) {
		return true
	}
	return false
}

func legacySystemdUnitIsSingbox(unit string) bool {
	unit = strings.TrimSuffix(strings.ToLower(unit), ".service")
	for _, candidate := range append(append([]string{}, legacySingboxSystemdNames...), GetSingboxSystemdName()) {
		if unit == strings.TrimSuffix(strings.ToLower(candidate), ".service") {
			return true
		}
	}
	return false
}

func legacySystemdUnitIsMihomo(unit string) bool {
	unit = strings.TrimSuffix(strings.ToLower(unit), ".service")
	candidates := []string{"mihomo", "metacubex-mihomo", "s-ui-mihomo", "sui-mihomo", GetMihomoSystemdName()}
	for _, candidate := range candidates {
		if unit == strings.TrimSuffix(strings.ToLower(candidate), ".service") {
			return true
		}
	}
	return false
}

func legacySystemdUnitIsAcme(unit string) bool {
	for _, candidate := range acmeSystemdUnitCandidates {
		if strings.EqualFold(unit, uninstallSystemdUnitFileName(candidate)) {
			return true
		}
	}
	return false
}

func systemdDropInDirectoriesForUninstall(unit string) []string {
	fileName := uninstallSystemdUnitFileName(unit)
	if fileName == "" {
		return nil
	}
	result := make([]string, 0, len(uninstallSystemdSearchRoots))
	for _, root := range uninstallSystemdSearchRoots {
		result = append(result, filepath.Join(root, fileName+".d"))
	}
	return result
}

func stopSystemdUnitForUninstall(unit string, disable bool) error {
	unit = strings.TrimSpace(unit)
	if unit == "" {
		return nil
	}
	if !isKworSystemdHost() {
		return nil
	}
	systemctlPath, err := exec.LookPath("systemctl")
	if err != nil {
		return fmt.Errorf("find systemctl while stopping %s: %w", unit, err)
	}
	stopErr := runCommandWithTimeout(systemCommandTimeout, systemctlPath, "stop", unit)
	active, activeErr := systemdUnitActiveForUninstall(systemctlPath, unit)
	if activeErr != nil {
		if stopErr != nil {
			return fmt.Errorf("systemctl stop %s failed (%v) and status verification failed: %w", unit, stopErr, activeErr)
		}
		return fmt.Errorf("verify systemd unit %s after stop: %w", unit, activeErr)
	}
	if active {
		if stopErr != nil {
			return fmt.Errorf("systemctl stop %s: %w", unit, stopErr)
		}
		return fmt.Errorf("systemd unit remains active after stop: %s", unit)
	}
	if disable {
		disableErr := runCommandWithTimeout(systemCommandTimeout, systemctlPath, "disable", unit)
		enabled, enabledErr := systemdUnitEnabledForUninstall(systemctlPath, unit)
		if enabledErr != nil {
			if disableErr != nil {
				return fmt.Errorf("systemctl disable %s failed (%v) and status verification failed: %w", unit, disableErr, enabledErr)
			}
			return fmt.Errorf("verify systemd unit %s after disable: %w", unit, enabledErr)
		}
		if enabled {
			if disableErr != nil {
				return fmt.Errorf("systemctl disable %s: %w", unit, disableErr)
			}
			return fmt.Errorf("systemd unit remains enabled after disable: %s", unit)
		}
	}
	return nil
}

func systemdUnitActiveForUninstall(systemctlPath string, unit string) (bool, error) {
	status, err := uninstallSystemdShowFn(systemctlPath, unit)
	if err != nil {
		return false, err
	}
	loadState := strings.ToLower(strings.TrimSpace(status.LoadState))
	if loadState == "not-found" {
		return false, nil
	}
	if loadState == "" {
		return false, errors.New("systemd returned an empty LoadState")
	}
	switch strings.ToLower(strings.TrimSpace(status.ActiveState)) {
	case "active", "activating", "reloading", "deactivating":
		return true, nil
	case "inactive", "failed":
		return false, nil
	default:
		return false, fmt.Errorf("unknown systemd ActiveState %q", status.ActiveState)
	}
}

func systemdUnitEnabledForUninstall(systemctlPath string, unit string) (bool, error) {
	status, err := uninstallSystemdShowFn(systemctlPath, unit)
	if err != nil {
		return false, err
	}
	loadState := strings.ToLower(strings.TrimSpace(status.LoadState))
	state := strings.ToLower(strings.TrimSpace(status.UnitFileState))
	if loadState == "not-found" && (state == "" || state == "not-found") {
		return false, nil
	}
	switch state {
	case "enabled", "enabled-runtime", "linked", "linked-runtime", "alias":
		return true, nil
	case "disabled", "static", "indirect", "generated", "transient", "not-found", "masked", "masked-runtime":
		return false, nil
	case "":
		return false, errors.New("systemd returned an empty UnitFileState for a loaded unit")
	default:
		return false, fmt.Errorf("unknown systemd UnitFileState %q", status.UnitFileState)
	}
}

func uninstallSystemdUnitExists(unit string) bool {
	for _, path := range uninstallSystemdUnitPaths(unit) {
		if uninstallPathExists(path) {
			return true
		}
	}
	return false
}

func uninstallSystemdUnitPaths(unit string) []string {
	fileName := uninstallSystemdUnitFileName(unit)
	if fileName == "" {
		return nil
	}
	paths := make([]string, 0, len(uninstallSystemdSearchRoots)*2)
	for _, root := range uninstallSystemdSearchRoots {
		paths = append(paths, filepath.Join(root, fileName))
		paths = append(paths, filepath.Join(root, fileName+".d"))
	}
	return paths
}

func uninstallSystemdUnitFileName(unit string) string {
	unit = strings.TrimSpace(unit)
	if unit == "" {
		return ""
	}
	for _, suffix := range []string{".service", ".timer", ".socket", ".path", ".target"} {
		if strings.HasSuffix(unit, suffix) {
			return unit
		}
	}
	return unit + ".service"
}

func uninstallSystemdEnableLinks(unit string) ([]string, error) {
	fileName := uninstallSystemdUnitFileName(unit)
	if fileName == "" {
		return nil, nil
	}
	result := make([]string, 0)
	seen := make(map[string]struct{})
	for _, root := range uninstallSystemdEnableLinkRoots {
		info, err := os.Lstat(root)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, fmt.Errorf("inspect systemd enable-link root %s: %w", root, err)
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("systemd enable-link root is not a directory: %s", root)
		}
		walkErr := uninstallSystemdWalkDirFn(root, func(path string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry == nil || entry.IsDir() || entry.Name() != fileName {
				return nil
			}
			parentName := filepath.Base(filepath.Dir(path))
			if !strings.HasSuffix(parentName, ".wants") && !strings.HasSuffix(parentName, ".requires") {
				return nil
			}
			path = filepath.Clean(path)
			if _, exists := seen[path]; !exists {
				seen[path] = struct{}{}
				result = append(result, path)
			}
			return nil
		})
		if walkErr != nil {
			return nil, fmt.Errorf("scan systemd enable links under %s for %s: %w", root, unit, walkErr)
		}
	}
	sort.Strings(result)
	return result, nil
}

func ownedSystemdEnableLinksForUninstall(unit string, artifacts []string) ([]string, error) {
	if len(artifacts) == 0 {
		return nil, nil
	}
	ownedTargets := make(map[string]struct{}, len(artifacts)*2)
	for _, artifact := range artifacts {
		artifact = filepath.Clean(artifact)
		ownedTargets[artifact] = struct{}{}
		if resolved, err := filepath.EvalSymlinks(artifact); err == nil {
			ownedTargets[filepath.Clean(resolved)] = struct{}{}
		}
	}
	links, err := uninstallSystemdEnableLinks(unit)
	if err != nil {
		return nil, err
	}
	ownedLinks := make([]string, 0, len(links))
	for _, link := range links {
		info, err := os.Lstat(link)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("inspect systemd enable link %s: %w", link, err)
		}
		if info.Mode()&os.ModeSymlink == 0 {
			return nil, fmt.Errorf("refuse to remove non-symlink systemd enable artifact: %s", link)
		}
		target, err := os.Readlink(link)
		if err != nil {
			return nil, fmt.Errorf("read systemd enable link %s: %w", link, err)
		}
		if !filepath.IsAbs(target) {
			target = filepath.Join(filepath.Dir(link), target)
		}
		target = filepath.Clean(target)
		if _, owned := ownedTargets[target]; !owned {
			return nil, fmt.Errorf("refuse to remove systemd enable link with an unverified target: %s -> %s", link, target)
		}
		ownedLinks = append(ownedLinks, link)
	}
	return ownedLinks, nil
}

func verifySystemdUnitAbsentForUninstall(unit string) error {
	unit = strings.TrimSpace(unit)
	if unit == "" {
		return nil
	}
	for _, path := range uninstallSystemdUnitPaths(unit) {
		if uninstallPathExists(path) {
			return fmt.Errorf("systemd unit artifact remains: %s", path)
		}
	}
	links, linksErr := uninstallSystemdEnableLinks(unit)
	if linksErr != nil {
		return linksErr
	}
	if len(links) > 0 {
		return fmt.Errorf("systemd enable link remains: %s", strings.Join(links, ", "))
	}
	if !isKworSystemdHost() {
		return nil
	}
	systemctlPath, err := exec.LookPath("systemctl")
	if err != nil {
		return fmt.Errorf("find systemctl while verifying %s removal: %w", unit, err)
	}
	status, err := uninstallSystemdShowFn(systemctlPath, unit)
	if err != nil {
		return fmt.Errorf("show systemd unit %s after removal: %w", unit, err)
	}
	if err := verifySystemdUnitStatusAbsentForUninstall(unit, status); err != nil {
		return err
	}
	return nil
}

func verifySystemdUnitStatusAbsentForUninstall(unit string, status uninstallSystemdUnitStatus) error {
	if path := strings.TrimSpace(status.FragmentPath); path != "" {
		return fmt.Errorf("systemd unit %s still has fragment path: %s", unit, path)
	}
	if len(status.DropInPaths) > 0 {
		return fmt.Errorf("systemd unit %s still has drop-in paths: %s", unit, strings.Join(status.DropInPaths, ", "))
	}
	loadState := strings.ToLower(strings.TrimSpace(status.LoadState))
	if loadState != "not-found" {
		return fmt.Errorf("systemd unit %s remains loaded: %s", unit, status.LoadState)
	}
	activeState := strings.ToLower(strings.TrimSpace(status.ActiveState))
	switch activeState {
	case "inactive", "failed":
	default:
		return fmt.Errorf("systemd unit %s remains active: %s", unit, status.ActiveState)
	}
	unitFileState := strings.ToLower(strings.TrimSpace(status.UnitFileState))
	switch unitFileState {
	case "", "not-found":
	default:
		return fmt.Errorf("systemd unit %s still has unit-file state: %s", unit, status.UnitFileState)
	}
	return nil
}

func legacySystemdUnitLooksOwned(unit string, options KworUninstallOptions) bool {
	for _, path := range uninstallSystemdUnitPaths(unit) {
		if strings.HasSuffix(path, ".d") {
			entries, err := os.ReadDir(path)
			if err != nil {
				continue
			}
			for _, entry := range entries {
				if !entry.IsDir() && systemdArtifactBelongsToKwor(unit, filepath.Join(path, entry.Name()), options, nil, nil, nil, true) {
					return true
				}
			}
			continue
		}
		if systemdArtifactBelongsToKwor(unit, path, options, nil, nil, nil, true) {
			return true
		}
	}
	return false
}

func cleanupOwnedHostFilesForUninstall(store *hostOwnershipStore, manifest *HostOwnershipManifest, report *UninstallReport) error {
	var failures []error
	for _, resource := range manifestResources(manifest, HostResourceHostFile) {
		if err := store.MarkState(resource.ID, hostResourceStateCleanupPending); err != nil {
			failures = append(failures, err)
			continue
		}
		resourceFailed := false
		for _, path := range resource.Paths {
			switch resource.CleanupPolicy {
			case HostCleanupUnlockOnly:
				shouldClear, actionErr := shouldClearOwnedHostFileImmutable(resource, path)
				if actionErr != nil {
					failures = append(failures, actionErr)
					resourceFailed = true
					continue
				}
				if shouldClear {
					if err := clearManagedFileImmutableFlag(path, "kwor host config", managedFileRewriteOptions{IgnoreUnsupportedUnlockOnSymlink: true}); err != nil {
						failures = append(failures, err)
						resourceFailed = true
						continue
					}
					immutable, verifyErr := detectFileImmutable(path)
					if verifyErr != nil {
						failures = append(failures, fmt.Errorf("verify immutable unlock for %s: %w", path, verifyErr))
						resourceFailed = true
						continue
					}
					if immutable {
						failures = append(failures, fmt.Errorf("immutable flag remains after kwor unlock: %s", path))
						resourceFailed = true
						continue
					}
					report.addPreserved("unlocked " + path)
				} else if uninstallPathExists(path) {
					report.addPreserved("preserved immutable state " + path)
				}
			case HostCleanupDelete:
				action, err := ownedUninstallResourcePathAction(resource, path)
				if err != nil {
					failures = append(failures, err)
					resourceFailed = true
					continue
				}
				if action == ownedUninstallPathUnresolved {
					report.addWarning(fmt.Sprintf("保留无法确认归属的 %s 资源路径: %s", resource.Kind, path))
					continue
				}
				if action != ownedUninstallPathRemove {
					if uninstallPathExists(path) {
						report.addPreserved(path)
					}
					continue
				}
				if err := removeOwnedUninstallPath(path); err != nil {
					failures = append(failures, err)
					resourceFailed = true
					continue
				}
				report.addRemoved(path)
			default:
				report.addPreserved(path)
			}
		}
		if !resourceFailed {
			if err := store.Remove(resource.ID); err != nil {
				failures = append(failures, err)
			}
		}
	}
	return errors.Join(failures...)
}

func cleanupRegisteredExternalPathsForUninstall(store *hostOwnershipStore, manifest *HostOwnershipManifest, options KworUninstallOptions, report *UninstallReport) error {
	var failures []error
	for _, resource := range manifest.Resources {
		switch resource.Kind {
		case HostResourcePanelRuntime, HostResourceSystemdUnit, HostResourceManagedCore, HostResourceNftTable, HostResourceHostFile, HostResourceKernelForward:
			continue
		}
		if err := store.MarkState(resource.ID, hostResourceStateCleanupPending); err != nil {
			failures = append(failures, err)
			continue
		}
		if resource.Kind == HostResourceVnStat {
			if err := cleanupRegisteredVnstatResourceForUninstall(resource, options, report); err != nil {
				failures = append(failures, err)
				continue
			}
			if err := store.Remove(resource.ID); err != nil {
				failures = append(failures, err)
			}
			continue
		}
		type externalPathCleanupPlan struct {
			path   string
			action ownedUninstallPathAction
		}
		plans := make([]externalPathCleanupPlan, 0, len(resource.Paths))
		resourceFailed := false
		for _, path := range resource.Paths {
			if path == options.DataDir || isUninstallPathWithin(path, options.DataDir) {
				continue
			}
			action, err := ownedUninstallResourcePathAction(resource, path)
			if err != nil {
				failures = append(failures, err)
				resourceFailed = true
				continue
			}
			if action == ownedUninstallPathUnresolved {
				report.addWarning(fmt.Sprintf("保留无法确认归属的 %s 资源路径: %s", resource.Kind, path))
				continue
			}
			plans = append(plans, externalPathCleanupPlan{path: path, action: action})
		}
		for _, plan := range plans {
			if plan.action != ownedUninstallPathRemove {
				if uninstallPathExists(plan.path) {
					report.addPreserved(plan.path)
				}
				continue
			}
			if err := removeOwnedUninstallPath(plan.path); err != nil {
				failures = append(failures, fmt.Errorf("remove registered %s path %s: %w", resource.Kind, plan.path, err))
				resourceFailed = true
				continue
			}
			report.addRemoved(plan.path)
		}
		if !resourceFailed {
			if err := store.Remove(resource.ID); err != nil {
				failures = append(failures, err)
			}
		}
	}
	return errors.Join(failures...)
}

func cleanupRegisteredVnstatResourceForUninstall(resource HostResource, options KworUninstallOptions, report *UninstallReport) error {
	if strings.TrimSpace(resource.Metadata["ownership"]) != vnstatOwnershipPanelInstalled {
		return errors.New("refuse to remove vnstat without panel ownership evidence")
	}
	installMethod := normalizeVnstatInstallMethod(resource.Metadata["installMethod"], resource.Metadata["packageManager"])
	if installMethod == "" {
		return errors.New("registered vnstat installation method is missing")
	}
	paths := normalizeOwnershipStrings(resource.Paths, true)
	hasPreservedPath := false
	for _, path := range paths {
		if isUninstallPathWithin(path, options.DataDir) {
			continue
		}
		if !isSafeVnstatResidualPath(path) {
			return fmt.Errorf("refuse to remove unsafe registered vnstat path: %s", path)
		}
		action, err := ownedUninstallResourcePathAction(resource, path)
		if err != nil {
			return err
		}
		if action == ownedUninstallPathUnresolved {
			return fmt.Errorf("refuse to remove unverified vnstat resource path: %s", path)
		}
		if action == ownedUninstallPathPreserve {
			hasPreservedPath = true
			report.addPreserved(path)
		}
	}
	if hasPreservedPath {
		// A pre-existing vnStat path means the package or daemon may belong to
		// the host. Preserve every related resource rather than partly removing
		// a shared installation.
		return nil
	}
	if !hostResourceHasVerifiedPostWriteState(resource) {
		return errors.New("refuse to remove vnstat without a verified post-write ownership record")
	}
	if err := stopRegisteredVnstatProcessesForUninstall(resource); err != nil {
		return err
	}
	if installMethod == vnstatInstallMethodSystemPackage {
		manager, ok := managedVnstatPackageManagerForUninstall(trafficOverviewVnstatManifest{
			PackageManager: resource.Metadata["packageManager"],
		})
		if !ok || manager == nil {
			return errors.New("registered vnstat package manager cannot be verified")
		}
		if packageName := strings.TrimSpace(resource.Metadata["packageName"]); packageName != "" && packageName != vnstatPackageName {
			return fmt.Errorf("refuse to remove unexpected registered vnstat package: %s", packageName)
		}
		for _, command := range manager.RemovePlan {
			if err := runInstallCommand(command); err != nil {
				return fmt.Errorf("remove registered vnstat package: %w", err)
			}
		}
	}
	for _, path := range paths {
		if isUninstallPathWithin(path, options.DataDir) {
			continue
		}
		action, err := ownedUninstallResourcePathAction(resource, path)
		if err != nil {
			return err
		}
		if action != ownedUninstallPathRemove {
			if uninstallPathExists(path) {
				return fmt.Errorf("refuse to remove unverified vnstat resource path: %s", path)
			}
			continue
		}
		if err := removeOwnedUninstallPath(path); err != nil {
			return fmt.Errorf("remove registered vnstat path %s: %w", path, err)
		}
		report.addRemoved(path)
	}
	return nil
}

// stopRegisteredVnstatProcessesForUninstall is the database-independent
// fallback for a damaged or already-removed vnStat settings record. The host
// ownership record must name the exact standard daemon path before this code
// touches a process, including a Linux executable shown as "(deleted)".
func stopRegisteredVnstatProcessesForUninstall(resource HostResource) error {
	seen := make(map[string]struct{})
	for _, rawPath := range resource.Paths {
		daemonPath := normalizeVnstatPath(rawPath)
		if daemonPath != normalizeVnstatPath("/usr/sbin/vnstatd") {
			continue
		}
		if _, exists := seen[daemonPath]; exists {
			continue
		}
		seen[daemonPath] = struct{}{}
		if err := terminateVnstatPIDs(vnstatDaemonPIDsAtPath(daemonPath)); err != nil {
			return fmt.Errorf("stop registered vnstat daemon %s: %w", daemonPath, err)
		}
		if remaining := vnstatDaemonPIDsAtPath(daemonPath); len(remaining) > 0 {
			return fmt.Errorf("registered vnstat daemon is still running at %s: %v", daemonPath, remaining)
		}
	}
	return nil
}

func cleanupPanelRuntimeForUninstall(store *hostOwnershipStore, options KworUninstallOptions, report *UninstallReport) error {
	manifest, _, err := store.Load()
	if err != nil {
		return err
	}
	if manifest == nil {
		return errors.New("host ownership manifest disappeared before runtime cleanup")
	}
	var panel *HostResource
	for index := range manifest.Resources {
		if manifest.Resources[index].ID == "panel-runtime" {
			panel = &manifest.Resources[index]
			break
		}
	}
	if panel == nil {
		return errors.New("panel runtime ownership record is missing")
	}
	var failures []error
	if err := store.MarkState(panel.ID, hostResourceStateCleanupPending); err != nil {
		failures = append(failures, fmt.Errorf("mark panel runtime for cleanup: %w", err))
	}
	paths := append([]string{}, panel.Paths...)
	paths = append(paths, options.RuntimePaths...)
	paths = append(paths, options.DatabasePath, options.LegacyDatabasePath, options.DataDir)
	paths = normalizeOwnershipStrings(paths, true)
	sort.Slice(paths, func(left, right int) bool {
		return len(paths[left]) > len(paths[right])
	})
	type panelRuntimeCleanupPlan struct {
		path     string
		remove   bool
		mutable  bool
		preserve bool
	}
	plans := make([]panelRuntimeCleanupPlan, 0, len(paths))
	// Build every target before removal. A path-level failure is recorded below
	// without preventing independent kwor runtime paths from being removed.
	for _, path := range paths {
		if path == "" || path == HostOwnershipManifestPath() {
			continue
		}
		if options.PreservePanelRuntime && sameUninstallPath(path, options.PanelBinaryPath) {
			continue
		}
		if panelRuntimePathCanRemoveWithoutHash(*panel, path, options) {
			plans = append(plans, panelRuntimeCleanupPlan{path: path, remove: true, mutable: true})
			continue
		}
		action, err := ownedUninstallResourcePathAction(*panel, path)
		if err != nil {
			failures = append(failures, err)
			continue
		}
		if action == ownedUninstallPathUnresolved {
			report.addWarning("保留无法确认归属的面板运行文件: " + path)
			continue
		}
		if action != ownedUninstallPathRemove {
			plans = append(plans, panelRuntimeCleanupPlan{path: path, preserve: uninstallPathExists(path)})
			continue
		}
		plans = append(plans, panelRuntimeCleanupPlan{path: path, remove: true})
	}
	for _, plan := range plans {
		if plan.preserve {
			report.addPreserved(plan.path)
			continue
		}
		if !plan.remove {
			continue
		}
		if err := removeOwnedUninstallPath(plan.path); err != nil {
			var removeErr error
			if plan.mutable {
				removeErr = fmt.Errorf("remove mutable panel runtime path %s: %w", plan.path, err)
			} else {
				removeErr = fmt.Errorf("remove panel runtime path %s: %w", plan.path, err)
			}
			failures = append(failures, removeErr)
			continue
		}
		report.addRemoved(plan.path)
	}
	if len(failures) > 0 {
		return errors.Join(failures...)
	}
	if err := store.Remove(panel.ID); err != nil {
		return err
	}
	return nil
}

// panelRuntimePathCanRemoveWithoutHash 只处理面板运行期会自然变化的路径，
// 尤其是 Promanager_data 树和旧版 ./db 数据库。目录外静态运行文件由
// 精确的写前归属记录确认；内容变化只会记录为诊断信息，不阻止卸载。
func panelRuntimePathCanRemoveWithoutHash(resource HostResource, path string, options KworUninstallOptions) bool {
	if !hostResourceHasVerifiedPostWriteState(resource) {
		return false
	}
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "" || path == "." || path == "/" || !uninstallPathExists(path) {
		return false
	}
	if safePanelDataDirForUninstall(options.DataDir) && isUninstallPathWithin(path, options.DataDir) {
		return true
	}
	if safeLegacyPanelDatabasePathForUninstall(path, options) {
		return true
	}
	return false
}

func safePanelDataDirForUninstall(dataDir string) bool {
	dataDir = filepath.Clean(strings.TrimSpace(dataDir))
	if dataDir == "" || dataDir == "." || dataDir == "/" {
		return false
	}
	return filepath.Base(dataDir) == "Promanager_data"
}

func safeLegacyPanelDatabasePathForUninstall(path string, options KworUninstallOptions) bool {
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "" || path == "." || path == "/" {
		return false
	}
	if !sameUninstallPath(path, options.LegacyDatabasePath) {
		return false
	}
	if filepath.Base(path) != config.GetName()+".db" {
		return false
	}
	legacyDBDir := filepath.Join(options.PanelBinDir, "db")
	return isUninstallPathWithin(path, legacyDBDir)
}

type ownedUninstallPathAction uint8

const (
	ownedUninstallPathAbsent ownedUninstallPathAction = iota
	ownedUninstallPathRemove
	ownedUninstallPathPreserve
	ownedUninstallPathUnresolved
)

func ownedUninstallResourcePathAction(resource HostResource, path string) (ownedUninstallPathAction, error) {
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "" || !uninstallPathExists(path) {
		return ownedUninstallPathAbsent, nil
	}
	before, recordedBefore := resource.Before[path]
	if !hostResourceHasVerifiedPostWriteState(resource) {
		if recordedBefore && before.Existed {
			return ownedUninstallPathPreserve, nil
		}
		if recordedBefore && !before.Existed {
			// The path was reserved as a project-created resource. A failed
			// write must not turn a later uninstall into an undeletable install.
			return ownedUninstallPathRemove, nil
		}
		return ownedUninstallPathUnresolved, nil
	}
	if !recordedBefore || before.Existed {
		return ownedUninstallPathPreserve, nil
	}
	// An active record with a non-existing pre-write state is exact ownership
	// evidence. Hashes remain useful for diagnostics, but user edits must not
	// block a requested default uninstall of a known kwor resource.
	return ownedUninstallPathRemove, nil
}

func pendingOwnedUninstallPathCanRemove(resource HostResource, path string) bool {
	before, recorded := resource.Before[filepath.Clean(path)]
	if !recorded || before.Existed {
		return false
	}
	switch resource.Kind {
	case HostResourceHostFile:
		return resource.CleanupPolicy == HostCleanupDelete && legacyHostFileLooksOwned(path)
	case HostResourceUpdateWorkspace:
		return panelUpdateWorkspaceDirectoryOwned(path)
	default:
		return false
	}
}

func activeMutableOwnedUninstallPathCanRemove(resource HostResource, path string) bool {
	if resource.Kind != HostResourceUpdateWorkspace || !hostResourceHasVerifiedPostWriteState(resource) {
		return false
	}
	path = filepath.Clean(strings.TrimSpace(path))
	if panelUpdateWorkspaceDirectoryOwned(path) {
		return true
	}
	if panelUpdateWorkspacePathStrict(path) {
		entries, err := os.ReadDir(path)
		return err == nil && len(entries) == 0
	}
	targetBinary := filepath.Clean(strings.TrimSpace(resource.Metadata["targetBinary"]))
	if targetBinary == "" || targetBinary == "." || path != targetBinary+".bak" {
		return false
	}
	before, recorded := resource.Before[path]
	return recorded && !before.Existed
}

func shouldClearOwnedHostFileImmutable(resource HostResource, path string) (bool, error) {
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "" || !uninstallPathExists(path) {
		return false, nil
	}
	before, recorded := resource.Before[path]
	if !recorded {
		return false, nil
	}
	// Existing v1 ledgers did not record the immutable baseline. Preserve an
	// unknown or originally immutable flag rather than clearing a user flag.
	if before.Existed && (!before.ImmutableKnown || before.Immutable) {
		return false, nil
	}
	if hostResourceHasVerifiedPostWriteState(resource) {
		expectedHash := strings.TrimSpace(resource.Hashes[path])
		if expectedHash == "" {
			return false, nil
		}
		actualHash := ownershipPathHashes([]string{path})[path]
		if actualHash == "" || actualHash != expectedHash {
			return false, nil
		}
	}
	immutable, err := ownedHostFileImmutableDetectFn(path)
	if err != nil {
		return false, fmt.Errorf("detect immutable state for %s: %w", path, err)
	}
	return immutable, nil
}

func shouldRemoveOwnedUninstallResourcePath(resource HostResource, path string) (bool, error) {
	action, err := ownedUninstallResourcePathAction(resource, path)
	if err != nil {
		return false, err
	}
	return action == ownedUninstallPathAbsent || action == ownedUninstallPathRemove, nil
}

func verifyOwnedUninstallResourcePath(resource HostResource, path string) error {
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "" || !uninstallPathExists(path) {
		return nil
	}
	expectedHash := strings.TrimSpace(resource.Hashes[path])
	if expectedHash == "" {
		return nil
	}
	actualHash := ownershipPathHashes([]string{path})[path]
	if actualHash == "" {
		return fmt.Errorf("cannot verify %s resource path before removal: %s", resource.Kind, path)
	}
	if actualHash != expectedHash {
		return fmt.Errorf("refuse to remove modified %s resource path: %s", resource.Kind, path)
	}
	return nil
}

func removeOwnedUninstallPath(path string) error {
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "" || path == "." || path == "/" {
		return fmt.Errorf("refuse to remove unsafe path %q", path)
	}
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	} else {
		if err := os.RemoveAll(path); err != nil {
			return err
		}
	}
	if uninstallPathExists(path) {
		return fmt.Errorf("path remains after removal")
	}
	return nil
}

func uninstallPathExists(path string) bool {
	_, err := os.Lstat(strings.TrimSpace(path))
	return err == nil
}

func isUninstallPathWithin(path string, parent string) bool {
	path = filepath.Clean(strings.TrimSpace(path))
	parent = filepath.Clean(strings.TrimSpace(parent))
	if path == "" || parent == "" {
		return false
	}
	rel, err := filepath.Rel(parent, path)
	if err != nil {
		return false
	}
	return rel == "." || (rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)))
}

func sameUninstallPath(left string, right string) bool {
	left = filepath.Clean(strings.TrimSpace(left))
	right = filepath.Clean(strings.TrimSpace(right))
	if left == "" || right == "" {
		return false
	}
	if runtime.GOOS == "windows" {
		return strings.EqualFold(left, right)
	}
	return left == right
}

func manifestResources(manifest *HostOwnershipManifest, kind HostResourceKind) []HostResource {
	if manifest == nil {
		return nil
	}
	result := make([]HostResource, 0)
	for _, resource := range manifest.Resources {
		if resource.Kind == kind {
			result = append(result, resource)
		}
	}
	return result
}

// ReconcileIncompleteKworUninstall runs before a fresh start creates new
// systemd units. A normal installation has a manifest with Uninstalling=false
// and is never touched by this path.
func ReconcileIncompleteKworUninstall(options KworUninstallOptions) error {
	if runtime.GOOS != "linux" || runningInsideContainer() {
		return nil
	}
	pending, err := IsKworUninstalling()
	if err != nil {
		return err
	}
	if !pending {
		state, found, stateErr := LoadKworUninstallLifecycleState()
		if stateErr != nil {
			return stateErr
		}
		if !found || state == nil || !lifecycleStateBlocksNewWork(*state) {
			return nil
		}
		if lifecycleUninstallTakeoverBlocked(*state, kworLifecycleNowFn(), lifecycleUninstallWorkerAlive(*state)) {
			return errors.New("已有卸载 worker 正在运行，拒绝同时启动恢复")
		}
	}
	options.PreservePanelRuntime = true
	_, err = PerformKworUninstall(options)
	return err
}

// MigrateLegacyKworHostOwnership adopts only resources that can be identified
// from their fixed project paths or their unit/rule contents. It runs before a
// normal start so future cleanup no longer depends on the panel database.
func MigrateLegacyKworHostOwnership(options KworUninstallOptions) error {
	if runtime.GOOS != "linux" || runningInsideContainer() {
		return nil
	}
	options = normalizeKworUninstallOptions(options)
	lock, err := AcquireKworLifecycleLock()
	if err != nil {
		return err
	}
	defer func() { _ = lock.Release() }()
	store := defaultHostOwnershipStore()

	if manifest, found, loadErr := LoadHostOwnershipManifest(); loadErr != nil {
		return loadErr
	} else if found && manifest != nil && strings.TrimSpace(manifest.HostID) != "" && manifest.HostID != hostOwnershipHostID() {
		return errors.New("host ownership manifest belongs to a different host")
	}

	units := append([]string{}, uninstallLegacySystemdUnits...)
	units = append(units, acmeSystemdUnitCandidates...)
	units = append(units, options.PanelServiceName, GetSingboxSystemdName(), GetMihomoSystemdName(), "kwor-vnstat")
	seenUnits := make(map[string]struct{}, len(units))
	for _, unit := range units {
		unit = strings.TrimSpace(unit)
		if unit == "" {
			continue
		}
		if _, exists := seenUnits[unit]; exists {
			continue
		}
		seenUnits[unit] = struct{}{}
		if !legacySystemdUnitLooksOwned(unit, options) {
			continue
		}
		paths := ownedSystemdUnitArtifactPaths(unit, options, nil, nil, nil, true)
		if len(paths) == 0 {
			continue
		}
		resourceID := "legacy-systemd-" + strings.ReplaceAll(unit, "/", "_")
		if err := RegisterSystemdHostOwnership(resourceID, unit, paths, map[string]string{"migrated": "legacy-content"}); err != nil {
			return fmt.Errorf("migrate legacy systemd unit %s: %w", unit, err)
		}
	}

	if tables, tableErr := detectLegacyOwnedNftTables(); tableErr != nil {
		return tableErr
	} else if len(tables) > 0 {
		if err := RegisterNftHostOwnership("legacy-nftables", tables); err != nil {
			return fmt.Errorf("migrate legacy nftables ownership: %w", err)
		}
	}

	for _, path := range uninstallDedicatedSysctlFiles {
		if !uninstallPathExists(path) {
			continue
		}
		if !legacyHostFileLooksOwned(path) {
			continue
		}
		resourceID := "legacy-host-file-" + strings.ReplaceAll(filepath.Base(path), ".", "-")
		if err := adoptLegacyMarkedHostFileOwnership(store, resourceID, path, HostCleanupDelete); err != nil {
			return fmt.Errorf("migrate legacy sysctl file %s: %w", path, err)
		}
	}

	mtuScriptPath := filepath.Join(options.DataDir, "mtu", managedMTUScriptFileName)
	if uninstallPathExists(mtuScriptPath) && legacyHostFileLooksOwned(mtuScriptPath) {
		if err := adoptLegacyMarkedHostFileOwnership(store, "mtu-script", mtuScriptPath, HostCleanupDelete); err != nil {
			return fmt.Errorf("migrate legacy MTU script: %w", err)
		}
	}

	acmeRoot := filepath.Clean(filepath.Join(options.DataDir, "acme"))
	if uninstallPathExists(filepath.Join(acmeRoot, "acme.sh")) {
		if err := RegisterAcmeHostOwnership([]string{acmeRoot}, nil); err != nil {
			return fmt.Errorf("migrate managed acme runtime: %w", err)
		}
	}
	return nil
}

func adoptLegacyMarkedHostFileOwnership(store *hostOwnershipStore, id string, path string, cleanupPolicy string) error {
	if store == nil {
		return errors.New("host ownership store is nil")
	}
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "" || !legacyHostFileLooksOwned(path) {
		return errors.New("legacy host file has no kwor ownership marker")
	}
	resource, err := store.Upsert(HostResource{
		ID:            id,
		Kind:          HostResourceHostFile,
		State:         hostResourceStatePending,
		CleanupPolicy: cleanupPolicy,
		Paths:         []string{path},
		Before:        ownershipPathsAssumedNew([]string{path}),
	})
	if err != nil {
		return err
	}
	return VerifyAndActivateHostResourceForStore(store, resource.ID)
}

func legacyHostFileLooksOwned(path string) bool {
	path = filepath.Clean(strings.TrimSpace(path))
	if path == "" || !uninstallPathExists(path) {
		return false
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(string(content)), kworOwnershipMarker)
}
