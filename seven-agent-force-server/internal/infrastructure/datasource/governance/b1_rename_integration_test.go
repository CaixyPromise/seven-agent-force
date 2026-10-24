package governance

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	platformdomain "github.com/CaixyPromise/seven-agent/force/internal/app/platform/domain"
	platforminfra "github.com/CaixyPromise/seven-agent/force/internal/app/platform/infrastructure"
	ssodomain "github.com/CaixyPromise/seven-agent/force/internal/app/sso/domain"
	ssoinfra "github.com/CaixyPromise/seven-agent/force/internal/app/sso/infrastructure"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/store"
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
	"github.com/pressly/goose/v3"
)

const (
	dg4B1RenameAcceptanceEnv  = "DG4_B1_RENAME_ACCEPTANCE"
	dg4B1TestDialectEnv       = "DG4_B1_TEST_DIALECT"
	dg4B1TestDSNEnv           = "DG4_B1_TEST_DSN"
	dg4ResetIsolatedSchemaEnv = "DG4_RESET_ISOLATED_SCHEMA"
	dg4B1MigrationVersion     = int64(20260731100000)
	dg4B1FixtureClientID      = "dg4-b1-sso-client"
	dg4B1FixturePlatformCode  = "dg4-b1-platform"
	dg4B1FixtureSessionID     = "dg4-b1-session"
	dg4B1FixtureFamilyID      = "dg4-b1-family"
	dg4B1FixtureCode          = "dg4-b1-authorization-code"
	dg4B1FixtureIssuerKID     = "dg4-b1-issuer-key"
	dg4B1FixtureTraceID       = "dg4-b1-audit"
	dg4B1FixtureUserID        = int64(9202607311001)
	dg4B1FixtureSecretID      = int64(9202607311002)
)

// b1TableMapping is fixed test data. It never accepts identifiers from callers.
type b1TableMapping struct {
	legacy string
	target string
}

var b1TableMappings = []b1TableMapping{
	{legacy: "sysSsoAuditLog", target: "sys_sso_audit_log"},
	{legacy: "sysSsoAuthorizationCode", target: "sys_sso_authorization_code"},
	{legacy: "sysSsoClient", target: "sys_sso_client"},
	{legacy: "sysSsoClientRedirectUri", target: "sys_sso_client_redirect_uri"},
	{legacy: "sysSsoClientSecret", target: "sys_sso_client_secret"},
	{legacy: "sysSsoConsentGrant", target: "sys_sso_consent_grant"},
	{legacy: "sysSsoIssuerKey", target: "sys_sso_issuer_key"},
	{legacy: "sysSsoRefreshTokenFamily", target: "sys_sso_refresh_token_family"},
	{legacy: "sysSsoSession", target: "sys_sso_session"},
	{legacy: "sysPlatform", target: "sys_platform"},
	{legacy: "sysPlatformDefaultRole", target: "sys_platform_default_role"},
	{legacy: "sysPlatformLoginMethod", target: "sys_platform_login_method"},
	{legacy: "sysPlatformSourceRule", target: "sys_platform_source_rule"},
	{legacy: "sysPlatformSsoClient", target: "sys_platform_sso_client"},
}

type b1IntegrationProvider struct {
	driver  string
	dialect string
	db      *sql.DB
	sqlxDB  *sqlx.DB
}

func (p *b1IntegrationProvider) Driver() string               { return p.driver }
func (p *b1IntegrationProvider) Dialect() string              { return p.dialect }
func (p *b1IntegrationProvider) DB() *sql.DB                  { return p.db }
func (p *b1IntegrationProvider) SQLX() *sqlx.DB               { return p.sqlxDB }
func (p *b1IntegrationProvider) Transactor() store.Transactor { return nil }
func (p *b1IntegrationProvider) Configured() bool             { return true }
func (p *b1IntegrationProvider) Close() error                 { return p.db.Close() }

