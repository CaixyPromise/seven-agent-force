//go:build integration

package notification

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	notificationapp "github.com/CaixyPromise/seven-agent/force/internal/app/notification/application"
	notificationdomain "github.com/CaixyPromise/seven-agent/force/internal/app/notification/domain"
	notificationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/notification/facade"
	notificationinfra "github.com/CaixyPromise/seven-agent/force/internal/app/notification/infrastructure"
	apperrors "github.com/CaixyPromise/seven-agent/force/internal/common/errors"
	mysqldatasource "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/mysql"
	postgresdatasource "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/postgres"
	dbstore "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/store"
	secretvalueinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/crypto/secretvalue"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/outboundurl"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/xid"
	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
)

const (
	g62SceneIntegrationEnv = "SEVEN_G62_SCENE_INTEGRATION"
	g62MySQLDSNEnv         = "SEVEN_G62_MYSQL_DSN"
	g62PostgresDSNEnv      = "SEVEN_G62_POSTGRES_DSN"
	g62IsolatedDatabase    = "seven_notification_g62"
)

// TestG62SceneRevisionMySQLIntegration exercises the real MySQL repository,
// transaction boundary and bounded local Outbox fallback. The receiver is an
// in-memory controlled driver; no real provider credential or HTTP request is
// used.
func TestG62SceneRevisionMySQLIntegration(t *testing.T) {
	if strings.TrimSpace(os.Getenv(g62SceneIntegrationEnv)) != "1" {
		t.Skip("set SEVEN_G62_SCENE_INTEGRATION=1 to run the isolated G6.2 probes")
	}
	dsn := strings.TrimSpace(os.Getenv(g62MySQLDSNEnv))
	if dsn == "" {
		t.Skip("set SEVEN_G62_MYSQL_DSN to the isolated G6.2 MySQL database")
	}
	if err := g62ValidateMySQLDSN(dsn); err != nil {
		t.Fatalf("validate isolated MySQL database: %v", err)
	}
	provider, err := mysqldatasource.NewProvider(config.MySQLConfig{
		Enabled: true, DSN: dsn, MaxOpenConns: 8, MaxIdleConns: 2,
	}, zap.NewNop())
	if err != nil {
		t.Fatalf("open isolated MySQL database: %v", err)
	}
	t.Cleanup(func() { _ = provider.Close() })
	g62RunSceneRevisionIntegration(t, "mysql", provider.SQLX(), provider.Transactor())
}

// TestG62SceneRevisionPostgresIntegration proves the same acceptance and
// delivery chain against PostgreSQL's quoted camel-case schema.
func TestG62SceneRevisionPostgresIntegration(t *testing.T) {
	if strings.TrimSpace(os.Getenv(g62SceneIntegrationEnv)) != "1" {
		t.Skip("set SEVEN_G62_SCENE_INTEGRATION=1 to run the isolated G6.2 probes")
	}
	dsn := strings.TrimSpace(os.Getenv(g62PostgresDSNEnv))
	if dsn == "" {
		t.Skip("set SEVEN_G62_POSTGRES_DSN to the isolated G6.2 PostgreSQL database")
	}
	if err := g62ValidatePostgresDSN(dsn); err != nil {
		t.Fatalf("validate isolated PostgreSQL database: %v", err)
	}
	provider, err := postgresdatasource.NewProvider(config.PostgresConfig{
		Enabled: true, DSN: dsn, MaxOpenConns: 8, MaxIdleConns: 2,
	}, zap.NewNop())
	if err != nil {
		t.Fatalf("open isolated PostgreSQL database: %v", err)
	}
	t.Cleanup(func() { _ = provider.Close() })
	g62RunSceneRevisionIntegration(t, "postgres", provider.SQLX(), provider.Transactor())
}

