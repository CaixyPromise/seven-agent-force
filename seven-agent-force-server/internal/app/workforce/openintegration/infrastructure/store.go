package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/openintegration/domain"
	secretvalue "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/crypto/secretvalue"
)

var (
	ErrNotFound             = errors.New("open integration resource not found")
	ErrUnauthorized         = errors.New("open integration credential is invalid")
	ErrApplicationDisabled  = errors.New("open integration application is disabled")
	ErrScenarioUnavailable  = errors.New("open integration scenario is unavailable")
	ErrIdempotencyConflict  = errors.New("open integration task idempotency conflict")
	ErrWebhookNotConfigured = errors.New("open integration webhook is not configured")
)

type Store struct {
	db                              *pgxpool.Pool
	idGen                           IDGenerator
	beforeProjectionApplicationLock func()
}

type IDGenerator interface{ NextID() int64 }

type OrphanDiagnostics struct {
	APICredentialsWithoutApplication              int64
	SigningCredentialsWithoutApplication          int64
	TasksWithoutApplication                       int64
	PublicEventsWithoutApplication                int64
	PublicEventsWithoutTask                       int64
	PublicEventsWithoutSourceTaskEvent            int64
	DeliveriesWithoutApplication                  int64
	DeliveriesWithoutPublicEvent                  int64
	DeliveryAttemptsWithoutDelivery               int64
	DeliveryAttemptsWithoutSigningCredential      int64
	DeliveryAttemptsWithoutGraceSigningCredential int64
}

func (d OrphanDiagnostics) Total() int64 {
	return d.APICredentialsWithoutApplication + d.SigningCredentialsWithoutApplication +
		d.TasksWithoutApplication + d.PublicEventsWithoutApplication + d.PublicEventsWithoutTask +
		d.PublicEventsWithoutSourceTaskEvent + d.DeliveriesWithoutApplication +
		d.DeliveriesWithoutPublicEvent + d.DeliveryAttemptsWithoutDelivery +
		d.DeliveryAttemptsWithoutSigningCredential + d.DeliveryAttemptsWithoutGraceSigningCredential
}

