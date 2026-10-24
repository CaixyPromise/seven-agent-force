package infrastructure

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/binary"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	dbstore "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/store"
	msgoutbox "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/messaging/outbox"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/cachepolicy"
	"github.com/bytedance/sonic"
	"github.com/jmoiron/sqlx"
)

const cacheFreshnessFenceWait = 2 * time.Second

// OutboxAdapter is a strict adapter over the shared sys_outbox_event Store.
// It never creates a cache-specific table and requires the application
// transaction that changed the underlying config/dict data.
type OutboxAdapter struct {
	store  *msgoutbox.Store
	db     *sqlx.DB
	nextID func() int64
}

func NewOutboxAdapter(db *sqlx.DB, nextID func() int64) *OutboxAdapter {
	return &OutboxAdapter{store: msgoutbox.NewStore(db), db: db, nextID: nextID}
}

func (a *OutboxAdapter) Append(ctx context.Context, event cachepolicy.InvalidationEnvelope) error {
	if a == nil || a.store == nil || a.nextID == nil {
		return fmt.Errorf("cache governance outbox is not configured")
	}
	if dbstore.SQLXFromContext(ctx) == nil {
		return fmt.Errorf("cache invalidation registration requires an active transaction")
	}
	if err := event.Validate(); err != nil {
		return err
	}
	id := a.nextID()
	if id <= 0 {
		return fmt.Errorf("cache governance outbox id generator returned an invalid id")
	}
	payload, err := sonic.Marshal(event)
	if err != nil {
		return fmt.Errorf("encode cache invalidation event: %w", err)
	}
	return a.store.Append(ctx, &msgoutbox.Event{
		ID:            id,
		EventID:       event.EventID,
		EventOwner:    cachepolicy.CacheGovernanceOutboxOwner,
		ScopeID:       cachepolicy.StorageScopeSystemGlobal,
		EventType:     cachepolicy.CacheInvalidationEventType,
		AggregateType: cachepolicy.CacheInvalidationAggregate,
		AggregateID:   event.TargetDigest,
		Payload:       string(payload),
	})
}

// AppendTargeted persists a strict content-free DG6.2 event in the same
// database transaction as the session fact. It intentionally shares the
// generic outbox table while using a distinct allowlisted event type.
func (a *OutboxAdapter) AppendTargeted(ctx context.Context, event cachepolicy.TargetedInvalidationEnvelope) error {
	if a == nil || a.store == nil || a.nextID == nil {
		return fmt.Errorf("cache governance outbox is not configured")
	}
	if dbstore.SQLXFromContext(ctx) == nil {
		return fmt.Errorf("targeted cache invalidation registration requires an active transaction")
	}
	if err := event.Validate(); err != nil {
		return err
	}
	id := a.nextID()
	if id <= 0 {
		return fmt.Errorf("cache governance outbox id generator returned an invalid id")
	}
	payload, err := sonic.Marshal(event)
	if err != nil {
		return fmt.Errorf("encode targeted cache invalidation event: %w", err)
	}
	return a.store.Append(ctx, &msgoutbox.Event{ID: id, EventID: event.EventID, EventOwner: cachepolicy.CacheGovernanceOutboxOwner, ScopeID: cachepolicy.StorageScopeSystemGlobal, EventType: cachepolicy.TargetedCacheInvalidationEventType, AggregateType: cachepolicy.CacheInvalidationAggregate, AggregateID: event.TargetDigest, Payload: string(payload)})
}

