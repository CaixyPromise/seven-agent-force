package infrastructure

import (
	"context"
	"errors"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/facade"
	"github.com/jackc/pgx/v5/pgxpool"
)

// This exercises the real authorization/decision SQL, not a row mock. The
// transaction rolls back every fixture; it is not a Docker recovery test.
func TestEvaluationReplayPostgresWaitUserAndFencing(t *testing.T) {
	dsn := os.Getenv("RESULT_PROCESSOR_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("RESULT_PROCESSOR_TEST_POSTGRES_DSN requires an isolated migrated database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal("test database connection failed")
	}
	defer db.Close()
	var name string
	if err := db.QueryRow(ctx, "SELECT current_database()").Scan(&name); err != nil || !strings.HasPrefix(name, "seven_worker_batch2_") {
		t.Fatal("isolated Batch2 database required")
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	// These identities belong only to this rolled-back fixture.
	const task, turn, attempt, cycle int64 = 920001, 920002, 920003, 920004
	const processorRelease, processorID, processorRevision int64 = 920013, 920018, 920019
	digest := strings.Repeat("a", 64)
	imageDigest := "sha256:" + strings.Repeat("b", 64)
	snapshot := `{"runtimeReleaseId":"920014","runtimeImageDigest":"` + imageDigest + `","resultProcessorReleaseId":"920013","resultProcessorDigest":"` + digest + `","modelCredentialId":"920022","credentialVersion":"1","modelProtocol":"OPENAI","primaryModel":"fixture-model","maxEvaluationCycles":3,"isolationProfile":"seven.worker-image/v1"}`
	statements := []string{
		`INSERT INTO workforce_result_processor_releases(id,processor_id,scope_type,tenant_id,revision_id,version,manifest_json,files_json,archive_object_key,archive_sha256,archive_size_bytes,published_by_principal_id) VALUES(` + strconv.FormatInt(processorRelease, 10) + `,` + strconv.FormatInt(processorID, 10) + `,'TENANT',920010,` + strconv.FormatInt(processorRevision, 10) + `,'fixture','{"maxEvaluationCycles":3}','[]','fixture', '` + digest + `',1,920011)`,
		`INSERT INTO workforce_tasks(id,tenant_id,created_by,idempotency_key,request_hash,repository_url,repository_ref,task_instruction,status,current_task_turn_id,result_processor_release_id,project_id,project_version_id,review_enabled,model_credential_id,primary_model,execution_requirements_json,execution_requirements_digest) VALUES(920001,920010,920011,'batch2-replay','` + digest + `','','','fixture','REVIEWING',920002,920013,920020,920020,TRUE,920022,'fixture-model','{}','` + digest + `')`,
		`INSERT INTO workforce_task_turns(id,task_id,tenant_id,turn_number,instruction,execution_snapshot_digest,continuation_lineage_id,created_by,processor_release_id,processor_digest,environment_snapshot_json,secret_version_identities,session_digest) VALUES(920002,920001,920010,1,'fixture','` + digest + `',920012,920011,920013,'` + digest + `','{"bindings":[]}','[]','session')`,
		`INSERT INTO workforce_task_assignments(id,task_id,execution_node_id,worker_id,worker_instance_id,generation,execution_requirement_digest,agent_runtime_release_id,runtime_image_ref,runtime_image_digest,adapter_identity,adapter_version,adapter_protocol,adapter_capabilities_json,model_credential_id,credential_version,model_protocol,primary_model,provider_proxy_identity,worker_image_contract,isolation_profile,resolved_execution_snapshot_json,selection_digest,lease_token_sha256,lease_expires_at,status,message_id) VALUES(920017,920001,920016,920015,'instance',1,'` + digest + `',920014,'fixture@` + imageDigest + `','` + imageDigest + `','fixture-adapter','1','seven.agent-adapter/v1','[]',920022,1,'OPENAI','fixture-model','fixture-provider-proxy','seven.worker-image/v1','seven.worker-image/v1','` + snapshot + `','` + digest + `','` + sha256Hex("assignment-lease") + `',CURRENT_TIMESTAMP+INTERVAL '1 hour','CLAIMED',920021)`,
		`INSERT INTO workforce_task_attempts(id,task_id,attempt_no,worker_id,workspace_path,status,lease_token,lease_expires_at,runtime_driver,task_turn_id,execution_node_id,worker_instance_id,host_boot_id,assignment_id,assignment_selection_digest,resolved_execution_snapshot_json) VALUES(920003,920001,1,920015,'fixture','RUNNING','` + sha256Hex("fixture-lease") + `',CURRENT_TIMESTAMP+INTERVAL '1 hour','docker',920002,920016,'instance','boot',920017,'` + digest + `','` + snapshot + `')`,
		`INSERT INTO workforce_attempt_runtimes(attempt_id,execution_node_id,assignment_id,worker_id,worker_instance_id,host_boot_id,generation,execution_identity_digest,expected_container_name,harness_image_digest,status) VALUES(920003,920016,920017,920015,'instance','boot',1,'identity','fixture','` + imageDigest + `','RUNNING')`,
		`INSERT INTO workforce_result_evaluation_cycles(id,task_id,task_turn_id,execution_node_id,attempt_id,cycle_number,session_digest,contract_digest,decision,reason_code,contract_json) VALUES(920004,920001,920002,920016,920003,1,'session','contract-digest','WAIT_USER','CHECK_UNAVAILABLE','{"decision":"WAIT_USER"}')`,
	}
	for _, sql := range statements {
		if _, err := tx.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	owner := facade.MachineIdentity{WorkerID: 920015, InstanceID: "instance", HostBootID: "boot", Generation: 1, OwnerEpoch: 1}
	pending := facade.EvaluationReplayIdentity{CycleID: cycle, CycleNumber: 1, SessionDigest: "session"}
	store := &Store{}
	for n := 0; n < 2; n++ {
		result, err := store.readEvaluationTx(ctx, tx, owner, attempt, "fixture-lease", pending)
		if err != nil || !result.Found || result.Directive.Directive != facade.DirectiveWaitForUser || result.Directive.ContractDigest != "contract-digest" {
			t.Fatalf("persisted WAIT_USER replay failed: %v", err)
		}
	}
	stale := owner
	stale.OwnerEpoch++
	if _, err := store.readEvaluationTx(ctx, tx, stale, attempt, "fixture-lease", pending); !errors.Is(err, ErrConflict) {
		t.Fatal("stale owner replay accepted")
	}
	if _, err := tx.Exec(ctx, `UPDATE workforce_tasks SET cancel_requested_at=CURRENT_TIMESTAMP WHERE id=$1`, task); err != nil {
		t.Fatal(err)
	}
	if _, err := store.readEvaluationTx(ctx, tx, owner, attempt, "fixture-lease", pending); !errors.Is(err, ErrConflict) {
		t.Fatal("cancelled replay accepted")
	}
	var cycles, turns int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM workforce_result_evaluation_cycles WHERE task_id=$1`, task).Scan(&cycles); err != nil {
		t.Fatal(err)
	}
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM workforce_task_turns WHERE task_id=$1`, task).Scan(&turns); err != nil {
		t.Fatal(err)
	}
	if cycles != 1 || turns != 1 {
		t.Fatal("replay created duplicate evaluation or Turn")
	}
	_ = turn
}
