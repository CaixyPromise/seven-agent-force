package docker

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/store"
	secretvalueinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/crypto/secretvalue"
	"github.com/jmoiron/sqlx"
)

type OperationRecord struct {
	ID                       int64          `db:"id"`
	OperationType            string         `db:"operationType"`
	TargetType               string         `db:"targetType"`
	TargetID                 sql.NullString `db:"targetId"`
	TargetName               sql.NullString `db:"targetName"`
	Status                   string         `db:"status"`
	ProgressPercent          int            `db:"progressPercent"`
	CurrentStage             sql.NullString `db:"currentStage"`
	ErrorSummary             sql.NullString `db:"errorSummary"`
	ResultJSON               sql.NullString `db:"resultJson"`
	RequestPayloadPreview    sql.NullString `db:"requestPayloadPreview"`
	RequestPayloadCiphertext sql.NullString `db:"requestPayloadCiphertext"`
	RequestPayloadEDEK       sql.NullString `db:"requestPayloadEdek"`
	RequestPayloadWrapKeyRef sql.NullString `db:"requestPayloadWrapKeyRef"`
	ActorUserID              sql.NullInt64  `db:"actorUserId"`
	ActorUsername            sql.NullString `db:"actorUsername"`
	RetryOf                  sql.NullInt64  `db:"retryOf"`
	CancelRequested          bool           `db:"cancelRequested"`
	TimeoutAt                sql.NullTime   `db:"timeoutAt"`
	StartedAt                sql.NullTime   `db:"startedAt"`
	FinishedAt               sql.NullTime   `db:"finishedAt"`
	HeartbeatAt              sql.NullTime   `db:"heartbeatAt"`
	CreateTime               sql.NullTime   `db:"createTime"`
	UpdateTime               sql.NullTime   `db:"updateTime"`
}

type OperationEventRecord struct {
	ID          int64          `db:"id"`
	OperationID int64          `db:"operationId"`
	Sequence    int64          `db:"sequence"`
	EventType   string         `db:"eventType"`
	Stage       sql.NullString `db:"stage"`
	Percent     sql.NullInt64  `db:"percent"`
	Message     sql.NullString `db:"message"`
	PayloadJSON sql.NullString `db:"payloadJson"`
	OccurredAt  time.Time      `db:"occurredAt"`
}

var dockerOperationPostgresRenderer = store.MustNewPostgresRenderer([]string{
	"operationType",
	"targetType",
	"targetId",
	"targetName",
	"progressPercent",
	"currentStage",
	"errorSummary",
	"resultJson",
	"requestPayloadPreview",
	"requestPayloadCiphertext",
	"requestPayloadEdek",
	"requestPayloadWrapKeyRef",
	"actorUserId",
	"actorUsername",
	"retryOf",
	"cancelRequested",
	"timeoutAt",
	"startedAt",
	"finishedAt",
	"heartbeatAt",
	"createTime",
	"updateTime",
	"operationId",
	"eventType",
	"payloadJson",
	"occurredAt",
	"integrityStatus",
	"diagnosticId",
	"integrityVersion",
	"diagnosedAt",
	"integrityScope",
	"integrityRelationshipType",
	"integrityReason",
	"eventId",
	"expectedIntegrityVersion",
}, "cancelRequested")

var (
	ErrOperationParentNotFound        = errors.New("docker operation parent not found")
	ErrOperationMutationConflict      = errors.New("docker operation mutation conflicts with an existing record")
	ErrOperationEventMutationConflict = errors.New("docker operation event mutation conflicts with an existing record")
	ErrOperationOrphanChanged         = errors.New("docker operation event orphan changed after diagnosis")
)

type OperationEventOrphanDiagnostic struct {
	EventID                   int64          `db:"id"`
	OperationID               int64          `db:"operationId"`
	Sequence                  int64          `db:"sequence"`
	DiagnosticID              sql.NullString `db:"diagnosticId"`
	IntegrityVersion          int64          `db:"integrityVersion"`
	IntegrityScope            sql.NullString `db:"integrityScope"`
	IntegrityRelationshipType sql.NullString `db:"integrityRelationshipType"`
	IntegrityReason           sql.NullString `db:"integrityReason"`
	OccurredAt                time.Time      `db:"occurredAt"`
}

const (
	operationEventIntegrityScope            = "docker_operation_event"
	operationEventIntegrityRelationshipType = "docker_operation_event.operationId->docker_operation.id"
	operationEventIntegrityReason           = "MISSING_PARENT"
)

type OperationEventOrphanCleanupCommand struct {
	AuditID                  int64
	DiagnosticID             string
	EventID                  int64
	OperationID              int64
	Sequence                 int64
	ExpectedIntegrityVersion int64
	ActorUserID              int64
	ActorUsername            string
	Reason                   string
}

type OperationEventOrphanCleanupResult string

const (
	OperationEventOrphanCleanupDeleted       OperationEventOrphanCleanupResult = "DELETED"
	OperationEventOrphanCleanupAlreadyDone   OperationEventOrphanCleanupResult = "ALREADY_DONE"
	OperationEventOrphanCleanupParentPresent OperationEventOrphanCleanupResult = "PARENT_PRESENT"
)

type operationEventOrphanAuditRecord struct {
	DiagnosticID             string `db:"diagnosticId"`
	EventID                  int64  `db:"eventId"`
	OperationID              int64  `db:"operationId"`
	Sequence                 int64  `db:"sequence"`
	ExpectedIntegrityVersion int64  `db:"expectedIntegrityVersion"`
	Action                   string `db:"action"`
	Result                   string `db:"result"`
	ActorUserID              int64  `db:"actorUserId"`
	ActorUsername            string `db:"actorUsername"`
	Reason                   string `db:"reason"`
}

type OperationRepository struct {
	db         store.SQLX
	transactor store.Transactor
	dialect    string
}

func NewOperationRepository(provider store.Provider) (*OperationRepository, error) {
	if provider == nil || provider.SQLX() == nil {
		return nil, fmt.Errorf("docker operation repository requires datasource provider")
	}
	if provider.Transactor() == nil || !provider.Transactor().Enabled() {
		return nil, fmt.Errorf("docker operation repository requires datasource transactor")
	}
	return &OperationRepository{
		db:         provider.SQLX(),
		transactor: provider.Transactor(),
		dialect:    strings.ToLower(strings.TrimSpace(provider.Dialect())),
	}, nil
}

func (r *OperationRepository) executor(ctx context.Context) store.SQLX {
	return store.SQLXExecutor(ctx, r.db)
}

