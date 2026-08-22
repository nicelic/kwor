package database

import (
	"errors"
	"sync"
)

var (
	dbResetHooksMu       sync.Mutex
	dbResetHooks         []func()
	dbLifecycleHooksMu   sync.Mutex
	dbBeforeBackupHooks  []func() error
	dbBeforeRestoreHooks []func() error
	dbAfterRestoreHooks  []func()
	dbRestoreAbortHooks  []func()
)

func RegisterDBResetHook(hook func()) {
	if hook == nil {
		return
	}

	dbResetHooksMu.Lock()
	dbResetHooks = append(dbResetHooks, hook)
	dbResetHooksMu.Unlock()
}

func runDBResetHooks() {
	dbResetHooksMu.Lock()
	hooks := append([]func(){}, dbResetHooks...)
	dbResetHooksMu.Unlock()

	for _, hook := range hooks {
		if hook != nil {
			hook()
		}
	}
}

func RegisterDBBeforeBackupHook(hook func() error) {
	if hook == nil {
		return
	}
	dbLifecycleHooksMu.Lock()
	dbBeforeBackupHooks = append(dbBeforeBackupHooks, hook)
	dbLifecycleHooksMu.Unlock()
}

func RegisterDBBeforeRestoreHook(hook func() error) {
	if hook == nil {
		return
	}
	dbLifecycleHooksMu.Lock()
	dbBeforeRestoreHooks = append(dbBeforeRestoreHooks, hook)
	dbLifecycleHooksMu.Unlock()
}

func RegisterDBAfterRestoreHook(hook func()) {
	if hook == nil {
		return
	}
	dbLifecycleHooksMu.Lock()
	dbAfterRestoreHooks = append(dbAfterRestoreHooks, hook)
	dbLifecycleHooksMu.Unlock()
}

func RegisterDBRestoreAbortHook(hook func()) {
	if hook == nil {
		return
	}
	dbLifecycleHooksMu.Lock()
	dbRestoreAbortHooks = append(dbRestoreAbortHooks, hook)
	dbLifecycleHooksMu.Unlock()
}

func runDBBeforeBackupHooks() error {
	dbLifecycleHooksMu.Lock()
	hooks := append([]func() error(nil), dbBeforeBackupHooks...)
	dbLifecycleHooksMu.Unlock()
	var firstErr error
	for _, hook := range hooks {
		if hook == nil {
			continue
		}
		if err := hook(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func runDBBeforeRestoreHooks() error {
	dbLifecycleHooksMu.Lock()
	hooks := append([]func() error(nil), dbBeforeRestoreHooks...)
	dbLifecycleHooksMu.Unlock()
	var errs []error
	for _, hook := range hooks {
		if hook == nil {
			continue
		}
		if err := hook(); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

func runDBAfterRestoreHooks() {
	dbLifecycleHooksMu.Lock()
	hooks := append([]func(){}, dbAfterRestoreHooks...)
	dbLifecycleHooksMu.Unlock()
	for _, hook := range hooks {
		if hook != nil {
			hook()
		}
	}
}

func runDBRestoreAbortHooks() {
	dbLifecycleHooksMu.Lock()
	hooks := append([]func(){}, dbRestoreAbortHooks...)
	dbLifecycleHooksMu.Unlock()
	for _, hook := range hooks {
		if hook != nil {
			hook()
		}
	}
}
