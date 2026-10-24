package infrastructure

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	routingfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/executionrouting/facade"
	projectfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/project/facade"
	providerapp "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/provider/application"
	providerdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/provider/domain"
	providerfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/provider/facade"
	providerinfra "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/provider/infrastructure"
	processorfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/facade"
	notificationdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/tasknotification/domain"
	workspaceinputdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/workspaceinput/domain"
	workspaceinputfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/workspaceinput/facade"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type selectionRuntime struct{}

func (selectionRuntime) ValidateTaskSelection(context.Context, int64, int64, int64, string, projectfacade.ExecutionRequirements) error {
	return nil
}
func (selectionRuntime) ListTaskOptions(context.Context, int64, int64, int64) (routingfacade.RuntimeOptions, error) {
	return routingfacade.RuntimeOptions{}, nil
}
func (selectionRuntime) ValidateTaskSelectionInTransaction(_ context.Context, _ pgx.Tx, _, _, _ int64, protocol string, requirements projectfacade.ExecutionRequirements) error {
	if protocol != providerdomain.ProtocolOpenAI || len(requirements.AdapterCapabilities) != 1 || requirements.AdapterCapabilities[0] != "execution.resume" {
		return routingfacade.ErrRuntimeSelectionIncompatible
	}
	return nil
}

type selectionSubscriptions struct{ fail bool }

func (s selectionSubscriptions) CreateSubscription(context.Context, pgx.Tx, int64, int64, int64, notificationdomain.SubscriptionCommand) (notificationdomain.Subscription, bool, error) {
	if s.fail {
		return notificationdomain.Subscription{}, false, errors.New("test subscription failure")
	}
	return notificationdomain.Subscription{}, false, nil
}

type selectionBarrier struct {
	providerfacade.TaskSelectionValidator
	entered chan struct{}
	proceed chan struct{}
}

func (b selectionBarrier) ValidateTaskSelectionInTransaction(ctx context.Context, tx pgx.Tx, tenant, id int64, model string) (providerdomain.RuntimeSelection, error) {
	value, err := b.TaskSelectionValidator.ValidateTaskSelectionInTransaction(ctx, tx, tenant, id, model)
	if err != nil {
		return value, err
	}
	close(b.entered)
	select {
	case <-b.proceed:
	case <-ctx.Done():
		return value, ctx.Err()
	}
	return value, nil
}

type selectionFixture struct {
	db              *pgxpool.Pool
	ctx             context.Context
	store           *Store
	credential      *providerapp.Service
	credentialStore *providerinfra.Store
	params          CreateParams
}

type updateCommitBarrier struct {
	providerapp.Repository
	entered, proceed chan struct{}
}