func g62RunSceneRevisionIntegration(t *testing.T, dialect string, db *sqlx.DB, tx dbstore.Transactor) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	if err := g62AssertSceneSchema(ctx, db, dialect); err != nil {
		t.Fatalf("assert G6.2 migration schema: %v", err)
	}

	unique := fmt.Sprintf("%d", time.Now().UnixNano())
	scopeID := "g62-" + dialect + "-" + unique
	sceneCode := "g62_scene_" + unique
	channelCode := "g62-controlled-" + unique
	templateCode := "g62-template-" + unique

	if err := g62CleanupScope(ctx, db, dialect, scopeID, channelCode); err != nil {
		t.Fatalf("clear prior G6.2 fixture: %v", err)
	}
	t.Cleanup(func() { _ = g62CleanupScope(context.Background(), db, dialect, scopeID, channelCode) })

	driver := &g62ControlledDriver{}
	idGen, err := xid.New(62)
	if err != nil {
		t.Fatalf("new G6.2 ID generator: %v", err)
	}
	repo := notificationinfra.NewRepository(db)
	// Enterprise application connections remain URL-capable configurations, so
	// keep the real SSRF validator in this test composition. The controlled
	// driver never creates an HTTP request and the Feishu app-id-only config
	// contains no arbitrary endpoint to resolve.
	urls := notificationinfra.NewChannelURLValidator(outboundurl.NewOutboundURLGuard(outboundurl.Options{}))
	service := notificationapp.NewService(
		tx,
		repo,
		notificationdomain.NewService(),
		g62SecretService{},
		g62DriverRegistry{driver: driver},
		urls,
		nil,
		idGen,
	)
	service.SetScopeID(scopeID)
	service.BindExternalTargetDigester(g62ExternalTargetDigester{})

	channel, err := service.UpsertChannel(ctx, notificationfacade.ChannelUpsertRequest{
		ChannelCode: channelCode,
		ChannelName: "G6.2 受控飞书应用",
		ChannelType: notificationdomain.ChannelTypeFeishuApp,
		Status:      notificationdomain.ChannelStatusEnabled,
		Priority:    100,
		ProviderConfig: &notificationfacade.ProviderChannelConfig{
			FeishuAppID: "cli_g62_controlled",
		},
		// This fixed test-only value is encrypted by g62SecretService and
		// never leaves the process. It is not a tenant credential.
		SecretPlain: "g62-controlled-secret",
	}, 6201)
	if err != nil {
		t.Fatalf("create controlled connection: %v", err)
	}
	if channel == nil || channel.ChannelCode != channelCode || !channel.SecretConfigured {
		t.Fatalf("unexpected controlled connection: %#v", channel)
	}

	createdTemplate, err := service.CreateTemplateDefinition(ctx, notificationfacade.TemplateDefinitionCreateRequest{
		TemplateCode: templateCode,
		Draft:        g62TemplateDraft("G6.2 原始模板", "原始正文 {{.name}}"),
	}, 6202)
	if err != nil {
		t.Fatalf("create versioned template: %v", err)
	}
	publishedTemplate, err := service.PublishTemplateRevision(ctx, createdTemplate.CurrentDraft.ID, notificationfacade.TemplateRevisionPublishRequest{
		ExpectedVersion: createdTemplate.CurrentDraft.RevisionVersion,
	}, 6203)
	if err != nil {
		t.Fatalf("publish versioned template: %v", err)
	}
	if publishedTemplate.CurrentPublished == nil {
		t.Fatal("published template is missing")
	}

	createdScene, err := service.CreateSceneDefinition(ctx, notificationfacade.SceneDefinitionCreateRequest{
		SceneCode: sceneCode,
		Draft: notificationfacade.SceneRevisionDraftInput{
			SceneName:          "G6.2 飞书成员通知",
			ReceiverKind:       notificationdomain.SceneReceiverKindFeishuOpenID,
			TemplateRevisionID: publishedTemplate.CurrentPublished.ID,
			ConnectionRef:      channelCode,
			Enabled:            true,
		},
	}, 6204)
	if err != nil {
		t.Fatalf("create scene draft: %v", err)
	}
	publishedScene, err := service.PublishSceneRevision(ctx, createdScene.CurrentDraft.ID, notificationfacade.SceneRevisionPublishRequest{
		ExpectedVersion: createdScene.CurrentDraft.RevisionVersion,
	}, 6205)
	if err != nil {
		t.Fatalf("publish scene: %v", err)
	}
	if publishedScene.CurrentPublished == nil || publishedScene.CurrentPublished.ConnectionRef != channelCode {
		t.Fatalf("published scene did not freeze the one sending way: %#v", publishedScene)
	}

	request := notificationfacade.PublishRequest{
		EventKey:          "g62.scene.delivery",
		IdempotencyKey:    "g62-first-" + unique,
		SceneCode:         sceneCode,
		TemplateVariables: map[string]any{"name": "受控成员"},
		ExternalRecipients: []notificationfacade.ExternalRecipient{{
			IdentityKind: notificationfacade.ExternalIdentityFeishuOpenID,
			Subject:      "ou_g62_controlled",
		}},
	}
	first, err := service.Publish(ctx, request)
	if err != nil {
		t.Fatalf("publish through scene client: %v", err)
	}
	if first.Duplicate || strings.TrimSpace(first.NotificationID) == "" {
		t.Fatalf("first receipt=%#v", first)
	}
	notification, err := repo.FindLogicalNotificationByIdempotency(ctx, scopeID, request.EventKey, request.IdempotencyKey)
	if err != nil || notification == nil {
		t.Fatalf("read accepted notification err=%v notification=%#v", err, notification)
	}
	snapshots, err := repo.ListSceneSnapshotsByNotificationID(ctx, notification.ID)
	if err != nil || len(snapshots) != 1 {
		t.Fatalf("scene snapshots err=%v snapshots=%#v", err, snapshots)
	}
	snapshot := snapshots[0]
	if snapshot.SceneRevisionID != publishedScene.CurrentPublished.ID || snapshot.TemplateRevisionID != publishedTemplate.CurrentPublished.ID || snapshot.ConnectionRef != channelCode || snapshot.Resolution != notificationdomain.SceneSnapshotResolutionAccepted {
		t.Fatalf("accepted scene snapshot=%#v", snapshot)
	}
	deliveries, err := repo.ListDeliveriesByNotificationID(ctx, notification.ID)
	if err != nil || len(deliveries) != 1 {
		t.Fatalf("accepted deliveries err=%v deliveries=%#v", err, deliveries)
	}
	if deliveries[0].SceneSnapshotID == nil || *deliveries[0].SceneSnapshotID != snapshot.ID || deliveries[0].RenderedText != "原始正文 受控成员" || deliveries[0].ExternalTargetID == nil {
		t.Fatalf("delivery did not carry the accepted snapshot/rendering: %#v", deliveries[0])
	}
	if err := g62AssertNoExternalInboxState(ctx, db, dialect, scopeID); err != nil {
		t.Fatalf("external delivery polluted inbox state: %v", err)
	}

	// Change the template and scene after acceptance. A same-idempotency retry
	// must reuse the old snapshot rather than resolving today's configuration.
	nextTemplate, err := service.CreateTemplateDraftFromPublished(ctx, templateCode, 6206)
	if err != nil {
		t.Fatalf("create next template draft: %v", err)
	}
	savedTemplate, err := service.SaveTemplateRevisionDraft(ctx, nextTemplate.CurrentDraft.ID, notificationfacade.TemplateRevisionSaveRequest{
		ExpectedVersion: nextTemplate.CurrentDraft.RevisionVersion,
		Draft:           g62TemplateDraft("G6.2 新模板", "新正文 {{.name}}"),
	}, 6207)
	if err != nil {
		t.Fatalf("save next template draft: %v", err)
	}
	nextPublishedTemplate, err := service.PublishTemplateRevision(ctx, savedTemplate.CurrentDraft.ID, notificationfacade.TemplateRevisionPublishRequest{
		ExpectedVersion: savedTemplate.CurrentDraft.RevisionVersion,
	}, 6208)
	if err != nil || nextPublishedTemplate.CurrentPublished == nil {
		t.Fatalf("publish next template err=%v template=%#v", err, nextPublishedTemplate)
	}
	nextScene, err := service.CreateSceneDraftFromPublished(ctx, sceneCode, notificationdomain.SceneReceiverKindFeishuOpenID, 6209)
	if err != nil {
		t.Fatalf("create next scene draft: %v", err)
	}
	savedScene, err := service.SaveSceneRevisionDraft(ctx, nextScene.CurrentDraft.ID, notificationfacade.SceneRevisionSaveRequest{
		ExpectedVersion: nextScene.CurrentDraft.RevisionVersion,
		Draft: notificationfacade.SceneRevisionDraftInput{
			SceneName:          "G6.2 飞书成员通知 v2",
			ReceiverKind:       notificationdomain.SceneReceiverKindFeishuOpenID,
			TemplateRevisionID: nextPublishedTemplate.CurrentPublished.ID,
			ConnectionRef:      channelCode,
			Enabled:            true,
		},
	}, 6210)
	if err != nil {
		t.Fatalf("save next scene draft: %v", err)
	}
	if _, err := service.PublishSceneRevision(ctx, savedScene.CurrentDraft.ID, notificationfacade.SceneRevisionPublishRequest{ExpectedVersion: savedScene.CurrentDraft.RevisionVersion}, 6211); err != nil {
		t.Fatalf("publish next scene: %v", err)
	}

	second, err := service.Publish(ctx, request)
	if err != nil {
		t.Fatalf("idempotent retry after configuration changes: %v", err)
	}
	if !second.Duplicate || second.NotificationID != first.NotificationID {
		t.Fatalf("retry did not reuse accepted notification first=%#v second=%#v", first, second)
	}
	snapshots, err = repo.ListSceneSnapshotsByNotificationID(ctx, notification.ID)
	if err != nil || len(snapshots) != 1 || snapshots[0].ID != snapshot.ID || snapshots[0].TemplateRevisionID != publishedTemplate.CurrentPublished.ID {
		t.Fatalf("retry re-resolved accepted scene snapshots=%#v err=%v", snapshots, err)
	}
	deliveries, err = repo.ListDeliveriesByNotificationID(ctx, notification.ID)
	if err != nil || len(deliveries) != 1 || deliveries[0].RenderedText != "原始正文 受控成员" {
		t.Fatalf("retry changed accepted delivery deliveries=%#v err=%v", deliveries, err)
	}

	// Publish the same next draft concurrently through the real transaction
	// boundary. Exactly one request may advance the immutable current pointer.
	concurrentDraft, err := service.CreateSceneDraftFromPublished(ctx, sceneCode, notificationdomain.SceneReceiverKindFeishuOpenID, 62115)
	if err != nil || concurrentDraft.CurrentDraft == nil {
		t.Fatalf("create concurrent scene draft err=%v scene=%#v", err, concurrentDraft)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	for actorID := int64(62116); actorID <= 62117; actorID++ {
		actorID := actorID
		go func() {
			<-start
			_, publishErr := service.PublishSceneRevision(ctx, concurrentDraft.CurrentDraft.ID, notificationfacade.SceneRevisionPublishRequest{
				ExpectedVersion: concurrentDraft.CurrentDraft.RevisionVersion,
			}, actorID)
			results <- publishErr
		}()
	}
	close(start)
	successfulPublish := 0
	for range 2 {
		if publishErr := <-results; publishErr == nil {
			successfulPublish++
		}
	}
	if successfulPublish != 1 {
		t.Fatalf("concurrent scene publish successes=%d, want exactly one", successfulPublish)
	}

	if err := service.RelayOutbox(ctx, 20); err != nil {
		t.Fatalf("relay first accepted delivery through controlled receiver: %v", err)
	}
	deliveries, err = repo.ListDeliveriesByNotificationID(ctx, notification.ID)
	if err != nil || len(deliveries) != 1 || deliveries[0].Status != notificationdomain.DeliveryStatusProviderAccepted {
		t.Fatalf("controlled delivery status=%#v err=%v", deliveries, err)
	}
	if driver.Count() != 1 || driver.Last().Target != "ou_g62_controlled" || driver.Last().Text != "原始正文 受控成员" {
		t.Fatalf("controlled receiver calls=%d last=%#v", driver.Count(), driver.Last())
	}
	if err := g62AssertDispatchOutboxDone(ctx, db, dialect, scopeID, deliveries[0].DeliveryID); err != nil {
		t.Fatalf("dispatch outbox was not completed: %v", err)
	}

	// Accept a second delivery while the connection is enabled, then apply the
	// emergency connection stop before dispatch. It must not choose another
	// channel or emit a second controlled receiver call.
	gatedRequest := request
	gatedRequest.IdempotencyKey = "g62-connection-gate-" + unique
	gatedRequest.TemplateVariables = map[string]any{"name": "未外发成员"}
	gated, err := service.Publish(ctx, gatedRequest)
	if err != nil || gated.Duplicate {
		t.Fatalf("accept delivery before connection stop receipt=%#v err=%v", gated, err)
	}
	if _, err := service.UpsertChannel(ctx, notificationfacade.ChannelUpsertRequest{
		ChannelCode: channelCode,
		ChannelName: "G6.2 受控飞书应用",
		ChannelType: notificationdomain.ChannelTypeFeishuApp,
		Status:      notificationdomain.ChannelStatusDisabled,
		Priority:    100,
		ProviderConfig: &notificationfacade.ProviderChannelConfig{
			FeishuAppID: "cli_g62_controlled",
		},
	}, 6212); err != nil {
		t.Fatalf("emergency disable controlled connection: %v", err)
	}
	if err := service.RelayOutbox(ctx, 20); err != nil {
		t.Fatalf("relay stopped connection delivery: %v", err)
	}
	gatedNotification, err := repo.FindLogicalNotificationByIdempotency(ctx, scopeID, gatedRequest.EventKey, gatedRequest.IdempotencyKey)
	if err != nil || gatedNotification == nil {
		t.Fatalf("read gated notification err=%v notification=%#v", err, gatedNotification)
	}
	gatedDeliveries, err := repo.ListDeliveriesByNotificationID(ctx, gatedNotification.ID)
	if err != nil || len(gatedDeliveries) != 1 || gatedDeliveries[0].Status != notificationdomain.DeliveryStatusFailed {
		t.Fatalf("stopped connection delivery err=%v deliveries=%#v", err, gatedDeliveries)
	}
	if driver.Count() != 1 {
		t.Fatalf("stopped connection unexpectedly reached controlled receiver calls=%d", driver.Count())
	}

	// A disabled published scene is an explicit refusal, not a silent V1
	// fallback. It also stays independent from the previous accepted snapshot.
	if _, err := service.StopSceneDefinition(ctx, sceneCode, notificationdomain.SceneReceiverKindFeishuOpenID, 6213); err != nil {
		t.Fatalf("stop published scene: %v", err)
	}
	disabledRequest := request
	disabledRequest.IdempotencyKey = "g62-scene-disabled-" + unique
	if _, err := service.Publish(ctx, disabledRequest); g62ReasonCode(err) != "SCENE_DISABLED" {
		t.Fatalf("disabled published scene error=%v reason=%q, want SCENE_DISABLED", err, g62ReasonCode(err))
	}
	if driver.Count() != 1 {
		t.Fatalf("disabled scene unexpectedly reached controlled receiver calls=%d", driver.Count())
	}
	if err := g62AssertNoExternalInboxState(ctx, db, dialect, scopeID); err != nil {
		t.Fatalf("G6.2 external scenes created inbox state: %v", err)
	}

	foreignService := notificationapp.NewService(tx, repo, notificationdomain.NewService(), g62SecretService{}, g62DriverRegistry{driver: driver}, nil, nil, idGen)
	foreignService.SetScopeID(scopeID + "-foreign")
	foreignService.BindExternalTargetDigester(g62ExternalTargetDigester{})
	if _, err := foreignService.GetSceneDefinition(ctx, sceneCode, notificationdomain.SceneReceiverKindFeishuOpenID); err == nil {
		t.Fatal("foreign scope read a G6.2 scene")
	}

	// The same scene identity cannot acquire a second sending way. This is a
	// real storage/repository check, not only a frontend restriction.
	if _, err := service.CreateSceneDefinition(ctx, notificationfacade.SceneDefinitionCreateRequest{
		SceneCode: sceneCode,
		Draft: notificationfacade.SceneRevisionDraftInput{
			SceneName:          "重复发送方式",
			ReceiverKind:       notificationdomain.SceneReceiverKindFeishuOpenID,
			TemplateRevisionID: nextPublishedTemplate.CurrentPublished.ID,
			ConnectionRef:      channelCode,
			Enabled:            true,
		},
	}, 6214); err == nil {
		t.Fatal("one scene receiver kind accepted a second sending way")
	}

	if err := g62AssertSafeSceneAudit(ctx, db, dialect, scopeID); err != nil {
		t.Fatalf("scene audit safety: %v", err)
	}
}

