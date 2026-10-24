package infrastructure

import (
	"context"
	"testing"
	"time"

	authorizationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/authorization/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/features"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
)

func TestPagePermissionsUsesTwoBoundedFeatureAwareQueries(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	repo := &Repository{db: sqlx.NewDb(db, "sqlmock")}
	now := time.Date(2026, 7, 31, 12, 0, 0, 0, time.UTC)

	featureWhere := `WHERE isDeleted = 0 AND code LIKE \? AND \(featureCode IS NULL OR featureCode = '' OR featureCode IN \(\?\)\)`
	mock.ExpectQuery(`SELECT COUNT\(1\) FROM sys_permission `+featureWhere).
		WithArgs("%system%", string(features.PlatformControl)).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(41))
	mock.ExpectQuery(`(?s)FROM sys_permission `+featureWhere+`\s+ORDER BY code ASC, id ASC\s+LIMIT \? OFFSET \?`).
		WithArgs("%system%", string(features.PlatformControl), int64(20), int64(20)).
		WillReturnRows(sqlmock.NewRows([]string{
			"permission_id", "code", "feature_code", "name", "resource_type",
			"method", "path", "status", "description", "create_time", "update_time",
		}).AddRow(
			int64(102), "system:platform:list", string(features.PlatformControl), "Platforms", "API",
			"GET", "/system/platform", 0, "", now, now,
		))

	records, total, err := repo.PagePermissions(context.Background(), authorizationfacade.PermissionPageQuery{
		Current: 2,
		Size:    20,
		Code:    "system",
	}, true, []string{string(features.PlatformControl), string(features.PlatformControl)})
	if err != nil {
		t.Fatalf("PagePermissions(): %v", err)
	}
	if total != 41 || len(records) != 1 || records[0].Code != "system:platform:list" {
		t.Fatalf("records=%#v total=%d", records, total)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}

func TestPagePermissionsWithNoEnabledFeaturesKeepsOnlyUnscopedPermissions(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer db.Close()
	repo := &Repository{db: sqlx.NewDb(db, "sqlmock")}

	featureWhere := `WHERE isDeleted = 0 AND \(featureCode IS NULL OR featureCode = ''\)`
	mock.ExpectQuery(`SELECT COUNT\(1\) FROM sys_permission ` + featureWhere).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(0))
	mock.ExpectQuery(`(?s)FROM sys_permission `+featureWhere+`\s+ORDER BY code ASC, id ASC\s+LIMIT \? OFFSET \?`).
		WithArgs(int64(10), int64(0)).
		WillReturnRows(sqlmock.NewRows([]string{
			"permission_id", "code", "feature_code", "name", "resource_type",
			"method", "path", "status", "description", "create_time", "update_time",
		}))

	records, total, err := repo.PagePermissions(context.Background(), authorizationfacade.PermissionPageQuery{
		Current: 1,
		Size:    10,
	}, true, nil)
	if err != nil {
		t.Fatalf("PagePermissions(): %v", err)
	}
	if total != 0 || len(records) != 0 {
		t.Fatalf("records=%#v total=%d", records, total)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("sql expectations: %v", err)
	}
}
