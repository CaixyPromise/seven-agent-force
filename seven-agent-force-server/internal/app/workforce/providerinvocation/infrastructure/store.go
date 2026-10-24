package infrastructure

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"time"

	invocationdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/providerinvocation/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct{ db *pgxpool.Pool }

func NewStore(db *pgxpool.Pool) *Store { return &Store{db: db} }

func (s *Store) Reserve(ctx context.Context, invocationID int64, identity invocationdomain.RequestIdentity, operation invocationdomain.Operation, body []byte, now time.Time) (invocationdomain.Reservation, error) {
	if err := identity.Validate(); err != nil {
		return invocationdomain.Reservation{}, err
	}
	if invocationID <= 0 {
		return invocationdomain.Reservation{}, invocationError("MODEL_INVOCATION_ID_INVALID", 500, "model invocation identity is invalid", nil)
	}
	requestedModel, err := requestModel(body)
	if err != nil {
		return invocationdomain.Reservation{}, err
	}
	if identity.ExecutionKind.Normalized() == invocationdomain.ExecutionKindReviewRun {
		return s.reserveReview(ctx, invocationID, identity, operation, requestedModel, now)
	}
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return invocationdomain.Reservation{}, err
	}
	defer tx.Rollback(ctx)
	var tenantID, taskID, workerID, generation, ownerEpoch, assignmentID, credentialID, credentialVersion int64
	var instanceID, hostBootID, leaseHash, taskStatus, attemptStatus, runtimeStatus, protocol, modelProtocol, primaryModel string
	var leaseExpires time.Time
	// Native interactions and lease renewal lock Task before Attempt/Runtime.
	// Acquire that parent explicitly before the joined authority read; SQL's
	// row-mark order must not hold Attempt while waiting for an interaction.
	err = tx.QueryRow(ctx, `SELECT t.id FROM workforce_tasks t JOIN workforce_task_attempts a ON a.task_id=t.id WHERE a.id=$1 FOR UPDATE OF t`, identity.AttemptID).Scan(&taskID)
	if errors.Is(err, pgx.ErrNoRows) {
		return invocationdomain.Reservation{}, invocationError("ATTEMPT_NOT_FOUND", 404, "Attempt does not exist", err)
	}
	if err != nil {
		return invocationdomain.Reservation{}, err
	}
	err = tx.QueryRow(ctx, `SELECT t.tenant_id,t.id,a.worker_id,a.worker_instance_id,a.host_boot_id,a.lease_token,t.status,a.status,r.status,a.generation,a.owner_epoch,a.lease_expires_at,a.assignment_id,t.model_credential_id,t.primary_model
FROM workforce_task_attempts a JOIN workforce_tasks t ON t.id=a.task_id JOIN workforce_attempt_runtimes r ON r.attempt_id=a.id
WHERE a.id=$1 FOR UPDATE OF a,r,t`, identity.AttemptID).Scan(&tenantID, &taskID, &workerID, &instanceID, &hostBootID, &leaseHash, &taskStatus, &attemptStatus, &runtimeStatus, &generation, &ownerEpoch, &leaseExpires, &assignmentID, &credentialID, &primaryModel)
	if errors.Is(err, pgx.ErrNoRows) {
		return invocationdomain.Reservation{}, invocationError("ATTEMPT_NOT_FOUND", 404, "Attempt does not exist", err)
	}
	if err != nil {
		return invocationdomain.Reservation{}, err
	}
	if err = tx.QueryRow(ctx, `SELECT model_protocol FROM workforce_task_assignments WHERE id=$1 AND task_id=$2`, assignmentID, taskID).Scan(&modelProtocol); err != nil {
		return invocationdomain.Reservation{}, err
	}
	if err = tx.QueryRow(ctx, `SELECT secret_version,protocol FROM workforce_model_credentials WHERE id=$1 AND tenant_id=$2 AND status='ACTIVE'`, credentialID, tenantID).Scan(&credentialVersion, &protocol); errors.Is(err, pgx.ErrNoRows) {
		return invocationdomain.Reservation{}, invocationError("MODEL_CREDENTIAL_DISABLED", 409, "Model credential is not available", err)
	} else if err != nil {
		return invocationdomain.Reservation{}, err
	}
	if protocol != modelProtocol {
		return invocationdomain.Reservation{}, invocationError("MODEL_CREDENTIAL_PROTOCOL_IN_USE", 409, "Model credential protocol no longer matches the active session", nil)
	}
	if taskID != identity.TaskID || workerID != identity.WorkerID || instanceID != identity.InstanceID || hostBootID != identity.HostBootID || generation != identity.Generation || ownerEpoch != identity.OwnerEpoch || leaseHash != hashToken(identity.LeaseToken) || !leaseExpires.After(now) {
		return invocationdomain.Reservation{}, invocationError("ATTEMPT_IDENTITY_CONFLICT", 409, "Attempt execution authority is invalid", nil)
	}
	if taskStatus != "RUNNING" || attemptStatus != "RUNNING" || runtimeStatus != "RUNNING" {
		return invocationdomain.Reservation{}, invocationError("ATTEMPT_NOT_RUNNING", 409, "Attempt is not accepting model invocations", nil)
	}
	// Agent execution time is charged by the Worker's resumable budget. A
	// wall-clock deadline here would incorrectly count time waiting for a user.
	if err = ensureUnique(ctx, tx, tenantID, identity); err != nil {
		return invocationdomain.Reservation{}, err
	}
	bodyDigest, _ := hex.DecodeString(identity.BodySHA256)
	_, err = tx.Exec(ctx, `INSERT INTO workforce_model_invocations(id,tenant_id,task_id,protocol_invocation_key,execution_kind,attempt_id,review_run_id,review_run_attempt_id,generation,owner_epoch,worker_id,worker_instance_id,host_boot_id,model_credential_id,credential_version,protocol,primary_model,requested_model,response_model,operation,is_primary_model,request_body_sha256,nonce_sha256,status,started_at) VALUES($1,$2,$3,$4,'AGENT_ATTEMPT',$5,NULL,NULL,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,NULL,$16,$17,$18,$19,'AUTHORIZED',$20)`, invocationID, tenantID, taskID, identity.ProtocolInvocationKey, identity.AttemptID, generation, ownerEpoch, workerID, instanceID, hostBootID, credentialID, credentialVersion, protocol, primaryModel, requestedModel, operation, requestedModel == primaryModel, bodyDigest, digestToken(identity.Nonce), now)
	if isUniqueViolation(err) {
		return invocationdomain.Reservation{}, invocationError("INVOCATION_IDENTITY_CONFLICT", 409, "Invocation identity has already been used", err)
	}
	if err != nil {
		return invocationdomain.Reservation{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return invocationdomain.Reservation{}, err
	}
	return invocationdomain.Reservation{ID: invocationID, ProtocolInvocationKey: identity.ProtocolInvocationKey, ExecutionKind: invocationdomain.ExecutionKindAgentAttempt, TaskID: taskID, AttemptID: identity.AttemptID, OwnerEpoch: ownerEpoch, RequestedModel: requestedModel, StartedAt: now, Frozen: invocationdomain.FrozenModel{TenantID: tenantID, CredentialID: credentialID, CredentialVersion: credentialVersion, Protocol: protocol, PrimaryModel: primaryModel}}, nil
}

