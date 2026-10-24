package infrastructure

import (
	"context"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/interaction/domain"
	"github.com/CaixyPromise/seven-agent/force/internal/app/workforce/interaction/facade"
	"github.com/jackc/pgx/v5"
)

// Summaries uses two bounded set queries in the same transaction. Reading
// watermark never consumes an OPEN question or modifies read-state.
func (s *Store) Summaries(ctx context.Context, tx pgx.Tx, tenant, principal int64, tasks, active []int64) (map[int64]facade.TaskSummary, error) {
	rows, e := tx.Query(ctx, `WITH requested AS (SELECT unnest($2::bigint[]) task_id),
 question_pending AS (SELECT task_id,id,'CONVERSATION_ITEM'::text kind FROM workforce_agent_interactions WHERE tenant_id=$1 AND task_id=ANY($3::bigint[]) AND kind='QUESTION' AND state='OPEN' AND expires_at>CURRENT_TIMESTAMP),
 operation_pending AS (SELECT task_id,id,'CONVERSATION_ITEM'::text kind FROM workforce_task_conversation_items WHERE tenant_id=$1 AND task_id=ANY($3::bigint[]) AND type='OPERATION' AND body_json->>'status'='WAITING_APPROVAL'),
 delivery_pending AS (SELECT task_id,id,'DELIVERY_REQUEST'::text kind FROM workforce_delivery_requests WHERE tenant_id=$1 AND task_id=ANY($2::bigint[]) AND status='WAITING_APPROVAL'),
 pending_rows AS (SELECT * FROM question_pending UNION ALL SELECT * FROM operation_pending UNION ALL SELECT * FROM delivery_pending),
 pending AS (SELECT task_id,count(*) n,(array_agg(kind ORDER BY id))[1] first_kind,(array_agg(id ORDER BY id))[1] first_id FROM pending_rows GROUP BY task_id)
 SELECT r.task_id,COALESCE(p.n,0),COALESCE(p.first_kind,''),COALESCE(p.first_id,0) FROM requested r LEFT JOIN pending p ON p.task_id=r.task_id`, tenant, tasks, active)
	if e != nil {
		return nil, e
	}
	defer rows.Close()
	out := map[int64]facade.TaskSummary{}
	for rows.Next() {
		var id int64
		var s facade.TaskSummary
		if e = rows.Scan(&id, &s.PendingActionCount, &s.FirstPendingActionKind, &s.FirstPendingActionID); e != nil {
			return nil, e
		}
		if (s.FirstPendingActionKind == "") != (s.FirstPendingActionID == 0) || (s.FirstPendingActionKind != "" && s.FirstPendingActionKind != facade.PendingConversationItem && s.FirstPendingActionKind != facade.PendingCompletionCheck && s.FirstPendingActionKind != facade.PendingDeliveryRequest) {
			return nil, domain.ErrInvalid
		}
		out[id] = s
	}
	if e = rows.Err(); e != nil {
		return nil, e
	}
	rows.Close()
	checks, e := tx.Query(ctx, `SELECT ec.task_id,count(*),min(c.id)
 FROM workforce_result_evaluation_cycles ec
 JOIN workforce_continuation_checkpoints c ON c.task_id=ec.task_id AND c.task_turn_id=ec.task_turn_id AND c.attempt_id=ec.attempt_id AND c.evaluation_cycle_id=ec.id
 WHERE ec.task_id=ANY($1::bigint[]) AND ec.decision='WAIT_USER' AND ec.status='PERSISTED'
   AND c.purpose='RESULT' AND c.decision='WAIT_USER' AND c.status='AVAILABLE' AND c.format_version=2 AND c.consumed_by_turn_id IS NULL AND c.expires_at>CURRENT_TIMESTAMP
 GROUP BY ec.task_id`, active)
	if e != nil {
		return nil, e
	}
	defer checks.Close()
	for checks.Next() {
		var id, first int64
		var count int
		if e = checks.Scan(&id, &count, &first); e != nil {
			return nil, e
		}
		value, ok := out[id]
		if !ok {
			return nil, domain.ErrInvalid
		}
		value.PendingActionCount += count
		if value.FirstPendingActionID == 0 || first < value.FirstPendingActionID {
			value.FirstPendingActionKind = facade.PendingCompletionCheck
			value.FirstPendingActionID = first
		}
		out[id] = value
	}
	if e = checks.Err(); e != nil {
		return nil, e
	}
	checks.Close()
	unread, e := tx.Query(ctx, `WITH requested AS (SELECT unnest($3::bigint[]) task_id),
 changes AS (SELECT task_id,max(change_sequence) watermark FROM workforce_task_conversation_items WHERE tenant_id=$1 AND task_id=ANY($3::bigint[]) GROUP BY task_id)
 SELECT r.task_id,COALESCE(c.watermark,0)>COALESCE(rs.watermark,0) FROM requested r LEFT JOIN changes c ON c.task_id=r.task_id LEFT JOIN workforce_task_conversation_read_states rs ON rs.task_id=r.task_id AND rs.tenant_id=$1 AND rs.actor_principal_id=$2`, tenant, principal, tasks)
	if e != nil {
		return nil, e
	}
	defer unread.Close()
	for unread.Next() {
		var id int64
		var hasUnread bool
		if e = unread.Scan(&id, &hasUnread); e != nil {
			return nil, e
		}
		value := out[id]
		value.HasUnreadUpdates = hasUnread
		out[id] = value
	}
	if e = unread.Err(); e != nil {
		return nil, e
	}
	unread.Close()
	activity, e := tx.Query(ctx, `SELECT DISTINCT ON(task_id) task_id,left(COALESCE(body_json->>'text',''),512) FROM workforce_task_conversation_items WHERE tenant_id=$1 AND task_id=ANY($2::bigint[]) AND type IN ('ACTIVITY','ASSISTANT_MESSAGE') ORDER BY task_id,id DESC`, tenant, tasks)
	if e != nil {
		return nil, e
	}
	defer activity.Close()
	for activity.Next() {
		var id int64
		var summary string
		if e = activity.Scan(&id, &summary); e != nil {
			return nil, e
		}
		value, ok := out[id]
		if ok {
			value.ActivitySummary = summary
			out[id] = value
		}
	}
	return out, activity.Err()
}
