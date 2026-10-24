package infrastructure

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	completioncheck "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/completioncheck/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/reviewrun/domain"
	taskinfra "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/task/infrastructure"
)

func TestReviewCompletionCheckPersistedOccurrenceConflict(t *testing.T) {
	ctx, db, ids := openReviewRunTestDB(t)
	store := newReviewRunTestStore(db, ids)
	fixture := installReviewRunFixture(t, ctx, db, ids, true)
	t.Cleanup(func() { fixture.cleanup(ctx, db) })
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	setDigest := func(digest string) {
		t.Helper()
		_, err := tx.Exec(ctx, `UPDATE workforce_task_turns SET completion_check_digest=$2 WHERE id=$1`, fixture.turnID, digest)
		if err != nil {
			t.Fatal(err)
		}
	}
	digest := strings.Repeat("a", 64)
	setDigest(digest)
	first, err := store.GateVerifiedFinalization(ctx, tx, fixture.finalizationInput())
	if err != nil {
		t.Fatal(err)
	}
	var stored string
	if err = tx.QueryRow(ctx, `SELECT completion_check_digest FROM workforce_review_runs WHERE id=$1`, first.ReviewRunID).Scan(&stored); err != nil || stored != digest {
		t.Fatalf("persisted digest=%q error=%v", stored, err)
	}
	replay, err := store.GateVerifiedFinalization(ctx, tx, fixture.finalizationInput())
	if err != nil || replay.ReviewRunID != first.ReviewRunID {
		t.Fatalf("replay=%+v error=%v", replay, err)
	}
	setDigest(strings.Repeat("b", 64))
	if _, err = store.GateVerifiedFinalization(ctx, tx, fixture.finalizationInput()); !errors.Is(err, domain.ErrIdentityConflict) {
		t.Fatalf("different completion digest should conflict: %v", err)
	}
}

func TestReviewCompletionCheckHistoricalTurnProjection(t *testing.T) {
	ctx, db, ids := openReviewRunTestDB(t)
	store := newReviewRunTestStore(db, ids)
	fixture := installReviewRunFixture(t, ctx, db, ids, true)
	t.Cleanup(func() { fixture.cleanup(ctx, db) })
	oldInput := completioncheck.Input{Mode: "SCRIPT", Script: "PRIVATE_SCRIPT", Instructions: "old acceptance criteria"}
	oldJSON, err := json.Marshal(oldInput)
	if err != nil {
		t.Fatal(err)
	}
	oldDigest, err := completioncheck.Digest(oldInput)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `UPDATE workforce_task_turns SET completion_check_json=$2::jsonb,completion_check_digest=$3,completion_check_references_json='[]'::jsonb WHERE id=$1`, fixture.turnID, string(oldJSON), oldDigest); err != nil {
		t.Fatal(err)
	}
	runID := createReviewRun(t, ctx, db, store, fixture)
	// Replaying finalization must keep resolving the source Turn. ProjectVersion
	// changes are append-only and cannot rewrite this Task's acceptance input.
	if replayID := createReviewRun(t, ctx, db, store, fixture); replayID != runID {
		t.Fatal("latest Task input changed historical occurrence identity")
	}
	item, err := store.RunDetailForTask(ctx, fixture.tenantID, runID)
	if err != nil || item.CompletionCheckDigest != oldDigest || item.TaskTurnID != fixture.turnID {
		t.Fatalf("historical identity mismatch: %+v %v", item, err)
	}
	reader := taskinfra.NewStore(db, ids)
	input, err := reader.IndependentReviewInput(ctx, fixture.tenantID, item.TaskID, item.TaskTurnID)
	if err != nil || input.CompletionCheckDigest != oldDigest || input.Instructions != oldInput.Instructions {
		t.Fatalf("historical reader used latest Task: %+v %v", input, err)
	}
	safeJSON, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"PRIVATE_SCRIPT", "hostPath", "objectKey"} {
		if strings.Contains(string(safeJSON), forbidden) {
			t.Fatalf("historical reader exposed %s", forbidden)
		}
	}
	if _, err = store.RunDetailForTask(ctx, fixture.tenantID+1, runID); !errors.Is(err, domain.ErrNotFound) {
		t.Fatalf("cross tenant history was not hidden: %v", err)
	}
	// Legacy per-Turn NULLs stay absent.
	if _, err = db.Exec(ctx, `UPDATE workforce_task_turns SET completion_check_json=NULL,completion_check_digest=NULL,completion_check_references_json=NULL WHERE id=$1`, fixture.turnID); err != nil {
		t.Fatal(err)
	}
	legacy, err := reader.IndependentReviewInput(ctx, fixture.tenantID, fixture.taskID, fixture.turnID)
	if err != nil || legacy.CompletionCheckDigest != "" || legacy.Instructions != "" {
		t.Fatalf("legacy turn acquired latest Task input: %+v %v", legacy, err)
	}
	// Invalid JSON discriminants produce a stable error, never raw script/context.
	if _, err = db.Exec(ctx, `UPDATE workforce_task_turns SET completion_check_json='{"mode":"PRIVATE_INVALID_MODE","script":"PRIVATE_SCRIPT"}'::jsonb,completion_check_digest=$2 WHERE id=$1`, fixture.turnID, oldDigest); err != nil {
		t.Fatal(err)
	}
	_, err = reader.IndependentReviewInput(ctx, fixture.tenantID, fixture.taskID, fixture.turnID)
	if err == nil || strings.Contains(err.Error(), "PRIVATE") {
		t.Fatalf("invalid discriminant echoed context: %v", err)
	}
}
