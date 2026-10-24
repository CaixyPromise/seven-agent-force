package infrastructure

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	artifactinfra "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/artifact/infrastructure"
	continuationinfra "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/continuation/infrastructure"
	routingfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/executionrouting/facade"
	routinginfra "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/executionrouting/infrastructure"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/reviewrun/domain"
	reviewfacade "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/reviewrun/facade"
	taskinfra "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/infrastructure"
	taskdispatchdomain "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/taskdispatch/domain"
	tasknotification "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/tasknotification/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/shared/stepup"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

const reviewRunTestDSNEnv = "REVIEW_RUN_TEST_POSTGRES_DSN"

type reviewRunTestIDGen struct{ next atomic.Int64 }

type reviewRunQueryCounter struct{ count atomic.Int64 }

func (c *reviewRunQueryCounter) TraceQueryStart(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryStartData) context.Context {
	c.count.Add(1)
	return ctx
}

func (*reviewRunQueryCounter) TraceQueryEnd(context.Context, *pgx.Conn, pgx.TraceQueryEndData) {}

func newReviewRunTestIDGen() *reviewRunTestIDGen {
	g := &reviewRunTestIDGen{}
	// This stays below PostgreSQL BIGINT while avoiding collisions with the
	// fixture ranges used by the independently-versioned Workforce packages.
	g.next.Store(time.Now().UTC().UnixNano())
	return g
}

func (g *reviewRunTestIDGen) NextID() int64 { return g.next.Add(1) }

func newReviewRunTestStore(db *pgxpool.Pool, ids IDGenerator) *Store {
	store := New(db, ids)
	store.SetAssignmentResolver(routinginfra.NewResolver(routingfacade.PlatformIdentity{}))
	return store
}

func TestReviewRunHistoryUsesBoundedQueryAndTaskCursorIndex(t *testing.T) {
	ctx, db, ids := openReviewRunTestDB(t)
	store := newReviewRunTestStore(db, ids)
	fixture := installReviewRunFixture(t, ctx, db, ids, true)
	t.Cleanup(func() { fixture.cleanup(ctx, db) })
	firstRunID := createReviewRun(t, ctx, db, store, fixture)

	allIDs := []int64{firstRunID}
	for ordinal := 2; ordinal <= 6; ordinal++ {
		turnID, runID := ids.NextID(), ids.NextID()
		if _, err := db.Exec(ctx, `INSERT INTO workforce_task_turns
(id,task_id,tenant_id,turn_number,instruction,execution_snapshot_digest,continuation_lineage_id,created_by,processor_release_id,processor_digest,status)
SELECT $1,task_id,tenant_id,$2,instruction,execution_snapshot_digest,continuation_lineage_id,created_by,processor_release_id,processor_digest,'SUCCEEDED'
FROM workforce_task_turns WHERE id=$3`, turnID, ordinal, fixture.turnID); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(ctx, `INSERT INTO workforce_review_runs (
id,tenant_id,task_id,task_turn_id,attempt_id,finalization_run_id,result_commit,result_tree,
hard_timeout_seconds,transient_retry_limit,automatic_revision_limit,identity_digest,completion_check_digest,status,created_at)
SELECT $1,tenant_id,task_id,$2,attempt_id,$3,result_commit,result_tree,
hard_timeout_seconds,transient_retry_limit,automatic_revision_limit,$4,completion_check_digest,'PENDING',created_at+$5::BIGINT*INTERVAL '1 second'
FROM workforce_review_runs WHERE id=$6`, runID, turnID, ids.NextID(), fmt.Sprintf("%064x", ordinal), ordinal, firstRunID); err != nil {
			t.Fatal(err)
		}
		allIDs = append(allIDs, runID)
	}

	dsn := strings.TrimSpace(os.Getenv(reviewRunTestDSNEnv))
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	counter := &reviewRunQueryCounter{}
	config.ConnConfig.Tracer = counter
	countedDB, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer countedDB.Close()
	countedStore := New(countedDB, ids)
	counter.count.Store(0)
	page1, err := countedStore.ListRunsForTask(ctx, fixture.tenantID, fixture.taskID, 0, 3)
	if err != nil {
		t.Fatal(err)
	}
	if got := counter.count.Load(); got != 2 {
		t.Fatalf("ReviewRun history database queries=%d, want exactly 2 for a bounded page", got)
	}
	if len(page1.Items) != 3 || page1.NextCursor == 0 {
		t.Fatalf("first ReviewRun page=%+v", page1)
	}
	page2, err := countedStore.ListRunsForTask(ctx, fixture.tenantID, fixture.taskID, page1.NextCursor, 3)
	if err != nil {
		t.Fatal(err)
	}
	if len(page2.Items) != 3 || page2.NextCursor != 0 {
		t.Fatalf("second ReviewRun page=%+v", page2)
	}
	seen := map[int64]bool{}
	for _, page := range []domain.RunPage{page1, page2} {
		for _, item := range page.Items {
			if seen[item.ID] {
				t.Fatalf("ReviewRun %d appeared on both cursor pages", item.ID)
			}
			seen[item.ID] = true
		}
	}
	if len(seen) != len(allIDs) {
		t.Fatalf("cursor history IDs=%d want=%d", len(seen), len(allIDs))
	}

	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err = tx.Exec(ctx, `SET LOCAL enable_seqscan=off`); err != nil {
		t.Fatal(err)
	}
	rows, err := tx.Query(ctx, `EXPLAIN (COSTS OFF) SELECT id FROM workforce_review_runs
WHERE tenant_id=$1 AND task_id=$2 AND id<$3 ORDER BY id DESC LIMIT 4`, fixture.tenantID, fixture.taskID, page1.NextCursor)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var plan strings.Builder
	for rows.Next() {
		var line string
		if err = rows.Scan(&line); err != nil {
			t.Fatal(err)
		}
		plan.WriteString(line)
	}
	if err = rows.Err(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plan.String(), "workforce_review_runs_task_read_idx") {
		t.Fatalf("cursor query did not use task-scoped read index:\n%s", plan.String())
	}
}

func TestReviewRunFinalizerGateDefersOnlyReviewEnabledTasks(t *testing.T) {
	ctx, db, ids := openReviewRunTestDB(t)
	reviewStore := newReviewRunTestStore(db, ids)
	artifactStore := artifactinfra.New(db, ids)
	artifactStore.SetReviewRunGate(reviewStore)

	enabled := installReviewRunFixture(t, ctx, db, ids, true)
	t.Cleanup(func() { enabled.cleanup(ctx, db) })
	uploadID, envelope, lease := enabled.installUpload(t, ctx, db, ids)
	if err := artifactStore.VerifyUpload(ctx, envelope, uploadID, lease); err != nil {
		t.Fatalf("verify enabled Task artifact upload: %v", err)
	}
	assertReviewRunStatus(t, ctx, db, enabled.taskID, domain.StatusPending)
	assertFixtureStatuses(t, ctx, db, enabled, "REVIEWING", "REVIEWING", "REVIEWING", "REVIEWING", "PENDING_REVIEW")
	assertCount(t, ctx, db, `SELECT count(*) FROM workforce_task_events WHERE task_id=$1 AND event_type='task.succeeded'`, enabled.taskID, 0)

	// A Finalizer retry after the transaction committed must reuse the same
	// ReviewRun and must not increment Task version a second time.
	var firstRunID, firstVersion int64
	if err := db.QueryRow(ctx, `SELECT id FROM workforce_review_runs WHERE task_id=$1`, enabled.taskID).Scan(&firstRunID); err != nil {
		t.Fatal(err)
	}
	if err := db.QueryRow(ctx, `SELECT version FROM workforce_tasks WHERE id=$1`, enabled.taskID).Scan(&firstVersion); err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	decision, err := reviewStore.GateVerifiedFinalization(ctx, tx, enabled.finalizationInput())
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatalf("replay independent review finalizer gate: %v", err)
	}
	if !decision.ReviewRequired || decision.ReviewRunID != firstRunID || decision.Status != domain.StatusPending {
		_ = tx.Rollback(ctx)
		t.Fatalf("unexpected replay decision: %+v", decision)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	var replayVersion int64
	if err = db.QueryRow(ctx, `SELECT version FROM workforce_tasks WHERE id=$1`, enabled.taskID).Scan(&replayVersion); err != nil {
		t.Fatal(err)
	}
	if replayVersion != firstVersion {
		t.Fatalf("finalizer replay changed Task version: got %d want %d", replayVersion, firstVersion)
	}

	tx, err = db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	conflicting := enabled.finalizationInput()
	conflicting.ResultTree = strings.Repeat("f", 40)
	if _, err = reviewStore.GateVerifiedFinalization(ctx, tx, conflicting); !errors.Is(err, domain.ErrIdentityConflict) {
		_ = tx.Rollback(ctx)
		t.Fatalf("different Finalizer identity error=%v, want identity conflict", err)
	}
	_ = tx.Rollback(ctx)

	disabled := installReviewRunFixture(t, ctx, db, ids, false)
	t.Cleanup(func() { disabled.cleanup(ctx, db) })
	disabledUploadID, disabledEnvelope, disabledLease := disabled.installUpload(t, ctx, db, ids)
	if err := artifactStore.VerifyUpload(ctx, disabledEnvelope, disabledUploadID, disabledLease); err != nil {
		t.Fatalf("verify legacy/disabled Task artifact upload: %v", err)
	}
	assertCount(t, ctx, db, `SELECT count(*) FROM workforce_review_runs WHERE task_id=$1`, disabled.taskID, 0)
	assertFixtureStatuses(t, ctx, db, disabled, "SUCCEEDED", "SUCCEEDED", "SUCCEEDED", "SUCCEEDED", "AVAILABLE")
	assertCount(t, ctx, db, `SELECT count(*) FROM workforce_task_events WHERE task_id=$1 AND event_type='task.succeeded'`, disabled.taskID, 1)
}

func TestReviewRunConcurrentFinalizerReplayConvergesToOneRun(t *testing.T) {
	ctx, db, ids := openReviewRunTestDB(t)
	store := newReviewRunTestStore(db, ids)
	artifactStore := artifactinfra.New(db, ids)
	artifactStore.SetReviewRunGate(store)
	fixture := installReviewRunFixture(t, ctx, db, ids, true)
	t.Cleanup(func() { fixture.cleanup(ctx, db) })

	type result struct {
		decision reviewfacade.FinalizationDecision
		err      error
	}
	ready := make(chan struct{}, 2)
	start := make(chan struct{})
	results := make(chan result, 2)
	var workers sync.WaitGroup
	for range 2 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			tx, err := db.Begin(ctx)
			if err != nil {
				results <- result{err: err}
				return
			}
			defer tx.Rollback(ctx)
			ready <- struct{}{}
			<-start
			decision, err := store.GateVerifiedFinalization(ctx, tx, fixture.finalizationInput())
			if err == nil {
				err = tx.Commit(ctx)
			}
			results <- result{decision: decision, err: err}
		}()
	}
	<-ready
	<-ready
	close(start)
	workers.Wait()
	close(results)
	var runID int64
	for item := range results {
		if item.err != nil || !item.decision.ReviewRequired || item.decision.ReviewRunID == 0 {
			t.Fatalf("concurrent finalizer result=%+v", item)
		}
		if runID == 0 {
			runID = item.decision.ReviewRunID
		} else if item.decision.ReviewRunID != runID {
			t.Fatalf("duplicate Finalizer created distinct ReviewRuns: %d and %d", runID, item.decision.ReviewRunID)
		}
	}
	assertCount(t, ctx, db, `SELECT count(*) FROM workforce_review_runs WHERE task_id=$1`, fixture.taskID, 1)
	assertReviewRunStatusByID(t, ctx, db, runID, domain.StatusPending)
}

