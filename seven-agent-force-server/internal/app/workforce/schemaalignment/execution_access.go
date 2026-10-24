package schemaalignment

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

func ensureExecutionAccessContinuationSchema(ctx context.Context, tx pgx.Tx) error {
	statements := []string{
		`ALTER TABLE workforce_execution_operations ADD COLUMN IF NOT EXISTS native_tool_call_id TEXT`,
		`UPDATE workforce_execution_operations SET native_tool_call_id=local_tool_call_key WHERE native_tool_call_id IS NULL`,
		`ALTER TABLE workforce_execution_operations ALTER COLUMN native_tool_call_id SET NOT NULL`,
		`ALTER TABLE workforce_execution_operations DROP CONSTRAINT IF EXISTS workforce_execution_operations_native_tool_call_id_ck`,
		`ALTER TABLE workforce_execution_operations ADD CONSTRAINT workforce_execution_operations_native_tool_call_id_ck CHECK (octet_length(native_tool_call_id) BETWEEN 1 AND 512 AND native_tool_call_id ~ '^[A-Za-z0-9][A-Za-z0-9._:-]*$')`,
		`ALTER TABLE workforce_task_turns ADD COLUMN IF NOT EXISTS source_execution_operation_id BIGINT`,
		`ALTER TABLE workforce_task_turns DROP CONSTRAINT IF EXISTS workforce_task_turns_source_execution_operation_ck`,
		`ALTER TABLE workforce_task_turns ADD CONSTRAINT workforce_task_turns_source_execution_operation_ck CHECK (source_execution_operation_id IS NULL OR (source_execution_operation_id>0 AND source_checkpoint_id IS NOT NULL))`,
		`CREATE UNIQUE INDEX IF NOT EXISTS workforce_turn_execution_operation_once_idx ON workforce_task_turns(source_execution_operation_id) WHERE source_execution_operation_id IS NOT NULL`,
	}
	for _, statement := range statements {
		if _, err := tx.Exec(ctx, statement); err != nil {
			return fmt.Errorf("align execution access continuation schema: %w", err)
		}
	}
	return nil
}

