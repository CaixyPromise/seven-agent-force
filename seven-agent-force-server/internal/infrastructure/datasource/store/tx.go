package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sync"

	"github.com/jmoiron/sqlx"
)

type txContextKey struct{}
type consistentSnapshotContextKey struct{}
type afterCommitContextKey struct{}

// afterCommitCallbacks lives only in a transaction context created by one of
// the transactors below. Nested application services append to the same list;
// the callbacks are discarded on rollback and run only after the outermost
// commit succeeds.
type afterCommitCallbacks struct {
	mu                sync.Mutex
	callbacks         []func()
	rollbackCallbacks []func()
	resources         map[string]any
}

func (c *afterCommitCallbacks) addRollback(callback func()) bool {
	if c == nil || callback == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.rollbackCallbacks = append(c.rollbackCallbacks, callback)
	return true
}

func (c *afterCommitCallbacks) add(callback func()) bool {
	if c == nil || callback == nil {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.callbacks = append(c.callbacks, callback)
	return true
}

func (c *afterCommitCallbacks) getOrCreateResource(key string, factory func() (any, error)) (any, bool, error) {
	if c == nil || key == "" || factory == nil {
		return nil, false, errors.New("transaction resource registration is invalid")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if existing, ok := c.resources[key]; ok {
		return existing, false, nil
	}
	resource, err := factory()
	if err != nil {
		return nil, false, err
	}
	if resource == nil {
		return nil, false, errors.New("transaction resource factory returned nil")
	}
	if c.resources == nil {
		c.resources = make(map[string]any)
	}
	c.resources[key] = resource
	return resource, true, nil
}

func (c *afterCommitCallbacks) deleteResource(key string, expected any) {
	if c == nil || key == "" || expected == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if current, ok := c.resources[key]; ok && current == expected {
		delete(c.resources, key)
	}
}

func (c *afterCommitCallbacks) run() {
	if c == nil {
		return
	}
	c.mu.Lock()
	callbacks := append([]func(){}, c.callbacks...)
	c.callbacks = nil
	c.rollbackCallbacks = nil
	c.resources = nil
	c.mu.Unlock()
	for _, callback := range callbacks {
		if callback != nil {
			callback()
		}
	}
}

func (c *afterCommitCallbacks) runRollback() {
	if c == nil {
		return
	}
	c.mu.Lock()
	callbacks := append([]func(){}, c.rollbackCallbacks...)
	c.callbacks = nil
	c.rollbackCallbacks = nil
	c.resources = nil
	c.mu.Unlock()
	for _, callback := range callbacks {
		if callback != nil {
			callback()
		}
	}
}

// RegisterAfterCommit queues a local post-commit action only when ctx belongs
// to a store-managed transaction. It returns false for an unmanaged context,
// allowing callers to use their explicit non-transaction fallback instead of
// pretending a commit hook exists.
func RegisterAfterCommit(ctx context.Context, callback func()) bool {
	if ctx == nil || callback == nil {
		return false
	}
	callbacks, _ := ctx.Value(afterCommitContextKey{}).(*afterCommitCallbacks)
	return callbacks.add(callback)
}

// RegisterAfterRollback queues a local completion action only for a
// store-managed transaction. It is deliberately paired with RegisterAfterCommit
// for resources (such as a DG5 freshness fence) acquired before an outer
// transaction and therefore requiring release on either final outcome.
func RegisterAfterRollback(ctx context.Context, callback func()) bool {
	if ctx == nil || callback == nil {
		return false
	}
	callbacks, _ := ctx.Value(afterCommitContextKey{}).(*afterCommitCallbacks)
	return callbacks.addRollback(callback)
}

// GetOrCreateTransactionResource returns storage owned by the real
// store-managed transaction in ctx. It is intentionally generic: application
// facades can coalesce a lease or similar completion-bound resource without
// importing this datasource package or exposing the transaction internals.
// available=false means ctx is not a store-managed transaction.
func GetOrCreateTransactionResource(ctx context.Context, key string, factory func() (any, error)) (value any, created bool, available bool, err error) {
	if ctx == nil {
		return nil, false, false, nil
	}
	callbacks, _ := ctx.Value(afterCommitContextKey{}).(*afterCommitCallbacks)
	if callbacks == nil {
		return nil, false, false, nil
	}
	value, created, err = callbacks.getOrCreateResource(key, factory)
	return value, created, true, err
}

// DeleteTransactionResource removes a resource only when it is still the
// expected instance. Completion callbacks use it after release so a later
// sibling mutation cannot reuse an already-released advisory lease.
func DeleteTransactionResource(ctx context.Context, key string, expected any) {
	if ctx == nil {
		return
	}
	callbacks, _ := ctx.Value(afterCommitContextKey{}).(*afterCommitCallbacks)
	callbacks.deleteResource(key, expected)
}

type sqlStateCarrier interface {
	SQLState() string
}

// IsSerializationFailure reports retryable transaction serialization conflicts
// without coupling application code to a specific database driver.
func IsSerializationFailure(err error) bool {
	var state sqlStateCarrier
	return errors.As(err, &state) && state.SQLState() == "40001"
}

// InConsistentTransaction reports whether ctx carries the marked
// repeatable-read transaction required by guarded cross-module writers.
func InConsistentTransaction(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	existing := SQLXFromContext(ctx)
	consistent, marked := ctx.Value(consistentSnapshotContextKey{}).(*sqlx.Tx)
	return marked && existing != nil && consistent == existing
}

type Transactor interface {
	Enabled() bool
	WithinTransaction(ctx context.Context, fn func(ctx context.Context) error) error
}

// Snapshotter executes a composed read in one database-supported consistent
// snapshot. Implementations must not silently downgrade to independent reads.
type Snapshotter interface {
	Enabled() bool
	WithinReadOnlySnapshot(ctx context.Context, fn func(ctx context.Context) error) error
}

// ConsistentTransactor is a repeatable-read read-write boundary. It is used
// only by workflows that must perform composed reads and writes against one
// stable view; ordinary transactions are deliberately not treated as snapshots.
type ConsistentTransactor interface {
	Enabled() bool
	WithinConsistentTransaction(ctx context.Context, fn func(ctx context.Context) error) error
}

// SQLTransactor wraps one *sql.DB and exposes a database-agnostic transaction
// boundary to application services. Repository methods continue using
// store.Executor(ctx, r.db) and do not need transaction-specific branches.
type SQLTransactor struct {
	db *sql.DB
}

func NewSQLTransactor(db *sql.DB) *SQLTransactor {
	return &SQLTransactor{db: db}
}

func (t *SQLTransactor) Enabled() bool {
	return t != nil && t.db != nil
}

func (t *SQLTransactor) WithinTransaction(ctx context.Context, fn func(ctx context.Context) error) error {
	if t == nil || t.db == nil {
		return errors.New("datasource transactor is not configured")
	}
	if fn == nil {
		return errors.New("transaction callback must not be nil")
	}
	if TxFromContext(ctx) != nil {
		return fn(ctx)
	}

	tx, err := t.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin transaction: %w", err)
	}

	callbacks := &afterCommitCallbacks{}
	txCtx := context.WithValue(ctx, txContextKey{}, tx)
	txCtx = context.WithValue(txCtx, afterCommitContextKey{}, callbacks)
	if err := fn(txCtx); err != nil {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			callbacks.runRollback()
			return fmt.Errorf("rollback transaction after %v: %w", err, rollbackErr)
		}
		callbacks.runRollback()
		return err
	}

	if err := tx.Commit(); err != nil {
		callbacks.runRollback()
		return fmt.Errorf("commit transaction: %w", err)
	}
	callbacks.run()
	return nil
}