func TestReviewRunWorkerFencingRetryBudgetAndCancellation(t *testing.T) {
	ctx, db, ids := openReviewRunTestDB(t)
	recorder := &reviewRunNotificationRecorder{}
	store := newReviewRunTestStore(db, ids)
	store.SetTaskNotificationRecorder(recorder)
	fixture := installReviewRunFixture(t, ctx, db, ids, true)
	t.Cleanup(func() { fixture.cleanup(ctx, db) })
	runID := createReviewRun(t, ctx, db, store, fixture)
	worker := installReviewWorker(t, ctx, db, ids, fixture.reviewerImage, 1, 2, true)
	t.Cleanup(func() { worker.cleanup(ctx, db) })

	for attempt := 1; attempt <= 3; attempt++ {
		if attempt > 1 {
			if _, err := db.Exec(ctx, `UPDATE workforce_review_runs SET next_retry_at=CURRENT_TIMESTAMP-INTERVAL '1 second' WHERE id=$1`, runID); err != nil {
				t.Fatal(err)
			}
		}
		assigned, err := store.AssignAvailable(ctx, 1)
		if err != nil || assigned != 1 {
			t.Fatalf("assign review attempt %d: assigned=%d err=%v", attempt, assigned, err)
		}
		var epoch int64
		if err = db.QueryRow(ctx, `SELECT owner_epoch FROM workforce_review_runs WHERE id=$1`, runID).Scan(&epoch); err != nil {
			t.Fatal(err)
		}
		if _, err = store.Claim(ctx, worker.envelope(fmt.Sprintf("review-stale-%d", attempt), 0), runID); !errors.Is(err, domain.ErrFenceRejected) {
			t.Fatalf("claim without assignment owner epoch error=%v, want fenced", err)
		}
		envelope := worker.envelope(fmt.Sprintf("review-fence-%d", attempt), epoch)
		claim, err := store.Claim(ctx, envelope, runID)
		if err != nil {
			t.Fatalf("claim review attempt %d: %v", attempt, err)
		}
		if claim.LeaseToken == "" || claim.OwnerEpoch != epoch || claim.ReviewerImage != fixture.reviewerImage {
			t.Fatalf("unsafe or incomplete claim: %+v", claim)
		}
		if _, err = store.Claim(ctx, envelope, runID); !errors.Is(err, domain.ErrNotFound) {
			t.Fatalf("double claim error=%v, want no claimable occurrence", err)
		}
		permit, err := store.AcquireLaunchPermit(ctx, envelope, runID, claim.LeaseToken)
		if err != nil || permit.PermitToken == "" || permit.OwnerEpoch != epoch {
			t.Fatalf("acquire launch permit: permit=%+v err=%v", permit, err)
		}
		var leaseHash, permitHash string
		if err = db.QueryRow(ctx, `SELECT lease_token_sha256,launch_permit_sha256 FROM workforce_review_runs WHERE id=$1`, runID).Scan(&leaseHash, &permitHash); err != nil {
			t.Fatal(err)
		}
		if leaseHash == claim.LeaseToken || permitHash == permit.PermitToken || len(leaseHash) != 64 || len(permitHash) != 64 {
			t.Fatalf("raw ReviewRun capability token was persisted: lease=%q permit=%q", leaseHash, permitHash)
		}
		container := reviewfacade.ContainerRegistration{
			ContainerID:             fmt.Sprintf("%064x", attempt),
			ContainerName:           fmt.Sprintf("review-retry-%d", attempt),
			ImageDigest:             fixture.reviewerImage,
			ExecutionIdentityDigest: fmt.Sprintf("%064x", attempt+100),
		}
		if err = store.RecordContainer(ctx, envelope, runID, container, claim.LeaseToken, permit.PermitID, permit.PermitToken); err != nil {
			t.Fatalf("record review container %d: %v", attempt, err)
		}
		if err = store.RecordTransientFailure(ctx, envelope, runID, claim.LeaseToken, "REVIEWER_TRANSIENT_FAILURE"); err != nil {
			t.Fatalf("record review failure %d: %v", attempt, err)
		}
		if attempt < 3 {
			assertReviewRunStatusByID(t, ctx, db, runID, domain.StatusRetryWait)
			// A retryable Run must not carry any old Attempt control identity.
			// In particular, its hard timeout must be replaced on Claim and a
			// different fenced owner must be able to register a new container.
			var retryDeadline *time.Time
			var runtimeID, runtimeName, runtimeIdentity string
			if err = db.QueryRow(ctx, `SELECT execution_deadline_at,runtime_container_id,runtime_container_name,runtime_execution_identity_digest FROM workforce_review_runs WHERE id=$1`, runID).Scan(&retryDeadline, &runtimeID, &runtimeName, &runtimeIdentity); err != nil {
				t.Fatal(err)
			}
			if retryDeadline != nil || runtimeID != "" || runtimeName != "" || runtimeIdentity != "" {
				t.Fatalf("retry %d retained prior attempt control identity: deadline=%v container=(%q,%q,%q)", attempt, retryDeadline, runtimeID, runtimeName, runtimeIdentity)
			}
			continue
		}
		assertReviewRunStatusByID(t, ctx, db, runID, domain.StatusManualReview)
		if _, err = store.RenewLease(ctx, envelope, runID, claim.LeaseToken); !errors.Is(err, domain.ErrFenceRejected) {
			t.Fatalf("late owner renewed exhausted run: %v", err)
		}
	}
	assertCount(t, ctx, db, `SELECT count(*) FROM workforce_task_turns WHERE task_id=$1`, fixture.taskID, 1)
	assertCount(t, ctx, db, `SELECT count(*) FROM workforce_review_run_attempts WHERE review_run_id=$1`, runID, 3)
	assertCount(t, ctx, db, `SELECT count(*) FROM workforce_task_events WHERE task_id=$1 AND event_type='independent_review.attention_required'`, fixture.taskID, 1)
	if len(recorder.transitions) != 1 || recorder.transitions[0].SourceType != tasknotification.SourceIndependentReviewRun || recorder.transitions[0].ReasonCode != "INDEPENDENT_REVIEW_RETRY_EXHAUSTED" {
		t.Fatalf("retry exhaustion did not emit exactly one safe action fact: %+v", recorder.transitions)
	}

	// Cancellation must invalidate the active lease/permit and supersede the
	// queued control message, so a stale owner cannot revive the ReviewRun.
	cancelled := installReviewRunFixture(t, ctx, db, ids, true)
	t.Cleanup(func() { cancelled.cleanup(ctx, db) })
	cancelRunID := createReviewRun(t, ctx, db, store, cancelled)
	if assigned, err := store.AssignAvailable(ctx, 1); err != nil || assigned != 1 {
		t.Fatalf("assign cancellable ReviewRun: assigned=%d err=%v", assigned, err)
	}
	var cancelEpoch int64
	if err := db.QueryRow(ctx, `SELECT owner_epoch FROM workforce_review_runs WHERE id=$1`, cancelRunID).Scan(&cancelEpoch); err != nil {
		t.Fatal(err)
	}
	cancelEnvelope := worker.envelope("review-cancel", cancelEpoch)
	cancelClaim, err := store.Claim(ctx, cancelEnvelope, cancelRunID)
	if err != nil {
		t.Fatal(err)
	}
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.CancelForTask(ctx, tx, cancelled.taskID, "TASK_CANCELLED"); err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	assertReviewRunStatusByID(t, ctx, db, cancelRunID, domain.StatusCancelled)
	assertCount(t, ctx, db, `SELECT count(*) FROM workforce_outbox_events WHERE aggregate_type='review_run' AND aggregate_id=$1 AND status='SUPERSEDED'`, cancelRunID, 1)
	if _, err = store.RenewLease(ctx, cancelEnvelope, cancelRunID, cancelClaim.LeaseToken); !errors.Is(err, domain.ErrFenceRejected) {
		t.Fatalf("cancelled ReviewRun accepted stale lease: %v", err)
	}
}

func TestReviewRunHardDeadlineReconcilesWithoutWaitingForLeaseExpiry(t *testing.T) {
	ctx, db, ids := openReviewRunTestDB(t)
	store := newReviewRunTestStore(db, ids)
	fixture := installReviewRunFixture(t, ctx, db, ids, true)
	t.Cleanup(func() { fixture.cleanup(ctx, db) })
	runID := createReviewRun(t, ctx, db, store, fixture)
	worker := installReviewWorker(t, ctx, db, ids, fixture.reviewerImage, 1, 2, true)
	t.Cleanup(func() { worker.cleanup(ctx, db) })

	if assigned, err := store.AssignAvailable(ctx, 1); err != nil || assigned != 1 {
		t.Fatalf("assign review run: assigned=%d err=%v", assigned, err)
	}
	var epoch int64
	if err := db.QueryRow(ctx, `SELECT owner_epoch FROM workforce_review_runs WHERE id=$1`, runID).Scan(&epoch); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Claim(ctx, worker.envelope("review-hard-deadline", epoch), runID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `UPDATE workforce_review_runs
SET execution_deadline_at=CURRENT_TIMESTAMP-INTERVAL '1 second',lease_expires_at=CURRENT_TIMESTAMP+INTERVAL '1 hour'
WHERE id=$1`, runID); err != nil {
		t.Fatal(err)
	}
	if reconciled, err := store.ReconcileExpired(ctx, 1); err != nil || reconciled != 1 {
		t.Fatalf("reconcile hard deadline: reconciled=%d err=%v", reconciled, err)
	}
	assertReviewRunStatusByID(t, ctx, db, runID, domain.StatusRetryWait)
	assertString(t, ctx, db, `SELECT error_code FROM workforce_review_runs WHERE id=$1`, runID, "REVIEW_LEASE_EXPIRED")
}

func TestReviewRunCanonicalVerdictsPersistOnlySafeEvidence(t *testing.T) {
	ctx, db, ids := openReviewRunTestDB(t)
	store := newReviewRunTestStore(db, ids)
	artifactStore := artifactinfra.New(db, ids)
	artifactStore.SetReviewRunGate(store)
	for _, test := range []struct {
		name          string
		report        string
		wantRun       string
		wantTask      string
		wantArtifact  string
		wantSubmitErr error
		wantEvent     string
		wantCanonical bool
	}{
		{name: "pass unlocks finalization", report: `{"schemaVersion":"review-report/v1","verdict":"PASS","score":100,"summary":"result is safe","reviewTests":[{"command":"git status","exitCode":0,"status":"PASSED","summary":"working tree is clean"}]}`, wantRun: domain.StatusPassed, wantTask: "SUCCEEDED", wantArtifact: "AVAILABLE", wantEvent: "task.succeeded", wantCanonical: true},
		{name: "uncomposed revise fails closed", report: `{"schemaVersion":"review-report/v1","verdict":"REVISE","score":60,"summary":"revision is required"}`, wantRun: domain.StatusManualReview, wantTask: "MANUAL_REVIEW", wantArtifact: "PENDING_REVIEW", wantEvent: "independent_review.attention_required", wantCanonical: true},
		{name: "uncomposed frozen evidence fails closed", report: `{"schemaVersion":"review-report/v1","verdict":"REVISE","summary":"revision is required","findings":[{"id":"f1","severity":"WARNING","category":"CORRECTNESS","title":"review","message":"line is frozen evidence","evidence":{"path":"README.md","line":10}}]}`, wantRun: domain.StatusManualReview, wantTask: "MANUAL_REVIEW", wantArtifact: "PENDING_REVIEW", wantEvent: "independent_review.attention_required", wantCanonical: true},
		{name: "reject requires manual action", report: `{"schemaVersion":"review-report/v1","verdict":"REJECT","score":0,"summary":"result is unsafe"}`, wantRun: domain.StatusRejected, wantTask: "MANUAL_REVIEW", wantArtifact: "PENDING_REVIEW", wantEvent: "independent_review.attention_required", wantCanonical: true},
		{name: "contradictory pass fails closed", report: `{"schemaVersion":"review-report/v1","verdict":"PASS","summary":"not safe","findings":[{"id":"f1","severity":"BLOCKING","category":"SECURITY","title":"blocking","message":"blocking","evidence":{"path":"README.md","line":1}}]}`, wantRun: domain.StatusManualReview, wantTask: "MANUAL_REVIEW", wantArtifact: "PENDING_REVIEW", wantSubmitErr: domain.ErrInvalidReport, wantEvent: "independent_review.attention_required", wantCanonical: false},
		{name: "unknown repository evidence fails closed", report: `{"schemaVersion":"review-report/v1","verdict":"REVISE","summary":"evidence is not in frozen result","findings":[{"id":"f1","severity":"WARNING","category":"CORRECTNESS","title":"missing","message":"missing","evidence":{"path":"missing.go","line":1}}]}`, wantRun: domain.StatusManualReview, wantTask: "MANUAL_REVIEW", wantArtifact: "PENDING_REVIEW", wantSubmitErr: domain.ErrInvalidReport, wantEvent: "independent_review.attention_required", wantCanonical: false},
		{name: "out of range repository line fails closed", report: `{"schemaVersion":"review-report/v1","verdict":"REVISE","summary":"line is not in frozen result","findings":[{"id":"f1","severity":"WARNING","category":"CORRECTNESS","title":"line","message":"line","evidence":{"path":"README.md","line":11}}]}`, wantRun: domain.StatusManualReview, wantTask: "MANUAL_REVIEW", wantArtifact: "PENDING_REVIEW", wantSubmitErr: domain.ErrInvalidReport, wantEvent: "independent_review.attention_required", wantCanonical: false},
		{name: "secret candidate fails closed", report: `{"schemaVersion":"review-report/v1","verdict":"REVISE","summary":"Authorization: Bearer top-secret"}`, wantRun: domain.StatusManualReview, wantTask: "MANUAL_REVIEW", wantArtifact: "PENDING_REVIEW", wantSubmitErr: domain.ErrInvalidReport, wantEvent: "independent_review.attention_required", wantCanonical: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			fixture := installReviewRunFixture(t, ctx, db, ids, true)
			t.Cleanup(func() { fixture.cleanup(ctx, db) })
			uploadID, uploadEnvelope, uploadLease := fixture.installUpload(t, ctx, db, ids)
			if err := artifactStore.VerifyUpload(ctx, uploadEnvelope, uploadID, uploadLease); err != nil {
				t.Fatal(err)
			}
			var runID int64
			if err := db.QueryRow(ctx, `SELECT id FROM workforce_review_runs WHERE task_id=$1`, fixture.taskID).Scan(&runID); err != nil {
				t.Fatal(err)
			}
			worker := installReviewWorker(t, ctx, db, ids, fixture.reviewerImage, 1, 2, true)
			t.Cleanup(func() { worker.cleanup(ctx, db) })
			if assigned, err := store.AssignAvailable(ctx, 1); err != nil || assigned != 1 {
				t.Fatalf("assign run: assigned=%d err=%v", assigned, err)
			}
			var epoch int64
			if err := db.QueryRow(ctx, `SELECT owner_epoch FROM workforce_review_runs WHERE id=$1`, runID).Scan(&epoch); err != nil {
				t.Fatal(err)
			}
			envelope := worker.envelope("review-report-"+test.name, epoch)
			claim, err := store.Claim(ctx, envelope, runID)
			if err != nil {
				t.Fatal(err)
			}
			permit, err := store.AcquireLaunchPermit(ctx, envelope, runID, claim.LeaseToken)
			if err != nil {
				t.Fatal(err)
			}
			container := reviewfacade.ContainerRegistration{ContainerID: strings.Repeat("c", 64), ContainerName: fmt.Sprintf("review-report-%d", runID), ImageDigest: fixture.reviewerImage, ExecutionIdentityDigest: strings.Repeat("d", 64)}
			if err = store.RecordContainer(ctx, envelope, runID, container, claim.LeaseToken, permit.PermitID, permit.PermitToken); err != nil {
				t.Fatal(err)
			}
			_, submitErr := store.SubmitReport(ctx, envelope, runID, reviewfacade.ReportSubmission{LeaseToken: claim.LeaseToken, PermitID: permit.PermitID, PermitToken: permit.PermitToken, Report: []byte(test.report), Usage: domain.UsageFacts{Requests: 1, InputTokens: 7, OutputTokens: 3, DurationMilli: 12}})
			if !errors.Is(submitErr, test.wantSubmitErr) {
				t.Fatalf("SubmitReport error=%v want %v", submitErr, test.wantSubmitErr)
			}
			assertReviewRunStatusByID(t, ctx, db, runID, test.wantRun)
			assertString(t, ctx, db, `SELECT status FROM workforce_tasks WHERE id=$1`, fixture.taskID, test.wantTask)
			assertString(t, ctx, db, `SELECT status FROM workforce_artifacts WHERE task_id=$1`, fixture.taskID, test.wantArtifact)
			var events int
			if err = db.QueryRow(ctx, `SELECT count(*) FROM workforce_task_events WHERE task_id=$1 AND event_type=$2`, fixture.taskID, test.wantEvent).Scan(&events); err != nil {
				t.Fatal(err)
			}
			if test.wantEvent == "" && events != 0 {
				t.Fatalf("unexpected task event count=%d", events)
			}
			if test.wantEvent != "" && events != 1 {
				t.Fatalf("expected exactly one %s event, got %d", test.wantEvent, events)
			}
			var canonical, digest string
			if err = db.QueryRow(ctx, `SELECT COALESCE(canonical_report_json::text,''),COALESCE(canonical_report_digest,'') FROM workforce_review_runs WHERE id=$1`, runID).Scan(&canonical, &digest); err != nil {
				t.Fatal(err)
			}
			if test.wantCanonical != (canonical != "" && digest != "") {
				t.Fatalf("canonical persistence=(%q,%q), want=%t", canonical, digest, test.wantCanonical)
			}
			if strings.Contains(canonical, "top-secret") || strings.Contains(canonical, "hunter2") {
				t.Fatalf("canonical report retained secret text: %s", canonical)
			}
			if test.wantCanonical {
				view, err := store.ReportForTask(ctx, fixture.tenantID, fixture.taskID)
				if err != nil || !view.Required || view.Status != test.wantRun || view.ReportDigest == "" {
					t.Fatalf("safe task report=%+v err=%v", view, err)
				}
				encoded, _ := json.Marshal(view)
				for _, forbidden := range []string{"worker", "lease", "permit", "container", "provider", "top-secret", "hunter2"} {
					if strings.Contains(strings.ToLower(string(encoded)), forbidden) {
						t.Fatalf("task report leaked %q: %s", forbidden, encoded)
					}
				}
			}
		})
	}
}

