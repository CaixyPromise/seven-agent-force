package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/openintegration/domain"
	secretvalue "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/crypto/secretvalue"
)

func (s *Store) ProjectTerminalEvents(ctx context.Context, limit int) (int, error) {
	if limit < 1 || limit > 32 {
		limit = 32
	}
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	type candidate struct {
		sourceID, sequence, tenantID, applicationID, taskID, turnID int64
		eventType, correlationID, taskStatus, webhookURL            string
		occurredAt                                                  time.Time
	}
	items := make([]candidate, 0, limit)
	rows, queryErr := tx.Query(ctx, `SELECT e.id,e.event_type,e.sequence,e.created_at,t.tenant_id,COALESCE(t.integration_application_id,0),t.id,COALESCE(t.current_task_turn_id,0),COALESCE(t.external_correlation_id,''),t.status,COALESCE(a.webhook_url,'')
FROM workforce_task_events e
JOIN workforce_tasks t ON t.id=e.task_id
LEFT JOIN workforce_integration_applications a ON a.id=t.integration_application_id AND a.tenant_id=t.tenant_id
WHERE e.open_integration_projection_status='PENDING'
  AND ((e.event_type='task.succeeded' AND t.status='SUCCEEDED')
    OR (e.event_type='task.failed' AND t.status='FAILED')
    OR (e.event_type='task.cancelled' AND t.status='CANCELLED')
    OR (e.event_type='task.timed_out' AND t.status='TIMED_OUT'))
ORDER BY e.id FOR UPDATE OF e SKIP LOCKED LIMIT $1`, limit)
	if queryErr != nil {
		return 0, queryErr
	}
	for rows.Next() {
		var item candidate
		if scanErr := rows.Scan(&item.sourceID, &item.eventType, &item.sequence, &item.occurredAt, &item.tenantID, &item.applicationID, &item.taskID, &item.turnID, &item.correlationID, &item.taskStatus, &item.webhookURL); scanErr != nil {
			rows.Close()
			return 0, scanErr
		}
		items = append(items, item)
	}
	rows.Close()
	if rowsErr := rows.Err(); rowsErr != nil {
		return 0, rowsErr
	}
	projectedCount := 0
	for _, item := range items {
		if item.applicationID <= 0 || item.webhookURL == "" {
			if _, err := tx.Exec(ctx, `UPDATE workforce_task_events SET open_integration_projection_status='SKIPPED' WHERE id=$1 AND open_integration_projection_status='PENDING'`, item.sourceID); err != nil {
				return 0, err
			}
			continue
		}
		if item.turnID <= 0 {
			return 0, errors.New("Open Task terminal event is missing its current Task Turn identity")
		}
		initialDeliveryStatus := "PENDING"
		var applicationStatus string
		if s.beforeProjectionApplicationLock != nil {
			s.beforeProjectionApplicationLock()
		}
		// Serialize projection with enable/disable so a disable commit cannot be
		// followed by a newly-created PENDING delivery for the same application.
		if err := tx.QueryRow(ctx, `SELECT status FROM workforce_integration_applications WHERE id=$1 AND tenant_id=$2 FOR UPDATE`, item.applicationID, item.tenantID).Scan(&applicationStatus); err != nil {
			return 0, err
		}
		if applicationStatus != domain.ApplicationEnabled {
			initialDeliveryStatus = "SUSPENDED"
		}
		if s.idGen == nil {
			return 0, errors.New("Open Integration ID generator is unavailable")
		}
		eventID, deliveryID := s.idGen.NextID(), s.idGen.NextID()
		status, reasonCode, summary := domain.PublicStatus(item.taskStatus)
		envelope := domain.PublicEventEnvelope{
			Schema: "seven.workforce.public-event/v1", EventID: eventID, DeliveryID: deliveryID,
			EventType: item.eventType, OccurredAt: item.occurredAt.UTC().Format(time.RFC3339Nano), Sequence: item.sequence,
			TenantID: item.tenantID, ApplicationID: item.applicationID, CorrelationID: item.correlationID,
			Subject: domain.EventSubject{Type: "TASK", ID: item.taskID},
			Data:    domain.EventData{TaskID: item.taskID, TaskTurnID: item.turnID, Status: status, ReasonCode: reasonCode, ResultSummary: summary},
			Links: domain.EventLinks{
				Task:    "/api/open/workforce/v1/tasks/" + strconv.FormatInt(item.taskID, 10),
				Outputs: "/api/open/workforce/v1/tasks/" + strconv.FormatInt(item.taskID, 10) + "/outputs",
			},
		}
		payload, err := json.Marshal(envelope)
		if err != nil || len(payload) > 64<<10 {
			return 0, fmt.Errorf("marshal public event: %w", err)
		}
		tag, err := tx.Exec(ctx, `INSERT INTO workforce_public_events
(id,tenant_id,integration_application_id,source_task_event_id,task_id,task_turn_id,event_type,sequence,correlation_id,payload_json,occurred_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) ON CONFLICT (source_task_event_id) DO NOTHING`, eventID, item.tenantID, item.applicationID, item.sourceID, item.taskID, item.turnID, item.eventType, item.sequence, item.correlationID, payload, item.occurredAt)
		if err != nil {
			return 0, err
		}
		if tag.RowsAffected() != 0 {
			if _, err := tx.Exec(ctx, `INSERT INTO workforce_webhook_deliveries
(id,tenant_id,integration_application_id,public_event_id,task_id,event_type,status,next_attempt_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,CURRENT_TIMESTAMP) ON CONFLICT (public_event_id) DO NOTHING`, deliveryID, item.tenantID, item.applicationID, eventID, item.taskID, item.eventType, initialDeliveryStatus); err != nil {
				return 0, err
			}
			projectedCount++
		}
		if _, err := tx.Exec(ctx, `UPDATE workforce_task_events SET open_integration_projection_status='PROJECTED' WHERE id=$1 AND open_integration_projection_status='PENDING'`, item.sourceID); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return projectedCount, nil
}

type DeliveryClaim struct {
	Delivery                 domain.Delivery
	CurrentSigningCredential EncryptedSigningCredential
	GraceSigningCredential   *EncryptedSigningCredential
}

type EncryptedSigningCredential struct {
	ID     int64
	Secret secretvalue.SecretValue
}

// SigningCredentialLease serializes an outbound request with Application
// status and Signing Secret rotation. The dedicated PostgreSQL advisory lock
// stays held through HTTP delivery and Attempt persistence, so a committed
// rotation can never be followed by a request using its newly revoked version.
type SigningCredentialLease struct {
	Current EncryptedSigningCredential
	Grace   *EncryptedSigningCredential
	conn    *pgxpool.Conn
	lockKey int64
	once    sync.Once
	err     error
}

func (l *SigningCredentialLease) Release() error {
	if l == nil {
		return nil
	}
	l.once.Do(func() {
		if l.conn == nil {
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var unlocked bool
		l.err = l.conn.QueryRow(ctx, `SELECT pg_advisory_unlock($1)`, l.lockKey).Scan(&unlocked)
		if l.err == nil && unlocked {
			l.conn.Release()
			l.conn = nil
			return
		}
		if l.err == nil {
			l.err = errors.New("webhook signing credential lifecycle lock was not held")
		}
		// An unlock timeout or ambiguous result must never return a possibly
		// locked session to the pool. Hijacking and closing destroys it.
		raw := l.conn.Hijack()
		l.conn = nil
		if closeErr := raw.Close(ctx); closeErr != nil {
			l.err = errors.Join(l.err, closeErr)
		}
	})
	return l.err
}

func (s *Store) RenewDeliveryLease(ctx context.Context, lease *SigningCredentialLease, delivery domain.Delivery, owner string, duration time.Duration) error {
	if lease == nil || lease.conn == nil || delivery.ID <= 0 || delivery.ApplicationID <= 0 || delivery.TenantID <= 0 || delivery.AttemptCount <= 0 || strings.TrimSpace(owner) == "" {
		return errors.New("webhook delivery lease renewal input is invalid")
	}
	if duration <= 0 {
		duration = time.Minute
	}
	result, err := lease.conn.Exec(ctx, `UPDATE workforce_webhook_deliveries
SET lease_expires_at=CURRENT_TIMESTAMP+make_interval(secs => $5),updated_at=CURRENT_TIMESTAMP
WHERE id=$1 AND tenant_id=$2 AND integration_application_id=$3
  AND attempt_count=$4 AND status='SENDING' AND lease_owner=$6`,
		delivery.ID, delivery.TenantID, delivery.ApplicationID, delivery.AttemptCount, int64(duration/time.Second), owner)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return errors.New("webhook delivery lease was lost before send")
	}
	return nil
}

func (s *Store) AcquireSigningCredentialLease(ctx context.Context, tenantID, applicationID int64) (*SigningCredentialLease, error) {
	if tenantID <= 0 || applicationID <= 0 {
		return nil, errors.New("webhook signing credential scope is invalid")
	}
	conn, err := s.db.Acquire(ctx)
	if err != nil {
		return nil, err
	}
	lease := &SigningCredentialLease{conn: conn, lockKey: -applicationID}
	cleanup := func(cause error) (*SigningCredentialLease, error) {
		_ = lease.Release()
		return nil, cause
	}
	if _, err = conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, lease.lockKey); err != nil {
		// A transport error can leave the server-side session holding the lock
		// even though the caller never observed a successful result. Destroy the
		// session instead of returning an ambiguous advisory lock to the pool.
		closeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		raw := conn.Hijack()
		_ = raw.Close(closeCtx)
		lease.conn = nil
		return nil, err
	}
	var applicationStatus string
	if err = conn.QueryRow(ctx, `SELECT status FROM workforce_integration_applications WHERE tenant_id=$1 AND id=$2`, tenantID, applicationID).Scan(&applicationStatus); errors.Is(err, pgx.ErrNoRows) {
		return cleanup(ErrNotFound)
	} else if err != nil {
		return cleanup(err)
	}
	if applicationStatus != domain.ApplicationEnabled {
		return cleanup(ErrApplicationDisabled)
	}
	rows, err := conn.Query(ctx, `SELECT id,tenant_id,integration_application_id,status,encode(signing_secret_ciphertext,'base64'),encode(signing_secret_edek,'base64'),signing_secret_wrap_key_ref
FROM workforce_integration_signing_credentials
WHERE tenant_id=$1 AND integration_application_id=$2
  AND status IN ('CURRENT','GRACE') AND revoked_at IS NULL
  AND (status='CURRENT' OR valid_until>CURRENT_TIMESTAMP)
ORDER BY CASE status WHEN 'CURRENT' THEN 0 ELSE 1 END,id`, tenantID, applicationID)
	if err != nil {
		return cleanup(err)
	}
	credentials := make([]signingCredentialRecord, 0, 2)
	for rows.Next() {
		var credential signingCredentialRecord
		if err = rows.Scan(&credential.ID, &credential.TenantID, &credential.ApplicationID, &credential.Status, &credential.Secret.CiphertextB64, &credential.Secret.EDEKB64, &credential.Secret.WrapKeyRef); err != nil {
			rows.Close()
			return cleanup(err)
		}
		credentials = append(credentials, credential)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return cleanup(err)
	}
	claims := []DeliveryClaim{{Delivery: domain.Delivery{TenantID: tenantID, ApplicationID: applicationID}}}
	if err = attachSigningCredentials(claims, credentials); err != nil {
		return cleanup(err)
	}
	lease.Current = claims[0].CurrentSigningCredential
	lease.Grace = claims[0].GraceSigningCredential
	return lease, nil
}

