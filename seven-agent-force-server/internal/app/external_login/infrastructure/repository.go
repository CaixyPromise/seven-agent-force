package infrastructure

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/external_login/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/store"
	"github.com/bytedance/sonic"
	"github.com/jmoiron/sqlx"
)

type Repository struct {
	db         store.SQLX
	transactor store.Transactor
	postgres   bool
}

func NewRepository(provider store.Provider) (*Repository, error) {
	if provider == nil {
		return nil, fmt.Errorf("external login repository requires datasource provider")
	}
	var db store.SQLX
	if sqlxDB := provider.SQLX(); sqlxDB != nil {
		db = sqlxDB
	}
	transactor := provider.Transactor()
	if transactor == nil {
		if sqlxDB, ok := db.(*sqlx.DB); ok {
			transactor = store.NewSQLXTransactor(sqlxDB)
		}
	}
	dialect := strings.ToLower(strings.TrimSpace(provider.Dialect()))
	return &Repository{
		db:         db,
		transactor: transactor,
		postgres:   dialect == "postgres" || dialect == "postgresql" || dialect == "pgx",
	}, nil
}

func (r *Repository) rebind(exec store.SQLX, query string) string {
	if r.postgres {
		query = externalLoginPostgresRenderer.RenderPostgres(query)
	}
	return exec.Rebind(query)
}

func (r *Repository) executor(ctx context.Context) store.SQLX {
	return store.SQLXExecutor(ctx, r.db)
}

func (r *Repository) requireExecutor(ctx context.Context) (store.SQLX, error) {
	exec := r.executor(ctx)
	if exec == nil {
		return nil, fmt.Errorf("external login repository datasource is not configured")
	}
	return exec, nil
}

func (r *Repository) withinTransaction(ctx context.Context, fn func(ctx context.Context) error) error {
	if store.SQLXFromContext(ctx) != nil {
		return fn(ctx)
	}
	if r.transactor == nil || !r.transactor.Enabled() {
		return fmt.Errorf("external login repository transaction support is not configured")
	}
	return r.transactor.WithinTransaction(ctx, fn)
}

type providerRow struct {
	ID                       int64          `db:"id"`
	ProviderCode             string         `db:"providerCode"`
	ProviderName             string         `db:"providerName"`
	ProtocolType             string         `db:"protocolType"`
	Issuer                   sql.NullString `db:"issuer"`
	AuthorizationEndpoint    string         `db:"authorizationEndpoint"`
	TokenEndpoint            string         `db:"tokenEndpoint"`
	UserinfoEndpoint         sql.NullString `db:"userinfoEndpoint"`
	JWKSURI                  sql.NullString `db:"jwksUri"`
	ClientID                 string         `db:"clientId"`
	ClientSecretCiphertext   sql.NullString `db:"clientSecretCiphertext"`
	ClientSecretEDEK         sql.NullString `db:"clientSecretEdek"`
	ClientSecretWrapKeyRef   sql.NullString `db:"clientSecretWrapKeyRef"`
	ScopesJSON               string         `db:"scopesJson"`
	RedirectURI              string         `db:"redirectUri"`
	DisplayName              string         `db:"displayName"`
	Icon                     sql.NullString `db:"icon"`
	SortOrder                int            `db:"sortOrder"`
	DisplayEnabled           int            `db:"displayEnabled"`
	LoginEnabled             int            `db:"loginEnabled"`
	BindEnabled              int            `db:"bindEnabled"`
	EmailAutoBindEnabled     int            `db:"emailAutoBindEnabled"`
	AccountAutoCreateEnabled int            `db:"accountAutoCreateEnabled"`
	Status                   int            `db:"status"`
	MetadataJSON             sql.NullString `db:"metadataJson"`
	CreatorID                sql.NullInt64  `db:"creatorId"`
	UpdaterID                sql.NullInt64  `db:"updaterId"`
	CreateTime               time.Time      `db:"createTime"`
	UpdateTime               time.Time      `db:"updateTime"`
}

type providerMethodRow struct {
	ID                 int64          `db:"id"`
	ProviderCode       string         `db:"providerCode"`
	MethodKey          string         `db:"methodKey"`
	CapabilityCode     string         `db:"capabilityCode"`
	RequiredScopesJSON sql.NullString `db:"requiredScopesJson"`
	Status             int            `db:"status"`
	MetadataJSON       sql.NullString `db:"metadataJson"`
	CreateTime         time.Time      `db:"createTime"`
	UpdateTime         time.Time      `db:"updateTime"`
}

type identityRow struct {
	ID                  int64          `db:"id"`
	ProviderCode        string         `db:"providerCode"`
	ExternalIssuer      sql.NullString `db:"externalIssuer"`
	ExternalSubject     string         `db:"externalSubject"`
	SubjectCiphertext   sql.NullString `db:"subject_ciphertext"`
	SubjectEDEK         sql.NullString `db:"subject_edek"`
	SubjectWrapKeyRef   sql.NullString `db:"subject_wrap_key_ref"`
	SubjectDigest       sql.NullString `db:"subject_digest"`
	SubjectDigestKeyRef sql.NullString `db:"subject_digest_key_ref"`
	UserID              int64          `db:"userId"`
	ExternalLogin       sql.NullString `db:"externalLogin"`
	ExternalEmail       sql.NullString `db:"externalEmail"`
	EmailVerified       int            `db:"emailVerified"`
	DisplayName         sql.NullString `db:"displayName"`
	AvatarURL           sql.NullString `db:"avatarUrl"`
	ProfileJSON         sql.NullString `db:"profileJson"`
	Status              int            `db:"status"`
	FirstLinkedAt       time.Time      `db:"firstLinkedAt"`
	LastLoginAt         sql.NullTime   `db:"lastLoginAt"`
	LastVerifiedAt      sql.NullTime   `db:"lastVerifiedAt"`
	MetadataJSON        sql.NullString `db:"metadataJson"`
	CreatorID           sql.NullInt64  `db:"creatorId"`
	UpdaterID           sql.NullInt64  `db:"updaterId"`
	CreateTime          time.Time      `db:"createTime"`
	UpdateTime          time.Time      `db:"updateTime"`
}

type managedProviderCommandRow struct {
	ProviderCode      string    `db:"providerCode"`
	ConnectionVersion string    `db:"connectionVersion"`
	RequestHash       string    `db:"requestHash"`
	CreateTime        time.Time `db:"createTime"`
}