func TestReviewRunRecoveryFencesOldOwnerAndAcceptsExactRegisteredContainer(t *testing.T) {
	ctx, db, ids := openReviewRunTestDB(t)
	store := newReviewRunTestStore(db, ids)
	artifactStore := artifactinfra.New(db, ids)
	artifactStore.SetReviewRunGate(store)
	fixture := installReviewRunFixture(t, ctx, db, ids, true)
	t.Cleanup(func() { fixture.cleanup(ctx, db) })
	uploadID, uploadEnvelope, uploadLease := fixture.installUpload(t, ctx, db, ids)
	if err := artifactStore.VerifyUpload(ctx, uploadEnvelope, uploadID, uploadLease); err != nil {
		t.Fatal(err)
	}
	var runID int64
	if err := db.QueryRow(ctx, `SELECT id FROM workforce_review_runs WHERE task_id=$1`, fixture.taskID).Scan(&runID); err != nil {
		t.Fatal(err)
	}
	worker := installReviewWorker(t, ctx, db, ids, fixture.reviewerImage, 1, 2, true)
	t.Cleanup(func() { worker.cleanup(ctx, db) })
	if assigned, err := store.AssignAvailable(ctx, 1); err != nil || assigned != 1 {
		t.Fatalf("assign: assigned=%d err=%v", assigned, err)
	}
	var epoch int64
	if err := db.QueryRow(ctx, `SELECT owner_epoch FROM workforce_review_runs WHERE id=$1`, runID).Scan(&epoch); err != nil {
		t.Fatal(err)
	}
	oldOwner := worker.envelope("recover-old", epoch)
	claim, err := store.Claim(ctx, oldOwner, runID)
	if err != nil {
		t.Fatal(err)
	}
	permit, err := store.AcquireLaunchPermit(ctx, oldOwner, runID, claim.LeaseToken)
	if err != nil {
		t.Fatal(err)
	}
	container := reviewfacade.ContainerRegistration{ContainerID: strings.Repeat("e", 64), ContainerName: fmt.Sprintf("recover-review-%d", runID), ImageDigest: fixture.reviewerImage, ExecutionIdentityDigest: strings.Repeat("f", 64)}
	if err = store.RecordContainer(ctx, oldOwner, runID, container, claim.LeaseToken, permit.PermitID, permit.PermitToken); err != nil {
		t.Fatal(err)
	}

	newInstance, newBoot := fmt.Sprintf("recovered-instance-%d", ids.NextID()), fmt.Sprintf("recovered-boot-%d", ids.NextID())
	if _, err = db.Exec(ctx, `UPDATE workforce_workers SET current_instance_id=$2 WHERE id=$1`, worker.id, newInstance); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `INSERT INTO workforce_worker_sessions (instance_id,worker_id,host_boot_id,started_at,last_heartbeat_at,consumer_state,docker_ready,capabilities_json) VALUES ($1,$2,$3,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,'CONSUMING',TRUE,'{"independentReviewConsumer":"v1"}'::jsonb)`, newInstance, worker.id, newBoot); err != nil {
		t.Fatal(err)
	}
	newOwner := reviewfacade.MachineEnvelope{SchemaVersion: 1, WorkerID: worker.id, InstanceID: newInstance, HostBootID: newBoot, RequestID: "recover-new", Generation: 1, OwnerEpoch: epoch}
	if _, err = store.Recover(ctx, newOwner, runID, reviewfacade.ContainerRegistration{ContainerID: container.ContainerID, ContainerName: container.ContainerName, ImageDigest: container.ImageDigest, ExecutionIdentityDigest: strings.Repeat("0", 64)}); !errors.Is(err, domain.ErrFenceRejected) {
		t.Fatalf("mismatched local container recovery error=%v, want fence", err)
	}
	recovered, err := store.Recover(ctx, newOwner, runID, container)
	if err != nil || recovered.LeaseToken == "" || recovered.OwnerEpoch != epoch {
		t.Fatalf("recover exact container: claim=%+v err=%v", recovered, err)
	}
	if _, err = store.RenewLease(ctx, oldOwner, runID, claim.LeaseToken); !errors.Is(err, domain.ErrFenceRejected) {
		t.Fatalf("old owner renewed after recovery: %v", err)
	}
	if _, err = store.AcquireLaunchPermit(ctx, newOwner, runID, recovered.LeaseToken); err != nil {
		t.Fatalf("recovered owner cannot acquire fresh permit: %v", err)
	}
	if _, err = db.Exec(ctx, `UPDATE workforce_review_runs SET status='CANCELLED',lease_expires_at=NULL WHERE id=$1`, runID); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Recover(ctx, newOwner, runID, container); !errors.Is(err, domain.ErrFenceRejected) {
		t.Fatalf("cancelled run was recovered: %v", err)
	}
}

func TestReviewRunAssignmentRequiresExactImageAndCapacity(t *testing.T) {
	ctx, db, ids := openReviewRunTestDB(t)
	store := newReviewRunTestStore(db, ids)
	first := installReviewRunFixture(t, ctx, db, ids, true)
	second := installReviewRunFixture(t, ctx, db, ids, true)
	t.Cleanup(func() { first.cleanup(ctx, db) })
	t.Cleanup(func() { second.cleanup(ctx, db) })
	firstRunID := createReviewRun(t, ctx, db, store, first)
	secondRunID := createReviewRun(t, ctx, db, store, second)
	if _, err := db.Exec(ctx, `UPDATE workforce_review_runs SET created_at=CASE id WHEN $1 THEN TIMESTAMPTZ '2000-01-01' ELSE TIMESTAMPTZ '2000-01-02' END WHERE id IN ($1,$2)`, firstRunID, secondRunID); err != nil {
		t.Fatal(err)
	}
	worker := installReviewWorker(t, ctx, db, ids, first.reviewerImage, 1, 2, false)
	t.Cleanup(func() { worker.cleanup(ctx, db) })

	if assigned, err := store.AssignAvailable(ctx, 2); err != nil || assigned != 0 {
		t.Fatalf("worker lacking Review capabilities was selected: assigned=%d err=%v", assigned, err)
	}
	assertReviewRunStatusByID(t, ctx, db, firstRunID, domain.StatusPending)
	if _, err := db.Exec(ctx, `UPDATE workforce_worker_images SET
capabilities_json='["harness.codex.review","review.structured-output"]'::jsonb,
manifest_json=jsonb_set(jsonb_set(manifest_json,'{adapter,capabilities}','["harness.codex.review","review.structured-output"]'::jsonb,true),
'{probe,capabilities}','[{"name":"harness.codex.review","status":"supported"},{"name":"review.structured-output","status":"supported"}]'::jsonb,true)
WHERE worker_id=$1 AND role='attempt'`, worker.id); err != nil {
		t.Fatal(err)
	}
	if assigned, err := store.AssignAvailable(ctx, 2); err != nil || assigned != 1 {
		t.Fatalf("exact admitted Review image was not selected: assigned=%d err=%v", assigned, err)
	}
	var assignedRunID, pendingRunID, activeTaskID int64
	if err := db.QueryRow(ctx, `SELECT id,task_id FROM workforce_review_runs WHERE id IN ($1,$2) AND status='ASSIGNED'`, firstRunID, secondRunID).Scan(&assignedRunID, &activeTaskID); err != nil {
		t.Fatal(err)
	}
	if assignedRunID == firstRunID {
		pendingRunID = secondRunID
	} else {
		pendingRunID = firstRunID
	}
	assertReviewRunStatusByID(t, ctx, db, pendingRunID, domain.StatusPending)

	// Raising the review-slot limit alone must not bypass the physical total
	// container guard: the active Provider Review owns Reviewer+relay (2), so
	// a second Provider Review cannot fit in a total capacity of 3.
	if _, err := db.Exec(ctx, `UPDATE workforce_workers SET max_active_reviews=2,max_total_containers=3 WHERE id=$1`, worker.id); err != nil {
		t.Fatal(err)
	}
	if assigned, err := store.AssignAvailable(ctx, 1); err != nil || assigned != 0 {
		t.Fatalf("total container guard was bypassed: assigned=%d err=%v", assigned, err)
	}
	assertReviewRunStatusByID(t, ctx, db, pendingRunID, domain.StatusPending)
	decision, err := store.ReviewRunRetention(ctx, activeTaskID)
	if err != nil || !decision.ActiveReferences || decision.ReasonCode != "ACTIVE_REVIEW_RUN" {
		t.Fatalf("active ReviewRun did not block workspace retention: decision=%+v err=%v", decision, err)
	}
	// The Worker calls Task's GC facade rather than ReviewRun storage directly.
	// Make the pre-Slice-1 references portable/terminal, then prove the added
	// ReviewRun authority is the precise reason it still refuses collection.
	for _, statement := range []string{
		`UPDATE workforce_tasks SET status='SUCCEEDED',finished_at=CURRENT_TIMESTAMP WHERE id=$1`,
		`UPDATE workforce_task_turns SET status='SUCCEEDED',finished_at=CURRENT_TIMESTAMP WHERE task_id=$1`,
		`UPDATE workforce_task_attempts SET status='SUCCEEDED',finished_at=CURRENT_TIMESTAMP WHERE task_id=$1`,
		`UPDATE workforce_execution_nodes SET status='SUCCEEDED' WHERE task_id=$1`,
	} {
		if _, err := db.Exec(ctx, statement, activeTaskID); err != nil {
			t.Fatal(err)
		}
	}
	taskStore := taskinfra.NewStore(db, ids)
	taskStore.SetReviewRunWorkspaceRetention(store)
	eligibility, err := taskStore.WorkspaceGCEligibility(ctx, activeTaskID)
	if err != nil || !eligibility.ActiveReferences || eligibility.ReasonCode != "ACTIVE_REVIEW_RUN" {
		t.Fatalf("Task GC facade bypassed active ReviewRun retention: eligibility=%+v err=%v", eligibility, err)
	}
}

func TestReviewRunAssignmentAcceptsStructuredWorkerProbeCapabilities(t *testing.T) {
	ctx, db, ids := openReviewRunTestDB(t)
	store := newReviewRunTestStore(db, ids)
	fixture := installReviewRunFixture(t, ctx, db, ids, true)
	t.Cleanup(func() { fixture.cleanup(ctx, db) })
	runID := createReviewRun(t, ctx, db, store, fixture)
	worker := installReviewWorker(t, ctx, db, ids, fixture.reviewerImage, 1, 2, true)
	t.Cleanup(func() { worker.cleanup(ctx, db) })

	// A live seven-worker image inventory persists capability probe records,
	// rather than the compact string array used by older test fixtures.  Only
	// explicit SUPPORTED records may satisfy an immutable ReviewRun requirement.
	if _, err := db.Exec(ctx, `UPDATE workforce_worker_images
SET capabilities_json='{"capabilities":[{"name":"harness.codex.review","status":"SUPPORTED"},{"name":"review.structured-output","status":"SUPPORTED"}]}'::jsonb
WHERE worker_id=$1 AND image_digest=$2`, worker.id, fixture.reviewerImage); err != nil {
		t.Fatal(err)
	}
	assigned, err := store.AssignAvailable(ctx, 1)
	if err != nil || assigned != 1 {
		t.Fatalf("structured admitted image evidence was not eligible: assigned=%d err=%v", assigned, err)
	}
	assertReviewRunStatusByID(t, ctx, db, runID, domain.StatusAssigned)
}

