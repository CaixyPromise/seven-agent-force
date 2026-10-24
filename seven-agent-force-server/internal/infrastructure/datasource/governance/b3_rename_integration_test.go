package governance

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	notificationapp "github.com/CaixyPromise/seven-agent/force/internal/app/notification/application"
	notificationdomain "github.com/CaixyPromise/seven-agent/force/internal/app/notification/domain"
	notificationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/notification/facade"
	notificationinfra "github.com/CaixyPromise/seven-agent/force/internal/app/notification/infrastructure"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/store"
	_ "github.com/go-sql-driver/mysql"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/jmoiron/sqlx"
	"github.com/pressly/goose/v3"
)

const (
	dg4B3RenameAcceptanceEnv = "DG4_B3_RENAME_ACCEPTANCE"
	dg4B3TestDialectEnv      = "DG4_B3_TEST_DIALECT"
	dg4B3TestDSNEnv          = "DG4_B3_TEST_DSN"
	dg4B3MigrationVersion    = int64(20260731120000)

	dg4B3FixtureScope          = "dg4-b3"
	dg4B3FixtureUserID         = int64(9202607313001)
	dg4B3FixtureActorID        = int64(9202607313002)
	dg4B3FixtureChannelCode    = "dg4-b3-mock"
	dg4B3FixtureTemplateCode   = "dg4_b3_template"
	dg4B3FixtureSceneCode      = "dg4_b3_scene"
	dg4B3FixtureDirectEvent    = "dg4.b3.direct"
	dg4B3FixtureDeferredEvent  = "dg4.b3.deferred"
	dg4B3FixtureSceneEvent     = "dg4.b3.scene"
	dg4B3FixtureExternalID     = "dg4-b3-external-target"
	dg4B3FixtureDeliveryID     = "dg4-b3-delivery"
	dg4B3FixtureHTTPDeliveryID = "dg4-b3-http-delivery"
)

// b3TableMapping is fixed migration contract data. It is never constructed
// from a caller, request, fixture, or database value.
type b3TableMapping struct {
	legacy string
	target string
}

var b3TableMappings = []b3TableMapping{
	{legacy: "sysNotification", target: "sys_notification"},
	{legacy: "sysNotificationChannel", target: "sys_notification_channel"},
	{legacy: "sysNotificationDelivery", target: "sys_notification_delivery"},
	{legacy: "sysNotificationDeliveryAttempt", target: "sys_notification_delivery_attempt"},
	{legacy: "sysNotificationDeliveryDiagnosticAudit", target: "sys_notification_delivery_diagnostic_audit"},
	{legacy: "sysNotificationDeliveryEphemeralContent", target: "sys_notification_delivery_ephemeral_content"},
	{legacy: "sysNotificationExternalTarget", target: "sys_notification_external_target"},
	{legacy: "sysNotificationHTTPDeliverySnapshot", target: "sys_notification_http_delivery_snapshot"},
	{legacy: "sysNotificationMailbox", target: "sys_notification_mailbox"},
	{legacy: "sysNotificationMaterializationTask", target: "sys_notification_materialization_task"},
	{legacy: "sysNotificationRecipient", target: "sys_notification_recipient"},
	{legacy: "sysNotificationSceneBinding", target: "sys_notification_scene_binding"},
	{legacy: "sysNotificationSceneDefinition", target: "sys_notification_scene_definition"},
	{legacy: "sysNotificationSceneRevision", target: "sys_notification_scene_revision"},
	{legacy: "sysNotificationSceneRevisionAudit", target: "sys_notification_scene_revision_audit"},
	{legacy: "sysNotificationSceneSnapshot", target: "sys_notification_scene_snapshot"},
	{legacy: "sysNotificationTemplate", target: "sys_notification_template"},
	{legacy: "sysNotificationTemplateDefinition", target: "sys_notification_template_definition"},
	{legacy: "sysNotificationTemplateRevision", target: "sys_notification_template_revision"},
	{legacy: "sysNotificationTemplateRevisionAudit", target: "sys_notification_template_revision_audit"},
}

type b3IntegrationProvider struct {
	driver  string
	dialect string
	db      *sql.DB
	sqlxDB  *sqlx.DB
}

func (p *b3IntegrationProvider) Close() error { return p.db.Close() }

