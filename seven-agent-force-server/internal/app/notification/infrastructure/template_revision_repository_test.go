package infrastructure

import (
	"context"
	"regexp"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/notification/domain"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
)

func TestTemplateDefinitionRepositoryBindsScopeBeforePagination(t *testing.T) {
	rawDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer rawDB.Close()
	repo := NewRepository(sqlx.NewDb(rawDB, "mysql"))

	mock.ExpectQuery(`(?s)SELECT COUNT\(1\) FROM \(SELECT .* FROM sys_notification_template_definition WHERE isDeleted=0 AND scopeId=\?\) t`).
		WithArgs("scope-a").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(`(?s)SELECT .* FROM sys_notification_template_definition WHERE isDeleted=0 AND scopeId=\? ORDER BY updateTime DESC, id DESC LIMIT \? OFFSET \?`).
		WithArgs("scope-a", 20, 0).
		WillReturnRows(templateDefinitionRows(101, "scope-a", "account_notice", nil, nil))

	items, total, err := repo.ListTemplateDefinitions(context.Background(), domain.TemplateDefinitionQuery{ScopeID: "scope-a", Current: 1, PageSize: 20})
	if err != nil || total != 1 || len(items) != 1 || items[0].ScopeID != "scope-a" {
		t.Fatalf("ListTemplateDefinitions() items=%#v total=%d err=%v", items, total, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestPublishTemplateRevisionUsesConditionalDraftAndAtomicPointerSteps(t *testing.T) {
	rawDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer rawDB.Close()
	repo := NewRepository(sqlx.NewDb(rawDB, "mysql"))
	now := time.Date(2026, 7, 27, 10, 0, 0, 0, time.UTC)

	mock.ExpectExec(`UPDATE sys_notification_template_revision SET state=\?, revisionVersion=revisionVersion\+1, publishedAt=\?, publishedBy=\?, updaterId=\?, updateTime=NOW\(\) WHERE id=\? AND templateDefinitionId=\? AND state=\? AND revisionVersion=\?`).
		WithArgs(domain.TemplateRevisionStatePublished, now, int64(9), int64(9), int64(201), int64(101), domain.TemplateRevisionStateDraft, 3).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE sys_notification_template_revision SET state=\?, updateTime=NOW\(\) WHERE templateDefinitionId=\? AND id<>\? AND state=\?`).
		WithArgs(domain.TemplateRevisionStateSuperseded, int64(101), int64(201), domain.TemplateRevisionStatePublished).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectExec(`UPDATE sys_notification_template_definition SET currentDraftRevisionId=NULL, currentPublishedRevisionId=\?, version=version\+1, updaterId=\?, updateTime=NOW\(\) WHERE id=\? AND currentDraftRevisionId=\? AND isDeleted=0`).
		WithArgs(int64(201), int64(9), int64(101), int64(201)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	changed, err := repo.PublishTemplateRevision(context.Background(), 101, 201, 3, 9, now)
	if err != nil || !changed {
		t.Fatalf("PublishTemplateRevision() changed=%t err=%v", changed, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestTemplateRevisionRepositoryQuotesPostgresCamelCaseIdentifiers(t *testing.T) {
	rawDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer rawDB.Close()
	repo := NewRepository(sqlx.NewDb(rawDB, "postgres"))
	item := &domain.TemplateDefinition{
		ID:           101,
		ScopeID:      "scope-a",
		TemplateCode: "account_notice",
		TemplateName: "账户提醒",
		Locale:       "zh-CN",
		Version:      1,
	}

	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO "sys_notification_template_definition" (id, "scopeId", "templateCode", "templateName", "locale", "currentDraftRevisionId", "currentPublishedRevisionId", version, "creatorId", "updaterId") VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	if err := repo.InsertTemplateDefinition(context.Background(), item); err != nil {
		t.Fatalf("InsertTemplateDefinition() error=%v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestTemplateRevisionAuditRepositoryQuotesPostgresActorID(t *testing.T) {
	rawDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer rawDB.Close()
	repo := NewRepository(sqlx.NewDb(rawDB, "postgres"))

	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO "sys_notification_template_revision_audit" (id, "templateDefinitionId", "scopeId", "action", "fromRevisionNo", "toRevisionNo", "actorId") VALUES ($1, $2, $3, $4, $5, $6, $7)`)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	if err := repo.InsertTemplateRevisionAudit(context.Background(), &domain.TemplateRevisionAudit{
		ID:                   301,
		TemplateDefinitionID: 101,
		ScopeID:              "scope-a",
		Action:               "PUBLISH",
	}); err != nil {
		t.Fatalf("InsertTemplateRevisionAudit() error=%v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func templateDefinitionRows(id int64, scopeID, code string, draftID, publishedID *int64) *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "scopeId", "templateCode", "templateName", "locale", "currentDraftRevisionId", "currentPublishedRevisionId", "version", "creatorId", "updaterId", "createTime", "updateTime", "isDeleted",
	}).AddRow(id, scopeID, code, "账户提醒", "zh-CN", draftID, publishedID, 1, nil, nil, time.Now(), time.Now(), 0)
}