func g62TemplateDraft(name, text string) notificationfacade.TemplateRevisionDraftInput {
	return notificationfacade.TemplateRevisionDraftInput{
		TemplateName:    name,
		Locale:          "zh-CN",
		SubjectTemplate: "通知 {{.name}}",
		TextTemplate:    text,
		Variables: []notificationfacade.TemplateRevisionVariable{{
			Name:           "name",
			Type:           notificationdomain.TemplateVariableTypeString,
			Required:       true,
			MaxLength:      80,
			SampleValue:    "样例成员",
			Classification: notificationdomain.TemplateVariableClassificationPublic,
		}},
	}
}

func g62ReasonCode(err error) string {
	var appErr *apperrors.AppError
	if !errors.As(err, &appErr) || appErr == nil {
		return ""
	}
	details, ok := appErr.Details().(map[string]string)
	if !ok {
		return ""
	}
	return details["reasonCode"]
}

type g62SecretService struct{}

func (g62SecretService) EncryptString(_ context.Context, plain string) (secretvalueinfra.SecretValue, error) {
	return secretvalueinfra.SecretValue{
		CiphertextB64: base64.RawURLEncoding.EncodeToString([]byte(plain)),
		EDEKB64:       "g62-test-edek",
		WrapKeyRef:    "g62-test-key",
	}, nil
}

