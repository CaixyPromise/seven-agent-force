package infrastructure

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
)

func TestSSOPostgresRendererUsesLowerSnakeTablesAndQuotesCamelCaseFields(t *testing.T) {
	query := `SELECT c.clientId, r.redirectUri, s.sessionId
FROM sys_sso_client c
JOIN sys_sso_client_redirect_uri r ON r.clientId = c.clientId
JOIN sys_sso_session s ON s.clientId = c.clientId
WHERE c.isDeleted = 0 AND r.postLogoutRedirectUri = ?`
	got := ssoPostgresRenderer.RenderPostgres(query)
	for _, fragment := range []string{
		`FROM sys_sso_client c`, `JOIN sys_sso_client_redirect_uri r`, `JOIN sys_sso_session s`,
		`c."clientId"`, `r."redirectUri"`, `s."sessionId"`,
		`c."isDeleted"`, `r."postLogoutRedirectUri"`,
	} {
		if !strings.Contains(got, fragment) {
			t.Fatalf("rendered query missing %q: %s", fragment, got)
		}
	}
	for _, quoted := range []string{`"sys_sso_client"`, `"sys_sso_client_redirect_uri"`, `"sys_sso_session"`} {
		if strings.Contains(got, quoted) {
			t.Fatalf("lower snake table should not require PostgreSQL quoting %q: %s", quoted, got)
		}
	}
}

func TestSSOPostgresCutoffUsesPortableCurrentTimestamp(t *testing.T) {
	rawDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer rawDB.Close()
	db := sqlx.NewDb(rawDB, "sqlmock")
	repo, err := NewRepository(&dg2SSOIntegrationProvider{
		driver: "sqlmock", dialect: "pgx", db: rawDB, sqlxDB: db,
	})
	if err != nil {
		t.Fatalf("new repository: %v", err)
	}
	now := time.Now().UTC()
	mock.ExpectQuery(`^SELECT CURRENT_TIMESTAMP$`).
		WillReturnRows(sqlmock.NewRows([]string{"cutoff"}).AddRow(now))
	cutoff, err := repo.CaptureManagedSessionCutoff(context.Background())
	if err != nil || !cutoff.Equal(now) {
		t.Fatalf("cutoff=%v err=%v", cutoff, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestSSOPostgresRendererQuotesConsentConflictTarget(t *testing.T) {
	query := `INSERT INTO sys_sso_consent_grant (userId, clientId, isDeleted)
VALUES (?, ?, 0)
ON CONFLICT (userId, clientId, isDeleted) DO UPDATE SET updateTime = CURRENT_TIMESTAMP`
	got := ssoPostgresRenderer.RenderPostgres(query)
	for _, fragment := range []string{
		`INSERT INTO sys_sso_consent_grant`, `("userId", "clientId", "isDeleted")`,
		`"updateTime"`,
	} {
		if !strings.Contains(got, fragment) {
			t.Fatalf("rendered query missing %q: %s", fragment, got)
		}
	}
	if strings.Contains(got, `"sys_sso_consent_grant"`) {
		t.Fatalf("lower snake consent table should not require PostgreSQL quoting: %s", got)
	}
}