// TestB1SSOPlatformRenameAcceptance is intentionally executed as a controlled
// restart sequence. "prepare" creates the legacy schema only in the exact
// isolated database. "before" runs from the source snapshot where B1, B2,
// and B3 still use legacy physical names. After B1 migration, "after" runs
// from the B1-stage snapshot: B1 uses renamed names while B2/B3 remain legacy.
// "migrate" and "forward" only exercise the migration harness. This prevents
// a fully renamed repository from being mistaken for either historical stage.
func TestB1SSOPlatformRenameAcceptance(t *testing.T) {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv(dg4B1RenameAcceptanceEnv)))
	if mode != "prepare" && mode != "before" && mode != "migrate" && mode != "after" && mode != "forward" {
		t.Skip("set DG4_B1_RENAME_ACCEPTANCE=prepare|before|migrate|after|forward with the exact isolated database")
	}
	dialect := strings.ToLower(strings.TrimSpace(os.Getenv(dg4B1TestDialectEnv)))
	dsn := strings.TrimSpace(os.Getenv(dg4B1TestDSNEnv))
	if dialect == "" || dsn == "" {
		t.Skip("set DG4_B1_TEST_DIALECT and DG4_B1_TEST_DSN for the exact isolated database")
	}
	if dialect != "mysql" && dialect != "postgres" {
		t.Fatalf("unsupported B1 test dialect %q", dialect)
	}
	driver := "mysql"
	if dialect == "postgres" {
		driver = "pgx"
	}
	db, err := sql.Open(driver, dsn)
	if err != nil {
		t.Fatalf("open B1 %s database: %v", dialect, err)
	}
	if err := AssertConnectedDatabase(context.Background(), db, dialect); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	provider := &b1IntegrationProvider{driver: driver, dialect: dialect, db: db, sqlxDB: sqlx.NewDb(db, driver)}
	t.Cleanup(func() { _ = provider.Close() })

	if mode == "prepare" {
		if strings.TrimSpace(os.Getenv(dg4ResetIsolatedSchemaEnv)) == "apply" {
			dg4ResetExactIsolatedSchema(t, context.Background(), provider.db, dialect)
		}
		b1PrepareLegacySchema(t, context.Background(), provider.db, dialect)
		b1AssertTableState(t, context.Background(), provider.db, dialect, true)
		return
	}

	version := b1GooseVersion(t, context.Background(), provider.db)
	if mode == "before" && version != dg0LegacyMigrationCutoff {
		t.Fatalf("B1 pre-rename version=%d, want=%d", version, dg0LegacyMigrationCutoff)
	}
	if mode == "migrate" {
		if version != dg0LegacyMigrationCutoff {
			t.Fatalf("B1 migration start version=%d, want=%d", version, dg0LegacyMigrationCutoff)
		}
		b1ApplyMigration(t, context.Background(), provider.db, dialect)
		if version = b1GooseVersion(t, context.Background(), provider.db); version != dg4B1MigrationVersion {
			t.Fatalf("B1 migration finish version=%d, want=%d", version, dg4B1MigrationVersion)
		}
		return
	}
	if mode == "forward" {
		if version != dg4B1MigrationVersion {
			t.Fatalf("B1 forward-recovery version=%d, want=%d", version, dg4B1MigrationVersion)
		}
		b1AssertForwardOnlyDownRejected(t, context.Background(), provider.db, dialect)
		return
	}
	if mode == "after" && version < dg4B1MigrationVersion {
		t.Fatalf("B1 post-rename version=%d, require at least %d", version, dg4B1MigrationVersion)
	}

	ssoRepository, err := ssoinfra.NewRepository(provider)
	if err != nil {
		t.Fatalf("new B1 SSO repository: %v", err)
	}
	platformRepository, err := platforminfra.NewRepository(provider)
	if err != nil {
		t.Fatalf("new B1 platform repository: %v", err)
	}

	ctx := context.Background()
	if mode == "before" {
		b1CreateFixture(t, ctx, provider, ssoRepository, platformRepository, true)
		b1AssertBusinessFixture(t, ctx, ssoRepository, platformRepository)
		b1AssertTableState(t, ctx, provider.db, dialect, true)
		dg4CapturePhysicalSignatures(t, ctx, provider.db, dialect, "b1", dg4B1PhysicalMappings())
		return
	}

	b1AssertBusinessFixture(t, ctx, ssoRepository, platformRepository)
	dg4AssertPhysicalSignatures(t, ctx, provider.db, dialect, "b1", dg4B1PhysicalMappings())
	b1UpdateAndCreateAfterRename(t, ctx, provider, ssoRepository, platformRepository)
	b1AssertTableState(t, ctx, provider.db, dialect, false)
}

