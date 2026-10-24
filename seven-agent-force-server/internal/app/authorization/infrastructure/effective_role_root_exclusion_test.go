package infrastructure

import (
	"context"
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/app/authorization/domain"
	userfacade "github.com/CaixyPromise/seven-agent/force/internal/app/system/user/facade"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
)

func TestListEffectiveRoleIDsExcludesAuthorizationRootFromPostInheritance(t *testing.T) {
	rawDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer rawDB.Close()
	repo := &Repository{db: sqlx.NewDb(rawDB, "mysql"), users: postAuthorizationContext{}}
	mock.ExpectQuery(`(?s)SELECT sur\.roleId.*sys_user_role`).
		WithArgs(int64(1001), authorizationSetQueryMaxIDs+1).
		WillReturnRows(sqlmock.NewRows([]string{"roleId"}).AddRow(int64(2)))
	mock.ExpectQuery(`(?s)SELECT spr\.roleId.*sys_post_role.*COALESCE\(sr\.systemKey, ''\) <> \?`).
		WithArgs(int64(7), domain.AuthorizationRootSystemKey, authorizationSetQueryMaxIDs).
		WillReturnRows(sqlmock.NewRows([]string{"roleId"}).AddRow(int64(3)))

	roleIDs, err := repo.listEffectiveRoleIDs(context.Background(), 1001)
	if err != nil {
		t.Fatalf("list effective roles: %v", err)
	}
	if len(roleIDs) != 2 || roleIDs[0] != 2 || roleIDs[1] != 3 {
		t.Fatalf("unexpected effective role ids: %#v", roleIDs)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

type postAuthorizationContext struct {
	userfacade.AuthorizationContextFacade
}

func (postAuthorizationContext) ListAuthorizationPosts(context.Context, int64) ([]userfacade.AuthorizationPostRecord, error) {
	return []userfacade.AuthorizationPostRecord{{PostID: 7}}, nil
}