type loginStateRow struct {
	ID                      int64          `db:"id"`
	StateID                 string         `db:"stateId"`
	ProviderCode            string         `db:"providerCode"`
	PlatformCode            sql.NullString `db:"platformCode"`
	ProvisioningAuthorityID sql.NullString `db:"provisioningAuthorityId"`
	LoginTransactionID      sql.NullString `db:"loginTransactionId"`
	RedirectAfterLogin      sql.NullString `db:"redirectAfterLogin"`
	BindUserID              sql.NullInt64  `db:"bindUserId"`
	StateHash               string         `db:"stateHash"`
	NonceHash               sql.NullString `db:"nonceHash"`
	CodeVerifierCiphertext  sql.NullString `db:"codeVerifierCiphertext"`
	CodeVerifierEDEK        sql.NullString `db:"codeVerifierEdek"`
	CodeVerifierWrapKeyRef  sql.NullString `db:"codeVerifierWrapKeyRef"`
	Issuer                  sql.NullString `db:"issuer"`
	ProviderConfigDigest    sql.NullString `db:"providerConfigDigest"`
	RedirectURI             string         `db:"redirectUri"`
	ExpiresAt               time.Time      `db:"expiresAt"`
	ConsumedAt              sql.NullTime   `db:"consumedAt"`
	Status                  int            `db:"status"`
	LoginIP                 sql.NullString `db:"loginIp"`
	UserAgent               sql.NullString `db:"userAgent"`
	TraceID                 sql.NullString `db:"traceId"`
	CreateTime              time.Time      `db:"createTime"`
	UpdateTime              time.Time      `db:"updateTime"`
}

type tokenRow struct {
	ID                 int64          `db:"id"`
	ProviderCode       string         `db:"providerCode"`
	IdentityID         int64          `db:"identityId"`
	UserID             int64          `db:"userId"`
	TokenPurpose       string         `db:"tokenPurpose"`
	ScopeJSON          sql.NullString `db:"scopeJson"`
	ScopeHash          string         `db:"scopeHash"`
	TokenSetCiphertext string         `db:"tokenSetCiphertext"`
	TokenSetEDEK       string         `db:"tokenSetEdek"`
	TokenSetWrapKeyRef string         `db:"tokenSetWrapKeyRef"`
	AccessExpiresAt    sql.NullTime   `db:"accessExpiresAt"`
	RefreshExpiresAt   sql.NullTime   `db:"refreshExpiresAt"`
	LastRefreshAt      sql.NullTime   `db:"lastRefreshAt"`
	RevokedAt          sql.NullTime   `db:"revokedAt"`
	Status             int            `db:"status"`
	Version            int            `db:"version"`
	MetadataJSON       sql.NullString `db:"metadataJson"`
	CreateTime         time.Time      `db:"createTime"`
	UpdateTime         time.Time      `db:"updateTime"`
}

func (r *Repository) InsertProvider(ctx context.Context, item *domain.Provider, actorID int64) error {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return err
	}
	_, err = exec.ExecContext(ctx, r.rebind(exec, `
INSERT INTO sys_external_login_provider (
    providerCode, providerName, protocolType, issuer, authorizationEndpoint, tokenEndpoint,
    userinfoEndpoint, jwksUri, clientId, clientSecretCiphertext, clientSecretEdek,
    clientSecretWrapKeyRef, scopesJson, redirectUri, displayName, icon, sortOrder,
    displayEnabled, loginEnabled, bindEnabled, emailAutoBindEnabled, accountAutoCreateEnabled, status, metadataJson,
    creatorId, updaterId, isDeleted
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0)`),
		item.ProviderCode, item.ProviderName, item.ProtocolType, nullIfBlank(item.Issuer),
		item.AuthorizationEndpoint, item.TokenEndpoint, nullIfBlank(item.UserinfoEndpoint), nullIfBlank(item.JWKSURI),
		item.ClientID, nullIfBlank(item.ClientSecretCiphertext), nullIfBlank(item.ClientSecretEDEK),
		nullIfBlank(item.ClientSecretWrapKeyRef), jsonString(item.Scopes), item.RedirectURI, item.DisplayName,
		nullIfBlank(item.Icon), item.SortOrder, boolToInt(item.DisplayEnabled), boolToInt(item.LoginEnabled),
		boolToInt(item.BindEnabled), boolToInt(item.EmailAutoBindEnabled), boolToInt(item.AccountAutoCreateEnabled),
		item.Status, nullIfBlank(item.MetadataJSON),
		actorID, actorID,
	)
	if err != nil {
		return fmt.Errorf("insert external login provider: %w", err)
	}
	return nil
}

func (r *Repository) UpdateProvider(ctx context.Context, item *domain.Provider, actorID int64) error {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return err
	}
	_, err = exec.ExecContext(ctx, r.rebind(exec, `
UPDATE sys_external_login_provider
SET providerName = ?, protocolType = ?, issuer = ?, authorizationEndpoint = ?, tokenEndpoint = ?,
    userinfoEndpoint = ?, jwksUri = ?, clientId = ?, clientSecretCiphertext = ?, clientSecretEdek = ?,
    clientSecretWrapKeyRef = ?, scopesJson = ?, redirectUri = ?, displayName = ?, icon = ?, sortOrder = ?,
    displayEnabled = ?, loginEnabled = ?, bindEnabled = ?, emailAutoBindEnabled = ?, accountAutoCreateEnabled = ?, metadataJson = ?,
    updaterId = ?, updateTime = CURRENT_TIMESTAMP
WHERE providerCode = ? AND isDeleted = 0`),
		item.ProviderName, item.ProtocolType, nullIfBlank(item.Issuer), item.AuthorizationEndpoint, item.TokenEndpoint,
		nullIfBlank(item.UserinfoEndpoint), nullIfBlank(item.JWKSURI), item.ClientID, nullIfBlank(item.ClientSecretCiphertext),
		nullIfBlank(item.ClientSecretEDEK), nullIfBlank(item.ClientSecretWrapKeyRef), jsonString(item.Scopes),
		item.RedirectURI, item.DisplayName, nullIfBlank(item.Icon), item.SortOrder, boolToInt(item.DisplayEnabled),
		boolToInt(item.LoginEnabled), boolToInt(item.BindEnabled), boolToInt(item.EmailAutoBindEnabled),
		boolToInt(item.AccountAutoCreateEnabled), nullIfBlank(item.MetadataJSON), actorID, item.ProviderCode,
	)
	if err != nil {
		return fmt.Errorf("update external login provider: %w", err)
	}
	return nil
}

