package infrastructure

import (
	"context"
	"errors"
	"fmt"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/completioncheck/application"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/completioncheck/facade"
	continuation "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/facade"
	principal "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/principal/infrastructure"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/xid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

// Real PostgreSQL repository/application test. The safe-point and continuation
// ports below are SQL-backed fixtures; they do NOT claim production checkpoint
// purpose, Worker quiescence, or runtime restore acceptance.
type revisionSafePointFixture struct{}

func (a revisionSafePointFixture) ResolveRevisionSafePoint(ctx context.Context, tx pgx.Tx, m facade.MachineAuthority, attempt int64, lease string, cycle int64) (*facade.RevisionSafePoint, error) {
	// This fixture has one cycle (1); production derives cycle/checkpoint using
	// existing dispatch and continuation authority, not fixture constants.
	if cycle != 1 {
		return nil, facade.ErrStale
	}
	var p facade.RevisionSafePoint
	p.Authority, p.AttemptID, p.ExecutionLease = m, attempt, lease
	e := tx.QueryRow(ctx, `SELECT t.tenant_id,t.id,c.turn,c.id FROM workforce_tasks t JOIN revision_test_checkpoints c ON c.task=t.id WHERE c.attempt=$1 FOR UPDATE OF t`, attempt).Scan(&p.TenantID, &p.TaskID, &p.TaskTurnID, &p.CheckpointID)
	if e != nil {
		return nil, facade.ErrNotFound
	}
	if e = a.ValidateRevisionSafePoint(ctx, tx, p); e != nil {
		return nil, e
	}
	r, e := NewRevisionStore().FindSafePointRevision(ctx, tx, p.TenantID, p.TaskID, p.TaskTurnID, attempt)
	if e != nil {
		return nil, e
	}
	if r == nil {
		return nil, nil
	}
	p.RevisionID, p.ExpectedVersion = r.ID, r.Version
	if r.Status == "APPLIED" {
		p.ExpectedVersion--
	}
	return &p, nil
}

func (revisionSafePointFixture) ValidateRevisionSafePoint(ctx context.Context, tx pgx.Tx, p facade.RevisionSafePoint) error {
	var status string
	if e := tx.QueryRow(ctx, `SELECT status FROM workforce_tasks WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, p.TenantID, p.TaskID).Scan(&status); e != nil {
		return facade.ErrNotFound
	}
	if status == "CANCELLED" {
		return facade.ErrActionNotAllowed
	}
	var stopped bool
	var worker, generation, owner int64
	var instance, boot, lease string
	e := tx.QueryRow(ctx, `SELECT stopped,worker,generation,owner,instance,boot,lease FROM revision_test_checkpoints WHERE id=$1 AND task=$2 AND turn=$3 AND attempt=$4 FOR UPDATE`, p.CheckpointID, p.TaskID, p.TaskTurnID, p.AttemptID).Scan(&stopped, &worker, &generation, &owner, &instance, &boot, &lease)
	if e != nil || !stopped || worker != p.Authority.WorkerID || generation != p.Authority.Generation || owner != p.Authority.OwnerEpoch || instance != p.Authority.InstanceID || boot != p.Authority.HostBootID || lease != p.ExecutionLease {
		return facade.ErrActionNotAllowed
	}
	return nil
}

type revisionTurnFixture struct {
	ids           *xid.Generator
	fail, corrupt bool
}

func (f revisionTurnFixture) CreateCompletionRevisionTurnTx(ctx context.Context, tx pgx.Tx, r continuation.CompletionRevisionTurnRequest) (continuation.CompletionRevisionTurnResult, error) {
	id := f.ids.NextID()
	var out continuation.CompletionRevisionTurnResult
	_, e := tx.Exec(ctx, `INSERT INTO workforce_task_turns(id,tenant_id,task_id,instruction,completion_check_json,completion_check_digest,completion_check_references_json,completion_check_baseline_result_id,execution_snapshot_digest,processor_release_id,processor_digest) SELECT $1,tenant_id,task_id,instruction,$2,$3,$4,NULL,execution_snapshot_digest,processor_release_id,processor_digest FROM workforce_task_turns WHERE id=$5`, id, []byte(r.InputJSON), r.CheckDigest, []byte(r.ReferencesJSON), r.SourceTaskTurnID)
	if e != nil {
		return out, e
	}
	_, e = tx.Exec(ctx, `INSERT INTO workforce_task_input_snapshots SELECT $1,tenant_id,task_id,$2,source_commit,source_identity_digest FROM workforce_task_input_snapshots WHERE task_turn_id=$3`, f.ids.NextID(), id, r.SourceTaskTurnID)
	if e != nil {
		return out, e
	}
	_, e = tx.Exec(ctx, `UPDATE workforce_tasks SET current_task_turn_id=$2,version=version+1,status='QUEUED' WHERE id=$1`, r.TaskID, id)
	if e != nil {
		return out, e
	}
	_, e = tx.Exec(ctx, `UPDATE revision_test_checkpoints SET consumed_by=$2 WHERE id=$1 AND consumed_by IS NULL`, r.CheckpointID, id)
	if e != nil {
		return out, e
	}
	if f.corrupt {
		_, e = tx.Exec(ctx, `UPDATE workforce_task_turns SET processor_digest=$2 WHERE id=$1`, id, strings.Repeat("f", 64))
		if e != nil {
			return out, e
		}
	}
	if f.fail {
		return out, errors.New("private checkpoint body secret")
	}
	return continuation.CompletionRevisionTurnResult{TaskTurnID: id}, nil
}

type revisionFixture struct {
	ctx     context.Context
	pool    *pgxpool.Pool
	ids     *xid.Generator
	service *application.RevisionService
	digest  string
}

func (f revisionFixture) exec(t *testing.T, q string, args ...any) {
	t.Helper()
	if _, e := f.pool.Exec(f.ctx, q, args...); e != nil {
		t.Fatal(e)
	}
}
func (f revisionFixture) count(t *testing.T, q string, args ...any) int64 {
	t.Helper()
	var n int64
	if e := f.pool.QueryRow(f.ctx, q, args...).Scan(&n); e != nil {
		t.Fatal(e)
	}
	return n
}
func (f revisionFixture) reset(t *testing.T) {
	f.exec(t, `TRUNCATE workforce_completion_check_revisions,workforce_tasks,workforce_task_turns,workforce_task_attempts,workforce_task_input_snapshots,workforce_task_input_references,workforce_reference_assets,workforce_completion_check_results,workforce_task_events,revision_test_checkpoints,workforce_principals,sys_user`)
	f.exec(t, `INSERT INTO sys_user VALUES(7)`)
	f.exec(t, `INSERT INTO workforce_tasks VALUES(20,10,30,7,'RUNNING')`)
	f.exec(t, `INSERT INTO workforce_task_turns VALUES(30,10,20,'Original task','{"mode":"COMMAND","command":"go test ./...","instructions":"old instructions"}',$1,'[]',60,$2,9,$2)`, f.digest, strings.Repeat("a", 64))
	f.exec(t, `INSERT INTO workforce_task_attempts VALUES(40,20,30)`)
	f.exec(t, `INSERT INTO workforce_task_input_snapshots VALUES(50,10,20,30,$1,$2)`, strings.Repeat("b", 40), strings.Repeat("c", 64))
	f.exec(t, `INSERT INTO workforce_completion_check_results VALUES(60,20,'baseline must remain unchanged')`)
	f.exec(t, `INSERT INTO revision_test_checkpoints VALUES(70,20,30,40,true,8,1,1,'instance','boot','lease',NULL)`)
}
func revisionUser() facade.RevisionUser {
	return facade.RevisionUser{TenantID: 10, TaskID: 20, UserID: 7, CanContinue: true}
}
func revisionRequest(key string) facade.RevisionRequest {
	return facade.RevisionRequest{ExpectedVersion: 7, IdempotencyKey: key, CompletionCheck: &facade.Input{Mode: "COMMAND", Command: "go test ./...", Instructions: "old instructions\nnew instructions"}}
}
func revisionPoint(id int64) facade.RevisionSafePoint {
	return facade.RevisionSafePoint{TenantID: 10, TaskID: 20, TaskTurnID: 30, AttemptID: 40, CheckpointID: 70, RevisionID: id, ExpectedVersion: 1, Authority: facade.MachineAuthority{WorkerID: 8, InstanceID: "instance", HostBootID: "boot", Generation: 1, OwnerEpoch: 1}, ExecutionLease: "lease"}
}
func (f revisionFixture) apply(s *application.RevisionService, p facade.RevisionSafePoint) (facade.Revision, error) {
	tx, e := f.pool.Begin(f.ctx)
	if e != nil {
		return facade.Revision{}, e
	}
	defer tx.Rollback(f.ctx)
	r, e := s.ApplyRevisionTx(f.ctx, tx, p)
	if e != nil {
		return r, e
	}
	return r, tx.Commit(f.ctx)
}

func TestRevisionPostgreSQL(t *testing.T) {
	dsn := os.Getenv("COMPLETION_CHECK_REVISION_TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("NOT RUN: set COMPLETION_CHECK_REVISION_TEST_POSTGRES_DSN to the authorized disposable QA database")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	admin, e := pgxpool.New(ctx, dsn)
	if e != nil {
		t.Fatal("PostgreSQL configuration failed")
	}
	defer admin.Close()
	if e = admin.Ping(ctx); e != nil {
		t.Fatal("PostgreSQL connection failed")
	}
	schema := fmt.Sprintf("completion_revision_test_%d", time.Now().UnixNano())
	quoted := pgx.Identifier{schema}.Sanitize()
	if _, e = admin.Exec(ctx, "CREATE SCHEMA "+quoted); e != nil {
		t.Fatal(e)
	}
	defer func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, e := admin.Exec(cleanup, "DROP SCHEMA "+quoted+" CASCADE"); e != nil {
			t.Error("fixture schema cleanup failed")
		}
	}()
	cfg, e := pgxpool.ParseConfig(dsn)
	if e != nil {
		t.Fatal("PostgreSQL configuration failed")
	}
	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	pool, e := pgxpool.NewWithConfig(ctx, cfg)
	if e != nil {
		t.Fatal(e)
	}
	defer pool.Close()
	ids, e := xid.New(49)
	if e != nil {
		t.Fatal(e)
	}
	f := revisionFixture{ctx: ctx, pool: pool, ids: ids}
	f.digest, _ = facade.Digest(facade.Input{Mode: "COMMAND", Command: "go test ./...", Instructions: "old instructions"})
	f.service = application.NewRevisions(pool, ids, NewRevisionStore(), principal.NewResolver(), revisionSafePointFixture{}, revisionTurnFixture{ids: ids})
	f.exec(t, RevisionSchemaSQL)
	f.exec(t, RevisionSchemaSQL)
	f.exec(t, `CREATE TABLE workforce_tasks(id BIGINT PRIMARY KEY,tenant_id BIGINT,current_task_turn_id BIGINT,version BIGINT,status VARCHAR(32));
 CREATE TABLE workforce_task_turns(id BIGINT PRIMARY KEY,tenant_id BIGINT,task_id BIGINT,instruction TEXT,completion_check_json JSONB,completion_check_digest VARCHAR(64),completion_check_references_json JSONB,completion_check_baseline_result_id BIGINT,execution_snapshot_digest VARCHAR(64),processor_release_id BIGINT,processor_digest VARCHAR(64));
 CREATE TABLE workforce_task_attempts(id BIGINT PRIMARY KEY,task_id BIGINT,task_turn_id BIGINT);
 CREATE TABLE workforce_task_input_snapshots(id BIGINT PRIMARY KEY,tenant_id BIGINT,task_id BIGINT,task_turn_id BIGINT,source_commit VARCHAR(64),source_identity_digest VARCHAR(64));
 CREATE TABLE workforce_reference_assets(id BIGINT PRIMARY KEY,tenant_id BIGINT,display_name TEXT,content_sha256 VARCHAR(64),object_sha256 VARCHAR(64),size_bytes BIGINT);
 CREATE TABLE workforce_task_input_references(task_input_snapshot_id BIGINT,reference_asset_id BIGINT,reference_content_sha256 VARCHAR(64));
 CREATE TABLE workforce_completion_check_results(id BIGINT PRIMARY KEY,task_id BIGINT,observation TEXT);
 CREATE TABLE workforce_task_events(task_id BIGINT,attempt_id BIGINT,event_type VARCHAR(64),sequence BIGINT,payload_json JSONB,UNIQUE(task_id,sequence));
 CREATE TABLE sys_user(id BIGINT PRIMARY KEY);
 CREATE TABLE workforce_principals(id BIGINT PRIMARY KEY,principal_type VARCHAR(32),user_id BIGINT);
 CREATE UNIQUE INDEX revision_test_user_principal ON workforce_principals(user_id) WHERE principal_type='USER';
 CREATE TABLE revision_test_checkpoints(id BIGINT PRIMARY KEY,task BIGINT,turn BIGINT,attempt BIGINT,stopped BOOLEAN,worker BIGINT,generation BIGINT,owner BIGINT,instance TEXT,boot TEXT,lease TEXT,consumed_by BIGINT);`)
	t.Run("request replay unique pending leaves running source unchanged", func(t *testing.T) {
		f.reset(t)
		view, e := f.service.RevisionView(ctx, revisionUser())
		if e != nil || len(view.AllowedActions) != 1 || view.AllowedActions[0] != facade.RequestRevision || view.Pending != nil {
			t.Fatal(view, e)
		}
		queryOnly := revisionUser()
		queryOnly.CanContinue = false
		view, e = f.service.RevisionView(ctx, queryOnly)
		if e != nil || len(view.AllowedActions) != 0 {
			t.Fatal(view, e)
		}
		r, e := f.service.RequestRevision(ctx, revisionUser(), revisionRequest("one"))
		if e != nil || r.Status != "PENDING" || r.SourceBaselineResultID != 60 {
			t.Fatal(r, e)
		}
		view, e = f.service.RevisionView(ctx, revisionUser())
		if e != nil || len(view.AllowedActions) != 0 || view.Pending == nil || view.Pending.ID != r.ID {
			t.Fatal(view, e)
		}
		replay, e := f.service.RequestRevision(ctx, revisionUser(), revisionRequest("one"))
		if e != nil || !replay.Replayed || replay.ID != r.ID {
			t.Fatal(replay, e)
		}
		if _, e = f.service.RequestRevision(ctx, revisionUser(), revisionRequest("two")); !errors.Is(e, facade.ErrConflict) {
			t.Fatal(e)
		}
		changed := revisionRequest("one")
		changed.CompletionCheck.Instructions += "\nother"
		if _, e = f.service.RequestRevision(ctx, revisionUser(), changed); !errors.Is(e, facade.ErrIdempotencyConflict) {
			t.Fatal(e)
		}
		if f.count(t, `SELECT current_task_turn_id FROM workforce_tasks`) != 30 || f.count(t, `SELECT version FROM workforce_tasks`) != 7 || f.count(t, `SELECT count(*) FROM workforce_task_turns`) != 1 {
			t.Fatal("request mutated running execution")
		}
		if f.count(t, `SELECT count(*) FROM workforce_principals WHERE id=$1 AND principal_type='USER' AND user_id=7`, r.PrincipalID) != 1 || r.PrincipalID == 7 {
			t.Fatal("unstructured actor")
		}
	})
	t.Run("machine derived apply replay and fencing", func(t *testing.T) {
		f.reset(t)
		p := revisionPoint(0)
		empty, e := f.service.ApplyPendingAtSafePoint(ctx, p.Authority, 40, "lease", 1)
		if e != nil || empty.Applied {
			t.Fatal(empty, e)
		}
		if _, e = f.service.RequestRevision(ctx, revisionUser(), revisionRequest("machine-apply")); e != nil {
			t.Fatal(e)
		}
		first, e := f.service.ApplyPendingAtSafePoint(ctx, p.Authority, 40, "lease", 1)
		if e != nil || !first.Applied || first.TaskTurnID <= 0 {
			t.Fatal(first, e)
		}
		again, e := f.service.ApplyPendingAtSafePoint(ctx, p.Authority, 40, "lease", 1)
		if e != nil || again != first || f.count(t, `SELECT count(*) FROM workforce_task_turns`) != 2 {
			t.Fatal(again, e)
		}
		if _, e = f.service.ApplyPendingAtSafePoint(ctx, p.Authority, 40, "wrong", 1); e == nil {
			t.Fatal("replay bypassed lease")
		}
	})
	t.Run("concurrent pending request arbitration", func(t *testing.T) {
		f.reset(t)
		var wg sync.WaitGroup
		errs := make(chan error, 2)
		for _, key := range []string{"a", "b"} {
			wg.Add(1)
			go func(key string) {
				defer wg.Done()
				_, e := f.service.RequestRevision(ctx, revisionUser(), revisionRequest(key))
				errs <- e
			}(key)
		}
		wg.Wait()
		close(errs)
		success := 0
		for e := range errs {
			if e == nil {
				success++
			} else if !errors.Is(e, facade.ErrConflict) {
				t.Fatal(e)
			}
		}
		if success != 1 || f.count(t, `SELECT count(*) FROM workforce_completion_check_revisions WHERE status='PENDING'`) != 1 {
			t.Fatal("multiple pending versions")
		}
	})
	t.Run("permission stale version and non additive rejection", func(t *testing.T) {
		f.reset(t)
		u := revisionUser()
		u.CanContinue = false
		if _, e := f.service.RequestRevision(ctx, u, revisionRequest("a")); !errors.Is(e, facade.ErrActionNotAllowed) {
			t.Fatal(e)
		}
		u = revisionUser()
		u.TenantID = 99
		if _, e := f.service.RequestRevision(ctx, u, revisionRequest("a")); !errors.Is(e, facade.ErrNotFound) {
			t.Fatal(e)
		}
		r := revisionRequest("a")
		r.ExpectedVersion = 6
		if _, e := f.service.RequestRevision(ctx, revisionUser(), r); !errors.Is(e, facade.ErrStale) {
			t.Fatal(e)
		}
		r = revisionRequest("a")
		r.CompletionCheck.Command = "true"
		if _, e := f.service.RequestRevision(ctx, revisionUser(), r); !errors.Is(e, facade.ErrInvalid) {
			t.Fatal(e)
		}
		f.exec(t, `UPDATE workforce_tasks SET status='CANCELLED'`)
		if _, e := f.service.RequestRevision(ctx, revisionUser(), revisionRequest("a")); !errors.Is(e, facade.ErrActionNotAllowed) {
			t.Fatal(e)
		}
		if f.count(t, `SELECT count(*) FROM workforce_completion_check_revisions`) != 0 {
			t.Fatal("rejected request persisted")
		}
	})
	t.Run("references require frozen task authorization", func(t *testing.T) {
		f.reset(t)
		r := revisionRequest("refs")
		r.CompletionCheck.ReferenceAssetIDs = []int64{81}
		if _, e := f.service.RequestRevision(ctx, revisionUser(), r); !errors.Is(e, facade.ErrInvalid) {
			t.Fatal(e)
		}
		f.exec(t, `INSERT INTO workforce_reference_assets VALUES(81,99,'wrong tenant',$1,$1,12)`, strings.Repeat("d", 64))
		f.exec(t, `INSERT INTO workforce_task_input_references VALUES(50,81,$1)`, strings.Repeat("d", 64))
		if _, e := f.service.RequestRevision(ctx, revisionUser(), r); !errors.Is(e, facade.ErrInvalid) {
			t.Fatal(e)
		}
		f.exec(t, `UPDATE workforce_reference_assets SET tenant_id=10`)
		pending, e := f.service.RequestRevision(ctx, revisionUser(), r)
		if e != nil || len(pending.References) != 1 {
			t.Fatal(pending, e)
		}
		f.exec(t, `UPDATE workforce_reference_assets SET object_sha256=$1`, strings.Repeat("e", 64))
		if _, e = f.apply(f.service, revisionPoint(pending.ID)); !errors.Is(e, facade.ErrConflict) {
			t.Fatal(e)
		}
		if f.count(t, `SELECT count(*) FROM workforce_task_turns`) != 1 {
			t.Fatal("changed reference applied")
		}
	})
	t.Run("pending reader and safe point apply preserves baseline", func(t *testing.T) {
		f.reset(t)
		r, e := f.service.RequestRevision(ctx, revisionUser(), revisionRequest("apply"))
		if e != nil {
			t.Fatal(e)
		}
		for _, status := range []string{"VALIDATING", "REVIEWING"} {
			f.exec(t, `UPDATE workforce_tasks SET status=$1 WHERE id=20`, status)
			tx, err := pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			pending, err := f.service.PendingRevisionTx(ctx, tx, 10, 20, 30)
			_ = tx.Rollback(ctx)
			if err != nil || pending == nil || pending.ID != r.ID {
				t.Fatal(status, pending, err)
			}
		}
		f.exec(t, `UPDATE workforce_tasks SET status='RUNNING' WHERE id=20`)
		tx, e := pool.Begin(ctx)
		if e != nil {
			t.Fatal(e)
		}
		pending, e := f.service.PendingRevisionTx(ctx, tx, 10, 20, 30)
		_ = tx.Rollback(ctx)
		if e != nil || pending == nil || pending.ID != r.ID {
			t.Fatal(pending, e)
		}
		applied, e := f.apply(f.service, revisionPoint(r.ID))
		if e != nil || applied.Status != "APPLIED" || applied.AppliedTurnID <= 0 || applied.Version != 2 {
			t.Fatal(applied, e)
		}
		if f.count(t, `SELECT completion_check_baseline_result_id FROM workforce_task_turns WHERE id=30`) != 60 || f.count(t, `SELECT count(*) FROM workforce_completion_check_results WHERE observation='baseline must remain unchanged'`) != 1 || f.count(t, `SELECT count(*) FROM workforce_task_turns WHERE id=$1 AND completion_check_baseline_result_id IS NULL`, applied.AppliedTurnID) != 1 {
			t.Fatal("baseline evidence overwritten/reused under new digest")
		}
		if f.count(t, `SELECT count(*) FROM workforce_task_events`) != 2 {
			t.Fatal("pending/apply facts missing")
		}
		replayed, e := f.apply(f.service, revisionPoint(r.ID))
		if e != nil || !replayed.Replayed || replayed.AppliedTurnID != applied.AppliedTurnID {
			t.Fatal(replayed, e)
		}
	})
	t.Run("unsafe point cancelled lease missing implementation fail closed", func(t *testing.T) {
		f.reset(t)
		r, e := f.service.RequestRevision(ctx, revisionUser(), revisionRequest("safe"))
		if e != nil {
			t.Fatal(e)
		}
		p := revisionPoint(r.ID)
		p.Authority.OwnerEpoch = 2
		if _, e = f.apply(f.service, p); !errors.Is(e, facade.ErrActionNotAllowed) {
			t.Fatal(e)
		}
		f.exec(t, `UPDATE revision_test_checkpoints SET stopped=false`)
		if _, e = f.apply(f.service, revisionPoint(r.ID)); !errors.Is(e, facade.ErrActionNotAllowed) {
			t.Fatal(e)
		}
		f.exec(t, `UPDATE revision_test_checkpoints SET stopped=true`)
		missing := application.NewRevisions(pool, ids, NewRevisionStore(), principal.NewResolver(), revisionSafePointFixture{}, nil)
		if _, e = f.apply(missing, revisionPoint(r.ID)); !errors.Is(e, facade.ErrRevisionUnavailable) {
			t.Fatal(e)
		}
		f.exec(t, `UPDATE workforce_tasks SET status='CANCELLED'`)
		if _, e = f.apply(f.service, revisionPoint(r.ID)); !errors.Is(e, facade.ErrActionNotAllowed) {
			t.Fatal(e)
		}
	})
	t.Run("new turn failure and identity drift rollback whole transaction", func(t *testing.T) {
		for _, turns := range []revisionTurnFixture{{ids: ids, fail: true}, {ids: ids, corrupt: true}} {
			f.reset(t)
			r, e := f.service.RequestRevision(ctx, revisionUser(), revisionRequest("rollback"))
			if e != nil {
				t.Fatal(e)
			}
			s := application.NewRevisions(pool, ids, NewRevisionStore(), principal.NewResolver(), revisionSafePointFixture{}, turns)
			if _, e = f.apply(s, revisionPoint(r.ID)); e == nil || strings.Contains(e.Error(), "secret") {
				t.Fatal(e)
			}
			if f.count(t, `SELECT count(*) FROM workforce_task_turns`) != 1 || f.count(t, `SELECT current_task_turn_id FROM workforce_tasks`) != 30 || f.count(t, `SELECT count(*) FROM revision_test_checkpoints WHERE consumed_by IS NULL`) != 1 || f.count(t, `SELECT count(*) FROM workforce_completion_check_revisions WHERE status='PENDING'`) != 1 {
				t.Fatal("partial apply escaped rollback")
			}
		}
	})
}
