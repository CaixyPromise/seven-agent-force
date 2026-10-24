package infrastructure

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/application"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/facade"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// TestOperationSuspendPostgresAtomicResume exercises the production PostgreSQL
// transaction path in a disposable schema. It intentionally requires an
// explicit test DSN and never falls back to a developer database.
func TestOperationSuspendPostgresAtomicResume(t *testing.T) {
	dsn := strings.TrimSpace(os.Getenv("CONTINUATION_SUSPEND_TEST_POSTGRES_DSN"))
	if dsn == "" {
		t.Skip("set CONTINUATION_SUSPEND_TEST_POSTGRES_DSN to an isolated PostgreSQL test instance")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	db := newOperationSuspendTestDB(t, ctx, dsn)
	store := New(db, &suspendIDs{})
	service := application.NewOperationResume(store)
	request := application.OperationResumeRequest{
		TenantID:         1,
		TaskID:           2,
		SourceTaskTurnID: 3,
		SourceAttemptID:  4,
		OperationID:      50,
		CheckpointID:     6,
		DecisionID:       51,
		CreatorUserID:    8,
		Action:           "ALLOW_ONCE",
	}

	t.Run("caller rollback leaves no decision or resume residue", func(t *testing.T) {
		tx, err := db.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		insertOperationDecision(t, ctx, tx, request.DecisionID, request.OperationID, request.Action)
		target, err := store.FindOperationResumeTx(ctx, tx, facade.OperationResumeLookup{TenantID: 1, TaskID: 2, AttemptID: 4, OperationID: 50})
		if err != nil || target == nil || target.SourceTaskTurnID != 3 || target.CheckpointID != 6 || target.ResumedTaskTurnID != 0 {
			t.Fatalf("find before rollback = %+v, err=%v", target, err)
		}
		if _, err = service.ResumeOperationTx(ctx, tx, request); err != nil {
			t.Fatal(err)
		}
		if err = tx.Rollback(ctx); err != nil {
			t.Fatal(err)
		}

		assertOperationSuspendInitialState(t, ctx, db)
		var decisions int
		if err = db.QueryRow(ctx, `SELECT count(*) FROM workforce_execution_operation_decisions WHERE id=51`).Scan(&decisions); err != nil || decisions != 0 {
			t.Fatalf("rolled-back decision count=%d err=%v", decisions, err)
		}
	})

	t.Run("cancelled task refuses resume", func(t *testing.T) {
		if _, err := db.Exec(ctx, `UPDATE workforce_tasks SET cancel_requested_at=now() WHERE id=2`); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			_, _ = db.Exec(context.Background(), `UPDATE workforce_tasks SET cancel_requested_at=NULL WHERE id=2`)
		})

		tx, err := db.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		target, err := store.FindOperationResumeTx(ctx, tx, facade.OperationResumeLookup{TenantID: 1, TaskID: 2, AttemptID: 4, OperationID: 50})
		if err != nil || target == nil {
			t.Fatalf("find cancelled operation = %+v, err=%v", target, err)
		}
		if _, err = service.ResumeOperationTx(ctx, tx, request); err == nil {
			t.Fatal("cancelled operation resume was accepted")
		}
		if err = tx.Rollback(ctx); err != nil {
			t.Fatal(err)
		}
		if _, err = db.Exec(ctx, `UPDATE workforce_tasks SET cancel_requested_at=NULL WHERE id=2`); err != nil {
			t.Fatal(err)
		}
		assertOperationSuspendInitialState(t, ctx, db)
	})

	var resumedTurnID int64
	t.Run("two concurrent decisions create one successor", func(t *testing.T) {
		// Both executionaccess callers persist their decision and invoke
		// ResumeOperationTx in the same transaction. The Task/checkpoint locks
		// and unique consumption identities permit only one transaction to win.
		start := make(chan struct{})
		results := make(chan operationResumeAttempt, 2)
		for i := 0; i < 2; i++ {
			concurrentRequest := request
			concurrentRequest.DecisionID = int64(60 + i)
			go func(r application.OperationResumeRequest) {
				<-start
				results <- runOperationResumeTransaction(ctx, db, store, service, r)
			}(concurrentRequest)
		}
		close(start)

		successes := 0
		for i := 0; i < 2; i++ {
			result := <-results
			if result.err == nil {
				successes++
				resumedTurnID = result.turnID
			}
		}
		if successes != 1 {
			t.Fatalf("concurrent successful ResumeOperationTx calls=%d, want 1", successes)
		}
		if resumedTurnID <= 1000 {
			t.Fatalf("unexpected resumed turn id %d", resumedTurnID)
		}
		var committedDecisions int
		if err := db.QueryRow(ctx, `SELECT count(*) FROM workforce_execution_operation_decisions WHERE operation_id=50`).Scan(&committedDecisions); err != nil || committedDecisions != 1 {
			t.Fatalf("committed decision count=%d err=%v", committedDecisions, err)
		}
	})

	assertOperationResumeCommittedState(t, ctx, db, resumedTurnID)

	t.Run("idempotent replay finds the existing resumed turn", func(t *testing.T) {
		tx, err := db.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback(ctx)
		target, err := store.FindOperationResumeTx(ctx, tx, facade.OperationResumeLookup{TenantID: 1, TaskID: 2, AttemptID: 4, OperationID: 50})
		if err != nil {
			t.Fatal(err)
		}
		if target == nil || target.SourceTaskTurnID != 3 || target.CheckpointID != 6 || target.ResumedTaskTurnID != resumedTurnID {
			t.Fatalf("replay target=%+v, want resumed turn %d", target, resumedTurnID)
		}
	})
}

