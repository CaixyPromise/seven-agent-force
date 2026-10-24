package workforce

import (
	"context"
	"errors"
	"strconv"
	"time"

	interaction "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/interaction/facade"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/shared/eventlog"
	notification "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/tasknotification/facade"
	"github.com/jackc/pgx/v5"
)

// Conversation updates share the existing committed Task event stream. Content
// remains behind the Task-authorized read model; SSE carries only invalidation.
type interactionEvents struct {
	notifications notification.TransactionalRecorder
}

func (r interactionEvents) RecordInteractionEvent(ctx context.Context, tx pgx.Tx, e interaction.Event) (int64, error) {
	if e.AttentionRequired {
		if r.notifications == nil {
			return 0, errors.New("INTERACTION_UNAVAILABLE")
		}
		reasonCode, summary := "AGENT_INPUT_REQUIRED", "Agent 需要你回答一个问题"
		if e.Type == "OPERATION" {
			reasonCode, summary = "AGENT_OPERATION_APPROVAL_REQUIRED", "Agent 需要访问外部服务"
		}
		_, err := r.notifications.RecordTransition(ctx, tx, notification.TaskNotificationTransition{
			TenantID: e.TenantID, TaskID: e.TaskID, TaskTurnID: e.TaskTurnID,
			SourceType: notification.SourceTaskAttempt, SourceID: e.AttemptID, SourceOccurrence: e.ItemID,
			ReasonCode: reasonCode, Recoverability: notification.RecoverabilityUserRecoverable,
			Summary: summary, OccurredAt: time.Now().UTC(),
		})
		if err != nil {
			return 0, err
		}
	}
	return eventlog.AppendWithSequence(ctx, tx, e.TaskID, e.AttemptID, "conversation.updated", map[string]string{
		"itemId":  strconv.FormatInt(e.ItemID, 10),
		"version": strconv.FormatInt(e.Version, 10),
		"kind":    e.Type,
	})
}