// DiagnoseOrphans keeps relationship integrity observable without database
// foreign keys. Each bounded query reads at most two physical table instances.
func (s *Store) DiagnoseOrphans(ctx context.Context) (OrphanDiagnostics, error) {
	var result OrphanDiagnostics
	checks := []struct {
		destination *int64
		query       string
	}{
		{&result.APICredentialsWithoutApplication, `SELECT COUNT(*) FROM workforce_integration_api_credentials c WHERE NOT EXISTS (SELECT 1 FROM workforce_integration_applications a WHERE a.id=c.integration_application_id AND a.tenant_id=c.tenant_id)`},
		{&result.SigningCredentialsWithoutApplication, `SELECT COUNT(*) FROM workforce_integration_signing_credentials c WHERE NOT EXISTS (SELECT 1 FROM workforce_integration_applications a WHERE a.id=c.integration_application_id AND a.tenant_id=c.tenant_id)`},
		{&result.TasksWithoutApplication, `SELECT COUNT(*) FROM workforce_tasks t WHERE t.integration_application_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM workforce_integration_applications a WHERE a.id=t.integration_application_id AND a.tenant_id=t.tenant_id)`},
		{&result.PublicEventsWithoutApplication, `SELECT COUNT(*) FROM workforce_public_events e WHERE NOT EXISTS (SELECT 1 FROM workforce_integration_applications a WHERE a.id=e.integration_application_id AND a.tenant_id=e.tenant_id)`},
		{&result.PublicEventsWithoutTask, `SELECT COUNT(*) FROM workforce_public_events e WHERE NOT EXISTS (SELECT 1 FROM workforce_tasks t WHERE t.id=e.task_id AND t.tenant_id=e.tenant_id)`},
		{&result.PublicEventsWithoutSourceTaskEvent, `SELECT COUNT(*) FROM workforce_public_events e WHERE NOT EXISTS (SELECT 1 FROM workforce_task_events te WHERE te.id=e.source_task_event_id AND te.task_id=e.task_id)`},
		{&result.DeliveriesWithoutApplication, `SELECT COUNT(*) FROM workforce_webhook_deliveries d WHERE NOT EXISTS (SELECT 1 FROM workforce_integration_applications a WHERE a.id=d.integration_application_id AND a.tenant_id=d.tenant_id)`},
		{&result.DeliveriesWithoutPublicEvent, `SELECT COUNT(*) FROM workforce_webhook_deliveries d WHERE NOT EXISTS (SELECT 1 FROM workforce_public_events e WHERE e.id=d.public_event_id AND e.integration_application_id=d.integration_application_id)`},
		{&result.DeliveryAttemptsWithoutDelivery, `SELECT COUNT(*) FROM workforce_webhook_delivery_attempts a WHERE NOT EXISTS (SELECT 1 FROM workforce_webhook_deliveries d WHERE d.id=a.delivery_id AND d.integration_application_id=a.integration_application_id)`},
		{&result.DeliveryAttemptsWithoutSigningCredential, `SELECT COUNT(*) FROM workforce_webhook_delivery_attempts a WHERE NOT EXISTS (SELECT 1 FROM workforce_integration_signing_credentials c WHERE c.id=a.signing_credential_id AND c.integration_application_id=a.integration_application_id AND c.tenant_id=a.tenant_id)`},
		{&result.DeliveryAttemptsWithoutGraceSigningCredential, `SELECT COUNT(*) FROM workforce_webhook_delivery_attempts a WHERE a.grace_signing_credential_id IS NOT NULL AND NOT EXISTS (SELECT 1 FROM workforce_integration_signing_credentials c WHERE c.id=a.grace_signing_credential_id AND c.integration_application_id=a.integration_application_id AND c.tenant_id=a.tenant_id)`},
	}
	for _, check := range checks {
		if err := s.db.QueryRow(ctx, check.query).Scan(check.destination); err != nil {
			return OrphanDiagnostics{}, err
		}
	}
	return result, nil
}

func NewStore(db *pgxpool.Pool, idGen IDGenerator, _ ...string) *Store {
	return &Store{db: db, idGen: idGen}
}

func (s *Store) ValidateScenarioForSubmission(context.Context, int64, string) error {
	// The legacy scenario-based Task submission path is deliberately disabled.
	// Open Integration submits only through the Project-scoped Task boundary.
	return ErrScenarioUnavailable
}

type CreateApplicationParams struct {
	TenantID                                                         int64
	Name, WebhookURL, CredentialID, CredentialDigest, CredentialHint string
	SigningCredential                                                *CreateSigningCredential
	CreatedByUserID                                                  int64
}

type CreateSigningCredential struct {
	Hint   string
	Secret secretvalue.SecretValue
}

type RotateAPIKeyParams struct {
	TenantID, ApplicationID, ActorUserID                int64
	CredentialLocator, CredentialDigest, CredentialHint string
	GracePeriod                                         time.Duration
}

type RotateSigningSecretParams struct {
	TenantID, ApplicationID, ActorUserID int64
	SigningSecretHint                    string
	GracePeriod                          time.Duration
}

type CredentialRotationRecord struct {
	ApplicationID        int64
	CredentialID         int64
	Version              int
	Hint                 string
	PreviousCredentialID int64
	PreviousValidUntil   time.Time
	CreatedAt            time.Time
}

