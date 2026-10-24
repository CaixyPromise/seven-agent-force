package infrastructure

import (
	"context"
	"database/sql"
	"regexp"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/sso/domain"
	ssofacade "github.com/CaixyPromise/seven-agent/force/internal/app/sso/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/store"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
)

type nilSQLXProvider struct{}

func (nilSQLXProvider) Driver() string               { return "mysql" }
func (nilSQLXProvider) Dialect() string              { return "mysql" }
func (nilSQLXProvider) DB() *sql.DB                  { return nil }
func (nilSQLXProvider) SQLX() *sqlx.DB               { return nil }
func (nilSQLXProvider) Close() error                 { return nil }
func (nilSQLXProvider) Transactor() store.Transactor { return nil }
func (nilSQLXProvider) Configured() bool             { return false }

func TestFindClientWithoutDatasourceReturnsError(t *testing.T) {
	repo, err := NewRepository(nilSQLXProvider{})
	if err != nil {
		t.Fatalf("NewRepository() error = %v", err)
	}
	if _, err := repo.FindClient(context.Background(), "seven-first-party"); err == nil {
		t.Fatal("expected datasource error when SQLX is unavailable")
	}
}

func TestRepositoryListClientsFiltersAndCounts(t *testing.T) {
	db, mock, repo := newSSORepositoryMock(t)
	defer db.Close()
	now := time.Date(2026, 6, 18, 10, 0, 0, 0, time.UTC)
	status := domain.ClientStatusActive

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT COUNT(*) FROM sys_sso_client c WHERE c.isDeleted = 0 AND c.status = ? AND c.clientType = ? AND (c.clientId LIKE ? OR c.clientName LIKE ?)`)).
		WithArgs(status, "PUBLIC", "%console%", "%console%").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(`SELECT c\.id, c\.clientId, c\.clientName, c\.clientType`).
		WithArgs(status, "PUBLIC", "%console%", "%console%", 10, 10).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "clientId", "clientName", "clientType", "clientAuthMethod", "grantTypesJson", "scopesJson",
			"requirePkce", "requireConsent", "trustedFirstParty", "accessTokenTtlSec", "refreshTokenTtlSec",
			"status", "metadataJson", "activeRedirectCount", "activeSecretCount", "createTime", "updateTime",
		}).AddRow(
			int64(7), "authorization-console", "Authorization Console", "PUBLIC", "none",
			`["authorization_code","refresh_token"]`, `["openid","profile"]`,
			1, 0, 1, 1800, 2592000, domain.ClientStatusActive, `{"seed":true}`, 2, 1, now, now,
		))

	items, total, err := repo.ListClients(context.Background(), ssofacade.ClientAdminQuery{
		Keyword:    "console",
		Status:     &status,
		ClientType: "PUBLIC",
		Current:    2,
		PageSize:   10,
	})
	if err != nil {
		t.Fatalf("list clients: %v", err)
	}
	if total != 1 || len(items) != 1 {
		t.Fatalf("expected one client and total=1, got total=%d items=%d", total, len(items))
	}
	item := items[0]
	if item.ClientID != "authorization-console" || !item.RequirePKCE || !item.TrustedFirstParty {
		t.Fatalf("unexpected client projection: %#v", item)
	}
	if item.ActiveRedirectCount != 2 || item.ActiveSecretCount != 1 {
		t.Fatalf("unexpected active counts: redirects=%d secrets=%d", item.ActiveRedirectCount, item.ActiveSecretCount)
	}
	assertSQLExpectations(t, mock)
}

func TestRepositoryInsertClientStoresJSONFields(t *testing.T) {
	db, mock, repo := newSSORepositoryMock(t)
	defer db.Close()

	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO sys_sso_client (
    clientId, clientName, clientType, clientAuthMethod, grantTypesJson, scopesJson,
    requirePkce, requireConsent, trustedFirstParty, accessTokenTtlSec, refreshTokenTtlSec,
    status, metadataJson, creatorId, updaterId, isDeleted
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0)`)).
		WithArgs(
			"demo-client", "Demo Client", "PUBLIC", "none",
			`["authorization_code","refresh_token"]`, `["openid","email"]`,
			1, 0, 1, 1800, 2592000, domain.ClientStatusActive, `{"owner":"security"}`, int64(100), int64(100),
		).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := repo.InsertClient(context.Background(), &domain.Client{
		ClientID:           "demo-client",
		ClientName:         "Demo Client",
		ClientType:         "PUBLIC",
		ClientAuthMethod:   "none",
		GrantTypes:         []string{"authorization_code", "refresh_token"},
		Scopes:             []string{"openid", "email"},
		RequirePKCE:        true,
		TrustedFirstParty:  true,
		AccessTokenTTLSec:  1800,
		RefreshTokenTTLSec: 2592000,
		Status:             domain.ClientStatusActive,
		MetadataJSON:       `{"owner":"security"}`,
	}, 100)
	if err != nil {
		t.Fatalf("insert client: %v", err)
	}
	assertSQLExpectations(t, mock)
}

