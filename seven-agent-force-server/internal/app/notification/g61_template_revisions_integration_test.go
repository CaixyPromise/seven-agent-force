//go:build integration

package notification

import (
	"context"
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
	mysqldatasource "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/mysql"
	postgresdatasource "github.com/CaixyPromise/seven-agent/force/internal/infrastructure/datasource/postgres"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/config"
	mysqldriver "github.com/go-sql-driver/mysql"
	"github.com/jmoiron/sqlx"
	"go.uber.org/zap"
)

const (
	g61TemplateIntegrationEnv = "SEVEN_G61_TEMPLATE_INTEGRATION"
	g61MySQLDSNEnv            = "SEVEN_G61_MYSQL_DSN"
	g61PostgresDSNEnv         = "SEVEN_G61_POSTGRES_DSN"
	g61IsolatedDatabase       = "seven_notification_g61"
)

// TestG61TemplateRevisionMySQLIntegration proves the real MySQL repository,
// transaction boundary, migration schema, immutable publication and zero
// notification-delivery side effects. It runs only against the explicitly
// named isolated G6.1 database.
func TestG61TemplateRevisionMySQLIntegration(t *testing.T) {
	if strings.TrimSpace(os.Getenv(g61TemplateIntegrationEnv)) != "1" {
		t.Skip("set SEVEN_G61_TEMPLATE_INTEGRATION=1 to run the isolated G6.1 probes")
	}
	dsn := strings.TrimSpace(os.Getenv(g61MySQLDSNEnv))
	if dsn == "" {
		t.Skip("set SEVEN_G61_MYSQL_DSN to the isolated G6.1 MySQL database")
	}
	if err := g61ValidateMySQLDSN(dsn); err != nil {
		t.Fatalf("validate isolated MySQL database: %v", err)
	}
	provider, err := mysqldatasource.NewProvider(config.MySQLConfig{
		Enabled: true, DSN: dsn, MaxOpenConns: 6, MaxIdleConns: 2,
	}, zap.NewNop())
	if err != nil {
		t.Fatalf("open isolated MySQL database: %v", err)
	}
	t.Cleanup(func() { _ = provider.Close() })
	repo := notificationinfra.NewRepository(provider.SQLX())
	service := notificationapp.NewService(provider.Transactor(), repo, notificationdomain.NewService(), nil, nil, nil, nil, nil)
	g61RunTemplateRevisionIntegration(t, "mysql", provider.SQLX(), service, repo)
}

// TestG61TemplateRevisionPostgresIntegration exercises the same repository
// contract with PostgreSQL's quoted camelCase identifiers and JSONB storage.
func TestG61TemplateRevisionPostgresIntegration(t *testing.T) {
	if strings.TrimSpace(os.Getenv(g61TemplateIntegrationEnv)) != "1" {
		t.Skip("set SEVEN_G61_TEMPLATE_INTEGRATION=1 to run the isolated G6.1 probes")
	}
	dsn := strings.TrimSpace(os.Getenv(g61PostgresDSNEnv))
	if dsn == "" {
		t.Skip("set SEVEN_G61_POSTGRES_DSN to the isolated G6.1 PostgreSQL database")
	}
	if err := g61ValidatePostgresDSN(dsn); err != nil {
		t.Fatalf("validate isolated PostgreSQL database: %v", err)
	}
	provider, err := postgresdatasource.NewProvider(config.PostgresConfig{
		Enabled: true, DSN: dsn, MaxOpenConns: 6, MaxIdleConns: 2,
	}, zap.NewNop())
	if err != nil {
		t.Fatalf("open isolated PostgreSQL database: %v", err)
	}
	t.Cleanup(func() { _ = provider.Close() })
	repo := notificationinfra.NewRepository(provider.SQLX())
	service := notificationapp.NewService(provider.Transactor(), repo, notificationdomain.NewService(), nil, nil, nil, nil, nil)
	g61RunTemplateRevisionIntegration(t, "postgres", provider.SQLX(), service, repo)
}

