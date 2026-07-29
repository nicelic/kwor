//go:build !linux

package service

import "os/exec"

func acquireKworLifecycleMetadataLock() (func() error, error) {
	return func() error { return nil }, nil
}

func StartKworLifecycleControlServer() error { return nil }

func StopKworLifecycleControlServer() {}

func RequestKworLifecycleQuiesce() error { return QuiesceKworManagedOperations() }

func lifecycleUninstallWorkerAlive(KworUninstallLifecycleState) bool { return false }

func terminateKworManagedOperation(KworManagedOperationRecord) error { return nil }

func kworManagedOperationAlive(KworManagedOperationRecord) (bool, error) { return false, nil }

func kworLifecycleProcessStartTime(int) (uint64, error) { return 0, nil }

func kworManagedOperationProcessIdentity(int, string) (uint64, error) { return 0, nil }

func verifyKworPanelUpdateSystemdUnit(string, string, string, bool) (bool, error) { return false, nil }

func clearKworLifecycleRuntimeArtifactsLocked() error { return nil }

func finishKworLifecycleRuntimeCleanup() error { return nil }

func syncKworLifecycleDirectory(string) error { return nil }

func kworManagedOperationProcessGroup(int) int { return 0 }

func stopStartedKworDetachedWorker(int, int, uint64) error { return nil }

func prepareKworManagedCommand(*exec.Cmd) {}