// AppendRefresh writes the one strict DG6.3 global operation into the shared
// outbox inside the caller's real transaction. It intentionally has no cache
// class, target, key, identity, or value fields.
func (a *OutboxAdapter) AppendRefresh(ctx context.Context, event cachepolicy.CacheRefreshEnvelope) error {
	if a == nil || a.store == nil || a.nextID == nil {
		return fmt.Errorf("cache governance outbox is not configured")
	}
	if dbstore.SQLXFromContext(ctx) == nil {
		return fmt.Errorf("cache refresh registration requires an active transaction")
	}
	if err := event.Validate(); err != nil {
		return err
	}
	id := a.nextID()
	if id <= 0 {
		return fmt.Errorf("cache governance outbox id generator returned an invalid id")
	}
	payload, err := sonic.Marshal(event)
	if err != nil {
		return fmt.Errorf("encode cache refresh event: %w", err)
	}
	return a.store.Append(ctx, &msgoutbox.Event{ID: id, EventID: event.EventID, EventOwner: cachepolicy.CacheGovernanceOutboxOwner, ScopeID: cachepolicy.StorageScopeSystemGlobal, EventType: cachepolicy.CacheRefreshEventType, AggregateType: cachepolicy.CacheRefreshAggregate, AggregateID: cachepolicy.CacheRefreshAggregateID, Payload: string(payload)})
}

func (a *OutboxAdapter) ListReady(ctx context.Context, limit int) ([]cachepolicy.OutboxEvent, error) {
	if a == nil || a.store == nil {
		return nil, fmt.Errorf("cache governance outbox is not configured")
	}
	// DG5's Sonic envelope is intentionally byte-bounded before it reaches the
	// relay. The shared Store projects a safe empty payload plus a boolean for
	// an oversized row, so an owner/type-matching malicious database record can
	// be claimed and marked DEAD without being copied into process memory.
	events, err := a.store.ListReadyForScopePayloadBounded(ctx, cachepolicy.CacheGovernanceOutboxOwner, cachepolicy.StorageScopeSystemGlobal, []string{cachepolicy.CacheInvalidationEventType}, cachepolicy.MaxInvalidationEnvelopeBytes, limit)
	return mapEvents(events), err
}

func (a *OutboxAdapter) ListTargetedReady(ctx context.Context, limit int) ([]cachepolicy.OutboxEvent, error) {
	if a == nil || a.store == nil {
		return nil, fmt.Errorf("cache governance outbox is not configured")
	}
	events, err := a.store.ListReadyForScopePayloadBounded(ctx, cachepolicy.CacheGovernanceOutboxOwner, cachepolicy.StorageScopeSystemGlobal, []string{cachepolicy.TargetedCacheInvalidationEventType}, cachepolicy.MaxInvalidationEnvelopeBytes, limit)
	return mapEvents(events), err
}

func (a *OutboxAdapter) ListRefreshReady(ctx context.Context, limit int) ([]cachepolicy.OutboxEvent, error) {
	if a == nil || a.store == nil {
		return nil, fmt.Errorf("cache governance outbox is not configured")
	}
	events, err := a.store.ListReadyForScopePayloadBounded(ctx, cachepolicy.CacheGovernanceOutboxOwner, cachepolicy.StorageScopeSystemGlobal, []string{cachepolicy.CacheRefreshEventType}, cachepolicy.MaxInvalidationEnvelopeBytes, limit)
	return mapEvents(events), err
}

func (a *OutboxAdapter) ListUnknown(ctx context.Context, limit int) ([]cachepolicy.OutboxEvent, error) {
	if a == nil || a.store == nil {
		return nil, fmt.Errorf("cache governance outbox is not configured")
	}
	events, err := a.store.ListUnknownReadyForScopePayloadBounded(ctx, cachepolicy.CacheGovernanceOutboxOwner, cachepolicy.StorageScopeSystemGlobal, []string{cachepolicy.CacheInvalidationEventType}, cachepolicy.MaxInvalidationEnvelopeBytes, limit)
	return mapEvents(events), err
}

func (a *OutboxAdapter) ListTargetedUnknown(ctx context.Context, limit int) ([]cachepolicy.OutboxEvent, error) {
	if a == nil || a.store == nil {
		return nil, fmt.Errorf("cache governance outbox is not configured")
	}
	events, err := a.store.ListUnknownReadyForScopePayloadBounded(ctx, cachepolicy.CacheGovernanceOutboxOwner, cachepolicy.StorageScopeSystemGlobal, []string{cachepolicy.TargetedCacheInvalidationEventType}, cachepolicy.MaxInvalidationEnvelopeBytes, limit)
	return mapEvents(events), err
}