func g61RunTemplateRevisionIntegration(t *testing.T, dialect string, db *sqlx.DB, service *notificationapp.Service, repo *notificationinfra.Repository) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := g61AssertTemplateRevisionSchema(ctx, db, dialect); err != nil {
		t.Fatalf("assert G6.1 migration schema: %v", err)
	}

	unique := fmt.Sprintf("%d", time.Now().UnixNano())
	scopeA := "g61-" + dialect + "-" + unique
	scopeB := scopeA + "-foreign"
	templateCode := "g61_" + dialect + "_" + unique
	if err := g61CleanupTemplateScope(ctx, db, dialect, scopeA); err != nil {
		t.Fatalf("clear prior G6.1 fixture: %v", err)
	}
	t.Cleanup(func() {
		_ = g61CleanupTemplateScope(context.Background(), db, dialect, scopeA)
	})

	service.SetScopeID(scopeA)
	foreignService := notificationapp.NewService(provider.Transactor(), repo, notificationdomain.NewService(), nil, nil, nil, nil, nil)
	foreignService.SetScopeID(scopeB)

	effectsBefore, err := g61ReadNotificationEffects(ctx, db, dialect)
	if err != nil {
		t.Fatalf("read pre-preview notification effects: %v", err)
	}
	previewOnlyValue := "preview-only-sensitive-value"
	preview, err := service.PreviewTemplateRevision(ctx, notificationfacade.TemplateRevisionPreviewRequest{
		Draft:     g61Draft("G6.1 preview", "你好 {{.name}}，金额 {{.amount}}，状态 {{.enabled}}，时间 {{.occurredAt}}，备注 {{.note}}"),
		Variables: g61PreviewValues(previewOnlyValue),
	})
	if err != nil {
		t.Fatalf("preview valid draft: %v", err)
	}
	if !strings.Contains(preview.Text, previewOnlyValue) {
		t.Fatalf("preview did not render supplied value: %#v", preview)
	}
	if err := g61AssertNoNotificationEffects(ctx, db, dialect, effectsBefore); err != nil {
		t.Fatalf("preview created notification side effects: %v", err)
	}
	for name, values := range map[string]map[string]any{
		"extra value": {
			"name": "小七", "amount": 18.5, "enabled": true, "occurredAt": time.Now().UTC().Format(time.RFC3339), "note": previewOnlyValue, "extra": "not allowed",
		},
		"missing required": {
			"amount": 18.5, "enabled": true, "occurredAt": time.Now().UTC().Format(time.RFC3339), "note": previewOnlyValue,
		},
		"wrong number type": {
			"name": "小七", "amount": "not-a-number", "enabled": true, "occurredAt": time.Now().UTC().Format(time.RFC3339), "note": previewOnlyValue,
		},
	} {
		if _, err := service.PreviewTemplateRevision(ctx, notificationfacade.TemplateRevisionPreviewRequest{
			Draft:     g61Draft("G6.1 invalid "+name, "{{.name}} {{.amount}} {{.enabled}} {{.occurredAt}} {{.note}}"),
			Variables: values,
		}); err == nil {
			t.Fatalf("preview %s unexpectedly succeeded", name)
		}
	}
	if _, err := service.PreviewTemplateRevision(ctx, notificationfacade.TemplateRevisionPreviewRequest{
		Draft:     g61Draft("G6.1 invalid syntax", "{{if .name}}x{{end}}"),
		Variables: g61PreviewValues(previewOnlyValue),
	}); err == nil {
		t.Fatal("preview with control syntax unexpectedly succeeded")
	}
	if err := g61AssertNoNotificationEffects(ctx, db, dialect, effectsBefore); err != nil {
		t.Fatalf("invalid preview created notification side effects: %v", err)
	}

	created, err := service.CreateTemplateDefinition(ctx, notificationfacade.TemplateDefinitionCreateRequest{
		TemplateCode: templateCode,
		Draft:        g61Draft("G6.1 template", "{{.name}} {{.amount}} {{.enabled}} {{.occurredAt}} {{.note}}"),
	}, 601)
	if err != nil {
		t.Fatalf("create versioned draft: %v", err)
	}
	if created.CurrentDraft == nil || created.CurrentDraft.State != notificationdomain.TemplateRevisionStateDraft || created.CurrentPublished != nil {
		t.Fatalf("unexpected created definition: %#v", created)
	}
	if g61SensitiveSamplePersisted(created.CurrentDraft.Variables) {
		t.Fatal("sensitive variable sample value was returned from persisted revision")
	}
	if _, err := service.CreateTemplateDefinition(ctx, notificationfacade.TemplateDefinitionCreateRequest{
		TemplateCode: templateCode + "_secret",
		Draft: notificationfacade.TemplateRevisionDraftInput{
			TemplateName: "forbidden secret", Locale: "zh-CN", TextTemplate: "{{.otp}}",
			Variables: []notificationfacade.TemplateRevisionVariable{{Name: "otp", Type: notificationdomain.TemplateVariableTypeSecretEphemeral, Required: true, Classification: notificationdomain.TemplateVariableClassificationSensitive}},
		},
	}, 602); err == nil {
		t.Fatal("ordinary template accepted SECRET_EPHEMERAL")
	}

	saved, err := service.SaveTemplateRevisionDraft(ctx, created.CurrentDraft.ID, notificationfacade.TemplateRevisionSaveRequest{
		ExpectedVersion: created.CurrentDraft.RevisionVersion,
		Draft:           g61Draft("G6.1 template saved", "更新 {{.name}} {{.amount}} {{.enabled}} {{.occurredAt}} {{.note}}"),
	}, 603)
	if err != nil {
		t.Fatalf("save draft: %v", err)
	}
	if saved.CurrentDraft == nil || saved.CurrentDraft.RevisionVersion <= created.CurrentDraft.RevisionVersion {
		t.Fatalf("draft optimistic version did not advance: created=%#v saved=%#v", created.CurrentDraft, saved.CurrentDraft)
	}
	if _, err := service.SaveTemplateRevisionDraft(ctx, created.CurrentDraft.ID, notificationfacade.TemplateRevisionSaveRequest{
		ExpectedVersion: created.CurrentDraft.RevisionVersion,
		Draft:           g61Draft("stale", "{{.name}} {{.amount}} {{.enabled}} {{.occurredAt}} {{.note}}"),
	}, 604); !errors.Is(err, notificationdomain.ErrTemplateRevisionConflict) {
		t.Fatalf("stale draft save error=%v, want conflict", err)
	}

	published, err := service.PublishTemplateRevision(ctx, saved.CurrentDraft.ID, notificationfacade.TemplateRevisionPublishRequest{ExpectedVersion: saved.CurrentDraft.RevisionVersion}, 605)
	if err != nil {
		t.Fatalf("publish first revision: %v", err)
	}
	if published.CurrentDraft != nil || published.CurrentPublished == nil || published.CurrentPublished.State != notificationdomain.TemplateRevisionStatePublished {
		t.Fatalf("unexpected published definition: %#v", published)
	}
	firstPublishedID := published.CurrentPublished.ID
	firstDigest := published.CurrentPublished.ContentDigest
	if _, err := service.SaveTemplateRevisionDraft(ctx, firstPublishedID, notificationfacade.TemplateRevisionSaveRequest{
		ExpectedVersion: published.CurrentPublished.RevisionVersion,
		Draft:           g61Draft("must fail", "{{.name}} {{.amount}} {{.enabled}} {{.occurredAt}} {{.note}}"),
	}, 606); !errors.Is(err, notificationdomain.ErrTemplateRevisionImmutable) {
		t.Fatalf("published revision save error=%v, want immutable", err)
	}

	next, err := service.CreateTemplateDraftFromPublished(ctx, templateCode, 607)
	if err != nil {
		t.Fatalf("clone published revision to draft: %v", err)
	}
	if next.CurrentDraft == nil || next.CurrentDraft.RevisionNo != 2 || next.CurrentPublished == nil || next.CurrentPublished.ID != firstPublishedID {
		t.Fatalf("published clone did not preserve history: %#v", next)
	}
	if _, err := foreignService.GetTemplateDefinition(ctx, templateCode); err == nil {
		t.Fatal("foreign scope read versioned template")
	}
	if _, err := foreignService.SaveTemplateRevisionDraft(ctx, next.CurrentDraft.ID, notificationfacade.TemplateRevisionSaveRequest{
		ExpectedVersion: next.CurrentDraft.RevisionVersion,
		Draft:           g61Draft("foreign edit", "{{.name}} {{.amount}} {{.enabled}} {{.occurredAt}} {{.note}}"),
	}, 608); !errors.Is(err, notificationdomain.ErrTemplateDefinitionNotFound) {
		t.Fatalf("foreign scope edit error=%v, want hidden definition", err)
	}
	if _, err := foreignService.PublishTemplateRevision(ctx, next.CurrentDraft.ID, notificationfacade.TemplateRevisionPublishRequest{ExpectedVersion: next.CurrentDraft.RevisionVersion}, 609); !errors.Is(err, notificationdomain.ErrTemplateDefinitionNotFound) {
		t.Fatalf("foreign scope publish error=%v, want hidden definition", err)
	}

	var publishWG sync.WaitGroup
	publishResults := make(chan error, 2)
	for range 2 {
		publishWG.Add(1)
		go func() {
			defer publishWG.Done()
			_, publishErr := service.PublishTemplateRevision(ctx, next.CurrentDraft.ID, notificationfacade.TemplateRevisionPublishRequest{ExpectedVersion: next.CurrentDraft.RevisionVersion}, 610)
			publishResults <- publishErr
		}()
	}
	publishWG.Wait()
	close(publishResults)
	successfulPublishes := 0
	for publishErr := range publishResults {
		if publishErr == nil {
			successfulPublishes++
			continue
		}
		if !errors.Is(publishErr, notificationdomain.ErrTemplateRevisionConflict) && !errors.Is(publishErr, notificationdomain.ErrTemplateRevisionImmutable) {
			t.Fatalf("concurrent publish error=%v", publishErr)
		}
	}
	if successfulPublishes != 1 {
		t.Fatalf("concurrent publish successes=%d, want exactly one", successfulPublishes)
	}
	firstRevision, err := repo.FindTemplateRevisionByID(ctx, firstPublishedID)
	if err != nil || firstRevision == nil || firstRevision.State != notificationdomain.TemplateRevisionStateSuperseded || firstRevision.ContentDigest != firstDigest {
		t.Fatalf("first published revision was changed instead of superseded revision=%#v err=%v", firstRevision, err)
	}
	latest, err := service.GetTemplateDefinition(ctx, templateCode)
	if err != nil || latest.CurrentDraft != nil || latest.CurrentPublished == nil || latest.CurrentPublished.RevisionNo != 2 || latest.CurrentPublished.State != notificationdomain.TemplateRevisionStatePublished {
		t.Fatalf("published pointer is not atomic latest=%#v err=%v", latest, err)
	}
	if len(latest.Revisions) != 2 || latest.Revisions[0].RevisionNo != 2 || latest.Revisions[0].State != notificationdomain.TemplateRevisionStatePublished || latest.Revisions[1].RevisionNo != 1 || latest.Revisions[1].State != notificationdomain.TemplateRevisionStateSuperseded || latest.Revisions[1].ContentDigest != firstDigest {
		t.Fatalf("published revision history is not readable and immutable: %#v", latest.Revisions)
	}

	if err := g61AssertSafeAudit(ctx, db, dialect, scopeA, previewOnlyValue); err != nil {
		t.Fatalf("audit safety: %v", err)
	}
	if err := g61AssertNoNotificationEffects(ctx, db, dialect, effectsBefore); err != nil {
		t.Fatalf("template lifecycle created notification effects: %v", err)
	}
}

