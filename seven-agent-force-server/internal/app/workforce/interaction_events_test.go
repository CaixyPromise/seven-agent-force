package workforce

import (
	"context"
	"errors"
	"strings"
	"testing"

	interaction "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/interaction/facade"
	notification "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/tasknotification/facade"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

type conversationEventTx struct {
	pgx.Tx
	writes  int
	payload string
}

func (t *conversationEventTx) Exec(_ context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	if strings.HasPrefix(sql, "INSERT INTO workforce_task_events") {
		t.writes++
		t.payload = string(args[4].([]byte))
	}
	return pgconn.NewCommandTag("INSERT 0 1"), nil
}
func (*conversationEventTx) QueryRow(context.Context, string, ...any) pgx.Row {
	return conversationEventRow{}
}

type conversationEventRow struct{}

func (conversationEventRow) Scan(dest ...any) error { *dest[0].(*int64) = 27; return nil }

type conversationAttentionRecorder struct {
	calls []notification.TaskNotificationTransition
	err   error
	tx    pgx.Tx
}

func (r *conversationAttentionRecorder) RecordTransition(_ context.Context, tx pgx.Tx, fact notification.TaskNotificationTransition) (notification.TransitionDisposition, error) {
	r.calls = append(r.calls, fact)
	r.tx = tx
	return "TASK_ACTION_REQUIRED", r.err
}

func TestConversationAttentionUsesQuestionOccurrenceAndSameTransaction(t *testing.T) {
	tx := &conversationEventTx{}
	recorder := &conversationAttentionRecorder{}
	r := interactionEvents{notifications: recorder}
	e := interaction.Event{TenantID: 1, TaskID: 2, TaskTurnID: 3, AttemptID: 4, ItemID: 9007199254740993, Version: 1, Type: "QUESTION", AttentionRequired: true}
	seq, err := r.RecordInteractionEvent(context.Background(), tx, e)
	if err != nil || seq != 27 || len(recorder.calls) != 1 || recorder.tx != tx {
		t.Fatalf("sequence=%d calls=%d err=%v", seq, len(recorder.calls), err)
	}
	f := recorder.calls[0]
	if f.SourceType != notification.SourceTaskAttempt || f.SourceID != 4 || f.SourceOccurrence != e.ItemID || f.ReasonCode != "AGENT_INPUT_REQUIRED" {
		t.Fatalf("wrong attention identity: %+v", f)
	}
	if !strings.Contains(tx.payload, `"itemId":"9007199254740993"`) || strings.Contains(tx.payload, "questionText") {
		t.Fatal("SSE must contain only safe lossless invalidation")
	}
	e.Version = 2
	e.AttentionRequired = false
	if _, err = r.RecordInteractionEvent(context.Background(), tx, e); err != nil || len(recorder.calls) != 1 {
		t.Fatal("answer/status update must not create another attention")
	}
}

func TestConversationAttentionFailureDoesNotAppendSuccessEvent(t *testing.T) {
	tx := &conversationEventTx{}
	recorder := &conversationAttentionRecorder{err: errors.New("notification unavailable")}
	_, err := (interactionEvents{notifications: recorder}).RecordInteractionEvent(context.Background(), tx, interaction.Event{TaskID: 2, AttentionRequired: true})
	if err == nil || tx.writes != 0 {
		t.Fatal("failed attention persistence must roll back the question transaction")
	}
}