func (s *Store) CreateApplication(ctx context.Context, p CreateApplicationParams) (domain.Application, error) {
	if p.CreatedByUserID <= 0 {
		return domain.Application{}, errors.New("integration application creator user identity is required")
	}
	if s.idGen == nil {
		return domain.Application{}, errors.New("integration application ID generator is unavailable")
	}
	if p.TenantID <= 0 || utf8.RuneCountInString(p.WebhookURL) > 2048 || strings.TrimSpace(p.CredentialID) == "" || strings.TrimSpace(p.CredentialDigest) == "" {
		return domain.Application{}, errors.New("integration application credential material is incomplete")
	}
	if (p.WebhookURL == "") != (p.SigningCredential == nil) {
		return domain.Application{}, errors.New("integration application webhook signing credential does not match webhook configuration")
	}
	if p.SigningCredential != nil && (strings.TrimSpace(p.SigningCredential.Hint) == "" || !completeSecretValue(p.SigningCredential.Secret)) {
		return domain.Application{}, errors.New("integration application signing credential material is incomplete")
	}
	applicationID, apiCredentialID := s.idGen.NextID(), s.idGen.NextID()
	if applicationID <= 0 || apiCredentialID <= 0 {
		return domain.Application{}, errors.New("integration application generated a non-positive identity")
	}
	application := domain.Application{
		ID: applicationID, TenantID: p.TenantID, CreatedByUserID: p.CreatedByUserID, Name: p.Name,
		WebhookURL: p.WebhookURL, Status: domain.ApplicationEnabled,
		APICredential: domain.CredentialSummary{ID: apiCredentialID, Version: 1, Hint: p.CredentialHint},
	}
	var signingCredentialID int64
	if p.SigningCredential != nil {
		signingCredentialID = s.idGen.NextID()
		if signingCredentialID <= 0 {
			return domain.Application{}, errors.New("integration application generated a non-positive signing credential identity")
		}
		application.SigningCredential = &domain.CredentialSummary{ID: signingCredentialID, Version: 1, Hint: p.SigningCredential.Hint}
	}
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return domain.Application{}, err
	}
	defer tx.Rollback(ctx)
	if err = tx.QueryRow(ctx, `INSERT INTO workforce_integration_applications
(id,tenant_id,name,status,webhook_url,created_by)
VALUES ($1,$2,$3,'ENABLED',$4,$5)
	RETURNING created_at,updated_at`, application.ID, p.TenantID, p.Name, p.WebhookURL, p.CreatedByUserID).Scan(&application.CreatedAt, &application.UpdatedAt); err != nil {
		return domain.Application{}, err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO workforce_integration_api_credentials
(id,tenant_id,integration_application_id,credential_locator,credential_digest,credential_hint,version,status,created_by)
VALUES ($1,$2,$3,$4,$5,$6,1,'CURRENT',$7)`, apiCredentialID, p.TenantID, application.ID, p.CredentialID, p.CredentialDigest, p.CredentialHint, p.CreatedByUserID); err != nil {
		return domain.Application{}, err
	}
	if p.SigningCredential != nil {
		if _, err = tx.Exec(ctx, `INSERT INTO workforce_integration_signing_credentials
(id,tenant_id,integration_application_id,signing_secret_ciphertext,signing_secret_edek,signing_secret_wrap_key_ref,signing_secret_hint,version,status,created_by)
		VALUES ($1,$2,$3,decode($4,'base64'),decode($5,'base64'),$6,$7,1,'CURRENT',$8)`, signingCredentialID, p.TenantID, application.ID, p.SigningCredential.Secret.CiphertextB64, p.SigningCredential.Secret.EDEKB64, p.SigningCredential.Secret.WrapKeyRef, p.SigningCredential.Hint, p.CreatedByUserID); err != nil {
			return domain.Application{}, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.Application{}, err
	}
	return application, nil
}

func (s *Store) ListApplications(ctx context.Context, tenantID int64, limit int) ([]domain.Application, error) {
	if limit < 1 || limit > 100 {
		limit = 50
	}
	rows, err := s.db.Query(ctx, `SELECT a.id,a.name,a.webhook_url,a.status,a.created_at,a.updated_at,a.last_used_at,
COALESCE((SELECT d.status FROM workforce_webhook_deliveries d WHERE d.tenant_id=a.tenant_id AND d.integration_application_id=a.id ORDER BY d.updated_at DESC LIMIT 1),'')
FROM workforce_integration_applications a WHERE a.tenant_id=$1 ORDER BY a.created_at DESC LIMIT $2`, tenantID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]domain.Application, 0)
	for rows.Next() {
		var item domain.Application
		item.TenantID = tenantID
		if err := rows.Scan(&item.ID, &item.Name, &item.WebhookURL, &item.Status, &item.CreatedAt, &item.UpdatedAt, &item.LastUsedAt, &item.LatestDeliveryStatus); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err = rows.Err(); err != nil || len(items) == 0 {
		return items, err
	}
	if err = s.loadCredentialSummaries(ctx, tenantID, items, true); err != nil {
		return nil, err
	}
	if err = s.loadCredentialSummaries(ctx, tenantID, items, false); err != nil {
		return nil, err
	}
	return items, nil
}

func (s *Store) loadCredentialSummaries(ctx context.Context, tenantID int64, items []domain.Application, api bool) error {
	applicationIDs := make([]int64, 0, len(items))
	byID := make(map[int64]int, len(items))
	for i := range items {
		applicationIDs = append(applicationIDs, items[i].ID)
		byID[items[i].ID] = i
	}
	query := `SELECT id,integration_application_id,version,signing_secret_hint,status,valid_until
FROM workforce_integration_signing_credentials
WHERE tenant_id=$1 AND integration_application_id=ANY($2) AND (status='CURRENT' OR (status='GRACE' AND valid_until>CURRENT_TIMESTAMP))
ORDER BY integration_application_id,version DESC`
	if api {
		query = `SELECT id,integration_application_id,version,credential_hint,status,valid_until
FROM workforce_integration_api_credentials
WHERE tenant_id=$1 AND integration_application_id=ANY($2) AND (status='CURRENT' OR (status='GRACE' AND valid_until>CURRENT_TIMESTAMP))
ORDER BY integration_application_id,version DESC`
	}
	rows, err := s.db.Query(ctx, query, tenantID, applicationIDs)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var id, applicationID int64
		var version int
		var hint, status string
		var validUntil *time.Time
		if err = rows.Scan(&id, &applicationID, &version, &hint, &status, &validUntil); err != nil {
			return err
		}
		index, ok := byID[applicationID]
		if !ok {
			return errors.New("credential query returned an application outside the requested batch")
		}
		if status == domain.CredentialCurrent {
			summary := domain.CredentialSummary{ID: id, Version: version, Hint: hint}
			if api {
				items[index].APICredential = summary
			} else {
				items[index].SigningCredential = &summary
			}
			continue
		}
		if validUntil == nil {
			return errors.New("grace credential has no validity deadline")
		}
		grace := &domain.CredentialGraceSummary{ID: id, Version: version, ValidUntil: *validUntil}
		if api {
			items[index].APIKeyGrace = grace
		} else {
			items[index].SigningSecretGrace = grace
		}
	}
	if err = rows.Err(); err != nil {
		return err
	}
	for i := range items {
		if api && items[i].APICredential.ID <= 0 {
			return errors.New("integration application is missing its current credential")
		}
		if !api && items[i].WebhookURL != "" && items[i].SigningCredential == nil {
			return errors.New("webhook integration application is missing its current signing credential")
		}
	}
	return nil
}

func (s *Store) RotateAPIKey(ctx context.Context, p RotateAPIKeyParams) (CredentialRotationRecord, error) {
	if err := validateRotationInput(s.idGen, p.TenantID, p.ApplicationID, p.ActorUserID, p.GracePeriod); err != nil {
		return CredentialRotationRecord{}, err
	}
	if strings.TrimSpace(p.CredentialLocator) == "" || strings.TrimSpace(p.CredentialDigest) == "" || strings.TrimSpace(p.CredentialHint) == "" {
		return CredentialRotationRecord{}, errors.New("API credential rotation material is incomplete")
	}
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return CredentialRotationRecord{}, err
	}
	defer tx.Rollback(ctx)
	if err = lockApplicationForCredentialRotation(ctx, tx, p.TenantID, p.ApplicationID); err != nil {
		return CredentialRotationRecord{}, err
	}
	var previousID int64
	var previousVersion int
	if err = tx.QueryRow(ctx, `SELECT id,version FROM workforce_integration_api_credentials
WHERE tenant_id=$1 AND integration_application_id=$2 AND status='CURRENT' FOR UPDATE`, p.TenantID, p.ApplicationID).Scan(&previousID, &previousVersion); errors.Is(err, pgx.ErrNoRows) {
		return CredentialRotationRecord{}, errors.New("integration application has no current API credential")
	} else if err != nil {
		return CredentialRotationRecord{}, err
	}
	var rotationTime, graceUntil time.Time
	if err = tx.QueryRow(ctx, `SELECT CURRENT_TIMESTAMP,CURRENT_TIMESTAMP+make_interval(secs => $1)`, int64(p.GracePeriod/time.Second)).Scan(&rotationTime, &graceUntil); err != nil {
		return CredentialRotationRecord{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE workforce_integration_api_credentials SET
status=CASE WHEN id=$3 THEN 'GRACE' ELSE 'REVOKED' END,
valid_until=CASE WHEN id=$3 THEN $4::timestamptz ELSE NULL END,
revoked_at=CASE WHEN id=$3 THEN NULL ELSE $5::timestamptz END,
updated_at=$5
WHERE tenant_id=$1 AND integration_application_id=$2 AND status IN ('CURRENT','GRACE')`, p.TenantID, p.ApplicationID, previousID, graceUntil, rotationTime); err != nil {
		return CredentialRotationRecord{}, err
	}
	record := CredentialRotationRecord{
		ApplicationID: p.ApplicationID, CredentialID: s.idGen.NextID(), Version: previousVersion + 1,
		Hint: p.CredentialHint, PreviousCredentialID: previousID, PreviousValidUntil: graceUntil,
	}
	if record.CredentialID <= 0 {
		return CredentialRotationRecord{}, errors.New("API credential generated a non-positive identity")
	}
	if err = tx.QueryRow(ctx, `INSERT INTO workforce_integration_api_credentials
(id,tenant_id,integration_application_id,credential_locator,credential_digest,credential_hint,version,status,created_by,created_at,updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,'CURRENT',$8,$9,$9) RETURNING created_at`, record.CredentialID, p.TenantID, p.ApplicationID, p.CredentialLocator, p.CredentialDigest, p.CredentialHint, record.Version, p.ActorUserID, rotationTime).Scan(&record.CreatedAt); err != nil {
		return CredentialRotationRecord{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return CredentialRotationRecord{}, err
	}
	return record, nil
}