// TestB3NotificationRenameAcceptance is intentionally a controlled restart
// sequence. "before" runs from the B2-stage source snapshot: B1/B2 names are
// renamed and B3 names remain legacy. "migrate" applies only B3. "after" uses
// the fully renamed source tree to read prior records, update mutable records,
// and create new records. "forward" proves the deliberate rejected-Down
// recovery contract without altering the schema.
func TestB3NotificationRenameAcceptance(t *testing.T) {
	mode := strings.ToLower(strings.TrimSpace(os.Getenv(dg4B3RenameAcceptanceEnv)))
	if mode != "before" && mode != "migrate" && mode != "after" && mode != "forward" {
		t.Skip("set DG4_B3_RENAME_ACCEPTANCE=before|migrate|after|forward with the exact isolated database")
	}
	dialect := strings.ToLower(strings.TrimSpace(os.Getenv(dg4B3TestDialectEnv)))
	dsn := strings.TrimSpace(os.Getenv(dg4B3TestDSNEnv))
	if dialect == "" || dsn == "" {
		t.Skip("set DG4_B3_TEST_DIALECT and DG4_B3_TEST_DSN for the exact isolated database")
	}
	if dialect != "mysql" && dialect != "postgres" {
		t.Fatalf("unsupported B3 test dialect %q", dialect)
	}
	driver := "mysql"
	if dialect == "postgres" {
		driver = "pgx"
	}
	db, err := sql.Open(driver, dsn)
	if err != nil {
		t.Fatalf("open B3 %s database: %v", dialect, err)
	}
	if err := AssertConnectedDatabase(context.Background(), db, dialect); err != nil {
		_ = db.Close()
		t.Fatal(err)
	}
	provider := &b3IntegrationProvider{driver: driver, dialect: dialect, db: db, sqlxDB: sqlx.NewDb(db, driver)}
	t.Cleanup(func() { _ = provider.Close() })

	version := b1GooseVersion(t, context.Background(), provider.db)
	if mode == "before" && version != dg4B2MigrationVersion {
		t.Fatalf("B3 pre-rename version=%d, want=%d", version, dg4B2MigrationVersion)
	}
	if mode == "migrate" {
		if version != dg4B2MigrationVersion {
			t.Fatalf("B3 migration start version=%d, want=%d", version, dg4B2MigrationVersion)
		}
		b3ApplyMigration(t, context.Background(), provider.db, dialect)
		if version = b1GooseVersion(t, context.Background(), provider.db); version != dg4B3MigrationVersion {
			t.Fatalf("B3 migration finish version=%d, want=%d", version, dg4B3MigrationVersion)
		}
		return
	}
	if mode == "forward" {
		if version != dg4B3MigrationVersion {
			t.Fatalf("B3 forward-recovery version=%d, want=%d", version, dg4B3MigrationVersion)
		}
		b3AssertForwardOnlyDownRejected(t, context.Background(), provider.db, dialect)
		return
	}
	if mode == "after" && version < dg4B3MigrationVersion {
		t.Fatalf("B3 post-rename version=%d, require at least %d", version, dg4B3MigrationVersion)
	}

	repo := notificationinfra.NewRepository(provider.sqlxDB)
	service := notificationapp.NewService(store.NewSQLXTransactor(provider.sqlxDB), repo, notificationdomain.NewService(), nil, nil, nil, nil, nil)
	service.SetScopeID(dg4B3FixtureScope)
	ctx := context.Background()
	if mode == "before" {
		b3CreateFixture(t, ctx, provider, repo, service)
		b3AssertFixture(t, ctx, provider, repo, service, true)
		b3AssertTableState(t, ctx, provider.db, dialect, true)
		dg4CapturePhysicalSignatures(t, ctx, provider.db, dialect, "b3", dg4B3PhysicalMappings())
		return
	}

	b3AssertFixture(t, ctx, provider, repo, service, false)
	dg4AssertPhysicalSignatures(t, ctx, provider.db, dialect, "b3", dg4B3PhysicalMappings())
	b3UpdateAndCreateAfterRename(t, ctx, provider, repo, service)
	b3AssertFixture(t, ctx, provider, repo, service, false)
	b3AssertTableState(t, ctx, provider.db, dialect, false)
	dg4DropPhysicalSignatures(t, ctx, provider.db)
}

func b3AssertForwardOnlyDownRejected(t *testing.T, ctx context.Context, db *sql.DB, dialect string) {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve B3 repository root for rejected Down: %v", err)
	}
	dir := filepath.Join(root, "migrations", dialect)
	goose.SetTableName(goose.DefaultTablename)
	if err := goose.SetDialect(dialect); err != nil {
		t.Fatalf("set B3 %s goose dialect for rejected Down: %v", dialect, err)
	}
	versionBefore := b1GooseVersion(t, ctx, db)
	err = goose.DownContext(ctx, db, dir)
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "forward-only") {
		t.Fatalf("B3 rejected Down error=%v, want forward-only failure", err)
	}
	if versionAfter := b1GooseVersion(t, ctx, db); versionAfter != versionBefore {
		t.Fatalf("B3 rejected Down changed version from %d to %d", versionBefore, versionAfter)
	}
}