func (r *OperationRepository) InsertOperation(ctx context.Context, row OperationRecord) error {
	return r.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := r.lockIntegrityGuard(txCtx, row.ID); err != nil {
			return err
		}
		existing, err := r.getOperationForUpdate(txCtx, row.ID)
		if err != nil {
			return err
		}
		if existing != nil {
			if operationInsertMatches(*existing, row) {
				return nil
			}
			return ErrOperationMutationConflict
		}
		exec := r.executor(txCtx)
		_, err = exec.ExecContext(txCtx, r.rebind(exec, `
INSERT INTO docker_operation (
	id, operationType, targetType, targetId, targetName, status, progressPercent, currentStage, errorSummary,
	resultJson, requestPayloadPreview, requestPayloadCiphertext, requestPayloadEdek, requestPayloadWrapKeyRef,
	actorUserId, actorUsername, retryOf, cancelRequested, timeoutAt, createTime, updateTime
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NOW(), NOW())`),
			row.ID, row.OperationType, row.TargetType, nullableString(row.TargetID.String), nullableString(row.TargetName.String), row.Status, row.ProgressPercent,
			nullableString(row.CurrentStage.String), nullableString(row.ErrorSummary.String), nullableString(row.ResultJSON.String),
			nullableString(row.RequestPayloadPreview.String), nullableString(row.RequestPayloadCiphertext.String),
			nullableString(row.RequestPayloadEDEK.String), nullableString(row.RequestPayloadWrapKeyRef.String),
			nullableInt64(row.ActorUserID.Int64), nullableString(row.ActorUsername.String), nullableInt64(row.RetryOf.Int64),
			r.booleanValue(row.CancelRequested), nullableOperationTime(row.TimeoutAt))
		if err != nil {
			if isDuplicateKeyError(err) {
				return ErrOperationMutationConflict
			}
			return fmt.Errorf("insert docker operation: %w", err)
		}
		return nil
	})
}

func (r *OperationRepository) GetOperation(ctx context.Context, id int64) (*OperationRecord, error) {
	exec := r.executor(ctx)
	var row OperationRecord
	query := r.rebind(exec, `
SELECT id, operationType, targetType, targetId, targetName, status, progressPercent, currentStage, errorSummary, resultJson,
	requestPayloadPreview, requestPayloadCiphertext, requestPayloadEdek, requestPayloadWrapKeyRef,
	actorUserId, actorUsername, retryOf, cancelRequested, timeoutAt, startedAt, finishedAt, heartbeatAt, createTime, updateTime
FROM docker_operation WHERE id = ? LIMIT 1`)
	if err := sqlx.GetContext(ctx, exec, &row, query, id); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get docker operation: %w", err)
	}
	return &row, nil
}

func (r *OperationRepository) ListOperations(ctx context.Context, current, size int64, status, operationType string) (*PageResult[OperationRecord], error) {
	exec := r.executor(ctx)
	if current <= 0 {
		current = 1
	}
	if size <= 0 {
		size = 20
	}
	if size > 200 {
		size = 200
	}
	where := []string{"1=1"}
	args := []any{}
	if strings.TrimSpace(status) != "" {
		where = append(where, "status = ?")
		args = append(args, strings.TrimSpace(status))
	}
	if strings.TrimSpace(operationType) != "" {
		where = append(where, "operationType = ?")
		args = append(args, strings.TrimSpace(operationType))
	}
	whereSQL := strings.Join(where, " AND ")
	var total int64
	if err := sqlx.GetContext(ctx, exec, &total, r.rebind(exec, "SELECT COUNT(1) FROM docker_operation WHERE "+whereSQL), args...); err != nil {
		return nil, fmt.Errorf("count docker operations: %w", err)
	}
	queryArgs := append([]any{}, args...)
	queryArgs = append(queryArgs, size, (current-1)*size)
	var rows []OperationRecord
	query := r.rebind(exec, `
SELECT id, operationType, targetType, targetId, targetName, status, progressPercent, currentStage, errorSummary, resultJson,
	requestPayloadPreview, requestPayloadCiphertext, requestPayloadEdek, requestPayloadWrapKeyRef,
	actorUserId, actorUsername, retryOf, cancelRequested, timeoutAt, startedAt, finishedAt, heartbeatAt, createTime, updateTime
FROM docker_operation WHERE `+whereSQL+` ORDER BY createTime DESC, id DESC LIMIT ? OFFSET ?`)
	if err := sqlx.SelectContext(ctx, exec, &rows, query, queryArgs...); err != nil {
		return nil, fmt.Errorf("list docker operations: %w", err)
	}
	return &PageResult[OperationRecord]{Current: current, Size: size, Total: total, Records: rows}, nil
}

func (r *OperationRepository) LatestOperation(ctx context.Context, query LatestOperationQuery) (*OperationRecord, error) {
	exec := r.executor(ctx)
	where := []string{"1=1"}
	args := []any{}
	if strings.TrimSpace(query.TargetType) != "" {
		where = append(where, "targetType = ?")
		args = append(args, strings.ToLower(strings.TrimSpace(query.TargetType)))
	}
	if strings.TrimSpace(query.TargetID) != "" {
		where = append(where, "targetId = ?")
		args = append(args, strings.TrimSpace(query.TargetID))
	}
	if strings.TrimSpace(query.TargetName) != "" {
		where = append(where, "targetName = ?")
		args = append(args, strings.TrimSpace(query.TargetName))
	}
	if strings.TrimSpace(query.OperationType) != "" {
		where = append(where, "operationType = ?")
		args = append(args, strings.TrimSpace(query.OperationType))
	}
	var row OperationRecord
	sqlText := `
SELECT id, operationType, targetType, targetId, targetName, status, progressPercent, currentStage, errorSummary, resultJson,
	requestPayloadPreview, requestPayloadCiphertext, requestPayloadEdek, requestPayloadWrapKeyRef,
	actorUserId, actorUsername, retryOf, cancelRequested, timeoutAt, startedAt, finishedAt, heartbeatAt, createTime, updateTime
FROM docker_operation
WHERE ` + strings.Join(where, " AND ") + `
ORDER BY createTime DESC, id DESC
LIMIT 1`
	if err := sqlx.GetContext(ctx, exec, &row, r.rebind(exec, sqlText), args...); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("latest docker operation: %w", err)
	}
	return &row, nil
}