func (s *Store) RotateSigningSecret(ctx context.Context, p RotateSigningSecretParams, encrypted secretvalue.SecretValue) (CredentialRotationRecord, error) {
	if err := validateRotationInput(s.idGen, p.TenantID, p.ApplicationID, p.ActorUserID, p.GracePeriod); err != nil {
		return CredentialRotationRecord{}, err
	}
	if strings.TrimSpace(p.SigningSecretHint) == "" || !completeSecretValue(encrypted) {
		return CredentialRotationRecord{}, errors.New("signing credential rotation material is incomplete")
	}
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return CredentialRotationRecord{}, err
	}
	defer tx.Rollback(ctx)
	if err = lockCredentialLifecycle(ctx, tx, p.ApplicationID); err != nil {
		return CredentialRotationRecord{}, err
	}
	if err = lockApplicationForCredentialRotation(ctx, tx, p.TenantID, p.ApplicationID); err != nil {
		return CredentialRotationRecord{}, err
	}
	var webhookURL string
	if err = tx.QueryRow(ctx, `SELECT webhook_url FROM workforce_integration_applications WHERE tenant_id=$1 AND id=$2`, p.TenantID, p.ApplicationID).Scan(&webhookURL); err != nil {
		return CredentialRotationRecord{}, err
	}
	if strings.TrimSpace(webhookURL) == "" {
		return CredentialRotationRecord{}, ErrWebhookNotConfigured
	}
	var previousID int64
	var previousVersion int
	if err = tx.QueryRow(ctx, `SELECT id,version FROM workforce_integration_signing_credentials
WHERE tenant_id=$1 AND integration_application_id=$2 AND status='CURRENT' FOR UPDATE`, p.TenantID, p.ApplicationID).Scan(&previousID, &previousVersion); errors.Is(err, pgx.ErrNoRows) {
		return CredentialRotationRecord{}, errors.New("integration application has no current signing credential")
	} else if err != nil {
		return CredentialRotationRecord{}, err
	}
	var rotationTime, graceUntil time.Time
	if err = tx.QueryRow(ctx, `SELECT CURRENT_TIMESTAMP,CURRENT_TIMESTAMP+make_interval(secs => $1)`, int64(p.GracePeriod/time.Second)).Scan(&rotationTime, &graceUntil); err != nil {
		return CredentialRotationRecord{}, err
	}
	if _, err = tx.Exec(ctx, `UPDATE workforce_integration_signing_credentials SET
status=CASE WHEN id=$3 THEN 'GRACE' ELSE 'REVOKED' END,
valid_until=CASE WHEN id=$3 THEN $4::timestamptz ELSE NULL END,
revoked_at=CASE WHEN id=$3 THEN NULL ELSE $5::timestamptz END,
updated_at=$5
WHERE tenant_id=$1 AND integration_application_id=$2 AND status IN ('CURRENT','GRACE')`, p.TenantID, p.ApplicationID, previousID, graceUntil, rotationTime); err != nil {
		return CredentialRotationRecord{}, err
	}
	record := CredentialRotationRecord{
		ApplicationID: p.ApplicationID, CredentialID: s.idGen.NextID(), Version: previousVersion + 1,
		Hint: p.SigningSecretHint, PreviousCredentialID: previousID, PreviousValidUntil: graceUntil,
	}
	if record.CredentialID <= 0 {
		return CredentialRotationRecord{}, errors.New("signing credential generated a non-positive identity")
	}
	if err = tx.QueryRow(ctx, `INSERT INTO workforce_integration_signing_credentials
(id,tenant_id,integration_application_id,signing_secret_ciphertext,signing_secret_edek,signing_secret_wrap_key_ref,signing_secret_hint,version,status,created_by,created_at,updated_at)
VALUES ($1,$2,$3,decode($4,'base64'),decode($5,'base64'),$6,$7,$8,'CURRENT',$9,$10,$10) RETURNING created_at`, record.CredentialID, p.TenantID, p.ApplicationID, encrypted.CiphertextB64, encrypted.EDEKB64, encrypted.WrapKeyRef, p.SigningSecretHint, record.Version, p.ActorUserID, rotationTime).Scan(&record.CreatedAt); err != nil {
		return CredentialRotationRecord{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return CredentialRotationRecord{}, err
	}
	return record, nil
}

