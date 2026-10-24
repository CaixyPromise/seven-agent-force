package infrastructure

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	tasknotification "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/tasknotification/facade"
)

type artifactNotificationRecorder struct {
	transitions []tasknotification.TaskNotificationTransition
	err         error
}

func (r *artifactNotificationRecorder) RecordTransition(_ context.Context, _ pgx.Tx, transition tasknotification.TaskNotificationTransition) (tasknotification.TransitionDisposition, error) {
	r.transitions = append(r.transitions, transition)
	return "TASK_COMPLETED", r.err
}

func TestRecordTaskCompletedNotificationUsesFinalizationIdentity(t *testing.T) {
	recorder := &artifactNotificationRecorder{}
	store := &Store{}
	store.SetTaskNotificationRecorder(recorder)

	occurredAt := time.Date(2026, 8, 30, 1, 2, 3, 0, time.UTC)
	if err := store.recordTaskCompletedNotification(context.Background(), nil, 101, 102, 103, 104, 105, occurredAt); err != nil {
		t.Fatalf("record completed notification: %v", err)
	}
	if len(recorder.transitions) != 1 {
		t.Fatalf("recorded transitions=%d, want 1", len(recorder.transitions))
	}
	fact := recorder.transitions[0]
	if fact.TenantID != 101 || fact.TaskID != 102 || fact.TaskTurnID != 103 || fact.SourceID != 104 {
		t.Fatalf("fact identity=%+v", fact)
	}
	if fact.SourceType != tasknotification.SourceFinalizationRun || fact.SourceOccurrence != 105 || fact.ReasonCode != "TASK_TURN_SUCCEEDED" {
		t.Fatalf("fact contract=%+v", fact)
	}
	if !fact.OccurredAt.Equal(occurredAt) {
		t.Fatalf("occurredAt=%s, want %s", fact.OccurredAt, occurredAt)
	}
}

func TestRecordTaskCompletedNotificationPropagatesRecorderFailure(t *testing.T) {
	wantErr := errors.New("record fact failed")
	recorder := &artifactNotificationRecorder{err: wantErr}
	store := &Store{}
	store.SetTaskNotificationRecorder(recorder)

	if err := store.recordTaskCompletedNotification(context.Background(), nil, 101, 102, 103, 104, 105, time.Now()); !errors.Is(err, wantErr) {
		t.Fatalf("recorder error=%v, want %v", err, wantErr)
	}
}
