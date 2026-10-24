package infrastructure

import (
	"context"
	"errors"
	"fmt"
	"github.com/bwmarrin/snowflake"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/delivery/application"
	projectfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/project/facade"
	taskfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/facade"
	notificationfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/tasknotification/facade"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TEXT-only fixtures miss PostgreSQL's conflicting status parameter inference.
// Clone the migrated table definitions, never live rows or credential material.
func TestDeliveryMachineResultsAgainstMigratedStatusTypes(t *testing.T) {
	dsn := os.Getenv("DELIVERY_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("set DELIVERY_TEST_POSTGRES_DSN to an isolated migrated PostgreSQL database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	schema := fmt.Sprintf("delivery_status_%d", time.Now().UnixNano())
	quoted := pgx.Identifier{schema}.Sanitize()
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+quoted); err != nil {
		t.Fatal(err)
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		if _, err := admin.Exec(cleanup, "DROP SCHEMA "+quoted+" CASCADE"); err != nil {
			t.Error(err)
		}
	}()
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema + ",pg_catalog"
	db, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	for _, table := range []string{"workforce_delivery_requests", "workforce_delivery_attempts", "workforce_delivery_operations", "workforce_delivery_intents", "workforce_task_git_plans", "workforce_workers", "workforce_worker_sessions", "workforce_outbox_events", "workforce_task_events", "workforce_pull_request_publications", "workforce_principals", "workforce_delivery_decisions"} {
		if _, err = db.Exec(ctx, "CREATE TABLE "+table+" (LIKE public."+table+" INCLUDING ALL)"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = db.Exec(ctx, `ALTER TABLE workforce_delivery_requests ADD COLUMN IF NOT EXISTS reauthorized_project_version_id BIGINT`); err != nil {
		t.Fatal(err)
	}
	// Minimal related identity/result tables remain inside the private schema;
	// the Delivery tables above retain their real migrated constraints.
	if _, err = db.Exec(ctx, `CREATE TABLE sys_user(id BIGINT PRIMARY KEY);
CREATE TABLE workforce_tasks(id BIGINT PRIMARY KEY,tenant_id BIGINT,status TEXT,current_task_turn_id BIGINT);
CREATE TABLE workforce_continuation_checkpoints(id BIGINT PRIMARY KEY,task_id BIGINT,task_turn_id BIGINT,status TEXT,result_commit TEXT);`); err != nil {
		t.Fatal(err)
	}
	node, err := snowflake.NewNode(903)
	if err != nil {
		t.Fatal(err)
	}
	store := New(db, deliveryFixtureIDs{node})
	authority := application.MachineAuthority{WorkerID: 40, InstanceID: "fixture-instance", HostBootID: "fixture-boot", Generation: 1}
	commit := strings.Repeat("d", 40)
	reset := func(t *testing.T, reconciliationOnly bool) {
		t.Helper()
		_, err := db.Exec(ctx, `TRUNCATE workforce_delivery_decisions,sys_user,workforce_tasks,workforce_continuation_checkpoints,workforce_principals,workforce_delivery_attempts,workforce_delivery_requests,workforce_delivery_operations,workforce_delivery_intents,workforce_task_git_plans,workforce_workers,workforce_worker_sessions,workforce_outbox_events,workforce_task_events,workforce_pull_request_publications;
INSERT INTO workforce_delivery_requests (id,tenant_id,task_id,task_turn_id,checkpoint_id,action_type,action_digest,processor_release_id,processor_digest,result_commit,result_tree,selection_digest,project_id,target_config_digest,policy_digest,base_branch,destination_branch,expected_base_commit,delivery_commit,commit_timestamp,approval_mode,status,idempotency_key,request_digest,created_by_principal_id)
VALUES (10,1,20,21,22,'GIT_PUSH','fixture-action',23,'fixture-processor','fixture-result','fixture-tree','fixture-selection',24,'fixture-target','fixture-policy','main','codex/fixture','fixture-base',repeat('d',40),CURRENT_TIMESTAMP,'AFTER_APPROVAL','CONFIRMING','fixture-key','fixture-request',25);
INSERT INTO workforce_delivery_operations (id,tenant_id,delivery_request_id,operation_kind,status,idempotency_key,request_digest) VALUES (11,1,10,'PUSH_BRANCH','CONFIRMING','fixture-push','fixture-request'),(12,1,10,'CREATE_PULL_REQUEST','WAITING_DEPENDENCY','fixture-review','fixture-request')`)
		if err != nil {
			t.Fatal(err)
		}
		_, err = db.Exec(ctx, `INSERT INTO workforce_delivery_attempts (id,request_id,attempt_number,worker_id,worker_instance_id,host_boot_id,generation,lease_sha256,lease_expires_at,push_authorization_sha256,push_authorization_expires_at,phase,status,reconciliation_only) VALUES (30,10,1,40,'fixture-instance','fixture-boot',1,$1,CURRENT_TIMESTAMP+INTERVAL '1 minute',$2,CURRENT_TIMESTAMP+INTERVAL '1 minute','PREPARED','RUNNING',$3)`, secretHash("fixture-lease"), secretHash("fixture-authorization"), reconciliationOnly)
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, c := range []struct {
		name, method, want string
		readable           bool
		remote             string
	}{
		{"unreadable result", "result", "OUTCOME_UNKNOWN", false, ""},
		{"verified result", "result", "SUCCEEDED", true, commit},
		{"unchanged result", "result", "RETRY_WAIT", true, ""},
		{"reported failure", "failure", "RETRY_WAIT", false, ""},
		{"readback verified", "reconcile", "SUCCEEDED", true, commit},
		{"readback unchanged", "reconcile", "RETRY_WAIT", true, ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			reset(t, c.method == "reconcile")
			result := application.DeliveryResult{Readable: c.readable, RemoteAfter: c.remote}
			switch c.method {
			case "result":
				err = store.RecordResult(ctx, authority, 30, "fixture-lease", "fixture-authorization", result)
			case "failure":
				err = store.RecordFailure(ctx, authority, 30, "fixture-lease", "DELIVERY_WORKER_TRANSIENT")
			case "reconcile":
				err = store.RecordReconciliation(ctx, authority, 30, "fixture-lease", result)
			}
			if err != nil {
				t.Fatal(err)
			}
			var attempt, request, operation, review string
			err = db.QueryRow(ctx, `SELECT a.status,r.status,o.status,p.status FROM workforce_delivery_attempts a JOIN workforce_delivery_requests r ON r.id=a.request_id JOIN workforce_delivery_operations o ON o.delivery_request_id=r.id AND o.operation_kind='PUSH_BRANCH' JOIN workforce_delivery_operations p ON p.delivery_request_id=r.id AND p.operation_kind='CREATE_PULL_REQUEST' WHERE a.id=30`).Scan(&attempt, &request, &operation, &review)
			if err != nil || attempt != c.want || request != c.want || operation != c.want {
				t.Fatalf("states=%s/%s/%s want=%s err=%v", attempt, request, operation, c.want, err)
			}
			wantReview := "WAITING_DEPENDENCY"
			if c.want == "SUCCEEDED" {
				wantReview = "READY"
			}
			if review != wantReview {
				t.Fatalf("review=%s want=%s", review, wantReview)
			}
		})
	}
	execFixture := func(t *testing.T, query string) {
		t.Helper()
		if _, err := db.Exec(ctx, query); err != nil {
			t.Fatal(err)
		}
	}
	prepareExpired := func(t *testing.T) {
		reset(t, false)
		execFixture(t, `UPDATE workforce_delivery_attempts SET lease_expires_at=CURRENT_TIMESTAMP-INTERVAL '1 minute';
INSERT INTO workforce_task_git_plans (id,tenant_id,task_id,task_turn_id,project_id,repository_scheme,source_commit,delivery_mode,delivery_authorization,plan_digest) VALUES (13,1,20,21,24,'HTTPS','fixture-base','NEW_BRANCH_AND_DRAFT_REVIEW','ASK_AFTER_GATES',repeat('a',64));
INSERT INTO workforce_workers (id,name,current_instance_id,token_hint) VALUES (40,'fixture-worker','fixture-instance','fixture-not-a-token');
INSERT INTO workforce_worker_sessions (instance_id,worker_id,host_boot_id,started_at,last_heartbeat_at,consumer_state,capabilities_json) VALUES ('fixture-instance',40,'fixture-boot',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,'CONSUMING','{"gitDelivery":true}');`)
	}
	assertUnchanged := func(t *testing.T) {
		t.Helper()
		var status string
		var count int
		if err := db.QueryRow(ctx, `SELECT status FROM workforce_delivery_attempts WHERE id=30`).Scan(&status); err != nil || status != "RUNNING" {
			t.Fatalf("old attempt changed: %s %v", status, err)
		}
		if err := db.QueryRow(ctx, `SELECT count(*) FROM workforce_outbox_events`).Scan(&count); err != nil || count != 0 {
			t.Fatalf("unexpected dispatch: %d %v", count, err)
		}
	}
	t.Run("expired recovery is atomic readback only and concurrent safe", func(t *testing.T) {
		prepareExpired(t)
		var wg sync.WaitGroup
		outcomes := make(chan error, 2)
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func() { defer wg.Done(); _, err := store.Reconcile(ctx, 1, 7, 10); outcomes <- err }()
		}
		wg.Wait()
		close(outcomes)
		passed, denied := 0, 0
		for err := range outcomes {
			if err == nil {
				passed++
			} else if errors.Is(err, application.ErrReconciliationNotAllowed) {
				denied++
			} else {
				t.Fatal(err)
			}
		}
		if passed != 1 || denied != 1 {
			t.Fatalf("recovery results=%d/%d", passed, denied)
		}
		var oldStatus, requestStatus string
		var pending, pushPermits, outbox int
		if err := db.QueryRow(ctx, `SELECT status FROM workforce_delivery_attempts WHERE id=30`).Scan(&oldStatus); err != nil || oldStatus != "OUTCOME_UNKNOWN" {
			t.Fatalf("old=%s %v", oldStatus, err)
		}
		if err := db.QueryRow(ctx, `SELECT status FROM workforce_delivery_requests WHERE id=10`).Scan(&requestStatus); err != nil || requestStatus != "DISPATCHED" {
			t.Fatalf("request=%s %v", requestStatus, err)
		}
		if err := db.QueryRow(ctx, `SELECT count(*) FILTER (WHERE status='PENDING' AND reconciliation_only),count(*) FILTER (WHERE id<>30 AND push_authorization_sha256<>'' ) FROM workforce_delivery_attempts`).Scan(&pending, &pushPermits); err != nil || pending != 1 || pushPermits != 0 {
			t.Fatalf("readback=%d pushpermits=%d err=%v", pending, pushPermits, err)
		}
		if err := db.QueryRow(ctx, `SELECT count(*) FROM workforce_outbox_events`).Scan(&outbox); err != nil || outbox != 1 {
			t.Fatalf("outbox=%d %v", outbox, err)
		}
		if err := store.RecordResult(ctx, authority, 30, "fixture-lease", "fixture-authorization", application.DeliveryResult{Readable: true, RemoteAfter: commit}); err == nil || err.Error() != "DELIVERY_ATTEMPT_NOT_RUNNING" {
			t.Fatalf("old owner not fenced: %v", err)
		}
	})
	t.Run("live lease refuses recovery", func(t *testing.T) {
		prepareExpired(t)
		execFixture(t, `UPDATE workforce_delivery_attempts SET lease_expires_at=CURRENT_TIMESTAMP+INTERVAL '1 minute'`)
		if _, err := store.Reconcile(ctx, 1, 7, 10); !errors.Is(err, application.ErrReconciliationNotAllowed) {
			t.Fatal(err)
		}
		assertUnchanged(t)
	})
	t.Run("cross tenant refuses recovery", func(t *testing.T) {
		prepareExpired(t)
		if _, err := store.Reconcile(ctx, 2, 7, 10); !errors.Is(err, application.ErrNotFound) {
			t.Fatal(err)
		}
		assertUnchanged(t)
	})
	t.Run("missing worker rolls back expiry fencing", func(t *testing.T) {
		prepareExpired(t)
		execFixture(t, `UPDATE workforce_worker_sessions SET consumer_state='OFFLINE'`)
		if _, err := store.Reconcile(ctx, 1, 7, 10); err == nil {
			t.Fatal("missing worker accepted")
		}
		assertUnchanged(t)
	})

	prepareExhausted := func(t *testing.T) {
		prepareExpired(t)
		execFixture(t, `INSERT INTO sys_user(id) VALUES(7);
INSERT INTO workforce_tasks(id,tenant_id,status,current_task_turn_id) VALUES(20,1,'SUCCEEDED',21);
INSERT INTO workforce_continuation_checkpoints(id,task_id,task_turn_id,status,result_commit) VALUES(22,20,21,'AVAILABLE','fixture-result');
UPDATE workforce_delivery_requests SET status='MANUAL_REVIEW',reason_code='DELIVERY_READBACK_RETRY_EXHAUSTED',target_config_digest=repeat('a',64),authorization_digest=repeat('b',64);
UPDATE workforce_task_git_plans SET target_config_digest=repeat('a',64),push_authorization_digest=repeat('b',64);
UPDATE workforce_delivery_attempts SET status='OUTCOME_UNKNOWN',reason_code='REMOTE_REF_UNAVAILABLE',phase='VERIFIED';
INSERT INTO workforce_delivery_attempts(id,request_id,attempt_number,worker_id,worker_instance_id,host_boot_id,status,reconciliation_only,reason_code) VALUES(31,10,2,40,'fixture-instance','fixture-boot','OUTCOME_UNKNOWN',true,'REMOTE_REF_UNAVAILABLE'),(32,10,3,40,'fixture-instance','fixture-boot','OUTCOME_UNKNOWN',true,'REMOTE_REF_UNAVAILABLE');`)
	}
	t.Run("reauthorization requires new approval then schedules readback only", func(t *testing.T) {
		prepareExhausted(t)
		recorder := &deliveryApprovalRecorder{}
		store.SetTaskNotificationRecorder(recorder)
		defer store.SetTaskNotificationRecorder(nil)
		r := application.Reauthorization{TenantID: 1, Actor: 7, RequestID: 10, Reason: "test transport repaired"}
		view, err := store.ReauthorizeRequest(ctx, r)
		if err != nil || view.Status != "WAITING_APPROVAL" {
			t.Fatalf("reauth=%+v err=%v", view, err)
		}
		if len(recorder.calls) != 1 || recorder.calls[0].SourceType != notificationfacade.SourceDeliveryRequest || recorder.calls[0].SourceID != 10 || recorder.calls[0].SourceOccurrence <= 0 || recorder.calls[0].ReasonCode != "DELIVERY_WAITING_APPROVAL" {
			t.Fatalf("wrong recovery notification identity: %+v", recorder.calls)
		}
		var budget, outbox, old int
		if err = db.QueryRow(ctx, `SELECT count(*) FROM workforce_outbox_events`).Scan(&outbox); err != nil || outbox != 0 {
			t.Fatalf("reauth dispatched without approval: %d %v", outbox, err)
		}
		if err = db.QueryRow(ctx, `SELECT count(*) FROM workforce_delivery_attempts WHERE status='OUTCOME_UNKNOWN'`).Scan(&old); err != nil || old != 3 {
			t.Fatalf("old history=%d %v", old, err)
		}
		if _, err = store.ReauthorizeRequest(ctx, r); err == nil {
			t.Fatal("duplicate reauthorization accepted")
		}
		if len(recorder.calls) != 1 {
			t.Fatal("duplicate reauthorization recreated approval notification")
		}
		view, err = store.Approve(ctx, application.Approval{TenantID: 1, Actor: 7, RequestID: 10, Reason: "approve frozen result"})
		if err != nil || view.Status != "DISPATCHED" {
			t.Fatalf("approve=%+v err=%v", view, err)
		}
		if err = db.QueryRow(ctx, `SELECT count(*) FROM workforce_delivery_attempts WHERE status='PENDING' AND reconciliation_only AND push_authorization_sha256=''`).Scan(&budget); err != nil || budget != 1 {
			t.Fatalf("first attempt is not readback only: %d %v", budget, err)
		}
		if err = db.QueryRow(ctx, `SELECT count(*) FROM workforce_task_events WHERE event_type='delivery.request_reauthorized'`).Scan(&budget); err != nil || budget != 1 {
			t.Fatalf("audit count=%d %v", budget, err)
		}
	})
	t.Run("reauthorized user decision is readback only and idempotent", func(t *testing.T) {
		prepareExhausted(t)
		view, err := store.ReauthorizeRequest(ctx, application.Reauthorization{TenantID: 1, Actor: 7, RequestID: 10, Reason: "test transport repaired"})
		if err != nil {
			t.Fatal(err)
		}
		d := application.Decision{TenantID: 1, TaskID: 20, RequestID: 10, Actor: 7, ExpectedVersion: view.Version, Action: "DELIVER", IdempotencyKey: "approve-new-epoch"}
		if _, _, err = store.Decide(ctx, d); err != nil {
			t.Fatal(err)
		}
		if _, replay, err := store.Decide(ctx, d); err != nil || !replay {
			t.Fatalf("replay=%v err=%v", replay, err)
		}
		var count int
		if err = db.QueryRow(ctx, `SELECT count(*) FROM workforce_delivery_attempts WHERE status='PENDING' AND reconciliation_only AND push_authorization_sha256=''`).Scan(&count); err != nil || count != 1 {
			t.Fatalf("readback count=%d %v", count, err)
		}
	})
	for _, c := range []struct {
		name, mutation string
		tenant         int64
	}{
		{"wrong tenant", "", 2},
		{"live attempt", "UPDATE workforce_delivery_attempts SET status='RUNNING' WHERE id=32", 1},
		{"superseded result", "UPDATE workforce_tasks SET current_task_turn_id=99", 1},
		{"unavailable checkpoint", "UPDATE workforce_continuation_checkpoints SET status='DELETED'", 1},
		{"changed frozen target", "UPDATE workforce_task_git_plans SET target_config_digest=repeat('c',64)", 1},
	} {
		t.Run("reauthorization refuses "+c.name, func(t *testing.T) {
			prepareExhausted(t)
			if c.mutation != "" {
				execFixture(t, c.mutation)
			}
			if _, err := store.ReauthorizeRequest(ctx, application.Reauthorization{TenantID: c.tenant, Actor: 7, RequestID: 10, Reason: "test repair"}); err == nil {
				t.Fatal("unsafe reauthorization accepted")
			}
			var status string
			var count int
			if err := db.QueryRow(ctx, `SELECT status FROM workforce_delivery_requests WHERE id=10`).Scan(&status); err != nil || status != "MANUAL_REVIEW" {
				t.Fatalf("request changed: %s %v", status, err)
			}
			if err := db.QueryRow(ctx, `SELECT count(*) FROM workforce_outbox_events`).Scan(&count); err != nil || count != 0 {
				t.Fatalf("unexpected dispatch: %d %v", count, err)
			}
		})
	}

	t.Run("explicit ProjectVersion binds materialization and review without changing original plan", func(t *testing.T) {
		prepareExhausted(t)
		original, b := deliveryBindingFixture()
		reader := &deliveryBindingReaderFixture{binding: b}
		store.SetFrozenRepositoryPlanReader(deliveryFrozenReaderFixture{original})
		store.SetDeliveryRepositoryBindingReader(reader)
		if _, err := store.ReauthorizeRequest(ctx, application.Reauthorization{TenantID: 1, Actor: 7, RequestID: 10, ProjectVersionID: 101, Reason: "replacement test credential verified"}); err != nil {
			t.Fatal(err)
		}
		plan, err := store.loadDeliveryRepositoryPlan(ctx, 1, 20, 21, 10)
		if err != nil || plan.Credential.Version != 2 {
			t.Fatalf("binding version=%d err=%v", plan.Credential.Version, err)
		}
		var bound int64
		var originalTarget string
		if err = db.QueryRow(ctx, `SELECT reauthorized_project_version_id FROM workforce_delivery_requests WHERE id=10`).Scan(&bound); err != nil || bound != 101 {
			t.Fatalf("reference=%d %v", bound, err)
		}
		if err = db.QueryRow(ctx, `SELECT target_config_digest FROM workforce_task_git_plans WHERE task_id=20`).Scan(&originalTarget); err != nil || originalTarget != original.TargetConfigDigest {
			t.Fatalf("original plan overwritten: %v", err)
		}
		if _, _, err = store.Decide(ctx, application.Decision{TenantID: 1, TaskID: 20, RequestID: 10, Actor: 7, ExpectedVersion: 2, Action: "DELIVER", IdempotencyKey: "bind-approve"}); err != nil {
			t.Fatal(err)
		}
		var id int64
		if err = db.QueryRow(ctx, `SELECT id FROM workforce_delivery_attempts WHERE status='PENDING' AND reconciliation_only`).Scan(&id); err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec(ctx, `UPDATE workforce_delivery_attempts SET status='RUNNING',lease_sha256=$2,lease_expires_at=CURRENT_TIMESTAMP+INTERVAL '1 minute' WHERE id=$1;`, id, secretHash("replacement-lease")); err != nil {
			t.Fatal(err)
		}
		execFixture(t, `UPDATE workforce_delivery_requests SET status='CONFIRMING' WHERE id=10`)
		envelope, err := store.ResolveCredentialEnvelope(ctx, authority, id, "replacement-lease")
		if err != nil || envelope.Version != 2 || envelope.Ciphertext != b.Ciphertext {
			t.Fatalf("materialization disagrees: %v", err)
		}
		var reviewVersion, reviewAuthorization string
		if err = db.QueryRow(ctx, `SELECT authorization_version,authorization_digest FROM workforce_delivery_operations WHERE operation_kind='CREATE_PULL_REQUEST'`).Scan(&reviewVersion, &reviewAuthorization); err != nil {
			t.Fatal(err)
		}
		work := application.ReviewWork{TenantID: 1, ProjectID: 24, CredentialVersion: reviewVersion, AuthorizationDigest: reviewAuthorization, ConnectionConfigDigest: plan.TargetConfigDigest}
		if err = applyFrozenReviewPlan(&work, plan); err != nil || work.CredentialEnvelope.Version != 2 {
			t.Fatalf("review disagrees: %v", err)
		}
		// Changing the current version never silently advances the approved binding.
		reader.binding.Current = false
		if plan, err = store.loadDeliveryRepositoryPlan(ctx, 1, 20, 21, 10); err != nil || plan.Credential.Version != 2 {
			t.Fatalf("approved reference moved: %v", err)
		}
		if _, err = db.Exec(ctx, `UPDATE workforce_delivery_requests SET reauthorized_project_version_id=102 WHERE id=10`); err != nil {
			t.Fatal(err)
		}
		if _, err = store.loadDeliveryRepositoryPlan(ctx, 1, 20, 21, 10); err == nil {
			t.Fatal("unconfirmed reference accepted")
		}
	})
	for _, c := range []struct {
		name   string
		change func(*projectfacade.DeliveryRepositoryBinding)
	}{
		{"stale project version", func(b *projectfacade.DeliveryRepositoryBinding) { b.Current = false }},
		{"different remote", func(b *projectfacade.DeliveryRepositoryBinding) { b.Host = "elsewhere.invalid" }},
	} {
		t.Run("explicit binding rejects "+c.name, func(t *testing.T) {
			prepareExhausted(t)
			p, b := deliveryBindingFixture()
			c.change(&b)
			store.SetFrozenRepositoryPlanReader(deliveryFrozenReaderFixture{p})
			store.SetDeliveryRepositoryBindingReader(&deliveryBindingReaderFixture{binding: b})
			if _, err := store.ReauthorizeRequest(ctx, application.Reauthorization{TenantID: 1, Actor: 7, RequestID: 10, ProjectVersionID: 101, Reason: "test replacement"}); err == nil {
				t.Fatal("unsafe binding accepted")
			}
			var state string
			if err := db.QueryRow(ctx, `SELECT status FROM workforce_delivery_requests WHERE id=10`).Scan(&state); err != nil || state != "MANUAL_REVIEW" {
				t.Fatalf("failed binding changed state: %v", err)
			}
		})
	}

}

type deliveryFixtureIDs struct{ node *snowflake.Node }

type deliveryApprovalRecorder struct {
	calls []notificationfacade.TaskNotificationTransition
}

func (r *deliveryApprovalRecorder) RecordTransition(_ context.Context, _ pgx.Tx, transition notificationfacade.TaskNotificationTransition) (notificationfacade.TransitionDisposition, error) {
	r.calls = append(r.calls, transition)
	return "TASK_ACTION_REQUIRED", nil
}

func (g deliveryFixtureIDs) NextID() int64 { return g.node.Generate().Int64() }

type deliveryBindingReaderFixture struct {
	binding projectfacade.DeliveryRepositoryBinding
}

func (r *deliveryBindingReaderFixture) DeliveryRepositoryBinding(_ context.Context, tenant, project, version int64) (projectfacade.DeliveryRepositoryBinding, error) {
	if tenant != 1 || project != r.binding.ProjectID || version != r.binding.VersionID {
		return projectfacade.DeliveryRepositoryBinding{}, application.ErrNotFound
	}
	return r.binding, nil
}

type deliveryFrozenReaderFixture struct{ plan frozenRepositoryPlan }

func (r deliveryFrozenReaderFixture) FrozenRepositoryPlan(_ context.Context, tenant, task, turn int64) (taskfacade.FrozenRepositoryPlan, error) {
	p := r.plan
	if tenant != 1 || task != 20 || turn != 21 {
		return taskfacade.FrozenRepositoryPlan{}, application.ErrNotFound
	}
	return taskfacade.FrozenRepositoryPlan{ProjectID: p.ProjectID, ProjectVersionID: p.ProjectVersionID, ProviderType: p.ProviderType, Scheme: p.Scheme, Host: p.Host, Port: p.Port, RepositoryPath: p.RepositoryPath, DisplayURL: p.DisplayURL, WebURL: p.WebURL, RepositoryConfigDigest: p.RepositoryConfigDigest, TargetConfigDigest: p.TargetConfigDigest, CredentialVersion: p.Credential.Version, CredentialCiphertext: p.Credential.Ciphertext, CredentialEDEK: p.Credential.EDEK, CredentialWrapKeyRef: p.Credential.WrapKeyRef, BaseBranch: p.BaseBranch, BaseCommit: p.BaseCommit, ResultBranch: p.ResultBranch, DeliveryMode: p.DeliveryMode, PlanDigest: p.PlanDigest, PushAuthorizationDigest: p.PushAuthorizationDigest, ReviewAuthorizationDigest: p.ReviewAuthorizationDigest}, nil
}