// RegisterAfterCommit implements the narrow optional application hook without
// forcing business services to import this infrastructure package.
func (t *SQLTransactor) RegisterAfterCommit(ctx context.Context, callback func()) bool {
	return RegisterAfterCommit(ctx, callback)
}

func (t *SQLTransactor) RegisterAfterRollback(ctx context.Context, callback func()) bool {
	return RegisterAfterRollback(ctx, callback)
}

func (t *SQLTransactor) GetOrCreateTransactionResource(ctx context.Context, key string, factory func() (any, error)) (any, bool, bool, error) {
	return GetOrCreateTransactionResource(ctx, key, factory)
}

func (t *SQLTransactor) DeleteTransactionResource(ctx context.Context, key string, expected any) {
	DeleteTransactionResource(ctx, key, expected)
}

func TxFromContext(ctx context.Context) *sql.Tx {
	if ctx == nil {
		return nil
	}
	tx, _ := ctx.Value(txContextKey{}).(*sql.Tx)
	return tx
}

// SQLXTransactor wraps one *sqlx.DB and stores both raw *sql.Tx and *sqlx.Tx in
// context so command repositories using store.Executor and query repositories
// using store.SQLXExecutor can share the same transaction boundary.
type SQLXTransactor struct {
	db *sqlx.DB
}

func NewSQLXTransactor(db *sqlx.DB) *SQLXTransactor {
	return &SQLXTransactor{db: db}
}

func (t *SQLXTransactor) Enabled() bool {
	return t != nil && t.db != nil
}