// dg4ResetExactIsolatedSchema is deliberately opt-in. It exists so the
// staged acceptance can start from a known-empty *isolated* database without
// asking an operator to run unguarded cleanup SQL. The caller has already
// proved the server-selected database name, and this function proves it again
// immediately before any destructive statement.
func dg4ResetExactIsolatedSchema(t *testing.T, ctx context.Context, db *sql.DB, dialect string) {
	t.Helper()
	if err := AssertConnectedDatabase(ctx, db, dialect); err != nil {
		t.Fatal(err)
	}
	switch dialect {
	case "mysql":
		rows, err := db.QueryContext(ctx, `
SELECT table_name
FROM information_schema.tables
WHERE table_schema = DATABASE() AND table_type = 'BASE TABLE'
ORDER BY table_name`)
		if err != nil {
			t.Fatalf("list DG4 isolated MySQL tables before reset: %v", err)
		}
		tableNames := make([]string, 0)
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				_ = rows.Close()
				t.Fatalf("scan DG4 isolated MySQL table name: %v", err)
			}
			tableNames = append(tableNames, name)
		}
		if err := rows.Close(); err != nil {
			t.Fatalf("close DG4 isolated MySQL table rows: %v", err)
		}
		for _, tableName := range tableNames {
			if _, err := db.ExecContext(ctx, "DROP TABLE "+dg4QuotePhysicalTable("mysql", tableName)); err != nil {
				t.Fatalf("drop DG4 isolated MySQL table %q: %v", tableName, err)
			}
		}
	case "postgres":
		if _, err := db.ExecContext(ctx, `DROP SCHEMA public CASCADE`); err != nil {
			t.Fatalf("drop DG4 isolated PostgreSQL public schema: %v", err)
		}
		if _, err := db.ExecContext(ctx, `CREATE SCHEMA public`); err != nil {
			t.Fatalf("recreate DG4 isolated PostgreSQL public schema: %v", err)
		}
	default:
		t.Fatalf("unsupported DG4 reset dialect %q", dialect)
	}
}

func b1ApplyMigration(t *testing.T, ctx context.Context, db *sql.DB, dialect string) {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve B1 repository root: %v", err)
	}
	dir := filepath.Join(root, "migrations", dialect)
	goose.SetTableName(goose.DefaultTablename)
	if err := goose.SetDialect(dialect); err != nil {
		t.Fatalf("set B1 %s goose dialect: %v", dialect, err)
	}
	if err := goose.UpToContext(ctx, db, dir, dg4B1MigrationVersion); err != nil {
		t.Fatalf("apply B1 %s in-place table rename: %v", dialect, err)
	}
}

func b1PrepareLegacySchema(t *testing.T, ctx context.Context, db *sql.DB, dialect string) {
	t.Helper()
	plan, err := MigrationPlanFor(dialect)
	if err != nil {
		t.Fatalf("resolve B1 %s migration plan: %v", dialect, err)
	}
	assertEmptyDatabase(t, ctx, db, plan.Dialect)
	root, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve B1 repository root for legacy preparation: %v", err)
	}
	goose.SetTableName(goose.DefaultTablename)
	if err := goose.SetDialect(plan.Dialect); err != nil {
		t.Fatalf("set B1 %s goose dialect for legacy preparation: %v", plan.Dialect, err)
	}
	if plan.CleanBaselineDir != "" {
		baselineDir := filepath.Join(root, filepath.FromSlash(plan.CleanBaselineDir))
		if err := goose.UpContext(ctx, db, baselineDir); err != nil {
			t.Fatalf("apply B1 %s clean baseline: %v", plan.Dialect, err)
		}
	}
	migrationsDir := filepath.Join(root, filepath.FromSlash(plan.MigrationsDir))
	if err := goose.UpToContext(ctx, db, migrationsDir, dg0LegacyMigrationCutoff); err != nil {
		t.Fatalf("apply B1 %s legacy cutoff %d: %v", plan.Dialect, dg0LegacyMigrationCutoff, err)
	}
	if version := b1GooseVersion(t, ctx, db); version != dg0LegacyMigrationCutoff {
		t.Fatalf("B1 legacy preparation version=%d, want=%d", version, dg0LegacyMigrationCutoff)
	}
}

