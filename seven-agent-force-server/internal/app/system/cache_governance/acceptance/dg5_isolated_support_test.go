package acceptance

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"

	store "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/store"
	"github.com/jmoiron/sqlx"
	"github.com/pressly/goose/v3"
)

// This test-only support lives beside the DG5 application acceptance rather
// than under infrastructure. It deliberately duplicates the tiny immutable
// allowlist needed to prove that the application-level scenario can only
// mutate the two dedicated local governance databases.
const (
	MySQLDatabaseName     = "seven_database_governance_mysql"
	PostgresDatabaseName  = "seven_database_governance_pg"
	ForwardRecoveryResume = "resume-forward"
)

type MigrationPlan struct {
	Dialect            string
	DatabaseName       string
	CleanBaselineDir   string
	MigrationsDir      string
	UpgradeFromVersion int64
	ForwardRecovery    string
}

func MigrationPlanFor(dialect string) (MigrationPlan, error) {
	switch normalizeDialect(dialect) {
	case "mysql":
		return MigrationPlan{Dialect: "mysql", DatabaseName: MySQLDatabaseName, MigrationsDir: "migrations/mysql", UpgradeFromVersion: 20260730100000, ForwardRecovery: ForwardRecoveryResume}, nil
	case "postgres":
		return MigrationPlan{Dialect: "postgres", DatabaseName: PostgresDatabaseName, CleanBaselineDir: "migrations/postgres-baseline", MigrationsDir: "migrations/postgres", UpgradeFromVersion: 20260730100000, ForwardRecovery: ForwardRecoveryResume}, nil
	default:
		return MigrationPlan{}, fmt.Errorf("unsupported DG5 database dialect %q", dialect)
	}
}

func AssertConnectedDatabase(ctx context.Context, db *sql.DB, dialect string) error {
	if db == nil {
		return fmt.Errorf("refusing DG5 database operation: nil database")
	}
	plan, err := MigrationPlanFor(dialect)
	if err != nil {
		return err
	}
	query := "SELECT DATABASE()"
	if plan.Dialect == "postgres" {
		query = "SELECT current_database()"
	}
	var databaseName string
	if err := db.QueryRowContext(ctx, query).Scan(&databaseName); err != nil {
		return fmt.Errorf("read connected %s database name: %w", plan.Dialect, err)
	}
	if databaseName != plan.DatabaseName {
		return fmt.Errorf("refusing DG5 database operation: connected database %q, require exact %q", databaseName, plan.DatabaseName)
	}
	return nil
}

func normalizeDialect(dialect string) string {
	switch strings.ToLower(strings.TrimSpace(dialect)) {
	case "mysql":
		return "mysql"
	case "postgres", "postgresql", "pgx":
		return "postgres"
	default:
		return ""
	}
}

// protectedBatchProvider is deliberately minimal and only exists in the
// application acceptance package. It exposes the production repository and
// transaction interfaces without making datasource governance tests depend on
// application/domain/facade packages.
type protectedBatchProvider struct {
	driver  string
	dialect string
	db      *sql.DB
	sqlxDB  *sqlx.DB
}

func (p *protectedBatchProvider) Driver() string  { return p.driver }
func (p *protectedBatchProvider) Dialect() string { return p.dialect }
func (p *protectedBatchProvider) DB() *sql.DB     { return p.db }
func (p *protectedBatchProvider) SQLX() *sqlx.DB  { return p.sqlxDB }
func (p *protectedBatchProvider) Transactor() store.Transactor {
	return store.NewSQLXTransactor(p.sqlxDB)
}
func (p *protectedBatchProvider) Configured() bool { return true }
func (p *protectedBatchProvider) Close() error     { return p.db.Close() }

func assertEmptyDatabase(t *testing.T, ctx context.Context, db queryRower, dialect string) {
	t.Helper()
	var count int
	var err error
	switch normalizeDialect(dialect) {
	case "mysql":
		err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = DATABASE()`).Scan(&count)
	case "postgres":
		err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = 'public'`).Scan(&count)
	default:
		t.Fatalf("unsupported database dialect %q", dialect)
	}
	if err != nil {
		t.Fatalf("count pre-migration tables: %v", err)
	}
	if count != 0 {
		t.Fatalf("DG5 migration database must be empty, found %d tables", count)
	}
}

func latestMigrationVersion(t *testing.T, migrationsDir string) int64 {
	t.Helper()
	migrations, err := goose.CollectMigrations(migrationsDir, 0, goose.MaxVersion)
	if err != nil {
		t.Fatalf("collect migrations from %s: %v", migrationsDir, err)
	}
	if len(migrations) == 0 {
		t.Fatalf("no migrations in %s", migrationsDir)
	}
	return migrations[len(migrations)-1].Version
}

