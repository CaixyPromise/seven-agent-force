package infrastructure

import (
	"context"
	"database/sql"
	"errors"
	"regexp"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/external_login/domain"
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

func TestRepositoryInsertProviderStoresEncryptedSecretColumns(t *testing.T) {
	db, mock, repo := newExternalLoginRepositoryMock(t)
	defer db.Close()

	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO sys_external_login_provider (
    providerCode, providerName, protocolType, issuer, authorizationEndpoint, tokenEndpoint,
    userinfoEndpoint, jwksUri, clientId, clientSecretCiphertext, clientSecretEdek,
    clientSecretWrapKeyRef, scopesJson, redirectUri, displayName, icon, sortOrder,
    displayEnabled, loginEnabled, bindEnabled, emailAutoBindEnabled, accountAutoCreateEnabled, status, metadataJson,
    creatorId, updaterId, isDeleted
) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0)`)).
		WithArgs(
			"github", "GitHub", domain.ProtocolTypeOAuth2, nil, "https://github.com/login/oauth/authorize",
			"https://github.com/login/oauth/access_token", nil, nil, "client-id", "ciphertext",
			"edek", "master-2026", `["read:user","user:email"]`, "https://app.example/callback",
			"GitHub", "github", 10, 1, 1, 1, 0, 0, domain.ProviderStatusActive, nil, int64(100), int64(100),
		).
		WillReturnResult(sqlmock.NewResult(1, 1))

	err := repo.InsertProvider(context.Background(), &domain.Provider{
		ProviderCode:           "github",
		ProviderName:           "GitHub",
		ProtocolType:           domain.ProtocolTypeOAuth2,
		AuthorizationEndpoint:  "https://github.com/login/oauth/authorize",
		TokenEndpoint:          "https://github.com/login/oauth/access_token",
		ClientID:               "client-id",
		ClientSecretCiphertext: "ciphertext",
		ClientSecretEDEK:       "edek",
		ClientSecretWrapKeyRef: "master-2026",
		Scopes:                 []string{"read:user", "user:email"},
		RedirectURI:            "https://app.example/callback",
		DisplayName:            "GitHub",
		Icon:                   "github",
		SortOrder:              10,
		DisplayEnabled:         true,
		LoginEnabled:           true,
		BindEnabled:            true,
		Status:                 domain.ProviderStatusActive,
	}, 100)
	if err != nil {
		t.Fatalf("insert provider: %v", err)
	}
	assertSQLExpectations(t, mock)
}

func TestRepositoryListLoginMethodsFiltersDisplayLoginAndStatus(t *testing.T) {
	db, mock, repo := newExternalLoginRepositoryMock(t)
	defer db.Close()
	now := time.Date(2026, 6, 21, 10, 0, 0, 0, time.UTC)

	mock.ExpectQuery(`SELECT p\.id, p\.providerCode, p\.providerName, p\.protocolType`).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "providerCode", "providerName", "protocolType", "issuer", "authorizationEndpoint", "tokenEndpoint",
			"userinfoEndpoint", "jwksUri", "clientId", "clientSecretCiphertext", "clientSecretEdek", "clientSecretWrapKeyRef",
			"scopesJson", "redirectUri", "displayName", "icon", "sortOrder", "displayEnabled", "loginEnabled", "bindEnabled",
			"emailAutoBindEnabled", "status", "metadataJson", "creatorId", "updaterId", "createTime", "updateTime",
		}).AddRow(
			int64(1), "github", "GitHub", domain.ProtocolTypeOAuth2, nil, "https://github.com/login/oauth/authorize",
			"https://github.com/login/oauth/access_token", nil, nil, "client-id", "ciphertext", "edek", "master-2026",
			`["read:user"]`, "https://app.example/callback", "GitHub", "github", 10, 1, 1, 1, 0,
			domain.ProviderStatusActive, nil, nil, nil, now, now,
		))

	items, err := repo.ListLoginMethods(context.Background())
	if err != nil {
		t.Fatalf("list login methods: %v", err)
	}
	if len(items) != 1 || items[0].ProviderCode != "github" || !items[0].DisplayEnabled || !items[0].LoginEnabled {
		t.Fatalf("unexpected login methods: %#v", items)
	}
	assertSQLExpectations(t, mock)
}

func TestRepositoryRevokeTokensByProviderUpdatesOnlyActiveRows(t *testing.T) {
	db, mock, repo := newExternalLoginRepositoryMock(t)
	defer db.Close()
	now := time.Date(2026, 6, 21, 11, 0, 0, 0, time.UTC)

	mock.ExpectExec(regexp.QuoteMeta(`UPDATE sys_external_oauth_token
SET revokedAt = ?, status = ?, version = version + 1, updateTime = ?, metadataJson = JSON_SET(COALESCE(metadataJson, JSON_OBJECT()), '$.revokeReason', ?)
WHERE providerCode = ? AND status = ? AND revokedAt IS NULL AND isDeleted = 0`)).
		WithArgs(now, domain.TokenStatusRevoked, now, "provider disabled", "github", domain.TokenStatusActive).
		WillReturnResult(sqlmock.NewResult(0, 2))

	affected, err := repo.RevokeTokensByProvider(context.Background(), "github", now, "provider disabled")
	if err != nil {
		t.Fatalf("revoke by provider: %v", err)
	}
	if affected != 2 {
		t.Fatalf("expected 2 affected rows, got %d", affected)
	}
	assertSQLExpectations(t, mock)
}

func TestRepositoryRevokeTokensByIdentityUpdatesOnlyActiveRows(t *testing.T) {
	db, mock, repo := newExternalLoginRepositoryMock(t)
	defer db.Close()
	now := time.Date(2026, 6, 21, 11, 30, 0, 0, time.UTC)

	mock.ExpectExec(regexp.QuoteMeta(`UPDATE sys_external_oauth_token
SET revokedAt = ?, status = ?, version = version + 1, updateTime = ?, metadataJson = JSON_SET(COALESCE(metadataJson, JSON_OBJECT()), '$.revokeReason', ?)
WHERE identityId = ? AND status = ? AND revokedAt IS NULL AND isDeleted = 0`)).
		WithArgs(now, domain.TokenStatusRevoked, now, "identity disabled", int64(99), domain.TokenStatusActive).
		WillReturnResult(sqlmock.NewResult(0, 1))

	affected, err := repo.RevokeTokensByIdentity(context.Background(), 99, now, "identity disabled")
	if err != nil {
		t.Fatalf("revoke by identity: %v", err)
	}
	if affected != 1 {
		t.Fatalf("expected 1 affected row, got %d", affected)
	}
	assertSQLExpectations(t, mock)
}

func TestRepositoryListIdentitiesFiltersAndPaginates(t *testing.T) {
	db, mock, repo := newExternalLoginRepositoryMock(t)
	defer db.Close()
	now := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	status := domain.IdentityStatusActive
	userID := int64(1001)

	mock.ExpectQuery(regexp.QuoteMeta(`SELECT COUNT(*) FROM sys_external_user_identity i WHERE i.isDeleted = 0 AND i.scope_type = 'SYSTEM' AND i.providerCode = ? AND i.userId = ? AND i.status = ? AND (i.externalLogin LIKE ? OR i.externalEmail LIKE ? OR i.displayName LIKE ?)`)).
		WithArgs("github", userID, status, "%octo%", "%octo%", "%octo%").
		WillReturnRows(sqlmock.NewRows([]string{"COUNT(*)"}).AddRow(int64(1)))
	mock.ExpectQuery(`SELECT id, providerCode, externalIssuer, COALESCE\(externalSubject,''\) AS externalSubject, subject_ciphertext`).
		WithArgs("github", userID, status, "%octo%", "%octo%", "%octo%", 20, 20).
		WillReturnRows(sqlmock.NewRows([]string{
			"id", "providerCode", "externalIssuer", "externalSubject", "subject_ciphertext", "subject_edek", "subject_wrap_key_ref", "subject_digest", "subject_digest_key_ref", "userId", "externalLogin", "externalEmail", "emailVerified",
			"displayName", "avatarUrl", "profileJson", "status", "firstLinkedAt", "lastLoginAt", "lastVerifiedAt",
			"metadataJson", "creatorId", "updaterId", "createTime", "updateTime",
		}).AddRow(
			int64(10), "github", nil, "", "cipher", "edek", "kid", "digest", "kid", userID, "octocat", "octo@example.com", 1,
			"Octo Cat", "https://example.com/avatar.png", `{"login":"octocat"}`, status, now.Add(-time.Hour),
			now, now, nil, int64(100), int64(101), now.Add(-time.Hour), now,
		))

	items, total, err := repo.ListIdentities(context.Background(), domain.IdentityQuery{
		ProviderCode: "github",
		UserID:       &userID,
		Status:       &status,
		Keyword:      "octo",
		Current:      2,
		PageSize:     20,
	})
	if err != nil {
		t.Fatalf("list identities: %v", err)
	}
	if total != 1 || len(items) != 1 || items[0].ID != 10 || items[0].ExternalLogin != "octocat" || !items[0].EmailVerified {
		t.Fatalf("unexpected identities total=%d items=%#v", total, items)
	}
	assertSQLExpectations(t, mock)
}

func TestRepositoryFindIdentityByIssuerUsesProtectedDigestLookup(t *testing.T) {
	db, mock, repo := newExternalLoginRepositoryMock(t)
	defer db.Close()
	now := time.Now().UTC()
	columns := []string{"id", "providerCode", "externalIssuer", "externalSubject", "subject_ciphertext", "subject_edek", "subject_wrap_key_ref", "subject_digest", "subject_digest_key_ref", "userId", "externalLogin", "externalEmail", "emailVerified", "displayName", "avatarUrl", "profileJson", "status", "firstLinkedAt", "lastLoginAt", "lastVerifiedAt", "metadataJson", "creatorId", "updaterId", "createTime", "updateTime"}
	mock.ExpectQuery(`WHERE providerCode = \? AND COALESCE\(externalIssuer,''\) = \? AND subject_digest = \? AND scope_type='SYSTEM'`).
		WithArgs("hub:node-a", "https://hub.example", "same-sub").
		WillReturnRows(sqlmock.NewRows(columns).
			AddRow(int64(2), "hub:node-a", "https://hub.example", "", "cipher", "edek", "kid", "same-sub", "kid", int64(2002), nil, nil, 0, nil, nil, nil, domain.IdentityStatusActive, now, nil, nil, nil, nil, nil, now, now))

	identity, err := repo.FindIdentityBySubject(context.Background(), "hub:node-a", "https://hub.example", "same-sub")
	if err != nil {
		t.Fatalf("find issuer identity: %v", err)
	}
	if identity == nil || identity.ID != 2 || identity.ExternalIssuer != "https://hub.example" {
		t.Fatalf("digest collision selected wrong identity: %#v", identity)
	}
	assertSQLExpectations(t, mock)
}

func TestRepositoryFindOrdinaryIdentityRetainsProviderSubjectSemantics(t *testing.T) {
	db, mock, repo := newExternalLoginRepositoryMock(t)
	defer db.Close()
	now := time.Now().UTC()
	mock.ExpectQuery(`WHERE providerCode = \? AND COALESCE\(externalIssuer,''\) = \? AND subject_digest = \? AND scope_type='SYSTEM'`).WithArgs("github", "", "octocat").
		WillReturnRows(sqlmock.NewRows([]string{"id", "providerCode", "externalIssuer", "externalSubject", "subject_ciphertext", "subject_edek", "subject_wrap_key_ref", "subject_digest", "subject_digest_key_ref", "userId", "externalLogin", "externalEmail", "emailVerified", "displayName", "avatarUrl", "profileJson", "status", "firstLinkedAt", "lastLoginAt", "lastVerifiedAt", "metadataJson", "creatorId", "updaterId", "createTime", "updateTime"}).
			AddRow(int64(3), "github", nil, "octocat", nil, nil, nil, "octocat", nil, int64(3003), "octocat", nil, 0, nil, nil, nil, domain.IdentityStatusActive, now, nil, nil, nil, nil, nil, now, now))
	identity, err := repo.FindIdentityBySubject(context.Background(), "github", "", "octocat")
	if err != nil || identity == nil || identity.UserID != 3003 || identity.ExternalIssuer != "" {
		t.Fatalf("ordinary identity=%#v err=%v", identity, err)
	}
	assertSQLExpectations(t, mock)
}

func TestRepositoryInsertIdentityPersistsValidatedIssuer(t *testing.T) {
	db, mock, repo := newExternalLoginRepositoryMock(t)
	defer db.Close()
	now := time.Now().UTC()
	mock.ExpectExec(`INSERT INTO sys_external_user_identity`).WithArgs(
		int64(77), "hub:node-a", "https://hub.example", nil, int64(1001), nil, nil, 0, nil, nil, nil,
		domain.IdentityStatusActive, now, &now, &now, nil, int64(1001), int64(1001), "cipher", "edek", "kid", "digest", "digest-kid",
	).WillReturnResult(sqlmock.NewResult(1, 1))
	identity := &domain.ExternalIdentity{ID: 77, ProviderCode: "hub:node-a", ExternalIssuer: "https://hub.example", ExternalSubject: "digest", SubjectCiphertext: "cipher", SubjectEDEK: "edek", SubjectWrapKeyRef: "kid", SubjectDigest: "digest", SubjectDigestKeyRef: "digest-kid", UserID: 1001, Status: domain.IdentityStatusActive, FirstLinkedAt: now, LastLoginAt: &now, LastVerifiedAt: &now}
	if err := repo.InsertIdentity(context.Background(), identity, 1001); err != nil {
		t.Fatalf("insert issuer identity: %v", err)
	}
	assertSQLExpectations(t, mock)
}

func TestRepositoryConsumeLoginStateRejectsConsumedState(t *testing.T) {
	db, mock, repo := newExternalLoginRepositoryMock(t)
	defer db.Close()
	now := time.Date(2026, 6, 21, 12, 0, 0, 0, time.UTC)
	expiresAt := now.Add(5 * time.Minute)

	stateRows := sqlmock.NewRows([]string{
		"id", "stateId", "providerCode", "platformCode", "provisioningAuthorityId", "loginTransactionId", "redirectAfterLogin", "bindUserId", "stateHash", "nonceHash",
		"codeVerifierCiphertext", "codeVerifierEdek", "codeVerifierWrapKeyRef", "issuer", "redirectUri", "expiresAt",
		"consumedAt", "status", "loginIp", "userAgent", "traceId", "createTime", "updateTime",
	}).AddRow(
		int64(7), "state-1", "github", "seven-admin", "plprov-1", "txn-1", "/home", nil, "hash-1", "nonce-1", "pkce-cipher",
		"pkce-edek", "master-2026", "https://github.com", "https://app.example/callback", expiresAt,
		nil, domain.LoginStateStatusActive, "127.0.0.1", "ua", "trace", now, now,
	)

	mock.ExpectQuery(`SELECT id, stateId, providerCode, platformCode, provisioningAuthorityId, loginTransactionId`).
		WithArgs("hash-1", domain.LoginStateStatusActive, now).
		WillReturnRows(stateRows)
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE sys_external_oauth_login_state
SET consumedAt = ?, status = ?, updateTime = ?
WHERE id = ? AND status = ? AND consumedAt IS NULL AND expiresAt > ? AND isDeleted = 0`)).
		WithArgs(now, domain.LoginStateStatusConsumed, now, int64(7), domain.LoginStateStatusActive, now).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT id, stateId, providerCode, platformCode, provisioningAuthorityId, loginTransactionId`).
		WithArgs("hash-1", domain.LoginStateStatusActive, now).
		WillReturnError(sql.ErrNoRows)

	item, err := repo.ConsumeLoginState(context.Background(), "hash-1", now)
	if err != nil {
		t.Fatalf("consume active state: %v", err)
	}
	if item == nil || item.ID != 7 || item.StateHash != "hash-1" {
		t.Fatalf("unexpected consumed state: %#v", item)
	}
	again, err := repo.ConsumeLoginState(context.Background(), "hash-1", now)
	if err != nil {
		t.Fatalf("consume again: %v", err)
	}
	if again != nil {
		t.Fatalf("expected consumed state to be rejected, got %#v", again)
	}
	assertSQLExpectations(t, mock)
}

func TestRepositoryUpdateTokenSetRejectsRevokedRows(t *testing.T) {
	db, mock, repo := newExternalLoginRepositoryMock(t)
	defer db.Close()
	accessExpiresAt := time.Date(2026, 6, 21, 13, 0, 0, 0, time.UTC)
	lastRefreshAt := time.Date(2026, 6, 21, 12, 50, 0, 0, time.UTC)

	mock.ExpectExec(regexp.QuoteMeta(`UPDATE sys_external_oauth_token