func (a *OutboxAdapter) ListRefreshUnknown(ctx context.Context, limit int) ([]cachepolicy.OutboxEvent, error) {
	if a == nil || a.store == nil {
		return nil, fmt.Errorf("cache governance outbox is not configured")
	}
	events, err := a.store.ListUnknownReadyForScopePayloadBounded(ctx, cachepolicy.CacheGovernanceOutboxOwner, cachepolicy.StorageScopeSystemGlobal, []string{cachepolicy.CacheRefreshEventType}, cachepolicy.MaxInvalidationEnvelopeBytes, limit)
	return mapEvents(events), err
}

func (a *OutboxAdapter) Claim(ctx context.Context, id int64, eventType, worker string) (*cachepolicy.Lease, bool, error) {
	if a == nil || a.store == nil {
		return nil, false, fmt.Errorf("cache governance outbox is not configured")
	}
	lease, claimed, err := a.store.ClaimForScope(ctx, cachepolicy.CacheGovernanceOutboxOwner, cachepolicy.StorageScopeSystemGlobal, strings.TrimSpace(eventType), id, strings.TrimSpace(worker))
	if err != nil || !claimed {
		return nil, claimed, err
	}
	return &cachepolicy.Lease{Token: lease.Token, Until: lease.Until}, true, nil
}

func (a *OutboxAdapter) Mark(ctx context.Context, id int64, eventType, leaseToken, status, reason string, retryCount int, nextRetryAt *time.Time) (bool, error) {
	if a == nil || a.store == nil {
		return false, fmt.Errorf("cache governance outbox is not configured")
	}
	return a.store.MarkForScope(ctx, cachepolicy.CacheGovernanceOutboxOwner, cachepolicy.StorageScopeSystemGlobal, strings.TrimSpace(eventType), id, strings.TrimSpace(leaseToken), strings.TrimSpace(status), strings.TrimSpace(reason), retryCount, nextRetryAt)
}

// AcquireRead holds a per-data-class advisory mutex while a classified read
// decides whether an L1/L2 candidate can be returned. A mutation holds the
// same mutex across its database commit, preventing the pre-relay stale race:
// a read after commit can only proceed after it observes the committed outbox
// row or the generation the relay advanced before marking that row DONE.
func (a *OutboxAdapter) AcquireRead(ctx context.Context, dataClass cachepolicy.DataClass) (cachepolicy.FreshnessLease, error) {
	global, err := a.acquireFenceByName(ctx, globalRefreshFenceName, globalRefreshFenceKey)
	if err != nil {
		return nil, err
	}
	lease, err := a.acquireFence(ctx, dataClass)
	if err != nil {
		global.Release()
		return nil, err
	}
	outstanding, err := a.hasOutstandingInvalidation(ctx, lease.conn, dataClass)
	if err != nil {
		lease.Release()
		global.Release()
		return nil, err
	}
	refreshOutstanding, err := a.hasOutstandingRefresh(ctx, global.conn)
	if err != nil {
		lease.Release()
		global.Release()
		return nil, err
	}
	global.trusted = !refreshOutstanding
	lease.trusted = !outstanding && !refreshOutstanding
	return &combinedFreshnessLease{global: global, local: lease}, nil
}

// AcquireMutation serializes the whole business-data plus outbox transaction
// with classified cache reads. It is intentionally acquired before beginning
// the transaction and released only after commit or rollback completion.
func (a *OutboxAdapter) AcquireMutation(ctx context.Context, dataClass cachepolicy.DataClass) (cachepolicy.FreshnessLease, error) {
	return a.acquireFence(ctx, dataClass)
}