func TestReviewRunCanonicalReportVerdictsAndFencing(t *testing.T) {
	ctx, db, ids := openReviewRunTestDB(t)
	recorder := &reviewRunNotificationRecorder{}
	store := newReviewRunTestStore(db, ids)
	store.SetTaskNotificationRecorder(recorder)
	artifactStore := artifactinfra.New(db, ids)
	artifactStore.SetReviewRunGate(store)
	fixture := installReviewRunFixture(t, ctx, db, ids, true)
	t.Cleanup(func() { fixture.cleanup(ctx, db) })
	uploadID, uploadEnvelope, uploadLease := fixture.installUpload(t, ctx, db, ids)
	if err := artifactStore.VerifyUpload(ctx, uploadEnvelope, uploadID, uploadLease); err != nil {
		t.Fatal(err)
	}
	var runID int64
	if err := db.QueryRow(ctx, `SELECT id FROM workforce_review_runs WHERE task_id=$1`, fixture.taskID).Scan(&runID); err != nil {
		t.Fatal(err)
	}
	worker := installReviewWorker(t, ctx, db, ids, fixture.reviewerImage, 1, 2, true)
	t.Cleanup(func() { worker.cleanup(ctx, db) })
	if assigned, err := store.AssignAvailable(ctx, 1); err != nil || assigned != 1 {
		t.Fatalf("assign: assigned=%d err=%v", assigned, err)
	}
	var epoch int64
	if err := db.QueryRow(ctx, `SELECT owner_epoch FROM workforce_review_runs WHERE id=$1`, runID).Scan(&epoch); err != nil {
		t.Fatal(err)
	}
	envelope := worker.envelope("canonical-pass", epoch)
	claim, err := store.Claim(ctx, envelope, runID)
	if err != nil {
		t.Fatal(err)
	}
	permit, err := store.AcquireLaunchPermit(ctx, envelope, runID, claim.LeaseToken)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.RecordContainer(ctx, envelope, runID, reviewfacade.ContainerRegistration{ContainerID: strings.Repeat("a", 64), ContainerName: "seven-review-fixture", ImageDigest: fixture.reviewerImage, ExecutionIdentityDigest: strings.Repeat("b", 64)}, claim.LeaseToken, permit.PermitID, permit.PermitToken); err != nil {
		t.Fatal(err)
	}
	report := []byte(`{"schemaVersion":"review-report/v1","verdict":"PASS","score":100,"summary":"fixture passed","reviewTests":[{"command":"git status","exitCode":0,"status":"PASSED","summary":"clean"}]}`)
	// A launch permit is short-lived until the container is durably registered.
	// After registration it is completion authority bound to that exact
	// container and follows the live lease, so a valid 60-minute review cannot
	// be rejected merely because it exceeded the 90-second launch window.
	now := time.Now().UTC()
	store.now = func() time.Time { return now.Add(91 * time.Second) }
	result, err := store.SubmitReport(ctx, envelope, runID, reviewfacade.ReportSubmission{LeaseToken: claim.LeaseToken, PermitID: permit.PermitID, PermitToken: permit.PermitToken, Report: report})
	if err != nil || result.Status != domain.StatusPassed || result.Verdict != "PASS" || len(result.ReportDigest) != 64 {
		t.Fatalf("submit PASS: result=%+v err=%v", result, err)
	}
	assertReviewRunStatusByID(t, ctx, db, runID, domain.StatusPassed)
	assertFixtureStatuses(t, ctx, db, fixture, "SUCCEEDED", "SUCCEEDED", "SUCCEEDED", "SUCCEEDED", "AVAILABLE")
	assertCount(t, ctx, db, `SELECT count(*) FROM workforce_task_events WHERE task_id=$1 AND event_type='task.succeeded'`, fixture.taskID, 1)
	if _, err = store.SubmitReport(ctx, envelope, runID, reviewfacade.ReportSubmission{LeaseToken: claim.LeaseToken, PermitID: permit.PermitID, PermitToken: permit.PermitToken, Report: report}); !errors.Is(err, domain.ErrFenceRejected) {
		t.Fatalf("late report error=%v, want fence", err)
	}
}

func TestReviewRunReviseCreatesExactlyOneSuccessorTurn(t *testing.T) {
	ctx, db, ids := openReviewRunTestDB(t)
	store := newReviewRunTestStore(db, ids)
	store.SetReviewRevisionTurnCreator(continuationinfra.New(db, ids))
	artifactStore := artifactinfra.New(db, ids)
	artifactStore.SetReviewRunGate(store)
	fixture := installReviewRunFixture(t, ctx, db, ids, true)
	t.Cleanup(func() { fixture.cleanup(ctx, db) })
	checkpointID := fixture.installContinuationAuthority(t, ctx, db, ids)
	uploadID, uploadEnvelope, uploadLease := fixture.installUpload(t, ctx, db, ids)
	if err := artifactStore.VerifyUpload(ctx, uploadEnvelope, uploadID, uploadLease); err != nil {
		t.Fatal(err)
	}
	var runID int64
	if err := db.QueryRow(ctx, `SELECT id FROM workforce_review_runs WHERE task_id=$1`, fixture.taskID).Scan(&runID); err != nil {
		t.Fatal(err)
	}
	worker := installReviewWorker(t, ctx, db, ids, fixture.reviewerImage, 1, 2, true)
	t.Cleanup(func() { worker.cleanup(ctx, db) })
	if assigned, err := store.AssignAvailable(ctx, 1); err != nil || assigned != 1 {
		t.Fatalf("assign: assigned=%d err=%v", assigned, err)
	}
	var epoch int64
	if err := db.QueryRow(ctx, `SELECT owner_epoch FROM workforce_review_runs WHERE id=$1`, runID).Scan(&epoch); err != nil {
		t.Fatal(err)
	}
	envelope := worker.envelope("automatic-revision", epoch)
	claim, err := store.Claim(ctx, envelope, runID)
	if err != nil {
		t.Fatal(err)
	}
	permit, err := store.AcquireLaunchPermit(ctx, envelope, runID, claim.LeaseToken)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.RecordContainer(ctx, envelope, runID, reviewfacade.ContainerRegistration{ContainerID: strings.Repeat("9", 64), ContainerName: "automatic-revision", ImageDigest: fixture.reviewerImage, ExecutionIdentityDigest: strings.Repeat("8", 64)}, claim.LeaseToken, permit.PermitID, permit.PermitToken); err != nil {
		t.Fatal(err)
	}
	report := []byte(`{"schemaVersion":"review-report/v1","verdict":"REVISE","score":60,"summary":"revision required","findings":[{"id":"f1","severity":"WARNING","category":"CORRECTNESS","title":"fix","message":"fix the result","evidence":{"path":"README.md","line":1}}]}`)
	result, err := store.SubmitReport(ctx, envelope, runID, reviewfacade.ReportSubmission{LeaseToken: claim.LeaseToken, PermitID: permit.PermitID, PermitToken: permit.PermitToken, Report: report})
	if err != nil || result.Status != domain.StatusRevisionRequired || result.ReportDigest == "" {
		t.Fatalf("submit REVISE: result=%+v err=%v", result, err)
	}

	var successorID, sourceTurnID, sourceRunID, triggerPrincipalID, lineageID, consumedBy int64
	var kind, reportDigest, taskStatus, sourceTurnStatus, sourceAttemptStatus, artifactStatus string
	if err = db.QueryRow(ctx, `SELECT tr.id,COALESCE(tr.source_turn_id,0),COALESCE(tr.source_review_run_id,0),COALESCE(tr.revision_trigger_principal_id,0),tr.continuation_lineage_id,tr.review_revision_kind,COALESCE(tr.review_report_digest,''),t.status
FROM workforce_task_turns tr JOIN workforce_tasks t ON t.current_task_turn_id=tr.id
WHERE tr.task_id=$1 AND tr.turn_number=2`, fixture.taskID).Scan(&successorID, &sourceTurnID, &sourceRunID, &triggerPrincipalID, &lineageID, &kind, &reportDigest, &taskStatus); err != nil {
		t.Fatal(err)
	}
	if sourceTurnID != fixture.turnID || sourceRunID != runID || triggerPrincipalID == 0 || lineageID != fixture.taskID || kind != "AUTOMATIC" || reportDigest != result.ReportDigest || taskStatus != "QUEUED" {
		t.Fatalf("successor identity mismatch: id=%d source=%d run=%d trigger=%d lineage=%d kind=%q digest=%q status=%q", successorID, sourceTurnID, sourceRunID, triggerPrincipalID, lineageID, kind, reportDigest, taskStatus)
	}
	if err = db.QueryRow(ctx, `SELECT status FROM workforce_task_turns WHERE id=$1`, fixture.turnID).Scan(&sourceTurnStatus); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(ctx, `SELECT status FROM workforce_task_attempts WHERE id=$1`, fixture.attemptID).Scan(&sourceAttemptStatus); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(ctx, `SELECT status FROM workforce_artifacts WHERE task_id=$1 AND attempt_id=$2`, fixture.taskID, fixture.attemptID).Scan(&artifactStatus); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(ctx, `SELECT COALESCE(consumed_by_turn_id,0) FROM workforce_continuation_checkpoints WHERE id=$1`, checkpointID).Scan(&consumedBy); err != nil {
		t.Fatal(err)
	}
	if sourceTurnStatus != "SUPERSEDED" || sourceAttemptStatus != "SUPERSEDED" || artifactStatus != "SUPERSEDED" || consumedBy != successorID {
		t.Fatalf("source closure mismatch: turn=%q attempt=%q artifact=%q checkpointConsumer=%d successor=%d", sourceTurnStatus, sourceAttemptStatus, artifactStatus, consumedBy, successorID)
	}
	assertCount(t, ctx, db, `SELECT count(*) FROM workforce_task_turns WHERE source_review_run_id=$1`, runID, 1)
	assertCount(t, ctx, db, `SELECT count(*) FROM workforce_task_events WHERE task_id=$1 AND event_type IN ('task.succeeded','independent_review.attention_required')`, fixture.taskID, 0)
	if _, err = store.SubmitReport(ctx, envelope, runID, reviewfacade.ReportSubmission{LeaseToken: claim.LeaseToken, PermitID: permit.PermitID, PermitToken: permit.PermitToken, Report: report}); !errors.Is(err, domain.ErrFenceRejected) {
		t.Fatalf("late duplicate report error=%v, want fence", err)
	}
	assertCount(t, ctx, db, `SELECT count(*) FROM workforce_task_turns WHERE source_review_run_id=$1`, runID, 1)
}

