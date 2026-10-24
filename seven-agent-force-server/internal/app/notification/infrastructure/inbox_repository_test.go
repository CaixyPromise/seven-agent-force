package infrastructure

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/notification/domain"
	dbstore "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/store"
	"github.com/DATA-DOG/go-sqlmock"
	"github.com/jmoiron/sqlx"
)

func TestCreateLogicalNotificationTreatsDuplicateAsExistingEvenWithFoundRows(t *testing.T) {
	rawDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer rawDB.Close()
	db := sqlx.NewDb(rawDB, "mysql")
	repo := NewRepository(db)
	item := testLogicalNotification(3001, "ntf_3001")

	// A MySQL connection configured with CLIENT_FOUND_ROWS may report one row
	// for an unchanged duplicate update. The post-write idempotency lookup is
	// the authority, not RowsAffected.
	mock.ExpectExec("INSERT INTO sys_notification").WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery("SELECT id, notificationId, scopeId").
		WithArgs(item.ScopeID, item.EventKey, item.IdempotencyKey).
		WillReturnRows(logicalNotificationRows(2999, "ntf_existing", item))

	created, err := repo.CreateLogicalNotification(context.Background(), item)
	if err != nil {
		t.Fatalf("CreateLogicalNotification() error=%v", err)
	}
	if created {
		t.Fatal("duplicate logical notification was reported as newly created")
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestInsertRecipientsCountsOnlyNewRowsWithMySQLDuplicateKey(t *testing.T) {
	rawDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer rawDB.Close()
	db := sqlx.NewDb(rawDB, "mysql")
	repo := NewRepository(db)
	item := domain.Recipient{
		ID:             4001,
		RecipientID:    "nrc_4001",
		NotificationID: 3001,
		ScopeID:        "local",
		UserID:         101,
		EventKey:       "test.event",
		Category:       "GENERAL",
		Priority:       "NORMAL",
		Title:          "标题",
		Content:        "正文",
		MailboxVersion: 5001,
	}

	mock.ExpectExec("INSERT INTO sys_notification_recipient").WillReturnResult(sqlmock.NewResult(4001, 1))
	inserted, err := repo.InsertRecipients(context.Background(), []domain.Recipient{item})
	if err != nil || inserted != 1 {
		t.Fatalf("new recipient inserted=%d err=%v, want 1 nil", inserted, err)
	}

	duplicate := item
	duplicate.ID = 4002
	duplicate.RecipientID = "nrc_4002"
	mock.ExpectExec("INSERT INTO sys_notification_recipient").WillReturnResult(sqlmock.NewResult(4001, 1))
	inserted, err = repo.InsertRecipients(context.Background(), []domain.Recipient{duplicate})
	if err != nil || inserted != 0 {
		t.Fatalf("duplicate recipient inserted=%d err=%v, want 0 nil", inserted, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestInsertExternalTargetsUsesDeterministicChunks(t *testing.T) {
	rawDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer rawDB.Close()
	db := sqlx.NewDb(rawDB, "mysql")
	repo := NewRepository(db)
	items := make([]domain.ExternalTarget, 100)
	for index := range items {
		id := int64(index + 1)
		items[index] = domain.ExternalTarget{
			ID:                  id,
			ExternalTargetID:    fmt.Sprintf("net_%d", id),
			NotificationID:      3001,
			ScopeID:             "local",
			ConnectionRef:       "feishu-app",
			ProviderCode:        domain.ChannelTypeFeishuApp,
			IdentityKind:        domain.ExternalIdentityFeishuOpenID,
			SubjectCiphertext:   "ciphertext",
			SubjectEDEK:         "edek",
			SubjectWrapKeyRef:   "wrap-key",
			SubjectDigest:       fmt.Sprintf("digest-%d", id),
			SubjectDigestKeyRef: "digest-key",
			ProviderParamsJSON:  "{}",
		}
	}

	batchPattern := `(?s)INSERT INTO sys_notification_external_target .*VALUES \([^)]+\),\s*\([^)]+\)`
	mock.ExpectBegin()
	mock.ExpectExec(batchPattern).WillReturnResult(sqlmock.NewResult(0, 50))
	mock.ExpectExec(batchPattern).WillReturnResult(sqlmock.NewResult(0, 50))
	mock.ExpectCommit()

	err = dbstore.NewSQLXTransactor(db).WithinTransaction(context.Background(), func(txCtx context.Context) error {
		return repo.InsertExternalTargets(txCtx, items)
	})
	if err != nil {
		t.Fatalf("InsertExternalTargets() error=%v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestInsertExternalTargetsUsesEmptyAuthorityFieldsForNonEnterpriseRecipient(t *testing.T) {
	rawDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer rawDB.Close()
	db := sqlx.NewDb(rawDB, "mysql")
	repo := NewRepository(db)
	item := domain.ExternalTarget{
		ID:                  1,
		ExternalTargetID:    "net_email_1",
		NotificationID:      3001,
		ScopeID:             "local",
		ConnectionRef:       "email",
		ProviderCode:        domain.ChannelTypeEmail,
		IdentityKind:        domain.ExternalIdentityEmailAddress,
		SubjectCiphertext:   "ciphertext",
		SubjectEDEK:         "edek",
		SubjectWrapKeyRef:   "wrap-key",
		SubjectDigest:       "digest-email-1",
		SubjectDigestKeyRef: "digest-key",
		ProviderParamsJSON:  "{}",
	}

	mock.ExpectBegin()
	mock.ExpectExec(`INSERT INTO sys_notification_external_target`).
		WithArgs(
			item.ID, item.ExternalTargetID, item.NotificationID, item.ScopeID,
			nil, nil, nil, item.ExternalIdentityVersion, nil, item.EnterpriseConnectionVersion,
			"", "", item.ConnectionRef, item.ProviderCode, item.IdentityKind,
			item.SubjectCiphertext, item.SubjectEDEK, item.SubjectWrapKeyRef, item.SubjectDigest, item.SubjectDigestKeyRef, item.ProviderParamsJSON,
		).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err = dbstore.NewSQLXTransactor(db).WithinTransaction(context.Background(), func(txCtx context.Context) error {
		return repo.InsertExternalTargets(txCtx, []domain.ExternalTarget{item})
	})
	if err != nil {
		t.Fatalf("InsertExternalTargets() error=%v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestNotificationBatchWritesRequireActiveTransaction(t *testing.T) {
	rawDB, _, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer rawDB.Close()
	db := sqlx.NewDb(rawDB, "mysql")
	repo := NewRepository(db)

	target := domain.ExternalTarget{
		ID:                  1,
		ExternalTargetID:    "net_1",
		NotificationID:      3001,
		ScopeID:             "local",
		ConnectionRef:       "feishu-app",
		ProviderCode:        domain.ChannelTypeFeishuApp,
		IdentityKind:        domain.ExternalIdentityFeishuOpenID,
		SubjectCiphertext:   "ciphertext",
		SubjectEDEK:         "edek",
		SubjectWrapKeyRef:   "wrap-key",
		SubjectDigest:       "digest-1",
		SubjectDigestKeyRef: "digest-key",
		ProviderParamsJSON:  "{}",
	}
	delivery := domain.Delivery{
		ID:            1,
		DeliveryID:    "delivery-1",
		RequestDigest: "digest-1",
		ChannelCode:   "feishu-app",
		ChannelType:   domain.ChannelTypeFeishuApp,
		Status:        domain.DeliveryStatusPending,
	}
	event := domain.OutboxEvent{
		ID:            1,
		EventID:       "notification:1",
		EventType:     domain.OutboxEventNotificationDispatch,
		AggregateType: domain.OutboxAggregateNotification,
		AggregateID:   "delivery-1",
		Payload:       "{}",
	}
	snapshot := domain.HTTPDeliverySnapshot{
		ID:          1,
		DeliveryID:  "delivery-1",
		ScopeID:     "local",
		ChannelCode: "static-http",
		ChannelType: domain.ChannelTypeFeishuWebhook,
		ConfigJSON:  "{}",
	}
	for name, write := range map[string]func() error{
		"targets":    func() error { return repo.InsertExternalTargets(context.Background(), []domain.ExternalTarget{target}) },
		"deliveries": func() error { return repo.InsertDeliveries(context.Background(), []domain.Delivery{delivery}) },
		"http-snapshots": func() error {
			return repo.InsertHTTPDeliverySnapshots(context.Background(), []domain.HTTPDeliverySnapshot{snapshot})
		},
		"outbox": func() error { return repo.AppendOutboxBatch(context.Background(), []domain.OutboxEvent{event}) },
	} {
		t.Run(name, func(t *testing.T) {
			err := write()
			if err == nil || !strings.Contains(err.Error(), "active transaction") {
				t.Fatalf("write error=%v, want active transaction requirement", err)
			}
		})
	}
}

func TestInsertHTTPDeliverySnapshotsUsesDeterministicChunks(t *testing.T) {
	rawDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer rawDB.Close()
	db := sqlx.NewDb(rawDB, "mysql")
	repo := NewRepository(db)
	items := make([]domain.HTTPDeliverySnapshot, 100)
	for index := range items {
		id := int64(index + 1)
		items[index] = domain.HTTPDeliverySnapshot{
			ID:          id,
			DeliveryID:  fmt.Sprintf("delivery-%d", id),
			ScopeID:     "local",
			ChannelCode: fmt.Sprintf("static-http-%d", id),
			ChannelType: domain.ChannelTypeFeishuWebhook,
			ConfigJSON:  "{}",
		}
	}

	batchPattern := `(?s)INSERT INTO sys_notification_http_delivery_snapshot .*VALUES \([^)]+\),\s*\([^)]+\)`
	mock.ExpectBegin()
	mock.ExpectExec(batchPattern).WillReturnResult(sqlmock.NewResult(0, 50))
	mock.ExpectExec(batchPattern).WillReturnResult(sqlmock.NewResult(0, 50))
	mock.ExpectCommit()

	err = dbstore.NewSQLXTransactor(db).WithinTransaction(context.Background(), func(txCtx context.Context) error {
		return repo.InsertHTTPDeliverySnapshots(txCtx, items)
	})
	if err != nil {
		t.Fatalf("InsertHTTPDeliverySnapshots() error=%v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestInsertInboxRecipientsRejectsMoreThanMaterializationBatchBeforeSQL(t *testing.T) {
	rawDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer rawDB.Close()
	repo := NewRepository(sqlx.NewDb(rawDB, "mysql"))
	items := make([]domain.Recipient, 101)
	for index := range items {
		id := int64(index + 1)
		items[index] = domain.Recipient{
			ID:             id,
			RecipientID:    fmt.Sprintf("recipient-%d", id),
			NotificationID: 3001,
			ScopeID:        "local",
			UserID:         id,
		}
	}

	created, err := repo.InsertInboxRecipients(context.Background(), items)
	if err == nil || !strings.Contains(err.Error(), "batch exceeds limit") {
		t.Fatalf("InsertInboxRecipients() created=%d error=%v, want hard batch limit", len(created), err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("limit rejection reached SQL: %v", err)
	}
}

func TestInsertDeliveriesUsesDeterministicChunks(t *testing.T) {
	rawDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer rawDB.Close()
	db := sqlx.NewDb(rawDB, "mysql")
	repo := NewRepository(db)
	items := make([]domain.Delivery, 100)
	for index := range items {
		id := int64(index + 1)
		items[index] = domain.Delivery{
			ID:            id,
			DeliveryID:    fmt.Sprintf("delivery-%d", id),
			RequestDigest: fmt.Sprintf("digest-%d", id),
			ChannelCode:   "feishu-app",
			ChannelType:   domain.ChannelTypeFeishuApp,
			Status:        domain.DeliveryStatusPending,
		}
	}

	batchPattern := `(?s)INSERT INTO sys_notification_delivery .*VALUES \([^)]+\),\s*\([^)]+\)`
	mock.ExpectBegin()
	mock.ExpectExec(batchPattern).WillReturnResult(sqlmock.NewResult(0, 50))
	mock.ExpectExec(batchPattern).WillReturnResult(sqlmock.NewResult(0, 50))
	mock.ExpectCommit()

	err = dbstore.NewSQLXTransactor(db).WithinTransaction(context.Background(), func(txCtx context.Context) error {
		return repo.InsertDeliveries(txCtx, items)
	})
	if err != nil {
		t.Fatalf("InsertDeliveries() error=%v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAppendOutboxBatchUsesDeterministicChunks(t *testing.T) {
	rawDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer rawDB.Close()
	db := sqlx.NewDb(rawDB, "mysql")
	repo := NewRepository(db)
	events := make([]domain.OutboxEvent, 100)
	for index := range events {
		id := int64(index + 1)
		events[index] = domain.OutboxEvent{
			ID:            id,
			EventID:       fmt.Sprintf("notification:%d", id),
			ScopeID:       "local",
			EventType:     domain.OutboxEventNotificationDispatch,
			AggregateType: domain.OutboxAggregateNotification,
			AggregateID:   fmt.Sprintf("delivery-%d", id),
			Payload:       "{}",
			Status:        "PENDING",
		}
	}

	batchPattern := `(?s)INSERT INTO sys_outbox_event .*VALUES \([^)]+\),\s*\([^)]+\)`
	mock.ExpectBegin()
	mock.ExpectExec(batchPattern).WillReturnResult(sqlmock.NewResult(0, 50))
	mock.ExpectExec(batchPattern).WillReturnResult(sqlmock.NewResult(0, 50))
	mock.ExpectCommit()

	err = dbstore.NewSQLXTransactor(db).WithinTransaction(context.Background(), func(txCtx context.Context) error {
		return repo.AppendOutboxBatch(txCtx, events)
	})
	if err != nil {
		t.Fatalf("AppendOutboxBatch() error=%v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAppendOutboxBatchQuotesPostgresCamelCase(t *testing.T) {
	rawDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer rawDB.Close()
	db := sqlx.NewDb(rawDB, "postgres")
	repo := NewRepository(db)
	event := domain.OutboxEvent{
		ID:            1,
		EventID:       "notification:1",
		ScopeID:       "local",
		EventType:     domain.OutboxEventNotificationDispatch,
		AggregateType: domain.OutboxAggregateNotification,
		AggregateID:   "delivery-1",
		Payload:       "{}",
		Status:        "PENDING",
	}

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO "sys_outbox_event" (id, "eventId", "eventOwner", "scopeId", "eventType", "aggregateType", "aggregateId", payload, "status", "retryCount", "nextRetryAt", "errorMsg") VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12)`)).
		WithArgs(event.ID, event.EventID, notificationOutboxOwner, event.ScopeID, event.EventType, event.AggregateType, event.AggregateID, event.Payload, event.Status, event.RetryCount, nil, nil).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectCommit()

	err = dbstore.NewSQLXTransactor(db).WithinTransaction(context.Background(), func(txCtx context.Context) error {
		return repo.AppendOutboxBatch(txCtx, []domain.OutboxEvent{event})
	})
	if err != nil {
		t.Fatalf("AppendOutboxBatch() error=%v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestCreateLogicalNotificationQuotesCamelCaseSchemaForPostgres(t *testing.T) {
	rawDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer rawDB.Close()
	repo := NewRepository(sqlx.NewDb(rawDB, "postgres"))
	item := testLogicalNotification(3002, "ntf_3002")

	mock.ExpectExec(regexp.QuoteMeta(`INSERT INTO "sys_notification" (id, "notificationId", "scopeId", "organizationId", "eventKey", "idempotencyKey"`)).
		WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`(?s)SELECT .* FROM "sys_notification" WHERE "scopeId"=\$1 AND "eventKey"=\$2 AND "idempotencyKey"=\$3 LIMIT 1`).
		WithArgs(item.ScopeID, item.EventKey, item.IdempotencyKey).
		WillReturnRows(logicalNotificationRows(item.ID, item.NotificationID, item))

	created, err := repo.CreateLogicalNotification(context.Background(), item)
	if err != nil || !created {
		t.Fatalf("CreateLogicalNotification() created=%t err=%v", created, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestAdvanceMailboxChangeQuotesMailboxSchemaForPostgres(t *testing.T) {
	rawDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer rawDB.Close()
	repo := NewRepository(sqlx.NewDb(rawDB, "postgres"))
	now := time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC)

	mock.ExpectQuery(`(?s)INSERT INTO "sys_notification_mailbox".*ON CONFLICT \("scopeId", "userId"\) DO UPDATE.*"changeSequence"="sys_notification_mailbox"\."changeSequence" \+ 1.*RETURNING id, "scopeId", "userId", "mailboxKey", "changeSequence", "createTime", "updateTime"`).
		WithArgs("local", int64(42), sqlmock.AnyArg()).
		WillReturnRows(mailboxRows(7001, "local", 42, "mbx_opaque", 3, now))

	mailbox, err := repo.AdvanceMailboxChange(context.Background(), "local", 42)
	if err != nil || mailbox == nil || mailbox.ChangeSequence != 3 || mailbox.MailboxKey != "mbx_opaque" {
		t.Fatalf("AdvanceMailboxChange() mailbox=%#v err=%v", mailbox, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestLockExpiredInboxRecipientReturnsOnlyDueUnprocessedProjection(t *testing.T) {
	rawDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer rawDB.Close()
	repo := NewRepository(sqlx.NewDb(rawDB, "mysql"))
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	expiresAt := now.Add(-time.Second)
	item := domain.Recipient{
		ID:             5001,
		RecipientID:    "nrc_expiring",
		NotificationID: 4001,
		ScopeID:        "local",
		UserID:         101,
		EventKey:       "account.session.expiring",
		Category:       "ACCOUNT",
		Priority:       "NORMAL",
		Title:          "会话即将到期",
		Content:        "完整正文",
		ExpiresAt:      &expiresAt,
		MailboxVersion: 7,
		CreateTime:     now.Add(-time.Minute),
		UpdateTime:     now.Add(-time.Minute),
	}

	mock.ExpectQuery(`(?s)SELECT .*expiresAt, expiredAt, firstSeenAt.*FROM sys_notification_recipient WHERE id=\? AND expiredAt IS NULL AND expiresAt IS NOT NULL AND expiresAt <= \? FOR UPDATE`).
		WithArgs(item.ID, now).
		WillReturnRows(inboxRecipientRows(item))

	locked, err := repo.LockExpiredInboxRecipient(context.Background(), item.ID, now)
	if err != nil || locked == nil || locked.RecipientID != item.RecipientID || locked.ExpiredAt != nil {
		t.Fatalf("LockExpiredInboxRecipient() item=%#v err=%v", locked, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestListExpiredInboxRecipientsStaysInsideConfiguredScope(t *testing.T) {
	rawDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer rawDB.Close()
	repo := NewRepository(sqlx.NewDb(rawDB, "mysql"))
	now := time.Date(2026, 7, 23, 12, 0, 0, 0, time.UTC)
	expiresAt := now.Add(-time.Second)
	item := domain.Recipient{
		ID:             5001,
		RecipientID:    "nrc_expiring",
		NotificationID: 4001,
		ScopeID:        "local",
		UserID:         101,
		EventKey:       "account.session.expiring",
		Category:       "ACCOUNT",
		Priority:       "NORMAL",
		Title:          "会话即将到期",
		Content:        "完整正文",
		ExpiresAt:      &expiresAt,
		MailboxVersion: 7,
		CreateTime:     now.Add(-time.Minute),
		UpdateTime:     now.Add(-time.Minute),
	}

	mock.ExpectQuery(`(?s)SELECT .*FROM sys_notification_recipient WHERE scopeId=\? AND expiredAt IS NULL AND expiresAt IS NOT NULL AND expiresAt <= \? ORDER BY expiresAt ASC, id ASC LIMIT \?`).
		WithArgs("local", now, 50).
		WillReturnRows(inboxRecipientRows(item))

	items, err := repo.ListExpiredInboxRecipients(context.Background(), "local", now, 50)
	if err != nil || len(items) != 1 || items[0].ScopeID != "local" {
		t.Fatalf("ListExpiredInboxRecipients() items=%#v err=%v", items, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestMaterializationRepositoryFencesListClaimAdvanceAndFailureByScope(t *testing.T) {
	rawDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer rawDB.Close()
	repo := NewRepository(sqlx.NewDb(rawDB, "mysql"))
	now := time.Date(2026, 7, 28, 9, 0, 0, 0, time.UTC)
	task := domain.MaterializationTask{
		ID: 6101, TaskID: "task-local", NotificationID: 6001, ScopeID: "local",
		AudienceJSON: `{"userIds":[101]}`, Cursor: `{}`, Status: domain.TaskStatusPending,
		NextRunAt: now.Add(-time.Minute), CreateTime: now.Add(-time.Hour), UpdateTime: now.Add(-time.Hour),
	}

	mock.ExpectQuery(`(?s)SELECT .* FROM sys_notification_materialization_task WHERE scopeId=\? AND \(.*ORDER BY nextRunAt ASC, id ASC LIMIT \?`).
		WithArgs("local", domain.TaskStatusPending, sqlmock.AnyArg(), domain.TaskStatusProcessing, sqlmock.AnyArg(), 20).
		WillReturnRows(materializationTaskRows(task))
	items, err := repo.ListReadyMaterializationTasks(context.Background(), "local", 20)
	if err != nil || len(items) != 1 || items[0].ScopeID != "local" {
		t.Fatalf("ListReadyMaterializationTasks() items=%#v err=%v", items, err)
	}

	mock.ExpectExec(`(?s)UPDATE sys_notification_materialization_task SET status=\?, leaseOwner=\?, leaseToken=\?, leaseUntil=\?.*WHERE scopeId=\? AND id=\? AND \(`).
		WithArgs(domain.TaskStatusProcessing, "worker-a", sqlmock.AnyArg(), sqlmock.AnyArg(), domain.TaskStatusProcessing, "local", task.ID, domain.TaskStatusPending, now, domain.TaskStatusProcessing, now).
		WillReturnResult(sqlmock.NewResult(0, 0))
	claimed, ok, err := repo.TryClaimMaterializationTask(context.Background(), "local", task.ID, "worker-a", now)
	if err != nil || ok || claimed != nil {
		t.Fatalf("wrong-owner claim result task=%#v claimed=%t err=%v", claimed, ok, err)
	}

	mock.ExpectExec(`(?s)UPDATE sys_notification_materialization_task SET materializationCursor=\?.*WHERE scopeId=\? AND id=\? AND status=\? AND leaseToken=\?`).
		WithArgs(`{}`, domain.TaskStatusDone, int64(1), now, "local", task.ID, domain.TaskStatusProcessing, "foreign-lease").
		WillReturnResult(sqlmock.NewResult(0, 0))
	advanced, err := repo.AdvanceMaterializationTask(context.Background(), "local", task.ID, "foreign-lease", `{}`, domain.TaskStatusDone, 1, now)
	if err != nil || advanced {
		t.Fatalf("wrong-owner advance=%t err=%v", advanced, err)
	}

	mock.ExpectExec(`(?s)UPDATE sys_notification_materialization_task SET status=\?, retryCount=\?.*WHERE scopeId=\? AND id=\? AND status=\? AND leaseToken=\?`).
		WithArgs(domain.TaskStatusPending, 1, now, "retry", "local", task.ID, domain.TaskStatusProcessing, "foreign-lease").
		WillReturnResult(sqlmock.NewResult(0, 0))
	failed, err := repo.FailMaterializationTask(context.Background(), "local", task.ID, "foreign-lease", domain.TaskStatusPending, "retry", 1, now)
	if err != nil || failed {
		t.Fatalf("wrong-owner failure update=%t err=%v", failed, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestListChannelsBindsCurrentScopeInSQLBeforePagination(t *testing.T) {
	rawDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer rawDB.Close()
	repo := NewRepository(sqlx.NewDb(rawDB, "mysql"))

	// Both count and row queries must include scope before LIMIT, otherwise a
	// foreign backlog can leak metadata or starve the current scope's page.
	mock.ExpectQuery(`(?s)SELECT COUNT\(1\) FROM \(SELECT .* FROM sys_notification_channel WHERE isDeleted=0 AND scopeId=\?\) t`).
		WithArgs("node:gray-b").
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(1))
	mock.ExpectQuery(`(?s)SELECT .* FROM sys_notification_channel WHERE isDeleted=0 AND scopeId=\? ORDER BY priority ASC, id DESC LIMIT \? OFFSET \?`).
		WithArgs("node:gray-b", 20, 0).
		WillReturnRows(channelRows(domain.Channel{ID: 1, ChannelCode: "scope-b", ChannelName: "scope b", ChannelType: domain.ChannelTypeMock, ScopeID: "node:gray-b", Status: domain.ChannelStatusEnabled, Priority: 10}))

	items, total, err := repo.ListChannels(context.Background(), domain.ChannelQuery{ScopeID: "node:gray-b", Current: 1, PageSize: 20})
	if err != nil || total != 1 || len(items) != 1 || items[0].ScopeID != "node:gray-b" {
		t.Fatalf("ListChannels() items=%#v total=%d err=%v", items, total, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestFindActiveSceneBindingBindsCurrentScope(t *testing.T) {
	rawDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer rawDB.Close()
	repo := NewRepository(sqlx.NewDb(rawDB, "mysql"))
	const scopeID = "node:gray-b"

	mock.ExpectQuery(`(?s)SELECT .* FROM sys_notification_scene_binding WHERE sceneCode=\? AND enabled=\? AND isDeleted=0 AND scopeId=\? ORDER BY priority ASC, id DESC LIMIT 1`).
		WithArgs("scope-test", 1, scopeID).
		WillReturnRows(sceneBindingRows(domain.SceneBinding{ID: 2, SceneCode: "scope-test", ScopeID: scopeID, SceneName: "Scope B", ChannelCode: "scope-b-channel", TemplateCode: "scope-b-template", Enabled: true, Priority: 10, MaxRetry: 3, RetryIntervalSeconds: 60}))

	binding, err := repo.FindActiveSceneBinding(context.Background(), scopeID, "scope-test")
	if err != nil || binding == nil || binding.ScopeID != scopeID {
		t.Fatalf("FindActiveSceneBinding() binding=%#v err=%v", binding, err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestUpsertChannelBindsUpdateToScopeBeforeWriting(t *testing.T) {
	rawDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer rawDB.Close()
	repo := NewRepository(sqlx.NewDb(rawDB, "mysql"))
	foreign := domain.Channel{
		ID:          11,
		ChannelCode: "scope-race-channel",
		ChannelName: "Scope A channel",
		ChannelType: domain.ChannelTypeMock,
		ScopeID:     "node:gray-a",
		Status:      domain.ChannelStatusEnabled,
		Priority:    100,
	}
	attempt := foreign
	attempt.ScopeID = "node:gray-b"
	attempt.ChannelName = "Scope B attempted overwrite"

	mock.ExpectQuery(`(?s)SELECT id, channelCode, channelName, channelType, .* FROM sys_notification_channel WHERE channelCode=\? AND isDeleted=0 LIMIT 1`).
		WithArgs(attempt.ChannelCode).
		WillReturnRows(channelRows(foreign))
	mock.ExpectExec(`(?s)UPDATE sys_notification_channel SET .* WHERE channelCode=\? AND isDeleted=0 AND scopeId=\?`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`(?s)SELECT id, channelCode, channelName, channelType, .* FROM sys_notification_channel WHERE channelCode=\? AND isDeleted=0 LIMIT 1`).
		WithArgs(attempt.ChannelCode).
		WillReturnRows(channelRows(foreign))

	if err := repo.UpsertChannel(context.Background(), &attempt); !errors.Is(err, domain.ErrScopedConfigurationNotFound) {
		t.Fatalf("foreign scope UpsertChannel() error=%v, want scoped miss", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestUpsertChannelTreatsSameScopeZeroRowUpdateAsNoop(t *testing.T) {
	rawDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer rawDB.Close()
	repo := NewRepository(sqlx.NewDb(rawDB, "mysql"))
	item := domain.Channel{
		ID:          12,
		ChannelCode: "scope-noop-channel",
		ChannelName: "Scope B channel",
		ChannelType: domain.ChannelTypeMock,
		ScopeID:     "node:gray-b",
		Status:      domain.ChannelStatusEnabled,
		Priority:    100,
	}

	mock.ExpectQuery(`(?s)SELECT id, channelCode, channelName, channelType, .* FROM sys_notification_channel WHERE channelCode=\? AND isDeleted=0 LIMIT 1`).
		WithArgs(item.ChannelCode).
		WillReturnRows(channelRows(item))
	mock.ExpectExec(`(?s)UPDATE sys_notification_channel SET .* WHERE channelCode=\? AND isDeleted=0 AND scopeId=\?`).
		WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectQuery(`(?s)SELECT id, channelCode, channelName, channelType, .* FROM sys_notification_channel WHERE channelCode=\? AND isDeleted=0 LIMIT 1`).
		WithArgs(item.ChannelCode).
		WillReturnRows(channelRows(item))

	if err := repo.UpsertChannel(context.Background(), &item); err != nil {
		t.Fatalf("same scope no-op UpsertChannel() error=%v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestUpsertChannelStoresEmptyEnterpriseConnectionDigestForEmail(t *testing.T) {
	rawDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer rawDB.Close()
	repo := NewRepository(sqlx.NewDb(rawDB, "postgres"))
	item := domain.Channel{
		ID:          13,
		ChannelCode: "smtp-primary",
		ChannelName: "Platform SMTP",
		ChannelType: domain.ChannelTypeEmail,
		ScopeID:     "local",
		Status:      domain.ChannelStatusEnabled,
		Priority:    100,
		ConfigJSON:  `{"host":"smtp.example.com","port":587,"startTls":true}`,
	}

	mock.ExpectQuery(`(?s)SELECT .* FROM "sys_notification_channel" WHERE "channelCode"=\$1 AND "isDeleted"=0 LIMIT 1`).
		WithArgs(item.ChannelCode).
		WillReturnError(sql.ErrNoRows)
	mock.ExpectExec(`(?s)INSERT INTO "sys_notification_channel" .*"enterpriseConnectionDigest".* VALUES`).
		WithArgs(
			item.ID, item.ChannelCode, item.ChannelName, item.ChannelType, item.ScopeID, "SYSTEM",
			nil, nil, int64(0), "", item.Status, item.Priority, item.ConfigJSON,
			nil, nil, nil, nil, nil, nil, nil,
		).
		WillReturnResult(sqlmock.NewResult(item.ID, 1))

	if err := repo.UpsertChannel(context.Background(), &item); err != nil {
		t.Fatalf("UpsertChannel() error=%v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestInsertDeliveryStoresNonNullSnapshotDefaults(t *testing.T) {
	rawDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer rawDB.Close()
	repo := NewRepository(sqlx.NewDb(rawDB, "postgres"))
	item := domain.Delivery{
		ID:            14,
		DeliveryID:    "otp-14",
		RequestDigest: "digest-14",
		SceneCode:     domain.SceneChallengeOTP,
		ChannelCode:   "smtp-primary",
		ChannelType:   domain.ChannelTypeEmail,
		TemplateCode:  "otp-template",
		Target:        "member@example.com",
		TargetMasked:  "m***@example.com",
		ContentTier:   domain.DeliveryContentTierSecretEphemeral,
		Status:        domain.DeliveryStatusPending,
	}

	mock.ExpectExec(`(?s)INSERT INTO "sys_notification_delivery" .*"presentationType".* VALUES`).
		WithArgs(
			item.ID, item.DeliveryID, item.RequestDigest, nil, nil, nil, nil,
			item.SceneCode, item.ChannelCode, item.ChannelType, item.TemplateCode,
			item.Target, item.TargetMasked, nil, nil, nil, nil, nil,
			domain.DeliveryPresentationText, "", "", "", item.ContentTier,
			item.Status, 0, 0, nil, nil, nil, nil, nil,
		).
		WillReturnResult(sqlmock.NewResult(item.ID, 1))

	if err := repo.InsertDelivery(context.Background(), &item); err != nil {
		t.Fatalf("InsertDelivery() error=%v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestInsertInboxRecipientsAdvancesMailboxOnlyForNewMySQLRows(t *testing.T) {
	rawDB, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock: %v", err)
	}
	defer rawDB.Close()
	repo := NewRepository(sqlx.NewDb(rawDB, "mysql"))
	now := time.Date(2026, 7, 22, 12, 0, 0, 0, time.UTC)
	item := domain.Recipient{
		ID:             5001,
		RecipientID:    "nrc_5001",
		NotificationID: 4001,
		ScopeID:        "local",
		UserID:         101,
		EventKey:       "account.security.changed",
		Category:       "ACCOUNT",
		Priority:       "NORMAL",
		Title:          "账号安全设置已更新",
		Content:        "完整正文",
	}

	mock.ExpectExec(`INSERT INTO sys_notification_recipient`).WillReturnResult(sqlmock.NewResult(item.ID, 1))
	mock.ExpectExec(`INSERT INTO sys_notification_mailbox`).WithArgs(item.ScopeID, item.UserID, sqlmock.AnyArg()).WillReturnResult(sqlmock.NewResult(0, 1))
	mock.ExpectQuery(`SELECT id, scopeId, userId, mailboxKey, changeSequence, createTime, updateTime\s+FROM sys_notification_mailbox WHERE scopeId=\? AND userId=\? FOR UPDATE`).
		WithArgs(item.ScopeID, item.UserID).
		WillReturnRows(mailboxRows(8001, item.ScopeID, item.UserID, "mbx_opaque", 1, now))
	mock.ExpectExec(`UPDATE sys_notification_recipient`).WithArgs(int64(1), item.ID, item.ScopeID, item.UserID).WillReturnResult(sqlmock.NewResult(0, 1))

	created, err := repo.InsertInboxRecipients(context.Background(), []domain.Recipient{item})
	if err != nil || len(created) != 1 || created[0].MailboxVersion != 1 {
		t.Fatalf("InsertInboxRecipients() created=%#v err=%v", created, err)
	}

	duplicate := item
	duplicate.ID = 5002
	duplicate.RecipientID = "nrc_5002"
	mock.ExpectExec(`INSERT INTO sys_notification_recipient`).WillReturnResult(sqlmock.NewResult(item.ID, 1))
	created, err = repo.InsertInboxRecipients(context.Background(), []domain.Recipient{duplicate})
	if err != nil || len(created) != 0 {
		t.Fatalf("duplicate InsertInboxRecipients() created=%#v err=%v", created, err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func testLogicalNotification(id int64, notificationID string) *domain.LogicalNotification {
	return &domain.LogicalNotification{
		ID:                 id,
		NotificationID:     notificationID,
		ScopeID:            "local",
		EventKey:           "test.event",
		IdempotencyKey:     "test-key",
		RequestFingerprint: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		AudienceJSON:       `{"userIds":[101]}`,
		Category:           "GENERAL",
		Priority:           "NORMAL",
		Title:              "标题",
		Content:            "正文",
		Status:             domain.NotificationStatusMaterialized,
	}
}

func logicalNotificationRows(id int64, notificationID string, template *domain.LogicalNotification) *sqlmock.Rows {
	now := time.Date(2026, 7, 22, 10, 0, 0, 0, time.UTC)
	return sqlmock.NewRows([]string{
		"id", "notificationId", "scopeId", "eventKey", "idempotencyKey", "requestFingerprint", "audienceJson", "category", "priority", "mandatory", "title", "content", "deepLink", "scheduleAt", "expiresAt", "traceId", "status", "creatorId", "createTime", "updateTime",
	}).AddRow(id, notificationID, template.ScopeID, template.EventKey, template.IdempotencyKey, template.RequestFingerprint, template.AudienceJSON, template.Category, template.Priority, false, template.Title, template.Content, "", nil, nil, "", template.Status, nil, now, now)
}

func mailboxRows(id int64, scopeID string, userID int64, mailboxKey string, sequence int64, now time.Time) *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "scopeId", "userId", "mailboxKey", "changeSequence", "createTime", "updateTime",
	}).AddRow(id, scopeID, userID, mailboxKey, sequence, now, now)
}

func inboxRecipientRows(item domain.Recipient) *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "recipientId", "notificationId", "scopeId", "userId", "eventKey", "category", "priority", "mandatory", "title", "content",
		"deepLink", "expiresAt", "expiredAt", "firstSeenAt", "readAt", "archivedAt", "mailboxVersion", "createTime", "updateTime",
	}).AddRow(
		item.ID, item.RecipientID, item.NotificationID, item.ScopeID, item.UserID, item.EventKey, item.Category, item.Priority, item.Mandatory, item.Title, item.Content,
		item.DeepLink, item.ExpiresAt, item.ExpiredAt, item.FirstSeenAt, item.ReadAt, item.ArchivedAt, item.MailboxVersion, item.CreateTime, item.UpdateTime,
	)
}

func materializationTaskRows(item domain.MaterializationTask) *sqlmock.Rows {
	return sqlmock.NewRows([]string{
		"id", "taskId", "notificationId", "scopeId", "audienceJson", "materializationCursor", "status",
		"materializedCount", "retryCount", "nextRunAt", "leaseOwner", "leaseToken", "leaseUntil",
		"lastError", "createTime", "updateTime",
	}).AddRow(
		item.ID, item.TaskID, item.NotificationID, item.ScopeID, item.AudienceJSON, item.Cursor, item.Status,
		item.MaterializedCount, item.RetryCount, item.NextRunAt, item.LeaseOwner, item.LeaseToken, item.LeaseUntil,
		item.LastError, item.CreateTime, item.UpdateTime,
	)
}

func channelRows(item domain.Channel) *sqlmock.Rows {
	now := time.Date(2026, 7, 26, 10, 0, 0, 0, time.UTC)
	return sqlmock.NewRows([]string{
		"id", "channelCode", "channelName", "channelType", "scopeId", "status", "priority", "configJson", "secretCiphertext", "secretEdek", "secretWrapKeyRef", "rateLimitJson", "metadataJson", "creatorId", "updaterId", "createTime", "updateTime", "isDeleted",
	}).AddRow(item.ID, item.ChannelCode, item.ChannelName, item.ChannelType, item.ScopeID, item.Status, item.Priority, item.ConfigJSON, item.SecretCiphertext, item.SecretEDEK, item.SecretWrapKeyRef, item.RateLimitJSON, item.MetadataJSON, item.CreatorID, item.UpdaterID, now, now, item.IsDeleted)
}

func templateRows(item domain.Template) *sqlmock.Rows {
	now := time.Date(2026, 7, 26, 10, 0, 0, 0, time.UTC)
	return sqlmock.NewRows([]string{
		"id", "templateCode", "scopeId", "templateName", "sceneCode", "channelType", "locale", "subjectTemplate", "textTemplate", "htmlTemplate", "markdownTemplate", "jsonTemplate", "variablesJson", "status", "version", "creatorId", "updaterId", "createTime", "updateTime", "isDeleted",
	}).AddRow(item.ID, item.TemplateCode, item.ScopeID, item.TemplateName, item.SceneCode, item.ChannelType, item.Locale, item.SubjectTemplate, item.TextTemplate, item.HTMLTemplate, item.MarkdownTemplate, item.JSONTemplate, item.VariablesJSON, item.Status, item.Version, item.CreatorID, item.UpdaterID, now, now, item.IsDeleted)
}

func sceneBindingRows(item domain.SceneBinding) *sqlmock.Rows {
	now := time.Date(2026, 7, 26, 10, 0, 0, 0, time.UTC)
	return sqlmock.NewRows([]string{
		"id", "sceneCode", "scopeId", "sceneName", "channelCode", "templateCode", "enabled", "priority", "maxRetry", "retryIntervalSeconds", "metadataJson", "creatorId", "updaterId", "createTime", "updateTime", "isDeleted",
	}).AddRow(item.ID, item.SceneCode, item.ScopeID, item.SceneName, item.ChannelCode, item.TemplateCode, item.Enabled, item.Priority, item.MaxRetry, item.RetryIntervalSeconds, item.MetadataJSON, item.CreatorID, item.UpdaterID, now, now, item.IsDeleted)
}