func (a *OutboxAdapter) AcquireTargetedRead(ctx context.Context, dataClass cachepolicy.DataClass, targetKind, targetDigest string) (cachepolicy.FreshnessLease, error) {
	if dataClass != cachepolicy.DataClassActiveSessionValidity || strings.TrimSpace(targetKind) != "active-session" || !cachepolicy.IsDigest(targetDigest) {
		return nil, cachepolicy.ErrInvalidationEnvelope
	}
	global, err := a.acquireFenceByName(ctx, globalRefreshFenceName, globalRefreshFenceKey)
	if err != nil {
		return nil, err
	}
	lease, err := a.acquireTargetedFence(ctx, targetDigest)
	if err != nil {
		global.Release()
		return nil, err
	}
	outstanding, err := a.hasOutstandingTargetedInvalidation(ctx, lease.conn, targetDigest)
	if err != nil {
		lease.Release()
		global.Release()
		return nil, err
	}
	refreshOutstanding, err := a.hasOutstandingRefresh(ctx, global.conn)
	if err != nil {
		lease.Release()
		global.Release()
		return nil, err
	}
	global.trusted = !refreshOutstanding
	lease.trusted = !outstanding && !refreshOutstanding
	return &combinedFreshnessLease{global: global, local: lease}, nil
}

// AcquireRefreshMutation is the one global advisory fence used to serialize
// refresh creation/coalescing. It is not exposed to normal cache managers and
// is released only after the application transaction completes.
func (a *OutboxAdapter) AcquireRefreshMutation(ctx context.Context) (cachepolicy.FreshnessLease, error) {
	return a.acquireFenceByName(ctx, globalRefreshFenceName, globalRefreshFenceKey)
}

func (a *OutboxAdapter) FindActiveRefresh(ctx context.Context) (*cachepolicy.RefreshOperation, error) {
	return a.findRefresh(ctx, true)
}

func (a *OutboxAdapter) FindLatestCompletedRefresh(ctx context.Context) (*cachepolicy.RefreshOperation, error) {
	return a.findRefresh(ctx, false)
}

func (a *OutboxAdapter) findRefresh(ctx context.Context, active bool) (*cachepolicy.RefreshOperation, error) {
	if a == nil || a.db == nil {
		return nil, fmt.Errorf("cache governance outbox is not configured")
	}
	query := refreshOperationQuery(dbstore.IsPostgres(a.db), active)
	exec := dbstore.SQLXExecutor(ctx, a.db)
	var item struct {
		EventID     string       `db:"eventId"`
		CompletedAt sql.NullTime `db:"updateTime"`
	}
	err := sqlx.GetContext(ctx, exec, &item, exec.Rebind(query), cachepolicy.CacheGovernanceOutboxOwner, cachepolicy.StorageScopeSystemGlobal, cachepolicy.CacheRefreshEventType, cachepolicy.CacheRefreshAggregate, cachepolicy.CacheRefreshAggregateID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("find cache refresh operation: %w", err)
	}
	return &cachepolicy.RefreshOperation{EventID: strings.TrimSpace(item.EventID), CompletedAt: item.CompletedAt.Time.UTC()}, nil
}

func (a *OutboxAdapter) AcquireTargetedMutation(ctx context.Context, dataClass cachepolicy.DataClass, targetKind, targetDigest string) (cachepolicy.FreshnessLease, error) {
	if dataClass != cachepolicy.DataClassActiveSessionValidity || strings.TrimSpace(targetKind) != "active-session" || !cachepolicy.IsDigest(targetDigest) {
		return nil, cachepolicy.ErrInvalidationEnvelope
	}
	return a.acquireTargetedFence(ctx, targetDigest)
}

// BeginTargetedMutationFence creates a lazy transaction-scoped collector. A
// bulk revoke may enumerate many pages of sessions; using a dedicated
// connection for every target exhausts PostgreSQL long before the transaction
// reaches its outbox commit. This collector holds all exact advisory locks on
// one connection until the caller's commit/rollback callback releases it.
func (a *OutboxAdapter) BeginTargetedMutationFence(_ context.Context) (cachepolicy.TargetedMutationFence, error) {
	if a == nil || a.db == nil {
		return nil, fmt.Errorf("cache freshness fence is not configured")
	}
	return &outboxTargetedMutationFence{adapter: a, postgres: dbstore.IsPostgres(a.db), held: make(map[string]struct{})}, nil
}

type targetedFenceLock struct {
	name string
	key  int64
}