func (g62SecretService) DecryptString(_ context.Context, value secretvalueinfra.SecretValue) (string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(value.CiphertextB64)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

type g62ExternalTargetDigester struct{}

func (g62ExternalTargetDigester) Digest(_ context.Context, keyRef, scopeID, connectionRef, identityKind, subject string) (string, string, error) {
	if strings.TrimSpace(keyRef) == "" {
		keyRef = "g62-test-digest-key"
	}
	sum := sha256.Sum256([]byte(strings.Join([]string{scopeID, connectionRef, identityKind, subject}, "\x00")))
	return hex.EncodeToString(sum[:]), keyRef, nil
}

type g62DriverRegistry struct{ driver *g62ControlledDriver }

func (r g62DriverRegistry) Driver(channelType string) notificationapp.ChannelDriver {
	if strings.EqualFold(strings.TrimSpace(channelType), notificationdomain.ChannelTypeFeishuApp) {
		return r.driver
	}
	return nil
}

type g62ControlledDriver struct {
	mu       sync.Mutex
	messages []notificationapp.DriverMessage
}

func (d *g62ControlledDriver) Send(ctx context.Context, message notificationapp.DriverMessage) error {
	_, err := d.SendResult(ctx, message)
	return err
}

func (d *g62ControlledDriver) SendResult(_ context.Context, message notificationapp.DriverMessage) (notificationapp.DriverResult, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.messages = append(d.messages, message)
	return notificationapp.DriverResult{
		Status:            notificationapp.DriverResultProviderAccepted,
		ProviderReference: "g62-controlled-receiver",
	}, nil
}

func (d *g62ControlledDriver) Count() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.messages)
}