func (s *Store) reserveReview(ctx context.Context, invocationID int64, identity invocationdomain.RequestIdentity, operation invocationdomain.Operation, requestedModel string, now time.Time) (invocationdomain.Reservation, error) {
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return invocationdomain.Reservation{}, err
	}
	defer tx.Rollback(ctx)
	var tenantID, taskID, workerID, ownerEpoch, credentialID, credentialVersion int64
	var instanceID, hostBootID, leaseHash, status, attemptStatus, protocol, primaryModel string
	var leaseExpires, deadline *time.Time
	err = tx.QueryRow(ctx, `SELECT r.tenant_id,r.task_id,a.worker_id,a.worker_instance_id,a.host_boot_id,a.owner_epoch,a.lease_token_sha256,a.lease_expires_at,r.status,a.status,r.model_credential_id,c.secret_version,c.protocol,r.primary_model,r.execution_deadline_at
FROM workforce_review_runs r JOIN workforce_review_run_attempts a ON a.review_run_id=r.id
JOIN workforce_model_credentials c ON c.id=r.model_credential_id AND c.tenant_id=r.tenant_id AND c.status='ACTIVE' AND c.protocol=r.model_protocol
WHERE r.id=$1 AND a.id=$2 FOR UPDATE OF r,a`, identity.ReviewRunID, identity.ReviewRunAttemptID).Scan(&tenantID, &taskID, &workerID, &instanceID, &hostBootID, &ownerEpoch, &leaseHash, &leaseExpires, &status, &attemptStatus, &credentialID, &credentialVersion, &protocol, &primaryModel, &deadline)
	if errors.Is(err, pgx.ErrNoRows) {
		return invocationdomain.Reservation{}, invocationError("REVIEW_RUN_NOT_FOUND", 404, "ReviewRun does not exist", err)
	}
	if err != nil {
		return invocationdomain.Reservation{}, err
	}
	if taskID != identity.TaskID || workerID != identity.WorkerID || instanceID != identity.InstanceID || hostBootID != identity.HostBootID || ownerEpoch != identity.OwnerEpoch || leaseHash != hashToken(identity.LeaseToken) || leaseExpires == nil || !leaseExpires.After(now) {
		return invocationdomain.Reservation{}, invocationError("REVIEW_RUN_OWNER_STALE", 409, "ReviewRun authority is stale", nil)
	}
	if status != "RUNNING" || attemptStatus != "RUNNING" || deadline == nil || !deadline.After(now) {
		return invocationdomain.Reservation{}, invocationError("REVIEW_RUN_NOT_RUNNING", 409, "ReviewRun is not accepting model invocations", nil)
	}
	if err = ensureUnique(ctx, tx, tenantID, identity); err != nil {
		return invocationdomain.Reservation{}, err
	}
	bodyDigest, _ := hex.DecodeString(identity.BodySHA256)
	_, err = tx.Exec(ctx, `INSERT INTO workforce_model_invocations(id,tenant_id,task_id,protocol_invocation_key,execution_kind,attempt_id,review_run_id,review_run_attempt_id,generation,owner_epoch,worker_id,worker_instance_id,host_boot_id,model_credential_id,credential_version,protocol,primary_model,requested_model,response_model,operation,is_primary_model,request_body_sha256,nonce_sha256,status,started_at) VALUES($1,$2,$3,$4,'REVIEW_RUN',NULL,$5,$6,0,$7,$8,$9,$10,$11,$12,$13,$14,$15,NULL,$16,$17,$18,$19,'AUTHORIZED',$20)`, invocationID, tenantID, taskID, identity.ProtocolInvocationKey, identity.ReviewRunID, identity.ReviewRunAttemptID, ownerEpoch, workerID, instanceID, hostBootID, credentialID, credentialVersion, protocol, primaryModel, requestedModel, operation, requestedModel == primaryModel, bodyDigest, digestToken(identity.Nonce), now)
	if err != nil {
		return invocationdomain.Reservation{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return invocationdomain.Reservation{}, err
	}
	return invocationdomain.Reservation{ID: invocationID, ProtocolInvocationKey: identity.ProtocolInvocationKey, ExecutionKind: invocationdomain.ExecutionKindReviewRun, TaskID: taskID, ReviewRunID: identity.ReviewRunID, ReviewRunAttemptID: identity.ReviewRunAttemptID, OwnerEpoch: ownerEpoch, RequestedModel: requestedModel, StartedAt: now, Frozen: invocationdomain.FrozenModel{TenantID: tenantID, CredentialID: credentialID, CredentialVersion: credentialVersion, Protocol: protocol, PrimaryModel: primaryModel, ExecutionDeadline: deadline.UTC()}}, nil
}