func (s *Store) ClaimDeliveries(ctx context.Context, owner string, limit int, lease time.Duration) ([]DeliveryClaim, error) {
	if limit < 1 || limit > 32 {
		limit = 32
	}
	if lease <= 0 {
		lease = time.Minute
	}
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `SELECT d.id,d.public_event_id,d.task_id,d.event_type,d.status,d.attempt_count,e.payload_json,d.tenant_id,d.integration_application_id,a.webhook_url
FROM workforce_webhook_deliveries d
JOIN workforce_public_events e ON e.id=d.public_event_id
JOIN workforce_integration_applications a ON a.id=d.integration_application_id AND a.tenant_id=d.tenant_id
WHERE a.status='ENABLED' AND ((d.status IN ('PENDING','RETRY_WAIT') AND d.next_attempt_at<=CURRENT_TIMESTAMP)
  OR (d.status='SENDING' AND d.lease_expires_at<CURRENT_TIMESTAMP))
ORDER BY d.next_attempt_at,d.created_at FOR UPDATE OF d SKIP LOCKED LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	claims := make([]DeliveryClaim, 0, limit)
	for rows.Next() {
		var claim DeliveryClaim
		if err := rows.Scan(&claim.Delivery.ID, &claim.Delivery.EventID, &claim.Delivery.TaskID, &claim.Delivery.EventType, &claim.Delivery.Status, &claim.Delivery.AttemptCount, &claim.Delivery.Payload, &claim.Delivery.TenantID, &claim.Delivery.ApplicationID, &claim.Delivery.WebhookURL); err != nil {
			rows.Close()
			return nil, err
		}
		claims = append(claims, claim)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(claims) > 0 {
		applicationIDs := distinctDeliveryApplicationIDs(claims)
		credentialRows, err := tx.Query(ctx, `SELECT id,tenant_id,integration_application_id,status,encode(signing_secret_ciphertext,'base64'),encode(signing_secret_edek,'base64'),signing_secret_wrap_key_ref
FROM workforce_integration_signing_credentials
WHERE integration_application_id=ANY($1::bigint[])
  AND status IN ('CURRENT','GRACE')
  AND revoked_at IS NULL
  AND (status='CURRENT' OR valid_until>CURRENT_TIMESTAMP)
ORDER BY integration_application_id,CASE status WHEN 'CURRENT' THEN 0 ELSE 1 END,id`, applicationIDs)
		if err != nil {
			return nil, err
		}
		credentials := make([]signingCredentialRecord, 0, len(applicationIDs)*2)
		for credentialRows.Next() {
			var credential signingCredentialRecord
			if err := credentialRows.Scan(&credential.ID, &credential.TenantID, &credential.ApplicationID, &credential.Status, &credential.Secret.CiphertextB64, &credential.Secret.EDEKB64, &credential.Secret.WrapKeyRef); err != nil {
				credentialRows.Close()
				return nil, err
			}
			credentials = append(credentials, credential)
		}
		credentialRows.Close()
		if err := credentialRows.Err(); err != nil {
			return nil, err
		}
		if err := attachSigningCredentials(claims, credentials); err != nil {
			return nil, err
		}
	}
	for index := range claims {
		claims[index].Delivery.AttemptCount++
		if _, err := tx.Exec(ctx, `UPDATE workforce_webhook_deliveries SET status='SENDING',attempt_count=$2,lease_owner=$3,lease_expires_at=CURRENT_TIMESTAMP+make_interval(secs => $4),updated_at=CURRENT_TIMESTAMP WHERE id=$1`, claims[index].Delivery.ID, claims[index].Delivery.AttemptCount, owner, int64(lease/time.Second)); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return claims, nil
}

type signingCredentialRecord struct {
	ID, TenantID, ApplicationID int64
	Status                      string
	Secret                      secretvalue.SecretValue
}

func distinctDeliveryApplicationIDs(claims []DeliveryClaim) []int64 {
	seen := make(map[int64]struct{}, len(claims))
	result := make([]int64, 0, len(claims))
	for _, claim := range claims {
		if _, exists := seen[claim.Delivery.ApplicationID]; exists {
			continue
		}
		seen[claim.Delivery.ApplicationID] = struct{}{}
		result = append(result, claim.Delivery.ApplicationID)
	}
	return result
}

func attachSigningCredentials(claims []DeliveryClaim, credentials []signingCredentialRecord) error {
	type signingSet struct {
		current *EncryptedSigningCredential
		grace   *EncryptedSigningCredential
		tenant  int64
	}
	sets := make(map[int64]*signingSet, len(claims))
	for _, claim := range claims {
		if claim.Delivery.ApplicationID <= 0 || claim.Delivery.TenantID <= 0 {
			return errors.New("webhook delivery signing identity is invalid")
		}
		set := sets[claim.Delivery.ApplicationID]
		if set == nil {
			sets[claim.Delivery.ApplicationID] = &signingSet{tenant: claim.Delivery.TenantID}
		} else if set.tenant != claim.Delivery.TenantID {
			return errors.New("webhook delivery application tenant is inconsistent")
		}
	}
	for _, credential := range credentials {
		set := sets[credential.ApplicationID]
		if set == nil || credential.ID <= 0 || credential.TenantID != set.tenant {
			return errors.New("webhook signing credential scope is invalid")
		}
		value := &EncryptedSigningCredential{ID: credential.ID, Secret: credential.Secret}
		switch credential.Status {
		case domain.CredentialCurrent:
			if set.current != nil {
				return errors.New("webhook signing credential has duplicate CURRENT rows")
			}
			set.current = value
		case domain.CredentialGrace:
			if set.grace != nil {
				return errors.New("webhook signing credential has duplicate active GRACE rows")
			}
			set.grace = value
		default:
			return errors.New("webhook signing credential status is invalid")
		}
	}
	for index := range claims {
		set := sets[claims[index].Delivery.ApplicationID]
		if set == nil || set.current == nil {
			return errors.New("webhook signing credential CURRENT row is missing")
		}
		claims[index].CurrentSigningCredential = *set.current
		claims[index].Delivery.SigningCredentialID = set.current.ID
		if set.grace != nil {
			grace := *set.grace
			claims[index].GraceSigningCredential = &grace
			graceID := grace.ID
			claims[index].Delivery.GraceSigningCredentialID = &graceID
		}
	}
	return nil
}

type DeliveryOutcome struct {
	Status, FailureClass   string
	HTTPStatus             *int
	Duration               time.Duration
	NextAttemptAt          *time.Time
	SigningCredentialsUsed bool
}

func (s *Store) FinishDeliveryWithLease(ctx context.Context, lease *SigningCredentialLease, delivery domain.Delivery, owner string, outcome DeliveryOutcome) error {
	if lease == nil || lease.conn == nil || lease.Current.ID != delivery.SigningCredentialID || strings.TrimSpace(owner) == "" {
		return errors.New("webhook signing credential lease does not match delivery")
	}
	tx, err := lease.conn.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := s.finishDeliveryTx(ctx, tx, delivery, owner, outcome); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) finishDeliveryTx(ctx context.Context, tx pgx.Tx, delivery domain.Delivery, owner string, outcome DeliveryOutcome) error {
	if s.idGen == nil {
		return errors.New("Open Integration ID generator is unavailable")
	}
	if delivery.SigningCredentialID <= 0 {
		return errors.New("webhook delivery signing credential is unavailable")
	}
	if delivery.GraceSigningCredentialID != nil && (*delivery.GraceSigningCredentialID <= 0 || *delivery.GraceSigningCredentialID == delivery.SigningCredentialID) {
		return errors.New("webhook delivery grace signing credential is invalid")
	}
	attemptID := s.idGen.NextID()
	if _, err := tx.Exec(ctx, `INSERT INTO workforce_webhook_delivery_attempts
(id,tenant_id,integration_application_id,delivery_id,attempt_number,status,http_status,failure_class,duration_millis,signing_credential_id,grace_signing_credential_id)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`, attemptID, delivery.TenantID, delivery.ApplicationID, delivery.ID, delivery.AttemptCount, outcome.Status, outcome.HTTPStatus, outcome.FailureClass, outcome.Duration.Milliseconds(), delivery.SigningCredentialID, delivery.GraceSigningCredentialID); err != nil {
		return err
	}
	var next any
	if outcome.NextAttemptAt != nil {
		next = outcome.NextAttemptAt.UTC()
	}
	result, err := tx.Exec(ctx, `UPDATE workforce_webhook_deliveries SET status=$3::varchar,last_http_status=$4,last_failure_class=$5,next_attempt_at=COALESCE($6,next_attempt_at),lease_owner=NULL,lease_expires_at=NULL,delivered_at=CASE WHEN $3::varchar='DELIVERED' THEN CURRENT_TIMESTAMP ELSE delivered_at END,updated_at=CURRENT_TIMESTAMP WHERE id=$1 AND attempt_count=$2 AND status='SENDING' AND lease_owner=$7`, delivery.ID, delivery.AttemptCount, outcome.Status, outcome.HTTPStatus, outcome.FailureClass, next, owner)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return errors.New("webhook delivery lease was lost")
	}
	if outcome.SigningCredentialsUsed {
		credentialIDs := deliverySigningCredentialIDs(delivery)
		updated, err := tx.Exec(ctx, `UPDATE workforce_integration_signing_credentials
SET last_used_at=CURRENT_TIMESTAMP
WHERE tenant_id=$1 AND integration_application_id=$2 AND id=ANY($3::bigint[])`, delivery.TenantID, delivery.ApplicationID, credentialIDs)
		if err != nil {
			return err
		}
		if updated.RowsAffected() != int64(len(credentialIDs)) {
			return errors.New("webhook signing credential usage scope changed")
		}
	}
	return nil
}

func deliverySigningCredentialIDs(delivery domain.Delivery) []int64 {
	result := []int64{delivery.SigningCredentialID}
	if delivery.GraceSigningCredentialID != nil && *delivery.GraceSigningCredentialID != delivery.SigningCredentialID {
		result = append(result, *delivery.GraceSigningCredentialID)
	}
	return result
}