func b3ApplyMigration(t *testing.T, ctx context.Context, db *sql.DB, dialect string) {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", "..", ".."))
	if err != nil {
		t.Fatalf("resolve B3 repository root: %v", err)
	}
	dir := filepath.Join(root, "migrations", dialect)
	goose.SetTableName(goose.DefaultTablename)
	if err := goose.SetDialect(dialect); err != nil {
		t.Fatalf("set B3 %s goose dialect: %v", dialect, err)
	}
	if err := goose.UpToContext(ctx, db, dir, dg4B3MigrationVersion); err != nil {
		t.Fatalf("apply B3 %s in-place notification table rename: %v", dialect, err)
	}
}

func b3CreateFixture(t *testing.T, ctx context.Context, provider *b3IntegrationProvider, repo *notificationinfra.Repository, service *notificationapp.Service) {
	t.Helper()
	legacyTemplate, err := repo.FindTemplateByCode(ctx, "challenge_otp_mock_zh_cn")
	if err != nil || legacyTemplate == nil {
		t.Fatalf("read seeded legacy notification template: template=%#v err=%v", legacyTemplate, err)
	}
	legacyBinding, err := repo.FindActiveSceneBinding(ctx, "local", notificationdomain.SceneChallengeOTP)
	if err != nil || legacyBinding == nil {
		t.Fatalf("read seeded legacy notification scene binding: binding=%#v err=%v", legacyBinding, err)
	}
	// The pre-rename run can be resumed after a diagnostic assertion failure
	// without mutating a record that has deliberately gained an external-target
	// snapshot later in this fixture. A fresh isolated database always starts
	// here with no direct notification.
	existing, err := repo.FindLogicalNotificationByIdempotency(ctx, dg4B3FixtureScope, dg4B3FixtureDirectEvent, "dg4-b3-direct-before")
	if err != nil {
		t.Fatalf("check B3 fixture resume state: %v", err)
	}
	if existing != nil {
		return
	}

	base := time.Now().UnixNano()
	if err := repo.UpsertChannel(ctx, &notificationdomain.Channel{
		ID:          base,
		ChannelCode: dg4B3FixtureChannelCode,
		ChannelName: "DG4 B3 旧通知连接",
		ChannelType: notificationdomain.ChannelTypeMock,
		ScopeID:     dg4B3FixtureScope,
		Status:      notificationdomain.ChannelStatusEnabled,
		Priority:    10,
		ConfigJSON:  `{"capturePrefix":"dg4-b3"}`,
	}); err != nil {
		t.Fatalf("create B3 notification channel: %v", err)
	}

	directReceipt, err := service.Publish(ctx, notificationfacade.PublishRequest{
		EventKey:       dg4B3FixtureDirectEvent,
		IdempotencyKey: "dg4-b3-direct-before",
		Audience:       notificationfacade.Audience{UserIDs: []int64{dg4B3FixtureUserID}},
		Category:       "SYSTEM",
		Priority:       "NORMAL",
		Title:          "DG4 B3 旧通知",
		Content:        "通知表原地改名前写入的收件箱记录",
		CreatorID:      dg4B3FixtureActorID,
	})
	if err != nil || strings.TrimSpace(directReceipt.NotificationID) == "" {
		t.Fatalf("create B3 direct notification: receipt=%#v err=%v", directReceipt, err)
	}
	if _, err := service.Publish(ctx, notificationfacade.PublishRequest{
		EventKey:       dg4B3FixtureDeferredEvent,
		IdempotencyKey: "dg4-b3-deferred-before",
		Audience:       notificationfacade.Audience{RoleIDs: []int64{1}},
		Category:       "SYSTEM",
		Priority:       "NORMAL",
		Title:          "DG4 B3 延迟物化",
		Content:        "通知表原地改名前写入的物化任务",
		CreatorID:      dg4B3FixtureActorID,
	}); err != nil {
		t.Fatalf("create B3 materialization task through client: %v", err)
	}

	template, err := service.CreateTemplateDefinition(ctx, notificationfacade.TemplateDefinitionCreateRequest{
		TemplateCode: dg4B3FixtureTemplateCode,
		Draft: notificationfacade.TemplateRevisionDraftInput{
			TemplateName:    "DG4 B3 模板",
			Locale:          "zh-CN",
			SubjectTemplate: "DG4 B3 {{.name}}",
			TextTemplate:    "DG4 B3 原地改名 {{.name}}",
			Variables: []notificationfacade.TemplateRevisionVariable{{
				Name: "name", Type: notificationdomain.TemplateVariableTypeString, Required: true, MaxLength: 64, Classification: notificationdomain.TemplateVariableClassificationPublic,
			}},
		},
	}, dg4B3FixtureActorID)
	if err != nil || template.CurrentDraft == nil {
		t.Fatalf("create B3 template definition: template=%#v err=%v", template, err)
	}
	publishedTemplate, err := service.PublishTemplateRevision(ctx, template.CurrentDraft.ID, notificationfacade.TemplateRevisionPublishRequest{ExpectedVersion: template.CurrentDraft.RevisionVersion}, dg4B3FixtureActorID)
	if err != nil || publishedTemplate.CurrentPublished == nil {
		t.Fatalf("publish B3 template revision: template=%#v err=%v", publishedTemplate, err)
	}
	scene, err := service.CreateSceneDefinition(ctx, notificationfacade.SceneDefinitionCreateRequest{
		SceneCode: dg4B3FixtureSceneCode,
		Draft: notificationfacade.SceneRevisionDraftInput{
			SceneName:          "DG4 B3 站内信场景",
			ReceiverKind:       notificationdomain.SceneReceiverKindInApp,
			TemplateRevisionID: publishedTemplate.CurrentPublished.ID,
			Enabled:            true,
		},
	}, dg4B3FixtureActorID)
	if err != nil || scene.CurrentDraft == nil {
		t.Fatalf("create B3 scene definition: scene=%#v err=%v", scene, err)
	}
	publishedScene, err := service.PublishSceneRevision(ctx, scene.CurrentDraft.ID, notificationfacade.SceneRevisionPublishRequest{ExpectedVersion: scene.CurrentDraft.RevisionVersion}, dg4B3FixtureActorID)
	if err != nil || publishedScene.CurrentPublished == nil {
		t.Fatalf("publish B3 scene revision: scene=%#v err=%v", publishedScene, err)
	}
	if _, err := service.Publish(ctx, notificationfacade.PublishRequest{
		EventKey:          dg4B3FixtureSceneEvent,
		IdempotencyKey:    "dg4-b3-scene-before",
		SceneCode:         dg4B3FixtureSceneCode,
		TemplateVariables: map[string]any{"name": "before"},
		Audience:          notificationfacade.Audience{UserIDs: []int64{dg4B3FixtureUserID}},
		CreatorID:         dg4B3FixtureActorID,
	}); err != nil {
		t.Fatalf("create B3 scene notification: %v", err)
	}

	directNotification, err := repo.FindLogicalNotificationByIdempotency(ctx, dg4B3FixtureScope, dg4B3FixtureDirectEvent, "dg4-b3-direct-before")
	if err != nil || directNotification == nil {
		t.Fatalf("read B3 direct notification before external fixture: notification=%#v err=%v", directNotification, err)
	}
	if err := store.NewSQLXTransactor(provider.sqlxDB).WithinTransaction(ctx, func(txCtx context.Context) error {
		return repo.InsertExternalTargets(txCtx, []notificationdomain.ExternalTarget{{
			ID:                  base + 1,
			ExternalTargetID:    dg4B3FixtureExternalID,
			NotificationID:      directNotification.ID,
			ScopeID:             dg4B3FixtureScope,
			ConnectionRef:       dg4B3FixtureChannelCode,
			ProviderCode:        "DG4_B3",
			IdentityKind:        notificationdomain.ExternalIdentityFeishuOpenID,
			SubjectCiphertext:   "fixture-ciphertext",
			SubjectEDEK:         "fixture-edek",
			SubjectWrapKeyRef:   "fixture-key",
			SubjectDigest:       "fixture-subject-digest",
			SubjectDigestKeyRef: "fixture-digest-key",
			ProviderParamsJSON:  `{}`,
		}})
	}); err != nil {
		t.Fatalf("create B3 external target through repository: %v", err)
	}
	externalTarget, err := repo.FindExternalTargetByID(ctx, base+1)
	if err != nil || externalTarget == nil {
		t.Fatalf("read B3 external target after insert: target=%#v err=%v", externalTarget, err)
	}
	if err := repo.InsertDelivery(ctx, &notificationdomain.Delivery{
		ID:               base + 2,
		DeliveryID:       dg4B3FixtureDeliveryID,
		RequestDigest:    "dg4-b3-delivery-digest",
		NotificationID:   &directNotification.ID,
		ExternalTargetID: &externalTarget.ID,
		SceneCode:        "DG4_B3",
		ChannelCode:      dg4B3FixtureChannelCode,
		ChannelType:      notificationdomain.ChannelTypeMock,
		TemplateCode:     dg4B3FixtureTemplateCode,
		TargetMasked:     "masked",
		PayloadJSON:      `{}`,
		RenderedSubject:  "DG4 B3 delivery",
		RenderedText:     "delivery before rename",
		ContentTier:      notificationdomain.DeliveryContentTierPublic,
		Status:           notificationdomain.DeliveryStatusPending,
		MaxRetry:         1,
		CreatorID:        b3Int64Ptr(dg4B3FixtureActorID),
	}); err != nil {
		t.Fatalf("create B3 delivery: %v", err)
	}
	if err := repo.InsertDeliveryAttempt(ctx, &notificationdomain.DeliveryAttempt{
		ID:           base + 3,
		AttemptID:    "dg4-b3-attempt-before",
		DeliveryID:   dg4B3FixtureDeliveryID,
		AttemptNo:    1,
		Status:       notificationdomain.DeliveryStatusFailed,
		FailureClass: "FIXTURE",
		Diagnostic:   "fixture",
	}); err != nil {
		t.Fatalf("create B3 delivery attempt: %v", err)
	}
	if err := repo.InsertDelivery(ctx, &notificationdomain.Delivery{
		ID:              base + 4,
		DeliveryID:      dg4B3FixtureHTTPDeliveryID,
		RequestDigest:   "dg4-b3-http-digest",
		NotificationID:  &directNotification.ID,
		SceneCode:       "DG4_B3_HTTP",
		ChannelCode:     "dg4-b3-http",
		ChannelType:     notificationdomain.ChannelTypeHTTPConnector,
		TemplateCode:    dg4B3FixtureTemplateCode,
		TargetMasked:    "controlled",
		PayloadJSON:     `{}`,
		RenderedSubject: "DG4 B3 HTTP",
		RenderedText:    "http snapshot before rename",
		ContentTier:     notificationdomain.DeliveryContentTierPublic,
		Status:          notificationdomain.DeliveryStatusPending,
		MaxRetry:        1,
		CreatorID:       b3Int64Ptr(dg4B3FixtureActorID),
	}); err != nil {
		t.Fatalf("create B3 HTTP delivery: %v", err)
	}
	if err := repo.InsertHTTPDeliverySnapshot(ctx, &notificationdomain.HTTPDeliverySnapshot{
		ID:              base + 5,
		DeliveryID:      dg4B3FixtureHTTPDeliveryID,
		ScopeID:         dg4B3FixtureScope,
		ChannelCode:     "dg4-b3-http",
		ChannelType:     notificationdomain.ChannelTypeHTTPConnector,
		ChannelPriority: 10,
		ConfigJSON:      `{"method":"POST"}`,
	}); err != nil {
		t.Fatalf("create B3 HTTP delivery snapshot: %v", err)
	}
	if err := repo.InsertDeliveryEphemeralContent(ctx, &notificationdomain.DeliveryEphemeralContent{
		ID:         base + 6,
		DeliveryID: dg4B3FixtureDeliveryID,
		ScopeID:    dg4B3FixtureScope,
		Ciphertext: "fixture-ephemeral-ciphertext",
		EDEK:       "fixture-ephemeral-edek",
		WrapKeyRef: "fixture-ephemeral-key",
		ExpiresAt:  time.Now().UTC().Add(time.Hour),
	}); err != nil {
		t.Fatalf("create B3 delivery ephemeral content: %v", err)
	}
	if err := repo.InsertDeliveryDiagnosticAudit(ctx, &notificationdomain.DeliveryDiagnosticAudit{
		ID:          base + 7,
		ScopeID:     dg4B3FixtureScope,
		DeliveryID:  dg4B3FixtureDeliveryID,
		ActorID:     dg4B3FixtureActorID,
		ContentTier: notificationdomain.DeliveryContentTierPublic,
		ReasonCode:  notificationdomain.DeliveryDiagnosticReasonIncident,
		ResultCode:  notificationdomain.DeliveryDiagnosticResultAllowed,
		TraceID:     "dg4-b3-diagnostic-before",
	}); err != nil {
		t.Fatalf("create B3 delivery diagnostic audit: %v", err)
	}
}

