package infrastructure

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	taskinfra "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/infrastructure"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/taskdispatch/application"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/taskdispatch/domain"
)

func TestPostgreSQLExecutionBudgetReportsAreMonotonicAndFenced(t *testing.T) {
	pool := dispatchQATables(t, "workforce_tasks", "workforce_task_attempts", "workforce_attempt_runtimes", "workforce_task_events", "workforce_model_credentials", "workforce_task_input_snapshots", "workforce_task_input_references", "workforce_execution_nodes")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	digest := strings.Repeat("a", 64)
	for _, query := range []string{
		`INSERT INTO workforce_tasks(id,tenant_id,created_by,idempotency_key,request_hash,repository_url,repository_ref,task_instruction,status,execution_timeout_seconds,result_processor_release_id,project_id,project_version_id,execution_requirements_json,execution_requirements_digest,model_credential_id,primary_model) VALUES(20,10,7,'qa','qa','https://example.invalid/qa','main','qa','RUNNING',60,81,82,83,'{}','` + digest + `',85,'qa-model')`,
		`INSERT INTO workforce_task_attempts(id,task_id,attempt_no,worker_id,workspace_path,status,lease_token,lease_expires_at,runtime_driver,worker_instance_id,host_boot_id,generation,owner_epoch,assignment_id,execution_node_id,assignment_selection_digest,resolved_execution_snapshot_json) VALUES(40,20,1,8,'/tmp/qa','RUNNING','` + hashToken("lease") + `',CURRENT_TIMESTAMP+interval '1 hour','docker','qa-instance','qa-boot',3,5,60,50,'` + digest + `','{}')`,
		`INSERT INTO workforce_attempt_runtimes(attempt_id,execution_node_id,assignment_id,worker_id,worker_instance_id,host_boot_id,generation,owner_epoch,execution_identity_digest,expected_container_name,harness_image_digest,status,execution_lease_sha256,execution_lease_expires_at) VALUES(40,50,60,8,'qa-instance','qa-boot',3,5,'qa','qa','qa','RUNNING','qa',CURRENT_TIMESTAMP+interval '1 hour')`,
	} {
		if _, err := pool.Exec(ctx, query); err != nil {
			t.Fatal(err)
		}
	}
	store := New(pool, nil)
	owner := domain.MachineEnvelope{WorkerID: 8, InstanceID: "qa-instance", HostBootID: "qa-boot", Generation: 3, OwnerEpoch: 5}
	fortySeconds := int64(40000)
	if _, err := store.RenewExecutionLease(ctx, owner, 40, "lease", &fortySeconds); err != nil {
		t.Fatal(err)
	}
	thirtySeconds := int64(30000)
	if _, err := store.RenewExecutionLease(ctx, owner, 40, "lease", &thirtySeconds); !errors.Is(err, application.ErrIdentityConflict) {
		t.Fatalf("decreasing confirmed Task time: %v", err)
	}
	stale := owner
	stale.OwnerEpoch++
	if _, err := store.RenewExecutionLease(ctx, stale, 40, "lease", &fortySeconds); !errors.Is(err, application.ErrIdentityConflict) {
		t.Fatalf("stale owner updated Task time: %v", err)
	}
	var persisted int64
	if err := pool.QueryRow(ctx, `SELECT execution_active_milliseconds FROM workforce_tasks WHERE id=20`).Scan(&persisted); err != nil || persisted != fortySeconds {
		t.Fatalf("confirmed active time=%d, error=%v", persisted, err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO workforce_task_events(id,task_id,attempt_id,event_type,sequence,payload_json) VALUES(70,20,40,'harness.log',1,'{}')`); err != nil {
		t.Fatal(err)
	}
	view, err := taskinfra.NewStore(pool, nil).Get(ctx, 20)
	if err != nil || view.ExecutionActiveMilliseconds != fortySeconds || view.LastProgressAt == "" {
		t.Fatalf("task progress projection: active=%d, last=%q, err=%v", view.ExecutionActiveMilliseconds, view.LastProgressAt, err)
	}
}