SET tokenSetCiphertext = ?, tokenSetEdek = ?, tokenSetWrapKeyRef = ?, accessExpiresAt = ?,
    refreshExpiresAt = ?, lastRefreshAt = ?, status = ?, version = version + 1,
    metadataJson = ?, updateTime = CURRENT_TIMESTAMP
WHERE id = ? AND version = ? AND status = ? AND revokedAt IS NULL AND isDeleted = 0`)).
		WithArgs("cipher-new", "edek-new", "master-2026", &accessExpiresAt, nil, &lastRefreshAt, domain.TokenStatusActive, nil, int64(70), 5, domain.TokenStatusActive).
		WillReturnResult(sqlmock.NewResult(0, 0))

	updated, err := repo.UpdateTokenSet(context.Background(), &domain.OAuthToken{
		ID:                 70,
		TokenSetCiphertext: "cipher-new",
		TokenSetEDEK:       "edek-new",
		TokenSetWrapKeyRef: "master-2026",
		AccessExpiresAt:    &accessExpiresAt,
		LastRefreshAt:      &lastRefreshAt,
		Status:             domain.TokenStatusActive,
	}, 5)
	if err != nil {
		t.Fatalf("update token set: %v", err)
	}
	if updated {
		t.Fatal("expected revoked/non-active token update guard to reject stale refresh")
	}
	assertSQLExpectations(t, mock)
}

func TestRepositoryRevokeTokenIncrementsVersionAndSkipsRevokedRows(t *testing.T) {
	db, mock, repo := newExternalLoginRepositoryMock(t)
	defer db.Close()
	now := time.Date(2026, 6, 21, 13, 30, 0, 0, time.UTC)

	mock.ExpectExec(regexp.QuoteMeta(`UPDATE sys_external_oauth_token
