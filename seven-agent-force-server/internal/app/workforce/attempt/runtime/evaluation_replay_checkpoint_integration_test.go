package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/checkpoint"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/facade"
	processorstore "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/infrastructure"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/xid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Joint local evidence only: committed SQL authority -> actual Worker pending
// replay over a modeled Supervisor pipe -> production checkpoint pack/restore.
// It does NOT exercise PersistEvaluation, a real Supervisor or Worker restart,
// encrypted object storage, checkpoint registration, or capacity release.
func TestPendingACKPostgresAuthorityCheckpointJoint(t *testing.T) {
	dsn := os.Getenv("RESULT_PROCESSOR_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("requires existing isolated RESULT_PROCESSOR_TEST_POSTGRES_DSN")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal("test database connection failed")
	}
	t.Cleanup(db.Close)
	var database string
	if err = db.QueryRow(ctx, "SELECT current_database()").Scan(&database); err != nil || !strings.HasPrefix(database, "seven_worker_batch2_") || strings.Contains(database, "live") {
		t.Fatal("isolated non-live Batch2 test database required")
	}
	gen, err := xid.New(49)
	if err != nil {
		t.Fatal(err)
	}
	task, turn, attempt, cycle := gen.NextID(), gen.NextID(), gen.NextID(), gen.NextID()
	tenant, actor, lineage, release, harness, worker, node, assignment := gen.NextID(), gen.NextID(), gen.NextID(), gen.NextID(), gen.NextID(), gen.NextID(), gen.NextID(), gen.NextID()
	wait := waitDirective(t)
	wait.CycleID = cycle
	leaseHash := sha256.Sum256([]byte("joint-fixture-lease"))
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	statements := []struct {
		sql  string
		args []any
	}{
		{`INSERT INTO workforce_tasks(id,tenant_id,created_by,idempotency_key,request_hash,repository_url,repository_ref,task_instruction,harness_alias,status,current_task_turn_id) VALUES($1,$2,$3,$4,'fixture','','','joint fixture','fixture','REVIEWING',$5)`, []any{task, tenant, actor, fmt.Sprintf("joint-%d", task), turn}},
		{`INSERT INTO workforce_task_turns(id,task_id,tenant_id,turn_number,instruction,execution_snapshot_digest,continuation_lineage_id,created_by,processor_release_id,processor_digest,session_digest) VALUES($1,$2,$3,1,'fixture','snapshot',$4,$5,$6,'processor','session')`, []any{turn, task, tenant, lineage, actor, release}},
		{`INSERT INTO workforce_task_execution_snapshots(task_id,tenant_id,harness_release_id,harness_image_digest,isolation_profile,snapshot_json,result_processor_release_id,result_processor_digest,max_evaluation_cycles) VALUES($1,$2,$3,'sha256:fixture','fixture','{}',$4,'processor',3)`, []any{task, tenant, harness, release}},
		{`INSERT INTO workforce_task_attempts(id,task_id,attempt_no,worker_id,workspace_path,status,lease_token,lease_expires_at,runtime_driver,task_turn_id,execution_node_id,worker_instance_id,host_boot_id) VALUES($1,$2,1,$3,'fixture','RUNNING',$4,CURRENT_TIMESTAMP+INTERVAL '1 hour','docker',$5,$6,'joint-instance','joint-boot')`, []any{attempt, task, worker, hex.EncodeToString(leaseHash[:]), turn, node}},
		{`INSERT INTO workforce_attempt_runtimes(attempt_id,execution_node_id,assignment_id,worker_id,worker_instance_id,host_boot_id,generation,execution_identity_digest,expected_container_name,harness_image_digest,status) VALUES($1,$2,$3,$4,'joint-instance','joint-boot',1,'identity','joint-fixture','sha256:fixture','RUNNING')`, []any{attempt, node, assignment, worker}},
		{`INSERT INTO workforce_result_evaluation_cycles(id,task_id,task_turn_id,execution_node_id,attempt_id,cycle_number,session_digest,contract_digest,decision,reason_code,contract_json) VALUES($1,$2,$3,$4,$5,1,'session',$6,'WAIT_USER','CHECK_UNAVAILABLE',$7)`, []any{cycle, task, turn, node, attempt, wait.ContractDigest, string(wait.Contract)}},
	}
	for _, s := range statements {
		if _, err = tx.Exec(ctx, s.sql, s.args...); err != nil {
			t.Fatal(err)
		}
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	// Public Store.ReadEvaluation owns its transaction, so this fixture must be
	// committed. Cleanup addresses only the exact newly allocated identities.
	t.Cleanup(func() {
		cleanupCtx, done := context.WithTimeout(context.Background(), 15*time.Second)
		defer done()
		for _, target := range []struct {
			table, key string
			id         int64
		}{
			{"workforce_result_evaluation_cycles", "id", cycle}, {"workforce_attempt_runtimes", "attempt_id", attempt},
			{"workforce_task_attempts", "id", attempt}, {"workforce_task_execution_snapshots", "task_id", task},
			{"workforce_task_turns", "id", turn}, {"workforce_tasks", "id", task},
		} {
			if _, e := db.Exec(cleanupCtx, "DELETE FROM "+target.table+" WHERE "+target.key+"=$1", target.id); e != nil {
				t.Errorf("fixture cleanup %s failed: %v", target.table, e)
				continue
			}
			var n int
			if e := db.QueryRow(cleanupCtx, "SELECT count(*) FROM "+target.table+" WHERE "+target.key+"=$1", target.id).Scan(&n); e != nil || n != 0 {
				t.Errorf("fixture cleanup verification %s count=%d err=%v", target.table, n, e)
			}
		}
		t.Logf("cleanup checked exact task=%d turn=%d attempt=%d cycle=%d across six fixture tables", task, turn, attempt, cycle)
	})
	store := processorstore.New(db, gen)
	owner := facade.MachineIdentity{WorkerID: worker, InstanceID: "joint-instance", HostBootID: "joint-boot", Generation: 1, OwnerEpoch: 1}
	item, a, state := pendingFixture()
	item.TaskID, item.ExecutionSnapshot.ResultProcessorReleaseID = task, release
	a.ID, a.OwnerEpoch = attempt, 1
	state.TaskID, state.AttemptID, state.EvaluationCycleID, state.OwnerEpoch = task, attempt, cycle, 1
	root := t.TempDir()
	var servers sync.WaitGroup
	var mu sync.Mutex
	var commands []string
	var acks []map[string]any
	dials := 0
	session := newExecutionSession(ctx, task, attempt, 1, 1, func(context.Context, string) (executionTransport, error) {
		dials++
		client, server := net.Pipe()
		servers.Add(1)
		go func() {
			defer servers.Done()
			defer server.Close()
			dec := json.NewDecoder(server)
			for {
				var frame struct {
					ID      string         `json:"id"`
					Request map[string]any `json:"request"`
				}
				if dec.Decode(&frame) != nil {
					return
				}
				command, _ := frame.Request["command"].(string)
				mu.Lock()
				commands = append(commands, command)
				lost := false
				if command == "continue-evaluation" {
					acks = append(acks, frame.Request)
					lost = len(acks) == 1
				}
				mu.Unlock()
				if lost {
					return
				} // ACK request received, no response; pending retained.
				response := supervisorResponse{OK: true, State: state}
				if command != "state" && command != "evaluation-state" && command != "continue-evaluation" {
					response.OK = false
					response.ReasonCode = "UNEXPECTED_JOINT_COMMAND"
				}
				if json.NewEncoder(server).Encode(map[string]any{"id": frame.ID, "response": response}) != nil {
					return
				}
			}
		}()
		return client, nil
	})
	t.Cleanup(func() { _ = session.Close(); servers.Wait() })
	d := &Docker{}
	d.cfg.WorkspaceRoot = root
	reads := 0
	gate := RuntimeGate{
		EvaluationReady: func(int64, int) error { t.Fatal("must not create new cycle"); return nil },
		MaterializeEvaluation: func(int64, int) (facade.Materialization, error) {
			t.Fatal("must not rematerialize")
			return facade.Materialization{}, nil
		},
		CheckEvaluation: func(context.Context, int64) error { t.Fatal("must not rerun Processor"); return nil },
		PersistEvaluation: func(facade.EvaluationCandidate) (facade.EvaluationDirective, error) {
			t.Fatal("must not resubmit persisted evaluation")
			return facade.EvaluationDirective{}, nil
		},
		ReadEvaluation: func(c context.Context, p facade.EvaluationReplayIdentity) (facade.EvaluationReplay, error) {
			reads++
			if p.CycleID != cycle || p.CycleNumber != 1 || p.SessionDigest != "session" {
				t.Fatal("pending identity changed")
			}
			result, e := store.ReadEvaluation(c, owner, attempt, "joint-fixture-lease", p)
			if e == nil && (!result.Found || result.Directive.ContractDigest != wait.ContractDigest || result.Directive.Directive != facade.DirectiveWaitForUser) {
				t.Fatal("DB authority differs")
			}
			return result, e
		},
	}
	sessionCtx := context.WithValue(ctx, executionSessionKey{}, session)
	if e := d.driveResultEvaluations(sessionCtx, "pipe-only", item, a, Workspace{}, gate); e != nil {
		t.Fatalf("single drive must recover lost ACK: %v", e)
	}
	_ = session.Close()
	servers.Wait()
	wantCommands := []string{"state", "evaluation-state", "continue-evaluation", "state", "evaluation-state", "continue-evaluation"}
	if reads != 2 || dials != 2 || !reflect.DeepEqual(commands, wantCommands) || len(acks) != 2 || !reflect.DeepEqual(acks[0], acks[1]) {
		t.Fatalf("replay mismatch reads=%d dials=%d commands=%v", reads, dials, commands)
	}
	if acks[1]["cycleId"] != strconv.FormatInt(cycle, 10) || acks[1]["decision"] != "WAIT_USER" || acks[1]["directive"] != facade.DirectiveWaitForUser {
		t.Fatal("ACK authority mismatch")
	}
	stale := owner
	stale.OwnerEpoch++
	if _, e := store.ReadEvaluation(ctx, stale, attempt, "joint-fixture-lease", facade.EvaluationReplayIdentity{CycleID: cycle, CycleNumber: 1, SessionDigest: "session"}); !errors.Is(e, processorstore.ErrConflict) {
		t.Fatalf("stale owner accepted: %v", e)
	}
	contract, e := ReadAuthoritativeEvaluation(root, task, a, cycle, "WAIT_USER")
	if e != nil {
		t.Fatal(e)
	}
	if !reflect.DeepEqual(contract.Continuation.Includes, []string{"result.txt"}) {
		t.Fatal("DB continuation plan lost")
	}
	repo, stateRoot := filepath.Join(root, "repo"), filepath.Join(root, "state")
	for _, p := range []string{repo, stateRoot} {
		if e = os.MkdirAll(p, 0700); e != nil {
			t.Fatal(e)
		}
	}
	write := func(p, body string) {
		t.Helper()
		if e := os.WriteFile(p, []byte(body), 0600); e != nil {
			t.Fatal(e)
		}
	}
	git := func(dir string, args ...string) string {
		t.Helper()
		cmd := exec.CommandContext(ctx, "git", append([]string{"-c", "core.hooksPath=/dev/null", "-c", "core.fsmonitor=false", "-C", dir}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_TERMINAL_PROMPT=0", "GIT_ALLOW_PROTOCOL=file")
		b, e := cmd.CombinedOutput()
		if e != nil {
			t.Fatalf("fixture git %s: %v", args[0], e)
		}
		return strings.TrimSpace(string(b))
	}
	git(repo, "init", "-q")
	git(repo, "config", "user.name", "Joint Test")
	git(repo, "config", "user.email", "joint@example.invalid")
	write(filepath.Join(repo, "tracked.txt"), "trusted tree\n")
	write(filepath.Join(repo, ".gitignore"), "result.txt\nprivate.txt\n")
	git(repo, "add", ".")
	git(repo, "commit", "-qm", "fixture")
	commit, tree := git(repo, "rev-parse", "HEAD"), git(repo, "rev-parse", "HEAD^{tree}")
	write(filepath.Join(repo, "result.txt"), "selected continuation\n")
	write(filepath.Join(repo, "private.txt"), "not selected\n")
	write(filepath.Join(stateRoot, "session.json"), `{"session":"session","round":1}`)
	input := checkpoint.Input{TaskID: task, TaskTurnID: turn, AttemptID: attempt, Generation: 1, EvaluationCycleID: cycle, Decision: contract.Decision, SessionDigest: state.SessionDigest, RepoRoot: repo, StateRoot: stateRoot, BaseCommit: commit, ResultCommit: commit, ResultTree: tree, Outputs: contract.Outputs, ContinuationIncludes: contract.Continuation.Includes}
	frozen, e := checkpoint.FreezeToSpool(ctx, input, filepath.Join(root, "spool"))
	if e != nil {
		t.Fatal(e)
	}
	if frozen.FormatVersion != 2 || frozen.Identity != checkpoint.Identity(input) {
		t.Fatal("checkpoint identity mismatch")
	}
	for _, blob := range []checkpoint.SpoolBlob{frozen.RepositoryBundle, frozen.WorkspaceOverlay, frozen.Session, frozen.Manifest} {
		b, e := os.ReadFile(blob.Path)
		if e != nil {
			t.Fatal(e)
		}
		sum := sha256.Sum256(b)
		if hex.EncodeToString(sum[:]) != blob.SHA256 || int64(len(b)) != blob.SizeBytes {
			t.Fatal("checkpoint blob integrity mismatch")
		}
	}
	manifestBytes, e := os.ReadFile(frozen.Manifest.Path)
	if e != nil {
		t.Fatal(e)
	}
	var manifest map[string]any
	if e = json.Unmarshal(manifestBytes, &manifest); e != nil {
		t.Fatal(e)
	}
	for key, want := range map[string]string{"taskId": strconv.FormatInt(task, 10), "taskTurnId": strconv.FormatInt(turn, 10), "attemptId": strconv.FormatInt(attempt, 10), "evaluationCycleId": strconv.FormatInt(cycle, 10), "decision": "WAIT_USER", "sessionDigest": "session", "resultCommit": commit, "resultTree": tree, "checkpointIdentity": frozen.Identity} {
		if manifest[key] != want {
			t.Fatalf("manifest %s mismatch", key)
		}
	}
	restored, restoredState := filepath.Join(root, "restored"), filepath.Join(root, "restored-state")
	if e = os.MkdirAll(restored, 0700); e != nil {
		t.Fatal(e)
	}
	git(restored, "init", "-q")
	git(restored, "fetch", "-q", frozen.RepositoryBundle.Path, "refs/seven/checkpoints/*:refs/seven/checkpoints/*")
	git(restored, "checkout", "--detach", "-q", commit)
	for _, part := range []struct {
		blob           checkpoint.SpoolBlob
		target, prefix string
	}{{frozen.WorkspaceOverlay, restored, "overlay"}, {frozen.Session, restoredState, "session"}} {
		f, e := os.Open(part.blob.Path)
		if e != nil {
			t.Fatal(e)
		}
		if part.prefix == "overlay" {
			e = checkpoint.ExtractOverlay(ctx, f, part.target, part.prefix, checkpoint.MaxArchiveBytes)
		} else {
			e = checkpoint.Extract(ctx, f, part.target, part.prefix, checkpoint.MaxSessionArchiveBytes)
		}
		_ = f.Close()
		if e != nil {
			t.Fatal(e)
		}
	}
	if git(restored, "rev-parse", "HEAD") != commit || git(restored, "rev-parse", "HEAD^{tree}") != tree {
		t.Fatal("restored git identity differs")
	}
	for path, want := range map[string]string{filepath.Join(restored, "tracked.txt"): "trusted tree\n", filepath.Join(restored, "result.txt"): "selected continuation\n", filepath.Join(restoredState, "session.json"): `{"session":"session","round":1}`} {
		b, e := os.ReadFile(path)
		if e != nil || string(b) != want {
			t.Fatalf("restored %s mismatch: %v", filepath.Base(path), e)
		}
	}
	if _, e = os.Stat(filepath.Join(restored, "private.txt")); !os.IsNotExist(e) {
		t.Fatal("undeclared private file restored")
	}
	for _, table := range []string{"workforce_result_evaluation_cycles", "workforce_task_turns"} {
		var count int
		if e = db.QueryRow(ctx, "SELECT count(*) FROM "+table+" WHERE task_id=$1", task).Scan(&count); e != nil || count != 1 {
			t.Fatalf("duplicate fixture %s: %d %v", table, count, e)
		}
	}
	t.Logf("joint PASS database=%s task=%d cycle=%d: DB WAIT_USER reads=2 ACK responses lost=1 identical ACKs=2 reconnect state-first; restored bundle/tree, selected overlay and session; checkpoint=%s", database, task, cycle, frozen.Identity)
}