func ensureUnique(ctx context.Context, tx pgx.Tx, tenantID int64, identity invocationdomain.RequestIdentity) error {
	var body, nonce []byte
	var status string
	err := tx.QueryRow(ctx, `SELECT request_body_sha256,nonce_sha256,status FROM workforce_model_invocations WHERE tenant_id=$1 AND protocol_invocation_key=$2`, tenantID, identity.ProtocolInvocationKey).Scan(&body, &nonce, &status)
	if err == nil {
		expectedBody, _ := hex.DecodeString(identity.BodySHA256)
		if !bytes.Equal(body, expectedBody) || !bytes.Equal(nonce, digestToken(identity.Nonce)) {
			return invocationError("INVOCATION_IDENTITY_CONFLICT", 409, "Invocation identity was reused with another payload", nil)
		}
		return invocationError("INVOCATION_REPLAY_NOT_AVAILABLE", 409, "Model invocation cannot be replayed", nil)
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	return nil
}
func requestModel(body []byte) (string, error) {
	var value struct {
		Model string `json:"model"`
	}
	if json.Unmarshal(body, &value) != nil {
		return "", invocationError("MODEL_REQUEST_INVALID", 400, "model request must be valid JSON", nil)
	}
	value.Model = strings.TrimSpace(value.Model)
	if value.Model == "" || len(value.Model) > 256 {
		return "", invocationError("MODEL_REQUEST_MODEL_INVALID", 400, "model request model is invalid", nil)
	}
	return value.Model, nil
}

func (s *Store) SetCredentialVersion(ctx context.Context, id int64, version int64) error {
	if version <= 0 {
		return invocationError("MODEL_CREDENTIAL_VERSION_INVALID", 409, "model credential version is invalid", nil)
	}
	result, err := s.db.Exec(ctx, `UPDATE workforce_model_invocations SET credential_version=$2 WHERE id=$1 AND status='AUTHORIZED'`, id, version)
	if err == nil && result.RowsAffected() != 1 {
		return invocationError("MODEL_INVOCATION_NOT_ACTIVE", 409, "model invocation is not active", nil)
	}
	return err
}

func (s *Store) MarkStreaming(ctx context.Context, id int64, statusCode int, requestID string, at time.Time) error {
	result, err := s.db.Exec(ctx, `UPDATE workforce_model_invocations SET status='STREAMING',upstream_status_code=$2,upstream_request_id=NULLIF($3,''),headers_at=$4 WHERE id=$1 AND status='AUTHORIZED'`, id, statusCode, safe(requestID), at)
	if err == nil && result.RowsAffected() != 1 {
		return invocationError("INVOCATION_STATE_CONFLICT", 409, "Invocation is no longer authorized", nil)
	}
	return err
}
func (s *Store) Finish(ctx context.Context, f invocationdomain.Finish) error {
	usage := nullableJSON(f.Usage.Raw)
	_, err := s.db.Exec(ctx, `UPDATE workforce_model_invocations SET status=$2,upstream_status_code=COALESCE($3,upstream_status_code),upstream_request_id=COALESCE(NULLIF($4,''),upstream_request_id),first_byte_at=COALESCE($5,first_byte_at),finished_at=$6,duration_milliseconds=GREATEST(0,(EXTRACT(EPOCH FROM($6-started_at))*1000)::INTEGER),response_model=COALESCE(NULLIF($7,''),response_model),input_tokens=$8,cached_input_tokens=$9,cache_creation_input_tokens=$10,output_tokens=$11,reasoning_output_tokens=$12,total_tokens=$13,usage_json=$14,error_code=NULLIF($15,'') WHERE id=$1 AND status IN('AUTHORIZED','STREAMING')`, f.ID, f.Status, f.StatusCode, safe(f.UpstreamRequestID), f.FirstByteAt, f.FinishedAt, f.ResponseModel, nullable(f.Usage.InputTokens), nullable(f.Usage.CachedInputTokens), nullable(f.Usage.CacheCreationInputTokens), nullable(f.Usage.OutputTokens), nullable(f.Usage.ReasoningOutputTokens), nullable(f.Usage.TotalTokens), usage, f.ErrorCode)
	return err
}
func nullable(v *int64) any {
	if v == nil {
		return nil
	}
	return *v
}
func nullableJSON(v json.RawMessage) any {
	if len(v) == 0 {
		return nil
	}
	return v
}

func (s *Store) CancelActive(ctx context.Context, tenantID, attemptID int64, reason string, at time.Time) error {
	_, err := s.db.Exec(ctx, `UPDATE workforce_model_invocations SET status='CANCELLED',finished_at=$4,duration_milliseconds=GREATEST(0,(EXTRACT(EPOCH FROM($4-started_at))*1000)::INTEGER),error_code=$3 WHERE tenant_id=$1 AND attempt_id=$2 AND status IN('AUTHORIZED','STREAMING')`, tenantID, attemptID, reason, at)
	return err
}
func (s *Store) CancelReviewRunActive(ctx context.Context, tenantID, runID, attemptID int64, reason string, at time.Time) error {
	_, err := s.db.Exec(ctx, `UPDATE workforce_model_invocations SET status='CANCELLED',finished_at=$5,error_code=$4 WHERE tenant_id=$1 AND review_run_id=$2 AND review_run_attempt_id=$3 AND status IN('AUTHORIZED','STREAMING')`, tenantID, runID, attemptID, reason, at)
	return err
}
func (s *Store) CancelReviewRunActiveForTask(ctx context.Context, tenantID, taskID int64, reason string, at time.Time) error {
	_, err := s.db.Exec(ctx, `UPDATE workforce_model_invocations SET status='CANCELLED',finished_at=$3,error_code=$4 WHERE tenant_id=$1 AND task_id=$2 AND execution_kind='REVIEW_RUN' AND status IN('AUTHORIZED','STREAMING')`, tenantID, taskID, at, reason)
	return err
}
func (s *Store) CancelCredentialActive(ctx context.Context, tenantID, credentialID int64, reason string, at time.Time) error {
	_, err := s.db.Exec(ctx, `UPDATE workforce_model_invocations SET status='CANCELLED',finished_at=$4,error_code=$3 WHERE tenant_id=$1 AND model_credential_id=$2 AND status IN('AUTHORIZED','STREAMING')`, tenantID, credentialID, reason, at)
	return err
}

func (s *Store) Get(ctx context.Context, tenantID, id int64) (invocationdomain.View, error) {
	return scanView(s.db.QueryRow(ctx, viewSQL+` WHERE tenant_id=$1 AND id=$2`, tenantID, id))
}
func (s *Store) ListAttempt(ctx context.Context, tenantID, attemptID int64, limit int) ([]invocationdomain.View, error) {
	rows, err := s.db.Query(ctx, viewSQL+` WHERE tenant_id=$1 AND attempt_id=$2 ORDER BY started_at DESC LIMIT $3`, tenantID, attemptID, limit)
	return scanViews(rows, err)
}
func (s *Store) ListReviewRun(ctx context.Context, tenantID, runID, attemptID int64, limit int) ([]invocationdomain.View, error) {
	rows, err := s.db.Query(ctx, viewSQL+` WHERE tenant_id=$1 AND review_run_id=$2 AND review_run_attempt_id=$3 ORDER BY started_at DESC LIMIT $4`, tenantID, runID, attemptID, limit)
	return scanViews(rows, err)
}
func (s *Store) ListTask(ctx context.Context, tenantID, taskID int64, limit int) ([]invocationdomain.View, error) {
	rows, err := s.db.Query(ctx, viewSQL+` WHERE tenant_id=$1 AND task_id=$2 ORDER BY started_at DESC LIMIT $3`, tenantID, taskID, limit)
	return scanViews(rows, err)
}
func (s *Store) ListCredential(ctx context.Context, tenantID, credentialID int64, limit int) ([]invocationdomain.View, error) {
	rows, err := s.db.Query(ctx, viewSQL+` WHERE tenant_id=$1 AND model_credential_id=$2 ORDER BY started_at DESC LIMIT $3`, tenantID, credentialID, limit)
	return scanViews(rows, err)
}

func (s *Store) GetAnalytics(ctx context.Context, tenantID int64, days int, now time.Time) (invocationdomain.Analytics, error) {
	day := time.Date(now.UTC().Year(), now.UTC().Month(), now.UTC().Day(), 0, 0, 0, 0, time.UTC)
	periodStart := day.AddDate(0, 0, -(days - 1))
	periodEnd := day.AddDate(0, 0, 1)
	previousStart := periodStart.AddDate(0, 0, -days)
	monthStart := time.Date(day.Year(), day.Month(), 1, 0, 0, 0, 0, time.UTC)
	result := invocationdomain.Analytics{Days: days, Timeline: []invocationdomain.AnalyticsPoint{}, ModelDistribution: []invocationdomain.ModelDistribution{}}

	err := s.db.QueryRow(ctx, `SELECT
  COUNT(*) FILTER (WHERE status='ACTIVE'),
  COALESCE(SUM(jsonb_array_length(models_json)) FILTER (WHERE status='ACTIVE'),0),
  (SELECT COUNT(*) FROM workforce_model_invocations WHERE tenant_id=$1 AND started_at >= $2 AND started_at < $3),
  (SELECT COALESCE(SUM(total_tokens),0) FROM workforce_model_invocations WHERE tenant_id=$1 AND started_at >= $2 AND started_at < $3)
FROM workforce_model_credentials WHERE tenant_id=$1`, tenantID, monthStart, periodEnd).Scan(
		&result.Summary.ActiveCredentialCount, &result.Summary.ModelCount, &result.Summary.InvocationCount, &result.Summary.TotalTokens,
	)
	if err != nil {
		return invocationdomain.Analytics{}, err
	}

	rows, err := s.db.Query(ctx, `WITH days AS (
  SELECT generate_series($2::date, ($3::date - 1), interval '1 day')::date AS day
), usage AS (
  SELECT (started_at AT TIME ZONE 'UTC')::date AS day, COUNT(*) AS calls, COALESCE(SUM(total_tokens),0) AS tokens
  FROM workforce_model_invocations WHERE tenant_id=$1 AND started_at >= $2 AND started_at < $3 GROUP BY (started_at AT TIME ZONE 'UTC')::date
)
SELECT to_char(days.day,'YYYY-MM-DD'),COALESCE(usage.calls,0),COALESCE(usage.tokens,0)
FROM days LEFT JOIN usage USING(day) ORDER BY days.day`, tenantID, periodStart, periodEnd)
	if err != nil {
		return invocationdomain.Analytics{}, err
	}
	for rows.Next() {
		var point invocationdomain.AnalyticsPoint
		if err = rows.Scan(&point.Date, &point.InvocationCount, &point.TotalTokens); err != nil {
			rows.Close()
			return invocationdomain.Analytics{}, err
		}
		result.Timeline = append(result.Timeline, point)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return invocationdomain.Analytics{}, err
	}
	rows.Close()

	rows, err = s.db.Query(ctx, `SELECT requested_model,COUNT(*) FROM workforce_model_invocations
WHERE tenant_id=$1 AND started_at >= $2 AND started_at < $3
GROUP BY requested_model ORDER BY COUNT(*) DESC,requested_model LIMIT 8`, tenantID, periodStart, periodEnd)
	if err != nil {
		return invocationdomain.Analytics{}, err
	}
	for rows.Next() {
		var item invocationdomain.ModelDistribution
		if err = rows.Scan(&item.Model, &item.InvocationCount); err != nil {
			rows.Close()
			return invocationdomain.Analytics{}, err
		}
		result.ModelDistribution = append(result.ModelDistribution, item)
	}
	if err = rows.Err(); err != nil {
		rows.Close()
		return invocationdomain.Analytics{}, err
	}
	rows.Close()

	var currentCalls, previousCalls, currentTokens, previousTokens, inputTokens, cachedTokens int64
	var p95 *float64
	err = s.db.QueryRow(ctx, `SELECT
  COUNT(*) FILTER (WHERE started_at >= $2 AND started_at < $3),
  COUNT(*) FILTER (WHERE started_at >= $4 AND started_at < $2),
  COALESCE(SUM(total_tokens) FILTER (WHERE started_at >= $2 AND started_at < $3),0),
  COALESCE(SUM(total_tokens) FILTER (WHERE started_at >= $4 AND started_at < $2),0),
  COALESCE(SUM(input_tokens) FILTER (WHERE started_at >= $2 AND started_at < $3),0),
  COALESCE(SUM(cached_input_tokens) FILTER (WHERE started_at >= $2 AND started_at < $3),0),
  percentile_cont(0.95) WITHIN GROUP (ORDER BY duration_milliseconds) FILTER (WHERE started_at >= $2 AND started_at < $3 AND status='SUCCEEDED' AND duration_milliseconds IS NOT NULL)
FROM workforce_model_invocations WHERE tenant_id=$1 AND started_at >= $4 AND started_at < $3`, tenantID, periodStart, periodEnd, previousStart).Scan(
		&currentCalls, &previousCalls, &currentTokens, &previousTokens, &inputTokens, &cachedTokens, &p95,
	)
	if err != nil {
		return invocationdomain.Analytics{}, err
	}
	result.Comparison.InvocationPercent = percentChange(currentCalls, previousCalls)
	result.Comparison.TokenPercent = percentChange(currentTokens, previousTokens)
	if inputTokens > 0 {
		value := math.Round(float64(cachedTokens)/float64(inputTokens)*1000) / 10
		result.CacheHitRate = &value
	}
	if p95 != nil {
		value := int64(math.Round(*p95))
		result.P95DurationMillis = &value
	}
	return result, nil
}

func percentChange(current, previous int64) *float64 {
	if previous == 0 {
		return nil
	}
	value := math.Round((float64(current-previous)/float64(previous))*1000) / 10
	return &value
}

const viewSQL = `SELECT id,execution_kind,task_id,COALESCE((SELECT name FROM workforce_tasks WHERE workforce_tasks.id=workforce_model_invocations.task_id AND workforce_tasks.tenant_id=workforce_model_invocations.tenant_id),''),attempt_id,review_run_id,review_run_attempt_id,generation,owner_epoch,worker_id,worker_instance_id,host_boot_id,model_credential_id,credential_version,protocol,primary_model,requested_model,COALESCE(response_model,''),is_primary_model,operation,status,upstream_status_code,COALESCE(duration_milliseconds,0),CASE WHEN first_byte_at IS NULL THEN NULL ELSE GREATEST(0,(EXTRACT(EPOCH FROM(first_byte_at-started_at))*1000)::INTEGER) END,COALESCE(upstream_request_id,''),input_tokens,cached_input_tokens,cache_creation_input_tokens,output_tokens,reasoning_output_tokens,total_tokens,COALESCE(error_code,''),to_char(started_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),COALESCE(to_char(finished_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS.MS"Z"'),'') FROM workforce_model_invocations`

type scanner interface{ Scan(...any) error }

func scanView(row scanner) (invocationdomain.View, error) {
	var v invocationdomain.View
	var attempt, run, runAttempt *int64
	err := row.Scan(&v.ID, &v.ExecutionKind, &v.TaskID, &v.TaskName, &attempt, &run, &runAttempt, &v.Generation, &v.OwnerEpoch, &v.WorkerID, &v.WorkerInstanceID, &v.HostBootID, &v.ModelCredentialID, &v.CredentialVersion, &v.Protocol, &v.PrimaryModel, &v.RequestedModel, &v.ResponseModel, &v.IsPrimaryModel, &v.Operation, &v.Status, &v.StatusCode, &v.DurationMillis, &v.FirstByteMillis, &v.UpstreamRequestID, &v.Usage.InputTokens, &v.Usage.CachedInputTokens, &v.Usage.CacheCreationInputTokens, &v.Usage.OutputTokens, &v.Usage.ReasoningOutputTokens, &v.Usage.TotalTokens, &v.ErrorCode, &v.StartedAt, &v.FinishedAt)
	if attempt != nil {
		v.AttemptID = *attempt
	}
	if run != nil {
		v.ReviewRunID = *run
	}
	if runAttempt != nil {
		v.ReviewRunAttemptID = *runAttempt
	}
	return v, err
}
func scanViews(rows pgx.Rows, err error) ([]invocationdomain.View, error) {
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := []invocationdomain.View{}
	for rows.Next() {
		item, e := scanView(rows)
		if e != nil {
			return nil, e
		}
		result = append(result, item)
	}
	return result, rows.Err()
}

func hashToken(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
func digestToken(value string) []byte {
	sum := sha256.Sum256([]byte(value))
	return sum[:]
}
func safe(value string) string {
	value = strings.TrimSpace(value)
	if len(value) > 256 {
		value = value[:256]
	}
	for _, r := range value {
		if r < 32 || r == 127 {
			return ""
		}
	}
	return value
}
func invocationError(code string, status int, message string, cause error) error {
	return &invocationdomain.Error{ReasonCode: code, HTTPStatus: status, Message: message, Cause: cause}
}
func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
