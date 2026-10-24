package acceptance

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"testing"
	"time"

	sharedconfig "github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/go-viper/mapstructure/v2"
	"github.com/jackc/pgx/v5"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
	"github.com/pressly/goose/v3"
	"github.com/spf13/viper"
)

const (
	dg5AcceptanceEnv      = "DG5_CACHE_GOVERNANCE_ACCEPTANCE"
	dg5DialectEnv         = "DG5_CACHE_GOVERNANCE_DIALECT"
	dg5MigrationModeEnv   = "DG5_CACHE_GOVERNANCE_MIGRATION_MODE"
	dg5MySQLLocalDSNEnv   = "DG5_MYSQL_LOCAL_DSN"
	dg5PostgresSocketDir  = "/tmp"
	dg5PostgresSocketPort = uint16(5432)
)

type dg5IsolatedDatabase struct {
	dialect  string
	driver   string
	dsn      string
	db       *sql.DB
	provider *protectedBatchProvider
	cfg      sharedconfig.Config
}

// TestDG5MigrationCleanAndUpgrade is intentionally opt-in and destructive
// only inside the exact governance database selected by the server-side
// guard. It builds both the empty-database and supported-upgrade paths from
// the local developer connection settings without ever reusing the configured
// application database name.
func TestDG5MigrationCleanAndUpgrade(t *testing.T) {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv(dg5MigrationModeEnv)))
	if mode != "clean" && mode != "upgrade" && mode != "forward-recovery" {
		t.Skip("set DG5_CACHE_GOVERNANCE_MIGRATION_MODE=clean|upgrade|forward-recovery together with the exact isolated dialect")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	target := openDG5IsolatedDatabase(t, ctx)
	t.Cleanup(func() { _ = target.db.Close() })
	if err := AssertConnectedDatabase(ctx, target.db, target.dialect); err != nil {
		t.Fatal(err)
	}
	resetDG5IsolatedSchema(t, ctx, target.db, target.dialect)
	assertEmptyDatabase(t, ctx, target.db, target.dialect)

	plan, err := MigrationPlanFor(target.dialect)
	if err != nil {
		t.Fatalf("resolve DG5 migration plan: %v", err)
	}
	root, err := filepath.Abs(filepath.Join("..", "..", "..", "..", ".."))
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
		t.Fatalf("set %s goose dialect: %v", plan.Dialect, err)
	}
	if baselineDir != "" {
		if err := goose.UpContext(ctx, target.db, baselineDir); err != nil {
			t.Fatalf("apply %s clean baseline: %v", target.dialect, err)
		}
	}
	if mode == "upgrade" || mode == "forward-recovery" {
		if err := goose.UpToContext(ctx, target.db, migrationsDir, plan.UpgradeFromVersion); err != nil {
			t.Fatalf("apply %s %s baseline %d: %v", target.dialect, mode, plan.UpgradeFromVersion, err)
		}
		version, versionErr := goose.GetDBVersionContext(ctx, target.db)
		if versionErr != nil || version != plan.UpgradeFromVersion {
			t.Fatalf("%s baseline version=%d err=%v, want=%d", mode, version, versionErr, plan.UpgradeFromVersion)
		}
		seedUpgradeFixture(t, ctx, target.db, target.dialect)
		if mode == "forward-recovery" {
			reopenDG5IsolatedDatabase(t, ctx, target)
			t.Logf("DG5 %s migration resumed after an isolated process restart at version %d", target.dialect, plan.UpgradeFromVersion)
		}
	}
	if err := goose.UpContext(ctx, target.db, migrationsDir); err != nil {
		t.Fatalf("apply complete %s migration history: %v", target.dialect, err)
	}
	version, err := goose.GetDBVersionContext(ctx, target.db)
	if err != nil {
		t.Fatalf("read final %s migration version: %v", target.dialect, err)
	}
	if want := latestMigrationVersion(t, migrationsDir); version != want {
		t.Fatalf("final %s migration version=%d, want=%d", target.dialect, version, want)
	}
	assertDG5ForwardOnlyDownRejected(t, ctx, target.db, migrationsDir, version)
	if mode == "upgrade" || mode == "forward-recovery" {
		assertUpgradeFixturePreserved(t, ctx, target.db, target.dialect)
	}
	assertRetainedCamelCaseRoundTrip(t, ctx, target.db, target.dialect)
	t.Logf("DG5 %s migration path verified against isolated %s database", mode, target.dialect)
}

