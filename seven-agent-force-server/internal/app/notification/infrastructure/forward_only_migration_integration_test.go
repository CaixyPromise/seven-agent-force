//go:build integration

package infrastructure

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/mysql"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/postgres"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/pressly/goose/v3"
	"go.uber.org/zap"
)

const (
	forwardOnlyIntegrationEnv = "SEVEN_NOTIFICATION_FORWARD_ONLY_INTEGRATION"
	forwardOnlyMySQLDSNEnv    = "SEVEN_NOTIFICATION_FORWARD_ONLY_MYSQL_DSN"
	forwardOnlyPostgresDSNEnv = "SEVEN_NOTIFICATION_FORWARD_ONLY_POSTGRES_DSN"
	forwardOnlyDatabasePrefix = "seven_notification_forward_guard"
)

var forwardOnlyMigrationMu sync.Mutex

func TestNotificationForwardOnlyMySQLDownPreservesGooseVersion(t *testing.T) {
	if strings.TrimSpace(os.Getenv(forwardOnlyIntegrationEnv)) != "1" {
		t.Skip("set SEVEN_NOTIFICATION_FORWARD_ONLY_INTEGRATION=1 to run isolated migration probes")
	}
	dsn := strings.TrimSpace(os.Getenv(forwardOnlyMySQLDSNEnv))
	parsed, err := mysqldriver.ParseDSN(dsn)
	if err != nil || !strings.HasPrefix(parsed.DBName, forwardOnlyDatabasePrefix) {
		t.Fatalf("forward-only MySQL test requires an isolated %s* database, got %q: %v", forwardOnlyDatabasePrefix, parsed.DBName, err)
	}
	provider, err := mysql.NewProvider(config.MySQLConfig{Enabled: true, DSN: dsn, MaxOpenConns: 2, MaxIdleConns: 1}, zap.NewNop())
	if err != nil {
		t.Fatalf("open isolated MySQL database: %v", err)
	}
	t.Cleanup(func() { _ = provider.Close() })
	runForwardOnlyMigrationGuards(t, provider.DB(), "mysql")
}

func TestNotificationForwardOnlyPostgresDownPreservesGooseVersion(t *testing.T) {
	if strings.TrimSpace(os.Getenv(forwardOnlyIntegrationEnv)) != "1" {
		t.Skip("set SEVEN_NOTIFICATION_FORWARD_ONLY_INTEGRATION=1 to run isolated migration probes")
	}
	dsn := strings.TrimSpace(os.Getenv(forwardOnlyPostgresDSNEnv))
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse isolated PostgreSQL DSN: %v", err)
	}
	databaseName := strings.TrimPrefix(parsed.Path, "/")
	if !strings.HasPrefix(databaseName, forwardOnlyDatabasePrefix) {
		t.Fatalf("forward-only PostgreSQL test requires an isolated %s* database, got %q", forwardOnlyDatabasePrefix, databaseName)
	}
	provider, err := postgres.NewProvider(config.PostgresConfig{Enabled: true, DSN: dsn, MaxOpenConns: 2, MaxIdleConns: 1}, zap.NewNop())
	if err != nil {
		t.Fatalf("open isolated PostgreSQL database: %v", err)
	}
	t.Cleanup(func() { _ = provider.Close() })
	runForwardOnlyMigrationGuards(t, provider.DB(), "postgres")
}

