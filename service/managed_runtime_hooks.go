package service

import (
	"context"
	"database/sql"
	"reflect"
	"sync"

	"gorm.io/gorm"
)

type managedRuntimeHookScope struct {
	mu    sync.Mutex
	hooks []func() error
}

var managedRuntimeHookScopes sync.Map

type managedRuntimeHookTx struct {
	connPool    gorm.ConnPool
	txCommitter gorm.TxCommitter
	tx          gorm.Tx
	scopeDB     *gorm.DB
}

func BeginManagedRuntimeHookScope(db *gorm.DB) {
	if db == nil {
		return
	}
	managedRuntimeHookScopes.LoadOrStore(db, &managedRuntimeHookScope{})
}

func QueueManagedRuntimeHook(db *gorm.DB, hook func() error) error {
	if hook == nil {
		return nil
	}
	if db == nil {
		return hook()
	}

	scopeValue, ok := managedRuntimeHookScopes.Load(db)
	if !ok {
		autoScope, autoScopeOK := ensureManagedRuntimeHookAutoScope(db)
		if !autoScopeOK {
			return hook()
		}
		scopeValue = autoScope
	}

	scope := scopeValue.(*managedRuntimeHookScope)
	scope.mu.Lock()
	scope.hooks = append(scope.hooks, hook)
	scope.mu.Unlock()
	return nil
}

func RunManagedRuntimeHookScope(db *gorm.DB) error {
	if db == nil {
		return nil
	}

	scopeValue, ok := managedRuntimeHookScopes.LoadAndDelete(db)
	if !ok {
		return nil
	}

	scope := scopeValue.(*managedRuntimeHookScope)
	scope.mu.Lock()
	hooks := append([]func() error(nil), scope.hooks...)
	scope.hooks = nil
	scope.mu.Unlock()

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

func DiscardManagedRuntimeHookScope(db *gorm.DB) {
	if db == nil {
		return
	}
	managedRuntimeHookScopes.Delete(db)
}

func ensureManagedRuntimeHookAutoScope(db *gorm.DB) (*managedRuntimeHookScope, bool) {
	if db == nil || db.Statement == nil || db.Statement.ConnPool == nil {
		return nil, false
	}

	if scopeValue, ok := managedRuntimeHookScopes.Load(db); ok {
		return scopeValue.(*managedRuntimeHookScope), true
	}

	txCommitter, ok := db.Statement.ConnPool.(gorm.TxCommitter)
	if !ok || txCommitter == nil {
		return nil, false
	}
	if reflect.ValueOf(txCommitter).Kind() == reflect.Ptr && reflect.ValueOf(txCommitter).IsNil() {
		return nil, false
	}

	scope := &managedRuntimeHookScope{}
	actual, loaded := managedRuntimeHookScopes.LoadOrStore(db, scope)
	if loaded {
		return actual.(*managedRuntimeHookScope), true
	}

	if err := wrapManagedRuntimeHookTransaction(db, txCommitter); err != nil {
		managedRuntimeHookScopes.Delete(db)
		return nil, false
	}

	return scope, true
}

func wrapManagedRuntimeHookTransaction(db *gorm.DB, txCommitter gorm.TxCommitter) error {
	if db == nil || db.Statement == nil || db.Statement.ConnPool == nil {
		return gorm.ErrInvalidTransaction
	}

	if _, ok := db.Statement.ConnPool.(*managedRuntimeHookTx); ok {
		return nil
	}

	wrapper := &managedRuntimeHookTx{
		connPool:    db.Statement.ConnPool,
		txCommitter: txCommitter,
		scopeDB:     db,
	}
	if tx, ok := db.Statement.ConnPool.(gorm.Tx); ok {
		wrapper.tx = tx
	}

	db.Statement.ConnPool = wrapper
	return nil
}

func (tx *managedRuntimeHookTx) PrepareContext(ctx context.Context, query string) (*sql.Stmt, error) {
	return tx.connPool.PrepareContext(ctx, query)
}

func (tx *managedRuntimeHookTx) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	return tx.connPool.ExecContext(ctx, query, args...)
}

func (tx *managedRuntimeHookTx) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	return tx.connPool.QueryContext(ctx, query, args...)
}

func (tx *managedRuntimeHookTx) QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row {
	return tx.connPool.QueryRowContext(ctx, query, args...)
}

func (tx *managedRuntimeHookTx) Commit() error {
	if tx.txCommitter == nil {
		return gorm.ErrInvalidTransaction
	}
	if err := tx.txCommitter.Commit(); err != nil {
		DiscardManagedRuntimeHookScope(tx.scopeDB)
		return err
	}
	return RunManagedRuntimeHookScope(tx.scopeDB)
}

func (tx *managedRuntimeHookTx) Rollback() error {
	DiscardManagedRuntimeHookScope(tx.scopeDB)
	if tx.txCommitter == nil {
		return gorm.ErrInvalidTransaction
	}
	return tx.txCommitter.Rollback()
}

func (tx *managedRuntimeHookTx) StmtContext(ctx context.Context, stmt *sql.Stmt) *sql.Stmt {
	if tx.tx != nil {
		return tx.tx.StmtContext(ctx, stmt)
	}
	return stmt
}