// outboxTargetedMutationFence is deliberately not a FreshnessLease: mutation
// callers do not inspect Trusted, and this collection can grow in target count
// without growing its database connection count.
type outboxTargetedMutationFence struct {
	adapter  *OutboxAdapter
	postgres bool

	mu        sync.Mutex
	conn      *sql.Conn
	held      map[string]struct{}
	locks     []targetedFenceLock
	batchHeld bool
	released  bool
}

func (b *outboxTargetedMutationFence) AcquireTargetedMutation(ctx context.Context, dataClass cachepolicy.DataClass, targetKind, targetDigest string) error {
	if b == nil || b.adapter == nil || b.adapter.db == nil || dataClass != cachepolicy.DataClassActiveSessionValidity || strings.TrimSpace(targetKind) != "active-session" || !cachepolicy.IsDigest(targetDigest) {
		return cachepolicy.ErrInvalidationEnvelope
	}
	name := "seven:dg6:session-v2:freshness:" + targetDigest[:32]
	key := cacheTargetFenceKey(targetDigest)
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.released {
		return fmt.Errorf("targeted cache mutation fence is already released")
	}
	if _, alreadyHeld := b.held[targetDigest]; alreadyHeld {
		return nil
	}
	lockCtx, cancel := context.WithTimeout(ctx, cacheFreshnessFenceWait)
	defer cancel()
	if b.conn == nil {
		conn, err := b.adapter.db.Conn(lockCtx)
		if err != nil {
			return fmt.Errorf("acquire cache freshness fence connection: %w", err)
		}
		b.conn = conn
	}
	// Every streaming bulk mutation first obtains this one batch-writer lock.
	// It establishes a global order before any digest-specific lock, avoiding a
	// wait cycle when two overlapping revocations enumerate the same sessions
	// in different page/digest orders. Single-target revocations use the legacy
	// exact-target lease and never pay this batch serialization cost.
	if !b.batchHeld {
		if err := b.lock(lockCtx, targetedBatchFenceName, targetedBatchFenceKey); err != nil {
			return err
		}
		b.locks = append(b.locks, targetedFenceLock{name: targetedBatchFenceName, key: targetedBatchFenceKey})
		b.batchHeld = true
	}
	if err := b.lock(lockCtx, name, key); err != nil {
		return err
	}
	b.held[targetDigest] = struct{}{}
	b.locks = append(b.locks, targetedFenceLock{name: name, key: key})
	return nil
}

func (b *outboxTargetedMutationFence) lock(ctx context.Context, name string, key int64) error {
	if b == nil || b.conn == nil {
		return fmt.Errorf("cache freshness fence is not configured")
	}
	if b.postgres {
		if _, err := b.conn.ExecContext(ctx, `SELECT pg_advisory_lock($1)`, key); err != nil {
			return fmt.Errorf("acquire cache freshness fence: %w", err)
		}
		return nil
	}
	var acquired sql.NullInt64
	if err := b.conn.QueryRowContext(ctx, `SELECT GET_LOCK(?, ?)`, name, int(cacheFreshnessFenceWait/time.Second)).Scan(&acquired); err != nil {
		return fmt.Errorf("acquire cache freshness fence: %w", err)
	}
	if !acquired.Valid || acquired.Int64 != 1 {
		return fmt.Errorf("cache freshness fence unavailable")
	}
	return nil
}

func (b *outboxTargetedMutationFence) Release() {
	if b == nil {
		return
	}
	b.mu.Lock()
	if b.released {
		b.mu.Unlock()
		return
	}
	b.released = true
	conn := b.conn
	postgres := b.postgres
	locks := append([]targetedFenceLock(nil), b.locks...)
	b.mu.Unlock()
	if conn == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), cacheFreshnessFenceWait)
	defer cancel()
	for index := len(locks) - 1; index >= 0; index-- {
		if postgres {
			_, _ = conn.ExecContext(ctx, `SELECT pg_advisory_unlock($1)`, locks[index].key)
		} else {
			var released sql.NullInt64
			_ = conn.QueryRowContext(ctx, `SELECT RELEASE_LOCK(?)`, locks[index].name).Scan(&released)
		}
	}
	_ = conn.Close()
}