func (r *Repository) UpdateProviderStatus(ctx context.Context, providerCode string, status int, actorID int64, now time.Time) (bool, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return false, err
	}
	result, err := exec.ExecContext(ctx, r.rebind(exec, `
UPDATE sys_external_login_provider
SET status = ?, updaterId = ?, updateTime = ?
WHERE providerCode = ? AND status <> ? AND isDeleted = 0`), status, actorID, now, providerCode, status)
	if err != nil {
		return false, fmt.Errorf("update external login provider status: %w", err)
	}
	affected, _ := result.RowsAffected()
	return affected > 0, nil
}

func (r *Repository) FindProvider(ctx context.Context, providerCode string) (*domain.Provider, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return nil, err
	}
	var row providerRow
	if err := sqlx.GetContext(ctx, exec, &row, r.rebind(exec, providerSelectSQL+`
FROM sys_external_login_provider p
WHERE p.providerCode = ? AND p.isDeleted = 0
LIMIT 1`), providerCode); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("find external login provider: %w", err)
	}
	item := mapProviderRow(row)
	return &item, nil
}

func (r *Repository) FindProviderForUpdate(ctx context.Context, providerCode string) (*domain.Provider, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return nil, err
	}
	var row providerRow
	if err := sqlx.GetContext(ctx, exec, &row, r.rebind(exec, providerSelectSQL+`
FROM sys_external_login_provider p
WHERE p.providerCode = ? AND p.isDeleted = 0
LIMIT 1 FOR UPDATE`), providerCode); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("lock external login provider: %w", err)
	}
	item := mapProviderRow(row)
	return &item, nil
}

func (r *Repository) ListProviders(ctx context.Context, query domain.ProviderQuery) ([]domain.Provider, int64, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return nil, 0, err
	}
	where, args := buildProviderWhere(query)
	var total int64
	if err := sqlx.GetContext(ctx, exec, &total, r.rebind(exec, `SELECT COUNT(*) FROM sys_external_login_provider p `+where), args...); err != nil {
		return nil, 0, fmt.Errorf("count external login providers: %w", err)
	}
	current, pageSize := normalizePage(query.Current, query.PageSize)
	listArgs := append([]any{}, args...)
	listArgs = append(listArgs, pageSize, (current-1)*pageSize)
	var rows []providerRow
	if err := sqlx.SelectContext(ctx, exec, &rows, r.rebind(exec, providerSelectSQL+`
FROM sys_external_login_provider p `+where+`
ORDER BY p.sortOrder ASC, p.id ASC
LIMIT ? OFFSET ?`), listArgs...); err != nil {
		return nil, 0, fmt.Errorf("list external login providers: %w", err)
	}
	return mapProviderRows(rows), total, nil
}

func (r *Repository) ListLoginMethods(ctx context.Context) ([]domain.Provider, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return nil, err
	}
	var rows []providerRow
	if err := sqlx.SelectContext(ctx, exec, &rows, r.rebind(exec, providerSelectSQL+`
FROM sys_external_login_provider p
WHERE p.status = ? AND p.displayEnabled = 1 AND p.loginEnabled = 1 AND p.isDeleted = 0
  AND EXISTS (
      SELECT 1 FROM sys_external_provider_method m
      WHERE m.providerCode = p.providerCode AND m.status = ? AND m.isDeleted = 0
  )
ORDER BY p.sortOrder ASC, p.id ASC`), domain.ProviderStatusActive, domain.ProviderMethodStatusActive); err != nil {
		return nil, fmt.Errorf("list external login methods: %w", err)
	}
	return mapProviderRows(rows), nil
}

func (r *Repository) ReplaceProviderMethods(ctx context.Context, providerCode string, methods []domain.ProviderMethod) error {
	return r.withinTransaction(ctx, func(txCtx context.Context) error {
		exec, err := r.requireExecutor(txCtx)
		if err != nil {
			return err
		}
		for _, method := range methods {
			if _, err = exec.ExecContext(txCtx, r.rebind(exec, `
DELETE FROM sys_external_provider_method
WHERE providerCode = ? AND methodKey = ? AND isDeleted = 1`), providerCode, method.MethodKey); err != nil {
				return fmt.Errorf("delete stale external provider method: %w", err)
			}
		}
		if _, err = exec.ExecContext(txCtx, r.rebind(exec, `
UPDATE sys_external_provider_method
SET isDeleted = 1, updateTime = CURRENT_TIMESTAMP
WHERE providerCode = ? AND isDeleted = 0`), providerCode); err != nil {
			return fmt.Errorf("soft delete external provider methods: %w", err)
		}
		for _, method := range methods {
			if _, err = exec.ExecContext(txCtx, r.rebind(exec, `
INSERT INTO sys_external_provider_method (
    providerCode, methodKey, capabilityCode, requiredScopesJson, status, metadataJson, isDeleted
) VALUES (?, ?, ?, ?, ?, ?, 0)`),
				providerCode, method.MethodKey, method.CapabilityCode, nullIfBlank(jsonString(method.RequiredScopes)),
				method.Status, nullIfBlank(method.MetadataJSON),
			); err != nil {
				return fmt.Errorf("insert external provider method: %w", err)
			}
		}
		return nil
	})
}

func (r *Repository) ListProviderMethods(ctx context.Context, providerCode string) ([]domain.ProviderMethod, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return nil, err
	}
	var rows []providerMethodRow
	if err := sqlx.SelectContext(ctx, exec, &rows, r.rebind(exec, `
SELECT id, providerCode, methodKey, capabilityCode, requiredScopesJson, status, metadataJson, createTime, updateTime
FROM sys_external_provider_method
WHERE providerCode = ? AND isDeleted = 0
ORDER BY id ASC`), providerCode); err != nil {
		return nil, fmt.Errorf("list external provider methods: %w", err)
	}
	items := make([]domain.ProviderMethod, 0, len(rows))
	for _, row := range rows {
		items = append(items, mapProviderMethodRow(row))
	}
	return items, nil
}

