package service

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"
)

const (
	coreReleaseAssetRequestTimeout = 300 * time.Second
	coreBinaryDownloadTimeout      = 30 * time.Minute
	coreManagedTaskDeadline        = 35 * time.Minute
	coreManagedTaskTerminalTTL     = 40 * time.Minute
)

var coreDownloadTaskHandles sync.Map // map[string]*ManagedDownloadTaskHandle

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
	if strings.EqualFold(strings.TrimSpace(coreName), "mihomo") {
		return GetMihomoManagedCoreDownloadProgress(id)
	}
	return GetSingboxManagedCoreDownloadProgress(id)
}

func StopManagedCoreDownload(coreName string, id string) (*CoreDownloadProgress, error) {
	if strings.EqualFold(strings.TrimSpace(coreName), "mihomo") {
		return StopMihomoManagedCoreDownload(id)
	}
	return StopSingboxManagedCoreDownload(id)
}

func coreDownloadTaskCancelledError(ctx context.Context) error {
	if ctx != nil && ctx.Err() != nil {
		return ctx.Err()
	}
	return fmt.Errorf("core download task stopped")
}
