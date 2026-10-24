package infrastructure

import (
	"context"
	"regexp"
	"strings"
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/app/platform/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/store"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
)

func TestPlatformPolicyReplaceRequiresActiveTransaction(t *testing.T) {
	tests := []struct {
		name string
		call func(*Repository) error
	}{
		{
			name: "login methods",
			call: func(repo *Repository) error {
				return repo.ReplaceLoginMethods(context.Background(), "seven-admin", nil, 7)
			},
		},
		{
			name: "source rules",
			call: func(repo *Repository) error {
				return repo.ReplaceSourceRules(context.Background(), "seven-admin", nil, 7)
			},
		},
		{
			name: "default roles",
			call: func(repo *Repository) error {
				return repo.ReplaceDefaultRoles(context.Background(), "seven-admin", nil, 7)
			},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			db, _, err := sqlmock.New()
			if err != nil {
				t.Fatalf("sqlmock: %v", err)
			}
			defer db.Close()
			repo := &Repository{db: sqlx.NewDb(db, "sqlmock")}
			err = test.call(repo)
			if err == nil || !strings.Contains(err.Error(), "active transaction") {
				t.Fatalf("replace without transaction err=%v, want active transaction rejection", err)
			}
		})
	}
}

func TestReplaceLoginMethodsUsesFixedSizeMultiValueInsertChunks(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	sqlxDB := sqlx.NewDb(db, "sqlmock")
	repo := &Repository{db: sqlxDB}
	methods := make([]domain.LoginMethod, 51)
	for index := range methods {
		methods[index] = domain.LoginMethod{
			MethodType:     domain.MethodExternalOAuth,
			ProviderCode:   "provider",
			DisplayName:    "Provider",
			DisplayEnabled: true,
			LoginEnabled:   true,
		}
	}

	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM sys_platform_login_method WHERE platformCode = \? AND isDeleted = 1`).
		WithArgs("seven-admin").
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec(`UPDATE sys_platform_login_method SET isDeleted = 1, updaterId = \?, updateTime = NOW\(\) WHERE platformCode = \? AND isDeleted = 0`).
		WithArgs(int64(7), "seven-admin").
		WillReturnResult(sqlmock.NewResult(0, 2))
	loginTuple := `(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, NOW(), NOW(), 0)`
	mock.ExpectExec(`(?s)INSERT INTO sys_platform_login_method.*VALUES\s+` + regexp.QuoteMeta(strings.TrimSuffix(strings.Repeat(loginTuple+", ", 50), ", "))).
		WillReturnResult(sqlmock.NewResult(1, 50))
	mock.ExpectExec(`(?s)INSERT INTO sys_platform_login_method.*VALUES\s+` + regexp.QuoteMeta(loginTuple)).
		WillReturnResult(sqlmock.NewResult(51, 1))
	mock.ExpectCommit()

	transactor := store.NewSQLXTransactor(sqlxDB)
	err = transactor.WithinTransaction(context.Background(), func(txCtx context.Context) error {
		return repo.ReplaceLoginMethods(txCtx, "seven-admin", methods, 7)
	})
	if err != nil {
		t.Fatalf("ReplaceLoginMethods() error=%v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestReplaceSourceRulesAndDefaultRolesUseMultiValueInsert(t *testing.T) {
	t.Run("source rules", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("sqlmock: %v", err)
		}
		defer db.Close()
		sqlxDB := sqlx.NewDb(db, "sqlmock")
		repo := &Repository{db: sqlxDB}
		mock.ExpectBegin()
		mock.ExpectExec(`DELETE FROM sys_platform_source_rule WHERE platformCode = \? AND isDeleted = 1`).
			WithArgs("seven-admin").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(`UPDATE sys_platform_source_rule SET isDeleted = 1, updaterId = \?, updateTime = NOW\(\) WHERE platformCode = \? AND isDeleted = 0`).
			WithArgs(int64(7), "seven-admin").
			WillReturnResult(sqlmock.NewResult(0, 2))
		tuple := `(?, ?, ?, ?, ?, ?, ?, ?, NOW(), NOW(), 0)`
		mock.ExpectExec(`(?s)INSERT INTO sys_platform_source_rule.*VALUES\s+` + regexp.QuoteMeta(tuple+", "+tuple)).
			WillReturnResult(sqlmock.NewResult(1, 2))
		mock.ExpectCommit()
		transactor := store.NewSQLXTransactor(sqlxDB)
		err = transactor.WithinTransaction(context.Background(), func(txCtx context.Context) error {
			return repo.ReplaceSourceRules(txCtx, "seven-admin", []domain.SourceRule{
				{MatchType: domain.MatchHost, MatchValue: "one.example.test", Status: domain.StatusActive},
				{MatchType: domain.MatchHost, MatchValue: "two.example.test", Status: domain.StatusActive},
			}, 7)
		})
		if err != nil {
			t.Fatalf("ReplaceSourceRules() error=%v", err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("sql expectations: %v", err)
		}
	})

	t.Run("default roles", func(t *testing.T) {
		db, mock, err := sqlmock.New()
		if err != nil {
			t.Fatalf("sqlmock: %v", err)
		}
		defer db.Close()
		sqlxDB := sqlx.NewDb(db, "sqlmock")
		repo := &Repository{db: sqlxDB}
		mock.ExpectBegin()
		mock.ExpectExec(`DELETE FROM sys_platform_default_role WHERE platformCode = \? AND isDeleted = 1`).
			WithArgs("seven-admin").
			WillReturnResult(sqlmock.NewResult(0, 0))
		mock.ExpectExec(`UPDATE sys_platform_default_role SET isDeleted = 1, updaterId = \?, updateTime = NOW\(\) WHERE platformCode = \? AND isDeleted = 0`).
			WithArgs(int64(7), "seven-admin").
			WillReturnResult(sqlmock.NewResult(0, 2))
		tuple := `(?, ?, ?, ?, ?, ?, NOW(), NOW(), 0)`
		mock.ExpectExec(`(?s)INSERT INTO sys_platform_default_role.*VALUES\s+` + regexp.QuoteMeta(tuple+", "+tuple)).
			WillReturnResult(sqlmock.NewResult(1, 2))
		mock.ExpectCommit()
		transactor := store.NewSQLXTransactor(sqlxDB)
		err = transactor.WithinTransaction(context.Background(), func(txCtx context.Context) error {
			return repo.ReplaceDefaultRoles(txCtx, "seven-admin", []domain.DefaultRole{
				{RoleID: 101, AutoAssignEnabled: true, Status: domain.StatusActive},
				{RoleID: 102, AutoAssignEnabled: true, Status: domain.StatusActive},
			}, 7)
		})
		if err != nil {
			t.Fatalf("ReplaceDefaultRoles() error=%v", err)
		}
		if err := mock.ExpectationsWereMet(); err != nil {
			t.Fatalf("sql expectations: %v", err)
		}
	})
}