func (r *Repository) FindIdentityBySubject(ctx context.Context, providerCode, externalIssuer, externalSubject string) (*domain.ExternalIdentity, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return nil, err
	}
	var row identityRow
	if err := sqlx.GetContext(ctx, exec, &row, r.rebind(exec, identitySelectSQL+`
FROM sys_external_user_identity
WHERE providerCode = ? AND COALESCE(externalIssuer,'') = ? AND subject_digest = ? AND scope_type='SYSTEM' AND isDeleted = 0
LIMIT 1`), providerCode, strings.TrimSpace(externalIssuer), externalSubject); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("find protected external identity: %w", err)
	}
	item := mapIdentityRow(row)
	return &item, nil
}

func (r *Repository) InsertIdentity(ctx context.Context, item *domain.ExternalIdentity, actorID int64) error {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return err
	}
	_, err = exec.ExecContext(ctx, r.rebind(exec, `
INSERT INTO sys_external_user_identity (
    id, providerCode, externalIssuer, externalSubject, userId, externalLogin, externalEmail, emailVerified, displayName,
    avatarUrl, profileJson, status, firstLinkedAt, lastLoginAt, lastVerifiedAt, metadataJson,
    creatorId, updaterId, isDeleted, scope_type, subject_ciphertext, subject_edek, subject_wrap_key_ref, subject_digest, subject_digest_key_ref
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, 'SYSTEM', ?, ?, ?, ?, ?)`),
		item.ID, item.ProviderCode, strings.TrimSpace(item.ExternalIssuer), nullIfBlank(systemLegacySubject(item)), item.UserID, nullIfBlank(item.ExternalLogin), nullIfBlank(item.ExternalEmail),
		boolToInt(item.EmailVerified), nullIfBlank(item.DisplayName), nullIfBlank(item.AvatarURL), nullIfBlank(item.ProfileJSON),
		item.Status, item.FirstLinkedAt, item.LastLoginAt, item.LastVerifiedAt, nullIfBlank(item.MetadataJSON), actorID, actorID,
		nullIfBlank(item.SubjectCiphertext), nullIfBlank(item.SubjectEDEK), nullIfBlank(item.SubjectWrapKeyRef), firstNonBlank(item.SubjectDigest, item.ExternalSubject), nullIfBlank(item.SubjectDigestKeyRef),
	)
	if err != nil {
		return fmt.Errorf("insert external identity: %w", err)
	}
	return nil
}

func (r *Repository) CountIdentitiesByProvider(ctx context.Context, providerCode string) (int64, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return 0, err
	}
	var count int64
	if err := sqlx.GetContext(ctx, exec, &count, r.rebind(exec, `SELECT COUNT(*) FROM sys_external_user_identity WHERE providerCode = ? AND scope_type='SYSTEM' AND isDeleted = 0`), providerCode); err != nil {
		return 0, fmt.Errorf("count external identities by provider: %w", err)
	}
	return count, nil
}

func (r *Repository) FindManagedProviderCommand(ctx context.Context, providerCode, connectionVersion string) (*domain.ManagedProviderCommand, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return nil, err
	}
	var row managedProviderCommandRow
	if err := sqlx.GetContext(ctx, exec, &row, r.rebind(exec, `SELECT providerCode, connectionVersion, requestHash, createdAt AS createTime
FROM sys_external_managed_provider_command WHERE providerCode = ? AND connectionVersion = ? LIMIT 1`), providerCode, connectionVersion); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("find managed provider command: %w", err)
	}
	item := domain.ManagedProviderCommand{ProviderCode: row.ProviderCode, ConnectionVersion: row.ConnectionVersion, RequestHash: row.RequestHash, CreateTime: row.CreateTime}
	return &item, nil
}

func (r *Repository) InsertManagedProviderCommand(ctx context.Context, command *domain.ManagedProviderCommand) error {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return err
	}
	_, err = exec.ExecContext(ctx, r.rebind(exec, `INSERT INTO sys_external_managed_provider_command (providerCode, connectionVersion, requestHash, createdAt) VALUES (?, ?, ?, ?)`),
		command.ProviderCode, command.ConnectionVersion, command.RequestHash, command.CreateTime)
	if err != nil {
		return fmt.Errorf("insert managed provider command: %w", err)
	}
	return nil
}

func (r *Repository) ListIdentities(ctx context.Context, query domain.IdentityQuery) ([]domain.ExternalIdentity, int64, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return nil, 0, err
	}
	where, args := buildIdentityWhere(query)
	var total int64
	if err := sqlx.GetContext(ctx, exec, &total, r.rebind(exec, `SELECT COUNT(*) FROM sys_external_user_identity i `+where), args...); err != nil {
		return nil, 0, fmt.Errorf("count external identities: %w", err)
	}
	current, pageSize := normalizePage(query.Current, query.PageSize)
	listArgs := append([]any{}, args...)
	listArgs = append(listArgs, pageSize, (current-1)*pageSize)
	var rows []identityRow
	if err := sqlx.SelectContext(ctx, exec, &rows, r.rebind(exec, identitySelectSQL+`
FROM sys_external_user_identity i `+where+`
ORDER BY i.updateTime DESC, i.id DESC
LIMIT ? OFFSET ?`), listArgs...); err != nil {
		return nil, 0, fmt.Errorf("list external identities: %w", err)
	}
	items := make([]domain.ExternalIdentity, 0, len(rows))
	for _, row := range rows {
		items = append(items, mapIdentityRow(row))
	}
	return items, total, nil
}

func (r *Repository) UpdateIdentityStatus(ctx context.Context, identityID int64, status int, actorID int64, now time.Time) (bool, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return false, err
	}
	result, err := exec.ExecContext(ctx, r.rebind(exec, `
UPDATE sys_external_user_identity
SET status = ?, updaterId = ?, updateTime = ?
WHERE id = ? AND status <> ? AND scope_type='SYSTEM' AND isDeleted = 0`), status, actorID, now, identityID, status)
	if err != nil {
		return false, fmt.Errorf("update external identity status: %w", err)
	}
	affected, _ := result.RowsAffected()
	return affected > 0, nil
}

