package outbox

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"

	dbstore "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/store"
)

const (
	defaultLeaseTTL = 10 * time.Minute

	eventColumns = `id, eventId, eventOwner, scopeId, eventType, aggregateType, aggregateId, payload, status,
		retryCount, nextRetryAt, errorMsg, leaseOwner, leaseToken, leaseUntil, createTime, updateTime`
)

var postgresOutboxIdentifiers = []string{
	"aggregateType",
	"aggregateId",
	"eventOwner",
	"scopeId",
	"eventType",
	"eventId",
	"retryCount",
	"nextRetryAt",
	"errorMsg",
	"leaseOwner",
	"leaseToken",
	"leaseUntil",
	"createTime",
	"updateTime",
	"messageId",
}

var postgresOutboxRenderer = dbstore.MustNewPostgresRenderer(postgresOutboxIdentifiers)

// ErrConsumeLeaseHeld means a broker redelivery encountered a still-live
// consumer lease. It is retryable: acknowledging that delivery would orphan
// the work if the original worker had already crashed.
var ErrConsumeLeaseHeld = errors.New("message consume lease is still held")

// Event is one transactionally persisted outbound message. EventOwner and
// ScopeID are explicit routing boundaries: a relay may only list, claim, or
// complete events owned by its own module and configured scope.
type Event struct {
	ID            int64
	EventID       string
	EventOwner    string
	ScopeID       string
	EventType     string
	AggregateType string
	AggregateID   string
	Payload       string
	// PayloadOversized is set only by the bounded listing APIs. They replace an
	// over-limit body with an empty string before it crosses the SQL boundary,
	// allowing a protocol owner to mark the row terminally invalid without
	// decoding, logging, or retaining its original content.
	PayloadOversized bool
	Status           string
	RetryCount       int
	NextRetryAt      time.Time
	LastError        string
	LeaseOwner       string
	LeaseToken       string
	LeaseUntil       time.Time
	CreateTime       time.Time
	UpdateTime       time.Time
}

// Lease is the fencing capability issued to the worker that claimed an event.
// A worker must present the token when it transitions the event out of
// PROCESSING, so a stale worker cannot overwrite a newer worker's result.
type Lease struct {
	Token string
	Until time.Time
}

type Store struct {
	db       *sqlx.DB
	leaseTTL time.Duration
	now      func() time.Time
}

func NewStore(db *sqlx.DB) *Store {
	return &Store{db: db, leaseTTL: defaultLeaseTTL, now: time.Now}
}