// reopenDG5IsolatedDatabase simulates a migration-process restart after an
// interrupted supported upgrade. It reuses the already validated, exact
// isolated DSN and validates the server-selected database name again before
// the remaining migrations can run.
func reopenDG5IsolatedDatabase(t *testing.T, ctx context.Context, target *dg5IsolatedDatabase) {
	t.Helper()
	if target == nil || target.db == nil || target.provider == nil {
		t.Fatal("DG5 isolated migration target is not configured for restart")
	}
	if err := target.db.Close(); err != nil {
		t.Fatalf("close DG5 isolated migration connection before restart: %v", err)
	}
	reopened, err := sql.Open(target.driver, target.dsn)
	if err != nil {
		t.Fatalf("reopen DG5 isolated %s migration connection: %v", target.dialect, err)
	}
	if err := reopened.PingContext(ctx); err != nil {
		_ = reopened.Close()
		t.Fatalf("ping restarted DG5 isolated %s database: %v", target.dialect, err)
	}
	if err := AssertConnectedDatabase(ctx, reopened, target.dialect); err != nil {
		_ = reopened.Close()
		t.Fatal(err)
	}
	target.db = reopened
	target.provider.db = reopened
	target.provider.sqlxDB = sqlx.NewDb(reopened, target.driver)
}

// assertDG5ForwardOnlyDownRejected proves that the current terminal migration
// cannot delete an authorization that may have existed before its idempotent
// seed. The database version must remain unchanged so the only safe recovery
// is an explicit, audited forward operation.
func assertDG5ForwardOnlyDownRejected(t *testing.T, ctx context.Context, db *sql.DB, migrationsDir string, latestVersion int64) {
	t.Helper()
	// The two CONFIG_ASSET migrations after CACHE_REFRESH_V3 have deliberately
	// non-destructive Down sections. Exercise those safe rollbacks first, then
	// prove the pre-existing cache-refresh permission boundary still rejects an
	// automatic downgrade. This keeps the assertion stable when an additive,
	// safely repeatable migration is appended to the history.
	for _, wantVersion := range []int64{20260810100000, 20260802100000} {
		if err := goose.DownContext(ctx, db, migrationsDir); err != nil {
			t.Fatalf("apply safe migration Down to %d: %v", wantVersion, err)
		}
		version, err := goose.GetDBVersionContext(ctx, db)
		if err != nil {
			t.Fatalf("read migration version after safe Down: %v", err)
		}
		if version != wantVersion {
			t.Fatalf("safe migration Down version=%d, want=%d", version, wantVersion)
		}
	}
	versionBefore := int64(20260802100000)
	downErr := goose.DownContext(ctx, db, migrationsDir)
	if downErr == nil || !strings.Contains(strings.ToLower(downErr.Error()), "forward-only") {
		t.Fatalf("DG5 migration Down error=%v, want forward-only rejection", downErr)
	}
	versionAfter, err := goose.GetDBVersionContext(ctx, db)
	if err != nil {
		t.Fatalf("read DG5 migration version after rejected Down: %v", err)
	}
	if versionAfter != versionBefore {
		t.Fatalf("rejected DG5 migration Down changed version from %d to %d", versionBefore, versionAfter)
	}
	if err := goose.UpContext(ctx, db, migrationsDir); err != nil {
		t.Fatalf("restore migrations after forward-only verification: %v", err)
	}
	versionAfter, err = goose.GetDBVersionContext(ctx, db)
	if err != nil {
		t.Fatalf("read migration version after forward-only restore: %v", err)
	}
	if versionAfter != latestVersion {
		t.Fatalf("migration version after forward-only restore=%d, want=%d", versionAfter, latestVersion)
	}
}

