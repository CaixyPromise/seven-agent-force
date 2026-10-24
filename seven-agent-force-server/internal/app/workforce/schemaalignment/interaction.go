package schemaalignment

import (
	"context"
	"fmt"
	"github.com/jackc/pgx/v5"
)

func verifyInteractionSchema(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}) error {
	for _, table := range []string{"workforce_agent_interactions", "workforce_agent_interaction_responses", "workforce_task_conversation_items", "workforce_task_conversation_read_states", "workforce_completion_check_revisions"} {
		for _, column := range []string{"id", "tenant_id", "task_id"} {
			var ok bool
			if err := q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name=$1 AND column_name=$2 AND data_type='bigint' AND is_nullable='NO')`, table, column).Scan(&ok); err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("interaction schema identity mismatch: %s.%s", table, column)
			}
		}
	}
	var blockingColumn bool
	if err := q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name='workforce_agent_interactions' AND column_name='is_blocking' AND data_type='boolean' AND is_nullable='NO')`).Scan(&blockingColumn); err != nil {
		return err
	}
	if !blockingColumn {
		return fmt.Errorf("interaction schema column mismatch: workforce_agent_interactions.is_blocking")
	}
	// Each independent relationship has a bounded logical read. Keep these
	// statements separate rather than joining unrelated ownership graphs.
	checks := []string{
		`SELECT NOT EXISTS(SELECT 1 FROM workforce_agent_interactions WHERE kind='QUESTION' AND body_json->'isBlocking' IS DISTINCT FROM to_jsonb(is_blocking))`,
		`SELECT NOT EXISTS(SELECT 1 FROM workforce_agent_interactions i LEFT JOIN workforce_tasks t ON t.id=i.task_id LEFT JOIN workforce_principals p ON p.id=i.actor_principal_id WHERE t.id IS NULL OR t.tenant_id<>i.tenant_id OR p.id IS NULL)`,
		`SELECT NOT EXISTS(SELECT 1 FROM workforce_agent_interaction_responses r LEFT JOIN workforce_agent_interactions i ON i.id=r.interaction_id LEFT JOIN workforce_principals p ON p.id=r.actor_principal_id WHERE i.id IS NULL OR i.tenant_id<>r.tenant_id OR i.task_id<>r.task_id OR p.id IS NULL OR p.principal_type<>'USER')`,
		`SELECT NOT EXISTS(SELECT 1 FROM workforce_task_conversation_items i LEFT JOIN workforce_tasks t ON t.id=i.task_id WHERE t.id IS NULL OR t.tenant_id<>i.tenant_id)`,
		`SELECT NOT EXISTS(SELECT 1 FROM workforce_task_conversation_read_states r LEFT JOIN workforce_tasks t ON t.id=r.task_id LEFT JOIN workforce_principals p ON p.id=r.actor_principal_id WHERE t.id IS NULL OR t.tenant_id<>r.tenant_id OR p.id IS NULL OR p.principal_type<>'USER')`,
		`SELECT NOT EXISTS(SELECT 1 FROM pg_constraint WHERE contype='f' AND conrelid IN('workforce_agent_interactions'::regclass,'workforce_agent_interaction_responses'::regclass,'workforce_task_conversation_items'::regclass,'workforce_task_conversation_read_states'::regclass,'workforce_completion_check_revisions'::regclass))`,
		`SELECT NOT EXISTS(SELECT 1 FROM workforce_completion_check_revisions r LEFT JOIN workforce_task_turns tr ON tr.id=r.source_task_turn_id LEFT JOIN workforce_principals p ON p.id=r.actor_principal_id WHERE tr.id IS NULL OR tr.task_id<>r.task_id OR tr.tenant_id<>r.tenant_id OR p.id IS NULL OR p.principal_type<>'USER')`,
		`SELECT NOT EXISTS(SELECT 1 FROM workforce_continuation_checkpoints c LEFT JOIN workforce_agent_interactions i ON i.id=c.interaction_id WHERE c.purpose='INTERACTION_SUSPEND' AND (i.id IS NULL OR i.task_id<>c.task_id OR i.attempt_id<>c.attempt_id OR c.decision<>'' OR c.evaluation_cycle_id IS NOT NULL))`,
		`SELECT NOT EXISTS(SELECT 1 FROM workforce_task_turns tr LEFT JOIN workforce_agent_interaction_responses r ON r.id=tr.source_interaction_response_id LEFT JOIN workforce_continuation_checkpoints c ON c.id=tr.source_checkpoint_id WHERE tr.source_interaction_response_id IS NOT NULL AND (r.id IS NULL OR r.task_id<>tr.task_id OR r.tenant_id<>tr.tenant_id OR c.id IS NULL OR c.purpose<>'INTERACTION_SUSPEND' OR c.consumed_by_turn_id<>tr.id OR c.interaction_id<>r.interaction_id))`,
	}
	for index, query := range checks {
		var valid bool
		if err := q.QueryRow(ctx, query).Scan(&valid); err != nil {
			return err
		}
		if !valid {
			return fmt.Errorf("interaction relationship integrity mismatch: check %d", index+1)
		}
	}
	return nil
}
