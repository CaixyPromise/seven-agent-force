package bootstrap

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/store"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
)

type fakeRunner struct {
	upCalls   int
	upToCalls int
	upVersion int64
	upToArg   int64
	upToRet   int64
	upRet     int64
}

func (r *fakeRunner) Up(ctx context.Context, db *sql.DB, dialect, dir, table string) (int64, error) {
	r.upCalls++
	return r.upRet, nil
}

func (r *fakeRunner) UpTo(ctx context.Context, db *sql.DB, dialect, dir, table string, version int64) (int64, error) {
	r.upToCalls++
	r.upToArg = version
	return r.upToRet, nil
}

func (r *fakeRunner) Version(ctx context.Context, db *sql.DB, dialect, table string) (int64, error) {
	return r.upRet, nil
}

type fakeInspector struct {
	inspection Inspection
	err        error
}

func (i *fakeInspector) Inspect(ctx context.Context, db *sql.DB, versionTable string) (Inspection, error) {
	return i.inspection, i.err
}

type fakeProvider struct {
	driver     string
	dialect    string
	db         *sql.DB
	configured bool
}

func (p *fakeProvider) Driver() string               { return p.driver }
func (p *fakeProvider) Dialect() string              { return p.dialect }
func (p *fakeProvider) DB() *sql.DB                  { return p.db }
func (p *fakeProvider) SQLX() *sqlx.DB               { return nil }
func (p *fakeProvider) Transactor() store.Transactor { return nil }
func (p *fakeProvider) Configured() bool             { return p.configured }
func (p *fakeProvider) Close() error                 { return nil }

func TestBootstrapEmptySchemaRunsBaselineThenUpdate(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	runner := &fakeRunner{upToRet: 20260422000000, upRet: 20260423000000}
	service := NewServiceWithRunner(zap.NewNop(), runner)
	service.inspector = &fakeInspector{inspection: Inspection{State: SchemaStateEmpty, VersionTable: "goose_db_version"}}

	result, err := service.Bootstrap(context.Background(), &fakeProvider{
		driver:     "mysql",
		dialect:    "mysql",
		db:         db,
		configured: true,
	}, config.DatasourceBootstrapConfig{
		Enabled:         true,
		Mode:            config.BootstrapModeBoth,
		MigrationsDir:   "migrations/mysql",
		VersionTable:    "goose_db_version",
		BaselineVersion: "20260422000000",
	})
	if err != nil {
		t.Fatalf("bootstrap empty schema: %v", err)
	}
	if !result.BaselineApplied || !result.UpdateApplied {
		t.Fatalf("unexpected result: %+v", result)
	}
	if runner.upToCalls != 1 || runner.upCalls != 1 {
		t.Fatalf("unexpected runner calls: %+v", runner)
	}
	if runner.upToArg != 20260422000000 {
		t.Fatalf("unexpected baseline version: %d", runner.upToArg)
	}
}

func TestBootstrapManagedSchemaRunsUpdateOnly(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	runner := &fakeRunner{upRet: 20260423000000}
	service := NewServiceWithRunner(zap.NewNop(), runner)
	service.inspector = &fakeInspector{inspection: Inspection{State: SchemaStateManaged, VersionTable: "goose_db_version", VersionTableExists: true}}

	result, err := service.Bootstrap(context.Background(), &fakeProvider{
		driver:     "mysql",
		dialect:    "mysql",
		db:         db,
		configured: true,
	}, config.DatasourceBootstrapConfig{
		Enabled:       true,
		Mode:          config.BootstrapModeBoth,
		MigrationsDir: "migrations/mysql",
		VersionTable:  "goose_db_version",
	})
	if err != nil {
		t.Fatalf("bootstrap managed schema: %v", err)
	}
	if result.BaselineApplied || result.SyncApplied || !result.UpdateApplied {
		t.Fatalf("unexpected result: %+v", result)
	}
	if runner.upToCalls != 0 || runner.upCalls != 1 {
		t.Fatalf("unexpected runner calls: %+v", runner)
	}
}