func (a *OutboxAdapter) acquireTargetedFence(ctx context.Context, targetDigest string) (*outboxFreshnessLease, error) {
	return a.acquireFenceByName(ctx, "seven:dg6:session-v2:freshness:"+targetDigest[:32], cacheTargetFenceKey(targetDigest))
}

func (a *OutboxAdapter) acquireFence(ctx context.Context, dataClass cachepolicy.DataClass) (*outboxFreshnessLease, error) {
	if a == nil || a.db == nil {
		return nil, fmt.Errorf("cache freshness fence is not configured")
	}
	if _, ok := cachepolicy.Entry(dataClass); !ok {
		return nil, cachepolicy.ErrInvalidationEnvelope
	}
	return a.acquireFenceByName(ctx, cacheFenceName(dataClass), cacheFenceKey(dataClass))
}

func (a *OutboxAdapter) acquireFenceByName(ctx context.Context, name string, key int64) (*outboxFreshnessLease, error) {
	if a == nil || a.db == nil {
		return nil, fmt.Errorf("cache freshness fence is not configured")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	lockCtx, cancel := context.WithTimeout(ctx, cacheFreshnessFenceWait)
	defer cancel()
	conn, err := a.db.Conn(lockCtx)
	if err != nil {
		return nil, fmt.Errorf("acquire cache freshness fence connection: %w", err)
	}
	lease := &outboxFreshnessLease{
		conn:     conn,
		postgres: dbstore.IsPostgres(a.db),
		name:     name,
		key:      key,
	}
	if err := lease.lock(lockCtx); err != nil {
		_ = conn.Close()
		return nil, err
	}
	return lease, nil
}

func (a *OutboxAdapter) hasOutstandingTargetedInvalidation(ctx context.Context, conn *sql.Conn, targetDigest string) (bool, error) {
	query := outstandingInvalidationQuery(dbstore.IsPostgres(a.db))
	var outstanding bool
	err := conn.QueryRowContext(ctx, query, cachepolicy.CacheGovernanceOutboxOwner, cachepolicy.StorageScopeSystemGlobal, cachepolicy.TargetedCacheInvalidationEventType, cachepolicy.CacheInvalidationAggregate, targetDigest).Scan(&outstanding)
	if err != nil {
		return false, fmt.Errorf("check targeted cache freshness state: %w", err)
	}
	return outstanding, nil
}

func (a *OutboxAdapter) hasOutstandingInvalidation(ctx context.Context, conn *sql.Conn, dataClass cachepolicy.DataClass) (bool, error) {
	if a == nil || a.db == nil || conn == nil {
		return false, fmt.Errorf("cache freshness fence is not configured")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	query := outstandingInvalidationQuery(dbstore.IsPostgres(a.db))
	var outstanding bool
	if err := conn.QueryRowContext(ctx, query,
		cachepolicy.CacheGovernanceOutboxOwner,
		cachepolicy.StorageScopeSystemGlobal,
		cachepolicy.CacheInvalidationEventType,
		cachepolicy.CacheInvalidationAggregate,
		cachepolicy.ClassTargetDigest(dataClass),
	).Scan(&outstanding); err != nil {
		return false, fmt.Errorf("check cache freshness state: %w", err)
	}
	return outstanding, nil
}

func (a *OutboxAdapter) hasOutstandingRefresh(ctx context.Context, conn *sql.Conn) (bool, error) {
	if a == nil || a.db == nil || conn == nil {
		return false, fmt.Errorf("cache freshness fence is not configured")
	}
	query := outstandingRefreshQuery(dbstore.IsPostgres(a.db))
	var outstanding bool
	if err := conn.QueryRowContext(ctx, query,
		cachepolicy.CacheGovernanceOutboxOwner,
		cachepolicy.StorageScopeSystemGlobal,
		cachepolicy.CacheRefreshEventType,
		cachepolicy.CacheRefreshAggregate,
		cachepolicy.CacheRefreshAggregateID,
	).Scan(&outstanding); err != nil {
		return false, fmt.Errorf("check global cache refresh state: %w", err)
	}
	return outstanding, nil
}

func refreshOperationQuery(postgres, active bool) string {
	columns := "eventId, updateTime"
	where := "eventOwner=? AND scopeId=? AND eventType=? AND aggregateType=? AND aggregateId=?"
	orderBy := "updateTime DESC, id DESC"
	if postgres {
		columns = `"eventId", "updateTime"`
		where = `"eventOwner"=$1 AND "scopeId"=$2 AND "eventType"=$3 AND "aggregateType"=$4 AND "aggregateId"=$5`
		orderBy = `"updateTime" DESC, id DESC`
	}
	if active {
		return `SELECT ` + columns + ` FROM sys_outbox_event WHERE ` + where + ` AND status IN ('PENDING', 'PROCESSING', 'FAILED') ORDER BY ` + orderBy + ` LIMIT 1`
	}
	return `SELECT ` + columns + ` FROM sys_outbox_event WHERE ` + where + ` AND status = 'DONE' ORDER BY ` + orderBy + ` LIMIT 1`
}

// outstandingInvalidationQuery treats every status other than exact DONE as
// untrusted, including a future/corrupt state outside the current state
// machine. The zero-stale contract must fail closed rather than exchange that
// integrity protection for a narrower index predicate. The aggregateType
// predicate still binds the query to DG5's own protocol and lets the existing
// aggregate index constrain the candidate relation.
func outstandingInvalidationQuery(postgres bool) string {
	if postgres {
		return `SELECT EXISTS(SELECT 1 FROM sys_outbox_event
WHERE "eventOwner"=$1 AND "scopeId"=$2 AND "eventType"=$3 AND "aggregateType"=$4 AND "aggregateId"=$5
AND status <> 'DONE')`
	}
	return `SELECT EXISTS(SELECT 1 FROM sys_outbox_event
WHERE eventOwner=? AND scopeId=? AND eventType=? AND aggregateType=? AND aggregateId=?
AND status <> 'DONE')`
}

// outstandingRefreshQuery is deliberately narrower than V1/V2's predicate.
// V3 marks only a strictly-decoded, content-free invalid/oversized envelope
// DEAD before it can advance the epoch or publish. Such a terminal diagnosis
// must not permanently force every governed read to source-only. Every other
// status, including an unknown future/corrupt value, remains fail-closed.
func outstandingRefreshQuery(postgres bool) string {
	if postgres {
		return `SELECT EXISTS(SELECT 1 FROM sys_outbox_event
WHERE "eventOwner"=$1 AND "scopeId"=$2 AND "eventType"=$3 AND "aggregateType"=$4 AND "aggregateId"=$5
AND status NOT IN ('DONE', 'DEAD'))`
	}
	return `SELECT EXISTS(SELECT 1 FROM sys_outbox_event
WHERE eventOwner=? AND scopeId=? AND eventType=? AND aggregateType=? AND aggregateId=?
AND status NOT IN ('DONE', 'DEAD'))`
}

type outboxFreshnessLease struct {
	conn     *sql.Conn
	postgres bool
	name     string
	key      int64
	trusted  bool
	once     sync.Once
}

// combinedFreshnessLease preserves the global-before-local acquisition order
// for every V1/V2 candidate read. Release is intentionally reverse order, so
// a V3 mutation cannot commit between the V3 state check and candidate accept.
type combinedFreshnessLease struct {
	global *outboxFreshnessLease
	local  *outboxFreshnessLease
}

func (l *combinedFreshnessLease) Trusted() bool {
	return l != nil && l.global != nil && l.local != nil && l.global.Trusted() && l.local.Trusted()
}

func (l *combinedFreshnessLease) Release() {
	if l == nil {
		return
	}
	if l.local != nil {
		l.local.Release()
	}
	if l.global != nil {
		l.global.Release()
	}
}

func (l *outboxFreshnessLease) Trusted() bool {
	return l != nil && l.trusted
}

func (l *outboxFreshnessLease) Release() {
	if l == nil {
		return
	}
	l.once.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), cacheFreshnessFenceWait)
		defer cancel()
		if l.conn != nil {
			if l.postgres {
				_, _ = l.conn.ExecContext(ctx, `SELECT pg_advisory_unlock($1)`, l.key)
			} else {
				var released sql.NullInt64
				_ = l.conn.QueryRowContext(ctx, `SELECT RELEASE_LOCK(?)`, l.name).Scan(&released)
			}
			_ = l.conn.Close()
		}
	})
}

