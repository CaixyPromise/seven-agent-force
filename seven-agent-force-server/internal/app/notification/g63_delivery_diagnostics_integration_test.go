//go:build integration

package notification

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	notificationapp "github.com/CaixyPromise/seven-agent/force/internal/app/notification/application"
	notificationdomain "github.com/CaixyPromise/seven-agent/force/internal/app/notification/domain"
	notificationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/notification/facade"
	notificationinfra "github.com/CaixyPromise/seven-agent/force/internal/app/notification/infrastructure"
	mysqldatasource "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/mysql"
	postgresdatasource "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/postgres"
	dbstore "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/store"
	secretvalueinfra "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/crypto/secretvalue"
	"github.com/CaixyPromise/seven-agent/force/internal/infrastructure/security/outboundurl"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/stepup"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/xid"
	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	"github.com/pressly/goose/v3"
	"go.uber.org/zap"
)

const (
	g63DeliveryDiagnosticsIntegrationEnv       = "SEVEN_G63_DELIVERY_DIAGNOSTICS_INTEGRATION"
	g63MySQLDSNEnv                             = "SEVEN_G63_MYSQL_DSN"
	g63PostgresDSNEnv                          = "SEVEN_G63_POSTGRES_DSN"
	g63IsolatedDatabase                        = "seven_notification_g63"
	g63MigrationVersion                  int64 = 20260727170000
	g63PreviousMigrationVersion          int64 = 20260727160000
)

var g63MigrationMu sync.Mutex

// TestG63DeliveryDiagnosticsMySQLIntegration uses only the exact G6.3
// isolated MySQL database. It runs the migration down/up boundary and a
// controlled receiver; it never loads a developer database or provider secret.
func TestG63DeliveryDiagnosticsMySQLIntegration(t *testing.T) {
	if strings.TrimSpace(os.Getenv(g63DeliveryDiagnosticsIntegrationEnv)) != "1" {
		t.Skip("set SEVEN_G63_DELIVERY_DIAGNOSTICS_INTEGRATION=1 to run isolated G6.3 probes")
	}
	dsn := strings.TrimSpace(os.Getenv(g63MySQLDSNEnv))
	if dsn == "" {
		t.Skip("set SEVEN_G63_MYSQL_DSN to the exact isolated G6.3 MySQL database")
	}
	if err := g63ValidateMySQLDSN(dsn); err != nil {
		t.Fatalf("validate isolated MySQL database: %v", err)
	}
	provider, err := mysqldatasource.NewProvider(config.MySQLConfig{
		Enabled: true, DSN: dsn, MaxOpenConns: 8, MaxIdleConns: 2,
	}, zap.NewNop())
	if err != nil {
		t.Fatalf("open isolated MySQL database: %v", err)
	}
	t.Cleanup(func() { _ = provider.Close() })
	if err := g63RoundTripMigration(context.Background(), provider.DB(), "mysql"); err != nil {
		t.Fatalf("G6.3 MySQL migration down/up: %v", err)
	}
	g63RunDeliveryDiagnosticsIntegration(t, "mysql", provider.SQLX(), provider.Transactor())
}

// TestG63DeliveryDiagnosticsPostgresIntegration proves the same boundary
// against PostgreSQL's quoted camel-case schema. The test requires an exact
// isolated DB name before it opens any connection.
func TestG63DeliveryDiagnosticsPostgresIntegration(t *testing.T) {
	if strings.TrimSpace(os.Getenv(g63DeliveryDiagnosticsIntegrationEnv)) != "1" {
		t.Skip("set SEVEN_G63_DELIVERY_DIAGNOSTICS_INTEGRATION=1 to run isolated G6.3 probes")
	}
	dsn := strings.TrimSpace(os.Getenv(g63PostgresDSNEnv))
	if dsn == "" {
		t.Skip("set SEVEN_G63_POSTGRES_DSN to the exact isolated G6.3 PostgreSQL database")
	}
	if err := g63ValidatePostgresDSN(dsn); err != nil {
		t.Fatalf("validate isolated PostgreSQL database: %v", err)
	}
	provider, err := postgresdatasource.NewProvider(config.PostgresConfig{
		Enabled: true, DSN: dsn, MaxOpenConns: 8, MaxIdleConns: 2,
	}, zap.NewNop())
	if err != nil {
		t.Fatalf("open isolated PostgreSQL database: %v", err)
	}
	t.Cleanup(func() { _ = provider.Close() })
	if err := g63RoundTripMigration(context.Background(), provider.DB(), "postgres"); err != nil {
		t.Fatalf("G6.3 PostgreSQL migration down/up: %v", err)
	}
	g63RunDeliveryDiagnosticsIntegration(t, "postgres", provider.SQLX(), provider.Transactor())
}

