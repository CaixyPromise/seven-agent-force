package infrastructure

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type integrationTestIDGen struct{ next int64 }

func (g *integrationTestIDGen) NextID() int64 { g.next++; return g.next }

type integrationApplicationGuard struct{}

func (integrationApplicationGuard) ValidateApplicationForSubmission(ctx context.Context, tx pgx.Tx, tenantID, applicationID int64, creatorUserID int64) error {
	var status string
	var boundCreatorUserID int64
	err := tx.QueryRow(ctx, `SELECT status,created_by FROM workforce_integration_applications WHERE id=$1 AND tenant_id=$2 FOR UPDATE`, applicationID, tenantID).Scan(&status, &boundCreatorUserID)
	if err != nil {
		return err
	}
	if status != "ENABLED" {
		return errors.New("application disabled")
	}
	if boundCreatorUserID != creatorUserID {
		return errors.New("application creator mismatch")
	}
	return nil
}

type coordinatedIntegrationApplicationGuard struct {
	entered chan struct{}
	proceed chan struct{}
}

func (g coordinatedIntegrationApplicationGuard) ValidateApplicationForSubmission(ctx context.Context, tx pgx.Tx, tenantID, applicationID int64, creatorUserID int64) error {
	close(g.entered)
	<-g.proceed
	return integrationApplicationGuard{}.ValidateApplicationForSubmission(ctx, tx, tenantID, applicationID, creatorUserID)
}

func TestIdempotencyLookupRevalidatesApplicationAfterConcurrentDisable(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("OPEN_INTEGRATION_TEST_POSTGRES_DSN"))
	if dsn == "" {
		t.Skip("set OPEN_INTEGRATION_TEST_POSTGRES_DSN to an isolated migrated PostgreSQL database")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	tenantID, applicationID := int64(95001), int64(95002)
	if err = insertOpenSubmissionApplication(ctx, db, tenantID, applicationID, "disable race", "credential-disable-race"); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = db.Exec(context.Background(), `DELETE FROM workforce_integration_api_credentials WHERE integration_application_id=$1`, applicationID)
		_, _ = db.Exec(context.Background(), `DELETE FROM workforce_integration_signing_credentials WHERE integration_application_id=$1`, applicationID)
		_, _ = db.Exec(context.Background(), `DELETE FROM workforce_integration_applications WHERE id=$1`, applicationID)
	}()

	entered, proceed := make(chan struct{}), make(chan struct{})
	store := NewStore(db, &integrationTestIDGen{next: 95100})
	store.SetSubmissionGuard(coordinatedIntegrationApplicationGuard{entered: entered, proceed: proceed})
	lookup := make(chan error, 1)
	go func() {
		_, _, _, err := store.FindIdempotent(ctx, tenantID, 101, "open:disabled", strings.Repeat("a", 64), applicationID)
		lookup <- err
	}()
	<-entered
	if _, err := db.Exec(ctx, `UPDATE workforce_integration_applications SET status='DISABLED' WHERE id=$1`, applicationID); err != nil {
		t.Fatal(err)
	}
	close(proceed)
	if err := <-lookup; err == nil || !strings.Contains(err.Error(), "disabled") {
		t.Fatalf("idempotency lookup bypassed the concurrent disable: %v", err)
	}
}

func TestConcurrentOpenSubmissionCreatesOneTaskGraph(t *testing.T) {
	// Reuse the current Project/model/Runtime transaction fixture rather than
	// bypassing the mandatory execution-selection contract with a legacy task.
	f := newSelectionFixture(t)
	ctx, db, store := f.ctx, f.db, f.store
	tenantID, applicationID := f.params.TenantID, f.params.TenantID+50
	if err := insertOpenSubmissionApplication(ctx, db, tenantID, applicationID, "concurrency test", "credential-concurrency-test"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = db.Exec(context.Background(), `DELETE FROM workforce_integration_api_credentials WHERE integration_application_id=$1`, applicationID)
		_, _ = db.Exec(context.Background(), `DELETE FROM workforce_integration_signing_credentials WHERE integration_application_id=$1`, applicationID)
		_, _ = db.Exec(context.Background(), `DELETE FROM workforce_integration_applications WHERE id=$1`, applicationID)
	})
	params := f.params
	params.CreatedBy = 101
	params.SubmissionChannel = "OPEN_REST"
	params.IntegrationApplicationID = applicationID
	params.ExternalCorrelationID = "concurrent-1"
	store.SetSubmissionGuard(integrationApplicationGuard{})
	type outcome struct {
		taskID   int64
		replayed bool
		err      error
	}
	start := make(chan struct{})
	results := make(chan outcome, 2)
	var ready sync.WaitGroup
	ready.Add(2)
	for range 2 {
		go func() {
			ready.Done()
			<-start
			task, replayed, err := store.Create(ctx, params)
			results <- outcome{taskID: task.ID, replayed: replayed, err: err}
		}()
	}
	ready.Wait()
	close(start)
	first, second := <-results, <-results
	if first.err != nil || second.err != nil {
		t.Fatalf("concurrent submissions failed: first=%v second=%v", first.err, second.err)
	}
	if first.taskID <= 0 || first.taskID != second.taskID || first.replayed == second.replayed {
		t.Fatalf("unexpected outcomes: first=%+v second=%+v", first, second)
	}
	taskID := first.taskID

	for table, query := range map[string]string{
		"task": `SELECT COUNT(*) FROM workforce_tasks WHERE id=$1`,
		"turn": `SELECT COUNT(*) FROM workforce_task_turns WHERE task_id=$1`,
		"node": `SELECT COUNT(*) FROM workforce_execution_nodes WHERE task_id=$1`,
	} {
		var count int
		if err := db.QueryRow(ctx, query, taskID).Scan(&count); err != nil || count != 1 {
			t.Fatalf("%s graph count=%d err=%v", table, count, err)
		}
	}
	var taskCreatorUserID, turnCreatorUserID int64
	var taskApplicationID int64
	if err := db.QueryRow(ctx, `SELECT created_by,integration_application_id FROM workforce_tasks WHERE id=$1`, taskID).Scan(&taskCreatorUserID, &taskApplicationID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, `SELECT created_by FROM workforce_task_turns WHERE task_id=$1 AND turn_number=1`, taskID).Scan(&turnCreatorUserID); err != nil {
		t.Fatal(err)
	}
	if taskCreatorUserID != 101 || turnCreatorUserID != 101 || taskApplicationID != applicationID {
		t.Fatalf("Open Task lost dual creator identity: taskUser=%d turnUser=%d application=%d", taskCreatorUserID, turnCreatorUserID, taskApplicationID)
	}
	if _, _, _, err := store.FindIdempotent(ctx, tenantID, 102, params.IdempotencyKey, params.RequestHash, applicationID); err == nil || !strings.Contains(err.Error(), "creator mismatch") {
		t.Fatalf("application owner mismatch was not rejected: %v", err)
	}

	conflicting := params
	conflicting.RequestHash = strings.Repeat("d", 64)
	if _, _, err := store.Create(ctx, conflicting); !errors.Is(err, ErrIdempotencyConflict) {
		t.Fatalf("same key with different hash did not conflict: %v", err)
	}
}