func g61Draft(name, text string) notificationfacade.TemplateRevisionDraftInput {
	return notificationfacade.TemplateRevisionDraftInput{
		TemplateName:    name,
		Locale:          "zh-CN",
		SubjectTemplate: "通知给 {{.name}}",
		TextTemplate:    text,
		Variables: []notificationfacade.TemplateRevisionVariable{
			{Name: "name", Type: notificationdomain.TemplateVariableTypeString, Required: true, MaxLength: 80, SampleValue: "样例用户", Classification: notificationdomain.TemplateVariableClassificationPublic},
			{Name: "amount", Type: notificationdomain.TemplateVariableTypeNumber, Required: true, SampleValue: 1.5, Classification: notificationdomain.TemplateVariableClassificationPublic},
			{Name: "enabled", Type: notificationdomain.TemplateVariableTypeBoolean, Required: true, SampleValue: true, Classification: notificationdomain.TemplateVariableClassificationPublic},
			{Name: "occurredAt", Type: notificationdomain.TemplateVariableTypeDateTime, Required: true, SampleValue: "2026-07-27T00:00:00Z", Classification: notificationdomain.TemplateVariableClassificationPublic},
			{Name: "note", Type: notificationdomain.TemplateVariableTypeString, Required: true, MaxLength: 160, SampleValue: "schema-sensitive-sample", Classification: notificationdomain.TemplateVariableClassificationSensitive},
		},
	}
}

