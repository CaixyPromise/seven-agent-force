package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	pf "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/principal/facade"
	tf "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/facade"
	a "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/workitem/application"
	d "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/workitem/domain"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type integrationIDs struct{ n atomic.Int64 }

func (i *integrationIDs) NextID() int64 { return i.n.Add(1) + 8000000000000000 }

type integrationPrincipals struct{}

func (integrationPrincipals) EnsureUser(_ context.Context, _ pgx.Tx, _ pf.IDGenerator, id int64) (int64, error) {
	return id, nil
}
func (integrationPrincipals) EnsureService(_ context.Context, _ pgx.Tx, _ pf.IDGenerator, _ string) (int64, error) {
	return 1, nil
}
func TestWorkItemPostgresTransactionAndDecision(t *testing.T) {
	dsn := os.Getenv("FORCE_WORKITEM_TEST_DSN")
	if dsn == "" {
		t.Skip("requires isolated migrated FORCE_WORKITEM_TEST_DSN")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	ids := &integrationIDs{}
	s := New(pool, ids, integrationPrincipals{})
	item := d.Item{ID: ids.NextID(), TenantID: 11, ProjectID: 12, ProjectVersionID: 13, Creator: 14, Name: "test", Instruction: "test", Status: "PLANNING", Version: 1, Runs: []d.Run{{ID: ids.NextID(), Stage: "PLAN", Number: 1, TaskID: 15, Status: "RUNNING"}}, Usage: json.RawMessage(`{"mode":"OBSERVE"}`)}
	err = s.InTx(ctx, func(tx a.Tx) error {
		if e := tx.Insert(ctx, item); e != nil {
			return e
		}
		return tx.Decision(ctx, 11, 14, 0, "create-key", "digest", item)
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := s.Get(ctx, 11, item.ID)
	if err != nil || got.ID != item.ID || len(got.Runs) != 1 {
		t.Fatal(got, err)
	}
	if _, err = s.Get(ctx, 12, item.ID); !errors.Is(err, d.ErrNotFound) {
		t.Fatal("cross-tenant read accepted", err)
	}
	err = s.InTx(ctx, func(tx a.Tx) error {
		got, e := tx.Replay(ctx, 11, 14, 0, "create-key", "digest")
		if e != nil {
			return e
		}
		if got == nil || got.ID != item.ID {
			t.Fatal("missing replay")
		}
		_, e = tx.Replay(ctx, 11, 14, 0, "create-key", "other")
		if !errors.Is(e, d.ErrConflict) {
			t.Fatal("key reuse accepted", e)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	fail := errors.New("rollback")
	err = s.InTx(ctx, func(tx a.Tx) error {
		x, e := tx.Get(ctx, 11, item.ID)
		if e != nil {
			return e
		}
		x.Status = "CANCELLING"
		x.Version++
		if e = tx.Save(ctx, x); e != nil {
			return e
		}
		return fail
	})
	if !errors.Is(err, fail) {
		t.Fatal(err)
	}
	got, err = s.Get(ctx, 11, item.ID)
	if err != nil || got.Status != "PLANNING" || got.Version != 1 {
		t.Fatal("transaction did not roll back", got.Status, err)
	}
}

type integrationStages struct {
	created   atomic.Int64
	cancelled atomic.Int64
	source    tf.StageSource
}
type integrationPrepared struct{ stages *integrationStages }

func (p integrationPrepared) Source() tf.StageSource { return p.stages.source }
func (p integrationPrepared) CreateTx(_ context.Context, _ pgx.Tx, _ string) (tf.StageTask, error) {
	id := p.stages.created.Add(1) + 7000000000000000
	return tf.StageTask{ID: id, Status: "QUEUED"}, nil
}
func (s *integrationStages) PrepareStage(context.Context, tf.StageRequest) (tf.PreparedStage, error) {
	return integrationPrepared{s}, nil
}
func (s *integrationStages) StageTask(_ context.Context, _ int64, id int64) (tf.StageTask, error) {
	return tf.StageTask{ID: id, Status: "SUCCEEDED"}, nil
}
func (s *integrationStages) CancelStageTask(context.Context, int64, int64, int64) error {
	s.cancelled.Add(1)
	return nil
}
func TestConcurrentApprovalPostgresCreatesOneStage(t *testing.T) {
	dsn := os.Getenv("FORCE_WORKITEM_TEST_DSN")
	if dsn == "" {
		t.Skip("requires isolated migrated FORCE_WORKITEM_TEST_DSN")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	ids := &integrationIDs{}
	ids.n.Store(time.Now().UnixNano())
	store := New(pool, ids, integrationPrincipals{})
	source := tf.StageSource{ProjectID: 12, ProjectVersionID: 13, RevisionID: 14, Kind: "SOURCE_REVISION", Commit: "frozen", IdentityDigest: "digest"}
	planTaskID := ids.NextID()
	rev := d.Revision{ID: ids.NextID(), Number: 1, TaskID: planTaskID, OutputID: 16, Plan: d.Plan{Title: "approved"}, Source: source}
	_, rev.Digest = rev.DigestBody()
	item := d.Item{ID: ids.NextID(), TenantID: 11, ProjectID: 12, ProjectVersionID: 13, Creator: 17, Name: "test", Instruction: "test", Status: "AWAITING_PLAN_APPROVAL", Version: 2, Source: source, CurrentPlan: &rev, Runs: []d.Run{{ID: ids.NextID(), Stage: "PLAN", Number: 1, TaskID: planTaskID, Status: "SUCCEEDED"}}, Usage: json.RawMessage(`{"mode":"OBSERVE"}`)}
	if err = store.InTx(ctx, func(tx a.Tx) error { return tx.Insert(ctx, item) }); err != nil {
		t.Fatal(err)
	}
	stages := &integrationStages{source: source}
	svc := a.New(store, stages, nil, ids, nil)
	req := a.Approve{ExpectedVersion: 2, PlanRevisionID: rev.ID, ContentDigest: rev.Digest, Decision: "APPROVE"}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, key := range []string{"approval-1", "approval-2"} {
		wg.Add(1)
		go func(k string) { defer wg.Done(); _, e := svc.Approve(ctx, 11, 17, item.ID, k, req); results <- e }(key)
	}
	wg.Wait()
	close(results)
	success := 0
	for e := range results {
		if e == nil {
			success++
		} else if !errors.Is(e, d.ErrConflict) {
			t.Fatal(e)
		}
	}
	if success != 1 || stages.created.Load() != 1 {
		t.Fatalf("approval race produced %d successes and %d stage calls", success, stages.created.Load())
	}
	got, err := store.Get(ctx, 11, item.ID)
	if err != nil || got.Status != "IMPLEMENTING" || len(got.Runs) != 2 {
		t.Fatal("incorrect final state", err)
	}
	var approvals int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM workforce_work_item_decisions WHERE work_item_id=$1 AND decision_kind='APPROVE_PLAN' AND plan_revision_id=$2 AND plan_content_digest=$3`, item.ID, rev.ID, rev.Digest).Scan(&approvals); err != nil || approvals != 1 {
		t.Fatal("approval audit not unique", err)
	}
}
