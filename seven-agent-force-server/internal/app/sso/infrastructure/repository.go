package infrastructure

import (
	"context"
	"database/sql"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/sso/domain"
	ssofacade "github.com/CaixyPromise/seven-agent/force/internal/app/sso/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/store"
	"github.com/bytedance/sonic"
	"github.com/jmoiron/sqlx"
)

type Repository struct {
	db       store.SQLX
	postgres bool
}

const (
	ssoClientRedirectRowMax       = 100
	ssoClientRedirectReplaceChunk = 50
	ssoSessionRevocationPageMax   = 100
)

func NewRepository(provider store.Provider) (*Repository, error) {
	if provider == nil {
		return nil, fmt.Errorf("sso repository requires datasource provider")
	}
	var db store.SQLX
	if sqlxDB := provider.SQLX(); sqlxDB != nil {
		db = sqlxDB
	}
	dialect := strings.ToLower(strings.TrimSpace(provider.Dialect()))
	return &Repository{
		db:       db,
		postgres: dialect == "postgres" || dialect == "postgresql" || dialect == "pgx",
	}, nil
}

func (r *Repository) rebind(exec store.SQLX, query string) string {
	if r.postgres {
		query = ssoPostgresRenderer.RenderPostgres(query)
	}
	return exec.Rebind(query)
}

func (r *Repository) executor(ctx context.Context) store.SQLX {
	return store.SQLXExecutor(ctx, r.db)
}

func (r *Repository) requireExecutor(ctx context.Context) (store.SQLX, error) {
	exec := r.executor(ctx)
	if exec == nil {
		return nil, fmt.Errorf("sso repository datasource is not configured")
	}
	return exec, nil
}

func (r *Repository) CaptureManagedSessionCutoff(ctx context.Context) (time.Time, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return time.Time{}, err
	}
	var cutoff time.Time
	query := `SELECT CURRENT_TIMESTAMP(6)`
	if r.postgres {
		query = `SELECT CURRENT_TIMESTAMP`
	}
	if err := sqlx.GetContext(ctx, exec, &cutoff, r.rebind(exec, query)); err != nil {
		return time.Time{}, fmt.Errorf("capture managed session cutoff: %w", err)
	}
	return cutoff.UTC(), nil
}

type clientRow struct {
	ID                 int64          `db:"id"`
	ClientID           string         `db:"clientId"`
	ClientName         string         `db:"clientName"`
	ClientType         string         `db:"clientType"`
	ClientAuthMethod   string         `db:"clientAuthMethod"`
	GrantTypesJSON     string         `db:"grantTypesJson"`
	ScopesJSON         string         `db:"scopesJson"`
	RequirePKCE        int            `db:"requirePkce"`
	RequireConsent     int            `db:"requireConsent"`
	TrustedFirstParty  int            `db:"trustedFirstParty"`
	AccessTokenTTLSec  int            `db:"accessTokenTtlSec"`
	RefreshTokenTTLSec int            `db:"refreshTokenTtlSec"`
	Status             int            `db:"status"`
	MetadataJSON       sql.NullString `db:"metadataJson"`
}

type redirectRow struct {
	RedirectURI        string         `db:"redirectUri"`
	PostLogoutRedirect sql.NullString `db:"postLogoutRedirectUri"`
}

type secretRow struct {
	SecretHash string `db:"secretHash"`
}

type clientAdminRow struct {
	ID                  int64          `db:"id"`
	ClientID            string         `db:"clientId"`
	ClientName          string         `db:"clientName"`
	ClientType          string         `db:"clientType"`
	ClientAuthMethod    string         `db:"clientAuthMethod"`
	GrantTypesJSON      string         `db:"grantTypesJson"`
	ScopesJSON          string         `db:"scopesJson"`
	RequirePKCE         int            `db:"requirePkce"`
	RequireConsent      int            `db:"requireConsent"`
	TrustedFirstParty   int            `db:"trustedFirstParty"`
	AccessTokenTTLSec   int            `db:"accessTokenTtlSec"`
	RefreshTokenTTLSec  int            `db:"refreshTokenTtlSec"`
	Status              int            `db:"status"`
	MetadataJSON        sql.NullString `db:"metadataJson"`
	ActiveRedirectCount int            `db:"activeRedirectCount"`
	ActiveSecretCount   int            `db:"activeSecretCount"`
	CreateTime          time.Time      `db:"createTime"`
	UpdateTime          time.Time      `db:"updateTime"`
}

type redirectAdminRow struct {
	ID                    int64          `db:"id"`
	ClientID              string         `db:"clientId"`
	RedirectURI           string         `db:"redirectUri"`
	PostLogoutRedirectURI sql.NullString `db:"postLogoutRedirectUri"`
	Status                int            `db:"status"`
	CreateTime            time.Time      `db:"createTime"`
	UpdateTime            time.Time      `db:"updateTime"`
}

type secretSummaryRow struct {
	ID         int64          `db:"id"`
	ClientID   string         `db:"clientId"`
	SecretHint sql.NullString `db:"secretHint"`
	ExpiresAt  sql.NullTime   `db:"expiresAt"`
	Status     int            `db:"status"`
	CreateTime time.Time      `db:"createTime"`
	UpdateTime time.Time      `db:"updateTime"`
}

type sessionRow struct {
	ID                   int64          `db:"id"`
	SessionID            string         `db:"sessionId"`
	UserID               int64          `db:"userId"`
	ClientID             string         `db:"clientId"`
	PlatformCode         sql.NullString `db:"platformCode"`
	DeviceID             sql.NullString `db:"deviceId"`
	LoginIP              sql.NullString `db:"loginIp"`
	UserAgent            sql.NullString `db:"userAgent"`
	ACR                  sql.NullString `db:"acr"`
	AMRJSON              sql.NullString `db:"amrJson"`
	LoginMethod          sql.NullString `db:"loginMethod"`
	ExternalProviderCode sql.NullString `db:"externalProviderCode"`
	ExternalIdentityID   sql.NullInt64  `db:"externalIdentityId"`
	LoginAt              time.Time      `db:"loginAt"`
	LastAccessAt         sql.NullTime   `db:"lastAccessAt"`
	ExpiresAt            time.Time      `db:"expiresAt"`
	RevokedAt            sql.NullTime   `db:"revokedAt"`
	Status               int            `db:"status"`
	MetadataJSON         sql.NullString `db:"metadataJson"`
	CreateTime           time.Time      `db:"createTime"`
	UpdateTime           time.Time      `db:"updateTime"`
}

type codeRow struct {
	ID                  int64          `db:"id"`
	Code                string         `db:"code"`
	ClientID            string         `db:"clientId"`
	UserID              int64          `db:"userId"`
	SessionID           string         `db:"sessionId"`
	RedirectURI         string         `db:"redirectUri"`
	ScopesJSON          string         `db:"scopesJson"`
	CodeChallenge       sql.NullString `db:"codeChallenge"`
	CodeChallengeMethod sql.NullString `db:"codeChallengeMethod"`
	Nonce               sql.NullString `db:"nonce"`
	ACR                 sql.NullString `db:"acr"`
	AMRJSON             sql.NullString `db:"amrJson"`
	ExpiresAt           time.Time      `db:"expiresAt"`
	ConsumedAt          sql.NullTime   `db:"consumedAt"`
	Status              int            `db:"status"`
	MetadataJSON        sql.NullString `db:"metadataJson"`
	CreateTime          time.Time      `db:"createTime"`
	UpdateTime          time.Time      `db:"updateTime"`
}

type familyRow struct {
	ID                int64          `db:"id"`
	FamilyID          string         `db:"familyId"`
	SessionID         string         `db:"sessionId"`
	ClientID          string         `db:"clientId"`
	UserID            int64          `db:"userId"`
	CurrentTokenHash  string         `db:"currentTokenHash"`
	PreviousTokenHash sql.NullString `db:"previousTokenHash"`
	ReuseDetected     int            `db:"reuseDetected"`
	RotatedAt         sql.NullTime   `db:"rotatedAt"`
	ExpiresAt         time.Time      `db:"expiresAt"`
	RevokedAt         sql.NullTime   `db:"revokedAt"`
	Status            int            `db:"status"`
	MetadataJSON      sql.NullString `db:"metadataJson"`
	CreateTime        time.Time      `db:"createTime"`
	UpdateTime        time.Time      `db:"updateTime"`
}

type consentRow struct {
	ID           int64          `db:"id"`
	UserID       int64          `db:"userId"`
	ClientID     string         `db:"clientId"`
	ScopesJSON   string         `db:"scopesJson"`
	GrantedAt    time.Time      `db:"grantedAt"`
	RevokedAt    sql.NullTime   `db:"revokedAt"`
	Status       int            `db:"status"`
	MetadataJSON sql.NullString `db:"metadataJson"`
	CreateTime   time.Time      `db:"createTime"`
	UpdateTime   time.Time      `db:"updateTime"`
}

