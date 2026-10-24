package infrastructure

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	tasknotification "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/tasknotification/facade"
)

type resultProcessorNotificationRecorder struct {
	transitions []tasknotification.TaskNotificationTransition
	err         error
}

func (r *resultProcessorNotificationRecorder) RecordTransition(_ context.Context, _ pgx.Tx, transition tasknotification.TaskNotificationTransition) (tasknotification.TransitionDisposition, error) {
	r.transitions = append(r.transitions, transition)
	return "TASK_ACTION_REQUIRED", r.err
}

func TestRecordContinuationNotificationUsesAvailableCheckpointIdentity(t *testing.T) {
	tests := []struct {
		decision   string
		reasonCode string
	}{
		{decision: "WAIT_USER", reasonCode: "PROCESSOR_WAIT_USER"},
		{decision: "MANUAL_REVIEW", reasonCode: "PROCESSOR_MANUAL_REVIEW"},
	}
	for _, test := range tests {
		t.Run(test.decision, func(t *testing.T) {
			recorder := &resultProcessorNotificationRecorder{}
			store := &Store{}
			store.SetTaskNotificationRecorder(recorder)
			scope := evaluationScope{TenantID: 101, TaskID: 102, TurnID: 103}

			occurredAt := time.Date(2026, 8, 30, 1, 2, 3, 0, time.UTC)
			if err := store.recordContinuationNotification(context.Background(), nil, scope, 104, 105, test.decision, occurredAt); err != nil {
				t.Fatalf("record continuation notification: %v", err)
			}
			if len(recorder.transitions) != 1 {
				t.Fatalf("recorded transitions=%d, want 1", len(recorder.transitions))
			}
			fact := recorder.transitions[0]
			if fact.TenantID != 101 || fact.TaskID != 102 || fact.TaskTurnID != 103 || fact.SourceID != 104 {
				t.Fatalf("fact identity=%+v", fact)
			}
			if fact.SourceType != tasknotification.SourceContinuationCheckpoint || fact.ReasonCode != test.reasonCode || fact.SourceOccurrence != 105 {
				t.Fatalf("fact contract=%+v", fact)
			}
			if !fact.OccurredAt.Equal(occurredAt) {
				t.Fatalf("occurredAt=%s, want %s", fact.OccurredAt, occurredAt)
			}
		})
	}
}

func TestRecordContinuationNotificationSkipsPassAndPropagatesRecorderFailure(t *testing.T) {
	recorder := &resultProcessorNotificationRecorder{}
	store := &Store{}
	store.SetTaskNotificationRecorder(recorder)
	scope := evaluationScope{TenantID: 101, TaskID: 102, TurnID: 103}

	occurredAt := time.Date(2026, 8, 30, 1, 2, 3, 0, time.UTC)
	if err := store.recordContinuationNotification(context.Background(), nil, scope, 104, 105, "PASS", occurredAt); err != nil {
		t.Fatalf("PASS notification hook: %v", err)
	}
	if len(recorder.transitions) != 0 {
		t.Fatalf("PASS recorded transitions=%d, want 0", len(recorder.transitions))
	}

	wantErr := errors.New("record fact failed")
	recorder.err = wantErr
	if err := store.recordContinuationNotification(context.Background(), nil, scope, 104, 105, "WAIT_USER", occurredAt); !errors.Is(err, wantErr) {
		t.Fatalf("recorder error=%v, want %v", err, wantErr)
	}
}

func TestReviewRuntimeClosureRejectsEveryZeroRowTransition(t *testing.T) {
	for _, component := range []string{"ATTEMPT", "RUNTIME", "EXECUTION_NODE", "ASSIGNMENT"} {
		t.Run(component, func(t *testing.T) {
			err := requireReviewRuntimeTransition(0, component)
			if err == nil || err.Error() != "CHECKPOINT_RUNTIME_"+component+"_STALE" {
				t.Fatalf("zero-row transition error=%v", err)
			}
		})
	}
	if err := requireReviewRuntimeTransition(1, "ATTEMPT"); err != nil {
		t.Fatalf("single-row transition: %v", err)
	}
	if err := requireReviewRuntimeTransition(2, "ATTEMPT"); err == nil {
		t.Fatal("multi-row transition must fail closed")
	}
}
