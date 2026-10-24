package infrastructure

import (
	"context"
	"database/sql"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/app/system/user/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/governance"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/store"
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
)

type dg2UserIntegrationProvider struct {
	driver  string
	dialect string
	db      *sql.DB
	sqlxDB  *sqlx.DB
}

func (p *dg2UserIntegrationProvider) Driver() string               { return p.driver }
func (p *dg2UserIntegrationProvider) Dialect() string              { return p.dialect }
func (p *dg2UserIntegrationProvider) DB() *sql.DB                  { return p.db }
func (p *dg2UserIntegrationProvider) SQLX() *sqlx.DB               { return p.sqlxDB }
func (p *dg2UserIntegrationProvider) Transactor() store.Transactor { return nil }
func (p *dg2UserIntegrationProvider) Configured() bool             { return true }
func (p *dg2UserIntegrationProvider) Close() error                 { return p.db.Close() }

func TestUserPostRoleRepositoryDatabaseDialectAcceptance(t *testing.T) {
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
	provider := &dg2UserIntegrationProvider{
		driver: driver, dialect: dialect, db: db, sqlxDB: sqlx.NewDb(db, driver),
	}
	t.Cleanup(func() { _ = provider.Close() })
	repo, err := NewRepository(provider)
	if err != nil {
		t.Fatalf("new user repository: %v", err)
	}

	const (
		postID  int64 = 9202607302001
		userID1 int64 = 9202607302002
		userID2 int64 = 9202607302003
		roleID  int64 = 9202607302004
	)
	rollback := errors.New("dg2 user rollback")
	transactor := store.NewSQLXTransactor(provider.sqlxDB)
	err = transactor.WithinTransaction(context.Background(), func(txCtx context.Context) error {
		exec := store.SQLXExecutor(txCtx, provider.sqlxDB)
		insertUsers := `INSERT INTO sys_user (id, userAccount, nickName, userEmail, status, isDeleted) VALUES (?, ?, ?, ?, 0, 0), (?, ?, ?, ?, 0, 0)`
		if dialect == "postgres" {
			insertUsers = `INSERT INTO sys_user (id, "userAccount", "nickName", "userEmail", status, "isDeleted") VALUES ($1, $2, $3, $4, 0, FALSE), ($5, $6, $7, $8, 0, FALSE)`
		}
		if _, insertErr := exec.ExecContext(txCtx, insertUsers,
			userID1, "dg2selector1", "DG2 Selector One", "dg2selector1@example.invalid",
			userID2, "dg2selector2", "DG2 Selector Two", "dg2selector2@example.invalid",
		); insertErr != nil {
			return insertErr
		}
		insert := `INSERT INTO sys_user_position (userId, postId, isPrimary, isDeleted) VALUES (?, ?, 0, 0), (?, ?, 0, 0)`
		if dialect == "postgres" {
			insert = `INSERT INTO sys_user_position ("userId", "postId", "isPrimary", "isDeleted") VALUES ($1, $2, FALSE, FALSE), ($3, $4, FALSE, FALSE)`
		}
		if _, insertErr := exec.ExecContext(txCtx, insert, userID1, postID, userID2, postID); insertErr != nil {
			return insertErr
		}
		first, pageErr := repo.ListUserIDsByPostIDPage(txCtx, postID, 0, 1)
		if pageErr != nil || len(first) != 1 || first[0] != userID1 {
			t.Fatalf("first post user page=%v err=%v", first, pageErr)
		}
		second, pageErr := repo.ListUserIDsByPostIDPage(txCtx, postID, first[0], 1)
		if pageErr != nil || len(second) != 1 || second[0] != userID2 {
			t.Fatalf("second post user page=%v err=%v", second, pageErr)
		}
		if replaceErr := repo.ReplacePostRoles(txCtx, postID, []int64{roleID}, 0); replaceErr != nil {
			return replaceErr
		}
		roleIDs, listErr := repo.ListPostRoleIDs(txCtx, postID)
		if listErr != nil || len(roleIDs) != 1 || roleIDs[0] != roleID {
			t.Fatalf("post role round trip=%v err=%v", roleIDs, listErr)
		}
		options, selectorErr := repo.ListUserOptions(txCtx, domain.UserSelectorQuery{Keyword: "dg2selector", Limit: 10})
		if selectorErr != nil || len(options) != 2 {
			t.Fatalf("selector options=%v err=%v", options, selectorErr)
		}
		visible, selectorErr := repo.FindVisibleUserOptionByID(txCtx, userID1, domain.DataScopeFilter{})
		if selectorErr != nil || visible == nil || visible.AccountName != "dg2selector1" {
			t.Fatalf("selector detail=%#v err=%v", visible, selectorErr)
		}
		return rollback
	})
	if !errors.Is(err, rollback) {
		t.Fatalf("user repository acceptance must reach deliberate rollback, got %v", err)
	}
}