func b1AssertForwardOnlyDownRejected(t *testing.T, ctx context.Context, db *sql.DB, dialect string) {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve B1 repository root for rejected Down: %v", err)
	}
	dir := filepath.Join(root, "migrations", dialect)
	goose.SetTableName(goose.DefaultTablename)
	if err := goose.SetDialect(dialect); err != nil {
		t.Fatalf("set B1 %s goose dialect for rejected Down: %v", dialect, err)
	}
	assertForwardOnlyDownRejected(t, ctx, db, dir)
}

func b1GooseVersion(t *testing.T, ctx context.Context, db *sql.DB) int64 {
	t.Helper()
	var version int64
	if err := db.QueryRowContext(ctx, `SELECT MAX(version_id) FROM goose_db_version`).Scan(&version); err != nil {
		t.Fatalf("read B1 goose version: %v", err)
	}
	return version
}

func b1CreateFixture(
	t *testing.T,
	ctx context.Context,
	provider *b1IntegrationProvider,
	ssoRepository *ssoinfra.Repository,
	platformRepository *platforminfra.Repository,
	legacy bool,
) {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Microsecond)
	err := store.NewSQLXTransactor(provider.sqlxDB).WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := ssoRepository.InsertClient(txCtx, &ssodomain.Client{
			ClientID:           dg4B1FixtureClientID,
			ClientName:         "DG4 B1 Legacy Client",
			ClientType:         "CONFIDENTIAL",
			ClientAuthMethod:   "client_secret_basic",
			GrantTypes:         []string{"authorization_code", "refresh_token"},
			Scopes:             []string{"openid", "profile"},
			RequirePKCE:        true,
			AccessTokenTTLSec:  1800,
			RefreshTokenTTLSec: 2592000,
			Status:             ssodomain.ClientStatusActive,
			MetadataJSON:       `{"fixture":"dg4-b1"}`,
		}, 0); err != nil {
			return err
		}
		if err := ssoRepository.ReplaceClientRedirectURIs(txCtx, dg4B1FixtureClientID, []ssodomain.ClientRedirectURI{{
			RedirectURI:           "https://dg4-b1.example.test/callback",
			PostLogoutRedirectURI: "https://dg4-b1.example.test/logout",
			Status:                ssodomain.ClientStatusActive,
		}}, 0, now); err != nil {
			return err
		}
		if err := ssoRepository.InsertClientSecret(txCtx, &ssodomain.ClientSecret{
			ID:         dg4B1FixtureSecretID,
			ClientID:   dg4B1FixtureClientID,
			SecretHash: "dg4-b1-secret-hash",
			SecretHint: "b1",
			Status:     ssodomain.ClientStatusActive,
		}, 0); err != nil {
			return err
		}
		if err := ssoRepository.UpsertConsentGrant(txCtx, &ssodomain.ConsentGrant{
			UserID:    dg4B1FixtureUserID,
			ClientID:  dg4B1FixtureClientID,
			Scopes:    []string{"openid", "profile"},
			GrantedAt: now,
			Status:    ssodomain.ConsentStatusActive,
		}); err != nil {
			return err
		}
		if err := ssoRepository.InsertSession(txCtx, &ssodomain.Session{
			SessionID:    dg4B1FixtureSessionID,
			UserID:       dg4B1FixtureUserID,
			ClientID:     dg4B1FixtureClientID,
			PlatformCode: dg4B1FixturePlatformCode,
			LoginMethod:  "PASSWORD",
			LoginAt:      now,
			ExpiresAt:    now.Add(time.Hour),
			Status:       ssodomain.SessionStatusActive,
		}); err != nil {
			return err
		}
		if err := ssoRepository.InsertRefreshTokenFamily(txCtx, &ssodomain.RefreshTokenFamily{
			FamilyID:         dg4B1FixtureFamilyID,
			SessionID:        dg4B1FixtureSessionID,
			ClientID:         dg4B1FixtureClientID,
			UserID:           dg4B1FixtureUserID,
			CurrentTokenHash: "dg4-b1-current-token",
			ExpiresAt:        now.Add(time.Hour),
			Status:           ssodomain.RefreshFamilyStatusActive,
		}); err != nil {
			return err
		}
		if err := ssoRepository.InsertAuthorizationCode(txCtx, &ssodomain.AuthorizationCode{
			Code:        dg4B1FixtureCode,
			ClientID:    dg4B1FixtureClientID,
			UserID:      dg4B1FixtureUserID,
			SessionID:   dg4B1FixtureSessionID,
			RedirectURI: "https://dg4-b1.example.test/callback",
			Scopes:      []string{"openid", "profile"},
			ExpiresAt:   now.Add(time.Hour),
			Status:      ssodomain.CodeStatusActive,
		}); err != nil {
			return err
		}
		if err := ssoRepository.InsertAuditLog(txCtx, ssodomain.AuditLog{
			EventType: "DG4_B1",
			ClientID:  dg4B1FixtureClientID,
			Result:    "SUCCESS",
			TraceID:   dg4B1FixtureTraceID,
		}); err != nil {
			return err
		}

		if err := platformRepository.InsertPlatform(txCtx, platformdomain.Platform{
			PlatformCode:       dg4B1FixturePlatformCode,
			PlatformName:       "DG4 B1 Legacy Platform",
			PlatformType:       "ADMIN",
			DefaultRedirectURL: "https://dg4-b1.example.test/",
			Status:             platformdomain.StatusActive,
		}, 0); err != nil {
			return err
		}
		if err := platformRepository.ReplaceLoginMethods(txCtx, dg4B1FixturePlatformCode, []platformdomain.LoginMethod{{
			MethodType:     platformdomain.MethodPassword,
			DisplayName:    "Password",
			SortOrder:      1,
			DisplayEnabled: true,
			LoginEnabled:   true,
		}}, 0); err != nil {
			return err
		}
		if err := platformRepository.ReplaceSourceRules(txCtx, dg4B1FixturePlatformCode, []platformdomain.SourceRule{{
			MatchType:  platformdomain.MatchClientID,
			MatchValue: dg4B1FixtureClientID,
			Priority:   1,
			Status:     platformdomain.StatusActive,
		}}, 0); err != nil {
			return err
		}
		if err := platformRepository.ReplaceDefaultRoles(txCtx, dg4B1FixturePlatformCode, []platformdomain.DefaultRole{{
			RoleID:            1,
			AutoAssignEnabled: true,
			Status:            platformdomain.StatusActive,
		}}, 0); err != nil {
			return err
		}

		exec := store.SQLXFromContext(txCtx)
		if exec == nil {
			return fmt.Errorf("B1 fixture transaction executor is missing")
		}
		if err := b1Exec(txCtx, exec, `INSERT INTO `+b1PhysicalTable(provider.dialect, legacy, "sysSsoIssuerKey")+` (
  `+b1Column(provider.dialect, "kid")+`, `+b1Column(provider.dialect, "algorithm")+`, `+b1Column(provider.dialect, "publicKeyPem")+`, `+b1Column(provider.dialect, "keyStatus")+`, `+b1Column(provider.dialect, "createTime")+`, `+b1Column(provider.dialect, "updateTime")+`, `+b1Column(provider.dialect, "isDeleted")+`
) VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, 0)`, dg4B1FixtureIssuerKID, "RS256", "fixture-public-key", "ACTIVE"); err != nil {
			return err
		}
		return b1Exec(txCtx, exec, `INSERT INTO `+b1PhysicalTable(provider.dialect, legacy, "sysPlatformSsoClient")+` (
  `+b1Column(provider.dialect, "platformCode")+`, `+b1Column(provider.dialect, "clientId")+`, `+b1Column(provider.dialect, "status")+`, `+b1Column(provider.dialect, "createTime")+`, `+b1Column(provider.dialect, "updateTime")+`, `+b1Column(provider.dialect, "isDeleted")+`
) VALUES (?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP, 0)`, dg4B1FixturePlatformCode, dg4B1FixtureClientID, platformdomain.StatusActive)
	})
	if err != nil {
		t.Fatalf("create B1 pre-rename fixture: %v", err)
	}
}