func TestAcceptRiskPreservesAIVerdictAndSetsHumanOutcome(t *testing.T) {
	ctx, db, ids := openReviewRunTestDB(t)
	recorder := &reviewRunNotificationRecorder{}
	store := newReviewRunTestStore(db, ids)
	store.SetTaskNotificationRecorder(recorder)
	artifactStore := artifactinfra.New(db, ids)
	artifactStore.SetReviewRunGate(store)
	fixture := installReviewRunFixture(t, ctx, db, ids, true)
	t.Cleanup(func() { fixture.cleanup(ctx, db) })
	fixture.installContinuationAuthority(t, ctx, db, ids)
	uploadID, uploadEnvelope, uploadLease := fixture.installUpload(t, ctx, db, ids)
	if err := artifactStore.VerifyUpload(ctx, uploadEnvelope, uploadID, uploadLease); err != nil {
		t.Fatal(err)
	}
	var runID int64
	if err := db.QueryRow(ctx, `SELECT id FROM workforce_review_runs WHERE task_id=$1`, fixture.taskID).Scan(&runID); err != nil {
		t.Fatal(err)
	}
	worker := installReviewWorker(t, ctx, db, ids, fixture.reviewerImage, 1, 2, true)
	t.Cleanup(func() { worker.cleanup(ctx, db) })
	if assigned, err := store.AssignAvailable(ctx, 1); err != nil || assigned != 1 {
		t.Fatalf("assign: assigned=%d err=%v", assigned, err)
	}
	var epoch int64
	if err := db.QueryRow(ctx, `SELECT owner_epoch FROM workforce_review_runs WHERE id=$1`, runID).Scan(&epoch); err != nil {
		t.Fatal(err)
	}
	envelope := worker.envelope("human-accept-risk", epoch)
	claim, err := store.Claim(ctx, envelope, runID)
	if err != nil {
		t.Fatal(err)
	}
	permit, err := store.AcquireLaunchPermit(ctx, envelope, runID, claim.LeaseToken)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.RecordContainer(ctx, envelope, runID, reviewfacade.ContainerRegistration{ContainerID: strings.Repeat("7", 64), ContainerName: "human-accept-risk", ImageDigest: fixture.reviewerImage, ExecutionIdentityDigest: strings.Repeat("6", 64)}, claim.LeaseToken, permit.PermitID, permit.PermitToken); err != nil {
		t.Fatal(err)
	}
	report := []byte(`{"schemaVersion":"review-report/v1","verdict":"REJECT","score":10,"summary":"unsafe result"}`)
	submitted, err := store.SubmitReport(ctx, envelope, runID, reviewfacade.ReportSubmission{LeaseToken: claim.LeaseToken, PermitID: permit.PermitID, PermitToken: permit.PermitToken, Report: report})
	if err != nil || submitted.Status != domain.StatusRejected {
		t.Fatalf("submit REJECT: result=%+v err=%v", submitted, err)
	}
	reason := "Reviewed the remaining risk and approved delivery."
	request := reviewfacade.HumanDecisionRequest{
		TenantID: fixture.tenantID, ReviewRunID: runID, ActorUserID: fixture.creatorID,
		Action: domain.HumanActionAcceptRisk, Reason: reason, IdempotencyKey: "accept-risk-once",
		Proof: stepup.ProofMetadata{BusinessAction: domain.BusinessActionHumanDecision, AssuranceLevel: "AAL2", ProofIdentifier: "proof-secret-value", ChallengeIdentifier: "challenge-secret-value", AuthenticationMethods: []string{"TOTP"}},
	}
	failedCheckID := ids.NextID()
	if _, err = db.Exec(ctx, `INSERT INTO workforce_check_runs (id,task_id,attempt_id,ordinal,name,command_digest,status,exit_code,timeout_seconds,output,started_at,finished_at) VALUES ($1,$2,$3,1,'fixture deterministic check',$4,'FAILED',1,120,'redacted',CURRENT_TIMESTAMP,CURRENT_TIMESTAMP)`, failedCheckID, fixture.taskID, fixture.attemptID, strings.Repeat("5", 64)); err != nil {
		t.Fatal(err)
	}
	if _, _, err = store.Decide(ctx, request); !errors.Is(err, domain.ErrDeterministicAuthorityInvalid) {
		t.Fatalf("accepted risk over deterministic failure error=%v, want authority invalid", err)
	}
	if _, err = db.Exec(ctx, `DELETE FROM workforce_check_runs WHERE id=$1`, failedCheckID); err != nil {
		t.Fatal(err)
	}
	// Hold the authority row until both identical requests have completed
	// their optimistic replay lookup. Once released, one request applies the
	// decision and the other must recheck under the lock and return a replay.
	blocker, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = blocker.Exec(ctx, `SELECT id FROM workforce_review_runs WHERE id=$1 FOR UPDATE`, runID); err != nil {
		t.Fatal(err)
	}
	type decisionResult struct {
		decision reviewfacade.HumanDecisionResult
		replay   bool
		err      error
	}
	results := make(chan decisionResult, 2)
	for range 2 {
		go func() {
			item, wasReplay, decideErr := store.Decide(ctx, request)
			results <- decisionResult{decision: item, replay: wasReplay, err: decideErr}
		}()
	}
	time.Sleep(100 * time.Millisecond)
	if err = blocker.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	var decision reviewfacade.HumanDecisionResult
	replayCount := 0
	for range 2 {
		item := <-results
		if item.err != nil || item.decision.QualityOutcome != domain.QualityHumanAccepted || item.decision.AIVerdict != "REJECT" {
			t.Fatalf("concurrent accept risk: result=%+v", item)
		}
		if decision.ID == 0 {
			decision = item.decision
		} else if decision.ID != item.decision.ID {
			t.Fatalf("identical decisions diverged: %d and %d", decision.ID, item.decision.ID)
		}
		if item.replay {
			replayCount++
		}
	}
	if replayCount != 1 {
		t.Fatalf("concurrent identical decision replay count=%d want=1", replayCount)
	}
	replayed, replay, err := store.Decide(ctx, request)
	if err != nil || !replay || replayed.ID != decision.ID {
		t.Fatalf("decision replay: decision=%+v replay=%t err=%v", replayed, replay, err)
	}
	conflict := request
	conflict.Reason = "A different approval reason."
	if _, _, err = store.Decide(ctx, conflict); !errors.Is(err, domain.ErrHumanIdempotency) {
		t.Fatalf("same key with changed body error=%v, want idempotency conflict", err)
	}
	assertString(t, ctx, db, `SELECT status FROM workforce_tasks WHERE id=$1`, fixture.taskID, "SUCCEEDED")
	assertString(t, ctx, db, `SELECT quality_outcome FROM workforce_tasks WHERE id=$1`, fixture.taskID, domain.QualityHumanAccepted)
	assertString(t, ctx, db, `SELECT report_verdict FROM workforce_review_runs WHERE id=$1`, runID, "REJECT")
	assertString(t, ctx, db, `SELECT status FROM workforce_artifacts WHERE task_id=$1`, fixture.taskID, "AVAILABLE")
	assertCount(t, ctx, db, `SELECT count(*) FROM workforce_task_events WHERE task_id=$1 AND event_type='task.succeeded'`, fixture.taskID, 1)
	var proofHash, challengeHash, methods string
	if err = db.QueryRow(ctx, `SELECT proof_identifier_sha256,challenge_identifier_sha256,authentication_methods_json::text FROM workforce_human_review_decisions WHERE id=$1`, decision.ID).Scan(&proofHash, &challengeHash, &methods); err != nil {
		t.Fatal(err)
	}
	if len(proofHash) != 64 || len(challengeHash) != 64 || proofHash == request.Proof.ProofIdentifier || challengeHash == request.Proof.ChallengeIdentifier || methods != `["TOTP"]` {
		t.Fatalf("unsafe proof persistence: proof=%q challenge=%q methods=%s", proofHash, challengeHash, methods)
	}
	summary, err := store.ReportForTask(ctx, fixture.tenantID, fixture.taskID)
	if err != nil || summary.Verdict != "REJECT" || summary.QualityOutcome != domain.QualityHumanAccepted || len(summary.AvailableActions) != 0 {
		t.Fatalf("accepted-risk summary=%+v err=%v", summary, err)
	}
	page, err := store.ListRunsForTask(ctx, fixture.tenantID, fixture.taskID, 0, 20)
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != runID || page.Items[0].HumanDecision == nil {
		t.Fatalf("review history=%+v err=%v", page, err)
	}
	if page.Items[0].Report == nil || page.Items[0].Report.AutomaticRevisionLimit != 3 || page.Items[0].Report.AutomaticRevisionCount != 0 || len(page.Items[0].Report.AvailableActions) != 0 || page.Items[0].Report.RevisionGuidanceRequired {
		t.Fatalf("review detail report projection lost business fields: %#v", page.Items[0].Report)
	}
	if _, err = store.RunDetailForTask(ctx, fixture.tenantID+1, runID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("cross-tenant detail error=%v, want not found", err)
	}
	encoded, _ := json.Marshal(page)
	for _, forbidden := range []string{"proof-secret-value", "challenge-secret-value", "worker_instance", "ownerEpoch", "leaseToken", "permitToken", "containerId", "providerConfig"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("safe review history leaked %q: %s", forbidden, encoded)
		}
	}
}

func TestManualRevisionCreatesExactlyOneTurnWithoutResettingAutomaticBudget(t *testing.T) {
	ctx, db, ids := openReviewRunTestDB(t)
	store := newReviewRunTestStore(db, ids)
	store.SetReviewRevisionTurnCreator(continuationinfra.New(db, ids))
	artifactStore := artifactinfra.New(db, ids)
	artifactStore.SetReviewRunGate(store)
	fixture := installReviewRunFixture(t, ctx, db, ids, true)
	t.Cleanup(func() { fixture.cleanup(ctx, db) })
	checkpointID := fixture.installContinuationAuthority(t, ctx, db, ids)
	uploadID, uploadEnvelope, uploadLease := fixture.installUpload(t, ctx, db, ids)
	if err := artifactStore.VerifyUpload(ctx, uploadEnvelope, uploadID, uploadLease); err != nil {
		t.Fatal(err)
	}
	var runID int64
	if err := db.QueryRow(ctx, `SELECT id FROM workforce_review_runs WHERE task_id=$1`, fixture.taskID).Scan(&runID); err != nil {
		t.Fatal(err)
	}
	report, digest, err := domain.CanonicalizeReport([]byte(`{"schemaVersion":"review-report/v1","verdict":"REJECT","score":20,"summary":"human correction required"}`))
	if err != nil {
		t.Fatal(err)
	}
	reportJSON, _ := json.Marshal(report)
	for _, statement := range []string{
		`UPDATE workforce_review_runs SET status='REJECTED',report_verdict='REJECT',canonical_report_json=$2::jsonb,canonical_report_digest=$3,closure_reason_code='INDEPENDENT_REVIEW_REJECTED',finished_at=CURRENT_TIMESTAMP WHERE id=$1`,
		`UPDATE workforce_tasks SET status='MANUAL_REVIEW' WHERE id=$1`,
		`UPDATE workforce_task_turns SET status='MANUAL_REVIEW' WHERE id=$1`,
		`UPDATE workforce_task_attempts SET status='MANUAL_REVIEW' WHERE id=$1`,
		`UPDATE workforce_execution_nodes SET status='MANUAL_REVIEW' WHERE id=$1`,
	} {
		var execErr error
		switch {
		case strings.Contains(statement, "workforce_review_runs"):
			_, execErr = db.Exec(ctx, statement, runID, reportJSON, digest)
		case strings.Contains(statement, "workforce_tasks"):
			_, execErr = db.Exec(ctx, statement, fixture.taskID)
		case strings.Contains(statement, "workforce_task_turns"):
			_, execErr = db.Exec(ctx, statement, fixture.turnID)
		case strings.Contains(statement, "workforce_task_attempts"):
			_, execErr = db.Exec(ctx, statement, fixture.attemptID)
		default:
			_, execErr = db.Exec(ctx, statement, fixture.nodeID)
		}
		if execErr != nil {
			t.Fatal(execErr)
		}
	}
	reason := "Authorize one correction Turn from the trusted checkpoint."
	guidance := "Keep the public API compatible and correct only the reported pagination boundary."
	request := reviewfacade.HumanDecisionRequest{
		TenantID: fixture.tenantID, ReviewRunID: runID, ActorUserID: fixture.creatorID,
		Action: domain.HumanActionAuthorizeRevision, Reason: reason, RevisionGuidance: guidance, IdempotencyKey: "manual-revision-once",
		Proof: stepup.ProofMetadata{BusinessAction: domain.BusinessActionHumanDecision, AssuranceLevel: "AAL2", ProofIdentifier: "manual-proof", ChallengeIdentifier: "manual-challenge", AuthenticationMethods: []string{"PASSKEY"}},
	}
	decision, replay, err := store.Decide(ctx, request)
	if err != nil || replay || decision.ResultingTaskTurnID == 0 {
		t.Fatalf("authorize revision: decision=%+v replay=%t err=%v", decision, replay, err)
	}
	replayed, replay, err := store.Decide(ctx, request)
	if err != nil || !replay || replayed.ID != decision.ID || replayed.ResultingTaskTurnID != decision.ResultingTaskTurnID {
		t.Fatalf("authorize replay: decision=%+v replay=%t err=%v", replayed, replay, err)
	}
	var kind, taskStatus string
	var sourceRunID, consumedBy int64
	if err = db.QueryRow(ctx, `SELECT review_revision_kind,source_review_run_id FROM workforce_task_turns WHERE id=$1`, decision.ResultingTaskTurnID).Scan(&kind, &sourceRunID); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(ctx, `SELECT status FROM workforce_tasks WHERE id=$1`, fixture.taskID).Scan(&taskStatus); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(ctx, `SELECT COALESCE(consumed_by_turn_id,0) FROM workforce_continuation_checkpoints WHERE id=$1`, checkpointID).Scan(&consumedBy); err != nil {
		t.Fatal(err)
	}
	if kind != "HUMAN_AUTHORIZED" || sourceRunID != runID || taskStatus != "QUEUED" || consumedBy != decision.ResultingTaskTurnID {
		t.Fatalf("manual successor mismatch: kind=%q run=%d task=%q consumedBy=%d", kind, sourceRunID, taskStatus, consumedBy)
	}
	var storedReason, storedGuidance, successorInstruction string
	if err = db.QueryRow(ctx, `SELECT reason,revision_guidance FROM workforce_human_review_decisions WHERE id=$1`, decision.ID).Scan(&storedReason, &storedGuidance); err != nil {
		t.Fatal(err)
	}
	if err = db.QueryRow(ctx, `SELECT instruction FROM workforce_task_turns WHERE id=$1`, decision.ResultingTaskTurnID).Scan(&successorInstruction); err != nil {
		t.Fatal(err)
	}
	if storedReason != reason || storedGuidance != guidance || !strings.Contains(successorInstruction, guidance) || strings.Contains(successorInstruction, reason) {
		t.Fatalf("reason/guidance separation failed: reason=%q guidance=%q instruction=%q", storedReason, storedGuidance, successorInstruction)
	}
	conflict := request
	conflict.RevisionGuidance = "A different implementation direction."
	if _, _, err = store.Decide(ctx, conflict); !errors.Is(err, domain.ErrHumanIdempotency) {
		t.Fatalf("same key with changed guidance error=%v, want idempotency conflict", err)
	}
	page, err := store.ListRunsForTask(ctx, fixture.tenantID, fixture.taskID, 0, 20)
	if err != nil || len(page.Items) != 1 || page.Items[0].HumanDecision == nil || page.Items[0].HumanDecision.RevisionGuidance != guidance {
		t.Fatalf("safe decision guidance projection=%+v err=%v", page, err)
	}
	assertCount(t, ctx, db, `SELECT count(*) FROM workforce_task_turns WHERE source_review_run_id=$1`, runID, 1)
	assertCount(t, ctx, db, `SELECT count(*) FROM workforce_task_turns WHERE task_id=$1 AND review_revision_kind='AUTOMATIC'`, fixture.taskID, 0)
}

