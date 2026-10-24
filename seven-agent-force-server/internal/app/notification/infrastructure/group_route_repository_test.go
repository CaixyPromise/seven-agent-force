package infrastructure

import (
	"context"
	"regexp"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
)

func TestSetChannelOrganizationOwnershipQuotesPostgresNotificationSchema(t *testing.T) {
	rawDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer rawDB.Close()
	repo := NewRepository(sqlx.NewDb(rawDB, "postgres"))

	mock.ExpectExec(regexp.QuoteMeta(`UPDATE "sys_notification_channel" SET "scopeType"='ORGANIZATION',"organizationId"=$1 WHERE id=$2 AND "isDeleted"=0`)).
		WithArgs(int64(101), int64(202)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := repo.SetChannelOrganizationOwnership(context.Background(), 202, 101); err != nil {
		t.Fatalf("SetChannelOrganizationOwnership() error=%v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}
