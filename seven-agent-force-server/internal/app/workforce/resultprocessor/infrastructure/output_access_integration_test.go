package infrastructure

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestAvailableOutputsFailClosedOnRelationDrift(t *testing.T) {
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

	tenantID := int64(8_400_000_000_000_010_001)
	taskID := int64(8_400_000_000_000_010_002)
	attemptID := int64(8_400_000_000_000_010_003)
	validArtifactID := int64(8_400_000_000_000_010_004)
	validOutputID := int64(8_400_000_000_000_010_005)
	digest := strings.Repeat("a", 64)

	defer func() {
		_, _ = db.Exec(context.Background(), `DELETE FROM workforce_result_outputs WHERE task_id=$1`, taskID)
		_, _ = db.Exec(context.Background(), `DELETE FROM workforce_artifacts WHERE task_id=$1`, taskID)
		_, _ = db.Exec(context.Background(), `DELETE FROM workforce_tasks WHERE id=$1`, taskID)
	}()

	if _, err = db.Exec(ctx, `INSERT INTO workforce_tasks
(id,tenant_id,created_by,idempotency_key,request_hash,repository_url,repository_ref,task_instruction,status,
 result_processor_release_id,project_id,project_version_id,review_enabled,execution_requirements_json,execution_requirements_digest)
VALUES ($1,$2,$3,$4,$5,'','','output access test','SUCCEEDED',$6,$7,$7,TRUE,'{}',$5)`,
		taskID, tenantID, int64(101), "output-access-test", strings.Repeat("f", 64),
		int64(8_400_000_000_000_010_012), int64(8_400_000_000_000_010_013)); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `INSERT INTO workforce_artifacts
(id,task_id,attempt_id,artifact_type,name,object_key,sha256,size_bytes,media_type,status)
VALUES ($1,$2,$3,'RESULT_OUTPUT','result','tasks/result',$4,7,'text/markdown','AVAILABLE')`, validArtifactID, taskID, attemptID, digest); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `INSERT INTO workforce_result_outputs
(id,task_id,task_turn_id,attempt_id,evaluation_cycle_id,name,source_path,output_kind,storage,artifact_id,sha256,size_bytes,status)
VALUES
($1,$2,$3,$4,$5,'result','SEVEN_WORKER_RESULT.md','PATH','IMMEDIATE',$6,$7,7,'AVAILABLE'),
($8,$2,$3,$4,$9,'wrong-digest','wrong.md','PATH','IMMEDIATE',$6,$10,7,'AVAILABLE'),
($11,$2,$3,$4,$12,'on-demand','later.zip','ARCHIVE','ON_DEMAND',$6,$7,7,'AVAILABLE')`,
		validOutputID, taskID, int64(8_400_000_000_000_010_006), attemptID, int64(8_400_000_000_000_010_007), validArtifactID, digest,
		int64(8_400_000_000_000_010_008), int64(8_400_000_000_000_010_009), strings.Repeat("b", 64),
		int64(8_400_000_000_000_010_010), int64(8_400_000_000_000_010_011)); err != nil {
		t.Fatal(err)
	}

	store := New(db, nil)
	items, err := store.ListAvailableOutputs(ctx, tenantID, taskID)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Output.ID != validOutputID || items[0].ObjectKey != "tasks/result" {
		t.Fatalf("available outputs = %+v", items)
	}
	item, err := store.AvailableOutput(ctx, tenantID, taskID, validOutputID)
	if err != nil || item.Output.ID != validOutputID {
		t.Fatalf("valid output = %+v, err=%v", item, err)
	}
	if _, err := store.AvailableOutput(ctx, tenantID, taskID, int64(8_400_000_000_000_010_008)); !errors.Is(err, ErrNotFound) {
		t.Fatalf("drifted output error = %v, want ErrNotFound", err)
	}
	if _, err := store.AvailableOutput(ctx, tenantID+1, taskID, validOutputID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-tenant output error = %v, want ErrNotFound", err)
	}
	if _, err := store.AvailableOutput(ctx, tenantID, taskID+99, validOutputID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("wrong-task output error = %v, want ErrNotFound", err)
	}

	relationDriftCases := []struct {
		name    string
		mutate  string
		args    []any
		restore string
	}{
		{"artifact-task", `UPDATE workforce_artifacts SET task_id=$2 WHERE id=$1`, []any{validArtifactID, taskID + 99}, `UPDATE workforce_artifacts SET task_id=$2 WHERE id=$1`},
		{"artifact-attempt", `UPDATE workforce_artifacts SET attempt_id=$2 WHERE id=$1`, []any{validArtifactID, attemptID + 99}, `UPDATE workforce_artifacts SET attempt_id=$2 WHERE id=$1`},
		{"artifact-type", `UPDATE workforce_artifacts SET artifact_type='RESULT_MANIFEST' WHERE id=$1`, []any{validArtifactID}, `UPDATE workforce_artifacts SET artifact_type='RESULT_OUTPUT' WHERE id=$1`},
		{"artifact-status", `UPDATE workforce_artifacts SET status='PENDING' WHERE id=$1`, []any{validArtifactID}, `UPDATE workforce_artifacts SET status='AVAILABLE' WHERE id=$1`},
		{"artifact-digest", `UPDATE workforce_artifacts SET sha256=$2 WHERE id=$1`, []any{validArtifactID, strings.Repeat("c", 64)}, `UPDATE workforce_artifacts SET sha256=$2 WHERE id=$1`},
		{"artifact-size", `UPDATE workforce_artifacts SET size_bytes=8 WHERE id=$1`, []any{validArtifactID}, `UPDATE workforce_artifacts SET size_bytes=7 WHERE id=$1`},
		{"output-artifact", `UPDATE workforce_result_outputs SET artifact_id=$2 WHERE id=$1`, []any{validOutputID, validArtifactID + 99}, `UPDATE workforce_result_outputs SET artifact_id=$2 WHERE id=$1`},
		{"output-status", `UPDATE workforce_result_outputs SET status='DECLARED' WHERE id=$1`, []any{validOutputID}, `UPDATE workforce_result_outputs SET status='AVAILABLE' WHERE id=$1`},
		{"output-storage", `UPDATE workforce_result_outputs SET storage='ON_DEMAND' WHERE id=$1`, []any{validOutputID}, `UPDATE workforce_result_outputs SET storage='IMMEDIATE' WHERE id=$1`},
		{"output-digest", `UPDATE workforce_result_outputs SET sha256=$2 WHERE id=$1`, []any{validOutputID, strings.Repeat("d", 64)}, `UPDATE workforce_result_outputs SET sha256=$2 WHERE id=$1`},
		{"output-size", `UPDATE workforce_result_outputs SET size_bytes=8 WHERE id=$1`, []any{validOutputID}, `UPDATE workforce_result_outputs SET size_bytes=7 WHERE id=$1`},
	}
	for _, test := range relationDriftCases {
		t.Run(test.name, func(t *testing.T) {
			if _, err := db.Exec(ctx, test.mutate, test.args...); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				restoreArgs := test.args
				switch test.name {
				case "artifact-task":
					restoreArgs = []any{validArtifactID, taskID}
				case "artifact-attempt":
					restoreArgs = []any{validArtifactID, attemptID}
				case "artifact-digest":
					restoreArgs = []any{validArtifactID, digest}
				case "output-artifact":
					restoreArgs = []any{validOutputID, validArtifactID}
				case "output-digest":
					restoreArgs = []any{validOutputID, digest}
				}
				_, _ = db.Exec(context.Background(), test.restore, restoreArgs...)
			})
			items, err := store.ListAvailableOutputs(ctx, tenantID, taskID)
			if err != nil {
				t.Fatal(err)
			}
			if len(items) != 0 {
				t.Fatalf("relation-drifted output was listed: %+v", items)
			}
			if _, err := store.AvailableOutput(ctx, tenantID, taskID, validOutputID); !errors.Is(err, ErrNotFound) {
				t.Fatalf("relation-drifted output error = %v, want ErrNotFound", err)
			}
		})
	}
}