func (s *Store) EnsureWebhookConfigured(ctx context.Context, tenantID, applicationID int64) error {
	if tenantID <= 0 || applicationID <= 0 {
		return domain.ErrInvalidRequest
	}
	var webhookURL string
	if err := s.db.QueryRow(ctx, `SELECT webhook_url FROM workforce_integration_applications WHERE tenant_id=$1 AND id=$2`, tenantID, applicationID).Scan(&webhookURL); errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	if strings.TrimSpace(webhookURL) == "" {
		return ErrWebhookNotConfigured
	}
	return nil
}

func validateRotationInput(idGen IDGenerator, tenantID, applicationID, actorUserID int64, gracePeriod time.Duration) error {
	if idGen == nil {
		return errors.New("integration credential ID generator is unavailable")
	}
	if tenantID <= 0 || applicationID <= 0 || actorUserID <= 0 {
		return errors.New("integration credential rotation identities are required")
	}
	if gracePeriod < time.Second {
		return errors.New("integration credential grace period must be at least one second")
	}
	return nil
}

func lockCredentialLifecycle(ctx context.Context, tx pgx.Tx, applicationID int64) error {
	if applicationID <= 0 {
		return errors.New("integration application identity is required")
	}
	_, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, -applicationID)
	return err
}

func lockApplicationForCredentialRotation(ctx context.Context, tx pgx.Tx, tenantID, applicationID int64) error {
	var id int64
	if err := tx.QueryRow(ctx, `SELECT id FROM workforce_integration_applications WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, tenantID, applicationID).Scan(&id); errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	} else {
		return err
	}
}

func (s *Store) SetApplicationStatus(ctx context.Context, tenantID, applicationID int64, status string) error {
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	if err := lockCredentialLifecycle(ctx, tx, applicationID); err != nil {
		return err
	}
	var current string
	if err := tx.QueryRow(ctx, `SELECT status FROM workforce_integration_applications WHERE id=$1 AND tenant_id=$2 FOR UPDATE`, applicationID, tenantID).Scan(&current); errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE workforce_integration_applications SET status=$3,updated_at=CURRENT_TIMESTAMP WHERE id=$1 AND tenant_id=$2`, applicationID, tenantID, status); err != nil {
		return err
	}
	if status == domain.ApplicationDisabled {
		_, err = tx.Exec(ctx, `UPDATE workforce_webhook_deliveries SET status='SUSPENDED',lease_owner=NULL,lease_expires_at=NULL,updated_at=CURRENT_TIMESTAMP WHERE tenant_id=$1 AND integration_application_id=$2 AND status IN ('PENDING','RETRY_WAIT','SENDING')`, tenantID, applicationID)
	} else {
		_, err = tx.Exec(ctx, `UPDATE workforce_webhook_deliveries SET status='PENDING',next_attempt_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP WHERE tenant_id=$1 AND integration_application_id=$2 AND status='SUSPENDED'`, tenantID, applicationID)
	}
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) Authenticate(ctx context.Context, token string) (domain.Identity, error) {
	credentialID, err := domain.ParseAPIKey(token)
	if err != nil {
		return domain.Identity{}, ErrUnauthorized
	}
	var identity domain.Identity
	var digest, applicationStatus, credentialStatus string
	var credentialEntityID int64
	var credentialUsable bool
	err = s.db.QueryRow(ctx, `SELECT a.id,a.tenant_id,a.name,a.created_by,a.status,c.id,c.credential_digest,c.status,
(c.status='CURRENT' OR (c.status='GRACE' AND c.valid_until>CURRENT_TIMESTAMP))
FROM workforce_integration_api_credentials c
JOIN workforce_integration_applications a ON a.id=c.integration_application_id AND a.tenant_id=c.tenant_id
WHERE c.credential_locator=$1`, credentialID).Scan(&identity.ApplicationID, &identity.TenantID, &identity.Name, &identity.CreatedByUserID, &applicationStatus, &credentialEntityID, &digest, &credentialStatus, &credentialUsable)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && !domain.DigestMatches(token, digest)) {
		return domain.Identity{}, ErrUnauthorized
	}
	if err != nil {
		return domain.Identity{}, err
	}
	if (credentialStatus != domain.CredentialCurrent && credentialStatus != domain.CredentialGrace) || !credentialUsable {
		return domain.Identity{}, ErrUnauthorized
	}
	if applicationStatus != domain.ApplicationEnabled {
		return domain.Identity{}, ErrApplicationDisabled
	}
	if identity.CreatedByUserID <= 0 {
		return domain.Identity{}, ErrUnauthorized
	}
	// Authentication remains an uncached read on every request so credential
	// revocation, grace expiry, and Application disable take effect immediately.
	// Usage timestamps are observability hints rather than security state; avoid
	// turning capabilities plus each downstream Tool call into hot-row WAL churn.
	_, _ = s.db.Exec(ctx, `UPDATE workforce_integration_api_credentials
SET last_used_at=CURRENT_TIMESTAMP
WHERE id=$1 AND (last_used_at IS NULL OR last_used_at<CURRENT_TIMESTAMP-INTERVAL '5 minutes')`, credentialEntityID)
	_, _ = s.db.Exec(ctx, `UPDATE workforce_integration_applications
SET last_used_at=CURRENT_TIMESTAMP
WHERE id=$1 AND (last_used_at IS NULL OR last_used_at<CURRENT_TIMESTAMP-INTERVAL '5 minutes')`, identity.ApplicationID)
	return identity, nil
}

