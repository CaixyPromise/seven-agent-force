package application

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	pf "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/resultprocessor/facade"
	tf "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/facade"
	d "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/workitem/domain"
	"github.com/jackc/pgx/v5"
)

type testIDs struct{ n atomic.Int64 }

func (i *testIDs) NextID() int64 { return i.n.Add(1) }

type fakeRepo struct {
	mu        sync.Mutex
	item      d.Item
	decisions map[string]fakeDecision
}
type fakeDecision struct {
	hash string
	item d.Item
}
type fakeTx struct{ r *fakeRepo }

func (r *fakeRepo) InTx(_ context.Context, fn func(Tx) error) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	return fn(&fakeTx{r})
}
func (r *fakeRepo) Get(_ context.Context, tenant, id int64) (d.Item, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.item.ID != id || r.item.TenantID != tenant {
		return d.Item{}, d.ErrNotFound
	}
	return r.item, nil
}
func (r *fakeRepo) List(context.Context, int64, int64, int) ([]d.Item, error) { return nil, nil }
func (r *fakeRepo) Active(context.Context, int) ([]d.Item, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return []d.Item{r.item}, nil
}
func (t *fakeTx) Native() pgx.Tx { return nil }
func (t *fakeTx) Get(_ context.Context, tenant, id int64) (d.Item, error) {
	if t.r.item.ID != id || t.r.item.TenantID != tenant {
		return d.Item{}, d.ErrNotFound
	}
	return t.r.item, nil
}
func (t *fakeTx) Replay(_ context.Context, tenant, actor, scope int64, key, hash string) (*d.Item, error) {
	d0, ok := t.r.decisions[key]
	if !ok {
		return nil, nil
	}
	if d0.hash != hash {
		return nil, d.ErrConflict
	}
	i := d0.item
	return &i, nil
}
func (t *fakeTx) Insert(_ context.Context, i d.Item) error { t.r.item = i; return nil }
func (t *fakeTx) Save(_ context.Context, i d.Item) error   { t.r.item = i; return nil }
func (t *fakeTx) Decision(_ context.Context, tenant, actor, scope int64, key, hash string, i d.Item) error {
	t.r.decisions[key] = fakeDecision{hash, i}
	return nil
}

type fakeStages struct {
	next        atomic.Int64
	created     atomic.Int64
	mu          sync.Mutex
	statuses    map[int64]string
	cancelCalls int
	requests    []tf.StageRequest
	source      tf.StageSource
}
type prepared struct{ f *fakeStages }

func (p prepared) Source() tf.StageSource { return p.f.source }
func (p prepared) CreateTx(_ context.Context, _ pgx.Tx, _ string) (tf.StageTask, error) {
	id := p.f.next.Add(1) + 100
	p.f.created.Add(1)
	p.f.mu.Lock()
	p.f.statuses[id] = "RUNNING"
	p.f.mu.Unlock()
	return tf.StageTask{ID: id, Status: "RUNNING"}, nil
}
func (f *fakeStages) PrepareStage(_ context.Context, r tf.StageRequest) (tf.PreparedStage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, r)
	return prepared{f}, nil
}
func (f *fakeStages) StageTask(_ context.Context, tenant, id int64) (tf.StageTask, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return tf.StageTask{ID: id, Status: f.statuses[id]}, nil
}
func (f *fakeStages) CancelStageTask(_ context.Context, tenant, id, actor int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cancelCalls++
	f.statuses[id] = "CANCELLED"
	return nil
}

type fakeOutput struct{ body string }