func b1AssertBusinessFixture(
	t *testing.T,
	ctx context.Context,
	ssoRepository *ssoinfra.Repository,
	platformRepository *platforminfra.Repository,
) {
	t.Helper()
	client, err := ssoRepository.FindClient(ctx, dg4B1FixtureClientID)
	if err != nil || client == nil || client.ClientID != dg4B1FixtureClientID {
		t.Fatalf("read B1 client through repository: client=%#v err=%v", client, err)
	}
	redirects, err := ssoRepository.ListClientRedirectURIs(ctx, dg4B1FixtureClientID)
	if err != nil || len(redirects) != 1 {
		t.Fatalf("read B1 client redirects through repository: redirects=%#v err=%v", redirects, err)
	}
	secrets, err := ssoRepository.ListClientSecrets(ctx, dg4B1FixtureClientID)
	if err != nil || len(secrets) != 1 {
		t.Fatalf("read B1 client secrets through repository: secrets=%#v err=%v", secrets, err)
	}
	consent, err := ssoRepository.FindConsentGrant(ctx, dg4B1FixtureUserID, dg4B1FixtureClientID)
	if err != nil || consent == nil || len(consent.Scopes) != 2 {
		t.Fatalf("read B1 consent through repository: consent=%#v err=%v", consent, err)
	}
	session, err := ssoRepository.FindSessionBySessionID(ctx, dg4B1FixtureSessionID)
	if err != nil || session == nil || session.ClientID != dg4B1FixtureClientID {
		t.Fatalf("read B1 session through repository: session=%#v err=%v", session, err)
	}
	family, err := ssoRepository.FindRefreshFamilyByCurrentHash(ctx, "dg4-b1-current-token")
	if err != nil || family == nil || family.FamilyID != dg4B1FixtureFamilyID {
		t.Fatalf("read B1 refresh family through repository: family=%#v err=%v", family, err)
	}
	code, err := ssoRepository.FindAuthorizationCode(ctx, dg4B1FixtureCode)
	if err != nil || code == nil || code.SessionID != dg4B1FixtureSessionID {
		t.Fatalf("read B1 authorization code through repository: code=%#v err=%v", code, err)
	}
	audit, err := ssoRepository.ListAuditEventsSince(ctx, time.Now().UTC().Add(-time.Hour))
	if err != nil || !b1ContainsAuditTrace(audit, dg4B1FixtureTraceID) {
		t.Fatalf("read B1 audit through repository: audit=%#v err=%v", audit, err)
	}

	platform, err := platformRepository.FindPlatform(ctx, dg4B1FixturePlatformCode)
	if err != nil || platform == nil || platform.PlatformCode != dg4B1FixturePlatformCode {
		t.Fatalf("read B1 platform through repository: platform=%#v err=%v", platform, err)
	}
	methods, err := platformRepository.ListLoginMethods(ctx, dg4B1FixturePlatformCode)
	if err != nil || len(methods) != 1 {
		t.Fatalf("read B1 platform login methods through repository: methods=%#v err=%v", methods, err)
	}
	rules, err := platformRepository.ListSourceRules(ctx, dg4B1FixturePlatformCode)
	if err != nil || len(rules) != 1 {
		t.Fatalf("read B1 platform source rules through repository: rules=%#v err=%v", rules, err)
	}
	roles, err := platformRepository.ListDefaultRoleRecords(ctx, dg4B1FixturePlatformCode)
	if err != nil || len(roles) != 1 {
		t.Fatalf("read B1 platform default roles through repository: roles=%#v err=%v", roles, err)
	}
	bindings, err := platformRepository.ListActiveSSOClientBindings(ctx)
	if err != nil || !b1ContainsBinding(bindings, dg4B1FixturePlatformCode, dg4B1FixtureClientID) {
		t.Fatalf("read B1 platform SSO bindings through repository: bindings=%#v err=%v", bindings, err)
	}
}