func (d *g62ControlledDriver) Last() notificationapp.DriverMessage {
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.messages) == 0 {
		return notificationapp.DriverMessage{}
	}
	return d.messages[len(d.messages)-1]
}

func g62AssertSceneSchema(ctx context.Context, db *sqlx.DB, dialect string) error {
	for _, table := range []string{
		"sys_notification_scene_definition",
		"sys_notification_scene_revision",
		"sys_notification_scene_revision_audit",
		"sys_notification_scene_snapshot",
	} {
		var count int
		query := "SELECT COUNT(1) FROM information_schema.tables WHERE table_name=?"
		if dialect == "postgres" {
			query += " AND table_schema=current_schema()"
		} else {
			query += " AND table_schema=DATABASE()"
		}
		if err := db.GetContext(ctx, &count, db.Rebind(query), table); err != nil {
			return err
		}
		if count != 1 {
			return fmt.Errorf("missing table %s", table)
		}
	}
	var sceneSnapshotColumn int
	columnQuery := "SELECT COUNT(1) FROM information_schema.columns WHERE table_name=? AND column_name=?"
	if dialect == "postgres" {
		columnQuery += " AND table_schema=current_schema()"
	} else {
		columnQuery += " AND table_schema=DATABASE()"
	}
	if err := db.GetContext(ctx, &sceneSnapshotColumn, db.Rebind(columnQuery), "sys_notification_delivery", "sceneSnapshotId"); err != nil {
		return err
	}
	if sceneSnapshotColumn != 1 {
		return errors.New("sys_notification_delivery.sceneSnapshotId is missing")
	}
	return nil
}