func insertOpenSubmissionApplication(ctx context.Context, db *pgxpool.Pool, tenantID, applicationID int64, name, locator string) error {
	if _, err := db.Exec(ctx, `INSERT INTO workforce_integration_applications
(id,tenant_id,name,status,webhook_url,created_by) VALUES ($1,$2,$3,'ENABLED','https://example.com/hook',$4)`, applicationID, tenantID, name, int64(101)); err != nil {
		return err
	}
	if _, err := db.Exec(ctx, `INSERT INTO workforce_integration_api_credentials
(id,tenant_id,integration_application_id,credential_locator,credential_digest,credential_hint,version,status,created_by)
VALUES ($1,$2,$3,$4,'digest','hint',1,'CURRENT',$5)`, applicationID+100_000, tenantID, applicationID, locator, int64(101)); err != nil {
		return err
	}
	_, err := db.Exec(ctx, `INSERT INTO workforce_integration_signing_credentials
(id,tenant_id,integration_application_id,signing_secret_ciphertext,signing_secret_edek,signing_secret_wrap_key_ref,signing_secret_hint,version,status,created_by)
VALUES ($1,$2,$3,'ciphertext','edek','key','hint',1,'CURRENT',$4)`, applicationID+200_000, tenantID, applicationID, int64(101))
	return err
}

func insertOpenSubmissionHarness(ctx context.Context, db *pgxpool.Pool, releaseID int64, alias string) error {
	digest := strings.Repeat("e", 64)
	evidence := `{"probe":{"resultProcessor":{"pythonRuntimeIdentity":"` + digest + `","bootstrapSha256":"` + digest + `","sdkSha256":"` + digest + `","moduleIndexSha256":"` + digest + `"}}}`
	if _, err := db.Exec(ctx, `INSERT INTO workforce_harness_releases
(id,alias,image_ref,image_digest,contract_version,supervisor_version,harness_type,harness_version,capabilities_json,admission_evidence_json,status)
VALUES ($1,$2,'fixture:latest',$3,'v1','v1','fake','v1','[]'::jsonb,$4::jsonb,'ACTIVE')`, releaseID, alias, digest, evidence); err != nil {
		return err
	}
	_, err := db.Exec(ctx, `INSERT INTO workforce_harness_channels(alias,harness_release_id) VALUES ($1,$2)`, alias, releaseID)
	return err
}

func cleanupOpenSubmissionTest(ctx context.Context, db *pgxpool.Pool, taskID, applicationID int64, harnessAlias string, harnessReleaseID int64) {
	if taskID > 0 {
		for _, query := range []string{
			`DELETE FROM workforce_task_notification_records WHERE task_id=$1`,
			`DELETE FROM workforce_task_notification_subscriptions WHERE task_id=$1`,
			`DELETE FROM workforce_execution_nodes WHERE task_id=$1`,
			`DELETE FROM workforce_task_events WHERE task_id=$1`,
			`DELETE FROM workforce_task_git_plans WHERE task_id=$1`,
			`DELETE FROM workforce_task_input_references WHERE task_input_snapshot_id IN (SELECT id FROM workforce_task_input_snapshots WHERE task_id=$1)`,
			`DELETE FROM workforce_task_input_snapshots WHERE task_id=$1`,
			`DELETE FROM workforce_task_turns WHERE task_id=$1`,
			`DELETE FROM workforce_task_execution_snapshots WHERE task_id=$1`,
			`DELETE FROM workforce_tasks WHERE id=$1`,
		} {
			_, _ = db.Exec(ctx, query, taskID)
		}
	}
	_, _ = db.Exec(ctx, `DELETE FROM workforce_integration_api_credentials WHERE integration_application_id=$1`, applicationID)
	_, _ = db.Exec(ctx, `DELETE FROM workforce_integration_signing_credentials WHERE integration_application_id=$1`, applicationID)
	_, _ = db.Exec(ctx, `DELETE FROM workforce_integration_applications WHERE id=$1`, applicationID)
	_, _ = db.Exec(ctx, `DELETE FROM workforce_harness_channels WHERE alias=$1`, harnessAlias)
	_, _ = db.Exec(ctx, `DELETE FROM workforce_harness_releases WHERE id=$1`, harnessReleaseID)
}
