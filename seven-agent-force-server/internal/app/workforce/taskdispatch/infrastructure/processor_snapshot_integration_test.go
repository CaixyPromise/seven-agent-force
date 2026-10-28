package infrastructure

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	routingfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/executionrouting/facade"
	projectfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/project/facade"
	taskdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/domain"
	"github.com/jackc/pgx/v5"
)

type processorSnapshotRecorder struct {
	snapshot *taskdomain.ExecutionSnapshot
}

func (r *processorSnapshotRecorder) ResolveAssignment(_ context.Context, _ pgx.Tx, _ routingfacade.NodeIdentity, _ projectfacade.ExecutionRequirements, snapshot taskdomain.ExecutionSnapshot) (routingfacade.ResolvedAssignment, string, error) {
	r.snapshot = &snapshot
	return routingfacade.ResolvedAssignment{}, routingfacade.WaitProjectInvalid, nil
}

func TestPostgreSQLDispatchPreservesProcessorExecutionPolicy(t *testing.T) {
	for _, tc := range []struct {
		name, manifest, scope, releaseDigest string
		tenantID                             any
		cycles                               int
		delivery                             string
		visible                              bool
		invalidAhead                         bool
	}{
		{"explicit tenant policy", `{"maxEvaluationCycles":5,"continuationPolicy":"CHECKPOINT_REQUIRED","deliveryPolicy":"DECLARED"}`, "TENANT", strings.Repeat("a", 64), int64(10), 5, "DECLARED", true, false},
		{"legacy system defaults", `{}`, "SYSTEM", strings.Repeat("a", 64), nil, 3, "OPTIONAL", true, false},
		{"foreign tenant release", `{}`, "TENANT", strings.Repeat("a", 64), int64(11), 0, "", false, false},
		{"different frozen archive", `{}`, "TENANT", strings.Repeat("b", 64), int64(10), 0, "", false, false},
		{"invalid higher priority task does not starve valid policy", `{}`, "SYSTEM", strings.Repeat("a", 64), nil, 3, "OPTIONAL", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pool := dispatchQATables(t, "workforce_tasks", "workforce_task_turns", "workforce_execution_nodes", "workforce_task_assignments", "workforce_result_processor_releases")
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			digest := strings.Repeat("a", 64)
			for _, query := range []string{
				`INSERT INTO workforce_tasks(id,tenant_id,created_by,idempotency_key,request_hash,repository_url,repository_ref,task_instruction,status,current_task_turn_id,result_processor_release_id,project_id,project_version_id,execution_requirements_json,execution_requirements_digest,execution_timeout_seconds) VALUES(20,10,7,'qa','qa','https://example.invalid/qa','main','qa','QUEUED',30,81,82,83,'{}','` + digest + `',600)`,
				`INSERT INTO workforce_task_turns(id,task_id,tenant_id,turn_number,instruction,execution_snapshot_digest,continuation_lineage_id,created_by,processor_release_id,processor_digest,parameters_json) VALUES(30,20,10,1,'qa','qa',30,7,81,'` + digest + `','{"target":"README.md"}')`,
				`INSERT INTO workforce_execution_nodes(id,task_id,task_turn_id,node_key,status,generation,execution_requirement_digest) VALUES(50,20,30,'qa','QUEUED',1,'` + digest + `')`,
			} {
				_, err := pool.Exec(ctx, query)
				authoritySQLCheck(t, err)
			}
			_, err := pool.Exec(ctx, `INSERT INTO workforce_result_processor_releases(id,processor_id,scope_type,tenant_id,revision_id,version,manifest_json,files_json,archive_object_key,archive_sha256,archive_size_bytes,published_by_principal_id) VALUES(81,80,$1,$2,79,'qa',$3,'[]','fixture',$4,1,7)`, tc.scope, tc.tenantID, json.RawMessage(tc.manifest), tc.releaseDigest)
			authoritySQLCheck(t, err)
			if tc.invalidAhead {
				for _, query := range []string{
					`INSERT INTO workforce_tasks(id,tenant_id,created_by,idempotency_key,request_hash,repository_url,repository_ref,task_instruction,status,current_task_turn_id,result_processor_release_id,project_id,project_version_id,execution_requirements_json,execution_requirements_digest,priority) VALUES(19,10,7,'invalid-ahead','qa','https://example.invalid/qa','main','qa','QUEUED',29,81,82,83,'{}','` + digest + `',100)`,
					`INSERT INTO workforce_task_turns(id,task_id,tenant_id,turn_number,instruction,execution_snapshot_digest,continuation_lineage_id,created_by,processor_release_id,processor_digest) VALUES(29,19,10,1,'qa','qa',29,7,81,'` + strings.Repeat("b", 64) + `')`,
					`INSERT INTO workforce_execution_nodes(id,task_id,task_turn_id,node_key,status,generation,execution_requirement_digest) VALUES(49,19,29,'invalid-ahead','QUEUED',1,'` + digest + `')`,
				} {
					_, err := pool.Exec(ctx, query)
					authoritySQLCheck(t, err)
				}
			}
			recorder := &processorSnapshotRecorder{}
			store := New(pool, nil)
			store.SetAssignmentResolver(recorder)
			tenantID, assigned, err := store.DispatchOne(ctx, []int64{})
			authoritySQLCheck(t, err)
			if tenantID != 10 || assigned || (recorder.snapshot != nil) != tc.visible {
				t.Fatalf("unexpected dispatch visibility: tenant=%d assigned=%v snapshot=%v", tenantID, assigned, recorder.snapshot)
			}
			if !tc.visible {
				return
			}
			snapshot := recorder.snapshot
			if snapshot.MaxEvaluationCycles != tc.cycles || snapshot.ContinuationPolicy != "CHECKPOINT_REQUIRED" || snapshot.DeliveryPolicy != tc.delivery {
				t.Fatalf("processor policy lost before runtime selection: %+v", snapshot)
			}
			var parameters map[string]string
			if err := json.Unmarshal(snapshot.ResultProcessorParameters, &parameters); err != nil {
				t.Fatal(err)
			}
			if snapshot.ResultProcessorReleaseID != 81 || snapshot.ResultProcessorDigest != digest || snapshot.ExecutionDeadlineSeconds != 600 || parameters["target"] != "README.md" {
				t.Fatalf("frozen processor identity, parameters or budget changed: %+v", snapshot)
			}
		})
	}
}