func TestRepositoryReplaceClientRedirectURIsSoftDeletesOldRows(t *testing.T) {
	db, mock, repo := newSSORepositoryMock(t)
	defer db.Close()
	now := time.Date(2026, 6, 18, 11, 0, 0, 0, time.UTC)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM sys_sso_client_redirect_uri
WHERE clientId = ? AND isDeleted = 1`)).
		WithArgs("demo-client").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE sys_sso_client_redirect_uri
SET isDeleted = 1, updaterId = ?, updateTime = ?
WHERE clientId = ? AND isDeleted = 0`)).
		WithArgs(int64(200), now, "demo-client").
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO sys_sso_client_redirect_uri (
    clientId, redirectUri, postLogoutRedirectUri, status, creatorId, createTime, updaterId, updateTime, isDeleted
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 0)`)).
		WithArgs("demo-client", "http://127.0.0.1:8080/callback", "http://127.0.0.1:8080/logout", domain.ClientStatusActive, int64(200), now, int64(200), now).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	sqlxDB := repo.db.(*sqlx.DB)
	err := store.NewSQLXTransactor(sqlxDB).WithinTransaction(context.Background(), func(txCtx context.Context) error {
		return repo.ReplaceClientRedirectURIs(txCtx, "demo-client", []domain.ClientRedirectURI{{
			RedirectURI:           "http://127.0.0.1:8080/callback",
			PostLogoutRedirectURI: "http://127.0.0.1:8080/logout",
			Status:                domain.ClientStatusActive,
		}}, 200, now)
	})
	if err != nil {
		t.Fatalf("replace redirect uris: %v", err)
	}
	assertSQLExpectations(t, mock)
}

func TestRepositoryListClientRedirectURIsReturnsActiveRowsOnly(t *testing.T) {
	db, mock, repo := newSSORepositoryMock(t)
	defer db.Close()
	now := time.Date(2026, 6, 18, 11, 30, 0, 0, time.UTC)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, clientId, redirectUri, postLogoutRedirectUri, status, createTime, updateTime
FROM sys_sso_client_redirect_uri
WHERE clientId = ? AND status = ? AND isDeleted = 0
ORDER BY createTime ASC, id ASC`)).
		WithArgs("demo-client", domain.ClientStatusActive).
		WillReturnRows(sqlmock.NewRows([]string{"id", "clientId", "redirectUri", "postLogoutRedirectUri", "status", "createTime", "updateTime"}).
			AddRow(int64(10), "demo-client", "https://demo.example/callback", "https://demo.example/logout", domain.ClientStatusActive, now, now))

	items, err := repo.ListClientRedirectURIs(context.Background(), " demo-client ")
	if err != nil {
		t.Fatalf("list redirect uris: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected one active redirect, got %d", len(items))
	}
	if items[0].RedirectURI != "https://demo.example/callback" || items[0].PostLogoutRedirectURI != "https://demo.example/logout" {
		t.Fatalf("unexpected redirect projection: %#v", items[0])
	}
	assertSQLExpectations(t, mock)
}

func TestRepositoryInsertClientSecretNeverReturnsHashInSummary(t *testing.T) {
	db, mock, repo := newSSORepositoryMock(t)
	defer db.Close()
	expiresAt := time.Date(2026, 7, 18, 0, 0, 0, 0, time.UTC)
	now := time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC)

	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO sys_sso_client_secret (
    id, clientId, secretHash, secretHint, expiresAt, status, creatorId, updaterId, isDeleted
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, 0)`)).
		WithArgs(int64(9001), "demo-client", "argon2id$hash", "sec_****1234", &expiresAt, domain.ClientStatusActive, int64(300), int64(300)).
		WillReturnResult(sqlmock.NewResult(1, 1))
	if err := repo.InsertClientSecret(context.Background(), &domain.ClientSecret{
		ID:         9001,
		ClientID:   "demo-client",
		SecretHash: "argon2id$hash",
		SecretHint: "sec_****1234",
		ExpiresAt:  &expiresAt,
		Status:     domain.ClientStatusActive,
	}, 300); err != nil {
		t.Fatalf("insert client secret: %v", err)
	}

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT id, clientId, secretHint, expiresAt, status, createTime, updateTime
FROM sys_sso_client_secret
WHERE clientId = ? AND isDeleted = 0
ORDER BY createTime DESC, id DESC`)).
		WithArgs("demo-client").
		WillReturnRows(sqlmock.NewRows([]string{"id", "clientId", "secretHint", "expiresAt", "status", "createTime", "updateTime"}).
			AddRow(int64(1), "demo-client", "sec_****1234", expiresAt, domain.ClientStatusActive, now, now))
	summaries, err := repo.ListClientSecrets(context.Background(), "demo-client")
	if err != nil {
		t.Fatalf("list client secrets: %v", err)
	}
	if len(summaries) != 1 || summaries[0].SecretHint != "sec_****1234" {
		t.Fatalf("unexpected secret summaries: %#v", summaries)
	}
	assertSQLExpectations(t, mock)
}

