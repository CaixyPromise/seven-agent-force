package infrastructure

import (
	"context"
	"database/sql"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/pressly/goose/v3"
)

const (
	dc23PostgresBaselineTestDSNEnv = "DC23_POSTGRES_BASELINE_TEST_DSN"
	dc23PostgresBaselineDBPrefix   = "seven_dc23_pg_json_"
)

func TestPostgresScalarFoundationAgainstCleanInstallBaseline(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv(dc23PostgresBaselineTestDSNEnv))
	if dsn == "" {
		t.Skip("set DC23_POSTGRES_BASELINE_TEST_DSN to an empty isolated PostgreSQL database")
	}
	parsed, err := url.Parse(dsn)
	if err != nil {
		t.Fatalf("parse PostgreSQL baseline test DSN: %v", err)
	}
	databaseName := strings.TrimPrefix(parsed.Path, "/")
	if !strings.HasPrefix(databaseName, dc23PostgresBaselineDBPrefix) {
		t.Fatalf("baseline test requires an isolated %s* database, got %q", dc23PostgresBaselineDBPrefix, databaseName)
	}

	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open PostgreSQL baseline test database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("ping PostgreSQL baseline test database: %v", err)
	}
	var publicTableCount int
	if err := db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM information_schema.tables
		WHERE table_schema = 'public'
	`).Scan(&publicTableCount); err != nil {
		t.Fatalf("count public tables before baseline: %v", err)
	}
	if publicTableCount != 0 {
		t.Fatalf("baseline test database must be empty, found %d public tables", publicTableCount)
	}

	baselineDir := migrationFixtureDir(t, "../../../../../migrations/postgres-baseline/20260719110000_clean_install_baseline.sql")
	scalarDir := migrationFixtureDir(t, "../../../../../migrations/postgres/20260730100000_scalar_configuration_foundation.sql")
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatalf("set PostgreSQL Goose dialect: %v", err)
	}
	goose.SetTableName(goose.DefaultTablename)
	goose.SetVerbose(false)

	if err := goose.UpContext(ctx, db, baselineDir); err != nil {
		t.Fatalf("apply real PostgreSQL clean-install baseline: %v", err)
	}
	assertPostgresColumnType(t, ctx, db, "sys_dict_item", "extJson", "json")

	if err := goose.UpContext(ctx, db, scalarDir); err != nil {
		t.Fatalf("apply scalar foundation after real clean-install baseline: %v", err)
	}
	assertPostgresDictionaryPresentation(t, ctx, db)

	if err := goose.DownContext(ctx, db, scalarDir); err != nil {
		t.Fatalf("down scalar foundation after clean-install baseline: %v", err)
	}
	if err := goose.UpContext(ctx, db, scalarDir); err != nil {
		t.Fatalf("re-up scalar foundation after clean-install baseline: %v", err)
	}
	assertPostgresDictionaryPresentation(t, ctx, db)
}

func migrationFixtureDir(t *testing.T, source string) string {
	t.Helper()
	absoluteSource, err := filepath.Abs(source)
	if err != nil {
		t.Fatalf("resolve migration %s: %v", source, err)
	}
	if _, err := os.Stat(absoluteSource); err != nil {
		t.Fatalf("stat migration %s: %v", absoluteSource, err)
	}
	dir := t.TempDir()
	if err := os.Symlink(absoluteSource, filepath.Join(dir, filepath.Base(absoluteSource))); err != nil {
		t.Fatalf("link migration %s: %v", absoluteSource, err)
	}
	return dir
}

func assertPostgresColumnType(t *testing.T, ctx context.Context, db *sql.DB, tableName, columnName, want string) {
	t.Helper()
	var got string
	if err := db.QueryRowContext(ctx, `
		SELECT data_type
		FROM information_schema.columns
		WHERE table_schema = 'public' AND table_name = $1 AND column_name = $2
	`, tableName, columnName).Scan(&got); err != nil {
		t.Fatalf("read %s.%s type: %v", tableName, columnName, err)
	}
	if got != want {
		t.Fatalf("%s.%s type=%q, want %q", tableName, columnName, got, want)
	}
}

func assertPostgresDictionaryPresentation(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()
	var colorToken, iconToken string
	if err := db.QueryRowContext(ctx, `
		SELECT "colorToken", "iconToken"
		FROM sys_dict_item
		WHERE "itemValue" = '1' AND "dictTypeId" = 2026042501001
	`).Scan(&colorToken, &iconToken); err != nil {
		t.Fatalf("read migrated clean-baseline dictionary presentation: %v", err)
	}
	if colorToken != "blue" || iconToken != "male" {
		t.Fatalf("migrated dictionary presentation=(%q,%q), want (blue,male)", colorToken, iconToken)
	}
}
