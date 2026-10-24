package external_login

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	externalapp "github.com/CaixyPromise/seven-agent/force/internal/app/external_login/application"
	"github.com/CaixyPromise/seven-agent/force/internal/app/external_login/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/external_login/facade"
	externalinfra "github.com/CaixyPromise/seven-agent/force/internal/app/external_login/infrastructure"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/mysql"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	"github.com/pressly/goose/v3"
)

const task7ExactIdentityMigrationVersion int64 = 20260712130000

func TestMySQLExternalIdentityExactSubjectMigrationRollback(t *testing.T) {
	dsn := os.Getenv("TASK7_MYSQL_DSN")
	if dsn == "" {
		t.Skip("TASK7_MYSQL_DSN is not set")
	}
	provider, err := mysql.NewProvider(config.MySQLConfig{Enabled: true, DSN: dsn, MaxOpenConns: 1, MaxIdleConns: 1}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = provider.Close() })
	db := provider.SQLX().DB
	ctx := context.Background()
	migrationsDir, err := filepath.Abs("../../../migrations/mysql")
	if err != nil {
		t.Fatal(err)
	}
	goose.SetTableName("goose_db_version")
	if err := goose.SetDialect("mysql"); err != nil {
		t.Fatal(err)
	}
	if err := goose.UpToContext(ctx, db, migrationsDir, task7ExactIdentityMigrationVersion); err != nil {
		t.Fatalf("prepare migration: %v", err)
	}
	const providerCode = "task7-rollback-probe"
	const issuer = "https://task7-rollback.example"
	if _, err := db.ExecContext(ctx, `DELETE FROM sysExternalUserIdentity WHERE providerCode IN (?, 'hub:scratch-node', 'hub:scratch-other')`, providerCode); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.ExecContext(context.Background(), `DELETE FROM sysExternalUserIdentity WHERE providerCode = ?`, providerCode)
	})

	t.Run("ordinary down and re-up", func(t *testing.T) {
		assertTask7MigrationVersion(t, ctx, db, task7ExactIdentityMigrationVersion)
		assertTask7RollbackPreflightExists(t, ctx, db, true)
		if err := goose.DownContext(ctx, db, migrationsDir); err != nil {
			t.Fatalf("ordinary down: %v", err)
		}
		assertTask7MigrationVersion(t, ctx, db, 20260712120000)
		assertTask7RollbackPreflightExists(t, ctx, db, false)
		if err := goose.UpToContext(ctx, db, migrationsDir, task7ExactIdentityMigrationVersion); err != nil {
			t.Fatalf("ordinary re-up: %v", err)
		}
		assertTask7MigrationVersion(t, ctx, db, task7ExactIdentityMigrationVersion)
		assertTask7RollbackPreflightExists(t, ctx, db, true)
	})

	for i, subject := range []string{"Alice", "alice"} {
		if _, err := db.ExecContext(ctx, `INSERT INTO sysExternalUserIdentity (providerCode, externalIssuer, externalSubject, userId, status, firstLinkedAt) VALUES (?, ?, ?, ?, 0, NOW())`, providerCode, issuer, subject, 92001+i); err != nil {
			t.Fatalf("insert case-distinct identity %q: %v", subject, err)
		}
	}

	t.Run("conflict down is atomic no-change", func(t *testing.T) {
		beforeIdentitySchema := showCreateTable(t, ctx, db, "sysExternalUserIdentity")
		beforeStateSchema := showCreateTable(t, ctx, db, "sysExternalOAuthLoginState")
		beforeRows := task7RollbackProbeRows(t, ctx, db, providerCode)

		err := goose.DownContext(ctx, db, migrationsDir)
		if err == nil {
			t.Fatal("conflicting down succeeded")
		}
		if !strings.Contains(err.Error(), "resolve case-insensitive external identity conflicts before retry") {
			t.Fatalf("down error is not actionable: %v", err)
		}
		assertTask7MigrationVersion(t, ctx, db, task7ExactIdentityMigrationVersion)
		assertTask7RollbackPreflightExists(t, ctx, db, true)
		if got := showCreateTable(t, ctx, db, "sysExternalUserIdentity"); got != beforeIdentitySchema {
			t.Fatal("identity schema changed after rejected down")
		}
		if got := showCreateTable(t, ctx, db, "sysExternalOAuthLoginState"); got != beforeStateSchema {
			t.Fatal("login state schema changed after rejected down")
		}
		if got := task7RollbackProbeRows(t, ctx, db, providerCode); got != beforeRows {
			t.Fatalf("identity data changed after rejected down: before=%q after=%q", beforeRows, got)
		}
	})

	t.Run("resolve conflict then down and re-up", func(t *testing.T) {
		if _, err := db.ExecContext(ctx, `DELETE FROM sysExternalUserIdentity WHERE providerCode = ? AND externalSubject = BINARY ?`, providerCode, "alice"); err != nil {
			t.Fatal(err)
		}
		if err := goose.DownContext(ctx, db, migrationsDir); err != nil {
			t.Fatalf("down after conflict resolution: %v", err)
		}
		assertTask7MigrationVersion(t, ctx, db, 20260712120000)
		assertTask7RollbackPreflightExists(t, ctx, db, false)
		if err := goose.UpToContext(ctx, db, migrationsDir, task7ExactIdentityMigrationVersion); err != nil {
			t.Fatalf("re-up after conflict resolution: %v", err)
		}
		assertTask7MigrationVersion(t, ctx, db, task7ExactIdentityMigrationVersion)
		assertTask7RollbackPreflightExists(t, ctx, db, true)
		if got := task7RollbackProbeRows(t, ctx, db, providerCode); got != "Alice:92001" {
			t.Fatalf("resolved identity was not preserved across down/re-up: %q", got)
		}
	})
}

