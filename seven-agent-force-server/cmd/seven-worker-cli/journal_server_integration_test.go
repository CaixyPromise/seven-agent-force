package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	wr "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/attempt/runtime"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/runtimeconfig"
	task "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/domain"
	app "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/taskdispatch/application"
	dispatch "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/taskdispatch/domain"
	infra "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/taskdispatch/infrastructure"
	workercontrol "github.com/CaixyPromise/seven-agent/force/internal/worker/control"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestActualJournalProducerToServerDatabase(t *testing.T) {
	dsn := os.Getenv("INTERACTION_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("requires isolated PostgreSQL QA schema")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal("invalid QA DSN")
	}
	admin, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	schema := fmt.Sprintf("qa_journal_%d", time.Now().UnixNano())
	quoted := pgx.Identifier{schema}.Sanitize()
	exec := func(sql string, args ...any) {
		t.Helper()
		if _, err := admin.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec("CREATE SCHEMA " + quoted)
	defer func() {
		cleanup, c := context.WithTimeout(context.Background(), 10*time.Second)
		defer c()
		if _, err := admin.Exec(cleanup, "DROP SCHEMA "+quoted+" CASCADE"); err != nil {
			t.Error(err)
		}
	}()
	for _, table := range []string{"workforce_tasks", "workforce_task_attempts", "workforce_attempt_runtimes", "workforce_runtime_reconciliation_runs", "workforce_attempt_facts", "workforce_task_events"} {
		exec("CREATE TABLE " + pgx.Identifier{schema, table}.Sanitize() + " (LIKE " + pgx.Identifier{"public", table}.Sanitize() + " INCLUDING ALL)")
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = quoted + ",pg_catalog"
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	run := func(sql string, args ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, sql, args...); err != nil {
			t.Fatal(err)
		}
	}
	for _, sql := range []string{
		`CREATE SEQUENCE qa_event_ids`, `ALTER TABLE workforce_task_events ALTER COLUMN id DROP IDENTITY IF EXISTS`, `ALTER TABLE workforce_task_events ALTER COLUMN id SET DEFAULT nextval('qa_event_ids'::regclass)`,
		`INSERT INTO workforce_tasks(id,tenant_id,created_by,idempotency_key,request_hash,repository_url,repository_ref,task_instruction,harness_alias,status,current_task_turn_id) VALUES(20,10,7,'qa','qa','','','qa','qa','RUNNING',30)`,
		`INSERT INTO workforce_task_attempts(id,task_id,task_turn_id,attempt_no,worker_id,workspace_path,status,lease_token,lease_expires_at,runtime_driver,worker_instance_id,host_boot_id,generation,owner_epoch,execution_node_id,assignment_id) VALUES(40,20,30,1,8,'/qa','RUNNING','placeholder',CURRENT_TIMESTAMP+interval '1 hour','docker','qa-new','qa-boot',1,2,50,60)`,
		`INSERT INTO workforce_attempt_runtimes(attempt_id,execution_node_id,assignment_id,worker_id,worker_instance_id,host_boot_id,generation,owner_epoch,execution_identity_digest,expected_container_name,harness_image_digest,status,authority_receipt_sha256) VALUES(40,50,60,8,'qa-new','qa-boot',1,2,'qa','qa','qa','RUNNING','qa-ack')`,
		`INSERT INTO workforce_runtime_reconciliation_runs(id,attempt_id,execution_node_id,worker_id,old_instance_id,new_instance_id,host_boot_id,old_owner_epoch,new_owner_epoch,status,runtime_evidence_sha256,supervisor_ack_sha256,deadline) VALUES(90,40,50,8,'qa-old','qa-new','qa-boot',1,2,'COMPLETED','qa-evidence','qa-ack',CURRENT_TIMESTAMP+interval '1 hour')`,
		`ALTER TABLE workforce_attempt_facts ADD COLUMN IF NOT EXISTS producer_source TEXT NOT NULL DEFAULT 'WORKER'`,
		`DROP INDEX IF EXISTS workforce_attempt_facts_source_sequence_uidx`,
		`ALTER TABLE workforce_attempt_facts DROP CONSTRAINT IF EXISTS workforce_attempt_facts_attempt_id_producer_sequence_key`,
		`ALTER TABLE workforce_attempt_facts ADD CONSTRAINT workforce_attempt_facts_attempt_id_producer_sequence_key UNIQUE(attempt_id,producer_sequence)`,
	} {
		run(sql)
	}
	lease := "qa-private-lease"
	sum := sha256.Sum256([]byte(lease))
	run(`UPDATE workforce_task_attempts SET lease_token=$1`, hex.EncodeToString(sum[:]))
	root := t.TempDir()
	path := filepath.Join(root, "20", "attempts", "40", "state")
	if err = os.MkdirAll(path, 0700); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(filepath.Join(path, "runtime-journal.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	state := &supervisorState{OwnerEpoch: 2, ReattachPermitID: 90, State: "RUNNING", HarnessPID: 12}
	producer := &controlServer{state: state, spec: attemptSpec{AttemptID: 40}, journal: f, journalSequence: 2}
	if err = producer.writeJournal("owner.adopted", reattachAckSnapshot(state, 1)); err != nil {
		t.Fatal(err)
	}
	docker := wr.NewDocker(config.WorkerConfig{WorkspaceRoot: root}, config.ProviderProxyConfig{})
	facts, err := docker.ReadRuntimeJournalFacts(task.WorkItem{TaskID: 20}, task.Attempt{ID: 40})
	if err != nil {
		t.Fatal(err)
	}
	wire, _ := json.Marshal(facts)
	if err = json.Unmarshal(wire, &facts); err != nil {
		t.Fatal(err)
	}
	store := infra.New(pool, nil)
	identity := dispatch.MachineEnvelope{WorkerID: 8, InstanceID: "qa-new", HostBootID: "qa-boot", Generation: 1, OwnerEpoch: 2}
	live := dispatch.Fact{FactID: "fac_live", ProducerSequence: 3, EventType: "harness.started", Payload: json.RawMessage(`{"harnessPid":12}`)}
	if err = store.RecordFacts(ctx, identity, 40, lease, []dispatch.Fact{live}); err != nil {
		t.Fatal(err)
	}
	if err = store.RecordJournalFacts(ctx, identity, 40, lease, facts); !errors.Is(err, app.ErrIdentityConflict) {
		t.Fatalf("legacy collision not reproduced: %v", err)
	}
	// Apply the exact explicit alignment transaction twice; never public data.
	for i := 0; i < 2; i++ {
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = tx.Exec(ctx, infra.FactProducerSchemaSQL); err != nil {
			tx.Rollback(ctx)
			t.Fatal(err)
		}
		if err = tx.Commit(ctx); err != nil {
			t.Fatal(err)
		}
	}
	// Include the actual HTTP Client marshalling path. Direct Store calls alone
	// miss merge()/RawMessage re-encoding and invalidate byte-bound digests.
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/internal/workforce/v1/attempts/40/journal-facts/batch" {
			t.Error("wrong journal route")
			w.WriteHeader(404)
			return
		}
		var req struct {
			dispatch.MachineEnvelope
			AttemptLeaseToken string          `json:"attemptLeaseToken"`
			Facts             []dispatch.Fact `json:"facts"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Error(err)
			w.WriteHeader(400)
			return
		}
		err := store.RecordJournalFacts(r.Context(), req.MachineEnvelope, 40, req.AttemptLeaseToken, req.Facts)
		w.Header().Set("Content-Type", "application/json")
		if err != nil {
			_ = json.NewEncoder(w).Encode(map[string]any{"code": 1, "message": "test journal rejected"})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": 0, "data": map[string]any{"ack": true}})
	}))
	defer server.Close()
	client, err := workercontrol.New(server.URL, "test-machine-token-not-used-outside-test", 8, "qa-new", "qa-boot")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err = client.ReplayJournalFacts(ctx, task.Attempt{ID: 40, Generation: 1, OwnerEpoch: 2, LeaseToken: lease}, facts); err != nil {
			t.Fatalf("actual HTTP producer-scoped replay: %v", err)
		}
	}
	reject := func(e dispatch.MachineEnvelope, batch []dispatch.Fact, journal bool) {
		t.Helper()
		var err error
		if journal {
			err = store.RecordJournalFacts(ctx, e, 40, lease, batch)
		} else {
			err = store.RecordFacts(ctx, e, 40, lease, batch)
		}
		if !errors.Is(err, app.ErrIdentityConflict) {
			t.Fatalf("immutable conflict not rejected: %v", err)
		}
	}
	old := identity
	old.OwnerEpoch = 1
	reject(old, facts, true)
	reject(identity, facts, false)
	bad := facts[0]
	bad.EventType = "task.packaging"
	reject(identity, []dispatch.Fact{bad}, true)
	bad = facts[0]
	bad.EventType = "hook.allow"
	reject(identity, []dispatch.Fact{bad}, true)
	bad = facts[0]
	bad.Payload = json.RawMessage(`{"changed":true}`)
	reject(identity, []dispatch.Fact{bad}, true)
	altered := live
	altered.EventType = "hook.allow"
	reject(identity, []dispatch.Fact{altered}, false)
	altered = live
	altered.ProducerSequence++
	reject(identity, []dispatch.Fact{altered}, false)
	altered = live
	altered.FactID = "fac_other"
	reject(identity, []dispatch.Fact{altered}, false)
	var count, events int
	var status string
	var owner int64
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM workforce_attempt_facts WHERE producer_sequence=3`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM workforce_task_events WHERE event_type='owner.adopted'`).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT status,owner_epoch FROM workforce_task_attempts WHERE id=40`).Scan(&status, &owner); err != nil {
		t.Fatal(err)
	}
	if count != 2 || events != 1 || status != "RUNNING" || owner != 2 {
		t.Fatal("replay mutated authority or duplicated evidence")
	}
}
