package infrastructure

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/sso/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/store"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
)

func TestReplaceClientRedirectURIsRequiresActiveTransaction(t *testing.T) {
	db, _, repo := newSSORepositoryMock(t)
	defer db.Close()
	err := repo.ReplaceClientRedirectURIs(context.Background(), "demo-client", nil, 7, time.Now().UTC())
	if err == nil || !strings.Contains(err.Error(), "active transaction") {
		t.Fatalf("replace without transaction err=%v, want active transaction rejection", err)
	}
}

func TestReplaceClientRedirectURIsRejectsBlankRedirectBeforeSQL(t *testing.T) {
	db, mock, repo := newSSORepositoryMock(t)
	defer db.Close()
	sqlxDB, ok := repo.db.(*sqlx.DB)
	if !ok {
		t.Fatal("repository does not expose sqlx DB")
	}
	mock.ExpectBegin()
	mock.ExpectRollback()
	err := store.NewSQLXTransactor(sqlxDB).WithinTransaction(context.Background(), func(txCtx context.Context) error {
		return repo.ReplaceClientRedirectURIs(txCtx, "demo-client", []domain.ClientRedirectURI{{
			PostLogoutRedirectURI: "https://client.example/logout",
			Status:                domain.ClientStatusActive,
		}}, 7, time.Now().UTC())
	})
	if err == nil || !strings.Contains(err.Error(), "redirect uri is empty") {
		t.Fatalf("blank redirect err=%v", err)
	}
	assertSQLExpectations(t, mock)
}

func TestReplaceClientRedirectURIsUsesFixedSizeMultiValueInsertChunks(t *testing.T) {
	db, mock, repo := newSSORepositoryMock(t)
	defer db.Close()
	sqlxDB, ok := repo.db.(*sqlx.DB)
	if !ok {
		t.Fatal("repository does not expose sqlx DB")
	}
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)
	redirects := make([]domain.ClientRedirectURI, 51)
	for index := range redirects {
		redirects[index] = domain.ClientRedirectURI{
			RedirectURI:           fmt.Sprintf("https://client-%03d.example/callback", index),
			PostLogoutRedirectURI: fmt.Sprintf("https://client-%03d.example/logout", index),
			Status:                domain.ClientStatusActive,
		}
	}

	mock.ExpectBegin()
	mock.ExpectExec(`DELETE FROM sys_sso_client_redirect_uri\s+WHERE clientId = \? AND isDeleted = 1`).
		WithArgs("demo-client").
		WillReturnResult(sqlmock.NewResult(0, 2))
	mock.ExpectExec(`UPDATE sys_sso_client_redirect_uri\s+SET isDeleted = 1, updaterId = \?, updateTime = \?\s+WHERE clientId = \? AND isDeleted = 0`).
		WithArgs(int64(7), now, "demo-client").
		WillReturnResult(sqlmock.NewResult(0, 5))
	tuple := `(?, ?, ?, ?, ?, ?, ?, ?, 0)`
	mock.ExpectExec(`(?s)INSERT INTO sys_sso_client_redirect_uri.*VALUES\s+` + regexp.QuoteMeta(strings.TrimSuffix(strings.Repeat(tuple+", ", 50), ", "))).
		WillReturnResult(sqlmock.NewResult(1, 50))
	mock.ExpectExec(`(?s)INSERT INTO sys_sso_client_redirect_uri.*VALUES\s+` + regexp.QuoteMeta(tuple)).
		WillReturnResult(sqlmock.NewResult(51, 1))
	mock.ExpectCommit()

	transactor := store.NewSQLXTransactor(sqlxDB)
	err := transactor.WithinTransaction(context.Background(), func(txCtx context.Context) error {
		return repo.ReplaceClientRedirectURIs(txCtx, "demo-client", redirects, 7, now)
	})
	if err != nil {
		t.Fatalf("ReplaceClientRedirectURIs(): %v", err)
	}
	assertSQLExpectations(t, mock)
}

func TestReplaceClientRedirectURIsRejectsOversizedRepositoryInputBeforeSQL(t *testing.T) {
	db, mock, repo := newSSORepositoryMock(t)
	defer db.Close()
	sqlxDB, ok := repo.db.(*sqlx.DB)
	if !ok {
		t.Fatal("repository does not expose sqlx DB")
	}
	redirects := make([]domain.ClientRedirectURI, 201)
	mock.ExpectBegin()
	mock.ExpectRollback()
	transactor := store.NewSQLXTransactor(sqlxDB)
	err := transactor.WithinTransaction(context.Background(), func(txCtx context.Context) error {
		return repo.ReplaceClientRedirectURIs(txCtx, "demo-client", redirects, 7, time.Now().UTC())
	})
	if err == nil || !strings.Contains(err.Error(), "exceeds limit") {
		t.Fatalf("oversized repository input err=%v", err)
	}
	assertSQLExpectations(t, mock)
}