type operationResumeAttempt struct {
	turnID int64
	err    error
}

func runOperationResumeTransaction(ctx context.Context, db *pgxpool.Pool, store *Store, service *application.OperationResumeService, request application.OperationResumeRequest) operationResumeAttempt {
	tx, err := db.Begin(ctx)
	if err != nil {
		return operationResumeAttempt{err: err}
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, `INSERT INTO workforce_execution_operation_decisions(id,operation_id,action) VALUES($1,$2,$3)`, request.DecisionID, request.OperationID, request.Action); err != nil {
		return operationResumeAttempt{err: err}
	}
	target, err := store.FindOperationResumeTx(ctx, tx, facade.OperationResumeLookup{TenantID: request.TenantID, TaskID: request.TaskID, AttemptID: request.SourceAttemptID, OperationID: request.OperationID})
	if err != nil || target == nil {
		if err == nil {
			err = fmt.Errorf("operation resume target not found")
		}
		return operationResumeAttempt{err: err}
	}
	result, err := service.ResumeOperationTx(ctx, tx, request)
	if err != nil {
		return operationResumeAttempt{err: err}
	}
	if err = tx.Commit(ctx); err != nil {
		return operationResumeAttempt{err: err}
	}
	return operationResumeAttempt{turnID: result.TaskTurnID}
}

func insertOperationDecision(t *testing.T, ctx context.Context, tx pgx.Tx, decisionID, operationID int64, action string) {
	t.Helper()
	if _, err := tx.Exec(ctx, `INSERT INTO workforce_execution_operation_decisions(id,operation_id,action) VALUES($1,$2,$3)`, decisionID, operationID, action); err != nil {
		t.Fatal(err)
	}
}