func openDG5IsolatedDatabase(t *testing.T, ctx context.Context) *dg5IsolatedDatabase {
	t.Helper()
	dialect := strings.ToLower(strings.TrimSpace(os.Getenv(dg5DialectEnv)))
	if dialect != "mysql" && dialect != "postgres" {
		t.Fatalf("set %s=mysql|postgres for the exact DG5 isolated database", dg5DialectEnv)
	}
	configDir, err := filepath.Abs(filepath.Join("..", "..", "..", "..", "..", "configs"))
	if err != nil {
		t.Fatalf("resolve local config directory: %v", err)
	}
	// Do not call the runtime loader here: its AutomaticEnv behavior is
	// correct for an application process but would allow a shell/CI DSN to
	// redirect this destructive acceptance harness. This test-only reader
	// merges the checked-in base and dev files without environment overlays.
	cfg := loadDG5CheckedInDevConfig(t, configDir)
	// Redis and RabbitMQ are non-database support services. Their credentials
	// may come from the normal local environment, but only a strict local Redis
	// endpoint and RabbitMQ credentials (never its URL/host/port) are allowed
	// back into this harness.
	dg5ApplyLocalSupportServiceConfig(t, configDir, &cfg)
	driver, dsn, host, port, user := dg5IsolatedDSN(t, cfg, dialect)
	db, err := sql.Open(driver, dsn)
	if err != nil {
		t.Fatalf("open DG5 isolated %s database: %v", dialect, err)
	}
	if err := db.PingContext(ctx); err != nil {
		_ = db.Close()
		t.Fatalf("ping DG5 isolated %s database: %v", dialect, err)
	}
	if err := AssertConnectedDatabase(ctx, db, dialect); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	// This is intentionally the only connection information emitted by the
	// acceptance harness: password, DSN, and source database stay private.
	t.Logf("DG5 local %s endpoint verified: host=%s port=%s database=%s user=%s", dialect, host, port, databaseNameForDialect(dialect), user)
	return &dg5IsolatedDatabase{
		dialect: dialect,
		driver:  driver,
		dsn:     dsn,
		db:      db,
		provider: &protectedBatchProvider{
			driver:  driver,
			dialect: dialect,
			db:      db,
			sqlxDB:  sqlx.NewDb(db, driver),
		},
		cfg: cfg,
	}
}

func loadDG5CheckedInDevConfig(t *testing.T, configDir string) sharedconfig.Config {
	t.Helper()
	loader := viper.New()
	loader.SetConfigType("yaml")
	path := dg5CheckedInConfigPath(t, configDir, "application.example")
	fileLoader := viper.New()
	fileLoader.SetConfigFile(path)
	if err := fileLoader.ReadInConfig(); err != nil {
		t.Fatalf("read checked-in DG5 public example config: %v", err)
	}
	if err := loader.MergeConfigMap(fileLoader.AllSettings()); err != nil {
		t.Fatalf("merge checked-in DG5 public example config: %v", err)
	}
	var cfg sharedconfig.Config
	decoderHook := viper.DecodeHook(mapstructure.ComposeDecodeHookFunc(
		mapstructure.StringToTimeDurationHookFunc(),
	))
	if err := loader.Unmarshal(&cfg, decoderHook); err != nil {
		t.Fatalf("unmarshal checked-in DG5 dev config: %v", err)
	}
	cfg.Profile = "dev"
	cfg.LoadedFiles = []string{path}
	return cfg
}

func dg5CheckedInConfigPath(t *testing.T, configDir, baseName string) string {
	t.Helper()
	for _, extension := range []string{".yaml", ".yml"} {
		path := filepath.Join(configDir, baseName+extension)
		if _, err := os.Stat(path); err == nil {
			return path
		} else if !os.IsNotExist(err) {
			t.Fatalf("stat checked-in DG5 config %s: %v", baseName, err)
		}
	}
	t.Fatalf("checked-in DG5 config %s(.yaml|.yml) not found", baseName)
	return ""
}

func dg5ApplyLocalSupportServiceConfig(t *testing.T, configDir string, cfg *sharedconfig.Config) {
	t.Helper()
	if cfg == nil {
		t.Fatal("DG5 support-service configuration target is required")
	}
	// SEVEN_PROFILE is the only profile selector accepted here. Datasource
	// values loaded by the normal application loader are deliberately ignored.
	t.Setenv("SEVEN_PROFILE", "dev")
	runtimeCfg, err := sharedconfig.Load(configDir)
	if err != nil {
		t.Fatalf("load local support-service configuration: %v", err)
	}
	if err := validateDG5LocalRedisConfig(runtimeCfg.Cache); err != nil {
		t.Fatal(err)
	}
	cfg.Cache = runtimeCfg.Cache
	cfg.RabbitMQ = sharedconfig.RabbitMQConfig{
		Username:     runtimeCfg.RabbitMQ.Username,
		Password:     runtimeCfg.RabbitMQ.Password,
		VHost:        runtimeCfg.RabbitMQ.VHost,
		Prefetch:     runtimeCfg.RabbitMQ.Prefetch,
		ReconnectMin: runtimeCfg.RabbitMQ.ReconnectMin,
		ReconnectMax: runtimeCfg.RabbitMQ.ReconnectMax,
	}
}

