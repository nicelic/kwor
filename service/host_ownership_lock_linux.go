//go:build linux

package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/sys/unix"
)

const hostOwnershipLifecycleLockPath = "/run/kwor/lifecycle.lock"

type KworLifecycleLock struct {
	file *os.File
	once sync.Once
}

func AcquireKworLifecycleLock() (*KworLifecycleLock, error) {
	if err := os.MkdirAll(filepath.Dir(hostOwnershipLifecycleLockPath), 0o750); err != nil {
		return nil, fmt.Errorf("create kwor lifecycle lock directory: %w", err)
	}
	if err := os.Chmod(filepath.Dir(hostOwnershipLifecycleLockPath), 0o750); err != nil {
		return nil, fmt.Errorf("set kwor lifecycle lock directory permissions: %w", err)
	}
	file, err := os.OpenFile(hostOwnershipLifecycleLockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open kwor lifecycle lock: %w", err)
	}
	if err := file.Chmod(0o600); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("set kwor lifecycle lock permissions: %w", err)
	}
	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		_ = file.Close()
		return nil, fmt.Errorf("another kwor lifecycle operation is already running: %w", err)
	}
	return &KworLifecycleLock{file: file}, nil
}

// AcquireKworLifecycleLockContext keeps the background-task API symmetric on
// Linux. flock is deliberately non-blocking here, so cancellation is checked
// before the single acquisition attempt.
func AcquireKworLifecycleLockContext(ctx context.Context) (*KworLifecycleLock, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return AcquireKworLifecycleLock()
}

func (l *KworLifecycleLock) Release() error {
	if l == nil || l.file == nil {
		return nil
	}
	var result error
	l.once.Do(func() {
		if err := unix.Flock(int(l.file.Fd()), unix.LOCK_UN); err != nil {
			result = err
		}
		if err := l.file.Close(); err != nil && result == nil {
			result = err
		}
	})
	return result
}