func TestRevisionLimitAndNoProgressEnterManualReviewOnce(t *testing.T) {
	for _, test := range []struct {
		name   string
		facts  progressFacts
		limit  int
		reason string
	}{
		{name: "zero frozen automatic limit", facts: progressFacts{}, limit: 0, reason: reasonRevisionLimit},
		{name: "frozen automatic limit", facts: progressFacts{AutomaticRevisionNumber: 3}, limit: 3, reason: reasonRevisionLimit},
		{name: "two cycles without material progress", facts: progressFacts{AutomaticRevisionNumber: 1, NoProgressStreak: 2}, limit: 3, reason: reasonNoMaterialProgress},
		{name: "human-authorized cycle does not reopen automatic loop", facts: progressFacts{CurrentRevisionKind: "HUMAN_AUTHORIZED"}, limit: 3, reason: reasonRevisionLimit},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, db, ids := openReviewRunTestDB(t)
			store := newReviewRunTestStore(db, ids)
			store.SetTaskNotificationRecorder(&reviewRunNotificationRecorder{})
			artifactStore := artifactinfra.New(db, ids)
			artifactStore.SetReviewRunGate(store)
			fixture := installReviewRunFixture(t, ctx, db, ids, true)
			t.Cleanup(func() { fixture.cleanup(ctx, db) })
			uploadID, uploadEnvelope, uploadLease := fixture.installUpload(t, ctx, db, ids)
			if err := artifactStore.VerifyUpload(ctx, uploadEnvelope, uploadID, uploadLease); err != nil {
				t.Fatal(err)
			}
			var runID int64
			if err := db.QueryRow(ctx, `SELECT id FROM workforce_review_runs WHERE task_id=$1`, fixture.taskID).Scan(&runID); err != nil {
				t.Fatal(err)
			}
			const ownerEpoch int64 = 1
			runtimeReleaseID, runAttemptID := ids.NextID(), ids.NextID()
			imageDigest := "sha256:" + strings.Repeat("c", 64)
			proxyDigest := "sha256:" + strings.Repeat("9", 64)
			selectionDigest := strings.Repeat("6", 64)
			if _, err := db.Exec(ctx, `UPDATE workforce_review_runs SET status='VALIDATING_REPORT',owner_epoch=$2,automatic_revision_limit=$3,
runtime_release_id=$4,reviewer_image_digest=$5,adapter_identity='fixture.review-adapter',adapter_version='1.0.0',adapter_protocol='seven.agent-adapter/v1',
adapter_capabilities_json='["harness.codex.review","review.structured-output"]'::jsonb,model_credential_id=$6,credential_version=1,
model_protocol='OPENAI',primary_model='review-fixture-model',provider_proxy_image_digest=$7,selection_digest=$8,worker_id=$9,worker_instance_id='policy-fixture'
WHERE id=$1`, runID, ownerEpoch, test.limit, runtimeReleaseID, imageDigest, fixture.providerID, proxyDigest, selectionDigest, fixture.workerID); err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(ctx, `INSERT INTO workforce_review_run_attempts
(id,review_run_id,attempt_number,runtime_release_id,reviewer_image_digest,adapter_identity,adapter_version,adapter_protocol,adapter_capabilities_json,
model_credential_id,credential_version,model_protocol,primary_model,provider_proxy_image_digest,selection_digest,
worker_id,worker_instance_id,host_boot_id,owner_epoch,status)
VALUES ($1,$2,1,$3,$4,'fixture.review-adapter','1.0.0','seven.agent-adapter/v1','["harness.codex.review","review.structured-output"]'::jsonb,
$5,1,'OPENAI','review-fixture-model',$6,$7,$8,'policy-fixture','policy-boot',$9,'VALIDATING_REPORT')`,
				runAttemptID, runID, runtimeReleaseID, imageDigest, fixture.providerID, proxyDigest, selectionDigest, fixture.workerID, ownerEpoch); err != nil {
				t.Fatal(err)
			}
			tx, err := db.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if err = store.transitionRevisionRequired(ctx, tx, runID, ownerEpoch, nil, "", test.facts); err != nil {
				_ = tx.Rollback(ctx)
				t.Fatal(err)
			}
			if err = tx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			assertReviewRunStatusByID(t, ctx, db, runID, domain.StatusManualReview)
			assertString(t, ctx, db, `SELECT status FROM workforce_tasks WHERE id=$1`, fixture.taskID, "MANUAL_REVIEW")
			assertString(t, ctx, db, `SELECT closure_reason_code FROM workforce_review_runs WHERE id=$1`, runID, test.reason)
			assertCount(t, ctx, db, `SELECT count(*) FROM workforce_task_turns WHERE source_review_run_id=$1`, runID, 0)
			assertCount(t, ctx, db, `SELECT count(*) FROM workforce_task_events WHERE task_id=$1 AND event_type='independent_review.attention_required'`, fixture.taskID, 1)
		})
	}
}

func TestCreatorCancelClosesIndependentManualReview(t *testing.T) {
	ctx, db, ids := openReviewRunTestDB(t)
	reviewStore := newReviewRunTestStore(db, ids)
	artifactStore := artifactinfra.New(db, ids)
	artifactStore.SetReviewRunGate(reviewStore)
	fixture := installReviewRunFixture(t, ctx, db, ids, true)
	t.Cleanup(func() { fixture.cleanup(ctx, db) })
	uploadID, uploadEnvelope, uploadLease := fixture.installUpload(t, ctx, db, ids)
	if err := artifactStore.VerifyUpload(ctx, uploadEnvelope, uploadID, uploadLease); err != nil {
		t.Fatal(err)
	}
	var runID int64
	if err := db.QueryRow(ctx, `SELECT id FROM workforce_review_runs WHERE task_id=$1`, fixture.taskID).Scan(&runID); err != nil {
		t.Fatal(err)
	}
	for _, change := range []struct {
		query string
		id    int64
	}{
		{`UPDATE workforce_review_runs SET status='MANUAL_REVIEW',closure_reason_code='INDEPENDENT_REVIEW_RETRY_EXHAUSTED',finished_at=CURRENT_TIMESTAMP WHERE id=$1`, runID},
		{`UPDATE workforce_tasks SET status='MANUAL_REVIEW' WHERE id=$1`, fixture.taskID},
		{`UPDATE workforce_task_turns SET status='MANUAL_REVIEW' WHERE id=$1`, fixture.turnID},
		{`UPDATE workforce_task_attempts SET status='MANUAL_REVIEW' WHERE id=$1`, fixture.attemptID},
		{`UPDATE workforce_execution_nodes SET status='MANUAL_REVIEW' WHERE id=$1`, fixture.nodeID},
	} {
		if _, err := db.Exec(ctx, change.query, change.id); err != nil {
			t.Fatal(err)
		}
	}
	taskStore := taskinfra.NewStore(db, ids)
	taskStore.SetReviewRunLifecycle(reviewStore)
	result, err := taskStore.Cancel(ctx, fixture.taskID, fixture.creatorID, "Creator cancelled during independent review")
	if err != nil || result.CurrentStatus != "CANCELLED" {
		t.Fatalf("creator cancel: result=%+v err=%v", result, err)
	}
	assertString(t, ctx, db, `SELECT status FROM workforce_tasks WHERE id=$1`, fixture.taskID, "CANCELLED")
	assertString(t, ctx, db, `SELECT status FROM workforce_review_runs WHERE id=$1`, runID, domain.StatusCancelled)
	assertString(t, ctx, db, `SELECT status FROM workforce_task_turns WHERE id=$1`, fixture.turnID, "CANCELLED")
	assertString(t, ctx, db, `SELECT status FROM workforce_task_attempts WHERE id=$1`, fixture.attemptID, "CANCELLED")
}

func TestHumanCancelClosesIndependentManualReviewAndReplays(t *testing.T) {
	ctx, db, ids := openReviewRunTestDB(t)
	store := newReviewRunTestStore(db, ids)
	artifactStore := artifactinfra.New(db, ids)
	artifactStore.SetReviewRunGate(store)
	fixture := installReviewRunFixture(t, ctx, db, ids, true)
	t.Cleanup(func() { fixture.cleanup(ctx, db) })
	uploadID, uploadEnvelope, uploadLease := fixture.installUpload(t, ctx, db, ids)
	if err := artifactStore.VerifyUpload(ctx, uploadEnvelope, uploadID, uploadLease); err != nil {
		t.Fatal(err)
	}
	var runID int64
	if err := db.QueryRow(ctx, `SELECT id FROM workforce_review_runs WHERE task_id=$1`, fixture.taskID).Scan(&runID); err != nil {
		t.Fatal(err)
	}
	for _, change := range []struct {
		query string
		id    int64
	}{
		{`UPDATE workforce_review_runs SET status='MANUAL_REVIEW',closure_reason_code='INDEPENDENT_REVIEW_RETRY_EXHAUSTED',finished_at=CURRENT_TIMESTAMP WHERE id=$1`, runID},
		{`UPDATE workforce_tasks SET status='MANUAL_REVIEW' WHERE id=$1`, fixture.taskID},
		{`UPDATE workforce_task_turns SET status='MANUAL_REVIEW' WHERE id=$1`, fixture.turnID},
		{`UPDATE workforce_task_attempts SET status='MANUAL_REVIEW' WHERE id=$1`, fixture.attemptID},
		{`UPDATE workforce_execution_nodes SET status='MANUAL_REVIEW' WHERE id=$1`, fixture.nodeID},
	} {
		if _, err := db.Exec(ctx, change.query, change.id); err != nil {
			t.Fatal(err)
		}
	}
	outboxID := ids.NextID()
	if _, err := db.Exec(ctx, `INSERT INTO workforce_outbox_events (id,aggregate_type,aggregate_id,event_type,payload_json,status) VALUES ($1,'review_run',$2,'review_run.assigned','{}','PENDING')`, outboxID, runID); err != nil {
		t.Fatal(err)
	}
	request := reviewfacade.HumanDecisionRequest{
		TenantID: fixture.tenantID, ReviewRunID: runID, ActorUserID: fixture.creatorID,
		Action: domain.HumanActionCancelTask, Reason: "Cancel this task after reviewing the independent review state.", IdempotencyKey: "human-cancel-once",
		Proof: stepup.ProofMetadata{BusinessAction: domain.BusinessActionHumanDecision, AssuranceLevel: "AAL2", ProofIdentifier: "cancel-proof", ChallengeIdentifier: "cancel-challenge", AuthenticationMethods: []string{"TOTP"}},
	}
	decision, replay, err := store.Decide(ctx, request)
	if err != nil || replay || decision.Action != domain.HumanActionCancelTask {
		t.Fatalf("human cancel: decision=%+v replay=%t err=%v", decision, replay, err)
	}
	replayed, replay, err := store.Decide(ctx, request)
	if err != nil || !replay || replayed.ID != decision.ID {
		t.Fatalf("human cancel replay: decision=%+v replay=%t err=%v", replayed, replay, err)
	}
	assertString(t, ctx, db, `SELECT status FROM workforce_tasks WHERE id=$1`, fixture.taskID, "CANCELLED")
	assertString(t, ctx, db, `SELECT status FROM workforce_review_runs WHERE id=$1`, runID, domain.StatusCancelled)
	assertString(t, ctx, db, `SELECT status FROM workforce_outbox_events WHERE id=$1`, outboxID, "SUPERSEDED")
	assertCount(t, ctx, db, `SELECT count(*) FROM workforce_human_review_decisions WHERE review_run_id=$1 AND action='CANCEL_TASK'`, runID, 1)
	lateEnvelope := reviewfacade.MachineEnvelope{SchemaVersion: 1, WorkerID: fixture.workerID, InstanceID: "late", HostBootID: "late", RequestID: "late-report", Generation: 1, OwnerEpoch: 1}
	if _, err = store.SubmitReport(ctx, lateEnvelope, runID, reviewfacade.ReportSubmission{LeaseToken: "late", PermitID: 1, PermitToken: "late", Report: []byte(`{"schemaVersion":"review-report/v1","verdict":"PASS","summary":"late"}`)}); !errors.Is(err, domain.ErrFenceRejected) {
		t.Fatalf("late report after human cancellation error=%v want fence", err)
	}
}