func TestBootstrapLegacySchemaRequiresExplicitSync(t *testing.T) {
	db, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	service := NewServiceWithRunner(zap.NewNop(), &fakeRunner{})
	service.inspector = &fakeInspector{inspection: Inspection{State: SchemaStateLegacyUnmanaged, VersionTable: "goose_db_version", BusinessTableCount: 3}}

	_, err = service.Bootstrap(context.Background(), &fakeProvider{
		driver:     "mysql",
		dialect:    "mysql",
		db:         db,
		configured: true,
	}, config.DatasourceBootstrapConfig{
		Enabled:         true,
		Mode:            config.BootstrapModeBoth,
		MigrationsDir:   "migrations/mysql",
		VersionTable:    "goose_db_version",
		BaselineVersion: "20260422000000",
	})
	if err == nil {
		t.Fatal("expected legacy unmanaged schema to fail without allowLegacySync")
	}
}

func TestBootstrapLegacySchemaSyncsThenUpdates(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })

	dir := t.TempDir()
	writeMigration(t, filepath.Join(dir, "20260422000000_baseline.sql"))
	writeMigration(t, filepath.Join(dir, "20260423000000_next.sql"))

	mock.ExpectBegin()
	mock.ExpectExec("CREATE TABLE goose_db_version").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("INSERT INTO goose_db_version \\(version_id, is_applied\\) VALUES \\(\\?, \\?\\)").
		WithArgs(int64(0), true).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT INTO goose_db_version \\(version_id, is_applied\\) VALUES \\(\\?, \\?\\)").
		WithArgs(int64(20260422000000), true).
		WillReturnResult(sqlmock.NewResult(2, 1))
	mock.ExpectCommit()

	runner := &fakeRunner{upRet: 20260423000000}
	service := NewServiceWithRunner(zap.NewNop(), runner)
	service.inspector = &fakeInspector{inspection: Inspection{
		State:              SchemaStateLegacyUnmanaged,
		VersionTable:       "goose_db_version",
		VersionTableExists: false,
		BusinessTableCount: 2,
	}}

	result, err := service.Bootstrap(context.Background(), &fakeProvider{
		driver:     "mysql",
		dialect:    "mysql",
		db:         db,
		configured: true,
	}, config.DatasourceBootstrapConfig{
		Enabled:         true,
		Mode:            config.BootstrapModeBoth,
		MigrationsDir:   dir,
		VersionTable:    "goose_db_version",
		BaselineVersion: "20260422000000",
		AllowLegacySync: true,
	})
	if err != nil {
		t.Fatalf("bootstrap legacy schema: %v", err)
	}
	if !result.SyncApplied || !result.UpdateApplied {
		t.Fatalf("unexpected result: %+v", result)
	}
	if runner.upCalls != 1 {
		t.Fatalf("expected one update call, got %+v", runner)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestNormalizedBootstrapConfigRejectsUnsafeVersionTableIdentifiers(t *testing.T) {
	for _, value := range []string{
		"goose_db_version; DROP TABLE sys_user",
		"`goose_db_version`",
		"public.goose_db_version",
		"goose_db_version_copy",
	} {
		if _, err := normalizedBootstrapConfig("mysql", config.DatasourceBootstrapConfig{VersionTable: value}); err == nil {
			t.Fatalf("unsafe version table %q unexpectedly accepted", value)
		}
	}
	cfg, err := normalizedBootstrapConfig("mysql", config.DatasourceBootstrapConfig{})
	if err != nil {
		t.Fatalf("default version table rejected: %v", err)
	}
	if cfg.VersionTable != "goose_db_version" {
		t.Fatalf("default version table=%q", cfg.VersionTable)
	}
}

func writeMigration(t *testing.T, path string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("-- +goose Up\nSELECT 1;\n"), 0o600); err != nil {
		t.Fatalf("write migration: %v", err)
	}
}