func (r *Repository) TouchIdentityLogin(ctx context.Context, identityID int64, profile domain.ExternalProfile, now time.Time) error {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return err
	}
	_, err = exec.ExecContext(ctx, r.rebind(exec, `
UPDATE sys_external_user_identity
SET externalLogin = ?, externalEmail = ?, emailVerified = ?, displayName = ?, avatarUrl = ?,
    profileJson = ?, lastLoginAt = ?, lastVerifiedAt = ?, updateTime = ?
WHERE id = ? AND scope_type='SYSTEM' AND isDeleted = 0`),
		nullIfBlank(profile.Login), nullIfBlank(profile.Email), boolToInt(profile.EmailVerified), nullIfBlank(profile.DisplayName),
		nullIfBlank(profile.AvatarURL), nullIfBlank(profile.RawProfile), now, now, now, identityID,
	)
	if err != nil {
		return fmt.Errorf("touch external identity login: %w", err)
	}
	return nil
}

func (r *Repository) InsertLoginState(ctx context.Context, item *domain.LoginState) error {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return err
	}
	_, err = exec.ExecContext(ctx, r.rebind(exec, `
INSERT INTO sys_external_oauth_login_state (
    stateId, providerCode, platformCode, provisioningAuthorityId, loginTransactionId, redirectAfterLogin, bindUserId, stateHash, nonceHash,
    codeVerifierCiphertext, codeVerifierEdek, codeVerifierWrapKeyRef, issuer, providerConfigDigest, redirectUri,
    expiresAt, consumedAt, status, loginIp, userAgent, traceId, isDeleted
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0)`),
		item.StateID, item.ProviderCode, nullIfBlank(item.PlatformCode), nullIfBlank(item.ProvisioningAuthorityID), nullIfBlank(item.LoginTransactionID), nullIfBlank(item.RedirectAfterLogin),
		nullableInt64Value(item.BindUserID), item.StateHash, nullIfBlank(item.NonceHash), nullIfBlank(item.CodeVerifierCiphertext), nullIfBlank(item.CodeVerifierEDEK),
		nullIfBlank(item.CodeVerifierWrapKeyRef), nullIfBlank(item.Issuer), nullIfBlank(item.ProviderConfigDigest), item.RedirectURI, item.ExpiresAt,
		item.ConsumedAt, item.Status, nullIfBlank(item.LoginIP), nullIfBlank(item.UserAgent), nullIfBlank(item.TraceID),
	)
	if err != nil {
		return fmt.Errorf("insert external login state: %w", err)
	}
	return nil
}

func (r *Repository) ConsumeLoginState(ctx context.Context, stateHash string, now time.Time) (*domain.LoginState, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return nil, err
	}
	var row loginStateRow
	if err := sqlx.GetContext(ctx, exec, &row, r.rebind(exec, loginStateSelectSQL+`
FROM sys_external_oauth_login_state
WHERE stateHash = ? AND status = ? AND expiresAt > ? AND isDeleted = 0
LIMIT 1`), stateHash, domain.LoginStateStatusActive, now); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("find external login state: %w", err)
	}
	result, err := exec.ExecContext(ctx, r.rebind(exec, `
UPDATE sys_external_oauth_login_state
SET consumedAt = ?, status = ?, updateTime = ?
WHERE id = ? AND status = ? AND consumedAt IS NULL AND expiresAt > ? AND isDeleted = 0`),
		now, domain.LoginStateStatusConsumed, now, row.ID, domain.LoginStateStatusActive, now)
	if err != nil {
		return nil, fmt.Errorf("consume external login state: %w", err)
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		return nil, nil
	}
	item := mapLoginStateRow(row)
	item.ConsumedAt = &now
	item.Status = domain.LoginStateStatusConsumed
	return &item, nil
}

func (r *Repository) InsertToken(ctx context.Context, item *domain.OAuthToken) error {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return err
	}
	_, err = exec.ExecContext(ctx, r.rebind(exec, `
INSERT INTO sys_external_oauth_token (
    providerCode, identityId, userId, tokenPurpose, scopeJson, scopeHash, tokenSetCiphertext,
    tokenSetEdek, tokenSetWrapKeyRef, accessExpiresAt, refreshExpiresAt, lastRefreshAt,
    revokedAt, status, version, metadataJson, isDeleted
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0)`),
		item.ProviderCode, item.IdentityID, item.UserID, item.TokenPurpose, nullIfBlank(jsonString(item.Scopes)),
		item.ScopeHash, item.TokenSetCiphertext, item.TokenSetEDEK, item.TokenSetWrapKeyRef, item.AccessExpiresAt,
		item.RefreshExpiresAt, item.LastRefreshAt, item.RevokedAt, item.Status, item.Version, nullIfBlank(item.MetadataJSON),
	)
	if err != nil {
		return fmt.Errorf("insert external oauth token: %w", err)
	}
	return nil
}

func (r *Repository) FindActiveToken(ctx context.Context, providerCode string, identityID int64, userID int64, tokenPurpose string, scopeHash string) (*domain.OAuthToken, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return nil, err
	}
	var row tokenRow
	if err := sqlx.GetContext(ctx, exec, &row, r.rebind(exec, tokenSelectSQL+`
FROM sys_external_oauth_token t
WHERE t.providerCode = ? AND t.identityId = ? AND t.userId = ? AND t.tokenPurpose = ?
  AND t.scopeHash = ? AND t.status = ? AND t.revokedAt IS NULL AND t.isDeleted = 0
ORDER BY t.updateTime DESC, t.id DESC
LIMIT 1`), strings.TrimSpace(providerCode), identityID, userID, strings.TrimSpace(tokenPurpose), strings.TrimSpace(scopeHash), domain.TokenStatusActive); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("find active external oauth token: %w", err)
	}
	item := mapTokenRow(row)
	return &item, nil
}

func (r *Repository) UpdateTokenSet(ctx context.Context, item *domain.OAuthToken, expectedVersion int) (bool, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return false, err
	}
	result, err := exec.ExecContext(ctx, r.rebind(exec, `
UPDATE sys_external_oauth_token
SET tokenSetCiphertext = ?, tokenSetEdek = ?, tokenSetWrapKeyRef = ?, accessExpiresAt = ?,
    refreshExpiresAt = ?, lastRefreshAt = ?, status = ?, version = version + 1,
    metadataJson = ?, updateTime = CURRENT_TIMESTAMP
WHERE id = ? AND version = ? AND status = ? AND revokedAt IS NULL AND isDeleted = 0`),
		item.TokenSetCiphertext, item.TokenSetEDEK, item.TokenSetWrapKeyRef, item.AccessExpiresAt,
		item.RefreshExpiresAt, item.LastRefreshAt, item.Status, nullIfBlank(item.MetadataJSON), item.ID, expectedVersion, domain.TokenStatusActive,
	)
	if err != nil {
		return false, fmt.Errorf("update external oauth token set: %w", err)
	}
	affected, _ := result.RowsAffected()
	return affected > 0, nil
}

