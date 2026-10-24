package schemaalignment

import (
	"context"
	"fmt"
	principalinfra "github.com/CaixyPromise/seven-agent/force/internal/app/workforce/principal/infrastructure"
	"github.com/jackc/pgx/v5"
)

func verifyCompletionCheckSchema(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}) error {
	for _, column := range []struct {
		table, name, kind string
		length            int
	}{
		{"workforce_review_runs", "completion_check_digest", "character varying", 64},
		{"workforce_task_turns", "completion_check_digest", "character varying", 64},
		{"workforce_task_turns", "completion_check_input_digest", "character varying", 64},
		{"workforce_task_turns", "completion_check_baseline_result_id", "bigint", 0},
		{"workforce_task_turns", "completion_check_json", "jsonb", 0},
		{"workforce_task_turns", "completion_check_references_json", "jsonb", 0},
		{"workforce_completion_check_results", "id", "bigint", 0},
		{"workforce_completion_check_results", "source_identity_digest", "character varying", 64},
		{"workforce_completion_check_decisions", "baseline_result_id", "bigint", 0},
		{"workforce_completion_check_decisions", "actor_principal_id", "bigint", 0},
		{"workforce_completion_check_cycle_authorizations", "cycle_id", "bigint", 0},
	} {
		var matched bool
		err := q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name=$1 AND column_name=$2 AND data_type=$3 AND COALESCE(character_maximum_length,0)=$4)`, column.table, column.name, column.kind, column.length).Scan(&matched)
		if err != nil {
			return err
		}
		if !matched {
			return fmt.Errorf("completion check schema mismatch: %s.%s", column.table, column.name)
		}
	}
	var constrained bool
	if err := q.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_constraint WHERE conrelid='workforce_review_runs'::regclass AND conname='workforce_review_runs_completion_check_digest_ck' AND contype='c') AND EXISTS(SELECT 1 FROM information_schema.columns WHERE table_name='workforce_review_runs' AND column_name='completion_check_digest' AND is_nullable='NO' AND column_default LIKE '%''%')`).Scan(&constrained); err != nil {
		return err
	}
	if !constrained {
		return fmt.Errorf("completion check review digest constraints missing")
	}
	var validActors bool
	if err := q.QueryRow(ctx, `SELECT NOT EXISTS(SELECT 1 FROM workforce_completion_check_decisions d LEFT JOIN workforce_principals p ON p.id=d.actor_principal_id WHERE p.id IS NULL OR p.principal_type<>'USER') AND NOT EXISTS(SELECT 1 FROM information_schema.columns WHERE table_schema='public' AND table_name='workforce_completion_check_decisions' AND column_name='actor_user_id')`).Scan(&validActors); err != nil {
		return err
	}
	if !validActors {
		return fmt.Errorf("completion check actor principal integrity mismatch")
	}
	return nil
}

// The legacy column contains numeric Framework user IDs, not synthetic actor
// strings. Resolve each USER through the same facade as live decisions; never
// rename/copy those values into the principal identity column.
func alignCompletionDecisionPrincipals(ctx context.Context, tx pgx.Tx, ids IDGenerator) (int64, error) {
	exists, err := columnExists(ctx, tx, "workforce_completion_check_decisions", "actor_user_id")
	if err != nil || !exists {
		return 0, err
	}
	if _, err = tx.Exec(ctx, `ALTER TABLE workforce_completion_check_decisions ADD COLUMN IF NOT EXISTS actor_principal_id BIGINT`); err != nil {
		return 0, err
	}
	rows, err := tx.Query(ctx, `SELECT DISTINCT actor_user_id FROM workforce_completion_check_decisions ORDER BY actor_user_id`)
	if err != nil {
		return 0, err
	}
	var users []int64
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		users = append(users, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return 0, err
	}
	resolver := principalinfra.NewResolver()
	var changed int64
	for _, user := range users {
		principal, resolveErr := resolver.EnsureUser(ctx, tx, ids, user)
		if resolveErr != nil {
			return changed, resolveErr
		}
		tag, updateErr := tx.Exec(ctx, `UPDATE workforce_completion_check_decisions SET actor_principal_id=$2 WHERE actor_user_id=$1`, user, principal)
		if updateErr != nil {
			return changed, updateErr
		}
		changed += tag.RowsAffected()
	}
	_, err = tx.Exec(ctx, `ALTER TABLE workforce_completion_check_decisions ALTER COLUMN actor_principal_id SET NOT NULL, ADD CONSTRAINT workforce_completion_check_decisions_actor_principal_ck CHECK(actor_principal_id>0), DROP COLUMN actor_user_id`)
	return changed, err
}