func g63RoundTripMigration(ctx context.Context, db *sql.DB, dialect string) error {
	g63MigrationMu.Lock()
	defer g63MigrationMu.Unlock()

	migrationsDir, err := filepath.Abs("../../../migrations/" + dialect)
	if err != nil {
		return fmt.Errorf("resolve %s migrations: %w", dialect, err)
	}
	goose.SetTableName("goose_db_version")
	if err := goose.SetDialect(dialect); err != nil {
		return err
	}
	if err := goose.UpToContext(ctx, db, migrationsDir, g63MigrationVersion); err != nil {
		return err
	}
	if version, err := goose.GetDBVersionContext(ctx, db); err != nil || version != g63MigrationVersion {
		return fmt.Errorf("migration version before down=%d err=%w", version, err)
	}
	if err := goose.DownContext(ctx, db, migrationsDir); err != nil {
		return err
	}
	if version, err := goose.GetDBVersionContext(ctx, db); err != nil || version != g63PreviousMigrationVersion {
		return fmt.Errorf("migration version after down=%d err=%w", version, err)
	}
	if err := goose.UpToContext(ctx, db, migrationsDir, g63MigrationVersion); err != nil {
		return err
	}
	if version, err := goose.GetDBVersionContext(ctx, db); err != nil || version != g63MigrationVersion {
		return fmt.Errorf("migration version after recovery=%d err=%w", version, err)
	}
	return nil
}