func g62AssertNoExternalInboxState(ctx context.Context, db *sqlx.DB, dialect, scopeID string) error {
	for _, item := range []struct {
		table string
		query string
	}{
		{
			table: "sys_notification_recipient",
			query: "SELECT COUNT(1) FROM " + g62Table(dialect, "sys_notification_recipient") +
				" WHERE " + g62Column(dialect, "scopeId") + "=?",
		},
		{
			table: "sys_notification_mailbox",
			query: "SELECT COUNT(1) FROM " + g62Table(dialect, "sys_notification_mailbox") +
				" WHERE " + g62Column(dialect, "scopeId") + "=?",
		},
		{
			table: "sys_outbox_event",
			query: "SELECT COUNT(1) FROM " + g62Table(dialect, "sys_outbox_event") +
				" WHERE " + g62Column(dialect, "scopeId") + "=? AND " +
				g62Column(dialect, "eventType") + "='notification.inbox.changed'",
		},
	} {
		var count int64
		if err := db.GetContext(ctx, &count, db.Rebind(item.query), scopeID); err != nil {
			return fmt.Errorf("count %s: %w", item.table, err)
		}
		if count != 0 {
			return fmt.Errorf("%s count=%d, want 0", item.table, count)
		}
	}
	return nil
}

func g62AssertDispatchOutboxDone(ctx context.Context, db *sqlx.DB, dialect, scopeID, deliveryID string) error {
	var status string
	query := "SELECT " + g62Column(dialect, "status") + " FROM " + g62Table(dialect, "sys_outbox_event") +
		" WHERE " + g62Column(dialect, "scopeId") + "=? AND " +
		g62Column(dialect, "eventType") + "=? AND " + g62Column(dialect, "aggregateId") + "=?" +
		" ORDER BY " + g62Column(dialect, "id") + " DESC LIMIT 1"
	if err := db.GetContext(ctx, &status, db.Rebind(query), scopeID, notificationdomain.OutboxEventNotificationDispatch, deliveryID); err != nil {
		return err
	}
	if status != "DONE" {
		return fmt.Errorf("dispatch Outbox status=%q", status)
	}
	return nil
}

