package docker

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/store"
	secretvalueinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/crypto/secretvalue"
	"github.com/jmoiron/sqlx"
)

type RegistryRecord struct {
	ID                     int64          `db:"id"`
	Name                   string         `db:"name"`
	Code                   string         `db:"code"`
	RegistryType           string         `db:"registryType"`
	Endpoint               string         `db:"endpoint"`
	APIBaseURL             sql.NullString `db:"apiBaseUrl"`
	AuthType               string         `db:"authType"`
	Username               sql.NullString `db:"username"`
	TokenRealm             sql.NullString `db:"tokenRealm"`
	TokenService           sql.NullString `db:"tokenService"`
	CredentialID           sql.NullInt64  `db:"credentialId"`
	NamespaceWhitelistJSON sql.NullString `db:"namespaceWhitelistJson"`
	TLSEnabled             bool           `db:"tlsEnabled"`
	InsecureSkipVerify     bool           `db:"insecureSkipVerify"`
	DefaultRegistry        bool           `db:"defaultRegistry"`
	Status                 int            `db:"status"`
	Description            sql.NullString `db:"description"`
	Sort                   int            `db:"sort"`
	SecretCiphertext       sql.NullString `db:"secretCiphertext"`
	SecretEDEK             sql.NullString `db:"secretEdek"`
	WrapKeyRef             sql.NullString `db:"wrapKeyRef"`
	Deleted                int            `db:"deleted"`
	CreateTime             sql.NullTime   `db:"createTime"`
	UpdateTime             sql.NullTime   `db:"updateTime"`
}

type RegistryRepository struct {
	db store.SQLX
}

func NewRegistryRepository(provider store.Provider) (*RegistryRepository, error) {
	if provider == nil || provider.SQLX() == nil {
		return nil, fmt.Errorf("docker registry repository requires datasource provider")
	}
	return &RegistryRepository{db: provider.SQLX()}, nil
}

func (r *RegistryRepository) executor(ctx context.Context) store.SQLX {
	return store.SQLXExecutor(ctx, r.db)
}

func (r *RegistryRepository) List(ctx context.Context) ([]RegistryRecord, error) {
	exec := r.executor(ctx)
	var rows []RegistryRecord
	query := exec.Rebind(`
SELECT id, name, code, registryType, endpoint, apiBaseUrl, authType, username, tokenRealm, tokenService,
	credentialId, namespaceWhitelistJson, tlsEnabled, insecureSkipVerify, defaultRegistry, status, description, sort,
	secretCiphertext, secretEdek, wrapKeyRef, deleted, createTime, updateTime
FROM docker_remote_registry
WHERE deleted = 0
ORDER BY defaultRegistry DESC, sort ASC, updateTime DESC, id DESC`)
	if err := sqlx.SelectContext(ctx, exec, &rows, query); err != nil {
		return nil, fmt.Errorf("list docker registries: %w", err)
	}
	return rows, nil
}

func (r *RegistryRepository) Get(ctx context.Context, id int64) (*RegistryRecord, error) {
	exec := r.executor(ctx)
	var row RegistryRecord
	query := exec.Rebind(`
SELECT id, name, code, registryType, endpoint, apiBaseUrl, authType, username, tokenRealm, tokenService,
	credentialId, namespaceWhitelistJson, tlsEnabled, insecureSkipVerify, defaultRegistry, status, description, sort,
	secretCiphertext, secretEdek, wrapKeyRef, deleted, createTime, updateTime
FROM docker_remote_registry
WHERE id = ? AND deleted = 0
LIMIT 1`)
	if err := sqlx.GetContext(ctx, exec, &row, query, id); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("get docker registry: %w", err)
	}
	return &row, nil
}

func (r *RegistryRepository) CodeExists(ctx context.Context, code string, excludeID int64) (bool, error) {
	exec := r.executor(ctx)
	args := []any{strings.TrimSpace(code)}
	sqlText := `SELECT COUNT(1) FROM docker_remote_registry WHERE deleted = 0 AND code = ?`
	if excludeID > 0 {
		sqlText += ` AND id <> ?`
		args = append(args, excludeID)
	}
	var count int64
	if err := sqlx.GetContext(ctx, exec, &count, exec.Rebind(sqlText), args...); err != nil {
		return false, fmt.Errorf("count docker registry by code: %w", err)
	}
	return count > 0, nil
}