func g63RunDeliveryDiagnosticsIntegration(t *testing.T, dialect string, db *sqlx.DB, tx dbstore.Transactor) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := g63AssertDiagnosticSchema(ctx, db, dialect); err != nil {
		t.Fatalf("assert G6.3 migration schema: %v", err)
	}

	unique := fmt.Sprintf("%d", time.Now().UnixNano())
	scopeID := "g63-" + dialect + "-" + unique
	sceneCode := "g63_scene_" + unique
	channelCode := "g63-controlled-" + unique
	templateCode := "g63-sensitive-template-" + unique
	otpChannelCode := "g63-otp-channel-" + unique
	otpTemplateCode := "g63-otp-template-" + unique
	if err := g63CleanupScope(ctx, db, dialect, scopeID, []string{channelCode, otpChannelCode}); err != nil {
		t.Fatalf("clear G6.3 fixture: %v", err)
	}
	t.Cleanup(func() {
		_ = g63CleanupScope(context.Background(), db, dialect, scopeID, []string{channelCode, otpChannelCode})
	})

	idGen, err := xid.New(63)
	if err != nil {
		t.Fatalf("new G6.3 ID generator: %v", err)
	}
	driver := &g63ControlledDriver{}
	repo := notificationinfra.NewRepository(db)
	urls := notificationinfra.NewChannelURLValidator(outboundurl.NewOutboundURLGuard(outboundurl.Options{}))
	service := notificationapp.NewService(
		tx,
		repo,
		notificationdomain.NewService(),
		g63SecretService{},
		g63DriverRegistry{driver: driver},
		urls,
		nil,
		idGen,
	)
	service.SetScopeID(scopeID)
	service.BindExternalTargetDigester(g63ExternalTargetDigester{})

	channel, err := service.UpsertChannel(ctx, notificationfacade.ChannelUpsertRequest{
		ChannelCode: channelCode,
		ChannelName: "G6.3 受控飞书应用",
		ChannelType: notificationdomain.ChannelTypeFeishuApp,
		Status:      notificationdomain.ChannelStatusEnabled,
		Priority:    100,
		ProviderConfig: &notificationfacade.ProviderChannelConfig{
			FeishuAppID: "cli_g63_controlled",
		},
		SecretPlain: "g63-controlled-secret",
	}, 6301)
	if err != nil || channel == nil || !channel.SecretConfigured {
		t.Fatalf("create controlled external connection channel=%#v err=%v", channel, err)
	}

	createdTemplate, err := service.CreateTemplateDefinition(ctx, notificationfacade.TemplateDefinitionCreateRequest{
		TemplateCode: templateCode,
		Draft: notificationfacade.TemplateRevisionDraftInput{
			TemplateName:    "G6.3 敏感通知",
			Locale:          "zh-CN",
			SubjectTemplate: "受控诊断 {{.case}}",
			TextTemplate:    "敏感正文 {{.case}}",
			Variables: []notificationfacade.TemplateRevisionVariable{{
				Name: "case", Type: notificationdomain.TemplateVariableTypeString, Required: true, MaxLength: 80,
				Classification: notificationdomain.TemplateVariableClassificationSensitive,
			}},
		},
	}, 6302)
	if err != nil || createdTemplate == nil || createdTemplate.CurrentDraft == nil {
		t.Fatalf("create sensitive template template=%#v err=%v", createdTemplate, err)
	}
	publishedTemplate, err := service.PublishTemplateRevision(ctx, createdTemplate.CurrentDraft.ID, notificationfacade.TemplateRevisionPublishRequest{
		ExpectedVersion: createdTemplate.CurrentDraft.RevisionVersion,
	}, 6303)
	if err != nil || publishedTemplate == nil || publishedTemplate.CurrentPublished == nil {
		t.Fatalf("publish sensitive template template=%#v err=%v", publishedTemplate, err)
	}
	createdScene, err := service.CreateSceneDefinition(ctx, notificationfacade.SceneDefinitionCreateRequest{
		SceneCode: sceneCode,
		Draft: notificationfacade.SceneRevisionDraftInput{
			SceneName:          "G6.3 受控成员通知",
			ReceiverKind:       notificationdomain.SceneReceiverKindFeishuOpenID,
			TemplateRevisionID: publishedTemplate.CurrentPublished.ID,
			ConnectionRef:      channelCode,
			Enabled:            true,
		},
	}, 6304)
	if err != nil || createdScene == nil || createdScene.CurrentDraft == nil {
		t.Fatalf("create G6.3 scene scene=%#v err=%v", createdScene, err)
	}
	publishedScene, err := service.PublishSceneRevision(ctx, createdScene.CurrentDraft.ID, notificationfacade.SceneRevisionPublishRequest{
		ExpectedVersion: createdScene.CurrentDraft.RevisionVersion,
	}, 6305)
	if err != nil || publishedScene == nil || publishedScene.CurrentPublished == nil {
		t.Fatalf("publish G6.3 scene scene=%#v err=%v", publishedScene, err)
	}

	privateValue := "敏感值-" + unique
	publishRequest := notificationfacade.PublishRequest{
		EventKey:          "g63.delivery.diagnostics",
		IdempotencyKey:    "g63-external-" + unique,
		SceneCode:         sceneCode,
		TemplateVariables: map[string]any{"case": privateValue},
		ExternalRecipients: []notificationfacade.ExternalRecipient{{
			IdentityKind: notificationfacade.ExternalIdentityFeishuOpenID,
			Subject:      "ou_g63_controlled_target",
		}},
	}
	receipt, err := service.Publish(ctx, publishRequest)
	if err != nil || receipt == nil || receipt.Duplicate || strings.TrimSpace(receipt.NotificationID) == "" {
		t.Fatalf("publish controlled external notification receipt=%#v err=%v", receipt, err)
	}
	notification, err := repo.FindLogicalNotificationByIdempotency(ctx, scopeID, publishRequest.EventKey, publishRequest.IdempotencyKey)
	if err != nil || notification == nil {
		t.Fatalf("read accepted G6.3 notification notification=%#v err=%v", notification, err)
	}
	deliveries, err := repo.ListDeliveriesByNotificationID(ctx, notification.ID)
	if err != nil || len(deliveries) != 1 {
		t.Fatalf("accepted G6.3 deliveries=%#v err=%v", deliveries, err)
	}
	externalDelivery := deliveries[0]
	if externalDelivery.ContentTier != notificationdomain.DeliveryContentTierSensitive || externalDelivery.RenderedText != "敏感正文 "+privateValue || externalDelivery.ExternalTargetID == nil {
		t.Fatalf("sensitive external delivery=%#v", externalDelivery)
	}
	snapshots, err := repo.ListSceneSnapshotsByNotificationID(ctx, notification.ID)
	if err != nil || len(snapshots) != 1 || snapshots[0].TemplateRevisionID != publishedTemplate.CurrentPublished.ID || snapshots[0].SceneRevisionID != publishedScene.CurrentPublished.ID {
		t.Fatalf("accepted G6.3 snapshot snapshots=%#v err=%v", snapshots, err)
	}

	list, err := service.ListDeliveries(ctx, notificationdomain.DeliveryQuery{Current: 1, PageSize: 20})
	if err != nil || list == nil || len(list.Records) != 1 {
		t.Fatalf("safe delivery list=%#v err=%v", list, err)
	}
	summaryRaw, err := json.Marshal(list.Records[0])
	if err != nil {
		t.Fatalf("marshal safe delivery list: %v", err)
	}
	for _, forbidden := range []string{privateValue, "敏感正文", "ou_g63_controlled_target", "g63-controlled-secret"} {
		if strings.Contains(string(summaryRaw), forbidden) {
			t.Fatalf("ordinary delivery list leaked %q: %s", forbidden, summaryRaw)
		}
	}
	if tier, err := service.DeliveryDiagnosticTier(ctx, externalDelivery.DeliveryID); err != nil || tier != notificationdomain.DeliveryContentTierSensitive {
		t.Fatalf("diagnostic content tier=%q err=%v", tier, err)
	}

	reasonCode := notificationdomain.DeliveryDiagnosticReasonIncident
	ticketReference := "G63-INC-" + unique
	proof := g63Proof(externalDelivery.DeliveryID, reasonCode, ticketReference, "g63-sensitive")
	content, err := service.ReadDeliveryDiagnosticContent(ctx, notificationapp.DeliveryDiagnosticReadCommand{
		DeliveryID: externalDelivery.DeliveryID, ReasonCode: reasonCode, TicketReference: ticketReference,
		ActorID: 6306, GrantedPermission: notificationdomain.DeliveryDiagnosticPermission(notificationdomain.DeliveryContentTierSensitive), StepUpProof: proof,
	})
	if err != nil || content == nil || content.Subject != "受控诊断 "+privateValue || content.Text != "敏感正文 "+privateValue || content.ExpiresAt != nil {
		t.Fatalf("read sensitive diagnostic content=%#v err=%v", content, err)
	}
	if _, err := service.ReadDeliveryDiagnosticContent(ctx, notificationapp.DeliveryDiagnosticReadCommand{
		DeliveryID: externalDelivery.DeliveryID, ReasonCode: reasonCode, TicketReference: ticketReference,
		ActorID: 6306, GrantedPermission: notificationdomain.DeliveryDiagnosticPermission(notificationdomain.DeliveryContentTierPublic), StepUpProof: proof,
	}); err == nil {
		t.Fatal("sensitive content accepted public-only capability")
	}
	foreignService := notificationapp.NewService(tx, repo, notificationdomain.NewService(), g63SecretService{}, g63DriverRegistry{driver: driver}, nil, nil, idGen)
	foreignService.SetScopeID(scopeID + "-foreign")
	foreignService.BindExternalTargetDigester(g63ExternalTargetDigester{})
	if content, err := foreignService.ReadDeliveryDiagnosticContent(ctx, notificationapp.DeliveryDiagnosticReadCommand{
		DeliveryID: externalDelivery.DeliveryID, ReasonCode: reasonCode, TicketReference: ticketReference,
		ActorID: 6307, GrantedPermission: notificationdomain.DeliveryDiagnosticPermission(notificationdomain.DeliveryContentTierSensitive), StepUpProof: proof,
	}); err == nil || content != nil {
		t.Fatalf("foreign scope content=%#v err=%v, want rejection", content, err)
	}

	if err := service.RelayOutbox(ctx, 20); err != nil {
		t.Fatalf("relay controlled G6.3 external delivery: %v", err)
	}
	if driver.Count() != 1 || driver.Last().Target != "ou_g63_controlled_target" || driver.Last().Text != "敏感正文 "+privateValue {
		t.Fatalf("controlled receiver messages=%d last=%#v", driver.Count(), driver.Last())
	}
	if err := g63AssertNoExternalInboxState(ctx, db, dialect, scopeID); err != nil {
		t.Fatalf("external delivery created inbox state: %v", err)
	}
	if err := g63AssertNoPlaintextInOutbox(ctx, db, dialect, scopeID, privateValue); err != nil {
		t.Fatalf("outbox plaintext check: %v", err)
	}

	// A later template/scene publication cannot rewrite the delivery that was
	// accepted before it. The diagnostic read continues to expose only the
	// accepted rendering and the original snapshot relationship.
	nextTemplate, err := service.CreateTemplateDraftFromPublished(ctx, templateCode, 6308)
	if err != nil || nextTemplate == nil || nextTemplate.CurrentDraft == nil {
		t.Fatalf("create next G6.3 template draft template=%#v err=%v", nextTemplate, err)
	}
	nextDraft, err := service.SaveTemplateRevisionDraft(ctx, nextTemplate.CurrentDraft.ID, notificationfacade.TemplateRevisionSaveRequest{
		ExpectedVersion: nextTemplate.CurrentDraft.RevisionVersion,
		Draft: notificationfacade.TemplateRevisionDraftInput{
			TemplateName: "G6.3 更新后的敏感通知", Locale: "zh-CN", SubjectTemplate: "新版 {{.case}}", TextTemplate: "新版正文 {{.case}}",
			Variables: []notificationfacade.TemplateRevisionVariable{{Name: "case", Type: notificationdomain.TemplateVariableTypeString, Required: true, MaxLength: 80, Classification: notificationdomain.TemplateVariableClassificationSensitive}},
		},
	}, 6309)
	if err != nil || nextDraft == nil {
		t.Fatalf("save next G6.3 template draft template=%#v err=%v", nextDraft, err)
	}
	if _, err := service.PublishTemplateRevision(ctx, nextDraft.CurrentDraft.ID, notificationfacade.TemplateRevisionPublishRequest{ExpectedVersion: nextDraft.CurrentDraft.RevisionVersion}, 6310); err != nil {
		t.Fatalf("publish next G6.3 template: %v", err)
	}
	storedExternal, err := repo.FindDeliveryForDiagnostic(ctx, scopeID, externalDelivery.DeliveryID)
	if err != nil || storedExternal == nil || storedExternal.RenderedText != "敏感正文 "+privateValue || storedExternal.ContentTier != notificationdomain.DeliveryContentTierSensitive {
		t.Fatalf("later version changed accepted delivery=%#v err=%v", storedExternal, err)
	}

	if _, err := service.UpsertChannel(ctx, notificationfacade.ChannelUpsertRequest{
		ChannelCode: otpChannelCode, ChannelName: "G6.3 短期秘密邮箱", ChannelType: notificationdomain.ChannelTypeEmail,
		Status: notificationdomain.ChannelStatusEnabled, Priority: 100,
	}, 6311); err != nil {
		t.Fatalf("create controlled OTP channel: %v", err)
	}
	if err := g63InsertPrivateOTPFixture(ctx, db, dialect, scopeID, otpChannelCode, otpTemplateCode); err != nil {
		t.Fatalf("create private OTP fixture: %v", err)
	}
	secretCode := "G63-OTP-" + unique
	if err := service.EnqueueChallengeOTP(ctx, notificationfacade.ChallengeOTPRequest{
		ToEmail: "controlled-g63@example.test", Code: secretCode, Scene: "RESET_PASSWORD", TTL: 2 * time.Minute,
	}); err != nil {
		t.Fatalf("enqueue short-lived secret delivery: %v", err)
	}
	otpDeliveryID, err := g63FindDeliveryID(ctx, db, dialect, otpChannelCode, notificationdomain.SceneChallengeOTP)
	if err != nil {
		t.Fatalf("find short-lived secret delivery: %v", err)
	}
	secretSummary, err := service.ListDeliveries(ctx, notificationdomain.DeliveryQuery{Current: 1, PageSize: 50})
	if err != nil || secretSummary == nil {
		t.Fatalf("list after OTP err=%v page=%#v", err, secretSummary)
	}
	secretSummaryRaw, _ := json.Marshal(secretSummary.Records)
	if strings.Contains(string(secretSummaryRaw), secretCode) {
		t.Fatalf("ordinary list leaked short-lived secret: %s", secretSummaryRaw)
	}
	secretTicket := "G63-SEC-" + unique
	secretContent, err := service.ReadDeliveryDiagnosticContent(ctx, notificationapp.DeliveryDiagnosticReadCommand{
		DeliveryID: otpDeliveryID, ReasonCode: notificationdomain.DeliveryDiagnosticReasonSecurityReview, TicketReference: secretTicket,
		ActorID: 6314, GrantedPermission: notificationdomain.DeliveryDiagnosticPermission(notificationdomain.DeliveryContentTierSecretEphemeral),
		StepUpProof: g63Proof(otpDeliveryID, notificationdomain.DeliveryDiagnosticReasonSecurityReview, secretTicket, "g63-secret"),
	})
	if err != nil || secretContent == nil || secretContent.Text != "验证码 "+secretCode || secretContent.ExpiresAt == nil {
		t.Fatalf("read active short-lived secret content=%#v err=%v", secretContent, err)
	}
	if err := g63AssertProtectedSecretEnvelope(ctx, db, dialect, scopeID, otpDeliveryID, secretCode); err != nil {
		t.Fatalf("short-lived secret envelope: %v", err)
	}
	if err := g63ExpireEphemeralContent(ctx, db, dialect, otpDeliveryID); err != nil {
		t.Fatalf("expire isolated short-lived content: %v", err)
	}
	if expired, err := service.ReadDeliveryDiagnosticContent(ctx, notificationapp.DeliveryDiagnosticReadCommand{
		DeliveryID: otpDeliveryID, ReasonCode: notificationdomain.DeliveryDiagnosticReasonSecurityReview, TicketReference: secretTicket,
		ActorID: 6314, GrantedPermission: notificationdomain.DeliveryDiagnosticPermission(notificationdomain.DeliveryContentTierSecretEphemeral),
		StepUpProof: g63Proof(otpDeliveryID, notificationdomain.DeliveryDiagnosticReasonSecurityReview, secretTicket, "g63-secret-expired"),
	}); err == nil || expired != nil {
		t.Fatalf("expired short-lived secret content=%#v err=%v, want rejection", expired, err)
	}
	if err := g63AssertDiagnosticAudit(ctx, db, dialect, scopeID, externalDelivery.DeliveryID, otpDeliveryID); err != nil {
		t.Fatalf("diagnostic audit safety: %v", err)
	}
}