func b1UpdateAndCreateAfterRename(
	t *testing.T,
	ctx context.Context,
	provider *b1IntegrationProvider,
	ssoRepository *ssoinfra.Repository,
	platformRepository *platforminfra.Repository,
) {
	t.Helper()
	now := time.Now().UTC().Truncate(time.Microsecond)
	err := store.NewSQLXTransactor(provider.sqlxDB).WithinTransaction(ctx, func(txCtx context.Context) error {
		client, err := ssoRepository.FindClient(txCtx, dg4B1FixtureClientID)
		if err != nil || client == nil {
			return fmt.Errorf("find legacy client after rename: %w", err)
		}
		client.ClientName = "DG4 B1 Renamed Client"
		if err := ssoRepository.UpdateClient(txCtx, client, 0); err != nil {
			return err
		}
		if err := ssoRepository.ReplaceClientRedirectURIs(txCtx, dg4B1FixtureClientID, []ssodomain.ClientRedirectURI{{
			RedirectURI:           "https://dg4-b1.example.test/after",
			PostLogoutRedirectURI: "https://dg4-b1.example.test/after-logout",
			Status:                ssodomain.ClientStatusActive,
		}}, 0, now); err != nil {
			return err
		}
		if updated, err := ssoRepository.UpdateClientSecretStatus(txCtx, dg4B1FixtureClientID, dg4B1FixtureSecretID, ssodomain.ClientStatusDisabled, 0, now); err != nil || !updated {
			return fmt.Errorf("update legacy client secret after rename: updated=%t err=%w", updated, err)
		}
		if consumed, err := ssoRepository.ConsumeAuthorizationCode(txCtx, dg4B1FixtureCode, now); err != nil || !consumed {
			return fmt.Errorf("consume legacy authorization code after rename: consumed=%t err=%w", consumed, err)
		}
		if rotated, err := ssoRepository.RotateRefreshFamily(txCtx, dg4B1FixtureFamilyID, "dg4-b1-current-token", "dg4-b1-next-token", now); err != nil || !rotated {
			return fmt.Errorf("rotate legacy refresh family after rename: rotated=%t err=%w", rotated, err)
		}
		if err := ssoRepository.TouchSession(txCtx, dg4B1FixtureSessionID, now); err != nil {
			return err
		}
		if err := ssoRepository.InsertClient(txCtx, &ssodomain.Client{
			ClientID:           dg4B1FixtureClientID + "-after",
			ClientName:         "DG4 B1 New Client",
			ClientType:         "PUBLIC",
			ClientAuthMethod:   "none",
			GrantTypes:         []string{"authorization_code"},
			Scopes:             []string{"openid"},
			RequirePKCE:        true,
			AccessTokenTTLSec:  1800,
			RefreshTokenTTLSec: 2592000,
			Status:             ssodomain.ClientStatusActive,
		}, 0); err != nil {
			return err
		}

		platform, err := platformRepository.FindPlatform(txCtx, dg4B1FixturePlatformCode)
		if err != nil || platform == nil {
			return fmt.Errorf("find legacy platform after rename: %w", err)
		}
		platform.PlatformName = "DG4 B1 Renamed Platform"
		if err := platformRepository.UpdatePlatform(txCtx, *platform, 0); err != nil {
			return err
		}
		if err := platformRepository.ReplaceLoginMethods(txCtx, dg4B1FixturePlatformCode, []platformdomain.LoginMethod{{
			MethodType:     platformdomain.MethodPassword,
			DisplayName:    "Password After Rename",
			SortOrder:      1,
			DisplayEnabled: true,
			LoginEnabled:   true,
		}}, 0); err != nil {
			return err
		}
		if err := platformRepository.ReplaceSourceRules(txCtx, dg4B1FixturePlatformCode, []platformdomain.SourceRule{{
			MatchType:  platformdomain.MatchClientID,
			MatchValue: dg4B1FixtureClientID + "-after",
			Priority:   2,
			Status:     platformdomain.StatusActive,
		}}, 0); err != nil {
			return err
		}
		if err := platformRepository.ReplaceDefaultRoles(txCtx, dg4B1FixturePlatformCode, []platformdomain.DefaultRole{{
			RoleID:            1,
			AutoAssignEnabled: false,
			Status:            platformdomain.StatusActive,
		}}, 0); err != nil {
			return err
		}
		return platformRepository.InsertPlatform(txCtx, platformdomain.Platform{
			PlatformCode: dg4B1FixturePlatformCode + "-after",
			PlatformName: "DG4 B1 New Platform",
			PlatformType: "PORTAL",
			Status:       platformdomain.StatusActive,
		}, 0)
	})
	if err != nil {
		t.Fatalf("update/create B1 records after rename: %v", err)
	}
	if client, err := ssoRepository.FindClient(ctx, dg4B1FixtureClientID+"-after"); err != nil || client == nil {
		t.Fatalf("read newly created B1 client after rename: client=%#v err=%v", client, err)
	}
	if platform, err := platformRepository.FindPlatform(ctx, dg4B1FixturePlatformCode+"-after"); err != nil || platform == nil {
		t.Fatalf("read newly created B1 platform after rename: platform=%#v err=%v", platform, err)
	}
}