func completeSecretValue(value secretvalue.SecretValue) bool {
	return strings.TrimSpace(value.CiphertextB64) != "" && strings.TrimSpace(value.EDEKB64) != "" && strings.TrimSpace(value.WrapKeyRef) != ""
}

func (s *Store) ValidateApplicationForSubmission(ctx context.Context, tx pgx.Tx, tenantID, applicationID int64, creatorUserID int64) error {
	var status string
	var boundCreatorUserID int64
	err := tx.QueryRow(ctx, `SELECT status,created_by FROM workforce_integration_applications WHERE id=$1 AND tenant_id=$2 FOR UPDATE`, applicationID, tenantID).Scan(&status, &boundCreatorUserID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrUnauthorized
	}
	if err != nil {
		return err
	}
	if status != domain.ApplicationEnabled {
		return ErrApplicationDisabled
	}
	if creatorUserID <= 0 || boundCreatorUserID != creatorUserID {
		return ErrUnauthorized
	}
	return nil
}

func (s *Store) GetOpenTask(ctx context.Context, identity domain.Identity, taskID int64) (domain.OpenTaskView, error) {
	var view domain.OpenTaskView
	var internalStatus string
	var createdAt time.Time
	var startedAt, finishedAt *time.Time
	err := s.db.QueryRow(ctx, `SELECT id,current_task_turn_id,project_id,status,external_correlation_id,created_at,started_at,finished_at
FROM workforce_tasks WHERE id=$1 AND tenant_id=$2 AND integration_application_id=$3`, taskID, identity.TenantID, identity.ApplicationID).Scan(&view.TaskID, &view.TaskTurnID, &view.ProjectID, &internalStatus, &view.ExternalCorrelationID, &createdAt, &startedAt, &finishedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.OpenTaskView{}, ErrNotFound
	}
	if err != nil {
		return domain.OpenTaskView{}, err
	}
	view.Schema = "seven.workforce.open-task/v2"
	view.Status, view.ReasonCode, view.ResultSummary = domain.PublicStatus(internalStatus)
	view.CreatedAt = createdAt.UTC().Format(time.RFC3339Nano)
	if startedAt != nil {
		view.StartedAt = startedAt.UTC().Format(time.RFC3339Nano)
	}
	if finishedAt != nil {
		view.FinishedAt = finishedAt.UTC().Format(time.RFC3339Nano)
	}
	view.Links.Self = "/api/open/workforce/v1/tasks/" + strconv.FormatInt(view.TaskID, 10)
	return view, nil
}