func g63Proof(deliveryID, reasonCode, ticketReference, suffix string) stepup.ProofMetadata {
	return stepup.ProofMetadata{
		BusinessAction:        notificationapp.DeliveryDiagnosticBusinessAction(),
		OperationBinding:      notificationapp.DeliveryDiagnosticOperationBinding(deliveryID, reasonCode, ticketReference),
		ProofIdentifier:       "proof-" + suffix,
		ChallengeIdentifier:   "challenge-" + suffix,
		AssuranceLevel:        "AAL2",
		AuthenticationMethods: []string{"TOTP"},
	}
}

type g63SecretService struct{}

func (g63SecretService) EncryptString(_ context.Context, plain string) (secretvalueinfra.SecretValue, error) {
	return secretvalueinfra.SecretValue{
		CiphertextB64: base64.RawURLEncoding.EncodeToString([]byte(plain)),
		EDEKB64:       "g63-test-edek",
		WrapKeyRef:    "g63-test-key",
	}, nil
}

func (g63SecretService) DecryptString(_ context.Context, value secretvalueinfra.SecretValue) (string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(value.CiphertextB64)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

type g63ExternalTargetDigester struct{}

func (g63ExternalTargetDigester) Digest(_ context.Context, keyRef, scopeID, connectionRef, identityKind, subject string) (string, string, error) {
	if strings.TrimSpace(keyRef) == "" {
		keyRef = "g63-test-digest-key"
	}
	sum := sha256.Sum256([]byte(strings.Join([]string{scopeID, connectionRef, identityKind, subject}, "\x00")))
	return hex.EncodeToString(sum[:]), keyRef, nil
}

type g63DriverRegistry struct{ driver *g63ControlledDriver }

func (r g63DriverRegistry) Driver(channelType string) notificationapp.ChannelDriver {
	if strings.EqualFold(strings.TrimSpace(channelType), notificationdomain.ChannelTypeFeishuApp) {
		return r.driver
	}
	return nil
}

type g63ControlledDriver struct {
	mu       sync.Mutex
	messages []notificationapp.DriverMessage
}

func (d *g63ControlledDriver) Send(ctx context.Context, message notificationapp.DriverMessage) error {
	_, err := d.SendResult(ctx, message)
	return err
}

func (d *g63ControlledDriver) SendResult(_ context.Context, message notificationapp.DriverMessage) (notificationapp.DriverResult, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.messages = append(d.messages, message)
	return notificationapp.DriverResult{Status: notificationapp.DriverResultProviderAccepted, ProviderReference: "g63-controlled-receiver"}, nil
}

func (d *g63ControlledDriver) Count() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.messages)
}