type auditEventRow struct {
	ID         int64          `db:"id"`
	EventType  string         `db:"eventType"`
	ClientID   sql.NullString `db:"clientId"`
	UserID     sql.NullInt64  `db:"userId"`
	SessionID  sql.NullString `db:"sessionId"`
	DeviceID   sql.NullString `db:"deviceId"`
	TenantID   sql.NullString `db:"tenantId"`
	LoginIP    sql.NullString `db:"loginIp"`
	UserAgent  sql.NullString `db:"userAgent"`
	Result     string         `db:"result"`
	ReasonCode sql.NullString `db:"reasonCode"`
	DetailJSON sql.NullString `db:"detailJson"`
	TraceID    sql.NullString `db:"traceId"`
	CreateTime time.Time      `db:"createTime"`
}

func (r *Repository) FindClient(ctx context.Context, clientID string) (*domain.Client, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return nil, err
	}
	var row clientRow
	query := r.rebind(exec, `
SELECT id, clientId, clientName, clientType, clientAuthMethod, grantTypesJson, scopesJson,
       requirePkce, requireConsent, trustedFirstParty, accessTokenTtlSec, refreshTokenTtlSec,
       status, metadataJson
FROM sys_sso_client
WHERE clientId = ? AND isDeleted = 0
LIMIT 1`)
	if err := sqlx.GetContext(ctx, exec, &row, query, clientID); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("find sso client: %w", err)
	}
	client := &domain.Client{
		ID:                 row.ID,
		ClientID:           row.ClientID,
		ClientName:         row.ClientName,
		ClientType:         row.ClientType,
		ClientAuthMethod:   row.ClientAuthMethod,
		RequirePKCE:        row.RequirePKCE == 1,
		RequireConsent:     row.RequireConsent == 1,
		TrustedFirstParty:  row.TrustedFirstParty == 1,
		AccessTokenTTLSec:  row.AccessTokenTTLSec,
		RefreshTokenTTLSec: row.RefreshTokenTTLSec,
		Status:             row.Status,
		MetadataJSON:       nullableString(row.MetadataJSON),
	}
	_ = sonic.UnmarshalString(row.GrantTypesJSON, &client.GrantTypes)
	_ = sonic.UnmarshalString(row.ScopesJSON, &client.Scopes)

	var redirects []redirectRow
	query = r.rebind(exec, `
SELECT redirectUri, postLogoutRedirectUri
FROM sys_sso_client_redirect_uri
WHERE clientId = ? AND status = 0 AND isDeleted = 0`)
	if err := sqlx.SelectContext(ctx, exec, &redirects, query, clientID); err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("list client redirect uris: %w", err)
	}
	for _, item := range redirects {
		client.RedirectURIs = append(client.RedirectURIs, item.RedirectURI)
		if item.PostLogoutRedirect.Valid && strings.TrimSpace(item.PostLogoutRedirect.String) != "" {
			client.PostLogoutRedirects = append(client.PostLogoutRedirects, strings.TrimSpace(item.PostLogoutRedirect.String))
		}
	}

	var secrets []secretRow
	query = r.rebind(exec, `
SELECT secretHash
FROM sys_sso_client_secret
WHERE clientId = ? AND status = 0 AND isDeleted = 0 AND (expiresAt IS NULL OR expiresAt > ?)`)
	if err := sqlx.SelectContext(ctx, exec, &secrets, query, clientID, time.Now().UTC()); err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("list client secrets: %w", err)
	}
	for _, item := range secrets {
		client.SecretHashes = append(client.SecretHashes, item.SecretHash)
	}
	return client, nil
}

func (r *Repository) ListEnabledClients(ctx context.Context) ([]domain.Client, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return nil, err
	}
	var rows []clientRow
	query := r.rebind(exec, `
SELECT id, clientId, clientName, clientType, clientAuthMethod, grantTypesJson, scopesJson,
       requirePkce, requireConsent, trustedFirstParty, accessTokenTtlSec, refreshTokenTtlSec,
       status, metadataJson
FROM sys_sso_client
WHERE status = ? AND isDeleted = 0
ORDER BY clientId ASC`)
	if err := sqlx.SelectContext(ctx, exec, &rows, query, domain.ClientStatusActive); err != nil {
		return nil, fmt.Errorf("list enabled sso clients: %w", err)
	}
	items := make([]domain.Client, 0, len(rows))
	for _, row := range rows {
		item := domain.Client{
			ID:                 row.ID,
			ClientID:           row.ClientID,
			ClientName:         row.ClientName,
			ClientType:         row.ClientType,
			ClientAuthMethod:   row.ClientAuthMethod,
			RequirePKCE:        row.RequirePKCE == 1,
			RequireConsent:     row.RequireConsent == 1,
			TrustedFirstParty:  row.TrustedFirstParty == 1,
			AccessTokenTTLSec:  row.AccessTokenTTLSec,
			RefreshTokenTTLSec: row.RefreshTokenTTLSec,
			Status:             row.Status,
			MetadataJSON:       nullableString(row.MetadataJSON),
		}
		_ = sonic.UnmarshalString(row.GrantTypesJSON, &item.GrantTypes)
		_ = sonic.UnmarshalString(row.ScopesJSON, &item.Scopes)
		items = append(items, item)
	}
	return items, nil
}

func (r *Repository) ListClients(ctx context.Context, query ssofacade.ClientAdminQuery) ([]domain.Client, int64, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return nil, 0, err
	}
	where, args := buildClientAdminWhere(query)
	var total int64
	countQuery := r.rebind(exec, `SELECT COUNT(*) FROM sys_sso_client c `+where)
	if err := sqlx.GetContext(ctx, exec, &total, countQuery, args...); err != nil {
		return nil, 0, fmt.Errorf("count sso clients: %w", err)
	}
	current, pageSize := normalizePage(query.Current, query.PageSize)
	listArgs := append([]any{}, args...)
	listArgs = append(listArgs, pageSize, (current-1)*pageSize)
	var rows []clientAdminRow
	listQuery := r.rebind(exec, `
SELECT c.id, c.clientId, c.clientName, c.clientType, c.clientAuthMethod, c.grantTypesJson, c.scopesJson,
       c.requirePkce, c.requireConsent, c.trustedFirstParty, c.accessTokenTtlSec, c.refreshTokenTtlSec,
       c.status, c.metadataJson,
       (SELECT COUNT(*) FROM sys_sso_client_redirect_uri r WHERE r.clientId = c.clientId AND r.status = 0 AND r.isDeleted = 0) AS activeRedirectCount,
       (SELECT COUNT(*) FROM sys_sso_client_secret s WHERE s.clientId = c.clientId AND s.status = 0 AND s.isDeleted = 0 AND (s.expiresAt IS NULL OR s.expiresAt > CURRENT_TIMESTAMP)) AS activeSecretCount,
       c.createTime, c.updateTime
FROM sys_sso_client c `+where+`
ORDER BY c.updateTime DESC, c.id DESC
LIMIT ? OFFSET ?`)
	if err := sqlx.SelectContext(ctx, exec, &rows, listQuery, listArgs...); err != nil {
		return nil, 0, fmt.Errorf("list sso clients: %w", err)
	}
	return mapAdminClients(rows), total, nil
}

func (r *Repository) FindClientDetail(ctx context.Context, clientID string) (*domain.Client, error) {
	return r.findClientDetail(ctx, clientID, false)
}

// FindClientDetailForUpdate locks one client row for managed ownership and secret mutation.
func (r *Repository) FindClientDetailForUpdate(ctx context.Context, clientID string) (*domain.Client, error) {
	return r.findClientDetail(ctx, clientID, true)
}

func (r *Repository) findClientDetail(ctx context.Context, clientID string, forUpdate bool) (*domain.Client, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return nil, err
	}
	var row clientAdminRow
	query := `
SELECT c.id, c.clientId, c.clientName, c.clientType, c.clientAuthMethod, c.grantTypesJson, c.scopesJson,
       c.requirePkce, c.requireConsent, c.trustedFirstParty, c.accessTokenTtlSec, c.refreshTokenTtlSec,
       c.status, c.metadataJson,
       (SELECT COUNT(*) FROM sys_sso_client_redirect_uri r WHERE r.clientId = c.clientId AND r.status = 0 AND r.isDeleted = 0) AS activeRedirectCount,
       (SELECT COUNT(*) FROM sys_sso_client_secret s WHERE s.clientId = c.clientId AND s.status = 0 AND s.isDeleted = 0 AND (s.expiresAt IS NULL OR s.expiresAt > CURRENT_TIMESTAMP)) AS activeSecretCount,
       c.createTime, c.updateTime
FROM sys_sso_client c
WHERE c.clientId = ? AND c.isDeleted = 0
LIMIT 1`
	if forUpdate {
		query += ` FOR UPDATE`
	}
	query = r.rebind(exec, query)
	if err := sqlx.GetContext(ctx, exec, &row, query, clientID); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("find sso client detail: %w", err)
	}
	items := mapAdminClients([]clientAdminRow{row})
	return &items[0], nil
}