func g61PreviewValues(note string) map[string]any {
	return map[string]any{
		"name":       "小七",
		"amount":     18.5,
		"enabled":    true,
		"occurredAt": "2026-07-27T12:34:56Z",
		"note":       note,
	}
}

func g61SensitiveSamplePersisted(variables []notificationfacade.TemplateRevisionVariable) bool {
	for _, variable := range variables {
		if variable.Classification == notificationdomain.TemplateVariableClassificationSensitive && variable.SampleValue != nil {
			return true
		}
	}
	return false
}

type g61NotificationEffects struct {
	Outbox       int64
	Notification int64
	Recipient    int64
	Delivery     int64
}

func g61ReadNotificationEffects(ctx context.Context, db *sqlx.DB, dialect string) (g61NotificationEffects, error) {
	result := g61NotificationEffects{}
	for _, entry := range []struct {
		table string
		value *int64
	}{
		{"sys_outbox_event", &result.Outbox},
		{"sys_notification", &result.Notification},
		{"sys_notification_recipient", &result.Recipient},
		{"sys_notification_delivery", &result.Delivery},
	} {
		if err := db.GetContext(ctx, entry.value, "SELECT COUNT(1) FROM "+g61Table(dialect, entry.table)); err != nil {
			return g61NotificationEffects{}, fmt.Errorf("count %s: %w", entry.table, err)
		}
	}
	return result, nil
}