SET revokedAt = ?, status = ?, version = version + 1, updateTime = ?, metadataJson = JSON_SET(COALESCE(metadataJson, JSON_OBJECT()), '$.revokeReason', ?)
WHERE id = ? AND status = ? AND revokedAt IS NULL AND isDeleted = 0`)).
		WithArgs(now, domain.TokenStatusRevoked, now, "manual revoke", int64(71), domain.TokenStatusActive).
		WillReturnResult(sqlmock.NewResult(0, 1))

	changed, err := repo.RevokeToken(context.Background(), 71, now, "manual revoke")
	if err != nil {
		t.Fatalf("revoke token: %v", err)
	}
	if !changed {
		t.Fatal("expected active token to be revoked")
	}
	assertSQLExpectations(t, mock)
}

func TestRepositoryReplaceProviderMethodsRollsBackOnInsertFailure(t *testing.T) {
	db, mock, repo := newExternalLoginRepositoryMockWithTransactor(t)
	defer db.Close()

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`DELETE FROM sys_external_provider_method
WHERE providerCode = ? AND methodKey = ? AND isDeleted = 1`)).
		WithArgs("github", "oauth-login").
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(regexp.QuoteMeta(`UPDATE sys_external_provider_method
SET isDeleted = 1, updateTime = CURRENT_TIMESTAMP
WHERE providerCode = ? AND isDeleted = 0`)).
		WithArgs("github").
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO sys_external_provider_method (
    providerCode, methodKey, capabilityCode, requiredScopesJson, status, metadataJson, isDeleted
) VALUES (?, ?, ?, ?, ?, ?, 0)`)).
		WithArgs("github", "oauth-login", "LOGIN", `["read:user"]`, domain.ProviderMethodStatusActive, nil).
		WillReturnError(errors.New("insert failed"))
	mock.ExpectRollback()

	err := repo.ReplaceProviderMethods(context.Background(), "github", []domain.ProviderMethod{{
		MethodKey:      "oauth-login",
		CapabilityCode: "LOGIN",
		RequiredScopes: []string{"read:user"},
		Status:         domain.ProviderMethodStatusActive,
	}})
	if err == nil {
		t.Fatal("expected insert failure")
	}
	assertSQLExpectations(t, mock)
}

