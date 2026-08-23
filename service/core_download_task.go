package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/alireza0/s-ui/logger"
)

const (
	coreReleaseAssetRequestTimeout = 300 * time.Second
	coreBinaryDownloadTimeout      = 30 * time.Minute
	coreManagedTaskDeadline        = 35 * time.Minute
	coreManagedTaskTerminalTTL     = 40 * time.Minute
)

var singboxCoreDownloadTaskManager = NewManagedDownloadTaskManagerWithOptions("sing-box core download", ManagedDownloadTaskManagerOptions{
	Deadline:    coreManagedTaskDeadline,
	TerminalTTL: coreManagedTaskTerminalTTL,
})
var mihomoCoreDownloadTaskManager = NewManagedDownloadTaskManagerWithOptions("mihomo core download", ManagedDownloadTaskManagerOptions{
	Deadline:    coreManagedTaskDeadline,
	TerminalTTL: coreManagedTaskTerminalTTL,
})

var coreDownloadTaskHandles sync.Map // map[string]*ManagedDownloadTaskHandle

func coreDownloadTaskManagerFor(coreName string) *ManagedDownloadTaskManager {
	if strings.EqualFold(strings.TrimSpace(coreName), "mihomo") {
		return mihomoCoreDownloadTaskManager
	}
	return singboxCoreDownloadTaskManager
}

func registerCoreDownloadTask(handle *ManagedDownloadTaskHandle) {
	if handle == nil || handle.ID() == "" {
		return
	}
	coreDownloadTaskHandles.Store(handle.ID(), handle)
}

func unregisterCoreDownloadTask(handle *ManagedDownloadTaskHandle) {
	if handle == nil || handle.ID() == "" {
		return
	}
	coreDownloadTaskHandles.Delete(handle.ID())
}

func updateManagedCoreDownloadTaskPhase(id string, phase string) {
	value, ok := coreDownloadTaskHandles.Load(strings.TrimSpace(id))
	if !ok {
		return
	}
	handle, ok := value.(*ManagedDownloadTaskHandle)
	if !ok || handle == nil {
		return
	}
	handle.SetPhase(phase, isCoreDownloadCancelableStage(phase))
}

func beginManagedCoreDownloadApplying(id string, phase string) bool {
	value, ok := coreDownloadTaskHandles.Load(strings.TrimSpace(id))
	if !ok {
		return true
	}
	handle, ok := value.(*ManagedDownloadTaskHandle)
	if !ok || handle == nil {
		return true
	}
	return handle.BeginApplying(phase)
}

func isCoreDownloadCancelableStage(phase string) bool {
	switch strings.TrimSpace(phase) {
	case "preparing", coreDownloadStageDownloading, coreDownloadStageExtracting, coreDownloadStageValidating:
		return true
	default:
		return false
	}
}

func applyManagedTaskToCoreProgress(progress *CoreDownloadProgress, coreName string, task ManagedDownloadTaskStatus) *CoreDownloadProgress {
	if progress == nil {
		progress = &CoreDownloadProgress{Status: coreDownloadStatusMissing}
	}
	if task.State == managedDownloadTaskIdle {
		return progress
	}
	if progress.ID == "" {
		progress.ID = task.ID
	}
	if progress.Core == "" {
		progress.Core = strings.TrimSpace(coreName)
	}
	progress.State = task.State
	progress.CanCancel = task.CanCancel
	progress.StopRequested = task.StopRequested
	progress.DeadlineExceeded = task.DeadlineExceeded
	progress.DeadlineAt = task.DeadlineAt
	if task.StartedAt > 0 {
		progress.StartedAt = task.StartedAt
	}
	if task.UpdatedAt > 0 {
		progress.UpdatedAt = task.UpdatedAt
	}
	if task.FinishedAt > 0 {
		progress.FinishedAt = task.FinishedAt
	}
	if task.Phase != "" {
		progress.Stage = task.Phase
	}
	if task.Error != "" {
		progress.Error = task.Error
	}
	switch task.State {
	case managedDownloadTaskQueued, managedDownloadTaskRunning, managedDownloadTaskStopping:
		progress.Status = coreDownloadStatusRunning
	case managedDownloadTaskSuccess:
		progress.Status = coreDownloadStatusSuccess
	case managedDownloadTaskCancelled, managedDownloadTaskTimedOut, managedDownloadTaskError:
		progress.Status = coreDownloadStatusError
	}
	return progress
}

func GetManagedCoreDownloadProgress(coreName string, id string) *CoreDownloadProgress {
	manager := coreDownloadTaskManagerFor(coreName)
	task := manager.Get(id)
	progressID := strings.TrimSpace(id)
	if progressID == "" && task.ID != "" {
		progressID = task.ID
	}
	progress := GetCoreDownloadProgress(progressID)
	return applyManagedTaskToCoreProgress(progress, coreName, task)
}

func StopManagedCoreDownload(coreName string, id string) (*CoreDownloadProgress, error) {
	manager := coreDownloadTaskManagerFor(coreName)
	task, err := manager.Stop(id)
	progress := GetManagedCoreDownloadProgress(coreName, id)
	progress = applyManagedTaskToCoreProgress(progress, coreName, task)
	return progress, err
}