func (r *OperationRepository) FindOperationsByIDs(ctx context.Context, operationIDs []int64) (map[int64]OperationRecord, error) {
	ids := uniquePositiveInt64s(operationIDs)
	if len(ids) == 0 {
		return map[int64]OperationRecord{}, nil
	}
	if len(ids) > 1000 {
		return nil, fmt.Errorf("operation id set exceeds 1000")
	}
	result := make(map[int64]OperationRecord, len(ids))
	exec := r.executor(ctx)
	for start := 0; start < len(ids); start += 100 {
		end := start + 100
		if end > len(ids) {
			end = len(ids)
		}
		query, args, err := sqlx.In(`
SELECT id, operationType, targetType, targetId, targetName, status, progressPercent, currentStage, errorSummary, resultJson,
	requestPayloadPreview, requestPayloadCiphertext, requestPayloadEdek, requestPayloadWrapKeyRef,
	actorUserId, actorUsername, retryOf, cancelRequested, timeoutAt, startedAt, finishedAt, heartbeatAt, createTime, updateTime
FROM docker_operation
WHERE id IN (?)`, ids[start:end])
		if err != nil {
			return nil, err
		}
		var rows []OperationRecord
		if err := sqlx.SelectContext(ctx, exec, &rows, r.rebind(exec, query), args...); err != nil {
			return nil, fmt.Errorf("find docker operations by ids: %w", err)
		}
		for _, row := range rows {
			result[row.ID] = row
		}
	}
	return result, nil
}

func (r *OperationRepository) LatestOperationsByTargetIDs(ctx context.Context, targetType string, targetIDs []string) (map[string]OperationRecord, error) {
	ids := uniqueNonBlankStrings(targetIDs)
	if len(ids) == 0 {
		return map[string]OperationRecord{}, nil
	}
	if len(ids) > 1000 {
		return nil, fmt.Errorf("operation target id set exceeds 1000")
	}
	result := make(map[string]OperationRecord, len(ids))
	exec := r.executor(ctx)
	for start := 0; start < len(ids); start += 100 {
		end := start + 100
		if end > len(ids) {
			end = len(ids)
		}
		query, args, err := sqlx.In(`
SELECT id, operationType, targetType, targetId, targetName, status, progressPercent, currentStage, errorSummary, resultJson,
	requestPayloadPreview, requestPayloadCiphertext, requestPayloadEdek, requestPayloadWrapKeyRef,
	actorUserId, actorUsername, retryOf, cancelRequested, timeoutAt, startedAt, finishedAt, heartbeatAt, createTime, updateTime
FROM (
	SELECT id, operationType, targetType, targetId, targetName, status, progressPercent, currentStage, errorSummary, resultJson,
		requestPayloadPreview, requestPayloadCiphertext, requestPayloadEdek, requestPayloadWrapKeyRef,
		actorUserId, actorUsername, retryOf, cancelRequested, timeoutAt, startedAt, finishedAt, heartbeatAt, createTime, updateTime,
		ROW_NUMBER() OVER (PARTITION BY targetId ORDER BY createTime DESC, id DESC) AS target_rank
	FROM docker_operation
	WHERE targetType = ? AND targetId IN (?)
) ranked
WHERE target_rank = 1
ORDER BY targetId`, strings.ToLower(strings.TrimSpace(targetType)), ids[start:end])
		if err != nil {
			return nil, err
		}
		var rows []OperationRecord
		if err := sqlx.SelectContext(ctx, exec, &rows, r.rebind(exec, query), args...); err != nil {
			return nil, fmt.Errorf("find latest docker operations by target ids: %w", err)
		}
		for _, row := range rows {
			result[strings.TrimSpace(row.TargetID.String)] = row
		}
	}
	return result, nil
}

func (r *OperationRepository) MarkRunning(ctx context.Context, id int64, stage string, progress int) error {
	exec := r.executor(ctx)
	result, err := exec.ExecContext(ctx, r.rebind(exec, `
UPDATE docker_operation
SET status = ?, currentStage = ?, progressPercent = ?, startedAt = COALESCE(startedAt, NOW()), heartbeatAt = NOW(), updateTime = NOW()
WHERE id = ? AND cancelRequested = 0 AND status IN (?, ?)`), string(OperationStatusRunning), nullableString(stage), progress, id, string(OperationStatusPending), string(OperationStatusRunning))
	if err != nil {
		return fmt.Errorf("mark docker operation running: %w", err)
	}
	if rows, rowErr := result.RowsAffected(); rowErr == nil && rows == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func uniquePositiveInt64s(values []int64) []int64 {
	seen := make(map[int64]struct{}, len(values))
	result := make([]int64, 0, len(values))
	for _, value := range values {
		if value <= 0 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result
}

func uniqueNonBlankStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func (r *OperationRepository) UpdateProgress(ctx context.Context, id int64, stage string, progress int) error {
	exec := r.executor(ctx)
	_, err := exec.ExecContext(ctx, r.rebind(exec, `
UPDATE docker_operation
SET currentStage = ?, progressPercent = ?, heartbeatAt = NOW(), updateTime = NOW()
WHERE id = ?`), nullableString(stage), progress, id)
	if err != nil {
		return fmt.Errorf("update docker operation progress: %w", err)
	}
	return nil
}

func (r *OperationRepository) Finish(ctx context.Context, id int64, status OperationStatus, progress int, stage, resultJSON, errorSummary string) error {
	exec := r.executor(ctx)
	_, err := exec.ExecContext(ctx, r.rebind(exec, `
UPDATE docker_operation
SET status = ?, progressPercent = ?, currentStage = ?, resultJson = ?, errorSummary = ?, finishedAt = NOW(), heartbeatAt = NOW(), updateTime = NOW()
WHERE id = ?`), string(status), progress, nullableString(stage), nullableString(resultJSON), nullableString(errorSummary), id)
	if err != nil {
		return fmt.Errorf("finish docker operation: %w", err)
	}
	return nil
}

func (r *OperationRepository) RequestCancel(ctx context.Context, id int64) error {
	exec := r.executor(ctx)
	_, err := exec.ExecContext(ctx, r.rebind(exec, `UPDATE docker_operation SET cancelRequested = 1, updateTime = NOW() WHERE id = ? AND status IN (?, ?)`), id, string(OperationStatusPending), string(OperationStatusRunning))
	if err != nil {
		return fmt.Errorf("request docker operation cancel: %w", err)
	}
	return nil
}

// DeleteOperationWithEvents follows the repository-wide lock order:
// integrity guard -> parent operation -> child events. The child rows are
// deleted before the parent so the lifecycle stays valid after the FK is
// eventually removed.
func (r *OperationRepository) DeleteOperationWithEvents(ctx context.Context, operationID int64) (bool, error) {
	if operationID <= 0 {
		return false, fmt.Errorf("delete docker operation: operationId must be positive")
	}
	deleted := false
	err := r.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := r.lockIntegrityGuard(txCtx, operationID); err != nil {
			return err
		}
		parent, err := r.getOperationForUpdate(txCtx, operationID)
		if err != nil {
			return err
		}
		if parent == nil {
			return nil
		}
		exec := r.executor(txCtx)
		if _, err := exec.ExecContext(txCtx, r.rebind(exec, `DELETE FROM docker_operation_event WHERE operationId = ?`), operationID); err != nil {
			return fmt.Errorf("delete docker operation events first: %w", err)
		}
		result, err := exec.ExecContext(txCtx, r.rebind(exec, `DELETE FROM docker_operation WHERE id = ?`), operationID)
		if err != nil {
			return fmt.Errorf("delete docker operation: %w", err)
		}
		rows, err := result.RowsAffected()
		if err != nil {
			return fmt.Errorf("inspect deleted docker operation: %w", err)
		}
		deleted = rows == 1
		return nil
	})
	return deleted, err
}