func verifyExecutionAccessSchema(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}) error {
	var nativeColumnType, nativeColumnNullable string
	if err := q.QueryRow(ctx, `SELECT data_type,is_nullable FROM information_schema.columns WHERE table_schema='public' AND table_name='workforce_execution_operations' AND column_name='native_tool_call_id'`).Scan(&nativeColumnType, &nativeColumnNullable); err != nil {
		return fmt.Errorf("execution access native tool call identity column mismatch: %w", err)
	}
	if nativeColumnType != "text" || nativeColumnNullable != "NO" {
		return fmt.Errorf("execution access native tool call identity column has %s/%s, want text/NO", nativeColumnType, nativeColumnNullable)
	}
	var nativeConstraint string
	if err := q.QueryRow(ctx, `SELECT pg_get_constraintdef(oid) FROM pg_constraint WHERE conrelid='workforce_execution_operations'::regclass AND conname='workforce_execution_operations_native_tool_call_id_ck' AND contype='c' AND convalidated`).Scan(&nativeConstraint); err != nil {
		return fmt.Errorf("execution access native tool call identity constraint mismatch: %w", err)
	}
	for _, token := range []string{"native_tool_call_id", "octet_length", "512", "A-Za-z0-9"} {
		if !strings.Contains(nativeConstraint, token) {
			return fmt.Errorf("execution access native tool call identity constraint is missing %q", token)
		}
	}
	var turnColumnType, turnColumnNullable string
	if err := q.QueryRow(ctx, `SELECT data_type,is_nullable FROM information_schema.columns WHERE table_schema='public' AND table_name='workforce_task_turns' AND column_name='source_execution_operation_id'`).Scan(&turnColumnType, &turnColumnNullable); err != nil {
		return fmt.Errorf("execution access continuation column mismatch: %w", err)
	}
	if turnColumnType != "bigint" || turnColumnNullable != "YES" {
		return fmt.Errorf("execution access continuation column has %s/%s, want bigint/YES", turnColumnType, turnColumnNullable)
	}
	var indexDefinition string
	if err := q.QueryRow(ctx, `SELECT pg_get_indexdef(i.oid) FROM pg_class i JOIN pg_index x ON x.indexrelid=i.oid WHERE i.relnamespace='public'::regnamespace AND i.relname='workforce_turn_execution_operation_once_idx' AND x.indisunique AND x.indisvalid AND x.indisready`).Scan(&indexDefinition); err != nil {
		return fmt.Errorf("execution access continuation index mismatch: %w", err)
	}
	for _, token := range []string{"source_execution_operation_id", "WHERE", "IS NOT NULL"} {
		if !strings.Contains(indexDefinition, token) {
			return fmt.Errorf("execution access continuation index is missing %q", token)
		}
	}
	tables := []string{
		"workforce_execution_operations",
		"workforce_execution_operation_decisions",
		"workforce_execution_operation_attempts",
		"workforce_task_network_grants",
	}
	for _, table := range tables {
		for _, column := range []string{"id", "tenant_id", "task_id"} {
			var ok bool
			if err := q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name=$1 AND column_name=$2 AND data_type='bigint' AND is_nullable='NO')`, table, column).Scan(&ok); err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("execution access schema identity mismatch: %s.%s", table, column)
			}
		}
	}
	checks := []string{
		`SELECT NOT EXISTS(SELECT 1 FROM pg_constraint WHERE contype='f' AND conrelid IN('workforce_execution_operations'::regclass,'workforce_execution_operation_decisions'::regclass,'workforce_execution_operation_attempts'::regclass,'workforce_task_network_grants'::regclass))`,
		`SELECT NOT EXISTS(SELECT 1 FROM pg_constraint WHERE contype='f' AND conrelid='workforce_task_turns'::regclass AND pg_get_constraintdef(oid) LIKE '%source_execution_operation_id%')`,
		`SELECT NOT EXISTS(SELECT 1 FROM workforce_execution_operations o LEFT JOIN workforce_tasks t ON t.id=o.task_id LEFT JOIN workforce_task_turns tr ON tr.id=o.task_turn_id WHERE t.id IS NULL OR t.tenant_id<>o.tenant_id OR tr.id IS NULL OR tr.task_id<>o.task_id)`,
		`SELECT NOT EXISTS(SELECT 1 FROM workforce_execution_operations o LEFT JOIN workforce_task_attempts a ON a.id=o.attempt_id WHERE a.id IS NULL OR a.task_id<>o.task_id OR a.task_turn_id<>o.task_turn_id)`,
		`SELECT NOT EXISTS(SELECT 1 FROM workforce_execution_operation_decisions d LEFT JOIN workforce_execution_operations o ON o.id=d.operation_id LEFT JOIN workforce_principals p ON p.id=d.actor_principal_id WHERE o.id IS NULL OR o.tenant_id<>d.tenant_id OR o.task_id<>d.task_id OR p.id IS NULL OR p.principal_type<>'USER')`,
		`SELECT NOT EXISTS(SELECT 1 FROM workforce_task_network_grants g LEFT JOIN workforce_tasks t ON t.id=g.task_id LEFT JOIN workforce_execution_operation_decisions d ON d.id=g.source_decision_id WHERE t.id IS NULL OR t.tenant_id<>g.tenant_id OR d.id IS NULL OR d.tenant_id<>g.tenant_id OR d.task_id<>g.task_id)`,
		`SELECT NOT EXISTS(SELECT 1 FROM workforce_task_network_grants g LEFT JOIN workforce_principals p ON p.id=g.actor_principal_id WHERE p.id IS NULL OR p.principal_type<>'USER')`,
		`SELECT NOT EXISTS(SELECT 1 FROM workforce_execution_operation_attempts a LEFT JOIN workforce_execution_operations o ON o.id=a.operation_id WHERE o.id IS NULL OR o.tenant_id<>a.tenant_id OR o.task_id<>a.task_id)`,
		`SELECT NOT EXISTS(SELECT 1 FROM workforce_task_turns tr LEFT JOIN workforce_execution_operations o ON o.id=tr.source_execution_operation_id LEFT JOIN workforce_continuation_checkpoints c ON c.id=tr.source_checkpoint_id WHERE tr.source_execution_operation_id IS NOT NULL AND (o.id IS NULL OR o.tenant_id<>tr.tenant_id OR o.task_id<>tr.task_id OR c.id IS NULL OR c.task_id<>tr.task_id OR c.purpose<>'INTERACTION_SUSPEND' OR c.operation_id<>tr.source_execution_operation_id OR c.consumed_by_turn_id<>tr.id))`,
		`SELECT NOT EXISTS(SELECT 1 FROM workforce_continuation_checkpoints c LEFT JOIN workforce_task_turns tr ON tr.id=c.consumed_by_turn_id WHERE c.purpose='INTERACTION_SUSPEND' AND c.operation_id IS NOT NULL AND c.consumed_by_turn_id IS NOT NULL AND (tr.id IS NULL OR tr.task_id<>c.task_id OR tr.source_checkpoint_id<>c.id OR tr.source_execution_operation_id<>c.operation_id))`,
		`SELECT NOT EXISTS(SELECT 1 FROM workforce_execution_operations WHERE octet_length(native_tool_call_id) NOT BETWEEN 1 AND 512 OR native_tool_call_id !~ '^[A-Za-z0-9][A-Za-z0-9._:-]*$')`,
	}
	for index, query := range checks {
		var ok bool
		if err := q.QueryRow(ctx, query).Scan(&ok); err != nil {
			return err
		}
		if !ok {
			return fmt.Errorf("execution access relationship integrity mismatch: check %d", index+1)
		}
	}
	return nil
}