func (o fakeOutput) ListAvailableOutputs(_ context.Context, tenant, task int64) ([]pf.AvailableOutput, error) {
	return []pf.AvailableOutput{{ID: 88, TaskID: task, Name: "development-plan", SizeBytes: int64(len(o.body)), SHA256: d.Hash([]byte(o.body))}}, nil
}
func (o fakeOutput) OpenAvailableOutput(_ context.Context, tenant, task, id int64) (pf.OutputDownload, error) {
	return pf.OutputDownload{Content: io.NopCloser(strings.NewReader(o.body))}, nil
}
func TestCreateApproveAndReplay(t *testing.T) {
	ctx := context.Background()
	repo := &fakeRepo{decisions: map[string]fakeDecision{}}
	stages := &fakeStages{statuses: map[int64]string{}, source: tf.StageSource{ProjectID: 7, ProjectVersionID: 8, RevisionID: 9, Kind: "SOURCE_REVISION", Commit: "abc", IdentityDigest: "digest"}}
	ids := &testIDs{}
	output := fakeOutput{`{"schema":"seven.force.development-plan/v1","title":"Plan","summary":"s","goals":["g"],"nonGoals":[],"approach":"a","affectedAreas":[],"steps":[{"key":"1","description":"step"}],"acceptanceCriteria":["done"],"risks":[],"openQuestions":[],"markdown":"md"}`}
	svc := New(repo, stages, output, ids, nil)
	selection := d.Selection{ModelSelection: tf.ModelSelection{CredentialID: 5, Model: "m"}, RuntimeReleaseID: 6}
	req := d.Create{Instruction: "build", PlanSelection: selection, ImplementationSelection: selection}
	item, err := svc.Create(ctx, 1, 2, 7, "create", req)
	if err != nil {
		t.Fatal(err)
	}
	if stages.created.Load() != 1 || item.Status != "PLANNING" {
		t.Fatal("planning not started")
	}
	again, err := svc.Create(ctx, 1, 2, 7, "create", req)
	if err != nil || again.ID != item.ID || stages.created.Load() != 1 {
		t.Fatal("create replay produced duplicate")
	}
	stages.mu.Lock()
	stages.statuses[item.Runs[0].TaskID] = "SUCCEEDED"
	stages.mu.Unlock()
	if err = svc.Reconcile(ctx); err != nil {
		t.Fatal(err)
	}
	item, err = svc.Get(ctx, 1, item.ID)
	if err != nil || item.CurrentPlan == nil || item.Status != "AWAITING_PLAN_APPROVAL" {
		t.Fatal("plan not imported", err)
	}
	r := Approve{ExpectedVersion: item.Version, PlanRevisionID: item.CurrentPlan.ID, ContentDigest: item.CurrentPlan.Digest, Decision: "APPROVE"}
	stale := r
	stale.ContentDigest = "stale"
	if _, err = svc.Approve(ctx, 1, 2, item.ID, "stale", stale); !errors.Is(err, d.ErrConflict) {
		t.Fatal("stale digest accepted", err)
	}
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, key := range []string{"a", "b"} {
		wg.Add(1)
		go func(key string) { defer wg.Done(); _, e := svc.Approve(ctx, 1, 2, item.ID, key, r); errs <- e }(key)
	}
	wg.Wait()
	close(errs)
	success := 0
	for e := range errs {
		if e == nil {
			success++
		} else if !errors.Is(e, d.ErrConflict) {
			t.Fatal(e)
		}
	}
	if success != 1 || stages.created.Load() != 2 {
		t.Fatalf("duplicate implement: success=%d tasks=%d", success, stages.created.Load())
	}
	stages.mu.Lock()
	last := stages.requests[len(stages.requests)-1]
	stages.mu.Unlock()
	if last.Purpose != "IMPLEMENT" || last.ProjectVersionID != 8 || last.SourceRevisionID != 9 || !json.Valid(last.ApprovedPlan) {
		t.Fatal("implementation did not carry approved plan and source")
	}
}
func TestCancellationReconcilesAfterRequest(t *testing.T) {
	repo := &fakeRepo{decisions: map[string]fakeDecision{}, item: d.Item{ID: 1, TenantID: 1, Status: "CANCELLING", Version: 2, CancelRequestedBy: 7, Runs: []d.Run{{TaskID: 12, Status: "RUNNING"}}}}
	stages := &fakeStages{statuses: map[int64]string{12: "RUNNING"}}
	svc := New(repo, stages, nil, &testIDs{}, nil)
	if err := svc.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if stages.cancelCalls != 1 {
		t.Fatal("cancel was not retried")
	}
	if err := svc.Reconcile(context.Background()); err != nil {
		t.Fatal(err)
	}
	if repo.item.Status != "CANCELLED" {
		t.Fatal("not cancelled", repo.item.Status)
	}
}

