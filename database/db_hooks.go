package database

import "sync"

var (
	dbResetHooksMu sync.Mutex
	dbResetHooks   []func()
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
