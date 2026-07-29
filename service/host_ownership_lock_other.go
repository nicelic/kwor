//go:build !linux

package service

import "sync"

// Non-Linux builds do not create host systemd/nftables resources. Keep an
// in-process lock so shared lifecycle callers remain safe and portable.
var kworLifecycleFallbackMu sync.Mutex

type KworLifecycleLock struct{}

func AcquireKworLifecycleLock() (*KworLifecycleLock, error) {
	kworLifecycleFallbackMu.Lock()
	return &KworLifecycleLock{}, nil
}

func (l *KworLifecycleLock) Release() error {
	kworLifecycleFallbackMu.Unlock()
	return nil
}