func (r *Repository) InsertClient(ctx context.Context, item *domain.Client, actorID int64) error {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return err
	}
	_, err = exec.ExecContext(ctx, r.rebind(exec, `
INSERT INTO sys_sso_client (
    clientId, clientName, clientType, clientAuthMethod, grantTypesJson, scopesJson,
    requirePkce, requireConsent, trustedFirstParty, accessTokenTtlSec, refreshTokenTtlSec,
    status, metadataJson, creatorId, updaterId, isDeleted
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0)`),
		item.ClientID,
		item.ClientName,
		item.ClientType,
		item.ClientAuthMethod,
		jsonString(item.GrantTypes),
		jsonString(item.Scopes),
		boolToInt(item.RequirePKCE),
		boolToInt(item.RequireConsent),
		boolToInt(item.TrustedFirstParty),
		item.AccessTokenTTLSec,
		item.RefreshTokenTTLSec,
		item.Status,
		nullIfBlank(item.MetadataJSON),
		actorID,
		actorID,
	)
	if err != nil {
		return fmt.Errorf("insert sso client: %w", err)
	}
	return nil
}

func (r *Repository) UpdateClient(ctx context.Context, item *domain.Client, actorID int64) error {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return err
	}
	_, err = exec.ExecContext(ctx, r.rebind(exec, `
UPDATE sys_sso_client
SET clientName = ?, clientType = ?, clientAuthMethod = ?, grantTypesJson = ?, scopesJson = ?,
    requirePkce = ?, requireConsent = ?, trustedFirstParty = ?, accessTokenTtlSec = ?,
    refreshTokenTtlSec = ?, metadataJson = ?, updaterId = ?, updateTime = CURRENT_TIMESTAMP
WHERE clientId = ? AND isDeleted = 0`),
		item.ClientName,
		item.ClientType,
		item.ClientAuthMethod,
		jsonString(item.GrantTypes),
		jsonString(item.Scopes),
		boolToInt(item.RequirePKCE),
		boolToInt(item.RequireConsent),
		boolToInt(item.TrustedFirstParty),
		item.AccessTokenTTLSec,
		item.RefreshTokenTTLSec,
		nullIfBlank(item.MetadataJSON),
		actorID,
		item.ClientID,
	)
	if err != nil {
		return fmt.Errorf("update sso client: %w", err)
	}
	return nil
}

func (r *Repository) UpdateClientStatus(ctx context.Context, clientID string, status int, actorID int64, now time.Time) (bool, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return false, err
	}
	result, err := exec.ExecContext(ctx, r.rebind(exec, `
UPDATE sys_sso_client
SET status = ?, updaterId = ?, updateTime = ?
WHERE clientId = ? AND status <> ? AND isDeleted = 0`), status, actorID, now, clientID, status)
	if err != nil {
		return false, fmt.Errorf("update sso client status: %w", err)
	}
	affected, _ := result.RowsAffected()
	return affected > 0, nil
}

func (r *Repository) ReplaceClientRedirectURIs(ctx context.Context, clientID string, redirects []domain.ClientRedirectURI, actorID int64, now time.Time) error {
	if len(redirects) > ssoClientRedirectRowMax {
		return fmt.Errorf("replace sso client redirect uris: item count %d exceeds limit %d", len(redirects), ssoClientRedirectRowMax)
	}
	exec := store.SQLXFromContext(ctx)
	if exec == nil {
		return fmt.Errorf("replace sso client redirect uris requires an active transaction")
	}
	clientID = strings.TrimSpace(clientID)
	if clientID == "" {
		return fmt.Errorf("replace sso client redirect uris: client id is empty")
	}
	items := append([]domain.ClientRedirectURI(nil), redirects...)
	sort.Slice(items, func(i, j int) bool {
		if items[i].RedirectURI != items[j].RedirectURI {
			return items[i].RedirectURI < items[j].RedirectURI
		}
		return items[i].PostLogoutRedirectURI < items[j].PostLogoutRedirectURI
	})
	for index := range items {
		items[index].RedirectURI = strings.TrimSpace(items[index].RedirectURI)
		items[index].PostLogoutRedirectURI = strings.TrimSpace(items[index].PostLogoutRedirectURI)
		if items[index].RedirectURI == "" {
			return fmt.Errorf("replace sso client redirect uris: redirect uri is empty")
		}
		if index > 0 && items[index-1].RedirectURI == items[index].RedirectURI {
			return fmt.Errorf("replace sso client redirect uris: duplicate redirect uri")
		}
	}
	if _, err := exec.ExecContext(ctx, r.rebind(exec, `
DELETE FROM sys_sso_client_redirect_uri
WHERE clientId = ? AND isDeleted = 1`), clientID); err != nil {
		return fmt.Errorf("delete stale sso client redirect uris: %w", err)
	}
	_, err := exec.ExecContext(ctx, r.rebind(exec, `
UPDATE sys_sso_client_redirect_uri
SET isDeleted = 1, updaterId = ?, updateTime = ?
WHERE clientId = ? AND isDeleted = 0`), actorID, now, clientID)
	if err != nil {
		return fmt.Errorf("soft delete sso client redirect uris: %w", err)
	}
	for start := 0; start < len(items); start += ssoClientRedirectReplaceChunk {
		end := min(start+ssoClientRedirectReplaceChunk, len(items))
		chunk := items[start:end]
		tuples := make([]string, 0, len(chunk))
		args := make([]any, 0, len(chunk)*8)
		for _, redirect := range chunk {
			tuples = append(tuples, `(?, ?, ?, ?, ?, ?, ?, ?, 0)`)
			args = append(args,
				clientID,
				redirect.RedirectURI,
				nullIfBlank(redirect.PostLogoutRedirectURI),
				redirect.Status,
				actorID,
				now,
				actorID,
				now,
			)
		}
		query := `
INSERT INTO sys_sso_client_redirect_uri (
    clientId, redirectUri, postLogoutRedirectUri, status, creatorId, createTime, updaterId, updateTime, isDeleted
) VALUES ` + strings.Join(tuples, ", ")
		if _, err := exec.ExecContext(ctx, r.rebind(exec, query), args...); err != nil {
			return fmt.Errorf("insert sso client redirect uri: %w", err)
		}
	}
	return nil
}

func (r *Repository) ListClientRedirectURIs(ctx context.Context, clientID string) ([]domain.ClientRedirectURI, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return nil, err
	}
	var rows []redirectAdminRow
	query := r.rebind(exec, `
SELECT id, clientId, redirectUri, postLogoutRedirectUri, status, createTime, updateTime
FROM sys_sso_client_redirect_uri
WHERE clientId = ? AND status = ? AND isDeleted = 0
ORDER BY createTime ASC, id ASC`)
	if err := sqlx.SelectContext(ctx, exec, &rows, query, strings.TrimSpace(clientID), domain.ClientStatusActive); err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("list sso client redirect uris: %w", err)
	}
	items := make([]domain.ClientRedirectURI, 0, len(rows))
	for _, row := range rows {
		items = append(items, domain.ClientRedirectURI{
			ID:                    row.ID,
			ClientID:              row.ClientID,
			RedirectURI:           row.RedirectURI,
			PostLogoutRedirectURI: nullableString(row.PostLogoutRedirectURI),
			Status:                row.Status,
			CreateTime:            row.CreateTime,
			UpdateTime:            row.UpdateTime,
		})
	}
	return items, nil
}

func (r *Repository) ListClientSecrets(ctx context.Context, clientID string) ([]domain.ClientSecretSummary, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return nil, err
	}
	var rows []secretSummaryRow
	query := r.rebind(exec, `
SELECT id, clientId, secretHint, expiresAt, status, createTime, updateTime
FROM sys_sso_client_secret
WHERE clientId = ? AND isDeleted = 0
ORDER BY createTime DESC, id DESC`)
	if err := sqlx.SelectContext(ctx, exec, &rows, query, clientID); err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("list sso client secrets: %w", err)
	}
	return mapSecretSummaries(rows), nil
}

func (r *Repository) InsertClientSecret(ctx context.Context, item *domain.ClientSecret, actorID int64) error {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return err
	}
	if item == nil || item.ID <= 0 {
		return fmt.Errorf("insert sso client secret: invalid id")
	}
	_, err = exec.ExecContext(ctx, r.rebind(exec, `
INSERT INTO sys_sso_client_secret (
    id, clientId, secretHash, secretHint, expiresAt, status, creatorId, updaterId, isDeleted
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 0)`),
		item.ID,
		item.ClientID,
		item.SecretHash,
		nullIfBlank(item.SecretHint),
		item.ExpiresAt,
		item.Status,
		actorID,
		actorID,
	)
	if err != nil {
		return fmt.Errorf("insert sso client secret: %w", err)
	}
	return nil
}

func (r *Repository) UpdateClientSecretStatus(ctx context.Context, clientID string, secretID int64, status int, actorID int64, now time.Time) (bool, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return false, err
	}
	result, err := exec.ExecContext(ctx, r.rebind(exec, `
UPDATE sys_sso_client_secret
SET status = ?, updaterId = ?, updateTime = ?
WHERE id = ? AND clientId = ? AND status <> ? AND isDeleted = 0`), status, actorID, now, secretID, clientID, status)
	if err != nil {
		return false, fmt.Errorf("update sso client secret status: %w", err)
	}
	affected, _ := result.RowsAffected()
	return affected > 0, nil
}