func runForwardOnlyMigrationGuards(t *testing.T, db *sql.DB, dialect string) {
	t.Helper()
	forwardOnlyMigrationMu.Lock()
	defer forwardOnlyMigrationMu.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := resetForwardOnlyFixture(ctx, db, dialect); err != nil {
		t.Fatalf("reset %s forward-only fixture: %v", dialect, err)
	}
	t.Cleanup(func() { _ = resetForwardOnlyFixture(context.Background(), db, dialect) })

	for _, fixture := range []struct {
		version int64
		name    string
	}{
		{version: 20260722110000, name: "notification_core_inbox"},
		{version: 20260722120000, name: "notification_mailbox_sync"},
		{version: 20260723110000, name: "notification_inbox_expiry_sync"},
		{version: 20260723120000, name: "notification_external_app_delivery"},
		{version: 20260727130000, name: "notification_http_connector_delivery"},
		{version: 20260727150000, name: "notification_template_revisions"},
		{version: 20260727160000, name: "notification_scene_revisions"},
		{version: 20260727170000, name: "notification_delivery_diagnostics"},
	} {
		source := filepath.Join("..", "..", "..", "..", "migrations", dialect, fmt.Sprintf("%d_%s.sql", fixture.version, fixture.name))
		payload, err := os.ReadFile(source)
		if err != nil {
			t.Fatalf("read %s migration: %v", dialect, err)
		}
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, filepath.Base(source)), payload, 0o600); err != nil {
			t.Fatalf("copy %s migration: %v", dialect, err)
		}
		table := fmt.Sprintf("goose_forward_guard_%d", fixture.version)
		goose.SetTableName(table)
		if err := goose.SetDialect(dialect); err != nil {
			t.Fatalf("set %s dialect: %v", dialect, err)
		}
		if err := goose.UpContext(ctx, db, dir); err != nil {
			t.Fatalf("apply %s migration %d: %v", dialect, fixture.version, err)
		}
		if version, err := goose.GetDBVersionContext(ctx, db); err != nil || version != fixture.version {
			t.Fatalf("%s migration %d version before Down=%d err=%v", dialect, fixture.version, version, err)
		}
		if err := goose.DownContext(ctx, db, dir); err == nil {
			t.Fatalf("%s migration %d Down unexpectedly succeeded", dialect, fixture.version)
		}
		if version, err := goose.GetDBVersionContext(ctx, db); err != nil || version != fixture.version {
			t.Fatalf("%s migration %d version after rejected Down=%d err=%v", dialect, fixture.version, version, err)
		}
		if err := goose.UpContext(ctx, db, dir); err != nil {
			t.Fatalf("%s migration %d no-op recovery Up: %v", dialect, fixture.version, err)
		}
	}
}