func (r *RegistryRepository) Insert(ctx context.Context, row RegistryRecord) error {
	exec := r.executor(ctx)
	_, err := exec.ExecContext(ctx, exec.Rebind(`
INSERT INTO docker_remote_registry (
	id, name, code, registryType, endpoint, apiBaseUrl, authType, username, tokenRealm, tokenService,
	credentialId, namespaceWhitelistJson, tlsEnabled, insecureSkipVerify, defaultRegistry, status, description, sort,
	secretCiphertext, secretEdek, wrapKeyRef, deleted, createTime, updateTime
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, NOW(), NOW())`),
		row.ID, row.Name, row.Code, row.RegistryType, row.Endpoint, nullableString(row.APIBaseURL.String),
		row.AuthType, nullableString(row.Username.String), nullableString(row.TokenRealm.String), nullableString(row.TokenService.String),
		nullableInt64(row.CredentialID.Int64), nullableString(row.NamespaceWhitelistJSON.String),
		boolInt(row.TLSEnabled), boolInt(row.InsecureSkipVerify), boolInt(row.DefaultRegistry), row.Status,
		nullableString(row.Description.String), row.Sort, nullableString(row.SecretCiphertext.String),
		nullableString(row.SecretEDEK.String), nullableString(row.WrapKeyRef.String))
	if err != nil {
		return fmt.Errorf("insert docker registry: %w", err)
	}
	return nil
}

func (r *RegistryRepository) Update(ctx context.Context, row RegistryRecord) error {
	exec := r.executor(ctx)
	_, err := exec.ExecContext(ctx, exec.Rebind(`
UPDATE docker_remote_registry
SET name = ?, code = ?, registryType = ?, endpoint = ?, apiBaseUrl = ?, authType = ?, username = ?,
	tokenRealm = ?, tokenService = ?, credentialId = ?, namespaceWhitelistJson = ?, tlsEnabled = ?,
	insecureSkipVerify = ?, defaultRegistry = ?, status = ?, description = ?, sort = ?, secretCiphertext = ?,
	secretEdek = ?, wrapKeyRef = ?, updateTime = NOW()
WHERE id = ? AND deleted = 0`),
		row.Name, row.Code, row.RegistryType, row.Endpoint, nullableString(row.APIBaseURL.String),
		row.AuthType, nullableString(row.Username.String), nullableString(row.TokenRealm.String), nullableString(row.TokenService.String),
		nullableInt64(row.CredentialID.Int64), nullableString(row.NamespaceWhitelistJSON.String),
		boolInt(row.TLSEnabled), boolInt(row.InsecureSkipVerify), boolInt(row.DefaultRegistry), row.Status,
		nullableString(row.Description.String), row.Sort, nullableString(row.SecretCiphertext.String),
		nullableString(row.SecretEDEK.String), nullableString(row.WrapKeyRef.String), row.ID)
	if err != nil {
		return fmt.Errorf("update docker registry: %w", err)
	}
	return nil
}

func (r *RegistryRepository) Delete(ctx context.Context, id int64) error {
	exec := r.executor(ctx)
	_, err := exec.ExecContext(ctx, exec.Rebind(`
DELETE FROM docker_remote_registry
WHERE id = ?`), id)
	if err != nil {
		return fmt.Errorf("delete local docker registry config: %w", err)
	}
	return nil
}

func (r *RegistryRepository) ClearOtherDefaults(ctx context.Context, keepID int64) error {
	exec := r.executor(ctx)
	_, err := exec.ExecContext(ctx, exec.Rebind(`
UPDATE docker_remote_registry
SET defaultRegistry = 0, updateTime = NOW()
WHERE deleted = 0 AND defaultRegistry = 1 AND id <> ?`), keepID)
	if err != nil {
		return fmt.Errorf("clear other default docker registries: %w", err)
	}
	return nil
}

func (r *RegistryRecord) SecretValue() secretvalueinfra.SecretValue {
	if r == nil {
		return secretvalueinfra.SecretValue{}
	}
	return secretvalueinfra.SecretValue{
		CiphertextB64: r.SecretCiphertext.String,
		EDEKB64:       r.SecretEDEK.String,
		WrapKeyRef:    r.WrapKeyRef.String,
	}
}

func nullableString(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return value
}

func nullableInt64(value int64) any {
	if value == 0 {
		return nil
	}
	return value
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