// DisableOtherActiveClientSecrets disables every active secret except the newly inserted one.
func (r *Repository) DisableOtherActiveClientSecrets(ctx context.Context, clientID string, keepSecretID int64, actorID int64, now time.Time) (int64, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return 0, err
	}
	result, err := exec.ExecContext(ctx, r.rebind(exec, `
UPDATE sys_sso_client_secret
SET status = ?, updaterId = ?, updateTime = ?
WHERE clientId = ? AND id <> ? AND status = ? AND isDeleted = 0`),
		domain.ClientStatusDisabled,
		actorID,
		now,
		strings.TrimSpace(clientID),
		keepSecretID,
		domain.ClientStatusActive,
	)
	if err != nil {
		return 0, fmt.Errorf("disable previous active sso client secrets: %w", err)
	}
	affected, _ := result.RowsAffected()
	return affected, nil
}

func (r *Repository) CountActiveClientSecrets(ctx context.Context, clientID string) (int64, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return 0, err
	}
	var count int64
	query := r.rebind(exec, `
SELECT COUNT(*)
FROM sys_sso_client_secret
WHERE clientId = ? AND status = ? AND isDeleted = 0 AND (expiresAt IS NULL OR expiresAt > ?)`)
	if err := sqlx.GetContext(ctx, exec, &count, query, clientID, domain.ClientStatusActive, time.Now().UTC()); err != nil {
		return 0, fmt.Errorf("count active sso client secrets: %w", err)
	}
	return count, nil
}

func (r *Repository) InsertSession(ctx context.Context, item *domain.Session) error {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return err
	}
	amrJSON := jsonString(item.AMR)
	_, err = exec.ExecContext(ctx, r.rebind(exec, `
INSERT INTO sys_sso_session (
    sessionId, userId, clientId, platformCode, deviceId, loginIp, userAgent, acr, amrJson,
    loginMethod, externalProviderCode, externalIdentityId,
    loginAt, lastAccessAt, expiresAt, revokedAt, status, metadataJson, isDeleted
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0)`),
		item.SessionID,
		item.UserID,
		item.ClientID,
		nullIfBlank(item.PlatformCode),
		nullIfBlank(item.DeviceID),
		nullIfBlank(item.LoginIP),
		nullIfBlank(item.UserAgent),
		nullIfBlank(item.ACR),
		nullIfBlank(amrJSON),
		nullIfBlank(item.LoginMethod),
		nullIfBlank(item.ExternalProviderCode),
		nullIfZero(item.ExternalIdentityID),
		item.LoginAt,
		item.LastAccessAt,
		item.ExpiresAt,
		item.RevokedAt,
		item.Status,
		nullIfBlank(item.MetadataJSON),
	)
	if err != nil {
		return fmt.Errorf("insert sso session: %w", err)
	}
	return nil
}

func (r *Repository) FindSessionBySessionID(ctx context.Context, sessionID string) (*domain.Session, error) {
	return r.findSession(ctx, `
SELECT id, sessionId, userId, clientId, platformCode, deviceId, loginIp, userAgent, acr, amrJson,
       loginMethod, externalProviderCode, externalIdentityId,
       loginAt, lastAccessAt, expiresAt, revokedAt, status, metadataJson, createTime, updateTime
FROM sys_sso_session
WHERE sessionId = ? AND isDeleted = 0
LIMIT 1`, sessionID)
}

func (r *Repository) ListSessionsByUserID(ctx context.Context, userID int64) ([]domain.Session, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return nil, err
	}
	var rows []sessionRow
	query := r.rebind(exec, `
SELECT id, sessionId, userId, clientId, platformCode, deviceId, loginIp, userAgent, acr, amrJson,
       loginMethod, externalProviderCode, externalIdentityId,
       loginAt, lastAccessAt, expiresAt, revokedAt, status, metadataJson, createTime, updateTime
FROM sys_sso_session
WHERE userId = ? AND isDeleted = 0
ORDER BY id DESC`)
	if err := sqlx.SelectContext(ctx, exec, &rows, query, userID); err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("list sso sessions by user: %w", err)
	}
	return mapSessions(rows), nil
}

func (r *Repository) CountSessionsByUserID(ctx context.Context, userID int64) (int64, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return 0, err
	}
	var count int64
	if err := sqlx.GetContext(ctx, exec, &count, r.rebind(exec, `SELECT COUNT(*) FROM sys_sso_session WHERE userId = ? AND isDeleted = 0`), userID); err != nil {
		return 0, fmt.Errorf("count sso sessions by user: %w", err)
	}
	return count, nil
}

func (r *Repository) ListSessionsByUserIDPage(ctx context.Context, userID int64, offset, limit int) ([]domain.Session, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return nil, err
	}
	if offset < 0 {
		offset = 0
	}
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	var rows []sessionRow
	query := r.rebind(exec, `
SELECT id, sessionId, userId, clientId, platformCode, deviceId, loginIp, userAgent, acr, amrJson,
       loginMethod, externalProviderCode, externalIdentityId,
       loginAt, lastAccessAt, expiresAt, revokedAt, status, metadataJson, createTime, updateTime
FROM sys_sso_session
WHERE userId = ? AND isDeleted = 0
ORDER BY id DESC
LIMIT ? OFFSET ?`)
	if err := sqlx.SelectContext(ctx, exec, &rows, query, userID, limit, offset); err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("list sso sessions by user page: %w", err)
	}
	return mapSessions(rows), nil
}

func (r *Repository) ListActiveSessions(ctx context.Context) ([]domain.Session, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return nil, err
	}
	var rows []sessionRow
	query := r.rebind(exec, `
SELECT id, sessionId, userId, clientId, platformCode, deviceId, loginIp, userAgent, acr, amrJson,
       loginMethod, externalProviderCode, externalIdentityId,
       loginAt, lastAccessAt, expiresAt, revokedAt, status, metadataJson, createTime, updateTime
FROM sys_sso_session
WHERE status = ? AND isDeleted = 0
ORDER BY id DESC
LIMIT 200`)
	if err := sqlx.SelectContext(ctx, exec, &rows, query, domain.SessionStatusActive); err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("list active sso sessions: %w", err)
	}
	return mapSessions(rows), nil
}

func (r *Repository) ListActiveSessionsByExternalProvider(ctx context.Context, providerCode string) ([]domain.Session, error) {
	return r.ListActiveSessionsByExternalProviderPage(ctx, providerCode, time.Now().UTC(), 0, ssoSessionRevocationPageMax)
}

// ListActiveSessionsByExternalProviderPage returns one deterministic hard-capped keyset page.
func (r *Repository) ListActiveSessionsByExternalProviderPage(ctx context.Context, providerCode string, cutoff time.Time, afterID int64, limit int) ([]domain.Session, error) {
	return r.listActiveSessionsPage(ctx,
		"externalProviderCode = ?",
		[]any{strings.TrimSpace(providerCode)},
		cutoff,
		afterID,
		limit,
		"external provider",
	)
}

func (r *Repository) ListActiveSessionsByPlatformCode(ctx context.Context, platformCode string) ([]domain.Session, error) {
	return r.ListActiveSessionsByPlatformCodePage(ctx, platformCode, time.Now().UTC(), 0, ssoSessionRevocationPageMax)
}

// ListActiveSessionsByPlatformCodePage returns one deterministic hard-capped keyset page.
func (r *Repository) ListActiveSessionsByPlatformCodePage(ctx context.Context, platformCode string, cutoff time.Time, afterID int64, limit int) ([]domain.Session, error) {
	return r.listActiveSessionsPage(ctx,
		"platformCode = ?",
		[]any{strings.TrimSpace(platformCode)},
		cutoff,
		afterID,
		limit,
		"platform",
	)
}

func (r *Repository) ListActiveSessionsByPlatformLoginMethod(ctx context.Context, platformCode, loginMethod, providerCode string) ([]domain.Session, error) {
	return r.ListActiveSessionsByPlatformLoginMethodPage(ctx, platformCode, loginMethod, providerCode, time.Now().UTC(), 0, ssoSessionRevocationPageMax)
}

// ListActiveSessionsByPlatformLoginMethodPage returns one deterministic hard-capped keyset page.
func (r *Repository) ListActiveSessionsByPlatformLoginMethodPage(ctx context.Context, platformCode, loginMethod, providerCode string, cutoff time.Time, afterID int64, limit int) ([]domain.Session, error) {
	return r.listActiveSessionsPage(ctx,
		"platformCode = ? AND loginMethod = ? AND COALESCE(externalProviderCode, '') = ?",
		[]any{strings.TrimSpace(platformCode), strings.TrimSpace(loginMethod), strings.TrimSpace(providerCode)},
		cutoff,
		afterID,
		limit,
		"platform login method",
	)
}