func TestPostgreSQLDispatchPreservesWorkItemStageAndPlan(t *testing.T) {
	pool := dispatchQATables(t, "workforce_tasks", "workforce_task_turns", "workforce_execution_nodes", "workforce_task_assignments", "workforce_result_processor_releases")
	ctx := context.Background()
	digest := strings.Repeat("a", 64)
	for _, query := range []string{
		`INSERT INTO workforce_tasks(id,tenant_id,created_by,idempotency_key,request_hash,repository_url,repository_ref,task_instruction,status,current_task_turn_id,result_processor_release_id,project_id,project_version_id,execution_requirements_json,execution_requirements_digest,work_item_input_json) VALUES(20,10,7,'qa-stage','qa','https://example.invalid/qa','main','qa','QUEUED',30,81,82,83,'{"stagePurpose":"IMPLEMENT"}','` + digest + `','{"contentDigest":"approved","plan":{"title":"Approved plan"}}')`,
		`INSERT INTO workforce_task_turns(id,task_id,tenant_id,turn_number,instruction,execution_snapshot_digest,continuation_lineage_id,created_by,processor_release_id,processor_digest) VALUES(30,20,10,1,'qa','qa',30,7,81,'` + digest + `')`,
		`INSERT INTO workforce_execution_nodes(id,task_id,task_turn_id,node_key,status,generation,execution_requirement_digest) VALUES(50,20,30,'qa','QUEUED',1,'` + digest + `')`,
		`INSERT INTO workforce_result_processor_releases(id,processor_id,scope_type,revision_id,version,manifest_json,files_json,archive_object_key,archive_sha256,archive_size_bytes,published_by_principal_id) VALUES(81,80,'SYSTEM',79,'qa','{}','[]','fixture','` + digest + `',1,7)`,
	} {
		_, err := pool.Exec(ctx, query)
		authoritySQLCheck(t, err)
	}
	recorder := &processorSnapshotRecorder{}
	store := New(pool, nil)
	store.SetAssignmentResolver(recorder)
	_, _, err := store.DispatchOne(ctx, []int64{})
	authoritySQLCheck(t, err)
	if recorder.snapshot == nil || recorder.snapshot.StagePurpose != "IMPLEMENT" {
		t.Fatal("stage purpose lost in dispatch")
	}
	var plan struct {
		ContentDigest string `json:"contentDigest"`
		Plan          struct {
			Title string `json:"title"`
		} `json:"plan"`
	}
	if err = json.Unmarshal(recorder.snapshot.ApprovedPlan, &plan); err != nil || plan.ContentDigest != "approved" || plan.Plan.Title != "Approved plan" {
		t.Fatalf("approved plan lost in dispatch: %s %v", recorder.snapshot.ApprovedPlan, err)
	}
}
