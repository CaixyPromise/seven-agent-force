package infrastructure

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/provider/application"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/provider/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct{ db *pgxpool.Pool }

func NewStore(db *pgxpool.Pool) *Store { return &Store{db: db} }

func (s *Store) Create(ctx context.Context, item domain.Credential) (domain.Credential, error) {
	models, _ := json.Marshal(item.Models)
	err := s.db.QueryRow(ctx, `
INSERT INTO workforce_model_credentials
 (id,tenant_id,name,protocol,base_url,secret_ciphertext,secret_edek,secret_wrap_key_ref,secret_hint,secret_version,models_json,status,version)
VALUES ($1,$2,$3,$4,$5,decode($6,'base64'),decode($7,'base64'),$8,$9,1,$10,$11,1)
RETURNING secret_version,version,created_at,updated_at`, item.ID, item.TenantID, item.Name, item.Protocol, item.BaseURL,
		item.SecretCiphertext, item.SecretEDEK, item.SecretWrapKeyRef, item.SecretHint, models, item.Status).
		Scan(&item.SecretVersion, &item.Version, &item.CreatedAt, &item.UpdatedAt)
	if isUniqueViolation(err) {
		return domain.Credential{}, application.ErrNameConflict
	}
	return item, err
}

func (s *Store) ListTenant(ctx context.Context, tenantID int64) ([]domain.Credential, error) {
	rows, err := s.db.Query(ctx, credentialSelect+` WHERE tenant_id=$1 AND status<>'DELETED' ORDER BY name,id`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]domain.Credential, 0)
	for rows.Next() {
		item, scanErr := scanCredential(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, item)
	}
	if err = rows.Err(); err != nil || len(items) == 0 {
		return items, err
	}
	usageRows, err := s.db.Query(ctx, `SELECT model_credential_id,COUNT(*),COALESCE(SUM(input_tokens),0),COALESCE(SUM(cached_input_tokens),0),COALESCE(SUM(output_tokens),0),COALESCE(SUM(total_tokens),0) FROM workforce_model_invocations WHERE tenant_id=$1 GROUP BY model_credential_id`, tenantID)
	if err != nil {
		return nil, err
	}
	defer usageRows.Close()
	usage := make(map[int64][5]int64)
	for usageRows.Next() {
		var id int64
		var values [5]int64
		if err = usageRows.Scan(&id, &values[0], &values[1], &values[2], &values[3], &values[4]); err != nil {
			return nil, err
		}
		usage[id] = values
	}
	if err = usageRows.Err(); err != nil {
		return nil, err
	}
	for index := range items {
		values := usage[items[index].ID]
		items[index].InvocationCount, items[index].InputTokens, items[index].CachedInputTokens = values[0], values[1], values[2]
		items[index].OutputTokens, items[index].TotalTokens = values[3], values[4]
	}
	return items, nil
}