func b3AssertFixture(t *testing.T, ctx context.Context, provider *b3IntegrationProvider, repo *notificationinfra.Repository, service *notificationapp.Service, legacy bool) {
	t.Helper()
	if item, err := repo.FindTemplateByCode(ctx, "challenge_otp_mock_zh_cn"); err != nil || item == nil {
		t.Fatalf("read B3 legacy template: template=%#v err=%v", item, err)
	}
	if item, err := repo.FindActiveSceneBinding(ctx, "local", notificationdomain.SceneChallengeOTP); err != nil || item == nil {
		t.Fatalf("read B3 legacy scene binding: binding=%#v err=%v", item, err)
	}
	channel, err := repo.FindChannelByCode(ctx, dg4B3FixtureChannelCode)
	if err != nil || channel == nil || channel.ScopeID != dg4B3FixtureScope {
		t.Fatalf("read B3 channel: channel=%#v err=%v", channel, err)
	}
	direct, err := repo.FindLogicalNotificationByIdempotency(ctx, dg4B3FixtureScope, dg4B3FixtureDirectEvent, "dg4-b3-direct-before")
	if err != nil || direct == nil || direct.Title != "DG4 B3 旧通知" {
		t.Fatalf("read B3 direct notification: notification=%#v err=%v", direct, err)
	}
	deferred, err := repo.FindLogicalNotificationByIdempotency(ctx, dg4B3FixtureScope, dg4B3FixtureDeferredEvent, "dg4-b3-deferred-before")
	if err != nil || deferred == nil {
		t.Fatalf("read B3 deferred notification: notification=%#v err=%v", deferred, err)
	}
	task, err := repo.FindMaterializationTaskByNotificationID(ctx, deferred.ID)
	if err != nil || task == nil || task.ScopeID != dg4B3FixtureScope {
		t.Fatalf("read B3 materialization task: task=%#v err=%v", task, err)
	}
	count, err := repo.CountUnreadInboxRecipients(ctx, dg4B3FixtureScope, dg4B3FixtureUserID)
	if err != nil || count < 2 {
		t.Fatalf("read B3 mailbox recipients: count=%d err=%v", count, err)
	}
	template, err := service.GetTemplateDefinition(ctx, dg4B3FixtureTemplateCode)
	if err != nil || template == nil || template.CurrentPublished == nil {
		t.Fatalf("read B3 template revision: template=%#v err=%v", template, err)
	}
	scene, err := service.GetSceneDefinition(ctx, dg4B3FixtureSceneCode, notificationdomain.SceneReceiverKindInApp)
	if err != nil || scene == nil || scene.CurrentPublished == nil {
		t.Fatalf("read B3 scene revision: scene=%#v err=%v", scene, err)
	}
	sceneNotification, err := repo.FindLogicalNotificationByIdempotency(ctx, dg4B3FixtureScope, dg4B3FixtureSceneEvent, "dg4-b3-scene-before")
	if err != nil || sceneNotification == nil {
		t.Fatalf("read B3 scene notification: notification=%#v err=%v", sceneNotification, err)
	}
	snapshots, err := repo.ListSceneSnapshotsByNotificationID(ctx, sceneNotification.ID)
	if err != nil || len(snapshots) == 0 {
		t.Fatalf("read B3 scene snapshot: snapshots=%#v err=%v", snapshots, err)
	}
	delivery, err := repo.FindDeliveryByID(ctx, dg4B3FixtureDeliveryID)
	if err != nil || delivery == nil || delivery.NotificationID == nil {
		t.Fatalf("read B3 delivery: delivery=%#v err=%v", delivery, err)
	}
	targets, err := repo.ListExternalTargetsByNotificationID(ctx, direct.ID)
	if err != nil || len(targets) == 0 || targets[0].ExternalTargetID != dg4B3FixtureExternalID {
		t.Fatalf("read B3 external target: targets=%#v err=%v", targets, err)
	}
	httpSnapshot, err := repo.FindHTTPDeliverySnapshotByDeliveryID(ctx, dg4B3FixtureHTTPDeliveryID)
	if err != nil || httpSnapshot == nil || httpSnapshot.ScopeID != dg4B3FixtureScope {
		t.Fatalf("read B3 HTTP snapshot: snapshot=%#v err=%v", httpSnapshot, err)
	}
	ephemeral, err := repo.FindDeliveryEphemeralContent(ctx, dg4B3FixtureScope, dg4B3FixtureDeliveryID)
	if err != nil || ephemeral == nil {
		t.Fatalf("read B3 delivery ephemeral content: item=%#v err=%v", ephemeral, err)
	}
	if err := b3AssertDiagnosticAudit(ctx, provider.sqlxDB, provider.dialect, legacy); err != nil {
		t.Fatalf("read B3 diagnostic audit record: %v", err)
	}
}