func (r *OperationRepository) TrimEvents(ctx context.Context, operationID int64, keep int) error {
	if keep <= 0 {
		return nil
	}
	return r.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := r.lockIntegrityGuard(txCtx, operationID); err != nil {
			return err
		}
		parent, err := r.getOperationForUpdate(txCtx, operationID)
		if err != nil {
			return err
		}
		if parent == nil {
			return nil
		}
		exec := r.executor(txCtx)
		_, err = exec.ExecContext(txCtx, r.rebind(exec, `
DELETE FROM docker_operation_event
WHERE operationId = ?
  AND sequence <= COALESCE((
    SELECT cutoff FROM (
      SELECT MAX(sequence) - ? AS cutoff
      FROM docker_operation_event
      WHERE operationId = ?
    ) t
	), 0)`), operationID, keep, operationID)
		if err != nil {
			return fmt.Errorf("trim docker operation events: %w", err)
		}
		return nil
	})
}

func (r *OperationRepository) CancelRequested(ctx context.Context, id int64) (bool, error) {
	row, err := r.GetOperation(ctx, id)
	if err != nil || row == nil {
		return false, err
	}
	return row.CancelRequested, nil
}

func (r *OperationRepository) AppendEvent(ctx context.Context, row OperationEventRecord) error {
	if row.ID <= 0 || row.OperationID <= 0 || strings.TrimSpace(row.EventType) == "" {
		return fmt.Errorf("insert docker operation event: id, operationId, and eventType are required")
	}
	return r.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := r.lockIntegrityGuard(txCtx, row.OperationID); err != nil {
			return err
		}
		parent, err := r.getOperationForUpdate(txCtx, row.OperationID)
		if err != nil {
			return err
		}
		if parent == nil {
			return ErrOperationParentNotFound
		}
		existing, err := r.getEventByID(txCtx, row.ID)
		if err != nil {
			return err
		}
		if existing != nil {
			if operationEventInsertMatches(*existing, row) {
				return nil
			}
			return ErrOperationEventMutationConflict
		}
		exec := r.executor(txCtx)
		if row.Sequence <= 0 {
			if err := sqlx.GetContext(txCtx, exec, &row.Sequence, r.rebind(exec, `SELECT COALESCE(MAX(sequence), 0) + 1 FROM docker_operation_event WHERE operationId = ?`), row.OperationID); err != nil {
				return fmt.Errorf("next docker operation event sequence: %w", err)
			}
		}
		_, err = exec.ExecContext(txCtx, r.rebind(exec, `
INSERT INTO docker_operation_event (id, operationId, sequence, eventType, stage, percent, message, payloadJson, occurredAt)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, NOW())`),
			row.ID, row.OperationID, row.Sequence, row.EventType, nullableString(row.Stage.String),
			nullableInt64(row.Percent.Int64), nullableString(row.Message.String), nullableString(row.PayloadJSON.String))
		if err == nil {
			return nil
		}
		if isDuplicateKeyError(err) {
			existing, lookupErr := r.getEventByID(txCtx, row.ID)
			if lookupErr == nil && existing != nil && operationEventInsertMatches(*existing, row) {
				return nil
			}
			return ErrOperationEventMutationConflict
		}
		return fmt.Errorf("insert docker operation event: %w", err)
	})
}

// DiagnoseOrphanEvents records a bounded orphan set by quarantining the exact
// immutable event versions. It never deletes data.
func (r *OperationRepository) DiagnoseOrphanEvents(ctx context.Context, afterEventID int64, limit int) ([]OperationEventOrphanDiagnostic, error) {
	if afterEventID < 0 {
		return nil, fmt.Errorf("diagnose docker operation event orphans: afterEventId must not be negative")
	}
	if limit <= 0 {
		limit = 100
	}
	if limit > 200 {
		limit = 200
	}
	var diagnostics []OperationEventOrphanDiagnostic
	err := r.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		exec := r.executor(txCtx)
		var candidates []OperationEventOrphanDiagnostic
		query := r.rebind(exec, `
SELECT e.id, e.operationId, e.sequence, e.diagnosticId, e.integrityVersion,
	e.integrityScope, e.integrityRelationshipType, e.integrityReason, e.occurredAt
FROM docker_operation_event e
LEFT JOIN docker_operation o ON o.id = e.operationId
WHERE e.id > ? AND o.id IS NULL
ORDER BY e.id ASC
LIMIT ?`)
		if err := sqlx.SelectContext(txCtx, exec, &candidates, query, afterEventID, limit); err != nil {
			return fmt.Errorf("scan docker operation event orphans: %w", err)
		}
		if len(candidates) == 0 {
			return nil
		}
		operationIDs := uniqueSortedOperationIDs(candidates)
		if err := r.ensureAndLockIntegrityGuards(txCtx, operationIDs); err != nil {
			return err
		}
		presentParents, err := r.listPresentOperationIDs(txCtx, operationIDs)
		if err != nil {
			return err
		}
		confirmed := make([]OperationEventOrphanDiagnostic, 0, len(candidates))
		for _, candidate := range candidates {
			if _, exists := presentParents[candidate.OperationID]; !exists {
				confirmed = append(confirmed, candidate)
			}
		}
		if len(confirmed) == 0 {
			return nil
		}
		if err := r.quarantineOperationEventOrphans(txCtx, confirmed); err != nil {
			return err
		}
		diagnostics, err = r.listDiagnosedOperationEventOrphans(txCtx, confirmed)
		if err != nil {
			return err
		}
		return nil
	})
	return diagnostics, err
}