func (b updateCommitBarrier) WithLockedCredential(ctx context.Context, tenant, id int64, fn func(providerdomain.Credential, providerapp.CredentialUpdateRepository) error) error {
	return b.Repository.WithLockedCredential(ctx, tenant, id, func(item providerdomain.Credential, repository providerapp.CredentialUpdateRepository) error {
		if err := fn(item, repository); err != nil {
			return err
		}
		close(b.entered)
		select {
		case <-b.proceed:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
}

func newSelectionFixture(t *testing.T) selectionFixture {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("WORKFORCE_CONSISTENCY_TEST_POSTGRES_DSN"))
	if dsn == "" {
		t.Skip("set WORKFORCE_CONSISTENCY_TEST_POSTGRES_DSN to an isolated PostgreSQL database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	t.Cleanup(cancel)
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["application_name"] = selectionApplicationName(t)
	db, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	id := time.Now().UnixNano()
	credentialStore := providerinfra.NewStore(db)
	_, err = credentialStore.Create(ctx, providerdomain.Credential{ID: id, TenantID: id, Name: "selection-test", Protocol: "OPENAI", BaseURL: "https://models.example/v1", Models: []string{"primary"}, SecretCiphertext: "YQ==", SecretEDEK: "Yg==", SecretWrapKeyRef: "test", Status: "ACTIVE"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(ctx, `INSERT INTO workforce_projects(id,tenant_id,project_key,current_version_id,created_by_principal_id,updated_by_principal_id,create_idempotency_key,create_request_digest) VALUES($1,$1,'selection-test',$2,$1,$1,'test',$3)`, id, id+1, strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup := context.Background()
		for _, table := range []string{"workforce_task_events", "workforce_execution_nodes", "workforce_task_git_plans", "workforce_task_input_snapshots", "workforce_task_turns"} {
			_, _ = db.Exec(cleanup, `DELETE FROM `+table+` WHERE task_id IN(SELECT id FROM workforce_tasks WHERE tenant_id=$1)`, id)
		}
		_, _ = db.Exec(cleanup, `DELETE FROM workforce_tasks WHERE tenant_id=$1`, id)
		_, _ = db.Exec(cleanup, `DELETE FROM workforce_projects WHERE id=$1`, id)
		_, _ = db.Exec(cleanup, `DELETE FROM workforce_model_credentials WHERE id=$1`, id)
	})
	credential := providerapp.NewService(credentialStore, nil, nil)
	store := NewStore(db, &integrationTestIDGen{next: id + 100})
	store.SetModelSelectionValidator(credential)
	store.SetRuntimeCatalog(selectionRuntime{})
	store.SetNotificationSubscriptions(selectionSubscriptions{})
	snapshot, err := workspaceinputdomain.FreezeEmptyTaskInput(id, nil, workspaceinputdomain.GitPlan{DeliveryMode: "NONE"})
	if err != nil {
		t.Fatal(err)
	}
	params := CreateParams{TenantID: id, CreatedBy: id, IdempotencyKey: "test", RequestHash: strings.Repeat("b", 64), Instruction: "test", Name: "test", Tags: []string{}, ModelCredentialID: id, PrimaryModel: "primary", ModelProtocol: "OPENAI", SelectedRuntimeReleaseID: id + 2, ExecutionTimeoutSeconds: 600,
		ProjectSpec:              &projectfacade.FrozenTaskSpec{ProjectID: id, ProjectVersionID: id + 1, ExecutionRequirementsDigest: strings.Repeat("c", 64), ExecutionRequirements: projectfacade.ExecutionRequirements{ExecutionKind: "AGENT_ATTEMPT", AdapterCapabilities: []string{"execution.resume"}}},
		TaskInput:                workspaceinputfacade.FrozenTaskInput{Snapshot: snapshot},
		ResultProcessorExecution: processorfacade.Execution{ReleaseID: 1, Digest: strings.Repeat("d", 64), Environment: json.RawMessage(`{}`), Parameters: json.RawMessage(`{}`), SecretVersionIdentities: json.RawMessage(`[]`), ContinuationPolicy: "NONE", DeliveryPolicy: "NONE"},
	}
	return selectionFixture{db, ctx, store, credential, credentialStore, params}
}

func selectionApplicationName(t *testing.T) string {
	return fmt.Sprintf("selection-%x", sha256.Sum256([]byte(t.Name())))[:50]
}

func waitSelectionLock(t *testing.T, f selectionFixture, result <-chan error) {
	t.Helper()
	for {
		select {
		case err := <-result:
			t.Fatalf("operation did not wait for credential lock: %v", err)
		case <-f.ctx.Done():
			t.Fatal(f.ctx.Err())
		default:
		}
		var waiting bool
		if err := f.db.QueryRow(f.ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE application_name=$1 AND wait_event_type='Lock')`, selectionApplicationName(t)).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			return
		}
	}
}

func TestTaskCreationPersistsUnlimitedAndCustomExecutionBudget(t *testing.T) {
	for _, tc := range []struct {
		name    string
		seconds int
	}{
		{name: "unlimited", seconds: 0},
		{name: "custom", seconds: 7200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newSelectionFixture(t)
			f.params.ExecutionTimeoutSeconds = tc.seconds
			created, replayed, err := f.store.Create(f.ctx, f.params)
			if err != nil || replayed {
				t.Fatalf("create task: replayed=%v err=%v", replayed, err)
			}
			var stored *int32
			if err := f.db.QueryRow(f.ctx, `SELECT execution_timeout_seconds FROM workforce_tasks WHERE id=$1`, created.ID).Scan(&stored); err != nil {
				t.Fatal(err)
			}
			if tc.seconds == 0 && stored != nil {
				t.Fatalf("unlimited task has timeout %d", *stored)
			}
			if tc.seconds > 0 && (stored == nil || *stored != int32(tc.seconds)) {
				t.Fatalf("custom task timeout=%v, want %d", stored, tc.seconds)
			}
		})
	}
}

func TestTaskCreationHoldsCredentialLockThroughCommit(t *testing.T) {
	f := newSelectionFixture(t)
	entered, proceed := make(chan struct{}), make(chan struct{})
	f.store.SetModelSelectionValidator(selectionBarrier{f.credential, entered, proceed})
	created := make(chan error, 1)
	go func() { _, _, err := f.store.Create(f.ctx, f.params); created <- err }()
	select {
	case <-entered:
	case <-f.ctx.Done():
		t.Fatal(f.ctx.Err())
	}
	updated := make(chan error, 1)
	go func() {
		_, err := f.credential.Update(f.ctx, f.params.TenantID, f.params.ModelCredentialID, providerapp.UpdateCommand{ExpectedVersion: 1, Name: "selection-test", Protocol: "ANTHROPIC", BaseURL: "https://models.example/v1", Models: []string{"primary"}})
		updated <- err
	}()
	waitSelectionLock(t, f, updated)
	close(proceed)
	if err := <-created; err != nil {
		t.Fatal(err)
	}
	if err := <-updated; !errors.Is(err, providerapp.ErrProtocolInUse) {
		t.Fatalf("update error=%v", err)
	}
}

func TestTaskCreationWaitsForProtocolUpdateCommit(t *testing.T) {
	f := newSelectionFixture(t)
	entered, proceed := make(chan struct{}), make(chan struct{})
	updater := providerapp.NewService(updateCommitBarrier{f.credentialStore, entered, proceed}, nil, nil)
	updated := make(chan error, 1)
	go func() {
		_, err := updater.Update(f.ctx, f.params.TenantID, f.params.ModelCredentialID, providerapp.UpdateCommand{ExpectedVersion: 1, Name: "selection-test", Protocol: "ANTHROPIC", BaseURL: "https://models.example/v1", Models: []string{"primary"}})
		updated <- err
	}()
	select {
	case <-entered:
	case <-f.ctx.Done():
		t.Fatal(f.ctx.Err())
	}
	created := make(chan error, 1)
	go func() { _, _, err := f.store.Create(f.ctx, f.params); created <- err }()
	waitSelectionLock(t, f, created)
	close(proceed)
	if err := <-updated; err != nil {
		t.Fatal(err)
	}
	if err := <-created; !errors.Is(err, routingfacade.ErrRuntimeSelectionIncompatible) {
		t.Fatalf("task creation used stale protocol: %v", err)
	}
}

func TestTaskSelectionRevalidatedAfterConcurrentCredentialEdit(t *testing.T) {
	for _, edit := range []string{"protocol", "disabled", "catalog", "name", "tenant", "deleted"} {
		t.Run(edit, func(t *testing.T) {
			f := newSelectionFixture(t)
			tx, err := f.db.Begin(f.ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback(context.Background())
			var query string
			switch edit {
			case "protocol":
				query = `UPDATE workforce_model_credentials SET protocol='ANTHROPIC' WHERE id=$1`
			case "disabled":
				query = `UPDATE workforce_model_credentials SET status='DISABLED' WHERE id=$1`
			case "catalog":
				query = `UPDATE workforce_model_credentials SET models_json='[]' WHERE id=$1`
			case "name":
				query = `UPDATE workforce_model_credentials SET name='renamed',secret_version=2,version=2 WHERE id=$1`
			case "tenant":
				foreign := f.params.ModelCredentialID + 20
				_, err = f.credentialStore.Create(f.ctx, providerdomain.Credential{ID: foreign, TenantID: foreign, Name: "foreign", Protocol: "OPENAI", BaseURL: "https://models.example/v1", Models: []string{"primary"}, SecretCiphertext: "YQ==", SecretEDEK: "Yg==", SecretWrapKeyRef: "test", Status: "ACTIVE"})
				if err != nil {
					t.Fatal(err)
				}
				defer f.db.Exec(context.Background(), `DELETE FROM workforce_model_credentials WHERE id=$1`, foreign)
				f.params.ModelCredentialID = foreign
				_, _, err = f.store.Create(f.ctx, f.params)
				if !errors.Is(err, providerapp.ErrNotFound) {
					t.Fatalf("cross tenant create error=%v", err)
				}
				return
			case "deleted":
				query = `DELETE FROM workforce_model_credentials WHERE id=$1`
			}
			if _, err = tx.Exec(f.ctx, query, f.params.ModelCredentialID); err != nil {
				t.Fatal(err)
			}
			created := make(chan error, 1)
			go func() { _, _, err := f.store.Create(f.ctx, f.params); created <- err }()
			waitSelectionLock(t, f, created)
			if err = tx.Commit(f.ctx); err != nil {
				t.Fatal(err)
			}
			err = <-created
			want := map[string]error{"protocol": routingfacade.ErrRuntimeSelectionIncompatible, "disabled": providerapp.ErrDisabled, "catalog": providerapp.ErrModelNotInCatalog, "deleted": providerapp.ErrNotFound}[edit]
			if !errors.Is(err, want) {
				t.Fatalf("error=%v want=%v", err, want)
			}
			if edit != "name" {
				var count int
				if err = f.db.QueryRow(f.ctx, `SELECT COUNT(*) FROM workforce_tasks WHERE tenant_id=$1`, f.params.TenantID).Scan(&count); err != nil || count != 0 {
					t.Fatalf("tasks=%d err=%v", count, err)
				}
			}
		})
	}
}

func TestTaskSelectionRollbackAndReplay(t *testing.T) {
	f := newSelectionFixture(t)
	f.store.SetNotificationSubscriptions(selectionSubscriptions{fail: true})
	if _, _, err := f.store.Create(f.ctx, f.params); err == nil {
		t.Fatal("expected rollback")
	}
	for _, table := range []string{"workforce_tasks", "workforce_task_turns", "workforce_task_input_snapshots", "workforce_task_git_plans"} {
		var count int
		if err := f.db.QueryRow(f.ctx, `SELECT COUNT(*) FROM `+table+` WHERE tenant_id=$1`, f.params.TenantID).Scan(&count); err != nil || count != 0 {
			t.Fatalf("%s count=%d err=%v", table, count, err)
		}
	}
	for _, table := range []string{"workforce_task_events", "workforce_execution_nodes"} {
		var count int
		if err := f.db.QueryRow(f.ctx, `SELECT COUNT(*) FROM `+table+` WHERE task_id BETWEEN $1 AND $2`, f.params.TenantID+100, f.params.TenantID+1000).Scan(&count); err != nil || count != 0 {
			t.Fatalf("%s residue=%d err=%v", table, count, err)
		}
	}
	var outboxCount int
	if err := f.db.QueryRow(f.ctx, `SELECT COUNT(*) FROM workforce_outbox_events WHERE aggregate_id BETWEEN $1 AND $2`, f.params.TenantID+100, f.params.TenantID+1000).Scan(&outboxCount); err != nil || outboxCount != 0 {
		t.Fatalf("outbox residue=%d err=%v", outboxCount, err)
	}
	if _, err := f.credential.Update(f.ctx, f.params.TenantID, f.params.ModelCredentialID, providerapp.UpdateCommand{ExpectedVersion: 1, Name: "selection-test", Protocol: "ANTHROPIC", BaseURL: "https://models.example/v1", Models: []string{"primary"}}); err != nil {
		t.Fatalf("rollback did not release lock: %v", err)
	}
	if _, err := f.credential.Update(f.ctx, f.params.TenantID, f.params.ModelCredentialID, providerapp.UpdateCommand{ExpectedVersion: 2, Name: "selection-test", Protocol: "OPENAI", BaseURL: "https://models.example/v1", Models: []string{"primary"}}); err != nil {
		t.Fatal(err)
	}
	f.store.SetNotificationSubscriptions(selectionSubscriptions{})
	task, _, err := f.store.Create(f.ctx, f.params)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = f.credential.SetStatus(f.ctx, f.params.TenantID, f.params.ModelCredentialID, "DISABLED", 3); err != nil {
		t.Fatal(err)
	}
	replayed, replay, err := f.store.Create(f.ctx, f.params)
	if err != nil || !replay || task.ID != replayed.ID {
		t.Fatalf("replay=%v id=%d err=%v", replay, replayed.ID, err)
	}
	var count int
	if err = f.db.QueryRow(f.ctx, `SELECT COUNT(*) FROM workforce_tasks WHERE tenant_id=$1`, f.params.TenantID).Scan(&count); err != nil || count != 1 {
		t.Fatalf("idempotent task count=%d err=%v", count, err)
	}
	f.params.RequestHash = strings.Repeat("e", 64)
	if _, _, err = f.store.Create(f.ctx, f.params); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("changed request did not conflict: %v", err)
	}
}