func (r *Repository) ListActiveSessionsByExternalIdentity(ctx context.Context, identityID int64) ([]domain.Session, error) {
	return r.ListActiveSessionsByExternalIdentityPage(ctx, identityID, time.Now().UTC(), 0, ssoSessionRevocationPageMax)
}

// ListActiveSessionsByExternalIdentityPage returns one deterministic hard-capped keyset page.
func (r *Repository) ListActiveSessionsByExternalIdentityPage(ctx context.Context, identityID int64, cutoff time.Time, afterID int64, limit int) ([]domain.Session, error) {
	return r.listActiveSessionsPage(ctx,
		"externalIdentityId = ?",
		[]any{identityID},
		cutoff,
		afterID,
		limit,
		"external identity",
	)
}

// ListActiveSessionsByUserIDPage returns one deterministic hard-capped keyset page for revocation side effects.
func (r *Repository) ListActiveSessionsByUserIDPage(ctx context.Context, userID int64, cutoff time.Time, afterID int64, limit int) ([]domain.Session, error) {
	return r.listActiveSessionsPage(ctx, "userId = ?", []any{userID}, cutoff, afterID, limit, "user")
}

// ListActiveSessionsByClientIDPage returns one deterministic hard-capped keyset page for revocation side effects.
func (r *Repository) ListActiveSessionsByClientIDPage(ctx context.Context, clientID string, cutoff time.Time, afterID int64, limit int) ([]domain.Session, error) {
	return r.listActiveSessionsPage(ctx, "clientId = ?", []any{strings.TrimSpace(clientID)}, cutoff, afterID, limit, "client")
}

func (r *Repository) listActiveSessionsPage(ctx context.Context, predicate string, predicateArgs []any, cutoff time.Time, afterID int64, limit int, scope string) ([]domain.Session, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return nil, err
	}
	if afterID < 0 {
		afterID = 0
	}
	if limit <= 0 || limit > ssoSessionRevocationPageMax {
		limit = ssoSessionRevocationPageMax
	}
	var rows []sessionRow
	query := r.rebind(exec, `
SELECT id, sessionId, userId, clientId, platformCode, deviceId, loginIp, userAgent, acr, amrJson,
       loginMethod, externalProviderCode, externalIdentityId,
       loginAt, lastAccessAt, expiresAt, revokedAt, status, metadataJson, createTime, updateTime
FROM sys_sso_session
WHERE `+predicate+` AND status = ? AND isDeleted = 0 AND createTime <= ?
  AND id > ?
ORDER BY id ASC
LIMIT ?`)
	args := make([]any, 0, len(predicateArgs)+4)
	args = append(args, predicateArgs...)
	args = append(args, domain.SessionStatusActive, cutoff.UTC(), afterID, limit)
	if err := sqlx.SelectContext(ctx, exec, &rows, query, args...); err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("list active sso sessions by %s page: %w", scope, err)
	}
	return mapSessions(rows), nil
}

func (r *Repository) CountActiveSessions(ctx context.Context) (int64, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return 0, err
	}
	var count int64
	query := r.rebind(exec, `SELECT COUNT(*) FROM sys_sso_session WHERE status = ? AND isDeleted = 0`)
	if err := sqlx.GetContext(ctx, exec, &count, query, domain.SessionStatusActive); err != nil {
		return 0, fmt.Errorf("count active sso sessions: %w", err)
	}
	return count, nil
}

func (r *Repository) TouchSession(ctx context.Context, sessionID string, touchedAt time.Time) error {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return err
	}
	_, err = exec.ExecContext(ctx, r.rebind(exec, `
UPDATE sys_sso_session
SET lastAccessAt = ?, updateTime = ?
WHERE sessionId = ? AND status = ? AND isDeleted = 0`), touchedAt, touchedAt, sessionID, domain.SessionStatusActive)
	if err != nil {
		return fmt.Errorf("touch sso session: %w", err)
	}
	return nil
}

func (r *Repository) RevokeSession(ctx context.Context, sessionID string, revokedAt time.Time) (bool, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return false, err
	}
	result, err := exec.ExecContext(ctx, r.rebind(exec, `
UPDATE sys_sso_session
SET revokedAt = ?, status = ?, updateTime = ?
WHERE sessionId = ? AND status = ? AND isDeleted = 0`),
		revokedAt, domain.SessionStatusRevoked, revokedAt, sessionID, domain.SessionStatusActive)
	if err != nil {
		return false, fmt.Errorf("revoke sso session: %w", err)
	}
	affected, _ := result.RowsAffected()
	return affected > 0, nil
}

func (r *Repository) RevokeSessionsByUserID(ctx context.Context, userID int64, revokedAt time.Time) (int64, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return 0, err
	}
	result, err := exec.ExecContext(ctx, r.rebind(exec, `
UPDATE sys_sso_session
SET revokedAt = ?, status = ?, updateTime = ?
WHERE userId = ? AND createTime <= ? AND status = ? AND isDeleted = 0`),
		revokedAt, domain.SessionStatusRevoked, revokedAt, userID, revokedAt, domain.SessionStatusActive)
	if err != nil {
		return 0, fmt.Errorf("revoke sso sessions by user: %w", err)
	}
	affected, _ := result.RowsAffected()
	return affected, nil
}

func (r *Repository) RevokeSessionsByUserIDAtOrBefore(ctx context.Context, userID int64, cutoff time.Time) (int64, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return 0, err
	}
	result, err := exec.ExecContext(ctx, r.rebind(exec, `
UPDATE sys_sso_session
SET revokedAt = ?, status = ?, updateTime = ?
WHERE userId = ? AND createTime <= ? AND status = ? AND isDeleted = 0`),
		cutoff, domain.SessionStatusRevoked, cutoff, userID, cutoff, domain.SessionStatusActive)
	if err != nil {
		return 0, fmt.Errorf("revoke sso sessions by user cutoff: %w", err)
	}
	affected, _ := result.RowsAffected()
	return affected, nil
}

func (r *Repository) RevokeSessionsByClientID(ctx context.Context, clientID string, revokedAt time.Time) (int64, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return 0, err
	}
	result, err := exec.ExecContext(ctx, r.rebind(exec, `
UPDATE sys_sso_session
SET revokedAt = ?, status = ?, updateTime = ?
WHERE clientId = ? AND createTime <= ? AND status = ? AND isDeleted = 0`),
		revokedAt, domain.SessionStatusRevoked, revokedAt, clientID, revokedAt, domain.SessionStatusActive)
	if err != nil {
		return 0, fmt.Errorf("revoke sso sessions by client: %w", err)
	}
	affected, _ := result.RowsAffected()
	return affected, nil
}

func (r *Repository) RevokeSessionsByPlatformCode(ctx context.Context, platformCode string, revokedAt time.Time) (int64, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return 0, err
	}
	result, err := exec.ExecContext(ctx, r.rebind(exec, `
UPDATE sys_sso_session
SET revokedAt = ?, status = ?, updateTime = ?
WHERE platformCode = ? AND createTime <= ? AND status = ? AND isDeleted = 0`),
		revokedAt, domain.SessionStatusRevoked, revokedAt, strings.TrimSpace(platformCode), revokedAt, domain.SessionStatusActive)
	if err != nil {
		return 0, fmt.Errorf("revoke sso sessions by platform: %w", err)
	}
	affected, _ := result.RowsAffected()
	return affected, nil
}

func (r *Repository) RevokeSessionsByPlatformLoginMethod(ctx context.Context, platformCode, loginMethod, providerCode string, revokedAt time.Time) (int64, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return 0, err
	}
	result, err := exec.ExecContext(ctx, r.rebind(exec, `
UPDATE sys_sso_session
SET revokedAt = ?, status = ?, updateTime = ?
WHERE platformCode = ? AND loginMethod = ? AND COALESCE(externalProviderCode, '') = ? AND createTime <= ? AND status = ? AND isDeleted = 0`),
		revokedAt, domain.SessionStatusRevoked, revokedAt, strings.TrimSpace(platformCode), strings.TrimSpace(loginMethod), strings.TrimSpace(providerCode), revokedAt, domain.SessionStatusActive)
	if err != nil {
		return 0, fmt.Errorf("revoke sso sessions by platform login method: %w", err)
	}
	affected, _ := result.RowsAffected()
	return affected, nil
}

func (r *Repository) RevokeSessionsByExternalProvider(ctx context.Context, providerCode string, revokedAt time.Time) (int64, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return 0, err
	}
	result, err := exec.ExecContext(ctx, r.rebind(exec, `
UPDATE sys_sso_session
SET revokedAt = ?, status = ?, updateTime = ?
WHERE externalProviderCode = ? AND createTime <= ? AND status = ? AND isDeleted = 0`),
		revokedAt, domain.SessionStatusRevoked, revokedAt, strings.TrimSpace(providerCode), revokedAt, domain.SessionStatusActive)
	if err != nil {
		return 0, fmt.Errorf("revoke sso sessions by external provider: %w", err)
	}
	affected, _ := result.RowsAffected()
	return affected, nil
}