func g62AssertSafeSceneAudit(ctx context.Context, db *sqlx.DB, dialect, scopeID string) error {
	var actions []string
	query := "SELECT " + g62Column(dialect, "action") + " FROM " + g62Table(dialect, "sys_notification_scene_revision_audit") + " WHERE " + g62Column(dialect, "scopeId") + "=?"
	if err := db.SelectContext(ctx, &actions, db.Rebind(query), scopeID); err != nil {
		return err
	}
	if len(actions) < 4 {
		return fmt.Errorf("scene audit actions=%d, want at least create/publish/copy/stop", len(actions))
	}
	var columns []string
	columnQuery := "SELECT column_name FROM information_schema.columns WHERE table_name=?"
	if dialect == "postgres" {
		columnQuery += " AND table_schema=current_schema()"
	} else {
		columnQuery += " AND table_schema=DATABASE()"
	}
	if err := db.SelectContext(ctx, &columns, db.Rebind(columnQuery), "sys_notification_scene_revision_audit"); err != nil {
		return err
	}
	for _, column := range columns {
		lower := strings.ToLower(column)
		for _, forbidden := range []string{"body", "content", "variable", "target", "secret", "credential", "provider", "url"} {
			if strings.Contains(lower, forbidden) {
				return fmt.Errorf("scene audit schema contains unsafe column %q", column)
			}
		}
	}
	return nil
}