func (s *Store) Append(ctx context.Context, event *Event) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("outbox store is not configured")
	}
	if event == nil {
		return fmt.Errorf("outbox event must not be nil")
	}
	if strings.TrimSpace(event.EventOwner) == "" {
		return fmt.Errorf("outbox event owner must not be empty")
	}
	if strings.TrimSpace(event.EventType) == "" {
		return fmt.Errorf("outbox event type must not be empty")
	}
	if event.ID <= 0 {
		return fmt.Errorf("outbox event id must be supplied by a distributed id generator")
	}
	if event.Status == "" {
		event.Status = "PENDING"
	}
	now := s.clock()
	exec := dbstore.SQLXExecutor(ctx, s.db)
	_, err := exec.ExecContext(ctx, exec.Rebind(s.sql(`
INSERT INTO sys_outbox_event (id, eventId, eventOwner, scopeId, eventType, aggregateType, aggregateId, payload, status, retryCount, nextRetryAt, errorMsg, leaseOwner, leaseToken, leaseUntil, createTime, updateTime)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)),
		event.ID, event.EventID, strings.TrimSpace(event.EventOwner), nullString(event.ScopeID), strings.TrimSpace(event.EventType), event.AggregateType, event.AggregateID,
		event.Payload, event.Status, event.RetryCount, nullTime(event.NextRetryAt), nullString(event.LastError), nullString(event.LeaseOwner),
		nullString(event.LeaseToken), nullTime(event.LeaseUntil), now, now)
	return err
}

func (s *Store) AppendBatch(ctx context.Context, events []*Event) error {
	if s == nil || s.db == nil {
		return fmt.Errorf("outbox store is not configured")
	}
	if len(events) == 0 {
		return nil
	}
	if len(events) > 50 {
		return fmt.Errorf("outbox batch exceeds 50")
	}
	now := s.clock()
	clauses := make([]string, 0, len(events))
	args := make([]any, 0, len(events)*17)
	for _, event := range events {
		if event == nil || strings.TrimSpace(event.EventOwner) == "" || strings.TrimSpace(event.EventType) == "" {
			return fmt.Errorf("outbox batch contains invalid event")
		}
		if event.ID <= 0 {
			return fmt.Errorf("outbox batch event id must be supplied by a distributed id generator")
		}
		if event.Status == "" {
			event.Status = "PENDING"
		}
		clauses = append(clauses, `(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`)
		args = append(args,
			event.ID, event.EventID, strings.TrimSpace(event.EventOwner), nullString(event.ScopeID),
			strings.TrimSpace(event.EventType), event.AggregateType, event.AggregateID, event.Payload,
			event.Status, event.RetryCount, nullTime(event.NextRetryAt), nullString(event.LastError),
			nullString(event.LeaseOwner), nullString(event.LeaseToken), nullTime(event.LeaseUntil), now, now,
		)
	}
	exec := dbstore.SQLXExecutor(ctx, s.db)
	_, err := exec.ExecContext(ctx, exec.Rebind(s.sql(`
INSERT INTO sys_outbox_event (id, eventId, eventOwner, scopeId, eventType, aggregateType, aggregateId, payload, status, retryCount, nextRetryAt, errorMsg, leaseOwner, leaseToken, leaseUntil, createTime, updateTime)
VALUES `+strings.Join(clauses, ", "))), args...)
	if err != nil {
		return fmt.Errorf("append outbox batch: %w", err)
	}
	return nil
}

// ListReady returns only an owner's known event types. Filtering happens in SQL
// before LIMIT so one module's backlog cannot starve another module.
func (s *Store) ListReady(ctx context.Context, eventOwner string, eventTypes []string, limit int) ([]Event, error) {
	return s.listReady(ctx, eventOwner, "", eventTypes, false, limit)
}

// ListReadyForScope applies the scope predicate in SQL before LIMIT. The
// compatibility treatment for the local scope lets a local upgrade drain
// legacy unscoped events, while non-local scopes cannot claim those ambiguous
// historical rows.
func (s *Store) ListReadyForScope(ctx context.Context, eventOwner, scopeID string, eventTypes []string, limit int) ([]Event, error) {
	return s.listReady(ctx, eventOwner, scopeID, eventTypes, false, limit)
}

// ListReadyForScopePayloadBounded is the opt-in counterpart for a strict
// protocol whose payload has a reviewed maximum byte size. Existing generic
// outbox consumers retain their unmodified payload contract; this method
// never scans an oversized body into Go memory.
func (s *Store) ListReadyForScopePayloadBounded(ctx context.Context, eventOwner, scopeID string, eventTypes []string, maxPayloadBytes, limit int) ([]Event, error) {
	return s.listReadyPayloadBounded(ctx, eventOwner, scopeID, eventTypes, false, maxPayloadBytes, limit)
}

// FindReady returns one exact owner/type/event-id tuple only when it is
// claimable. It never falls back to a broader ready-queue scan.
func (s *Store) FindReady(ctx context.Context, eventOwner, eventType, eventID string) (*Event, error) {
	return s.findReady(ctx, eventOwner, "", eventType, eventID)
}

// FindReadyForScope reads one exact ready event only when it belongs to the
// supplied scope. It is used by controlled operations that may not widen into
// another scope's Outbox work.
func (s *Store) FindReadyForScope(ctx context.Context, eventOwner, scopeID, eventType, eventID string) (*Event, error) {
	return s.findReady(ctx, eventOwner, scopeID, eventType, eventID)
}

func (s *Store) findReady(ctx context.Context, eventOwner, scopeID, eventType, eventID string) (*Event, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("outbox store is not configured")
	}
	owner := strings.TrimSpace(eventOwner)
	typ := strings.TrimSpace(eventType)
	id := strings.TrimSpace(eventID)
	if owner == "" || typ == "" || id == "" {
		return nil, fmt.Errorf("outbox ready lookup requires owner, type, and event id")
	}

	now := s.clock()
	scopePredicate, scopeArgs := scopedOutboxPredicate(scopeID)
	query := `SELECT ` + eventColumns + ` FROM sys_outbox_event
WHERE eventOwner=?` + scopePredicate + ` AND eventType=? AND eventId=? AND (
  (status IN ('PENDING','FAILED') AND (nextRetryAt IS NULL OR nextRetryAt <= ?))
  OR (status='PROCESSING' AND (leaseUntil IS NULL OR leaseUntil <= ?))
)
LIMIT 1`
	exec := dbstore.SQLXExecutor(ctx, s.db)
	var row eventRow
	args := []any{owner}
	args = append(args, scopeArgs...)
	args = append(args, typ, id, now, now)
	if err := sqlx.GetContext(ctx, exec, &row, exec.Rebind(s.sql(query)), args...); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	event := row.event()
	return &event, nil
}

// ListUnknownReady returns ready events belonging to eventOwner whose event
// type is not in knownEventTypes. Callers should claim and fail these events
// closed rather than silently completing or ignoring them.
func (s *Store) ListUnknownReady(ctx context.Context, eventOwner string, knownEventTypes []string, limit int) ([]Event, error) {
	return s.listReady(ctx, eventOwner, "", knownEventTypes, true, limit)
}

// ListUnknownReadyForScope preserves the fail-closed unknown-event policy
// without allowing one scope to dead-letter another scope's records.
func (s *Store) ListUnknownReadyForScope(ctx context.Context, eventOwner, scopeID string, knownEventTypes []string, limit int) ([]Event, error) {
	return s.listReady(ctx, eventOwner, scopeID, knownEventTypes, true, limit)
}

// ListUnknownReadyForScopePayloadBounded performs the same fail-closed
// ownership scan without selecting any unknown payload. Unknown protocol
// types are terminally handled from their metadata alone, so returning a body
// would only widen the attack surface.
func (s *Store) ListUnknownReadyForScopePayloadBounded(ctx context.Context, eventOwner, scopeID string, knownEventTypes []string, maxPayloadBytes, limit int) ([]Event, error) {
	return s.listReadyPayloadBounded(ctx, eventOwner, scopeID, knownEventTypes, true, maxPayloadBytes, limit)
}

func (s *Store) listReady(ctx context.Context, eventOwner, scopeID string, eventTypes []string, inverse bool, limit int) ([]Event, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("outbox store is not configured")
	}
	owner := strings.TrimSpace(eventOwner)
	if owner == "" {
		return nil, fmt.Errorf("outbox event owner must not be empty")
	}
	types, err := normalizeEventTypes(eventTypes)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 100
	}

	placeholders := strings.TrimRight(strings.Repeat("?,", len(types)), ",")
	typePredicate := "eventType IN (" + placeholders + ")"
	if inverse {
		typePredicate = "eventType NOT IN (" + placeholders + ")"
	}
	scopePredicate, scopeArgs := scopedOutboxPredicate(scopeID)
	query := `SELECT ` + eventColumns + ` FROM sys_outbox_event
WHERE eventOwner=?` + scopePredicate + ` AND ` + typePredicate + ` AND (
  (status IN ('PENDING','FAILED') AND (nextRetryAt IS NULL OR nextRetryAt <= ?))
  OR (status='PROCESSING' AND (leaseUntil IS NULL OR leaseUntil <= ?))
)
ORDER BY createTime ASC LIMIT ?`

	now := s.clock()
	args := make([]any, 0, len(types)+len(scopeArgs)+4)
	args = append(args, owner)
	args = append(args, scopeArgs...)
	for _, eventType := range types {
		args = append(args, eventType)
	}
	args = append(args, now, now, limit)
	var rows []eventRow
	exec := dbstore.SQLXExecutor(ctx, s.db)
	if err := sqlx.SelectContext(ctx, exec, &rows, exec.Rebind(s.sql(query)), args...); err != nil {
		return nil, err
	}
	result := make([]Event, 0, len(rows))
	for _, row := range rows {
		result = append(result, row.event())
	}
	return result, nil
}

// listReadyPayloadBounded keeps the regular list/claim ordering but projects
// a bounded payload. The CASE expression is evaluated by MySQL/PostgreSQL
// before the driver scans the value, so an oversized hostile row is not copied
// into an Event. inverse rows intentionally project no body at all: their
// type is already unsupported and dead-lettering must not inspect content.
func (s *Store) listReadyPayloadBounded(ctx context.Context, eventOwner, scopeID string, eventTypes []string, inverse bool, maxPayloadBytes, limit int) ([]Event, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("outbox store is not configured")
	}
	owner := strings.TrimSpace(eventOwner)
	if owner == "" {
		return nil, fmt.Errorf("outbox event owner must not be empty")
	}
	if maxPayloadBytes <= 0 {
		return nil, fmt.Errorf("outbox payload bound must be positive")
	}
	types, err := normalizeEventTypes(eventTypes)
	if err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = 100
	}

	placeholders := strings.TrimRight(strings.Repeat("?,", len(types)), ",")
	typePredicate := "eventType IN (" + placeholders + ")"
	if inverse {
		typePredicate = "eventType NOT IN (" + placeholders + ")"
	}
	scopePredicate, scopeArgs := scopedOutboxPredicate(scopeID)
	columns := boundedEventColumns(inverse)
	query := `SELECT ` + columns + ` FROM sys_outbox_event
WHERE eventOwner=?` + scopePredicate + ` AND ` + typePredicate + ` AND (
  (status IN ('PENDING','FAILED') AND (nextRetryAt IS NULL OR nextRetryAt <= ?))
  OR (status='PROCESSING' AND (leaseUntil IS NULL OR leaseUntil <= ?))
)
ORDER BY createTime ASC LIMIT ?`

	now := s.clock()
	args := make([]any, 0, len(types)+len(scopeArgs)+6)
	if !inverse {
		// The two select-list predicates occur before the WHERE placeholders.
		args = append(args, maxPayloadBytes, maxPayloadBytes)
	}
	args = append(args, owner)
	args = append(args, scopeArgs...)
	for _, eventType := range types {
		args = append(args, eventType)
	}
	args = append(args, now, now, limit)
	var rows []eventRow
	exec := dbstore.SQLXExecutor(ctx, s.db)
	if err := sqlx.SelectContext(ctx, exec, &rows, exec.Rebind(s.sql(query)), args...); err != nil {
		return nil, err
	}
	result := make([]Event, 0, len(rows))
	for _, row := range rows {
		result = append(result, row.event())
	}
	return result, nil
}

func boundedEventColumns(inverse bool) string {
	const beforePayload = `id, eventId, eventOwner, scopeId, eventType, aggregateType, aggregateId`
	const afterPayload = `status, retryCount, nextRetryAt, errorMsg, leaseOwner, leaseToken, leaseUntil, createTime, updateTime`
	if inverse {
		return beforePayload + `, '' AS payload, 0 AS payload_oversized, ` + afterPayload
	}
	return beforePayload + `,
CASE WHEN OCTET_LENGTH(COALESCE(payload, '')) > ? THEN '' ELSE COALESCE(payload, '') END AS payload,
CASE WHEN OCTET_LENGTH(COALESCE(payload, '')) > ? THEN 1 ELSE 0 END AS payload_oversized, ` + afterPayload
}

func scopedOutboxPredicate(scopeID string) (string, []any) {
	scopeID = strings.TrimSpace(scopeID)
	if scopeID == "" {
		return "", nil
	}
	if scopeID == "local" {
		return " AND (scopeId=? OR scopeId IS NULL)", []any{scopeID}
	}
	return " AND scopeId=?", []any{scopeID}
}

// Claim atomically claims one owner/type-scoped event and issues a fresh
// fencing token. A lease-expired PROCESSING row is eligible for reclaim.
func (s *Store) Claim(ctx context.Context, eventOwner, eventType string, id int64, worker string) (*Lease, bool, error) {
	return s.claim(ctx, eventOwner, "", eventType, id, worker, false)
}

// ClaimForScope is the strict-scope variant of Claim. Unlike the legacy local
// listing compatibility path, it never treats NULL as local: a caller that
// names a scope can only claim a row bearing that exact non-empty scope.
func (s *Store) ClaimForScope(ctx context.Context, eventOwner, scopeID, eventType string, id int64, worker string) (*Lease, bool, error) {
	return s.claim(ctx, eventOwner, scopeID, eventType, id, worker, true)
}

func (s *Store) claim(ctx context.Context, eventOwner, scopeID, eventType string, id int64, worker string, exactScope bool) (*Lease, bool, error) {
	if s == nil || s.db == nil {
		return nil, false, fmt.Errorf("outbox store is not configured")
	}
	owner := strings.TrimSpace(eventOwner)
	scope := strings.TrimSpace(scopeID)
	typ := strings.TrimSpace(eventType)
	worker = strings.TrimSpace(worker)
	if owner == "" || typ == "" || worker == "" || id <= 0 || (exactScope && scope == "") {
		return nil, false, fmt.Errorf("outbox claim requires owner, type, id, and worker")
	}
	now := s.clock()
	lease := &Lease{Token: uuid.NewString(), Until: now.Add(s.leaseDuration())}
	scopePredicate := ""
	scopeArgs := []any(nil)
	if exactScope {
		scopePredicate = " AND scopeId=?"
		scopeArgs = []any{scope}
	}
	exec := dbstore.SQLXExecutor(ctx, s.db)
	result, err := exec.ExecContext(ctx, exec.Rebind(s.sql(`UPDATE sys_outbox_event
SET status='PROCESSING', leaseOwner=?, leaseToken=?, leaseUntil=?, updateTime=?
WHERE id=? AND eventOwner=?`+scopePredicate+` AND eventType=? AND (
  (status IN ('PENDING','FAILED') AND (nextRetryAt IS NULL OR nextRetryAt <= ?))
  OR (status='PROCESSING' AND (leaseUntil IS NULL OR leaseUntil <= ?))
)`)), append([]any{worker, lease.Token, lease.Until, now, id, owner}, append(scopeArgs, typ, now, now)...)...)
	if err != nil {
		return nil, false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return nil, false, err
	}
	if rows == 0 {
		return nil, false, nil
	}
	return lease, true, nil
}

// Mark transitions a PROCESSING event only when the caller still owns its
// fencing token. false means the lease was superseded or the event is no
// longer in PROCESSING; callers must not retry the stale state write.
func (s *Store) Mark(ctx context.Context, eventOwner, eventType string, id int64, leaseToken, status, lastError string, retryCount int, nextRetryAt *time.Time) (bool, error) {
	return s.mark(ctx, eventOwner, "", eventType, id, leaseToken, status, lastError, retryCount, nextRetryAt, false)
}

// MarkForScope retains the claim's exact scope predicate at completion time.
// This closes the list-to-claim-to-mark race against a row whose scope changed
// after a worker read it, and prevents DG5 from ever completing local/NULL
// compatibility rows.
func (s *Store) MarkForScope(ctx context.Context, eventOwner, scopeID, eventType string, id int64, leaseToken, status, lastError string, retryCount int, nextRetryAt *time.Time) (bool, error) {
	return s.mark(ctx, eventOwner, scopeID, eventType, id, leaseToken, status, lastError, retryCount, nextRetryAt, true)
}

func (s *Store) mark(ctx context.Context, eventOwner, scopeID, eventType string, id int64, leaseToken, status, lastError string, retryCount int, nextRetryAt *time.Time, exactScope bool) (bool, error) {
	if s == nil || s.db == nil {
		return false, fmt.Errorf("outbox store is not configured")
	}
	owner := strings.TrimSpace(eventOwner)
	scope := strings.TrimSpace(scopeID)
	typ := strings.TrimSpace(eventType)
	leaseToken = strings.TrimSpace(leaseToken)
	if owner == "" || typ == "" || leaseToken == "" || id <= 0 || (exactScope && scope == "") {
		return false, fmt.Errorf("outbox mark requires owner, type, id, and lease token")
	}
	scopePredicate := ""
	scopeArgs := []any(nil)
	if exactScope {
		scopePredicate = " AND scopeId=?"
		scopeArgs = []any{scope}
	}
	exec := dbstore.SQLXExecutor(ctx, s.db)
	result, err := exec.ExecContext(ctx, exec.Rebind(s.sql(`UPDATE sys_outbox_event
SET status=?, errorMsg=?, retryCount=?, nextRetryAt=?, leaseOwner=NULL, leaseToken=NULL, leaseUntil=NULL, updateTime=?
WHERE id=? AND eventOwner=?`+scopePredicate+` AND eventType=? AND status='PROCESSING' AND leaseToken=?`)),
		append([]any{status, nullString(lastError), retryCount, nextRetryAt, s.clock(), id, owner}, append(scopeArgs, typ, leaseToken)...)...)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}

type eventRow struct {
	ID               int64          `db:"id"`
	EventID          string         `db:"eventId"`
	EventOwner       string         `db:"eventOwner"`
	ScopeID          sql.NullString `db:"scopeId"`
	EventType        string         `db:"eventType"`
	AggregateType    string         `db:"aggregateType"`
	AggregateID      string         `db:"aggregateId"`
	Payload          string         `db:"payload"`
	PayloadOversized int            `db:"payload_oversized"`
	Status           string         `db:"status"`
	RetryCount       int            `db:"retryCount"`
	NextRetryAt      sql.NullTime   `db:"nextRetryAt"`
	ErrorMsg         sql.NullString `db:"errorMsg"`
	LeaseOwner       sql.NullString `db:"leaseOwner"`
	LeaseToken       sql.NullString `db:"leaseToken"`
	LeaseUntil       sql.NullTime   `db:"leaseUntil"`
	CreateTime       sql.NullTime   `db:"createTime"`
	UpdateTime       sql.NullTime   `db:"updateTime"`
}

func (r eventRow) event() Event {
	return Event{
		ID:               r.ID,
		EventID:          r.EventID,
		EventOwner:       r.EventOwner,
		ScopeID:          r.ScopeID.String,
		EventType:        r.EventType,
		AggregateType:    r.AggregateType,
		AggregateID:      r.AggregateID,
		Payload:          r.Payload,
		PayloadOversized: r.PayloadOversized != 0,
		Status:           r.Status,
		RetryCount:       r.RetryCount,
		NextRetryAt:      r.NextRetryAt.Time,
		LastError:        r.ErrorMsg.String,
		LeaseOwner:       r.LeaseOwner.String,
		LeaseToken:       r.LeaseToken.String,
		LeaseUntil:       r.LeaseUntil.Time,
		CreateTime:       r.CreateTime.Time,
		UpdateTime:       r.UpdateTime.Time,
	}
}

// ConsumeLease fences one consumer invocation. It uses the same lease model
// as the outbox so failed or abandoned messages can be retried safely.
type ConsumeLease struct {
	Token string
	Until time.Time
}

// ConsumeLeaseHeldError requests a bounded broker requeue while another
// worker still owns the consume lease. It preserves the message until the
// lease can be reclaimed instead of acknowledging a potentially orphaned
// redelivery.
type ConsumeLeaseHeldError struct {
	Until time.Time
}

func (e ConsumeLeaseHeldError) Error() string {
	if e.Until.IsZero() {
		return ErrConsumeLeaseHeld.Error()
	}
	return fmt.Sprintf("%s until %s", ErrConsumeLeaseHeld, e.Until.UTC().Format(time.RFC3339Nano))
}

func (e ConsumeLeaseHeldError) Unwrap() error { return ErrConsumeLeaseHeld }

// Requeue tells the generic RabbitMQ consumer that this error must not be
// acknowledged or dead-lettered as a terminal processing failure.
func (e ConsumeLeaseHeldError) Requeue() bool { return true }

// RetryAfter keeps the consumer bounded under a live lease instead of hot
// looping an immediate RabbitMQ requeue.
func (e ConsumeLeaseHeldError) RetryAfter() time.Duration { return time.Second }

type ConsumeGuard struct {
	db       *sqlx.DB
	leaseTTL time.Duration
	now      func() time.Time
}

func NewConsumeGuard(db *sqlx.DB) *ConsumeGuard {
	return &ConsumeGuard{db: db, leaseTTL: defaultLeaseTTL, now: time.Now}
}

// Begin creates or safely reclaims a consume lease. DONE is idempotent and is
// never reclaimed; FAILED and expired PROCESSING rows are eligible for retry.
func (g *ConsumeGuard) Begin(ctx context.Context, messageID, consumer, worker, detail string) (*ConsumeLease, bool, error) {
	if g == nil || g.db == nil {
		return nil, false, fmt.Errorf("consume guard is not configured")
	}
	messageID = strings.TrimSpace(messageID)
	consumer = strings.TrimSpace(consumer)
	worker = strings.TrimSpace(worker)
	if messageID == "" || consumer == "" || worker == "" {
		return nil, false, fmt.Errorf("message id, consumer, and worker must not be empty")
	}
	now := g.clock()
	lease := &ConsumeLease{Token: uuid.NewString(), Until: now.Add(g.leaseDuration())}
	exec := dbstore.SQLXExecutor(ctx, g.db)
	_, err := exec.ExecContext(ctx, exec.Rebind(g.sql(`INSERT INTO sys_message_consume_log
(id, messageId, consumer, status, detail, leaseOwner, leaseToken, leaseUntil, createTime, updateTime)
VALUES (?, ?, ?, 'PROCESSING', ?, ?, ?, ?, ?, ?)`)),
		now.UnixNano(), messageID, consumer, nullString(detail), worker, lease.Token, lease.Until, now, now)
	if err == nil {
		return lease, true, nil
	}
	if !isDuplicate(err) {
		return nil, false, err
	}

	result, updateErr := exec.ExecContext(ctx, exec.Rebind(g.sql(`UPDATE sys_message_consume_log
SET status='PROCESSING', detail=?, leaseOwner=?, leaseToken=?, leaseUntil=?, updateTime=?
WHERE messageId=? AND consumer=? AND (
  status='FAILED' OR (status='PROCESSING' AND (leaseUntil IS NULL OR leaseUntil <= ?))
)`)), nullString(detail), worker, lease.Token, lease.Until, now, messageID, consumer, now)
	if updateErr != nil {
		return nil, false, updateErr
	}
	rows, updateErr := result.RowsAffected()
	if updateErr != nil {
		return nil, false, updateErr
	}
	if rows == 0 {
		state, stateErr := g.consumeState(ctx, messageID, consumer)
		if stateErr != nil {
			return nil, false, stateErr
		}
		if state.Status == "DONE" {
			return nil, false, nil
		}
		return nil, false, ConsumeLeaseHeldError{Until: state.LeaseUntil.Time}
	}
	return lease, true, nil
}

func (g *ConsumeGuard) Finish(ctx context.Context, messageID, consumer, leaseToken, detail string) (bool, error) {
	return g.complete(ctx, messageID, consumer, leaseToken, "DONE", detail)
}

func (g *ConsumeGuard) Fail(ctx context.Context, messageID, consumer, leaseToken, detail string) (bool, error) {
	return g.complete(ctx, messageID, consumer, leaseToken, "FAILED", detail)
}

func (g *ConsumeGuard) complete(ctx context.Context, messageID, consumer, leaseToken, status, detail string) (bool, error) {
	if g == nil || g.db == nil {
		return false, fmt.Errorf("consume guard is not configured")
	}
	messageID = strings.TrimSpace(messageID)
	consumer = strings.TrimSpace(consumer)
	leaseToken = strings.TrimSpace(leaseToken)
	if messageID == "" || consumer == "" || leaseToken == "" {
		return false, fmt.Errorf("message id, consumer, and lease token must not be empty")
	}
	exec := dbstore.SQLXExecutor(ctx, g.db)
	result, err := exec.ExecContext(ctx, exec.Rebind(g.sql(`UPDATE sys_message_consume_log
SET status=?, detail=?, leaseOwner=NULL, leaseToken=NULL, leaseUntil=NULL, updateTime=?
WHERE messageId=? AND consumer=? AND status='PROCESSING' AND leaseToken=?`)),
		status, nullString(detail), g.clock(), messageID, consumer, leaseToken)
	if err != nil {
		return false, err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return rows > 0, nil
}

func (g *ConsumeGuard) ConsumeOnce(ctx context.Context, messageID, consumer, worker, detail string, fn func(context.Context) error) (bool, error) {
	lease, claimed, err := g.Begin(ctx, messageID, consumer, worker, detail)
	if err != nil || !claimed {
		return claimed, err
	}
	if fn == nil {
		_, err := g.Finish(ctx, messageID, consumer, lease.Token, detail)
		return true, err
	}
	if err := fn(ctx); err != nil {
		_, _ = g.Fail(ctx, messageID, consumer, lease.Token, err.Error())
		return true, err
	}
	_, err = g.Finish(ctx, messageID, consumer, lease.Token, detail)
	return true, err
}

func (s *Store) clock() time.Time {
	if s != nil && s.now != nil {
		return s.now().UTC()
	}
	return time.Now().UTC()
}

func (s *Store) leaseDuration() time.Duration {
	if s != nil && s.leaseTTL > 0 {
		return s.leaseTTL
	}
	return defaultLeaseTTL
}

func (s *Store) sql(query string) string {
	if s == nil {
		return query
	}
	return postgresOutboxRenderer.Render(s.db, query)
}

func (g *ConsumeGuard) clock() time.Time {
	if g != nil && g.now != nil {
		return g.now().UTC()
	}
	return time.Now().UTC()
}

func (g *ConsumeGuard) leaseDuration() time.Duration {
	if g != nil && g.leaseTTL > 0 {
		return g.leaseTTL
	}
	return defaultLeaseTTL
}

func (g *ConsumeGuard) sql(query string) string {
	if g == nil {
		return query
	}
	return postgresOutboxRenderer.Render(g.db, query)
}

type consumeState struct {
	Status     string       `db:"status"`
	LeaseUntil sql.NullTime `db:"leaseUntil"`
}

func (g *ConsumeGuard) consumeState(ctx context.Context, messageID, consumer string) (consumeState, error) {
	var state consumeState
	exec := dbstore.SQLXExecutor(ctx, g.db)
	err := sqlx.GetContext(ctx, exec, &state, exec.Rebind(g.sql(`SELECT status, leaseUntil
FROM sys_message_consume_log WHERE messageId=? AND consumer=? LIMIT 1`)), messageID, consumer)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return consumeState{}, fmt.Errorf("consume lease disappeared while resolving duplicate message %q for consumer %q", messageID, consumer)
		}
		return consumeState{}, err
	}
	return state, nil
}

func normalizeEventTypes(eventTypes []string) ([]string, error) {
	result := make([]string, 0, len(eventTypes))
	seen := make(map[string]struct{}, len(eventTypes))
	for _, eventType := range eventTypes {
		normalized := strings.TrimSpace(eventType)
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		result = append(result, normalized)
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("outbox event types must not be empty")
	}
	return result, nil
}

func nullString(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return value
}

func nullTime(value time.Time) any {
	if value.IsZero() {
		return nil
	}
	return value
}

func isDuplicate(err error) bool {
	text := strings.ToLower(err.Error())
	return strings.Contains(text, "duplicate") || strings.Contains(text, "unique")
}
