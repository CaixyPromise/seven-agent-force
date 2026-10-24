package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/executionaccess/domain"
)

type Store struct{}

func New() *Store { return &Store{} }

func (s *Store) FindOccurrence(ctx context.Context, tx pgx.Tx, attempt, generation int64, key string) (*domain.Operation, error) {
	return scanOptionalOperation(tx.QueryRow(ctx, operationSelect+` WHERE attempt_id=$1 AND adapter_session_generation=$2 AND local_tool_call_key=$3`, attempt, generation, key))
}

func (s *Store) GetOperation(ctx context.Context, tx pgx.Tx, tenant, task, id int64, lock bool) (domain.Operation, error) {
	query := operationSelect + ` WHERE tenant_id=$1 AND task_id=$2 AND id=$3`
	if lock {
		query += ` FOR UPDATE`
	}
	o, err := scanOperation(tx.QueryRow(ctx, query, tenant, task, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return o, domain.ErrNotFound
	}
	return o, err
}

func (s *Store) PendingSuspendOperations(ctx context.Context, tx pgx.Tx, tenant, task, attempt, generation int64) ([]domain.Operation, error) {
	rows, err := tx.Query(ctx, operationSelect+` WHERE tenant_id=$1 AND task_id=$2 AND attempt_id=$3 AND adapter_session_generation=$4 AND status='WAITING_APPROVAL' ORDER BY id`, tenant, task, attempt, generation)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Operation
	for rows.Next() {
		o, scanErr := scanOperation(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

func (s *Store) NextReadyOperation(ctx context.Context, tx pgx.Tx, tenant, task, attempt, generation int64) (*domain.Operation, error) {
	return scanOptionalOperation(tx.QueryRow(ctx, operationSelect+nextReadyOperationClause, tenant, task, attempt, generation))
}

func (s *Store) RecoverableClaim(ctx context.Context, tx pgx.Tx, tenant, task, attempt, generation, worker, owner int64, instance, hostBootID, leaseDigest string) (*domain.Operation, error) {
	var id int64
	err := tx.QueryRow(ctx, recoverableClaimQuery, tenant, task, attempt, generation, worker, owner, instance, hostBootID, leaseDigest).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	o, err := s.GetOperation(ctx, tx, tenant, task, id, false)
	if err != nil {
		return nil, err
	}
	return &o, nil
}

const recoverableClaimQuery = `SELECT o.id FROM workforce_execution_operations o JOIN workforce_execution_operation_attempts a ON a.operation_id=o.id WHERE o.tenant_id=$1 AND o.task_id=$2 AND o.attempt_id=$3 AND o.adapter_session_generation=$4 AND o.status IN ('DISPATCHED','RUNNING') AND a.worker_id=$5 AND a.owner_epoch=$6 AND a.worker_instance_id=$7 AND a.host_boot_id=$8 AND a.lease_digest=$9 AND a.status IN ('DISPATCHED','RUNNING') AND a.lease_expires_at>CURRENT_TIMESTAMP ORDER BY o.id LIMIT 1 FOR UPDATE OF o,a SKIP LOCKED`

func (s *Store) InsertOperation(ctx context.Context, tx pgx.Tx, o domain.Operation) error {
	targets, err := json.Marshal(o.Targets)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO workforce_execution_operations
(id,tenant_id,task_id,task_turn_id,attempt_id,adapter_session_generation,authorization_grant_id,local_tool_call_key,native_tool_call_id,round_key,operation_kind,
	 descriptor_ciphertext,descriptor_edek,descriptor_key_ref,descriptor_digest,purpose,targets_json,status,version,
	 result_delivery_status,agent_continued,result_digest,result_summary,exit_code,expires_at,created_at,updated_at,started_at,finished_at,
	 result_output_ciphertext,result_output_edek,result_output_key_ref)
VALUES($1,$2,$3,$4,$5,$6,NULLIF($7::BIGINT,0),$8,$9,$10,$11,decode($12,'base64'),decode($13,'base64'),$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,$28,$29,decode(NULLIF($30,''),'base64'),decode(NULLIF($31,''),'base64'),NULLIF($32,''))`,
		o.ID, o.TenantID, o.TaskID, o.TaskTurnID, o.AttemptID, o.AdapterSessionGeneration, o.AuthorizationGrantID, o.LocalToolCallKey, o.NativeToolCallID, o.RoundKey, o.Kind,
		o.Descriptor.Ciphertext, o.Descriptor.EDEK, o.Descriptor.KeyRef, o.DescriptorDigest, o.Purpose, targets, o.Status, o.Version,
		o.ResultDeliveryStatus, o.AgentContinued, o.ResultDigest, o.ResultSummary, o.ExitCode, o.ExpiresAt, o.CreatedAt, o.UpdatedAt, o.StartedAt, o.FinishedAt,
		o.ResultOutput.Ciphertext, o.ResultOutput.EDEK, o.ResultOutput.KeyRef)
	return err
}

func (s *Store) UpdateOperation(ctx context.Context, tx pgx.Tx, o domain.Operation) error {
	tag, err := tx.Exec(ctx, `UPDATE workforce_execution_operations SET status=$2,version=$3,authorization_grant_id=NULLIF($4::BIGINT,0),result_delivery_status=$5,agent_continued=$6,result_digest=$7,result_summary=$8,exit_code=$9,updated_at=$10,started_at=$11,finished_at=$12,result_output_ciphertext=decode(NULLIF($13,''),'base64'),result_output_edek=decode(NULLIF($14,''),'base64'),result_output_key_ref=NULLIF($15,'') WHERE id=$1 AND tenant_id=$16 AND task_id=$17 AND version=$18`, o.ID, o.Status, o.Version, o.AuthorizationGrantID, o.ResultDeliveryStatus, o.AgentContinued, o.ResultDigest, o.ResultSummary, o.ExitCode, o.UpdatedAt, o.StartedAt, o.FinishedAt, o.ResultOutput.Ciphertext, o.ResultOutput.EDEK, o.ResultOutput.KeyRef, o.TenantID, o.TaskID, o.Version-1)
	if err == nil && tag.RowsAffected() != 1 {
		return domain.ErrStale
	}
	return err
}

func (s *Store) FindDecision(ctx context.Context, tx pgx.Tx, tenant, task, principal int64, key string) (*domain.Decision, error) {
	var d domain.Decision
	err := tx.QueryRow(ctx, `SELECT id,tenant_id,task_id,operation_id,actor_principal_id,action,idempotency_key,request_digest,reason,status,created_at FROM workforce_execution_operation_decisions WHERE tenant_id=$1 AND task_id=$2 AND actor_principal_id=$3 AND idempotency_key=$4`, tenant, task, principal, key).Scan(&d.ID, &d.TenantID, &d.TaskID, &d.OperationID, &d.ActorPrincipalID, &d.Action, &d.IdempotencyKey, &d.RequestDigest, &d.Reason, &d.Status, &d.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &d, err
}

func (s *Store) InsertDecision(ctx context.Context, tx pgx.Tx, d domain.Decision) error {
	_, err := tx.Exec(ctx, `INSERT INTO workforce_execution_operation_decisions(id,tenant_id,task_id,operation_id,actor_principal_id,action,idempotency_key,request_digest,reason,status,created_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, d.ID, d.TenantID, d.TaskID, d.OperationID, d.ActorPrincipalID, d.Action, d.IdempotencyKey, d.RequestDigest, d.Reason, d.Status, d.CreatedAt)
	return err
}

func (s *Store) ActiveGrants(ctx context.Context, tx pgx.Tx, tenant, task int64, lock bool) ([]domain.Grant, error) {
	query := grantSelect + ` WHERE tenant_id=$1 AND task_id=$2 ORDER BY id DESC`
	if lock {
		query += ` FOR UPDATE`
	}
	rows, err := tx.Query(ctx, query, tenant, task)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.Grant
	for rows.Next() {
		g, err := scanGrant(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (s *Store) GrantAudits(ctx context.Context, tx pgx.Tx, tenant, task int64) ([]domain.GrantAudit, error) {
	rows, err := tx.Query(ctx, grantAuditSelect, tenant, task)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []domain.GrantAudit
	var grantIDs []int64
	var sourceOperationIDs []int64
	for rows.Next() {
		var item domain.GrantAudit
		var targets []byte
		err = rows.Scan(
			&item.ID, &item.TenantID, &item.TaskID, &item.SourceDecisionID, &item.ActorPrincipalID,
			&targets, &item.TargetsDigest, &item.Status, &item.Version, &item.RevokeIdempotencyKey,
			&item.RevokeRequestDigest, &item.ExpiresAt, &item.CreatedAt, &item.UpdatedAt, &item.RevokedAt,
			&item.ActorUserID, &item.SourceOperationID,
		)
		if err != nil {
			return nil, err
		}
		if err = json.Unmarshal(targets, &item.Targets); err != nil {
			return nil, err
		}
		out = append(out, item)
		grantIDs = append(grantIDs, item.ID)
		sourceOperationIDs = append(sourceOperationIDs, item.SourceOperationID)
	}
	if err = rows.Err(); err != nil || len(out) == 0 {
		return out, err
	}
	rows.Close()

	operationRows, err := tx.Query(ctx, grantAuditOperationSelect, tenant, task, grantIDs, sourceOperationIDs)
	if err != nil {
		return nil, err
	}
	defer operationRows.Close()
	byGrant := make(map[int64]*domain.GrantOperationAudit, len(out))
	sourceGrant := make(map[int64]int64, len(out))
	for _, item := range out {
		sourceGrant[item.SourceOperationID] = item.ID
	}
	for operationRows.Next() {
		var operationID, grantID int64
		var purpose string
		var audit domain.GrantOperationAudit
		if err = operationRows.Scan(&operationID, &grantID, &purpose, &audit.Status, &audit.ResultSummary, &audit.ResultDeliveryStatus, &audit.ExitCode, &audit.AgentContinued, &audit.FinishedAt); err != nil {
			return nil, err
		}
		if sourceID, isSource := sourceGrant[operationID]; isSource {
			grantID = sourceID
			for index := range out {
				if out[index].ID == grantID {
					out[index].Purpose = purpose
					break
				}
			}
		}
		if _, exists := byGrant[grantID]; !exists {
			copy := audit
			byGrant[grantID] = &copy
		}
	}
	if err = operationRows.Err(); err != nil {
		return nil, err
	}
	for index := range out {
		out[index].LastOperation = byGrant[out[index].ID]
	}
	return out, nil
}

func (s *Store) GetGrant(ctx context.Context, tx pgx.Tx, tenant, task, id int64, lock bool) (domain.Grant, error) {
	query := grantSelect + ` WHERE tenant_id=$1 AND task_id=$2 AND id=$3`
	if lock {
		query += ` FOR UPDATE`
	}
	g, err := scanGrant(tx.QueryRow(ctx, query, tenant, task, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return g, domain.ErrNotFound
	}
	return g, err
}

func (s *Store) InsertGrant(ctx context.Context, tx pgx.Tx, g domain.Grant) error {
	targets, e := json.Marshal(g.Targets)
	if e != nil {
		return e
	}
	_, e = tx.Exec(ctx, `INSERT INTO workforce_task_network_grants(id,tenant_id,task_id,source_decision_id,actor_principal_id,targets_json,targets_digest,status,version,expires_at,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12)`, g.ID, g.TenantID, g.TaskID, g.SourceDecisionID, g.ActorPrincipalID, targets, g.TargetsDigest, g.Status, g.Version, g.ExpiresAt, g.CreatedAt, g.UpdatedAt)
	return e
}

func (s *Store) UpdateGrant(ctx context.Context, tx pgx.Tx, g domain.Grant) error {
	tag, e := tx.Exec(ctx, `UPDATE workforce_task_network_grants SET status=$2,version=$3,revoke_idempotency_key=NULLIF($4,''),revoke_request_digest=NULLIF($5,''),updated_at=$6,revoked_at=$7 WHERE id=$1 AND tenant_id=$8 AND task_id=$9 AND version=$10`, g.ID, g.Status, g.Version, g.RevokeIdempotencyKey, g.RevokeRequestDigest, g.UpdatedAt, g.RevokedAt, g.TenantID, g.TaskID, g.Version-1)
	if e == nil && tag.RowsAffected() != 1 {
		return domain.ErrStale
	}
	return e
}

func (s *Store) InsertAttempt(ctx context.Context, tx pgx.Tx, a domain.Attempt) error {
	_, e := tx.Exec(ctx, `INSERT INTO workforce_execution_operation_attempts(id,tenant_id,task_id,operation_id,attempt_number,worker_id,worker_instance_id,host_boot_id,owner_epoch,lease_digest,lease_expires_at,status,container_id,relay_identity,result_digest,exit_code,created_at,updated_at,started_at,finished_at) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20)`, a.ID, a.TenantID, a.TaskID, a.OperationID, a.AttemptNumber, a.WorkerID, a.WorkerInstanceID, a.HostBootID, a.OwnerEpoch, a.LeaseDigest, a.LeaseExpiresAt, a.Status, a.ContainerID, a.RelayIdentity, a.ResultDigest, a.ExitCode, a.CreatedAt, a.UpdatedAt, a.StartedAt, a.FinishedAt)
	return e
}

func (s *Store) AttemptForOperation(ctx context.Context, tx pgx.Tx, operation int64, lock bool) (*domain.Attempt, error) {
	query := attemptSelect + ` WHERE operation_id=$1 ORDER BY attempt_number DESC LIMIT 1`
	if lock {
		query += ` FOR UPDATE`
	}
	a, e := scanAttempt(tx.QueryRow(ctx, query, operation))
	if errors.Is(e, pgx.ErrNoRows) {
		return nil, nil
	}
	return &a, e
}
func (s *Store) GetAttempt(ctx context.Context, tx pgx.Tx, operation, id int64, lock bool) (domain.Attempt, error) {
	query := attemptSelect + ` WHERE operation_id=$1 AND id=$2`
	if lock {
		query += ` FOR UPDATE`
	}
	a, e := scanAttempt(tx.QueryRow(ctx, query, operation, id))
	if errors.Is(e, pgx.ErrNoRows) {
		return a, domain.ErrNotFound
	}
	return a, e
}
func (s *Store) UpdateAttempt(ctx context.Context, tx pgx.Tx, a domain.Attempt) error {
	tag, e := tx.Exec(ctx, `UPDATE workforce_execution_operation_attempts SET status=$2,container_id=$3,relay_identity=$4,result_digest=$5,exit_code=$6,lease_expires_at=$7,updated_at=$8,started_at=$9,finished_at=$10 WHERE id=$1`, a.ID, a.Status, a.ContainerID, a.RelayIdentity, a.ResultDigest, a.ExitCode, a.LeaseExpiresAt, a.UpdatedAt, a.StartedAt, a.FinishedAt)
	if e == nil && tag.RowsAffected() != 1 {
		return domain.ErrStale
	}
	return e
}

func (s *Store) PendingResults(ctx context.Context, tx pgx.Tx, tenant, task, attempt, generation int64, limit int) ([]domain.Operation, error) {
	rows, e := tx.Query(ctx, operationSelect+` WHERE tenant_id=$1 AND task_id=$2 AND attempt_id=$3 AND adapter_session_generation=$4 AND result_delivery_status='SAVED' ORDER BY id LIMIT $5`, tenant, task, attempt, generation, limit)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []domain.Operation
	for rows.Next() {
		o, e := scanOperation(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

func (s *Store) CancellableOperations(ctx context.Context, tx pgx.Tx, tenant, task int64) ([]domain.Operation, error) {
	rows, e := tx.Query(ctx, operationSelect+` WHERE tenant_id=$1 AND task_id=$2 AND status IN ('WAITING_APPROVAL','READY','DISPATCHED','RUNNING') ORDER BY id FOR UPDATE`, tenant, task)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []domain.Operation
	for rows.Next() {
		o, e := scanOperation(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

func (s *Store) TaskGrants(ctx context.Context, tx pgx.Tx, tenant, task int64) ([]domain.Grant, error) {
	rows, e := tx.Query(ctx, grantSelect+` WHERE tenant_id=$1 AND task_id=$2 ORDER BY id FOR UPDATE`, tenant, task)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []domain.Grant
	for rows.Next() {
		g, e := scanGrant(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (s *Store) CancelActiveAttempts(ctx context.Context, tx pgx.Tx, tenant, task int64, now time.Time) error {
	_, e := tx.Exec(ctx, `UPDATE workforce_execution_operation_attempts SET status='OUTCOME_UNKNOWN',updated_at=$3,finished_at=COALESCE(finished_at,$3) WHERE tenant_id=$1 AND task_id=$2 AND status IN ('DISPATCHED','RUNNING')`, tenant, task, now)
	return e
}
func (s *Store) ExpiredOperations(ctx context.Context, tx pgx.Tx, limit int, now time.Time) ([]domain.Operation, error) {
	rows, e := tx.Query(ctx, operationSelect+` WHERE status IN ('WAITING_APPROVAL','READY') AND expires_at<=$1 ORDER BY expires_at,id LIMIT $2 FOR UPDATE SKIP LOCKED`, now, limit)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []domain.Operation
	for rows.Next() {
		o, e := scanOperation(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

func (s *Store) ExpiredClaimOperations(ctx context.Context, tx pgx.Tx, limit int, now time.Time) ([]domain.Operation, error) {
	rows, e := tx.Query(ctx, expiredClaimOperationQuery, now, limit)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var ids []int64
	for rows.Next() {
		var id int64
		if e = rows.Scan(&id); e != nil {
			return nil, e
		}
		ids = append(ids, id)
	}
	if e = rows.Err(); e != nil {
		return nil, e
	}
	out := make([]domain.Operation, 0, len(ids))
	for _, id := range ids {
		o, err := scanOperation(tx.QueryRow(ctx, operationSelect+` WHERE id=$1`, id))
		if err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, nil
}

const expiredClaimOperationQuery = `SELECT o.id FROM workforce_execution_operations o JOIN workforce_execution_operation_attempts a ON a.operation_id=o.id WHERE o.status IN ('DISPATCHED','RUNNING') AND a.status IN ('DISPATCHED','RUNNING') AND a.lease_expires_at<=$1 ORDER BY a.lease_expires_at,o.id LIMIT $2 FOR UPDATE OF o,a SKIP LOCKED`

func (s *Store) ExpiredGrants(ctx context.Context, tx pgx.Tx, limit int, now time.Time) ([]domain.Grant, error) {
	rows, e := tx.Query(ctx, grantSelect+` WHERE status='ACTIVE' AND expires_at<=$1 ORDER BY expires_at,id LIMIT $2 FOR UPDATE SKIP LOCKED`, now, limit)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []domain.Grant
	for rows.Next() {
		g, e := scanGrant(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (s *Store) OperationsForGrant(ctx context.Context, tx pgx.Tx, tenant, task, grant int64) ([]domain.Operation, error) {
	rows, e := tx.Query(ctx, operationSelect+` WHERE tenant_id=$1 AND task_id=$2 AND authorization_grant_id=$3 AND status IN ('READY','DISPATCHED','RUNNING') ORDER BY id FOR UPDATE`, tenant, task, grant)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	var out []domain.Operation
	for rows.Next() {
		o, e := scanOperation(rows)
		if e != nil {
			return nil, e
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

func (s *Store) CancelOperationAttempts(ctx context.Context, tx pgx.Tx, operation int64, now time.Time) error {
	_, e := tx.Exec(ctx, `UPDATE workforce_execution_operation_attempts SET status='OUTCOME_UNKNOWN',updated_at=$2,finished_at=COALESCE(finished_at,$2) WHERE operation_id=$1 AND status IN ('DISPATCHED','RUNNING')`, operation, now)
	return e
}

const operationSelect = `SELECT id,tenant_id,task_id,task_turn_id,attempt_id,adapter_session_generation,COALESCE(authorization_grant_id,0),local_tool_call_key,native_tool_call_id,round_key,operation_kind,encode(descriptor_ciphertext,'base64'),encode(descriptor_edek,'base64'),descriptor_key_ref,descriptor_digest,purpose,targets_json,status,version,result_delivery_status,agent_continued,result_digest,result_summary,exit_code,expires_at,created_at,updated_at,started_at,finished_at,COALESCE(encode(result_output_ciphertext,'base64'),''),COALESCE(encode(result_output_edek,'base64'),''),COALESCE(result_output_key_ref,'') FROM workforce_execution_operations`

const nextReadyOperationClause = ` WHERE tenant_id=$1 AND task_id=$2 AND attempt_id=$3 AND adapter_session_generation=$4 AND status='READY' AND expires_at>CURRENT_TIMESTAMP ORDER BY id LIMIT 1 FOR UPDATE SKIP LOCKED`

func scanOperation(row rowScanner) (domain.Operation, error) {
	var o domain.Operation
	var targets []byte
	e := row.Scan(&o.ID, &o.TenantID, &o.TaskID, &o.TaskTurnID, &o.AttemptID, &o.AdapterSessionGeneration, &o.AuthorizationGrantID, &o.LocalToolCallKey, &o.NativeToolCallID, &o.RoundKey, &o.Kind, &o.Descriptor.Ciphertext, &o.Descriptor.EDEK, &o.Descriptor.KeyRef, &o.DescriptorDigest, &o.Purpose, &targets, &o.Status, &o.Version, &o.ResultDeliveryStatus, &o.AgentContinued, &o.ResultDigest, &o.ResultSummary, &o.ExitCode, &o.ExpiresAt, &o.CreatedAt, &o.UpdatedAt, &o.StartedAt, &o.FinishedAt, &o.ResultOutput.Ciphertext, &o.ResultOutput.EDEK, &o.ResultOutput.KeyRef)
	if e == nil {
		e = json.Unmarshal(targets, &o.Targets)
	}
	return o, e
}
func scanOptionalOperation(row pgx.Row) (*domain.Operation, error) {
	o, e := scanOperation(row)
	if errors.Is(e, pgx.ErrNoRows) {
		return nil, nil
	}
	return &o, e
}

const grantSelect = `SELECT id,tenant_id,task_id,source_decision_id,actor_principal_id,targets_json,targets_digest,status,version,COALESCE(revoke_idempotency_key,''),COALESCE(revoke_request_digest,''),expires_at,created_at,updated_at,revoked_at FROM workforce_task_network_grants`
const grantAuditSelect = `SELECT g.id,g.tenant_id,g.task_id,g.source_decision_id,g.actor_principal_id,g.targets_json,g.targets_digest,g.status,g.version,COALESCE(g.revoke_idempotency_key,''),COALESCE(g.revoke_request_digest,''),g.expires_at,g.created_at,g.updated_at,g.revoked_at,p.user_id,d.operation_id
FROM workforce_task_network_grants g
JOIN workforce_principals p ON p.id=g.actor_principal_id AND p.principal_type='USER'
JOIN workforce_execution_operation_decisions d ON d.id=g.source_decision_id AND d.tenant_id=g.tenant_id AND d.task_id=g.task_id
WHERE g.tenant_id=$1 AND g.task_id=$2 ORDER BY g.id DESC`

const grantAuditOperationSelect = `SELECT o.id,COALESCE(o.authorization_grant_id,0),o.purpose,o.status,o.result_summary,o.result_delivery_status,o.exit_code,o.agent_continued,o.finished_at
FROM workforce_execution_operations o
WHERE o.tenant_id=$1 AND o.task_id=$2 AND (o.authorization_grant_id=ANY($3::BIGINT[]) OR o.id=ANY($4::BIGINT[]))
ORDER BY o.created_at DESC,o.id DESC`

type rowScanner interface{ Scan(...any) error }

func scanGrant(row rowScanner) (domain.Grant, error) {
	var g domain.Grant
	var targets []byte
	e := row.Scan(&g.ID, &g.TenantID, &g.TaskID, &g.SourceDecisionID, &g.ActorPrincipalID, &targets, &g.TargetsDigest, &g.Status, &g.Version, &g.RevokeIdempotencyKey, &g.RevokeRequestDigest, &g.ExpiresAt, &g.CreatedAt, &g.UpdatedAt, &g.RevokedAt)
	if e == nil {
		e = json.Unmarshal(targets, &g.Targets)
	}
	return g, e
}

const attemptSelect = `SELECT id,tenant_id,task_id,operation_id,attempt_number,worker_id,worker_instance_id,host_boot_id,owner_epoch,lease_digest,lease_expires_at,status,container_id,relay_identity,result_digest,exit_code,created_at,updated_at,started_at,finished_at FROM workforce_execution_operation_attempts`

func scanAttempt(row rowScanner) (domain.Attempt, error) {
	var a domain.Attempt
	e := row.Scan(&a.ID, &a.TenantID, &a.TaskID, &a.OperationID, &a.AttemptNumber, &a.WorkerID, &a.WorkerInstanceID, &a.HostBootID, &a.OwnerEpoch, &a.LeaseDigest, &a.LeaseExpiresAt, &a.Status, &a.ContainerID, &a.RelayIdentity, &a.ResultDigest, &a.ExitCode, &a.CreatedAt, &a.UpdatedAt, &a.StartedAt, &a.FinishedAt)
	return a, e
}