func validateDG5LocalRedisConfig(cacheCfg sharedconfig.CacheConfig) error {
	if !cacheCfg.Redis.Enabled || cacheCfg.Redis.Mode != sharedconfig.RedisCacheModeSingle {
		return fmt.Errorf("DG5 acceptance requires one configured local Redis instance")
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(cacheCfg.Redis.Single.Addr))
	if err != nil || !dg5LoopbackTCPHost(host) {
		return fmt.Errorf("refusing non-local Redis DG5 endpoint")
	}
	return nil
}

func dg5IsolatedDSN(t *testing.T, cfg sharedconfig.Config, dialect string) (driver, dsn, host, port, user string) {
	t.Helper()
	switch dialect {
	case "mysql":
		localDSN := strings.TrimSpace(os.Getenv(dg5MySQLLocalDSNEnv))
		if localDSN == "" {
			t.Fatalf("set %s to a loopback MySQL connection before DG5 acceptance", dg5MySQLLocalDSNEnv)
		}
		parsed, err := mysqldriver.ParseDSN(localDSN)
		if err != nil {
			t.Fatalf("parse local MySQL DSN: %v", err)
		}
		if err := validateDG5MySQLIsolatedConfig(parsed, MySQLDatabaseName, false); err != nil {
			t.Fatal(err)
		}
		parsed.DBName = MySQLDatabaseName
		rewritten := parsed.FormatDSN()
		verified, err := mysqldriver.ParseDSN(rewritten)
		if err != nil {
			t.Fatalf("parse rewritten MySQL DG5 DSN: %v", err)
		}
		if err := validateDG5MySQLIsolatedConfig(verified, MySQLDatabaseName, true); err != nil {
			t.Fatal(err)
		}
		return "mysql", rewritten, mysqlHost(verified), mysqlPort(verified), verified.User
	case "postgres":
		return dg5PostgresSocketDSN(t)
	default:
		t.Fatalf("unsupported DG5 dialect %q", dialect)
		return "", "", "", "", ""
	}
}

func validateDG5MySQLIsolatedConfig(parsed *mysqldriver.Config, database string, requireTarget bool) error {
	if parsed == nil {
		return fmt.Errorf("refusing empty MySQL DG5 connection configuration")
	}
	if strings.ToLower(strings.TrimSpace(parsed.Net)) != "tcp" || !dg5LoopbackTCPHost(mysqlHost(parsed)) {
		return fmt.Errorf("refusing non-local MySQL DG5 endpoint")
	}
	if requireTarget && parsed.DBName != database {
		return fmt.Errorf("refusing MySQL DG5 target database mismatch")
	}
	return nil
}

func dg5PostgresSocketDSN(t *testing.T) (driver, dsn, host, port, userName string) {
	t.Helper()
	if err := validateDG5PostgresAmbientEnvironment(); err != nil {
		t.Fatal(err)
	}
	currentUser, err := user.Current()
	if err != nil || strings.TrimSpace(currentUser.Username) == "" {
		t.Fatalf("resolve local PostgreSQL peer user: %v", err)
	}
	endpoint := &url.URL{
		Scheme: "postgresql",
		User:   url.User(currentUser.Username),
		Path:   "/" + PostgresDatabaseName,
	}
	query := endpoint.Query()
	query.Set("host", dg5PostgresSocketDir)
	query.Set("port", fmt.Sprintf("%d", dg5PostgresSocketPort))
	query.Set("sslmode", "disable")
	endpoint.RawQuery = query.Encode()
	dsn = endpoint.String()
	parsed, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parse local PostgreSQL peer socket configuration: %v", err)
	}
	if err := validateDG5PostgresIsolatedConfig(parsed, PostgresDatabaseName); err != nil {
		t.Fatal(err)
	}
	return "pgx", dsn, parsed.Host, fmt.Sprintf("%d", parsed.Port), parsed.User
}

func validateDG5PostgresAmbientEnvironment() error {
	for _, key := range []string{"PGSERVICE", "PGSERVICEFILE"} {
		if _, present := os.LookupEnv(key); present {
			return fmt.Errorf("refusing ambient PostgreSQL %s for DG5 acceptance", key)
		}
	}
	return nil
}