func (r *Repository) RevokeToken(ctx context.Context, tokenID int64, now time.Time, reason string) (bool, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return false, err
	}
	query := `
UPDATE sys_external_oauth_token
SET revokedAt = ?, status = ?, version = version + 1, updateTime = ?, metadataJson = JSON_SET(COALESCE(metadataJson, JSON_OBJECT()), '$.revokeReason', ?)
WHERE id = ? AND status = ? AND revokedAt IS NULL AND isDeleted = 0`
	if r.postgres {
		query = `
UPDATE sys_external_oauth_token
SET revokedAt = ?, status = ?, version = version + 1, updateTime = ?, metadataJson = jsonb_set(COALESCE(metadataJson::jsonb, '{}'::jsonb), '{revokeReason}', to_jsonb(?::text), true)::json
WHERE id = ? AND status = ? AND revokedAt IS NULL AND isDeleted = 0`
	}
	result, err := exec.ExecContext(ctx, r.rebind(exec, query), now, domain.TokenStatusRevoked, now, reason, tokenID, domain.TokenStatusActive)
	if err != nil {
		return false, fmt.Errorf("revoke external oauth token: %w", err)
	}
	affected, _ := result.RowsAffected()
	return affected > 0, nil
}

func (r *Repository) RevokeTokensByProvider(ctx context.Context, providerCode string, now time.Time, reason string) (int64, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return 0, err
	}
	query := `
UPDATE sys_external_oauth_token
SET revokedAt = ?, status = ?, version = version + 1, updateTime = ?, metadataJson = JSON_SET(COALESCE(metadataJson, JSON_OBJECT()), '$.revokeReason', ?)
WHERE providerCode = ? AND status = ? AND revokedAt IS NULL AND isDeleted = 0`
	if r.postgres {
		query = `
UPDATE sys_external_oauth_token
SET revokedAt = ?, status = ?, version = version + 1, updateTime = ?, metadataJson = jsonb_set(COALESCE(metadataJson::jsonb, '{}'::jsonb), '{revokeReason}', to_jsonb(?::text), true)::json
WHERE providerCode = ? AND status = ? AND revokedAt IS NULL AND isDeleted = 0`
	}
	result, err := exec.ExecContext(ctx, r.rebind(exec, query), now, domain.TokenStatusRevoked, now, reason, providerCode, domain.TokenStatusActive)
	if err != nil {
		return 0, fmt.Errorf("revoke external oauth tokens by provider: %w", err)
	}
	affected, _ := result.RowsAffected()
	return affected, nil
}

func (r *Repository) RevokeTokensByIdentity(ctx context.Context, identityID int64, now time.Time, reason string) (int64, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return 0, err
	}
	query := `
UPDATE sys_external_oauth_token
SET revokedAt = ?, status = ?, version = version + 1, updateTime = ?, metadataJson = JSON_SET(COALESCE(metadataJson, JSON_OBJECT()), '$.revokeReason', ?)
WHERE identityId = ? AND status = ? AND revokedAt IS NULL AND isDeleted = 0`
	if r.postgres {
		query = `
UPDATE sys_external_oauth_token
SET revokedAt = ?, status = ?, version = version + 1, updateTime = ?, metadataJson = jsonb_set(COALESCE(metadataJson::jsonb, '{}'::jsonb), '{revokeReason}', to_jsonb(?::text), true)::json
WHERE identityId = ? AND status = ? AND revokedAt IS NULL AND isDeleted = 0`
	}
	result, err := exec.ExecContext(ctx, r.rebind(exec, query), now, domain.TokenStatusRevoked, now, reason, identityID, domain.TokenStatusActive)
	if err != nil {
		return 0, fmt.Errorf("revoke external oauth tokens by identity: %w", err)
	}
	affected, _ := result.RowsAffected()
	return affected, nil
}

func (r *Repository) ListTokens(ctx context.Context, query domain.TokenQuery) ([]domain.OAuthToken, int64, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return nil, 0, err
	}
	where, args := buildTokenWhere(query)
	var total int64
	if err := sqlx.GetContext(ctx, exec, &total, r.rebind(exec, `SELECT COUNT(*) FROM sys_external_oauth_token t `+where), args...); err != nil {
		return nil, 0, fmt.Errorf("count external oauth tokens: %w", err)
	}
	current, pageSize := normalizePage(query.Current, query.PageSize)
	listArgs := append([]any{}, args...)
	listArgs = append(listArgs, pageSize, (current-1)*pageSize)
	var rows []tokenRow
	if err := sqlx.SelectContext(ctx, exec, &rows, r.rebind(exec, tokenSelectSQL+`
FROM sys_external_oauth_token t `+where+`
ORDER BY t.updateTime DESC, t.id DESC
LIMIT ? OFFSET ?`), listArgs...); err != nil {
		return nil, 0, fmt.Errorf("list external oauth tokens: %w", err)
	}
	items := make([]domain.OAuthToken, 0, len(rows))
	for _, row := range rows {
		items = append(items, mapTokenRow(row))
	}
	return items, total, nil
}

const providerSelectSQL = `
SELECT p.id, p.providerCode, p.providerName, p.protocolType, p.issuer, p.authorizationEndpoint, p.tokenEndpoint,
       p.userinfoEndpoint, p.jwksUri, p.clientId, p.clientSecretCiphertext, p.clientSecretEdek,
       p.clientSecretWrapKeyRef, p.scopesJson, p.redirectUri, p.displayName, p.icon, p.sortOrder,
       p.displayEnabled, p.loginEnabled, p.bindEnabled, p.emailAutoBindEnabled, p.accountAutoCreateEnabled, p.status, p.metadataJson,
       p.creatorId, p.updaterId, p.createTime, p.updateTime`

const identitySelectSQL = `
SELECT id, providerCode, externalIssuer, COALESCE(externalSubject,'') AS externalSubject, subject_ciphertext, subject_edek, subject_wrap_key_ref, subject_digest, subject_digest_key_ref, userId, externalLogin, externalEmail, emailVerified,
       displayName, avatarUrl, profileJson, status, firstLinkedAt, lastLoginAt, lastVerifiedAt,
       metadataJson, creatorId, updaterId, createTime, updateTime`

