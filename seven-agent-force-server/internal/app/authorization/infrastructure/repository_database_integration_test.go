package infrastructure

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	dbgovernance "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/governance"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/store"
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
)

type dg2AuthorizationIntegrationProvider struct {
	driver  string
	dialect string
	db      *sql.DB
	sqlxDB  *sqlx.DB
}

func (p *dg2AuthorizationIntegrationProvider) Driver() string               { return p.driver }
func (p *dg2AuthorizationIntegrationProvider) Dialect() string              { return p.dialect }
func (p *dg2AuthorizationIntegrationProvider) DB() *sql.DB                  { return p.db }
func (p *dg2AuthorizationIntegrationProvider) SQLX() *sqlx.DB               { return p.sqlxDB }
func (p *dg2AuthorizationIntegrationProvider) Transactor() store.Transactor { return nil }
func (p *dg2AuthorizationIntegrationProvider) Configured() bool             { return true }
func (p *dg2AuthorizationIntegrationProvider) Close() error                 { return p.db.Close() }

func TestAuthorizationSplitReadsDatabaseDialectAcceptance(t *testing.T) {
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
	if err := dbgovernance.AssertConnectedDatabase(context.Background(), db, dialect); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	provider := &dg2AuthorizationIntegrationProvider{
		driver: driver, dialect: dialect, db: db, sqlxDB: sqlx.NewDb(db, driver),
	}
	t.Cleanup(func() { _ = provider.Close() })
	repo, err := NewRepository(provider, nil)
	if err != nil {
		t.Fatalf("new authorization repository: %v", err)
	}

	ctx := context.Background()
	baseID := time.Now().UTC().UnixNano()
	userID, roleID, permissionID := baseID, baseID+1, baseID+2
	relationID := baseID + 3
	rollback := errors.New("dg2 rollback")
	transactor := store.NewSQLXTransactor(provider.sqlxDB)
	err = transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		fixtures := []struct {
			query string
			args  []any
		}{
			{
				query: `INSERT INTO sys_user (id, userAccount, nickName, status, userGender, createTime, updateTime, isDeleted)
VALUES (?, ?, ?, ?, ?, NOW(), NOW(), ?)`,
				args: []any{userID, fmt.Sprintf("dg2_%d", baseID), "DG2 Fixture", 0, false, false},
			},
			{
				query: `INSERT INTO sys_role (id, name, code, dataScope, status, type, createTime, updateTime, isDeleted)
VALUES (?, ?, ?, ?, ?, ?, NOW(), NOW(), ?)`,
				args: []any{roleID, "DG2 Role", fmt.Sprintf("DG2_ROLE_%d", baseID), 5, 0, 0, false},
			},
			{
				query: `INSERT INTO sys_permission (id, code, name, resourceType, status, createTime, updateTime, isDeleted)
VALUES (?, ?, ?, ?, ?, NOW(), NOW(), ?)`,
				args: []any{permissionID, fmt.Sprintf("dg2:permission:%d", baseID), "DG2 Permission", "API", 0, false},
			},
			{
				query: `INSERT INTO sys_user_role (id, userId, roleId, createTime, updateTime, isDeleted)
VALUES (?, ?, ?, NOW(), NOW(), ?)`,
				args: []any{relationID, userID, roleID, false},
			},
			{
				query: `INSERT INTO sys_role_permission (id, roleId, permissionId, source, createTime, updateTime)
VALUES (?, ?, ?, ?, NOW(), NOW())`,
				args: []any{relationID + 1, roleID, permissionID, rolePermissionSourceDirect},
			},
		}
		for _, fixture := range fixtures {
			if err := repo.exec(txCtx, fixture.query, fixture.args...); err != nil {
				return err
			}
		}
		user, err := repo.FindAccessUser(txCtx, userID)
		if err != nil || user == nil {
			return fmt.Errorf("find access user: user=%#v err=%w", user, err)
		}
		sources, err := repo.ListAccessRoleSources(txCtx, userID)
		if err != nil || len(sources) != 1 || sources[0].RoleID != roleID {
			return fmt.Errorf("list split role sources: sources=%#v err=%w", sources, err)
		}
		grants, err := repo.ListAccessGrantRecords(txCtx, userID)
		if err != nil || len(grants) != 1 || grants[0].PermissionID != permissionID {
			return fmt.Errorf("list split grant records: grants=%#v err=%w", grants, err)
		}
		if _, err := repo.ListAccessMemberships(txCtx, userID); err != nil {
			return fmt.Errorf("list split memberships: %w", err)
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("authorization acceptance must reach deliberate rollback, got %v", err)
	}
}