func (s *Store) GetTenant(ctx context.Context, tenantID, id int64) (domain.Credential, error) {
	item, err := scanCredential(s.db.QueryRow(ctx, credentialSelect+` WHERE tenant_id=$1 AND id=$2 AND status<>'DELETED'`, tenantID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Credential{}, application.ErrNotFound
	}
	return item, err
}

func (s *Store) GetTenantForShare(ctx context.Context, tx pgx.Tx, tenantID, id int64) (domain.Credential, error) {
	item, err := scanCredential(tx.QueryRow(ctx, credentialSelect+` WHERE tenant_id=$1 AND id=$2 AND status<>'DELETED' FOR SHARE`, tenantID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Credential{}, application.ErrNotFound
	}
	return item, err
}

func (s *Store) Update(ctx context.Context, item domain.Credential, rotateSecret bool) (domain.Credential, error) {
	return updateCredential(ctx, s.db, item, rotateSecret)
}

type credentialQuery interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}

func updateCredential(ctx context.Context, query credentialQuery, item domain.Credential, rotateSecret bool) (domain.Credential, error) {
	models, _ := json.Marshal(item.Models)
	secretIncrement := 0
	if rotateSecret {
		secretIncrement = 1
	}
	err := query.QueryRow(ctx, `
UPDATE workforce_model_credentials
SET name=$3,protocol=$4,base_url=$5,secret_ciphertext=decode($6,'base64'),secret_edek=decode($7,'base64'),secret_wrap_key_ref=$8,secret_hint=$9,
    secret_version=secret_version+$10,models_json=$11,status=$12,version=version+1,updated_at=CURRENT_TIMESTAMP
WHERE tenant_id=$1 AND id=$2 AND version=$13
RETURNING secret_version,version,created_at,updated_at`, item.TenantID, item.ID, item.Name, item.Protocol, item.BaseURL,
		item.SecretCiphertext, item.SecretEDEK, item.SecretWrapKeyRef, item.SecretHint, secretIncrement, models, item.Status, item.Version).
		Scan(&item.SecretVersion, &item.Version, &item.CreatedAt, &item.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Credential{}, application.ErrVersionConflict
	}
	if isUniqueViolation(err) {
		return domain.Credential{}, application.ErrNameConflict
	}
	return item, err
}

func (s *Store) UpdateDiscovery(ctx context.Context, tenantID, id int64, status, reason string) error {
	_, err := s.db.Exec(ctx, `UPDATE workforce_model_credentials SET last_discovery_status=$3,last_discovery_reason_code=$4,last_discovered_at=CURRENT_TIMESTAMP,updated_at=CURRENT_TIMESTAMP WHERE tenant_id=$1 AND id=$2`, tenantID, id, status, reason)
	return err
}

func countActiveProtocolUses(ctx context.Context, query credentialQuery, tenantID, credentialID int64) (int, error) {
	var count int
	err := query.QueryRow(ctx, `SELECT
	 (SELECT COUNT(*) FROM workforce_tasks t WHERE t.tenant_id=$1 AND t.model_credential_id=$2 AND t.status NOT IN ('SUCCEEDED','FAILED','CANCELLED','TIMED_OUT','MANUAL_REVIEW'))
	 +(SELECT COUNT(*) FROM workforce_model_invocations i WHERE i.tenant_id=$1 AND i.model_credential_id=$2 AND i.status IN ('AUTHORIZED','STREAMING'))`, tenantID, credentialID).Scan(&count)
	return count, err
}

type credentialUpdateTransaction struct{ tx pgx.Tx }

func (r credentialUpdateTransaction) CountActiveProtocolUses(ctx context.Context, tenantID, id int64) (int, error) {
	return countActiveProtocolUses(ctx, r.tx, tenantID, id)
}

func (r credentialUpdateTransaction) Update(ctx context.Context, item domain.Credential, rotate bool) (domain.Credential, error) {
	return updateCredential(ctx, r.tx, item, rotate)
}

func (s *Store) WithLockedCredential(ctx context.Context, tenantID, id int64, fn func(domain.Credential, application.CredentialUpdateRepository) error) error {
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.ReadCommitted})
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	item, err := scanCredential(tx.QueryRow(ctx, credentialSelect+` WHERE tenant_id=$1 AND id=$2 AND status<>'DELETED' FOR UPDATE`, tenantID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return application.ErrNotFound
	}
	if err != nil {
		return err
	}
	if err = fn(item, credentialUpdateTransaction{tx: tx}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (s *Store) Delete(ctx context.Context, tenantID, credentialID, expectedVersion int64) (domain.DeleteResult, error) {
	tx, err := s.db.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return domain.DeleteResult{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var version int64
	err = tx.QueryRow(ctx, `SELECT version FROM workforce_model_credentials WHERE tenant_id=$1 AND id=$2 AND status<>'DELETED' FOR UPDATE`, tenantID, credentialID).Scan(&version)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.DeleteResult{}, application.ErrNotFound
	}
	if err != nil {
		return domain.DeleteResult{}, err
	}
	if version != expectedVersion {
		return domain.DeleteResult{}, application.ErrVersionConflict
	}

	referenced := false
	for _, check := range credentialReferenceChecks(tenantID, credentialID) {
		if err = tx.QueryRow(ctx, check.query, check.args...).Scan(&referenced); err != nil {
			return domain.DeleteResult{}, err
		}
		if referenced {
			break
		}
	}

	if referenced {
		command, updateErr := tx.Exec(ctx, `UPDATE workforce_model_credentials
SET secret_ciphertext=NULL,secret_edek=NULL,secret_wrap_key_ref=NULL,secret_hint='',models_json='[]'::jsonb,
    status='DELETED',last_discovery_status='',last_discovery_reason_code='',last_discovered_at=NULL,
    version=version+1,updated_at=CURRENT_TIMESTAMP
WHERE tenant_id=$1 AND id=$2 AND version=$3 AND status<>'DELETED'`, tenantID, credentialID, expectedVersion)
		if updateErr != nil {
			return domain.DeleteResult{}, updateErr
		}
		if command.RowsAffected() != 1 {
			return domain.DeleteResult{}, application.ErrVersionConflict
		}
	} else {
		command, deleteErr := tx.Exec(ctx, `DELETE FROM workforce_model_credentials WHERE tenant_id=$1 AND id=$2 AND version=$3 AND status<>'DELETED'`, tenantID, credentialID, expectedVersion)
		if deleteErr != nil {
			return domain.DeleteResult{}, deleteErr
		}
		if command.RowsAffected() != 1 {
			return domain.DeleteResult{}, application.ErrVersionConflict
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.DeleteResult{}, err
	}
	return domain.DeleteResult{Deleted: true, RetainedHistory: referenced}, nil
}

type credentialReferenceCheck struct {
	query string
	args  []any
}

func credentialReferenceChecks(tenantID, credentialID int64) []credentialReferenceCheck {
	return []credentialReferenceCheck{
		{query: `SELECT EXISTS(SELECT 1 FROM workforce_tasks WHERE tenant_id=$1 AND model_credential_id=$2)`, args: []any{tenantID, credentialID}},
		{query: `SELECT EXISTS(SELECT 1 FROM workforce_model_invocations WHERE tenant_id=$1 AND model_credential_id=$2)`, args: []any{tenantID, credentialID}},
		{query: `SELECT EXISTS(SELECT 1 FROM workforce_task_assignments WHERE model_credential_id=$1)`, args: []any{credentialID}},
		{query: `SELECT EXISTS(SELECT 1 FROM workforce_review_runs WHERE tenant_id=$1 AND model_credential_id=$2)`, args: []any{tenantID, credentialID}},
		{query: `SELECT EXISTS(SELECT 1 FROM workforce_review_run_attempts WHERE model_credential_id=$1)`, args: []any{credentialID}},
	}
}

const credentialSelect = `SELECT id,tenant_id,name,protocol,base_url,encode(secret_ciphertext,'base64'),encode(secret_edek,'base64'),secret_wrap_key_ref,secret_hint,secret_version,models_json,status,version,last_discovery_status,last_discovery_reason_code,last_discovered_at,created_at,updated_at FROM workforce_model_credentials`

type rowScanner interface{ Scan(...any) error }

func scanCredential(row rowScanner) (domain.Credential, error) {
	var item domain.Credential
	var models []byte
	err := row.Scan(&item.ID, &item.TenantID, &item.Name, &item.Protocol, &item.BaseURL, &item.SecretCiphertext, &item.SecretEDEK, &item.SecretWrapKeyRef, &item.SecretHint, &item.SecretVersion, &models, &item.Status, &item.Version, &item.LastDiscoveryStatus, &item.LastDiscoveryReasonCode, &item.LastDiscoveredAt, &item.CreatedAt, &item.UpdatedAt)
	if err != nil {
		return domain.Credential{}, err
	}
	if err = json.Unmarshal(models, &item.Models); err != nil {
		return domain.Credential{}, err
	}
	return item, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