func TestCreatorCancelPreservesHistoricalReviewClosure(t *testing.T) {
	for _, test := range []struct {
		name, historicalStatus, currentStatus string
	}{
		{name: "historical rejected current manual", historicalStatus: domain.StatusRejected, currentStatus: domain.StatusManualReview},
		{name: "historical manual current rejected", historicalStatus: domain.StatusManualReview, currentStatus: domain.StatusRejected},
	} {
		t.Run(test.name, func(t *testing.T) {
			ctx, db, ids := openReviewRunTestDB(t)
			store := newReviewRunTestStore(db, ids)
			fixture := installReviewRunFixture(t, ctx, db, ids, true)
			t.Cleanup(func() { fixture.cleanup(ctx, db) })
			historicalRunID := createReviewRun(t, ctx, db, store, fixture)
			report, digest, err := domain.CanonicalizeReport([]byte(`{"schemaVersion":"review-report/v1","verdict":"REJECT","score":20,"summary":"historical rejection evidence"}`))
			if err != nil {
				t.Fatal(err)
			}
			reportJSON, _ := json.Marshal(report)
			if _, err = db.Exec(ctx, `UPDATE workforce_review_runs SET status=$2,report_verdict='REJECT',canonical_report_json=$3::jsonb,canonical_report_digest=$4,finished_at=CURRENT_TIMESTAMP WHERE id=$1`, historicalRunID, test.historicalStatus, reportJSON, digest); err != nil {
				t.Fatal(err)
			}

			currentTurnID, currentRunID := ids.NextID(), ids.NextID()
			if _, err = db.Exec(ctx, `INSERT INTO workforce_task_turns
(id,task_id,tenant_id,turn_number,instruction,execution_snapshot_digest,continuation_lineage_id,created_by,processor_release_id,processor_digest,status)
SELECT $1,task_id,tenant_id,2,instruction,execution_snapshot_digest,continuation_lineage_id,created_by,processor_release_id,processor_digest,'MANUAL_REVIEW'
FROM workforce_task_turns WHERE id=$2`, currentTurnID, fixture.turnID); err != nil {
				t.Fatal(err)
			}
			if _, err = db.Exec(ctx, `INSERT INTO workforce_review_runs (
id,tenant_id,task_id,task_turn_id,attempt_id,finalization_run_id,result_commit,result_tree,
hard_timeout_seconds,transient_retry_limit,automatic_revision_limit,identity_digest,completion_check_digest,status,
report_verdict,canonical_report_json,canonical_report_digest,finished_at)
SELECT $1,tenant_id,task_id,$2,attempt_id,$3,result_commit,result_tree,
hard_timeout_seconds,transient_retry_limit,automatic_revision_limit,$4,completion_check_digest,$5,
'REJECT',$6::jsonb,$7,CURRENT_TIMESTAMP
FROM workforce_review_runs WHERE id=$8`, currentRunID, currentTurnID, ids.NextID(), fmt.Sprintf("%064x", currentRunID), test.currentStatus, reportJSON, digest, historicalRunID); err != nil {
				t.Fatal(err)
			}
			if _, err = db.Exec(ctx, `UPDATE workforce_tasks SET current_task_turn_id=$2,status='MANUAL_REVIEW' WHERE id=$1`, fixture.taskID, currentTurnID); err != nil {
				t.Fatal(err)
			}

			actorPrincipalID := fixture.principalID
			decisionID := ids.NextID()
			if _, err = db.Exec(ctx, `INSERT INTO workforce_human_review_decisions
(id,tenant_id,task_id,task_turn_id,review_run_id,action,idempotency_key,request_digest,actor_principal_id,reason,
proof_identifier_sha256,challenge_identifier_sha256,assurance_level,authentication_methods_json,ai_verdict,canonical_report_digest,status)
VALUES ($1,$2,$3,$4,$5,'AUTHORIZE_ONE_REVISION','historical-decision',$6,$7,'historical audited decision',$8,$9,'AAL2','["TOTP"]'::jsonb,'REJECT',$10,'APPLIED')`,
				decisionID, fixture.tenantID, fixture.taskID, fixture.turnID, historicalRunID, strings.Repeat("7", 64), actorPrincipalID, strings.Repeat("8", 64), strings.Repeat("9", 64), digest); err != nil {
				t.Fatal(err)
			}

			// An older active Run still represents live work and must be fenced even
			// though it does not belong to the current Turn.
			activeTurnID, activeRunID := ids.NextID(), ids.NextID()
			if _, err = db.Exec(ctx, `INSERT INTO workforce_task_turns
(id,task_id,tenant_id,turn_number,instruction,execution_snapshot_digest,continuation_lineage_id,created_by,processor_release_id,processor_digest,status)
SELECT $1,task_id,tenant_id,3,instruction,execution_snapshot_digest,continuation_lineage_id,created_by,processor_release_id,processor_digest,'SUCCEEDED'
FROM workforce_task_turns WHERE id=$2`, activeTurnID, fixture.turnID); err != nil {
				t.Fatal(err)
			}
			if _, err = db.Exec(ctx, `INSERT INTO workforce_review_runs (
id,tenant_id,task_id,task_turn_id,attempt_id,finalization_run_id,result_commit,result_tree,
hard_timeout_seconds,transient_retry_limit,automatic_revision_limit,identity_digest,completion_check_digest,status)
SELECT $1,tenant_id,task_id,$2,attempt_id,$3,result_commit,result_tree,
hard_timeout_seconds,transient_retry_limit,automatic_revision_limit,$4,completion_check_digest,'PENDING'
FROM workforce_review_runs WHERE id=$5`, activeRunID, activeTurnID, ids.NextID(), fmt.Sprintf("%064x", activeRunID), historicalRunID); err != nil {
				t.Fatal(err)
			}

			tx, err := db.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			if err = store.CancelForTask(ctx, tx, fixture.taskID, "TASK_CANCELLED"); err != nil {
				_ = tx.Rollback(ctx)
				t.Fatal(err)
			}
			if err = tx.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			assertString(t, ctx, db, `SELECT status FROM workforce_review_runs WHERE id=$1`, historicalRunID, test.historicalStatus)
			assertString(t, ctx, db, `SELECT canonical_report_digest FROM workforce_review_runs WHERE id=$1`, historicalRunID, digest)
			assertString(t, ctx, db, `SELECT status FROM workforce_review_runs WHERE id=$1`, currentRunID, domain.StatusCancelled)
			assertString(t, ctx, db, `SELECT status FROM workforce_review_runs WHERE id=$1`, activeRunID, domain.StatusCancelled)
			var preservedDecisions int
			if err = db.QueryRow(ctx, `SELECT count(*) FROM workforce_human_review_decisions WHERE id=$1 AND status='APPLIED' AND canonical_report_digest=$2`, decisionID, digest).Scan(&preservedDecisions); err != nil {
				t.Fatal(err)
			}
			if preservedDecisions != 1 {
				t.Fatalf("historical HumanDecision count=%d want=1", preservedDecisions)
			}
		})
	}
}

type reviewRunFixture struct {
	tenantID, taskID, turnID, attemptID, nodeID, finalizationID, workerID, providerID int64
	creatorID, principalID, projectID, projectVersionID                               int64
	commit, tree, reviewerImage                                                       string
}

func installReviewRunFixture(t *testing.T, ctx context.Context, db *pgxpool.Pool, ids *reviewRunTestIDGen, reviewRequired bool) reviewRunFixture {
	t.Helper()
	fixture := reviewRunFixture{
		tenantID: ids.NextID(), taskID: ids.NextID(), turnID: ids.NextID(), attemptID: ids.NextID(), nodeID: ids.NextID(), finalizationID: ids.NextID(), workerID: ids.NextID(),
		commit: strings.Repeat("a", 40), tree: strings.Repeat("b", 40), reviewerImage: "sha256:" + strings.Repeat("c", 64),
	}
	createdBy := ids.NextID()
	var err error
	fixture.creatorID = createdBy
	if _, err := db.Exec(ctx, `INSERT INTO sys_user (id,"userAccount","nickName") VALUES ($1,$2,$3)`, createdBy, fmt.Sprintf("rf-%x", createdBy), "Review Fixture"); err != nil {
		t.Fatal(err)
	}
	fixture.principalID, fixture.projectID, fixture.projectVersionID = ids.NextID(), ids.NextID(), ids.NextID()
	if _, err = db.Exec(ctx, `INSERT INTO workforce_principals (id,principal_type,user_id) VALUES ($1,'USER',$2)`, fixture.principalID, createdBy); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `INSERT INTO workforce_projects
(id,tenant_id,project_key,status,current_version_id,created_by_principal_id,updated_by_principal_id,create_idempotency_key,create_request_digest)
VALUES ($1,$2,$3,'ACTIVE',$4,$5,$5,$6,$7)`, fixture.projectID, fixture.tenantID,
		fmt.Sprintf("review-fixture-%d", fixture.projectID), fixture.projectVersionID, fixture.principalID,
		fmt.Sprintf("review-fixture-%d", fixture.projectID), strings.Repeat("9", 64)); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `INSERT INTO workforce_project_versions
(id,tenant_id,project_id,name,kind,completion_check_digest,result_processor_release_id,review_prompt,
 delivery_mode,delivery_authorization,created_by_principal_id)
VALUES ($1,$2,$3,'ReviewRun fixture','GENERAL',$4,$5,'Inspect the frozen result for correctness and regression risk.',
 'NONE','NONE',$6)`, fixture.projectVersionID, fixture.tenantID, fixture.projectID, strings.Repeat("a", 64), ids.NextID(), fixture.principalID); err != nil {
		t.Fatal(err)
	}
	if reviewRequired {
		fixture.providerID = ids.NextID()
		if _, err = db.Exec(ctx, `INSERT INTO workforce_model_credentials
(id,tenant_id,name,protocol,base_url,secret_ciphertext,secret_edek,secret_wrap_key_ref,secret_hint,secret_version,models_json,status)
VALUES ($1,$2,'Review fixture','OPENAI','http://127.0.0.1:1',decode('Y2lwaGVy','base64'),decode('ZWRlaw==','base64'),'fixture','hint',1,'["review-fixture-model"]'::jsonb,'ACTIVE')`, fixture.providerID, fixture.tenantID); err != nil {
			t.Fatal(err)
		}
	}
	var modelCredentialID any
	var primaryModel any
	if reviewRequired {
		modelCredentialID = fixture.providerID
		primaryModel = "review-fixture-model"
	}
	if _, err = db.Exec(ctx, `INSERT INTO workforce_tasks
(id,tenant_id,created_by,idempotency_key,request_hash,repository_url,repository_ref,task_instruction,status,current_task_turn_id,
result_processor_release_id,result_processor_parameters_json,name,project_id,project_version_id,review_enabled,
model_credential_id,primary_model,execution_requirements_json,execution_requirements_digest)
VALUES ($1,$2,$3,$4,$5,'','','review-run fixture','UPLOADING',$6,$7,'{}'::jsonb,'ReviewRun fixture',$8,$9,$10,$11,$12,'{}'::jsonb,$13)`,
		fixture.taskID, fixture.tenantID, createdBy, fmt.Sprintf("review-run-%d", fixture.taskID), strings.Repeat("d", 64), fixture.turnID,
		ids.NextID(), fixture.projectID, fixture.projectVersionID, reviewRequired, modelCredentialID, primaryModel, strings.Repeat("b", 64)); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `INSERT INTO workforce_task_turns
(id,task_id,tenant_id,turn_number,instruction,execution_snapshot_digest,continuation_lineage_id,created_by,processor_release_id,processor_digest,status)
VALUES ($1,$2,$3,1,'review fixture',$4,$5,$6,$7,$8,'ACTIVE')`, fixture.turnID, fixture.taskID, fixture.tenantID, strings.Repeat("e", 64), fixture.taskID, createdBy, ids.NextID(), strings.Repeat("f", 64)); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `INSERT INTO workforce_execution_nodes
(id,task_id,task_turn_id,node_key,status,execution_requirement_digest)
VALUES ($1,$2,$3,'main','UPLOADING',$4)`, fixture.nodeID, fixture.taskID, fixture.turnID, strings.Repeat("1", 64)); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `INSERT INTO workforce_task_attempts
(id,task_id,attempt_no,worker_id,workspace_path,status,lease_token,lease_expires_at,runtime_driver,task_turn_id,assignment_selection_digest,resolved_execution_snapshot_json)
VALUES ($1,$2,1,$3,'/fixture','EXECUTION_COMPLETED','fixture',CURRENT_TIMESTAMP+INTERVAL '1 hour','docker',$4,$5,'{}'::jsonb)`, fixture.attemptID, fixture.taskID, fixture.workerID, fixture.turnID, strings.Repeat("7", 64)); err != nil {
		t.Fatal(err)
	}
	manifest, _ := json.Marshal(map[string]any{"resultCommit": fixture.commit, "resultTree": fixture.tree, "reviewEvidence": []map[string]any{{"path": "README.md", "lineCount": 10}}})
	if _, err = db.Exec(ctx, `INSERT INTO workforce_finalization_runs
(id,task_id,task_turn_id,execution_node_id,attempt_id,generation,input_identity_digest,completion_proposal_id,result_commit,result_tree,manifest_json,status,worker_id,worker_instance_id,host_boot_id,frozen_at)
VALUES ($1,$2,$3,$4,$5,1,$6,$7,$8,$9,$10::jsonb,'FROZEN',$11,'fixture-finalizer','fixture-boot',CURRENT_TIMESTAMP)`,
		fixture.finalizationID, fixture.taskID, fixture.turnID, fixture.nodeID, fixture.attemptID, strings.Repeat("2", 64), ids.NextID(), fixture.commit, fixture.tree, string(manifest), fixture.workerID); err != nil {
		t.Fatal(err)
	}
	return fixture
}

func (f reviewRunFixture) finalizationInput() reviewfacade.FinalizationInput {
	return reviewfacade.FinalizationInput{TenantID: f.tenantID, TaskID: f.taskID, TaskTurnID: f.turnID, AttemptID: f.attemptID, FinalizationRunID: f.finalizationID, ResultCommit: f.commit, ResultTree: f.tree}
}

func (f reviewRunFixture) installUpload(t *testing.T, ctx context.Context, db *pgxpool.Pool, ids *reviewRunTestIDGen) (int64, taskdispatchdomain.MachineEnvelope, string) {
	t.Helper()
	uploadID := ids.NextID()
	lease := "upload-lease-" + fmt.Sprint(uploadID)
	envelope := taskdispatchdomain.MachineEnvelope{SchemaVersion: 1, WorkerID: f.workerID, InstanceID: "fixture-postprocess", HostBootID: "fixture-postprocess-boot", RequestID: "fixture-upload", Generation: 1}
	if _, err := db.Exec(ctx, `INSERT INTO workforce_artifact_upload_attempts
(id,finalization_run_id,task_id,task_turn_id,attempt_id,artifact_type,artifact_name,object_key,sha256,size_bytes,media_type,worker_id,host_boot_id,status,lease_sha256,lease_expires_at)
VALUES ($1,$2,$3,$4,$5,'PATCH','result.patch',$6,$7,1,'text/x-diff',$8,$9,'UPLOADING',$10,CURRENT_TIMESTAMP+INTERVAL '5 minutes')`,
		uploadID, f.finalizationID, f.taskID, f.turnID, f.attemptID, fmt.Sprintf("fixture/%d/result.patch", f.taskID), strings.Repeat("6", 64), f.workerID, envelope.HostBootID, testSHA256(lease)); err != nil {
		t.Fatal(err)
	}
	return uploadID, envelope, lease
}