func TestEditPlanCreatesRevisionWithoutModelAndRejectsStaleApproval(t *testing.T) {
	ctx := context.Background()
	plan := d.Plan{Schema: "seven.force.development-plan/v1", Title: "Original", Approach: "Explore", Goals: []string{"g"}, Steps: []d.Step{{Key: "1", Description: "build"}}, AcceptanceCriteria: []string{"done"}, OpenQuestions: []d.Question{{Key: "q", Question: "scope?", Blocking: true}}}
	rev := d.Revision{ID: 51, Number: 1, TaskID: 60, OutputID: 61, Plan: plan}
	_, rev.Digest = rev.DigestBody()
	item := d.Item{ID: 10, TenantID: 11, ProjectID: 12, ProjectVersionID: 13, Creator: 14, Status: "AWAITING_PLAN_APPROVAL", Version: 2, CurrentPlan: &rev, Runs: []d.Run{{ID: 41, Stage: "PLAN", Number: 1, TaskID: 60, Status: "SUCCEEDED"}}}
	repo := &fakeRepo{item: item, decisions: map[string]fakeDecision{}}
	stages := &fakeStages{statuses: map[int64]string{60: "SUCCEEDED"}}
	svc := New(repo, stages, nil, &testIDs{}, nil)
	plan.OpenQuestions[0].Resolution = "yes"
	raw, _ := json.Marshal(plan)
	edited, err := svc.Edit(ctx, 11, 14, 10, "edit", EditPlan{ExpectedVersion: 2, PlanRevisionID: 51, ContentDigest: rev.Digest, Plan: raw})
	if err != nil {
		t.Fatal(err)
	}
	if edited.CurrentPlan.ID == 51 || edited.CurrentPlan.ParentRevisionID != 51 || edited.CurrentPlan.Origin != "USER_AUTHORED" || edited.CurrentPlan.Digest == rev.Digest || edited.CurrentPlan.Plan.HasBlockingQuestions() || stages.created.Load() != 0 {
		t.Fatal("edit did not create a safe model-free revision")
	}
	_, err = svc.Approve(ctx, 11, 14, 10, "stale", Approve{ExpectedVersion: 2, PlanRevisionID: 51, ContentDigest: rev.Digest, Decision: "APPROVE"})
	if !errors.Is(err, d.ErrConflict) {
		t.Fatal("old approval accepted", err)
	}
}

func TestCancelWithApprovalVersionAfterApprovalWins(t *testing.T) {
	source := tf.StageSource{ProjectID: 7, ProjectVersionID: 8, Kind: "SOURCE_REVISION", Commit: "original"}
	rev := d.Revision{ID: 10, Number: 1, Plan: d.Plan{Title: "plan"}, Source: source}
	_, rev.Digest = rev.DigestBody()
	repo := &fakeRepo{decisions: map[string]fakeDecision{}, item: d.Item{ID: 1, TenantID: 2, ProjectID: 7, ProjectVersionID: 8, Creator: 3, Status: "AWAITING_PLAN_APPROVAL", Version: 4, CurrentPlan: &rev, Source: source, Runs: []d.Run{{ID: 11, Stage: "PLAN", Number: 1, TaskID: 12, Status: "SUCCEEDED"}}}}
	stages := &fakeStages{statuses: map[int64]string{12: "SUCCEEDED"}, source: source}
	svc := New(repo, stages, nil, &testIDs{}, nil)
	_, err := svc.Approve(context.Background(), 2, 3, 1, "approval", Approve{ExpectedVersion: 4, PlanRevisionID: 10, ContentDigest: rev.Digest, Decision: "APPROVE"})
	if err != nil {
		t.Fatal(err)
	}
	item, err := svc.Cancel(context.Background(), 2, 3, 1, 4)
	if err != nil || item.Status != "CANCELLING" || stages.cancelCalls != 1 {
		t.Fatal("cancellation lost approval race", item.Status, err)
	}
}