func (d *g63ControlledDriver) Last() notificationapp.DriverMessage {
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.messages) == 0 {
		return notificationapp.DriverMessage{}
	}
	return d.messages[len(d.messages)-1]
}

func g63AssertDiagnosticSchema(ctx context.Context, db *sqlx.DB, dialect string) error {
	for _, table := range []string{"sys_notification_delivery_ephemeral_content", "sys_notification_delivery_diagnostic_audit"} {
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
	var columnCount int
	query := "SELECT COUNT(1) FROM information_schema.columns WHERE table_name=? AND column_name=?"
	if dialect == "postgres" {
		query += " AND table_schema=current_schema()"
	} else {
		query += " AND table_schema=DATABASE()"
	}
	if err := db.GetContext(ctx, &columnCount, db.Rebind(query), "sys_notification_delivery", "contentTier"); err != nil {
		return err
	}
	if columnCount != 1 {
		return errors.New("sys_notification_delivery.contentTier is missing")
	}
	return nil
}

func g63AssertNoExternalInboxState(ctx context.Context, db *sqlx.DB, dialect, scopeID string) error {
	for _, item := range []struct {
		table string
		query string
	}{
		{"sys_notification_recipient", "SELECT COUNT(1) FROM " + g63Table(dialect, "sys_notification_recipient") + " WHERE " + g63Column(dialect, "scopeId") + "=?"},
		{"sys_notification_mailbox", "SELECT COUNT(1) FROM " + g63Table(dialect, "sys_notification_mailbox") + " WHERE " + g63Column(dialect, "scopeId") + "=?"},
		{"notification.inbox.changed", "SELECT COUNT(1) FROM " + g63Table(dialect, "sys_outbox_event") + " WHERE " + g63Column(dialect, "scopeId") + "=? AND " + g63Column(dialect, "eventType") + "='notification.inbox.changed'"},
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

func g63AssertNoPlaintextInOutbox(ctx context.Context, db *sqlx.DB, dialect, scopeID, plaintext string) error {
	var payloads []string
	query := "SELECT COALESCE(" + g63Column(dialect, "payload") + ", '') FROM " + g63Table(dialect, "sys_outbox_event") + " WHERE " + g63Column(dialect, "scopeId") + "=?"
	if err := db.SelectContext(ctx, &payloads, db.Rebind(query), scopeID); err != nil {
		return err
	}
	for _, payload := range payloads {
		if strings.Contains(payload, plaintext) {
			return errors.New("outbox payload contains delivery plaintext")
		}
	}
	return nil
}

func g63FindDeliveryID(ctx context.Context, db *sqlx.DB, dialect, channelCode, sceneCode string) (string, error) {
	var deliveryID string
	query := "SELECT " + g63Column(dialect, "deliveryId") + " FROM " + g63Table(dialect, "sys_notification_delivery") +
		" WHERE " + g63Column(dialect, "channelCode") + "=? AND " + g63Column(dialect, "sceneCode") + "=? ORDER BY " + g63Column(dialect, "id") + " DESC LIMIT 1"
	if err := db.GetContext(ctx, &deliveryID, db.Rebind(query), channelCode, sceneCode); err != nil {
		return "", err
	}
	return deliveryID, nil
}

// g63InsertPrivateOTPFixture seeds the internal Challenge OTP configuration
// directly. The public legacy template/scene management APIs no longer exist.
func g63InsertPrivateOTPFixture(ctx context.Context, db *sqlx.DB, dialect, scopeID, channelCode, templateCode string) error {
	templateQuery := "INSERT INTO " + g63Table(dialect, "sys_notification_template") +
		" (" + strings.Join([]string{
		g63Column(dialect, "id"), g63Column(dialect, "templateCode"), g63Column(dialect, "scopeId"),
		g63Column(dialect, "templateName"), g63Column(dialect, "sceneCode"), g63Column(dialect, "channelType"),
		g63Column(dialect, "locale"), g63Column(dialect, "subjectTemplate"), g63Column(dialect, "textTemplate"),
		g63Column(dialect, "status"), g63Column(dialect, "version"),
	}, ",") + ") VALUES (?,?,?,?,?,?,?,?,?,?,?)"
	if _, err := db.ExecContext(ctx, db.Rebind(templateQuery), time.Now().UnixNano(), templateCode, scopeID, "G6.3 短期秘密模板", notificationdomain.SceneChallengeOTP, notificationdomain.ChannelTypeEmail, "zh-CN", "验证码", "验证码 {{.Code}}", notificationdomain.ChannelStatusEnabled, 1); err != nil {
		return err
	}
	bindingQuery := "INSERT INTO " + g63Table(dialect, "sys_notification_scene_binding") +
		" (" + strings.Join([]string{
		g63Column(dialect, "id"), g63Column(dialect, "scopeId"), g63Column(dialect, "sceneCode"),
		g63Column(dialect, "sceneName"), g63Column(dialect, "channelCode"), g63Column(dialect, "templateCode"),
		g63Column(dialect, "enabled"), g63Column(dialect, "priority"), g63Column(dialect, "maxRetry"),
		g63Column(dialect, "retryIntervalSeconds"),
	}, ",") + ") VALUES (?,?,?,?,?,?,?,?,?,?)"
	_, err := db.ExecContext(ctx, db.Rebind(bindingQuery), time.Now().UnixNano()+1, scopeID, notificationdomain.SceneChallengeOTP, "G6.3 验证码", channelCode, templateCode, 1, 100, 0, 60)
	return err
}

func g63AssertProtectedSecretEnvelope(ctx context.Context, db *sqlx.DB, dialect, scopeID, deliveryID, secret string) error {
	var ciphertext string
	query := "SELECT ciphertext FROM " + g63Table(dialect, "sys_notification_delivery_ephemeral_content") + " WHERE " + g63Column(dialect, "scopeId") + "=? AND " + g63Column(dialect, "deliveryId") + "=?"
	if err := db.GetContext(ctx, &ciphertext, db.Rebind(query), scopeID, deliveryID); err != nil {
		return err
	}
	if strings.Contains(ciphertext, secret) {
		return errors.New("ephemeral ciphertext contains plaintext secret")
	}
	var normalCount int
	payloadText := "COALESCE(" + g63Column(dialect, "payloadJson") + ", '')"
	if dialect == "postgres" {
		payloadText = "COALESCE(" + g63Column(dialect, "payloadJson") + "::text, '')"
	}
	normalQuery := "SELECT COUNT(1) FROM " + g63Table(dialect, "sys_notification_delivery") + " WHERE " + g63Column(dialect, "deliveryId") + "=? AND (COALESCE(" + g63Column(dialect, "renderedSubject") + ", '')<>'' OR COALESCE(" + g63Column(dialect, "renderedText") + ", '')<>'' OR " + payloadText + "<>'')"
	if err := db.GetContext(ctx, &normalCount, db.Rebind(normalQuery), deliveryID); err != nil {
		return err
	}
	if normalCount != 0 {
		return errors.New("short-lived secret escaped into normal delivery columns")
	}
	return nil
}

func g63ExpireEphemeralContent(ctx context.Context, db *sqlx.DB, dialect, deliveryID string) error {
	query := "UPDATE " + g63Table(dialect, "sys_notification_delivery_ephemeral_content") + " SET " + g63Column(dialect, "expiresAt") + "=? WHERE " + g63Column(dialect, "deliveryId") + "=?"
	_, err := db.ExecContext(ctx, db.Rebind(query), time.Now().UTC().Add(-time.Minute), deliveryID)
	return err
}

func g63AssertDiagnosticAudit(ctx context.Context, db *sqlx.DB, dialect, scopeID, externalDeliveryID, secretDeliveryID string) error {
	var results []string
	query := "SELECT " + g63Column(dialect, "resultCode") + " FROM " + g63Table(dialect, "sys_notification_delivery_diagnostic_audit") + " WHERE " + g63Column(dialect, "scopeId") + "=? ORDER BY " + g63Column(dialect, "id")
	if err := db.SelectContext(ctx, &results, db.Rebind(query), scopeID); err != nil {
		return err
	}
	for _, required := range []string{
		notificationdomain.DeliveryDiagnosticResultAllowed,
		notificationdomain.DeliveryDiagnosticResultDenied,
		notificationdomain.DeliveryDiagnosticResultExpired,
	} {
		found := false
		for _, result := range results {
			if result == required {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("diagnostic audit lacks result %s: %v", required, results)
		}
	}
	var columns []string
	columnQuery := "SELECT column_name FROM information_schema.columns WHERE table_name=?"
	if dialect == "postgres" {
		columnQuery += " AND table_schema=current_schema()"
	} else {
		columnQuery += " AND table_schema=DATABASE()"
	}
	if err := db.SelectContext(ctx, &columns, db.Rebind(columnQuery), "sys_notification_delivery_diagnostic_audit"); err != nil {
		return err
	}
	for _, column := range columns {
		lower := strings.ToLower(column)
		// contentTier is a classification enum, not a content-bearing column.
		// All other listed names would make the audit table retain a secret,
		// body, raw target or provider response.
		for _, forbidden := range []string{"body", "payload", "target", "secret", "credential", "provider", "url"} {
			if strings.Contains(lower, forbidden) {
				return fmt.Errorf("diagnostic audit schema contains unsafe column %q", column)
			}
		}
	}
	for _, deliveryID := range []string{externalDeliveryID, secretDeliveryID} {
		var count int
		rowQuery := "SELECT COUNT(1) FROM " + g63Table(dialect, "sys_notification_delivery_diagnostic_audit") + " WHERE " + g63Column(dialect, "scopeId") + "=? AND " + g63Column(dialect, "deliveryId") + "=?"
		if err := db.GetContext(ctx, &count, db.Rebind(rowQuery), scopeID, deliveryID); err != nil || count == 0 {
			return fmt.Errorf("missing audit for delivery %s count=%d err=%w", deliveryID, count, err)
		}
	}
	return nil
}

func g63CleanupScope(ctx context.Context, db *sqlx.DB, dialect, scopeID string, channelCodes []string) error {
	table := func(name string) string { return g63Table(dialect, name) }
	column := func(name string) string { return g63Column(dialect, name) }
	quotedCodes, args := g63Placeholders(channelCodes)
	deliveryIDs := "SELECT " + column("deliveryId") + " FROM " + table("sys_notification_delivery") + " WHERE " + column("channelCode") + " IN (" + quotedCodes + ")"
	notificationIDs := "SELECT " + column("id") + " FROM " + table("sys_notification") + " WHERE " + column("scopeId") + "=?"
	sceneIDs := "SELECT " + column("id") + " FROM " + table("sys_notification_scene_definition") + " WHERE " + column("scopeId") + "=?"
	templateIDs := "SELECT " + column("id") + " FROM " + table("sys_notification_template_definition") + " WHERE " + column("scopeId") + "=?"
	operations := []struct {
		query string
		args  []any
	}{
		{"DELETE FROM " + table("sys_notification_delivery_diagnostic_audit") + " WHERE " + column("scopeId") + "=?", []any{scopeID}},
		{"DELETE FROM " + table("sys_notification_delivery_ephemeral_content") + " WHERE " + column("scopeId") + "=?", []any{scopeID}},
		{"DELETE FROM " + table("sys_notification_delivery_attempt") + " WHERE " + column("deliveryId") + " IN (" + deliveryIDs + ")", args},
		{"DELETE FROM " + table("sys_notification_delivery") + " WHERE " + column("channelCode") + " IN (" + quotedCodes + ")", args},
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
		{"DELETE FROM " + table("sys_notification_scene_binding") + " WHERE " + column("scopeId") + "=?", []any{scopeID}},
		{"DELETE FROM " + table("sys_notification_template") + " WHERE " + column("scopeId") + "=?", []any{scopeID}},
		{"DELETE FROM " + table("sys_notification_channel") + " WHERE " + column("scopeId") + "=?", []any{scopeID}},
	}
	for _, operation := range operations {
		if _, err := db.ExecContext(ctx, db.Rebind(operation.query), operation.args...); err != nil {
			return err
		}
	}
	return nil
}

func g63Placeholders(values []string) (string, []any) {
	if len(values) == 0 {
		return "NULL", nil
	}
	placeholders := make([]string, 0, len(values))
	args := make([]any, 0, len(values))
	for _, value := range values {
		placeholders = append(placeholders, "?")
		args = append(args, value)
	}
	return strings.Join(placeholders, ","), args
}

func g63Table(dialect, name string) string {
	if dialect == "postgres" {
		return `"` + name + `"`
	}
	return name
}

func g63Column(dialect, name string) string {
	if dialect != "postgres" {
		return name
	}
	return `"` + name + `"`
}

func g63ValidateMySQLDSN(dsn string) error {
	parsed, err := mysqldriver.ParseDSN(dsn)
	if err != nil {
		return fmt.Errorf("parse MySQL DSN: %w", err)
	}
	if !strings.EqualFold(strings.TrimSpace(parsed.DBName), g63IsolatedDatabase) {
		return fmt.Errorf("G6.3 requires database %q", g63IsolatedDatabase)
	}
	return nil
}

func g63ValidatePostgresDSN(dsn string) error {
	parsed, err := url.Parse(dsn)
	if err != nil || parsed == nil || strings.TrimSpace(parsed.Scheme) == "" {
		return errors.New("G6.3 PostgreSQL DSN must be a parseable URL")
	}
	if !strings.EqualFold(strings.Trim(strings.TrimSpace(parsed.Path), "/"), g63IsolatedDatabase) {
		return fmt.Errorf("G6.3 requires database %q", g63IsolatedDatabase)
	}
	return nil
}
