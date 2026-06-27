package database

import "sync"

var (
	dbRestoreHooksMu sync.Mutex
	beforeDBRestore  []func() error
	afterDBRestore   []func() error
)

func RegisterBeforeDBRestoreHook(hook func() error) {
	if hook == nil {
		return
	}
	dbRestoreHooksMu.Lock()
	beforeDBRestore = append(beforeDBRestore, hook)
	dbRestoreHooksMu.Unlock()
}

func RegisterAfterDBRestoreHook(hook func() error) {
	if hook == nil {
		return
	}
	dbRestoreHooksMu.Lock()
	afterDBRestore = append(afterDBRestore, hook)
	dbRestoreHooksMu.Unlock()
}

func runBeforeDBRestoreHooks() error {
	dbRestoreHooksMu.Lock()
	hooks := append([]func() error{}, beforeDBRestore...)
	dbRestoreHooksMu.Unlock()

	for _, hook := range hooks {
		if hook == nil {
			continue
		}
		if err := hook(); err != nil {
			return err
		}
	}
	return nil
}

func runAfterDBRestoreHooks() error {
	dbRestoreHooksMu.Lock()
	hooks := append([]func() error{}, afterDBRestore...)
	dbRestoreHooksMu.Unlock()

	for _, hook := range hooks {
		if hook == nil {
			continue
		}
		if err := hook(); err != nil {
			return err
		}
	}
	return nil
}