func (l *outboxFreshnessLease) lock(ctx context.Context) error {
	if l == nil || l.conn == nil {
		return fmt.Errorf("cache freshness fence is not configured")
	}
	if l.postgres {
		if _, err := l.conn.ExecContext(ctx, `SELECT pg_advisory_lock($1)`, l.key); err != nil {
			return fmt.Errorf("acquire cache freshness fence: %w", err)
		}
		return nil
	}
	var acquired sql.NullInt64
	if err := l.conn.QueryRowContext(ctx, `SELECT GET_LOCK(?, ?)`, l.name, int(cacheFreshnessFenceWait/time.Second)).Scan(&acquired); err != nil {
		return fmt.Errorf("acquire cache freshness fence: %w", err)
	}
	if !acquired.Valid || acquired.Int64 != 1 {
		return fmt.Errorf("cache freshness fence unavailable")
	}
	return nil
}

func cacheFenceName(dataClass cachepolicy.DataClass) string {
	digest := cachepolicy.ClassTargetDigest(dataClass)
	return "seven:dg5:freshness:" + digest[:32]
}

func cacheFenceKey(dataClass cachepolicy.DataClass) int64 {
	digest := sha256.Sum256([]byte("seven:dg5:freshness:" + string(dataClass)))
	value := binary.BigEndian.Uint64(digest[:8]) & math.MaxInt64
	if value == 0 {
		value = 1
	}
	return int64(value)
}

