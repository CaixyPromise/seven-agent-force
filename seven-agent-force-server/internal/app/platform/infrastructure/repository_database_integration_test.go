package infrastructure

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/governance"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/store"
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
)

type dg2PlatformIntegrationProvider struct {
	driver  string
	dialect string
	db      *sql.DB
	sqlxDB  *sqlx.DB
}

func (p *dg2PlatformIntegrationProvider) Driver() string               { return p.driver }
func (p *dg2PlatformIntegrationProvider) Dialect() string              { return p.dialect }
func (p *dg2PlatformIntegrationProvider) DB() *sql.DB                  { return p.db }
func (p *dg2PlatformIntegrationProvider) SQLX() *sqlx.DB               { return p.sqlxDB }
func (p *dg2PlatformIntegrationProvider) Transactor() store.Transactor { return nil }
func (p *dg2PlatformIntegrationProvider) Configured() bool             { return true }
func (p *dg2PlatformIntegrationProvider) Close() error                 { return p.db.Close() }

func TestPlatformRepositoryDatabaseDialectAcceptance(t *testing.T) {
	dialect := strings.TrimSpace(os.Getenv("DG2_TEST_DIALECT"))
	dsn := strings.TrimSpace(os.Getenv("DG2_TEST_DSN"))
	if dialect == "" || dsn == "" {
		t.Skip("set DG2_TEST_DIALECT and DG2_TEST_DSN for the exact isolated governance database")
	}
	driver := "mysql"
	if dialect == "postgres" {
		driver = "pgx"
	}
	db, err := sql.Open(driver, dsn)
	if err != nil {
		t.Fatalf("open %s: %v", dialect, err)
	}
	if err := governance.AssertConnectedDatabase(context.Background(), db, dialect); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	provider := &dg2PlatformIntegrationProvider{
		driver: driver, dialect: dialect, db: db, sqlxDB: sqlx.NewDb(db, driver),
	}
	t.Cleanup(func() { _ = provider.Close() })
	repo, err := NewRepository(provider)
	if err != nil {
		t.Fatalf("new platform repository: %v", err)
	}

	ctx := context.Background()
	platform, err := repo.FindManagedDefaultPlatform(ctx)
	if err != nil {
		t.Fatalf("find managed platform: %v", err)
	}
	if platform == nil || strings.TrimSpace(platform.PlatformCode) == "" {
		t.Fatal("exact migrated database has no managed default platform fixture")
	}
	code := platform.PlatformCode
	methods, err := repo.ListManagedLoginMethods(ctx, code)
	if err != nil {
		t.Fatalf("list managed login methods: %v", err)
	}
	rules, err := repo.ListManagedSourceRules(ctx, code)
	if err != nil {
		t.Fatalf("list managed source rules: %v", err)
	}
	roles, err := repo.ListDefaultRoleRecords(ctx, code)
	if err != nil {
		t.Fatalf("list managed default roles: %v", err)
	}
	if _, err := repo.ListDefaultRoles(ctx, code, 3); err != nil {
		t.Fatalf("list safe default roles: %v", err)
	}
	roleIDs := make([]int64, 0, len(roles))
	for _, role := range roles {
		roleIDs = append(roleIDs, role.RoleID)
	}
	if _, err := repo.ValidateDefaultRoles(ctx, roleIDs); err != nil {
		t.Fatalf("validate default role set: %v", err)
	}
	providerCodes := []string{"dg2-provider-not-present"}
	for _, method := range methods {
		if strings.TrimSpace(method.ProviderCode) != "" {
			providerCodes = append(providerCodes, method.ProviderCode)
		}
	}
	if _, err := repo.ListManagedExternalProviderCodes(ctx, providerCodes); err != nil {
		t.Fatalf("list managed external provider set: %v", err)
	}

	rollback := errors.New("dg2 rollback")
	transactor := store.NewSQLXTransactor(provider.sqlxDB)
	err = transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := repo.ReplaceLoginMethods(txCtx, code, methods, 0); err != nil {
			return err
		}
		if err := repo.ReplaceSourceRules(txCtx, code, rules, 0); err != nil {
			return err
		}
		if err := repo.ReplaceDefaultRoles(txCtx, code, roles, 0); err != nil {
			return err
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("replace acceptance must reach deliberate rollback, got %v", err)
	}
}
