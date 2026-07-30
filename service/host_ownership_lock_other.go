//go:build !linux

package service

import (
	"context"
	"sync"
	"time"
)

// Non-Linux builds do not create host systemd/nftables resources. Keep an
// in-process lock so shared lifecycle callers remain safe and portable.
var kworLifecycleFallbackMu sync.Mutex

type KworLifecycleLock struct{}

func AcquireKworLifecycleLock() (*KworLifecycleLock, error) {
	return AcquireKworLifecycleLockContext(context.Background())
}

func AcquireKworLifecycleLockContext(ctx context.Context) (*KworLifecycleLock, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if kworLifecycleFallbackMu.TryLock() {
		return &KworLifecycleLock{}, nil
	}

	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-ticker.C:
			if kworLifecycleFallbackMu.TryLock() {
				return &KworLifecycleLock{}, nil
			}
		}
	}
}

func (l *KworLifecycleLock) Release() error {
	kworLifecycleFallbackMu.Unlock()
	return nil
}