func (s *Store) ListDeliveries(ctx context.Context, tenantID, applicationID int64, limit int) ([]domain.Delivery, error) {
	if limit < 1 || limit > 50 {
		limit = 50
	}
	var exists bool
	if err := s.db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM workforce_integration_applications WHERE tenant_id=$1 AND id=$2)`, tenantID, applicationID).Scan(&exists); err != nil {
		return nil, err
	}
	if !exists {
		return nil, ErrNotFound
	}
	rows, err := s.db.Query(ctx, `SELECT d.id,d.public_event_id,d.task_id,d.event_type,d.status,d.attempt_count,d.last_http_status,d.next_attempt_at,d.updated_at
FROM workforce_webhook_deliveries d WHERE d.tenant_id=$1 AND d.integration_application_id=$2 ORDER BY d.updated_at DESC LIMIT $3`, tenantID, applicationID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]domain.Delivery, 0)
	for rows.Next() {
		var item domain.Delivery
		if err := rows.Scan(&item.ID, &item.EventID, &item.TaskID, &item.EventType, &item.Status, &item.AttemptCount, &item.LastHTTPStatus, &item.NextAttemptAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func MarshalEnvelope(envelope domain.PublicEventEnvelope) json.RawMessage {
	body, _ := json.Marshal(envelope)
	return body
}

func bearerToken(value string) string {
	scheme, token, ok := strings.Cut(strings.TrimSpace(value), " ")
	if !ok || !strings.EqualFold(scheme, "Bearer") {
		return ""
	}
	return strings.TrimSpace(token)
}

func BearerToken(value string) string { return bearerToken(value) }