func b3UpdateAndCreateAfterRename(t *testing.T, ctx context.Context, provider *b3IntegrationProvider, repo *notificationinfra.Repository, service *notificationapp.Service) {
	t.Helper()
	channel, err := repo.FindChannelByCode(ctx, dg4B3FixtureChannelCode)
	if err != nil || channel == nil {
		t.Fatalf("read B3 channel before update: channel=%#v err=%v", channel, err)
	}
	channel.ChannelName = "DG4 B3 改名后通知连接"
	if err := repo.UpsertChannel(ctx, channel); err != nil {
		t.Fatalf("update B3 channel after rename: %v", err)
	}
	if err := repo.MarkDeliveryProviderAccepted(ctx, dg4B3FixtureDeliveryID, "dg4-b3-provider-after", time.Now().UTC()); err != nil {
		t.Fatalf("update B3 delivery after rename: %v", err)
	}

	template, err := service.CreateTemplateDraftFromPublished(ctx, dg4B3FixtureTemplateCode, dg4B3FixtureActorID)
	if err != nil || template.CurrentDraft == nil {
		t.Fatalf("create B3 next template draft: template=%#v err=%v", template, err)
	}
	updatedTemplate, err := service.SaveTemplateRevisionDraft(ctx, template.CurrentDraft.ID, notificationfacade.TemplateRevisionSaveRequest{
		ExpectedVersion: template.CurrentDraft.RevisionVersion,
		Draft: notificationfacade.TemplateRevisionDraftInput{
			TemplateName:    "DG4 B3 模板新版本",
			Locale:          "zh-CN",
			SubjectTemplate: "DG4 B3 after {{.name}}",
			TextTemplate:    "DG4 B3 改名后 {{.name}}",
			Variables: []notificationfacade.TemplateRevisionVariable{{
				Name: "name", Type: notificationdomain.TemplateVariableTypeString, Required: true, MaxLength: 64, Classification: notificationdomain.TemplateVariableClassificationPublic,
			}},
		},
	}, dg4B3FixtureActorID)
	if err != nil || updatedTemplate.CurrentDraft == nil {
		t.Fatalf("update B3 template draft: template=%#v err=%v", updatedTemplate, err)
	}
	publishedTemplate, err := service.PublishTemplateRevision(ctx, updatedTemplate.CurrentDraft.ID, notificationfacade.TemplateRevisionPublishRequest{ExpectedVersion: updatedTemplate.CurrentDraft.RevisionVersion}, dg4B3FixtureActorID)
	if err != nil || publishedTemplate.CurrentPublished == nil {
		t.Fatalf("publish B3 next template revision: template=%#v err=%v", publishedTemplate, err)
	}

	scene, err := service.CreateSceneDraftFromPublished(ctx, dg4B3FixtureSceneCode, notificationdomain.SceneReceiverKindInApp, dg4B3FixtureActorID)
	if err != nil || scene.CurrentDraft == nil {
		t.Fatalf("create B3 next scene draft: scene=%#v err=%v", scene, err)
	}
	updatedScene, err := service.SaveSceneRevisionDraft(ctx, scene.CurrentDraft.ID, notificationfacade.SceneRevisionSaveRequest{
		ExpectedVersion: scene.CurrentDraft.RevisionVersion,
		Draft: notificationfacade.SceneRevisionDraftInput{
			SceneName:          "DG4 B3 站内信场景新版本",
			ReceiverKind:       notificationdomain.SceneReceiverKindInApp,
			TemplateRevisionID: publishedTemplate.CurrentPublished.ID,
			Enabled:            true,
		},
	}, dg4B3FixtureActorID)
	if err != nil || updatedScene.CurrentDraft == nil {
		t.Fatalf("update B3 scene draft: scene=%#v err=%v", updatedScene, err)
	}
	if _, err := service.PublishSceneRevision(ctx, updatedScene.CurrentDraft.ID, notificationfacade.SceneRevisionPublishRequest{ExpectedVersion: updatedScene.CurrentDraft.RevisionVersion}, dg4B3FixtureActorID); err != nil {
		t.Fatalf("publish B3 next scene revision: %v", err)
	}
	if _, err := service.Publish(ctx, notificationfacade.PublishRequest{
		EventKey:          dg4B3FixtureSceneEvent,
		IdempotencyKey:    fmt.Sprintf("dg4-b3-scene-after-%d", time.Now().UnixNano()),
		SceneCode:         dg4B3FixtureSceneCode,
		TemplateVariables: map[string]any{"name": "after"},
		Audience:          notificationfacade.Audience{UserIDs: []int64{dg4B3FixtureUserID}},
		CreatorID:         dg4B3FixtureActorID,
	}); err != nil {
		t.Fatalf("create B3 scene notification after rename: %v", err)
	}

	base := time.Now().UnixNano()
	direct, err := repo.FindLogicalNotificationByIdempotency(ctx, dg4B3FixtureScope, dg4B3FixtureDirectEvent, "dg4-b3-direct-before")
	if err != nil || direct == nil {
		t.Fatalf("read B3 direct notification before post-rename inserts: notification=%#v err=%v", direct, err)
	}
	if err := repo.InsertDelivery(ctx, &notificationdomain.Delivery{
		ID:              base,
		DeliveryID:      fmt.Sprintf("dg4-b3-delivery-after-%d", base),
		RequestDigest:   fmt.Sprintf("dg4-b3-digest-after-%d", base),
		NotificationID:  &direct.ID,
		SceneCode:       "DG4_B3",
		ChannelCode:     dg4B3FixtureChannelCode,
		ChannelType:     notificationdomain.ChannelTypeMock,
		TemplateCode:    dg4B3FixtureTemplateCode,
		TargetMasked:    "masked-after",
		PayloadJSON:     `{}`,
		RenderedSubject: "DG4 B3 delivery after",
		RenderedText:    "delivery created after rename",
		ContentTier:     notificationdomain.DeliveryContentTierPublic,
		Status:          notificationdomain.DeliveryStatusPending,
		MaxRetry:        1,
		CreatorID:       b3Int64Ptr(dg4B3FixtureActorID),
	}); err != nil {
		t.Fatalf("create B3 delivery after rename: %v", err)
	}
	if err := repo.InsertDeliveryDiagnosticAudit(ctx, &notificationdomain.DeliveryDiagnosticAudit{
		ID:          base + 1,
		ScopeID:     dg4B3FixtureScope,
		DeliveryID:  dg4B3FixtureDeliveryID,
		ActorID:     dg4B3FixtureActorID,
		ContentTier: notificationdomain.DeliveryContentTierPublic,
		ReasonCode:  notificationdomain.DeliveryDiagnosticReasonIncident,
		ResultCode:  notificationdomain.DeliveryDiagnosticResultAllowed,
		TraceID:     "dg4-b3-diagnostic-after",
	}); err != nil {
		t.Fatalf("create B3 diagnostic audit after rename: %v", err)
	}
	_ = provider
}