func cacheTargetFenceKey(targetDigest string) int64 {
	digest := sha256.Sum256([]byte("seven:dg6:session-v2:freshness:" + targetDigest))
	value := binary.BigEndian.Uint64(digest[:8]) & math.MaxInt64
	if value == 0 {
		value = 1
	}
	return int64(value)
}

const globalRefreshFenceName = "seven:dg6:cache-refresh-v3"

var globalRefreshFenceKey = cacheTargetFenceKey("5db3695d454c3d4e25f3389104bbd57368de27ca2b6ad1ea16590dfa691842d4")

const targetedBatchFenceName = "seven:dg6:session-v2:bulk-mutation"

var targetedBatchFenceKey = cacheTargetFenceKey("0d65f8a9d73f9b4ab1b09988601aef36d2d8abbe0b384b2c4dd9a60cb760612f")

func mapEvents(events []msgoutbox.Event) []cachepolicy.OutboxEvent {
	result := make([]cachepolicy.OutboxEvent, 0, len(events))
	for _, event := range events {
		result = append(result, cachepolicy.OutboxEvent{
			ID:               event.ID,
			EventID:          event.EventID,
			EventOwner:       event.EventOwner,
			ScopeID:          event.ScopeID,
			EventType:        event.EventType,
			AggregateType:    event.AggregateType,
			AggregateID:      event.AggregateID,
			Payload:          event.Payload,
			PayloadOversized: event.PayloadOversized,
			RetryCount:       event.RetryCount,
		})
	}
	return result
}

var _ cachepolicy.OutboxPort = (*OutboxAdapter)(nil)
var _ cachepolicy.FreshnessGate = (*OutboxAdapter)(nil)
var _ cachepolicy.TargetedOutboxPort = (*OutboxAdapter)(nil)
var _ cachepolicy.TargetedFreshnessGate = (*OutboxAdapter)(nil)
var _ cachepolicy.TargetedMutationFenceFactory = (*OutboxAdapter)(nil)
var _ cachepolicy.RefreshOutboxPort = (*OutboxAdapter)(nil)
var _ cachepolicy.RefreshFreshnessGate = (*OutboxAdapter)(nil)