func (r *Repository) RevokeSessionsByExternalIdentity(ctx context.Context, identityID int64, revokedAt time.Time) (int64, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return 0, err
	}
	result, err := exec.ExecContext(ctx, r.rebind(exec, `
UPDATE sys_sso_session
SET revokedAt = ?, status = ?, updateTime = ?
WHERE externalIdentityId = ? AND createTime <= ? AND status = ? AND isDeleted = 0`),
		revokedAt, domain.SessionStatusRevoked, revokedAt, identityID, revokedAt, domain.SessionStatusActive)
	if err != nil {
		return 0, fmt.Errorf("revoke sso sessions by external identity: %w", err)
	}
	affected, _ := result.RowsAffected()
	return affected, nil
}

func (r *Repository) ExpireActiveSessions(ctx context.Context, now time.Time) error {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return err
	}
	_, err = exec.ExecContext(ctx, r.rebind(exec, `
UPDATE sys_sso_session
SET status = ?, updateTime = ?
WHERE status = ? AND expiresAt <= ? AND isDeleted = 0`),
		domain.SessionStatusExpired, now, domain.SessionStatusActive, now)
	if err != nil {
		return fmt.Errorf("expire sso sessions: %w", err)
	}
	return nil
}

func (r *Repository) InsertAuthorizationCode(ctx context.Context, item *domain.AuthorizationCode) error {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return err
	}
	_, err = exec.ExecContext(ctx, r.rebind(exec, `
INSERT INTO sys_sso_authorization_code (
    code, clientId, userId, sessionId, redirectUri, scopesJson, codeChallenge, codeChallengeMethod,
    nonce, acr, amrJson, expiresAt, consumedAt, status, metadataJson, isDeleted
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0)`),
		item.Code,
		item.ClientID,
		item.UserID,
		item.SessionID,
		item.RedirectURI,
		jsonString(item.Scopes),
		nullIfBlank(item.CodeChallenge),
		nullIfBlank(item.CodeChallengeMethod),
		nullIfBlank(item.Nonce),
		nullIfBlank(item.ACR),
		nullIfBlank(jsonString(item.AMR)),
		item.ExpiresAt,
		item.ConsumedAt,
		item.Status,
		nullIfBlank(item.MetadataJSON),
	)
	if err != nil {
		return fmt.Errorf("insert sso authorization code: %w", err)
	}
	return nil
}

func (r *Repository) FindAuthorizationCode(ctx context.Context, code string) (*domain.AuthorizationCode, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return nil, err
	}
	var row codeRow
	query := r.rebind(exec, `
SELECT id, code, clientId, userId, sessionId, redirectUri, scopesJson, codeChallenge, codeChallengeMethod,
       nonce, acr, amrJson, expiresAt, consumedAt, status, metadataJson, createTime, updateTime
FROM sys_sso_authorization_code
WHERE code = ? AND isDeleted = 0
LIMIT 1`)
	if err := sqlx.GetContext(ctx, exec, &row, query, code); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("find sso authorization code: %w", err)
	}
	return mapCode(row), nil
}

func (r *Repository) ConsumeAuthorizationCode(ctx context.Context, code string, consumedAt time.Time) (bool, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return false, err
	}
	result, err := exec.ExecContext(ctx, r.rebind(exec, `
UPDATE sys_sso_authorization_code
SET consumedAt = ?, status = ?, updateTime = ?
WHERE code = ? AND status = ? AND isDeleted = 0`),
		consumedAt, domain.CodeStatusConsumed, consumedAt, code, domain.CodeStatusActive)
	if err != nil {
		return false, fmt.Errorf("consume authorization code: %w", err)
	}
	affected, _ := result.RowsAffected()
	return affected > 0, nil
}

func (r *Repository) ExpireAuthorizationCodes(ctx context.Context, now time.Time) error {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return err
	}
	_, err = exec.ExecContext(ctx, r.rebind(exec, `
UPDATE sys_sso_authorization_code
SET status = ?, updateTime = ?
WHERE status = ? AND expiresAt <= ? AND isDeleted = 0`),
		domain.CodeStatusExpired, now, domain.CodeStatusActive, now)
	if err != nil {
		return fmt.Errorf("expire authorization codes: %w", err)
	}
	return nil
}

func (r *Repository) InsertRefreshTokenFamily(ctx context.Context, item *domain.RefreshTokenFamily) error {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return err
	}
	_, err = exec.ExecContext(ctx, r.rebind(exec, `
INSERT INTO sys_sso_refresh_token_family (
    familyId, sessionId, clientId, userId, currentTokenHash, previousTokenHash, reuseDetected,
    rotatedAt, expiresAt, revokedAt, status, metadataJson, isDeleted
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0)`),
		item.FamilyID,
		item.SessionID,
		item.ClientID,
		item.UserID,
		item.CurrentTokenHash,
		nullIfBlank(item.PreviousTokenHash),
		boolToInt(item.ReuseDetected),
		item.RotatedAt,
		item.ExpiresAt,
		item.RevokedAt,
		item.Status,
		nullIfBlank(item.MetadataJSON),
	)
	if err != nil {
		return fmt.Errorf("insert refresh token family: %w", err)
	}
	return nil
}

func (r *Repository) FindRefreshFamilyByCurrentHash(ctx context.Context, hash string) (*domain.RefreshTokenFamily, error) {
	return r.findFamily(ctx, `SELECT id, familyId, sessionId, clientId, userId, currentTokenHash, previousTokenHash, reuseDetected,
       rotatedAt, expiresAt, revokedAt, status, metadataJson, createTime, updateTime
FROM sys_sso_refresh_token_family WHERE currentTokenHash = ? AND isDeleted = 0 LIMIT 1`, hash)
}

func (r *Repository) FindRefreshFamilyByPreviousHash(ctx context.Context, hash string) (*domain.RefreshTokenFamily, error) {
	return r.findFamily(ctx, `SELECT id, familyId, sessionId, clientId, userId, currentTokenHash, previousTokenHash, reuseDetected,
       rotatedAt, expiresAt, revokedAt, status, metadataJson, createTime, updateTime
FROM sys_sso_refresh_token_family WHERE previousTokenHash = ? AND isDeleted = 0 LIMIT 1`, hash)
}

func (r *Repository) RotateRefreshFamily(ctx context.Context, familyID, previousHash, nextHash string, rotatedAt time.Time) (bool, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return false, err
	}
	result, err := exec.ExecContext(ctx, r.rebind(exec, `
UPDATE sys_sso_refresh_token_family
SET previousTokenHash = ?, currentTokenHash = ?, rotatedAt = ?, updateTime = ?
WHERE familyId = ? AND currentTokenHash = ? AND status = ? AND isDeleted = 0`),
		previousHash, nextHash, rotatedAt, rotatedAt, familyID, previousHash, domain.RefreshFamilyStatusActive)
	if err != nil {
		return false, fmt.Errorf("rotate refresh family: %w", err)
	}
	affected, _ := result.RowsAffected()
	return affected > 0, nil
}

func (r *Repository) MarkRefreshFamilyReuseDetected(ctx context.Context, familyID string, revokedAt time.Time) error {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return err
	}
	_, err = exec.ExecContext(ctx, r.rebind(exec, `
UPDATE sys_sso_refresh_token_family
SET reuseDetected = 1, revokedAt = ?, status = ?, updateTime = ?
WHERE familyId = ? AND isDeleted = 0`),
		revokedAt, domain.RefreshFamilyStatusRevoked, revokedAt, familyID)
	if err != nil {
		return fmt.Errorf("mark refresh family reuse detected: %w", err)
	}
	return nil
}

func (r *Repository) RevokeRefreshFamiliesBySessionID(ctx context.Context, sessionID string, revokedAt time.Time) error {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return err
	}
	_, err = exec.ExecContext(ctx, r.rebind(exec, `
UPDATE sys_sso_refresh_token_family
SET revokedAt = ?, status = ?, updateTime = ?
WHERE sessionId = ? AND status = ? AND isDeleted = 0`),
		revokedAt, domain.RefreshFamilyStatusRevoked, revokedAt, sessionID, domain.RefreshFamilyStatusActive)
	if err != nil {
		return fmt.Errorf("revoke refresh families by session: %w", err)
	}
	return nil
}

func (r *Repository) RevokeRefreshFamiliesByExternalProvider(ctx context.Context, providerCode string, revokedAt time.Time) error {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return err
	}
	_, err = exec.ExecContext(ctx, r.rebind(exec, `
UPDATE sys_sso_refresh_token_family
SET revokedAt = ?, status = ?, updateTime = ?
WHERE status = ? AND isDeleted = 0
  AND sessionId IN (
    SELECT sessionId
    FROM sys_sso_session
    WHERE externalProviderCode = ? AND createTime <= ? AND status = ? AND isDeleted = 0
  )`),
		revokedAt, domain.RefreshFamilyStatusRevoked, revokedAt,
		domain.RefreshFamilyStatusActive, strings.TrimSpace(providerCode), revokedAt, domain.SessionStatusActive)
	if err != nil {
		return fmt.Errorf("revoke sso refresh families by external provider: %w", err)
	}
	return nil
}