func (s *CoreManagerService) executeManagedSingboxCoreDownloadTask(handle *ManagedDownloadTaskHandle, version string, target SingboxCoreDownloadTarget, customURL string, afterSuccess func()) error {
	if handle == nil {
		return fmt.Errorf("sing-box core download task handle is nil")
	}
	registerCoreDownloadTask(handle)
	defer unregisterCoreDownloadTask(handle)
	defer func() {
		if recovered := recover(); recovered != nil {
			panicErr := fmt.Errorf("sing-box core download task panicked: %v", recovered)
			logger.Error(panicErr)
			handle.FinishError("failed", panicErr)
		}
	}()
	if !handle.MarkRunning("preparing") {
		handle.FinishCancelled("cancelled")
		return context.Canceled
	}
	ctx := handle.Context()
	var downloadErr error
	if customURL != "" {
		_, downloadErr = s.downloadCoreFromURLWithContext(ctx, customURL, handle.ID())
	} else {
		_, downloadErr = s.downloadCoreWithContext(ctx, version, target, handle.ID())
	}
	if downloadErr != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			handle.FinishCancelled("cancelled")
		} else {
			handle.FinishError("failed", downloadErr)
		}
		return downloadErr
	}
	if customURL != "" {
		if saveErr := s.SaveCustomDownloadURL(customURL); saveErr != nil {
			logger.Warning("save core custom download url failed: ", saveErr)
		}
	}
	if afterSuccess != nil {
		afterSuccess()
	}
	handle.FinishSuccess(coreDownloadStageCompleted)
	return nil
}

func (s *CoreManagerService) StartManagedCoreDownload(version string, target SingboxCoreDownloadTarget, customURL string, afterSuccess func()) (*CoreDownloadProgress, error) {
	version = strings.TrimSpace(version)
	customURL = strings.TrimSpace(customURL)
	fingerprint := "release|" + version + "|" + target.OS + "|" + target.Arch + "|" + target.Libc
	if customURL != "" {
		fingerprint = "custom|" + customURL
	}
	handle, status, created, err := singboxCoreDownloadTaskManager.Start("singbox-core-download", fingerprint)
	if err != nil {
		return nil, err
	}
	if !created {
		return applyManagedTaskToCoreProgress(GetCoreDownloadProgress(status.ID), "sing-box", status), nil
	}
	go func() {
		_ = s.executeManagedSingboxCoreDownloadTask(handle, version, target, customURL, afterSuccess)
	}()
	return applyManagedTaskToCoreProgress(GetCoreDownloadProgress(status.ID), "sing-box", status), nil
}

func (s *MihomoCoreManagerService) executeManagedMihomoCoreDownloadTask(handle *ManagedDownloadTaskHandle, version string, target MihomoCoreDownloadTarget, customURL string, afterSuccess func()) error {
	if handle == nil {
		return fmt.Errorf("mihomo core download task handle is nil")
	}
	registerCoreDownloadTask(handle)
	defer unregisterCoreDownloadTask(handle)
	defer func() {
		if recovered := recover(); recovered != nil {
			panicErr := fmt.Errorf("mihomo core download task panicked: %v", recovered)
			logger.Error(panicErr)
			handle.FinishError("failed", panicErr)
		}
	}()
	if !handle.MarkRunning("preparing") {
		handle.FinishCancelled("cancelled")
		return context.Canceled
	}
	ctx := handle.Context()
	var downloadErr error
	if customURL != "" {
		_, downloadErr = s.downloadCoreFromURLWithContext(ctx, customURL, handle.ID())
	} else {
		_, downloadErr = s.downloadCoreWithContext(ctx, version, target, handle.ID())
	}
	if downloadErr != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			handle.FinishCancelled("cancelled")
		} else {
			handle.FinishError("failed", downloadErr)
		}
		return downloadErr
	}
	if customURL != "" {
		if saveErr := s.SaveCustomDownloadURL(customURL); saveErr != nil {
			logger.Warning("save mihomo custom download url failed: ", saveErr)
		}
	}
	if afterSuccess != nil {
		afterSuccess()
	}
	handle.FinishSuccess(coreDownloadStageCompleted)
	return nil
}

func (s *MihomoCoreManagerService) StartManagedCoreDownload(version string, target MihomoCoreDownloadTarget, customURL string, afterSuccess func()) (*CoreDownloadProgress, error) {
	version = strings.TrimSpace(version)
	customURL = strings.TrimSpace(customURL)
	fingerprint := "release|" + version + "|" + target.OS + "|" + target.Arch + "|" + target.Amd64Level
	if customURL != "" {
		fingerprint = "custom|" + customURL
	}
	handle, status, created, err := mihomoCoreDownloadTaskManager.Start("mihomo-core-download", fingerprint)
	if err != nil {
		return nil, err
	}
	if !created {
		return applyManagedTaskToCoreProgress(GetCoreDownloadProgress(status.ID), "mihomo", status), nil
	}
	go func() {
		_ = s.executeManagedMihomoCoreDownloadTask(handle, version, target, customURL, afterSuccess)
	}()
	return applyManagedTaskToCoreProgress(GetCoreDownloadProgress(status.ID), "mihomo", status), nil
}

func coreDownloadTaskCancelledError(ctx context.Context) error {
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	return fmt.Errorf("core download task stopped")
}