func seedUpgradeFixture(t *testing.T, ctx context.Context, db execQueryRower, dialect string) {
	t.Helper()
	const id int64 = 9202607301002
	if normalizeDialect(dialect) == "postgres" {
		if _, err := db.ExecContext(ctx, `
INSERT INTO sys_config_group
	(id, "groupCode", "groupName", "module", "status", "createTime", "updateTime")
VALUES ($1, $2, $3, $4, $5, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, id, "dg1.upgrade.fixture", "DG1 Upgrade Fixture", "governance", 1); err != nil {
			t.Fatalf("seed PostgreSQL supported-upgrade fixture: %v", err)
		}
		return
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO sys_config_group
	(id, groupCode, groupName, module, status, createTime, updateTime)
VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, id, "dg1.upgrade.fixture", "DG1 Upgrade Fixture", "governance", 1); err != nil {
		t.Fatalf("seed MySQL supported-upgrade fixture: %v", err)
	}
}

func assertUpgradeFixturePreserved(t *testing.T, ctx context.Context, db execQueryRower, dialect string) {
	t.Helper()
	const id int64 = 9202607301002
	query := `SELECT groupCode, groupName, status FROM sys_config_group WHERE id=?`
	if normalizeDialect(dialect) == "postgres" {
		query = `SELECT "groupCode", "groupName", "status" FROM sys_config_group WHERE "id"=$1`
	}
	var groupCode, groupName string
	var status int
	if err := db.QueryRowContext(ctx, query, id).Scan(&groupCode, &groupName, &status); err != nil {
		t.Fatalf("read %s supported-upgrade fixture after remaining migrations: %v", dialect, err)
	}
	if groupCode != "dg1.upgrade.fixture" || groupName != "DG1 Upgrade Fixture" || status != 1 {
		t.Fatalf("%s supported-upgrade fixture changed: code=%q name=%q status=%d", dialect, groupCode, groupName, status)
	}
}

func assertRetainedCamelCaseRoundTrip(t *testing.T, ctx context.Context, db execQueryRower, dialect string) {
	t.Helper()
	const id int64 = 9202607301001
	if normalizeDialect(dialect) == "postgres" {
		if _, err := db.ExecContext(ctx, `
INSERT INTO sys_config_group
	(id, "groupCode", "groupName", "module", "status", "createTime", "updateTime")
VALUES ($1, $2, $3, $4, $5, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, id, "dg1.camel.case", "DG1 Camel Case", "governance", 1); err != nil {
			t.Fatalf("insert PostgreSQL retained camelCase row: %v", err)
		}
		var groupCode string
		if err := db.QueryRowContext(ctx, `SELECT "groupCode" FROM sys_config_group WHERE "id"=$1`, id).Scan(&groupCode); err != nil {
			t.Fatalf("read PostgreSQL retained camelCase row: %v", err)
		}
		if groupCode != "dg1.camel.case" {
			t.Fatalf("PostgreSQL groupCode=%q", groupCode)
		}
		if _, err := db.ExecContext(ctx, `UPDATE sys_config_group SET "groupName"=$1 WHERE "id"=$2`, "DG1 Updated", id); err != nil {
			t.Fatalf("update PostgreSQL retained camelCase row: %v", err)
		}
		if _, err := db.ExecContext(ctx, `DELETE FROM sys_config_group WHERE "id"=$1`, id); err != nil {
			t.Fatalf("delete PostgreSQL retained camelCase row: %v", err)
		}
		return
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO sys_config_group
	(id, groupCode, groupName, module, status, createTime, updateTime)
VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`, id, "dg1.camel.case", "DG1 Camel Case", "governance", 1); err != nil {
		t.Fatalf("insert MySQL retained camelCase row: %v", err)
	}
	var groupCode string
	if err := db.QueryRowContext(ctx, `SELECT groupCode FROM sys_config_group WHERE id=?`, id).Scan(&groupCode); err != nil {
		t.Fatalf("read MySQL retained camelCase row: %v", err)
	}
	if groupCode != "dg1.camel.case" {
		t.Fatalf("MySQL groupCode=%q", groupCode)
	}
	if _, err := db.ExecContext(ctx, `UPDATE sys_config_group SET groupName=? WHERE id=?`, "DG1 Updated", id); err != nil {
		t.Fatalf("update MySQL retained camelCase row: %v", err)
	}
	if _, err := db.ExecContext(ctx, `DELETE FROM sys_config_group WHERE id=?`, id); err != nil {
		t.Fatalf("delete MySQL retained camelCase row: %v", err)
	}
}

type queryRower interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

type execQueryRower interface {
	queryRower
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}