func (r *Repository) RevokeRefreshFamiliesByPlatformCode(ctx context.Context, platformCode string, revokedAt time.Time) error {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return err
	}
	_, err = exec.ExecContext(ctx, r.rebind(exec, `
UPDATE sys_sso_refresh_token_family
SET revokedAt = ?, status = ?, updateTime = ?
WHERE status = ? AND isDeleted = 0
  AND sessionId IN (
    SELECT sessionId
    FROM sys_sso_session
    WHERE platformCode = ? AND createTime <= ? AND status = ? AND isDeleted = 0
  )`),
		revokedAt, domain.RefreshFamilyStatusRevoked, revokedAt,
		domain.RefreshFamilyStatusActive, strings.TrimSpace(platformCode), revokedAt, domain.SessionStatusActive)
	if err != nil {
		return fmt.Errorf("revoke sso refresh families by platform: %w", err)
	}
	return nil
}

func (r *Repository) RevokeRefreshFamiliesByPlatformLoginMethod(ctx context.Context, platformCode, loginMethod, providerCode string, revokedAt time.Time) error {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return err
	}
	_, err = exec.ExecContext(ctx, r.rebind(exec, `
UPDATE sys_sso_refresh_token_family
SET revokedAt = ?, status = ?, updateTime = ?
WHERE status = ? AND isDeleted = 0
  AND sessionId IN (
    SELECT sessionId
    FROM sys_sso_session
    WHERE platformCode = ? AND loginMethod = ? AND COALESCE(externalProviderCode, '') = ? AND createTime <= ? AND status = ? AND isDeleted = 0
  )`),
		revokedAt, domain.RefreshFamilyStatusRevoked, revokedAt,
		domain.RefreshFamilyStatusActive, strings.TrimSpace(platformCode), strings.TrimSpace(loginMethod), strings.TrimSpace(providerCode), revokedAt, domain.SessionStatusActive)
	if err != nil {
		return fmt.Errorf("revoke sso refresh families by platform login method: %w", err)
	}
	return nil
}

func (r *Repository) RevokeRefreshFamiliesByExternalIdentity(ctx context.Context, identityID int64, revokedAt time.Time) error {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return err
	}
	_, err = exec.ExecContext(ctx, r.rebind(exec, `
UPDATE sys_sso_refresh_token_family
SET revokedAt = ?, status = ?, updateTime = ?
WHERE status = ? AND isDeleted = 0
  AND sessionId IN (
    SELECT sessionId
    FROM sys_sso_session
    WHERE externalIdentityId = ? AND createTime <= ? AND status = ? AND isDeleted = 0
  )`),
		revokedAt, domain.RefreshFamilyStatusRevoked, revokedAt,
		domain.RefreshFamilyStatusActive, identityID, revokedAt, domain.SessionStatusActive)
	if err != nil {
		return fmt.Errorf("revoke sso refresh families by external identity: %w", err)
	}
	return nil
}

func (r *Repository) RevokeRefreshFamiliesByUserID(ctx context.Context, userID int64, revokedAt time.Time) error {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return err
	}
	_, err = exec.ExecContext(ctx, r.rebind(exec, `
UPDATE sys_sso_refresh_token_family
SET revokedAt = ?, status = ?, updateTime = ?
WHERE userId = ? AND createTime <= ? AND status = ? AND isDeleted = 0`),
		revokedAt, domain.RefreshFamilyStatusRevoked, revokedAt, userID, revokedAt, domain.RefreshFamilyStatusActive)
	if err != nil {
		return fmt.Errorf("revoke refresh families by user: %w", err)
	}
	return nil
}

func (r *Repository) RevokeRefreshFamiliesByUserIDAtOrBefore(ctx context.Context, userID int64, cutoff time.Time) error {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return err
	}
	_, err = exec.ExecContext(ctx, r.rebind(exec, `
UPDATE sys_sso_refresh_token_family
SET revokedAt = ?, status = ?, updateTime = ?
WHERE userId = ? AND createTime <= ? AND status = ? AND isDeleted = 0`),
		cutoff, domain.RefreshFamilyStatusRevoked, cutoff, userID, cutoff, domain.RefreshFamilyStatusActive)
	if err != nil {
		return fmt.Errorf("revoke refresh families by user cutoff: %w", err)
	}
	return nil
}

func (r *Repository) ExpireRefreshFamilies(ctx context.Context, now time.Time) error {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return err
	}
	_, err = exec.ExecContext(ctx, r.rebind(exec, `
UPDATE sys_sso_refresh_token_family
SET status = ?, updateTime = ?
WHERE status = ? AND expiresAt <= ? AND isDeleted = 0`),
		domain.RefreshFamilyStatusExpired, now, domain.RefreshFamilyStatusActive, now)
	if err != nil {
		return fmt.Errorf("expire refresh families: %w", err)
	}
	return nil
}

func (r *Repository) FindConsentGrant(ctx context.Context, userID int64, clientID string) (*domain.ConsentGrant, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return nil, err
	}
	var row consentRow
	query := r.rebind(exec, `
SELECT id, userId, clientId, scopesJson, grantedAt, revokedAt, status, metadataJson, createTime, updateTime
FROM sys_sso_consent_grant
WHERE userId = ? AND clientId = ? AND isDeleted = 0
LIMIT 1`)
	if err := sqlx.GetContext(ctx, exec, &row, query, userID, clientID); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("find consent grant: %w", err)
	}
	return mapConsent(row), nil
}

func (r *Repository) UpsertConsentGrant(ctx context.Context, item *domain.ConsentGrant) error {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return err
	}
	query := `
INSERT INTO sys_sso_consent_grant (
    userId, clientId, scopesJson, grantedAt, revokedAt, status, metadataJson, isDeleted
) VALUES (?, ?, ?, ?, ?, ?, ?, 0)
ON DUPLICATE KEY UPDATE
    scopesJson = VALUES(scopesJson),
    grantedAt = VALUES(grantedAt),
    revokedAt = VALUES(revokedAt),
    status = VALUES(status),
    metadataJson = VALUES(metadataJson),
    isDeleted = 0`
	if r.postgres {
		query = `
INSERT INTO sys_sso_consent_grant (
    userId, clientId, scopesJson, grantedAt, revokedAt, status, metadataJson, isDeleted
) VALUES (?, ?, ?, ?, ?, ?, ?, 0)
ON CONFLICT (userId, clientId, isDeleted) DO UPDATE SET
    scopesJson = EXCLUDED.scopesJson,
    grantedAt = EXCLUDED.grantedAt,
    revokedAt = EXCLUDED.revokedAt,
    status = EXCLUDED.status,
    metadataJson = EXCLUDED.metadataJson,
    isDeleted = 0`
	}
	query = r.rebind(exec, query)
	_, err = exec.ExecContext(ctx, query,
		item.UserID,
		item.ClientID,
		jsonString(item.Scopes),
		item.GrantedAt,
		item.RevokedAt,
		item.Status,
		nullIfBlank(item.MetadataJSON),
	)
	if err != nil {
		return fmt.Errorf("upsert consent grant: %w", err)
	}
	return nil
}

func (r *Repository) InsertAuditLog(ctx context.Context, item domain.AuditLog) error {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return err
	}
	query := r.rebind(exec, `
INSERT INTO sys_sso_audit_log (
    eventType, clientId, userId, sessionId, deviceId, tenantId, loginIp, userAgent,
    result, reasonCode, detailJson, traceId, isDeleted
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0)`)
	_, err = exec.ExecContext(ctx, query,
		item.EventType,
		nullIfBlank(item.ClientID),
		item.UserID,
		nullIfBlank(item.SessionID),
		nullIfBlank(item.DeviceID),
		nullIfBlank(item.TenantID),
		nullIfBlank(item.LoginIP),
		nullIfBlank(item.UserAgent),
		nullIfBlank(item.Result),
		nullIfBlank(item.ReasonCode),
		nullIfBlank(item.DetailJSON),
		nullIfBlank(item.TraceID),
	)
	if err != nil {
		return fmt.Errorf("insert sso audit log: %w", err)
	}
	return nil
}

func (r *Repository) ListAuditEventsSince(ctx context.Context, startTime time.Time) ([]domain.AuditEvent, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return nil, err
	}
	var rows []auditEventRow
	query := r.rebind(exec, `
SELECT id, eventType, clientId, userId, sessionId, deviceId, tenantId, loginIp, userAgent,
       result, reasonCode, detailJson, traceId, createTime
FROM sys_sso_audit_log
WHERE createTime >= ? AND isDeleted = 0
ORDER BY createTime DESC, id DESC
LIMIT 5000`)
	if err := sqlx.SelectContext(ctx, exec, &rows, query, startTime); err != nil {
		return nil, fmt.Errorf("list sso audit events since: %w", err)
	}
	items := make([]domain.AuditEvent, 0, len(rows))
	for _, row := range rows {
		item := domain.AuditEvent{
			ID:         row.ID,
			EventType:  row.EventType,
			ClientID:   nullableString(row.ClientID),
			SessionID:  nullableString(row.SessionID),
			DeviceID:   nullableString(row.DeviceID),
			TenantID:   nullableString(row.TenantID),
			LoginIP:    nullableString(row.LoginIP),
			UserAgent:  nullableString(row.UserAgent),
			Result:     row.Result,
			ReasonCode: nullableString(row.ReasonCode),
			DetailJSON: nullableString(row.DetailJSON),
			TraceID:    nullableString(row.TraceID),
			CreatedAt:  row.CreateTime,
		}
		if row.UserID.Valid {
			value := row.UserID.Int64
			item.UserID = &value
		}
		items = append(items, item)
	}
	return items, nil
}