func assertTask7RollbackPreflightExists(t *testing.T, ctx context.Context, db *sql.DB, want bool) {
	t.Helper()
	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.ROUTINES WHERE ROUTINE_SCHEMA = DATABASE() AND ROUTINE_TYPE = 'PROCEDURE' AND ROUTINE_NAME = 'task7ExactIdentityRollbackPreflight'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if got := count == 1; got != want {
		t.Fatalf("rollback preflight exists=%t want=%t", got, want)
	}
}

func assertTask7MigrationVersion(t *testing.T, ctx context.Context, db *sql.DB, want int64) {
	t.Helper()
	got, err := goose.GetDBVersionContext(ctx, db)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("migration version=%d want=%d", got, want)
	}
}

func showCreateTable(t *testing.T, ctx context.Context, db *sql.DB, table string) string {
	t.Helper()
	var name, ddl string
	if err := db.QueryRowContext(ctx, "SHOW CREATE TABLE "+table).Scan(&name, &ddl); err != nil {
		t.Fatal(err)
	}
	return ddl
}

func task7RollbackProbeRows(t *testing.T, ctx context.Context, db *sql.DB, providerCode string) string {
	t.Helper()
	rows, err := db.QueryContext(ctx, `SELECT externalSubject, userId FROM sysExternalUserIdentity WHERE providerCode = ? ORDER BY BINARY externalSubject`, providerCode)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var values []string
	for rows.Next() {
		var subject string
		var userID int64
		if err := rows.Scan(&subject, &userID); err != nil {
			t.Fatal(err)
		}
		values = append(values, subject+":"+fmt.Sprint(userID))
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return strings.Join(values, ",")
}

func TestMySQLManagedOIDCProviderProjectionAndIdentityLookup(t *testing.T) {
	dsn := os.Getenv("TASK7_MYSQL_DSN")
	if dsn == "" {
		t.Skip("TASK7_MYSQL_DSN is not set")
	}
	provider, err := mysql.NewProvider(config.MySQLConfig{Enabled: true, DSN: dsn, MaxOpenConns: 4, MaxIdleConns: 2}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = provider.Close() })
	repository, err := externalinfra.NewRepository(provider)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	fixtureCleanup := []string{
		`DELETE FROM sysExternalUserIdentity WHERE providerCode IN ('hub:scratch-node','hub:scratch-other')`,
		`DELETE FROM sysExternalProviderMethod WHERE providerCode IN ('hub:scratch-node','hub:scratch-other')`,
		`DELETE FROM sysExternalManagedProviderCommand WHERE providerCode IN ('hub:scratch-node','hub:scratch-other')`,
		`DELETE FROM sysExternalLoginProvider WHERE providerCode IN ('hub:scratch-node','hub:scratch-other')`,
	}
	for _, statement := range fixtureCleanup {
		if _, err := provider.SQLX().ExecContext(ctx, statement); err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		for _, statement := range fixtureCleanup {
			_, _ = provider.SQLX().ExecContext(context.Background(), statement)
		}
	})
	service := externalapp.NewService(externalapp.ServiceDeps{
		Transactor: provider.Transactor(), Repository: repositoryAdapter{Repository: repository},
		Discovery: mysqlProbeDiscovery{}, SecretValue: mysqlProbeSecretValue{},
	})
	command := facade.ManagedOIDCProviderCommand{
		OwnerNodeCode: "scratch-node", ConnectionVersion: "v1", Enabled: true, DisplayName: "Scratch Hub",
		Issuer: "https://scratch-hub.example", ClientID: "hub-node-scratch-node", ClientSecret: "scratch-secret",
		RedirectURI: "https://scratch-node.example/callback",
	}
	if err := service.ApplyManagedOIDCProvider(ctx, command); err != nil {
		t.Fatalf("apply managed provider: %v", err)
	}
	if err := service.ApplyManagedOIDCProvider(ctx, command); err != nil {
		t.Fatalf("replay managed provider: %v", err)
	}
	next := command
	next.ConnectionVersion = "v2"
	next.TargetRevision = 2
	if err := service.ApplyManagedOIDCProvider(ctx, next); err != nil {
		t.Fatalf("apply next managed provider generation: %v", err)
	}
	stored, err := repository.FindProvider(ctx, "hub:scratch-node")
	if err != nil || stored == nil {
		t.Fatalf("read managed provider=%#v err=%v", stored, err)
	}
	var metadata struct {
		OwnerNodeCode      string `json:"ownerNodeCode"`
		PersistLoginTokens bool   `json:"persistLoginTokens"`
	}
	if err := json.Unmarshal([]byte(stored.MetadataJSON), &metadata); err != nil {
		t.Fatalf("decode managed metadata: %v", err)
	}
	if stored.Issuer != command.Issuer || stored.ClientID != command.ClientID || stored.ClientSecretCiphertext == command.ClientSecret ||
		stored.TokenEndpointAuthMethod != domain.TokenEndpointAuthMethodClientSecretBasic ||
		metadata.OwnerNodeCode != "scratch-node" || metadata.PersistLoginTokens {
		t.Fatalf("managed provider projection=%#v", stored)
	}
	now := time.Now().UTC()
	identity := &domain.ExternalIdentity{ProviderCode: stored.ProviderCode, ExternalIssuer: stored.Issuer, ExternalSubject: "scratch-sub", UserID: 91001, Status: domain.IdentityStatusActive, FirstLinkedAt: now}
	if err := repository.InsertIdentity(ctx, identity, identity.UserID); err != nil {
		t.Fatalf("insert managed identity: %v", err)
	}
	found, err := repository.FindIdentityBySubject(ctx, stored.ProviderCode, stored.Issuer, identity.ExternalSubject)
	if err != nil || found == nil || found.UserID != identity.UserID || found.ExternalIssuer != stored.Issuer {
		t.Fatalf("issuer identity lookup=%#v err=%v", found, err)
	}
	caseVariant := &domain.ExternalIdentity{ProviderCode: stored.ProviderCode, ExternalIssuer: stored.Issuer, ExternalSubject: "Scratch-Sub", UserID: 91002, Status: domain.IdentityStatusActive, FirstLinkedAt: now}
	if err := repository.InsertIdentity(ctx, caseVariant, caseVariant.UserID); err != nil {
		t.Fatalf("insert case-distinct managed identity: %v", err)
	}
	for subject, wantUserID := range map[string]int64{"scratch-sub": 91001, "Scratch-Sub": 91002} {
		got, lookupErr := repository.FindIdentityBySubject(ctx, stored.ProviderCode, stored.Issuer, subject)
		if lookupErr != nil || got == nil || got.UserID != wantUserID || got.ExternalSubject != subject {
			t.Fatalf("exact subject lookup %q=%#v err=%v", subject, got, lookupErr)
		}
	}
	changed := command
	changed.ConnectionVersion = "v2"
	changed.Issuer = "https://replacement-hub.example"
	if err := service.ApplyManagedOIDCProvider(ctx, changed); err == nil {
		t.Fatal("managed issuer changed after first identity")
	}
	if err := service.DisableManagedOIDCProvider(ctx, "scratch-node", "v3", 3); err != nil {
		t.Fatalf("disable managed provider: %v", err)
	}
	disabled, err := repository.FindProvider(ctx, stored.ProviderCode)
	if err != nil || disabled == nil || disabled.Status != domain.ProviderStatusDisabled || disabled.LoginEnabled || disabled.DisplayEnabled {
		t.Fatalf("disabled managed provider=%#v err=%v", disabled, err)
	}
}

