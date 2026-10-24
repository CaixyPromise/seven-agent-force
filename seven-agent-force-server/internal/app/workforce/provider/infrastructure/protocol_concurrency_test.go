package infrastructure

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/provider/application"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/provider/domain"
)

func protocolTestPool(t *testing.T) (*pgxpool.Pool, context.Context) {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv("WORKFORCE_CONSISTENCY_TEST_POSTGRES_DSN"))
	if dsn == "" {
		t.Skip("set WORKFORCE_CONSISTENCY_TEST_POSTGRES_DSN to an isolated PostgreSQL database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["application_name"] = t.Name()
	db, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	return db, ctx
}

func seedProtocolCredential(t *testing.T, ctx context.Context, db *pgxpool.Pool) domain.Credential {
	t.Helper()
	id := time.Now().UnixNano()
	item, err := NewStore(db).Create(ctx, domain.Credential{
		ID: id, TenantID: id, Name: "protocol-test", Protocol: domain.ProtocolOpenAI,
		BaseURL: "https://models.example/v1", Models: []string{"primary"},
		SecretCiphertext: "YQ==", SecretEDEK: "Yg==", SecretWrapKeyRef: "test-key", Status: domain.StatusActive,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(context.Background(), `DELETE FROM workforce_tasks WHERE tenant_id=$1`, id)
		_, _ = db.Exec(context.Background(), `DELETE FROM workforce_model_credentials WHERE id=$1`, id)
	})
	return item
}

// Observe the PostgreSQL lock itself, rather than guessing transaction order
// from sleeps. The timeout belongs to the test context.
func waitProtocolLock(t *testing.T, ctx context.Context, db *pgxpool.Pool, result <-chan error) {
	t.Helper()
	for {
		select {
		case err := <-result:
			t.Fatalf("update finished before waiting for the credential lock: %v", err)
		case <-ctx.Done():
			t.Fatal(ctx.Err())
		default:
		}
		var waiting bool
		if err := db.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE application_name=$1 AND wait_event_type='Lock')`, t.Name()).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			return
		}
	}
}

func TestProtocolUpdateChecksUsesAfterTaskLockReleased(t *testing.T) {
	db, ctx := protocolTestPool(t)
	item := seedProtocolCredential(t, ctx, db)
	store := NewStore(db)
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if _, err = store.GetTenantForShare(ctx, tx, item.TenantID, item.ID); err != nil {
		t.Fatal(err)
	}
	service := application.NewService(store, nil, nil)
	result := make(chan error, 1)
	go func() {
		_, err := service.Update(ctx, item.TenantID, item.ID, application.UpdateCommand{
			ExpectedVersion: item.Version, Name: item.Name, Protocol: domain.ProtocolAnthropic, BaseURL: item.BaseURL, Models: item.Models,
		})
		result <- err
	}()
	waitProtocolLock(t, ctx, db, result)
	if _, err = tx.Exec(ctx, `INSERT INTO workforce_tasks(id,tenant_id,created_by,idempotency_key,request_hash,repository_url,repository_ref,task_instruction,status,result_processor_release_id,project_id,project_version_id,execution_requirements_json,execution_requirements_digest,model_credential_id,primary_model,selected_runtime_release_id)
VALUES($1,$2,$2,'protocol-race','test','','','test','QUEUED',1,1,1,'{}',$3,$2,'primary',1)`, item.ID+1, item.ID, strings.Repeat("a", 64)); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err = <-result; !errors.Is(err, application.ErrProtocolInUse) {
		t.Fatalf("protocol update bypassed the newly committed Task: %v", err)
	}
	current, err := store.GetTenant(ctx, item.TenantID, item.ID)
	if err != nil || current.Protocol != domain.ProtocolOpenAI {
		t.Fatalf("protocol=%s err=%v", current.Protocol, err)
	}
}

func TestCredentialUpdateRechecksVersionAfterLockWait(t *testing.T) {
	db, ctx := protocolTestPool(t)
	item := seedProtocolCredential(t, ctx, db)
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(ctx, `UPDATE workforce_model_credentials SET name='first-edit',version=version+1 WHERE id=$1`, item.ID); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() {
		_, err := application.NewService(NewStore(db), nil, nil).Update(ctx, item.TenantID, item.ID, application.UpdateCommand{ExpectedVersion: 1, Name: "second-edit", Protocol: item.Protocol, BaseURL: item.BaseURL, Models: item.Models})
		result <- err
	}()
	waitProtocolLock(t, ctx, db, result)
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err = <-result; !errors.Is(err, application.ErrVersionConflict) {
		t.Fatalf("version check error=%v", err)
	}
	current, err := NewStore(db).GetTenant(ctx, item.TenantID, item.ID)
	if err != nil || current.Name != "first-edit" || current.Version != 2 {
		t.Fatalf("name=%s version=%d err=%v", current.Name, current.Version, err)
	}
}
