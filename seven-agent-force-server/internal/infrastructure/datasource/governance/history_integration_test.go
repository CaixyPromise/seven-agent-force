package governance

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	"github.com/pressly/goose/v3"
	"go.uber.org/zap"
)

const (
	dg1AcceptanceModeEnv       = "DG1_DATABASE_GOVERNANCE_ACCEPTANCE"
	dg3ContractModeEnv         = "DG3_DATABASE_GOVERNANCE_CONTRACT"
	dg3IntegrityBeforeContract = int64(20260730150000)
	dg3IntegrityContract       = int64(20260730160000)
	dg5CacheRefreshPermission  = int64(20260802100000)
)

func TestDG1MigrationHistory(t *testing.T) {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv(dg1AcceptanceModeEnv)))
	if mode != "clean" && mode != "upgrade" && mode != "forward-recovery" && mode != "dg3-foreign-key-present" {
		t.Skip("set DG1_DATABASE_GOVERNANCE_ACCEPTANCE=clean|upgrade|forward-recovery|dg3-foreign-key-present for the exact isolated database")
	}

	configDir, err := filepath.Abs(filepath.Join("..", "..", "..", "..", "configs"))
	if err != nil {
		t.Fatalf("resolve config directory: %v", err)
	}
	cfg, err := config.Load(configDir)
	if err != nil {
		t.Fatalf("load isolated database configuration: %v", err)
	}
	plan, err := MigrationPlanFor(cfg.Datasource.Driver)
	if err != nil {
		t.Fatalf("resolve migration plan: %v", err)
	}
	provider, err := datasource.NewProvider(cfg.Datasource, zap.NewNop())
	if err != nil {
		t.Fatalf("open isolated database: %v", err)
	}
	if provider == nil || !provider.Configured() || provider.DB() == nil {
		t.Fatal("isolated database provider is not configured")
	}
	t.Cleanup(func() { _ = provider.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if err := provider.DB().PingContext(ctx); err != nil {
		t.Fatalf("ping isolated %s database: %v", plan.Dialect, err)
	}
	if err := AssertConnectedDatabase(ctx, provider.DB(), plan.Dialect); err != nil {
		t.Fatal(err)
	}
	assertEmptyDatabase(t, ctx, provider.DB(), plan.Dialect)

	root, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	migrationsDir := filepath.Join(root, filepath.FromSlash(plan.MigrationsDir))
	baselineDir := ""
	if plan.CleanBaselineDir != "" {
		baselineDir = filepath.Join(root, filepath.FromSlash(plan.CleanBaselineDir))
	}
	goose.SetTableName(goose.DefaultTablename)
	goose.SetVerbose(false)
	if err := goose.SetDialect(plan.Dialect); err != nil {
		t.Fatalf("set %s migration dialect: %v", plan.Dialect, err)
	}

	if baselineDir != "" {
		if err := goose.UpContext(ctx, provider.DB(), baselineDir); err != nil {
			t.Fatalf("apply %s clean baseline: %v", plan.Dialect, err)
		}
	}
	if mode == "dg3-foreign-key-present" {
		if err := goose.UpToContext(ctx, provider.DB(), migrationsDir, dg3IntegrityBeforeContract); err != nil {
			t.Fatalf("apply %s DG3 pre-contract version %d: %v", plan.Dialect, dg3IntegrityBeforeContract, err)
		}
		version, err := goose.GetDBVersionContext(ctx, provider.DB())
		if err != nil || version != dg3IntegrityBeforeContract {
			t.Fatalf("DG3 pre-contract version=%d err=%v, want=%d", version, err, dg3IntegrityBeforeContract)
		}
		assertForwardOnlyDownRejected(t, ctx, provider.DB(), migrationsDir)
		assertRetainedCamelCaseRoundTrip(t, ctx, provider.DB(), plan.Dialect)
		return
	}
	if mode == "forward-recovery" {
		if baselineDir != "" {
			assertForwardOnlyDownRejected(t, ctx, provider.DB(), baselineDir)
			if err := goose.UpContext(ctx, provider.DB(), baselineDir); err != nil {
				t.Fatalf("resume forward-only baseline after rejected Down: %v", err)
			}
		} else {
			if err := goose.UpToContext(ctx, provider.DB(), migrationsDir, plan.UpgradeFromVersion); err != nil {
				t.Fatalf("apply interrupted %s history to %d: %v", plan.Dialect, plan.UpgradeFromVersion, err)
			}
			seedUpgradeFixture(t, ctx, provider.DB(), plan.Dialect)
		}
		if err := goose.UpContext(ctx, provider.DB(), migrationsDir); err != nil {
			t.Fatalf("continue forward after recovered baseline: %v", err)
		}
		if baselineDir == "" {
			assertUpgradeFixturePreserved(t, ctx, provider.DB(), plan.Dialect)
		}
		assertLatestHistoryForwardOnlyDownRejected(t, ctx, provider.DB(), migrationsDir)
		assertRetainedCamelCaseRoundTrip(t, ctx, provider.DB(), plan.Dialect)
		return
	}
	if mode == "upgrade" {
		if err := goose.UpToContext(ctx, provider.DB(), migrationsDir, plan.UpgradeFromVersion); err != nil {
			t.Fatalf("apply %s supported upgrade baseline %d: %v", plan.Dialect, plan.UpgradeFromVersion, err)
		}
		version, err := goose.GetDBVersionContext(ctx, provider.DB())
		if err != nil || version != plan.UpgradeFromVersion {
			t.Fatalf("supported upgrade version=%d err=%v, want=%d", version, err, plan.UpgradeFromVersion)
		}
		seedUpgradeFixture(t, ctx, provider.DB(), plan.Dialect)
	}
	if err := goose.UpContext(ctx, provider.DB(), migrationsDir); err != nil {
		t.Fatalf("apply complete %s migration history: %v", plan.Dialect, err)
	}
	wantVersion := latestMigrationVersion(t, migrationsDir)
	version, err := goose.GetDBVersionContext(ctx, provider.DB())
	if err != nil {
		t.Fatalf("read final %s migration version: %v", plan.Dialect, err)
	}
	if version != wantVersion {
		t.Fatalf("final %s migration version=%d, want=%d", plan.Dialect, version, wantVersion)
	}
	if mode == "upgrade" {
		assertUpgradeFixturePreserved(t, ctx, provider.DB(), plan.Dialect)
	}
	assertRetainedCamelCaseRoundTrip(t, ctx, provider.DB(), plan.Dialect)
}

func TestDG3ContractMigration(t *testing.T) {
	if strings.ToLower(strings.TrimSpace(os.Getenv(dg3ContractModeEnv))) != "apply" {
		t.Skip("set DG3_DATABASE_GOVERNANCE_CONTRACT=apply for the exact isolated database")
	}

	configDir, err := filepath.Abs(filepath.Join("..", "..", "..", "..", "configs"))
	if err != nil {
		t.Fatalf("resolve config directory: %v", err)
	}
	cfg, err := config.Load(configDir)
	if err != nil {
		t.Fatalf("load isolated database configuration: %v", err)
	}
	plan, err := MigrationPlanFor(cfg.Datasource.Driver)
	if err != nil {
		t.Fatalf("resolve migration plan: %v", err)
	}
	provider, err := datasource.NewProvider(cfg.Datasource, zap.NewNop())
	if err != nil {
		t.Fatalf("open isolated database: %v", err)
	}
	if provider == nil || !provider.Configured() || provider.DB() == nil {
		t.Fatal("isolated database provider is not configured")
	}
	t.Cleanup(func() { _ = provider.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	if err := provider.DB().PingContext(ctx); err != nil {
		t.Fatalf("ping isolated %s database: %v", plan.Dialect, err)
	}
	if err := AssertConnectedDatabase(ctx, provider.DB(), plan.Dialect); err != nil {
		t.Fatal(err)
	}
	if err := goose.SetDialect(plan.Dialect); err != nil {
		t.Fatalf("set %s migration dialect: %v", plan.Dialect, err)
	}
	goose.SetTableName(goose.DefaultTablename)
	version, err := goose.GetDBVersionContext(ctx, provider.DB())
	if err != nil || version != dg3IntegrityBeforeContract {
		t.Fatalf("DG3 contract start version=%d err=%v, want=%d", version, err, dg3IntegrityBeforeContract)
	}

	root, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	migrationsDir := filepath.Join(root, filepath.FromSlash(plan.MigrationsDir))
	if err := goose.UpToContext(ctx, provider.DB(), migrationsDir, dg3IntegrityContract); err != nil {
		t.Fatalf("apply %s DG3 contract version %d: %v", plan.Dialect, dg3IntegrityContract, err)
	}
	version, err = goose.GetDBVersionContext(ctx, provider.DB())
	if err != nil || version != dg3IntegrityContract {
		t.Fatalf("DG3 contract version=%d err=%v, want=%d", version, err, dg3IntegrityContract)
	}
	assertForwardOnlyDownRejected(t, ctx, provider.DB(), migrationsDir)
}

func assertEmptyDatabase(t *testing.T, ctx context.Context, db queryRower, dialect string) {
	t.Helper()
	var count int
	var err error
	switch normalizeDialect(dialect) {
	case "mysql":
		err = db.QueryRowContext(ctx, `
SELECT COUNT(*) FROM information_schema.tables
WHERE table_schema = DATABASE()`).Scan(&count)
	case "postgres":
		err = db.QueryRowContext(ctx, `
SELECT COUNT(*) FROM information_schema.tables
WHERE table_schema = 'public'`).Scan(&count)
	default:
		t.Fatalf("unsupported database dialect %q", dialect)
	}
	if err != nil {
		t.Fatalf("count pre-migration tables: %v", err)
	}
	if count != 0 {
		t.Fatalf("DG1 migration database must be empty, found %d tables", count)
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

func assertForwardOnlyDownRejected(t *testing.T, ctx context.Context, db *sql.DB, migrationsDir string) {
	t.Helper()
	versionBefore, err := goose.GetDBVersionContext(ctx, db)
	if err != nil {
		t.Fatalf("read version before rejected rollback: %v", err)
	}
	downErr := goose.DownContext(ctx, db, migrationsDir)
	if downErr == nil {
		t.Fatal("forward-only migration Down unexpectedly succeeded")
	}
	if !strings.Contains(strings.ToLower(downErr.Error()), "forward-only") {
		t.Fatalf("migration Down failed for an unexpected reason: %v", downErr)
	}
	versionAfter, err := goose.GetDBVersionContext(ctx, db)
	if err != nil {
		t.Fatalf("read version after rejected rollback: %v", err)
	}
	if versionAfter != versionBefore {
		t.Fatalf("rejected rollback changed version from %d to %d", versionBefore, versionAfter)
	}
}

// assertLatestHistoryForwardOnlyDownRejected walks back the two currently
// reversible configuration-asset migrations before checking the existing DG5
// forward-only boundary. It then reapplies the complete history so this
// acceptance path also proves that a safe Down followed by forward recovery
// returns to the exact latest version.
func assertLatestHistoryForwardOnlyDownRejected(t *testing.T, ctx context.Context, db *sql.DB, migrationsDir string) {
	t.Helper()
	latestVersion, err := goose.GetDBVersionContext(ctx, db)
	if err != nil {
		t.Fatalf("read latest version before rollback boundary check: %v", err)
	}
	for range 2 {
		if err := goose.DownContext(ctx, db, migrationsDir); err != nil {
			t.Fatalf("apply safe configuration-asset Down before forward-only boundary: %v", err)
		}
	}
	boundaryVersion, err := goose.GetDBVersionContext(ctx, db)
	if err != nil {
		t.Fatalf("read version at forward-only boundary: %v", err)
	}
	if boundaryVersion != dg5CacheRefreshPermission {
		t.Fatalf("forward-only boundary version=%d, want=%d", boundaryVersion, dg5CacheRefreshPermission)
	}
	assertForwardOnlyDownRejected(t, ctx, db, migrationsDir)
	if err := goose.UpContext(ctx, db, migrationsDir); err != nil {
		t.Fatalf("resume complete history after forward-only boundary check: %v", err)
	}
	restoredVersion, err := goose.GetDBVersionContext(ctx, db)
	if err != nil {
		t.Fatalf("read restored latest version: %v", err)
	}
	if restoredVersion != latestVersion {
		t.Fatalf("forward recovery restored version=%d, want=%d", restoredVersion, latestVersion)
	}
}

func seedUpgradeFixture(t *testing.T, ctx context.Context, db execQueryRower, dialect string) {
	t.Helper()
	const id int64 = 9202607301002
	if normalizeDialect(dialect) == "postgres" {
		_, err := db.ExecContext(ctx, `
INSERT INTO sys_config_group
	(id, "groupCode", "groupName", "module", "status", "createTime", "updateTime")
VALUES ($1, $2, $3, $4, $5, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
			id, "dg1.upgrade.fixture", "DG1 Upgrade Fixture", "governance", 1)
		if err != nil {
			t.Fatalf("seed PostgreSQL supported-upgrade fixture: %v", err)
		}
		return
	}
	if _, err := db.ExecContext(ctx, `
INSERT INTO sys_config_group
	(id, groupCode, groupName, module, status, createTime, updateTime)
VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		id, "dg1.upgrade.fixture", "DG1 Upgrade Fixture", "governance", 1); err != nil {
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
VALUES ($1, $2, $3, $4, $5, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
			id, "dg1.camel.case", "DG1 Camel Case", "governance", 1); err != nil {
			t.Fatalf("insert PostgreSQL retained camelCase row: %v", err)
		}
		var groupCode string
		if err := db.QueryRowContext(ctx,
			`SELECT "groupCode" FROM sys_config_group WHERE "id"=$1`, id,
		).Scan(&groupCode); err != nil {
			t.Fatalf("read PostgreSQL retained camelCase row: %v", err)
		}
		if groupCode != "dg1.camel.case" {
			t.Fatalf("PostgreSQL groupCode=%q", groupCode)
		}
		if _, err := db.ExecContext(ctx,
			`UPDATE sys_config_group SET "groupName"=$1 WHERE "id"=$2`,
			"DG1 Updated", id,
		); err != nil {
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
VALUES (?, ?, ?, ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		id, "dg1.camel.case", "DG1 Camel Case", "governance", 1); err != nil {
		t.Fatalf("insert MySQL retained camelCase row: %v", err)
	}
	var groupCode string
	if err := db.QueryRowContext(ctx,
		`SELECT groupCode FROM sys_config_group WHERE id=?`, id,
	).Scan(&groupCode); err != nil {
		t.Fatalf("read MySQL retained camelCase row: %v", err)
	}
	if groupCode != "dg1.camel.case" {
		t.Fatalf("MySQL groupCode=%q", groupCode)
	}
	if _, err := db.ExecContext(ctx,
		`UPDATE sys_config_group SET groupName=? WHERE id=?`,
		"DG1 Updated", id,
	); err != nil {
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