const loginStateSelectSQL = `
SELECT id, stateId, providerCode, platformCode, provisioningAuthorityId, loginTransactionId, redirectAfterLogin, bindUserId, stateHash, nonceHash,
       codeVerifierCiphertext, codeVerifierEdek, codeVerifierWrapKeyRef, issuer, providerConfigDigest, redirectUri,
       expiresAt, consumedAt, status, loginIp, userAgent, traceId, createTime, updateTime`

const tokenSelectSQL = `
SELECT t.id, t.providerCode, t.identityId, t.userId, t.tokenPurpose, t.scopeJson, t.scopeHash,
       t.tokenSetCiphertext, t.tokenSetEdek, t.tokenSetWrapKeyRef, t.accessExpiresAt,
       t.refreshExpiresAt, t.lastRefreshAt, t.revokedAt, t.status, t.version, t.metadataJson,
       t.createTime, t.updateTime`

func buildProviderWhere(query domain.ProviderQuery) (string, []any) {
	parts := []string{"p.isDeleted = 0"}
	args := make([]any, 0, 4)
	if value := strings.TrimSpace(query.ProviderCode); value != "" {
		parts = append(parts, "p.providerCode = ?")
		args = append(args, value)
	}
	if value := strings.TrimSpace(query.ProtocolType); value != "" {
		parts = append(parts, "p.protocolType = ?")
		args = append(args, value)
	}
	if query.Status != nil {
		parts = append(parts, "p.status = ?")
		args = append(args, *query.Status)
	}
	if query.DisplayOnly {
		parts = append(parts, "p.displayEnabled = 1")
	}
	if keyword := strings.TrimSpace(query.Keyword); keyword != "" {
		parts = append(parts, "(p.providerCode LIKE ? OR p.providerName LIKE ? OR p.displayName LIKE ?)")
		like := "%" + keyword + "%"
		args = append(args, like, like, like)
	}
	return "WHERE " + strings.Join(parts, " AND "), args
}

func buildIdentityWhere(query domain.IdentityQuery) (string, []any) {
	parts := []string{"i.isDeleted = 0", "i.scope_type = 'SYSTEM'"}
	args := make([]any, 0, 6)
	if value := strings.TrimSpace(query.ProviderCode); value != "" {
		parts = append(parts, "i.providerCode = ?")
		args = append(args, value)
	}
	if query.UserID != nil {
		parts = append(parts, "i.userId = ?")
		args = append(args, *query.UserID)
	}
	if query.Status != nil {
		parts = append(parts, "i.status = ?")
		args = append(args, *query.Status)
	}
	if keyword := strings.TrimSpace(query.Keyword); keyword != "" {
		parts = append(parts, "(i.externalLogin LIKE ? OR i.externalEmail LIKE ? OR i.displayName LIKE ?)")
		like := "%" + keyword + "%"
		args = append(args, like, like, like)
	}
	return "WHERE " + strings.Join(parts, " AND "), args
}

func buildTokenWhere(query domain.TokenQuery) (string, []any) {
	parts := []string{"t.isDeleted = 0"}
	args := make([]any, 0, 5)
	if value := strings.TrimSpace(query.ProviderCode); value != "" {
		parts = append(parts, "t.providerCode = ?")
		args = append(args, value)
	}
	if query.IdentityID != nil {
		parts = append(parts, "t.identityId = ?")
		args = append(args, *query.IdentityID)
	}
	if query.UserID != nil {
		parts = append(parts, "t.userId = ?")
		args = append(args, *query.UserID)
	}
	if value := strings.TrimSpace(query.TokenPurpose); value != "" {
		parts = append(parts, "t.tokenPurpose = ?")
		args = append(args, value)
	}
	if query.Status != nil {
		parts = append(parts, "t.status = ?")
		args = append(args, *query.Status)
	}
	return "WHERE " + strings.Join(parts, " AND "), args
}

func mapProviderRows(rows []providerRow) []domain.Provider {
	items := make([]domain.Provider, 0, len(rows))
	for _, row := range rows {
		items = append(items, mapProviderRow(row))
	}
	return items
}

func mapProviderRow(row providerRow) domain.Provider {
	metadataJSON := nullableString(row.MetadataJSON)
	var metadata struct {
		TokenEndpointAuthMethod string `json:"tokenEndpointAuthMethod"`
	}
	_ = sonic.UnmarshalString(metadataJSON, &metadata)
	item := domain.Provider{
		ID:                       row.ID,
		ProviderCode:             row.ProviderCode,
		ProviderName:             row.ProviderName,
		ProtocolType:             row.ProtocolType,
		Issuer:                   nullableString(row.Issuer),
		AuthorizationEndpoint:    row.AuthorizationEndpoint,
		TokenEndpoint:            row.TokenEndpoint,
		TokenEndpointAuthMethod:  strings.TrimSpace(metadata.TokenEndpointAuthMethod),
		UserinfoEndpoint:         nullableString(row.UserinfoEndpoint),
		JWKSURI:                  nullableString(row.JWKSURI),
		ClientID:                 row.ClientID,
		ClientSecretCiphertext:   nullableString(row.ClientSecretCiphertext),
		ClientSecretEDEK:         nullableString(row.ClientSecretEDEK),
		ClientSecretWrapKeyRef:   nullableString(row.ClientSecretWrapKeyRef),
		RedirectURI:              row.RedirectURI,
		DisplayName:              row.DisplayName,
		Icon:                     nullableString(row.Icon),
		SortOrder:                row.SortOrder,
		DisplayEnabled:           row.DisplayEnabled == 1,
		LoginEnabled:             row.LoginEnabled == 1,
		BindEnabled:              row.BindEnabled == 1,
		EmailAutoBindEnabled:     row.EmailAutoBindEnabled == 1,
		AccountAutoCreateEnabled: row.AccountAutoCreateEnabled == 1,
		Status:                   row.Status,
		MetadataJSON:             metadataJSON,
		CreatorID:                nullableInt64(row.CreatorID),
		UpdaterID:                nullableInt64(row.UpdaterID),
		CreateTime:               row.CreateTime,
		UpdateTime:               row.UpdateTime,
	}
	_ = sonic.UnmarshalString(row.ScopesJSON, &item.Scopes)
	return item
}