// CleanupOrphanEvent is intentionally an audited, exact-version mutation. A
// caller cannot use it as an unbounded global-delete path.
func (r *OperationRepository) CleanupOrphanEvent(ctx context.Context, command OperationEventOrphanCleanupCommand) (OperationEventOrphanCleanupResult, error) {
	if err := validateOperationEventOrphanCleanup(command); err != nil {
		return "", err
	}
	result := OperationEventOrphanCleanupResult("")
	err := r.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := r.lockIntegrityGuard(txCtx, command.OperationID); err != nil {
			return err
		}
		if prior, found, err := r.getOrphanCleanupAudit(txCtx, command.AuditID); err != nil {
			return err
		} else if found {
			if !orphanCleanupAuditMatches(*prior, command) {
				return ErrOperationEventMutationConflict
			}
			result = OperationEventOrphanCleanupResult(prior.Result)
			return nil
		}
		parent, err := r.getOperationForUpdate(txCtx, command.OperationID)
		if err != nil {
			return err
		}
		if parent != nil {
			if err := r.resolveDiagnosedOrphan(txCtx, command); err != nil {
				return err
			}
			result = OperationEventOrphanCleanupParentPresent
			return r.insertOrphanCleanupAudit(txCtx, command, result)
		}
		event, err := r.getQuarantinedEventForUpdate(txCtx, command.EventID)
		if err != nil {
			return err
		}
		if event == nil {
			return ErrOperationOrphanChanged
		}
		if event.OperationID != command.OperationID ||
			event.Sequence != command.Sequence ||
			!event.DiagnosticID.Valid ||
			event.DiagnosticID.String != command.DiagnosticID ||
			event.IntegrityVersion != command.ExpectedIntegrityVersion ||
			!operationEventDiagnosticMetadataMatches(*event) {
			return ErrOperationOrphanChanged
		}
		exec := r.executor(txCtx)
		deleteResult, err := exec.ExecContext(txCtx, r.rebind(exec, `
DELETE FROM docker_operation_event
WHERE id = ? AND operationId = ? AND sequence = ? AND diagnosticId = ? AND integrityVersion = ?`),
			command.EventID, command.OperationID, command.Sequence, command.DiagnosticID, command.ExpectedIntegrityVersion)
		if err != nil {
			return fmt.Errorf("delete diagnosed docker operation event orphan: %w", err)
		}
		rows, err := deleteResult.RowsAffected()
		if err != nil {
			return fmt.Errorf("inspect docker operation event orphan cleanup: %w", err)
		}
		if rows != 1 {
			return ErrOperationOrphanChanged
		}
		result = OperationEventOrphanCleanupDeleted
		return r.insertOrphanCleanupAudit(txCtx, command, result)
	})
	return result, err
}

func (r *OperationRepository) ListEvents(ctx context.Context, operationID, afterSequence int64, limit int) ([]OperationEventRecord, error) {
	exec := r.executor(ctx)
	if limit <= 0 {
		limit = 200
	}
	if limit > 1000 {
		limit = 1000
	}
	var rows []OperationEventRecord
	query := r.rebind(exec, `
SELECT id, operationId, sequence, eventType, stage, percent, message, payloadJson, occurredAt
FROM docker_operation_event
WHERE operationId = ? AND sequence > ?
ORDER BY sequence ASC
LIMIT ?`)
	if err := sqlx.SelectContext(ctx, exec, &rows, query, operationID, afterSequence, limit); err != nil {
		return nil, fmt.Errorf("list docker operation events: %w", err)
	}
	return rows, nil
}

func (r *OperationRepository) rebind(exec store.SQLX, query string) string {
	if r.dialect == "postgres" || r.dialect == "postgresql" || r.dialect == "pgx" {
		query = dockerOperationPostgresRenderer.RenderPostgres(query)
	}
	return exec.Rebind(query)
}

func (r *OperationRepository) booleanValue(value bool) any {
	if r.dialect == "postgres" || r.dialect == "postgresql" || r.dialect == "pgx" {
		return value
	}
	return boolInt(value)
}

func (r *OperationRepository) lockIntegrityGuard(ctx context.Context, operationID int64) error {
	exec := r.executor(ctx)
	var lockedID int64
	lockSQL := r.rebind(exec, `
SELECT operationId FROM docker_operation_integrity_guard WHERE operationId = ? FOR UPDATE`)
	err := sqlx.GetContext(ctx, exec, &lockedID, lockSQL, operationID)
	if err == nil {
		return nil
	}
	if err != sql.ErrNoRows {
		return fmt.Errorf("lock docker operation integrity guard: %w", err)
	}
	insertSQL := `INSERT IGNORE INTO docker_operation_integrity_guard (operationId, createTime) VALUES (?, NOW())`
	if r.dialect == "postgres" || r.dialect == "postgresql" || r.dialect == "pgx" {
		insertSQL = `INSERT INTO docker_operation_integrity_guard (operationId, createTime) VALUES (?, NOW()) ON CONFLICT (operationId) DO NOTHING`
	}
	if _, err := exec.ExecContext(ctx, r.rebind(exec, insertSQL), operationID); err != nil {
		return fmt.Errorf("ensure docker operation integrity guard: %w", err)
	}
	if err := sqlx.GetContext(ctx, exec, &lockedID, lockSQL, operationID); err != nil {
		return fmt.Errorf("lock docker operation integrity guard: %w", err)
	}
	return nil
}