func g61AssertNoNotificationEffects(ctx context.Context, db *sqlx.DB, dialect string, expected g61NotificationEffects) error {
	actual, err := g61ReadNotificationEffects(ctx, db, dialect)
	if err != nil {
		return err
	}
	if actual != expected {
		return fmt.Errorf("before=%+v after=%+v", expected, actual)
	}
	return nil
}

func g61AssertTemplateRevisionSchema(ctx context.Context, db *sqlx.DB, dialect string) error {
	for _, table := range []string{"sys_notification_template_definition", "sys_notification_template_revision", "sys_notification_template_revision_audit"} {
		var count int
		query := `SELECT COUNT(1) FROM information_schema.tables WHERE table_schema=`
		if dialect == "postgres" {
			query += `current_schema() AND table_name=?`
		} else {
			query += `DATABASE() AND table_name=?`
		}
		if err := db.GetContext(ctx, &count, db.Rebind(query), table); err != nil {
			return err
		}
		if count != 1 {
			return fmt.Errorf("missing table %s", table)
		}
	}
	return nil
}

func g61AssertSafeAudit(ctx context.Context, db *sqlx.DB, dialect, scopeID, forbidden string) error {
	var actions []string
	query := "SELECT " + g61Column(dialect, "action") + " FROM " + g61Table(dialect, "sys_notification_template_revision_audit") + " WHERE " + g61Column(dialect, "scopeId") + "=?"
	if err := db.SelectContext(ctx, &actions, db.Rebind(query), scopeID); err != nil {
		return err
	}
	if len(actions) < 4 {
		return fmt.Errorf("audit action count=%d, want at least create/save/publish/copy", len(actions))
	}
	for _, action := range actions {
		if strings.Contains(action, forbidden) {
			return fmt.Errorf("audit action leaked preview value")
		}
	}
	var columns []string
	query = `SELECT column_name FROM information_schema.columns WHERE table_name=?`
	if dialect == "postgres" {
		query += ` AND table_schema=current_schema()`
	} else {
		query += ` AND table_schema=DATABASE()`
	}
	if err := db.SelectContext(ctx, &columns, db.Rebind(query), "sys_notification_template_revision_audit"); err != nil {
		return err
	}
	for _, column := range columns {
		lower := strings.ToLower(column)
		for _, forbiddenName := range []string{"body", "content", "variable", "preview", "target", "secret", "credential", "provider"} {
			if strings.Contains(lower, forbiddenName) {
				return fmt.Errorf("audit schema has forbidden column %q", column)
			}
		}
	}
	return nil
}