func validateDG5PostgresIsolatedConfig(parsed *pgx.ConnConfig, database string) error {
	if parsed == nil {
		return fmt.Errorf("refusing empty PostgreSQL DG5 connection configuration")
	}
	if parsed.Database != database {
		return fmt.Errorf("refusing PostgreSQL DG5 target database mismatch")
	}
	if parsed.Host != dg5PostgresSocketDir {
		return fmt.Errorf("refusing non-local PostgreSQL DG5 endpoint")
	}
	if parsed.Port != dg5PostgresSocketPort {
		return fmt.Errorf("refusing PostgreSQL DG5 socket port mismatch")
	}
	if len(parsed.Fallbacks) != 0 {
		return fmt.Errorf("refusing PostgreSQL DG5 fallback endpoints")
	}
	if strings.TrimSpace(parsed.User) == "" {
		return fmt.Errorf("refusing PostgreSQL DG5 connection without a local peer user")
	}
	return nil
}

func mysqlHost(cfg *mysqldriver.Config) string {
	if cfg == nil || strings.TrimSpace(cfg.Addr) == "" {
		return "127.0.0.1"
	}
	host, _, err := strings.Cut(cfg.Addr, ":")
	if err {
		return host
	}
	return cfg.Addr
}

func mysqlPort(cfg *mysqldriver.Config) string {
	if cfg == nil || strings.TrimSpace(cfg.Addr) == "" {
		return "3306"
	}
	_, port, found := strings.Cut(cfg.Addr, ":")
	if found && strings.TrimSpace(port) != "" {
		return port
	}
	return "3306"
}

func databaseNameForDialect(dialect string) string {
	if dialect == "postgres" {
		return PostgresDatabaseName
	}
	return MySQLDatabaseName
}

func dg5LocalHost(host string) bool {
	switch strings.ToLower(strings.TrimSpace(host)) {
	case "127.0.0.1", "localhost", "::1", "/tmp":
		// /tmp is the explicitly observed local PostgreSQL Unix-socket
		// directory for this governed acceptance. It is not a network endpoint
		// and this test-only guard does not accept arbitrary filesystem paths.
		return true
	default:
		return false
	}
}

// The DG5 harness owns all destructive-isolation and support-service guards.
// Keeping these tests beside the acceptance implementation prevents a generic
// datasource-infrastructure package from becoming an application-policy
// exception or importing DG5-only helpers across the DDD boundary.
func TestDG5LocalEndpointGuardAllowsOnlyLoopbackAndExplicitLocalSocket(t *testing.T) {
	tests := []struct {
		endpoint string
		want     bool
	}{
		{endpoint: "127.0.0.1", want: true},
		{endpoint: "localhost", want: true},
		{endpoint: "::1", want: true},
		{endpoint: "/tmp", want: true},
		{endpoint: "", want: false},
		{endpoint: "10.0.0.8", want: false},
		{endpoint: "db.internal", want: false},
		{endpoint: "/private/tmp", want: false},
	}
	for _, test := range tests {
		t.Run(test.endpoint, func(t *testing.T) {
			if got := dg5LocalHost(test.endpoint); got != test.want {
				t.Fatalf("dg5LocalHost(%q)=%t, want %t", test.endpoint, got, test.want)
			}
		})
	}
}

func TestDG5PostgresConnectionGuardRejectsFallbacks(t *testing.T) {
	parsed, err := pgx.ParseConfig("postgresql://local@/seven_database_governance_pg?host=%2Ftmp,evil.invalid&sslmode=disable")
	if err != nil {
		t.Fatalf("parse hostile PostgreSQL DSN: %v", err)
	}
	if parsed.Host != "/tmp" || len(parsed.Fallbacks) == 0 {
		t.Fatalf("test requires first local host plus a parsed fallback: host=%q fallbacks=%d", parsed.Host, len(parsed.Fallbacks))
	}
	if err := validateDG5PostgresIsolatedConfig(parsed, PostgresDatabaseName); err == nil {
		t.Fatal("PostgreSQL isolated-database guard accepted a fallback endpoint")
	}
}