func newExternalLoginRepositoryMock(t *testing.T) (*sql.DB, sqlmock.Sqlmock, *Repository) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	return db, mock, &Repository{db: sqlx.NewDb(db, "sqlmock")}
}

func newExternalLoginRepositoryMockWithTransactor(t *testing.T) (*sql.DB, sqlmock.Sqlmock, *Repository) {
	t.Helper()
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	sqlxDB := sqlx.NewDb(db, "sqlmock")
	provider := staticSQLXProvider{
		db:         sqlxDB,
		transactor: store.NewSQLXTransactor(sqlxDB),
	}
	repo, err := NewRepository(provider)
	if err != nil {
		t.Fatalf("new repository: %v", err)
	}
	return db, mock, repo
}

type staticSQLXProvider struct {
	db         *sqlx.DB
	transactor store.Transactor
}

func (p staticSQLXProvider) Driver() string               { return "mysql" }
func (p staticSQLXProvider) Dialect() string              { return "mysql" }
func (p staticSQLXProvider) DB() *sql.DB                  { return p.db.DB }
func (p staticSQLXProvider) SQLX() *sqlx.DB               { return p.db }
func (p staticSQLXProvider) Close() error                 { return nil }
func (p staticSQLXProvider) Transactor() store.Transactor { return p.transactor }
func (p staticSQLXProvider) Configured() bool             { return p.db != nil }

func assertSQLExpectations(t *testing.T, mock sqlmock.Sqlmock) {
	t.Helper()
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}