func b1AssertTableState(t *testing.T, ctx context.Context, db *sql.DB, dialect string, legacy bool) {
	t.Helper()
	for _, mapping := range b1TableMappings {
		legacyExists := b1TableExists(t, ctx, db, dialect, mapping.legacy)
		targetExists := b1TableExists(t, ctx, db, dialect, mapping.target)
		if legacy {
			if !legacyExists || targetExists {
				t.Fatalf("B1 pre-rename table state %s -> %s: legacy=%t target=%t", mapping.legacy, mapping.target, legacyExists, targetExists)
			}
			continue
		}
		if legacyExists || !targetExists {
			t.Fatalf("B1 post-rename table state %s -> %s: legacy=%t target=%t", mapping.legacy, mapping.target, legacyExists, targetExists)
		}
		var rows int64
		query := `SELECT COUNT(*) FROM ` + b1PhysicalTable(dialect, false, mapping.legacy)
		if err := db.QueryRowContext(ctx, query).Scan(&rows); err != nil {
			t.Fatalf("count B1 target table %s: %v", mapping.target, err)
		}
		if rows < 1 {
			t.Fatalf("B1 target table %s lost all rows", mapping.target)
		}
	}
}

func b1TableExists(t *testing.T, ctx context.Context, db *sql.DB, dialect, table string) bool {
	t.Helper()
	var count int
	query := `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE() AND table_name = ?`
	if dialect == "postgres" {
		query = `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = 'public' AND table_name = $1`
	}
	if err := db.QueryRowContext(ctx, query, table).Scan(&count); err != nil {
		t.Fatalf("inspect B1 table %s: %v", table, err)
	}
	return count == 1
}

func b1PhysicalTable(dialect string, legacy bool, legacyName string) string {
	for _, mapping := range b1TableMappings {
		if mapping.legacy != legacyName {
			continue
		}
		if legacy {
			if dialect == "postgres" {
				return `"` + mapping.legacy + `"`
			}
			return mapping.legacy
		}
		return mapping.target
	}
	panic("unknown fixed B1 table " + legacyName)
}

func b1Column(dialect, name string) string {
	if dialect == "postgres" {
		return `"` + name + `"`
	}
	return name
}

func b1Exec(ctx context.Context, exec store.SQLX, query string, args ...any) error {
	_, err := exec.ExecContext(ctx, exec.Rebind(query), args...)
	return err
}

func b1ContainsAuditTrace(events []ssodomain.AuditEvent, traceID string) bool {
	for _, event := range events {
		if event.TraceID == traceID {
			return true
		}
	}
	return false
}

func b1ContainsBinding(bindings []platformdomain.SSOClientBinding, platformCode, clientID string) bool {
	for _, binding := range bindings {
		if binding.PlatformCode == platformCode && binding.ClientID == clientID {
			return true
		}
	}
	return false
}