func TestDG5PostgresSocketConfigurationPinsAmbientPort(t *testing.T) {
	t.Setenv("PGHOST", "evil.invalid")
	t.Setenv("PGUSER", "untrusted")
	t.Setenv("PGPORT", "6543")
	parsed, err := pgx.ParseConfig("postgresql://peer-user@/seven_database_governance_pg?host=%2Ftmp&port=5432&sslmode=disable")
	if err != nil {
		t.Fatalf("parse pinned PostgreSQL socket configuration: %v", err)
	}
	if err := validateDG5PostgresIsolatedConfig(parsed, PostgresDatabaseName); err != nil {
		t.Fatalf("pinned PostgreSQL socket configuration rejected: %v", err)
	}
	if parsed.Port != dg5PostgresSocketPort || parsed.Host != dg5PostgresSocketDir || parsed.User != "peer-user" {
		t.Fatalf("ambient PostgreSQL environment influenced pinned socket config: host=%q port=%d user=%q", parsed.Host, parsed.Port, parsed.User)
	}
}

func TestDG5LocalRedisGuardRejectsRemoteEndpoint(t *testing.T) {
	if err := validateDG5LocalRedisConfig(sharedconfig.CacheConfig{
		Redis: sharedconfig.RedisCacheConfig{
			Enabled: true,
			Mode:    sharedconfig.RedisCacheModeSingle,
			Single:  sharedconfig.RedisSingleConfig{Addr: "redis.internal:6379"},
		},
	}); err == nil {
		t.Fatal("DG5 Redis guard accepted a remote endpoint")
	}
}

func TestDG5MySQLGuardRejectsNonLoopbackEndpoint(t *testing.T) {
	if err := validateDG5MySQLIsolatedConfig(&mysqldriver.Config{Net: "tcp", Addr: "mysql.internal:3306"}, MySQLDatabaseName, false); err == nil {
		t.Fatal("DG5 MySQL guard accepted a non-loopback endpoint")
	}
}

func TestDG5PostgresGuardRejectsAmbientServiceProfile(t *testing.T) {
	t.Setenv("PGSERVICE", "untrusted-service")
	if err := validateDG5PostgresAmbientEnvironment(); err == nil {
		t.Fatal("DG5 PostgreSQL guard accepted an ambient service profile")
	}
}

func TestDG5CheckedInConfigIgnoresDatasourceEnvironment(t *testing.T) {
	configDir, err := filepath.Abs(filepath.Join("..", "..", "..", "..", "..", "configs"))
	if err != nil {
		t.Fatalf("resolve config directory: %v", err)
	}
	t.Setenv("DATASOURCE_MYSQL_DSN", "mysql://untrusted.invalid/other")
	t.Setenv("DATASOURCE_POSTGRES_DSN", "postgresql://untrusted.invalid/other")
	cfg := loadDG5CheckedInDevConfig(t, configDir)
	if cfg.Datasource.MySQL.DSN == "mysql://untrusted.invalid/other" || cfg.Datasource.Postgres.DSN == "postgresql://untrusted.invalid/other" {
		t.Fatal("DG5 isolated acceptance inherited a datasource environment override")
	}
}

func dg5LoopbackTCPHost(host string) bool {
	switch strings.ToLower(strings.TrimSpace(host)) {
	case "127.0.0.1", "::1":
		return true
	default:
		return false
	}
}

func resetDG5IsolatedSchema(t *testing.T, ctx context.Context, db *sql.DB, dialect string) {
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
			t.Fatalf("list isolated MySQL tables before reset: %v", err)
		}
		tableNames := make([]string, 0)
		for rows.Next() {
			var name string
			if err := rows.Scan(&name); err != nil {
				_ = rows.Close()
				t.Fatalf("scan isolated MySQL table name: %v", err)
			}
			tableNames = append(tableNames, name)
		}
		if err := rows.Close(); err != nil {
			t.Fatalf("close isolated MySQL table rows: %v", err)
		}
		for _, name := range tableNames {
			quoted := "`" + strings.ReplaceAll(name, "`", "``") + "`"
			if _, err := db.ExecContext(ctx, "DROP TABLE "+quoted); err != nil {
				t.Fatalf("drop isolated MySQL table: %v", err)
			}
		}
	case "postgres":
		if _, err := db.ExecContext(ctx, `DROP SCHEMA public CASCADE`); err != nil {
			t.Fatalf("drop isolated PostgreSQL public schema: %v", err)
		}
		if _, err := db.ExecContext(ctx, `CREATE SCHEMA public`); err != nil {
			t.Fatalf("recreate isolated PostgreSQL public schema: %v", err)
		}
	default:
		t.Fatalf("unsupported DG5 dialect %q", dialect)
	}
}