func TestRepositoryUpdateClientStatusCanRevokeClientSessions(t *testing.T) {
	db, mock, repo := newSSORepositoryMock(t)
	defer db.Close()
	now := time.Date(2026, 6, 18, 13, 0, 0, 0, time.UTC)

	mock.ExpectExec(regexp.QuoteMeta(`UPDATE sys_sso_client
SET status = ?, updaterId = ?, updateTime = ?
WHERE clientId = ? AND status <> ? AND isDeleted = 0`)).
		WithArgs(domain.ClientStatusDisabled, int64(400), now, "demo-client", domain.ClientStatusDisabled).
		WillReturnResult(sqlmock.NewResult(0, 1))
	changed, err := repo.UpdateClientStatus(context.Background(), "demo-client", domain.ClientStatusDisabled, 400, now)
	if err != nil {
		t.Fatalf("update client status: %v", err)
	}
	if !changed {
		t.Fatal("expected status change")
	}

	mock.ExpectExec(regexp.QuoteMeta(`UPDATE sys_sso_session
SET revokedAt = ?, status = ?, updateTime = ?
WHERE clientId = ? AND createTime <= ? AND status = ? AND isDeleted = 0`)).
		WithArgs(now, domain.SessionStatusRevoked, now, "demo-client", now, domain.SessionStatusActive).
		WillReturnResult(sqlmock.NewResult(0, 3))
	affected, err := repo.RevokeSessionsByClientID(context.Background(), "demo-client", now)
	if err != nil {
		t.Fatalf("revoke sessions by client: %v", err)
	}
	if affected != 3 {
		t.Fatalf("expected 3 revoked sessions, got %d", affected)
	}
	assertSQLExpectations(t, mock)
}

func TestRepositoryCanRevokePlatformSessionsAndRefreshFamilies(t *testing.T) {
	db, mock, repo := newSSORepositoryMock(t)
	defer db.Close()
	now := time.Date(2026, 6, 22, 16, 0, 0, 0, time.UTC)

	mock.ExpectExec(regexp.QuoteMeta(`UPDATE sys_sso_refresh_token_family
SET revokedAt = ?, status = ?, updateTime = ?
WHERE status = ? AND isDeleted = 0
  AND sessionId IN (
    SELECT sessionId
    FROM sys_sso_session
    WHERE platformCode = ? AND createTime <= ? AND status = ? AND isDeleted = 0
  )`)).
		WithArgs(now, domain.RefreshFamilyStatusRevoked, now, domain.RefreshFamilyStatusActive, "seven-admin", now, domain.SessionStatusActive).
		WillReturnResult(sqlmock.NewResult(0, 2))
	if err := repo.RevokeRefreshFamiliesByPlatformCode(context.Background(), " seven-admin ", now); err != nil {
		t.Fatalf("revoke refresh families by platform: %v", err)
	}

	mock.ExpectExec(regexp.QuoteMeta(`UPDATE sys_sso_session
SET revokedAt = ?, status = ?, updateTime = ?
WHERE platformCode = ? AND createTime <= ? AND status = ? AND isDeleted = 0`)).
		WithArgs(now, domain.SessionStatusRevoked, now, "seven-admin", now, domain.SessionStatusActive).
		WillReturnResult(sqlmock.NewResult(0, 2))
	affected, err := repo.RevokeSessionsByPlatformCode(context.Background(), " seven-admin ", now)
	if err != nil {
		t.Fatalf("revoke sessions by platform: %v", err)
	}
	if affected != 2 {
		t.Fatalf("expected 2 revoked sessions, got %d", affected)
	}
	assertSQLExpectations(t, mock)
}