func (r *OperationRepository) ensureAndLockIntegrityGuards(ctx context.Context, operationIDs []int64) error {
	if len(operationIDs) == 0 {
		return nil
	}
	exec := r.executor(ctx)
	query, queryArgs, err := r.inQuery(exec, `
SELECT operationId
FROM docker_operation_integrity_guard
WHERE operationId IN (?)
ORDER BY operationId ASC
FOR UPDATE`, operationIDs)
	if err != nil {
		return fmt.Errorf("build docker operation integrity guard lock: %w", err)
	}
	var locked []int64
	if err := sqlx.SelectContext(ctx, exec, &locked, query, queryArgs...); err != nil {
		return fmt.Errorf("lock docker operation integrity guards: %w", err)
	}
	lockedSet := make(map[int64]struct{}, len(locked))
	for _, id := range locked {
		lockedSet[id] = struct{}{}
	}
	missing := make([]int64, 0, len(operationIDs)-len(locked))
	for _, id := range operationIDs {
		if _, exists := lockedSet[id]; !exists {
			missing = append(missing, id)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	values := make([]string, 0, len(missing))
	insertArgs := make([]any, 0, len(missing))
	for _, operationID := range missing {
		values = append(values, "(?, NOW())")
		insertArgs = append(insertArgs, operationID)
	}
	insertSQL := `INSERT IGNORE INTO docker_operation_integrity_guard (operationId, createTime) VALUES ` + strings.Join(values, ", ")
	if r.dialect == "postgres" || r.dialect == "postgresql" || r.dialect == "pgx" {
		insertSQL = `INSERT INTO docker_operation_integrity_guard (operationId, createTime) VALUES ` +
			strings.Join(values, ", ") + ` ON CONFLICT (operationId) DO NOTHING`
	}
	if _, err := exec.ExecContext(ctx, r.rebind(exec, insertSQL), insertArgs...); err != nil {
		return fmt.Errorf("ensure docker operation integrity guards: %w", err)
	}
	missingQuery, missingArgs, err := r.inQuery(exec, `
SELECT operationId
FROM docker_operation_integrity_guard
WHERE operationId IN (?)
ORDER BY operationId ASC
FOR UPDATE`, missing)
	if err != nil {
		return fmt.Errorf("build missing docker operation integrity guard lock: %w", err)
	}
	var newlyLocked []int64
	if err := sqlx.SelectContext(ctx, exec, &newlyLocked, missingQuery, missingArgs...); err != nil {
		return fmt.Errorf("lock missing docker operation integrity guards: %w", err)
	}
	if len(newlyLocked) != len(missing) {
		return fmt.Errorf("lock docker operation integrity guards: locked %d of %d missing guards", len(newlyLocked), len(missing))
	}
	return nil
}

func (r *OperationRepository) listPresentOperationIDs(ctx context.Context, operationIDs []int64) (map[int64]struct{}, error) {
	present := make(map[int64]struct{}, len(operationIDs))
	if len(operationIDs) == 0 {
		return present, nil
	}
	exec := r.executor(ctx)
	query, args, err := r.inQuery(exec, `SELECT id FROM docker_operation WHERE id IN (?)`, operationIDs)
	if err != nil {
		return nil, fmt.Errorf("build docker operation parent recheck: %w", err)
	}
	var ids []int64
	if err := sqlx.SelectContext(ctx, exec, &ids, query, args...); err != nil {
		return nil, fmt.Errorf("recheck docker operation parents: %w", err)
	}
	for _, id := range ids {
		present[id] = struct{}{}
	}
	return present, nil
}

func (r *OperationRepository) quarantineOperationEventOrphans(ctx context.Context, candidates []OperationEventOrphanDiagnostic) error {
	if len(candidates) == 0 {
		return nil
	}
	exec := r.executor(ctx)
	caseParts := make([]string, 0, len(candidates))
	args := make([]any, 0, len(candidates)*3)
	eventIDs := make([]int64, 0, len(candidates))
	for _, candidate := range candidates {
		caseParts = append(caseParts, "WHEN ? THEN ?")
		args = append(args, candidate.EventID, operationEventDiagnosticID(candidate.EventID, candidate.OperationID, candidate.Sequence))
		eventIDs = append(eventIDs, candidate.EventID)
	}
	baseSQL := `
UPDATE docker_operation_event
SET integrityVersion = CASE WHEN diagnosticId IS NULL THEN integrityVersion + 1 ELSE integrityVersion END,
	integrityStatus = ?,
	diagnosticId = CASE id ` + strings.Join(caseParts, " ") + ` END,
	integrityScope = ?,
	integrityRelationshipType = ?,
	integrityReason = ?,
	diagnosedAt = COALESCE(diagnosedAt, NOW())
WHERE id IN (?)
  AND integrityStatus IN (?, ?)
  AND NOT EXISTS (
	SELECT 1 FROM docker_operation o WHERE o.id = docker_operation_event.operationId
  )`
	queryArgs := make([]any, 0, len(args)+6)
	queryArgs = append(queryArgs, "QUARANTINED")
	queryArgs = append(queryArgs, args...)
	queryArgs = append(queryArgs, operationEventIntegrityScope, operationEventIntegrityRelationshipType, operationEventIntegrityReason)
	queryArgs = append(queryArgs, eventIDs, "ACTIVE", "QUARANTINED")
	query, queryArgs, err := sqlx.In(baseSQL, queryArgs...)
	if err != nil {
		return fmt.Errorf("build docker operation event orphan quarantine: %w", err)
	}
	if _, err := exec.ExecContext(ctx, r.rebind(exec, query), queryArgs...); err != nil {
		return fmt.Errorf("quarantine docker operation event orphans: %w", err)
	}
	return nil
}

func (r *OperationRepository) listDiagnosedOperationEventOrphans(ctx context.Context, candidates []OperationEventOrphanDiagnostic) ([]OperationEventOrphanDiagnostic, error) {
	if len(candidates) == 0 {
		return nil, nil
	}
	eventIDs := make([]int64, 0, len(candidates))
	for _, candidate := range candidates {
		eventIDs = append(eventIDs, candidate.EventID)
	}
	exec := r.executor(ctx)
	query, args, err := r.inQuery(exec, `
SELECT id, operationId, sequence, diagnosticId, integrityVersion,
	integrityScope, integrityRelationshipType, integrityReason, occurredAt
FROM docker_operation_event
WHERE id IN (?) AND integrityStatus = ?
ORDER BY id ASC`, eventIDs, "QUARANTINED")
	if err != nil {
		return nil, fmt.Errorf("build diagnosed docker operation event query: %w", err)
	}
	var diagnostics []OperationEventOrphanDiagnostic
	if err := sqlx.SelectContext(ctx, exec, &diagnostics, query, args...); err != nil {
		return nil, fmt.Errorf("list diagnosed docker operation event orphans: %w", err)
	}
	return diagnostics, nil
}

func (r *OperationRepository) inQuery(exec store.SQLX, query string, args ...any) (string, []any, error) {
	query, flattened, err := sqlx.In(query, args...)
	if err != nil {
		return "", nil, err
	}
	return r.rebind(exec, query), flattened, nil
}

func uniqueSortedOperationIDs(candidates []OperationEventOrphanDiagnostic) []int64 {
	seen := make(map[int64]struct{}, len(candidates))
	for _, candidate := range candidates {
		seen[candidate.OperationID] = struct{}{}
	}
	ids := make([]int64, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(left, right int) bool { return ids[left] < ids[right] })
	return ids
}

func (r *OperationRepository) getOperationForUpdate(ctx context.Context, operationID int64) (*OperationRecord, error) {
	exec := r.executor(ctx)
	var row OperationRecord
	query := r.rebind(exec, `
SELECT id, operationType, targetType, targetId, targetName, status, progressPercent, currentStage, errorSummary, resultJson,
	requestPayloadPreview, requestPayloadCiphertext, requestPayloadEdek, requestPayloadWrapKeyRef,
	actorUserId, actorUsername, retryOf, cancelRequested, timeoutAt, startedAt, finishedAt, heartbeatAt, createTime, updateTime
FROM docker_operation
WHERE id = ?
FOR UPDATE`)
	if err := sqlx.GetContext(ctx, exec, &row, query, operationID); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("lock docker operation parent: %w", err)
	}
	return &row, nil
}

func (r *OperationRepository) getEventByID(ctx context.Context, eventID int64) (*OperationEventRecord, error) {
	exec := r.executor(ctx)
	var row OperationEventRecord
	query := r.rebind(exec, `
SELECT id, operationId, sequence, eventType, stage, percent, message, payloadJson, occurredAt
FROM docker_operation_event
WHERE id = ?
LIMIT 1`)
	if err := sqlx.GetContext(ctx, exec, &row, query, eventID); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get docker operation event idempotency record: %w", err)
	}
	return &row, nil
}