func (r *Repository) findSession(ctx context.Context, query string, args ...any) (*domain.Session, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return nil, err
	}
	var row sessionRow
	if err := sqlx.GetContext(ctx, exec, &row, r.rebind(exec, query), args...); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("find sso session: %w", err)
	}
	items := mapSessions([]sessionRow{row})
	if len(items) == 0 {
		return nil, nil
	}
	return &items[0], nil
}

func (r *Repository) findFamily(ctx context.Context, query string, hash string) (*domain.RefreshTokenFamily, error) {
	exec, err := r.requireExecutor(ctx)
	if err != nil {
		return nil, err
	}
	var row familyRow
	if err := sqlx.GetContext(ctx, exec, &row, r.rebind(exec, query), hash); err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("find refresh family: %w", err)
	}
	return mapFamily(row), nil
}

func mapSessions(rows []sessionRow) []domain.Session {
	items := make([]domain.Session, 0, len(rows))
	for _, row := range rows {
		var amr []string
		if row.AMRJSON.Valid && strings.TrimSpace(row.AMRJSON.String) != "" {
			_ = sonic.UnmarshalString(row.AMRJSON.String, &amr)
		}
		item := domain.Session{
			ID:                   row.ID,
			SessionID:            row.SessionID,
			UserID:               row.UserID,
			ClientID:             row.ClientID,
			PlatformCode:         nullableString(row.PlatformCode),
			DeviceID:             nullableString(row.DeviceID),
			LoginIP:              nullableString(row.LoginIP),
			UserAgent:            nullableString(row.UserAgent),
			ACR:                  nullableString(row.ACR),
			AMR:                  amr,
			LoginMethod:          nullableString(row.LoginMethod),
			ExternalProviderCode: nullableString(row.ExternalProviderCode),
			LoginAt:              row.LoginAt,
			ExpiresAt:            row.ExpiresAt,
			Status:               row.Status,
			MetadataJSON:         nullableString(row.MetadataJSON),
			CreateTime:           row.CreateTime,
			UpdateTime:           row.UpdateTime,
		}
		if row.LastAccessAt.Valid {
			item.LastAccessAt = &row.LastAccessAt.Time
		}
		if row.RevokedAt.Valid {
			item.RevokedAt = &row.RevokedAt.Time
		}
		if row.ExternalIdentityID.Valid {
			item.ExternalIdentityID = row.ExternalIdentityID.Int64
		}
		items = append(items, item)
	}
	return items
}

func mapCode(row codeRow) *domain.AuthorizationCode {
	var scopes []string
	_ = sonic.UnmarshalString(row.ScopesJSON, &scopes)
	var amr []string
	if row.AMRJSON.Valid && strings.TrimSpace(row.AMRJSON.String) != "" {
		_ = sonic.UnmarshalString(row.AMRJSON.String, &amr)
	}
	item := &domain.AuthorizationCode{
		ID:                  row.ID,
		Code:                row.Code,
		ClientID:            row.ClientID,
		UserID:              row.UserID,
		SessionID:           row.SessionID,
		RedirectURI:         row.RedirectURI,
		Scopes:              scopes,
		CodeChallenge:       nullableString(row.CodeChallenge),
		CodeChallengeMethod: nullableString(row.CodeChallengeMethod),
		Nonce:               nullableString(row.Nonce),
		ACR:                 nullableString(row.ACR),
		AMR:                 amr,
		ExpiresAt:           row.ExpiresAt,
		Status:              row.Status,
		MetadataJSON:        nullableString(row.MetadataJSON),
		CreateTime:          row.CreateTime,
		UpdateTime:          row.UpdateTime,
	}
	if row.ConsumedAt.Valid {
		item.ConsumedAt = &row.ConsumedAt.Time
	}
	return item
}

func mapFamily(row familyRow) *domain.RefreshTokenFamily {
	item := &domain.RefreshTokenFamily{
		ID:                row.ID,
		FamilyID:          row.FamilyID,
		SessionID:         row.SessionID,
		ClientID:          row.ClientID,
		UserID:            row.UserID,
		CurrentTokenHash:  row.CurrentTokenHash,
		PreviousTokenHash: nullableString(row.PreviousTokenHash),
		ReuseDetected:     row.ReuseDetected == 1,
		ExpiresAt:         row.ExpiresAt,
		Status:            row.Status,
		MetadataJSON:      nullableString(row.MetadataJSON),
		CreateTime:        row.CreateTime,
		UpdateTime:        row.UpdateTime,
	}
	if row.RotatedAt.Valid {
		item.RotatedAt = &row.RotatedAt.Time
	}
	if row.RevokedAt.Valid {
		item.RevokedAt = &row.RevokedAt.Time
	}
	return item
}

func mapConsent(row consentRow) *domain.ConsentGrant {
	var scopes []string
	_ = sonic.UnmarshalString(row.ScopesJSON, &scopes)
	item := &domain.ConsentGrant{
		ID:           row.ID,
		UserID:       row.UserID,
		ClientID:     row.ClientID,
		Scopes:       scopes,
		GrantedAt:    row.GrantedAt,
		Status:       row.Status,
		MetadataJSON: nullableString(row.MetadataJSON),
		CreateTime:   row.CreateTime,
		UpdateTime:   row.UpdateTime,
	}
	if row.RevokedAt.Valid {
		item.RevokedAt = &row.RevokedAt.Time
	}
	return item
}

func buildClientAdminWhere(query ssofacade.ClientAdminQuery) (string, []any) {
	conditions := []string{"WHERE c.isDeleted = 0"}
	args := make([]any, 0, 3)
	if query.Status != nil {
		conditions = append(conditions, "c.status = ?")
		args = append(args, *query.Status)
	}
	if strings.TrimSpace(query.ClientType) != "" {
		conditions = append(conditions, "c.clientType = ?")
		args = append(args, strings.TrimSpace(query.ClientType))
	}
	if strings.TrimSpace(query.Keyword) != "" {
		keyword := "%" + strings.TrimSpace(query.Keyword) + "%"
		conditions = append(conditions, "(c.clientId LIKE ? OR c.clientName LIKE ?)")
		args = append(args, keyword, keyword)
	}
	return strings.Join(conditions, " AND "), args
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

func mapAdminClients(rows []clientAdminRow) []domain.Client {
	items := make([]domain.Client, 0, len(rows))
	for _, row := range rows {
		item := domain.Client{
			ID:                  row.ID,
			ClientID:            row.ClientID,
			ClientName:          row.ClientName,
			ClientType:          row.ClientType,
			ClientAuthMethod:    row.ClientAuthMethod,
			RequirePKCE:         row.RequirePKCE == 1,
			RequireConsent:      row.RequireConsent == 1,
			TrustedFirstParty:   row.TrustedFirstParty == 1,
			AccessTokenTTLSec:   row.AccessTokenTTLSec,
			RefreshTokenTTLSec:  row.RefreshTokenTTLSec,
			Status:              row.Status,
			MetadataJSON:        nullableString(row.MetadataJSON),
			ActiveRedirectCount: row.ActiveRedirectCount,
			ActiveSecretCount:   row.ActiveSecretCount,
			CreateTime:          row.CreateTime,
			UpdateTime:          row.UpdateTime,
		}
		_ = sonic.UnmarshalString(row.GrantTypesJSON, &item.GrantTypes)
		_ = sonic.UnmarshalString(row.ScopesJSON, &item.Scopes)
		items = append(items, item)
	}
	return items
}

func mapSecretSummaries(rows []secretSummaryRow) []domain.ClientSecretSummary {
	items := make([]domain.ClientSecretSummary, 0, len(rows))
	for _, row := range rows {
		item := domain.ClientSecretSummary{
			ID:         row.ID,
			ClientID:   row.ClientID,
			SecretHint: nullableString(row.SecretHint),
			Status:     row.Status,
			CreateTime: row.CreateTime,
			UpdateTime: row.UpdateTime,
		}
		if row.ExpiresAt.Valid {
			item.ExpiresAt = &row.ExpiresAt.Time
		}
		items = append(items, item)
	}
	return items
}

func nullableString(value sql.NullString) string {
	if value.Valid {
		return strings.TrimSpace(value.String)
	}
	return ""
}

func nullIfBlank(value string) any {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return strings.TrimSpace(value)
}

func nullIfZero(value int64) any {
	if value == 0 {
		return nil
	}
	return value
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