func TestRepositoryPagesUserSessionsWithBoundedLimitAndCount(t *testing.T) {
	db, mock, repo := newSSORepositoryMock(t)
	defer db.Close()
	now := time.Date(2026, 7, 11, 12, 0, 0, 0, time.UTC)
	mock.ExpectQuery(`SELECT COUNT\(\*\) FROM sys_sso_session WHERE userId = \? AND isDeleted = 0`).
		WithArgs(int64(2001)).WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(201))
	total, err := repo.CountSessionsByUserID(context.Background(), 2001)
	if err != nil || total != 201 {
		t.Fatalf("CountSessionsByUserID()=%d err=%v", total, err)
	}
	mock.ExpectQuery(`FROM sys_sso_session\s+WHERE userId = \? AND isDeleted = 0\s+ORDER BY id DESC\s+LIMIT \? OFFSET \?`).
		WithArgs(int64(2001), 25, 50).
		WillReturnRows(sessionRows().AddRow(9, "session-9", 2001, "client", "seven-admin", nil, nil, nil, nil, nil, nil, nil, nil, now, now, now.Add(time.Hour), nil, domain.SessionStatusActive, nil, now, now))
	items, err := repo.ListSessionsByUserIDPage(context.Background(), 2001, 50, 25)
	if err != nil || len(items) != 1 || items[0].SessionID != "session-9" {
		t.Fatalf("ListSessionsByUserIDPage()=%+v err=%v", items, err)
	}
	assertSQLExpectations(t, mock)
}

func TestRepositoryRevokesUserSessionsAndRefreshFamiliesAtCutoff(t *testing.T) {
	db, mock, repo := newSSORepositoryMock(t)
	defer db.Close()
	cutoff := time.Date(2026, 7, 11, 12, 0, 0, 123000000, time.UTC)
	mock.ExpectExec(`UPDATE sys_sso_refresh_token_family\s+SET revokedAt = \?, status = \?, updateTime = \?\s+WHERE userId = \? AND createTime <= \? AND status = \? AND isDeleted = 0`).
		WithArgs(cutoff, domain.RefreshFamilyStatusRevoked, cutoff, int64(2001), cutoff, domain.RefreshFamilyStatusActive).
		WillReturnResult(sqlmock.NewResult(0, 2))
	if err := repo.RevokeRefreshFamiliesByUserIDAtOrBefore(context.Background(), 2001, cutoff); err != nil {
		t.Fatalf("revoke refresh families at cutoff: %v", err)
	}
	mock.ExpectExec(`UPDATE sys_sso_session\s+SET revokedAt = \?, status = \?, updateTime = \?\s+WHERE userId = \? AND createTime <= \? AND status = \? AND isDeleted = 0`).
		WithArgs(cutoff, domain.SessionStatusRevoked, cutoff, int64(2001), cutoff, domain.SessionStatusActive).
		WillReturnResult(sqlmock.NewResult(0, 3))
	changed, err := repo.RevokeSessionsByUserIDAtOrBefore(context.Background(), 2001, cutoff)
	if err != nil || changed != 3 {
		t.Fatalf("RevokeSessionsByUserIDAtOrBefore()=%d err=%v", changed, err)
	}
	assertSQLExpectations(t, mock)
}

func TestRepositoryCapturesManagedSessionCutoffFromMySQL(t *testing.T) {
	db, mock, repo := newSSORepositoryMock(t)
	defer db.Close()

	cutoff := time.Date(2026, 7, 12, 8, 30, 0, 654321000, time.UTC)
	mock.ExpectQuery(regexp.QuoteMeta(`SELECT CURRENT_TIMESTAMP(6)`)).
		WillReturnRows(sqlmock.NewRows([]string{"cutoff"}).AddRow(cutoff))
	got, err := repo.CaptureManagedSessionCutoff(t.Context())
	if err != nil || !got.Equal(cutoff) {
		t.Fatalf("managed cutoff=%s err=%v want=%s", got, err, cutoff)
	}
	assertSQLExpectations(t, mock)
}

func sessionRows() *sqlmock.Rows {
	return sqlmock.NewRows([]string{"id", "sessionId", "userId", "clientId", "platformCode", "deviceId", "loginIp", "userAgent", "acr", "amrJson", "loginMethod", "externalProviderCode", "externalIdentityId", "loginAt", "lastAccessAt", "expiresAt", "revokedAt", "status", "metadataJson", "createTime", "updateTime"})
}

func newSSORepositoryMock(t *testing.T) (*sql.DB, sqlmock.Sqlmock, *Repository) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	return db, mock, &Repository{db: sqlx.NewDb(db, "sqlmock")}
}

func assertSQLExpectations(t *testing.T, mock sqlmock.Sqlmock) {
	t.Helper()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}