func (t *SQLXTransactor) WithinTransaction(ctx context.Context, fn func(ctx context.Context) error) error {
	if t == nil || t.db == nil {
		return errors.New("datasource transactor is not configured")
	}
	if fn == nil {
		return errors.New("transaction callback must not be nil")
	}
	if SQLXFromContext(ctx) != nil {
		return fn(ctx)
	}

	tx, err := t.db.BeginTxx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin sqlx transaction: %w", err)
	}

	callbacks := &afterCommitCallbacks{}
	txCtx := context.WithValue(ctx, txContextKey{}, tx.Tx)
	txCtx = context.WithValue(txCtx, sqlxContextKey{}, tx)
	txCtx = context.WithValue(txCtx, afterCommitContextKey{}, callbacks)
	if err := fn(txCtx); err != nil {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			callbacks.runRollback()
			return fmt.Errorf("rollback sqlx transaction after %v: %w", err, rollbackErr)
		}
		callbacks.runRollback()
		return err
	}

	if err := tx.Commit(); err != nil {
		callbacks.runRollback()
		return fmt.Errorf("commit sqlx transaction: %w", err)
	}
	callbacks.run()
	return nil
}

// RegisterAfterCommit implements the narrow optional application hook without
// forcing business services to import this infrastructure package.
func (t *SQLXTransactor) RegisterAfterCommit(ctx context.Context, callback func()) bool {
	return RegisterAfterCommit(ctx, callback)
}

func (t *SQLXTransactor) RegisterAfterRollback(ctx context.Context, callback func()) bool {
	return RegisterAfterRollback(ctx, callback)
}

func (t *SQLXTransactor) GetOrCreateTransactionResource(ctx context.Context, key string, factory func() (any, error)) (any, bool, bool, error) {
	return GetOrCreateTransactionResource(ctx, key, factory)
}

func (t *SQLXTransactor) DeleteTransactionResource(ctx context.Context, key string, expected any) {
	DeleteTransactionResource(ctx, key, expected)
}

// WithinReadOnlySnapshot provides the stable view required when one logical
// read is decomposed into several bounded set queries. Repeatable read is
// supported by both MySQL and PostgreSQL and avoids impossible compositions
// when related rows change between component queries.
func (t *SQLXTransactor) WithinReadOnlySnapshot(ctx context.Context, fn func(ctx context.Context) error) error {
	if t == nil || t.db == nil {
		return errors.New("datasource snapshotter is not configured")
	}
	if fn == nil {
		return errors.New("snapshot callback must not be nil")
	}
	if existing := SQLXFromContext(ctx); existing != nil {
		snapshotTx, marked := ctx.Value(consistentSnapshotContextKey{}).(*sqlx.Tx)
		if !marked || snapshotTx != existing {
			return errors.New("existing sqlx transaction is not a read-only repeatable-read snapshot")
		}
		return fn(ctx)
	}

	tx, err := t.db.BeginTxx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return fmt.Errorf("begin read-only snapshot: %w", err)
	}
	txCtx := context.WithValue(ctx, txContextKey{}, tx.Tx)
	txCtx = context.WithValue(txCtx, sqlxContextKey{}, tx)
	txCtx = context.WithValue(txCtx, consistentSnapshotContextKey{}, tx)
	if err := fn(txCtx); err != nil {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			return fmt.Errorf("rollback read-only snapshot after %v: %w", err, rollbackErr)
		}
		return err
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit read-only snapshot: %w", err)
	}
	return nil
}

func (t *SQLXTransactor) WithinConsistentTransaction(ctx context.Context, fn func(ctx context.Context) error) error {
	if t == nil || t.db == nil {
		return errors.New("datasource consistent transactor is not configured")
	}
	if fn == nil {
		return errors.New("consistent transaction callback must not be nil")
	}
	if existing := SQLXFromContext(ctx); existing != nil {
		consistentTx, marked := ctx.Value(consistentSnapshotContextKey{}).(*sqlx.Tx)
		if !marked || consistentTx != existing {
			return errors.New("existing sqlx transaction is not a repeatable-read consistent transaction")
		}
		return fn(ctx)
	}
	tx, err := t.db.BeginTxx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead})
	if err != nil {
		return fmt.Errorf("begin consistent transaction: %w", err)
	}
	callbacks := &afterCommitCallbacks{}
	txCtx := context.WithValue(ctx, txContextKey{}, tx.Tx)
	txCtx = context.WithValue(txCtx, sqlxContextKey{}, tx)
	txCtx = context.WithValue(txCtx, consistentSnapshotContextKey{}, tx)
	txCtx = context.WithValue(txCtx, afterCommitContextKey{}, callbacks)
	if err := fn(txCtx); err != nil {
		if rollbackErr := tx.Rollback(); rollbackErr != nil {
			callbacks.runRollback()
			return fmt.Errorf("rollback consistent transaction after %v: %w", err, rollbackErr)
		}
		callbacks.runRollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		callbacks.runRollback()
		return fmt.Errorf("commit consistent transaction: %w", err)
	}
	callbacks.run()
	return nil
}
