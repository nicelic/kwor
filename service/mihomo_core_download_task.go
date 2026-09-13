package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/alireza0/s-ui/logger"
)

var mihomoCoreDownloadTaskManager = NewManagedDownloadTaskManagerWithOptions("mihomo core download", ManagedDownloadTaskManagerOptions{
	Deadline:    coreManagedTaskDeadline,
	TerminalTTL: coreManagedTaskTerminalTTL,
})

func GetMihomoManagedCoreDownloadProgress(id string) *CoreDownloadProgress {
	task := mihomoCoreDownloadTaskManager.Get(id)
	progressID := strings.TrimSpace(id)
	if progressID == "" && task.ID != "" {
		progressID = task.ID
	}
	progress := GetCoreDownloadProgress(progressID)
	return applyManagedTaskToCoreProgress(progress, "mihomo", task)
}

func StopMihomoManagedCoreDownload(id string) (*CoreDownloadProgress, error) {
	task, err := mihomoCoreDownloadTaskManager.Stop(id)
	progress := GetMihomoManagedCoreDownloadProgress(id)
	progress = applyManagedTaskToCoreProgress(progress, "mihomo", task)
	return progress, err
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