func b3AssertDiagnosticAudit(ctx context.Context, db *sqlx.DB, dialect string, legacy bool) error {
	var count int
	query := `SELECT COUNT(*) FROM ` + b3PhysicalTable(dialect, legacy, "sysNotificationDeliveryDiagnosticAudit") + ` WHERE ` + b3Column(dialect, "scopeId") + `=? AND ` + b3Column(dialect, "deliveryId") + `=?`
	if err := db.GetContext(ctx, &count, db.Rebind(query), dg4B3FixtureScope, dg4B3FixtureDeliveryID); err != nil {
		return err
	}
	if count == 0 {
		return fmt.Errorf("no diagnostic audit fixture row")
	}
	return nil
}

func b3Column(dialect, name string) string {
	if dialect == "postgres" {
		return `"` + name + `"`
	}
	return name
}

func b3Int64Ptr(value int64) *int64 {
	return &value
}

func b3AssertTableState(t *testing.T, ctx context.Context, db *sql.DB, dialect string, legacy bool) {
	t.Helper()
	for _, mapping := range b3TableMappings {
		legacyExists := b1TableExists(t, ctx, db, dialect, mapping.legacy)
		targetExists := b1TableExists(t, ctx, db, dialect, mapping.target)
		if legacy {
			if !legacyExists || targetExists {
				t.Fatalf("B3 pre-rename table state %s -> %s: legacy=%t target=%t", mapping.legacy, mapping.target, legacyExists, targetExists)
			}
			continue
		}
		if legacyExists || !targetExists {
			t.Fatalf("B3 post-rename table state %s -> %s: legacy=%t target=%t", mapping.legacy, mapping.target, legacyExists, targetExists)
		}
		var rows int64
		if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+mapping.target).Scan(&rows); err != nil {
			t.Fatalf("count B3 target table %s: %v", mapping.target, err)
		}
		if rows == 0 {
			t.Fatalf("B3 target table %s lost all rows", mapping.target)
		}
	}
}

func b3PhysicalTable(dialect string, legacy bool, legacyName string) string {
	for _, mapping := range b3TableMappings {
		if mapping.legacy != legacyName {
			continue
		}
		if legacy {
			if dialect == "postgres" {
				return `"` + mapping.legacy + `"`
			}
			return mapping.legacy
		}
		return mapping.target
	}
	panic("unknown fixed B3 table " + legacyName)
}