func resetForwardOnlyFixture(ctx context.Context, db *sql.DB, dialect string) error {
	statements := []string{
		`DROP TABLE IF EXISTS goose_forward_guard_20260722110000`,
		`DROP TABLE IF EXISTS goose_forward_guard_20260722120000`,
		`DROP TABLE IF EXISTS goose_forward_guard_20260723110000`,
		`DROP TABLE IF EXISTS goose_forward_guard_20260723120000`,
		`DROP TABLE IF EXISTS goose_forward_guard_20260727130000`,
		`DROP TABLE IF EXISTS goose_forward_guard_20260727150000`,
		`DROP TABLE IF EXISTS goose_forward_guard_20260727160000`,
		`DROP TABLE IF EXISTS goose_forward_guard_20260727170000`,
		`DROP TABLE IF EXISTS sysNotificationDeliveryDiagnosticAudit`,
		`DROP TABLE IF EXISTS sysNotificationDeliveryEphemeralContent`,
		`DROP TABLE IF EXISTS sysNotificationSceneSnapshot`,
		`DROP TABLE IF EXISTS sysNotificationSceneRevisionAudit`,
		`DROP TABLE IF EXISTS sysNotificationSceneRevision`,
		`DROP TABLE IF EXISTS sysNotificationSceneDefinition`,
		`DROP TABLE IF EXISTS sysNotificationTemplateRevisionAudit`,
		`DROP TABLE IF EXISTS sysNotificationTemplateRevision`,
		`DROP TABLE IF EXISTS sysNotificationTemplateDefinition`,
		`DROP TABLE IF EXISTS sysNotificationHTTPDeliverySnapshot`,
		`DROP TABLE IF EXISTS sysNotificationDeliveryAttempt`,
		`DROP TABLE IF EXISTS sysNotificationExternalTarget`,
		`DROP TABLE IF EXISTS sysNotificationMailbox`,
		`DROP TABLE IF EXISTS sysNotificationMaterializationTask`,
		`DROP TABLE IF EXISTS sysNotificationRecipient`,
		`DROP TABLE IF EXISTS sysNotification`,
		`DROP TABLE IF EXISTS sysNotificationDelivery`,
		`DROP TABLE IF EXISTS sysNotificationChannel`,
		`DROP TABLE IF EXISTS sys_role_permission`,
		`DROP TABLE IF EXISTS sys_permission`,
		`DROP TABLE IF EXISTS sys_role`,
	}
	if dialect == "postgres" {
		statements = []string{
			`DROP TABLE IF EXISTS "goose_forward_guard_20260722110000"`,
			`DROP TABLE IF EXISTS "goose_forward_guard_20260722120000"`,
			`DROP TABLE IF EXISTS "goose_forward_guard_20260723110000"`,
			`DROP TABLE IF EXISTS "goose_forward_guard_20260723120000"`,
			`DROP TABLE IF EXISTS "goose_forward_guard_20260727130000"`,
			`DROP TABLE IF EXISTS "goose_forward_guard_20260727150000"`,
			`DROP TABLE IF EXISTS "goose_forward_guard_20260727160000"`,
			`DROP TABLE IF EXISTS "goose_forward_guard_20260727170000"`,
			`DROP TABLE IF EXISTS "sysNotificationDeliveryDiagnosticAudit"`,
			`DROP TABLE IF EXISTS "sysNotificationDeliveryEphemeralContent"`,
			`DROP TABLE IF EXISTS "sysNotificationSceneSnapshot"`,
			`DROP TABLE IF EXISTS "sysNotificationSceneRevisionAudit"`,
			`DROP TABLE IF EXISTS "sysNotificationSceneRevision"`,
			`DROP TABLE IF EXISTS "sysNotificationSceneDefinition"`,
			`DROP TABLE IF EXISTS "sysNotificationTemplateRevisionAudit"`,
			`DROP TABLE IF EXISTS "sysNotificationTemplateRevision"`,
			`DROP TABLE IF EXISTS "sysNotificationTemplateDefinition"`,
			`DROP TABLE IF EXISTS "sysNotificationHTTPDeliverySnapshot"`,
			`DROP TABLE IF EXISTS "sysNotificationDeliveryAttempt"`,
			`DROP TABLE IF EXISTS "sysNotificationExternalTarget"`,
			`DROP TABLE IF EXISTS "sysNotificationMailbox"`,
			`DROP TABLE IF EXISTS "sysNotificationMaterializationTask"`,
			`DROP TABLE IF EXISTS "sysNotificationRecipient"`,
			`DROP TABLE IF EXISTS "sysNotification"`,
			`DROP TABLE IF EXISTS "sysNotificationDelivery"`,
			`DROP TABLE IF EXISTS "sysNotificationChannel"`,
		}
	}
	for _, statement := range statements {
		if _, err := db.ExecContext(ctx, statement); err != nil {
			return err
		}
	}
	fixtures := []string{
		`CREATE TABLE sysNotificationChannel (id BIGINT PRIMARY KEY, channelType varchar(32) NOT NULL)`,
		`CREATE TABLE sysNotificationDelivery (
			id BIGINT PRIMARY KEY,
			requestDigest varchar(64),
			renderedMarkdown TEXT NULL,
			lastError varchar(255),
			createTime DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE sys_permission (
			id BIGINT PRIMARY KEY,
			code varchar(191) NOT NULL,
			name varchar(191) NOT NULL,
			resourceType varchar(32) NOT NULL,
			method varchar(16) NOT NULL,
			path varchar(512) NOT NULL,
			status INT NOT NULL,
			description varchar(512),
			creatorId BIGINT,
			createTime DATETIME,
			updaterId BIGINT,
			updateTime DATETIME,
			isDeleted TINYINT NOT NULL
		)`,
		`CREATE TABLE sys_role (
			id BIGINT PRIMARY KEY,
			systemKey varchar(128),
			isDeleted TINYINT NOT NULL
		)`,
		`CREATE TABLE sys_role_permission (
			id BIGINT PRIMARY KEY,
			roleId BIGINT NOT NULL,
			permissionId BIGINT NOT NULL,
			source varchar(16),
			creatorId BIGINT,
			createTime DATETIME,
			updateTime DATETIME
		)`,
	}
	if dialect == "postgres" {
		fixtures = []string{
			`CREATE TABLE "sysNotificationChannel" ("id" BIGINT PRIMARY KEY, "channelType" varchar(32) NOT NULL)`,
			`CREATE TABLE "sysNotificationDelivery" (
				"id" BIGINT PRIMARY KEY,
				"requestDigest" varchar(64),
				"renderedMarkdown" text,
				"lastError" varchar(255),
				"createTime" timestamptz NOT NULL DEFAULT CURRENT_TIMESTAMP
			)`,
		}
	}
	for _, fixture := range fixtures {
		if _, err := db.ExecContext(ctx, fixture); err != nil {
			return err
		}
	}
	return nil
}