func assertOperationSuspendInitialState(t *testing.T, ctx context.Context, db *pgxpool.Pool) {
	t.Helper()
	var currentTurnID, consumedTurnID int64
	var taskStatus, turnStatus, attemptStatus, nodeStatus, phase string
	if err := db.QueryRow(ctx, `SELECT t.current_task_turn_id,t.status,tr.status,a.status,n.status,c.interaction_phase,COALESCE(c.consumed_by_turn_id,0)
FROM workforce_tasks t
JOIN workforce_task_turns tr ON tr.id=3
JOIN workforce_task_attempts a ON a.id=4
JOIN workforce_execution_nodes n ON n.id=11
JOIN workforce_continuation_checkpoints c ON c.id=6
WHERE t.id=2`).Scan(&currentTurnID, &taskStatus, &turnStatus, &attemptStatus, &nodeStatus, &phase, &consumedTurnID); err != nil {
		t.Fatal(err)
	}
	if currentTurnID != 3 || taskStatus != "INTERACTION_SUSPENDED" || turnStatus != "INTERACTION_SUSPENDED" || attemptStatus != "INTERACTION_SUSPENDED" || nodeStatus != "INTERACTION_SUSPENDED" || phase != "SUSPENDED" || consumedTurnID != 0 {
		t.Fatalf("unexpected pre-resume state: current=%d task=%s turn=%s attempt=%s node=%s phase=%s consumed=%d", currentTurnID, taskStatus, turnStatus, attemptStatus, nodeStatus, phase, consumedTurnID)
	}
	var successorCount int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM workforce_task_turns WHERE source_execution_operation_id=50 OR source_checkpoint_id=6`).Scan(&successorCount); err != nil || successorCount != 0 {
		t.Fatalf("pre-resume successor count=%d err=%v", successorCount, err)
	}
}

func assertOperationResumeCommittedState(t *testing.T, ctx context.Context, db *pgxpool.Pool, resumedTurnID int64) {
	t.Helper()
	var (
		currentTurnID, sourceOperationID, sourceCheckpointID, sourceTurnID int64
		continuationLineageID, consumedTurnID                              int64
		taskStatus, snapshot, phase, session                               string
		oldTurnStatus, oldAttemptStatus, oldNodeStatus, newNodeStatus      string
		consumedAt                                                         *time.Time
	)
	err := db.QueryRow(ctx, `SELECT t.current_task_turn_id,t.status,
tr.source_execution_operation_id,tr.source_checkpoint_id,tr.source_turn_id,tr.execution_snapshot_digest,tr.continuation_lineage_id,
c.interaction_phase,c.session_lineage_digest,c.consumed_by_turn_id,c.consumed_at,
old.status,a.status,oldn.status,newn.status
FROM workforce_tasks t
JOIN workforce_task_turns tr ON tr.id=t.current_task_turn_id
JOIN workforce_continuation_checkpoints c ON c.id=6
JOIN workforce_task_turns old ON old.id=3
JOIN workforce_task_attempts a ON a.id=4
JOIN workforce_execution_nodes oldn ON oldn.id=11
JOIN workforce_execution_nodes newn ON newn.task_turn_id=tr.id
WHERE t.id=2`).Scan(
		&currentTurnID, &taskStatus,
		&sourceOperationID, &sourceCheckpointID, &sourceTurnID, &snapshot, &continuationLineageID,
		&phase, &session, &consumedTurnID, &consumedAt,
		&oldTurnStatus, &oldAttemptStatus, &oldNodeStatus, &newNodeStatus,
	)
	if err != nil {
		t.Fatal(err)
	}
	if currentTurnID != resumedTurnID || sourceOperationID != 50 || sourceCheckpointID != 6 || sourceTurnID != 3 {
		t.Fatalf("successor identity mismatch: current=%d operation=%d checkpoint=%d source=%d", currentTurnID, sourceOperationID, sourceCheckpointID, sourceTurnID)
	}
	if taskStatus != "QUEUED" || snapshot != "frozen" || continuationLineageID != 9 || phase != "RESUMED" || session != "private-lineage" || consumedTurnID != resumedTurnID || consumedAt == nil {
		t.Fatalf("resume inheritance mismatch: task=%s snapshot=%s lineage=%d phase=%s session=%s consumed=%d at=%v", taskStatus, snapshot, continuationLineageID, phase, session, consumedTurnID, consumedAt)
	}
	if oldTurnStatus != "SUPERSEDED" || oldAttemptStatus != "SUPERSEDED" || oldNodeStatus != "SUPERSEDED" || newNodeStatus != "QUEUED" {
		t.Fatalf("supersede mismatch: turn=%s attempt=%s node=%s newNode=%s", oldTurnStatus, oldAttemptStatus, oldNodeStatus, newNodeStatus)
	}

	for name, query := range map[string]string{
		"source operation": `SELECT count(*) FROM workforce_task_turns WHERE source_execution_operation_id=50`,
		"checkpoint":       `SELECT count(*) FROM workforce_task_turns WHERE source_checkpoint_id=6`,
		"successor":        `SELECT count(*) FROM workforce_task_turns WHERE source_turn_id=3`,
		"input snapshot":   `SELECT count(*) FROM workforce_task_input_snapshots WHERE task_turn_id=$1 AND input_snapshot_digest='input'`,
		"git plan":         `SELECT count(*) FROM workforce_task_git_plans WHERE task_turn_id=$1`,
	} {
		var count int
		var countErr error
		if strings.Contains(query, "$1") {
			countErr = db.QueryRow(ctx, query, resumedTurnID).Scan(&count)
		} else {
			countErr = db.QueryRow(ctx, query).Scan(&count)
		}
		if countErr != nil || count != 1 {
			t.Fatalf("%s count=%d err=%v", name, count, countErr)
		}
	}
}

func newOperationSuspendTestDB(t *testing.T, ctx context.Context, dsn string) *pgxpool.Pool {
	t.Helper()
	admin, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("operation_suspend_test_%d", time.Now().UnixNano())
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+pgx.Identifier{schema}.Sanitize()); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		admin.Close()
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	db, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		admin.Close()
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, suspendTestSchema); err != nil {
		db.Close()
		_, _ = admin.Exec(context.Background(), "DROP SCHEMA "+pgx.Identifier{schema}.Sanitize()+" CASCADE")
		admin.Close()
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, operationSuspendFixtureExtension); err != nil {
		db.Close()
		_, _ = admin.Exec(context.Background(), "DROP SCHEMA "+pgx.Identifier{schema}.Sanitize()+" CASCADE")
		admin.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		db.Close()
		dropCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = admin.Exec(dropCtx, "DROP SCHEMA "+pgx.Identifier{schema}.Sanitize()+" CASCADE")
		admin.Close()
	})
	return db
}

const operationSuspendFixtureExtension = `
CREATE UNIQUE INDEX workforce_turn_execution_operation_once_idx ON workforce_task_turns(source_execution_operation_id) WHERE source_execution_operation_id IS NOT NULL;
CREATE UNIQUE INDEX workforce_checkpoint_operation_once_idx ON workforce_continuation_checkpoints(operation_id) WHERE operation_id IS NOT NULL;
UPDATE workforce_continuation_checkpoints SET interaction_id=NULL,operation_id=50 WHERE id=6;
CREATE TABLE workforce_execution_operations(id BIGINT PRIMARY KEY,tenant_id BIGINT NOT NULL,task_id BIGINT NOT NULL,task_turn_id BIGINT NOT NULL,attempt_id BIGINT NOT NULL,status TEXT NOT NULL);
CREATE TABLE workforce_execution_operation_decisions(id BIGINT PRIMARY KEY,operation_id BIGINT NOT NULL,action TEXT NOT NULL);
INSERT INTO workforce_execution_operations(id,tenant_id,task_id,task_turn_id,attempt_id,status) VALUES(50,1,2,3,4,'WAITING_APPROVAL');
`