func (r *OperationRepository) getQuarantinedEventForUpdate(ctx context.Context, eventID int64) (*OperationEventOrphanDiagnostic, error) {
	exec := r.executor(ctx)
	var row OperationEventOrphanDiagnostic
	query := r.rebind(exec, `
SELECT id, operationId, sequence, diagnosticId, integrityVersion,
	integrityScope, integrityRelationshipType, integrityReason, occurredAt
FROM docker_operation_event
WHERE id = ? AND integrityStatus = ?
FOR UPDATE`)
	if err := sqlx.GetContext(ctx, exec, &row, query, eventID, "QUARANTINED"); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("lock diagnosed docker operation event orphan: %w", err)
	}
	return &row, nil
}

func (r *OperationRepository) getOrphanCleanupAudit(ctx context.Context, auditID int64) (*operationEventOrphanAuditRecord, bool, error) {
	exec := r.executor(ctx)
	var row operationEventOrphanAuditRecord
	query := r.rebind(exec, `
SELECT diagnosticId, eventId, operationId, sequence, expectedIntegrityVersion, action, result, actorUserId, actorUsername, reason
FROM docker_operation_event_orphan_audit
WHERE id = ?
LIMIT 1`)
	if err := sqlx.GetContext(ctx, exec, &row, query, auditID); err != nil {
		if err == sql.ErrNoRows {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("get docker operation event orphan audit: %w", err)
	}
	return &row, true, nil
}

func (r *OperationRepository) resolveDiagnosedOrphan(ctx context.Context, command OperationEventOrphanCleanupCommand) error {
	event, err := r.getQuarantinedEventForUpdate(ctx, command.EventID)
	if err != nil {
		return err
	}
	if event == nil ||
		event.OperationID != command.OperationID ||
		event.Sequence != command.Sequence ||
		!event.DiagnosticID.Valid ||
		event.DiagnosticID.String != command.DiagnosticID ||
		event.IntegrityVersion != command.ExpectedIntegrityVersion ||
		!operationEventDiagnosticMetadataMatches(*event) {
		return ErrOperationOrphanChanged
	}
	exec := r.executor(ctx)
	result, err := exec.ExecContext(ctx, r.rebind(exec, `
UPDATE docker_operation_event
SET integrityStatus = ?, diagnosticId = NULL, integrityVersion = integrityVersion + 1,
	integrityScope = NULL, integrityRelationshipType = NULL, integrityReason = NULL
WHERE id = ? AND operationId = ? AND sequence = ? AND diagnosticId = ? AND integrityVersion = ?`),
		"ACTIVE", command.EventID, command.OperationID, command.Sequence, command.DiagnosticID, command.ExpectedIntegrityVersion)
	if err != nil {
		return fmt.Errorf("resolve repaired docker operation event orphan: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("inspect repaired docker operation event orphan: %w", err)
	}
	if rows != 1 {
		return ErrOperationOrphanChanged
	}
	return nil
}

func (r *OperationRepository) insertOrphanCleanupAudit(ctx context.Context, command OperationEventOrphanCleanupCommand, result OperationEventOrphanCleanupResult) error {
	exec := r.executor(ctx)
	_, err := exec.ExecContext(ctx, r.rebind(exec, `
INSERT INTO docker_operation_event_orphan_audit (
	id, diagnosticId, eventId, operationId, sequence, expectedIntegrityVersion, action, result,
	actorUserId, actorUsername, reason, createTime
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NOW())`),
		command.AuditID, command.DiagnosticID, command.EventID, command.OperationID, command.Sequence,
		command.ExpectedIntegrityVersion, "DELETE", string(result), command.ActorUserID,
		strings.TrimSpace(command.ActorUsername), strings.TrimSpace(command.Reason))
	if err != nil {
		if isDuplicateKeyError(err) {
			return ErrOperationEventMutationConflict
		}
		return fmt.Errorf("audit docker operation event orphan cleanup: %w", err)
	}
	return nil
}

func validateOperationEventOrphanCleanup(command OperationEventOrphanCleanupCommand) error {
	if command.AuditID <= 0 ||
		command.EventID <= 0 ||
		command.OperationID <= 0 ||
		command.Sequence <= 0 ||
		command.ExpectedIntegrityVersion <= 0 ||
		strings.TrimSpace(command.DiagnosticID) == "" ||
		command.ActorUserID <= 0 ||
		strings.TrimSpace(command.ActorUsername) == "" ||
		strings.TrimSpace(command.Reason) == "" {
		return fmt.Errorf("cleanup docker operation event orphan: audit, diagnostic, exact version, actor, and reason are required")
	}
	if command.DiagnosticID != operationEventDiagnosticID(command.EventID, command.OperationID, command.Sequence) {
		return fmt.Errorf("cleanup docker operation event orphan: diagnostic identity mismatch")
	}
	if len(command.DiagnosticID) > 191 || len(strings.TrimSpace(command.ActorUsername)) > 128 || len(strings.TrimSpace(command.Reason)) > 512 {
		return fmt.Errorf("cleanup docker operation event orphan: diagnostic, actor, or reason exceeds storage limit")
	}
	return nil
}

func operationEventDiagnosticID(eventID, operationID, sequence int64) string {
	return fmt.Sprintf("docker-operation-event:%d:%d:%d", eventID, operationID, sequence)
}

func operationEventDiagnosticMetadataMatches(event OperationEventOrphanDiagnostic) bool {
	return event.IntegrityScope.Valid &&
		event.IntegrityScope.String == operationEventIntegrityScope &&
		event.IntegrityRelationshipType.Valid &&
		event.IntegrityRelationshipType.String == operationEventIntegrityRelationshipType &&
		event.IntegrityReason.Valid &&
		event.IntegrityReason.String == operationEventIntegrityReason
}

func orphanCleanupAuditMatches(existing operationEventOrphanAuditRecord, command OperationEventOrphanCleanupCommand) bool {
	return existing.DiagnosticID == command.DiagnosticID &&
		existing.EventID == command.EventID &&
		existing.OperationID == command.OperationID &&
		existing.Sequence == command.Sequence &&
		existing.ExpectedIntegrityVersion == command.ExpectedIntegrityVersion &&
		existing.Action == "DELETE" &&
		existing.ActorUserID == command.ActorUserID &&
		existing.ActorUsername == strings.TrimSpace(command.ActorUsername) &&
		existing.Reason == strings.TrimSpace(command.Reason)
}

func operationInsertMatches(existing, requested OperationRecord) bool {
	return existing.ID == requested.ID &&
		existing.OperationType == requested.OperationType &&
		existing.TargetType == requested.TargetType &&
		nullStringEqual(existing.TargetID, requested.TargetID) &&
		nullStringEqual(existing.TargetName, requested.TargetName) &&
		existing.Status == requested.Status &&
		existing.ProgressPercent == requested.ProgressPercent &&
		nullStringEqual(existing.CurrentStage, requested.CurrentStage) &&
		nullStringEqual(existing.ErrorSummary, requested.ErrorSummary) &&
		nullStringEqual(existing.ResultJSON, requested.ResultJSON) &&
		nullStringEqual(existing.RequestPayloadPreview, requested.RequestPayloadPreview) &&
		nullStringEqual(existing.RequestPayloadCiphertext, requested.RequestPayloadCiphertext) &&
		nullStringEqual(existing.RequestPayloadEDEK, requested.RequestPayloadEDEK) &&
		nullStringEqual(existing.RequestPayloadWrapKeyRef, requested.RequestPayloadWrapKeyRef) &&
		nullInt64Equal(existing.ActorUserID, requested.ActorUserID) &&
		nullStringEqual(existing.ActorUsername, requested.ActorUsername) &&
		nullInt64Equal(existing.RetryOf, requested.RetryOf) &&
		existing.CancelRequested == requested.CancelRequested &&
		nullTimeEqual(existing.TimeoutAt, requested.TimeoutAt)
}

func operationEventInsertMatches(existing, requested OperationEventRecord) bool {
	return existing.ID == requested.ID &&
		existing.OperationID == requested.OperationID &&
		(requested.Sequence <= 0 || existing.Sequence == requested.Sequence) &&
		existing.EventType == requested.EventType &&
		nullStringEqual(existing.Stage, requested.Stage) &&
		nullInt64Equal(existing.Percent, requested.Percent) &&
		nullStringEqual(existing.Message, requested.Message) &&
		nullStringEqual(existing.PayloadJSON, requested.PayloadJSON)
}

func nullStringEqual(left, right sql.NullString) bool {
	leftValue := strings.TrimSpace(left.String)
	rightValue := strings.TrimSpace(right.String)
	return leftValue == rightValue && (left.Valid || leftValue != "") == (right.Valid || rightValue != "")
}

func nullInt64Equal(left, right sql.NullInt64) bool {
	return left.Int64 == right.Int64 && (left.Valid || left.Int64 != 0) == (right.Valid || right.Int64 != 0)
}

func nullTimeEqual(left, right sql.NullTime) bool {
	leftValid := left.Valid && !left.Time.IsZero()
	rightValid := right.Valid && !right.Time.IsZero()
	if leftValid != rightValid {
		return false
	}
	return !leftValid || left.Time.UTC().Truncate(time.Second).Equal(right.Time.UTC().Truncate(time.Second))
}

func isDuplicateKeyError(err error) bool {
	if err == nil {
		return false
	}
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "duplicate") || strings.Contains(message, "1062")
}

func (r *OperationRepository) OperationStats(ctx context.Context) (total, succeeded, failed, policyViolations int64, err error) {
	exec := r.executor(ctx)
	if err = sqlx.GetContext(ctx, exec, &total, r.rebind(exec, `SELECT COUNT(1) FROM docker_operation`)); err != nil {
		err = fmt.Errorf("count docker operations: %w", err)
		return
	}
	if err = sqlx.GetContext(ctx, exec, &succeeded, r.rebind(exec, `SELECT COUNT(1) FROM docker_operation WHERE status = ?`), string(OperationStatusSucceeded)); err != nil {
		err = fmt.Errorf("count docker succeeded operations: %w", err)
		return
	}
	if err = sqlx.GetContext(ctx, exec, &failed, r.rebind(exec, `SELECT COUNT(1) FROM docker_operation WHERE status IN (?, ?, ?)`), string(OperationStatusFailed), string(OperationStatusCancelled), string(OperationStatusTimeout)); err != nil {
		err = fmt.Errorf("count docker failed operations: %w", err)
		return
	}
	if err = sqlx.GetContext(ctx, exec, &policyViolations, r.rebind(exec, `SELECT COUNT(1) FROM docker_operation_event WHERE eventType = ?`), string(OperationEventPolicy)); err != nil {
		err = fmt.Errorf("count docker policy events: %w", err)
	}
	return
}

func (r *OperationRecord) PayloadSecret() secretvalueinfra.SecretValue {
	if r == nil {
		return secretvalueinfra.SecretValue{}
	}
	return secretvalueinfra.SecretValue{
		CiphertextB64: r.RequestPayloadCiphertext.String,
		EDEKB64:       r.RequestPayloadEDEK.String,
		WrapKeyRef:    r.RequestPayloadWrapKeyRef.String,
	}
}

func nullableOperationTime(value sql.NullTime) any {
	if !value.Valid || value.Time.IsZero() {
		return nil
	}
	// MySQL DATETIME stores whole seconds in the current schema, while
	// PostgreSQL timestamps retain microseconds. Persist the portable common
	// precision so a retry compares identically after either database reloads it.
	return value.Time.UTC().Truncate(time.Second)
}

func jsonMap(value string) map[string]any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(value), &result); err != nil {
		return map[string]any{"raw": value}
	}
	return result
}