func mapProviderMethodRow(row providerMethodRow) domain.ProviderMethod {
	item := domain.ProviderMethod{
		ID:             row.ID,
		ProviderCode:   row.ProviderCode,
		MethodKey:      row.MethodKey,
		CapabilityCode: row.CapabilityCode,
		Status:         row.Status,
		MetadataJSON:   nullableString(row.MetadataJSON),
		CreateTime:     row.CreateTime,
		UpdateTime:     row.UpdateTime,
	}
	if row.RequiredScopesJSON.Valid {
		_ = sonic.UnmarshalString(row.RequiredScopesJSON.String, &item.RequiredScopes)
	}
	return item
}

func mapIdentityRow(row identityRow) domain.ExternalIdentity {
	item := domain.ExternalIdentity{
		ID:                  row.ID,
		ProviderCode:        row.ProviderCode,
		ExternalIssuer:      nullableString(row.ExternalIssuer),
		ExternalSubject:     row.ExternalSubject,
		SubjectCiphertext:   nullableString(row.SubjectCiphertext),
		SubjectEDEK:         nullableString(row.SubjectEDEK),
		SubjectWrapKeyRef:   nullableString(row.SubjectWrapKeyRef),
		SubjectDigest:       nullableString(row.SubjectDigest),
		SubjectDigestKeyRef: nullableString(row.SubjectDigestKeyRef),
		UserID:              row.UserID,
		ExternalLogin:       nullableString(row.ExternalLogin),
		ExternalEmail:       nullableString(row.ExternalEmail),
		EmailVerified:       row.EmailVerified == 1,
		DisplayName:         nullableString(row.DisplayName),
		AvatarURL:           nullableString(row.AvatarURL),
		ProfileJSON:         nullableString(row.ProfileJSON),
		Status:              row.Status,
		FirstLinkedAt:       row.FirstLinkedAt,
		LastLoginAt:         nullableTime(row.LastLoginAt),
		LastVerifiedAt:      nullableTime(row.LastVerifiedAt),
		MetadataJSON:        nullableString(row.MetadataJSON),
		CreatorID:           nullableInt64(row.CreatorID),
		UpdaterID:           nullableInt64(row.UpdaterID),
		CreateTime:          row.CreateTime,
		UpdateTime:          row.UpdateTime,
	}
	return item
}

func mapLoginStateRow(row loginStateRow) domain.LoginState {
	return domain.LoginState{
		ID:                      row.ID,
		StateID:                 row.StateID,
		ProviderCode:            row.ProviderCode,
		PlatformCode:            nullableString(row.PlatformCode),
		ProvisioningAuthorityID: nullableString(row.ProvisioningAuthorityID),
		LoginTransactionID:      nullableString(row.LoginTransactionID),
		RedirectAfterLogin:      nullableString(row.RedirectAfterLogin),
		BindUserID:              nullableInt64ValueOrZero(row.BindUserID),
		StateHash:               row.StateHash,
		NonceHash:               nullableString(row.NonceHash),
		CodeVerifierCiphertext:  nullableString(row.CodeVerifierCiphertext),
		CodeVerifierEDEK:        nullableString(row.CodeVerifierEDEK),
		CodeVerifierWrapKeyRef:  nullableString(row.CodeVerifierWrapKeyRef),
		Issuer:                  nullableString(row.Issuer),
		ProviderConfigDigest:    nullableString(row.ProviderConfigDigest),
		RedirectURI:             row.RedirectURI,
		ExpiresAt:               row.ExpiresAt,
		ConsumedAt:              nullableTime(row.ConsumedAt),
		Status:                  row.Status,
		LoginIP:                 nullableString(row.LoginIP),
		UserAgent:               nullableString(row.UserAgent),
		TraceID:                 nullableString(row.TraceID),
		CreateTime:              row.CreateTime,
		UpdateTime:              row.UpdateTime,
	}
}

func mapTokenRow(row tokenRow) domain.OAuthToken {
	item := domain.OAuthToken{
		ID:                 row.ID,
		ProviderCode:       row.ProviderCode,
		IdentityID:         row.IdentityID,
		UserID:             row.UserID,
		TokenPurpose:       row.TokenPurpose,
		ScopeHash:          row.ScopeHash,
		TokenSetCiphertext: row.TokenSetCiphertext,
		TokenSetEDEK:       row.TokenSetEDEK,
		TokenSetWrapKeyRef: row.TokenSetWrapKeyRef,
		AccessExpiresAt:    nullableTime(row.AccessExpiresAt),
		RefreshExpiresAt:   nullableTime(row.RefreshExpiresAt),
		LastRefreshAt:      nullableTime(row.LastRefreshAt),
		RevokedAt:          nullableTime(row.RevokedAt),
		Status:             row.Status,
		Version:            row.Version,
		MetadataJSON:       nullableString(row.MetadataJSON),
		CreateTime:         row.CreateTime,
		UpdateTime:         row.UpdateTime,
	}
	if row.ScopeJSON.Valid {
		_ = sonic.UnmarshalString(row.ScopeJSON.String, &item.Scopes)
	}
	return item
}

func normalizePage(current, pageSize int) (int, int) {
	if current <= 0 {
		current = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 200 {
		pageSize = 200
	}
	return current, pageSize
}

func nullableString(value sql.NullString) string {
	if value.Valid {
		return strings.TrimSpace(value.String)
	}
	return ""
}

func systemLegacySubject(item *domain.ExternalIdentity) string {
	if item == nil || strings.TrimSpace(item.SubjectCiphertext) != "" {
		return ""
	}
	return strings.TrimSpace(item.ExternalSubject)
}

func firstNonBlank(values ...string) string {
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			return value
		}
	}
	return ""
}

func nullableInt64(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	v := value.Int64
	return &v
}

func nullableInt64Value(value int64) any {
	if value <= 0 {
		return nil
	}
	return value
}

func nullableInt64ValueOrZero(value sql.NullInt64) int64 {
	if !value.Valid {
		return 0
	}
	return value.Int64
}

func nullableTime(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	v := value.Time
	return &v
}

func nullIfBlank(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return strings.TrimSpace(value)
}

func jsonString(value any) string {
	raw, err := sonic.Marshal(value)
	if err != nil {
		return "[]"
	}
	return string(raw)
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
