package infrastructure

import (
	"context"
	"testing"

	"github.com/CaixyPromise/seven-agent/force/internal/app/authorization/domain"
	authorizationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/authorization/facade"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
)

func TestRepositoryMapsAndPersistsAuthorizationFeatureCodes(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	repo := &Repository{db: sqlx.NewDb(db, "sqlmock")}

	mock.ExpectQuery(`(?s)SELECT.*COALESCE\(featureCode, ''\) AS feature_code.*FROM sys_menu`).
		WillReturnRows(sqlmock.NewRows([]string{"menu_id", "feature_code"}).AddRow(int64(10), "docker.admin"))
	menus, err := repo.ListMenus(context.Background(), false)
	if err != nil {
		t.Fatalf("list menus: %v", err)
	}
	if len(menus) != 1 || menus[0].FeatureCode != "docker.admin" {
		t.Fatalf("unexpected menu featureCode: %#v", menus)
	}

	mock.ExpectQuery(`(?s)SELECT.*COALESCE\(featureCode, ''\) AS feature_code.*FROM sys_permission`).
		WillReturnRows(sqlmock.NewRows([]string{"permission_id", "code", "feature_code"}).
			AddRow(int64(20), "system:platform:list", "platform.control"))
	permissions, err := repo.ListPermissions(context.Background(), authorizationfacade.PermissionQuery{})
	if err != nil {
		t.Fatalf("list permissions: %v", err)
	}
	if len(permissions) != 1 || permissions[0].FeatureCode != "platform.control" {
		t.Fatalf("unexpected permission featureCode: %#v", permissions)
	}

	mock.ExpectExec(`INSERT INTO sys_menu .*featureCode`).WillReturnResult(sqlmock.NewResult(1, 1))
	if err := repo.CreateMenu(context.Background(), domain.MenuRecord{MenuID: 10, Name: "Docker", FeatureCode: "docker.admin"}, 1); err != nil {
		t.Fatalf("create menu: %v", err)
	}

	mock.ExpectExec(`INSERT INTO sys_permission .*featureCode`).WillReturnResult(sqlmock.NewResult(1, 1))
	if err := repo.CreatePermission(context.Background(), domain.PermissionRecord{PermissionID: 20, Code: "system:platform:list", Name: "Platform", FeatureCode: "platform.control"}, 1); err != nil {
		t.Fatalf("create permission: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}