type mysqlProbeDiscovery struct{}

func (mysqlProbeDiscovery) DiscoverOIDC(_ context.Context, issuer string) (externalapp.OIDCDiscoveryResult, error) {
	return externalapp.OIDCDiscoveryResult{Issuer: issuer, AuthorizationEndpoint: issuer + "/authorize", TokenEndpoint: issuer + "/token", UserinfoEndpoint: issuer + "/userinfo", JWKSURI: issuer + "/jwks"}, nil
}

type mysqlProbeSecretValue struct{}

func (mysqlProbeSecretValue) EncryptString(_ context.Context, plain string) (externalapp.EncryptedSecretValue, error) {
	return externalapp.EncryptedSecretValue{CiphertextB64: "cipher:" + plain, EDEKB64: "scratch-edek", WrapKeyRef: "scratch-key"}, nil
}
func (mysqlProbeSecretValue) DecryptString(_ context.Context, value externalapp.EncryptedSecretValue) (string, error) {
	return strings.TrimPrefix(value.CiphertextB64, "cipher:"), nil
}
func (mysqlProbeSecretValue) EncryptBytes(ctx context.Context, plain []byte) (externalapp.EncryptedSecretValue, error) {
	return mysqlProbeSecretValue{}.EncryptString(ctx, string(plain))
}
func (mysqlProbeSecretValue) DecryptBytes(ctx context.Context, value externalapp.EncryptedSecretValue) ([]byte, error) {
	plain, err := mysqlProbeSecretValue{}.DecryptString(ctx, value)
	return []byte(plain), err
}