func (f reviewRunFixture) installContinuationAuthority(t *testing.T, ctx context.Context, db *pgxpool.Pool, ids *reviewRunTestIDGen) int64 {
	t.Helper()
	digest := func(r byte) string { return strings.Repeat(string(r), 64) }
	if _, err := db.Exec(ctx, `INSERT INTO workforce_task_input_snapshots
(id,tenant_id,task_id,task_turn_id,source_kind,source_commit,source_tree,source_identity_digest,repository_bundle_object_key,repository_bundle_content_sha256,repository_bundle_object_sha256,repository_bundle_size_bytes,input_snapshot_digest)
VALUES ($1,$2,$3,$4,'REPOSITORY',$5,$6,$7,$8,$9,$10,1,$11)`, ids.NextID(), f.tenantID, f.taskID, f.turnID, f.commit, f.tree, digest('1'), "fixture/input.bundle", digest('2'), digest('3'), digest('4')); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO workforce_task_git_plans
(id,tenant_id,task_id,task_turn_id,source_ref_type,source_ref_name,source_commit,review_base_branch,review_base_commit,result_branch,delivery_mode,delivery_authorization,plan_digest,status)
VALUES ($1,$2,$3,$4,'COMMIT',$5,$6,'main',$6,$7,'NONE','',$8,'ACTIVE')`, ids.NextID(), f.tenantID, f.taskID, f.turnID, f.commit, f.commit, "review/result", digest('5')); err != nil {
		t.Fatal(err)
	}
	checkpointID := ids.NextID()
	if _, err := db.Exec(ctx, `INSERT INTO workforce_continuation_checkpoints
(id,task_id,task_turn_id,attempt_id,evaluation_cycle_id,decision,checkpoint_identity_sha256,format_version,
 repository_bundle_object_key,repository_bundle_content_sha256,repository_bundle_object_sha256,repository_bundle_size_bytes,
 workspace_overlay_object_key,workspace_overlay_content_sha256,workspace_overlay_object_sha256,workspace_overlay_size_bytes,
 selection_digest,selection_policy_digest,gitignore_digest,session_object_key,session_sha256,identity_manifest_object_key,manifest_sha256,
 task_execution_snapshot_digest,base_commit,result_commit,result_tree,session_lineage_digest,session_content_sha256,session_object_sha256,session_size_bytes,
 manifest_content_sha256,manifest_object_sha256,manifest_size_bytes,encryption_algorithm,encryption_key_id,wrapped_data_key,status,expires_at,available_at)
VALUES ($1,$2,$3,$4,$5,'PASS',$6,2,$7,$8,$9,1,$10,$11,$12,1,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,1,$27,$28,1,'AES-256-GCM','fixture-key','fixture-wrapped','AVAILABLE',CURRENT_TIMESTAMP+INTERVAL '1 hour',CURRENT_TIMESTAMP)`,
		checkpointID, f.taskID, f.turnID, f.attemptID, ids.NextID(), digest('6'), "fixture/repository.bundle", digest('7'), digest('8'), "fixture/overlay.bundle", digest('9'), digest('a'), digest('b'), digest('c'), digest('d'), "fixture/session.bundle", digest('e'), "fixture/manifest.json", digest('f'), digest('1'), f.commit, f.commit, f.tree, digest('2'), digest('3'), digest('4'), digest('5'), digest('6')); err != nil {
		t.Fatal(err)
	}
	return checkpointID
}

func (f reviewRunFixture) cleanup(ctx context.Context, db *pgxpool.Pool) {
	queries := []string{
		`DELETE FROM workforce_human_review_decisions WHERE task_id=$1`,
		`DELETE FROM workforce_outbox_events WHERE aggregate_type='review_run' AND aggregate_id IN (SELECT id FROM workforce_review_runs WHERE task_id=$1)`,
		`DELETE FROM workforce_outbox_events WHERE aggregate_type='artifact_upload' AND aggregate_id IN (SELECT id FROM workforce_artifact_upload_attempts WHERE task_id=$1)`,
		`DELETE FROM workforce_review_run_attempts WHERE review_run_id IN (SELECT id FROM workforce_review_runs WHERE task_id=$1)`,
		`DELETE FROM workforce_review_runs WHERE task_id=$1`,
		`DELETE FROM workforce_artifacts WHERE task_id=$1`,
		`DELETE FROM workforce_artifact_upload_attempts WHERE task_id=$1`,
		`DELETE FROM workforce_finalization_runs WHERE task_id=$1`,
		`DELETE FROM workforce_task_events WHERE task_id=$1`,
		`DELETE FROM workforce_task_input_references WHERE task_input_snapshot_id IN (SELECT id FROM workforce_task_input_snapshots WHERE task_id=$1)`,
		`DELETE FROM workforce_task_input_snapshots WHERE task_id=$1`,
		`DELETE FROM workforce_task_git_plans WHERE task_id=$1`,
		`DELETE FROM workforce_continuation_checkpoints WHERE task_id=$1`,
		`DELETE FROM workforce_check_runs WHERE task_id=$1`,
		`DELETE FROM workforce_execution_nodes WHERE task_id=$1`,
		`DELETE FROM workforce_task_attempts WHERE task_id=$1`,
		`DELETE FROM workforce_task_turns WHERE task_id=$1`,
		`DELETE FROM workforce_tasks WHERE id=$1`,
		`DELETE FROM workforce_model_credentials WHERE id=$3`,
		`DELETE FROM workforce_principals WHERE user_id=$2`,
		`DELETE FROM sys_user WHERE id=$2`,
	}
	for _, query := range queries {
		if strings.Contains(query, "$3") {
			_, _ = db.Exec(ctx, query, f.taskID, f.creatorID, f.providerID)
		} else if strings.Contains(query, "$2") {
			_, _ = db.Exec(ctx, query, f.taskID, f.creatorID)
		} else {
			_, _ = db.Exec(ctx, query, f.taskID)
		}
	}
}

func createReviewRun(t *testing.T, ctx context.Context, db *pgxpool.Pool, store *Store, fixture reviewRunFixture) int64 {
	t.Helper()
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	decision, err := store.GateVerifiedFinalization(ctx, tx, fixture.finalizationInput())
	if err != nil {
		_ = tx.Rollback(ctx)
		t.Fatal(err)
	}
	if !decision.ReviewRequired || decision.ReviewRunID == 0 {
		_ = tx.Rollback(ctx)
		t.Fatalf("expected review-required finalization decision, got %+v", decision)
	}
	if err = tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	return decision.ReviewRunID
}

type reviewWorker struct {
	id, maxReviews, maxTotal int64
	instance, boot           string
}

func installReviewWorker(t *testing.T, ctx context.Context, db *pgxpool.Pool, ids *reviewRunTestIDGen, image string, maxReviews, maxTotal int64, reviewCapabilities bool) reviewWorker {
	t.Helper()
	worker := reviewWorker{id: ids.NextID(), maxReviews: maxReviews, maxTotal: maxTotal, instance: fmt.Sprintf("review-worker-%d", ids.NextID()), boot: fmt.Sprintf("review-boot-%d", ids.NextID())}
	if _, err := db.Exec(ctx, `INSERT INTO workforce_workers (id,name,token_hint,max_active_attempts,max_active_reviews,max_total_containers,current_instance_id) VALUES ($1,$2,'fixture',1,$3,$4,$5)`, worker.id, fmt.Sprintf("Review Worker %d", worker.id), maxReviews, maxTotal, worker.instance); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO workforce_worker_sessions (instance_id,worker_id,host_boot_id,started_at,last_heartbeat_at,consumer_state,docker_ready,capabilities_json) VALUES ($1,$2,$3,CURRENT_TIMESTAMP,CURRENT_TIMESTAMP,'CONSUMING',TRUE,'{"independentReviewConsumer":"v1"}'::jsonb)`, worker.instance, worker.id, worker.boot); err != nil {
		t.Fatal(err)
	}
	capabilities := []string{}
	if reviewCapabilities {
		capabilities = []string{"harness.codex.review", "review.structured-output"}
	}
	probeCapabilities := make([]map[string]string, 0, len(capabilities))
	for _, capability := range capabilities {
		probeCapabilities = append(probeCapabilities, map[string]string{"name": capability, "status": "supported"})
	}
	manifest, _ := json.Marshal(map[string]any{
		"adapter": map[string]any{
			"id": "fixture.review-adapter", "version": "1.0.0", "capabilities": capabilities,
			"protocol": map[string]any{"name": "seven.agent-adapter", "min": 1, "max": 1},
		},
		"probe": map[string]any{"capabilities": probeCapabilities},
		// Admitted images declare their supported provider protocol; omission
		// correctly makes a Runtime ineligible in the real assignment resolver.
		"scheduling": map[string]any{"providerProtocols": []string{"OPENAI"}, "hookEvents": []string{}},
		"resultProcessor": map[string]any{
			"pythonRuntimeIdentity": strings.Repeat("1", 64), "bootstrapSha256": strings.Repeat("2", 64),
			"sdkSha256": strings.Repeat("3", 64), "moduleIndexSha256": strings.Repeat("4", 64),
		},
	})
	capabilityJSON, _ := json.Marshal(capabilities)
	releaseID := ids.NextID()
	if _, err := db.Exec(ctx, `INSERT INTO workforce_harness_releases
(id,alias,image_ref,image_digest,contract_version,supervisor_version,harness_type,harness_version,capabilities_json,
scheduling_scope,scheduling_priority,environment_capabilities_json,execution_kinds_json,scheduling_digest,admission_evidence_json,status)
VALUES ($1,$2,$3,$4,'seven.worker-image/v1','2','codex','0.147.0',$5::jsonb,'PRODUCTION',100,'[]'::jsonb,'["REVIEW_RUN"]'::jsonb,$6,'{}'::jsonb,'ACTIVE')
ON CONFLICT (image_digest) DO NOTHING`, releaseID, fmt.Sprintf("review-runtime-%d", releaseID), "fixture@"+image, image, string(capabilityJSON), strings.Repeat("6", 64)); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO workforce_worker_images
(worker_id,instance_id,image_digest,image_ref,role,contract_version,manifest_digest,manifest_json,capabilities_json,
scheduling_scope,scheduling_priority,environment_capabilities_json,execution_kinds_json,scheduling_digest,admission_status,presence_status,inventory_version,inventory_digest)
VALUES ($1,$2,$3,$4,'attempt','seven.worker-image/v1',$5,$6::jsonb,$7::jsonb,'PRODUCTION',100,'[]'::jsonb,'["REVIEW_RUN"]'::jsonb,$8,'ADMITTED','PRESENT',1,$9)`,
		worker.id, worker.instance, image, "fixture@"+image, strings.Repeat("7", 64), string(manifest), string(capabilityJSON), strings.Repeat("6", 64), strings.Repeat("8", 64)); err != nil {
		t.Fatal(err)
	}
	proxyDigest := "sha256:" + strings.Repeat("9", 64)
	if _, err := db.Exec(ctx, `INSERT INTO workforce_worker_images
(worker_id,instance_id,image_digest,image_ref,role,contract_version,manifest_digest,manifest_json,capabilities_json,
scheduling_scope,scheduling_priority,environment_capabilities_json,execution_kinds_json,scheduling_digest,admission_status,presence_status,inventory_version,inventory_digest)
VALUES ($1,$2,$3,$4,'provider-proxy','seven.worker-image/v1',$5,'{}'::jsonb,'{"providerProxy":true}'::jsonb,
'PRODUCTION',100,'[]'::jsonb,'[]'::jsonb,$6,'ADMITTED','PRESENT',1,$7)`, worker.id, worker.instance, proxyDigest,
		"fixture@"+proxyDigest, strings.Repeat("a", 64), strings.Repeat("b", 64), strings.Repeat("c", 64)); err != nil {
		t.Fatal(err)
	}
	return worker
}

func (w reviewWorker) envelope(requestID string, epoch int64) reviewfacade.MachineEnvelope {
	return reviewfacade.MachineEnvelope{SchemaVersion: 1, WorkerID: w.id, InstanceID: w.instance, HostBootID: w.boot, RequestID: requestID, Generation: 1, OwnerEpoch: epoch}
}

func (w reviewWorker) cleanup(ctx context.Context, db *pgxpool.Pool) {
	_, _ = db.Exec(ctx, `DELETE FROM workforce_worker_images WHERE worker_id=$1`, w.id)
	_, _ = db.Exec(ctx, `DELETE FROM workforce_worker_sessions WHERE worker_id=$1`, w.id)
	_, _ = db.Exec(ctx, `DELETE FROM workforce_workers WHERE id=$1`, w.id)
}

type reviewRunNotificationRecorder struct {
	transitions []tasknotification.TaskNotificationTransition
}

func (r *reviewRunNotificationRecorder) RecordTransition(_ context.Context, _ pgx.Tx, transition tasknotification.TaskNotificationTransition) (tasknotification.TransitionDisposition, error) {
	r.transitions = append(r.transitions, transition)
	return "TASK_ACTION_REQUIRED", nil
}

func openReviewRunTestDB(t *testing.T) (context.Context, *pgxpool.Pool, *reviewRunTestIDGen) {
	t.Helper()
	dsn := strings.TrimSpace(os.Getenv(reviewRunTestDSNEnv))
	if dsn == "" {
		t.Skip("set REVIEW_RUN_TEST_POSTGRES_DSN to an isolated migrated PostgreSQL database")
	}
	ctx := context.Background()
	db, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	return ctx, db, newReviewRunTestIDGen()
}

func assertFixtureStatuses(t *testing.T, ctx context.Context, db *pgxpool.Pool, fixture reviewRunFixture, task, turn, attempt, node, artifact string) {
	t.Helper()
	assertString(t, ctx, db, `SELECT status FROM workforce_tasks WHERE id=$1`, fixture.taskID, task)
	assertString(t, ctx, db, `SELECT status FROM workforce_task_turns WHERE id=$1`, fixture.turnID, turn)
	assertString(t, ctx, db, `SELECT status FROM workforce_task_attempts WHERE id=$1`, fixture.attemptID, attempt)
	assertString(t, ctx, db, `SELECT status FROM workforce_execution_nodes WHERE id=$1`, fixture.nodeID, node)
	assertString(t, ctx, db, `SELECT status FROM workforce_artifacts WHERE task_id=$1`, fixture.taskID, artifact)
}

func assertReviewRunStatus(t *testing.T, ctx context.Context, db *pgxpool.Pool, taskID int64, want string) {
	t.Helper()
	assertString(t, ctx, db, `SELECT status FROM workforce_review_runs WHERE task_id=$1`, taskID, want)
}

func assertReviewRunStatusByID(t *testing.T, ctx context.Context, db *pgxpool.Pool, runID int64, want string) {
	t.Helper()
	assertString(t, ctx, db, `SELECT status FROM workforce_review_runs WHERE id=$1`, runID, want)
}

func assertString(t *testing.T, ctx context.Context, db *pgxpool.Pool, query string, id int64, want string) {
	t.Helper()
	var got string
	if err := db.QueryRow(ctx, query, id).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("query status=%q want %q: %s", got, want, query)
	}
}

func assertCount(t *testing.T, ctx context.Context, db *pgxpool.Pool, query string, id int64, want int) {
	t.Helper()
	var got int
	if err := db.QueryRow(ctx, query, id).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("query count=%d want %d: %s", got, want, query)
	}
}

func testSHA256(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}