func g61CleanupTemplateScope(ctx context.Context, db *sqlx.DB, dialect, scopeID string) error {
	definition := g61Table(dialect, "sys_notification_template_definition")
	revision := g61Table(dialect, "sys_notification_template_revision")
	audit := g61Table(dialect, "sys_notification_template_revision_audit")
	scopeColumn := g61Column(dialect, "scopeId")
	definitionID := g61Column(dialect, "templateDefinitionId")
	id := g61Column(dialect, "id")
	if _, err := db.ExecContext(ctx, db.Rebind("DELETE FROM "+audit+" WHERE "+scopeColumn+"=?"), scopeID); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, db.Rebind("DELETE FROM "+revision+" WHERE "+definitionID+" IN (SELECT "+id+" FROM "+definition+" WHERE "+scopeColumn+"=?)"), scopeID); err != nil {
		return err
	}
	if _, err := db.ExecContext(ctx, db.Rebind("DELETE FROM "+definition+" WHERE "+scopeColumn+"=?"), scopeID); err != nil {
		return err
	}
	return nil
}

func g61Table(dialect, name string) string {
	if dialect == "postgres" {
		return `"` + name + `"`
	}
	return name
}

func g61Column(dialect, name string) string {
	if dialect == "postgres" {
		return `"` + name + `"`
	}
	return name
}

func g61ValidateMySQLDSN(dsn string) error {
	parsed, err := mysqldriver.ParseDSN(dsn)
	if err != nil {
		return fmt.Errorf("parse MySQL DSN: %w", err)
	}
	if !strings.EqualFold(strings.TrimSpace(parsed.DBName), g61IsolatedDatabase) {
		return fmt.Errorf("G6.1 requires database %q", g61IsolatedDatabase)
	}
	return nil
}

func g61ValidatePostgresDSN(dsn string) error {
	parsed, err := url.Parse(dsn)
	if err != nil || parsed == nil || strings.TrimSpace(parsed.Scheme) == "" {
		return fmt.Errorf("G6.1 PostgreSQL DSN must be a parseable URL")
	}
	if !strings.EqualFold(strings.Trim(strings.TrimSpace(parsed.Path), "/"), g61IsolatedDatabase) {
		return fmt.Errorf("G6.1 requires database %q", g61IsolatedDatabase)
	}
	return nil
}