func g62CleanupScope(ctx context.Context, db *sqlx.DB, dialect, scopeID, channelCode string) error {
	table := func(name string) string { return g62Table(dialect, name) }
	column := func(name string) string { return g62Column(dialect, name) }
	notificationIDs := "SELECT " + column("id") + " FROM " + table("sys_notification") + " WHERE " + column("scopeId") + "=?"
	sceneIDs := "SELECT " + column("id") + " FROM " + table("sys_notification_scene_definition") + " WHERE " + column("scopeId") + "=?"
	templateIDs := "SELECT " + column("id") + " FROM " + table("sys_notification_template_definition") + " WHERE " + column("scopeId") + "=?"
	deliveryIDs := "SELECT " + column("deliveryId") + " FROM " + table("sys_notification_delivery") + " WHERE " + column("notificationId") + " IN (" + notificationIDs + ")"
	operations := []struct {
		query string
		args  []any
	}{
		{"DELETE FROM " + table("sys_notification_delivery_attempt") + " WHERE " + column("deliveryId") + " IN (" + deliveryIDs + ")", []any{scopeID}},
		{"DELETE FROM " + table("sys_notification_delivery") + " WHERE " + column("notificationId") + " IN (" + notificationIDs + ")", []any{scopeID}},
		{"DELETE FROM " + table("sys_notification_external_target") + " WHERE " + column("notificationId") + " IN (" + notificationIDs + ")", []any{scopeID}},
		{"DELETE FROM " + table("sys_notification_scene_snapshot") + " WHERE " + column("scopeId") + "=?", []any{scopeID}},
		{"DELETE FROM " + table("sys_notification_materialization_task") + " WHERE " + column("notificationId") + " IN (" + notificationIDs + ")", []any{scopeID}},
		{"DELETE FROM " + table("sys_notification_recipient") + " WHERE " + column("scopeId") + "=?", []any{scopeID}},
		{"DELETE FROM " + table("sys_notification_mailbox") + " WHERE " + column("scopeId") + "=?", []any{scopeID}},
		{"DELETE FROM " + table("sys_outbox_event") + " WHERE " + column("scopeId") + "=?", []any{scopeID}},
		{"DELETE FROM " + table("sys_notification") + " WHERE " + column("scopeId") + "=?", []any{scopeID}},
		{"DELETE FROM " + table("sys_notification_scene_revision_audit") + " WHERE " + column("scopeId") + "=?", []any{scopeID}},
		{"DELETE FROM " + table("sys_notification_scene_revision") + " WHERE " + column("sceneDefinitionId") + " IN (" + sceneIDs + ")", []any{scopeID}},
		{"DELETE FROM " + table("sys_notification_scene_definition") + " WHERE " + column("scopeId") + "=?", []any{scopeID}},
		{"DELETE FROM " + table("sys_notification_template_revision_audit") + " WHERE " + column("scopeId") + "=?", []any{scopeID}},
		{"DELETE FROM " + table("sys_notification_template_revision") + " WHERE " + column("templateDefinitionId") + " IN (" + templateIDs + ")", []any{scopeID}},
		{"DELETE FROM " + table("sys_notification_template_definition") + " WHERE " + column("scopeId") + "=?", []any{scopeID}},
		{"DELETE FROM " + table("sys_notification_channel") + " WHERE " + column("channelCode") + "=?", []any{channelCode}},
	}
	for _, operation := range operations {
		if _, err := db.ExecContext(ctx, db.Rebind(operation.query), operation.args...); err != nil {
			return err
		}
	}
	return nil
}

func g62Table(dialect, name string) string {
	if dialect == "postgres" {
		return `"` + name + `"`
	}
	return name
}

func g62Column(dialect, name string) string {
	if dialect != "postgres" {
		return name
	}
	return `"` + name + `"`
}

func g62ValidateMySQLDSN(dsn string) error {
	parsed, err := mysqldriver.ParseDSN(dsn)
	if err != nil {
		return fmt.Errorf("parse MySQL DSN: %w", err)
	}
	if !strings.EqualFold(strings.TrimSpace(parsed.DBName), g62IsolatedDatabase) {
		return fmt.Errorf("G6.2 requires database %q", g62IsolatedDatabase)
	}
	return nil
}

func g62ValidatePostgresDSN(dsn string) error {
	parsed, err := url.Parse(dsn)
	if err != nil || parsed == nil || strings.TrimSpace(parsed.Scheme) == "" {
		return errors.New("G6.2 PostgreSQL DSN must be a parseable URL")
	}
	if !strings.EqualFold(strings.Trim(strings.TrimSpace(parsed.Path), "/"), g62IsolatedDatabase) {
		return fmt.Errorf("G6.2 requires database %q", g62IsolatedDatabase)
	}
	return nil
}
